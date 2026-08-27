package sim

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

// TestGrowCropPotatoWetSky 覆盖马铃薯在湿润且露天下推进一阶段。
func TestGrowCropPotatoWetSky(t *testing.T) {
	next, changed := growCrop(core.PotatoStage3ID, true, true)
	if !changed || next != core.PotatoStage4ID {
		t.Fatalf("growCrop(PotatoStage3, wet+sky) = (%d, %v)，想要 (%d, true)", next, changed, core.PotatoStage4ID)
	}
}

// TestGrowCropCarrotWetSky 覆盖胡萝卜在湿润且露天下推进一阶段。
func TestGrowCropCarrotWetSky(t *testing.T) {
	next, changed := growCrop(core.CarrotStage3ID, true, true)
	if !changed || next != core.CarrotStage4ID {
		t.Fatalf("growCrop(CarrotStage3, wet+sky) = (%d, %v)，想要 (%d, true)", next, changed, core.CarrotStage4ID)
	}
}

// TestGrowCropMaturePotatoCarrotStays 覆盖马铃薯与胡萝卜成熟后不再推进。
func TestGrowCropMaturePotatoCarrotStays(t *testing.T) {
	for _, block := range []core.BlockID{core.PotatoStage7ID, core.CarrotStage7ID} {
		next, changed := growCrop(block, true, true)
		if changed || next != block {
			t.Fatalf("growCrop(%d 成熟) = (%d, %v)，想要原样 false", block, next, changed)
		}
	}
}

// TestCropYieldPotatoRange 覆盖马铃薯成熟收获 1..4 且可重放。
func TestCropYieldPotatoRange(t *testing.T) {
	positions := []struct {
		dim core.DimensionID
		pos core.BlockPos
	}{
		{core.Overworld, core.BlockPos{X: 1, Y: 2, Z: 3}},
		{core.Overworld, core.BlockPos{X: -7, Y: -59, Z: 13}},
		{core.DimensionID(7), core.BlockPos{X: 1023, Y: 319, Z: -1024}},
	}
	seen := [5]bool{}
	calls := 0
	for seed := int64(-3); seed <= 3; seed++ {
		for tick := uint64(0); tick < 64; tick++ {
			for _, p := range positions {
				// 重放一致性
				a := cropYieldRollsPotato(seed, tick, p.dim, p.pos)
				b := cropYieldRollsPotato(seed, tick, p.dim, p.pos)
				if a != b {
					t.Fatalf("potato yield 不可重放 seed=%d tick=%d dim=%d pos=%v %d vs %d", seed, tick, p.dim, p.pos, a, b)
				}
				if a < 1 || a > 4 {
					t.Fatalf("potato yield 越界 seed=%d tick=%d dim=%d pos=%v got %d", seed, tick, p.dim, p.pos, a)
				}
				seen[a] = true
				calls++
			}
		}
	}
	if calls < 1000 {
		t.Fatalf("抽样次数 %d 过少", calls)
	}
	for v := uint8(1); v <= 4; v++ {
		if !seen[v] {
			t.Fatalf("potato yield %d 从未出现，分布退化 (%d 次)", v, calls)
		}
	}
}

// TestCropYieldCarrotRange 覆盖胡萝卜成熟收获 1..4 且可重放。
func TestCropYieldCarrotRange(t *testing.T) {
	positions := []struct {
		dim core.DimensionID
		pos core.BlockPos
	}{
		{core.Overworld, core.BlockPos{X: 1, Y: 2, Z: 3}},
		{core.Overworld, core.BlockPos{X: -7, Y: -59, Z: 13}},
	}
	seen := [5]bool{}
	for seed := int64(-2); seed <= 2; seed++ {
		for tick := uint64(0); tick < 64; tick++ {
			for _, p := range positions {
				a := cropYieldRollsCarrot(seed, tick, p.dim, p.pos)
				b := cropYieldRollsCarrot(seed, tick, p.dim, p.pos)
				if a != b {
					t.Fatalf("carrot yield 不可重放 seed=%d tick=%d %d vs %d", seed, tick, a, b)
				}
				if a < 1 || a > 4 {
					t.Fatalf("carrot yield 越界 %d", a)
				}
				seen[a] = true
			}
		}
	}
	for v := uint8(1); v <= 4; v++ {
		if !seen[v] {
			t.Fatalf("carrot yield %d 从未出现", v)
		}
	}
}

