package main

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"unsafe"

	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// The metrics tests pin the status/metrics family: the MCM1 header plus fixed
// 16-byte kind-prefixed records — the session phase word (the connect family's
// vocabulary), the terminal-cause classification of the recorded terminal
// error, and the bounded step counters. The pull is non-consuming like the
// frame pull: it observes session state and changes nothing. Terminal-cause
// classes are driven through the real transport seams where cheap (dial
// failure, handshake and login rejection, protocol violation, receiver death,
// clean close) and pinned by classifier unit cases for the shapes the seams
// cannot produce.

// metricsWireRecord is the decoded view of one 16-byte status record.
type metricsWireRecord struct {
	kind  uint32
	value uint64
}

// decodeMetricsWire decodes one complete record, pinning the header identity
// and the count-to-length arithmetic before returning every record.
func decodeMetricsWire(t *testing.T, buffer []byte) []metricsWireRecord {
	t.Helper()
	if len(buffer) < int(StatusHeaderBytes) {
		t.Fatalf("record of %d bytes is shorter than the header", len(buffer))
	}
	if magic := binary.LittleEndian.Uint32(buffer[0:4]); magic != uint32(MagicStatus) {
		t.Fatalf("header magic = %#x, want status magic %#x", magic, uint32(MagicStatus))
	}
	if layout := binary.LittleEndian.Uint32(buffer[4:8]); layout != StatusVersion {
		t.Fatalf("header layout = %d, want %d", layout, StatusVersion)
	}
	if reserved := binary.LittleEndian.Uint32(buffer[12:16]); reserved != 0 {
		t.Fatalf("header reserved = %d, want zero", reserved)
	}
	count := binary.LittleEndian.Uint32(buffer[8:12])
	if count == 0 || count > MaxStatusRecords {
		t.Fatalf("record count %d is outside 1..%d", count, MaxStatusRecords)
	}
	wantBytes := metricsWireBytes(int(count))
	if len(buffer) != wantBytes {
		t.Fatalf("record length %d does not match count %d => %d bytes", len(buffer), count, wantBytes)
	}
	records := make([]metricsWireRecord, count)
	for index := range records {
		base := int(StatusHeaderBytes) + index*int(StatusRecordBytes)
		records[index] = metricsWireRecord{
			kind:  binary.LittleEndian.Uint32(buffer[base : base+4]),
			value: binary.LittleEndian.Uint64(buffer[base+8 : base+16]),
		}
		if reserved := binary.LittleEndian.Uint32(buffer[base+4 : base+8]); reserved != 0 {
			t.Fatalf("record %d reserved word = %d, want zero", index, reserved)
		}
	}
	return records
}

// metricsQuerySize performs the zero-capacity two-phase size query.
func metricsQuerySize(t *testing.T, handle uint64) (uint32, Status) {
	t.Helper()
	required := uint32(0xDEADBEEF)
	status := coreStatusPull(handle, nil, 0, &required)
	return required, status
}

// metricsCallerFrame models one C caller's frame with the same discipline as
// the world and frame tests.
func metricsCallerFrame(capacity uint32) (buffer []byte, sizeOut *uint32) {
	storage := poisonedBuffer(int(capacity) + 40)
	buffer = storage[:int(capacity):int(capacity)]
	sizeOffset := (int(capacity)+7)&^7 + 32
	return buffer, (*uint32)(unsafe.Pointer(&storage[sizeOffset]))
}

// metricsConsume performs one pull into a fresh caller frame of the declared
// capacity. The capacity must be positive so the buffer has an address.
func metricsConsume(handle uint64, capacity uint32) (buffer []byte, required uint32, status Status) {
	buffer, sizeOut := metricsCallerFrame(capacity)
	*sizeOut = uint32(0xDEADBEEF)
	status = coreStatusPull(handle, &buffer[0], capacity, sizeOut)
	return buffer, *sizeOut, status
}

// metricsRecordValue returns the value of the one record of the given kind,
// failing when the kind is absent or duplicated.
func metricsRecordValue(t *testing.T, records []metricsWireRecord, kind uint32) uint64 {
	t.Helper()
	var found bool
	var value uint64
	for _, record := range records {
		if record.kind != kind {
			continue
		}
		if found {
			t.Fatalf("record kind %d appears twice", kind)
		}
		found = true
		value = record.value
	}
	if !found {
		t.Fatalf("record kind %d is absent from %+v", kind, records)
	}
	return value
}

