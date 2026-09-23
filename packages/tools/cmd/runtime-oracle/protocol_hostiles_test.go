package main

// This file is the hostile mob packet producer group: the three Play
// server-to-client families that publish the hostile bodies one subscriber can
// see (`HostileSpawn`, `HostileState` and `HostileDespawn`) each register a
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
// pose, health and kind messages, and the strict-order rule — so the category
// table classifies each one at the boundary that owns it. The identity is the
// one boundary the Go side refuses before the record validator runs in spirit
// but not in fact: `HostileSpawn.Validate` reports a zero ID through its own
// message, which is the identity boundary the Rust checked newtype publishes
// for the same bytes.

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
	// hostileSpawnFamily is the play hostile spawn family this group registers.
	hostileSpawnFamily = "protocol.server.HostileSpawn"
	// hostileStateFamily is the play hostile state batch family this group registers.
	hostileStateFamily = "protocol.server.HostileState"
	// hostileDespawnFamily is the play hostile despawn family this group registers.
	hostileDespawnFamily = "protocol.server.HostileDespawn"
	// hostilesVersion is the protocol version all three families are pinned to.
	hostilesVersion = "45"
	// hostilesProducerID is the exporter's producer identity for this group's
	// candidate assets and merged manifest.
	hostilesProducerID = "runtime-oracle/protocol-hostiles"

	// hostileSpawnCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	hostileSpawnCorpusRelDir = corpusCasesRelDir + "/protocol/HostileSpawn"
	// hostileStateCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	hostileStateCorpusRelDir = corpusCasesRelDir + "/protocol/HostileState"
	// hostileDespawnCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	hostileDespawnCorpusRelDir = corpusCasesRelDir + "/protocol/HostileDespawn"

	// hostileSpawnWireBytes is the fixed spawn record stride.
	hostileSpawnWireBytes = 30
	// hostileStateWireBytes is the fixed state record stride.
	hostileStateWireBytes = 38
	// hostileDespawnWireBytes is the fixed despawn record stride.
	hostileDespawnWireBytes = 8
	// hostilesMaxRecords is the hostile record budget one batch may carry.
	hostilesMaxRecords = 64
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	hostileSpawnDecodeCaseID             = hostileSpawnFamily + "/" + hostilesVersion + "/decode-valid"
	hostileSpawnEncodeCaseID             = hostileSpawnFamily + "/" + hostilesVersion + "/encode-valid"
	hostileSpawnFullDecodeCaseID         = hostileSpawnFamily + "/" + hostilesVersion + "/decode-count-sixty-four"
	hostileSpawnZeroIDDecodeCaseID       = hostileSpawnFamily + "/" + hostilesVersion + "/decode-id-zero"
	hostileSpawnDimensionDecodeCaseID    = hostileSpawnFamily + "/" + hostilesVersion + "/decode-dimension-depths"
	hostileSpawnHealthZeroDecodeCaseID   = hostileSpawnFamily + "/" + hostilesVersion + "/decode-health-zero"
	hostileSpawnHealthAboveDecodeCaseID  = hostileSpawnFamily + "/" + hostilesVersion + "/decode-health-above"
	hostileSpawnKindTwoDecodeCaseID      = hostileSpawnFamily + "/" + hostilesVersion + "/decode-kind-two"
	hostileSpawnKindTwoEncodeCaseID      = hostileSpawnFamily + "/" + hostilesVersion + "/encode-kind-two"
	hostileSpawnNaNDecodeCaseID          = hostileSpawnFamily + "/" + hostilesVersion + "/decode-nan-position"
	hostileSpawnReversedDecodeCaseID     = hostileSpawnFamily + "/" + hostilesVersion + "/decode-reversed-ids"
	hostileSpawnTrailingCaseID           = hostileSpawnFamily + "/" + hostilesVersion + "/decode-trailing-byte"
	hostileStateDecodeCaseID             = hostileStateFamily + "/" + hostilesVersion + "/decode-valid"
	hostileStateEncodeCaseID             = hostileStateFamily + "/" + hostilesVersion + "/encode-valid"
	hostileStateZeroIDDecodeCaseID       = hostileStateFamily + "/" + hostilesVersion + "/decode-id-zero"
	hostileStateNaNVelocityDecodeCaseID  = hostileStateFamily + "/" + hostilesVersion + "/decode-nan-velocity"
	hostileStateHealthAboveDecodeCaseID  = hostileStateFamily + "/" + hostilesVersion + "/decode-health-above"
	hostileStateKindTwoDecodeCaseID      = hostileStateFamily + "/" + hostilesVersion + "/decode-kind-two"
	hostileStateDuplicateDecodeCaseID    = hostileStateFamily + "/" + hostilesVersion + "/decode-duplicate-ids"
	hostileStateCountAboveDecodeCaseID   = hostileStateFamily + "/" + hostilesVersion + "/decode-count-above"
	hostileStateFullDecodeCaseID         = hostileStateFamily + "/" + hostilesVersion + "/decode-count-sixty-four"
	hostileStateTrailingCaseID           = hostileStateFamily + "/" + hostilesVersion + "/decode-trailing-byte"
	hostileDespawnDecodeCaseID           = hostileDespawnFamily + "/" + hostilesVersion + "/decode-valid"
	hostileDespawnEncodeCaseID           = hostileDespawnFamily + "/" + hostilesVersion + "/encode-valid"
	hostileDespawnZeroIDDecodeCaseID     = hostileDespawnFamily + "/" + hostilesVersion + "/decode-id-zero"
	hostileDespawnDuplicateDecodeCaseID  = hostileDespawnFamily + "/" + hostilesVersion + "/decode-duplicate-ids"
	hostileDespawnReversedDecodeCaseID   = hostileDespawnFamily + "/" + hostilesVersion + "/decode-reversed-ids"
	hostileDespawnCountAboveDecodeCaseID = hostileDespawnFamily + "/" + hostilesVersion + "/decode-count-above"
	hostileDespawnFullDecodeCaseID       = hostileDespawnFamily + "/" + hostilesVersion + "/decode-count-sixty-four"
	hostileDespawnTrailingCaseID         = hostileDespawnFamily + "/" + hostilesVersion + "/decode-trailing-byte"
)

// hostilesFamilyKeys pins each family's complete packet key. A case names its
// key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs. All three families are
// server-to-client play packets.
var hostilesFamilyKeys = map[string]PacketKeySpec{
	hostileSpawnFamily:   {Direction: packetDirectionServer, State: packetStatePlay, ID: 22},
	hostileStateFamily:   {Direction: packetDirectionServer, State: packetStatePlay, ID: 23},
	hostileDespawnFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 24},
}

// hostilesFamilies lists the three families in registry order, with the corpus
// directory that holds each family's assets. The order is the review order,
// not a dispatch table.
func hostilesFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{hostileSpawnFamily, hostileSpawnCorpusRelDir},
		{hostileStateFamily, hostileStateCorpusRelDir},
		{hostileDespawnFamily, hostileDespawnCorpusRelDir},
	}
}

