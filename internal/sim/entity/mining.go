package entity

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/world"
)

func miningRule(block core.BlockID, held core.ItemID) (uint16, bool) {
	if core.IsDoor(block) {
		return 15, true
	}
	if core.IsBed(block) {
		return 15, true
	}
	if core.IsCrop(block) {
		return 1, true
	}
	if core.IsFarmland(block) {
		return 5, true
	}
	switch block {
	case core.DirtID, core.GrassID, core.SandID, core.GravelID, core.LeavesID,
		core.GlassID, core.WhiteWoolID, core.ClayID, core.SnowBlockID:
		return 5, true
	case core.OakLogID, core.OakPlanksID, core.WorkbenchID:
		return 15, true
	case core.StoneID, core.CobblestoneID, core.SmoothStoneID, core.BrickID,
		core.RoofTileID, core.MossyCobblestoneID:
		switch held {
		case core.ItemNone, core.ItemBrokenStonePickaxe, core.ItemBrokenIronPickaxe:
			return 30, true
		case core.ItemStonePickaxe:
			return 15, true
		case core.ItemIronPickaxe:
			return 8, true
		default:
			return 30, false
		}
	case core.StoneBrickID, core.FurnaceID, core.ChestID, core.LightBlockID,
		core.CoalOreID, core.IronOreID:
		switch held {
		case core.ItemStonePickaxe:
			return 15, true
		case core.ItemIronPickaxe:
			return 8, true
		default:
			return 30, false
		}
	case core.IronBlockID:
		switch held {
		case core.ItemStonePickaxe:
			return 20, false
		case core.ItemIronPickaxe:
			return 10, true
		default:
			return 40, false
		}
	default:
		return 0, false
	}
}

func stepMiningProgress(actor *actorState, target core.BlockPos, block core.BlockID) {
	held := actor.inventory.Hotbar.Slots[actor.inventory.Hotbar.Selected].Item
	required, harvestable := miningRule(block, held)
	if required == 0 {
		actor.mining = miningState{}
		return
	}
	if actor.mining.target == target && actor.mining.block == block &&
		actor.mining.held == held && actor.mining.requiredTicks != 0 {
		if actor.mining.progressTicks < actor.mining.requiredTicks {
			actor.mining.progressTicks++
		}
		return
	}
	actor.mining = miningState{
		target:        target,
		block:         block,
		held:          held,
		progressTicks: 1,
		requiredTicks: required,
		harvestable:   harvestable,
	}
}

func companionMineableBlock(block core.BlockID) bool {
	if core.IsCrop(block) || core.IsFarmland(block) || core.IsTorch(block) {
		return false
	}
	_, ok := core.BlockDrop(block)
	return ok
}

func (engine *Engine) advanceMining(
	mutation *realm.Mutation,
	result *TickResult,
) {
	var sessions [8]SessionID
	count := 0
	for id, session := range engine.sessions {
		if session.player == nil || session.player.lifecycle != PlayerActive {
			continue
		}
		if count == len(sessions) {
			panic("entity: more than eight active player sessions")
		}
		index := count
		for index > 0 && sessions[index-1] > id {
			sessions[index] = sessions[index-1]
			index--
		}
		sessions[index] = id
		count++
	}

	for _, id := range sessions[:count] {
		session := engine.sessions[id]
		player := session.player
		if !player.miningHeld || player.meleeSuppressedMining || player.reset || !session.hasView || session.viewContainer {
			player.mining = miningState{}
			continue
		}
		dimension := engine.dimension(session.dimension)
		if dimension == nil {
			player.mining = miningState{}
			continue
		}
		origin := player.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
		hit, ok, err := core.RaycastBlocks(
			origin,
			LookDirection(player.yaw, player.pitch),
			engine.tunables.InteractionReach,
			blockRaycastSampler(dimension),
		)
		if err != nil || !ok {
			player.mining = miningState{}
			continue
		}
		block, ready := dimension.BlockAt(hit.Block)
		if !ready {
			player.mining = miningState{}
			continue
		}
		stepMiningProgress(&player.actorState, hit.Block, block)
		if player.mining.requiredTicks == 0 ||
			player.mining.progressTicks < player.mining.requiredTicks {
			continue
		}
		minedBlock := player.mining.block
		reason, rejected := engine.completeMining(
			session.dimension,
			player.mining.target,
			player.mining.block,
			player.mining.harvestable,
			mutation,
		)
		player.mining = miningState{}
		if rejected {
			result.Rejected = append(result.Rejected, Rejection{
				Session: id, Sequence: player.lastInputSequence, Reason: reason,
			})
			continue
		}
		player.applyExhaustion(exhaustionMiningMilli, engine.tunables.ExhaustionThresholdMilli)
		held := player.inventory.Hotbar.Slots[player.inventory.Hotbar.Selected].Item
		if !hoeHarvestDurabilityExempt(minedBlock, held) &&
			consumeToolDurability(&player.actorState) {
			player.inventoryDirty = true
		}
	}

	if len(engine.companions) == 0 {
		return
	}
	for _, id := range engine.activeCompanionIDs() {
		engine.advanceCompanionMining(engine.companions[id], mutation)
	}
}

