package main

import (
	"bytes"
	"encoding/hex"
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

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file is the Go producer for the domain movement and ray command family.
// It executes every committed corpus case through the current Go protocol
// command DTOs: the verdict comes from each DTO's `Validate`, and an admitted
// command is encoded and decoded again through the production codec so the
// frozen evidence carries the exact wire payload the encoder produced. No world
// authority, listener, model call or native ABI is involved: the producer only
// reads immutable corpus inputs, calls in-process validators and moves bytes
// through the real codec, and the frozen corpus files it materializes are
// read-only inputs for later consumers.

const (
	// domainControlFamily is the corpus family this package executes. Its
	// eventual owner is the Rust domain crate, which owns the movement and ray
	// command payloads this family pins.
	domainControlFamily = "domain.command_control"
	// domainControlVersion labels the family as the current value rules rather
	// than a numbered schema: the command payload rules have no version
	// history, so a version number would imply a migration path that does not
	// exist.
	domainControlVersion = "current"
	// domainControlOperation is the manifest operation name for a command
	// admission case. An admitted case additionally round-trips through the
	// codec inside the same admission step, because the payload's wire layout
	// is part of what the case pins.
	domainControlOperation = "admit"
	// domainControlConsumer is the manifest consumer the change pins for this
	// family: the Rust crate that owns these rules.
	domainControlConsumer = "mornlea_domain"
	// domainControlCorpusRelDir is the repository-relative directory holding
	// the frozen command control corpus cases.
	domainControlCorpusRelDir = "testdata/runtime-migration/cases/domain/command_control"
	// domainControlProducerTestRelPath and the two producer test names locate
	// the package-local producer that executes the protocol command DTOs. The
	// topic name is the entry point the domain plan's filter requires; a corpus
	// whose producer test is missing has no independently executed evidence at
	// all.
	domainControlProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_command_control_test.go"
	domainControlProducerExecuteName = "TestDomainCommandControlOracleExecutesEveryCase"
	domainControlProducerTopicName   = "TestDomainOracle_command_control"
	// domainControlCorpusReportName is the published report file name for the
	// executed command control evidence.
	domainControlCorpusReportName = "runtime-corpus-domain-command-control.json"
	// domainControlSource is the primary provenance source of the command
	// payload rules.
	domainControlSource = "packages/shared/network/protocol/message_command.go"
	// domainControlNumericSemantics records the numeric contract this family
	// pins, in the same shape the other families use.
	domainControlNumericSemantics = "finite look angles only; full i8 movement axes; hotbar slot 0..8; resync dimension 0/1; independent held flags; reject non-finite rotation and out-of-range slots"
)

// domainControlFamilySources is the merged provenance set the family records.
// Every entry is a file the producer's rules or wire layout are read from, so a
// change to any of them is a change to the recorded evidence. The container
// message file belongs to the set because the open-container ray intent is
// declared beside the container messages rather than with the other ray
// intents, and its validator is the authority that command's row executes.
var domainControlFamilySources = []string{
	"packages/shared/network/protocol/message_command.go",
	"packages/shared/network/protocol/message_container.go",
	"packages/shared/network/protocol/packet.go",
	"packages/shared/network/codec/codec_client.go",
	"packages/shared/core/item.go",
	"packages/shared/core/block.go",
}

// domainControlLabelPattern is the shape a corpus label must have: a lowercase
// slug with an optional zero-padded numeric suffix, so a boundary row sorts in
// numeric order under a lexical sort.
var domainControlLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// updateDomainControlCorpus rewrites the frozen command control corpus from the
// executed protocol DTOs. It follows the same discipline as the other fixture
// update flags: an ordinary run only compares, so a frozen artifact is never
// silently regenerated to match an implementation.
var updateDomainControlCorpus = flag.Bool(
	"update-domain-command-control-corpus",
	false,
	"rewrite testdata/runtime-migration/cases/domain/command_control from the executed protocol command DTOs",
)

// domainControlInput is the frozen, self-describing corpus input for one case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. Every field is a pointer so an absent
// field and an explicit zero stay distinguishable, which matters because a zero
// sequence, a zero revision, a zero move axis and a false held flag are all
// meaningful values. `Yaw` and `Pitch` are decimal float text rather than JSON
// numbers because a JSON number cannot express NaN, an infinity or a signed
// zero, and those exact values are the boundaries this family pins.
type domainControlInput struct {
	Consumer     string  `json:"consumer"`
	Rule         string  `json:"rule"`
	Sequence     *uint64 `json:"sequence,omitempty"`
	MoveX        *int8   `json:"move_x,omitempty"`
	MoveZ        *int8   `json:"move_z,omitempty"`
	Jump         *bool   `json:"jump,omitempty"`
	Yaw          *string `json:"yaw,omitempty"`
	Pitch        *string `json:"pitch,omitempty"`
	Mining       *bool   `json:"mining,omitempty"`
	Eating       *bool   `json:"eating,omitempty"`
	Sprinting    *bool   `json:"sprinting,omitempty"`
	Sneaking     *bool   `json:"sneaking,omitempty"`
	Slot         *uint8  `json:"slot,omitempty"`
	Dimension    *int32  `json:"dimension,omitempty"`
	ChunkX       *int32  `json:"chunk_x,omitempty"`
	ChunkZ       *int32  `json:"chunk_z,omitempty"`
	HaveRevision *uint64 `json:"have_revision,omitempty"`
}

func (in domainControlInput) withSequence(sequence uint64) domainControlInput {
	in.Sequence = &sequence
	return in
}

func (in domainControlInput) withMoveX(moveX int8) domainControlInput {
	in.MoveX = &moveX
	return in
}

func (in domainControlInput) withMoveZ(moveZ int8) domainControlInput {
	in.MoveZ = &moveZ
	return in
}

func (in domainControlInput) withJump(jump bool) domainControlInput {
	in.Jump = &jump
	return in
}

func (in domainControlInput) withYaw(yaw string) domainControlInput {
	in.Yaw = &yaw
	return in
}

func (in domainControlInput) withPitch(pitch string) domainControlInput {
	in.Pitch = &pitch
	return in
}

func (in domainControlInput) withMining(mining bool) domainControlInput {
	in.Mining = &mining
	return in
}

func (in domainControlInput) withEating(eating bool) domainControlInput {
	in.Eating = &eating
	return in
}

func (in domainControlInput) withSprinting(sprinting bool) domainControlInput {
	in.Sprinting = &sprinting
	return in
}

func (in domainControlInput) withSneaking(sneaking bool) domainControlInput {
	in.Sneaking = &sneaking
	return in
}

func (in domainControlInput) withSlot(slot uint8) domainControlInput {
	in.Slot = &slot
	return in
}

func (in domainControlInput) withDimension(dimension int32) domainControlInput {
	in.Dimension = &dimension
	return in
}

func (in domainControlInput) withChunk(x, z int32) domainControlInput {
	in.ChunkX = &x
	in.ChunkZ = &z
	return in
}

func (in domainControlInput) withHaveRevision(revision uint64) domainControlInput {
	in.HaveRevision = &revision
	return in
}

// domainControlCase is one frozen corpus case: its label and its input, and
// nothing else.
type domainControlCase struct {
	label string
	input domainControlInput
}

// domainControlCases is the ordered case table the producer executes.
//
// The rows are the boundaries the movement and ray command rules name: the full
// i8 axis range and an out-of-look-range but finite pitch that the protocol
// still admits, the non-finite rotation every angle-carrying DTO rejects, the
// four held flags each surviving on their own, the hotbar slot boundary at
// eight and nine, the resync dimension and zero-revision rules, and each ray
// intent sharing the finite-angle rule while retaining the exact signed zero
// bits. The seed row mirrors the payload the domain plan names so a toggle of
// any single field is observable in the recorded outcome.
func domainControlCases() []domainControlCase {
	inputs := []domainControlCase{
		{
			label: "player-input-seed",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "player-input"}.
				withSequence(7).withMoveX(1).withMoveZ(-1).withJump(true).
				withYaw("0.5").withPitch("-0.25").
				withMining(true).withEating(true).withSprinting(true).withSneaking(true),
		},
		{
			label: "player-input-full-axes",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "player-input"}.
				withSequence(1).withMoveX(math.MinInt8).withMoveZ(math.MaxInt8).withJump(false).
				withYaw("0").withPitch("4").
				withMining(false).withEating(false).withSprinting(false).withSneaking(false),
		},
		{
			label: "player-input-non-finite-yaw",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "player-input"}.
				withSequence(2).withMoveX(0).withMoveZ(0).withJump(false).
				withYaw("NaN").withPitch("0").
				withMining(false).withEating(false).withSprinting(false).withSneaking(false),
		},
		{
			label: "player-input-non-finite-pitch",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "player-input"}.
				withSequence(2).withMoveX(0).withMoveZ(0).withJump(false).
				withYaw("0").withPitch("Inf").
				withMining(false).withEating(false).withSprinting(false).withSneaking(false),
		},
		{
			label: "player-input-negative-infinity-pitch",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "player-input"}.
				withSequence(2).withMoveX(0).withMoveZ(0).withJump(false).
				withYaw("0").withPitch("-Inf").
				withMining(false).withEating(false).withSprinting(false).withSneaking(false),
		},
		{
			label: "player-input-primary-only",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "player-input"}.
				withSequence(3).withMoveX(0).withMoveZ(0).withJump(false).
				withYaw("0").withPitch("0").
				withMining(true).withEating(false).withSprinting(false).withSneaking(false),
		},
		{
			label: "player-input-eating-only",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "player-input"}.
				withSequence(3).withMoveX(0).withMoveZ(0).withJump(false).
				withYaw("0").withPitch("0").
				withMining(false).withEating(true).withSprinting(false).withSneaking(false),
		},
		{
			label: "player-input-sprinting-only",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "player-input"}.
				withSequence(3).withMoveX(0).withMoveZ(0).withJump(false).
				withYaw("0").withPitch("0").
				withMining(false).withEating(false).withSprinting(true).withSneaking(false),
		},
		{
			label: "player-input-sneaking-only",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "player-input"}.
				withSequence(3).withMoveX(0).withMoveZ(0).withJump(false).
				withYaw("0").withPitch("0").
				withMining(false).withEating(false).withSprinting(false).withSneaking(true),
		},
		{
			label: "place-block-slot-eight",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "place-block"}.
				withSequence(4).withYaw("0").withPitch("0").withSlot(8),
		},
		{
			label: "place-block-slot-nine",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "place-block"}.
				withSequence(4).withYaw("0").withPitch("0").withSlot(9),
		},
		{
			label: "place-block-slot-max",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "place-block"}.
				withSequence(4).withYaw("0").withPitch("0").withSlot(math.MaxUint8),
		},
		{
			label: "place-block-non-finite-yaw",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "place-block"}.
				withSequence(4).withYaw("NaN").withPitch("0").withSlot(0),
		},
		{
			label: "select-hotbar-slot-eight",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "select-hotbar"}.
				withSequence(5).withSlot(8),
		},
		{
			label: "select-hotbar-slot-nine",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "select-hotbar"}.
				withSequence(5).withSlot(9),
		},
		{
			label: "chunk-resync-depths-zero-revision",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "chunk-resync"}.
				withSequence(6).withDimension(int32(core.Depths)).withChunk(-3, 7).withHaveRevision(0),
		},
		{
			label: "chunk-resync-overworld-held-revision",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "chunk-resync"}.
				withSequence(6).withDimension(int32(core.Overworld)).withChunk(0, 0).withHaveRevision(41),
		},
		{
			label: "chunk-resync-unknown-dimension",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "chunk-resync"}.
				withSequence(6).withDimension(2).withChunk(-3, 7).withHaveRevision(0),
		},
	}

	rays := []domainControlCase{
		{
			label: "till-soil-negative-zero",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "till-soil"}.
				withSequence(8).withYaw("-0").withPitch("-0"),
		},
		{
			label: "till-soil-non-finite-yaw",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "till-soil"}.
				withSequence(8).withYaw("NaN").withPitch("0"),
		},
		{
			label: "bone-meal-negative-zero",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "bone-meal"}.
				withSequence(9).withYaw("-0").withPitch("-0"),
		},
		{
			label: "bone-meal-non-finite-pitch",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "bone-meal"}.
				withSequence(9).withYaw("0").withPitch("-Inf"),
		},
		{
			label: "collect-water-negative-zero",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "collect-water"}.
				withSequence(10).withYaw("-0").withPitch("-0"),
		},
		{
			label: "collect-water-non-finite-yaw",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "collect-water"}.
				withSequence(10).withYaw("Inf").withPitch("0"),
		},
		{
			label: "place-water-negative-zero",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "place-water"}.
				withSequence(11).withYaw("-0").withPitch("-0"),
		},
		{
			label: "place-water-non-finite-pitch",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "place-water"}.
				withSequence(11).withYaw("0").withPitch("NaN"),
		},
		{
			label: "open-container-negative-zero",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "open-container"}.
				withSequence(12).withYaw("-0").withPitch("-0"),
		},
		{
			label: "open-container-non-finite-yaw",
			input: domainControlInput{Consumer: domainControlConsumer, Rule: "open-container"}.
				withSequence(12).withYaw("NaN").withPitch("0"),
		},
	}

	cases := make([]domainControlCase, 0, len(inputs)+len(rays))
	cases = append(cases, inputs...)
	cases = append(cases, rays...)
	return cases
}

