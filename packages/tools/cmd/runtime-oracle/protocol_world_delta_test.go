package main

// This file is the world delta packet producer group: the two Play
// server-to-client families that carry a variable record count
// (`BlockChanges`, a canonical uvarint count plus fixed-stride block changes,
// and `ForgetChunks`, a canonical uvarint count plus fixed-stride chunk
// coordinates) each register a decode and an encode route through the shared
// packet-case runner, so every case is executed by the real Go codec rather
// than restated here.
//
// The decode cases are the Go decoder's own bytes: the valid payloads, the
// empty revision barrier, one mutated payload at each validator boundary, the
// count above the ceiling the decode arm refuses before its record-length
// rule, one trailing byte and a proper truncation inside a record. The encode
// cases carry canonical JSON fields and either the reviewed wire the Go
// encoder has to publish or a DTO the outbound validator has to refuse. Every
// negative carries exactly one violation. No case in this group declares a
// session, a subscription or a revision policy: per-session publication is a
// later authority concern and stays outside the protocol contract.

import (
	"bytes"
	"crypto/sha256"
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
	// blockChangesFamily is the play block changes family this group registers.
	blockChangesFamily = "protocol.server.BlockChanges"
	// forgetChunksFamily is the play forget chunks family this group registers.
	forgetChunksFamily = "protocol.server.ForgetChunks"
	// worldDeltaVersion is the protocol version both families are pinned to.
	worldDeltaVersion = "45"
	// worldDeltaProducerID is the exporter's producer identity for this group's
	// candidate assets and merged manifest.
	worldDeltaProducerID = "runtime-oracle/protocol-world-delta"

	// blockChangesCorpusRelDir is the repository-relative directory holding the
	// block changes family's committed case assets.
	blockChangesCorpusRelDir = corpusCasesRelDir + "/protocol/BlockChanges"
	// forgetChunksCorpusRelDir is the repository-relative directory holding the
	// forget chunks family's committed case assets.
	forgetChunksCorpusRelDir = corpusCasesRelDir + "/protocol/ForgetChunks"

	// worldDeltaMaxRecords is the record-count ceiling both families share on
	// the wire, which the decode arm refuses before its record-length rule.
	worldDeltaMaxRecords = 4096
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	blockChangesDecodeCaseID    = blockChangesFamily + "/" + worldDeltaVersion + "/decode-valid"
	blockChangesEncodeCaseID    = blockChangesFamily + "/" + worldDeltaVersion + "/encode-valid"
	blockChangesBarrierDecodeID = blockChangesFamily + "/" + worldDeltaVersion + "/decode-empty-barrier"
	blockChangesBarrierEncodeID = blockChangesFamily + "/" + worldDeltaVersion + "/encode-empty-barrier"
	blockChangesBaseZeroID      = blockChangesFamily + "/" + worldDeltaVersion + "/decode-base-zero"
	blockChangesRevisionGapID   = blockChangesFamily + "/" + worldDeltaVersion + "/decode-revision-gap"
	blockChangesAboveWorldID    = blockChangesFamily + "/" + worldDeltaVersion + "/decode-y-above-world"
	blockChangesWrongChunkID    = blockChangesFamily + "/" + worldDeltaVersion + "/decode-wrong-chunk"
	blockChangesUnregisteredID  = blockChangesFamily + "/" + worldDeltaVersion + "/decode-unregistered-block"
	blockChangesUnsortedID      = blockChangesFamily + "/" + worldDeltaVersion + "/decode-unsorted-index"
	blockChangesAboveWorldEncID = blockChangesFamily + "/" + worldDeltaVersion + "/encode-y-above-world"
	blockChangesCountAboveID    = blockChangesFamily + "/" + worldDeltaVersion + "/decode-count-above-max"
	blockChangesTruncatedID     = blockChangesFamily + "/" + worldDeltaVersion + "/decode-truncated"
	blockChangesTrailingID      = blockChangesFamily + "/" + worldDeltaVersion + "/decode-trailing-byte"
	forgetChunksDecodeCaseID    = forgetChunksFamily + "/" + worldDeltaVersion + "/decode-valid"
	forgetChunksEncodeCaseID    = forgetChunksFamily + "/" + worldDeltaVersion + "/encode-valid"
	forgetChunksZeroCountID     = forgetChunksFamily + "/" + worldDeltaVersion + "/decode-zero-count"
	forgetChunksDuplicateID     = forgetChunksFamily + "/" + worldDeltaVersion + "/decode-duplicate-chunk"
	forgetChunksDuplicateEncID  = forgetChunksFamily + "/" + worldDeltaVersion + "/encode-duplicate-chunk"
	forgetChunksDimensionTwoID  = forgetChunksFamily + "/" + worldDeltaVersion + "/decode-dimension-two"
	forgetChunksTruncatedID     = forgetChunksFamily + "/" + worldDeltaVersion + "/decode-truncated"
	forgetChunksTrailingID      = forgetChunksFamily + "/" + worldDeltaVersion + "/decode-trailing-byte"
)

