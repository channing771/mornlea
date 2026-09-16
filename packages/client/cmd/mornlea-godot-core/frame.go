//go:build cgo

package main

import (
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/channing771/mornlea/packages/client/presentation"
)

// This file owns the frame family export surface: the two-phase pull of the
// frame snapshot retained by the step family (see step.go for the retention
// ruling). Unlike the world batch, a frame pull is deliberately NON-consuming:
// a world batch is a queue element whose generation-guarded consumption
// commits exactly once, while a frame is presentational state the host may
// read repeatedly between steps — the rendered scene, HUD, and target feedback
// of one revision stay valid until the next step publishes a newer snapshot.
// A pull therefore only snapshots the retained value and encodes it; there is
// no commit step, no generation bookkeeping, and no per-family pull mutex,
// because concurrent pulls of an immutable value cannot tear it and only a new
// step replaces the served snapshot (latest-completed-step wins, mirroring the
// world family's epoch ruling).

// Producer-side frame wire vocabulary. The frozen include/
// mornlea_client_core.h defines the MCF1 header, the entity and name limits,
// and the snapshot version; the body record layout below is defined here by
// the producer, like the world operation layout in world.go, and will be
// pinned by the Rust consumer when its frame bridge lands. Every record size
// is a multiple of `ABIAlignment` so a consumer may append records without
// re-alignment. The registry descriptor for the frame family deliberately
// keeps `RecordBytes` at zero in this generation because both language pin
// suites pin that table; switching the descriptor to the entity record size is
// a coordinated cross-language update owned by the consumer task.
//
// Body record sequence after the 40-byte header, in little-endian words:
//
//	camera record, 40 bytes — provenance `presentation.CameraSnapshot`:
//	  offset 0   ready     u32   1 when the camera is ready, else 0
//	  offset 4   pos_x     f32   `CameraSnapshot.Position[0]` in Mornlea units
//	  offset 8   pos_y     f32   `CameraSnapshot.Position[1]`
//	  offset 12  pos_z     f32   `CameraSnapshot.Position[2]`
//	  offset 16  yaw       f32   `CameraSnapshot.Yaw`, unbounded accumulated
//	  offset 20  pitch     f32   `CameraSnapshot.Pitch`
//	  offset 24  fov_y     f32   `CameraSnapshot.FOVY`, vertical field of view
//	  offset 28  aspect    f32   `CameraSnapshot.Aspect`
//	  offset 32  near      f32   `CameraSnapshot.Near` clipping plane
//	  offset 36  far       f32   `CameraSnapshot.Far` clipping plane
//
//	HUD record, 16 bytes — provenance `presentation.HUDSnapshot`:
//	  offset 0   ready     u32   1 when the HUD is ready, else 0
//	  offset 4   health    u32   `HUDSnapshot.Health` widened from u8
//	  offset 8   hunger    u32   `HUDSnapshot.Hunger` widened from u8
//	  offset 12  oxygen    u32   `HUDSnapshot.Oxygen` widened from u16
//
//	environment record, 48 bytes — provenance `presentation.EnvironmentSnapshot`:
//	  offset 0   ready          u32   1 when environment is confirmed
//	  offset 4   reserved       u32   zero
//	  offset 8   server_tick    u64   `EnvironmentSnapshot.ServerTick`
//	  offset 16  world_ticks    u64   `EnvironmentSnapshot.WorldTimeTicks`
//	  offset 24  day_offset     u32   `EnvironmentSnapshot.DayPhaseOffset`
//	  offset 28  weather        u32   `core.WeatherKind`
//	  offset 32  season         u32   `core.Season`
//	  offset 36  season_steps   u32   `EnvironmentSnapshot.SeasonProgress`
//	  offset 40  temperature    i32   `EnvironmentSnapshot.Temperature`
//	  offset 44  reserved       u32   zero
//
//	entity-batch tick record, 8 bytes — provenance `presentation.EntityBatch`:
//	  offset 0   server_tick    u64   `EntityBatch.ServerTick` shared by every
//	                                  entity record that follows
//
//	entity records, 48 bytes each, `EntityBatch` order — provenance
//	`presentation.EntityRecord`:
//	  offset 0   kind       u32   `EntityKindRemotePlayer` is 1
//	  offset 4   reserved   u32   zero
//	  offset 8   player_id  16B   raw `core.PlayerID` UUIDv4 bytes
//	  offset 24  dimension  u32   `core.DimensionID`, 0 Overworld 1 Depths
//	  offset 28  pos_x      f32   `EntityRecord.Position[0]`
//	  offset 32  pos_y      f32   `EntityRecord.Position[1]`
//	  offset 36  pos_z      f32   `EntityRecord.Position[2]`
//	  offset 40  yaw        f32   `EntityRecord.Yaw`
//	  offset 44  pitch      f32   `EntityRecord.Pitch`
//
//	target record, 16 bytes — provenance `presentation.TargetSnapshot`:
//	  offset 0   visible    u32   1 when a target is locked, else 0
//	  offset 4   x          i32   `TargetSnapshot.Position.X`
//	  offset 8   y          i32   `TargetSnapshot.Position.Y`
//	  offset 12  z          i32   `TargetSnapshot.Position.Z`
//
//	phase/error record, 8 bytes — provenance `presentation.FrameSnapshot`:
//	  offset 0   phase      u32   `presentation.SessionPhase`, the same word
//	                              vocabulary the connect family reports
//	  offset 4   error      u32   `presentation.ErrorCode`; the presentation
//	                              contract couples a present error to the
//	                              disconnected phase
//
//	target-name payload: name_len raw UTF-8 bytes of
//	`TargetSnapshot.Name` at the record tail; a hidden target carries none.
//
// Readiness ruling: the presentation validators guarantee a not-ready camera,
// HUD, or environment is exactly the zero value, so its record serializes the
// zero ready word plus zero payload fields — the producer never fabricates
// presentational values for unconfirmed state. The maximal whole-record size
// is 576 bytes (header plus seven entity records plus the 64-byte name), the
// consumer's single allocation ceiling for a maximal frame.
const (
	// FrameCameraRecordBytes is the fixed camera record size.
	FrameCameraRecordBytes = 40
	// FrameHUDRecordBytes is the fixed HUD record size.
	FrameHUDRecordBytes = 16
	// FrameEnvironmentRecordBytes is the fixed environment record size.
	FrameEnvironmentRecordBytes = 48
	// FrameEntityTickRecordBytes is the fixed entity-batch tick record size.
	FrameEntityTickRecordBytes = 8
	// FrameEntityRecordBytes is the fixed per-entity record size.
	FrameEntityRecordBytes = 48
	// FrameTargetRecordBytes is the fixed target record size.
	FrameTargetRecordBytes = 16
	// FramePhaseErrorRecordBytes is the fixed phase/error record size.
	FramePhaseErrorRecordBytes = 8
)

