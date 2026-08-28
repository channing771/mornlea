package capture

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	application "github.com/channing771/mornlea/cmd/mornlea/app"
	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/render/hud"
)

func TestHUDCaptureScenesFixAllAuthoritativeSurvivalValues(t *testing.T) {
	hotbar := captureSceneByName(t, "hud-hotbar-health")
	if hotbar.HUD == nil || hotbar.HUD.Health != core.MaxHealth ||
		hotbar.HUD.Oxygen != core.MaxOxygenTicks || hotbar.HUD.Hunger != core.MaxHunger {
		t.Fatalf("hud-hotbar-health HUD=%+v，想要满生命、满氧气与满饥饿", hotbar.HUD)
	}

	for _, name := range []string{"inventory-crafting", "workbench-crafting", "chest-container", "furnace-container"} {
		scene := captureSceneByName(t, name)
		if scene.HUD == nil {
			t.Fatalf("%s 缺少已确认生命、耗损氧气与饥饿夹具", name)
		}
		if scene.HUD.Health != 5 {
			t.Fatalf("%s health=%d，想要低生命 5", name, scene.HUD.Health)
		}
		if scene.HUD.Oxygen != core.MaxOxygenTicks/3 {
			t.Fatalf("%s oxygen=%d，想要 %d", name, scene.HUD.Oxygen, core.MaxOxygenTicks/3)
		}
		if scene.HUD.Hunger != 9 {
			t.Fatalf("%s hunger=%d，想要 9", name, scene.HUD.Hunger)
		}
	}
}

func TestHUDCaptureSurvivalFeedbackFixtureRestoresState(t *testing.T) {
	scene := captureSceneByName(t, "hud-survival-feedback")
	wantFixture := captureHUDFixture{
		Health: 5,
		Oxygen: core.MaxOxygenTicks / 3,
		Hunger: 9,
		Mining: hud.MiningOverlay{
			Active: true, ProgressTicks: 4, RequiredTicks: 9, Harvestable: false,
		},
	}
	if scene.HUD == nil || *scene.HUD != wantFixture {
		t.Fatalf("HUD fixture=%+v，想要 %+v", scene.HUD, wantFixture)
	}

	wantState := physics.State{
		Position: mgl32.Vec3{1.25, 65, -3.5},
		Velocity: mgl32.Vec3{0.5, -0.25, 0.75},
		OnGround: true,
	}
	originalPredictor := client.NewPredictor()
	if err := originalPredictor.Begin(network.PlayerState{
		ServerTick: 7, Dimension: core.Overworld,
		Position: wantState.Position, Velocity: wantState.Velocity, OnGround: wantState.OnGround,
		Yaw: 0.75, Pitch: -0.2, Ready: true,
		Health: core.MaxHealth, Oxygen: core.MaxOxygenTicks, Hunger: 17,
	}); err != nil {
		t.Fatal(err)
	}
	originalMining := hud.MiningOverlay{
		Active: true, ProgressTicks: 1, RequiredTicks: 2, Harvestable: true,
	}
	app := &application.Application{}
	app.SetPredictor(originalPredictor)
	*app.Camera() = client.Camera{Yaw: 0, Pitch: -0.25}
	app.SetMiningOverlay(originalMining)

	restore, err := applyCaptureHUDFixture(app, scene.HUD)
	if err != nil {
		t.Fatal(err)
	}
	if app.Predictor() == originalPredictor {
		t.Fatal("HUD fixture 没有使用独立 predictor")
	}
	if got, ready := app.Predictor().State(); !ready || got != wantState {
		t.Fatalf("fixture physics=%+v/%v，想要 %+v/true", got, ready, wantState)
	}
	if got, ready := app.Predictor().Health(); !ready || got != wantFixture.Health {
		t.Fatalf("fixture health=%d/%v，想要 %d/true", got, ready, wantFixture.Health)
	}
	if got, ready := app.Predictor().Oxygen(); !ready || got != wantFixture.Oxygen {
		t.Fatalf("fixture oxygen=%d/%v，想要 %d/true", got, ready, wantFixture.Oxygen)
	}
	if got, ready := app.Predictor().Hunger(); !ready || got != wantFixture.Hunger {
		t.Fatalf("fixture hunger=%d/%v，想要 %d/true", got, ready, wantFixture.Hunger)
	}
	if app.MiningOverlay() != wantFixture.Mining {
		t.Fatalf("fixture mining=%+v，想要 %+v", app.MiningOverlay(), wantFixture.Mining)
	}

	restore()
	if app.Predictor() != originalPredictor || app.MiningOverlay() != originalMining {
		t.Fatalf("首次 restore 后 predictor/mining=%p/%+v，想要 %p/%+v",
			app.Predictor(), app.MiningOverlay(), originalPredictor, originalMining)
	}
	restore()
	if app.Predictor() != originalPredictor || app.MiningOverlay() != originalMining {
		t.Fatalf("重复 restore 后 predictor/mining=%p/%+v，想要 %p/%+v",
			app.Predictor(), app.MiningOverlay(), originalPredictor, originalMining)
	}
	if got, ready := app.Predictor().Hunger(); !ready || got != 17 {
		t.Fatalf("重复 restore 后 hunger=%d/%v，想要 17/true", got, ready)
	}

	secondFixture := wantFixture
	secondFixture.Hunger = core.MaxHunger
	secondRestore, err := applyCaptureHUDFixture(app, &secondFixture)
	if err != nil {
		t.Fatalf("对同一 app 第二次 apply: %v", err)
	}
	if got, ready := app.Predictor().Hunger(); !ready || got != core.MaxHunger {
		t.Fatalf("第二次 apply hunger=%d/%v，想要 %d/true", got, ready, core.MaxHunger)
	}
	secondRestore()
	if app.Predictor() != originalPredictor || app.MiningOverlay() != originalMining {
		t.Fatal("第二次 apply/restore 没有恢复完整 predictor 与 mining")
	}
}

