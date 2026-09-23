package main

// This file is the player and private outcome packet producer group: the four
// Play server-to-client families that are addressed to the owning session
// alone — `PlayerState` (the fixed 93-byte body, survival and world-time
// record), `CommandRejected` (a sequence plus one reject reason byte),
// `PlaceBlockSucceeded` (the sequence the authority acknowledged) and
// `CombatHit` (the fixed 10-byte melee confirmation) — each register a decode
// and an encode route through the shared packet-case runner, so every case is
// executed by the real Go codec rather than restated here.
//
// The decode cases are the Go decoder's own bytes: the canonical vectors, the
// active-mining variant, one mutated payload at each validator boundary, the
// wire reject-reason boundaries, the combat-hit range and kind boundaries, one
// trailing byte and one proper truncation. The encode cases carry canonical
// JSON fields and either the reviewed wire the Go encoder has to publish or a
// DTO the outbound validator has to refuse. Every negative carries exactly one
// violation. No case in this group declares a routing recipient, a session, a
// subscription or a damage settlement: per-session publication is a later
// authority concern and stays outside the protocol contract.
//
// The reject reason is carried as its frozen wire number, never as an internal
// enum cast: the Go internal enum runs 0..14 while the wire enum runs 1..15,
// so the producer publishes and requests the wire value and the Rust consumer
// owns the explicit bidirectional translation the group test pins.

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

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// playerStateFamily is the play player state family this group registers.
	playerStateFamily = "protocol.server.PlayerState"
	// commandRejectedFamily is the play command reject family this group
	// registers.
	commandRejectedFamily = "protocol.server.CommandRejected"
	// placeBlockSucceededFamily is the play place block success family this
	// group registers.
	placeBlockSucceededFamily = "protocol.server.PlaceBlockSucceeded"
	// combatHitFamily is the play combat hit family this group registers.
	combatHitFamily = "protocol.server.CombatHit"
	// playerOutcomesVersion is the protocol version all four families are
	// pinned to.
	playerOutcomesVersion = "45"
	// playerOutcomesProducerID is the exporter's producer identity for this
	// group's candidate assets and merged manifest.
	playerOutcomesProducerID = "runtime-oracle/protocol-player-outcomes"

	// playerStateCorpusRelDir is the repository-relative directory holding the
	// player state family's committed case assets.
	playerStateCorpusRelDir = corpusCasesRelDir + "/protocol/PlayerState"
	// commandRejectedCorpusRelDir is the repository-relative directory holding
	// the command rejected family's committed case assets.
	commandRejectedCorpusRelDir = corpusCasesRelDir + "/protocol/CommandRejected"
	// placeBlockSucceededCorpusRelDir is the repository-relative directory
	// holding the place block succeeded family's committed case assets.
	placeBlockSucceededCorpusRelDir = corpusCasesRelDir + "/protocol/PlaceBlockSucceeded"
	// combatHitCorpusRelDir is the repository-relative directory holding the
	// combat hit family's committed case assets.
	combatHitCorpusRelDir = corpusCasesRelDir + "/protocol/CombatHit"

	// playerStateWireBytes is the fixed stride of the whole payload.
	playerStateWireBytes = 93
	// commandRejectedWireBytes is the fixed stride: sequence plus reason.
	commandRejectedWireBytes = 9
	// placeBlockSucceededWireBytes is the fixed stride: sequence alone.
	placeBlockSucceededWireBytes = 8
	// combatHitWireBytes is the fixed stride: tick, damage and target kind.
	combatHitWireBytes = 10
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	playerStateDecodeCaseID      = playerStateFamily + "/" + playerOutcomesVersion + "/decode-valid"
	playerStateEncodeCaseID      = playerStateFamily + "/" + playerOutcomesVersion + "/encode-valid"
	playerStateActiveMiningID    = playerStateFamily + "/" + playerOutcomesVersion + "/decode-active-mining"
	playerStateMiningInvalidID   = playerStateFamily + "/" + playerOutcomesVersion + "/decode-mining-combination-invalid"
	playerStateNaNPositionID     = playerStateFamily + "/" + playerOutcomesVersion + "/decode-nan-position"
	playerStateHealthAboveID     = playerStateFamily + "/" + playerOutcomesVersion + "/decode-health-above"
	playerStateHealthAboveEncID  = playerStateFamily + "/" + playerOutcomesVersion + "/encode-health-above"
	playerStateOxygenAboveID     = playerStateFamily + "/" + playerOutcomesVersion + "/decode-oxygen-above"
	playerStateHungerAboveID     = playerStateFamily + "/" + playerOutcomesVersion + "/decode-hunger-above"
	playerStateDayOffsetAboveID  = playerStateFamily + "/" + playerOutcomesVersion + "/decode-day-offset-above"
	playerStateWeatherThreeID    = playerStateFamily + "/" + playerOutcomesVersion + "/decode-weather-three"
	playerStateSeasonFourID      = playerStateFamily + "/" + playerOutcomesVersion + "/decode-season-four"
	playerStateArmorAboveID      = playerStateFamily + "/" + playerOutcomesVersion + "/decode-armor-above"
	playerStateTrailingID        = playerStateFamily + "/" + playerOutcomesVersion + "/decode-trailing-byte"
	commandRejectedDecodeID      = commandRejectedFamily + "/" + playerOutcomesVersion + "/decode-valid"
	commandRejectedEncodeID      = commandRejectedFamily + "/" + playerOutcomesVersion + "/encode-valid"
	commandRejectedReasonHighID  = commandRejectedFamily + "/" + playerOutcomesVersion + "/decode-reason-fifteen"
	commandRejectedReasonZeroID  = commandRejectedFamily + "/" + playerOutcomesVersion + "/decode-reason-zero"
	commandRejectedReasonZeroEnc = commandRejectedFamily + "/" + playerOutcomesVersion + "/encode-reason-zero"
	commandRejectedReasonSixteen = commandRejectedFamily + "/" + playerOutcomesVersion + "/decode-reason-sixteen"
	placeBlockSucceededDecodeID  = placeBlockSucceededFamily + "/" + playerOutcomesVersion + "/decode-valid"
	placeBlockSucceededEncodeID  = placeBlockSucceededFamily + "/" + playerOutcomesVersion + "/encode-valid"
	placeBlockSucceededTruncID   = placeBlockSucceededFamily + "/" + playerOutcomesVersion + "/decode-truncated"
	placeBlockSucceededTrailing  = placeBlockSucceededFamily + "/" + playerOutcomesVersion + "/decode-trailing-byte"
	combatHitDecodeID            = combatHitFamily + "/" + playerOutcomesVersion + "/decode-valid"
	combatHitEncodeID            = combatHitFamily + "/" + playerOutcomesVersion + "/encode-valid"
	combatHitKindThreeID         = combatHitFamily + "/" + playerOutcomesVersion + "/decode-kind-three"
	combatHitTickZeroID          = combatHitFamily + "/" + playerOutcomesVersion + "/decode-tick-zero"
	combatHitDamageZeroID        = combatHitFamily + "/" + playerOutcomesVersion + "/decode-damage-zero"
	combatHitDamageAboveID       = combatHitFamily + "/" + playerOutcomesVersion + "/decode-damage-above"
	combatHitKindZeroID          = combatHitFamily + "/" + playerOutcomesVersion + "/decode-kind-zero"
	combatHitKindFourID          = combatHitFamily + "/" + playerOutcomesVersion + "/decode-kind-four"
)

