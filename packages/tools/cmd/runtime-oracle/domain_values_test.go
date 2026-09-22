package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file is the Go producer for the domain value family. It executes every
// committed corpus case through the current Go core item and identity
// validators, so a recorded outcome is what the checked-in authority decides
// about the value rather than a copy of a Rust result or a hand-written
// expectation. No world authority, listener, model call or native ABI is
// involved: the producer only reads immutable corpus inputs and calls
// in-process functions, and the frozen corpus files it materializes are
// read-only inputs for later consumers.

const (
	// domainValuesFamily is the corpus family this package executes. Its
	// eventual owner is the Rust domain crate, which is why the consumer is
	// named rather than left generic.
	domainValuesFamily = "domain.values"
	// domainValuesVersion labels the family as the current value rules rather
	// than a numbered schema: the item tables have no version history, so a
	// version number would imply a migration path that does not exist.
	domainValuesVersion = "current"
	// domainValuesOperation is the manifest operation name for a value
	// admission case.
	domainValuesOperation = "admit"
	// domainValuesConsumer is the manifest consumer the change pins for this
	// family: the Rust crate that owns these rules.
	domainValuesConsumer = "mornlea_domain"
	// domainValuesCorpusRelDir is the repository-relative directory holding the
	// frozen domain value corpus cases.
	domainValuesCorpusRelDir = "testdata/runtime-migration/cases/domain-values"
	// domainValuesProducerTestRelPath and domainValuesProducerTestName locate
	// the package-local producer that executes the core validators. A corpus
	// whose producer test is missing has no independently executed evidence at
	// all.
	domainValuesProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_values_test.go"
	domainValuesProducerTestName    = "TestDomainValuesOracleExecutesEveryCase"
	// domainValuesCorpusReportName is the published report file name for the
	// executed domain value evidence.
	domainValuesCorpusReportName = "runtime-corpus-domain-values.json"
	// domainValuesSource is the primary provenance source of the item tables.
	domainValuesSource = "packages/shared/core/item.go"
	// domainValuesNumericSemantics records the numeric contract this family
	// pins, in the same shape the other families use.
	domainValuesNumericSemantics = "registered item numbering; fixed per-chunk slot counts; nonzero generation; reject unregistered items and out-of-range values"
)

// domainValuesFamilySources is the merged provenance set the family records.
// Every entry is a file the producer's rules are read from, so a change to any
// of them is a change to the recorded evidence.
var domainValuesFamilySources = []string{
	"packages/shared/core/item.go",
	"packages/shared/core/smelting.go",
	"packages/shared/core/drop.go",
	"packages/shared/core/container.go",
	"packages/shared/core/furnace.go",
	"packages/shared/core/chest.go",
	"packages/shared/network/protocol/message_container.go",
}

// domainValuesLabelPattern is the shape a corpus label must have: a lowercase
// slug with an optional zero-padded numeric suffix, so a per-item row sorts in
// numeric order under a lexical sort.
var domainValuesLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// domainValuesInput is the frozen, self-describing corpus input for one case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. Optional fields are pointers so an
// absent field and an explicit zero stay distinguishable, which matters because
// item zero, count zero and generation zero are all meaningful values.
type domainValuesInput struct {
	Consumer   string `json:"consumer"`
	Rule       string `json:"rule"`
	Item       *int   `json:"item,omitempty"`
	Count      *int   `json:"count,omitempty"`
	Durability *int   `json:"durability,omitempty"`
	Dimension  *int32 `json:"dimension,omitempty"`
	ChunkX     *int32 `json:"chunk_x,omitempty"`
	ChunkZ     *int32 `json:"chunk_z,omitempty"`
	Slot       *int   `json:"slot,omitempty"`
	Generation *int64 `json:"generation,omitempty"`
	Kind       string `json:"kind,omitempty"`
}

func (in domainValuesInput) withItem(item int) domainValuesInput {
	in.Item = &item
	return in
}

func (in domainValuesInput) withCount(count int) domainValuesInput {
	in.Count = &count
	return in
}

func (in domainValuesInput) withDurability(durability int) domainValuesInput {
	in.Durability = &durability
	return in
}

func (in domainValuesInput) withDimension(dimension int32) domainValuesInput {
	in.Dimension = &dimension
	return in
}

func (in domainValuesInput) withChunk(x, z int32) domainValuesInput {
	in.ChunkX = &x
	in.ChunkZ = &z
	return in
}

