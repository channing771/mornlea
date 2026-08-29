//go:build darwin

package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/render/hud"
)

func (a *Application) Close() error {
	a.closeOnce.Do(func() {
		a.CloseClientSession(nil)
		a.closeErr = errors.Join(a.closeErr, a.clientCloseErr)
		if a.serverCancel != nil {
			a.serverCancel()
		}
		if a.serverDone != nil {
			if err := <-a.serverDone; err != nil && err != context.Canceled {
				a.closeErr = errors.Join(a.closeErr, err)
			}
		}
		if a.releaseResources != nil {
			a.releaseResources()
		}
	})
	return a.closeErr
}

func (a *Application) releaseOwnedResources() {
	// 音频先于窗口和渲染器关闭，避免系统队列在图形宿主已经释放后仍持有回调。
	if a.closeAudio != nil {
		a.closeAudio()
	}
	// layouter(名牌/HUD/面板)无 GPU 资源,无需释放;字形图集停 worker,
	// 渲染器句柄经 Close 归还 Rust。
	if a.glyphAtlas != nil {
		a.glyphAtlas.Release()
	}
	if a.mesher != nil {
		a.mesher.Close()
	}
	// 远环调度器先于渲染器关闭:Close 停 worker 并等待在途生成返回,之后
	// 不再有任何 sink 调用落到已释放的渲染器句柄上。幂等且 nil 安全。
	if a.lodScheduler != nil {
		a.lodScheduler.Close()
	}
	if a.renderer != nil {
		a.renderer.Close()
	}
	if a.window != nil {
		a.window.Close()
	}
}

// CloseClientSession closes only the current client endpoint. The embedded
// server belongs to the whole Application and is stopped exclusively by Close.
func (a *Application) CloseClientSession(cause error) {
	a.clientCloseOnce.Do(func() {
		if cause != nil {
			slog.Info("关闭客户端会话", "cause", cause)
		}
		if a.receiver != nil {
			a.clientCloseErr = a.receiver.Close()
		} else if a.clientEndpoint != nil {
			a.clientCloseErr = a.clientEndpoint.Close()
		}
		// 拆链发生在游戏内时聊天输入框可能开着——镜像复位后按原状态恢复光标。
		wasChatOpen := a.chatInput.open
		a.resetSessionOwnedState()
		if wasChatOpen && a.window != nil {
			a.window.SetCursorCaptured(true)
		}
	})
}

// resetSessionOwnedState 清空全部会话态：客户端镜像、伙伴/聊天事件环、输入框、
// 背包与容器视图、进食/受击呈现、掉落物等，并把 `clientSessionClosed` 置位。
//
// 它与端点关闭解耦成独立方法，因为拆链有两个入口共用同一份收摊语义：断线/
// 窗口关闭路径经 `CloseClientSession`（进程生命周期内一次），暂停页「退回主菜
// 单」直接调用本方法——后者必须允许跨开合周期重复执行（第二次进入游戏再退回
// 时 `clientCloseOnce` 已耗尽），同时把端点/接收器与 Host 生命周期的关闭交给
// `releaseWorldConnection` 负责。光标是否恢复由调用方按相位意图决定，不在本
// 方法内处理。
func (a *Application) resetSessionOwnedState() {
	if a.remotePlayers != nil {
		a.remotePlayers.Reset()
	}
	if a.companions != nil {
		a.companions.Reset()
	}
	if a.chatEvents != nil {
		a.chatEvents.Reset()
	}
	a.chatInput.Cancel()
	a.clearFormattedChatLines()
	// 容量与 client.ChatEventCapacity 同源（E9/C9）：与 Application 字段声明
	// 共用同一常量，断线重置不会因字面量漂移而改变缓冲长度。
	a.chatEventBuffer = [client.ChatEventCapacity]network.ChatEvent{}
	a.inventory.Reset()
	a.audioFeedback.Reset()
	a.combatFeedback.Reset()
	if a.hostiles != nil {
		a.hostiles.Reset()
	}
	a.furnace.Reset()
	a.chest.Reset()
	// 合成网格镜像随会话一并清空：断线后不继承任何已确认网格状态
	//（spec authoritative-grid-crafting「断线清空客户端镜像」）。
	a.crafting.Reset()
	a.miningOverlay = hud.MiningOverlay{}
	// 进食进度是纯呈现预测，随会话一起清零，不得漏进重连后的第一帧。
	a.eatingTracker.Reset()
	a.damageFeedback.Reset()
	a.damageStrength = 0
	a.inventoryOpen = false
	a.inventorySource = -1
	a.itemDrops.Reset()
	a.clientSessionClosed = true
}
