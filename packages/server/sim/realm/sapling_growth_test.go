package realm

// sapling_growth_test.go：随机 tick 的树苗生长。覆盖 spec 的「只在露天且空间
// 足够时按随机 tick 确定性生长」与「生长写入跨区块原子且失败零副作用」两组
// Scenario，以及 farming delta 的读取上界。
//
// 用例一律驱动生产入口 `AdvanceCrops`（读取预算用例例外：它直接调用分发器
// `advanceCropCell` 以隔离单格的读取增量），抽样、判定、几何与写入都在真实
// 路径上。测试只负责把夹具与 tick 选到「该观察的那一步」——`saplingGrowthTick`
// 用包内同一条抽样与判定函数找出同时命中抽样与判定的 tick，因此不复制任何
// 生产逻辑。

import (
	"testing"

	"github.com/channing771/mornlea/packages/server/updates"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

// saplingGrowthSeed 是生长用例的世界种子：固定值让命中 tick 与几何期望都能在
// 用例内现算。
const saplingGrowthSeed = int64(0x5a911eaf)

// saplingGrowthSamples 是生长用例的每区段抽样数：取 64（tunables 上限）让单格
// 每 tick 被抽中的概率足够高，用例在几十个 tick 内就能观察到生长。
const saplingGrowthSamples = 64

// saplingGrowthConfig 返回生长用例的环境参数快照。
func saplingGrowthConfig() EnvironmentConfig {
	return EnvironmentConfig{
		RandomTicksPerSection: saplingGrowthSamples,
		DropPickupDelayTicks:  10,
	}
}

// saplingGrowthKey 是生长用例唯一的活动区块键。
var saplingGrowthKey = core.ChunkKey{Dimension: core.Overworld}

// stepSaplingGrowth 以给定 tick 驱动一次随机 tick 阶段并提交事务，返回本次提交
// 的区块变更批次；未产生任何写入时批次为空。
func stepSaplingGrowth(
	t *testing.T,
	state *State,
	seed int64,
	tick uint64,
	config EnvironmentConfig,
) []ChunkChangeBatch {
	t.Helper()
	state.SetEnvironmentTick(tick, seed, config)
	mutation := state.NewMutation()
	state.AdvanceCrops([]core.ChunkKey{saplingGrowthKey}, mutation)
	return mutation.Commit()
}

// saplingGrowthTick 返回最小的 tick：随机抽样在该 tick 命中 position 所在格，
// 且 `sampler.SaplingGrowthRoll` 的结果等于 hit。用例据此在确定步数内把一次真实的生长
// 尝试固定在「判定命中」或「判定未命中」两种状态上。
func saplingGrowthTick(
	t *testing.T,
	seed int64,
	key core.ChunkKey,
	samples int,
	position core.BlockPos,
	hit bool,
) uint64 {
	t.Helper()
	localX, _, localZ := position.Local()
	localY := int(position.Y-core.MinY) & core.SectionMask
	cell := localX | localZ<<core.SectionShift | localY<<(core.SectionShift*2)
	for tick := range uint64(1 << 20) {
		sampled := false
		for _, candidate := range sampler.SampleCells(seed, tick, key, position.SectionIndex(), samples, nil) {
			if candidate == cell {
				sampled = true
				break
			}
		}
		if sampled && sampler.SaplingGrowthRoll(seed, tick, key.Dimension, position) == hit {
			return tick
		}
	}
	t.Fatalf("2^20 个 tick 内没有找到「抽样命中且判定=%v」的 tick", hit)
	return 0
}

// generateReadyChunk 把一个全空气区块生成并转为 Ready，供跨区块用例控制邻居
// 区块的就绪状态（空间校验只要求空气，因此全空气邻居天然通过）。
func generateReadyChunk(t *testing.T, dimension *Dimension, pos core.ChunkPos) {
	t.Helper()
	chunk := world.NewChunk(pos)
	if !dimension.BeginGeneration(pos) {
		t.Fatalf("区块 %+v 未开始生成", pos)
	}
	if err := dimension.ApplyGenerated(pos, chunk); err != nil {
		t.Fatalf("区块 %+v 生成失败：%v", pos, err)
	}
}

// saplingGrowthGeometry 返回 seed 与根坐标下 engine 给出的树形几何，供期望值
// 现算；几何为空时直接失败（夹具失效）。
func saplingGrowthGeometry(t *testing.T, seed int64, root core.BlockPos) []worldgen.TreeBlock {
	t.Helper()
	records := worldgen.TreeBlocks(seed, root)
	if len(records) == 0 {
		t.Fatalf("engine 对根 %+v 返回空几何", root)
	}
	return records
}

// saplingGrowthTarget 把一条几何记录换算成世界坐标。
func saplingGrowthTarget(root core.BlockPos, record worldgen.TreeBlock) core.BlockPos {
	return core.BlockPos{
		X: root.X + int32(record.DX),
		Y: root.Y + int32(record.DY),
		Z: root.Z + int32(record.DZ),
	}
}

// saplingGrowthObstruction 返回几何里第一条水平偏移非零的记录（树冠格），供
// 「空间不足」与「覆盖短草」用例选一个几何确实会写入、且与树苗不同列的目标格。
// 必须排除同列记录：树干格落在树苗正上方，占用它挡的是露天判定而非空间校验。
func saplingGrowthObstruction(t *testing.T, records []worldgen.TreeBlock) worldgen.TreeBlock {
	t.Helper()
	for _, record := range records {
		if record.DX != 0 || record.DZ != 0 {
			return record
		}
	}
	t.Fatalf("几何没有水平偏移非零的记录，无法构造占用格")
	return worldgen.TreeBlock{}
}

// TestSaplingGrowthReplaysIdentically 覆盖 spec Scenario「相同输入重放一致」：
// 两个同构世界推进相同 tick 数后区块 Hash 逐格一致，且世界确实发生了变化
// （否则两个什么都没发生的世界也会一致，断言恒真）。
func TestSaplingGrowthReplaysIdentically(t *testing.T) {
	const ticks = 256
	saplings := map[core.BlockPos]core.BlockID{}
	for _, x := range []int32{1, 9} {
		for _, z := range []int32{1, 9} {
			saplings[core.BlockPos{X: x, Y: 1, Z: z}] = core.SaplingID
		}
	}
	run := func() (before, after [32]byte) {
		state, dimension := wildPlantFixture(saplings)
		chunk, ready := dimension.ReadyChunk(core.ChunkPos{})
		if !ready {
			t.Fatal("生长夹具区块未就绪")
		}
		before = chunk.Hash()
		for tick := range uint64(ticks) {
			stepSaplingGrowth(t, state, saplingGrowthSeed, tick, saplingGrowthConfig())
		}
		after = chunk.Hash()
		return before, after
	}
	firstBefore, firstAfter := run()
	secondBefore, secondAfter := run()
	if firstBefore != secondBefore {
		t.Fatalf("两次的初始世界就不同，夹具本身不确定")
	}
	if firstAfter == firstBefore {
		t.Fatalf("%d 个 tick 里世界一动没动，重放一致的断言恒真", ticks)
	}
	if firstAfter != secondAfter {
		t.Fatalf("重放 %d 个 tick 后区块 Hash 不同：%x 与 %x", ticks, firstAfter, secondAfter)
	}
}

// TestSaplingGrowthAppliesEngineGeometry 覆盖正面路径：抽样与判定同时命中的
// 树苗被 engine 几何替换——树苗格变为树干底，其余记录逐格落在几何指定的方块
// 上，受影响区块的 revision 恰好推进一次，且不产生任何掉落物。
func TestSaplingGrowthAppliesEngineGeometry(t *testing.T) {
	sapling := core.BlockPos{X: 8, Y: 1, Z: 8}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{sapling: core.SaplingID})
	records := saplingGrowthGeometry(t, saplingGrowthSeed, sapling)

	tick := saplingGrowthTick(t, saplingGrowthSeed, saplingGrowthKey, saplingGrowthSamples, sapling, true)
	batches := stepSaplingGrowth(t, state, saplingGrowthSeed, tick, saplingGrowthConfig())
	if len(batches) != 1 {
		t.Fatalf("生长提交了 %d 个区块批次，想要 1", len(batches))
	}
	if batches[0].NewRevision != batches[0].BaseRevision+1 {
		t.Fatalf("revision %d→%d，想要恰好推进一次", batches[0].BaseRevision, batches[0].NewRevision)
	}
	changes := map[core.BlockPos]core.BlockID{}
	for _, change := range batches[0].Changes {
		changes[change.Position] = change.Block
	}
	if len(changes) != len(records) {
		t.Fatalf("登记了 %d 个变更格，几何有 %d 条记录", len(changes), len(records))
	}
	for _, record := range records {
		target := saplingGrowthTarget(sapling, record)
		if changes[target] != record.Block {
			t.Fatalf("变更格 %+v 登记为 %d，想要几何记录 %d", target, changes[target], record.Block)
		}
		if got, ready := dimension.BlockAt(target); !ready || got != record.Block {
			t.Fatalf("世界格 %+v 为 %d（ready=%v），想要几何记录 %d", target, got, ready, record.Block)
		}
	}
	if got, _ := dimension.BlockAt(sapling); got != core.OakLogID {
		t.Fatalf("树苗格=%d，想要树干底 %d", got, core.OakLogID)
	}
	chunk, ready := dimension.ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("生长后的区块未就绪")
	}
	for slot := range core.DropsPerChunk {
		if drop := chunk.Drop(slot); drop.Active {
			t.Fatalf("生长产生了掉落物 %+v，想要零掉落", drop)
		}
	}
}