func (in domainValuesInput) withSlot(slot int) domainValuesInput {
	in.Slot = &slot
	return in
}

func (in domainValuesInput) withGeneration(generation int64) domainValuesInput {
	in.Generation = &generation
	return in
}

// domainValuesCase is one frozen corpus case: its label and its input, and
// nothing else.
type domainValuesCase struct {
	label string
	input domainValuesInput
}

// domainValuesCases is the ordered case table the producer executes.
//
// The item table contributes one row per item number in the Go registered
// range, which is every number up to and including the `core.ItemIDMax`
// sentinel, so the unregistered numbers are evidence too rather than an
// untested assumption. The remaining rows are the boundary values each rule
// names: count and durability limits, the empty stack, the drop slot and
// generation bounds, and the two different container array sizes.
func domainValuesCases() []domainValuesCase {
	itemTable := make([]domainValuesCase, 0, int(core.ItemIDMax)+1)
	for item := 0; item <= int(core.ItemIDMax); item++ {
		itemTable = append(itemTable, domainValuesCase{
			label: fmt.Sprintf("item-table-%03d", item),
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-table"}.withItem(item),
		})
	}

	stacks := []domainValuesCase{
		{
			label: "item-stack-empty",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemNone)),
		},
		{
			label: "item-stack-empty-count",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemNone)).withCount(1),
		},
		{
			label: "item-stack-empty-durability",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemNone)).withDurability(1),
		},
		{
			label: "item-stack-unregistered-sentinel",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemIDMax)).withCount(1),
		},
		{
			label: "item-stack-unregistered-above-sentinel",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemIDMax) + 1).withCount(1),
		},
		{
			label: "item-stack-count-zero",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemStone)),
		},
		{
			label: "item-stack-count-limit",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemStone)).withCount(int(core.MaxStackCount)),
		},
		{
			label: "item-stack-count-above-limit",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemStone)).withCount(int(core.MaxStackCount) + 1),
		},
		{
			label: "item-stack-single-count-limit",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemStonePickaxe)).withCount(1).
				withDurability(int(domainValuesDurabilityMax(core.ItemStonePickaxe))),
		},
		{
			label: "item-stack-single-count-above-limit",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemStonePickaxe)).withCount(2).
				withDurability(int(domainValuesDurabilityMax(core.ItemStonePickaxe))),
		},
		{
			label: "item-stack-durability-zero-tool",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemStonePickaxe)).withCount(1),
		},
		{
			label: "item-stack-durability-one-tool",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemStonePickaxe)).withCount(1).withDurability(1),
		},
		{
			label: "item-stack-durability-max-tool",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemStonePickaxe)).withCount(1).
				withDurability(int(domainValuesDurabilityMax(core.ItemStonePickaxe))),
		},
		{
			label: "item-stack-durability-above-max-tool",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemStonePickaxe)).withCount(1).
				withDurability(int(domainValuesDurabilityMax(core.ItemStonePickaxe)) + 1),
		},
		{
			label: "item-stack-durability-zero-armor",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemIronHelmet)).withCount(1),
		},
		{
			label: "item-stack-durability-max-armor",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemIronBoots)).withCount(1).
				withDurability(int(domainValuesDurabilityMax(core.ItemIronBoots))),
		},
		{
			label: "item-stack-durability-on-nondurable",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemStone)).withCount(1).withDurability(1),
		},
		{
			label: "item-stack-broken-tool-form",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "item-stack"}.
				withItem(int(core.ItemBrokenStonePickaxe)).withCount(1),
		},
	}

	drops := []domainValuesCase{
		{
			label: "drop-id-negative-dimension",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "drop-id"}.
				withDimension(-1).withChunk(0, 0).withSlot(31).withGeneration(1),
		},
		{
			label: "drop-id-minimum-dimension",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "drop-id"}.
				withDimension(math.MinInt32).withChunk(0, 0).withSlot(0).withGeneration(1),
		},
		{
			label: "drop-id-maximum-dimension",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "drop-id"}.
				withDimension(math.MaxInt32).withChunk(0, 0).withSlot(0).withGeneration(1),
		},
		{
			label: "drop-id-last-slot",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "drop-id"}.
				withDimension(0).withChunk(3, -4).withSlot(int(core.DropsPerChunk) - 1).withGeneration(1),
		},
		{
			label: "drop-id-slot-above-array",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "drop-id"}.
				withDimension(0).withChunk(3, -4).withSlot(int(core.DropsPerChunk)).withGeneration(1),
		},
		{
			label: "drop-id-zero-generation",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "drop-id"}.
				withDimension(0).withChunk(3, -4).withSlot(0),
		},
		{
			label: "drop-id-maximum-generation",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "drop-id"}.
				withDimension(0).withChunk(3, -4).withSlot(0).withGeneration(math.MaxUint32),
		},
	}

	containers := []domainValuesCase{
		{
			label: "container-ref-furnace-last-slot",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "container-ref", Kind: "furnace"}.
				withChunk(-2, 5).withSlot(int(core.FurnacesPerChunk) - 1).withGeneration(1),
		},
		{
			label: "container-ref-furnace-slot-above-array",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "container-ref", Kind: "furnace"}.
				withChunk(-2, 5).withSlot(int(core.FurnacesPerChunk)).withGeneration(1),
		},
		{
			label: "container-ref-chest-last-slot",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "container-ref", Kind: "chest"}.
				withChunk(-2, 5).withSlot(int(core.ChestsPerChunk) - 1).withGeneration(1),
		},
		{
			label: "container-ref-chest-slot-above-array",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "container-ref", Kind: "chest"}.
				withChunk(-2, 5).withSlot(int(core.ChestsPerChunk)).withGeneration(1),
		},
		{
			label: "container-ref-chest-furnace-slot",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "container-ref", Kind: "chest"}.
				withChunk(-2, 5).withSlot(int(core.FurnacesPerChunk) - 1).withGeneration(1),
		},
		{
			label: "container-ref-zero-generation",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "container-ref", Kind: "furnace"}.
				withChunk(0, 0).withSlot(0),
		},
		{
			label: "container-ref-maximum-generation",
			input: domainValuesInput{Consumer: domainValuesConsumer, Rule: "container-ref", Kind: "chest"}.
				withChunk(0, 0).withSlot(0).withGeneration(math.MaxUint32),
		},
	}

	cases := make([]domainValuesCase, 0, len(itemTable)+len(stacks)+len(drops)+len(containers))
	cases = append(cases, itemTable...)
	cases = append(cases, stacks...)
	cases = append(cases, drops...)
	cases = append(cases, containers...)
	return cases
}

