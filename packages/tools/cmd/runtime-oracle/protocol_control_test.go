package main

// This file is the control packet producer group: the seven non-gameplay
// control families (`ServerHello`, `HandshakeReject`, `LoginSuccess`,
// `LoginReject`, `KeepAlive`, `KeepAliveReply` and `Disconnect`) each register
// a decode and an encode route through the shared packet-case runner, so every
// case is executed by the real Go codec rather than restated here.
//
// The decode cases are the Go decoder's own bytes: a valid payload, a mutated
// payload at one boundary each, or a proper truncation of a reviewed payload.
// The encode cases carry canonical JSON fields and the reviewed wire the Go
// encoder has to publish, or a DTO the Go outbound validator has to refuse. No
// case in this group declares a session, a deadline or a send: login
// lifecycle and keepalive scheduling are server and client core policy and
// stay outside the protocol contract.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// serverHelloFamily is the handshake server hello family this group registers.
	serverHelloFamily = "protocol.server.ServerHello"
	// handshakeRejectFamily is the handshake reject family this group registers.
	handshakeRejectFamily = "protocol.server.HandshakeReject"
	// loginSuccessFamily is the login success family this group registers.
	loginSuccessFamily = "protocol.server.LoginSuccess"
	// loginRejectFamily is the login reject family this group registers.
	loginRejectFamily = "protocol.server.LoginReject"
	// keepAliveFamily is the play keep alive family this group registers.
	keepAliveFamily = "protocol.server.KeepAlive"
	// keepAliveReplyFamily is the play keep alive reply family this group registers.
	keepAliveReplyFamily = "protocol.client.KeepAliveReply"
	// disconnectFamily is the play disconnect family this group registers.
	disconnectFamily = "protocol.server.Disconnect"
	// controlVersion is the protocol version every control family is pinned to.
	controlVersion = "45"
	// controlProducerID is the exporter's producer identity for this group's
	// candidate assets and merged manifest.
	controlProducerID = "runtime-oracle/protocol-control"

	// packetDirectionServer names the server-to-client direction.
	packetDirectionServer = "server-to-client"
	// packetStatePlay names the play state in one packet key.
	packetStatePlay = "play"

	// controlMessageMaxBytes is the Go codec's string bound for the control
	// message families, which the maximum-message cases sit exactly on.
	controlMessageMaxBytes = 256
)

// Corpus-relative directories holding each family's committed case assets.
const (
	serverHelloCorpusRelDir     = corpusCasesRelDir + "/protocol/ServerHello"
	handshakeRejectCorpusRelDir = corpusCasesRelDir + "/protocol/HandshakeReject"
	loginSuccessCorpusRelDir    = corpusCasesRelDir + "/protocol/LoginSuccess"
	loginRejectCorpusRelDir     = corpusCasesRelDir + "/protocol/LoginReject"
	keepAliveCorpusRelDir       = corpusCasesRelDir + "/protocol/KeepAlive"
	keepAliveReplyCorpusRelDir  = corpusCasesRelDir + "/protocol/KeepAliveReply"
	disconnectCorpusRelDir      = corpusCasesRelDir + "/protocol/Disconnect"
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	serverHelloDecodeCaseID         = serverHelloFamily + "/" + controlVersion + "/decode-current-version"
	serverHelloEncodeCaseID         = serverHelloFamily + "/" + controlVersion + "/encode-current-version"
	serverHelloPreviousCaseID       = serverHelloFamily + "/" + controlVersion + "/decode-previous-version"
	handshakeRejectDecodeCaseID     = handshakeRejectFamily + "/" + controlVersion + "/decode-valid"
	handshakeRejectEncodeCaseID     = handshakeRejectFamily + "/" + controlVersion + "/encode-valid"
	handshakeRejectUnknownDecodeID  = handshakeRejectFamily + "/" + controlVersion + "/decode-unknown-code"
	handshakeRejectUnknownEncodeID  = handshakeRejectFamily + "/" + controlVersion + "/encode-unknown-code"
	handshakeRejectTruncatedCaseID  = handshakeRejectFamily + "/" + controlVersion + "/decode-message-length-exceeds-payload"
	loginSuccessDecodeCaseID        = loginSuccessFamily + "/" + controlVersion + "/decode-zero-seed"
	loginSuccessEncodeCaseID        = loginSuccessFamily + "/" + controlVersion + "/encode-zero-seed"
	loginSuccessInvalidIDCaseID     = loginSuccessFamily + "/" + controlVersion + "/decode-invalid-uuid"
	loginSuccessTrailingCaseID      = loginSuccessFamily + "/" + controlVersion + "/decode-trailing-byte"
	loginRejectDecodeCaseID         = loginRejectFamily + "/" + controlVersion + "/decode-code-one-empty-message"
	loginRejectEncodeCaseID         = loginRejectFamily + "/" + controlVersion + "/encode-code-seven-maximum-message"
	loginRejectCodeZeroCaseID       = loginRejectFamily + "/" + controlVersion + "/decode-code-zero"
	loginRejectCodeEightCaseID      = loginRejectFamily + "/" + controlVersion + "/decode-code-eight"
	loginRejectDeclaredLengthCaseID = loginRejectFamily + "/" + controlVersion + "/decode-declared-length-above-bound"
	loginRejectTruncatedCaseID      = loginRejectFamily + "/" + controlVersion + "/decode-message-length-exceeds-payload"
	loginRejectEncodeOversizeID     = loginRejectFamily + "/" + controlVersion + "/encode-message-above-bound"
	keepAliveDecodeCaseID           = keepAliveFamily + "/" + controlVersion + "/decode-valid"
	keepAliveEncodeCaseID           = keepAliveFamily + "/" + controlVersion + "/encode-valid"
	keepAliveZeroTokenCaseID        = keepAliveFamily + "/" + controlVersion + "/decode-zero-token"
	keepAliveReplyDecodeCaseID      = keepAliveReplyFamily + "/" + controlVersion + "/decode-valid"
	keepAliveReplyEncodeCaseID      = keepAliveReplyFamily + "/" + controlVersion + "/encode-valid"
	keepAliveReplyZeroDecodeID      = keepAliveReplyFamily + "/" + controlVersion + "/decode-zero-token"
	keepAliveReplyZeroEncodeID      = keepAliveReplyFamily + "/" + controlVersion + "/encode-zero-token"
	disconnectDecodeCaseID          = disconnectFamily + "/" + controlVersion + "/decode-code-one-empty-message"
	disconnectEncodeCaseID          = disconnectFamily + "/" + controlVersion + "/encode-code-five-maximum-message"
	disconnectCodeZeroCaseID        = disconnectFamily + "/" + controlVersion + "/decode-code-zero"
	disconnectCodeSixCaseID         = disconnectFamily + "/" + controlVersion + "/decode-code-six"
	disconnectDeclaredLengthCaseID  = disconnectFamily + "/" + controlVersion + "/decode-declared-length-above-bound"
	disconnectTruncatedCaseID       = disconnectFamily + "/" + controlVersion + "/decode-message-length-exceeds-payload"
	disconnectTrailingCaseID        = disconnectFamily + "/" + controlVersion + "/decode-trailing-byte"
	disconnectEncodeOversizeID      = disconnectFamily + "/" + controlVersion + "/encode-message-above-bound"
)

