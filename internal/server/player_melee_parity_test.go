package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/storage"
)

type meleeWireTranscript struct {
	Healths       []uint8
	DeathReset    bool
	RemoteSamples [][3]uint32
}

func TestPlayerMeleeMemoryTCPWireParity(t *testing.T) {
	memory := runPlayerMeleeWireScript(t, "memory")
	tcp := runPlayerMeleeWireScript(t, "tcp")
	if !reflect.DeepEqual(memory, tcp) {
		t.Fatalf("近战 Memory/TCP wire transcript 不一致\nmemory=%+v\ntcp=%+v", memory, tcp)
	}
	if !memory.DeathReset || len(memory.Healths) < 10 {
		t.Fatalf("近战 transcript 未观察到十次伤害和死亡 reset: %+v", memory)
	}
}

func runPlayerMeleeWireScript(t *testing.T, transport string) meleeWireTranscript {
	t.Helper()
	attacker := integrationIdentity(0x91, "MeleeAttacker")
	target := integrationIdentity(0x92, "MeleeTarget")
	store := storage.NewMemory(storage.Metadata{FormatVersion: 2, Seed: 42, SpawnDimension: core.Overworld})
	seedMeleePlayer := func(identity network.Identity, position mgl32.Vec3) {
		location := storage.PlayerLocation{Dimension: core.Overworld, Position: [3]float32(position)}
		if _, err := store.SavePlayer(context.Background(), wellFedPlayerSave(storage.PlayerSave{
			PlayerID: identity.PlayerID, Revision: 1, DisplayName: identity.DisplayName,
			Current: location, Safe: &location,
		})); err != nil {
			t.Fatal(err)
		}
	}
	seedMeleePlayer(attacker, mgl32.Vec3{0.5, 1.001, 4.5})
	seedMeleePlayer(target, mgl32.Vec3{0.5, 1.001, 2.5})
	config := hostTestConfig()
	config.MaxPlayers = 2
	config.ViewRadius = 0
	config.AutosaveTicks = 1000
	host := mustNewHost(t, config, flatGenerator{}, store)
	attackerEndpoint, attackerDone, closeAttackerTransport := openParityTransport(t, host, transport, attacker)
	targetEndpoint, targetDone, closeTargetTransport := openParityTransport(t, host, transport, target)
	t.Cleanup(func() {
		_ = attackerEndpoint.Close()
		_ = targetEndpoint.Close()
		closeAttackerTransport()
		closeTargetTransport()
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		for _, done := range []<-chan error{attackerDone, targetDone} {
			select {
			case err := <-done:
				if err != nil && !errors.Is(err, network.ErrClosed) {
					t.Errorf("%s melee accept worker: %v", transport, err)
				}
			case <-ctx.Done():
				t.Errorf("%s melee accept worker 未退出: %v", transport, ctx.Err())
			}
		}
		if err := host.Shutdown(ctx); err != nil {
			t.Errorf("%s melee Host.Shutdown: %v", transport, err)
		}
	})

	clients := [2]network.ClientEndpoint{attackerEndpoint, targetEndpoint}
	identities := [2]core.PlayerID{attacker.PlayerID, target.PlayerID}
	ready := [2]bool{}
	for !ready[0] || !ready[1] {
		states, _ := meleeWireTick(t, host, clients, identities)
		for index, state := range states {
			ready[index] = ready[index] || state.Ready
		}
	}
	remoteReady := false
	for range 10 {
		_, remotes := meleeWireTick(t, host, clients, identities)
		if remotes[0].PlayerID == target.PlayerID && remotes[1].PlayerID == attacker.PlayerID {
			remoteReady = true
			break
		}
	}
	if !remoteReady {
		t.Fatal("近战脚本未从 wire 收到双方远端位置")
	}

	sendIntegration(t, attackerEndpoint, network.PlayerInput{Sequence: 1, Yaw: 0, Pitch: 0, Mining: true})
	waitIntegrationCondition(t, fmt.Sprintf("%s melee input queued", transport), func() bool {
		return len(host.world.incoming) > 0
	})
	result := meleeWireTranscript{Healths: make([]uint8, 0, 10), RemoteSamples: make([][3]uint32, 0, 100)}
	lastHealth := core.MaxHealth
	for range 120 {
		states, remotes := meleeWireTick(t, host, clients, identities)
		state := states[1]
		if state.Health != lastHealth {
			result.Healths = append(result.Healths, state.Health)
			lastHealth = state.Health
		}
		if remotes[1].PlayerID == attacker.PlayerID {
			result.RemoteSamples = append(result.RemoteSamples, remotePositionBits(remotes[1].Position))
		}
		if state.Reset && state.Health == core.MaxHealth {
			result.DeathReset = true
			break
		}
	}
	if !result.DeathReset || len(result.RemoteSamples) == 0 {
		t.Fatalf("%s 近战未在限制 tick 内观察到目标死亡 reset: %+v", transport, result)
	}
	return result
}

func meleeWireTick(
	t *testing.T,
	host *Host,
	clients [2]network.ClientEndpoint,
	identities [2]core.PlayerID,
) ([2]network.PlayerState, [2]network.RemotePlayerState) {
	t.Helper()
	result := host.world.StepForTest()
	var states [2]network.PlayerState
	var remotes [2]network.RemotePlayerState
	for index, endpoint := range clients {
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		for {
			message, err := endpoint.Recv(ctx)
			if err != nil {
				cancel()
				t.Fatalf("近战 tick %d player %d Recv: %v", result.Tick, index, err)
			}
			switch message := message.(type) {
			case network.RemotePlayerStates:
				for _, remote := range message.Players {
					if remote.PlayerID == identities[1-index] {
						remotes[index] = remote
					}
				}
			case network.PlayerState:
				cancel()
				if message.ServerTick != result.Tick {
					t.Fatalf("近战 player %d ServerTick=%d，想要 %d", index, message.ServerTick, result.Tick)
				}
				assertValidIntegrationPlayerState(t, message)
				states[index] = message
				goto next
			}
		}
	next:
	}
	return states, remotes
}

func remotePositionBits(position mgl32.Vec3) [3]uint32 {
	return [3]uint32{math.Float32bits(position[0]), math.Float32bits(position[1]), math.Float32bits(position[2])}
}
