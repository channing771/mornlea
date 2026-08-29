package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/sim/tuning"
)

// farmlandMoistureFluidAdapter 构造真实的流体写入适配器，并把目标格六邻中的空气
// 封成石头，保证写入的水不会在夹具外自行流动。
func farmlandMoistureFluidAdapter(
	t *testing.T,
	engine *Engine,
	position core.BlockPos,
) *fluidWorld {
	t.Helper()
	for _, neighbor := range fluidNeighbors(position) {
		if cropBlockAt(t, engine, neighbor) == core.AirID {
			engine.SetBlockForTest(neighbor, core.StoneID)
		}
	}
	return &fluidWorld{
		engine:    engine,
		id:        core.Overworld,
		dimension: engine.dimension(core.Overworld),
		scope:     engine.realm.FluidScope(),
		pending:   engine.newMutation(),
	}
}

// TestFarmlandMoistureFluidMembershipChanges 覆盖同 tick 放水变湿与失水变干。
func TestFarmlandMoistureFluidMembershipChanges(t *testing.T) {
	engine, _ := readyCropWorld(t)
	farmland := cropFixtureFarmland
	water := farmland
	water.X += farmlandWetRadius
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)

	adapter.SetBlock(water, core.WaterSourceID)
	engine.advanceFarmlandMoisture(adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("同 tick 放水后耕地=%s，想要湿耕地", blockLabel(got))
	}

	adapter.SetBlock(water, core.AirID)
	engine.advanceFarmlandMoisture(adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandDryID {
		t.Fatalf("同 tick 失水后耕地=%s，想要干耕地", blockLabel(got))
	}
}

// TestPlayerPlacementRemovingLastIrrigationDriesFarmlandSameTick 锁定普通玩家放置
// 覆盖最后一格灌溉水时，经真实命令路径在同一权威 tick 重判附近耕地。
func TestPlayerPlacementRemovingLastIrrigationDriesFarmlandSameTick(t *testing.T) {
	t.Cleanup(func() { tuning.SetTunables(tuning.DefaultTunables()) })
	tunables := tuning.DefaultTunables()
	tunables.RandomTicksPerSection = 0
	tuning.SetTunables(tunables)
	engine, session := readyMovementPlayer(t)
	for tick := 0; engine.realm.FarmlandRescanPendingLen() > 0; tick++ {
		if tick == 8 {
			t.Fatalf("初始湿度重扫 8 tick 后仍有 %d 个 job", engine.realm.FarmlandRescanPendingLen())
		}
		engine.Step()
	}
	if pending := engine.realm.FarmlandMoisturePendingLen() - engine.farmlandMoisture.head; pending != 0 {
		t.Fatalf("放置前仍有 %d 个旧湿度候选", pending)
	}

	water := core.BlockPos{X: 0, Y: -1, Z: 1}
	farmland := core.BlockPos{X: 1, Y: -1, Z: 1}
	support := core.BlockPos{X: 0, Y: -2, Z: 1}
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: 0, Z: 0}, core.AirID)
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: 0, Z: 1}, core.AirID)
	engine.SetBlockForTest(support, core.StoneID)
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: -1, Z: 0}, core.StoneID)
	engine.SetBlockForTest(core.BlockPos{X: 0, Y: -1, Z: 2}, core.StoneID)
	engine.SetBlockForTest(farmland, core.FarmlandWetID)
	engine.SetBlockForTest(water, core.WaterSourceID)
	player := engine.sessions[session].player
	player.inventory.Hotbar.Slots[0] = core.ItemStack{Item: core.ItemStone, Count: 2}
	eye := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	yaw, pitch := lookAtBlockCenter(eye, support)
	watch := watchFarmlandMoistureCandidateAtPhase(engine, farmlandMoistureKey{
		dimension: core.Overworld,
		position:  farmland,
	})

	engine.Enqueue(Command{
		Session: session, Sequence: 2, Kind: CommandPlaceBlock,
		Yaw: yaw, Pitch: pitch, Slot: 0,
	})
	result := engine.Step()

	if got := cropBlockAt(t, engine, water); got != core.StoneID {
		t.Fatalf("放置目标=%s，想要石头", blockLabel(got))
	}
	if !watch.phaseSeen || !watch.candidateSeen {
		t.Fatalf("湿度阶段未观察到耕地候选：phaseSeen=%v candidateSeen=%v",
			watch.phaseSeen, watch.candidateSeen)
	}
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandDryID {
		t.Fatalf("覆盖最后灌溉水的同 tick 耕地=%s，想要干耕地；reads=%d inspections=%d",
			blockLabel(got), engine.realm.FarmlandBlockReads(),
			engine.realm.FarmlandCandidateInspections())
	}
	if len(result.Rejected) != 0 || len(result.PlacementSuccesses) != 1 ||
		result.PlacementSuccesses[0] != (PlacementSuccess{Session: session, Sequence: 2}) {
		t.Fatalf("放置确认不正确：rejected=%+v successes=%+v",
			result.Rejected, result.PlacementSuccesses)
	}
	wantStack := core.ItemStack{Item: core.ItemStone, Count: 1}
	if got := player.inventory.Hotbar.Slots[0]; got != wantStack {
		t.Fatalf("放置后栏位=%+v，想要恰好扣一件后的 %+v", got, wantStack)
	}
	if len(result.Inventories) != 1 || result.Inventories[0].Session != session ||
		result.Inventories[0].Inventory.Hotbar.Slots[0] != wantStack {
		t.Fatalf("放置没有发布扣料后的背包：%+v", result.Inventories)
	}
}

