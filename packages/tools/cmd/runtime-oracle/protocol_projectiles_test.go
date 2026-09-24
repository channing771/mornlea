package main

// This file is the projectile packet producer group: the three Play
// server-to-client families that publish the projectiles one subscriber can
// see (`ProjectileSpawn`, `ProjectileState` and `ProjectileDespawn`) each
// register a decode and an encode route through the shared packet-case runner,
// so every case is executed by the real Go codec rather than restated here.
//
// The decode cases are the Go decoder's own bytes: the canonical payloads, the
// full-record boundary, and one structural or value mutation each. The encode
// cases carry canonical JSON fields and either the reviewed wire the Go encoder
// has to publish or a DTO the outbound validator has to refuse. Every negative
// carries exactly one violation.
//
// The three families answer every boundary with a distinct Go message — the
// count bound the codec applies before the family decoder, the exact
// remaining-length check, the per-record identity, kind, dimension and pose
// messages, and the strict-order rule — so the category table classifies each
// one at the boundary that owns it. The identity is the one boundary the Go
// side refuses before the record validator runs in spirit but not in fact:
// `ProjectileSpawn.Validate` reports a zero ID through its own message, which
// is the identity boundary the Rust checked newtype publishes for the same
// bytes.
//
// The over-ceiling case is the boundary this family group resolves: the Go
// decode path applies a fixed per-family payload maximum before the family
// decoder runs, and the Rust decoders now apply the same ceiling as a
// pre-parse size check, so both sides publish the capacity category for the
// same bytes instead of the truncation category the length rule alone would
// report. The hostile and passive groups record the opposite ruling.

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
	// projectileSpawnFamily is the play projectile spawn family this group
	// registers.
	projectileSpawnFamily = "protocol.server.ProjectileSpawn"
	// projectileStateFamily is the play projectile state batch family this
	// group registers.
	projectileStateFamily = "protocol.server.ProjectileState"
	// projectileDespawnFamily is the play projectile despawn family this group
	// registers.
	projectileDespawnFamily = "protocol.server.ProjectileDespawn"
	// projectilesVersion is the protocol version all three families are pinned
	// to.
	projectilesVersion = "45"
	// projectilesProducerID is the exporter's producer identity for this
	// group's candidate assets and merged manifest.
	projectilesProducerID = "runtime-oracle/protocol-projectiles"

	// projectileSpawnCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	projectileSpawnCorpusRelDir = corpusCasesRelDir + "/protocol/ProjectileSpawn"
	// projectileStateCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	projectileStateCorpusRelDir = corpusCasesRelDir + "/protocol/ProjectileState"
	// projectileDespawnCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	projectileDespawnCorpusRelDir = corpusCasesRelDir + "/protocol/ProjectileDespawn"

	// projectileSpawnWireBytes is the fixed spawn record stride: u64 ID, u8
	// kind, i32 dimension and two three-component vectors.
	projectileSpawnWireBytes = 37
	// projectileStateWireBytes is the fixed state record stride: u64 ID and one
	// three-component position, with no kind, dimension or velocity.
	projectileStateWireBytes = 20
	// projectileDespawnWireBytes is the fixed despawn record stride.
	projectileDespawnWireBytes = 8
	// projectilesMaxRecords is the projectile record budget one batch may
	// carry.
	projectilesMaxRecords = 128
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	projectileSpawnDecodeCaseID          = projectileSpawnFamily + "/" + projectilesVersion + "/decode-valid"
	projectileSpawnEncodeCaseID          = projectileSpawnFamily + "/" + projectilesVersion + "/encode-valid"
	projectileSpawnFullDecodeCaseID      = projectileSpawnFamily + "/" + projectilesVersion + "/decode-count-one-hundred-twenty-eight"
	projectileSpawnZeroIDDecodeCaseID    = projectileSpawnFamily + "/" + projectilesVersion + "/decode-id-zero"
	projectileSpawnKindTwoDecodeCaseID   = projectileSpawnFamily + "/" + projectilesVersion + "/decode-kind-two"
	projectileSpawnKindTwoEncodeCaseID   = projectileSpawnFamily + "/" + projectilesVersion + "/encode-kind-two"
	projectileSpawnDimensionCaseID       = projectileSpawnFamily + "/" + projectilesVersion + "/decode-dimension-two"
	projectileSpawnNaNDecodeCaseID       = projectileSpawnFamily + "/" + projectilesVersion + "/decode-nan-position"
	projectileSpawnReversedDecodeCaseID  = projectileSpawnFamily + "/" + projectilesVersion + "/decode-reversed-ids"
	projectileSpawnCountAboveCaseID      = projectileSpawnFamily + "/" + projectilesVersion + "/decode-count-above"
	projectileSpawnTrailingCaseID        = projectileSpawnFamily + "/" + projectilesVersion + "/decode-trailing-byte"
	projectileStateDecodeCaseID          = projectileStateFamily + "/" + projectilesVersion + "/decode-valid"
	projectileStateEncodeCaseID          = projectileStateFamily + "/" + projectilesVersion + "/encode-valid"
	projectileStateZeroIDDecodeCaseID    = projectileStateFamily + "/" + projectilesVersion + "/decode-id-zero"
	projectileStateNaNDecodeCaseID       = projectileStateFamily + "/" + projectilesVersion + "/decode-nan-position"
	projectileStateNaNEncodeCaseID       = projectileStateFamily + "/" + projectilesVersion + "/encode-nan-position"
	projectileStateDuplicateDecodeCaseID = projectileStateFamily + "/" + projectilesVersion + "/decode-duplicate-ids"
	projectileStateCountAboveCaseID      = projectileStateFamily + "/" + projectilesVersion + "/decode-count-above"
	projectileStateFullDecodeCaseID      = projectileStateFamily + "/" + projectilesVersion + "/decode-count-one-hundred-twenty-eight"
	projectileStateTrailingCaseID        = projectileStateFamily + "/" + projectilesVersion + "/decode-trailing-byte"
	projectileDespawnDecodeCaseID        = projectileDespawnFamily + "/" + projectilesVersion + "/decode-valid"
	projectileDespawnEncodeCaseID        = projectileDespawnFamily + "/" + projectilesVersion + "/encode-valid"
	projectileDespawnZeroIDDecodeCaseID  = projectileDespawnFamily + "/" + projectilesVersion + "/decode-id-zero"
	projectileDespawnDuplicateCaseID     = projectileDespawnFamily + "/" + projectilesVersion + "/decode-duplicate-ids"
	projectileDespawnReversedCaseID      = projectileDespawnFamily + "/" + projectilesVersion + "/decode-reversed-ids"
	projectileDespawnCountAboveCaseID    = projectileDespawnFamily + "/" + projectilesVersion + "/decode-count-above"
	projectileDespawnFullDecodeCaseID    = projectileDespawnFamily + "/" + projectilesVersion + "/decode-count-one-hundred-twenty-eight"
	projectileDespawnTrailingCaseID      = projectileDespawnFamily + "/" + projectilesVersion + "/decode-trailing-byte"
	projectileDespawnOverCeilingCaseID   = projectileDespawnFamily + "/" + projectilesVersion + "/decode-over-ceiling"
)

// projectilesFamilyKeys pins each family's complete packet key. A case names
// its key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs. All three families are
// server-to-client play packets.
var projectilesFamilyKeys = map[string]PacketKeySpec{
	projectileSpawnFamily:   {Direction: packetDirectionServer, State: packetStatePlay, ID: 29},
	projectileStateFamily:   {Direction: packetDirectionServer, State: packetStatePlay, ID: 30},
	projectileDespawnFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 31},
}

// projectilesFamilies lists the three families in registry order, with the
// corpus directory that holds each family's assets. The order is the review
// order, not a dispatch table.
func projectilesFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{projectileSpawnFamily, projectileSpawnCorpusRelDir},
		{projectileStateFamily, projectileStateCorpusRelDir},
		{projectileDespawnFamily, projectileDespawnCorpusRelDir},
	}
}