// TestCropYieldPotatoCarrotIndependent 覆盖两作物产量流独立（不同 salt）。
func TestCropYieldPotatoCarrotIndependent(t *testing.T) {
	if cropYieldPotatoSalt == cropYieldCarrotSalt {
		t.Fatal("potato 与 carrot salt 相同，两流同源")
	}
	if cropYieldPotatoSalt == cropYieldRollSalt || cropYieldCarrotSalt == cropYieldRollSalt {
		t.Fatal("新作物 salt 与小麦同源")
	}
	// 至少在固定样本上两流不完全同步
	divergences := 0
	samples := []struct {
		seed int64
		tick uint64
		dim  core.DimensionID
		pos  core.BlockPos
	}{
		{0, 0, core.Overworld, core.BlockPos{X: 1, Y: 2, Z: 3}},
		{0, 1, core.Overworld, core.BlockPos{X: 1, Y: 2, Z: 3}},
		{1, 0, core.Overworld, core.BlockPos{X: -7, Y: -59, Z: 13}},
		{-42, 987654321, core.Overworld, core.BlockPos{X: -7, Y: -59, Z: 13}},
		{12345, 1 << 40, core.DimensionID(7), core.BlockPos{X: 1023, Y: 319, Z: -1024}},
	}
	for _, s := range samples {
		p := cropYieldRollsPotato(s.seed, s.tick, s.dim, s.pos)
		c := cropYieldRollsCarrot(s.seed, s.tick, s.dim, s.pos)
		if p != c {
			divergences++
		}
	}
	if divergences == 0 {
		t.Fatal("potato 与 carrot 在固定样本上完全同步，疑似同 salt")
	}
}

// TestPoisonRollRangeAndDeterminism 覆盖毒土豆 2% 判定可重放且分布可观察。
func TestPoisonRollRangeAndDeterminism(t *testing.T) {
	// 重放一致性
	pos := core.BlockPos{X: 8, Y: 2, Z: 8}
	a := poisonRoll(42, 100, core.Overworld, pos)
	b := poisonRoll(42, 100, core.Overworld, pos)
	if a != b {
		t.Fatalf("poisonRoll 不可重放 %v vs %v", a, b)
	}
	// 统计：足够多样本中应有约 2% 为真，且非退化为恒真/恒假
	trues := 0
	total := 0
	for seed := int64(-5); seed <= 5; seed++ {
		for tick := uint64(0); tick < 200; tick++ {
			for x := int32(-2); x <= 2; x++ {
				p := core.BlockPos{X: x, Y: 1, Z: 0}
				if poisonRoll(seed, tick, core.Overworld, p) {
					trues++
				}
				total++
			}
		}
	}
	if trues == 0 || trues == total {
		t.Fatalf("poisonRoll 退化 trues=%d total=%d", trues, total)
	}
	// 宽松区间：1%..4%（理论 2%），样本量足够大时该区间足以否掉 0% 或 10% 的错误实现
	ratio := float64(trues) / float64(total)
	if ratio < 0.005 || ratio > 0.05 {
		t.Fatalf("poisonRoll 比例异常 %d/%d=%.3f 想要约 0.02", trues, total, ratio)
	}
	if poisonPotatoSalt == cropYieldPotatoSalt || poisonPotatoSalt == cropYieldCarrotSalt {
		t.Fatal("poison salt 与产量 salt 相同")
	}
}
