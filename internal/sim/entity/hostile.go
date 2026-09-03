package entity

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/world"
)

// 夜行者的固定数值契约。这些值由边界测试锁定，不随玩家数或夜行者数放大；
// 与 `internal/storage` 夜行者存档 codec 中的周期/累计上限同源，任何一侧单独
// 调整都必须同步另一侧并更新 golden。
const (
	// maxHostiles 是全服同时存在的夜行者数量上限。
	maxHostiles = 64
	// hostileCooldownPeriodTicks 是攻击/受击/灼烧三个计时器共享的周期长度。
	hostileCooldownPeriodTicks uint8 = 20
	// maxHostileDistantTicks 是远离全部 active 玩家的 despawn 累计 tick 上限。
	maxHostileDistantTicks uint16 = 600
)

// hostileState 是一只夜行者的权威身体事实。字段面与 `internal/storage` 的
// `StoredHostileMob` 记录同构（转换由 server 装配层完成，`sim` 不得依赖
// storage），另有两项运行时派生物：`input` 是本 tick 的控制意图（夜间追击
// 接入前恒为仅保留 yaw 的中性输入），`fresh` 标记本 tick 刚生成的个体——
// 生成次序在物理之前，但新个体下一 tick 才参与积分。
//
// 集合形态刻意不建 map 也不建 ECS：全服至多 64 只，按 ID 严格升序的定容
// 切片配合二分查找已是最廉价的确定性结构。
type hostileState struct {
	state physics.State
	input physics.Input
	id    uint64
	// dimension 恒为夜行者所在维度；恢复与生成都校验维度存在。
	dimension core.DimensionID
	yaw       float32
	// health 合法区间 1..core.MaxHealth；归零的个体在同一权威 tick 内走死亡
	// 结算（移除 + 掉落）后不会留存。
	health uint8
	// attackCooldown/hurtCooldown 是攻击与受击保护的剩余 tick；burnCooldown
	// 是灼烧周期的剩余 tick（露天白昼逐 tick 递减，归零结算 1 点伤害并回到
	// 满周期；遮顶或夜间重置回满周期）。
	attackCooldown uint8
	hurtCooldown   uint8
	burnCooldown   uint8
	// hasTarget 与 targetPlayer 成对表达追逐目标；无目标时 `targetPlayer`
	// 必须为零值，与存档记录的成对约束一致。目标选择由 server 编排层做出并经
	// `PlanHostileChase` 写回权威事实；`nextRepathTicks` 是持久化世界时间轴上
	// 的下一次重规划 tick，重规划节奏因此跨重启保留。
	hasTarget       bool
	targetPlayer    core.PlayerID
	nextRepathTicks uint64
	// attackIntent 与 attackTargetSession 是本 tick 已冻结的攻击意图（瞬态，
	// 不进快照或存档）：由 applyHostileActions 在 tick 边界统一写定，由
	// advanceCombat 在同 tick 稍后统一结算并清零。
	attackIntent        bool
	attackTargetSession SessionID
	// distantTicks 是距全部 active 玩家水平超过 despawn 半径的累计 tick，
	// 回到范围内即清零。
	distantTicks uint16
	fresh        bool
}

// hostileSet 是按 ID 严格升序维护的夜行者集合。切片容量在构造时按上限
// 预分配，热路径上不产生任何增长式分配。
type hostileSet struct {
	entries []hostileState
}

func newHostileSet() hostileSet {
	return hostileSet{entries: make([]hostileState, 0, maxHostiles)}
}

