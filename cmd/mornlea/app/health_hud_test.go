//go:build darwin

package app

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// health_hud_test.go：生命/氧气/饥饿的权威镜像语义在 hud 分节组装层的端到端
// 覆盖。呈现已迁 WebView HUD 组件，GPU 保留面不再消费这些值；「只消费已确认
// 权威值、未确认完全隐藏、reset 清空」的契约由 `assembleHUDState` 与
// `internal/client` 的构造器共同承担，这里走 wire → Predictor → 组装 的完整路径。

// beginConfirmedSurvival 注入一份权威 PlayerState，把 Predictor 的生存镜像设为
// 给定值，供各用例钉住「确认值是什么、满值/未确认是否缺席」。
func beginConfirmedSurvival(t *testing.T, app *Application, tick uint64, health uint8, oxygen uint16, hunger uint8) {
	t.Helper()
	if err := app.predictor.Begin(network.PlayerState{
		ServerTick: tick, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true,
		Ready: true, Health: health, Oxygen: oxygen, Hunger: hunger,
	}); err != nil {
		t.Fatal(err)
	}
}

// Mutation killed: forwarding a predicted/stale Health value, or failing to clear
// Health after an authoritative player-state reset would let the HUD show a
// number the server never confirmed.
func TestHUDStateHealthReflectsOnlyConfirmedPredictorState(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.inventory.Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
		t.Fatal(err)
	}

	// 收到权威状态之前：Predictor 尚未就绪，hud 分节不得携带任何生命值。
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("未确认生命值 RenderFrame=(%v,%v)", rendered, err)
	}
	if state := app.assembleHUDState(); state.Health != nil {
		t.Fatalf("未确认生命值下行了分节: %+v", state.Health)
	}

	// 收到生命值为 12 的权威状态：hud 分节携带确认值。
	beginConfirmedSurvival(t, app, 1, 12, core.MaxOxygenTicks, core.MaxHunger)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("已确认生命值 RenderFrame=(%v,%v)", rendered, err)
	}
	state := app.assembleHUDState()
	if state.Health == nil || state.Health.Value != 12 {
		t.Fatalf("确认生命值 12 的分节=%+v，想要 12", state.Health)
	}

	// 权威玩家状态 reset（Ready=false）：即使背包镜像仍然确认，生命值也必须
	// 清空，不能继续下行断线前的陈旧数值。
	if _, err := app.predictor.ApplyPlayerState(network.PlayerState{
		ServerTick: 2, Dimension: core.Overworld, Ready: false,
	}, client.MirrorCollisionSource{Mirror: app.mirror, Dimension: core.Overworld}); err != nil {
		t.Fatal(err)
	}
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("玩家状态 reset 后 RenderFrame=(%v,%v)", rendered, err)
	}
	if state := app.assembleHUDState(); state.Health != nil {
		t.Fatalf("玩家状态 reset 后生命分节=%+v，想要缺席", state.Health)
	}
}

// Mutation killed: keeping the survival sections alive after the client session
// closes would show values the current session never confirmed.
func TestHUDStateSurvivalHiddenAfterDisconnect(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.inventory.Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
		t.Fatal(err)
	}
	beginConfirmedSurvival(t, app, 1, 12, core.MaxOxygenTicks, core.MaxHunger)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("已确认状态 RenderFrame=(%v,%v)", rendered, err)
	}
	if state := app.assembleHUDState(); state.Health == nil || state.Hotbar == nil {
		t.Fatalf("确认状态的分节缺席: hotbar=%v health=%v", state.Hotbar, state.Health)
	}

	app.CloseClientSession(nil)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("断线后 RenderFrame=(%v,%v)", rendered, err)
	}
	state := app.assembleHUDState()
	if state.Hotbar != nil || state.Health != nil || state.Hunger != nil || state.Oxygen != nil {
		t.Fatalf("断线后仍携带生存分节: hotbar=%v health=%v hunger=%v oxygen=%v",
			state.Hotbar, state.Health, state.Hunger, state.Oxygen)
	}
	// 会话关闭同时退出 hud 分节下行窗口：回到游戏相位后的第一次冲刷必须
	// 无条件下行完整分节（此处以窗口位表达）。
	if app.hudPushInWindow {
		t.Fatal("断线后 hud 分节下行窗口仍开启")
	}
}

