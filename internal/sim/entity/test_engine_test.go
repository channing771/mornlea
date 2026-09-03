package entity

import (
	"sort"

	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/tuning"
	"github.com/channing771/mornlea/packages/shared/world"
)

// Engine 是 entity 白盒测试的状态与 realm 夹具，不包含 runtime inbox、
// 订阅协调或 tick 阶段编排。
type Engine struct {
	*engineContext
	viewEntries   []TickSessionView
	viewOverrides []TickSessionView
}

// fixtureTick 只把一次 `BeginTick` 的三个值放在一起；它不选择或
// 调用任何 production stage。
type fixtureTick struct {
	context  TickContext
	mutation *realm.Mutation
	result   TickResult
}

func NewEngine(_ int, worldTime uint64, seed int64) *Engine {
	state := NewState(seed)
	context := state.context(
		realm.NewState(core.Overworld),
		0,
		worldTime,
		0,
		tuning.ActiveTunables(),
		physics.ActiveTunables(),
		ViewSnapshot{},
	)
	return &Engine{engineContext: context}
}

// beginTick 只创建 realm mutation 并调用 production `State.BeginTick`。
// 每个测试必须在调用点显式选择所需阶段。
func (engine *Engine) beginTick() fixtureTick {
	engine.tunables = tuning.ActiveTunables()
	engine.physicsTunables = physics.ActiveTunables()
	views := engine.viewSnapshot()
	engine.engineContext.views = views
	mutation := engine.realm.NewMutation()
	return fixtureTick{
		context: engine.State.BeginTick(TickInput{
			Realm:           engine.realm,
			Tick:            engine.tick.Load(),
			WorldTime:       engine.worldTime.Load(),
			DayPhaseOffset:  engine.DayPhaseOffset(),
			Tunables:        engine.tunables,
			PhysicsTunables: engine.physicsTunables,
			Views:           views,
		}, mutation),
		mutation: mutation,
		result:   TickResult{},
	}
}

// commitMutation 只把 owner mutation 提交结果转为既有 contract DTO。
func commitMutation(pending *realm.Mutation, result *TickResult) {
	for _, batch := range pending.Commit() {
		changes := make([]BlockChange, len(batch.Changes))
		for index, change := range batch.Changes {
			changes[index] = BlockChange{Position: change.Position, Block: change.Block}
		}
		result.Changes = append(result.Changes, ChunkChangeBatch{
			Dimension: batch.Dimension, Chunk: batch.Chunk,
			BaseRevision: batch.BaseRevision, NewRevision: batch.NewRevision,
			Changes: changes,
		})
	}
}

// advanceFixtureClock 只推进测试值时钟，不发布状态或调用任何阶段。
func (engine *Engine) advanceFixtureClock() {
	engine.tick.Add(1)
	engine.worldTime.Add(1)
}

func (engine *Engine) viewSnapshot() ViewSnapshot {
	entries := engine.viewEntries[:0]
	for id := range engine.sessions {
		subscription, ok := engine.State.SessionSubscription(id)
		if !ok {
			continue
		}
		origin := core.ChunkKey{
			Dimension: subscription.Dimension,
			Pos:       subscription.Center,
		}
		entry := TickSessionView{
			Session: id,
			View: SessionView{
				Ready:  true,
				Center: subscription.Center,
			},
			Origin:       origin,
			OriginWanted: true,
		}
		for _, override := range engine.viewOverrides {
			if override.Session == id {
				entry = override
				break
			}
		}
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Session < entries[j].Session })
	engine.viewEntries = entries
	return NewViewSnapshot(entries)
}

// setSessionViewForTest 只覆盖测试传给 production stage 的只读订阅快照。
func (engine *Engine) setSessionViewForTest(
	id SessionID,
	view SessionView,
	originWanted bool,
) {
	subscription, _ := engine.State.SessionSubscription(id)
	entry := TickSessionView{
		Session: id,
		View:    view,
		Origin: core.ChunkKey{
			Dimension: subscription.Dimension,
			Pos:       subscription.Center,
		},
		OriginWanted: originWanted,
	}
	for index := range engine.viewOverrides {
		if engine.viewOverrides[index].Session == id {
			engine.viewOverrides[index] = entry
			engine.engineContext.views = engine.viewSnapshot()
			return
		}
	}
	engine.viewOverrides = append(engine.viewOverrides, entry)
	engine.engineContext.views = engine.viewSnapshot()
}

func (engine *engineContext) newMutation() *pendingChunkChanges {
	return engine.realm.NewMutation()
}

func (engine *engineContext) finishChanges(
	pending *pendingChunkChanges,
	result *TickResult,
) {
	commitMutation(pending, result)
}

func (engine *Engine) SeedForTest() int64 { return engine.seed }

func (engine *Engine) SetWorldTimeForTest(value uint64) {
	engine.worldTime.Store(value)
}

func (engine *Engine) TouchChunkForTest(key core.ChunkKey) {
	if dimension := engine.dimension(key.Dimension); dimension != nil {
		dimension.Touch(key.Pos)
	}
}

func (engine *Engine) enqueueFluidUpdate(
	dimension core.DimensionID,
	position core.BlockPos,
) {
	engine.realm.EnqueueFluidUpdate(dimension, position)
}

func (engine *Engine) CloneReadyChunk(
	key core.ChunkKey,
) (*world.Chunk, uint64, bool) {
	dimension := engine.dimension(key.Dimension)
	if dimension == nil {
		return nil, 0, false
	}
	return dimension.CloneReadyChunk(key.Pos)
}

func (engine *Engine) ChunkHash(key core.ChunkKey) ([32]byte, uint64, bool) {
	chunk, revision, ok := engine.CloneReadyChunk(key)
	if !ok {
		return [32]byte{}, 0, false
	}
	return chunk.Hash(), revision, true
}

func (engine *Engine) ChunkInfo(key core.ChunkKey) (ChunkInfo, bool) {
	dimension := engine.dimension(key.Dimension)
	if dimension == nil {
		return ChunkInfo{}, false
	}
	info, ok := dimension.Info(key.Pos)
	return ChunkInfo{
		State:                ChunkState(info.State),
		Revision:             info.Revision,
		PersistedRevision:    info.PersistedRevision,
		SaveInFlightRevision: info.SaveInFlightRevision,
		Err:                  info.Err,
	}, ok
}
