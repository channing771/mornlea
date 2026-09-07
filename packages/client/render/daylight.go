package render

import (
	"math"

	"github.com/channing771/mornlea/packages/shared/core"
)

// DayLengthTicks 是一个完整昼夜的权威 tick 数。
const DayLengthTicks = 24000

const (
	cloudTicksPerBlock  = 80
	cloudBlocksPerMacro = 64
)

// CloudOffset 是供天空 shader 使用的拆分云时间偏移。
type CloudOffset struct {
	Local  float32
	MacroX uint32
}

// CloudOffsetAt 从权威世界时间计算精确的云层偏移，避免绝对时间转 float32。
// 双偏移语义不变：`Local` 与 `MacroX` 仍按既有口径拆分，细节层相对漂移由
// 天空 shader 内部派生，本函数输出口径不因此改变。
func CloudOffsetAt(worldTime uint64) CloudOffset {
	blocks := worldTime / cloudTicksPerBlock
	return CloudOffset{
		Local:  float32(blocks%cloudBlocksPerMacro) + float32(worldTime%cloudTicksPerBlock)/float32(cloudTicksPerBlock),
		MacroX: uint32(blocks / cloudBlocksPerMacro),
	}
}

// indoorBrightness 是完全没有直射天空光时的地形基础亮度。
const indoorBrightness = 0.08

var (
	// nightSkyColor 是午夜天空背景色，dayCloudColor 是既有日间 clear color。
	nightSkyColor = [4]float32{0.02, 0.03, 0.08, 1}
	daySkyColor   = [4]float32{0.42, 0.68, 0.92, 1}
)

// DayNight 是某个显示相位下与世界空间明暗有关的全部固定值。
// 它完全由绝对世界时间、显示相位偏移与年相位（季节 warp 输入）决定（偏移只
// 平移显示相位、年相位只伸缩昼弧，都不回写绝对时间），不含任何可变状态。
type DayNight struct {
	Sun            float32
	Daylight       float32
	ClearColor     [4]float32
	SunDirection   [3]float32
	MoonDirection  [3]float32
	StarVisibility float32
}

// smoothstep 是 `t*t*(3-2t)` 的手写实现（`t` 已钳制到 0..1），不引入新依赖。
func smoothstep(t float32) float32 {
	return t * t * (3 - 2*t)
}

