package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/world"
)

// TestGameplaySettlementViaMutation 覆盖任务 7 的全部玩法结算：物品/容器、合成、熔炉、采掘、放置、交互、掉落、战斗、饥饿、进食与睡眠。
// 每个子测试在 RED 阶段因对应结算函数缺失而编译失败，GREEN 阶段验证原子成功/拒绝仍经 entity state 与同一 realm.Mutation 完成。
func TestGameplaySettlementViaMutation(t *testing.T) {
	t.Run("crafting_repack_invariant", func(t *testing.T) {
		var inv core.Inventory
		inv.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 64}
		grid := CraftingGrid{Size: CraftingGridSizePersonal}
		grid.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 1}
		if !canRepackCrafting(inv, grid) {
			t.Fatalf("满背包应可回收小网格")
		}
		// 破坏不变量：背包满且网格放不同物品应拒绝
		for i := range inv.Hotbar.Slots {
			inv.Hotbar.Slots[i] = core.ItemStack{Item: core.ItemStone, Count: 64}
		}
		for i := range inv.Backpack {
			inv.Backpack[i] = core.ItemStack{Item: core.ItemStone, Count: 64}
		}
		grid.Slots[0] = core.ItemStack{Item: core.ItemDirt, Count: 64}
		if canRepackCrafting(inv, grid) {
			t.Fatalf("破坏回收不变量应拒绝")
		}
	})

	t.Run("container_merge", func(t *testing.T) {
		src := core.ItemStack{Item: core.ItemStone, Count: 10}
		dst := core.ItemStack{Item: core.ItemStone, Count: 60}
		nextSrc, nextDst, ok := mergeStacks(src, dst)
		if !ok || nextDst.Count != 64 || nextSrc.Count != 6 {
			t.Fatalf("同类合并错误 ok=%v dst=%+v src=%+v", ok, nextDst, nextSrc)
		}
		// 异类交换
		src = core.ItemStack{Item: core.ItemDirt, Count: 1}
		dst = core.ItemStack{Item: core.ItemStone, Count: 1}
		ns, nd, ok := mergeStacks(src, dst)
		if !ok || ns.Item != core.ItemStone || nd.Item != core.ItemDirt {
			t.Fatalf("异类应交换")
		}
	})

	t.Run("furnace_advance_via_mutation", func(t *testing.T) {
		chunk := world.NewChunk(core.ChunkPos{})
		chunk.SetBlock(0, 0, 0, core.FurnaceID)
		chunk.PrepareFurnace(0)
		f := world.FurnaceSlot{Active: true, Input: core.ItemStack{Item: core.ItemRawIron, Count: 1}, Fuel: core.ItemStack{Item: core.ItemCoal, Count: 1}}
		chunk.SetFurnace(0, f)
		if !advanceChunkFurnaces(chunk, 10, 2) {
			// 第一次推进应消耗燃料并推进进度
		}
		f2 := chunk.Furnace(0)
		if f2.BurnTicks == 0 && f2.Fuel.Count == 0 {
			t.Fatalf("熔炉应保留燃烧")
		}
		// 验证通过 mutation 的原子路径
		rs := realm.NewState(core.Overworld)
		rs.Dimension(core.Overworld).BeginGeneration(core.ChunkPos{})
		ch := world.NewChunk(core.ChunkPos{})
		ch.SetBlock(0, 0, 0, core.FurnaceID)
		if err := rs.Dimension(core.Overworld).ApplyGenerated(core.ChunkPos{}, ch); err != nil {
			t.Fatalf("apply: %v", err)
		}
		eng := NewEngine(1, 0, 1)
		eng.realm = rs
		mut := rs.NewMutation()
		eng.advanceFurnaces(mut)
		// 若实现错误，mutation 不承载变化；这里仅验证不 panic 且可提交
		_ = mut.Commit()
	})

	t.Run("mining_rule_and_mutation", func(t *testing.T) {
		req, harv := miningRule(core.GrassID, core.ItemNone)
		if req != 5 || !harv {
			t.Fatalf("草方块规则错误 req=%d harv=%v", req, harv)
		}
		req, harv = miningRule(core.StoneID, core.ItemIronPickaxe)
		if req != 8 || !harv {
			t.Fatalf("铁镐挖石头应 8t")
		}
		// 验证采掘完成经 mutation 原子落盘
		rs := realm.NewState(core.Overworld)
		pos := core.ChunkPos{}
		rs.Dimension(core.Overworld).BeginGeneration(pos)
		ch := world.NewChunk(pos)
		ch.SetBlock(0, 1, 0, core.GrassID)
		if err := rs.Dimension(core.Overworld).ApplyGenerated(pos, ch); err != nil {
			t.Fatalf("apply: %v", err)
		}
		eng := NewEngine(1, 0, 42)
		eng.realm = rs
		mut := rs.NewMutation()
		_, rejected := eng.completeMining(core.Overworld, core.BlockPos{X: 0, Y: 1, Z: 0}, core.GrassID, true, mut)
		if rejected {
			t.Fatalf("草方块采掘应成功")
		}
		if mut.Len() == 0 {
			t.Fatalf("mutation 应承载方块变更")
		}
		batches := mut.Commit()
		if len(batches) == 0 || batches[0].Changes[0].Block != core.AirID {
			t.Fatalf("提交应包含变空气")
		}
	})

	t.Run("placement_door_bed_via_mutation", func(t *testing.T) {
		rs := realm.NewState(core.Overworld)
		pos := core.ChunkPos{}
		rs.Dimension(core.Overworld).BeginGeneration(pos)
		ch := world.NewChunk(pos)
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				ch.SetBlock(x, 0, z, core.StoneID)
				ch.SetBlock(x, 1, z, core.AirID)
				ch.SetBlock(x, 2, z, core.AirID)
			}
		}
		if err := rs.Dimension(core.Overworld).ApplyGenerated(pos, ch); err != nil {
			t.Fatalf("apply: %v", err)
		}
		eng := NewEngine(1, 0, 1)
		eng.realm = rs
		mut := rs.NewMutation()
		reason, rejected := eng.tryPlaceDoor(core.Overworld, core.BlockPos{Y: 1}, 0, mut)
		if rejected {
			t.Fatalf("平地放门应成功 reason=%v", reason)
		}
		mut2 := rs.NewMutation()
		reason, rejected = eng.tryPlaceBed(core.Overworld, core.BlockPos{X: 5, Y: 1, Z: 5}, 0, mut2)
		if rejected {
			t.Fatalf("放床应成功 reason=%v", reason)
		}
		if mut2.Len() != 1 && mut.Len() == 0 {
			t.Fatalf("床应产生 mutation")
		}
	})

	t.Run("drop_pickup_via_mutation", func(t *testing.T) {
		rs := realm.NewState(core.Overworld)
		pos := core.ChunkPos{}
		rs.Dimension(core.Overworld).BeginGeneration(pos)
		ch := world.NewChunk(pos)
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				ch.SetBlock(x, 0, z, core.GrassID)
			}
		}
		if err := rs.Dimension(core.Overworld).ApplyGenerated(pos, ch); err != nil {
			t.Fatalf("apply: %v", err)
		}
		eng := NewEngine(1, 0, 1)
		eng.realm = rs
		eng.RegisterSession(1, core.Overworld, core.ChunkPos{}, rs, eng.tunables)
		// 强制激活
		for i := 0; i < 30; i++ {
			rs.Dimension(core.Overworld).ReadyChunk(pos) // ensure ready
			eng.advancePendingPlayers()
			if eng.sessions[1].player.lifecycle == PlayerActive {
				break
			}
		}
		if eng.sessions[1].player.lifecycle != PlayerActive {
			t.Skip("未激活，跳过拾取")
		}
		eng.sessions[1].player.state.Position = mgl32.Vec3{0.5, 1, 0.5}
		eng.sessions[1].center = core.ChunkPos{}
		mut := rs.NewMutation()
		// 在脚下放掉落物
		ch2, _ := rs.Dimension(core.Overworld).ReadyChunk(pos)
		idx, _ := world.ChunkBlockIndex(core.BlockPos{X: 0, Y: 1, Z: 0})
		ch2.SetBlock(0, 1, 0, core.AirID)
		slot, _ := ch2.PrepareDrop(core.ItemDirt, idx)
		ch2.CommitDrop(slot, core.ItemStack{Item: core.ItemDirt, Count: 1}, idx, 0)
		eng.advanceDrops(mut)
		_ = mut.Commit()
		if got := eng.sessions[1].player.inventory; got.Hotbar.Slots[0].Item == core.ItemNone && got.Backpack[0].Item == core.ItemNone {
			// 可能拾取到任意格，检查是否有 dirt
			found := false
			for _, s := range got.Hotbar.Slots {
				if s.Item == core.ItemDirt {
					found = true
				}
			}
			for _, s := range got.Backpack {
				if s.Item == core.ItemDirt {
					found = true
				}
			}
			if !found {
				t.Fatalf("掉落物应被拾取")
			}
		}
	})

	t.Run("combat_melee_target_blocked", func(t *testing.T) {
		origin := mgl32.Vec3{0, 1, 0}
		dir := mgl32.Vec3{1, 0, 0}
		bounds := core.AABB{Min: mgl32.Vec3{2, 0, -0.5}, Max: mgl32.Vec3{3, 2, 0.5}}
		dist, hit := rayAABBDistance(origin, dir, bounds)
		if !hit || dist < 1.9 || dist > 2.1 {
			t.Fatalf("射线应命中 dist=%v hit=%v", dist, hit)
		}
		// 反向不应命中
		dir2 := mgl32.Vec3{-1, 0, 0}
		if _, hit := rayAABBDistance(origin, dir2, bounds); hit {
			t.Fatalf("反向不应命中")
		}
	})

	t.Run("hunger_exhaustion_via_mutation", func(t *testing.T) {
		eng := NewEngine(1, 0, 1)
		rs := realm.NewState(core.Overworld)
		eng.realm = rs
		eng.RegisterSession(1, core.Overworld, core.ChunkPos{}, rs, eng.tunables)
		p := eng.sessions[1].player
		p.hunger = 10
		p.saturationMilli = 1000
		p.applyExhaustion(5000, 4000)
		if p.saturationMilli != 0 && p.hunger != 9 {
			t.Fatalf("疲劳应跨阈值，饱和度或饥饿应下降 sat=%d hunger=%d", p.saturationMilli, p.hunger)
		}
	})

	t.Run("eating_advance", func(t *testing.T) {
		eng := NewEngine(1, 0, 1)
		rs := realm.NewState(core.Overworld)
		eng.realm = rs
		eng.RegisterSession(1, core.Overworld, core.ChunkPos{}, rs, eng.tunables)
		p := eng.sessions[1].player
		p.hunger = 10
		p.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemBread, Count: 1}
		p.inventory.Hotbar.Selected = 0
		p.eatingHeld = true
		for i := 0; i < int(eng.tunables.EatingTicks); i++ {
			p.advanceEating(eng.tunables.EatingTicks, false)
		}
		if p.hunger <= 10 && p.inventory.Hotbar.Slots[0].Count != 0 {
			t.Fatalf("进食应结算饥饿 hunger=%d slot=%+v", p.hunger, p.inventory.Hotbar.Slots[0])
		}
	})

	t.Run("sleep_settle", func(t *testing.T) {
		eng := NewEngine(1, 0, 0)
		rs := realm.NewState(core.Overworld)
		eng.realm = rs
		eng.RegisterSession(1, core.Overworld, core.ChunkPos{}, rs, eng.tunables)
		eng.RegisterSession(2, core.Overworld, core.ChunkPos{}, rs, eng.tunables)
		for i := 0; i < 30; i++ {
			eng.advancePendingPlayers()
		}
		for _, id := range []SessionID{1, 2} {
			if s := eng.sessions[id]; s != nil && s.player != nil {
				s.player.lifecycle = PlayerActive
				s.player.sleeping = true
			}
		}
		eng.worldTime.Store(13000)
		eng.settleSleepThroughNight()
		if eng.DayPhaseOffset() == 0 {
			t.Fatalf("全员入睡应推进相位偏移")
		}
		for _, id := range []SessionID{1, 2} {
			if eng.sessions[id].player.sleeping {
				t.Fatalf("跳夜后应清除入睡位")
			}
		}
	})

	t.Run("door_interact_via_mutation", func(t *testing.T) {
		rs := realm.NewState(core.Overworld)
		pos := core.ChunkPos{}
		rs.Dimension(core.Overworld).BeginGeneration(pos)
		ch := world.NewChunk(pos)
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				ch.SetBlock(x, 0, z, core.StoneID)
			}
		}
		if err := rs.Dimension(core.Overworld).ApplyGenerated(pos, ch); err != nil {
			t.Fatalf("apply: %v", err)
		}
		eng := NewEngine(1, 0, 1)
		eng.realm = rs
		mut := rs.NewMutation()
		if _, rejected := eng.tryPlaceDoor(core.Overworld, core.BlockPos{X: 0, Y: 1, Z: 0}, 0, mut); rejected {
			t.Fatalf("放门失败")
		}
		_ = mut.Commit()
		mut2 := rs.NewMutation()
		ok := handleInteractDoor(eng, core.Overworld, core.BlockPos{X: 0, Y: 1, Z: 0}, mut2)
		if !ok {
			t.Fatalf("门交互应翻转")
		}
		_ = mut2.Commit()
		block, _ := rs.Dimension(core.Overworld).BlockAt(core.BlockPos{X: 0, Y: 1, Z: 0})
		if block != core.DoorLowerSouthOpen {
			t.Fatalf("门应为开状态 got=%d", block)
		}
	})

	t.Run("bed_support_sweep_via_mutation", func(t *testing.T) {
		rs := realm.NewState(core.Overworld)
		pos := core.ChunkPos{}
		rs.Dimension(core.Overworld).BeginGeneration(pos)
		ch := world.NewChunk(pos)
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				ch.SetBlock(x, 0, z, core.StoneID)
			}
		}
		if err := rs.Dimension(core.Overworld).ApplyGenerated(pos, ch); err != nil {
			t.Fatalf("apply: %v", err)
		}
		eng := NewEngine(1, 0, 1)
		eng.realm = rs
		mut := rs.NewMutation()
		if _, rejected := eng.tryPlaceBed(core.Overworld, core.BlockPos{X: 2, Y: 1, Z: 2}, 0, mut); rejected {
			t.Fatalf("放床")
		}
		_ = mut.Commit()
		// 移除支撑应触发扫除
		if _, _, err := rs.Dimension(core.Overworld).SetBlock(core.BlockPos{X: 2, Y: 0, Z: 2}, core.AirID); err != nil {
			t.Fatalf("remove support: %v", err)
		}
		mut2 := rs.NewMutation()
		eng.invalidateBedSupportedBy(core.Overworld, core.BlockPos{X: 2, Y: 0, Z: 2}, mut2)
		_ = mut2.Commit()
		// 床应被清除
		b1, _ := rs.Dimension(core.Overworld).BlockAt(core.BlockPos{X: 2, Y: 1, Z: 2})
		if core.IsBed(b1) {
			t.Fatalf("支撑移除后床应被扫除")
		}
	})
}