// worldDeltaFamilyKeys pins each family's complete packet key. A case names its
// key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs. Both families are the first
// server-to-client play families of the plan.
var worldDeltaFamilyKeys = map[string]PacketKeySpec{
	blockChangesFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 1},
	forgetChunksFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 2},
}

// worldDeltaFamilies lists the two families in registry order, with the corpus
// directory that holds each family's assets. The order is the review order,
// not a dispatch table.
func worldDeltaFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{blockChangesFamily, blockChangesCorpusRelDir},
		{forgetChunksFamily, forgetChunksCorpusRelDir},
	}
}

// worldDeltaUvarint renders the canonical uvarint of one value, matching the
// Go primitive's shortest form, so a reviewed wire literal is the encoder's
// own encoding of the value it names.
func worldDeltaUvarint(value uint32) []byte {
	var encoded []byte
	for value >= 1<<7 {
		encoded = append(encoded, byte(value)|0x80)
		value >>= 7
	}
	return append(encoded, byte(value))
}

// worldDeltaBlockPos renders one change's wire record: the three little-endian
// i32 coordinates and the little-endian u16 block.
func worldDeltaBlockPos(x, y, z int32, block uint16) []byte {
	wire := make([]byte, 0, 14)
	for _, value := range []int32{x, y, z} {
		var field [4]byte
		field[0] = byte(value)
		field[1] = byte(value >> 8)
		field[2] = byte(value >> 16)
		field[3] = byte(value >> 24)
		wire = append(wire, field[:]...)
	}
	return append(wire, byte(block), byte(block>>8))
}

// worldDeltaChunkPos renders one chunk coordinate pair.
func worldDeltaChunkPos(x, z int32) []byte {
	wire := make([]byte, 0, 8)
	for _, value := range []int32{x, z} {
		var field [4]byte
		field[0] = byte(value)
		field[1] = byte(value >> 8)
		field[2] = byte(value >> 16)
		field[3] = byte(value >> 24)
		wire = append(wire, field[:]...)
	}
	return wire
}

// worldDeltaBlockChangesWire is the reviewed wire literal the canonical
// BlockChanges vector pins: dimension 1 (Depths), chunk (−1,0), base revision
// 1, new revision 2, uvarint count 1, and one change at x=−1, y=−64, z=0,
// block 1.
var worldDeltaBlockChangesWire = joinWire(
	[]byte{0x01, 0x00, 0x00, 0x00},                         // dimension
	[]byte{0xff, 0xff, 0xff, 0xff},                         // chunk X
	[]byte{0x00, 0x00, 0x00, 0x00},                         // chunk Z
	[]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // base revision
	[]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, // new revision
	[]byte{0x01},                      // count
	worldDeltaBlockPos(-1, -64, 0, 1), // one change
)

// worldDeltaBarrierWire is the reviewed empty revision barrier: the same
// header with uvarint count 0 and no records.
var worldDeltaBarrierWire = joinWire(
	[]byte{0x01, 0x00, 0x00, 0x00},
	[]byte{0xff, 0xff, 0xff, 0xff},
	[]byte{0x00, 0x00, 0x00, 0x00},
	[]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	[]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	[]byte{0x00},
)

// worldDeltaForgetChunksWire is the reviewed wire literal the canonical
// ForgetChunks vector pins: dimension 1 (Depths), uvarint count 2, and the
// chunks (1,0) and (−1,0) in the submitted order.
var worldDeltaForgetChunksWire = joinWire(
	[]byte{0x01, 0x00, 0x00, 0x00},
	[]byte{0x02},
	worldDeltaChunkPos(1, 0),
	worldDeltaChunkPos(-1, 0),
)

// joinWire concatenates reviewed byte groups into one payload literal.
func joinWire(groups ...[]byte) []byte {
	wire := make([]byte, 0)
	for _, group := range groups {
		wire = append(wire, group...)
	}
	return wire
}

// worldDeltaBlockChangesHeader is the reviewed header the mutation cases reuse:
// the dimension, chunk and revision fields of the canonical vector.
func worldDeltaBlockChangesHeader() []byte {
	return joinWire(
		[]byte{0x01, 0x00, 0x00, 0x00},
		[]byte{0xff, 0xff, 0xff, 0xff},
		[]byte{0x00, 0x00, 0x00, 0x00},
		[]byte{0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		[]byte{0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
	)
}

// worldDeltaRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and
// the outbound validators name, never over a sentinel identity, and a failure
// with no mapping is a hard error. The dimension violations of the two
// families are the invalid-enum boundary the domain owns; an unregistered
// block is the invalid-enum boundary the registered-block predicate owns;
// every revision, count, span, chunk-membership, sortedness, zero-count and
// duplicate rejection is the invalid-value range boundary. The decode arm's
// count bound fires before its record-length rule, so a count above the
// ceiling is resolved as invalid-value even when the remaining payload is also
// short, and only the length rule itself and the short-input primitive map to
// the truncated category.
func worldDeltaRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "dimension is not overworld or depths"):
		return "invalid-enum", true
	case strings.Contains(message, "block ID "):
		return "invalid-enum", true
	case strings.Contains(message, "invalid revision transition"),
		strings.Contains(message, "block changes count exceeds"),
		strings.Contains(message, "block Y "),
		strings.Contains(message, "is outside chunk"),
		strings.Contains(message, "block changes are not strictly index sorted"),
		strings.Contains(message, "forget chunks count is outside"),
		strings.Contains(message, "forget chunks contains a duplicate chunk"),
		strings.Contains(message, "packet count is outside"):
		return "invalid-value", true
	case strings.Contains(message, "packet count exceeds remaining payload"),
		strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	}
	return "", false
}

