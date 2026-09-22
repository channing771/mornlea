package realm

// grass_spread_test.go covers five random-tick grass-spread scenarios: adjacent
// grass converts surface dirt and records a batch; no neighbor or solid cover
// blocks spread; unready cross-chunk grass neither spreads nor loads; independent
// engines replay bitwise; and read work remains bounded.
//
// Tests drive production `AdvanceCrops`; the per-cell read-budget pin calls
// `advanceCropCell` only to isolate incremental reads. Sampling and rolls stay on
// the real path. `grassSpreadHittingTicks` selects observable ticks through the
// same package-local pure functions without reproducing production logic.

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/server/updates"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// `grassSpreadSeed` makes selected hit ticks reproducible within each test.
const grassSpreadSeed = int64(0x6ea11c0ffee)

// `grassSpreadSamples` uses the tunable maximum of 64 so a cell is selected
// often enough for tests to observe spread within tens of ticks.
const grassSpreadSamples = 64

// `grassSpreadConfig` uses clear spring-dawn climate so the snow fallback does
// not write and grass spread is the fixture's only random-tick writer.
func grassSpreadConfig() EnvironmentConfig {
	return EnvironmentConfig{
		RandomTicksPerSection: grassSpreadSamples,
		DropPickupDelayTicks:  10,
	}
}

// `grassSpreadKey` is the fixture's only active chunk.
var grassSpreadKey = core.ChunkKey{Dimension: core.Overworld}

// `stepGrassSpread` drives and commits one random-tick phase, returning its
// published chunk batches or an empty slice when nothing changed.
func stepGrassSpread(t *testing.T, state *State, tick uint64) []ChunkChangeBatch {
	t.Helper()
	state.SetEnvironmentTick(tick, grassSpreadSeed, grassSpreadConfig())
	mutation := state.NewMutation()
	state.AdvanceCrops([]core.ChunkKey{grassSpreadKey}, mutation)
	return mutation.Commit()
}

// `grassSpreadHittingTicks` finds the first `count` ticks where sampling selects
// `position` and `sampler.GrassSpreadRoll` equals `hit`. It uses the same pure
// sampler inputs as `AdvanceCrops`, so negative tests prove the cell was examined
// and rejected by the intended rule.
func grassSpreadHittingTicks(t *testing.T, position core.BlockPos, hit bool, count int) []uint64 {
	t.Helper()
	key := core.ChunkKey{Dimension: core.Overworld, Pos: position.Chunk()}
	localX, localY, localZ := position.Local()
	cell := localX | localZ<<core.SectionShift | localY<<(core.SectionShift*2)
	ticks := make([]uint64, 0, count)
	var scratch []int
	for tick := uint64(0); tick < 1<<20; tick++ {
		scratch = sampler.SampleCells(grassSpreadSeed, tick, key, position.SectionIndex(), grassSpreadSamples, scratch)
		sampled := false
		for _, candidate := range scratch {
			if candidate == cell {
				sampled = true
				break
			}
		}
		if sampled && sampler.GrassSpreadRoll(grassSpreadSeed, tick, key.Dimension, position) == hit {
			ticks = append(ticks, tick)
			if len(ticks) == count {
				return ticks
			}
		}
	}
	t.Fatalf("2^20 个 tick 内没有找到 %d 个「抽样命中且骰子=%v」的 tick", count, hit)
	return nil
}

// Scenario: surface dirt next to grass becomes grass.

