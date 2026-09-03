package entity

import (
	"math"
	"sort"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/world"
)

const (
	// DropInterestRadius 是掉落物 tick 与同步的固定区块半径。
	DropInterestRadius = core.DropInterestRadius
)

// sessionDropWantedSnapshot 返回该会话的固定半径掉落物兴趣区块。
// 只有已进入 Play 的玩家参与掉落物 tick 与同步。
func (engine *engineContext) sessionDropWantedSnapshot(
	session *sessionState,
) map[core.ChunkKey]struct{} {
	wanted := make(map[core.ChunkKey]struct{})
	if session.player == nil || session.player.lifecycle != PlayerActive ||
		engine.dimension(session.dimension) == nil {
		return wanted
	}
	center := engine.sessionView(session).Center
	for dz := -DropInterestRadius; dz <= DropInterestRadius; dz++ {
		for dx := -DropInterestRadius; dx <= DropInterestRadius; dx++ {
			wanted[core.ChunkKey{
				Dimension: session.dimension,
				Pos: core.ChunkPos{
					X: center.X + int32(dx),
					Z: center.Z + int32(dz),
				},
			}] = struct{}{}
		}
	}
	return wanted
}

// advanceDrops 在单写者 tick 中推进兴趣范围内的掉落物寿命并处理拾取。
// 它按 ChunkKey、SessionID、槽位稳定扫描，最坏为玩家数 × 25 区块 × 32 槽。
func (engine *engineContext) advanceDrops(pending *pendingChunkChanges) {
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
			engine.touchChunk(key, pending)
		}
	}
}

// advanceChunkDrops 推进一个区块的全部槽，返回该区块是否发生变化。
// lifetimeTicks、pickupRange 是调用方在本 tick 入口取的快照值，本函数全程只用它们，
// 不再重新读取生效参数。
func (engine *engineContext) advanceChunkDrops(
	key core.ChunkKey,
	chunk *world.Chunk,
	sessions []SessionID,
	lifetimeTicks uint32,
	pickupRange float32,
) bool {
	// 只有可观察的变化（过期、拾取）推进区块 revision；
	// 年龄与拾取延迟是权威内部计数，不产生每 tick 的发布。
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

// pickUpDrop 让范围内的玩家按稳定顺序尽量拾取该堆，返回堆是否发生变化。
func (engine *engineContext) pickUpDrop(
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
		// 一次整堆装入：先快捷栏后背包，余量留在地面。
		next, remainder := player.inventory.AddStack(drop.Stack)
		taken := drop.Stack.Count - remainder.Count
		if taken == 0 {
			continue
		}
		// 回收不变量（design.md D4）：拾取不得挤占合成网格回收所需的背包
		// 预算。预演失败时整堆拒绝——部分拾取也不例外，掉落物原样留在世界、
		// 权威物品状态不变；下一 tick 若玩家腾出空间仍可正常拾取。
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

// dropCenter 返回掉落物所在方块的中心世界坐标。
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

// withinPickupRange 报告玩家是否在拾取距离内。pickupRange 由调用方传入本 tick 的
// 快照值，这个自由函数本身绝不读取 ActiveTunables。
func withinPickupRange(player, center mgl32.Vec3, pickupRange float32) bool {
	return center.Sub(player).Len() <= pickupRange
}

// activeInterestKeys 返回本 tick 需要推进的区块，按稳定顺序排列且不重复。
// 掉落物与熔炉共用同一集合，并复用引擎 scratch，因此稳定玩家数下不产生每 tick 分配。
func (engine *engineContext) activeInterestKeys() []core.ChunkKey {
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
		center := engine.sessionView(session).Center
		for dx := -DropInterestRadius; dx <= DropInterestRadius; dx++ {
			for dz := -DropInterestRadius; dz <= DropInterestRadius; dz++ {
				key := core.ChunkKey{
					Dimension: session.dimension,
					Pos: core.ChunkPos{
						X: center.X + int32(dx),
						Z: center.Z + int32(dz),
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

func (engine *engineContext) sortedActiveSessions() []SessionID {
	sessions := engine.dropSessionScratch[:0]
	for id, session := range engine.sessions {
		if session.player != nil && session.player.lifecycle == PlayerActive {
			sessions = append(sessions, id)
		}
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i] < sessions[j] })
	engine.dropSessionScratch = sessions
	return sessions
}

// touchChunk 为只有掉落物变化的区块登记一个零方块 revision barrier。
func (engine *engineContext) touchChunk(
	key core.ChunkKey,
	pending *pendingChunkChanges,
) {
	pending.Touch(key)
}

// SetChunkDropForTest 直接写入一个已 Ready 区块的掉落物槽，仅供测试构造固定场景。
func (engine *engineContext) SetChunkDropForTest(
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

// SetBlockForTest 直接写入一个已 Ready 区块的方块，仅供测试构造固定场景。
func (engine *engineContext) SetBlockForTest(position core.BlockPos, block core.BlockID) {
	dimension := engine.dimension(core.Overworld)
	if dimension == nil {
		return
	}
	_, _, _ = dimension.SetBlock(position, block)
}

// AppendSessionDrops 按稳定 ID 顺序把该会话兴趣范围内的当前掉落物追加到 dst。
// 调用方可复用 dst 底层数组；结果最多 MaxSessionDrops 项。
func (engine *engineContext) AppendSessionDrops(id SessionID, dst []DropSnapshot) []DropSnapshot {
	session := engine.sessions[id]
	if session == nil || session.player == nil ||
		session.player.lifecycle != PlayerActive {
		return dst
	}
	dimension := engine.dimension(session.dimension)
	if dimension == nil {
		return dst
	}
	center := engine.sessionView(session).Center
	// 按 ChunkKey 的排序顺序直接遍历固定半径，避免每次调用分配集合。
	for dx := -DropInterestRadius; dx <= DropInterestRadius; dx++ {
		for dz := -DropInterestRadius; dz <= DropInterestRadius; dz++ {
			key := core.ChunkKey{
				Dimension: session.dimension,
				Pos: core.ChunkPos{
					X: center.X + int32(dx),
					Z: center.Z + int32(dz),
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

// dropSelectedItem 在单写者 tick 内把权威选中快捷栏中的一个物品原地转移为掉落物。
//
// 全部预检在任何写入之前完成：任一失败都不改变背包、掉落物、区块 revision
// 或 persistence 状态。位置取玩家脚底方块，栏位取权威选中格，两者都不由客户端提供。
func (engine *engineContext) dropSelectedItem(
	session *sessionState,
	pending *pendingChunkChanges,
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
	engine.touchChunk(key, pending)
	return 0, false
}
