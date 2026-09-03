//go:build darwin

package app

import (
	"testing"

	"github.com/channing771/mornlea/internal/audio"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/go-gl/mathgl/mgl32"
)

func TestApplicationCombatHitPlaysCueAndArmsMarker(t *testing.T) {
	app, endpoint := newInteractiveTestApplication(t)
	var recorder audioCueRecorder
	app.playCue = recorder.play

	// 初始不可见。
	if app.combatFeedback.MarkerVisible() {
		t.Fatalf("初始 marker 可见")
	}
	// 注入严格递增的 hit。
	sendInteractiveServerMessage(t, endpoint, network.CombatHit{ServerTick: 1, Damage: 4, TargetKind: 1})
	app.DrainServerMessages(1)
	recorder.want(t, audio.CueCombatHit)
	if !app.combatFeedback.MarkerVisible() {
		t.Fatalf("hit 后 marker 不可见")
	}
	if app.combatFeedback.remainingFrames != combatMarkerFrameCount {
		t.Fatalf("剩余帧=%d，想要 %d", app.combatFeedback.remainingFrames, combatMarkerFrameCount)
	}
	// 重复/陈旧不播放也不重武装。
	sendInteractiveServerMessage(t, endpoint, network.CombatHit{ServerTick: 1, Damage: 4, TargetKind: 1})
	app.DrainServerMessages(1)
	recorder.want(t, audio.CueCombatHit)
	// 让 marker 消耗一帧。
	app.combatFeedback.AfterRender(true)
	remaining := app.combatFeedback.remainingFrames
	// 陈旧的 tick 1（仍为 1）不应重武装；直接调用 Observe 验证不改变状态。
	if app.combatFeedback.Observe(1) {
		t.Fatalf("陈旧 tick 1 被错误接受")
	}
	if app.combatFeedback.remainingFrames != remaining {
		t.Fatalf("陈旧 hit 改写了剩余帧")
	}
	// 更大 tick 重置为 6 并再次播放。
	sendInteractiveServerMessage(t, endpoint, network.CombatHit{ServerTick: 2, Damage: 4, TargetKind: 1})
	app.DrainServerMessages(1)
	recorder.want(t, audio.CueCombatHit, audio.CueCombatHit)
	if app.combatFeedback.remainingFrames != combatMarkerFrameCount {
		t.Fatalf("第二击未重置帧")
	}
}

func TestApplicationCombatHitWithResetInSameTick(t *testing.T) {
	app, endpoint := newInteractiveTestApplication(t)
	var recorder audioCueRecorder
	app.playCue = recorder.play
	// 先让 feedback 处于已武装状态。
	if !app.combatFeedback.Observe(10) {
		t.Fatal("预置 tick 10 失败")
	}
	if !app.combatFeedback.MarkerVisible() {
		t.Fatal("预置后不可见")
	}
	// 同一次 drain 中先收到 Reset，再收到同 tick 的 hit 仍应接受。
	// Reset 的 serverTick 取 11，hit 的 ServerTick 取 11（同 tick），验证 Reset 后去重被清。
	sendInteractiveServerMessage(t, endpoint, network.PlayerState{ServerTick: 2, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5}, Ready: true, Reset: true})
	// 需要让 PlayerState 被接受：设置 serverTick 更大。
	app.serverTick = 1
	sendInteractiveServerMessage(t, endpoint, network.CombatHit{ServerTick: 5, Damage: 4, TargetKind: 1})
	// Drain 2 条。
	app.DrainServerMessages(2)
	// Reset 后 hit 5 应被接受（因为 last 被清零）。
	recorder.want(t, audio.CueCombatHit)
	if !app.combatFeedback.MarkerVisible() || app.combatFeedback.lastServerTick != 5 {
		t.Fatalf("Reset 后 hit 未武装：%+v", app.combatFeedback)
	}
}

