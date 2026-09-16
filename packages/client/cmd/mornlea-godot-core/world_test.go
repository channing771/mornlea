package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/channing771/mornlea/packages/client/mesh"
	"github.com/channing771/mornlea/packages/client/presentation"
	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
)

// The world tests pin the world family's two-phase drain: one pull export
// whose zero-capacity query reports the exact required size without
// consuming, whose insufficient-capacity call reports the fresh required size
// and writes nothing, and whose exact write commits consumption exactly once.
// The wire record is the frozen MCW1 header plus fixed 32-byte operation
// records and the concatenated packed-quad payload; the tests decode every
// field against the presentation value that produced it. Online sessions come
// from the scripted transport, and world batches arrive through the
// `stepRuntime` seam as fully formed `runtime.StepResult` values, so no test
// opens a socket or drives the real mesher.

// worldSeamQuad builds one valid packed quad distinct per seed, staying
// inside the plain-quad domain (non-plant face and material, no corner
// heights) so `mesh.UnpackQuad` round-trips it.
func worldSeamQuad(seed uint8) uint64 {
	return mesh.Quad{
		X: seed, Y: 2, Z: 3, W: 4, H: 3,
		Face: mesh.FacePosX, Mat: uint16(5 + seed), AO: 1, Light: 9,
	}.Pack()
}

// worldSeamPayload builds a validated section payload of count distinct
// packed quads.
func worldSeamPayload(t *testing.T, count int) presentation.SectionMeshPayload {
	t.Helper()
	packed := make([]uint64, count)
	for index := range packed {
		packed[index] = worldSeamQuad(uint8(index + 1))
	}
	payload, err := presentation.NewSectionMeshPayload(packed)
	if err != nil {
		t.Fatalf("build payload of %d quads: %v", count, err)
	}
	return payload
}

// worldSeamMixedBatch builds the canonical mixed batch: two upserts with
// three and five quads (negative section coordinates on the first) plus two
// drops, one in the Depths dimension.
func worldSeamMixedBatch(t *testing.T, epoch, atlas uint64) presentation.WorldBatch {
	t.Helper()
	upserts := []presentation.SectionMeshUpsert{
		{
			Key:      core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: -12, Y: 3, Z: 45}},
			Revision: 101,
			Payload:  worldSeamPayload(t, 3),
		},
		{
			Key:      core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: 300, Y: 23, Z: -400}},
			Revision: 102,
			Payload:  worldSeamPayload(t, 5),
		},
	}
	drops := []presentation.SectionDrop{
		{Key: core.SectionKey{Dimension: core.Depths, Pos: core.SectionPos{X: 7, Y: 0, Z: -9}}, Revision: 201},
		{Key: core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: -1, Y: 12, Z: 2}}, Revision: 202},
	}
	batch, err := presentation.NewWorldBatch(epoch, presentation.AtlasRevision(atlas), upserts, drops)
	if err != nil {
		t.Fatalf("build mixed batch: %v", err)
	}
	return batch
}

// worldSeamDropBatch builds a drop-only batch; drop operations carry no quad
// payload, so the wire record has a zero-length quad area.
func worldSeamDropBatch(t *testing.T, epoch, atlas uint64) presentation.WorldBatch {
	t.Helper()
	drops := []presentation.SectionDrop{
		{Key: core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: 5, Y: 1, Z: 6}}, Revision: 11},
		{Key: core.SectionKey{Dimension: core.Depths, Pos: core.SectionPos{X: -8, Y: 22, Z: 9}}, Revision: 12},
		{Key: core.SectionKey{Dimension: core.Overworld, Pos: core.SectionPos{X: 1, Y: 0, Z: -2}}, Revision: 13},
	}
	batch, err := presentation.NewWorldBatch(epoch, presentation.AtlasRevision(atlas), nil, drops)
	if err != nil {
		t.Fatalf("build drop batch: %v", err)
	}
	return batch
}

// worldSeamFrame builds the benign frame carried by scripted step results.
func worldSeamFrame(revision uint64) presentation.FrameSnapshot {
	return presentation.FrameSnapshot{
		Version:  presentation.FrameSnapshotVersion,
		Revision: revision,
		Epoch:    1,
		Phase:    clientruntime.ConnectionPhaseLoading,
	}
}

// worldSeamResult wraps one optional world batch in a scripted step result.
func worldSeamResult(revision uint64, batch presentation.WorldBatch, hasWorld bool) clientruntime.StepResult {
	result := clientruntime.StepResult{Frame: worldSeamFrame(revision), HasWorld: hasWorld}
	if hasWorld {
		result.World = batch
	}
	return result
}

// installWorldResultSeam replaces `stepRuntime` with a seam serving the
// scripted results in order and restores the production step afterwards; the
// seam's call counter is safe only because package tests run sequentially
// and exactly one goroutine drives steps within one test.
func installWorldResultSeam(t *testing.T, results ...clientruntime.StepResult) *int {
	t.Helper()
	calls := new(int)
	previous := stepRuntime
	stepRuntime = func(_ *clientruntime.Runtime, _ time.Duration, _, _ int) (clientruntime.StepResult, error) {
		index := *calls
		*calls++
		if index >= len(results) {
			return clientruntime.StepResult{}, errors.New("world seam: no scripted result left")
		}
		return results[index], nil
	}
	t.Cleanup(func() { stepRuntime = previous })
	return calls
}

