// Package realm 持有权威世界状态与区块变更事务及环境推进。
package realm

import (
	"encoding/binary"
	"slices"
	"sort"

	"github.com/channing771/mornlea/packages/server/fluid"
	"github.com/channing771/mornlea/packages/server/updates"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
	"github.com/channing771/mornlea/packages/shared/worldgen"
)

const (
	farmlandWetRadius                 = 4
	farmlandWetLayersAbove            = 1
	farmlandMoistureCandidatesPerTick = 65_536
	farmlandMoistureReadsPerTick      = 65_536
	farmlandWetNeighborReads          = (2*farmlandWetRadius + 1) * (2*farmlandWetRadius + 1) * (farmlandWetLayersAbove + 1)
	farmlandMoistureRescanSide        = core.SectionSize + 2*farmlandWetRadius
	farmlandMoistureRescanCells       = farmlandMoistureRescanSide * farmlandMoistureRescanSide * core.SectionsPerChunk * core.SectionSize
)

// EnvironmentConfig 是环境阶段在一个权威 tick 内固定使用的参数快照。
type EnvironmentConfig struct {
	FluidFlowDelayTicks     uint32
	FluidUpdatesPerTick     uint32
	FluidRescanCellsPerTick uint32
	DropPickupDelayTicks    uint8
	RandomTicksPerSection   uint8
	CropGrowthChancePercent uint8
	// Weather/YearPhase/EffectiveDayPhase 是当 tick 的气候束，供随机 tick 的
	// 积雪/消融判定取局部温度：Weather 是权威天气种类；YearPhase 与
	// EffectiveDayPhase 沿 `core.YearPhaseAt`/`core.EffectiveDayPhase` 的语义
	// （0..1 年相位与 0..23999 季节化日内相位），由 runtime 从当 tick 季节快照
	// 投影——realm 不感知季节偏移或天气时钟，不自建季节算式。零值（晴、春分
	// 黎明）无降水，不产生任何积雪写入，因此只填 tunables 六项的既有调用方
	// 行为不变。
	Weather           core.WeatherKind
	YearPhase         float64
	EffectiveDayPhase uint16
}

// EnvironmentMutation 将环境写入和同 tick 的区块变更收敛到同一事务。
type EnvironmentMutation struct {
	*Mutation
	state *State
}

// farmlandMoistureState 是湿度阶段的本 tick 计量与全块重扫状态。候选待办本身
// 不在这里：自 FIFO → 统一调度器迁移（change unified-block-updates-world-streaming）
// 起，湿度候选由各维度共享的 `updates.Queue` 实例承载（kind=FarmlandMoisture、
// 新鲜入队 due=当 tick，经 `fluid.Queue.Scheduler()` 取得），消费顺序从入队序
// 改为调度器确定性全序——这是 authoritative-farming delta 允许的唯一行为可见
// 差异，预算、平衡态与同 tick 重判语义原样保持。
type farmlandMoistureState struct {
	rescans              farmlandMoistureRescanState
	candidateInspections int
	blockReads           int
}

type farmlandMoistureRescanState struct {
	pending []core.ChunkKey
	queued  map[core.ChunkKey]struct{}
	cursor  int
}

type fluidRescanState struct {
	pending []core.ChunkKey
	queued  map[core.ChunkKey]struct{}
	plane   int
	section int
	// scratch 是 native 重扫调用的复用缓冲（输入拼装/输出解码/坐标切片），
	// box/meta 是盒体与元数据表的编码复用缓冲；三者按需增长、跨 tick 复用，
	// 不跨 tick 缓存编码内容（每次调用现场重组，语义与逐 tick 读世界一致）。
	scratch fluid.RescanScratch
	box     []byte
	meta    []byte
}

type environmentState struct {
	config                EnvironmentConfig
	tick                  uint64
	seed                  int64
	fluidQueues           map[core.DimensionID]*fluid.Queue
	scope                 map[core.ChunkKey]struct{}
	scopeNext             map[core.ChunkKey]struct{}
	fluidDimensionScratch []core.DimensionID
	fluidRescan           fluidRescanState
	farmlandMoisture      farmlandMoistureState
	// moistureHandler 是注册进各维度调度器 FarmlandMoisture 域的稳定处理回调
	// （只捕获 State 指针，构造一次跨 tick 复用）；moistureDimension/
	// moistureMutation 是它每次推进时的工作上下文，由 AdvanceFarmlandMoisture
	// 先设置再驱动对应维度，注册路径因此不产生闭包分配。
	moistureHandler   updates.Handler
	moistureDimension *Dimension
	moistureMutation  *EnvironmentMutation
	cropCellScratch   []int
	cropCellsExamined int
	cropBlockReads    int
}

// NewEnvironmentMutation 将环境参数附着到当前 tick 的区块事务。
func (state *State) NewEnvironmentMutation(
	mutation *Mutation,
	tick uint64,
	config EnvironmentConfig,
) *EnvironmentMutation {
	if mutation == nil || mutation.state != state {
		panic("realm: environment mutation belongs to another state")
	}
	state.environment.config = config
	state.environment.tick = tick
	return &EnvironmentMutation{Mutation: mutation, state: state}
}

// SetBlock 写入一格并把真实变更登记到本次环境事务。
func (mutation *EnvironmentMutation) SetBlock(
	dimensionID core.DimensionID,
	position core.BlockPos,
	block core.BlockID,
) (old core.BlockID, changed bool, err error) {
	dimension := mutation.state.Dimension(dimensionID)
	if dimension == nil {
		return core.AirID, false, ErrChunkNotReady
	}
	old, changed, err = dimension.SetBlock(position, block)
	if err != nil || !changed {
		return old, changed, err
	}
	mutation.Record(dimensionID, position, block)
	mutation.state.environment.enqueueFluidUpdate(dimensionID, position)
	if core.IsFluid(old) != core.IsFluid(block) {
		mutation.state.environment.enqueueFarmlandMoistureAroundFluid(dimensionID, position)
	}
	return old, true, nil
}

// SetSeed 设置世界种子，供作物随机抽样使用。
func (state *State) SetSeed(seed int64) {
	state.environment.seed = seed
}

func (state *State) SetEnvironmentTick(tick uint64, seed int64, cfg EnvironmentConfig) {
	state.environment.tick = tick
	state.environment.seed = seed
	state.environment.config = cfg
}

func (state *environmentState) fluidQueue(dimension core.DimensionID) *fluid.Queue {
	if state.fluidQueues == nil {
		state.fluidQueues = make(map[core.DimensionID]*fluid.Queue)
	}
	queue := state.fluidQueues[dimension]
	if queue == nil {
		queue = fluid.NewQueue()
		state.fluidQueues[dimension] = queue
	}
	return queue
}

func fluidNeighbors(position core.BlockPos) [6]core.BlockPos {
	return [6]core.BlockPos{
		{X: position.X, Y: position.Y + 1, Z: position.Z},
		{X: position.X, Y: position.Y - 1, Z: position.Z},
		{X: position.X + 1, Y: position.Y, Z: position.Z},
		{X: position.X - 1, Y: position.Y, Z: position.Z},
		{X: position.X, Y: position.Y, Z: position.Z + 1},
		{X: position.X, Y: position.Y, Z: position.Z - 1},
	}
}

func (state *environmentState) enqueueFluidUpdate(dimension core.DimensionID, position core.BlockPos) {
	queue := state.fluidQueue(dimension)
	queue.Enqueue(position, state.tick, uint64(state.config.FluidFlowDelayTicks))
	for _, neighbor := range fluidNeighbors(position) {
		queue.Enqueue(neighbor, state.tick, uint64(state.config.FluidFlowDelayTicks))
	}
}

func (state *State) EnqueueFluidUpdate(dimension core.DimensionID, position core.BlockPos) {
	state.environment.enqueueFluidUpdate(dimension, position)
}

func (state *State) EnqueueFarmlandMoisture(dimension core.DimensionID, position core.BlockPos) {
	state.environment.enqueueFarmlandMoisture(dimension, position)
}

func (state *State) EnqueueFarmlandMoistureAroundFluid(dimension core.DimensionID, position core.BlockPos) {
	state.environment.enqueueFarmlandMoistureAroundFluid(dimension, position)
}

func (state *State) FluidQueue(dimension core.DimensionID) *fluid.Queue {
	return state.environment.fluidQueue(dimension)
}

// AppendFluidScopeKeys 把当前流体推进范围追加到调用方 slice，不导出 owner map。
func (state *State) AppendFluidScopeKeys(dst []core.ChunkKey) []core.ChunkKey {
	for key := range state.environment.scope {
		dst = append(dst, key)
	}
	slices.SortFunc(dst, func(left, right core.ChunkKey) int {
		switch {
		case chunkKeyLess(left, right):
			return -1
		case chunkKeyLess(right, left):
			return 1
		default:
			return 0
		}
	})
	return dst
}

