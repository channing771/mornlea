package main

// This file is the passive mob packet producer group: the three Play
// server-to-client families that publish the passive mob bodies one subscriber
// can see (`PassiveSpawn`, `PassiveState` and `PassiveDespawn`) each register a
// decode and an encode route through the shared packet-case runner, so every
// case is executed by the real Go codec rather than restated here.
//
// The decode cases are the Go decoder's own bytes: the canonical payloads, the
// full-record boundary, and one structural or value mutation each. The encode
// cases carry canonical JSON fields and either the reviewed wire the Go encoder
// has to publish or a DTO the outbound validator has to refuse. Every negative
// carries exactly one violation.
//
// The three families answer every boundary with a distinct Go message — the
// count, the exact remaining-length check, the per-record identity, dimension,
// pose, health, grazing and reason messages, and the strict-order rule — so the
// category table classifies each one at the boundary that owns it. The identity
// is the one boundary the Go side refuses through its validator rather than at
// the read, exactly as the hostile group does: `PassiveSpawn.Validate` reports a
// zero ID through its own message, which is the identity boundary the Rust
// checked newtype publishes for the same bytes.
//
// The 64-record wire bound is the protocol budget the decoder accepts. The
// smaller live capacity the authority converges on is a separate concern that
// never reaches this producer, so the full-record cases admit 64 records on all
// three families.

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

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// passiveSpawnFamily is the play passive spawn family this group registers.
	passiveSpawnFamily = "protocol.server.PassiveSpawn"
	// passiveStateFamily is the play passive state batch family this group registers.
	passiveStateFamily = "protocol.server.PassiveState"
	// passiveDespawnFamily is the play passive despawn family this group registers.
	passiveDespawnFamily = "protocol.server.PassiveDespawn"
	// passivesVersion is the protocol version all three families are pinned to.
	passivesVersion = "45"
	// passivesProducerID is the exporter's producer identity for this group's
	// candidate assets and merged manifest.
	passivesProducerID = "runtime-oracle/protocol-passives"

	// passiveSpawnCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	passiveSpawnCorpusRelDir = corpusCasesRelDir + "/protocol/PassiveSpawn"
	// passiveStateCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	passiveStateCorpusRelDir = corpusCasesRelDir + "/protocol/PassiveState"
	// passiveDespawnCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	passiveDespawnCorpusRelDir = corpusCasesRelDir + "/protocol/PassiveDespawn"

	// passiveSpawnWireBytes is the fixed spawn record stride: u64 ID + i32
	// dimension + 3×f32 position + f32 yaw + u8 health = 29. No kind byte is
	// carried, because a passive mob has no category to publish.
	passiveSpawnWireBytes = 29
	// passiveStateWireBytes is the fixed state record stride: u64 ID + 3×f32
	// position + 3×f32 velocity + f32 yaw + u8 health + u8 grazing = 38.
	passiveStateWireBytes = 38
	// passiveDespawnWireBytes is the fixed despawn record stride: u64 ID + u8
	// reason = 9.
	passiveDespawnWireBytes = 9
	// passivesMaxRecords is the passive record budget one batch may carry. It
	// is the protocol budget, not the smaller live capacity the authority
	// converges on.
	passivesMaxRecords = 64
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	passiveSpawnDecodeCaseID             = passiveSpawnFamily + "/" + passivesVersion + "/decode-valid"
	passiveSpawnEncodeCaseID             = passiveSpawnFamily + "/" + passivesVersion + "/encode-valid"
	passiveSpawnFullDecodeCaseID         = passiveSpawnFamily + "/" + passivesVersion + "/decode-count-sixty-four"
	passiveSpawnZeroIDDecodeCaseID       = passiveSpawnFamily + "/" + passivesVersion + "/decode-id-zero"
	passiveSpawnDimensionDecodeCaseID    = passiveSpawnFamily + "/" + passivesVersion + "/decode-dimension-depths"
	passiveSpawnHealthZeroDecodeCaseID   = passiveSpawnFamily + "/" + passivesVersion + "/decode-health-zero"
	passiveSpawnHealthAboveDecodeCaseID  = passiveSpawnFamily + "/" + passivesVersion + "/decode-health-above"
	passiveSpawnHealthAboveEncodeCaseID  = passiveSpawnFamily + "/" + passivesVersion + "/encode-health-above"
	passiveSpawnNaNDecodeCaseID          = passiveSpawnFamily + "/" + passivesVersion + "/decode-nan-position"
	passiveSpawnReversedDecodeCaseID     = passiveSpawnFamily + "/" + passivesVersion + "/decode-reversed-ids"
	passiveSpawnTrailingCaseID           = passiveSpawnFamily + "/" + passivesVersion + "/decode-trailing-byte"
	passiveStateDecodeCaseID             = passiveStateFamily + "/" + passivesVersion + "/decode-valid"
	passiveStateEncodeCaseID             = passiveStateFamily + "/" + passivesVersion + "/encode-valid"
	passiveStateGrazingTwoDecodeCaseID   = passiveStateFamily + "/" + passivesVersion + "/decode-grazing-two"
	passiveStateGrazingTwoEncodeCaseID   = passiveStateFamily + "/" + passivesVersion + "/encode-grazing-two"
	passiveStateNaNVelocityDecodeCaseID  = passiveStateFamily + "/" + passivesVersion + "/decode-nan-velocity"
	passiveStateZeroIDDecodeCaseID       = passiveStateFamily + "/" + passivesVersion + "/decode-id-zero"
	passiveStateDuplicateDecodeCaseID    = passiveStateFamily + "/" + passivesVersion + "/decode-duplicate-ids"
	passiveStateCountAboveDecodeCaseID   = passiveStateFamily + "/" + passivesVersion + "/decode-count-above"
	passiveStateFullDecodeCaseID         = passiveStateFamily + "/" + passivesVersion + "/decode-count-sixty-four"
	passiveStateTrailingCaseID           = passiveStateFamily + "/" + passivesVersion + "/decode-trailing-byte"
	passiveDespawnDecodeCaseID           = passiveDespawnFamily + "/" + passivesVersion + "/decode-valid"
	passiveDespawnEncodeCaseID           = passiveDespawnFamily + "/" + passivesVersion + "/encode-valid"
	passiveDespawnReasonTwoDecodeCaseID  = passiveDespawnFamily + "/" + passivesVersion + "/decode-reason-two"
	passiveDespawnReasonTwoEncodeCaseID  = passiveDespawnFamily + "/" + passivesVersion + "/encode-reason-two"
	passiveDespawnZeroIDDecodeCaseID     = passiveDespawnFamily + "/" + passivesVersion + "/decode-id-zero"
	passiveDespawnDuplicateDecodeCaseID  = passiveDespawnFamily + "/" + passivesVersion + "/decode-duplicate-ids"
	passiveDespawnReversedDecodeCaseID   = passiveDespawnFamily + "/" + passivesVersion + "/decode-reversed-ids"
	passiveDespawnCountAboveDecodeCaseID = passiveDespawnFamily + "/" + passivesVersion + "/decode-count-above"
	passiveDespawnTrailingCaseID         = passiveDespawnFamily + "/" + passivesVersion + "/decode-trailing-byte"
)

// passivesFamilyKeys pins each family's complete packet key. A case names its
// key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs. All three families are
// server-to-client play packets.
var passivesFamilyKeys = map[string]PacketKeySpec{
	passiveSpawnFamily:   {Direction: packetDirectionServer, State: packetStatePlay, ID: 26},
	passiveStateFamily:   {Direction: packetDirectionServer, State: packetStatePlay, ID: 27},
	passiveDespawnFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 28},
}

// passivesFamilies lists the three families in registry order, with the corpus
// directory that holds each family's assets. The order is the review order,
// not a dispatch table.
func passivesFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{passiveSpawnFamily, passiveSpawnCorpusRelDir},
		{passiveStateFamily, passiveStateCorpusRelDir},
		{passiveDespawnFamily, passiveDespawnCorpusRelDir},
	}
}

