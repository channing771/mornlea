package main

// This file is the remote player packet producer group: the three Play
// server-to-client families that publish the other players one subscriber can
// see (`RemotePlayerSpawn`, `RemotePlayerDespawn` and `RemotePlayerStates`)
// each register a decode and an encode route through the shared packet-case
// runner, so every case is executed by the real Go codec rather than restated
// here.
//
// The decode cases are the Go decoder's own bytes: the canonical payloads, the
// batch count and order boundaries, and one structural mutation each. The
// encode cases carry canonical JSON fields and either the reviewed wire the Go
// encoder has to publish or a DTO the outbound validator has to refuse. Every
// negative carries exactly one violation.
//
// The Go `RemotePlayerSpawn.Validate` folds the identity, the name, the
// dimension and the pose finiteness into one predicate with the single message
// `network: invalid remote player spawn`, so those boundaries cannot publish
// distinct categories on the Go side. This group therefore freezes only the
// boundaries where both sides agree — the name and the finiteness — and the
// zero and wrong-version identity and the unknown dimension stay Rust
// group-test pins. The despawn's identity message is family-specific and the
// batch's count, order and per-record messages are each their own boundary, so
// those families classify exactly as their tables require.

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

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// remotePlayerSpawnFamily is the play remote player spawn family this group registers.
	remotePlayerSpawnFamily = "protocol.server.RemotePlayerSpawn"
	// remotePlayerDespawnFamily is the play remote player despawn family this group registers.
	remotePlayerDespawnFamily = "protocol.server.RemotePlayerDespawn"
	// remotePlayerStatesFamily is the play remote player state batch family this group registers.
	remotePlayerStatesFamily = "protocol.server.RemotePlayerStates"
	// remotePlayersVersion is the protocol version all three families are pinned to.
	remotePlayersVersion = "45"
	// remotePlayersProducerID is the exporter's producer identity for this
	// group's candidate assets and merged manifest.
	remotePlayersProducerID = "runtime-oracle/protocol-remote-players"

	// remotePlayerSpawnCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	remotePlayerSpawnCorpusRelDir = corpusCasesRelDir + "/protocol/RemotePlayerSpawn"
	// remotePlayerDespawnCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	remotePlayerDespawnCorpusRelDir = corpusCasesRelDir + "/protocol/RemotePlayerDespawn"
	// remotePlayerStatesCorpusRelDir is the repository-relative directory
	// holding this family's committed case assets.
	remotePlayerStatesCorpusRelDir = corpusCasesRelDir + "/protocol/RemotePlayerStates"

	// remotePlayerStateWireBytes is the fixed record stride.
	remotePlayerStateWireBytes = 41
	// remotePlayersMaxRecords is the peer-session budget one batch may carry.
	remotePlayersMaxRecords = 7
	// remotePlayersMaxWireBytes is the fixed payload ceiling the Go decoder
	// applies before it allocates.
	remotePlayersMaxWireBytes = 8 + 1 + remotePlayersMaxRecords*remotePlayerStateWireBytes
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	spawnDecodeCaseID     = remotePlayerSpawnFamily + "/" + remotePlayersVersion + "/decode-valid"
	spawnEncodeCaseID     = remotePlayerSpawnFamily + "/" + remotePlayersVersion + "/encode-valid"
	spawnPaddedNameDecID  = remotePlayerSpawnFamily + "/" + remotePlayersVersion + "/decode-padded-name"
	spawnPaddedNameEncID  = remotePlayerSpawnFamily + "/" + remotePlayersVersion + "/encode-padded-name"
	spawnNaNPitchDecID    = remotePlayerSpawnFamily + "/" + remotePlayersVersion + "/decode-nan-pitch"
	despawnDecodeCaseID   = remotePlayerDespawnFamily + "/" + remotePlayersVersion + "/decode-valid"
	despawnEncodeCaseID   = remotePlayerDespawnFamily + "/" + remotePlayersVersion + "/encode-valid"
	despawnZeroUUIDDecID  = remotePlayerDespawnFamily + "/" + remotePlayersVersion + "/decode-zero-uuid"
	despawnTrailingID     = remotePlayerDespawnFamily + "/" + remotePlayersVersion + "/decode-trailing-byte"
	statesDecodeCaseID    = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/decode-valid"
	statesEncodeCaseID    = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/encode-valid"
	statesCountOneID      = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/decode-count-one"
	statesCountSevenID    = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/decode-count-seven"
	statesCountZeroID     = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/decode-count-zero"
	statesCountEightID    = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/decode-count-eight"
	statesDuplicateID     = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/decode-duplicate-uuids"
	statesReversedID      = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/decode-reversed-uuids"
	statesDimensionTwoID  = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/decode-dimension-two"
	statesInfiniteCoordID = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/decode-infinite-coordinate"
	statesTrailingID      = remotePlayerStatesFamily + "/" + remotePlayersVersion + "/decode-trailing-byte"
)

// remotePlayersFamilyKeys pins each family's complete packet key. A case names
// its key instead of trusting its family, so a case registered under the wrong
// family, state or ID fails before the codec runs. All three families are
// server-to-client play packets.
var remotePlayersFamilyKeys = map[string]PacketKeySpec{
	remotePlayerSpawnFamily:   {Direction: packetDirectionServer, State: packetStatePlay, ID: 7},
	remotePlayerDespawnFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 8},
	remotePlayerStatesFamily:  {Direction: packetDirectionServer, State: packetStatePlay, ID: 9},
}

// remotePlayersFamilies lists the three families in registry order, with the
// corpus directory that holds each family's assets. The order is the review
// order, not a dispatch table.
func remotePlayersFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{remotePlayerSpawnFamily, remotePlayerSpawnCorpusRelDir},
		{remotePlayerDespawnFamily, remotePlayerDespawnCorpusRelDir},
		{remotePlayerStatesFamily, remotePlayerStatesCorpusRelDir},
	}
}

// remotePlayersU64 renders one little-endian u64 field.
func remotePlayersU64(value uint64) []byte {
	wire := make([]byte, 8)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// remotePlayersU32 renders one little-endian u32 field.
func remotePlayersU32(value uint32) []byte {
	wire := make([]byte, 4)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// remotePlayersFloatWire renders one IEEE-754 binary32 field, preserving the
// exact bits so a negative zero survives the wire round trip.
func remotePlayersFloatWire(value float32) []byte {
	return remotePlayersU32(math.Float32bits(value))
}

// remotePlayersUvarint renders one canonical uvarint count prefix.
func remotePlayersUvarint(value uint32) []byte {
	wire := make([]byte, 0, 5)
	for value >= 1<<7 {
		wire = append(wire, byte(value)|0x80)
		value >>= 7
	}
	return append(wire, byte(value))
}

// remotePlayersNegZero is the negative zero the reviewed vectors carry.
func remotePlayersNegZero() float32 {
	return float32(math.Copysign(0, -1))
}

// remotePlayersControlPlayer is the reviewed identity: UUIDv4 with the version
// and variant nibbles the control player carries.
func remotePlayersControlPlayer() core.PlayerID {
	return core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}
}

// remotePlayersNextPlayer is the control identity's byte-successor.
func remotePlayersNextPlayer() core.PlayerID {
	return core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 16}
}

