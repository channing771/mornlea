package runtime

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/tuning"
	"github.com/channing771/mornlea/internal/world"
)

// —— 生长与干湿的端到端夹具 ——
//
// 三条设计约束，每条都直接对应一类假绿：
//
//  1. **概率必须置满**。CropGrowthChancePercent 不设 100 时，「作物没长」的断言
//     在「本来就没通过概率判定」的情况下也会绿，用例静默失去意义。
//  2. **抽样必须能打到夹具格**。RandomTicksPerSection 置到上限 64，单格每 tick
//     被抽中的概率约 1/64，cropFixtureTicks 个 tick 内期望被抽中约 9 次。抽样是
//     纯哈希、seed 与 tick 序列都固定，因此这不是"大概率"而是确定的事实。
//  3. **每条「不发生」都要有只改一个条件的对照**。对照会发生变化，就证明夹具格
//     确实被抽中过——否则「没长」既可能是规则拒绝，也可能是根本没看过这一格。
const cropFixtureTicks = 600

// 夹具坐标。取区块中央而不是原点：9×9 的湿润窗口必须整个落在唯一一个已就绪
// 区块内，否则「相邻区块未加载按无水」会混进判定；同时也避开出生点所在的列。
var (
	cropFixtureFarmland = core.BlockPos{X: 8, Y: 1, Z: 8}
	cropFixtureCrop     = core.BlockPos{X: 8, Y: 2, Z: 8}
	cropFixtureCover    = core.BlockPos{X: 8, Y: 3, Z: 8}
)

// cropFlatChunk 生成作物测试用的平坦区块：y=-1 与 y=0 两层石头地基、y=1 草皮，
// 其余空气。
//
// 地基是必需的：夹具里的水源可能放在耕地的下一层（y=0），下方必须实心，否则
// 水会一路流到世界底部，「范围内有水」这个前置在第一个 tick 就自己消失。两层
// 而不是一层，正是为了让 y=0 这一层也能放水。列顶仍是 y=1，露天判定不受影响。
func cropFlatChunk(pos core.ChunkPos) *world.Chunk {
	chunk := world.NewChunk(pos)
	for x := range core.SectionSize {
		for z := range core.SectionSize {
			chunk.SetBlock(x, -1, z, core.StoneID)
			chunk.SetBlock(x, 0, z, core.StoneID)
			chunk.SetBlock(x, 1, z, core.GrassID)
		}
	}
	chunk.Compact()
	return chunk
}

// placeContainedWater 放一格水源，并把它周围仍是空气的六个面邻格填成石头。
//
// 这样这一格水在流体规则下是不动点：源永不自然消失，六个邻格都不可被等级 1
// 替换，evalCell 对它产出空写入集合。夹具里的水**必须原地不动**——它一旦流走，
// 「范围内有水」这个前置就在第一个 tick 自己消失，用例会因为前置蒸发而绿。
// 复用 internal/sim/fluid.go 的 fluidNeighbors，不另写一份邻格表。
func placeContainedWater(t *testing.T, engine *Engine, position core.BlockPos) {
	t.Helper()
	for _, neighbor := range fluidNeighbors(position) {
		if cropBlockAt(t, engine, neighbor) == core.AirID {
			engine.SetBlockForTest(neighbor, core.StoneID)
		}
	}
	engine.SetBlockForTest(position, core.WaterSourceID)
}

// cropFixture 描述一个作物夹具的四个可独立开关的条件。对照用例只改其中一个。
type cropFixture struct {
	// farmland 是 cropFixtureFarmland 处写入的耕地编号。
	farmland core.BlockID
	// crop 是 cropFixtureCrop 处写入的作物编号；AirID 表示不放作物。
	crop core.BlockID
	// waterDistance > 0 时在耕地的 waterDY 层、该水平距离处放一个水源；
	// 0 表示不放水。
	waterDistance int32
	// waterDY 是水源相对耕地的层偏移：0 同层、+1 上一层、-1 下一层。规格写的
	// 是「同层或上一层」，三个取值恰好覆盖窗口的内侧、上边界与下边界外侧。
	waterDY int32
	// cover 是作物正上方写入的方块编号；AirID 表示不遮挡。
	//
	// 类型是方块编号而不是 bool：规格说的是「之上不存在**任何非空气**方块」，
	// 而实现读的是 world.Chunk.HighestOpaque——那个名字里的 Opaque 名不副实
	// （它返回的是最高**非空气**方块）。只用石头遮挡的话，「不透明才算遮挡」
	// 这种实现照样全绿，名字与规格之间的缝没人守。
	cover core.BlockID
}

// readyCropWorld 构造一名 active 玩家与一个已 Ready 的平坦区块，并把
// RandomTicksPerSection 与 CropGrowthChancePercent 置到端到端测试的设置。
//
// viewRadius 取 0，因此只有区块 (0,0) 会就绪：活动兴趣范围里的其余 24 个 key
// 一直是 Absent，被 advanceCrops 跳过。这让单 tick 的考察量固定为
// 24 个区段 × 64 条抽样，测试跑得起 600 个 tick。
func readyCropWorld(t *testing.T) (*Engine, SessionID) {
	t.Helper()
	engine, sessions := readyCropWorldAt(t, core.ChunkPos{})
	return engine, sessions[0]
}