// projectilesU64 renders one little-endian u64 field.
func projectilesU64(value uint64) []byte {
	wire := make([]byte, 8)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// projectilesI32 renders one little-endian i32 field.
func projectilesI32(value int32) []byte {
	wire := make([]byte, 4)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// projectilesFloatWire renders one IEEE-754 binary32 field, preserving the
// exact bits so a negative zero survives the wire round trip.
func projectilesFloatWire(value float32) []byte {
	wire := make([]byte, 4)
	bits := math.Float32bits(value)
	for index := range wire {
		wire[index] = byte(bits >> (8 * index))
	}
	return wire
}

// projectilesNegZero is the negative zero the reviewed vectors carry.
func projectilesNegZero() float32 {
	return float32(math.Copysign(0, -1))
}

// projectilesBatchHeader renders the tick and the one-byte count prefix.
func projectilesBatchHeader(tick uint64, count uint8) []byte {
	wire := append([]byte(nil), projectilesU64(tick)...)
	return append(wire, count)
}

// projectilesSpawnRecord is one spawn record before it is published.
type projectilesSpawnRecord struct {
	id        uint64
	kind      uint8
	dimension int32
	position  [3]float32
	velocity  [3]float32
}

// projectilesSpawnRecordWire renders one spawn record in the wire field order:
// the identity, the kind, the dimension, the position and the velocity. The
// kind precedes the dimension, and no yaw or health exists.
func projectilesSpawnRecordWire(record projectilesSpawnRecord) []byte {
	wire := make([]byte, 0, projectileSpawnWireBytes)
	wire = append(wire, projectilesU64(record.id)...)
	wire = append(wire, record.kind)
	wire = append(wire, projectilesI32(record.dimension)...)
	for _, value := range record.position {
		wire = append(wire, projectilesFloatWire(value)...)
	}
	for _, value := range record.velocity {
		wire = append(wire, projectilesFloatWire(value)...)
	}
	return wire
}

// projectilesStateRecord is one state record before it is published.
type projectilesStateRecord struct {
	id       uint64
	position [3]float32
}

// projectilesStateRecordWire renders one state record in the wire field order:
// the identity and the position alone. A projectile's kind, dimension and
// velocity are fixed for its whole life, so the mirror records them at spawn.
func projectilesStateRecordWire(record projectilesStateRecord) []byte {
	wire := make([]byte, 0, projectileStateWireBytes)
	wire = append(wire, projectilesU64(record.id)...)
	for _, value := range record.position {
		wire = append(wire, projectilesFloatWire(value)...)
	}
	return wire
}

// projectilesSpawnWire renders one spawn payload: the tick, the count that may
// disagree with the records present, and the ordered records.
func projectilesSpawnWire(tick uint64, declared uint8, records []projectilesSpawnRecord) []byte {
	wire := append([]byte(nil), projectilesBatchHeader(tick, declared)...)
	for _, record := range records {
		wire = append(wire, projectilesSpawnRecordWire(record)...)
	}
	return wire
}

// projectilesStateWire renders one state payload: the tick, the count that may
// disagree with the records present, and the ordered records.
func projectilesStateWire(tick uint64, declared uint8, records []projectilesStateRecord) []byte {
	wire := append([]byte(nil), projectilesBatchHeader(tick, declared)...)
	for _, record := range records {
		wire = append(wire, projectilesStateRecordWire(record)...)
	}
	return wire
}

// projectilesDespawnWire renders one despawn payload: the tick, the count that
// may disagree with the identities present, and the ordered identities.
func projectilesDespawnWire(tick uint64, declared uint8, ids []uint64) []byte {
	wire := append([]byte(nil), projectilesBatchHeader(tick, declared)...)
	for _, id := range ids {
		wire = append(wire, projectilesU64(id)...)
	}
	return wire
}

// projectilesAscendingSpawnRecord renders one record of the full spawn batch,
// so the reviewed ceiling vector is built from the same field values as the
// canonical vector.
func projectilesAscendingSpawnRecord(id uint64) projectilesSpawnRecord {
	return projectilesSpawnRecord{
		id:        id,
		kind:      protocol.ProjectileKindArrow,
		dimension: int32(core.Overworld),
		position:  [3]float32{1, 2, 3},
		velocity:  [3]float32{0.5, 0, 0},
	}
}

// projectilesAscendingStateRecord renders one record of the full state batch.
func projectilesAscendingStateRecord(id uint64) projectilesStateRecord {
	return projectilesStateRecord{
		id:       id,
		position: [3]float32{1, 2, 3},
	}
}

// projectilesAscendingDespawnID renders one identity of the full despawn batch.
func projectilesAscendingDespawnID(id uint64) uint64 {
	return id
}

// projectilesFullSpawnRecords is the reviewed 128-record spawn batch:
// identities 1 through 128 in ascending order, which is exactly the fixed
// record ceiling and the payload's wire ceiling.
func projectilesFullSpawnRecords() []projectilesSpawnRecord {
	records := make([]projectilesSpawnRecord, 0, projectilesMaxRecords)
	for id := uint64(1); id <= projectilesMaxRecords; id++ {
		records = append(records, projectilesAscendingSpawnRecord(id))
	}
	return records
}

// projectilesFullStateRecords is the reviewed 128-record state batch.
func projectilesFullStateRecords() []projectilesStateRecord {
	records := make([]projectilesStateRecord, 0, projectilesMaxRecords)
	for id := uint64(1); id <= projectilesMaxRecords; id++ {
		records = append(records, projectilesAscendingStateRecord(id))
	}
	return records
}

// projectilesFullDespawnIDs is the reviewed 128-identity despawn batch.
func projectilesFullDespawnIDs() []uint64 {
	ids := make([]uint64, 0, projectilesMaxRecords)
	for id := uint64(1); id <= projectilesMaxRecords; id++ {
		ids = append(ids, projectilesAscendingDespawnID(id))
	}
	return ids
}

// projectilesCanonicalSpawnRecords is the reviewed two-record spawn batch: the
// two kinds across the two playable dimensions, with a negative zero in the
// first position and in the second velocity. A shard in the overworld and an
// arrow in the depths are both publishable records at this boundary, which is
// what the wire admits and the narrower authority rule does not.
func projectilesCanonicalSpawnRecords() []projectilesSpawnRecord {
	return []projectilesSpawnRecord{
		{
			id:        1,
			kind:      protocol.ProjectileKindShard,
			dimension: int32(core.Overworld),
			position:  [3]float32{projectilesNegZero(), 1, 2},
			velocity:  [3]float32{0.5, -1.25, 0},
		},
		{
			id:        2,
			kind:      protocol.ProjectileKindArrow,
			dimension: int32(core.Depths),
			position:  [3]float32{3, 4, 5},
			velocity:  [3]float32{projectilesNegZero(), 2, 3},
		},
	}
}

// projectilesCanonicalStateRecords is the reviewed two-record state batch,
// whose first position carries a negative zero.
func projectilesCanonicalStateRecords() []projectilesStateRecord {
	return []projectilesStateRecord{
		{
			id:       1,
			position: [3]float32{projectilesNegZero(), 1, 2},
		},
		{
			id:       2,
			position: [3]float32{3, 4, 5},
		},
	}
}

// projectilesCanonicalDespawnIDs is the reviewed two-identity despawn batch.
func projectilesCanonicalDespawnIDs() []uint64 {
	return []uint64{1, 2}
}

// projectilesWithSpawnByte replaces one byte of a spawn payload at offset.
func projectilesWithSpawnByte(source []byte, offset int, value uint8) []byte {
	wire := append([]byte(nil), source...)
	wire[offset] = value
	return wire
}

// projectilesWithStateFloat replaces one binary32 field of a state payload at
// offset.
func projectilesWithStateFloat(source []byte, offset int, value float32) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[offset:offset+4], projectilesFloatWire(value))
	return wire
}