// findIndex 二分查找 ID，返回命中下标；未命中返回 -1。
func (set *hostileSet) findIndex(id uint64) int {
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

// insert 以二分定位把新个体插入升序位置。重复 ID 或超出容量时拒绝并返回
// false——拒绝是生成与恢复两条入口共享的边界，绝不截断或覆盖既有个体。
func (set *hostileSet) insert(entry hostileState) bool {
	if len(set.entries) >= maxHostiles {
		return false
	}
	index := sort.Search(len(set.entries), func(i int) bool {
		return set.entries[i].id >= entry.id
	})
	if index < len(set.entries) && set.entries[index].id == entry.id {
		return false
	}
	set.entries = append(set.entries, hostileState{})
	copy(set.entries[index+1:], set.entries[index:])
	set.entries[index] = entry
	return true
}

// removeAt 删除指定下标的个体并保持其余个体的相对顺序（升序）不变。
func (set *hostileSet) removeAt(index int) {
	copy(set.entries[index:], set.entries[index+1:])
	set.entries = set.entries[:len(set.entries)-1]
}

// RestoreHostile 把一条持久化记录恢复为权威身体。校验与存档侧的记录矩阵
// 对齐：ID 非零且不重复、维度存在、身体状态有限且位置在世界高度内、生命
// 为正且不超上限、三个冷却不越过周期、远离累计不越界、目标成对一致。任何
// 一条不成立都整体拒绝且不改变既有集合，由调用方决定启动是否失败。
func (engine *engineContext) RestoreHostile(mob HostileMob) error {
	if err := validateHostileMob(mob); err != nil {
		return err
	}
	if engine.dimension(mob.Dimension) == nil {
		return fmt.Errorf("sim: hostile dimension %d is unknown", mob.Dimension)
	}
	if engine.hostiles.findIndex(mob.ID) >= 0 {
		return fmt.Errorf("sim: duplicate hostile ID %d", mob.ID)
	}
	entry := hostileState{
		state:           mob.State,
		input:           physics.Input{Yaw: mob.Yaw},
		id:              mob.ID,
		dimension:       mob.Dimension,
		yaw:             mob.Yaw,
		health:          mob.Health,
		attackCooldown:  mob.AttackCooldown,
		hurtCooldown:    mob.HurtCooldown,
		burnCooldown:    mob.BurnCooldown,
		hasTarget:       mob.HasTarget,
		targetPlayer:    mob.PlayerID,
		nextRepathTicks: mob.NextRepathTicks,
		distantTicks:    mob.DistantTicks,
	}
	if !engine.hostiles.insert(entry) {
		return fmt.Errorf("sim: hostile capacity %d exhausted", maxHostiles)
	}
	return nil
}

// validateHostileMob 校验单条夜行者记录的全部不变量，判据与
// `internal/storage` 的记录校验矩阵一致：两个包靠同值常量与对齐的用例
// 保持双向边界一致，`sim` 不得依赖 storage。
func validateHostileMob(mob HostileMob) error {
	if mob.ID == 0 {
		return errors.New("sim: hostile ID is zero")
	}
	if !physics.ValidState(mob.State) {
		return errors.New("sim: hostile state is not finite")
	}
	if mob.State.Position.Y() < float32(core.MinY) || mob.State.Position.Y() >= float32(core.MaxY) {
		return fmt.Errorf("sim: hostile position Y %v outside world", mob.State.Position.Y())
	}
	if mob.Health == 0 || mob.Health > core.MaxHealth {
		return fmt.Errorf("sim: hostile health %d outside 1..%d", mob.Health, core.MaxHealth)
	}
	// 固定顺序枚举三个冷却，保证同一输入的拒绝文案可复现。
	for _, cooldown := range []struct {
		name  string
		value uint8
	}{
		{"attack", mob.AttackCooldown},
		{"hurt", mob.HurtCooldown},
		{"burn", mob.BurnCooldown},
	} {
		if cooldown.value > hostileCooldownPeriodTicks {
			return fmt.Errorf("sim: hostile %s cooldown %d exceeds period %d",
				cooldown.name, cooldown.value, hostileCooldownPeriodTicks)
		}
	}
	if mob.DistantTicks > maxHostileDistantTicks {
		return fmt.Errorf("sim: hostile distant ticks %d exceeds limit %d",
			mob.DistantTicks, maxHostileDistantTicks)
	}
	if !mob.HasTarget {
		if mob.PlayerID != (core.PlayerID{}) {
			return errors.New("sim: hostile without target keeps player ID")
		}
		return nil
	}
	if !mob.PlayerID.Valid() {
		return errors.New("sim: hostile target is not a valid UUIDv4")
	}
	return nil
}

// HostileMobs 返回按 ID 升序的全量值快照。调用频率由保存与发布节奏决定，
// 不在权威 tick 热路径上，因此分配一份新切片。
func (engine *engineContext) HostileMobs() []HostileMob {
	mobs := make([]HostileMob, 0, len(engine.hostiles.entries))
	for index := range engine.hostiles.entries {
		mobs = append(mobs, engine.hostileMobAt(index))
	}
	return mobs
}

func (engine *engineContext) hostileMobAt(index int) HostileMob {
	entry := &engine.hostiles.entries[index]
	return HostileMob{
		ID:              entry.id,
		Dimension:       entry.dimension,
		State:           entry.state,
		Yaw:             entry.yaw,
		Health:          entry.health,
		AttackCooldown:  entry.attackCooldown,
		HurtCooldown:    entry.hurtCooldown,
		BurnCooldown:    entry.burnCooldown,
		HasTarget:       entry.hasTarget,
		PlayerID:        entry.targetPlayer,
		NextRepathTicks: entry.nextRepathTicks,
		DistantTicks:    entry.distantTicks,
	}
}

func (hostile *hostileState) applyDamage(damage int32) {
	if damage <= 0 {
		return
	}
	if damage >= int32(hostile.health) {
		hostile.health = 0
		return
	}
	hostile.health -= uint8(damage)
}

// advanceHostileMovement 把全部夜行者汇入与玩家/伙伴相同的 Rust
// `physics.Step` 积分出口：每个个体用既有输入步进恰好一次，位移完全由权威
// 物理决定，不新写任何 Go 积分。身体与玩家同形（同 AABB），浸没标志复用
// `physics.SubmersionFlags` 这唯一一份浸没规则，积分参数与玩家/伙伴共用同
// 一份 physics tunables 快照。处理顺序即切片顺序（ID 升序）。
//
// 本 tick 刚生成的个体（`fresh`）跳过并清除标记：生成判定发生在 tick 边界、
// 先于物理，但新生个体的第一次位移从下一 tick 开始。坠出世界或状态失真的
// 个体没有玩家那样的重生路径，按确定性移除处理（不掉落、不保留半移除状
// 态），避免留下无法持久化或积分发散的身体。移除阈值取世界下界本体
// （`Y < core.MinY` 即移除）：坠落个体会滞留，世界下界以下的任何位置都过
// 不了存档记录校验，容许 `[MinY-16, MinY)` 滞留窗只会把不可持久化的位置
// 写进存档、令重启恢复整体失败。
func (engine *engineContext) advanceHostileMovement() {
	for index := 0; index < len(engine.hostiles.entries); {
		entry := &engine.hostiles.entries[index]
		if entry.fresh {
			entry.fresh = false
			index++
			continue
		}
		source := dimensionCollisionSource{dimension: engine.dimension(entry.dimension)}
		input := entry.input
		input.BodyInFluid, input.EyeInFluid = physics.SubmersionFlagsWithTunables(
			entry.state.Position, source, engine.physicsTunables,
		)
		entry.state = physics.StepWithTunables(
			entry.state, input, source, engine.physicsTunables,
		).State
		if !physics.ValidState(entry.state) || entry.state.Position.Y() < float32(core.MinY) {
			engine.hostiles.removeAt(index)
			continue
		}
		index++
	}
}

// PlanHostileChase 把一次有界追逐的目标选择与重规划节奏写回权威事实：目标成
// 对约束与存档记录一致（无目标时玩家 ID 必须为零、有目标时必须是合法
// UUIDv4），`nextRepathTicks` 是持久化世界时间轴上的下一次重规划 tick。唯一
// 调用方是持有 `stepMu` 的 server 编排层（tick 边界单写者）；未知 ID 或成对
// 约束不成立时整体拒绝并返回 false，绝不留下半更新的追逐事实。
func (engine *engineContext) PlanHostileChase(
	id uint64,
	hasTarget bool,
	target core.PlayerID,
	nextRepathTicks uint64,
) bool {
	index := engine.hostiles.findIndex(id)
	if index < 0 {
		return false
	}
	if !hasTarget {
		if target != (core.PlayerID{}) {
			return false
		}
	} else if !target.Valid() {
		return false
	}
	entry := &engine.hostiles.entries[index]
	entry.hasTarget = hasTarget
	entry.targetPlayer = target
	entry.nextRepathTicks = nextRepathTicks
	return true
}

// advanceHostiles 只推进生成、意图消费和移动；统一战斗、灼烧、死亡与远离消失
// 由调用方在 hostile 阶段紧随其后按固定生命周期顺序编排。
func (engine *engineContext) advanceHostiles(actions []HostileAction) {
	engine.advanceHostileSpawn()
	engine.applyHostileActions(actions)
	engine.advanceHostileMovement()
}

// phaseIsDay 报告显示相位是否为白昼：与客户端昼夜曲线（sun = sin(2πp/24000)，
// 见 internal/render 的 `DayNightAt`）一致，太阳在地平线上（sin>0）即相位
// 1..11999。灼烧与夜间生成共用 `core.DisplayDayPhase` 这一唯一时间源。
func phaseIsDay(phase uint16) bool {
	return phase >= 1 && phase <= 11999
}

// advanceHostileBurn 推进白昼露天灼烧：显示相位为白昼且身体上空依次无
// `core.BlockOpaque` 方块（露天含天空光判据）时，灼烧剩余计时逐 tick 递减，
// 归零即扣 1 点生命并回到满周期；遮顶或夜间一律把计时重置回满周期（等价于
// 「已灼烧的累计从 0 重新开始」）。生命归零的个体由本 tick 稍后的
// settleHostileDeaths 统一移除与掉落。
func (engine *engineContext) advanceHostileBurn(worldTime uint64) {
	phase := core.DisplayDayPhase(worldTime, engine.DayPhaseOffset())
	if !phaseIsDay(phase) {
		engine.resetHostileBurnTimers()
		return
	}
	for index := range engine.hostiles.entries {
		entry := &engine.hostiles.entries[index]
		dimension := engine.dimension(entry.dimension)
		if !engine.hostileSkyExposed(dimension, entry.state.Position) {
			entry.burnCooldown = hostileCooldownPeriodTicks
			continue
		}
		if entry.burnCooldown > 0 {
			entry.burnCooldown--
		}
		if entry.burnCooldown == 0 {
			entry.burnCooldown = hostileCooldownPeriodTicks
			if entry.health > 0 {
				entry.health--
			}
		}
	}
}

func (engine *engineContext) resetHostileBurnTimers() {
	for index := range engine.hostiles.entries {
		engine.hostiles.entries[index].burnCooldown = hostileCooldownPeriodTicks
	}
}

// hostileSkyExposed 报告身体上空是否直通世界顶：从身体上缘所在格起向上逐格
// 检查，任何 `core.BlockOpaque` 方块（玻璃/树叶等透明方块不算遮挡）都判为
// 遮顶。上空进入未加载区块时按「不露天」保守处理——灼烧宁可漏判也不能凭
// 借未加载的世界状态扣血。
func (engine *engineContext) hostileSkyExposed(dimension *Dimension, position mgl32.Vec3) bool {
	if dimension == nil {
		return false
	}
	column := blockPosOf(position)
	for y := int32(math.Floor(float64(position.Y() + physics.PlayerHeight))); y < core.MaxY; y++ {
		block, ready := dimension.BlockAt(core.BlockPos{X: column.X, Y: y, Z: column.Z})
		if !ready {
			return false
		}
		if core.BlockOpaque(block) {
			return false
		}
	}
	return true
}

// advanceHostileDistant 推进远离消失：距全部 active 玩家（同维）水平距离
// >64 格时逐 tick 累计 `DistantTicks`，累计满 600 即移除且不产生掉落；回到
// 范围内（≤64）立即清零累计。没有 active 玩家时「距全部玩家 >64」按空集
// 成立，累计照常推进。
func (engine *engineContext) advanceHostileDistant() {
	distantSq := float32(hostileDistantRadius) * float32(hostileDistantRadius)
	sessions := engine.sortedActiveSessions()
	for index := 0; index < len(engine.hostiles.entries); {
		entry := &engine.hostiles.entries[index]
		within := false
		for _, id := range sessions {
			session := engine.sessions[id]
			if session == nil || session.player == nil || session.dimension != entry.dimension {
				continue
			}
			if horizontalDistanceSq(session.player.state.Position, entry.state.Position) <= distantSq {
				within = true
				break
			}
		}
		if within {
			entry.distantTicks = 0
			index++
			continue
		}
		entry.distantTicks++
		if entry.distantTicks >= maxHostileDistantTicks {
			engine.hostiles.removeAt(index)
			continue
		}
		index++
	}
}

// hostileDistantRadius 是远离消失的水平判界（格）：>64 累计、≤64 清零，
// 边界本身归入范围内。
const hostileDistantRadius = 64

// settleHostileDeaths 结算本 tick 生命归零的夜行者：经既有掉落契约在死亡
// chunk 环形尝试放置 1 个腐肉后同 tick 移除，绝不留下半移除状态。处理顺序
// 即切片顺序（ID 升序），掉落放置顺序因此可复现。
func (engine *engineContext) settleHostileDeaths(pending *pendingChunkChanges) {
	for index := 0; index < len(engine.hostiles.entries); {
		entry := &engine.hostiles.entries[index]
		if entry.health != 0 {
			index++
			continue
		}
		engine.dropHostileLoot(entry, pending)
		engine.hostiles.removeAt(index)
	}
}

// dropHostileLoot 在死亡位置所在 chunk 环形尝试放置 1 个腐肉：候选 chunk 按
// deathDropChunks 的既定全序（环形半径 0、1、2…，同圈稳定排序）逐个预演，
// 首个有容量的 chunk 承接掉落；全部已加载可用 chunk 均满时确定性省略掉落，
// 死亡仍由调用方完成。
func (engine *engineContext) dropHostileLoot(
	entry *hostileState,
	pending *pendingChunkChanges,
) {
	dimension := engine.dimension(entry.dimension)
	if dimension == nil {
		return
	}
	death := blockPosOf(entry.state.Position)
	batch := [1]core.ItemStack{{Item: core.ItemRottenFlesh, Count: 1}}
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

// horizontalDistanceSq 返回两点间的水平距离平方，供半径判定统一使用：
// 比较在平方域进行，避免为每次判定做开方。
func horizontalDistanceSq(from, to mgl32.Vec3) float32 {
	dx := to.X() - from.X()
	dz := to.Z() - from.Z()
	return dx*dx + dz*dz
}

// blockPosOf 返回实体脚底所在的方块坐标（与掉落死亡结算同一定位规则）。
func blockPosOf(position mgl32.Vec3) core.BlockPos {
	return core.BlockPos{
		X: int32(math.Floor(float64(position.X()))),
		Y: int32(math.Floor(float64(position.Y()))),
		Z: int32(math.Floor(float64(position.Z()))),
	}
}