// metricsPullOnce queries and pulls the current record set.
func metricsPullOnce(t *testing.T, handle uint64) []metricsWireRecord {
	t.Helper()
	required, status := metricsQuerySize(t, handle)
	if status != StatusInsufficientCapacity || required == 0 {
		t.Fatalf("metrics query = (%d, %d), want a positive size and StatusInsufficientCapacity", required, status)
	}
	storage, _, status := metricsConsume(handle, required)
	if status != StatusOK {
		t.Fatalf("metrics pull = %d, want StatusOK", status)
	}
	return decodeMetricsWire(t, storage)
}

// assertMetricsCanary proves a buffer keeps its poison after a call that must
// not write it.
func assertMetricsCanary(t *testing.T, buffer []byte, context string) {
	t.Helper()
	for index, value := range buffer {
		if value != 0xA5 {
			t.Fatalf("%s wrote buffer byte %d despite writing nothing", context, index)
		}
	}
}

// TestMetricsWireVocabularyPinsPilotValues pins the producer-side status wire
// vocabulary: the fixed record size, the kind codes, the terminal-cause codes,
// and the pilot record-set arithmetic inside the frozen header limit.
func TestMetricsWireVocabularyPinsPilotValues(t *testing.T) {
	if got, want := uint32(StatusRecordBytes), uint32(16); got != want {
		t.Fatalf("status record bytes = %d, want %d", got, want)
	}
	if StatusRecordBytes/ABIAlignment*ABIAlignment != StatusRecordBytes {
		t.Fatalf("status record bytes %d are not a multiple of the ABI alignment %d", StatusRecordBytes, ABIAlignment)
	}
	kinds := []struct {
		name string
		got  uint32
		want uint32
	}{
		{"phase record kind", StatusRecordPhase, 1},
		{"terminal-cause record kind", StatusRecordTerminalCause, 2},
		{"steps-completed record kind", StatusRecordStepsCompleted, 3},
		{"messages-processed record kind", StatusRecordMessagesProcessed, 4},
	}
	for _, kind := range kinds {
		if kind.got != kind.want {
			t.Fatalf("%s = %d, want %d", kind.name, kind.got, kind.want)
		}
	}
	causes := []struct {
		name string
		got  uint32
		want uint32
	}{
		{"terminal cause none", TerminalCauseNone, 0},
		{"terminal cause dial", TerminalCauseDial, 1},
		{"terminal cause handshake", TerminalCauseHandshake, 2},
		{"terminal cause login", TerminalCauseLogin, 3},
		{"terminal cause protocol", TerminalCauseProtocol, 4},
		{"terminal cause receiver", TerminalCauseReceiver, 5},
		{"terminal cause internal", TerminalCauseInternal, 6},
	}
	for _, cause := range causes {
		if cause.got != cause.want {
			t.Fatalf("%s = %d, want %d", cause.name, cause.got, cause.want)
		}
	}
	if got, want := metricsWireBytes(4), int(StatusHeaderBytes)+4*int(StatusRecordBytes); got != want {
		t.Fatalf("pilot record set bytes = %d, want %d", got, want)
	}
	if got, want := metricsWireBytes(4), 80; got != want {
		t.Fatalf("pilot record set bytes literal = %d, want %d", got, want)
	}
	if got, want := metricsWireBytes(int(MaxStatusRecords)), 16+int(MaxStatusRecords)*16; got != want {
		t.Fatalf("maximal record set bytes = %d, want %d", got, want)
	}
	// The phase record carries the connect family's vocabulary verbatim; pin
	// the numeric phase words a consumer decodes.
	if got, want := ConnectPhaseLoading, uint32(3); got != want {
		t.Fatalf("loading phase word = %d, want %d", got, want)
	}
	if got, want := ConnectPhaseDisconnected, uint32(5); got != want {
		t.Fatalf("disconnected phase word = %d, want %d", got, want)
	}
}

