package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// stackSplitFurnaceFixture 在玩家脚下放一只已登记槽位的熔炉并按需改写玩家
// 物品与网格；查看关系由 openStackSplitFurnace 显式建立。
func stackSplitFurnaceFixture(
	t *testing.T,
	inventory core.Inventory,
	furnace world.FurnaceSlot,
	grid CraftingGrid,
) (*Engine, SessionID, core.FurnaceRef) {
	t.Helper()
	engine, session := stackSplitReadyPlayer(t, inventory, grid)
	index, indexed := world.ChunkBlockIndex(core.BlockPos{})
	if !indexed {
		t.Fatal("熔炉方块没有区块索引")
	}
	engine.SetBlockForTest(core.BlockPos{}, core.FurnaceID)
	furnace.BlockIndex = index
	furnace.Active = true
	if furnace.Generation == 0 {
		furnace.Generation = 1
	}
	engine.SetChunkFurnaceForTest(core.ChunkKey{Dimension: core.Overworld}, 0, furnace)
	ref := core.FurnaceRef{
		Dimension: core.Overworld, Chunk: core.ChunkPos{},
		Kind: core.ContainerKindFurnace, Slot: 0, Generation: furnace.Generation,
	}
	return engine, session, ref
}

// openStackSplitFurnace 用权威射线命令建立熔炉查看关系。
func openStackSplitFurnace(t *testing.T, engine *Engine, session SessionID) {
	t.Helper()
	opened := applyPlayerCommandsTick(engine, []Command{{
		Session: session, Sequence: 2, Kind: CommandOpenFurnace, Pitch: fluidLookDown,
	}})
	if len(opened.Rejected) != 0 || len(opened.Furnaces) != 1 {
		t.Fatalf("打开熔炉失败: rejected=%+v furnaces=%+v", opened.Rejected, opened.Furnaces)
	}
}

// stackSplitFurnaceAt 读取熔炉当前权威值。
func stackSplitFurnaceAt(t *testing.T, engine *Engine, ref core.FurnaceRef) world.FurnaceSlot {
	t.Helper()
	_, furnace, ok := engine.furnaceView(ref)
	if !ok {
		t.Fatal("熔炉引用失效")
	}
	return furnace
}

// TestStackSplitFurnaceSingleIntoInputKeepsProgress 验收熔炉输入槽约束复用：
// 单件同类并入不换物品，熔炼进度不重置。
func TestStackSplitFurnaceSingleIntoInputKeepsProgress(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemRawIron, Count: 5}
	furnace := world.FurnaceSlot{
		Input:         core.ItemStack{Item: core.ItemRawIron, Count: 3},
		ProgressTicks: 137,
		BurnTicks:     1463,
	}
	engine, session, ref := stackSplitFurnaceFixture(t, inventory, furnace, CraftingGrid{})
	openStackSplitFurnace(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 3, StackViewContainer, 0, core.FurnaceInputSlot, true, ref),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("单件并入输入槽被拒绝: %+v", result.Rejected)
	}
	got := stackSplitFurnaceAt(t, engine, ref)
	if got.Input != (core.ItemStack{Item: core.ItemRawIron, Count: 4}) {
		t.Fatalf("输入槽 = %+v，想要恰好 4 个", got.Input)
	}
	if got.ProgressTicks != 137 || got.BurnTicks != 1463 {
		t.Fatalf("同类并入不应重置进度: %+v", got)
	}
	if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar != (core.ItemStack{Item: core.ItemRawIron, Count: 4}) {
		t.Fatalf("来源格 = %+v，想要减 1 剩 4", hotbar)
	}
}

// TestStackSplitFurnaceSlotConstraints 验收 spec 场景「熔炉槽位约束沿既有
// 规则」：燃料槽只收煤、输入槽只收熔炼表内物品，异类整单拒绝；输出槽只可
// 作为来源，作为目标在 sim 层一律拒绝（协议层放行由权威层兜底）。
func TestStackSplitFurnaceSlotConstraints(t *testing.T) {
	cases := []struct {
		name   string
		item   core.ItemID
		target uint8
		single bool
		ok     bool
	}{
		{"煤单件进燃料", core.ItemCoal, core.FurnaceFuelSlot, true, true},
		{"石头单件进燃料拒", core.ItemStone, core.FurnaceFuelSlot, true, false},
		{"石头半组进燃料拒", core.ItemStone, core.FurnaceFuelSlot, false, false},
		{"粗铁单件进输入", core.ItemRawIron, core.FurnaceInputSlot, true, true},
		{"石头单件进输入拒", core.ItemStone, core.FurnaceInputSlot, true, false},
		{"铁锭单件进输出拒", core.ItemIronIngot, core.FurnaceOutputSlot, true, false},
		{"空输出作半组目标拒", core.ItemRawIron, core.FurnaceOutputSlot, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var inventory core.Inventory
			inventory.Hotbar.Slots[0] = core.ItemStack{Item: tc.item, Count: 8}
			engine, session, ref := stackSplitFurnaceFixture(t, inventory, world.FurnaceSlot{}, CraftingGrid{})
			openStackSplitFurnace(t, engine, session)

			result := finishPlayerWorldTick(engine, []Command{
				stackSplitCommand(session, 3, StackViewContainer, 0, tc.target, tc.single, ref),
			})
			got := stackSplitFurnaceAt(t, engine, ref)
			if tc.ok {
				if len(result.Rejected) != 0 {
					t.Fatalf("合法移动被拒绝: %+v", result.Rejected)
				}
				moved := got.Input
				if tc.target == core.FurnaceFuelSlot {
					moved = got.Fuel
				}
				want := uint8(1)
				if !tc.single {
					want = 4
				}
				if moved != (core.ItemStack{Item: tc.item, Count: want}) {
					t.Fatalf("熔炉格 = %+v，想要 %d 个", moved, want)
				}
				if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar.Count != 8-want {
					t.Fatalf("来源格 = %+v，想要剩 %d", hotbar, 8-want)
				}
				return
			}
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
				t.Fatalf("非法移动 result=%+v，想要恰好一次 RejectInvalidInput", result)
			}
			if got.Input != (core.ItemStack{}) || got.Fuel != (core.ItemStack{}) || got.Output != (core.ItemStack{}) {
				t.Fatalf("被拒绝的移动修改了熔炉: %+v", got)
			}
			if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar.Count != 8 {
				t.Fatalf("被拒绝的移动修改了玩家物品: %+v", hotbar)
			}
		})
	}
}

