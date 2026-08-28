package sim

import (
	"slices"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
)

type pendingChunkChanges = realm.Mutation

func (engine *Engine) newMutation() *pendingChunkChanges {
	return engine.realm.NewMutation()
}

func (engine *Engine) recordChange(
	dimensionID core.DimensionID,
	position core.BlockPos,
	block core.BlockID,
	pending *pendingChunkChanges,
) {
	pending.Record(dimensionID, position, block)
	engine.realm.EnqueueFluidUpdate(dimensionID, position)
}

func (engine *Engine) finishChanges(pending *pendingChunkChanges, result *TickResult) {
	for _, batch := range pending.Commit() {
		changes := make([]BlockChange, len(batch.Changes))
		for index, change := range batch.Changes {
			changes[index] = BlockChange{Position: change.Position, Block: change.Block}
		}
		result.Changes = append(result.Changes, ChunkChangeBatch{
			Dimension:    batch.Dimension,
			Chunk:        batch.Chunk,
			BaseRevision: batch.BaseRevision,
			NewRevision:  batch.NewRevision,
			Changes:      changes,
		})
	}
}

// sortChunkKeys 用泛型排序避免 sort.Slice 的反射 swapper 分配，
// 使权威 tick 的热路径保持零分配。
func sortChunkKeys(keys []core.ChunkKey) {
	slices.SortFunc(keys, func(left, right core.ChunkKey) int {
		switch {
		case chunkKeyLess(left, right):
			return -1
		case chunkKeyLess(right, left):
			return 1
		default:
			return 0
		}
	})
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
