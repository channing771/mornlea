package main

// This file is the companion packet producer group: the three Play
// server-to-client families that publish the companions one subscriber can see
// (`CompanionSpawn`, `CompanionStates` and `CompanionDespawn`) each register a
// decode and an encode route through the shared packet-case runner, so every
// case is executed by the real Go codec rather than restated here.
//
// The decode cases are the Go decoder's own bytes: the canonical payloads, the
// batch count and order boundaries, and one structural mutation each. The
// encode cases carry canonical JSON fields and either the reviewed wire the Go
// encoder has to publish or a DTO the outbound validator has to refuse. Every
// negative carries exactly one violation.
//
// The Go `CompanionSpawn.Validate` and per-record `CompanionState.validate` are
// folded single-message predicates (`network: invalid companion spawn` and
// `invalid companion state`), so those messages resolve at the value boundary
// this group freezes: the name, the pose and the pitch range. The spawn's zero
// or wrong-version identity and its depths dimension are not corpus cases,
// because the single message cannot publish a distinct category for them and
// the Rust group test pins them at their own variants instead. The distinct
// batch count and order messages resolve at the value boundary, the despawn's
// identity message at the identity boundary, and the fixed payload ceilings at
// the capacity boundary. One boundary is a prefix rule rather than a trailing
// rule: the Go decoder applies an exact remaining-length check for this batch,
// so a payload with one extra byte reports `network: companion states length
// does not match count`, and the despawn's 16-byte ceiling refuses its extra
// byte before any field is read. Both resolve at the boundaries the Rust
// consumer publishes for the same bytes.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// companionSpawnFamily is the play companion spawn family this group registers.
	companionSpawnFamily = "protocol.server.CompanionSpawn"
	// companionDespawnFamily is the play companion despawn family this group registers.
	companionDespawnFamily = "protocol.server.CompanionDespawn"
	// companionStatesFamily is the play companion state batch family this group registers.
	companionStatesFamily = "protocol.server.CompanionStates"
	// companionsVersion is the protocol version all three families are pinned to.
	companionsVersion = "45"
	// companionsProducerID is the exporter's producer identity for this
	// group's candidate assets and merged manifest.
	companionsProducerID = "runtime-oracle/protocol-companions"

	// companionSpawnCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	companionSpawnCorpusRelDir = corpusCasesRelDir + "/protocol/CompanionSpawn"
	// companionDespawnCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	companionDespawnCorpusRelDir = corpusCasesRelDir + "/protocol/CompanionDespawn"
	// companionStatesCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	companionStatesCorpusRelDir = corpusCasesRelDir + "/protocol/CompanionStates"

	// companionStateWireBytes is the fixed record stride.
	companionStateWireBytes = protocol.CompanionStateWireBytes
	// companionsMaxRecords is the companion-activity budget one batch may carry.
	companionsMaxRecords = protocol.MaxCompanionStates
	// companionsMaxWireBytes is the fixed payload ceiling the Go decoder
	// applies before it allocates.
	companionsMaxWireBytes = protocol.CompanionStatesMaxWireBytes
	// companionSpawnMaxWireBytes is the fixed payload ceiling the Go decoder
	// applies to a spawn payload before it allocates.
	companionSpawnMaxWireBytes = protocol.CompanionSpawnMaxWireBytes
	// companionDespawnMaxWireBytes is the fixed payload ceiling the Go decoder
	// applies to a despawn payload, which equals the identity stride.
	companionDespawnMaxWireBytes = len(protocol.CompanionDespawn{}.ID)
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	companionsspawnDecodeCaseID     = companionSpawnFamily + "/" + companionsVersion + "/decode-valid"
	companionsspawnEncodeCaseID     = companionSpawnFamily + "/" + companionsVersion + "/encode-valid"
	companionsspawnSpacedNameDecID  = companionSpawnFamily + "/" + companionsVersion + "/decode-embedded-space-name"
	companionsspawnSpacedNameEncID  = companionSpawnFamily + "/" + companionsVersion + "/encode-embedded-space-name"
	companionsspawnPitchAboveDecID  = companionSpawnFamily + "/" + companionsVersion + "/decode-pitch-above-limit"
	companionsspawnNaNYawDecID      = companionSpawnFamily + "/" + companionsVersion + "/decode-nan-yaw"
	companionsdespawnDecodeCaseID   = companionDespawnFamily + "/" + companionsVersion + "/decode-valid"
	companionsdespawnEncodeCaseID   = companionDespawnFamily + "/" + companionsVersion + "/encode-valid"
	companionsdespawnZeroIDDecID    = companionDespawnFamily + "/" + companionsVersion + "/decode-zero-id"
	companionsdespawnTrailingID     = companionDespawnFamily + "/" + companionsVersion + "/decode-trailing-byte"
	companionsstatesDecodeCaseID    = companionStatesFamily + "/" + companionsVersion + "/decode-valid"
	companionsstatesEncodeCaseID    = companionStatesFamily + "/" + companionsVersion + "/encode-valid"
	companionsstatesCountFourID     = companionStatesFamily + "/" + companionsVersion + "/decode-count-four"
	companionsstatesCountZeroID     = companionStatesFamily + "/" + companionsVersion + "/decode-count-zero"
	companionsstatesCountFiveID     = companionStatesFamily + "/" + companionsVersion + "/decode-count-five"
	companionsstatesDuplicateID     = companionStatesFamily + "/" + companionsVersion + "/decode-duplicate-ids"
	companionsstatesReversedID      = companionStatesFamily + "/" + companionsVersion + "/decode-reversed-ids"
	companionsstatesPitchAboveDecID = companionStatesFamily + "/" + companionsVersion + "/decode-pitch-above-limit"
	companionsstatesTrailingID      = companionStatesFamily + "/" + companionsVersion + "/decode-trailing-byte"
)

// companionFamilyKeys pins each family's complete packet key. A case names its
// key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs. All three families are
// server-to-client play packets.
var companionFamilyKeys = map[string]PacketKeySpec{
	companionSpawnFamily:   {Direction: packetDirectionServer, State: packetStatePlay, ID: 17},
	companionDespawnFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 19},
	companionStatesFamily:  {Direction: packetDirectionServer, State: packetStatePlay, ID: 18},
}