// playerOutcomesFamilyKeys pins each family's complete packet key. A case
// names its key instead of trusting its family, so a case registered under the
// wrong family, state or ID fails before the codec runs. All four families are
// server-to-client play packets.
var playerOutcomesFamilyKeys = map[string]PacketKeySpec{
	playerStateFamily:         {Direction: packetDirectionServer, State: packetStatePlay, ID: 3},
	commandRejectedFamily:     {Direction: packetDirectionServer, State: packetStatePlay, ID: 4},
	placeBlockSucceededFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 20},
	combatHitFamily:           {Direction: packetDirectionServer, State: packetStatePlay, ID: 25},
}

// playerOutcomesFamilies lists the four families in registry order, with the
// corpus directory that holds each family's assets. The order is the review
// order, not a dispatch table.
func playerOutcomesFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{playerStateFamily, playerStateCorpusRelDir},
		{commandRejectedFamily, commandRejectedCorpusRelDir},
		{placeBlockSucceededFamily, placeBlockSucceededCorpusRelDir},
		{combatHitFamily, combatHitCorpusRelDir},
	}
}

// playerOutcomesU64 renders one little-endian u64 field.
func playerOutcomesU64(value uint64) []byte {
	wire := make([]byte, 8)
	for index := range wire {
		wire[index] = byte(value >> (8 * index))
	}
	return wire
}

// playerOutcomesU16 renders one little-endian u16 field.
func playerOutcomesU16(value uint16) []byte {
	return []byte{byte(value), byte(value >> 8)}
}

// playerOutcomesI32 renders one little-endian i32 field.
func playerOutcomesI32(value int32) []byte {
	return playerOutcomesU64(uint64(value))[:4]
}

// playerOutcomesBool renders one boolean field the Go primitive accepts.
func playerOutcomesBool(value bool) []byte {
	if value {
		return []byte{0x01}
	}
	return []byte{0x00}
}

// playerStateVector is one PlayerState payload in the Go encoder's field
// order, so a mutation of one field is a mutation of one boundary.
type playerStateVector struct {
	serverTick        uint64
	lastInputSequence uint64
	dimension         int32
	position          [3]uint32
	velocity          [3]uint32
	yaw               uint32
	pitch             uint32
	onGround          bool
	ready             bool
	reset             bool
	miningActive      bool
	miningTarget      [3]int32
	miningProgress    uint16
	miningRequired    uint16
	miningHarvestable bool
	health            uint8
	oxygen            uint16
	hunger            uint8
	saturationZero    bool
	dayPhaseOffset    uint16
	worldTimeTicks    uint64
	weather           uint8
	season            uint8
	seasonProgress    uint8
	temperature       int8
	armor             uint8
}

// wire renders the vector as the 93-byte payload the encoder publishes.
func (v playerStateVector) wire() []byte {
	wire := make([]byte, 0, playerStateWireBytes)
	wire = append(wire, playerOutcomesU64(v.serverTick)...)
	wire = append(wire, playerOutcomesU64(v.lastInputSequence)...)
	wire = append(wire, playerOutcomesI32(v.dimension)...)
	for _, bits := range v.position {
		wire = append(wire, playerOutcomesU64(uint64(bits))[:4]...)
	}
	for _, bits := range v.velocity {
		wire = append(wire, playerOutcomesU64(uint64(bits))[:4]...)
	}
	wire = append(wire, playerOutcomesU64(uint64(v.yaw))[:4]...)
	wire = append(wire, playerOutcomesU64(uint64(v.pitch))[:4]...)
	wire = append(wire, playerOutcomesBool(v.onGround)...)
	wire = append(wire, playerOutcomesBool(v.ready)...)
	wire = append(wire, playerOutcomesBool(v.reset)...)
	wire = append(wire, playerOutcomesBool(v.miningActive)...)
	for _, coordinate := range v.miningTarget {
		wire = append(wire, playerOutcomesI32(coordinate)...)
	}
	wire = append(wire, playerOutcomesU16(v.miningProgress)...)
	wire = append(wire, playerOutcomesU16(v.miningRequired)...)
	wire = append(wire, playerOutcomesBool(v.miningHarvestable)...)
	wire = append(wire, v.health)
	wire = append(wire, playerOutcomesU16(v.oxygen)...)
	wire = append(wire, v.hunger)
	wire = append(wire, playerOutcomesBool(v.saturationZero)...)
	wire = append(wire, playerOutcomesU16(v.dayPhaseOffset)...)
	wire = append(wire, playerOutcomesU64(v.worldTimeTicks)...)
	wire = append(wire, v.weather, v.season, v.seasonProgress, byte(v.temperature), v.armor)
	return wire
}

// playerStateCanonical is the reviewed canonical vector: zero server tick,
// input sequence and world time, dimension 1 (Depths), a negative-zero
// position X and pitch, an inactive mining block with an exact-zero target,
// progress, requirement and harvestable flag, and every named boundary
// maximum — health 20, oxygen 300, hunger 20, day phase offset 23999,
// weather 2, season 3, in-season progress 255, temperature −128 and armor
// points 20.
func playerStateCanonical() playerStateVector {
	return playerStateVector{
		dimension:      1,
		position:       [3]uint32{0x8000_0000, 0, 0},
		pitch:          0x8000_0000,
		health:         20,
		oxygen:         300,
		hunger:         20,
		dayPhaseOffset: 23999,
		weather:        2,
		season:         3,
		seasonProgress: 255,
		temperature:    -128,
		armor:          20,
	}
}

// playerStateActiveMining is the active mining variant: a non-zero target,
// progress 1 below requirement 2 and a true harvestable flag, on the same
// canonical survival and world-time values.
func playerStateActiveMining() playerStateVector {
	active := playerStateCanonical()
	active.miningActive = true
	active.miningTarget = [3]int32{1, 2, 3}
	active.miningProgress = 1
	active.miningRequired = 2
	active.miningHarvestable = true
	return active
}

// playerOutcomesCommandRejectedWire renders one CommandRejected payload: the
// little-endian sequence and the one-byte wire reason.
func playerOutcomesCommandRejectedWire(sequence uint64, reason uint8) []byte {
	return joinWire(playerOutcomesU64(sequence), []byte{reason})
}

// playerOutcomesPlaceBlockSucceededWire renders one PlaceBlockSucceeded
// payload: the little-endian sequence alone.
func playerOutcomesPlaceBlockSucceededWire(sequence uint64) []byte {
	return playerOutcomesU64(sequence)
}

// playerOutcomesCombatHitWire renders one CombatHit payload: the
// little-endian server tick, the damage byte and the target kind byte.
func playerOutcomesCombatHitWire(serverTick uint64, damage, targetKind uint8) []byte {
	return joinWire(playerOutcomesU64(serverTick), []byte{damage, targetKind})
}

