package entity

import (
	"errors"
	"fmt"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/world"
)

func (engine *engineContext) executePlacement(
	command Command,
	pending *pendingChunkChanges,
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
	view := engine.sessionView(session)
	if !view.Ready || dimension == nil {
		return RejectInvalidRay, true
	}
	originKey := core.ChunkKey{
		Dimension: dimensionID,
		Pos:       originBlock.Chunk(),
	}
	if !engine.sessionWantsChunk(session, originKey) {
		return RejectInvalidRay, true
	}
	player := session.player
	// 放置先从请求栏位解析物品和方块，并在快捷栏副本上预演扣除。
	if command.Slot >= core.HotbarSlots {
		return RejectInvalidSlot, true
	}
	item := player.inventory.Hotbar.Slots[command.Slot].Item
	// 火把是唯一面向相关的物品：最终形态由命中面决定，这里只先确认「它是
	// 可放置物品」，真正的形态解析推迟到射线命中之后经
	// core.PlaceableBlockAtFace 统一完成。非火把物品不经过这个特判，行为不变。
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
	// 形态解析的唯一窗口：物品 × 命中面 → 写入方块。火把在底面（与非法面值）
	// 没有可放置形态，在这里拒绝；其余物品的形状与面无关，对任意合法面恒返回
	// 与 ItemPlacement 相同的方块，预检通过的它们不会在这里新增拒绝路径。
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
	// 流体格不算被占用：射线现在会穿过水命中水下的固体，落点因此常常是一格水，
	// 放置必须直接把它覆盖掉（与 Minecraft 系的「向水里放方块」同语义）。覆盖
	// 走的是下面同一条 SetBlock → recordChange 路径，因此该格及其六个面邻格会被
	// enqueueFluidUpdate 重新入队，被切断的下游流动水才会按规则变干，水面不会
	// 留下一个不会被填回的洞。湿度候选不能挂在通用 `recordChange`；`SetBlock`
	// 确认写入变化后，还要用本处已有的写前与写后方块编号单独判流体 membership。
	occupied := (block != core.AirID && !core.IsFluid(block)) || placementOverlapsPlayer(
		placement,
		target,
		player.state.Position,
	)
	if occupied {
		return RejectOccupied, true
	}
	// 作物是唯一有「落脚方块」前置的放置物：种子只能种在耕地正上方。这条前置
	// **追加**在通用校验之后，因此非种子物品的放置行为一字不变。
	//
	// 判据读的是**目标格正下方的方块编号**，不是命中面：玩家通常俯视耕地顶面
	// （目标格就在耕地之上），但也可以瞄准旁边方块的侧面把目标格落到耕地上方，
	// 两种瞄法都应放行。要求"命中面必须是耕地"会把后一种合法瞄法误拒。
	//
	// 「手持物不适用于当前目标」与「选中物品不产生可放置方块」是同一类事实，
	// 复用放置路径与翻地已在用的 RejectInvalidBlock，不新增 wire 值。
	if core.IsDoor(placement) {
		dir := yawToDoorDir(command.Yaw)
		reason, rejected := engine.tryPlaceDoor(dimensionID, target, dir, pending)
		if rejected {
			return reason, true
		}
		player.inventory.Hotbar = consumed
		player.inventoryDirty = true
		return 0, false
	}
	if core.IsBed(placement) {
		// 床与门共用同一套 yaw → 水平方向派生（`core/bed.go` 把床的方向编码
		// 冻结为门先例同序），床头落在床尾的朝向侧邻格，由 `tryPlaceBed`
		// 原子完成；单值放置映射给出的默认形态（南向床尾）在这里被朝向展开。
		dir := yawToDoorDir(command.Yaw)
		reason, rejected := engine.tryPlaceBed(dimensionID, target, dir, pending)
		if rejected {
			return reason, true
		}
		player.inventory.Hotbar = consumed
		player.inventoryDirty = true
		return 0, false
	}
	if core.IsCrop(placement) {
		// 种子不复用上面「流体可覆盖」的通用放置语义：规格字面写死种子 MUST
		// NOT 被放置在非空气格，流体也不例外——往水里种麦子没有玩法意义，
		// 且与组 4 翻地已经把流体判为占用的判据保持一致（Ruling 31）。
		// 因此这里额外判目标格必须是 AirID；通用 occupied 判据已经挡掉了
		// 非空气非流体的固体，这条只需要补上流体这一种漏网情况。
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
	// 火把的「落脚支撑」前置，与种子同类：追加在通用校验之后，因此非火把
	// 物品的放置行为一字不变。四条拒绝都不扣物品（扣料提交在最后一段）：
	//   - 目标格为流体：通用放置允许把方块直接放进水里，火把例外——它零碰撞
	//     且不允许水进入其格，盖进水里等于造出一格悬浊的矛盾状态；
	//   - 目标格玩家占位：火把零碰撞，通用占位判据（逐碰撞盒求交）对它恒空，
	//     单独用整格 AABB 判交；
	//   - 支撑格未加载：沿用未就绪拒绝语义；
	//   - 支撑格非实心（空气、流体、作物、火把自身）：形态映射给出的支撑格
	//     必须真的能承载火把。
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
	// 放置熔炉或箱子必须先预留槽位；槽位耗尽时不改方块也不扣物品。
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
			pending,
		)
		if core.IsFluid(block) != core.IsFluid(placement) {
			engine.realm.EnqueueFarmlandMoistureAroundFluid(dimensionID, target)
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

func validPlayerInput(command Command) bool {
	return command.MoveX >= -1 && command.MoveX <= 1 &&
		command.MoveZ >= -1 && command.MoveZ <= 1 &&
		validPlayerLook(command.Yaw, command.Pitch)
}

func validPlayerLook(yaw, pitch float32) bool {
	const maxPitch = float32(math.Pi/2 - 0.01)
	return finiteInputComponent(yaw) && finiteInputComponent(pitch) &&
		pitch >= -maxPitch && pitch <= maxPitch
}

func finiteInputComponent(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}

func normalizeYaw(yaw float32) float32 {
	normalized := math.Mod(float64(yaw)+math.Pi, 2*math.Pi)
	if normalized < 0 {
		normalized += 2 * math.Pi
	}
	return float32(normalized - math.Pi)
}

func adjacentBlock(block core.BlockPos, face core.BlockFace) core.BlockPos {
	switch face {
	case core.BlockFaceNegX:
		block.X--
	case core.BlockFacePosX:
		block.X++
	case core.BlockFaceNegY:
		block.Y--
	case core.BlockFacePosY:
		block.Y++
	case core.BlockFaceNegZ:
		block.Z--
	case core.BlockFacePosZ:
		block.Z++
	default:
		panic(fmt.Sprintf("sim: invalid hit face %d", face))
	}
	return block
}

func placementOverlapsPlayer(
	block core.BlockID,
	position core.BlockPos,
	playerPosition mgl32.Vec3,
) bool {
	playerBounds := physics.PlayerBounds(playerPosition)
	boxes := physics.BlockCollisionBoxes(block, true)
	offset := mgl32.Vec3{float32(position.X), float32(position.Y), float32(position.Z)}
	for index := 0; index < min(int(boxes.Count), len(boxes.Boxes)); index++ {
		box := core.AABB{
			Min: boxes.Boxes[index].Min.Add(offset),
			Max: boxes.Boxes[index].Max.Add(offset),
		}
		if playerBounds.Overlaps(box) {
			return true
		}
	}
	return false
}

func mapSetBlockError(err error) RejectReason {
	if errors.Is(err, ErrChunkNotReady) || errors.Is(err, ErrBlockOutOfWorld) {
		return RejectChunkNotReady
	}
	return RejectInvalidRay
}
