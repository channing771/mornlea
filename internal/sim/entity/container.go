package entity

import (
	"errors"
	"slices"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/world"
)

// containerChunk 解析一个容器引用所在的区块；区块未加载或未 Ready 都视为失效。
func (engine *Engine) containerChunk(ref core.ContainerRef) (*world.Chunk, bool) {
	dimension := engine.dimension(ref.Dimension)
	if dimension == nil {
		return nil, false
	}
	return dimension.ReadyChunk(ref.Chunk)
}

// chestView 定位一个箱子引用当前指向的槽；引用失效时返回 false。
func (engine *Engine) chestView(ref core.ContainerRef) (*world.Chunk, world.ChestSlot, bool) {
	chunk, ok := engine.containerChunk(ref)
	if !ok {
		return nil, world.ChestSlot{}, false
	}
	chest := chunk.Chest(int(ref.Slot))
	if !chest.Active || chest.Generation != ref.Generation {
		return nil, world.ChestSlot{}, false
	}
	return chunk, chest, true
}

// withinContainerReach 报告玩家是否仍在某个容器的交互范围内；
func withinContainerReach(
	eye mgl32.Vec3,
	chunk core.ChunkPos,
	blockIndex uint32,
	reach float32,
) bool {
	position, ok := world.BlockPosFromChunkIndex(chunk, blockIndex)
	if !ok {
		return false
	}
	center := mgl32.Vec3{
		float32(position.X) + 0.5,
		float32(position.Y) + 0.5,
		float32(position.Z) + 0.5,
	}
	return center.Sub(eye).Len() <= reach
}

// openContainer 处理打开请求：用权威射线在六格内命中活动熔炉或箱子才建立查看关系；
func (engine *Engine) openContainer(id SessionID, command Command) (RejectReason, bool) {
	session := engine.sessions[id]
	if session == nil || session.player == nil || session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	dimension := engine.dimension(session.dimension)
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	origin := session.player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	direction := LookDirection(command.Yaw, command.Pitch)
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
	block, ready := dimension.BlockAt(hit.Block)
	if !ready {
		return RejectChunkNotReady, true
	}
	var kind core.ContainerKind
	workbench := false
	switch block {
	case core.FurnaceID:
		kind = core.ContainerKindFurnace
	case core.ChestID:
		kind = core.ContainerKindChest
	case core.WorkbenchID:
		workbench = true
	default:
		return RejectNoTarget, true
	}
	key := core.ChunkKey{Dimension: session.dimension, Pos: hit.Block.Chunk()}
	chunk, exists := dimension.ReadyChunk(key.Pos)
	if !exists {
		return RejectChunkNotReady, true
	}
	index, indexed := world.ChunkBlockIndex(hit.Block)
	if !indexed {
		return RejectNoTarget, true
	}
	if workbench {
		player := session.player
		player.crafting.Size = CraftingGridSizeWorkbench
		player.workbench = hit.Block
		player.craftingDirty = true
		session.viewContainer = false
		session.container = core.ContainerRef{}
		return 0, false
	}

	var ref core.ContainerRef
	switch kind {
	case core.ContainerKindFurnace:
		slot, found := chunk.FurnaceAt(index)
		if !found {
			return RejectNoTarget, true
		}
		ref = core.ContainerRef{
			Dimension: session.dimension, Chunk: key.Pos, Kind: kind,
			Slot: uint8(slot), Generation: chunk.Furnace(slot).Generation,
		}
	case core.ContainerKindChest:
		slot, found := chunk.ChestAt(index)
		if !found {
			return RejectNoTarget, true
		}
		ref = core.ContainerRef{
			Dimension: session.dimension, Chunk: key.Pos, Kind: kind,
			Slot: uint8(slot), Generation: chunk.Chest(slot).Generation,
		}
	}
	session.container = ref
	session.viewContainer = true
	return 0, false
}