// frameWireBytes returns the exact wire size of one frame encoding: the frozen
// header, every fixed record, one entity record per entity, and the target
// name payload. The presentation validator bounds the entity count; the name
// bound is the encoder's own guard, so the result always fits the
// `MaxEntityRecords` and `MaxTargetNameBytes` budget.
func frameWireBytes(entities, nameLen int) int {
	return int(FrameHeaderBytes) + int(FrameCameraRecordBytes) + int(FrameHUDRecordBytes) +
		int(FrameEnvironmentRecordBytes) + int(FrameEntityTickRecordBytes) +
		entities*int(FrameEntityRecordBytes) + int(FrameTargetRecordBytes) +
		int(FramePhaseErrorRecordBytes) + nameLen
}

// frameEncode produces the wire record of one frame snapshot. It is a variable
// so tests can inject a panic through the seam and prove the export boundary
// converts it to `StatusPanic` without writing output; it is never redefined
// outside tests.
var frameEncode = encodeFrameSnapshot

// encodeFrameSnapshot builds the wire record: the MCF1 header (magic, layout,
// snapshot version, entity count, name length, zero reserved, revision,
// epoch) followed by the fixed camera, HUD, environment, and entity-batch
// tick records, one record per entity in presentation order, the target
// record, the phase/error record, and the target-name payload at the tail.
// The function is a pure function of the snapshot value and the frozen
// constants: it never consults the family registry, the session, or any other
// producer state. That purity is the optional-family contract — a later
// registry growth (new families, bumped compatible versions) cannot change
// what a v1 consumer decodes, and the golden-byte pins in frame_test.go turn
// any divergence into a test failure. The snapshot is revalidated first
// because the presentation validators own the bounds the encoding inherits
// (entity capacity, readiness zeroing, terminal-error coupling); the target
// name bound is this encoder's own guard, and a retained value that fails
// either violates a producer invariant and yields
// nil, which callers map to `StatusInternal` rather than publishing a partial
// record.
func encodeFrameSnapshot(frame presentation.FrameSnapshot) []byte {
	if err := frame.Validate(); err != nil {
		return nil
	}
	entities := frame.Entities.Records()
	name := frame.Target.Name
	if len(entities) > int(MaxEntityRecords) || len(name) > int(MaxTargetNameBytes) {
		return nil
	}
	record := make([]byte, frameWireBytes(len(entities), len(name)))
	binary.LittleEndian.PutUint32(record[0:4], uint32(MagicFrame))
	binary.LittleEndian.PutUint32(record[4:8], FrameVersion)
	binary.LittleEndian.PutUint32(record[8:12], FrameSnapshotVersion)
	binary.LittleEndian.PutUint32(record[12:16], uint32(len(entities)))
	binary.LittleEndian.PutUint32(record[16:20], uint32(len(name)))
	// The reserved word at record[20:24] stays zero because the record is
	// freshly allocated.
	binary.LittleEndian.PutUint64(record[24:32], frame.Revision)
	binary.LittleEndian.PutUint64(record[32:40], frame.Epoch)
	offset := int(FrameHeaderBytes)
	framePutCamera(record[offset:offset+FrameCameraRecordBytes], frame.Camera)
	offset += int(FrameCameraRecordBytes)
	framePutHUD(record[offset:offset+FrameHUDRecordBytes], frame.HUD)
	offset += int(FrameHUDRecordBytes)
	framePutEnvironment(record[offset:offset+FrameEnvironmentRecordBytes], frame.Environment)
	offset += int(FrameEnvironmentRecordBytes)
	binary.LittleEndian.PutUint64(record[offset:offset+int(FrameEntityTickRecordBytes)], frame.Entities.ServerTick)
	offset += int(FrameEntityTickRecordBytes)
	for _, entity := range entities {
		framePutEntity(record[offset:offset+FrameEntityRecordBytes], entity)
		offset += int(FrameEntityRecordBytes)
	}
	framePutTarget(record[offset:offset+FrameTargetRecordBytes], frame.Target)
	offset += int(FrameTargetRecordBytes)
	binary.LittleEndian.PutUint32(record[offset:offset+4], uint32(frame.Phase))
	binary.LittleEndian.PutUint32(record[offset+4:offset+8], uint32(frame.Error.Code))
	offset += int(FramePhaseErrorRecordBytes)
	copy(record[offset:], name)
	return record
}