// passivesU64 renders one little-endian u64 field.
func passivesU64(value uint64) []byte {
	wire := make([]byte, 8)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// passivesI32 renders one little-endian i32 field.
func passivesI32(value int32) []byte {
	wire := make([]byte, 4)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// passivesFloatWire renders one IEEE-754 binary32 field, preserving the exact
// bits so a negative zero survives the wire round trip.
func passivesFloatWire(value float32) []byte {
	wire := make([]byte, 4)
	bits := math.Float32bits(value)
	for index := range wire {
		wire[index] = byte(bits >> (8 * index))
	}
	return wire
}

// passivesNegZero is the negative zero the reviewed vectors carry.
func passivesNegZero() float32 {
	return float32(math.Copysign(0, -1))
}

// passivesBatchHeader renders the tick and the one-byte count prefix.
func passivesBatchHeader(tick uint64, count uint8) []byte {
	wire := append([]byte(nil), passivesU64(tick)...)
	return append(wire, count)
}

// passivesSpawnRecord is one spawn record before it is published.
type passivesSpawnRecord struct {
	id        uint64
	dimension int32
	position  [3]float32
	yaw       float32
	health    uint8
}

// passivesSpawnRecordWire renders one spawn record in the wire field order:
// the identity, the dimension, the position, the yaw and the health. No kind
// byte follows the health, because a passive mob has no category to publish.
func passivesSpawnRecordWire(record passivesSpawnRecord) []byte {
	wire := make([]byte, 0, passiveSpawnWireBytes)
	wire = append(wire, passivesU64(record.id)...)
	wire = append(wire, passivesI32(record.dimension)...)
	for _, value := range record.position {
		wire = append(wire, passivesFloatWire(value)...)
	}
	wire = append(wire, passivesFloatWire(record.yaw)...)
	return append(wire, record.health)
}

// passivesStateRecord is one state record before it is published.
type passivesStateRecord struct {
	id       uint64
	position [3]float32
	velocity [3]float32
	yaw      float32
	health   uint8
	grazing  uint8
}

// passivesStateRecordWire renders one state record in the wire field order: the
// identity, the position, the velocity, the yaw, the health and the grazing
// bit. The dimension is absent from the record entirely.
func passivesStateRecordWire(record passivesStateRecord) []byte {
	wire := make([]byte, 0, passiveStateWireBytes)
	wire = append(wire, passivesU64(record.id)...)
	for _, value := range record.position {
		wire = append(wire, passivesFloatWire(value)...)
	}
	for _, value := range record.velocity {
		wire = append(wire, passivesFloatWire(value)...)
	}
	wire = append(wire, passivesFloatWire(record.yaw)...)
	return append(wire, record.health, record.grazing)
}

// passivesDespawnRecord is one despawn record before it is published.
type passivesDespawnRecord struct {
	id     uint64
	reason uint8
}

// passivesDespawnRecordWire renders one despawn record: the identity and the
// removal reason.
func passivesDespawnRecordWire(record passivesDespawnRecord) []byte {
	wire := make([]byte, 0, passiveDespawnWireBytes)
	wire = append(wire, passivesU64(record.id)...)
	return append(wire, record.reason)
}

// passivesSpawnWire renders one spawn payload: the tick, the count that may
// disagree with the records present, and the ordered records.
func passivesSpawnWire(tick uint64, declared uint8, records []passivesSpawnRecord) []byte {
	wire := append([]byte(nil), passivesBatchHeader(tick, declared)...)
	for _, record := range records {
		wire = append(wire, passivesSpawnRecordWire(record)...)
	}
	return wire
}

// passivesStateWire renders one state payload: the tick, the count that may
// disagree with the records present, and the ordered records.
func passivesStateWire(tick uint64, declared uint8, records []passivesStateRecord) []byte {
	wire := append([]byte(nil), passivesBatchHeader(tick, declared)...)
	for _, record := range records {
		wire = append(wire, passivesStateRecordWire(record)...)
	}
	return wire
}

// passivesDespawnWire renders one despawn payload: the tick, the count that may
// disagree with the records present, and the ordered records.
func passivesDespawnWire(tick uint64, declared uint8, records []passivesDespawnRecord) []byte {
	wire := append([]byte(nil), passivesBatchHeader(tick, declared)...)
	for _, record := range records {
		wire = append(wire, passivesDespawnRecordWire(record)...)
	}
	return wire
}

// passivesAscendingSpawnRecord renders one record of the full spawn batch, so
// the reviewed ceiling vector is built from the same field values as the
// canonical vector.
func passivesAscendingSpawnRecord(id uint64) passivesSpawnRecord {
	return passivesSpawnRecord{
		id:        id,
		dimension: int32(core.Overworld),
		position:  [3]float32{1, 2, 3},
		yaw:       0.5,
		health:    10,
	}
}

// passivesAscendingStateRecord renders one record of the full state batch.
func passivesAscendingStateRecord(id uint64) passivesStateRecord {
	return passivesStateRecord{
		id:       id,
		position: [3]float32{1, 2, 3},
		velocity: [3]float32{0, 0.25, 3},
		yaw:      0.5,
		health:   10,
		grazing:  0,
	}
}

// passivesAscendingDespawnRecord renders one record of the full despawn batch.
func passivesAscendingDespawnRecord(id uint64) passivesDespawnRecord {
	return passivesDespawnRecord{id: id, reason: protocol.PassiveDespawnVanished}
}

// passivesFullSpawnRecords is the reviewed 64-record spawn batch: identities 1
// through 64 in ascending order, which is exactly the fixed record ceiling.
func passivesFullSpawnRecords() []passivesSpawnRecord {
	records := make([]passivesSpawnRecord, 0, passivesMaxRecords)
	for id := uint64(1); id <= passivesMaxRecords; id++ {
		records = append(records, passivesAscendingSpawnRecord(id))
	}
	return records
}

// passivesFullStateRecords is the reviewed 64-record state batch.
func passivesFullStateRecords() []passivesStateRecord {
	records := make([]passivesStateRecord, 0, passivesMaxRecords)
	for id := uint64(1); id <= passivesMaxRecords; id++ {
		records = append(records, passivesAscendingStateRecord(id))
	}
	return records
}

// passivesFullDespawnRecords is the reviewed 64-record despawn batch.
func passivesFullDespawnRecords() []passivesDespawnRecord {
	records := make([]passivesDespawnRecord, 0, passivesMaxRecords)
	for id := uint64(1); id <= passivesMaxRecords; id++ {
		records = append(records, passivesAscendingDespawnRecord(id))
	}
	return records
}

// passivesCanonicalSpawnRecords is the reviewed two-record spawn batch: the
// health boundaries 1 and 20, the overworld dimension, and a first position
// carrying a negative zero.
func passivesCanonicalSpawnRecords() []passivesSpawnRecord {
	return []passivesSpawnRecord{
		{
			id:        1,
			dimension: int32(core.Overworld),
			position:  [3]float32{passivesNegZero(), 1, 2},
			yaw:       0,
			health:    1,
		},
		{
			id:        2,
			dimension: int32(core.Overworld),
			position:  [3]float32{3, 4, 5},
			yaw:       -2.5,
			health:    core.MaxHealth,
		},
	}
}

// passivesCanonicalStateRecords is the reviewed two-record state batch, whose
// first velocity also carries a negative zero and whose grazing bits take both
// published values.
func passivesCanonicalStateRecords() []passivesStateRecord {
	return []passivesStateRecord{
		{
			id:       1,
			position: [3]float32{passivesNegZero(), 1, 2},
			velocity: [3]float32{passivesNegZero(), 0.25, 3},
			yaw:      0.5,
			health:   13,
			grazing:  1,
		},
		{
			id:       2,
			position: [3]float32{3, 4, 5},
			velocity: [3]float32{0.5, -1.25, 0},
			yaw:      -2.5,
			health:   7,
			grazing:  0,
		},
	}
}

// passivesCanonicalDespawnRecords is the reviewed two-record despawn batch:
// both published removal reasons.
func passivesCanonicalDespawnRecords() []passivesDespawnRecord {
	return []passivesDespawnRecord{
		{id: 1, reason: protocol.PassiveDespawnVanished},
		{id: 2, reason: protocol.PassiveDespawnDied},
	}
}

// passivesWithSpawnByte replaces one byte of a spawn payload at offset.
func passivesWithSpawnByte(source []byte, offset int, value uint8) []byte {
	wire := append([]byte(nil), source...)
	wire[offset] = value
	return wire
}

// passivesWithStateByte replaces one byte of a state payload at offset.
func passivesWithStateByte(source []byte, offset int, value uint8) []byte {
	wire := append([]byte(nil), source...)
	wire[offset] = value
	return wire
}

// passivesWithDespawnByte replaces one byte of a despawn payload at offset.
func passivesWithDespawnByte(source []byte, offset int, value uint8) []byte {
	wire := append([]byte(nil), source...)
	wire[offset] = value
	return wire
}

// passivesWithSpawnFloat replaces one binary32 field of a spawn payload at
// offset.
func passivesWithSpawnFloat(source []byte, offset int, value float32) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[offset:offset+4], passivesFloatWire(value))
	return wire
}

