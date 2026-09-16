package main

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"

	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
)

// The input tests pin the per-frame input batch surface: the frozen MCN1
// header plus fixed 16-byte event records, the whole-batch atomic decode into
// exactly one `runtime.SemanticInput`, the validation order from handle
// lifecycle through session state, pointer shape, and batch content to the
// runtime submission, and the whole-batch rejection ruling (one invalid record
// rejects the batch with no submission). Online sessions come from the same
// scripted transport the connect tests use, so no test opens a socket; the
// `submitSemanticInput` seam records what the runtime would receive.

// inputSubmitLog records every aggregated intent handed through the submit
// seam so tests can prove both the exact submission content and that a
// rejected batch reached the runtime zero times.
type inputSubmitLog struct {
	mu     sync.Mutex
	inputs []clientruntime.SemanticInput
}

func (log *inputSubmitLog) record(input clientruntime.SemanticInput) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.inputs = append(log.inputs, input)
}

func (log *inputSubmitLog) count() int {
	log.mu.Lock()
	defer log.mu.Unlock()
	return len(log.inputs)
}

func (log *inputSubmitLog) recorded() []clientruntime.SemanticInput {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]clientruntime.SemanticInput(nil), log.inputs...)
}

// installInputSubmitSeam replaces `submitSemanticInput` for one test and
// restores the production submission afterwards; package tests run
// sequentially, so the swap is race-free.
func installInputSubmitSeam(t *testing.T, submit func(established *clientruntime.Runtime, input clientruntime.SemanticInput) error) *inputSubmitLog {
	t.Helper()
	log := &inputSubmitLog{}
	previous := submitSemanticInput
	submitSemanticInput = func(established *clientruntime.Runtime, input clientruntime.SemanticInput) error {
		log.record(input)
		return submit(established, input)
	}
	t.Cleanup(func() { submitSemanticInput = previous })
	return log
}

// connectSessionOnlineInput drives one session through the scripted transport
// to the online phase and returns its handle.
func connectSessionOnlineInput(t *testing.T) uint64 {
	t.Helper()
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())
	beginConnecting(t, handle, "127.0.0.1:25565")
	<-script.dialEntered
	close(script.dialRelease)
	connectPollUntilPhase(t, handle, ConnectPhaseLoading)
	return handle
}

// inputEventWords is one wire event record's three content words; the fourth
// record word is the zero reserved word.
type inputEventWords struct {
	action uint32
	value  uint32
	aux    uint32
}

// inputMoveEvent builds the strafe/forward axis event.
func inputMoveEvent(moveX, moveZ int32) inputEventWords {
	return inputEventWords{action: InputActionMove, value: uint32(moveX), aux: uint32(moveZ)}
}

// inputLookEvent builds the absolute look-pose event from two float bit
// patterns, mirroring the wire encoding.
func inputLookEvent(yaw, pitch float32) inputEventWords {
	return inputEventWords{action: InputActionLook, value: math.Float32bits(yaw), aux: math.Float32bits(pitch)}
}

// inputStateEvent builds one boolean action event.
func inputStateEvent(action, state uint32) inputEventWords {
	return inputEventWords{action: action, value: state}
}

// inputBatchPointer encodes one well-formed batch into fresh 8-byte-aligned
// storage, as a C caller's header-plus-records buffer would be aligned.
func inputBatchPointer(events ...inputEventWords) (*byte, uint32) {
	length := int(InputHeaderBytes) + len(events)*int(InputEventBytes)
	storage := make([]uint64, length/8+2)
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&storage[0])), length)
	encodeInputBatch(buffer, events)
	return &buffer[0], uint32(length)
}

// encodeInputBatch writes the input header and every event record into buffer.
func encodeInputBatch(buffer []byte, events []inputEventWords) {
	binary.LittleEndian.PutUint32(buffer[0:4], uint32(MagicInput))
	binary.LittleEndian.PutUint32(buffer[4:8], InputVersion)
	binary.LittleEndian.PutUint32(buffer[8:12], uint32(len(events)))
	// The reserved word at buffer[12:16] and every record reserved word stay
	// zero because the storage is freshly allocated.
	offset := int(InputHeaderBytes)
	for _, event := range events {
		binary.LittleEndian.PutUint32(buffer[offset:offset+4], event.action)
		binary.LittleEndian.PutUint32(buffer[offset+4:offset+8], event.value)
		binary.LittleEndian.PutUint32(buffer[offset+8:offset+12], event.aux)
		offset += int(InputEventBytes)
	}
}