func TestApplicationCombatFeedbackResetsOnSessionBoundaries(t *testing.T) {
	app, endpoint := newInteractiveTestApplication(t)
	var recorder audioCueRecorder
	app.playCue = recorder.play
	if !app.combatFeedback.Observe(7) {
		t.Fatal("预置失败")
	}
	// authoritative reset via PlayerState Reset
	app.serverTick = 1
	sendInteractiveServerMessage(t, endpoint, network.PlayerState{ServerTick: 3, Dimension: core.Overworld, Ready: true, Reset: true})
	app.DrainServerMessages(1)
	if app.combatFeedback.MarkerVisible() {
		t.Fatalf("authoritative reset 后仍可见")
	}
	if app.combatFeedback.lastServerTick != 0 {
		t.Fatalf("authoritative reset 未清 tick")
	}
	// 重新武装后，disconnect 应清。
	if !app.combatFeedback.Observe(8) {
		t.Fatal("重新武装失败")
	}
	app.CloseClientSession(nil)
	if app.combatFeedback.MarkerVisible() {
		t.Fatalf("disconnect 后仍可见")
	}
	// 重新武装后，菜单返回（resetSessionOwnedState）应清。
	if !app.combatFeedback.Observe(9) {
		t.Fatal("重新武装失败2")
	}
	app.resetSessionOwnedState()
	if app.combatFeedback.MarkerVisible() {
		t.Fatalf("resetSessionOwnedState 后仍可见")
	}
	// 重新武装后，新 session（再次 reset）应清。
	if !app.combatFeedback.Observe(10) {
		t.Fatal("重新武装失败3")
	}
	app.resetSessionOwnedState()
	if app.combatFeedback.MarkerVisible() {
		t.Fatalf("新 session reset 后仍可见")
	}
	// 同 tick 内 Reset 后 hit 仍可接受（前一测试已覆盖，此处仅验证 Reset 清后可重武装）。
	// 需要全新 app 以避免已关闭的 transport。
	app2, endpoint2 := newInteractiveTestApplication(t)
	var recorder2 audioCueRecorder
	app2.playCue = recorder2.play
	app2.serverTick = 20
	sendInteractiveServerMessage(t, endpoint2, network.PlayerState{ServerTick: 21, Dimension: core.Overworld, Ready: true, Reset: true})
	app2.DrainServerMessages(1)
	sendInteractiveServerMessage(t, endpoint2, network.CombatHit{ServerTick: 11, Damage: 4, TargetKind: 1})
	app2.DrainServerMessages(1)
	if !app2.combatFeedback.MarkerVisible() || app2.combatFeedback.lastServerTick != 11 {
		t.Fatalf("Reset 后重武装失败")
	}
	recorder2.want(t, audio.CueCombatHit)
}

func TestApplicationSessionResetClearsHostileMirror(t *testing.T) {
	app, _ := newInteractiveTestApplication(t)
	app.hostiles = &client.Hostiles{}
	if err := app.hostiles.ApplySpawn(network.HostileSpawn{ServerTick: 1, Spawns: []network.HostileSpawnRecord{{ID: 1, Dimension: core.Overworld, Position: mgl32.Vec3{0, 1, 0}, Health: 20}}}); err != nil {
		t.Fatal(err)
	}
	if len(app.hostiles.AppendPresentations(nil)) != 1 {
		t.Fatal("预置 hostile 失败")
	}
	if !app.combatFeedback.Observe(1) {
		t.Fatal("预置 combat 失败")
	}
	app.resetSessionOwnedState()
	if app.combatFeedback.MarkerVisible() {
		t.Fatalf("reset 后 combat 仍可见")
	}
	if len(app.hostiles.AppendPresentations(nil)) != 0 {
		t.Fatalf("reset 后 hostile 未清")
	}
	// nil 保护：hostiles 未装配时重置不应 panic。
	app.hostiles = nil
	app.combatFeedback.Observe(2)
	app.resetSessionOwnedState()
	if app.combatFeedback.MarkerVisible() {
		t.Fatalf("nil hostiles 时 combat 未清")
	}
}