// controlPlayerID is the identity the LoginSuccess cases carry: version
// nibble 4 and variant bits 10, which is what the Go validator admits.
var controlPlayerID = core.PlayerID{
	0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15,
}

// controlUvarint renders the canonical uvarint of one value, matching the Go
// primitive's shortest form, so a review wire literal is the encoder's own
// encoding of the value it names.
func controlUvarint(value uint32) []byte {
	var encoded []byte
	for value >= 1<<7 {
		encoded = append(encoded, byte(value)|0x80)
		value >>= 7
	}
	return append(encoded, byte(value))
}

// controlFamilies lists the seven families in registry order, with the corpus
// directory that holds each family's assets. The order is the review order,
// not a dispatch table.
func controlFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{serverHelloFamily, serverHelloCorpusRelDir},
		{handshakeRejectFamily, handshakeRejectCorpusRelDir},
		{loginSuccessFamily, loginSuccessCorpusRelDir},
		{loginRejectFamily, loginRejectCorpusRelDir},
		{keepAliveFamily, keepAliveCorpusRelDir},
		{keepAliveReplyFamily, keepAliveReplyCorpusRelDir},
		{disconnectFamily, disconnectCorpusRelDir},
	}
}

// controlFamilyKeys pins each family's complete packet key. A case names its
// key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs.
var controlFamilyKeys = map[string]PacketKeySpec{
	serverHelloFamily:     {Direction: packetDirectionServer, State: packetStateHandshake, ID: 0},
	handshakeRejectFamily: {Direction: packetDirectionServer, State: packetStateHandshake, ID: 1},
	loginSuccessFamily:    {Direction: packetDirectionServer, State: packetStateLogin, ID: 0},
	loginRejectFamily:     {Direction: packetDirectionServer, State: packetStateLogin, ID: 1},
	keepAliveFamily:       {Direction: packetDirectionServer, State: packetStatePlay, ID: 5},
	keepAliveReplyFamily:  {Direction: packetDirectionClient, State: packetStatePlay, ID: 4},
	disconnectFamily:      {Direction: packetDirectionServer, State: packetStatePlay, ID: 6},
}

// controlRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder names,
// never over a sentinel identity, and a failure with no mapping is a hard
// error. The one boundary the Go sentinels cannot express is a declared string
// length the payload cannot complete: `byteDecoder.string` answers it with the
// same error as a malformed UTF-8 message, while the frozen boundary taxonomy
// resolves an incomplete payload as the truncated category. The case's own
// derivation base decides between the two, because a rejected proper prefix of
// the reviewed payload is exactly the incomplete condition. The Rust consumer
// maps the same condition to its own truncated decode error, so both
// implementations publish one category.
func controlRejectionCategory(err error, derivedFrom, input []byte) (string, bool) {
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
	case strings.Contains(message, "unsupported server protocol version"):
		return "unsupported-version", true
	case strings.Contains(message, "invalid handshake reject code"),
		strings.Contains(message, "invalid login reject code"),
		strings.Contains(message, "invalid disconnect code"),
		strings.Contains(message, "unknown packet ID"):
		return "invalid-enum", true
	case strings.Contains(message, "not UUIDv4"):
		return "invalid-identity", true
	case strings.Contains(message, "keep alive token is zero"),
		strings.Contains(message, "keep alive reply token is zero"):
		return "invalid-value", true
	case strings.Contains(message, "invalid string"):
		if isRejectedPrefix(derivedFrom, input) {
			return "truncated", true
		}
		return "invalid-value", true
	}
	return "", false
}

