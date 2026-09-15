package protocol

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestDropStackPacketIDIsFrozen 钉死整组丢弃命令的最终编号：C→S
// `DropStack=21`，由协议 v45 承载；「下一个仍未分配」上界写成「末项 +1」，
// 下次追加 packet 时它会跟着末项走，不会静默退化成「测一个已合法的 ID」。
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

// TestDropStackValidateAcceptsAllViewDomains 覆盖三个视图域的合法边界：各域
// 最后一个统一索引必须放行；零序号与 `MoveStackPartial`/`QuickMoveStack`
// 同一惯例放行（丢弃不参与取出确认协议）。
func TestDropStackValidateAcceptsAllViewDomains(t *testing.T) {
	valid := []ClientPacket{
		DropStack{Sequence: 1, View: StackViewInventory, Slot: core.InventorySlots - 1},
		DropStack{Sequence: 1, View: StackViewCrafting, Slot: GridCraftingViewSlots - 1},
		DropStack{Sequence: 1, Container: testChestRef(), View: StackViewContainer, Slot: core.ChestViewSlots - 1},
		DropStack{Sequence: 1, Container: stackSplittingFurnaceRef(), View: StackViewContainer, Slot: core.FurnaceViewSlots - 1},
		// 零序号跟随分堆命令族惯例：丢弃命令不做「过期序列不重复效果」确认。
		DropStack{View: StackViewInventory, Slot: 0},
	}
	for _, packet := range valid {
		if err := ValidateClientPacket(StatePlay, packet); err != nil {
			t.Fatalf("合法丢弃命令 %T 被拒绝: %v", packet, err)
		}
	}
}

// TestDropStackValidateRejectionMatrix 覆盖整组丢弃的整包拒绝矩阵：非法
// 视图域、容器视图零值/未知种类引用、非容器视图非零引用与按视图分派的
// 索引越界——每一条都在协议校验层整单拒绝，与 `MoveStackPartial` 的静态
// 值域判定完全同界。
func TestDropStackValidateRejectionMatrix(t *testing.T) {
	chest := testChestRef()
	furnace := stackSplittingFurnaceRef()
	invalid := []ClientPacket{
		// 视图域非法：3 与饱和字节都不是 {背包, 合成, 容器}。
		DropStack{Container: chest, View: 3, Slot: 0},
		DropStack{View: 255, Slot: 0},
		// 容器视图携带零值引用。
		DropStack{View: StackViewContainer, Slot: 0},
		// 容器视图携带未知种类引用。
		DropStack{
			Container: core.ContainerRef{Dimension: core.Overworld, Kind: core.ContainerKind(9), Generation: 1},
			View:      StackViewContainer, Slot: 0,
		},
		// 非容器视图携带非零引用。
		DropStack{Container: chest, View: StackViewInventory, Slot: 0},
		DropStack{Container: chest, View: StackViewCrafting, Slot: 0},
		// 背包视图索引越界（0..35）。
		DropStack{View: StackViewInventory, Slot: core.InventorySlots},
		// 合成视图索引越界（0..44）。
		DropStack{View: StackViewCrafting, Slot: GridCraftingViewSlots},
		// 熔炉视图索引越界（0..38）。
		DropStack{Container: furnace, View: StackViewContainer, Slot: core.FurnaceViewSlots},
		// 箱子视图索引越界（0..62）。
		DropStack{Container: chest, View: StackViewContainer, Slot: core.ChestViewSlots},
	}
	for _, packet := range invalid {
		if err := ValidateClientPacket(StatePlay, packet); err == nil {
			t.Fatalf("非法丢弃命令 %T 通过了校验", packet)
		}
	}
}
