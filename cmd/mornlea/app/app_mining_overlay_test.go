//go:build darwin

package app

// app_mining_overlay_test.go：采掘 overlay 只由权威 PlayerState 驱动，reset/断线清理。

import (
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/render/hud"
	"github.com/go-gl/mathgl/mgl32"
	"testing"
)

// 杀死变异：从本地按键推进采掘条或忽略 inactive 权威状态都会改变镜像。
func TestApplicationMiningOverlayUsesOnlyConfirmedPlayerState(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	state := network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		MiningActive: true, MiningTarget: core.BlockPos{X: 1, Y: 10, Z: 2},
		MiningProgressTicks: 6, MiningRequiredTicks: 15, MiningHarvestable: true,
	}
	sendInteractiveServerMessage(t, serverEndpoint, state)
	app.DrainServerMessages(1)
	want := hud.MiningOverlay{
		Active: true, ProgressTicks: 6, RequiredTicks: 15, Harvestable: true,
	}
	if app.miningOverlay != want {
		t.Fatalf("权威采掘镜像=%+v，想要 %+v", app.miningOverlay, want)
	}

	for range 2 {
		app.applyInteractiveInput(
			physics.FixedDelta, client.Movement{}, client.Actions{Mining: true}, true,
		)
		if _, ok := receiveInteractiveClientMessage(t, serverEndpoint).(network.PlayerInput); !ok {
			t.Fatal("本地按住没有发送持续输入")
		}
		if app.miningOverlay != want {
			t.Fatalf("无新 PlayerState 时本地输入改写采掘镜像: %+v", app.miningOverlay)
		}
	}

	inactive := state
	inactive.ServerTick = 2
	inactive.MiningActive = false
	inactive.MiningTarget = core.BlockPos{}
	inactive.MiningProgressTicks = 0
	inactive.MiningRequiredTicks = 0
	inactive.MiningHarvestable = false
	sendInteractiveServerMessage(t, serverEndpoint, inactive)
	app.DrainServerMessages(1)
	if app.miningOverlay != (hud.MiningOverlay{}) {
		t.Fatalf("inactive 后采掘镜像=%+v，想要零值", app.miningOverlay)
	}
}

// 杀死变异：旧或重复 PlayerState 不得回滚 app 的已确认 tick、采掘条或 reset 生命周期。
func TestApplicationMiningOverlayIgnoresStaleAndEqualPlayerState(t *testing.T) {
	app, serverEndpoint := newInteractiveTestApplication(t)
	active := network.PlayerState{
		ServerTick: 2, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
		MiningActive: true, MiningTarget: core.BlockPos{X: 1, Y: 10, Z: 2},
		MiningProgressTicks: 6, MiningRequiredTicks: 15, MiningHarvestable: true,
	}
	sendInteractiveServerMessage(t, serverEndpoint, active)
	app.DrainServerMessages(1)
	want := hud.MiningOverlay{
		Active: true, ProgressTicks: 6, RequiredTicks: 15, Harvestable: true,
	}
	app.inventoryOpen = true
	app.inventorySource = 8

	for _, tick := range []uint64{1, 2} {
		sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
			ServerTick: tick, Dimension: core.Overworld,
			Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true, Reset: true,
		})
		app.DrainServerMessages(1)
		if app.serverTick != 2 || app.miningOverlay != want {
			t.Fatalf("tick=%d 后 app tick/overlay=%d/%+v，想要 2/%+v",
				tick, app.serverTick, app.miningOverlay, want)
		}
		if !app.inventoryOpen || app.inventorySource != 8 {
			t.Fatalf("tick=%d 的旧 reset 改写界面: open=%v source=%d",
				tick, app.inventoryOpen, app.inventorySource)
		}
	}

	newer := active
	newer.ServerTick = 3
	newer.MiningProgressTicks = 7
	sendInteractiveServerMessage(t, serverEndpoint, newer)
	app.DrainServerMessages(1)
	if app.serverTick != 3 || app.miningOverlay.ProgressTicks != 7 {
		t.Fatalf("更新状态未生效: tick/overlay=%d/%+v", app.serverTick, app.miningOverlay)
	}
}

// 杀死变异：reset 或连接关闭遗漏清理会把上一会话进度留在下一帧。
func TestApplicationMiningOverlayClearsOnResetAndSessionClose(t *testing.T) {
	for _, test := range []struct {
		name  string
		clear func(*Application, network.ServerEndpoint)
	}{
		{
			name: "Reset",
			clear: func(app *Application, endpoint network.ServerEndpoint) {
				sendInteractiveServerMessage(t, endpoint, network.PlayerState{
					ServerTick: 2, Dimension: core.Overworld,
					Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true, Reset: true,
				})
				app.DrainServerMessages(1)
			},
		},
		{name: "关闭会话", clear: func(app *Application, _ network.ServerEndpoint) {
			app.CloseClientSession(nil)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			app, serverEndpoint := newInteractiveTestApplication(t)
			sendInteractiveServerMessage(t, serverEndpoint, network.PlayerState{
				ServerTick: 1, Dimension: core.Overworld,
				Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true, Ready: true,
				MiningActive: true, MiningTarget: core.BlockPos{X: 1, Y: 10, Z: 2},
				MiningProgressTicks: 6, MiningRequiredTicks: 15, MiningHarvestable: true,
			})
			app.DrainServerMessages(1)
			if !app.miningOverlay.Active {
				t.Fatal("测试前置没有建立 active 权威采掘镜像")
			}

			test.clear(app, serverEndpoint)
			if app.miningOverlay != (hud.MiningOverlay{}) {
				t.Fatalf("清理后采掘镜像=%+v，想要零值", app.miningOverlay)
			}
		})
	}
}
