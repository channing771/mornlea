package entity

import (
	"math"
	"sort"

	"github.com/channing771/mornlea/internal/companion"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/sim/tuning"
	"github.com/channing771/mornlea/internal/world"
)

// Engine 是迁入 entity 的白盒玩法测试夹具，不参与生产构建。
type Engine struct {
	*engineContext
	viewRadius         int
	views              map[SessionID]testSessionView
	wanted             map[core.ChunkKey]struct{}
	observers          map[SessionID]*testObserver
	lastSequences      map[SessionID]uint64
	commands           []Command
	companionActions   []CompanionAction
	hostileActions     []HostileAction
	acquired           []AcquiredChunk
	generated          []GeneratedChunk
	subscriptionsDirty bool
	stepPhaseObserver  func(stepPhase)
}

type testObserver struct {
	lastSequence uint64
	view         testSessionView
}

type testSessionView struct {
	SessionView
	Wanted map[core.ChunkKey]struct{}
}

type stepPhase uint8

const (
	phasePlayerCommands stepPhase = iota + 1
	phaseCompanionActions
	phasePhysicsAdvance
	phaseHostileAdvance
	phaseFluidAdvance
	phaseFarmlandMoistureAdvance
	phaseCropAdvance
)

func NewEngine(viewRadius int, worldTime uint64, seed int64) *Engine {
	context := NewState(seed).context(
		realm.NewState(core.Overworld),
		0,
		worldTime,
		0,
		tuning.ActiveTunables(),
		physics.ActiveTunables(),
		nil,
	)
	engine := &Engine{
		engineContext: context,
		viewRadius:    viewRadius,
		views:         make(map[SessionID]testSessionView),
		wanted:        make(map[core.ChunkKey]struct{}),
		observers:     make(map[SessionID]*testObserver),
		lastSequences: make(map[SessionID]uint64),
	}
	context.views = engine
	return engine
}

func (engine *Engine) EntitySessionView(id SessionID) SessionView {
	return engine.views[id].SessionView
}

func (engine *Engine) EntitySessionWantsChunk(
	id SessionID,
	key core.ChunkKey,
) bool {
	_, wanted := engine.views[id].Wanted[key]
	return wanted
}

func (engine *Engine) Enqueue(command Command) {
	engine.commands = append(engine.commands, command)
}

func (engine *Engine) SubmitAcquired(chunk AcquiredChunk) {
	engine.acquired = append(engine.acquired, chunk)
}

func (engine *Engine) SubmitGenerated(chunk GeneratedChunk) {
	engine.generated = append(engine.generated, chunk)
}

func (engine *Engine) EnqueueCompanionAction(action CompanionAction) bool {
	if len(engine.companionActions) >= companion.MaxActive {
		return false
	}
	engine.companionActions = append(engine.companionActions, action)
	return true
}

func (engine *Engine) EnqueueHostileAction(action HostileAction) bool {
	if len(engine.hostileActions) >= maxHostiles {
		return false
	}
	engine.hostileActions = append(engine.hostileActions, action)
	return true
}

func (engine *Engine) WantsChunk(key core.ChunkKey) bool {
	_, ok := engine.wanted[key]
	return ok
}

func (engine *Engine) RegisterObserverSession(id SessionID) {
	if engine.sessions[id] != nil || engine.observers[id] != nil {
		panic("sim: duplicate registered session")
	}
	engine.observers[id] = &testObserver{}
}

func (engine *Engine) UnregisterSession(id SessionID) (PlayerSnapshot, bool) {
	if engine.observers[id] != nil {
		delete(engine.observers, id)
		delete(engine.lastSequences, id)
		engine.subscriptionsDirty = true
		return PlayerSnapshot{}, false
	}
	snapshot, ok := engine.engineContext.UnregisterSession(id)
	delete(engine.views, id)
	delete(engine.lastSequences, id)
	engine.subscriptionsDirty = true
	return snapshot, ok
}

func (engine *Engine) TickCount() uint64 { return engine.tick.Load() }

func (engine *Engine) SeedForTest() int64 { return engine.seed }

func (engine *Engine) SetWorldTimeForTest(value uint64) {
	engine.worldTime.Store(value)
}

func (engine *Engine) SetDayPhaseOffsetForTest(value uint16) {
	engine.dayPhaseOffset.Store(uint64(value))
}

func (engine *Engine) RestoreDayPhaseOffset(value uint16) {
	engine.dayPhaseOffset.Store(uint64(value))
}