// runDomainControl executes one corpus case through the current Go protocol
// command DTOs.
//
// The verdict always comes from the DTO's own `Validate`, which is the same
// validator the codec applies on both the encode and the decode side. The rule
// name and the rejection category come from the same bounds the DTO reads, and
// the two are cross-checked against each other, so a classification that
// disagrees with the authority fails the run instead of publishing a
// plausible-looking rejection.
func runDomainControl(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainControlDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainControlConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainControlConsumer)
	}

	switch spec.Rule {
	case "player-input":
		return domainControlRunPlayerInput(c, spec)
	case "place-block":
		return domainControlRunPlaceBlock(c, spec)
	case "select-hotbar":
		return domainControlRunSelectHotbar(c, spec)
	case "chunk-resync":
		return domainControlRunChunkResync(c, spec)
	case "till-soil":
		return domainControlRunTillSoil(c, spec)
	case "bone-meal":
		return domainControlRunBoneMeal(c, spec)
	case "collect-water":
		return domainControlRunCollectWater(c, spec)
	case "place-water":
		return domainControlRunPlaceWater(c, spec)
	case "open-container":
		return domainControlRunOpenContainer(c, spec)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}
}

// domainControlRunPlayerInput admits one movement payload. The axes are client
// claims the authority judges later, so the DTO keeps the full i8 range and the
// producer records whatever the case named.
func domainControlRunPlayerInput(c CaseSpec, spec domainControlInput) (Outcome, []byte, error) {
	sequence, err := domainControlSequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}
	moveX, err := domainControlAxis(c, "move_x", spec.MoveX)
	if err != nil {
		return Outcome{}, nil, err
	}
	moveZ, err := domainControlAxis(c, "move_z", spec.MoveZ)
	if err != nil {
		return Outcome{}, nil, err
	}
	jump, err := domainControlFlag(c, "jump", spec.Jump)
	if err != nil {
		return Outcome{}, nil, err
	}
	yaw, err := domainControlAngle(c, "yaw", spec.Yaw)
	if err != nil {
		return Outcome{}, nil, err
	}
	pitch, err := domainControlAngle(c, "pitch", spec.Pitch)
	if err != nil {
		return Outcome{}, nil, err
	}
	mining, err := domainControlFlag(c, "mining", spec.Mining)
	if err != nil {
		return Outcome{}, nil, err
	}
	eating, err := domainControlFlag(c, "eating", spec.Eating)
	if err != nil {
		return Outcome{}, nil, err
	}
	sprinting, err := domainControlFlag(c, "sprinting", spec.Sprinting)
	if err != nil {
		return Outcome{}, nil, err
	}
	sneaking, err := domainControlFlag(c, "sneaking", spec.Sneaking)
	if err != nil {
		return Outcome{}, nil, err
	}

	input := protocol.PlayerInput{
		Sequence:  sequence,
		MoveX:     moveX,
		MoveZ:     moveZ,
		Jump:      jump,
		Yaw:       yaw,
		Pitch:     pitch,
		Mining:    mining,
		Eating:    eating,
		Sprinting: sprinting,
		Sneaking:  sneaking,
	}
	category, rule := domainControlPlayerInputRule(input)
	return domainControlFinish(c, "player-input", input, domainControlNormalizePlayerInput(input), category, rule, input.Validate() == nil)
}