// runDomainValues executes one corpus case through the current Go core item and
// identity validators.
//
// The verdict always comes from the authority itself: `core.ItemStack.Valid`,
// `core.DropID.Valid`, or the protocol container validator. The rule name and
// the rejection category come from the same tables the authority reads, and the
// two are cross-checked against each other, so a classification that disagrees
// with the authority fails the run instead of publishing a plausible-looking
// rejection.
func runDomainValues(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainValuesDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainValuesConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainValuesConsumer)
	}

	switch spec.Rule {
	case "item-table":
		return domainValuesRunItemTable(c, spec)
	case "item-stack":
		return domainValuesRunItemStack(c, spec)
	case "drop-id":
		return domainValuesRunDropID(c, spec)
	case "container-ref":
		return domainValuesRunContainerRef(c, spec)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}
}

// domainValuesRunItemTable resolves one item number through the three Go item
// tables and records every answer, so a registered number, a durable number and
// a smelting input are each pinned by the same row.
func domainValuesRunItemTable(c CaseSpec, spec domainValuesInput) (Outcome, []byte, error) {
	item, err := domainValuesItemNumber(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}

	limit, registered := core.ItemStackLimit(item)
	durability, durable := core.ItemMaxDurability(item)
	output, smeltable := core.SmeltingOutput(item)

	fields := map[string]any{
		"item":             int(item),
		"registered":       registered,
		"smelting_product": domainValuesIsSmeltingProduct(int(item)),
	}
	if registered {
		fields["stack_limit"] = int(limit)
	}
	if durable {
		fields["durability_max"] = int(durability)
	}
	if smeltable {
		fields["smelting_output"] = int(output)
	}
	return Outcome{Kind: "ok", Category: "item-table", Fields: fields}, nil, nil
}