// TestSaplingGrowthSkipsNonSkyExposed 覆盖 spec Scenario「不露天的树苗不生长」：
// 树苗正上方存在任何非空气方块时，即使抽样与判定同时命中也不生长。
func TestSaplingGrowthSkipsNonSkyExposed(t *testing.T) {
	sapling := core.BlockPos{X: 8, Y: 1, Z: 8}
	roof := core.BlockPos{X: 8, Y: 2, Z: 8}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		sapling: core.SaplingID,
		roof:    core.StoneID,
	})

	tick := saplingGrowthTick(t, saplingGrowthSeed, saplingGrowthKey, saplingGrowthSamples, sapling, true)
	batches := stepSaplingGrowth(t, state, saplingGrowthSeed, tick, saplingGrowthConfig())
	if len(batches) != 0 {
		t.Fatalf("不露天的树苗发生了写入：%+v", batches)
	}
	if got, _ := dimension.BlockAt(sapling); got != core.SaplingID {
		t.Fatalf("不露天的树苗=%d，想要保持树苗", got)
	}
	if got, _ := dimension.BlockAt(roof); got != core.StoneID {
		t.Fatalf("顶盖格=%d，想要保持石头", got)
	}
}

// TestSaplingGrowthSkipsUnsupportedSapling 覆盖支撑不符的分支：正下方不再是
// 泥土或草地时，即使抽样与判定同时命中也不生长。
func TestSaplingGrowthSkipsUnsupportedSapling(t *testing.T) {
	sapling := core.BlockPos{X: 8, Y: 1, Z: 8}
	support := core.BlockPos{X: 8, Y: 0, Z: 8}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		sapling: core.SaplingID,
		support: core.StoneID,
	})

	tick := saplingGrowthTick(t, saplingGrowthSeed, saplingGrowthKey, saplingGrowthSamples, sapling, true)
	batches := stepSaplingGrowth(t, state, saplingGrowthSeed, tick, saplingGrowthConfig())
	if len(batches) != 0 {
		t.Fatalf("支撑不符的树苗发生了写入：%+v", batches)
	}
	if got, _ := dimension.BlockAt(sapling); got != core.SaplingID {
		t.Fatalf("支撑不符的树苗=%d，想要保持树苗", got)
	}
}