// TestFarmlandMoistureFluidBoundaries 锁定流体反向候选的水平与垂直边界。
func TestFarmlandMoistureFluidBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name     string
		distance int32
		dy       int32
		want     core.BlockID
	}{
		{"距离 4 下一层", 4, -1, core.FarmlandDryID},
		{"距离 4 同层", 4, 0, core.FarmlandWetID},
		{"距离 4 上一层", 4, 1, core.FarmlandWetID},
		{"距离 5 下一层", 5, -1, core.FarmlandDryID},
		{"距离 5 同层", 5, 0, core.FarmlandDryID},
		{"距离 5 上一层", 5, 1, core.FarmlandDryID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := readyCropWorld(t)
			farmland := cropFixtureFarmland
			water := farmland
			water.X += tc.distance
			water.Y += tc.dy
			engine.SetBlockForTest(farmland, core.FarmlandDryID)
			adapter := farmlandMoistureFluidAdapter(t, engine, water)

			adapter.SetBlock(water, core.WaterSourceID)
			engine.advanceFarmlandMoisture(adapter.pending)
			if got := cropBlockAt(t, engine, farmland); got != tc.want {
				t.Fatalf("真实流体写入后耕地=%s，想要 %s", blockLabel(got), blockLabel(tc.want))
			}
		})
	}
}

// TestFarmlandMoistureFluidLevelChangeDoesNotEnqueue 锁定流体等级变化不生产候选。
func TestFarmlandMoistureFluidLevelChangeDoesNotEnqueue(t *testing.T) {
	engine, _ := readyCropWorld(t)
	water := cropFixtureFarmland
	water.X += farmlandWetRadius
	adapter := farmlandMoistureFluidAdapter(t, engine, water)
	engine.SetBlockForTest(water, core.WaterSourceID)
	engine.farmlandMoisture = farmlandMoistureState{}
	engine.realm.ResetFarmlandMoisture()

	adapter.SetBlock(water, core.WaterLevel1ID)
	if engine.realm.FarmlandMoisturePendingLen() != 0 || engine.realm.FarmlandQueuedCount() != 0 {
		t.Fatalf("流体等级变化产生了湿度候选：pending=%d queued=%d",
			engine.realm.FarmlandMoisturePendingLen(), engine.realm.FarmlandQueuedCount())
	}
}

// TestFarmlandMoistureFluidFloodedCropMembershipChange 锁定作物冲毁的真实流体写入
// 同样生产湿度候选；该写入在 `fluidWorld.SetBlock` 内走独立结算分支。
func TestFarmlandMoistureFluidFloodedCropMembershipChange(t *testing.T) {
	engine, _ := readyCropWorld(t)
	farmland := cropFixtureFarmland
	water := farmland
	water.X += farmlandWetRadius
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)
	engine.SetBlockForTest(water, core.WheatStage0ID)

	adapter.SetBlock(water, core.WaterSourceID)
	if got := cropBlockAt(t, engine, water); got != core.WaterSourceID {
		t.Fatalf("作物冲毁后目标=%s，想要水源", blockLabel(got))
	}
	engine.advanceFarmlandMoisture(adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("作物冲毁产生流体后耕地=%s，想要湿耕地", blockLabel(got))
	}
}

