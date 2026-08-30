package runtime

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件锁定夜行者的确定性夜间生成：锚点玩家按已排序 active session 与
// `WorldTimeTicks % 会话数` 选取、splitmix64 整数派生半径与轴向（水平距离
// 24..48）、候选哈希低 8 位 <13 才尝试、双格空气/下方 solid/非流体/完整
// loaded/局部区块光 ≤7/夜间窗口全部必要、全服 ≤64 与每玩家 48 格内 ≤8、
// 每 tick 至多验证一个候选、相同输入重放逐位一致，以及 ID 冲突重散列。

// spawnTestEngine 构造带一名已激活锚点玩家的引擎（世界种子可指定）。
func spawnTestEngine(t *testing.T, seed int64) (*Engine, SessionID) {
	t.Helper()
	engine := NewEngine(0, 0, seed)
	session := SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	engine.Step()
	engine.SubmitAcquired(AcquiredChunk{Key: core.ChunkKey{Dimension: core.Overworld}, Missing: true})
	result := engine.Step()
	for _, key := range result.Generate {
		engine.SubmitGenerated(GeneratedChunk{
			Dimension: core.Overworld,
			Chunk:     movementFlatChunk(key.Pos),
		})
	}
	spawned := engine.Step()
	if !spawned.Players[0].Ready {
		t.Fatal("锚点玩家未在预算内激活")
	}
	return engine, session
}

// clearHostilesForTest 清空夜行者集合，供探针循环复用同一个引擎。
func clearHostilesForTest(engine *Engine) {
	engine.hostiles.entries = engine.hostiles.entries[:0]
}

// findSpawningTick 自 start 起按 step 步长扫描至多 limit 个世界时间值，返回
// 第一个让引擎实际生成一只夜行者的 tick。arena 与锚点必须已就绪，否则扫描
// 本身没有意义。
func findSpawningTick(t *testing.T, engine *Engine, start, step uint64, limit int) uint64 {
	t.Helper()
	for offset := range limit {
		tick := start + uint64(offset)*step
		engine.worldTime.Store(tick)
		before := len(engine.hostiles.entries)
		engine.advanceHostileSpawn()
		delta := len(engine.hostiles.entries) - before
		if delta > 1 {
			t.Fatalf("tick %d 单次生成判定生成了 %d 只夜行者，想要至多 1", tick, delta)
		}
		if delta == 1 {
			return tick
		}
	}
	t.Fatalf("自 %d 步长 %d 扫描 %d 个 tick 内没有可生成的候选", start, step, limit)
	return 0
}

