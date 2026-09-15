package realm

// grass_spread_test.go：随机 tick 的草蔓延。覆盖 spec「表面泥土经随机 tick
// 蔓延为草」的五组 Scenario：邻草蔓延并登记进当 tick 批次、无草邻/被实体方块
// 覆盖不蔓延、唯一草邻在未就绪区块不蔓延且不同步加载、双引擎重放逐位一致、
// 读取预算有界。
//
// 用例一律驱动生产入口 `AdvanceCrops`（读取预算的每格钉子例外：它直接调用
// 分发器 `advanceCropCell` 以隔离单格的读取增量），抽样与骰子都在真实路径上。
// 测试只负责把夹具与 tick 选到「该观察的那一步」——`grassSpreadHittingTicks`
// 用包内同一条抽样与判定函数找出「抽样命中且骰子等于期望」的 tick，因此不复制
// 任何生产逻辑。

import (
	"slices"
	"testing"

	"github.com/channing771/mornlea/packages/server/updates"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// grassSpreadSeed 是蔓延用例的世界种子：固定值让命中 tick 能在用例内现算。
const grassSpreadSeed = int64(0x6ea11c0ffee)

// grassSpreadSamples 是蔓延用例的每区段抽样数：取 64（tunables 上限）让单格
// 每 tick 被抽中的概率足够高，用例在几十个 tick 内就能观察到蔓延。
const grassSpreadSamples = 64

// grassSpreadConfig 返回蔓延用例的环境参数快照：零值气候（晴、春分黎明）下
// 积雪兜底不写任何方块，夹具里唯一的随机 tick 写入者就是草蔓延。
func grassSpreadConfig() EnvironmentConfig {
	return EnvironmentConfig{
		RandomTicksPerSection: grassSpreadSamples,
		DropPickupDelayTicks:  10,
	}
}

// grassSpreadKey 是蔓延用例唯一的活动区块键。
var grassSpreadKey = core.ChunkKey{Dimension: core.Overworld}

// stepGrassSpread 以给定 tick 驱动一次随机 tick 阶段并提交事务，返回本次提交
// 的区块变更批次；未产生任何写入时批次为空。
func stepGrassSpread(t *testing.T, state *State, tick uint64) []ChunkChangeBatch {
	t.Helper()
	state.SetEnvironmentTick(tick, grassSpreadSeed, grassSpreadConfig())
	mutation := state.NewMutation()
	state.AdvanceCrops([]core.ChunkKey{grassSpreadKey}, mutation)
	return mutation.Commit()
}

// grassSpreadHittingTicks 预解前 count 个「随机 tick 抽样命中 position 且
// `sampler.GrassSpreadRoll` 等于 hit」的 tick。与 `AdvanceCrops` 用同一条
// `sampler.SampleCells` 纯函数（同种子、同区块键、同区段、同样本数），推进这些
// tick 必然命中夹具格——「不发生」类用例因此能证明格子被看过、只是被规则拒绝。
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

// —— Scenario：邻接草的表面泥土最终成草 ——

// TestGrassSpreadConvertsSurfaceDirtNextToGrass 覆盖正面路径：抽样与骰子同时
// 命中的邻草表面泥土被写成草方块，变更登记进当 tick 的区块变更批次（流式同步
// 的载体）、受影响区块 revision 恰好推进一次，草源格保持草。
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

// TestGrassSpreadUnderSnowCover 钉住「上方为季节雪覆盖仍蔓延」的设计裁决：雪层
// 方块 ID 以 `snow_cover.go` 现行为准——`advanceSnowCover` 只在白名单地表上方写
// `core.SnowLayer1BlockID..SnowLayer4BlockID`（四档雪层，见 `core.IsSnowLayer`），
// 雪落在地表之上、视作非实体遮蔽；蔓延写的是地表格自身，上方雪层原样保留。
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

// —— Scenario：无草邻或被实体方块覆盖不蔓延 ——

// TestGrassSpreadSkipsWithoutGrassNeighborOrUnderSolidCover 覆盖两条拒绝路径：
// 四个水平邻格均为泥土（无草源）、上方被实体方块（石头）覆盖。两种夹具都推进
// 「抽样命中且骰子命中」的 tick——被看过且只被规则拒绝，泥土与遮挡物保持原样、
// 零写入。
func TestGrassSpreadSkipsWithoutGrassNeighborOrUnderSolidCover(t *testing.T) {
	isolated := core.BlockPos{X: 4, Y: 1, Z: 4}
	noNeighbor := map[core.BlockPos]core.BlockID{isolated: core.DirtID}
	// 四邻全泥土：与「四邻全空气」同样无草，用泥土排除「邻居是空气才不查」的
	// 退化解。
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

// —— Scenario：邻接 chunk 未就绪不蔓延且不同步加载 ——

// TestGrassSpreadCrossChunkUnreadyNeighborAborts 覆盖跨区块就绪闸门：泥土贴着
// 区块东界、唯一的草源位于邻区块 (1,0) 的边沿对面。邻区块未生成（Absent）时，
// 骰子命中的 tick 也不蔓延——邻格经 `dimension.BlockAt` 读作未就绪并按「无草邻」
// 处理，且不得触发任何同步加载（邻区块保持未就绪）；把邻区块生成好、放入草源
// 后，同一 tick 重试即蔓延，写入只落在泥土所在区块。
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

// —— Scenario：重放一致 ——

// grassSpreadChange 是重放比对的最小单元：位置加写入后的方块。
type grassSpreadChange struct {
	position core.BlockPos
	block    core.BlockID
}

// TestGrassSpreadReplaysIdentically 覆盖「相同世界种子与初始世界、两个独立引擎
// 推进相同 tick 数」：逐 tick 的变更序列与最终区块 Hash 逐位一致，且世界确实
// 发生了蔓延（否则两个什么都没发生的世界也会一致，断言恒真）。
func TestGrassSpreadReplaysIdentically(t *testing.T) {
	const ticks = 256
	blocks := func() map[core.BlockPos]core.BlockID {
		// y=1 整层泥土，只留对角两粒草种：大片泥土在 256 tick 内逐步被两处
		// 草源吃掉，逐 tick 序列里才有实质内容可比。
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

// —— Scenario：读预算有界 ——

// TestGrassSpreadReadsNeighborsOnlyOnRollHit 锁定读取预算的诚实性：骰子未命中
// 时一个邻居也不读（自读之外只剩积雪兜底的上方 1 次，共 2 次）；骰子命中且蔓延
// 完成时恰好读上方 1 加水平 4（草源放在扫描序最后一位，逼出全部 4 次邻居读取）。
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

// TestGrassSpreadReadBudgetBoundedByExaminedCells 覆盖「读次数不超过被检查格数
// 的固定小常数倍」：两个同构世界（一个纯草皮表面、一个 y=1 整层泥土加一格草
// 源）推进同一 tick，读取都不得超过考察格数的 7 倍——上界组成是「格自身 1 +
// 蔓延判定至多 5（上方 1 加水平 4）+ 积雪兜底的上方 1」，且泥土世界的读数必须
// 严格大于草皮世界，证明邻居读取确实计入了预算。
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

	// 选一个「泥土格被抽样且骰子命中」的 tick：两个世界推进同一 tick，泥土
	// 世界的邻居读取必然被计入。
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

// —— 判定流的确定性与独立性 ——

// TestGrassSpreadRollIsIndependentAndDeterministic 钉住蔓延骰子的三条性质：
// 可复现、命中率约 1/4、盐值与既有各条判定流两两不同（否则「同一格既通过蔓延
// 判定又通过生长/退化判定」会成为系统性的同源偏差）。
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
	// 理论命中率 1/4；区间取 1/8..1/2，足以否掉恒真、恒假与 1/8 等错误掩码。
	if hits < total/8 || hits > total/2 {
		t.Fatalf("1/4 判定命中 %d/%d，比例异常", hits, total)
	}
}
