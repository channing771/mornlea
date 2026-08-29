package runtime

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestFarmlandMoistureRescanPositionOrder 锁定完整高度 halo 的 `y,z,x` 游标还原。
func TestFarmlandMoistureRescanPositionOrder(t *testing.T) {
	if farmlandMoistureRescanSide != 24 || farmlandMoistureRescanCells != 221_184 {
		t.Fatalf("重扫尺寸=%d/%d，想要 24/221184",
			farmlandMoistureRescanSide, farmlandMoistureRescanCells)
	}
	key := core.ChunkKey{
		Dimension: core.Overworld,
		Pos:       core.ChunkPos{X: 2, Z: -3},
	}
	for _, tc := range []struct {
		cursor int
		want   core.BlockPos
	}{
		{0, core.BlockPos{X: 28, Y: core.MinY, Z: -52}},
		{1, core.BlockPos{X: 29, Y: core.MinY, Z: -52}},
		{23, core.BlockPos{X: 51, Y: core.MinY, Z: -52}},
		{24, core.BlockPos{X: 28, Y: core.MinY, Z: -51}},
		{575, core.BlockPos{X: 51, Y: core.MinY, Z: -29}},
		{576, core.BlockPos{X: 28, Y: core.MinY + 1, Z: -52}},
		{221_183, core.BlockPos{X: 51, Y: core.MaxY - 1, Z: -29}},
	} {
		if got := farmlandMoistureRescanPosition(key, tc.cursor); got != tc.want {
			t.Fatalf("游标 %d 还原为 %+v，想要 %+v", tc.cursor, got, tc.want)
		}
	}
}

// TestFarmlandMoistureRescanSpreadsAcrossFourTicks 锁定单个 job 的预算续扫边界。
func TestFarmlandMoistureRescanSpreadsAcrossFourTicks(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	key := core.ChunkKey{Dimension: core.Overworld}
	engine.fluidScope = map[core.ChunkKey]struct{}{key: {}}
	engine.farmlandMoisture.rescans.enqueueChunk(key)

	for tick, wantCursor := range []int{65_536, 131_072, 196_608} {
		engine.advanceFarmlandMoisture(engine.newMutation())
		if got := engine.farmlandMoisture.blockReads; got != farmlandMoistureReadsPerTick {
			t.Fatalf("第 %d tick 读取=%d，想要 %d", tick+1, got, farmlandMoistureReadsPerTick)
		}
		if got := engine.farmlandMoisture.rescans.cursor; got != wantCursor {
			t.Fatalf("第 %d tick 游标=%d，想要 %d", tick+1, got, wantCursor)
		}
	}
	engine.advanceFarmlandMoisture(engine.newMutation())
	if got := engine.farmlandMoisture.blockReads; got != 24_576 {
		t.Fatalf("第 4 tick 读取=%d，想要 24576", got)
	}
	if got := len(engine.farmlandMoisture.rescans.pending); got != 0 {
		t.Fatalf("第 4 tick 后仍有 %d 个重扫 job，想要 0", got)
	}
	if engine.farmlandMoisture.rescans.cursor != 0 {
		t.Fatalf("重扫完成后游标=%d，想要 0", engine.farmlandMoisture.rescans.cursor)
	}
}

// TestFarmlandMoistureRescanEventsConsumeBudgetFirst 锁定事件候选先于游标推进。
func TestFarmlandMoistureRescanEventsConsumeBudgetFirst(t *testing.T) {
	engine, _ := readyCropWorld(t)
	key := core.ChunkKey{Dimension: core.Overworld}
	engine.farmlandMoisture.rescans.enqueueChunk(key)
	for x := int32(0); x < 10; x++ {
		engine.enqueueFarmlandMoisture(core.Overworld, core.BlockPos{X: x, Y: core.MinY})
	}

	engine.advanceFarmlandMoisture(engine.newMutation())
	if got := engine.farmlandMoisture.blockReads; got != farmlandMoistureReadsPerTick {
		t.Fatalf("事件加重扫读取=%d，想要 %d", got, farmlandMoistureReadsPerTick)
	}
	if got := engine.farmlandMoisture.rescans.cursor; got != 65_526 {
		t.Fatalf("10 个事件后的重扫游标=%d，想要 65526", got)
	}
}

