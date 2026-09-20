package runtime

import (
	"container/list"
	"errors"
	"fmt"
	"math"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
)

var (
	// `ErrMeshReadyOverflow` is a terminal session error: continuing would silently lose a section
	// replacement or drop required to converge the presentation host with the authoritative mirror.
	ErrMeshReadyOverflow = errors.New("runtime: mesh ready queue overflow")
	errMeshPipeline      = errors.New("runtime: mesh pipeline failure")
)

// `MeshOptions` configures the existing client mesher as an optional runtime-owned subsystem.
// The registry becomes immutable shared input after configuration; mesh workers never mutate it.
type MeshOptions struct {
	Registry      *assets.Registry
	Workers       int
	ReadyCapacity int
	AtlasRevision presentation.AtlasRevision
}

// `Validate` rejects mesh ownership that could publish an unbounded or unidentified world batch.
func (options MeshOptions) Validate() error {
	if options.Registry == nil {
		return errors.New("runtime: mesh registry is required")
	}
	if options.Workers < 1 {
		return errors.New("runtime: mesh worker count must be positive")
	}
	if options.ReadyCapacity < 1 || options.ReadyCapacity > presentation.MaxWorldBatchOperations {
		return fmt.Errorf(
			"runtime: mesh ready capacity %d is outside 1..%d",
			options.ReadyCapacity,
			presentation.MaxWorldBatchOperations,
		)
	}
	if options.AtlasRevision == 0 {
		return errors.New("runtime: mesh atlas revision must be positive")
	}
	return nil
}

// `MeshStats` combines worker backpressure with the bounded host-publication queue identity.
type MeshStats struct {
	DirtySections   int
	QueuedJobs      int
	InFlightJobs    int
	ReadyResults    int
	ResultCapacity  int
	CompletedMeshes uint64
	ReadyOperations int
	ReadyCapacity   int
	Epoch           uint64
}

type meshReadyOperation struct {
	key      core.SectionKey
	revision uint64
	// `quads` and `conn` keep the raw mesher result. Packing into the presentation
	// payload happens lazily in `DrainWorldBatch`, so the legacy host-facing
	// section drain publishes the same result without a pack/unpack round trip.
	quads []mesh.Quad
	conn  mesh.Connectivity
	// `upsert` marks a section replacement with quads; `meshed` marks any
	// operation derived from a mesher result. Empty meshes are drops that still
	// carry valid connectivity, while world-forget removals know none.
	upsert bool
	meshed bool
}

// `meshReadyQueue` coalesces by section without scanning unrelated pending work. Its linked-list
// order is deterministic publication order; the index makes invalidation and replacement O(1).
// Capacity accounting counts only payload-bearing upsert operations: drop operations carry no
// packed mesh bytes, so a world-forget burst of any size neither needs ready space nor can push
// pending upserts out of their no-loss publication guarantee.
type meshReadyQueue struct {
	capacity int
	upserts  int
	ordered  list.List
	byKey    map[core.SectionKey]*list.Element
}

func newMeshReadyQueue(capacity int) meshReadyQueue {
	return meshReadyQueue{capacity: capacity, byKey: make(map[core.SectionKey]*list.Element, capacity)}
}

func (queue *meshReadyQueue) len() int {
	return queue.ordered.Len()
}

// `free` reports the remaining upsert publication space. A backlog of payload-less
// drops never blocks mesh scheduling.
func (queue *meshReadyQueue) free() int {
	return queue.capacity - queue.upserts
}

// `removeElement` unlinks one queued operation while keeping the upsert
// capacity accounting exact. Every removal path must go through it.
func (queue *meshReadyQueue) removeElement(element *list.Element) {
	operation := element.Value.(meshReadyOperation)
	if operation.upsert {
		queue.upserts--
	}
	queue.ordered.Remove(element)
	delete(queue.byKey, operation.key)
}

func (queue *meshReadyQueue) remove(key core.SectionKey) {
	element := queue.byKey[key]
	if element == nil {
		return
	}
	queue.removeElement(element)
}

func (queue *meshReadyQueue) clear() {
	queue.ordered.Init()
	clear(queue.byKey)
	queue.upserts = 0
}

