package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file is the Go producer for the domain inventory and container
// publication event family. It executes every committed corpus case through
// the current Go protocol DTOs: the verdict comes from
// `protocol.ValidateServerPacket`, which is the same Play-state validator the
// codec applies on both the encode and the decode side, and an admitted record
// is normalized field by field through the frozen field map. No world
// authority, listener, model call or native ABI is involved: the producer only
// reads immutable corpus inputs and calls in-process validators, and the
// frozen corpus files it materializes are read-only inputs for later
// consumers.
//
// The five rules this family executes are the Go `protocol.InventoryState`,
// `protocol.CraftingState`, `protocol.FurnaceState`, `protocol.ChestState` and
// `protocol.ContainerClosed` records, which are the item and container
// publications an authoritative session sends to the one player that owns
// them.
//
// Two Go wire rules deliberately stay out of this producer, because the domain
// values they describe do not carry them and a case pinning one could not be
// replayed against the Rust consumer. A container reference's wire dimension
// is validated by the protocol conversion before a `ContainerRef` exists, so
// the producer stamps the overworld — the only dimension both container arrays
// live in — and the input carries no dimension field; and the per-stack
// validity rules are shared with the values family, so the stack classifier is
// the values producer's rather than a second copy. Every array a case carries
// is serialized explicitly, an empty one included, so a replay consumer never
// reads a missing key as an absent array.

const (
	// domainEventInventoryFamily is the corpus family this package executes.
	// The family is the existing `domain.event` row, whose eventual owner is
	// the Rust domain crate that owns the replay observation records.
	domainEventInventoryFamily = "domain.event"
	// domainEventInventoryVersion is the family's discovered version. A case
	// has to name its family's version, so the case identities carry this
	// segment rather than one this producer chose.
	domainEventInventoryVersion = "1"
	// domainEventInventoryOperation is the manifest operation name for an
	// inventory or container publication admission case.
	domainEventInventoryOperation = "admit"
	// domainEventInventoryConsumer is the manifest consumer the change pins
	// for this family: the Rust crate that owns these records.
	domainEventInventoryConsumer = "mornlea_domain"
	// domainEventInventoryCorpusRelDir is the repository-relative directory
	// holding the frozen event inventory corpus cases.
	domainEventInventoryCorpusRelDir = "testdata/runtime-migration/cases/domain/event_inventory"
	// domainEventInventoryProducerTestRelPath and the two producer test names
	// locate the package-local producer that executes the protocol DTOs. The
	// topic name is the entry point the domain plan's filter requires; a
	// corpus whose producer test is missing has no independently executed
	// evidence at all.
	domainEventInventoryProducerTestRelPath = "packages/tools/cmd/runtime-oracle/domain_event_inventory_test.go"
	domainEventInventoryProducerExecuteName = "TestDomainEventInventoryOracleExecutesEveryCase"
	domainEventInventoryProducerTopicName   = "TestDomainOracle_event_inventory"
	// domainEventInventoryCorpusReportName is the published report file name
	// for the executed event inventory evidence.
	domainEventInventoryCorpusReportName = "runtime-corpus-domain-event-inventory.json"
	// domainEventInventorySource is the primary provenance source of the
	// container publication rules.
	domainEventInventorySource = "packages/shared/network/protocol/message_container.go"

	// domainEventInventoryRuleInventory, domainEventInventoryRuleCrafting,
	// domainEventInventoryRuleFurnace, domainEventInventoryRuleChest and
	// domainEventInventoryRuleClosed are the five rule names a case names. The
	// family is shared with the world observations and the player and outcome
	// records, so the rule name is the discriminator the manifest and the
	// family router both read.
	domainEventInventoryRuleInventory = "inventory-state"
	domainEventInventoryRuleCrafting  = "crafting-state"
	domainEventInventoryRuleFurnace   = "furnace-state"
	domainEventInventoryRuleChest     = "chest-state"
	domainEventInventoryRuleClosed    = "container-closed"

	// domainEventInventoryCraftingPersonal and
	// domainEventInventoryCraftingWorkbench are the two published grid side
	// lengths. The Go protocol package hardcodes them beside its own
	// `CraftingState.Validate` because network cannot depend on sim, so this
	// producer carries the same pair and the value tests on both sides pin
	// them.
	domainEventInventoryCraftingPersonal  = 2
	domainEventInventoryCraftingWorkbench = 3

	// domainEventInventoryKindFurnace and domainEventInventoryKindChest are
	// the two published container kinds, with furnace the zero value. The
	// input carries the raw number so an unknown kind stays expressible: the
	// container-neutral closure notification is the one record whose Go
	// validator rejects a kind neither container array names.
	domainEventInventoryKindFurnace = 0
	domainEventInventoryKindChest   = 1
)

// domainEventInventoryFamilySources is the merged provenance set the family
// records. Every entry is a file the producer's rules are read from, so a
// change to any of them is a change to the recorded evidence. The core item
// and inventory files belong to the set because the slot and stack rules are
// the Go `core` rules those files define, the furnace and chest files carry
// the timer bounds and the per-chunk array sizes, the smelting file is the
// authority behind the input and output whitelists, and the registry file
// decides which container records are published in the play state.
var domainEventInventoryFamilySources = []string{
	"packages/shared/network/protocol/message_inventory.go",
	"packages/shared/network/protocol/message_container.go",
	"packages/shared/network/protocol/registry.go",
	"packages/shared/core/inventory.go",
	"packages/shared/core/item.go",
	"packages/shared/core/smelting.go",
	"packages/shared/core/furnace.go",
	"packages/shared/core/chest.go",
	"packages/shared/core/container.go",
}

