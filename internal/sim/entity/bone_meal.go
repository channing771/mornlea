package entity

import (
	"errors"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// executeBoneMeal 处理一条骨粉催熟命令：把视线内的未成熟作物推进一阶段，并从
// 权威选中的骨粉扣减恰好一个。
//
// 校验顺序与 executeTillSoil 同形，结构上保证拒绝路径零消耗零写入。
func (engine *engineContext) executeBoneMeal(
	command Command,
	pending *pendingChunkChanges,
) (RejectReason, bool) {
	session := engine.sessions[command.Session]
	if session == nil || session.player == nil ||
		session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	dimension := engine.dimension(session.dimension)
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	player := session.player
	origin := player.state.Position.
		Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	hit, ok, err := core.RaycastBlocks(
		origin,
		LookDirection(command.Yaw, command.Pitch),
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
	block, ready := dimension.BlockAt(hit.Block)
	if !ready {
		return RejectChunkNotReady, true
	}
	// 只有未成熟作物可被催熟；成熟与非作物均拒绝。
	if !core.IsCrop(block) || core.CropStage(block) == 7 {
		return RejectInvalidBlock, true
	}
	// 手持物按权威选中格解析。
	selected := player.inventory.Hotbar.Slots[player.inventory.Hotbar.Selected]
	if selected.Item != core.ItemBoneMeal || selected.Count == 0 {
		return RejectInvalidBlock, true
	}
	// 预演扣除：在全部校验通过后、写入前准备消耗副本，写入成功后再提交。
	consumed, ok := player.inventory.Hotbar.Consume(player.inventory.Hotbar.Selected)
	if !ok {
		return RejectInvalidBlock, true
	}
	next := block + 1 // Stage0..6 连续递增即下一阶段；Stage7 已在上游拒绝。
	_, changed, setErr := dimension.SetBlock(hit.Block, next)
	if setErr != nil {
		return mapSetBlockError(setErr), true
	}
	if !changed {
		return RejectNoTarget, true
	}
	engine.recordChange(session.dimension, hit.Block, next, pending)
	player.inventory.Hotbar = consumed
	player.inventoryDirty = true
	return 0, false
}
