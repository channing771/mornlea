package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file is the Go producer for the domain command-outcome and
// player-publication event family. It executes every committed corpus case
// through the current Go protocol DTOs: the verdict comes from
// `protocol.ValidateServerPacket`, which is the same Play-state validator the
// codec applies on both the encode and the decode side, and an admitted record
// is normalized field by field through the frozen field map. No world
// authority, listener, model call or native ABI is involved: the producer only
// reads immutable corpus inputs and calls in-process validators, and the frozen
// corpus files it materializes are read-only inputs for later consumers.
//
// The four rules this family executes are the Go `protocol.PlayerState`,
// `protocol.CommandRejected`, `protocol.PlaceBlockSucceeded` and
// `protocol.CombatHit` records, which are the outcomes an authoritative tick
// publishes to one session.

const (
	// domainEventPlayerFamily is the corpus family this package executes. The
	// family is the existing `domain.event` row, whose eventual owner is the
	// Rust domain crate that owns the replay observation records.
	domainEventPlayerFamily = "domain.event"
	// domainEventPlayerVersion is the family's discovered version. A case has
	// to name its family's version, so the case identities carry this segment
	// rather than one this producer chose.
	domainEventPlayerVersion = "1"
	// domainEventPlayerOperation is the manifest operation name for an outcome
	// or player-publication admission case.
	domainEventPlayerOperation = "admit"
	// domainEventPlayerConsumer is the manifest consumer the change pins for
	// this family: the Rust crate that owns these records.
	domainEventPlayerConsumer = "mornlea_domain"
	// domainEventPlayerCorpusRelDir is the repository-relative directory
	// holding the frozen event player corpus cases.
	domainEventPlayerCorpusRelDir = "testdata/runtime-migration/cases/domain/event_player"
	// domainEventPlayerProducerTestRelPath and the two producer test names
	// locate the package-local producer that executes the protocol DTOs. The
	// topic name is the entry point the domain plan's filter requires; a corpus
	// whose producer test is missing has no independently executed evidence at
	// all.
	domainEventPlayerProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_event_player_test.go"
	domainEventPlayerProducerExecuteName = "TestDomainEventPlayerOracleExecutesEveryCase"
	domainEventPlayerProducerTopicName   = "TestDomainOracle_event_player"
	// domainEventPlayerCorpusReportName is the published report file name for
	// the executed event player evidence.
	domainEventPlayerCorpusReportName = "runtime-corpus-domain-event-player.json"
	// domainEventPlayerSource is the primary provenance source of the player
	// publication and command outcome rules.
	domainEventPlayerSource = "packages/shared/network/protocol/message_player.go"
)

// domainEventPlayerFamilySources is the merged provenance set the family
// records. Every entry is a file the producer's rules are read from, so a
// change to any of them is a change to the recorded evidence. The combat and
// registry files belong to the set because the combat hit record is declared
// beside them and the reject-reason registry is the authority that decides
// which reason names are published.
var domainEventPlayerFamilySources = []string{
	"packages/shared/network/protocol/message_player.go",
	"packages/shared/network/protocol/message_combat.go",
	"packages/shared/network/protocol/registry.go",
	"packages/shared/core/health.go",
	"packages/shared/core/hunger.go",
	"packages/shared/core/armor.go",
	"packages/shared/core/weather.go",
	"packages/shared/core/season.go",
	"packages/shared/core/day_phase.go",
	"packages/shared/core/combat.go",
	"packages/shared/core/pos.go",
}

// domainEventPlayerLabelPattern is the shape a corpus label must have: a
// lowercase slug with an optional zero-padded numeric suffix, so a boundary row
// sorts in numeric order under a lexical sort.
var domainEventPlayerLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// updateDomainEventPlayerCorpus rewrites the frozen event player corpus from
// the executed protocol DTOs. It follows the same discipline as the other
// fixture update flags: an ordinary run only compares, so a frozen artifact is
// never silently regenerated to match an implementation.
var updateDomainEventPlayerCorpus = flag.Bool(
	"update-domain-event-player-corpus",
	false,
	"rewrite testdata/runtime-migration/cases/domain/event_player from the executed protocol outcome and player state DTOs",
)

// domainEventPlayerInput is the frozen, self-describing corpus input for one
// case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. Every field is a pointer so an
// absent field and an explicit zero stay distinguishable, which matters
// because a zero sequence, a zero server tick, a zero health and a false flag
// are all meaningful values. `Position` and `Velocity` are three decimal float
// texts rather than JSON numbers because a JSON number cannot express NaN, an
// infinity or a signed zero, and those exact values are boundaries this family
// pins. The four rules share one envelope because each rule reads only the
// fields it names.
type domainEventPlayerInput struct {
	Consumer string `json:"consumer"`
	Rule     string `json:"rule"`

	// Player publication fields, in the Go DTO's declaration order.
	ServerTick        *uint64  `json:"server_tick,omitempty"`
	LastInputSequence *uint64  `json:"last_input_sequence,omitempty"`
	Dimension         *int32   `json:"dimension,omitempty"`
	Position          []string `json:"position,omitempty"`
	Velocity          []string `json:"velocity,omitempty"`
	Yaw               *string  `json:"yaw,omitempty"`
	Pitch             *string  `json:"pitch,omitempty"`
	OnGround          *bool    `json:"on_ground,omitempty"`
	Ready             *bool    `json:"ready,omitempty"`
	Reset             *bool    `json:"reset,omitempty"`
	MiningActive      *bool    `json:"mining_active,omitempty"`
	MiningTarget      []int32  `json:"mining_target,omitempty"`
	MiningProgress    *int     `json:"mining_progress,omitempty"`
	MiningRequired    *int     `json:"mining_required,omitempty"`
	MiningHarvestable *bool    `json:"mining_harvestable,omitempty"`
	Health            *int     `json:"health,omitempty"`
	Oxygen            *int     `json:"oxygen,omitempty"`
	Hunger            *int     `json:"hunger,omitempty"`
	SaturationZero    *bool    `json:"saturation_zero,omitempty"`
	DayPhaseOffset    *int     `json:"day_phase_offset,omitempty"`
	WorldTimeTicks    *uint64  `json:"world_time_ticks,omitempty"`
	WeatherKind       *int     `json:"weather_kind,omitempty"`
	Season            *int     `json:"season,omitempty"`
	SeasonProgress    *int     `json:"season_progress,omitempty"`
	Temperature       *int     `json:"temperature,omitempty"`
	ArmorPoints       *int     `json:"armor_points,omitempty"`

	// Outcome fields the rejection, success and hit rules read. The rejection
	// and the success share the sequence, the hit shares the server tick with
	// the player publication, and the hit alone names a damage and a target
	// kind.
	Sequence   *uint64 `json:"sequence,omitempty"`
	Reason     *string `json:"reason,omitempty"`
	Damage     *int    `json:"damage,omitempty"`
	TargetKind *int    `json:"target_kind,omitempty"`
}

func (in domainEventPlayerInput) withSequence(sequence uint64) domainEventPlayerInput {
	in.Sequence = &sequence
	return in
}

func (in domainEventPlayerInput) withReason(reason string) domainEventPlayerInput {
	in.Reason = &reason
	return in
}

func (in domainEventPlayerInput) withDamage(damage int) domainEventPlayerInput {
	in.Damage = &damage
	return in
}

func (in domainEventPlayerInput) withTargetKind(kind int) domainEventPlayerInput {
	in.TargetKind = &kind
	return in
}

// domainEventPlayerCase is one frozen corpus case: its label and its input,
// and nothing else.
type domainEventPlayerCase struct {
	label string
	input domainEventPlayerInput
}