// isRejectedPrefix reports whether input is a proper prefix of the reviewed
// payload the case was derived from, which is the incomplete-payload shape.
//
// A proper prefix of a valid fixed record is missing bytes, so the record
// cannot be complete; a payload that merely reuses the family's bytes with a
// mutated value is not a prefix and keeps its own category.
func isRejectedPrefix(derivedFrom, input []byte) bool {
	return len(derivedFrom) > 0 && len(input) < len(derivedFrom) && bytes.HasPrefix(derivedFrom, input)
}

// controlDerivedFrom lists the reviewed payload each truncated control case
// was derived from, so the boundary resolver can tell an incomplete payload
// from a mutated one.
func controlDerivedFrom(caseID string) []byte {
	switch caseID {
	case handshakeRejectTruncatedCaseID:
		return append([]byte(nil), controlHandshakeRejectMessageWire...)
	case loginRejectTruncatedCaseID:
		return append([]byte(nil), controlLoginRejectMessageWire...)
	case disconnectTruncatedCaseID:
		return append([]byte(nil), controlDisconnectMessageWire...)
	default:
		return nil
	}
}

// controlPacketKey resolves one packet key to the Go state and numeric ID the
// codec dispatches on, and reports whether the packet travels client-to-server.
func controlPacketKey(c CaseSpec) (protocol.State, uint32, bool, error) {
	if c.PacketKey == nil {
		return 0, 0, false, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := controlFamilyKeys[c.Family]
	if !owned {
		return 0, 0, false, fmt.Errorf("runtime-oracle: case %s names family %q, which no control producer owns", c.ID, c.Family)
	}
	if *c.PacketKey != registered {
		return 0, 0, false, fmt.Errorf("runtime-oracle: case %s names key %+v, want %+v", c.ID, *c.PacketKey, registered)
	}
	switch registered.State {
	case packetStateHandshake:
		return protocol.StateHandshake, registered.ID, false, nil
	case packetStateLogin:
		return protocol.StateLogin, registered.ID, false, nil
	case packetStatePlay:
		return protocol.StatePlay, registered.ID, registered.Direction == packetDirectionClient, nil
	default:
		return 0, 0, false, fmt.Errorf("runtime-oracle: case %s names unknown state %q", c.ID, registered.State)
	}
}

// controlFields renders the semantic fields one decoded control packet
// publishes.
//
// The fields come from the DTO the production decoder returned, never from the
// case input. Identities render as lowercase hexadecimal, u64 values as
// decimal strings so the full range stays lossless, and the small enums as
// their declared integers, which is the canonical field encoding the Rust
// consumer publishes for the same case.
func controlFields(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.ServerHello:
		return map[string]any{"protocol_version": message.ProtocolVersion}, nil
	case protocol.HandshakeReject:
		return map[string]any{
			"server_protocol_version": message.ServerProtocolVersion,
			"code":                    uint8(message.Code),
			"message":                 message.Message,
		}, nil
	case protocol.LoginSuccess:
		return map[string]any{
			"player_id":  hex.EncodeToString(message.PlayerID[:]),
			"world_seed": strconv.FormatUint(message.WorldSeed, 10),
		}, nil
	case protocol.LoginReject:
		return map[string]any{
			"code":    uint8(message.Code),
			"message": message.Message,
		}, nil
	case protocol.KeepAlive:
		return map[string]any{"token": strconv.FormatUint(message.Token, 10)}, nil
	case protocol.KeepAliveReply:
		return map[string]any{"token": strconv.FormatUint(message.Token, 10)}, nil
	case protocol.Disconnect:
		return map[string]any{
			"code":    uint8(message.Code),
			"message": message.Message,
		}, nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// newControlCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newControlCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// runControlDecode executes one control decode case through the real Go
// decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` or `DecodeClient`
// and classifies the failure it returns. It never reimplements the
// length-prefix, UTF-8, enum or trailing-byte rules, so the recorded outcome
// is whatever the production codec decides about these exact bytes.
func runControlDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, client, err := controlPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newControlCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet any
	if client {
		packet, err = wireCodec.DecodeClient(state, packetID, input)
	} else {
		packet, err = wireCodec.DecodeServer(state, packetID, input)
	}
	if err != nil {
		category, classified := controlRejectionCategory(err, controlDerivedFrom(c.ID), input)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := controlFields(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// serverHelloEncodeRequest is the canonical JSON field input one ServerHello
// encode case carries.
type serverHelloEncodeRequest struct {
	ProtocolVersion uint32 `json:"protocol_version"`
}

// handshakeRejectEncodeRequest is the canonical JSON field input one
// HandshakeReject encode case carries.
type handshakeRejectEncodeRequest struct {
	ServerProtocolVersion uint32 `json:"server_protocol_version"`
	Code                  uint8  `json:"code"`
	Message               string `json:"message"`
}

// loginSuccessEncodeRequest is the canonical JSON field input one LoginSuccess
// encode case carries. The identity is lowercase hexadecimal and the seed is a
// decimal JSON integer, which the full u64 range survives.
type loginSuccessEncodeRequest struct {
	PlayerID  string `json:"player_id"`
	WorldSeed uint64 `json:"world_seed"`
}

// loginRejectEncodeRequest is the canonical JSON field input one LoginReject
// encode case carries.
type loginRejectEncodeRequest struct {
	Code    uint8  `json:"code"`
	Message string `json:"message"`
}

// keepAliveEncodeRequest is the canonical JSON field input one KeepAlive or
// KeepAliveReply encode case carries.
type keepAliveEncodeRequest struct {
	Token uint64 `json:"token"`
}

// disconnectEncodeRequest is the canonical JSON field input one Disconnect
// encode case carries.
type disconnectEncodeRequest struct {
	Code    uint8  `json:"code"`
	Message string `json:"message"`
}

// runControlEncode executes one control encode case through the real Go
// encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects
// must never be recorded as evidence.
func runControlEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, client, err := controlPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newControlCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet any
	switch c.Family {
	case serverHelloFamily:
		var request serverHelloEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.ServerHello{ProtocolVersion: request.ProtocolVersion}
	case handshakeRejectFamily:
		var request handshakeRejectEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.HandshakeReject{
			ServerProtocolVersion: request.ServerProtocolVersion,
			Code:                  protocol.HandshakeRejectCode(request.Code),
			Message:               request.Message,
		}
	case loginSuccessFamily:
		var request loginSuccessEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		id, err := playerIDFromHex(c, request.PlayerID)
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.LoginSuccess{PlayerID: id, WorldSeed: request.WorldSeed}
	case loginRejectFamily:
		var request loginRejectEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.LoginReject{
			Code:    protocol.LoginRejectCode(request.Code),
			Message: request.Message,
		}
	case keepAliveFamily:
		var request keepAliveEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.KeepAlive{Token: request.Token}
	case keepAliveReplyFamily:
		var request keepAliveEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.KeepAliveReply{Token: request.Token}
	case disconnectFamily:
		var request disconnectEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.Disconnect{
			Code:    protocol.DisconnectCode(request.Code),
			Message: request.Message,
		}
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	var encodedID uint32
	var payload []byte
	if client {
		clientPacket, ok := packet.(protocol.ClientPacket)
		if !ok {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s built %T, which is not a client packet", c.ID, packet)
		}
		encodedID, payload, err = wireCodec.EncodeClient(state, clientPacket)
	} else {
		serverPacket, ok := packet.(protocol.ServerPacket)
		if !ok {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s built %T, which is not a server packet", c.ID, packet)
		}
		encodedID, payload, err = wireCodec.EncodeServer(state, serverPacket)
	}
	if err != nil {
		category, classified := controlRejectionCategory(err, nil, nil)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified encode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	if encodedID != packetID {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoder published packet ID %d, want %d", c.ID, encodedID, packetID)
	}
	var decoded any
	if client {
		decoded, err = wireCodec.DecodeClient(state, encodedID, payload)
	} else {
		decoded, err = wireCodec.DecodeServer(state, encodedID, payload)
	}
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload does not decode back: %w", c.ID, err)
	}
	if !reflect.DeepEqual(decoded, packet) {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload decodes to %#v, want %#v", c.ID, decoded, packet)
	}
	fields, err := controlFields(c, decoded)
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

// controlCorpusRoutes is the closed route map the seven families execute.
func controlCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range controlFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: controlVersion, Operation: "decode"}] = runControlDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: controlVersion, Operation: "encode"}] = runControlEncode
	}
	return routes
}

