package main

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/render/hud"
)

func TestHUDCaptureSurvivalFeedbackFixtureRestoresState(t *testing.T) {
	scene := captureSceneByName(t, "hud-survival-feedback")
	wantFixture := captureHUDFixture{
		Health: 5,
		Oxygen: core.MaxOxygenTicks / 3,
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
		Health: core.MaxHealth, Oxygen: core.MaxOxygenTicks,
	}); err != nil {
		t.Fatal(err)
	}
	originalMining := hud.MiningOverlay{
		Active: true, ProgressTicks: 1, RequiredTicks: 2, Harvestable: true,
	}
	app := &application{
		predictor:     originalPredictor,
		camera:        client.Camera{Yaw: 0, Pitch: -0.25},
		miningOverlay: originalMining,
	}

	restore, err := applyCaptureHUDFixture(app, scene.HUD)
	if err != nil {
		t.Fatal(err)
	}
	if app.predictor == originalPredictor {
		t.Fatal("HUD fixture 没有使用独立 predictor")
	}
	if got, ready := app.predictor.State(); !ready || got != wantState {
		t.Fatalf("fixture physics=%+v/%v，想要 %+v/true", got, ready, wantState)
	}
	if got, ready := app.predictor.Health(); !ready || got != wantFixture.Health {
		t.Fatalf("fixture health=%d/%v，想要 %d/true", got, ready, wantFixture.Health)
	}
	if got, ready := app.predictor.Oxygen(); !ready || got != wantFixture.Oxygen {
		t.Fatalf("fixture oxygen=%d/%v，想要 %d/true", got, ready, wantFixture.Oxygen)
	}
	if app.miningOverlay != wantFixture.Mining {
		t.Fatalf("fixture mining=%+v，想要 %+v", app.miningOverlay, wantFixture.Mining)
	}

	restore()
	if app.predictor != originalPredictor || app.miningOverlay != originalMining {
		t.Fatalf("首次 restore 后 predictor/mining=%p/%+v，想要 %p/%+v",
			app.predictor, app.miningOverlay, originalPredictor, originalMining)
	}
	restore()
	if app.predictor != originalPredictor || app.miningOverlay != originalMining {
		t.Fatalf("重复 restore 后 predictor/mining=%p/%+v，想要 %p/%+v",
			app.predictor, app.miningOverlay, originalPredictor, originalMining)
	}
}