// domainEventPlayerSeedState is the seed player publication: every finite
// vector is zero, the survival scalars sit at their authoritative maxima, the
// day phase offset at the last tick of the cycle, the world time at zero, the
// weather and season at their first value, the season progress at 255 and the
// temperature at the minimum int8, and the mining block is the canonical
// inactive one.
func domainEventPlayerSeedState() protocol.PlayerState {
	return protocol.PlayerState{
		ServerTick:          0,
		LastInputSequence:   0,
		Dimension:           core.Overworld,
		Position:            mgl32.Vec3{},
		Velocity:            mgl32.Vec3{},
		Yaw:                 0,
		Pitch:               0,
		OnGround:            true,
		Ready:               true,
		Reset:               false,
		MiningActive:        false,
		MiningTarget:        core.BlockPos{},
		MiningProgressTicks: 0,
		MiningRequiredTicks: 0,
		MiningHarvestable:   false,
		Health:              core.MaxHealth,
		Oxygen:              core.MaxOxygenTicks,
		Hunger:              core.MaxHunger,
		SaturationZero:      false,
		DayPhaseOffset:      core.DayLengthTicks - 1,
		WorldTimeTicks:      0,
		WeatherKind:         core.WeatherClear,
		Season:              core.SeasonSpring,
		SeasonProgress:      255,
		Temperature:         -128,
		ArmorPoints:         core.MaxArmorPoints,
	}
}

// domainEventPlayerStateCase renders one player-publication row: the seed
// state with exactly one field changed, so a boundary row is always the seed
// plus the value the rule names.
func domainEventPlayerStateCase(label string, mutate func(*protocol.PlayerState)) domainEventPlayerCase {
	state := domainEventPlayerSeedState()
	if mutate != nil {
		mutate(&state)
	}
	return domainEventPlayerCase{label: label, input: domainEventPlayerStateInput(state)}
}

// domainEventPlayerStateInput renders one Go player publication value as the
// frozen input envelope. The three float texts per vector and the two angle
// texts keep NaN, the infinities and a signed zero expressible, so the
// envelope is a lossless rendering of the DTO value.
func domainEventPlayerStateInput(state protocol.PlayerState) domainEventPlayerInput {
	input := domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "player-state"}
	input.ServerTick = &state.ServerTick
	input.LastInputSequence = &state.LastInputSequence
	dimension := int32(state.Dimension)
	input.Dimension = &dimension
	input.Position = []string{
		domainEventPlayerFloatText(state.Position.X()),
		domainEventPlayerFloatText(state.Position.Y()),
		domainEventPlayerFloatText(state.Position.Z()),
	}
	input.Velocity = []string{
		domainEventPlayerFloatText(state.Velocity.X()),
		domainEventPlayerFloatText(state.Velocity.Y()),
		domainEventPlayerFloatText(state.Velocity.Z()),
	}
	input.Yaw = domainEventPlayerFloatTextPointer(state.Yaw)
	input.Pitch = domainEventPlayerFloatTextPointer(state.Pitch)
	input.OnGround = &state.OnGround
	input.Ready = &state.Ready
	input.Reset = &state.Reset
	input.MiningActive = &state.MiningActive
	input.MiningTarget = []int32{state.MiningTarget.X, state.MiningTarget.Y, state.MiningTarget.Z}
	miningProgress := int(state.MiningProgressTicks)
	input.MiningProgress = &miningProgress
	miningRequired := int(state.MiningRequiredTicks)
	input.MiningRequired = &miningRequired
	input.MiningHarvestable = &state.MiningHarvestable
	health := int(state.Health)
	input.Health = &health
	oxygen := int(state.Oxygen)
	input.Oxygen = &oxygen
	hunger := int(state.Hunger)
	input.Hunger = &hunger
	input.SaturationZero = &state.SaturationZero
	dayPhaseOffset := int(state.DayPhaseOffset)
	input.DayPhaseOffset = &dayPhaseOffset
	input.WorldTimeTicks = &state.WorldTimeTicks
	weatherKind := int(state.WeatherKind)
	input.WeatherKind = &weatherKind
	season := int(state.Season)
	input.Season = &season
	seasonProgress := int(state.SeasonProgress)
	input.SeasonProgress = &seasonProgress
	temperature := int(state.Temperature)
	input.Temperature = &temperature
	armorPoints := int(state.ArmorPoints)
	input.ArmorPoints = &armorPoints
	return input
}

