package main

// This file is the client control packet producer group: the four Play
// client-to-server control families (`PlayerInput`, `PlaceBlock`,
// `RequestChunkResync` and `SelectHotbar`) each register a decode and an encode
// route through the shared packet-case runner, so every case is executed by the
// real Go codec rather than restated here.
//
// The decode cases are the Go decoder's own bytes: a valid payload, a mutated
// payload at one boundary each, or a proper truncation of a reviewed payload.
// The encode cases carry canonical JSON fields and the reviewed wire the Go
// encoder has to publish, or a DTO the Go outbound validator has to refuse. The
// input values themselves move no world state: the target ray-cast, the
// movement-axis [-1,1] rule, the resync policy and the slot contents stay
// authority-owned, so no case in this group declares a session, a deadline or a
// send.

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
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
	// playerInputFamily is the play player input family this group registers.
	playerInputFamily = "protocol.client.PlayerInput"
	// placeBlockFamily is the play place block family this group registers.
	placeBlockFamily = "protocol.client.PlaceBlock"
	// requestChunkResyncFamily is the play chunk resync family this group registers.
	requestChunkResyncFamily = "protocol.client.RequestChunkResync"
	// selectHotbarFamily is the play hotbar selection family this group registers.
	selectHotbarFamily = "protocol.client.SelectHotbar"
	// clientControlVersion is the protocol version every family is pinned to.
	clientControlVersion = "45"
	// clientControlProducerID is the exporter's producer identity for this
	// group's candidate assets and merged manifest.
	clientControlProducerID = "runtime-oracle/protocol-client-control"

	// playerInputCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	playerInputCorpusRelDir = corpusCasesRelDir + "/protocol/PlayerInput"
	// placeBlockCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	placeBlockCorpusRelDir = corpusCasesRelDir + "/protocol/PlaceBlock"
	// requestChunkResyncCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	requestChunkResyncCorpusRelDir = corpusCasesRelDir + "/protocol/RequestChunkResync"
	// selectHotbarCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	selectHotbarCorpusRelDir = corpusCasesRelDir + "/protocol/SelectHotbar"
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	playerInputDecodeCaseID       = playerInputFamily + "/" + clientControlVersion + "/decode-valid"
	playerInputEncodeCaseID       = playerInputFamily + "/" + clientControlVersion + "/encode-valid"
	playerInputNaNDecodeID        = playerInputFamily + "/" + clientControlVersion + "/decode-nan-yaw"
	playerInputBoolTwoDecodeID    = playerInputFamily + "/" + clientControlVersion + "/decode-bool-two"
	playerInputNaNEncodeID        = playerInputFamily + "/" + clientControlVersion + "/encode-nan-yaw"
	playerInputTrailingCaseID     = playerInputFamily + "/" + clientControlVersion + "/decode-trailing-byte"
	placeBlockDecodeCaseID        = placeBlockFamily + "/" + clientControlVersion + "/decode-valid"
	placeBlockEncodeCaseID        = placeBlockFamily + "/" + clientControlVersion + "/encode-valid"
	placeBlockSlotNineDecodeID    = placeBlockFamily + "/" + clientControlVersion + "/decode-slot-nine"
	placeBlockInfinitePitchDecode = placeBlockFamily + "/" + clientControlVersion + "/decode-infinite-pitch"
	placeBlockSlotNineEncodeID    = placeBlockFamily + "/" + clientControlVersion + "/encode-slot-nine"
	resyncDecodeCaseID            = requestChunkResyncFamily + "/" + clientControlVersion + "/decode-valid"
	resyncEncodeCaseID            = requestChunkResyncFamily + "/" + clientControlVersion + "/encode-valid"
	resyncDimensionTwoDecodeID    = requestChunkResyncFamily + "/" + clientControlVersion + "/decode-dimension-two"
	resyncDimensionTwoEncodeID    = requestChunkResyncFamily + "/" + clientControlVersion + "/encode-dimension-two"
	selectHotbarDecodeCaseID      = selectHotbarFamily + "/" + clientControlVersion + "/decode-valid"
	selectHotbarEncodeCaseID      = selectHotbarFamily + "/" + clientControlVersion + "/encode-valid"
	selectHotbarSlotNineDecodeID  = selectHotbarFamily + "/" + clientControlVersion + "/decode-slot-nine"
	selectHotbarSlotNineEncodeID  = selectHotbarFamily + "/" + clientControlVersion + "/encode-slot-nine"
)

