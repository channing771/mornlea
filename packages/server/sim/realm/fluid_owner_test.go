package realm

import (
	"testing"

	"github.com/channing771/mornlea/packages/server/fluid"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

func fluidOwnerFixture(t *testing.T, id core.DimensionID) (*State, core.ChunkKey) {
	t.Helper()
	state := NewState(id)
	key := core.ChunkKey{Dimension: id}
	dimension := state.Dimension(id)
	chunk := world.NewChunk(key.Pos)
	for x := range core.SectionSize {
		for z := range core.SectionSize {
			chunk.SetBlock(x, 0, z, core.StoneID)
		}
	}
	// 封闭水池提供大量可证不动点，孤立水源提供真实推进项。
	for x := 3; x <= 12; x++ {
		for z := 3; z <= 12; z++ {
			if x == 3 || x == 12 || z == 3 || z == 12 {
				chunk.SetBlock(x, 1, z, core.StoneID)
				continue
			}
			chunk.SetBlock(x, 1, z, core.WaterSourceID)
		}
	}
	chunk.SetBlock(1, 1, 1, core.WaterSourceID)
	chunk.Compact()
	if !dimension.BeginGeneration(key.Pos) {
		t.Fatal("流体 owner 区块未开始生成")
	}
	if err := dimension.ApplyGenerated(key.Pos, chunk); err != nil {
		t.Fatal(err)
	}
	return state, key
}

func settleOwnerQueue(
	t *testing.T,
	state *State,
	key core.ChunkKey,
	queue *fluid.Queue,
) {
	t.Helper()
	scope := map[core.ChunkKey]struct{}{key: {}}
	for tick := uint64(1); tick <= 1000; tick++ {
		if queue.Len() == 0 {
			return
		}
		mutation := state.NewMutation()
		queue.Advance(tick, &fluidWorld{
			state: state, id: key.Dimension, dimension: state.Dimension(key.Dimension),
			scope: scope, mutation: mutation,
		}, 1<<20, 1)
		mutation.Commit()
	}
	t.Fatalf("流体队列在上限内没有排空，剩余 %d", queue.Len())
}

func TestFluidWorldTreatsOutOfScopeAsUnreplaceable(t *testing.T) {
	state, key := fluidOwnerFixture(t, core.Overworld)
	dimension := state.Dimension(core.Overworld)
	outsideKey := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 9}}
	outside := world.NewChunk(outsideKey.Pos)
	outside.SetBlock(0, 5, 0, core.WaterSourceID)
	if !dimension.BeginGeneration(outsideKey.Pos) {
		t.Fatal("范围外区块未开始生成")
	}
	if err := dimension.ApplyGenerated(outsideKey.Pos, outside); err != nil {
		t.Fatal(err)
	}
	mutation := state.NewMutation()
	adapter := &fluidWorld{
		state: state, id: core.Overworld, dimension: dimension,
		scope: map[core.ChunkKey]struct{}{key: {}}, mutation: mutation,
	}
	inScopeAir := core.BlockPos{X: 2, Y: 5, Z: 2}
	if got := adapter.BlockAt(inScopeAir); got != core.AirID {
		t.Fatalf("范围内空气=%d", got)
	}
	for _, position := range []core.BlockPos{
		{X: 9 << core.SectionShift, Y: 5},
		{X: 400, Y: 5, Z: 400},
		{X: 1, Y: core.MinY - 1, Z: 1},
		{X: 1, Y: core.MaxY, Z: 1},
	} {
		if got := adapter.BlockAt(position); got != core.BarrierID {
			t.Fatalf("范围外 %+v=%d，想要屏障", position, got)
		}
	}
	position := core.BlockPos{X: 9 << core.SectionShift, Y: 5}
	adapter.SetBlock(position, core.AirID)
	if mutation.Len() != 0 {
		t.Fatalf("范围外写入登记了 %d 个区块", mutation.Len())
	}
	if block, _ := dimension.BlockAt(position); block != core.WaterSourceID {
		t.Fatalf("范围外水被改写为 %d", block)
	}
}