// domainEventPlayerCases is the ordered case table the producer executes.
//
// The player-publication rows are the seed plus one boundary value each: the
// four survival maxima one above their limit, the day phase offset at the day
// length, the unknown weather and season, the unknown dimension, the three
// non-finite pose and angle fields, the other playable dimension, the two
// full-range scalars at their opposite extreme, and the mining union in every
// shape the Go validator names. The outcome rows are the fifteen published
// rejection reasons plus an unregistered one, the placement success at a zero
// and a nonzero sequence, and the combat hit at every boundary of its tick,
// damage and target-kind rules.
func domainEventPlayerCases() []domainEventPlayerCase {
	cases := []domainEventPlayerCase{
		domainEventPlayerStateCase("player-state-seed", nil),
		domainEventPlayerStateCase("player-state-health-above-max", func(state *protocol.PlayerState) {
			state.Health = core.MaxHealth + 1
		}),
		domainEventPlayerStateCase("player-state-oxygen-above-max", func(state *protocol.PlayerState) {
			state.Oxygen = core.MaxOxygenTicks + 1
		}),
		domainEventPlayerStateCase("player-state-hunger-above-max", func(state *protocol.PlayerState) {
			state.Hunger = core.MaxHunger + 1
		}),
		domainEventPlayerStateCase("player-state-armor-points-above-max", func(state *protocol.PlayerState) {
			state.ArmorPoints = core.MaxArmorPoints + 1
		}),
		domainEventPlayerStateCase("player-state-day-phase-offset-at-day-length", func(state *protocol.PlayerState) {
			state.DayPhaseOffset = core.DayLengthTicks
		}),
		domainEventPlayerStateCase("player-state-weather-kind-unknown", func(state *protocol.PlayerState) {
			state.WeatherKind = core.WeatherThunder + 1
		}),
		domainEventPlayerStateCase("player-state-season-unknown", func(state *protocol.PlayerState) {
			state.Season = core.SeasonWinter + 1
		}),
		domainEventPlayerStateCase("player-state-dimension-unknown", func(state *protocol.PlayerState) {
			state.Dimension = core.DimensionID(2)
		}),
		domainEventPlayerStateCase("player-state-depths-dimension", func(state *protocol.PlayerState) {
			state.Dimension = core.Depths
		}),
		domainEventPlayerStateCase("player-state-non-finite-position", func(state *protocol.PlayerState) {
			state.Position = mgl32.Vec3{float32(math.NaN()), 0, 0}
		}),
		domainEventPlayerStateCase("player-state-non-finite-velocity", func(state *protocol.PlayerState) {
			state.Velocity = mgl32.Vec3{0, float32(math.Inf(1)), 0}
		}),
		domainEventPlayerStateCase("player-state-non-finite-rotation", func(state *protocol.PlayerState) {
			state.Yaw = float32(math.NaN())
		}),
		domainEventPlayerStateCase("player-state-full-range-progress-and-temperature", func(state *protocol.PlayerState) {
			state.SeasonProgress = 0
			state.Temperature = 127
		}),
		domainEventPlayerStateCase("player-state-mining-inactive-with-target", func(state *protocol.PlayerState) {
			state.MiningTarget = core.BlockPos{X: 1, Y: 2, Z: 3}
		}),
		domainEventPlayerStateCase("player-state-mining-inactive-with-progress", func(state *protocol.PlayerState) {
			state.MiningProgressTicks = 1
		}),
		domainEventPlayerStateCase("player-state-mining-inactive-with-required", func(state *protocol.PlayerState) {
			state.MiningRequiredTicks = 2
		}),
		domainEventPlayerStateCase("player-state-mining-inactive-harvestable", func(state *protocol.PlayerState) {
			state.MiningHarvestable = true
		}),
		domainEventPlayerStateCase("player-state-mining-active-progress-one-of-two", func(state *protocol.PlayerState) {
			state.MiningActive = true
			state.MiningTarget = core.BlockPos{X: -4, Y: 64, Z: 7}
			state.MiningProgressTicks = 1
			state.MiningRequiredTicks = 2
			state.MiningHarvestable = true
		}),
		domainEventPlayerStateCase("player-state-mining-active-progress-zero", func(state *protocol.PlayerState) {
			state.MiningActive = true
			state.MiningProgressTicks = 0
			state.MiningRequiredTicks = 2
		}),
		domainEventPlayerStateCase("player-state-mining-active-progress-at-required", func(state *protocol.PlayerState) {
			state.MiningActive = true
			state.MiningProgressTicks = 2
			state.MiningRequiredTicks = 2
		}),
	}

	reasons := []struct {
		label  string
		reason protocol.RejectReason
	}{
		{"invalid-ray", protocol.RejectInvalidRay},
		{"no-target", protocol.RejectNoTarget},
		{"chunk-not-ready", protocol.RejectChunkNotReady},
		{"protected-block", protocol.RejectProtectedBlock},
		{"invalid-block", protocol.RejectInvalidBlock},
		{"occupied", protocol.RejectOccupied},
		{"invalid-input", protocol.RejectInvalidInput},
		{"player-not-ready", protocol.RejectPlayerNotReady},
		{"invalid-slot", protocol.RejectInvalidSlot},
		{"hotbar-full", protocol.RejectHotbarFull},
		{"drop-capacity", protocol.RejectDropCapacity},
		{"container-capacity", protocol.RejectContainerCapacity},
		{"not-fluid-source", protocol.RejectNotFluidSource},
		{"bucket-mismatch", protocol.RejectBucketMismatch},
		{"not-armor", protocol.RejectNotArmor},
	}
	for _, entry := range reasons {
		cases = append(cases, domainEventPlayerCase{
			label: "command-rejected-" + entry.label,
			input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "command-rejected"}.
				withSequence(7).withReason(string(entry.reason)),
		})
	}
	// A zero sequence is legal on a rejection: the Go wire accepts it and the
	// `/warp` rejects are published with it outside the tick result.
	cases = append(cases, domainEventPlayerCase{
		label: "command-rejected-zero-sequence",
		input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "command-rejected"}.
			withSequence(0).withReason(string(protocol.RejectInvalidRay)),
	})
	cases = append(cases, domainEventPlayerCase{
		label: "command-rejected-unknown-reason",
		input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "command-rejected"}.
			withSequence(7).withReason("unknown_reason"),
	})

	cases = append(cases,
		domainEventPlayerCase{
			label: "place-block-succeeded-zero-sequence",
			input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "place-block-succeeded"}.
				withSequence(0),
		},
		domainEventPlayerCase{
			label: "place-block-succeeded-acknowledged-sequence",
			input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "place-block-succeeded"}.
				withSequence(41),
		},
	)

	cases = append(cases,
		domainEventPlayerCase{
			label: "combat-hit-admitted",
			input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "combat-hit"}.
				withServerTick(7).withDamage(1).withTargetKind(int(core.CombatTargetHostile)),
		},
		domainEventPlayerCase{
			label: "combat-hit-maximum-damage",
			input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "combat-hit"}.
				withServerTick(8).withDamage(int(core.MaxHealth)).withTargetKind(int(core.CombatTargetPassive)),
		},
		domainEventPlayerCase{
			label: "combat-hit-zero-server-tick",
			input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "combat-hit"}.
				withServerTick(0).withDamage(1).withTargetKind(int(core.CombatTargetPlayer)),
		},
		domainEventPlayerCase{
			label: "combat-hit-zero-damage",
			input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "combat-hit"}.
				withServerTick(7).withDamage(0).withTargetKind(int(core.CombatTargetPlayer)),
		},
		domainEventPlayerCase{
			label: "combat-hit-damage-above-max",
			input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "combat-hit"}.
				withServerTick(7).withDamage(int(core.MaxHealth) + 1).withTargetKind(int(core.CombatTargetPlayer)),
		},
		domainEventPlayerCase{
			label: "combat-hit-unknown-target-kind-zero",
			input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "combat-hit"}.
				withServerTick(7).withDamage(1).withTargetKind(0),
		},
		domainEventPlayerCase{
			label: "combat-hit-unknown-target-kind-above-passive",
			input: domainEventPlayerInput{Consumer: domainEventPlayerConsumer, Rule: "combat-hit"}.
				withServerTick(7).withDamage(1).withTargetKind(int(core.CombatTargetPassive) + 1),
		},
	)
	return cases
}

func (in domainEventPlayerInput) withServerTick(tick uint64) domainEventPlayerInput {
	in.ServerTick = &tick
	return in
}

// runDomainEventPlayer executes one corpus case through the current Go
// protocol outcome and player-state DTOs.
//
// The verdict always comes from `protocol.ValidateServerPacket` in the play
// state, which is the same validator the codec applies on both the encode and
// the decode side. The rule name and the rejection category come from the same
// bounds the DTO reads, and the two are cross-checked against each other, so a
// classification that disagrees with the authority fails the run instead of
// publishing a plausible-looking rejection.
func runDomainEventPlayer(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainEventPlayerDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainEventPlayerConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainEventPlayerConsumer)
	}

	switch spec.Rule {
	case "player-state":
		return domainEventPlayerRunPlayerState(c, spec)
	case "command-rejected":
		return domainEventPlayerRunCommandRejected(c, spec)
	case "place-block-succeeded":
		return domainEventPlayerRunPlacementSuccess(c, spec)
	case "combat-hit":
		return domainEventPlayerRunCombatHit(c, spec)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}
}

// domainEventPlayerRunPlayerState admits one player publication, which is the
// only record in this family that carries a field map of its own.
func domainEventPlayerRunPlayerState(c CaseSpec, spec domainEventPlayerInput) (Outcome, []byte, error) {
	state, err := domainEventPlayerPlayerState(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventPlayerNormalizePlayerState(state)
	category, rule := domainEventPlayerPlayerStateRule(state)
	return domainEventPlayerFinish(c, "player-state", fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, state) == nil)
}

// domainEventPlayerRunCommandRejected admits one rejection, whose entire
// payload is the sequence and the reason.
func domainEventPlayerRunCommandRejected(c CaseSpec, spec domainEventPlayerInput) (Outcome, []byte, error) {
	if spec.Sequence == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s requires a sequence", c.ID)
	}
	if spec.Reason == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s requires a reason", c.ID)
	}
	rejection := protocol.CommandRejected{Sequence: *spec.Sequence, Reason: protocol.RejectReason(*spec.Reason)}
	fields := map[string]any{
		"sequence": strconv.FormatUint(rejection.Sequence, 10),
		"reason":   string(rejection.Reason),
	}
	category, rule := domainEventPlayerRejectionRule(rejection)
	return domainEventPlayerFinish(c, "command-rejected", fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, rejection) == nil)
}

// domainEventPlayerRunPlacementSuccess admits one placement success. The Go
// packet carries no rule of its own beyond the registry entry, so the
// validator admits every sequence including zero and the classifier has
// nothing to name.
func domainEventPlayerRunPlacementSuccess(c CaseSpec, spec domainEventPlayerInput) (Outcome, []byte, error) {
	if spec.Sequence == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s requires a sequence", c.ID)
	}
	success := protocol.PlaceBlockSucceeded{Sequence: *spec.Sequence}
	fields := map[string]any{
		"sequence": strconv.FormatUint(success.Sequence, 10),
	}
	return domainEventPlayerFinish(c, "place-block-succeeded", fields, "", "", protocol.ValidateServerPacket(protocol.StatePlay, success) == nil)
}

