package main

// This file is the inventory and container publication packet producer group:
// the five Play server-to-client families that publish one player's item and
// container state (`InventoryState`, `CraftingState`, `FurnaceState`,
// `ChestState` and `ContainerClosed`) each register a decode and an encode
// route through the shared packet-case runner, so every case is executed by
// the real Go codec rather than restated here.
//
// The decode cases are the Go decoder's own bytes: the canonical payloads, the
// validator boundaries, and one structural mutation each. The encode cases
// carry canonical JSON fields and either the reviewed wire the Go encoder has
// to publish or a DTO the outbound validator has to refuse. Every negative
// carries exactly one violation. No case in this group declares a viewer
// session, a recipe or a smelting outcome: routing and settlement stay with
// the authority, so the wire carries only the fixed record.
//
// The slot values move through the real Go decoder without a per-slot rule
// inside the family decoder, so the family validators answer with one shared
// message per family; the category table resolves that message at the
// boundary the Rust consumer publishes for the same bytes, which is the
// item-stack rule's own split (an unregistered item number is `invalid-enum`,
// a count or durability violation is `invalid-value`).

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network/codec"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

const (
	// inventoryStateFamily is the play inventory publication family this group registers.
	inventoryStateFamily = "protocol.server.InventoryState"
	// craftingStateFamily is the play crafting publication family this group registers.
	craftingStateFamily = "protocol.server.CraftingState"
	// furnaceStateFamily is the play furnace publication family this group registers.
	furnaceStateFamily = "protocol.server.FurnaceState"
	// chestStateFamily is the play chest publication family this group registers.
	chestStateFamily = "protocol.server.ChestState"
	// containerClosedFamily is the play container closure family this group registers.
	containerClosedFamily = "protocol.server.ContainerClosed"
	// inventoryPublicationVersion is the protocol version all five families are pinned to.
	inventoryPublicationVersion = "45"
	// inventoryPublicationProducerID is the exporter's producer identity for this
	// group's candidate assets and merged manifest.
	inventoryPublicationProducerID = "runtime-oracle/protocol-inventory-publication"

	// inventoryStateCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	inventoryStateCorpusRelDir = corpusCasesRelDir + "/protocol/InventoryState"
	// craftingStateCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	craftingStateCorpusRelDir = corpusCasesRelDir + "/protocol/CraftingState"
	// furnaceStateCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	furnaceStateCorpusRelDir = corpusCasesRelDir + "/protocol/FurnaceState"
	// chestStateCorpusRelDir is the repository-relative directory holding this
	// family's committed case assets.
	chestStateCorpusRelDir = corpusCasesRelDir + "/protocol/ChestState"
	// containerClosedCorpusRelDir is the repository-relative directory holding
	// this family's committed case assets.
	containerClosedCorpusRelDir = corpusCasesRelDir + "/protocol/ContainerClosed"

	// inventoryStateWireBytes is the fixed stride: selected plus 36 stacks.
	inventoryStateWireBytes = 1 + (core.HotbarSlots+core.BackpackSlots)*5
	// craftingStateWireBytes is the fixed stride: size plus nine grid stacks and output.
	craftingStateWireBytes = 1 + core.CraftingGridSlots*5 + 5
	// furnaceStateWireBytes is the fixed stride: reference plus three stacks and two timers.
	furnaceStateWireBytes = 18 + 3*5 + 1 + 2
	// chestStateWireBytes is the fixed stride: reference plus 27 stacks.
	chestStateWireBytes = 18 + core.ChestSlots*5
	// containerClosedWireBytes is the fixed stride: the reference alone.
	containerClosedWireBytes = 18
)

// Case identities this group registers. Both the manifest entry and the asset
// files use them, so a controller merge can be reviewed case by case.
const (
	inventoryDecodeCaseID      = inventoryStateFamily + "/" + inventoryPublicationVersion + "/decode-valid"
	inventoryEncodeCaseID      = inventoryStateFamily + "/" + inventoryPublicationVersion + "/encode-valid"
	inventorySelectedNineDecID = inventoryStateFamily + "/" + inventoryPublicationVersion + "/decode-selected-nine"
	inventorySelectedNineEncID = inventoryStateFamily + "/" + inventoryPublicationVersion + "/encode-selected-nine"
	inventoryToolDurabilityID  = inventoryStateFamily + "/" + inventoryPublicationVersion + "/decode-tool-at-durability-zero"
	inventoryTrailingID        = inventoryStateFamily + "/" + inventoryPublicationVersion + "/decode-trailing-byte"
	inventoryTruncatedID       = inventoryStateFamily + "/" + inventoryPublicationVersion + "/decode-truncated"
	craftingDecodeCaseID       = craftingStateFamily + "/" + inventoryPublicationVersion + "/decode-valid"
	craftingEncodeCaseID       = craftingStateFamily + "/" + inventoryPublicationVersion + "/encode-valid"
	craftingSizeThreeID        = craftingStateFamily + "/" + inventoryPublicationVersion + "/decode-size-three"
	craftingSizeZeroID         = craftingStateFamily + "/" + inventoryPublicationVersion + "/decode-size-zero"
	craftingSizeFourID         = craftingStateFamily + "/" + inventoryPublicationVersion + "/decode-size-four"
	craftingResidueSlotFourID  = craftingStateFamily + "/" + inventoryPublicationVersion + "/decode-personal-residue-slot-four"
	craftingResidueSlotFourEnc = craftingStateFamily + "/" + inventoryPublicationVersion + "/encode-personal-residue-slot-four"
	craftingTrailingID         = craftingStateFamily + "/" + inventoryPublicationVersion + "/decode-trailing-byte"
	furnaceDecodeCaseID        = furnaceStateFamily + "/" + inventoryPublicationVersion + "/decode-valid"
	furnaceEncodeCaseID        = furnaceStateFamily + "/" + inventoryPublicationVersion + "/encode-valid"
	furnaceProgressAtLimitID   = furnaceStateFamily + "/" + inventoryPublicationVersion + "/decode-progress-at-limit"
	furnaceBurnAboveID         = furnaceStateFamily + "/" + inventoryPublicationVersion + "/decode-burn-above"
	furnaceWrongKindID         = furnaceStateFamily + "/" + inventoryPublicationVersion + "/decode-wrong-kind"
	furnaceZeroGenerationID    = furnaceStateFamily + "/" + inventoryPublicationVersion + "/decode-zero-generation"
	furnaceInvalidFuelID       = furnaceStateFamily + "/" + inventoryPublicationVersion + "/decode-invalid-fuel"
	furnaceInvalidOutputID     = furnaceStateFamily + "/" + inventoryPublicationVersion + "/decode-invalid-output"
	furnaceInvalidFuelEncID    = furnaceStateFamily + "/" + inventoryPublicationVersion + "/encode-invalid-fuel"
	furnaceTrailingID          = furnaceStateFamily + "/" + inventoryPublicationVersion + "/decode-trailing-byte"
	chestDecodeCaseID          = chestStateFamily + "/" + inventoryPublicationVersion + "/decode-valid"
	chestEncodeCaseID          = chestStateFamily + "/" + inventoryPublicationVersion + "/encode-valid"
	chestWrongKindID           = chestStateFamily + "/" + inventoryPublicationVersion + "/decode-wrong-kind"
	chestRefSlotAboveID        = chestStateFamily + "/" + inventoryPublicationVersion + "/decode-ref-slot-above"
	chestInvalidStackID        = chestStateFamily + "/" + inventoryPublicationVersion + "/decode-invalid-stack"
	chestTruncatedID           = chestStateFamily + "/" + inventoryPublicationVersion + "/decode-truncated"
	chestTrailingID            = chestStateFamily + "/" + inventoryPublicationVersion + "/decode-trailing-byte"
	closedDecodeCaseID         = containerClosedFamily + "/" + inventoryPublicationVersion + "/decode-valid"
	closedEncodeCaseID         = containerClosedFamily + "/" + inventoryPublicationVersion + "/encode-valid"
	closedExactNoneID          = containerClosedFamily + "/" + inventoryPublicationVersion + "/decode-exact-none"
	closedForeignDimensionID   = containerClosedFamily + "/" + inventoryPublicationVersion + "/decode-foreign-dimension"
	closedKindTwoID            = containerClosedFamily + "/" + inventoryPublicationVersion + "/decode-kind-two"
)

// inventoryPublicationFamilyKeys pins each family's complete packet key. A
// case names its key instead of trusting its family, so a case registered
// under the wrong family, state or ID fails before the codec runs. All five
// families are server-to-client play packets.
var inventoryPublicationFamilyKeys = map[string]PacketKeySpec{
	inventoryStateFamily:  {Direction: packetDirectionServer, State: packetStatePlay, ID: 10},
	craftingStateFamily:   {Direction: packetDirectionServer, State: packetStatePlay, ID: 21},
	furnaceStateFamily:    {Direction: packetDirectionServer, State: packetStatePlay, ID: 13},
	chestStateFamily:      {Direction: packetDirectionServer, State: packetStatePlay, ID: 15},
	containerClosedFamily: {Direction: packetDirectionServer, State: packetStatePlay, ID: 14},
}

// inventoryPublicationFamilies lists the five families in registry order, with
// the corpus directory that holds each family's assets. The order is the
// review order, not a dispatch table.
func inventoryPublicationFamilies() []struct {
	id     string
	relDir string
} {
	return []struct {
		id     string
		relDir string
	}{
		{inventoryStateFamily, inventoryStateCorpusRelDir},
		{craftingStateFamily, craftingStateCorpusRelDir},
		{furnaceStateFamily, furnaceStateCorpusRelDir},
		{chestStateFamily, chestStateCorpusRelDir},
		{containerClosedFamily, containerClosedCorpusRelDir},
	}
}