// hostilesU64 renders one little-endian u64 field.
func hostilesU64(value uint64) []byte {
	wire := make([]byte, 8)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// hostilesI32 renders one little-endian i32 field.
func hostilesI32(value int32) []byte {
	wire := make([]byte, 4)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// hostilesFloatWire renders one IEEE-754 binary32 field, preserving the exact
// bits so a negative zero survives the wire round trip.
func hostilesFloatWire(value float32) []byte {
	wire := make([]byte, 4)
	bits := math.Float32bits(value)
	for index := range wire {
		wire[index] = byte(bits >> (8 * index))
	}
	return wire
}

// hostilesNegZero is the negative zero the reviewed vectors carry.
func hostilesNegZero() float32 {
	return float32(math.Copysign(0, -1))
}

// hostilesBatchHeader renders the tick and the one-byte count prefix.
func hostilesBatchHeader(tick uint64, count uint8) []byte {
	wire := append([]byte(nil), hostilesU64(tick)...)
	return append(wire, count)
}

// hostilesSpawnRecord is one spawn record before it is published.
type hostilesSpawnRecord struct {
	id        uint64
	dimension int32
	position  [3]float32
	yaw       float32
	health    uint8
	kind      uint8
}

// hostilesSpawnRecordWire renders one spawn record in the wire field order:
// the identity, the dimension, the position, the yaw, the health and the kind.
func hostilesSpawnRecordWire(record hostilesSpawnRecord) []byte {
	wire := make([]byte, 0, hostileSpawnWireBytes)
	wire = append(wire, hostilesU64(record.id)...)
	wire = append(wire, hostilesI32(record.dimension)...)
	for _, value := range record.position {
		wire = append(wire, hostilesFloatWire(value)...)
	}
	wire = append(wire, hostilesFloatWire(record.yaw)...)
	return append(wire, record.health, record.kind)
}

// hostilesStateRecord is one state record before it is published.
type hostilesStateRecord struct {
	id       uint64
	position [3]float32
	velocity [3]float32
	yaw      float32
	health   uint8
	kind     uint8
}

// hostilesStateRecordWire renders one state record in the wire field order: the
// identity, the position, the velocity, the yaw, the health and the kind. The
// dimension is absent from the record entirely.
func hostilesStateRecordWire(record hostilesStateRecord) []byte {
	wire := make([]byte, 0, hostileStateWireBytes)
	wire = append(wire, hostilesU64(record.id)...)
	for _, value := range record.position {
		wire = append(wire, hostilesFloatWire(value)...)
	}
	for _, value := range record.velocity {
		wire = append(wire, hostilesFloatWire(value)...)
	}
	wire = append(wire, hostilesFloatWire(record.yaw)...)
	return append(wire, record.health, record.kind)
}

// hostilesSpawnWire renders one spawn payload: the tick, the count that may
// disagree with the records present, and the ordered records.
func hostilesSpawnWire(tick uint64, declared uint8, records []hostilesSpawnRecord) []byte {
	wire := append([]byte(nil), hostilesBatchHeader(tick, declared)...)
	for _, record := range records {
		wire = append(wire, hostilesSpawnRecordWire(record)...)
	}
	return wire
}

// hostilesStateWire renders one state payload: the tick, the count that may
// disagree with the records present, and the ordered records.
func hostilesStateWire(tick uint64, declared uint8, records []hostilesStateRecord) []byte {
	wire := append([]byte(nil), hostilesBatchHeader(tick, declared)...)
	for _, record := range records {
		wire = append(wire, hostilesStateRecordWire(record)...)
	}
	return wire
}

// hostilesDespawnWire renders one despawn payload: the tick, the count that may
// disagree with the identities present, and the ordered identities.
func hostilesDespawnWire(tick uint64, declared uint8, ids []uint64) []byte {
	wire := append([]byte(nil), hostilesBatchHeader(tick, declared)...)
	for _, id := range ids {
		wire = append(wire, hostilesU64(id)...)
	}
	return wire
}

// hostilesAscendingSpawnRecord renders one record of the full spawn batch, so
// the reviewed ceiling vector is built from the same field values as the
// canonical vector.
func hostilesAscendingSpawnRecord(id uint64) hostilesSpawnRecord {
	return hostilesSpawnRecord{
		id:        id,
		dimension: int32(core.Overworld),
		position:  [3]float32{1, 2, 3},
		yaw:       0.5,
		health:    10,
		kind:      protocol.HostileKindNightwalker,
	}
}

// hostilesAscendingStateRecord renders one record of the full state batch.
func hostilesAscendingStateRecord(id uint64) hostilesStateRecord {
	return hostilesStateRecord{
		id:       id,
		position: [3]float32{1, 2, 3},
		velocity: [3]float32{0, 0.25, 3},
		yaw:      0.5,
		health:   10,
		kind:     protocol.HostileKindNightwalker,
	}
}

// hostilesFullSpawnRecords is the reviewed 64-record spawn batch: identities 1
// through 64 in ascending order, which is exactly the fixed record ceiling.
func hostilesFullSpawnRecords() []hostilesSpawnRecord {
	records := make([]hostilesSpawnRecord, 0, hostilesMaxRecords)
	for id := uint64(1); id <= hostilesMaxRecords; id++ {
		records = append(records, hostilesAscendingSpawnRecord(id))
	}
	return records
}

// hostilesFullStateRecords is the reviewed 64-record state batch.
func hostilesFullStateRecords() []hostilesStateRecord {
	records := make([]hostilesStateRecord, 0, hostilesMaxRecords)
	for id := uint64(1); id <= hostilesMaxRecords; id++ {
		records = append(records, hostilesAscendingStateRecord(id))
	}
	return records
}

// hostilesFullDespawnIDs is the reviewed 64-identity despawn batch.
func hostilesFullDespawnIDs() []uint64 {
	ids := make([]uint64, 0, hostilesMaxRecords)
	for id := uint64(1); id <= hostilesMaxRecords; id++ {
		ids = append(ids, id)
	}
	return ids
}

// hostilesCanonicalSpawnRecords is the reviewed two-record spawn batch: the
// health and kind boundaries 1 and 20 with 0 and 1, the overworld dimension,
// and a first position carrying a negative zero.
func hostilesCanonicalSpawnRecords() []hostilesSpawnRecord {
	return []hostilesSpawnRecord{
		{
			id:        1,
			dimension: int32(core.Overworld),
			position:  [3]float32{hostilesNegZero(), 1, 2},
			yaw:       0,
			health:    1,
			kind:      protocol.HostileKindNightwalker,
		},
		{
			id:        2,
			dimension: int32(core.Overworld),
			position:  [3]float32{3, 4, 5},
			yaw:       -2.5,
			health:    core.MaxHealth,
			kind:      protocol.HostileKindBoneThrower,
		},
	}
}

// hostilesCanonicalStateRecords is the reviewed two-record state batch, whose
// first velocity also carries a negative zero.
func hostilesCanonicalStateRecords() []hostilesStateRecord {
	return []hostilesStateRecord{
		{
			id:       1,
			position: [3]float32{hostilesNegZero(), 1, 2},
			velocity: [3]float32{hostilesNegZero(), 0.25, 3},
			yaw:      0.5,
			health:   13,
			kind:     protocol.HostileKindBoneThrower,
		},
		{
			id:       2,
			position: [3]float32{3, 4, 5},
			velocity: [3]float32{0.5, -1.25, 0},
			yaw:      -2.5,
			health:   7,
			kind:     protocol.HostileKindNightwalker,
		},
	}
}

// hostilesCanonicalDespawnIDs is the reviewed two-identity despawn batch.
func hostilesCanonicalDespawnIDs() []uint64 {
	return []uint64{1, 2}
}

// hostilesWithSpawnByte replaces one byte of a spawn payload at offset.
func hostilesWithSpawnByte(source []byte, offset int, value uint8) []byte {
	wire := append([]byte(nil), source...)
	wire[offset] = value
	return wire
}

// hostilesWithSpawnFloat replaces one binary32 field of a spawn payload at
// offset.
func hostilesWithSpawnFloat(source []byte, offset int, value float32) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[offset:offset+4], hostilesFloatWire(value))
	return wire
}