// passivesWithStateFloat replaces one binary32 field of a state payload at
// offset.
func passivesWithStateFloat(source []byte, offset int, value float32) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[offset:offset+4], passivesFloatWire(value))
	return wire
}

// passivesFloatBitsText renders one binary32 as its eight-digit
// lowercase-hexadecimal bit string, the canonical float encoding both
// directions publish.
func passivesFloatBitsText(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// passivesIDText renders one u64 identity as its decimal string, so the full
// identity range stays lossless in the normalized fields.
func passivesIDText(value uint64) string {
	return strconv.FormatUint(value, 10)
}

// passivesVectorText renders one three-component vector as its ordered
// bit-string array.
func passivesVectorText(values [3]float32) []string {
	rendered := make([]string, 0, len(values))
	for _, value := range values {
		rendered = append(rendered, passivesFloatBitsText(value))
	}
	return rendered
}

// passivesVec3 renders one mathgl vector as its three ordered components.
func passivesVec3(value mgl32.Vec3) [3]float32 {
	return [3]float32{value.X(), value.Y(), value.Z()}
}

// passivesSpawnRecordFields renders one spawn record's semantic fields.
func passivesSpawnRecordFields(record protocol.PassiveSpawnRecord) map[string]any {
	return map[string]any{
		"id":        passivesIDText(record.ID),
		"dimension": int32(record.Dimension),
		"position":  passivesVectorText(passivesVec3(record.Position)),
		"yaw":       passivesFloatBitsText(record.Yaw),
		"health":    record.Health,
	}
}

// passivesStateRecordFields renders one state record's semantic fields.
func passivesStateRecordFields(record protocol.PassiveStateRecord) map[string]any {
	return map[string]any{
		"id":       passivesIDText(record.ID),
		"position": passivesVectorText(passivesVec3(record.Position)),
		"velocity": passivesVectorText(passivesVec3(record.Velocity)),
		"yaw":      passivesFloatBitsText(record.Yaw),
		"health":   record.Health,
		"grazing":  record.Grazing,
	}
}

// passivesDespawnRecordFields renders one despawn record's semantic fields.
func passivesDespawnRecordFields(record protocol.PassiveDespawnRecord) map[string]any {
	return map[string]any{
		"id":     passivesIDText(record.ID),
		"reason": record.Reason,
	}
}

// passivesSpawnFields renders the semantic fields one spawn publishes.
//
// The tick is a decimal string so the full u64 range stays lossless, the
// records publish in wire order and never sorted.
func passivesSpawnFields(spawn protocol.PassiveSpawn) map[string]any {
	records := make([]map[string]any, 0, len(spawn.Spawns))
	for _, record := range spawn.Spawns {
		records = append(records, passivesSpawnRecordFields(record))
	}
	return map[string]any{
		"server_tick": passivesIDText(spawn.ServerTick),
		"spawns":      records,
	}
}

// passivesStateFields renders the semantic fields one state batch publishes.
func passivesStateFields(state protocol.PassiveState) map[string]any {
	records := make([]map[string]any, 0, len(state.States))
	for _, record := range state.States {
		records = append(records, passivesStateRecordFields(record))
	}
	return map[string]any{
		"server_tick": passivesIDText(state.ServerTick),
		"states":      records,
	}
}

// passivesDespawnFields renders the semantic fields one despawn publishes.
func passivesDespawnFields(despawn protocol.PassiveDespawn) map[string]any {
	records := make([]map[string]any, 0, len(despawn.Despawns))
	for _, record := range despawn.Despawns {
		records = append(records, passivesDespawnRecordFields(record))
	}
	return map[string]any{
		"server_tick": passivesIDText(despawn.ServerTick),
		"despawns":    records,
	}
}

// passivesRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and the
// outbound validators name, never over a sentinel identity, and a failure with
// no mapping is a hard error. The primitive answers precede the validator
// messages, because the decoder rejects a non-finite float while reading and
// only then hands the record to `Validate`.
//
// The message shapes are each their own boundary. The count bound and the
// strict-order rule are value violations; the remaining-length check the Go
// decoder applies before it reads a record is the truncation boundary, which is
// the same boundary the Rust exact-record rule publishes for a padded payload;
// the per-record identity message is the identity boundary; the dimension,
// grazing and reason messages are enum violations, because each names a closed
// wire set; and the pose and health messages are value violations.
func passivesRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "invalid float32"):
		return "invalid-value", true
	case strings.Contains(message, "count is outside 1..64"):
		return "invalid-value", true
	case strings.Contains(message, "length does not match count"):
		return "truncated", true
	case strings.Contains(message, "are not strictly sorted"):
		return "invalid-value", true
	case strings.Contains(message, "ID is zero"):
		return "invalid-identity", true
	case strings.Contains(message, "dimension"):
		return "invalid-enum", true
	case strings.Contains(message, "grazing"):
		return "invalid-enum", true
	case strings.Contains(message, "reason"):
		return "invalid-enum", true
	case strings.Contains(message, "is not finite"):
		return "invalid-value", true
	case strings.Contains(message, "health"):
		return "invalid-value", true
	}
	return "", false
}

// passivesPacketKey resolves one packet key to the Go state and numeric ID the
// codec dispatches on, and reports whether the packet travels server-to-client.
func passivesPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := passivesFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no passive producer owns", c.ID, c.Family)
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

// newPassivesCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newPassivesCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// passivesFieldsForPacket renders the semantic fields of one decoded
// publication packet, dispatching on the concrete DTO the production decoder
// returned.
func passivesFieldsForPacket(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.PassiveSpawn:
		return passivesSpawnFields(message), nil
	case protocol.PassiveState:
		return passivesStateFields(message), nil
	case protocol.PassiveDespawn:
		return passivesDespawnFields(message), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// runPassivesDecode executes one publication decode case through the real Go
// decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` and classifies
// the failure it returns. It never reimplements the count, length, identity or
// ordering rules, so the recorded outcome is whatever the production codec
// decides about these exact bytes.
func runPassivesDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := passivesPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newPassivesCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		category, classified := passivesRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := passivesFieldsForPacket(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// passivesSpawnRecordRequest is the canonical JSON field input one spawn
// record carries.
type passivesSpawnRecordRequest struct {
	ID        uint64   `json:"id"`
	Dimension int32    `json:"dimension"`
	Position  []string `json:"position"`
	Yaw       string   `json:"yaw"`
	Health    uint8    `json:"health"`
}

// core renders the request as the Go spawn record DTO.
func (request passivesSpawnRecordRequest) core() protocol.PassiveSpawnRecord {
	return protocol.PassiveSpawnRecord{
		ID:        request.ID,
		Dimension: core.DimensionID(request.Dimension),
		Position:  mgl32.Vec3(passivesPositionFromText(request.Position)),
		Yaw:       passivesAngleFromText(request.Yaw),
		Health:    request.Health,
	}
}

// passivesStateRecordRequest is the canonical JSON field input one state
// record carries.
type passivesStateRecordRequest struct {
	ID       uint64   `json:"id"`
	Position []string `json:"position"`
	Velocity []string `json:"velocity"`
	Yaw      string   `json:"yaw"`
	Health   uint8    `json:"health"`
	Grazing  uint8    `json:"grazing"`
}

// core renders the request as the Go state record DTO.
func (request passivesStateRecordRequest) core() protocol.PassiveStateRecord {
	return protocol.PassiveStateRecord{
		ID:       request.ID,
		Position: mgl32.Vec3(passivesPositionFromText(request.Position)),
		Velocity: mgl32.Vec3(passivesPositionFromText(request.Velocity)),
		Yaw:      passivesAngleFromText(request.Yaw),
		Health:   request.Health,
		Grazing:  request.Grazing,
	}
}

// passivesDespawnRecordRequest is the canonical JSON field input one despawn
// record carries.
type passivesDespawnRecordRequest struct {
	ID     uint64 `json:"id"`
	Reason uint8  `json:"reason"`
}

// core renders the request as the Go despawn record DTO.
func (request passivesDespawnRecordRequest) core() protocol.PassiveDespawnRecord {
	return protocol.PassiveDespawnRecord{ID: request.ID, Reason: request.Reason}
}

// passivesSpawnRequest is the canonical JSON field input one spawn encode case
// carries.
type passivesSpawnRequest struct {
	ServerTick uint64                       `json:"server_tick"`
	Spawns     []passivesSpawnRecordRequest `json:"spawns"`
}

// core renders the request as the Go spawn DTO the encoder validates.
func (request passivesSpawnRequest) core() protocol.PassiveSpawn {
	spawns := make([]protocol.PassiveSpawnRecord, 0, len(request.Spawns))
	for _, record := range request.Spawns {
		spawns = append(spawns, record.core())
	}
	return protocol.PassiveSpawn{ServerTick: request.ServerTick, Spawns: spawns}
}

// passivesStateRequest is the canonical JSON field input one state encode case
// carries.
type passivesStateRequest struct {
	ServerTick uint64                       `json:"server_tick"`
	States     []passivesStateRecordRequest `json:"states"`
}

// core renders the request as the Go state DTO the encoder validates.
func (request passivesStateRequest) core() protocol.PassiveState {
	states := make([]protocol.PassiveStateRecord, 0, len(request.States))
	for _, record := range request.States {
		states = append(states, record.core())
	}
	return protocol.PassiveState{ServerTick: request.ServerTick, States: states}
}

// passivesDespawnRequest is the canonical JSON field input one despawn encode
// case carries.
type passivesDespawnRequest struct {
	ServerTick uint64                         `json:"server_tick"`
	Despawns   []passivesDespawnRecordRequest `json:"despawns"`
}

// core renders the request as the Go despawn DTO the encoder validates.
func (request passivesDespawnRequest) core() protocol.PassiveDespawn {
	despawns := make([]protocol.PassiveDespawnRecord, 0, len(request.Despawns))
	for _, record := range request.Despawns {
		despawns = append(despawns, record.core())
	}
	return protocol.PassiveDespawn{ServerTick: request.ServerTick, Despawns: despawns}
}

// passivesParseBitsText decodes one eight-digit hexadecimal bit string.
func passivesParseBitsText(text string) (float32, error) {
	bits, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: bits %q are not hexadecimal: %w", text, err)
	}
	return math.Float32frombits(uint32(bits)), nil
}

// passivesPositionFromText decodes one three-element bit-string array.
func passivesPositionFromText(values []string) [3]float32 {
	if len(values) != 3 {
		panic(fmt.Sprintf("runtime-oracle: reviewed position carries %d components", len(values)))
	}
	var parsed [3]float32
	for index, text := range values {
		value, err := passivesParseBitsText(text)
		if err != nil {
			panic("runtime-oracle: reviewed position is not hexadecimal: " + err.Error())
		}
		parsed[index] = value
	}
	return parsed
}

// passivesAngleFromText decodes one bit-string angle field.
func passivesAngleFromText(text string) float32 {
	value, err := passivesParseBitsText(text)
	if err != nil {
		panic("runtime-oracle: reviewed angle is not hexadecimal: " + err.Error())
	}
	return value
}

// passivesBitRequest renders one binary32 as its canonical JSON request text.
func passivesBitRequest(value float32) string {
	return passivesFloatBitsText(value)
}

// passivesSpawnRecordRequestFields renders one spawn record as its canonical
// JSON request fields.
func passivesSpawnRecordRequestFields(record passivesSpawnRecord) passivesSpawnRecordRequest {
	return passivesSpawnRecordRequest{
		ID:        record.id,
		Dimension: record.dimension,
		Position: []string{
			passivesBitRequest(record.position[0]),
			passivesBitRequest(record.position[1]),
			passivesBitRequest(record.position[2]),
		},
		Yaw:    passivesBitRequest(record.yaw),
		Health: record.health,
	}
}

// passivesStateRecordRequestFields renders one state record as its canonical
// JSON request fields.
func passivesStateRecordRequestFields(record passivesStateRecord) passivesStateRecordRequest {
	return passivesStateRecordRequest{
		ID: record.id,
		Position: []string{
			passivesBitRequest(record.position[0]),
			passivesBitRequest(record.position[1]),
			passivesBitRequest(record.position[2]),
		},
		Velocity: []string{
			passivesBitRequest(record.velocity[0]),
			passivesBitRequest(record.velocity[1]),
			passivesBitRequest(record.velocity[2]),
		},
		Yaw:     passivesBitRequest(record.yaw),
		Health:  record.health,
		Grazing: record.grazing,
	}
}

// passivesDespawnRecordRequestFields renders one despawn record as its
// canonical JSON request fields.
func passivesDespawnRecordRequestFields(record passivesDespawnRecord) passivesDespawnRecordRequest {
	return passivesDespawnRecordRequest{ID: record.id, Reason: record.reason}
}

// passivesSpawnRequestFields is the canonical JSON request the valid spawn
// encode case carries, which is the reviewed wire's own field set.
func passivesSpawnRequestFields() passivesSpawnRequest {
	records := passivesCanonicalSpawnRecords()
	spawns := make([]passivesSpawnRecordRequest, 0, len(records))
	for _, record := range records {
		spawns = append(spawns, passivesSpawnRecordRequestFields(record))
	}
	return passivesSpawnRequest{ServerTick: 0, Spawns: spawns}
}

// passivesStateRequestFields is the canonical JSON request the valid state
// encode case carries.
func passivesStateRequestFields() passivesStateRequest {
	records := passivesCanonicalStateRecords()
	states := make([]passivesStateRecordRequest, 0, len(records))
	for _, record := range records {
		states = append(states, passivesStateRecordRequestFields(record))
	}
	return passivesStateRequest{ServerTick: 0, States: states}
}

// passivesDespawnRequestFields is the canonical JSON request the valid despawn
// encode case carries.
func passivesDespawnRequestFields() passivesDespawnRequest {
	records := passivesCanonicalDespawnRecords()
	despawns := make([]passivesDespawnRecordRequest, 0, len(records))
	for _, record := range records {
		despawns = append(despawns, passivesDespawnRecordRequestFields(record))
	}
	return passivesDespawnRequest{ServerTick: 0, Despawns: despawns}
}

// passivesHealthAboveSpawnRequest is the invalid encode request whose only
// violation is the health span: the reviewed record with health 21.
func passivesHealthAboveSpawnRequest() passivesSpawnRequest {
	request := passivesSpawnRequestFields()
	records := passivesCanonicalSpawnRecords()
	over := records[0]
	over.health = core.MaxHealth + 1
	request.Spawns[0] = passivesSpawnRecordRequestFields(over)
	return request
}

// passivesGrazingTwoStateRequest is the invalid encode request whose only
// violation is the closed grazing pair: the reviewed record with grazing 2.
func passivesGrazingTwoStateRequest() passivesStateRequest {
	request := passivesStateRequestFields()
	records := passivesCanonicalStateRecords()
	over := records[0]
	over.grazing = 2
	request.States[0] = passivesStateRecordRequestFields(over)
	return request
}

// passivesReasonTwoDespawnRequest is the invalid encode request whose only
// violation is the closed reason pair: the reviewed record with reason 2.
func passivesReasonTwoDespawnRequest() passivesDespawnRequest {
	request := passivesDespawnRequestFields()
	over := passivesCanonicalDespawnRecords()[0]
	over.reason = 2
	request.Despawns[0] = passivesDespawnRecordRequestFields(over)
	return request
}

// runPassivesEncode executes one publication encode case through the real Go
// encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects must
// never be recorded as evidence.
func runPassivesEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := passivesPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newPassivesCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ServerPacket
	switch c.Family {
	case passiveSpawnFamily:
		var request passivesSpawnRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	case passiveStateFamily:
		var request passivesStateRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	case passiveDespawnFamily:
		var request passivesDespawnRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	encodedID, payload, err := wireCodec.EncodeServer(state, packet)
	if err != nil {
		category, classified := passivesRejectionCategory(err)
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
	fields, err := passivesFieldsForPacket(c, decoded)
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

// passivesCorpusRoutes is the closed route map the three families execute.
func passivesCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range passivesFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: passivesVersion, Operation: "decode"}] = runPassivesDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: passivesVersion, Operation: "encode"}] = runPassivesEncode
	}
	return routes
}

// passivesCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer result.
type passivesCaseDefinition struct {
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

// passivesFullSpawnOutcome is the reviewed 64-record spawn batch outcome.
func passivesFullSpawnOutcome() Outcome {
	records := passivesFullSpawnRecords()
	spawns := make([]protocol.PassiveSpawnRecord, 0, len(records))
	for _, record := range records {
		spawns = append(spawns, protocol.PassiveSpawnRecord{
			ID:        record.id,
			Dimension: core.Overworld,
			Position:  record.position,
			Yaw:       record.yaw,
			Health:    record.health,
		})
	}
	return Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   passivesSpawnFields(protocol.PassiveSpawn{ServerTick: 0, Spawns: spawns}),
	}
}