// `TestGrassSpreadConvertsSurfaceDirtNextToGrass` proves a sampled roll hit beside
// grass writes the dirt, publishes it in the tick batch, advances revision once,
// and preserves the source grass.
func TestGrassSpreadConvertsSurfaceDirtNextToGrass(t *testing.T) {
	dirt := core.BlockPos{X: 8, Y: 1, Z: 8}
	source := core.BlockPos{X: 9, Y: 1, Z: 8}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		dirt:   core.DirtID,
		source: core.GrassID,
	})

	tick := grassSpreadHittingTicks(t, dirt, true, 1)[0]
	batches := stepGrassSpread(t, state, tick)
	if len(batches) != 1 {
		t.Fatalf("蔓延提交了 %d 个区块批次，想要 1", len(batches))
	}
	if batches[0].NewRevision != batches[0].BaseRevision+1 {
		t.Fatalf("revision %d→%d，想要恰好推进一次",
			batches[0].BaseRevision, batches[0].NewRevision)
	}
	if len(batches[0].Changes) != 1 ||
		batches[0].Changes[0].Position != dirt ||
		batches[0].Changes[0].Block != core.GrassID {
		t.Fatalf("当 tick 批次登记了 %+v，想要恰好一格 %+v→草方块",
			batches[0].Changes, dirt)
	}
	if got, ready := dimension.BlockAt(dirt); !ready || got != core.GrassID {
		t.Fatalf("泥土格 %+v=%d（ready=%v），想要草方块", dirt, got, ready)
	}
	if got, _ := dimension.BlockAt(source); got != core.GrassID {
		t.Fatalf("草源格 %+v=%d，想要保持草方块", source, got)
	}
}

// `TestGrassSpreadUnderSnowCover` pins seasonal snow as non-solid cover. The
// four `core.IsSnowLayer` levels remain above the surface while spread changes
// only the dirt block beneath them.
func TestGrassSpreadUnderSnowCover(t *testing.T) {
	dirt := core.BlockPos{X: 8, Y: 1, Z: 8}
	source := core.BlockPos{X: 9, Y: 1, Z: 8}
	snowAbove := core.BlockPos{X: 8, Y: 2, Z: 8}
	if !core.IsSnowLayer(core.SnowLayer2BlockID) {
		t.Fatal("夹具失效：SnowLayer2BlockID 不是雪层方块")
	}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		dirt:      core.DirtID,
		source:    core.GrassID,
		snowAbove: core.SnowLayer2BlockID,
	})

	tick := grassSpreadHittingTicks(t, dirt, true, 1)[0]
	batches := stepGrassSpread(t, state, tick)
	if len(batches) != 1 || len(batches[0].Changes) != 1 ||
		batches[0].Changes[0].Position != dirt ||
		batches[0].Changes[0].Block != core.GrassID {
		t.Fatalf("雪层下的蔓延批次=%+v，想要恰好一格泥土→草", batches)
	}
	if got, _ := dimension.BlockAt(snowAbove); got != core.SnowLayer2BlockID {
		t.Fatalf("上方雪层=%d，想要保持 2 档原样", got)
	}
}

// Scenario: no grass neighbor or solid cover prevents spread.

// `TestGrassSpreadSkipsWithoutGrassNeighborOrUnderSolidCover` selects real sample
// and roll hits for two rejection paths: four dirt neighbors without a source,
// and stone overhead. Both preserve their blocks and publish no writes.
func TestGrassSpreadSkipsWithoutGrassNeighborOrUnderSolidCover(t *testing.T) {
	isolated := core.BlockPos{X: 4, Y: 1, Z: 4}
	noNeighbor := map[core.BlockPos]core.BlockID{isolated: core.DirtID}
	// Four dirt neighbors exclude a degenerate implementation that inspects only
	// air neighbors while still providing no grass source.
	for _, offset := range []core.BlockPos{{X: 1}, {X: -1}, {Z: 1}, {Z: -1}} {
		noNeighbor[core.BlockPos{
			X: isolated.X + offset.X, Y: isolated.Y, Z: isolated.Z + offset.Z,
		}] = core.DirtID
	}

	covered := core.BlockPos{X: 8, Y: 1, Z: 8}
	withCover := map[core.BlockPos]core.BlockID{
		covered:                        core.DirtID,
		{X: covered.X + 1, Y: 1, Z: 8}: core.GrassID,
		{X: covered.X, Y: covered.Y + 1, Z: covered.Z}: core.StoneID,
	}

	for name, fixture := range map[string]struct {
		blocks  map[core.BlockPos]core.BlockID
		target  core.BlockPos
		extra   core.BlockPos
		extraID core.BlockID
	}{
		"无草邻":    {blocks: noNeighbor, target: isolated, extra: core.BlockPos{X: 5, Y: 1, Z: 4}, extraID: core.DirtID},
		"实体方块覆盖": {blocks: withCover, target: covered, extra: core.BlockPos{X: 8, Y: 2, Z: 8}, extraID: core.StoneID},
	} {
		state, dimension := wildPlantFixture(fixture.blocks)
		for _, tick := range grassSpreadHittingTicks(t, fixture.target, true, 3) {
			if batches := stepGrassSpread(t, state, tick); len(batches) != 0 {
				t.Fatalf("%s 夹具在骰子命中的 tick %d 发生了写入：%+v", name, tick, batches)
			}
		}
		if got, _ := dimension.BlockAt(fixture.target); got != core.DirtID {
			t.Fatalf("%s：泥土格 %+v=%d，想要保持泥土", name, fixture.target, got)
		}
		if got, _ := dimension.BlockAt(fixture.extra); got != fixture.extraID {
			t.Fatalf("%s：关联格 %+v=%d，想要保持 %d", name, fixture.extra, got, fixture.extraID)
		}
	}
}

