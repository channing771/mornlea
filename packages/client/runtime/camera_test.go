package runtime

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

func TestCameraModeCyclesOnUnblockedF5EdgesAndLatchesBlockedPress(t *testing.T) {
	runtime := newCameraRuntime(t)

	if got := runtime.CameraPresentation().Mode; got != client.CameraFirstPerson {
		t.Fatalf("initial camera mode = %v, want first person", got)
	}
	runtime.UpdateCameraMode(true, true)
	if got := runtime.CameraPresentation().Mode; got != client.CameraFirstPerson {
		t.Fatalf("blocked F5 changed camera mode to %v", got)
	}
	// The blocked press still updates the edge latch, so holding F5 while unblocked does not cycle.
	runtime.UpdateCameraMode(true, false)
	if got := runtime.CameraPresentation().Mode; got != client.CameraFirstPerson {
		t.Fatalf("held blocked F5 changed camera mode to %v", got)
	}
	runtime.UpdateCameraMode(false, false)
	runtime.UpdateCameraMode(true, false)
	if got := runtime.CameraPresentation().Mode; got != client.CameraThirdPersonBack {
		t.Fatalf("first unblocked F5 edge camera mode = %v, want back", got)
	}
	runtime.UpdateCameraMode(false, false)
	runtime.UpdateCameraMode(true, false)
	if got := runtime.CameraPresentation().Mode; got != client.CameraThirdPersonFront {
		t.Fatalf("second unblocked F5 edge camera mode = %v, want front", got)
	}
}

func TestCameraModeAcceptsPersistedPreferenceAndRejectsInvalidValueAtomically(t *testing.T) {
	runtime := newCameraRuntime(t)
	if err := runtime.SetCameraMode(client.CameraThirdPersonFront); err != nil {
		t.Fatal(err)
	}
	if got := runtime.CameraPresentation().Mode; got != client.CameraThirdPersonFront {
		t.Fatalf("explicit camera mode = %v, want front", got)
	}
	if err := runtime.SetCameraMode(client.CameraMode(99)); err == nil {
		t.Fatal("SetCameraMode accepted an invalid mode")
	}
	if got := runtime.CameraPresentation().Mode; got != client.CameraThirdPersonFront {
		t.Fatalf("rejected camera mode changed state to %v", got)
	}
	runtime.ResetMirrors()
	if got := runtime.CameraPresentation().Mode; got != client.CameraThirdPersonFront {
		t.Fatalf("session reset lost explicit camera preference: %v", got)
	}
}

func TestCameraRejectsNonFiniteAndOutOfRangeSemanticPose(t *testing.T) {
	runtime := newCameraRuntime(t)
	for _, input := range []SemanticInput{
		{Yaw: float32(math.Inf(1))},
		{Pitch: float32(math.NaN())},
		{Pitch: float32(math.Pi / 2)},
	} {
		if err := runtime.SubmitInput(input); err == nil {
			t.Fatalf("SubmitInput(%+v) accepted invalid semantic look", input)
		}
	}
}

func TestCameraLookDeltaClampsPitchAndRejectsNonFiniteValues(t *testing.T) {
	runtime := newCameraRuntime(t)
	readyCameraRuntime(t, runtime, mgl32.Vec3{0.5, 10, 0.5}, 0, 0)
	if err := runtime.ApplyLookDelta(0.25, 10); err != nil {
		t.Fatal(err)
	}
	got := runtime.CameraPresentation().Camera
	wantPitch := float32(math.Pi/2 - 0.01)
	if got.Yaw != 0.25 || got.Pitch != wantPitch {
		t.Fatalf("clamped camera orientation = %v/%v, want 0.25/%v", got.Yaw, got.Pitch, wantPitch)
	}
	if err := runtime.ApplyLookDelta(float32(math.NaN()), 0); err == nil {
		t.Fatal("ApplyLookDelta accepted non-finite yaw")
	}
	if after := runtime.CameraPresentation().Camera; after != got {
		t.Fatalf("invalid look delta changed camera: before=%+v after=%+v", got, after)
	}
}

func TestCameraProjectionUsesValidatedLocalConfigurationAcrossMirrorReset(t *testing.T) {
	runtime := newCameraRuntime(t)
	projection := CameraProjection{FOVY: 1.05, Aspect: 16.0 / 9.0, Near: 0.2, Far: 1536}
	if err := runtime.SetCameraProjection(projection); err != nil {
		t.Fatal(err)
	}
	readyCameraRuntime(t, runtime, mgl32.Vec3{0.5, 10, 0.5}, 0, 0)
	if got := runtime.CameraPresentation().Camera; got.FOVY != projection.FOVY || got.Aspect != projection.Aspect ||
		got.Near != projection.Near || got.Far != projection.Far {
		t.Fatalf("camera projection = %+v, want %+v", got, projection)
	}

	before := runtime.CameraPresentation().Camera
	if err := runtime.SetCameraProjection(CameraProjection{FOVY: float32(math.NaN()), Aspect: 2, Near: 0.1, Far: 100}); err == nil {
		t.Fatal("SetCameraProjection accepted non-finite FOV")
	}
	if err := runtime.SetCameraProjection(CameraProjection{FOVY: 1.1, Aspect: 2, Near: 10, Far: 1}); err == nil {
		t.Fatal("SetCameraProjection accepted inverted clipping planes")
	}
	if after := runtime.CameraPresentation().Camera; after != before {
		t.Fatalf("rejected projection changed camera: before=%+v after=%+v", before, after)
	}

	runtime.ResetMirrors()
	readyCameraRuntime(t, runtime, mgl32.Vec3{1.5, 20, -3.5}, 0, 0)
	if got := runtime.CameraPresentation().Camera; got.FOVY != projection.FOVY || got.Aspect != projection.Aspect ||
		got.Near != projection.Near || got.Far != projection.Far {
		t.Fatalf("mirror reset lost local projection: %+v, want %+v", got, projection)
	}
}