// domainEventPlayerRunCombatHit admits one combat hit, whose entire payload is
// the server tick, the damage and the target kind.
func domainEventPlayerRunCombatHit(c CaseSpec, spec domainEventPlayerInput) (Outcome, []byte, error) {
	serverTick, err := domainEventPlayerUint64(c, "server_tick", spec.ServerTick)
	if err != nil {
		return Outcome{}, nil, err
	}
	damage, err := domainEventPlayerByte(c, "damage", spec.Damage)
	if err != nil {
		return Outcome{}, nil, err
	}
	targetKind, err := domainEventPlayerByte(c, "target_kind", spec.TargetKind)
	if err != nil {
		return Outcome{}, nil, err
	}
	hit := protocol.CombatHit{
		ServerTick: serverTick,
		Damage:     damage,
		TargetKind: core.CombatTargetKind(targetKind),
	}
	fields := map[string]any{
		"server_tick": strconv.FormatUint(hit.ServerTick, 10),
		"damage":      int(hit.Damage),
		"target_kind": int(hit.TargetKind),
	}
	category, rule := domainEventPlayerCombatHitRule(hit)
	return domainEventPlayerFinish(c, "combat-hit", fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, hit) == nil)
}

// domainEventPlayerFinish records one admission.
//
// The classification is cross-checked against the authority's verdict, a
// rejection publishes the broken rule beside its category, and an admitted
// record publishes its normalized field map. No codec round trip is recorded:
// this family pins the semantic field values, and the wire layout of these
// packets belongs to the protocol nodes that port the codecs.
func domainEventPlayerFinish(c CaseSpec, subject string, fields map[string]any, category, rule string, admitted bool) (Outcome, []byte, error) {
	if admitted == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with the protocol validator (admitted=%v)", c.ID, rule, admitted)
	}
	if !admitted {
		fields["rule"] = rule
		return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
	}
	return Outcome{Kind: "ok", Category: subject, Fields: fields}, nil, nil
}

// domainEventPlayerPlayerState resolves one player publication from its frozen
// envelope. Every field is required, because the record is complete by
// definition: an absent field would silently become a zero and a zero is a
// meaningful value on most of them.
func domainEventPlayerPlayerState(c CaseSpec, spec domainEventPlayerInput) (protocol.PlayerState, error) {
	serverTick, err := domainEventPlayerUint64(c, "server_tick", spec.ServerTick)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	lastInputSequence, err := domainEventPlayerUint64(c, "last_input_sequence", spec.LastInputSequence)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	if spec.Dimension == nil {
		return protocol.PlayerState{}, fmt.Errorf("runtime-oracle: case %s requires a dimension", c.ID)
	}
	position, err := domainEventPlayerVec3(c, "position", spec.Position)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	velocity, err := domainEventPlayerVec3(c, "velocity", spec.Velocity)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	yaw, err := domainEventPlayerAngle(c, "yaw", spec.Yaw)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	pitch, err := domainEventPlayerAngle(c, "pitch", spec.Pitch)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	onGround, err := domainEventPlayerFlag(c, "on_ground", spec.OnGround)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	ready, err := domainEventPlayerFlag(c, "ready", spec.Ready)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	reset, err := domainEventPlayerFlag(c, "reset", spec.Reset)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	miningActive, err := domainEventPlayerFlag(c, "mining_active", spec.MiningActive)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	miningTarget, err := domainEventPlayerBlockPos(c, "mining_target", spec.MiningTarget)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	miningProgress, err := domainEventPlayerUint16(c, "mining_progress", spec.MiningProgress)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	miningRequired, err := domainEventPlayerUint16(c, "mining_required", spec.MiningRequired)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	miningHarvestable, err := domainEventPlayerFlag(c, "mining_harvestable", spec.MiningHarvestable)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	health, err := domainEventPlayerByte(c, "health", spec.Health)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	oxygen, err := domainEventPlayerUint16(c, "oxygen", spec.Oxygen)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	hunger, err := domainEventPlayerByte(c, "hunger", spec.Hunger)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	saturationZero, err := domainEventPlayerFlag(c, "saturation_zero", spec.SaturationZero)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	dayPhaseOffset, err := domainEventPlayerUint16(c, "day_phase_offset", spec.DayPhaseOffset)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	worldTimeTicks, err := domainEventPlayerUint64(c, "world_time_ticks", spec.WorldTimeTicks)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	weatherKind, err := domainEventPlayerByte(c, "weather_kind", spec.WeatherKind)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	season, err := domainEventPlayerByte(c, "season", spec.Season)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	seasonProgress, err := domainEventPlayerByte(c, "season_progress", spec.SeasonProgress)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	temperature, err := domainEventPlayerInt8(c, "temperature", spec.Temperature)
	if err != nil {
		return protocol.PlayerState{}, err
	}
	armorPoints, err := domainEventPlayerByte(c, "armor_points", spec.ArmorPoints)
	if err != nil {
		return protocol.PlayerState{}, err
	}

	return protocol.PlayerState{
		ServerTick:          serverTick,
		LastInputSequence:   lastInputSequence,
		Dimension:           core.DimensionID(*spec.Dimension),
		Position:            position,
		Velocity:            velocity,
		Yaw:                 yaw,
		Pitch:               pitch,
		OnGround:            onGround,
		Ready:               ready,
		Reset:               reset,
		MiningActive:        miningActive,
		MiningTarget:        miningTarget,
		MiningProgressTicks: miningProgress,
		MiningRequiredTicks: miningRequired,
		MiningHarvestable:   miningHarvestable,
		Health:              health,
		Oxygen:              oxygen,
		Hunger:              hunger,
		SaturationZero:      saturationZero,
		DayPhaseOffset:      dayPhaseOffset,
		WorldTimeTicks:      worldTimeTicks,
		WeatherKind:         core.WeatherKind(weatherKind),
		Season:              core.Season(season),
		SeasonProgress:      seasonProgress,
		Temperature:         temperature,
		ArmorPoints:         armorPoints,
	}, nil
}

// domainEventPlayerNormalizePlayerState renders one player publication in the
// normalized field map. The two u64 fields are decimal strings and the five
// f32 fields are their IEEE-754 bit strings, so a signed zero stays observable
// instead of collapsing onto its positive counterpart.
func domainEventPlayerNormalizePlayerState(state protocol.PlayerState) map[string]any {
	return map[string]any{
		"server_tick":         strconv.FormatUint(state.ServerTick, 10),
		"last_input_sequence": strconv.FormatUint(state.LastInputSequence, 10),
		"dimension":           int(state.Dimension),
		"position":            domainEventPlayerVectorBits(state.Position),
		"velocity":            domainEventPlayerVectorBits(state.Velocity),
		"yaw":                 domainEventPlayerFloatBits(state.Yaw),
		"pitch":               domainEventPlayerFloatBits(state.Pitch),
		"on_ground":           state.OnGround,
		"ready":               state.Ready,
		"reset":               state.Reset,
		"mining_active":       state.MiningActive,
		"mining_target": []int{
			int(state.MiningTarget.X),
			int(state.MiningTarget.Y),
			int(state.MiningTarget.Z),
		},
		"mining_progress":    int(state.MiningProgressTicks),
		"mining_required":    int(state.MiningRequiredTicks),
		"mining_harvestable": state.MiningHarvestable,
		"health":             int(state.Health),
		"oxygen":             int(state.Oxygen),
		"hunger":             int(state.Hunger),
		"saturation_zero":    state.SaturationZero,
		"day_phase_offset":   int(state.DayPhaseOffset),
		"world_time_ticks":   strconv.FormatUint(state.WorldTimeTicks, 10),
		"weather_kind":       int(state.WeatherKind),
		"season":             int(state.Season),
		"season_progress":    int(state.SeasonProgress),
		"temperature":        int(state.Temperature),
		"armor_points":       int(state.ArmorPoints),
	}
}