func (engine *Engine) advanceCompanionMining(
	entry *companionState,
	mutation *realm.Mutation,
) {
	if !entry.miningHeld {
		entry.mining = miningState{}
		return
	}
	dimension := engine.dimension(entry.dimension)
	if dimension == nil {
		entry.mining = miningState{}
		return
	}
	target := entry.miningTarget
	eye := entry.state.Position.Add(mgl32.Vec3{0, engine.physicsTunables.EyeHeight, 0})
	hit, ok, err := core.RaycastBlocks(
		eye,
		blockCenterVec3(target).Sub(eye),
		engine.tunables.InteractionReach,
		blockRaycastSampler(dimension),
	)
	if err != nil || !ok || hit.Block != target {
		entry.mining = miningState{}
		return
	}
	block, ready := dimension.BlockAt(target)
	if !ready {
		entry.mining = miningState{}
		return
	}
	if !companionMineableBlock(block) {
		entry.mining = miningState{}
		return
	}
	stepMiningProgress(&entry.actorState, target, block)
	if entry.mining.requiredTicks == 0 ||
		entry.mining.progressTicks < entry.mining.requiredTicks {
		return
	}
	engine.completeCompanionMining(entry, mutation)
}

func CompanionMineContainerStaging(
	block core.BlockID,
	harvestable bool,
	contents []core.ItemStack,
	inventory core.Inventory,
) (yields []core.ItemStack, staged core.Inventory, ok bool) {
	var body core.ItemID
	switch block {
	case core.ChestID:
		body = core.ItemChest
	case core.FurnaceID:
		body = core.ItemFurnace
	default:
		return nil, inventory, false
	}
	yields = make([]core.ItemStack, 0, 1+len(contents))
	if harvestable {
		yields = append(yields, core.ItemStack{Item: body, Count: 1})
	}
	for _, stack := range contents {
		if stack == (core.ItemStack{}) {
			continue
		}
		yields = append(yields, stack)
	}
	staged = inventory
	for _, stack := range yields {
		var leftover core.ItemStack
		staged, leftover = staged.AddStack(stack)
		if leftover.Count != 0 {
			return yields, inventory, false
		}
	}
	return yields, staged, true
}

func (engine *Engine) completeCompanionMining(
	entry *companionState,
	mutation *realm.Mutation,
) {
	if !companionMineableBlock(entry.mining.block) {
		entry.mining = miningState{}
		return
	}
	if entry.mining.block == core.ChestID || entry.mining.block == core.FurnaceID {
		engine.completeCompanionContainerMining(entry, mutation)
		return
	}
	if core.IsBed(entry.mining.block) {
		footPos, headPos, ok := bedHalfPositions(entry.mining.target, entry.mining.block)
		if !ok {
			entry.mining = miningState{}
			return
		}
		var staged core.Inventory
		if entry.mining.harvestable {
			var leftover core.ItemStack
			staged, leftover = entry.inventory.AddStack(core.ItemStack{Item: core.ItemBed, Count: 1})
			if leftover.Count != 0 {
				return
			}
		}
		if _, rejected := engine.clearBedPair(entry.dimension, footPos, headPos, mutation); rejected {
			entry.mining = miningState{}
			return
		}
		if entry.mining.harvestable {
			entry.inventory = staged
			entry.inventoryDirty = true
		}
		if consumeToolDurability(&entry.actorState) {
			entry.inventoryDirty = true
		}
		entry.mining = miningState{}
		return
	}
	item, _ := core.BlockDrop(entry.mining.block)
	var staged core.Inventory
	if entry.mining.harvestable {
		var leftover core.ItemStack
		staged, leftover = entry.inventory.AddStack(core.ItemStack{Item: item, Count: 1})
		if leftover.Count != 0 {
			return
		}
	}
	_, changed, err := engine.dimension(entry.dimension).SetBlock(entry.mining.target, core.AirID)
	if err != nil || !changed {
		entry.mining = miningState{}
		return
	}
	engine.recordChange(entry.dimension, entry.mining.target, core.AirID, mutation)
	if entry.mining.harvestable {
		entry.inventory = staged
		entry.inventoryDirty = true
	}
	if consumeToolDurability(&entry.actorState) {
		entry.inventoryDirty = true
	}
	entry.mining = miningState{}
}