// worldStepOnce drives one step carrying the next scripted seam result.
func worldStepOnce(t *testing.T, handle uint64) Status {
	t.Helper()
	pointer, length := stepRequestPointer(0, 0, 0)
	return coreStep(handle, pointer, length)
}

// worldQuerySize performs the zero-capacity two-phase size query.
func worldQuerySize(t *testing.T, handle uint64) (uint32, Status) {
	t.Helper()
	required := uint32(0xDEADBEEF)
	status := coreWorldPull(handle, nil, 0, &required)
	return required, status
}

// worldCallerFrame models one C caller's frame for the pull: the write
// buffer at the start of one caller-owned allocation and the size word at an
// aligned offset beyond every declared span, so the two pointers can never
// drift into accidental numeric overlap with unrelated Go allocations (a Go
// test's stack locals can sit numerically beside heap objects, which a C
// caller's own allocations never do). The backing storage is also padded past
// the smallest Go size class so the buffer base keeps the ABI alignment a C
// caller's allocation would have.
func worldCallerFrame(capacity uint32) (buffer []byte, sizeOut *uint32) {
	storage := poisonedBuffer(int(capacity) + 40)
	buffer = storage[:int(capacity):int(capacity)]
	sizeOffset := (int(capacity)+7)&^7 + 32
	return buffer, (*uint32)(unsafe.Pointer(&storage[sizeOffset]))
}

// worldConsume performs one consume into a fresh caller frame of exactly the
// declared capacity, returning the buffer view, the reported size word, and
// the status. The capacity must be positive so the buffer has an address.
func worldConsume(handle uint64, capacity uint32) (buffer []byte, required uint32, status Status) {
	buffer, sizeOut := worldCallerFrame(capacity)
	*sizeOut = uint32(0xDEADBEEF)
	status = coreWorldPull(handle, &buffer[0], capacity, sizeOut)
	return buffer, *sizeOut, status
}

// worldWireOperation is the decoded view of one 32-byte operation record.
type worldWireOperation struct {
	kind      uint32
	dimension uint32
	x, y, z   int32
	quadCount uint32
	revision  uint64
}

// worldWireBatch is the decoded view of one complete world record.
type worldWireBatch struct {
	operationCount uint32
	quadCount      uint32
	epoch          uint64
	atlasRevision  uint64
	operations     []worldWireOperation
	quads          []uint64
}

// decodeWorldWire decodes one complete record, pinning the header identity
// and the count-to-length arithmetic before returning every field.
func decodeWorldWire(t *testing.T, buffer []byte) worldWireBatch {
	t.Helper()
	if len(buffer) < int(WorldHeaderBytes) {
		t.Fatalf("record of %d bytes is shorter than the header", len(buffer))
	}
	if magic := binary.LittleEndian.Uint32(buffer[0:4]); magic != uint32(MagicWorld) {
		t.Fatalf("header magic = %#x, want world magic %#x", magic, uint32(MagicWorld))
	}
	if layout := binary.LittleEndian.Uint32(buffer[4:8]); layout != WorldVersion {
		t.Fatalf("header layout = %d, want %d", layout, WorldVersion)
	}
	decoded := worldWireBatch{
		operationCount: binary.LittleEndian.Uint32(buffer[8:12]),
		quadCount:      binary.LittleEndian.Uint32(buffer[12:16]),
		epoch:          binary.LittleEndian.Uint64(buffer[16:24]),
		atlasRevision:  binary.LittleEndian.Uint64(buffer[24:32]),
	}
	wantBytes := worldBatchWireBytes(int(decoded.operationCount), int(decoded.quadCount))
	if len(buffer) != wantBytes {
		t.Fatalf("record length %d does not match counts (%d operations, %d quads => %d bytes)",
			len(buffer), decoded.operationCount, decoded.quadCount, wantBytes)
	}
	decoded.operations = make([]worldWireOperation, decoded.operationCount)
	offset := int(WorldHeaderBytes)
	for index := range decoded.operations {
		record := buffer[offset : offset+int(WorldOperationBytes)]
		decoded.operations[index] = worldWireOperation{
			kind:      binary.LittleEndian.Uint32(record[0:4]),
			dimension: binary.LittleEndian.Uint32(record[4:8]),
			x:         int32(binary.LittleEndian.Uint32(record[8:12])),
			y:         int32(binary.LittleEndian.Uint32(record[12:16])),
			z:         int32(binary.LittleEndian.Uint32(record[16:20])),
			quadCount: binary.LittleEndian.Uint32(record[20:24]),
			revision:  binary.LittleEndian.Uint64(record[24:32]),
		}
		offset += int(WorldOperationBytes)
	}
	decoded.quads = make([]uint64, decoded.quadCount)
	for index := range decoded.quads {
		base := offset + index*8
		decoded.quads[index] = binary.LittleEndian.Uint64(buffer[base : base+8])
	}
	return decoded
}

// worldRecordLength reads a consumed buffer's count words and returns the
// exact encoded length, so a race test can trim a capacity-sized buffer back
// to the record a smaller latest batch wrote.
func worldRecordLength(t *testing.T, buffer []byte) int {
	t.Helper()
	operations := binary.LittleEndian.Uint32(buffer[8:12])
	quads := binary.LittleEndian.Uint32(buffer[12:16])
	return worldBatchWireBytes(int(operations), int(quads))
}

