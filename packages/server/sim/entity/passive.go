package entity

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/world"
)

// 被动牛的固定数值契约。这些值由边界测试锁定，不随玩家数或被动牛数放大。
const (
	// maxPassives 是全服同时存在的被动牛数量上限。
	maxPassives = 32
	// passiveFleeDurationTicks 是单次受击后逃跑的固定时长（`tick`）。
	passiveFleeDurationTicks uint8 = 60
	// passiveIdleLookRadius 是闲时看人的水平半径（格）：同维最近 `active`
	// 玩家进入此半径内（含边界）且逃跑/吃草/引诱均未生效时，牛原地转向该
	// 玩家；离开半径即恢复漫游朝向派生。
	passiveIdleLookRadius = 6
	// passiveIdleLookMaxTurn 是闲时转向每 `tick` 的有界转向角（弧度）：不瞬
	// 移，多拍收敛。
	passiveIdleLookMaxTurn = float32(0.2)
	// passiveIdleLookStopDistance 是闲时靠近的个人空间止步距离（格）：进入
	// 后只冻位移不冻朝向，避免顶进玩家身体；离开 6 格半径即恢复漫游。
	passiveIdleLookStopDistance = float32(1.5)
)

// passiveState 是一头被动牛的权威身体事实。字段面与夜行者侧对齐但类型独立：
// `state` 是与玩家同形的物理体，`id`/`dimension`/`yaw`/`health` 组成持久化身
// 体事实（经 `PassiveMob` 快照进出，转换由 `server` 装配层以后完成；快照上
// 的瞬态 `Grazing` 呈现位只读事件态、不参与恢复）；另有三项运行时派生物：
// `input` 是本 `tick` 的控制意图（漫游或逃跑朝向），`home` 是出生区块（漫
// 游不得离开其邻域，重启后以加载位置重新锚定，不落盘），`fleeTicks`/
// `fleeFrom` 是受击逃跑的剩余时长与伤害来源（瞬态，不进快照或存档），
// `grazeTicks`/`grazePos` 是吃草事件的剩余时长与触发格（瞬态，不进快照或
// 存档，重启后事件自然消失），`fresh` 标记本 `tick` 刚生成的个体——生成次
// 序在物理之前，但新个体下一 `tick` 才参与积分。
//
// 被动牛没有攻击、目标与灼烧语义：此处刻意不设夜行者侧的冷却与目标字段，任
// 何还击能力在类型层面即不存在。
//
// 集合形态复用夜行者先例：全服至多 32 头，按 `id` 严格升序的定容切片配合二
// 分查找已是最廉价的确定性结构。
type passiveState struct {
	state physics.State
	input physics.Input
	id    uint64
	// dimension 恒为被动牛所在维度；恢复与生成都校验维度存在。
	dimension core.DimensionID
	yaw       float32
	// health 合法区间 1..`core.MaxHealth`；归零的个体在同一权威 `tick` 内走死
	// 亡结算（移除 + 掉落）后不会留存。
	health    uint8
	home      core.ChunkPos
	fleeTicks uint8
	fleeFrom  mgl32.Vec3
	// grazeTicks 是吃草事件剩余 `tick`（0 表示不在事件中），`grazePos` 是触
	// 发时记录的草方块坐标：结算只写这一格，瞬态内存字段，不进快照或存档。
	grazeTicks uint8
	grazePos   core.BlockPos
	fresh      bool
}

// passiveSet 是按 `id` 严格升序维护的被动牛集合。切片容量在构造时按上限
// 预分配，热路径上不产生任何增长式分配。
type passiveSet struct {
	entries []passiveState
}

func newPassiveSet() passiveSet {
	return passiveSet{entries: make([]passiveState, 0, maxPassives)}
}

// findIndex 二分查找 `id`，返回命中下标；未命中返回 -1。
func (set *passiveSet) findIndex(id uint64) int {
	low, high := 0, len(set.entries)-1
	for low <= high {
		mid := int(uint(low+high) >> 1)
		switch {
		case set.entries[mid].id < id:
			low = mid + 1
		case set.entries[mid].id > id:
			high = mid - 1
		default:
			return mid
		}
	}
	return -1
}

