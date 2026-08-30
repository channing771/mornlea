package entity

import (
	"errors"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/world"
)

func (engine *Engine) executePlacement(
	command Command,
	mutation *realm.Mutation,
) (RejectReason, bool) {
	session := engine.sessions[command.Session]
	if command.Kind != CommandPlaceBlock {
		return RejectInvalidRay, true
	}
	if session == nil || session.player == nil ||
		session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	dimensionID := session.dimension
	origin := session.player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	direction := LookDirection(command.Yaw, command.Pitch)
	dimension := engine.dimension(dimensionID)
	originBlock := core.BlockPos{
		X: int32(math.Floor(float64(origin.X()))),
		Y: int32(math.Floor(float64(origin.Y()))),
		Z: int32(math.Floor(float64(origin.Z()))),
	}
	if !session.hasView || dimension == nil {
		return RejectInvalidRay, true
	}
	originKey := core.ChunkKey{
		Dimension: dimensionID,
		Pos:       originBlock.Chunk(),
	}
	if _, subscribed := session.wanted[originKey]; !subscribed {
		return RejectInvalidRay, true
	}
	player := session.player
	if command.Slot >= core.HotbarSlots {
		return RejectInvalidSlot, true
	}
	item := player.inventory.Hotbar.Slots[command.Slot].Item
	if _, ok := core.ItemPlacement(item); !ok && item != core.ItemTorch {
		return RejectInvalidBlock, true
	}
	consumed, ok := player.inventory.Hotbar.Consume(command.Slot)
	if !ok {
		return RejectInvalidBlock, true
	}

	hit, ok, err := core.RaycastBlocks(
		origin,
		direction,
		engine.tunables.InteractionReach,
		blockRaycastSampler(dimension),
	)
	if err != nil {
		if errors.Is(err, ErrChunkNotReady) {
			return RejectChunkNotReady, true
		}
		return RejectInvalidRay, true
	}
	if !ok {
		return RejectNoTarget, true
	}

	if hit.Face == core.BlockFaceNone {
		return RejectOccupied, true
	}
	placement, placeable := core.PlaceableBlockAtFace(item, hit.Face)
	if !placeable {
		return RejectInvalidBlock, true
	}
	target := adjacentBlock(hit.Block, hit.Face)
	if target.Y < core.MinY || target.Y >= core.MaxY {
		return RejectChunkNotReady, true
	}
	block, ready := dimension.BlockAt(target)
	if !ready {
		return RejectChunkNotReady, true
	}
	occupied := (block != core.AirID && !core.IsFluid(block)) || placementOverlapsPlayer(
		placement,
		target,
		player.state.Position,
	)
	if occupied {
		return RejectOccupied, true
	}
	if core.IsDoor(placement) {
		dir := yawToDoorDir(command.Yaw)
		reason, rejected := engine.tryPlaceDoor(dimensionID, target, dir, mutation)
		if rejected {
			return reason, true
		}
		player.inventory.Hotbar = consumed
		player.inventoryDirty = true
		return 0, false
	}
	if core.IsBed(placement) {
		dir := yawToDoorDir(command.Yaw)
		reason, rejected := engine.tryPlaceBed(dimensionID, target, dir, mutation)
		if rejected {
			return reason, true
		}
		player.inventory.Hotbar = consumed
		player.inventoryDirty = true
		return 0, false
	}
	if core.IsCrop(placement) {
		if block != core.AirID {
			return RejectInvalidBlock, true
		}
		below := target
		below.Y--
		belowBlock, belowReady := dimension.BlockAt(below)
		if !belowReady {
			return RejectChunkNotReady, true
		}
		if !core.IsFarmland(belowBlock) {
			return RejectInvalidBlock, true
		}
	}
	if core.IsTorch(placement) {
		if core.IsFluid(block) {
			return RejectInvalidBlock, true
		}
		if torchCellOverlapsPlayer(target, player.state.Position) {
			return RejectOccupied, true
		}
		support, hasSupport := torchSupport(placement, target)
		if !hasSupport {
			return RejectInvalidBlock, true
		}
		supportBlock, supportReady := dimension.BlockAt(support)
		if !supportReady {
			return RejectChunkNotReady, true
		}
		if !torchSupportBlockSolid(supportBlock) {
			return RejectInvalidBlock, true
		}
	}
	targetChunk, targetOK := dimension.ReadyChunk(target.Chunk())
	targetIndex, targetIndexed := world.ChunkBlockIndex(target)
	furnaceSlot, reserveFurnace := -1, false
	chestSlot, reserveChest := -1, false
	if placement == core.FurnaceID {
		if !targetOK || !targetIndexed {
			return RejectChunkNotReady, true
		}
		slot, ok := targetChunk.PrepareFurnace(targetIndex)
		if !ok {
			return RejectContainerCapacity, true
		}
		furnaceSlot, reserveFurnace = slot, true
	}
	if placement == core.ChestID {
		if !targetOK || !targetIndexed {
			return RejectChunkNotReady, true
		}
		slot, ok := targetChunk.PrepareChest(targetIndex)
		if !ok {
			return RejectContainerCapacity, true
		}
		chestSlot, reserveChest = slot, true
	}
	_, changed, setErr := dimension.SetBlock(target, placement)
	if setErr != nil {
		return mapSetBlockError(setErr), true
	}
	if changed {
		engine.recordChange(
			dimensionID,
			target,
			placement,
			mutation,
		)
		if core.IsFluid(block) != core.IsFluid(placement) {
			engine.enqueueFarmlandMoistureAroundFluid(dimensionID, target)
		}
		if reserveFurnace {
			targetChunk.CommitFurnace(furnaceSlot, targetIndex)
		}
		if reserveChest {
			targetChunk.CommitChest(chestSlot, targetIndex)
		}
		player.inventory.Hotbar = consumed
		player.inventoryDirty = true
	}
	return 0, false
}

func (engine *Engine) enqueueFarmlandMoistureAroundFluid(dimensionID core.DimensionID, pos core.BlockPos) {
	// 放置导致流体格与非流体格互换时，复用与 `sim` 相同的湿度候选入队路径：
	// 委托至 `realm` 的环境状态，避免在 `entity` 另建一套入队逻辑。
	engine.realm.EnqueueFarmlandMoistureAroundFluid(dimensionID, pos)
}
