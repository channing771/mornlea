package runtime

import (
	"sort"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim/entity"
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

// Step 严格串行编排一个权威 tick；实体状态只经 `entity.State` 推进。
func (engine *Engine) Step() TickResult {
	engine.tunables = tuning.ActiveTunables()
	engine.physicsTunables = physics.ActiveTunables()
	commands, acquired, generated := engine.takeInbox()
	companionActions := engine.takeCompanionActions()
	hostileActions := engine.takeHostileActions()
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

	output := engine.entities.Step(entity.StepInput{
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
		Hooks: entity.StepHooks{
			PlayerCommands: func() { engine.notifyStepPhase(phasePlayerCommands) },
			CompanionActions: func() {
				engine.notifyStepPhase(phaseCompanionActions)
			},
			ApplyChunks: func(result *TickResult) {
				var currentWanted map[core.ChunkKey]struct{}
				if len(acquired) != 0 || len(generated) != 0 {
					currentWanted = engine.wantedSnapshot()
				}
				engine.applyAcquired(acquired, currentWanted, result)
				engine.applyGenerated(generated, currentWanted, result)
			},
			PhysicsAdvance: func() { engine.notifyStepPhase(phasePhysicsAdvance) },
			ReconcileSubscriptions: func(entityChanged bool, result *TickResult) {
				if !viewChanged && !entityChanged && !engine.subscriptionsDirty {
					return
				}
				engine.subscriptionsDirty = false
				engine.reconcileSubscriptions(result)
			},
			HostileAdvance: func() { engine.notifyStepPhase(phaseHostileAdvance) },
			FluidAdvance:   func() { engine.notifyStepPhase(phaseFluidAdvance) },
			FarmlandAdvance: func() {
				engine.notifyStepPhase(phaseFarmlandMoistureAdvance)
			},
			CropAdvance:    func() { engine.notifyStepPhase(phaseCropAdvance) },
			ActiveInterest: engine.activeInterestKeys,
		},
	})

	engine.dayPhaseOffset.Store(uint64(output.DayPhaseOffset))
	engine.syncRealmTestMirrors()
	result := output.Result
	result.Tick = engine.tick.Add(1)
	result.WorldTimeTicks = engine.advanceWorldTime()
	return result
}

// syncRealmTestMirrors 维持既有包内环境测试探针；权威队列与统计仍只存在 realm。
func (engine *Engine) syncRealmTestMirrors() {
	if scope := engine.realm.FluidScope(); scope != nil {
		if engine.fluidScope == nil {
			engine.fluidScope = make(map[core.ChunkKey]struct{}, len(scope))
		} else {
			clear(engine.fluidScope)
		}
		for key := range scope {
			engine.fluidScope[key] = struct{}{}
		}
	}
	engine.fluidQueues = engine.realm.FluidQueuesMap()
	engine.cropCellsExamined, engine.cropBlockReads = engine.realm.CropStats()
}
