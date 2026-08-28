package sim

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/tuning"
)

// TestGrowCropIsExhaustivelySpecified 穷举 8 个小麦阶段 × {湿, 干} ×
// {露天, 遮蔽} 共 32 种输入，逐条写死期望值。
//
// 期望值是**手写常量**而不是「再跑一遍实现」：后者在任何实现下都成立，等于
// 没测。表里 WheatStage7ID 那两行（湿且露天）是成熟不推进这条规则的唯一守卫。
func TestGrowCropIsExhaustivelySpecified(t *testing.T) {
	stages := [8]core.BlockID{
		core.WheatStage0ID, core.WheatStage1ID, core.WheatStage2ID, core.WheatStage3ID,
		core.WheatStage4ID, core.WheatStage5ID, core.WheatStage6ID, core.WheatStage7ID,
	}
	// wantNext[stage] 是「湿且露天」时的期望结果；其余三种环境一律不变。
	wantNext := [8]core.BlockID{
		core.WheatStage1ID, core.WheatStage2ID, core.WheatStage3ID, core.WheatStage4ID,
		core.WheatStage5ID, core.WheatStage6ID, core.WheatStage7ID, core.WheatStage7ID,
	}
	wantChanged := [8]bool{true, true, true, true, true, true, true, false}
	for stage, block := range stages {
		for _, env := range []struct {
			wet, sky bool
		}{{true, true}, {true, false}, {false, true}, {false, false}} {
			next, changed := growCrop(block, env.wet, env.sky)
			expectNext, expectChanged := block, false
			if env.wet && env.sky {
				expectNext, expectChanged = wantNext[stage], wantChanged[stage]
			}
			if next != expectNext || changed != expectChanged {
				t.Errorf(
					"growCrop(阶段 %d, wet=%v, sky=%v) = (%d, %v)，想要 (%d, %v)",
					stage, env.wet, env.sky, next, changed, expectNext, expectChanged,
				)
			}
		}
	}
}

// TestGrowCropLeavesNonCropsAlone 证明非作物编号一律原样返回。耕地必须在其中
// ——它与作物编号相邻，「落在农业编号区间内就推进」这类实现只有这条会红。
func TestGrowCropLeavesNonCropsAlone(t *testing.T) {
	for _, block := range []core.BlockID{
		core.AirID, core.StoneID, core.DirtID, core.GrassID,
		core.FarmlandDryID, core.FarmlandWetID,
		core.WaterSourceID, core.WaterLevel1ID,
	} {
		next, changed := growCrop(block, true, true)
		if next != block || changed {
			t.Errorf("growCrop(%s) = (%d, %v)，非作物必须原样返回", blockLabel(block), next, changed)
		}
	}
}

// setCropGrowthChance 改写生效中的生长概率。readyCropWorldAt 已经登记了恢复
// 默认值的 Cleanup，因此这里不必再登记一次。
func setCropGrowthChance(percent uint8) {
	tunables := tuning.ActiveTunables()
	tunables.CropGrowthChancePercent = percent
	tuning.SetTunables(tunables)
}

// —— Scenario：作物按时间推进阶段，且只在露天与湿润时生长 ——

// TestExposedWetCropAdvancesStage 覆盖 Scenario「露天且湿润的作物推进阶段」。
func TestExposedWetCropAdvancesStage(t *testing.T) {
	engine := newCropWorld(t, cropFixture{
		farmland:      core.FarmlandWetID,
		crop:          core.WheatStage0ID,
		waterDistance: 4,
	})
	ticks, ok := stepUntilBlock(engine, cropFixtureCrop, core.WheatStage1ID)
	if !ok {
		t.Fatalf(
			"%d 个 tick 后作物仍是 %s，露天湿润的作物必须推进阶段",
			cropFixtureTicks, blockLabel(cropBlockAt(t, engine, cropFixtureCrop)),
		)
	}
	t.Logf("第 %d 个 tick 推进到阶段 1", ticks)
	// 耕地必须始终是湿的：范围内的水一直在，干湿双向转换不得把它误判成干。
	if got := cropBlockAt(t, engine, cropFixtureFarmland); got != core.FarmlandWetID {
		t.Fatalf("耕地变成了 %s，范围内有水时必须保持湿耕地", blockLabel(got))
	}
}