// playerOutcomesPacketKey resolves one packet key to the Go state and numeric
// ID the codec dispatches on. All four families are server-to-client play
// packets, so a case naming another direction fails before the codec runs.
func playerOutcomesPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := playerOutcomesFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no player outcomes producer owns", c.ID, c.Family)
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

// newPlayerOutcomesCodec builds the production codec one producer call uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newPlayerOutcomesCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// playerStateFields renders the semantic fields one decoded PlayerState
// publishes.
//
// The fields come from the DTO the production decoder returned, never from the
// case input. The ticks and the sequences render as decimal strings so the
// full u64 range stays lossless, the floating-point values render as their
// eight-digit hexadecimal bit strings so a negative zero survives the round
// trip, the mining target renders as its ordered integer triple, and every
// closed enum renders as the declared integer it carries.
func playerStateFields(c CaseSpec, packet any) (map[string]any, error) {
	message, ok := packet.(protocol.PlayerState)
	if !ok {
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
	return map[string]any{
		"server_tick":           strconv.FormatUint(message.ServerTick, 10),
		"last_input_sequence":   strconv.FormatUint(message.LastInputSequence, 10),
		"dimension":             int32(message.Dimension),
		"position":              playerOutcomesVec3Bits(message.Position),
		"velocity":              playerOutcomesVec3Bits(message.Velocity),
		"yaw":                   float32BitsHex(message.Yaw),
		"pitch":                 float32BitsHex(message.Pitch),
		"on_ground":             message.OnGround,
		"ready":                 message.Ready,
		"reset":                 message.Reset,
		"mining_active":         message.MiningActive,
		"mining_target":         playerOutcomesBlockPos(message.MiningTarget),
		"mining_progress_ticks": message.MiningProgressTicks,
		"mining_required_ticks": message.MiningRequiredTicks,
		"mining_harvestable":    message.MiningHarvestable,
		"health":                message.Health,
		"oxygen":                message.Oxygen,
		"hunger":                message.Hunger,
		"saturation_zero":       message.SaturationZero,
		"day_phase_offset":      message.DayPhaseOffset,
		"world_time_ticks":      strconv.FormatUint(message.WorldTimeTicks, 10),
		"weather_kind":          uint8(message.WeatherKind),
		"season":                uint8(message.Season),
		"season_progress":       message.SeasonProgress,
		"temperature":           message.Temperature,
		"armor_points":          message.ArmorPoints,
	}, nil
}

// playerOutcomesVec3Bits renders one vector's three components as their bit
// strings, which is the canonical field encoding the Rust consumer publishes.
func playerOutcomesVec3Bits(vector mgl32.Vec3) []string {
	return []string{
		float32BitsHex(vector[0]),
		float32BitsHex(vector[1]),
		float32BitsHex(vector[2]),
	}
}

// playerOutcomesBlockPos renders one block position as its ordered integer
// triple.
func playerOutcomesBlockPos(position core.BlockPos) map[string]any {
	return map[string]any{
		"x": position.X,
		"y": position.Y,
		"z": position.Z,
	}
}

// playerOutcomesCommandRejectedFields renders the semantic fields one decoded
// CommandRejected publishes: the sequence as a decimal string and the reason
// as its frozen wire number, which is the value both implementations answer
// with rather than an internal enum cast.
func playerOutcomesCommandRejectedFields(message protocol.CommandRejected) map[string]any {
	reason, _ := protocol.CommandRejectReasonID(message.Reason)
	return map[string]any{
		"sequence": strconv.FormatUint(message.Sequence, 10),
		"reason":   reason,
	}
}

// playerOutcomesPlaceBlockSucceededFields renders the semantic fields one
// decoded PlaceBlockSucceeded publishes.
func playerOutcomesPlaceBlockSucceededFields(message protocol.PlaceBlockSucceeded) map[string]any {
	return map[string]any{
		"sequence": strconv.FormatUint(message.Sequence, 10),
	}
}

// playerOutcomesCombatHitFields renders the semantic fields one decoded
// CombatHit publishes: the server tick as a decimal string and the damage and
// target kind as the plain integers the wire carries.
func playerOutcomesCombatHitFields(message protocol.CombatHit) map[string]any {
	return map[string]any{
		"server_tick": strconv.FormatUint(message.ServerTick, 10),
		"damage":      message.Damage,
		"target_kind": uint8(message.TargetKind),
	}
}

// playerOutcomesFields renders the semantic fields one decoded private
// outcome packet publishes.
func playerOutcomesFields(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.PlayerState:
		return playerStateFields(c, message)
	case protocol.CommandRejected:
		return playerOutcomesCommandRejectedFields(message), nil
	case protocol.PlaceBlockSucceeded:
		return playerOutcomesPlaceBlockSucceededFields(message), nil
	case protocol.CombatHit:
		return playerOutcomesCombatHitFields(message), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// playerOutcomesRejectionCategory resolves the language-neutral rejection
// category for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and
// the outbound validators name, never over a sentinel identity, and a failure
// with no mapping is a hard error. The dimension, weather, season, reject
// reason and combat target kind violations are the invalid-enum boundaries
// their closed domains own; every health, oxygen, hunger, day phase, armor,
// mining-union and combat range violation is the invalid-value boundary, and
// so is the float primitive's non-finite rejection, which the decoder answers
// before the validator's own non-finite messages can run; the short-input
// primitive and the exact-length combat check map to the truncated category;
// one trailing byte keeps its own category. The two directions publish one
// category for the same bytes, which is what the explicit reason translation
// on the Rust side preserves.
func playerOutcomesRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "dimension is not overworld or depths"),
		strings.Contains(message, "out-of-range weather"),
		strings.Contains(message, "out-of-range season"),
		strings.Contains(message, "unknown command rejection reason"),
		strings.Contains(message, "combat hit target kind"):
		return "invalid-enum", true
	case strings.Contains(message, "non-finite position"),
		strings.Contains(message, "non-finite velocity"),
		strings.Contains(message, "non-finite rotation"),
		strings.Contains(message, "invalid float32"),
		strings.Contains(message, "out-of-range health"),
		strings.Contains(message, "out-of-range oxygen"),
		strings.Contains(message, "out-of-range hunger"),
		strings.Contains(message, "out-of-range day phase offset"),
		strings.Contains(message, "out-of-range armor points"),
		strings.Contains(message, "inactive player state has mining fields"),
		strings.Contains(message, "active player state has invalid mining progress"),
		strings.Contains(message, "combat hit server tick is zero"),
		strings.Contains(message, "combat hit damage"):
		return "invalid-value", true
	case strings.Contains(message, "combat hit payload must be exactly 10 bytes"),
		strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	}
	return "", false
}