// assertAxisAlignedAtDistance 断言候选恰为锚点方块沿单一水平轴、距离在
// 24..48（含）处的落点——这是「水平距离 24..48 且来自锚点」的精确形态。
func assertAxisAlignedAtDistance(t *testing.T, position mgl32.Vec3, anchorX, anchorZ int32) {
	t.Helper()
	const (
		minRadius = 24
		maxRadius = 48
	)
	x := int32(math.Floor(float64(position.X())))
	z := int32(math.Floor(float64(position.Z())))
	switch {
	case z == anchorZ && abs32(x-anchorX) >= minRadius && abs32(x-anchorX) <= maxRadius:
		return
	case x == anchorX && abs32(z-anchorZ) >= minRadius && abs32(z-anchorZ) <= maxRadius:
		return
	default:
		t.Fatalf("候选 (%d,%d) 不是锚点 (%d,%d) 沿单一水平轴 24..48 的落点", x, z, anchorX, anchorZ)
	}
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// loadSpawnArena 装载覆盖 [minX,maxX]×[minZ,maxZ] 方块范围的 flat 竞技场。
func loadSpawnArena(t *testing.T, engine *Engine, minX, maxX, minZ, maxZ int32) {
	t.Helper()
	loadFlatChunks(t, engine.dimension(core.Overworld),
		minX>>core.SectionShift, maxX>>core.SectionShift,
		minZ>>core.SectionShift, maxZ>>core.SectionShift)
}

// spawnedPosition 返回引擎中（唯一一只）夜行者的脚底位置。
func spawnedPosition(t *testing.T, engine *Engine) mgl32.Vec3 {
	t.Helper()
	if len(engine.hostiles.entries) != 1 {
		t.Fatalf("夜行者数量=%d，想要 1", len(engine.hostiles.entries))
	}
	return engine.hostiles.entries[0].state.Position
}

// spawnCandidateColumn 探得 (tick, 锚点) 下的候选列方块坐标。要求引擎已在该
// tick 生成过一只夜行者（flat 地表候选恒在 y=1）。坐标取 floor（脚底中心
// 0.5 偏移落在方块内），与权威 `blockPosOf` 同一定位规则。
func spawnCandidateColumn(t *testing.T, engine *Engine) core.BlockPos {
	t.Helper()
	if len(engine.hostiles.entries) != 1 {
		t.Fatalf("探针夹具失效：夜行者数量=%d，想要 1", len(engine.hostiles.entries))
	}
	position := engine.hostiles.entries[0].state.Position
	return core.BlockPos{
		X: int32(math.Floor(float64(position.X()))),
		Y: 1,
		Z: int32(math.Floor(float64(position.Z()))),
	}
}

func TestHostileSpawnColumnDerivationStaysInContractWindow(t *testing.T) {
	// 派生函数的窗口契约：半径恒在 24..48（含）、轴向是四个水平轴之一、
	// 候选列 = 锚点 + 轴向量 × 半径，且全部输入为整数。
	for tick := uint64(0); tick < 2000; tick++ {
		base := splitmix64(uint64(0) ^ tick)
		x, z, radius, axis := hostileSpawnColumn(base, 100, -40)
		if radius < 24 || radius > 48 {
			t.Fatalf("tick %d 派生半径 %d 越出 24..48", tick, radius)
		}
		if axis < 0 || axis > 3 {
			t.Fatalf("tick %d 派生轴向 %d 越出四轴", tick, axis)
		}
		wantDX := [4]int32{1, -1, 0, 0}[axis]
		wantDZ := [4]int32{0, 0, 1, -1}[axis]
		if x != 100+wantDX*int32(radius) || z != -40+wantDZ*int32(radius) {
			t.Fatalf("tick %d 候选列 (%d,%d) 与轴向量不符", tick, x, z)
		}
	}
}

func TestHostileSpawnPicksAnchorBySortedSessionAndWorldTime(t *testing.T) {
	engine, _ := readyMeleePlayers(t, 2)
	// 竞技场同时覆盖两个锚点的候选环：玩家 1 在原点、玩家 2 在 (40,0)。
	loadSpawnArena(t, engine, -48, 88, -48, 48)
	setMeleePlayer(engine, 1, mgl32.Vec3{0.5, 1, 0.5}, 0)
	setMeleePlayer(engine, 2, mgl32.Vec3{40.5, 1, 0.5}, 0)

	// 会话数为 2：偶数 tick 锚点是排序列表首位的会话 1。
	clearHostilesForTest(engine)
	evenTick := findSpawningTick(t, engine, 13000, 2, 4000)
	if evenTick%2 != 0 {
		t.Fatalf("探针 tick %d 不是偶数，夹具失效", evenTick)
	}
	assertAxisAlignedAtDistance(t, spawnedPosition(t, engine), 0, 0)

	// 奇数 tick 锚点是会话 2：候选必须改为以 (40,0) 为锚。
	clearHostilesForTest(engine)
	oddTick := findSpawningTick(t, engine, 13001, 2, 4000)
	if oddTick%2 != 1 {
		t.Fatalf("探针 tick %d 不是奇数，夹具失效", oddTick)
	}
	assertAxisAlignedAtDistance(t, spawnedPosition(t, engine), 40, 0)
}

func TestHostileSpawnOnlyWithinNightWindow(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)

	// 白昼相位与窗口外相位一律不生成（扫描覆盖大量门槛通过概率）。
	for _, phase := range []uint64{2400, 12000, 12999, 23001, 23500} {
		for offset := range 40 {
			engine.worldTime.Store(phase + uint64(offset)*24000)
			before := len(engine.hostiles.entries)
			engine.advanceHostileSpawn()
			if len(engine.hostiles.entries) != before {
				t.Fatalf("显示相位 %d 生成了夜行者，想要拒绝", phase)
			}
		}
	}

	// 窗口两端（含端点）在门槛通过时必须可以生成。
	for _, phase := range []uint64{13000, 23000} {
		clearHostilesForTest(engine)
		findSpawningTick(t, engine, phase, 24000, 600)
	}
}