// projectilesFloatBitsText renders one binary32 as its eight-digit
// lowercase-hexadecimal bit string, the canonical float encoding both
// directions publish.
func projectilesFloatBitsText(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// projectilesIDText renders one u64 identity as its decimal string, so the
// full identity range stays lossless in the normalized fields.
func projectilesIDText(value uint64) string {
	return strconv.FormatUint(value, 10)
}

// projectilesVectorText renders one three-component vector as its ordered
// bit-string array.
func projectilesVectorText(values [3]float32) []string {
	rendered := make([]string, 0, len(values))
	for _, value := range values {
		rendered = append(rendered, projectilesFloatBitsText(value))
	}
	return rendered
}

// projectilesVec3 renders one mathgl vector as its three ordered components.
func projectilesVec3(value mgl32.Vec3) [3]float32 {
	return [3]float32{value.X(), value.Y(), value.Z()}
}

// projectilesSpawnRecordFields renders one spawn record's semantic fields.
//
// The identity is a decimal string so the full u64 range stays lossless, the
// kind and the dimension are plain wire integers, and both vectors are ordered
// bit-string arrays so a negative zero stays distinct.
func projectilesSpawnRecordFields(record protocol.ProjectileSpawnRecord) map[string]any {
	return map[string]any{
		"id":        projectilesIDText(record.ID),
		"kind":      record.Kind,
		"dimension": int32(record.Dimension),
		"position":  projectilesVectorText(projectilesVec3(record.Position)),
		"velocity":  projectilesVectorText(projectilesVec3(record.Velocity)),
	}
}

// projectilesStateRecordFields renders one state record's semantic fields,
// which carry the identity and the position alone.
func projectilesStateRecordFields(record protocol.ProjectileStateRecord) map[string]any {
	return map[string]any{
		"id":       projectilesIDText(record.ID),
		"position": projectilesVectorText(projectilesVec3(record.Position)),
	}
}

// projectilesDespawnFields renders the semantic fields one despawn publishes.
func projectilesDespawnFields(despawn protocol.ProjectileDespawn) map[string]any {
	ids := make([]string, 0, len(despawn.IDs))
	for _, id := range despawn.IDs {
		ids = append(ids, projectilesIDText(id))
	}
	return map[string]any{
		"server_tick": projectilesIDText(despawn.ServerTick),
		"ids":         ids,
	}
}

// projectilesSpawnFields renders the semantic fields one spawn publishes.
//
// The tick is a decimal string so the full u64 range stays lossless, the
// records publish in wire order and never sorted.
func projectilesSpawnFields(spawn protocol.ProjectileSpawn) map[string]any {
	records := make([]map[string]any, 0, len(spawn.Spawns))
	for _, record := range spawn.Spawns {
		records = append(records, projectilesSpawnRecordFields(record))
	}
	return map[string]any{
		"server_tick": projectilesIDText(spawn.ServerTick),
		"spawns":      records,
	}
}

// projectilesStateFields renders the semantic fields one state batch publishes.
func projectilesStateFields(state protocol.ProjectileState) map[string]any {
	records := make([]map[string]any, 0, len(state.States))
	for _, record := range state.States {
		records = append(records, projectilesStateRecordFields(record))
	}
	return map[string]any{
		"server_tick": projectilesIDText(state.ServerTick),
		"states":      records,
	}
}

// projectilesRejectionCategory resolves the language-neutral rejection
// category for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and the
// outbound validators name, never over a sentinel identity, and a failure with
// no mapping is a hard error. The primitive answers precede the validator
// messages, because the decoder rejects a non-finite float while reading and
// only then hands the record to `Validate`.
//
// The pre-parse payload ceiling is the capacity boundary: the codec applies the
// family's fixed maximum before the family decoder runs, and the Rust decoders
// answer the same bytes with their capacity variant, so the over-ceiling case
// freezes one category on both sides. The remaining-length check the Go decoder
// applies before it reads a record is the truncation boundary, which is the
// same boundary the Rust exact-record rule publishes for a padded payload; the
// per-record identity message is the identity boundary; the dimension and kind
// messages are enum violations; and the pose messages, which the float
// primitive reaches first for a NaN, are value violations.
func projectilesRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "payload exceeds fixed maximum"):
		return "capacity", true
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "invalid float32"):
		return "invalid-value", true
	case strings.Contains(message, "count is outside 1..128"):
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
	}
	return "", false
}

// projectilesPacketKey resolves one packet key to the Go state and numeric ID
// the codec dispatches on, and reports whether the packet travels
// server-to-client.
func projectilesPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := projectilesFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no projectile producer owns", c.ID, c.Family)
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

// newProjectilesCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newProjectilesCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// projectilesFieldsForPacket renders the semantic fields of one decoded
// publication packet, dispatching on the concrete DTO the production decoder
// returned.
func projectilesFieldsForPacket(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.ProjectileSpawn:
		return projectilesSpawnFields(message), nil
	case protocol.ProjectileState:
		return projectilesStateFields(message), nil
	case protocol.ProjectileDespawn:
		return projectilesDespawnFields(message), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// runProjectilesDecode executes one publication decode case through the real
// Go decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` and classifies
// the failure it returns. It never reimplements the count, length, identity or
// ordering rules, so the recorded outcome is whatever the production codec
// decides about these exact bytes.
func runProjectilesDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := projectilesPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newProjectilesCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		category, classified := projectilesRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := projectilesFieldsForPacket(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// projectilesSpawnRecordRequest is the canonical JSON field input one spawn
// record carries.
type projectilesSpawnRecordRequest struct {
	ID        uint64   `json:"id"`
	Kind      uint8    `json:"kind"`
	Dimension int32    `json:"dimension"`
	Position  []string `json:"position"`
	Velocity  []string `json:"velocity"`
}

// core renders the request as the Go spawn record DTO.
func (request projectilesSpawnRecordRequest) core() protocol.ProjectileSpawnRecord {
	return protocol.ProjectileSpawnRecord{
		ID:        request.ID,
		Kind:      request.Kind,
		Dimension: core.DimensionID(request.Dimension),
		Position:  mgl32.Vec3(projectilesPositionFromText(request.Position)),
		Velocity:  mgl32.Vec3(projectilesPositionFromText(request.Velocity)),
	}
}

// projectilesStateRecordRequest is the canonical JSON field input one state
// record carries.
type projectilesStateRecordRequest struct {
	ID       uint64   `json:"id"`
	Position []string `json:"position"`
}

// core renders the request as the Go state record DTO.
func (request projectilesStateRecordRequest) core() protocol.ProjectileStateRecord {
	return protocol.ProjectileStateRecord{
		ID:       request.ID,
		Position: mgl32.Vec3(projectilesPositionFromText(request.Position)),
	}
}

// projectilesSpawnRequest is the canonical JSON field input one spawn encode
// case carries.
type projectilesSpawnRequest struct {
	ServerTick uint64                          `json:"server_tick"`
	Spawns     []projectilesSpawnRecordRequest `json:"spawns"`
}

// core renders the request as the Go spawn DTO the encoder validates.
func (request projectilesSpawnRequest) core() protocol.ProjectileSpawn {
	spawns := make([]protocol.ProjectileSpawnRecord, 0, len(request.Spawns))
	for _, record := range request.Spawns {
		spawns = append(spawns, record.core())
	}
	return protocol.ProjectileSpawn{ServerTick: request.ServerTick, Spawns: spawns}
}

// projectilesStateRequest is the canonical JSON field input one state encode
// case carries.
type projectilesStateRequest struct {
	ServerTick uint64                          `json:"server_tick"`
	States     []projectilesStateRecordRequest `json:"states"`
}

// core renders the request as the Go state DTO the encoder validates.
func (request projectilesStateRequest) core() protocol.ProjectileState {
	states := make([]protocol.ProjectileStateRecord, 0, len(request.States))
	for _, record := range request.States {
		states = append(states, record.core())
	}
	return protocol.ProjectileState{ServerTick: request.ServerTick, States: states}
}

// projectilesDespawnRequest is the canonical JSON field input one despawn
// encode case carries.
type projectilesDespawnRequest struct {
	ServerTick uint64   `json:"server_tick"`
	IDs        []uint64 `json:"ids"`
}

// core renders the request as the Go despawn DTO the encoder validates.
func (request projectilesDespawnRequest) core() protocol.ProjectileDespawn {
	return protocol.ProjectileDespawn{ServerTick: request.ServerTick, IDs: request.IDs}
}

// projectilesParseBitsText decodes one eight-digit hexadecimal bit string.
func projectilesParseBitsText(text string) (float32, error) {
	bits, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: bits %q are not hexadecimal: %w", text, err)
	}
	return math.Float32frombits(uint32(bits)), nil
}

// projectilesPositionFromText decodes one three-element bit-string array.
func projectilesPositionFromText(values []string) [3]float32 {
	if len(values) != 3 {
		panic(fmt.Sprintf("runtime-oracle: reviewed position carries %d components", len(values)))
	}
	var parsed [3]float32
	for index, text := range values {
		value, err := projectilesParseBitsText(text)
		if err != nil {
			panic("runtime-oracle: reviewed position is not hexadecimal: " + err.Error())
		}
		parsed[index] = value
	}
	return parsed
}

// projectilesBitRequest renders one binary32 as its canonical JSON request
// text.
func projectilesBitRequest(value float32) string {
	return projectilesFloatBitsText(value)
}

// projectilesSpawnRecordRequestFields renders one spawn record as its
// canonical JSON request fields.
func projectilesSpawnRecordRequestFields(record projectilesSpawnRecord) projectilesSpawnRecordRequest {
	return projectilesSpawnRecordRequest{
		ID:        record.id,
		Kind:      record.kind,
		Dimension: record.dimension,
		Position: []string{
			projectilesBitRequest(record.position[0]),
			projectilesBitRequest(record.position[1]),
			projectilesBitRequest(record.position[2]),
		},
		Velocity: []string{
			projectilesBitRequest(record.velocity[0]),
			projectilesBitRequest(record.velocity[1]),
			projectilesBitRequest(record.velocity[2]),
		},
	}
}

// projectilesStateRecordRequestFields renders one state record as its
// canonical JSON request fields.
func projectilesStateRecordRequestFields(record projectilesStateRecord) projectilesStateRecordRequest {
	return projectilesStateRecordRequest{
		ID: record.id,
		Position: []string{
			projectilesBitRequest(record.position[0]),
			projectilesBitRequest(record.position[1]),
			projectilesBitRequest(record.position[2]),
		},
	}
}

// projectilesSpawnRequestFields is the canonical JSON request the valid spawn
// encode case carries, which is the reviewed wire's own field set.
func projectilesSpawnRequestFields() projectilesSpawnRequest {
	records := projectilesCanonicalSpawnRecords()
	spawns := make([]projectilesSpawnRecordRequest, 0, len(records))
	for _, record := range records {
		spawns = append(spawns, projectilesSpawnRecordRequestFields(record))
	}
	return projectilesSpawnRequest{ServerTick: 0, Spawns: spawns}
}

// projectilesStateRequestFields is the canonical JSON request the valid state
// encode case carries.
func projectilesStateRequestFields() projectilesStateRequest {
	records := projectilesCanonicalStateRecords()
	states := make([]projectilesStateRecordRequest, 0, len(records))
	for _, record := range records {
		states = append(states, projectilesStateRecordRequestFields(record))
	}
	return projectilesStateRequest{ServerTick: 0, States: states}
}

// projectilesDespawnRequestFields is the canonical JSON request the valid
// despawn encode case carries.
func projectilesDespawnRequestFields() projectilesDespawnRequest {
	return projectilesDespawnRequest{ServerTick: 0, IDs: projectilesCanonicalDespawnIDs()}
}

// projectilesKindTwoSpawnRequest is the invalid encode request whose only
// violation is the closed kind rule: the reviewed record with kind 2.
func projectilesKindTwoSpawnRequest() projectilesSpawnRequest {
	request := projectilesSpawnRequestFields()
	records := projectilesCanonicalSpawnRecords()
	kindTwo := records[0]
	kindTwo.kind = 2
	request.Spawns[0] = projectilesSpawnRecordRequestFields(kindTwo)
	return request
}

// projectilesNaNStateRequest is the invalid encode request whose only
// violation is the finiteness rule: the reviewed state record with a NaN
// position, which the outbound validator refuses before the encoder writes it.
func projectilesNaNStateRequest() projectilesStateRequest {
	request := projectilesStateRequestFields()
	records := projectilesCanonicalStateRecords()
	nonFinite := records[0]
	nonFinite.position = [3]float32{float32(math.NaN()), 1, 2}
	request.States[0] = projectilesStateRecordRequestFields(nonFinite)
	return request
}

// runProjectilesEncode executes one publication encode case through the real
// Go encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects must
// never be recorded as evidence.
func runProjectilesEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := projectilesPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newProjectilesCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ServerPacket
	switch c.Family {
	case projectileSpawnFamily:
		var request projectilesSpawnRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	case projectileStateFamily:
		var request projectilesStateRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	case projectileDespawnFamily:
		var request projectilesDespawnRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	encodedID, payload, err := wireCodec.EncodeServer(state, packet)
	if err != nil {
		category, classified := projectilesRejectionCategory(err)
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
	fields, err := projectilesFieldsForPacket(c, decoded)
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

// projectilesCorpusRoutes is the closed route map the three families execute.
func projectilesCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range projectilesFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: projectilesVersion, Operation: "decode"}] = runProjectilesDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: projectilesVersion, Operation: "encode"}] = runProjectilesEncode
	}
	return routes
}

// projectilesCaseDefinition declares one case from literals before any
// producer runs, so the expectation is the review contract rather than a
// producer result.
type projectilesCaseDefinition struct {
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

// projectilesFullSpawnOutcome is the reviewed 128-record spawn batch outcome.
func projectilesFullSpawnOutcome() Outcome {
	records := projectilesFullSpawnRecords()
	spawns := make([]protocol.ProjectileSpawnRecord, 0, len(records))
	for _, record := range records {
		spawns = append(spawns, protocol.ProjectileSpawnRecord{
			ID:        record.id,
			Kind:      record.kind,
			Dimension: core.DimensionID(record.dimension),
			Position:  record.position,
			Velocity:  record.velocity,
		})
	}
	return Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   projectilesSpawnFields(protocol.ProjectileSpawn{ServerTick: 0, Spawns: spawns}),
	}
}

// projectilesFullStateOutcome is the reviewed 128-record state batch outcome.
func projectilesFullStateOutcome() Outcome {
	records := projectilesFullStateRecords()
	states := make([]protocol.ProjectileStateRecord, 0, len(records))
	for _, record := range records {
		states = append(states, protocol.ProjectileStateRecord{
			ID:       record.id,
			Position: record.position,
		})
	}
	return Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   projectilesStateFields(protocol.ProjectileState{ServerTick: 0, States: states}),
	}
}

// projectilesFullDespawnOutcome is the reviewed 128-identity despawn batch
// outcome.
func projectilesFullDespawnOutcome() Outcome {
	return Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: projectilesDespawnFields(
			protocol.ProjectileDespawn{ServerTick: 0, IDs: projectilesFullDespawnIDs()},
		),
	}
}

// projectilesCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid cases
// use the canonical payloads the Go encoder produces for these fields, including
// the 128-record boundary of each family; the malformed decode cases mutate
// that payload at one boundary each, and the invalid encode cases build DTOs
// the production validator refuses, so each rejection names the boundary that
// owns it.
//
// The projectile messages are distinct per boundary, so no boundary is latent:
// the pre-parse payload ceiling, the count bound, the exact remaining-length
// check, the per-record identity, kind, dimension and pose messages, and the
// two strict-order messages each name their own condition. No case combines
// two violations, and no case names a doubly invalid record where the Go and
// Rust gate orders could diverge — every negative record carries exactly one
// violation beside its otherwise valid fields.
func projectilesCaseDefinitions() []projectilesCaseDefinition {
	validSpawn := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   projectilesSpawnFields(projectilesSpawnRequestFields().core()),
	}
	validState := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   projectilesStateFields(projectilesStateRequestFields().core()),
	}
	validDespawn := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   projectilesDespawnFields(projectilesDespawnRequestFields().core()),
	}
	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	invalidEnum := Outcome{Kind: "error", Category: "invalid-enum"}
	invalidIdentity := Outcome{Kind: "error", Category: "invalid-identity"}
	truncated := Outcome{Kind: "error", Category: "truncated"}
	capacity := Outcome{Kind: "error", Category: "capacity"}

	spawnWire := projectilesSpawnWire(0, 2, projectilesCanonicalSpawnRecords())
	stateWire := projectilesStateWire(0, 2, projectilesCanonicalStateRecords())
	despawnWire := projectilesDespawnWire(0, 2, projectilesCanonicalDespawnIDs())
	fullSpawnWire := projectilesSpawnWire(0, projectilesMaxRecords, projectilesFullSpawnRecords())
	fullStateWire := projectilesStateWire(0, projectilesMaxRecords, projectilesFullStateRecords())
	fullDespawnWire := projectilesDespawnWire(0, projectilesMaxRecords, projectilesFullDespawnIDs())

	// One spawn record is the tick, the count prefix and the fixed 37-byte
	// stride, so the record fields sit at fixed offsets inside it.
	const spawnRecordBase = 9
	const spawnKindOffset = spawnRecordBase + 8
	const spawnDimensionOffset = spawnKindOffset + 1
	const spawnPositionOffset = spawnDimensionOffset + 4
	// One state record is the tick, the count prefix and the fixed 20-byte
	// stride, and its position follows the identity immediately.
	const statePositionOffset = 9 + 8

	definitions := make([]projectilesCaseDefinition, 0, 29)
	definitions = append(definitions,
		projectilesCaseDefinition{
			id:     projectileSpawnDecodeCaseID,
			family: projectileSpawnFamily,
			relDir: projectileSpawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), spawnWire...),
			expect: validSpawn,
		},
		projectilesCaseDefinition{
			id:      projectileSpawnEncodeCaseID,
			family:  projectileSpawnFamily,
			relDir:  projectileSpawnCorpusRelDir,
			op:      "encode",
			request: projectilesSpawnRequestFields(),
			wire:    append([]byte(nil), spawnWire...),
			expect:  validSpawn,
		},
		projectilesCaseDefinition{
			// One hundred twenty-eight records is the count and record
			// ceiling, admitted exactly at the fixed payload bound.
			id:     projectileSpawnFullDecodeCaseID,
			family: projectileSpawnFamily,
			relDir: projectileSpawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), fullSpawnWire...),
			expect: projectilesFullSpawnOutcome(),
		},
		projectilesCaseDefinition{
			// The zero identity is the identity boundary, which the Go
			// validator answers with its own message.
			id:     projectileSpawnZeroIDDecodeCaseID,
			family: projectileSpawnFamily,
			relDir: projectileSpawnCorpusRelDir,
			op:     "decode",
			input: projectilesSpawnWire(0, 1, []projectilesSpawnRecord{
				func() projectilesSpawnRecord {
					record := projectilesCanonicalSpawnRecords()[0]
					record.id = 0
					return record
				}(),
			}),
			expect: invalidIdentity,
		},
		projectilesCaseDefinition{
			// Kind 2 is outside the closed pair the wire publishes.
			id:     projectileSpawnKindTwoDecodeCaseID,
			family: projectileSpawnFamily,
			relDir: projectileSpawnCorpusRelDir,
			op:     "decode",
			input:  projectilesWithSpawnByte(spawnWire, spawnKindOffset, 2),
			expect: invalidEnum,
		},
		projectilesCaseDefinition{
			// The encode twin of the kind refusal: the same reviewed record
			// with kind 2 as a DTO, which the outbound validator refuses.
			id:      projectileSpawnKindTwoEncodeCaseID,
			family:  projectileSpawnFamily,
			relDir:  projectileSpawnCorpusRelDir,
			op:      "encode",
			request: projectilesKindTwoSpawnRequest(),
			expect:  invalidEnum,
		},
		projectilesCaseDefinition{
			// Dimension 2 is outside the two playable dimensions; the identity,
			// kind, pose and velocity stay valid.
			id:     projectileSpawnDimensionCaseID,
			family: projectileSpawnFamily,
			relDir: projectileSpawnCorpusRelDir,
			op:     "decode",
			input: projectilesSpawnWire(0, 1, []projectilesSpawnRecord{
				func() projectilesSpawnRecord {
					record := projectilesCanonicalSpawnRecords()[0]
					record.dimension = 2
					return record
				}(),
			}),
			expect: invalidEnum,
		},
		projectilesCaseDefinition{
			// One NaN position word is answered by the decoder's float
			// primitive before the validator runs.
			id:     projectileSpawnNaNDecodeCaseID,
			family: projectileSpawnFamily,
			relDir: projectileSpawnCorpusRelDir,
			op:     "decode",
			input:  projectilesWithStateFloat(spawnWire, spawnPositionOffset, float32(math.NaN())),
			expect: invalidValue,
		},
		projectilesCaseDefinition{
			id:     projectileSpawnReversedDecodeCaseID,
			family: projectileSpawnFamily,
			relDir: projectileSpawnCorpusRelDir,
			op:     "decode",
			input: projectilesSpawnWire(0, 2, []projectilesSpawnRecord{
				projectilesCanonicalSpawnRecords()[1],
				projectilesCanonicalSpawnRecords()[0],
			}),
			expect: invalidValue,
		},
		projectilesCaseDefinition{
			// A tiny payload whose declared count is above the ceiling: the
			// count bound fires before the record-length rule on both sides.
			id:     projectileSpawnCountAboveCaseID,
			family: projectileSpawnFamily,
			relDir: projectileSpawnCorpusRelDir,
			op:     "decode",
			input: projectilesSpawnWire(0, projectilesMaxRecords+1, []projectilesSpawnRecord{
				projectilesCanonicalSpawnRecords()[0],
			}),
			expect: invalidValue,
		},
		projectilesCaseDefinition{
			// The Go decoder applies the exact remaining-length rule, so a
			// one-byte-longer payload is answered at the truncation boundary
			// rather than by its end-of-payload check.
			id:     projectileSpawnTrailingCaseID,
			family: projectileSpawnFamily,
			relDir: projectileSpawnCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), spawnWire...), 0x00),
			expect: truncated,
		},
		projectilesCaseDefinition{
			id:     projectileStateDecodeCaseID,
			family: projectileStateFamily,
			relDir: projectileStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), stateWire...),
			expect: validState,
		},
		projectilesCaseDefinition{
			id:      projectileStateEncodeCaseID,
			family:  projectileStateFamily,
			relDir:  projectileStateCorpusRelDir,
			op:      "encode",
			request: projectilesStateRequestFields(),
			wire:    append([]byte(nil), stateWire...),
			expect:  validState,
		},
		projectilesCaseDefinition{
			id:     projectileStateZeroIDDecodeCaseID,
			family: projectileStateFamily,
			relDir: projectileStateCorpusRelDir,
			op:     "decode",
			input: projectilesStateWire(0, 1, []projectilesStateRecord{
				func() projectilesStateRecord {
					record := projectilesCanonicalStateRecords()[0]
					record.id = 0
					return record
				}(),
			}),
			expect: invalidIdentity,
		},
		projectilesCaseDefinition{
			// One NaN position word is answered by the float primitive.
			id:     projectileStateNaNDecodeCaseID,
			family: projectileStateFamily,
			relDir: projectileStateCorpusRelDir,
			op:     "decode",
			input:  projectilesWithStateFloat(stateWire, statePositionOffset, float32(math.NaN())),
			expect: invalidValue,
		},
		projectilesCaseDefinition{
			// The encode twin of the finiteness refusal: the same reviewed
			// record with a NaN position as a DTO, which the outbound
			// validator refuses.
			id:      projectileStateNaNEncodeCaseID,
			family:  projectileStateFamily,
			relDir:  projectileStateCorpusRelDir,
			op:      "encode",
			request: projectilesNaNStateRequest(),
			expect:  invalidValue,
		},
		projectilesCaseDefinition{
			id:     projectileStateDuplicateDecodeCaseID,
			family: projectileStateFamily,
			relDir: projectileStateCorpusRelDir,
			op:     "decode",
			input: projectilesStateWire(0, 2, []projectilesStateRecord{
				projectilesCanonicalStateRecords()[0],
				projectilesCanonicalStateRecords()[0],
			}),
			expect: invalidValue,
		},
		projectilesCaseDefinition{
			// A tiny payload whose declared count is above the ceiling: the
			// count bound fires before the record-length rule on both sides.
			id:     projectileStateCountAboveCaseID,
			family: projectileStateFamily,
			relDir: projectileStateCorpusRelDir,
			op:     "decode",
			input: projectilesStateWire(0, projectilesMaxRecords+1, []projectilesStateRecord{
				projectilesCanonicalStateRecords()[0],
			}),
			expect: invalidValue,
		},
		projectilesCaseDefinition{
			id:     projectileStateFullDecodeCaseID,
			family: projectileStateFamily,
			relDir: projectileStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), fullStateWire...),
			expect: projectilesFullStateOutcome(),
		},
		projectilesCaseDefinition{
			id:     projectileStateTrailingCaseID,
			family: projectileStateFamily,
			relDir: projectileStateCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), stateWire...), 0x00),
			expect: truncated,
		},
		projectilesCaseDefinition{
			id:     projectileDespawnDecodeCaseID,
			family: projectileDespawnFamily,
			relDir: projectileDespawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), despawnWire...),
			expect: validDespawn,
		},
		projectilesCaseDefinition{
			id:      projectileDespawnEncodeCaseID,
			family:  projectileDespawnFamily,
			relDir:  projectileDespawnCorpusRelDir,
			op:      "encode",
			request: projectilesDespawnRequestFields(),
			wire:    append([]byte(nil), despawnWire...),
			expect:  validDespawn,
		},
		projectilesCaseDefinition{
			id:     projectileDespawnZeroIDDecodeCaseID,
			family: projectileDespawnFamily,
			relDir: projectileDespawnCorpusRelDir,
			op:     "decode",
			input:  projectilesDespawnWire(0, 1, []uint64{0}),
			expect: invalidIdentity,
		},
		projectilesCaseDefinition{
			id:     projectileDespawnDuplicateCaseID,
			family: projectileDespawnFamily,
			relDir: projectileDespawnCorpusRelDir,
			op:     "decode",
			input:  projectilesDespawnWire(0, 2, []uint64{1, 1}),
			expect: invalidValue,
		},
		projectilesCaseDefinition{
			id:     projectileDespawnReversedCaseID,
			family: projectileDespawnFamily,
			relDir: projectileDespawnCorpusRelDir,
			op:     "decode",
			input:  projectilesDespawnWire(0, 2, []uint64{2, 1}),
			expect: invalidValue,
		},
		projectilesCaseDefinition{
			// A tiny payload whose declared count is above the ceiling: the
			// count bound fires before the record-length rule on both sides.
			id:     projectileDespawnCountAboveCaseID,
			family: projectileDespawnFamily,
			relDir: projectileDespawnCorpusRelDir,
			op:     "decode",
			input:  projectilesDespawnWire(0, projectilesMaxRecords+1, []uint64{1}),
			expect: invalidValue,
		},
		projectilesCaseDefinition{
			id:     projectileDespawnFullDecodeCaseID,
			family: projectileDespawnFamily,
			relDir: projectileDespawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), fullDespawnWire...),
			expect: projectilesFullDespawnOutcome(),
		},
		projectilesCaseDefinition{
			id:     projectileDespawnTrailingCaseID,
			family: projectileDespawnFamily,
			relDir: projectileDespawnCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), despawnWire...), 0x00),
			expect: truncated,
		},
		projectilesCaseDefinition{
			// One byte above the derived wire ceiling is refused by the
			// pre-parse payload maximum on the Go side and by the pre-parse
			// size check on the Rust side, so this is the one over-ceiling
			// boundary both implementations publish as the capacity category.
			id:     projectileDespawnOverCeilingCaseID,
			family: projectileDespawnFamily,
			relDir: projectileDespawnCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), fullDespawnWire...), 0x00),
			expect: capacity,
		},
	)

	return definitions
}