// clientControlFamilyKeys pins each family's complete packet key. A case names
// its key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs.
var clientControlFamilyKeys = map[string]PacketKeySpec{
	playerInputFamily:        {Direction: packetDirectionClient, State: packetStatePlay, ID: 0},
	placeBlockFamily:         {Direction: packetDirectionClient, State: packetStatePlay, ID: 2},
	requestChunkResyncFamily: {Direction: packetDirectionClient, State: packetStatePlay, ID: 3},
	selectHotbarFamily:       {Direction: packetDirectionClient, State: packetStatePlay, ID: 5},
}

// clientControlFamilies lists the four families in registry order, with the
// corpus directory that holds each family's assets. The order is the review
// order, not a dispatch table.
func clientControlFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{playerInputFamily, playerInputCorpusRelDir},
		{placeBlockFamily, placeBlockCorpusRelDir},
		{requestChunkResyncFamily, requestChunkResyncCorpusRelDir},
		{selectHotbarFamily, selectHotbarCorpusRelDir},
	}
}

// The reviewed wire literals the case table pins. Each is the Go encoder's own
// output for the fields it names, so a producer that encodes anything else
// fails the comparison instead of the candidate agreeing with itself.
var (
	// clientControlPlayerInputWire carries the extreme axes and the -0.0 /
	// 2.0 look bit patterns: the bit pattern is pinned, not the numeric value,
	// so a negative-zero normalization on either side fails the comparison.
	clientControlPlayerInputWire = []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x80, 0x7f, 0x01,
		0x00, 0x00, 0x00, 0x80,
		0x00, 0x00, 0x00, 0x40,
		0x01, 0x00, 0x01, 0x00,
	}
	clientControlPlaceBlockWire = []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xa0, 0x3f,
		0x00, 0x00, 0x00, 0x80,
		0x08,
	}
	clientControlResyncWire = []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x00, 0x00,
		0xff, 0xff, 0xff, 0xff,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	clientControlSelectHotbarWire = []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x08,
	}
)

// float32BitsHex renders one f32 as its eight-digit lowercase-headecimal bit
// string, which is the canonical encoding both the normalized fields and the
// encode requests carry: -0.0 and +0.0 are the same number but different bits,
// and JSON cannot carry the difference.
func float32BitsHex(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// float32FromBitsHex decodes one eight-digit hexadecimal bit string into an
// f32, so an encode request round-trips the exact bits through JSON.
func float32FromBitsHex(c CaseSpec, text string) (float32, error) {
	bits, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: case %s: decode %s bit string: %w", c.ID, text, err)
	}
	return math.Float32frombits(uint32(bits)), nil
}

// clientControlRejectionCategory resolves the language-neutral rejection
// category for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and the
// outbound validators name, never over a sentinel identity, and a failure with
// no mapping is a hard error. The primitive answers precede the validator
// messages, because the decoder rejects a non-finite float or a non-0/1 boolean
// tag while reading and only then hands the record to `Validate`.
func clientControlRejectionCategory(err error) (string, bool) {
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
	case strings.Contains(message, "invalid boolean"):
		return "invalid-enum", true
	case strings.Contains(message, "invalid float32"),
		strings.Contains(message, "non-finite rotation"),
		strings.Contains(message, "slot is outside 0..8"):
		return "invalid-value", true
	case strings.Contains(message, "chunk resync dimension is not overworld or depths"):
		return "invalid-enum", true
	}
	return "", false
}

// clientControlPacketKey resolves one packet key to the Go state and numeric ID
// the codec dispatches on, and reports whether the packet travels
// client-to-server.
func clientControlPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := clientControlFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no client control producer owns", c.ID, c.Family)
	}
	if *c.PacketKey != registered {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names key %+v, want %+v", c.ID, *c.PacketKey, registered)
	}
	if registered.State != packetStatePlay {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names state %q, want play", c.ID, registered.State)
	}
	if registered.Direction != packetDirectionClient {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names direction %q, want client-to-server", c.ID, registered.Direction)
	}
	return protocol.StatePlay, registered.ID, nil
}