func TestHostileSpawnRejectsBrightCandidate(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := findSpawningTick(t, engine, 13000, 1, 400)
	candidate := spawnCandidateColumn(t, engine)

	// 在候选格曼哈顿距离 6 处放一枝火把：候选局部区块光 = 14−6 = 8 > 7，
	// 必须拒绝。
	engine.SetBlockForTest(core.BlockPos{X: candidate.X + 6, Y: 1, Z: candidate.Z}, core.TorchStandingID)
	clearHostilesForTest(engine)
	engine.worldTime.Store(tick)
	engine.advanceHostileSpawn()
	if len(engine.hostiles.entries) != 0 {
		t.Fatal("局部区块光 8 的候选被接受，想要拒绝")
	}

	// 同一火把移到距离 7：候选光 = 7 ≤ 7，恢复可生成（临界值判定）。
	engine.SetBlockForTest(core.BlockPos{X: candidate.X + 6, Y: 1, Z: candidate.Z}, core.AirID)
	engine.SetBlockForTest(core.BlockPos{X: candidate.X + 7, Y: 1, Z: candidate.Z}, core.TorchStandingID)
	engine.advanceHostileSpawn()
	if got := engine.hostileBlockLight(engine.dimension(core.Overworld), candidate); got != 7 {
		t.Fatalf("夹具候选光=%d，想要 7", got)
	}
	if len(engine.hostiles.entries) != 1 {
		t.Fatalf("局部区块光 7 的候选被拒绝，想要生成（数量=%d）", len(engine.hostiles.entries))
	}
}

func TestHostileSpawnRejectsFluidColumn(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := findSpawningTick(t, engine, 13000, 1, 400)
	column := spawnCandidateColumn(t, engine)

	// 候选列灌两格水：候选格自身为流体；更深处的「支撑格」也是流体
	// （非 solid）——整列不再存在任何合法落点，必须拒绝。
	engine.SetBlockForTest(column, core.WaterSourceID)
	engine.SetBlockForTest(core.BlockPos{X: column.X, Y: 2, Z: column.Z}, core.WaterSourceID)
	clearHostilesForTest(engine)
	engine.worldTime.Store(tick)
	engine.advanceHostileSpawn()
	if len(engine.hostiles.entries) != 0 {
		t.Fatal("流体候选列被接受，想要拒绝")
	}
}

func TestHostileSpawnRejectsSingleCellVerticalGap(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := findSpawningTick(t, engine, 13000, 1, 400)
	column := spawnCandidateColumn(t, engine)

	// 整列填实后只在 y=100 挖出单格空气：高度 2 的竖向空间不成立，整列
	// 必须无落点（自上而下的列扫描不得把单格空气当成落脚点）。
	for y := int32(1); y < core.MaxY-1; y++ {
		engine.SetBlockForTest(core.BlockPos{X: column.X, Y: y, Z: column.Z}, core.StoneID)
	}
	engine.SetBlockForTest(core.BlockPos{X: column.X, Y: 100, Z: column.Z}, core.AirID)
	clearHostilesForTest(engine)
	engine.worldTime.Store(tick)
	engine.advanceHostileSpawn()
	if len(engine.hostiles.entries) != 0 {
		t.Fatal("单格竖向空气被当成落点，想要拒绝")
	}
}

func TestHostileSpawnRejectsUnloadedCandidateChunk(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	// 不装载任何候选区块：只有原点区块就绪，24..48 的候选必然落在未加载
	// 区块，绝不能为生成触发同步加载，只能拒绝。
	for offset := range 400 {
		engine.worldTime.Store(13000 + uint64(offset))
		before := len(engine.hostiles.entries)
		engine.advanceHostileSpawn()
		if len(engine.hostiles.entries) != before {
			t.Fatalf("tick %d 在未加载区块生成夜行者", 13000+offset)
		}
	}
}

func TestHostileSpawnRejectsAtGlobalCap(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := findSpawningTick(t, engine, 13000, 1, 400)

	// 先证夹具：同一 tick 在干净集合上确实生成。
	clearHostilesForTest(engine)
	engine.worldTime.Store(tick)
	engine.advanceHostileSpawn()
	if len(engine.hostiles.entries) != 1 {
		t.Fatal("夹具失效：干净集合未能生成")
	}

	// 填满全服 64 只后同一 tick 必须拒绝。
	clearHostilesForTest(engine)
	for id := 1; id <= 64; id++ {
		if err := engine.RestoreHostile(validTestHostile(uint64(id))); err != nil {
			t.Fatalf("预置夜行者 %d：%v", id, err)
		}
	}
	engine.worldTime.Store(tick)
	engine.advanceHostileSpawn()
	if len(engine.hostiles.entries) != 64 {
		t.Fatalf("满编后夜行者数量=%d，想要保持 64", len(engine.hostiles.entries))
	}
}

