//go:build cgo

package main

import (
	"encoding/binary"
	"unsafe"

	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
)

// This file owns the world family export surface: the two-phase drain of the
// world batch retained by the step family (see step.go for the retention
// ruling). One pull export serves both phases of the capacity protocol from
// the header orientation: a zero-capacity call is the query and never
// consumes producer state, a below-required capacity reports the fresh
// required size and writes nothing, and a sufficient capacity receives one
// exact write whose success is what commits consumption. The producer
// validates and encodes completely into owned bounded scratch space before
// that single copy, so a caller buffer can never observe a partial record.

// Wire layout of one world operation record, in little-endian words:
//
//	offset 0   kind        u32   `WorldOperationUpsert` or `WorldOperationDrop`
//	offset 4   dimension   u32   `core.DimensionID` of the section key, pinned
//	                          to 0 for Overworld and 1 for Depths
//	offset 8   pos_x       i32   `core.SectionPos.X`
//	offset 12  pos_y       i32   `core.SectionPos.Y`
//	offset 16  pos_z       i32   `core.SectionPos.Z`
//	offset 20  quad_count  u32   this upsert's packed quads; zero for drops
//	offset 24  revision    u64   the operation's presentation revision
//
// The maximal whole-record size is 4,325,408 bytes (header plus 4096
// operation records plus 524,288 packed quads): that is the consumer's
// single allocation ceiling for a maximal batch.
// `WorldOperationBytes` and the kind codes are producer-side wire vocabulary
// for the world family, like the input event layout in input.go: the frozen
// include/mornlea_client_core.h defines the MCW1 header, the counts, and the
// limits, while the body record layout is defined here by the producer and
// will be pinned by the Rust consumer when its pull buffer lands. The
// registry descriptor for the world family deliberately keeps `RecordBytes`
// at zero in this generation because both language pin suites pin that
// table; switching the descriptor to this fixed size is a coordinated
// cross-language update owned by the consumer task.
const WorldOperationBytes = 32

// World operation kind codes. The codes map exactly onto the two
// `presentation.WorldBatch` operation types and invent no third kind. Codes
// are frozen: new codes append and none is repurposed.
const (
	// WorldOperationUpsert replaces one complete section; its packed-quad
	// payload follows in the record's quad area.
	WorldOperationUpsert uint32 = 1
	// WorldOperationDrop removes one complete section and carries no quad
	// payload.
	WorldOperationDrop uint32 = 2
)

// worldBatchWireBytes returns the exact wire size of one batch encoding: the
// frozen header, one fixed record per operation, and eight bytes per packed
// quad. The presentation validators bound both counts, so the result always
// fits the `MaxWorldBatchOperations` and `MaxWorldBatchQuads` budget.
func worldBatchWireBytes(operations, packedQuads int) int {
	return int(WorldHeaderBytes) + operations*WorldOperationBytes + packedQuads*8
}

// worldBatchEncode produces the wire record of one world batch. It is a
// variable so tests can inject a panic through the seam and prove the export
// boundary converts it to `StatusPanic` without writing output; it is never
// redefined outside tests.
var worldBatchEncode = encodeWorldBatch

// encodeWorldBatch builds the wire record: the MCW1 header (magic, layout,
// operation and quad counts, epoch, atlas revision) followed by one
// operation record per upsert in presentation order, then one per drop in
// presentation order, then the packed-quad payload concatenated in the same
// record order (each upsert contributes its payload, each drop nothing, so a
// consumer walks the records' quad counts to find every payload offset).
// Determinism rests on the presentation value: `WorldBatch.Upserts` and
// `WorldBatch.Drops` return the stored order, which the runtime fixed when
// it built the batch, so the same value always encodes to the same bytes.
// The batch is revalidated first because the presentation validators own
// every bound the encoding relies on (counts, per-section quad limits,
// duplicate keys, packed-quad integrity); a retained value that fails them
// violates a producer invariant and yields nil, which callers map to
// `StatusInternal` rather than publishing a partial record.
func encodeWorldBatch(batch presentation.WorldBatch) []byte {
	if err := batch.Validate(); err != nil {
		return nil
	}
	upserts := batch.Upserts()
	drops := batch.Drops()
	packedQuadCount := 0
	for _, upsert := range upserts {
		packedQuadCount += upsert.Payload.Len()
	}
	record := make([]byte, worldBatchWireBytes(len(upserts)+len(drops), packedQuadCount))
	binary.LittleEndian.PutUint32(record[0:4], uint32(MagicWorld))
	binary.LittleEndian.PutUint32(record[4:8], WorldVersion)
	binary.LittleEndian.PutUint32(record[8:12], uint32(len(upserts)+len(drops)))
	binary.LittleEndian.PutUint32(record[12:16], uint32(packedQuadCount))
	binary.LittleEndian.PutUint64(record[16:24], batch.Epoch)
	binary.LittleEndian.PutUint64(record[24:32], uint64(batch.AtlasRevision))
	offset := int(WorldHeaderBytes)
	quadOffset := worldBatchWireBytes(len(upserts)+len(drops), 0)
	for _, upsert := range upserts {
		quads := upsert.Payload.PackedQuads()
		worldPutOperation(record[offset:offset+WorldOperationBytes],
			WorldOperationUpsert, upsert.Key, uint32(len(quads)), upsert.Revision)
		for _, packed := range quads {
			binary.LittleEndian.PutUint64(record[quadOffset:quadOffset+8], packed)
			quadOffset += 8
		}
		offset += WorldOperationBytes
	}
	for _, drop := range drops {
		worldPutOperation(record[offset:offset+WorldOperationBytes],
			WorldOperationDrop, drop.Key, 0, drop.Revision)
		offset += WorldOperationBytes
	}
	return record
}

