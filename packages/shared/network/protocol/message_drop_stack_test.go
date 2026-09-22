package protocol

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestDropStackPacketIDIsFrozen freezes `DropStack=21` for v45 C-to-S traffic.
// Expressing the next-unassigned boundary relative to the last ID keeps a later
// append from silently turning the assertion into a check of an assigned ID.
func TestDropStackPacketIDIsFrozen(t *testing.T) {
	assertClientRegistry(t, []struct {
		state  State
		packet ClientPacket
		id     uint32
	}{
		{StatePlay, DropStack{}, 21},
	})
	if _, ok := ClientPacketForID(StatePlay, 21+1); ok {
		t.Fatal("Play client packet ID 22 必须保持未分配")
	}
	if ProtocolVersion != 45 {
		t.Fatalf("协议版本 = %d，想要 45——整组丢弃命令由 v45 承载", ProtocolVersion)
	}
}

// TestDropStackValidateAcceptsAllViewDomains covers each view's final valid
// unified index. A zero sequence follows `MoveStackPartial` and
// `QuickMoveStack` because drops do not participate in take confirmation.
func TestDropStackValidateAcceptsAllViewDomains(t *testing.T) {
	valid := []ClientPacket{
		DropStack{Sequence: 1, View: StackViewInventory, Slot: core.InventorySlots - 1},
		DropStack{Sequence: 1, View: StackViewCrafting, Slot: GridCraftingViewSlots - 1},
		DropStack{Sequence: 1, Container: testChestRef(), View: StackViewContainer, Slot: core.ChestViewSlots - 1},
		DropStack{Sequence: 1, Container: stackSplittingFurnaceRef(), View: StackViewContainer, Slot: core.FurnaceViewSlots - 1},
		// Zero follows the stack-splitting convention because drops have no
		// stale-sequence acknowledgement.
		DropStack{View: StackViewInventory, Slot: 0},
	}
	for _, packet := range valid {
		if err := ValidateClientPacket(StatePlay, packet); err != nil {
			t.Fatalf("合法丢弃命令 %T 被拒绝: %v", packet, err)
		}
	}
}

// TestDropStackValidateRejectionMatrix rejects the whole command for invalid
// views, invalid or mismatched references, and view-specific slot overflow.
// These static domains intentionally match `MoveStackPartial`.
func TestDropStackValidateRejectionMatrix(t *testing.T) {
	chest := testChestRef()
	furnace := stackSplittingFurnaceRef()
	invalid := []ClientPacket{
		// Neither 3 nor the saturated byte identifies an inventory, crafting,
		// or container view.
		DropStack{Container: chest, View: 3, Slot: 0},
		DropStack{View: 255, Slot: 0},
		// A container view cannot use the zero reference.
		DropStack{View: StackViewContainer, Slot: 0},
		// A container view cannot use an unknown container kind.
		DropStack{
			Container: core.ContainerRef{Dimension: core.Overworld, Kind: core.ContainerKind(9), Generation: 1},
			View:      StackViewContainer, Slot: 0,
		},
		// Non-container views cannot carry a container reference.
		DropStack{Container: chest, View: StackViewInventory, Slot: 0},
		DropStack{Container: chest, View: StackViewCrafting, Slot: 0},
		// Inventory slots are bounded to 0..35.
		DropStack{View: StackViewInventory, Slot: core.InventorySlots},
		// Crafting slots are bounded to 0..44.
		DropStack{View: StackViewCrafting, Slot: GridCraftingViewSlots},
		// Furnace slots are bounded to 0..38.
		DropStack{Container: furnace, View: StackViewContainer, Slot: core.FurnaceViewSlots},
		// Chest slots are bounded to 0..62.
		DropStack{Container: chest, View: StackViewContainer, Slot: core.ChestViewSlots},
	}
	for _, packet := range invalid {
		if err := ValidateClientPacket(StatePlay, packet); err == nil {
			t.Fatalf("非法丢弃命令 %T 通过了校验", packet)
		}
	}
}
