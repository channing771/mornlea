package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

// 本文件锁定夜行者的生命周期契约：白昼露天灼烧每 20 tick 扣 1 且遮顶/夜间
// 重置计时、距全部 active 玩家水平 >64 格累计 600 active tick 后无掉落移除
// （回到范围内清零）、死亡在同一权威 tick 移除并经既有掉落契约环形尝试放置
// 1 个腐肉（全满时确定性省略掉落仍完成死亡）。

// testDayTick / testNightTick 是夹具用的显示相位代表值（本行 offset 恒 0）。
const (
	testDayTick   uint64 = 1000
	testNightTick uint64 = 15000
)

// countLoadedDrops 统计已加载区块中指定物品的激活掉落堆总数。
func countLoadedDrops(t *testing.T, engine *Engine, item core.ItemID) int {
	t.Helper()
	count := 0
	dimension := engine.dimension(core.Overworld)
	for _, pos := range dimension.ReadyChunkPositions(nil) {
		chunk, ready := dimension.ReadyChunk(pos)
		if !ready {
			continue
		}
		for slot := range core.DropsPerChunk {
			if drop := chunk.Drop(slot); drop.Active && drop.Stack.Item == item {
				count++
			}
		}
	}
	return count
}

func TestHostileBurnDamagesEveryTwentyTicksWhenExposed(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(21)
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	// 白昼露天：第 20 tick 首次扣 1，第 40 tick 再扣 1，计时连续。
	for range 19 {
		engine.advanceHostileBurn(testDayTick)
	}
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth {
		t.Fatalf("前 19 tick 生命=%d，想要仍为 %d", got, core.MaxHealth)
	}
	engine.advanceHostileBurn(testDayTick)
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth-1 {
		t.Fatalf("第 20 tick 生命=%d，想要 %d", got, core.MaxHealth-1)
	}
	for range 20 {
		engine.advanceHostileBurn(testDayTick)
	}
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth-2 {
		t.Fatalf("第 40 tick 生命=%d，想要 %d", got, core.MaxHealth-2)
	}
}

func TestHostileBurnTimerResetsWhenCovered(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(22)
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	// 先露天烧 10 tick（计时过半），再加盖遮顶推进 60 tick。
	for range 10 {
		engine.advanceHostileBurn(testDayTick)
	}
	if got := engine.hostiles.entries[0].burnCooldown; got != 10 {
		t.Fatalf("灼烧剩余=%d，想要 10", got)
	}
	engine.SetBlockForTest(core.BlockPos{X: 2, Y: 3, Z: 2}, core.StoneID)
	for range 60 {
		engine.advanceHostileBurn(testDayTick)
	}
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth {
		t.Fatalf("遮顶期间生命=%d，想要不受灼烧影响", got)
	}
	if got := engine.hostiles.entries[0].burnCooldown; got != hostileCooldownPeriodTicks {
		t.Fatalf("遮顶后灼烧剩余=%d，想要重置为满周期（新计时从 0 开始）", got)
	}
	// 重新露天：满周期走完才会再次扣血。
	engine.SetBlockForTest(core.BlockPos{X: 2, Y: 3, Z: 2}, core.AirID)
	for range 19 {
		engine.advanceHostileBurn(testDayTick)
	}
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth {
		t.Fatalf("重置后前 19 tick 生命=%d，想要不变", got)
	}
	engine.advanceHostileBurn(testDayTick)
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth-1 {
		t.Fatalf("重置后第 20 tick 生命=%d，想要 %d", got, core.MaxHealth-1)
	}
}

func TestHostileBurnTimerResetsAtNight(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(23)
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	// 白昼烧 5 tick 后入夜：夜间不扣血且计时重置。
	for range 5 {
		engine.advanceHostileBurn(testDayTick)
	}
	engine.advanceHostileBurn(testNightTick)
	engine.advanceHostileBurn(testNightTick)
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth {
		t.Fatalf("夜间生命=%d，想要不变", got)
	}
	if got := engine.hostiles.entries[0].burnCooldown; got != hostileCooldownPeriodTicks {
		t.Fatalf("夜间灼烧剩余=%d，想要重置为满周期", got)
	}
}