// buildProjectilesCandidate builds one case's manifest entry and asset bytes
// from its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildProjectilesCandidate(t *testing.T, definition projectilesCaseDefinition) projectilesCandidate {
	t.Helper()

	label := projectilesLabel(definition.id)
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
		Version:      projectilesVersion,
		Operation:    definition.op,
		PacketKey:    projectilesKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return projectilesCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// projectilesKeyPointer resolves one family's reviewed packet key for a case
// spec.
func projectilesKeyPointer(family string) *PacketKeySpec {
	key, owned := projectilesFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// projectilesCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type projectilesCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which
	// the producer's own bytes are compared against.
	Wire []byte
}

// projectilesCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func projectilesCandidates(t *testing.T, root string) []projectilesCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := projectilesCaseDefinitions()
	candidates := make([]projectilesCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildProjectilesCandidate(t, definition)
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

// projectilesRegisteredCase reports whether the base manifest already carries
// one case identity, so a re-merge of an integrated candidate adds nothing.
func projectilesRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// projectilesSelection is this group's registration: its cases, the Go sources
// its rules are read from, and the routes the three families execute.
func projectilesSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := projectilesCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range projectilesFamilies() {
		sources[family.id] = projectilesServerSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(projectilesFamilies())*2)
	for _, family := range projectilesFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: projectilesVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: projectilesVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  projectilesProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// projectilesServerSources lists the Go sources each family reads its rules
// from: the shared server codec dispatch with its decode arms and the payload
// ceiling, the message file that owns the DTO validators, and the family's own
// wire codec file.
func projectilesServerSources(family string) []string {
	return []string{
		"packages/shared/network/codec/codec_server.go",
		"packages/shared/network/protocol/message_projectile.go",
		"packages/shared/network/codec/codec_projectile.go",
	}
}

// projectilesManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func projectilesManifest(t *testing.T, root string, candidates []projectilesCandidate) Inventory {
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
		if !projectilesOwnsFamily(family) {
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
		t.Fatalf("encode projectiles working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write projectiles working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load projectiles working manifest: %v", err)
	}
	return loaded
}

// projectilesOwnsFamily reports whether this group registers cases for one
// family.
func projectilesOwnsFamily(family string) bool {
	_, owned := projectilesFamilyKeys[family]
	return owned
}

// projectilesScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve under
// the root the runner is given.
func projectilesScratchRoot(t *testing.T, root string, candidates []projectilesCandidate) string {
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

// projectilesCandidateByID resolves one candidate by its case identity.
func projectilesCandidateByID(t *testing.T, candidates []projectilesCandidate, id string) projectilesCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return projectilesCandidate{}
}

// projectilesObservation resolves one executed observation by its case
// identity.
func projectilesObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// projectilesExportPublished guards the single publication per test process,
// because the exporter's producer child is create-exclusive and more than one
// test in this package observes the same candidates.
var projectilesExportPublished bool

// projectilesCandidatesExport publishes the reviewed candidates and the
// complete merged manifest candidate through the existing external exporter and
// returns the published producer directory. An unset export variable publishes
// nothing and returns "", so an ordinary test run never writes outside its own
// temporary storage.
func projectilesCandidatesExport(t *testing.T, root string, candidates []projectilesCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if projectilesExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-projectiles")
	}
	projectilesExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, projectilesProducerID, assets)
	if err != nil {
		t.Fatalf("export projectiles candidates: %v", err)
	}
	return published
}

