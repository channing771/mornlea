package realm

import (
	"sort"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

type BlockChange struct {
	Position core.BlockPos
	Block    core.BlockID
}

type ChunkChangeBatch struct {
	Dimension    core.DimensionID
	Chunk        core.ChunkPos
	BaseRevision uint64
	NewRevision  uint64
	Changes      []BlockChange
}

type pendingChunkChanges struct {
	baseRevision uint64
	changes      map[uint32]BlockChange
	dirty        map[int]struct{}
}

// Mutation 收集一个权威 tick 内全部区块变更，并只在 Commit 时推进 revision。
type Mutation struct {
	state     *State
	pending   map[core.ChunkKey]*pendingChunkChanges
	committed bool
}

type ChangedBlock struct {
	Dimension core.DimensionID
	Position  core.BlockPos
}

func (state *State) NewMutation() *Mutation {
	return &Mutation{state: state, pending: make(map[core.ChunkKey]*pendingChunkChanges)}
}

func (mutation *Mutation) Record(dimensionID core.DimensionID, position core.BlockPos, block core.BlockID) {
	key := core.ChunkKey{Dimension: dimensionID, Pos: position.Chunk()}
	changeSet := mutation.pending[key]
	if changeSet == nil {
		record := mutation.state.dimensions[dimensionID].records[key.Pos]
		changeSet = &pendingChunkChanges{
			baseRevision: record.Revision,
			changes:      make(map[uint32]BlockChange),
			dirty:        make(map[int]struct{}),
		}
		mutation.pending[key] = changeSet
	}
	index, ok := world.ChunkBlockIndex(position)
	if !ok {
		panic("sim: changed block has no chunk index")
	}
	changeSet.changes[index] = BlockChange{Position: position, Block: block}
	changeSet.dirty[position.SectionIndex()] = struct{}{}
}

// Touch 为不改方块的区块状态变化登记 revision barrier。
func (mutation *Mutation) Touch(key core.ChunkKey) {
	if mutation.pending[key] != nil {
		return
	}
	record := mutation.state.dimensions[key.Dimension].records[key.Pos]
	mutation.pending[key] = &pendingChunkChanges{
		baseRevision: record.Revision,
		changes:      make(map[uint32]BlockChange),
		dirty:        make(map[int]struct{}),
	}
}

func (mutation *Mutation) Len() int {
	return len(mutation.pending)
}

func (mutation *Mutation) Has(key core.ChunkKey) bool {
	return mutation.pending[key] != nil
}

// ChangedBlocks 返回本次事务开始提交前的稳定方块变更快照。
func (mutation *Mutation) ChangedBlocks() []ChangedBlock {
	keys := mutation.sortedKeys()
	changes := make([]ChangedBlock, 0)
	for _, key := range keys {
		changeSet := mutation.pending[key]
		for _, index := range sortedIndices(changeSet.changes) {
			changes = append(changes, ChangedBlock{Dimension: key.Dimension, Position: changeSet.changes[index].Position})
		}
	}
	return changes
}

func (mutation *Mutation) Commit() []ChunkChangeBatch {
	if mutation.committed {
		return nil
	}
	mutation.committed = true
	keys := mutation.sortedKeys()
	batches := make([]ChunkChangeBatch, 0, len(keys))
	for _, key := range keys {
		changeSet := mutation.pending[key]
		dimension := mutation.state.dimensions[key.Dimension]
		record := dimension.records[key.Pos]
		for sectionIndex := range changeSet.dirty {
			record.Chunk.Section(sectionIndex).Blocks.Compact()
		}
		record.Revision++
		// section 压缩与 revision 推进同时落地：估算缓存键随之失效并重算。
		dimension.refreshRecord(key.Pos, record)

		indices := sortedIndices(changeSet.changes)
		changes := make([]BlockChange, 0, len(indices))
		for _, index := range indices {
			changes = append(changes, changeSet.changes[index])
		}
		batches = append(batches, ChunkChangeBatch{
			Dimension:    key.Dimension,
			Chunk:        key.Pos,
			BaseRevision: changeSet.baseRevision,
			NewRevision:  record.Revision,
			Changes:      changes,
		})
	}
	return batches
}

func (mutation *Mutation) sortedKeys() []core.ChunkKey {
	keys := make([]core.ChunkKey, 0, len(mutation.pending))
	for key := range mutation.pending {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return chunkKeyLess(keys[i], keys[j]) })
	return keys
}

func sortedIndices(changes map[uint32]BlockChange) []uint32 {
	indices := make([]uint32, 0, len(changes))
	for index := range changes {
		indices = append(indices, index)
	}
	sort.Slice(indices, func(i, j int) bool { return indices[i] < indices[j] })
	return indices
}

func chunkKeyLess(left, right core.ChunkKey) bool {
	if left.Dimension != right.Dimension {
		return left.Dimension < right.Dimension
	}
	if left.Pos.X != right.Pos.X {
		return left.Pos.X < right.Pos.X
	}
	return left.Pos.Z < right.Pos.Z
}