func TestHostileDistantDespawnAfterSixHundredActiveTicks(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 玩家在原点，夜行者水平 80 格（>64）且远离累计已到 599：再推进 1 tick
	// 必须移除且不产生任何掉落。
	mob := validTestHostile(24)
	mob.State.Position = mgl32.Vec3{80.5, 1, 0.5}
	mob.DistantTicks = 599
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	engine.advanceHostileDistant()
	if len(engine.hostiles.entries) != 0 {
		t.Fatalf("远离 600 tick 后夜行者仍在（数量=%d）", len(engine.hostiles.entries))
	}
	if got := countLoadedDrops(t, engine, core.ItemRottenFlesh); got != 0 {
		t.Fatalf("远离消失产生了 %d 个掉落物，想要 0", got)
	}
}

func TestHostileDistantCounterResetsWithinRange(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 恰好 64 格：在范围内（>64 才累计），599 的累计被清零并保留。
	mob := validTestHostile(25)
	mob.State.Position = mgl32.Vec3{64.5, 1, 0.5}
	mob.DistantTicks = 599
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	engine.advanceHostileDistant()
	entry := &engine.hostiles.entries[0]
	if entry.id != 25 {
		t.Fatal("范围内夜行者被移除，想要保留")
	}
	if entry.distantTicks != 0 {
		t.Fatalf("回到范围内累计=%d，想要清零", entry.distantTicks)
	}

	// 65 格（>64）：同 tick 起开始累计。
	engine.hostiles.entries[0].state.Position = mgl32.Vec3{65.5, 1, 0.5}
	engine.advanceHostileDistant()
	if got := engine.hostiles.entries[0].distantTicks; got != 1 {
		t.Fatalf("超出 64 格后的累计=%d，想要 1", got)
	}
}

func TestHostileBurnDeathDropsSingleRottenFlesh(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(26)
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	mob.Health = 1
	mob.BurnCooldown = 1
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	// 生命 1、灼烧计时到期：同一 tick 内灼烧致死 + 移除 + 掉落。
	engine.advanceHostileBurn(testDayTick)
	pending := engine.newMutation()
	engine.settleHostileDeaths(pending)
	if len(engine.hostiles.entries) != 0 {
		t.Fatal("死亡后夜行者仍留在集合中，想要同 tick 移除")
	}
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 0, Z: 0}}
	if !pending.Has(key) {
		t.Fatal("死亡掉落未登记区块变更 barrier")
	}
	if got := countLoadedDrops(t, engine, core.ItemRottenFlesh); got != 1 {
		t.Fatalf("死亡掉落腐肉=%d，想要恰好 1", got)
	}
}

func TestHostileDeathDropRingsToNeighborChunkWhenDeathChunkFull(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 装载 3×3 邻域并填满死亡区块（原点）的全部掉落槽。
	loadFlatChunks(t, engine.dimension(core.Overworld), -1, 1, -1, 1)
	dimension := engine.dimension(core.Overworld)
	origin, ready := dimension.ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("origin chunk is not ready")
	}
	for slot := range core.DropsPerChunk {
		origin.SetDrop(slot, world.DropSlot{
			Generation: 1,
			Active:     true,
			Stack:      core.ItemStack{Item: core.ItemTorch, Count: 1},
			BlockIndex: uint32(slot),
		})
	}
	mob := validTestHostile(27)
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	// 死亡结算的入口条件是生命归零；恢复校验只收健康个体，这里直接置零
	// 等价于灼烧/攻击刚好扣完的瞬间。
	engine.hostiles.entries[0].health = 0
	pending := engine.newMutation()
	engine.settleHostileDeaths(pending)
	if len(engine.hostiles.entries) != 0 {
		t.Fatal("掉落槽全满时死亡未完成，想要仍移除")
	}
	if got := countLoadedDrops(t, engine, core.ItemRottenFlesh); got != 1 {
		t.Fatalf("环形尝试后腐肉=%d，想要恰好 1", got)
	}
	neighbor := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -1, Z: -1}}
	if !pending.Has(neighbor) {
		t.Fatal("环形掉落未在邻接区块登记变更 barrier")
	}
}

