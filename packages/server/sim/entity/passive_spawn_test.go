package entity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定被动牛的确定性昼间生成：锚点玩家按已排序 `active` 会话与
// `WorldTimeTicks % 会话数` 选取、`splitmix64` 整数派生半径与轴向（水平距离
// 24..48）、候选格为草方块正上方双格空气 + 下方 solid + 非流体 + 完整
// `loaded`、白昼窗口全部必要、全服不超过 32 头与每玩家 48 格内不超过 6 头、
// 每 tick 至多验证一个候选、相同输入重放逐位一致，以及 `id` 冲突重散列。

// passiveSpawnScan 自 `start` 起按 `step` 步长扫描至多 `limit` 个世界时间值，
// 返回第一个让引擎实际生成一头被动牛的 tick。每次推进至多产生一头，超量即
// 失败。
func passiveSpawnScan(t *testing.T, engine *Engine, start, step uint64, limit int) uint64 {
	t.Helper()
	for offset := range limit {
		tick := start + uint64(offset)*step
		engine.worldTime.Store(tick)
		before := len(engine.passives.entries)
		engine.advancePassiveSpawn()
		delta := len(engine.passives.entries) - before
		if delta > 1 {
			t.Fatalf("tick %d 单次生成判定生成了 %d 头被动牛，想要至多 1", tick, delta)
		}
		if delta == 1 {
			return tick
		}
	}
	t.Fatalf("自 %d 步长 %d 扫描 %d 个 tick 内没有可生成的候选", start, step, limit)
	return 0
}

// passiveCandidateColumn 探得当前唯一被动牛所在的候选列方块坐标（`flat` 地表
// 候选恒在 y=1）。坐标取 `floor`，与权威 `blockPosOf` 同一定位规则。
func passiveCandidateColumn(t *testing.T, engine *Engine) core.BlockPos {
	t.Helper()
	if len(engine.passives.entries) != 1 {
		t.Fatalf("探针夹具失效：被动牛数量=%d，想要 1", len(engine.passives.entries))
	}
	position := engine.passives.entries[0].state.Position
	return core.BlockPos{
		X: int32(math.Floor(float64(position.X()))),
		Y: 1,
		Z: int32(math.Floor(float64(position.Z()))),
	}
}

func TestPassiveSpawnOnlyDuringDay(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)

	// 非白昼相位一律不生成：同一相位连查三个昼夜周期。
	for _, phase := range []uint64{0, 12000, 12999, 13000, 15000, 23000, 23001, 23999} {
		for cycle := range 3 {
			engine.worldTime.Store(phase + uint64(cycle)*24000)
			before := len(engine.passives.entries)
			engine.advancePassiveSpawn()
			if len(engine.passives.entries) != before {
				t.Fatalf("显示相位 %d 生成了被动牛，想要拒绝", phase)
			}
		}
	}

	// 白昼两端（含边界）在竞技场就绪时必须可以生成。
	for _, phase := range []uint64{1, 11999} {
		clearPassivesForTest(engine)
		engine.worldTime.Store(phase)
		engine.advancePassiveSpawn()
		if len(engine.passives.entries) != 1 {
			t.Fatalf("显示相位 %d 未生成被动牛，想要生成", phase)
		}
	}
}

func TestPassiveSpawnPicksAnchorBySortedSessionAndWorldTime(t *testing.T) {
	engine, _ := readyMeleePlayers(t, 2)
	// 竞技场同时覆盖两个锚点的候选环：玩家 1 在原点、玩家 2 在 (40,0)。
	loadSpawnArena(t, engine, -48, 88, -48, 48)
	setMeleePlayer(engine, 1, mgl32.Vec3{0.5, 1, 0.5}, 0)
	setMeleePlayer(engine, 2, mgl32.Vec3{40.5, 1, 0.5}, 0)

	// 会话数为 2：偶数 tick 锚点是排序列表首位的会话 1（tick 2 为白昼）。
	clearPassivesForTest(engine)
	engine.worldTime.Store(2)
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 1 {
		t.Fatal("偶数白昼 tick 未生成被动牛，夹具失效")
	}
	assertAxisAlignedAtDistance(t, engine.passives.entries[0].state.Position, 0, 0)

	// 奇数 tick 锚点是会话 2：候选必须改为以 (40,0) 为锚（tick 3 为白昼）。
	clearPassivesForTest(engine)
	engine.worldTime.Store(3)
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 1 {
		t.Fatal("奇数白昼 tick 未生成被动牛，夹具失效")
	}
	assertAxisAlignedAtDistance(t, engine.passives.entries[0].state.Position, 40, 0)
}