// remotePlayersAscendingPlayer renders one identity of the full seven-record
// batch, ascending in unsigned byte order.
func remotePlayersAscendingPlayer(index int) core.PlayerID {
	return core.PlayerID{0, 0, 0, 0, 0, 0, byte(0x40 + index), 0, byte(0x80 + index), 0, 0, 0, 0, 0, 0, 0}
}

// remotePlayersSpawnWire renders one spawn payload in the wire field order:
// the identity, the length-prefixed name, the tick, the dimension, the
// position, the yaw and the pitch.
func remotePlayersSpawnWire(id core.PlayerID, name string, tick uint64, dimension int32, position [3]float32, yaw, pitch float32) []byte {
	wire := make([]byte, 0, 16+2+len(name)+8+4+20)
	wire = append(wire, id[:]...)
	wire = append(wire, remotePlayersUvarint(uint32(len(name)))...)
	wire = append(wire, []byte(name)...)
	wire = append(wire, remotePlayersU64(tick)...)
	wire = append(wire, remotePlayersU32(uint32(dimension))...)
	for _, value := range position {
		wire = append(wire, remotePlayersFloatWire(value)...)
	}
	wire = append(wire, remotePlayersFloatWire(yaw)...)
	return append(wire, remotePlayersFloatWire(pitch)...)
}

// remotePlayersCanonicalSpawnWire is the reviewed spawn literal: the control
// identity, the canonical name "Alice", tick 0, the depths dimension, a
// position carrying a negative zero, yaw 0 and the wire-valid pitch 2.0.
func remotePlayersCanonicalSpawnWire() []byte {
	return remotePlayersSpawnWire(remotePlayersControlPlayer(), "Alice", 0, int32(core.Depths),
		[3]float32{remotePlayersNegZero(), 1, 2}, 0, 2.0)
}

// remotePlayersPaddedNameSpawnWire is the name-bound mutation: the same record
// with the non-canonical name `" Alice "`, which `NormalizeDisplayName` would
// have to trim.
func remotePlayersPaddedNameSpawnWire() []byte {
	return remotePlayersSpawnWire(remotePlayersControlPlayer(), " Alice ", 0, int32(core.Depths),
		[3]float32{remotePlayersNegZero(), 1, 2}, 0, 2.0)
}

// remotePlayersNaNPitchSpawnWire is the finiteness mutation: the same record
// with a NaN pitch, which the decoder's float primitive answers before the
// validator runs.
func remotePlayersNaNPitchSpawnWire() []byte {
	return remotePlayersSpawnWire(remotePlayersControlPlayer(), "Alice", 0, int32(core.Depths),
		[3]float32{remotePlayersNegZero(), 1, 2}, 0, float32(math.NaN()))
}

// remotePlayersDespawnWire renders one despawn payload: the identity alone.
func remotePlayersDespawnWire(id core.PlayerID) []byte {
	wire := make([]byte, 0, 16)
	return append(wire, id[:]...)
}

// remotePlayersStateRecord is one batch record before it is published.
type remotePlayersStateRecord struct {
	id       core.PlayerID
	dim      int32
	position [3]float32
	yaw      float32
	pitch    float32
	reset    bool
}

// remotePlayersRecordWire renders one record in the wire field order.
func remotePlayersRecordWire(record remotePlayersStateRecord) []byte {
	wire := make([]byte, 0, remotePlayerStateWireBytes)
	wire = append(wire, record.id[:]...)
	wire = append(wire, remotePlayersU32(uint32(record.dim))...)
	for _, value := range record.position {
		wire = append(wire, remotePlayersFloatWire(value)...)
	}
	wire = append(wire, remotePlayersFloatWire(record.yaw)...)
	wire = append(wire, remotePlayersFloatWire(record.pitch)...)
	if record.reset {
		return append(wire, 1)
	}
	return append(wire, 0)
}

// remotePlayersRecordsWire renders the ordered record region.
func remotePlayersRecordsWire(records []remotePlayersStateRecord) []byte {
	wire := make([]byte, 0, len(records)*remotePlayerStateWireBytes)
	for _, record := range records {
		wire = append(wire, remotePlayersRecordWire(record)...)
	}
	return wire
}

// remotePlayersStatesWire renders one batch payload: the tick, the canonical
// uvarint count and the ordered records.
func remotePlayersStatesWire(tick uint64, records []remotePlayersStateRecord) []byte {
	wire := append([]byte(nil), remotePlayersU64(tick)...)
	wire = append(wire, remotePlayersUvarint(uint32(len(records)))...)
	return append(wire, remotePlayersRecordsWire(records)...)
}

// remotePlayersStatesWireWithDeclaredCount renders one batch payload whose
// declared count may disagree with the records present, which is the shape the
// count bound has to answer first.
func remotePlayersStatesWireWithDeclaredCount(tick uint64, declared uint32, records []remotePlayersStateRecord) []byte {
	wire := append([]byte(nil), remotePlayersU64(tick)...)
	wire = append(wire, remotePlayersUvarint(declared)...)
	return append(wire, remotePlayersRecordsWire(records)...)
}

// remotePlayersCanonicalRecords is the reviewed two-record batch: the control
// identity in the overworld and its byte-successor in the depths, both poses
// carrying a negative zero, with the two reset flags.
func remotePlayersCanonicalRecords() []remotePlayersStateRecord {
	return []remotePlayersStateRecord{
		{
			id:       remotePlayersControlPlayer(),
			dim:      int32(core.Overworld),
			position: [3]float32{remotePlayersNegZero(), 1, 2},
			yaw:      0,
			pitch:    2.0,
			reset:    false,
		},
		{
			id:       remotePlayersNextPlayer(),
			dim:      int32(core.Depths),
			position: [3]float32{remotePlayersNegZero(), -1.5, 0},
			yaw:      0,
			pitch:    2.0,
			reset:    true,
		},
	}
}

// remotePlayersSingleRecord is the reviewed one-record batch the count-boundary
// admit case carries.
func remotePlayersSingleRecord() []remotePlayersStateRecord {
	record := remotePlayersCanonicalRecords()[0]
	return []remotePlayersStateRecord{record}
}