// FluidScopeContains 报告区块是否属于最近完成的流体推进范围。
func (state *State) FluidScopeContains(key core.ChunkKey) bool {
	_, contains := state.environment.scope[key]
	return contains
}

// FluidRescanPendingCount 返回尚未完成的边界重扫数量。
func (state *State) FluidRescanPendingCount() int {
	return len(state.environment.fluidRescan.pending)
}

func (state *State) CropCellsExamined() int {
	return state.environment.cropCellsExamined
}

func (state *State) CropBlockReads() int {
	return state.environment.cropBlockReads
}

// enqueueFarmlandMoisture 把一格湿度候选排进该维度的统一调度器实例
// （FarmlandMoisture 域、due=当前 tick）：无旧积压时它在同一权威 tick 的流体
// 推进子阶段之后即被结算。去重与「只提前不推迟」由调度器的 (pos, kind) 键承载，
// 这里不再自持队列结构。
func (state *environmentState) enqueueFarmlandMoisture(dimension core.DimensionID, position core.BlockPos) {
	if position.Y < core.MinY || position.Y >= core.MaxY {
		return
	}
	state.fluidQueue(dimension).Scheduler().Enqueue(position, updates.KindFarmlandMoisture, state.tick)
}

func (state *environmentState) enqueueFarmlandMoistureAroundFluid(
	dimension core.DimensionID,
	position core.BlockPos,
) {
	for y := position.Y - farmlandWetLayersAbove; y <= position.Y; y++ {
		for z := position.Z - farmlandWetRadius; z <= position.Z+farmlandWetRadius; z++ {
			for x := position.X - farmlandWetRadius; x <= position.X+farmlandWetRadius; x++ {
				state.enqueueFarmlandMoisture(dimension, core.BlockPos{X: x, Y: y, Z: z})
			}
		}
	}
}

func (state *State) farmlandIsWet(dimension *Dimension, position core.BlockPos) bool {
	for dy := int32(0); dy <= farmlandWetLayersAbove; dy++ {
		for dz := int32(-farmlandWetRadius); dz <= farmlandWetRadius; dz++ {
			for dx := int32(-farmlandWetRadius); dx <= farmlandWetRadius; dx++ {
				block, ready := dimension.BlockAt(core.BlockPos{
					X: position.X + dx,
					Y: position.Y + dy,
					Z: position.Z + dz,
				})
				state.environment.farmlandMoisture.blockReads++
				if ready && core.IsFluid(block) {
					return true
				}
			}
		}
	}
	return false
}

func (state *farmlandMoistureRescanState) enqueueChunk(key core.ChunkKey) {
	if state.queued == nil {
		state.queued = make(map[core.ChunkKey]struct{})
	}
	if _, exists := state.queued[key]; exists {
		return
	}
	state.queued[key] = struct{}{}
	state.pending = append(state.pending, key)
}

func (state *farmlandMoistureRescanState) dropOutOfScope(scope map[core.ChunkKey]struct{}) {
	if len(state.pending) == 0 {
		return
	}
	head := state.pending[0]
	kept := state.pending[:0]
	for _, key := range state.pending {
		if _, active := scope[key]; !active {
			delete(state.queued, key)
			continue
		}
		kept = append(kept, key)
	}
	state.pending = kept
	if len(kept) == 0 || kept[0] != head {
		state.cursor = 0
	}
}

func (state *farmlandMoistureRescanState) pop() {
	key := state.pending[0]
	delete(state.queued, key)
	state.pending = append(state.pending[:0], state.pending[1:]...)
	state.cursor = 0
}

func farmlandMoistureRescanPosition(key core.ChunkKey, cursor int) core.BlockPos {
	x := cursor % farmlandMoistureRescanSide
	z := (cursor / farmlandMoistureRescanSide) % farmlandMoistureRescanSide
	y := cursor / (farmlandMoistureRescanSide * farmlandMoistureRescanSide)
	return core.BlockPos{
		X: (key.Pos.X << core.SectionShift) - farmlandWetRadius + int32(x),
		Y: core.MinY + int32(y),
		Z: (key.Pos.Z << core.SectionShift) - farmlandWetRadius + int32(z),
	}
}

func (state *State) updateEnvironmentScope(active []core.ChunkKey) {
	environment := &state.environment
	if environment.scope == nil {
		environment.scope = make(map[core.ChunkKey]struct{})
		environment.scopeNext = make(map[core.ChunkKey]struct{})
	}
	clear(environment.scopeNext)
	for _, key := range active {
		dimension := state.Dimension(key.Dimension)
		if dimension == nil {
			continue
		}
		if _, ready := dimension.ReadyChunk(key.Pos); !ready {
			continue
		}
		environment.scopeNext[key] = struct{}{}
	}
	for _, key := range active {
		if _, inScope := environment.scopeNext[key]; !inScope {
			continue
		}
		if _, wasActive := environment.scope[key]; wasActive {
			continue
		}
		environment.farmlandMoisture.rescans.enqueueChunk(key)
	}
	environment.scope, environment.scopeNext = environment.scopeNext, environment.scope
}

func (state *State) runFarmlandMoistureRescans(budget int) {
	moisture := &state.environment.farmlandMoisture
	rescans := &moisture.rescans
	rescans.dropOutOfScope(state.environment.scope)
	for budget > 0 && len(rescans.pending) > 0 {
		key := rescans.pending[0]
		dimension := state.Dimension(key.Dimension)
		if dimension == nil {
			rescans.pop()
			continue
		}
		for budget > 0 && rescans.cursor < farmlandMoistureRescanCells {
			position := farmlandMoistureRescanPosition(key, rescans.cursor)
			block, ready := dimension.BlockAt(position)
			moisture.blockReads++
			budget--
			rescans.cursor++
			if !ready || !core.IsFarmland(block) {
				continue
			}
			farmlandKey := core.ChunkKey{Dimension: key.Dimension, Pos: position.Chunk()}
			if _, active := state.environment.scope[farmlandKey]; active {
				state.environment.enqueueFarmlandMoisture(key.Dimension, position)
			}
		}
		if rescans.cursor < farmlandMoistureRescanCells {
			return
		}
		rescans.pop()
	}
}

// AdvanceFarmlandMoisture 按统一调度器确定性全序与双预算处理活动区块内的湿度
// 候选：每维度在自己的共享调度器实例上以 `AdvanceKinds(now, FarmlandMoisture)`
// 结算（engine_step 的固定阶段序保证流体推进子阶段先行，新鲜候选同 tick 重判）。
// 候选检查数与方块读取数的预算是跨维度合计的全局上界，由处理回调内的计量守卫
// 承载；预算不足的候选按原 dueTick 回插顺延，不丢失。全块重扫保留在本阶段尾部。
func (state *State) AdvanceFarmlandMoisture(active []core.ChunkKey, mutation *EnvironmentMutation) {
	state.updateEnvironmentScope(active)
	moisture := &state.environment.farmlandMoisture
	moisture.blockReads = 0
	moisture.candidateInspections = 0
	if state.environment.moistureHandler == nil {
		state.environment.moistureHandler = state.newFarmlandMoistureHandler()
	}
	now := state.environment.tick
	for _, id := range state.sortedFluidDimensions() {
		queue := state.environment.fluidQueues[id]
		if queue.Scheduler().LenOf(updates.KindFarmlandMoisture) == 0 {
			continue
		}
		dimension := state.Dimension(id)
		if dimension == nil {
			continue
		}
		state.environment.moistureDimension = dimension
		state.environment.moistureMutation = mutation
		scheduler := queue.Scheduler()
		scheduler.Register(updates.KindFarmlandMoisture, farmlandMoistureCandidatesPerTick, state.environment.moistureHandler)
		scheduler.AdvanceKinds(now, updates.KindFarmlandMoisture)
	}
	state.runFarmlandMoistureRescans(farmlandMoistureReadsPerTick - moisture.blockReads)
}