// publishContainers 在全部命令与推进之后校验查看关系，
func (engine *Engine) publishContainers(result *TickResult) {
	sessions := engine.containerViewerScratch[:0]
	for id, session := range engine.sessions {
		if session.viewContainer {
			sessions = append(sessions, id)
		}
	}
	slices.Sort(sessions)
	engine.containerViewerScratch = sessions

	for _, id := range sessions {
		session := engine.sessions[id]
		ref := session.container
		if session.player == nil || session.player.lifecycle != PlayerActive ||
			session.dimension != ref.Dimension {
			session.viewContainer = false
			result.FurnaceEnds = append(result.FurnaceEnds, FurnaceEnd{Session: id, Furnace: ref})
			continue
		}
		eye := session.player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
		switch ref.Kind {
		case core.ContainerKindChest:
			chunk, chest, ok := engine.chestView(ref)
			if !ok || !withinContainerReach(eye, chunk.Pos, chest.BlockIndex, engine.tunables.InteractionReach) {
				session.viewContainer = false
				result.FurnaceEnds = append(result.FurnaceEnds, FurnaceEnd{Session: id, Furnace: ref})
				continue
			}
			result.Chests = append(result.Chests, ChestUpdate{
				Session: id,
				Chest:   ref,
				Items:   chest.Items,
			})
		case core.ContainerKindFurnace:
			chunk, furnace, ok := engine.furnaceView(ref)
			if !ok || !withinContainerReach(eye, chunk.Pos, furnace.BlockIndex, engine.tunables.InteractionReach) {
				session.viewContainer = false
				result.FurnaceEnds = append(result.FurnaceEnds, FurnaceEnd{Session: id, Furnace: ref})
				continue
			}
			result.Furnaces = append(result.Furnaces, FurnaceUpdate{
				Session:       id,
				Furnace:       ref,
				Input:         furnace.Input,
				Fuel:          furnace.Fuel,
				Output:        furnace.Output,
				ProgressTicks: furnace.ProgressTicks,
				BurnTicks:     furnace.BurnTicks,
			})
		default:
			session.viewContainer = false
			result.FurnaceEnds = append(result.FurnaceEnds, FurnaceEnd{Session: id, Furnace: ref})
		}
	}
}

// mergeStacks 计算把 source 整堆或部分移入 target 后两侧的新值
func mergeStacks(source, target core.ItemStack) (nextSource, nextTarget core.ItemStack, ok bool) {
	switch {
	case target.Item == core.ItemNone:
		return core.ItemStack{}, source, true
	case target.Item == source.Item:
		limit, hasLimit := core.ItemStackLimit(target.Item)
		if !hasLimit || target.Count >= limit {
			return core.ItemStack{}, core.ItemStack{}, false
		}
		moved := min(limit-target.Count, source.Count)
		nextTarget = target
		nextTarget.Count += moved
		if source.Count > moved {
			nextSource = core.ItemStack{Item: source.Item, Count: source.Count - moved}
		}
		return nextSource, nextTarget, true
	default:
		return target, source, true
	}
}

// moveChestStack 在玩家物品与箱子的值副本上计算一次整堆移动，
func moveChestStack(
	inventory core.Inventory,
	chest world.ChestSlot,
	from, to uint8,
) (core.Inventory, world.ChestSlot, bool) {
	if from >= core.ChestViewSlots || to >= core.ChestViewSlots || from == to {
		return inventory, chest, false
	}
	if !inventory.Valid() || !chest.Valid() || !chest.Active {
		return inventory, chest, false
	}
	if from < core.InventorySlots && to < core.InventorySlots {
		next, ok := inventory.MoveStack(from, to)
		return next, chest, ok
	}

	source, ok := chestViewSlot(inventory, chest, from)
	if !ok || source.Item == core.ItemNone {
		return inventory, chest, false
	}
	target, ok := chestViewSlot(inventory, chest, to)
	if !ok {
		return inventory, chest, false
	}

	nextSource, nextTarget, ok := mergeStacks(source, target)
	if !ok {
		return inventory, chest, false
	}

	nextInventory, nextChest := inventory, chest
	if nextInventory, nextChest, ok = setChestViewSlot(
		nextInventory, nextChest, from, nextSource,
	); !ok {
		return inventory, chest, false
	}
	if nextInventory, nextChest, ok = setChestViewSlot(
		nextInventory, nextChest, to, nextTarget,
	); !ok {
		return inventory, chest, false
	}
	if !nextChest.Valid() || !nextInventory.Valid() {
		return inventory, chest, false
	}
	return nextInventory, nextChest, true
}