// readyCropWorldAt 是 readyCropWorld 的多锚点版本：每个 anchor 注册一名玩家，
// 因此**恰好**这些区块会就绪。跨区块用例靠它同时让两个区块进入 Ready，再靠
// UnregisterSession 让其中一个离开。
func readyCropWorldAt(t *testing.T, anchors ...core.ChunkPos) (*Engine, []SessionID) {
	t.Helper()
	t.Cleanup(func() { tuning.SetTunables(tuning.DefaultTunables()) })
	tunables := tuning.DefaultTunables()
	tunables.RandomTicksPerSection = 64
	tunables.CropGrowthChancePercent = 100
	tuning.SetTunables(tunables)

	engine := NewEngine(0, 0, 0)
	sessions := make([]SessionID, 0, len(anchors))
	for index, anchor := range anchors {
		session := SessionID(index + 1)
		sessions = append(sessions, session)
		engine.RegisterSession(session, core.Overworld, anchor)
	}
	for range 8 {
		result := engine.Step()
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
	}
	for _, session := range sessions {
		if player, ok := engine.Player(session); !ok || !player.Ready {
			t.Fatalf("会话 %d 的玩家未 Ready: %+v", session, player)
		}
	}
	return engine, sessions
}

// applyCropFixture 把夹具写进已就绪的世界。
func applyCropFixture(t *testing.T, engine *Engine, fixture cropFixture) {
	t.Helper()
	engine.SetBlockForTest(cropFixtureFarmland, fixture.farmland)
	if fixture.crop != core.AirID {
		engine.SetBlockForTest(cropFixtureCrop, fixture.crop)
	}
	if fixture.waterDistance > 0 {
		water := cropFixtureFarmland
		water.X += fixture.waterDistance
		water.Y += fixture.waterDY
		placeContainedWater(t, engine, water)
	}
	if fixture.cover != core.AirID {
		if core.IsFluid(fixture.cover) {
			// 流体遮挡物同样要封住，否则它会流走，「被遮挡」这个前置在第一个
			// tick 就消失。placeContainedWater 会先把六个空气邻格填成石头，
			// 其中「下方」那一格是作物本身（非空气、非流体，不可替换），因此
			// 不会被填掉，也不会被水冲走。
			placeContainedWater(t, engine, cropFixtureCover)
		} else {
			engine.SetBlockForTest(cropFixtureCover, fixture.cover)
		}
	}
}

// newCropWorld 一步构造世界并写入夹具。
func newCropWorld(t *testing.T, fixture cropFixture) *Engine {
	t.Helper()
	engine, _ := readyCropWorld(t)
	applyCropFixture(t, engine, fixture)
	return engine
}

// cropBlockAt 读取主世界某格的权威方块，区块未就绪时直接失败。
func cropBlockAt(t *testing.T, engine *Engine, position core.BlockPos) core.BlockID {
	t.Helper()
	block, ready := engine.dimension(core.Overworld).BlockAt(position)
	if !ready {
		t.Fatalf("方块 %+v 所在区块未就绪", position)
	}
	return block
}

// stepUntilBlock 推进权威 tick 直到 position 变成 want，返回花掉的 tick 数；
// 到 cropFixtureTicks 仍未变成 want 时返回 (0, false)。
func stepUntilBlock(
	engine *Engine, position core.BlockPos, want core.BlockID,
) (ticks int, ok bool) {
	for tick := 1; tick <= cropFixtureTicks; tick++ {
		engine.Step()
		block, ready := engine.dimension(core.Overworld).BlockAt(position)
		if ready && block == want {
			return tick, true
		}
	}
	return 0, false
}

// stepCropTicks 推进固定 tick 数。
func stepCropTicks(engine *Engine) {
	for range cropFixtureTicks {
		engine.Step()
	}
}

// assertCropGrowth 断言夹具格上的作物「相对起始阶段是否推进过」。
//
// 断言的是**阶段号的大小关系**而不是某个具体阶段：cropFixtureTicks 个 tick 里
// 这一格会被抽中若干次，具体停在哪一阶段取决于抽中次数，钉死具体阶段等于把
// 哈希序列的实现细节写进期望值。而「推进过 / 没推进过」正是 Scenario 要问的。
func assertCropGrowth(t *testing.T, engine *Engine, start core.BlockID, wantGrowth bool) {
	t.Helper()
	got := cropBlockAt(t, engine, cropFixtureCrop)
	if !core.IsCrop(got) {
		t.Fatalf("夹具格上是 %s，已经不是作物", blockLabel(got))
	}
	grew := core.CropStage(got) > core.CropStage(start)
	if grew != wantGrowth {
		t.Fatalf("%d 个 tick 后作物从 %s 变成 %s（推进=%v），想要推进=%v",
			cropFixtureTicks, blockLabel(start), blockLabel(got), grew, wantGrowth)
	}
}