// insert 以二分定位把新个体插入升序位置。重复 `id` 或超出容量时拒绝并返回
// `false`——拒绝是生成与恢复两条入口共享的边界，绝不截断或覆盖既有个体。
func (set *passiveSet) insert(entry passiveState) bool {
	if len(set.entries) >= maxPassives {
		return false
	}
	index := sort.Search(len(set.entries), func(i int) bool {
		return set.entries[i].id >= entry.id
	})
	if index < len(set.entries) && set.entries[index].id == entry.id {
		return false
	}
	set.entries = append(set.entries, passiveState{})
	copy(set.entries[index+1:], set.entries[index:])
	set.entries[index] = entry
	return true
}

// removeAt 删除指定下标的个体并保持其余个体的相对顺序（升序）不变。
func (set *passiveSet) removeAt(index int) {
	copy(set.entries[index:], set.entries[index+1:])
	set.entries = set.entries[:len(set.entries)-1]
}

// RestorePassive 把一条身体记录恢复为权威事实。校验判据是夜行者记录矩阵的
// 子集（`id` 非零且不重复、维度存在、身体状态有限且位置在世界高度内、生命
// 为正且不超上限）：逃跑、吃草与出生区块都是运行时派生物，恢复时分别清零
// 与按加载位置重新锚定——记录上的瞬态 `Grazing` 位被忽略，重启后事件恒不
// 恢复。任何一条不成立都整体拒绝且不改变既有集合。
func (engine *engineContext) RestorePassive(mob PassiveMob) error {
	if err := validatePassiveMob(mob); err != nil {
		return err
	}
	if engine.dimension(mob.Dimension) == nil {
		return fmt.Errorf("sim: passive dimension %d is unknown", mob.Dimension)
	}
	if engine.passives.findIndex(mob.ID) >= 0 {
		return fmt.Errorf("sim: duplicate passive ID %d", mob.ID)
	}
	entry := passiveState{
		state:     mob.State,
		input:     physics.Input{Yaw: mob.Yaw},
		id:        mob.ID,
		dimension: mob.Dimension,
		yaw:       mob.Yaw,
		health:    mob.Health,
		home:      blockPosOf(mob.State.Position).Chunk(),
	}
	if !engine.passives.insert(entry) {
		return fmt.Errorf("sim: passive capacity %d exhausted", maxPassives)
	}
	return nil
}

// validatePassiveMob 校验单条被动牛记录的全部不变量。
func validatePassiveMob(mob PassiveMob) error {
	if mob.ID == 0 {
		return errors.New("sim: passive ID is zero")
	}
	if !physics.ValidState(mob.State) {
		return errors.New("sim: passive state is not finite")
	}
	if mob.State.Position.Y() < float32(core.MinY) || mob.State.Position.Y() >= float32(core.MaxY) {
		return fmt.Errorf("sim: passive position Y %v outside world", mob.State.Position.Y())
	}
	if mob.Health == 0 || mob.Health > core.MaxHealth {
		return fmt.Errorf("sim: passive health %d outside 1..%d", mob.Health, core.MaxHealth)
	}
	return nil
}

// PassiveMobs 返回按 `id` 升序的全量值快照。调用频率由保存与发布节奏决定，
// 不在权威 `tick` 热路径上，因此分配一份新切片。
func (engine *engineContext) PassiveMobs() []PassiveMob {
	mobs := make([]PassiveMob, 0, len(engine.passives.entries))
	for index := range engine.passives.entries {
		mobs = append(mobs, engine.passiveMobAt(index))
	}
	return mobs
}

func (engine *engineContext) passiveMobAt(index int) PassiveMob {
	entry := &engine.passives.entries[index]
	return PassiveMob{
		ID:        entry.id,
		Dimension: entry.dimension,
		State:     entry.state,
		Yaw:       entry.yaw,
		Health:    entry.health,
		Grazing:   entry.grazeTicks > 0,
	}
}