// worldDeltaPacketKey resolves one packet key to the Go state and numeric ID
// the codec dispatches on. Both families are server-to-client play packets, so
// a case naming another direction fails before the codec runs.
func worldDeltaPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := worldDeltaFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no world delta producer owns", c.ID, c.Family)
	}
	if *c.PacketKey != registered {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names key %+v, want %+v", c.ID, *c.PacketKey, registered)
	}
	if registered.State != packetStatePlay {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names state %q, want play", c.ID, registered.State)
	}
	if registered.Direction != packetDirectionServer {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names direction %q, want server-to-client", c.ID, registered.Direction)
	}
	return protocol.StatePlay, registered.ID, nil
}

// newWorldDeltaCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newWorldDeltaCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// worldDeltaChangeFields renders one block change's semantic fields: the
// absolute world coordinates and the block number as plain JSON integers.
func worldDeltaChangeFields(change protocol.BlockChange) map[string]any {
	return map[string]any{
		"x":     change.Position.X,
		"y":     change.Position.Y,
		"z":     change.Position.Z,
		"block": uint16(change.Block),
	}
}

// worldDeltaChunkFields renders one chunk coordinate's semantic fields.
func worldDeltaChunkFields(chunk core.ChunkPos) map[string]any {
	return map[string]any{
		"x": chunk.X,
		"z": chunk.Z,
	}
}

// worldDeltaFields renders the semantic fields one decoded world delta packet
// publishes.
//
// The fields come from the DTO the production decoder returned, never from the
// case input. The revisions render as decimal strings so the full u64 range
// stays lossless, the coordinates and blocks render as plain JSON integers,
// and the record arrays render in the submitted wire order: the forget batch
// is deliberately not sorted, because the wire order is the order the
// authority replayed. The empty barrier renders its change list as an empty
// array rather than a null.
func worldDeltaFields(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.BlockChanges:
		changes := make([]map[string]any, 0, len(message.Changes))
		for _, change := range message.Changes {
			changes = append(changes, worldDeltaChangeFields(change))
		}
		return map[string]any{
			"dimension":     int32(message.Dimension),
			"chunk_x":       message.Chunk.X,
			"chunk_z":       message.Chunk.Z,
			"base_revision": strconv.FormatUint(message.BaseRevision, 10),
			"new_revision":  strconv.FormatUint(message.NewRevision, 10),
			"changes":       changes,
		}, nil
	case protocol.ForgetChunks:
		chunks := make([]map[string]any, 0, len(message.Chunks))
		for _, chunk := range message.Chunks {
			chunks = append(chunks, worldDeltaChunkFields(chunk))
		}
		return map[string]any{
			"dimension": int32(message.Dimension),
			"chunks":    chunks,
		}, nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// runWorldDeltaDecode executes one world delta decode case through the real Go
// decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` and classifies
// the failure it returns. It never reimplements the count, coordinate, span or
// sortedness rules, so the recorded outcome is whatever the production codec
// decides about these exact bytes.
func runWorldDeltaDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := worldDeltaPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newWorldDeltaCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		category, classified := worldDeltaRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := worldDeltaFields(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// worldDeltaBlockChangeRequest is one block change's canonical JSON fields,
// which are the wire integers the record carries.
type worldDeltaBlockChangeRequest struct {
	X     int32  `json:"x"`
	Y     int32  `json:"y"`
	Z     int32  `json:"z"`
	Block uint16 `json:"block"`
}

// blockChangesEncodeRequest is the canonical JSON field input one BlockChanges
// encode case carries.
type blockChangesEncodeRequest struct {
	Dimension    int32                          `json:"dimension"`
	ChunkX       int32                          `json:"chunk_x"`
	ChunkZ       int32                          `json:"chunk_z"`
	BaseRevision uint64                         `json:"base_revision"`
	NewRevision  uint64                         `json:"new_revision"`
	Changes      []worldDeltaBlockChangeRequest `json:"changes"`
}

// forgetChunksEncodeRequest is the canonical JSON field input one ForgetChunks
// encode case carries.
type forgetChunksEncodeRequest struct {
	Dimension int32                    `json:"dimension"`
	Chunks    []worldDeltaChunkRequest `json:"chunks"`
}

// worldDeltaChunkRequest is one chunk coordinate's canonical JSON fields.
type worldDeltaChunkRequest struct {
	X int32 `json:"x"`
	Z int32 `json:"z"`
}

// worldDeltaPacket builds the DTO one encode case names from its typed fields,
// so the outbound validator decides about the same record the decode path
// would publish.
func worldDeltaPacket(c CaseSpec, input []byte) (protocol.ServerPacket, error) {
	switch c.Family {
	case blockChangesFamily:
		var request blockChangesEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return nil, err
		}
		changes := make([]protocol.BlockChange, 0, len(request.Changes))
		for _, change := range request.Changes {
			changes = append(changes, protocol.BlockChange{
				Position: core.BlockPos{X: change.X, Y: change.Y, Z: change.Z},
				Block:    core.BlockID(change.Block),
			})
		}
		return protocol.BlockChanges{
			Dimension:    core.DimensionID(request.Dimension),
			Chunk:        core.ChunkPos{X: request.ChunkX, Z: request.ChunkZ},
			BaseRevision: request.BaseRevision,
			NewRevision:  request.NewRevision,
			Changes:      changes,
		}, nil
	case forgetChunksFamily:
		var request forgetChunksEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return nil, err
		}
		chunks := make([]core.ChunkPos, 0, len(request.Chunks))
		for _, chunk := range request.Chunks {
			chunks = append(chunks, core.ChunkPos{X: chunk.X, Z: chunk.Z})
		}
		return protocol.ForgetChunks{
			Dimension: core.DimensionID(request.Dimension),
			Chunks:    chunks,
		}, nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}
}