// companionFamilies lists the three families in registry order, with the corpus
// directory that holds each family's assets. The order is the review order, not
// a dispatch table.
func companionFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{companionSpawnFamily, companionSpawnCorpusRelDir},
		{companionDespawnFamily, companionDespawnCorpusRelDir},
		{companionStatesFamily, companionStatesCorpusRelDir},
	}
}

// companionsU64 renders one little-endian u64 field.
func companionsU64(value uint64) []byte {
	wire := make([]byte, 8)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// companionsU32 renders one little-endian u32 field.
func companionsU32(value uint32) []byte {
	wire := make([]byte, 4)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// companionsFloatWire renders one IEEE-754 binary32 field, preserving the exact
// bits so a negative zero survives the wire round trip.
func companionsFloatWire(value float32) []byte {
	return companionsU32(math.Float32bits(value))
}

// companionsUvarint renders one canonical uvarint count prefix.
func companionsUvarint(value uint32) []byte {
	wire := make([]byte, 0, 5)
	for value >= 1<<7 {
		wire = append(wire, byte(value)|0x80)
		value >>= 7
	}
	return append(wire, byte(value))
}

// companionsNameWire renders one length-prefixed name slot, so a name the Go
// encoder would refuse is still describable byte for byte.
func companionsNameWire(name string) []byte {
	wire := append([]byte(nil), companionsUvarint(uint32(len(name)))...)
	return append(wire, []byte(name)...)
}

// companionsNegZero is the negative zero the reviewed vectors carry.
func companionsNegZero() float32 {
	return float32(math.Copysign(0, -1))
}

// companionsHalfPi is the inclusive pitch limit a companion pose carries.
func companionsHalfPi() float32 {
	return float32(math.Pi / 2)
}

// companionsNextAboveHalfPi is the next binary32 above the inclusive limit.
func companionsNextAboveHalfPi() float32 {
	return math.Float32frombits(math.Float32bits(companionsHalfPi()) + 1)
}

// companionsControlCompanion is the reviewed identity: UUIDv4 with the version
// and variant nibbles the control companion carries.
func companionsControlCompanion() companion.ID {
	return companion.ID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}
}

// companionsAscendingCompanion renders one identity of the full four-record
// batch, ascending in unsigned byte order.
func companionsAscendingCompanion(index uint8) companion.ID {
	return companion.ID{0, 0, 0, 0, 0, 0, 0x40 + index, 0, 0x80 + index, 0, 0, 0, 0, 0, 0, 0}
}

// companionsControlFromText decodes one identity text for the wire builders.
func companionsControlFromText(text string) companion.ID {
	id, err := companionsParseID(text)
	if err != nil {
		panic("runtime-oracle: reviewed identity is not hexadecimal: " + err.Error())
	}
	return id
}

// companionsPositionFromText decodes one position bit-string array for the wire
// builders.
func companionsPositionFromText(values []string) mgl32.Vec3 {
	position, err := companionsParseBits(values)
	if err != nil {
		panic("runtime-oracle: reviewed position is not hexadecimal: " + err.Error())
	}
	return position
}

// companionsAngleFromText decodes one angle bit string for the wire builders.
func companionsAngleFromText(text string) float32 {
	value, err := companionsParseBitsText(text)
	if err != nil {
		panic("runtime-oracle: reviewed angle is not hexadecimal: " + err.Error())
	}
	return value
}

// companionsSpawnWire renders one spawn payload in the wire field order: the
// identity, the length-prefixed name, the tick, the dimension, the position,
// the yaw and the pitch.
func companionsSpawnWire(id companion.ID, name string, tick uint64, dimension int32, position mgl32.Vec3, yaw, pitch float32) []byte {
	wire := make([]byte, 0, 16+2+len(name)+8+4+20)
	wire = append(wire, id[:]...)
	wire = append(wire, companionsNameWire(name)...)
	wire = append(wire, companionsU64(tick)...)
	wire = append(wire, companionsU32(uint32(dimension))...)
	for _, value := range position {
		wire = append(wire, companionsFloatWire(value)...)
	}
	wire = append(wire, companionsFloatWire(yaw)...)
	return append(wire, companionsFloatWire(pitch)...)
}

// companionsCanonicalSpawnWire is the reviewed spawn literal: the control
// identity, the canonical companion name "Mira", tick 0, the overworld, a
// position carrying a negative zero, yaw 0 and the pitch at exactly half a
// turn.
func companionsCanonicalSpawnWire() []byte {
	return companionsSpawnWire(companionsControlCompanion(), "Mira", 0, int32(core.Overworld),
		mgl32.Vec3{companionsNegZero(), 1, 2}, 0, companionsHalfPi())
}

// companionsSpacedNameSpawnWire is the name-bound mutation: the same record
// with the companion name "Mira Bell", which carries an embedded space the
// companion name rule rejects.
func companionsSpacedNameSpawnWire() []byte {
	return companionsSpawnWire(companionsControlCompanion(), "Mira Bell", 0, int32(core.Overworld),
		mgl32.Vec3{companionsNegZero(), 1, 2}, 0, companionsHalfPi())
}

// companionsPitchAboveSpawnWire is the pitch-bound mutation: the same record
// with the next binary32 above the inclusive limit.
func companionsPitchAboveSpawnWire() []byte {
	return companionsSpawnWire(companionsControlCompanion(), "Mira", 0, int32(core.Overworld),
		mgl32.Vec3{companionsNegZero(), 1, 2}, 0, companionsNextAboveHalfPi())
}

// companionsNaNYawSpawnWire is the finiteness mutation: the same record with a
// NaN yaw, which the decoder's float primitive answers before the validator
// runs.
func companionsNaNYawSpawnWire() []byte {
	return companionsSpawnWire(companionsControlCompanion(), "Mira", 0, int32(core.Overworld),
		mgl32.Vec3{companionsNegZero(), 1, 2}, float32(math.NaN()), companionsHalfPi())
}

// companionsDespawnWire renders one despawn payload: the identity alone.
func companionsDespawnWire(id companion.ID) []byte {
	wire := make([]byte, 0, companionDespawnMaxWireBytes)
	return append(wire, id[:]...)
}

// companionsStateRecord is one batch record before it is published.
type companionsStateRecord struct {
	id       companion.ID
	dim      int32
	position mgl32.Vec3
	yaw      float32
	pitch    float32
	reset    bool
}

// companionsRecordWire renders one record in the wire field order.
func companionsRecordWire(record companionsStateRecord) []byte {
	wire := make([]byte, 0, companionStateWireBytes)
	wire = append(wire, record.id[:]...)
	wire = append(wire, companionsU32(uint32(record.dim))...)
	for _, value := range record.position {
		wire = append(wire, companionsFloatWire(value)...)
	}
	wire = append(wire, companionsFloatWire(record.yaw)...)
	wire = append(wire, companionsFloatWire(record.pitch)...)
	if record.reset {
		return append(wire, 1)
	}
	return append(wire, 0)
}

// companionsRecordsWire renders the ordered record region.
func companionsRecordsWire(records []companionsStateRecord) []byte {
	wire := make([]byte, 0, len(records)*companionStateWireBytes)
	for _, record := range records {
		wire = append(wire, companionsRecordWire(record)...)
	}
	return wire
}

// companionsStatesWire renders one batch payload: the tick, the canonical
// uvarint count and the ordered records.
func companionsStatesWire(tick uint64, declared uint32, records []companionsStateRecord) []byte {
	wire := append([]byte(nil), companionsU64(tick)...)
	wire = append(wire, companionsUvarint(declared)...)
	return append(wire, companionsRecordsWire(records)...)
}

// companionsSingleRecord is the reviewed one-record batch the canonical vector
// carries.
func companionsSingleRecord() []companionsStateRecord {
	return []companionsStateRecord{{
		id:       companionsControlCompanion(),
		dim:      int32(core.Overworld),
		position: mgl32.Vec3{companionsNegZero(), 1, 2},
		yaw:      0,
		pitch:    companionsHalfPi(),
		reset:    false,
	}}
}

// companionsFullRecords is the full four-record batch: four identities
// ascending in unsigned byte order, which is exactly the fixed wire ceiling.
func companionsFullRecords() []companionsStateRecord {
	records := make([]companionsStateRecord, 0, companionsMaxRecords)
	for index := uint8(0); index < uint8(companionsMaxRecords); index++ {
		records = append(records, companionsStateRecord{
			id:       companionsAscendingCompanion(index),
			dim:      int32(core.Overworld),
			position: mgl32.Vec3{1, 2, 3},
			yaw:      0,
			pitch:    companionsHalfPi(),
			reset:    index%2 == 0,
		})
	}
	return records
}

// companionsWithFloat replaces one binary32 field of a payload at offset.
func companionsWithFloat(source []byte, offset int, value float32) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[offset:offset+4], companionsFloatWire(value))
	return wire
}