func chestViewSlot(
	inventory core.Inventory,
	chest world.ChestSlot,
	slot uint8,
) (core.ItemStack, bool) {
	if slot >= core.ChestFirstSlot {
		return chest.Items[slot-core.ChestFirstSlot], true
	}
	return inventory.Slot(slot)
}

func setChestViewSlot(
	inventory core.Inventory,
	chest world.ChestSlot,
	slot uint8,
	stack core.ItemStack,
) (core.Inventory, world.ChestSlot, bool) {
	if slot >= core.ChestFirstSlot {
		if !stack.Valid() {
			return inventory, chest, false
		}
		chest.Items[slot-core.ChestFirstSlot] = stack
		return inventory, chest, true
	}
	next, ok := inventory.SetSlot(slot, stack)
	if !ok {
		return inventory, chest, false
	}
	return next, chest, true
}

// applyContainerMove 处理跨容器移动命令，成功时同时提交玩家物品与区块中的容器；
func (engine *Engine) applyContainerMove(
	id SessionID,
	command Command,
	mutation *realm.Mutation,
) (RejectReason, bool) {
	session := engine.sessions[id]
	if session == nil || session.player == nil || session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	if !session.viewContainer || session.container != command.Furnace {
		return RejectInvalidInput, true
	}
	ref := command.Furnace
	key := core.ChunkKey{Dimension: ref.Dimension, Pos: ref.Chunk}

	switch ref.Kind {
	case core.ContainerKindChest:
		chunk, chest, ok := engine.chestView(ref)
		if !ok {
			return RejectInvalidInput, true
		}
		nextInventory, nextChest, ok := moveChestStack(
			session.player.inventory, chest, command.Slot, command.ToSlot,
		)
		if !ok {
			return RejectInvalidInput, true
		}
		if !canRepackCrafting(nextInventory, session.player.crafting) {
			return RejectInvalidInput, true
		}
		chunk.SetChest(int(ref.Slot), nextChest)
		engine.touchChunk(key, mutation)
		session.player.inventory = nextInventory
		session.player.inventoryDirty = true
		return 0, false
	case core.ContainerKindFurnace:
		chunk, furnace, ok := engine.furnaceView(ref)
		if !ok {
			return RejectInvalidInput, true
		}
		nextInventory, nextFurnace, ok := moveFurnaceStack(
			session.player.inventory, furnace, command.Slot, command.ToSlot,
		)
		if !ok {
			return RejectInvalidInput, true
		}
		if !canRepackCrafting(nextInventory, session.player.crafting) {
			return RejectInvalidInput, true
		}
		chunk.SetFurnace(int(ref.Slot), nextFurnace)
		engine.touchChunk(key, mutation)
		session.player.inventory = nextInventory
		session.player.inventoryDirty = true
		return 0, false
	default:
		return RejectInvalidInput, true
	}
}

// SetChunkChestForTest 直接写入一个已 Ready 区块的箱子槽，仅供测试构造固定场景。
func (engine *Engine) SetChunkChestForTest(
	key core.ChunkKey,
	slot int,
	value world.ChestSlot,
) {
	dimension := engine.dimension(key.Dimension)
	if dimension == nil {
		return
	}
	dimension.UpdateReadyChunk(key.Pos, func(chunk *world.Chunk) { chunk.SetChest(slot, value) })
}