func TestHostileDeathWithAllSlotsFullOmitsDropDeterministically(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 3×3 邻域全部填满：死亡完成但确定性省略掉落，且不动任何已有物品。
	loadFlatChunks(t, engine.dimension(core.Overworld), -1, 1, -1, 1)
	dimension := engine.dimension(core.Overworld)
	for _, pos := range dimension.ReadyChunkPositions(nil) {
		chunk, ready := dimension.ReadyChunk(pos)
		if !ready {
			continue
		}
		for slot := range core.DropsPerChunk {
			chunk.SetDrop(slot, world.DropSlot{
				Generation: 1,
				Active:     true,
				Stack:      core.ItemStack{Item: core.ItemTorch, Count: 1},
				BlockIndex: uint32(slot),
			})
		}
	}
	mob := validTestHostile(28)
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	engine.hostiles.entries[0].health = 0
	engine.settleHostileDeaths(engine.newMutation())
	if len(engine.hostiles.entries) != 0 {
		t.Fatal("全满时死亡未完成，想要仍移除")
	}
	if got := countLoadedDrops(t, engine, core.ItemRottenFlesh); got != 0 {
		t.Fatalf("全满时出现了 %d 个腐肉，想要 0", got)
	}
	if got := countLoadedDrops(t, engine, core.ItemTorch); got != 9*int(core.DropsPerChunk) {
		t.Fatalf("既有掉落物被破坏：火把堆=%d，想要 %d", got, 9*int(core.DropsPerChunk))
	}
}

func TestHostileFallingBelowWorldIsRemoved(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 挖穿脚底支撑：夜行者坠出世界后没有玩家那样的重生路径，必须在落地
	// 无望时被确定性移除，避免留下无法持久化的身体。
	engine.SetBlockForTest(core.BlockPos{X: 2, Y: 0, Z: 2}, core.AirID)
	mob := validTestHostile(29)
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	mob.State.OnGround = true
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	for range 200 {
		engine.advanceHostileMovement()
		if len(engine.hostiles.entries) == 0 {
			return
		}
	}
	t.Fatal("坠出世界的夜行者 200 tick 内未被移除")
}

// 坠落移除阈值与世界下界本体对齐（Y < core.MinY 即移除）：夜行者坠落会滞留
// 而玩家会重生，世界下界以下的任何位置都无法通过存档记录校验——若允许个体
// 滞留在 [MinY-16, MinY) 窗口，autosave 会把不可持久化的位置写进存档，重启
// 恢复因此整体失败。本用例逐 tick 观察：首次越过下界的同一 tick 必须已经
// 移除，绝不允许再往下多走一格。
func TestHostileFallingRemovedAtWorldFloorNotBelow(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	engine.SetBlockForTest(core.BlockPos{X: 2, Y: 0, Z: 2}, core.AirID)
	mob := validTestHostile(32)
	mob.State.Position = mgl32.Vec3{2.5, 1, 2.5}
	mob.State.OnGround = true
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	for range 400 {
		engine.advanceHostileMovement()
		if len(engine.hostiles.entries) == 0 {
			return
		}
		if y := engine.hostiles.entries[0].state.Position.Y(); y < float32(core.MinY) {
			t.Fatalf("夜行者 Y=%v 已低于世界下界 %d 仍未移除，想要越界即移除", y, core.MinY)
		}
	}
	t.Fatal("坠出世界的夜行者 400 tick 内未被移除")
}

func TestHostileDistantDespawnUsesActivePlayersOfSameDimension(t *testing.T) {
	// 累计判据只看 active 玩家的实际位置：玩家在 64 格内清零，玩家离开后
	// 重新从 0 累计。
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(30)
	mob.State.Position = mgl32.Vec3{50.5, 1, 0.5}
	mob.DistantTicks = 598
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	// 玩家就在 64 格内：累计清零，不移除。
	engine.advanceHostileDistant()
	if got := engine.hostiles.entries[0].distantTicks; got != 0 {
		t.Fatalf("玩家在范围内累计=%d，想要清零", got)
	}
	// 玩家移出（>64）后累计从 0 重新开始推进。
	engine.sessions[1].player.state.Position = mgl32.Vec3{200.5, 1, 0.5}
	engine.advanceHostileDistant()
	if got := engine.hostiles.entries[0].distantTicks; got != 1 {
		t.Fatalf("玩家离开后累计=%d，想要 1", got)
	}
}