// clientControlFields renders the semantic fields one decoded client control
// packet publishes.
//
// The fields come from the DTO the production decoder returned, never from the
// case input. u64 values render as decimal strings so the full range stays
// lossless, and the two look angles render as their eight-digit hexadecimal bit
// strings so a negative zero survives the round trip: the canonical field
// encoding is the Rust consumer's too.
func clientControlFields(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.PlayerInput:
		return map[string]any{
			"sequence":  strconv.FormatUint(message.Sequence, 10),
			"move_x":    message.MoveX,
			"move_z":    message.MoveZ,
			"jump":      message.Jump,
			"yaw":       float32BitsHex(message.Yaw),
			"pitch":     float32BitsHex(message.Pitch),
			"mining":    message.Mining,
			"eating":    message.Eating,
			"sprinting": message.Sprinting,
			"sneaking":  message.Sneaking,
		}, nil
	case protocol.PlaceBlock:
		return map[string]any{
			"sequence": strconv.FormatUint(message.Sequence, 10),
			"yaw":      float32BitsHex(message.Yaw),
			"pitch":    float32BitsHex(message.Pitch),
			"slot":     message.Slot,
		}, nil
	case protocol.RequestChunkResync:
		return map[string]any{
			"sequence":      strconv.FormatUint(message.Sequence, 10),
			"dimension":     uint8(message.Dimension),
			"chunk_x":       message.Chunk.X,
			"chunk_z":       message.Chunk.Z,
			"have_revision": strconv.FormatUint(message.HaveRevision, 10),
		}, nil
	case protocol.SelectHotbar:
		return map[string]any{
			"sequence": strconv.FormatUint(message.Sequence, 10),
			"slot":     message.Slot,
		}, nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// newClientControlCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newClientControlCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// runClientControlDecode executes one client control decode case through the
// real Go decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeClient` and classifies
// the failure it returns. It never reimplements the length-prefix, UTF-8, enum
// or trailing-byte rules, so the recorded outcome is whatever the production
// codec decides about these exact bytes.
func runClientControlDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := clientControlPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newClientControlCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeClient(state, packetID, input)
	if err != nil {
		category, classified := clientControlRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := clientControlFields(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// playerInputEncodeRequest is the canonical JSON field input one PlayerInput
// encode case carries. The look angles are bit strings, and the sequence is a
// decimal JSON integer, which the full u64 range survives.
type playerInputEncodeRequest struct {
	Sequence  uint64 `json:"sequence"`
	MoveX     int8   `json:"move_x"`
	MoveZ     int8   `json:"move_z"`
	Jump      bool   `json:"jump"`
	Yaw       string `json:"yaw"`
	Pitch     string `json:"pitch"`
	Mining    bool   `json:"mining"`
	Eating    bool   `json:"eating"`
	Sprinting bool   `json:"sprinting"`
	Sneaking  bool   `json:"sneaking"`
}

// placeBlockEncodeRequest is the canonical JSON field input one PlaceBlock
// encode case carries.
type placeBlockEncodeRequest struct {
	Sequence uint64 `json:"sequence"`
	Yaw      string `json:"yaw"`
	Pitch    string `json:"pitch"`
	Slot     uint8  `json:"slot"`
}

// requestChunkResyncEncodeRequest is the canonical JSON field input one
// RequestChunkResync encode case carries.
type requestChunkResyncEncodeRequest struct {
	Sequence     uint64 `json:"sequence"`
	Dimension    uint8  `json:"dimension"`
	ChunkX       int32  `json:"chunk_x"`
	ChunkZ       int32  `json:"chunk_z"`
	HaveRevision uint64 `json:"have_revision"`
}

// selectHotbarEncodeRequest is the canonical JSON field input one SelectHotbar
// encode case carries.
type selectHotbarEncodeRequest struct {
	Sequence uint64 `json:"sequence"`
	Slot     uint8  `json:"slot"`
}

// runClientControlEncode executes one client control encode case through the
// real Go encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects
// must never be recorded as evidence.
func runClientControlEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := clientControlPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newClientControlCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ClientPacket
	switch c.Family {
	case playerInputFamily:
		var request playerInputEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		yaw, err := float32FromBitsHex(c, request.Yaw)
		if err != nil {
			return Outcome{}, nil, err
		}
		pitch, err := float32FromBitsHex(c, request.Pitch)
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.PlayerInput{
			Sequence:  request.Sequence,
			MoveX:     request.MoveX,
			MoveZ:     request.MoveZ,
			Jump:      request.Jump,
			Yaw:       yaw,
			Pitch:     pitch,
			Mining:    request.Mining,
			Eating:    request.Eating,
			Sprinting: request.Sprinting,
			Sneaking:  request.Sneaking,
		}
	case placeBlockFamily:
		var request placeBlockEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		yaw, err := float32FromBitsHex(c, request.Yaw)
		if err != nil {
			return Outcome{}, nil, err
		}
		pitch, err := float32FromBitsHex(c, request.Pitch)
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.PlaceBlock{
			Sequence: request.Sequence,
			Yaw:      yaw,
			Pitch:    pitch,
			Slot:     request.Slot,
		}
	case requestChunkResyncFamily:
		var request requestChunkResyncEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.RequestChunkResync{
			Sequence:     request.Sequence,
			Dimension:    core.DimensionID(request.Dimension),
			Chunk:        core.ChunkPos{X: request.ChunkX, Z: request.ChunkZ},
			HaveRevision: request.HaveRevision,
		}
	case selectHotbarFamily:
		var request selectHotbarEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.SelectHotbar{Sequence: request.Sequence, Slot: request.Slot}
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	encodedID, payload, err := wireCodec.EncodeClient(state, packet)
	if err != nil {
		category, classified := clientControlRejectionCategory(err)
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
	fields, err := clientControlFields(c, decoded)
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

// clientControlCorpusRoutes is the closed route map the four families execute.
func clientControlCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range clientControlFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: clientControlVersion, Operation: "decode"}] = runClientControlDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: clientControlVersion, Operation: "encode"}] = runClientControlEncode
	}
	return routes
}