// domainEventPlayerVectorBits renders one f32 vector as the three hexadecimal
// IEEE-754 bit strings the normalized corpus vocabulary uses.
func domainEventPlayerVectorBits(vector mgl32.Vec3) []string {
	return []string{
		domainEventPlayerFloatBits(vector.X()),
		domainEventPlayerFloatBits(vector.Y()),
		domainEventPlayerFloatBits(vector.Z()),
	}
}

// domainEventPlayerFloatBits renders one float32 as the eight-digit
// hexadecimal IEEE-754 bit string the normalized corpus vocabulary uses.
func domainEventPlayerFloatBits(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// domainEventPlayerFloatText renders one float32 as decimal float text, so
// NaN, the infinities and a signed zero stay expressible in the frozen input.
// The normalized outcome renders the same value as its IEEE-754 bit string, so
// the input and the outcome pin the value in two independent renderings.
func domainEventPlayerFloatText(value float32) string {
	return strconv.FormatFloat(float64(value), 'g', -1, 32)
}

// domainEventPlayerFloatTextPointer renders one optional float32 field as
// decimal float text, keeping an absent field distinguishable from a zero.
func domainEventPlayerFloatTextPointer(value float32) *string {
	text := domainEventPlayerFloatText(value)
	return &text
}

// domainEventPlayerPlayerStateRule names the first rule one player publication
// breaks, in the same order the Go validator checks them: the dimension, the
// three finite float fields, the four bounded scalars, the two enumerated
// scalars, and finally the mining union, whose inactive member has to be
// entirely empty and whose active member needs a progress strictly inside its
// requirement.
func domainEventPlayerPlayerStateRule(state protocol.PlayerState) (string, string) {
	if state.Dimension != core.Overworld && state.Dimension != core.Depths {
		return "invalid-enum", "player_state.dimension"
	}
	if !domainEventPlayerFiniteVec3(state.Position) {
		return "invalid-value", "player_state.finite_position"
	}
	if !domainEventPlayerFiniteVec3(state.Velocity) {
		return "invalid-value", "player_state.finite_velocity"
	}
	if !domainEventPlayerFinite(state.Yaw) || !domainEventPlayerFinite(state.Pitch) {
		return "invalid-value", "player_state.finite_rotation"
	}
	if !core.ValidHealth(state.Health) {
		return "invalid-value", "player_state.health_range"
	}
	if !core.ValidOxygen(state.Oxygen) {
		return "invalid-value", "player_state.oxygen_range"
	}
	if !core.ValidHunger(state.Hunger) {
		return "invalid-value", "player_state.hunger_range"
	}
	if state.DayPhaseOffset >= core.DayLengthTicks {
		return "invalid-value", "player_state.day_phase_offset_range"
	}
	if state.WeatherKind > core.WeatherThunder {
		return "invalid-enum", "player_state.weather_kind"
	}
	if state.Season > core.SeasonWinter {
		return "invalid-enum", "player_state.season"
	}
	if state.ArmorPoints > core.MaxArmorPoints {
		return "invalid-value", "player_state.armor_points_range"
	}
	if !state.MiningActive {
		if state.MiningTarget != (core.BlockPos{}) || state.MiningProgressTicks != 0 ||
			state.MiningRequiredTicks != 0 || state.MiningHarvestable {
			return "invalid-value", "player_state.mining_inactive_residue"
		}
		return "", ""
	}
	if state.MiningProgressTicks == 0 || state.MiningProgressTicks >= state.MiningRequiredTicks {
		return "invalid-value", "player_state.mining_progress_range"
	}
	return "", ""
}

// domainEventPlayerRejectionRule names the rule a rejection breaks: the Go
// registry decides which reason names are published, and an unknown one is an
// enum rejection rather than a value rejection.
func domainEventPlayerRejectionRule(rejection protocol.CommandRejected) (string, string) {
	if _, ok := protocol.CommandRejectReasonID(rejection.Reason); !ok {
		return "invalid-enum", "command_rejected.registered_reason"
	}
	return "", ""
}

// domainEventPlayerCombatHitRule names the first rule one combat hit breaks,
// in the same order the Go validator checks them: the tick, the damage, then
// the target kind.
func domainEventPlayerCombatHitRule(hit protocol.CombatHit) (string, string) {
	if hit.ServerTick == 0 {
		return "invalid-value", "combat_hit.server_tick_nonzero"
	}
	if hit.Damage == 0 || hit.Damage > core.MaxHealth {
		return "invalid-value", "combat_hit.damage_range"
	}
	if !hit.TargetKind.Valid() {
		return "invalid-enum", "combat_hit.target_kind"
	}
	return "", ""
}

// domainEventPlayerFinite reports whether one float32 is finite, which is the
// rule every angle and vector component in this family shares.
func domainEventPlayerFinite(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

// domainEventPlayerFiniteVec3 reports whether every component of one vector is
// finite.
func domainEventPlayerFiniteVec3(vector mgl32.Vec3) bool {
	return domainEventPlayerFinite(vector.X()) &&
		domainEventPlayerFinite(vector.Y()) &&
		domainEventPlayerFinite(vector.Z())
}

// domainEventPlayerUint64 resolves one unsigned 64-bit field a case names.
func domainEventPlayerUint64(c CaseSpec, name string, value *uint64) (uint64, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	return *value, nil
}

// domainEventPlayerVec3 resolves one three-component float vector from its
// decimal float texts. NaN, the infinities and a signed zero are all
// expressible, because those exact values are the boundaries the finite rule
// pins.
func domainEventPlayerVec3(c CaseSpec, name string, components []string) (mgl32.Vec3, error) {
	if len(components) != 3 {
		return mgl32.Vec3{}, fmt.Errorf("runtime-oracle: case %s requires three %s components", c.ID, name)
	}
	values := make([]float32, 3)
	for index, text := range components {
		parsed, err := strconv.ParseFloat(text, 32)
		if err != nil {
			return mgl32.Vec3{}, fmt.Errorf("runtime-oracle: case %s names %s component %d %q, which is not a float32: %w", c.ID, name, index, text, err)
		}
		values[index] = float32(parsed)
	}
	return mgl32.Vec3{values[0], values[1], values[2]}, nil
}

// domainEventPlayerAngle resolves one look angle from its decimal float text.
func domainEventPlayerAngle(c CaseSpec, name string, value *string) (float32, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	parsed, err := strconv.ParseFloat(*value, 32)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not a float32: %w", c.ID, name, *value, err)
	}
	return float32(parsed), nil
}

// domainEventPlayerBlockPos resolves one block position from its three
// coordinates. The coordinates carry no range rule of their own, so the
// resolver only reports a missing or malformed field.
func domainEventPlayerBlockPos(c CaseSpec, name string, coordinates []int32) (core.BlockPos, error) {
	if len(coordinates) != 3 {
		return core.BlockPos{}, fmt.Errorf("runtime-oracle: case %s requires three %s coordinates", c.ID, name)
	}
	return core.BlockPos{X: coordinates[0], Y: coordinates[1], Z: coordinates[2]}, nil
}

// domainEventPlayerFlag resolves one boolean field. A false flag is a value,
// not an absence, so the pointer is required rather than defaulted.
func domainEventPlayerFlag(c CaseSpec, name string, value *bool) (bool, error) {
	if value == nil {
		return false, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	return *value, nil
}

// domainEventPlayerByte resolves one u8 field a case names as a decimal
// integer.
func domainEventPlayerByte(c CaseSpec, name string, value *int) (uint8, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	if *value < 0 || *value > math.MaxUint8 {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %d, which is outside 0..255", c.ID, name, *value)
	}
	return uint8(*value), nil
}

// domainEventPlayerUint16 resolves one u16 field a case names as a decimal
// integer.
func domainEventPlayerUint16(c CaseSpec, name string, value *int) (uint16, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	if *value < 0 || *value > math.MaxUint16 {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %d, which is outside 0..65535", c.ID, name, *value)
	}
	return uint16(*value), nil
}

// domainEventPlayerInt8 resolves one i8 field a case names as a decimal
// integer.
func domainEventPlayerInt8(c CaseSpec, name string, value *int) (int8, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	if *value < math.MinInt8 || *value > math.MaxInt8 {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %d, which is outside -128..127", c.ID, name, *value)
	}
	return int8(*value), nil
}

// domainEventPlayerDecodeInput reads one frozen corpus input and rejects
// trailing content, so a producer never executes bytes the case did not name.
func domainEventPlayerDecodeInput(data []byte) (domainEventPlayerInput, error) {
	var spec domainEventPlayerInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainEventPlayerInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainEventPlayerInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainEventPlayerRecord pairs one executed case with the input and outcome
// the producer produced for it.
type domainEventPlayerRecord struct {
	label   string
	input   domainEventPlayerInput
	outcome Outcome
}

// domainEventPlayerExecute runs the whole case table through the producer and
// returns one record per case in table order.
func domainEventPlayerExecute(t *testing.T) []domainEventPlayerRecord {
	t.Helper()

	cases := domainEventPlayerCases()
	records := make([]domainEventPlayerRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainEventPlayer(CaseSpec{ID: domainEventPlayerCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainEventPlayerRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// domainEventPlayerSyncCorpus writes or verifies the frozen corpus files.
//
// An ordinary run is read-only: it proves the committed files still match what
// the protocol DTOs produce now, so a drifted artifact fails instead of being
// regenerated. The explicit update flag rewrites them, which is the only way a
// frozen artifact changes.
func domainEventPlayerSyncCorpus(t *testing.T, records []domainEventPlayerRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainEventPlayerCorpusRelDir))
	want := make(map[string][]byte, len(records)*2)
	for _, record := range records {
		input, err := json.MarshalIndent(record.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", record.label, err)
		}
		outcome, err := json.MarshalIndent(record.outcome, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus outcome for %s: %v", record.label, err)
		}
		want[record.label+".input.json"] = append(input, '\n')
		want[record.label+".expected.json"] = append(outcome, '\n')
	}

	if *updateDomainEventPlayerCorpus {
		for relative, data := range want {
			target := filepath.Join(corpusDir, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create corpus directory: %v", err)
			}
			if err := os.WriteFile(target, data, 0o644); err != nil {
				t.Fatalf("write %s: %v", relative, err)
			}
		}
		return
	}

	for relative, data := range want {
		target := filepath.Join(corpusDir, filepath.FromSlash(relative))
		committed, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read frozen corpus case %s: %v (rerun with -update-domain-event-player-corpus after reviewing the change)", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed protocol DTO (rerun with -update-domain-event-player-corpus after reviewing the change)", relative)
		}
	}

	var committed []string
	if err := filepath.WalkDir(corpusDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		relative, relErr := filepath.Rel(corpusDir, path)
		if relErr != nil {
			return relErr
		}
		committed = append(committed, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		t.Fatalf("walk corpus directory: %v", err)
	}
	sort.Strings(committed)
	for _, relative := range committed {
		if _, expected := want[relative]; !expected {
			t.Errorf("frozen corpus case %s is not produced by any executed table row", relative)
		}
	}
}

// domainEventPlayerCaseID renders the manifest case identity one corpus label
// carries. The version segment is the family's published version, because a
// case has to name the version of the family it belongs to.
func domainEventPlayerCaseID(label string) string {
	return domainEventPlayerFamily + "/" + domainEventPlayerVersion + "/" + label
}

// domainEventPlayerWorkingManifest assembles the manifest this node executes
// inside a harness-owned temporary directory.
//
// The frozen manifest carries the `domain.event` family but no case for it,
// because the controller merges manifest fragments after acceptance. The
// working manifest is therefore a family-scoped selection: `Cases` holds
// exactly the cases the committed corpus registers, every other family's case
// list is cleared because `Reconcile` requires each family's list to match the
// cases this selection registers for it, and the `domain.event` entry keeps
// its discovered identity — kind, role, versions, source, eventual owner and
// numeric semantics — while its provenance and case list are replaced with
// this producer's. Registering the family in the live registry is deliberately
// left to the manifest merge.
func domainEventPlayerWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainEventPlayerCorpusCases(t, root)

	cloned := Inventory{
		SchemaVersion:  frozen.SchemaVersion,
		SourceRevision: frozen.SourceRevision,
		Identities:     frozen.Identities,
		Families:       append([]Family(nil), frozen.Families...),
		Cases:          cases,
	}
	caseIDs := make([]string, 0, len(cases))
	for _, c := range cases {
		caseIDs = append(caseIDs, c.ID)
	}
	sort.Strings(caseIDs)
	sources := make([]SourceSpec, 0, len(domainEventPlayerFamilySources))
	for _, relative := range domainEventPlayerFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	registered := false
	for index := range cloned.Families {
		if cloned.Families[index].ID != domainEventPlayerFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		cloned.Families[index].Sources = sources
		cloned.Families[index].Cases = caseIDs
		registered = true
	}
	if !registered {
		t.Fatalf("working manifest has no %s family", domainEventPlayerFamily)
	}

	encoded, err := encodeInventory(cloned)
	if err != nil {
		t.Fatalf("encode working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load working manifest: %v", err)
	}
	return loaded
}

// domainEventPlayerCorpusCases reads the frozen corpus and registers one case
// per committed input, with digests proven against the files on disk.
func domainEventPlayerCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainEventPlayerCorpusRelDir))
	var inputs []string
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".input.json") {
			inputs = append(inputs, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk event player corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no event player case under %s", domainEventPlayerCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainEventPlayerLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainEventPlayerReadEnvelope(t, input)
		if envelope.Consumer != domainEventPlayerConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainEventPlayerConsumer)
		}
		if strings.TrimSpace(envelope.Rule) == "" {
			t.Fatalf("corpus case %s names no rule", label)
		}

		inputHash, err := hashFile(input)
		if err != nil {
			t.Fatalf("hash %s: %v", label, err)
		}
		expectedPath := strings.TrimSuffix(input, ".input.json") + ".expected.json"
		expectedHash, err := hashFile(expectedPath)
		if err != nil {
			t.Fatalf("hash %s: %v", label+".expected.json", err)
		}
		cases = append(cases, CaseSpec{
			ID:           domainEventPlayerCaseID(label),
			Family:       domainEventPlayerFamily,
			Version:      domainEventPlayerVersion,
			Operation:    domainEventPlayerOperation,
			Input:        AssetRef{Path: domainEventPlayerCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainEventPlayerCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainEventPlayerConsumer,
		})
	}
	return cases
}

