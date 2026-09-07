package entity

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 踩踏雪层留脚印（change snow-cover-accumulation）：落足实体水平位移累计跨过
// 固定步长时，把脚部所在格的雪层削低一档（1 档踩碎为空气）。玩家与被动牛共
// 用 `noteSnowFootprint` 同一判定；写块沿用耕地踩踏（trample.go）的两段式——
// 玩家物理推进位于订阅收敛之前，收集与结算必须分离，被动牛推进虽已在写入区
// 内，也汇入同一份暂存与同一结算出口，保证两类实体的脚印走同一条读判、写入
// 与广播路径。
//
// 几何取「脚部所在格」而非脚下支撑格：雪层全档无碰撞，实体由下方承载方块支
// 撑、脚部恰在雪层格内。站满格方块时脚底 Y 恰为整数格顶（如 1.0），floor 直
// 接取到上方雪层格——对照吃草/踩踏取支撑格时先减 `physics.GroundProbe` 的写
// 法，这里刻意不减。

// snowFootprintStride 是触发一次落足采样的水平位移累计阈值（格）。取值小于
// 一格宽：直线行进时每个经过的格子都有采样点落在其中，脚印链连续；同格内的
// 重复跨阈值由「落足格记忆」挡住，来回踱步不会把一格雪反复削穿。
const snowFootprintStride = float32(0.6)

// snowFootprintTracker 是单个实体的瞬态脚印累计器：`travel` 是自上次采样以来
// 的水平位移累计，`cell`/`cellValid` 记忆最近一次采样（无论那次是否真的写入）
// 的落足格，离开该格之前不再重复采样同格。全部字段只在串行权威 tick 内读写，
// 不落盘、不进快照与协议；实体消亡即随宿主状态一起丢弃，重启后从零开始。
type snowFootprintTracker struct {
	travel    float32
	cell      core.BlockPos
	cellValid bool
}

// snowFootprintCell 是一次跨阈值采样收集到的脚印候选格（维度必须随坐标一起
// 暂存，理由同 tramplePendingCell）；是否真是雪层由结算阶段的读判决定，收集
// 阶段不读方块。
type snowFootprintCell struct {
	dimension core.DimensionID
	position  core.BlockPos
}

// noteSnowFootprint 在物理推进后累计一个落足实体的水平位移，跨过阈值时采样
// 脚部所在格并登记候选。边界语义：
//   - 不在地面（跳起/下落/水中上浮途中）：既不累计也不清零——空中位移不是
//     「落足移动」，但已累计的里程保留到落地后继续，落地冲击可以立刻补上
//     缺口；
//   - 跨过阈值但落足格与上次采样相同：只清零累计、不登记——同一段站姿内
//     同格至多削一次，静止站立自然落在「位移不足」分支，永不触发；
//   - 落足格非雪层：结算读判直接跳过，累计已在采样时清零，跨雪/非雪边界
//     不会连续多削。
//
// 单次调用即单实体的一个权威 tick：无论该步位移多大都至多登记一个候选格，
// 「每实体每 tick ≤1 格」由此结构性保证。
func (engine *engineContext) noteSnowFootprint(
	tracker *snowFootprintTracker,
	dimension core.DimensionID,
	before, after mgl32.Vec3,
	onGround bool,
) {
	if !onGround {
		return
	}
	dx := after.X() - before.X()
	dz := after.Z() - before.Z()
	tracker.travel += float32(math.Sqrt(float64(dx*dx + dz*dz)))
	if tracker.travel < snowFootprintStride {
		return
	}
	tracker.travel = 0
	cell := blockPosOf(after)
	if tracker.cellValid && tracker.cell == cell {
		return
	}
	tracker.cell, tracker.cellValid = cell, true
	engine.snowFootprintPending = append(engine.snowFootprintPending, snowFootprintCell{
		dimension: dimension,
		position:  cell,
	})
}

// settleSnowFootprints 结算本 tick 收集到的全部脚印候选格并清空暂存（每 tick
// 一次，绝无跨 tick 残留）。由 `TickContext.SettleSnowFootprints` 在与耕地踩踏
// 相同的写入区调用（订阅收敛之后、随机 tick 之前）。收集序即结算序：玩家按
// 会话升序、被动牛按 id 升序，重放一致。
func (engine *engineContext) settleSnowFootprints(pending *pendingChunkChanges) {
	cells := engine.snowFootprintPending
	for index := range cells {
		engine.settleSnowFootprintCell(cells[index], pending)
	}
	engine.snowFootprintPending = engine.snowFootprintPending[:0]
}

// settleSnowFootprintCell 结算单个脚印候选格：读判雪层、削一档（1 档→空气）
// 并经 `recordChange` 汇入本 tick 变更批次（吃草先例：先写块成功再登记，
// `EnqueueFluidUpdate` 随之让邻接流体获得重估机会）。非雪层（含已被本 tick
// 更早候选削低的格之外的任何方块）、未加载区块与写入失败一律静默跳过——脚印
// 没有拒绝通道，放弃即可观察（雪层还在）且无信息丢失；多实体同格时各自至多
// 一次登记，读判永远基于最新方块状态，结果与结算次序无关。
func (engine *engineContext) settleSnowFootprintCell(
	cell snowFootprintCell,
	pending *pendingChunkChanges,
) {
	dimension := engine.dimension(cell.dimension)
	if dimension == nil {
		return
	}
	block, ready := dimension.BlockAt(cell.position)
	if !ready {
		return
	}
	tier, isLayer := core.SnowLayerTier(block)
	if !isLayer {
		return
	}
	next := core.AirID
	if tier > 1 {
		next = block - 1
	}
	if _, changed, err := dimension.SetBlock(cell.position, next); err != nil || !changed {
		return
	}
	engine.recordChange(cell.dimension, cell.position, next, pending)
}