// domainValuesRunItemStack admits one ordinary slot value through
// `core.ItemStack.Valid`.
func domainValuesRunItemStack(c CaseSpec, spec domainValuesInput) (Outcome, []byte, error) {
	item, err := domainValuesItemNumber(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	count, err := domainValuesByteField(c, "count", spec.Count)
	if err != nil {
		return Outcome{}, nil, err
	}
	durability, err := domainValuesUint16Field(c, "durability", spec.Durability)
	if err != nil {
		return Outcome{}, nil, err
	}

	stack := core.ItemStack{Item: item, Count: count, Durability: durability}
	fields := map[string]any{
		"item":       int(item),
		"count":      int(count),
		"durability": int(durability),
	}
	category, rule := domainValuesItemStackRule(stack)
	if stack.Valid() == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with core.ItemStack.Valid=%v", c.ID, rule, stack.Valid())
	}
	if stack.Valid() {
		return Outcome{Kind: "ok", Category: "item-stack", Fields: fields}, nil, nil
	}
	fields["rule"] = rule
	return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
}

// domainValuesRunDropID admits one drop identity through `core.DropID.Valid`.
func domainValuesRunDropID(c CaseSpec, spec domainValuesInput) (Outcome, []byte, error) {
	if spec.Dimension == nil || spec.ChunkX == nil || spec.ChunkZ == nil || spec.Slot == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: drop-id requires dimension, chunk and slot", c.ID)
	}
	slot, err := domainValuesByteField(c, "slot", spec.Slot)
	if err != nil {
		return Outcome{}, nil, err
	}
	generation, err := domainValuesUint32Field(c, "generation", spec.Generation)
	if err != nil {
		return Outcome{}, nil, err
	}

	drop := core.DropID{
		Dimension:  core.DimensionID(*spec.Dimension),
		Chunk:      core.ChunkPos{X: *spec.ChunkX, Z: *spec.ChunkZ},
		Slot:       slot,
		Generation: generation,
	}
	fields := map[string]any{
		"dimension":  int(drop.Dimension),
		"chunk_x":    int(drop.Chunk.X),
		"chunk_z":    int(drop.Chunk.Z),
		"slot":       int(drop.Slot),
		"generation": int64(drop.Generation),
	}

	category, rule := domainValuesDropIDRule(drop)
	if drop.Valid() == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with core.DropID.Valid=%v", c.ID, rule, drop.Valid())
	}
	if drop.Valid() {
		return Outcome{Kind: "ok", Category: "drop-id", Fields: fields}, nil, nil
	}
	fields["rule"] = rule
	return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
}

// domainValuesRunContainerRef admits one container reference through the
// protocol container validator.
//
// The reference is inherently an overworld value, so the case pins the
// overworld dimension explicitly and the validator's own dimension check is
// what makes a foreign dimension a rejection rather than something this
// producer decides on its own.
func domainValuesRunContainerRef(c CaseSpec, spec domainValuesInput) (Outcome, []byte, error) {
	if spec.ChunkX == nil || spec.ChunkZ == nil || spec.Slot == nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: container-ref requires chunk and slot", c.ID)
	}
	kind, err := domainValuesContainerKind(c, spec.Kind)
	if err != nil {
		return Outcome{}, nil, err
	}
	slot, err := domainValuesByteField(c, "slot", spec.Slot)
	if err != nil {
		return Outcome{}, nil, err
	}
	generation, err := domainValuesUint32Field(c, "generation", spec.Generation)
	if err != nil {
		return Outcome{}, nil, err
	}

	ref := core.ContainerRef{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: *spec.ChunkX, Z: *spec.ChunkZ},
		Kind:       kind,
		Slot:       slot,
		Generation: generation,
	}
	fields := map[string]any{
		"kind":       domainValuesKindName(kind),
		"chunk_x":    int(ref.Chunk.X),
		"chunk_z":    int(ref.Chunk.Z),
		"slot":       int(ref.Slot),
		"dimension":  int(ref.Dimension),
		"generation": int64(ref.Generation),
	}

	category, rule := domainValuesContainerRefRule(ref)
	accepted := protocol.ContainerClosed{Container: ref}.Validate() == nil
	if accepted == (rule != "") {
		return Outcome{}, nil, fmt.Errorf(
			"runtime-oracle: case %s: rule classification %q disagrees with the container validator (accepted=%v)", c.ID, rule, accepted)
	}
	if accepted {
		return Outcome{Kind: "ok", Category: "container-ref", Fields: fields}, nil, nil
	}
	fields["rule"] = rule
	return Outcome{Kind: "error", Category: category, Fields: fields}, nil, nil
}

