package main

// This file is the inbound negotiation producer group: `ClientHello` and
// `LoginStart` each register a decode and an encode route through the shared
// packet-case runner, so the two families that decide whether a peer may log in
// are executed by the real Go codec rather than restated here.
//
// The decode path is the structural one. The Go inbound decoder deliberately
// preserves a semantically invalid hello and login start so the login driver
// can answer with its frozen rejection codes, which is why the corpus carries
// only the ordinary structural cases here: an old version and an over-long raw
// name are admitted by the Go inbound path and refused by the Go outbound
// encoder, so no case can carry them both ways. Those values are pinned instead
// by the paired codec/driver table in
// `packages/shared/network/protocol_admission_oracle_test.go` and by the Rust
// admission suite, whose case identities this group's cases do not duplicate.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// clientHelloFamily is the handshake hello family this group registers.
	clientHelloFamily = "protocol.client.ClientHello"
	// loginStartFamily is the login start family this group registers.
	loginStartFamily = "protocol.client.LoginStart"
	// negotiationVersion is the protocol version both families are pinned to.
	negotiationVersion = "45"
	// negotiationProducerID is the exporter's producer identity for this
	// group's candidate assets and merged manifest.
	negotiationProducerID = "runtime-oracle/protocol-negotiation"
	// clientHelloCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	clientHelloCorpusRelDir = corpusCasesRelDir + "/protocol/ClientHello"
	// loginStartCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	loginStartCorpusRelDir = corpusCasesRelDir + "/protocol/LoginStart"

	// packetDirectionClient is the packet key direction both families carry.
	packetDirectionClient = "client-to-server"
	// packetStateHandshake names the handshake state in one packet key.
	packetStateHandshake = "handshake"
	// packetStateLogin names the login state in one packet key.
	packetStateLogin = "login"

	// packetOutcomeCategory labels an accepted packet outcome. The frozen
	// vocabulary fixes the error categories; the success label only says the
	// value came from a packet codec rather than from framing or a domain rule.
	packetOutcomeCategory = "packet"
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	clientHelloDecodeCaseID         = clientHelloFamily + "/" + negotiationVersion + "/decode-current-version"
	clientHelloEncodeCaseID         = clientHelloFamily + "/" + negotiationVersion + "/encode-current-version"
	clientHelloTrailingCaseID       = clientHelloFamily + "/" + negotiationVersion + "/decode-trailing-byte"
	clientHelloNoncanonicalCaseID   = clientHelloFamily + "/" + negotiationVersion + "/decode-noncanonical-version"
	loginStartDecodeCaseID          = loginStartFamily + "/" + negotiationVersion + "/decode-valid"
	loginStartEncodeCaseID          = loginStartFamily + "/" + negotiationVersion + "/encode-valid"
	loginStartMissingDistanceCaseID = loginStartFamily + "/" + negotiationVersion + "/decode-missing-distance"
	loginStartTrailingCaseID        = loginStartFamily + "/" + negotiationVersion + "/decode-trailing-byte"
	loginStartInvalidUTF8CaseID     = loginStartFamily + "/" + negotiationVersion + "/decode-invalid-utf8-name"
	loginStartOversizedCaseID       = loginStartFamily + "/" + negotiationVersion + "/decode-oversized-payload"
)

// negotiationPlayerID is the identity the LoginStart cases carry: version
// nibble 4 and variant bits 10, which is what the Go validator admits.
var negotiationPlayerID = core.PlayerID{
	0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15,
}

// negotiationLoginWire is the exact payload the LoginStart encode case has to
// publish: the 16 identity bytes, the canonical length prefix 5, the name
// Alice, and the trailing view-distance byte. It is a review-contract literal
// rather than a value a producer returned, so the case fails when the
// production encoder disagrees with the reviewed bytes.
var negotiationLoginWire = func() []byte {
	wire := append([]byte(nil), negotiationPlayerID[:]...)
	wire = append(wire, 5)
	wire = append(wire, []byte("Alice")...)
	wire = append(wire, 2)
	return wire
}()