func TestHostileSpawnRejectsNinthNearAnchorPlayer(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := findSpawningTick(t, engine, 13000, 1, 400)

	// 锚点玩家 48 格内已有 8 只：生成必须被拒绝。
	clearHostilesForTest(engine)
	for id := 1; id <= 8; id++ {
		mob := validTestHostile(uint64(id))
		mob.State.Position = mgl32.Vec3{float32(id) + 0.5, 1, 0.5}
		if err := engine.RestoreHostile(mob); err != nil {
			t.Fatalf("预置近处夜行者 %d：%v", id, err)
		}
	}
	engine.worldTime.Store(tick)
	engine.advanceHostileSpawn()
	if len(engine.hostiles.entries) != 8 {
		t.Fatalf("第 9 只附近夜行者被接受：数量=%d", len(engine.hostiles.entries))
	}

	// 同样 8 只但全部挪到锚点 48 格之外：不再计入附近上限，同 tick 恢复生成。
	for index := range engine.hostiles.entries {
		engine.hostiles.entries[index].state.Position =
			mgl32.Vec3{100 + float32(index+1) + 0.5, 1, 0.5}
	}
	engine.advanceHostileSpawn()
	if len(engine.hostiles.entries) != 9 {
		t.Fatalf("远处 8 只不应计入附近上限：数量=%d", len(engine.hostiles.entries))
	}
}

func TestHostileSpawnRehashesConflictingID(t *testing.T) {
	engine, _ := spawnTestEngine(t, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	tick := findSpawningTick(t, engine, 13000, 1, 400)
	derivedID := engine.hostiles.entries[0].id
	if derivedID == 0 {
		t.Fatal("派生 ID 不应为零")
	}

	// 预置一只与派生 ID 相同的夜行者：生成不得放弃也不得重复，必须重散列
	// 出新的唯一 ID。
	clearHostilesForTest(engine)
	conflict := validTestHostile(derivedID)
	conflict.State.Position = mgl32.Vec3{100.5, 1, 100.5}
	if err := engine.RestoreHostile(conflict); err != nil {
		t.Fatalf("预置冲突 ID：%v", err)
	}
	engine.worldTime.Store(tick)
	engine.advanceHostileSpawn()
	if len(engine.hostiles.entries) != 2 {
		t.Fatalf("ID 冲突后未生成重散列个体：数量=%d", len(engine.hostiles.entries))
	}
	// 派生 ID 恰好出现一次（冲突个体未被覆盖），另一只的 ID 非零且不同。
	seenDerived := 0
	nextID := uint64(0)
	for index := range engine.hostiles.entries {
		switch id := engine.hostiles.entries[index].id; id {
		case derivedID:
			seenDerived++
		default:
			nextID = id
		}
	}
	if seenDerived != 1 || nextID == 0 {
		t.Fatalf("重散列结果异常：派生 ID 出现 %d 次，另一 ID=%d", seenDerived, nextID)
	}
}

func TestHostileSpawnReplayIsDeterministic(t *testing.T) {
	// 相同 seed + tick 序列 + 玩家集合：两只独立引擎的生成序列（ID、位置、
	// 身体值）必须逐项相同。
	run := func() []HostileMob {
		engine, _ := spawnTestEngine(t, 42)
		loadSpawnArena(t, engine, -48, 48, -48, 48)
		for offset := range 240 {
			engine.worldTime.Store(13000 + uint64(offset))
			engine.advanceHostileSpawn()
		}
		return engine.HostileMobs()
	}
	first, second := run(), run()
	if len(first) == 0 {
		t.Fatal("240 个夜间 tick 内没有生成任何夜行者，夹具失效")
	}
	if len(first) != len(second) {
		t.Fatalf("两次重放数量=%d/%d，想要一致", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("第 %d 只夜行者重放不一致：%+v vs %+v", index, first[index], second[index])
		}
	}
}

func TestHostileSpawnGateMatchesHashLowByte(t *testing.T) {
	// 生成门槛必须钉在候选哈希的低 8 位上：生成个体的 ID 即候选哈希，因此
	// 每次成功生成的 ID 低 8 位必须 <13（13/256 契约的可观察形态）。每观察
	// 一次即清空集合，保证个体 ID 未经历冲突重散列。
	engine, _ := spawnTestEngine(t, 7)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	spawned := 0
	for offset := range 600 {
		engine.worldTime.Store(13000 + uint64(offset))
		engine.advanceHostileSpawn()
		for index := range engine.hostiles.entries {
			spawned++
			if id := engine.hostiles.entries[index].id; id&0xFF >= 13 {
				t.Fatalf("生成的 ID %d 低 8 位 ≥13，违反 13/256 门槛", id)
			}
		}
		clearHostilesForTest(engine)
	}
	if spawned == 0 {
		t.Fatal("600 个夜间 tick 内没有任何生成，夹具失效")
	}
}

func TestHostileSpawnWithoutActiveSessionsDoesNothing(t *testing.T) {
	engine := NewEngine(0, 13000, 0)
	loadSpawnArena(t, engine, -48, 48, -48, 48)
	engine.advanceHostileSpawn()
	if len(engine.hostiles.entries) != 0 {
		t.Fatal("没有 active 会话仍生成了夜行者")
	}
}
