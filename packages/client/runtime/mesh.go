package runtime

import (
	"container/list"
	"errors"
	"fmt"

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
	payload  presentation.SectionMeshPayload
	upsert   bool
}

// `meshReadyQueue` coalesces by section without scanning unrelated pending work. Its linked-list
// order is deterministic publication order; the index makes invalidation and replacement O(1).
type meshReadyQueue struct {
	capacity int
	ordered  list.List
	byKey    map[core.SectionKey]*list.Element
}

func newMeshReadyQueue(capacity int) meshReadyQueue {
	return meshReadyQueue{capacity: capacity, byKey: make(map[core.SectionKey]*list.Element, capacity)}
}

func (queue *meshReadyQueue) len() int {
	return queue.ordered.Len()
}

func (queue *meshReadyQueue) free() int {
	return queue.capacity - queue.len()
}

func (queue *meshReadyQueue) remove(key core.SectionKey) {
	element := queue.byKey[key]
	if element == nil {
		return
	}
	queue.ordered.Remove(element)
	delete(queue.byKey, key)
}

func (queue *meshReadyQueue) clear() {
	queue.ordered.Init()
	clear(queue.byKey)
}

func (queue *meshReadyQueue) canTransition(remove, put []core.SectionKey) bool {
	removed := make(map[core.SectionKey]struct{}, len(remove)+len(put))
	for _, key := range remove {
		if queue.byKey[key] != nil {
			removed[key] = struct{}{}
		}
	}
	for _, key := range put {
		if queue.byKey[key] != nil {
			removed[key] = struct{}{}
		}
	}
	inserted := make(map[core.SectionKey]struct{}, len(put))
	for _, key := range put {
		inserted[key] = struct{}{}
	}
	return queue.len()-len(removed)+len(inserted) <= queue.capacity
}

func (queue *meshReadyQueue) put(operation meshReadyOperation) {
	if element := queue.byKey[operation.key]; element != nil {
		element.Value = operation
		queue.ordered.MoveToBack(element)
		return
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
	runtime.meshReady = newMeshReadyQueue(options.ReadyCapacity)
	runtime.meshEpoch = 1
	runtime.sectionRevisions = make(map[core.SectionKey]uint64)
	runtime.meshChunks = make(map[core.ChunkKey]struct{})
	runtime.sessionMu.Unlock()
	return nil
}

// `AdvanceMeshes` performs at most `budget` scheduling attempts and accepted-result drains.
// It never waits for a worker. A full publication queue applies backpressure by leaving both the
// existing mesher result queue and the remaining dirty work untouched for a later host drain.
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
	if free == 0 {
		runtime.sessionMu.Unlock()
		return nil
	}
	work := min(budget, free)
	runtime.mesher.Schedule(runtime.mirrors.world, work)
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
		return meshReadyOperation{key: key, revision: revision}, nil
	}
	packed, err := packMeshedQuads(result.Quads)
	if err != nil {
		return meshReadyOperation{}, fmt.Errorf("runtime: pack section mesh %v: %w", key, err)
	}
	payload, err := presentation.NewSectionMeshPayload(packed)
	if err != nil {
		return meshReadyOperation{}, fmt.Errorf("runtime: encode section mesh %v: %w", key, err)
	}
	return meshReadyOperation{key: key, revision: revision, payload: payload, upsert: true}, nil
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
			if packedQuads+operation.payload.Len() > presentation.MaxWorldBatchPackedQuads {
				break
			}
			packedQuads += operation.payload.Len()
			upserts = append(upserts, presentation.SectionMeshUpsert{
				Key: operation.key, Revision: operation.revision, Payload: operation.payload,
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
		operation := element.Value.(meshReadyOperation)
		delete(runtime.meshReady.byKey, operation.key)
		runtime.meshReady.ordered.Remove(element)
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
	if !runtime.meshReady.canTransition(update.Dirty, keys) {
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