// inventoryPublicationU16 renders one little-endian u16 field.
func inventoryPublicationU16(value uint16) []byte {
	return []byte{byte(value), byte(value >> 8)}
}

// inventoryPublicationStackWire renders one fixed 5-byte item stack in the
// Go encoder's field order, so a mutation of one field is a mutation of one
// boundary.
func inventoryPublicationStackWire(item core.ItemID, count uint8, durability uint16) []byte {
	return joinWire(inventoryPublicationU16(uint16(item)), []byte{count}, inventoryPublicationU16(durability))
}

// inventoryPublicationRefWire renders one container reference in the wire
// field order, shared by every reference-carrying family.
func inventoryPublicationRefWire(dimension int32, chunkX, chunkZ int32, kind core.ContainerKind, slot uint8, generation uint32) []byte {
	wire := make([]byte, 0, 18)
	for _, value := range []int32{dimension, chunkX, chunkZ} {
		wire = append(wire, byte(value), byte(value>>8), byte(value>>16), byte(value>>24))
	}
	wire = append(wire, byte(kind), slot)
	return append(wire,
		byte(generation), byte(generation>>8), byte(generation>>16), byte(generation>>24),
	)
}

// inventoryPublicationFurnaceRefWire is the reviewed furnace reference:
// overworld dimension, chunk −1/2, kind 0, physical slot 31 and generation 1.
func inventoryPublicationFurnaceRefWire() []byte {
	return inventoryPublicationRefWire(0, -1, 2, core.ContainerKindFurnace, 31, 1)
}

// inventoryPublicationChestRefWire is the reviewed chest reference: the same
// chunk column with kind 1, physical slot 15 and generation 1.
func inventoryPublicationChestRefWire() []byte {
	return inventoryPublicationRefWire(0, -1, 2, core.ContainerKindChest, 15, 1)
}

// inventoryPublicationCoreRef renders one reviewed reference as the Go domain
// value the production DTO carries.
func inventoryPublicationCoreRef(dimension int32, chunkX, chunkZ int32, kind core.ContainerKind, slot uint8, generation uint32) core.ContainerRef {
	return core.ContainerRef{
		Dimension:  core.DimensionID(dimension),
		Chunk:      core.ChunkPos{X: chunkX, Z: chunkZ},
		Kind:       kind,
		Slot:       slot,
		Generation: generation,
	}
}

// inventoryPublicationFurnaceCoreRef is the reviewed furnace reference.
func inventoryPublicationFurnaceCoreRef() core.ContainerRef {
	return inventoryPublicationCoreRef(0, -1, 2, core.ContainerKindFurnace, 31, 1)
}

// inventoryPublicationChestCoreRef is the reviewed chest reference.
func inventoryPublicationChestCoreRef() core.ContainerRef {
	return inventoryPublicationCoreRef(0, -1, 2, core.ContainerKindChest, 15, 1)
}

// inventoryPublicationEmptyStacks renders one fixed array of empty stacks.
func inventoryPublicationEmptyStacks(count int) []byte {
	wire := make([]byte, 0, count*5)
	for index := 0; index < count; index++ {
		wire = append(wire, inventoryPublicationStackWire(0, 0, 0)...)
	}
	return wire
}

// inventoryPublicationInventoryWire is the reviewed InventoryState literal:
// selected index 8, stone and a full grass stack and one single-count stone
// pickaxe at legal durability in the hotbar, dirt, coal and stone in the
// backpack, and every other slot the exact empty triple.
func inventoryPublicationInventoryWire() []byte {
	wire := []byte{0x08}
	wire = append(wire, inventoryPublicationStackWire(core.ItemStone, 5, 0)...)
	wire = append(wire, inventoryPublicationEmptyStacks(3)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemGrass, 64, 0)...)
	wire = append(wire, inventoryPublicationEmptyStacks(3)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemStonePickaxe, 1, 60)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemDirt, 1, 0)...)
	wire = append(wire, inventoryPublicationEmptyStacks(12)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemCoal, 12, 0)...)
	wire = append(wire, inventoryPublicationEmptyStacks(12)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemStone, 9, 0)...)
	if len(wire) != inventoryStateWireBytes {
		panic("the reviewed inventory literal is not the fixed stride")
	}
	return wire
}

// inventoryPublicationInventoryDTO is the reviewed canonical inventory.
func inventoryPublicationInventoryDTO() core.Inventory {
	var hotbar core.Hotbar
	hotbar.Selected = 8
	hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 5}
	hotbar.Slots[4] = core.ItemStack{Item: core.ItemGrass, Count: 64}
	hotbar.Slots[8] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 60}
	inventory := core.Inventory{Hotbar: hotbar}
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemDirt, Count: 1}
	inventory.Backpack[13] = core.ItemStack{Item: core.ItemCoal, Count: 12}
	inventory.Backpack[core.BackpackSlots-1] = core.ItemStack{Item: core.ItemStone, Count: 9}
	return inventory
}

// inventoryPublicationCraftingWire is the reviewed CraftingState literal:
// personal size, stone and stick in the four usable cells, the five extension
// cells exactly empty, and the derived stone-brick output.
func inventoryPublicationCraftingWire() []byte {
	wire := []byte{0x02}
	wire = append(wire, inventoryPublicationStackWire(core.ItemStone, 2, 0)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemStone, 2, 0)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemStick, 1, 0)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemStick, 1, 0)...)
	wire = append(wire, inventoryPublicationEmptyStacks(5)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemStoneBrick, 1, 0)...)
	if len(wire) != craftingStateWireBytes {
		panic("the reviewed crafting literal is not the fixed stride")
	}
	return wire
}

// inventoryPublicationCraftingDTO is the reviewed canonical personal grid.
func inventoryPublicationCraftingDTO() protocol.CraftingState {
	var slots [core.CraftingGridSlots]core.ItemStack
	slots[0] = core.ItemStack{Item: core.ItemStone, Count: 2}
	slots[1] = core.ItemStack{Item: core.ItemStone, Count: 2}
	slots[2] = core.ItemStack{Item: core.ItemStick, Count: 1}
	slots[3] = core.ItemStack{Item: core.ItemStick, Count: 1}
	return protocol.CraftingState{
		Size:   2,
		Slots:  slots,
		Output: core.ItemStack{Item: core.ItemStoneBrick, Count: 1},
	}
}

// inventoryPublicationCraftingSizeThreeWire is the workbench variant: size 3
// with all nine cells carrying content, which the size domain admits.
func inventoryPublicationCraftingSizeThreeWire() []byte {
	wire := []byte{0x03}
	for index := 0; index < core.CraftingGridSlots; index++ {
		wire = append(wire, inventoryPublicationStackWire(core.ItemStone, 1, 0)...)
	}
	wire = append(wire, inventoryPublicationStackWire(core.ItemStone, 1, 0)...)
	if len(wire) != craftingStateWireBytes {
		panic("the workbench crafting literal is not the fixed stride")
	}
	return wire
}

// inventoryPublicationFurnaceWire is the reviewed FurnaceState literal: the
// furnace reference, raw beef input, coal fuel, cooked beef output, progress
// 199 and burn 1600.
func inventoryPublicationFurnaceWire() []byte {
	wire := append([]byte(nil), inventoryPublicationFurnaceRefWire()...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemRawBeef, 1, 0)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemCoal, 1, 0)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemCookedBeef, 1, 0)...)
	wire = append(wire, byte(core.FurnaceSmeltTicks-1))
	wire = append(wire, inventoryPublicationU16(core.FurnaceBurnTicks)...)
	if len(wire) != furnaceStateWireBytes {
		panic("the reviewed furnace literal is not the fixed stride")
	}
	return wire
}

// inventoryPublicationFurnaceDTO is the reviewed canonical furnace mirror.
func inventoryPublicationFurnaceDTO() protocol.FurnaceState {
	return protocol.FurnaceState{
		Furnace:       inventoryPublicationFurnaceCoreRef(),
		Input:         core.ItemStack{Item: core.ItemRawBeef, Count: 1},
		Fuel:          core.ItemStack{Item: core.ItemCoal, Count: 1},
		Output:        core.ItemStack{Item: core.ItemCookedBeef, Count: 1},
		ProgressTicks: core.FurnaceSmeltTicks - 1,
		BurnTicks:     core.FurnaceBurnTicks,
	}
}

// inventoryPublicationChestWire is the reviewed ChestState literal: the chest
// reference with stone, a single-count stone pickaxe at legal durability and
// dirt, and every other slot the exact empty triple.
func inventoryPublicationChestWire() []byte {
	wire := append([]byte(nil), inventoryPublicationChestRefWire()...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemStone, 5, 0)...)
	wire = append(wire, inventoryPublicationEmptyStacks(14)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemStonePickaxe, 1, 60)...)
	wire = append(wire, inventoryPublicationEmptyStacks(10)...)
	wire = append(wire, inventoryPublicationStackWire(core.ItemDirt, 1, 0)...)
	if len(wire) != chestStateWireBytes {
		panic("the reviewed chest literal is not the fixed stride")
	}
	return wire
}

// inventoryPublicationChestDTO is the reviewed canonical chest mirror.
func inventoryPublicationChestDTO() protocol.ChestState {
	var items [core.ChestSlots]core.ItemStack
	items[0] = core.ItemStack{Item: core.ItemStone, Count: 5}
	items[15] = core.ItemStack{Item: core.ItemStonePickaxe, Count: 1, Durability: 60}
	items[core.ChestSlots-1] = core.ItemStack{Item: core.ItemDirt, Count: 1}
	return protocol.ChestState{Chest: inventoryPublicationChestCoreRef(), Items: items}
}