// remotePlayersFullRecords is the full seven-record batch: seven identities
// ascending in unsigned byte order, which is exactly the fixed wire ceiling.
func remotePlayersFullRecords() []remotePlayersStateRecord {
	records := make([]remotePlayersStateRecord, 0, remotePlayersMaxRecords)
	for index := 0; index < remotePlayersMaxRecords; index++ {
		records = append(records, remotePlayersStateRecord{
			id:       remotePlayersAscendingPlayer(index),
			dim:      int32(core.Overworld),
			position: [3]float32{1, 2, 3},
			yaw:      0,
			pitch:    2.0,
			reset:    index%2 == 0,
		})
	}
	return records
}

// remotePlayersWithByte replaces one byte of a payload at offset.
func remotePlayersWithByte(source []byte, offset int, value uint8) []byte {
	wire := append([]byte(nil), source...)
	wire[offset] = value
	return wire
}

// remotePlayersWithU32 replaces one little-endian u32 field of a payload at
// offset.
func remotePlayersWithU32(source []byte, offset int, value uint32) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[offset:offset+4], remotePlayersU32(value))
	return wire
}

// remotePlayersWithFloat replaces one binary32 field of a payload at offset.
func remotePlayersWithFloat(source []byte, offset int, value float32) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[offset:offset+4], remotePlayersFloatWire(value))
	return wire
}

// remotePlayersFloatBitsText renders one binary32 as its eight-digit
// lowercase-hexadecimal bit string, the canonical float encoding both
// directions publish.
func remotePlayersFloatBitsText(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// remotePlayersIDText renders one identity as its 32-lowercase-hexadecimal
// text, so a zero or non-UUIDv4 byte sequence stays observable.
func remotePlayersIDText(id core.PlayerID) string {
	return hex.EncodeToString(id[:])
}

// remotePlayersSpawnFields renders the semantic fields one spawn publishes.
//
// The tick is a decimal string so the full u64 range stays lossless, the
// dimension is the plain wire integer, and the pose publishes as bit strings
// so a negative zero stays distinct.
func remotePlayersSpawnFields(spawn protocol.RemotePlayerSpawn) map[string]any {
	return map[string]any{
		"player_id":    remotePlayersIDText(spawn.PlayerID),
		"display_name": spawn.DisplayName,
		"server_tick":  strconv.FormatUint(spawn.ServerTick, 10),
		"dimension":    int32(spawn.Dimension),
		"position": []string{
			remotePlayersFloatBitsText(spawn.Position[0]),
			remotePlayersFloatBitsText(spawn.Position[1]),
			remotePlayersFloatBitsText(spawn.Position[2]),
		},
		"yaw":   remotePlayersFloatBitsText(spawn.Yaw),
		"pitch": remotePlayersFloatBitsText(spawn.Pitch),
	}
}

// remotePlayersDespawnFields renders the semantic fields one despawn
// publishes.
func remotePlayersDespawnFields(despawn protocol.RemotePlayerDespawn) map[string]any {
	return map[string]any{
		"player_id": remotePlayersIDText(despawn.PlayerID),
	}
}

// remotePlayersStateFields renders the semantic fields one batch record
// publishes.
func remotePlayersStateFields(state protocol.RemotePlayerState) map[string]any {
	return map[string]any{
		"player_id": remotePlayersIDText(state.PlayerID),
		"dimension": int32(state.Dimension),
		"position": []string{
			remotePlayersFloatBitsText(state.Position[0]),
			remotePlayersFloatBitsText(state.Position[1]),
			remotePlayersFloatBitsText(state.Position[2]),
		},
		"yaw":   remotePlayersFloatBitsText(state.Yaw),
		"pitch": remotePlayersFloatBitsText(state.Pitch),
		"reset": state.Reset,
	}
}

// remotePlayersStatesFields renders the semantic fields one batch publishes.
//
// The records publish in wire order, never sorted, so a batch the authority
// ordered is observed in the order it carried.
func remotePlayersStatesFields(states protocol.RemotePlayerStates) map[string]any {
	players := make([]map[string]any, 0, len(states.Players))
	for _, state := range states.Players {
		players = append(players, remotePlayersStateFields(state))
	}
	return map[string]any{
		"server_tick": strconv.FormatUint(states.ServerTick, 10),
		"players":     players,
	}
}

// remotePlayersRejectionCategory resolves the language-neutral rejection
// category for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and
// the outbound validators name, never over a sentinel identity, and a failure
// with no mapping is a hard error. The primitive answers precede the validator
// messages, because the decoder rejects a non-finite float while reading and
// only then hands the record to `Validate`.
//
// The batch messages are each their own boundary: the count bound, the
// remaining-length check, the strict-order rule and the per-record combined
// predicate. The per-record predicate is reached only by the dimension
// mutation this group registers — an infinite coordinate is answered by the
// float primitive first and a duplicate or descending identity by the order
// rule — so it resolves at the enum boundary, which is the category the Rust
// consumer publishes for the same bytes. The spawn's single combined message
// resolves as a value boundary, which is the one category the name and
// finiteness cases this group freezes share with the Rust string and float
// value variants. The despawn's message is family-specific and resolves at the
// identity boundary.
func remotePlayersRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "invalid float32"):
		return "invalid-value", true
	case strings.Contains(message, "remote player states payload exceeds 296 bytes"):
		return "capacity", true
	case strings.Contains(message, "remote player state count is outside 1..7"):
		return "invalid-value", true
	case strings.Contains(message, "remote player states exceed remaining payload"):
		return "truncated", true
	case strings.Contains(message, "remote player states are not strictly sorted"):
		return "invalid-value", true
	case strings.Contains(message, "remote player state "):
		return "invalid-enum", true
	case strings.Contains(message, "invalid remote player spawn"):
		return "invalid-value", true
	case strings.Contains(message, "invalid remote player despawn"):
		return "invalid-identity", true
	}
	return "", false
}

// remotePlayersPacketKey resolves one packet key to the Go state and numeric
// ID the codec dispatches on, and reports whether the packet travels
// server-to-client.
func remotePlayersPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := remotePlayersFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no remote player producer owns", c.ID, c.Family)
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

// newRemotePlayersCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newRemotePlayersCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// remotePlayersFieldsForPacket renders the semantic fields of one decoded
// publication packet, dispatching on the concrete DTO the production decoder
// returned.
func remotePlayersFieldsForPacket(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.RemotePlayerSpawn:
		return remotePlayersSpawnFields(message), nil
	case protocol.RemotePlayerDespawn:
		return remotePlayersDespawnFields(message), nil
	case protocol.RemotePlayerStates:
		return remotePlayersStatesFields(message), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// runRemotePlayersDecode executes one publication decode case through the real
// Go decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` and classifies
// the failure it returns. It never reimplements the length, name or ordering
// rules, so the recorded outcome is whatever the production codec decides
// about these exact bytes.
func runRemotePlayersDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := remotePlayersPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newRemotePlayersCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		category, classified := remotePlayersRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := remotePlayersFieldsForPacket(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// remotePlayersSpawnRequest is the canonical JSON field input one spawn encode
// case carries.
type remotePlayersSpawnRequest struct {
	PlayerID    string   `json:"player_id"`
	DisplayName string   `json:"display_name"`
	ServerTick  uint64   `json:"server_tick"`
	Dimension   int32    `json:"dimension"`
	Position    []string `json:"position"`
	Yaw         string   `json:"yaw"`
	Pitch       string   `json:"pitch"`
}

// core renders the request as the Go spawn DTO the encoder validates.
func (request remotePlayersSpawnRequest) core() (protocol.RemotePlayerSpawn, error) {
	id, err := remotePlayersParseID(request.PlayerID)
	if err != nil {
		return protocol.RemotePlayerSpawn{}, err
	}
	position, err := remotePlayersParseBits(request.Position)
	if err != nil {
		return protocol.RemotePlayerSpawn{}, err
	}
	yaw, err := remotePlayersParseBitsText(request.Yaw)
	if err != nil {
		return protocol.RemotePlayerSpawn{}, err
	}
	pitch, err := remotePlayersParseBitsText(request.Pitch)
	if err != nil {
		return protocol.RemotePlayerSpawn{}, err
	}
	return protocol.RemotePlayerSpawn{
		PlayerID:    id,
		DisplayName: request.DisplayName,
		ServerTick:  request.ServerTick,
		Dimension:   core.DimensionID(request.Dimension),
		Position:    mgl32.Vec3{position[0], position[1], position[2]},
		Yaw:         yaw,
		Pitch:       pitch,
	}, nil
}

// remotePlayersDespawnRequest is the canonical JSON field input one despawn
// encode case carries.
type remotePlayersDespawnRequest struct {
	PlayerID string `json:"player_id"`
}

// core renders the request as the Go despawn DTO the encoder validates.
func (request remotePlayersDespawnRequest) core() (protocol.RemotePlayerDespawn, error) {
	id, err := remotePlayersParseID(request.PlayerID)
	if err != nil {
		return protocol.RemotePlayerDespawn{}, err
	}
	return protocol.RemotePlayerDespawn{PlayerID: id}, nil
}

// remotePlayersStateRequest is the canonical JSON field input one state
// record carries.
type remotePlayersStateRequest struct {
	PlayerID  string   `json:"player_id"`
	Dimension int32    `json:"dimension"`
	Position  []string `json:"position"`
	Yaw       string   `json:"yaw"`
	Pitch     string   `json:"pitch"`
	Reset     bool     `json:"reset"`
}

// core renders the request as the Go state record DTO.
func (request remotePlayersStateRequest) core() (protocol.RemotePlayerState, error) {
	id, err := remotePlayersParseID(request.PlayerID)
	if err != nil {
		return protocol.RemotePlayerState{}, err
	}
	position, err := remotePlayersParseBits(request.Position)
	if err != nil {
		return protocol.RemotePlayerState{}, err
	}
	yaw, err := remotePlayersParseBitsText(request.Yaw)
	if err != nil {
		return protocol.RemotePlayerState{}, err
	}
	pitch, err := remotePlayersParseBitsText(request.Pitch)
	if err != nil {
		return protocol.RemotePlayerState{}, err
	}
	return protocol.RemotePlayerState{
		PlayerID:  id,
		Dimension: core.DimensionID(request.Dimension),
		Position:  mgl32.Vec3{position[0], position[1], position[2]},
		Yaw:       yaw,
		Pitch:     pitch,
		Reset:     request.Reset,
	}, nil
}

// remotePlayersStatesRequest is the canonical JSON field input one batch
// encode case carries.
type remotePlayersStatesRequest struct {
	ServerTick uint64                      `json:"server_tick"`
	Players    []remotePlayersStateRequest `json:"players"`
}

// core renders the request as the Go batch DTO the encoder validates.
func (request remotePlayersStatesRequest) core() (protocol.RemotePlayerStates, error) {
	players := make([]protocol.RemotePlayerState, 0, len(request.Players))
	for index, record := range request.Players {
		state, err := record.core()
		if err != nil {
			return protocol.RemotePlayerStates{}, fmt.Errorf("runtime-oracle: player %d: %w", index, err)
		}
		players = append(players, state)
	}
	return protocol.RemotePlayerStates{ServerTick: request.ServerTick, Players: players}, nil
}

// remotePlayersParseID decodes one 32-hexadecimal identity field.
func remotePlayersParseID(text string) (core.PlayerID, error) {
	var id core.PlayerID
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

// remotePlayersParseBits decodes one three-element bit-string array.
func remotePlayersParseBits(values []string) ([3]float32, error) {
	if len(values) != 3 {
		return [3]float32{}, fmt.Errorf("runtime-oracle: position carries %d components", len(values))
	}
	var parsed [3]float32
	for index, text := range values {
		value, err := remotePlayersParseBitsText(text)
		if err != nil {
			return parsed, err
		}
		parsed[index] = value
	}
	return parsed, nil
}

// remotePlayersParseBitsText decodes one eight-digit hexadecimal bit string.
func remotePlayersParseBitsText(text string) (float32, error) {
	bits, err := strconv.ParseUint(text, 16, 32)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: bits %q are not hexadecimal: %w", text, err)
	}
	return math.Float32frombits(uint32(bits)), nil
}

// runRemotePlayersEncode executes one publication encode case through the real
// Go encoder named by the case's own packet key and reads the result back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects
// must never be recorded as evidence.
func runRemotePlayersEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := remotePlayersPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newRemotePlayersCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ServerPacket
	switch c.Family {
	case remotePlayerSpawnFamily:
		var request remotePlayersSpawnRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		spawn, err := request.core()
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = spawn
	case remotePlayerDespawnFamily:
		var request remotePlayersDespawnRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		despawn, err := request.core()
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = despawn
	case remotePlayerStatesFamily:
		var request remotePlayersStatesRequest
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
		category, classified := remotePlayersRejectionCategory(err)
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
	fields, err := remotePlayersFieldsForPacket(c, decoded)
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

// remotePlayersCorpusRoutes is the closed route map the three families
// execute.
func remotePlayersCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range remotePlayersFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: remotePlayersVersion, Operation: "decode"}] = runRemotePlayersDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: remotePlayersVersion, Operation: "encode"}] = runRemotePlayersEncode
	}
	return routes
}