// TestStackSplitFurnaceOutputOnlyAsSource 锁定输出槽只可作为来源：从输出格
// 单件取出 1 个、余 4 个留在输出格（白名单物品按余量原地保留）。
func TestStackSplitFurnaceOutputOnlyAsSource(t *testing.T) {
	furnace := world.FurnaceSlot{
		Output: core.ItemStack{Item: core.ItemIronIngot, Count: 5},
	}
	engine, session, ref := stackSplitFurnaceFixture(t, core.Inventory{}, furnace, CraftingGrid{})
	openStackSplitFurnace(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 3, StackViewContainer, core.FurnaceOutputSlot, 0, true, ref),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("从输出格单件取出被拒绝: %+v", result.Rejected)
	}
	if got := stackSplitFurnaceAt(t, engine, ref).Output; got != (core.ItemStack{Item: core.ItemIronIngot, Count: 4}) {
		t.Fatalf("输出格 = %+v，想要余 4", got)
	}
	if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar != (core.ItemStack{Item: core.ItemIronIngot, Count: 1}) {
		t.Fatalf("目标格 = %+v，想要恰好 1 个", hotbar)
	}
}

// TestStackSplitFurnaceHalfOutOfFuelKeepsRemainder 锁定燃料格作为来源的半组：
// 3 个煤移出 ceil(3/2)=2 个，余 1 个仍在燃料格。
func TestStackSplitFurnaceHalfOutOfFuelKeepsRemainder(t *testing.T) {
	furnace := world.FurnaceSlot{
		Fuel: core.ItemStack{Item: core.ItemCoal, Count: 3},
	}
	engine, session, ref := stackSplitFurnaceFixture(t, core.Inventory{}, furnace, CraftingGrid{})
	openStackSplitFurnace(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 3, StackViewContainer, core.FurnaceFuelSlot, 0, false, ref),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("从燃料格半组取出被拒绝: %+v", result.Rejected)
	}
	if got := stackSplitFurnaceAt(t, engine, ref).Fuel; got != (core.ItemStack{Item: core.ItemCoal, Count: 1}) {
		t.Fatalf("燃料格 = %+v，想要余 1", got)
	}
	if hotbar := engine.sessions[session].player.inventory.Hotbar.Slots[0]; hotbar != (core.ItemStack{Item: core.ItemCoal, Count: 2}) {
		t.Fatalf("目标格 = %+v，想要 2 个", hotbar)
	}
}

// TestStackSplitFurnaceRejectsBrokenRepackInvariant 锁定熔炉→背包增量的回收
// 预演与箱子路径同形：占用网格回收依赖的唯一空格时整体拒绝。
func TestStackSplitFurnaceRejectsBrokenRepackInvariant(t *testing.T) {
	furnace := world.FurnaceSlot{
		Output: core.ItemStack{Item: core.ItemIronIngot, Count: 30},
	}
	engine, session, ref := stackSplitFurnaceFixture(
		t, stackSplitContainerRepackInventory(), furnace, stackSplitContainerRepackGrid(),
	)
	openStackSplitFurnace(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		stackSplitCommand(session, 3, StackViewContainer, core.FurnaceOutputSlot, 18, false, ref),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("破坏回收不变量的移动 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := stackSplitFurnaceAt(t, engine, ref).Output; got != (core.ItemStack{Item: core.ItemIronIngot, Count: 30}) {
		t.Fatalf("被拒绝的移动修改了熔炉: %+v", got)
	}
	player := engine.sessions[session].player
	if player.inventory.Backpack[0] != (core.ItemStack{}) {
		t.Fatalf("被拒绝的移动修改了背包: %+v", player.inventory)
	}
}

// TestStackSplitFurnaceValueDomainRejects 锁定熔炉域值域：熔炉统一视图上界
// 39 与箱子专属栏位都按 RejectInvalidInput 拒绝，不依赖网络层已按 Kind 校验。
func TestStackSplitFurnaceValueDomainRejects(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemRawIron, Count: 4}
	engine, session, ref := stackSplitFurnaceFixture(t, inventory, world.FurnaceSlot{}, CraftingGrid{})
	openStackSplitFurnace(t, engine, session)

	for name, command := range map[string]Command{
		"熔炉视图上界": stackSplitCommand(session, 3, StackViewContainer, 0, core.FurnaceViewSlots, false, ref),
		"箱子专属栏位": stackSplitCommand(session, 3, StackViewContainer, 0, core.ChestFirstSlot+5, false, ref),
	} {
		t.Run(name, func(t *testing.T) {
			result := finishPlayerWorldTick(engine, []Command{command})
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
				t.Fatalf("result=%+v，想要恰好一次 RejectInvalidInput", result)
			}
			if got := stackSplitFurnaceAt(t, engine, ref); got.Input != (core.ItemStack{}) {
				t.Fatalf("被拒绝的移动修改了熔炉: %+v", got)
			}
			if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got.Count != 4 {
				t.Fatalf("被拒绝的移动修改了玩家物品: %+v", got)
			}
		})
	}
}
