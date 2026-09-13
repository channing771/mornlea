package protocol

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// stackSplittingFurnaceRef 构造合法熔炉引用：Overworld 固定槽位数组内一格、
// generation 非零（与既有 `testChestRef` 的箱子引用成对覆盖两种容器）。
func stackSplittingFurnaceRef() core.ContainerRef {
	return core.ContainerRef{
		Dimension:  core.Overworld,
		Chunk:      core.ChunkPos{X: 4, Z: -2},
		Kind:       core.ContainerKindFurnace,
		Slot:       3,
		Generation: 5,
	}
}

// TestStackSplittingPacketIDsAreFrozen 钉死分堆双命令的最终编号：C→S
// `MoveStackPartial=19`、`QuickMoveStack=20`，由协议 v44 承载；「下一个仍未
// 分配」上界随之推进到 21。上界断言写成「末项 +1」而不是裸字面量，下次追加
// packet 时它会跟着末项走，不会静默退化成「测一个已合法的 ID」。
func TestStackSplittingPacketIDsAreFrozen(t *testing.T) {
	assertClientRegistry(t, []struct {
		state  State
		packet ClientPacket
		id     uint32
	}{
		{StatePlay, MoveStackPartial{}, 19},
		{StatePlay, QuickMoveStack{}, 20},
	})
	if _, ok := ClientPacketForID(StatePlay, 20+1); ok {
		t.Fatal("Play client packet ID 21 必须保持未分配")
	}
	if ProtocolVersion != 44 {
		t.Fatalf("协议版本 = %d，想要 44——分堆双命令由 v44 承载", ProtocolVersion)
	}
}

// TestStackSplittingValidateAcceptsAllViewDomains 覆盖三个视图域的合法边界：
// 各域最后一个统一索引必须放行；零序号跟随既有背包/容器移动命令的惯例
// 放行（只有参与确认协议的取出命令拒绝零序号）。
func TestStackSplittingValidateAcceptsAllViewDomains(t *testing.T) {
	valid := []ClientPacket{
		MoveStackPartial{Sequence: 1, View: StackViewInventory, From: 0, To: core.InventorySlots - 1},
		MoveStackPartial{Sequence: 1, View: StackViewCrafting, From: GridCraftingViewSlots - 1, To: 0},
		MoveStackPartial{Sequence: 1, Container: testChestRef(), View: StackViewContainer, From: core.ChestViewSlots - 1, To: 0, Single: true},
		MoveStackPartial{Sequence: 1, Container: stackSplittingFurnaceRef(), View: StackViewContainer, From: core.FurnaceViewSlots - 1, To: 0},
		QuickMoveStack{Sequence: 1, View: StackViewInventory, From: core.InventorySlots - 1},
		QuickMoveStack{Sequence: 1, View: StackViewCrafting, From: GridCraftingViewSlots - 1},
		QuickMoveStack{Sequence: 1, Container: testChestRef(), View: StackViewContainer, From: core.ChestViewSlots - 1},
		QuickMoveStack{Sequence: 1, Container: stackSplittingFurnaceRef(), View: StackViewContainer, From: core.FurnaceViewSlots - 1},
		// 零序号与 `MoveInventoryStack`/`MoveContainerStack` 同一惯例：分堆
		// 双命令不参与「过期序列不重复效果」的取出确认，放行零序号。
		MoveStackPartial{View: StackViewInventory, From: 0, To: 1},
		QuickMoveStack{View: StackViewCrafting, From: 9},
	}
	for _, packet := range valid {
		if err := ValidateClientPacket(StatePlay, packet); err != nil {
			t.Fatalf("合法分堆命令 %T 被拒绝: %v", packet, err)
		}
	}
}