// `canUpsertTransition` guards the no-loss upsert publication contract: replacing the
// operations registered for `remove` and `put` keys must keep the payload-bearing upsert
// population within capacity. World-message transitions insert only drop operations, so
// the guard protects upserts while remaining silent for drop-only forget bursts.
func (queue *meshReadyQueue) canUpsertTransition(remove, put []core.SectionKey) bool {
	removedUpserts := make(map[core.SectionKey]struct{}, len(remove)+len(put))
	for _, keys := range [][]core.SectionKey{remove, put} {
		for _, key := range keys {
			element := queue.byKey[key]
			if element == nil {
				continue
			}
			if element.Value.(meshReadyOperation).upsert {
				removedUpserts[key] = struct{}{}
			}
		}
	}
	return queue.upserts-len(removedUpserts) <= queue.capacity
}

func (queue *meshReadyQueue) put(operation meshReadyOperation) {
	if element := queue.byKey[operation.key]; element != nil {
		previous := element.Value.(meshReadyOperation)
		element.Value = operation
		queue.ordered.MoveToBack(element)
		switch {
		case operation.upsert && !previous.upsert:
			queue.upserts++
		case !operation.upsert && previous.upsert:
			queue.upserts--
		}
		return
	}
	if operation.upsert {
		queue.upserts++
	}
	queue.byKey[operation.key] = queue.ordered.PushBack(operation)
}

// `ConfigureMeshing` transfers worker lifecycle and bounded publication ownership to `runtime`.
// It must run before the host starts draining server messages so no loaded section misses dirtiness.
func (runtime *Runtime) ConfigureMeshing(options MeshOptions) error {
	if runtime == nil {
		return errors.New("runtime: nil runtime")
	}
	if err := options.Validate(); err != nil {
		return err
	}
	mesher := client.NewMesher(options.Registry, options.Workers)
	runtime.sessionMu.Lock()
	if runtime.sessionClosed {
		runtime.sessionMu.Unlock()
		mesher.Close()
		return errors.New("runtime: client session is closed")
	}
	if runtime.mesher != nil {
		runtime.sessionMu.Unlock()
		mesher.Close()
		return errors.New("runtime: meshing is already configured")
	}
	runtime.meshOptions = options
	runtime.mesher = mesher
	runtime.ownsMesher = true
	runtime.meshReady = newMeshReadyQueue(options.ReadyCapacity)
	runtime.meshEpoch = 1
	runtime.sectionRevisions = make(map[core.SectionKey]uint64)
	runtime.meshChunks = make(map[core.ChunkKey]struct{})
	runtime.sessionMu.Unlock()
	return nil
}

// `AdvanceMeshes` performs at most `budget` scheduling attempts and accepted-result drains.
// It never waits for a worker. A publication queue full of payload-bearing upserts applies
// backpressure by leaving both the existing mesher result queue and the remaining dirty work
// untouched for a later host drain; payload-less drop backlogs never block scheduling.
func (runtime *Runtime) AdvanceMeshes(budget int) error {
	if runtime == nil {
		return errors.New("runtime: nil runtime")
	}
	if budget < 0 {
		return errors.New("runtime: negative mesh budget")
	}
	if budget == 0 {
		return nil
	}

	runtime.sessionMu.Lock()
	if runtime.sessionClosed {
		runtime.sessionMu.Unlock()
		return errors.New("runtime: client session is closed")
	}
	if runtime.mesher == nil || runtime.mirrors == nil || runtime.mirrors.world == nil {
		runtime.sessionMu.Unlock()
		return nil
	}
	free := runtime.meshReady.free()
	if free <= 0 {
		runtime.sessionMu.Unlock()
		return nil
	}
	work := min(budget, free)
	center := client.ViewCenter{Dimension: core.Overworld}
	if runtime.predictor != nil {
		if feet, ready := runtime.predictor.PresentationPosition(0); ready {
			center.Chunk = core.BlockPos{
				X: int32(math.Floor(float64(feet.X()))),
				Z: int32(math.Floor(float64(feet.Z()))),
			}.Chunk()
		}
	}
	runtime.mesher.Schedule(runtime.mirrors.world, center, work)
	results := runtime.mesher.Drain(runtime.mirrors.world, work)
	operations := make([]meshReadyOperation, 0, len(results))
	for _, result := range results {
		operation, err := runtime.meshOperationLocked(result)
		if err != nil {
			runtime.sessionMu.Unlock()
			runtime.stopForMeshFailure(err)
			return err
		}
		operations = append(operations, operation)
	}
	for _, operation := range operations {
		runtime.meshReady.put(operation)
	}
	runtime.sessionMu.Unlock()
	return nil
}

