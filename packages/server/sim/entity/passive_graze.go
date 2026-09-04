package entity

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// 被动牛吃草事件的固定数值契约（边界测试锁定，不随玩家数或被动牛数放大）。
const (
	// passiveGrazePeriodTicks 是吃草抽选的分母：命中即 1/600（约 30 秒每牛期望一次）。
	passiveGrazePeriodTicks = 600
	// passiveGrazeDurationTicks 是单次吃草事件的低头时长（`tick`）。
	passiveGrazeDurationTicks = 20
	// passiveGrazeRollSalt 让吃草抽选的哈希流与漫游朝向派生互相独立：两者若同
	// 源，“朝某方向走”与“开始吃草”会出现结构性相关，某些朝向的牛永远不低头。
	passiveGrazeRollSalt = 0x51ab3e4d07c3f291
)

// passiveGrazeHit 报告该牛本 `tick` 是否命中吃草抽选：(`worldSeed`、`tick`、
// `id`) 的纯整数哈希对分母取模，不读全局随机数、不遍历 `map`，每牛每 `tick`
// 常数时间。
func passiveGrazeHit(seed int64, tick uint64, id uint64) bool {
	hash := splitmix64(uint64(seed) ^ passiveGrazeRollSalt)
	hash = splitmix64(hash ^ tick)
	return splitmix64(hash^id)%passiveGrazePeriodTicks == 0
}

// passiveGrazeSupport 返回实体脚下支撑格：几何取法与踩踏收集同形（脚底 Y 减
// `physics.GroundProbe` 后下取整），保证吃草读到的“站立方块”与物理支撑判定
// 是同一套数值尺度。
func passiveGrazeSupport(position mgl32.Vec3) core.BlockPos {
	return core.BlockPos{
		X: int32(math.Floor(float64(position.X()))),
		Y: int32(math.Floor(float64(position.Y() - physics.GroundProbe))),
		Z: int32(math.Floor(float64(position.Z()))),
	}
}

// advancePassiveGraze 推进全部被动牛的吃草事件。处理顺序即切片顺序（`id` 升
// 序），每牛每 `tick` 至多一次抽选、至多一次方块读判、至多一次单格写入。
func (engine *engineContext) advancePassiveGraze(pending *pendingChunkChanges) {
	for index := range engine.passives.entries {
		engine.advancePassiveGrazeOne(&engine.passives.entries[index], pending)
	}
}

// advancePassiveGrazeOne 推进单头牛的吃草事件。
//
// 不在事件中时触发条件（按廉价前置、昂贵后置排列）：未逃跑、本 `tick` 抽选
// 命中、脚下支撑格是草方块且所属 `chunk` 完整加载。触发拍计为事件第 1
// `tick`，落子后直接走一步事件中路径，因此恰好 20 次推进后结算。
//
// 事件中每拍先判中断再倒数：逃跑（有效伤害已由 `DamagePassive` 清事件态，
// 这里是防御性复核）、支撑格变化（移动或被挪走）、触发格非草或 `chunk` 不
// 再就绪（被挖、被翻或被卸载）都只终结事件、不写块。中断不碰任何“食欲”
// 状态——抽选本就无状态无冷却，下一拍命中且仍站草即可重开。
//
// 倒数归零时结算：重验触发格仍为草且 `chunk` 就绪，否则丢弃；写入经
// `SetBlock` 落世界再 `recordChange` 汇入本 `tick` 的变更批次，每事件至多
// 1 格。`touchChunk` 显式标记该区块被本事件触及（已有 `Record` 时它是幂等
// 无操作，`revision` 只推进一次）。
func (engine *engineContext) advancePassiveGrazeOne(
	entry *passiveState,
	pending *pendingChunkChanges,
) {
	if entry.grazeTicks == 0 && !engine.tryStartPassiveGraze(entry) {
		return
	}
	if entry.fleeTicks > 0 {
		entry.grazeTicks = 0
		return
	}
	dimension := engine.dimension(entry.dimension)
	if dimension == nil {
		entry.grazeTicks = 0
		return
	}
	if passiveGrazeSupport(entry.state.Position) != entry.grazePos {
		entry.grazeTicks = 0
		return
	}
	block, ready := dimension.BlockAt(entry.grazePos)
	if !ready || block != core.GrassID {
		entry.grazeTicks = 0
		return
	}
	entry.grazeTicks--
	if entry.grazeTicks == 0 {
		engine.settlePassiveGraze(entry, dimension, pending)
	}
}

// tryStartPassiveGraze 尝试为单头牛开启吃草事件：命中则记录触发格并装填
// 剩余 `tick`，返回是否进入事件。调用方紧接着走事件中路径完成触发拍。
func (engine *engineContext) tryStartPassiveGraze(entry *passiveState) bool {
	if entry.fleeTicks > 0 {
		return false
	}
	if !passiveGrazeHit(engine.seed, engine.tick.Load(), entry.id) {
		return false
	}
	dimension := engine.dimension(entry.dimension)
	if dimension == nil {
		return false
	}
	support := passiveGrazeSupport(entry.state.Position)
	block, ready := dimension.BlockAt(support)
	if !ready || block != core.GrassID {
		return false
	}
	entry.grazePos = support
	entry.grazeTicks = passiveGrazeDurationTicks
	return true
}

// settlePassiveGraze 结算到期的吃草事件：触发格仍为草才写泥土。调用方已在
// 同一拍内确认牛仍站在触发格上（事件期间移动输入被冻结，见
// `advancePassiveMovement`），这里只重验方块——单线程 `tick` 内方块不可能
// 在读判与写入之间变化，重验是幂等的丢弃守卫而非竞态修复。
func (engine *engineContext) settlePassiveGraze(
	entry *passiveState,
	dimension *Dimension,
	pending *pendingChunkChanges,
) {
	block, ready := dimension.BlockAt(entry.grazePos)
	if !ready || block != core.GrassID {
		return
	}
	if _, changed, err := dimension.SetBlock(entry.grazePos, core.DirtID); err != nil || !changed {
		return
	}
	engine.recordChange(entry.dimension, entry.grazePos, core.DirtID, pending)
	engine.touchChunk(
		core.ChunkKey{Dimension: entry.dimension, Pos: entry.grazePos.Chunk()},
		pending,
	)
}