// assertWorldCanary proves a buffer keeps its poison after a call that must
// not write it.
func assertWorldCanary(t *testing.T, buffer []byte, context string) {
	t.Helper()
	for index, value := range buffer {
		if value != 0xA5 {
			t.Fatalf("%s wrote buffer byte %d despite writing nothing", context, index)
		}
	}
}

// TestWorldBatchWireVocabularyPinsPilotValues pins the producer-side world
// wire vocabulary: the fixed operation-record size, the kind codes, and the
// maximal whole-record arithmetic inside the frozen header limits.
func TestWorldBatchWireVocabularyPinsPilotValues(t *testing.T) {
	if got, want := WorldOperationBytes, 32; got != want {
		t.Fatalf("operation record bytes = %d, want %d", got, want)
	}
	if got, want := WorldOperationUpsert, uint32(1); got != want {
		t.Fatalf("upsert kind = %d, want %d", got, want)
	}
	if got, want := WorldOperationDrop, uint32(2); got != want {
		t.Fatalf("drop kind = %d, want %d", got, want)
	}
	if got, want := WorldHeaderBytes, uint32(32); got != want {
		t.Fatalf("world header bytes = %d, want %d", got, want)
	}
	if got, want := worldBatchWireBytes(int(MaxWorldBatchOperations), int(MaxWorldBatchQuads)),
		int(WorldHeaderBytes)+int(MaxWorldBatchOperations)*WorldOperationBytes+int(MaxWorldBatchQuads)*8; got != want {
		t.Fatalf("maximal batch bytes = %d, want %d", got, want)
	}
	// Pin the literal dimension wire codes so a domain renumber cannot pass
	// silently through the uint32 cast the encoder uses.
	if got, want := uint32(core.Overworld), uint32(0); got != want {
		t.Fatalf("overworld dimension code = %d, want %d", got, want)
	}
	if got, want := uint32(core.Depths), uint32(1); got != want {
		t.Fatalf("depths dimension code = %d, want %d", got, want)
	}
	// Pin the maximal consumer allocation ceiling as a literal.
	if got, want := worldBatchWireBytes(int(MaxWorldBatchOperations), int(MaxWorldBatchQuads)), 4_325_408; got != want {
		t.Fatalf("maximal batch bytes = %d, want %d", got, want)
	}
}