// TestStackSplittingValidateRejectionMatrix 覆盖分堆双命令的整包拒绝矩阵：
// 非法视图域、容器视图零值/未知种类引用、非容器视图非零引用、按视图分派的
// 索引越界与部分移动同格——每一条都在协议校验层整单拒绝。
func TestStackSplittingValidateRejectionMatrix(t *testing.T) {
	chest := testChestRef()
	furnace := stackSplittingFurnaceRef()
	invalid := []ClientPacket{
		// 视图域非法：3 与饱和字节都不是 {背包, 合成, 容器}。
		MoveStackPartial{Container: chest, View: 3, From: 0, To: 1},
		MoveStackPartial{View: 255, From: 0, To: 1},
		QuickMoveStack{View: 3, From: 0},
		QuickMoveStack{View: 255, From: 0},
		// 容器视图携带零值引用。
		MoveStackPartial{View: StackViewContainer, From: 0, To: 1},
		QuickMoveStack{View: StackViewContainer, From: 0},
		// 容器视图携带未知种类引用。
		MoveStackPartial{
			Container: core.ContainerRef{Dimension: core.Overworld, Kind: core.ContainerKind(9), Generation: 1},
			View:      StackViewContainer, From: 0, To: 1,
		},
		// 非容器视图携带非零引用。
		MoveStackPartial{Container: chest, View: StackViewInventory, From: 0, To: 1},
		MoveStackPartial{Container: chest, View: StackViewCrafting, From: 0, To: 1},
		QuickMoveStack{Container: chest, View: StackViewInventory, From: 0},
		QuickMoveStack{Container: chest, View: StackViewCrafting, From: 0},
		// 背包视图索引越界（0..35）。
		MoveStackPartial{View: StackViewInventory, From: core.InventorySlots, To: 0},
		MoveStackPartial{View: StackViewInventory, From: 0, To: core.InventorySlots},
		QuickMoveStack{View: StackViewInventory, From: core.InventorySlots},
		// 合成视图索引越界（0..44）。
		MoveStackPartial{View: StackViewCrafting, From: GridCraftingViewSlots, To: 0},
		MoveStackPartial{View: StackViewCrafting, From: 0, To: GridCraftingViewSlots},
		QuickMoveStack{View: StackViewCrafting, From: GridCraftingViewSlots},
		// 熔炉视图索引越界（0..38）。
		MoveStackPartial{Container: furnace, View: StackViewContainer, From: core.FurnaceViewSlots, To: 0},
		MoveStackPartial{Container: furnace, View: StackViewContainer, From: 0, To: core.FurnaceViewSlots},
		QuickMoveStack{Container: furnace, View: StackViewContainer, From: core.FurnaceViewSlots},
		// 箱子视图索引越界（0..62）。
		MoveStackPartial{Container: chest, View: StackViewContainer, From: core.ChestViewSlots, To: 0},
		MoveStackPartial{Container: chest, View: StackViewContainer, From: 0, To: core.ChestViewSlots},
		QuickMoveStack{Container: chest, View: StackViewContainer, From: core.ChestViewSlots},
		// 部分移动来源等于目标。
		MoveStackPartial{View: StackViewInventory, From: 7, To: 7},
		MoveStackPartial{View: StackViewCrafting, From: 9, To: 9},
		MoveStackPartial{Container: chest, View: StackViewContainer, From: 2, To: 2},
	}
	for _, packet := range invalid {
		if err := ValidateClientPacket(StatePlay, packet); err == nil {
			t.Fatalf("非法分堆命令 %T 通过了校验", packet)
		}
	}
}

// TestStackSplittingViewConstantsAreFrozen 钉死视图域枚举的 wire 取值：
// {背包, 合成, 容器} = {0, 1, 2}，sim 权威路径与前端桥都消费这组导出常量。
func TestStackSplittingViewConstantsAreFrozen(t *testing.T) {
	if StackViewInventory != 0 || StackViewCrafting != 1 || StackViewContainer != 2 {
		t.Fatalf("视图域常量 = %d/%d/%d，想要 0/1/2",
			StackViewInventory, StackViewCrafting, StackViewContainer)
	}
}