// domainControlRunPlaceBlock admits one placement intent, whose only payload
// bounds are the finite rotation and the hotbar slot range.
func domainControlRunPlaceBlock(c CaseSpec, spec domainControlInput) (Outcome, []byte, error) {
	sequence, err := domainControlSequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}
	yaw, err := domainControlAngle(c, "yaw", spec.Yaw)
	if err != nil {
		return Outcome{}, nil, err
	}
	pitch, err := domainControlAngle(c, "pitch", spec.Pitch)
	if err != nil {
		return Outcome{}, nil, err
	}
	slot, err := domainControlSlot(c, spec.Slot)
	if err != nil {
		return Outcome{}, nil, err
	}

	command := protocol.PlaceBlock{Sequence: sequence, Yaw: yaw, Pitch: pitch, Slot: slot}
	category, rule := domainControlPlaceBlockRule(command)
	return domainControlFinish(c, "place-block", command, domainControlNormalizePlaceBlock(command), category, rule, command.Validate() == nil)
}

// domainControlRunSelectHotbar admits one hotbar selection, whose only payload
// bound is the slot range.
func domainControlRunSelectHotbar(c CaseSpec, spec domainControlInput) (Outcome, []byte, error) {
	sequence, err := domainControlSequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}
	slot, err := domainControlSlot(c, spec.Slot)
	if err != nil {
		return Outcome{}, nil, err
	}

	command := protocol.SelectHotbar{Sequence: sequence, Slot: slot}
	category, rule := domainControlSelectHotbarRule(command)
	return domainControlFinish(c, "select-hotbar", command, domainControlNormalizeSelectHotbar(command), category, rule, command.Validate() == nil)
}

