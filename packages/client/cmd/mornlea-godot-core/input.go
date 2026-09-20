//go:build cgo

package main

import (
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/channing771/mornlea/packages/client/runtime"
)

// This file owns the input family export surface: the per-frame device event
// batch (`coreSubmitInput`) decoded from the frozen MCN1 header plus fixed
// event records into exactly one `runtime.SemanticInput` submission. The
// whole batch is validated and decoded into owned memory before anything is
// submitted, so one invalid record rejects the batch with no partial state
// change; the calling thread performs only bounded work and never blocks on
// the network.

// Wire layout of one input event record, in little-endian words:
//
//	offset 0   action   u32   one `InputAction*` code
//	offset 4   value    u32   move strafe axis, boolean state, or yaw bits
//	offset 8   aux      u32   move forward axis or pitch bits; else zero
//	offset 12  reserved u32   must be zero; validated
//
// `InputEventBytes` is producer-side wire vocabulary for the input family,
// like the connection phase words in connect.go: the frozen header defines
// the batch header and event limit, while the record layout and action codes
// are defined here by the producer and will be pinned by the Rust consumer
// when its input bridge lands. The registry descriptor for the input family
// deliberately keeps `RecordBytes` at zero in this generation because both
// language pin suites pin that table; switching the descriptor to this fixed
// size is a coordinated cross-language update owned by the consumer task.
const InputEventBytes = 16

// Input action codes, one per `runtime.SemanticInput` field group. The codes
// map exactly onto the semantic domain the runtime validates and invent no
// action the runtime cannot consume: there is deliberately no text, chat, or
// hotbar record because `runtime.SemanticInput` has no such channel, so no
// text-capacity bound exists to define. Text and command actions arrive as a
// reviewed compatible family addition when the runtime gains their channel.
// Codes are frozen: new codes append and none is repurposed.
const (
	// InputActionMove carries the strafe axis in `value` and the forward
	// axis in `aux`, each in {-1, 0, 1}.
	InputActionMove uint32 = 1
	// InputActionLook carries the absolute semantic look pose: the yaw
	// float bits in `value` and the pitch float bits in `aux`.
	InputActionLook uint32 = 2
	// InputActionJump carries the jump state in `value`, 0 or 1.
	InputActionJump uint32 = 3
	// InputActionMining carries the mining state in `value`, 0 or 1.
	InputActionMining uint32 = 4
	// InputActionEating carries the eating state in `value`, 0 or 1.
	InputActionEating uint32 = 5
	// InputActionSprinting carries the sprinting state in `value`, 0 or 1.
	InputActionSprinting uint32 = 6
	// InputActionSneaking carries the sneaking state in `value`, 0 or 1.
	InputActionSneaking uint32 = 7
)

// InputBatchMaxBytes is the largest well-formed batch: the frozen header plus
// `MaxInputEvents` fixed records. A length above it is an argument-shape
// defect rejected before any buffer byte is read.
const InputBatchMaxBytes = InputHeaderBytes + MaxInputEvents*InputEventBytes

// One-batch-one-frame ruling: `runtime.SubmitInput` copies a whole-frame
// control intent over the previous one (it overwrites `semanticInput`, and
// its contract states that device delta accumulation stays outside the
// runtime), and the desktop adapter polls the device state once per frame. A
// wire batch therefore carries one frame's events, folded in arrival order
// with last-event-wins per action kind, and submits exactly one aggregate;
// action kinds absent from the batch keep the zero value because the host
// sends the complete per-frame state every frame. Submitting an empty batch
// is the legitimate "no control input this frame" case.

// submitSemanticInput hands one aggregated frame intent to the online
// runtime. It is a variable so tests can record submissions, script runtime
// rejection, and inject panics through the seam; it is never redefined
// outside tests. The production body is the non-blocking copy semantics of
// `runtime.Runtime.SubmitInput`: it validates and copies the intent without
// touching the outbound queue, so a full outbound queue never blocks or fails
// this export — queue backpressure belongs to the step family's bounded send
// contract.
var submitSemanticInput = func(established *runtime.Runtime, input runtime.SemanticInput) error {
	return established.SubmitInput(input)
}