// runWorldDeltaEncode executes one world delta encode case through the real Go
// encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects
// must never be recorded as evidence.
func runWorldDeltaEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := worldDeltaPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newWorldDeltaCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := worldDeltaPacket(c, input)
	if err != nil {
		return Outcome{}, nil, err
	}
	encodedID, payload, err := wireCodec.EncodeServer(state, packet)
	if err != nil {
		category, classified := worldDeltaRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified encode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	if encodedID != packetID {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoder published packet ID %d, want %d", c.ID, encodedID, packetID)
	}
	decoded, err := wireCodec.DecodeServer(state, encodedID, payload)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload does not decode back: %w", c.ID, err)
	}
	if !reflect.DeepEqual(decoded, packet) {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: encoded payload decodes to %#v, want %#v", c.ID, decoded, packet)
	}
	fields, err := worldDeltaFields(c, decoded)
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

// worldDeltaCorpusRoutes is the closed route map the two families execute.
func worldDeltaCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range worldDeltaFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: worldDeltaVersion, Operation: "decode"}] = runWorldDeltaDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: worldDeltaVersion, Operation: "encode"}] = runWorldDeltaEncode
	}
	return routes
}

// worldDeltaCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer
// result.
type worldDeltaCaseDefinition struct {
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

// worldDeltaCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid
// cases use the canonical payloads the Go encoder produces for these fields,
// including the empty revision barrier; the malformed decode cases mutate that
// payload at one boundary each, and the invalid encode cases build a DTO the
// production validator refuses, so each rejection names the boundary that owns
// it. The count-above-max case is refused by the decode arm's count bound
// before its record-length rule, which is the order both implementations pin.
func worldDeltaCaseDefinitions() []worldDeltaCaseDefinition {
	validChanges := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: map[string]any{
			"dimension":     int32(1),
			"chunk_x":       int32(-1),
			"chunk_z":       int32(0),
			"base_revision": "1",
			"new_revision":  "2",
			"changes": []map[string]any{
				{"x": int32(-1), "y": int32(-64), "z": int32(0), "block": uint16(1)},
			},
		},
	}
	barrier := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: map[string]any{
			"dimension":     int32(1),
			"chunk_x":       int32(-1),
			"chunk_z":       int32(0),
			"base_revision": "1",
			"new_revision":  "2",
			"changes":       []map[string]any{},
		},
	}
	validForget := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: map[string]any{
			"dimension": int32(1),
			"chunks": []map[string]any{
				{"x": int32(1), "z": int32(0)},
				{"x": int32(-1), "z": int32(0)},
			},
		},
	}
	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	invalidEnum := Outcome{Kind: "error", Category: "invalid-enum"}
	truncated := Outcome{Kind: "error", Category: "truncated"}
	trailing := Outcome{Kind: "error", Category: "trailing"}

	// Mutations of the reviewed header and record bytes. The header carries
	// the dimension, the chunk and the two revisions, so a base-zero or
	// revision-gap mutation rewrites the revision bytes in place on a copy and
	// keeps the empty-barrier count, which leaves the revision transition as
	// the single violation the validator answers.
	baseZero := append(append([]byte(nil), worldDeltaBlockChangesHeader()...), 0x00)
	copy(baseZero[12:20], make([]byte, 8))
	revisionGap := append(append([]byte(nil), worldDeltaBlockChangesHeader()...), 0x00)
	revisionGap[20] = 0x03
	aboveWorld := joinWire(
		worldDeltaBlockChangesHeader(),
		[]byte{0x01},
		worldDeltaBlockPos(-1, 320, 0, 1),
	)
	wrongChunk := joinWire(
		worldDeltaBlockChangesHeader(),
		[]byte{0x01},
		worldDeltaBlockPos(15, -64, 0, 1),
	)
	unregisteredBlock := joinWire(
		worldDeltaBlockChangesHeader(),
		[]byte{0x01},
		worldDeltaBlockPos(-1, -64, 0, 90),
	)
	unsortedIndex := joinWire(
		worldDeltaBlockChangesHeader(),
		worldDeltaUvarint(2),
		worldDeltaBlockPos(-1, -64, 1, 1),
		worldDeltaBlockPos(-1, -64, 0, 1),
	)
	countAboveMax := joinWire(
		worldDeltaBlockChangesHeader(),
		worldDeltaUvarint(worldDeltaMaxRecords+1),
		worldDeltaBlockPos(-1, -64, 0, 1),
	)
	blockChangesFull := joinWire(
		worldDeltaBlockChangesHeader(),
		[]byte{0x01},
		worldDeltaBlockPos(-1, -64, 0, 1),
	)
	blockChangesTruncated := blockChangesFull[:38]
	forgetFull := joinWire(
		[]byte{0x01, 0x00, 0x00, 0x00},
		[]byte{0x02},
		worldDeltaChunkPos(1, 0),
		worldDeltaChunkPos(-1, 0),
	)
	forgetTruncated := forgetFull[:9]
	forgetZeroCount := joinWire([]byte{0x01, 0x00, 0x00, 0x00}, []byte{0x00})
	forgetDuplicate := joinWire(
		[]byte{0x01, 0x00, 0x00, 0x00},
		[]byte{0x02},
		worldDeltaChunkPos(1, 0),
		worldDeltaChunkPos(1, 0),
	)
	dimensionTwo := joinWire(
		[]byte{0x02, 0x00, 0x00, 0x00},
		[]byte{0x02},
		worldDeltaChunkPos(1, 0),
		worldDeltaChunkPos(-1, 0),
	)

	definitions := make([]worldDeltaCaseDefinition, 0, 22)
	definitions = append(definitions,
		worldDeltaCaseDefinition{
			id:     blockChangesDecodeCaseID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), worldDeltaBlockChangesWire...),
			expect: validChanges,
		},
		worldDeltaCaseDefinition{
			id:      blockChangesEncodeCaseID,
			family:  blockChangesFamily,
			relDir:  blockChangesCorpusRelDir,
			op:      "encode",
			request: blockChangesEncodeRequest{Dimension: 1, ChunkX: -1, ChunkZ: 0, BaseRevision: 1, NewRevision: 2, Changes: []worldDeltaBlockChangeRequest{{X: -1, Y: -64, Z: 0, Block: 1}}},
			wire:    append([]byte(nil), worldDeltaBlockChangesWire...),
			expect:  validChanges,
		},
		worldDeltaCaseDefinition{
			id:     blockChangesBarrierDecodeID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), worldDeltaBarrierWire...),
			expect: barrier,
		},
		worldDeltaCaseDefinition{
			id:      blockChangesBarrierEncodeID,
			family:  blockChangesFamily,
			relDir:  blockChangesCorpusRelDir,
			op:      "encode",
			request: blockChangesEncodeRequest{Dimension: 1, ChunkX: -1, ChunkZ: 0, BaseRevision: 1, NewRevision: 2, Changes: []worldDeltaBlockChangeRequest{}},
			wire:    append([]byte(nil), worldDeltaBarrierWire...),
			expect:  barrier,
		},
		worldDeltaCaseDefinition{
			id:     blockChangesBaseZeroID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  baseZero,
			expect: invalidValue,
		},
		worldDeltaCaseDefinition{
			id:     blockChangesRevisionGapID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  revisionGap,
			expect: invalidValue,
		},
		worldDeltaCaseDefinition{
			id:     blockChangesAboveWorldID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  aboveWorld,
			expect: invalidValue,
		},
		worldDeltaCaseDefinition{
			id:     blockChangesWrongChunkID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  wrongChunk,
			expect: invalidValue,
		},
		worldDeltaCaseDefinition{
			id:     blockChangesUnregisteredID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  unregisteredBlock,
			expect: invalidEnum,
		},
		worldDeltaCaseDefinition{
			id:     blockChangesUnsortedID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  unsortedIndex,
			expect: invalidValue,
		},
		worldDeltaCaseDefinition{
			id:      blockChangesAboveWorldEncID,
			family:  blockChangesFamily,
			relDir:  blockChangesCorpusRelDir,
			op:      "encode",
			request: blockChangesEncodeRequest{Dimension: 1, ChunkX: -1, ChunkZ: 0, BaseRevision: 1, NewRevision: 2, Changes: []worldDeltaBlockChangeRequest{{X: -1, Y: 320, Z: 0, Block: 1}}},
			expect:  invalidValue,
		},
		worldDeltaCaseDefinition{
			id:     blockChangesCountAboveID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  countAboveMax,
			expect: invalidValue,
		},
		worldDeltaCaseDefinition{
			id:     blockChangesTruncatedID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  blockChangesTruncated,
			expect: truncated,
		},
		worldDeltaCaseDefinition{
			id:     blockChangesTrailingID,
			family: blockChangesFamily,
			relDir: blockChangesCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), worldDeltaBlockChangesWire...), 0x00),
			expect: trailing,
		},
		worldDeltaCaseDefinition{
			id:     forgetChunksDecodeCaseID,
			family: forgetChunksFamily,
			relDir: forgetChunksCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), worldDeltaForgetChunksWire...),
			expect: validForget,
		},
		worldDeltaCaseDefinition{
			id:      forgetChunksEncodeCaseID,
			family:  forgetChunksFamily,
			relDir:  forgetChunksCorpusRelDir,
			op:      "encode",
			request: forgetChunksEncodeRequest{Dimension: 1, Chunks: []worldDeltaChunkRequest{{X: 1, Z: 0}, {X: -1, Z: 0}}},
			wire:    append([]byte(nil), worldDeltaForgetChunksWire...),
			expect:  validForget,
		},
		worldDeltaCaseDefinition{
			id:     forgetChunksZeroCountID,
			family: forgetChunksFamily,
			relDir: forgetChunksCorpusRelDir,
			op:     "decode",
			input:  forgetZeroCount,
			expect: invalidValue,
		},
		worldDeltaCaseDefinition{
			id:     forgetChunksDuplicateID,
			family: forgetChunksFamily,
			relDir: forgetChunksCorpusRelDir,
			op:     "decode",
			input:  forgetDuplicate,
			expect: invalidValue,
		},
		worldDeltaCaseDefinition{
			id:      forgetChunksDuplicateEncID,
			family:  forgetChunksFamily,
			relDir:  forgetChunksCorpusRelDir,
			op:      "encode",
			request: forgetChunksEncodeRequest{Dimension: 1, Chunks: []worldDeltaChunkRequest{{X: 1, Z: 0}, {X: 1, Z: 0}}},
			expect:  invalidValue,
		},
		worldDeltaCaseDefinition{
			id:     forgetChunksDimensionTwoID,
			family: forgetChunksFamily,
			relDir: forgetChunksCorpusRelDir,
			op:     "decode",
			input:  dimensionTwo,
			expect: invalidEnum,
		},
		worldDeltaCaseDefinition{
			id:     forgetChunksTruncatedID,
			family: forgetChunksFamily,
			relDir: forgetChunksCorpusRelDir,
			op:     "decode",
			input:  forgetTruncated,
			expect: truncated,
		},
		worldDeltaCaseDefinition{
			id:     forgetChunksTrailingID,
			family: forgetChunksFamily,
			relDir: forgetChunksCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), worldDeltaForgetChunksWire...), 0x00),
			expect: trailing,
		},
	)

	return definitions
}

