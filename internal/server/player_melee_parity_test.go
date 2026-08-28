package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/sim"
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

func TestEightPlayersSameTickPrimaryInputKeepsSessionOrder(t *testing.T) {
	const seed int64 = 160017
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{}}
	memory := storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: seed, SpawnDimension: core.Overworld})
	if _, err := memory.SaveBatch(context.Background(), []storage.ChunkSave{{
		Key: key, Revision: 1, Chunk: multiplayerManualGenerator{}.GenerateChunk(key.Pos),
	}}); err != nil {
		t.Fatal(err)
	}
	tracked := &trackedMemoryStore{MemoryStore: memory}
	config := hostTestConfig()
	config.Seed = seed
	config.MaxPlayers = multiplayerClientCount
	config.ViewRadius = 0
	config.OutboxCapacity = 4096
	running := NewWorld(config, multiplayerManualGenerator{}, tracked)
	clients := make([]*multiplayerTCPClient, multiplayerClientCount)
	t.Cleanup(func() {
		for _, connected := range clients {
			if connected != nil {
				_ = closeTask16Client(connected)
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), longWaitDeadline)
		defer cancel()
		if err := running.Shutdown(ctx); err != nil {
			t.Errorf("关闭八人同 tick 近战服务端: %v", err)
		}
	})
	// Session 2 先连接，若近战错误地按插入顺序而不是 `SessionID` 处理，下面的
	// wire 采掘分流断言会恰好反转。
	for _, index := range [...]int{1, 0, 2, 3, 4, 5, 6, 7} {
		identity := multiplayerIdentity(byte(0xa0+index), multiplayerNames[index])
		clientEndpoint, serverEndpoint := network.NewMemoryPair(4096)
		if _, err := running.AttachSession(eightMeleeSessionSpec(index, identity, serverEndpoint)); err != nil {
			_ = clientEndpoint.Close()
			_ = serverEndpoint.Close()
			t.Fatalf("AttachSession player %d: %v", index, err)
		}
		clients[index] = &multiplayerTCPClient{
			identity: identity, endpoint: clientEndpoint, receiver: client.NewReceiver(clientEndpoint, 4096),
			mirror: client.NewMirror(), drops: client.NewItemDrops(), remotes: client.NewRemotePlayers(),
		}
	}
	warmCtx, cancelWarm := context.WithTimeout(context.Background(), longWaitDeadline)
	defer cancelWarm()
	for !manualMultiplayerStable(running, tracked, clients, key) {
		result := running.StepForTest()
		drainMultiplayerClientsToTick(t, warmCtx, "eight-melee", clients, result.Tick)
		if err := warmCtx.Err(); err != nil {
			t.Fatalf("eight-melee warm-up: %v", err)
		}
	}
	for index, connected := range clients {
		// 前两个会话共同瞄准第三个；第二个在目标冷却后必须转入既有采掘分支。
		input := network.PlayerInput{Sequence: 1, Yaw: math.Pi, Pitch: -0.2, Mining: true}
		if index%2 == 1 {
			input.Yaw = 0
		}
		if index == 1 {
			input.Yaw = float32(math.Pi - math.Atan2(0.7, 2))
		}
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		err := connected.endpoint.Send(ctx, input)
		cancel()
		if err != nil {
			t.Fatalf("player %d primary input: %v", index, err)
		}
	}
	waitIntegrationCondition(t, "eight melee inputs queued", func() bool { return len(running.incoming) == multiplayerClientCount })
	result := running.StepForTest()
	drainCtx, cancelDrain := context.WithTimeout(context.Background(), waitDeadline)
	drainMultiplayerClientsToTick(t, drainCtx, "eight-melee", clients, result.Tick)
	cancelDrain()
	running.stepMu.Lock()
	sessionCount := len(running.sessions)
	running.stepMu.Unlock()
	if sessionCount != multiplayerClientCount {
		t.Fatalf("同 tick primary action 后 session=%d，想要 %d", sessionCount, multiplayerClientCount)
	}
	for index, connected := range clients {
		if connected.local.LastInputSequence != 1 {
			t.Fatalf("session %d sequence=%d，想要 1", index+1, connected.local.LastInputSequence)
		}
		switch index {
		case 0:
			if connected.local.Health != core.MaxHealth || connected.local.MiningActive {
				t.Fatalf("较小 SessionID 攻击者 state=%+v，想要未受伤且采掘被抑制", connected.local)
			}
		case 1:
			if connected.local.Health != core.MaxHealth || !connected.local.MiningActive ||
				connected.local.MiningTarget != multiplayerManualTarget || connected.local.MiningProgressTicks != 1 {
				t.Fatalf("较大 SessionID 攻击者 state=%+v，想要未受伤且继续采掘", connected.local)
			}
		case 2:
			if connected.local.Health != core.MaxHealth-2 {
				t.Fatalf("共享目标 state=%+v，想要只扣 2 血", connected.local)
			}
		default:
			if connected.local.Health != core.MaxHealth {
				t.Fatalf("容量玩家 %d state=%+v，想要满血", index+1, connected.local)
			}
		}
	}
}

func eightMeleeSessionSpec(index int, identity network.Identity, endpoint network.ServerEndpoint) SessionSpec {
	var position mgl32.Vec3
	if index < 3 {
		// 两名攻击者在第三名玩家前方并列，第二条斜射线在目标之后继续命中固定采掘墙。
		position = mgl32.Vec3{1.5, 1.001, 0.5}
		switch index {
		case 1:
			position[0] = 2.2
		case 2:
			position[2] = 2.5
		}
	} else {
		pair := index / 2
		position = mgl32.Vec3{float32(pair*4) + 0.5, 1.001, 4.5}
		if index%2 == 1 {
			position[2] = 2.5
		}
	}
	location := sim.PlayerLocation{Dimension: core.Overworld, Position: position}
	return SessionSpec{
		ID: sim.SessionID(index + 1), Generation: 1,
		PlayerID: identity.PlayerID, DisplayName: identity.DisplayName, Endpoint: endpoint,
		Restore: sim.PlayerRestore{Current: &location, Safe: &location, SpawnDimension: core.Overworld},
	}
}

func runPlayerMeleeWireScript(t *testing.T, transport string) meleeWireTranscript {
	t.Helper()
	attacker := integrationIdentity(0x91, "MeleeAttacker")
	target := integrationIdentity(0x92, "MeleeTarget")
	store := storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld})
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
	waitIntegrationLoginReady(
		t,
		fmt.Sprintf("%s melee wire", transport),
		func() bool { return ready[0] && ready[1] },
		func() string { return fmt.Sprintf("ready=%v", ready) },
		func() {
			states, _ := meleeWireTick(t, host, clients, identities)
			for index, state := range states {
				ready[index] = ready[index] || state.Ready
			}
		},
	)
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