// negotiationHelloWire is the exact payload the ClientHello encode case
// publishes: the canonical uvarint of the current protocol version.
var negotiationHelloWire = []byte{45}

// negotiationRejectionCategory resolves the language-neutral rejection
// category for one real Go codec failure.
//
// The Go decoder reports its own unexported sentinels, so the mapping is a
// closed table over the wire conditions those sentinels name rather than over
// a sentinel identity. A failure with no mapping is a hard error, because an
// unclassified rejection must never be recorded as evidence.
func negotiationRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "invalid uvarint"):
		return "invalid-varint", true
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "exceeds 64 KiB"):
		return "capacity", true
	case strings.Contains(message, "invalid string"):
		return "invalid-value", true
	case strings.Contains(message, "invalid float32"):
		return "invalid-value", true
	case strings.Contains(message, "invalid boolean"):
		return "invalid-enum", true
	case strings.Contains(message, "not UUIDv4"), strings.Contains(message, "invalid login display name"):
		return "invalid-identity", true
	case strings.Contains(message, "unsupported client protocol version"):
		return "unsupported-version", true
	case strings.Contains(message, "login view distance"):
		return "invalid-value", true
	case strings.Contains(message, "unknown packet ID"):
		return "invalid-enum", true
	}
	return "", false
}

// negotiationPacketKey resolves one packet key to the Go state and numeric ID
// the codec dispatches on.
//
// A case names its key instead of trusting its family, so a case registered
// under the wrong family or state fails before the codec runs rather than
// quietly executing a different packet.
func negotiationPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	key := c.PacketKey
	switch key.State {
	case packetStateHandshake:
		state := protocol.StateHandshake
		if key.ID != 0 || key.Direction != packetDirectionClient {
			return 0, 0, fmt.Errorf("runtime-oracle: case %s names handshake key %+v", c.ID, key)
		}
		return state, key.ID, nil
	case packetStateLogin:
		if key.ID != 0 || key.Direction != packetDirectionClient {
			return 0, 0, fmt.Errorf("runtime-oracle: case %s names login key %+v", c.ID, key)
		}
		return protocol.StateLogin, key.ID, nil
	default:
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names unknown state %q", c.ID, key.State)
	}
}

// negotiationFields renders the semantic fields one decoded packet publishes.
//
// The fields come from the DTO the production decoder returned, never from the
// case input, so the recorded evidence describes what the codec actually
// parsed. Identities render as lowercase hexadecimal and the distance as the
// declared integer, which is the canonical field encoding the Rust consumer
// publishes for the same case.
func negotiationFields(c CaseSpec, packet protocol.ClientPacket) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.ClientHello:
		return map[string]any{"protocol_version": message.ProtocolVersion}, nil
	case protocol.LoginStart:
		return map[string]any{
			"display_name":  message.DisplayName,
			"player_id":     hex.EncodeToString(message.PlayerID[:]),
			"view_distance": message.ViewDistance,
		}, nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// newNegotiationCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newNegotiationCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// runNegotiationDecode executes one negotiation decode case through the real