// inputBatchWords writes a batch with an explicit header event count and raw
// record content, for hand-crafted invalid batches.
func inputBatchWords(eventCount uint32, records ...inputEventWords) (*byte, uint32) {
	length := int(InputHeaderBytes) + len(records)*int(InputEventBytes)
	storage := make([]uint64, length/8+2)
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&storage[0])), length)
	binary.LittleEndian.PutUint32(buffer[0:4], uint32(MagicInput))
	binary.LittleEndian.PutUint32(buffer[4:8], InputVersion)
	binary.LittleEndian.PutUint32(buffer[8:12], eventCount)
	for index, event := range records {
		offset := int(InputHeaderBytes) + index*int(InputEventBytes)
		binary.LittleEndian.PutUint32(buffer[offset:offset+4], event.action)
		binary.LittleEndian.PutUint32(buffer[offset+4:offset+8], event.value)
		binary.LittleEndian.PutUint32(buffer[offset+8:offset+12], event.aux)
	}
	return &buffer[0], uint32(length)
}

// patchInputWord overwrites one little-endian word of a built batch so a test
// can corrupt exactly one header or record field.
func patchInputWord(buffer *byte, offset int, value uint32) {
	view := unsafe.Slice(buffer, offset+4)
	binary.LittleEndian.PutUint32(view[offset:offset+4], value)
}

// TestInputWireVocabularyPinsPilotValues pins the producer-side input wire
// vocabulary to explicit literals: the fixed event record size, the seven
// action codes in their frozen order, and the maximal batch length derived
// from the frozen header event limit. The Rust consumer mirrors these values
// when its input bridge lands; moving any of them silently fails here.
func TestInputWireVocabularyPinsPilotValues(t *testing.T) {
	if InputEventBytes != 16 {
		t.Fatalf("input event record bytes = %d, want the pinned literal 16", InputEventBytes)
	}
	actions := []struct {
		value uint32
		want  uint32
		name  string
	}{
		{InputActionMove, 1, "move"},
		{InputActionLook, 2, "look"},
		{InputActionJump, 3, "jump"},
		{InputActionMining, 4, "mining"},
		{InputActionEating, 5, "eating"},
		{InputActionSprinting, 6, "sprinting"},
		{InputActionSneaking, 7, "sneaking"},
	}
	for _, action := range actions {
		if action.value != action.want {
			t.Fatalf("input action %s = %d, want the pinned literal %d", action.name, action.value, action.want)
		}
	}
	if max := InputBatchMaxBytes; max != InputHeaderBytes+MaxInputEvents*InputEventBytes {
		t.Fatalf("maximal input batch = %d, want header %d plus %d events of %d bytes",
			max, InputHeaderBytes, MaxInputEvents, InputEventBytes)
	}
	if max := InputBatchMaxBytes; max != 2064 {
		t.Fatalf("maximal input batch = %d, want the pinned literal 2064", max)
	}
}

// TestInputSubmitAggregatesOneFrameIntoSingleSemanticInput proves the
// one-batch-one-frame ruling: every event kind folds into one aggregate with
// last-event-wins per kind, and exactly one submission reaches the online
// runtime carrying the complete frame intent.
func TestInputSubmitAggregatesOneFrameIntoSingleSemanticInput(t *testing.T) {
	handle := connectSessionOnlineInput(t)
	log := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error { return nil })

	pointer, length := inputBatchPointer(
		inputMoveEvent(1, -1),
		inputLookEvent(0.75, -0.25),
		inputStateEvent(InputActionJump, 1),
		inputStateEvent(InputActionMining, 1),
		inputStateEvent(InputActionEating, 1),
		inputStateEvent(InputActionSprinting, 1),
		inputStateEvent(InputActionSneaking, 1),
		inputMoveEvent(0, 1),
	)
	if status := coreSubmitInput(handle, pointer, length); status != StatusOK {
		t.Fatalf("submit of a full frame = %d, want StatusOK", status)
	}
	if count := log.count(); count != 1 {
		t.Fatalf("one accepted batch produced %d submissions, want exactly 1", count)
	}
	want := clientruntime.SemanticInput{
		MoveX: 0, MoveZ: 1,
		Jump: true,
		Yaw:  0.75, Pitch: -0.25,
		Mining: true, Eating: true, Sprinting: true, Sneaking: true,
	}
	recorded := log.recorded()
	if len(recorded) != 1 || recorded[0] != want {
		t.Fatalf("submitted aggregate = %+v, want %+v (last move event wins)", recorded, want)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after submit = %d, want StatusOK", status)
	}
}