// TestCoveredCropDoesNotGrow 覆盖 Scenario「被遮挡的作物不生长」。
//
// 四条子用例共用同一个夹具构造，**只差 cover 一个字段**：对照必须长，
// 被遮挡的必须不长。没有对照的话，「不长」在「600 个 tick 里恰好没抽中这一格」
// 时同样成立，断言与规则无关。
//
// 三种遮挡物缺一不可。规格写的是「之上不存在**任何非空气**方块」，而实现读的
// 是 world.Chunk.HighestOpaque——那个名字里的 Opaque 名不副实（它返回的是最高
// 非空气方块）。只用石头的话，「只有不透明方块才算遮挡」这种实现照样全绿：
// 玻璃是非空气、非不透明的 cutout 类，水是非空气、非不透明的流体，两者正好
// 卡在名字与规格之间的那道缝上。
func TestCoveredCropDoesNotGrow(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cover      core.BlockID
		wantGrowth bool
	}{
		{"对照：无遮挡必须推进", core.AirID, true},
		{"正上方是石头时不推进", core.StoneID, false},
		{"正上方是玻璃时不推进", core.GlassID, false},
		{"正上方是水时不推进", core.WaterSourceID, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newCropWorld(t, cropFixture{
				farmland:      core.FarmlandWetID,
				crop:          core.WheatStage0ID,
				waterDistance: 4,
				cover:         tc.cover,
			})
			stepCropTicks(engine)
			assertCropGrowth(t, engine, core.WheatStage0ID, tc.wantGrowth)
		})
	}
}

// TestCropOnDryFarmlandDoesNotGrow 覆盖 Scenario「干耕地上的作物不生长」。
//
// 同样是「只改一个条件」的成对用例：对照在范围内放水（耕地保持湿），
// 主用例不放水（耕地保持干），其余完全相同。
//
// **`farmland` 与 `waterDistance` 一起描述同一个一致的湿度环境。** 作物阶段只读
// 持久化的干/湿编号，不维护湿度；正向夹具写湿耕地和范围内水，负向夹具写干耕地
// 且不放水，避免用陈旧方块状态测试生长规则。
func TestCropOnDryFarmlandDoesNotGrow(t *testing.T) {
	for _, tc := range []struct {
		name          string
		farmland      core.BlockID
		waterDistance int32
		wantGrowth    bool
	}{
		{"对照：湿耕地上必须推进", core.FarmlandWetID, 4, true},
		{"干耕地上不推进", core.FarmlandDryID, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newCropWorld(t, cropFixture{
				farmland:      tc.farmland,
				crop:          core.WheatStage0ID,
				waterDistance: tc.waterDistance,
			})
			stepCropTicks(engine)
			assertCropGrowth(t, engine, core.WheatStage0ID, tc.wantGrowth)
		})
	}
}

// TestMatureCropStaysMature 覆盖 Scenario「成熟作物不再推进」。
//
// 对照是同一夹具下的阶段 6：它必须推进到阶段 7，证明这一格在 600 个 tick 里
// 确实被抽中过；而阶段 7 必须停在阶段 7。
func TestMatureCropStaysMature(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start core.BlockID
		want  core.BlockID
	}{
		{"对照：阶段 6 推进到阶段 7", core.WheatStage6ID, core.WheatStage7ID},
		{"阶段 7 保持阶段 7", core.WheatStage7ID, core.WheatStage7ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newCropWorld(t, cropFixture{
				farmland:      core.FarmlandWetID,
				crop:          tc.start,
				waterDistance: 4,
			})
			stepCropTicks(engine)
			if got := cropBlockAt(t, engine, cropFixtureCrop); got != tc.want {
				t.Fatalf("%d 个 tick 后作物是 %s，想要 %s",
					cropFixtureTicks, blockLabel(got), blockLabel(tc.want))
			}
		})
	}
}

// TestZeroGrowthChanceNeverAdvancesCrop 覆盖「0 = 永不推进」的端到端语义。
//
// 两条用例使用相同的持久化湿耕地夹具和确定性抽样序列；概率 100 的对照必须生长，
// 证明夹具作物确实被抽中过，因此概率 0 不生长只能来自概率判定拒绝。
func TestZeroGrowthChanceNeverAdvancesCrop(t *testing.T) {
	for _, tc := range []struct {
		name       string
		percent    uint8
		wantGrowth bool
	}{
		{"对照：概率 100 必须推进", 100, true},
		{"概率 0 永不推进", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newCropWorld(t, cropFixture{
				farmland:      core.FarmlandWetID,
				crop:          core.WheatStage0ID,
				waterDistance: 4,
			})
			setCropGrowthChance(tc.percent)
			stepCropTicks(engine)
			assertCropGrowth(t, engine, core.WheatStage0ID, tc.wantGrowth)
		})
	}
}
