package runtime

import (
	"math"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 季节派生：runtime 持有的季节偏移（seed 派生、装配期写死）与绝对世界时间、
// 显示偏移的组合入口。每 tick 尾部与每次非 tick 玩家查询都经这里的同一快照
// 函数取值，保证发布路径与查询路径看到的季节/温度完全同源；派生全部是
// core 纯函数的 O(1) 组合，无分配、无 I/O，不进入随机数。

// seasonSnapshot 是一个绝对时刻的季节派生束：季节与季内进度是世界单值，
// yearPhase 与季节化日内相位供逐人温度求值复用（同一 tick 的所有玩家共享
// 同一份，温度只按各人 Y 分叉）。
type seasonSnapshot struct {
	season            core.Season
	progress          uint8
	yearPhase         float64
	effectiveDayPhase uint16
}

// seasonSnapshotAt 以（绝对世界时间、显示相位偏移、当 tick 天气所处时刻）为
// 快照派生季节束。昼夜弧随年相位伸缩，因此 effPhase 必须与 yearPhase 同一
// 快照求出，跨快照拼接会使温度公式的日内项漂移。
func (engine *Engine) seasonSnapshotAt(worldTime uint64, dayPhaseOffset uint16) seasonSnapshot {
	yearPhase := core.YearPhaseAt(worldTime, engine.seasonOffset)
	return seasonSnapshot{
		season:            core.SeasonAt(worldTime, engine.seasonOffset),
		progress:          core.SeasonProgressAt(worldTime, engine.seasonOffset),
		yearPhase:         yearPhase,
		effectiveDayPhase: core.EffectiveDayPhase(worldTime, dayPhaseOffset, core.DayArcTicks(yearPhase)),
	}
}

// temperatureAt 求给定高度处的观察温度并收窄为 wire 用的 int8：就近取整
// （half away from zero——.5 恰值时远离零取整，即标准库 math 包 Round 的
// 语义）。core 已把结果 clamp 到 [-40,45]，取整后仍落在 int8 域内，无需
// 二次裁剪。天气取当 tick 的权威种类（tick 路径为结果里的本 tick 结束值，
// 查询路径为引擎当前值）。
func (snapshot seasonSnapshot) temperatureAt(weather core.WeatherKind, y float32) int8 {
	temperature := core.TemperatureAt(
		snapshot.yearPhase, snapshot.effectiveDayPhase, weather, y,
	)
	return int8(math.Round(float64(temperature)))
}