// inventoryPublicationWithByte replaces one byte of a payload at offset.
func inventoryPublicationWithByte(source []byte, offset int, value uint8) []byte {
	wire := append([]byte(nil), source...)
	wire[offset] = value
	return wire
}

// inventoryPublicationWithU16 replaces one little-endian u16 field of a
// payload at offset.
func inventoryPublicationWithU16(source []byte, offset int, value uint16) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[offset:offset+2], inventoryPublicationU16(value))
	return wire
}

// inventoryPublicationWithRef replaces the leading 18-byte reference of a
// reference-carrying payload, leaving every other field in place.
func inventoryPublicationWithRef(source []byte, reference []byte) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[:18], reference)
	return wire
}

// inventoryPublicationWithStack replaces one 5-byte stack of a payload at the
// given slot offset.
func inventoryPublicationWithStack(source []byte, offset int, stack []byte) []byte {
	wire := append([]byte(nil), source...)
	copy(wire[offset:offset+5], stack)
	return wire
}

// inventoryPublicationStackFields renders one slot value. All three fields
// are published, because the empty slot is the zero triple and a missing
// field would not prove the authority published exactly that.
func inventoryPublicationStackFields(stack core.ItemStack) map[string]any {
	return map[string]any{
		"item":       int(stack.Item),
		"count":      int(stack.Count),
		"durability": int(stack.Durability),
	}
}

// inventoryPublicationStacksFields renders one ordered stack array.
func inventoryPublicationStacksFields(stacks []core.ItemStack) []map[string]any {
	rendered := make([]map[string]any, 0, len(stacks))
	for _, stack := range stacks {
		rendered = append(rendered, inventoryPublicationStackFields(stack))
	}
	return rendered
}

// inventoryPublicationRefFields renders the nested container reference object
// every reference-carrying family publishes: the raw wire integers, so a
// negative chunk coordinate and a dimension the authority refuses both
// survive the round trip.
func inventoryPublicationRefFields(ref core.ContainerRef) map[string]any {
	return map[string]any{
		"dimension":  int32(ref.Dimension),
		"chunk_x":    ref.Chunk.X,
		"chunk_z":    ref.Chunk.Z,
		"kind":       uint8(ref.Kind),
		"slot":       ref.Slot,
		"generation": ref.Generation,
	}
}

// inventoryPublicationInventoryFields renders the semantic fields one
// inventory state publishes.
func inventoryPublicationInventoryFields(inventory core.Inventory) map[string]any {
	return map[string]any{
		"selected": inventory.Hotbar.Selected,
		"hotbar":   inventoryPublicationStacksFields(inventory.Hotbar.Slots[:]),
		"backpack": inventoryPublicationStacksFields(inventory.Backpack[:]),
	}
}

// inventoryPublicationCraftingFields renders the semantic fields one crafting
// state publishes.
func inventoryPublicationCraftingFields(state protocol.CraftingState) map[string]any {
	return map[string]any{
		"size":   state.Size,
		"slots":  inventoryPublicationStacksFields(state.Slots[:]),
		"output": inventoryPublicationStackFields(state.Output),
	}
}

// inventoryPublicationFurnaceFields renders the semantic fields one furnace
// state publishes.
func inventoryPublicationFurnaceFields(state protocol.FurnaceState) map[string]any {
	return map[string]any{
		"furnace":        inventoryPublicationRefFields(state.Furnace),
		"input":          inventoryPublicationStackFields(state.Input),
		"fuel":           inventoryPublicationStackFields(state.Fuel),
		"output":         inventoryPublicationStackFields(state.Output),
		"progress_ticks": state.ProgressTicks,
		"burn_ticks":     state.BurnTicks,
	}
}

// inventoryPublicationChestFields renders the semantic fields one chest state
// publishes.
func inventoryPublicationChestFields(state protocol.ChestState) map[string]any {
	return map[string]any{
		"chest": inventoryPublicationRefFields(state.Chest),
		"items": inventoryPublicationStacksFields(state.Items[:]),
	}
}

// inventoryPublicationClosedFields renders the semantic fields one container
// closure publishes.
func inventoryPublicationClosedFields(closed protocol.ContainerClosed) map[string]any {
	return map[string]any{
		"container": inventoryPublicationRefFields(closed.Container),
	}
}

// inventoryPublicationRejectionCategory resolves the language-neutral
// rejection category for one real Go codec failure.
//
// The mapping is a closed table over the wire conditions the Go decoder and
// the outbound validators name, never over a sentinel identity, and a failure
// with no mapping is a hard error. The primitive answer precedes the
// validator message, because the decoder rejects a short or over-long payload
// while reading and only then hands the record to `Validate`.
//
// The reference gates answer the kind first, so the kind messages are the
// invalid-enum boundaries while the dimension, slot and generation messages
// are the invalid-value boundaries. The two stack-rule messages resolve at the
// item-stack rule's own split, which is what the Rust consumer publishes for
// the same bytes: the chest message resolves to `invalid-enum` because the one
// stack-rule case that family registers carries an unregistered item number,
// while the inventory message resolves to `invalid-value` because that
// family's two stack-rule cases are the selected index and a durability
// violation. No crafting stack case is registered, so that family's slot
// messages are not part of the reviewed table.
func inventoryPublicationRejectionCategory(err error) (string, bool) {
	message := err.Error()
	switch {
	case strings.Contains(message, "short input"):
		return "truncated", true
	case strings.Contains(message, "trailing bytes"):
		return "trailing", true
	case strings.Contains(message, "unknown container kind"),
		strings.Contains(message, "furnace ref kind is not furnace"),
		strings.Contains(message, "chest ref kind is not chest"),
		strings.Contains(message, "chest slot holds an invalid item stack"):
		return "invalid-enum", true
	case strings.Contains(message, "furnace dimension is not overworld"),
		strings.Contains(message, "chest dimension is not overworld"),
		strings.Contains(message, "furnace slot is outside 0..31"),
		strings.Contains(message, "chest slot is outside 0..15"),
		strings.Contains(message, "furnace generation is zero"),
		strings.Contains(message, "chest generation is zero"),
		strings.Contains(message, "furnace timers are outside their fixed ranges"),
		strings.Contains(message, "furnace slot holds an item it cannot contain"),
		strings.Contains(message, "inventory state is not a valid fixed inventory"),
		strings.Contains(message, "crafting state size is not 2 or 3"),
		strings.Contains(message, "personal crafting state has residue beyond the 2x2 grid"):
		return "invalid-value", true
	}
	return "", false
}