// remotePlayersCaseDefinition declares one case from literals before any
// producer runs, so the expectation is the review contract rather than a
// producer result.
type remotePlayersCaseDefinition struct {
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

// remotePlayersBitRequest renders one binary32 as its canonical JSON request
// text.
func remotePlayersBitRequest(value float32) string {
	return remotePlayersFloatBitsText(value)
}

// remotePlayersSpawnRequestFields is the canonical JSON request the valid
// spawn encode case carries, which is the reviewed wire's own field set.
func remotePlayersSpawnRequestFields() remotePlayersSpawnRequest {
	return remotePlayersSpawnRequest{
		PlayerID:    remotePlayersIDText(remotePlayersControlPlayer()),
		DisplayName: "Alice",
		ServerTick:  0,
		Dimension:   int32(core.Depths),
		Position: []string{
			remotePlayersBitRequest(remotePlayersNegZero()),
			remotePlayersBitRequest(1),
			remotePlayersBitRequest(2),
		},
		Yaw:   remotePlayersBitRequest(0),
		Pitch: remotePlayersBitRequest(2.0),
	}
}

// remotePlayersPaddedNameSpawnRequest is the invalid encode request whose only
// violation is the name rule: the same record with `" Alice "`.
func remotePlayersPaddedNameSpawnRequest() remotePlayersSpawnRequest {
	request := remotePlayersSpawnRequestFields()
	request.DisplayName = " Alice "
	return request
}

// remotePlayersDespawnRequestFields is the canonical JSON request the valid
// despawn encode case carries.
func remotePlayersDespawnRequestFields() remotePlayersDespawnRequest {
	return remotePlayersDespawnRequest{PlayerID: remotePlayersIDText(remotePlayersControlPlayer())}
}

// remotePlayersStateRequestFields renders one record as its canonical JSON
// request fields.
func remotePlayersStateRequestFields(record remotePlayersStateRecord) remotePlayersStateRequest {
	return remotePlayersStateRequest{
		PlayerID:  remotePlayersIDText(record.id),
		Dimension: record.dim,
		Position: []string{
			remotePlayersBitRequest(record.position[0]),
			remotePlayersBitRequest(record.position[1]),
			remotePlayersBitRequest(record.position[2]),
		},
		Yaw:   remotePlayersBitRequest(record.yaw),
		Pitch: remotePlayersBitRequest(record.pitch),
		Reset: record.reset,
	}
}

// remotePlayersStatesRequestFields is the canonical JSON request the valid
// batch encode case carries.
func remotePlayersStatesRequestFields() remotePlayersStatesRequest {
	records := remotePlayersCanonicalRecords()
	players := make([]remotePlayersStateRequest, 0, len(records))
	for _, record := range records {
		players = append(players, remotePlayersStateRequestFields(record))
	}
	return remotePlayersStatesRequest{ServerTick: 0, Players: players}
}

// remotePlayersSpawnDTO is the reviewed canonical spawn.
func remotePlayersSpawnDTO() protocol.RemotePlayerSpawn {
	spawn, err := remotePlayersSpawnRequestFields().core()
	if err != nil {
		panic("runtime-oracle: the reviewed spawn request is not a valid DTO: " + err.Error())
	}
	return spawn
}

// remotePlayersDespawnDTO is the reviewed canonical despawn.
func remotePlayersDespawnDTO() protocol.RemotePlayerDespawn {
	despawn, err := remotePlayersDespawnRequestFields().core()
	if err != nil {
		panic("runtime-oracle: the reviewed despawn request is not a valid DTO: " + err.Error())
	}
	return despawn
}

// remotePlayersStatesDTO is the reviewed canonical batch.
func remotePlayersStatesDTO() protocol.RemotePlayerStates {
	states, err := remotePlayersStatesRequestFields().core()
	if err != nil {
		panic("runtime-oracle: the reviewed states request is not a valid DTO: " + err.Error())
	}
	return states
}

// remotePlayersCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid
// cases use the canonical payloads the Go encoder produces for these fields,
// including the two boundary-admitting counts; the malformed decode cases
// mutate that payload at one boundary each, and the invalid encode case builds
// a DTO the production validator refuses, so each rejection names the boundary
// that owns it.
//
// The spawn's identity and dimension boundaries are deliberately absent: the
// Go validator answers them with the same message as the name and finiteness
// boundaries, so a corpus case could not distinguish them and the Rust
// group-test pins them instead. No case combines two violations.
func remotePlayersCaseDefinitions() []remotePlayersCaseDefinition {
	validSpawn := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   remotePlayersSpawnFields(remotePlayersSpawnDTO()),
	}
	validDespawn := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   remotePlayersDespawnFields(remotePlayersDespawnDTO()),
	}
	validStates := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   remotePlayersStatesFields(remotePlayersStatesDTO()),
	}
	validCountOne := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: remotePlayersStatesFields(protocol.RemotePlayerStates{
			ServerTick: 0,
			Players: []protocol.RemotePlayerState{
				func() protocol.RemotePlayerState {
					state, err := remotePlayersStateRequestFields(remotePlayersSingleRecord()[0]).core()
					if err != nil {
						panic("runtime-oracle: the reviewed count-one record is not a valid DTO: " + err.Error())
					}
					return state
				}(),
			},
		}),
	}
	validCountSeven := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: remotePlayersStatesFields(protocol.RemotePlayerStates{
			ServerTick: 0,
			Players: func() []protocol.RemotePlayerState {
				players := make([]protocol.RemotePlayerState, 0, remotePlayersMaxRecords)
				for _, record := range remotePlayersFullRecords() {
					state, err := remotePlayersStateRequestFields(record).core()
					if err != nil {
						panic("runtime-oracle: the reviewed count-seven record is not a valid DTO: " + err.Error())
					}
					players = append(players, state)
				}
				return players
			}(),
		}),
	}
	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	invalidEnum := Outcome{Kind: "error", Category: "invalid-enum"}
	invalidIdentity := Outcome{Kind: "error", Category: "invalid-identity"}
	trailing := Outcome{Kind: "error", Category: "trailing"}

	spawnWire := remotePlayersCanonicalSpawnWire()
	despawnWire := remotePlayersDespawnWire(remotePlayersControlPlayer())
	statesWire := remotePlayersStatesWire(0, remotePlayersCanonicalRecords())
	countOneWire := remotePlayersStatesWire(0, remotePlayersSingleRecord())
	countSevenWire := remotePlayersStatesWire(0, remotePlayersFullRecords())

	// The pitch is the last four bytes of a spawn payload; the position
	// begins after the identity, the name and the tick.
	const spawnPitchOffset = 50
	// A one-record batch's position begins after the tick, the count, the
	// identity and the dimension.
	const statesPositionOffset = 29

	definitions := make([]remotePlayersCaseDefinition, 0, 20)
	definitions = append(definitions,
		remotePlayersCaseDefinition{
			id:     spawnDecodeCaseID,
			family: remotePlayerSpawnFamily,
			relDir: remotePlayerSpawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), spawnWire...),
			expect: validSpawn,
		},
		remotePlayersCaseDefinition{
			id:      spawnEncodeCaseID,
			family:  remotePlayerSpawnFamily,
			relDir:  remotePlayerSpawnCorpusRelDir,
			op:      "encode",
			request: remotePlayersSpawnRequestFields(),
			wire:    append([]byte(nil), spawnWire...),
			expect:  validSpawn,
		},
		remotePlayersCaseDefinition{
			id:     spawnPaddedNameDecID,
			family: remotePlayerSpawnFamily,
			relDir: remotePlayerSpawnCorpusRelDir,
			op:     "decode",
			input:  remotePlayersPaddedNameSpawnWire(),
			expect: invalidValue,
		},
		remotePlayersCaseDefinition{
			id:      spawnPaddedNameEncID,
			family:  remotePlayerSpawnFamily,
			relDir:  remotePlayerSpawnCorpusRelDir,
			op:      "encode",
			request: remotePlayersPaddedNameSpawnRequest(),
			expect:  invalidValue,
		},
		remotePlayersCaseDefinition{
			// The NaN pitch is answered by the decoder's float primitive
			// before the validator runs, which is the boundary both sides
			// publish.
			id:     spawnNaNPitchDecID,
			family: remotePlayerSpawnFamily,
			relDir: remotePlayerSpawnCorpusRelDir,
			op:     "decode",
			input:  remotePlayersNaNPitchSpawnWire(),
			expect: invalidValue,
		},
		remotePlayersCaseDefinition{
			id:     despawnDecodeCaseID,
			family: remotePlayerDespawnFamily,
			relDir: remotePlayerDespawnCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), despawnWire...),
			expect: validDespawn,
		},
		remotePlayersCaseDefinition{
			id:      despawnEncodeCaseID,
			family:  remotePlayerDespawnFamily,
			relDir:  remotePlayerDespawnCorpusRelDir,
			op:      "encode",
			request: remotePlayersDespawnRequestFields(),
			wire:    append([]byte(nil), despawnWire...),
			expect:  validDespawn,
		},
		remotePlayersCaseDefinition{
			id:     despawnZeroUUIDDecID,
			family: remotePlayerDespawnFamily,
			relDir: remotePlayerDespawnCorpusRelDir,
			op:     "decode",
			input:  remotePlayersDespawnWire(core.PlayerID{}),
			expect: invalidIdentity,
		},
		remotePlayersCaseDefinition{
			id:     despawnTrailingID,
			family: remotePlayerDespawnFamily,
			relDir: remotePlayerDespawnCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), despawnWire...), 0x00),
			expect: trailing,
		},
		remotePlayersCaseDefinition{
			id:     statesDecodeCaseID,
			family: remotePlayerStatesFamily,
			relDir: remotePlayerStatesCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), statesWire...),
			expect: validStates,
		},
		remotePlayersCaseDefinition{
			id:      statesEncodeCaseID,
			family:  remotePlayerStatesFamily,
			relDir:  remotePlayerStatesCorpusRelDir,
			op:      "encode",
			request: remotePlayersStatesRequestFields(),
			wire:    append([]byte(nil), statesWire...),
			expect:  validStates,
		},
		remotePlayersCaseDefinition{
			// A single record is the count lower boundary, admitted beside
			// the canonical two-record vector.
			id:     statesCountOneID,
			family: remotePlayerStatesFamily,
			relDir: remotePlayerStatesCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), countOneWire...),
			expect: validCountOne,
		},
		remotePlayersCaseDefinition{
			// Seven records is the count and wire ceiling, admitted exactly
			// at the fixed payload bound.
			id:     statesCountSevenID,
			family: remotePlayerStatesFamily,
			relDir: remotePlayerStatesCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), countSevenWire...),
			expect: validCountSeven,
		},
		remotePlayersCaseDefinition{
			id:     statesCountZeroID,
			family: remotePlayerStatesFamily,
			relDir: remotePlayerStatesCorpusRelDir,
			op:     "decode",
			input:  remotePlayersStatesWireWithDeclaredCount(0, 0, nil),
			expect: invalidValue,
		},
		remotePlayersCaseDefinition{
			// A tiny payload whose declared count is above the ceiling: the
			// count bound fires before the record-length rule on both sides.
			id:     statesCountEightID,
			family: remotePlayerStatesFamily,
			relDir: remotePlayerStatesCorpusRelDir,
			op:     "decode",
			input:  remotePlayersStatesWireWithDeclaredCount(0, 8, remotePlayersSingleRecord()),
			expect: invalidValue,
		},
		remotePlayersCaseDefinition{
			id:     statesDuplicateID,
			family: remotePlayerStatesFamily,
			relDir: remotePlayerStatesCorpusRelDir,
			op:     "decode",
			input: remotePlayersStatesWire(0, []remotePlayersStateRecord{
				remotePlayersSingleRecord()[0],
				remotePlayersSingleRecord()[0],
			}),
			expect: invalidValue,
		},
		remotePlayersCaseDefinition{
			id:     statesReversedID,
			family: remotePlayerStatesFamily,
			relDir: remotePlayerStatesCorpusRelDir,
			op:     "decode",
			input: remotePlayersStatesWire(0, []remotePlayersStateRecord{
				remotePlayersStateRequestFields(remotePlayersCanonicalRecords()[1]).record(),
				remotePlayersStateRequestFields(remotePlayersCanonicalRecords()[0]).record(),
			}),
			expect: invalidValue,
		},
		remotePlayersCaseDefinition{
			// The dimension is the single violation; the identity and pose
			// stay valid so the per-record predicate fires at the dimension
			// alone.
			id:     statesDimensionTwoID,
			family: remotePlayerStatesFamily,
			relDir: remotePlayerStatesCorpusRelDir,
			op:     "decode",
			input: remotePlayersStatesWire(0, []remotePlayersStateRecord{
				remotePlayersStateRequestFields(remotePlayersSingleRecord()[0]).recordWithDimension(2),
			}),
			expect: invalidEnum,
		},
		remotePlayersCaseDefinition{
			// One infinite position word is answered by the float primitive
			// before the validator runs.
			id:     statesInfiniteCoordID,
			family: remotePlayerStatesFamily,
			relDir: remotePlayerStatesCorpusRelDir,
			op:     "decode",
			input: remotePlayersWithFloat(countOneWire, statesPositionOffset,
				float32(math.Inf(1))),
			expect: invalidValue,
		},
		remotePlayersCaseDefinition{
			id:     statesTrailingID,
			family: remotePlayerStatesFamily,
			relDir: remotePlayerStatesCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), statesWire...), 0x00),
			expect: trailing,
		},
	)

	return definitions
}