// TestSaplingGrowthRollMissKeepsSapling 覆盖 spec Scenario「判定未命中保持树苗」：
// 抽样命中但独立 salt 的 1/8 判定未命中时，树苗保持、零写入、零掉落，且没有
// 读取树形几何——读取量仍在「被考察格数 × 2」的既有上界内。
func TestSaplingGrowthRollMissKeepsSapling(t *testing.T) {
	sapling := core.BlockPos{X: 8, Y: 1, Z: 8}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{sapling: core.SaplingID})

	tick := saplingGrowthTick(t, saplingGrowthSeed, saplingGrowthKey, saplingGrowthSamples, sapling, false)
	batches := stepSaplingGrowth(t, state, saplingGrowthSeed, tick, saplingGrowthConfig())
	if len(batches) != 0 {
		t.Fatalf("判定未命中却发生了写入：%+v", batches)
	}
	if got, _ := dimension.BlockAt(sapling); got != core.SaplingID {
		t.Fatalf("判定未命中后树苗=%d，想要保持树苗", got)
	}
	examined, reads := state.CropStats()
	if examined == 0 {
		t.Fatal("随机 tick 阶段一格都没考察，读取上界断言恒真")
	}
	if reads > 2*examined {
		t.Fatalf("判定未命中时读取=%d，超过考察格数 %d 的两倍（疑似读了树形几何）", reads, examined)
	}
}

