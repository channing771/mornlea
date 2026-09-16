package runtime

import (
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

// TestMeshRevisionOverflowStopsSessionWithoutPartialPublication keeps the
// no-partial-publication terminal contract on a genuinely unrepresentable state:
// a section whose revision counter is exhausted. Payload-less forget bursts no
// longer terminate (see the warp-scale test); revision exhaustion still does.
func TestMeshRevisionOverflowStopsSessionWithoutPartialPublication(t *testing.T) {
	runtime := newMeshRuntimeForTest(t, 8)
	applyMeshSnapshot(t, runtime, 1)
	key := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{}}
	runtime.sessionMu.Lock()
	runtime.sectionRevisions[key] = ^uint64(0)
	runtime.sessionMu.Unlock()
	_, err := runtime.ApplyMessage(network.ForgetChunks{
		Dimension: core.Overworld, Chunks: []core.ChunkPos{{}},
	})
	if err == nil {
		t.Fatal("revision-exhausted forget was accepted")
	}
	if runtime.Phase() != ConnectionPhaseDisconnected || runtime.Err() == nil {
		t.Fatalf("terminal phase/error = %v/%v, want disconnected failure", runtime.Phase(), runtime.Err())
	}
	if _, ok, drainErr := runtime.DrainWorldBatch(1); ok || drainErr == nil {
		t.Fatalf("terminal DrainWorldBatch() = (%v, %v), want closed error without partial batch", ok, drainErr)
	}
}

// TestMeshMassForgetKeepsUnrelatedPendingUpserts pins the upsert no-loss
// preflight property: a transition mixing dirty invalidation with a forget
// burst far beyond ready capacity leaves unrelated pending upserts and their
// revisions byte-identical and never terminates the session.
func TestMeshMassForgetKeepsUnrelatedPendingUpserts(t *testing.T) {
	runtime := newMeshRuntimeForTest(t, 1)
	survivor := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: -1}}
	invalidated := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: -2}}
	runtime.sessionMu.Lock()
	runtime.sectionRevisions[survivor] = 1
	runtime.sectionRevisions[invalidated] = 1
	runtime.meshReady.put(meshReadyOperation{
		key: survivor, revision: 1, quads: []mesh.Quad{{}}, upsert: true, meshed: true,
	})
	runtime.meshReady.put(meshReadyOperation{
		key: invalidated, revision: 1, quads: []mesh.Quad{{}}, upsert: true, meshed: true,
	})
	forgotten := make([]core.SectionKey, 0, 3*core.SectionsPerChunk)
	for x := int32(1); x <= 3; x++ {
		for y := int32(0); y < core.SectionsPerChunk; y++ {
			forgotten = append(forgotten, core.SectionKey{
				Dimension: core.Overworld, Pos: core.SectionPos{X: x, Y: y},
			})
		}
	}
	err := runtime.applyMeshUpdateLocked(client.MirrorUpdate{
		Dirty:     []core.SectionKey{invalidated},
		Forgotten: forgotten,
	})
	element := runtime.meshReady.byKey[survivor]
	readyLen := runtime.meshReady.len()
	gotRevision := runtime.sectionRevisions[survivor]
	_, invalidatedPresent := runtime.meshReady.byKey[invalidated]
	runtime.sessionMu.Unlock()
	if err != nil {
		t.Fatalf("mass-forget transition error = %v, want nil (drops are capacity-exempt)", err)
	}
	if runtime.Phase() == ConnectionPhaseDisconnected {
		t.Fatal("mass-forget transition terminated the session")
	}
	if element == nil || !element.Value.(meshReadyOperation).upsert || gotRevision != 1 {
		t.Fatalf("survivor pending upsert changed: element=%v revision=%d", element, gotRevision)
	}
	if readyLen != 1+3*core.SectionsPerChunk {
		t.Fatalf("post-transition queue length = %d, want survivor upsert plus forget drops", readyLen)
	}
	if invalidatedPresent {
		t.Fatal("dirty invalidation left the invalidated section's pending upsert queued")
	}
}

// TestMeshUpsertDrainRestoresSchedulingCapacity pins the upsert capacity
// accounting: draining published upserts must give the scheduling budget back,
// so meshing continues after the lifetime drain count passes the ready
// capacity. A leaking counter stalls the pipeline exactly like a full queue.
func TestMeshUpsertDrainRestoresSchedulingCapacity(t *testing.T) {
	const capacity = 4
	const upsertTargets = capacity + 2
	runtime := newMeshRuntimeForTest(t, capacity)
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for index := range sections {
		block := core.AirID
		// Alternate stone and air so every stone section stays mesh-visible:
		// each one has a loaded air section directly above, and absent
		// horizontal neighbors (encoded as barriers by the mesher) cannot
		// make the section mesh empty and publish as a drop instead.
		if index < upsertTargets*2 && index%2 == 0 {
			block = core.StoneID
		}
		sections[index] = network.SectionData{
			Y: int32(index), Storage: network.SectionSingle, Single: block,
		}
	}
	if _, err := runtime.ApplyMessage(network.ChunkSnapshot{
		Dimension: core.Overworld, Chunk: core.ChunkPos{}, Revision: 1, Sections: sections,
	}); err != nil {
		t.Fatal(err)
	}
	// Publish strictly more upserts than the ready capacity holds; drops for
	// the air sections may interleave and never consume budget.
	publishedUpserts := 0
	for publishedUpserts < upsertTargets {
		waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool {
			return stats.ReadyOperations >= 1
		})
		batch := drainRuntimeWorldBatch(t, runtime, 1)
		if len(batch.Upserts()) > 0 {
			publishedUpserts += len(batch.Upserts())
		}
		runtime.sessionMu.Lock()
		free := runtime.meshReady.free()
		runtime.sessionMu.Unlock()
		if publishedUpserts < upsertTargets && free < 1 {
			t.Fatalf("after %d upserts no scheduling budget left (free=%d): upsert accounting leaks",
				publishedUpserts, free)
		}
	}
}

