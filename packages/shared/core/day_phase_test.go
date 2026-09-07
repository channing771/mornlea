package core

import (
	"math"
	"testing"
)

// day_phase_test.go：`DisplayDayPhase` 是显示相位的全仓唯一计算入口（sim 判夜与
// 客户端呈现都应经它取相位），本文件锁定其「先对 `worldTime` 做 `%24000`、再与
// `offset` 相加取模」的语义与回绕边界。函数随本行自带交付（夜行者行未合并时
// 先行缺位），rebase 合并时与夜行者行的同一函数去重，只保留一份。

// TestDisplayDayPhaseMatchesPureModuloSemantics 用独立算式对拍纯语义：offset 为 0
// 时相位必须退化为 `worldTime % 24000`；非零 offset 时等于「先取模再相加取模」。
func TestDisplayDayPhaseMatchesPureModuloSemantics(t *testing.T) {
	worldTimes := []uint64{0, 1, 12999, 13000, 23000, 23001, 23999, 24000, 24001, 1 << 40}
	offsets := []uint16{0, 1, 12000, 23999}
	for _, worldTime := range worldTimes {
		for _, offset := range offsets {
			want := uint16((worldTime%24000 + uint64(offset)) % 24000)
			if got := DisplayDayPhase(worldTime, offset); got != want {
				t.Fatalf("DisplayDayPhase(%d, %d) = %d，想要 %d", worldTime, offset, got, want)
			}
		}
	}
}

// TestDisplayDayPhaseMaxWorldTime 锁定 `worldTime` 取 uint64 最大值时不溢出、
// 不回绕成错值：期望值由同一纯算式独立给出。
func TestDisplayDayPhaseMaxWorldTime(t *testing.T) {
	const maxWorldTime = math.MaxUint64
	want := uint16((maxWorldTime%24000 + 0) % 24000)
	if got := DisplayDayPhase(maxWorldTime, 0); got != want {
		t.Fatalf("DisplayDayPhase(MaxUint64, 0) = %d，想要 %d", got, want)
	}
	want = uint16((maxWorldTime%24000 + 23999) % 24000)
	if got := DisplayDayPhase(maxWorldTime, 23999); got != want {
		t.Fatalf("DisplayDayPhase(MaxUint64, 23999) = %d，想要 %d", got, want)
	}
}

// TestDisplayDayPhaseWrapsAtCycleStart 锁定回绕边界：相位 23999 加 1 个 offset
// 必须回到 0（周期起点 = 白昼），这是跳夜「显示相位到 0」语义的算术基础。
func TestDisplayDayPhaseWrapsAtCycleStart(t *testing.T) {
	if got := DisplayDayPhase(23999, 1); got != 0 {
		t.Fatalf("DisplayDayPhase(23999, 1) = %d，想要 0", got)
	}
	// offset 恰好把相位推过周期起点：落在 0 之后剩余的部分必须保留。
	if got := DisplayDayPhase(23999, 2); got != 1 {
		t.Fatalf("DisplayDayPhase(23999, 2) = %d，想要 1", got)
	}
	// offset 上界 23999：任意相位加上它都不应溢出或错回绕。
	if got := DisplayDayPhase(0, 23999); got != 23999 {
		t.Fatalf("DisplayDayPhase(0, 23999) = %d，想要 23999", got)
	}
}

// TestIsDisplayNightPhaseBounds 锁定夜间的统一窗口定义 13000..23000（含两端）：
// 与夜行者行的生成窗口是同一份夜间定义，边界值两侧必须严格区分。
func TestIsDisplayNightPhaseBounds(t *testing.T) {
	for _, phase := range []uint16{12999, 0, 12000, 23001, 23999} {
		if IsDisplayNightPhase(phase) {
			t.Fatalf("相位 %d 不应判为夜间", phase)
		}
	}
	for _, phase := range []uint16{13000, 13001, 18000, 22999, 23000} {
		if !IsDisplayNightPhase(phase) {
			t.Fatalf("相位 %d 应判为夜间", phase)
		}
	}
}

// ---- 季节 warp 层（DayFractionAt / DayArcTicks / EffectiveDayPhase / ----
// ---- EffectiveMorningOffset），数值锚点以 change design 的数值总表为准。 ----

