package entity

import (
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/world"
)

const (
	DropInterestRadius = core.DropInterestRadius
)

func (engine *Engine) sessionDropWantedSnapshot(
	session *sessionState,
) map[core.ChunkKey]struct{} {
	wanted := make(map[core.ChunkKey]struct{})
	if session.player == nil || session.player.lifecycle != PlayerActive ||
		engine.dimension(session.dimension) == nil {
		return wanted
	}
	for dz := -DropInterestRadius; dz <= DropInterestRadius; dz++ {
		for dx := -DropInterestRadius; dx <= DropInterestRadius; dx++ {
			wanted[core.ChunkKey{
				Dimension: session.dimension,
				Pos: core.ChunkPos{
					X: session.center.X + int32(dx),
					Z: session.center.Z + int32(dz),
				},
			}] = struct{}{}
		}
	}
	return wanted
}

func (engine *Engine) advanceDrops(mutation *realm.Mutation) {
	keys := engine.activeInterestKeys()
	if len(keys) == 0 {
		return
	}
	sessions := engine.sortedActiveSessions()
	lifetimeTicks := engine.tunables.DropLifetimeTicks
	pickupRange := engine.tunables.DropPickupRange
	for _, key := range keys {
		dimension := engine.dimension(key.Dimension)
		if dimension == nil {
			continue
		}
		chunk, ok := dimension.ReadyChunk(key.Pos)
		if !ok {
			continue
		}
		if engine.advanceChunkDrops(key, chunk, sessions, lifetimeTicks, pickupRange) {
			engine.touchChunk(key, mutation)
		}
	}
}

func (engine *Engine) advanceChunkDrops(
	key core.ChunkKey,
	chunk *world.Chunk,
	sessions []SessionID,
	lifetimeTicks uint32,
	pickupRange float32,
) bool {
	changed := false
	for slot := range core.DropsPerChunk {
		drop := chunk.Drop(slot)
		if !drop.Active {
			continue
		}
		if drop.PickupDelayTicks > 0 {
			drop.PickupDelayTicks--
		}
		drop.AgeTicks++
		if drop.AgeTicks >= lifetimeTicks {
			chunk.ClearDrop(slot)
			changed = true
			continue
		}
		chunk.SetDrop(slot, drop)
		if drop.PickupDelayTicks > 0 {
			continue
		}
		if engine.pickUpDrop(key, chunk, slot, sessions, pickupRange) {
			changed = true
		}
	}
	return changed
}

func (engine *Engine) pickUpDrop(
	key core.ChunkKey,
	chunk *world.Chunk,
	slot int,
	sessions []SessionID,
	pickupRange float32,
) bool {
	center, ok := dropCenter(key.Pos, chunk.Drop(slot).BlockIndex)
	if !ok {
		return false
	}
	changed := false
	for _, id := range sessions {
		session := engine.sessions[id]
		if session == nil || session.player == nil || session.dimension != key.Dimension {
			continue
		}
		drop := chunk.Drop(slot)
		if !drop.Active || drop.Stack.Count == 0 {
			return true
		}
		if !withinPickupRange(session.player.state.Position, center, pickupRange) {
			continue
		}
		player := session.player
		next, remainder := player.inventory.AddStack(drop.Stack)
		taken := drop.Stack.Count - remainder.Count
		if taken == 0 {
			continue
		}
		if !canRepackCrafting(next, player.crafting) {
			continue
		}
		player.inventory = next
		player.inventoryDirty = true
		drop.Stack.Count -= taken
		if drop.Stack.Count == 0 {
			chunk.ClearDrop(slot)
			return true
		}
		chunk.SetDrop(slot, drop)
		changed = true
	}
	return changed
}

func dropCenter(chunk core.ChunkPos, blockIndex uint32) (mgl32.Vec3, bool) {
	position, ok := world.BlockPosFromChunkIndex(chunk, blockIndex)
	if !ok {
		return mgl32.Vec3{}, false
	}
	return mgl32.Vec3{
		float32(position.X) + 0.5,
		float32(position.Y) + 0.5,
		float32(position.Z) + 0.5,
	}, true
}

func withinPickupRange(player, center mgl32.Vec3, pickupRange float32) bool {
	return center.Sub(player).Len() <= pickupRange
}