// TestFarmlandMoistureRescanDropsOutOfScopeAndRestarts 锁定离开 scope 后从零重扫。
func TestFarmlandMoistureRescanDropsOutOfScopeAndRestarts(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	key := core.ChunkKey{Dimension: core.Overworld}
	engine.fluidScope = map[core.ChunkKey]struct{}{key: {}}
	engine.farmlandMoisture.rescans.enqueueChunk(key)
	engine.runFarmlandMoistureRescans(17)
	if got := engine.farmlandMoisture.rescans.cursor; got != 17 {
		t.Fatalf("首段重扫游标=%d，想要 17", got)
	}

	delete(engine.fluidScope, key)
	engine.advanceFarmlandMoisture(engine.newMutation())
	if len(engine.farmlandMoisture.rescans.pending) != 0 || engine.farmlandMoisture.rescans.cursor != 0 {
		t.Fatalf("离开 scope 后重扫未清空：pending=%d cursor=%d",
			len(engine.farmlandMoisture.rescans.pending), engine.farmlandMoisture.rescans.cursor)
	}
	if _, queued := engine.farmlandMoisture.rescans.queued[key]; queued {
		t.Fatal("离开 scope 的 job 仍在去重集合中")
	}

	engine.fluidScope[key] = struct{}{}
	engine.farmlandMoisture.rescans.enqueueChunk(key)
	engine.runFarmlandMoistureRescans(1)
	if got := engine.farmlandMoisture.rescans.cursor; got != 1 {
		t.Fatalf("重新进入后的游标=%d，想要从零推进到 1", got)
	}
}

func finishFarmlandMoistureRescan(t *testing.T, engine *Engine) {
	t.Helper()
	for tick := 0; len(engine.farmlandMoisture.rescans.pending) > 0; tick++ {
		if tick == 10 {
			t.Fatalf("重扫在 10 tick 内没有完成，仍有 %d 个 job",
				len(engine.farmlandMoisture.rescans.pending))
		}
		engine.farmlandMoisture.blockReads = 0
		engine.runFarmlandMoistureRescans(farmlandMoistureReadsPerTick)
	}
}

// TestFarmlandMoistureRescanOnlyQueuesActiveReadyFarmland 锁定 halo 的 scope 门禁。
func TestFarmlandMoistureRescanOnlyQueuesActiveReadyFarmland(t *testing.T) {
	center := core.ChunkPos{}
	left := core.ChunkPos{X: -1}
	engine, _ := readyCropWorldAt(t, center, left)
	centerKey := core.ChunkKey{Dimension: core.Overworld, Pos: center}
	leftKey := core.ChunkKey{Dimension: core.Overworld, Pos: left}
	centerFarmland := core.BlockPos{X: 0, Y: core.MinY, Z: 0}
	leftFarmland := core.BlockPos{X: -1, Y: core.MinY, Z: 0}
	// `readyCropWorldAt` 已经推进过初始重扫；清空它，确保本用例的新 job 从游标零开始。
	engine.farmlandMoisture = farmlandMoistureState{}
	engine.SetBlockForTest(centerFarmland, core.FarmlandWetID)
	engine.SetBlockForTest(leftFarmland, core.FarmlandWetID)

	delete(engine.fluidScope, leftKey)
	engine.farmlandMoisture.rescans.enqueueChunk(centerKey)
	finishFarmlandMoistureRescan(t, engine)
	centerCandidate := farmlandMoistureKey{dimension: core.Overworld, position: centerFarmland}
	leftCandidate := farmlandMoistureKey{dimension: core.Overworld, position: leftFarmland}
	if _, queued := engine.farmlandMoisture.queued[centerCandidate]; !queued {
		t.Fatal("目标区块内的 active Ready 耕地没有被重扫入队")
	}
	if _, queued := engine.farmlandMoisture.queued[leftCandidate]; queued {
		t.Fatal("halo 中自身区块不在 scope 的耕地被错误入队")
	}
	if got := cropBlockAt(t, engine, centerFarmland); got != core.FarmlandWetID {
		t.Fatalf("重扫 tick 内候选已被处理成 %s，候选应从下一 tick 才开始处理", blockLabel(got))
	}

	engine.fluidScope[leftKey] = struct{}{}
	engine.farmlandMoisture.rescans.enqueueChunk(centerKey)
	finishFarmlandMoistureRescan(t, engine)
	if _, queued := engine.farmlandMoisture.queued[leftCandidate]; !queued {
		t.Fatal("halo 中自身区块 active Ready 的耕地没有被入队")
	}
}
