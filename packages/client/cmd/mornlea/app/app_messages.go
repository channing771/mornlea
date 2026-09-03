//go:build darwin

package app

import (
	"log/slog"
	"runtime"

	"github.com/channing771/mornlea/packages/client/audio"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render/hud"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

func (a *Application) DrainServerMessages(maxMessages int) {
	if maxMessages <= 0 || a.clientSessionClosed {
		return
	}
	for range maxMessages {
		if a.receiver == nil {
			return
		}
		message, ok := a.receiver.TryRecv()
		if !ok {
			runtime.Gosched()
			message, ok = a.receiver.TryRecv()
			if !ok {
				return
			}
		}
		if state, ok := message.(network.PlayerState); ok {
			if state.ServerTick <= a.serverTick {
				continue
			}
			source := client.MirrorCollisionSource{
				Mirror:    a.mirror,
				Dimension: core.Overworld,
			}
			result, err := a.predictor.ApplyPlayerState(state, source)
			if err != nil {
				a.CloseClientSession(err)
				return
			}
			if state.Reset {
				a.audioFeedback.Reset()
				a.combatFeedback.Reset()
				// 权威 reset 是会话边界：hud 分节纪律层丢弃旧基线，回到游戏
				// 相位后的第一次冲刷无条件下行一份完整分节。
				a.resetHUDStatePush()
			} else {
				// 浸没标志在权威位置上对只读镜像就地求值，与预测共用
				// `physics.SubmersionFlags` 唯一实现；缺块按干燥（宁可漏响不假响）。
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
				} else {
					a.inventorySource = -1
				}
			}
			if result.ResetView {
				a.camera.Yaw = result.Yaw
				a.camera.Pitch = result.Pitch
			}
			continue
		}
		if hit, ok := message.(network.CombatHit); ok {
			if a.combatFeedback.Observe(hit.ServerTick) {
				a.playLocalCue(audio.CueCombatHit)
				// marker 武装是 hud 分节变化源：显隐由 WebView 组件按下行驱动。
				a.hudPush.Mark()
			}
			continue
		}
		if state, ok := message.(network.InventoryState); ok {
			if err := a.inventory.Apply(state); err != nil {
				a.CloseClientSession(err)
				return
			}
			// 背包镜像确认（栏位物品、数量、耐久与选中下标）驱动快捷栏与容器
			// 内容的 hud 分节。
			a.hudPush.Mark()
			if cue, play := a.audioFeedback.ObserveInventoryState(state); play {
				a.playLocalCue(cue)
			}
			continue
		}
		if success, ok := message.(network.PlaceBlockSucceeded); ok {
			if cue, play := a.audioFeedback.ObservePlacementSuccess(success); play {
				a.playLocalCue(cue)
			}
			continue
		}
		if state, ok := message.(network.FurnaceState); ok {
			previous, opened := a.furnace.Ref()
			if err := a.furnace.Apply(state); err != nil {
				a.CloseClientSession(err)
				return
			}
			// 容器开关与内容变化都进入 hud 分节（containerOpen 布局位与熔炉
			// 保留面内容）。
			a.hudPush.Mark()
			// 熔炉与箱子互斥：新熔炉状态到达时丢弃可能过期的箱子镜像，
			// 否则两个镜像会同时报告 opened，点击分流会用错容器。
			a.chest.Reset()
			if !opened || previous != state.Furnace {
				a.inventorySource = -1
			}
			a.inventoryOpen = true
			if a.window != nil {
				a.window.SetCursorCaptured(false)
			}
			continue
		}
		if state, ok := message.(network.ChestState); ok {
			previous, opened := a.chest.Ref()
			if err := a.chest.Apply(state); err != nil {
				a.CloseClientSession(err)
				return
			}
			a.hudPush.Mark()
			a.furnace.Reset()
			if !opened || previous != state.Chest {
				a.inventorySource = -1
			}
			a.inventoryOpen = true
			if a.window != nil {
				a.window.SetCursorCaptured(false)
			}
			continue
		}
		if state, ok := message.(network.CraftingState); ok {
			previous, confirmed := a.crafting.State()
			if err := a.crafting.Apply(state); err != nil {
				a.CloseClientSession(err)
				return
			}
			a.hudPush.Mark()
			switch {
			case state.Size == 3 && (!confirmed || previous.Size != 3):
				// 尺寸 3 只能来自工作台交互：与熔炉/箱子到达时一致，打开
				// 界面、释放鼠标并丢弃可能过期的容器镜像；工作台打开也会
				// 结束既有容器查看关系（sim 侧语义），镜像同步互斥。
				a.furnace.Reset()
				a.chest.Reset()
				a.inventorySource = -1
				a.inventoryOpen = true
				if a.window != nil {
					a.window.SetCursorCaptured(false)
				}
			case confirmed && previous.Size == 3 && state.Size != 3 && a.inventoryOpen:
				// 网格尺寸降级 = 服务端关闭通知（显式关闭后的回收、离开
				// 距离或工作台被挖）：关闭界面、清除来源并重新捕获鼠标。
				// 个人网格镜像保留，后续尺寸 2 状态继续 latest-wins。
				a.clearContainerUI()
			}
			continue
		}
		if closed, ok := message.(network.ContainerClosed); ok {
			furnaceCurrent, furnaceOpened := a.furnace.Ref()
			if err := a.furnace.Close(closed); err != nil {
				a.CloseClientSession(err)
				return
			}
			chestCurrent, chestOpened := a.chest.Ref()
			if err := a.chest.Close(closed); err != nil {
				a.CloseClientSession(err)
				return
			}
			if (furnaceOpened && furnaceCurrent == closed.Container) ||
				(chestOpened && chestCurrent == closed.Container) {
				a.clearContainerUI()
			}
			a.hudPush.Mark()
			continue
		}
		switch message.(type) {
		case network.ItemDropUpserts, network.ItemDropRemoves:
			if err := a.itemDrops.Apply(message); err != nil {
				a.CloseClientSession(err)
				return
			}
			continue
		}
		switch message.(type) {
		case network.RemotePlayerSpawn, *network.RemotePlayerSpawn,
			network.RemotePlayerDespawn, *network.RemotePlayerDespawn,
			network.RemotePlayerStates, *network.RemotePlayerStates:
			if err := a.remotePlayers.Apply(message); err != nil {
				a.CloseClientSession(err)
				return
			}
			continue
		}
		switch message := message.(type) {
		case network.CompanionSpawn:
			if err := a.companions.ApplySpawn(message); err != nil {
				a.CloseClientSession(err)
				return
			}
			continue
		case network.CompanionStates:
			if err := a.companions.ApplyStates(message); err != nil {
				a.CloseClientSession(err)
				return
			}
			continue
		case network.CompanionDespawn:
			if err := a.companions.ApplyDespawn(message); err != nil {
				a.CloseClientSession(err)
				return
			}
			continue
		case network.HostileSpawn:
			if err := a.hostiles.ApplySpawn(message); err != nil {
				a.CloseClientSession(err)
				return
			}
			continue
		case network.HostileState:
			if err := a.hostiles.ApplyStates(message); err != nil {
				a.CloseClientSession(err)
				return
			}
			continue
		case network.HostileDespawn:
			if err := a.hostiles.ApplyDespawn(message); err != nil {
				a.CloseClientSession(err)
				return
			}
			continue
		case network.ChatEvent:
			if err := a.chatEvents.Apply(message); err != nil {
				a.CloseClientSession(err)
				return
			}
			// 聊天行缓冲是 hud 分节的一部分：新事件确认后行缓冲随之变化。
			a.hudPush.Mark()
			continue
		}
		changes, isBlockChanges := message.(network.BlockChanges)
		blockChangesApplied := false
		if isBlockChanges {
			chunk, loaded := a.mirror.Chunk(changes.Dimension, changes.Chunk)
			blockChangesApplied = loaded && !chunk.Desynced && chunk.Revision == changes.BaseRevision
		}
		update, err := a.mirror.Apply(message)
		if err != nil {
			a.CloseClientSession(err)
			return
		}
		if isBlockChanges && blockChangesApplied {
			if cue, play := a.audioFeedback.ObserveBlockChanges(changes); play {
				a.playLocalCue(cue)
			}
		}
		switch message := message.(type) {
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
			update.Resync.Sequence = a.nextSequence()
			if err := a.send(update.Resync); err != nil {
				slog.Warn("发送区块 resync 失败", "error", err)
			}
		}
		if update.Rejected != nil {
			slog.Warn("权威命令被拒绝",
				"sequence", update.Rejected.Sequence, "reason", update.Rejected.Reason)
		}
		if a.mesher != nil {
			a.mesher.MarkDirty(update.Dirty...)
		}
		for _, key := range update.Forgotten {
			if key.Dimension != core.Overworld {
				continue
			}
			a.scheduler.QueueSection(key.Pos, nil)
			if key.Pos.Y == 0 {
				a.mesher.ForgetChunk(key.Dimension, core.ChunkPos{X: key.Pos.X, Z: key.Pos.Z})
			}
		}
	}
}
