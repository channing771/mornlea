package runtime

import (
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/server/sim/realm"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

func requireChunkInfo(t *testing.T, dimension *Dimension, pos core.ChunkPos) realm.ChunkInfo {
	t.Helper()
	info, exists := dimension.Info(pos)
	if !exists {
		t.Fatalf("chunk %+v is absent", pos)
	}
	return info
}

func TestGeneratedChunkIsDirtyUntilPersisted(t *testing.T) {
	dimension := NewDimension(core.Overworld)
	pos := core.ChunkPos{X: 2, Z: -4}
	if !dimension.BeginGeneration(pos) {
		t.Fatal("generation not started")
	}
	if err := dimension.ApplyGenerated(pos, world.NewChunk(pos)); err != nil {
		t.Fatal(err)
	}
	info := requireChunkInfo(t, dimension, pos)
	if info.Revision != 1 || info.PersistedRevision != 0 || !info.Dirty {
		t.Fatalf("generated record=%+v", info)
	}
	if unloaded := dimension.RequestUnload(pos); unloaded ||
		requireChunkInfo(t, dimension, pos).State != realm.ChunkUnloading ||
		requireChunkInfo(t, dimension, pos).Chunk == nil || !requireChunkInfo(t, dimension, pos).UnloadRequested {
		t.Fatalf("dirty chunk was discarded: %+v", requireChunkInfo(t, dimension, pos))
	}
}

func TestLoadedChunkKeepsPersistedRevisionAndCancelsUnload(t *testing.T) {
	pos := core.ChunkPos{}
	clean := NewDimension(core.Overworld)
	if !clean.BeginLoading(pos) {
		t.Fatal("load not started")
	}
	if err := clean.ApplyLoaded(pos, world.NewChunk(pos), 7, 7, false, false); err != nil {
		t.Fatal(err)
	}
	if !clean.RequestUnload(pos) {
		t.Fatal("clean loaded chunk should unload immediately")
	}
	if _, exists := clean.Info(pos); exists {
		t.Fatal("clean loaded chunk was retained")
	}

	dirty := NewDimension(core.Overworld)
	if !dirty.BeginLoading(pos) {
		t.Fatal("dirty load not started")
	}
	if err := dirty.ApplyLoaded(pos, world.NewChunk(pos), 8, 7, false, false); err != nil {
		t.Fatal(err)
	}
	if unloaded := dirty.RequestUnload(pos); unloaded {
		t.Fatal("dirty loaded chunk was discarded")
	}
	info := requireChunkInfo(t, dirty, pos)
	chunk := info.Chunk
	if info.State != realm.ChunkUnloading || chunk == nil || !info.UnloadRequested {
		t.Fatalf("dirty unload=%+v", info)
	}
	if !dirty.CancelUnload(pos) {
		t.Fatal("cancel unload failed")
	}
	info = requireChunkInfo(t, dirty, pos)
	if info.State != realm.ChunkReady ||
		info.Chunk != chunk || info.UnloadRequested {
		t.Fatalf("cancel unload=%+v", info)
	}
}

func TestPersistenceLifecyclePreservesRecoveryAndRewriteMetadata(t *testing.T) {
	tests := []struct {
		name              string
		revision          uint64
		persistedRevision uint64
		needsRewrite      bool
		recovered         bool
	}{
		{
			name:              "recovered older payload",
			revision:          3,
			persistedRevision: 1,
			needsRewrite:      true,
			recovered:         true,
		},
		{
			name:              "format migration rewrite",
			revision:          7,
			persistedRevision: 7,
			needsRewrite:      true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dimension := NewDimension(core.Overworld)
			pos := core.ChunkPos{X: -3, Z: 5}
			if !dimension.BeginLoading(pos) {
				t.Fatal("load not started")
			}
			if err := dimension.ApplyLoaded(
				pos,
				world.NewChunk(pos),
				test.revision,
				test.persistedRevision,
				test.needsRewrite,
				test.recovered,
			); err != nil {
				t.Fatal(err)
			}
			info := requireChunkInfo(t, dimension, pos)
			if info.Revision != test.revision ||
				info.PersistedRevision != test.persistedRevision ||
				info.NeedsRewrite != test.needsRewrite ||
				info.Recovered != test.recovered || !info.Dirty {
				t.Fatalf("loaded record=%+v", info)
			}
		})
	}
}