func TestHUDCaptureFixtureSuccessDeferAndErrorPreserveState(t *testing.T) {
	originalPredictor := client.NewPredictor()
	if err := originalPredictor.Begin(network.PlayerState{
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{2, 70, 3},
		Ready:     true,
		Health:    12,
		Oxygen:    222,
		Hunger:    13,
	}); err != nil {
		t.Fatal(err)
	}
	originalMining := hud.MiningOverlay{Active: true, ProgressTicks: 2, RequiredTicks: 5}
	app := &application.Application{}
	app.SetPredictor(originalPredictor)
	app.SetMiningOverlay(originalMining)

	func() {
		restore, err := applyCaptureHUDFixture(app, &captureHUDFixture{
			Health: 4, Oxygen: 100, Hunger: 7,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer restore()
		if app.Predictor() == originalPredictor {
			t.Fatal("成功 apply 没有安装临时 predictor")
		}
	}()
	if app.Predictor() != originalPredictor || app.MiningOverlay() != originalMining {
		t.Fatal("defer restore 没有恢复完整 predictor 与 mining")
	}

	restore, err := applyCaptureHUDFixture(app, &captureHUDFixture{
		Health: 4, Oxygen: 100, Hunger: core.MaxHunger + 1,
	})
	if err == nil || restore != nil {
		t.Fatalf("非法 hunger apply 的 error/restore nil=%v/%v，想要 true/true", err != nil, restore == nil)
	}
	if app.Predictor() != originalPredictor || app.MiningOverlay() != originalMining {
		t.Fatal("错误返回改变了 predictor 或 mining")
	}
	if got, ready := app.Predictor().Hunger(); !ready || got != 13 {
		t.Fatalf("错误返回后 hunger=%d/%v，想要 13/true", got, ready)
	}
}
