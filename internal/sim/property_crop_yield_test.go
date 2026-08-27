package sim

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件锁定成熟小麦收获产量的三条性质（change crop-random-drop-count）：同一
// 输入重放逐件相同（确定性契约）、两类数量落在 [1,3] 且取遍整个区间（防退化）、
// yield 哈希流与生长判定哈希流不同源（防结构性相关）。精确到某格某 tick 的具体
// 数值不在此锁定——那是哈希常量的偶然值，锁它会挡住任何合法的 salt 更换。
//
// 全部断言只用固定输入清单，不用 math/rand、不用 map 遍历顺序，因此每条结论
// 都是跨平台跨运行逐位稳定的。

// TestCropYieldReplayIsDeterministic 覆盖 Scenario「同一输入重放得到相同数量」：
// 同一 `(seed, tick, dimension, position)` 调用 `cropYieldRolls` 两次，小麦与
// 种子两类数量必须逐件相同。输入清单覆盖正负种子、tick 0 与高位 tick、两个维度
// 与负坐标——确定性必须在输入域的角落里都成立，而不只在中枢的"漂亮"输入上。
func TestCropYieldReplayIsDeterministic(t *testing.T) {
	inputs := []struct {
		name      string
		seed      int64
		tick      uint64
		dimension core.DimensionID
		position  core.BlockPos
	}{
		{"零种子零tick", 0, 0, core.Overworld, core.BlockPos{X: 0, Y: 1, Z: 5}},
		{"负种子负坐标", -42, 987654321, core.Overworld, core.BlockPos{X: -7, Y: -59, Z: 13}},
		{"高位tick", 12345, 1 << 40, core.DimensionID(7), core.BlockPos{X: 1023, Y: 319, Z: -1024}},
		{"同坐标另一维度", 0, 0, core.DimensionID(7), core.BlockPos{X: 0, Y: 1, Z: 5}},
	}
	for _, input := range inputs {
		t.Run(input.name, func(t *testing.T) {
			wheatA, seedsA := cropYieldRolls(input.seed, input.tick, input.dimension, input.position)
			wheatB, seedsB := cropYieldRolls(input.seed, input.tick, input.dimension, input.position)
			if wheatA != wheatB || seedsA != seedsB {
				t.Fatalf("同输入两次调用结果不同：(小麦 %d, 种子 %d) vs (小麦 %d, 种子 %d)",
					wheatA, seedsA, wheatB, seedsB)
			}
		})
	}
}

// TestCropYieldStaysInRangeAndCoversValues 覆盖 Scenario「收获成熟作物数量有界」
// 的量化面：7 个种子 × 64 个 tick × 12 组维度/坐标共 5376 次抽样（远超千次量级）
// 内，小麦与种子两类数量都必须落在闭区间 [1,3]，且**各自取遍** 1、2、3——只断言
// 上界的话，「恒为 1」的退化为实现照样全绿，而那正是本 change 要消灭的固定掉落。
// 维度/坐标清单刻意混入负值与两个不同维度，防止任何「按坐标低位截断」之类的
// 实现只在整齐输入上碰巧合规。
func TestCropYieldStaysInRangeAndCoversValues(t *testing.T) {
	positions := []struct {
		dimension core.DimensionID
		position  core.BlockPos
	}{
		{core.Overworld, core.BlockPos{X: 0, Y: 1, Z: 5}},
		{core.Overworld, core.BlockPos{X: 0, Y: 0, Z: 0}},
		{core.Overworld, core.BlockPos{X: -7, Y: -59, Z: 13}},
		{core.Overworld, core.BlockPos{X: 1023, Y: 319, Z: -1024}},
		{core.Overworld, core.BlockPos{X: -1, Y: 320, Z: 1}},
		{core.Overworld, core.BlockPos{X: 513, Y: -60, Z: -511}},
		{core.DimensionID(7), core.BlockPos{X: 0, Y: 1, Z: 5}},
		{core.DimensionID(7), core.BlockPos{X: 0, Y: 0, Z: 0}},
		{core.DimensionID(7), core.BlockPos{X: -7, Y: -59, Z: 13}},
		{core.DimensionID(7), core.BlockPos{X: 1023, Y: 319, Z: -1024}},
		{core.DimensionID(7), core.BlockPos{X: -33, Y: 17, Z: 29}},
		{core.DimensionID(7), core.BlockPos{X: 511, Y: -1, Z: 509}},
	}
	var wheatSeen [4]bool // 下标即数量本身，0 恒为 false
	var seedsSeen [4]bool
	calls := 0
	for seed := int64(-3); seed <= 3; seed++ {
		for tick := uint64(0); tick < 64; tick++ {
			for _, sampled := range positions {
				wheat, seeds := cropYieldRolls(seed, tick, sampled.dimension, sampled.position)
				if wheat < 1 || wheat > 3 {
					t.Fatalf("seed=%d tick=%d dim=%d pos=%v 的小麦数量 %d 越界 [1,3]",
						seed, tick, sampled.dimension, sampled.position, wheat)
				}
				if seeds < 1 || seeds > 3 {
					t.Fatalf("seed=%d tick=%d dim=%d pos=%v 的种子数量 %d 越界 [1,3]",
						seed, tick, sampled.dimension, sampled.position, seeds)
				}
				wheatSeen[wheat] = true
				seedsSeen[seeds] = true
				calls++
			}
		}
	}
	// 样本量守卫：位置清单被误删空时这里先红，别让「零次抽样全过」变成假绿。
	if calls < 4000 {
		t.Fatalf("抽样次数 %d 少于预期的数千次量级，区间穷举失去意义", calls)
	}
	for value := uint8(1); value <= 3; value++ {
		if !wheatSeen[value] {
			t.Fatalf("%d 次抽样中小麦数量 %d 从未出现，分布退化", calls, value)
		}
		if !seedsSeen[value] {
			t.Fatalf("%d 次抽样中种子数量 %d 从未出现，分布退化", calls, value)
		}
	}
}