// Scenario: an unready adjacent chunk neither spreads nor loads synchronously.

// `TestGrassSpreadCrossChunkUnreadyNeighborAborts` places dirt at the east edge
// and its only grass source across the boundary. While that chunk is absent, a
// roll hit must neither spread nor load it. Once ready, retrying the same tick
// spreads and records only the dirt's chunk.
func TestGrassSpreadCrossChunkUnreadyNeighborAborts(t *testing.T) {
	dirt := core.BlockPos{X: core.SectionSize - 1, Y: 1, Z: 8}
	neighborChunk := core.ChunkPos{X: 1}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{dirt: core.DirtID})

	tick := grassSpreadHittingTicks(t, dirt, true, 1)[0]
	batches := stepGrassSpread(t, state, tick)
	if len(batches) != 0 {
		t.Fatalf("邻区块未就绪却发生了写入：%+v", batches)
	}
	if got, _ := dimension.BlockAt(dirt); got != core.DirtID {
		t.Fatalf("邻区块未就绪时泥土格=%d，想要保持泥土", got)
	}
	if _, ready := dimension.ReadyChunk(neighborChunk); ready {
		t.Fatalf("邻区块 %+v 被同步加载为就绪，读取必须只看已就绪区块", neighborChunk)
	}

	neighbor := world.NewChunk(neighborChunk)
	neighbor.SetBlock(0, 1, 8, core.GrassID)
	neighbor.Compact()
	if !dimension.BeginGeneration(neighborChunk) {
		t.Fatalf("邻区块 %+v 未开始生成", neighborChunk)
	}
	if err := dimension.ApplyGenerated(neighborChunk, neighbor); err != nil {
		t.Fatalf("邻区块 %+v 生成失败：%v", neighborChunk, err)
	}
	batches = stepGrassSpread(t, state, tick)
	if len(batches) != 1 || len(batches[0].Changes) != 1 ||
		batches[0].Changes[0].Position != dirt ||
		batches[0].Changes[0].Block != core.GrassID {
		t.Fatalf("邻区块就绪后同一 tick 重试的批次=%+v，想要恰好一格泥土→草", batches)
	}
	if batches[0].Dimension != core.Overworld || batches[0].Chunk != (core.ChunkPos{}) {
		t.Fatalf("蔓延写入了非夹具区块 dim=%d %+v", batches[0].Dimension, batches[0].Chunk)
	}
}

// Scenario: replay is deterministic.

// `grassSpreadChange` is the minimal replay observation: position and new block.
type grassSpreadChange struct {
	position core.BlockPos
	block    core.BlockID
}