// newFarmlandMoistureHandler 构造湿度域处理回调：每个被调度器弹出的候选先计入全局
// 检查预算，再按「范围 → 读取守卫 → 目标格 → 邻域判定」推进；处置结果里
// HandleDeferred 让调度器把候选按原 dueTick 回插并暂停湿度域——余额不足的
// 判定不保存部分结果，候选继续占用待办、后续 tick 仍按全序可达。
func (state *State) newFarmlandMoistureHandler() updates.Handler {
	return func(entry updates.Entry) updates.HandleResult {
		environment := &state.environment
		moisture := &environment.farmlandMoisture
		if moisture.candidateInspections >= farmlandMoistureCandidatesPerTick {
			// 跨维度合计的检查预算已耗尽：本候选不消耗检查额度，按原 dueTick
			// 顺延到下一 tick（另一维度可能已花完全局额度）。
			return updates.HandleDeferred
		}
		moisture.candidateInspections++
		dimension := environment.moistureDimension
		chunkKey := core.ChunkKey{Dimension: dimension.id, Pos: entry.Pos.Chunk()}
		if _, ok := environment.scope[chunkKey]; !ok {
			// 候选离开 active Ready 范围：检查即丢弃（0 方块读取、计入检查数），
			// 重入范围由全块重扫在固定预算内重建湿度。
			return updates.HandleConsumed
		}
		if moisture.blockReads >= farmlandMoistureReadsPerTick {
			// 连目标格的 1 次读取都付不起：不消耗读取，整条顺延。
			return updates.HandleDeferred
		}
		block, ready := dimension.BlockAt(entry.Pos)
		moisture.blockReads++
		if !ready || !core.IsFarmland(block) {
			return updates.HandleConsumed
		}
		if farmlandMoistureReadsPerTick-moisture.blockReads < farmlandWetNeighborReads {
			// 邻域判定不可跨 tick 拆分：保留待办并暂停本域推进，下一 tick 重判。
			return updates.HandleDeferred
		}
		next := core.FarmlandDryID
		if state.farmlandIsWet(dimension, entry.Pos) {
			next = core.FarmlandWetID
		}
		if next != block {
			_, _, _ = environment.moistureMutation.SetBlock(dimension.id, entry.Pos, next)
		}
		return updates.HandleConsumed
	}
}

// Fluid 相关

type fluidWorld struct {
	state     *State
	id        core.DimensionID
	dimension *Dimension
	scope     map[core.ChunkKey]struct{}
	mutation  *Mutation
}

func (w *fluidWorld) chunk(position core.BlockPos) *world.Chunk {
	if position.Y < core.MinY || position.Y >= core.MaxY {
		return nil
	}
	key := core.ChunkKey{Dimension: w.id, Pos: position.Chunk()}
	if _, inScope := w.scope[key]; !inScope {
		return nil
	}
	chunk, ok := w.dimension.ReadyChunk(key.Pos)
	if !ok {
		return nil
	}
	return chunk
}

func (w *fluidWorld) BlockAt(position core.BlockPos) core.BlockID {
	chunk := w.chunk(position)
	if chunk == nil {
		return core.BarrierID
	}
	x, _, z := position.Local()
	return chunk.BlockAt(x, position.Y, z)
}

func (w *fluidWorld) SetBlock(position core.BlockPos, id core.BlockID) {
	chunk := w.chunk(position)
	if chunk == nil {
		return
	}
	x, _, z := position.Local()
	old := chunk.BlockAt(x, position.Y, z)
	if old == id {
		return
	}
	if w.settleFloodedCrop(chunk, position, old, id) || w.settleFloodedSapling(chunk, position, old, id) {
		if next := chunk.BlockAt(x, position.Y, z); core.IsFluid(old) != core.IsFluid(next) {
			w.state.environment.enqueueFarmlandMoistureAroundFluid(w.id, position)
		}
		return
	}
	chunk.SetBlock(x, position.Y, z, id)
	if core.IsFluid(old) != core.IsFluid(id) {
		w.state.environment.enqueueFarmlandMoistureAroundFluid(w.id, position)
	}
	w.mutation.Record(w.id, position, id)
}

// settleFloodedCrop 在流体写入的目标格当前是作物时结算冲毁。
func (w *fluidWorld) settleFloodedCrop(
	chunk *world.Chunk,
	position core.BlockPos,
	old core.BlockID,
	id core.BlockID,
) bool {
	if !core.IsCrop(old) || !core.IsFluid(id) {
		return false
	}
	blockIndex, indexed := world.ChunkBlockIndex(position)
	if !indexed {
		return true
	}
	x, _, z := position.Local()
	item, harvestable := core.BlockDrop(old)
	var stacks [2]core.ItemStack
	count := 0
	if harvestable {
		if old == core.WheatStage7ID {
			wheatCount, seedCount := sampler.CropYieldRolls(
				w.state.environment.seed, w.state.environment.tick, w.id, position,
			)
			stacks[count] = core.ItemStack{Item: item, Count: wheatCount}
			count++
			stacks[count] = core.ItemStack{Item: core.ItemWheatSeeds, Count: seedCount}
			count++
		} else {
			stacks[count] = core.ItemStack{Item: item, Count: 1}
			count++
		}
	}
	if count > 0 {
		next, capacityOK := chunk.PrepareDropBatch(
			stacks[:count], blockIndex, w.state.environment.config.DropPickupDelayTicks,
		)
		if !capacityOK {
			w.state.environment.enqueueFluidUpdate(w.id, position)
			return true
		}
		chunk.SetBlock(x, position.Y, z, id)
		w.mutation.Record(w.id, position, id)
		chunk.CommitDropBatch(next)
		return true
	}
	return false
}

// settleFloodedSapling 在流体写入的目标格当前是树苗时结算冲毁：掉落恰好 1 个
// 树苗，容量不足时原子拒绝并保留树苗、把该格重新排程等待重试（与 `settleFloodedCrop`
// 同语义——任何时刻都不会出现树苗已被替换而掉落物未产出的状态）。
func (w *fluidWorld) settleFloodedSapling(
	chunk *world.Chunk,
	position core.BlockPos,
	old core.BlockID,
	id core.BlockID,
) bool {
	if !core.IsSapling(old) || !core.IsFluid(id) {
		return false
	}
	blockIndex, indexed := world.ChunkBlockIndex(position)
	if !indexed {
		return true
	}
	x, _, z := position.Local()
	stacks := [1]core.ItemStack{{Item: core.ItemSapling, Count: 1}}
	next, capacityOK := chunk.PrepareDropBatch(
		stacks[:], blockIndex, w.state.environment.config.DropPickupDelayTicks,
	)
	if !capacityOK {
		w.state.environment.enqueueFluidUpdate(w.id, position)
		return true
	}
	chunk.SetBlock(x, position.Y, z, id)
	w.mutation.Record(w.id, position, id)
	chunk.CommitDropBatch(next)
	return true
}

type fluidBoundaryPlane struct {
	dx, dz         int32
	x0, x1, z0, z1 int
}

var fluidBoundaryPlanes = [4]fluidBoundaryPlane{
	{dx: 1, x0: 0, x1: 0, z0: 0, z1: core.SectionMask},
	{dx: -1, x0: core.SectionMask, x1: core.SectionMask, z0: 0, z1: core.SectionMask},
	{dz: 1, x0: 0, x1: core.SectionMask, z0: 0, z1: 0},
	{dz: -1, x0: 0, x1: core.SectionMask, z0: core.SectionMask, z1: core.SectionMask},
}

// rescanChunkFluids 对一个刚进入流体推进范围的区块执行边界重扫入队，扫描核心
// 经 `fluid.ScanRescanRegion` 送入 Rust engine kernel。
//
// 平面编排与游标语义和旧 Go 实现逐字一致：plane 0 是本区块整块，plane 1..4
// 依次是四个水平邻块贴着本区块的边界平面；邻块未就绪的平面跳过、不调 kernel、
// 不记额度（该邻块自己进入范围时会做对称的一次重扫）。重扫可中断：游标
// （第几个平面、第几个区段）跨 tick 保留，最多花掉 budget 格的检查额度。
// 逐位等价性由 `rescan_differential_test.go` 对冻结 oracle 的差分门禁钉死。
func (state *State) rescanChunkFluids(
	queue *fluid.Queue,
	dimension *Dimension,
	pos core.ChunkPos,
	now, delay uint64,
	budget int,
) (spent int, done bool) {
	if _, ready := dimension.ReadyChunk(pos); !ready {
		state.environment.fluidRescan.resetCursor()
		return 0, true
	}
	rs := &state.environment.fluidRescan
	for rs.plane <= len(fluidBoundaryPlanes) {
		chunkPos := pos
		x0, x1, z0, z1 := 0, core.SectionMask, 0, core.SectionMask
		if rs.plane > 0 {
			plane := fluidBoundaryPlanes[rs.plane-1]
			chunkPos = core.ChunkPos{X: pos.X + plane.dx, Z: pos.Z + plane.dz}
			if _, ready := dimension.ReadyChunk(chunkPos); !ready {
				rs.plane++
				rs.section = 0
				continue
			}
			x0, x1, z0, z1 = plane.x0, plane.x1, plane.z0, plane.z1
		}
		used, finished := rs.scanPlane(queue, dimension, chunkPos, x0, x1, z0, z1, now, delay, budget-spent)
		spent += used
		if !finished {
			return spent, false
		}
		rs.plane++
		rs.section = 0
	}
	rs.resetCursor()
	return spent, true
}