// projectilesDecodeCaseIDs lists the valid decode cases the producer pins.
func projectilesDecodeCaseIDs() []string {
	return []string{
		projectileSpawnDecodeCaseID,
		projectileSpawnFullDecodeCaseID,
		projectileStateDecodeCaseID,
		projectileStateFullDecodeCaseID,
		projectileDespawnDecodeCaseID,
		projectileDespawnFullDecodeCaseID,
	}
}

// projectilesEncodeCaseIDs lists the valid encode cases the producer pins.
func projectilesEncodeCaseIDs() []string {
	return []string{
		projectileSpawnEncodeCaseID,
		projectileStateEncodeCaseID,
		projectileDespawnEncodeCaseID,
	}
}

// projectilesRejectedDecodeCaseIDs lists the malformed decode cases.
func projectilesRejectedDecodeCaseIDs() []string {
	return []string{
		projectileSpawnZeroIDDecodeCaseID,
		projectileSpawnKindTwoDecodeCaseID,
		projectileSpawnDimensionCaseID,
		projectileSpawnNaNDecodeCaseID,
		projectileSpawnReversedDecodeCaseID,
		projectileSpawnCountAboveCaseID,
		projectileSpawnTrailingCaseID,
		projectileStateZeroIDDecodeCaseID,
		projectileStateNaNDecodeCaseID,
		projectileStateDuplicateDecodeCaseID,
		projectileStateCountAboveCaseID,
		projectileStateTrailingCaseID,
		projectileDespawnZeroIDDecodeCaseID,
		projectileDespawnDuplicateCaseID,
		projectileDespawnReversedCaseID,
		projectileDespawnCountAboveCaseID,
		projectileDespawnTrailingCaseID,
		projectileDespawnOverCeilingCaseID,
	}
}

// projectilesRejectedEncodeCaseIDs lists the invalid encode cases.
func projectilesRejectedEncodeCaseIDs() []string {
	return []string{
		projectileSpawnKindTwoEncodeCaseID,
		projectileStateNaNEncodeCaseID,
	}
}

// projectilesLabel renders one case's asset label from its identity.
func projectilesLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// TestProtocolProjectilesOracleDecodesEveryValidCase pins that the real Go
// decoder publishes the canonical fields for every family's valid decode case,
// including the full 128-record boundary.
func TestProtocolProjectilesOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := projectilesCandidates(t, root)

	for _, id := range projectilesDecodeCaseIDs() {
		candidate := projectilesCandidateByID(t, candidates, id)
		outcome, encoded, err := runProjectilesDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runProjectilesDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolProjectilesOracleEncodesCanonicalWire pins each encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolProjectilesOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := projectilesCandidates(t, root)

	for _, id := range projectilesEncodeCaseIDs() {
		candidate := projectilesCandidateByID(t, candidates, id)
		outcome, encoded, err := runProjectilesEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runProjectilesEncode(%s): %v", id, err)
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

// TestProtocolProjectilesOracleRejectsMalformedCasesAtTheirBoundary pins that
// every malformed decode case and the invalid encode cases are refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolProjectilesOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := projectilesCandidates(t, root)

	for _, id := range projectilesRejectedDecodeCaseIDs() {
		candidate := projectilesCandidateByID(t, candidates, id)
		outcome, encoded, err := runProjectilesDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runProjectilesDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range projectilesRejectedEncodeCaseIDs() {
		candidate := projectilesCandidateByID(t, candidates, id)
		outcome, encoded, err := runProjectilesEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runProjectilesEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolProjectilesOracleExpectedFieldMutationFailsComparison pins that
// the recorded expectation is a commitment: replacing an expected field fails
// comparison against what the producer decoded.
func TestProtocolProjectilesOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := projectilesCandidates(t, root)

	decode := projectilesCandidateByID(t, candidates, projectileStateDecodeCaseID)
	produced, _, err := runProjectilesDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runProjectilesDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	// The position is the contract: it is the only vector a state record
	// carries, so a normalization that dropped it would hide the wire-valid
	// position this vector carries.
	records := fields["states"].([]map[string]any)
	record := records[0]
	record["position"] = []string{
		projectilesFloatBitsText(1),
		projectilesFloatBitsText(2),
		projectilesFloatBitsText(3),
	}
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated position compares equal to the produced outcome")
	}

	encode := projectilesCandidateByID(t, candidates, projectileSpawnEncodeCaseID)
	producedEncode, _, err := runProjectilesEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runProjectilesEncode: %v", err)
	}
	mutatedEncode := encode.Expect
	mutatedEncode.Fields = map[string]any{"server_tick": "1"}
	if outcomesEqual(mutatedEncode, producedEncode) {
		t.Fatal("mutated encode fields compare equal to the produced outcome")
	}
}

// TestProtocolProjectilesOracleRoutesExecuteEveryCase pins that every
// registered case is executed exactly once through its own route.
func TestProtocolProjectilesOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := projectilesCandidates(t, root)
	manifest := projectilesManifest(t, root, candidates)
	staged := projectilesScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, projectilesCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := projectilesObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolProjectilesOracleRunnerRejectsUnregisteredRoute pins that a case
// naming a route this group does not claim fails before its producer runs, so
// a case cannot claim coverage from its name alone.
func TestProtocolProjectilesOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := projectilesCandidates(t, root)
	manifest := projectilesManifest(t, root, candidates)
	staged := projectilesScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range projectilesFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: projectilesVersion, Operation: "decode"}] = runProjectilesDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range projectilesFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+projectilesVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolProjectilesOracleManifestMergeRegistersProjectileRoutes pins