// TestSaplingGrowthRejectsOccupiedGeometry 覆盖 spec Scenario「空间不足零副作用」：
// 几何任一目标格被非空气、非短草方块占用时放弃本次生长，树苗、占用格与区块
// revision 全部保持不变。
func TestSaplingGrowthRejectsOccupiedGeometry(t *testing.T) {
	sapling := core.BlockPos{X: 8, Y: 1, Z: 8}
	records := saplingGrowthGeometry(t, saplingGrowthSeed, sapling)
	obstruction := saplingGrowthTarget(sapling, saplingGrowthObstruction(t, records))
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		sapling:     core.SaplingID,
		obstruction: core.StoneID,
	})
	info, ok := dimension.Info(core.ChunkPos{})
	if !ok {
		t.Fatal("夹具区块未就绪")
	}

	tick := saplingGrowthTick(t, saplingGrowthSeed, saplingGrowthKey, saplingGrowthSamples, sapling, true)
	batches := stepSaplingGrowth(t, state, saplingGrowthSeed, tick, saplingGrowthConfig())
	if len(batches) != 0 {
		t.Fatalf("空间不足却发生了写入：%+v", batches)
	}
	if got, _ := dimension.BlockAt(sapling); got != core.SaplingID {
		t.Fatalf("空间不足后树苗=%d，想要保持树苗", got)
	}
	if got, _ := dimension.BlockAt(obstruction); got != core.StoneID {
		t.Fatalf("占用格 %+v=%d，想要保持不变", obstruction, got)
	}
	after, _ := dimension.Info(core.ChunkPos{})
	if after.Revision != info.Revision {
		t.Fatalf("放弃生长后 revision %d→%d，想要不变", info.Revision, after.Revision)
	}
}

