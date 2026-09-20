package runtime

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// `SemanticInput` is the platform-neutral control intent accepted from a presentation host.
// It deliberately contains no device state, protocol sequence, or transport policy.
type SemanticInput struct {
	MoveX     int8
	MoveZ     int8
	Jump      bool
	Yaw       float32
	Pitch     float32
	Mining    bool
	Eating    bool
	Sprinting bool
	Sneaking  bool
}

// `ReconcileResult` reports the only host-visible view reset produced by authoritative correction.
type ReconcileResult struct {
	ResetView bool
	Yaw       float32
	Pitch     float32
}

// `PredictionSnapshot` is a detached view of the predictor's current physical and confirmed state.
// Every field is copied, so hosts may retain a snapshot across later input, correction, or reset.
type PredictionSnapshot struct {
	State                physics.State
	Ready                bool
	PresentationPosition mgl32.Vec3
	PresentationReady    bool
	HistoryLength        int
	Suspended            bool
	Health               uint8
	Oxygen               uint16
	Hunger               uint8
	SaturationZero       bool
	Weather              core.WeatherKind
	Season               core.Season
	SeasonProgress       uint8
	Temperature          int8
	Armor                uint8
	EyeInFluid           bool
}

// `SubmitInput` validates and copies the host's latest intent without allocating a protocol
// sequence, advancing physics, or performing network I/O. A later explicit advance consumes it.
func (runtime *Runtime) SubmitInput(input SemanticInput) error {
	if runtime == nil {
		return errors.New("runtime: nil runtime")
	}
	if err := validateSemanticInput(input); err != nil {
		return err
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return errors.New("runtime: client session is closed")
	}
	runtime.semanticInput = input
	// The host submits an explicit semantic pose. Device delta accumulation stays outside runtime,
	// while the copied look state remains the sole source for prediction and presentation rays.
	runtime.cameraYaw = input.Yaw
	runtime.cameraPitch = input.Pitch
	return nil
}

// `AdvancePrediction` advances the existing fixed-step predictor with the latest submitted intent.
// The caller owns the send deadline through `ctx`; this interim synchronous seam is not suitable
// for the later non-blocking frame `Step`, which will provide bounded outbound queue ownership.
func (runtime *Runtime) AdvancePrediction(ctx context.Context, elapsed time.Duration) (PredictionSnapshot, error) {
	if runtime == nil {
		return PredictionSnapshot{}, errors.New("runtime: nil runtime")
	}
	if ctx == nil {
		return PredictionSnapshot{}, errors.New("runtime: nil prediction context")
	}
	if elapsed < 0 {
		return PredictionSnapshot{}, errors.New("runtime: negative prediction elapsed time")
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return runtime.predictionSnapshotLocked(0), errors.New("runtime: client session is closed")
	}
	if runtime.predictor == nil {
		runtime.predictor = client.NewPredictor()
	}
	control := runtime.semanticInput.control()
	source := client.MirrorCollisionSource{Mirror: runtime.mirrors.world, Dimension: core.Overworld}
	err := runtime.predictor.Advance(
		elapsed,
		control,
		source,
		runtime.nextSequenceLocked,
		func(message network.PlayerInput) error {
			if runtime.sender == nil {
				return errors.New("runtime: client input sender is unavailable")
			}
			return runtime.sender.Send(ctx, message)
		},
	)
	return runtime.predictionSnapshotLocked(elapsed), err
}

// `Prediction` returns a detached view without advancing fixed steps or correction smoothing.
func (runtime *Runtime) Prediction() PredictionSnapshot {
	if runtime == nil {
		return PredictionSnapshot{}
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	return runtime.predictionSnapshotLocked(0)
}

func (runtime *Runtime) applyPlayerStateLocked(
	outcome MessageOutcome,
	message network.PlayerState,
) (MessageOutcome, error) {
	if message.ServerTick <= runtime.playerTick {
		return outcome, nil
	}
	if runtime.predictor == nil {
		runtime.predictor = client.NewPredictor()
	}
	source := client.MirrorCollisionSource{Mirror: runtime.mirrors.world, Dimension: core.Overworld}
	result, err := runtime.predictor.ApplyPlayerState(message, source)
	if err != nil {
		return outcome, err
	}
	runtime.playerTick = message.ServerTick
	runtime.phaseMu.Lock()
	if message.Ready {
		runtime.phase = ConnectionPhasePlay
	} else {
		runtime.phase = ConnectionPhaseLoading
	}
	runtime.phaseMu.Unlock()
	outcome.PredictionChanged = true
	outcome.Reconcile = ReconcileResult{
		ResetView: result.ResetView,
		Yaw:       result.Yaw,
		Pitch:     result.Pitch,
	}
	if result.ResetView {
		// Authoritative resets own the view orientation as well as the predicted player position.
		// Camera mode is deliberately not reset: it is a local preference, not session authority.
		runtime.cameraYaw = result.Yaw
		runtime.cameraPitch = result.Pitch
	}
	if message.Reset {
		runtime.cameraTargetReset = true
	}
	outcome.Prediction = runtime.predictionSnapshotLocked(0)
	return outcome, nil
}

func (runtime *Runtime) predictionSnapshotLocked(presentationElapsed time.Duration) PredictionSnapshot {
	if runtime.predictor == nil {
		return PredictionSnapshot{}
	}
	predictor := runtime.predictor
	state, ready := predictor.State()
	presentationPosition, presentationReady := predictor.PresentationPosition(presentationElapsed)
	health, _ := predictor.Health()
	oxygen, _ := predictor.Oxygen()
	hunger, _ := predictor.Hunger()
	saturationZero, _ := predictor.SaturationZero()
	weather, _ := predictor.Weather()
	season, _ := predictor.Season()
	seasonProgress, _ := predictor.SeasonProgress()
	temperature, _ := predictor.Temperature()
	armor, _ := predictor.Armor()
	return PredictionSnapshot{
		State:                state,
		Ready:                ready,
		PresentationPosition: presentationPosition,
		PresentationReady:    presentationReady,
		HistoryLength:        predictor.HistoryLen(),
		Suspended:            predictor.Suspended(),
		Health:               health,
		Oxygen:               oxygen,
		Hunger:               hunger,
		SaturationZero:       saturationZero,
		Weather:              weather,
		Season:               season,
		SeasonProgress:       seasonProgress,
		Temperature:          temperature,
		Armor:                armor,
		EyeInFluid:           predictor.EyeInFluid(),
	}
}

func (runtime *Runtime) nextSequenceLocked() uint64 {
	runtime.sequence++
	return runtime.sequence
}

func (input SemanticInput) control() client.Control {
	return client.Control{
		MoveX:     input.MoveX,
		MoveZ:     input.MoveZ,
		Jump:      input.Jump,
		Yaw:       input.Yaw,
		Pitch:     input.Pitch,
		Mining:    input.Mining,
		Eating:    input.Eating,
		Sprinting: input.Sprinting,
		Sneaking:  input.Sneaking,
	}
}

func validateSemanticInput(input SemanticInput) error {
	if input.MoveX < -1 || input.MoveX > 1 || input.MoveZ < -1 || input.MoveZ > 1 {
		return errors.New("runtime: invalid semantic movement input")
	}
	const maxPitch = float32(math.Pi/2 - 0.01)
	if math.IsNaN(float64(input.Yaw)) || math.IsInf(float64(input.Yaw), 0) ||
		math.IsNaN(float64(input.Pitch)) || math.IsInf(float64(input.Pitch), 0) ||
		input.Pitch < -maxPitch || input.Pitch > maxPitch {
		return errors.New("runtime: invalid semantic look input")
	}
	return nil
}