// companionsFloatBitsText renders one binary32 as its eight-digit
// lowercase-hexadecimal bit string, the canonical float encoding both
// directions publish.
func companionsFloatBitsText(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// companionsIDText renders one identity as its 32-lowercase-hexadecimal text,
// so a zero or non-UUIDv4 byte sequence stays observable.
func companionsIDText(id companion.ID) string {
	return hex.EncodeToString(id[:])
}

// companionsSpawnFields renders the semantic fields one spawn publishes.
//
// The tick is a decimal string so the full u64 range stays lossless, the
// dimension is the plain wire integer, and the pose publishes as bit strings
// so a negative zero stays distinct.
func companionsSpawnFields(spawn protocol.CompanionSpawn) map[string]any {
	return map[string]any{
		"companion_id": companionsIDText(spawn.ID),
		"name":         spawn.Name,
		"tick":         strconv.FormatUint(spawn.Tick, 10),
		"dimension":    int32(spawn.Dimension),
		"position": []string{
			companionsFloatBitsText(spawn.Position[0]),
			companionsFloatBitsText(spawn.Position[1]),
			companionsFloatBitsText(spawn.Position[2]),
		},
		"yaw":   companionsFloatBitsText(spawn.Yaw),
		"pitch": companionsFloatBitsText(spawn.Pitch),
	}
}

// companionsDespawnFields renders the semantic fields one despawn publishes.
func companionsDespawnFields(despawn protocol.CompanionDespawn) map[string]any {
	return map[string]any{
		"companion_id": companionsIDText(despawn.ID),
	}
}

// companionsStateFields renders the semantic fields one batch record publishes.
func companionsStateFields(state protocol.CompanionState) map[string]any {
	return map[string]any{
		"companion_id": companionsIDText(state.ID),
		"dimension":    int32(state.Dimension),
		"position": []string{
			companionsFloatBitsText(state.Position[0]),
			companionsFloatBitsText(state.Position[1]),
			companionsFloatBitsText(state.Position[2]),
		},
		"yaw":   companionsFloatBitsText(state.Yaw),
		"pitch": companionsFloatBitsText(state.Pitch),
		"reset": state.Reset,
	}
}

// companionsStatesFields renders the semantic fields one batch publishes.
//
// The records publish in wire order, never sorted, so a batch the authority
// ordered is observed in the order it carried.
func companionsStatesFields(states protocol.CompanionStates) map[string]any {
	records := make([]map[string]any, 0, len(states.States))
	for _, state := range states.States {
		records = append(records, companionsStateFields(state))
	}
	return map[string]any{
		"tick":   strconv.FormatUint(states.Tick, 10),
		"states": records,
	}
}

// companionsRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and the
// outbound validators name, never over a sentinel identity, and a failure with
// no mapping is a hard error. The primitive answers precede the validator
// messages, because the decoder rejects a non-finite float while reading and
// only then hands the record to `Validate`.
//
// The folded spawn and per-record messages resolve at the value boundary,
// which is the one category the name, pose and pitch cases share with the Rust
// string and float variants. The batch count, order and remaining-length
// messages each resolve at the value boundary except the remaining-length one,
// which is the truncation boundary the Rust exact-record rule publishes. The
// fixed ceilings resolve at the capacity boundary, and the despawn's identity
// message at the identity boundary.
func companionsRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "payload exceeds fixed maximum"):
		return "capacity", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "invalid float32"):
		return "invalid-value", true
	case strings.Contains(message, "companion state count is outside 1..4"):
		return "invalid-value", true
	case strings.Contains(message, "companion states length does not match count"):
		return "truncated", true
	case strings.Contains(message, "companion states are not strictly sorted"):
		return "invalid-value", true
	case strings.Contains(message, "companion state "):
		return "invalid-value", true
	case strings.Contains(message, "invalid companion spawn"):
		return "invalid-value", true
	case strings.Contains(message, "invalid companion despawn"):
		return "invalid-identity", true
	}
	return "", false
}