// scanPlane 执行单个 (区块, 平面) 扫描单元：现场编码以 chunkPos 为中心的
// MFL1 盒，交 kernel 扫描，把产出坐标按现行 `now+delay` 入队。盒组装不跨
// tick 缓存——每次调用现场重组，与旧实现逐 tick 读世界语义一致；同一调用
// 内每个平面至多进入一次，编码即每平面一次。
//
// 盒中心必须是被扫描区块（engine header 契约「被扫描区块是盒中心区块」）：
// 五段平面各有自己的「当前区块」，边界平面的盒中心是邻块，扫描条带落在该
// 盒的中心列 1..16（chunkPos 的局部 x0..x1/z0..z1 加 1 映射为盒内局部列）。
// 非正剩余额度（前序平面整段超支后）在编码盒之前零进度返回未完成：镜像旧
// Go 实现 `enqueueChunkFluids` 入口的 `spent >= budget` 检查对非正预算的行为，
// 同时省掉一次注定零进度的盒编码。
func (rs *fluidRescanState) scanPlane(
	queue *fluid.Queue,
	dimension *Dimension,
	chunkPos core.ChunkPos,
	x0, x1, z0, z1 int,
	now, delay uint64,
	budget int,
) (spent int, done bool) {
	if budget <= 0 {
		return 0, false
	}
	rs.box, rs.meta = encodeRescanBox(rs.box, rs.meta, dimension, chunkPos)
	positions, spent, done, resume := fluid.ScanRescanRegion(rs.box, rs.meta, fluid.RescanRegion{
		Center:       chunkPos,
		X0:           x0 + 1,
		X1:           x1 + 1,
		Z0:           z0 + 1,
		Z1:           z1 + 1,
		StartSection: rs.section,
		Budget:       budget,
	}, &rs.scratch)
	for _, position := range positions {
		queue.Enqueue(position, now, delay)
	}
	rs.section = resume
	return spent, done
}

// MFL1 盒体编码常量，与 engine `fluid_rescan.rs` 逐字一致（header 由
// `internal/fluid` 的包装拼装，此处只编码 header 之后的盒体与元数据表）。
const (
	// rescanBoxSectionUniform 是均匀区段记录的 kind 标记。
	rescanBoxSectionUniform uint8 = 0
	// rescanBoxSectionDense 是密集区段记录的 kind 标记。
	rescanBoxSectionDense uint8 = 1
	// rescanBoxSkirtColumns 是裙边列数：四边各 16 列 + 四角 4 列。
	rescanBoxSkirtColumns = 4*core.SectionSize + 4
	// rescanBoxMetadataBytes 是元数据表字节数：9 区块 × 24 区段 × 3B。
	rescanBoxMetadataBytes = 9 * core.SectionsPerChunk * 3
)

// rescanMetadataChunkOrder 是元数据表的区块序：(0,0)、(-1,-1)、(0,-1)、
// (1,-1)、(-1,0)、(1,0)、(-1,1)、(0,1)、(1,1)。
var rescanMetadataChunkOrder = [9][2]int32{
	{0, 0}, {-1, -1}, {0, -1}, {1, -1}, {-1, 0}, {1, 0}, {-1, 1}, {0, 1}, {1, 1},
}

// encodeRescanBox 组装以 center 为盒中心的 MFL1 盒体与元数据表，追加进复用
// 缓冲并返回。三段编码与 engine 布局逐字节对应：
//
//   - 中心区块 24 条区段记录：`IsUniform` 命中走 kind=0 均匀记录；否则 kind=1
//     按 `blockIndex` 同序（x + z*16 + y16*256）线性展开 4096×u16；
//   - 裙边 `rescanBoxSkirtColumns` 列 × 384 u16：就绪邻块取真实列数据，未就绪
//     邻块整列填 Barrier（镜像旧 `fluidRescanBlockAt` 对未就绪读 Barrier，
//     Barrier 的「实心不可替换」语义由此进入区段级与五邻不动点判定）；
//   - 元数据 `rescanBoxMetadataBytes` 字节：就绪区块均匀段记 (1, id)、非均匀
//     记 (0,0,0)，未就绪区块整块记均匀 Barrier（镜像旧
//     `fluidSectionUnreplaceable` 对未就绪返回不可替换）。
//
// center 必须就绪（调用方 `rescanChunkFluids` 已保证）；世界高度外的读在
// kernel 侧按 Barrier 处理，编码侧列数据本身只覆盖 `[core.MinY, core.MaxY)`。
func encodeRescanBox(box, meta []byte, dimension *Dimension, center core.ChunkPos) ([]byte, []byte) {
	chunk, ready := dimension.ReadyChunk(center)
	if !ready {
		panic("realm: 重扫盒中心区块未就绪")
	}
	box = box[:0]
	for sectionIndex := range core.SectionsPerChunk {
		section := chunk.Section(sectionIndex)
		if id, uniform := section.Blocks.IsUniform(); uniform {
			box = append(box, rescanBoxSectionUniform, 0)
			box = binary.LittleEndian.AppendUint16(box, uint16(id))
			continue
		}
		box = append(box, rescanBoxSectionDense, 0)
		for localY := range core.SectionSize {
			for localZ := range core.SectionSize {
				for localX := range core.SectionSize {
					box = binary.LittleEndian.AppendUint16(box, uint16(section.Blocks.Get(localX, localY, localZ)))
				}
			}
		}
	}
	// 裙边列序固定：四组各 16 列连续排列——(x=-1,z=0..15)、(x=16,z=0..15)、
	// (z=-1,x=0..15)、(z=16,x=0..15)，随后四角；盒内局部列 0/17 即中心区块
	// 局部的 -1/16。角列不参与五邻判定（偏移无对角），仍按同一就绪规则如实编码。
	for index := range core.SectionSize {
		box = encodeRescanSkirtColumn(box, dimension, center, 0, index+1)
	}
	for index := range core.SectionSize {
		box = encodeRescanSkirtColumn(box, dimension, center, core.SectionSize+1, index+1)
	}
	for index := range core.SectionSize {
		box = encodeRescanSkirtColumn(box, dimension, center, index+1, 0)
	}
	for index := range core.SectionSize {
		box = encodeRescanSkirtColumn(box, dimension, center, index+1, core.SectionSize+1)
	}
	for _, corner := range [4][2]int{{0, 0}, {core.SectionSize + 1, 0}, {0, core.SectionSize + 1}, {core.SectionSize + 1, core.SectionSize + 1}} {
		box = encodeRescanSkirtColumn(box, dimension, center, corner[0], corner[1])
	}
	meta = meta[:0]
	for _, offset := range rescanMetadataChunkOrder {
		neighbor, ready := dimension.ReadyChunk(core.ChunkPos{X: center.X + offset[0], Z: center.Z + offset[1]})
		for sectionIndex := range core.SectionsPerChunk {
			switch {
			case !ready:
				meta = append(meta, 1, byte(core.BarrierID), 0)
			default:
				if id, uniform := neighbor.Section(sectionIndex).Blocks.IsUniform(); uniform {
					meta = append(meta, 1)
					meta = binary.LittleEndian.AppendUint16(meta, uint16(id))
				} else {
					meta = append(meta, 0, 0, 0)
				}
			}
		}
	}
	return box, meta
}

// encodeRescanSkirtColumn 把盒内局部列 (boxX, boxZ)（取值 0 或 17，即中心
// 区块局部的 -1/16）的世界整列 384 格追加进 box：就绪邻块取真实数据，未就绪
// 整列填 Barrier。
func encodeRescanSkirtColumn(
	box []byte,
	dimension *Dimension,
	center core.ChunkPos,
	boxX, boxZ int,
) []byte {
	worldX := center.X<<core.SectionShift + int32(boxX) - 1
	worldZ := center.Z<<core.SectionShift + int32(boxZ) - 1
	chunk, ready := dimension.ReadyChunk(core.ChunkPos{X: worldX >> core.SectionShift, Z: worldZ >> core.SectionShift})
	if !ready {
		for range core.SectionsPerChunk * core.SectionSize {
			box = binary.LittleEndian.AppendUint16(box, uint16(core.BarrierID))
		}
		return box
	}
	localX := int(worldX & core.SectionMask)
	localZ := int(worldZ & core.SectionMask)
	for y := int32(core.MinY); y < core.MaxY; y++ {
		box = binary.LittleEndian.AppendUint16(box, uint16(chunk.BlockAt(localX, y, localZ)))
	}
	return box
}

func (state *fluidRescanState) resetCursor() {
	state.plane = 0
	state.section = 0
}

func (state *fluidRescanState) enqueueChunk(key core.ChunkKey) {
	if state.queued == nil {
		state.queued = make(map[core.ChunkKey]struct{})
	}
	if _, exists := state.queued[key]; exists {
		return
	}
	state.queued[key] = struct{}{}
	state.pending = append(state.pending, key)
}