// TestDayFractionAtAnchors 锚定昼弧比例曲线：分点 0.5、夏至 0.65、冬至 0.35，
// yearPhase 整周期回绕后回到分点值。
func TestDayFractionAtAnchors(t *testing.T) {
	cases := []struct {
		yearPhase float64
		want      float64
	}{
		{0, 0.5},
		{0.25, 0.65},
		{0.5, 0.5},
		{0.75, 0.35},
		{1, 0.5},
	}
	for _, tc := range cases {
		if got := DayFractionAt(tc.yearPhase); math.Abs(got-tc.want) > 1e-9 {
			t.Fatalf("DayFractionAt(%v) = %v，想要 %v", tc.yearPhase, got, tc.want)
		}
	}
}

// TestDayArcTicksAnchorsAndEvenness 锚定昼弧 tick：冬至 8400、夏至 15600、
// 分点 12000；全年逐 tick 扫描结果恒为偶数且落在 [8400,15600]。
func TestDayArcTicksAnchorsAndEvenness(t *testing.T) {
	anchors := map[float64]uint16{
		0:    12000,
		0.25: 15600,
		0.5:  12000,
		0.75: 8400,
	}
	for yearPhase, want := range anchors {
		if got := DayArcTicks(yearPhase); got != want {
			t.Fatalf("DayArcTicks(%v) = %d，想要 %d", yearPhase, got, want)
		}
	}
	const yearSamples = 288000
	for k := 0; k <= yearSamples; k++ {
		arc := DayArcTicks(float64(k) / yearSamples)
		if arc%2 == 1 {
			t.Fatalf("DayArcTicks(%v) = %d 为奇数", float64(k)/yearSamples, arc)
		}
		if arc < 8400 || arc > 15600 {
			t.Fatalf("DayArcTicks(%v) = %d 越出 [8400,15600]", float64(k)/yearSamples, arc)
		}
	}
}

// TestEffectiveDayPhaseEquinoxIdentity 分点（dayArc=12000）warp 严格恒等：
// 对同一批 worldTime×offset 组合（含 uint64 最大值）逐值等于未 warp 的
// `DisplayDayPhase`——既有全部行为与视觉基线在分点逐字节不变。
func TestEffectiveDayPhaseEquinoxIdentity(t *testing.T) {
	worldTimes := []uint64{0, 1, 12999, 13000, 23000, 23001, 23999, 24000, 24001, 1 << 40, math.MaxUint64}
	offsets := []uint16{0, 1, 12000, 23999}
	for _, worldTime := range worldTimes {
		for _, offset := range offsets {
			want := DisplayDayPhase(worldTime, offset)
			if got := EffectiveDayPhase(worldTime, offset, 12000); got != want {
				t.Fatalf("EffectiveDayPhase(%d, %d, 12000) = %d，想要 %d（分点恒等）", worldTime, offset, got, want)
			}
		}
	}
}

// TestEffectiveDayPhaseSummerAnchors 夏至昼弧 15600：白昼支路 15600→12000 压
// 缩、黑夜支路 8400→12000 拉伸，关键点值手算钉死（p 由 worldTime=p、offset=0
// 构造）。
func TestEffectiveDayPhaseSummerAnchors(t *testing.T) {
	const dayArc = uint16(15600)
	cases := []struct {
		p    uint16
		want uint16
	}{
		{0, 0},       // 白昼始
		{7800, 6000}, // 7800·12000/15600 = 6000：白昼中点（正午）仍是 6000
		{15599, 11999},
		{15600, 12000}, // 黑夜始
		{19800, 18000}, // 12000 + 4200·12000/8400 = 18000：黑夜中点（午夜）仍是 18000
		{23999, 23998}, // 黑夜末：10/7 拉伸下 23999 不可命中
	}
	for _, tc := range cases {
		if got := EffectiveDayPhase(uint64(tc.p), 0, dayArc); got != tc.want {
			t.Fatalf("EffectiveDayPhase(%d, 0, %d) = %d，想要 %d", tc.p, dayArc, got, tc.want)
		}
	}
}

