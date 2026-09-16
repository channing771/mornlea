package main

import (
	"encoding/binary"
	"math"
	"slices"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/channing771/mornlea/packages/client/presentation"
	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
)

// The frame tests pin the frame family's two-phase pull: the MCF1 header plus
// the fixed camera, HUD, environment, entity-batch-tick, entity, target, and
// phase/error records with the variable target-name payload at the tail. The
// pull is deliberately NON-consuming — a frame is presentational state the host
// may read repeatedly between steps, unlike a world batch — so repeated pulls
// return identical bytes and only a new step replaces the served snapshot.
// Roundtrip fidelity is pinned twice: once by decoding every field against the
// presentation value that produced it, and once against an independent
// reference encoder written from the documented layout, which is also the
// optional-family purity proof: v1 frame bytes are a pure function of the
// snapshot and never of the family registry. Online sessions come from the
// scripted transport and frames arrive through the `stepRuntime` seam, so no
// test opens a socket.

// framePlayerID builds one valid UUIDv4 player identity distinct per seed.
func framePlayerID(seed byte) core.PlayerID {
	id := core.PlayerID{seed, 2, 3, 4, 5, 6, 0x4a, 0x6b, 0x8c, 0x7d, 10, 11, 12, 13, 14, seed}
	if !id.Valid() {
		panic("frame test player ID is not a valid UUIDv4")
	}
	return id
}

// frameEntityBatch builds a validated entity batch of count distinct records.
func frameEntityBatch(t *testing.T, serverTick uint64, count int) presentation.EntityBatch {
	t.Helper()
	records := make([]presentation.EntityRecord, 0, count)
	for index := 0; index < count; index++ {
		records = append(records, presentation.EntityRecord{
			Kind:      presentation.EntityKindRemotePlayer,
			PlayerID:  framePlayerID(byte(index + 1)),
			Dimension: core.DimensionID(index % 2),
			Position:  [3]float32{float32(index) + 0.5, 64 - float32(index), -2.25},
			Yaw:       float32(index)*0.5 - 1,
			Pitch:     0.1*float32(index) - 0.3,
		})
	}
	batch, err := presentation.NewEntityBatch(serverTick, records)
	if err != nil {
		t.Fatalf("build entity batch of %d records: %v", count, err)
	}
	return batch
}

// frameRichSnapshot builds the canonical rich fixture: a ready camera,
// environment, and HUD, three entities at one server tick, and a visible named
// target.
func frameRichSnapshot(t *testing.T, revision uint64) presentation.FrameSnapshot {
	t.Helper()
	frame := presentation.FrameSnapshot{
		Version:  presentation.FrameSnapshotVersion,
		Revision: revision,
		Epoch:    9,
		Phase:    clientruntime.ConnectionPhasePlay,
		Camera: presentation.CameraSnapshot{
			Ready:    true,
			Position: [3]float32{12.5, 64.25, -8.75},
			Yaw:      -2.25,
			Pitch:    0.4,
			FOVY:     1.2,
			Aspect:   16.0 / 9.0,
			Near:     0.05,
			Far:      512,
		},
		HUD: presentation.HUDSnapshot{Ready: true, Health: 15, Hunger: 12, Oxygen: 240},
		Environment: presentation.EnvironmentSnapshot{
			Ready: true, ServerTick: 4242, WorldTimeTicks: 98_765, DayPhaseOffset: 321,
			Weather: core.WeatherRain, Season: core.SeasonAutumn, SeasonProgress: 91, Temperature: -7,
		},
		Entities: frameEntityBatch(t, 4242, 3),
		Target:   presentation.TargetSnapshot{Visible: true, Position: core.BlockPos{X: 3, Y: 64, Z: -5}, Name: "stone"},
	}
	if err := frame.Validate(); err != nil {
		t.Fatalf("rich fixture does not validate: %v", err)
	}
	return frame
}

// frameNotReadySnapshot builds the all-not-ready fixture: camera, HUD, and
// environment not ready (zero payloads by the presentation contract), no
// entities, and a hidden target with no name.
func frameNotReadySnapshot(revision uint64) presentation.FrameSnapshot {
	return presentation.FrameSnapshot{
		Version:  presentation.FrameSnapshotVersion,
		Revision: revision,
		Epoch:    2,
		Phase:    clientruntime.ConnectionPhaseLoading,
	}
}

// frameTerminalSnapshot builds the terminal fixture: the disconnected phase
// with its coupled error state and no presentational payloads.
func frameTerminalSnapshot(revision uint64) presentation.FrameSnapshot {
	return presentation.FrameSnapshot{
		Version:  presentation.FrameSnapshotVersion,
		Revision: revision,
		Epoch:    4,
		Phase:    clientruntime.ConnectionPhaseDisconnected,
		Error:    presentation.ErrorState{Code: presentation.ErrorConnection},
	}
}

// frameQuerySize performs the zero-capacity two-phase size query.
func frameQuerySize(t *testing.T, handle uint64) (uint32, Status) {
	t.Helper()
	required := uint32(0xDEADBEEF)
	status := coreFramePull(handle, nil, 0, &required)
	return required, status
}

// frameCallerFrame models one C caller's frame for the pull with the same
// discipline as the world tests: the write buffer at the start of one
// caller-owned allocation and the size word at an aligned offset beyond every
// declared span, so the two pointers can never drift into accidental numeric
// overlap with unrelated Go allocations.
func frameCallerFrame(capacity uint32) (buffer []byte, sizeOut *uint32) {
	storage := poisonedBuffer(int(capacity) + 40)
	buffer = storage[:int(capacity):int(capacity)]
	sizeOffset := (int(capacity)+7)&^7 + 32
	return buffer, (*uint32)(unsafe.Pointer(&storage[sizeOffset]))
}

// frameConsume performs one pull into a fresh caller frame of the declared
// capacity, returning the buffer view, the reported size word, and the status.
// The capacity must be positive so the buffer has an address.
func frameConsume(handle uint64, capacity uint32) (buffer []byte, required uint32, status Status) {
	buffer, sizeOut := frameCallerFrame(capacity)
	*sizeOut = uint32(0xDEADBEEF)
	status = coreFramePull(handle, &buffer[0], capacity, sizeOut)
	return buffer, *sizeOut, status
}

// frameServeThroughSession drives scripted frames through the full export path
// — create, connect online, one step per frame, one pull per frame — and
// returns the served byte slices in script order.
func frameServeThroughSession(t *testing.T, frames ...presentation.FrameSnapshot) [][]byte {
	t.Helper()
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	results := make([]clientruntime.StepResult, 0, len(frames))
	for _, frame := range frames {
		results = append(results, clientruntime.StepResult{Frame: frame})
	}
	installWorldResultSeam(t, results...)
	served := make([][]byte, 0, len(frames))
	for index := range frames {
		if status := worldStepOnce(t, handle); status != StatusOK {
			t.Fatalf("scripted frame step %d = %d, want StatusOK", index, status)
		}
		required, status := frameQuerySize(t, handle)
		if status != StatusInsufficientCapacity || required == 0 {
			t.Fatalf("frame query %d = (%d, %d), want a positive size and StatusInsufficientCapacity", index, required, status)
		}
		storage, _, status := frameConsume(handle, required)
		if status != StatusOK {
			t.Fatalf("frame consume %d = %d, want StatusOK", index, status)
		}
		served = append(served, storage)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after serving frames = %d, want StatusOK", status)
	}
	return served
}