func (engine *Engine) Step() TickResult {
	engine.tunables = tuning.ActiveTunables()
	engine.physicsTunables = physics.ActiveTunables()
	commands := append([]Command(nil), engine.commands...)
	companionActions := append([]CompanionAction(nil), engine.companionActions...)
	hostileActions := append([]HostileAction(nil), engine.hostileActions...)
	acquired := append([]AcquiredChunk(nil), engine.acquired...)
	generated := append([]GeneratedChunk(nil), engine.generated...)
	engine.commands = engine.commands[:0]
	engine.companionActions = engine.companionActions[:0]
	engine.hostileActions = engine.hostileActions[:0]
	engine.acquired = engine.acquired[:0]
	engine.generated = engine.generated[:0]
	sort.SliceStable(commands, func(i, j int) bool {
		if commands[i].Session != commands[j].Session {
			return commands[i].Session < commands[j].Session
		}
		return commands[i].Sequence < commands[j].Sequence
	})
	entityCommands := make([]Command, 0, len(commands))
	observerChanged := false
	for _, command := range commands {
		if observer := engine.observers[command.Session]; observer != nil {
			if command.Kind != CommandTrustedObserverCenter ||
				command.Sequence <= observer.lastSequence {
				continue
			}
			observer.lastSequence = command.Sequence
			observer.view.Ready = true
			observer.view.Center = command.Center
			observer.view.Wanted = make(map[core.ChunkKey]struct{})
			for dz := -engine.viewRadius; dz <= engine.viewRadius; dz++ {
				for dx := -engine.viewRadius; dx <= engine.viewRadius; dx++ {
					observer.view.Wanted[core.ChunkKey{
						Dimension: command.Dimension,
						Pos: core.ChunkPos{
							X: command.Center.X + int32(dx),
							Z: command.Center.Z + int32(dz),
						},
					}] = struct{}{}
				}
			}
			observerChanged = true
			continue
		}
		if engine.sessions[command.Session] == nil ||
			command.Sequence <= engine.lastSequences[command.Session] {
			continue
		}
		engine.lastSequences[command.Session] = command.Sequence
		entityCommands = append(entityCommands, command)
	}

	output := engine.State.Step(StepInput{
		Realm:            engine.realm,
		Tick:             engine.tick.Load(),
		WorldTime:        engine.worldTime.Load(),
		DayPhaseOffset:   engine.DayPhaseOffset(),
		Tunables:         engine.tunables,
		PhysicsTunables:  engine.physicsTunables,
		Views:            engine,
		Commands:         entityCommands,
		CompanionActions: companionActions,
		HostileActions:   hostileActions,
		Hooks: StepHooks{
			PlayerCommands:   func() { engine.notifyPhase(phasePlayerCommands) },
			CompanionActions: func() { engine.notifyPhase(phaseCompanionActions) },
			ApplyChunks: func(result *TickResult) {
				engine.applyTestAcquired(acquired, result)
				engine.applyTestGenerated(generated, result)
			},
			PhysicsAdvance: func() { engine.notifyPhase(phasePhysicsAdvance) },
			ReconcileSubscriptions: func(changed bool, result *TickResult) {
				if changed || observerChanged || engine.subscriptionsDirty ||
					len(engine.views) != len(engine.sessions) {
					engine.reconcileTestSubscriptions(result)
					engine.subscriptionsDirty = false
				}
			},
			HostileAdvance: func() { engine.notifyPhase(phaseHostileAdvance) },
			FluidAdvance:   func() { engine.notifyPhase(phaseFluidAdvance) },
			FarmlandAdvance: func() {
				engine.notifyPhase(phaseFarmlandMoistureAdvance)
			},
			CropAdvance: func() { engine.notifyPhase(phaseCropAdvance) },
			ActiveInterest: func() []core.ChunkKey {
				return engine.activeInterestKeys()
			},
		},
	})
	engine.dayPhaseOffset.Store(uint64(output.DayPhaseOffset))
	engine.tick.Add(1)
	engine.worldTime.Add(1)
	return output.Result
}