// TestEffectiveDayPhaseWinterAnchors 冬至昼弧 8400：白昼支路 8400→12000 拉伸、
// 黑夜支路 15600→12000 压缩。
func TestEffectiveDayPhaseWinterAnchors(t *testing.T) {
	const dayArc = uint16(8400)
	cases := []struct {
		p    uint16
		want uint16
	}{
		{0, 0},
		{4200, 6000}, // 4200·12000/8400 = 6000：正午仍是 6000
		{8399, 11998},
		{8400, 12000},
		{12000, 14769}, // 12000 + floor(3600·12000/15600) = 14769
		{23999, 23999}, // 黑夜末恰为周期顶
	}
	for _, tc := range cases {
		if got := EffectiveDayPhase(uint64(tc.p), 0, dayArc); got != tc.want {
			t.Fatalf("EffectiveDayPhase(%d, 0, %d) = %d，想要 %d", tc.p, dayArc, got, tc.want)
		}
	}
}

// TestEffectiveDayPhaseMonotonicAcrossPhase 对覆盖值域两端的多个昼弧全周期扫
// 描：季节化相位随线性相位单调不减、起点为 0、全程不越出 23999。
func TestEffectiveDayPhaseMonotonicAcrossPhase(t *testing.T) {
	for _, dayArc := range []uint16{2, 8400, 12000, 12002, 15600, 23998, 24000} {
		if got := EffectiveDayPhase(0, 0, dayArc); got != 0 {
			t.Fatalf("dayArc=%d 相位 0 处得到 %d，想要 0", dayArc, got)
		}
		prev := uint16(0)
		for p := uint64(1); p < DayLengthTicks; p++ {
			e := EffectiveDayPhase(p, 0, dayArc)
			if e < prev {
				t.Fatalf("dayArc=%d 线性相位 %d 处季节化相位回退：%d -> %d", dayArc, p, prev, e)
			}
			prev = e
		}
	}
}

// TestEffectiveDayPhaseRejectsInvalidDayArc 非法昼弧（0、奇数、>24000）按契约
// 返回 0，绝不除零或越界。
func TestEffectiveDayPhaseRejectsInvalidDayArc(t *testing.T) {
	for _, dayArc := range []uint16{0, 1, 3, 12001, 23999, 24001, 65535} {
		if got := EffectiveDayPhase(12345, 6000, dayArc); got != 0 {
			t.Fatalf("非法 dayArc=%d 应返回 0，得到 %d", dayArc, got)
		}
	}
}

// TestEffectiveDayPhaseSeasonChangeContinuity 走真实派生链（YearPhaseAt →
// DayArcTicks → EffectiveDayPhase）扫一整年：昼弧随年相位缓慢漂移，相邻 tick
// 的季节化相位差不瞬跳（双向 ≤ 5），仅在线性相位回绕到周期起点处归零。
// 上界推导：支路步进 ≤ ceil(12000/8400) = 2；昼弧「取偶」使舍入值跨过奇整数
// 时单 tick 至多变 2（全年探测钉死为 2），重映射偏移 < 2·12000/8400 ≈ 2.9；
// 再加取整松弛，上界恰为 5（多年实测正向最大 = 5）。
//
// ±2 昼弧漂移的负向微抖只由昼弧 +2 产生，且只落在白昼支路后半段与黑夜支路
// 前半段（记 p 为线性相位、A 为昼弧、q=p−A、N=24000−A：白昼支路
// Δ=12000(A−2p)/(A(A+2)) 仅 p>A/2 为负，黑夜支路 Δ=12000(2q−N)/(N(N−2)) 仅
// q<N/2 为负，两处 |Δ| < 12000/8400 ≈ 1.43，取整对齐后均实测至多 −2；昼弧
// −2 只产生正向步进）。这是「取偶」的固有代价，属连续性内的抖动而非瞬跳。
func TestEffectiveDayPhaseSeasonChangeContinuity(t *testing.T) {
	const offset = uint16(7700)
	const seasonOffset = uint64(99)
	const start = uint64(1_000_000)
	arcChanges := 0
	prevArc := DayArcTicks(YearPhaseAt(start, seasonOffset))
	prevE := EffectiveDayPhase(start, offset, prevArc)
	for tick := start + 1; tick <= start+YearTicks+DayLengthTicks; tick++ {
		arc := DayArcTicks(YearPhaseAt(tick, seasonOffset))
		if arc != prevArc {
			arcChanges++
		}
		e := EffectiveDayPhase(tick, offset, arc)
		if DisplayDayPhase(tick, offset) == 0 {
			// 线性相位回绕到周期起点：季节化相位从白昼支路起点重新出发。
			if e != 0 {
				t.Fatalf("tick=%d 线性相位回绕处季节化相位 = %d，想要 0", tick, e)
			}
		} else {
			delta := int(e) - int(prevE)
			if delta < 0 {
				delta = -delta
			}
			if delta > 5 {
				t.Fatalf("tick=%d（dayArc %d→%d）季节化相位跳变 %d -> %d，差值超过 5", tick, prevArc, arc, prevE, e)
			}
		}
		prevArc = arc
		prevE = e
	}
	if arcChanges == 0 {
		t.Fatalf("全年扫描未观察到昼弧变化，测试未覆盖漂移路径")
	}
}