// TestWorldBatchQueryReportsExactSizeAndRepeatQueriesDoNotConsume pins the
// two-phase query: the required size is the exact byte count of the encoded
// batch, repeated queries return the same size without consuming, and the
// subsequent exact-capacity consume serves the retained batch and commits,
// leaving the second pull with the documented no-batch outcome.
func TestWorldBatchQueryReportsExactSizeAndRepeatQueriesDoNotConsume(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	batch := worldSeamMixedBatch(t, 3, 11)
	installWorldResultSeam(t, worldSeamResult(1, batch, true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted world step = %d, want StatusOK", status)
	}

	wantBytes := worldBatchWireBytes(4, 8)
	for query := 0; query < 3; query++ {
		required, status := worldQuerySize(t, handle)
		if status != StatusInsufficientCapacity {
			t.Fatalf("query %d = %d, want StatusInsufficientCapacity", query, status)
		}
		if required != uint32(wantBytes) {
			t.Fatalf("query %d required = %d, want %d", query, required, wantBytes)
		}
	}

	storage, required, status := worldConsume(handle, uint32(wantBytes))
	if status != StatusOK {
		t.Fatalf("consume after repeated queries = %d, want StatusOK", status)
	}
	if required != uint32(0xDEADBEEF) {
		t.Fatalf("consume wrote the size out-parameter %d; a successful write reports no size", required)
	}
	decoded := decodeWorldWire(t, storage)
	if decoded.epoch != 3 || decoded.atlasRevision != 11 {
		t.Fatalf("consumed identity = epoch %d atlas %d, want 3 and 11", decoded.epoch, decoded.atlasRevision)
	}

	required, status = worldQuerySize(t, handle)
	if status != StatusOK || required != 0 {
		t.Fatalf("query after consume = (%d, %d), want 0 and StatusOK (no batch)", required, status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the drain = %d, want StatusOK", status)
	}
}

// TestWorldBatchInsufficientCapacityWritesNothingAndKeepsBatchConsumable
// proves a below-required capacity never writes a partial record: the canary
// survives, the fresh required size is reported, and the same batch stays
// consumable afterwards.
func TestWorldBatchInsufficientCapacityWritesNothingAndKeepsBatchConsumable(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	batch := worldSeamMixedBatch(t, 4, 12)
	installWorldResultSeam(t, worldSeamResult(1, batch, true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted world step = %d, want StatusOK", status)
	}
	wantBytes := uint32(worldBatchWireBytes(4, 8))

	for _, capacity := range []uint32{1, wantBytes - 1, wantBytes - 8} {
		storage, required, status := worldConsume(handle, capacity)
		if status != StatusInsufficientCapacity {
			t.Fatalf("capacity %d status = %d, want StatusInsufficientCapacity", capacity, status)
		}
		if required != wantBytes {
			t.Fatalf("capacity %d reported required = %d, want %d", capacity, required, wantBytes)
		}
		assertWorldCanary(t, storage, "insufficient-capacity consume")
	}

	storage, _, status := worldConsume(handle, wantBytes)
	if status != StatusOK {
		t.Fatalf("consume after failed drains = %d, want StatusOK (failures consumed nothing)", status)
	}
	if decoded := decodeWorldWire(t, storage); decoded.epoch != 4 || decoded.atlasRevision != 12 {
		t.Fatalf("late consume identity = epoch %d atlas %d, want 4 and 12", decoded.epoch, decoded.atlasRevision)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the drain = %d, want StatusOK", status)
	}
}

// TestWorldBatchOversizedCapacityWritesExactlyAndLeavesTailUntouched proves
// an oversized buffer receives exactly the required bytes and the surplus
// tail survives untouched, byte-for-byte equal to an exact-capacity write of
// an identical batch.
func TestWorldBatchOversizedCapacityWritesExactlyAndLeavesTailUntouched(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	installWorldResultSeam(t,
		worldSeamResult(1, worldSeamMixedBatch(t, 5, 13), true),
		worldSeamResult(2, worldSeamMixedBatch(t, 5, 13), true),
	)
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("first scripted world step = %d, want StatusOK", status)
	}
	wantBytes := uint32(worldBatchWireBytes(4, 8))

	oversized, _, status := worldConsume(handle, wantBytes+13)
	if status != StatusOK {
		t.Fatalf("oversized consume = %d, want StatusOK", status)
	}
	for index := int(wantBytes); index < len(oversized); index++ {
		if oversized[index] != 0xA5 {
			t.Fatalf("oversized write touched tail byte %d beyond the required size", index)
		}
	}

	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("second scripted world step = %d, want StatusOK", status)
	}
	exact, _, status := worldConsume(handle, wantBytes)
	if status != StatusOK {
		t.Fatalf("exact consume = %d, want StatusOK", status)
	}
	for index := 0; index < int(wantBytes); index++ {
		if oversized[index] != exact[index] {
			t.Fatalf("oversized write diverges from the exact record at byte %d", index)
		}
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the drain = %d, want StatusOK", status)
	}
}

// TestWorldBatchEncodesUpsertsThenDropsFaithfully decodes every field of a
// mixed batch: header identity and counts, upsert records followed by drop
// records in presentation order, signed section coordinates, per-operation
// revisions, per-operation quad counts, and the packed-quad payload
// concatenated in record order with drops contributing nothing.
func TestWorldBatchEncodesUpsertsThenDropsFaithfully(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	batch := worldSeamMixedBatch(t, 9, 77)
	installWorldResultSeam(t, worldSeamResult(1, batch, true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted world step = %d, want StatusOK", status)
	}

	upserts := batch.Upserts()
	drops := batch.Drops()
	wantBytes := worldBatchWireBytes(len(upserts)+len(drops), 8)
	storage, _, status := worldConsume(handle, uint32(wantBytes))
	if status != StatusOK {
		t.Fatalf("mixed consume = %d, want StatusOK", status)
	}
	decoded := decodeWorldWire(t, storage)
	if decoded.operationCount != 4 || decoded.quadCount != 8 {
		t.Fatalf("counts = (%d operations, %d quads), want (4, 8)", decoded.operationCount, decoded.quadCount)
	}
	if decoded.epoch != 9 || decoded.atlasRevision != 77 {
		t.Fatalf("identity = epoch %d atlas %d, want 9 and 77", decoded.epoch, decoded.atlasRevision)
	}
	for index, upsert := range upserts {
		operation := decoded.operations[index]
		if operation.kind != WorldOperationUpsert {
			t.Fatalf("operation %d kind = %d, want upsert %d", index, operation.kind, WorldOperationUpsert)
		}
		if operation.dimension != uint32(upsert.Key.Dimension) || operation.x != upsert.Key.Pos.X ||
			operation.y != upsert.Key.Pos.Y || operation.z != upsert.Key.Pos.Z {
			t.Fatalf("upsert %d section = (%d, %d, %d, %d), want %v",
				index, operation.dimension, operation.x, operation.y, operation.z, upsert.Key)
		}
		if operation.revision != upsert.Revision {
			t.Fatalf("upsert %d revision = %d, want %d", index, operation.revision, upsert.Revision)
		}
		if operation.quadCount != uint32(upsert.Payload.Len()) {
			t.Fatalf("upsert %d quad count = %d, want %d", index, operation.quadCount, upsert.Payload.Len())
		}
	}
	for index, drop := range drops {
		operation := decoded.operations[len(upserts)+index]
		if operation.kind != WorldOperationDrop {
			t.Fatalf("drop %d kind = %d, want drop %d", index, operation.kind, WorldOperationDrop)
		}
		if operation.dimension != uint32(drop.Key.Dimension) || operation.x != drop.Key.Pos.X ||
			operation.y != drop.Key.Pos.Y || operation.z != drop.Key.Pos.Z {
			t.Fatalf("drop %d section = (%d, %d, %d, %d), want %v",
				index, operation.dimension, operation.x, operation.y, operation.z, drop.Key)
		}
		if operation.revision != drop.Revision {
			t.Fatalf("drop %d revision = %d, want %d", index, operation.revision, drop.Revision)
		}
		if operation.quadCount != 0 {
			t.Fatalf("drop %d carries %d quads, want none", index, operation.quadCount)
		}
	}
	wantQuads := append(append([]uint64(nil), upserts[0].Payload.PackedQuads()...), upserts[1].Payload.PackedQuads()...)
	for index, want := range wantQuads {
		if decoded.quads[index] != want {
			t.Fatalf("payload quad %d = %#x, want %#x in record order", index, decoded.quads[index], want)
		}
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the drain = %d, want StatusOK", status)
	}
}

// TestWorldBatchDropOnlyBatchKeepsZeroQuadSizeArithmetic pins the all-drop
// batch: the whole record is the header plus operation records with a zero
// quad count and an empty payload area.
func TestWorldBatchDropOnlyBatchKeepsZeroQuadSizeArithmetic(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	batch := worldSeamDropBatch(t, 6, 21)
	installWorldResultSeam(t, worldSeamResult(1, batch, true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted world step = %d, want StatusOK", status)
	}

	wantBytes := uint32(worldBatchWireBytes(3, 0))
	required, status := worldQuerySize(t, handle)
	if status != StatusInsufficientCapacity || required != wantBytes {
		t.Fatalf("drop-only query = (%d, %d), want (%d, StatusInsufficientCapacity)", required, status, wantBytes)
	}
	storage, _, status := worldConsume(handle, wantBytes)
	if status != StatusOK {
		t.Fatalf("drop-only consume = %d, want StatusOK", status)
	}
	decoded := decodeWorldWire(t, storage)
	if decoded.quadCount != 0 || len(decoded.quads) != 0 {
		t.Fatalf("drop-only quad area = (%d declared, %d decoded), want empty", decoded.quadCount, len(decoded.quads))
	}
	for index, operation := range decoded.operations {
		if operation.kind != WorldOperationDrop || operation.quadCount != 0 {
			t.Fatalf("drop-only operation %d = %+v, want a drop with no quads", index, operation)
		}
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the drain = %d, want StatusOK", status)
	}
}

// TestWorldBatchNoWorldStepAndIdleSessionReportNoBatch pins the no-batch
// outcome: an idle session that never stepped, a completed step without a
// world publication, and an already consumed batch all report `StatusOK`
// with size zero and write no buffer byte.
func TestWorldBatchNoWorldStepAndIdleSessionReportNoBatch(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	storage, required, status := worldConsume(idle, 64)
	if status != StatusOK || required != 0 {
		t.Fatalf("idle pull = (%d, %d), want (0, StatusOK)", required, status)
	}
	assertWorldCanary(t, storage, "idle no-batch pull")
	if status := coreDestroy(idle); status != StatusOK {
		t.Fatalf("destroy idle = %d, want StatusOK", status)
	}

	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	installWorldResultSeam(t,
		worldSeamResult(1, presentation.WorldBatch{}, false),
		worldSeamResult(2, worldSeamMixedBatch(t, 7, 30), true),
	)
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("no-world step = %d, want StatusOK", status)
	}
	storage, required, status = worldConsume(handle, 64)
	if status != StatusOK || required != 0 {
		t.Fatalf("no-world pull = (%d, %d), want (0, StatusOK)", required, status)
	}
	assertWorldCanary(t, storage, "no-world no-batch pull")

	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("world step = %d, want StatusOK", status)
	}
	if _, _, status := worldConsume(handle, uint32(worldBatchWireBytes(4, 8))); status != StatusOK {
		t.Fatalf("consume of the published batch = %d, want StatusOK", status)
	}
	storage, required, status = worldConsume(handle, 64)
	if status != StatusOK || required != 0 {
		t.Fatalf("post-consume pull = (%d, %d), want (0, StatusOK)", required, status)
	}
	assertWorldCanary(t, storage, "post-consume no-batch pull")
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the drain = %d, want StatusOK", status)
	}
}

