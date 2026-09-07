//go:build darwin

package app

import (
	"github.com/channing771/mornlea/packages/shared/core"
)

// seasonalYearPhase 由镜像的季节两字段重建呈现侧年相位：
//
//	yearPhase = (season + progress/256) / 4
//
// 量化来源：progress 是服务端 `core.SeasonProgressAt` 的 0..255 u8 量化
// （floor(季内已过 tick·256/72000)），重建因此带固有量化台阶——progress 每
// 前进 1，yearPhase 前进 1/1024（约 281 权威 tick 一次），昼弧与冷色权重对
// 该量级的敏感度远低于一次可见台阶，量化误差已裁决接受（见 change
// ledger 4.1：客户端 yearPhase 由镜像 Season+SeasonProgress 重建）。
//
// 接受台阶：season/seasonProgress 与 weather 同一接受纪律——只认更新
// `ServerTick` 的权威状态、随 `worldTimeFrozen` 一并钉住、客户端不按本地
// 世界时间外插；yearPhase 因此只在权威状态到达时按上述量化步进前进，
// 帧内恒定。昼夜 warp 与降水形态判定统一消费本函数的输出，不得各自从
// 世界时间自建季节算式。
func seasonalYearPhase(season core.Season, progress uint8) float64 {
	return (float64(season) + float64(progress)/256) / 4
}

// YearPhase 返回呈现侧年相位：由镜像 Season+SeasonProgress 经量化重建（见
// `seasonalYearPhase`），供昼夜弧 warp、冬季冷色 tint 与降水形态判定消费；
// 抓帧侧经 `SetCaptureSeason` 直写镜像后同样经它读回。
func (a *Application) YearPhase() float64 {
	return seasonalYearPhase(a.season, a.seasonProgress)
}