func (state *fluidRescanState) dropOutOfScope(scope map[core.ChunkKey]struct{}) {
	if len(state.pending) == 0 {
		return
	}
	head := state.pending[0]
	kept := state.pending[:0]
	for _, key := range state.pending {
		if _, inScope := scope[key]; !inScope {
			delete(state.queued, key)
			continue
		}
		kept = append(kept, key)
	}
	state.pending = kept
	if len(kept) == 0 || kept[0] != head {
		state.resetCursor()
	}
}

func (state *State) runFluidRescans(now, delay uint64) {
	rs := &state.environment.fluidRescan
	rs.dropOutOfScope(state.environment.scope)
	budget := int(state.environment.config.FluidRescanCellsPerTick)
	for budget > 0 && len(rs.pending) > 0 {
		key := rs.pending[0]
		dimension := state.Dimension(key.Dimension)
		if dimension == nil {
			rs.resetCursor()
			delete(rs.queued, key)
			rs.pending = append(rs.pending[:0], rs.pending[1:]...)
			continue
		}
		spent, done := state.rescanChunkFluids(
			state.environment.fluidQueue(key.Dimension), dimension, key.Pos, now, delay, budget,
		)
		budget -= spent
		if !done {
			return
		}
		delete(rs.queued, key)
		rs.pending = append(rs.pending[:0], rs.pending[1:]...)
	}
}

func (state *State) AdvanceFluids(active []core.ChunkKey, mutation *Mutation) {
	now := state.environment.tick
	delay := uint64(state.environment.config.FluidFlowDelayTicks)
	budget := int(state.environment.config.FluidUpdatesPerTick)
	if state.environment.scope == nil {
		state.environment.scope = make(map[core.ChunkKey]struct{})
		state.environment.scopeNext = make(map[core.ChunkKey]struct{})
	}
	clear(state.environment.scopeNext)
	for _, key := range active {
		dimension := state.Dimension(key.Dimension)
		if dimension == nil {
			continue
		}
		if _, ok := dimension.ReadyChunk(key.Pos); !ok {
			continue
		}
		state.environment.scopeNext[key] = struct{}{}
	}
	for _, key := range active {
		if _, inScope := state.environment.scopeNext[key]; !inScope {
			continue
		}
		if _, wasInScope := state.environment.scope[key]; wasInScope {
			continue
		}
		state.environment.fluidRescan.enqueueChunk(key)
		state.environment.farmlandMoisture.rescans.enqueueChunk(key)
	}
	state.environment.scope, state.environment.scopeNext = state.environment.scopeNext, state.environment.scope
	state.runFluidRescans(now, delay)
	for _, id := range state.sortedFluidDimensions() {
		queue := state.environment.fluidQueues[id]
		dimension := state.Dimension(id)
		// 跳过判断按 FluidFlow 域计量：同一调度器实例还承载湿度域的待办，
		// 跨域总数（Len）非零不代表流体有待推进的到期项。
		if dimension == nil || queue.Scheduler().LenOf(updates.KindFluidFlow) == 0 {
			continue
		}
		queue.Advance(now, &fluidWorld{
			state:     state,
			id:        id,
			dimension: dimension,
			scope:     state.environment.scope,
			mutation:  mutation,
		}, budget, delay)
	}
}