// DayNightAt 按固定曲线计算给定绝对世界时间与显示相位偏移下的昼夜状态：
//
//	phase    = EffectiveDayPhase(worldTime, offset, DayArcTicks(yearPhase))
//	          （分点昼弧 12000 时逐值恒等于 DisplayDayPhase）
//	sun      = max(0, sin(2π·phase/24000))
//	daylight = 0.12 + 0.88*smoothstep(sun)（晨昏 shoulder：低太阳压暗更快）
//
// 季节 warp：显示相位经全仓唯一入口 `core.EffectiveDayPhase` 按当年昼弧
// （`core.DayArcTicks(yearPhase)`）重映射——昼长随年相位连续正弦伸缩，但
// 一天仍是 24000 绝对 tick，云层漂移等绝对时间呈现不受影响（见
// `CloudOffsetAt`）。可选参数 `yearPhase` 省略时为春始分点（昼弧 12000），
// warp 严格恒等、冷色权重为零：既有两参调用与视觉基线逐值不变。
//
// 冬季冷色 tint：`ClearColor` 在既有夜昼 lerp 与暖色叠加之后，按冷冬权重
// `max(0, −sin(2π·yearPhase))` 向冷蓝偏移（见 `applyWinterColdTint`），分点
// 与夏至恰为零。
//
// 低太阳色温近似（非真实黑体辐射）：暖光 (1.0,0.55,0.30) 按
// `1-warmth` 向白光过渡，`warmth = 1-smoothstep(sun/0.5)`；`ClearColor`
// 在既有夜昼 lerp 后乘该暖色，并用 `smoothstep((daylight-0.12)/0.28)`
// 门控强度——夜间 `Daylight` 为 0.12 时强度为 0，天空保持纯净夜色。
//
// 偏移是服务端随权威玩家状态下发、客户端只读的显示相位单值（跳夜交付）：
// 全仓相位算式收敛在 `core.DisplayDayPhase`/`core.EffectiveDayPhase`，客户端
// 不得自建（含季节 warp——昼弧只能由 `core.DayArcTicks` 派生传入 yearPhase，
// 不得在这里另建算式）。
func DayNightAt(worldTime uint64, dayPhaseOffset uint16, yearPhase ...float64) DayNight {
	seasonal := 0.0
	if len(yearPhase) > 0 {
		seasonal = yearPhase[0]
	}
	phase := float64(core.EffectiveDayPhase(worldTime, dayPhaseOffset, core.DayArcTicks(seasonal))) / DayLengthTicks
	theta := 2 * math.Pi * phase
	sun := math.Sin(theta)
	if sun < 0 {
		sun = 0
	}
	sunClip := float32(sun)
	daylight := 0.12 + 0.88*smoothstep(sunClip)
	warmParam := sunClip / 0.5
	if warmParam > 1 {
		warmParam = 1
	}
	sunTint := [3]float32{1, 0.55 + 0.45*smoothstep(warmParam), 0.30 + 0.70*smoothstep(warmParam)}
	tintParam := (daylight - 0.12) / 0.28
	if tintParam < 0 {
		tintParam = 0
	}
	if tintParam > 1 {
		tintParam = 1
	}
	tintStrength := smoothstep(tintParam)
	starVisibility := 1 - (sun/0.25)*(sun/0.25)*(3-2*(sun/0.25))
	if sun >= 0.25 {
		starVisibility = 0
	}
	result := DayNight{
		Sun:            sunClip,
		Daylight:       daylight,
		SunDirection:   [3]float32{float32(math.Cos(theta)), float32(math.Sin(theta)), 0},
		MoonDirection:  [3]float32{-float32(math.Cos(theta)), -float32(math.Sin(theta)), 0},
		StarVisibility: float32(starVisibility),
	}
	for index := range result.ClearColor {
		night, day := nightSkyColor[index], daySkyColor[index]
		result.ClearColor[index] = night + (day-night)*result.Sun
	}
	for index, tint := range sunTint {
		result.ClearColor[index] *= 1 - tintStrength*(1-tint)
	}
	result.ClearColor = applyWinterColdTint(result.ClearColor, seasonal)
	return result
}

// winterColdTintMaxShift 是冬季冷色 tint 的单通道幅度上界：任意年相位下
// `ClearColor` 每通道偏离无季节基线的量都不超过该值（spec 有界条款）。
const winterColdTintMaxShift = float32(0.03)

// winterColdWeight 返回冷冬权重：max(0, −sin(2π·yearPhase))。冬至
// （yearPhase=0.75）为 1、夏至与春秋分点为 0——全年只在冷季为正，随年相位
// 连续过渡，换季不存在跳变。
func winterColdWeight(yearPhase float64) float64 {
	weight := -math.Sin(2 * math.Pi * yearPhase)
	if weight < 0 {
		return 0
	}
	return weight
}

// applyWinterColdTint 把 clear 色按冷冬权重向冷蓝偏移：红端全幅压低、绿端
// 半幅压低、蓝端半幅抬升——同一组方向系数作用于夜昼 lerp 与暖色叠加之后的
// 最终混色，夜昼两端同向偏冷；alpha 不参与，结果逐通道 clamp 到 [0,1]
// （冬季夜间的红端会触到下界）。权重为零（分点/夏至）时严格恒等，返回原值，
// 基线帧逐字节不变。
func applyWinterColdTint(color [4]float32, yearPhase float64) [4]float32 {
	weight := winterColdWeight(yearPhase)
	if weight == 0 {
		return color
	}
	shift := winterColdTintMaxShift * float32(weight)
	out := color
	out[0] -= shift
	out[1] -= shift * 0.5
	out[2] += shift * 0.5
	for channel := range 3 {
		out[channel] = min(max(out[channel], 0), 1)
	}
	return out
}

// TerrainBrightness 返回天空光为 sky（0..15）的面在给定 daylight 下的基础亮度。
// 调用方仍需乘以既有的朝向系数与 AO。
func TerrainBrightness(daylight float32, sky uint8) float32 {
	return indoorBrightness + float32(sky)/15*(daylight-indoorBrightness)
}