// TestInputSubmitEmptyBatchSubmitsZeroIntent pins the empty-batch ruling: a
// header with zero events is one valid frame that submits the zero intent,
// because the host sends the complete per-frame state and a frame with no
// events means no control input.
func TestInputSubmitEmptyBatchSubmitsZeroIntent(t *testing.T) {
	handle := connectSessionOnlineInput(t)
	log := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error { return nil })

	pointer, length := inputBatchPointer()
	if length != InputHeaderBytes {
		t.Fatalf("empty batch length = %d, want the header size %d", length, InputHeaderBytes)
	}
	if status := coreSubmitInput(handle, pointer, length); status != StatusOK {
		t.Fatalf("submit of an empty frame = %d, want StatusOK", status)
	}
	if count := log.count(); count != 1 {
		t.Fatalf("empty batch produced %d submissions, want 1", count)
	}
	if recorded := log.recorded(); len(recorded) != 1 || recorded[0] != (clientruntime.SemanticInput{}) {
		t.Fatalf("empty batch aggregate = %+v, want the zero semantic input", recorded)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after empty submit = %d, want StatusOK", status)
	}
}

// TestInputSubmitRejectsWrongMagicAndLayoutAsABIMismatch pins the record
// identity rule: a buffer whose magic is not the input family tag or whose
// layout word is not the input family version reports `StatusABIMismatch`,
// before any record content is judged.
func TestInputSubmitRejectsWrongMagicAndLayoutAsABIMismatch(t *testing.T) {
	handle := connectSessionOnlineInput(t)
	log := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error { return nil })

	cases := []struct {
		name   string
		offset int
		value  uint32
	}{
		{"wrong family magic", 0, uint32(MagicConnection)},
		{"zero magic", 0, 0},
		{"newer layout", 4, InputVersion + 1},
		{"zero layout", 4, 0},
	}
	for _, testCase := range cases {
		pointer, length := inputBatchPointer(
			inputMoveEvent(0, 1),
			inputStateEvent(InputActionJump, 1),
		)
		patchInputWord(pointer, testCase.offset, testCase.value)
		if status := coreSubmitInput(handle, pointer, length); status != StatusABIMismatch {
			t.Fatalf("submit with %s = %d, want StatusABIMismatch", testCase.name, status)
		}
	}
	// A record identity defect outranks an invalid record later in the batch.
	pointer, length := inputBatchPointer(inputEventWords{action: 99})
	patchInputWord(pointer, 0, uint32(MagicStep))
	if status := coreSubmitInput(handle, pointer, length); status != StatusABIMismatch {
		t.Fatalf("submit with wrong magic before an invalid record = %d, want StatusABIMismatch", status)
	}
	if count := log.count(); count != 0 {
		t.Fatalf("identity rejections produced %d submissions, want 0", count)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after identity rejections = %d, want StatusOK", status)
	}
}