func TestRecoveredOnlyLoadedChunkRequiresRewriteOnUnload(t *testing.T) {
	dimension := NewDimension(core.Overworld)
	pos := core.ChunkPos{X: 7, Z: -11}
	if !dimension.BeginLoading(pos) {
		t.Fatal("load not started")
	}
	if err := dimension.ApplyLoaded(pos, world.NewChunk(pos), 6, 6, false, true); err != nil {
		t.Fatal(err)
	}
	info := requireChunkInfo(t, dimension, pos)
	if !info.Recovered || !info.NeedsRewrite || !info.Dirty {
		t.Fatalf("recovered-only record=%+v", info)
	}
	if dimension.RequestUnload(pos) {
		t.Fatal("recovered-only chunk discarded")
	}
	info = requireChunkInfo(t, dimension, pos)
	if info.State != realm.ChunkUnloading ||
		!info.UnloadRequested || info.Chunk == nil {
		t.Fatalf("recovered-only unload=%+v", info)
	}
}

func TestEngineAcquiredHitPropagatesExactPersistenceState(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	key := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -6, Z: 3}}
	engine.RegisterObserverSession(71)
	engine.Enqueue(Command{
		Session: 71, Sequence: 1, Kind: CommandTrustedObserverCenter,
		Dimension: key.Dimension, Center: key.Pos,
	})
	requested := engine.Step()
	if !reflect.DeepEqual(requested.Acquire, []core.ChunkKey{key}) {
		t.Fatalf("Acquire=%+v", requested.Acquire)
	}
	chunk := world.NewChunk(key.Pos)
	engine.SubmitAcquired(AcquiredChunk{
		Key:               key,
		Chunk:             chunk,
		Revision:          12,
		PersistedRevision: 9,
		NeedsRewrite:      true,
		Recovered:         true,
	})

	loaded := engine.Step()
	info := requireChunkInfo(t, engine.dimension(key.Dimension), key.Pos)
	if !reflect.DeepEqual(loaded.Ready, []core.ChunkKey{key}) ||
		info.State != realm.ChunkReady || info.Chunk != chunk ||
		info.Revision != 12 || info.PersistedRevision != 9 ||
		!info.NeedsRewrite || !info.Recovered {
		t.Fatalf("Ready=%+v record=%+v", loaded.Ready, info)
	}
}

func TestEngineForgottenCleanAcquiredHitIsDeleted(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	oldKey := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 3}}
	newKey := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 40}}
	engine.RegisterObserverSession(72)
	engine.Enqueue(Command{
		Session: 72, Sequence: 1, Kind: CommandTrustedObserverCenter,
		Dimension: oldKey.Dimension, Center: oldKey.Pos,
	})
	engine.Step()
	engine.Enqueue(Command{
		Session: 72, Sequence: 2, Kind: CommandTrustedObserverCenter,
		Dimension: newKey.Dimension, Center: newKey.Pos,
	})
	engine.Step()
	engine.SubmitAcquired(AcquiredChunk{
		Key: oldKey, Chunk: world.NewChunk(oldKey.Pos),
		Revision: 5, PersistedRevision: 5,
	})

	loaded := engine.Step()
	if len(loaded.Ready) != 0 {
		t.Fatalf("forgotten clean hit published Ready=%+v", loaded.Ready)
	}
	if _, exists := engine.dimension(oldKey.Dimension).Info(oldKey.Pos); exists {
		t.Fatal("forgotten clean hit retained authority")
	}
}

func TestPersistenceLifecycleRejectsPersistedRevisionAboveCurrent(t *testing.T) {
	dimension := NewDimension(core.Overworld)
	pos := core.ChunkPos{X: 1, Z: 1}
	if !dimension.BeginLoading(pos) {
		t.Fatal("load not started")
	}
	if err := dimension.ApplyLoaded(pos, world.NewChunk(pos), 4, 5, false, false); err == nil {
		t.Fatal("persisted revision above current was accepted")
	}
	if info := requireChunkInfo(t, dimension, pos); info.State != realm.ChunkLoading || info.Chunk != nil {
		t.Fatalf("invalid load changed authority: %+v", info)
	}
}