// runPlayerOutcomesDecode executes one private outcome decode case through
// the real Go decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` and classifies
// the failure it returns. It never reimplements the length, enum, range or
// trailing-byte rules, so the recorded outcome is whatever the production
// codec decides about these exact bytes.
func runPlayerOutcomesDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := playerOutcomesPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newPlayerOutcomesCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		category, classified := playerOutcomesRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := playerOutcomesFields(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// playerStateEncodeRequest is the canonical JSON field input one PlayerState
// encode case carries. The floating-point values are bit strings, the ticks
// and sequences are decimal JSON integers the full u64 range survives, and
// the mining target is its ordered integer triple.
type playerStateEncodeRequest struct {
	ServerTick        uint64                 `json:"server_tick"`
	LastInputSequence uint64                 `json:"last_input_sequence"`
	Dimension         int32                  `json:"dimension"`
	Position          []string               `json:"position"`
	Velocity          []string               `json:"velocity"`
	Yaw               string                 `json:"yaw"`
	Pitch             string                 `json:"pitch"`
	OnGround          bool                   `json:"on_ground"`
	Ready             bool                   `json:"ready"`
	Reset             bool                   `json:"reset"`
	MiningActive      bool                   `json:"mining_active"`
	MiningTarget      playerOutcomesPosField `json:"mining_target"`
	MiningProgress    uint16                 `json:"mining_progress_ticks"`
	MiningRequired    uint16                 `json:"mining_required_ticks"`
	MiningHarvestable bool                   `json:"mining_harvestable"`
	Health            uint8                  `json:"health"`
	Oxygen            uint16                 `json:"oxygen"`
	Hunger            uint8                  `json:"hunger"`
	SaturationZero    bool                   `json:"saturation_zero"`
	DayPhaseOffset    uint16                 `json:"day_phase_offset"`
	WorldTimeTicks    uint64                 `json:"world_time_ticks"`
	Weather           uint8                  `json:"weather_kind"`
	Season            uint8                  `json:"season"`
	SeasonProgress    uint8                  `json:"season_progress"`
	Temperature       int8                   `json:"temperature"`
	Armor             uint8                  `json:"armor_points"`
}

// playerOutcomesPosField is one integer triple's canonical JSON fields.
type playerOutcomesPosField struct {
	X int32 `json:"x"`
	Y int32 `json:"y"`
	Z int32 `json:"z"`
}

// commandRejectedEncodeRequest is the canonical JSON field input one
// CommandRejected encode case carries. The reason is the frozen wire number,
// never an internal enum name.
type commandRejectedEncodeRequest struct {
	Sequence uint64 `json:"sequence"`
	Reason   uint8  `json:"reason"`
}

// placeBlockSucceededEncodeRequest is the canonical JSON field input one
// PlaceBlockSucceeded encode case carries.
type placeBlockSucceededEncodeRequest struct {
	Sequence uint64 `json:"sequence"`
}

// combatHitEncodeRequest is the canonical JSON field input one CombatHit
// encode case carries.
type combatHitEncodeRequest struct {
	ServerTick uint64 `json:"server_tick"`
	Damage     uint8  `json:"damage"`
	TargetKind uint8  `json:"target_kind"`
}

// playerOutcomesVec3 decodes one three-component bit-string vector, so an
// encode request round-trips the exact bits through JSON.
func playerOutcomesVec3(c CaseSpec, bits []string) (mgl32.Vec3, error) {
	if len(bits) != 3 {
		return mgl32.Vec3{}, fmt.Errorf("runtime-oracle: case %s carries %d vector components, want 3", c.ID, len(bits))
	}
	var vector mgl32.Vec3
	for index, text := range bits {
		value, err := float32FromBitsHex(c, text)
		if err != nil {
			return mgl32.Vec3{}, err
		}
		vector[index] = value
	}
	return vector, nil
}

// playerOutcomesPacket builds the DTO one encode case names from its typed
// fields, so the outbound validator decides about the same record the decode
// path would publish.
//
// A reject reason outside the frozen 1..15 interval names no registered Go
// reason, so the produced DTO carries an unregistered reason string and the
// outbound validator refuses it with the same unknown-reason rejection the
// decode path answers for the same wire byte: the case is a wire-level
// violation, never a silent zero publication.
func playerOutcomesPacket(c CaseSpec, input []byte) (protocol.ServerPacket, error) {
	switch c.Family {
	case playerStateFamily:
		var request playerStateEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return nil, err
		}
		position, err := playerOutcomesVec3(c, request.Position)
		if err != nil {
			return nil, err
		}
		velocity, err := playerOutcomesVec3(c, request.Velocity)
		if err != nil {
			return nil, err
		}
		yaw, err := float32FromBitsHex(c, request.Yaw)
		if err != nil {
			return nil, err
		}
		pitch, err := float32FromBitsHex(c, request.Pitch)
		if err != nil {
			return nil, err
		}
		return protocol.PlayerState{
			ServerTick:          request.ServerTick,
			LastInputSequence:   request.LastInputSequence,
			Dimension:           core.DimensionID(request.Dimension),
			Position:            position,
			Velocity:            velocity,
			Yaw:                 yaw,
			Pitch:               pitch,
			OnGround:            request.OnGround,
			Ready:               request.Ready,
			Reset:               request.Reset,
			MiningActive:        request.MiningActive,
			MiningTarget:        core.BlockPos{X: request.MiningTarget.X, Y: request.MiningTarget.Y, Z: request.MiningTarget.Z},
			MiningProgressTicks: request.MiningProgress,
			MiningRequiredTicks: request.MiningRequired,
			MiningHarvestable:   request.MiningHarvestable,
			Health:              request.Health,
			Oxygen:              request.Oxygen,
			Hunger:              request.Hunger,
			SaturationZero:      request.SaturationZero,
			DayPhaseOffset:      request.DayPhaseOffset,
			WorldTimeTicks:      request.WorldTimeTicks,
			WeatherKind:         core.WeatherKind(request.Weather),
			Season:              core.Season(request.Season),
			SeasonProgress:      request.SeasonProgress,
			Temperature:         request.Temperature,
			ArmorPoints:         request.Armor,
		}, nil
	case commandRejectedFamily:
		var request commandRejectedEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return nil, err
		}
		reason, ok := protocol.CommandRejectReasonForID(request.Reason)
		if !ok {
			reason = protocol.RejectReason(fmt.Sprintf("wire reason %d", request.Reason))
		}
		return protocol.CommandRejected{Sequence: request.Sequence, Reason: reason}, nil
	case placeBlockSucceededFamily:
		var request placeBlockSucceededEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return nil, err
		}
		return protocol.PlaceBlockSucceeded{Sequence: request.Sequence}, nil
	case combatHitFamily:
		var request combatHitEncodeRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return nil, err
		}
		return protocol.CombatHit{
			ServerTick: request.ServerTick,
			Damage:     request.Damage,
			TargetKind: core.CombatTargetKind(request.TargetKind),
		}, nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}
}