// The reviewed wire literals the case table pins. Each is the Go encoder's own
// output for the fields it names, so a producer that encodes anything else
// fails the comparison instead of the candidate agreeing with itself.
var (
	controlServerHelloWire     = []byte{45}
	controlHandshakeRejectWire = []byte{42, 1, 0}
	controlLoginSuccessWire    = func() []byte {
		wire := append([]byte(nil), controlPlayerID[:]...)
		return append(wire, 0, 0, 0, 0, 0, 0, 0, 0)
	}()
	controlLoginRejectWire = []byte{1, 0}
	controlKeepAliveWire   = []byte{1, 0, 0, 0, 0, 0, 0, 0}
	controlDisconnectWire  = []byte{1, 0}
	// controlLoginRejectMessageWire, controlDisconnectMessageWire and
	// controlHandshakeRejectMessageWire carry a two-byte message, and the
	// truncated cases cut one byte off each.
	controlLoginRejectMessageWire     = []byte{1, 0x02, 'n', 'o'}
	controlDisconnectMessageWire      = []byte{6, 0x02, 'b', 'y'}
	controlHandshakeRejectMessageWire = []byte{42, 1, 0x02, 'n', 'o'}
	controlLoginRejectMaximumWire     = append(append([]byte{7}, controlUvarint(controlMessageMaxBytes)...), bytes.Repeat([]byte{'m'}, controlMessageMaxBytes)...)
	controlDisconnectMaximumWire      = append(append([]byte{5}, controlUvarint(controlMessageMaxBytes)...), bytes.Repeat([]byte{'b'}, controlMessageMaxBytes)...)
	controlLoginRejectDeclaredWire    = append(append([]byte{1}, controlUvarint(controlMessageMaxBytes+1)...), bytes.Repeat([]byte{'a'}, controlMessageMaxBytes+1)...)
	controlDisconnectDeclaredWire     = append(append([]byte{6}, controlUvarint(controlMessageMaxBytes+1)...), bytes.Repeat([]byte{'a'}, controlMessageMaxBytes+1)...)
)

// controlCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer
// result.
type controlCaseDefinition struct {
	id     string
	family string
	relDir string
	op     string
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

// controlCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid
// cases use the canonical payload the Go encoder produces for these fields;
// the malformed decode cases mutate that payload at one boundary each, and the
// invalid encode cases build a DTO the production validator refuses, so each
// rejection names the boundary that owns it.
func controlCaseDefinitions() []controlCaseDefinition {
	loginValidFields := map[string]any{
		"player_id":  hex.EncodeToString(controlPlayerID[:]),
		"world_seed": "0",
	}
	return []controlCaseDefinition{
		{
			id:     serverHelloDecodeCaseID,
			family: serverHelloFamily,
			relDir: serverHelloCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), controlServerHelloWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   map[string]any{"protocol_version": uint32(45)},
			},
		},
		{
			id:     serverHelloEncodeCaseID,
			family: serverHelloFamily,
			relDir: serverHelloCorpusRelDir,
			op:     "encode",
			request: serverHelloEncodeRequest{
				ProtocolVersion: 45,
			},
			wire: append([]byte(nil), controlServerHelloWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   map[string]any{"protocol_version": uint32(45)},
			},
		},
		{
			id:     serverHelloPreviousCaseID,
			family: serverHelloFamily,
			relDir: serverHelloCorpusRelDir,
			op:     "decode",
			input:  []byte{44},
			expect: Outcome{Kind: "error", Category: "unsupported-version"},
		},
		{
			id:     handshakeRejectDecodeCaseID,
			family: handshakeRejectFamily,
			relDir: handshakeRejectCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), controlHandshakeRejectWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields: map[string]any{
					"server_protocol_version": uint32(42),
					"code":                    uint8(1),
					"message":                 "",
				},
			},
		},
		{
			id:     handshakeRejectEncodeCaseID,
			family: handshakeRejectFamily,
			relDir: handshakeRejectCorpusRelDir,
			op:     "encode",
			request: handshakeRejectEncodeRequest{
				ServerProtocolVersion: 42,
				Code:                  1,
				Message:               "",
			},
			wire: append([]byte(nil), controlHandshakeRejectWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields: map[string]any{
					"server_protocol_version": uint32(42),
					"code":                    uint8(1),
					"message":                 "",
				},
			},
		},
		{
			id:     handshakeRejectUnknownDecodeID,
			family: handshakeRejectFamily,
			relDir: handshakeRejectCorpusRelDir,
			op:     "decode",
			input:  []byte{42, 2, 0},
			expect: Outcome{Kind: "error", Category: "invalid-enum"},
		},
		{
			id:     handshakeRejectUnknownEncodeID,
			family: handshakeRejectFamily,
			relDir: handshakeRejectCorpusRelDir,
			op:     "encode",
			request: handshakeRejectEncodeRequest{
				ServerProtocolVersion: 42,
				Code:                  2,
				Message:               "",
			},
			expect: Outcome{Kind: "error", Category: "invalid-enum"},
		},
		{
			id:     handshakeRejectTruncatedCaseID,
			family: handshakeRejectFamily,
			relDir: handshakeRejectCorpusRelDir,
			op:     "decode",
			input:  []byte{42, 1, 0x02, 'n'},
			expect: Outcome{Kind: "error", Category: "truncated"},
		},
		{
			id:     loginSuccessDecodeCaseID,
			family: loginSuccessFamily,
			relDir: loginSuccessCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), controlLoginSuccessWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   loginValidFields,
			},
		},
		{
			id:     loginSuccessEncodeCaseID,
			family: loginSuccessFamily,
			relDir: loginSuccessCorpusRelDir,
			op:     "encode",
			request: loginSuccessEncodeRequest{
				PlayerID:  hex.EncodeToString(controlPlayerID[:]),
				WorldSeed: 0,
			},
			wire: append([]byte(nil), controlLoginSuccessWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   loginValidFields,
			},
		},
		{
			id:     loginSuccessInvalidIDCaseID,
			family: loginSuccessFamily,
			relDir: loginSuccessCorpusRelDir,
			op:     "decode",
			input:  make([]byte, 24),
			expect: Outcome{Kind: "error", Category: "invalid-identity"},
		},
		{
			id:     loginSuccessTrailingCaseID,
			family: loginSuccessFamily,
			relDir: loginSuccessCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), controlLoginSuccessWire...), 0x00),
			expect: Outcome{Kind: "error", Category: "trailing"},
		},
		{
			id:     loginRejectDecodeCaseID,
			family: loginRejectFamily,
			relDir: loginRejectCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), controlLoginRejectWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields: map[string]any{
					"code":    uint8(1),
					"message": "",
				},
			},
		},
		{
			id:     loginRejectEncodeCaseID,
			family: loginRejectFamily,
			relDir: loginRejectCorpusRelDir,
			op:     "encode",
			request: loginRejectEncodeRequest{
				Code:    7,
				Message: strings.Repeat("m", controlMessageMaxBytes),
			},
			wire: append([]byte(nil), controlLoginRejectMaximumWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields: map[string]any{
					"code":    uint8(7),
					"message": strings.Repeat("m", controlMessageMaxBytes),
				},
			},
		},
		{
			id:     loginRejectCodeZeroCaseID,
			family: loginRejectFamily,
			relDir: loginRejectCorpusRelDir,
			op:     "decode",
			input:  []byte{0, 0},
			expect: Outcome{Kind: "error", Category: "invalid-enum"},
		},
		{
			id:     loginRejectCodeEightCaseID,
			family: loginRejectFamily,
			relDir: loginRejectCorpusRelDir,
			op:     "decode",
			input:  []byte{8, 0},
			expect: Outcome{Kind: "error", Category: "invalid-enum"},
		},
		{
			id:     loginRejectDeclaredLengthCaseID,
			family: loginRejectFamily,
			relDir: loginRejectCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), controlLoginRejectDeclaredWire...),
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     loginRejectTruncatedCaseID,
			family: loginRejectFamily,
			relDir: loginRejectCorpusRelDir,
			op:     "decode",
			input:  []byte{1, 0x02, 'n'},
			expect: Outcome{Kind: "error", Category: "truncated"},
		},
		{
			id:     loginRejectEncodeOversizeID,
			family: loginRejectFamily,
			relDir: loginRejectCorpusRelDir,
			op:     "encode",
			request: loginRejectEncodeRequest{
				Code:    1,
				Message: strings.Repeat("a", controlMessageMaxBytes+1),
			},
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     keepAliveDecodeCaseID,
			family: keepAliveFamily,
			relDir: keepAliveCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), controlKeepAliveWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   map[string]any{"token": "1"},
			},
		},
		{
			id:     keepAliveEncodeCaseID,
			family: keepAliveFamily,
			relDir: keepAliveCorpusRelDir,
			op:     "encode",
			request: keepAliveEncodeRequest{
				Token: 1,
			},
			wire: append([]byte(nil), controlKeepAliveWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   map[string]any{"token": "1"},
			},
		},
		{
			id:     keepAliveZeroTokenCaseID,
			family: keepAliveFamily,
			relDir: keepAliveCorpusRelDir,
			op:     "decode",
			input:  make([]byte, 8),
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     keepAliveReplyDecodeCaseID,
			family: keepAliveReplyFamily,
			relDir: keepAliveReplyCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), controlKeepAliveWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   map[string]any{"token": "1"},
			},
		},
		{
			id:     keepAliveReplyEncodeCaseID,
			family: keepAliveReplyFamily,
			relDir: keepAliveReplyCorpusRelDir,
			op:     "encode",
			request: keepAliveEncodeRequest{
				Token: 1,
			},
			wire: append([]byte(nil), controlKeepAliveWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   map[string]any{"token": "1"},
			},
		},
		{
			id:     keepAliveReplyZeroDecodeID,
			family: keepAliveReplyFamily,
			relDir: keepAliveReplyCorpusRelDir,
			op:     "decode",
			input:  make([]byte, 8),
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     keepAliveReplyZeroEncodeID,
			family: keepAliveReplyFamily,
			relDir: keepAliveReplyCorpusRelDir,
			op:     "encode",
			request: keepAliveEncodeRequest{
				Token: 0,
			},
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     disconnectDecodeCaseID,
			family: disconnectFamily,
			relDir: disconnectCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), controlDisconnectWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields: map[string]any{
					"code":    uint8(1),
					"message": "",
				},
			},
		},
		{
			id:     disconnectEncodeCaseID,
			family: disconnectFamily,
			relDir: disconnectCorpusRelDir,
			op:     "encode",
			request: disconnectEncodeRequest{
				Code:    5,
				Message: strings.Repeat("b", controlMessageMaxBytes),
			},
			wire: append([]byte(nil), controlDisconnectMaximumWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields: map[string]any{
					"code":    uint8(5),
					"message": strings.Repeat("b", controlMessageMaxBytes),
				},
			},
		},
		{
			id:     disconnectCodeZeroCaseID,
			family: disconnectFamily,
			relDir: disconnectCorpusRelDir,
			op:     "decode",
			input:  []byte{0, 0},
			expect: Outcome{Kind: "error", Category: "invalid-enum"},
		},
		{
			id:     disconnectCodeSixCaseID,
			family: disconnectFamily,
			relDir: disconnectCorpusRelDir,
			op:     "decode",
			input:  []byte{6, 0},
			expect: Outcome{Kind: "error", Category: "invalid-enum"},
		},
		{
			id:     disconnectDeclaredLengthCaseID,
			family: disconnectFamily,
			relDir: disconnectCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), controlDisconnectDeclaredWire...),
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     disconnectTruncatedCaseID,
			family: disconnectFamily,
			relDir: disconnectCorpusRelDir,
			op:     "decode",
			input:  []byte{6, 0x02, 'b'},
			expect: Outcome{Kind: "error", Category: "truncated"},
		},
		{
			id:     disconnectTrailingCaseID,
			family: disconnectFamily,
			relDir: disconnectCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), controlDisconnectWire...), 0x00),
			expect: Outcome{Kind: "error", Category: "trailing"},
		},
		{
			id:     disconnectEncodeOversizeID,
			family: disconnectFamily,
			relDir: disconnectCorpusRelDir,
			op:     "encode",
			request: disconnectEncodeRequest{
				Code:    1,
				Message: strings.Repeat("a", controlMessageMaxBytes+1),
			},
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
	}
}

