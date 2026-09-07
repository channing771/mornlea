package core

import (
	"math"
	"testing"
)

// temperature_test.go：锁定跨端共享温度公式的钢锚点与行为边界——三钢锚点
// （夏至正午海平面 30℃、夏至正午 Y=88 恰 0℃、冬至正午海平面 −8℃）、降水固
// 定降温 −4℃、日内正弦项的极值与过零相位、海平面以下不升温、合成结果
// clamp 到 [−40, 45]，以及 `PrecipitationIsSnow` 的雪点判定。全部期望值按
// 合成公式手算：季节基线 11+19·sin(2π·yearPhase)，日内项 5·sin(2π·(effPhase
// −6000)/24000)，雨/雷暴 −4℃，海平面以上每格 −1.25℃。

// temperatureNear 报告 float32 结果与手算期望值在 1e-4 容差内相等：合成在
// float64 里做，正弦零点（如 sin(π)）带 ~1e-16 量级误差，1e-4 容差足够覆盖
// 又不会吞掉真实回归。
func temperatureNear(got, want float32) bool {
	return math.Abs(float64(got-want)) < 1e-4
}

// TestTemperatureSteelAnchors 三钢锚点：夏至正午海平面恰 30℃；同条件 Y=88
// 恰 0℃（24 格 × 1.25℃/格 恰好抵消 30℃，高山夏雪与旧雪线锚连续）；冬至正
// 午海平面 −8℃（低地冬季全境降雪）。三条都要求日内项在正午（6000）为 0。
func TestTemperatureSteelAnchors(t *testing.T) {
	if got := TemperatureAt(0.25, 6000, WeatherClear, 64); !temperatureNear(got, 30) {
		t.Fatalf("夏至正午海平面 = %f℃，想要 30℃", got)
	}
	if got := TemperatureAt(0.25, 6000, WeatherClear, 88); !temperatureNear(got, 0) {
		t.Fatalf("夏至正午 Y=88 = %f℃，想要 0℃（高山夏雪与旧雪线连续）", got)
	}
	got := TemperatureAt(0.75, 6000, WeatherClear, 64)
	if !temperatureNear(got, -8) || got >= 0 {
		t.Fatalf("冬至正午海平面 = %f℃，想要 −8℃（低于 0℃）", got)
	}
}

// TestTemperaturePrecipitationChill 降水固定降温：雨与雷暴同为 −4℃，晴天不
// 降温。以夏至正午海平面（裸值 30℃）为基准验证。
func TestTemperaturePrecipitationChill(t *testing.T) {
	clear := TemperatureAt(0.25, 6000, WeatherClear, 64)
	if !temperatureNear(clear, 30) {
		t.Fatalf("晴天下界基准 = %f℃，想要 30℃", clear)
	}
	if got := TemperatureAt(0.25, 6000, WeatherRain, 64); !temperatureNear(got, 26) {
		t.Fatalf("夏至正午雨 = %f℃，想要 30−4=26℃", got)
	}
	if got := TemperatureAt(0.25, 6000, WeatherThunder, 64); !temperatureNear(got, 26) {
		t.Fatalf("夏至正午雷暴 = %f℃，想要 30−4=26℃", got)
	}
}

// TestTemperatureDailyCycle 日内正弦项的取值相位：公式 5·sin(2π·(effPhase
// −6000)/24000) 在正午（6000）与午夜（18000）过零、黄昏（12000）+5 达日最
// 高、黎明（0）−5 达日最低。夏至日全曲线手算：25/30/35/30；冬至日谷值在黎
// 明，11−19−5=−13。
func TestTemperatureDailyCycle(t *testing.T) {
	summer := []struct {
		effPhase uint16
		want     float32
	}{
		{0, 25},
		{6000, 30},
		{12000, 35},
		{18000, 30},
	}
	for _, tc := range summer {
		if got := TemperatureAt(0.25, tc.effPhase, WeatherClear, 64); !temperatureNear(got, tc.want) {
			t.Fatalf("夏至 effPhase=%d = %f℃，想要 %f℃", tc.effPhase, got, tc.want)
		}
	}
	winter := []struct {
		effPhase uint16
		want     float32
	}{
		{0, -13},    // 黎明谷值：11−19−5
		{6000, -8},  // 正午
		{18000, -8}, // 午夜（日内项过零）
	}
	for _, tc := range winter {
		if got := TemperatureAt(0.75, tc.effPhase, WeatherClear, 64); !temperatureNear(got, tc.want) {
			t.Fatalf("冬至 effPhase=%d = %f℃，想要 %f℃", tc.effPhase, got, tc.want)
		}
	}
}

// TestTemperatureNoWarmingBelowSeaLevel 海平面以下不升温：y=0 与 y=64 同值；
// 递减只在 y>64 时生效（y=65 恰 −1.25℃），y=64 边界本身零递减。
func TestTemperatureNoWarmingBelowSeaLevel(t *testing.T) {
	atSeaFloor := TemperatureAt(0.25, 6000, WeatherClear, 0)
	atSeaLevel := TemperatureAt(0.25, 6000, WeatherClear, 64)
	if !temperatureNear(atSeaFloor, 30) || !temperatureNear(atSeaLevel, 30) {
		t.Fatalf("y=0 与 y=64 应同值 30℃，得到 %f℃ / %f℃", atSeaFloor, atSeaLevel)
	}
	if got := TemperatureAt(0.25, 6000, WeatherClear, 65); !temperatureNear(got, 28.75) {
		t.Fatalf("y=65 = %f℃，想要 30−1.25=28.75℃", got)
	}
}

