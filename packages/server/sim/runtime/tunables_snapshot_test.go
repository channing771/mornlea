package runtime

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
	"github.com/channing771/mornlea/packages/shared/tuning"
)

// TestEngineRefreshesSnapshotAtTickStart 证明快照在 tick 入口刷新，
// 且同一 tick 内不再变化。
func TestEngineRefreshesSnapshotAtTickStart(t *testing.T) {
	t.Cleanup(func() { tuning.SetTunables(tuning.DefaultTunables()) })

	engine := NewEngine(0, 0, 0)

	changed := tuning.DefaultTunables()
	changed.InteractionReach = 3
	tuning.SetTunables(changed)

	engine.Step()
	if engine.tunables.InteractionReach != 3 {
		t.Fatalf("tick 后引擎快照 InteractionReach = %v，want 3",
			engine.tunables.InteractionReach)
	}

	// tick 之间修改，在下一次 Step 之前引擎快照不应改变。
	again := tuning.DefaultTunables()
	again.InteractionReach = 5
	tuning.SetTunables(again)
	if engine.tunables.InteractionReach != 3 {
		t.Fatal("引擎快照必须只在 tick 入口刷新")
	}
	engine.Step()
	if engine.tunables.InteractionReach != 5 {
		t.Fatalf("下一次 tick 后应刷新为 5，实际 %v", engine.tunables.InteractionReach)
	}
}

func TestTickKeepsCapturedGroundAccelerationAfterGlobalUpdate(t *testing.T) {
	t.Cleanup(func() { physics.SetTunables(physics.DefaultTunables()) })

	captured := physics.DefaultTunables()
	captured.GroundAcceleration = 2
	updated := captured
	updated.GroundAcceleration = 40
	physics.SetTunables(captured)

	control, controlSession := readyMovementPlayer(t)
	subject, subjectSession := readyMovementPlayer(t)
	control.Enqueue(Command{
		Session: controlSession, Sequence: 1, Kind: CommandPlayerInput, MoveX: 1,
	})
	subject.Enqueue(Command{
		Session: subjectSession, Sequence: 1, Kind: CommandPlayerInput, MoveX: 1,
	})
	want := onlyOrchestratedMovementPlayer(t, control.Step())
	got := onlyOrchestratedMovementPlayer(t, stepWithPhysicsUpdateAtPhase(
		t, subject, phasePhysicsAdvance, updated,
	))
	if physics.ActiveTunables() != updated {
		t.Fatal("阶段屏障未发布新的 physics tunables")
	}
	if got.State != want.State {
		t.Fatalf("tick 中途更新 GroundAcceleration 改变了当前 tick：got=%+v want=%+v",
			got.State, want.State)
	}
}

func TestTickKeepsCapturedEyeHeightForSubmersionAfterGlobalUpdate(t *testing.T) {
	t.Cleanup(func() { physics.SetTunables(physics.DefaultTunables()) })

	captured := physics.DefaultTunables()
	captured.EyeHeight = 1.62
	updated := captured
	updated.EyeHeight = 0.2
	physics.SetTunables(captured)

	control, _ := readyMovementPlayer(t)
	subject, _ := readyMovementPlayer(t)
	placeWaterAtCapturedEye(t, control, captured.EyeHeight)
	placeWaterAtCapturedEye(t, subject, captured.EyeHeight)
	want := onlyOrchestratedMovementPlayer(t, control.Step())
	got := onlyOrchestratedMovementPlayer(t, stepWithPhysicsUpdateAtPhase(
		t, subject, phasePhysicsAdvance, updated,
	))
	if physics.ActiveTunables() != updated {
		t.Fatal("阶段屏障未发布新的 physics tunables")
	}
	if want.Oxygen != core.MaxOxygenTicks-1 {
		t.Fatalf("对照夹具未让旧眼高浸没：oxygen=%d", want.Oxygen)
	}
	if got.Oxygen != want.Oxygen {
		t.Fatalf("tick 中途更新 EyeHeight 改变了当前 tick 氧气：got=%d want=%d",
			got.Oxygen, want.Oxygen)
	}
}

func stepWithPhysicsUpdateAtPhase(
	t *testing.T,
	engine *Engine,
	barrier stepPhase,
	updated physics.Tunables,
) TickResult {
	t.Helper()
	entered := make(chan struct{})
	release := make(chan struct{})
	engine.stepPhaseObserver = func(phase stepPhase) {
		if phase != barrier {
			return
		}
		close(entered)
		<-release
	}
	done := make(chan TickResult, 1)
	go func() { done <- engine.Step() }()
	<-entered
	physics.SetTunables(updated)
	close(release)
	result := <-done
	engine.stepPhaseObserver = nil
	return result
}

func placeWaterAtCapturedEye(t *testing.T, engine *Engine, eyeHeight float32) {
	t.Helper()
	player, ok := engine.Player(1)
	if !ok || !player.Ready {
		t.Fatalf("水下夹具玩家未激活：%+v", player)
	}
	position := player.State.Position
	chunk, ok := engine.dimension(core.Overworld).ReadyChunk(positionToChunk(position))
	if !ok {
		t.Fatal("水下夹具区块未 ready")
	}
	x := int32(math.Floor(float64(position.X())))
	z := int32(math.Floor(float64(position.Z())))
	chunk.SetBlock(
		int(x&core.SectionMask),
		int32(math.Floor(float64(position.Y()+eyeHeight))),
		int(z&core.SectionMask),
		core.WaterSourceID,
	)
}

func positionToChunk(position [3]float32) core.ChunkPos {
	return core.BlockPos{
		X: int32(math.Floor(float64(position[0]))),
		Z: int32(math.Floor(float64(position[2]))),
	}.Chunk()
}