// controlLabel renders one case's asset label from its identity.
func controlLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// controlCandidate builds one case's manifest entry and asset bytes from its
// definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildControlCandidate(t *testing.T, definition controlCaseDefinition) controlCandidate {
	t.Helper()

	label := controlLabel(definition.id)
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
		Version:      controlVersion,
		Operation:    definition.op,
		PacketKey:    controlKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return controlCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// controlKeyPointer resolves one family's reviewed packet key for a case spec.
func controlKeyPointer(family string) *PacketKeySpec {
	key, owned := controlFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// controlCandidate is one reviewed case: its manifest specification, the exact
// asset bytes it publishes, and the expectation an independent execution has to
// reproduce.
type controlCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which the
	// producer's own bytes are compared against.
	Wire []byte
}

// controlCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func controlCandidates(t *testing.T, root string) []controlCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := controlCaseDefinitions()
	candidates := make([]controlCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildControlCandidate(t, definition)
		if candidate.Spec.PacketKey == nil {
			t.Fatalf("case %s names family %q, which has no reviewed packet key", candidate.Spec.ID, candidate.Spec.Family)
		}
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

// controlRegisteredCase reports whether the base manifest already carries one
// case identity, so a re-merge of an integrated candidate adds nothing.
func controlRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// controlSelection is this group's registration: its cases, the Go sources its
// rules are read from, and the routes the seven families execute.
func controlSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := controlCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{
		serverHelloFamily:     controlServerSources(),
		handshakeRejectFamily: controlServerSources(),
		loginSuccessFamily:    controlServerSources(),
		loginRejectFamily:     controlServerSources(),
		keepAliveFamily:       controlServerSources(),
		keepAliveReplyFamily:  controlClientSources(),
		disconnectFamily:      controlServerSources(),
	}
	routes := make([]ConsumerRoute, 0, len(controlFamilies())*2)
	for _, family := range controlFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: controlVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: controlVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  controlProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// controlServerSources lists the Go sources the server-to-client control
// families read their rules from.
func controlServerSources() []string {
	return []string{
		"packages/shared/network/codec/codec_server.go",
		"packages/shared/network/protocol/packet.go",
	}
}

// controlClientSources lists the Go sources the client-to-server control
// family reads its rules from.
func controlClientSources() []string {
	return []string{
		"packages/shared/network/codec/codec_client.go",
		"packages/shared/network/protocol/packet.go",
	}
}

// controlManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func controlManifest(t *testing.T, root string, candidates []controlCandidate) Inventory {
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
		if !controlOwnsFamily(family) {
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
		t.Fatalf("encode control working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write control working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load control working manifest: %v", err)
	}
	return loaded
}