// onlineRuntime returns the runtime of an online session. A session that
// never began connecting or is still establishing has no published runtime
// and reports `StatusInvalidState`; a session that reached its terminal
// disconnect reports `StatusDisconnected`; only the online state hands out
// the runtime. The pointer stays valid after the lock because the session
// object owns it until teardown, matching the `sessionFor` resolution rule.
func (session *clientSession) onlineRuntime() (*runtime.Runtime, Status) {
	session.mu.Lock()
	defer session.mu.Unlock()
	switch session.state {
	case connectStateOnline:
		return session.runtime, StatusOK
	case connectStateTerminal:
		return nil, StatusDisconnected
	default:
		return nil, StatusInvalidState
	}
}

// disconnected reports whether the session reached its terminal state; the
// submission error path uses it to distinguish a disconnect racing the
// submission from a semantic rejection.
func (session *clientSession) disconnected() bool {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.state == connectStateTerminal
}

// foldInputEvent decodes one event record and folds it into the aggregate.
// The decode is pure: it reads only the record bytes and mutates only the
// owned aggregate, so a rejection leaves the batch's earlier events without
// effect (the caller discards the aggregate on any nonzero status).
func foldInputEvent(aggregate *runtime.SemanticInput, record []byte) Status {
	action := binary.LittleEndian.Uint32(record[0:4])
	value := binary.LittleEndian.Uint32(record[4:8])
	aux := binary.LittleEndian.Uint32(record[8:12])
	if binary.LittleEndian.Uint32(record[12:16]) != 0 {
		return StatusInputRejected
	}
	switch action {
	case InputActionMove:
		// The runtime validates the same {-1, 0, 1} domain on submission;
		// rejecting here keeps one malformed record from poisoning the
		// whole submission attempt after the batch was accepted.
		if !validMoveAxis(value) || !validMoveAxis(aux) {
			return StatusInputRejected
		}
		aggregate.MoveX = int8(int32(value))
		aggregate.MoveZ = int8(int32(aux))
	case InputActionLook:
		yaw := math.Float32frombits(value)
		pitch := math.Float32frombits(aux)
		// Finiteness is a wire-domain concern (NaN and infinities have no
		// meaningful device pose); the pitch magnitude domain stays owned
		// by `validateSemanticInput` and its rejection maps to
		// `StatusInputRejected` unchanged.
		if math.IsNaN(float64(yaw)) || math.IsInf(float64(yaw), 0) ||
			math.IsNaN(float64(pitch)) || math.IsInf(float64(pitch), 0) {
			return StatusInputRejected
		}
		aggregate.Yaw = yaw
		aggregate.Pitch = pitch
	case InputActionJump, InputActionMining, InputActionEating, InputActionSprinting, InputActionSneaking:
		if aux != 0 {
			return StatusInputRejected
		}
		if value > 1 {
			return StatusInputRejected
		}
		state := value == 1
		switch action {
		case InputActionJump:
			aggregate.Jump = state
		case InputActionMining:
			aggregate.Mining = state
		case InputActionEating:
			aggregate.Eating = state
		case InputActionSprinting:
			aggregate.Sprinting = state
		case InputActionSneaking:
			aggregate.Sneaking = state
		}
	default:
		return StatusInputRejected
	}
	return StatusOK
}

// validMoveAxis reports whether one wire word is a legal signed movement
// axis: exactly -1, 0, or 1 in two's complement.
func validMoveAxis(word uint32) bool {
	axis := int32(word)
	return axis >= -1 && axis <= 1
}