// worldPutOperation writes one 32-byte operation record from its
// presentation fields; the section coordinates keep their signed 32-bit
// values because `core.SectionPos` coordinates are signed and the
// presentation validators already bound the Y range.
func worldPutOperation(record []byte, kind uint32, key core.SectionKey, quadCount uint32, revision uint64) {
	binary.LittleEndian.PutUint32(record[0:4], kind)
	binary.LittleEndian.PutUint32(record[4:8], uint32(key.Dimension))
	binary.LittleEndian.PutUint32(record[8:12], uint32(key.Pos.X))
	binary.LittleEndian.PutUint32(record[12:16], uint32(key.Pos.Y))
	binary.LittleEndian.PutUint32(record[16:20], uint32(key.Pos.Z))
	binary.LittleEndian.PutUint32(record[20:24], quadCount)
	binary.LittleEndian.PutUint64(record[24:32], revision)
}

// worldPullSnapshot is one consistent view of the world batch the pull
// family may drain. The batch value shares immutable backing arrays with the
// retained step result (the runtime builds owned batches and retention
// replaces the whole value), so the snapshot stays valid across later steps
// without copying the payloads here.
type worldPullSnapshot struct {
	batch      presentation.WorldBatch
	generation uint64
	available  bool
}

// worldPullSnapshot captures the pullable state under the session mutex. A
// batch is available exactly when a step result was retained, it published a
// world batch, and that result's generation was not consumed yet. The
// generation pair (see `clientSession`) gives the pull family a monotonic
// identity for the retained result without borrowing frame-revision
// semantics.
func (session *clientSession) worldPullSnapshot() worldPullSnapshot {
	session.mu.Lock()
	defer session.mu.Unlock()
	snapshot := worldPullSnapshot{generation: session.stepGeneration}
	switch {
	case !session.hasStepResult:
		return snapshot
	case !session.stepResult.HasWorld:
		return snapshot
	case session.worldPulledGeneration == session.stepGeneration:
		return snapshot
	}
	snapshot.batch = session.stepResult.World
	snapshot.available = true
	return snapshot
}

// worldPullCommit marks the served generation consumed. The commit targets
// the generation the pull served, not the currently retained one: a step
// that retained a newer result while this pull was encoding must not have
// its fresh batch swallowed by an older pull's commit, so the newer batch
// stays consumable. This is the only state change a successful pull makes.
func (session *clientSession) worldPullCommit(generation uint64) {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.stepGeneration == generation {
		session.worldPulledGeneration = generation
	}
}

