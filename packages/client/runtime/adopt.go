package runtime

import (
	"errors"
	"fmt"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// `MaxMeshReadyCapacity` re-exports the presentation publication bound so a host
// adopting meshing selects its ready-queue capacity against one contract value.
const MaxMeshReadyCapacity = presentation.MaxWorldBatchOperations

// `AdoptedSession` names the host-owned session objects that one adopted runtime
// mutates in place. Adoption exists for the legacy presentation host during the
// staged runtime migration: that host keeps constructing, resetting, and
// replacing its mirror, predictor, and receiver objects, so the adopted runtime
// applies protocol message routing, prediction, and mesh scheduling to those
// exact objects instead of a parallel state copy. `Receiver` and `Sender` are
// used but never owned or closed by the adopted runtime; endpoint, receiver, and
// mesher lifecycles remain with the host. `PlayerTick` and `Sequence` seed the
// session-wide stale-state guard and protocol sequence space so an adoption
// replacing a previous runtime continues both without resetting them.
type AdoptedSession struct {
	Receiver      Receiver
	Sender        network.ClientEndpoint
	World         *client.Mirror
	Predictor     *client.Predictor
	Inventory     *client.InventoryMirror
	Crafting      *client.CraftingMirror
	Chest         *client.ChestMirror
	Furnace       *client.FurnaceMirror
	Chat          *client.ChatEvents
	ItemDrops     *client.ItemDrops
	RemotePlayers *client.RemotePlayers
	Companions    *client.Companions
	Hostiles      *client.Hostiles
	Passives      *client.Passives
	Projectiles   *client.Projectiles
	PlayerTick    uint64
	Sequence      uint64
}

// `Adopt` builds a runtime around host-owned session objects. The returned
// runtime starts in the loading phase of a logged-in session and owns no
// resources, so discarding it without `Close` leaks nothing; hosts that do call
// `Close` release only runtime-internal state and never the adopted receiver,
// sender, or mesher. Nil entity mirrors are preserved as-is so the drain
// observes exactly the behavior the host's own routing would produce.
func Adopt(session AdoptedSession) (*Runtime, error) {
	if session.World == nil {
		return nil, errors.New("runtime: adopted session world mirror is required")
	}
	if session.Predictor == nil {
		return nil, errors.New("runtime: adopted session predictor is required")
	}
	if isNilInterface(session.Receiver) {
		session.Receiver = nil
	}
	if isNilInterface(session.Sender) {
		session.Sender = nil
	}
	return &Runtime{
		phase:            ConnectionPhaseLoading,
		mirrors:          adoptSessionMirrors(session),
		predictor:        session.Predictor,
		sequence:         session.Sequence,
		playerTick:       session.PlayerTick,
		receiver:         session.Receiver,
		sender:           session.Sender,
		cameraProjection: DefaultCameraProjection(),
	}, nil
}

func adoptSessionMirrors(session AdoptedSession) *sessionMirrors {
	return &sessionMirrors{
		world:         session.World,
		inventory:     session.Inventory,
		crafting:      session.Crafting,
		chest:         session.Chest,
		furnace:       session.Furnace,
		chat:          session.Chat,
		itemDrops:     session.ItemDrops,
		remotePlayers: session.RemotePlayers,
		companions:    session.Companions,
		hostiles:      session.Hostiles,
		passives:      session.Passives,
		projectiles:   session.Projectiles,
	}
}

// `NextSequence` allocates the next value of the single session-wide protocol
// sequence space shared by runtime-owned player input, chunk resyncs, and
// host-originated commands. Hosts that send protocol commands directly keep
// their numbering coherent with runtime-owned sends through this method.
func (runtime *Runtime) NextSequence() uint64 {
	if runtime == nil {
		return 0
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	return runtime.nextSequenceLocked()
}

// `Sequence` returns the last allocated sequence value without allocating.
func (runtime *Runtime) Sequence() uint64 {
	if runtime == nil {
		return 0
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	return runtime.sequence
}

// `AdoptMeshing` transfers scheduling and bounded-publication ownership of an
// existing host-built mesher to the runtime. Unlike `ConfigureMeshing`, the
// mesher keeps the registry, worker pool, and dirty state exactly as the host
// built them; the runtime only takes the ready queue, per-section revisions,
// and epoch identity. The ready-capacity argument follows the same bounds as
// `MeshOptions.ReadyCapacity`. The host keeps mesher ownership: closing the
// runtime never closes an adopted mesher.
func (runtime *Runtime) AdoptMeshing(mesher *client.Mesher, readyCapacity int) error {
	if runtime == nil {
		return errors.New("runtime: nil runtime")
	}
	if mesher == nil {
		return errors.New("runtime: adopted mesher is required")
	}
	if readyCapacity < 1 || readyCapacity > presentation.MaxWorldBatchOperations {
		return fmt.Errorf(
			"runtime: mesh ready capacity %d is outside 1..%d",
			readyCapacity,
			presentation.MaxWorldBatchOperations,
		)
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return errors.New("runtime: client session is closed")
	}
	if runtime.mesher != nil {
		return errors.New("runtime: meshing is already configured")
	}
	// The atlas revision only names `WorldBatch` identity; an adopted pipeline
	// publishes through the host-facing section drain first, so a neutral
	// positive value keeps the batch path valid without a host-supplied atlas.
	runtime.meshOptions = MeshOptions{AtlasRevision: presentation.AtlasRevision(1), ReadyCapacity: readyCapacity}
	runtime.mesher = mesher
	runtime.ownsMesher = false
	runtime.meshReady = newMeshReadyQueue(readyCapacity)
	runtime.meshEpoch = 1
	runtime.sectionRevisions = make(map[core.SectionKey]uint64)
	runtime.meshChunks = make(map[core.ChunkKey]struct{})
	return nil
}

// `MeshSectionResult` is one drained ready-queue operation in the legacy
// host-friendly shape: raw quads plus section connectivity. It serves hosts that
// upload through the existing section scheduler, while the packed `WorldBatch`
// path serves new presentation hosts; both drains consume the same
// runtime-owned ready queue, budgets, revisions, and epochs, and each operation
// leaves the queue through exactly one drain.
type MeshSectionResult struct {
	Dimension core.DimensionID
	Pos       core.SectionPos
	Quads     []mesh.Quad
	Conn      mesh.Connectivity
	Revision  uint64
	// `Upsert` distinguishes a section replacement from a section drop; drops
	// carry no quads.
	Upsert bool
	// `ConnectivityKnown` reports whether this operation carries mesher
	// connectivity. Empty meshes are drops whose connectivity still must be
	// registered for visibility traversal; world-forget removals know none, and
	// hosts must leave any earlier registration untouched for them.
	ConnectivityKnown bool
}

// `DrainMeshedSections` consumes at most `budget` payload-bearing upsert
// operations in publication order. Payload-less drop operations encountered in
// that span drain without consuming budget: they carry no mesh bytes, and the
// legacy host applies world-forget drops at message time, so a warp-scale drop
// backlog never delays fresh section uploads. Total work per call stays bounded
// by the loaded-section count. Budgets beyond `MaxMeshReadyCapacity` are
// rejected.
func (runtime *Runtime) DrainMeshedSections(dst []MeshSectionResult, budget int) ([]MeshSectionResult, bool, error) {
	if runtime == nil {
		return dst, false, errors.New("runtime: nil runtime")
	}
	if budget < 0 || budget > presentation.MaxWorldBatchOperations {
		return dst, false, errors.New("runtime: invalid meshed section budget")
	}
	if budget == 0 {
		return dst, false, nil
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return dst, false, errors.New("runtime: client session is closed")
	}
	if runtime.mesher == nil || runtime.meshReady.len() == 0 {
		return dst, false, nil
	}
	selected := 0
	upserts := 0
	for element := runtime.meshReady.ordered.Front(); element != nil && upserts < budget; element = element.Next() {
		operation := element.Value.(meshReadyOperation)
		dst = append(dst, MeshSectionResult{
			Dimension:         operation.key.Dimension,
			Pos:               operation.key.Pos,
			Quads:             operation.quads,
			Conn:              operation.conn,
			Revision:          operation.revision,
			Upsert:            operation.upsert,
			ConnectivityKnown: operation.meshed,
		})
		selected++
		if operation.upsert {
			upserts++
		}
	}
	if selected == 0 {
		return dst, false, nil
	}
	runtime.removeSelectedMeshOperations(selected)
	return dst, true, nil
}

// `removeSelectedMeshOperations` removes the first `count` queue entries that
// the caller just published. The caller publishes in queue order without
// dropping the lock in between, so front-removal stays exact.
func (runtime *Runtime) removeSelectedMeshOperations(count int) {
	for range count {
		element := runtime.meshReady.ordered.Front()
		if element == nil {
			return
		}
		runtime.meshReady.removeElement(element)
	}
}
