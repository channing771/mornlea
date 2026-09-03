package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestUnregisterActiveReturnsLastSnapshotAndDropsOldCommands(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	id := SessionID(31)
	current := PlayerLocation{
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{2.5, 1, 0.5},
	}
	safe := PlayerLocation{
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{16.5, 1, 0.5},
	}
	loadFlatChunks(t, engine.dimension(core.Overworld), 0, 1, 0, 0)
	engine.RegisterPlayer(id, PlayerRestore{
		Current:        &current,
		Safe:           &safe,
		Yaw:            0.625,
		Pitch:          -0.125,
		SpawnDimension: core.Overworld,
	})
	active := onlyRuntimePlayerUpdate(t, engine.Step(), id)
	if !active.Ready || len(engine.wanted) == 0 {
		t.Fatalf("active=%+v wanted=%+v", active, engine.wanted)
	}
	engine.Enqueue(Command{
		Session: id, Sequence: 1, Kind: CommandPlayerInput,
		Yaw: 0.875, Pitch: 0.25,
	})
	engine.Step()
	before, ok := engine.PlayerSnapshot(id)
	if !ok {
		t.Fatal("active player snapshot unavailable")
	}
	lastPosition := mgl32.Vec3{3.5, 1, 0.5}
	engine.SetPlayerPositionForTest(id, lastPosition)
	engine.Enqueue(Command{
		Session: id, Sequence: 2, Kind: CommandPlayerInput, MoveX: 1,
	})

	snapshot, ok := engine.UnregisterSession(id)
	if !ok ||
		snapshot.Current != (PlayerLocation{
			Dimension: core.Overworld,
			Position:  lastPosition,
		}) ||
		snapshot.Yaw != 0.875 || snapshot.Pitch != 0.25 ||
		snapshot.Safe == nil || before.Safe == nil || *snapshot.Safe != *before.Safe {
		t.Fatalf("UnregisterSession=(%+v,%v)", snapshot, ok)
	}
	engine.Enqueue(Command{
		Session: id, Sequence: 3, Kind: CommandPlayerInput, MoveZ: 1,
	})
	result := engine.Step()
	if len(result.Players) != 0 || len(result.Rejected) != 0 ||
		len(engine.wanted) != 0 {
		t.Fatalf("old command or subscription survived: result=%+v wanted=%+v",
			result, engine.wanted)
	}
	if _, exists := engine.subscriptions[id]; exists {
		t.Fatal("old command recreated unregistered session")
	}
	if _, repeated := engine.UnregisterSession(id); repeated {
		t.Fatal("repeated unregister returned a snapshot")
	}
}

func TestUnregisterPendingReturnsNoSnapshotAndShrinksSubscriptions(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	id := SessionID(32)
	engine.RegisterPlayer(id, PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{},
	})
	requested := engine.Step()
	if len(requested.Acquire) == 0 || len(engine.wanted) == 0 {
		t.Fatalf("pending subscription not established: %+v", requested)
	}
	if snapshot, ok := engine.PlayerSnapshot(id); ok || snapshot != (PlayerSnapshot{}) {
		t.Fatalf("pending PlayerSnapshot=(%+v,%v), want zero,false", snapshot, ok)
	}

	snapshot, ok := engine.UnregisterSession(id)
	if ok || snapshot != (PlayerSnapshot{}) {
		t.Fatalf("pending UnregisterSession=(%+v,%v), want zero,false", snapshot, ok)
	}
	result := engine.Step()
	if len(result.Players) != 0 || len(engine.wanted) != 0 {
		t.Fatalf("pending session survived unregister: result=%+v wanted=%+v",
			result, engine.wanted)
	}
	if _, exists := engine.subscriptions[id]; exists {
		t.Fatal("pending session still registered")
	}
	if _, repeated := engine.UnregisterSession(id); repeated {
		t.Fatal("repeated pending unregister returned true")
	}
}

func TestOnlyRegisteredObserverAcceptsObserverCommands(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.Enqueue(Command{
		Session: 40, Sequence: 1, Kind: CommandTrustedObserverCenter,
		Dimension: core.Overworld, Center: core.ChunkPos{X: 7},
	})
	engine.Enqueue(Command{
		Session: 40, Sequence: 2, Kind: CommandResync,
		Dimension: core.Overworld,
	})
	unknown := engine.Step()
	if len(unknown.Acquire) != 0 || len(unknown.Resync) != 0 ||
		len(unknown.Rejected) != 0 {
		t.Fatalf("unknown commands were not silent: %+v", unknown)
	}
	if _, exists := engine.subscriptions[40]; exists {
		t.Fatal("unknown observer command registered a session")
	}

	const observer = SessionID(41)
	engine.RegisterObserverSession(observer)
	engine.Enqueue(Command{
		Session: observer, Sequence: 99, Kind: CommandPlayerInput, MoveX: 1,
	})
	engine.Enqueue(Command{
		Session: observer, Sequence: 1, Kind: CommandTrustedObserverCenter,
		Dimension: core.Overworld, Center: core.ChunkPos{X: 3, Z: -2},
	})
	registered := engine.Step()
	want := core.ChunkKey{
		Dimension: core.Overworld,
		Pos:       core.ChunkPos{X: 3, Z: -2},
	}
	if len(registered.Rejected) != 0 || len(registered.Acquire) != 1 ||
		registered.Acquire[0] != want {
		t.Fatalf("registered observer result=%+v, want acquire %+v",
			registered, want)
	}
}