// clientControlCaseDefinition declares one case from literals before any
// producer runs, so the expectation is the review contract rather than a
// producer result.
type clientControlCaseDefinition struct {
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

// clientControlValidFields renders the normalized fields the four valid decode
// cases publish, beside the wire each family's encode case has to produce.
type clientControlValidFields struct {
	fields map[string]any
	wire   []byte
}

// clientControlCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid cases
// use the canonical payload the Go encoder produces for these fields; the
// malformed decode cases mutate that payload at one boundary each, and the
// invalid encode cases build a DTO the production validator refuses, so each
// rejection names the boundary that owns it.
func clientControlCaseDefinitions() []clientControlCaseDefinition {
	playerInputFields := map[string]any{
		"sequence":  "0",
		"move_x":    int8(-128),
		"move_z":    int8(127),
		"jump":      true,
		"yaw":       "80000000",
		"pitch":     "40000000",
		"mining":    true,
		"eating":    false,
		"sprinting": true,
		"sneaking":  false,
	}
	playerInputRequest := playerInputEncodeRequest{
		Sequence:  0,
		MoveX:     -128,
		MoveZ:     127,
		Jump:      true,
		Yaw:       "80000000",
		Pitch:     "40000000",
		Mining:    true,
		Eating:    false,
		Sprinting: true,
		Sneaking:  false,
	}
	placeBlockFields := map[string]any{
		"sequence": "0",
		"yaw":      "3fa00000",
		"pitch":    "80000000",
		"slot":     uint8(8),
	}
	placeBlockRequest := placeBlockEncodeRequest{
		Sequence: 0,
		Yaw:      "3fa00000",
		Pitch:    "80000000",
		Slot:     8,
	}
	resyncFields := map[string]any{
		"sequence":      "0",
		"dimension":     uint8(1),
		"chunk_x":       int32(-1),
		"chunk_z":       int32(0),
		"have_revision": "0",
	}
	resyncRequest := requestChunkResyncEncodeRequest{
		Sequence:     0,
		Dimension:    1,
		ChunkX:       -1,
		ChunkZ:       0,
		HaveRevision: 0,
	}
	selectHotbarFields := map[string]any{
		"sequence": "0",
		"slot":     uint8(8),
	}
	selectHotbarRequest := selectHotbarEncodeRequest{Sequence: 0, Slot: 8}

	mutated := func(source []byte, offset int, value byte) []byte {
		wire := append([]byte(nil), source...)
		wire[offset] = value
		return wire
	}
	mutatedWord := func(source []byte, offset int, value uint32) []byte {
		wire := append([]byte(nil), source...)
		copy(wire[offset:offset+4], []byte{
			byte(value),
			byte(value >> 8),
			byte(value >> 16),
			byte(value >> 24),
		})
		return wire
	}

	return []clientControlCaseDefinition{
		{
			id:     playerInputDecodeCaseID,
			family: playerInputFamily,
			relDir: playerInputCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientControlPlayerInputWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   playerInputFields,
			},
		},
		{
			id:      playerInputEncodeCaseID,
			family:  playerInputFamily,
			relDir:  playerInputCorpusRelDir,
			op:      "encode",
			request: playerInputRequest,
			wire:    append([]byte(nil), clientControlPlayerInputWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   playerInputFields,
			},
		},
		{
			id:     playerInputNaNDecodeID,
			family: playerInputFamily,
			relDir: playerInputCorpusRelDir,
			op:     "decode",
			input:  mutatedWord(clientControlPlayerInputWire, 11, 0x7fc0_0000),
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     playerInputBoolTwoDecodeID,
			family: playerInputFamily,
			relDir: playerInputCorpusRelDir,
			op:     "decode",
			input:  mutated(clientControlPlayerInputWire, 10, 0x02),
			expect: Outcome{Kind: "error", Category: "invalid-enum"},
		},
		{
			id:     playerInputNaNEncodeID,
			family: playerInputFamily,
			relDir: playerInputCorpusRelDir,
			op:     "encode",
			request: playerInputEncodeRequest{
				Sequence:  0,
				MoveX:     -128,
				MoveZ:     127,
				Jump:      true,
				Yaw:       "7fc00000",
				Pitch:     "40000000",
				Mining:    true,
				Eating:    false,
				Sprinting: true,
				Sneaking:  false,
			},
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     playerInputTrailingCaseID,
			family: playerInputFamily,
			relDir: playerInputCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), clientControlPlayerInputWire...), 0x00),
			expect: Outcome{Kind: "error", Category: "trailing"},
		},
		{
			id:     placeBlockDecodeCaseID,
			family: placeBlockFamily,
			relDir: placeBlockCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientControlPlaceBlockWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   placeBlockFields,
			},
		},
		{
			id:      placeBlockEncodeCaseID,
			family:  placeBlockFamily,
			relDir:  placeBlockCorpusRelDir,
			op:      "encode",
			request: placeBlockRequest,
			wire:    append([]byte(nil), clientControlPlaceBlockWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   placeBlockFields,
			},
		},
		{
			id:     placeBlockSlotNineDecodeID,
			family: placeBlockFamily,
			relDir: placeBlockCorpusRelDir,
			op:     "decode",
			input:  mutated(clientControlPlaceBlockWire, 16, 0x09),
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     placeBlockInfinitePitchDecode,
			family: placeBlockFamily,
			relDir: placeBlockCorpusRelDir,
			op:     "decode",
			input:  mutatedWord(clientControlPlaceBlockWire, 12, 0x7f80_0000),
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     placeBlockSlotNineEncodeID,
			family: placeBlockFamily,
			relDir: placeBlockCorpusRelDir,
			op:     "encode",
			request: placeBlockEncodeRequest{
				Sequence: 0,
				Yaw:      "3fa00000",
				Pitch:    "80000000",
				Slot:     9,
			},
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:     resyncDecodeCaseID,
			family: requestChunkResyncFamily,
			relDir: requestChunkResyncCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientControlResyncWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   resyncFields,
			},
		},
		{
			id:      resyncEncodeCaseID,
			family:  requestChunkResyncFamily,
			relDir:  requestChunkResyncCorpusRelDir,
			op:      "encode",
			request: resyncRequest,
			wire:    append([]byte(nil), clientControlResyncWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   resyncFields,
			},
		},
		{
			id:     resyncDimensionTwoDecodeID,
			family: requestChunkResyncFamily,
			relDir: requestChunkResyncCorpusRelDir,
			op:     "decode",
			input:  mutatedWord(clientControlResyncWire, 8, 2),
			expect: Outcome{Kind: "error", Category: "invalid-enum"},
		},
		{
			id:     resyncDimensionTwoEncodeID,
			family: requestChunkResyncFamily,
			relDir: requestChunkResyncCorpusRelDir,
			op:     "encode",
			request: requestChunkResyncEncodeRequest{
				Sequence:     0,
				Dimension:    2,
				ChunkX:       -1,
				ChunkZ:       0,
				HaveRevision: 0,
			},
			expect: Outcome{Kind: "error", Category: "invalid-enum"},
		},
		{
			id:     selectHotbarDecodeCaseID,
			family: selectHotbarFamily,
			relDir: selectHotbarCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), clientControlSelectHotbarWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   selectHotbarFields,
			},
		},
		{
			id:      selectHotbarEncodeCaseID,
			family:  selectHotbarFamily,
			relDir:  selectHotbarCorpusRelDir,
			op:      "encode",
			request: selectHotbarRequest,
			wire:    append([]byte(nil), clientControlSelectHotbarWire...),
			expect: Outcome{
				Kind:     "ok",
				Category: packetOutcomeCategory,
				Fields:   selectHotbarFields,
			},
		},
		{
			id:     selectHotbarSlotNineDecodeID,
			family: selectHotbarFamily,
			relDir: selectHotbarCorpusRelDir,
			op:     "decode",
			input:  mutated(clientControlSelectHotbarWire, 8, 0x09),
			expect: Outcome{Kind: "error", Category: "invalid-value"},
		},
		{
			id:      selectHotbarSlotNineEncodeID,
			family:  selectHotbarFamily,
			relDir:  selectHotbarCorpusRelDir,
			op:      "encode",
			request: selectHotbarEncodeRequest{Sequence: 0, Slot: 9},
			expect:  Outcome{Kind: "error", Category: "invalid-value"},
		},
	}
}