// `TestGrassSpreadReplaysIdentically` compares per-tick changes and final chunk
// hashes from two independent engines with identical initial state. It also
// requires actual spread so an inert pair cannot pass vacuously.
func TestGrassSpreadReplaysIdentically(t *testing.T) {
	const ticks = 256
	blocks := func() map[core.BlockPos]core.BlockID {
		// Fill y=1 with dirt except for two diagonal grass seeds, producing enough
		// changes over 256 ticks for a meaningful replay comparison.
		layer := make(map[core.BlockPos]core.BlockID, core.SectionSize*core.SectionSize)
		for x := range int32(core.SectionSize) {
			for z := range int32(core.SectionSize) {
				layer[core.BlockPos{X: x, Y: 1, Z: z}] = core.DirtID
			}
		}
		layer[core.BlockPos{X: 0, Y: 1, Z: 0}] = core.GrassID
		layer[core.BlockPos{X: core.SectionSize - 1, Y: 1, Z: core.SectionSize - 1}] = core.GrassID
		return layer
	}
	run := func() (perTick [][]grassSpreadChange, after [32]byte) {
		state, dimension := wildPlantFixture(blocks())
		for tick := range uint64(ticks) {
			var changes []grassSpreadChange
			for _, batch := range stepGrassSpread(t, state, tick) {
				for _, change := range batch.Changes {
					changes = append(changes, grassSpreadChange{position: change.Position, block: change.Block})
				}
			}
			perTick = append(perTick, changes)
		}
		chunk, ready := dimension.ReadyChunk(core.ChunkPos{})
		if !ready {
			t.Fatal("蔓延夹具区块未就绪")
		}
		return perTick, chunk.Hash()
	}
	firstTicks, firstHash := run()
	secondTicks, secondHash := run()
	written := 0
	for _, changes := range firstTicks {
		written += len(changes)
	}
	if written == 0 {
		t.Fatalf("%d 个 tick 里世界一动没动，重放一致的断言恒真", ticks)
	}
	equal := func(a, b grassSpreadChange) bool { return a == b }
	if !slices.EqualFunc(firstTicks, secondTicks, func(a, b []grassSpreadChange) bool {
		return slices.EqualFunc(a, b, equal)
	}) {
		t.Fatal("同 seed 同 tick 序列的逐 tick 变更序列不同，蔓延不是确定性派生")
	}
	if firstHash != secondHash {
		t.Fatalf("重放 %d tick 后区块 Hash 不同：%x 与 %x", ticks, firstHash, secondHash)
	}
}

// Scenario: read work is bounded.

// `TestGrassSpreadReadsNeighborsOnlyOnRollHit` proves a miss reads no neighbors,
// leaving the cell plus snow-overhead baseline of two reads. A successful hit
// reads overhead and all four neighbors by placing grass last in probe order.
func TestGrassSpreadReadsNeighborsOnlyOnRollHit(t *testing.T) {
	dirt := core.BlockPos{X: 8, Y: 1, Z: 8}
	source := core.BlockPos{X: 8, Y: 1, Z: 7}
	readsFor := func(t *testing.T, hit bool) int {
		t.Helper()
		state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
			dirt:   core.DirtID,
			source: core.GrassID,
		})
		chunk, ready := dimension.ReadyChunk(core.ChunkPos{})
		if !ready {
			t.Fatal("蔓延夹具区块未就绪")
		}
		tick := grassSpreadHittingTicks(t, dirt, hit, 1)[0]
		state.SetEnvironmentTick(tick, grassSpreadSeed, grassSpreadConfig())
		before := state.CropBlockReads()
		state.advanceCropCell(dimension, core.Overworld, chunk, dirt, tick, state.NewMutation())
		return state.CropBlockReads() - before
	}
	if missReads := readsFor(t, false); missReads != 2 {
		t.Fatalf("骰子未命中时读取=%d，想要 2（格自身与积雪兜底的上方判定）", missReads)
	}
	if hitReads := readsFor(t, true); hitReads != 6 {
		t.Fatalf("骰子命中且蔓延完成时读取=%d，想要 6（格自身、上方与水平四邻）", hitReads)
	}
}