// runPlayerOutcomesEncode executes one private outcome encode case through
// the real Go encoder named by the case's own packet key and reads the result
// back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode
// case is refused by the same validator the decode path applies. The
// read-back guards the other direction, because bytes the production decoder
// rejects must never be recorded as evidence.
func runPlayerOutcomesEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := playerOutcomesPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newPlayerOutcomesCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := playerOutcomesPacket(c, input)
	if err != nil {
		return Outcome{}, nil, err
	}
	encodedID, payload, err := wireCodec.EncodeServer(state, packet)
	if err != nil {
		category, classified := playerOutcomesRejectionCategory(err)
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
	fields, err := playerOutcomesFields(c, decoded)
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

// playerOutcomesCorpusRoutes is the closed route map the four families
// execute.
func playerOutcomesCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range playerOutcomesFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: playerOutcomesVersion, Operation: "decode"}] = runPlayerOutcomesDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: playerOutcomesVersion, Operation: "encode"}] = runPlayerOutcomesEncode
	}
	return routes
}

// playerOutcomesCaseDefinition declares one case from literals before any
// producer runs, so the expectation is the review contract rather than a
// producer result.
type playerOutcomesCaseDefinition struct {
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

// playerOutcomesValidStateFields renders the normalized fields the canonical
// player state vector publishes.
func playerOutcomesValidStateFields() map[string]any {
	return map[string]any{
		"server_tick":           "0",
		"last_input_sequence":   "0",
		"dimension":             int32(1),
		"position":              []string{"80000000", "00000000", "00000000"},
		"velocity":              []string{"00000000", "00000000", "00000000"},
		"yaw":                   "00000000",
		"pitch":                 "80000000",
		"on_ground":             false,
		"ready":                 false,
		"reset":                 false,
		"mining_active":         false,
		"mining_target":         map[string]any{"x": int32(0), "y": int32(0), "z": int32(0)},
		"mining_progress_ticks": uint16(0),
		"mining_required_ticks": uint16(0),
		"mining_harvestable":    false,
		"health":                uint8(20),
		"oxygen":                uint16(300),
		"hunger":                uint8(20),
		"saturation_zero":       false,
		"day_phase_offset":      uint16(23999),
		"world_time_ticks":      "0",
		"weather_kind":          uint8(2),
		"season":                uint8(3),
		"season_progress":       uint8(255),
		"temperature":           int8(-128),
		"armor_points":          uint8(20),
	}
}

// playerOutcomesActiveMiningFields renders the normalized fields the active
// mining vector publishes.
func playerOutcomesActiveMiningFields() map[string]any {
	fields := playerOutcomesValidStateFields()
	fields["mining_active"] = true
	fields["mining_target"] = map[string]any{"x": int32(1), "y": int32(2), "z": int32(3)}
	fields["mining_progress_ticks"] = uint16(1)
	fields["mining_required_ticks"] = uint16(2)
	fields["mining_harvestable"] = true
	return fields
}

// playerStateRequestFields renders the canonical JSON encode request the
// valid player state cases carry, which is the reviewed wire's own field set.
func playerStateRequestFields() playerStateEncodeRequest {
	zero := "00000000"
	return playerStateEncodeRequest{
		Dimension:      1,
		Position:       []string{"80000000", zero, zero},
		Velocity:       []string{zero, zero, zero},
		Yaw:            zero,
		Pitch:          "80000000",
		Health:         20,
		Oxygen:         300,
		Hunger:         20,
		DayPhaseOffset: 23999,
		Weather:        2,
		Season:         3,
		SeasonProgress: 255,
		Temperature:    -128,
		Armor:          20,
	}
}

// playerOutcomesCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid
// cases use the canonical payloads the Go encoder produces for these fields,
// including the active mining variant; the malformed decode cases mutate that
// payload at one boundary each, and the invalid encode cases build a DTO the
// production validator refuses, so each rejection names the boundary that
// owns it. The reject reason and the combat target kind are published as
// their wire numbers, so the reason-zero and reason-sixteen cases are the
// frozen interval's own boundaries.
func playerOutcomesCaseDefinitions() []playerOutcomesCaseDefinition {
	validState := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   playerOutcomesValidStateFields(),
	}
	activeMining := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   playerOutcomesActiveMiningFields(),
	}
	validRejection := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: map[string]any{
			"sequence": "0",
			"reason":   uint8(1),
		},
	}
	reasonFifteen := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: map[string]any{
			"sequence": "0",
			"reason":   uint8(15),
		},
	}
	validPlace := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: map[string]any{
			"sequence": "0",
		},
	}
	validHit := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: map[string]any{
			"server_tick": "1",
			"damage":      uint8(1),
			"target_kind": uint8(1),
		},
	}
	kindThree := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: map[string]any{
			"server_tick": "1",
			"damage":      uint8(1),
			"target_kind": uint8(3),
		},
	}
	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	invalidEnum := Outcome{Kind: "error", Category: "invalid-enum"}
	truncated := Outcome{Kind: "error", Category: "truncated"}
	trailing := Outcome{Kind: "error", Category: "trailing"}

	canonicalWire := playerStateCanonical().wire()
	activeWire := playerStateActiveMining().wire()
	if len(canonicalWire) != playerStateWireBytes || len(activeWire) != playerStateWireBytes {
		panic("the player state vectors must be the fixed 93-byte stride")
	}

	// One-boundary mutations of the canonical vector's wire bytes. The byte
	// offsets are the Go encoder's field order: position words at 20..32,
	// health at 73, oxygen at 74..76, hunger at 76, day phase offset at
	// 78..80, weather at 88, season at 89 and armor at 92.
	mutate := func(mutator func(playerStateVector) playerStateVector) []byte {
		return mutator(playerStateCanonical()).wire()
	}
	miningInvalid := mutate(func(v playerStateVector) playerStateVector {
		v.miningProgress = 1
		return v
	})
	nanPosition := mutate(func(v playerStateVector) playerStateVector {
		v.position[0] = 0x7fc0_0000
		return v
	})
	healthAbove := mutate(func(v playerStateVector) playerStateVector {
		v.health = 21
		return v
	})
	oxygenAbove := mutate(func(v playerStateVector) playerStateVector {
		v.oxygen = 301
		return v
	})
	hungerAbove := mutate(func(v playerStateVector) playerStateVector {
		v.hunger = 21
		return v
	})
	dayOffsetAbove := mutate(func(v playerStateVector) playerStateVector {
		v.dayPhaseOffset = 24000
		return v
	})
	weatherThree := mutate(func(v playerStateVector) playerStateVector {
		v.weather = 3
		return v
	})
	seasonFour := mutate(func(v playerStateVector) playerStateVector {
		v.season = 4
		return v
	})
	armorAbove := mutate(func(v playerStateVector) playerStateVector {
		v.armor = 21
		return v
	})

	// The valid request and its one-boundary health mutation, so the encode
	// twin answers the same validator the decode case does.
	healthAboveRequest := playerStateRequestFields()
	healthAboveRequest.Health = 21

	commandValidWire := playerOutcomesCommandRejectedWire(0, 1)
	commandFifteenWire := playerOutcomesCommandRejectedWire(0, 15)
	commandZeroWire := playerOutcomesCommandRejectedWire(0, 0)
	commandSixteenWire := playerOutcomesCommandRejectedWire(0, 16)
	placeValidWire := playerOutcomesPlaceBlockSucceededWire(0)
	hitValidWire := playerOutcomesCombatHitWire(1, 1, 1)
	hitKindThreeWire := playerOutcomesCombatHitWire(1, 1, 3)

	definitions := make([]playerOutcomesCaseDefinition, 0, 32)
	definitions = append(definitions,
		playerOutcomesCaseDefinition{
			id:     playerStateDecodeCaseID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), canonicalWire...),
			expect: validState,
		},
		playerOutcomesCaseDefinition{
			id:      playerStateEncodeCaseID,
			family:  playerStateFamily,
			relDir:  playerStateCorpusRelDir,
			op:      "encode",
			request: playerStateRequestFields(),
			wire:    append([]byte(nil), canonicalWire...),
			expect:  validState,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateActiveMiningID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), activeWire...),
			expect: activeMining,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateMiningInvalidID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  miningInvalid,
			expect: invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateNaNPositionID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  nanPosition,
			expect: invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateHealthAboveID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  healthAbove,
			expect: invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:      playerStateHealthAboveEncID,
			family:  playerStateFamily,
			relDir:  playerStateCorpusRelDir,
			op:      "encode",
			request: healthAboveRequest,
			expect:  invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateOxygenAboveID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  oxygenAbove,
			expect: invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateHungerAboveID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  hungerAbove,
			expect: invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateDayOffsetAboveID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  dayOffsetAbove,
			expect: invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateWeatherThreeID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  weatherThree,
			expect: invalidEnum,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateSeasonFourID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  seasonFour,
			expect: invalidEnum,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateArmorAboveID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  armorAbove,
			expect: invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:     playerStateTrailingID,
			family: playerStateFamily,
			relDir: playerStateCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), canonicalWire...), 0x00),
			expect: trailing,
		},
		playerOutcomesCaseDefinition{
			id:     commandRejectedDecodeID,
			family: commandRejectedFamily,
			relDir: commandRejectedCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), commandValidWire...),
			expect: validRejection,
		},
		playerOutcomesCaseDefinition{
			id:      commandRejectedEncodeID,
			family:  commandRejectedFamily,
			relDir:  commandRejectedCorpusRelDir,
			op:      "encode",
			request: commandRejectedEncodeRequest{Sequence: 0, Reason: 1},
			wire:    append([]byte(nil), commandValidWire...),
			expect:  validRejection,
		},
		playerOutcomesCaseDefinition{
			id:     commandRejectedReasonHighID,
			family: commandRejectedFamily,
			relDir: commandRejectedCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), commandFifteenWire...),
			expect: reasonFifteen,
		},
		playerOutcomesCaseDefinition{
			id:     commandRejectedReasonZeroID,
			family: commandRejectedFamily,
			relDir: commandRejectedCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), commandZeroWire...),
			expect: invalidEnum,
		},
		playerOutcomesCaseDefinition{
			id:      commandRejectedReasonZeroEnc,
			family:  commandRejectedFamily,
			relDir:  commandRejectedCorpusRelDir,
			op:      "encode",
			request: commandRejectedEncodeRequest{Sequence: 0, Reason: 0},
			expect:  invalidEnum,
		},
		playerOutcomesCaseDefinition{
			id:     commandRejectedReasonSixteen,
			family: commandRejectedFamily,
			relDir: commandRejectedCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), commandSixteenWire...),
			expect: invalidEnum,
		},
		playerOutcomesCaseDefinition{
			id:     placeBlockSucceededDecodeID,
			family: placeBlockSucceededFamily,
			relDir: placeBlockSucceededCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), placeValidWire...),
			expect: validPlace,
		},
		playerOutcomesCaseDefinition{
			id:      placeBlockSucceededEncodeID,
			family:  placeBlockSucceededFamily,
			relDir:  placeBlockSucceededCorpusRelDir,
			op:      "encode",
			request: placeBlockSucceededEncodeRequest{Sequence: 0},
			wire:    append([]byte(nil), placeValidWire...),
			expect:  validPlace,
		},
		playerOutcomesCaseDefinition{
			id:     placeBlockSucceededTruncID,
			family: placeBlockSucceededFamily,
			relDir: placeBlockSucceededCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), placeValidWire[:placeBlockSucceededWireBytes-1]...),
			expect: truncated,
		},
		playerOutcomesCaseDefinition{
			id:     placeBlockSucceededTrailing,
			family: placeBlockSucceededFamily,
			relDir: placeBlockSucceededCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), placeValidWire...), 0x00),
			expect: trailing,
		},
		playerOutcomesCaseDefinition{
			id:     combatHitDecodeID,
			family: combatHitFamily,
			relDir: combatHitCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), hitValidWire...),
			expect: validHit,
		},
		playerOutcomesCaseDefinition{
			id:      combatHitEncodeID,
			family:  combatHitFamily,
			relDir:  combatHitCorpusRelDir,
			op:      "encode",
			request: combatHitEncodeRequest{ServerTick: 1, Damage: 1, TargetKind: 1},
			wire:    append([]byte(nil), hitValidWire...),
			expect:  validHit,
		},
		playerOutcomesCaseDefinition{
			id:     combatHitKindThreeID,
			family: combatHitFamily,
			relDir: combatHitCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), hitKindThreeWire...),
			expect: kindThree,
		},
		playerOutcomesCaseDefinition{
			id:     combatHitTickZeroID,
			family: combatHitFamily,
			relDir: combatHitCorpusRelDir,
			op:     "decode",
			input:  playerOutcomesCombatHitWire(0, 1, 1),
			expect: invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:     combatHitDamageZeroID,
			family: combatHitFamily,
			relDir: combatHitCorpusRelDir,
			op:     "decode",
			input:  playerOutcomesCombatHitWire(1, 0, 1),
			expect: invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:     combatHitDamageAboveID,
			family: combatHitFamily,
			relDir: combatHitCorpusRelDir,
			op:     "decode",
			input:  playerOutcomesCombatHitWire(1, 21, 1),
			expect: invalidValue,
		},
		playerOutcomesCaseDefinition{
			id:     combatHitKindZeroID,
			family: combatHitFamily,
			relDir: combatHitCorpusRelDir,
			op:     "decode",
			input:  playerOutcomesCombatHitWire(1, 1, 0),
			expect: invalidEnum,
		},
		playerOutcomesCaseDefinition{
			id:     combatHitKindFourID,
			family: combatHitFamily,
			relDir: combatHitCorpusRelDir,
			op:     "decode",
			input:  playerOutcomesCombatHitWire(1, 1, 4),
			expect: invalidEnum,
		},
	)

	return definitions
}