func (passive *passiveState) applyDamage(damage int32) {
	if damage <= 0 {
		return
	}
	if damage >= int32(passive.health) {
		passive.health = 0
		return
	}
	passive.health -= uint8(damage)
}

// DamagePassive 结算一次对被动牛的有效伤害并触发逃跑：生命按夜行者同规则扣
// 减（归零由本 `tick` 稍后的死亡结算统一移除与掉落），同时以固定时长与伤害
// 来源位置进入逃跑状态。有效伤害同时终结吃草事件（只清事件态、不写块；抽
// 选无状态无冷却，无需也不存在可清的“食欲”）。非正伤害对已知个体是无操作
// 受理（生命与漫游不变，吃草也不受打扰）；未知 `id` 整体拒绝并返回 `false`，
// 绝不留下半更新的逃跑事实。
func (engine *engineContext) DamagePassive(id uint64, damage int32, from mgl32.Vec3) bool {
	index := engine.passives.findIndex(id)
	if index < 0 {
		return false
	}
	entry := &engine.passives.entries[index]
	if damage <= 0 {
		return true
	}
	entry.applyDamage(damage)
	entry.fleeTicks = passiveFleeDurationTicks
	entry.fleeFrom = from
	entry.grazeTicks = 0
	return true
}

// advancePassives 只推进生成、移动与吃草；死亡结算由调用方在被动阶段紧随其
// 后按固定生命周期顺序编排。吃草排在移动之后：事件结算的写块与移动冻结读
// 的是同一拍的位置，顺序固定则重放一致。
func (engine *engineContext) advancePassives(pending *pendingChunkChanges) {
	engine.advancePassiveSpawn()
	engine.advancePassiveMovement()
	engine.advancePassiveGraze(pending)
}

// advancePassiveMovement 把全部被动牛汇入与玩家/夜行者相同的 `Rust`
// `physics.Step` 积分出口：每个个体用本 `tick` 的漫游或逃跑输入步进恰好一
// 次，位移完全由权威物理决定，不新写任何 `Go` 积分。处理顺序即切片顺序
// （`id` 升序）。
//
// 本 `tick` 刚生成的个体（`fresh`）跳过并清除标记：生成判定发生在 `tick` 边
// 界、先于物理，但新生个体的第一次位移从下一 `tick` 开始。坠出世界或状态失
// 真的个体按确定性移除处理（不掉落、不保留半移除状态），移除阈值与夜行者侧
// 一致（`Y < core.MinY` 即移除）。积分后若落点离开出生区块邻域（切比雪夫距
// 离大于 1 区块），回滚到步进前状态——即「无路径时停止」的确定性形态，穿墙
// 则由权威碰撞在积分内直接阻止。
func (engine *engineContext) advancePassiveMovement() {
	for index := 0; index < len(engine.passives.entries); {
		entry := &engine.passives.entries[index]
		if entry.fresh {
			entry.fresh = false
			index++
			continue
		}
		// 吃草事件期间牛静止（与漫游/逃跑输入无仲裁：冻结即中断了位移来
		// 源，有效伤害则已由 `DamagePassive` 先行终结事件），因此跳过积分、
		// 朝向更新与邻域回滚，位置逐位冻结。
		if entry.grazeTicks > 0 {
			entry.input = physics.Input{Yaw: entry.yaw}
			index++
			continue
		}
		entry.input = engine.passiveStepInput(entry)
		source := dimensionCollisionSource{dimension: engine.dimension(entry.dimension)}
		input := entry.input
		input.BodyInFluid, input.EyeInFluid = physics.SubmersionFlagsWithTunables(
			entry.state.Position, source, engine.physicsTunables,
		)
		previous := entry.state
		entry.state = physics.StepWithTunables(
			entry.state, input, source, engine.physicsTunables,
		).State
		if !physics.ValidState(entry.state) || entry.state.Position.Y() < float32(core.MinY) {
			engine.passives.removeAt(index)
			continue
		}
		if outsideHomeNeighborhood(entry.home, entry.state.Position) {
			entry.state = previous
		}
		index++
	}
}