// companionsPacketKey resolves one packet key to the Go state and numeric ID
// the codec dispatches on, and reports whether the packet travels
// server-to-client.
func companionsPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := companionFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no companion producer owns", c.ID, c.Family)
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

// newCompanionsCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newCompanionsCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// companionsFieldsForPacket renders the semantic fields of one decoded
// publication packet, dispatching on the concrete DTO the production decoder
// returned.
func companionsFieldsForPacket(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.CompanionSpawn:
		return companionsSpawnFields(message), nil
	case protocol.CompanionDespawn:
		return companionsDespawnFields(message), nil
	case protocol.CompanionStates:
		return companionsStatesFields(message), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// runCompanionsDecode executes one publication decode case through the real Go
// decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` and classifies
// the failure it returns. It never reimplements the length, name or ordering
// rules, so the recorded outcome is whatever the production codec decides
// about these exact bytes.
func runCompanionsDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := companionsPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newCompanionsCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		category, classified := companionsRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := companionsFieldsForPacket(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// companionsSpawnRequest is the canonical JSON field input one spawn encode
// case carries.
type companionsSpawnRequest struct {
	CompanionID string   `json:"companion_id"`
	Name        string   `json:"name"`
	Tick        uint64   `json:"tick"`
	Dimension   int32    `json:"dimension"`
	Position    []string `json:"position"`
	Yaw         string   `json:"yaw"`
	Pitch       string   `json:"pitch"`
}

// core renders the request as the Go spawn DTO the encoder validates.
func (request companionsSpawnRequest) core() (protocol.CompanionSpawn, error) {
	id, err := companionsParseID(request.CompanionID)
	if err != nil {
		return protocol.CompanionSpawn{}, err
	}
	position, err := companionsParseBits(request.Position)
	if err != nil {
		return protocol.CompanionSpawn{}, err
	}
	yaw, err := companionsParseBitsText(request.Yaw)
	if err != nil {
		return protocol.CompanionSpawn{}, err
	}
	pitch, err := companionsParseBitsText(request.Pitch)
	if err != nil {
		return protocol.CompanionSpawn{}, err
	}
	return protocol.CompanionSpawn{
		ID:        id,
		Name:      request.Name,
		Tick:      request.Tick,
		Dimension: core.DimensionID(request.Dimension),
		Position:  position,
		Yaw:       yaw,
		Pitch:     pitch,
	}, nil
}

// companionsDespawnRequest is the canonical JSON field input one despawn encode
// case carries.
type companionsDespawnRequest struct {
	CompanionID string `json:"companion_id"`
}

// core renders the request as the Go despawn DTO the encoder validates.
func (request companionsDespawnRequest) core() (protocol.CompanionDespawn, error) {
	id, err := companionsParseID(request.CompanionID)
	if err != nil {
		return protocol.CompanionDespawn{}, err
	}
	return protocol.CompanionDespawn{ID: id}, nil
}

// companionsStateRequest is the canonical JSON field input one state record
// carries.
type companionsStateRequest struct {
	CompanionID string   `json:"companion_id"`
	Dimension   int32    `json:"dimension"`
	Position    []string `json:"position"`
	Yaw         string   `json:"yaw"`
	Pitch       string   `json:"pitch"`
	Reset       bool     `json:"reset"`
}

// core renders the request as the Go state record DTO.
func (request companionsStateRequest) core() (protocol.CompanionState, error) {
	id, err := companionsParseID(request.CompanionID)
	if err != nil {
		return protocol.CompanionState{}, err
	}
	position, err := companionsParseBits(request.Position)
	if err != nil {
		return protocol.CompanionState{}, err
	}
	yaw, err := companionsParseBitsText(request.Yaw)
	if err != nil {
		return protocol.CompanionState{}, err
	}
	pitch, err := companionsParseBitsText(request.Pitch)
	if err != nil {
		return protocol.CompanionState{}, err
	}
	return protocol.CompanionState{
		ID:        id,
		Dimension: core.DimensionID(request.Dimension),
		Position:  position,
		Yaw:       yaw,
		Pitch:     pitch,
		Reset:     request.Reset,
	}, nil
}

// companionsStatesRequest is the canonical JSON field input one batch encode
// case carries.
type companionsStatesRequest struct {
	Tick   uint64                   `json:"tick"`
	States []companionsStateRequest `json:"states"`
}

// core renders the request as the Go batch DTO the encoder validates.
func (request companionsStatesRequest) core() (protocol.CompanionStates, error) {
	states := make([]protocol.CompanionState, 0, len(request.States))
	for index, record := range request.States {
		state, err := record.core()
		if err != nil {
			return protocol.CompanionStates{}, fmt.Errorf("runtime-oracle: state %d: %w", index, err)
		}
		states = append(states, state)
	}
	return protocol.CompanionStates{Tick: request.Tick, States: states}, nil
}

// companionsParseID decodes one 32-hexadecimal identity field.
func companionsParseID(text string) (companion.ID, error) {
	var id companion.ID
	raw, err := hex.DecodeString(text)
	if err != nil {
		return id, fmt.Errorf("runtime-oracle: identity %q is not hexadecimal: %w", text, err)
	}
	if len(raw) != len(id) {
		return id, fmt.Errorf("runtime-oracle: identity %q is not %d bytes", text, len(id))
	}
	copy(id[:], raw)
	return id, nil
}

// companionsParseBits decodes one three-element bit-string array.
func companionsParseBits(values []string) (mgl32.Vec3, error) {
	if len(values) != 3 {
		return mgl32.Vec3{}, fmt.Errorf("runtime-oracle: position carries %d components", len(values))
	}
	var parsed [3]float32
	for index, text := range values {
		value, err := companionsParseBitsText(text)
		if err != nil {
			return mgl32.Vec3{}, err
		}
		parsed[index] = value
	}
	return mgl32.Vec3{parsed[0], parsed[1], parsed[2]}, nil
}

// companionsParseBitsText decodes one eight-digit hexadecimal bit string.
func companionsParseBitsText(text string) (float32, error) {
	bits, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: bits %q are not hexadecimal: %w", text, err)
	}
	return math.Float32frombits(uint32(bits)), nil
}

// runCompanionsEncode executes one publication encode case through the real Go
// encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects
// must never be recorded as evidence.
func runCompanionsEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := companionsPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newCompanionsCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ServerPacket
	switch c.Family {
	case companionSpawnFamily:
		var request companionsSpawnRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		spawn, err := request.core()
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = spawn
	case companionDespawnFamily:
		var request companionsDespawnRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		despawn, err := request.core()
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = despawn
	case companionStatesFamily:
		var request companionsStatesRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		states, err := request.core()
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = states
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	encodedID, payload, err := wireCodec.EncodeServer(state, packet)
	if err != nil {
		category, classified := companionsRejectionCategory(err)
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
	fields, err := companionsFieldsForPacket(c, decoded)
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

// companionsCorpusRoutes is the closed route map the three families execute.
func companionsCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range companionFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: companionsVersion, Operation: "decode"}] = runCompanionsDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: companionsVersion, Operation: "encode"}] = runCompanionsEncode
	}
	return routes
}

// companionsCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer
// result.
type companionsCaseDefinition struct {
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

// companionsBitRequest renders one binary32 as its canonical JSON request text.
func companionsBitRequest(value float32) string {
	return companionsFloatBitsText(value)
}

// companionsSpawnRequestFields is the canonical JSON request the valid spawn
// encode case carries, which is the reviewed wire's own field set.
func companionsSpawnRequestFields() companionsSpawnRequest {
	return companionsSpawnRequest{
		CompanionID: companionsIDText(companionsControlCompanion()),
		Name:        "Mira",
		Tick:        0,
		Dimension:   int32(core.Overworld),
		Position: []string{
			companionsBitRequest(companionsNegZero()),
			companionsBitRequest(1),
			companionsBitRequest(2),
		},
		Yaw:   companionsBitRequest(0),
		Pitch: companionsBitRequest(companionsHalfPi()),
	}
}

// companionsSpacedNameSpawnRequest is the invalid encode request whose only
// violation is the companion name rule: the same record with "Mira Bell".
func companionsSpacedNameSpawnRequest() companionsSpawnRequest {
	request := companionsSpawnRequestFields()
	request.Name = "Mira Bell"
	return request
}

// companionsDespawnRequestFields is the canonical JSON request the valid
// despawn encode case carries.
func companionsDespawnRequestFields() companionsDespawnRequest {
	return companionsDespawnRequest{CompanionID: companionsIDText(companionsControlCompanion())}
}

// companionsStateRequestFields renders one record as its canonical JSON request
// fields.
func companionsStateRequestFields(record companionsStateRecord) companionsStateRequest {
	return companionsStateRequest{
		CompanionID: companionsIDText(record.id),
		Dimension:   record.dim,
		Position: []string{
			companionsBitRequest(record.position[0]),
			companionsBitRequest(record.position[1]),
			companionsBitRequest(record.position[2]),
		},
		Yaw:   companionsBitRequest(record.yaw),
		Pitch: companionsBitRequest(record.pitch),
		Reset: record.reset,
	}
}

// companionsStatesRequestFields is the canonical JSON request the valid batch
// encode case carries.
func companionsStatesRequestFields() companionsStatesRequest {
	records := companionsSingleRecord()
	states := make([]companionsStateRequest, 0, len(records))
	for _, record := range records {
		states = append(states, companionsStateRequestFields(record))
	}
	return companionsStatesRequest{Tick: 0, States: states}
}

// companionsSpawnDTO is the reviewed canonical spawn.
func companionsSpawnDTO() protocol.CompanionSpawn {
	spawn, err := companionsSpawnRequestFields().core()
	if err != nil {
		panic("runtime-oracle: the reviewed spawn request is not a valid DTO: " + err.Error())
	}
	return spawn
}

// companionsDespawnDTO is the reviewed canonical despawn.
func companionsDespawnDTO() protocol.CompanionDespawn {
	despawn, err := companionsDespawnRequestFields().core()
	if err != nil {
		panic("runtime-oracle: the reviewed despawn request is not a valid DTO: " + err.Error())
	}
	return despawn
}

// companionsStatesDTO is the reviewed canonical batch.
func companionsStatesDTO() protocol.CompanionStates {
	states, err := companionsStatesRequestFields().core()
	if err != nil {
		panic("runtime-oracle: the reviewed states request is not a valid DTO: " + err.Error())
	}
	return states
}

// companionsFullStatesDTO is the reviewed full four-record batch, the fixed
// companion-activity ceiling.
func companionsFullStatesDTO() protocol.CompanionStates {
	records := companionsFullRecords()
	states := make([]protocol.CompanionState, 0, len(records))
	for _, record := range records {
		state, err := companionsStateRequestFields(record).core()
		if err != nil {
			panic("runtime-oracle: the reviewed full batch record is not a valid DTO: " + err.Error())
		}
		states = append(states, state)
	}
	return protocol.CompanionStates{Tick: 0, States: states}
}

// companionsCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid cases
// use the canonical payloads the Go encoder produces for these fields,
// including the full four-record boundary; the malformed decode cases mutate
// that payload at one boundary each, and the invalid encode case builds a DTO
// the production validator refuses, so each rejection names the boundary that
// owns it.
//
// The spawn's identity and dimension boundaries are deliberately absent: the Go
// validator answers them with the same message as the name and finiteness
// boundaries, so a corpus case could not distinguish them and the Rust
// group-test pins them instead. No case combines two violations.
func companionsCaseDefinitions() []companionsCaseDefinition {
	validSpawn := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   companionsSpawnFields(companionsSpawnDTO()),
	}
	validDespawn := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   companionsDespawnFields(companionsDespawnDTO()),
	}
	validStates := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   companionsStatesFields(companionsStatesDTO()),
	}
	validCountFour := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   companionsStatesFields(companionsFullStatesDTO()),
	}
	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	invalidIdentity := Outcome{Kind: "error", Category: "invalid-identity"}
	capacity := Outcome{Kind: "error", Category: "capacity"}
	truncated := Outcome{Kind: "error", Category: "truncated"}

	spawnWire := companionsCanonicalSpawnWire()
	despawnWire := companionsDespawnWire(companionsControlCompanion())
	statesWire := companionsStatesWire(0, 1, companionsSingleRecord())
	countFourWire := companionsStatesWire(0, uint32(companionsMaxRecords), companionsFullRecords())

	// A one-record batch's pitch is the four bytes before its trailing reset
	// flag, after the tick, the count, the identity, the dimension and the
	// position.
	const statesPitchOffset = 45

	definitions := make([]companionsCaseDefinition, 0, 19)
	definitions = append(definitions,
		companionsCaseDefinition{
			id:     companionsspawnDecodeCaseID,
			family: companionSpawnFamily,
			relDir: companionSpawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), spawnWire...),
			expect: validSpawn,
		},
		companionsCaseDefinition{
			id:      companionsspawnEncodeCaseID,
			family:  companionSpawnFamily,
			relDir:  companionSpawnCorpusRelDir,
			op:      "encode",
			request: companionsSpawnRequestFields(),
			wire:    append([]byte(nil), spawnWire...),
			expect:  validSpawn,
		},
		companionsCaseDefinition{
			id:     companionsspawnSpacedNameDecID,
			family: companionSpawnFamily,
			relDir: companionSpawnCorpusRelDir,
			op:     "decode",
			input:  companionsSpacedNameSpawnWire(),
			expect: invalidValue,
		},
		companionsCaseDefinition{
			id:      companionsspawnSpacedNameEncID,
			family:  companionSpawnFamily,
			relDir:  companionSpawnCorpusRelDir,
			op:      "encode",
			request: companionsSpacedNameSpawnRequest(),
			expect:  invalidValue,
		},
		companionsCaseDefinition{
			// The next binary32 above the inclusive half-turn limit is
			// finite, so the decoder's float primitive admits it and the
			// validator's pitch range answers, which is the boundary both
			// sides publish.
			id:     companionsspawnPitchAboveDecID,
			family: companionSpawnFamily,
			relDir: companionSpawnCorpusRelDir,
			op:     "decode",
			input:  companionsPitchAboveSpawnWire(),
			expect: invalidValue,
		},
		companionsCaseDefinition{
			// The NaN yaw is answered by the decoder's float primitive
			// before the validator runs, which is the boundary both sides
			// publish.
			id:     companionsspawnNaNYawDecID,
			family: companionSpawnFamily,
			relDir: companionSpawnCorpusRelDir,
			op:     "decode",
			input:  companionsNaNYawSpawnWire(),
			expect: invalidValue,
		},
		companionsCaseDefinition{
			id:     companionsdespawnDecodeCaseID,
			family: companionDespawnFamily,
			relDir: companionDespawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), despawnWire...),
			expect: validDespawn,
		},
		companionsCaseDefinition{
			id:      companionsdespawnEncodeCaseID,
			family:  companionDespawnFamily,
			relDir:  companionDespawnCorpusRelDir,
			op:      "encode",
			request: companionsDespawnRequestFields(),
			wire:    append([]byte(nil), despawnWire...),
			expect:  validDespawn,
		},
		companionsCaseDefinition{
			id:     companionsdespawnZeroIDDecID,
			family: companionDespawnFamily,
			relDir: companionDespawnCorpusRelDir,
			op:     "decode",
			input:  companionsDespawnWire(companion.ID{}),
			expect: invalidIdentity,
		},
		companionsCaseDefinition{
			// The despawn payload is exactly one identity, so the fixed
			// ceiling refuses the extra byte before any field is read rather
			// than answering with a trailing-byte boundary.
			id:     companionsdespawnTrailingID,
			family: companionDespawnFamily,
			relDir: companionDespawnCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), despawnWire...), 0x00),
			expect: capacity,
		},
		companionsCaseDefinition{
			id:     companionsstatesDecodeCaseID,
			family: companionStatesFamily,
			relDir: companionStatesCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), statesWire...),
			expect: validStates,
		},
		companionsCaseDefinition{
			id:      companionsstatesEncodeCaseID,
			family:  companionStatesFamily,
			relDir:  companionStatesCorpusRelDir,
			op:      "encode",
			request: companionsStatesRequestFields(),
			wire:    append([]byte(nil), statesWire...),
			expect:  validStates,
		},
		companionsCaseDefinition{
			// Four records is the count and wire ceiling, admitted exactly at
			// the fixed payload bound.
			id:     companionsstatesCountFourID,
			family: companionStatesFamily,
			relDir: companionStatesCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), countFourWire...),
			expect: validCountFour,
		},
		companionsCaseDefinition{
			id:     companionsstatesCountZeroID,
			family: companionStatesFamily,
			relDir: companionStatesCorpusRelDir,
			op:     "decode",
			input:  companionsStatesWire(0, 0, nil),
			expect: invalidValue,
		},
		companionsCaseDefinition{
			// A tiny payload whose declared count is above the ceiling: the
			// count bound fires before the record-length rule on both sides,
			// while a full five-record payload would exceed the fixed ceiling
			// first and answer at the capacity boundary instead.
			id:     companionsstatesCountFiveID,
			family: companionStatesFamily,
			relDir: companionStatesCorpusRelDir,
			op:     "decode",
			input:  companionsStatesWire(0, uint32(companionsMaxRecords)+1, companionsSingleRecord()),
			expect: invalidValue,
		},
		companionsCaseDefinition{
			id:     companionsstatesDuplicateID,
			family: companionStatesFamily,
			relDir: companionStatesCorpusRelDir,
			op:     "decode",
			input: companionsStatesWire(0, 2, []companionsStateRecord{
				companionsSingleRecord()[0],
				companionsSingleRecord()[0],
			}),
			expect: invalidValue,
		},
		companionsCaseDefinition{
			// The descending twin: the byte-successor identity first, so the
			// strict-order rule answers.
			id:     companionsstatesReversedID,
			family: companionStatesFamily,
			relDir: companionStatesCorpusRelDir,
			op:     "decode",
			input: companionsStatesWire(0, 2, []companionsStateRecord{
				{
					id:       companion.ID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 16},
					dim:      int32(core.Overworld),
					position: mgl32.Vec3{companionsNegZero(), -1.5, 0},
					yaw:      0,
					pitch:    companionsHalfPi(),
					reset:    true,
				},
				companionsSingleRecord()[0],
			}),
			expect: invalidValue,
		},
		companionsCaseDefinition{
			id:     companionsstatesPitchAboveDecID,
			family: companionStatesFamily,
			relDir: companionStatesCorpusRelDir,
			op:     "decode",
			input:  companionsWithFloat(statesWire, statesPitchOffset, companionsNextAboveHalfPi()),
			expect: invalidValue,
		},
		companionsCaseDefinition{
			// This batch is the one family whose Go decoder applies an exact
			// remaining-length rule, so the extra byte reports the
			// remaining-length message rather than the trailing-byte boundary.
			id:     companionsstatesTrailingID,
			family: companionStatesFamily,
			relDir: companionStatesCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), statesWire...), 0x00),
			expect: truncated,
		},
	)

	return definitions
}