// playerOutcomesLabel renders one case's asset label from its identity.
func playerOutcomesLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// buildPlayerOutcomesCandidate builds one case's manifest entry and asset
// bytes from its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildPlayerOutcomesCandidate(t *testing.T, definition playerOutcomesCaseDefinition) playerOutcomesCandidate {
	t.Helper()

	label := playerOutcomesLabel(definition.id)
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
		Version:      playerOutcomesVersion,
		Operation:    definition.op,
		PacketKey:    playerOutcomesKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return playerOutcomesCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// playerOutcomesKeyPointer resolves one family's reviewed packet key for a
// case spec.
func playerOutcomesKeyPointer(family string) *PacketKeySpec {
	key, owned := playerOutcomesFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// playerOutcomesCandidate is one reviewed case: its manifest specification,
// the exact asset bytes it publishes, and the expectation an independent
// execution has to reproduce.
type playerOutcomesCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which
	// the producer's own bytes are compared against.
	Wire []byte
}

// playerOutcomesCandidates builds this group's complete reviewed case set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func playerOutcomesCandidates(t *testing.T, root string) []playerOutcomesCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := playerOutcomesCaseDefinitions()
	candidates := make([]playerOutcomesCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildPlayerOutcomesCandidate(t, definition)
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

// playerOutcomesRegisteredCase reports whether the base manifest already
// carries one case identity, so a re-merge of an integrated candidate adds
// nothing.
func playerOutcomesRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// playerOutcomesSelection is this group's registration: its cases, the Go
// sources its rules are read from, and the routes the four families execute.
func playerOutcomesSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := playerOutcomesCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range playerOutcomesFamilies() {
		sources[family.id] = playerOutcomesServerSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(playerOutcomesFamilies())*2)
	for _, family := range playerOutcomesFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: playerOutcomesVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: playerOutcomesVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  playerOutcomesProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// playerOutcomesServerSources lists the Go sources each family reads its
// rules from: the shared server codec dispatch with its decode arms, and the
// family's own message file, which owns the validator the shared dispatch
// applies at the tail.
func playerOutcomesServerSources(family string) []string {
	shared := []string{
		"packages/shared/network/codec/codec_server.go",
	}
	switch family {
	case playerStateFamily:
		return append(shared, "packages/shared/network/protocol/message_player.go")
	case commandRejectedFamily:
		return append(shared, "packages/shared/network/protocol/message_player.go")
	case placeBlockSucceededFamily:
		return append(shared, "packages/shared/network/protocol/message_player.go")
	case combatHitFamily:
		return append(shared, "packages/shared/network/protocol/message_combat.go")
	default:
		return nil
	}
}

// playerOutcomesManifest assembles the family-scoped selection the route
// runner executes: this group's candidates with every other family cleared,
// so reconciliation accepts the scoped manifest.
func playerOutcomesManifest(t *testing.T, root string, candidates []playerOutcomesCandidate) Inventory {
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
		if !playerOutcomesOwnsFamily(family) {
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
		t.Fatalf("encode player outcomes working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write player outcomes working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load player outcomes working manifest: %v", err)
	}
	return loaded
}

// playerOutcomesOwnsFamily reports whether this group registers cases for one
// family.
func playerOutcomesOwnsFamily(family string) bool {
	_, owned := playerOutcomesFamilyKeys[family]
	return owned
}

// playerOutcomesScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve
// under the root the runner is given.
func playerOutcomesScratchRoot(t *testing.T, root string, candidates []playerOutcomesCandidate) string {
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

// playerOutcomesCandidateByID resolves one candidate by its case identity.
func playerOutcomesCandidateByID(t *testing.T, candidates []playerOutcomesCandidate, id string) playerOutcomesCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return playerOutcomesCandidate{}
}

// playerOutcomesObservation resolves one executed observation by its case
// identity.
func playerOutcomesObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// playerOutcomesExportPublished guards the single publication per test
// process, because the exporter's producer child is create-exclusive and more
// than one test in this package observes the same candidates.
var playerOutcomesExportPublished bool

// playerOutcomesCandidatesExport publishes the reviewed candidates and the
// complete merged manifest candidate through the existing external exporter
// and returns the published producer directory. An unset export variable
// publishes nothing and returns "", so an ordinary test run never writes
// outside its own temporary storage.
func playerOutcomesCandidatesExport(t *testing.T, root string, candidates []playerOutcomesCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if playerOutcomesExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-player-outcomes")
	}
	playerOutcomesExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, playerOutcomesProducerID, assets)
	if err != nil {
		t.Fatalf("export player outcomes candidates: %v", err)
	}
	return published
}