// worldDeltaLabel renders one case's asset label from its identity.
func worldDeltaLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// buildWorldDeltaCandidate builds one case's manifest entry and asset bytes
// from its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildWorldDeltaCandidate(t *testing.T, definition worldDeltaCaseDefinition) worldDeltaCandidate {
	t.Helper()

	label := worldDeltaLabel(definition.id)
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
		Version:      worldDeltaVersion,
		Operation:    definition.op,
		PacketKey:    worldDeltaKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return worldDeltaCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// worldDeltaKeyPointer resolves one family's reviewed packet key for a case
// spec.
func worldDeltaKeyPointer(family string) *PacketKeySpec {
	key, owned := worldDeltaFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// worldDeltaCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type worldDeltaCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which the
	// producer's own bytes are compared against.
	Wire []byte
}

// worldDeltaCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func worldDeltaCandidates(t *testing.T, root string) []worldDeltaCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := worldDeltaCaseDefinitions()
	candidates := make([]worldDeltaCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildWorldDeltaCandidate(t, definition)
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

// worldDeltaRegisteredCase reports whether the base manifest already carries
// one case identity, so a re-merge of an integrated candidate adds nothing.
func worldDeltaRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// worldDeltaSelection is this group's registration: its cases, the Go sources
// its rules are read from, and the routes the two families execute.
func worldDeltaSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := worldDeltaCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range worldDeltaFamilies() {
		sources[family.id] = worldDeltaServerSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(worldDeltaFamilies())*2)
	for _, family := range worldDeltaFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: worldDeltaVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: worldDeltaVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  worldDeltaProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// worldDeltaServerSources lists the Go sources each family reads its rules
// from: the shared server codec dispatch with its decode arms, and the
// family's own message file, which owns the validator the shared dispatch
// applies at the tail.
func worldDeltaServerSources(family string) []string {
	shared := []string{
		"packages/shared/network/codec/codec_server.go",
	}
	switch family {
	case blockChangesFamily:
		return append(shared, "packages/shared/network/protocol/snapshot.go")
	case forgetChunksFamily:
		return append(shared, "packages/shared/network/protocol/message_chunk.go")
	default:
		return nil
	}
}

// worldDeltaManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func worldDeltaManifest(t *testing.T, root string, candidates []worldDeltaCandidate) Inventory {
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
		if !worldDeltaOwnsFamily(family) {
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
		t.Fatalf("encode world delta working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write world delta working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load world delta working manifest: %v", err)
	}
	return loaded
}