// that the merged manifest registers all three families' routes and case
// lists, leaves the source revision alone, and records this group's provenance
// sources.
func TestProtocolProjectilesOracleManifestMergeRegistersProjectileRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, projectilesSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range projectilesFamilies() {
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
		for _, want := range projectilesServerSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so
	// a re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range projectilesCandidates(t, root) {
		if !projectilesRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolProjectilesOracleCandidatesExportForReview publishes the
// reviewed candidates and the manifest candidate. An unset export variable
// publishes nothing, so the tracked corpus is never written by this package.
func TestProtocolProjectilesOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := projectilesCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), projectilesSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := projectilesCandidatesExport(t, root, candidates, merged)
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
	for _, family := range projectilesFamilies() {
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

// TestProtocolProjectilesGoDecoderAppliesExactRecordLength pins the rule this
// node's trailing-byte category is derived from: the Go decoder rejects a
// payload whose remaining length is not exactly `count` records before it reads
// one, so a one-byte-longer payload is answered at the truncation boundary
// rather than by a trailing-byte check. This is the opposite of the item drop
// batch, whose Go decoder applies a minimum-record budget and answers the same
// extra byte as trailing bytes.
func TestProtocolProjectilesGoDecoderAppliesExactRecordLength(t *testing.T) {
	wireCodec, err := newProjectilesCodec()
	if err != nil {
		t.Fatalf("newProjectilesCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	for _, family := range []struct {
		id   uint32
		wire []byte
	}{
		{29, projectilesSpawnWire(0, 1, projectilesCanonicalSpawnRecords()[:1])},
		{30, projectilesStateWire(0, 1, projectilesCanonicalStateRecords()[:1])},
		{31, projectilesDespawnWire(0, 1, projectilesCanonicalDespawnIDs()[:1])},
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
	overSpawn := append(append([]byte(nil), projectilesBatchHeader(0, projectilesMaxRecords+1)...),
		projectilesSpawnRecordWire(projectilesAscendingSpawnRecord(1))...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 29, overSpawn); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "projectile spawn count is outside 1..128") {
		t.Fatalf("the Go decoder answered the over-count spawn payload with %v", err)
	}
	overState := append(append([]byte(nil), projectilesBatchHeader(0, projectilesMaxRecords+1)...),
		projectilesStateRecordWire(projectilesAscendingStateRecord(1))...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 30, overState); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "projectile state count is outside 1..128") {
		t.Fatalf("the Go decoder answered the over-count state payload with %v", err)
	}
	overDespawn := append(append([]byte(nil), projectilesBatchHeader(0, projectilesMaxRecords+1)...),
		projectilesU64(1)...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 31, overDespawn); err == nil {
		t.Fatal("the Go decoder accepted a declared count above the ceiling")
	} else if !strings.Contains(err.Error(), "projectile despawn count is outside 1..128") {
		t.Fatalf("the Go decoder answered the over-count despawn payload with %v", err)
	}

	// The zero-collection refusal precedes every record rule, so the empty
	// batch is refused at the count bound.
	emptySpawn := append([]byte(nil), projectilesBatchHeader(0, 0)...)
	if _, err := wireCodec.DecodeServer(protocol.StatePlay, 29, emptySpawn); err == nil {
		t.Fatal("the Go decoder accepted an empty batch")
	} else if !strings.Contains(err.Error(), "projectile spawn count is outside 1..128") {
		t.Fatalf("the Go decoder answered the empty batch with %v", err)
	}
}

// TestProtocolProjectilesGoDecoderAppliesFixedWireCeiling pins the boundary
// this node resolved: the Go decode path refuses a payload above the family's
// fixed maximum before the family decoder runs, which is the capacity refusal
// the Rust pre-parse size check publishes for the same bytes. The over-ceiling
// corpus case is frozen from exactly this shape.
func TestProtocolProjectilesGoDecoderAppliesFixedWireCeiling(t *testing.T) {
	wireCodec, err := newProjectilesCodec()
	if err != nil {
		t.Fatalf("newProjectilesCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	ceilings := []struct {
		id      uint32
		atBound []byte
		over    []byte
		message string
	}{
		{
			id:      29,
			atBound: projectilesSpawnWire(0, projectilesMaxRecords, projectilesFullSpawnRecords()),
			message: "network: payload exceeds fixed maximum",
		},
		{
			id:      30,
			atBound: projectilesStateWire(0, projectilesMaxRecords, projectilesFullStateRecords()),
			message: "network: payload exceeds fixed maximum",
		},
		{
			id:      31,
			atBound: projectilesDespawnWire(0, projectilesMaxRecords, projectilesFullDespawnIDs()),
			message: "network: payload exceeds fixed maximum",
		},
	}
	for _, family := range ceilings {
		over := append(append([]byte(nil), family.atBound...), 0x00)
		if _, err := wireCodec.DecodeServer(protocol.StatePlay, family.id, over); err == nil {
			t.Fatalf("packet %d: the Go decoder accepted an over-ceiling payload", family.id)
		} else if !strings.Contains(err.Error(), family.message) {
			t.Fatalf("packet %d: the Go decoder answered the over-ceiling payload with %v", family.id, err)
		}
	}
	// The full ceiling itself is admitted, which is what makes one byte above
	// it the single over-ceiling mutation the corpus case freezes.
	if _, err := wireCodec.DecodeServer(
		protocol.StatePlay,
		31,
		projectilesDespawnWire(0, projectilesMaxRecords, projectilesFullDespawnIDs()),
	); err != nil {
		t.Fatalf("the Go decoder refused the full 128-record despawn batch: %v", err)
	}
}

// TestProtocolProjectilesGoEncoderRefusesClosedPairBytes pins that the
// production encoder runs the outbound validator before it writes the record,
// so a projectile naming an unknown kind or a non-finite pose never becomes a
// silently published value.
//
// The Rust side published that kind silently before this node's surface
// conversion, which is the behavioral red the fallible surface closes; the Go
// encoder has always refused the DTO, and the corpus cases record that refusal
// so the two implementations stay pinned to one boundary.
func TestProtocolProjectilesGoEncoderRefusesClosedPairBytes(t *testing.T) {
	wireCodec, err := newProjectilesCodec()
	if err != nil {
		t.Fatalf("newProjectilesCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	request := projectilesKindTwoSpawnRequest()
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, request.core()); err == nil {
		t.Fatal("the Go encoder published an unknown projectile kind")
	} else if !strings.Contains(err.Error(), "projectile spawn kind 2 is invalid") {
		t.Fatalf("the Go encoder refused the unknown kind with %v", err)
	}

	// A non-finite state position is refused on the outbound surface too.
	nonFinite := projectilesNaNStateRequest()
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, nonFinite.core()); err == nil {
		t.Fatal("the Go encoder published a non-finite projectile state")
	} else if !strings.Contains(err.Error(), "projectile state is not finite") {
		t.Fatalf("the Go encoder refused the non-finite state with %v", err)
	}

	// The zero identity is refused on the outbound surface too, which is the
	// boundary the Rust checked newtype publishes for the same record.
	zeroID := projectilesDespawnRequest{ServerTick: 0, IDs: []uint64{0}}
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, zeroID.core()); err == nil {
		t.Fatal("the Go encoder published a zero projectile identity")
	} else if !strings.Contains(err.Error(), "projectile despawn 0 ID is zero") {
		t.Fatalf("the Go encoder refused the zero identity with %v", err)
	}
}

// TestProtocolProjectilesGoRecordStridesMatchTheFrozenLayout pins the three
// record strides and the single field difference between them, so a layout
// change on either side fails here before it reaches a corpus case.
func TestProtocolProjectilesGoRecordStridesMatchTheFrozenLayout(t *testing.T) {
	if projectileSpawnWireBytes != 37 {
		t.Fatalf("spawn record stride = %d, want 37", projectileSpawnWireBytes)
	}
	if projectileStateWireBytes != 20 {
		t.Fatalf("state record stride = %d, want 20", projectileStateWireBytes)
	}
	if projectileDespawnWireBytes != 8 {
		t.Fatalf("despawn record stride = %d, want 8", projectileDespawnWireBytes)
	}
	spawn := projectilesSpawnRecordWire(projectilesCanonicalSpawnRecords()[0])
	if len(spawn) != projectileSpawnWireBytes {
		t.Fatalf("rendered spawn record is %d bytes, want %d", len(spawn), projectileSpawnWireBytes)
	}
	state := projectilesStateRecordWire(projectilesCanonicalStateRecords()[0])
	if len(state) != projectileStateWireBytes {
		t.Fatalf("rendered state record is %d bytes, want %d", len(state), projectileStateWireBytes)
	}
	// The spawn record names the kind in the byte after the identity and the
	// dimension in the four bytes after that, and the state record carries
	// neither: its position starts immediately after the identity.
	if spawn[8] != protocol.ProjectileKindShard {
		t.Fatalf("spawn kind byte = %d, want %d", spawn[8], protocol.ProjectileKindShard)
	}
	if got := spawn[9:13]; got[0] != 0 || got[1] != 0 || got[2] != 0 || got[3] != 0 {
		t.Fatalf("spawn dimension word = %v, want the overworld", got)
	}
	if !bytes.Equal(spawn[13:17], state[8:12]) {
		t.Fatalf("spawn position %v differs from state position %v", spawn[13:17], state[8:12])
	}
	// The three derived payload ceilings are the header plus the full record
	// budget, which is the bound the pre-parse check applies.
	if got := 9 + projectilesMaxRecords*projectileSpawnWireBytes; got != 4745 {
		t.Fatalf("derived spawn payload ceiling = %d, want 4745", got)
	}
	if got := 9 + projectilesMaxRecords*projectileStateWireBytes; got != 2569 {
		t.Fatalf("derived state payload ceiling = %d, want 2569", got)
	}
	if got := 9 + projectilesMaxRecords*projectileDespawnWireBytes; got != 1033 {
		t.Fatalf("derived despawn payload ceiling = %d, want 1033", got)
	}
}

// TestProtocolProjectilesProducerIDIsAllowlisted is a compile-time-adjacent
// guard that the exported producer identity the standing contract pre-allowlists
// is the one this file publishes.
func TestProtocolProjectilesProducerIDIsAllowlisted(t *testing.T) {
	if projectilesProducerID != "runtime-oracle/protocol-projectiles" {
		t.Fatalf("producer ID = %s, want runtime-oracle/protocol-projectiles", projectilesProducerID)
	}
	if _, ok := validProducerIDs[projectilesProducerID]; !ok {
		t.Fatalf("producer ID %s is not in the closed exporter allowlist", projectilesProducerID)
	}
}
