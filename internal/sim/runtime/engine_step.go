package runtime

import (
	"sort"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/entity"
	"github.com/channing771/mornlea/internal/sim/realm"
	"github.com/channing771/mornlea/internal/sim/tuning"
)

// stepPhase 标识权威 tick 的固定阶段顺序。
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

func (engine *Engine) notifyStepPhase(phase stepPhase) {
	if engine.stepPhaseObserver != nil {
		engine.stepPhaseObserver(phase)
	}
}

func realmEnvironmentConfig(tunables tuning.Tunables) realm.EnvironmentConfig {
	return realm.EnvironmentConfig{
		FluidFlowDelayTicks:     tunables.FluidFlowDelayTicks,
		FluidUpdatesPerTick:     tunables.FluidUpdatesPerTick,
		FluidRescanCellsPerTick: tunables.FluidRescanCellsPerTick,
		DropPickupDelayTicks:    tunables.DropPickupDelayTicks,
		RandomTicksPerSection:   tunables.RandomTicksPerSection,
		CropGrowthChancePercent: tunables.CropGrowthChancePercent,
	}
}

// Step 严格串行编排一个权威 tick；实体状态只经 `entity.State` 推进。
func (engine *Engine) Step() TickResult {
	engine.tunables = tuning.ActiveTunables()
	engine.physicsTunables = physics.ActiveTunables()
	currentTick := engine.tick.Load()
	currentWorldTime := engine.worldTime.Load()
	config := realmEnvironmentConfig(engine.tunables)
	engine.realm.SetEnvironmentTick(currentTick, engine.seed, config)
	commands, acquired, generated := engine.takeInbox()
	companionActions := engine.takeCompanionActions()
	sort.SliceStable(commands, func(i, j int) bool {
		if commands[i].Session != commands[j].Session {
			return commands[i].Session < commands[j].Session
		}
		return commands[i].Sequence < commands[j].Sequence
	})

	entityCommands := make([]Command, 0, len(commands))
	viewChanged := engine.subscriptionsDirty
	engine.subscriptionsDirty = false
	for _, command := range commands {
		session := engine.subscriptions[command.Session]
		if session == nil {
			continue
		}
		if command.Kind == CommandTrustedObserverCenter {
			if !session.trustedObserver || command.Sequence <= session.lastTrustedObserverSequence {
				continue
			}
			session.lastTrustedObserverSequence = command.Sequence
			session.hasView = true
			session.dimension = command.Dimension
			session.center = command.Center
			viewChanged = true
			continue
		}
		if session.trustedObserver || command.Sequence <= session.lastSequence {
			continue
		}
		session.lastSequence = command.Sequence
		entityCommands = append(entityCommands, command)
	}

	result := TickResult{Forget: make(map[SessionID][]core.ChunkKey)}
	pending := engine.realm.NewMutation()
	tick := engine.entities.BeginTick(entity.TickInput{
		Realm:           engine.realm,
		Tick:            currentTick,
		WorldTime:       currentWorldTime,
		DayPhaseOffset:  engine.DayPhaseOffset(),
		Tunables:        engine.tunables,
		PhysicsTunables: engine.physicsTunables,
		Views:           engine.entityViewSnapshot(),
	}, pending)

	engine.notifyStepPhase(phasePlayerCommands)
	tick.ApplyPlayerCommands(entityCommands, &result)
	engine.notifyStepPhase(phaseCompanionActions)
	tick.ApplyCompanionActions(companionActions)

	var currentWanted map[core.ChunkKey]struct{}
	if len(acquired) != 0 || len(generated) != 0 {
		currentWanted = engine.wantedSnapshot()
	}
	engine.applyAcquired(acquired, currentWanted, &result)
	engine.applyGenerated(generated, currentWanted, &result)

	engine.notifyStepPhase(phasePhysicsAdvance)
	entityViewChanged := tick.AdvanceActors()
	if viewChanged || entityViewChanged || engine.subscriptionsDirty {
		engine.subscriptionsDirty = false
		engine.reconcileSubscriptions(&result)
		tick.SetViews(engine.entityViewSnapshot())
	}

	engine.notifyStepPhase(phaseHostileAdvance)
	hostileActions := engine.takeHostileActions()
	tick.AdvanceHostiles(hostileActions, &result)
	tick.SettleGameplay(&result)

	engine.activeChunkScratch = tick.AppendActiveInterestKeys(engine.activeChunkScratch[:0])
	active := engine.activeChunkScratch
	engine.notifyStepPhase(phaseFluidAdvance)
	engine.realm.AdvanceFluids(active, pending)
	engine.notifyStepPhase(phaseFarmlandMoistureAdvance)
	environment := engine.realm.NewEnvironmentMutation(pending, currentTick, config)
	engine.realm.AdvanceFarmlandMoisture(active, environment)
	engine.notifyStepPhase(phaseCropAdvance)
	tick.SettleTramples()
	engine.realm.AdvanceCrops(active, pending)

	tick.FinishWorld(&result)
	engine.realm.SweepUnsupportedTorches(pending)
	engine.realm.SweepUnsupportedBeds(pending)
	finishRealmMutation(pending, &result)
	sortChunkKeys(result.Ready)

	result.Tick = currentTick + 1
	result.WorldTimeTicks = currentWorldTime + 1
	engine.dayPhaseOffset.Store(uint64(tick.Publish(&result)))
	result.Tick = engine.tick.Add(1)
	result.WorldTimeTicks = engine.advanceWorldTime()
	return result
}

func finishRealmMutation(pending *realm.Mutation, result *TickResult) {
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