// passiveStepInput 决定单个个体本 `tick` 的控制意图，优先级逃跑＞引诱＞闲
// 时看人＞漫游：逃跑剩余时长内沿远离伤害来源方向前进（与夜行者追击同款的
// 世界轴到朝向折算）；吃草事件中只给中性输入（事件个体的位移已由
// `advancePassiveMovement` 冻结，这里是输入层的防御性表态，防未来调用方绕
// 过冻结直调本函数）；否则有引诱目标就转向目标、无目标但有 6 格内闲时目标
// 就面向玩家靠近到 1.5 格止步（止步后仍跟踪朝向）、两者皆无才以世界种子、
// `tick` 与 `id` 确定性派生的朝向漫游。全部输入为纯函数派生，不读全局随机
// 数、不遍历 `map`。
func (engine *engineContext) passiveStepInput(entry *passiveState) physics.Input {
	if entry.fleeTicks > 0 {
		entry.fleeTicks--
		dx := entry.state.Position.X() - entry.fleeFrom.X()
		dz := entry.state.Position.Z() - entry.fleeFrom.Z()
		if dx != 0 || dz != 0 {
			yaw := normalizeYaw(float32(math.Atan2(float64(-dx), float64(-dz))))
			entry.yaw = yaw
			return physics.Input{MoveZ: 1, Yaw: yaw}
		}
	}
	if entry.grazeTicks > 0 {
		return physics.Input{Yaw: entry.yaw}
	}
	if target, ok := engine.passiveTemptTarget(entry); ok {
		dx := target.X() - entry.state.Position.X()
		dz := target.Z() - entry.state.Position.Z()
		want := normalizeYaw(float32(math.Atan2(float64(-dx), float64(-dz))))
		stopSq := float32(passiveTemptStopDistance) * float32(passiveTemptStopDistance)
		if dx*dx+dz*dz > stopSq {
			entry.yaw = turnYawToward(entry.yaw, want, passiveIdleLookMaxTurn)
			return physics.Input{MoveZ: 1, Yaw: entry.yaw}
		}
		// 止步只冻结位移，不冻结朝向：引诱牛（含止步）每 tick 有界转向持麦
		// 玩家，头部朝向与身体一致（头部无 yaw 通道，随身体走）。
		entry.yaw = turnYawToward(entry.yaw, want, passiveIdleLookMaxTurn)
		return physics.Input{Yaw: entry.yaw}
	}
	if target, ok := engine.passiveIdleLookTarget(entry); ok {
		dx := target.X() - entry.state.Position.X()
		dz := target.Z() - entry.state.Position.Z()
		if dx != 0 || dz != 0 {
			want := normalizeYaw(float32(math.Atan2(float64(-dx), float64(-dz))))
			entry.yaw = turnYawToward(entry.yaw, want, passiveIdleLookMaxTurn)
		}
		// 闲时靠近：1.5 格外保持漫游速度贴近，1.5 格内只冻位移不冻朝向。
		if dx*dx+dz*dz > passiveIdleLookStopDistance*passiveIdleLookStopDistance {
			return physics.Input{MoveZ: 1, Yaw: entry.yaw}
		}
		return physics.Input{Yaw: entry.yaw}
	}
	base := splitmix64(uint64(engine.seed) ^ engine.tick.Load() ^ entry.id)
	yaw := normalizeYaw(float32(base&0xFFFFFF) * (2 * math.Pi / 0x1000000))
	entry.yaw = yaw
	return physics.Input{MoveZ: 1, Yaw: yaw}
}

// passiveIdleLookTarget 在同维 `active` 玩家中找 6 格内的最近者（水平平方域
// 比较，等距并列取会话 `id` 更小者），返回其身体位置。逃跑/吃草/引诱生效时
// 调用方不走到这里，本规则天然让路。
func (engine *engineContext) passiveIdleLookTarget(entry *passiveState) (mgl32.Vec3, bool) {
	radiusSq := float32(passiveIdleLookRadius) * float32(passiveIdleLookRadius)
	var best mgl32.Vec3
	bestSq := radiusSq
	found := false
	for _, id := range engine.sortedActiveSessions() {
		session := engine.sessions[id]
		if session == nil || session.player == nil || session.dimension != entry.dimension {
			continue
		}
		distSq := horizontalDistanceSq(session.player.state.Position, entry.state.Position)
		if distSq > radiusSq {
			continue
		}
		if found && distSq >= bestSq {
			continue
		}
		bestSq = distSq
		best = session.player.state.Position
		found = true
	}
	return best, found
}