// Go client decoder.
//
// The producer hands the payload and the case's own packet key to
// `DecodeClient` and classifies the failure it returns. It never reimplements
// the length-prefix, UTF-8 or trailing-byte rules, so the recorded outcome is
// whatever the production codec decides about these exact bytes.
func runNegotiationDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := negotiationPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newNegotiationCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeClient(state, packetID, input)
	if err != nil {
		category, classified := negotiationRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := negotiationFields(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// clientHelloEncodeRequest is the canonical JSON field input one ClientHello
// encode case carries.
type clientHelloEncodeRequest struct {
	ProtocolVersion uint32 `json:"protocol_version"`
}

// loginStartEncodeRequest is the canonical JSON field input one LoginStart
// encode case carries. The identity is lowercase hexadecimal, which is the same
// rendering the decode outcome publishes.
type loginStartEncodeRequest struct {
	PlayerID     string `json:"player_id"`
	DisplayName  string `json:"display_name"`
	ViewDistance uint8  `json:"view_distance"`
}

// decodeEncodeRequest reads one encode case's typed fields, rejecting unknown
// fields and trailing content so a case cannot smuggle a byte sequence into the
// producer.
func decodeEncodeRequest(c CaseSpec, input []byte, request any) error {
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.DisallowUnknownFields()
	if err := dec.Decode(request); err != nil {
		return fmt.Errorf("runtime-oracle: case %s: decode encode fields: %w", c.ID, err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("runtime-oracle: case %s: encode input carries trailing content", c.ID)
	}
	return nil
}

// runNegotiationEncode executes one negotiation encode case through the real
// Go client encoder and reads the result back through the decoder.
//
// The producer builds the DTO from the typed fields and calls `EncodeClient`,
// which runs the outbound validation first. The read-back guards the other
// direction, because bytes the production decoder rejects must never be
// recorded as evidence.
func runNegotiationEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := negotiationPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newNegotiationCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ClientPacket
	switch c.Family {
	case clientHelloFamily:
		var request clientHelloEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.ClientHello{ProtocolVersion: request.ProtocolVersion}
	case loginStartFamily:
		var request loginStartEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		id, err := playerIDFromHex(c, request.PlayerID)
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.LoginStart{
			PlayerID:     id,
			DisplayName:  request.DisplayName,
			ViewDistance: request.ViewDistance,
		}
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	encodedID, payload, err := wireCodec.EncodeClient(state, packet)
	if err != nil {
		category, classified := negotiationRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified encode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	if encodedID != packetID {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoder published packet ID %d, want %d", c.ID, encodedID, packetID)
	}
	decoded, err := wireCodec.DecodeClient(state, encodedID, payload)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload does not decode back: %w", c.ID, err)
	}
	if !reflect.DeepEqual(decoded, packet) {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload decodes to %#v, want %#v", c.ID, decoded, packet)
	}
	fields, err := negotiationFields(c, decoded)
	if err != nil {
		return Outcome{}, nil, err
	}
	sum := sha256.Sum256(payload)
	return Outcome{
		Kind:                 "ok",
		Category:             packetOutcomeCategory,
		Fields:               fields,
		EncodedPayloadDigest: fmt.Sprintf("sha256:%x", sum),
	}, payload, nil
}

// playerIDFromHex reads one canonical lowercase hexadecimal identity field.
func playerIDFromHex(c CaseSpec, text string) (core.PlayerID, error) {
	var id core.PlayerID
	raw, err := hex.DecodeString(text)
	if err != nil {
		return id, fmt.Errorf("runtime-oracle: case %s: player_id is not hexadecimal: %w", c.ID, err)
	}
	if len(raw) != len(id) {
		return id, fmt.Errorf("runtime-oracle: case %s: player_id is %d bytes, want %d", c.ID, len(raw), len(id))
	}
	copy(id[:], raw)
	return id, nil
}

// negotiationCorpusRoutes is the closed route map the two families execute.
func negotiationCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range []string{clientHelloFamily, loginStartFamily} {
		routes[ConsumerRoute{FamilyID: family, Version: negotiationVersion, Operation: "decode"}] = runNegotiationDecode
		routes[ConsumerRoute{FamilyID: family, Version: negotiationVersion, Operation: "encode"}] = runNegotiationEncode
	}
	return routes
}

// negotiationCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type negotiationCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which
	// the producer's own bytes are compared against.
	Wire []byte
}

// negotiationCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer result.
type negotiationCaseDefinition struct {
	id     string
	family string
	relDir string
	op     string
	key    PacketKeySpec
	// input is the binary decode payload, already mutated where the case is a
	// malformed one.
	input []byte
	// request is the JSON encode payload, nil for a decode case.
	request any
	// wire is the expected encoded payload for an encode case.
	wire []byte
	// expect is the normalized outcome an execution has to reproduce.
	expect Outcome
}

// negotiationCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid cases
// use the canonical payload the Go encoder produces for these fields; the
// malformed decode cases mutate that payload at one boundary each, so each
// rejection names the boundary that owns it.
func negotiationCaseDefinitions() []negotiationCaseDefinition {
	loginStartValid := func() []byte {
		payload := append([]byte(nil), negotiationPlayerID[:]...)
		payload = append(payload, 5)
		payload = append(payload, []byte("Alice")...)
		payload = append(payload, 2)
		return payload
	}()
	loginValidFields := map[string]any{
		"display_name":  "Alice",
		"player_id":     hex.EncodeToString(negotiationPlayerID[:]),
		"view_distance": uint8(2),
	}

	return []negotiationCaseDefinition{
		{
			id:     clientHelloDecodeCaseID,
			family: clientHelloFamily,
			relDir: clientHelloCorpusRelDir,
			op:     "decode",
			key:    PacketKeySpec{Direction: packetDirectionClient, State: packetStateHandshake, ID: 0},
			input:  append([]byte(nil), negotiationHelloWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   map[string]any{"protocol_version": uint32(45)},
			},
		},
		{
			id:     clientHelloEncodeCaseID,
			family: clientHelloFamily,
			relDir: clientHelloCorpusRelDir,
			op:     "encode",
			key:    PacketKeySpec{Direction: packetDirectionClient, State: packetStateHandshake, ID: 0},
			request: clientHelloEncodeRequest{
				ProtocolVersion: 45,
			},
			wire: append([]byte(nil), negotiationHelloWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   map[string]any{"protocol_version": uint32(45)},
			},
		},
		{
			id:     clientHelloTrailingCaseID,
			family: clientHelloFamily,
			relDir: clientHelloCorpusRelDir,
			op:     "decode",
			key:    PacketKeySpec{Direction: packetDirectionClient, State: packetStateHandshake, ID: 0},
			input:  []byte{45, 0},
			expect: Outcome{Kind: "error", Category: "trailing"},
		},
		{
			id:     clientHelloNoncanonicalCaseID,
			family: clientHelloFamily,
			relDir: clientHelloCorpusRelDir,
			op:     "decode",
			key:    PacketKeySpec{Direction: packetDirectionClient, State: packetStateHandshake, ID: 0},
			input:  []byte{0x80, 0x00},
			expect: Outcome{Kind: "error", Category: "invalid-varint"},
		},
		{
			id:     loginStartDecodeCaseID,
			family: loginStartFamily,
			relDir: loginStartCorpusRelDir,
			op:     "decode",
			key:    PacketKeySpec{Direction: packetDirectionClient, State: packetStateLogin, ID: 0},
			input:  append([]byte(nil), loginStartValid...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   loginValidFields,
			},
		},
		{
			id:     loginStartEncodeCaseID,
			family: loginStartFamily,
			relDir: loginStartCorpusRelDir,
			op:     "encode",
			key:    PacketKeySpec{Direction: packetDirectionClient, State: packetStateLogin, ID: 0},
			request: loginStartEncodeRequest{
				PlayerID:     hex.EncodeToString(negotiationPlayerID[:]),
				DisplayName:  "Alice",
				ViewDistance: 2,
			},
			wire: append([]byte(nil), negotiationLoginWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   loginValidFields,
			},
		},
		{
			id:     loginStartMissingDistanceCaseID,
			family: loginStartFamily,
			relDir: loginStartCorpusRelDir,
			op:     "decode",
			key:    PacketKeySpec{Direction: packetDirectionClient, State: packetStateLogin, ID: 0},
			input:  loginStartValid[:len(loginStartValid)-1],
			expect: Outcome{Kind: "error", Category: "truncated"},
		},
		{
			id:     loginStartTrailingCaseID,
			family: loginStartFamily,
			relDir: loginStartCorpusRelDir,
			op:     "decode",
			key:    PacketKeySpec{Direction: packetDirectionClient, State: packetStateLogin, ID: 0},
			input:  append(append([]byte(nil), loginStartValid...), 0),
			expect: Outcome{Kind: "error", Category: "trailing"},
		},
		{
			id:     loginStartInvalidUTF8CaseID,
			family: loginStartFamily,
			relDir: loginStartCorpusRelDir,
			op:     "decode",
			key:    PacketKeySpec{Direction: packetDirectionClient, State: packetStateLogin, ID: 0},
			input: func() []byte {
				payload := append([]byte(nil), negotiationPlayerID[:]...)
				payload = append(payload, 5)
				payload = append(payload, []byte("Al\xffice")...)
				payload = append(payload, 2)
				return payload
			}(),
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     loginStartOversizedCaseID,
			family: loginStartFamily,
			relDir: loginStartCorpusRelDir,
			op:     "decode",
			key:    PacketKeySpec{Direction: packetDirectionClient, State: packetStateLogin, ID: 0},
			input:  make([]byte, codec.MaxSmallPayload+1),
			expect: Outcome{Kind: "error", Category: "capacity"},
		},
	}
}

// negotiationLabel renders one case's asset label from its identity.
func negotiationLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// negotiationCandidate builds one case's manifest entry and asset bytes from
// its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildNegotiationCandidate(t *testing.T, definition negotiationCaseDefinition) negotiationCandidate {
	t.Helper()

	label := negotiationLabel(definition.id)
	inputPath := filepath.ToSlash(filepath.Join(definition.relDir, label+".input"+inputExtension(definition.op)))
	expectedPath := filepath.ToSlash(filepath.Join(definition.relDir, label+".expected.json"))

	var input []byte
	switch {
	case definition.op == "decode":
		input = append([]byte(nil), definition.input...)
	case definition.request != nil:
		rendered, err := json.MarshalIndent(definition.request, "", "  ")
		if err != nil {
			t.Fatalf("case %s: marshal encode input: %v", definition.id, err)
		}
		input = append(rendered, '\n')
	default:
		t.Fatalf("case %s: neither a decode payload nor an encode request", definition.id)
	}

	expect := definition.expect
	if definition.wire != nil {
		sum := sha256.Sum256(definition.wire)
		expect.EncodedPayloadDigest = fmt.Sprintf("sha256:%x", sum)
	}
	expected, err := marshalIndentedOutcome(expect)
	if err != nil {
		t.Fatalf("case %s: render expectation: %v", definition.id, err)
	}
	assets := map[string][]byte{
		inputPath:    input,
		expectedPath: append(expected, '\n'),
	}

	spec := CaseSpec{
		ID:           definition.id,
		Family:       definition.family,
		Version:      negotiationVersion,
		Operation:    definition.op,
		PacketKey:    &definition.key,
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return negotiationCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// inputExtension names one case input asset's extension from its operation.
func inputExtension(operation string) string {
	if operation == "encode" {
		return ".json"
	}
	return ".bin"
}

// inputFormat names the manifest input format one operation carries.
func inputFormat(operation string) string {
	if operation == "encode" {
		return "json"
	}
	return "binary"
}

// negotiationCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func negotiationCandidates(t *testing.T, root string) []negotiationCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := negotiationCaseDefinitions()
	candidates := make([]negotiationCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildNegotiationCandidate(t, definition)
		for relative, want := range candidate.Assets {
			tracked, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
			if readErr != nil {
				if !os.IsNotExist(readErr) {
					t.Fatalf("read tracked asset %s: %v", relative, readErr)
				}
				continue
			}
			if !bytes.Equal(tracked, want) {
				t.Fatalf("tracked asset %s differs from the reviewed candidate", relative)
			}
		}
		if existing, ok := registered[candidate.Spec.ID]; ok {
			if !reflect.DeepEqual(existing, candidate.Spec) {
				t.Fatalf("tracked case %s is %#v, want the reviewed candidate %#v", candidate.Spec.ID, existing, candidate.Spec)
			}
		}
		candidates = append(candidates, candidate)
	}
	return candidates
}

// negotiationRegisteredCase reports whether the base manifest already carries
// one case identity, so a re-merge of an integrated candidate adds nothing.
func negotiationRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// negotiationSelection is this group's registration: its cases, the Go sources
// its rules are read from, and the four routes the two families execute.
func negotiationSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := negotiationCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{
		clientHelloFamily: {
			"packages/shared/network/codec/codec_client.go",
			"packages/shared/network/protocol/packet.go",
		},
		loginStartFamily: {
			"packages/shared/network/codec/codec_client.go",
			"packages/shared/network/protocol/packet.go",
		},
	}
	routes := make([]ConsumerRoute, 0, 4)
	for _, family := range []string{clientHelloFamily, loginStartFamily} {
		routes = append(routes,
			ConsumerRoute{FamilyID: family, Version: negotiationVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family, Version: negotiationVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  negotiationProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// negotiationManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func negotiationManifest(t *testing.T, root string, candidates []negotiationCandidate) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)

	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	cloned := Inventory{
		SchemaVersion:  frozen.SchemaVersion,
		SourceRevision: frozen.SourceRevision,
		Identities:     frozen.Identities,
		Families:       append([]Family(nil), frozen.Families...),
		Cases:          cases,
	}
	for index := range cloned.Families {
		family := cloned.Families[index].ID
		if family != clientHelloFamily && family != loginStartFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		var listed []string
		for _, c := range cloned.Cases {
			if c.Family == family {
				listed = append(listed, c.ID)
			}
		}
		sort.Strings(listed)
		cloned.Families[index].Cases = listed
	}

	encoded, err := encodeInventory(cloned)
	if err != nil {
		t.Fatalf("encode negotiation working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write negotiation working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load negotiation working manifest: %v", err)
	}
	return loaded
}

// negotiationScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve under
// the root the runner is given.
func negotiationScratchRoot(t *testing.T, root string, candidates []negotiationCandidate) string {
	t.Helper()
	staged := t.TempDir()
	for _, candidate := range candidates {
		for relative, data := range candidate.Assets {
			target := filepath.Join(staged, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create staged parent for %s: %v", relative, err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				t.Fatalf("stage candidate asset %s: %v", relative, err)
			}
		}
	}
	return staged
}

// negotiationCandidateByID resolves one candidate by its case identity.
func negotiationCandidateByID(t *testing.T, candidates []negotiationCandidate, id string) negotiationCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return negotiationCandidate{}
}

// negotiationObservation resolves one executed observation by its case identity.
func negotiationObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// negotiationExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var negotiationExportPublished bool

// negotiationCandidatesExport publishes the reviewed candidates and the
// complete merged manifest candidate through the existing external exporter and
// returns the published producer directory. An unset export variable publishes
// nothing and returns "", so an ordinary test run never writes outside its own
// temporary storage.
func negotiationCandidatesExport(t *testing.T, root string, candidates []negotiationCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if negotiationExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-negotiation")
	}
	negotiationExportPublished = true

	manifest, err := encodeInventory(merged)
	if err != nil {
		t.Fatalf("encode manifest candidate: %v", err)
	}
	assets := []generatedAsset{{RelativePath: "contracts.json", Data: append(manifest, '\n')}}
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		for relative := range candidate.Assets {
			if seen[relative] {
				continue
			}
			seen[relative] = true
			assets = append(assets, generatedAsset{RelativePath: relative, Data: candidate.Assets[relative]})
		}
	}
	sort.Slice(assets[1:], func(i, j int) bool { return assets[i+1].RelativePath < assets[j+1].RelativePath })
	published, err := exportGeneratedAssets(root, exportRoot, negotiationProducerID, assets)
	if err != nil {
		t.Fatalf("export negotiation candidates: %v", err)
	}
	return published
}

