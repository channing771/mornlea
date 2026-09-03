package entity

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/tuning"
	"github.com/channing771/mornlea/packages/shared/world"
)

func (state *State) RegisterPlayer(
	id SessionID,
	restore PlayerRestore,
	realmState *realm.State,
	tunables tuning.Tunables,
) {
	state.context(realmState, 0, 0, 0, tunables, physics.Tunables{}, ViewSnapshot{}).
		RegisterPlayer(id, restore)
}

func (state *State) RegisterSession(
	id SessionID,
	dimension core.DimensionID,
	anchor core.ChunkPos,
	realmState *realm.State,
	tunables tuning.Tunables,
) {
	state.RegisterPlayer(id, PlayerRestore{
		SpawnDimension: dimension,
		SpawnAnchor:    anchor,
	}, realmState, tunables)
}

func (state *State) Player(
	id SessionID,
	worldTime uint64,
	dayPhaseOffset uint16,
	view SessionView,
) (PlayerUpdate, bool) {
	return state.context(
		nil, 0, worldTime, dayPhaseOffset, tuning.Tunables{}, physics.Tunables{},
		singleViewSnapshot(id, view),
	).Player(id)
}

func (state *State) PlayerSnapshot(id SessionID) (PlayerSnapshot, bool) {
	return (&engineContext{State: state}).PlayerSnapshot(id)
}

func (state *State) PlayerHash(id SessionID) ([32]byte, bool) {
	return (&engineContext{State: state}).PlayerHash(id)
}

func (state *State) UnregisterSession(id SessionID) (PlayerSnapshot, bool) {
	return (&engineContext{State: state}).UnregisterSession(id)
}

func (state *State) SetPlayerPositionForTest(id SessionID, position mgl32.Vec3) {
	(&engineContext{State: state}).SetPlayerPositionForTest(id, position)
}

func (state *State) RegisterCompanion(
	restore CompanionRestore,
	realmState *realm.State,
) {
	state.context(realmState, 0, 0, 0, tuning.Tunables{}, physics.Tunables{}, ViewSnapshot{}).
		RegisterCompanion(restore)
}

func (state *State) CompanionBodies() []companion.Body {
	return (&engineContext{State: state}).CompanionBodies()
}

func (state *State) RestoreHostile(
	mob HostileMob,
	realmState *realm.State,
) error {
	return state.context(realmState, 0, 0, 0, tuning.Tunables{}, physics.Tunables{}, ViewSnapshot{}).
		RestoreHostile(mob)
}

func (state *State) HostileMobs() []HostileMob {
	return (&engineContext{State: state}).HostileMobs()
}

func (state *State) PlanHostileChase(
	id uint64,
	hasTarget bool,
	target core.PlayerID,
	nextRepathTicks uint64,
) bool {
	return (&engineContext{State: state}).PlanHostileChase(
		id, hasTarget, target, nextRepathTicks,
	)
}

func (state *State) AppendSessionDrops(
	id SessionID,
	dst []DropSnapshot,
	realmState *realm.State,
	view SessionView,
) []DropSnapshot {
	return state.context(
		realmState, 0, 0, 0, tuning.Tunables{}, physics.Tunables{},
		singleViewSnapshot(id, view),
	).AppendSessionDrops(id, dst)
}

func (state *State) PlayerCrafting(id SessionID) (CraftingGrid, core.ItemStack, bool) {
	return (&engineContext{State: state}).PlayerCrafting(id)
}

func (state *State) SetPlayerCraftingGridForTest(
	id SessionID,
	mutate func(CraftingGrid) CraftingGrid,
) {
	(&engineContext{State: state}).SetPlayerCraftingGridForTest(id, mutate)
}

func (state *State) SetPlayerInventoryForTest(
	id SessionID,
	mutate func(core.Inventory) core.Inventory,
) {
	(&engineContext{State: state}).SetPlayerInventoryForTest(id, mutate)
}

func (state *State) SetChunkChestForTest(
	key core.ChunkKey,
	blockIndex int,
	chest world.ChestSlot,
	realmState *realm.State,
) {
	state.context(realmState, 0, 0, 0, tuning.Tunables{}, physics.Tunables{}, ViewSnapshot{}).
		SetChunkChestForTest(key, blockIndex, chest)
}

func (state *State) SetChunkFurnaceForTest(
	key core.ChunkKey,
	blockIndex int,
	furnace world.FurnaceSlot,
	realmState *realm.State,
) {
	state.context(realmState, 0, 0, 0, tuning.Tunables{}, physics.Tunables{}, ViewSnapshot{}).
		SetChunkFurnaceForTest(key, blockIndex, furnace)
}

func (state *State) AdvanceFurnacesForBenchmark(
	realmState *realm.State,
	tunables tuning.Tunables,
	views ViewSnapshot,
) {
	// 基准入口把短命上下文保留在调用栈，避免跨包委派把固定工作量热路径
	// 变成每 tick 一次堆分配。
	context := engineContext{
		State:    state,
		realm:    realmState,
		tunables: tunables,
		views:    views,
	}
	context.AdvanceFurnacesForBenchmark()
}

func (state *State) AppendActiveInterestKeys(
	dst []core.ChunkKey,
	realmState *realm.State,
	views ViewSnapshot,
) []core.ChunkKey {
	keys := state.context(
		realmState, 0, 0, 0, tuning.Tunables{}, physics.Tunables{}, views,
	).activeInterestKeys()
	return append(dst, keys...)
}

func (state *State) SetChunkDropForTest(
	key core.ChunkKey,
	slot int,
	drop world.DropSlot,
	realmState *realm.State,
) {
	state.context(realmState, 0, 0, 0, tuning.Tunables{}, physics.Tunables{}, ViewSnapshot{}).
		SetChunkDropForTest(key, slot, drop)
}

func (state *State) SetBlockForTest(
	position core.BlockPos,
	block core.BlockID,
	realmState *realm.State,
) {
	state.context(realmState, 0, 0, 0, tuning.Tunables{}, physics.Tunables{}, ViewSnapshot{}).
		SetBlockForTest(position, block)
}
