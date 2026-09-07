package capture

import (
	"fmt"

	"github.com/channing771/mornlea/packages/shared/core"
)

// captureSnowCoverServerTick 是雪景场景钉死的权威 tick：必须大于 rain-noon 的
// 注入 tick（`1 << 19`，同一抓帧运行内预测器的单调校验已见过它——雪景排在双
// 机位之后，晴天场景不再抬升 tick），同时小于 water-underwater 的注入 tick
// （`1 << 20`），排在后面的末场景仍能单调前进。
const captureSnowCoverServerTick = uint64(1)<<19 + 1

// captureWinterSeasonProgress 是「冬中钉」的季内进度：SeasonWinter/128 重建
// yearPhase=(3+0.5)/4=0.875（冬季 [0.75,1) 的中点）。冬中（而非冬至）取值使
// 温度离雪点留出余量（海平面正午雨温约 −6.4℃），冷色 tint 权重仍有
// −sin(2π·0.875)≈0.71（上界 1 的七成），天空冷色与积雪稳定性两不牺牲。
const captureWinterSeasonProgress = uint8(128)

// captureWinterNoonDayPhaseOffset 是「冬中正午钉」的显示相位补偿：冬中昼弧
// 9454（DayArcTicks(0.875)，四舍五入取偶）下，worldTime%24000=6000 时的线性
// 相位需为 6000·9454/12000=4727 才能经白昼支路 warp 回正午 6000，offset =
// (4727−6000+24000)%24000 = 22727。补偿后天空与日照的相位输入与分点正午逐值
// 一致，画面差异只来自季节（冷色 tint）、天气（雪形降水）与预铺雪层。
const captureWinterNoonDayPhaseOffset = 22727

// captureSnowCoverBandMinX/MaxX 是预铺雪层带覆盖的 x 范围（含两端）：种子 42
// 的地形在机位右前方是湖、白名单地表集中在 x=-16..3（湖岸线沿 z 缓慢东移），
// 带窗口跟着地表走而不是以机位对称展开；两侧边界在四条带的距离上都处于水平
// 视场内，全部落在 3×3 夹具窗口里。
const (
	captureSnowCoverBandMinX = int32(-16)
	captureSnowCoverBandMaxX = int32(3)
)

// captureSnowCoverBand 是一条按 z 划分的雪层带：由近及远四条带依次铺 1..4 档，
// 相邻带的边界在相近的画面高度上形成 1/16 格的顶面高差对比（「不同档位对比
// 区」）；最近的一档带离机位最近、屏幕像素最大，最薄的 2/16 顶面因此仍可辨。
// 最远的 4 档带刻意比前三条深（6 行对 4 行）：远处行被湖面占去大半，加深行数
// 才保住一块与近带同量级的可辨区域。
type captureSnowCoverBand struct {
	minZ, maxZ int32
	tier       uint8
}

// captureSnowCoverBands 是四档雪层带的冻结分区表；生产夹具与测试锁各声明一份
// 语义（测试按坐标反查分区），任何一侧单独漂移都会红。
var captureSnowCoverBands = [...]captureSnowCoverBand{
	{minZ: 0, maxZ: 3, tier: 1},
	{minZ: -4, maxZ: -1, tier: 2},
	{minZ: -8, maxZ: -5, tier: 3},
	{minZ: -14, maxZ: -9, tier: 4},
}

// isSnowCoverSurface 报告 id 是否是可积雪的白名单地表（草、泥土、石头、沙、
// 砾石与整块雪顶面）。与 realm 随机 tick 的 `snowGroundWhitelist` 同一张表：
// 服务端约束的是机制写入，这里约束的是夹具预铺——两侧漂移会让夹具摆出机制
// 永远不会产生的雪层位（或漏铺机制会覆盖的位），golden 随之失真。
func isSnowCoverSurface(id core.BlockID) bool {
	switch id {
	case core.GrassID, core.DirtID, core.StoneID, core.SandID, core.GravelID, core.SnowBlockID:
		return true
	default:
		return false
	}
}

// prepareSnowCover 装入雪景场景的固定地形：复用橡树林种子 42 的 3×3 生成区块
// （与其余场景同一地形源），再按分区表把 1..4 档雪层预铺在镜头前的白名单地表
// 顶面上。选址只取「本列最高非空气格且恰为白名单地表」的列——树冠（树叶）、
// 短草与水面一律跳过，与积雪机制的露天判定同一形状，夹具因此不会摆出机制
// 产生不了的状态。种子固定，扫描与预铺逐字节确定。
func prepareSnowCover(app SceneApplication) error {
	if err := prepareOakGrove(app); err != nil {
		return err
	}
	blocks := make(map[core.ChunkPos]map[core.BlockPos]core.BlockID)
	for _, band := range captureSnowCoverBands {
		for z := band.minZ; z <= band.maxZ; z++ {
			for x := captureSnowCoverBandMinX; x <= captureSnowCoverBandMaxX; x++ {
				target, ok := snowCoverSurfaceCell(app, x, z)
				if !ok {
					continue
				}
				chunk := target.Chunk()
				if blocks[chunk] == nil {
					blocks[chunk] = make(map[core.BlockPos]core.BlockID)
				}
				blocks[chunk][target] = core.SnowLayer1BlockID + core.BlockID(band.tier) - 1
			}
		}
	}
	return applyCaptureBlocks(app, blocks, captureWaterBasinChunkRadius, "雪层四档")
}