// domainValuesItemStackRule names the first rule an ordinary slot value breaks,
// read from the same tables `core.ItemStack.Valid` reads. An empty result means
// the value is admitted.
func domainValuesItemStackRule(stack core.ItemStack) (string, string) {
	limit, registered := core.ItemStackLimit(stack.Item)
	if !registered {
		if stack.Item == core.ItemNone {
			if stack.Count != 0 {
				return "invalid-value", "item_stack.absent_item_count"
			}
			if stack.Durability != 0 {
				return "invalid-value", "item_stack.absent_item_durability"
			}
			// The zero triple is the canonical empty stack, which the Go rule
			// admits, so the classifier reports no broken rule for it.
			return "", ""
		}
		return "invalid-enum", "item_stack.registered_item"
	}
	if stack.Count == 0 || stack.Count > limit {
		return "invalid-value", "item_stack.count_range"
	}
	max, durable := core.ItemMaxDurability(stack.Item)
	if durable {
		if stack.Durability < 1 || stack.Durability > max {
			return "invalid-value", "item_stack.durability_range"
		}
		return "", ""
	}
	if stack.Durability != 0 {
		return "invalid-value", "item_stack.nondurable_durability"
	}
	return "", ""
}

// domainValuesDropIDRule names the first rule a drop identity breaks, read from
// the same bounds `core.DropID.Valid` reads. The dimension is deliberately not
// checked: the Go rule checks only the slot range and the generation.
func domainValuesDropIDRule(drop core.DropID) (string, string) {
	if drop.Slot >= core.DropsPerChunk {
		return "invalid-value", "drop_id.slot_range"
	}
	if drop.Generation == 0 {
		return "invalid-value", "drop_id.generation_nonzero"
	}
	return "", ""
}

// domainValuesContainerRefRule names the first rule a container reference
// breaks, read from the per-chunk array sizes the Go container validators read.
func domainValuesContainerRefRule(ref core.ContainerRef) (string, string) {
	switch ref.Kind {
	case core.ContainerKindFurnace:
		if ref.Slot >= core.FurnacesPerChunk {
			return "invalid-value", "container_ref.furnace_slot_range"
		}
	case core.ContainerKindChest:
		if ref.Slot >= core.ChestsPerChunk {
			return "invalid-value", "container_ref.chest_slot_range"
		}
	default:
		return "invalid-enum", "container_ref.known_kind"
	}
	if ref.Generation == 0 {
		return "invalid-value", "container_ref.generation_nonzero"
	}
	return "", ""
}

// domainValuesIsSmeltingProduct reports whether one item number is a product of
// the Go smelting table.
//
// The set is derived by scanning the registered range with
// `core.SmeltingOutput` instead of being listed a second time, so a product that
// stops being produced stops being accepted here in the same edit and the two
// cannot drift apart.
func domainValuesIsSmeltingProduct(item int) bool {
	for candidate := 1; candidate < int(core.ItemIDMax); candidate++ {
		if output, ok := core.SmeltingOutput(core.ItemID(candidate)); ok && int(output) == item {
			return true
		}
	}
	return false
}

// domainValuesKindName renders one container kind for the recorded fields.
func domainValuesKindName(kind core.ContainerKind) string {
	switch kind {
	case core.ContainerKindFurnace:
		return "furnace"
	case core.ContainerKindChest:
		return "chest"
	default:
		return "unknown"
	}
}

// domainValuesContainerKind resolves the kind name one case names. The furnace
// kind is the Go zero value, so it is accepted both by name and by omission.
func domainValuesContainerKind(c CaseSpec, name string) (core.ContainerKind, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "furnace":
		return core.ContainerKindFurnace, nil
	case "chest":
		return core.ContainerKindChest, nil
	default:
		return core.ContainerKindFurnace, fmt.Errorf("runtime-oracle: case %s names unknown container kind %q", c.ID, name)
	}
}

// domainValuesItemNumber resolves the item field one case names.
func domainValuesItemNumber(c CaseSpec, spec domainValuesInput) (core.ItemID, error) {
	if spec.Item == nil {
		return core.ItemNone, fmt.Errorf("runtime-oracle: case %s requires an item number", c.ID)
	}
	if *spec.Item < 0 || *spec.Item > int(^uint16(0)) {
		return core.ItemNone, fmt.Errorf("runtime-oracle: case %s names item %d outside the uint16 range", c.ID, *spec.Item)
	}
	return core.ItemID(*spec.Item), nil
}