// companionsLabel renders one case's asset label from its identity.
func companionsLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// buildCompanionsCandidate builds one case's manifest entry and asset bytes
// from its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildCompanionsCandidate(t *testing.T, definition companionsCaseDefinition) companionsCandidate {
	t.Helper()

	label := companionsLabel(definition.id)
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
		Version:      companionsVersion,
		Operation:    definition.op,
		PacketKey:    companionsKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return companionsCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// companionsKeyPointer resolves one family's reviewed packet key for a case
// spec.
func companionsKeyPointer(family string) *PacketKeySpec {
	key, owned := companionFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// companionsCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type companionsCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which
	// the producer's own bytes are compared against.
	Wire []byte
}

// companionsCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func companionsCandidates(t *testing.T, root string) []companionsCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := companionsCaseDefinitions()
	candidates := make([]companionsCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildCompanionsCandidate(t, definition)
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

// companionsRegisteredCase reports whether the base manifest already carries
// one case identity, so a re-merge of an integrated candidate adds nothing.
func companionsRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// companionsSelection is this group's registration: its cases, the Go sources
// its rules are read from, and the routes the three families execute.
func companionsSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := companionsCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range companionFamilies() {
		sources[family.id] = companionsServerSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(companionFamilies())*2)
	for _, family := range companionFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: companionsVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: companionsVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  companionsProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// companionsServerSources lists the Go sources each family reads its rules
// from: the shared server codec dispatch with its decode arms and the payload
// ceiling, the family's own message file, which owns the validator the shared
// dispatch applies at the tail, and the family's wire codec, which owns the
// field order and the record stride.
func companionsServerSources(family string) []string {
	return []string{
		"packages/shared/network/codec/codec_server.go",
		"packages/shared/network/protocol/message_companion.go",
		"packages/shared/network/codec/companion_wire.go",
	}
}

// companionsManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func companionsManifest(t *testing.T, root string, candidates []companionsCandidate) Inventory {
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
		if !companionsOwnsFamily(family) {
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
		t.Fatalf("encode companions working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write companions working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load companions working manifest: %v", err)
	}
	return loaded
}