// TestInputSubmitValidatesPointerAndLengthShape pins the argument contract:
// a null buffer, a length below the header size, a length above the maximal
// batch, or a pointer not aligned to `ABIAlignment` reports
// `StatusInvalidArgument`, the oversized length is rejected before any buffer
// byte is read, and no rejection submits anything. The input family has no
// output pointer, so no input-output overlap rule applies beyond this shape
// check.
func TestInputSubmitValidatesPointerAndLengthShape(t *testing.T) {
	handle := connectSessionOnlineInput(t)
	log := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error { return nil })

	pointer, length := inputBatchPointer(inputMoveEvent(0, 1))
	if status := coreSubmitInput(handle, nil, length); status != StatusInvalidArgument {
		t.Fatalf("submit with null buffer = %d, want StatusInvalidArgument", status)
	}
	if status := coreSubmitInput(handle, pointer, 0); status != StatusInvalidArgument {
		t.Fatalf("submit with zero length = %d, want StatusInvalidArgument", status)
	}
	if status := coreSubmitInput(handle, pointer, InputHeaderBytes-1); status != StatusInvalidArgument {
		t.Fatalf("submit with a short length = %d, want StatusInvalidArgument", status)
	}
	// The lying length exceeds the maximal batch while the storage is far
	// smaller, so the rejection must come from the length bound alone.
	if status := coreSubmitInput(handle, pointer, InputBatchMaxBytes+1); status != StatusInvalidArgument {
		t.Fatalf("submit with an oversized length = %d, want StatusInvalidArgument", status)
	}
	storage := make([]byte, int(length)+8)
	copy(storage[1:], unsafe.Slice(pointer, int(length)))
	misaligned := (*byte)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), 1)))
	if status := coreSubmitInput(handle, misaligned, length); status != StatusInvalidArgument {
		t.Fatalf("submit with a misaligned buffer = %d, want StatusInvalidArgument", status)
	}
	if count := log.count(); count != 0 {
		t.Fatalf("shape rejections produced %d submissions, want 0", count)
	}
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(handle, &phase); status != StatusOK || phase != ConnectPhaseLoading {
		t.Fatalf("poll after shape rejections = (%d, %d), want StatusOK and loading", status, phase)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after shape rejections = %d, want StatusOK", status)
	}
}

// TestInputSubmitRejectsHeaderContentViolations pins the header content
// domain: a nonzero reserved word, an event count above `MaxInputEvents`, a
// body shorter than the declared count, and trailing bytes after the last
// record each reject the whole batch with `StatusInputRejected` and no
// submission.
func TestInputSubmitRejectsHeaderContentViolations(t *testing.T) {
	handle := connectSessionOnlineInput(t)
	log := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error { return nil })

	reservedPointer, reservedLength := inputBatchPointer(inputMoveEvent(0, 1))
	patchInputWord(reservedPointer, 12, 1)
	if status := coreSubmitInput(handle, reservedPointer, reservedLength); status != StatusInputRejected {
		t.Fatalf("submit with nonzero header reserved = %d, want StatusInputRejected", status)
	}

	overCountPointer, overCountLength := inputBatchWords(MaxInputEvents+1, inputMoveEvent(0, 1))
	if status := coreSubmitInput(handle, overCountPointer, overCountLength); status != StatusInputRejected {
		t.Fatalf("submit with event count %d = %d, want StatusInputRejected", MaxInputEvents+1, status)
	}

	shortPointer, shortLength := inputBatchWords(3, inputMoveEvent(0, 1), inputMoveEvent(0, 1))
	if status := coreSubmitInput(handle, shortPointer, shortLength); status != StatusInputRejected {
		t.Fatalf("submit with a body shorter than the declared count = %d, want StatusInputRejected", status)
	}

	fullPointer, fullLength := inputBatchPointer(inputMoveEvent(0, 1))
	for trailing := uint32(1); trailing < InputEventBytes; trailing++ {
		if status := coreSubmitInput(handle, fullPointer, fullLength+trailing); status != StatusInputRejected {
			t.Fatalf("submit with %d trailing bytes = %d, want StatusInputRejected", trailing, status)
		}
	}
	if count := log.count(); count != 0 {
		t.Fatalf("header content rejections produced %d submissions, want 0", count)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after header rejections = %d, want StatusOK", status)
	}
}

// TestInputSubmitAcceptsBoundaryEventCounts proves the exact event-count
// boundary: a batch declaring exactly `MaxInputEvents` records submits once,
// and the rejection above belongs to content while a consistent body for more
// than the limit exceeds the length shape bound.
func TestInputSubmitAcceptsBoundaryEventCounts(t *testing.T) {
	handle := connectSessionOnlineInput(t)
	log := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error { return nil })

	events := make([]inputEventWords, MaxInputEvents)
	for index := range events {
		events[index] = inputMoveEvent(1, 0)
	}
	pointer, length := inputBatchPointer(events...)
	if status := coreSubmitInput(handle, pointer, length); status != StatusOK {
		t.Fatalf("submit of the %d-event boundary batch = %d, want StatusOK", MaxInputEvents, status)
	}
	if count := log.count(); count != 1 {
		t.Fatalf("boundary batch produced %d submissions, want 1", count)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the boundary batch = %d, want StatusOK", status)
	}
}