// TestHUDStateOxygenFollowsAuthoritativePlayerState 锁定氧气的异常态语义：
// 满氧与未确认都不产生分节，耗损时按确认值下行（气泡数由前端按十格等分解析）。
func TestHUDStateOxygenFollowsAuthoritativePlayerState(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	if err := app.inventory.Apply(network.InventoryState{Inventory: core.Inventory{}}); err != nil {
		t.Fatal(err)
	}
	tick := uint64(1)
	apply := func(oxygen uint16) *client.UIHudOxygen {
		t.Helper()
		tick++
		if _, err := app.predictor.ApplyPlayerState(network.PlayerState{
			ServerTick: tick, Dimension: core.Overworld,
			Position: mgl32.Vec3{0.5, 10, 0.5}, OnGround: true,
			Ready: true, Health: 12, Oxygen: oxygen, Hunger: core.MaxHunger,
		}, client.MirrorCollisionSource{Mirror: app.mirror, Dimension: core.Overworld}); err != nil {
			t.Fatal(err)
		}
		if rendered, err := app.RenderFrame(1); err != nil || !rendered {
			t.Fatalf("氧气 %d RenderFrame=(%v,%v)", oxygen, rendered, err)
		}
		return app.assembleHUDState().Oxygen
	}

	beginConfirmedSurvival(t, app, 1, 12, core.MaxOxygenTicks, core.MaxHunger)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("满氧 RenderFrame=(%v,%v)", rendered, err)
	}
	if state := app.assembleHUDState(); state.Oxygen != nil {
		t.Fatalf("满氧下行了分节: %+v", state.Oxygen)
	}

	if oxygen := apply(core.MaxOxygenTicks / 2); oxygen == nil || oxygen.Value != core.MaxOxygenTicks/2 {
		t.Fatalf("半氧分节=%+v，想要 %d", oxygen, core.MaxOxygenTicks/2)
	}
	if oxygen := apply(core.MaxOxygenTicks / 4); oxygen == nil || oxygen.Value != core.MaxOxygenTicks/4 {
		t.Fatalf("四分之一氧分节=%+v，想要 %d", oxygen, core.MaxOxygenTicks/4)
	}
	if oxygen := apply(core.MaxOxygenTicks); oxygen != nil {
		t.Fatalf("氧气回满后分节=%+v，想要缺席", oxygen)
	}
}

// TestHUDStateHungerFollowsAuthoritativePlayerState 锁定饥饿的权威语义：未收到
// 权威状态时分节缺席；收到之后携带 `Hunger` 而不是同一条消息里的 `Health`。
func TestHUDStateHungerFollowsAuthoritativePlayerState(t *testing.T) {
	app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})

	// 收到权威状态之前：Predictor 未就绪，饥饿值未确认，分节缺席。
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("未确认权威状态 RenderFrame=(%v,%v)", rendered, err)
	}
	if state := app.assembleHUDState(); state.Hunger != nil || state.Health != nil {
		t.Fatalf("未确认权威状态下行了生存分节: hunger=%v health=%v", state.Hunger, state.Health)
	}

	beginConfirmedSurvival(t, app, 1, 14, core.MaxOxygenTicks, 9)
	if rendered, err := app.RenderFrame(1); err != nil || !rendered {
		t.Fatalf("已确认权威状态 RenderFrame=(%v,%v)", rendered, err)
	}
	state := app.assembleHUDState()
	if state.Hunger == nil || state.Hunger.Value != 9 {
		t.Fatalf("饥饿分节=%+v，想要 9", state.Hunger)
	}
	if state.Health == nil || state.Health.Value != 14 {
		t.Fatalf("生命分节=%+v，想要 14（两条分节各取各的确认值）", state.Health)
	}
}