func (engine *Engine) notifyPhase(phase stepPhase) {
	if engine.stepPhaseObserver != nil {
		engine.stepPhaseObserver(phase)
	}
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

func (engine *Engine) reconcileTestSubscriptions(result *TickResult) {
	union := make(map[core.ChunkKey]struct{})
	for id := range engine.sessions {
		subscription, ok := engine.State.SessionSubscription(id)
		if !ok {
			continue
		}
		previous := engine.views[id]
		view := testSessionView{
			SessionView: SessionView{
				Ready:  true,
				Center: subscription.Center,
			},
			Wanted: make(map[core.ChunkKey]struct{}),
		}
		if engine.dimension(subscription.Dimension) != nil {
			for dz := -engine.viewRadius; dz <= engine.viewRadius; dz++ {
				for dx := -engine.viewRadius; dx <= engine.viewRadius; dx++ {
					key := core.ChunkKey{
						Dimension: subscription.Dimension,
						Pos: core.ChunkPos{
							X: subscription.Center.X + int32(dx),
							Z: subscription.Center.Z + int32(dz),
						},
					}
					view.Wanted[key] = struct{}{}
					union[key] = struct{}{}
				}
			}
		}
		for _, key := range subscription.Pending {
			view.Wanted[key] = struct{}{}
			union[key] = struct{}{}
		}
		for key := range previous.Wanted {
			if _, retained := view.Wanted[key]; !retained {
				result.Forget[id] = append(result.Forget[id], key)
			}
		}
		sortChunkKeys(result.Forget[id])
		engine.views[id] = view
	}
	for _, observer := range engine.observers {
		for key := range observer.view.Wanted {
			union[key] = struct{}{}
		}
	}
	engine.State.AddCompanionWanted(union)
	keys := make([]core.ChunkKey, 0, len(union))
	for key := range union {
		_, retained := engine.wanted[key]
		dimension := engine.dimension(key.Dimension)
		info, exists := dimension.Info(key.Pos)
		if !retained || exists && info.State == realm.ChunkFailed {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		leftDistance := engine.testSubscriptionDistanceSquared(keys[i])
		rightDistance := engine.testSubscriptionDistanceSquared(keys[j])
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		return chunkKeyLess(keys[i], keys[j])
	})
	for _, key := range keys {
		dimension := engine.dimension(key.Dimension)
		if dimension == nil {
			continue
		}
		if dimension.CancelUnload(key.Pos) {
			result.Ready = append(result.Ready, key)
			continue
		}
		if dimension.BeginLoading(key.Pos) {
			result.Acquire = append(result.Acquire, key)
		}
	}
	for key := range engine.wanted {
		if _, retained := union[key]; retained {
			continue
		}
		if dimension := engine.dimension(key.Dimension); dimension != nil {
			dimension.RequestUnload(key.Pos)
		}
	}
	engine.wanted = union
}

func (engine *Engine) testSubscriptionDistanceSquared(key core.ChunkKey) int64 {
	distance := int64(math.MaxInt64)
	for _, view := range engine.views {
		if _, wanted := view.Wanted[key]; !wanted {
			continue
		}
		dx := int64(key.Pos.X - view.Center.X)
		dz := int64(key.Pos.Z - view.Center.Z)
		distance = min(distance, dx*dx+dz*dz)
	}
	for _, observer := range engine.observers {
		if _, wanted := observer.view.Wanted[key]; !wanted {
			continue
		}
		dx := int64(key.Pos.X - observer.view.Center.X)
		dz := int64(key.Pos.Z - observer.view.Center.Z)
		distance = min(distance, dx*dx+dz*dz)
	}
	if candidate, relevant := engine.State.CompanionSubscriptionDistanceSquared(key); relevant {
		distance = min(distance, candidate)
	}
	return distance
}

func (engine *Engine) applyTestAcquired(
	chunks []AcquiredChunk,
	result *TickResult,
) {
	sort.SliceStable(chunks, func(i, j int) bool {
		return chunkKeyLess(chunks[i].Key, chunks[j].Key)
	})
	for _, acquired := range chunks {
		key := acquired.Key
		dimension := engine.dimension(key.Dimension)
		if dimension == nil {
			continue
		}
		info, ok := dimension.Info(key.Pos)
		if !ok || info.State != realm.ChunkLoading {
			continue
		}
		switch {
		case acquired.Err != nil:
			dimension.MarkLoadFailed(key.Pos, acquired.Err)
			if engine.State.CompanionWantsChunk(key) {
				engine.subscriptionsDirty = true
			}
		case acquired.Missing:
			if _, retained := engine.wanted[key]; !retained {
				dimension.DropLoading(key.Pos)
			} else if dimension.MarkGenerating(key.Pos) {
				result.Generate = append(result.Generate, key)
			}
		default:
			if err := dimension.ApplyLoaded(
				key.Pos,
				acquired.Chunk,
				acquired.Revision,
				acquired.PersistedRevision,
				acquired.NeedsRewrite,
				acquired.Recovered,
			); err != nil {
				dimension.MarkLoadFailed(key.Pos, err)
				if engine.State.CompanionWantsChunk(key) {
					engine.subscriptionsDirty = true
				}
				continue
			}
			result.Ready = append(result.Ready, key)
		}
	}
}

func (engine *Engine) applyTestGenerated(
	chunks []GeneratedChunk,
	result *TickResult,
) {
	sort.SliceStable(chunks, func(i, j int) bool {
		left := core.ChunkKey{Dimension: chunks[i].Dimension, Pos: chunks[i].Pos}
		right := core.ChunkKey{Dimension: chunks[j].Dimension, Pos: chunks[j].Pos}
		return chunkKeyLess(left, right)
	})
	for _, generated := range chunks {
		key := core.ChunkKey{Dimension: generated.Dimension, Pos: generated.Pos}
		dimension := engine.dimension(key.Dimension)
		if dimension == nil {
			continue
		}
		info, ok := dimension.Info(key.Pos)
		if !ok || info.State != realm.ChunkGenerating {
			continue
		}
		if generated.Err != nil {
			dimension.MarkFailed(key.Pos, generated.Err)
			if engine.State.CompanionWantsChunk(key) {
				engine.subscriptionsDirty = true
			}
			continue
		}
		if err := dimension.ApplyGenerated(key.Pos, generated.Chunk); err != nil {
			dimension.MarkFailed(key.Pos, err)
			if engine.State.CompanionWantsChunk(key) {
				engine.subscriptionsDirty = true
			}
			continue
		}
		result.Ready = append(result.Ready, key)
	}
}