// TestInputSubmitRejectsInvalidMiddleEventWholeBatchAtomic is the whole-batch
// atomicity proof: with valid records before and after one invalid middle
// record, the batch reports `StatusInputRejected` and the runtime receives
// nothing. Every record-level defect class is covered.
func TestInputSubmitRejectsInvalidMiddleEventWholeBatchAtomic(t *testing.T) {
	handle := connectSessionOnlineInput(t)
	log := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error { return nil })

	invalidMiddle := []struct {
		name  string
		event inputEventWords
	}{
		{"zero action", inputEventWords{action: 0, value: 1}},
		{"unknown action", inputEventWords{action: InputActionSneaking + 1, value: 1}},
		{"strafe axis above one", inputMoveEvent(2, 0)},
		{"forward axis below minus one", inputMoveEvent(0, -2)},
		{"large axis value", inputMoveEvent(999, -999)},
		{"boolean state value two", inputStateEvent(InputActionJump, 2)},
		{"boolean auxiliary word", inputEventWords{action: InputActionJump, value: 1, aux: 1}},
		{"NaN yaw", inputLookEvent(float32(math.NaN()), 0)},
		{"NaN pitch", inputLookEvent(0, float32(math.NaN()))},
		{"positive infinite yaw", inputLookEvent(float32(math.Inf(1)), 0)},
		{"negative infinite pitch", inputLookEvent(0, float32(math.Inf(-1)))},
	}
	for index, testCase := range invalidMiddle {
		pointer, length := inputBatchPointer(
			inputMoveEvent(1, 1),
			testCase.event,
			inputLookEvent(0.5, -0.5),
			inputStateEvent(InputActionSprinting, 1),
		)
		if status := coreSubmitInput(handle, pointer, length); status != StatusInputRejected {
			t.Fatalf("case %d (%s) = %d, want StatusInputRejected", index, testCase.name, status)
		}
		if count := log.count(); count != 0 {
			t.Fatalf("case %d (%s) still reached the runtime %d times, want 0 submissions",
				index, testCase.name, count)
		}
	}
	// A nonzero record reserved word in the middle of the batch rejects it
	// whole as well; the record's own action words stay valid.
	pointer, length := inputBatchPointer(
		inputMoveEvent(1, 1),
		inputStateEvent(InputActionJump, 1),
		inputLookEvent(0.5, -0.5),
	)
	patchInputWord(pointer, int(InputHeaderBytes)+int(InputEventBytes)+12, 1)
	if status := coreSubmitInput(handle, pointer, length); status != StatusInputRejected {
		t.Fatalf("submit with a nonzero middle record reserved word = %d, want StatusInputRejected", status)
	}
	if count := log.count(); count != 0 {
		t.Fatalf("the reserved-word rejection reached the runtime %d times, want 0", count)
	}
	// After every rejection the session stays online and a valid batch still
	// submits exactly once, proving no partial state was consumed.
	pointer, length = inputBatchPointer(inputMoveEvent(1, 0))
	if status := coreSubmitInput(handle, pointer, length); status != StatusOK {
		t.Fatalf("valid submit after rejections = %d, want StatusOK", status)
	}
	if count := log.count(); count != 1 {
		t.Fatalf("valid submit after rejections produced %d submissions, want 1", count)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after whole-batch rejections = %d, want StatusOK", status)
	}
}