// frameWireEntity is the decoded view of one 48-byte entity record.
type frameWireEntity struct {
	kind      uint32
	playerID  [16]byte
	dimension uint32
	position  [3]float32
	yaw       float32
	pitch     float32
}

// frameWire is the decoded view of one complete frame record.
type frameWire struct {
	frameVersion   uint32
	entityCount    uint32
	nameLen        uint32
	revision       uint64
	epoch          uint64
	camera         presentation.CameraSnapshot
	cameraReady    uint32
	hud            presentation.HUDSnapshot
	hudReady       uint32
	environment    presentation.EnvironmentSnapshot
	environmentRdy uint32
	entityTick     uint64
	entities       []frameWireEntity
	targetVisible  uint32
	targetPos      core.BlockPos
	phase          uint32
	errorCode      uint32
	name           string
}

// decodeFrameWire decodes one complete record, pinning the header identity and
// the count-to-length arithmetic before returning every field.
func decodeFrameWire(t *testing.T, buffer []byte) frameWire {
	t.Helper()
	if len(buffer) < int(FrameHeaderBytes) {
		t.Fatalf("record of %d bytes is shorter than the header", len(buffer))
	}
	if magic := binary.LittleEndian.Uint32(buffer[0:4]); magic != uint32(MagicFrame) {
		t.Fatalf("header magic = %#x, want frame magic %#x", magic, uint32(MagicFrame))
	}
	if layout := binary.LittleEndian.Uint32(buffer[4:8]); layout != FrameVersion {
		t.Fatalf("header layout = %d, want %d", layout, FrameVersion)
	}
	decoded := frameWire{
		frameVersion: binary.LittleEndian.Uint32(buffer[8:12]),
		entityCount:  binary.LittleEndian.Uint32(buffer[12:16]),
		nameLen:      binary.LittleEndian.Uint32(buffer[16:20]),
		revision:     binary.LittleEndian.Uint64(buffer[24:32]),
		epoch:        binary.LittleEndian.Uint64(buffer[32:40]),
	}
	if decoded.frameVersion != FrameSnapshotVersion {
		t.Fatalf("frame version = %d, want %d", decoded.frameVersion, FrameSnapshotVersion)
	}
	if reserved := binary.LittleEndian.Uint32(buffer[20:24]); reserved != 0 {
		t.Fatalf("header reserved = %d, want zero", reserved)
	}
	if decoded.entityCount > MaxEntityRecords {
		t.Fatalf("entity count %d exceeds the frozen limit %d", decoded.entityCount, MaxEntityRecords)
	}
	if decoded.nameLen > MaxTargetNameBytes {
		t.Fatalf("name length %d exceeds the frozen limit %d", decoded.nameLen, MaxTargetNameBytes)
	}
	wantBytes := frameWireBytes(int(decoded.entityCount), int(decoded.nameLen))
	if len(buffer) != wantBytes {
		t.Fatalf("record length %d does not match counts (%d entities, %d name bytes => %d bytes)",
			len(buffer), decoded.entityCount, decoded.nameLen, wantBytes)
	}
	offset := int(FrameHeaderBytes)
	camera := buffer[offset : offset+int(FrameCameraRecordBytes)]
	decoded.cameraReady = binary.LittleEndian.Uint32(camera[0:4])
	decoded.camera.Ready = decoded.cameraReady == 1
	decoded.camera.Position = [3]float32{
		math.Float32frombits(binary.LittleEndian.Uint32(camera[4:8])),
		math.Float32frombits(binary.LittleEndian.Uint32(camera[8:12])),
		math.Float32frombits(binary.LittleEndian.Uint32(camera[12:16])),
	}
	decoded.camera.Yaw = math.Float32frombits(binary.LittleEndian.Uint32(camera[16:20]))
	decoded.camera.Pitch = math.Float32frombits(binary.LittleEndian.Uint32(camera[20:24]))
	decoded.camera.FOVY = math.Float32frombits(binary.LittleEndian.Uint32(camera[24:28]))
	decoded.camera.Aspect = math.Float32frombits(binary.LittleEndian.Uint32(camera[28:32]))
	decoded.camera.Near = math.Float32frombits(binary.LittleEndian.Uint32(camera[32:36]))
	decoded.camera.Far = math.Float32frombits(binary.LittleEndian.Uint32(camera[36:40]))
	offset += int(FrameCameraRecordBytes)
	hud := buffer[offset : offset+int(FrameHUDRecordBytes)]
	decoded.hudReady = binary.LittleEndian.Uint32(hud[0:4])
	decoded.hud.Ready = decoded.hudReady == 1
	decoded.hud.Health = uint8(binary.LittleEndian.Uint32(hud[4:8]))
	decoded.hud.Hunger = uint8(binary.LittleEndian.Uint32(hud[8:12]))
	decoded.hud.Oxygen = uint16(binary.LittleEndian.Uint32(hud[12:16]))
	offset += int(FrameHUDRecordBytes)
	environment := buffer[offset : offset+int(FrameEnvironmentRecordBytes)]
	decoded.environmentRdy = binary.LittleEndian.Uint32(environment[0:4])
	decoded.environment.Ready = decoded.environmentRdy == 1
	decoded.environment.ServerTick = binary.LittleEndian.Uint64(environment[8:16])
	decoded.environment.WorldTimeTicks = binary.LittleEndian.Uint64(environment[16:24])
	decoded.environment.DayPhaseOffset = uint16(binary.LittleEndian.Uint32(environment[24:28]))
	decoded.environment.Weather = core.WeatherKind(binary.LittleEndian.Uint32(environment[28:32]))
	decoded.environment.Season = core.Season(binary.LittleEndian.Uint32(environment[32:36]))
	decoded.environment.SeasonProgress = uint8(binary.LittleEndian.Uint32(environment[36:40]))
	decoded.environment.Temperature = int8(int32(binary.LittleEndian.Uint32(environment[40:44])))
	if reserved := binary.LittleEndian.Uint32(environment[4:8]); reserved != 0 {
		t.Fatalf("environment reserved word = %d, want zero", reserved)
	}
	if reserved := binary.LittleEndian.Uint32(environment[44:48]); reserved != 0 {
		t.Fatalf("environment tail reserved word = %d, want zero", reserved)
	}
	offset += int(FrameEnvironmentRecordBytes)
	decoded.entityTick = binary.LittleEndian.Uint64(buffer[offset : offset+int(FrameEntityTickRecordBytes)])
	offset += int(FrameEntityTickRecordBytes)
	decoded.entities = make([]frameWireEntity, decoded.entityCount)
	for index := range decoded.entities {
		entity := buffer[offset : offset+int(FrameEntityRecordBytes)]
		decoded.entities[index] = frameWireEntity{
			kind:      binary.LittleEndian.Uint32(entity[0:4]),
			playerID:  [16]byte(entity[8:24]),
			dimension: binary.LittleEndian.Uint32(entity[24:28]),
			position: [3]float32{
				math.Float32frombits(binary.LittleEndian.Uint32(entity[28:32])),
				math.Float32frombits(binary.LittleEndian.Uint32(entity[32:36])),
				math.Float32frombits(binary.LittleEndian.Uint32(entity[36:40])),
			},
			yaw:   math.Float32frombits(binary.LittleEndian.Uint32(entity[40:44])),
			pitch: math.Float32frombits(binary.LittleEndian.Uint32(entity[44:48])),
		}
		if reserved := binary.LittleEndian.Uint32(entity[4:8]); reserved != 0 {
			t.Fatalf("entity %d reserved word = %d, want zero", index, reserved)
		}
		offset += int(FrameEntityRecordBytes)
	}
	target := buffer[offset : offset+int(FrameTargetRecordBytes)]
	decoded.targetVisible = binary.LittleEndian.Uint32(target[0:4])
	decoded.targetPos = core.BlockPos{
		X: int32(binary.LittleEndian.Uint32(target[4:8])),
		Y: int32(binary.LittleEndian.Uint32(target[8:12])),
		Z: int32(binary.LittleEndian.Uint32(target[12:16])),
	}
	offset += int(FrameTargetRecordBytes)
	decoded.phase = binary.LittleEndian.Uint32(buffer[offset : offset+4])
	decoded.errorCode = binary.LittleEndian.Uint32(buffer[offset+4 : offset+8])
	offset += int(FramePhaseErrorRecordBytes)
	decoded.name = string(buffer[offset : offset+int(decoded.nameLen)])
	return decoded
}