func TestFluidRescanFixedPointSkipMatchesFullRescan(t *testing.T) {
	fastState, key := fluidOwnerFixture(t, core.Overworld)
	fullState, _ := fluidOwnerFixture(t, core.Overworld)
	fastQueue := fluid.NewQueue()
	fastState.rescanChunkFluids(
		fastQueue, fastState.Dimension(core.Overworld), key.Pos, 0, 1, 1<<30,
	)
	fullQueue := fluid.NewQueue()
	fullChunk, _ := fullState.Dimension(core.Overworld).ReadyChunk(key.Pos)
	for y := int32(core.MinY); y < int32(core.MaxY); y++ {
		for x := range core.SectionSize {
			for z := range core.SectionSize {
				if core.IsFluid(fullChunk.BlockAt(x, y, z)) {
					fullQueue.Enqueue(core.BlockPos{X: int32(x), Y: y, Z: int32(z)}, 0, 1)
				}
			}
		}
	}
	if fastQueue.Len() == 0 || fastQueue.Len() >= fullQueue.Len() {
		t.Fatalf("捷径入队=%d，朴素入队=%d，夹具未覆盖跳过与真实推进", fastQueue.Len(), fullQueue.Len())
	}
	settleOwnerQueue(t, fastState, key, fastQueue)
	settleOwnerQueue(t, fullState, key, fullQueue)
	fastChunk, _ := fastState.Dimension(core.Overworld).ReadyChunk(key.Pos)
	settledFullChunk, _ := fullState.Dimension(core.Overworld).ReadyChunk(key.Pos)
	if fastChunk.Hash() != settledFullChunk.Hash() {
		t.Fatal("不动点捷径与朴素重扫的最终世界不同")
	}
}

func TestFluidRescanSpreadsAcrossTicksAndStaysComplete(t *testing.T) {
	state, key := fluidOwnerFixture(t, core.Overworld)
	state.environment.scope = map[core.ChunkKey]struct{}{key: {}}
	state.environment.config.FluidRescanCellsPerTick = 1
	state.environment.fluidRescan.enqueueChunk(key)
	ticks := 0
	for len(state.environment.fluidRescan.pending) != 0 {
		ticks++
		if ticks > 1000 {
			t.Fatal("分摊重扫没有结束")
		}
		state.runFluidRescans(0, 1)
	}
	if ticks < 2 {
		t.Fatalf("重扫只用了 %d tick，预算未分摊工作", ticks)
	}
	spreadCount := state.environment.fluidQueue(core.Overworld).Len()
	reference, _ := fluidOwnerFixture(t, core.Overworld)
	referenceQueue := fluid.NewQueue()
	reference.rescanChunkFluids(
		referenceQueue, reference.Dimension(core.Overworld), key.Pos, 0, 1, 1<<30,
	)
	if spreadCount != referenceQueue.Len() {
		t.Fatalf("分摊重扫入队=%d，一次性=%d", spreadCount, referenceQueue.Len())
	}
}

func TestFluidRescanDropsChunkThatLeavesScope(t *testing.T) {
	state, key := fluidOwnerFixture(t, core.Overworld)
	state.environment.scope = map[core.ChunkKey]struct{}{key: {}}
	state.environment.config.FluidRescanCellsPerTick = 1
	state.environment.fluidRescan.enqueueChunk(key)
	state.runFluidRescans(0, 1)
	rescan := &state.environment.fluidRescan
	if len(rescan.pending) != 1 || rescan.plane == 0 && rescan.section == 0 {
		t.Fatalf("重扫没有停在中间：%+v", *rescan)
	}
	delete(state.environment.scope, key)
	state.runFluidRescans(0, 1)
	if len(rescan.pending) != 0 || len(rescan.queued) != 0 || rescan.plane != 0 || rescan.section != 0 {
		t.Fatalf("离开范围后重扫状态未清空：%+v", *rescan)
	}
	state.environment.scope[key] = struct{}{}
	rescan.enqueueChunk(key)
	state.environment.config.FluidRescanCellsPerTick = 1 << 20
	state.runFluidRescans(0, 1)
	if len(rescan.pending) != 0 || state.environment.fluidQueue(core.Overworld).Len() == 0 {
		t.Fatal("重新进入范围后没有从头完成重扫")
	}
}

func TestFluidRescanUsesQueueOfItsOwnDimension(t *testing.T) {
	const other = core.Overworld + 1
	state, key := fluidOwnerFixture(t, other)
	state.environment.scope = map[core.ChunkKey]struct{}{key: {}}
	state.environment.config.FluidRescanCellsPerTick = 1 << 20
	state.environment.fluidRescan.enqueueChunk(key)
	state.runFluidRescans(0, 1)
	if queue := state.environment.fluidQueues[other]; queue == nil || queue.Len() == 0 {
		t.Fatal("重扫没有写入所属维度的队列")
	}
	if queue := state.environment.fluidQueues[core.Overworld]; queue != nil && queue.Len() != 0 {
		t.Fatalf("主世界队列被污染，Len=%d", queue.Len())
	}
}