// TestInputSubmitValidatesHandleAndStateBeforeArgumentsAndContent pins the
// validation order: an unknown or destroyed handle outranks every argument
// defect, the session state (idle, connecting, terminal) outranks the buffer
// shape and content, and only an online session reaches the shape checks.
func TestInputSubmitValidatesHandleAndStateBeforeArgumentsAndContent(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	log := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error { return nil })

	if status := coreSubmitInput(0, nil, 0); status != StatusInvalidHandle {
		t.Fatalf("submit on the zero handle with a null buffer = %d, want StatusInvalidHandle", status)
	}
	if status := coreSubmitInput(makeSessionHandle(3, 1), nil, 0); status != StatusInvalidHandle {
		t.Fatalf("submit on a never-issued handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreSubmitInput(destroyed, nil, 0); status != StatusInvalidState {
		t.Fatalf("submit on a destroyed handle = %d, want StatusInvalidState", status)
	}

	validPointer, validLength := inputBatchPointer(inputMoveEvent(0, 1))
	if status := coreSubmitInput(idle, validPointer, validLength); status != StatusInvalidState {
		t.Fatalf("submit on an idle session with a valid buffer = %d, want StatusInvalidState", status)
	}
	if status := coreSubmitInput(idle, nil, 0); status != StatusInvalidState {
		t.Fatalf("submit on an idle session with a null buffer = %d, want StatusInvalidState (state precedes shape)", status)
	}
	if status := coreDestroy(idle); status != StatusOK {
		t.Fatalf("destroy of the idle session = %d, want StatusOK", status)
	}

	connecting := connectSessionOnlineInputConnecting(t)
	if status := coreSubmitInput(connecting, validPointer, validLength); status != StatusInvalidState {
		t.Fatalf("submit while connecting with a valid buffer = %d, want StatusInvalidState", status)
	}
	if status := coreSubmitInput(connecting, nil, 0); status != StatusInvalidState {
		t.Fatalf("submit while connecting with a null buffer = %d, want StatusInvalidState (state precedes shape)", status)
	}
	if status := coreDisconnect(connecting); status != StatusOK {
		t.Fatalf("disconnect of the connecting session = %d, want StatusOK", status)
	}

	online := connectSessionOnlineInput(t)
	if status := coreDisconnect(online); status != StatusOK {
		t.Fatalf("disconnect of the online session = %d, want StatusOK", status)
	}
	if status := coreSubmitInput(online, validPointer, validLength); status != StatusDisconnected {
		t.Fatalf("submit on a terminal session with a valid buffer = %d, want StatusDisconnected", status)
	}
	if count := log.count(); count != 0 {
		t.Fatalf("lifecycle rejections produced %d submissions, want 0", count)
	}
	if status := coreDestroy(online); status != StatusOK {
		t.Fatalf("destroy of the online session = %d, want StatusOK", status)
	}
}

// connectSessionOnlineInputConnecting leaves one session blocked inside the
// dial so it stays in the connecting phase.
func connectSessionOnlineInputConnecting(t *testing.T) uint64 {
	t.Helper()
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())
	beginConnecting(t, handle, "127.0.0.1:25565")
	<-script.dialEntered
	return handle
}

// TestInputSubmitMapsSeamRejectionPanicAndDisconnectRace pins the submission
// outcome mapping: a runtime rejection maps to `StatusInputRejected`, a panic
// inside the submission seam converts to `StatusPanic`, and a rejection that
// races a disconnect maps to `StatusDisconnected` because the session reached
// its terminal state while the submission was in flight.
func TestInputSubmitMapsSeamRejectionPanicAndDisconnectRace(t *testing.T) {
	handle := connectSessionOnlineInput(t)
	rejected := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error {
		return errInputSeamRejected
	})
	pointer, length := inputBatchPointer(inputMoveEvent(1, 0))
	if status := coreSubmitInput(handle, pointer, length); status != StatusInputRejected {
		t.Fatalf("submit with a rejecting runtime = %d, want StatusInputRejected", status)
	}
	if count := rejected.count(); count != 1 {
		t.Fatalf("rejecting seam saw %d submissions, want 1 (the batch itself was valid)", count)
	}

	panicking := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error {
		panic("input seam panic")
	})
	if status := coreSubmitInput(handle, pointer, length); status != StatusPanic {
		t.Fatalf("submit with a panicking seam = %d, want StatusPanic", status)
	}
	if count := panicking.count(); count != 1 {
		t.Fatalf("panicking seam saw %d submissions, want 1", count)
	}

	racing := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error {
		if status := coreDisconnect(handle); status != StatusOK {
			t.Errorf("disconnect inside the seam = %d, want StatusOK", status)
		}
		return errInputSeamRejected
	})
	if status := coreSubmitInput(handle, pointer, length); status != StatusDisconnected {
		t.Fatalf("submit whose rejection raced a disconnect = %d, want StatusDisconnected", status)
	}
	if count := racing.count(); count != 1 {
		t.Fatalf("racing seam saw %d submissions, want 1", count)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the mapping cases = %d, want StatusOK", status)
	}
}