// frameRecordLength reads a served buffer's header counts and returns the
// exact encoded length, so a test can trim a capacity-padded buffer back to
// the record the pull wrote.
func frameRecordLength(buffer []byte) int {
	entities := binary.LittleEndian.Uint32(buffer[12:16])
	nameLen := binary.LittleEndian.Uint32(buffer[16:20])
	return frameWireBytes(int(entities), int(nameLen))
}

// frameReferenceBytes encodes the documented v1 frame layout independently of
// the production encoder: every offset and size below is a literal transcribed
// from the layout comment in frame.go, not a reference to production
// constants. It exists so the pinned bytes are produced by a second
// implementation; see the optional-family purity proof.
func frameReferenceBytes(t *testing.T, frame presentation.FrameSnapshot) []byte {
	t.Helper()
	if err := frame.Validate(); err != nil {
		t.Fatalf("reference fixture does not validate: %v", err)
	}
	entities := frame.Entities.Records()
	name := frame.Target.Name
	buffer := make([]byte, 176+48*len(entities)+len(name))
	word := func(offset int, value uint32) { binary.LittleEndian.PutUint32(buffer[offset:offset+4], value) }
	bits := func(offset int, value float32) { word(offset, math.Float32bits(value)) }
	double := func(offset int, value uint64) { binary.LittleEndian.PutUint64(buffer[offset:offset+8], value) }
	flag := func(offset int, ready bool) {
		if ready {
			word(offset, 1)
		}
	}
	word(0, 0x3146434D) // "MCF1"
	word(4, 1)          // family layout
	word(8, 1)          // frame snapshot version
	word(12, uint32(len(entities)))
	word(16, uint32(len(name)))
	double(24, frame.Revision)
	double(32, frame.Epoch)
	flag(40, frame.Camera.Ready)
	bits(44, frame.Camera.Position[0])
	bits(48, frame.Camera.Position[1])
	bits(52, frame.Camera.Position[2])
	bits(56, frame.Camera.Yaw)
	bits(60, frame.Camera.Pitch)
	bits(64, frame.Camera.FOVY)
	bits(68, frame.Camera.Aspect)
	bits(72, frame.Camera.Near)
	bits(76, frame.Camera.Far)
	flag(80, frame.HUD.Ready)
	word(84, uint32(frame.HUD.Health))
	word(88, uint32(frame.HUD.Hunger))
	word(92, uint32(frame.HUD.Oxygen))
	flag(96, frame.Environment.Ready)
	double(104, frame.Environment.ServerTick)
	double(112, frame.Environment.WorldTimeTicks)
	word(120, uint32(frame.Environment.DayPhaseOffset))
	word(124, uint32(frame.Environment.Weather))
	word(128, uint32(frame.Environment.Season))
	word(132, uint32(frame.Environment.SeasonProgress))
	word(136, uint32(int32(frame.Environment.Temperature)))
	double(144, frame.Entities.ServerTick)
	for index, entity := range entities {
		base := 152 + 48*index
		word(base, uint32(entity.Kind))
		copy(buffer[base+8:base+24], entity.PlayerID[:])
		word(base+24, uint32(entity.Dimension))
		bits(base+28, entity.Position[0])
		bits(base+32, entity.Position[1])
		bits(base+36, entity.Position[2])
		bits(base+40, entity.Yaw)
		bits(base+44, entity.Pitch)
	}
	entityTail := 152 + 48*len(entities)
	flag(entityTail, frame.Target.Visible)
	word(entityTail+4, uint32(frame.Target.Position.X))
	word(entityTail+8, uint32(frame.Target.Position.Y))
	word(entityTail+12, uint32(frame.Target.Position.Z))
	word(entityTail+16, uint32(frame.Phase))
	word(entityTail+20, uint32(frame.Error.Code))
	copy(buffer[entityTail+24:], name)
	return buffer
}

// assertFrameCanary proves a buffer keeps its poison after a call that must not
// write it.
func assertFrameCanary(t *testing.T, buffer []byte, context string) {
	t.Helper()
	for index, value := range buffer {
		if value != 0xA5 {
			t.Fatalf("%s wrote buffer byte %d despite writing nothing", context, index)
		}
	}
}