// record renders one request back as its wire record shape.
func (request remotePlayersStateRequest) record() remotePlayersStateRecord {
	return remotePlayersStateRecord{
		id:       remotePlayersControlPlayerFromText(request.PlayerID),
		dim:      request.Dimension,
		position: remotePlayersPositionFromText(request.Position),
		yaw:      remotePlayersAngleFromText(request.Yaw),
		pitch:    remotePlayersAngleFromText(request.Pitch),
		reset:    request.Reset,
	}
}

// recordWithDimension renders one request's record with a replaced dimension.
func (request remotePlayersStateRequest) recordWithDimension(dimension int32) remotePlayersStateRecord {
	record := request.record()
	record.dim = dimension
	return record
}

// remotePlayersControlPlayerFromText decodes one identity text for the wire
// builders.
func remotePlayersControlPlayerFromText(text string) core.PlayerID {
	id, err := remotePlayersParseID(text)
	if err != nil {
		panic("runtime-oracle: reviewed identity is not hexadecimal: " + err.Error())
	}
	return id
}

// remotePlayersPositionFromText decodes one position bit-string array for the
// wire builders.
func remotePlayersPositionFromText(values []string) [3]float32 {
	position, err := remotePlayersParseBits(values)
	if err != nil {
		panic("runtime-oracle: reviewed position is not hexadecimal: " + err.Error())
	}
	return position
}