// TestWorldBatchStaleQueriedSizeNotHonoredAfterNewerStep pins the epoch
// ruling: pulls always serve the latest completed step, so a size queried
// before a newer step is not honored against the new batch — the
// insufficient-capacity path reports the fresh size, a re-query confirms it,
// and the consume serves the new batch with its own epoch, atlas revision,
// and content.
func TestWorldBatchStaleQueriedSizeNotHonoredAfterNewerStep(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	oldBatch := worldSeamDropBatch(t, 2, 41)
	newBatch := worldSeamMixedBatch(t, 8, 42)
	installWorldResultSeam(t, worldSeamResult(1, oldBatch, true), worldSeamResult(2, newBatch, true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("first scripted step = %d, want StatusOK", status)
	}
	staleSize, status := worldQuerySize(t, handle)
	if status != StatusInsufficientCapacity {
		t.Fatalf("first query = %d, want StatusInsufficientCapacity", status)
	}
	freshSize := uint32(worldBatchWireBytes(4, 8))
	if staleSize == freshSize {
		t.Fatalf("test fixtures must have distinct sizes (both %d)", staleSize)
	}

	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("second scripted step = %d, want StatusOK", status)
	}
	storage, required, status := worldConsume(handle, staleSize)
	if status != StatusInsufficientCapacity {
		t.Fatalf("consume with the stale size = %d, want StatusInsufficientCapacity", status)
	}
	if required != freshSize {
		t.Fatalf("stale consume reported required = %d, want the fresh size %d", required, freshSize)
	}
	assertWorldCanary(t, storage, "stale-size consume")

	confirmed, status := worldQuerySize(t, handle)
	if status != StatusInsufficientCapacity || confirmed != freshSize {
		t.Fatalf("re-query = (%d, %d), want (%d, StatusInsufficientCapacity)", confirmed, status, freshSize)
	}
	storage, _, status = worldConsume(handle, freshSize)
	if status != StatusOK {
		t.Fatalf("fresh consume = %d, want StatusOK", status)
	}
	decoded := decodeWorldWire(t, storage)
	if decoded.epoch != newBatch.Epoch || decoded.atlasRevision != uint64(newBatch.AtlasRevision) {
		t.Fatalf("fresh identity = epoch %d atlas %d, want %d and %d",
			decoded.epoch, decoded.atlasRevision, newBatch.Epoch, uint64(newBatch.AtlasRevision))
	}
	if decoded.operationCount != 4 || decoded.quadCount != 8 {
		t.Fatalf("fresh counts = (%d, %d), want the mixed batch (4, 8)", decoded.operationCount, decoded.quadCount)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the drain = %d, want StatusOK", status)
	}
}