// TestMetricsIdleSessionReportsNotReadyPhaseAndZeroCounters pins the idle
// record set: phase not ready, terminal cause none, and zero counters; the
// status family always serves content, so a query never reports size zero.
func TestMetricsIdleSessionReportsNotReadyPhaseAndZeroCounters(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	defer func() {
		if status := coreDestroy(idle); status != StatusOK {
			t.Fatalf("destroy idle = %d, want StatusOK", status)
		}
	}()
	records := metricsPullOnce(t, idle)
	if got := metricsRecordValue(t, records, StatusRecordPhase); got != uint64(ConnectPhaseNotReady) {
		t.Fatalf("idle phase = %d, want %d", got, ConnectPhaseNotReady)
	}
	if got := metricsRecordValue(t, records, StatusRecordTerminalCause); got != uint64(TerminalCauseNone) {
		t.Fatalf("idle terminal cause = %d, want %d", got, TerminalCauseNone)
	}
	if got := metricsRecordValue(t, records, StatusRecordStepsCompleted); got != 0 {
		t.Fatalf("idle steps completed = %d, want 0", got)
	}
	if got := metricsRecordValue(t, records, StatusRecordMessagesProcessed); got != 0 {
		t.Fatalf("idle messages processed = %d, want 0", got)
	}
	if len(records) != 4 {
		t.Fatalf("idle record count = %d, want the four pilot records", len(records))
	}
}

// TestMetricsOnlineSessionCountsStepsAndMessages pins the live counters: two
// scripted steps with distinct message counts accumulate into the totals while
// the online phase word mirrors the runtime's loading phase.
func TestMetricsOnlineSessionCountsStepsAndMessages(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	first := clientruntime.StepResult{Frame: frameNotReadySnapshot(1), MessagesProcessed: 3}
	second := clientruntime.StepResult{Frame: frameNotReadySnapshot(2), MessagesProcessed: 5}
	installWorldResultSeam(t, first, second)
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("first scripted step = %d, want StatusOK", status)
	}
	records := metricsPullOnce(t, handle)
	if got := metricsRecordValue(t, records, StatusRecordStepsCompleted); got != 1 {
		t.Fatalf("steps completed after one step = %d, want 1", got)
	}
	if got := metricsRecordValue(t, records, StatusRecordMessagesProcessed); got != 3 {
		t.Fatalf("messages processed after one step = %d, want 3", got)
	}
	if got := metricsRecordValue(t, records, StatusRecordPhase); got != uint64(ConnectPhaseLoading) {
		t.Fatalf("online phase = %d, want %d", got, ConnectPhaseLoading)
	}

	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("second scripted step = %d, want StatusOK", status)
	}
	records = metricsPullOnce(t, handle)
	if got := metricsRecordValue(t, records, StatusRecordStepsCompleted); got != 2 {
		t.Fatalf("steps completed after two steps = %d, want 2", got)
	}
	if got := metricsRecordValue(t, records, StatusRecordMessagesProcessed); got != 8 {
		t.Fatalf("messages processed after two steps = %d, want 8", got)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy online = %d, want StatusOK", status)
	}
}

// TestMetricsEstablishingSessionReportsConnectingPhase pins the establishing
// record set: while the connect goroutine is inside establishment, the phase
// record reports the connecting word without touching the runtime.
func TestMetricsEstablishingSessionReportsConnectingPhase(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	release := make(chan struct{})
	installConnectTransport(t, clientruntime.RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) {
			<-release
			return nil, errors.New("metrics seam: released dial failure")
		},
	})
	beginConnecting(t, handle, "127.0.0.1:25565")

	records := metricsPullOnce(t, handle)
	if got := metricsRecordValue(t, records, StatusRecordPhase); got != uint64(ConnectPhaseConnecting) {
		t.Fatalf("establishing phase = %d, want %d", got, ConnectPhaseConnecting)
	}
	close(release)
	connectPollUntilStatus(t, handle, StatusDisconnected)
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after establishment = %d, want StatusOK", status)
	}
}

// metricsTerminalCase drives one scripted establishment failure to the
// terminal state and returns the record set observed afterwards.
func metricsTerminalCase(t *testing.T, dependencies clientruntime.RemoteDependencies) []metricsWireRecord {
	t.Helper()
	resetSessionTable(t)
	handle := createSessionOK(t)
	installConnectTransport(t, dependencies)
	beginConnecting(t, handle, "127.0.0.1:25565")
	connectPollUntilStatus(t, handle, StatusDisconnected)
	records := metricsPullOnce(t, handle)
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after terminal = %d, want StatusOK", status)
	}
	return records
}

// metricsLoginDependencies builds transport dependencies whose dial succeeds
// on the fake stream and whose login seam returns the scripted error.
func metricsLoginDependencies(loginErr error) clientruntime.RemoteDependencies {
	return clientruntime.RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) {
			return &connectTestPacketStream{}, nil
		},
		Login: func(context.Context, network.ClientPacketStream, network.Identity, uint8) (network.ClientEndpoint, uint64, error) {
			return nil, 0, loginErr
		},
	}
}