// worldDeltaOwnsFamily reports whether this group registers cases for one
// family.
func worldDeltaOwnsFamily(family string) bool {
	_, owned := worldDeltaFamilyKeys[family]
	return owned
}

// worldDeltaScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve
// under the root the runner is given.
func worldDeltaScratchRoot(t *testing.T, root string, candidates []worldDeltaCandidate) string {
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

// worldDeltaCandidateByID resolves one candidate by its case identity.
func worldDeltaCandidateByID(t *testing.T, candidates []worldDeltaCandidate, id string) worldDeltaCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return worldDeltaCandidate{}
}

// worldDeltaObservation resolves one executed observation by its case
// identity.
func worldDeltaObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// worldDeltaExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var worldDeltaExportPublished bool

// worldDeltaCandidatesExport publishes the reviewed candidates and the
// complete merged manifest candidate through the existing external exporter
// and returns the published producer directory. An unset export variable
// publishes nothing and returns "", so an ordinary test run never writes
// outside its own temporary storage.
func worldDeltaCandidatesExport(t *testing.T, root string, candidates []worldDeltaCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if worldDeltaExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-world-delta")
	}
	worldDeltaExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, worldDeltaProducerID, assets)
	if err != nil {
		t.Fatalf("export world delta candidates: %v", err)
	}
	return published
}