// TestWorldBatchValidatesHandleAndPointerShapeBeforeContent pins the
// validation order: the handle lifecycle outranks every pointer defect, the
// pointer shape outranks the no-batch content outcome, and every rejection
// leaves the retained batch consumable.
func TestWorldBatchValidatesHandleAndPointerShapeBeforeContent(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	if status := coreWorldPull(0, nil, 0, nil); status != StatusInvalidHandle {
		t.Fatalf("pull on the zero handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreWorldPull(makeSessionHandle(3, 1), nil, 0, nil); status != StatusInvalidHandle {
		t.Fatalf("pull on a never-issued handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreWorldPull(destroyed, nil, 0, nil); status != StatusInvalidState {
		t.Fatalf("pull on a destroyed handle = %d, want StatusInvalidState", status)
	}
	if status := coreWorldPull(idle, nil, 0, nil); status != StatusInvalidArgument {
		t.Fatalf("pull with a null size out-parameter = %d, want StatusInvalidArgument (shape precedes content)", status)
	}

	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	installWorldResultSeam(t, worldSeamResult(1, worldSeamMixedBatch(t, 3, 11), true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted world step = %d, want StatusOK", status)
	}
	wantBytes := uint32(worldBatchWireBytes(4, 8))

	if status := coreWorldPull(handle, nil, 0, nil); status != StatusInvalidArgument {
		t.Fatalf("pull with a null size out-parameter = %d, want StatusInvalidArgument", status)
	}
	if status := coreWorldPull(handle, nil, wantBytes, new(uint32)); status != StatusInvalidArgument {
		t.Fatalf("pull with a null buffer and positive capacity = %d, want StatusInvalidArgument", status)
	}
	misalignedStorage := poisonedBuffer(int(wantBytes) + 32)
	misaligned := (*byte)(unsafe.Pointer(&misalignedStorage[1]))
	misalignedSize := (*uint32)(unsafe.Pointer(&misalignedStorage[int(wantBytes)+24]))
	*misalignedSize = uint32(0xDEADBEEF)
	if status := coreWorldPull(handle, misaligned, wantBytes, misalignedSize); status != StatusInvalidArgument {
		t.Fatalf("pull with a misaligned buffer = %d, want StatusInvalidArgument", status)
	}
	if *misalignedSize != 0xDEADBEEF {
		t.Fatalf("shape rejection wrote the size out-parameter %d despite failure", *misalignedSize)
	}
	assertWorldCanary(t, misalignedStorage[:int(wantBytes)+1], "misaligned pull")

	required, status := worldQuerySize(t, handle)
	if status != StatusInsufficientCapacity || required != wantBytes {
		t.Fatalf("query after rejections = (%d, %d), want (%d, StatusInsufficientCapacity) (rejections consumed nothing)",
			required, status, wantBytes)
	}
	if status := coreDestroy(idle); status != StatusOK {
		t.Fatalf("destroy idle = %d, want StatusOK", status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy online = %d, want StatusOK", status)
	}
}

// TestWorldBatchRejectsSizeOutParamAliasingWriteBuffer pins the overlap
// rule: the size out-parameter must not alias the caller-declared write
// buffer, the rejection writes nothing, and the batch stays consumable; an
// out-parameter just past the span is legal because the exact write never
// reaches it.
func TestWorldBatchRejectsSizeOutParamAliasingWriteBuffer(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	installWorldResultSeam(t, worldSeamResult(1, worldSeamMixedBatch(t, 3, 11), true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted world step = %d, want StatusOK", status)
	}
	wantBytes := uint32(worldBatchWireBytes(4, 8))

	for _, aliasOffset := range []uintptr{0, 8, uintptr(wantBytes - 4)} {
		storage := poisonedBuffer(int(wantBytes))
		aliasedSize := (*uint32)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), aliasOffset)))
		if status := coreWorldPull(handle, &storage[0], wantBytes, aliasedSize); status != StatusInvalidArgument {
			t.Fatalf("pull with the size word at buffer offset %d = %d, want StatusInvalidArgument", aliasOffset, status)
		}
		assertWorldCanary(t, storage, "aliased size-word pull")
	}

	storage := poisonedBuffer(int(wantBytes) + 8)
	outsideSize := (*uint32)(unsafe.Pointer(unsafe.Pointer(&storage[wantBytes])))
	if status := coreWorldPull(handle, &storage[0], wantBytes, outsideSize); status != StatusOK {
		t.Fatalf("pull with the size word just past the span = %d, want StatusOK", status)
	}
	if *outsideSize != uint32(0xA5A5A5A5) {
		t.Fatalf("successful pull wrote the size word %#x; success reports no size", *outsideSize)
	}
	for index := int(wantBytes); index < len(storage); index++ {
		if storage[index] != 0xA5 {
			t.Fatalf("successful pull touched byte %d beyond the required size", index)
		}
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the overlap walk = %d, want StatusOK", status)
	}
}

// TestWorldBatchTerminalSessionStillServesRetainedBatch pins the teardown
// ruling: retention survives disconnect, so pulling after the session reached
// its terminal state serves the last retained batch and consumes it normally,
// and the following pull observes the no-batch outcome.
func TestWorldBatchTerminalSessionStillServesRetainedBatch(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	installWorldResultSeam(t, worldSeamResult(1, worldSeamMixedBatch(t, 4, 15), true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted world step = %d, want StatusOK", status)
	}
	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("disconnect before the pull = %d, want StatusOK", status)
	}

	wantBytes := uint32(worldBatchWireBytes(4, 8))
	required, status := worldQuerySize(t, handle)
	if status != StatusInsufficientCapacity || required != wantBytes {
		t.Fatalf("terminal query = (%d, %d), want (%d, StatusInsufficientCapacity)", required, status, wantBytes)
	}
	storage, _, status := worldConsume(handle, wantBytes)
	if status != StatusOK {
		t.Fatalf("terminal consume = %d, want StatusOK", status)
	}
	if decoded := decodeWorldWire(t, storage); decoded.epoch != 4 || decoded.atlasRevision != 15 {
		t.Fatalf("terminal identity = epoch %d atlas %d, want 4 and 15", decoded.epoch, decoded.atlasRevision)
	}
	required, status = worldQuerySize(t, handle)
	if status != StatusOK || required != 0 {
		t.Fatalf("terminal query after consume = (%d, %d), want (0, StatusOK)", required, status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the terminal drain = %d, want StatusOK", status)
	}
}

// TestWorldBatchConcurrentConsumesSucceedExactlyOnce pins the exactly-once
// rule on one published batch: two concurrent consumers race one
// exact-capacity consume while a third goroutine only queries; exactly one
// consumer receives the record bytes, the loser observes the no-batch
// outcome with an untouched buffer, and no query ever reports a torn size.
func TestWorldBatchConcurrentConsumesSucceedExactlyOnce(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	installWorldResultSeam(t, worldSeamResult(1, worldSeamMixedBatch(t, 3, 11), true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted world step = %d, want StatusOK", status)
	}
	wantBytes := uint32(worldBatchWireBytes(4, 8))

	type consumeOutcome struct {
		status   Status
		required uint32
		storage  []byte
	}
	outcomes := make(chan consumeOutcome, 2)
	sizes := make(chan uint32, 64)
	var group sync.WaitGroup
	for worker := 0; worker < 2; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			storage, required, status := worldConsume(handle, wantBytes)
			outcomes <- consumeOutcome{status: status, required: required, storage: storage}
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		for query := 0; query < 32; query++ {
			required := uint32(0xDEADBEEF)
			switch status := coreWorldPull(handle, nil, 0, &required); status {
			case StatusInsufficientCapacity:
				sizes <- required
			case StatusOK:
				sizes <- 0
			default:
				t.Errorf("query status = %d, want StatusOK or StatusInsufficientCapacity", status)
				return
			}
		}
	}()
	group.Wait()
	close(outcomes)
	close(sizes)

	var winner []byte
	losers := 0
	for outcome := range outcomes {
		if outcome.status != StatusOK {
			t.Fatalf("concurrent consume status = %d, want StatusOK (the loser reads the no-batch outcome)", outcome.status)
		}
		if outcome.required == 0 {
			losers++
			assertWorldCanary(t, outcome.storage, "no-batch loser")
			continue
		}
		if outcome.required != uint32(0xDEADBEEF) {
			t.Fatalf("concurrent consume reported size %d; success and no-batch are the only outcomes", outcome.required)
		}
		if winner != nil {
			t.Fatal("both concurrent consumes received the batch")
		}
		winner = outcome.storage
	}
	if winner == nil || losers != 1 {
		t.Fatalf("exactly-once outcome = winner %v losers %d, want one winner and one loser", winner != nil, losers)
	}
	decodeWorldWire(t, winner)
	for size := range sizes {
		if size != 0 && size != wantBytes {
			t.Fatalf("query observed torn size %d, want 0 or %d", size, wantBytes)
		}
	}
	if required, status := worldQuerySize(t, handle); status != StatusOK || required != 0 {
		t.Fatalf("final query = (%d, %d), want (0, StatusOK)", required, status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the race = %d, want StatusOK", status)
	}
}

// TestWorldBatchConcurrentWithStepsStaysConsistent races consumers against a
// stepping goroutine publishing successive batches with unique epochs: every
// successful pull decodes to the exact expected record of exactly one
// scripted batch, no batch is consumed twice (distinct epochs across
// successes), and after the stepper finishes the consumers drain to the
// documented no-batch outcome.
func TestWorldBatchConcurrentWithStepsStaysConsistent(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)

	const steps = 8
	expected := make(map[uint64][]byte, steps)
	results := make([]clientruntime.StepResult, 0, steps)
	for index := 1; index <= steps; index++ {
		var batch presentation.WorldBatch
		if index%2 == 0 {
			batch = worldSeamMixedBatch(t, uint64(index), uint64(40+index))
		} else {
			batch = worldSeamDropBatch(t, uint64(index), uint64(40+index))
		}
		encoded := encodeWorldBatch(batch)
		if encoded == nil {
			t.Fatalf("fixture batch %d failed to encode", index)
		}
		expected[batch.Epoch] = encoded
		results = append(results, worldSeamResult(uint64(index), batch, true))
	}
	installWorldResultSeam(t, results...)

	stepped := make(chan struct{})
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		defer close(stepped)
		for index := 0; index < steps; index++ {
			if status := worldStepOnce(t, handle); status != StatusOK {
				t.Errorf("scripted step %d = %d, want StatusOK", index, status)
				return
			}
		}
	}()

	var seenMutex sync.Mutex
	seenEpochs := make(map[uint64]bool)
	for worker := 0; worker < 3; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < 100_000; iteration++ {
				required := uint32(0xDEADBEEF)
				switch status := coreWorldPull(handle, nil, 0, &required); status {
				case StatusInsufficientCapacity:
					storage, consumed, consumeStatus := worldConsume(handle, required)
					if consumeStatus == StatusInsufficientCapacity {
						// A newer, larger batch landed between the query and
						// the consume; the next iteration re-queries.
						continue
					}
					if consumeStatus != StatusOK {
						t.Errorf("consume status = %d, want StatusOK or StatusInsufficientCapacity", consumeStatus)
						return
					}
					if consumed == 0 {
						// Another consumer drained the batch first.
						continue
					}
					recordLength := worldRecordLength(t, storage)
					decoded := decodeWorldWire(t, storage[:recordLength])
					want, known := expected[decoded.epoch]
					if !known || !bytes.Equal(storage[:recordLength], want) {
						t.Errorf("consumed record does not match the scripted batch for epoch %d", decoded.epoch)
						return
					}
					seenMutex.Lock()
					if seenEpochs[decoded.epoch] {
						t.Errorf("batch epoch %d was consumed twice", decoded.epoch)
					}
					seenEpochs[decoded.epoch] = true
					seenMutex.Unlock()
				case StatusOK:
					// No batch is pullable right now; exit once the stepper
					// finished, because then nothing new can appear.
					select {
					case <-stepped:
						return
					default:
					}
				default:
					t.Errorf("query status = %d, want StatusOK or StatusInsufficientCapacity", status)
					return
				}
			}
		}()
	}
	group.Wait()

	seenMutex.Lock()
	consumedBatches := len(seenEpochs)
	seenMutex.Unlock()
	if consumedBatches == 0 {
		t.Fatal("no batch was consumed across the race")
	}
	if required, status := worldQuerySize(t, handle); status != StatusOK || required != 0 {
		t.Fatalf("final query = (%d, %d), want (0, StatusOK)", required, status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the race = %d, want StatusOK", status)
	}
}