// TestFrameSnapshotWireVocabularyPinsPilotValues pins the producer-side frame
// wire vocabulary: the fixed record sizes, the maximal whole-record arithmetic
// inside the frozen header limits, and the identity of the phase and error
// words the records carry.
func TestFrameSnapshotWireVocabularyPinsPilotValues(t *testing.T) {
	sizes := []struct {
		name  string
		value uint32
		want  uint32
	}{
		{"camera record bytes", FrameCameraRecordBytes, 40},
		{"HUD record bytes", FrameHUDRecordBytes, 16},
		{"environment record bytes", FrameEnvironmentRecordBytes, 48},
		{"entity-batch tick record bytes", FrameEntityTickRecordBytes, 8},
		{"entity record bytes", FrameEntityRecordBytes, 48},
		{"target record bytes", FrameTargetRecordBytes, 16},
		{"phase/error record bytes", FramePhaseErrorRecordBytes, 8},
		{"frame header bytes", FrameHeaderBytes, 40},
	}
	for _, size := range sizes {
		if size.value != size.want {
			t.Fatalf("%s = %d, want %d", size.name, size.value, size.want)
		}
		if size.value%ABIAlignment != 0 {
			t.Fatalf("%s = %d is not a multiple of the ABI alignment", size.name, size.value)
		}
	}
	if got, want := frameWireBytes(0, 0), 176; got != want {
		t.Fatalf("empty frame bytes = %d, want %d", got, want)
	}
	// Pin the maximal consumer allocation ceiling as a literal: seven entity
	// records plus the 64-byte target name.
	if got, want := frameWireBytes(int(MaxEntityRecords), int(MaxTargetNameBytes)), 576; got != want {
		t.Fatalf("maximal frame bytes = %d, want %d", got, want)
	}
	if got, want := MaxEntityRecords, uint32(7); got != want {
		t.Fatalf("entity record limit = %d, want %d", got, want)
	}
	if got, want := MaxTargetNameBytes, uint32(64); got != want {
		t.Fatalf("target name byte limit = %d, want %d", got, want)
	}
	// The entity kind, session phase, and error words keep their presentation
	// identities on the wire; pin the numeric values so a domain renumber
	// cannot pass silently through the uint32 casts.
	pins := []struct {
		name string
		got  uint32
		want uint32
	}{
		{"remote player entity kind", uint32(presentation.EntityKindRemotePlayer), 1},
		{"disconnected phase word", uint32(clientruntime.ConnectionPhaseDisconnected), 5},
		{"play phase word", uint32(clientruntime.ConnectionPhasePlay), 4},
		{"error none word", uint32(presentation.ErrorNone), 0},
		{"error connection word", uint32(presentation.ErrorConnection), 1},
		{"error login word", uint32(presentation.ErrorLogin), 2},
		{"error protocol word", uint32(presentation.ErrorProtocol), 3},
		{"error overflow word", uint32(presentation.ErrorOverflow), 4},
		{"error internal word", uint32(presentation.ErrorInternal), 5},
	}
	for _, pin := range pins {
		if pin.got != pin.want {
			t.Fatalf("%s = %d, want %d", pin.name, pin.got, pin.want)
		}
	}
}

// TestFrameSnapshotRichRoundtripDecodesEveryField serves the rich fixture
// through the full export path and decodes every record field against the
// presentation value that produced it, including entity order and the target
// name at the tail.
func TestFrameSnapshotRichRoundtripDecodesEveryField(t *testing.T) {
	frame := frameRichSnapshot(t, 7)
	served := frameServeThroughSession(t, frame)
	decoded := decodeFrameWire(t, served[0])

	entities := frame.Entities.Records()
	if decoded.entityCount != uint32(len(entities)) || decoded.nameLen != uint32(len("stone")) {
		t.Fatalf("counts = (%d entities, %d name bytes), want (%d, %d)",
			decoded.entityCount, decoded.nameLen, len(entities), len("stone"))
	}
	if decoded.revision != frame.Revision || decoded.epoch != frame.Epoch {
		t.Fatalf("identity = revision %d epoch %d, want %d and %d", decoded.revision, decoded.epoch, frame.Revision, frame.Epoch)
	}
	if decoded.phase != uint32(frame.Phase) || decoded.errorCode != uint32(frame.Error.Code) {
		t.Fatalf("phase/error = (%d, %d), want (%d, %d)", decoded.phase, decoded.errorCode, frame.Phase, frame.Error.Code)
	}
	if decoded.cameraReady != 1 || decoded.camera != frame.Camera {
		t.Fatalf("camera = (ready %d, %+v), want (1, %+v)", decoded.cameraReady, decoded.camera, frame.Camera)
	}
	if decoded.hudReady != 1 || decoded.hud != frame.HUD {
		t.Fatalf("HUD = (ready %d, %+v), want (1, %+v)", decoded.hudReady, decoded.hud, frame.HUD)
	}
	if decoded.environmentRdy != 1 || decoded.environment != frame.Environment {
		t.Fatalf("environment = (ready %d, %+v), want (1, %+v)", decoded.environmentRdy, decoded.environment, frame.Environment)
	}
	if decoded.entityTick != frame.Entities.ServerTick {
		t.Fatalf("entity-batch tick = %d, want %d", decoded.entityTick, frame.Entities.ServerTick)
	}
	for index, entity := range entities {
		wire := decoded.entities[index]
		if wire.kind != uint32(entity.Kind) || wire.playerID != entity.PlayerID ||
			wire.dimension != uint32(entity.Dimension) || wire.position != entity.Position ||
			wire.yaw != entity.Yaw || wire.pitch != entity.Pitch {
			t.Fatalf("entity %d = %+v, want %+v", index, wire, entity)
		}
	}
	if decoded.targetVisible != 1 || decoded.targetPos != frame.Target.Position {
		t.Fatalf("target = (visible %d, %+v), want (1, %+v)", decoded.targetVisible, decoded.targetPos, frame.Target.Position)
	}
	if decoded.name != frame.Target.Name {
		t.Fatalf("target name = %q, want %q", decoded.name, frame.Target.Name)
	}
}

// TestFrameSnapshotHUDNotReadyEncodesZeroedSemantics pins the not-ready
// contract: a frame whose camera, HUD, and environment are not ready serializes
// ready zero words with zero-valued payload fields — the presentation zero the
// validator guarantees — and never fabricated values.
func TestFrameSnapshotHUDNotReadyEncodesZeroedSemantics(t *testing.T) {
	frame := frameNotReadySnapshot(3)
	served := frameServeThroughSession(t, frame)
	decoded := decodeFrameWire(t, served[0])

	if decoded.cameraReady != 0 || decoded.hudReady != 0 || decoded.environmentRdy != 0 {
		t.Fatalf("ready words = (camera %d, HUD %d, environment %d), want all zero",
			decoded.cameraReady, decoded.hudReady, decoded.environmentRdy)
	}
	if decoded.camera != (presentation.CameraSnapshot{}) || decoded.hud != (presentation.HUDSnapshot{}) ||
		decoded.environment != (presentation.EnvironmentSnapshot{}) {
		t.Fatalf("not-ready payloads were fabricated: %+v %+v %+v", decoded.camera, decoded.hud, decoded.environment)
	}
	// Every payload byte of the three fixed records must be zero, not merely
	// decode to zero fields.
	cameraArea := served[0][FrameHeaderBytes : FrameHeaderBytes+FrameCameraRecordBytes+FrameHUDRecordBytes+FrameEnvironmentRecordBytes]
	for index := 4; index < len(cameraArea); index++ {
		if cameraArea[index] != 0 {
			t.Fatalf("not-ready payload byte %d = %#x, want zero", index, cameraArea[index])
		}
	}
	if decoded.entityCount != 0 || len(decoded.entities) != 0 {
		t.Fatalf("entity count = %d, want zero", decoded.entityCount)
	}
	if decoded.targetVisible != 0 || decoded.nameLen != 0 || decoded.name != "" {
		t.Fatalf("hidden target = (visible %d, name %q), want (0, empty)", decoded.targetVisible, decoded.name)
	}
}