// TestProtocolNegotiationOracleDecodesCurrentHelloAndLoginStart pins that the
// real Go inbound decoder publishes the canonical fields for both families'
// valid cases.
func TestProtocolNegotiationOracleDecodesCurrentHelloAndLoginStart(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := negotiationCandidates(t, root)

	for _, id := range []string{clientHelloDecodeCaseID, loginStartDecodeCaseID} {
		candidate := negotiationCandidateByID(t, candidates, id)
		outcome, encoded, err := runNegotiationDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runNegotiationDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolNegotiationOracleEncodesCanonicalWire pins both encode producers
// against the reviewed wire literals and against the recorded digest.
func TestProtocolNegotiationOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := negotiationCandidates(t, root)

	for _, id := range []string{clientHelloEncodeCaseID, loginStartEncodeCaseID} {
		candidate := negotiationCandidateByID(t, candidates, id)
		outcome, encoded, err := runNegotiationEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runNegotiationEncode(%s): %v", id, err)
		}
		if !bytes.Equal(encoded, candidate.Wire) {
			t.Fatalf("case %s encoded %x, want %x", id, encoded, candidate.Wire)
		}
		sum := sha256.Sum256(candidate.Wire)
		if outcome.EncodedPayloadDigest != fmt.Sprintf("sha256:%x", sum) {
			t.Fatalf("case %s digest = %s, want sha256:%x", id, outcome.EncodedPayloadDigest, sum)
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolNegotiationOracleRejectsMalformedDecodesAtTheirBoundary pins that
// every malformed decode case is refused by the production decoder and
// classified at the boundary that owns it.
func TestProtocolNegotiationOracleRejectsMalformedDecodesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := negotiationCandidates(t, root)

	for _, id := range []string{
		clientHelloTrailingCaseID,
		clientHelloNoncanonicalCaseID,
		loginStartMissingDistanceCaseID,
		loginStartTrailingCaseID,
		loginStartInvalidUTF8CaseID,
		loginStartOversizedCaseID,
	} {
		candidate := negotiationCandidateByID(t, candidates, id)
		outcome, encoded, err := runNegotiationDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runNegotiationDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolNegotiationOracleOutboundEncoderRefusesAnOverLongRawName pins the
// asymmetry this node's evidence rests on: the Go outbound encoder refuses a raw
// display name beyond 128 bytes, so an over-long raw name cannot become a
// two-way corpus case and stays in the paired admission table.
func TestProtocolNegotiationOracleOutboundEncoderRefusesAnOverLongRawName(t *testing.T) {
	wireCodec, err := newNegotiationCodec()
	if err != nil {
		t.Fatalf("create codec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	overLong := strings.Repeat(" ", 128) + "Alice"
	if len(overLong) <= 128 {
		t.Fatalf("over-long name is %d bytes, want more than 128", len(overLong))
	}
	_, _, err = wireCodec.EncodeClient(protocol.StateLogin, protocol.LoginStart{
		PlayerID:     negotiationPlayerID,
		DisplayName:  overLong,
		ViewDistance: 2,
	})
	if err == nil {
		t.Fatal("the outbound encoder accepted a raw name beyond 128 bytes")
	}
	category, classified := negotiationRejectionCategory(err)
	if !classified || category != "invalid-value" {
		t.Fatalf("rejection category = %q (classified %v), want invalid-value", category, classified)
	}
}

// TestProtocolNegotiationOracleExpectedFieldMutationFailsComparison pins that
// the recorded expectation is a commitment: replacing the expected display name
// fails comparison against what the producer decoded, and replacing the encoded
// digest fails comparison against the reviewed wire.
func TestProtocolNegotiationOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := negotiationCandidates(t, root)

	decode := negotiationCandidateByID(t, candidates, loginStartDecodeCaseID)
	produced, _, err := runNegotiationDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runNegotiationDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	fields["display_name"] = "Alicia"
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated display name compares equal to the produced outcome")
	}

	encode := negotiationCandidateByID(t, candidates, loginStartEncodeCaseID)
	producedEncode, _, err := runNegotiationEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runNegotiationEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolNegotiationOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolNegotiationOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := negotiationCandidates(t, root)
	manifest := negotiationManifest(t, root, candidates)
	staged := negotiationScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, negotiationCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := negotiationObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolNegotiationOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so a
// case cannot claim coverage from its name alone.
func TestProtocolNegotiationOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := negotiationCandidates(t, root)
	manifest := negotiationManifest(t, root, candidates)
	staged := negotiationScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range []string{clientHelloFamily, loginStartFamily} {
		decodeOnly[ConsumerRoute{FamilyID: family, Version: negotiationVersion, Operation: "decode"}] = runNegotiationDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	// The runner walks the case list in order, so the rejection names the
	// first case whose encode route is unregistered.
	named := false
	for _, family := range []string{clientHelloFamily, loginStartFamily} {
		if strings.Contains(err.Error(), family+"/"+negotiationVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name either family's encode route", err)
	}
}

// TestProtocolNegotiationOracleManifestMergeRegistersPacketRoutes pins that the
// merged manifest registers both families' routes and case lists, leaves the
// source revision alone, and records this group's provenance sources.
func TestProtocolNegotiationOracleManifestMergeRegistersPacketRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, negotiationSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range []string{clientHelloFamily, loginStartFamily} {
		var row *Family
		for index := range merged.Families {
			if merged.Families[index].ID == family {
				row = &merged.Families[index]
				break
			}
		}
		if row == nil {
			t.Fatalf("merged manifest has no %s family", family)
		}
		var listed []string
		for _, c := range merged.Cases {
			if c.Family == family {
				listed = append(listed, c.ID)
			}
		}
		sort.Strings(listed)
		if !reflect.DeepEqual(row.Cases, listed) {
			t.Fatalf("%s family lists %v, want %v", family, row.Cases, listed)
		}
		if len(listed) == 0 {
			t.Fatalf("%s family registers no case", family)
		}
		sources := make(map[string]bool, len(row.Sources))
		for _, source := range row.Sources {
			hash, hashErr := hashFile(filepath.Join(root, filepath.FromSlash(source.Path)))
			if hashErr != nil {
				t.Fatalf("hash provenance source %s: %v", source.Path, hashErr)
			}
			if hash != source.SHA256 {
				t.Fatalf("provenance source %s records %s, disk has %s", source.Path, source.SHA256, hash)
			}
			sources[source.Path] = true
		}
		for _, required := range []string{
			"packages/shared/network/codec/codec_client.go",
			"packages/shared/network/protocol/packet.go",
		} {
			if !sources[required] {
				t.Fatalf("%s provenance drops %s", family, required)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so
	// a re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range negotiationCandidates(t, root) {
		if !negotiationRegisteredCase(base, candidate.Spec.ID) {
			wantCases++
		}
	}
	if len(merged.Cases) != wantCases {
		t.Fatalf("merged carries %d cases, want %d", len(merged.Cases), wantCases)
	}
}

// TestProtocolNegotiationOracleCandidatesExportForReview publishes the
// reviewed candidates and the manifest candidate. An unset export variable
// publishes nothing, so the tracked corpus is never written by this package.
func TestProtocolNegotiationOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := negotiationCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), negotiationSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := negotiationCandidatesExport(t, root, candidates, merged)
	if strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv)) == "" {
		if published != "" {
			t.Fatalf("published %s with the export variable unset", published)
		}
		return
	}
	if published == "" {
		t.Fatal("no candidate was published")
	}
	for _, candidate := range candidates {
		for relative := range candidate.Assets {
			path := filepath.Join(published, filepath.FromSlash(relative))
			if _, statErr := os.Lstat(path); statErr != nil {
				t.Fatalf("candidate asset %s is missing: %v", relative, statErr)
			}
		}
	}
	written, err := os.ReadFile(filepath.Join(published, "contracts.json"))
	if err != nil {
		t.Fatalf("read manifest candidate: %v", err)
	}
	encoded, err := encodeInventory(merged)
	if err != nil {
		t.Fatalf("encode merged manifest: %v", err)
	}
	if !bytes.Equal(written, append(encoded, '\n')) {
		t.Fatal("manifest candidate is not the encoded merged manifest")
	}
	reloaded, err := LoadInventory(filepath.Join(published, "contracts.json"))
	if err != nil {
		t.Fatalf("reload manifest candidate: %v", err)
	}
	if err := verifyProtocolManifest(root, loadRealManifest(t, root), reloaded); err != nil {
		t.Fatalf("manifest candidate does not reconcile: %v", err)
	}
}
