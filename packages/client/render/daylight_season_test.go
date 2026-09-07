package render

import (
	"math"
	"testing"
)

// TestDayNightAtExplicitEquinoxMatchesUnwarpedCall 显式分点 yearPhase 与省略
// 参数的调用逐值恒等：昼弧 12000 的 warp 严格恒等且冷色权重为 0，既有两参
// 调用（等价于春始分点）与显式春始/秋始的昼夜状态必须完全一致。
func TestDayNightAtExplicitEquinoxMatchesUnwarpedCall(t *testing.T) {
	for _, offset := range []uint16{0, 1, 6000, 13000, 23999} {
		for worldTime := uint64(0); worldTime < 3*DayLengthTicks; worldTime += 499 {
			base := DayNightAt(worldTime, offset)
			for _, yearPhase := range []float64{0, 0.5} {
				if got := DayNightAt(worldTime, offset, yearPhase); got != base {
					t.Fatalf("分点 yearPhase=%v 在 (%d,%d) 的昼夜状态 = %+v，想要与未 warp 调用一致",
						yearPhase, worldTime, offset, base)
				}
			}
		}
	}
}

// TestDayNightAtWinterSolsticeCompressesDaylightArc 冬至昼弧压缩：昼弧 8400
// 下线性相位 8400 处太阳已落（季节化相位恰 12000），同一相位在分点下仍是
// 白昼；黄昏提前（线性 10000 已入夜）、正午提前（线性 4200 季节化相位恰
// 6000，太阳天顶），天体方向全部跟随季节化相位。
func TestDayNightAtWinterSolsticeCompressesDaylightArc(t *testing.T) {
	// 落日边界：线性 8400 的季节化相位为 8400·12000/8400 = 12000，太阳恰在地平线。
	sunset := DayNightAt(8400, 0, 0.75)
	if !closeEnough(sunset.Sun, 0) || !closeEnough(sunset.Daylight, 0.12) {
		t.Fatalf("冬至线性 8400 = sun %v daylight %v，想要落日（0/0.12）", sunset.Sun, sunset.Daylight)
	}
	if equinox := DayNightAt(8400, 0, 0); equinox.Sun <= 0 {
		t.Fatalf("夹具无效：分点线性 8400 太阳 = %v，想要仍为白昼", equinox.Sun)
	}
	// 黄昏提前：分点下线性 10000 仍是白昼，冬至已入夜。
	if got := DayNightAt(10000, 0, 0.75); !closeEnough(got.Sun, 0) || !closeEnough(got.Daylight, 0.12) {
		t.Fatalf("冬至线性 10000 = sun %v daylight %v，想要已入夜（0/0.12）", got.Sun, got.Daylight)
	}
	// 正午提前：线性 4200 的季节化相位恰 6000，太阳天顶、方向与分点正午一致。
	winterNoon := DayNightAt(4200, 0, 0.75)
	if winterNoon.Sun != 1 || winterNoon.Daylight != 1 {
		t.Fatalf("冬至线性 4200 = sun %v daylight %v，想要天顶正午（1/1）", winterNoon.Sun, winterNoon.Daylight)
	}
	noon := DayNightAt(6000, 0)
	if !closeDirection(winterNoon.SunDirection, noon.SunDirection) {
		t.Fatalf("冬至提前正午的太阳方向 = %v，想要与分点正午一致 %v",
			winterNoon.SunDirection, noon.SunDirection)
	}
}

// TestDayNightAtSummerSolsticeStretchesDaylightArc 夏至昼弧拉长：昼弧 15600
// 下线性 13000（分点已入夜）仍是白昼，线性 15600 才落日——与冬至同相位方向
// 相反，昼长跨度比约 15600:8400。
func TestDayNightAtSummerSolsticeStretchesDaylightArc(t *testing.T) {
	if equinox := DayNightAt(13000, 0, 0); !closeEnough(equinox.Sun, 0) {
		t.Fatalf("夹具无效：分点线性 13000 太阳 = %v，想要已入夜", equinox.Sun)
	}
	if got := DayNightAt(13000, 0, 0.25); got.Sun <= 0 {
		t.Fatalf("夏至线性 13000 太阳 = %v，想要仍为白昼（季节化相位 10000）", got.Sun)
	}
	sunset := DayNightAt(15600, 0, 0.25)
	if !closeEnough(sunset.Sun, 0) {
		t.Fatalf("夏至线性 15600 太阳 = %v，想要恰落日（季节化相位 12000）", sunset.Sun)
	}
	if morning := DayNightAt(10000, 0, 0.75); !closeEnough(morning.Sun, 0) {
		t.Fatalf("夹具无效：冬至线性 10000 太阳 = %v，想要已入夜", morning.Sun)
	}
}

