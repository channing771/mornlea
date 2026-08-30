package entity

import (
	"errors"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
)

func doorLowerID(dir int, open bool) core.BlockID {
	switch dir {
	case 0:
		if open {
			return core.DoorLowerSouthOpen
		}
		return core.DoorLowerSouthClosed
	case 1:
		if open {
			return core.DoorLowerWestOpen
		}
		return core.DoorLowerWestClosed
	case 2:
		if open {
			return core.DoorLowerNorthOpen
		}
		return core.DoorLowerNorthClosed
	case 3:
		if open {
			return core.DoorLowerEastOpen
		}
		return core.DoorLowerEastClosed
	default:
		return core.AirID
	}
}

func isSolidSupport(id core.BlockID) bool {
	return core.IsFarmland(id) || (core.RegisteredBlock(id) && id != core.AirID && id != core.GlassID && id != core.LeavesID && !core.IsFluid(id) && !core.IsCrop(id) && !core.IsDoor(id))
}

func (engine *Engine) tryPlaceDoor(dimensionID core.DimensionID, lower core.BlockPos, dir int, mutation *realm.Mutation) (RejectReason, bool) {
	if dir < 0 || dir > 3 {
		return RejectInvalidBlock, true
	}
	upper := core.BlockPos{X: lower.X, Y: lower.Y + 1, Z: lower.Z}
	if lower.Y < core.MinY || lower.Y >= core.MaxY || upper.Y < core.MinY || upper.Y >= core.MaxY {
		return RejectChunkNotReady, true
	}
	dimension := engine.dimension(dimensionID)
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	lowerBlock, lowerReady := dimension.BlockAt(lower)
	if !lowerReady {
		return RejectChunkNotReady, true
	}
	upperBlock, upperReady := dimension.BlockAt(upper)
	if !upperReady {
		return RejectChunkNotReady, true
	}
	if lowerBlock != core.AirID || upperBlock != core.AirID {
		return RejectOccupied, true
	}
	below := core.BlockPos{X: lower.X, Y: lower.Y - 1, Z: lower.Z}
	belowBlock, belowReady := dimension.BlockAt(below)
	if !belowReady {
		return RejectChunkNotReady, true
	}
	if !isSolidSupport(belowBlock) {
		return RejectInvalidBlock, true
	}
	lowerID := doorLowerID(dir, false)
	upperID := core.DoorUpper
	oldLower, _, errLower := dimension.SetBlock(lower, lowerID)
	if errLower != nil {
		return mapSetBlockError(errLower), true
	}
	oldUpper, _, errUpper := dimension.SetBlock(upper, upperID)
	if errUpper != nil {
		_, _, _ = dimension.SetBlock(lower, oldLower)
		return mapSetBlockError(errUpper), true
	}
	engine.recordChange(dimensionID, lower, lowerID, mutation)
	engine.recordChange(dimensionID, upper, upperID, mutation)
	_ = oldLower
	_ = oldUpper
	return 0, false
}

func handleInteractDoor(engine *Engine, dimensionID core.DimensionID, pos core.BlockPos, mutation *realm.Mutation) bool {
	dimension := engine.dimension(dimensionID)
	if dimension == nil {
		return false
	}
	block, ready := dimension.BlockAt(pos)
	if !ready {
		return false
	}
	var lowerPos core.BlockPos
	switch {
	case core.IsDoorLower(block):
		lowerPos = pos
	case core.IsDoorUpper(block):
		lowerPos = core.BlockPos{X: pos.X, Y: pos.Y - 1, Z: pos.Z}
	default:
		return false
	}
	lowerID, lowerReady := dimension.BlockAt(lowerPos)
	if !lowerReady || !core.IsDoorLower(lowerID) {
		return false
	}
	upperPos := core.BlockPos{X: lowerPos.X, Y: lowerPos.Y + 1, Z: lowerPos.Z}
	upperID, upperReady := dimension.BlockAt(upperPos)
	if !upperReady || !core.IsDoorUpper(upperID) {
		return false
	}
	dir := core.DoorDir(lowerID)
	if dir < 0 {
		return false
	}
	open := core.IsDoorOpen(lowerID)
	newLower := doorLowerID(dir, !open)
	_, _, err := dimension.SetBlock(lowerPos, newLower)
	if err != nil {
		return false
	}
	engine.recordChange(dimensionID, lowerPos, newLower, mutation)
	return true
}

func (engine *Engine) executeInteractDoor(command Command, mutation *realm.Mutation) (RejectReason, bool) {
	session := engine.sessions[command.Session]
	if session == nil || session.player == nil || session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	dimensionID := session.dimension
	dimension := engine.dimension(dimensionID)
	if dimension == nil || !session.hasView {
		return RejectInvalidRay, true
	}
	origin := session.player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	direction := LookDirection(command.Yaw, command.Pitch)
	hit, ok, err := core.RaycastBlocks(origin, direction, engine.tunables.InteractionReach, blockRaycastSampler(dimension))
	if err != nil {
		if errors.Is(err, ErrChunkNotReady) {
			return RejectChunkNotReady, true
		}
		return RejectInvalidRay, true
	}
	if !ok {
		return RejectNoTarget, true
	}
	block, ready := dimension.BlockAt(hit.Block)
	if !ready {
		return RejectChunkNotReady, true
	}
	if !core.IsDoor(block) {
		return 0, false
	}
	if !handleInteractDoor(engine, dimensionID, hit.Block, mutation) {
		return RejectNoTarget, true
	}
	return 0, false
}