// worldDeltaDecodeCaseIDs lists the valid decode cases the producer pins.
func worldDeltaDecodeCaseIDs() []string {
	return []string{
		blockChangesDecodeCaseID,
		blockChangesBarrierDecodeID,
		forgetChunksDecodeCaseID,
	}
}

// worldDeltaEncodeCaseIDs lists the valid encode cases the producer pins.
func worldDeltaEncodeCaseIDs() []string {
	return []string{
		blockChangesEncodeCaseID,
		blockChangesBarrierEncodeID,
		forgetChunksEncodeCaseID,
	}
}

// worldDeltaRejectedDecodeCaseIDs lists the malformed decode cases.
func worldDeltaRejectedDecodeCaseIDs() []string {
	return []string{
		blockChangesBaseZeroID,
		blockChangesRevisionGapID,
		blockChangesAboveWorldID,
		blockChangesWrongChunkID,
		blockChangesUnregisteredID,
		blockChangesUnsortedID,
		blockChangesCountAboveID,
		blockChangesTruncatedID,
		blockChangesTrailingID,
		forgetChunksZeroCountID,
		forgetChunksDuplicateID,
		forgetChunksDimensionTwoID,
		forgetChunksTruncatedID,
		forgetChunksTrailingID,
	}
}

// worldDeltaRejectedEncodeCaseIDs lists the invalid encode cases.
func worldDeltaRejectedEncodeCaseIDs() []string {
	return []string{
		blockChangesAboveWorldEncID,
		forgetChunksDuplicateEncID,
	}
}