// TestFarmlandMoistureFluidCrossChunkBoundary 锁定 x=16 的水会唤醒 x=15 的耕地。
func TestFarmlandMoistureFluidCrossChunkBoundary(t *testing.T) {
	engine, _ := readyCropWorldAt(t, core.ChunkPos{}, core.ChunkPos{X: 1})
	farmland := core.BlockPos{X: 15, Y: 1, Z: 8}
	water := core.BlockPos{X: 16, Y: 1, Z: 8}
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)

	adapter.SetBlock(water, core.WaterSourceID)
	engine.advanceFarmlandMoisture(adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("跨区块真实流体写入后耕地=%s，想要湿耕地", blockLabel(got))
	}
}

// TestFarmlandMoistureRestartRecoversStaleWetFarmland 覆盖 fresh `Engine` 首次看见
// Ready 区块时，以固定读取预算恢复存档中的陈旧湿耕地。
func TestFarmlandMoistureRestartRecoversStaleWetFarmland(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	const session = SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	farmland := core.BlockPos{X: 8, Y: 1, Z: 8}

	for tick := 0; tick < 16; tick++ {
		result := engine.Step()
		if reads := engine.realm.FarmlandBlockReads(); reads > farmlandMoistureReadsPerTick {
			t.Fatalf("第 %d tick 湿度读取=%d，超过预算 %d",
				tick, reads, farmlandMoistureReadsPerTick)
		}
		for _, key := range result.Acquire {
			engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
		}
		for _, key := range result.Generate {
			chunk := cropFlatChunk(key.Pos)
			if key.Pos == farmland.Chunk() {
				x, _, z := farmland.Local()
				chunk.SetBlock(x, farmland.Y, z, core.FarmlandWetID)
			}
			engine.SubmitGenerated(GeneratedChunk{
				Dimension: key.Dimension,
				Pos:       key.Pos,
				Chunk:     chunk,
			})
		}
		if block, ready := engine.dimension(core.Overworld).BlockAt(farmland); ready && block == core.FarmlandDryID {
			return
		}
	}
	t.Fatalf("重启恢复未在预算内把陈旧耕地改干，当前=%s",
		blockLabel(cropBlockAt(t, engine, farmland)))
}