// domainEventPlayerReadEnvelope reads the provenance envelope of one frozen
// corpus input.
func domainEventPlayerReadEnvelope(t *testing.T, path string) domainEventPlayerInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainEventPlayerInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainEventPlayerCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainEventPlayerCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainEventPlayerOracleExecutesEveryCase runs the whole case table
// through the real Go protocol DTOs, proves the frozen corpus still matches
// what they produce, and then runs the same cases through the production
// runner so the executed evidence satisfies the completeness rules a published
// trace report does.
func TestDomainEventPlayerOracleExecutesEveryCase(t *testing.T) {
	records := domainEventPlayerExecute(t)
	domainEventPlayerSyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainEventPlayerWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainEventPlayerAssertCaseSpecs(t, root, manifest)
	domainEventPlayerAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainEventPlayerCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
		expected := readExpectedOutcome(t, root, c)
		if !outcomesEqual(obs.Outcome, expected) {
			t.Fatalf("case %s produced %#v, want %#v", obs.CaseID, obs.Outcome, expected)
		}
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTrace(trace, manifest); err != nil {
		t.Fatalf("executed event player evidence failed trace validation: %v", err)
	}
}

// TestDomainEventPlayerOracleCaseIdentitiesAreTheRustDomainConsumer pins the
// manifest identity of every event player case: the operation, the family
// version, the family and the consumer the change names for the Rust crate
// that owns these records.
func TestDomainEventPlayerOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPlayerWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainEventPlayerOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainEventPlayerOperation)
		}
		if c.Version != domainEventPlayerVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainEventPlayerVersion)
		}
		if c.Family != domainEventPlayerFamily {
			t.Fatalf("case %s declares family %q, want %q", c.ID, c.Family, domainEventPlayerFamily)
		}
		if c.RustConsumer != domainEventPlayerConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainEventPlayerConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainEventPlayerLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainEventPlayerReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainEventPlayerConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainEventPlayerConsumer)
		}
	}
}

