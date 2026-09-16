package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

func adoptedSessionForTest(t *testing.T) (*Runtime, AdoptedSession) {
	t.Helper()
	session := AdoptedSession{
		Receiver:      &messageTestReceiver{},
		World:         client.NewMirror(),
		Predictor:     client.NewPredictor(),
		Inventory:     &client.InventoryMirror{},
		Crafting:      &client.CraftingMirror{},
		Chest:         &client.ChestMirror{},
		Furnace:       &client.FurnaceMirror{},
		Chat:          &client.ChatEvents{},
		ItemDrops:     client.NewItemDrops(),
		RemotePlayers: client.NewRemotePlayers(),
		Companions:    &client.Companions{},
		Hostiles:      &client.Hostiles{},
		Passives:      &client.Passives{},
		Projectiles:   &client.Projectiles{},
	}
	runtime, err := Adopt(session)
	if err != nil {
		t.Fatal(err)
	}
	return runtime, session
}

func TestAdoptedSessionDrainsIntoHostOwnedMirrors(t *testing.T) {
	runtime, session := adoptedSessionForTest(t)
	spawnID := core.PlayerID{0: 0x12, 6: 0x40, 8: 0x80, 15: 1}
	receiver := &messageTestReceiver{messages: []network.ServerMessage{
		network.InventoryState{},
		network.RemotePlayerSpawn{PlayerID: spawnID, DisplayName: "Host", ServerTick: 1},
	}}
	runtime.receiver = receiver

	outcomes, err := runtime.DrainMessages(nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcomes) != 2 {
		t.Fatalf("adopted drain outcomes = %d, want 2", len(outcomes))
	}
	// The host-owned mirror objects, not runtime-allocated copies, must carry the
	// applied state; this is the seam the legacy presentation host depends on.
	if _, confirmed := session.Inventory.State(); !confirmed {
		t.Fatal("adopted drain left the host inventory mirror unconfirmed")
	}
	if presentations := session.RemotePlayers.AppendPresentations(nil); len(presentations) != 1 {
		t.Fatalf("adopted drain remote presentations = %d, want 1", len(presentations))
	}
}

func TestAdoptedSessionSeedsPlayerTickGuard(t *testing.T) {
	runtime, session := adoptedSessionForTest(t)
	session.PlayerTick = 9
	adopted, err := Adopt(session)
	if err != nil {
		t.Fatal(err)
	}
	stale := network.PlayerState{ServerTick: 9, Dimension: core.Overworld, Ready: true, Reset: true}
	outcome, err := adopted.ApplyMessage(stale)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.PredictionChanged {
		t.Fatal("seeded player tick accepted a stale authoritative state")
	}
	fresh := stale
	fresh.ServerTick = 10
	outcome, err = adopted.ApplyMessage(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.PredictionChanged {
		t.Fatal("adopted session rejected a newer authoritative state")
	}
	_ = runtime
}

func TestAdoptedSessionSharesSequenceSpaceWithHostCommands(t *testing.T) {
	_, session := adoptedSessionForTest(t)
	session.Sequence = 41
	adopted, err := Adopt(session)
	if err != nil {
		t.Fatal(err)
	}
	if got := adopted.Sequence(); got != 41 {
		t.Fatalf("adopted sequence seed = %d, want 41", got)
	}
	if got := adopted.NextSequence(); got != 42 {
		t.Fatalf("host command sequence = %d, want 42", got)
	}
	if got := adopted.Sequence(); got != 42 {
		t.Fatalf("sequence after host command = %d, want 42", got)
	}
}

func TestAdoptRejectsMissingCoreSessionObjects(t *testing.T) {
	if _, err := Adopt(AdoptedSession{Predictor: client.NewPredictor()}); err == nil {
		t.Fatal("adoption without a world mirror was accepted")
	}
	if _, err := Adopt(AdoptedSession{World: client.NewMirror()}); err == nil {
		t.Fatal("adoption without a predictor was accepted")
	}
}

func TestAdoptMeshingPublishesSectionsWithConnectivity(t *testing.T) {
	runtime, _ := adoptedSessionForTest(t)
	hostMesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(hostMesher.Close)
	if err := runtime.AdoptMeshing(hostMesher, 32); err != nil {
		t.Fatal(err)
	}
	// A mixed section (one stone block in air) yields both mesh quads and a
	// nonzero flood-fill connectivity, so the drain can prove it carries both.
	if _, err := runtime.ApplyMessage(mixedCornerStoneSnapshot()); err != nil {
		t.Fatal(err)
	}

	waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool {
		return stats.ReadyOperations == 2
	})
	sections, ok, err := runtime.DrainMeshedSections(nil, 2)
	if err != nil || !ok {
		t.Fatalf("DrainMeshedSections() = (%v, %v), want sections", ok, err)
	}
	if len(sections) != 2 {
		t.Fatalf("drained sections = %d, want 2", len(sections))
	}
	section := sections[0]
	wantKey := core.SectionPos{}
	if section.Dimension != core.Overworld || section.Pos != wantKey || section.Revision != 1 || !section.Upsert {
		t.Fatalf("drained section = %+v, want overworld %v revision 1 upsert", section, wantKey)
	}
	if len(section.Quads) == 0 {
		t.Fatal("drained section lost its mesh quads")
	}
	if section.Conn == 0 || !section.ConnectivityKnown {
		t.Fatal("drained section lost its connectivity")
	}
	// The all-air neighbour meshes empty: it is a drop that must still carry
	// known connectivity so a legacy host can register visibility traversal.
	empty := sections[1]
	if empty.Upsert || len(empty.Quads) != 0 {
		t.Fatalf("all-air section = %+v, want an empty-mesh drop", empty)
	}
	if !empty.ConnectivityKnown || empty.Conn == 0 {
		t.Fatalf("empty-mesh drop connectivity = known:%v value:%d, want known nonzero", empty.ConnectivityKnown, empty.Conn)
	}
}

