package entity

// publishFixture 只发布当前 entity 状态并推进夹具时钟；它不调用
// 玩法、环境或提交阶段。
func publishFixture(engine *Engine, tick *fixtureTick) TickResult {
	tick.result.Tick = engine.tick.Load() + 1
	tick.result.WorldTimeTicks = engine.worldTime.Load() + 1
	engine.dayPhaseOffset.Store(uint64(tick.context.Publish(&tick.result)))
	engine.advanceFixtureClock()
	return tick.result
}

// advanceActorsTick 只运行 production actor 阶段。
func advanceActorsTick(engine *Engine) TickResult {
	tick := engine.beginTick()
	tick.context.AdvanceActors()
	return publishFixture(engine, &tick)
}

// applyPlayerCommandsTick 只运行 production 玩家命令阶段。
func applyPlayerCommandsTick(engine *Engine, commands []Command) TickResult {
	tick := engine.beginTick()
	tick.context.ApplyPlayerCommands(commands, &tick.result)
	return publishFixture(engine, &tick)
}

// settlePlayerInteractionsTick 只把玩家命令收集与其产生的世界交互结算
// 串在同一个 `TickContext`，不推进 actor、hostile 或 realm 环境。
func settlePlayerInteractionsTick(engine *Engine, commands []Command) TickResult {
	tick := engine.beginTick()
	tick.context.ApplyPlayerCommands(commands, &tick.result)
	tick.context.SettleGameplay(&tick.result)
	commitMutation(tick.mutation, &tick.result)
	return publishFixture(engine, &tick)
}

// finishPlayerWorldTick 只把玩家命令收集与 container/mining/workbench
// 结算串在同一个 `TickContext`，不运行其他阶段。
func finishPlayerWorldTick(engine *Engine, commands []Command) TickResult {
	tick := engine.beginTick()
	tick.context.ApplyPlayerCommands(commands, &tick.result)
	tick.context.FinishWorld(&tick.result)
	commitMutation(tick.mutation, &tick.result)
	return publishFixture(engine, &tick)
}

// applyCompanionActionsTick 只运行 production 伙伴 action 阶段。
func applyCompanionActionsTick(engine *Engine, actions []CompanionAction) TickResult {
	tick := engine.beginTick()
	tick.context.ApplyCompanionActions(actions)
	return publishFixture(engine, &tick)
}

// advanceHostilesTick 只运行 production hostile 阶段并提交该阶段的写入。
func advanceHostilesTick(engine *Engine, actions []HostileAction) TickResult {
	tick := engine.beginTick()
	tick.context.AdvanceHostiles(actions, &tick.result)
	commitMutation(tick.mutation, &tick.result)
	return publishFixture(engine, &tick)
}

// settleGameplayTick 只运行 production gameplay settlement 阶段并提交写入。
func settleGameplayTick(engine *Engine) TickResult {
	tick := engine.beginTick()
	tick.context.SettleGameplay(&tick.result)
	commitMutation(tick.mutation, &tick.result)
	return publishFixture(engine, &tick)
}

// finishWorldTick 只运行 production container/mining/workbench 阶段并提交写入。
func finishWorldTick(engine *Engine) TickResult {
	tick := engine.beginTick()
	tick.context.FinishWorld(&tick.result)
	commitMutation(tick.mutation, &tick.result)
	return publishFixture(engine, &tick)
}