// TestMetricsTerminalCauseClassificationMapsEveryProducerClass drives every
// real terminal-cause class through the fake transport: dial failure, server
// handshake rejection, server login rejection, client-detected protocol
// violation, and post-login receiver death; a clean user close records no
// cause, so its class is none.
func TestMetricsTerminalCauseClassificationMapsEveryProducerClass(t *testing.T) {
	cases := []struct {
		name  string
		deps  clientruntime.RemoteDependencies
		cause uint32
	}{
		{
			name: "dial failure",
			deps: clientruntime.RemoteDependencies{
				Dial: func(context.Context, string) (network.ClientPacketStream, error) {
					return nil, errors.New("metrics seam: connection refused")
				},
			},
			cause: TerminalCauseDial,
		},
		{
			name: "handshake rejection",
			deps: metricsLoginDependencies(&network.RemoteError{
				State: protocol.StateHandshake, Code: uint8(protocol.HandshakeVersionMismatch), Message: "version",
			}),
			cause: TerminalCauseHandshake,
		},
		{
			name: "login rejection",
			deps: metricsLoginDependencies(&network.RemoteError{
				State: protocol.StateLogin, Code: 1, Message: "busy",
			}),
			cause: TerminalCauseLogin,
		},
		{
			name:  "protocol violation",
			deps:  metricsLoginDependencies(errors.New("network: protocol violation: unexpected server handshake packet")),
			cause: TerminalCauseProtocol,
		},
	}
	for _, testCase := range cases {
		records := metricsTerminalCase(t, testCase.deps)
		if phase := metricsRecordValue(t, records, StatusRecordPhase); phase != uint64(ConnectPhaseDisconnected) {
			t.Fatalf("%s: phase = %d, want %d", testCase.name, phase, ConnectPhaseDisconnected)
		}
		if cause := metricsRecordValue(t, records, StatusRecordTerminalCause); cause != uint64(testCase.cause) {
			t.Fatalf("%s: terminal cause = %d, want %d", testCase.name, cause, testCase.cause)
		}
	}

	// Receiver death: the runtime records the terminal cause through
	// `recordRuntimeTerminal`, which classifies by provenance.
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	script.receiver.setErr(errors.New("metrics seam: receiver died"))
	connectPollUntilStatus(t, handle, StatusDisconnected)
	records := metricsPullOnce(t, handle)
	if cause := metricsRecordValue(t, records, StatusRecordTerminalCause); cause != uint64(TerminalCauseReceiver) {
		t.Fatalf("receiver death: terminal cause = %d, want %d", cause, TerminalCauseReceiver)
	}
	if phase := metricsRecordValue(t, records, StatusRecordPhase); phase != uint64(ConnectPhaseDisconnected) {
		t.Fatalf("receiver death: phase = %d, want %d", phase, ConnectPhaseDisconnected)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after receiver death = %d, want StatusOK", status)
	}

	// Clean close: a user disconnect records no cause, so the class is none
	// while the phase word carries the disconnect.
	script = newStepTransportScript()
	handle = connectSessionOnlineStep(t, script)
	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("clean disconnect = %d, want StatusOK", status)
	}
	records = metricsPullOnce(t, handle)
	if cause := metricsRecordValue(t, records, StatusRecordTerminalCause); cause != uint64(TerminalCauseNone) {
		t.Fatalf("clean close: terminal cause = %d, want %d", cause, TerminalCauseNone)
	}
	if phase := metricsRecordValue(t, records, StatusRecordPhase); phase != uint64(ConnectPhaseDisconnected) {
		t.Fatalf("clean close: phase = %d, want %d", phase, ConnectPhaseDisconnected)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after clean close = %d, want StatusOK", status)
	}
}

// TestMetricsClassifierUnitPinsRemainingShapes pins the classifier on producer
// error shapes the transport seams do not reach: the nil cause, the receiver
// factory failure, and an unrecognized error.
func TestMetricsClassifierUnitPinsRemainingShapes(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		cause uint32
	}{
		{"nil cause", nil, TerminalCauseNone},
		{"receiver factory failure", errors.New("runtime: create receiver: boom"), TerminalCauseInternal},
		{"unrecognized failure", errors.New("runtime: meshing is already configured"), TerminalCauseInternal},
		{"wrapped dial failure", errors.New("runtime: dial remote \"host:1\": connection refused"), TerminalCauseDial},
	}
	for _, testCase := range cases {
		if cause := classifyEstablishmentTerminal(testCase.err); cause != testCase.cause {
			t.Fatalf("%s: classified %d, want %d", testCase.name, cause, testCase.cause)
		}
	}
}