// TestCropYieldStreamIndependentOfGrowthStream 锁定双流独立性（design.md D1）：
// yield 流必须由独立 salt 驱动，不得与 `cropGrowthRoll` 的生长判定流在相同
// `(seed, tick)` 前缀上同源——否则「这株长成了」与「这株掉多少」会结构性相关，
// 例如某些格子永远长熟又永远高产。两层断言：
//
//  1. salt 常量不等——结构前提，同值则两条流从第一折起逐位相同；
//  2. 固定样本集上两函数的输出存在分歧——把两条流的输出投影成布尔
//     （生长判定取 50% 通过与否；产量取小麦 ≥ 2 与否），投影本身不承载玩法
//     语义，锁的只是「同一批输入上两条流给出可区分的输出序列」这一事实。
//
// 全部输入写死在清单里，哈希又是纯函数，所以分歧数是逐位确定的常量：本测试
// 今天绿，重跑一万次也绿，不存在抽样抖动。
func TestCropYieldStreamIndependentOfGrowthStream(t *testing.T) {
	if cropYieldRollSalt == cropGrowthRollSalt {
		t.Fatal("cropYieldRollSalt 与 cropGrowthRollSalt 相等，yield 流与生长流同源")
	}
	samples := []struct {
		seed      int64
		tick      uint64
		dimension core.DimensionID
		position  core.BlockPos
	}{
		{0, 0, core.Overworld, core.BlockPos{X: 0, Y: 1, Z: 5}},
		{0, 1, core.Overworld, core.BlockPos{X: 0, Y: 1, Z: 5}},
		{0, 2, core.Overworld, core.BlockPos{X: -7, Y: -59, Z: 13}},
		{0, 3, core.Overworld, core.BlockPos{X: 1023, Y: 319, Z: -1024}},
		{1, 0, core.Overworld, core.BlockPos{X: -33, Y: 17, Z: 29}},
		{1, 100, core.Overworld, core.BlockPos{X: 511, Y: -1, Z: 509}},
		{-1, 7, core.Overworld, core.BlockPos{X: 4, Y: -20, Z: -4}},
		{-42, 987654321, core.Overworld, core.BlockPos{X: -7, Y: -59, Z: 13}},
		{0, 0, core.DimensionID(7), core.BlockPos{X: 0, Y: 1, Z: 5}},
		{0, 1, core.DimensionID(7), core.BlockPos{X: 0, Y: 1, Z: 5}},
		{0, 2, core.DimensionID(7), core.BlockPos{X: -7, Y: -59, Z: 13}},
		{0, 3, core.DimensionID(7), core.BlockPos{X: 1023, Y: 319, Z: -1024}},
		{1, 0, core.DimensionID(7), core.BlockPos{X: -33, Y: 17, Z: 29}},
		{1, 100, core.DimensionID(7), core.BlockPos{X: 511, Y: -1, Z: 509}},
		{12345, 1 << 40, core.DimensionID(7), core.BlockPos{X: 8, Y: 2, Z: 8}},
		{-3, 63, core.DimensionID(7), core.BlockPos{X: -512, Y: 0, Z: 512}},
	}
	divergences := 0
	for _, sampled := range samples {
		growthPass := cropGrowthRoll(sampled.seed, sampled.tick, sampled.dimension, sampled.position, 50)
		wheat, _ := cropYieldRolls(sampled.seed, sampled.tick, sampled.dimension, sampled.position)
		if growthPass != (wheat >= 2) {
			divergences++
		}
	}
	if divergences == 0 {
		t.Fatal("固定样本集上生长判定与产量投影完全同步，yield 流疑似与生长流同源")
	}
}