// inventoryPublicationPacketKey resolves one packet key to the Go state and
// numeric ID the codec dispatches on, and reports whether the packet travels
// server-to-client.
func inventoryPublicationPacketKey(c CaseSpec) (protocol.State, uint32, error) {
	if c.PacketKey == nil {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names no packet key", c.ID)
	}
	registered, owned := inventoryPublicationFamilyKeys[c.Family]
	if !owned {
		return 0, 0, fmt.Errorf("runtime-oracle: case %s names family %q, which no inventory publication producer owns", c.ID, c.Family)
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

// newInventoryPublicationCodec builds the production codec one producer call
// uses.
//
// The codec owns the snapshot compression context, which these families never
// touch; it is still the single encoder/decoder entry point, so a producer
// cannot bypass it with a hand-written payload writer. The caller closes it.
func newInventoryPublicationCodec() (*codec.Codec, error) {
	wireCodec, err := codec.NewCodec()
	if err != nil {
		return nil, fmt.Errorf("runtime-oracle: create codec: %w", err)
	}
	return wireCodec, nil
}

// inventoryPublicationFieldsForPacket renders the semantic fields of one
// decoded publication packet, dispatching on the concrete DTO the production
// decoder returned.
func inventoryPublicationFieldsForPacket(c CaseSpec, packet any) (map[string]any, error) {
	switch message := packet.(type) {
	case protocol.InventoryState:
		return inventoryPublicationInventoryFields(message.Inventory), nil
	case protocol.CraftingState:
		return inventoryPublicationCraftingFields(message), nil
	case protocol.FurnaceState:
		return inventoryPublicationFurnaceFields(message), nil
	case protocol.ChestState:
		return inventoryPublicationChestFields(message), nil
	case protocol.ContainerClosed:
		return inventoryPublicationClosedFields(message), nil
	default:
		return nil, fmt.Errorf("runtime-oracle: case %s decoded %T, which no field renderer owns", c.ID, packet)
	}
}

// runInventoryPublicationDecode executes one publication decode case through
// the real Go decoder named by the case's own packet key.
//
// The producer hands the payload and the key to `DecodeServer` and classifies
// the failure it returns. It never reimplements the length, stack or reference
// rules, so the recorded outcome is whatever the production codec decides
// about these exact bytes.
func runInventoryPublicationDecode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := inventoryPublicationPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newInventoryPublicationCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	packet, err := wireCodec.DecodeServer(state, packetID, input)
	if err != nil {
		category, classified := inventoryPublicationRejectionCategory(err)
		if !classified {
			return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s: unclassified decode rejection: %w", c.ID, err)
		}
		return Outcome{Kind: "error", Category: category}, nil, nil
	}
	fields, err := inventoryPublicationFieldsForPacket(c, packet)
	if err != nil {
		return Outcome{}, nil, err
	}
	return Outcome{Kind: "ok", Category: packetOutcomeCategory, Fields: fields}, nil, nil
}

// inventoryPublicationStackRequest is the canonical JSON one slot value
// carries. Every field is a plain JSON integer.
type inventoryPublicationStackRequest struct {
	Item       int `json:"item"`
	Count      int `json:"count"`
	Durability int `json:"durability"`
}

// core renders one JSON slot value as the Go domain value the DTO carries.
func (request inventoryPublicationStackRequest) core() core.ItemStack {
	return core.ItemStack{
		Item:       core.ItemID(request.Item),
		Count:      uint8(request.Count),
		Durability: uint16(request.Durability),
	}
}

// inventoryPublicationRefRequest is the canonical JSON container reference one
// reference-carrying encode case carries.
type inventoryPublicationRefRequest struct {
	Dimension  int32  `json:"dimension"`
	ChunkX     int32  `json:"chunk_x"`
	ChunkZ     int32  `json:"chunk_z"`
	Kind       uint8  `json:"kind"`
	Slot       uint8  `json:"slot"`
	Generation uint32 `json:"generation"`
}

// core renders one JSON reference as the Go domain value the DTO carries.
func (request inventoryPublicationRefRequest) core() core.ContainerRef {
	return inventoryPublicationCoreRef(request.Dimension, request.ChunkX, request.ChunkZ,
		core.ContainerKind(request.Kind), request.Slot, request.Generation)
}

// inventoryPublicationInventoryRequest is the canonical JSON field input one
// inventory encode case carries.
type inventoryPublicationInventoryRequest struct {
	Selected uint8                              `json:"selected"`
	Hotbar   []inventoryPublicationStackRequest `json:"hotbar"`
	Backpack []inventoryPublicationStackRequest `json:"backpack"`
}

// core renders the request as the Go inventory the DTO carries.
func (request inventoryPublicationInventoryRequest) core() (core.Inventory, error) {
	if len(request.Hotbar) != core.HotbarSlots || len(request.Backpack) != core.BackpackSlots {
		return core.Inventory{}, fmt.Errorf("runtime-oracle: inventory request carries %d hotbar and %d backpack slots", len(request.Hotbar), len(request.Backpack))
	}
	var hotbar core.Hotbar
	hotbar.Selected = request.Selected
	for index, stack := range request.Hotbar {
		hotbar.Slots[index] = stack.core()
	}
	inventory := core.Inventory{Hotbar: hotbar}
	for index, stack := range request.Backpack {
		inventory.Backpack[index] = stack.core()
	}
	return inventory, nil
}

// inventoryPublicationCraftingRequest is the canonical JSON field input one
// crafting encode case carries.
type inventoryPublicationCraftingRequest struct {
	Size   uint8                              `json:"size"`
	Slots  []inventoryPublicationStackRequest `json:"slots"`
	Output inventoryPublicationStackRequest   `json:"output"`
}

// core renders the request as the Go crafting state the DTO carries.
func (request inventoryPublicationCraftingRequest) core() (protocol.CraftingState, error) {
	if len(request.Slots) != core.CraftingGridSlots {
		return protocol.CraftingState{}, fmt.Errorf("runtime-oracle: crafting request carries %d grid slots", len(request.Slots))
	}
	var slots [core.CraftingGridSlots]core.ItemStack
	for index, stack := range request.Slots {
		slots[index] = stack.core()
	}
	return protocol.CraftingState{Size: request.Size, Slots: slots, Output: request.Output.core()}, nil
}

// inventoryPublicationFurnaceRequest is the canonical JSON field input one
// furnace encode case carries.
type inventoryPublicationFurnaceRequest struct {
	Furnace       inventoryPublicationRefRequest   `json:"furnace"`
	Input         inventoryPublicationStackRequest `json:"input"`
	Fuel          inventoryPublicationStackRequest `json:"fuel"`
	Output        inventoryPublicationStackRequest `json:"output"`
	ProgressTicks uint8                            `json:"progress_ticks"`
	BurnTicks     uint16                           `json:"burn_ticks"`
}

// core renders the request as the Go furnace state the DTO carries.
func (request inventoryPublicationFurnaceRequest) core() protocol.FurnaceState {
	return protocol.FurnaceState{
		Furnace:       request.Furnace.core(),
		Input:         request.Input.core(),
		Fuel:          request.Fuel.core(),
		Output:        request.Output.core(),
		ProgressTicks: request.ProgressTicks,
		BurnTicks:     request.BurnTicks,
	}
}

// inventoryPublicationChestRequest is the canonical JSON field input one chest
// encode case carries.
type inventoryPublicationChestRequest struct {
	Chest inventoryPublicationRefRequest     `json:"chest"`
	Items []inventoryPublicationStackRequest `json:"items"`
}

// core renders the request as the Go chest state the DTO carries.
func (request inventoryPublicationChestRequest) core() (protocol.ChestState, error) {
	if len(request.Items) != core.ChestSlots {
		return protocol.ChestState{}, fmt.Errorf("runtime-oracle: chest request carries %d slots", len(request.Items))
	}
	var items [core.ChestSlots]core.ItemStack
	for index, stack := range request.Items {
		items[index] = stack.core()
	}
	return protocol.ChestState{Chest: request.Chest.core(), Items: items}, nil
}

// inventoryPublicationClosedRequest is the canonical JSON field input one
// container closure encode case carries.
type inventoryPublicationClosedRequest struct {
	Container inventoryPublicationRefRequest `json:"container"`
}

// core renders the request as the Go closure the DTO carries.
func (request inventoryPublicationClosedRequest) core() protocol.ContainerClosed {
	return protocol.ContainerClosed{Container: request.Container.core()}
}

// runInventoryPublicationEncode executes one publication encode case through
// the real Go encoder named by the case's own packet key and reads the result
// back.
//
// The producer builds the DTO from the typed fields and calls the production
// encoder, which runs the outbound validation first, so a negative encode case
// is refused by the same validator the decode path applies. The read-back
// guards the other direction, because bytes the production decoder rejects
// must never be recorded as evidence.
func runInventoryPublicationEncode(c CaseSpec, input []byte) (Outcome, []byte, error) {
	state, packetID, err := inventoryPublicationPacketKey(c)
	if err != nil {
		return Outcome{}, nil, err
	}
	wireCodec, err := newInventoryPublicationCodec()
	if err != nil {
		return Outcome{}, nil, err
	}
	defer func() { _ = wireCodec.Close() }()

	var packet protocol.ServerPacket
	switch c.Family {
	case inventoryStateFamily:
		var request inventoryPublicationInventoryRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		inventory, err := request.core()
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = protocol.InventoryState{Inventory: inventory}
	case craftingStateFamily:
		var request inventoryPublicationCraftingRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		state, err := request.core()
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = state
	case furnaceStateFamily:
		var request inventoryPublicationFurnaceRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	case chestStateFamily:
		var request inventoryPublicationChestRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		state, err := request.core()
		if err != nil {
			return Outcome{}, nil, err
		}
		packet = state
	case containerClosedFamily:
		var request inventoryPublicationClosedRequest
		if err := decodeEncodeRequest(c, input, &request); err != nil {
			return Outcome{}, nil, err
		}
		packet = request.core()
	default:
		return Outcome{}, nil, fmt.Errorf("runtime-oracle: case %s names family %q, which no encode producer owns", c.ID, c.Family)
	}

	encodedID, payload, err := wireCodec.EncodeServer(state, packet)
	if err != nil {
		category, classified := inventoryPublicationRejectionCategory(err)
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
	fields, err := inventoryPublicationFieldsForPacket(c, decoded)
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

// inventoryPublicationCorpusRoutes is the closed route map the five families
// execute.
func inventoryPublicationCorpusRoutes() map[ConsumerRoute]GoOperation {
	routes := map[ConsumerRoute]GoOperation{}
	for _, family := range inventoryPublicationFamilies() {
		routes[ConsumerRoute{FamilyID: family.id, Version: inventoryPublicationVersion, Operation: "decode"}] = runInventoryPublicationDecode
		routes[ConsumerRoute{FamilyID: family.id, Version: inventoryPublicationVersion, Operation: "encode"}] = runInventoryPublicationEncode
	}
	return routes
}

// inventoryPublicationCaseDefinition declares one case from literals before
// any producer runs, so the expectation is the review contract rather than a
// producer result.
type inventoryPublicationCaseDefinition struct {
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

// inventoryPublicationSlotRequest renders one slot value as its canonical
// JSON request fields.
func inventoryPublicationSlotRequest(item int, count int, durability int) inventoryPublicationStackRequest {
	return inventoryPublicationStackRequest{Item: item, Count: count, Durability: durability}
}

// inventoryPublicationInventoryRequestFields renders the canonical JSON
// request the valid inventory cases carry, which is the reviewed wire's own
// field set.
func inventoryPublicationInventoryRequestFields() inventoryPublicationInventoryRequest {
	hotbar := make([]inventoryPublicationStackRequest, core.HotbarSlots)
	hotbar[0] = inventoryPublicationSlotRequest(int(core.ItemStone), 5, 0)
	hotbar[4] = inventoryPublicationSlotRequest(int(core.ItemGrass), 64, 0)
	hotbar[8] = inventoryPublicationSlotRequest(int(core.ItemStonePickaxe), 1, 60)
	backpack := make([]inventoryPublicationStackRequest, core.BackpackSlots)
	backpack[0] = inventoryPublicationSlotRequest(int(core.ItemDirt), 1, 0)
	backpack[13] = inventoryPublicationSlotRequest(int(core.ItemCoal), 12, 0)
	backpack[core.BackpackSlots-1] = inventoryPublicationSlotRequest(int(core.ItemStone), 9, 0)
	return inventoryPublicationInventoryRequest{Selected: 8, Hotbar: hotbar, Backpack: backpack}
}

// inventoryPublicationCraftingRequestFields renders the canonical JSON
// request the valid crafting cases carry.
func inventoryPublicationCraftingRequestFields() inventoryPublicationCraftingRequest {
	slots := make([]inventoryPublicationStackRequest, core.CraftingGridSlots)
	slots[0] = inventoryPublicationSlotRequest(int(core.ItemStone), 2, 0)
	slots[1] = inventoryPublicationSlotRequest(int(core.ItemStone), 2, 0)
	slots[2] = inventoryPublicationSlotRequest(int(core.ItemStick), 1, 0)
	slots[3] = inventoryPublicationSlotRequest(int(core.ItemStick), 1, 0)
	return inventoryPublicationCraftingRequest{
		Size:   2,
		Slots:  slots,
		Output: inventoryPublicationSlotRequest(int(core.ItemStoneBrick), 1, 0),
	}
}

// inventoryPublicationFurnaceRequestFields renders the canonical JSON request
// the valid furnace cases carry.
func inventoryPublicationFurnaceRequestFields() inventoryPublicationFurnaceRequest {
	return inventoryPublicationFurnaceRequest{
		Furnace:       inventoryPublicationFurnaceRefRequestFields(),
		Input:         inventoryPublicationSlotRequest(int(core.ItemRawBeef), 1, 0),
		Fuel:          inventoryPublicationSlotRequest(int(core.ItemCoal), 1, 0),
		Output:        inventoryPublicationSlotRequest(int(core.ItemCookedBeef), 1, 0),
		ProgressTicks: core.FurnaceSmeltTicks - 1,
		BurnTicks:     core.FurnaceBurnTicks,
	}
}

// inventoryPublicationChestRequestFields renders the canonical JSON request
// the valid chest cases carry.
func inventoryPublicationChestRequestFields() inventoryPublicationChestRequest {
	items := make([]inventoryPublicationStackRequest, core.ChestSlots)
	items[0] = inventoryPublicationSlotRequest(int(core.ItemStone), 5, 0)
	items[15] = inventoryPublicationSlotRequest(int(core.ItemStonePickaxe), 1, 60)
	items[core.ChestSlots-1] = inventoryPublicationSlotRequest(int(core.ItemDirt), 1, 0)
	return inventoryPublicationChestRequest{Chest: inventoryPublicationChestRefRequestFields(), Items: items}
}

// inventoryPublicationFurnaceRefRequestFields is the reviewed furnace
// reference JSON fields.
func inventoryPublicationFurnaceRefRequestFields() inventoryPublicationRefRequest {
	return inventoryPublicationRefRequest{Dimension: 0, ChunkX: -1, ChunkZ: 2, Kind: uint8(core.ContainerKindFurnace), Slot: 31, Generation: 1}
}

// inventoryPublicationChestRefRequestFields is the reviewed chest reference
// JSON fields.
func inventoryPublicationChestRefRequestFields() inventoryPublicationRefRequest {
	return inventoryPublicationRefRequest{Dimension: 0, ChunkX: -1, ChunkZ: 2, Kind: uint8(core.ContainerKindChest), Slot: 15, Generation: 1}
}

// inventoryPublicationCaseDefinitions is this group's reviewed case table.
//
// Every case carries the complete packet key and one operation. The valid
// cases use the canonical payloads the Go encoder produces for these fields,
// including the workbench crafting variant; the malformed decode cases mutate
// that payload at one boundary each, and the invalid encode cases build a DTO
// the production validator refuses, so each rejection names the boundary that
// owns it. No case combines an unknown reference kind with a nonzero
// dimension, because the Go reference gate answers the kind first while the
// Rust conversion answers the dimension first and the two would publish
// different categories for the same bytes.
func inventoryPublicationCaseDefinitions() []inventoryPublicationCaseDefinition {
	validInventory := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   inventoryPublicationInventoryFields(inventoryPublicationInventoryDTO()),
	}
	validCrafting := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   inventoryPublicationCraftingFields(inventoryPublicationCraftingDTO()),
	}
	validCraftingThree := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields: map[string]any{
			"size": uint8(3),
			"slots": inventoryPublicationStacksFields(
				inventoryPublicationCraftingSizeThreeSlots(),
			),
			"output": inventoryPublicationStackFields(
				inventoryPublicationCraftingSizeThreeDTO().Output,
			),
		},
	}
	validFurnace := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   inventoryPublicationFurnaceFields(inventoryPublicationFurnaceDTO()),
	}
	validChest := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   inventoryPublicationChestFields(inventoryPublicationChestDTO()),
	}
	validClosedFurnace := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   inventoryPublicationClosedFields(protocol.ContainerClosed{Container: inventoryPublicationFurnaceCoreRef()}),
	}
	validClosedChest := Outcome{
		Kind:     "ok",
		Category: packetOutcomeCategory,
		Fields:   inventoryPublicationClosedFields(protocol.ContainerClosed{Container: inventoryPublicationChestCoreRef()}),
	}
	invalidValue := Outcome{Kind: "error", Category: "invalid-value"}
	invalidEnum := Outcome{Kind: "error", Category: "invalid-enum"}
	truncated := Outcome{Kind: "error", Category: "truncated"}
	trailing := Outcome{Kind: "error", Category: "trailing"}

	inventoryWire := inventoryPublicationInventoryWire()
	craftingWire := inventoryPublicationCraftingWire()
	craftingThreeWire := inventoryPublicationCraftingSizeThreeWire()
	furnaceWire := inventoryPublicationFurnaceWire()
	chestWire := inventoryPublicationChestWire()
	furnaceRefWire := inventoryPublicationFurnaceRefWire()
	chestRefWire := inventoryPublicationChestRefWire()

	// The tool slot is hotbar 8 at payload offset 41, so its durability field
	// is the u16 at offset 44.
	const toolSlotOffset = 41
	const toolDurabilityOffset = 44
	// The fuel slot is the second stack after the reference at offset 23, the
	// output slot the third at offset 28.
	const furnaceFuelOffset = 23
	const furnaceOutputOffset = 28
	// The coal and dirt slot offsets inside the backpack region.
	const backpackCoalOffset = 1 + (core.HotbarSlots+13)*5

	definitions := make([]inventoryPublicationCaseDefinition, 0, 37)
	definitions = append(definitions,
		inventoryPublicationCaseDefinition{
			id:     inventoryDecodeCaseID,
			family: inventoryStateFamily,
			relDir: inventoryStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), inventoryWire...),
			expect: validInventory,
		},
		inventoryPublicationCaseDefinition{
			id:      inventoryEncodeCaseID,
			family:  inventoryStateFamily,
			relDir:  inventoryStateCorpusRelDir,
			op:      "encode",
			request: inventoryPublicationInventoryRequestFields(),
			wire:    append([]byte(nil), inventoryWire...),
			expect:  validInventory,
		},
		inventoryPublicationCaseDefinition{
			id:     inventorySelectedNineDecID,
			family: inventoryStateFamily,
			relDir: inventoryStateCorpusRelDir,
			op:     "decode",
			input:  inventoryPublicationWithByte(inventoryWire, 0, 9),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:      inventorySelectedNineEncID,
			family:  inventoryStateFamily,
			relDir:  inventoryStateCorpusRelDir,
			op:      "encode",
			request: inventoryPublicationSelectedNineRequest(),
			expect:  invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:     inventoryToolDurabilityID,
			family: inventoryStateFamily,
			relDir: inventoryStateCorpusRelDir,
			op:     "decode",
			input:  inventoryPublicationWithU16(inventoryWire, toolDurabilityOffset, 0),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:     inventoryTrailingID,
			family: inventoryStateFamily,
			relDir: inventoryStateCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), inventoryWire...), 0x00),
			expect: trailing,
		},
		inventoryPublicationCaseDefinition{
			// The cut lands inside the backpack region, so the checked
			// indexed reads report the same short-input boundary.
			id:     inventoryTruncatedID,
			family: inventoryStateFamily,
			relDir: inventoryStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), inventoryWire[:backpackCoalOffset+5]...),
			expect: truncated,
		},
		inventoryPublicationCaseDefinition{
			id:     craftingDecodeCaseID,
			family: craftingStateFamily,
			relDir: craftingStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), craftingWire...),
			expect: validCrafting,
		},
		inventoryPublicationCaseDefinition{
			id:      craftingEncodeCaseID,
			family:  craftingStateFamily,
			relDir:  craftingStateCorpusRelDir,
			op:      "encode",
			request: inventoryPublicationCraftingRequestFields(),
			wire:    append([]byte(nil), craftingWire...),
			expect:  validCrafting,
		},
		inventoryPublicationCaseDefinition{
			id:     craftingSizeThreeID,
			family: craftingStateFamily,
			relDir: craftingStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), craftingThreeWire...),
			expect: validCraftingThree,
		},
		inventoryPublicationCaseDefinition{
			id:     craftingSizeZeroID,
			family: craftingStateFamily,
			relDir: craftingStateCorpusRelDir,
			op:     "decode",
			input:  inventoryPublicationWithByte(craftingWire, 0, 0),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:     craftingSizeFourID,
			family: craftingStateFamily,
			relDir: craftingStateCorpusRelDir,
			op:     "decode",
			input:  inventoryPublicationWithByte(craftingWire, 0, 4),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:     craftingResidueSlotFourID,
			family: craftingStateFamily,
			relDir: craftingStateCorpusRelDir,
			op:     "decode",
			input: inventoryPublicationWithStack(craftingWire, 1+4*5,
				inventoryPublicationStackWire(core.ItemStone, 1, 0)),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:      craftingResidueSlotFourEnc,
			family:  craftingStateFamily,
			relDir:  craftingStateCorpusRelDir,
			op:      "encode",
			request: inventoryPublicationCraftingResidueRequest(),
			expect:  invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:     craftingTrailingID,
			family: craftingStateFamily,
			relDir: craftingStateCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), craftingWire...), 0x00),
			expect: trailing,
		},
		inventoryPublicationCaseDefinition{
			id:     furnaceDecodeCaseID,
			family: furnaceStateFamily,
			relDir: furnaceStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), furnaceWire...),
			expect: validFurnace,
		},
		inventoryPublicationCaseDefinition{
			id:      furnaceEncodeCaseID,
			family:  furnaceStateFamily,
			relDir:  furnaceStateCorpusRelDir,
			op:      "encode",
			request: inventoryPublicationFurnaceRequestFields(),
			wire:    append([]byte(nil), furnaceWire...),
			expect:  validFurnace,
		},
		inventoryPublicationCaseDefinition{
			id:     furnaceProgressAtLimitID,
			family: furnaceStateFamily,
			relDir: furnaceStateCorpusRelDir,
			op:     "decode",
			input: inventoryPublicationWithByte(furnaceWire, 33,
				byte(core.FurnaceSmeltTicks)),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:     furnaceBurnAboveID,
			family: furnaceStateFamily,
			relDir: furnaceStateCorpusRelDir,
			op:     "decode",
			input: inventoryPublicationWithU16(furnaceWire, 34,
				core.FurnaceBurnTicks+1),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			// A valid chest reference beside the furnace family is the single
			// kind violation; the dimension stays the overworld.
			id:     furnaceWrongKindID,
			family: furnaceStateFamily,
			relDir: furnaceStateCorpusRelDir,
			op:     "decode",
			input:  inventoryPublicationWithRef(furnaceWire, chestRefWire),
			expect: invalidEnum,
		},
		inventoryPublicationCaseDefinition{
			id:     furnaceZeroGenerationID,
			family: furnaceStateFamily,
			relDir: furnaceStateCorpusRelDir,
			op:     "decode",
			input: inventoryPublicationWithRef(furnaceWire,
				inventoryPublicationRefWire(0, -1, 2, core.ContainerKindFurnace, 31, 0)),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			// The fuel slot holds a registered item that is not coal, so the
			// whitelist is the single violation.
			id:     furnaceInvalidFuelID,
			family: furnaceStateFamily,
			relDir: furnaceStateCorpusRelDir,
			op:     "decode",
			input: inventoryPublicationWithStack(furnaceWire, furnaceFuelOffset,
				inventoryPublicationStackWire(core.ItemStone, 1, 0)),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			// The output slot holds a registered item that is not a smelting
			// product.
			id:     furnaceInvalidOutputID,
			family: furnaceStateFamily,
			relDir: furnaceStateCorpusRelDir,
			op:     "decode",
			input: inventoryPublicationWithStack(furnaceWire, furnaceOutputOffset,
				inventoryPublicationStackWire(core.ItemStone, 1, 0)),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:      furnaceInvalidFuelEncID,
			family:  furnaceStateFamily,
			relDir:  furnaceStateCorpusRelDir,
			op:      "encode",
			request: inventoryPublicationFurnaceInvalidFuelRequest(),
			expect:  invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:     furnaceTrailingID,
			family: furnaceStateFamily,
			relDir: furnaceStateCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), furnaceWire...), 0x00),
			expect: trailing,
		},
		inventoryPublicationCaseDefinition{
			id:     chestDecodeCaseID,
			family: chestStateFamily,
			relDir: chestStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), chestWire...),
			expect: validChest,
		},
		inventoryPublicationCaseDefinition{
			id:      chestEncodeCaseID,
			family:  chestStateFamily,
			relDir:  chestStateCorpusRelDir,
			op:      "encode",
			request: inventoryPublicationChestRequestFields(),
			wire:    append([]byte(nil), chestWire...),
			expect:  validChest,
		},
		inventoryPublicationCaseDefinition{
			// A valid furnace reference beside the chest family is the single
			// kind violation.
			id:     chestWrongKindID,
			family: chestStateFamily,
			relDir: chestStateCorpusRelDir,
			op:     "decode",
			input:  inventoryPublicationWithRef(chestWire, furnaceRefWire),
			expect: invalidEnum,
		},
		inventoryPublicationCaseDefinition{
			id:     chestRefSlotAboveID,
			family: chestStateFamily,
			relDir: chestStateCorpusRelDir,
			op:     "decode",
			input: inventoryPublicationWithRef(chestWire,
				inventoryPublicationRefWire(0, -1, 2, core.ContainerKindChest, 16, 1)),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			// One unregistered item number in the first slot: the item-stack
			// rule's own enum boundary, which the shared Go message resolves
			// at because no count or durability case is registered here.
			id:     chestInvalidStackID,
			family: chestStateFamily,
			relDir: chestStateCorpusRelDir,
			op:     "decode",
			input: inventoryPublicationWithStack(chestWire, 18,
				inventoryPublicationStackWire(core.ItemID(200), 1, 0)),
			expect: invalidEnum,
		},
		inventoryPublicationCaseDefinition{
			// The cut lands inside the stack region.
			id:     chestTruncatedID,
			family: chestStateFamily,
			relDir: chestStateCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), chestWire[:100]...),
			expect: truncated,
		},
		inventoryPublicationCaseDefinition{
			id:     chestTrailingID,
			family: chestStateFamily,
			relDir: chestStateCorpusRelDir,
			op:     "decode",
			input:  append(append([]byte(nil), chestWire...), 0x00),
			expect: trailing,
		},
		inventoryPublicationCaseDefinition{
			id:     closedDecodeCaseID,
			family: containerClosedFamily,
			relDir: containerClosedCorpusRelDir,
			op:     "decode",
			input:  append([]byte(nil), furnaceRefWire...),
			expect: validClosedFurnace,
		},
		inventoryPublicationCaseDefinition{
			// Both kinds publish: the encode twin carries the chest reference.
			id:      closedEncodeCaseID,
			family:  containerClosedFamily,
			relDir:  containerClosedCorpusRelDir,
			op:      "encode",
			request: inventoryPublicationClosedRequest{Container: inventoryPublicationChestRefRequestFields()},
			wire:    append([]byte(nil), chestRefWire...),
			expect:  validClosedChest,
		},
		inventoryPublicationCaseDefinition{
			// The exact all-zero record is refused through its zero
			// generation: kind 0 is a furnace and the overworld dimension is
			// satisfied, so `validFurnaceRef` answers at the generation.
			id:     closedExactNoneID,
			family: containerClosedFamily,
			relDir: containerClosedCorpusRelDir,
			op:     "decode",
			input:  make([]byte, containerClosedWireBytes),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			id:     closedForeignDimensionID,
			family: containerClosedFamily,
			relDir: containerClosedCorpusRelDir,
			op:     "decode",
			input: inventoryPublicationRefWire(-1, -1, 2,
				core.ContainerKindFurnace, 31, 1),
			expect: invalidValue,
		},
		inventoryPublicationCaseDefinition{
			// The unknown kind is the single violation; the dimension, slot
			// and generation stay valid so no second rule fires.
			id:     closedKindTwoID,
			family: containerClosedFamily,
			relDir: containerClosedCorpusRelDir,
			op:     "decode",
			input: inventoryPublicationRefWire(0, -1, 2,
				core.ContainerKind(2), 15, 1),
			expect: invalidEnum,
		},
	)

	return definitions
}