// framePutReadyWord writes one readiness word: one when ready, zero otherwise.
// A zero word must stay zero, so the false branch writes nothing.
func framePutReadyWord(word []byte, ready bool) {
	if ready {
		binary.LittleEndian.PutUint32(word, 1)
	}
}

// framePutFloat writes one IEEE-754 float32 word; the presentation validators
// reject non-finite values before the encoder runs.
func framePutFloat(word []byte, value float32) {
	binary.LittleEndian.PutUint32(word, math.Float32bits(value))
}

// framePutCamera writes the 40-byte camera record from its presentation
// fields; a not-ready camera is the validated zero value, so every payload
// word writes zero naturally.
func framePutCamera(record []byte, camera presentation.CameraSnapshot) {
	framePutReadyWord(record[0:4], camera.Ready)
	framePutFloat(record[4:8], camera.Position[0])
	framePutFloat(record[8:12], camera.Position[1])
	framePutFloat(record[12:16], camera.Position[2])
	framePutFloat(record[16:20], camera.Yaw)
	framePutFloat(record[20:24], camera.Pitch)
	framePutFloat(record[24:28], camera.FOVY)
	framePutFloat(record[28:32], camera.Aspect)
	framePutFloat(record[32:36], camera.Near)
	framePutFloat(record[36:40], camera.Far)
}