func (engine *Engine) activeInterestKeys() []core.ChunkKey {
	if engine.dropKeySeen == nil {
		engine.dropKeySeen = make(map[core.ChunkKey]struct{}, core.MaxSessionDrops)
	}
	clear(engine.dropKeySeen)
	keys := engine.dropKeyScratch[:0]
	for _, session := range engine.sessions {
		if session.player == nil || session.player.lifecycle != PlayerActive ||
			engine.dimension(session.dimension) == nil {
			continue
		}
		for dx := -DropInterestRadius; dx <= DropInterestRadius; dx++ {
			for dz := -DropInterestRadius; dz <= DropInterestRadius; dz++ {
				key := core.ChunkKey{
					Dimension: session.dimension,
					Pos: core.ChunkPos{
						X: session.center.X + int32(dx),
						Z: session.center.Z + int32(dz),
					},
				}
				if _, seen := engine.dropKeySeen[key]; seen {
					continue
				}
				engine.dropKeySeen[key] = struct{}{}
				keys = append(keys, key)
			}
		}
	}
	sortChunkKeys(keys)
	engine.dropKeyScratch = keys
	return keys
}

// sortedActiveSessions 已在 helpers.go 定义，此处不重复；若需独立定义则复用该实现
// 为避免重复，提供别名包装由 helpers.go 提供，此处不重复定义。

func (engine *Engine) SetChunkDropForTest(
	key core.ChunkKey,
	slot int,
	value world.DropSlot,
) {
	dimension := engine.dimension(key.Dimension)
	if dimension == nil {
		return
	}
	dimension.UpdateReadyChunk(key.Pos, func(chunk *world.Chunk) { chunk.SetDrop(slot, value) })
}

func (engine *Engine) SetBlockForTest(position core.BlockPos, block core.BlockID) {
	dimension := engine.dimension(core.Overworld)
	if dimension == nil {
		return
	}
	_, _, _ = dimension.SetBlock(position, block)
}

func (engine *Engine) AppendSessionDrops(id SessionID, dst []DropSnapshot) []DropSnapshot {
	session := engine.sessions[id]
	if session == nil || session.player == nil ||
		session.player.lifecycle != PlayerActive {
		return dst
	}
	dimension := engine.dimension(session.dimension)
	if dimension == nil {
		return dst
	}
	for dx := -DropInterestRadius; dx <= DropInterestRadius; dx++ {
		for dz := -DropInterestRadius; dz <= DropInterestRadius; dz++ {
			key := core.ChunkKey{
				Dimension: session.dimension,
				Pos: core.ChunkPos{
					X: session.center.X + int32(dx),
					Z: session.center.Z + int32(dz),
				},
			}
			chunk, ok := dimension.ReadyChunk(key.Pos)
			if !ok {
				continue
			}
			dst = appendChunkDrops(dst, key, chunk)
		}
	}
	return dst
}

func appendChunkDrops(
	dst []DropSnapshot,
	key core.ChunkKey,
	chunk *world.Chunk,
) []DropSnapshot {
	for slot := range core.DropsPerChunk {
		drop := chunk.Drop(slot)
		if !drop.Active {
			continue
		}
		dst = append(dst, DropSnapshot{
			ID: core.DropID{
				Dimension:  key.Dimension,
				Chunk:      key.Pos,
				Slot:       uint8(slot),
				Generation: drop.Generation,
			},
			BlockIndex: drop.BlockIndex,
			Item:       drop.Stack.Item,
			Count:      drop.Stack.Count,
			Durability: drop.Stack.Durability,
		})
	}
	return dst
}

func (engine *Engine) dropSelectedItem(
	session *sessionState,
	mutation *realm.Mutation,
) (RejectReason, bool) {
	if session.player == nil || session.player.lifecycle != PlayerActive {
		return RejectPlayerNotReady, true
	}
	player := session.player
	selected := player.inventory.Hotbar.Selected
	nextHotbar, ok := player.inventory.Hotbar.Consume(selected)
	if !ok {
		return RejectInvalidSlot, true
	}
	stack := player.inventory.Hotbar.Slots[selected]
	position := core.BlockPos{
		X: int32(math.Floor(float64(player.state.Position.X()))),
		Y: int32(math.Floor(float64(player.state.Position.Y()))),
		Z: int32(math.Floor(float64(player.state.Position.Z()))),
	}
	key := core.ChunkKey{Dimension: session.dimension, Pos: position.Chunk()}
	dimension := engine.dimension(key.Dimension)
	if dimension == nil {
		return RejectChunkNotReady, true
	}
	chunk, ready := dimension.ReadyChunk(key.Pos)
	if !ready {
		return RejectChunkNotReady, true
	}
	blockIndex, ok := world.ChunkBlockIndex(position)
	if !ok {
		return RejectChunkNotReady, true
	}
	dropSlot, ok := chunk.PrepareDrop(stack.Item, blockIndex)
	if !ok {
		return RejectDropCapacity, true
	}
	dropped := stack
	dropped.Count = 1
	chunk.CommitDrop(dropSlot, dropped, blockIndex, engine.tunables.PlayerDropPickupDelayTicks)
	player.inventory.Hotbar = nextHotbar
	player.inventoryDirty = true
	engine.touchChunk(key, mutation)
	return 0, false
}