func (engine *Engine) completeCompanionContainerMining(
	entry *companionState,
	mutation *realm.Mutation,
) {
	dimension := engine.dimension(entry.dimension)
	chunk, recordOK := dimension.ReadyChunk(entry.mining.target.Chunk())
	blockIndex, indexOK := world.ChunkBlockIndex(entry.mining.target)
	if !recordOK || !indexOK {
		entry.mining = miningState{}
		return
	}
	var contents []core.ItemStack
	chestSlot, furnaceSlot := 0, 0
	switch entry.mining.block {
	case core.ChestID:
		slot, found := chunk.ChestAt(blockIndex)
		if !found {
			entry.mining = miningState{}
			return
		}
		chestSlot = slot
		chest := chunk.Chest(slot)
		contents = chest.Items[:]
	case core.FurnaceID:
		slot, found := chunk.FurnaceAt(blockIndex)
		if !found {
			entry.mining = miningState{}
			return
		}
		furnaceSlot = slot
		furnace := chunk.Furnace(slot)
		contents = []core.ItemStack{furnace.Input, furnace.Fuel, furnace.Output}
	}
	_, staged, ok := CompanionMineContainerStaging(
		entry.mining.block, entry.mining.harvestable, contents, entry.inventory,
	)
	if !ok {
		return
	}
	_, changed, err := dimension.SetBlock(entry.mining.target, core.AirID)
	if err != nil || !changed {
		entry.mining = miningState{}
		return
	}
	engine.recordChange(entry.dimension, entry.mining.target, core.AirID, mutation)
	switch entry.mining.block {
	case core.ChestID:
		chunk.DeactivateChest(chestSlot)
	case core.FurnaceID:
		chunk.DeactivateFurnace(furnaceSlot)
	}
	entry.inventory = staged
	entry.inventoryDirty = true
	if consumeToolDurability(&entry.actorState) {
		entry.inventoryDirty = true
	}
	entry.mining = miningState{}
}

func cropYieldRolls(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) (uint8, uint8) {
	// 简化：返回中间值，保持在 1..3 范围内，满足成熟小麦的多产物约束
	h := splitmix64(uint64(seed) ^ (tick*0x9e3779b97f4a7c15) ^ uint64(pos.X)*31 ^ uint64(pos.Y)*131 ^ uint64(pos.Z)*731 ^ uint64(dim)*997)
	return uint8(1 + h%3), uint8(1 + (h>>2)%3)
}
func cropYieldRollsPotato(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) uint8 {
	h := splitmix64(uint64(seed) ^ tick ^ uint64(pos.X)*17 ^ uint64(pos.Z)*19)
	return uint8(1 + h%4)
}
func cropYieldRollsCarrot(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) uint8 {
	h := splitmix64(uint64(seed) ^ (tick*0x85ebca6b) ^ uint64(pos.X)*23)
	return uint8(1 + h%4)
}
func poisonRoll(seed int64, tick uint64, dim core.DimensionID, pos core.BlockPos) bool {
	h := splitmix64(uint64(seed) ^ (tick*0xc2b2ae35) ^ uint64(pos.Y)*7)
	return h%50 == 0
}

func hoeHarvestDurabilityExempt(block core.BlockID, item core.ItemID) bool {
	return core.IsCrop(block) && core.TillingTool(item)
}

func consumeToolDurability(actor *actorState) bool {
	selected := actor.inventory.Hotbar.Selected
	stack := actor.inventory.Hotbar.Slots[selected]
	if _, ok := core.ItemMaxDurability(stack.Item); !ok {
		return false
	}
	if stack.Durability > 1 {
		stack.Durability--
		actor.inventory.Hotbar.Slots[selected] = stack
		return true
	}
	broken, ok := core.ItemBrokenForm(stack.Item)
	if !ok {
		return false
	}
	actor.inventory.Hotbar.Slots[selected] = core.ItemStack{Item: broken, Count: 1}
	return true
}