// domainValuesByteField resolves an optional field that must fit a uint8.
func domainValuesByteField(c CaseSpec, name string, value *int) (uint8, error) {
	if value == nil {
		return 0, nil
	}
	if *value < 0 || *value > int(^uint8(0)) {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %d outside the uint8 range", c.ID, name, *value)
	}
	return uint8(*value), nil
}

// domainValuesUint16Field resolves an optional field that must fit a uint16.
func domainValuesUint16Field(c CaseSpec, name string, value *int) (uint16, error) {
	if value == nil {
		return 0, nil
	}
	if *value < 0 || *value > int(^uint16(0)) {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %d outside the uint16 range", c.ID, name, *value)
	}
	return uint16(*value), nil
}

// domainValuesUint32Field resolves an optional field that must fit a uint32.
func domainValuesUint32Field(c CaseSpec, name string, value *int64) (uint32, error) {
	if value == nil {
		return 0, nil
	}
	if *value < 0 || *value > int64(^uint32(0)) {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %d outside the uint32 range", c.ID, name, *value)
	}
	return uint32(*value), nil
}

// domainValuesDurabilityMax unwraps the Go durability table for one item,
// reporting zero for a nondurable item. It is a thin read of the same table the
// authority reads, not a second copy of the maxima.
func domainValuesDurabilityMax(item core.ItemID) uint16 {
	max, durable := core.ItemMaxDurability(item)
	if !durable {
		return 0
	}
	return max
}

// domainValuesDecodeInput reads one frozen corpus input and rejects trailing
// content, so a producer never executes bytes the case did not name.
func domainValuesDecodeInput(data []byte) (domainValuesInput, error) {
	var spec domainValuesInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainValuesInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainValuesInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainValuesRecord pairs one executed case with the input and outcome the
// producer produced for it.
type domainValuesRecord struct {
	label   string
	input   domainValuesInput
	outcome Outcome
}

// domainValuesExecute runs the whole case table through the producer and returns
// one record per case in table order.
func domainValuesExecute(t *testing.T) []domainValuesRecord {
	t.Helper()

	cases := domainValuesCases()
	records := make([]domainValuesRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainValues(CaseSpec{ID: domainValuesCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainValuesRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// `domainValuesSyncCorpus` compares committed assets and optionally exports a
// complete producer candidate for controller review.
//
// Ordinary runs compare committed assets read-only against current producer
// output. Explicit `RUNTIME_ORACLE_EXPORT_DIR` publication writes the complete
// candidate to a fresh external directory and never mutates tracked assets.
func domainValuesSyncCorpus(t *testing.T, records []domainValuesRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainValuesCorpusRelDir))
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

	for relative, data := range want {
		target := filepath.Join(corpusDir, filepath.FromSlash(relative))
		committed, err := os.ReadFile(target)
		if err != nil {
			t.Fatalf("read frozen corpus case %s: %v", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed validator", relative)
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

	var relatives []string
	for relative := range want {
		relatives = append(relatives, relative)
	}
	sort.Strings(relatives)
	assets := make([]generatedAsset, 0, len(relatives))
	for _, relative := range relatives {
		assets = append(assets, generatedAsset{
			RelativePath: relative,
			Data:         want[relative],
		})
	}
	exportGeneratedAssetsFromEnvironment(t, root, "runtime-oracle/domain-values", assets)
}

// domainValuesCaseID renders the manifest case identity one corpus label
// carries.
func domainValuesCaseID(label string) string {
	return domainValuesFamily + "/" + domainValuesVersion + "/" + label
}

// `domainValuesWorkingManifest` clones the merged committed manifest into a
// producer-scoped selection stored in harness-owned temporary storage.
// `Cases` is narrowed to this producer's cases, unrelated family case lists are
// cleared, and the existing selected family receives its complete identity,
// current provenance and case list before `ReconcileWorking` validates it.
func domainValuesWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainValuesCorpusCases(t, root)

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
	sources := make([]SourceSpec, 0, len(domainValuesFamilySources))
	for _, relative := range domainValuesFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	selected := Family{
		ID:                domainValuesFamily,
		Kind:              "domain",
		Role:              "input",
		CurrentVersion:    domainValuesVersion,
		SupportedVersions: []string{domainValuesVersion},
		Source:            domainValuesSource,
		EventualOwner:     ownerDomain,
		NumericSemantics:  domainValuesNumericSemantics,
		Sources:           sources,
		Cases:             caseIDs,
	}
	registered := false
	for index := range cloned.Families {
		// A family this selection does not execute registers no case, so its
		// list must be empty for the manifest to describe itself.
		if cloned.Families[index].ID != domainValuesFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		cloned.Families[index] = selected
		registered = true
	}
	if !registered {
		t.Fatalf("working manifest has no %s family", domainValuesFamily)
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

// domainValuesCorpusCases reads the frozen corpus and registers one case per
// committed input, with digests proven against the files on disk.
func domainValuesCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainValuesCorpusRelDir))
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
		t.Fatalf("walk domain values corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no domain value case under %s", domainValuesCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainValuesLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainValuesReadEnvelope(t, input)
		if envelope.Consumer != domainValuesConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainValuesConsumer)
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
			ID:           domainValuesCaseID(label),
			Family:       domainValuesFamily,
			Version:      domainValuesVersion,
			Operation:    domainValuesOperation,
			Input:        AssetRef{Path: domainValuesCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainValuesCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainValuesConsumer,
		})
	}
	return cases
}

// domainValuesReadEnvelope reads the provenance envelope of one frozen corpus
// input.
func domainValuesReadEnvelope(t *testing.T, path string) domainValuesInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainValuesInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainValuesCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainValuesCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainValuesOracleExecutesEveryCase runs the whole case table through the
// real Go validators, proves the frozen corpus still matches what they produce,
// and then runs the same cases through the production runner so the executed
// evidence satisfies the completeness rules a published trace report does.
func TestDomainValuesOracleExecutesEveryCase(t *testing.T) {
	records := domainValuesExecute(t)
	domainValuesSyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainValuesWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainValuesAssertCaseSpecs(t, root, manifest)
	domainValuesAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainValuesCaseByID(t, manifest, obs.CaseID)
		expected := readExpectedOutcome(t, root, c)
		if !outcomesEqual(obs.Outcome, expected) {
			t.Fatalf("case %s produced %#v, want %#v", obs.CaseID, obs.Outcome, expected)
		}
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	for _, obs := range trace.Observations {
		c := domainValuesCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed domain value evidence failed trace validation: %v", err)
	}
}

// TestDomainValuesOracleCaseIdentitiesAreTheRustDomainConsumer pins the manifest
// identity of every domain value case: the operation, the version, the family
// and the consumer the change names for the Rust crate that owns these rules.
func TestDomainValuesOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainValuesWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainValuesOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainValuesOperation)
		}
		if c.Version != domainValuesVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainValuesVersion)
		}
		if c.RustConsumer != domainValuesConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainValuesConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainValuesLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainValuesReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainValuesConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainValuesConsumer)
		}
	}
}