func TestPersistenceLifecycleLoadTransitionsSupportMissDropAndRetry(t *testing.T) {
	dimension := NewDimension(core.Overworld)
	generatePos := core.ChunkPos{X: 3}
	if !dimension.BeginLoading(generatePos) || !dimension.MarkGenerating(generatePos) {
		t.Fatal("load miss did not transition Loading to Generating")
	}
	if err := dimension.ApplyGenerated(generatePos, world.NewChunk(generatePos)); err != nil {
		t.Fatal(err)
	}

	droppedPos := core.ChunkPos{X: 4}
	if !dimension.BeginLoading(droppedPos) {
		t.Fatal("drop load not started")
	}
	dimension.DropLoading(droppedPos)
	if _, exists := dimension.Info(droppedPos); exists {
		t.Fatal("dropped loading record was retained")
	}

	failedPos := core.ChunkPos{X: 5}
	wantErr := errors.New("load failed")
	if !dimension.BeginLoading(failedPos) {
		t.Fatal("failed load not started")
	}
	dimension.MarkLoadFailed(failedPos, wantErr)
	info := requireChunkInfo(t, dimension, failedPos)
	if info.State != realm.ChunkFailed || !errors.Is(info.Err, wantErr) {
		t.Fatalf("failed load record=%+v", info)
	}
	if !dimension.BeginLoading(failedPos) || requireChunkInfo(t, dimension, failedPos).Err != nil {
		t.Fatalf("failed load did not retry cleanly: %+v", requireChunkInfo(t, dimension, failedPos))
	}
}

func TestPersistenceLifecycleBlockChangeAdvancesDirtyRevision(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.Enqueue(Command{
		Session:  session,
		Sequence: 1,
		Kind:     CommandPlayerInput,
		Pitch:    -float32(math.Pi)/2 + 0.01,
		Mining:   true,
	})
	var result TickResult
	for range 5 {
		result = engine.Step()
	}
	if len(result.Changes) != 1 {
		t.Fatalf("block change batches=%+v", result.Changes)
	}
	info := requireChunkInfo(t, engine.dimension(core.Overworld), core.ChunkPos{})
	if info.Revision != 2 || info.PersistedRevision != 0 || !info.Dirty {
		t.Fatalf("changed record=%+v", info)
	}
}

func TestPersistenceLifecycleRetainsLateGeneratedChunkForSaving(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	const session = SessionID(41)
	engine.RegisterObserverSession(session)
	first := core.ChunkPos{X: 2, Z: -4}
	second := core.ChunkPos{X: 20, Z: 20}
	engine.Enqueue(Command{
		Session: session, Sequence: 1, Kind: CommandTrustedObserverCenter,
		Dimension: core.Overworld, Center: first,
	})
	acquired := engine.Step()
	engine.SubmitAcquired(AcquiredChunk{
		Key: core.ChunkKey{Dimension: core.Overworld, Pos: first}, Missing: true,
	})
	generated := engine.Step()
	if len(acquired.Acquire) != 1 || len(generated.Generate) != 1 {
		t.Fatalf("acquire=%+v generate=%+v", acquired.Acquire, generated.Generate)
	}
	engine.Enqueue(Command{
		Session: session, Sequence: 2, Kind: CommandTrustedObserverCenter,
		Dimension: core.Overworld, Center: second,
	})
	engine.Step()

	engine.SubmitGenerated(GeneratedChunk{
		Dimension: core.Overworld,
		Pos:       first,
		Chunk:     world.NewChunk(first),
	})
	result := engine.Step()
	key := core.ChunkKey{Dimension: core.Overworld, Pos: first}
	for _, ready := range result.Ready {
		if ready == key {
			t.Fatalf("forgotten chunk published ready: %+v", result.Ready)
		}
	}
	info := requireChunkInfo(t, engine.dimension(core.Overworld), first)
	if info.State != realm.ChunkUnloading || info.Chunk == nil ||
		info.Revision != 1 || info.PersistedRevision != 0 || !info.Dirty {
		t.Fatalf("late generated chunk was not retained: %+v", info)
	}

	engine.Enqueue(Command{
		Session: session, Sequence: 3, Kind: CommandTrustedObserverCenter,
		Dimension: core.Overworld, Center: first,
	})
	resubscribed := engine.Step()
	info = requireChunkInfo(t, engine.dimension(core.Overworld), first)
	if info.State != realm.ChunkReady || info.UnloadRequested ||
		!reflect.DeepEqual(resubscribed.Ready, []core.ChunkKey{key}) {
		t.Fatalf("retained chunk was not reused: record=%+v ready=%+v", info, resubscribed.Ready)
	}
}