// inventoryPublicationCraftingSizeThreeSlots is the reviewed workbench grid's
// ordered slot list.
func inventoryPublicationCraftingSizeThreeSlots() []core.ItemStack {
	slots := inventoryPublicationCraftingSizeThreeDTO().Slots
	return slots[:]
}

// inventoryPublicationCraftingSizeThreeDTO is the reviewed workbench grid the
// size-three case publishes.
func inventoryPublicationCraftingSizeThreeDTO() protocol.CraftingState {
	var slots [core.CraftingGridSlots]core.ItemStack
	for index := range slots {
		slots[index] = core.ItemStack{Item: core.ItemStone, Count: 1}
	}
	return protocol.CraftingState{
		Size:   3,
		Slots:  slots,
		Output: core.ItemStack{Item: core.ItemStone, Count: 1},
	}
}

// inventoryPublicationSelectedNineRequest is the invalid encode request whose
// only violation is the selected index.
func inventoryPublicationSelectedNineRequest() inventoryPublicationInventoryRequest {
	request := inventoryPublicationInventoryRequestFields()
	request.Selected = 9
	return request
}

// inventoryPublicationCraftingResidueRequest is the invalid encode request
// whose only violation is the personal-grid residue cell.
func inventoryPublicationCraftingResidueRequest() inventoryPublicationCraftingRequest {
	request := inventoryPublicationCraftingRequestFields()
	request.Slots[4] = inventoryPublicationSlotRequest(int(core.ItemStone), 1, 0)
	return request
}

