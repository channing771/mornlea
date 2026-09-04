package entity

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// 被动牛昼间生成的固定数值契约（边界测试锁定，不随世界规模放大）。
const (
	// 任一 `active` 玩家水平 48 格半径内的被动牛数量上限。
	maxPassivesNearPlayer = 6
	// 候选 `id` 冲突时的重散列预算；耗尽仍冲突则本 `tick` 放弃。
	passiveSpawnMaxRehashes = 64
)

// maxPassivesNearRadius 是「玩家附近」的水平半径（格），与需求的 48 一致；
// 独立成常量是为了让上限（6）与半径（48）两个契约值各自可引用。
const maxPassivesNearRadius = 48

// advancePassiveSpawn 是权威 `tick` 的被动牛昼间生成判定：每 `tick` 恰好推导
// 一个候选（锚点玩家、候选列、落点 `Y` 依次确定），全部必要条件按「廉价前
// 置、昂贵后置」的顺序校验，任一不成立即放弃且本 `tick` 不再考察其它候选。
// 生成发生在 `tick` 边界、先于物理阶段；新个体下一 `tick` 才参与积分（见
// `advancePassiveMovement` 的 `fresh` 约定）。
//
// 条件清单：白昼相位、`active` 锚点、全服不超过 32 头、候选 `chunk` 完整加
// 载、草方块正上方双格空气 + 下方 solid + 候选格与支撑格均非流体、每玩家
// 48 格内不超过 6 头、非零唯一 `id`。候选列派生复用夜行者侧的同窗函数（水平
// 距离 24..48 的整数派生），`id` 复用同一条候选哈希链（两套集合相互独立，
// `id` 无需跨集合唯一）；被动侧不设概率门槛，通过全部条件的候选必须生成。
// 整个判定只读世界，绝不为生成触发同步加载。
func (engine *engineContext) advancePassiveSpawn() {
	now := engine.worldTime.Load()
	if !phaseIsDay(core.DisplayDayPhase(now, engine.DayPhaseOffset())) {
		return
	}
	sessions := engine.sortedActiveSessions()
	if len(sessions) == 0 {
		return
	}
	// 全服上限是最廉价的前置：满编时连锚点推导都不必做。
	if len(engine.passives.entries) >= maxPassives {
		return
	}
	anchorSession := engine.sessions[sessions[now%uint64(len(sessions))]]
	if anchorSession == nil || anchorSession.player == nil {
		return
	}
	dimension := engine.dimension(anchorSession.dimension)
	if dimension == nil {
		return
	}
	anchor := blockPosOf(anchorSession.player.state.Position)
	x, z, _, _ := hostileSpawnColumn(splitmix64(uint64(engine.seed)^now), anchor.X, anchor.Z)
	if info, ok := dimension.Info(core.BlockPos{X: x, Z: z}.Chunk()); !ok || info.State != realm.ChunkReady {
		return
	}
	y, ok := passiveSpawnColumnSpot(dimension, x, z)
	if !ok {
		return
	}
	candidate := mgl32.Vec3{float32(x) + 0.5, float32(y), float32(z) + 0.5}
	if engine.passiveNearLimitExceeded(anchorSession.dimension, candidate) {
		return
	}
	// `id` 即候选哈希（非零）；与既有个体冲突时沿哈希链重散列，预算耗尽仍
	// 冲突则放弃本 `tick`，绝不截断或覆盖既有集合。
	id := hostileCandidateHash(engine.seed, now, x, y, z)
	for attempt := 0; attempt < passiveSpawnMaxRehashes; attempt++ {
		if id != 0 && engine.passives.findIndex(id) < 0 {
			engine.passives.insert(passiveState{
				state:     physics.State{Position: candidate, OnGround: true},
				id:        id,
				dimension: anchorSession.dimension,
				health:    core.MaxHealth,
				home:      core.BlockPos{X: x, Y: y, Z: z}.Chunk(),
				fresh:     true,
			})
			return
		}
		id = splitmix64(id)
	}
}

// passiveSpawnColumnSpot 自上而下扫描候选列，返回第一处「双格空气 + 下方为
// 草方块支撑 + 候选格与支撑格均非流体」的落点 `Y`。solid 判据复用权威碰撞
// 表（`physics.BlockCollisionBoxes`）：提供碰撞体的方块才算支撑，流体与作
// 物、火把一样不构成支撑；草方块相等性是本判定的特有条件，其余分支与夜行者
// 侧同形。列内所有方块同属一个 `chunk`，调用方已保证其完整加载。
func passiveSpawnColumnSpot(dimension *Dimension, x, z int32) (int32, bool) {
	for y := int32(core.MaxY - 2); y >= core.MinY+1; y-- {
		lower, ready := dimension.BlockAt(core.BlockPos{X: x, Y: y, Z: z})
		if !ready || lower != core.AirID || core.IsFluid(lower) {
			continue
		}
		upper, ready := dimension.BlockAt(core.BlockPos{X: x, Y: y + 1, Z: z})
		if !ready || upper != core.AirID || core.IsFluid(upper) {
			continue
		}
		support, ready := dimension.BlockAt(core.BlockPos{X: x, Y: y - 1, Z: z})
		if !ready || support != core.GrassID || core.IsFluid(support) {
			continue
		}
		if physics.BlockCollisionBoxes(support, true).Count == 0 {
			continue
		}
		return y, true
	}
	return 0, false
}

// passiveNearLimitExceeded 报告「任一 `active` 玩家水平 48 格内已有 6 头被动
// 牛，且候选落在该玩家的 48 格半径内」。距离全部在平方域比较，避免开方；
// 统计范围只含同维个体，成本至多 `active` 玩家数 × 32。
func (engine *engineContext) passiveNearLimitExceeded(dimensionID core.DimensionID, candidate mgl32.Vec3) bool {
	nearSq := float32(maxPassivesNearRadius) * float32(maxPassivesNearRadius)
	for _, id := range engine.sortedActiveSessions() {
		session := engine.sessions[id]
		if session == nil || session.player == nil || session.dimension != dimensionID {
			continue
		}
		playerPos := session.player.state.Position
		if horizontalDistanceSq(playerPos, candidate) > nearSq {
			continue
		}
		count := 0
		for index := range engine.passives.entries {
			entry := &engine.passives.entries[index]
			if entry.dimension == dimensionID &&
				horizontalDistanceSq(entry.state.Position, playerPos) <= nearSq {
				count++
			}
		}
		if count >= maxPassivesNearPlayer {
			return true
		}
	}
	return false
}
