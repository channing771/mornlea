// 本文件实现 server 侧夜行者有界追逐编排（hostileManager）：目标选择、不可
// 变路径快照的两槽非阻塞派发、结果在 tick 边界按 ID 序应用与过期丢弃、以及
// 经 `sim.HostileAction` 轴量提交的 waypoint 执行。
//
// 并发模型与 companion manager 同构（权威 tick 是唯一写者）：
//   - slots/mobs/targets 只在持有 stepMu 的 tick 路径读写（Server.step 的
//     advanceHostileChase 调用点，先于 engine.Step）；
//   - worker goroutine 只持有不可变值（`pathfind.PathGrid`），经容量 2 的
//     results channel 回送结果；semaphore（容量 2，即「两槽」）约束在途 A*
//     数，槽位在 FindPath 返回后、结果发送前显式释放；
//   - 权威 tick 对 results 只做非阻塞排空，绝不等待 A*；派发侧的 semaphore
//     获取必须是非阻塞 select，满槽时该夜行者本次顺延、下一 tick 重规划。
package server

import (
	"bytes"
	"cmp"
	"context"
	"math"
	"slices"
	"sort"
	"sync"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/pathfind"
	"github.com/channing771/mornlea/internal/sim"
)

// 有界追逐的固定数值契约：每 tick 至多构造 2 份路径快照、在途 A* 恒 ≤2（两
// 槽）、成功应用路径后按 20 tick 的节奏周期性重规划（与攻击/受击/灼烧共用
// 的 20 tick 周期同源）。预算与槽位都有硬上界，权威 tick 绝不因夜行者系统产
// 生无界工作或阻塞等待。
const (
	// hostileMaxSnapshotsPerTick 是单次推进最多构造的路径快照份数（含网格
	// 深拷贝的世界读取）。预算在循环头部检查，第三只到期夜行者连世界读取
	// 都不会发生。
	hostileMaxSnapshotsPerTick = 2
	// hostileMaxInFlightPaths 是同时在途的 A* 计算数（两槽），也是 results
	// channel 的容量：容量恰好覆盖峰值，worker 发送永不因缓冲不足而滞留。
	hostileMaxInFlightPaths = 2
	// hostileRepathPeriodTicks 是路径成功应用后的周期性重规划间隔。追逐的
	// 目标是移动玩家：除 revision 失效与路径走尽外，这个节奏保证目标漂移
	// 与「卡住」的个体也会被周期性重排（写入持久化世界时间轴，跨重启保留）。
	hostileRepathPeriodTicks = uint64(20)
)

// hostileAttackRangeSquared 是攻击距离边界的平方。与 sim 的结算重验使用同一
// 份导出常量与同一表达式次序，两侧的 1.8 格边界逐位一致。
var hostileAttackRangeSquared = sim.HostileAttackRange * sim.HostileAttackRange

// hostileTargetPlayer 是编排层消费的在线玩家事实：稳定 ID、所属会话（攻击
// 意图的目标寻址，`sim` 不维护 PlayerID 到会话的映射）、维度与权威位置。
// 生产实现来自会话注册表（onlineHostileTargets），测试可注入固定集合。
type hostileTargetPlayer struct {
	id        core.PlayerID
	session   sim.SessionID
	dimension core.DimensionID
	position  [3]float32
}

// hostileChaseSlot 是一只夜行者的追逐编排状态。只有权威 tick 写。generation
// 在目标变化或路径作废时推进，用于在结果回送时拦截过期派发；target 与
// generation 一起构成「已过期（generation/target 变化）的结果丢弃」的判据。
type hostileChaseSlot struct {
	target    core.PlayerID
	hasTarget bool
	// generation 是槽位的派发世代；每次目标变化或路径作废都 +1。
	generation   uint64
	pathInFlight bool
	path         *pathfind.PathResult
	waypoint     int
}

// hostilePathOutcome 是一次寻路的结果，携带派发时刻的槽位世代、目标与维度供
// 过期判定与 revision 重验。
type hostilePathOutcome struct {
	mobID      uint64
	generation uint64
	target     core.PlayerID
	dimension  core.DimensionID
	result     pathfind.PathResult
	err        error
}