// TestSaplingGrowthOverwritesShortGrassWithoutDrops 覆盖 spec Scenario「覆盖短草
// 零掉落」：几何目标格当前是短草时生长照常完成，被覆盖的短草不产生任何掉落物。
func TestSaplingGrowthOverwritesShortGrassWithoutDrops(t *testing.T) {
	sapling := core.BlockPos{X: 8, Y: 1, Z: 8}
	records := saplingGrowthGeometry(t, saplingGrowthSeed, sapling)
	obstruction := saplingGrowthObstruction(t, records)
	grassCell := saplingGrowthTarget(sapling, obstruction)
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		sapling:   core.SaplingID,
		grassCell: core.ShortGrassID,
	})

	tick := saplingGrowthTick(t, saplingGrowthSeed, saplingGrowthKey, saplingGrowthSamples, sapling, true)
	batches := stepSaplingGrowth(t, state, saplingGrowthSeed, tick, saplingGrowthConfig())
	if len(batches) != 1 {
		t.Fatalf("覆盖短草的生长提交了 %d 个区块批次，想要 1", len(batches))
	}
	if got, _ := dimension.BlockAt(grassCell); got != obstruction.Block {
		t.Fatalf("被覆盖的短草格=%d，想要几何方块 %d", got, obstruction.Block)
	}
	chunk, ready := dimension.ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("生长后的区块未就绪")
	}
	for slot := range core.DropsPerChunk {
		if drop := chunk.Drop(slot); drop.Active {
			t.Fatalf("覆盖短草产生了掉落物 %+v，想要零掉落", drop)
		}
	}
}

// TestSaplingGrowthRejectsRootAboveEngineLimit 钉住根坐标的 engine 上界前置守卫：
// 最坏普通橡树的顶格在根上 8 格，`root.Y > MaxY-9` 的请求会被 engine 以硬状态
// 拒绝并让 Go 桥 panic；玩家可以在高处搭塔种苗，生长分支必须先挡下它。
func TestSaplingGrowthRejectsRootAboveEngineLimit(t *testing.T) {
	sapling := core.BlockPos{X: 8, Y: core.MaxY - 8, Z: 8}
	support := core.BlockPos{X: 8, Y: core.MaxY - 9, Z: 8}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{
		support: core.DirtID,
		sapling: core.SaplingID,
	})

	tick := saplingGrowthTick(t, saplingGrowthSeed, saplingGrowthKey, saplingGrowthSamples, sapling, true)
	batches := stepSaplingGrowth(t, state, saplingGrowthSeed, tick, saplingGrowthConfig())
	if len(batches) != 0 {
		t.Fatalf("越界根坐标发生了写入：%+v", batches)
	}
	if got, _ := dimension.BlockAt(sapling); got != core.SaplingID {
		t.Fatalf("越界根坐标的树苗=%d，想要保持树苗", got)
	}
	if got, _ := dimension.BlockAt(support); got != core.DirtID {
		t.Fatalf("支撑格=%d，想要保持泥土", got)
	}
}