// TestWorldBatchPanicConversionKeepsBatchConsumableWithoutOutput injects a
// panic through the encoder seam: the export converts it to `StatusPanic`,
// writes neither the buffer nor the size word, and leaves the batch
// consumable for a later successful drain.
func TestWorldBatchPanicConversionKeepsBatchConsumableWithoutOutput(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	installWorldResultSeam(t, worldSeamResult(1, worldSeamMixedBatch(t, 3, 11), true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted world step = %d, want StatusOK", status)
	}
	wantBytes := uint32(worldBatchWireBytes(4, 8))

	buffer, sizeOut := worldCallerFrame(wantBytes)
	*sizeOut = uint32(0xDEADBEEF)
	previous := worldBatchEncode
	worldBatchEncode = func(presentation.WorldBatch) []byte { panic("injected world encoder panic") }
	status := coreWorldPull(handle, &buffer[0], wantBytes, sizeOut)
	worldBatchEncode = previous
	if status != StatusPanic {
		t.Fatalf("pull during the injected panic = %d, want StatusPanic", status)
	}
	if *sizeOut != 0xDEADBEEF {
		t.Fatalf("panic wrote the size out-parameter %d", *sizeOut)
	}
	assertWorldCanary(t, buffer, "panicking pull")

	storage, _, status := worldConsume(handle, wantBytes)
	if status != StatusOK {
		t.Fatalf("consume after the converted panic = %d, want StatusOK (the panic consumed nothing)", status)
	}
	decodeWorldWire(t, storage)
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the panic walk = %d, want StatusOK", status)
	}
}