// hostilePathJob 是投递给 worker goroutine 的不可变寻路任务：网格是构造期一
// 次性深拷贝的快照，worker 不触碰任何活世界状态。
type hostilePathJob struct {
	mobID       uint64
	generation  uint64
	target      core.PlayerID
	dimension   core.DimensionID
	grid        pathfind.PathGrid
	start, goal pathfind.PathCell
}

// hostileManager 编排全部夜行者的有界追逐。零值不可用，经 newHostileManager
// 构造；关闭顺序见 beginShutdown/close。
type hostileManager struct {
	engine *sim.Engine
	table  pathfind.PathBlockTable

	// onlinePlayers 返回 tick 边界的在线玩家事实（按 ID 升序、仅存活且已激
	// 活），由 Server 在构造后注入——会话注册表归 Server 所有，manager 只消
	// 费这一权威源。nil 是防御缺省（视同无人在线）。调用方必须持有 stepMu，
	// 与 manager 其余状态同一单写者边界。
	onlinePlayers func() []hostileTargetPlayer

	// mobs/targets 是本 tick 的一致观察截面（refreshMobs 一次取齐），编排各
	// 阶段共用，避免重复读取；slots 按 ID 建档并随集合裁剪。
	mobs    []sim.HostileMob
	targets []hostileTargetPlayer
	slots   map[uint64]*hostileChaseSlot

	semaphore chan struct{}
	results   chan hostilePathOutcome

	ctx       context.Context
	cancel    context.CancelFunc
	waitGroup sync.WaitGroup
}