// TestMetricsTerminalSessionKeepsServingRecordsAndCounters pins the teardown
// ruling: after a terminal transition the status pull keeps reporting
// `StatusOK` with its record set, and counters earned before the terminal
// transition survive.
func TestMetricsTerminalSessionKeepsServingRecordsAndCounters(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	installWorldResultSeam(t, clientruntime.StepResult{Frame: frameNotReadySnapshot(1), MessagesProcessed: 6})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted step = %d, want StatusOK", status)
	}
	script.receiver.setErr(errors.New("metrics seam: receiver died"))
	connectPollUntilStatus(t, handle, StatusDisconnected)

	records := metricsPullOnce(t, handle)
	if got := metricsRecordValue(t, records, StatusRecordStepsCompleted); got != 1 {
		t.Fatalf("terminal steps completed = %d, want 1 (counters survive teardown)", got)
	}
	if got := metricsRecordValue(t, records, StatusRecordMessagesProcessed); got != 6 {
		t.Fatalf("terminal messages processed = %d, want 6", got)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy terminal = %d, want StatusOK", status)
	}
}

// TestMetricsTwoPhaseProtocolIsNonConsumingAndWritesExactly pins the two-phase
// protocol: repeated queries report the same size, repeated pulls return
// identical bytes without changing any counter, an insufficient capacity
// writes nothing and reports the fresh size, and an oversized capacity writes
// exactly with an untouched tail.
func TestMetricsTwoPhaseProtocolIsNonConsumingAndWritesExactly(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	installWorldResultSeam(t, clientruntime.StepResult{Frame: frameNotReadySnapshot(1), MessagesProcessed: 2})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("scripted step = %d, want StatusOK", status)
	}
	wantBytes := uint32(metricsWireBytes(4))

	for query := 0; query < 3; query++ {
		required, status := metricsQuerySize(t, handle)
		if status != StatusInsufficientCapacity || required != wantBytes {
			t.Fatalf("query %d = (%d, %d), want (%d, StatusInsufficientCapacity)", query, required, status, wantBytes)
		}
	}
	var reference []byte
	for pull := 0; pull < 3; pull++ {
		storage, required, status := metricsConsume(handle, wantBytes)
		if status != StatusOK {
			t.Fatalf("non-consuming pull %d = %d, want StatusOK", pull, status)
		}
		if required != uint32(0xDEADBEEF) {
			t.Fatalf("pull %d reported a size word %d; success reports no size", pull, required)
		}
		if pull > 0 && string(storage) != string(reference) {
			t.Fatalf("pull %d diverged from the first pull; a metrics pull must not consume", pull)
		}
		reference = storage
	}
	if got := metricsRecordValue(t, decodeMetricsWire(t, reference), StatusRecordStepsCompleted); got != 1 {
		t.Fatalf("steps completed after repeated pulls = %d, want 1 (pulls consume nothing)", got)
	}

	for _, capacity := range []uint32{1, wantBytes - 1, wantBytes - 8} {
		storage, required, status := metricsConsume(handle, capacity)
		if status != StatusInsufficientCapacity {
			t.Fatalf("capacity %d status = %d, want StatusInsufficientCapacity", capacity, status)
		}
		if required != wantBytes {
			t.Fatalf("capacity %d reported required = %d, want %d", capacity, required, wantBytes)
		}
		assertMetricsCanary(t, storage, "insufficient-capacity metrics pull")
	}

	oversized, _, status := metricsConsume(handle, wantBytes+17)
	if status != StatusOK {
		t.Fatalf("oversized pull = %d, want StatusOK", status)
	}
	for index := int(wantBytes); index < len(oversized); index++ {
		if oversized[index] != 0xA5 {
			t.Fatalf("oversized write touched tail byte %d beyond the required size", index)
		}
	}
	if string(oversized[:wantBytes]) != string(reference) {
		t.Fatal("oversized write diverges from the exact record")
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the metrics walk = %d, want StatusOK", status)
	}
}

