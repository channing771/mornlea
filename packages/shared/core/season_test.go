package core

import (
	"math"
	"testing"
)

// season_test.go：锁定季节推进的确定性算式——每季 72000 权威 tick、整年
// 288000 tick 回绕、季节起点偏移由世界 seed 确定性派生（同 seed 必然稳定、
// 不落任何持久化字段）、季内进度 0..255 量化在季首/季末/换季三处的边界。

// TestSeasonProgressionSequence 锚定推进序列：偏移 0 时 worldTime=0 恰为春季
// 起点，之后春→夏→秋→冬→春，288000 tick 恰整年回绕。
func TestSeasonProgressionSequence(t *testing.T) {
	cases := []struct {
		worldTime uint64
		want      Season
	}{
		{0, SeasonSpring},
		{1, SeasonSpring},
		{71999, SeasonSpring},
		{72000, SeasonSummer},
		{143999, SeasonSummer},
		{144000, SeasonAutumn},
		{215999, SeasonAutumn},
		{216000, SeasonWinter},
		{287999, SeasonWinter},
		{288000, SeasonSpring},
		{360000, SeasonSummer},
		{576000, SeasonSpring},
	}
	for _, tc := range cases {
		if got := SeasonAt(tc.worldTime, 0); got != tc.want {
			t.Fatalf("SeasonAt(%d, 0) = %s，想要 %s", tc.worldTime, got, tc.want)
		}
	}
}

// TestSeasonAtHonorsOffset 锚定偏移的平移语义：偏移 1000 把所有季节边界整体
// 前移 1000 tick，并在年内取模链上正确回绕。
func TestSeasonAtHonorsOffset(t *testing.T) {
	const offset = uint64(1000)
	cases := []struct {
		worldTime uint64
		want      Season
	}{
		{0, SeasonSpring},     // 年内序号 1000
		{70999, SeasonSpring}, // 年内序号 71999，春季末 tick
		{71000, SeasonSummer}, // 年内序号 72000，夏季首 tick
		{286999, SeasonWinter},
		{287000, SeasonSpring}, // 年内序号 (287000+1000)%288000 = 0，整年回绕
	}
	for _, tc := range cases {
		if got := SeasonAt(tc.worldTime, offset); got != tc.want {
			t.Fatalf("SeasonAt(%d, %d) = %s，想要 %s", tc.worldTime, offset, got, tc.want)
		}
	}
}

// TestSeasonOffsetFromSeedDeterministic 同 seed 派生稳定且落在 0..287999；
// 固定 seed 样本（含负值与 int64 两端）内存在互不相同的偏移，证明盐把季节
// 偏移流隔离成非常函数。
func TestSeasonOffsetFromSeedDeterministic(t *testing.T) {
	seeds := []int64{0, 1, 2, 3, 42, -1, math.MinInt64, math.MaxInt64}
	seen := make(map[uint64]struct{}, len(seeds))
	for _, seed := range seeds {
		first := SeasonOffsetFromSeed(seed)
		second := SeasonOffsetFromSeed(seed)
		if first != second {
			t.Fatalf("seed %d 两次派生不一致：%d vs %d", seed, first, second)
		}
		if first >= YearTicks {
			t.Fatalf("seed %d 派生偏移 %d 越界（≥ %d）", seed, first, YearTicks)
		}
		seen[first] = struct{}{}
	}
	if len(seen) < 2 {
		t.Fatalf("固定样本内所有 seed 派生出同一偏移，盐未生效：%v", seen)
	}
}

// TestSeasonProgressQuantization 锚定季内进度的三处边界：季首 0、季末最后一
// tick 255、换季瞬间回绕 0。
func TestSeasonProgressQuantization(t *testing.T) {
	cases := []struct {
		worldTime uint64
		want      uint8
	}{
		{0, 0},
		{1, 0},
		{71999, 255},
		{72000, 0},
		{143999, 255},
		{144000, 0},
		{287999, 255},
		{288000, 0},
	}
	for _, tc := range cases {
		if got := SeasonProgressAt(tc.worldTime, 0); got != tc.want {
			t.Fatalf("SeasonProgressAt(%d, 0) = %d，想要 %d", tc.worldTime, got, tc.want)
		}
	}
}

// TestSeasonProgressMonotonicWithinSeason 全年扫描（含非零偏移）：季内进度随
// 时间单调不减，只在换季边界（年内序号每满 72000）回绕到 0。
func TestSeasonProgressMonotonicWithinSeason(t *testing.T) {
	for _, offset := range []uint64{0, 1, 123456, 287999} {
		prev := SeasonProgressAt(0, offset)
		for tick := uint64(1); tick <= YearTicks+SeasonLengthTicks; tick++ {
			progress := SeasonProgressAt(tick, offset)
			if (tick+offset)%SeasonLengthTicks == 0 {
				if progress != 0 {
					t.Fatalf("offset=%d tick=%d 是换季边界，进度应为 0，得到 %d", offset, tick, progress)
				}
			} else if progress < prev {
				t.Fatalf("offset=%d tick=%d 季内进度回退：%d -> %d", offset, tick, prev, progress)
			}
			prev = progress
		}
	}
}

// TestYearPhaseAtAnchors 锚定年相位：起点 0、半年 0.5、整年回绕 0；偏移先进
// 取模链再除，uint64 最大值不溢出。
func TestYearPhaseAtAnchors(t *testing.T) {
	near := func(got, want float64) bool {
		return math.Abs(got-want) < 1e-9
	}
	if got := YearPhaseAt(0, 0); !near(got, 0) {
		t.Fatalf("YearPhaseAt(0, 0) = %f，想要 0", got)
	}
	if got := YearPhaseAt(144000, 0); !near(got, 0.5) {
		t.Fatalf("YearPhaseAt(144000, 0) = %f，想要 0.5", got)
	}
	if got := YearPhaseAt(288000, 0); !near(got, 0) {
		t.Fatalf("YearPhaseAt(288000, 0) = %f，想要 0（整年回绕）", got)
	}
	if got := YearPhaseAt(0, 1000); !near(got, 1000.0/288000) {
		t.Fatalf("YearPhaseAt(0, 1000) = %f，想要 %f", got, 1000.0/288000)
	}
	if got := YearPhaseAt(287000, 1000); !near(got, 0) {
		t.Fatalf("YearPhaseAt(287000, 1000) = %f，想要 0（带偏移整年回绕）", got)
	}
	got := YearPhaseAt(math.MaxUint64, 287999)
	if got < 0 || got >= 1 {
		t.Fatalf("YearPhaseAt(MaxUint64, 287999) = %f，越出 [0,1)", got)
	}
}

// TestSeasonString 锁定季节英文名与越界兜底格式。
func TestSeasonString(t *testing.T) {
	cases := map[Season]string{
		SeasonSpring: "Spring",
		SeasonSummer: "Summer",
		SeasonAutumn: "Autumn",
		SeasonWinter: "Winter",
	}
	for season, want := range cases {
		if got := season.String(); got != want {
			t.Fatalf("Season(%d).String() = %q，想要 %q", uint8(season), got, want)
		}
	}
	if got := Season(4).String(); got != "Season(4)" {
		t.Fatalf("越界季节 String() = %q，想要 %q", got, "Season(4)")
	}
}