func (runtime *Runtime) meshOperationLocked(result client.MeshedSection) (meshReadyOperation, error) {
	key := core.SectionKey{Dimension: result.Dimension, Pos: result.Pos}
	revision, err := runtime.nextSectionRevisionLocked(key)
	if err != nil {
		return meshReadyOperation{}, err
	}
	if len(result.Quads) == 0 {
		return meshReadyOperation{key: key, revision: revision, conn: result.Conn, meshed: true}, nil
	}
	return meshReadyOperation{
		key: key, revision: revision, quads: result.Quads, conn: result.Conn, upsert: true, meshed: true,
	}, nil
}

func packMeshedQuads(quads []mesh.Quad) (packed []uint64, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			packed = nil
			err = fmt.Errorf("invalid mesher quad: %v", recovered)
		}
	}()
	packed = make([]uint64, len(quads))
	for index, quad := range quads {
		packed[index] = quad.Pack()
	}
	return packed, nil
}

// `DrainWorldBatch` consumes at most `budget` ready section operations as one owned atomic batch.
// Payload bytes are also bounded, so a large ready queue is split without truncating an operation.
func (runtime *Runtime) DrainWorldBatch(budget int) (presentation.WorldBatch, bool, error) {
	if runtime == nil {
		return presentation.WorldBatch{}, false, errors.New("runtime: nil runtime")
	}
	if budget < 0 || budget > presentation.MaxWorldBatchOperations {
		return presentation.WorldBatch{}, false, errors.New("runtime: invalid world batch budget")
	}
	if budget == 0 {
		return presentation.WorldBatch{}, false, nil
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	if runtime.sessionClosed {
		return presentation.WorldBatch{}, false, errors.New("runtime: client session is closed")
	}
	if runtime.mesher == nil || runtime.meshReady.len() == 0 {
		return presentation.WorldBatch{}, false, nil
	}

	upserts := make([]presentation.SectionMeshUpsert, 0, min(budget, runtime.meshReady.len()))
	drops := make([]presentation.SectionDrop, 0, min(budget, runtime.meshReady.len()))
	selected := make([]*list.Element, 0, min(budget, runtime.meshReady.len()))
	packedQuads := 0
	for element := runtime.meshReady.ordered.Front(); element != nil && len(selected) < budget; element = element.Next() {
		operation := element.Value.(meshReadyOperation)
		if operation.upsert {
			if packedQuads+len(operation.quads) > presentation.MaxWorldBatchPackedQuads {
				break
			}
			// Packing is deferred to this publication point so the host-facing
			// section drain can share one queue without paying pack/unpack copies.
			packed, err := packMeshedQuads(operation.quads)
			if err != nil {
				return presentation.WorldBatch{}, false, fmt.Errorf(
					"runtime: pack section mesh %v: %w", operation.key, err)
			}
			payload, err := presentation.NewSectionMeshPayload(packed)
			if err != nil {
				return presentation.WorldBatch{}, false, fmt.Errorf(
					"runtime: encode section mesh %v: %w", operation.key, err)
			}
			packedQuads += len(operation.quads)
			upserts = append(upserts, presentation.SectionMeshUpsert{
				Key: operation.key, Revision: operation.revision, Payload: payload,
			})
		} else {
			drops = append(drops, presentation.SectionDrop{Key: operation.key, Revision: operation.revision})
		}
		selected = append(selected, element)
	}
	if len(selected) == 0 {
		return presentation.WorldBatch{}, false, errors.New("runtime: ready mesh exceeds world batch capacity")
	}
	batch, err := presentation.NewWorldBatch(
		runtime.meshEpoch,
		runtime.meshOptions.AtlasRevision,
		upserts,
		drops,
	)
	if err != nil {
		return presentation.WorldBatch{}, false, fmt.Errorf("runtime: build world batch: %w", err)
	}
	for _, element := range selected {
		runtime.meshReady.removeElement(element)
	}
	return batch, true, nil
}

// `MeshStats` returns a bounded diagnostic snapshot without exposing worker or queue ownership.
func (runtime *Runtime) MeshStats() MeshStats {
	if runtime == nil {
		return MeshStats{}
	}
	runtime.sessionMu.Lock()
	defer runtime.sessionMu.Unlock()
	stats := MeshStats{Epoch: runtime.meshEpoch}
	if runtime.mesher != nil {
		mesherStats := runtime.mesher.Stats()
		stats.DirtySections = mesherStats.DirtySections
		stats.QueuedJobs = mesherStats.QueuedJobs
		stats.InFlightJobs = mesherStats.InFlightJobs
		stats.ReadyResults = mesherStats.ReadyResults
		stats.ResultCapacity = mesherStats.ResultCapacity
		stats.CompletedMeshes = mesherStats.CompletedMeshes
		stats.ReadyOperations = runtime.meshReady.len()
		stats.ReadyCapacity = runtime.meshReady.capacity
	}
	return stats
}

func (runtime *Runtime) applyMeshUpdateLocked(update client.MirrorUpdate) error {
	if runtime.mesher == nil {
		return nil
	}
	forgottenChunks := make(map[core.ChunkKey]struct{}, len(update.Forgotten)/core.SectionsPerChunk)
	keys := make([]core.SectionKey, 0, len(update.Forgotten))
	for _, key := range update.Forgotten {
		keys = append(keys, key)
		forgottenChunks[core.ChunkKey{
			Dimension: key.Dimension,
			Pos:       core.ChunkPos{X: key.Pos.X, Z: key.Pos.Z},
		}] = struct{}{}
	}
	if !runtime.meshReady.canUpsertTransition(update.Dirty, keys) {
		return fmt.Errorf("%w: %w", errMeshPipeline, ErrMeshReadyOverflow)
	}
	for _, key := range keys {
		if runtime.sectionRevisions[key] == ^uint64(0) {
			return fmt.Errorf("%w: section revision overflow for %v", errMeshPipeline, key)
		}
	}

	for _, key := range update.Dirty {
		runtime.meshReady.remove(key)
		chunk := core.ChunkKey{
			Dimension: key.Dimension,
			Pos:       core.ChunkPos{X: key.Pos.X, Z: key.Pos.Z},
		}
		if _, loaded := runtime.mirrors.world.Chunk(chunk.Dimension, chunk.Pos); loaded {
			runtime.meshChunks[chunk] = struct{}{}
		}
	}
	runtime.mesher.MarkDirty(update.Dirty...)
	for _, key := range keys {
		runtime.meshReady.remove(key)
	}
	for chunk := range forgottenChunks {
		runtime.mesher.ForgetChunk(chunk.Dimension, chunk.Pos)
		delete(runtime.meshChunks, chunk)
	}
	for _, key := range keys {
		revision, err := runtime.nextSectionRevisionLocked(key)
		if err != nil {
			return fmt.Errorf("%w: %w", errMeshPipeline, err)
		}
		runtime.meshReady.put(meshReadyOperation{key: key, revision: revision})
	}
	return nil
}

func (runtime *Runtime) nextSectionRevisionLocked(key core.SectionKey) (uint64, error) {
	revision := runtime.sectionRevisions[key] + 1
	if revision == 0 {
		return 0, fmt.Errorf("runtime: section revision overflow for %v", key)
	}
	runtime.sectionRevisions[key] = revision
	return revision, nil
}

func (runtime *Runtime) resetMeshingLocked(closeMesher bool) *client.Mesher {
	old := runtime.mesher
	if old == nil {
		return nil
	}
	runtime.meshReady.clear()
	clear(runtime.sectionRevisions)
	runtime.meshEpoch++
	if runtime.meshEpoch == 0 {
		runtime.meshEpoch++
	}
	// Forgetting tracked chunks invalidates queued and in-flight generations without waiting for a
	// worker. An old result can still reach the bounded mesher result channel, but `Drain` rejects it
	// because no matching dirty generation survives this epoch boundary.
	for chunk := range runtime.meshChunks {
		old.ForgetChunk(chunk.Dimension, chunk.Pos)
	}
	clear(runtime.meshChunks)
	if closeMesher {
		runtime.mesher = nil
		// An adopted mesher belongs to the host: reset drops runtime publication
		// state but never closes the host's worker pool.
		if !runtime.ownsMesher {
			return nil
		}
		return old
	}
	return nil
}

func (runtime *Runtime) stopForMeshFailure(err error) {
	runtime.terminalMu.Lock()
	if runtime.terminalErr == nil {
		runtime.terminalErr = err
	}
	runtime.terminalMu.Unlock()
	_ = runtime.Close()
}