// TestMetricsValidatesHandleAndPointerShapeBeforeContent pins the validation
// order: the handle lifecycle outranks every pointer defect and the pointer
// shape outranks content; every rejection leaves the record set unchanged, and
// the size out-parameter may not alias the write buffer's declared span.
func TestMetricsValidatesHandleAndPointerShapeBeforeContent(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	if status := coreStatusPull(0, nil, 0, nil); status != StatusInvalidHandle {
		t.Fatalf("metrics pull on the zero handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreStatusPull(makeSessionHandle(3, 1), nil, 0, nil); status != StatusInvalidHandle {
		t.Fatalf("metrics pull on a never-issued handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreStatusPull(destroyed, nil, 0, nil); status != StatusInvalidState {
		t.Fatalf("metrics pull on a destroyed handle = %d, want StatusInvalidState", status)
	}
	if status := coreStatusPull(idle, nil, 0, nil); status != StatusInvalidArgument {
		t.Fatalf("metrics pull with a null size out-parameter = %d, want StatusInvalidArgument", status)
	}
	if status := coreStatusPull(idle, nil, 64, new(uint32)); status != StatusInvalidArgument {
		t.Fatalf("metrics pull with a null buffer and positive capacity = %d, want StatusInvalidArgument", status)
	}

	wantBytes := uint32(metricsWireBytes(4))
	for _, aliasOffset := range []uintptr{0, 8, uintptr(wantBytes - 4)} {
		storage := poisonedBuffer(int(wantBytes))
		aliasedSize := (*uint32)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), aliasOffset)))
		if status := coreStatusPull(idle, &storage[0], wantBytes, aliasedSize); status != StatusInvalidArgument {
			t.Fatalf("metrics pull with the size word at buffer offset %d = %d, want StatusInvalidArgument", aliasOffset, status)
		}
		assertMetricsCanary(t, storage, "aliased size-word metrics pull")
	}
	misalignedStorage := poisonedBuffer(int(wantBytes) + 32)
	misaligned := (*byte)(unsafe.Pointer(&misalignedStorage[1]))
	misalignedSize := (*uint32)(unsafe.Pointer(&misalignedStorage[int(wantBytes)+24]))
	*misalignedSize = uint32(0xDEADBEEF)
	if status := coreStatusPull(idle, misaligned, wantBytes, misalignedSize); status != StatusInvalidArgument {
		t.Fatalf("metrics pull with a misaligned buffer = %d, want StatusInvalidArgument", status)
	}
	if *misalignedSize != 0xDEADBEEF {
		t.Fatalf("shape rejection wrote the size out-parameter %d despite failure", *misalignedSize)
	}
	assertMetricsCanary(t, misalignedStorage[:int(wantBytes)+1], "misaligned metrics pull")

	storage := poisonedBuffer(int(wantBytes) + 8)
	outsideSize := (*uint32)(unsafe.Pointer(unsafe.Pointer(&storage[wantBytes])))
	if status := coreStatusPull(idle, &storage[0], wantBytes, outsideSize); status != StatusOK {
		t.Fatalf("metrics pull with the size word just past the span = %d, want StatusOK", status)
	}
	if *outsideSize != uint32(0xA5A5A5A5) {
		t.Fatalf("successful pull wrote the size word %#x; success reports no size", *outsideSize)
	}
	if status := coreDestroy(idle); status != StatusOK {
		t.Fatalf("destroy idle = %d, want StatusOK", status)
	}
}

// TestMetricsPanicConversionWritesNothing injects a panic through the encoder
// seam: the export converts it to `StatusPanic`, writes neither the buffer nor
// the size word, and a later pull serves the unchanged record set.
func TestMetricsPanicConversionWritesNothing(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	wantBytes := uint32(metricsWireBytes(4))

	before := metricsPullOnce(t, idle)
	buffer, sizeOut := metricsCallerFrame(wantBytes)
	*sizeOut = uint32(0xDEADBEEF)
	previous := statusEncode
	statusEncode = func(statusSnapshot) []byte { panic("injected status encoder panic") }
	status := coreStatusPull(idle, &buffer[0], wantBytes, sizeOut)
	statusEncode = previous
	if status != StatusPanic {
		t.Fatalf("pull during the injected panic = %d, want StatusPanic", status)
	}
	if *sizeOut != 0xDEADBEEF {
		t.Fatalf("panic wrote the size out-parameter %d", *sizeOut)
	}
	assertMetricsCanary(t, buffer, "panicking metrics pull")

	after := metricsPullOnce(t, idle)
	for index := range before {
		if before[index] != after[index] {
			t.Fatalf("record %d changed across a converted panic", index)
		}
	}
	if status := coreDestroy(idle); status != StatusOK {
		t.Fatalf("destroy after the panic walk = %d, want StatusOK", status)
	}
}
