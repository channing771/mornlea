package sim

import "github.com/channing771/mornlea/internal/core"

const (
	// farmlandWetRadius 是判定湿润时向四周扫描的水平切比雪夫距离，4 对应 9×9
	// 的水平窗口。
	farmlandWetRadius = 4
	// farmlandWetLayersAbove 是除耕地自身所在层外向上扫描的层数；取 1 表示只读
	// 同层与上一层。
	farmlandWetLayersAbove            = 1
	farmlandMoistureCandidatesPerTick = 65_536
	farmlandMoistureReadsPerTick      = 65_536
	farmlandWetNeighborReads          = (2*farmlandWetRadius + 1) * (2*farmlandWetRadius + 1) * (farmlandWetLayersAbove + 1)
	farmlandMoistureRescanSide        = core.SectionSize + 2*farmlandWetRadius
	farmlandMoistureRescanCells       = farmlandMoistureRescanSide * farmlandMoistureRescanSide * core.SectionsPerChunk * core.SectionSize
)

type farmlandMoistureKey struct {
	dimension core.DimensionID
	position  core.BlockPos
}

// farmlandMoistureRescanState 持有独立于流体重扫的湿度恢复队列。
type farmlandMoistureRescanState struct {
	pending []core.ChunkKey
	queued  map[core.ChunkKey]struct{}
	cursor  int
}

// enqueueChunk 按区块首次进入 active Ready 范围的顺序登记重扫 job。
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

// dropOutOfScope 丢弃离开 active Ready 范围的 job；队首变化时游标归零。
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

// pop 删除完成或失效的队首 job，并复位只属于该队首的游标。
func (state *farmlandMoistureRescanState) pop() {
	key := state.pending[0]
	delete(state.queued, key)
	state.pending = append(state.pending[:0], state.pending[1:]...)
	state.cursor = 0
}

// farmlandMoistureState 是候选 FIFO、去重集合与恢复重扫的单写者状态。
type farmlandMoistureState struct {
	pending              []farmlandMoistureKey
	head                 int
	queued               map[farmlandMoistureKey]struct{}
	rescans              farmlandMoistureRescanState
	candidateInspections int
	blockReads           int
}

// pop 删除当前队首，并在消费前缀足够大时以 O(1) rebase 丢弃该前缀。
func (state *farmlandMoistureState) pop() {
	key := state.pending[state.head]
	delete(state.queued, key)
	state.head++
	if state.head == len(state.pending) {
		state.pending = state.pending[:0]
		state.head = 0
		return
	}
	if state.head >= 4096 && state.head*2 >= len(state.pending) {
		state.pending = state.pending[state.head:]
		state.head = 0
	}
}

// enqueueFarmlandMoisture 按首次出现顺序登记一个世界高度内的候选。
func (engine *Engine) enqueueFarmlandMoisture(
	dimension core.DimensionID,
	position core.BlockPos,
) {
	if position.Y < core.MinY || position.Y >= core.MaxY {
		return
	}
	state := &engine.farmlandMoisture
	if state.queued == nil {
		state.queued = make(map[farmlandMoistureKey]struct{})
	}
	key := farmlandMoistureKey{dimension: dimension, position: position}
	if _, exists := state.queued[key]; exists {
		return
	}
	state.queued[key] = struct{}{}
	state.pending = append(state.pending, key)
}

// enqueueFarmlandMoistureAroundFluid 按 `y,z,x` 顺序登记可能受流体格影响的耕地。
func (engine *Engine) enqueueFarmlandMoistureAroundFluid(
	dimension core.DimensionID,
	position core.BlockPos,
) {
	for y := position.Y - farmlandWetLayersAbove; y <= position.Y; y++ {
		for z := position.Z - farmlandWetRadius; z <= position.Z+farmlandWetRadius; z++ {
			for x := position.X - farmlandWetRadius; x <= position.X+farmlandWetRadius; x++ {
				engine.enqueueFarmlandMoisture(dimension, core.BlockPos{X: x, Y: y, Z: z})
			}
		}
	}
}

// farmlandIsWet 按固定顺序查询湿润邻域，并记录每次规则方块读取。
func (engine *Engine) farmlandIsWet(dimension *Dimension, position core.BlockPos) bool {
	state := &engine.farmlandMoisture
	for dy := int32(0); dy <= farmlandWetLayersAbove; dy++ {
		for dz := int32(-farmlandWetRadius); dz <= farmlandWetRadius; dz++ {
			for dx := int32(-farmlandWetRadius); dx <= farmlandWetRadius; dx++ {
				block, ready := dimension.BlockAt(core.BlockPos{
					X: position.X + dx,
					Y: position.Y + dy,
					Z: position.Z + dz,
				})
				state.blockReads++
				if ready && core.IsFluid(block) {
					return true
				}
			}
		}
	}
	return false
}

// farmlandMoistureRescanPosition 按 `y,z,x` 顺序还原完整高度 halo 的游标。
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

// runFarmlandMoistureRescans 用事件候选留下的预算推进独立恢复重扫。
func (engine *Engine) runFarmlandMoistureRescans(budget int) {
	state := &engine.farmlandMoisture.rescans
	state.dropOutOfScope(engine.fluidScope)
	for budget > 0 && len(state.pending) > 0 {
		key := state.pending[0]
		dimension := engine.dimensions[key.Dimension]
		if dimension == nil {
			state.pop()
			continue
		}
		for budget > 0 && state.cursor < farmlandMoistureRescanCells {
			position := farmlandMoistureRescanPosition(key, state.cursor)
			block, ready := dimension.BlockAt(position)
			engine.farmlandMoisture.blockReads++
			budget--
			state.cursor++
			if !ready || !core.IsFarmland(block) {
				continue
			}
			farmlandKey := core.ChunkKey{Dimension: key.Dimension, Pos: position.Chunk()}
			if _, active := engine.fluidScope[farmlandKey]; active {
				engine.enqueueFarmlandMoisture(key.Dimension, position)
			}
		}
		if state.cursor < farmlandMoistureRescanCells {
			return
		}
		state.pop()
	}
}

// advanceFarmlandMoisture 在独立的候选检查与读取预算内按 FIFO 处理湿度候选。
func (engine *Engine) advanceFarmlandMoisture(
	pending *pendingChunkChanges,
) {
	state := &engine.farmlandMoisture
	state.candidateInspections = 0
	state.blockReads = 0
	for state.candidateInspections < farmlandMoistureCandidatesPerTick &&
		state.blockReads < farmlandMoistureReadsPerTick && state.head < len(state.pending) {
		key := state.pending[state.head]
		state.candidateInspections++
		chunkKey := core.ChunkKey{Dimension: key.dimension, Pos: key.position.Chunk()}
		if _, active := engine.fluidScope[chunkKey]; !active {
			state.pop()
			continue
		}
		dimension := engine.dimensions[key.dimension]
		if dimension == nil {
			state.pop()
			continue
		}
		block, ready := dimension.BlockAt(key.position)
		state.blockReads++
		if !ready || !core.IsFarmland(block) {
			state.pop()
			continue
		}
		if farmlandMoistureReadsPerTick-state.blockReads < farmlandWetNeighborReads {
			break
		}
		next := core.FarmlandDryID
		if engine.farmlandIsWet(dimension, key.position) {
			next = core.FarmlandWetID
		}
		if next != block {
			if _, changed, err := dimension.SetBlock(key.position, next); err == nil && changed {
				engine.recordChange(key.dimension, key.position, next, pending)
			}
		}
		state.pop()
	}
	engine.runFarmlandMoistureRescans(farmlandMoistureReadsPerTick - state.blockReads)
}