// TestEffectiveMorningOffsetRoundTrip 反解闭环：可命中的 morningPhase 精确回
// 到自身；不可命中（warp 非满射）的落到下一个可命中相位（至多 +1；周期顶端
// 的不可命中值回落到前一个）。可达性只由公开的 `EffectiveDayPhase` 全域扫描
// 独立判定，不与实现共享算式。
func TestEffectiveMorningOffsetRoundTrip(t *testing.T) {
	for _, dayArc := range []uint16{8400, 8402, 12000, 12002, 15600} {
		reachable := make([]bool, DayLengthTicks)
		for p := uint64(0); p < DayLengthTicks; p++ {
			reachable[EffectiveDayPhase(p, 0, dayArc)] = true
		}
		for morningPhase := uint16(0); morningPhase < DayLengthTicks; morningPhase++ {
			ok := reachable[morningPhase]
			// 不可命中时的期望：下一个可命中相位；若顶端不可命中（昼弧>12000 的
			// 黑夜支路顶部），则回落到前一个可命中相位。
			wantNext := morningPhase + 1
			if !ok && morningPhase == DayLengthTicks-1 {
				wantNext = morningPhase - 1
			}
			for _, worldTime := range []uint64{0, 1, 23999, 123456789, math.MaxUint64} {
				offset := EffectiveMorningOffset(worldTime, dayArc, morningPhase)
				e := EffectiveDayPhase(worldTime, offset, dayArc)
				if ok && e != morningPhase {
					t.Fatalf("dayArc=%d morningPhase=%d worldTime=%d 可命中但反解得 %d", dayArc, morningPhase, worldTime, e)
				}
				if !ok && e != wantNext {
					t.Fatalf("dayArc=%d morningPhase=%d worldTime=%d 不可命中，期望落在 %d，得到 %d", dayArc, morningPhase, worldTime, wantNext, e)
				}
			}
		}
	}
}

// TestEffectiveMorningOffsetHandAnchors 手算钉死两个反解样例：M=0 回到线性相
// 位 0；M=6000 在夏至昼弧下回到线性相位 7800（白昼中点仍是 6000）。
func TestEffectiveMorningOffsetHandAnchors(t *testing.T) {
	if got := EffectiveMorningOffset(777, 15600, 0); got != 23223 {
		t.Fatalf("EffectiveMorningOffset(777, 15600, 0) = %d，想要 23223", got)
	}
	if e := EffectiveDayPhase(777, 23223, 15600); e != 0 {
		t.Fatalf("闭环校验失败：e = %d，想要 0", e)
	}
	if got := EffectiveMorningOffset(0, 15600, 6000); got != 7800 {
		t.Fatalf("EffectiveMorningOffset(0, 15600, 6000) = %d，想要 7800", got)
	}
	if e := EffectiveDayPhase(0, 7800, 15600); e != 6000 {
		t.Fatalf("闭环校验失败：e = %d，想要 6000", e)
	}
}

// TestEffectiveMorningOffsetRejectsInvalidInput 非法昼弧与越界 morningPhase
// 按契约返回 0。
func TestEffectiveMorningOffsetRejectsInvalidInput(t *testing.T) {
	for _, dayArc := range []uint16{0, 1, 3, 12001, 24001, 65535} {
		if got := EffectiveMorningOffset(12345, dayArc, 6000); got != 0 {
			t.Fatalf("非法 dayArc=%d 应返回 0，得到 %d", dayArc, got)
		}
	}
	for _, morningPhase := range []uint16{24000, 32768, 65535} {
		if got := EffectiveMorningOffset(12345, 15600, morningPhase); got != 0 {
			t.Fatalf("越界 morningPhase=%d 应返回 0，得到 %d", morningPhase, got)
		}
	}
}