// domainControlRunChunkResync admits one resync request. The dimension is the
// only payload bound; the revision and the coordinates are client-held facts
// with no protocol range.
func domainControlRunChunkResync(c CaseSpec, spec domainControlInput) (Outcome, []byte, error) {
	sequence, err := domainControlSequence(c, spec.Sequence)
	if err != nil {
		return Outcome{}, nil, err
	}
	if spec.Dimension == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: chunk-resync requires a dimension", c.ID)
	}
	if spec.ChunkX == nil || spec.ChunkZ == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: chunk-resync requires chunk coordinates", c.ID)
	}
	if spec.HaveRevision == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: chunk-resync requires a held revision", c.ID)
	}

	request := protocol.RequestChunkResync{
		Sequence:     sequence,
		Dimension:    core.DimensionID(*spec.Dimension),
		Chunk:        core.ChunkPos{X: *spec.ChunkX, Z: *spec.ChunkZ},
		HaveRevision: *spec.HaveRevision,
	}
	category, rule := domainControlResyncRule(request)
	return domainControlFinish(c, "chunk-resync", request, domainControlNormalizeResync(request), category, rule, request.Validate() == nil)
}

// domainControlRunTillSoil admits one till-soil ray intent.
func domainControlRunTillSoil(c CaseSpec, spec domainControlInput) (Outcome, []byte, error) {
	sequence, yaw, pitch, err := domainControlRayFields(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	command := protocol.TillSoil{Sequence: sequence, Yaw: yaw, Pitch: pitch}
	category, rule := domainControlRayRule(yaw, pitch, "till-soil")
	return domainControlFinish(c, "till-soil", command, domainControlNormalizeRay(sequence, yaw, pitch), category, rule, command.Validate() == nil)
}

// domainControlRunBoneMeal admits one bone-meal ray intent.
func domainControlRunBoneMeal(c CaseSpec, spec domainControlInput) (Outcome, []byte, error) {
	sequence, yaw, pitch, err := domainControlRayFields(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	command := protocol.BoneMeal{Sequence: sequence, Yaw: yaw, Pitch: pitch}
	category, rule := domainControlRayRule(yaw, pitch, "bone-meal")
	return domainControlFinish(c, "bone-meal", command, domainControlNormalizeRay(sequence, yaw, pitch), category, rule, command.Validate() == nil)
}

// domainControlRunCollectWater admits one collect-water ray intent.
func domainControlRunCollectWater(c CaseSpec, spec domainControlInput) (Outcome, []byte, error) {
	sequence, yaw, pitch, err := domainControlRayFields(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	command := protocol.CollectWater{Sequence: sequence, Yaw: yaw, Pitch: pitch}
	category, rule := domainControlRayRule(yaw, pitch, "collect-water")
	return domainControlFinish(c, "collect-water", command, domainControlNormalizeRay(sequence, yaw, pitch), category, rule, command.Validate() == nil)
}

// domainControlRunPlaceWater admits one place-water ray intent.
func domainControlRunPlaceWater(c CaseSpec, spec domainControlInput) (Outcome, []byte, error) {
	sequence, yaw, pitch, err := domainControlRayFields(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	command := protocol.PlaceWater{Sequence: sequence, Yaw: yaw, Pitch: pitch}
	category, rule := domainControlRayRule(yaw, pitch, "place-water")
	return domainControlFinish(c, "place-water", command, domainControlNormalizeRay(sequence, yaw, pitch), category, rule, command.Validate() == nil)
}

// domainControlRunOpenContainer admits one open-container ray intent. The
// command is declared beside the container messages rather than with the other
// ray intents, but `protocol.OpenContainer` publishes the same finite-angle
// rule, so it executes through the shared ray resolver and rule classifier.
func domainControlRunOpenContainer(c CaseSpec, spec domainControlInput) (Outcome, []byte, error) {
	sequence, yaw, pitch, err := domainControlRayFields(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	command := protocol.OpenContainer{Sequence: sequence, Yaw: yaw, Pitch: pitch}
	category, rule := domainControlRayRule(yaw, pitch, "open-container")
	return domainControlFinish(c, "open-container", command, domainControlNormalizeRay(sequence, yaw, pitch), category, rule, command.Validate() == nil)
}

// domainControlRayFields resolves the payload every ray intent shares: the
// sequence and the two look angles. Every ray DTO declared beside the movement
// commands publishes exactly these fields and applies the same finite-angle
// rule, so one resolver and one rule serve all five intents.
func domainControlRayFields(c CaseSpec, spec domainControlInput) (uint64, float32, float32, error) {
	sequence, err := domainControlSequence(c, spec.Sequence)
	if err != nil {
		return 0, 0, 0, err
	}
	yaw, err := domainControlAngle(c, "yaw", spec.Yaw)
	if err != nil {
		return 0, 0, 0, err
	}
	pitch, err := domainControlAngle(c, "pitch", spec.Pitch)
	if err != nil {
		return 0, 0, 0, err
	}
	return sequence, yaw, pitch, nil
}

// domainControlFinish records one command admission.
//
// The classification is cross-checked against the authority's verdict, a
// rejection publishes the broken rule beside its category, and an admitted
// command is round-tripped through the production codec so the recorded outcome
// carries the exact wire payload the encoder produced.
func domainControlFinish(c CaseSpec, subject string, packet protocol.ClientPacket, fields map[string]any, category, rule string, admitted bool) (Outcome, []byte, error) {
	if admitted == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with the protocol validator (admitted=%v)", c.ID, rule, admitted)
	}
	if !admitted {
		fields["rule"] = rule
		return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
	}
	wire, err := domainControlRoundTrip(c, packet, fields)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields["wire"] = wire
	return Outcome{Kind: "ok", Category: subject, Fields: fields}, nil, nil
}

// domainControlRoundTrip encodes one admitted command through the production
// codec and decodes it back, proving the wire layout carries every field the
// DTO validated. The encode and decode paths both run the production
// validation, so a payload the DTO admits but the wire cannot represent fails
// here instead of being recorded as accepted.
func domainControlRoundTrip(c CaseSpec, packet protocol.ClientPacket, fields map[string]any) (string, error) {
	instance, err := codec.NewCodec()
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: case %s: create codec: %w", c.ID, err)
	}
	defer func() { _ = instance.Close() }()

	packetID, payload, err := instance.EncodeClient(protocol.StatePlay, packet)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: case %s: encode command: %w", c.ID, err)
	}
	decoded, err := instance.DecodeClient(protocol.StatePlay, packetID, payload)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: case %s: decode command: %w", c.ID, err)
	}
	roundTripped, err := domainControlNormalizePacket(decoded)
	if err != nil {
		return "", fmt.Errorf("runtime-oracle: case %s: normalize decoded command: %w", c.ID, err)
	}
	if !reflect.DeepEqual(roundTripped, fields) {
		return "", fmt.Errorf("runtime-oracle: case %s: codec round-trip changed the normalized command: got %#v, want %#v", c.ID, roundTripped, fields)
	}
	return hex.EncodeToString(payload), nil
}

// domainControlNormalizePacket renders one decoded command in the normalized
// field map, so a decoded payload is compared field by field against the
// command that was encoded.
func domainControlNormalizePacket(packet protocol.ClientPacket) (map[string]any, error) {
	switch command := packet.(type) {
	case protocol.PlayerInput:
		return domainControlNormalizePlayerInput(command), nil
	case protocol.PlaceBlock:
		return domainControlNormalizePlaceBlock(command), nil
	case protocol.SelectHotbar:
		return domainControlNormalizeSelectHotbar(command), nil
	case protocol.RequestChunkResync:
		return domainControlNormalizeResync(command), nil
	case protocol.TillSoil:
		return domainControlNormalizeRay(command.Sequence, command.Yaw, command.Pitch), nil
	case protocol.BoneMeal:
		return domainControlNormalizeRay(command.Sequence, command.Yaw, command.Pitch), nil
	case protocol.CollectWater:
		return domainControlNormalizeRay(command.Sequence, command.Yaw, command.Pitch), nil
	case protocol.PlaceWater:
		return domainControlNormalizeRay(command.Sequence, command.Yaw, command.Pitch), nil
	case protocol.OpenContainer:
		return domainControlNormalizeRay(command.Sequence, command.Yaw, command.Pitch), nil
	default:
		return nil, fmt.Errorf("decoded packet %T is not a command this family executes", packet)
	}
}

// domainControlNormalizePlayerInput renders the movement payload. The u64
// sequence is a decimal string and the two f32 angles are their IEEE-754 bit
// strings, so a signed zero stays observable instead of collapsing onto its
// positive counterpart.
func domainControlNormalizePlayerInput(input protocol.PlayerInput) map[string]any {
	return map[string]any{
		"sequence":  strconv.FormatUint(input.Sequence, 10),
		"move_x":    int(input.MoveX),
		"move_z":    int(input.MoveZ),
		"jump":      input.Jump,
		"yaw":       domainControlFloatBits(input.Yaw),
		"pitch":     domainControlFloatBits(input.Pitch),
		"mining":    input.Mining,
		"eating":    input.Eating,
		"sprinting": input.Sprinting,
		"sneaking":  input.Sneaking,
	}
}

// domainControlNormalizePlaceBlock renders the placement intent.
func domainControlNormalizePlaceBlock(command protocol.PlaceBlock) map[string]any {
	return map[string]any{
		"sequence": strconv.FormatUint(command.Sequence, 10),
		"yaw":      domainControlFloatBits(command.Yaw),
		"pitch":    domainControlFloatBits(command.Pitch),
		"slot":     int(command.Slot),
	}
}

// domainControlNormalizeSelectHotbar renders the hotbar selection.
func domainControlNormalizeSelectHotbar(command protocol.SelectHotbar) map[string]any {
	return map[string]any{
		"sequence": strconv.FormatUint(command.Sequence, 10),
		"slot":     int(command.Slot),
	}
}

// domainControlNormalizeResync renders the resync request.
func domainControlNormalizeResync(request protocol.RequestChunkResync) map[string]any {
	return map[string]any{
		"sequence":      strconv.FormatUint(request.Sequence, 10),
		"dimension":     int(request.Dimension),
		"chunk_x":       int(request.Chunk.X),
		"chunk_z":       int(request.Chunk.Z),
		"have_revision": strconv.FormatUint(request.HaveRevision, 10),
	}
}

// domainControlNormalizeRay renders one ray intent, whose entire payload is the
// sequence and the two angles.
func domainControlNormalizeRay(sequence uint64, yaw, pitch float32) map[string]any {
	return map[string]any{
		"sequence": strconv.FormatUint(sequence, 10),
		"yaw":      domainControlFloatBits(yaw),
		"pitch":    domainControlFloatBits(pitch),
	}
}

// domainControlFloatBits renders one float32 as the eight-digit hexadecimal
// IEEE-754 bit string the normalized corpus vocabulary uses.
func domainControlFloatBits(value float32) string {
	return fmt.Sprintf("%08x", math.Float32bits(value))
}

// domainControlPlayerInputRule names the first rule a movement payload breaks.
// The axes and the held flags have no protocol range, so the rotation is the
// only rule this DTO publishes.
func domainControlPlayerInputRule(input protocol.PlayerInput) (string, string) {
	if !domainControlFinite(input.Yaw) || !domainControlFinite(input.Pitch) {
		return "invalid-value", "player_input.finite_rotation"
	}
	return "", ""
}

// domainControlPlaceBlockRule names the first rule a placement intent breaks,
// in the same order the DTO checks them: the rotation, then the slot range.
func domainControlPlaceBlockRule(command protocol.PlaceBlock) (string, string) {
	if !domainControlFinite(command.Yaw) || !domainControlFinite(command.Pitch) {
		return "invalid-value", "place_block.finite_rotation"
	}
	if command.Slot >= core.HotbarSlots {
		return "invalid-value", "place_block.slot_range"
	}
	return "", ""
}

// domainControlSelectHotbarRule names the first rule a hotbar selection breaks.
func domainControlSelectHotbarRule(command protocol.SelectHotbar) (string, string) {
	if command.Slot >= core.HotbarSlots {
		return "invalid-value", "select_hotbar.slot_range"
	}
	return "", ""
}

// domainControlResyncRule names the first rule a resync request breaks. The
// dimension is an enumerated value, so an unknown one is an enum rejection
// rather than a range rejection.
func domainControlResyncRule(request protocol.RequestChunkResync) (string, string) {
	if request.Dimension != core.Overworld && request.Dimension != core.Depths {
		return "invalid-enum", "chunk_resync.dimension"
	}
	return "", ""
}

// domainControlRayRule names the first rule one ray intent breaks: the shared
// finite-angle rule every angle-carrying command DTO publishes.
func domainControlRayRule(yaw, pitch float32, subject string) (string, string) {
	if !domainControlFinite(yaw) || !domainControlFinite(pitch) {
		return "invalid-value", subject + ".finite_rotation"
	}
	return "", ""
}

// domainControlFinite reports whether one look angle is finite, which is the
// one rotation rule shared by every command DTO in this family.
func domainControlFinite(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

// domainControlSequence resolves the sequence field one case names.
func domainControlSequence(c CaseSpec, value *uint64) (uint64, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires a sequence", c.ID)
	}
	return *value, nil
}

// domainControlAxis resolves one movement axis. The full i8 range is legal, so
// the resolver only reports a missing field.
func domainControlAxis(c CaseSpec, name string, value *int8) (int8, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	return *value, nil
}

// domainControlAngle resolves one look angle from its decimal float text. NaN,
// the infinities and a signed zero are all expressible, because those exact
// values are the boundaries the finite rule pins.
func domainControlAngle(c CaseSpec, name string, value *string) (float32, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	parsed, err := strconv.ParseFloat(*value, 32)
	if err != nil {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %q, which is not a float32: %w", c.ID, name, *value, err)
	}
	return float32(parsed), nil
}

// domainControlFlag resolves one held action flag. A false flag is a value, not
// an absence, so the pointer is required rather than defaulted.
func domainControlFlag(c CaseSpec, name string, value *bool) (bool, error) {
	if value == nil {
		return false, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	return *value, nil
}

// domainControlSlot resolves the hotbar slot field one case names. The DTO's
// own range check decides whether the value is legal.
func domainControlSlot(c CaseSpec, value *uint8) (uint8, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires a slot", c.ID)
	}
	return *value, nil
}

// domainControlDecodeInput reads one frozen corpus input and rejects trailing
// content, so a producer never executes bytes the case did not name.
func domainControlDecodeInput(data []byte) (domainControlInput, error) {
	var spec domainControlInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainControlInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainControlInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainControlRecord pairs one executed case with the input and outcome the
// producer produced for it.
type domainControlRecord struct {
	label   string
	input   domainControlInput
	outcome Outcome
}

// domainControlExecute runs the whole case table through the producer and
// returns one record per case in table order.
func domainControlExecute(t *testing.T) []domainControlRecord {
	t.Helper()

	cases := domainControlCases()
	records := make([]domainControlRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainControl(CaseSpec{ID: domainControlCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainControlRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// domainControlSyncCorpus writes or verifies the frozen corpus files.
//
// An ordinary run is read-only: it proves the committed files still match what
// the protocol DTOs and the codec produce now, so a drifted artifact fails
// instead of being regenerated. The explicit update flag rewrites them, which
// is the only way a frozen artifact changes.
func domainControlSyncCorpus(t *testing.T, records []domainControlRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainControlCorpusRelDir))
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

	if *updateDomainControlCorpus {
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
			t.Fatalf("read frozen corpus case %s: %v (rerun with -update-domain-command-control-corpus after reviewing the change)", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed protocol DTO (rerun with -update-domain-command-control-corpus after reviewing the change)", relative)
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

// domainControlCaseID renders the manifest case identity one corpus label
// carries.
func domainControlCaseID(label string) string {
	return domainControlFamily + "/" + domainControlVersion + "/" + label
}

// domainControlWorkingManifest assembles the manifest this node executes inside
// a harness-owned temporary directory.
//
// The frozen manifest does not carry this family, because the controller merges
// manifest fragments after acceptance. The working manifest is therefore a
// family-scoped selection: `Cases` holds exactly the `domainControlFamily`
// cases the committed corpus registers, every other family's case list is
// cleared because `Reconcile` requires each family's list to match the cases
// this selection registers for it, and the family itself is appended with the
// provenance the producer reads. Registering the family in the live registry is
// deliberately left to the manifest merge: adding it here would make the frozen
// corpus fail reconciliation as an uncovered family before the fragment lands.
func domainControlWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainControlCorpusCases(t, root)

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
	sources := make([]SourceSpec, 0, len(domainControlFamilySources))
	for _, relative := range domainControlFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	cloned.Families = append(cloned.Families, Family{
		ID:                domainControlFamily,
		Kind:              "domain",
		Role:              "input",
		CurrentVersion:    domainControlVersion,
		SupportedVersions: []string{domainControlVersion},
		Source:            domainControlSource,
		EventualOwner:     ownerDomain,
		NumericSemantics:  domainControlNumericSemantics,
		Sources:           sources,
		Cases:             caseIDs,
	})
	for index := range cloned.Families {
		// A family this selection does not execute registers no case, so its
		// list must be empty for the manifest to describe itself.
		if cloned.Families[index].ID != domainControlFamily {
			cloned.Families[index].Cases = nil
		}
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

// domainControlCorpusCases reads the frozen corpus and registers one case per
// committed input, with digests proven against the files on disk.
func domainControlCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainControlCorpusRelDir))
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
		t.Fatalf("walk command control corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no command control case under %s", domainControlCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainControlLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainControlReadEnvelope(t, input)
		if envelope.Consumer != domainControlConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainControlConsumer)
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
			ID:           domainControlCaseID(label),
			Family:       domainControlFamily,
			Version:      domainControlVersion,
			Operation:    domainControlOperation,
			Input:        AssetRef{Path: domainControlCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainControlCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainControlConsumer,
		})
	}
	return cases
}

// domainControlReadEnvelope reads the provenance envelope of one frozen corpus
// input.
func domainControlReadEnvelope(t *testing.T, path string) domainControlInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainControlInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainControlCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainControlCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainCommandControlOracleExecutesEveryCase runs the whole case table
// through the real Go protocol DTOs and the production codec, proves the frozen
// corpus still matches what they produce, and then runs the same cases through
// the production runner so the executed evidence satisfies the completeness
// rules a published trace report does.
func TestDomainCommandControlOracleExecutesEveryCase(t *testing.T) {
	records := domainControlExecute(t)
	domainControlSyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainControlWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainControlAssertCaseSpecs(t, root, manifest)
	domainControlAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainControlCaseByID(t, manifest, obs.CaseID)
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
		t.Fatalf("executed command control evidence failed trace validation: %v", err)
	}
}

// TestDomainCommandControlOracleCaseIdentitiesAreTheRustDomainConsumer pins the
// manifest identity of every command control case: the operation, the version,
// the family and the consumer the change names for the Rust crate that owns
// these rules.
func TestDomainCommandControlOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainControlWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainControlOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainControlOperation)
		}
		if c.Version != domainControlVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainControlVersion)
		}
		if c.RustConsumer != domainControlConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainControlConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainControlLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainControlReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainControlConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainControlConsumer)
		}
	}
}