// hostilesWithStateFloat replaces one binary32 field of a state payload at
// offset.
func hostilesWithStateFloat(source []byte, offset int, value float32) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[offset:offset+4], hostilesFloatWire(value))
	return wire
}

// hostilesFloatBitsText renders one binary32 as its eight-digit
// lowercase-hexadecimal bit string, the canonical float encoding both
// directions publish.
func hostilesFloatBitsText(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// hostilesIDText renders one u64 identity as its decimal string, so the full
// identity range stays lossless in the normalized fields.
func hostilesIDText(value uint64) string {
	return strconv.FormatUint(value, 10)
}

// hostilesVectorText renders one three-component vector as its ordered
// bit-string array.
func hostilesVectorText(values [3]float32) []string {
	rendered := make([]string, 0, len(values))
	for _, value := range values {
		rendered = append(rendered, hostilesFloatBitsText(value))
	}
	return rendered
}

// hostilesSpawnRecordFields renders one spawn record's semantic fields.
func hostilesSpawnRecordFields(record protocol.HostileSpawnRecord) map[string]any {
	return map[string]any{
		"id":        hostilesIDText(record.ID),
		"dimension": int32(record.Dimension),
		"position":  hostilesVectorText(hostilesVec3(record.Position)),
		"yaw":       hostilesFloatBitsText(record.Yaw),
		"health":    record.Health,
		"kind":      record.Kind,
	}
}

// hostilesStateRecordFields renders one state record's semantic fields.
func hostilesStateRecordFields(record protocol.HostileStateRecord) map[string]any {
	return map[string]any{
		"id":       hostilesIDText(record.ID),
		"position": hostilesVectorText(hostilesVec3(record.Position)),
		"velocity": hostilesVectorText(hostilesVec3(record.Velocity)),
		"yaw":      hostilesFloatBitsText(record.Yaw),
		"health":   record.Health,
		"kind":     record.Kind,
	}
}

// hostilesVec3 renders one mathgl vector as its three ordered components.
func hostilesVec3(value mgl32.Vec3) [3]float32 {
	return [3]float32{value.X(), value.Y(), value.Z()}
}

// hostilesDespawnFields renders the semantic fields one despawn publishes.
func hostilesDespawnFields(despawn protocol.HostileDespawn) map[string]any {
	ids := make([]string, 0, len(despawn.IDs))
	for _, id := range despawn.IDs {
		ids = append(ids, hostilesIDText(id))
	}
	return map[string]any{
		"server_tick": hostilesIDText(despawn.ServerTick),
		"ids":         ids,
	}
}

// hostilesSpawnFields renders the semantic fields one spawn publishes.
//
// The tick is a decimal string so the full u64 range stays lossless, the
// records publish in wire order and never sorted.
func hostilesSpawnFields(spawn protocol.HostileSpawn) map[string]any {
	records := make([]map[string]any, 0, len(spawn.Spawns))
	for _, record := range spawn.Spawns {
		records = append(records, hostilesSpawnRecordFields(record))
	}
	return map[string]any{
		"server_tick": hostilesIDText(spawn.ServerTick),
		"spawns":      records,
	}
}

// hostilesStateFields renders the semantic fields one state batch publishes.
func hostilesStateFields(state protocol.HostileState) map[string]any {
	records := make([]map[string]any, 0, len(state.States))
	for _, record := range state.States {
		records = append(records, hostilesStateRecordFields(record))
	}
	return map[string]any{
		"server_tick": hostilesIDText(state.ServerTick),
		"states":      records,
	}
}

// hostilesRejectionCategory resolves the language-neutral rejection category
// for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and the
// outbound validators name, never over a sentinel identity, and a failure with
// no mapping is a hard error. The primitive answers precede the validator
// messages, because the decoder rejects a non-finite float while reading and
// only then hands the record to `Validate`.
//
// The three message shapes are each their own boundary. The count bound and the
// strict-order rule are value violations; the remaining-length check the Go
// decoder applies before it reads a record is the truncation boundary, which is
// the same boundary the Rust exact-record rule publishes for a padded payload;
// the per-record identity message is the identity boundary; the dimension and
// kind messages are enum violations; and the pose messages, which the float
// primitive reaches first for a NaN, are value violations.
func hostilesRejectionCategory(err error) (string, bool) {
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
	case strings.Contains(message, "kind"):
		return "invalid-enum", true
	case strings.Contains(message, "pose is not finite"), strings.Contains(message, "is not finite"):
		return "invalid-value", true
	case strings.Contains(message, "health"):
		return "invalid-value", true
	}
	return "", false
}

// hostilesPacketKey resolves one packet key to the Go state and numeric ID the
// codec dispatches on, and reports whether the packet travels server-to-client.
func hostilesPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := hostilesFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no hostile producer owns", c.ID, c.Family)
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

// newHostilesCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newHostilesCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// hostilesFieldsForPacket renders the semantic fields of one decoded
// publication packet, dispatching on the concrete DTO the production decoder
// returned.
func hostilesFieldsForPacket(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.HostileSpawn:
		return hostilesSpawnFields(message), nil
	case protocol.HostileState:
		return hostilesStateFields(message), nil
	case protocol.HostileDespawn:
		return hostilesDespawnFields(message), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// runHostilesDecode executes one publication decode case through the real Go
// decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` and classifies
// the failure it returns. It never reimplements the count, length, identity or
// ordering rules, so the recorded outcome is whatever the production codec
// decides about these exact bytes.
func runHostilesDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := hostilesPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newHostilesCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		category, classified := hostilesRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := hostilesFieldsForPacket(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// hostilesSpawnRecordRequest is the canonical JSON field input one spawn
// record carries.
type hostilesSpawnRecordRequest struct {
	ID        uint64   `json:"id"`
	Dimension int32    `json:"dimension"`
	Position  []string `json:"position"`
	Yaw       string   `json:"yaw"`
	Health    uint8    `json:"health"`
	Kind      uint8    `json:"kind"`
}

// core renders the request as the Go spawn record DTO.
func (request hostilesSpawnRecordRequest) core() protocol.HostileSpawnRecord {
	return protocol.HostileSpawnRecord{
		ID:        request.ID,
		Dimension: core.DimensionID(request.Dimension),
		Position:  mgl32.Vec3(hostilesPositionFromText(request.Position)),
		Yaw:       hostilesAngleFromText(request.Yaw),
		Health:    request.Health,
		Kind:      request.Kind,
	}
}

// hostilesStateRecordRequest is the canonical JSON field input one state
// record carries.
type hostilesStateRecordRequest struct {
	ID       uint64   `json:"id"`
	Position []string `json:"position"`
	Velocity []string `json:"velocity"`
	Yaw      string   `json:"yaw"`
	Health   uint8    `json:"health"`
	Kind     uint8    `json:"kind"`
}

// core renders the request as the Go state record DTO.
func (request hostilesStateRecordRequest) core() protocol.HostileStateRecord {
	return protocol.HostileStateRecord{
		ID:       request.ID,
		Position: mgl32.Vec3(hostilesPositionFromText(request.Position)),
		Velocity: mgl32.Vec3(hostilesPositionFromText(request.Velocity)),
		Yaw:      hostilesAngleFromText(request.Yaw),
		Health:   request.Health,
		Kind:     request.Kind,
	}
}

// hostilesSpawnRequest is the canonical JSON field input one spawn encode case
// carries.
type hostilesSpawnRequest struct {
	ServerTick uint64                       `json:"server_tick"`
	Spawns     []hostilesSpawnRecordRequest `json:"spawns"`
}

// core renders the request as the Go spawn DTO the encoder validates.
func (request hostilesSpawnRequest) core() protocol.HostileSpawn {
	spawns := make([]protocol.HostileSpawnRecord, 0, len(request.Spawns))
	for _, record := range request.Spawns {
		spawns = append(spawns, record.core())
	}
	return protocol.HostileSpawn{ServerTick: request.ServerTick, Spawns: spawns}
}

// hostilesStateRequest is the canonical JSON field input one state encode case
// carries.
type hostilesStateRequest struct {
	ServerTick uint64                       `json:"server_tick"`
	States     []hostilesStateRecordRequest `json:"states"`
}

// core renders the request as the Go state DTO the encoder validates.
func (request hostilesStateRequest) core() protocol.HostileState {
	states := make([]protocol.HostileStateRecord, 0, len(request.States))
	for _, record := range request.States {
		states = append(states, record.core())
	}
	return protocol.HostileState{ServerTick: request.ServerTick, States: states}
}

// hostilesDespawnRequest is the canonical JSON field input one despawn encode
// case carries.
type hostilesDespawnRequest struct {
	ServerTick uint64   `json:"server_tick"`
	IDs        []uint64 `json:"ids"`
}

// core renders the request as the Go despawn DTO the encoder validates.
func (request hostilesDespawnRequest) core() protocol.HostileDespawn {
	return protocol.HostileDespawn{ServerTick: request.ServerTick, IDs: request.IDs}
}

// hostilesParseBitsText decodes one eight-digit hexadecimal bit string.
func hostilesParseBitsText(text string) (float32, error) {
	bits, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: bits %q are not hexadecimal: %w", text, err)
	}
	return math.Float32frombits(uint32(bits)), nil
}

// hostilesPositionFromText decodes one three-element bit-string array.
func hostilesPositionFromText(values []string) [3]float32 {
	if len(values) != 3 {
		panic(fmt.Sprintf("runtime-oracle: reviewed position carries %d components", len(values)))
	}
	var parsed [3]float32
	for index, text := range values {
		value, err := hostilesParseBitsText(text)
		if err != nil {
			panic("runtime-oracle: reviewed position is not hexadecimal: " + err.Error())
		}
		parsed[index] = value
	}
	return parsed
}

// hostilesAngleFromText decodes one bit-string angle field.
func hostilesAngleFromText(text string) float32 {
	value, err := hostilesParseBitsText(text)
	if err != nil {
		panic("runtime-oracle: reviewed angle is not hexadecimal: " + err.Error())
	}
	return value
}

// hostilesBitRequest renders one binary32 as its canonical JSON request text.
func hostilesBitRequest(value float32) string {
	return hostilesFloatBitsText(value)
}

// hostilesSpawnRecordRequestFields renders one spawn record as its canonical
// JSON request fields.
func hostilesSpawnRecordRequestFields(record hostilesSpawnRecord) hostilesSpawnRecordRequest {
	return hostilesSpawnRecordRequest{
		ID:        record.id,
		Dimension: record.dimension,
		Position: []string{
			hostilesBitRequest(record.position[0]),
			hostilesBitRequest(record.position[1]),
			hostilesBitRequest(record.position[2]),
		},
		Yaw:    hostilesBitRequest(record.yaw),
		Health: record.health,
		Kind:   record.kind,
	}
}

// hostilesStateRecordRequestFields renders one state record as its canonical
// JSON request fields.
func hostilesStateRecordRequestFields(record hostilesStateRecord) hostilesStateRecordRequest {
	return hostilesStateRecordRequest{
		ID: record.id,
		Position: []string{
			hostilesBitRequest(record.position[0]),
			hostilesBitRequest(record.position[1]),
			hostilesBitRequest(record.position[2]),
		},
		Velocity: []string{
			hostilesBitRequest(record.velocity[0]),
			hostilesBitRequest(record.velocity[1]),
			hostilesBitRequest(record.velocity[2]),
		},
		Yaw:    hostilesBitRequest(record.yaw),
		Health: record.health,
		Kind:   record.kind,
	}
}

// hostilesSpawnRequestFields is the canonical JSON request the valid spawn
// encode case carries, which is the reviewed wire's own field set.
func hostilesSpawnRequestFields() hostilesSpawnRequest {
	records := hostilesCanonicalSpawnRecords()
	spawns := make([]hostilesSpawnRecordRequest, 0, len(records))
	for _, record := range records {
		spawns = append(spawns, hostilesSpawnRecordRequestFields(record))
	}
	return hostilesSpawnRequest{ServerTick: 0, Spawns: spawns}
}

// hostilesStateRequestFields is the canonical JSON request the valid state
// encode case carries.
func hostilesStateRequestFields() hostilesStateRequest {
	records := hostilesCanonicalStateRecords()
	states := make([]hostilesStateRecordRequest, 0, len(records))
	for _, record := range records {
		states = append(states, hostilesStateRecordRequestFields(record))
	}
	return hostilesStateRequest{ServerTick: 0, States: states}
}

// hostilesDespawnRequestFields is the canonical JSON request the valid despawn
// encode case carries.
func hostilesDespawnRequestFields() hostilesDespawnRequest {
	return hostilesDespawnRequest{ServerTick: 0, IDs: hostilesCanonicalDespawnIDs()}
}

// hostilesKindTwoSpawnRequest is the invalid encode request whose only
// violation is the closed kind rule: the reviewed record with kind 2.
func hostilesKindTwoSpawnRequest() hostilesSpawnRequest {
	request := hostilesSpawnRequestFields()
	records := hostilesCanonicalSpawnRecords()
	kindTwo := records[0]
	kindTwo.kind = 2
	request.Spawns[0] = hostilesSpawnRecordRequestFields(kindTwo)
	return request
}

// runHostilesEncode executes one publication encode case through the real Go
// encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects must
// never be recorded as evidence.
func runHostilesEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := hostilesPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newHostilesCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ServerPacket
	switch c.Family {
	case hostileSpawnFamily:
		var request hostilesSpawnRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	case hostileStateFamily:
		var request hostilesStateRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	case hostileDespawnFamily:
		var request hostilesDespawnRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	encodedID, payload, err := wireCodec.EncodeServer(state, packet)
	if err != nil {
		category, classified := hostilesRejectionCategory(err)
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
	fields, err := hostilesFieldsForPacket(c, decoded)
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

// hostilesCorpusRoutes is the closed route map the three families execute.
func hostilesCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range hostilesFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: hostilesVersion, Operation: "decode"}] = runHostilesDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: hostilesVersion, Operation: "encode"}] = runHostilesEncode
	}
	return routes
}

// hostilesCaseDefinition declares one case from literals before any producer
// runs, so the expectation is the review contract rather than a producer result.
type hostilesCaseDefinition struct {
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

// hostilesFullSpawnOutcome is the reviewed 64-record spawn batch outcome.
func hostilesFullSpawnOutcome() Outcome {
	records := hostilesFullSpawnRecords()
	spawns := make([]protocol.HostileSpawnRecord, 0, len(records))
	for _, record := range records {
		spawns = append(spawns, protocol.HostileSpawnRecord{
			ID:        record.id,
			Dimension: core.Overworld,
			Position:  record.position,
			Yaw:       record.yaw,
			Health:    record.health,
			Kind:      record.kind,
		})
	}
	return Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   hostilesSpawnFields(protocol.HostileSpawn{ServerTick: 0, Spawns: spawns}),
	}
}