// companionsOwnsFamily reports whether this group registers cases for one
// family.
func companionsOwnsFamily(family string) bool {
	_, owned := companionFamilyKeys[family]
	return owned
}

// companionsScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve under
// the root the runner is given.
func companionsScratchRoot(t *testing.T, root string, candidates []companionsCandidate) string {
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

// companionsCandidateByID resolves one candidate by its case identity.
func companionsCandidateByID(t *testing.T, candidates []companionsCandidate, id string) companionsCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return companionsCandidate{}
}

// companionsObservation resolves one executed observation by its case identity.
func companionsObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// companionsExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var companionsExportPublished bool

// companionsCandidatesExport publishes the reviewed candidates and the
// complete merged manifest candidate through the existing external exporter and
// returns the published producer directory. An unset export variable publishes
// nothing and returns "", so an ordinary test run never writes outside its own
// temporary storage.
func companionsCandidatesExport(t *testing.T, root string, candidates []companionsCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if companionsExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-companions")
	}
	companionsExportPublished = true
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
	published, err := exportGeneratedAssets(root, exportRoot, companionsProducerID, assets)
	if err != nil {
		t.Fatalf("export companions candidates: %v", err)
	}
	return published
}

// companionsDecodeCaseIDs lists the valid decode cases the producer pins.
func companionsDecodeCaseIDs() []string {
	return []string{
		companionsspawnDecodeCaseID,
		companionsdespawnDecodeCaseID,
		companionsstatesDecodeCaseID,
		companionsstatesCountFourID,
	}
}

// companionsEncodeCaseIDs lists the valid encode cases the producer pins.
func companionsEncodeCaseIDs() []string {
	return []string{
		companionsspawnEncodeCaseID,
		companionsdespawnEncodeCaseID,
		companionsstatesEncodeCaseID,
	}
}

// companionsRejectedDecodeCaseIDs lists the malformed decode cases.
func companionsRejectedDecodeCaseIDs() []string {
	return []string{
		companionsspawnSpacedNameDecID,
		companionsspawnPitchAboveDecID,
		companionsspawnNaNYawDecID,
		companionsdespawnZeroIDDecID,
		companionsdespawnTrailingID,
		companionsstatesCountZeroID,
		companionsstatesCountFiveID,
		companionsstatesDuplicateID,
		companionsstatesReversedID,
		companionsstatesPitchAboveDecID,
		companionsstatesTrailingID,
	}
}

// companionsRejectedEncodeCaseIDs lists the invalid encode cases.
func companionsRejectedEncodeCaseIDs() []string {
	return []string{companionsspawnSpacedNameEncID}
}