// TestFrameSnapshotEntityCapacityBoundaryAtExactlySevenRecords pins the entity
// capacity boundary: a batch of exactly `MaxEntityRecords` records encodes with
// a matching wire count and faithful records, and the presentation constructor
// rejects a larger batch, so the wire field can never observe more than seven.
func TestFrameSnapshotEntityCapacityBoundaryAtExactlySevenRecords(t *testing.T) {
	batch := frameEntityBatch(t, 777, int(MaxEntityRecords))
	frame := frameRichSnapshot(t, 5)
	frame.Entities = batch
	frame.Environment.ServerTick = 777
	if err := frame.Validate(); err != nil {
		t.Fatalf("seven-entity fixture does not validate: %v", err)
	}
	served := frameServeThroughSession(t, frame)
	decoded := decodeFrameWire(t, served[0])

	if decoded.entityCount != MaxEntityRecords || len(decoded.entities) != int(MaxEntityRecords) {
		t.Fatalf("entity count = %d with %d decoded, want %d", decoded.entityCount, len(decoded.entities), MaxEntityRecords)
	}
	if decoded.entityTick != 777 {
		t.Fatalf("entity-batch tick = %d, want 777", decoded.entityTick)
	}
	for index, entity := range batch.Records() {
		wire := decoded.entities[index]
		if wire.playerID != entity.PlayerID || wire.position != entity.Position || wire.yaw != entity.Yaw {
			t.Fatalf("boundary entity %d = %+v, want %+v", index, wire, entity)
		}
	}
	wantBytes := frameWireBytes(int(MaxEntityRecords), len("stone"))
	if len(served[0]) != wantBytes {
		t.Fatalf("seven-entity record = %d bytes, want %d", len(served[0]), wantBytes)
	}

	overflow := make([]presentation.EntityRecord, int(MaxEntityRecords)+1)
	for index := range overflow {
		overflow[index] = presentation.EntityRecord{
			Kind:     presentation.EntityKindRemotePlayer,
			PlayerID: framePlayerID(byte(index + 1)),
		}
	}
	if _, err := presentation.NewEntityBatch(1, overflow); err == nil {
		t.Fatal("presentation accepted an eight-record batch; the wire boundary is unguarded")
	}
}

// TestFrameSnapshotTargetNameBoundaryAtSixtyFourBytes pins the target-name
// capacity: a visible target whose name is exactly `MaxTargetNameBytes` UTF-8
// bytes serializes with a matching name_len header word and a faithful tail.
func TestFrameSnapshotTargetNameBoundaryAtSixtyFourBytes(t *testing.T) {
	frame := frameRichSnapshot(t, 6)
	frame.Target.Name = strings.Repeat("granite", 9) + "g" // 7*9+1 = 64 bytes
	if len(frame.Target.Name) != int(MaxTargetNameBytes) {
		t.Fatalf("fixture name length = %d, want %d", len(frame.Target.Name), MaxTargetNameBytes)
	}
	if err := frame.Validate(); err != nil {
		t.Fatalf("64-byte name fixture does not validate: %v", err)
	}
	served := frameServeThroughSession(t, frame)
	decoded := decodeFrameWire(t, served[0])

	if decoded.nameLen != MaxTargetNameBytes || decoded.name != frame.Target.Name {
		t.Fatalf("target name = (%d bytes, %q), want (%d, %q)", decoded.nameLen, decoded.name, MaxTargetNameBytes, frame.Target.Name)
	}
	wantBytes := frameWireBytes(3, int(MaxTargetNameBytes))
	if len(served[0]) != wantBytes {
		t.Fatalf("64-byte name record = %d bytes, want %d", len(served[0]), wantBytes)
	}
}

// TestFrameSnapshotRepeatedPullsDoNotConsume pins the non-consuming ruling: a
// frame pull reads the latest retained snapshot without consuming it, so
// repeated queries report the same size and repeated pulls return identical
// bytes; only a new step replaces the served snapshot.
func TestFrameSnapshotRepeatedPullsDoNotConsume(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	first := frameRichSnapshot(t, 11)
	second := frameNotReadySnapshot(12)
	installWorldResultSeam(t, clientruntime.StepResult{Frame: first}, clientruntime.StepResult{Frame: second})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("first scripted frame step = %d, want StatusOK", status)
	}
	wantBytes := uint32(frameWireBytes(3, len("stone")))

	for query := 0; query < 3; query++ {
		required, status := frameQuerySize(t, handle)
		if status != StatusInsufficientCapacity || required != wantBytes {
			t.Fatalf("query %d = (%d, %d), want (%d, StatusInsufficientCapacity)", query, required, status, wantBytes)
		}
	}
	var reference []byte
	for pull := 0; pull < 5; pull++ {
		storage, required, status := frameConsume(handle, wantBytes)
		if status != StatusOK {
			t.Fatalf("non-consuming pull %d = %d, want StatusOK", pull, status)
		}
		if required != uint32(0xDEADBEEF) {
			t.Fatalf("pull %d reported a size word %d; success reports no size", pull, required)
		}
		if pull > 0 && string(storage) != string(reference) {
			t.Fatalf("pull %d diverged from the first pull; a frame pull must not consume", pull)
		}
		reference = storage
	}
	if decoded := decodeFrameWire(t, reference); decoded.revision != first.Revision {
		t.Fatalf("served revision = %d, want %d", decoded.revision, first.Revision)
	}

	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("second scripted frame step = %d, want StatusOK", status)
	}
	storage, _, status := frameConsume(handle, 1024)
	if status != StatusOK {
		t.Fatalf("pull after the newer step = %d, want StatusOK", status)
	}
	if decoded := decodeFrameWire(t, storage[:frameRecordLength(storage)]); decoded.revision != second.Revision {
		t.Fatalf("served revision after the newer step = %d, want %d (latest completed step wins)",
			decoded.revision, second.Revision)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the frame walk = %d, want StatusOK", status)
	}
}

// TestFrameSnapshotInsufficientCapacityWritesNothingAndStaysServable proves a
// below-required capacity never writes a partial record: the canary survives,
// the fresh required size is reported, and the frame stays servable with
// identical bytes afterwards.
func TestFrameSnapshotInsufficientCapacityWritesNothingAndStaysServable(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	frame := frameRichSnapshot(t, 13)
	installWorldResultSeam(t, clientruntime.StepResult{Frame: frame})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted frame step = %d, want StatusOK", status)
	}
	wantBytes := uint32(frameWireBytes(3, len("stone")))

	for _, capacity := range []uint32{1, wantBytes - 1, wantBytes - 8} {
		storage, required, status := frameConsume(handle, capacity)
		if status != StatusInsufficientCapacity {
			t.Fatalf("capacity %d status = %d, want StatusInsufficientCapacity", capacity, status)
		}
		if required != wantBytes {
			t.Fatalf("capacity %d reported required = %d, want %d", capacity, required, wantBytes)
		}
		assertFrameCanary(t, storage, "insufficient-capacity pull")
	}

	storage, _, status := frameConsume(handle, wantBytes)
	if status != StatusOK {
		t.Fatalf("pull after failed pulls = %d, want StatusOK (failures consumed nothing)", status)
	}
	if decoded := decodeFrameWire(t, storage); decoded.revision != frame.Revision {
		t.Fatalf("late pull revision = %d, want %d", decoded.revision, frame.Revision)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the frame walk = %d, want StatusOK", status)
	}
}

