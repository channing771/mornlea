package runtime

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// —— Scenario：生长推进完全确定且成本与作物数量无关 ——

// cropFieldWater 是作物田夹具里那一格水源的位置：它在田正中，因此距离 4 以内的
// 耕地会变湿、更远的会变干，一次夹具同时覆盖两个方向的转换。
var cropFieldWater = core.BlockPos{X: 8, Y: 1, Z: 8}

// plantCropField 在整个区块的 y=1 铺满持久化干/湿状态正确的耕地、y=2 铺满
// 阶段 0 的小麦，并在正中放一格水源，共 255 株作物。
func plantCropField(engine *Engine) {
	for x := range int32(core.SectionSize) {
		for z := range int32(core.SectionSize) {
			ground := core.BlockPos{X: x, Y: 1, Z: z}
			if ground == cropFieldWater {
				engine.SetBlockForTest(ground, core.WaterSourceID)
				continue
			}
			farmland := core.FarmlandDryID
			if x >= cropFieldWater.X-farmlandWetRadius && x <= cropFieldWater.X+farmlandWetRadius &&
				z >= cropFieldWater.Z-farmlandWetRadius && z <= cropFieldWater.Z+farmlandWetRadius {
				farmland = core.FarmlandWetID
			}
			engine.SetBlockForTest(ground, farmland)
			engine.SetBlockForTest(core.BlockPos{X: x, Y: 2, Z: z}, core.WheatStage0ID)
		}
	}
}

// TestCropGrowthReplaysIdentically 覆盖 Scenario「相同输入重放结果一致」。
//
// 比的是整块区块的 Hash 而不是某一格：逐格一致这条契约在任何一格上都成立，
// 用 Hash 一次覆盖 24 × 4096 格。同时断言「跑完之后的 Hash 与跑之前不同」——
// 否则两个什么都没发生的世界也会一致，断言恒真。
func TestCropGrowthReplaysIdentically(t *testing.T) {
	const replayTicks = 200
	key := core.ChunkKey{Dimension: core.Overworld}
	run := func() (before, after [32]byte) {
		engine, _ := readyCropWorld(t)
		plantCropField(engine)
		before, _, ok := engine.ChunkHash(key)
		if !ok {
			t.Fatalf("区块 %+v 未就绪", key)
		}
		for range replayTicks {
			engine.Step()
		}
		after, _, ok = engine.ChunkHash(key)
		if !ok {
			t.Fatalf("区块 %+v 未就绪", key)
		}
		return before, after
	}
	firstBefore, firstAfter := run()
	secondBefore, secondAfter := run()
	if firstBefore != secondBefore {
		t.Fatalf("两次的初始世界就不同，夹具本身不确定")
	}
	if firstAfter == firstBefore {
		t.Fatalf("%d 个 tick 里世界一动没动，重放一致的断言恒真", replayTicks)
	}
	if firstAfter != secondAfter {
		t.Fatalf("重放 %d 个 tick 后区块 Hash 不同：%x 与 %x",
			replayTicks, firstAfter, secondAfter)
	}
}

// TestCropTickCostIsIndependentOfCropCount 覆盖 Scenario
// 「作物数量增加不改变单 tick 考察量」。
//
// 两个世界的区段数完全相同，只差作物数：一个 0 株、一个 255 株。夹具必须这样
// 取——两个世界作物数相同的话，考察量当然相等，断言恒真。
func TestCropTickCostIsIndependentOfCropCount(t *testing.T) {
	barren, _ := readyCropWorld(t)
	planted, _ := readyCropWorld(t)
	plantCropField(planted)

	barren.Step()
	planted.Step()

	barrenExamined, barrenReads := barren.realm.CropStats()
	plantedExamined, plantedReads := planted.realm.CropStats()
	if barrenExamined == 0 {
		t.Fatal("空世界一格都没考察，两边相等的断言恒真")
	}
	if barrenExamined != plantedExamined {
		t.Fatalf("考察量随作物数量变化：0 株世界 %d 格，255 株世界 %d 格",
			barrenExamined, plantedExamined)
	}
	for name, stats := range map[string][2]int{
		"空世界": {barrenExamined, barrenReads}, "种植世界": {plantedExamined, plantedReads},
	} {
		if stats[1] > 2*stats[0] {
			t.Fatalf("%s 作物读取=%d，超过 2×%d", name, stats[1], stats[0])
		}
	}
	// 考察量必须正好是「已就绪区块数 × 区段数 × 每区段抽样数」。这条把
	// 「相等」升级成「等于一个与作物无关的解析式」，堵住"两边都退化成 0"
	// 以外的其他共同漂移。
	want := core.SectionsPerChunk * 64
	if barrenExamined != want {
		t.Fatalf("单 tick 考察量 %d，想要 %d（1 个已就绪区块 × %d 区段 × 64 抽样）",
			barrenExamined, want, core.SectionsPerChunk)
	}
}

// TestCropAllFarmlandReadsEachSampleOnce 锁定非作物样本的读取上界：干耕地退化
// 复用同一抽样本轮，需额外读取正上方是否为 `core.AirID`，因此每样本至多两次读取。
func TestCropAllFarmlandReadsEachSampleOnce(t *testing.T) {
	engine, _ := readyCropWorld(t)
	for y := int32(core.MinY); y < int32(core.MaxY); y++ {
		for x := range int32(core.SectionSize) {
			for z := range int32(core.SectionSize) {
				engine.SetBlockForTest(core.BlockPos{X: x, Y: y, Z: z}, core.FarmlandDryID)
			}
		}
	}

	active := engine.activeInterestKeys()
	engine.realm.AdvanceCrops(active, engine.realm.NewMutation())

	examined, reads := engine.realm.CropStats()
	if examined == 0 {
		t.Fatal("全耕地世界一格都没考察，读取等式无法证明成本")
	}
	if reads != 2*examined {
		t.Fatalf("全耕地阶段读取=%d，想要每个样本两次（含上方空气判定）、共 %d",
			reads, 2*examined)
	}
}