// TestProtocolCompanionsOracleDecodesEveryValidCase pins that the real Go
// decoder publishes the canonical fields for every family's valid decode case,
// including the full four-record boundary.
func TestProtocolCompanionsOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := companionsCandidates(t, root)

	for _, id := range companionsDecodeCaseIDs() {
		candidate := companionsCandidateByID(t, candidates, id)
		outcome, encoded, err := runCompanionsDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runCompanionsDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolCompanionsOracleEncodesCanonicalWire pins each encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolCompanionsOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := companionsCandidates(t, root)

	for _, id := range companionsEncodeCaseIDs() {
		candidate := companionsCandidateByID(t, candidates, id)
		outcome, encoded, err := runCompanionsEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runCompanionsEncode(%s): %v", id, err)
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

// TestProtocolCompanionsOracleRejectsMalformedCasesAtTheirBoundary pins that
// every malformed decode case and the invalid encode case are refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolCompanionsOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := companionsCandidates(t, root)

	for _, id := range companionsRejectedDecodeCaseIDs() {
		candidate := companionsCandidateByID(t, candidates, id)
		outcome, encoded, err := runCompanionsDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runCompanionsDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range companionsRejectedEncodeCaseIDs() {
		candidate := companionsCandidateByID(t, candidates, id)
		outcome, encoded, err := runCompanionsEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runCompanionsEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolCompanionsOracleExpectedFieldMutationFailsComparison pins that
// the recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded, and replacing the encoded
// digest fails comparison against the reviewed wire.
func TestProtocolCompanionsOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := companionsCandidates(t, root)

	decode := companionsCandidateByID(t, candidates, companionsspawnDecodeCaseID)
	produced, _, err := runCompanionsDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runCompanionsDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	// The pitch is the contract: a normalization that published the number
	// instead of the bit string would hide the boundary pitch this vector
	// carries.
	fields["pitch"] = companionsFloatBitsText(1.0)
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated pitch compares equal to the produced outcome")
	}

	encode := companionsCandidateByID(t, candidates, companionsstatesEncodeCaseID)
	producedEncode, _, err := runCompanionsEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runCompanionsEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolCompanionsOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolCompanionsOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := companionsCandidates(t, root)
	manifest := companionsManifest(t, root, candidates)
	staged := companionsScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, companionsCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := companionsObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolCompanionsOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so
// a case cannot claim coverage from its name alone.
func TestProtocolCompanionsOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := companionsCandidates(t, root)
	manifest := companionsManifest(t, root, candidates)
	staged := companionsScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range companionFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: companionsVersion, Operation: "decode"}] = runCompanionsDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range companionFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+companionsVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolCompanionsOracleManifestMergeRegistersCompanionRoutes pins that
// the merged manifest registers all three families' routes and case lists,
// leaves the source revision alone, and records this group's provenance
// sources.
func TestProtocolCompanionsOracleManifestMergeRegistersCompanionRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, companionsSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range companionFamilies() {
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
		for _, want := range companionsServerSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so
	// a re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range companionsCandidates(t, root) {
		if !companionsRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolCompanionsOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolCompanionsOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := companionsCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), companionsSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := companionsCandidatesExport(t, root, candidates, merged)
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

// TestProtocolCompanionsGoEncoderRefusesEmbeddedSpaceName pins that the
// production encoder runs the outbound validator before it writes the name
// slot, so a companion name carrying an embedded space never becomes a
// silently published value.
//
// The Rust side publishes the same bytes silently before this node's surface
// conversion, which is the behavioral red the fallible surface closes; the Go
// encoder has always refused the DTO, and the corpus case records that refusal
// so the two implementations stay pinned to one boundary.
func TestProtocolCompanionsGoEncoderRefusesEmbeddedSpaceName(t *testing.T) {
	wireCodec, err := newCompanionsCodec()
	if err != nil {
		t.Fatalf("newCompanionsCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	spawn := companionsSpawnDTO()
	spawn.Name = "Mira Bell"
	_, _, err = wireCodec.EncodeServer(protocol.StatePlay, spawn)
	if err == nil {
		t.Fatal("the Go encoder published a companion name with an embedded space")
	}
	if !strings.Contains(err.Error(), "invalid companion spawn") {
		t.Fatalf("the Go encoder refused the spaced name with %v", err)
	}
	// The silent wire the unvalidated Rust encoder would have published before
	// this node: the same name bytes behind the length prefix.
	silent := companionsSpacedNameSpawnWire()
	name := "Mira Bell"
	if !bytes.Equal(silent[16:17], []byte{byte(len(name))}) || !bytes.Equal(silent[17:17+len(name)], []byte(name)) {
		t.Fatalf("the silent wire carries %x", silent[16:17+len(name)])
	}
	if len(silent) != 58 {
		t.Fatalf("the silent wire is %d bytes, want 58", len(silent))
	}
}

// TestProtocolCompanionsGoDecoderAppliesExactRecordLength pins the one
// structural boundary this group shares with the Rust consumer: the Go decoder
// answers a payload whose remaining bytes disagree with the declared count with
// its remaining-length message, which both sides classify as a truncation
// rather than a trailing byte.
func TestProtocolCompanionsGoDecoderAppliesExactRecordLength(t *testing.T) {
	wireCodec, err := newCompanionsCodec()
	if err != nil {
		t.Fatalf("newCompanionsCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	wire := companionsStatesWire(0, 1, companionsSingleRecord())
	extra := append(append([]byte(nil), wire...), 0x00)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 18, extra); err == nil {
		t.Fatal("the Go decoder accepted a batch with a trailing byte")
	} else if !strings.Contains(err.Error(), "companion states length does not match count") {
		t.Fatalf("the Go decoder answered the trailing byte with %v", err)
	}

	// A five-record payload exceeds the fixed ceiling before the count bound
	// is observable, so the count boundary is pinned with a short payload.
	shortFive := companionsStatesWire(0, uint32(companionsMaxRecords)+1, companionsSingleRecord())
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 18, shortFive); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "companion state count is outside 1..4") {
		t.Fatalf("the Go decoder answered the short five-record payload with %v", err)
	}
}
