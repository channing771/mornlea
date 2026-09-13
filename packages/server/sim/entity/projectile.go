package entity

import (
	"math"
	"sort"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// 弹种编号与协议 v43 的 kind 字节同值（值域 {0,1}）：0 是骨刺（远程敌怪发射），
// 1 是箭（玩家弓发射）。协议层是 wire 值域的权威定义点，引擎侧按同值消费；
// 新增弹种必须同步扩展两侧值域并升版协议。
const (
	projectileKindShard uint8 = 0
	projectileKindArrow uint8 = 1
)

// 投射物的固定数值契约。全部值由边界测试锁定，不随玩家数或弹数放大；初速、
// 重力与寿命是规格写死的数字契约，调整即行为变更。
const (
	// maxProjectiles 是全服同时在飞的投射物数量上限。
	maxProjectiles = 128
	// projectileGravity 是弹道重力（格/秒²）。
	projectileGravity = float32(18)
	// 箭的两档初速（格/秒）：拉弓 6..19 tick 短档、≥20 tick 满档。
	projectileArrowShortSpeed = float32(16)
	projectileArrowFullSpeed  = float32(30)
	// projectileShardSpeed 是骨刺初速（格/秒）。
	projectileShardSpeed = float32(22)
	// 命中伤害（护甲减免前的原始值）：骨刺 3（只命中玩家）；箭短档 2、满档 5。
	projectileShardDamage      = int32(3)
	projectileArrowShortDamage = int32(2)
	projectileArrowFullDamage  = int32(5)
	// projectileMaxLifetimeTicks 是寿命上限（tick）：投射物恰好完成 100 个存活
	// tick 的推进步后，在第 101 个存活 tick 的阶段进入点消失。
	projectileMaxLifetimeTicks uint16 = 100
	// projectileMaxRehashes 是生成 ID 冲突时的重散列预算；耗尽仍冲突则本次
	// 生成整体放弃，绝不覆盖或截断既有集合。
	projectileMaxRehashes = 64
)

// projectileState 是一条在飞投射物的权威身体事实。投射物是服务端权威的瞬态
// 实体：不进快照或存档（重启后在飞投射物消失），不由客户端预测，客户端只持
// 有按服务器消息演进的镜像。
//
// `owner` 是发射者身份：箭为持有者会话 ID、骨刺为发射敌怪 ID——命中扫描据
// 此排除发射者本人。`damage` 是命中结算的原始伤害（玩家目标在结算点再经护甲
// 减免折算）。`age` 是已完成的推进步数，与寿命上限共同决定消失时点。
//
// 集合形态刻意与 `hostileSet` 同形：全服至多 128 条，按 ID 严格升序的定容切片
// 配合二分查找已是最廉价的确定性结构，无 map、无增长式分配。
type projectileState struct {
	id        uint64
	kind      uint8
	dimension core.DimensionID
	owner     uint64
	damage    int32
	age       uint16
	position  mgl32.Vec3
	velocity  mgl32.Vec3
}

// projectileSet 是按 ID 严格升序维护的投射物集合。切片容量在构造时按上限
// 预分配，热路径上不产生任何增长式分配。
type projectileSet struct {
	entries []projectileState
}

func newProjectileSet() projectileSet {
	return projectileSet{entries: make([]projectileState, 0, maxProjectiles)}
}

// findIndex 二分查找 ID，返回命中下标；未命中返回 -1。
func (set *projectileSet) findIndex(id uint64) int {
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

// insert 以二分定位把新投射物插入升序位置。重复 ID 或超出容量时拒绝并返回
// false——容量路径由 `spawnProjectile` 的腾位流程独占，直接插入绝不覆盖既有
// 个体。
func (set *projectileSet) insert(entry projectileState) bool {
	if len(set.entries) >= maxProjectiles {
		return false
	}
	index := sort.Search(len(set.entries), func(i int) bool {
		return set.entries[i].id >= entry.id
	})
	if index < len(set.entries) && set.entries[index].id == entry.id {
		return false
	}
	set.entries = append(set.entries, projectileState{})
	copy(set.entries[index+1:], set.entries[index:])
	set.entries[index] = entry
	return true
}

// removeAt 删除指定下标的个体并保持其余个体的相对顺序（升序）不变。
func (set *projectileSet) removeAt(index int) {
	copy(set.entries[index:], set.entries[index+1:])
	set.entries = set.entries[:len(set.entries)-1]
}

// spawnProjectile 把一条新投射物并入权威集合并返回派生 ID。ID 由
// (worldSeed, tick, 维度, 弹种, 发射者, 出生位置) 经确定性哈希派生，非零且与
// 既有个体不冲突；集合已满时移除 ID 最小（最旧）的一条腾位——被挤出个体的
// despawn 由发布侧的「镜像有而截面无」差异判据照常发出。弹种、维度、伤害与
// 出生事实任一不合法都整体拒绝，绝不留下半生成的个体。
func (engine *engineContext) spawnProjectile(
	kind uint8,
	dimension core.DimensionID,
	position mgl32.Vec3,
	velocity mgl32.Vec3,
	owner uint64,
	damage int32,
) (uint64, bool) {
	if !validProjectileKind(kind) || damage <= 0 {
		return 0, false
	}
	if engine.dimension(dimension) == nil {
		return 0, false
	}
	if !projectileFiniteVec(position) || !projectileFiniteVec(velocity) {
		return 0, false
	}
	id := engine.deriveProjectileID(dimension, kind, owner, position)
	if id == 0 {
		return 0, false
	}
	if len(engine.projectiles.entries) >= maxProjectiles {
		// 集合满：ID 严格升序保证首条即 ID 最小（最旧）的一条。
		engine.projectiles.removeAt(0)
	}
	if !engine.projectiles.insert(projectileState{
		id:        id,
		kind:      kind,
		dimension: dimension,
		owner:     owner,
		damage:    damage,
		position:  position,
		velocity:  velocity,
	}) {
		return 0, false
	}
	return id, true
}

// validProjectileKind 报告 kind 是否落在弹种值域 {0,1} 内。
func validProjectileKind(kind uint8) bool {
	return kind == projectileKindShard || kind == projectileKindArrow
}

// projectileFiniteVec 报告向量三个分量是否全部有限：出生事实的防御校验，
// 非有限位置/速度的投射物没有可判定的弹道。
func projectileFiniteVec(value mgl32.Vec3) bool {
	for axis := range 3 {
		component := float64(value[axis])
		if math.IsNaN(component) || math.IsInf(component, 0) {
			return false
		}
	}
	return true
}

// deriveProjectileID 由 (worldSeed, tick, 维度, 弹种, 发射者, 出生位置) 派生
// 非零确定性 ID：基准哈希经 `sampler.ProjectileSpawnHash` 求得，与既有个体
// 冲突时沿 `SplitMix64` 哈希链重散列（与敌怪 ID 链同形），预算耗尽返回 0。
// 无进程级随机源，相同输入的重放逐位一致。
func (engine *engineContext) deriveProjectileID(
	dimension core.DimensionID,
	kind uint8,
	owner uint64,
	position mgl32.Vec3,
) uint64 {
	id := sampler.ProjectileSpawnHash(
		engine.seed, engine.tick.Load(), dimension, kind, owner,
		[3]float32{position.X(), position.Y(), position.Z()},
	)
	for attempt := 0; attempt < projectileMaxRehashes; attempt++ {
		if id != 0 && engine.projectiles.findIndex(id) < 0 {
			return id
		}
		id = sampler.SplitMix64(id)
	}
	return 0
}

// advanceProjectiles 推进全部在飞投射物恰好一步：先施加重力再按位移积分
// （f32 与玩家/敌怪物理同语义，`physics.FixedDeltaSeconds` 同源步长），在同一
// 「上一位置→新位置」线段上解析方块与实体命中并取最先者，未命中才按新位置
// 继续飞行并做世界边界与订阅区两项消失判定。寿命耗尽在阶段进入点移除。处理
// 顺序即切片顺序（ID 升序），命中结算与移除因此可复现。
//
// 每 tick 成本有界：至多 128 条 ×（1 次线段 DDA + 玩家 ≤8 + 敌怪 ≤64 + 被动
// ≤32 的 AABB 扫描），全部 O(常数)，无 map 遍历、无阻塞 I/O。命中实体产生的
// 伤害只经既有伤害入口结算；弹击致死由本阶段之后的死亡结算统一完成——任何
// 0 血实体不会存活到下一权威 tick。
func (engine *engineContext) advanceProjectiles(result *TickResult) {
	for index := 0; index < len(engine.projectiles.entries); {
		entry := &engine.projectiles.entries[index]
		// 寿命耗尽：第 101 个存活 tick 的阶段进入点消失，不再推进一步。
		if entry.age >= projectileMaxLifetimeTicks {
			engine.projectiles.removeAt(index)
			continue
		}
		dimension := engine.dimension(entry.dimension)
		if dimension == nil {
			engine.projectiles.removeAt(index)
			continue
		}
		entry.velocity = entry.velocity.Sub(mgl32.Vec3{
			0, projectileGravity * physics.FixedDeltaSeconds, 0,
		})
		previous := entry.position
		delta := entry.velocity.Mul(physics.FixedDeltaSeconds)
		if engine.settleProjectileSegment(result, entry, dimension, previous, delta) {
			engine.projectiles.removeAt(index)
			continue
		}
		entry.position = previous.Add(delta)
		entry.age++
		// 越出世界高度或离开全部会话订阅区：消失（订阅区判定与发布可见性
		// 同构——没有任何会话订阅所在 chunk 的投射物不再对任何人可见）。
		next := entry.position
		if next.Y() < float32(core.MinY) || next.Y() >= float32(core.MaxY) ||
			engine.projectileChunkUnsubscribed(entry.dimension, next) {
			engine.projectiles.removeAt(index)
			continue
		}
		index++
	}
}

// settleProjectileSegment 在「上一位置→新位置」线段上解析命中并结算，发生
// 命中（方块或实体）返回 true，调用方据此移除投射物。方块与实体取参数 t
// 最先者；恰好同时命中时方块优先（先世界后实体的确定性平局规则）。
func (engine *engineContext) settleProjectileSegment(
	result *TickResult,
	entry *projectileState,
	dimension *Dimension,
	previous, delta mgl32.Vec3,
) bool {
	blockT, blockHit := projectileBlockHitT(dimension, previous, delta)
	targetKind, targetID, entityT, entityHit := engine.projectileEntityCandidate(entry, previous, delta)
	if entityHit && (!blockHit || entityT < blockT) {
		engine.settleProjectileEntityHit(result, entry, targetKind, targetID)
		return true
	}
	// 方块命中：投射物停在命中点同 tick 消失，不掉落任何物品。
	return blockHit
}

// projectileBlockHitT 把线段交给既有生产射线出口 `core.RaycastBlocks`
// （`blockRaycastSampler` 的 `core.InteractionTarget` 谓词：空气与流体不是
// 目标），返回首个命中方块在线段上的参数 t（0..1）。线段过短或射线出口报错
// （如区块未加载）时按未命中处理——宁可让投射物飞过未加载的世界状态，也不
// 凭缺失数据判定消失。
func projectileBlockHitT(dimension *Dimension, previous, delta mgl32.Vec3) (float32, bool) {
	length := delta.Len()
	if length < 1e-6 {
		return 0, false
	}
	hit, blocked, err := core.RaycastBlocks(
		previous, delta, length, blockRaycastSampler(dimension),
	)
	if err != nil || !blocked {
		return 0, false
	}
	return hit.Distance / length, true
}

// projectileEntityCandidate 在同一线段上扫描候选实体 AABB（与近战同一
// `physics.PlayerBounds` 身体边界），返回参数 t 最小的候选。t 相同时取目标
// 种类与 ID 更小者（与近战命中同族的确定性平局规则），结果与遍历顺序无关。
func (engine *engineContext) projectileEntityCandidate(
	entry *projectileState,
	previous, delta mgl32.Vec3,
) (core.CombatTargetKind, uint64, float32, bool) {
	bestT := float32(math.MaxFloat32)
	var bestKind core.CombatTargetKind
	var bestID uint64
	found := false
	consider := func(kind core.CombatTargetKind, id uint64, position mgl32.Vec3) {
		if !projectileTargetAllowed(entry, kind, id) {
			return
		}
		t, hit := segmentAABBEntryT(previous, delta, physics.PlayerBounds(position))
		if !hit {
			return
		}
		if !found || t < bestT ||
			t == bestT && (kind < bestKind || kind == bestKind && id < bestID) {
			found, bestT, bestKind, bestID = true, t, kind, id
		}
	}
	for index := range engine.hostiles.entries {
		candidate := &engine.hostiles.entries[index]
		if candidate.dimension != entry.dimension || candidate.health == 0 {
			continue
		}
		consider(core.CombatTargetHostile, candidate.id, candidate.state.Position)
	}
	for _, id := range engine.sortedActiveSessions() {
		session := engine.sessions[id]
		if session == nil || session.player == nil ||
			session.dimension != entry.dimension || session.player.health == 0 {
			continue
		}
		consider(core.CombatTargetPlayer, uint64(id), session.player.state.Position)
	}
	for index := range engine.passives.entries {
		candidate := &engine.passives.entries[index]
		if candidate.dimension != entry.dimension || candidate.health == 0 {
			continue
		}
		consider(core.CombatTargetPassive, candidate.id, candidate.state.Position)
	}
	return bestKind, bestID, bestT, found
}

// projectileTargetAllowed 按弹种过滤合法目标：骨刺只命中玩家；箭命中其他
// 玩家、敌怪与被动牛；两类弹种都不命中发射者本人（箭的持有者会话、骨刺的
// 发射敌怪——骨刺本就只扫玩家，此条是身份层面而非弹种层面的显式护栏）。
func projectileTargetAllowed(
	entry *projectileState,
	kind core.CombatTargetKind,
	id uint64,
) bool {
	if entry.kind == projectileKindArrow && kind == core.CombatTargetPlayer && id == entry.owner {
		return false
	}
	if entry.kind == projectileKindShard && kind == core.CombatTargetHostile && id == entry.owner {
		return false
	}
	switch entry.kind {
	case projectileKindShard:
		return kind == core.CombatTargetPlayer
	case projectileKindArrow:
		return kind == core.CombatTargetPlayer || kind == core.CombatTargetHostile ||
			kind == core.CombatTargetPassive
	default:
		return false
	}
}

// settleProjectileEntityHit 把一次实体命中结算进既有伤害面，不新增任何伤害
// 路径：
//   - 玩家目标：命中时点冻结的护甲点数经 `core.ReducedDamage` 折算有效伤害，
//     实际产生减免时全部参与件各耗 1 点耐久，再经 `applyDamage` 唯一入口结算
//     并沿弹速水平分量击退（与近战同一冲量常数）；
//   - 敌怪/被动目标：分别经敌怪受伤入口与被动受伤入口结算并施加击退；
//   - 玩家所有的箭命中实体时，向持有者会话追加既有近战命中私有确认
//     （`CombatHit`）；骨刺的发射者是敌怪，不产生确认。
func (engine *engineContext) settleProjectileEntityHit(
	result *TickResult,
	entry *projectileState,
	kind core.CombatTargetKind,
	id uint64,
) {
	settled := false
	switch kind {
	case core.CombatTargetPlayer:
		session := engine.sessions[SessionID(id)]
		if session == nil || session.player == nil || session.player.lifecycle != PlayerActive {
			return
		}
		player := session.player
		// 冻结时点即结算点：减免按本次命中读到的点数计算，随后才耗耐久，
		// 同一次命中不受耐久扣减自身的回写影响。
		points := core.ArmorPoints(player.armor)
		effective := core.ReducedDamage(entry.damage, points)
		player.applyDamage(effective)
		if points > 0 && effective < entry.damage {
			consumeArmorDurability(&player.armor)
		}
		player.state.Velocity = player.state.Velocity.Add(projectileKnockback(entry.velocity))
		settled = true
	case core.CombatTargetHostile:
		index := engine.hostiles.findIndex(id)
		if index < 0 {
			return
		}
		hostile := &engine.hostiles.entries[index]
		hostile.applyDamage(entry.damage)
		hostile.state.Velocity = hostile.state.Velocity.Add(projectileKnockback(entry.velocity))
		settled = true
	case core.CombatTargetPassive:
		// 被动牛经受击入口结算：扣血与固定时长逃跑同一次落定，`from` 取命中
		// 时刻的弹体位置（发射方向的自然来源）。
		if !engine.DamagePassive(id, entry.damage, entry.position) {
			return
		}
		if index := engine.passives.findIndex(id); index >= 0 {
			passive := &engine.passives.entries[index]
			passive.state.Velocity = passive.state.Velocity.Add(projectileKnockback(entry.velocity))
		}
		settled = true
	}
	if settled && entry.kind == projectileKindArrow {
		result.CombatHits = append(result.CombatHits, CombatHit{
			Session: SessionID(entry.owner), Damage: uint8(entry.damage), TargetKind: kind,
		})
	}
}

// projectileKnockback 沿弹速水平分量施加击退：与近战同一冲量常数，方向取
// 弹速的水平单位向量。竖直弹道（零水平分量）不产生水平击退——弹体没有朝向
// 可回退，与近战的 yaw 回退语义刻意不同。
func projectileKnockback(velocity mgl32.Vec3) mgl32.Vec3 {
	delta := mgl32.Vec3{velocity.X(), 0, velocity.Z()}
	if delta.LenSqr() == 0 {
		return delta
	}
	return delta.Normalize().Mul(combatKnockbackSpeed)
}

// segmentAABBEntryT 返回线段 origin→origin+delta 最早进入 bounds 的参数 t
// （0..1，0 表示起点已在盒内）；不相交返回 false。结构与近战的
// `rayAABBDistance` 同族：轴平行时起点落在该轴盒外即不可能命中，相交区间
// 裁剪到线段参数域 [0,1]。
func segmentAABBEntryT(origin, delta mgl32.Vec3, bounds core.AABB) (float32, bool) {
	near, far := float32(0), float32(1)
	for axis := range 3 {
		if math.Abs(float64(delta[axis])) < 1e-6 {
			if origin[axis] < bounds.Min[axis] || origin[axis] > bounds.Max[axis] {
				return 0, false
			}
			continue
		}
		entry := (bounds.Min[axis] - origin[axis]) / delta[axis]
		exit := (bounds.Max[axis] - origin[axis]) / delta[axis]
		if entry > exit {
			entry, exit = exit, entry
		}
		near = max(near, entry)
		far = min(far, exit)
		if near > far {
			return 0, false
		}
	}
	return near, true
}

// projectileChunkUnsubscribed 报告是否没有任何会话订阅投射物所在 chunk。
// 判定与 runtime 订阅集合同构：逐会话取「已就绪、同维度、生效半径的方形订阅
// 区」；中心与半径都来自同一份只读 view 快照（runtime 侧由
// `SessionSubscription` 派生并按引擎视界钳制），实体层不持第二份订阅事实。
// 判定是存在性折叠，结果与遍历顺序无关。
func (engine *engineContext) projectileChunkUnsubscribed(
	dimension core.DimensionID,
	position mgl32.Vec3,
) bool {
	key := blockPosOf(position).Chunk()
	if engine.views.singleValid &&
		engine.projectileSessionSubscribes(engine.views.single, dimension, key) {
		return false
	}
	for index := range engine.views.entries {
		if engine.projectileSessionSubscribes(engine.views.entries[index], dimension, key) {
			return false
		}
	}
	return true
}

// projectileSessionSubscribes 报告单个会话视图是否覆盖该 chunk。
func (engine *engineContext) projectileSessionSubscribes(
	entry TickSessionView,
	dimension core.DimensionID,
	key core.ChunkPos,
) bool {
	if !entry.View.Ready {
		return false
	}
	session := engine.sessions[entry.Session]
	if session == nil || session.dimension != dimension {
		return false
	}
	return absChunkDelta(key.X, entry.View.Center.X) <= int64(entry.View.Radius) &&
		absChunkDelta(key.Z, entry.View.Center.Z) <= int64(entry.View.Radius)
}

// projectilesSnapshot 返回按 ID 升序的全量在飞投射物值快照，供发布侧组装
// 按会话订阅的 spawn/state 批次。
func (engine *engineContext) projectilesSnapshot() []contract.ProjectileSnapshot {
	snapshots := make([]contract.ProjectileSnapshot, 0, len(engine.projectiles.entries))
	for index := range engine.projectiles.entries {
		entry := &engine.projectiles.entries[index]
		snapshots = append(snapshots, contract.ProjectileSnapshot{
			ID:        entry.id,
			Kind:      entry.kind,
			Dimension: entry.dimension,
			Position:  entry.position,
			Velocity:  entry.velocity,
		})
	}
	return snapshots
}