// TestFarmlandMoistureReentryRestartsRescan 覆盖邻块离开后丢弃半截 job，并在恢复
// Ready 时从游标零重新扫描，最终按邻块当前的水恢复边界耕地。
func TestFarmlandMoistureReentryRestartsRescan(t *testing.T) {
	center := core.ChunkPos{}
	right := core.ChunkPos{X: 1}
	centerKey := core.ChunkKey{Dimension: core.Overworld, Pos: center}
	rightKey := core.ChunkKey{Dimension: core.Overworld, Pos: right}
	farmland := core.BlockPos{X: 15, Y: 1, Z: 8}
	water := core.BlockPos{X: 16, Y: 1, Z: 8}
	engine, sessions := readyCropWorldAt(t, center, right)
	engine.SetBlockForTest(farmland, core.FarmlandWetID)
	placeContainedWater(t, engine, water)
	engine.farmlandMoisture = farmlandMoistureState{}
	engine.realm.ResetFarmlandMoisture()
	step := func(stage string) TickResult {
		result := engine.Step()
		if reads := engine.realm.FarmlandBlockReads(); reads > farmlandMoistureReadsPerTick {
			t.Fatalf("%s 湿度读取=%d，超过预算 %d", stage, reads, farmlandMoistureReadsPerTick)
		}
		return result
	}

	if engine.sessions[sessions[1]] == nil {
		t.Fatal("邻块离开前会话不存在")
	}
	engine.UnregisterSession(sessions[1])
	if _, exists := engine.sessions[sessions[1]]; exists {
		t.Fatal("邻块会话未被删除")
	}
	step("邻块离开 tick")
	if info, ok := engine.ChunkInfo(rightKey); ok && info.State == ChunkReady {
		t.Fatalf("邻块注销后仍是 Ready：%+v", info)
	}

	restore := func(session SessionID) int {
		t.Helper()
		engine.RegisterSession(session, core.Overworld, right)
		for range 8 {
			result := step("邻块恢复 tick")
			for _, key := range result.Acquire {
				engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
			}
			for _, key := range result.Generate {
				chunk := cropFlatChunk(key.Pos)
				if key.Pos == right {
					for _, neighbor := range fluidNeighbors(water) {
						if neighbor.Chunk() != right {
							continue
						}
						x, _, z := neighbor.Local()
						chunk.SetBlock(x, neighbor.Y, z, core.StoneID)
					}
					x, _, z := water.Local()
					chunk.SetBlock(x, water.Y, z, core.WaterSourceID)
				}
				engine.SubmitGenerated(GeneratedChunk{
					Dimension: key.Dimension,
					Pos:       key.Pos,
					Chunk:     chunk,
				})
			}
			if _, ready := engine.realm.FluidScope()[rightKey]; ready {
				if _, centerReady := engine.realm.FluidScope()[centerKey]; !centerReady {
					t.Fatal("边界耕地所在区块未与邻块同时恢复 Ready")
				}
				return engine.realm.FarmlandRescanCursor()
			}
		}
		t.Fatal("邻块未在预算内恢复 Ready")
		return 0
	}

	if cursor := restore(sessions[1]); cursor != farmlandMoistureReadsPerTick {
		t.Fatalf("首次重入游标=%d，想要 %d", cursor, farmlandMoistureReadsPerTick)
	}
	if engine.sessions[sessions[1]] == nil {
		t.Fatal("半截重扫期间邻块会话不存在")
	}
	engine.UnregisterSession(sessions[1])
	if _, exists := engine.sessions[sessions[1]]; exists {
		t.Fatal("半截重扫期间邻块会话未被删除")
	}
	step("半截重扫离开 tick")
	if engine.realm.FarmlandRescanPendingLen() != 0 || engine.realm.FarmlandRescanCursor() != 0 {
		t.Fatalf("离开 scope 后重扫未清零：pending=%d cursor=%d",
			engine.realm.FarmlandRescanPendingLen(), engine.realm.FarmlandRescanCursor())
	}
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandDryID {
		t.Fatalf("再次重入前耕地=%s，想要强制的陈旧干耕地", blockLabel(got))
	}
	farmlandKey := farmlandMoistureKey{dimension: core.Overworld, position: farmland}
	if queued := engine.realm.FarmlandQueued(farmlandKey.dimension, farmlandKey.position); queued {
		t.Fatal("再次重入前已有耕地候选，无法证明恢复来自重扫")
	}
	if cursor := restore(sessions[1]); cursor != farmlandMoistureReadsPerTick {
		t.Fatalf("再次重入游标=%d，想要从零推进到 %d", cursor, farmlandMoistureReadsPerTick)
	}
	if queued := engine.realm.FarmlandQueued(farmlandKey.dimension, farmlandKey.position); !queued {
		t.Fatal("再次重入的重扫没有发现并登记边界耕地")
	}
	for range 8 {
		step("重入收敛 tick")
		if cropBlockAt(t, engine, farmland) == core.FarmlandWetID {
			return
		}
	}
	t.Fatalf("邻块恢复后边界耕地=%s，想要按当前水状态恢复为湿耕地",
		blockLabel(cropBlockAt(t, engine, farmland)))
}