// inventoryPublicationFurnaceInvalidFuelRequest is the invalid encode request
// whose only violation is the fuel whitelist.
func inventoryPublicationFurnaceInvalidFuelRequest() inventoryPublicationFurnaceRequest {
	request := inventoryPublicationFurnaceRequestFields()
	request.Fuel = inventoryPublicationSlotRequest(int(core.ItemStone), 1, 0)
	return request
}

// inventoryPublicationLabel renders one case's asset label from its identity.
func inventoryPublicationLabel(id string) string {
	return id[strings.LastIndex(id, "/")+1:]
}

// buildInventoryPublicationCandidate builds one case's manifest entry and
// asset bytes from its definition.
//
// The expectation is rendered before the case is executed, so the recorded
// outcome is the review contract. An encode case carries the digest of its
// reviewed wire literal rather than of anything a producer produced.
func buildInventoryPublicationCandidate(t *testing.T, definition inventoryPublicationCaseDefinition) inventoryPublicationCandidate {
	t.Helper()

	label := inventoryPublicationLabel(definition.id)
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
		Version:      inventoryPublicationVersion,
		Operation:    definition.op,
		PacketKey:    inventoryPublicationKeyPointer(definition.family),
		Input:        AssetRef{Path: inputPath, SHA256: digestOf(t, assets[inputPath])},
		InputFormat:  inputFormat(definition.op),
		Expected:     AssetRef{Path: expectedPath, SHA256: digestOf(t, assets[expectedPath])},
		Checkpoints:  []string{"0"},
		RustConsumer: "mornlea_protocol",
	}
	return inventoryPublicationCandidate{Spec: spec, Assets: assets, Expect: expect, Wire: definition.wire}
}