// TestWorldBatchInvalidRetainedBatchMapsToInternalWithoutConsume pins the
// defensive mapping: a retained world value that fails the presentation
// validators is a producer invariant break, reports `StatusInternal`, writes
// nothing, and consumes nothing.
func TestWorldBatchInvalidRetainedBatchMapsToInternalWithoutConsume(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	invalid := presentation.WorldBatch{Epoch: 0, AtlasRevision: presentation.AtlasRevision(1)}
	if err := invalid.Validate(); err == nil {
		t.Fatal("fixture batch unexpectedly validates")
	}
	installWorldResultSeam(t, worldSeamResult(1, invalid, true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted invalid-batch step = %d, want StatusOK", status)
	}

	storage, sizeOut := worldCallerFrame(64)
	*sizeOut = uint32(0xDEADBEEF)
	if status := coreWorldPull(handle, &storage[0], 64, sizeOut); status != StatusInternal {
		t.Fatalf("pull of an invalid retained batch = %d, want StatusInternal", status)
	}
	if *sizeOut != 0xDEADBEEF {
		t.Fatalf("internal failure wrote the size out-parameter %d", *sizeOut)
	}
	assertWorldCanary(t, storage, "internal-failure pull")

	required := uint32(0xDEADBEEF)
	if status := coreWorldPull(handle, nil, 0, &required); status != StatusInternal {
		t.Fatalf("query of an invalid retained batch = %d, want StatusInternal (nothing was consumed)", status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the internal mapping = %d, want StatusOK", status)
	}
}
