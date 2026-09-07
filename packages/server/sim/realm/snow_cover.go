package realm

import (
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// 积雪与消融：随机 tick 命中白名单地表格时的确定性雪层推进。机制契约（change
// snow-cover-accumulation）：权威天气处于降水段且目标格局部温度 ≤ 雪点 0℃ 时，
// 露天白名单地表逐档加厚（上限 4 档）；局部温度 > 融点 2℃ 时逐档消融（1 档→空
// 气）；0..2℃ 回差区间稳定不动。全部判定由（世界种子、tick、坐标）经既有随机
// tick 抽样派生，本文件不引入第二条随机流、不新增遍历——预算与作物/耕地共用
// `RandomTicksPerSection` 框架，每次命中至多写 1 格。

// snowGroundWhitelist 报告 id 是否是可积雪白名单地表：草、泥土、石头、沙、砾石
// 与整块雪的顶面。雪层只落在这类方块的正上方；非白名单（耕地、木板、玻璃等）
// 一律不积。白名单全部是实心非流体方块，因此命中白名单格时「本格是流体」的水
// 下分支天然不成立。
func snowGroundWhitelist(id core.BlockID) bool {
	switch id {
	case core.GrassID, core.DirtID, core.StoneID, core.SandID, core.GravelID, core.SnowBlockID:
		return true
	default:
		return false
	}
}

// snowPrecipitating 报告权威天气是否处于降水段（雨或雷暴）：积雪只在降水期间发
// 生；晴天即使深冬也只保留既有雪层（消融另按温度判定，与降水无关）。
func snowPrecipitating(weather core.WeatherKind) bool {
	return weather == core.WeatherRain || weather == core.WeatherThunder
}

// advanceSnowCover 处理随机 tick 命中的一格地表格的雪层推进，至多写上方 1 格。
//
// 判定序（与设计钉死的顺序一致）：
//
//  1. 上方格已是雪层：温度 > 融点降 1 档（1 档→空气）；温度 ≤ 雪点且降水升 1 档
//     （≤4 档，到顶不再写入）；0..2℃ 回差区间不动。已有雪层不走放置判定的空
//     气前置，但升档继承露天条件——列顶门要求本列最高非空气格不高于雪层自身
//     （`HighestOpaque <= 雪层Y`：正常态列顶恰为雪层，天然通过；玩家在雪层上
//     加顶盖后列顶被抬高，升档被拒，机制外状态不会在顶盖下继续加厚）。消融分
//     支刻意不吃列顶门：顶盖下回暖照常逐档消——「升档（同 1）」的露天语义只
//     约束加厚，不约束消退。
//  2. 上方格为空气、本格是白名单地表且是本列最高非空气格（heightmap 顶，天空
//     直射）：温度 ≤ 雪点且降水时上方格置 1 档。上方为流体（水下）、悬挑下
//     （列顶更高）与非白名单地表都不写。
//
// 局部温度对上方格（雪层所在/将落位的高度）求值；上方格与本格同列同区块，读取
// 与写入都不跨区块——未就绪区块在抽样循环入口已被过滤，上方越出世界高度时
// `Dimension.SetBlock` 以错误拒绝，静默丢弃。写入走 `Dimension.SetBlock` +
// `Mutation.Record`（作物先例：先写块成功再登记，避免幽灵变更；雪层不是流体，
// 不入流体与耕地湿度队列）。
func (state *State) advanceSnowCover(
	dimension *Dimension,
	dimensionID core.DimensionID,
	chunk *world.Chunk,
	position core.BlockPos,
	block core.BlockID,
	mutation *Mutation,
) {
	if !snowGroundWhitelist(block) {
		return
	}
	above := core.BlockPos{X: position.X, Y: position.Y + 1, Z: position.Z}
	aboveBlock, ready := dimension.BlockAt(above)
	state.environment.cropBlockReads++
	if !ready {
		return
	}
	localX, _, localZ := position.Local()
	climate := state.environment.config
	temperature := core.TemperatureAt(
		climate.YearPhase, climate.EffectiveDayPhase, climate.Weather, float32(above.Y),
	)
	if tier, isLayer := core.SnowLayerTier(aboveBlock); isLayer {
		var next core.BlockID
		switch {
		case temperature > core.TemperatureMeltPoint:
			if tier == 1 {
				next = core.AirID
			} else {
				next = aboveBlock - 1
			}
		case temperature <= core.TemperatureSnowPoint && snowPrecipitating(climate.Weather) &&
			tier < 4 && chunk.HighestOpaque(localX, localZ) <= above.Y:
			next = aboveBlock + 1
		default:
			return
		}
		if _, changed, err := dimension.SetBlock(above, next); err != nil || !changed {
			return
		}
		mutation.Record(dimensionID, above, next)
		return
	}
	if aboveBlock != core.AirID || temperature > core.TemperatureSnowPoint || !snowPrecipitating(climate.Weather) {
		return
	}
	if chunk.HighestOpaque(localX, localZ) != position.Y {
		return
	}
	if _, changed, err := dimension.SetBlock(above, core.SnowLayer1BlockID); err != nil || !changed {
		return
	}
	mutation.Record(dimensionID, above, core.SnowLayer1BlockID)
}