// hostilesFullStateOutcome is the reviewed 64-record state batch outcome.
func hostilesFullStateOutcome() Outcome {
	records := hostilesFullStateRecords()
	states := make([]protocol.HostileStateRecord, 0, len(records))
	for _, record := range records {
		states = append(states, protocol.HostileStateRecord{
			ID:       record.id,
			Position: record.position,
			Velocity: record.velocity,
			Yaw:      record.yaw,
			Health:   record.health,
			Kind:     record.kind,
		})
	}
	return Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   hostilesStateFields(protocol.HostileState{ServerTick: 0, States: states}),
	}
}

// hostilesFullDespawnOutcome is the reviewed 64-identity despawn batch outcome.
func hostilesFullDespawnOutcome() Outcome {
	return Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   hostilesDespawnFields(protocol.HostileDespawn{ServerTick: 0, IDs: hostilesFullDespawnIDs()}),
	}
}

// hostilesCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid cases
// use the canonical payloads the Go encoder produces for these fields, including
// the 64-record boundary of each family; the malformed decode cases mutate that
// payload at one boundary each, and the invalid encode case builds a DTO the
// production validator refuses, so each rejection names the boundary that owns
// it.
//
// The hostile messages are distinct per boundary, so no boundary is latent: the
// count, the exact remaining-length check, the per-record identity, dimension,
// pose, health and kind messages, and the two strict-order messages each name
// their own condition. No case combines two violations, and no case names a
// doubly invalid record where the Go and Rust gate orders could diverge —
// every negative record carries exactly one violation beside its otherwise
// valid fields.
func hostilesCaseDefinitions() []hostilesCaseDefinition {
	validSpawn := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   hostilesSpawnFields(hostilesSpawnRequestFields().core()),
	}
	validState := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   hostilesStateFields(hostilesStateRequestFields().core()),
	}
	validDespawn := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   hostilesDespawnFields(hostilesDespawnRequestFields().core()),
	}
	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	invalidEnum := Outcome{Kind: "error", Category: "invalid-enum"}
	invalidIdentity := Outcome{Kind: "error", Category: "invalid-identity"}
	truncated := Outcome{Kind: "error", Category: "truncated"}

	spawnWire := hostilesSpawnWire(0, 2, hostilesCanonicalSpawnRecords())
	stateWire := hostilesStateWire(0, 2, hostilesCanonicalStateRecords())
	despawnWire := hostilesDespawnWire(0, 2, hostilesCanonicalDespawnIDs())
	fullSpawnWire := hostilesSpawnWire(0, hostilesMaxRecords, hostilesFullSpawnRecords())
	fullStateWire := hostilesStateWire(0, hostilesMaxRecords, hostilesFullStateRecords())
	fullDespawnWire := hostilesDespawnWire(0, hostilesMaxRecords, hostilesFullDespawnIDs())

	// One spawn record is the tick, the count prefix and the fixed 30-byte
	// stride, so the record fields sit at fixed offsets inside it.
	const spawnRecordBase = 9
	const spawnPositionOffset = spawnRecordBase + 8 + 4
	const spawnHealthOffset = spawnRecordBase + hostileSpawnWireBytes - 2
	const spawnKindOffset = spawnRecordBase + hostileSpawnWireBytes - 1
	// One state record is the tick, the count prefix and the fixed 38-byte
	// stride, and its velocity precedes the yaw.
	const stateRecordBase = 9
	const statePositionOffset = stateRecordBase + 8
	const stateVelocityOffset = statePositionOffset + 12
	const stateHealthOffset = stateRecordBase + hostileStateWireBytes - 2
	const stateKindOffset = stateRecordBase + hostileStateWireBytes - 1

	definitions := make([]hostilesCaseDefinition, 0, 30)
	definitions = append(definitions,
		hostilesCaseDefinition{
			id:     hostileSpawnDecodeCaseID,
			family: hostileSpawnFamily,
			relDir: hostileSpawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), spawnWire...),
			expect: validSpawn,
		},
		hostilesCaseDefinition{
			id:      hostileSpawnEncodeCaseID,
			family:  hostileSpawnFamily,
			relDir:  hostileSpawnCorpusRelDir,
			op:      "encode",
			request: hostilesSpawnRequestFields(),
			wire:    append([]byte(nil), spawnWire...),
			expect:  validSpawn,
		},
		hostilesCaseDefinition{
			// Sixty-four records is the count and record ceiling, admitted
			// exactly at the fixed payload bound.
			id:     hostileSpawnFullDecodeCaseID,
			family: hostileSpawnFamily,
			relDir: hostileSpawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), fullSpawnWire...),
			expect: hostilesFullSpawnOutcome(),
		},
		hostilesCaseDefinition{
			// The zero identity is the identity boundary, which the Go
			// validator answers with its own message.
			id:     hostileSpawnZeroIDDecodeCaseID,
			family: hostileSpawnFamily,
			relDir: hostileSpawnCorpusRelDir,
			op:     "decode",
			input: hostilesSpawnWire(0, 1, []hostilesSpawnRecord{
				func() hostilesSpawnRecord {
					record := hostilesCanonicalSpawnRecords()[0]
					record.id = 0
					return record
				}(),
			}),
			expect: invalidIdentity,
		},
		hostilesCaseDefinition{
			// The depths dimension is the single violation; the identity,
			// pose, health and kind stay valid.
			id:     hostileSpawnDimensionDecodeCaseID,
			family: hostileSpawnFamily,
			relDir: hostileSpawnCorpusRelDir,
			op:     "decode",
			input: hostilesSpawnWire(0, 1, []hostilesSpawnRecord{
				func() hostilesSpawnRecord {
					record := hostilesCanonicalSpawnRecords()[0]
					record.dimension = int32(core.Depths)
					return record
				}(),
			}),
			expect: invalidEnum,
		},
		hostilesCaseDefinition{
			id:     hostileSpawnHealthZeroDecodeCaseID,
			family: hostileSpawnFamily,
			relDir: hostileSpawnCorpusRelDir,
			op:     "decode",
			input:  hostilesWithSpawnByte(spawnWire, spawnHealthOffset, 0),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			// Twenty-one is one past the health maximum the Go core pins.
			id:     hostileSpawnHealthAboveDecodeCaseID,
			family: hostileSpawnFamily,
			relDir: hostileSpawnCorpusRelDir,
			op:     "decode",
			input:  hostilesWithSpawnByte(spawnWire, spawnHealthOffset, 21),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			// Kind 2 is outside the closed pair the wire publishes.
			id:     hostileSpawnKindTwoDecodeCaseID,
			family: hostileSpawnFamily,
			relDir: hostileSpawnCorpusRelDir,
			op:     "decode",
			input:  hostilesWithSpawnByte(spawnWire, spawnKindOffset, 2),
			expect: invalidEnum,
		},
		hostilesCaseDefinition{
			// The encode twin of the kind refusal: the same reviewed record
			// with kind 2 as a DTO, which the outbound validator refuses.
			id:      hostileSpawnKindTwoEncodeCaseID,
			family:  hostileSpawnFamily,
			relDir:  hostileSpawnCorpusRelDir,
			op:      "encode",
			request: hostilesKindTwoSpawnRequest(),
			expect:  invalidEnum,
		},
		hostilesCaseDefinition{
			// One NaN position word is answered by the decoder's float
			// primitive before the validator runs.
			id:     hostileSpawnNaNDecodeCaseID,
			family: hostileSpawnFamily,
			relDir: hostileSpawnCorpusRelDir,
			op:     "decode",
			input:  hostilesWithSpawnFloat(spawnWire, spawnPositionOffset, float32(math.NaN())),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			id:     hostileSpawnReversedDecodeCaseID,
			family: hostileSpawnFamily,
			relDir: hostileSpawnCorpusRelDir,
			op:     "decode",
			input:  hostilesSpawnWire(0, 2, []hostilesSpawnRecord{hostilesCanonicalSpawnRecords()[1], hostilesCanonicalSpawnRecords()[0]}),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			// The Go decoder applies the exact remaining-length rule, so a
			// one-byte-longer payload is answered at the truncation boundary
			// rather than by its end-of-payload check.
			id:     hostileSpawnTrailingCaseID,
			family: hostileSpawnFamily,
			relDir: hostileSpawnCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), spawnWire...), 0x00),
			expect: truncated,
		},
		hostilesCaseDefinition{
			id:     hostileStateDecodeCaseID,
			family: hostileStateFamily,
			relDir: hostileStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), stateWire...),
			expect: validState,
		},
		hostilesCaseDefinition{
			id:      hostileStateEncodeCaseID,
			family:  hostileStateFamily,
			relDir:  hostileStateCorpusRelDir,
			op:      "encode",
			request: hostilesStateRequestFields(),
			wire:    append([]byte(nil), stateWire...),
			expect:  validState,
		},
		hostilesCaseDefinition{
			id:     hostileStateZeroIDDecodeCaseID,
			family: hostileStateFamily,
			relDir: hostileStateCorpusRelDir,
			op:     "decode",
			input: hostilesStateWire(0, 1, []hostilesStateRecord{
				func() hostilesStateRecord {
					record := hostilesCanonicalStateRecords()[0]
					record.id = 0
					return record
				}(),
			}),
			expect: invalidIdentity,
		},
		hostilesCaseDefinition{
			// One NaN velocity word is answered by the float primitive.
			id:     hostileStateNaNVelocityDecodeCaseID,
			family: hostileStateFamily,
			relDir: hostileStateCorpusRelDir,
			op:     "decode",
			input:  hostilesWithStateFloat(stateWire, stateVelocityOffset, float32(math.NaN())),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			id:     hostileStateHealthAboveDecodeCaseID,
			family: hostileStateFamily,
			relDir: hostileStateCorpusRelDir,
			op:     "decode",
			input:  hostilesWithSpawnByte(stateWire, stateHealthOffset, 21),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			id:     hostileStateKindTwoDecodeCaseID,
			family: hostileStateFamily,
			relDir: hostileStateCorpusRelDir,
			op:     "decode",
			input:  hostilesWithSpawnByte(stateWire, stateKindOffset, 2),
			expect: invalidEnum,
		},
		hostilesCaseDefinition{
			id:     hostileStateDuplicateDecodeCaseID,
			family: hostileStateFamily,
			relDir: hostileStateCorpusRelDir,
			op:     "decode",
			input:  hostilesStateWire(0, 2, []hostilesStateRecord{hostilesCanonicalStateRecords()[0], hostilesCanonicalStateRecords()[0]}),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			// A tiny payload whose declared count is above the ceiling: the
			// count bound fires before the record-length rule on both sides.
			id:     hostileStateCountAboveDecodeCaseID,
			family: hostileStateFamily,
			relDir: hostileStateCorpusRelDir,
			op:     "decode",
			input:  hostilesStateWire(0, 65, []hostilesStateRecord{hostilesCanonicalStateRecords()[0]}),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			id:     hostileStateFullDecodeCaseID,
			family: hostileStateFamily,
			relDir: hostileStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), fullStateWire...),
			expect: hostilesFullStateOutcome(),
		},
		hostilesCaseDefinition{
			id:     hostileStateTrailingCaseID,
			family: hostileStateFamily,
			relDir: hostileStateCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), stateWire...), 0x00),
			expect: truncated,
		},
		hostilesCaseDefinition{
			id:     hostileDespawnDecodeCaseID,
			family: hostileDespawnFamily,
			relDir: hostileDespawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), despawnWire...),
			expect: validDespawn,
		},
		hostilesCaseDefinition{
			id:      hostileDespawnEncodeCaseID,
			family:  hostileDespawnFamily,
			relDir:  hostileDespawnCorpusRelDir,
			op:      "encode",
			request: hostilesDespawnRequestFields(),
			wire:    append([]byte(nil), despawnWire...),
			expect:  validDespawn,
		},
		hostilesCaseDefinition{
			id:     hostileDespawnZeroIDDecodeCaseID,
			family: hostileDespawnFamily,
			relDir: hostileDespawnCorpusRelDir,
			op:     "decode",
			input:  hostilesDespawnWire(0, 1, []uint64{0}),
			expect: invalidIdentity,
		},
		hostilesCaseDefinition{
			id:     hostileDespawnDuplicateDecodeCaseID,
			family: hostileDespawnFamily,
			relDir: hostileDespawnCorpusRelDir,
			op:     "decode",
			input:  hostilesDespawnWire(0, 2, []uint64{1, 1}),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			id:     hostileDespawnReversedDecodeCaseID,
			family: hostileDespawnFamily,
			relDir: hostileDespawnCorpusRelDir,
			op:     "decode",
			input:  hostilesDespawnWire(0, 2, []uint64{2, 1}),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			id:     hostileDespawnCountAboveDecodeCaseID,
			family: hostileDespawnFamily,
			relDir: hostileDespawnCorpusRelDir,
			op:     "decode",
			input:  hostilesDespawnWire(0, 65, []uint64{1}),
			expect: invalidValue,
		},
		hostilesCaseDefinition{
			id:     hostileDespawnFullDecodeCaseID,
			family: hostileDespawnFamily,
			relDir: hostileDespawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), fullDespawnWire...),
			expect: hostilesFullDespawnOutcome(),
		},
		hostilesCaseDefinition{
			id:     hostileDespawnTrailingCaseID,
			family: hostileDespawnFamily,
			relDir: hostileDespawnCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), despawnWire...), 0x00),
			expect: truncated,
		},
	)

	return definitions
}

