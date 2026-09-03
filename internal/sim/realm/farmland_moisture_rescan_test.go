package realm

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestFarmlandMoistureRescanPositionOrder 锁定完整高度 halo 的 `y,z,x` 游标还原。
func TestFarmlandMoistureRescanPositionOrder(t *testing.T) {
	if farmlandMoistureRescanSide != 24 || farmlandMoistureRescanCells != 221_184 {
		t.Fatalf("重扫尺寸=%d/%d，想要 24/221184", farmlandMoistureRescanSide, farmlandMoistureRescanCells)
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
	state := readyFarmlandMoistureState(t, core.ChunkPos{})
	key := core.ChunkKey{Dimension: core.Overworld}
	active := []core.ChunkKey{key}
	state.environment.farmlandMoisture.rescans.enqueueChunk(key)

	for tick, wantCursor := range []int{65_536, 131_072, 196_608} {
		advanceFarmlandMoistureTest(state, active, uint64(tick))
		if got := state.FarmlandBlockReads(); got != farmlandMoistureReadsPerTick {
			t.Fatalf("第 %d tick 读取=%d，想要 %d", tick+1, got, farmlandMoistureReadsPerTick)
		}
		if got := state.FarmlandRescanCursor(); got != wantCursor {
			t.Fatalf("第 %d tick 游标=%d，想要 %d", tick+1, got, wantCursor)
		}
	}
	advanceFarmlandMoistureTest(state, active, 3)
	if got := state.FarmlandBlockReads(); got != 24_576 {
		t.Fatalf("第 4 tick 读取=%d，想要 24576", got)
	}
	if got := state.FarmlandRescanPendingLen(); got != 0 {
		t.Fatalf("第 4 tick 后仍有 %d 个重扫 job，想要 0", got)
	}
	if state.FarmlandRescanCursor() != 0 {
		t.Fatalf("重扫完成后游标=%d，想要 0", state.FarmlandRescanCursor())
	}
}

// TestFarmlandMoistureRescanEventsConsumeBudgetFirst 锁定事件候选先于游标推进。
func TestFarmlandMoistureRescanEventsConsumeBudgetFirst(t *testing.T) {
	state := readyFarmlandMoistureState(t, core.ChunkPos{})
	key := core.ChunkKey{Dimension: core.Overworld}
	state.environment.farmlandMoisture.rescans.enqueueChunk(key)
	for x := int32(0); x < 10; x++ {
		state.EnqueueFarmlandMoisture(core.Overworld, core.BlockPos{X: x, Y: core.MinY})
	}

	advanceFarmlandMoistureTest(state, []core.ChunkKey{key}, 0)
	if got := state.FarmlandBlockReads(); got != farmlandMoistureReadsPerTick {
		t.Fatalf("事件加重扫读取=%d，想要 %d", got, farmlandMoistureReadsPerTick)
	}
	if got := state.FarmlandRescanCursor(); got != 65_526 {
		t.Fatalf("10 个事件后的重扫游标=%d，想要 65526", got)
	}
}

// TestFarmlandMoistureRescanDropsOutOfScopeAndRestarts 锁定离开 scope 后从零重扫。
func TestFarmlandMoistureRescanDropsOutOfScopeAndRestarts(t *testing.T) {
	state := readyFarmlandMoistureState(t, core.ChunkPos{})
	key := core.ChunkKey{Dimension: core.Overworld}
	state.environment.scope = map[core.ChunkKey]struct{}{key: {}}
	state.environment.farmlandMoisture.rescans.enqueueChunk(key)
	state.runFarmlandMoistureRescans(17)
	if got := state.FarmlandRescanCursor(); got != 17 {
		t.Fatalf("首段重扫游标=%d，想要 17", got)
	}

	delete(state.environment.scope, key)
	advanceFarmlandMoistureTest(state, nil, 0)
	if state.FarmlandRescanPendingLen() != 0 || state.FarmlandRescanCursor() != 0 {
		t.Fatalf("离开 scope 后重扫未清空：pending=%d cursor=%d",
			state.FarmlandRescanPendingLen(), state.FarmlandRescanCursor())
	}
	if _, queued := state.environment.farmlandMoisture.rescans.queued[key]; queued {
		t.Fatal("离开 scope 的 job 仍在去重集合中")
	}

	state.environment.scope = map[core.ChunkKey]struct{}{key: {}}
	state.environment.farmlandMoisture.rescans.enqueueChunk(key)
	state.runFarmlandMoistureRescans(1)
	if got := state.FarmlandRescanCursor(); got != 1 {
		t.Fatalf("重新进入后的游标=%d，想要从零推进到 1", got)
	}
}

func finishFarmlandMoistureRescan(t *testing.T, state *State) {
	t.Helper()
	for tick := 0; state.FarmlandRescanPendingLen() > 0; tick++ {
		if tick == 10 {
			t.Fatalf("重扫在 10 tick 内没有完成，仍有 %d 个 job", state.FarmlandRescanPendingLen())
		}
		state.environment.farmlandMoisture.blockReads = 0
		state.runFarmlandMoistureRescans(farmlandMoistureReadsPerTick)
	}
}

// TestFarmlandMoistureRescanOnlyQueuesActiveReadyFarmland 锁定 halo 的 scope 门禁。
func TestFarmlandMoistureRescanOnlyQueuesActiveReadyFarmland(t *testing.T) {
	center := core.ChunkPos{}
	left := core.ChunkPos{X: -1}
	state := readyFarmlandMoistureState(t, center, left)
	centerKey := core.ChunkKey{Dimension: core.Overworld, Pos: center}
	leftKey := core.ChunkKey{Dimension: core.Overworld, Pos: left}
	centerFarmland := core.BlockPos{X: 0, Y: core.MinY, Z: 0}
	leftFarmland := core.BlockPos{X: -1, Y: core.MinY, Z: 0}
	state.ResetFarmlandMoisture()
	setFarmlandMoistureTestBlock(t, state, centerFarmland, core.FarmlandWetID)
	setFarmlandMoistureTestBlock(t, state, leftFarmland, core.FarmlandWetID)

	state.environment.scope = map[core.ChunkKey]struct{}{centerKey: {}}
	state.environment.farmlandMoisture.rescans.enqueueChunk(centerKey)
	finishFarmlandMoistureRescan(t, state)
	if !state.FarmlandQueued(core.Overworld, centerFarmland) {
		t.Fatal("目标区块内的 active Ready 耕地没有被重扫入队")
	}
	if state.FarmlandQueued(core.Overworld, leftFarmland) {
		t.Fatal("halo 中自身区块不在 scope 的耕地被错误入队")
	}
	if got, _ := state.Dimension(core.Overworld).BlockAt(centerFarmland); got != core.FarmlandWetID {
		t.Fatalf("重扫 tick 内候选已被处理成 %d，候选应从下一 tick 才开始处理", got)
	}

	state.environment.scope[leftKey] = struct{}{}
	state.environment.farmlandMoisture.rescans.enqueueChunk(centerKey)
	finishFarmlandMoistureRescan(t, state)
	if !state.FarmlandQueued(core.Overworld, leftFarmland) {
		t.Fatal("halo 中自身区块 active Ready 的耕地没有被入队")
	}
}