func TestPassiveSpawnRequiresGrassSupport(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := passiveSpawnScan(t, engine, 1, 1, 400)
	column := passiveCandidateColumn(t, engine)

	// 支撑格换成石头（solid 但非草方块）：同一候选必须被拒绝。
	engine.SetBlockForTest(core.BlockPos{X: column.X, Y: 0, Z: column.Z}, core.StoneID)
	clearPassivesForTest(engine)
	engine.worldTime.Store(tick)
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 0 {
		t.Fatal("非草地支撑的候选被接受，想要拒绝")
	}
}

func TestPassiveSpawnRejectsFluidColumn(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := passiveSpawnScan(t, engine, 1, 1, 400)
	column := passiveCandidateColumn(t, engine)

	// 候选列灌两格水：候选格自身为流体，整列不再存在任何合法落点。
	engine.SetBlockForTest(column, core.WaterSourceID)
	engine.SetBlockForTest(core.BlockPos{X: column.X, Y: 2, Z: column.Z}, core.WaterSourceID)
	clearPassivesForTest(engine)
	engine.worldTime.Store(tick)
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 0 {
		t.Fatal("流体候选列被接受，想要拒绝")
	}
}

func TestPassiveSpawnRejectsSingleCellVerticalGap(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := passiveSpawnScan(t, engine, 1, 1, 400)
	column := passiveCandidateColumn(t, engine)

	// 整列填实后只在 y=100 挖出单格空气：高度 2 的竖向空间不成立，整列
	// 必须无落点。
	for y := int32(1); y < core.MaxY-1; y++ {
		engine.SetBlockForTest(core.BlockPos{X: column.X, Y: y, Z: column.Z}, core.StoneID)
	}
	engine.SetBlockForTest(core.BlockPos{X: column.X, Y: 100, Z: column.Z}, core.AirID)
	clearPassivesForTest(engine)
	engine.worldTime.Store(tick)
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 0 {
		t.Fatal("单格竖向空气被当成落点，想要拒绝")
	}
}

func TestPassiveSpawnRejectsUnloadedCandidateChunk(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	// 不装载任何候选区块：只有原点区块就绪，24..48 的候选必然落在未加载
	// 区块，绝不能为生成触发同步加载，只能拒绝。
	for offset := range 400 {
		engine.worldTime.Store(1 + uint64(offset))
		before := len(engine.passives.entries)
		engine.advancePassiveSpawn()
		if len(engine.passives.entries) != before {
			t.Fatalf("tick %d 在未加载区块生成被动牛", 1+offset)
		}
	}
}

func TestPassiveSpawnRejectsAtGlobalCap(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := passiveSpawnScan(t, engine, 1, 1, 400)

	// 先证夹具：同一 tick 在干净集合上确实生成。
	clearPassivesForTest(engine)
	engine.worldTime.Store(tick)
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 1 {
		t.Fatal("夹具失效：干净集合未能生成")
	}

	// 填满全服 32 头后同一 tick 必须拒绝。
	clearPassivesForTest(engine)
	for id := uint64(1); id <= 32; id++ {
		if err := engine.RestorePassive(validTestPassive(id)); err != nil {
			t.Fatalf("预置被动牛 %d：%v", id, err)
		}
	}
	engine.worldTime.Store(tick)
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 32 {
		t.Fatalf("满编后被动牛数量=%d，想要保持 32", len(engine.passives.entries))
	}
}