// clientControlLabel renders one case's asset label from its identity.
func clientControlLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// clientControlCandidate builds one case's manifest entry and asset bytes from
// its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildClientControlCandidate(t *testing.T, definition clientControlCaseDefinition) clientControlCandidate {
	t.Helper()

	label := clientControlLabel(definition.id)
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
		Version:      clientControlVersion,
		Operation:    definition.op,
		PacketKey:    clientControlKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return clientControlCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// clientControlKeyPointer resolves one family's reviewed packet key for a case
// spec.
func clientControlKeyPointer(family string) *PacketKeySpec {
	key, owned := clientControlFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// clientControlCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type clientControlCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which the
	// producer's own bytes are compared against.
	Wire []byte
}

// clientControlCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func clientControlCandidates(t *testing.T, root string) []clientControlCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := clientControlCaseDefinitions()
	candidates := make([]clientControlCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildClientControlCandidate(t, definition)
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

// clientControlRegisteredCase reports whether the base manifest already carries
// one case identity, so a re-merge of an integrated candidate adds nothing.
func clientControlRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// clientControlSelection is this group's registration: its cases, the Go
// sources its rules are read from, and the routes the four families execute.
func clientControlSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := clientControlCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range clientControlFamilies() {
		sources[family.id] = clientControlClientSources()
	}
	routes := make([]ConsumerRoute, 0, len(clientControlFamilies())*2)
	for _, family := range clientControlFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: clientControlVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: clientControlVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  clientControlProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// clientControlClientSources lists the Go sources the four client-to-server
// control families read their rules from.
func clientControlClientSources() []string {
	return []string{
		"packages/shared/network/codec/codec_client.go",
		"packages/shared/network/protocol/message_command.go",
	}
}

// clientControlManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func clientControlManifest(t *testing.T, root string, candidates []clientControlCandidate) Inventory {
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
		if !clientControlOwnsFamily(family) {
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
		t.Fatalf("encode client control working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write client control working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load client control working manifest: %v", err)
	}
	return loaded
}