// TestFrameSnapshotOversizedCapacityWritesExactlyAndLeavesTailUntouched proves
// an oversized buffer receives exactly the required bytes and the surplus tail
// survives untouched.
func TestFrameSnapshotOversizedCapacityWritesExactlyAndLeavesTailUntouched(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	frame := frameRichSnapshot(t, 14)
	installWorldResultSeam(t, clientruntime.StepResult{Frame: frame})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted frame step = %d, want StatusOK", status)
	}
	wantBytes := uint32(frameWireBytes(3, len("stone")))

	oversized, _, status := frameConsume(handle, wantBytes+21)
	if status != StatusOK {
		t.Fatalf("oversized pull = %d, want StatusOK", status)
	}
	for index := int(wantBytes); index < len(oversized); index++ {
		if oversized[index] != 0xA5 {
			t.Fatalf("oversized write touched tail byte %d beyond the required size", index)
		}
	}
	exact, _, status := frameConsume(handle, wantBytes)
	if status != StatusOK {
		t.Fatalf("exact pull = %d, want StatusOK", status)
	}
	if string(oversized[:wantBytes]) != string(exact) {
		t.Fatal("oversized write diverges from the exact record")
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the frame walk = %d, want StatusOK", status)
	}
}

// TestFrameSnapshotNoFrameYetReportsZeroSize pins the no-frame outcome: an idle
// session that never stepped and a session that reached its terminal state
// before any step both report `StatusOK` with size zero and write no buffer
// byte. Size zero is unambiguous because the frozen header alone is 40 bytes.
func TestFrameSnapshotNoFrameYetReportsZeroSize(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	storage, required, status := frameConsume(idle, 64)
	if status != StatusOK || required != 0 {
		t.Fatalf("idle frame pull = (%d, %d), want (0, StatusOK)", required, status)
	}
	assertFrameCanary(t, storage, "idle no-frame pull")
	if status := coreDestroy(idle); status != StatusOK {
		t.Fatalf("destroy idle = %d, want StatusOK", status)
	}

	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("disconnect before any step = %d, want StatusOK", status)
	}
	storage, required, status = frameConsume(handle, 64)
	if status != StatusOK || required != 0 {
		t.Fatalf("terminal-before-step frame pull = (%d, %d), want (0, StatusOK)", required, status)
	}
	assertFrameCanary(t, storage, "terminal no-frame pull")
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy terminal = %d, want StatusOK", status)
	}
}

// TestFrameSnapshotTerminalFrameServedAfterDisconnect pins the retention
// ruling: a step that publishes the terminal frame leaves it pullable after the
// session reached its terminal state — the pull reports `StatusOK` with the
// terminal snapshot (phase disconnected plus its error record), never
// `StatusDisconnected` — and the terminal frame stays pullable repeatedly.
func TestFrameSnapshotTerminalFrameServedAfterDisconnect(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	terminal := frameTerminalSnapshot(21)
	installWorldResultSeam(t, clientruntime.StepResult{Frame: terminal})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("terminal-frame step = %d, want StatusOK (the terminal frame is a successful step)", status)
	}
	// The step claimed the terminal transition, so a poll now reports the
	// disconnect while the frame pull keeps serving the retained frame.
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(handle, &phase); status != StatusDisconnected {
		t.Fatalf("poll after the terminal step = %d, want StatusDisconnected", status)
	}

	wantBytes := uint32(frameWireBytes(0, 0))
	for pull := 0; pull < 2; pull++ {
		required, status := frameQuerySize(t, handle)
		if status != StatusInsufficientCapacity || required != wantBytes {
			t.Fatalf("terminal query %d = (%d, %d), want (%d, StatusInsufficientCapacity)", pull, required, status, wantBytes)
		}
		storage, _, status := frameConsume(handle, wantBytes)
		if status != StatusOK {
			t.Fatalf("terminal pull %d = %d, want StatusOK (retention survives teardown)", pull, status)
		}
		decoded := decodeFrameWire(t, storage)
		if decoded.phase != uint32(clientruntime.ConnectionPhaseDisconnected) ||
			decoded.errorCode != uint32(presentation.ErrorConnection) {
			t.Fatalf("terminal record = (phase %d, error %d), want (%d, %d)",
				decoded.phase, decoded.errorCode, clientruntime.ConnectionPhaseDisconnected, presentation.ErrorConnection)
		}
		if decoded.cameraReady != 0 || decoded.hudReady != 0 || decoded.entityCount != 0 {
			t.Fatalf("terminal frame carries presentational payloads: %+v", decoded)
		}
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the terminal walk = %d, want StatusOK", status)
	}
}