// newHostileManager 构造夜行者追逐编排。worker 纪律与 companion manager 同
// 构：goroutine 随派发创建、结果经有界 channel 回送、ctx 取消时放弃结果。
func newHostileManager(engine *sim.Engine) *hostileManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &hostileManager{
		engine:    engine,
		table:     pathfind.NewPathBlockTable(productionCompanionPassableBlocks()),
		slots:     make(map[uint64]*hostileChaseSlot),
		semaphore: make(chan struct{}, hostileMaxInFlightPaths),
		results:   make(chan hostilePathOutcome, hostileMaxInFlightPaths),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// beginShutdown 进入关服序列：取消在途寻路。调用点（Server.Shutdown 冻结段）
// 已经停止推进权威 tick，此后不再有新的派发。
func (m *hostileManager) beginShutdown() { m.cancel() }

// close 等待全部在途 worker 退出。未被排空的结果直接放弃：路径不落盘，重启
// 后所有夜行者在首个到期 tick 重算（存档只保留重规划节奏，不保留路径）。
func (m *hostileManager) close() {
	m.cancel()
	m.waitGroup.Wait()
}

// advance 是 tick 边界的编排入口，固定次序为：观察截面刷新 → 结果应用（ID
// 序）→ 快照派发（含目标选择与追逐事实写定）→ waypoint 执行。派发先于执行，
// 保证「进入攻击距离的同一 tick」目标事实已经落盘、攻击意图可以立即冻结；全
// 部阶段在持有 stepMu 的 tick 路径内完成，任何一步都不等待 A*。
func (m *hostileManager) advance() {
	m.refreshMobs()
	m.applyPathOutcomes()
	m.dispatchSnapshots()
	m.advanceRunners()
}

// refreshMobs 缓存本 tick 的夜行者值快照与在线玩家事实，并让槽位集合跟随夜
// 行者集合（死亡/消失个体的槽位同 tick 裁剪，绝不残留）。
func (m *hostileManager) refreshMobs() {
	m.mobs = m.engine.HostileMobs()
	if m.onlinePlayers != nil {
		m.targets = m.onlinePlayers()
	} else {
		m.targets = nil
	}
	for id := range m.slots {
		if m.mobIndex(id) < 0 {
			delete(m.slots, id)
		}
	}
	for index := range m.mobs {
		id := m.mobs[index].ID
		if _, ok := m.slots[id]; !ok {
			// 新建档的槽位以 sim 恢复事实播种：存档恢复的目标在下一次派发
			// 记账之前就参与攻击距离裁决，不会因为槽位刚建而丢一个 tick。
			m.slots[id] = &hostileChaseSlot{
				hasTarget: m.mobs[index].HasTarget,
				target:    m.mobs[index].PlayerID,
			}
		}
	}
}

// mobIndex 二分查找夜行者下标；未命中返回 -1。mobs 按 ID 升序（sim 的集合秩
// 序），查找因此无分配且对数级。
func (m *hostileManager) mobIndex(id uint64) int {
	index := sort.Search(len(m.mobs), func(i int) bool {
		return m.mobs[i].ID >= id
	})
	if index < len(m.mobs) && m.mobs[index].ID == id {
		return index
	}
	return -1
}

// applyPathOutcomes 在 tick 边界非阻塞排空寻路结果并按 ID 升序应用：同世界状
// 态下的应用次序（进而重规划节奏的写定次序）因此可重放。每份结果先做过期判
// 定（槽位缺失、派发世代变化、目标变化）再应用。
func (m *hostileManager) applyPathOutcomes() {
	var drained []hostilePathOutcome
	for {
		select {
		case outcome := <-m.results:
			drained = append(drained, outcome)
			continue
		default:
		}
		break
	}
	slices.SortFunc(drained, func(a, b hostilePathOutcome) int {
		return cmp.Compare(a.mobID, b.mobID)
	})
	for _, outcome := range drained {
		m.applyPathOutcome(outcome)
	}
}

// applyPathOutcome 应用单份寻路结果。过期结果（generation/target 任一变化）
// 整体丢弃——该夜行者的最新决策已由触发变化的派发自行推进；失败或 revision
// 已变化的结果同样丢弃并把重规划排到下一 tick（「过期/失败结果丢弃并在下一
// tick 重规划」的固定契约）；成功结果携带的 revision 集合与当前权威状态一致
// 才落地，并按周期节奏写定下一次重规划 tick（经 `PlanHostileChase` 落入持久
// 化世界时间轴）。
func (m *hostileManager) applyPathOutcome(outcome hostilePathOutcome) {
	slot := m.slots[outcome.mobID]
	if slot == nil || !slot.pathInFlight {
		return
	}
	slot.pathInFlight = false
	if slot.generation != outcome.generation || slot.target != outcome.target {
		return
	}
	now := m.engine.WorldTime()
	if outcome.err != nil {
		slot.path = nil
		slot.waypoint = 0
		m.engine.PlanHostileChase(outcome.mobID, true, outcome.target, now+1)
		return
	}
	if !m.revisionsCurrent(outcome.dimension, outcome.result.Revisions) {
		slot.path = nil
		slot.waypoint = 0
		m.engine.PlanHostileChase(outcome.mobID, true, outcome.target, now+1)
		return
	}
	result := outcome.result
	slot.path = &result
	slot.waypoint = 0
	m.engine.PlanHostileChase(outcome.mobID, true, outcome.target, now+hostileRepathPeriodTicks)
}

// revisionsCurrent 报告结果携带的每个区块 revision 是否仍与当前权威状态一致
// （缺失、未 ready 或值失配都视为已变化）。只读 ChunkInfo 元数据，无深拷贝。
func (m *hostileManager) revisionsCurrent(dimension core.DimensionID, revisions []pathfind.ChunkRevision) bool {
	for _, want := range revisions {
		info, ok := m.engine.ChunkInfo(core.ChunkKey{Dimension: dimension, Pos: want.Chunk})
		if !ok || info.State != sim.ChunkReady || info.Revision != want.Revision {
			return false
		}
	}
	return true
}

// advanceRunners 推进全部夜行者的 waypoint 执行（ID 升序）：先做攻击距离裁
// 决——与所选目标的水平距离进入边界即停移并冻结一次攻击意图（冷却中不再重
// 复冻结）；否则对既有路径做提交前重验（revision 与当前格），消费已到达的
// waypoint 并把朝向下一 waypoint 的世界轴方向量经 `sim.HostileAction` 提交。
// 失效路径清空并把重规划排到下一 tick；路径走尽同样下一 tick 以目标当前位置
// 重规划。无路径的夜行者不提交任何移动意图——绝不穿墙直线接近目标。
func (m *hostileManager) advanceRunners() {
	now := m.engine.WorldTime()
	for index := range m.mobs {
		mob := &m.mobs[index]
		slot := m.slots[mob.ID]
		if slot == nil {
			continue
		}
		if slot.hasTarget {
			// 追逐事实以槽位为准：本 tick 派发的记账（或建档时的恢复事实播
			// 种）在这里可见，攻击意图因此能与目标选择同 tick 生效。
			target, ok := m.targetByID(slot.target)
			if ok && target.dimension == mob.Dimension &&
				withinHostileAttackRange([3]float32(mob.State.Position), target.position) {
				if mob.AttackCooldown == 0 {
					m.engine.EnqueueHostileAction(sim.HostileAction{
						ID:            mob.ID,
						AttackTarget:  true,
						TargetSession: target.session,
					})
				}
				continue
			}
		}
		if slot.path == nil {
			continue
		}
		// waypoint 提交前重验：结果携带的每个区块 revision 都必须与当前权威
		// 状态一致；失效即清空路径并按契约把重规划排到下一 tick。策略取零值
		// 即纯读判定——`ShouldUse` 不推进任何计数，夜行者不消费伙伴的三连失
		// 败预算，重算节奏由 nextRepath 单独承载。
		var policy pathfind.PathPolicy
		revisions := chunkRevisionsAround(
			m.engine, mob.Dimension, [3]float32(mob.State.Position))
		if !policy.ShouldUse(*slot.path, slot.waypoint, revisions) {
			slot.path = nil
			slot.waypoint = 0
			m.engine.PlanHostileChase(mob.ID, mob.HasTarget, mob.PlayerID, now+1)
			continue
		}
		// 当前格裁决先于提交输入：已到达的 waypoint 立即消费，绝不提交回头
		// 移动（与伙伴同款 0.35 格到达阈值）。
		for slot.waypoint < len(slot.path.Waypoints) &&
			arrivedAtWaypoint([3]float32(mob.State.Position), slot.path.Waypoints[slot.waypoint]) {
			slot.waypoint++
		}
		if slot.waypoint >= len(slot.path.Waypoints) {
			slot.path = nil
			slot.waypoint = 0
			m.engine.PlanHostileChase(mob.ID, mob.HasTarget, mob.PlayerID, now+1)
			continue
		}
		waypoint := slot.path.Waypoints[slot.waypoint]
		position := mob.State.Position
		dx := float32(waypoint.X) + 0.5 - position.X()
		dz := float32(waypoint.Z) + 0.5 - position.Z()
		length := float32(math.Sqrt(float64(dx*dx + dz*dz)))
		if length == 0 {
			continue
		}
		action := sim.HostileAction{
			ID:    mob.ID,
			MoveX: dx / length,
			MoveZ: dz / length,
		}
		if waypoint.Y > int32(math.Floor(float64(position.Y()))) {
			// 目标 waypoint 高于脚下一格及以上：StepHeight 不足以登上整格台
			// 阶，按住 Jump 由权威物理裁决（与伙伴的移动输入同款规则）。
			action.Jump = true
		}
		m.engine.EnqueueHostileAction(action)
	}
}

// dispatchSnapshots 为到期夜行者派发路径快照：按 ID 升序、每 tick 至多
// hostileMaxSnapshotsPerTick 份。「到期」以持久化的 `NextRepathTicks` 对世界
// 时间裁决；无可选目标、攻击距离内或槽位满的个体跳过或顺延。目标变化即刻作
// 废旧路径与在途结果（generation 推进）；两槽投递必须非阻塞，满槽时该夜行
// 者本次顺延、下一 tick 重规划。预算在循环头部检查，超预算的个体连世界读取
// 都不会发生。
func (m *hostileManager) dispatchSnapshots() {
	budget := hostileMaxSnapshotsPerTick
	now := m.engine.WorldTime()
	for index := range m.mobs {
		if budget == 0 {
			break
		}
		mob := &m.mobs[index]
		if mob.NextRepathTicks > now {
			continue
		}
		slot := m.slots[mob.ID]
		if slot == nil || slot.pathInFlight {
			continue
		}
		target, ok := m.nearestTarget(mob)
		if !ok {
			if mob.HasTarget {
				// 目标消失：清目标事实并把重选排到下一 tick。
				slot.hasTarget = false
				slot.target = core.PlayerID{}
				slot.generation++
				slot.path = nil
				slot.waypoint = 0
				m.engine.PlanHostileChase(mob.ID, false, core.PlayerID{}, now+1)
			}
			continue
		}
		if !slot.hasTarget || slot.target != target.id {
			// 目标变化（含首次选定）：旧路径指向过时终点，在途结果亦不再可
			// 信，一并作废。追逐事实先于攻击距离裁决写定，同 tick 的攻击冻
			// 结才有可寻址的目标。
			slot.hasTarget = true
			slot.target = target.id
			slot.generation++
			slot.path = nil
			slot.waypoint = 0
			m.engine.PlanHostileChase(mob.ID, true, target.id, now+1)
		}
		if withinHostileAttackRange([3]float32(mob.State.Position), target.position) {
			// 攻击距离内无需路径：停移与攻击冻结由 advanceRunners 裁决。
			continue
		}
		// 两槽非阻塞投递：满槽即顺延，绝不阻塞权威 tick 等待 A* 槽位。
		select {
		case m.semaphore <- struct{}{}:
		default:
			continue
		}
		grid, start, goal, ok := m.buildChaseGrid(*mob, target)
		if !ok {
			// 覆盖区块未就绪或窗缘无站立终点：本次顺延，不消耗快照预算。
			<-m.semaphore
			continue
		}
		budget--
		slot.pathInFlight = true
		m.waitGroup.Add(1)
		go m.pathWorker(hostilePathJob{
			mobID:      mob.ID,
			generation: slot.generation,
			target:     target.id,
			dimension:  mob.Dimension,
			grid:       grid,
			start:      start,
			goal:       goal,
		})
	}
}

// pathWorker 在 worker goroutine 上执行确定性整数 A* 并把结果回送 tick 边界。
// 槽位在计算返回后、结果发送前释放（`companion` worker 的同款时序论证）：
// semaphore 约束的是在途计算数，FindPath 返回即计算结束；ctx 取消（关服）时
// 放弃结果退出。
func (m *hostileManager) pathWorker(job hostilePathJob) {
	defer m.waitGroup.Done()
	result, err := pathfind.FindPath(job.grid, job.start, job.goal)
	<-m.semaphore
	select {
	case m.results <- hostilePathOutcome{
		mobID:      job.mobID,
		generation: job.generation,
		target:     job.target,
		dimension:  job.dimension,
		result:     result,
		err:        err,
	}:
	case <-m.ctx.Done():
	}
}

// nearestTarget 选择最近的 active 同维 live 玩家：比较在平方域进行，等距按
// `PlayerID` 字节序取较小者。targets 已按 ID 升序注入，扫描次序确定，同一观
// 察截面上的选择可重放。
func (m *hostileManager) nearestTarget(mob *sim.HostileMob) (hostileTargetPlayer, bool) {
	var nearest hostileTargetPlayer
	found := false
	var nearestDistance float32
	for _, candidate := range m.targets {
		if candidate.dimension != mob.Dimension {
			continue
		}
		dx := candidate.position[0] - mob.State.Position.X()
		dz := candidate.position[2] - mob.State.Position.Z()
		distance := dx*dx + dz*dz
		if !found || distance < nearestDistance ||
			(distance == nearestDistance && bytes.Compare(candidate.id[:], nearest.id[:]) < 0) {
			nearest, found, nearestDistance = candidate, true, distance
		}
	}
	return nearest, found
}

// targetByID 从本 tick 的观察截面解析目标玩家的最新事实；目标离线（不在集
// 合）返回 false。
func (m *hostileManager) targetByID(id core.PlayerID) (hostileTargetPlayer, bool) {
	for _, candidate := range m.targets {
		if candidate.id == id {
			return candidate, true
		}
	}
	return hostileTargetPlayer{}, false
}

// buildChaseGrid 在 tick 边界为一次追逐构造不可变网格与端点：窗口是以夜行者
// 站立格为中心的 33×9×33（半径常量取自 `companion`，与伙伴寻路同级）；覆盖
// 区块先经 ChunkInfo 元数据检查（廉价前置，未就绪即顺延、不做深拷贝），再整
// 份拷贝进 `pathfind.NewPathGrid`。终点经 chaseGoal 钳制解析。
func (m *hostileManager) buildChaseGrid(
	mob sim.HostileMob,
	target hostileTargetPlayer,
) (pathfind.PathGrid, pathfind.PathCell, pathfind.PathCell, bool) {
	center := standingCellOf([3]float32(mob.State.Position))
	window := pathfind.PathWindow{Center: center}
	view := companionChunkViewFor(m.engine, mob.Dimension, [3]float32(mob.State.Position))
	origin := window.Origin()
	sizeX, sizeY, sizeZ := window.Size()
	// 网格覆盖的区块矩形：窗口 33×33 格最多覆盖 3×3 区块，必然落在视图内。
	chunkOrigin := (core.BlockPos{X: origin.X, Z: origin.Z}).Chunk()
	chunkEnd := (core.BlockPos{X: origin.X + sizeX - 1, Z: origin.Z + sizeZ - 1}).Chunk()
	spanX := chunkEnd.X - chunkOrigin.X + 1
	spanZ := chunkEnd.Z - chunkOrigin.Z + 1
	if !view.allCoveredReady(chunkOrigin, spanX, spanZ) {
		return pathfind.PathGrid{}, pathfind.PathCell{}, pathfind.PathCell{}, false
	}
	revisions := make([]pathfind.ChunkRevision, 0, spanX*spanZ)
	for dz := chunkOrigin.Z; dz <= chunkEnd.Z; dz++ {
		for dx := chunkOrigin.X; dx <= chunkEnd.X; dx++ {
			revisions = append(revisions, pathfind.ChunkRevision{
				Chunk:    core.ChunkPos{X: dx, Z: dz},
				Revision: view.revisionAt(dx, dz),
			})
		}
	}
	grid, err := pathfind.NewPathGrid(
		core.BlockPos{X: origin.X, Y: origin.Y, Z: origin.Z},
		sizeX, sizeY, sizeZ,
		m.table,
		func(x, y, z int32) (core.BlockID, bool) {
			// 覆盖矩形已全部 ready，读取必然成功；防御性 ok=false 让
			// NewPathGrid 显式报错而不是读到半空网格。
			return view.blockAt(x, y, z)
		},
		revisions,
	)
	if err != nil {
		return pathfind.PathGrid{}, pathfind.PathCell{}, pathfind.PathCell{}, false
	}
	goal, ok := m.chaseGoal(view, center, standingCellOf(target.position))
	if !ok {
		return pathfind.PathGrid{}, pathfind.PathCell{}, pathfind.PathCell{}, false
	}
	return grid, center, goal, true
}

// chaseGoal 把目标玩家的站立格钳进以夜行者为中心的路径窗口：目标越窗时
// min/max 钳制天然把终点压到越窗轴上朝玩家方向的窗缘；再在窗缘列内按「离目
// 标原 Y 最近、同距取更低」选出可站立格（升序扫描 + 严格更优替换，同距保留
// 先见的更低格）。判据与 `companion` 的站立判定同行同表（feet/head 可通过、
// 正下方支撑阻挡），FindPath 会对最终端点再做一次权威校验；窗缘整列无站立
// 格时寻路无从发起，按顺延处理。
func (m *hostileManager) chaseGoal(
	view companionChunkView,
	center, raw pathfind.PathCell,
) (pathfind.PathCell, bool) {
	goal := pathfind.PathCell{
		X: clampInt32(raw.X,
			center.X-pathfind.PathWindowHorizontalRadius,
			center.X+pathfind.PathWindowHorizontalRadius),
		Z: clampInt32(raw.Z,
			center.Z-pathfind.PathWindowHorizontalRadius,
			center.Z+pathfind.PathWindowHorizontalRadius),
	}
	lowY := center.Y - pathfind.PathWindowVerticalRadius
	highY := center.Y + pathfind.PathWindowVerticalRadius
	best := pathfind.PathCell{}
	found := false
	bestDistance := int32(0)
	for y := lowY; y <= highY; y++ {
		candidate := pathfind.PathCell{X: goal.X, Y: y, Z: goal.Z}
		if !hostileStandable(view, m.table, candidate) {
			continue
		}
		distance := candidate.Y - raw.Y
		if distance < 0 {
			distance = -distance
		}
		if !found || distance < bestDistance {
			best, found, bestDistance = candidate, true, distance
		}
	}
	return best, found
}

// hostileStandable 与 `companion` 寻路的站立判定同判据同表：feet/head 可通
// 过、正下方支撑格阻挡；支撑格越出视图（未加载）不算站立。它在构造网格的
// 同一份区块拷贝上解析终点，不触碰活世界。
func hostileStandable(
	view companionChunkView,
	table pathfind.PathBlockTable,
	cell pathfind.PathCell,
) bool {
	feet, ok := view.blockAt(cell.X, cell.Y, cell.Z)
	if !ok || !table.PassableForTest(feet) {
		return false
	}
	head, ok := view.blockAt(cell.X, cell.Y+1, cell.Z)
	if !ok || !table.PassableForTest(head) {
		return false
	}
	support, ok := view.blockAt(cell.X, cell.Y-1, cell.Z)
	return ok && !table.PassableForTest(support)
}

// clampInt32 返回落在 [low, high] 内的最近值（low 不得大于 high；两个调用点
// 的边界由窗口半径常量保证）。
func clampInt32(value, low, high int32) int32 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

// withinHostileAttackRange 报告夜行者与目标的水平距离是否落在攻击边界内（平
// 方域比较，边界常量与 sim 的结算重验同源）。只用水平分量：垂直分离由重力
// 与寻路的 Y 语义处理，不参与停移判定。
func withinHostileAttackRange(from, to [3]float32) bool {
	dx := to[0] - from[0]
	dz := to[2] - from[2]
	return dx*dx+dz*dz <= hostileAttackRangeSquared
}

// onlineHostileTargets 枚举 tick 边界的在线玩家并归一为追逐目标事实：稳定 ID
// 来自会话注册表，维度/位置/会话取权威模拟，仅保留已激活且存活的玩家。结果
// 按 `PlayerID` 字节序升序，目标选择的等距裁决因此可重放。调用方必须持有
// stepMu（与伙伴的在线玩家快照同一单写者边界）。
func (server *Server) onlineHostileTargets() []hostileTargetPlayer {
	players := make([]hostileTargetPlayer, 0, len(server.playerSessions))
	for playerID, sessionID := range server.playerSessions {
		player, ok := server.engine.Player(sessionID)
		if !ok || !player.Ready || player.Health == 0 {
			continue
		}
		players = append(players, hostileTargetPlayer{
			id:        playerID,
			session:   sessionID,
			dimension: player.Dimension,
			position:  [3]float32(player.State.Position),
		})
	}
	slices.SortFunc(players, func(a, b hostileTargetPlayer) int {
		return bytes.Compare(a.id[:], b.id[:])
	})
	return players
}

// advanceHostileChase 是 Server.step 的夜行者编排调用点：先于 engine.Step，
// 本 tick 的夜行者意图必须先进 inbox 才能被夜行者阶段消费。
func (server *Server) advanceHostileChase() {
	if server.hostileManager != nil {
		server.hostileManager.advance()
	}
}