// TestDayNightAtWinterColdTintBoundedAndZeroAtAnchors 冬季冷色 tint：锚点权重
// 夏至/分点恰为零（基线逐字节不变）、冬至为一；正午与午夜两端同向偏冷
// （红/绿压低、蓝抬升）、alpha 不变；任意年相位下每通道偏离无季节基线不超
// 过上界 0.03，结果恒在 [0,1]。
func TestDayNightAtWinterColdTintBoundedAndZeroAtAnchors(t *testing.T) {
	for _, yearPhase := range []float64{0, 0.25, 0.5} {
		if got := winterColdWeight(yearPhase); got != 0 {
			t.Fatalf("yearPhase=%v 的冷色权重 = %v，想要 0（夏至/分点为零）", yearPhase, got)
		}
	}
	if got := winterColdWeight(0.75); got != 1 {
		t.Fatalf("冬至冷色权重 = %v，想要 1", got)
	}

	// 正午（冬至正午在冬至昼弧下位于线性 4200）：基线即日间 clear 色，冬至
	// 全幅冷偏移（红 −0.03、绿 −0.015、蓝 +0.015）。
	baseNoon := DayNightAt(6000, 0).ClearColor
	if baseNoon != [4]float32{0.42, 0.68, 0.92, 1} {
		t.Fatalf("夹具无效：正午基线 = %v，想要日间 clear 色", baseNoon)
	}
	wantNoon := [4]float32{0.42 - 0.03, 0.68 - 0.015, 0.92 + 0.015, 1}
	gotNoon := DayNightAt(4200, 0, 0.75).ClearColor
	for channel := range gotNoon {
		if !closeEnough(gotNoon[channel], wantNoon[channel]) {
			t.Fatalf("冬至正午 clear 通道 %d = %v，想要 %v", channel, gotNoon[channel], wantNoon[channel])
		}
	}

	// 午夜（冬至午夜位于线性 16200）：夜间基线向冷蓝同向偏移；红端 0.02−0.03
	// 落到钳制下界 0。
	baseNight := DayNightAt(18000, 0).ClearColor
	wantNight := [4]float32{0, 0.03 - 0.015, 0.08 + 0.015, 1}
	gotNight := DayNightAt(16200, 0, 0.75).ClearColor
	for channel := range gotNight {
		if !closeEnough(gotNight[channel], wantNight[channel]) {
			t.Fatalf("冬至午夜 clear 通道 %d = %v，想要 %v", channel, gotNight[channel], wantNight[channel])
		}
	}
	if gotNight[0] >= baseNight[0] || gotNight[1] >= baseNight[1] || gotNight[2] <= baseNight[2] {
		t.Fatalf("午夜冷偏移方向错误：%v → %v，想要红/绿降、蓝升", baseNight, gotNight)
	}

	// 有界与值域：对夜昼两端基色做全年相位扫描，每通道偏离 ≤ 0.03、落在
	// [0,1]、alpha 恒定；权重为零的锚点严格恒等。
	for _, base := range [][4]float32{baseNoon, baseNight} {
		for step := 0; step <= 2880; step++ {
			yearPhase := float64(step) / 2880
			tinted := applyWinterColdTint(base, yearPhase)
			for channel := range 3 {
				delta := math.Abs(float64(tinted[channel] - base[channel]))
				if delta > 0.03+1e-6 {
					t.Fatalf("yearPhase=%v 通道 %d 偏离 %v 超过上界 0.03", yearPhase, channel, delta)
				}
				if tinted[channel] < 0 || tinted[channel] > 1 {
					t.Fatalf("yearPhase=%v 通道 %d = %v 超出 [0,1]", yearPhase, channel, tinted[channel])
				}
			}
			if tinted[3] != base[3] {
				t.Fatalf("yearPhase=%v 的 alpha 被冷色 tint 改动", yearPhase)
			}
			if winterColdWeight(yearPhase) == 0 && tinted != base {
				t.Fatalf("yearPhase=%v 权重为零却改变了基色", yearPhase)
			}
		}
	}
}