// inventoryPublicationKeyPointer resolves one family's reviewed packet key
// for a case spec.
func inventoryPublicationKeyPointer(family string) *PacketKeySpec {
	key, owned := inventoryPublicationFamilyKeys[family]
	if !owned {
		return nil
	}
	resolved := key
	return &resolved
}

// inventoryPublicationCandidate is one reviewed case: its manifest
// specification, the exact asset bytes it publishes, and the expectation an
// independent execution has to reproduce.
type inventoryPublicationCandidate struct {
	Spec   CaseSpec
	Assets map[string][]byte
	Expect Outcome
	// Wire is the review-contract encoded payload for an encode case, which
	// the producer's own bytes are compared against.
	Wire []byte
}

// inventoryPublicationCandidates builds this group's complete reviewed case
// set.
//
// The candidate is derived from the repository state: when the tracked corpus
// already carries a case, its bytes must equal this candidate exactly, so an
// integration that drifted from the reviewed evidence fails here instead of
// silently changing what the case means.
func inventoryPublicationCandidates(t *testing.T, root string) []inventoryPublicationCandidate {
	t.Helper()

	frozen := loadRealManifest(t, root)
	registered := make(map[string]CaseSpec, len(frozen.Cases))
	for _, existing := range frozen.Cases {
		registered[existing.ID] = existing
	}

	definitions := inventoryPublicationCaseDefinitions()
	candidates := make([]inventoryPublicationCandidate, 0, len(definitions))
	for _, definition := range definitions {
		candidate := buildInventoryPublicationCandidate(t, definition)
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

// inventoryPublicationRegisteredCase reports whether the base manifest
// already carries one case identity, so a re-merge of an integrated candidate
// adds nothing.
func inventoryPublicationRegisteredCase(base Inventory, id string) bool {
	for _, existing := range base.Cases {
		if existing.ID == id {
			return true
		}
	}
	return false
}

// inventoryPublicationSelection is this group's registration: its cases, the
// Go sources its rules are read from, and the routes the five families
// execute.
func inventoryPublicationSelection(t *testing.T, root string) ProtocolSelection {
	t.Helper()
	candidates := inventoryPublicationCandidates(t, root)
	cases := make([]CaseSpec, 0, len(candidates))
	for _, candidate := range candidates {
		cases = append(cases, candidate.Spec)
	}
	sources := map[string][]string{}
	for _, family := range inventoryPublicationFamilies() {
		sources[family.id] = inventoryPublicationServerSources(family.id)
	}
	routes := make([]ConsumerRoute, 0, len(inventoryPublicationFamilies())*2)
	for _, family := range inventoryPublicationFamilies() {
		routes = append(routes,
			ConsumerRoute{FamilyID: family.id, Version: inventoryPublicationVersion, Operation: "decode"},
			ConsumerRoute{FamilyID: family.id, Version: inventoryPublicationVersion, Operation: "encode"},
		)
	}
	return ProtocolSelection{
		ProducerID:  inventoryPublicationProducerID,
		Cases:       cases,
		SourcePaths: sources,
		Routes:      routes,
	}
}

// inventoryPublicationServerSources lists the Go sources each family reads
// its rules from: the shared server codec dispatch with its decode arms, and
// the family's own message file, which owns the validator the shared dispatch
// applies at the tail.
func inventoryPublicationServerSources(family string) []string {
	shared := []string{
		"packages/shared/network/codec/codec_server.go",
	}
	switch family {
	case inventoryStateFamily, craftingStateFamily:
		return append(shared, "packages/shared/network/protocol/message_inventory.go")
	case furnaceStateFamily, chestStateFamily, containerClosedFamily:
		return append(shared, "packages/shared/network/protocol/message_container.go")
	default:
		return shared
	}
}

// inventoryPublicationManifest assembles the family-scoped selection the
// route runner executes: this group's candidates with every other family
// cleared, so reconciliation accepts the scoped manifest.
func inventoryPublicationManifest(t *testing.T, root string, candidates []inventoryPublicationCandidate) Inventory {
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
		if !inventoryPublicationOwnsFamily(family) {
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
		t.Fatalf("encode inventory publication working manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "contracts.json")
	if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
		t.Fatalf("write inventory publication working manifest: %v", err)
	}
	loaded, err := LoadInventory(path)
	if err != nil {
		t.Fatalf("load inventory publication working manifest: %v", err)
	}
	return loaded
}

// inventoryPublicationOwnsFamily reports whether this group registers cases
// for one family.
func inventoryPublicationOwnsFamily(family string) bool {
	_, owned := inventoryPublicationFamilyKeys[family]
	return owned
}

// inventoryPublicationScratchRoot stages this group's candidate assets in a
// harness-owned temporary directory, because a corpus case has to resolve
// under the root the runner is given.
func inventoryPublicationScratchRoot(t *testing.T, root string, candidates []inventoryPublicationCandidate) string {
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

// inventoryPublicationCandidateByID resolves one candidate by its case
// identity.
func inventoryPublicationCandidateByID(t *testing.T, candidates []inventoryPublicationCandidate, id string) inventoryPublicationCandidate {
	t.Helper()
	for _, candidate := range candidates {
		if candidate.Spec.ID == id {
			return candidate
		}
	}
	t.Fatalf("no candidate carries case %s", id)
	return inventoryPublicationCandidate{}
}

// inventoryPublicationObservation resolves one executed observation by its
// case identity.
func inventoryPublicationObservation(t *testing.T, observations []ExecutedObservation, id string) ExecutedObservation {
	t.Helper()
	for _, observation := range observations {
		if observation.CaseID == id {
			return observation
		}
	}
	t.Fatalf("no observation was produced for case %s", id)
	return ExecutedObservation{}
}

// inventoryPublicationExportPublished guards the single publication per test
// process, because the exporter's producer child is create-exclusive and more
// than one test in this package observes the same candidates.
var inventoryPublicationExportPublished bool

// inventoryPublicationCandidatesExport publishes the reviewed candidates and
// the complete merged manifest candidate through the existing external
// exporter and returns the published producer directory. An unset export
// variable publishes nothing and returns "", so an ordinary test run never
// writes outside its own temporary storage.
func inventoryPublicationCandidatesExport(t *testing.T, root string, candidates []inventoryPublicationCandidate, merged Inventory) string {
	t.Helper()
	exportRoot := strings.TrimSpace(os.Getenv(runtimeOracleExportDirEnv))
	if exportRoot == "" {
		return ""
	}
	if inventoryPublicationExportPublished {
		return filepath.Join(exportRoot, "runtime-oracle", "protocol-inventory-publication")
	}
	inventoryPublicationExportPublished = true

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
	published, err := exportGeneratedAssets(root, exportRoot, inventoryPublicationProducerID, assets)
	if err != nil {
		t.Fatalf("export inventory publication candidates: %v", err)
	}
	return published
}

// inventoryPublicationDecodeCaseIDs lists the valid decode cases the producer
// pins.
func inventoryPublicationDecodeCaseIDs() []string {
	return []string{
		inventoryDecodeCaseID,
		craftingDecodeCaseID,
		craftingSizeThreeID,
		furnaceDecodeCaseID,
		chestDecodeCaseID,
		closedDecodeCaseID,
	}
}

// inventoryPublicationEncodeCaseIDs lists the valid encode cases the producer
// pins.
func inventoryPublicationEncodeCaseIDs() []string {
	return []string{
		inventoryEncodeCaseID,
		craftingEncodeCaseID,
		furnaceEncodeCaseID,
		chestEncodeCaseID,
		closedEncodeCaseID,
	}
}

// inventoryPublicationRejectedDecodeCaseIDs lists the malformed decode cases.
func inventoryPublicationRejectedDecodeCaseIDs() []string {
	return []string{
		inventorySelectedNineDecID,
		inventoryToolDurabilityID,
		inventoryTrailingID,
		inventoryTruncatedID,
		craftingSizeZeroID,
		craftingSizeFourID,
		craftingResidueSlotFourID,
		craftingTrailingID,
		furnaceProgressAtLimitID,
		furnaceBurnAboveID,
		furnaceWrongKindID,
		furnaceZeroGenerationID,
		furnaceInvalidFuelID,
		furnaceInvalidOutputID,
		furnaceTrailingID,
		chestWrongKindID,
		chestRefSlotAboveID,
		chestInvalidStackID,
		chestTruncatedID,
		chestTrailingID,
		closedExactNoneID,
		closedForeignDimensionID,
		closedKindTwoID,
	}
}

// inventoryPublicationRejectedEncodeCaseIDs lists the invalid encode cases.
func inventoryPublicationRejectedEncodeCaseIDs() []string {
	return []string{
		inventorySelectedNineEncID,
		craftingResidueSlotFourEnc,
		furnaceInvalidFuelEncID,
	}
}

// TestProtocolInventoryPublicationOracleDecodesEveryValidCase pins that the
// real Go decoder publishes the canonical fields for every family's valid
// decode case, including the workbench crafting variant.
func TestProtocolInventoryPublicationOracleDecodesEveryValidCase(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := inventoryPublicationCandidates(t, root)

	for _, id := range inventoryPublicationDecodeCaseIDs() {
		candidate := inventoryPublicationCandidateByID(t, candidates, id)
		outcome, encoded, err := runInventoryPublicationDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runInventoryPublicationDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("decode case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolInventoryPublicationOracleEncodesCanonicalWire pins each encode
// producer against the reviewed wire literal and against the recorded digest.
func TestProtocolInventoryPublicationOracleEncodesCanonicalWire(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := inventoryPublicationCandidates(t, root)

	for _, id := range inventoryPublicationEncodeCaseIDs() {
		candidate := inventoryPublicationCandidateByID(t, candidates, id)
		outcome, encoded, err := runInventoryPublicationEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runInventoryPublicationEncode(%s): %v", id, err)
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

// TestProtocolInventoryPublicationOracleRejectsMalformedCasesAtTheirBoundary
// pins that every malformed decode case and invalid encode case is refused by
// the production codec and classified at the boundary that owns it.
func TestProtocolInventoryPublicationOracleRejectsMalformedCasesAtTheirBoundary(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := inventoryPublicationCandidates(t, root)

	for _, id := range inventoryPublicationRejectedDecodeCaseIDs() {
		candidate := inventoryPublicationCandidateByID(t, candidates, id)
		outcome, encoded, err := runInventoryPublicationDecode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runInventoryPublicationDecode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}

	for _, id := range inventoryPublicationRejectedEncodeCaseIDs() {
		candidate := inventoryPublicationCandidateByID(t, candidates, id)
		outcome, encoded, err := runInventoryPublicationEncode(candidate.Spec, candidate.Assets[candidate.Spec.Input.Path])
		if err != nil {
			t.Fatalf("runInventoryPublicationEncode(%s): %v", id, err)
		}
		if len(encoded) != 0 {
			t.Fatalf("rejected case %s published %d encoded bytes", id, len(encoded))
		}
		if !outcomesEqual(outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", id, outcome, candidate.Expect)
		}
	}
}

// TestProtocolInventoryPublicationOracleExpectedFieldMutationFailsComparison
// pins that the recorded expectation is a commitment: replacing an expected
// field fails comparison against what the producer decoded, and replacing the
// encoded digest fails comparison against the reviewed wire.
func TestProtocolInventoryPublicationOracleExpectedFieldMutationFailsComparison(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := inventoryPublicationCandidates(t, root)

	decode := inventoryPublicationCandidateByID(t, candidates, inventoryDecodeCaseID)
	produced, _, err := runInventoryPublicationDecode(decode.Spec, decode.Assets[decode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runInventoryPublicationDecode: %v", err)
	}
	mutated := decode.Expect
	fields := make(map[string]any, len(decode.Expect.Fields))
	for key, value := range decode.Expect.Fields {
		fields[key] = value
	}
	// The tool slot is the contract: a normalization that dropped the
	// durability field would hide the single-count tool the vector carries.
	inventory := inventoryPublicationInventoryDTO()
	fields["hotbar"] = append([]map[string]any(nil),
		append(inventoryPublicationStacksFields(
			inventory.Hotbar.Slots[:8],
		), map[string]any{"item": int(core.ItemStonePickaxe), "count": 1, "durability": 0})...,
	)
	mutated.Fields = fields
	if outcomesEqual(mutated, produced) {
		t.Fatal("mutated tool durability compares equal to the produced outcome")
	}

	encode := inventoryPublicationCandidateByID(t, candidates, furnaceEncodeCaseID)
	producedEncode, _, err := runInventoryPublicationEncode(encode.Spec, encode.Assets[encode.Spec.Input.Path])
	if err != nil {
		t.Fatalf("runInventoryPublicationEncode: %v", err)
	}
	mutatedDigest := producedEncode
	mutatedDigest.EncodedPayloadDigest = "sha256:" + strings.Repeat("0", 64)
	if outcomesEqual(mutatedDigest, encode.Expect) {
		t.Fatal("mutated encoded digest compares equal to the reviewed expectation")
	}
}

// TestProtocolInventoryPublicationOracleRoutesExecuteEveryCase executes this
// group's complete case set through the shared packet-case runner, once per
// case, and compares every observation with the reviewed expectation.
func TestProtocolInventoryPublicationOracleRoutesExecuteEveryCase(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := inventoryPublicationCandidates(t, root)
	manifest := inventoryPublicationManifest(t, root, candidates)
	staged := inventoryPublicationScratchRoot(t, root, candidates)

	observations, err := RunProtocolCases(staged, manifest, inventoryPublicationCorpusRoutes())
	if err != nil {
		t.Fatalf("RunProtocolCases: %v", err)
	}
	if len(observations) != len(candidates) {
		t.Fatalf("produced %d observations, want %d (one per case)", len(observations), len(candidates))
	}
	for _, candidate := range candidates {
		observation := inventoryPublicationObservation(t, observations, candidate.Spec.ID)
		if !outcomesEqual(observation.Outcome, candidate.Expect) {
			t.Fatalf("case %s produced %#v, want %#v", candidate.Spec.ID, observation.Outcome, candidate.Expect)
		}
	}
}

// TestProtocolInventoryPublicationOracleRunnerRejectsUnregisteredRoute pins
// that a case naming a route this group does not claim fails before its
// producer runs, so a case cannot claim coverage from its name alone.
func TestProtocolInventoryPublicationOracleRunnerRejectsUnregisteredRoute(t *testing.T) {
	root := mustRepoRoot(t)
	candidates := inventoryPublicationCandidates(t, root)
	manifest := inventoryPublicationManifest(t, root, candidates)
	staged := inventoryPublicationScratchRoot(t, root, candidates)

	decodeOnly := map[ConsumerRoute]GoOperation{}
	for _, family := range inventoryPublicationFamilies() {
		decodeOnly[ConsumerRoute{FamilyID: family.id, Version: inventoryPublicationVersion, Operation: "decode"}] = runInventoryPublicationDecode
	}
	observations, err := RunProtocolCases(staged, manifest, decodeOnly)
	if err == nil {
		t.Fatal("RunProtocolCases accepted a case whose route is not registered")
	}
	if len(observations) != 0 {
		t.Fatalf("produced %d observations before the route rejection", len(observations))
	}
	named := false
	for _, family := range inventoryPublicationFamilies() {
		if strings.Contains(err.Error(), family.id+"/"+inventoryPublicationVersion+"/encode") {
			named = true
			break
		}
	}
	if !named {
		t.Fatalf("rejection %v does not name any family's encode route", err)
	}
}

// TestProtocolInventoryPublicationOracleManifestMergeRegistersPublicationRoutes
// pins that the merged manifest registers all five families' routes and case
// lists, leaves the source revision alone, and records this group's
// provenance sources.
func TestProtocolInventoryPublicationOracleManifestMergeRegistersPublicationRoutes(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	base := loadRealManifest(t, root)
	merged, err := mergeProtocolSelections(root, base, inventoryPublicationSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}
	if merged.SourceRevision != base.SourceRevision {
		t.Fatalf("merged source_revision = %s, want %s", merged.SourceRevision, base.SourceRevision)
	}

	for _, family := range inventoryPublicationFamilies() {
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
		for _, want := range inventoryPublicationServerSources(family.id) {
			if !sources[want] {
				t.Fatalf("%s provenance drops %s", family.id, want)
			}
		}
	}

	// The merged case list is the sorted union of the base and this group, so
	// a re-merge of an already-registered candidate adds nothing.
	wantCases := len(base.Cases)
	for _, candidate := range inventoryPublicationCandidates(t, root) {
		if !inventoryPublicationRegisteredCase(base, candidate.Spec.ID) {
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

// TestProtocolInventoryPublicationOracleCandidatesExportForReview publishes
// the reviewed candidates and the manifest candidate. An unset export
// variable publishes nothing, so the tracked corpus is never written by this
// package.
func TestProtocolInventoryPublicationOracleCandidatesExportForReview(t *testing.T) {
	root := mustRepoRoot(t)
	before := computeTrackedCorpusDigest(t, root)
	defer assertTrackedCorpusUnchanged(t, root, before)

	candidates := inventoryPublicationCandidates(t, root)
	merged, err := mergeProtocolSelections(root, loadRealManifest(t, root), inventoryPublicationSelection(t, root))
	if err != nil {
		t.Fatalf("mergeProtocolSelections: %v", err)
	}

	published := inventoryPublicationCandidatesExport(t, root, candidates, merged)
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

// TestProtocolInventoryPublicationGoEncoderRefusesInvalidFuel pins that the
// production encoder runs the outbound validator before it writes the fuel
// slot, so a stone stack never becomes a silently published fuel value.
func TestProtocolInventoryPublicationGoEncoderRefusesInvalidFuel(t *testing.T) {
	wireCodec, err := newInventoryPublicationCodec()
	if err != nil {
		t.Fatalf("newInventoryPublicationCodec: %v", err)
	}
	defer func() { _ = wireCodec.Close() }()

	state := inventoryPublicationFurnaceDTO()
	state.Fuel = core.ItemStack{Item: core.ItemStone, Count: 1}
	if _, _, err := wireCodec.EncodeServer(protocol.StatePlay, state); err == nil {
		t.Fatal("the Go encoder published a non-coal fuel slot")
	}
	// The silent wire the previous behavior would have published: the fuel
	// slot at offset 23 carrying stone.
	wire := inventoryPublicationFurnaceWire()
	_ = wire
	silent := inventoryPublicationWithStack(inventoryPublicationFurnaceWire(), 23,
		inventoryPublicationStackWire(core.ItemStone, 1, 0))
	if len(silent) != furnaceStateWireBytes {
		t.Fatalf("the silent fuel wire is %d bytes", len(silent))
	}
	if !bytes.Equal(silent[23:28], inventoryPublicationStackWire(core.ItemStone, 1, 0)) {
		t.Fatalf("the silent fuel wire carries %x", silent[23:28])
	}
}