func (state *State) sortedFluidDimensions() []core.DimensionID {
	ids := state.environment.fluidDimensionScratch[:0]
	for id := range state.environment.fluidQueues {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	state.environment.fluidDimensionScratch = ids
	return ids
}

// --- Crop ---

// sampler 是随机面判定器的统一调用点：零值 `updates.Sampler` 是无状态纯函数
// 集合，包级变量只是固定「判定真相在 updates」的写法（与 sim/entity 先例一
// 致）——随机 tick 的抽样派生（`SampleCells`）与生长、退化、产量、毒素各判定
// 流都从这里取值。历史上哈希链与域盐值常量在本包与 sim/entity 各持一份本地
// 副本，变更 unified-block-updates-world-streaming 已原样搬迁到 updates 导出；
// 搬迁前实现在固定输入下的输出由 updates 的已知答案测试与本包的固定世界重放
// 钉子逐位钉住，换接保持搅拌输入顺序、次数与常量逐字不变。
var sampler = updates.Sampler{}

func growCrop(block core.BlockID, wet, skyExposed bool) (next core.BlockID, changed bool) {
	if !core.IsCrop(block) {
		return block, false
	}
	if block == core.WheatStage7ID || block == core.PotatoStage7ID || block == core.CarrotStage7ID {
		return block, false
	}
	if !wet || !skyExposed {
		return block, false
	}
	return block + 1, true
}

func cropSkyExposed(chunk *world.Chunk, position core.BlockPos) bool {
	localX, _, localZ := position.Local()
	return position.Y >= chunk.HighestOpaque(localX, localZ)
}

func (state *State) AdvanceCrops(active []core.ChunkKey, mutation *Mutation) {
	samples := int(state.environment.config.RandomTicksPerSection)
	state.environment.cropCellsExamined = 0
	state.environment.cropBlockReads = 0
	if samples <= 0 {
		return
	}
	tick := state.environment.tick
	seed := state.environment.seed
	// 按稳定顺序处理
	keys := make([]core.ChunkKey, 0, len(active))
	for _, key := range active {
		dimension := state.Dimension(key.Dimension)
		if dimension == nil {
			continue
		}
		if _, ready := dimension.ReadyChunk(key.Pos); !ready {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return chunkKeyLess(keys[i], keys[j]) })
	for _, key := range keys {
		dimension := state.Dimension(key.Dimension)
		chunk, ready := dimension.ReadyChunk(key.Pos)
		if !ready {
			continue
		}
		baseX := key.Pos.X << core.SectionShift
		baseZ := key.Pos.Z << core.SectionShift
		for sectionY := range core.SectionsPerChunk {
			baseY := int32(sectionY<<core.SectionShift) + core.MinY
			state.environment.cropCellScratch = sampler.SampleCells(
				seed, tick, key, sectionY, samples, state.environment.cropCellScratch,
			)
			for _, cell := range state.environment.cropCellScratch {
				localX := cell & core.SectionMask
				localZ := (cell >> core.SectionShift) & core.SectionMask
				localY := cell >> (core.SectionShift * 2)
				state.environment.cropCellsExamined++
				state.advanceCropCell(dimension, key.Dimension, chunk, core.BlockPos{
					X: baseX + int32(localX),
					Y: baseY + int32(localY),
					Z: baseZ + int32(localZ),
				}, tick, mutation)
			}
		}
	}
}

// advanceCropCell 是随机 tick 抽中一格后的判定分发器：作物生长、树苗生长、
// 干耕地退化、积雪/消融共用同一抽样与预算，各分支互斥（作物、树苗与耕地命中后
// 直接返回）。作物与耕地分支至多写 1 格；树苗生长按树形几何整棵写入（见
// `advanceSaplingCell`）。
func (state *State) advanceCropCell(
	dimension *Dimension,
	dimensionID core.DimensionID,
	chunk *world.Chunk,
	position core.BlockPos,
	tick uint64,
	mutation *Mutation,
) {
	localX, _, localZ := position.Local()
	block := chunk.BlockAt(localX, position.Y, localZ)
	state.environment.cropBlockReads++
	if core.IsCrop(block) {
		below := position
		below.Y--
		belowBlock, ready := dimension.BlockAt(below)
		state.environment.cropBlockReads++
		wet := ready && belowBlock == core.FarmlandWetID
		grown, changed := growCrop(block, wet, cropSkyExposed(chunk, position))
		if !changed {
			return
		}
		if !sampler.CropGrowthRoll(
			state.environment.seed, tick, dimensionID, position,
			state.environment.config.CropGrowthChancePercent,
		) {
			return
		}
		// 先写入区块，成功后再登记变更，避免幽灵变更
		if _, changed, err := dimension.SetBlock(position, grown); err != nil || !changed {
			return
		}
		mutation.Record(dimensionID, position, grown)
		return
	}
	if block == core.FarmlandDryID {
		above := position
		above.Y++
		aboveBlock, ready := dimension.BlockAt(above)
		state.environment.cropBlockReads++
		if !ready || aboveBlock != core.AirID {
			return
		}
		if !sampler.FarmlandRevertRoll(state.environment.seed, tick, dimensionID, position) {
			return
		}
		// 先写入区块，成功后再登记变更，避免幽灵变更
		if _, changed, err := dimension.SetBlock(position, core.DirtID); err != nil || !changed {
			return
		}
		mutation.Record(dimensionID, position, core.DirtID)
		return
	}
	if core.IsSapling(block) {
		state.advanceSaplingCell(dimension, dimensionID, chunk, position, tick, mutation)
		return
	}
	// 既非作物也非干耕地：按白名单地表交给积雪/消融判定（内部再拒非白名单与
	// 非空气上方，多数命中到此为止零额外读取）。
	state.advanceSnowCover(dimension, dimensionID, chunk, position, block, mutation)
}

// saplingGrowthWrite 是一次生长写入的记账项：目标格、写入后的方块与写入前的
// 旧值。旧值只用于失败回滚，不回滚则不必保留。
type saplingGrowthWrite struct {
	position core.BlockPos
	block    core.BlockID
	old      core.BlockID
}

// advanceSaplingCell 尝试让随机 tick 抽中的树苗长成一棵普通橡树。
//
// 判定序（全部满足才尝试写入，任一步不满足都保持树苗原样、后续 tick 可重试）：
//
//  1. 支撑：正下方仍是 `DirtID` 或 `GrassID`；
//  2. 露天：本列最高非空气格不高于树苗自身（复用作物同式 `cropSkyExposed`）；
//  3. 根坐标上界：`root.Y <= core.MaxY - 9`。engine 只接受该上界内的根坐标，
//     越界请求以硬状态拒绝并让 Go 桥 panic；玩家可以在高处搭塔种苗，因此这条
//     前置守卫是必须的；
//  4. 独立冻结 salt 的 1/8 判定 `sampler.SaplingGrowthRoll` 命中；
//  5. 树形几何（engine ABI 单一真源）的每条记录都在世界高度内且当前是 `AirID`
//     或 `ShortGrassID`。树苗自身那一格（树干底）例外：它就是被替换的格；
//  6. 全部目标区块处于 Ready。
//
// 写入全有或全无：先逐格保存旧值再写，任一格写入失败即把已写入的格恢复为旧值，
// 不登记任何变更；成功后每个被改写的格经 `Mutation.Record` 登记，受影响区块的
// revision 由既有提交路径各推进一次。覆盖短草不产生掉落（环境生长不是种子来源）。
//
// 读取预算：支撑 1 次读取后，只有判定命中才会求值树形几何并逐格校验（至多
// `128` 格），因此每 tick 的读取总量仍以「被考察格数 × 128」为上界。
func (state *State) advanceSaplingCell(
	dimension *Dimension,
	dimensionID core.DimensionID,
	chunk *world.Chunk,
	position core.BlockPos,
	tick uint64,
	mutation *Mutation,
) {
	below := core.BlockPos{X: position.X, Y: position.Y - 1, Z: position.Z}
	belowBlock, belowReady := dimension.BlockAt(below)
	state.environment.cropBlockReads++
	if !belowReady || (belowBlock != core.DirtID && belowBlock != core.GrassID) {
		return
	}
	if !cropSkyExposed(chunk, position) {
		return
	}
	if position.Y > core.MaxY-9 {
		return
	}
	if !sampler.SaplingGrowthRoll(state.environment.seed, tick, dimensionID, position) {
		return
	}
	records := worldgen.TreeBlocks(state.environment.seed, position)
	for _, record := range records {
		target := core.BlockPos{
			X: position.X + int32(record.DX),
			Y: position.Y + int32(record.DY),
			Z: position.Z + int32(record.DZ),
		}
		if target == position {
			continue
		}
		if target.Y < core.MinY || target.Y >= core.MaxY {
			return
		}
		block, _ := dimension.BlockAt(target)
		state.environment.cropBlockReads++
		if block != core.AirID && block != core.ShortGrassID {
			return
		}
	}
	// 目标区块就绪前置：任一未就绪即放弃，避免写到一半才发现（零副作用）。
	// 未就绪区块的格在上面的空间校验里读作空气，因此这条检查是唯一的就绪闸门。
	for _, record := range records {
		target := core.BlockPos{
			X: position.X + int32(record.DX),
			Y: position.Y + int32(record.DY),
			Z: position.Z + int32(record.DZ),
		}
		if _, ready := dimension.ReadyChunk(target.Chunk()); !ready {
			return
		}
	}
	writes := make([]saplingGrowthWrite, 0, len(records))
	for _, record := range records {
		target := core.BlockPos{
			X: position.X + int32(record.DX),
			Y: position.Y + int32(record.DY),
			Z: position.Z + int32(record.DZ),
		}
		old, changed, err := dimension.SetBlock(target, record.Block)
		if err != nil {
			for _, write := range writes {
				_, _, _ = dimension.SetBlock(write.position, write.old)
			}
			return
		}
		if changed {
			writes = append(writes, saplingGrowthWrite{
				position: target,
				block:    record.Block,
				old:      old,
			})
		}
	}
	for _, write := range writes {
		mutation.Record(dimensionID, write.position, write.block)
	}
}

// Torch/Bed support

// wildPlantSweepCell 是 wild plant 复核快照里的一条已变位置：维度加方块坐标
// 唯一定位一格。
type wildPlantSweepCell struct {
	dimension core.DimensionID
	position  core.BlockPos
}

// SweepUnsupportedWildPlants 清除失去草皮支撑的短草：取得本 mutation 当前
// `ChangedBlocks()` 的稳定快照，对每个变化格只检查正上方一格——若上方是
// `ShortGrassID` 且变化格的最终值不再是 `GrassID`，就把短草写为空气并登记到
// 同一 mutation，零掉落、不做任何掉落容量预演（环境清除不是种子来源，玩家
// 采除才是）。
//
// 有界性契约：短草清空产生的新登记不递归重扫——单格植物不需要级联，快照
// 在入口一次取定，工作量严格正比于本 tick 已受预算约束的 changed set 大小，
// 每个候选只有常数次读取与至多一次写入；不启动 goroutine、不做 I/O 或全
// 世界扫描。本 sweep 在 runtime 的固定阶段序里先于火把与床支撑复核执行，
// 后两者因此能看到清草产生的新变更。
func (state *State) SweepUnsupportedWildPlants(mutation *Mutation) {
	changes := mutation.ChangedBlocks()
	if len(changes) == 0 {
		return
	}
	cells := make([]wildPlantSweepCell, len(changes))
	for index, change := range changes {
		cells[index] = wildPlantSweepCell{dimension: change.Dimension, position: change.Position}
	}
	for _, cell := range cells {
		state.invalidateWildGrassAbove(cell.dimension, cell.position, mutation)
	}
}

// invalidateWildGrassAbove 检查 position 正上方一格：那里是短草且 position 的
// 最终内容不再是草方块时，短草支撑失效，同 mutation 清为空气。支撑格读取
// 变化后的最终值——同 tick 内多次写入以最后一次为准，被换回草方块的支撑
// 不触发清除。上方格未加载（跨区块边界）时跳过：短草所在区块必然已就绪才
// 会被生成，未就绪意味着整列已随区块卸载，没有可复核的权威状态（火把复核
// 同款取舍）。世界高度外的上方读作空气，天然不是短草。
func (state *State) invalidateWildGrassAbove(
	dimensionID core.DimensionID,
	position core.BlockPos,
	mutation *Mutation,
) {
	dimension := state.Dimension(dimensionID)
	if dimension == nil {
		return
	}
	above := core.BlockPos{X: position.X, Y: position.Y + 1, Z: position.Z}
	block, ready := dimension.BlockAt(above)
	if !ready || block != core.ShortGrassID {
		return
	}
	supportBlock, supportReady := dimension.BlockAt(position)
	if !supportReady || supportBlock == core.GrassID {
		return
	}
	// 零掉落清除：短草没有物品身份，不 PrepareDrop、不预留容量——掉落槽
	// 满载也必须完成（与作物冲毁、火把/床复核的容量语义刻意不同）。
	if _, changed, err := dimension.SetBlock(above, core.AirID); err != nil || !changed {
		return
	}
	mutation.Record(dimensionID, above, core.AirID)
}

// saplingSweepCell 是树苗复核快照里的一条已变位置：维度加方块坐标唯一定位一格。
// 与短草/火把/床的复核快照同形但各自独立，避免一条 sweep 的目标类型被另一条改动。
type saplingSweepCell struct {
	dimension core.DimensionID
	position  core.BlockPos
}

// SweepUnsupportedSaplings 清除失去泥土/草地支撑的树苗：取得本 mutation 当前
// `ChangedBlocks()` 的稳定快照，对每个变化格只检查正上方一格——若上方是树苗且
// 变化格的最终值不再是 `DirtID` 或 `GrassID`，就把树苗清为空气并掉落恰好 1 个
// 树苗，登记到同一 mutation。
//
// 掉落走与采掘同形的原子路径：先 `PrepareDrop` 预检容量，容量不足整次清除被
// 拒绝（树苗保留、不写方块、不登记变更），后续支撑变化可重试。有界性与短草
// sweep 相同：快照在入口一次取定、不递归重扫，工作量严格正比于本 tick 已受预算
// 约束的 changed set 大小。本 sweep 与短草 sweep 同相位、先于火把与床复核执行，
// 因此后两者能看到树苗清除产生的新变更。
func (state *State) SweepUnsupportedSaplings(mutation *Mutation) {
	changes := mutation.ChangedBlocks()
	if len(changes) == 0 {
		return
	}
	cells := make([]saplingSweepCell, len(changes))
	for index, change := range changes {
		cells[index] = saplingSweepCell{dimension: change.Dimension, position: change.Position}
	}
	for _, cell := range cells {
		state.invalidateSaplingAbove(cell.dimension, cell.position, mutation)
	}
}

// invalidateSaplingAbove 检查 position 正上方一格：那里是树苗且 position 的最终
// 内容不再是泥土或草地时，树苗支撑失效，同 mutation 清除并掉落一个树苗。支撑格
// 读取变化后的最终值——同 tick 内多次写入以最后一次为准，被换回泥土/草地的支撑
// 不触发清除。上方格未加载（跨区块边界）时跳过：树苗所在区块必然已就绪才会被
// 生成，未就绪意味着整列已随区块卸载，没有可复核的权威状态（短草与火把复核
// 同款取舍）。
func (state *State) invalidateSaplingAbove(
	dimensionID core.DimensionID,
	position core.BlockPos,
	mutation *Mutation,
) {
	dimension := state.Dimension(dimensionID)
	if dimension == nil {
		return
	}
	above := core.BlockPos{X: position.X, Y: position.Y + 1, Z: position.Z}
	block, ready := dimension.BlockAt(above)
	if !ready || !core.IsSapling(block) {
		return
	}
	supportBlock, supportReady := dimension.BlockAt(position)
	if !supportReady || supportBlock == core.DirtID || supportBlock == core.GrassID {
		return
	}
	state.removeUnsupportedSapling(dimensionID, above, mutation)
}

// removeUnsupportedSapling 把失去支撑的树苗清为空气并掉落 1 个树苗。容量预检
// 先于写入：预检失败整次拒绝，树苗与区块状态逐字段保持不变。
func (state *State) removeUnsupportedSapling(
	dimensionID core.DimensionID,
	position core.BlockPos,
	mutation *Mutation,
) {
	dimension := state.Dimension(dimensionID)
	chunk, recordOK := dimension.ReadyChunk(position.Chunk())
	index, indexOK := world.ChunkBlockIndex(position)
	if !recordOK || !indexOK {
		return
	}
	slot, capacityOK := chunk.PrepareDrop(core.ItemSapling, index)
	if !capacityOK {
		return
	}
	_, changed, err := dimension.SetBlock(position, core.AirID)
	if err != nil || !changed {
		return
	}
	mutation.Record(dimensionID, position, core.AirID)
	chunk.CommitDrop(
		slot,
		core.ItemStack{Item: core.ItemSapling, Count: 1},
		index,
		state.environment.config.DropPickupDelayTicks,
	)
}

func torchSupportOffset(block core.BlockID) (core.BlockPos, bool) {
	switch block {
	case core.TorchStandingID:
		return core.BlockPos{Y: -1}, true
	case core.TorchWallPosXID:
		return core.BlockPos{X: -1}, true
	case core.TorchWallNegXID:
		return core.BlockPos{X: 1}, true
	case core.TorchWallPosZID:
		return core.BlockPos{Z: -1}, true
	case core.TorchWallNegZID:
		return core.BlockPos{Z: 1}, true
	default:
		return core.BlockPos{}, false
	}
}

func torchSupport(block core.BlockID, pos core.BlockPos) (core.BlockPos, bool) {
	offset, ok := torchSupportOffset(block)
	if !ok {
		return core.BlockPos{}, false
	}
	return core.BlockPos{
		X: pos.X + offset.X,
		Y: pos.Y + offset.Y,
		Z: pos.Z + offset.Z,
	}, true
}

func torchSupportBlockSolid(id core.BlockID) bool {
	// 火把支撑判定等价于原 `physics.BlockCollisionBoxes(id, true).Count > 0`：
	// 零碰撞的空气/流体/植物（作物与短草）/火把与门上半不计为实心，其余已注册方块（含玻璃、树叶、床、门下半等）均有碰撞体。
	// 与床的 `isSolidSupport` 区分：床要求不透明实心（排除全部门），火把仅排除上半。
	if id == core.AirID || core.IsFluid(id) || core.IsPlant(id) || core.IsTorch(id) || core.IsDoorUpper(id) {
		return false
	}
	return core.RegisteredBlock(id)
}

var torchNeighborOffsets = [6]core.BlockPos{
	{X: 1}, {X: -1},
	{Y: 1}, {Y: -1},
	{Z: 1}, {Z: -1},
}

type torchSweepCell struct {
	dimension core.DimensionID
	position  core.BlockPos
}

func (state *State) SweepUnsupportedTorches(mutation *Mutation) {
	changes := mutation.ChangedBlocks()
	if len(changes) == 0 {
		return
	}
	cells := make([]torchSweepCell, len(changes))
	for index, change := range changes {
		cells[index] = torchSweepCell{dimension: change.Dimension, position: change.Position}
	}
	for _, cell := range cells {
		state.invalidateTorchesSupportedBy(cell.dimension, cell.position, mutation)
	}
}

func (state *State) invalidateTorchesSupportedBy(
	dimensionID core.DimensionID,
	position core.BlockPos,
	mutation *Mutation,
) {
	dimension := state.Dimension(dimensionID)
	if dimension == nil {
		return
	}
	for _, offset := range torchNeighborOffsets {
		neighbor := core.BlockPos{
			X: position.X + offset.X,
			Y: position.Y + offset.Y,
			Z: position.Z + offset.Z,
		}
		block, ready := dimension.BlockAt(neighbor)
		if !ready || !core.IsTorch(block) {
			continue
		}
		support, ok := torchSupport(block, neighbor)
		if !ok || support != position {
			continue
		}
		supportBlock, supportReady := dimension.BlockAt(position)
		if supportReady && torchSupportBlockSolid(supportBlock) {
			continue
		}
		state.removeUnsupportedTorch(dimensionID, neighbor, mutation)
	}
}

func (state *State) removeUnsupportedTorch(
	dimensionID core.DimensionID,
	position core.BlockPos,
	mutation *Mutation,
) {
	dimension := state.Dimension(dimensionID)
	chunk, recordOK := dimension.ReadyChunk(position.Chunk())
	index, indexOK := world.ChunkBlockIndex(position)
	if !recordOK || !indexOK {
		return
	}
	slot, capacityOK := chunk.PrepareDrop(core.ItemTorch, index)
	if !capacityOK {
		return
	}
	_, changed, err := dimension.SetBlock(position, core.AirID)
	if err != nil || !changed {
		return
	}
	mutation.Record(dimensionID, position, core.AirID)
	chunk.CommitDrop(
		slot,
		core.ItemStack{Item: core.ItemTorch, Count: 1},
		index,
		state.environment.config.DropPickupDelayTicks,
	)
}

// Bed

func bedHalfPositions(target core.BlockPos, block core.BlockID) (core.BlockPos, core.BlockPos, bool) {
	dir := core.BedDir(block)
	if dir < 0 {
		return target, target, false
	}
	if core.IsBedFoot(block) {
		return target, core.BedHeadNeighbor(target, dir), true
	}
	foot := target
	switch dir {
	case 0:
		foot.Z--
	case 1:
		foot.X++
	case 2:
		foot.Z++
	case 3:
		foot.X--
	}
	return foot, target, true
}

func isSolidSupport(id core.BlockID) bool {
	// 床/门支撑判定：要求不透明实心（耕地特判为实心，全部门形态均不计，植物
	// ——作物与短草——为零碰撞非完整格也不计），与火把的零碰撞判定区分。
	return core.IsFarmland(id) || (core.RegisteredBlock(id) && id != core.AirID && id != core.GlassID && id != core.LeavesID && !core.IsFluid(id) && !core.IsPlant(id) && !core.IsDoor(id))
}

type bedSweepCell struct {
	dimension core.DimensionID
	position  core.BlockPos
}

func (state *State) SweepUnsupportedBeds(mutation *Mutation) {
	changes := mutation.ChangedBlocks()
	if len(changes) == 0 {
		return
	}
	cells := make([]bedSweepCell, len(changes))
	for index, change := range changes {
		cells[index] = bedSweepCell{dimension: change.Dimension, position: change.Position}
	}
	for _, cell := range cells {
		state.invalidateBedSupportedBy(cell.dimension, cell.position, mutation)
	}
}

func (state *State) invalidateBedSupportedBy(
	dimensionID core.DimensionID,
	position core.BlockPos,
	mutation *Mutation,
) {
	dimension := state.Dimension(dimensionID)
	if dimension == nil {
		return
	}
	above := core.BlockPos{X: position.X, Y: position.Y + 1, Z: position.Z}
	block, ready := dimension.BlockAt(above)
	if !ready || !core.IsBed(block) {
		return
	}
	supportBlock, supportReady := dimension.BlockAt(position)
	if !supportReady || isSolidSupport(supportBlock) {
		return
	}
	footPos, headPos, ok := bedHalfPositions(above, block)
	if !ok {
		return
	}
	if _, rejected := state.removeBedWithDrop(dimensionID, above, footPos, headPos, true, mutation); rejected {
		return
	}
}

func (state *State) clearBedPair(
	dimensionID core.DimensionID,
	footPos, headPos core.BlockPos,
	mutation *Mutation,
) bool {
	dimension := state.Dimension(dimensionID)
	oldFoot, _ := dimension.BlockAt(footPos)
	_, _, errFoot := dimension.SetBlock(footPos, core.AirID)
	if errFoot != nil {
		return false
	}
	if _, _, errHead := dimension.SetBlock(headPos, core.AirID); errHead != nil {
		_, _, _ = dimension.SetBlock(footPos, oldFoot)
		return false
	}
	mutation.Record(dimensionID, footPos, core.AirID)
	mutation.Record(dimensionID, headPos, core.AirID)
	return true
}

func (state *State) removeBedWithDrop(
	dimensionID core.DimensionID,
	dropPos, footPos, headPos core.BlockPos,
	drop bool,
	mutation *Mutation,
) (bool, bool) {
	dimension := state.Dimension(dimensionID)
	chunk, recordOK := dimension.ReadyChunk(dropPos.Chunk())
	index, indexOK := world.ChunkBlockIndex(dropPos)
	if !recordOK || !indexOK {
		return false, true
	}
	if _, ok := world.ChunkBlockIndex(footPos); !ok {
		return false, true
	}
	if _, ok := world.ChunkBlockIndex(headPos); !ok {
		return false, true
	}
	var next [core.DropsPerChunk]world.DropSlot
	if drop {
		stacks := [1]core.ItemStack{{Item: core.ItemBed, Count: 1}}
		var capacityOK bool
		next, capacityOK = chunk.PrepareDropBatch(
			stacks[:], index, state.environment.config.DropPickupDelayTicks,
		)
		if !capacityOK {
			return false, true
		}
	}
	if !state.clearBedPair(dimensionID, footPos, headPos, mutation) {
		return false, true
	}
	if drop {
		chunk.CommitDropBatch(next)
	}
	return true, false
}

// Support candidates for testing

type TorchSupportCandidate struct {
	Position core.BlockPos
	Support  core.BlockPos
	Block    core.BlockID
}

type BedSupportCandidate struct {
	Position core.BlockPos
	Support  core.BlockPos
	Block    core.BlockID
}

func (state *State) TorchSupportCandidates(mutation *Mutation) []TorchSupportCandidate {
	changes := mutation.ChangedBlocks()
	seen := make(map[core.BlockPos]struct{})
	var candidates []TorchSupportCandidate
	for _, change := range changes {
		for _, offset := range torchNeighborOffsets {
			neighbor := core.BlockPos{
				X: change.Position.X + offset.X,
				Y: change.Position.Y + offset.Y,
				Z: change.Position.Z + offset.Z,
			}
			if _, dup := seen[neighbor]; dup {
				continue
			}
			seen[neighbor] = struct{}{}
			dimension := state.Dimension(change.Dimension)
			if dimension == nil {
				continue
			}
			block, ready := dimension.BlockAt(neighbor)
			if !ready || !core.IsTorch(block) {
				continue
			}
			support, ok := torchSupport(block, neighbor)
			if !ok || support != change.Position {
				continue
			}
			candidates = append(candidates, TorchSupportCandidate{
				Position: neighbor,
				Support:  support,
				Block:    block,
			})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Support != candidates[j].Support {
			if candidates[i].Support.X != candidates[j].Support.X {
				return candidates[i].Support.X < candidates[j].Support.X
			}
			if candidates[i].Support.Y != candidates[j].Support.Y {
				return candidates[i].Support.Y < candidates[j].Support.Y
			}
			return candidates[i].Support.Z < candidates[j].Support.Z
		}
		if candidates[i].Position.X != candidates[j].Position.X {
			return candidates[i].Position.X < candidates[j].Position.X
		}
		if candidates[i].Position.Y != candidates[j].Position.Y {
			return candidates[i].Position.Y < candidates[j].Position.Y
		}
		return candidates[i].Position.Z < candidates[j].Position.Z
	})
	return candidates
}

func (state *State) BedSupportCandidates(mutation *Mutation) []BedSupportCandidate {
	changes := mutation.ChangedBlocks()
	seen := make(map[core.BlockPos]struct{})
	var candidates []BedSupportCandidate
	for _, change := range changes {
		above := core.BlockPos{X: change.Position.X, Y: change.Position.Y + 1, Z: change.Position.Z}
		if _, dup := seen[above]; dup {
			continue
		}
		seen[above] = struct{}{}
		dimension := state.Dimension(change.Dimension)
		if dimension == nil {
			continue
		}
		block, ready := dimension.BlockAt(above)
		if !ready || !core.IsBed(block) {
			continue
		}
		candidates = append(candidates, BedSupportCandidate{
			Position: above,
			Support:  change.Position,
			Block:    block,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Support != candidates[j].Support {
			if candidates[i].Support.X != candidates[j].Support.X {
				return candidates[i].Support.X < candidates[j].Support.X
			}
			if candidates[i].Support.Y != candidates[j].Support.Y {
				return candidates[i].Support.Y < candidates[j].Support.Y
			}
			return candidates[i].Support.Z < candidates[j].Support.Z
		}
		if candidates[i].Position.X != candidates[j].Position.X {
			return candidates[i].Position.X < candidates[j].Position.X
		}
		if candidates[i].Position.Y != candidates[j].Position.Y {
			return candidates[i].Position.Y < candidates[j].Position.Y
		}
		return candidates[i].Position.Z < candidates[j].Position.Z
	})
	return candidates
}

// Crop stats helpers

func (state *State) CropStats() (examined int, reads int) {
	return state.environment.cropCellsExamined, state.environment.cropBlockReads
}

func (state *State) FarmlandMoistureStats() (candidates int, reads int) {
	m := state.environment.farmlandMoisture
	return m.candidateInspections, m.blockReads
}

func (state *State) FarmlandBlockReads() int { return state.environment.farmlandMoisture.blockReads }
func (state *State) FarmlandCandidateInspections() int {
	return state.environment.farmlandMoisture.candidateInspections
}
func (state *State) FarmlandRescanCursor() int {
	return state.environment.farmlandMoisture.rescans.cursor
}
func (state *State) FarmlandRescanPendingLen() int {
	return len(state.environment.farmlandMoisture.rescans.pending)
}

// FarmlandQueued 报告 (dim, pos) 是否有排队的湿度候选（FarmlandMoisture 域）。
func (state *State) FarmlandQueued(dim core.DimensionID, pos core.BlockPos) bool {
	_, ok := state.FarmlandMoistureDueTick(dim, pos)
	return ok
}

// FarmlandMoistureDueTick 返回 (dim, pos) 当前湿度候选的到期 tick；未在队返回
// false。供测试断言「只提前不推迟」的落位值，生产热路径不调用。
func (state *State) FarmlandMoistureDueTick(dim core.DimensionID, pos core.BlockPos) (uint64, bool) {
	queue := state.environment.fluidQueues[dim]
	if queue == nil {
		return 0, false
	}
	return queue.Scheduler().DueTick(pos, updates.KindFarmlandMoisture)
}

func (state *State) FarmlandRescanPending() []core.ChunkKey {
	return append([]core.ChunkKey(nil), state.environment.farmlandMoisture.rescans.pending...)
}

// FarmlandMoisturePendingLen 返回全部维度 FarmlandMoisture 域的排队候选总数
// （求和与 map 遍历顺序无关，结果确定）。
func (state *State) FarmlandMoisturePendingLen() int {
	total := 0
	for _, queue := range state.environment.fluidQueues {
		total += queue.Scheduler().LenOf(updates.KindFarmlandMoisture)
	}
	return total
}

// ResetFarmlandMoisture 清空湿度域的全部状态：各维度调度器上的 FarmlandMoisture
// 待办、计量与全块重扫队列（流体域待办不受影响——测试夹具用它隔离湿度域）。
func (state *State) ResetFarmlandMoisture() {
	state.environment.farmlandMoisture = farmlandMoistureState{}
	for _, queue := range state.environment.fluidQueues {
		queue.Scheduler().ClearKind(updates.KindFarmlandMoisture)
	}
}