func (engine *Engine) completeMining(
	dimensionID core.DimensionID,
	target core.BlockPos,
	block core.BlockID,
	harvestable bool,
	mutation *realm.Mutation,
) (RejectReason, bool) {
	dimension := engine.dimension(dimensionID)
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	chunk, recordOK := dimension.ReadyChunk(target.Chunk())
	blockIndex, indexOK := world.ChunkBlockIndex(target)
	if !recordOK || !indexOK {
		return RejectChunkNotReady, true
	}

	if core.IsDoor(block) {
		var lowerPos, upperPos core.BlockPos
		if core.IsDoorUpper(block) {
			lowerPos = core.BlockPos{X: target.X, Y: target.Y - 1, Z: target.Z}
			upperPos = target
		} else {
			lowerPos = target
			upperPos = core.BlockPos{X: target.X, Y: target.Y + 1, Z: target.Z}
		}
		lowerChunk, lowerOK := dimension.ReadyChunk(lowerPos.Chunk())
		_, upperOK := dimension.ReadyChunk(upperPos.Chunk())
		if !lowerOK || !upperOK {
			return RejectChunkNotReady, true
		}
		lowerIndex, lowerIndexed := world.ChunkBlockIndex(lowerPos)
		upperIndex, upperIndexed := world.ChunkBlockIndex(upperPos)
		if !lowerIndexed || !upperIndexed {
			return RejectChunkNotReady, true
		}
		var nextDrops [core.DropsPerChunk]world.DropSlot
		var hasNext bool
		if harvestable {
			stacks := [1]core.ItemStack{{Item: core.ItemDoor, Count: 1}}
			next, ok := lowerChunk.PrepareDropBatch(stacks[:], lowerIndex, engine.tunables.DropPickupDelayTicks)
			if !ok {
				return RejectDropCapacity, true
			}
			nextDrops = next
			hasNext = true
		}
		oldLower, _ := dimension.BlockAt(lowerPos)
		oldUpper, _ := dimension.BlockAt(upperPos)
		_, _, errLower := dimension.SetBlock(lowerPos, core.AirID)
		if errLower != nil {
			return mapSetBlockError(errLower), true
		}
		_, _, errUpper := dimension.SetBlock(upperPos, core.AirID)
		if errUpper != nil {
			_, _, _ = dimension.SetBlock(lowerPos, oldLower)
			_, _ = dimension.BlockAt(lowerPos)
			_ = oldUpper
			_ = upperIndex
			return mapSetBlockError(errUpper), true
		}
		engine.recordChange(dimensionID, lowerPos, core.AirID, mutation)
		engine.recordChange(dimensionID, upperPos, core.AirID, mutation)
		if hasNext {
			lowerChunk.CommitDropBatch(nextDrops)
		}
		return 0, false
	}

	if core.IsBed(block) {
		footPos, headPos, ok := bedHalfPositions(target, block)
		if !ok {
			return RejectProtectedBlock, true
		}
		otherPos := headPos
		if otherPos == target {
			otherPos = footPos
		}
		if _, otherOK := dimension.ReadyChunk(otherPos.Chunk()); !otherOK {
			return RejectChunkNotReady, true
		}
		return engine.removeBedWithDrop(dimensionID, target, footPos, headPos, harvestable, mutation)
	}

	if block == core.FurnaceID {
		furnaceSlot, found := chunk.FurnaceAt(blockIndex)
		if !found {
			return RejectChunkNotReady, true
		}
		furnace := chunk.Furnace(furnaceSlot)
		stacks := [4]core.ItemStack{
			{},
			furnace.Input,
			furnace.Fuel,
			furnace.Output,
		}
		if harvestable {
			stacks[0] = core.ItemStack{Item: core.ItemFurnace, Count: 1}
		}
		next, capacityOK := chunk.PrepareDropBatch(
			stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks,
		)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, mutation)
		chunk.DeactivateFurnace(furnaceSlot)
		chunk.CommitDropBatch(next)
		return 0, false
	}

	if block == core.ChestID {
		chestSlot, found := chunk.ChestAt(blockIndex)
		if !found {
			return RejectChunkNotReady, true
		}
		chest := chunk.Chest(chestSlot)
		var stacks [1 + core.ChestSlots]core.ItemStack
		if harvestable {
			stacks[0] = core.ItemStack{Item: core.ItemChest, Count: 1}
		}
		copy(stacks[1:], chest.Items[:])
		next, capacityOK := chunk.PrepareDropBatch(
			stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks,
		)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, mutation)
		chunk.DeactivateChest(chestSlot)
		chunk.CommitDropBatch(next)
		return 0, false
	}

	if block == core.PotatoStage7ID {
		if !harvestable {
			_, changed, err := dimension.SetBlock(target, core.AirID)
			if err != nil {
				return mapSetBlockError(err), true
			}
			if !changed {
				return RejectNoTarget, true
			}
			engine.recordChange(dimensionID, target, core.AirID, mutation)
			return 0, false
		}
		n := cropYieldRollsPotato(engine.seed, engine.tick.Load(), dimensionID, target)
		var stacks [2]core.ItemStack
		stacks[0] = core.ItemStack{Item: core.ItemPotato, Count: n}
		stackCount := 1
		if poisonRoll(engine.seed, engine.tick.Load(), dimensionID, target) {
			stacks[1] = core.ItemStack{Item: core.ItemPoisonousPotato, Count: 1}
			stackCount = 2
		}
		next, capacityOK := chunk.PrepareDropBatch(stacks[:stackCount], blockIndex, engine.tunables.DropPickupDelayTicks)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, mutation)
		chunk.CommitDropBatch(next)
		return 0, false
	}
	if block == core.CarrotStage7ID {
		if !harvestable {
			_, changed, err := dimension.SetBlock(target, core.AirID)
			if err != nil {
				return mapSetBlockError(err), true
			}
			if !changed {
				return RejectNoTarget, true
			}
			engine.recordChange(dimensionID, target, core.AirID, mutation)
			return 0, false
		}
		n := cropYieldRollsCarrot(engine.seed, engine.tick.Load(), dimensionID, target)
		stacks := [1]core.ItemStack{{Item: core.ItemCarrot, Count: n}}
		next, capacityOK := chunk.PrepareDropBatch(stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, mutation)
		chunk.CommitDropBatch(next)
		return 0, false
	}
	if block >= core.PotatoStage0ID && block <= core.PotatoStage6ID {
		if !harvestable {
			_, changed, err := dimension.SetBlock(target, core.AirID)
			if err != nil {
				return mapSetBlockError(err), true
			}
			if !changed {
				return RejectNoTarget, true
			}
			engine.recordChange(dimensionID, target, core.AirID, mutation)
			return 0, false
		}
		stacks := [1]core.ItemStack{{Item: core.ItemPotato, Count: 1}}
		next, capacityOK := chunk.PrepareDropBatch(stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, mutation)
		chunk.CommitDropBatch(next)
		return 0, false
	}
	if block >= core.CarrotStage0ID && block <= core.CarrotStage6ID {
		if !harvestable {
			_, changed, err := dimension.SetBlock(target, core.AirID)
			if err != nil {
				return mapSetBlockError(err), true
			}
			if !changed {
				return RejectNoTarget, true
			}
			engine.recordChange(dimensionID, target, core.AirID, mutation)
			return 0, false
		}
		stacks := [1]core.ItemStack{{Item: core.ItemCarrot, Count: 1}}
		next, capacityOK := chunk.PrepareDropBatch(stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, mutation)
		chunk.CommitDropBatch(next)
		return 0, false
	}

	item, ok := core.BlockDrop(block)
	if !ok {
		return RejectProtectedBlock, true
	}

	if block == core.WheatStage7ID && harvestable {
		wheatCount, seedCount := cropYieldRolls(engine.seed, engine.tick.Load(), dimensionID, target)
		stacks := [2]core.ItemStack{
			{Item: item, Count: wheatCount},
			{Item: core.ItemWheatSeeds, Count: seedCount},
		}
		next, capacityOK := chunk.PrepareDropBatch(
			stacks[:], blockIndex, engine.tunables.DropPickupDelayTicks,
		)
		if !capacityOK {
			return RejectDropCapacity, true
		}
		_, changed, err := dimension.SetBlock(target, core.AirID)
		if err != nil {
			return mapSetBlockError(err), true
		}
		if !changed {
			return RejectNoTarget, true
		}
		engine.recordChange(dimensionID, target, core.AirID, mutation)
		chunk.CommitDropBatch(next)
		return 0, false
	}

	dropSlot := 0
	if harvestable {
		var capacityOK bool
		dropSlot, capacityOK = chunk.PrepareDrop(item, blockIndex)
		if !capacityOK {
			return RejectDropCapacity, true
		}
	}
	_, changed, err := dimension.SetBlock(target, core.AirID)
	if err != nil {
		return mapSetBlockError(err), true
	}
	if !changed {
		return RejectNoTarget, true
	}
	engine.recordChange(dimensionID, target, core.AirID, mutation)
	if harvestable {
		chunk.CommitDrop(
			dropSlot,
			core.ItemStack{Item: item, Count: 1},
			blockIndex,
			engine.tunables.DropPickupDelayTicks,
		)
	}
	return 0, false
}
