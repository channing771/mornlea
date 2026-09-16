package presentation

import (
	"math"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func validTestFrame() FrameSnapshot {
	return FrameSnapshot{
		Version:  FrameSnapshotVersion,
		Revision: 1,
		Epoch:    1,
		Phase:    SessionPhasePlay,
		Camera: CameraSnapshot{
			Ready: true, Position: [3]float32{0.5, 34.6, 2.5},
			Yaw: 0.25, Pitch: -0.1,
			FOVY: 1.2217305, Aspect: 16.0 / 9.0, Near: 0.1, Far: 2000,
		},
		HUD:         HUDSnapshot{Ready: true, Health: 15, Hunger: 12, Oxygen: 240},
		Environment: EnvironmentSnapshot{Ready: true, ServerTick: 9, WorldTimeTicks: 1000},
		Entities: EntityBatch{
			ServerTick: 9,
			records: []EntityRecord{{
				Kind:      EntityKindRemotePlayer,
				PlayerID:  core.PlayerID{0: 7, 6: 0x40, 8: 0x80},
				Dimension: core.Overworld,
				Position:  [3]float32{1, 2, 3},
			}},
		},
		Target: TargetSnapshot{Visible: true, Position: core.BlockPos{X: 1, Y: 64, Z: -2}, Name: "Brick"},
	}
}

func TestFrameSnapshotAcceptsCompleteAggregateAndRejectsIdentityDrift(t *testing.T) {
	frame := validTestFrame()
	if err := frame.Validate(); err != nil {
		t.Fatalf("valid frame: %v", err)
	}

	var zero FrameSnapshot
	if err := zero.Validate(); err == nil {
		t.Fatal("zero frame validated without identity")
	}

	tests := []struct {
		name   string
		mutate func(*FrameSnapshot)
	}{
		{"unknown version", func(frame *FrameSnapshot) { frame.Version = FrameSnapshotVersion + 1 }},
		{"zero revision", func(frame *FrameSnapshot) { frame.Revision = 0 }},
		{"zero epoch", func(frame *FrameSnapshot) { frame.Epoch = 0 }},
		{"unknown phase", func(frame *FrameSnapshot) { frame.Phase = SessionPhase(200) }},
		{"camera payload drift", func(frame *FrameSnapshot) { frame.Camera.Position[0] = float32(math.NaN()) }},
		{"not-ready HUD payload", func(frame *FrameSnapshot) { frame.HUD.Ready = false }},
		{"environment payload drift", func(frame *FrameSnapshot) {
			frame.Environment.Weather = core.WeatherThunder + 1
		}},
		{"entity record drift", func(frame *FrameSnapshot) {
			frame.Entities.records[0].Position[1] = float32(math.Inf(1))
		}},
		{"target payload drift", func(frame *FrameSnapshot) { frame.Target.Position.Y = core.MinY - 1 }},
		{"error without disconnect", func(frame *FrameSnapshot) { frame.Error = ErrorState{Code: ErrorConnection} }},
		{"disconnect without error", func(frame *FrameSnapshot) {
			frame.Phase = SessionPhaseDisconnected
			frame.Camera = CameraSnapshot{}
			frame.HUD = HUDSnapshot{}
			frame.Environment = EnvironmentSnapshot{}
			frame.Entities = EntityBatch{}
			frame.Target = TargetSnapshot{}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			drifted := validTestFrame()
			test.mutate(&drifted)
			if err := drifted.Validate(); err == nil {
				t.Fatalf("FrameSnapshot.Validate() accepted %s", test.name)
			}
		})
	}
}

func TestFrameSnapshotAcceptsTerminalFrameAndOwnsEntityRecords(t *testing.T) {
	terminal := FrameSnapshot{
		Version: FrameSnapshotVersion, Revision: 4, Epoch: 2,
		Phase: SessionPhaseDisconnected,
		Error: ErrorState{Code: ErrorConnection},
	}
	if err := terminal.Validate(); err != nil {
		t.Fatalf("terminal frame: %v", err)
	}

	frame := validTestFrame()
	records := frame.Entities.Records()
	records[0].Position[0] = 99
	if retained := frame.Entities.Records(); retained[0].Position[0] == 99 {
		t.Fatal("FrameSnapshot entity records alias caller storage")
	}
}