// TestDomainCommandControlOracleWorkingManifestDescribesItself proves the
// working manifest is internally consistent without claiming a registry entry
// that the frozen corpus does not carry yet: every case passes the production
// case validation, the family's provenance hashes match disk, and the family's
// case list matches exactly the cases the selection registers.
func TestDomainCommandControlOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainControlWorkingManifest(t, root)
	domainControlAssertCaseSpecs(t, root, manifest)

	family, ok := domainControlFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainControlFamily)
	}
	if len(family.Sources) != len(domainControlFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainControlFamily, len(family.Sources), len(domainControlFamilySources))
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
		t.Fatalf("%s lists %d cases, want %d", domainControlFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainControlFamily, id)
		}
	}
}

// TestDomainCommandControlOracleOutcomesDistinguishAcceptedAndRejected pins that
// the producer is not returning one constant answer, that every rule records
// both an admitted command and a rejection, and that the boundaries the Rust
// command test enumerates are each present in the executed evidence.
func TestDomainCommandControlOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainControlExecute(t)
	domainControlAssertOutcomesDistinguishCases(t, records)

	tally := make(map[string]*domainControlRuleTally)
	for _, record := range records {
		entry, ok := tally[record.input.Rule]
		if !ok {
			entry = &domainControlRuleTally{}
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
		if len(entry.accepted) == 0 || len(entry.rejected) == 0 {
			t.Fatalf("rule %s records %d accepted and %d rejected cases, want both non-zero",
				rule, len(entry.accepted), len(entry.rejected))
		}
	}

	if !domainControlFieldsContain(tally["player-input"].accepted, "move_x", math.MinInt8) {
		t.Fatal("no admitted movement payload keeps the minimum i8 axis, which the protocol must not clamp")
	}
	if !domainControlFieldsContain(tally["player-input"].accepted, "move_z", math.MaxInt8) {
		t.Fatal("no admitted movement payload keeps the maximum i8 axis, which the protocol must not clamp")
	}
	for _, flag := range []string{"mining", "eating", "sprinting", "sneaking"} {
		if !domainControlHeldFlagSurvivesAlone(tally["player-input"].accepted, flag) {
			t.Fatalf("no admitted movement payload sets only %s, so the held flags are not independent", flag)
		}
	}
	if !domainControlFieldsContain(tally["place-block"].accepted, "slot", 8) {
		t.Fatal("no admitted placement intent uses hotbar slot eight, which is inside the range")
	}
	if !domainControlFieldsContain(tally["place-block"].rejected, "slot", 9) {
		t.Fatal("no rejected placement intent names hotbar slot nine, which is outside the range")
	}
	if !domainControlFieldsContain(tally["chunk-resync"].accepted, "dimension", 1) {
		t.Fatal("no admitted resync request names the depths dimension")
	}
	if !domainControlFieldsContain(tally["chunk-resync"].accepted, "have_revision", "0") {
		t.Fatal("no admitted resync request keeps a zero held revision")
	}
	if !domainControlRejectionNamesRule(tally, "chunk_resync.dimension") {
		t.Fatal("no rejection names the resync dimension rule, so an unknown dimension is not pinned")
	}
	if !domainControlRejectionNamesRule(tally, "player_input.finite_rotation") {
		t.Fatal("no rejection names the finite rotation rule, so a non-finite angle is not pinned")
	}

	negativeZero, positiveZero, wired := false, false, true
	for _, entry := range tally {
		for _, fields := range entry.accepted {
			if fields["yaw"] == "80000000" && fields["pitch"] == "80000000" {
				negativeZero = true
			}
			if fields["yaw"] == "00000000" {
				positiveZero = true
			}
			if _, ok := fields["wire"].(string); !ok {
				wired = false
			}
		}
		for _, fields := range entry.rejected {
			if _, ok := fields["wire"]; ok {
				t.Fatalf("rule %s publishes a rejection carrying codec round-trip bytes", entry.rule)
			}
		}
	}
	if !negativeZero {
		t.Fatal("no admitted command keeps the negative zero angle bits, so signed zero is not pinned")
	}
	if !positiveZero {
		t.Fatal("no admitted command keeps the positive zero angle bits, so the signed zero contrast is missing")
	}
	if !wired {
		t.Fatal("some admitted command published no codec round-trip payload")
	}
}

