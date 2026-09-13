package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// TestQuickMoveChestWholeStackToBackpackPickupPhases 验收 spec 场景「箱子
// 整堆快捷搬运回背包」：整堆按拾取四相位序并入背包——快捷栏空格先于背包
// 同类合并承接，箱子来源格清空。
func TestQuickMoveChestWholeStackToBackpackPickupPhases(t *testing.T) {
	var inventory core.Inventory
	inventory.Backpack[0] = core.ItemStack{Item: core.ItemStone, Count: 63}
	chest := world.ChestSlot{}
	chest.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
	openStackSplitChest(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		quickMoveCommand(session, 3, StackViewContainer, core.ChestFirstSlot, ref),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("箱子→背包被拒绝: %+v", result.Rejected)
	}
	got := engine.sessions[session].player.inventory
	// 四相位序：快捷栏没有同类石头，第二相位（快捷栏空格）先于第三相位
	//（背包同类合并）承接整堆。
	if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 64}) {
		t.Fatalf("快捷栏空格 = %+v，想要承接整堆 64", got.Hotbar.Slots[0])
	}
	if got.Backpack[0] != (core.ItemStack{Item: core.ItemStone, Count: 63}) {
		t.Fatalf("背包同类格 = %+v，想要保持 63 不变", got.Backpack[0])
	}
	if after := stackSplitChestAt(t, engine, ref).Items[0]; after != (core.ItemStack{}) {
		t.Fatalf("箱子来源格 = %+v，想要清空", after)
	}
}

// TestQuickMoveChestPartialAbsorptionKeepsRemainder 锁定箱子→背包的余量
// 留源：背包容量不足时部分吸收（非全有或全无），余量留在箱子来源格。
func TestQuickMoveChestPartialAbsorptionKeepsRemainder(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 62}
	quickMoveFillExcept(&inventory, []uint8{0}, core.ItemDirt)
	chest := world.ChestSlot{}
	chest.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
	openStackSplitChest(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		quickMoveCommand(session, 3, StackViewContainer, core.ChestFirstSlot, ref),
	})
	if len(result.Rejected) != 0 {
		t.Fatalf("部分吸收应成功而非拒绝: %+v", result.Rejected)
	}
	got := engine.sessions[session].player.inventory
	if got.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}) {
		t.Fatalf("同类合并格 = %+v，想要并到 64", got.Hotbar.Slots[0])
	}
	if after := stackSplitChestAt(t, engine, ref).Items[0]; after != (core.ItemStack{Item: core.ItemStone, Count: 62}) {
		t.Fatalf("箱子来源格 = %+v，想要余 62", after)
	}
}

// TestQuickMoveBackpackToChestFirstFitting 验收背包→箱子方向：统一索引
// 36..62 升序进首个「空或同类未满」格（异类与同类满格跳过），余量留源。
func TestQuickMoveBackpackToChestFirstFitting(t *testing.T) {
	t.Run("异类跳过同类未满承接", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
		chest := world.ChestSlot{}
		chest.Items[0] = core.ItemStack{Item: core.ItemDirt, Count: 10}
		chest.Items[1] = core.ItemStack{Item: core.ItemStone, Count: 32}
		engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
		openStackSplitChest(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			quickMoveCommand(session, 3, StackViewContainer, 0, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("背包→箱子被拒绝: %+v", result.Rejected)
		}
		after := stackSplitChestAt(t, engine, ref)
		if after.Items[0] != (core.ItemStack{Item: core.ItemDirt, Count: 10}) {
			t.Fatalf("异类箱子格被改动: %+v", after.Items[0])
		}
		if after.Items[1] != (core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}) {
			t.Fatalf("同类未满格 = %+v，想要并到 64", after.Items[1])
		}
		if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 32}) {
			t.Fatalf("来源格 = %+v，想要余 32", got)
		}
	})
	t.Run("同类满格跳过空格承接", func(t *testing.T) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
		chest := world.ChestSlot{}
		chest.Items[0] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}
		engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
		openStackSplitChest(t, engine, session)

		result := finishPlayerWorldTick(engine, []Command{
			quickMoveCommand(session, 3, StackViewContainer, 0, ref),
		})
		if len(result.Rejected) != 0 {
			t.Fatalf("空格承接被拒绝: %+v", result.Rejected)
		}
		after := stackSplitChestAt(t, engine, ref)
		if after.Items[0] != (core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount}) {
			t.Fatalf("同类满格 = %+v，想要保持 64", after.Items[0])
		}
		if after.Items[1] != (core.ItemStack{Item: core.ItemStone, Count: 64}) {
			t.Fatalf("空箱子格 = %+v，想要承接整堆", after.Items[1])
		}
		if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{}) {
			t.Fatalf("来源格 = %+v，想要清空", got)
		}
	})
}