func TestCameraFirstPersonUsesPredictedEyePoseAndTargetsMirrorBlock(t *testing.T) {
	runtime := newCameraRuntime(t)
	eye := mgl32.Vec3{0.5, 3.5, 2.5}
	readyCameraRuntime(t, runtime, eye.Sub(mgl32.Vec3{0, physics.ActiveTunables().EyeHeight, 0}), 0, 0)
	loadCameraChunks(t, runtime, core.ChunkPos{}, core.ChunkPos{Z: -1})
	setCameraMirrorBlock(t, runtime, core.BlockPos{X: 0, Y: 3, Z: -3}, core.BrickID)

	got := runtime.CameraPresentation()
	if got.Mode != client.CameraFirstPerson {
		t.Fatalf("camera mode = %v, want first person", got.Mode)
	}
	if got.Camera != (presentation.CameraSnapshot{
		Ready: true, Position: [3]float32(eye), FOVY: defaultCameraFOVY,
		Aspect: defaultCameraAspect, Near: defaultCameraNear, Far: defaultCameraFar,
	}) {
		t.Fatalf("first-person camera = %+v", got.Camera)
	}
	wantTarget := presentation.TargetSnapshot{
		Visible: true, Position: core.BlockPos{X: 0, Y: 3, Z: -3}, Name: "砖块",
	}
	if got.Target != wantTarget {
		t.Fatalf("target = %+v, want %+v", got.Target, wantTarget)
	}
}

func TestTargetNoHitPublishesAHiddenSnapshot(t *testing.T) {
	runtime := newCameraRuntime(t)
	eye := mgl32.Vec3{0.5, 3.5, 2.5}
	readyCameraRuntime(t, runtime, eye.Sub(mgl32.Vec3{0, physics.ActiveTunables().EyeHeight, 0}), 0, 0)
	loadCameraChunks(t, runtime, core.ChunkPos{}, core.ChunkPos{Z: -1})

	if got := runtime.CameraPresentation().Target; got != (presentation.TargetSnapshot{}) {
		t.Fatalf("empty ray target = %+v, want hidden", got)
	}
}

func TestCameraThirdPersonUsesMirrorOcclusionButKeepsEyeTargetRay(t *testing.T) {
	runtime := newCameraRuntime(t)
	eye := mgl32.Vec3{0.5, 10, 0.5}
	readyCameraRuntime(t, runtime, eye.Sub(mgl32.Vec3{0, physics.ActiveTunables().EyeHeight, 0}), 0, 0)
	loadCameraChunks(t, runtime, core.ChunkPos{}, core.ChunkPos{Z: 1}, core.ChunkPos{Z: -1})
	setCameraMirrorBlock(t, runtime, core.BlockPos{X: 0, Y: 10, Z: 1}, core.StoneID)
	setCameraMirrorBlock(t, runtime, core.BlockPos{X: 0, Y: 10, Z: -5}, core.BrickID)

	cycleCameraMode(t, runtime, client.CameraThirdPersonBack)
	back := runtime.CameraPresentation()
	if back.Camera.Position[2] <= eye.Z() || back.Camera.Position[2] >= 1 {
		t.Fatalf("back camera position = %v, want between eye and wall", back.Camera.Position)
	}
	if !back.Target.Visible || back.Target.Position != (core.BlockPos{X: 0, Y: 10, Z: -5}) {
		t.Fatalf("back target = %+v, want eye-ray block", back.Target)
	}

	cycleCameraMode(t, runtime, client.CameraThirdPersonFront)
	front := runtime.CameraPresentation()
	if front.Camera.Position != [3]float32{0.5, 10, -3.5} {
		t.Fatalf("front camera position = %v, want open front offset", front.Camera.Position)
	}
	if front.Camera.Yaw != float32(math.Pi) || front.Camera.Pitch != 0 {
		t.Fatalf("front camera orientation = %v/%v, want pi/0", front.Camera.Yaw, front.Camera.Pitch)
	}
	if !front.Target.Visible || front.Target.Position != (core.BlockPos{X: 0, Y: 10, Z: -5}) {
		t.Fatalf("front target = %+v, want unchanged eye-ray block", front.Target)
	}
}