// TestDomainCommandControlOracleRunnerRejectsUnregisteredFamily pins that a
// manifest naming a family this package cannot execute fails the run.
func TestDomainCommandControlOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainControlWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainCommandControlOracleRunnerRejectsOperationFamilyMismatch pins that a
// case cannot declare one operation and be executed by another.
func TestDomainCommandControlOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainControlWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation/family mismatch failure, got: %v", err)
	}
}

// TestDomainCommandControlOracleRunnerRejectsTamperedInput pins that a producer
// never executes bytes the manifest does not name.
func TestDomainCommandControlOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainControlWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainCommandControlOracleReportPublishesAndValidates assembles the
// executed evidence into a report, publishes it through the production atomic
// exporter, reloads it and validates it against the working manifest. This is
// the identity and content check a later Rust acceptance step performs; it does
// not claim any Rust behaviour.
func TestDomainCommandControlOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainControlWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTrace(trace, manifest); err != nil {
		t.Fatalf("executed command control evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainControlCorpusReportName)
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

// TestDomainCommandControlOracleRejectsMissingProducerTest pins that the
// executed evidence has a real producer behind it, including the topic-named
// entry point the domain plan's filter selects. A corpus whose producer test is
// gone has no independent execution, only frozen files, and a missing topic
// entry point would make the plan filter pass without running anything.
func TestDomainCommandControlOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainControlProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainControlProducerTestRelPath, err)
	}
	for _, name := range []string{domainControlProducerExecuteName, domainControlProducerTopicName} {
		declaration := "func " + name + "(t *testing.T) {"
		if !strings.Contains(string(data), declaration) {
			t.Fatalf("%s does not declare %s", domainControlProducerTestRelPath, declaration)
		}
	}
}