// clientControlOwnsFamily reports whether this group registers cases for one
// family.
func clientControlOwnsFamily(family string) bool {
	_, owned := clientControlFamilyKeys[family]
	return owned
}

// clientControlScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve under
// the root the runner is given.
func clientControlScratchRoot(t *testing.T, root string, candidates []clientControlCandidate) string {
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

// clientControlCandidateByID resolves one candidate by its case identity.
func clientControlCandidateByID(t *testing.T, candidates []clientControlCandidate, id string) clientControlCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return clientControlCandidate{}
}

// clientControlObservation resolves one executed observation by its case
// identity.
func clientControlObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// clientControlExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var clientControlExportPublished bool

// clientControlCandidatesExport publishes the reviewed candidates and the
// complete merged manifest candidate through the existing external exporter and
// returns the published producer directory. An unset export variable publishes
// nothing and returns "", so an ordinary test run never writes outside its own
// temporary storage.
func clientControlCandidatesExport(t *testing.T, root string, candidates []clientControlCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if clientControlExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-client-control")
	}
	clientControlExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, clientControlProducerID, assets)
	if err != nil {
		t.Fatalf("export client control candidates: %v", err)
	}
	return published
}

// TestProtocolClientControlOracleDecodesEveryValidCase pins that the real Go
// decoder publishes the canonical fields for every family's valid decode case,
// with the -0.0 look bits rendered as bit strings rather than numbers.
func TestProtocolClientControlOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientControlCandidates(t, root)

	for _, id := range []string{
		playerInputDecodeCaseID,
		placeBlockDecodeCaseID,
		resyncDecodeCaseID,
		selectHotbarDecodeCaseID,
	} {
		candidate := clientControlCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientControlDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientControlDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientControlOracleEncodesCanonicalWire pins each encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolClientControlOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientControlCandidates(t, root)

	for _, id := range []string{
		playerInputEncodeCaseID,
		placeBlockEncodeCaseID,
		resyncEncodeCaseID,
		selectHotbarEncodeCaseID,
	} {
		candidate := clientControlCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientControlEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientControlEncode(%s): %v", id, err)
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

// TestProtocolClientControlOracleRejectsMalformedCasesAtTheirBoundary pins that
// every malformed decode case and invalid encode case is refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolClientControlOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientControlCandidates(t, root)

	for _, id := range []string{
		playerInputNaNDecodeID,
		playerInputBoolTwoDecodeID,
		playerInputTrailingCaseID,
		placeBlockSlotNineDecodeID,
		placeBlockInfinitePitchDecode,
		resyncDimensionTwoDecodeID,
		selectHotbarSlotNineDecodeID,
	} {
		candidate := clientControlCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientControlDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientControlDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range []string{
		playerInputNaNEncodeID,
		placeBlockSlotNineEncodeID,
		resyncDimensionTwoEncodeID,
		selectHotbarSlotNineEncodeID,
	} {
		candidate := clientControlCandidateByID(t, candidates, id)
		outcome, encoded, err := runClientControlEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runClientControlEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientControlOracleExpectedFieldMutationFailsComparison pins that
// the recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded, and replacing the encoded
// digest fails comparison against the reviewed wire.
func TestProtocolClientControlOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientControlCandidates(t, root)

	decode := clientControlCandidateByID(t, candidates, playerInputDecodeCaseID)
	produced, _, err := runClientControlDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runClientControlDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	fields["yaw"] = "00000000"
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated yaw bit string compares equal to the produced outcome")
	}

	encode := clientControlCandidateByID(t, candidates, placeBlockEncodeCaseID)
	producedEncode, _, err := runClientControlEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runClientControlEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolClientControlOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolClientControlOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := clientControlCandidates(t, root)
	manifest := clientControlManifest(t, root, candidates)
	staged := clientControlScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, clientControlCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := clientControlObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolClientControlOracleRunnerRejectsUnregisteredRoute pins that a
// case naming a route this group does not claim fails before its producer runs,
// so a case cannot claim coverage from its name alone.
func TestProtocolClientControlOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := clientControlCandidates(t, root)
	manifest := clientControlManifest(t, root, candidates)
	staged := clientControlScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range clientControlFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: clientControlVersion, Operation: "decode"}] = runClientControlDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range clientControlFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+clientControlVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolClientControlOracleManifestMergeRegistersClientControlRoutes pins
// that the merged manifest registers all four families' routes and case lists,
// leaves the source revision alone, and records this group's provenance
// sources.
func TestProtocolClientControlOracleManifestMergeRegistersClientControlRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, clientControlSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range clientControlFamilies() {
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
		for _, want := range clientControlClientSources() {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so a
	// re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range clientControlCandidates(t, root) {
		if !clientControlRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolClientControlOracleCandidatesExportForReview publishes the
// reviewed candidates and the manifest candidate. An unset export variable
// publishes nothing, so the tracked corpus is never written by this package.
func TestProtocolClientControlOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := clientControlCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), clientControlSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := clientControlCandidatesExport(t, root, candidates, merged)
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