// TestTemperatureClampBounds 合成结果 clamp 到 [−40, 45]：冬季极端组合（冬至
// 黎明 −13、雷暴 −4、Y=200 递减 −170，裸值 −187）钳到下界 −40；全年相位 ×
// 全日相位 × 三天气 × 含极端高度的扫描不越上下界，且同输入两次求值逐位相同
// （纯函数无隐藏状态）。上界 45 是 wire int8 容量护栏而非可达值——公式理论
// 最大值是夏至黄昏晴天海平面 35℃。
func TestTemperatureClampBounds(t *testing.T) {
	if got := TemperatureAt(0.75, 0, WeatherThunder, 200); got != TemperatureMin {
		t.Fatalf("冬至黎明雷暴 Y=200 = %f℃，想要钳到下界 %f℃", got, TemperatureMin)
	}
	hottest := TemperatureAt(0.25, 12000, WeatherClear, 64)
	if !temperatureNear(hottest, 35) || hottest > TemperatureMax {
		t.Fatalf("理论最热组合 = %f℃，想要 35℃ 且不越上界 %f℃", hottest, TemperatureMax)
	}
	heights := []float32{0, 1, 32, 63, 64, 65, 88, 100, 200, 4096}
	weathers := []WeatherKind{WeatherClear, WeatherRain, WeatherThunder}
	for i := 0; i <= 96; i++ {
		yearPhase := float64(i) / 96
		for phase := uint32(0); phase < DayLengthTicks; phase += 375 {
			for _, weather := range weathers {
				for _, y := range heights {
					got := TemperatureAt(yearPhase, uint16(phase), weather, y)
					if got < TemperatureMin || got > TemperatureMax {
						t.Fatalf("yearPhase=%f effPhase=%d weather=%d y=%f 越界：%f℃",
							yearPhase, phase, weather, y, got)
					}
					if again := TemperatureAt(yearPhase, uint16(phase), weather, y); again != got {
						t.Fatalf("同输入两次求值不一致：%f℃ vs %f℃", got, again)
					}
				}
			}
		}
	}
}

// TestPrecipitationIsSnowScenarios 降雪判定的三个场景：夏季高山（夏至正午雨
// Y=90：30−4−32.5=−6.5 ≤ 雪点）、冬季低地（冬至正午雨海平面：−8−4=−12 ≤ 雪
// 点）为雪；分点正午低地雨（11−4=7 > 雪点）仍为雨。另锚定「恰等于雪点判
// 雪」的 ≤ 语义（夏至正午晴天 Y=88 恰 0℃）。
func TestPrecipitationIsSnowScenarios(t *testing.T) {
	if got := TemperatureAt(0.25, 6000, WeatherRain, 90); !temperatureNear(got, -6.5) {
		t.Fatalf("夏至正午雨 Y=90 = %f℃，想要 −6.5℃", got)
	}
	if !PrecipitationIsSnow(0.25, 6000, WeatherRain, 90) {
		t.Fatal("夏季高山（夏至正午雨 Y=90）应判雪")
	}
	if got := TemperatureAt(0.75, 6000, WeatherRain, 64); !temperatureNear(got, -12) {
		t.Fatalf("冬至正午雨海平面 = %f℃，想要 −12℃", got)
	}
	if !PrecipitationIsSnow(0.75, 6000, WeatherRain, 64) {
		t.Fatal("冬季低地（冬至正午雨海平面）应判雪")
	}
	if got := TemperatureAt(0, 6000, WeatherRain, 64); !temperatureNear(got, 7) {
		t.Fatalf("分点正午雨海平面 = %f℃，想要 11−4=7℃", got)
	}
	if PrecipitationIsSnow(0, 6000, WeatherRain, 64) {
		t.Fatal("分点正午低地雨（7℃ > 雪点）不应判雪")
	}
	if !PrecipitationIsSnow(0.25, 6000, WeatherClear, 88) {
		t.Fatal("恰等于雪点（夏至正午晴天 Y=88 = 0℃）应判雪（≤ 语义）")
	}
}

// TestTemperatureConstantsFrozen 冻结温度常量：雪点 0℃、融点 2℃（回差防抖
// 带，供积雪能力消费）、递减率 1.25℃/格、海平面 64（与 Rust worldgen 的
// `SEA_LEVEL_Y` 双端对齐）、上下界 [−40, 45]（落在 wire int8 容量内）。下游
// 协议与客户端表现直接依赖这组取值，改动必须走显式契约升级。
func TestTemperatureConstantsFrozen(t *testing.T) {
	if TemperatureSnowPoint != 0 {
		t.Fatalf("TemperatureSnowPoint = %f，想要 0", TemperatureSnowPoint)
	}
	if TemperatureMeltPoint != 2 {
		t.Fatalf("TemperatureMeltPoint = %f，想要 2", TemperatureMeltPoint)
	}
	if TemperatureLapsePerBlock != 1.25 {
		t.Fatalf("TemperatureLapsePerBlock = %f，想要 1.25", TemperatureLapsePerBlock)
	}
	if TemperatureSeaLevelY != 64 {
		t.Fatalf("TemperatureSeaLevelY = %f，想要 64", TemperatureSeaLevelY)
	}
	if TemperatureMin != -40 || TemperatureMax != 45 {
		t.Fatalf("温度上下界 = (%f, %f)，想要 (−40, 45)", TemperatureMin, TemperatureMax)
	}
}
