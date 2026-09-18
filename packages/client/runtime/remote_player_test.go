package runtime

import (
	"fmt"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestRemotePlayerFrameCoversSpawnStateInterpolationAndDespawn(t *testing.T) {
	id := remotePlayerTestID(1)
	receiver := &stepTestReceiver{messages: []network.ServerMessage{
		stepReadyState(7),
		network.RemotePlayerSpawn{
			PlayerID: id, DisplayName: "Pilot", ServerTick: 5,
			Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3},
		},
		network.RemotePlayerStates{
			ServerTick: 6,
			Players: []network.RemotePlayerState{{
				PlayerID: id, Dimension: core.Overworld, Position: mgl32.Vec3{5, 2, 3},
			}},
		},
	}}
	runtime := newLoggedInRuntime(receiver, nil, 0)
	t.Cleanup(func() { _ = runtime.Close() })

	result, err := runtime.Step(physicsFixedDeltaForRemotePlayerTest(), 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	records := result.Frame.Entities.Records()
	if len(records) != 1 || records[0].PlayerID != id || records[0].Position[0] <= 1 || records[0].Position[0] >= 5 {
		t.Fatalf("interpolated remote-player frame = %+v", records)
	}

	receiver.messages = []network.ServerMessage{network.RemotePlayerDespawn{PlayerID: id}}
	result, err = runtime.Step(0, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if records := result.Frame.Entities.Records(); len(records) != 0 {
		t.Fatalf("despawned remote-player frame = %+v, want empty", records)
	}
	if result.Frame.Entities.ServerTick != 7 {
		t.Fatalf("despawned remote-player tick = %d, want 7", result.Frame.Entities.ServerTick)
	}
}

func TestRemotePlayerDuplicateAndOutOfOrderMessagesPreservePublishedState(t *testing.T) {
	id := remotePlayerTestID(2)
	spawn := network.RemotePlayerSpawn{
		PlayerID: id, DisplayName: "Stable", ServerTick: 5,
		Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3},
	}
	duplicate := spawn
	duplicate.ServerTick = 6
	duplicate.Position[0] = 99
	receiver := &stepTestReceiver{messages: []network.ServerMessage{spawn, duplicate}}
	runtime := newLoggedInRuntime(receiver, nil, 0)
	t.Cleanup(func() { _ = runtime.Close() })
	if _, err := runtime.Step(0, 2, 0); err == nil {
		t.Fatal("duplicate remote-player spawn was accepted")
	}
	receiver.messages = []network.ServerMessage{network.RemotePlayerStates{
		ServerTick: 6,
		Players: []network.RemotePlayerState{{
			PlayerID: id, Dimension: core.Overworld, Position: mgl32.Vec3{5, 2, 3},
		}},
	}, network.RemotePlayerStates{
		ServerTick: 6,
		Players: []network.RemotePlayerState{{
			PlayerID: id, Dimension: core.Overworld, Position: mgl32.Vec3{99, 2, 3},
		}},
	}}
	if _, err := runtime.Step(0, 2, 0); err == nil {
		t.Fatal("out-of-order remote-player state was accepted")
	}

	result, err := runtime.Step(physicsFixedDeltaForRemotePlayerTest(), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	records := result.Frame.Entities.Records()
	if len(records) != 1 || records[0].Position[0] <= 1 || records[0].Position[0] >= 5 {
		t.Fatalf("failed remote-player messages changed the frame: %+v", records)
	}
}

func TestRemotePlayerFrameRejectsCapacityOverflow(t *testing.T) {
	receiver := &stepTestReceiver{}
	for index := 0; index < presentation.MaxEntityBatchRecords+1; index++ {
		receiver.messages = append(receiver.messages, network.RemotePlayerSpawn{
			PlayerID: remotePlayerTestID(byte(index + 1)), DisplayName: fmt.Sprintf("Pilot%d", index+1),
			ServerTick: uint64(index + 1), Dimension: core.Overworld,
		})
	}
	runtime := newLoggedInRuntime(receiver, nil, 0)
	t.Cleanup(func() { _ = runtime.Close() })
	if _, err := runtime.Step(0, presentation.MaxEntityBatchRecords+1, 0); err == nil {
		t.Fatal("over-capacity remote-player frame was truncated instead of rejected")
	}
}

func TestRemotePlayerSessionResetClearsPresentationState(t *testing.T) {
	receiver := &stepTestReceiver{messages: []network.ServerMessage{network.RemotePlayerSpawn{
		PlayerID: remotePlayerTestID(3), DisplayName: "Reset", ServerTick: 1,
		Dimension: core.Overworld,
	}}}
	runtime := newLoggedInRuntime(receiver, nil, 0)
	t.Cleanup(func() { _ = runtime.Close() })
	before, err := runtime.Step(0, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Frame.Entities.Records()) != 1 {
		t.Fatalf("pre-reset remote-player frame = %+v", before.Frame.Entities.Records())
	}

	runtime.ResetMirrors()
	after, err := runtime.Step(0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if records := after.Frame.Entities.Records(); len(records) != 0 {
		t.Fatalf("post-reset remote-player frame = %+v, want empty", records)
	}
}

func remotePlayerTestID(value byte) core.PlayerID {
	return core.PlayerID{0: value, 6: 0x40, 8: 0x80}
}

func physicsFixedDeltaForRemotePlayerTest() time.Duration {
	return 50 * time.Millisecond
}
