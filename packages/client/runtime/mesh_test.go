package runtime

import (
	"errors"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestMeshBudgetPublishesOneReadySection(t *testing.T) {
	runtime := newMeshRuntimeForTest(t, 32)
	applyMeshSnapshot(t, runtime, 1)

	waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool {
		return stats.ReadyOperations == 1
	})
	batch, ok, err := runtime.DrainWorldBatch(1)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("mesh budget produced no world batch")
	}
	upserts := batch.Upserts()
	if len(upserts) != 1 || len(batch.Drops()) != 0 {
		t.Fatalf("world batch upserts/drops = %d/%d, want 1/0", len(upserts), len(batch.Drops()))
	}
	wantKey := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{}}
	if upserts[0].Key != wantKey || upserts[0].Revision != 1 || upserts[0].Payload.Len() == 0 {
		t.Fatalf("first section upsert = %+v, want key %+v revision 1 with payload", upserts[0], wantKey)
	}
	if batch.Epoch != 1 || batch.AtlasRevision != 1 {
		t.Fatalf("world batch identity = epoch %d atlas %d, want 1/1", batch.Epoch, batch.AtlasRevision)
	}
	if stats := runtime.MeshStats(); stats.DirtySections != core.SectionsPerChunk-1 {
		t.Fatalf("one-section mesh budget left %d dirty sections, want %d", stats.DirtySections, core.SectionsPerChunk-1)
	}
}

func TestMeshBudgetZeroAndInvalidWorldBudgetPerformNoWork(t *testing.T) {
	runtime := newMeshRuntimeForTest(t, 32)
	applyMeshSnapshot(t, runtime, 1)
	before := runtime.MeshStats()
	if err := runtime.AdvanceMeshes(0); err != nil {
		t.Fatal(err)
	}
	if after := runtime.MeshStats(); after != before {
		t.Fatalf("zero mesh budget changed state: before=%+v after=%+v", before, after)
	}
	if err := runtime.AdvanceMeshes(-1); err == nil {
		t.Fatal("negative mesh budget was accepted")
	}
	if _, ok, err := runtime.DrainWorldBatch(0); err != nil || ok {
		t.Fatalf("zero world budget = (%v, %v), want no batch", ok, err)
	}
	if _, _, err := runtime.DrainWorldBatch(presentation.MaxWorldBatchOperations + 1); err == nil {
		t.Fatal("over-limit world budget was accepted")
	}
}

func TestMeshStaleResultIsRedirtiedBeforePublication(t *testing.T) {
	runtime := newMeshRuntimeForTest(t, 32)
	key := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{}}
	release := runtime.mesher.BlockForTest(key)
	t.Cleanup(release)
	applyMeshSnapshot(t, runtime, 1)
	if err := runtime.AdvanceMeshes(1); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool { return stats.InFlightJobs == 1 })

	if _, err := runtime.ApplyMessage(network.BlockChanges{
		Dimension: core.Overworld, Chunk: core.ChunkPos{}, BaseRevision: 1, NewRevision: 2,
		Changes: []network.BlockChange{{Position: core.BlockPos{Y: core.MinY + core.SectionSize - 1}, Block: core.DirtID}},
	}); err != nil {
		t.Fatal(err)
	}
	release()

	var current presentation.SectionMeshUpsert
	foundCurrent := false
	deadline := time.Now().Add(5 * time.Second)
	for !foundCurrent {
		waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool {
			return stats.ReadyOperations == 1
		})
		batch := drainRuntimeWorldBatch(t, runtime, 1)
		for _, upsert := range batch.Upserts() {
			if upsert.Key == key {
				current = upsert
				foundCurrent = true
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("current section was not published; stats=%+v", runtime.MeshStats())
		}
	}
	if current.Revision != 1 {
		t.Fatalf("current section revision = %d, want first publication revision", current.Revision)
	}
	packed := current.Payload.PackedQuads()
	if len(packed) == 0 {
		t.Fatal("current remesh published an empty payload")
	}
	foundDirt := false
	materials := make(map[uint16]int)
	for _, value := range packed {
		material := mesh.UnpackQuad(value).Mat
		materials[material]++
		if material == uint16(assets.LayerDirt) {
			foundDirt = true
			break
		}
	}
	if !foundDirt {
		t.Fatalf("published mesh retained the stale all-stone material: %v", materials)
	}
}

func TestSectionRevisionIncreasesForEmptyRemeshAndForget(t *testing.T) {
	runtime := newMeshRuntimeForTest(t, 64)
	key := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{}}
	applyMeshSnapshot(t, runtime, 1)
	waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool { return stats.ReadyOperations == 1 })
	first := drainRuntimeWorldBatch(t, runtime, 1)
	if got := first.Upserts()[0].Revision; got != 1 {
		t.Fatalf("initial section revision = %d, want 1", got)
	}

	if _, err := runtime.ApplyMessage(meshSnapshot(2, core.AirID)); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool { return stats.ReadyOperations == 1 })
	empty := drainRuntimeWorldBatch(t, runtime, 1)
	drops := empty.Drops()
	if len(empty.Upserts()) != 0 || len(drops) != 1 || drops[0].Key != key || drops[0].Revision != 2 {
		t.Fatalf("empty remesh = upserts %+v drops %+v, want key drop revision 2", empty.Upserts(), drops)
	}

	if _, err := runtime.ApplyMessage(network.ForgetChunks{
		Dimension: core.Overworld, Chunks: []core.ChunkPos{{}},
	}); err != nil {
		t.Fatal(err)
	}
	forgotten := drainRuntimeWorldBatch(t, runtime, core.SectionsPerChunk)
	var forgottenRevision uint64
	for _, drop := range forgotten.Drops() {
		if drop.Key == key {
			forgottenRevision = drop.Revision
		}
	}
	if forgottenRevision != 3 {
		t.Fatalf("forgotten section revision = %d, want 3", forgottenRevision)
	}
}