// TestDomainEventPlayerOracleWorkingManifestDescribesItself proves the working
// manifest is internally consistent: every case passes the production case
// validation, the family's provenance hashes match disk, and the family's case
// list matches exactly the cases the selection registers.
func TestDomainEventPlayerOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPlayerWorkingManifest(t, root)
	domainEventPlayerAssertCaseSpecs(t, root, manifest)

	family, ok := domainEventPlayerFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainEventPlayerFamily)
	}
	if family.Role != "event" || family.Kind != "domain" {
		t.Fatalf("%s declares kind %q role %q, want domain event", domainEventPlayerFamily, family.Kind, family.Role)
	}
	if family.CurrentVersion != domainEventPlayerVersion {
		t.Fatalf("%s declares version %q, want %q", domainEventPlayerFamily, family.CurrentVersion, domainEventPlayerVersion)
	}
	if len(family.Sources) != len(domainEventPlayerFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainEventPlayerFamily, len(family.Sources), len(domainEventPlayerFamilySources))
	}
	for _, source := range family.Sources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(source.Path)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", source.Path, err)
		}
		if hash != source.SHA256 {
			t.Fatalf("provenance source %s records %s, disk has %s", source.Path, source.SHA256, hash)
		}
	}

	registered := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		registered[c.ID] = true
	}
	if len(family.Cases) != len(registered) {
		t.Fatalf("%s lists %d cases, want %d", domainEventPlayerFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainEventPlayerFamily, id)
		}
	}
}

// TestDomainEventPlayerOracleOutcomesDistinguishAcceptedAndRejected pins that
// the producer is not returning one constant answer, that every rule records
// both an admitted record and a rejection, and that the boundaries the Rust
// event test enumerates are each present in the executed evidence.
func TestDomainEventPlayerOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainEventPlayerExecute(t)
	domainEventPlayerAssertOutcomesDistinguishCases(t, records)

	tally := make(map[string]*domainEventPlayerRuleTally)
	for _, record := range records {
		entry, ok := tally[record.input.Rule]
		if !ok {
			entry = &domainEventPlayerRuleTally{}
			tally[record.input.Rule] = entry
		}
		switch record.outcome.Kind {
		case "ok":
			entry.accepted = append(entry.accepted, record.outcome.Fields)
		case "error":
			entry.rejected = append(entry.rejected, record.outcome.Fields)
		}
	}
	for rule, entry := range tally {
		// A rule that records no rejection has no admission boundary to pair,
		// which is the placement success: the Go packet carries no rule.
		if len(entry.rejected) == 0 {
			continue
		}
		if len(entry.accepted) == 0 {
			t.Fatalf("rule %s records %d accepted and %d rejected cases, want both non-zero",
				rule, len(entry.accepted), len(entry.rejected))
		}
	}

	if !domainEventPlayerFieldsContain(tally["player-state"].accepted, "health", 20) {
		t.Fatal("no admitted player publication carries the maximum health, which the seed pins")
	}
	if !domainEventPlayerFieldsContain(tally["player-state"].accepted, "oxygen", 300) {
		t.Fatal("no admitted player publication carries the maximum oxygen")
	}
	if !domainEventPlayerFieldsContain(tally["player-state"].accepted, "day_phase_offset", 23999) {
		t.Fatal("no admitted player publication carries the last offset of the day")
	}
	if !domainEventPlayerFieldsContain(tally["player-state"].accepted, "season_progress", 255) {
		t.Fatal("no admitted player publication carries the maximum season progress")
	}
	if !domainEventPlayerFieldsContain(tally["player-state"].accepted, "temperature", -128) {
		t.Fatal("no admitted player publication carries the minimum temperature")
	}
	if !domainEventPlayerFieldsContain(tally["player-state"].accepted, "mining_active", true) {
		t.Fatal("no admitted player publication carries an active mining block")
	}
	if !domainEventPlayerFieldsContain(tally["player-state"].accepted, "mining_progress", 1) {
		t.Fatal("no admitted player publication carries a mining progress of one tick")
	}
	for _, boundary := range []struct {
		field string
		value any
		rule  string
	}{
		{"health", 21, "player_state.health_range"},
		{"oxygen", 301, "player_state.oxygen_range"},
		{"hunger", 21, "player_state.hunger_range"},
		{"armor_points", 21, "player_state.armor_points_range"},
		{"day_phase_offset", 24000, "player_state.day_phase_offset_range"},
		{"weather_kind", 3, "player_state.weather_kind"},
		{"season", 4, "player_state.season"},
		{"dimension", 2, "player_state.dimension"},
	} {
		if !domainEventPlayerFieldsContain(tally["player-state"].rejected, boundary.field, boundary.value) {
			t.Fatalf("no rejected player publication names %s %v, which is outside the published range", boundary.field, boundary.value)
		}
		if !domainEventPlayerRejectionNamesRule(tally, boundary.rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", boundary.rule)
		}
	}
	if !domainEventPlayerFieldsContain(tally["player-state"].rejected, "mining_target", []int{1, 2, 3}) {
		t.Fatal("no rejected player publication carries a nonzero target on an inactive mining block")
	}
	if !domainEventPlayerRejectionNamesRule(tally, "player_state.mining_inactive_residue") {
		t.Fatal("no rejection names the inactive mining residue rule, so the union is not pinned")
	}
	if !domainEventPlayerFieldsContain(tally["player-state"].rejected, "mining_progress", 2) {
		t.Fatal("no rejected player publication carries a mining progress at its requirement")
	}
	if !domainEventPlayerRejectionNamesRule(tally, "player_state.mining_progress_range") {
		t.Fatal("no rejection names the mining progress rule, so the active union member is not pinned")
	}

	if !domainEventPlayerFieldsContain(tally["command-rejected"].accepted, "sequence", "0") {
		t.Fatal("no admitted rejection carries a zero sequence, which the Go wire accepts")
	}
	if !domainEventPlayerFieldsContain(tally["command-rejected"].accepted, "reason", "not_armor") {
		t.Fatal("no admitted rejection names the last published reason")
	}
	if !domainEventPlayerRejectionNamesRule(tally, "command_rejected.registered_reason") {
		t.Fatal("no rejection names the registered reason rule, so an unknown reason is not pinned")
	}
	if !domainEventPlayerFieldsContain(tally["place-block-succeeded"].accepted, "sequence", "0") {
		t.Fatal("no admitted placement success carries a zero sequence")
	}

	if !domainEventPlayerFieldsContain(tally["combat-hit"].accepted, "damage", 20) {
		t.Fatal("no admitted combat hit carries the maximum damage")
	}
	if !domainEventPlayerFieldsContain(tally["combat-hit"].accepted, "target_kind", 3) {
		t.Fatal("no admitted combat hit names the passive target kind")
	}
	for _, boundary := range []struct {
		field string
		value any
		rule  string
	}{
		{"server_tick", "0", "combat_hit.server_tick_nonzero"},
		{"damage", 0, "combat_hit.damage_range"},
		{"damage", 21, "combat_hit.damage_range"},
		{"target_kind", 0, "combat_hit.target_kind"},
		{"target_kind", 4, "combat_hit.target_kind"},
	} {
		if !domainEventPlayerFieldsContain(tally["combat-hit"].rejected, boundary.field, boundary.value) {
			t.Fatalf("no rejected combat hit names %s %v, which is outside the published range", boundary.field, boundary.value)
		}
		if !domainEventPlayerRejectionNamesRule(tally, boundary.rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", boundary.rule)
		}
	}
}