// TestSaplingGrowthCrossChunkAbortsUntilNeighborReady 覆盖 spec Scenario
// 「跨区块未就绪零副作用」与「成功生长跨区块登记变更」：树苗贴近区块东界时
// 树冠跨到区块 (1,0)；邻居未就绪时整体放弃且零副作用，邻居就绪后同一 tick
// 重试成功，两个区块各登记本次改写的格并各推进一次 revision。
func TestSaplingGrowthCrossChunkAbortsUntilNeighborReady(t *testing.T) {
	sapling := core.BlockPos{X: core.SectionSize - 1, Y: 1, Z: 8}
	state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{sapling: core.SaplingID})
	records := saplingGrowthGeometry(t, saplingGrowthSeed, sapling)
	crosses := false
	for _, record := range records {
		if saplingGrowthTarget(sapling, record).Chunk() != sapling.Chunk() {
			crosses = true
			break
		}
	}
	if !crosses {
		t.Fatal("夹具失效：树形几何没有跨越区块边界")
	}
	before, ok := dimension.Info(core.ChunkPos{})
	if !ok {
		t.Fatal("夹具区块未就绪")
	}

	tick := saplingGrowthTick(t, saplingGrowthSeed, saplingGrowthKey, saplingGrowthSamples, sapling, true)
	batches := stepSaplingGrowth(t, state, saplingGrowthSeed, tick, saplingGrowthConfig())
	if len(batches) != 0 {
		t.Fatalf("邻居区块未就绪却发生了写入：%+v", batches)
	}
	if got, _ := dimension.BlockAt(sapling); got != core.SaplingID {
		t.Fatalf("邻居未就绪后树苗=%d，想要保持树苗", got)
	}
	if after, _ := dimension.Info(core.ChunkPos{}); after.Revision != before.Revision {
		t.Fatalf("放弃生长后 revision %d→%d，想要不变", before.Revision, after.Revision)
	}

	generateReadyChunk(t, dimension, core.ChunkPos{X: 1})
	batches = stepSaplingGrowth(t, state, saplingGrowthSeed, tick, saplingGrowthConfig())
	if len(batches) != 2 {
		t.Fatalf("跨区块生长提交了 %d 个区块批次，想要 2", len(batches))
	}
	seen := map[core.BlockPos]core.BlockID{}
	for _, batch := range batches {
		if batch.NewRevision != batch.BaseRevision+1 {
			t.Fatalf("区块 %+v revision %d→%d，想要恰好推进一次",
				batch.Chunk, batch.BaseRevision, batch.NewRevision)
		}
		for _, change := range batch.Changes {
			if change.Block != core.OakLogID && change.Block != core.LeavesID {
				t.Fatalf("跨区块生长登记了非树方块 %d 于 %+v", change.Block, change.Position)
			}
			seen[change.Position] = change.Block
		}
	}
	if len(seen) != len(records) {
		t.Fatalf("两个区块共登记 %d 个变更格，几何有 %d 条记录", len(seen), len(records))
	}
	for _, record := range records {
		target := saplingGrowthTarget(sapling, record)
		if seen[target] != record.Block {
			t.Fatalf("变更格 %+v 登记为 %d，想要几何记录 %d", target, seen[target], record.Block)
		}
	}
}

// TestSaplingGrowthReadsGeometryOnlyOnRollHit 锁定读取预算的诚实性：判定命中
// 时多出的读取恰好是树形几何的逐格校验（不超过记录数），判定未命中时一个也不
// 读——树形几何只在被考察格是树苗且判定命中时才被求值。
func TestSaplingGrowthReadsGeometryOnlyOnRollHit(t *testing.T) {
	sapling := core.BlockPos{X: 8, Y: 1, Z: 8}
	records := saplingGrowthGeometry(t, saplingGrowthSeed, sapling)

	readsFor := func(t *testing.T, hit bool) int {
		t.Helper()
		state, dimension := wildPlantFixture(map[core.BlockPos]core.BlockID{sapling: core.SaplingID})
		chunk, ready := dimension.ReadyChunk(core.ChunkPos{})
		if !ready {
			t.Fatal("生长夹具区块未就绪")
		}
		tick := saplingGrowthTick(t, saplingGrowthSeed, saplingGrowthKey, saplingGrowthSamples, sapling, hit)
		state.SetEnvironmentTick(tick, saplingGrowthSeed, saplingGrowthConfig())
		before := state.CropBlockReads()
		state.advanceCropCell(
			dimension, core.Overworld, chunk, sapling, tick, state.NewMutation(),
		)
		return state.CropBlockReads() - before
	}

	missReads := readsFor(t, false)
	if missReads != 2 {
		t.Fatalf("判定未命中时读取=%d，想要 2（格自身与支撑）", missReads)
	}
	hitReads := readsFor(t, true)
	if hitReads <= 2 {
		t.Fatalf("判定命中时读取=%d，没有把树形几何的逐格校验计入预算", hitReads)
	}
	if hitReads > 2+len(records) {
		t.Fatalf("判定命中时读取=%d，超过 2+几何记录数 %d", hitReads, len(records))
	}
}