// TestFrameSnapshotValidatesHandleAndPointerShapeBeforeContent pins the
// validation order: the handle lifecycle outranks every pointer defect, the
// pointer shape outranks the no-frame content outcome, and every rejection
// leaves the retained frame servable.
func TestFrameSnapshotValidatesHandleAndPointerShapeBeforeContent(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	if status := coreFramePull(0, nil, 0, nil); status != StatusInvalidHandle {
		t.Fatalf("frame pull on the zero handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreFramePull(makeSessionHandle(3, 1), nil, 0, nil); status != StatusInvalidHandle {
		t.Fatalf("frame pull on a never-issued handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreFramePull(destroyed, nil, 0, nil); status != StatusInvalidState {
		t.Fatalf("frame pull on a destroyed handle = %d, want StatusInvalidState", status)
	}
	if status := coreFramePull(idle, nil, 0, nil); status != StatusInvalidArgument {
		t.Fatalf("frame pull with a null size out-parameter = %d, want StatusInvalidArgument (shape precedes content)", status)
	}
	if status := coreDestroy(idle); status != StatusOK {
		t.Fatalf("destroy idle = %d, want StatusOK", status)
	}

	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	frame := frameRichSnapshot(t, 15)
	installWorldResultSeam(t, clientruntime.StepResult{Frame: frame})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted frame step = %d, want StatusOK", status)
	}
	wantBytes := uint32(frameWireBytes(3, len("stone")))

	if status := coreFramePull(handle, nil, 0, nil); status != StatusInvalidArgument {
		t.Fatalf("frame pull with a null size out-parameter = %d, want StatusInvalidArgument", status)
	}
	if status := coreFramePull(handle, nil, wantBytes, new(uint32)); status != StatusInvalidArgument {
		t.Fatalf("frame pull with a null buffer and positive capacity = %d, want StatusInvalidArgument", status)
	}
	misalignedStorage := poisonedBuffer(int(wantBytes) + 32)
	misaligned := (*byte)(unsafe.Pointer(&misalignedStorage[1]))
	misalignedSize := (*uint32)(unsafe.Pointer(&misalignedStorage[int(wantBytes)+24]))
	*misalignedSize = uint32(0xDEADBEEF)
	if status := coreFramePull(handle, misaligned, wantBytes, misalignedSize); status != StatusInvalidArgument {
		t.Fatalf("frame pull with a misaligned buffer = %d, want StatusInvalidArgument", status)
	}
	if *misalignedSize != 0xDEADBEEF {
		t.Fatalf("shape rejection wrote the size out-parameter %d despite failure", *misalignedSize)
	}
	assertFrameCanary(t, misalignedStorage[:int(wantBytes)+1], "misaligned frame pull")

	required, status := frameQuerySize(t, handle)
	if status != StatusInsufficientCapacity || required != wantBytes {
		t.Fatalf("query after rejections = (%d, %d), want (%d, StatusInsufficientCapacity) (rejections consumed nothing)",
			required, status, wantBytes)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy online = %d, want StatusOK", status)
	}
}

// TestFrameSnapshotRejectsSizeOutParamAliasingWriteBuffer pins the overlap
// rule: the size out-parameter must not alias the caller-declared write buffer,
// the rejection writes nothing, and the frame stays servable; an
// out-parameter just past the span is legal because the exact write never
// reaches it.
func TestFrameSnapshotRejectsSizeOutParamAliasingWriteBuffer(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	frame := frameRichSnapshot(t, 16)
	installWorldResultSeam(t, clientruntime.StepResult{Frame: frame})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted frame step = %d, want StatusOK", status)
	}
	wantBytes := uint32(frameWireBytes(3, len("stone")))

	for _, aliasOffset := range []uintptr{0, 8, uintptr(wantBytes - 4)} {
		storage := poisonedBuffer(int(wantBytes))
		aliasedSize := (*uint32)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), aliasOffset)))
		if status := coreFramePull(handle, &storage[0], wantBytes, aliasedSize); status != StatusInvalidArgument {
			t.Fatalf("frame pull with the size word at buffer offset %d = %d, want StatusInvalidArgument", aliasOffset, status)
		}
		assertFrameCanary(t, storage, "aliased size-word frame pull")
	}

	storage := poisonedBuffer(int(wantBytes) + 8)
	outsideSize := (*uint32)(unsafe.Pointer(unsafe.Pointer(&storage[wantBytes])))
	if status := coreFramePull(handle, &storage[0], wantBytes, outsideSize); status != StatusOK {
		t.Fatalf("frame pull with the size word just past the span = %d, want StatusOK", status)
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

// TestFrameSnapshotPanicConversionKeepsFrameServableWithoutOutput injects a
// panic through the encoder seam: the export converts it to `StatusPanic`,
// writes neither the buffer nor the size word, and — because a frame pull
// consumes nothing — the same frame stays servable with identical bytes.
func TestFrameSnapshotPanicConversionKeepsFrameServableWithoutOutput(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	frame := frameRichSnapshot(t, 17)
	installWorldResultSeam(t, clientruntime.StepResult{Frame: frame})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted frame step = %d, want StatusOK", status)
	}
	wantBytes := uint32(frameWireBytes(3, len("stone")))

	before, _, status := frameConsume(handle, wantBytes)
	if status != StatusOK {
		t.Fatalf("baseline pull = %d, want StatusOK", status)
	}
	buffer, sizeOut := frameCallerFrame(wantBytes)
	*sizeOut = uint32(0xDEADBEEF)
	previous := frameEncode
	frameEncode = func(presentation.FrameSnapshot) []byte { panic("injected frame encoder panic") }
	status = coreFramePull(handle, &buffer[0], wantBytes, sizeOut)
	frameEncode = previous
	if status != StatusPanic {
		t.Fatalf("pull during the injected panic = %d, want StatusPanic", status)
	}
	if *sizeOut != 0xDEADBEEF {
		t.Fatalf("panic wrote the size out-parameter %d", *sizeOut)
	}
	assertFrameCanary(t, buffer, "panicking frame pull")

	after, _, status := frameConsume(handle, wantBytes)
	if status != StatusOK {
		t.Fatalf("pull after the converted panic = %d, want StatusOK", status)
	}
	if string(before) != string(after) {
		t.Fatal("frame bytes changed across a converted panic")
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the panic walk = %d, want StatusOK", status)
	}
}

// TestFrameSnapshotInvalidRetainedFrameMapsToInternal pins the defensive
// mapping: a retained frame that fails the presentation validators is a
// producer invariant break, reports `StatusInternal`, and writes nothing.
func TestFrameSnapshotInvalidRetainedFrameMapsToInternal(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	invalid := frameRichSnapshot(t, 1) // rebuilt below with a zero revision
	invalid.Revision = 0               // violates the frame identity contract
	if err := invalid.Validate(); err == nil {
		t.Fatal("fixture frame unexpectedly validates")
	}
	installWorldResultSeam(t, clientruntime.StepResult{Frame: invalid})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted invalid-frame step = %d, want StatusOK", status)
	}

	storage, sizeOut := frameCallerFrame(64)
	*sizeOut = uint32(0xDEADBEEF)
	if status := coreFramePull(handle, &storage[0], 64, sizeOut); status != StatusInternal {
		t.Fatalf("pull of an invalid retained frame = %d, want StatusInternal", status)
	}
	if *sizeOut != 0xDEADBEEF {
		t.Fatalf("internal failure wrote the size out-parameter %d", *sizeOut)
	}
	assertFrameCanary(t, storage, "internal-failure frame pull")

	if required, status := frameQuerySize(t, handle); status != StatusInternal {
		t.Fatalf("query of an invalid retained frame = (%d, %d), want StatusInternal", required, status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the internal mapping = %d, want StatusOK", status)
	}
}