func TestCameraAuthoritativeResetAndMirrorResetClearPoseAndTargetButKeepMode(t *testing.T) {
	runtime := newCameraRuntime(t)
	eye := mgl32.Vec3{0.5, 3.5, 2.5}
	readyCameraRuntime(t, runtime, eye.Sub(mgl32.Vec3{0, physics.ActiveTunables().EyeHeight, 0}), 0.7, 0.3)
	loadCameraChunks(t, runtime, core.ChunkPos{}, core.ChunkPos{Z: -1})
	setCameraMirrorBlock(t, runtime, core.BlockPos{X: 0, Y: 3, Z: -3}, core.BrickID)
	cycleCameraMode(t, runtime, client.CameraThirdPersonBack)

	state := network.PlayerState{
		ServerTick: 2, Dimension: core.Overworld, Position: mgl32.Vec3{4.5, 20, -2.5},
		Yaw: 0.25, Pitch: -0.2, OnGround: true, Ready: true, Reset: true,
	}
	if _, err := runtime.ApplyMessage(state); err != nil {
		t.Fatal(err)
	}
	afterAuthority := runtime.CameraPresentation()
	if afterAuthority.Camera.Yaw != state.Yaw || afterAuthority.Camera.Pitch != state.Pitch {
		t.Fatalf("authoritative reset camera orientation = %v/%v, want %v/%v",
			afterAuthority.Camera.Yaw, afterAuthority.Camera.Pitch, state.Yaw, state.Pitch)
	}
	if afterAuthority.Target.Visible {
		t.Fatalf("authoritative reset retained target: %+v", afterAuthority.Target)
	}

	runtime.ResetMirrors()
	afterMirrorReset := runtime.CameraPresentation()
	if afterMirrorReset.Camera != (presentation.CameraSnapshot{}) || afterMirrorReset.Target != (presentation.TargetSnapshot{}) {
		t.Fatalf("mirror reset retained pose or target: %+v", afterMirrorReset)
	}
	if afterMirrorReset.Mode != client.CameraThirdPersonBack {
		t.Fatalf("mirror reset camera mode = %v, want local preference preserved", afterMirrorReset.Mode)
	}
}

func TestTargetAuthoritativeResetSuppressesOnePresentationAtTheSamePose(t *testing.T) {
	runtime := newCameraRuntime(t)
	feet := mgl32.Vec3{0.5, 1.88, 2.5}
	readyCameraRuntime(t, runtime, feet, 0, 0)
	loadCameraChunks(t, runtime, core.ChunkPos{}, core.ChunkPos{Z: -1})
	setCameraMirrorBlock(t, runtime, core.BlockPos{X: 0, Y: 3, Z: -3}, core.BrickID)
	if before := runtime.CameraPresentation().Target; !before.Visible {
		t.Fatalf("target before reset = %+v, want visible", before)
	}

	if _, err := runtime.ApplyMessage(network.PlayerState{
		ServerTick: 2, Dimension: core.Overworld, Position: feet,
		OnGround: true, Ready: true, Reset: true,
	}); err != nil {
		t.Fatal(err)
	}
	if resetFrame := runtime.CameraPresentation().Target; resetFrame != (presentation.TargetSnapshot{}) {
		t.Fatalf("target during reset presentation = %+v, want hidden", resetFrame)
	}
	if after := runtime.CameraPresentation().Target; !after.Visible {
		t.Fatalf("target after reset presentation = %+v, want visible", after)
	}
}

func newCameraRuntime(t *testing.T) *Runtime {
	t.Helper()
	runtime := newLoggedInRuntime(&messageTestReceiver{}, nil, 0)
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func readyCameraRuntime(t *testing.T, runtime *Runtime, feet mgl32.Vec3, yaw, pitch float32) {
	t.Helper()
	if err := runtime.SubmitInput(SemanticInput{Yaw: yaw, Pitch: pitch}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ApplyMessage(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld, Position: feet, Yaw: yaw, Pitch: pitch,
		OnGround: true, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func loadCameraChunks(t *testing.T, runtime *Runtime, chunks ...core.ChunkPos) {
	t.Helper()
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for index := range sections {
		sections[index] = network.SectionData{Y: int32(index), Storage: network.SectionSingle, Single: core.AirID}
	}
	for _, chunk := range chunks {
		if _, err := runtime.ApplyMessage(network.ChunkSnapshot{
			Dimension: core.Overworld, Chunk: chunk, Revision: 1, Sections: sections,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func setCameraMirrorBlock(t *testing.T, runtime *Runtime, position core.BlockPos, block core.BlockID) {
	t.Helper()
	if _, err := runtime.ApplyMessage(network.BlockChanges{
		Dimension: core.Overworld, Chunk: position.Chunk(), BaseRevision: 1, NewRevision: 2,
		Changes: []network.BlockChange{{Position: position, Block: block}},
	}); err != nil {
		t.Fatal(err)
	}
}

func cycleCameraMode(t *testing.T, runtime *Runtime, want client.CameraMode) {
	t.Helper()
	for runtime.CameraPresentation().Mode != want {
		runtime.UpdateCameraMode(false, false)
		runtime.UpdateCameraMode(true, false)
	}
}
