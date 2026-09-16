//go:build darwin

package app

import (
	"log/slog"

	"github.com/channing771/mornlea/packages/client/audio"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/client/render/hud"
	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// DrainServerMessages consumes at most maxMessages server messages through the
// adopted runtime's bounded drain. The runtime applies every mirror family, the
// predictor reconciliation, and stale-tick acceptance in place; this loop only
// performs the app-owned presentation side effects (audio cues, hud/game-panel
// dirty marks, container UI, cursor capture, environment presentation values)
// triggered from the returned outcomes.
func (a *Application) DrainServerMessages(maxMessages int) {
	if maxMessages <= 0 || a.clientSessionClosed {
		return
	}
	session := a.ensureSessionRuntime()
	if session == nil {
		return
	}
	var drainErr error
	a.drainOutcomes, drainErr = session.DrainMessages(a.drainOutcomes[:0], maxMessages)
	for _, outcome := range a.drainOutcomes {
		if a.applyServerMessageOutcome(outcome) {
			return
		}
	}
	if drainErr != nil {
		a.CloseClientSession(drainErr)
		return
	}
	a.sequence = session.Sequence()
}

// applyServerMessageOutcome performs the app-owned side effects of one drained
// message outcome. It reports true when the session was closed mid-drain and
// remaining outcomes must be skipped.
func (a *Application) applyServerMessageOutcome(outcome clientruntime.MessageOutcome) bool {
	if !outcome.Handled {
		return a.applyUnhandledServerMessage(outcome.Message)
	}
	switch message := outcome.Message.(type) {
	case network.PlayerState:
		a.applyPlayerStateOutcome(message, outcome)
		return false
	case network.InventoryState:
		a.gameUIDirty = true
		// 背包镜像确认（栏位物品、数量、耐久与选中下标）驱动快捷栏与容器
		// 内容的 hud 分节。
		a.hudPush.Mark()
		if cue, play := a.audioFeedback.ObserveInventoryState(message); play {
			a.playLocalCue(cue)
		}
		return false
	case network.FurnaceState, network.ChestState:
		a.gameUIDirty = true
		// 容器开关与内容变化都进入 hud 分节（containerOpen 布局位与熔炉
		// 保留面内容）。
		a.hudPush.Mark()
		// 熔炉与箱子互斥：新熔炉状态到达时丢弃可能过期的箱子镜像，
		// 否则两个镜像会同时报告 opened，点击分流会用错容器。
		if outcome.Container == clientruntime.ContainerTransitionOpenFurnace ||
			outcome.Container == clientruntime.ContainerTransitionOpenChest {
			a.inventoryOpen = true
			if a.window != nil {
				a.window.SetCursorCaptured(false)
			}
		}
		return false
	case network.CraftingState:
		a.gameUIDirty = true
		a.hudPush.Mark()
		switch outcome.Container {
		case clientruntime.ContainerTransitionOpenCrafting:
			// 尺寸 3 只能来自工作台交互：与熔炉/箱子到达时一致，打开
			// 界面、释放鼠标并丢弃可能过期的容器镜像；工作台打开也会
			// 结束既有容器查看关系（sim 侧语义），镜像同步互斥。
			a.inventoryOpen = true
			if a.window != nil {
				a.window.SetCursorCaptured(false)
			}
		case clientruntime.ContainerTransitionClose:
			// 网格尺寸降级 = 服务端关闭通知（显式关闭后的回收、离开
			// 距离或工作台被挖）：关闭界面、清除来源并重新捕获鼠标。
			// 个人网格镜像保留，后续尺寸 2 状态继续 latest-wins。
			if a.inventoryOpen {
				a.clearContainerUI()
			}
		}
		return false
	case network.ContainerClosed:
		if outcome.Container == clientruntime.ContainerTransitionClose {
			a.clearContainerUI()
		}
		a.hudPush.Mark()
		return false
	case network.ChatEvent:
		// 聊天行缓冲是 hud 分节的一部分：新事件确认后行缓冲随之变化。
		a.hudPush.Mark()
		return false
	}
	if outcome.Changes.Has(clientruntime.MirrorChangeWorld) {
		return a.applyWorldMirrorOutcome(outcome)
	}
	return false
}

// applyPlayerStateOutcome applies the presentation effects of one accepted
// authoritative player state. A stale or duplicate state (`PredictionChanged`
// false) produced no mirror or prediction change, so it produces no presentation
// change either, matching the legacy tick guard.
func (a *Application) applyPlayerStateOutcome(
	state network.PlayerState,
	outcome clientruntime.MessageOutcome,
) {
	if !outcome.PredictionChanged {
		return
	}
	if state.Reset {
		a.audioFeedback.Reset()
		a.combatFeedback.Reset()
		// 权威 reset 是会话边界：双手编码器的边沿状态与 marker 同步丢弃，
		// 否则残留攻击窗会在重生后继续挥动。
		a.ResetViewmodel()
		// 权威 reset（重生/传送）同样重锚踩雪呈现：旧位置的步频里程与
		// 踢雪事件锚点不得在重生点继续出声/扬尘。
		a.snowStep.Reset()
		a.frameSnowKick = render.SnowKickInput{}
		a.entityEncoder.ResetSnowKicks()
		// 权威 reset 是会话边界：hud 分节纪律层丢弃旧基线，回到游戏
		// 相位后的第一次冲刷无条件下行一份完整分节。
		a.resetHUDStatePush()
	} else {
		// 浸没标志在权威位置上对只读镜像就地求值，与预测共用
		// `physics.SubmersionFlags` 唯一实现；缺块按干燥（宁可漏响不假响）。
		source := client.MirrorCollisionSource{
			Mirror:    a.mirror,
			Dimension: core.Overworld,
		}
		_, bodyInFluid := physics.SubmersionFlags(state.Position, source)
		eatingCompleted, damaged, splashed := a.audioFeedback.ObservePlayerState(state, bodyInFluid)
		if eatingCompleted {
			a.playLocalCue(audio.CueEatingComplete)
		}
		if damaged {
			a.playLocalCue(audio.CueDamage)
		}
		if splashed {
			a.playLocalCue(audio.CueWaterSplash)
		}
	}
	a.serverTick = state.ServerTick
	// 权威状态确认（生命/饥饿/氧气/采掘进度与世界时间都在同一份消息里）
	// 是 hud 分节的主要变化源：置脏交纪律层合并，同一 tick 内的多次变化
	// 至多下行一次。
	a.hudPush.Mark()
	// 世界时间与显示相位偏移来自同一份权威状态、同一接受纪律（上面的
	// ServerTick 守卫已挡掉旧/重复状态）：偏移只平移昼夜呈现相位。
	// 冻结开关(capture 钉住天空状态)只拦这两个呈现量,其余权威状态
	// 照常前进——冻结不改变接受纪律本身。
	if !a.worldTimeFrozen {
		a.worldTimeTicks = state.WorldTimeTicks
		a.dayPhaseOffset = state.DayPhaseOffset
		// 天气与世界时间同一呈现量纪律：只认更新 tick（上面的
		// ServerTick 守卫），冻结开关一并钉住降水/天空/亮度输入。
		a.weather = state.WeatherKind
		// 季节三字段与天气同一接受纪律、同一冻结开关：昼夜 warp 与
		// 降水形态呈现只消费这份权威镜像，不读本地推算。
		a.season = state.Season
		a.seasonProgress = state.SeasonProgress
		a.temperature = state.Temperature
	}
	if state.Reset || !state.MiningActive {
		a.miningOverlay = hud.MiningOverlay{}
	} else {
		// Target/HasTarget 供世界空间裂纹呈现定位权威目标方块；
		// HasTarget 恒随 MiningActive 置位，服务端契约保证 active
		// 时 MiningTarget 有效。可采标志的唯一消费方（屏幕采掘条）
		// 已退役，协议侧 MiningHarvestable 不再进入镜像。
		a.miningOverlay = hud.MiningOverlay{
			Active:        true,
			Target:        state.MiningTarget,
			HasTarget:     state.MiningActive,
			ProgressTicks: state.MiningProgressTicks,
			RequiredTicks: state.MiningRequiredTicks,
		}
	}
	if state.Reset {
		a.blockTargetReset = true
		if a.containerOpen() {
			a.clearContainerUI()
		}
	}
	if outcome.Reconcile.ResetView {
		a.camera.Yaw = outcome.Reconcile.Yaw
		a.camera.Pitch = outcome.Reconcile.Pitch
	}
}

// applyWorldMirrorOutcome applies the app-owned effects of one accepted world
// mirror update: confirmed block-change audio, loaded-chunk bookkeeping, resync
// dispatch, and rejection logging. Dirty marking, chunk forgetting, and section
// drop publication live in the adopted runtime's mesh pipeline; the drained
// section results reach the scheduler in `RenderFrame`.
func (a *Application) applyWorldMirrorOutcome(outcome clientruntime.MessageOutcome) bool {
	update := outcome.World
	if outcome.Changes.Has(clientruntime.MirrorChangeBlockChangesApplied) {
		if changes, ok := outcome.Message.(network.BlockChanges); ok {
			if cue, play := a.audioFeedback.ObserveBlockChanges(changes); play {
				a.playLocalCue(cue)
			}
		}
	}
	switch message := outcome.Message.(type) {
	case network.ChunkSnapshot:
		if message.Dimension == core.Overworld {
			a.loadedChunks[message.Chunk] = struct{}{}
		}
	case network.ForgetChunks:
		if message.Dimension == core.Overworld {
			for _, position := range message.Chunks {
				delete(a.loadedChunks, position)
			}
		}
	}
	if update.Resync != nil {
		if err := a.send(update.Resync); err != nil {
			slog.Warn("发送区块 resync 失败", "error", err)
		}
	}
	if update.Rejected != nil {
		slog.Warn("权威命令被拒绝",
			"sequence", update.Rejected.Sequence, "reason", update.Rejected.Reason)
	}
	// The scheduler drops forgotten sections at message time (legacy timing) so a
	// warp-scale forget reclaims uploads immediately; the runtime additionally
	// queues payload-less drop operations whose later drain is an idempotent
	// no-op here. Dirty marking and chunk forgetting live in the runtime mesh
	// pipeline.
	for _, key := range update.Forgotten {
		if key.Dimension != core.Overworld {
			continue
		}
		a.scheduler.QueueSection(key.Pos, nil)
	}
	return false
}

// applyUnhandledServerMessage preserves the legacy routing for message classes
// the runtime does not own yet: placement audio, combat feedback, and the world
// mirror catch-all that fails the session on unknown messages.
func (a *Application) applyUnhandledServerMessage(message network.ServerMessage) bool {
	if success, ok := message.(network.PlaceBlockSucceeded); ok {
		if cue, play := a.audioFeedback.ObservePlacementSuccess(success); play {
			a.playLocalCue(cue)
		}
		return false
	}
	if hit, ok := message.(network.CombatHit); ok {
		if a.combatFeedback.Observe(hit.ServerTick) {
			a.playLocalCue(audio.CueCombatHit)
			// marker 武装是 hud 分节变化源：显隐由 WebView 组件按下行驱动。
			a.hudPush.Mark()
		}
		return false
	}
	if _, err := a.mirror.Apply(message); err != nil {
		a.CloseClientSession(err)
		return true
	}
	return false
}