// TestFarmlandMoistureReentryRecoversStaleWetFarmland 覆盖边界耕地离开相关 scope
// 期间失去邻块流体，并在邻块重新进入 active Ready 后由有界重扫恢复为干耕地。
func TestFarmlandMoistureReentryRecoversStaleWetFarmland(t *testing.T) {
	center := core.ChunkPos{}
	right := core.ChunkPos{X: 1}
	rightKey := core.ChunkKey{Dimension: core.Overworld, Pos: right}
	farmland := core.BlockPos{X: 15, Y: 1, Z: 8}
	water := core.BlockPos{X: 16, Y: 1, Z: 8}
	engine, sessions := readyCropWorldAt(t, center, right)
	engine.SetBlockForTest(farmland, core.FarmlandWetID)
	placeContainedWater(t, engine, water)
	engine.farmlandMoisture = farmlandMoistureState{}
	engine.realm.ResetFarmlandMoisture()
	step := func(stage string) TickResult {
		result := engine.Step()
		if reads := engine.realm.FarmlandBlockReads(); reads > farmlandMoistureReadsPerTick {
			t.Fatalf("%s 湿度读取=%d，超过预算 %d", stage, reads, farmlandMoistureReadsPerTick)
		}
		return result
	}

	if got := cropBlockAt(t, engine, water); got != core.WaterSourceID {
		t.Fatalf("邻块离开前水格=%s，想要水源", blockLabel(got))
	}
	engine.UnregisterSession(sessions[1])
	step("邻块离开 tick")
	if info, ok := engine.ChunkInfo(rightKey); ok && info.State == ChunkReady {
		t.Fatalf("邻块注销后仍是 Ready：%+v", info)
	}
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("邻块离开后陈旧耕地=%s，想要在重扫前保持湿耕地", blockLabel(got))
	}
	// 邻块不在 Ready scope 时直接改测试夹具，模拟存档侧失水；这条写入刻意不经过
	// `fluidWorld.SetBlock`，因此不会生产事件，恢复只能来自后续重扫。
	rightInfo, exists := engine.dimension(core.Overworld).Info(right)
	if !exists || rightInfo.State == realm.ChunkReady || rightInfo.Chunk == nil {
		t.Fatalf("邻块离开后记录不适合模拟失水：%+v", rightInfo)
	}
	waterX, _, waterZ := water.Local()
	rightInfo.Chunk.SetBlock(waterX, water.Y, waterZ, core.AirID)
	farmlandKey := farmlandMoistureKey{dimension: core.Overworld, position: farmland}
	if queued := engine.realm.FarmlandQueued(farmlandKey.dimension, farmlandKey.position); queued {
		t.Fatal("邻块重入前已有耕地候选，无法证明恢复来自重扫")
	}

	engine.RegisterSession(sessions[1], core.Overworld, right)
	reentered := false
	queuedByRescan := false
	for tick := 1; tick <= 16; tick++ {
		result := step("邻块无水重入 tick")
		for _, key := range result.Acquire {
			engine.SubmitAcquired(AcquiredChunk{Key: key, Missing: true})
		}
		for _, key := range result.Generate {
			engine.SubmitGenerated(GeneratedChunk{
				Dimension: key.Dimension,
				Pos:       key.Pos,
				Chunk:     cropFlatChunk(key.Pos),
			})
		}
		if _, ready := engine.realm.FluidScope()[rightKey]; ready {
			reentered = true
			block, blockReady := engine.dimension(core.Overworld).BlockAt(water)
			if !blockReady || core.IsFluid(block) {
				t.Fatalf("邻块重入后水格 ready=%v block=%s，想要已失去流体",
					blockReady, blockLabel(block))
			}
		}
		if queued := engine.realm.FarmlandQueued(farmlandKey.dimension, farmlandKey.position); queued {
			queuedByRescan = true
		}
		if cropBlockAt(t, engine, farmland) != core.FarmlandDryID {
			continue
		}
		if !reentered {
			t.Fatal("邻块尚未重新进入 active Ready，耕地就已变干")
		}
		if !queuedByRescan {
			t.Fatal("耕地变干前未观察到重扫候选")
		}
		return
	}
	t.Fatalf("邻块无水重入 16 tick 后边界耕地=%s，想要恢复为干耕地",
		blockLabel(cropBlockAt(t, engine, farmland)))
}

// —— Scenario：耕地的干湿由邻近流体决定并双向转换 ——

// TestFarmlandTurnsWetWithWaterInRange 覆盖 Scenario「水源在范围内使耕地变湿」。
func TestFarmlandTurnsWetWithWaterInRange(t *testing.T) {
	engine, _ := readyCropWorld(t)
	farmland := cropFixtureFarmland
	water := farmland
	water.X += farmlandWetRadius
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)

	adapter.SetBlock(water, core.WaterSourceID)
	engine.advanceFarmlandMoisture(adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("同 tick 放水后耕地=%s，范围内有水时必须变湿", blockLabel(got))
	}
}

