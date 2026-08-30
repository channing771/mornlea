package runtime

import (
	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/entity"
	"github.com/channing771/mornlea/internal/world"
)

type (
	PlayerLifecycle = entity.PlayerLifecycle
	CraftingGrid    = entity.CraftingGrid
)

const (
	PlayerPendingSpawn        = entity.PlayerPendingSpawn
	PlayerActive              = entity.PlayerActive
	CraftingGridSizePersonal  = entity.CraftingGridSizePersonal
	CraftingGridSizeWorkbench = entity.CraftingGridSizeWorkbench
	DropInterestRadius        = entity.DropInterestRadius
)

// CompanionMineContainerStaging 保留既有 runtime 入口并委托给 entity 的唯一规则。
func CompanionMineContainerStaging(
	block core.BlockID,
	harvestable bool,
	contents []core.ItemStack,
	inventory core.Inventory,
) ([]core.ItemStack, core.Inventory, bool) {
	return entity.CompanionMineContainerStaging(
		block, harvestable, contents, inventory,
	)
}

func (engine *Engine) EntitySessionView(id SessionID) entity.SessionView {
	session := engine.subscriptions[id]
	if session == nil || session.trustedObserver {
		return entity.SessionView{}
	}
	return entity.SessionView{
		Ready:  session.hasView,
		Center: session.center,
	}
}

func (engine *Engine) entityViewSnapshot() entity.ViewSnapshot {
	entries := engine.entityViewScratch[:0]
	for id, session := range engine.subscriptions {
		if session == nil || session.trustedObserver {
			continue
		}
		origin := core.ChunkKey{Dimension: session.dimension, Pos: session.center}
		_, originWanted := session.wanted[origin]
		entries = append(entries, entity.TickSessionView{
			Session: id,
			View: entity.SessionView{
				Ready: session.hasView, Center: session.center,
			},
			Origin: origin, OriginWanted: originWanted,
		})
	}
	engine.entityViewScratch = entries
	return entity.NewViewSnapshot(entries)
}

// RegisterPlayer 同时建立 runtime 订阅记录与唯一的实体权威状态。
func (engine *Engine) RegisterPlayer(id SessionID, restore PlayerRestore) {
	if engine.subscriptions[id] != nil {
		panic("sim: duplicate registered session")
	}
	engine.entities.RegisterPlayer(id, restore, engine.realm, engine.tunables)
	engine.subscriptions[id] = &subscriptionState{
		hasView:   true,
		dimension: restore.SpawnDimension,
		center:    restore.SpawnAnchor,
		wanted:    make(map[core.ChunkKey]struct{}),
	}
	engine.subscriptionsDirty = true
}

func (engine *Engine) RegisterSession(
	id SessionID,
	dimension core.DimensionID,
	anchor core.ChunkPos,
) {
	engine.RegisterPlayer(id, PlayerRestore{
		SpawnDimension: dimension,
		SpawnAnchor:    anchor,
	})
}

func (engine *Engine) Player(id SessionID) (PlayerUpdate, bool) {
	return engine.entities.Player(
		id, engine.WorldTime(), engine.DayPhaseOffset(), engine.EntitySessionView(id),
	)
}

func (engine *Engine) PlayerSnapshot(id SessionID) (PlayerSnapshot, bool) {
	return engine.entities.PlayerSnapshot(id)
}

func (engine *Engine) PlayerHash(id SessionID) ([32]byte, bool) {
	return engine.entities.PlayerHash(id)
}

func (engine *Engine) SetPlayerPositionForTest(id SessionID, position mgl32.Vec3) {
	engine.entities.SetPlayerPositionForTest(id, position)
}

func (engine *Engine) UnregisterSession(id SessionID) (PlayerSnapshot, bool) {
	if engine.subscriptions[id] == nil {
		return PlayerSnapshot{}, false
	}
	snapshot, ok := engine.entities.UnregisterSession(id)
	delete(engine.subscriptions, id)
	engine.subscriptionsDirty = true
	return snapshot, ok
}

func (engine *Engine) RegisterCompanion(restore CompanionRestore) {
	engine.entities.RegisterCompanion(restore, engine.realm)
	engine.subscriptionsDirty = true
}

func (engine *Engine) CompanionBodies() []companion.Body {
	return engine.entities.CompanionBodies()
}

func (engine *Engine) RestoreHostile(mob HostileMob) error {
	return engine.entities.RestoreHostile(mob, engine.realm)
}

func (engine *Engine) HostileMobs() []HostileMob {
	return engine.entities.HostileMobs()
}

func (engine *Engine) PlanHostileChase(
	id uint64,
	hasTarget bool,
	target core.PlayerID,
	nextRepathTicks uint64,
) bool {
	return engine.entities.PlanHostileChase(id, hasTarget, target, nextRepathTicks)
}

func (engine *Engine) AppendSessionDrops(
	id SessionID,
	dst []DropSnapshot,
) []DropSnapshot {
	return engine.entities.AppendSessionDrops(
		id, dst, engine.realm, engine.EntitySessionView(id),
	)
}

func (engine *Engine) PlayerCrafting(
	id SessionID,
) (CraftingGrid, core.ItemStack, bool) {
	return engine.entities.PlayerCrafting(id)
}

func (engine *Engine) SetPlayerCraftingGridForTest(
	id SessionID,
	mutate func(CraftingGrid) CraftingGrid,
) {
	engine.entities.SetPlayerCraftingGridForTest(id, mutate)
}

func (engine *Engine) SetPlayerInventoryForTest(
	id SessionID,
	mutate func(core.Inventory) core.Inventory,
) {
	engine.entities.SetPlayerInventoryForTest(id, mutate)
}

func (engine *Engine) SetChunkChestForTest(
	key core.ChunkKey,
	slot int,
	value world.ChestSlot,
) {
	engine.entities.SetChunkChestForTest(key, slot, value, engine.realm)
}

func (engine *Engine) SetChunkFurnaceForTest(
	key core.ChunkKey,
	slot int,
	value world.FurnaceSlot,
) {
	engine.entities.SetChunkFurnaceForTest(key, slot, value, engine.realm)
}

func (engine *Engine) SetChunkDropForTest(
	key core.ChunkKey,
	slot int,
	value world.DropSlot,
) {
	engine.entities.SetChunkDropForTest(key, slot, value, engine.realm)
}

func (engine *Engine) SetBlockForTest(position core.BlockPos, block core.BlockID) {
	engine.entities.SetBlockForTest(position, block, engine.realm)
}

func (engine *Engine) AdvanceFurnacesForBenchmark() {
	engine.entities.AdvanceFurnacesForBenchmark(
		engine.realm, engine.tunables, engine.entityViewSnapshot(),
	)
}

func (engine *Engine) ActiveInterestKeysForTest() []core.ChunkKey {
	return append([]core.ChunkKey(nil), engine.activeInterestKeys()...)
}

func (engine *Engine) activeInterestKeys() []core.ChunkKey {
	engine.activeChunkScratch = engine.entities.AppendActiveInterestKeys(
		engine.activeChunkScratch[:0], engine.realm, engine.entityViewSnapshot(),
	)
	return engine.activeChunkScratch
}