// passivesFullStateOutcome is the reviewed 64-record state batch outcome.
func passivesFullStateOutcome() Outcome {
	records := passivesFullStateRecords()
	states := make([]protocol.PassiveStateRecord, 0, len(records))
	for _, record := range records {
		states = append(states, protocol.PassiveStateRecord{
			ID:       record.id,
			Position: record.position,
			Velocity: record.velocity,
			Yaw:      record.yaw,
			Health:   record.health,
			Grazing:  record.grazing,
		})
	}
	return Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   passivesStateFields(protocol.PassiveState{ServerTick: 0, States: states}),
	}
}

// passivesFullDespawnOutcome is the reviewed 64-record despawn batch outcome.
func passivesFullDespawnOutcome() Outcome {
	records := passivesFullDespawnRecords()
	despawns := make([]protocol.PassiveDespawnRecord, 0, len(records))
	for _, record := range records {
		despawns = append(despawns, protocol.PassiveDespawnRecord{ID: record.id, Reason: record.reason})
	}
	return Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   passivesDespawnFields(protocol.PassiveDespawn{ServerTick: 0, Despawns: despawns}),
	}
}

// passivesCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid cases
// use the canonical payloads the Go encoder produces for these fields, including
// the 64-record boundary of each family; the malformed decode cases mutate that
// payload at one boundary each, and the invalid encode cases build DTOs the
// production validator refuses, so each rejection names the boundary that owns
// it.
//
// The passive messages are distinct per boundary, so no boundary is latent: the
// count, the exact remaining-length check, the per-record identity, dimension,
// pose, health, grazing and reason messages, and the two strict-order messages
// each name their own condition. No case combines two violations, and no case
// names a doubly invalid record — every negative record carries exactly one
// violation beside its otherwise valid fields. Neither family carries a
// kind-byte case, because a passive mob has no category to publish.
func passivesCaseDefinitions() []passivesCaseDefinition {
	validSpawn := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   passivesSpawnFields(passivesSpawnRequestFields().core()),
	}
	validState := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   passivesStateFields(passivesStateRequestFields().core()),
	}
	validDespawn := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   passivesDespawnFields(passivesDespawnRequestFields().core()),
	}
	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	invalidEnum := Outcome{Kind: "error", Category: "invalid-enum"}
	invalidIdentity := Outcome{Kind: "error", Category: "invalid-identity"}
	truncated := Outcome{Kind: "error", Category: "truncated"}

	spawnWire := passivesSpawnWire(0, 2, passivesCanonicalSpawnRecords())
	stateWire := passivesStateWire(0, 2, passivesCanonicalStateRecords())
	despawnWire := passivesDespawnWire(0, 2, passivesCanonicalDespawnRecords())
	fullSpawnWire := passivesSpawnWire(0, passivesMaxRecords, passivesFullSpawnRecords())
	fullStateWire := passivesStateWire(0, passivesMaxRecords, passivesFullStateRecords())

	// One spawn record is the tick, the count prefix and the fixed 29-byte
	// stride, so the record fields sit at fixed offsets inside it. No kind
	// byte closes the record: the health byte is the last field.
	const spawnRecordBase = 9
	const spawnPositionOffset = spawnRecordBase + 8 + 4
	const spawnHealthOffset = spawnRecordBase + passiveSpawnWireBytes - 1
	// One state record is the tick, the count prefix and the fixed 38-byte
	// stride, and its velocity precedes the yaw.
	const stateRecordBase = 9
	const statePositionOffset = stateRecordBase + 8
	const stateVelocityOffset = statePositionOffset + 12
	const stateHealthOffset = stateRecordBase + passiveStateWireBytes - 2
	const stateGrazingOffset = stateRecordBase + passiveStateWireBytes - 1
	// One despawn record is the tick, the count prefix and the fixed 9-byte
	// stride, and the reason byte closes it.
	const despawnRecordBase = 9
	const despawnReasonOffset = despawnRecordBase + passiveDespawnWireBytes - 1

	definitions := make([]passivesCaseDefinition, 0, 30)
	definitions = append(definitions,
		passivesCaseDefinition{
			id:     passiveSpawnDecodeCaseID,
			family: passiveSpawnFamily,
			relDir: passiveSpawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), spawnWire...),
			expect: validSpawn,
		},
		passivesCaseDefinition{
			id:      passiveSpawnEncodeCaseID,
			family:  passiveSpawnFamily,
			relDir:  passiveSpawnCorpusRelDir,
			op:      "encode",
			request: passivesSpawnRequestFields(),
			wire:    append([]byte(nil), spawnWire...),
			expect:  validSpawn,
		},
		passivesCaseDefinition{
			// Sixty-four records is the count and record ceiling, admitted
			// exactly at the fixed payload bound. The smaller live capacity
			// the authority converges on never enters this packet layer.
			id:     passiveSpawnFullDecodeCaseID,
			family: passiveSpawnFamily,
			relDir: passiveSpawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), fullSpawnWire...),
			expect: passivesFullSpawnOutcome(),
		},
		passivesCaseDefinition{
			// The zero identity is the identity boundary, which the Go
			// validator answers with its own message.
			id:     passiveSpawnZeroIDDecodeCaseID,
			family: passiveSpawnFamily,
			relDir: passiveSpawnCorpusRelDir,
			op:     "decode",
			input: passivesSpawnWire(0, 1, []passivesSpawnRecord{
				func() passivesSpawnRecord {
					record := passivesCanonicalSpawnRecords()[0]
					record.id = 0
					return record
				}(),
			}),
			expect: invalidIdentity,
		},
		passivesCaseDefinition{
			// The depths dimension is the single violation; the identity,
			// pose and health stay valid.
			id:     passiveSpawnDimensionDecodeCaseID,
			family: passiveSpawnFamily,
			relDir: passiveSpawnCorpusRelDir,
			op:     "decode",
			input: passivesSpawnWire(0, 1, []passivesSpawnRecord{
				func() passivesSpawnRecord {
					record := passivesCanonicalSpawnRecords()[0]
					record.dimension = int32(core.Depths)
					return record
				}(),
			}),
			expect: invalidEnum,
		},
		passivesCaseDefinition{
			id:     passiveSpawnHealthZeroDecodeCaseID,
			family: passiveSpawnFamily,
			relDir: passiveSpawnCorpusRelDir,
			op:     "decode",
			input:  passivesWithSpawnByte(spawnWire, spawnHealthOffset, 0),
			expect: invalidValue,
		},
		passivesCaseDefinition{
			// Twenty-one is one past the health maximum the Go core pins.
			id:     passiveSpawnHealthAboveDecodeCaseID,
			family: passiveSpawnFamily,
			relDir: passiveSpawnCorpusRelDir,
			op:     "decode",
			input:  passivesWithSpawnByte(spawnWire, spawnHealthOffset, 21),
			expect: invalidValue,
		},
		passivesCaseDefinition{
			// The encode twin of the health refusal: the same reviewed record
			// with health 21 as a DTO, which the outbound validator refuses.
			id:      passiveSpawnHealthAboveEncodeCaseID,
			family:  passiveSpawnFamily,
			relDir:  passiveSpawnCorpusRelDir,
			op:      "encode",
			request: passivesHealthAboveSpawnRequest(),
			expect:  invalidValue,
		},
		passivesCaseDefinition{
			// One NaN position word is answered by the decoder's float
			// primitive before the validator runs.
			id:     passiveSpawnNaNDecodeCaseID,
			family: passiveSpawnFamily,
			relDir: passiveSpawnCorpusRelDir,
			op:     "decode",
			input:  passivesWithSpawnFloat(spawnWire, spawnPositionOffset, float32(math.NaN())),
			expect: invalidValue,
		},
		passivesCaseDefinition{
			id:     passiveSpawnReversedDecodeCaseID,
			family: passiveSpawnFamily,
			relDir: passiveSpawnCorpusRelDir,
			op:     "decode",
			input:  passivesSpawnWire(0, 2, []passivesSpawnRecord{passivesCanonicalSpawnRecords()[1], passivesCanonicalSpawnRecords()[0]}),
			expect: invalidValue,
		},
		passivesCaseDefinition{
			// The Go decoder applies the exact remaining-length rule, so a
			// one-byte-longer payload is answered at the truncation boundary
			// rather than by its end-of-payload check.
			id:     passiveSpawnTrailingCaseID,
			family: passiveSpawnFamily,
			relDir: passiveSpawnCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), spawnWire...), 0x00),
			expect: truncated,
		},
		passivesCaseDefinition{
			id:     passiveStateDecodeCaseID,
			family: passiveStateFamily,
			relDir: passiveStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), stateWire...),
			expect: validState,
		},
		passivesCaseDefinition{
			id:      passiveStateEncodeCaseID,
			family:  passiveStateFamily,
			relDir:  passiveStateCorpusRelDir,
			op:      "encode",
			request: passivesStateRequestFields(),
			wire:    append([]byte(nil), stateWire...),
			expect:  validState,
		},
		passivesCaseDefinition{
			// Grazing 2 is outside the closed 0/1 pair the wire publishes.
			id:     passiveStateGrazingTwoDecodeCaseID,
			family: passiveStateFamily,
			relDir: passiveStateCorpusRelDir,
			op:     "decode",
			input:  passivesWithStateByte(stateWire, stateGrazingOffset, 2),
			expect: invalidEnum,
		},
		passivesCaseDefinition{
			// The encode twin of the grazing refusal: the same reviewed record
			// with grazing 2 as a DTO, which the outbound validator refuses.
			id:      passiveStateGrazingTwoEncodeCaseID,
			family:  passiveStateFamily,
			relDir:  passiveStateCorpusRelDir,
			op:      "encode",
			request: passivesGrazingTwoStateRequest(),
			expect:  invalidEnum,
		},
		passivesCaseDefinition{
			// One NaN velocity word is answered by the float primitive.
			id:     passiveStateNaNVelocityDecodeCaseID,
			family: passiveStateFamily,
			relDir: passiveStateCorpusRelDir,
			op:     "decode",
			input:  passivesWithStateFloat(stateWire, stateVelocityOffset, float32(math.NaN())),
			expect: invalidValue,
		},
		passivesCaseDefinition{
			id:     passiveStateZeroIDDecodeCaseID,
			family: passiveStateFamily,
			relDir: passiveStateCorpusRelDir,
			op:     "decode",
			input: passivesStateWire(0, 1, []passivesStateRecord{
				func() passivesStateRecord {
					record := passivesCanonicalStateRecords()[0]
					record.id = 0
					return record
				}(),
			}),
			expect: invalidIdentity,
		},
		passivesCaseDefinition{
			id:     passiveStateDuplicateDecodeCaseID,
			family: passiveStateFamily,
			relDir: passiveStateCorpusRelDir,
			op:     "decode",
			input:  passivesStateWire(0, 2, []passivesStateRecord{passivesCanonicalStateRecords()[0], passivesCanonicalStateRecords()[0]}),
			expect: invalidValue,
		},
		passivesCaseDefinition{
			// A tiny payload whose declared count is above the ceiling: the
			// count bound fires before the record-length rule on both sides.
			id:     passiveStateCountAboveDecodeCaseID,
			family: passiveStateFamily,
			relDir: passiveStateCorpusRelDir,
			op:     "decode",
			input:  passivesStateWire(0, 65, []passivesStateRecord{passivesCanonicalStateRecords()[0]}),
			expect: invalidValue,
		},
		passivesCaseDefinition{
			id:     passiveStateFullDecodeCaseID,
			family: passiveStateFamily,
			relDir: passiveStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), fullStateWire...),
			expect: passivesFullStateOutcome(),
		},
		passivesCaseDefinition{
			id:     passiveStateTrailingCaseID,
			family: passiveStateFamily,
			relDir: passiveStateCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), stateWire...), 0x00),
			expect: truncated,
		},
		passivesCaseDefinition{
			id:     passiveDespawnDecodeCaseID,
			family: passiveDespawnFamily,
			relDir: passiveDespawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), despawnWire...),
			expect: validDespawn,
		},
		passivesCaseDefinition{
			id:      passiveDespawnEncodeCaseID,
			family:  passiveDespawnFamily,
			relDir:  passiveDespawnCorpusRelDir,
			op:      "encode",
			request: passivesDespawnRequestFields(),
			wire:    append([]byte(nil), despawnWire...),
			expect:  validDespawn,
		},
		passivesCaseDefinition{
			// Reason 2 is outside the closed vanished/died pair.
			id:     passiveDespawnReasonTwoDecodeCaseID,
			family: passiveDespawnFamily,
			relDir: passiveDespawnCorpusRelDir,
			op:     "decode",
			input:  passivesWithDespawnByte(despawnWire, despawnReasonOffset, 2),
			expect: invalidEnum,
		},
		passivesCaseDefinition{
			// The encode twin of the reason refusal: the same reviewed record
			// with reason 2 as a DTO, which the outbound validator refuses.
			id:      passiveDespawnReasonTwoEncodeCaseID,
			family:  passiveDespawnFamily,
			relDir:  passiveDespawnCorpusRelDir,
			op:      "encode",
			request: passivesReasonTwoDespawnRequest(),
			expect:  invalidEnum,
		},
		passivesCaseDefinition{
			id:     passiveDespawnZeroIDDecodeCaseID,
			family: passiveDespawnFamily,
			relDir: passiveDespawnCorpusRelDir,
			op:     "decode",
			input:  passivesDespawnWire(0, 1, []passivesDespawnRecord{{id: 0, reason: protocol.PassiveDespawnVanished}}),
			expect: invalidIdentity,
		},
		passivesCaseDefinition{
			id:     passiveDespawnDuplicateDecodeCaseID,
			family: passiveDespawnFamily,
			relDir: passiveDespawnCorpusRelDir,
			op:     "decode",
			input:  passivesDespawnWire(0, 2, []passivesDespawnRecord{{id: 1, reason: protocol.PassiveDespawnVanished}, {id: 1, reason: protocol.PassiveDespawnDied}}),
			expect: invalidValue,
		},
		passivesCaseDefinition{
			id:     passiveDespawnReversedDecodeCaseID,
			family: passiveDespawnFamily,
			relDir: passiveDespawnCorpusRelDir,
			op:     "decode",
			input:  passivesDespawnWire(0, 2, []passivesDespawnRecord{{id: 2, reason: protocol.PassiveDespawnVanished}, {id: 1, reason: protocol.PassiveDespawnDied}}),
			expect: invalidValue,
		},
		passivesCaseDefinition{
			id:     passiveDespawnCountAboveDecodeCaseID,
			family: passiveDespawnFamily,
			relDir: passiveDespawnCorpusRelDir,
			op:     "decode",
			input:  passivesDespawnWire(0, 65, []passivesDespawnRecord{{id: 1, reason: protocol.PassiveDespawnVanished}}),
			expect: invalidValue,
		},
		passivesCaseDefinition{
			id:     passiveDespawnTrailingCaseID,
			family: passiveDespawnFamily,
			relDir: passiveDespawnCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), despawnWire...), 0x00),
			expect: truncated,
		},
	)

	return definitions
}