// `TestGrassSpreadReadBudgetBoundedByExaminedCells` compares an all-grass surface
// with a dirt layer containing one grass source. Reads stay within seven per
// examined cell: one self read, up to five spread reads, and one snow fallback.
// The dirt world must read more, proving neighbor reads count toward the budget.
func TestGrassSpreadReadBudgetBoundedByExaminedCells(t *testing.T) {
	barren, _ := wildPlantFixture(nil)
	spreadableBlocks := make(map[core.BlockPos]core.BlockID, core.SectionSize*core.SectionSize)
	for x := range int32(core.SectionSize) {
		for z := range int32(core.SectionSize) {
			spreadableBlocks[core.BlockPos{X: x, Y: 1, Z: z}] = core.DirtID
		}
	}
	spreadableBlocks[core.BlockPos{X: 0, Y: 1, Z: 0}] = core.GrassID
	spreadable, _ := wildPlantFixture(spreadableBlocks)

	// Select a tick where a dirt cell is sampled and its roll hits, then advance
	// both worlds at that same tick so neighbor reads must be counted.
	probe := core.BlockPos{X: 8, Y: 1, Z: 8}
	tick := grassSpreadHittingTicks(t, probe, true, 1)[0]
	stepGrassSpread(t, barren, tick)
	stepGrassSpread(t, spreadable, tick)

	barrenExamined, barrenReads := barren.CropStats()
	spreadExamined, spreadReads := spreadable.CropStats()
	if barrenExamined == 0 {
		t.Fatal("草皮世界一格都没考察，读取上界断言恒真")
	}
	if barrenExamined != spreadExamined {
		t.Fatalf("两个世界的考察量不同：%d 与 %d", barrenExamined, spreadExamined)
	}
	if barrenReads > 7*barrenExamined {
		t.Fatalf("草皮世界读取=%d，超过考察格数 %d 的 7 倍", barrenReads, barrenExamined)
	}
	if spreadReads > 7*spreadExamined {
		t.Fatalf("满泥土世界读取=%d，超过考察格数 %d 的 7 倍", spreadReads, spreadExamined)
	}
	if spreadReads <= barrenReads {
		t.Fatalf("满泥土世界读取=%d 不大于草皮世界 %d，邻居读取没有计入预算",
			spreadReads, barrenReads)
	}
}

// Scenario: the decision stream is deterministic and independent.

// `TestGrassSpreadRollIsIndependentAndDeterministic` pins replay, an approximate
// 1/4 hit rate, and a salt distinct from every existing decision stream.
func TestGrassSpreadRollIsIndependentAndDeterministic(t *testing.T) {
	position := core.BlockPos{X: 8, Y: 1, Z: 8}
	first := sampler.GrassSpreadRoll(grassSpreadSeed, 12345, core.Overworld, position)
	if first != sampler.GrassSpreadRoll(grassSpreadSeed, 12345, core.Overworld, position) {
		t.Fatal("同输入两次的蔓延判定不同，判定不是纯函数")
	}
	for _, salt := range []uint64{
		updates.CropGrowthRollSalt, updates.CropYieldRollSalt, updates.FarmlandRevertRollSalt,
		updates.SaplingGrowthRollSalt, updates.ShortGrassSeedDropSalt,
	} {
		if updates.GrassSpreadRollSalt == salt {
			t.Fatalf("蔓延判定盐值与既有盐值 %#x 相同", salt)
		}
	}

	hits, total := 0, 0
	for tick := range uint64(8192) {
		for x := int32(0); x < 4; x++ {
			if sampler.GrassSpreadRoll(grassSpreadSeed, tick, core.Overworld, core.BlockPos{X: x, Y: 1, Z: 8}) {
				hits++
			}
			total++
		}
	}
	if total != 8192*4 {
		t.Fatalf("样本数=%d，想要 %d", total, 8192*4)
	}
	// The broad 1/8..1/2 interval rejects constant outputs and common wrong masks.
	if hits < total/8 || hits > total/2 {
		t.Fatalf("1/4 判定命中 %d/%d，比例异常", hits, total)
	}
}
