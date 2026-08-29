package entity

import (
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/world"
)

func (engine *Engine) tryPlaceBed(dimensionID core.DimensionID, foot core.BlockPos, dir int, mutation *realm.Mutation) (RejectReason, bool) {
	if dir < 0 || dir > 3 {
		return RejectInvalidBlock, true
	}
	head := core.BedHeadNeighbor(foot, dir)
	if foot.Y < core.MinY || foot.Y >= core.MaxY {
		return RejectChunkNotReady, true
	}
	dimension := engine.dimension(dimensionID)
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	footBlock, footReady := dimension.BlockAt(foot)
	if !footReady {
		return RejectChunkNotReady, true
	}
	headBlock, headReady := dimension.BlockAt(head)
	if !headReady {
		return RejectChunkNotReady, true
	}
	if footBlock != core.AirID || headBlock != core.AirID {
		return RejectOccupied, true
	}
	footBelow, footBelowReady := dimension.BlockAt(core.BlockPos{X: foot.X, Y: foot.Y - 1, Z: foot.Z})
	if !footBelowReady {
		return RejectChunkNotReady, true
	}
	headBelow, headBelowReady := dimension.BlockAt(core.BlockPos{X: head.X, Y: head.Y - 1, Z: head.Z})
	if !headBelowReady {
		return RejectChunkNotReady, true
	}
	if !isSolidSupport(footBelow) || !isSolidSupport(headBelow) {
		return RejectInvalidBlock, true
	}
	footID := core.BedFootID(dir)
	headID := core.BedHeadID(dir)
	oldFoot, _, errFoot := dimension.SetBlock(foot, footID)
	if errFoot != nil {
		return mapSetBlockError(errFoot), true
	}
	_, _, errHead := dimension.SetBlock(head, headID)
	if errHead != nil {
		_, _, _ = dimension.SetBlock(foot, oldFoot)
		return mapSetBlockError(errHead), true
	}
	engine.recordChange(dimensionID, foot, footID, mutation)
	engine.recordChange(dimensionID, head, headID, mutation)
	return 0, false
}

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

func (engine *Engine) clearBedPair(
	dimensionID core.DimensionID,
	footPos, headPos core.BlockPos,
	mutation *realm.Mutation,
) (RejectReason, bool) {
	dimension := engine.dimension(dimensionID)
	oldFoot, _ := dimension.BlockAt(footPos)
	_, _, errFoot := dimension.SetBlock(footPos, core.AirID)
	if errFoot != nil {
		return mapSetBlockError(errFoot), true
	}
	if _, _, errHead := dimension.SetBlock(headPos, core.AirID); errHead != nil {
		_, _, _ = dimension.SetBlock(footPos, oldFoot)
		return mapSetBlockError(errHead), true
	}
	engine.recordChange(dimensionID, footPos, core.AirID, mutation)
	engine.recordChange(dimensionID, headPos, core.AirID, mutation)
	return 0, false
}

func (engine *Engine) removeBedWithDrop(
	dimensionID core.DimensionID,
	dropPos, footPos, headPos core.BlockPos,
	drop bool,
	mutation *realm.Mutation,
) (RejectReason, bool) {
	dimension := engine.dimension(dimensionID)
	chunk, recordOK := dimension.ReadyChunk(dropPos.Chunk())
	index, indexOK := world.ChunkBlockIndex(dropPos)
	if !recordOK || !indexOK {
		return RejectChunkNotReady, true
	}
	if _, ok := world.ChunkBlockIndex(footPos); !ok {
		return RejectChunkNotReady, true
	}
	if _, ok := world.ChunkBlockIndex(headPos); !ok {
		return RejectChunkNotReady, true
	}
	var next [core.DropsPerChunk]world.DropSlot
	if drop {
		stacks := [1]core.ItemStack{{Item: core.ItemBed, Count: 1}}
		var capacityOK bool
		next, capacityOK = chunk.PrepareDropBatch(
			stacks[:], index, engine.tunables.DropPickupDelayTicks,
		)
		if !capacityOK {
			return RejectDropCapacity, true
		}
	}
	if reason, rejected := engine.clearBedPair(dimensionID, footPos, headPos, mutation); rejected {
		return reason, true
	}
	if drop {
		chunk.CommitDropBatch(next)
	}
	return 0, false
}

type bedSweepCell struct {
	dimension core.DimensionID
	position  core.BlockPos
}

func (engine *Engine) sweepUnsupportedBeds(
	mutation *realm.Mutation,
) {
	engine.realm.SweepUnsupportedBeds(mutation)
}

func (engine *Engine) invalidateBedSupportedBy(
	dimensionID core.DimensionID,
	position core.BlockPos,
	mutation *realm.Mutation,
) {
	dimension := engine.dimension(dimensionID)
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
	if _, rejected := engine.removeBedWithDrop(dimensionID, above, footPos, headPos, true, mutation); rejected {
		return
	}
}