// snowCoverSurfaceCell 自上而下扫描一列，返回白名单地表正上方的雪层落位
// （列顶+1）。列顶非白名单（树冠、短草、水面等）或整列无地表时不可铺。
func snowCoverSurfaceCell(app SceneApplication, x, z int32) (core.BlockPos, bool) {
	// 覆盖种子 42 地形与树冠的全部高度（地表约 64..80、树顶 90 上下），
	// 两侧各留一倍余量；越界高度由镜像按未加载处理，自然跳过。
	for y := int32(112); y >= 40; y-- {
		surface := core.BlockPos{X: x, Y: y, Z: z}
		block, loaded := app.Mirror().BlockAt(core.Overworld, surface)
		if !loaded || block == core.AirID {
			continue
		}
		if !isSnowCoverSurface(block) {
			return core.BlockPos{}, false
		}
		return core.BlockPos{X: x, Y: y + 1, Z: z}, true
	}
	return core.BlockPos{}, false
}

// captureWinterYearPhase 是冬中钉重建出的年相位：镜像两字段按
// `seasonalYearPhase`（(season+progress/256)/4）重建，(3+128/256)/4 恰为精确
// 二进制浮点 0.875——钉值与重建都是常量，昼弧（`DayArcTicks(0.875)`=9454）
// 与形态自验直接消费本常量，不经 `SceneApplication` 扩散镜像读方法。
const captureWinterYearPhase = 0.875

// pinCaptureWinterNoon 把呈现侧季节输入钉在「冬中正午 + 相位补偿」：镜像季节钉
// SeasonWinter/128（yearPhase=0.875），显示相位按当季昼弧 9454 补偿，使
// worldTime%24000=6000 下 warp 后的季节化相位仍恰 6000。恒等自验按同一对钉值
// 复算，昼弧或世界时间被改动时当场失败，不产出错位天空；形态自验再确认冬中
// 雨在海平面（画面中最暖的高度，更高只会更冷）为雪形——降水柱全高皆雪。钉值
// 只对本场景生效，下一场景由抓帧管线的 `pinCaptureSeasonBaseline` 复位。
func pinCaptureWinterNoon(app SceneApplication) error {
	if err := app.SetCaptureSeason(core.SeasonWinter, captureWinterSeasonProgress); err != nil {
		return err
	}
	if err := app.SetCaptureDayPhaseOffset(captureWinterNoonDayPhaseOffset); err != nil {
		return err
	}
	effPhase := core.EffectiveDayPhase(6000, captureWinterNoonDayPhaseOffset, core.DayArcTicks(captureWinterYearPhase))
	if effPhase != 6000 {
		return fmt.Errorf("冬中相位补偿后季节化相位 = %d，想要 6000（正午）", effPhase)
	}
	if !core.PrecipitationIsSnow(captureWinterYearPhase, effPhase, core.WeatherRain, core.TemperatureSeaLevelY) {
		return fmt.Errorf("冬中海平面雨温高于雪点 %v℃，降水形态不是雪", core.TemperatureSnowPoint)
	}
	return nil
}

// applySnowCoverCaptureState 钉死雪景场景的全部呈现状态：共享清场与机位复用橡
// 树林（同一地形同一机位，正午天空与日照的相位输入和既有基线逐值一致，画面
// 差异只来自冬季冷色 tint、雪形降水与预铺雪层），再钉冬中正午并注入固定雨
// 天——形态按温度公式逐粒派生为雪。场景已进 `captureScenes`（紧随
// camera-third-front、先于 main-menu），注入的雨天由后继菜单场景的公共清场复
// 位为晴天，冬季钉由抓帧管线的分点默认锚复位。
func applySnowCoverCaptureState(app SceneApplication) error {
	if err := applyOakGroveCaptureState(app); err != nil {
		return err
	}
	if err := pinCaptureWinterNoon(app); err != nil {
		return fmt.Errorf("钉住 snow-cover 冬中正午: %w", err)
	}
	return injectFixedWeather(app, captureSnowCoverServerTick, core.WeatherRain)
}