// TestFarmlandTurnsDryAfterWaterRemoved 覆盖 Scenario「水被移除后耕地变干」。
//
// 夹具**先证明它湿过**再移除水：若起手就是干耕地，「改不改都是干」，断言恒真。
func TestFarmlandTurnsDryAfterWaterRemoved(t *testing.T) {
	engine, _ := readyCropWorld(t)
	farmland := cropFixtureFarmland
	water := farmland
	water.X += farmlandWetRadius
	engine.SetBlockForTest(farmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, water)

	adapter.SetBlock(water, core.WaterSourceID)
	engine.advanceFarmlandMoisture(adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandWetID {
		t.Fatalf("前置失败：放水后耕地=%s，「变干」无从谈起", blockLabel(got))
	}

	adapter.SetBlock(water, core.AirID)
	engine.advanceFarmlandMoisture(adapter.pending)
	if got := cropBlockAt(t, engine, farmland); got != core.FarmlandDryID {
		t.Fatalf("同 tick 移除最后一格水后耕地=%s，范围内无水时必须变干", blockLabel(got))
	}
}

// TestFarmlandWetnessRangeBoundary 覆盖 Scenario「范围外的水不产生湿润」。
//
// 四条子用例把湿润窗口的**四个边界**各钉一颗钉子，每一对都只差一个字段：
//
//   - 水平方向：距离 4 湿、距离 5 不湿。只测距离 5 的话，夹具在距离 4 处也没有
//     水，「不湿」在任何半径实现下都成立（包括半径写成 0 的实现）。
//   - 垂直方向：上一层湿、只在下一层不湿。规格写的是「同层**或上一层**」，
//     而所有正向夹具的水都放在同层——上界删掉（只看同层）与下界放宽（连下一层
//     也算）这两种实现，在只有同层夹具时都照样全绿。
func TestFarmlandWetnessRangeBoundary(t *testing.T) {
	for _, tc := range []struct {
		name     string
		distance int32
		dy       int32
		want     core.BlockID
	}{
		{"同层距离 4 的水使耕地变湿", 4, 0, core.FarmlandWetID},
		{"同层距离 5 的水不使耕地变湿", 5, 0, core.FarmlandDryID},
		{"上一层距离 4 的水使耕地变湿", 4, +1, core.FarmlandWetID},
		{"只在下一层的水不使耕地变湿", 4, -1, core.FarmlandDryID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _ := readyCropWorld(t)
			farmland := cropFixtureFarmland
			water := farmland
			water.X += tc.distance
			water.Y += tc.dy
			engine.SetBlockForTest(farmland, core.FarmlandDryID)
			adapter := farmlandMoistureFluidAdapter(t, engine, water)

			adapter.SetBlock(water, core.WaterSourceID)
			engine.advanceFarmlandMoisture(adapter.pending)
			if got := cropBlockAt(t, engine, farmland); got != tc.want {
				t.Fatalf("同 tick 真实流体写入后耕地=%s，想要 %s",
					blockLabel(got), blockLabel(tc.want))
			}
		})
	}
}

// —— 跨区块湿润与「相邻区块未加载按无水」 ——

// 跨区块夹具：耕地在世界 x=0（区块 (0,0) 的局部 x=0），它的 9×9 湿润窗口
// x ∈ [-4, 4] 因此跨进区块 (-1,0)；水放在 x=-2，**只存在于邻块里**。
var (
	cropCrossFarmland = core.BlockPos{X: 0, Y: 1, Z: 8}
	cropCrossWater    = core.BlockPos{X: -2, Y: 1, Z: 8}
)

// TestFarmlandWetnessCrossesChunkBoundary 覆盖流体事件唤醒邻块耕地，并由湿度规则
// 跨区块读取水源。区块离开再进入后的恢复由重扫专用测试锁定，不再等待随机 tick。
func TestFarmlandWetnessCrossesChunkBoundary(t *testing.T) {
	engine, _ := readyCropWorldAt(t, core.ChunkPos{}, core.ChunkPos{X: -1})
	engine.SetBlockForTest(cropCrossFarmland, core.FarmlandDryID)
	adapter := farmlandMoistureFluidAdapter(t, engine, cropCrossWater)

	adapter.SetBlock(cropCrossWater, core.WaterSourceID)
	engine.advanceFarmlandMoisture(adapter.pending)
	if got := cropBlockAt(t, engine, cropCrossFarmland); got != core.FarmlandWetID {
		t.Fatalf("同 tick 邻块放水后耕地=%s：湿润窗口没有读进相邻区块", blockLabel(got))
	}
}