// remotePlayersAngleFromText decodes one angle bit string for the wire
// builders.
func remotePlayersAngleFromText(text string) float32 {
	value, err := remotePlayersParseBitsText(text)
	if err != nil {
		panic("runtime-oracle: reviewed angle is not hexadecimal: " + err.Error())
	}
	return value
}

// remotePlayersLabel renders one case's asset label from its identity.
func remotePlayersLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// buildRemotePlayersCandidate builds one case's manifest entry and asset bytes
// from its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildRemotePlayersCandidate(t *testing.T, definition remotePlayersCaseDefinition) remotePlayersCandidate {
	t.Helper()

	label := remotePlayersLabel(definition.id)
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
		Version:      remotePlayersVersion,
		Operation:    definition.op,
		PacketKey:    remotePlayersKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return remotePlayersCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// remotePlayersKeyPointer resolves one family's reviewed packet key for a case
// spec.
func remotePlayersKeyPointer(family string) *PacketKeySpec {
	key, owned := remotePlayersFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// remotePlayersCandidate is one reviewed case: its manifest specification, the
// exact asset bytes it publishes, and the expectation an independent execution
// has to reproduce.
type remotePlayersCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which
	// the producer's own bytes are compared against.
	Wire []byte
}

// remotePlayersCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func remotePlayersCandidates(t *testing.T, root string) []remotePlayersCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := remotePlayersCaseDefinitions()
	candidates := make([]remotePlayersCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildRemotePlayersCandidate(t, definition)
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

// remotePlayersRegisteredCase reports whether the base manifest already
// carries one case identity, so a re-merge of an integrated candidate adds
// nothing.
func remotePlayersRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// remotePlayersSelection is this group's registration: its cases, the Go
// sources its rules are read from, and the routes the three families execute.
func remotePlayersSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := remotePlayersCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range remotePlayersFamilies() {
		sources[family.id] = remotePlayersServerSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(remotePlayersFamilies())*2)
	for _, family := range remotePlayersFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: remotePlayersVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: remotePlayersVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  remotePlayersProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// remotePlayersServerSources lists the Go sources each family reads its rules
// from: the shared server codec dispatch with its decode arms and the payload
// ceiling, and the family's own message file, which owns the validator the
// shared dispatch applies at the tail.
func remotePlayersServerSources(family string) []string {
	return []string{
		"packages/shared/network/codec/codec_server.go",
		"packages/shared/network/protocol/message_player.go",
	}
}

// remotePlayersManifest assembles the family-scoped selection the route runner
// executes: this group's candidates with every other family cleared, so
// reconciliation accepts the scoped manifest.
func remotePlayersManifest(t *testing.T, root string, candidates []remotePlayersCandidate) Inventory {
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
		if !remotePlayersOwnsFamily(family) {
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
		t.Fatalf("encode remote players working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write remote players working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load remote players working manifest: %v", err)
	}
	return loaded
}

// remotePlayersOwnsFamily reports whether this group registers cases for one
// family.
func remotePlayersOwnsFamily(family string) bool {
	_, owned := remotePlayersFamilyKeys[family]
	return owned
}

// remotePlayersScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve
// under the root the runner is given.
func remotePlayersScratchRoot(t *testing.T, root string, candidates []remotePlayersCandidate) string {
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

// remotePlayersCandidateByID resolves one candidate by its case identity.
func remotePlayersCandidateByID(t *testing.T, candidates []remotePlayersCandidate, id string) remotePlayersCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return remotePlayersCandidate{}
}

// remotePlayersObservation resolves one executed observation by its case
// identity.
func remotePlayersObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// remotePlayersExportPublished guards the single publication per test
// process, because the exporter's producer child is create-exclusive and more
// than one test in this package observes the same candidates.
var remotePlayersExportPublished bool

// remotePlayersCandidatesExport publishes the reviewed candidates and the
// complete merged manifest candidate through the existing external exporter
// and returns the published producer directory. An unset export variable
// publishes nothing and returns "", so an ordinary test run never writes
// outside its own temporary storage.
func remotePlayersCandidatesExport(t *testing.T, root string, candidates []remotePlayersCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if remotePlayersExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-remote-players")
	}
	remotePlayersExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, remotePlayersProducerID, assets)
	if err != nil {
		t.Fatalf("export remote players candidates: %v", err)
	}
	return published
}

// remotePlayersDecodeCaseIDs lists the valid decode cases the producer pins.
func remotePlayersDecodeCaseIDs() []string {
	return []string{
		spawnDecodeCaseID,
		despawnDecodeCaseID,
		statesDecodeCaseID,
		statesCountOneID,
		statesCountSevenID,
	}
}

// remotePlayersEncodeCaseIDs lists the valid encode cases the producer pins.
func remotePlayersEncodeCaseIDs() []string {
	return []string{
		spawnEncodeCaseID,
		despawnEncodeCaseID,
		statesEncodeCaseID,
	}
}

