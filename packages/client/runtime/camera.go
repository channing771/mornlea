package runtime

import (
	"errors"
	"math"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

const (
	defaultCameraFOVY   = float32(1.2217305) // 70 degrees, matching the legacy application default.
	defaultCameraAspect = float32(1)
	defaultCameraNear   = float32(0.1)
	defaultCameraFar    = float32(2000)
	targetRayDistance   = float32(6)
)

var errTargetPathUnknown = errors.New("runtime: target path is unknown")

// `CameraProjection` is the scalar camera configuration a presentation host may supply without
// transferring viewport matrices, render handles, or any device state into `runtime`.
type CameraProjection struct {
	FOVY   float32
	Aspect float32
	Near   float32
	Far    float32
}

// `DefaultCameraProjection` returns the legacy application's safe initial projection values.
func DefaultCameraProjection() CameraProjection {
	return CameraProjection{
		FOVY: defaultCameraFOVY, Aspect: defaultCameraAspect,
		Near: defaultCameraNear, Far: defaultCameraFar,
	}
}

// `Validate` applies the presentation camera contract before local projection configuration is
// published. A scalar configuration is checked through the same domain as frame-facing snapshots.
func (projection CameraProjection) Validate() error {
	return presentation.CameraSnapshot{
		Ready: true, FOVY: projection.FOVY, Aspect: projection.Aspect,
		Near: projection.Near, Far: projection.Far,
	}.Validate()
}

// `CameraPresentation` is an immutable render-facing result derived wholly from runtime-owned
// mirror, prediction, and local camera preference state. Hosts map devices to semantic input and
// write their own viewport matrices, but never supply a target ray or a render-camera position.
type CameraPresentation struct {
	Camera presentation.CameraSnapshot
	Mode   client.CameraMode
	Target presentation.TargetSnapshot
}

// `SetCameraMode` applies a validated local preference without synthesizing a device-key edge.
// Rejected values leave the current mode unchanged, and session reset preserves the accepted mode.
func (runtime *Runtime) SetCameraMode(mode client.CameraMode) error {
	if runtime == nil {
		return errors.New("runtime: nil runtime")
	}
	if !mode.Valid() {
		return errors.New("runtime: invalid camera mode")
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return errors.New("runtime: client session is closed")
	}
	runtime.cameraMode = mode
	return nil
}

// `UpdateCameraMode` applies the host-mapped F5 level using the legacy rising-edge behavior.
// The latch updates during blocked frames so a key held while a host UI blocks input cannot cause
// a delayed mode change when the host becomes unblocked.
func (runtime *Runtime) UpdateCameraMode(f5Down, blocked bool) {
	if runtime == nil {
		return
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return
	}
	if f5Down && !runtime.cameraF5WasDown && !blocked {
		runtime.cameraMode = runtime.cameraMode.Next()
	}
	runtime.cameraF5WasDown = f5Down
}

// `SetCameraProjection` atomically replaces the local presentation projection after validation.
// It intentionally persists across mirror resets because FOV and resize state are host-local,
// while no projection matrix is constructed or stored by the runtime.
func (runtime *Runtime) SetCameraProjection(projection CameraProjection) error {
	if runtime == nil {
		return errors.New("runtime: nil runtime")
	}
	if err := projection.Validate(); err != nil {
		return err
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return errors.New("runtime: client session is closed")
	}
	runtime.cameraProjection = projection
	return nil
}

// `ApplyLookDelta` accepts a host-normalized semantic look delta and clamps pitch with the same
// camera rule as the legacy client. It updates the explicit pose that later prediction sends,
// without reading a mouse, cursor, viewport, or wall clock.
func (runtime *Runtime) ApplyLookDelta(yaw, pitch float32) error {
	if runtime == nil {
		return errors.New("runtime: nil runtime")
	}
	if !finiteCameraFloat(yaw) || !finiteCameraFloat(pitch) {
		return errors.New("runtime: non-finite semantic look delta")
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return errors.New("runtime: client session is closed")
	}
	pose := client.Camera{Yaw: runtime.cameraYaw, Pitch: runtime.cameraPitch}
	pose.Rotate(yaw, pitch)
	if !finiteCameraFloat(pose.Yaw) || !finiteCameraFloat(pose.Pitch) {
		return errors.New("runtime: non-finite semantic look pose")
	}
	runtime.cameraYaw = pose.Yaw
	runtime.cameraPitch = pose.Pitch
	runtime.semanticInput.Yaw = pose.Yaw
	runtime.semanticInput.Pitch = pose.Pitch
	return nil
}

// `CameraPresentation` returns copied camera and target values suitable for later frame snapshots.
// A not-ready player publishes no pose or target; the local view preference remains observable so
// a host can retain it across a fresh authoritative session.
func (runtime *Runtime) CameraPresentation() CameraPresentation {
	if runtime == nil {
		return CameraPresentation{}
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if !runtime.cameraMode.Valid() {
		runtime.cameraMode = client.CameraFirstPerson
	}
	output := CameraPresentation{Mode: runtime.cameraMode}
	eye, ready := runtime.eyeCameraLocked()
	if !ready {
		return output
	}
	renderCamera := client.ResolveThirdPersonCamera(eye, runtime.cameraMode, runtime.thirdPersonSolidLocked)
	output.Camera = cameraSnapshot(renderCamera)
	if runtime.cameraTargetReset {
		// Consume the reset only when a ready presentation is published. Calls while loading do not
		// let a later first playable frame leak a target from the reset boundary.
		runtime.cameraTargetReset = false
		return output
	}
	output.Target = runtime.targetSnapshotLocked(eye)
	return output
}

func (runtime *Runtime) eyeCameraLocked() (client.Camera, bool) {
	if runtime.predictor == nil {
		return client.Camera{}, false
	}
	feet, ready := runtime.predictor.PresentationPosition(0)
	if !ready {
		return client.Camera{}, false
	}
	projection := runtime.cameraProjection
	if projection == (CameraProjection{}) {
		projection = DefaultCameraProjection()
	}
	return client.Camera{
		Pos:    feet.Add(mgl32.Vec3{0, physics.ActiveTunables().EyeHeight, 0}),
		Yaw:    runtime.cameraYaw,
		Pitch:  runtime.cameraPitch,
		FovY:   projection.FOVY,
		Aspect: projection.Aspect,
		Near:   projection.Near,
		Far:    projection.Far,
	}, true
}

func cameraSnapshot(camera client.Camera) presentation.CameraSnapshot {
	return presentation.CameraSnapshot{
		Ready:    true,
		Position: [3]float32(camera.Pos),
		Yaw:      camera.Yaw,
		Pitch:    camera.Pitch,
		FOVY:     camera.FovY,
		Aspect:   camera.Aspect,
		Near:     camera.Near,
		Far:      camera.Far,
	}
}

func (runtime *Runtime) thirdPersonSolidLocked(position core.BlockPos) (bool, error) {
	if runtime.mirrors == nil || runtime.mirrors.world == nil {
		return false, nil
	}
	id, loaded := runtime.mirrors.world.BlockAt(core.Overworld, position)
	if !loaded {
		return false, nil
	}
	chunk, loaded := runtime.mirrors.world.Chunk(core.Overworld, position.Chunk())
	if loaded && chunk != nil && chunk.Desynced {
		return false, nil
	}
	return core.BlockOpaque(id), nil
}

func (runtime *Runtime) targetSnapshotLocked(eye client.Camera) presentation.TargetSnapshot {
	if runtime.mirrors == nil || runtime.mirrors.world == nil {
		return presentation.TargetSnapshot{}
	}
	var targetID core.BlockID
	hit, found, err := core.RaycastBlocks(
		eye.Pos,
		eye.Forward(),
		targetRayDistance,
		func(position core.BlockPos) (bool, error) {
			id, loaded := runtime.mirrors.world.BlockAt(core.Overworld, position)
			chunk, _ := runtime.mirrors.world.Chunk(core.Overworld, position.Chunk())
			if !loaded || (chunk != nil && chunk.Desynced) || !core.RegisteredBlock(id) {
				return false, errTargetPathUnknown
			}
			targetID = id
			return core.InteractionTarget(id), nil
		},
	)
	if err != nil || !found {
		return presentation.TargetSnapshot{}
	}
	name, ok := core.BlockDisplayName(targetID)
	if !ok || name == "" {
		return presentation.TargetSnapshot{}
	}
	return presentation.TargetSnapshot{Visible: true, Position: hit.Block, Name: name}
}

func finiteCameraFloat(value float32) bool {
	return !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
}