// TestSaplingGrowthReadBudgetBoundedByExaminedCells 覆盖 farming delta 的
// Scenario「树苗数量增加不放大单 tick 读取总量」：两个 active Ready 区段数相同
// 的世界（一个没有树苗、一个每个被考察格都是树苗）单 tick 读取都不得超过
// 「被考察格数 × 128」，且树苗世界的读数确实包含了几何读取。
func TestSaplingGrowthReadBudgetBoundedByExaminedCells(t *testing.T) {
	barren, _ := wildPlantFixture(nil)
	plantedBlocks := map[core.BlockPos]core.BlockID{}
	for x := range int32(core.SectionSize) {
		for z := range int32(core.SectionSize) {
			plantedBlocks[core.BlockPos{X: x, Y: 1, Z: z}] = core.SaplingID
		}
	}
	planted, _ := wildPlantFixture(plantedBlocks)

	tick := saplingGrowthTick(
		t, saplingGrowthSeed, saplingGrowthKey, saplingGrowthSamples,
		core.BlockPos{X: 1, Y: 1, Z: 1}, true,
	)
	stepSaplingGrowth(t, barren, saplingGrowthSeed, tick, saplingGrowthConfig())
	stepSaplingGrowth(t, planted, saplingGrowthSeed, tick, saplingGrowthConfig())

	barrenExamined, barrenReads := barren.CropStats()
	plantedExamined, plantedReads := planted.CropStats()
	if barrenExamined == 0 {
		t.Fatal("空世界一格都没考察，读取上界断言恒真")
	}
	if barrenExamined != plantedExamined {
		t.Fatalf("两个世界的考察量不同：%d 与 %d", barrenExamined, plantedExamined)
	}
	if barrenReads > barrenExamined*128 {
		t.Fatalf("无树苗世界读取=%d，超过考察格数 %d × 128", barrenReads, barrenExamined)
	}
	if plantedReads > plantedExamined*128 {
		t.Fatalf("满树苗世界读取=%d，超过考察格数 %d × 128", plantedReads, plantedExamined)
	}
	if plantedReads <= barrenReads {
		t.Fatalf("满树苗世界读取=%d 不大于无树苗世界 %d，树形几何读取没有计入预算",
			plantedReads, barrenReads)
	}
}

// TestSaplingGrowthRollIsIndependentAndDeterministic 钉住生长判定的三条性质：
// 可复现、命中率约 1/8、salt 与既有各条判定流两两不同（否则「同一格既通过生长
// 判定又通过产量/退化判定」会成为系统性的同源偏差）。
func TestSaplingGrowthRollIsIndependentAndDeterministic(t *testing.T) {
	position := core.BlockPos{X: 8, Y: 1, Z: 8}
	const seed = int64(0x5a911eaf)
	first := sampler.SaplingGrowthRoll(seed, 12345, core.Overworld, position)
	if first != sampler.SaplingGrowthRoll(seed, 12345, core.Overworld, position) {
		t.Fatal("同输入两次的生长判定不同，判定不是纯函数")
	}
	for _, salt := range []uint64{
		updates.CropGrowthRollSalt, updates.CropYieldRollSalt, updates.CropYieldPotatoSalt,
		updates.CropYieldCarrotSalt, updates.PoisonPotatoSalt, updates.FarmlandRevertRollSalt,
	} {
		if updates.SaplingGrowthRollSalt == salt {
			t.Fatalf("生长判定 salt 与既有 salt %#x 相同", salt)
		}
	}

	hits, total := 0, 0
	for tick := range uint64(8192) {
		for x := int32(0); x < 4; x++ {
			if sampler.SaplingGrowthRoll(seed, tick, core.Overworld, core.BlockPos{X: x, Y: 1, Z: 8}) {
				hits++
			}
			total++
		}
	}
	if total != 8192*4 {
		t.Fatalf("样本数=%d，想要 %d", total, 8192*4)
	}
	// 理论命中率 1/8；区间取 1/16..1/4，足以否掉恒真、恒假与 1/2 等错误实现。
	if hits < total/16 || hits > total/4 {
		t.Fatalf("1/8 判定命中 %d/%d，比例异常", hits, total)
	}
}
