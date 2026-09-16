package presentation

import (
	"errors"
	"fmt"
)

// `FrameSnapshotVersion` identifies the current frame aggregate layout. The value
// must change whenever an existing frame field changes meaning; later capability
// families are negotiated separately instead of growing this aggregate.
const FrameSnapshotVersion uint64 = 1

// `FrameSnapshot` is the immutable one-frame aggregate published by a bounded runtime step.
// Every component is a copied value, so a host may retain a published frame across any later
// step, reset, or close without aliasing runtime-owned state. A zero value is never publishable:
// `Validate` requires the current version and a positive revision and epoch.
type FrameSnapshot struct {
	Version     uint64
	Revision    uint64
	Epoch       uint64
	Phase       SessionPhase
	Camera      CameraSnapshot
	HUD         HUDSnapshot
	Environment EnvironmentSnapshot
	Entities    EntityBatch
	Target      TargetSnapshot
	Error       ErrorState
}

// `Validate` applies every component domain plus the frame identity and terminal-state
// coupling before a producer publishes the aggregate to a presentation host.
func (frame FrameSnapshot) Validate() error {
	if frame.Version != FrameSnapshotVersion {
		return fmt.Errorf("presentation: frame version %d is not %d", frame.Version, FrameSnapshotVersion)
	}
	if frame.Revision == 0 {
		return errors.New("presentation: frame revision must be positive")
	}
	if frame.Epoch == 0 {
		return errors.New("presentation: frame epoch must be positive")
	}
	if !frame.Phase.Valid() {
		return fmt.Errorf("presentation: frame phase %d is outside the supported domain", frame.Phase)
	}
	if err := frame.Camera.Validate(); err != nil {
		return fmt.Errorf("presentation: frame camera: %w", err)
	}
	if err := frame.HUD.Validate(); err != nil {
		return fmt.Errorf("presentation: frame HUD: %w", err)
	}
	if err := frame.Environment.Validate(); err != nil {
		return fmt.Errorf("presentation: frame environment: %w", err)
	}
	if err := frame.Entities.Validate(); err != nil {
		return fmt.Errorf("presentation: frame entities: %w", err)
	}
	if err := frame.Target.Validate(); err != nil {
		return fmt.Errorf("presentation: frame target: %w", err)
	}
	if err := frame.Error.Validate(); err != nil {
		return fmt.Errorf("presentation: frame error: %w", err)
	}
	if frame.Error.Present() != (frame.Phase == SessionPhaseDisconnected) {
		return errors.New("presentation: terminal error state must accompany the disconnected phase")
	}
	return nil
}
