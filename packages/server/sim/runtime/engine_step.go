package runtime

import (
	"sort"

	"github.com/channing771/mornlea/packages/server/sim/entity"
	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/tuning"
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
	return engine.StepWithTunables(ActiveTickTunables())
}

// StepWithTunables 使用调用方在 tick 边界捕获的参数束推进一个权威 tick。
func (engine *Engine) StepWithTunables(tickTunables TickTunables) TickResult {
	engine.tunables = tickTunables.Simulation
	engine.physicsTunables = tickTunables.Physics
	currentTick := engine.tick.Load()
	currentWorldTime := engine.worldTime.Load()
	config := realmEnvironmentConfig(engine.tunables)
	// 气候束随环境参数一并下摆：积雪/消融在环境阶段（tick 中段）执行，此刻的
	// 权威天气与（tick 起点世界时间、当前显示偏移）的季节快照就是本 tick 的判定
	// 输入。与 tick 尾部发布用的（推进后时间、发布后偏移）快照刻意区分——环境
	// 推进必须与它正在推进的世界状态同一时刻，跳夜结算的偏移写者在其后。
	climate := engine.seasonSnapshotAt(currentWorldTime, engine.DayPhaseOffset())
	config.Weather = engine.weatherKind
	config.YearPhase = climate.yearPhase
	config.EffectiveDayPhase = climate.effectiveDayPhase
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
	// 被动牛紧随夜行者推进：同属有界实体阶段，`AdvancePassives` 内部已按
	// 生成→移动→死亡结算的固定时序编排，死亡掉落复用同一 `mutation`。
	tick.AdvancePassives()
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
	// 雪层脚印与耕地踩踏同域结算：两类收集都发生在更早的阶段（玩家物理、被
	// 动牛推进），统一在随机 tick 之前落块，本 tick 被削低的雪层不再参与抽样。
	tick.SettleSnowFootprints()
	engine.realm.AdvanceCrops(active, pending)

	tick.FinishWorld(&result)
	// 支撑复核的固定顺序：wild grass → torch → bed。wild plant sweep 先清
	// 失去草皮支撑的短草（同 mutation 零掉落），火把与床复核随后在自己的
	// 稳定快照里看到这些新变更——顺序颠倒会让落在短草上的火把/床悬空残留。
	engine.realm.SweepUnsupportedWildPlants(pending)
	engine.realm.SweepUnsupportedTorches(pending)
	engine.realm.SweepUnsupportedBeds(pending)
	finishRealmMutation(pending, &result)
	sortChunkKeys(result.Ready)

	result.Tick = currentTick + 1
	result.WorldTimeTicks = currentWorldTime + 1
	// 天气与绝对时间同源同频下发：先算出本 tick 结束时的权威值进结果，
	// 发布（`Publish` 内按人复制）之后再落回引擎——与 `advanceWorldTime`
	// 先后发布后推进的模式一致。
	nextWeatherKind, nextWeatherRemaining := advanceWeatherClock(
		engine.weatherKind, engine.weatherRemaining, engine.seed, result.Tick,
	)
	result.WeatherKind = nextWeatherKind
	engine.dayPhaseOffset.Store(uint64(tick.Publish(&result)))
	result.Tick = engine.tick.Add(1)
	result.WorldTimeTicks = engine.advanceWorldTime()
	engine.weatherKind, engine.weatherRemaining = result.WeatherKind, nextWeatherRemaining
	// 季节派生与天气同族，但必须在发布之后取快照：effPhase 依赖的显示偏移
	// 可能已被本 tick 的跳夜结算改写，`tick.Publish` 返回并落回引擎的值才是
	// 随本份 `PlayerUpdate.DayPhaseOffset` 一致下发的权威偏移。以本 tick 结束
	// 时的（时间、偏移）求一次季节束，世界单值写进结果、逐人温度按各玩家
	// Position.Y 与当 tick 天气补写进已组装好的玩家更新——发布侧随后照常
	// 整批复制，消费方无需感知两步组装。
	season := engine.seasonSnapshotAt(result.WorldTimeTicks, engine.DayPhaseOffset())
	result.Season = season.season
	result.SeasonProgress = season.progress
	for index := range result.Players {
		result.Players[index].Season = season.season
		result.Players[index].SeasonProgress = season.progress
		result.Players[index].Temperature = season.temperatureAt(
			result.WeatherKind, result.Players[index].State.Position.Y(),
		)
	}
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
