package runtime

import (
	"math"
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestEngineAppliesOnlyLatestPlayerInputOncePerTick(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	before, _ := engine.Player(session)
	engine.Enqueue(Command{Session: session, Sequence: 2, Kind: CommandPlayerInput, MoveZ: 1})
	engine.Enqueue(Command{Session: session, Sequence: 3, Kind: CommandPlayerInput, MoveX: 1})

	after := onlyOrchestratedMovementPlayer(t, engine.Step())
	if after.LastInputSequence != 3 {
		t.Fatalf("ack=%d，想要 3", after.LastInputSequence)
	}
	if after.State.Position.Z() != before.State.Position.Z() {
		t.Fatalf("较早 MoveZ 被执行: before=%v after=%v", before.State, after.State)
	}
	if after.State.Position.X() <= before.State.Position.X() {
		t.Fatalf("最新 MoveX 未执行: before=%v after=%v", before.State, after.State)
	}
}

func TestPlayerReplayProducesIdenticalPlayerAndWorldHashes(t *testing.T) {
	type replayState struct {
		playerHash [32]byte
		chunkHash  [32]byte
		revision   uint64
	}
	run := func() replayState {
		engine, session := readyMovementPlayer(t)
		script := [][]Command{
			{{Session: session, Sequence: 2, Kind: CommandPlayerInput, MoveX: 1, Yaw: 0.25, Pitch: -0.1}},
			nil,
			{
				{Session: session, Sequence: 3, Kind: CommandPlayerInput, MoveZ: 1, Yaw: -0.75, Pitch: 0.2},
				{Session: session, Sequence: 4, Kind: CommandPlayerInput, MoveX: -1, Yaw: 1.25, Pitch: 0.3},
			},
		}
		for _, commands := range script {
			for _, command := range commands {
				engine.Enqueue(command)
			}
			engine.Step()
		}
		playerHash, ok := engine.PlayerHash(session)
		if !ok {
			t.Fatal("权威玩家 hash 不可用")
		}
		chunkHash, revision, ok := engine.ChunkHash(core.ChunkKey{Dimension: core.Overworld})
		if !ok {
			t.Fatal("权威区块 hash 不可用")
		}
		return replayState{playerHash: playerHash, chunkHash: chunkHash, revision: revision}
	}

	if first, second := run(), run(); !reflect.DeepEqual(first, second) {
		t.Fatalf("两次玩家移动 replay 不同: %v != %v", first, second)
	}
}

func TestEngineMovesBeforeReconcilingAndExecutingInteractions(t *testing.T) {
	engine, sessionID := readyMovementPlayer(t)
	nextChunk := core.ChunkPos{X: 1}
	loadFlatChunks(t, engine.dimension(core.Overworld), 1, 1, 0, 0)
	engine.SetPlayerPositionForTest(sessionID, mgl32.Vec3{15.9, 1, 0.5})
	engine.Enqueue(Command{
		Session: sessionID, Sequence: 2, Kind: CommandPlayerInput, MoveX: 1,
	})
	engine.Enqueue(Command{
		Session: sessionID, Sequence: 3, Kind: CommandPlayerInput,
		MoveX: 1, Pitch: -float32(math.Pi)/2 + 0.01, Mining: true,
	})

	result := engine.Step()
	after := onlyOrchestratedMovementPlayer(t, result)
	if after.ViewCenter != nextChunk || after.State.Position.X() < 16 {
		t.Fatalf("订阅中心没有使用本 tick 权威移动结果: %+v", after)
	}
	if len(result.Rejected) != 0 || !after.Mining.Active || after.Mining.Target.Chunk() != nextChunk {
		t.Fatalf("移动后的新订阅没有在交互前生效: %+v", result)
	}
	chunk, _, ok := engine.CloneReadyChunk(core.ChunkKey{Dimension: core.Overworld, Pos: nextChunk})
	if !ok || chunk.BlockAt(0, 0, 0) != core.GrassID {
		t.Fatalf("权威采掘完成前修改了新订阅区块: ok=%v chunk=%v", ok, chunk)
	}
}

func TestPlayerCenterDerivationAlsoRunsWhenTrustedObserverChanges(t *testing.T) {
	engine, sessionID := readyMovementPlayer(t)
	engine.RegisterObserverSession(2)
	loadFlatChunks(t, engine.dimension(core.Overworld), 1, 1, 0, 0)
	engine.SetPlayerPositionForTest(sessionID, mgl32.Vec3{15.9, 1, 0.5})
	engine.Enqueue(Command{
		Session: sessionID, Sequence: 2, Kind: CommandPlayerInput, MoveX: 1,
	})
	engine.Enqueue(Command{
		Session: 2, Sequence: 1, Kind: CommandTrustedObserverCenter,
		Dimension: core.Overworld, Center: core.ChunkPos{X: 8},
	})

	playerUpdate := onlyOrchestratedMovementPlayer(t, engine.Step())
	if playerUpdate.ViewCenter != (core.ChunkPos{X: 1}) {
		t.Fatalf("legacy view 变化阻止了玩家中心派生: %+v", playerUpdate)
	}
}

func onlyOrchestratedMovementPlayer(t *testing.T, result TickResult) PlayerUpdate {
	t.Helper()
	if len(result.Players) != 1 {
		t.Fatalf("Players=%+v，想要恰好一个", result.Players)
	}
	return result.Players[0]
}
