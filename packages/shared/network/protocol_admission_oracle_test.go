package network

// This file is the Go half of the node's paired admission evidence. It observes
// the real `BeginServerLogin` decision for semantically invalid inbound values
// that the Go outbound encoder cannot produce, so the rejection order this
// package owns is pinned by the driver itself rather than by a restatement of
// its decision tree.
//
// The case identities are the same strings the Rust admission suite in
// `packages/engine/crates/mornlea_protocol/tests/protocol_admission.rs` carries,
// so a reviewer can match the two suites case by case. The corpus producer in
// `packages/tools/cmd/runtime-oracle` is deliberately not involved here: this
// file belongs to the package that owns the decision, hand-builds only the
// canonical invalid-inbound wire bytes, decodes them through the production
// inbound codec, and hands the resulting DTO to the existing login seams.

import (
	"context"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// protocolAdmissionPlayerID is the identity the login cases carry: version
// nibble 4 and variant bits 10, which the UUIDv4 rule admits.
var protocolAdmissionPlayerID = core.PlayerID{
	0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15,
}

// appendAdmissionUvarint renders one canonical unsigned varint, matching the
// production encoder's primitive so a hand-built payload is canonical wire.
func appendAdmissionUvarint(dst []byte, value uint32) []byte {
	for value >= 1<<7 {
		dst = append(dst, byte(value)|0x80)
		value >>= 7
	}
	return append(dst, byte(value))
}

// admissionHelloPayload is the canonical ClientHello payload for one version.
func admissionHelloPayload(version uint32) []byte {
	return appendAdmissionUvarint(nil, version)
}

// admissionLoginPayload is the canonical LoginStart payload for one identity,
// raw name and declared view distance.
func admissionLoginPayload(id core.PlayerID, name string, viewDistance uint8) []byte {
	payload := append([]byte(nil), id[:]...)
	payload = appendAdmissionUvarint(payload, uint32(len(name)))
	payload = append(payload, []byte(name)...)
	return append(payload, viewDistance)
}

// decodeAdmissionHello decodes one hand-built hello through the production
// inbound codec.
//
// The Go inbound decoder passes a structurally valid hello through without
// applying the version policy, which is what lets the driver answer a peer
// running another version with the frozen mismatch response instead of a bare
// decode failure.
func decodeAdmissionHello(t *testing.T, payload []byte) protocol.ClientHello {
	t.Helper()
	wireCodec, err := codec.NewCodec()
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeClient(protocol.StateHandshake, 0, payload)
	if err != nil {
		t.Fatalf("inbound hello decode: %v", err)
	}
	hello, ok := packet.(protocol.ClientHello)
	if !ok {
		t.Fatalf("inbound hello decode produced %T, want protocol.ClientHello", packet)
	}
	return hello
}

// decodeAdmissionLoginStart decodes one hand-built login start through the
// production inbound codec, which admits the raw name and the raw identity so
// the driver decides them.
func decodeAdmissionLoginStart(t *testing.T, payload []byte) protocol.LoginStart {
	t.Helper()
	wireCodec, err := codec.NewCodec()
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeClient(protocol.StateLogin, 0, payload)
	if err != nil {
		t.Fatalf("inbound login decode: %v", err)
	}
	start, ok := packet.(protocol.LoginStart)
	if !ok {
		t.Fatalf("inbound login decode produced %T, want protocol.LoginStart", packet)
	}
	return start
}

// protocolAdmissionRow is one paired case: the hand-built canonical wire bytes,
// the decision this package's driver observes, and the outcome the Rust
// admission suite has to reach for the same identity.
type protocolAdmissionRow struct {
	// id is shared with the Rust admission suite.
	id string
	// wantRust names the Rust-side outcome the same record has to produce.
	wantRust string
	// payload is the canonical inbound wire payload.
	payload []byte
	// wantRejectCode is the login rejection code the driver answers with; zero
	// means the login is admitted.
	wantRejectCode protocol.LoginRejectCode
	// wantName is the canonical display name an admitted login carries.
	wantName string
	// wantDistance is the declared view distance an admitted login keeps.
	wantDistance uint8
}

// protocolAdmissionLoginRows is the paired login admission table.
//
// Every row decodes its payload through the production inbound codec first, so
// a row can only reach the driver with bytes the real wire accepts. The order
// rows two and three pin is the Go decision order: identity before distance.
func protocolAdmissionLoginRows() []protocolAdmissionRow {
	return []protocolAdmissionRow{
		{
			id:             "login-zero-id-distance-one",
			wantRust:       "InvalidIdentity (identity before distance)",
			payload:        admissionLoginPayload(core.PlayerID{}, "Alice", 1),
			wantRejectCode: LoginInvalidIdentity,
		},
		{
			id:             "login-distance-one",
			wantRust:       "ProtocolViolation",
			payload:        admissionLoginPayload(protocolAdmissionPlayerID, "Alice", 1),
			wantRejectCode: LoginProtocolViolation,
		},
		{
			id:             "login-distance-65",
			wantRust:       "ProtocolViolation",
			payload:        admissionLoginPayload(protocolAdmissionPlayerID, "Alice", 65),
			wantRejectCode: LoginProtocolViolation,
		},
		{
			id:           "login-name-trimmed",
			wantRust:     "admitted \"Alice\" with unchanged distance 2",
			payload:      admissionLoginPayload(protocolAdmissionPlayerID, "  Alice  ", 2),
			wantName:     "Alice",
			wantDistance: 2,
		},
		{
			id: "login-long-raw-name",
			// The canonical byte bound applies after the trim, so this raw name
			// is admitted instead of being refused by the raw length.
			wantRust:     "admitted \"Alice\" with unchanged distance 2",
			payload:      admissionLoginPayload(protocolAdmissionPlayerID, strings.Repeat(" ", 128)+"Alice", 2),
			wantName:     "Alice",
			wantDistance: 2,
		},
		{
			id:             "login-33-scalars",
			wantRust:       "InvalidIdentity",
			payload:        admissionLoginPayload(protocolAdmissionPlayerID, strings.Repeat("a", 33), 2),
			wantRejectCode: LoginInvalidIdentity,
		},
		{
			id:             "login-control-character",
			wantRust:       "InvalidIdentity",
			payload:        admissionLoginPayload(protocolAdmissionPlayerID, "Ali\x01ce", 2),
			wantRejectCode: LoginInvalidIdentity,
		},
	}
}

// TestProtocolAdmissionOracleLoginDriverDecidesPairedRows runs the paired
// admission table through the real login driver and pins the rejection code and
// the admitted identity for each case.
func TestProtocolAdmissionOracleLoginDriverDecidesPairedRows(t *testing.T) {
	for _, row := range protocolAdmissionLoginRows() {
		t.Run(row.id, func(t *testing.T) {
			start := decodeAdmissionLoginStart(t, row.payload)
			stream := &staticLoginStartStream{start: start}
			pending, err := BeginServerLogin(context.Background(), stream, 0)

			if row.wantRejectCode == 0 {
				if err != nil {
					t.Fatalf("case %s: BeginServerLogin rejected an admissible login: %v", row.id, err)
				}
				if pending == nil {
					t.Fatalf("case %s: no pending login for an admissible record", row.id)
				}
				if got := pending.Identity().DisplayName; got != row.wantName {
					t.Fatalf("case %s: admitted name = %q, want %q (%s)", row.id, got, row.wantName, row.wantRust)
				}
				if got := pending.ViewDistance(); got != row.wantDistance {
					t.Fatalf("case %s: admitted distance = %d, want %d", row.id, got, row.wantDistance)
				}
				if err := pending.Reject(context.Background(), LoginServerFull, "admitted; closing the seam"); err != nil {
					t.Fatalf("case %s: close pending login: %v", row.id, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("case %s: driver admitted a record that must be rejected", row.id)
			}
			if pending != nil {
				t.Fatalf("case %s: rejected login returned a pending value", row.id)
			}
			reject, ok := stream.sent.(LoginReject)
			if !ok || stream.sentState != StateLogin || reject.Code != row.wantRejectCode {
				t.Fatalf("case %s: rejection = %#v (state %d), want LoginReject code %d (%s)",
					row.id, stream.sent, stream.sentState, row.wantRejectCode, row.wantRust)
			}
		})
	}
}

// TestProtocolAdmissionOracleHelloVersionMismatch pins that a structurally valid
// hello naming the previous protocol version is answered with the negotiated
// version pair, which is the outcome the Rust admission suite reaches through
// its own pure version-negotiation function.
func TestProtocolAdmissionOracleHelloVersionMismatch(t *testing.T) {
	const previousVersion = uint32(44)
	hello := decodeAdmissionHello(t, admissionHelloPayload(previousVersion))
	if hello.ProtocolVersion != previousVersion {
		t.Fatalf("inbound hello version = %d, want %d: the structural decoder must keep the peer's version", hello.ProtocolVersion, previousVersion)
	}

	stream := &staticClientHelloStream{version: hello.ProtocolVersion}
	if _, err := BeginServerLogin(context.Background(), stream, 0); err == nil {
		t.Fatal("hello-version-44: BeginServerLogin accepted a prior protocol version")
	}
	reject, ok := stream.sent.(HandshakeReject)
	if !ok || stream.sentState != StateHandshake ||
		reject.ServerProtocolVersion != ProtocolVersion ||
		reject.Code != HandshakeVersionMismatch {
		t.Fatalf("hello-version-44: rejection = %#v (state %d), want HandshakeReject version %d code %d (Rust: VersionMismatch{server_version:%d})",
			stream.sent, stream.sentState, ProtocolVersion, HandshakeVersionMismatch, ProtocolVersion)
	}
}

// TestProtocolAdmissionOracleOutboundRawNameBound pins the asymmetry this node's
// evidence rests on: the Go outbound encoder refuses a raw display name beyond
// 128 bytes, so that value cannot become a two-way corpus case and stays in this
// paired table, where the same value is admitted inbound after the trim.
func TestProtocolAdmissionOracleOutboundRawNameBound(t *testing.T) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	overLong := strings.Repeat(" ", 128) + "Alice"
	if len(overLong) <= 128 {
		t.Fatalf("over-long raw name is %d bytes, want more than 128", len(overLong))
	}
	if _, _, err := wireCodec.EncodeClient(protocol.StateLogin, LoginStart{
		PlayerID:     protocolAdmissionPlayerID,
		DisplayName:  overLong,
		ViewDistance: 2,
	}); err == nil {
		t.Fatal("the outbound encoder accepted a raw display name beyond 128 bytes")
	}

	// The same value decodes inbound and is admitted by the driver, which is
	// the reason the raw bound cannot be applied before the trim.
	start := decodeAdmissionLoginStart(t, admissionLoginPayload(protocolAdmissionPlayerID, overLong, 2))
	stream := &staticLoginStartStream{start: start}
	pending, err := BeginServerLogin(context.Background(), stream, 0)
	if err != nil {
		t.Fatalf("login-long-raw-name: driver rejected an admissible record: %v", err)
	}
	if got := pending.Identity().DisplayName; got != "Alice" {
		t.Fatalf("login-long-raw-name: admitted name = %q, want %q", got, "Alice")
	}
	if err := pending.Reject(context.Background(), LoginServerFull, "admitted; closing the seam"); err != nil {
		t.Fatalf("login-long-raw-name: close pending login: %v", err)
	}
}

// TestProtocolAdmissionOracleStructuralRejectionsStayAtTheCodec pins the four
// structural boundaries the Rust suite also carries: they reject inside the
// production inbound codec, so the driver never observes a decision for them.
func TestProtocolAdmissionOracleStructuralRejectionsStayAtTheCodec(t *testing.T) {
	rows := []struct {
		id      string
		state   protocol.State
		payload []byte
	}{
		{
			id:      "hello-trailing-byte",
			state:   protocol.StateHandshake,
			payload: []byte{45, 0},
		},
		{
			id:      "login-missing-distance",
			state:   protocol.StateLogin,
			payload: admissionLoginPayload(protocolAdmissionPlayerID, "Alice", 2)[:len(admissionLoginPayload(protocolAdmissionPlayerID, "Alice", 2))-1],
		},
		{
			id:      "login-trailing-byte",
			state:   protocol.StateLogin,
			payload: append(admissionLoginPayload(protocolAdmissionPlayerID, "Alice", 2), 0),
		},
		{
			id:      "login-invalid-utf8-name",
			state:   protocol.StateLogin,
			payload: admissionLoginPayload(protocolAdmissionPlayerID, "Al\xffice", 2),
		},
	}

	wireCodec, err := codec.NewCodec()
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	for _, row := range rows {
		t.Run(row.id, func(t *testing.T) {
			packet, err := wireCodec.DecodeClient(row.state, 0, row.payload)
			if err == nil {
				t.Fatalf("case %s: inbound codec accepted a structurally invalid payload as %T", row.id, packet)
			}
		})
	}
}

// TestProtocolAdmissionOraclePairedCaseIDsMatchTheRustSuite pins that the case
// identities this file carries are the ones the Rust admission suite names, so
// the two suites cannot drift into covering different records.
func TestProtocolAdmissionOraclePairedCaseIDsMatchTheRustSuite(t *testing.T) {
	rustIDs := []string{
		"hello-version-44",
		"hello-trailing-byte",
		"login-zero-id-distance-one",
		"login-distance-one",
		"login-distance-65",
		"login-name-trimmed",
		"login-long-raw-name",
		"login-33-scalars",
		"login-control-character",
		"login-missing-distance",
		"login-trailing-byte",
		"login-invalid-utf8-name",
	}
	seen := make(map[string]bool, len(rustIDs))
	for _, row := range protocolAdmissionLoginRows() {
		seen[row.id] = true
	}
	for _, id := range []string{"hello-version-44", "hello-trailing-byte", "login-missing-distance", "login-trailing-byte", "login-invalid-utf8-name"} {
		seen[id] = true
	}
	for _, id := range rustIDs {
		if !seen[id] {
			t.Fatalf("case %s is named by the Rust admission suite but is absent here", id)
		}
	}
	if len(seen) != len(rustIDs) {
		t.Fatalf("this file observes %d case identities, the Rust suite names %d", len(seen), len(rustIDs))
	}
}