// buildHostilesCandidate builds one case's manifest entry and asset bytes from
// its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildHostilesCandidate(t *testing.T, definition hostilesCaseDefinition) hostilesCandidate {
	t.Helper()

	label := hostilesLabel(definition.id)
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
		Version:      hostilesVersion,
		Operation:    definition.op,
		PacketKey:    hostilesKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return hostilesCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// hostilesKeyPointer resolves one family's reviewed packet key for a case spec.
func hostilesKeyPointer(family string) *PacketKeySpec {
	key, owned := hostilesFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// hostilesCandidate is one reviewed case: its manifest specification, the exact
// asset bytes it publishes, and the expectation an independent execution has to
// reproduce.
type hostilesCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which
	// the producer's own bytes are compared against.
	Wire []byte
}

// hostilesCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func hostilesCandidates(t *testing.T, root string) []hostilesCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := hostilesCaseDefinitions()
	candidates := make([]hostilesCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildHostilesCandidate(t, definition)
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

// hostilesRegisteredCase reports whether the base manifest already carries one
// case identity, so a re-merge of an integrated candidate adds nothing.
func hostilesRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// hostilesSelection is this group's registration: its cases, the Go sources its
// rules are read from, and the routes the three families execute.
func hostilesSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := hostilesCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range hostilesFamilies() {
		sources[family.id] = hostilesServerSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(hostilesFamilies())*2)
	for _, family := range hostilesFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: hostilesVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: hostilesVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  hostilesProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// hostilesServerSources lists the Go sources each family reads its rules from:
// the shared server codec dispatch with its decode arms and the payload
// ceiling, the message file that owns the DTO validators, and the family's own
// wire codec file.
func hostilesServerSources(family string) []string {
	return []string{
		"packages/shared/network/codec/codec_server.go",
		"packages/shared/network/protocol/message_hostile.go",
		"packages/shared/network/codec/hostile_wire.go",
	}
}

// hostilesManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func hostilesManifest(t *testing.T, root string, candidates []hostilesCandidate) Inventory {
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
		if !hostilesOwnsFamily(family) {
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
		t.Fatalf("encode hostiles working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write hostiles working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load hostiles working manifest: %v", err)
	}
	return loaded
}