// buildPassivesCandidate builds one case's manifest entry and asset bytes from
// its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildPassivesCandidate(t *testing.T, definition passivesCaseDefinition) passivesCandidate {
	t.Helper()

	label := passivesLabel(definition.id)
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
		Version:      passivesVersion,
		Operation:    definition.op,
		PacketKey:    passivesKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return passivesCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// passivesKeyPointer resolves one family's reviewed packet key for a case spec.
func passivesKeyPointer(family string) *PacketKeySpec {
	key, owned := passivesFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// passivesCandidate is one reviewed case: its manifest specification, the exact
// asset bytes it publishes, and the expectation an independent execution has to
// reproduce.
type passivesCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which
	// the producer's own bytes are compared against.
	Wire []byte
}

// passivesCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func passivesCandidates(t *testing.T, root string) []passivesCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := passivesCaseDefinitions()
	candidates := make([]passivesCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildPassivesCandidate(t, definition)
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

// passivesRegisteredCase reports whether the base manifest already carries one
// case identity, so a re-merge of an integrated candidate adds nothing.
func passivesRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// passivesSelection is this group's registration: its cases, the Go sources its
// rules are read from, and the routes the three families execute.
func passivesSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := passivesCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range passivesFamilies() {
		sources[family.id] = passivesServerSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(passivesFamilies())*2)
	for _, family := range passivesFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: passivesVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: passivesVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  passivesProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// passivesServerSources lists the Go sources each family reads its rules from:
// the shared server codec dispatch with its decode arms and the payload
// ceiling, the message file that owns the DTO validators, and the family's own
// wire codec file.
func passivesServerSources(family string) []string {
	return []string{
		"packages/shared/network/codec/codec_server.go",
		"packages/shared/network/protocol/message_passive.go",
		"packages/shared/network/codec/passive_wire.go",
	}
}

// passivesManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func passivesManifest(t *testing.T, root string, candidates []passivesCandidate) Inventory {
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
		if !passivesOwnsFamily(family) {
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
		t.Fatalf("encode passives working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write passives working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load passives working manifest: %v", err)
	}
	return loaded
}