// framePutHUD writes the 16-byte HUD record; the u8 and u16 fields widen to
// u32 words so every record keeps uniform word granularity.
func framePutHUD(record []byte, hud presentation.HUDSnapshot) {
	framePutReadyWord(record[0:4], hud.Ready)
	binary.LittleEndian.PutUint32(record[4:8], uint32(hud.Health))
	binary.LittleEndian.PutUint32(record[8:12], uint32(hud.Hunger))
	binary.LittleEndian.PutUint32(record[12:16], uint32(hud.Oxygen))
}

// framePutEnvironment writes the 48-byte environment record; the two reserved
// words stay zero because the record slice is freshly allocated.
func framePutEnvironment(record []byte, environment presentation.EnvironmentSnapshot) {
	framePutReadyWord(record[0:4], environment.Ready)
	binary.LittleEndian.PutUint64(record[8:16], environment.ServerTick)
	binary.LittleEndian.PutUint64(record[16:24], environment.WorldTimeTicks)
	binary.LittleEndian.PutUint32(record[24:28], uint32(environment.DayPhaseOffset))
	binary.LittleEndian.PutUint32(record[28:32], uint32(environment.Weather))
	binary.LittleEndian.PutUint32(record[32:36], uint32(environment.Season))
	binary.LittleEndian.PutUint32(record[36:40], uint32(environment.SeasonProgress))
	binary.LittleEndian.PutUint32(record[40:44], uint32(int32(environment.Temperature)))
}

// framePutEntity writes one 48-byte entity record; the player identity keeps
// its raw 16 UUIDv4 bytes and the signed dimension widens to its u32 code.
func framePutEntity(record []byte, entity presentation.EntityRecord) {
	binary.LittleEndian.PutUint32(record[0:4], uint32(entity.Kind))
	copy(record[8:24], entity.PlayerID[:])
	binary.LittleEndian.PutUint32(record[24:28], uint32(entity.Dimension))
	framePutFloat(record[28:32], entity.Position[0])
	framePutFloat(record[32:36], entity.Position[1])
	framePutFloat(record[36:40], entity.Position[2])
	framePutFloat(record[40:44], entity.Yaw)
	framePutFloat(record[44:48], entity.Pitch)
}

// framePutTarget writes the 16-byte target record; a hidden target is the
// validated zero value, so every word writes zero naturally.
func framePutTarget(record []byte, target presentation.TargetSnapshot) {
	framePutReadyWord(record[0:4], target.Visible)
	binary.LittleEndian.PutUint32(record[4:8], uint32(target.Position.X))
	binary.LittleEndian.PutUint32(record[8:12], uint32(target.Position.Y))
	binary.LittleEndian.PutUint32(record[12:16], uint32(target.Position.Z))
}

