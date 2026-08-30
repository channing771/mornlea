package runtime

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
)

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
	engine.realm.ResetFarmlandMoisture()
	step := func(stage string) TickResult {
		result := engine.Step()
		if reads := engine.realm.FarmlandBlockReads(); reads > farmlandMoistureReadsPerTick {
			t.Fatalf("%s 湿度读取=%d，超过预算 %d", stage, reads, farmlandMoistureReadsPerTick)
		}
		return result
	}

	if engine.subscriptions[sessions[1]] == nil {
		t.Fatal("邻块离开前会话不存在")
	}
	engine.UnregisterSession(sessions[1])
	if _, exists := engine.subscriptions[sessions[1]]; exists {
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
			if engine.realm.FluidScopeContains(rightKey) {
				if !engine.realm.FluidScopeContains(centerKey) {
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
	if engine.subscriptions[sessions[1]] == nil {
		t.Fatal("半截重扫期间邻块会话不存在")
	}
	engine.UnregisterSession(sessions[1])
	if _, exists := engine.subscriptions[sessions[1]]; exists {
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
	if queued := engine.realm.FarmlandQueued(core.Overworld, farmland); queued {
		t.Fatal("再次重入前已有耕地候选，无法证明恢复来自重扫")
	}
	if cursor := restore(sessions[1]); cursor != farmlandMoistureReadsPerTick {
		t.Fatalf("再次重入游标=%d，想要从零推进到 %d", cursor, farmlandMoistureReadsPerTick)
	}
	if queued := engine.realm.FarmlandQueued(core.Overworld, farmland); !queued {
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
	if queued := engine.realm.FarmlandQueued(core.Overworld, farmland); queued {
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
		if engine.realm.FluidScopeContains(rightKey) {
			reentered = true
			block, blockReady := engine.dimension(core.Overworld).BlockAt(water)
			if !blockReady || core.IsFluid(block) {
				t.Fatalf("邻块重入后水格 ready=%v block=%s，想要已失去流体",
					blockReady, blockLabel(block))
			}
		}
		if queued := engine.realm.FarmlandQueued(core.Overworld, farmland); queued {
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