func TestDomainValuesOracleWorkingManifestReconciles(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainValuesWorkingManifest(t, root)
	_, families, live := discoverLive(t)
	if _, err := ReconcileWorking(root, manifest, families, live, BaselineConsumerRegistry(), BaselineNegativeCoverageExceptions()); err != nil {
		t.Fatalf("values working manifest drifted from current registries: %v", err)
	}
}

// `TestDomainValuesOracleWorkingManifestDescribesItself` checks the producer-scoped
// selection from the merged manifest: every case passes production validation,
// provenance hashes match disk, and the selected family's case list matches the
// narrowed case index.
func TestDomainValuesOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainValuesWorkingManifest(t, root)
	domainValuesAssertCaseSpecs(t, root, manifest)

	family, ok := domainValuesFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainValuesFamily)
	}
	if len(family.Sources) != len(domainValuesFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainValuesFamily, len(family.Sources), len(domainValuesFamilySources))
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
		t.Fatalf("%s lists %d cases, want %d", domainValuesFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainValuesFamily, id)
		}
	}
}

// TestDomainValuesOracleOutcomesDistinguishAcceptedAndRejected pins that the
// producer is not returning one constant answer: the executed evidence has to
// record both accepted values and rejections, and the item table rows have to
// cover registered, unregistered, durable and smelting values.
func TestDomainValuesOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainValuesExecute(t)
	domainValuesAssertOutcomesDistinguishCases(t, records)

	accepted, rejected := 0, 0
	registered, unregistered, durable, smeltable := 0, 0, 0, 0
	for _, record := range records {
		switch record.outcome.Kind {
		case "ok":
			accepted++
		case "error":
			rejected++
		default:
			t.Fatalf("record %s publishes kind %q, want ok or error", record.label, record.outcome.Kind)
		}
		if record.outcome.Kind == "error" {
			if !domainValuesRejectionCategoryKnown(record.outcome.Category) {
				t.Fatalf("record %s publishes rejection category %q, which is not in the frozen vocabulary", record.label, record.outcome.Category)
			}
			continue
		}
		if record.input.Rule != "item-table" {
			continue
		}
		item := core.ItemID(*record.input.Item)
		if _, ok := core.ItemStackLimit(item); ok {
			registered++
		} else {
			unregistered++
		}
		if _, ok := core.ItemMaxDurability(item); ok {
			durable++
		}
		if _, ok := core.SmeltingOutput(item); ok {
			smeltable++
		}
	}
	if accepted == 0 || rejected == 0 {
		t.Fatalf("executed evidence records %d accepted and %d rejected cases, want both non-zero", accepted, rejected)
	}
	if registered == 0 || unregistered == 0 || durable == 0 || smeltable == 0 {
		t.Fatalf("item table rows cover %d registered, %d unregistered, %d durable and %d smeltable items, want all non-zero",
			registered, unregistered, durable, smeltable)
	}
}