// errInputSeamRejected distinguishes the scripted seam rejection from real
// runtime errors in the mapping tests.
var errInputSeamRejected = errors.New("input seam: scripted rejection")

// TestInputSubmitRealRuntimeAcceptsAndRejectsSemantics runs the production
// submission path against a real online runtime built through the scripted
// transport: a valid batch submits successfully, a look pose outside the
// runtime pitch domain is rejected through the real semantic validation, and
// the rejection consumes no state because the next valid batch still submits.
func TestInputSubmitRealRuntimeAcceptsAndRejectsSemantics(t *testing.T) {
	handle := connectSessionOnlineInput(t)

	validPointer, validLength := inputBatchPointer(
		inputMoveEvent(1, -1),
		inputLookEvent(2.5, -0.5),
		inputStateEvent(InputActionJump, 1),
	)
	if status := coreSubmitInput(handle, validPointer, validLength); status != StatusOK {
		t.Fatalf("real-runtime submit of a valid frame = %d, want StatusOK", status)
	}
	outOfDomainPointer, outOfDomainLength := inputBatchPointer(
		inputLookEvent(0, float32(math.Pi/2)),
	)
	if status := coreSubmitInput(handle, outOfDomainPointer, outOfDomainLength); status != StatusInputRejected {
		t.Fatalf("real-runtime submit with an out-of-domain pitch = %d, want StatusInputRejected", status)
	}
	if status := coreSubmitInput(handle, validPointer, validLength); status != StatusOK {
		t.Fatalf("real-runtime submit after a rejection = %d, want StatusOK (rejections consume no state)", status)
	}
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(handle, &phase); status != StatusOK || phase != ConnectPhaseLoading {
		t.Fatalf("poll after real submits = (%d, %d), want StatusOK and loading", status, phase)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after real submits = %d, want StatusOK", status)
	}
}

// TestInputSubmitConcurrentWithPollAndDisconnectStaysWithinVocabulary drives
// concurrent submits, polls, and one racing disconnect on an online session:
// every observed status stays inside the documented vocabulary, and the
// number of accepted submissions equals the number of submissions the seam
// observed, so no submit silently vanishes or double-submits.
func TestInputSubmitConcurrentWithPollAndDisconnectStaysWithinVocabulary(t *testing.T) {
	handle := connectSessionOnlineInput(t)
	var accepted atomic.Uint64
	log := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error { return nil })

	const submitters = 4
	const iterations = 200
	var group sync.WaitGroup
	stop := make(chan struct{})
	for worker := 0; worker < submitters; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			pointer, length := inputBatchPointer(inputMoveEvent(int32(worker%3-1), 1))
			for iteration := 0; iteration < iterations; iteration++ {
				switch status := coreSubmitInput(handle, pointer, length); status {
				case StatusOK:
					accepted.Add(1)
				case StatusInvalidState, StatusDisconnected:
					return
				default:
					t.Errorf("submit %d of worker %d = %d, want a documented status", iteration, worker, status)
					return
				}
				select {
				case <-stop:
					return
				default:
				}
			}
		}(worker)
	}
	for poller := 0; poller < 2; poller++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				phase := uint32(0xDEADBEEF)
				switch status := coreConnectPoll(handle, &phase); status {
				case StatusOK:
					if phase != ConnectPhaseLoading && phase != ConnectPhasePlay {
						t.Errorf("poll observed phase %d, want loading or play", phase)
					}
				case StatusDisconnected:
				default:
					t.Errorf("poll = %d, want StatusOK or StatusDisconnected", status)
					return
				}
				select {
				case <-stop:
					return
				default:
				}
			}
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		if status := coreDisconnect(handle); status != StatusOK {
			t.Errorf("racing disconnect = %d, want StatusOK", status)
		}
	}()
	group.Wait()
	close(stop)

	if got := accepted.Load(); uint64(log.count()) != got {
		t.Fatalf("seam observed %d submissions while %d submits returned StatusOK", log.count(), got)
	}
	connectPollUntilStatus(t, handle, StatusDisconnected)
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the race = %d, want StatusOK", status)
	}
}