// turnYawToward 把当前朝向按最短弧向目标朝向推进一个有界步长：步内可达即
// 直接落位，不瞬移、不绕远。
func turnYawToward(current, want, maxStep float32) float32 {
	delta := normalizeYaw(want - current)
	if delta > maxStep {
		return normalizeYaw(current + maxStep)
	}
	if delta < -maxStep {
		return normalizeYaw(current - maxStep)
	}
	return want
}

// outsideHomeNeighborhood 报告位置是否已离开出生区块邻域：邻域是出生区块外
// 扩一圈的 3×3 区块，超出即按切比雪夫距离大于 1 判定。
func outsideHomeNeighborhood(home core.ChunkPos, position mgl32.Vec3) bool {
	chunk := blockPosOf(position).Chunk()
	dx := int64(chunk.X) - int64(home.X)
	dz := int64(chunk.Z) - int64(home.Z)
	if dx < 0 {
		dx = -dx
	}
	if dz < 0 {
		dz = -dz
	}
	return dx > 1 || dz > 1
}

// PassiveDeaths 返回本 tick 死亡结算移除的被动牛 ID（升序）：调用方只读
// 消费，不得保留到下一次结算之后。
func (engine *engineContext) PassiveDeaths() []uint64 {
	return engine.passiveDeaths
}

// settlePassiveDeaths 结算本 `tick` 生命归零的被动牛：经既有掉落契约在死亡
// 位置所在 `chunk` 环形尝试放置 1 个生牛肉后同 `tick` 移除，绝不留下半移除
// 状态。处理顺序即切片顺序（`id` 升序），掉落放置顺序因此可复现。移除的 ID
// 同步记入当 tick 死亡集合（先清空再记录），供发布侧投影 despawn 原因位。
func (engine *engineContext) settlePassiveDeaths(pending *pendingChunkChanges) {
	engine.passiveDeaths = engine.passiveDeaths[:0]
	for index := 0; index < len(engine.passives.entries); {
		entry := &engine.passives.entries[index]
		if entry.health != 0 {
			index++
			continue
		}
		engine.passiveDeaths = append(engine.passiveDeaths, entry.id)
		engine.dropPassiveLoot(entry, pending)
		engine.passives.removeAt(index)
	}
}

// dropPassiveLoot 在死亡位置所在 `chunk` 环形尝试放置 1 个生牛肉：候选
// `chunk` 按 `deathDropChunks` 的既定全序逐个预演，首个有容量的 `chunk` 承接
// 掉落；全部已加载可用 `chunk` 均满时确定性省略掉落，死亡仍由调用方完成。
func (engine *engineContext) dropPassiveLoot(
	entry *passiveState,
	pending *pendingChunkChanges,
) {
	dimension := engine.dimension(entry.dimension)
	if dimension == nil {
		return
	}
	death := blockPosOf(entry.state.Position)
	batch := [1]core.ItemStack{{Item: core.ItemRawBeef, Count: 1}}
	for _, key := range engine.deathDropChunks(entry.dimension, death.Chunk()) {
		chunk, ready := dimension.ReadyChunk(key.Pos)
		if !ready {
			continue
		}
		blockIndex, indexed := world.ChunkBlockIndex(clampBlockToChunk(death, key.Pos))
		if !indexed {
			continue
		}
		next, ok := chunk.PrepareDropBatch(
			batch[:], blockIndex, engine.tunables.DropPickupDelayTicks,
		)
		if !ok {
			continue
		}
		chunk.CommitDropBatch(next)
		engine.touchChunk(key, pending)
		return
	}
}