// decodeInputBatch validates and decodes one whole batch from owned memory.
// The batch must be exactly the frozen header (MCN1 magic, the input family
// layout version, an event count within `MaxInputEvents`, zero reserved)
// followed by exactly that many fixed records; a magic or layout mismatch
// reports `StatusABIMismatch` (record identity), every other defect —
// nonzero reserved, over-limit count, a body longer or shorter than the
// declared count, or any invalid record — reports `StatusInputRejected`. A
// nonzero status leaves the caller's state untouched because the aggregate is
// returned by value and nothing was submitted yet.
func decodeInputBatch(batch []byte) (runtime.SemanticInput, Status) {
	if len(batch) < int(InputHeaderBytes) {
		return runtime.SemanticInput{}, StatusInvalidArgument
	}
	if binary.LittleEndian.Uint32(batch[0:4]) != uint32(MagicInput) {
		return runtime.SemanticInput{}, StatusABIMismatch
	}
	if binary.LittleEndian.Uint32(batch[4:8]) != InputVersion {
		return runtime.SemanticInput{}, StatusABIMismatch
	}
	if binary.LittleEndian.Uint32(batch[12:16]) != 0 {
		return runtime.SemanticInput{}, StatusInputRejected
	}
	eventCount := binary.LittleEndian.Uint32(batch[8:12])
	if eventCount > MaxInputEvents {
		return runtime.SemanticInput{}, StatusInputRejected
	}
	declaredBytes := int(InputHeaderBytes) + int(eventCount)*int(InputEventBytes)
	if len(batch) != declaredBytes {
		return runtime.SemanticInput{}, StatusInputRejected
	}
	aggregate := runtime.SemanticInput{}
	for index := uint32(0); index < eventCount; index++ {
		offset := int(InputHeaderBytes) + int(index)*int(InputEventBytes)
		if status := foldInputEvent(&aggregate, batch[offset:offset+int(InputEventBytes)]); status != StatusOK {
			return runtime.SemanticInput{}, status
		}
	}
	return aggregate, StatusOK
}

// coreSubmitInput validates and submits one per-frame input batch. It is the
// testable core behind the exported submit input symbol; the raw pointer
// arguments mirror the C surface.
//
// Validation order, each phase returning before the next runs:
//  1. Handle lifecycle (table): an unknown value reports
//     `StatusInvalidHandle` and a destroyed session `StatusInvalidState`,
//     both before any argument is read.
//  2. Session state: only an online session with a published runtime may
//     submit; an idle or connecting session reports `StatusInvalidState` and
//     a terminal session `StatusDisconnected`.
//  3. Pointer and length shape: a length below the header size or above
//     `InputBatchMaxBytes`, a null buffer, or a buffer not aligned to
//     `ABIAlignment` reports `StatusInvalidArgument`. The length bounds are
//     judged before any buffer byte is read, so a lying oversized length
//     never overreads. There is no output pointer, so no input-output
//     overlap rule applies.
//  4. Content: the batch is copied into owned bounded memory once and then
//     decoded purely from the copy (`decodeInputBatch`), so a caller mutating
//     the buffer concurrently cannot produce a half-validated submission.
//  5. Submission: exactly one aggregate reaches the runtime. A runtime
//     rejection reports `StatusInputRejected`; if the session (or its
//     runtime's phase) reached the terminal disconnect while the submission
//     was in flight, the terminal status wins and reports
//     `StatusDisconnected`.
//
// The export never blocks on the network: `runtime.Runtime.SubmitInput`
// copies the validated intent under its session lock and the outbound queue
// is drained by the step family. The caller's buffer is never retained after
// the call returns.
func coreSubmitInput(handle uint64, buffer *byte, length uint32) Status {
	return withPanicGuard(func() Status {
		return submitInput(handle, buffer, length)
	})
}

// submitInput is the unguarded body of `coreSubmitInput`; see the
// `coreSubmitInput` contract for the validation order and status mapping.
func submitInput(handle uint64, buffer *byte, length uint32) Status {
	session, status := clientSessions.sessionFor(handle)
	if status != StatusOK {
		return status
	}
	established, status := session.onlineRuntime()
	if status != StatusOK {
		return status
	}
	if length < InputHeaderBytes || length > InputBatchMaxBytes {
		return StatusInvalidArgument
	}
	if buffer == nil {
		return StatusInvalidArgument
	}
	if uintptr(unsafe.Pointer(buffer))%uintptr(ABIAlignment) != 0 {
		return StatusInvalidArgument
	}
	batch := make([]byte, int(length))
	copy(batch, unsafe.Slice(buffer, int(length)))
	input, status := decodeInputBatch(batch)
	if status != StatusOK {
		return status
	}
	if err := submitSemanticInput(established, input); err != nil {
		if session.disconnected() || established.Phase() == runtime.ConnectionPhaseDisconnected {
			return StatusDisconnected
		}
		return StatusInputRejected
	}
	return StatusOK
}