// domainEventInventoryLabelPattern is the shape a corpus label must have: a
// lowercase slug with an optional zero-padded numeric suffix, so a boundary
// row sorts in numeric order under a lexical sort.
var domainEventInventoryLabelPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*(-[0-9]+)?$`)

// updateDomainEventInventoryCorpus rewrites the frozen event inventory corpus
// from the executed protocol DTOs. It follows the same discipline as the other
// fixture update flags: an ordinary run only compares, so a frozen artifact is
// never silently regenerated to match an implementation.
var updateDomainEventInventoryCorpus = flag.Bool(
	"update-domain-event-inventory-corpus",
	false,
	"rewrite testdata/runtime-migration/cases/domain/event_inventory from the executed protocol inventory and container DTOs",
)

// domainEventInventoryInput is the frozen, self-describing corpus input for one
// case.
//
// It deliberately carries no expected outcome: a producer that could read the
// expectation from its own input would be able to agree with the recorded
// evidence instead of with the authority. Every scalar is a pointer so an
// absent field and an explicit zero stay distinguishable, which matters
// because a zero selected index, a zero size, a zero progress and a zero
// generation are all meaningful values. The four arrays are plain slices
// without "omitempty", so a case that carries an array always carries the key
// and an empty array renders as an explicit empty list rather than vanishing.
// The five rules share one envelope because each rule reads only the fields it
// names.
type domainEventInventoryInput struct {
	Consumer string `json:"consumer"`
	Rule     string `json:"rule"`

	// Inventory publication fields: the selected hotbar index and the two
	// fixed slot arrays.
	Selected *int                        `json:"selected,omitempty"`
	Hotbar   []domainEventInventoryStack `json:"hotbar"`
	Backpack []domainEventInventoryStack `json:"backpack"`

	// Crafting publication fields: the grid side length, the nine grid slots
	// and the derived output.
	Size   *int                        `json:"size,omitempty"`
	Slots  []domainEventInventoryStack `json:"slots"`
	Output *domainEventInventoryStack  `json:"output,omitempty"`

	// Furnace publication fields: the reference, the three slots and the two
	// timers.
	Container     *domainEventInventoryContainer `json:"container,omitempty"`
	Input         *domainEventInventoryStack     `json:"input,omitempty"`
	Fuel          *domainEventInventoryStack     `json:"fuel,omitempty"`
	ProgressTicks *int                           `json:"progress_ticks,omitempty"`
	BurnTicks     *int                           `json:"burn_ticks,omitempty"`

	// Chest publication fields: the reference and the twenty-seven chest
	// slots, which reuse the crafting grid's array field name so one rule
	// never carries two arrays.
	Items []domainEventInventoryStack `json:"items"`
}

// domainEventInventoryStack is the frozen rendering of one slot value. The
// three fields are written even when empty, because the empty slot is the
// exact zero triple the authority publishes and an absent field has to stay
// distinguishable from an explicit zero.
type domainEventInventoryStack struct {
	Item       int `json:"item"`
	Count      int `json:"count"`
	Durability int `json:"durability"`
}

// domainEventInventoryContainer is the frozen rendering of one container
// reference. It carries no dimension because the domain `ContainerRef` has
// none: the protocol conversion validates the raw wire dimension before it
// constructs one, so the producer stamps the overworld and a case cannot name
// a dimension the domain value could not replay.
type domainEventInventoryContainer struct {
	Kind       int     `json:"kind"`
	Chunk      []int32 `json:"chunk"`
	Slot       int     `json:"slot"`
	Generation uint32  `json:"generation"`
}

// domainEventInventoryCase is one frozen corpus case: its label and its input,
// and nothing else.
type domainEventInventoryCase struct {
	label string
	input domainEventInventoryInput
}

// domainEventInventoryBaseInput is the envelope every case starts from: the
// consumer, the rule and four explicitly empty arrays, so a case that names no
// array still serializes the key as an empty list rather than dropping it.
func domainEventInventoryBaseInput(rule string) domainEventInventoryInput {
	return domainEventInventoryInput{
		Consumer: domainEventInventoryConsumer,
		Rule:     rule,
		Hotbar:   []domainEventInventoryStack{},
		Backpack: []domainEventInventoryStack{},
		Slots:    []domainEventInventoryStack{},
		Items:    []domainEventInventoryStack{},
	}
}

func domainEventInventoryInt(value int) *int {
	return &value
}

// domainEventInventoryStackValue renders one Go slot value in the frozen
// envelope.
func domainEventInventoryStackValue(stack core.ItemStack) domainEventInventoryStack {
	return domainEventInventoryStack{
		Item:       int(stack.Item),
		Count:      int(stack.Count),
		Durability: int(stack.Durability),
	}
}

// domainEventInventoryStackList renders one fixed Go slot array in the frozen
// envelope.
func domainEventInventoryStackList(stacks []core.ItemStack) []domainEventInventoryStack {
	rendered := make([]domainEventInventoryStack, 0, len(stacks))
	for _, stack := range stacks {
		rendered = append(rendered, domainEventInventoryStackValue(stack))
	}
	return rendered
}

// domainEventInventoryRefValue renders one Go container reference in the
// frozen envelope, stamping the overworld dimension the Go validator requires
// and the domain value does not carry.
func domainEventInventoryRefValue(ref core.ContainerRef) domainEventInventoryContainer {
	return domainEventInventoryContainer{
		Kind:       int(ref.Kind),
		Chunk:      []int32{ref.Chunk.X, ref.Chunk.Z},
		Slot:       int(ref.Slot),
		Generation: ref.Generation,
	}
}

// domainEventInventoryFurnaceRef is the seed furnace reference: the overworld,
// chunk (2,-3), the last slot of the fixed furnace array and a nonzero
// generation.
func domainEventInventoryFurnaceRef() core.ContainerRef {
	return core.ContainerRef{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: 2, Z: -3},
		Kind:       core.ContainerKindFurnace,
		Slot:       core.FurnacesPerChunk - 1,
		Generation: 9,
	}
}

// domainEventInventoryChestRef is the seed chest reference: the overworld,
// chunk (-4,5), the last slot of the fixed chest array and a nonzero
// generation.
func domainEventInventoryChestRef() core.ContainerRef {
	return core.ContainerRef{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: -4, Z: 5},
		Kind:       core.ContainerKindChest,
		Slot:       core.ChestsPerChunk - 1,
		Generation: 4,
	}
}

// domainEventInventoryInventorySeed is the seed inventory publication: the
// last hotbar slot selected and every slot empty, which is the canonical
// empty publication.
func domainEventInventoryInventorySeed() core.Inventory {
	return core.Inventory{Hotbar: core.Hotbar{Selected: core.HotbarSlots - 1}}
}

// domainEventInventoryInventoryCase renders one inventory row: the seed
// publication with exactly one field changed, so a boundary row is always the
// seed plus the value the rule names.
func domainEventInventoryInventoryCase(label string, mutate func(*core.Inventory)) domainEventInventoryCase {
	inventory := domainEventInventoryInventorySeed()
	if mutate != nil {
		mutate(&inventory)
	}
	input := domainEventInventoryBaseInput(domainEventInventoryRuleInventory)
	input.Selected = domainEventInventoryInt(int(inventory.Hotbar.Selected))
	input.Hotbar = domainEventInventoryStackList(inventory.Hotbar.Slots[:])
	input.Backpack = domainEventInventoryStackList(inventory.Backpack[:])
	return domainEventInventoryCase{label: label, input: input}
}

// domainEventInventoryCraftingSeed is the seed crafting publication: the
// personal grid with every slot and the derived output empty.
func domainEventInventoryCraftingSeed() protocol.CraftingState {
	return protocol.CraftingState{Size: domainEventInventoryCraftingPersonal}
}

// domainEventInventoryCraftingCase renders one crafting row: the seed grid
// with exactly one field changed.
func domainEventInventoryCraftingCase(label string, mutate func(*protocol.CraftingState)) domainEventInventoryCase {
	state := domainEventInventoryCraftingSeed()
	if mutate != nil {
		mutate(&state)
	}
	input := domainEventInventoryBaseInput(domainEventInventoryRuleCrafting)
	size := int(state.Size)
	input.Size = &size
	input.Slots = domainEventInventoryStackList(state.Slots[:])
	input.Output = &domainEventInventoryStack{}
	*input.Output = domainEventInventoryStackValue(state.Output)
	return domainEventInventoryCase{label: label, input: input}
}

// domainEventInventoryFurnaceSeed is the seed furnace publication: raw iron
// smelting into an iron ingot on a coal fire, one tick short of the
// requirement and at the burn maximum.
func domainEventInventoryFurnaceSeed() protocol.FurnaceState {
	return protocol.FurnaceState{
		Furnace:       domainEventInventoryFurnaceRef(),
		Input:         core.ItemStack{Item: core.ItemRawIron, Count: 1},
		Fuel:          core.ItemStack{Item: core.ItemCoal, Count: 1},
		Output:        core.ItemStack{Item: core.ItemIronIngot, Count: 1},
		ProgressTicks: core.FurnaceSmeltTicks - 1,
		BurnTicks:     core.FurnaceBurnTicks,
	}
}

// domainEventInventoryFurnaceCase renders one furnace row: the seed
// publication with exactly one field changed.
func domainEventInventoryFurnaceCase(label string, mutate func(*protocol.FurnaceState)) domainEventInventoryCase {
	state := domainEventInventoryFurnaceSeed()
	if mutate != nil {
		mutate(&state)
	}
	input := domainEventInventoryBaseInput(domainEventInventoryRuleFurnace)
	input.Container = &domainEventInventoryContainer{}
	*input.Container = domainEventInventoryRefValue(state.Furnace)
	input.Input = &domainEventInventoryStack{}
	*input.Input = domainEventInventoryStackValue(state.Input)
	input.Fuel = &domainEventInventoryStack{}
	*input.Fuel = domainEventInventoryStackValue(state.Fuel)
	input.Output = &domainEventInventoryStack{}
	*input.Output = domainEventInventoryStackValue(state.Output)
	input.ProgressTicks = domainEventInventoryInt(int(state.ProgressTicks))
	input.BurnTicks = domainEventInventoryInt(int(state.BurnTicks))
	return domainEventInventoryCase{label: label, input: input}
}

// domainEventInventoryChestSeed is the seed chest publication: the chest
// reference with one stone stack in the last chest slot and every other slot
// empty.
func domainEventInventoryChestSeed() protocol.ChestState {
	chest := protocol.ChestState{Chest: domainEventInventoryChestRef()}
	chest.Items[core.ChestSlots-1] = core.ItemStack{Item: core.ItemStone, Count: 64}
	return chest
}

// domainEventInventoryChestCase renders one chest row: the seed publication
// with exactly one field changed.
func domainEventInventoryChestCase(label string, mutate func(*protocol.ChestState)) domainEventInventoryCase {
	state := domainEventInventoryChestSeed()
	if mutate != nil {
		mutate(&state)
	}
	input := domainEventInventoryBaseInput(domainEventInventoryRuleChest)
	input.Container = &domainEventInventoryContainer{}
	*input.Container = domainEventInventoryRefValue(state.Chest)
	input.Items = domainEventInventoryStackList(state.Items[:])
	return domainEventInventoryCase{label: label, input: input}
}

// domainEventInventoryClosedCase renders one closure row, whose entire payload
// is the container reference.
func domainEventInventoryClosedCase(label string, ref core.ContainerRef) domainEventInventoryCase {
	input := domainEventInventoryBaseInput(domainEventInventoryRuleClosed)
	input.Container = &domainEventInventoryContainer{}
	*input.Container = domainEventInventoryRefValue(ref)
	return domainEventInventoryCase{label: label, input: input}
}

// domainEventInventoryCases is the ordered case table the producer executes.
//
// The inventory rows are the seed plus one boundary each: the populated
// publication that proves the selected index and both arrays survive, and the
// selected index above the fixed hotbar count. The crafting rows are the
// personal seed, the residue the personal grid refuses at slot 4, the
// workbench's ninth slot and an unknown size. The furnace rows are the seed
// and its idle form, the two timer boundaries, every smelting input and every
// smelting product, the three slot whitelists, an unregistered input, the
// wrong reference kind, the slot above the fixed array and the zero
// generation. The chest rows are the seed, an unregistered item, the wrong
// reference kind and the slot above the fixed array. The closure rows are both
// kinds beside an unknown one, because the Go packet is named `FurnaceEnd` for
// historical reasons while carrying both container kinds.
func domainEventInventoryCases() []domainEventInventoryCase {
	cases := []domainEventInventoryCase{
		domainEventInventoryInventoryCase("inventory-state-seed", nil),
		domainEventInventoryInventoryCase("inventory-state-populated-slots", func(inventory *core.Inventory) {
			inventory.Hotbar.Selected = 0
			inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
			inventory.Hotbar.Slots[core.HotbarSlots-1] = core.ItemStack{Item: core.ItemCoal, Count: 1}
			inventory.Backpack[core.BackpackSlots-1] = core.ItemStack{Item: core.ItemIronIngot, Count: 1}
		}),
		domainEventInventoryInventoryCase("inventory-state-selected-above-hotbar", func(inventory *core.Inventory) {
			inventory.Hotbar.Selected = core.HotbarSlots
		}),

		domainEventInventoryCraftingCase("crafting-state-personal-seed", nil),
		domainEventInventoryCraftingCase("crafting-state-personal-residue-slot-four", func(state *protocol.CraftingState) {
			state.Slots[4] = core.ItemStack{Item: core.ItemStone, Count: 1}
		}),
		domainEventInventoryCraftingCase("crafting-state-workbench-slot-eight", func(state *protocol.CraftingState) {
			state.Size = domainEventInventoryCraftingWorkbench
			state.Slots[8] = core.ItemStack{Item: core.ItemStone, Count: 1}
		}),
		domainEventInventoryCraftingCase("crafting-state-size-unknown", func(state *protocol.CraftingState) {
			state.Size = domainEventInventoryCraftingWorkbench + 1
		}),

		domainEventInventoryFurnaceCase("furnace-state-seed", nil),
		domainEventInventoryFurnaceCase("furnace-state-idle", func(state *protocol.FurnaceState) {
			state.Input = core.ItemStack{}
			state.Fuel = core.ItemStack{}
			state.Output = core.ItemStack{}
			state.ProgressTicks = 0
			state.BurnTicks = 0
		}),
		domainEventInventoryFurnaceCase("furnace-state-progress-at-requirement", func(state *protocol.FurnaceState) {
			state.ProgressTicks = core.FurnaceSmeltTicks
		}),
		domainEventInventoryFurnaceCase("furnace-state-burn-above-maximum", func(state *protocol.FurnaceState) {
			state.BurnTicks = core.FurnaceBurnTicks + 1
		}),
		domainEventInventoryFurnaceCase("furnace-state-chest-reference", func(state *protocol.FurnaceState) {
			state.Furnace = domainEventInventoryChestRef()
		}),
		domainEventInventoryFurnaceCase("furnace-state-slot-above-array", func(state *protocol.FurnaceState) {
			state.Furnace.Slot = core.FurnacesPerChunk
		}),
		domainEventInventoryFurnaceCase("furnace-state-zero-generation", func(state *protocol.FurnaceState) {
			state.Furnace.Generation = 0
		}),
		domainEventInventoryFurnaceCase("furnace-state-fuel-stone", func(state *protocol.FurnaceState) {
			state.Fuel = core.ItemStack{Item: core.ItemStone, Count: 1}
		}),
		domainEventInventoryFurnaceCase("furnace-state-input-stone", func(state *protocol.FurnaceState) {
			state.Input = core.ItemStack{Item: core.ItemStone, Count: 1}
		}),
		domainEventInventoryFurnaceCase("furnace-state-output-stone", func(state *protocol.FurnaceState) {
			state.Output = core.ItemStack{Item: core.ItemStone, Count: 1}
		}),
		domainEventInventoryFurnaceCase("furnace-state-input-unregistered-item", func(state *protocol.FurnaceState) {
			state.Input = core.ItemStack{Item: core.ItemIDMax, Count: 1}
		}),

		domainEventInventoryChestCase("chest-state-seed", nil),
		domainEventInventoryChestCase("chest-state-unregistered-item", func(state *protocol.ChestState) {
			state.Items[0] = core.ItemStack{Item: core.ItemIDMax, Count: 1}
		}),
		domainEventInventoryChestCase("chest-state-furnace-reference", func(state *protocol.ChestState) {
			state.Chest = domainEventInventoryFurnaceRef()
		}),
		domainEventInventoryChestCase("chest-state-slot-above-array", func(state *protocol.ChestState) {
			state.Chest.Slot = core.ChestsPerChunk
		}),

		domainEventInventoryClosedCase("container-closed-furnace", domainEventInventoryFurnaceRef()),
		domainEventInventoryClosedCase("container-closed-chest", domainEventInventoryChestRef()),
		domainEventInventoryClosedCase("container-closed-unknown-kind", core.ContainerRef{
			Dimension:  core.Overworld,
			Chunk:      core.ChunkPos{X: 0, Z: 0},
			Kind:       core.ContainerKind(7),
			Slot:       0,
			Generation: 3,
		}),
	}

	// The input whitelist is exactly the Go `core.SmeltingOutput` key set and
	// the output whitelist is its product set, so each side of the table is
	// one row rather than one row per item number.
	for _, entry := range []struct {
		name string
		item core.ItemID
	}{
		{"raw-iron", core.ItemRawIron},
		{"sand", core.ItemSand},
		{"clay", core.ItemClay},
		{"raw-beef", core.ItemRawBeef},
	} {
		item := entry.item
		cases = append(cases, domainEventInventoryFurnaceCase("furnace-state-input-"+entry.name, func(state *protocol.FurnaceState) {
			state.Input = core.ItemStack{Item: item, Count: 1}
		}))
	}
	for _, entry := range []struct {
		name string
		item core.ItemID
	}{
		{"iron-ingot", core.ItemIronIngot},
		{"glass", core.ItemGlass},
		{"brick", core.ItemBrick},
		{"cooked-beef", core.ItemCookedBeef},
	} {
		product := entry.item
		cases = append(cases, domainEventInventoryFurnaceCase("furnace-state-output-"+entry.name, func(state *protocol.FurnaceState) {
			state.Output = core.ItemStack{Item: product, Count: 1}
		}))
	}
	return cases
}

// runDomainEventInventory executes one corpus case through the current Go
// protocol inventory and container DTOs.
//
// The verdict always comes from `protocol.ValidateServerPacket` in the play
// state, which is the same validator the codec applies on both the encode and
// the decode side. The rule name and the rejection category come from the same
// bounds the DTO reads, and the two are cross-checked against each other, so a
// classification that disagrees with the authority fails the run instead of
// publishing a plausible-looking rejection.
func runDomainEventInventory(c CaseSpec, input []byte) (Outcome, []byte, error) {
	spec, err := domainEventInventoryDecodeInput(input)
	if err != nil {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: decode input: %w", c.ID, err)
	}
	if spec.Consumer != domainEventInventoryConsumer {
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names consumer %q, want %q", c.ID, spec.Consumer, domainEventInventoryConsumer)
	}

	switch spec.Rule {
	case domainEventInventoryRuleInventory:
		return domainEventInventoryRunInventory(c, spec)
	case domainEventInventoryRuleCrafting:
		return domainEventInventoryRunCrafting(c, spec)
	case domainEventInventoryRuleFurnace:
		return domainEventInventoryRunFurnace(c, spec)
	case domainEventInventoryRuleChest:
		return domainEventInventoryRunChest(c, spec)
	case domainEventInventoryRuleClosed:
		return domainEventInventoryRunClosed(c, spec)
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names unknown rule %q", c.ID, spec.Rule)
	}
}

// domainEventInventoryRunInventory admits one inventory publication, whose
// payload is the selected index and the two fixed slot arrays.
func domainEventInventoryRunInventory(c CaseSpec, spec domainEventInventoryInput) (Outcome, []byte, error) {
	inventory, err := domainEventInventoryInventory(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	state := protocol.InventoryState{Inventory: inventory}
	fields := domainEventInventoryNormalizeInventory(inventory)
	category, rule := domainEventInventoryInventoryRule(inventory)
	return domainEventInventoryFinish(c, domainEventInventoryRuleInventory, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, state) == nil)
}

// domainEventInventoryRunCrafting admits one crafting publication.
func domainEventInventoryRunCrafting(c CaseSpec, spec domainEventInventoryInput) (Outcome, []byte, error) {
	state, err := domainEventInventoryCrafting(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventInventoryNormalizeCrafting(state)
	category, rule := domainEventInventoryCraftingRule(state)
	return domainEventInventoryFinish(c, domainEventInventoryRuleCrafting, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, state) == nil)
}

// domainEventInventoryRunFurnace admits one furnace publication.
func domainEventInventoryRunFurnace(c CaseSpec, spec domainEventInventoryInput) (Outcome, []byte, error) {
	state, err := domainEventInventoryFurnace(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventInventoryNormalizeFurnace(state)
	category, rule := domainEventInventoryFurnaceRule(state)
	return domainEventInventoryFinish(c, domainEventInventoryRuleFurnace, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, state) == nil)
}

// domainEventInventoryRunChest admits one chest publication.
func domainEventInventoryRunChest(c CaseSpec, spec domainEventInventoryInput) (Outcome, []byte, error) {
	state, err := domainEventInventoryChest(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventInventoryNormalizeChest(state)
	category, rule := domainEventInventoryChestRule(state)
	return domainEventInventoryFinish(c, domainEventInventoryRuleChest, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, state) == nil)
}

// domainEventInventoryRunClosed admits one closure notification, whose entire
// payload is the container reference.
func domainEventInventoryRunClosed(c CaseSpec, spec domainEventInventoryInput) (Outcome, []byte, error) {
	closed, err := domainEventInventoryClosed(c, spec)
	if err != nil {
		return Outcome{}, nil, err
	}
	fields := domainEventInventoryNormalizeRef(closed.Container)
	category, rule := domainEventInventoryClosedRule(closed.Container)
	return domainEventInventoryFinish(c, domainEventInventoryRuleClosed, fields, category, rule, protocol.ValidateServerPacket(protocol.StatePlay, closed) == nil)
}

// domainEventInventoryInventory resolves one inventory publication from its
// frozen envelope. Every field is required and both arrays have to carry their
// exact fixed counts, because the record is complete by definition: an absent
// field would silently become a zero and a zero selected index is a
// meaningful value.
func domainEventInventoryInventory(c CaseSpec, spec domainEventInventoryInput) (core.Inventory, error) {
	if spec.Selected == nil {
		return core.Inventory{}, fmt.Errorf("runtime-oracle: case %s requires a selected index", c.ID)
	}
	selected, err := domainEventInventoryByte(c, "selected", spec.Selected)
	if err != nil {
		return core.Inventory{}, err
	}
	hotbarStacks, err := domainEventInventoryStackArray(c, "hotbar", spec.Hotbar, core.HotbarSlots)
	if err != nil {
		return core.Inventory{}, err
	}
	backpackStacks, err := domainEventInventoryStackArray(c, "backpack", spec.Backpack, core.BackpackSlots)
	if err != nil {
		return core.Inventory{}, err
	}
	var hotbar [core.HotbarSlots]core.ItemStack
	copy(hotbar[:], hotbarStacks)
	var backpack [core.BackpackSlots]core.ItemStack
	copy(backpack[:], backpackStacks)
	return core.Inventory{
		Hotbar:   core.Hotbar{Selected: selected, Slots: hotbar},
		Backpack: backpack,
	}, nil
}

// domainEventInventoryCrafting resolves one crafting publication from its
// frozen envelope.
func domainEventInventoryCrafting(c CaseSpec, spec domainEventInventoryInput) (protocol.CraftingState, error) {
	if spec.Size == nil {
		return protocol.CraftingState{}, fmt.Errorf("runtime-oracle: case %s requires a size", c.ID)
	}
	size, err := domainEventInventoryByte(c, "size", spec.Size)
	if err != nil {
		return protocol.CraftingState{}, err
	}
	slotStacks, err := domainEventInventoryStackArray(c, "slots", spec.Slots, core.CraftingGridSlots)
	if err != nil {
		return protocol.CraftingState{}, err
	}
	if spec.Output == nil {
		return protocol.CraftingState{}, fmt.Errorf("runtime-oracle: case %s requires an output stack", c.ID)
	}
	output, err := domainEventInventoryResolveStack(c, "output", *spec.Output)
	if err != nil {
		return protocol.CraftingState{}, err
	}
	var slots [core.CraftingGridSlots]core.ItemStack
	copy(slots[:], slotStacks)
	return protocol.CraftingState{Size: size, Slots: slots, Output: output}, nil
}

// domainEventInventoryFurnace resolves one furnace publication from its frozen
// envelope. The reference is stamped with the overworld dimension, because the
// domain value carries none and the Go validator accepts only the overworld.
func domainEventInventoryFurnace(c CaseSpec, spec domainEventInventoryInput) (protocol.FurnaceState, error) {
	if spec.Container == nil {
		return protocol.FurnaceState{}, fmt.Errorf("runtime-oracle: case %s requires a container reference", c.ID)
	}
	ref, err := domainEventInventoryContainerRef(c, "container", *spec.Container)
	if err != nil {
		return protocol.FurnaceState{}, err
	}
	input, err := domainEventInventoryResolveStack(c, "input", domainEventInventoryStackOrEmpty(spec.Input))
	if err != nil {
		return protocol.FurnaceState{}, err
	}
	fuel, err := domainEventInventoryResolveStack(c, "fuel", domainEventInventoryStackOrEmpty(spec.Fuel))
	if err != nil {
		return protocol.FurnaceState{}, err
	}
	output, err := domainEventInventoryResolveStack(c, "output", domainEventInventoryStackOrEmpty(spec.Output))
	if err != nil {
		return protocol.FurnaceState{}, err
	}
	if spec.ProgressTicks == nil {
		return protocol.FurnaceState{}, fmt.Errorf("runtime-oracle: case %s requires a progress", c.ID)
	}
	progress, err := domainEventInventoryByte(c, "progress_ticks", spec.ProgressTicks)
	if err != nil {
		return protocol.FurnaceState{}, err
	}
	if spec.BurnTicks == nil {
		return protocol.FurnaceState{}, fmt.Errorf("runtime-oracle: case %s requires a burn time", c.ID)
	}
	burn, err := domainEventInventoryUint16(c, "burn_ticks", spec.BurnTicks)
	if err != nil {
		return protocol.FurnaceState{}, err
	}
	return protocol.FurnaceState{
		Furnace:       ref,
		Input:         input,
		Fuel:          fuel,
		Output:        output,
		ProgressTicks: progress,
		BurnTicks:     burn,
	}, nil
}

// domainEventInventoryChest resolves one chest publication from its frozen
// envelope.
func domainEventInventoryChest(c CaseSpec, spec domainEventInventoryInput) (protocol.ChestState, error) {
	if spec.Container == nil {
		return protocol.ChestState{}, fmt.Errorf("runtime-oracle: case %s requires a container reference", c.ID)
	}
	ref, err := domainEventInventoryContainerRef(c, "container", *spec.Container)
	if err != nil {
		return protocol.ChestState{}, err
	}
	itemStacks, err := domainEventInventoryStackArray(c, "items", spec.Items, core.ChestSlots)
	if err != nil {
		return protocol.ChestState{}, err
	}
	var items [core.ChestSlots]core.ItemStack
	copy(items[:], itemStacks)
	return protocol.ChestState{Chest: ref, Items: items}, nil
}

// domainEventInventoryClosed resolves one closure notification from its frozen
// envelope.
func domainEventInventoryClosed(c CaseSpec, spec domainEventInventoryInput) (protocol.ContainerClosed, error) {
	if spec.Container == nil {
		return protocol.ContainerClosed{}, fmt.Errorf("runtime-oracle: case %s requires a container reference", c.ID)
	}
	ref, err := domainEventInventoryContainerRef(c, "container", *spec.Container)
	if err != nil {
		return protocol.ContainerClosed{}, err
	}
	return protocol.ContainerClosed{Container: ref}, nil
}

// domainEventInventoryStackOrEmpty renders one optional stack as its frozen
// value. A nil stack collapses to the zero stack, so an absent field and an
// explicit empty stack serialize identically; the furnace path relies on that
// collapse, and no committed case omits the field.
func domainEventInventoryStackOrEmpty(stack *domainEventInventoryStack) domainEventInventoryStack {
	if stack == nil {
		return domainEventInventoryStack{}
	}
	return *stack
}

// domainEventInventoryContainerRef resolves one container reference from its
// frozen rendering, stamping the overworld dimension the Go validator requires
// and the domain value does not carry.
func domainEventInventoryContainerRef(c CaseSpec, name string, container domainEventInventoryContainer) (core.ContainerRef, error) {
	if len(container.Chunk) != 2 {
		return core.ContainerRef{}, fmt.Errorf("runtime-oracle: case %s requires two %s chunk coordinates", c.ID, name)
	}
	if container.Slot < 0 || container.Slot > 255 {
		return core.ContainerRef{}, fmt.Errorf("runtime-oracle: case %s names %s slot %d, which is outside 0..255", c.ID, name, container.Slot)
	}
	if container.Kind < 0 || container.Kind > 255 {
		return core.ContainerRef{}, fmt.Errorf("runtime-oracle: case %s names %s kind %d, which is outside 0..255", c.ID, name, container.Kind)
	}
	return core.ContainerRef{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: container.Chunk[0], Z: container.Chunk[1]},
		Kind:       core.ContainerKind(container.Kind),
		Slot:       uint8(container.Slot),
		Generation: container.Generation,
	}, nil
}

// domainEventInventoryResolveStack resolves one slot value from its frozen
// rendering.
func domainEventInventoryResolveStack(c CaseSpec, name string, stack domainEventInventoryStack) (core.ItemStack, error) {
	item, err := domainEventInventoryUint16(c, name+".item", domainEventInventoryInt(stack.Item))
	if err != nil {
		return core.ItemStack{}, err
	}
	count, err := domainEventInventoryByte(c, name+".count", domainEventInventoryInt(stack.Count))
	if err != nil {
		return core.ItemStack{}, err
	}
	durability, err := domainEventInventoryUint16(c, name+".durability", domainEventInventoryInt(stack.Durability))
	if err != nil {
		return core.ItemStack{}, err
	}
	return core.ItemStack{Item: core.ItemID(item), Count: count, Durability: durability}, nil
}

// domainEventInventoryStackArray resolves one fixed slot array from its frozen
// rendering, requiring the exact count the Go DTO's array type holds: a short
// array would silently become zero-valued slots and a long one names slots the
// record does not have.
func domainEventInventoryStackArray(c CaseSpec, name string, stacks []domainEventInventoryStack, count int) ([]core.ItemStack, error) {
	if len(stacks) != count {
		return nil, fmt.Errorf("runtime-oracle: case %s requires %d %s stacks, got %d", c.ID, count, name, len(stacks))
	}
	resolved := make([]core.ItemStack, 0, len(stacks))
	for index, stack := range stacks {
		value, err := domainEventInventoryResolveStack(c, fmt.Sprintf("%s[%d]", name, index), stack)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, value)
	}
	return resolved, nil
}

// domainEventInventoryFinish records one admission.
//
// The classification is cross-checked against the authority's verdict, a
// rejection publishes the broken rule beside its category, and an admitted
// record publishes its normalized field map. No codec round trip is recorded:
// this family pins the semantic field values, and the wire layout of these
// packets belongs to the protocol nodes that port the codecs.
func domainEventInventoryFinish(c CaseSpec, subject string, fields map[string]any, category, rule string, admitted bool) (Outcome, []byte, error) {
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

// domainEventInventoryNormalizeInventory renders one inventory publication in
// the normalized field map. Both arrays publish every slot, so a replay proves
// the exact sequence the record carried.
func domainEventInventoryNormalizeInventory(inventory core.Inventory) map[string]any {
	return map[string]any{
		"selected": int(inventory.Hotbar.Selected),
		"hotbar":   domainEventInventoryNormalizeStacks(inventory.Hotbar.Slots[:]),
		"backpack": domainEventInventoryNormalizeStacks(inventory.Backpack[:]),
	}
}

// domainEventInventoryNormalizeCrafting renders one crafting publication. The
// grid size is the semantic name the domain enum publishes rather than the
// wire's side length, so a replay compares the grid kind instead of a number.
func domainEventInventoryNormalizeCrafting(state protocol.CraftingState) map[string]any {
	return map[string]any{
		"size":   domainEventInventorySizeText(state.Size),
		"slots":  domainEventInventoryNormalizeStacks(state.Slots[:]),
		"output": domainEventInventoryNormalizeStack(state.Output),
	}
}

// domainEventInventoryNormalizeFurnace renders one furnace publication.
func domainEventInventoryNormalizeFurnace(state protocol.FurnaceState) map[string]any {
	fields := domainEventInventoryNormalizeRef(state.Furnace)
	fields["input"] = domainEventInventoryNormalizeStack(state.Input)
	fields["fuel"] = domainEventInventoryNormalizeStack(state.Fuel)
	fields["output"] = domainEventInventoryNormalizeStack(state.Output)
	fields["progress_ticks"] = int(state.ProgressTicks)
	fields["burn_ticks"] = int(state.BurnTicks)
	return fields
}

// domainEventInventoryNormalizeChest renders one chest publication.
func domainEventInventoryNormalizeChest(state protocol.ChestState) map[string]any {
	fields := domainEventInventoryNormalizeRef(state.Chest)
	fields["items"] = domainEventInventoryNormalizeStacks(state.Items[:])
	return fields
}

// domainEventInventoryNormalizeRef renders one container reference in the
// normalized field map. The reference has no dimension here, because the
// domain value has none; the kind is the stable text the corpus vocabulary
// uses and an unknown kind names itself as such.
func domainEventInventoryNormalizeRef(ref core.ContainerRef) map[string]any {
	return map[string]any{
		"container": map[string]any{
			"kind":       domainEventInventoryKindText(ref.Kind),
			"chunk":      []int{int(ref.Chunk.X), int(ref.Chunk.Z)},
			"slot":       int(ref.Slot),
			"generation": int(ref.Generation),
		},
	}
}

// domainEventInventoryNormalizeStacks renders one fixed slot array.
func domainEventInventoryNormalizeStacks(stacks []core.ItemStack) []map[string]any {
	rendered := make([]map[string]any, 0, len(stacks))
	for _, stack := range stacks {
		rendered = append(rendered, domainEventInventoryNormalizeStack(stack))
	}
	return rendered
}

// domainEventInventoryNormalizeStack renders one slot value. All three fields
// are published, because the empty slot is the zero triple and a missing field
// would not prove the authority published exactly that.
func domainEventInventoryNormalizeStack(stack core.ItemStack) map[string]any {
	return map[string]any{
		"item":       int(stack.Item),
		"count":      int(stack.Count),
		"durability": int(stack.Durability),
	}
}

// domainEventInventorySizeText renders one grid side length as the semantic
// name the domain enum publishes. An unknown size is a rejection, so the text
// never has to name one.
func domainEventInventorySizeText(size uint8) string {
	switch size {
	case domainEventInventoryCraftingPersonal:
		return "personal"
	case domainEventInventoryCraftingWorkbench:
		return "workbench"
	default:
		return "unknown"
	}
}

// domainEventInventoryKindText renders one container kind as the stable text
// the normalized corpus vocabulary uses. An unknown kind is a rejection the
// container-neutral notification publishes, so the text names it explicitly
// rather than collapsing onto a known kind.
func domainEventInventoryKindText(kind core.ContainerKind) string {
	switch kind {
	case core.ContainerKindFurnace:
		return "furnace"
	case core.ContainerKindChest:
		return "chest"
	default:
		return "unknown"
	}
}

// domainEventInventoryInventoryRule names the first rule one inventory
// publication breaks, in the same order the Go validator checks them: the
// selected index, then every hotbar slot, then every backpack slot.
//
// A rejection classifies as `invalid-enum` when the broken rule is an
// unregistered item number and as `invalid-value` otherwise, which is the
// classification rule a Rust consumer of this family applies to its own
// rejection variants. The stack rules themselves are the values producer's,
// so the two families cannot disagree about a category.
func domainEventInventoryInventoryRule(inventory core.Inventory) (string, string) {
	if inventory.Hotbar.Selected >= core.HotbarSlots {
		return "invalid-value", "inventory_state.selected_range"
	}
	for index, stack := range inventory.Hotbar.Slots {
		if category, rule := domainValuesItemStackRule(stack); rule != "" {
			return category, fmt.Sprintf("inventory_state.hotbar_slot_%d", index)
		}
	}
	for index, stack := range inventory.Backpack {
		if category, rule := domainValuesItemStackRule(stack); rule != "" {
			return category, fmt.Sprintf("inventory_state.backpack_slot_%d", index)
		}
	}
	return "", ""
}

// domainEventInventoryCraftingRule names the first rule one crafting
// publication breaks, in the same order the Go validator checks them: the
// size, then every slot in grid order with the personal residue rule, then the
// output.
func domainEventInventoryCraftingRule(state protocol.CraftingState) (string, string) {
	if state.Size != domainEventInventoryCraftingPersonal && state.Size != domainEventInventoryCraftingWorkbench {
		return "invalid-enum", "crafting_state.size"
	}
	for index, stack := range state.Slots {
		if category, rule := domainValuesItemStackRule(stack); rule != "" {
			return category, fmt.Sprintf("crafting_state.slot_%d", index)
		}
		if state.Size == domainEventInventoryCraftingPersonal &&
			index >= int(state.Size)*int(state.Size) && stack != (core.ItemStack{}) {
			return "invalid-value", "crafting_state.personal_residue"
		}
	}
	if category, rule := domainValuesItemStackRule(state.Output); rule != "" {
		return category, "crafting_state.output"
	}
	return "", ""
}

// domainEventInventoryFurnaceRule names the first rule one furnace publication
// breaks, in the same order the Go validator checks them: the reference kind,
// slot and generation, the two timers, and then the input, fuel and output
// whitelists.
//
// The Go validator checks both timers in one condition; this producer names
// the first bound the case breaks, so the progress and the burn boundaries are
// two rules a Rust consumer maps to its own timer rejection.
func domainEventInventoryFurnaceRule(state protocol.FurnaceState) (string, string) {
	if state.Furnace.Kind != core.ContainerKindFurnace {
		return "invalid-enum", "furnace_state.ref_kind"
	}
	if state.Furnace.Slot >= core.FurnacesPerChunk {
		return "invalid-value", "furnace_state.ref_slot"
	}
	if state.Furnace.Generation == 0 {
		return "invalid-value", "furnace_state.ref_generation"
	}
	if state.ProgressTicks >= core.FurnaceSmeltTicks {
		return "invalid-value", "furnace_state.progress_range"
	}
	if state.BurnTicks > core.FurnaceBurnTicks {
		return "invalid-value", "furnace_state.burn_range"
	}
	if category, rule := domainValuesItemStackRule(state.Input); rule != "" {
		return category, "furnace_state.input"
	}
	if state.Input.Item != core.ItemNone {
		if _, ok := core.SmeltingOutput(state.Input.Item); !ok {
			return "invalid-value", "furnace_state.input"
		}
	}
	if category, rule := domainValuesItemStackRule(state.Fuel); rule != "" {
		return category, "furnace_state.fuel"
	}
	if state.Fuel.Item != core.ItemNone && state.Fuel.Item != core.ItemCoal {
		return "invalid-value", "furnace_state.fuel"
	}
	if category, rule := domainValuesItemStackRule(state.Output); rule != "" {
		return category, "furnace_state.output"
	}
	switch state.Output.Item {
	case core.ItemNone, core.ItemIronIngot, core.ItemGlass, core.ItemBrick, core.ItemCookedBeef:
	default:
		return "invalid-value", "furnace_state.output"
	}
	return "", ""
}

// domainEventInventoryChestRule names the first rule one chest publication
// breaks, in the same order the Go validator checks them: the reference kind,
// slot and generation, then every chest slot.
func domainEventInventoryChestRule(state protocol.ChestState) (string, string) {
	if state.Chest.Kind != core.ContainerKindChest {
		return "invalid-enum", "chest_state.ref_kind"
	}
	if state.Chest.Slot >= core.ChestsPerChunk {
		return "invalid-value", "chest_state.ref_slot"
	}
	if state.Chest.Generation == 0 {
		return "invalid-value", "chest_state.ref_generation"
	}
	for index, stack := range state.Items {
		if category, rule := domainValuesItemStackRule(stack); rule != "" {
			return category, fmt.Sprintf("chest_state.item_%d", index)
		}
	}
	return "", ""
}

// domainEventInventoryClosedRule names the first rule one closure notification
// breaks, in the same order the Go validator checks them: the kind has to be
// one the two container arrays name, then the slot bound of that kind, then
// the generation. The record is container-neutral, so both kinds are admitted.
func domainEventInventoryClosedRule(ref core.ContainerRef) (string, string) {
	switch ref.Kind {
	case core.ContainerKindFurnace:
		if ref.Slot >= core.FurnacesPerChunk {
			return "invalid-value", "container_closed.ref_slot"
		}
	case core.ContainerKindChest:
		if ref.Slot >= core.ChestsPerChunk {
			return "invalid-value", "container_closed.ref_slot"
		}
	default:
		return "invalid-enum", "container_closed.ref_kind"
	}
	if ref.Generation == 0 {
		return "invalid-value", "container_closed.ref_generation"
	}
	return "", ""
}

// domainEventInventoryByte resolves one u8 field a case names as a decimal
// integer.
func domainEventInventoryByte(c CaseSpec, name string, value *int) (uint8, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	if *value < 0 || *value > 255 {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %d, which is outside 0..255", c.ID, name, *value)
	}
	return uint8(*value), nil
}

// domainEventInventoryUint16 resolves one u16 field a case names as a decimal
// integer.
func domainEventInventoryUint16(c CaseSpec, name string, value *int) (uint16, error) {
	if value == nil {
		return 0, fmt.Errorf("runtime-oracle: case %s requires %s", c.ID, name)
	}
	if *value < 0 || *value > 65535 {
		return 0, fmt.Errorf("runtime-oracle: case %s names %s %d, which is outside 0..65535", c.ID, name, *value)
	}
	return uint16(*value), nil
}

// domainEventInventoryDecodeInput reads one frozen corpus input and rejects
// trailing content, so a producer never executes bytes the case did not name.
func domainEventInventoryDecodeInput(data []byte) (domainEventInventoryInput, error) {
	var spec domainEventInventoryInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&spec); err != nil {
		return domainEventInventoryInput{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return domainEventInventoryInput{}, fmt.Errorf("trailing content after the JSON value")
	}
	return spec, nil
}

// domainEventInventoryRecord pairs one executed case with the input and
// outcome the producer produced for it.
type domainEventInventoryRecord struct {
	label   string
	input   domainEventInventoryInput
	outcome Outcome
}

// domainEventInventoryExecute runs the whole case table through the producer
// and returns one record per case in table order.
func domainEventInventoryExecute(t *testing.T) []domainEventInventoryRecord {
	t.Helper()

	cases := domainEventInventoryCases()
	records := make([]domainEventInventoryRecord, 0, len(cases))
	for _, entry := range cases {
		input, err := json.MarshalIndent(entry.input, "", "  ")
		if err != nil {
			t.Fatalf("encode corpus input for %s: %v", entry.label, err)
		}
		input = append(input, '\n')
		outcome, _, err := runDomainEventInventory(CaseSpec{ID: domainEventInventoryCaseID(entry.label)}, input)
		if err != nil {
			t.Fatalf("execute case %s: %v", entry.label, err)
		}
		records = append(records, domainEventInventoryRecord{label: entry.label, input: entry.input, outcome: outcome})
	}
	return records
}

// domainEventInventorySyncCorpus writes or verifies the frozen corpus files.
//
// An ordinary run is read-only: it proves the committed files still match what
// the protocol DTOs produce now, so a drifted artifact fails instead of being
// regenerated. The explicit update flag rewrites them, which is the only way a
// frozen artifact changes.
func domainEventInventorySyncCorpus(t *testing.T, records []domainEventInventoryRecord) {
	t.Helper()

	root := mustRepoRoot(t)
	corpusDir := filepath.Join(root, filepath.FromSlash(domainEventInventoryCorpusRelDir))
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

	if *updateDomainEventInventoryCorpus {
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
			t.Fatalf("read frozen corpus case %s: %v (rerun with -update-domain-event-inventory-corpus after reviewing the change)", relative, err)
		}
		if !bytes.Equal(committed, data) {
			t.Errorf("frozen corpus case %s drifted from the executed protocol DTO (rerun with -update-domain-event-inventory-corpus after reviewing the change)", relative)
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

// domainEventInventoryCaseID renders the manifest case identity one corpus
// label carries. The version segment is the family's published version,
// because a case has to name the version of the family it belongs to.
func domainEventInventoryCaseID(label string) string {
	return domainEventInventoryFamily + "/" + domainEventInventoryVersion + "/" + label
}

// domainEventInventoryWorkingManifest assembles the manifest this node executes
// inside a harness-owned temporary directory.
//
// The frozen manifest carries the `domain.event` family and its player and
// world event cases, because the controller merges manifest fragments after
// acceptance. The working manifest is therefore a family-scoped selection:
// `Cases` holds exactly the inventory and container cases the committed corpus
// registers, every other family's case list is cleared because `Reconcile`
// requires each family's list to match the cases this selection registers for
// it, and the `domain.event` entry keeps its discovered identity — kind, role,
// versions, source, eventual owner and numeric semantics — while its
// provenance and case list are replaced with this producer's. Registering the
// family's merged provenance and case list in the live registry is
// deliberately left to the manifest merge.
func domainEventInventoryWorkingManifest(t *testing.T, root string) Inventory {
	t.Helper()
	frozen := loadRealManifest(t, root)
	cases := domainEventInventoryCorpusCases(t, root)

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
	sources := make([]SourceSpec, 0, len(domainEventInventoryFamilySources))
	for _, relative := range domainEventInventoryFamilySources {
		hash, err := hashFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("hash provenance source %s: %v", relative, err)
		}
		sources = append(sources, SourceSpec{Path: relative, SHA256: hash})
	}
	registered := false
	for index := range cloned.Families {
		if cloned.Families[index].ID != domainEventInventoryFamily {
			cloned.Families[index].Cases = nil
			continue
		}
		cloned.Families[index].Sources = sources
		cloned.Families[index].Cases = caseIDs
		registered = true
	}
	if !registered {
		t.Fatalf("working manifest has no %s family", domainEventInventoryFamily)
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

// domainEventInventoryCorpusCases reads the frozen corpus and registers one
// case per committed input, with digests proven against the files on disk.
func domainEventInventoryCorpusCases(t *testing.T, root string) []CaseSpec {
	t.Helper()

	dir := filepath.Join(root, filepath.FromSlash(domainEventInventoryCorpusRelDir))
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
		t.Fatalf("walk event inventory corpus directory: %v", err)
	}
	if len(inputs) == 0 {
		t.Fatalf("no event inventory case under %s", domainEventInventoryCorpusRelDir)
	}
	sort.Strings(inputs)

	cases := make([]CaseSpec, 0, len(inputs))
	for _, input := range inputs {
		relative, relErr := filepath.Rel(dir, input)
		if relErr != nil {
			t.Fatalf("relative corpus path: %v", relErr)
		}
		label := strings.TrimSuffix(filepath.ToSlash(relative), ".input.json")
		if label == "" || !domainEventInventoryLabelPattern.MatchString(label) {
			t.Fatalf("corpus case %s has label %q, which is not a lowercase slug with an optional numeric suffix", filepath.ToSlash(relative), label)
		}
		envelope := domainEventInventoryReadEnvelope(t, input)
		if envelope.Consumer != domainEventInventoryConsumer {
			t.Fatalf("corpus case %s names consumer %q, want %q", label, envelope.Consumer, domainEventInventoryConsumer)
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
			ID:           domainEventInventoryCaseID(label),
			Family:       domainEventInventoryFamily,
			Version:      domainEventInventoryVersion,
			Operation:    domainEventInventoryOperation,
			Input:        AssetRef{Path: domainEventInventoryCorpusPath(root, input), SHA256: inputHash},
			InputFormat:  "json",
			Expected:     AssetRef{Path: domainEventInventoryCorpusPath(root, expectedPath), SHA256: expectedHash},
			Checkpoints:  []string{"0"},
			RustConsumer: domainEventInventoryConsumer,
		})
	}
	return cases
}

// domainEventInventoryReadEnvelope reads the provenance envelope of one frozen
// corpus input.
func domainEventInventoryReadEnvelope(t *testing.T, path string) domainEventInventoryInput {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var envelope domainEventInventoryInput
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return envelope
}

// domainEventInventoryCorpusPath renders one absolute corpus path as the
// repository-relative slash path the manifest requires.
func domainEventInventoryCorpusPath(root, absolute string) string {
	relative, err := filepath.Rel(root, absolute)
	if err != nil {
		return absolute
	}
	return filepath.ToSlash(relative)
}

// TestDomainEventInventoryOracleExecutesEveryCase runs the whole case table
// through the real Go protocol DTOs, proves the frozen corpus still matches
// what they produce, and then runs the same cases through the production
// runner so the executed evidence satisfies the completeness rules a published
// trace report does.
func TestDomainEventInventoryOracleExecutesEveryCase(t *testing.T) {
	records := domainEventInventoryExecute(t)
	domainEventInventorySyncCorpus(t, records)

	root := mustRepoRoot(t)
	manifest := domainEventInventoryWorkingManifest(t, root)
	if len(manifest.Cases) != len(records) {
		t.Fatalf("working manifest registers %d cases, want %d (one per executed table row)", len(manifest.Cases), len(records))
	}
	domainEventInventoryAssertCaseSpecs(t, root, manifest)
	domainEventInventoryAssertOutcomesDistinguishCases(t, records)

	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}
	if len(observations) != len(manifest.Cases) {
		t.Fatalf("produced %d observations, want %d", len(observations), len(manifest.Cases))
	}
	for _, obs := range observations {
		c := domainEventInventoryCaseByID(t, manifest, obs.CaseID)
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
		c := domainEventInventoryCaseByID(t, manifest, obs.CaseID)
		if obs.ExpectedDigest != c.Expected.SHA256 {
			t.Fatalf("observation for %s carries expected digest %s, want %s", obs.CaseID, obs.ExpectedDigest, c.Expected.SHA256)
		}
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event inventory evidence failed trace validation: %v", err)
	}
}

// TestDomainEventInventoryOracleCaseIdentitiesAreTheRustDomainConsumer pins
// the manifest identity of every event inventory case: the operation, the
// family version, the family and the consumer the change names for the Rust
// crate that owns these records.
func TestDomainEventInventoryOracleCaseIdentitiesAreTheRustDomainConsumer(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventInventoryWorkingManifest(t, root)

	labels := make(map[string]bool, len(manifest.Cases))
	for _, c := range manifest.Cases {
		if c.Operation != domainEventInventoryOperation {
			t.Fatalf("case %s declares operation %q, want %q", c.ID, c.Operation, domainEventInventoryOperation)
		}
		if c.Version != domainEventInventoryVersion {
			t.Fatalf("case %s declares version %q, want %q", c.ID, c.Version, domainEventInventoryVersion)
		}
		if c.Family != domainEventInventoryFamily {
			t.Fatalf("case %s declares family %q, want %q", c.ID, c.Family, domainEventInventoryFamily)
		}
		if c.RustConsumer != domainEventInventoryConsumer {
			t.Fatalf("case %s declares consumer %q, want %q", c.ID, c.RustConsumer, domainEventInventoryConsumer)
		}
		if c.InputFormat != "json" {
			t.Fatalf("case %s declares input_format %q, want json", c.ID, c.InputFormat)
		}
		prefix := c.Family + "/" + c.Version + "/"
		if !strings.HasPrefix(c.ID, prefix) || len(c.ID) <= len(prefix) {
			t.Fatalf("case %s does not match %s<label>", c.ID, prefix)
		}
		label := strings.TrimPrefix(c.ID, prefix)
		if !domainEventInventoryLabelPattern.MatchString(label) {
			t.Fatalf("case %s label %q is not a lowercase slug with an optional numeric suffix", c.ID, label)
		}
		if labels[label] {
			t.Fatalf("label %q is registered twice", label)
		}
		labels[label] = true

		envelope := domainEventInventoryReadEnvelope(t, filepath.Join(root, filepath.FromSlash(c.Input.Path)))
		if envelope.Consumer != domainEventInventoryConsumer {
			t.Fatalf("case %s was produced for consumer %q, want %q", c.ID, envelope.Consumer, domainEventInventoryConsumer)
		}
		switch envelope.Rule {
		case domainEventInventoryRuleInventory, domainEventInventoryRuleCrafting,
			domainEventInventoryRuleFurnace, domainEventInventoryRuleChest,
			domainEventInventoryRuleClosed:
		default:
			t.Fatalf("case %s names rule %q, which is not an inventory or container publication rule", c.ID, envelope.Rule)
		}
	}
}

// TestDomainEventInventoryOracleWorkingManifestDescribesItself proves the
// working manifest is internally consistent: every case passes the production
// case validation, the family's provenance hashes match disk, and the family's
// case list matches exactly the cases the selection registers.
func TestDomainEventInventoryOracleWorkingManifestDescribesItself(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventInventoryWorkingManifest(t, root)
	domainEventInventoryAssertCaseSpecs(t, root, manifest)

	family, ok := domainEventInventoryFamilySpec(manifest)
	if !ok {
		t.Fatalf("working manifest has no %s family", domainEventInventoryFamily)
	}
	if family.Role != "event" || family.Kind != "domain" {
		t.Fatalf("%s declares kind %q role %q, want domain event", domainEventInventoryFamily, family.Kind, family.Role)
	}
	if family.CurrentVersion != domainEventInventoryVersion {
		t.Fatalf("%s declares version %q, want %q", domainEventInventoryFamily, family.CurrentVersion, domainEventInventoryVersion)
	}
	if len(family.Sources) != len(domainEventInventoryFamilySources) {
		t.Fatalf("%s records %d provenance sources, want %d", domainEventInventoryFamily, len(family.Sources), len(domainEventInventoryFamilySources))
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
		t.Fatalf("%s lists %d cases, want %d", domainEventInventoryFamily, len(family.Cases), len(registered))
	}
	for _, id := range family.Cases {
		if !registered[id] {
			t.Fatalf("%s lists case %s which the selection does not register", domainEventInventoryFamily, id)
		}
	}
}

// TestDomainEventInventoryOracleOutcomesDistinguishAcceptedAndRejected pins
// that the producer is not returning one constant answer, that every rule
// records both an admitted record and a rejection, and that the boundaries the
// Rust event inventory test enumerates are each present in the executed
// evidence.
func TestDomainEventInventoryOracleOutcomesDistinguishAcceptedAndRejected(t *testing.T) {
	records := domainEventInventoryExecute(t)
	domainEventInventoryAssertOutcomesDistinguishCases(t, records)

	tally := make(map[string]*domainEventInventoryRuleTally)
	for _, record := range records {
		entry, ok := tally[record.input.Rule]
		if !ok {
			entry = &domainEventInventoryRuleTally{}
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
		// A rule that records no rejection has no admission boundary to pair.
		if len(entry.rejected) == 0 {
			continue
		}
		if len(entry.accepted) == 0 {
			t.Fatalf("rule %s records %d accepted and %d rejected cases, want both non-zero",
				rule, len(entry.accepted), len(entry.rejected))
		}
	}

	if !domainEventInventoryFieldsContain(tally[domainEventInventoryRuleInventory].accepted, "selected", 8) {
		t.Fatal("no admitted inventory publication carries the last selected hotbar slot")
	}
	if !domainEventInventoryNestedContains(tally[domainEventInventoryRuleInventory].accepted, "hotbar", "count", 64) {
		t.Fatal("no admitted inventory publication carries a full stack in its hotbar")
	}
	if !domainEventInventoryFieldsContain(tally[domainEventInventoryRuleCrafting].accepted, "size", "workbench") {
		t.Fatal("no admitted crafting publication carries the workbench grid")
	}
	if !domainEventInventoryNestedContains(tally[domainEventInventoryRuleCrafting].accepted, "slots", "item", 1) {
		t.Fatal("no admitted crafting publication carries a stone in its grid")
	}
	if !domainEventInventoryFieldsContain(tally[domainEventInventoryRuleFurnace].accepted, "progress_ticks", 199) {
		t.Fatal("no admitted furnace publication carries the progress one tick below the requirement")
	}
	if !domainEventInventoryFieldsContain(tally[domainEventInventoryRuleFurnace].accepted, "burn_ticks", 1600) {
		t.Fatal("no admitted furnace publication carries the burn maximum")
	}
	for _, product := range []int{7, 23, 24, 54} {
		if !domainEventInventoryNestedContains(tally[domainEventInventoryRuleFurnace].accepted, "output", "item", product) {
			t.Fatalf("no admitted furnace publication carries smelting product %d", product)
		}
	}
	if !domainEventInventoryFieldsContain(tally[domainEventInventoryRuleClosed].accepted, "container", map[string]any{
		"kind":       "chest",
		"chunk":      []int{-4, 5},
		"slot":       15,
		"generation": 4,
	}) {
		t.Fatal("no admitted closure carries the chest reference, which the historical FurnaceEnd name still permits")
	}

	for _, boundary := range []struct {
		field string
		value any
		rule  string
	}{
		{"selected", 9, "inventory_state.selected_range"},
		{"size", "unknown", "crafting_state.size"},
		{"progress_ticks", 200, "furnace_state.progress_range"},
		{"burn_ticks", 1601, "furnace_state.burn_range"},
	} {
		if !domainEventInventoryFieldsContain(tally[domainEventInventoryRuleInventory].rejected, boundary.field, boundary.value) &&
			!domainEventInventoryFieldsContain(tally[domainEventInventoryRuleCrafting].rejected, boundary.field, boundary.value) &&
			!domainEventInventoryFieldsContain(tally[domainEventInventoryRuleFurnace].rejected, boundary.field, boundary.value) {
			t.Fatalf("no rejection names %s %v, which is outside the published range", boundary.field, boundary.value)
		}
		if !domainEventInventoryRejectionNamesRule(tally, boundary.rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", boundary.rule)
		}
	}
	// A slot value and a container reference live inside their record's map,
	// so those boundaries are looked up there rather than at the record's top
	// level.
	for _, boundary := range []struct {
		field string
		value any
		rule  string
	}{
		{"count", 1, "crafting_state.personal_residue"},
		{"item", int(core.ItemIDMax), "furnace_state.input"},
		{"item", int(core.ItemIDMax), "chest_state.item_0"},
	} {
		if !domainEventInventoryNestedContains(tally[domainEventInventoryRuleCrafting].rejected, "slots", boundary.field, boundary.value) &&
			!domainEventInventoryNestedContains(tally[domainEventInventoryRuleFurnace].rejected, "input", boundary.field, boundary.value) &&
			!domainEventInventoryNestedContains(tally[domainEventInventoryRuleChest].rejected, "items", boundary.field, boundary.value) {
			t.Fatalf("no rejection names a slot carrying %s %v, which the slot rule rejects", boundary.field, boundary.value)
		}
		if !domainEventInventoryRejectionNamesRule(tally, boundary.rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", boundary.rule)
		}
	}
	for _, rule := range []string{
		"furnace_state.fuel",
		"furnace_state.output",
		"furnace_state.ref_kind",
		"furnace_state.ref_slot",
		"furnace_state.ref_generation",
		"chest_state.ref_kind",
		"chest_state.ref_slot",
		"container_closed.ref_kind",
	} {
		if !domainEventInventoryRejectionNamesRule(tally, rule) {
			t.Fatalf("no rejection names the %s rule, so the boundary is not pinned", rule)
		}
	}
}

// TestDomainEventInventoryOracleRunnerRejectsUnregisteredFamily pins that a
// manifest naming a family this package cannot execute fails the run.
func TestDomainEventInventoryOracleRunnerRejectsUnregisteredFamily(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventInventoryWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Family = "domain.unknown"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "family domain.unknown has no registered Go producer") {
		t.Fatalf("expected an unregistered-family failure, got: %v", err)
	}
}

// TestDomainEventInventoryOracleRunnerRejectsOperationFamilyMismatch pins that
// a case cannot declare one operation and be executed by another.
func TestDomainEventInventoryOracleRunnerRejectsOperationFamilyMismatch(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventInventoryWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Operation = "decode"
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "declares operation") {
		t.Fatalf("expected an operation-family mismatch failure, got: %v", err)
	}
}

// TestDomainEventInventoryOracleRunnerRejectsTamperedInput pins that a case
// whose committed bytes no longer match their recorded digest fails the run
// instead of executing bytes the manifest does not name.
func TestDomainEventInventoryOracleRunnerRejectsTamperedInput(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventInventoryWorkingManifest(t, root)
	for i := range manifest.Cases {
		manifest.Cases[i].Input.SHA256 = "sha256:" + strings.Repeat("0", 64)
	}
	_, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err == nil || !strings.Contains(err.Error(), "does not match disk") {
		t.Fatalf("expected an input digest failure, got: %v", err)
	}
}

// TestDomainEventInventoryOracleReportPublishesAndValidates assembles the
// executed evidence into a report, publishes it through the production atomic
// exporter, reloads it and validates it against the working manifest. This is
// the identity and content check a later Rust acceptance step performs; it
// does not claim any Rust behaviour.
func TestDomainEventInventoryOracleReportPublishesAndValidates(t *testing.T) {
	root := mustRepoRoot(t)
	manifest := domainEventInventoryWorkingManifest(t, root)
	observations, err := RunCases(root, manifest, goOperations, goFamilyOperations)
	if err != nil {
		t.Fatalf("RunCases: %v", err)
	}

	trace, err := traceFromObservations(manifest, observations)
	if err != nil {
		t.Fatalf("traceFromObservations: %v", err)
	}
	if err := ValidateTraceAtRoot(root, trace, manifest); err != nil {
		t.Fatalf("executed event inventory evidence failed trace validation: %v", err)
	}

	workspace, cleanup, err := NewTraceWorkspace(root)
	if err != nil {
		t.Fatalf("NewTraceWorkspace: %v", err)
	}
	defer cleanup()
	target := filepath.Join(workspace, domainEventInventoryCorpusReportName)
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

// TestDomainEventInventoryOracleRejectsMissingProducerTest pins that the
// executed evidence has a real producer behind it, including the topic-named
// entry point the domain plan's filter selects. A corpus whose producer test
// is gone has no independent execution, only frozen files, and a missing topic
// entry point would make the plan filter pass without running anything.
func TestDomainEventInventoryOracleRejectsMissingProducerTest(t *testing.T) {
	root := mustRepoRoot(t)
	path := filepath.Join(root, filepath.FromSlash(domainEventInventoryProducerTestRelPath))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read producer test %s: %v", domainEventInventoryProducerTestRelPath, err)
	}
	for _, name := range []string{domainEventInventoryProducerExecuteName, domainEventInventoryProducerTopicName} {
		declaration := "func " + name + "(t *testing.T) {"
		if !strings.Contains(string(data), declaration) {
			t.Fatalf("%s does not declare %s", domainEventInventoryProducerTestRelPath, declaration)
		}
	}
}

// TestDomainOracle_event_inventory is the topic-named entry point the domain
// plan names for this node. It delegates to the same executed table, so the
// two filters select one source of expected results rather than two.
func TestDomainOracle_event_inventory(t *testing.T) {
	TestDomainEventInventoryOracleExecutesEveryCase(t)
}

// domainEventInventoryRuleTally collects the accepted and rejected normalized
// outcomes one rule records, so a boundary assertion can look for the field
// value it names.
type domainEventInventoryRuleTally struct {
	accepted []map[string]any
	rejected []map[string]any
}

// domainEventInventoryFieldsContain reports whether one recorded outcome set
// holds a record whose named field carries the value a boundary assertion
// names.
func domainEventInventoryFieldsContain(records []map[string]any, field string, want any) bool {
	for _, fields := range records {
		if reflect.DeepEqual(fields[field], want) {
			return true
		}
	}
	return false
}

// domainEventInventoryNestedContains reports whether one recorded outcome set
// holds a record whose named member carries an entry with the named field
// value. A slot array and a single slot value both live inside their record's
// map, so a boundary that names one of their fields looks inside the member
// rather than at the record's top level. The member type is the producer's own
// in-memory rendering, because the executed records have not been decoded back
// from JSON.
func domainEventInventoryNestedContains(records []map[string]any, member, field string, want any) bool {
	for _, fields := range records {
		switch value := fields[member].(type) {
		case map[string]any:
			if reflect.DeepEqual(value[field], want) {
				return true
			}
		case []map[string]any:
			for _, item := range value {
				if reflect.DeepEqual(item[field], want) {
					return true
				}
			}
		}
	}
	return false
}

// domainEventInventoryRejectionNamesRule reports whether any recorded rejection
// names the broken rule a boundary assertion pins.
func domainEventInventoryRejectionNamesRule(tally map[string]*domainEventInventoryRuleTally, rule string) bool {
	for _, entry := range tally {
		for _, fields := range entry.rejected {
			if fields["rule"] == rule {
				return true
			}
		}
	}
	return false
}

// domainEventInventoryAssertCaseSpecs runs the production case validation over
// the whole selection, so a case with a bad path, digest, format or checkpoint
// fails here rather than surfacing later as a confusing coverage failure.
func domainEventInventoryAssertCaseSpecs(t *testing.T, root string, manifest Inventory) {
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

// domainEventInventoryAssertOutcomesDistinguishCases proves the executed
// evidence separates admitted records from rejections instead of publishing
// one constant answer, and that the rejections name a rule the authority
// publishes.
func domainEventInventoryAssertOutcomesDistinguishCases(t *testing.T, records []domainEventInventoryRecord) {
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

// domainEventInventoryFamilySpec resolves one family from a manifest
// selection.
func domainEventInventoryFamilySpec(manifest Inventory) (Family, bool) {
	for _, family := range manifest.Families {
		if family.ID == domainEventInventoryFamily {
			return family, true
		}
	}
	return Family{}, false
}

// domainEventInventoryCaseByID indexes a manifest selection by case identity.
func domainEventInventoryCaseByID(t *testing.T, manifest Inventory, id string) CaseSpec {
	t.Helper()
	for _, c := range manifest.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("case %s is missing from the working manifest", id)
	return CaseSpec{}
}