// playerOutcomesDecodeCaseIDs lists the valid decode cases the producer pins.
func playerOutcomesDecodeCaseIDs() []string {
	return []string{
		playerStateDecodeCaseID,
		playerStateActiveMiningID,
		commandRejectedDecodeID,
		commandRejectedReasonHighID,
		placeBlockSucceededDecodeID,
		combatHitDecodeID,
		combatHitKindThreeID,
	}
}

// playerOutcomesEncodeCaseIDs lists the valid encode cases the producer pins.
func playerOutcomesEncodeCaseIDs() []string {
	return []string{
		playerStateEncodeCaseID,
		commandRejectedEncodeID,
		placeBlockSucceededEncodeID,
		combatHitEncodeID,
	}
}

// playerOutcomesRejectedDecodeCaseIDs lists the malformed decode cases.
func playerOutcomesRejectedDecodeCaseIDs() []string {
	return []string{
		playerStateMiningInvalidID,
		playerStateNaNPositionID,
		playerStateHealthAboveID,
		playerStateOxygenAboveID,
		playerStateHungerAboveID,
		playerStateDayOffsetAboveID,
		playerStateWeatherThreeID,
		playerStateSeasonFourID,
		playerStateArmorAboveID,
		playerStateTrailingID,
		commandRejectedReasonZeroID,
		commandRejectedReasonSixteen,
		placeBlockSucceededTruncID,
		placeBlockSucceededTrailing,
		combatHitTickZeroID,
		combatHitDamageZeroID,
		combatHitDamageAboveID,
		combatHitKindZeroID,
		combatHitKindFourID,
	}
}

// playerOutcomesRejectedEncodeCaseIDs lists the invalid encode cases.
func playerOutcomesRejectedEncodeCaseIDs() []string {
	return []string{
		playerStateHealthAboveEncID,
		commandRejectedReasonZeroEnc,
	}
}

