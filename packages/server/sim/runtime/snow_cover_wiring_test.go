package runtime

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// —— 积雪气候束接线的端到端夹具 ——
//
// 接线契约：`StepWithTunables` 必须把当 tick 的权威天气与季节温度快照
// （yearPhase/effPhase）随 `realm.EnvironmentConfig` 传入 realm 环境推进。本文件
// 只验证接线——机制（升档/消融/回差/白名单/露天判定/预算）的定点覆盖在
// `packages/server/sim/realm/snow_cover_test.go`；气候束是原子三字段，冬季正例
// （雪落在草皮上）证明天气与季节快照都被真实消费，夏季反例证明温度确实随
// yearPhase 分叉，两例合起来排除「零值气候碰巧积雪」的假绿。
//
// 冬至世界时间锚：seed=0 的季节偏移是 158435（core golden 锚，见 season_test.go
// 的 `testSeasonOffset`），年内序号 (t+158435)%288000 = 216000 ⇔ yearPhase 恰为
// 0.75（冬至）。夏至锚直接复用同文件的 `testSolsticeWorldTime`（yearPhase 0.25）。
const snowWinterWorldTime = uint64(
	(int64(216000) - int64(testSeasonOffset) + core.YearTicks) % core.YearTicks,
)

// snowLayersInChunk 统计已就绪区块里的雪层，并校验每层都铺在 y=1 草皮（或被
// 牛吃成的泥土——静态夹具里不会发生，防御性放宽到白名单）正上方的 y=2。
func snowLayersInChunk(t *testing.T, engine *Engine) int {
	t.Helper()
	chunk, ready := engine.dimension(core.Overworld).ReadyChunk(core.ChunkPos{})
	if !ready {
		t.Fatal("原点区块未就绪")
	}
	layers := 0
	for localX := range core.SectionSize {
		for localZ := range core.SectionSize {
			for y := int32(-1); y < 16; y++ {
				tier, isLayer := core.SnowLayerTier(chunk.BlockAt(localX, y, localZ))
				if !isLayer {
					continue
				}
				layers++
				below := chunk.BlockAt(localX, y-1, localZ)
				if y != 2 || (below != core.GrassID && below != core.DirtID) {
					t.Fatalf("雪层出现在 (%d,%d,%d)（%d 档，下方 %d），只应铺在草皮正上方 y=2",
						localX, y, localZ, tier, below)
				}
			}
		}
	}
	return layers
}

// TestStepWiresWinterRainIntoSnowAccumulation 覆盖端到端正例：冬至雨天推进权威
// tick，露天草皮上必须出现雪层。抽样率 64（`readyCropWorld` 夹具值）下 40 个
// tick 对 256 列草皮的期望命中约 160 次，全空的概率约为 e^−160——夹具失绿只可能
// 是接线断了，不会是运气。
func TestStepWiresWinterRainIntoSnowAccumulation(t *testing.T) {
	engine, _ := readyCropWorld(t)
	engine.SetWorldTimeForTest(snowWinterWorldTime)
	engine.SetWeatherForTest(core.WeatherRain, 1<<28)
	for range 40 {
		engine.Step()
	}
	if layers := snowLayersInChunk(t, engine); layers == 0 {
		t.Fatal("40 个冬至雨天 tick 后草皮上没有任何雪层，气候束没有接入环境推进")
	}
}

// TestStepSummerRainAccumulatesNothing 覆盖端到端反例：夏至雨天海平面温度 26℃
// 高于雪点，推进任意多 tick 都不得出现雪层（结构不可能，非概率断言）。
func TestStepSummerRainAccumulatesNothing(t *testing.T) {
	engine, _ := readyCropWorld(t)
	engine.SetWorldTimeForTest(testSolsticeWorldTime)
	engine.SetWeatherForTest(core.WeatherRain, 1<<28)
	for range 40 {
		engine.Step()
	}
	if layers := snowLayersInChunk(t, engine); layers != 0 {
		t.Fatalf("夏至雨天出现 %d 格雪层，积雪没有按局部温度拒绝", layers)
	}
}