// `mixedCornerStoneSnapshot` builds a chunk whose first section is air except
// for one stone block in the corner, producing both quads and connectivity.
func mixedCornerStoneSnapshot() network.ChunkSnapshot {
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for index := range sections {
		sections[index] = network.SectionData{
			Y: int32(index), Storage: network.SectionSingle, Single: core.AirID,
		}
	}
	packed := make([]uint64, core.BlocksPerSection*4/64)
	packed[0] = 1
	sections[0] = network.SectionData{
		Y: 0, Storage: network.SectionIndexed, Bits: 4,
		Palette: []core.BlockID{core.AirID, core.StoneID}, Packed: packed,
	}
	return network.ChunkSnapshot{
		Dimension: core.Overworld, Chunk: core.ChunkPos{}, Revision: 1, Sections: sections,
	}
}

func TestAdoptMeshingKeepsHostMesherOwnershipAndDirtyState(t *testing.T) {
	runtime, _ := adoptedSessionForTest(t)
	hostMesher := client.NewMesher(assets.NewRegistry(), 1)
	t.Cleanup(hostMesher.Close)
	if _, err := runtime.ApplyMessage(meshSnapshot(1, core.StoneID)); err != nil {
		t.Fatal(err)
	}
	// The host marks its own mesher dirty before adoption; the adopted pipeline
	// must consume that pre-existing dirty state instead of starting empty.
	hostMesher.MarkDirty(core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{}})
	if err := runtime.AdoptMeshing(hostMesher, 32); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	// Closing the adopted runtime must not close the host-owned mesher: one more
	// adoption cycle can still schedule and drain the surviving dirty section.
	runtime2, err := Adopt(AdoptedSession{World: client.NewMirror(), Predictor: client.NewPredictor()})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime2.AdoptMeshing(hostMesher, 32); err != nil {
		t.Fatal(err)
	}
	if stats := runtime2.MeshStats(); stats.DirtySections != 1 {
		t.Fatalf("host dirty sections after close and re-adoption = %d, want 1", stats.DirtySections)
	}
}

func TestDrainMeshedSectionsPreservesWorldBatchPath(t *testing.T) {
	runtime := newMeshRuntimeForTest(t, 32)
	applyMeshSnapshot(t, runtime, 1)
	waitForRuntimeMesh(t, runtime, func(stats MeshStats) bool {
		return stats.ReadyOperations == 1
	})
	if _, ok, err := runtime.DrainMeshedSections(nil, 0); err != nil || ok {
		t.Fatalf("zero section budget = (%v, %v), want no sections", ok, err)
	}
	if _, _, err := runtime.DrainMeshedSections(nil, MaxMeshReadyCapacity+1); err == nil {
		t.Fatal("over-limit section budget was accepted")
	}
	batch := drainRuntimeWorldBatch(t, runtime, 1)
	upserts := batch.Upserts()
	if len(upserts) != 1 || upserts[0].Payload.Len() == 0 {
		t.Fatalf("world batch after lazy packing = %+v, want one packed upsert", upserts)
	}
}

func TestAdoptedSessionPredictionAdvancesHostPredictor(t *testing.T) {
	_, session := adoptedSessionForTest(t)
	clientEndpoint, serverEndpoint := network.NewMemoryPair(8)
	t.Cleanup(func() { _ = clientEndpoint.Close() })
	t.Cleanup(func() { _ = serverEndpoint.Close() })
	session.Sender = clientEndpoint
	if err := session.Predictor.Begin(network.PlayerState{
		ServerTick: 1, Dimension: core.Overworld,
		Position: mgl32.Vec3{0.5, 64, 0.5}, OnGround: true, Ready: true,
	}); err != nil {
		t.Fatal(err)
	}
	adopted, err := Adopt(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := adopted.SubmitInput(SemanticInput{MoveX: 1}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	snapshot, err := adopted.AdvancePrediction(ctx, physics.FixedDelta)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Ready {
		t.Fatal("adopted prediction snapshot is not ready")
	}
	if snapshot.State.Position == (mgl32.Vec3{0.5, 64, 0.5}) {
		t.Fatal("adopted prediction did not advance the host predictor")
	}
	hostState, ready := session.Predictor.State()
	if !ready || hostState.Position != snapshot.State.Position {
		t.Fatalf("host predictor position = %+v, want the advanced adopted position", hostState.Position)
	}
	if got := adopted.Sequence(); got != 1 {
		t.Fatalf("adopted input sequence = %d, want 1", got)
	}
}