// remotePlayersRejectedDecodeCaseIDs lists the malformed decode cases.
func remotePlayersRejectedDecodeCaseIDs() []string {
	return []string{
		spawnPaddedNameDecID,
		spawnNaNPitchDecID,
		despawnZeroUUIDDecID,
		despawnTrailingID,
		statesCountZeroID,
		statesCountEightID,
		statesDuplicateID,
		statesReversedID,
		statesDimensionTwoID,
		statesInfiniteCoordID,
		statesTrailingID,
	}
}

// remotePlayersRejectedEncodeCaseIDs lists the invalid encode cases.
func remotePlayersRejectedEncodeCaseIDs() []string {
	return []string{spawnPaddedNameEncID}
}

// TestProtocolRemotePlayersOracleDecodesEveryValidCase pins that the real Go
// decoder publishes the canonical fields for every family's valid decode case,
// including the two boundary-admitting counts.
func TestProtocolRemotePlayersOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := remotePlayersCandidates(t, root)

	for _, id := range remotePlayersDecodeCaseIDs() {
		candidate := remotePlayersCandidateByID(t, candidates, id)
		outcome, encoded, err := runRemotePlayersDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runRemotePlayersDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolRemotePlayersOracleEncodesCanonicalWire pins each encode producer
// against the reviewed wire literal and against the recorded digest.
func TestProtocolRemotePlayersOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := remotePlayersCandidates(t, root)

	for _, id := range remotePlayersEncodeCaseIDs() {
		candidate := remotePlayersCandidateByID(t, candidates, id)
		outcome, encoded, err := runRemotePlayersEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runRemotePlayersEncode(%s): %v", id, err)
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

// TestProtocolRemotePlayersOracleRejectsMalformedCasesAtTheirBoundary pins
// that every malformed decode case and the invalid encode case are refused by
// the production codec and classified at the boundary that owns it.
func TestProtocolRemotePlayersOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := remotePlayersCandidates(t, root)

	for _, id := range remotePlayersRejectedDecodeCaseIDs() {
		candidate := remotePlayersCandidateByID(t, candidates, id)
		outcome, encoded, err := runRemotePlayersDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runRemotePlayersDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range remotePlayersRejectedEncodeCaseIDs() {
		candidate := remotePlayersCandidateByID(t, candidates, id)
		outcome, encoded, err := runRemotePlayersEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runRemotePlayersEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolRemotePlayersOracleExpectedFieldMutationFailsComparison pins
// that the recorded expectation is a commitment: replacing an expected field
// fails comparison against what the producer decoded, and replacing the
// encoded digest fails comparison against the reviewed wire.
func TestProtocolRemotePlayersOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := remotePlayersCandidates(t, root)

	decode := remotePlayersCandidateByID(t, candidates, spawnDecodeCaseID)
	produced, _, err := runRemotePlayersDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runRemotePlayersDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	// The pitch is the contract: a normalization that published the number
	// instead of the bit string would hide the wire-valid 2.0 pitch this
	// vector carries.
	fields["pitch"] = remotePlayersFloatBitsText(1.0)
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated pitch compares equal to the produced outcome")
	}

	encode := remotePlayersCandidateByID(t, candidates, statesEncodeCaseID)
	producedEncode, _, err := runRemotePlayersEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runRemotePlayersEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolRemotePlayersOracleRoutesExecuteEveryCase executes this group's
// complete case set through the shared packet-case runner, once per case, and
// compares every observation with the reviewed expectation.
func TestProtocolRemotePlayersOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := remotePlayersCandidates(t, root)
	manifest := remotePlayersManifest(t, root, candidates)
	staged := remotePlayersScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, remotePlayersCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := remotePlayersObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolRemotePlayersOracleRunnerRejectsUnregisteredRoute pins that a
// case naming a route this group does not claim fails before its producer
// runs, so a case cannot claim coverage from its name alone.
func TestProtocolRemotePlayersOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := remotePlayersCandidates(t, root)
	manifest := remotePlayersManifest(t, root, candidates)
	staged := remotePlayersScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range remotePlayersFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: remotePlayersVersion, Operation: "decode"}] = runRemotePlayersDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range remotePlayersFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+remotePlayersVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolRemotePlayersOracleManifestMergeRegistersRemotePlayerRoutes
// pins that the merged manifest registers all three families' routes and case
// lists, leaves the source revision alone, and records this group's
// provenance sources.
func TestProtocolRemotePlayersOracleManifestMergeRegistersRemotePlayerRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, remotePlayersSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range remotePlayersFamilies() {
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
		for _, want := range remotePlayersServerSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so
	// a re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range remotePlayersCandidates(t, root) {
		if !remotePlayersRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolRemotePlayersOracleCandidatesExportForReview publishes the
// reviewed candidates and the manifest candidate. An unset export variable
// publishes nothing, so the tracked corpus is never written by this package.
func TestProtocolRemotePlayersOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := remotePlayersCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), remotePlayersSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := remotePlayersCandidatesExport(t, root, candidates, merged)
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

// TestProtocolRemotePlayersGoEncoderRefusesPaddedName pins that the production
// encoder runs the outbound validator before it writes the name slot, so a
// padded name never becomes a silently published value.
//
// The Rust side publishes the same bytes silently before this node's surface
// conversion, which is the behavioral red the fallible surface closes; the Go
// encoder has always refused the DTO, and the corpus case records that refusal
// so the two implementations stay pinned to one boundary.
func TestProtocolRemotePlayersGoEncoderRefusesPaddedName(t *testing.T) {
	wireCodec, err := newRemotePlayersCodec()
	if err != nil {
		t.Fatalf("newRemotePlayersCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	spawn := remotePlayersSpawnDTO()
	spawn.DisplayName = " Alice "
	_, _, err = wireCodec.EncodeServer(protocol.StatePlay, spawn)
	if err == nil {
		t.Fatal("the Go encoder published a padded remote player name")
	}
	if !strings.Contains(err.Error(), "invalid remote player spawn") {
		t.Fatalf("the Go encoder refused the padded name with %v", err)
	}
	// The silent wire the unvalidated Rust encoder would have published
	// before this node: the same name bytes behind the length prefix.
	silent := remotePlayersPaddedNameSpawnWire()
	name := " Alice "
	if !bytes.Equal(silent[16:17], []byte{byte(len(name))}) || !bytes.Equal(silent[17:17+len(name)], []byte(name)) {
		t.Fatalf("the silent wire carries %x", silent[16:17+len(name)])
	}
	if len(silent) != 56 {
		t.Fatalf("the silent wire is %d bytes, want 56", len(silent))
	}
}
