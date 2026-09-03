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
// 它完全由绝对世界时间与显示相位偏移决定（偏移只平移显示相位，不回写绝对
// 时间），不含任何可变状态。
type DayNight struct {
	Sun            float32
	Daylight       float32
	ClearColor     [4]float32
	SunDirection   [3]float32
	MoonDirection  [3]float32
	StarVisibility float32
}

// DayNightAt 按固定曲线计算给定绝对世界时间与显示相位偏移下的昼夜状态：
//
//	phase    = (worldTime mod 24000 + offset) mod 24000（经 `core.DisplayDayPhase`）
//	sun      = max(0, sin(2π·phase/24000))
//	daylight = 0.15 + 0.85*sun
//
// 偏移是服务端随权威玩家状态下发、客户端只读的显示相位单值（跳夜交付）：
// 全仓相位算式收敛在 `core.DisplayDayPhase`，客户端不得自建。云层漂移等绝对
// 时间驱动的呈现仍直接消费 worldTime（见 `CloudOffsetAt`），不受偏移影响。
func DayNightAt(worldTime uint64, dayPhaseOffset uint16) DayNight {
	phase := float64(core.DisplayDayPhase(worldTime, dayPhaseOffset)) / DayLengthTicks
	theta := 2 * math.Pi * phase
	sun := math.Sin(theta)
	if sun < 0 {
		sun = 0
	}
	starVisibility := 1 - (sun/0.25)*(sun/0.25)*(3-2*(sun/0.25))
	if sun >= 0.25 {
		starVisibility = 0
	}
	result := DayNight{
		Sun:            float32(sun),
		Daylight:       float32(0.15 + 0.85*sun),
		SunDirection:   [3]float32{float32(math.Cos(theta)), float32(math.Sin(theta)), 0},
		MoonDirection:  [3]float32{-float32(math.Cos(theta)), -float32(math.Sin(theta)), 0},
		StarVisibility: float32(starVisibility),
	}
	for index := range result.ClearColor {
		night, day := nightSkyColor[index], daySkyColor[index]
		result.ClearColor[index] = night + (day-night)*result.Sun
	}
	return result
}

// TerrainBrightness 返回天空光为 sky（0..15）的面在给定 daylight 下的基础亮度。
// 调用方仍需乘以既有的朝向系数与 AO。
func TerrainBrightness(daylight float32, sky uint8) float32 {
	return indoorBrightness + float32(sky)/15*(daylight-indoorBrightness)
}