// hostilesOwnsFamily reports whether this group registers cases for one family.
func hostilesOwnsFamily(family string) bool {
	_, owned := hostilesFamilyKeys[family]
	return owned
}

// hostilesScratchRoot stages this group's candidate assets in a harness-owned
// temporary directory, because a corpus case has to resolve under the root the
// runner is given.
func hostilesScratchRoot(t *testing.T, root string, candidates []hostilesCandidate) string {
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

// hostilesCandidateByID resolves one candidate by its case identity.
func hostilesCandidateByID(t *testing.T, candidates []hostilesCandidate, id string) hostilesCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return hostilesCandidate{}
}

// hostilesObservation resolves one executed observation by its case identity.
func hostilesObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// hostilesExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var hostilesExportPublished bool

// hostilesCandidatesExport publishes the reviewed candidates and the complete
// merged manifest candidate through the existing external exporter and returns
// the published producer directory. An unset export variable publishes nothing
// and returns "", so an ordinary test run never writes outside its own
// temporary storage.
func hostilesCandidatesExport(t *testing.T, root string, candidates []hostilesCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if hostilesExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-hostiles")
	}
	hostilesExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, hostilesProducerID, assets)
	if err != nil {
		t.Fatalf("export hostiles candidates: %v", err)
	}
	return published
}

// hostilesDecodeCaseIDs lists the valid decode cases the producer pins.
func hostilesDecodeCaseIDs() []string {
	return []string{
		hostileSpawnDecodeCaseID,
		hostileSpawnFullDecodeCaseID,
		hostileStateDecodeCaseID,
		hostileStateFullDecodeCaseID,
		hostileDespawnDecodeCaseID,
		hostileDespawnFullDecodeCaseID,
	}
}

// hostilesEncodeCaseIDs lists the valid encode cases the producer pins.
func hostilesEncodeCaseIDs() []string {
	return []string{
		hostileSpawnEncodeCaseID,
		hostileStateEncodeCaseID,
		hostileDespawnEncodeCaseID,
	}
}

// hostilesRejectedDecodeCaseIDs lists the malformed decode cases.
func hostilesRejectedDecodeCaseIDs() []string {
	return []string{
		hostileSpawnZeroIDDecodeCaseID,
		hostileSpawnDimensionDecodeCaseID,
		hostileSpawnHealthZeroDecodeCaseID,
		hostileSpawnHealthAboveDecodeCaseID,
		hostileSpawnKindTwoDecodeCaseID,
		hostileSpawnNaNDecodeCaseID,
		hostileSpawnReversedDecodeCaseID,
		hostileSpawnTrailingCaseID,
		hostileStateZeroIDDecodeCaseID,
		hostileStateNaNVelocityDecodeCaseID,
		hostileStateHealthAboveDecodeCaseID,
		hostileStateKindTwoDecodeCaseID,
		hostileStateDuplicateDecodeCaseID,
		hostileStateCountAboveDecodeCaseID,
		hostileStateTrailingCaseID,
		hostileDespawnZeroIDDecodeCaseID,
		hostileDespawnDuplicateDecodeCaseID,
		hostileDespawnReversedDecodeCaseID,
		hostileDespawnCountAboveDecodeCaseID,
		hostileDespawnTrailingCaseID,
	}
}

// hostilesRejectedEncodeCaseIDs lists the invalid encode cases.
func hostilesRejectedEncodeCaseIDs() []string {
	return []string{hostileSpawnKindTwoEncodeCaseID}
}

// hostilesLabel renders one case's asset label from its identity.
func hostilesLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// TestProtocolHostilesOracleDecodesEveryValidCase pins that the real Go decoder
// publishes the canonical fields for every family's valid decode case,
// including the full 64-record boundary.
func TestProtocolHostilesOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := hostilesCandidates(t, root)

	for _, id := range hostilesDecodeCaseIDs() {
		candidate := hostilesCandidateByID(t, candidates, id)
		outcome, encoded, err := runHostilesDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runHostilesDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolHostilesOracleEncodesCanonicalWire pins each encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolHostilesOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := hostilesCandidates(t, root)

	for _, id := range hostilesEncodeCaseIDs() {
		candidate := hostilesCandidateByID(t, candidates, id)
		outcome, encoded, err := runHostilesEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runHostilesEncode(%s): %v", id, err)
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

// TestProtocolHostilesOracleRejectsMalformedCasesAtTheirBoundary pins that
// every malformed decode case and the invalid encode case are refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolHostilesOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := hostilesCandidates(t, root)

	for _, id := range hostilesRejectedDecodeCaseIDs() {
		candidate := hostilesCandidateByID(t, candidates, id)
		outcome, encoded, err := runHostilesDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runHostilesDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range hostilesRejectedEncodeCaseIDs() {
		candidate := hostilesCandidateByID(t, candidates, id)
		outcome, encoded, err := runHostilesEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runHostilesEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolHostilesOracleExpectedFieldMutationFailsComparison pins that the
// recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded.
func TestProtocolHostilesOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := hostilesCandidates(t, root)

	decode := hostilesCandidateByID(t, candidates, hostileStateDecodeCaseID)
	produced, _, err := runHostilesDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runHostilesDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	// The velocity is the contract: the state record is the only hostile
	// record that carries it, so a normalization that dropped it would hide
	// the wire-valid velocity this vector carries.
	records := fields["states"].([]map[string]any)
	record := records[0]
	record["velocity"] = []string{
		hostilesFloatBitsText(1),
		hostilesFloatBitsText(2),
		hostilesFloatBitsText(3),
	}
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated velocity compares equal to the produced outcome")
	}

	encode := hostilesCandidateByID(t, candidates, hostileStateEncodeCaseID)
	producedEncode, _, err := runHostilesEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runHostilesEncode: %v", err)
	}
	mutatedEncode := encode.Expect
	mutatedEncode.Fields = map[string]any{"server_tick": "1"}
	if outcomesEqual(mutatedEncode, producedEncode) {
		t.Fatal("mutated encode fields compare equal to the produced outcome")
	}
}

// TestProtocolHostilesOracleRoutesExecuteEveryCase pins that every registered
// case is executed exactly once through its own route.
func TestProtocolHostilesOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := hostilesCandidates(t, root)
	manifest := hostilesManifest(t, root, candidates)
	staged := hostilesScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, hostilesCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := hostilesObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolHostilesOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so
// a case cannot claim coverage from its name alone.
func TestProtocolHostilesOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := hostilesCandidates(t, root)
	manifest := hostilesManifest(t, root, candidates)
	staged := hostilesScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range hostilesFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: hostilesVersion, Operation: "decode"}] = runHostilesDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range hostilesFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+hostilesVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolHostilesOracleManifestMergeRegistersHostileRoutes pins that the