// framePullSnapshot is one consistent view of the frame the pull family may
// serve. The snapshot value shares immutable backing arrays with the retained
// step result (the runtime builds owned entity batches and retention replaces
// the whole value), so it stays valid across later steps without copying
// here.
type framePullSnapshot struct {
	frame     presentation.FrameSnapshot
	available bool
}

// framePullSnapshot captures the pullable frame under the session mutex. A
// frame is available exactly when a step result was retained, whatever its
// phase: the retention ruling keeps the terminal frame pullable after a
// disconnect, so availability deliberately ignores the connection state.
func (session *clientSession) framePullSnapshot() framePullSnapshot {
	session.mu.Lock()
	defer session.mu.Unlock()
	snapshot := framePullSnapshot{}
	if session.hasStepResult {
		snapshot.frame = session.stepResult.Frame
		snapshot.available = true
	}
	return snapshot
}

// pullFrame serves the retained frame through the two-phase capacity
// protocol. Outcomes:
//   - No retained frame (no step completed, including a session that reached
//     its terminal state before any step): `StatusOK` and a zero size word,
//     writing no buffer byte. Size zero is unambiguous because the frozen
//     header alone is 40 bytes.
//   - Encoder rejection of the retained value: `StatusInternal` with no
//     output.
//   - Capacity below the encoded size: `StatusInsufficientCapacity` and the
//     fresh required size in the size word; no buffer byte is written.
//   - Sufficient capacity: the whole record is encoded into owned scratch and
//     copied once, exactly. Nothing is consumed: repeated pulls return
//     identical bytes until a later step retains a newer snapshot, whose own
//     bytes may be larger or smaller, so a size queried before a newer step is
//     answered with the fresh size like the world family's epoch ruling.
//
// The pull reads retained presentation state, not the live connection, so it
// never reports `StatusDisconnected`: a terminal session still pulls its
// terminal frame with `StatusOK` (retention survives teardown, see step.go);
// only a destroyed handle is rejected with `StatusInvalidState`.
func (session *clientSession) pullFrame(out *byte, capacity uint32, requiredOut *uint32) Status {
	snapshot := session.framePullSnapshot()
	if !snapshot.available {
		*requiredOut = 0
		return StatusOK
	}
	record := frameEncode(snapshot.frame)
	if record == nil {
		return StatusInternal
	}
	if uint32(len(record)) > capacity {
		*requiredOut = uint32(len(record))
		return StatusInsufficientCapacity
	}
	copy(unsafe.Slice(out, len(record)), record)
	return StatusOK
}

// coreFramePull serves the frame snapshot retained by the step family through
// the two-phase capacity protocol. It is the testable core behind the exported
// frame pull symbol; the raw pointer arguments mirror the C surface. A
// zero-capacity call is the required-size query; a positive capacity below the
// required size is the capacity signal; a sufficient capacity performs the one
// exact write. The pull is non-consuming (see pullFrame), so queries and
// consumes are freely repeatable between steps.
//
// Overlap ruling: this export holds exactly two caller pointers, the write
// buffer and the size out-parameter. A caller buffer cannot overlap
// producer-retained memory because the producer encodes into Go-heap scratch
// that no C pointer can reach. The one aliasing hazard inside the export is
// the size word landing inside the caller-declared buffer span: the size
// write would corrupt the record mid-protocol (or the copy would clobber the
// size word), so a size out-parameter pointing anywhere inside the write
// buffer's declared capacity span is rejected with `StatusInvalidArgument`,
// mirroring the engine ABI's pairwise-overlap discipline.
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
//  3. Content: the non-consuming two-phase serve above (snapshot, encode,
//     capacity, one exact write).
//
// A recovered panic reports `StatusPanic` with no output byte; the guarded
// path writes caller memory only as its final successful step, so a panic can
// only fire before any write. The caller's pointers are never retained after
// the call returns.
func coreFramePull(handle uint64, out *byte, capacity uint32, requiredOut *uint32) Status {
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
		return session.pullFrame(out, capacity, requiredOut)
	})
}