func TestMeshReadyOverflowStopsSessionWithoutPartialPublication(t *testing.T) {
	runtime := newMeshRuntimeForTest(t, 1)
	applyMeshSnapshot(t, runtime, 1)
	_, err := runtime.ApplyMessage(network.ForgetChunks{
		Dimension: core.Overworld, Chunks: []core.ChunkPos{{}},
	})
	if !errors.Is(err, ErrMeshReadyOverflow) {
		t.Fatalf("ForgetChunks error = %v, want %v", err, ErrMeshReadyOverflow)
	}
	if runtime.Phase() != ConnectionPhaseDisconnected || !errors.Is(runtime.Err(), ErrMeshReadyOverflow) {
		t.Fatalf("overflow phase/error = %v/%v, want disconnected overflow", runtime.Phase(), runtime.Err())
	}
	if _, ok, drainErr := runtime.DrainWorldBatch(1); ok || drainErr == nil {
		t.Fatalf("overflow DrainWorldBatch() = (%v, %v), want closed error without partial batch", ok, drainErr)
	}
}

func TestMeshReadyOverflowPreflightLeavesPendingQueueUnchanged(t *testing.T) {
	runtime := newMeshRuntimeForTest(t, 1)
	existing := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: -1}}
	runtime.sessionMu.Lock()
	runtime.sectionRevisions[existing] = 1
	runtime.meshReady.put(meshReadyOperation{key: existing, revision: 1})
	err := runtime.applyMeshUpdateLocked(client.MirrorUpdate{
		Dirty: []core.SectionKey{existing},
		Forgotten: []core.SectionKey{
			{Dimension: core.Overworld, Pos: core.SectionPos{X: 1}},
			{Dimension: core.Overworld, Pos: core.SectionPos{X: 2}},
		},
	})
	element := runtime.meshReady.ordered.Front()
	readyLen := runtime.meshReady.len()
	gotRevision := runtime.sectionRevisions[existing]
	runtime.sessionMu.Unlock()
	if !errors.Is(err, ErrMeshReadyOverflow) {
		t.Fatalf("mesh update error = %v, want %v", err, ErrMeshReadyOverflow)
	}
	if readyLen != 1 || element == nil || element.Value.(meshReadyOperation).key != existing || gotRevision != 1 {
		t.Fatalf("overflow changed pending queue or revision: len=%d element=%v revision=%d", readyLen, element, gotRevision)
	}
}

func TestMeshResetChangesEpochAndRejectsOldWorkerResults(t *testing.T) {
	runtime := newMeshRuntimeForTest(t, 32)
	key := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{}}
	release := runtime.mesher.BlockForTest(key)
	applyMeshSnapshot(t, runtime, 1)
	if err := runtime.AdvanceMeshes(1); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool { return stats.InFlightJobs == 1 })
	runtime.ResetMirrors()
	release()

	if stats := runtime.MeshStats(); stats.Epoch != 2 || stats.ReadyOperations != 0 {
		t.Fatalf("reset mesh stats = %+v, want epoch 2 with empty publication queue", stats)
	}
	applyMeshSnapshot(t, runtime, 1)
	waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool { return stats.ReadyOperations == 1 })
	batch := drainRuntimeWorldBatch(t, runtime, 1)
	if batch.Epoch != 2 {
		t.Fatalf("post-reset batch epoch = %d, want 2", batch.Epoch)
	}
	if upserts := batch.Upserts(); len(upserts) != 1 || upserts[0].Revision != 1 {
		t.Fatalf("post-reset upserts = %+v, want fresh revision 1", upserts)
	}
}

func newMeshRuntimeForTest(t *testing.T, readyCapacity int) *Runtime {
	t.Helper()
	runtime := newLoggedInRuntime(&messageTestReceiver{}, nil, 0)
	if err := runtime.ConfigureMeshing(MeshOptions{
		Registry: assets.NewRegistry(), Workers: 1, ReadyCapacity: readyCapacity,
		AtlasRevision: presentation.AtlasRevision(1),
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func applyMeshSnapshot(t *testing.T, runtime *Runtime, revision uint64) {
	t.Helper()
	if _, err := runtime.ApplyMessage(meshSnapshot(revision, core.StoneID)); err != nil {
		t.Fatal(err)
	}
}

func meshSnapshot(revision uint64, firstBlock core.BlockID) network.ChunkSnapshot {
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for index := range sections {
		block := core.AirID
		if index == 0 {
			block = firstBlock
		}
		sections[index] = network.SectionData{Y: int32(index), Storage: network.SectionSingle, Single: block}
	}
	return network.ChunkSnapshot{
		Dimension: core.Overworld, Chunk: core.ChunkPos{}, Revision: revision, Sections: sections,
	}
}

func waitForRuntimeMesh(t *testing.T, runtime *Runtime, ready func(MeshStats) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := runtime.AdvanceMeshes(1); err != nil {
			t.Fatal(err)
		}
		stats := runtime.MeshStats()
		if ready(stats) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("mesh condition timed out: %+v", stats)
		}
		time.Sleep(time.Millisecond)
	}
}

func drainRuntimeWorldBatch(t *testing.T, runtime *Runtime, budget int) presentation.WorldBatch {
	t.Helper()
	batch, ok, err := runtime.DrainWorldBatch(budget)
	if err != nil || !ok {
		t.Fatalf("DrainWorldBatch() = (%v, %v), want batch", ok, err)
	}
	return batch
}
