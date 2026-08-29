package entity

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/realm"
)

// 夜间生成的固定数值契约（边界测试锁定，不随世界规模放大）。周期/上限与
// `internal/storage` 存档 codec 的同值常量保持同源，任何一侧单独调整都必须
// 同步另一侧。
const (
	// 夜间生成窗口（显示相位，含端点）：经 `core.DisplayDayPhase` 折算。
	hostileSpawnPhaseStart = 13000
	hostileSpawnPhaseEnd   = 23000
	// 候选到锚点玩家的水平距离窗（含端点）。
	hostileSpawnMinRadius = int64(24)
	hostileSpawnMaxRadius = int64(48)
	// 生成门槛：候选哈希低 8 位 <13 才尝试（13/256 概率）。
	hostileSpawnGateThreshold = 13
	// 候选 ID 冲突时的重散列预算；耗尽仍冲突则本 tick 放弃。
	hostileSpawnMaxRehashes = 64
	// 候选格局部区块光的暗度判定上限（≤7 视为足够暗）。
	hostileSpawnLightLimit = 7
	// 任一 active 玩家水平 48 格半径内的夜行者数量上限。
	maxHostilesNearPlayer = 8
)

// hostileSpawnColumn 从本 tick 的基准哈希派生唯一候选列：半径取基准哈希的
// 低位列（24..48），轴向取高位段的二位（±x/±z 四轴），候选 = 锚点方块 +
// 轴向量 × 半径。全部输入为整数、无浮点、无全局 RNG，相同 (seed, tick, 锚点)
// 的派生逐位一致。
func hostileSpawnColumn(base uint64, anchorX, anchorZ int32) (x, z int32, radius int64, axis int) {
	radius = hostileSpawnMinRadius + int64(base%25)
	axis = int((base >> 32) & 3)
	deltaX := [4]int32{1, -1, 0, 0}[axis]
	deltaZ := [4]int32{0, 0, 1, -1}[axis]
	return anchorX + deltaX*int32(radius), anchorZ + deltaZ*int32(radius), radius, axis
}

// hostileCandidateHash 把候选坐标折进哈希链：基准哈希（seed^tick 过
// `splitmix64`）先混入 X/Z 的零扩展 uint32，再按同样的传播混入 Y。该哈希的
// 低 8 位是生成门槛，其非零值本身即候选 ID——ID 与门槛同源，重放必然逐位
// 一致。
func hostileCandidateHash(seed int64, tick uint64, x, y, z int32) uint64 {
	hash := splitmix64(uint64(seed) ^ tick)
	hash = splitmix64(hash ^ uint64(uint32(x)) ^ uint64(uint32(z)))
	return splitmix64(hash ^ uint64(uint32(y)))
}

// advanceHostileSpawn 是权威 tick 的夜间生成判定：每 tick 恰好推导一个候选
// （锚点玩家、候选列、落点 Y 依次确定），全部必要条件按「廉价前置、昂贵后
// 置」的顺序校验，任一不成立即放弃且本 tick 不再考察其它候选。生成发生在
// tick 边界、先于物理阶段；新个体下一 tick 才参与积分（见
// advanceHostileMovement 的 fresh 约定）。
//
// 条件清单（spec「夜间在暗处确定性生成」）：夜间窗口、active 锚点、全服
// ≤64、候选 chunk 完整加载、双格空气 + 下方 solid + 非流体落点、门槛哈希、
// 每玩家 48 格内 ≤8、局部区块光 ≤7、非零唯一 ID。整个判定只读世界，绝不
// 为生成触发同步加载。
func (engine *Engine) advanceHostileSpawn() {
	now := engine.worldTime.Load()
	phase := core.DisplayDayPhase(now, engine.DayPhaseOffset())
	if phase < hostileSpawnPhaseStart || phase > hostileSpawnPhaseEnd {
		return
	}
	sessions := engine.sortedActiveSessions()
	if len(sessions) == 0 {
		return
	}
	// 全服上限是最廉价的前置：满编时连锚点推导都不必做。
	if len(engine.hostiles.entries) >= maxHostiles {
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
	y, ok := hostileSpawnColumnSpot(dimension, x, z)
	if !ok {
		return
	}
	hash := hostileCandidateHash(engine.seed, now, x, y, z)
	if hash&0xFF >= hostileSpawnGateThreshold {
		return
	}
	candidate := mgl32.Vec3{float32(x) + 0.5, float32(y), float32(z) + 0.5}
	if engine.hostileNearLimitExceeded(anchorSession.dimension, candidate) {
		return
	}
	if engine.hostileBlockLight(dimension, core.BlockPos{X: x, Y: y, Z: z}) > hostileSpawnLightLimit {
		return
	}
	// ID 即候选哈希（非零）；与既有个体冲突时沿哈希链重散列，预算耗尽仍
	// 冲突则放弃本 tick，绝不截断或覆盖既有集合。
	id := hash
	for attempt := 0; attempt < hostileSpawnMaxRehashes; attempt++ {
		if id != 0 && engine.hostiles.findIndex(id) < 0 {
			engine.hostiles.insert(hostileState{
				state:        physics.State{Position: candidate, OnGround: true},
				id:           id,
				dimension:    anchorSession.dimension,
				health:       core.MaxHealth,
				burnCooldown: hostileCooldownPeriodTicks,
				fresh:        true,
			})
			return
		}
		id = splitmix64(id)
	}
}

// hostileSpawnColumnSpot 自上而下扫描候选列，返回第一处「双格空气 + 下方
// solid + 候选格与支撑格均非流体」的落点 Y。solid 判据复用权威碰撞表
// （`physics.BlockCollisionBoxes`）：提供碰撞体的方块才算支撑，流体与作物、
// 火把一样不构成支撑。列内所有方块同属一个 chunk，调用方已保证其完整加载。
func hostileSpawnColumnSpot(dimension *Dimension, x, z int32) (int32, bool) {
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
		if !ready || core.IsFluid(support) {
			continue
		}
		if physics.BlockCollisionBoxes(support, true).Count == 0 {
			continue
		}
		return y, true
	}
	return 0, false
}

// hostileNearLimitExceeded 报告「任一 active 玩家水平 48 格内已有 8 只夜行
// 者，且候选落在该玩家的 48 格半径内」。距离全部在平方域比较，避免开方；
// 统计范围只含同维个体，成本至多 active 玩家数 × 64。
func (engine *Engine) hostileNearLimitExceeded(dimensionID core.DimensionID, candidate mgl32.Vec3) bool {
	nearSq := float32(maxHostilesNearRadius) * float32(maxHostilesNearRadius)
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
		for index := range engine.hostiles.entries {
			entry := &engine.hostiles.entries[index]
			if entry.dimension == dimensionID &&
				horizontalDistanceSq(entry.state.Position, playerPos) <= nearSq {
				count++
			}
		}
		if count >= maxHostilesNearPlayer {
			return true
		}
	}
	return false
}

// maxHostilesNearRadius 是「玩家附近」的水平半径（格），与 spec 的 48 一致；
// 独立成常量是为了让上限（8）与半径（48）两个契约值各自可引用。
const maxHostilesNearRadius = 48