// TestQuickMoveChestRejectsWhenNoFit 验收 spec 场景「无可容纳位置整单拒绝」：
// 箱子 27 格全部为异类非空时背包来源的快捷搬运整单 RejectInvalidInput 且
// 两侧逐格零变化。
func TestQuickMoveChestRejectsWhenNoFit(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	chest := world.ChestSlot{}
	for slot := range chest.Items {
		chest.Items[slot] = core.ItemStack{Item: core.ItemDirt, Count: 10}
	}
	engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
	openStackSplitChest(t, engine, session)
	before := stackSplitChestAt(t, engine, ref)

	result := finishPlayerWorldTick(engine, []Command{
		quickMoveCommand(session, 3, StackViewContainer, 0, ref),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("无可容纳 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := stackSplitChestAt(t, engine, ref); got != before {
		t.Fatalf("被拒绝的移动修改了箱子: %+v", got)
	}
	if got := engine.sessions[session].player.inventory; got != inventory {
		t.Fatalf("被拒绝的移动修改了玩家物品: %+v", got)
	}
}

// TestQuickMoveChestRejectsBrokenRepackInvariant 锁定箱子→背包增量的回收
// 预演：整堆并入（54 并入同类 + 10 占用唯一空格）若挤占网格回收依赖的
// 唯一空格，整体拒绝且两侧逐格不变。
func TestQuickMoveChestRejectsBrokenRepackInvariant(t *testing.T) {
	chest := world.ChestSlot{}
	chest.Items[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	engine, session, ref := stackSplitChestFixture(
		t, stackSplitContainerRepackInventory(), chest, stackSplitContainerRepackGrid(),
	)
	openStackSplitChest(t, engine, session)

	result := finishPlayerWorldTick(engine, []Command{
		quickMoveCommand(session, 3, StackViewContainer, core.ChestFirstSlot, ref),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("破坏回收不变量的移动 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 64}) {
		t.Fatalf("被拒绝的移动修改了箱子: %+v", got)
	}
	player := engine.sessions[session].player
	if player.inventory.Backpack[0] != (core.ItemStack{}) ||
		player.inventory.Hotbar.Slots[0] != (core.ItemStack{Item: core.ItemStone, Count: 10}) {
		t.Fatalf("被拒绝的移动修改了背包: %+v", player.inventory)
	}
	if player.crafting.Slots[0] != (core.ItemStack{Item: core.ItemDirt, Count: 30}) {
		t.Fatalf("被拒绝的移动修改了网格: %+v", player.crafting)
	}
}

// TestQuickMoveChestSettlesInDeferredChunkPhase 锁定结算相位：容器视图的
// 快捷搬运在命令阶段只入队不结算，权威状态在区块写相位才变化。
func TestQuickMoveChestSettlesInDeferredChunkPhase(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
	chest := world.ChestSlot{}
	chest.Items[0] = core.ItemStack{Item: core.ItemDirt, Count: 10}
	engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
	openStackSplitChest(t, engine, session)
	command := quickMoveCommand(session, 3, StackViewContainer, 0, ref)

	// 只运行命令阶段：延迟入队的搬运既不结算也不产生拒绝，状态零变化。
	preliminary := applyPlayerCommandsTick(engine, []Command{command})
	if len(preliminary.Rejected) != 0 {
		t.Fatalf("命令阶段不应拒绝容器域搬运: %+v", preliminary.Rejected)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got.Count != 64 {
		t.Fatalf("命令阶段提前结算: %+v", got)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[1]; got != (core.ItemStack{}) {
		t.Fatalf("命令阶段提前写箱子: %+v", got)
	}

	// 同一命令经完整相位链（命令收集 + FinishWorld 区块写相位）结算。
	settled := finishPlayerWorldTick(engine, []Command{command})
	if len(settled.Rejected) != 0 {
		t.Fatalf("区块写相位结算被拒绝: %+v", settled.Rejected)
	}
	if got := stackSplitChestAt(t, engine, ref).Items[1]; got != (core.ItemStack{Item: core.ItemStone, Count: 64}) {
		t.Fatalf("箱子承接格 = %+v，想要整堆 64", got)
	}
	if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{}) {
		t.Fatalf("来源格 = %+v，想要清空", got)
	}
}

// TestQuickMoveChestValueDomainRejects 锁定箱子域值域与查看关系：统一视图
// 上界、空源、未建立查看关系与过期引用都按 RejectInvalidInput 整单拒绝。
func TestQuickMoveChestValueDomainRejects(t *testing.T) {
	var inventory core.Inventory
	inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
	engine, session, ref := stackSplitChestFixture(t, inventory, world.ChestSlot{}, CraftingGrid{})
	openStackSplitChest(t, engine, session)

	for name, command := range map[string]Command{
		"统一视图上界": quickMoveCommand(session, 3, StackViewContainer, core.ChestViewSlots, ref),
		"空源箱子格":  quickMoveCommand(session, 3, StackViewContainer, core.ChestFirstSlot+5, ref),
		"过期引用": quickMoveCommand(session, 3, StackViewContainer, 0, core.FurnaceRef{
			Dimension: ref.Dimension, Chunk: ref.Chunk, Kind: ref.Kind,
			Slot: ref.Slot, Generation: ref.Generation + 1,
		}),
	} {
		t.Run(name, func(t *testing.T) {
			result := finishPlayerWorldTick(engine, []Command{command})
			if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
				t.Fatalf("result=%+v，想要恰好一次 RejectInvalidInput", result)
			}
			if got := stackSplitChestAt(t, engine, ref).Items[0]; got != (core.ItemStack{}) {
				t.Fatalf("被拒绝的搬运修改了箱子: %+v", got)
			}
			if got := engine.sessions[session].player.inventory.Hotbar.Slots[0]; got != (core.ItemStack{Item: core.ItemStone, Count: 7}) {
				t.Fatalf("被拒绝的搬运修改了玩家物品: %+v", got)
			}
		})
	}

	// 未建立查看关系（未 open）时用独立夹具验证。
	freshEngine, freshSession, freshRef := stackSplitChestFixture(
		t, inventory, world.ChestSlot{}, CraftingGrid{},
	)
	result := finishPlayerWorldTick(freshEngine, []Command{
		quickMoveCommand(freshSession, 3, StackViewContainer, 0, freshRef),
	})
	if len(result.Rejected) != 1 || result.Rejected[0].Reason != RejectInvalidInput {
		t.Fatalf("未查看容器 result=%+v，想要恰好一次 RejectInvalidInput", result)
	}
}

// TestQuickMoveDeterministicReplay 验收 spec 场景「目标序确定可重放」：相同
// 栏位布局与请求序列在两个独立引擎上逐步重放，落位与余量逐格一致（覆盖
// 纯背包互移与箱子双向快捷搬运）。
func TestQuickMoveDeterministicReplay(t *testing.T) {
	run := func() (core.Inventory, world.ChestSlot) {
		var inventory core.Inventory
		inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 7}
		inventory.Backpack[0] = core.ItemStack{Item: core.ItemStone, Count: 61}
		inventory.Backpack[1] = core.ItemStack{Item: core.ItemCoal, Count: 5}
		chest := world.ChestSlot{}
		chest.Items[2] = core.ItemStack{Item: core.ItemStone, Count: core.MaxStackCount - 1}
		chest.Items[5] = core.ItemStack{Item: core.ItemDirt, Count: 10}
		engine, session, ref := stackSplitChestFixture(t, inventory, chest, CraftingGrid{})
		openStackSplitChest(t, engine, session)
		commands := []Command{
			quickMoveCommand(session, 3, StackViewContainer, 0, ref),
			quickMoveCommand(session, 4, StackViewContainer, core.ChestFirstSlot+2, ref),
			quickMoveCommand(session, 5, StackViewInventory, 9, core.FurnaceRef{}),
		}
		for index, command := range commands {
			var result TickResult
			if command.StackView == StackViewContainer {
				result = finishPlayerWorldTick(engine, []Command{command})
			} else {
				result = applyPlayerCommandsTick(engine, []Command{command})
			}
			if len(result.Rejected) != 0 {
				t.Fatalf("第 %d 步被拒绝: %+v", index, result.Rejected)
			}
		}
		return engine.sessions[session].player.inventory, stackSplitChestAt(t, engine, ref)
	}
	firstInventory, firstChest := run()
	secondInventory, secondChest := run()
	if firstInventory != secondInventory || firstChest != secondChest {
		t.Fatalf("重放不一致: inventory=%+v/%+v chest=%+v/%+v",
			firstInventory, secondInventory, firstChest, secondChest)
	}
}