// TestDomainOracle_command_control is the topic-named entry point the domain
// plan names for this node. It delegates to the same executed table, so the two
// filters select one source of expected results rather than two.
func TestDomainOracle_command_control(t *testing.T) {
	TestDomainCommandControlOracleExecutesEveryCase(t)
}

// domainControlRuleTally collects the accepted and rejected normalized outcomes
// one rule records, so a boundary assertion can look for the field value it
// names.
type domainControlRuleTally struct {
	rule     string
	accepted []map[string]any
	rejected []map[string]any
}

// domainControlFieldsContain reports whether one recorded outcome set holds a
// record whose named field carries the value a boundary assertion names.
func domainControlFieldsContain(records []map[string]any, field string, want any) bool {
	for _, fields := range records {
		if reflect.DeepEqual(fields[field], want) {
			return true
		}
	}
	return false
}

// domainControlHeldFlagSurvivesAlone reports whether one recorded outcome set
// holds a record where exactly the named held flag is set, which is what makes
// the four flags independent of each other.
func domainControlHeldFlagSurvivesAlone(records []map[string]any, flag string) bool {
	for _, fields := range records {
		alone := true
		for _, name := range []string{"mining", "eating", "sprinting", "sneaking"} {
			set, _ := fields[name].(bool)
			if set != (name == flag) {
				alone = false
				break
			}
		}
		if alone {
			return true
		}
	}
	return false
}

// domainControlRejectionNamesRule reports whether any recorded rejection names
// the broken rule a boundary assertion pins.
func domainControlRejectionNamesRule(tally map[string]*domainControlRuleTally, rule string) bool {
	for _, entry := range tally {
		for _, fields := range entry.rejected {
			if fields["rule"] == rule {
				return true
			}
		}
	}
	return false
}

// domainControlAssertCaseSpecs runs the production case validation over the
// whole selection, so a case with a bad path, digest, format or checkpoint
// fails here rather than surfacing later as a confusing coverage failure.
func domainControlAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
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

// domainControlAssertOutcomesDistinguishCases proves the executed evidence
// separates admitted commands from rejections instead of publishing one
// constant answer, and that the rejections name a rule the authority publishes.
func domainControlAssertOutcomesDistinguishCases(t *testing.T, records []domainControlRecord) {
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

// domainControlFamilySpec resolves one family from a manifest selection.
func domainControlFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainControlFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainControlCaseByID indexes a manifest selection by case identity.
func domainControlCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