// controlOwnsFamily reports whether this group registers cases for one family.
func controlOwnsFamily(family string) bool {
	_, owned := controlFamilyKeys[family]
	return owned
}

// controlScratchRoot stages this group's candidate assets in a harness-owned
// temporary directory, because a corpus case has to resolve under the root the
// runner is given.
func controlScratchRoot(t *testing.T, root string, candidates []controlCandidate) string {
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

// controlCandidateByID resolves one candidate by its case identity.
func controlCandidateByID(t *testing.T, candidates []controlCandidate, id string) controlCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return controlCandidate{}
}

// controlObservation resolves one executed observation by its case identity.
func controlObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// controlExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var controlExportPublished bool

// controlCandidatesExport publishes the reviewed candidates and the complete
// merged manifest candidate through the existing external exporter and returns
// the published producer directory. An unset export variable publishes nothing
// and returns "", so an ordinary test run never writes outside its own
// temporary storage.
func controlCandidatesExport(t *testing.T, root string, candidates []controlCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if controlExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-control")
	}
	controlExportPublished = true

	manifest, err := encodeInventory(merged)
	if err != nil {
		t.Fatalf("encode manifest candidate: %v", err)
	}
	assets := []generatedAsset{{RelativePath: "contracts.json", Data: append(manifest, '\n')}}
	relatives := make([]string, 0)
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		for relative := range candidate.Assets {
			if seen[relative] {
				continue
			}
			seen[relative] = true
			relatives = append(relatives, relative)
		}
	}
	sort.Strings(relatives)
	for _, relative := range relatives {
		for _, candidate := range candidates {
			if data, ok := candidate.Assets[relative]; ok {
				assets = append(assets, generatedAsset{RelativePath: relative, Data: data})
				break
			}
		}
	}
	published, err := exportGeneratedAssets(root, exportRoot, controlProducerID, assets)
	if err != nil {
		t.Fatalf("export control candidates: %v", err)
	}
	return published
}