// TestDomainEventPlayerOracleRunnerRejectsUnregisteredFamily pins that a
// manifest naming a family this package cannot execute fails the run.
func TestDomainEventPlayerOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPlayerWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainEventPlayerOracleRunnerRejectsOperationFamilyMismatch pins that a
// case cannot declare one operation and be executed by another.
func TestDomainEventPlayerOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPlayerWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation-family mismatch failure, got: %v", err)
	}
}

// TestDomainEventPlayerOracleRunnerRejectsTamperedInput pins that a case whose
// committed bytes no longer match their recorded digest fails the run instead
// of executing bytes the manifest does not name.
func TestDomainEventPlayerOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPlayerWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainEventPlayerOracleReportPublishesAndValidates assembles the
// executed evidence into a report, publishes it through the production atomic
// exporter, reloads it and validates it against the working manifest. This is
// the identity and content check a later Rust acceptance step performs; it
// does not claim any Rust behaviour.
func TestDomainEventPlayerOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventPlayerWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTrace(trace, manifest); err != nil {
		t.Fatalf("executed event player evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainEventPlayerCorpusReportName)
	if err := ExportTrace(root, target, trace, manifest); err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	loaded, err := LoadTrace(target, manifest)
	if err != nil {
		t.Fatalf("published report does not validate: %v", err)
	}
	if loaded.SchemaVersion != traceSchemaVersion {
		t.Fatalf("report schema_version = %d, want %d", loaded.SchemaVersion, traceSchemaVersion)
	}
	if loaded.SourceRevision != manifest.SourceRevision {
		t.Fatalf("report source revision = %s, want %s", loaded.SourceRevision, manifest.SourceRevision)
	}
	if len(loaded.Inputs) != len(manifest.Cases) || len(loaded.Observations) != len(manifest.Cases) {
		t.Fatalf("report carries %d inputs and %d observations, want %d each",
			len(loaded.Inputs), len(loaded.Observations), len(manifest.Cases))
	}
}

// TestDomainEventPlayerOracleRejectsMissingProducerTest pins that the executed
// evidence has a real producer behind it, including the topic-named entry
// point the domain plan's filter selects. A corpus whose producer test is gone
// has no independent execution, only frozen files, and a missing topic entry
// point would make the plan filter pass without running anything.
func TestDomainEventPlayerOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainEventPlayerProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainEventPlayerProducerTestRelPath, err)
	}
	for _, name := range []string{domainEventPlayerProducerExecuteName, domainEventPlayerProducerTopicName} {
		declaration := "func " + name + "(t *testing.T) {"
		if !strings.Contains(string(data), declaration) {
			t.Fatalf("%s does not declare %s", domainEventPlayerProducerTestRelPath, declaration)
		}
	}
}

// TestDomainOracle_event_player is the topic-named entry point the domain plan
// names for this node. It delegates to the same executed table, so the two
// filters select one source of expected results rather than two.
func TestDomainOracle_event_player(t *testing.T) {
	TestDomainEventPlayerOracleExecutesEveryCase(t)
}

// domainEventPlayerRuleTally collects the accepted and rejected normalized
// outcomes one rule records, so a boundary assertion can look for the field
// value it names.
type domainEventPlayerRuleTally struct {
	accepted []map[string]any
	rejected []map[string]any
}

// domainEventPlayerFieldsContain reports whether one recorded outcome set
// holds a record whose named field carries the value a boundary assertion
// names.
func domainEventPlayerFieldsContain(records []map[string]any, field string, want any) bool {
	for _, fields := range records {
		if reflect.DeepEqual(fields[field], want) {
			return true
		}
	}
	return false
}

// domainEventPlayerRejectionNamesRule reports whether any recorded rejection
// names the broken rule a boundary assertion pins.
func domainEventPlayerRejectionNamesRule(tally map[string]*domainEventPlayerRuleTally, rule string) bool {
	for _, entry := range tally {
		for _, fields := range entry.rejected {
			if fields["rule"] == rule {
				return true
			}
		}
	}
	return false
}

// domainEventPlayerAssertCaseSpecs runs the production case validation over
// the whole selection, so a case with a bad path, digest, format or checkpoint
// fails here rather than surfacing later as a confusing coverage failure.
func domainEventPlayerAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
	t.Helper()
	families := make(map[string]Family, len(manifest.Families))
	for _, family := range manifest.Families {
		families[family.ID] = family
	}
	for _, c := range manifest.Cases {
		if err := validateCaseSpec(root, c, families); err != nil {
			t.Fatalf("case %s: %v", c.ID, err)
		}
	}
}

// domainEventPlayerAssertOutcomesDistinguishCases proves the executed evidence
// separates admitted records from rejections instead of publishing one
// constant answer, and that the rejections name a rule the authority
// publishes.
func domainEventPlayerAssertOutcomesDistinguishCases(t *testing.T, records []domainEventPlayerRecord) {
	t.Helper()
	if len(records) == 0 {
		t.Fatal("executed evidence records no case")
	}
	accepted, rejected := 0, 0
	for _, record := range records {
		switch record.outcome.Kind {
		case "ok":
			accepted++
			if record.outcome.Category == "" {
				t.Fatalf("record %s publishes an accepted outcome with no category", record.label)
			}
		case "error":
			rejected++
			if !corpusStructuralCategories[record.outcome.Category] &&
				!corpusAdmissionCategories[record.outcome.Category] &&
				!corpusStorageCategories[record.outcome.Category] {
				t.Fatalf("record %s publishes rejection category %q, which is not in the frozen vocabulary", record.label, record.outcome.Category)
			}
			if record.outcome.Fields["rule"] == nil {
				t.Fatalf("record %s publishes a rejection with no rule name", record.label)
			}
		default:
			t.Fatalf("record %s publishes kind %q, want ok or error", record.label, record.outcome.Kind)
		}
	}
	if accepted == 0 || rejected == 0 {
		t.Fatalf("executed evidence records %d accepted and %d rejected cases, want both non-zero", accepted, rejected)
	}
}

// domainEventPlayerFamilySpec resolves one family from a manifest selection.
func domainEventPlayerFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainEventPlayerFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainEventPlayerCaseByID indexes a manifest selection by case identity.
func domainEventPlayerCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