// passivesOwnsFamily reports whether this group registers cases for one family.
func passivesOwnsFamily(family string) bool {
	_, owned := passivesFamilyKeys[family]
	return owned
}

// passivesScratchRoot stages this group's candidate assets in a harness-owned
// temporary directory, because a corpus case has to resolve under the root the
// runner is given.
func passivesScratchRoot(t *testing.T, root string, candidates []passivesCandidate) string {
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

// passivesCandidateByID resolves one candidate by its case identity.
func passivesCandidateByID(t *testing.T, candidates []passivesCandidate, id string) passivesCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return passivesCandidate{}
}

// passivesObservation resolves one executed observation by its case identity.
func passivesObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// passivesExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var passivesExportPublished bool

// passivesCandidatesExport publishes the reviewed candidates and the complete
// merged manifest candidate through the existing external exporter and returns
// the published producer directory. An unset export variable publishes nothing
// and returns "", so an ordinary test run never writes outside its own
// temporary storage.
func passivesCandidatesExport(t *testing.T, root string, candidates []passivesCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if passivesExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-passives")
	}
	passivesExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, passivesProducerID, assets)
	if err != nil {
		t.Fatalf("export passives candidates: %v", err)
	}
	return published
}

// passivesDecodeCaseIDs lists the valid decode cases the producer pins.
func passivesDecodeCaseIDs() []string {
	return []string{
		passiveSpawnDecodeCaseID,
		passiveSpawnFullDecodeCaseID,
		passiveStateDecodeCaseID,
		passiveStateFullDecodeCaseID,
		passiveDespawnDecodeCaseID,
	}
}

// passivesEncodeCaseIDs lists the valid encode cases the producer pins.
func passivesEncodeCaseIDs() []string {
	return []string{
		passiveSpawnEncodeCaseID,
		passiveStateEncodeCaseID,
		passiveDespawnEncodeCaseID,
	}
}

// passivesRejectedDecodeCaseIDs lists the malformed decode cases.
func passivesRejectedDecodeCaseIDs() []string {
	return []string{
		passiveSpawnZeroIDDecodeCaseID,
		passiveSpawnDimensionDecodeCaseID,
		passiveSpawnHealthZeroDecodeCaseID,
		passiveSpawnHealthAboveDecodeCaseID,
		passiveSpawnNaNDecodeCaseID,
		passiveSpawnReversedDecodeCaseID,
		passiveSpawnTrailingCaseID,
		passiveStateGrazingTwoDecodeCaseID,
		passiveStateNaNVelocityDecodeCaseID,
		passiveStateZeroIDDecodeCaseID,
		passiveStateDuplicateDecodeCaseID,
		passiveStateCountAboveDecodeCaseID,
		passiveStateTrailingCaseID,
		passiveDespawnReasonTwoDecodeCaseID,
		passiveDespawnZeroIDDecodeCaseID,
		passiveDespawnDuplicateDecodeCaseID,
		passiveDespawnReversedDecodeCaseID,
		passiveDespawnCountAboveDecodeCaseID,
		passiveDespawnTrailingCaseID,
	}
}

// passivesRejectedEncodeCaseIDs lists the invalid encode cases.
func passivesRejectedEncodeCaseIDs() []string {
	return []string{
		passiveSpawnHealthAboveEncodeCaseID,
		passiveStateGrazingTwoEncodeCaseID,
		passiveDespawnReasonTwoEncodeCaseID,
	}
}

// passivesLabel renders one case's asset label from its identity.
func passivesLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// TestProtocolPassivesOracleDecodesEveryValidCase pins that the real Go decoder
// publishes the canonical fields for every family's valid decode case,
// including the full 64-record boundary.
func TestProtocolPassivesOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := passivesCandidates(t, root)

	for _, id := range passivesDecodeCaseIDs() {
		candidate := passivesCandidateByID(t, candidates, id)
		outcome, encoded, err := runPassivesDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runPassivesDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolPassivesOracleEncodesCanonicalWire pins each encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolPassivesOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := passivesCandidates(t, root)

	for _, id := range passivesEncodeCaseIDs() {
		candidate := passivesCandidateByID(t, candidates, id)
		outcome, encoded, err := runPassivesEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runPassivesEncode(%s): %v", id, err)
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

// TestProtocolPassivesOracleRejectsMalformedCasesAtTheirBoundary pins that
// every malformed decode case and the invalid encode cases are refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolPassivesOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := passivesCandidates(t, root)

	for _, id := range passivesRejectedDecodeCaseIDs() {
		candidate := passivesCandidateByID(t, candidates, id)
		outcome, encoded, err := runPassivesDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runPassivesDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range passivesRejectedEncodeCaseIDs() {
		candidate := passivesCandidateByID(t, candidates, id)
		outcome, encoded, err := runPassivesEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runPassivesEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolPassivesOracleExpectedFieldMutationFailsComparison pins that the
// recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded.
func TestProtocolPassivesOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := passivesCandidates(t, root)

	decode := passivesCandidateByID(t, candidates, passiveStateDecodeCaseID)
	produced, _, err := runPassivesDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runPassivesDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	// The grazing bit is the contract: it is the field the state record closes
	// with, so a normalization that dropped it would hide the wire-valid
	// grazing this vector carries.
	records := fields["states"].([]map[string]any)
	record := records[0]
	record["grazing"] = 0
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated grazing compares equal to the produced outcome")
	}

	encode := passivesCandidateByID(t, candidates, passiveStateEncodeCaseID)
	producedEncode, _, err := runPassivesEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runPassivesEncode: %v", err)
	}
	mutatedEncode := encode.Expect
	mutatedEncode.Fields = map[string]any{"server_tick": "1"}
	if outcomesEqual(mutatedEncode, producedEncode) {
		t.Fatal("mutated encode fields compare equal to the produced outcome")
	}
}

// TestProtocolPassivesOracleRoutesExecuteEveryCase pins that every registered
// case is executed exactly once through its own route.
func TestProtocolPassivesOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := passivesCandidates(t, root)
	manifest := passivesManifest(t, root, candidates)
	staged := passivesScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, passivesCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := passivesObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolPassivesOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so
// a case cannot claim coverage from its name alone.
func TestProtocolPassivesOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := passivesCandidates(t, root)
	manifest := passivesManifest(t, root, candidates)
	staged := passivesScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range passivesFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: passivesVersion, Operation: "decode"}] = runPassivesDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range passivesFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+passivesVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolPassivesOracleManifestMergeRegistersPassiveRoutes pins that the
// merged manifest registers all three families' routes and case lists, leaves
// the source revision alone, and records this group's provenance sources.
func TestProtocolPassivesOracleManifestMergeRegistersPassiveRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, passivesSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range passivesFamilies() {
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
		for _, want := range passivesServerSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so
	// a re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range passivesCandidates(t, root) {
		if !passivesRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolPassivesOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolPassivesOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := passivesCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), passivesSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := passivesCandidatesExport(t, root, candidates, merged)
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
	if reloaded.SourceRevision != merged.SourceRevision {
		t.Fatalf("reloaded source_revision = %s, want %s", reloaded.SourceRevision, merged.SourceRevision)
	}
	for _, family := range passivesFamilies() {
		var row *Family
		for index := range reloaded.Families {
			if reloaded.Families[index].ID == family.id {
				row = &reloaded.Families[index]
				break
			}
		}
		if row == nil || len(row.Cases) == 0 {
			t.Fatalf("reloaded manifest carries no %s case", family.id)
		}
	}
}

