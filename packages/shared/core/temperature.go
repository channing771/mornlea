package core

import "math"

// 温度：跨端共享的局部温度纯函数。服务端与客户端对同一（年相位、日内相
// 位、天气、高度）输入必须派生出相同的温度——服务端把它作为权威观察值随
// `PlayerState` 发布，客户端对每个降水粒子按其高度用同一公式本地求值（单点
// wire 温度无法表达每粒子海拔差异，双端共享公式本地求值是既定裁决）。温度
// 不进权威状态机，只是四个输入的确定函数；本文件不读墙钟、不进随机数、无
// 全局状态、无副作用。
//
// 合成公式（海平面温度再加海拔递减）：
//
//	海平面温度 = 季节基线 11 + 19·sin(2π·yearPhase)
//	          + 日内项 5·sin(2π·(effPhase−6000)/24000)
//	          + 降水降温（雨/雷暴 −4，晴 0）
//	任意高度   = 海平面温度 − 1.25·max(0, y − 64)
//
// 校准钢锚点全部钉在公式字面取值上：夏至正午海平面 30℃；夏至正午 Y=88 恰
// 0℃（24 格 × 1.25℃/格 恰抵消 30℃ 基线，高山夏雪与旧雪线锚连续）；冬至正
// 午海平面 −8℃（低地冬季全境降雪）。日内项因此在正午（6000）与午夜
// （18000）过零、黄昏（12000）+5、黎明（0）−5。

// TemperatureSnowPoint 是降水形态判定的雪点（0℃）：局部温度不高于该值时降
// 水为雪，见 `PrecipitationIsSnow`。
const TemperatureSnowPoint float32 = 0

// TemperatureMeltPoint 是融点（2℃）：与雪点构成 0..2℃ 的回差滞回带，防止
// 降水形态/积雪在边界来回抖动。本常量只在此落库，消费留给积雪能力（后续
// change），本轮温度公式不使用它。
const TemperatureMeltPoint float32 = 2

// TemperatureLapsePerBlock 是海平面以上的海拔递减率（℃/格）。锚定关系：
// 24 格恰 −30℃，把夏至正午海平面 30℃ 抵消到 Y=88 的 0℃——这是与旧静态雪线
// （`WeatherSnowLineY`）行为连续的校准基础，改动会平移全部高度上的雪线。
const TemperatureLapsePerBlock float32 = 1.25

// TemperatureSeaLevelY 是温度海拔递减的基准海平面 Y（64），与 Rust worldgen
// 的 `SEA_LEVEL_Y`（engine `worldgen.rs`，同为 64）语义对齐的 Go 侧镜像常
// 量：地形注水与温度递减共用同一海平面。双端各自持有常量、无编译期联动，
// 任一侧调整海平面必须双侧同步并升 ABI/契约版本。
const TemperatureSeaLevelY float32 = 64

// 温度合成结果的 clamp 上下界（℃）。落在 wire int8（−128..127）容量内，保证
// 协议字段永不需要二次裁剪；下界由极端组合真实触达（冬季黎明雷暴高山），
// 上界是容量护栏——公式理论最大值为夏至黄昏晴天海平面 35℃。
const (
	TemperatureMin float32 = -40
	TemperatureMax float32 = 45
)

// TemperatureAt 返回给定年相位、季节化日内相位、天气与高度处的局部温度
// （℃，float32 精度）。`effPhase` 沿用 `EffectiveDayPhase` 的语义（正午
// 6000、午夜 18000），合法值域 0..23999 由调用方钳制；越界值只按正弦周期
// 回绕，无未定义行为。海拔递减只在 `y` 高于 `TemperatureSeaLevelY` 时生
// 效，海平面及以下不升温。合成在 float64 中完成、clamp 后收窄为 float32。
func TemperatureAt(yearPhase float64, effPhase uint16, weather WeatherKind, y float32) float32 {
	seasonal := 11 + 19*math.Sin(2*math.Pi*yearPhase)
	diurnal := 5 * math.Sin(2*math.Pi*(float64(effPhase)-6000)/DayLengthTicks)
	var precip float64
	switch weather {
	case WeatherRain, WeatherThunder:
		precip = -4
	}
	var lapse float64
	if y > TemperatureSeaLevelY {
		lapse = -float64(TemperatureLapsePerBlock) * float64(y-TemperatureSeaLevelY)
	}
	t := seasonal + diurnal + precip + lapse
	if t < float64(TemperatureMin) {
		return TemperatureMin
	}
	if t > float64(TemperatureMax) {
		return TemperatureMax
	}
	return float32(t)
}

// PrecipitationIsSnow 报告给定条件下降水形态是否为雪：局部温度不高于雪点
// （`TemperatureSnowPoint`，含等值）即雪，否则雨。形态是温度的派生量而非权
// 威状态——客户端与预测端都必须经本谓词（间接经 `TemperatureAt`）判定，不
// 得自建雪线或温度算式。
func PrecipitationIsSnow(yearPhase float64, effPhase uint16, weather WeatherKind, y float32) bool {
	return TemperatureAt(yearPhase, effPhase, weather, y) <= TemperatureSnowPoint
}