// TestProtocolPlayerOutcomesOracleDecodesEveryValidCase pins that the real Go
// decoder publishes the canonical fields for every valid decode case,
// including the active mining variant and the wire reason and kind
// boundaries.
func TestProtocolPlayerOutcomesOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := playerOutcomesCandidates(t, root)

	for _, id := range playerOutcomesDecodeCaseIDs() {
		candidate := playerOutcomesCandidateByID(t, candidates, id)
		outcome, encoded, err := runPlayerOutcomesDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runPlayerOutcomesDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolPlayerOutcomesOracleEncodesCanonicalWire pins the encode
// producer against the reviewed wire literals and against the recorded
// digests.
func TestProtocolPlayerOutcomesOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := playerOutcomesCandidates(t, root)

	for _, id := range playerOutcomesEncodeCaseIDs() {
		candidate := playerOutcomesCandidateByID(t, candidates, id)
		outcome, encoded, err := runPlayerOutcomesEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runPlayerOutcomesEncode(%s): %v", id, err)
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

// TestProtocolPlayerOutcomesOracleRejectsMalformedCasesAtTheirBoundary pins
// that every malformed decode case and invalid encode case is refused by the
// production codec and classified at the boundary that owns it.
func TestProtocolPlayerOutcomesOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := playerOutcomesCandidates(t, root)

	for _, id := range playerOutcomesRejectedDecodeCaseIDs() {
		candidate := playerOutcomesCandidateByID(t, candidates, id)
		outcome, encoded, err := runPlayerOutcomesDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runPlayerOutcomesDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range playerOutcomesRejectedEncodeCaseIDs() {
		candidate := playerOutcomesCandidateByID(t, candidates, id)
		outcome, encoded, err := runPlayerOutcomesEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runPlayerOutcomesEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolPlayerOutcomesOracleReasonBoundariesStayEnumPins pins the
// controller ruling this group's reason cases rest on: reason 0 and 16 are
// the closed interval's own boundaries and stay invalid-enum on both
// directions, while reason 15 is the admitted boundary — the wire reason is a
// translation the Rust side owns explicitly, never a domain-cast identity.
func TestProtocolPlayerOutcomesOracleReasonBoundariesStayEnumPins(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := playerOutcomesCandidates(t, root)

	for _, id := range []string{commandRejectedReasonZeroID, commandRejectedReasonSixteen, commandRejectedReasonZeroEnc} {
		candidate := playerOutcomesCandidateByID(t, candidates, id)
		var outcome Outcome
		var err error
		if candidate.Spec.Operation == "encode" {
			outcome, _, err = runPlayerOutcomesEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		} else {
			outcome, _, err = runPlayerOutcomesDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		}
		if err != nil {
			t.Fatalf("run(%s): %v", id, err)
		}
		if outcome.Kind != "error" || outcome.Category != "invalid-enum" {
			t.Fatalf("case %s produced %#v, want the invalid-enum category", id, outcome)
		}
	}

	boundary := playerOutcomesCandidateByID(t, candidates, commandRejectedReasonHighID)
	outcome, _, err := runPlayerOutcomesDecode(boundary.Spec, boundary.Assets[boundary.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runPlayerOutcomesDecode(%s): %v", boundary.Spec.ID, err)
	}
	if outcome.Kind != "ok" {
		t.Fatalf("case %s produced %#v, want the admitted boundary", boundary.Spec.ID, outcome)
	}
}

// TestProtocolPlayerOutcomesOracleExpectedFieldMutationFailsComparison pins
// that the recorded expectation is a commitment: replacing an expected field
// fails comparison against what the producer decoded, and replacing the
// encoded digest fails comparison against the reviewed wire.
func TestProtocolPlayerOutcomesOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := playerOutcomesCandidates(t, root)

	state := playerOutcomesCandidateByID(t, candidates, playerStateDecodeCaseID)
	produced, _, err := runPlayerOutcomesDecode(state.Spec, state.Assets[state.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runPlayerOutcomesDecode: %v", err)
	}
	mutated := state.Expect
	fields := make(map[string]any, len(state.Expect.Fields))
	for key, value := range state.Expect.Fields {
		fields[key] = value
	}
	// The negative zero is the contract: a normalization that published +0.0
	// for the position X would hide the exact bits the wire carries.
	fields["position"] = []string{"00000000", "00000000", "00000000"}
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated position bits compare equal to the produced outcome")
	}

	hit := playerOutcomesCandidateByID(t, candidates, combatHitDecodeID)
	producedHit, _, err := runPlayerOutcomesDecode(hit.Spec, hit.Assets[hit.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runPlayerOutcomesDecode: %v", err)
	}
	mutatedHit := producedHit
	hitFields := make(map[string]any, len(producedHit.Fields))
	for key, value := range producedHit.Fields {
		hitFields[key] = value
	}
	hitFields["target_kind"] = uint8(2)
	mutatedHit.Fields = hitFields
	if outcomesEqual(mutatedHit, hit.Expect) {
		t.Fatal("mutated target kind compares equal to the reviewed expectation")
	}

	encode := playerOutcomesCandidateByID(t, candidates, placeBlockSucceededEncodeID)
	producedEncode, _, err := runPlayerOutcomesEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runPlayerOutcomesEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolPlayerOutcomesOracleRoutesExecuteEveryCase executes this
// group's complete case set through the shared packet-case runner, once per
// case, and compares every observation with the reviewed expectation.
func TestProtocolPlayerOutcomesOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := playerOutcomesCandidates(t, root)
	manifest := playerOutcomesManifest(t, root, candidates)
	staged := playerOutcomesScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, playerOutcomesCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := playerOutcomesObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolPlayerOutcomesOracleRunnerRejectsUnregisteredRoute pins that a
// case naming a route this group does not claim fails before its producer
// runs, so a case cannot claim coverage from its name alone.
func TestProtocolPlayerOutcomesOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := playerOutcomesCandidates(t, root)
	manifest := playerOutcomesManifest(t, root, candidates)
	staged := playerOutcomesScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range playerOutcomesFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: playerOutcomesVersion, Operation: "decode"}] = runPlayerOutcomesDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range playerOutcomesFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+playerOutcomesVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolPlayerOutcomesOracleManifestMergeRegistersPrivateRoutes pins
// that the merged manifest registers all four families' routes and case
// lists, leaves the source revision alone, and records this group's
// provenance sources.
func TestProtocolPlayerOutcomesOracleManifestMergeRegistersPrivateRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, playerOutcomesSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range playerOutcomesFamilies() {
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
		for _, want := range playerOutcomesServerSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so
	// a re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range playerOutcomesCandidates(t, root) {
		if !playerOutcomesRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolPlayerOutcomesOracleCandidatesExportForReview publishes the
// reviewed candidates and the manifest candidate. An unset export variable
// publishes nothing, so the tracked corpus is never written by this package.
func TestProtocolPlayerOutcomesOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := playerOutcomesCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), playerOutcomesSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := playerOutcomesCandidatesExport(t, root, candidates, merged)
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

// playerOutcomesSilentReasonWire renders the byte the Go encoder would
// publish for an unregistered reject reason if the outbound validator did
// not run first. The producer keeps it out of the case table; it exists so
// the red evidence quotes the real production bytes.
func playerOutcomesSilentReasonWire() []byte {
	return playerOutcomesCommandRejectedWire(0, 0)
}

// TestProtocolPlayerOutcomesGoEncoderRefusesUnregisteredReason pins that the
// production encoder runs the outbound validator before it writes the reject
// reason byte, so an unregistered reason never becomes a silent zero.
func TestProtocolPlayerOutcomesGoEncoderRefusesUnregisteredReason(t *testing.T) {
	wireCodec, err := newPlayerOutcomesCodec()
	if err != nil {
		t.Fatalf("newPlayerOutcomesCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, protocol.CommandRejected{Reason: protocol.RejectReason("wire reason 0")}); err == nil {
		t.Fatal("the Go encoder published an unregistered reject reason")
	}
	silent := playerOutcomesSilentReasonWire()
	if len(silent) != commandRejectedWireBytes || silent[8] != 0 {
		t.Fatalf("the silent reason wire is %x", silent)
	}
}