// TestProtocolControlOracleDecodesEveryValidCase pins that the real Go decoder
// publishes the canonical fields for every family's valid decode case.
func TestProtocolControlOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := controlCandidates(t, root)

	for _, id := range []string{
		serverHelloDecodeCaseID,
		handshakeRejectDecodeCaseID,
		loginSuccessDecodeCaseID,
		loginRejectDecodeCaseID,
		keepAliveDecodeCaseID,
		keepAliveReplyDecodeCaseID,
		disconnectDecodeCaseID,
	} {
		candidate := controlCandidateByID(t, candidates, id)
		outcome, encoded, err := runControlDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runControlDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolControlOracleEncodesCanonicalWire pins each encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolControlOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := controlCandidates(t, root)

	for _, id := range []string{
		serverHelloEncodeCaseID,
		handshakeRejectEncodeCaseID,
		loginSuccessEncodeCaseID,
		loginRejectEncodeCaseID,
		keepAliveEncodeCaseID,
		keepAliveReplyEncodeCaseID,
		disconnectEncodeCaseID,
	} {
		candidate := controlCandidateByID(t, candidates, id)
		outcome, encoded, err := runControlEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runControlEncode(%s): %v", id, err)
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

// TestProtocolControlOracleRejectsMalformedCasesAtTheirBoundary pins that every
// malformed decode case and invalid encode case is refused by the production
// codec and classified at the boundary that owns it.
func TestProtocolControlOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := controlCandidates(t, root)

	for _, id := range []string{
		serverHelloPreviousCaseID,
		handshakeRejectUnknownDecodeID,
		handshakeRejectTruncatedCaseID,
		loginSuccessInvalidIDCaseID,
		loginSuccessTrailingCaseID,
		loginRejectCodeZeroCaseID,
		loginRejectCodeEightCaseID,
		loginRejectDeclaredLengthCaseID,
		loginRejectTruncatedCaseID,
		keepAliveZeroTokenCaseID,
		keepAliveReplyZeroDecodeID,
		disconnectCodeZeroCaseID,
		disconnectCodeSixCaseID,
		disconnectDeclaredLengthCaseID,
		disconnectTruncatedCaseID,
		disconnectTrailingCaseID,
	} {
		candidate := controlCandidateByID(t, candidates, id)
		outcome, encoded, err := runControlDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runControlDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range []string{
		handshakeRejectUnknownEncodeID,
		loginRejectEncodeOversizeID,
		keepAliveReplyZeroEncodeID,
		disconnectEncodeOversizeID,
	} {
		candidate := controlCandidateByID(t, candidates, id)
		outcome, encoded, err := runControlEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runControlEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolControlOracleIncompleteMessagePrefixIsTruncated pins the
// controller ruling this group's string boundary rests on: a declared message
// length the payload cannot complete is an incomplete payload, so the producer
// classifies it as the truncated category even though the Go string primitive
// answers it with the same sentinel as a malformed UTF-8 message, and the Rust
// control message reader reports the same category through its own truncated
// decode error.
func TestProtocolControlOracleIncompleteMessagePrefixIsTruncated(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := controlCandidates(t, root)

	for _, id := range []string{
		handshakeRejectTruncatedCaseID,
		loginRejectTruncatedCaseID,
		disconnectTruncatedCaseID,
	} {
		candidate := controlCandidateByID(t, candidates, id)
		outcome, _, err := runControlDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runControlDecode(%s): %v", id, err)
		}
		if outcome.Kind != "error" || outcome.Category != "truncated" {
			t.Fatalf("case %s produced %#v, want a truncated rejection", id, outcome)
		}
		// A payload that is not a proper prefix of the reviewed record keeps
		// the text-violation category, so the boundary resolver cannot pass by
		// classifying every invalid string as an incomplete payload.
		derived := controlDerivedFrom(id)
		if !isRejectedPrefix(derived, candidate.Assets[candidate.Spec.Input.Path]) {
			t.Fatalf("case %s is not a rejected prefix of its reviewed payload", id)
		}
		mutated := append([]byte(nil), derived...)
		mutated[len(mutated)-1] = 0xff
		spec := candidate.Spec
		spec.ID = id + "-mutated"
		outcome, _, err = runControlDecode(spec, mutated)
		if err != nil {
			t.Fatalf("runControlDecode(%s): %v", spec.ID, err)
		}
		if outcome.Kind != "error" || outcome.Category != "invalid-value" {
			t.Fatalf("mutated case %s produced %#v, want an invalid-value rejection", spec.ID, outcome)
		}
	}
}

// TestProtocolControlOracleExpectedFieldMutationFailsComparison pins that the
// recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded, and replacing the encoded
// digest fails comparison against the reviewed wire.
func TestProtocolControlOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := controlCandidates(t, root)

	decode := controlCandidateByID(t, candidates, loginSuccessDecodeCaseID)
	produced, _, err := runControlDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runControlDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	fields["world_seed"] = "1"
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated world seed compares equal to the produced outcome")
	}

	encode := controlCandidateByID(t, candidates, keepAliveEncodeCaseID)
	producedEncode, _, err := runControlEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runControlEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolControlOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolControlOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := controlCandidates(t, root)
	manifest := controlManifest(t, root, candidates)
	staged := controlScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, controlCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := controlObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolControlOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so a
// case cannot claim coverage from its name alone.
func TestProtocolControlOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := controlCandidates(t, root)
	manifest := controlManifest(t, root, candidates)
	staged := controlScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range controlFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: controlVersion, Operation: "decode"}] = runControlDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range controlFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+controlVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolControlOracleManifestMergeRegistersControlRoutes pins that the
// merged manifest registers all seven families' routes and case lists, leaves
// the source revision alone, and records this group's provenance sources.
func TestProtocolControlOracleManifestMergeRegistersControlRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, controlSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range controlFamilies() {
		var row *Family
		for index := range merged.Families {
			if merged.Families[index].ID == family.id {
				row = &merged.Families[index]
				break
			}
		}
		if row == nil {
			t.Fatalf("merged manifest has no %s family", family.id)
		}
		var listed []string
		for _, c := range merged.Cases {
			if c.Family == family.id {
				listed = append(listed, c.ID)
			}
		}
		sort.Strings(listed)
		if !reflect.DeepEqual(row.Cases, listed) {
			t.Fatalf("%s family lists %v, want %v", family.id, row.Cases, listed)
		}
		if len(listed) == 0 {
			t.Fatalf("%s family registers no case", family.id)
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
		required := controlServerSources()
		if family.id == keepAliveReplyFamily {
			required = controlClientSources()
		}
		for _, want := range required {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so a
	// re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range controlCandidates(t, root) {
		if !controlRegisteredCase(base, candidate.Spec.ID) {
			wantCases++
		}
	}
	if len(merged.Cases) != wantCases {
		t.Fatalf("merged carries %d cases, want %d", len(merged.Cases), wantCases)
	}
	for index := 1; index < len(merged.Cases); index++ {
		if merged.Cases[index].ID < merged.Cases[index-1].ID {
			t.Fatalf("merged case list is not sorted by id at %d", index)
		}
	}
}

// TestProtocolControlOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolControlOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := controlCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), controlSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := controlCandidatesExport(t, root, candidates, merged)
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