// TestMeshWarpScaleForgetBurstAppliesWithoutReadyOverflow pins the drop-exempt
// capacity contract: a dimension-warp ForgetChunks burst expands every forgotten
// chunk to a full section column, far beyond the ready capacity, and payload-less
// drop operations must apply completely instead of terminating the session.
func TestMeshWarpScaleForgetBurstAppliesWithoutReadyOverflow(t *testing.T) {
	const burstChunks = 300
	runtime := newMeshRuntimeForTest(t, 64)
	chunks := make([]core.ChunkPos, 0, burstChunks)
	for x := range burstChunks {
		chunk := core.ChunkPos{X: int32(x%18) - 9, Z: int32(x/18) - 9}
		chunks = append(chunks, chunk)
		snapshot := meshSnapshot(1, core.StoneID)
		snapshot.Chunk = chunk
		if _, err := runtime.ApplyMessage(snapshot); err != nil {
			t.Fatal(err)
		}
	}
	// Pending upserts for two sections of the first loaded chunk must be
	// invalidated by the burst.
	targets := []core.SectionKey{
		{Dimension: core.Overworld, Pos: core.SectionPos{X: -9, Z: -9}},
		{Dimension: core.Overworld, Pos: core.SectionPos{X: -9, Y: 3, Z: -9}},
	}
	runtime.sessionMu.Lock()
	for _, key := range targets {
		runtime.sectionRevisions[key] = 1
		runtime.meshReady.put(meshReadyOperation{
			key: key, revision: 1, quads: []mesh.Quad{{}}, upsert: true, meshed: true,
		})
	}
	runtime.sessionMu.Unlock()

	if _, err := runtime.ApplyMessage(network.ForgetChunks{
		Dimension: core.Overworld, Chunks: chunks,
	}); err != nil {
		t.Fatalf("warp-scale ForgetChunks error = %v, want nil (drops are capacity-exempt)", err)
	}
	if runtime.Phase() == ConnectionPhaseDisconnected || runtime.Err() != nil {
		t.Fatalf("burst terminated the session: phase=%v err=%v", runtime.Phase(), runtime.Err())
	}
	staleTargets := 0
	dropCount := 0
	runtime.sessionMu.Lock()
	for _, key := range targets {
		if element := runtime.meshReady.byKey[key]; element != nil && element.Value.(meshReadyOperation).upsert {
			staleTargets++
		}
	}
	for element := runtime.meshReady.ordered.Front(); element != nil; element = element.Next() {
		if operation := element.Value.(meshReadyOperation); !operation.upsert {
			dropCount++
		}
	}
	runtime.sessionMu.Unlock()
	if staleTargets != 0 {
		t.Fatalf("burst left %d pending upserts for forgotten sections", staleTargets)
	}
	if dropCount != burstChunks*core.SectionsPerChunk {
		t.Fatalf("queued forget drops = %d, want %d", dropCount, burstChunks*core.SectionsPerChunk)
	}

	// The section drain publishes the whole payload-less drop backlog in one
	// bounded-upsert call so a following upsert is never stuck behind it.
	sections, ok, err := runtime.DrainMeshedSections(nil, 1)
	if err != nil || !ok {
		t.Fatalf("DrainMeshedSections() = (%v, %v), want the drop backlog", ok, err)
	}
	if len(sections) != burstChunks*core.SectionsPerChunk {
		t.Fatalf("drained sections = %d, want the full drop backlog %d", len(sections), burstChunks*core.SectionsPerChunk)
	}
	for _, section := range sections {
		if section.Upsert {
			t.Fatalf("drop backlog contained an upsert: %+v", section)
		}
	}

	// The session stays usable: a fresh section still meshes and publishes.
	snapshot := meshSnapshot(1, core.StoneID)
	snapshot.Chunk = core.ChunkPos{X: 40, Z: 0}
	if _, err := runtime.ApplyMessage(snapshot); err != nil {
		t.Fatal(err)
	}
	waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool { return stats.ReadyOperations == 1 })
	batch := drainRuntimeWorldBatch(t, runtime, 1)
	upserts := batch.Upserts()
	wantKey := core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: 40}}
	if len(upserts) != 1 || upserts[0].Key != wantKey || upserts[0].Payload.Len() == 0 {
		t.Fatalf("post-burst upserts = %+v, want the fresh section publication at %+v", upserts, wantKey)
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