// merged manifest registers all three families' routes and case lists, leaves
// the source revision alone, and records this group's provenance sources.
func TestProtocolHostilesOracleManifestMergeRegistersHostileRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, hostilesSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range hostilesFamilies() {
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
		for _, want := range hostilesServerSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so
	// a re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range hostilesCandidates(t, root) {
		if !hostilesRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolHostilesOracleCandidatesExportForReview publishes the reviewed
// candidates and the manifest candidate. An unset export variable publishes
// nothing, so the tracked corpus is never written by this package.
func TestProtocolHostilesOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := hostilesCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), hostilesSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := hostilesCandidatesExport(t, root, candidates, merged)
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
	for _, family := range hostilesFamilies() {
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

// TestProtocolHostilesGoDecoderAppliesExactRecordLength pins the rule this
// node's trailing-byte category is derived from: the Go decoder rejects a
// payload whose remaining length is not exactly `count` records before it reads
// one, so a one-byte-longer payload is answered at the truncation boundary
// rather than by a trailing-byte check. This is the opposite of the item drop
// batch, whose Go decoder applies a minimum-record budget and answers the same
// extra byte as trailing bytes.
func TestProtocolHostilesGoDecoderAppliesExactRecordLength(t *testing.T) {
	wireCodec, err := newHostilesCodec()
	if err != nil {
		t.Fatalf("newHostilesCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	for _, family := range []struct {
		id       uint32
		wire     []byte
		overWire []byte
		short    []byte
	}{
		{22, hostilesSpawnWire(0, 1, hostilesCanonicalSpawnRecords()[:1]), nil, nil},
		{23, hostilesStateWire(0, 1, hostilesCanonicalStateRecords()[:1]), nil, nil},
		{24, hostilesDespawnWire(0, 1, hostilesCanonicalDespawnIDs()[:1]), nil, nil},
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
	overSpawn := append(append([]byte(nil), hostilesBatchHeader(0, 65)...), hostilesSpawnRecordWire(hostilesAscendingSpawnRecord(1))...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 22, overSpawn); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "hostile spawn count is outside 1..64") {
		t.Fatalf("the Go decoder answered the over-count spawn payload with %v", err)
	}
	overState := append(append([]byte(nil), hostilesBatchHeader(0, 65)...), hostilesStateRecordWire(hostilesAscendingStateRecord(1))...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 23, overState); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "hostile state count is outside 1..64") {
		t.Fatalf("the Go decoder answered the over-count state payload with %v", err)
	}
	overDespawn := append(append([]byte(nil), hostilesBatchHeader(0, 65)...), hostilesU64(1)...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 24, overDespawn); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "hostile despawn count is outside 1..64") {
		t.Fatalf("the Go decoder answered the over-count despawn payload with %v", err)
	}

	// The zero-collection refusal precedes every record rule, so the empty
	// batch is refused at the count bound.
	emptySpawn := append([]byte(nil), hostilesBatchHeader(0, 0)...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 22, emptySpawn); err == nil {
		t.Fatal("the Go decoder accepted an empty batch")
	} else if !strings.Contains(err.Error(), "hostile spawn count is outside 1..64") {
		t.Fatalf("the Go decoder answered the empty batch with %v", err)
	}
}

// TestProtocolHostilesGoEncoderRefusesUnknownKind pins that the production
// encoder runs the outbound validator before it writes the record, so a hostile
// naming an unknown kind never becomes a silently published value.
//
// The Rust side published that kind silently before this node's surface
// conversion, which is the behavioral red the fallible surface closes; the Go
// encoder has always refused the DTO, and the corpus case records that refusal
// so the two implementations stay pinned to one boundary.
func TestProtocolHostilesGoEncoderRefusesUnknownKind(t *testing.T) {
	wireCodec, err := newHostilesCodec()
	if err != nil {
		t.Fatalf("newHostilesCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	request := hostilesKindTwoSpawnRequest()
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, request.core()); err == nil {
		t.Fatal("the Go encoder published an unknown hostile kind")
	} else if !strings.Contains(err.Error(), "hostile spawn kind 2 is invalid") {
		t.Fatalf("the Go encoder refused the unknown kind with %v", err)
	}

	// The zero identity is refused on the outbound surface too, which is the
	// boundary the Rust checked newtype publishes for the same record.
	zeroID := hostilesDespawnRequest{ServerTick: 0, IDs: []uint64{0}}
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, zeroID.core()); err == nil {
		t.Fatal("the Go encoder published a zero hostile identity")
	} else if !strings.Contains(err.Error(), "hostile despawn 0 ID is zero") {
		t.Fatalf("the Go encoder refused the zero identity with %v", err)
	}
}

// TestProtocolHostilesGoRecordStridesMatchTheFrozenLayout pins the two record
// strides and the single field difference between them, so a layout change on
// either side fails here before it reaches a corpus case.
func TestProtocolHostilesGoRecordStridesMatchTheFrozenLayout(t *testing.T) {
	if hostileSpawnWireBytes != 30 {
		t.Fatalf("spawn record stride = %d, want 30", hostileSpawnWireBytes)
	}
	if hostileStateWireBytes != 38 {
		t.Fatalf("state record stride = %d, want 38", hostileStateWireBytes)
	}
	if hostileDespawnWireBytes != 8 {
		t.Fatalf("despawn record stride = %d, want 8", hostileDespawnWireBytes)
	}
	spawn := hostilesSpawnRecordWire(hostilesCanonicalSpawnRecords()[0])
	if len(spawn) != hostileSpawnWireBytes {
		t.Fatalf("rendered spawn record is %d bytes, want %d", len(spawn), hostileSpawnWireBytes)
	}
	state := hostilesStateRecordWire(hostilesCanonicalStateRecords()[0])
	if len(state) != hostileStateWireBytes {
		t.Fatalf("rendered state record is %d bytes, want %d", len(state), hostileStateWireBytes)
	}
	// The spawn record names the overworld in the four bytes after the
	// identity, and the state record carries no dimension at all: its pose
	// starts immediately after the identity.
	if got := spawn[8:12]; got[0] != 0 || got[1] != 0 || got[2] != 0 || got[3] != 0 {
		t.Fatalf("spawn dimension word = %v, want the overworld", got)
	}
	if !bytes.Equal(spawn[12:16], state[8:12]) {
		t.Fatalf("spawn position %v differs from state position %v", spawn[12:16], state[8:12])
	}
}

// TestProtocolHostilesProducerIDIsAllowlisted is a compile-time-adjacent guard
// that the exported producer identity the standing contract pre-allowlists is
// the one this file publishes.
func TestProtocolHostilesProducerIDIsAllowlisted(t *testing.T) {
	if hostilesProducerID != "runtime-oracle/protocol-hostiles" {
		t.Fatalf("producer ID = %s, want runtime-oracle/protocol-hostiles", hostilesProducerID)
	}
	if _, ok := validProducerIDs[hostilesProducerID]; !ok {
		t.Fatalf("producer ID %s is not in the closed exporter allowlist", hostilesProducerID)
	}
}