// TestProtocolWorldDeltaOracleDecodesEveryValidCase pins that the real Go
// decoder publishes the canonical fields for every valid decode case,
// including the empty revision barrier and the unsorted forget batch.
func TestProtocolWorldDeltaOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := worldDeltaCandidates(t, root)

	for _, id := range worldDeltaDecodeCaseIDs() {
		candidate := worldDeltaCandidateByID(t, candidates, id)
		outcome, encoded, err := runWorldDeltaDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runWorldDeltaDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolWorldDeltaOracleEncodesCanonicalWire pins the encode producer
// against the reviewed wire literals and against the recorded digests.
func TestProtocolWorldDeltaOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := worldDeltaCandidates(t, root)

	for _, id := range worldDeltaEncodeCaseIDs() {
		candidate := worldDeltaCandidateByID(t, candidates, id)
		outcome, encoded, err := runWorldDeltaEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runWorldDeltaEncode(%s): %v", id, err)
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

// TestProtocolWorldDeltaOracleRejectsMalformedCasesAtTheirBoundary pins that
// every malformed decode case and invalid encode case is refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolWorldDeltaOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := worldDeltaCandidates(t, root)

	for _, id := range worldDeltaRejectedDecodeCaseIDs() {
		candidate := worldDeltaCandidateByID(t, candidates, id)
		outcome, encoded, err := runWorldDeltaDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runWorldDeltaDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range worldDeltaRejectedEncodeCaseIDs() {
		candidate := worldDeltaCandidateByID(t, candidates, id)
		outcome, encoded, err := runWorldDeltaEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runWorldDeltaEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolWorldDeltaOracleCountBoundFiresBeforeTheRecordLength pins the
// controller ruling this group's count-above-max case rests on: the Go decode
// arm refuses a count above the ceiling before it compares the remaining
// payload with the record stride, so the payload carrying one record is
// answered with the count refusal and the invalid-value category rather than a
// length failure. The Rust decoder mirrors that order.
func TestProtocolWorldDeltaOracleCountBoundFiresBeforeTheRecordLength(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := worldDeltaCandidates(t, root)

	candidate := worldDeltaCandidateByID(t, candidates, blockChangesCountAboveID)
	outcome, _, err := runWorldDeltaDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runWorldDeltaDecode(%s): %v", candidate.Spec.ID, err)
	}
	if outcome.Kind != "error" || outcome.Category != "invalid-value" {
		t.Fatalf("case %s produced %#v, want the invalid-value category", candidate.Spec.ID, outcome)
	}

	// A payload that cuts inside the first change record is the length rule's
	// own boundary, so the resolver cannot pass by classifying every count
	// case as a value failure. The reviewed payload is 29 header-and-count
	// bytes plus the 14-byte record, and this case stops inside the record.
	short := worldDeltaCandidateByID(t, candidates, blockChangesTruncatedID)
	shortInput := short.Assets[short.Spec.Input.Path]
	if len(shortInput) < 29 || len(shortInput) >= 43 {
		t.Fatalf("the truncated case is not inside the change record: %d bytes", len(shortInput))
	}
	shortOutcome, _, err := runWorldDeltaDecode(short.Spec, short.Assets[short.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runWorldDeltaDecode(%s): %v", short.Spec.ID, err)
	}
	if shortOutcome.Kind != "error" || shortOutcome.Category != "truncated" {
		t.Fatalf("case %s produced %#v, want the truncated category", short.Spec.ID, shortOutcome)
	}
}

// TestProtocolWorldDeltaOracleExpectedFieldMutationFailsComparison pins that
// the recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded, and replacing the encoded
// digest fails comparison against the reviewed wire.
func TestProtocolWorldDeltaOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := worldDeltaCandidates(t, root)

	decode := worldDeltaCandidateByID(t, candidates, blockChangesDecodeCaseID)
	produced, _, err := runWorldDeltaDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runWorldDeltaDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	changes := make([]map[string]any, 0, 1)
	for _, change := range fields["changes"].([]map[string]any) {
		copied := make(map[string]any, len(change))
		for key, value := range change {
			copied[key] = value
		}
		copied["y"] = int32(-63)
		changes = append(changes, copied)
	}
	fields["changes"] = changes
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated change Y compares equal to the produced outcome")
	}

	forget := worldDeltaCandidateByID(t, candidates, forgetChunksDecodeCaseID)
	producedForget, _, err := runWorldDeltaDecode(forget.Spec, forget.Assets[forget.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runWorldDeltaDecode: %v", err)
	}
	forgetFields := make(map[string]any, len(forget.Expect.Fields))
	for key, value := range forget.Expect.Fields {
		forgetFields[key] = value
	}
	// The wire order is the contract: a normalization that sorted the chunks
	// would hide a reordering, so the mutation swaps the two submitted chunks.
	forgetFields["chunks"] = []map[string]any{
		{"x": int32(-1), "z": int32(0)},
		{"x": int32(1), "z": int32(0)},
	}
	mutatedForget := forget.Expect
	mutatedForget.Fields = forgetFields
	if outcomesEqual(mutatedForget, producedForget) {
		t.Fatal("mutated chunk order compares equal to the produced outcome")
	}

	encode := worldDeltaCandidateByID(t, candidates, blockChangesEncodeCaseID)
	producedEncode, _, err := runWorldDeltaEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runWorldDeltaEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolWorldDeltaOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolWorldDeltaOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := worldDeltaCandidates(t, root)
	manifest := worldDeltaManifest(t, root, candidates)
	staged := worldDeltaScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, worldDeltaCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := worldDeltaObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolWorldDeltaOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so
// a case cannot claim coverage from its name alone.
func TestProtocolWorldDeltaOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := worldDeltaCandidates(t, root)
	manifest := worldDeltaManifest(t, root, candidates)
	staged := worldDeltaScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range worldDeltaFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: worldDeltaVersion, Operation: "decode"}] = runWorldDeltaDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range worldDeltaFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+worldDeltaVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolWorldDeltaOracleManifestMergeRegistersWorldDeltaRoutes pins that
// the merged manifest registers both families' routes and case lists, leaves
// the source revision alone, and records this group's provenance sources.
func TestProtocolWorldDeltaOracleManifestMergeRegistersWorldDeltaRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, worldDeltaSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range worldDeltaFamilies() {
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
		for _, want := range worldDeltaServerSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so a
	// re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range worldDeltaCandidates(t, root) {
		if !worldDeltaRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolWorldDeltaOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolWorldDeltaOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := worldDeltaCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), worldDeltaSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := worldDeltaCandidatesExport(t, root, candidates, merged)
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