// TestFrameSnapshotConcurrentPullsWithStepsWorldAndMetricsStayConsistent races
// frame pulls, status pulls, and world pulls against a stepping goroutine
// publishing successive distinct frames: every frame pull decodes to the exact
// expected record of exactly one scripted frame (byte-for-byte equal to the
// reference encoding), repeated pulls never observe a torn or unknown record,
// and after the stepper finishes the frame pull serves the final frame.
func TestFrameSnapshotConcurrentPullsWithStepsWorldAndMetricsStayConsistent(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)

	const steps = 4
	expected := make(map[uint64][]byte, steps)
	results := make([]clientruntime.StepResult, 0, steps)
	for index := 1; index <= steps; index++ {
		var frame presentation.FrameSnapshot
		if index%2 == 0 {
			frame = frameRichSnapshot(t, uint64(100+index))
			frame.Entities = frameEntityBatch(t, uint64(900+index), index%int(MaxEntityRecords)+1)
		} else {
			frame = frameNotReadySnapshot(uint64(100 + index))
		}
		if err := frame.Validate(); err != nil {
			t.Fatalf("fixture frame %d does not validate: %v", index, err)
		}
		expected[frame.Revision] = frameReferenceBytes(t, frame)
		results = append(results, clientruntime.StepResult{Frame: frame})
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

	for worker := 0; worker < 3; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < 50_000; iteration++ {
				storage, required, status := frameConsume(handle, 1024)
				if status != StatusOK {
					t.Errorf("concurrent frame pull = %d, want StatusOK", status)
					return
				}
				if required == 0 {
					// The documented no-frame outcome: the stepper has not
					// completed its first step, so the buffer keeps its poison.
					select {
					case <-stepped:
						return
					default:
					}
					continue
				}
				revision := binary.LittleEndian.Uint64(storage[24:32])
				want, known := expected[revision]
				if !known || string(storage[:len(want)]) != string(want) {
					t.Errorf("concurrent frame pull served an unknown or torn record for revision %d", revision)
					return
				}
				if frameRecordLength(storage) != len(want) {
					t.Errorf("concurrent frame pull served a record whose header counts disagree with revision %d", revision)
					return
				}
				for tail := len(want); tail < len(storage); tail++ {
					if storage[tail] != 0xA5 {
						t.Errorf("concurrent frame pull touched the tail at byte %d", tail)
						return
					}
				}
				// The status and world families run concurrently on the same
				// session without disturbing the frame pull.
				statusRequired := uint32(0xDEADBEEF)
				if status := coreStatusPull(handle, nil, 0, &statusRequired); status != StatusInsufficientCapacity {
					t.Errorf("concurrent status query = %d, want StatusInsufficientCapacity", status)
					return
				}
				if status := coreWorldPull(handle, nil, 0, new(uint32)); status != StatusOK && status != StatusInsufficientCapacity {
					t.Errorf("concurrent world query = %d, want StatusOK or StatusInsufficientCapacity", status)
					return
				}
				select {
				case <-stepped:
					return
				default:
				}
			}
		}()
	}
	group.Wait()

	final, _, status := frameConsume(handle, 1024)
	if status != StatusOK {
		t.Fatalf("final frame pull = %d, want StatusOK", status)
	}
	if decoded := decodeFrameWire(t, final[:frameRecordLength(final)]); decoded.revision != 100+steps {
		t.Fatalf("final frame revision = %d, want %d", decoded.revision, 100+steps)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the race = %d, want StatusOK", status)
	}
}

// TestOptionalFamilyAdditionKeepsV1FrameDecodingUnchanged proves the
// optional-family purity ruling: the v1 frame wire bytes are a pure function of
// the `presentation.FrameSnapshot` value and the frozen MCF1 constants, never
// of the family registry. The proof has three legs. First, an independent
// reference encoder built from literal offsets (not production constants)
// reproduces the production encoder byte-for-byte for rich, all-not-ready, and
// terminal snapshots. Second, the full export path serves exactly those
// reference bytes, so any later change that made frame encoding consult
// registry content would diverge from the pin. Third, the registry accepts
// valid variant tables (a frame descriptor with a higher compatible version)
// without any frame-path involvement, which is exactly the shape of a later
// optional-family addition: `NewRegistry` grows append-only while v1 frame
// decoding stays pinned to these bytes.
func TestOptionalFamilyAdditionKeepsV1FrameDecodingUnchanged(t *testing.T) {
	rich := frameRichSnapshot(t, 31)
	notReady := frameNotReadySnapshot(32)
	terminal := frameTerminalSnapshot(33)
	for _, frame := range []presentation.FrameSnapshot{rich, notReady, terminal} {
		reference := frameReferenceBytes(t, frame)
		if produced := encodeFrameSnapshot(frame); produced == nil || string(produced) != string(reference) {
			t.Fatalf("production encoder diverged from the reference bytes for revision %d", frame.Revision)
		}
	}
	served := frameServeThroughSession(t, rich, notReady, terminal)
	for index, frame := range []presentation.FrameSnapshot{rich, notReady, terminal} {
		if string(served[index]) != string(frameReferenceBytes(t, frame)) {
			t.Fatalf("export path diverged from the reference bytes for script frame %d", index)
		}
	}

	registry := ClientCoreRegistry()
	descriptor, ok := registry.Descriptor(FamilyFrame)
	if !ok || descriptor.Version != FrameVersion || descriptor.RecordLimit != MaxEntityRecords {
		t.Fatalf("frame descriptor = %+v, want version %d and limit %d", descriptor, FrameVersion, MaxEntityRecords)
	}
	variant := frameRegistryVariantTable(t)
	// Index by lookup, not table order: assert the bumped entry really is the
	// frame descriptor so a table reordering cannot make this leg vacuous.
	index := slices.IndexFunc(variant, func(d RegistryDescriptor) bool { return d.Family == FamilyFrame })
	if index < 0 || variant[index].Version != FrameVersion {
		t.Fatalf("variant table frame entry = %+v at %d", variant[index], index)
	}
	variant[index].Version = FrameVersion + 1
	if _, err := NewRegistry(variant); err != nil {
		t.Fatalf("registry rejects a valid frame-version variant: %v", err)
	}
	// The variant proves nothing in the frame path reads the registry: after
	// building it, the pinned bytes are unchanged because the encoder's only
	// inputs are the snapshot and the frozen constants.
	if produced := encodeFrameSnapshot(rich); string(produced) != string(frameReferenceBytes(t, rich)) {
		t.Fatal("frame bytes changed after constructing a registry variant")
	}
}

// frameRegistryVariantTable copies the current descriptor table in family-ID
// order so a test can vary one entry without mutating the shared table.
func frameRegistryVariantTable(t *testing.T) []RegistryDescriptor {
	t.Helper()
	registry := ClientCoreRegistry()
	view := make([]RegistryDescriptor, registry.Len())
	if copied := registry.Descriptors(view); copied != len(view) {
		t.Fatalf("descriptor enumeration = %d, want %d", copied, len(view))
	}
	return view
}

// TestFrameTargetNameBudgetCoversEveryRegisteredBlockName pins the data-side
// guarantee behind the encoder's own name guard: presentation does not bound
// the target name, so every registered block display name must fit the wire
// budget, otherwise a long name would degrade the whole frame family to the
// internal failure status.
func TestFrameTargetNameBudgetCoversEveryRegisteredBlockName(t *testing.T) {
	for id := core.BlockID(0); id < core.BlockIDMax; id++ {
		name, ok := core.BlockDisplayName(id)
		if !ok {
			continue
		}
		if len(name) > int(MaxTargetNameBytes) {
			t.Fatalf("block %d display name is %d bytes, over the %d-byte wire budget", id, len(name), MaxTargetNameBytes)
		}
	}
}
