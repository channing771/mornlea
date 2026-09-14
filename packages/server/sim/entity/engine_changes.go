package entity

import (
	"slices"

	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
)

type pendingChunkChanges = realm.Mutation

// recordChange 是 entity 侧全部权威方块写入的唯一汇入点：先登记本 tick 的
// 变更批次，再把该写入的定时面反activate交给 realm 的统一入队门面——流体域
// 7 格（目标格 + 6 面邻域）、流体成员变化的湿窗口、新造耕地的单格湿度候选，
// 全部由 (old, block) 的方块类别派生。调用方必须传入写前旧值 old（门面只认
// 类别变化、不回读世界）；写入方不得绕过本入口自行挑入队面，唯一性由
// enqueue_entry_guard_test.go 守卫。
func (engine *engineContext) recordChange(
	dimensionID core.DimensionID,
	position core.BlockPos,
	old core.BlockID,
	block core.BlockID,
	pending *pendingChunkChanges,
) {
	pending.Record(dimensionID, position, block)
	engine.realm.EnqueueBlockWrite(dimensionID, position, old, block)
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