// TestProtocolPassivesGoDecoderAppliesExactRecordLength pins the rule this
// node's trailing-byte category is derived from: the Go decoder rejects a
// payload whose remaining length is not exactly `count` records before it reads
// one, so a one-byte-longer payload is answered at the truncation boundary
// rather than by a trailing-byte check. This is the opposite of the item drop
// batch, whose Go decoder applies a minimum-record budget and answers the same
// extra byte as trailing bytes.
func TestProtocolPassivesGoDecoderAppliesExactRecordLength(t *testing.T) {
	wireCodec, err := newPassivesCodec()
	if err != nil {
		t.Fatalf("newPassivesCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	for _, family := range []struct {
		id   uint32
		wire []byte
	}{
		{26, passivesSpawnWire(0, 1, passivesCanonicalSpawnRecords()[:1])},
		{27, passivesStateWire(0, 1, passivesCanonicalStateRecords()[:1])},
		{28, passivesDespawnWire(0, 1, passivesCanonicalDespawnRecords()[:1])},
	} {
		padded := append(append([]byte(nil), family.wire...), 0x00)
		if _, err := wireCodec.DecodeServer(protocol.StatePlay, family.id, padded); err == nil {
			t.Fatalf("packet %d: the Go decoder accepted a padded batch", family.id)
		} else if !strings.Contains(err.Error(), "length does not match count") {
			t.Fatalf("packet %d: the Go decoder answered the padded batch with %v", family.id, err)
		}
		truncated := family.wire[:len(family.wire)-1]
		if _, err := wireCodec.DecodeServer(protocol.StatePlay, family.id, truncated); err == nil {
			t.Fatalf("packet %d: the Go decoder accepted a short batch", family.id)
		} else if !strings.Contains(err.Error(), "length does not match count") {
			t.Fatalf("packet %d: the Go decoder answered the short batch with %v", family.id, err)
		}
	}

	// The count bound fires before the remaining-length rule, so a declared
	// count above the ceiling is refused at the count boundary even when the
	// payload cannot back it.
	overSpawn := append(append([]byte(nil), passivesBatchHeader(0, 65)...), passivesSpawnRecordWire(passivesAscendingSpawnRecord(1))...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 26, overSpawn); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "passive spawn count is outside 1..64") {
		t.Fatalf("the Go decoder answered the over-count spawn payload with %v", err)
	}
	overState := append(append([]byte(nil), passivesBatchHeader(0, 65)...), passivesStateRecordWire(passivesAscendingStateRecord(1))...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 27, overState); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "passive state count is outside 1..64") {
		t.Fatalf("the Go decoder answered the over-count state payload with %v", err)
	}
	overDespawn := append(append([]byte(nil), passivesBatchHeader(0, 65)...), passivesDespawnRecordWire(passivesAscendingDespawnRecord(1))...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 28, overDespawn); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "passive despawn count is outside 1..64") {
		t.Fatalf("the Go decoder answered the over-count despawn payload with %v", err)
	}

	// The zero-collection refusal precedes every record rule, so the empty
	// batch is refused at the count bound.
	emptySpawn := append([]byte(nil), passivesBatchHeader(0, 0)...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 26, emptySpawn); err == nil {
		t.Fatal("the Go decoder accepted an empty batch")
	} else if !strings.Contains(err.Error(), "passive spawn count is outside 1..64") {
		t.Fatalf("the Go decoder answered the empty batch with %v", err)
	}
}

// TestProtocolPassivesGoEncoderRefusesClosedPairBytes pins that the production
// encoder runs the outbound validator before it writes the record, so a passive
// naming an unknown grazing or reason byte never becomes a silently published
// value.
//
// The Rust side published those bytes silently before this node's surface
// conversion, which is the behavioral red the fallible surface closes; the Go
// encoder has always refused the DTO, and the corpus cases record that refusal
// so the two implementations stay pinned to one boundary.
func TestProtocolPassivesGoEncoderRefusesClosedPairBytes(t *testing.T) {
	wireCodec, err := newPassivesCodec()
	if err != nil {
		t.Fatalf("newPassivesCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	request := passivesGrazingTwoStateRequest()
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, request.core()); err == nil {
		t.Fatal("the Go encoder published an unknown grazing byte")
	} else if !strings.Contains(err.Error(), "passive state grazing 2 is invalid") {
		t.Fatalf("the Go encoder refused the unknown grazing with %v", err)
	}

	reasonRequest := passivesReasonTwoDespawnRequest()
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, reasonRequest.core()); err == nil {
		t.Fatal("the Go encoder published an unknown despawn reason")
	} else if !strings.Contains(err.Error(), "passive despawn reason 2 is invalid") {
		t.Fatalf("the Go encoder refused the unknown reason with %v", err)
	}

	// The health span is refused on the outbound surface too, which is the
	// boundary the Rust value gate publishes for the same record.
	healthRequest := passivesHealthAboveSpawnRequest()
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, healthRequest.core()); err == nil {
		t.Fatal("the Go encoder published health above the maximum")
	} else if !strings.Contains(err.Error(), "passive spawn health 21 outside 1..20") {
		t.Fatalf("the Go encoder refused the health violation with %v", err)
	}

	// The zero identity is refused on the outbound surface too, which is the
	// boundary the Rust checked newtype publishes for the same record.
	zeroID := passivesDespawnRequest{ServerTick: 0, Despawns: []passivesDespawnRecordRequest{{ID: 0, Reason: protocol.PassiveDespawnVanished}}}
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, zeroID.core()); err == nil {
		t.Fatal("the Go encoder published a zero passive identity")
	} else if !strings.Contains(err.Error(), "passive despawn ID is zero") {
		t.Fatalf("the Go encoder refused the zero identity with %v", err)
	}
}

// TestProtocolPassivesGoRecordStridesMatchTheFrozenLayout pins the three record
// strides and the single field difference between the two full-body records, so
// a layout change on either side fails here before it reaches a corpus case.
func TestProtocolPassivesGoRecordStridesMatchTheFrozenLayout(t *testing.T) {
	if passiveSpawnWireBytes != 29 {
		t.Fatalf("spawn record stride = %d, want 29", passiveSpawnWireBytes)
	}
	if passiveStateWireBytes != 38 {
		t.Fatalf("state record stride = %d, want 38", passiveStateWireBytes)
	}
	if passiveDespawnWireBytes != 9 {
		t.Fatalf("despawn record stride = %d, want 9", passiveDespawnWireBytes)
	}
	spawn := passivesSpawnRecordWire(passivesCanonicalSpawnRecords()[0])
	state := passivesStateRecordWire(passivesCanonicalStateRecords()[0])
	despawn := passivesDespawnRecordWire(passivesCanonicalDespawnRecords()[0])
	if len(spawn) != passiveSpawnWireBytes {
		t.Fatalf("rendered spawn record is %d bytes, want %d", len(spawn), passiveSpawnWireBytes)
	}
	if len(state) != passiveStateWireBytes {
		t.Fatalf("rendered state record is %d bytes, want %d", len(state), passiveStateWireBytes)
	}
	if len(despawn) != passiveDespawnWireBytes {
		t.Fatalf("rendered despawn record is %d bytes, want %d", len(despawn), passiveDespawnWireBytes)
	}
	// The spawn record names the overworld in the four bytes after the
	// identity, and neither record carries a kind byte: the state record
	// carries no dimension at all and starts its pose immediately after the
	// identity, while the spawn record's health closes the fixed 29-byte
	// stride and the state record's grazing bit closes the 38-byte stride.
	if got := spawn[8:12]; got[0] != 0 || got[1] != 0 || got[2] != 0 || got[3] != 0 {
		t.Fatalf("spawn dimension word = %v, want the overworld", got)
	}
	if !bytes.Equal(spawn[12:16], state[8:12]) {
		t.Fatalf("spawn position %v differs from state position %v", spawn[12:16], state[8:12])
	}
	if spawn[passiveSpawnWireBytes-1] != 1 {
		t.Fatalf("spawn record tail = %d, want the health byte 1 with no kind byte after it", spawn[passiveSpawnWireBytes-1])
	}
	if state[passiveStateWireBytes-1] != 1 {
		t.Fatalf("state record tail = %d, want the grazing byte 1", state[passiveStateWireBytes-1])
	}
	if despawn[passiveDespawnWireBytes-1] != 0 {
		t.Fatalf("despawn record tail = %d, want the reason byte 0", despawn[passiveDespawnWireBytes-1])
	}
}

// TestProtocolPassivesProducerIDIsAllowlisted is a compile-time-adjacent guard
// that the exported producer identity the standing contract pre-allowlists is
// the one this file publishes.
func TestProtocolPassivesProducerIDIsAllowlisted(t *testing.T) {
	if passivesProducerID != "runtime-oracle/protocol-passives" {
		t.Fatalf("producer ID = %s, want runtime-oracle/protocol-passives", passivesProducerID)
	}
	if _, ok := validProducerIDs[passivesProducerID]; !ok {
		t.Fatalf("producer ID %s is not in the closed exporter allowlist", passivesProducerID)
	}
}