// TestDomainValuesOracleRunnerRejectsUnregisteredFamily pins that a manifest
// naming a family this package cannot execute fails the run.
func TestDomainValuesOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainValuesWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainValuesOracleRunnerRejectsOperationFamilyMismatch pins that a case
// cannot declare one operation and be executed by another.
func TestDomainValuesOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainValuesWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation/family mismatch failure, got: %v", err)
	}
}

// TestDomainValuesOracleRunnerRejectsTamperedInput pins that a producer never
// executes bytes the manifest does not name.
func TestDomainValuesOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainValuesWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainValuesOracleReportPublishesAndValidates assembles the executed
// evidence into a report, publishes it through the production atomic exporter,
// reloads it and validates it against the working manifest. This is the identity
// and content check a later Rust acceptance step performs; it does not claim any
// Rust behaviour.
func TestDomainValuesOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainValuesWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed domain value evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainValuesCorpusReportName)
	if err := ExportTrace(root, target, trace, manifest); err != nil {
		t.Fatalf("ExportTrace: %v", err)
	}
	loaded, err := LoadTraceAtRoot(root, target, manifest)
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

// TestDomainValuesOracleRejectsMissingProducerTest pins that the executed
// evidence has a real producer behind it. A corpus whose producer test is gone
// has no independent execution, only frozen files.
func TestDomainValuesOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainValuesProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainValuesProducerTestRelPath, err)
	}
	declaration := "func " + domainValuesProducerTestName + "(t *testing.T) {"
	if !strings.Contains(string(data), declaration) {
		t.Fatalf("%s does not declare %s", domainValuesProducerTestRelPath, declaration)
	}
}

// TestDomainOracle_items_locations is the topic-named entry point the domain
// plan names for this node. It delegates to the same executed table, so the two
// filters select one source of expected results rather than two.
func TestDomainOracle_items_locations(t *testing.T) {
	TestDomainValuesOracleExecutesEveryCase(t)
}

// domainValuesAssertCaseSpecs runs the production case validation over the whole
// selection, so a case with a bad path, digest, format or checkpoint fails here
// rather than surfacing later as a confusing coverage failure.
func domainValuesAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
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

// domainValuesAssertOutcomesDistinguishCases proves the executed evidence
// separates accepted values from rejections instead of publishing one constant
// answer, and that the rejections name a rule the authority publishes.
func domainValuesAssertOutcomesDistinguishCases(t *testing.T, records []domainValuesRecord) {
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
			if !domainValuesRejectionCategoryKnown(record.outcome.Category) {
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

// domainValuesRejectionCategoryKnown reports whether one rejection category is
// inside the frozen execution-contract vocabulary.
func domainValuesRejectionCategoryKnown(category string) bool {
	return corpusStructuralCategories[category] || corpusAdmissionCategories[category] ||
		corpusStorageCategories[category]
}

// domainValuesFamilySpec resolves one family from a manifest selection.
func domainValuesFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainValuesFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainValuesCaseByID indexes a manifest selection by case identity.
func domainValuesCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