func TestPassiveSpawnRejectsSeventhNearAnchorPlayer(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := passiveSpawnScan(t, engine, 1, 1, 400)

	// 锚点玩家 48 格内已有 6 头：生成必须被拒绝。
	clearPassivesForTest(engine)
	for id := uint64(1); id <= 6; id++ {
		mob := validTestPassive(id)
		mob.State.Position = mgl32.Vec3{float32(id) + 0.5, 1, 0.5}
		if err := engine.RestorePassive(mob); err != nil {
			t.Fatalf("预置近处被动牛 %d：%v", id, err)
		}
	}
	engine.worldTime.Store(tick)
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 6 {
		t.Fatalf("第 7 头附近被动牛被接受：数量=%d", len(engine.passives.entries))
	}

	// 同样 6 头但全部挪到锚点 48 格之外：不再计入附近上限，同 tick 恢复生成。
	for index := range engine.passives.entries {
		engine.passives.entries[index].state.Position =
			mgl32.Vec3{100 + float32(index+1) + 0.5, 1, 0.5}
	}
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 7 {
		t.Fatalf("远处 6 头不应计入附近上限：数量=%d", len(engine.passives.entries))
	}
}

func TestPassiveSpawnRehashesConflictingID(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := passiveSpawnScan(t, engine, 1, 1, 400)
	derivedID := engine.passives.entries[0].id
	if derivedID == 0 {
		t.Fatal("派生 `id` 不应为零")
	}

	// 预置一头与派生 `id` 相同的被动牛：生成不得放弃也不得重复，必须重散列
	// 出新的唯一 `id`。
	clearPassivesForTest(engine)
	conflict := validTestPassive(derivedID)
	conflict.State.Position = mgl32.Vec3{100.5, 1, 100.5}
	if err := engine.RestorePassive(conflict); err != nil {
		t.Fatalf("预置冲突 `id`：%v", err)
	}
	engine.worldTime.Store(tick)
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 2 {
		t.Fatalf("`id` 冲突后未生成重散列个体：数量=%d", len(engine.passives.entries))
	}
	seenDerived := 0
	nextID := uint64(0)
	for index := range engine.passives.entries {
		switch id := engine.passives.entries[index].id; id {
		case derivedID:
			seenDerived++
		default:
			nextID = id
		}
	}
	if seenDerived != 1 || nextID == 0 {
		t.Fatalf("重散列结果异常：派生 `id` 出现 %d 次，另一 `id`=%d", seenDerived, nextID)
	}
}

func TestPassiveSpawnReplayIsDeterministic(t *testing.T) {
	// 相同种子 + tick 序列 + 玩家集合：两只独立引擎的生成序列（`id`、位置、
	// 身体值）必须逐项相同；无概率门槛时前 6 个白昼 tick 恰好生成 6 头。
	run := func() []PassiveMob {
		engine, _ := spawnTestEngine(t, 42)
		loadSpawnArena(t, engine, -48, 48, -48, 48)
		for tick := uint64(1); tick <= 6; tick++ {
			engine.worldTime.Store(tick)
			engine.advancePassiveSpawn()
		}
		return engine.PassiveMobs()
	}
	first, second := run(), run()
	if len(first) != 6 {
		t.Fatalf("前 6 个白昼 tick 生成了 %d 头，想要恰好 6", len(first))
	}
	if len(first) != len(second) {
		t.Fatalf("两次重放数量=%d/%d，想要一致", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("第 %d 头被动牛重放不一致：%+v vs %+v", index, first[index], second[index])
		}
	}
}

func TestPassiveSpawnValidatesAtMostOneCandidatePerTick(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	// 连续 50 个白昼 tick：单 tick 增量恒不超过 1，总量被附近上限截在 6。
	for tick := uint64(1); tick <= 50; tick++ {
		engine.worldTime.Store(tick)
		before := len(engine.passives.entries)
		engine.advancePassiveSpawn()
		if delta := len(engine.passives.entries) - before; delta > 1 {
			t.Fatalf("tick %d 生成了 %d 头，想要至多 1", tick, delta)
		}
	}
	if len(engine.passives.entries) != 6 {
		t.Fatalf("50 个白昼 tick 后被动牛=%d，想要被附近上限截在 6", len(engine.passives.entries))
	}
}

func TestPassiveSpawnWithoutActiveSessionsDoesNothing(t *testing.T) {
	engine := NewEngine(0, 1, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	engine.advancePassiveSpawn()
	if len(engine.passives.entries) != 0 {
		t.Fatal("没有 `active` 会话仍生成了被动牛")
	}
}