// pullWorldBatch runs the two-phase drain under `worldMu`, which serializes
// the whole query-encode-write-commit sequence so two concurrent consumes
// cannot both serve the one retained batch. The session mutex is taken only
// for the snapshot and the commit, so an in-flight pull never blocks polls,
// steps, or teardown. Outcomes:
//   - No unconsumed batch (no step completed, the latest step published no
//     world batch, or the batch was already consumed): `StatusOK` and a zero
//     size word, writing no buffer byte. Size zero is unambiguous because
//     the frozen header alone is 32 bytes and the presentation contract
//     requires at least one operation per batch.
//   - Encoder rejection of the retained value: `StatusInternal` with no
//     output; the batch stays consumable because nothing was committed.
//   - Capacity below the encoded size: `StatusInsufficientCapacity` and the
//     fresh required size in the size word; no buffer byte is written and
//     the batch stays consumable, so repeated queries are idempotent.
//   - Sufficient capacity: the whole record is encoded into owned scratch
//     and copied once, exactly; only after that copy does the commit run,
//     so consumption happens if and only if the exact write happened.
func (session *clientSession) pullWorldBatch(out *byte, capacity uint32, requiredOut *uint32) Status {
	session.worldMu.Lock()
	defer session.worldMu.Unlock()
	snapshot := session.worldPullSnapshot()
	if !snapshot.available {
		*requiredOut = 0
		return StatusOK
	}
	record := worldBatchEncode(snapshot.batch)
	if record == nil {
		return StatusInternal
	}
	if uint32(len(record)) > capacity {
		*requiredOut = uint32(len(record))
		return StatusInsufficientCapacity
	}
	copy(unsafe.Slice(out, len(record)), record)
	session.worldPullCommit(snapshot.generation)
	return StatusOK
}

// coreWorldPull drains the world batch retained by the step family through
// the two-phase capacity protocol. It is the testable core behind the
// exported world pull symbol; the raw pointer arguments mirror the C
// surface. A zero-capacity call is the required-size query; a positive
// capacity below the required size is the capacity signal; a sufficient
// capacity performs the one exact write whose success commits consumption.
//
// Epoch ruling: pulls always serve the latest completed step, matching the
// step retention ruling. The wire header carries the retained batch's own
// epoch and atlas revision, and the caller cannot request an older epoch
// (the frozen header has no epoch parameter), so a batch older than the
// retained generation cannot be pulled back. A size queried before a newer
// step is therefore never honored against the new batch: the consume either
// fails with the fresh required size (a larger new batch) or writes the new
// batch's exact bytes (a smaller one); the caller must re-query after any
// step to learn the new size.
//
// Session ruling: the pull reads retained presentation state, not the live
// connection, so it never reports `StatusDisconnected`. A terminal session
// still pulls its last retained batch and consumes it normally (retention
// survives teardown, see step.go); only a destroyed handle is rejected with
// `StatusInvalidState`.
//
// Overlap ruling: this export holds exactly two caller pointers, the write
// buffer and the size out-parameter. A caller buffer cannot overlap
// producer-retained memory because the producer encodes into Go-heap scratch
// that no C pointer can reach. The one aliasing hazard inside the export is
// the size word landing inside the caller-declared buffer span: the size
// write would corrupt the record mid-protocol (or the copy would clobber the
// size word), so a size out-parameter pointing anywhere inside the write
// buffer's declared capacity span is rejected with
// `StatusInvalidArgument`, mirroring the engine ABI's pairwise-overlap
// discipline.
//
// Validation order, each phase returning before the next runs:
//  1. Handle lifecycle (table): an unknown value reports
//     `StatusInvalidHandle` and a destroyed session `StatusInvalidState`,
//     both before any argument is read.
//  2. Pointer and length shape: the size out-parameter is mandatory because
//     every call may report a size; a positive capacity needs a non-null,
//     `ABIAlignment`-aligned buffer that does not contain the size word.
//     Violations report `StatusInvalidArgument` before any retained state is
//     read.
//  3. Content: the two-phase drain above (snapshot, encode, capacity,
//     one exact write, commit).
//
// A recovered panic reports `StatusPanic` with no output byte; the guarded
// path writes caller memory only as its final successful step, so a panic
// can only fire before any write, and nothing was committed. The caller's
// pointers are never retained after the call returns.
func coreWorldPull(handle uint64, out *byte, capacity uint32, requiredOut *uint32) Status {
	return withPanicGuard(func() Status {
		session, status := clientSessions.sessionFor(handle)
		if status != StatusOK {
			return status
		}
		if requiredOut == nil {
			return StatusInvalidArgument
		}
		if capacity > 0 {
			if out == nil {
				return StatusInvalidArgument
			}
			bufferAddress := uintptr(unsafe.Pointer(out))
			if bufferAddress%uintptr(ABIAlignment) != 0 {
				return StatusInvalidArgument
			}
			sizeAddress := uintptr(unsafe.Pointer(requiredOut))
			if sizeAddress >= bufferAddress && sizeAddress < bufferAddress+uintptr(capacity) {
				return StatusInvalidArgument
			}
		}
		return session.pullWorldBatch(out, capacity, requiredOut)
	})
}