func TestUnregisterObserverRemovesSubscriptionAndAllowsReregister(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	const observer = SessionID(71)
	key := core.ChunkKey{
		Dimension: core.Overworld,
		Pos:       core.ChunkPos{X: 3, Z: -2},
	}
	engine.RegisterObserverSession(observer)
	engine.Enqueue(Command{
		Session: observer, Sequence: 1, Kind: CommandTrustedObserverCenter,
		Dimension: key.Dimension, Center: key.Pos,
	})
	if result := engine.Step(); len(result.Acquire) != 1 ||
		result.Acquire[0] != key || !engine.WantsChunk(key) {
		t.Fatalf("observer subscription = %+v", result)
	}

	if snapshot, ok := engine.UnregisterSession(observer); ok ||
		snapshot != (PlayerSnapshot{}) {
		t.Fatalf("observer UnregisterSession = (%+v, %v)", snapshot, ok)
	}
	engine.Step()
	if engine.WantsChunk(key) {
		t.Fatal("observer unregister 后 union 仍需要旧区块")
	}
	engine.RegisterObserverSession(observer)
}

func TestRuntimeResetDoesNotReplayLoginRestoreCandidates(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	id := SessionID(15)
	current := PlayerLocation{
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{31.9, 1, 0.5},
	}
	safe := PlayerLocation{
		Dimension: core.Overworld,
		Position:  mgl32.Vec3{64.5, 1, 0.5},
	}
	engine.RegisterPlayer(id, PlayerRestore{
		Current:        &current,
		Safe:           &safe,
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{},
	})
	loadFlatChunks(t, engine.dimension(core.Overworld), 0, 1, 0, 0)

	waiting := onlyRuntimePlayerUpdate(t, engine.Step(), id)
	if waiting.Ready {
		t.Fatalf("restore crossed unknown chunk: %+v", waiting)
	}
	missing := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 2}}
	dimension := engine.dimension(missing.Dimension)
	if !dimension.MarkGenerating(missing.Pos) {
		t.Fatalf("restore chunk %+v did not enter generation", missing)
	}
	if err := dimension.ApplyGenerated(missing.Pos, movementFlatChunk(missing.Pos)); err != nil {
		t.Fatal(err)
	}
	activatedResult := engine.Step()
	activated := onlyRuntimePlayerUpdate(t, activatedResult, id)
	if !activated.Ready || activated.State.Position != current.Position {
		t.Fatalf("login restore=%+v", activated)
	}
	wantForget := []core.ChunkKey{
		{Dimension: core.Overworld, Pos: core.ChunkPos{}},
		{Dimension: core.Overworld, Pos: core.ChunkPos{X: 2}},
	}
	gotForget := activatedResult.Forget[id]
	if len(gotForget) != len(wantForget) ||
		gotForget[0] != wantForget[0] || gotForget[1] != wantForget[1] {
		t.Fatalf("restore subscription shrink=%+v, want %+v", gotForget, wantForget)
	}

	engine.SetPlayerPositionForTest(id, mgl32.Vec3{31.9, float32(core.MinY - 17), 0.5})
	resetting := onlyRuntimePlayerUpdate(t, engine.Step(), id)
	if resetting.Ready {
		t.Fatalf("invalid state did not enter PendingSpawn: %+v", resetting)
	}
	respawned := onlyRuntimePlayerUpdate(t, engine.Step(), id)
	if !respawned.Ready || respawned.State.Position != (mgl32.Vec3{0.5, 1, 0.5}) {
		t.Fatalf("runtime reset replayed login restore: %+v", respawned)
	}
	idle := engine.Step()
	stable := onlyRuntimePlayerUpdate(t, idle, id)
	if stable.State.Position != respawned.State.Position ||
		len(idle.Acquire) != 0 || len(idle.Generate) != 0 {
		t.Fatalf("idle tick replayed login restore candidates: %+v", idle)
	}
}

func onlyRuntimePlayerUpdate(t *testing.T, result TickResult, id SessionID) PlayerUpdate {
	t.Helper()
	for _, player := range result.Players {
		if player.Session == id {
			return player
		}
	}
	t.Fatalf("result missing player %d: %+v", id, result.Players)
	return PlayerUpdate{}
}
