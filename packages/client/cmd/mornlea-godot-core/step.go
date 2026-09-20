//go:build cgo

package main

import (
	"encoding/binary"
	"math"
	"time"
	"unsafe"

	"github.com/channing771/mornlea/packages/client/runtime"
)

// This file owns the step family export surface: the frozen MCS1 request
// record (see include/mornlea_client_core.h) decoded from an owned copy into
// one bounded step drive against the online runtime — a mesh-scheduling pass
// (`AdvanceMeshes`) followed by exactly one `runtime.Runtime.Step` call, each
// half bounded by the caller's single mesh budget, so the per-call total is at
// most twice that budget and still bounded — plus the per-session retention of
// the resulting `runtime.StepResult` for the later world and frame pull
// families. The export performs only bounded work and never waits on the
// network or a GPU.

// stepRuntime executes one bounded step drive against the online runtime. It
// is a variable so tests can record the decoded arguments, script step
// failure, and inject panics through the seam; it is never redefined outside
// tests. The production body is the bounded semantics of
// `runtime.Runtime.AdvanceMeshes` plus `runtime.Runtime.Step`: neither reads
// a wall clock and neither waits on the network, because every blocking
// transport send belongs to the runtime's asynchronous outbound worker and
// the step path itself only performs non-blocking enqueue operations (the
// queue send selects with a `default` arm and reports a full queue as an
// error), so this export returns exactly when both halves return.
//
// Mesh scheduling composition: the legacy application drives
// `AdvanceMeshes` from its own frame loop, but the pilot core has no such
// host, and `Step` only drains already-scheduled results into the world
// batch. One pilot step therefore first advances mesh scheduling under the
// same mesh budget (try-style worker scheduling and result draining, never
// a worker wait) and then steps; without this the pilot's world family
// would never publish a batch. Each half stays inside the caller's single
// mesh budget, bounding the whole export at twice that budget in section
// operations.
var stepRuntime = func(established *runtime.Runtime, elapsed time.Duration, messageBudget, meshBudget int) (runtime.StepResult, error) {
	if err := established.AdvanceMeshes(meshBudget); err != nil {
		return runtime.StepResult{}, err
	}
	return established.Step(elapsed, messageBudget, meshBudget)
}

// Step-result retention ruling: `runtime.Runtime.Step` is the single producer
// of at most one frame snapshot and one world batch per step, and the runtime's
// frame-revision uniqueness rests on a single-driver step contract. The step
// export therefore serializes steps per session through `stepMu` and retains
// the latest completed `runtime.StepResult` on the session; the world and frame
// pull families read that retained result instead of re-driving the runtime,
// so exactly one producer per frame feeds every consumer. A step that
// completes before the previous result was pulled simply overwrites it — the
// per-frame semantic is "pulls read the latest completed step" — and retention
// survives teardown because the frame family must still pull the terminal
// frame after a disconnect.

// retainStepResult stores the latest completed step result under the session
// mutex; see the retention ruling above for the overwrite semantics. Each
// retention also starts a fresh world-pull generation, so the pull family
// marks consumption against the new result rather than inheriting the
// consumed marker of the overwritten one (see world.go), and accumulates the
// result's inbound message count into the status family's session total (see
// status.go).
func (session *clientSession) retainStepResult(result runtime.StepResult) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.stepResult = result
	session.hasStepResult = true
	session.stepGeneration++
	session.messagesProcessedTotal += uint64(result.MessagesProcessed)
}

// retainedStep returns the latest completed step result and whether any step
// ever completed. The value is a copy, so callers may hold it across later
// steps; the world and frame pull families consume it through this accessor.
func (session *clientSession) retainedStep() (runtime.StepResult, bool) {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.stepResult, session.hasStepResult
}

// coreStep validates one step request and drives one bounded runtime step
// pass — the `stepRuntime` seam's mesh-scheduling call followed by exactly
// one `runtime.Runtime.Step`, each half under the caller's single mesh
// budget (at most twice that budget per export call). It is the testable
// core behind the exported step symbol; the raw pointer arguments mirror
// the C surface.
//
// Validation order, each phase returning before the next runs:
//  1. Handle lifecycle (table): an unknown value reports
//     `StatusInvalidHandle` and a destroyed session `StatusInvalidState`,
//     both before any argument is read.
//  2. Session state: only an online session with a published runtime may step;
//     an idle or connecting session reports `StatusInvalidState` and a
//     terminal session `StatusDisconnected`.
//  3. Pointer and length shape: the request length must equal
//     `StepRequestBytes` exactly (the record has no variable payload), the
//     pointer must be non-null and aligned to `ABIAlignment`; violations are
//     `StatusInvalidArgument` and are judged before any buffer byte is read.
//  4. Content, decoded from one owned copy of the record so a caller mutating
//     the buffer concurrently cannot split validation: a wrong family magic
//     or layout word reports `StatusABIMismatch` (record identity precedes
//     every content check); an elapsed-nanoseconds word above the maximal
//     representable int64 nanosecond count cannot become a non-negative Go
//     duration and reports `StatusInputRejected` — that representability
//     bound is the only elapsed domain check, because the runtime accepts any
//     non-negative duration and the predictor's own fixed-step cap bounds the
//     per-step work of a huge-but-representable advance (runtime-owned
//     semantic wrap edges at extreme durations stay in the runtime's domain);
//     a message budget above
//     `MaxStepMessageBudget` or a mesh budget above `MaxStepMeshBudget`
//     reports `StatusInputRejected` (both limits mirror the runtime's
//     validation domain, equality pinned by abi_test).
//     Zero elapsed and zero budgets are valid: a zero-size step still drains
//     within its budgets and publishes a frame but advances no prediction.
//  5. Execution: one `runtime.Runtime.Step` call, serialized per session so
//     the single-driver step contract holds even under a misbehaving host.
//
// Outcome mapping: success retains the result on the session and reports
// `StatusOK`. A step error maps to `StatusDisconnected` when the session or
// its runtime reached the terminal disconnect while the step was in flight
// (the runtime's argument-validation echoes are unreachable because phase 4
// pre-validates the identical domains); any other step failure is
// producer-internal and reports `StatusInternal`. A successful step that
// publishes a terminal frame (a dead receiver) still reports `StatusOK` with
// the terminal frame retained, and moves the session to its terminal state so
// later calls report `StatusDisconnected`. The caller's buffer is never
// retained after the call returns.
func coreStep(handle uint64, request *byte, length uint32) Status {
	return withPanicGuard(func() Status {
		return step(handle, request, length)
	})
}

// step is the unguarded body of `coreStep`; see the `coreStep` contract for
// the validation order and status mapping.
func step(handle uint64, request *byte, length uint32) Status {
	session, status := clientSessions.sessionFor(handle)
	if status != StatusOK {
		return status
	}
	established, status := session.onlineRuntime()
	if status != StatusOK {
		return status
	}
	if length != StepRequestBytes {
		return StatusInvalidArgument
	}
	if request == nil {
		return StatusInvalidArgument
	}
	if uintptr(unsafe.Pointer(request))%uintptr(ABIAlignment) != 0 {
		return StatusInvalidArgument
	}
	record := make([]byte, int(StepRequestBytes))
	copy(record, unsafe.Slice(request, int(StepRequestBytes)))
	if binary.LittleEndian.Uint32(record[0:4]) != uint32(MagicStep) {
		return StatusABIMismatch
	}
	if binary.LittleEndian.Uint32(record[4:8]) != StepVersion {
		return StatusABIMismatch
	}
	elapsedNanos := binary.LittleEndian.Uint64(record[8:16])
	if elapsedNanos > uint64(math.MaxInt64) {
		return StatusInputRejected
	}
	messageBudget := binary.LittleEndian.Uint32(record[16:20])
	if messageBudget > MaxStepMessageBudget {
		return StatusInputRejected
	}
	meshBudget := binary.LittleEndian.Uint32(record[20:24])
	if meshBudget > MaxStepMeshBudget {
		return StatusInputRejected
	}

	session.stepMu.Lock()
	defer session.stepMu.Unlock()
	result, err := stepRuntime(established, time.Duration(elapsedNanos), int(messageBudget), int(meshBudget))
	if err != nil {
		if session.disconnected() || established.Phase() == runtime.ConnectionPhaseDisconnected {
			return StatusDisconnected
		}
		return StatusInternal
	}
	session.retainStepResult(result)
	if result.Frame.Phase == runtime.ConnectionPhaseDisconnected {
		// The runtime self-closed through its terminal-error path and this
		// step published exactly one terminal frame. Retain it first so the
		// frame pull never observes a terminal session without its terminal
		// frame, then claim the session's terminal transition.
		session.recordRuntimeTerminal(established)
	}
	return StatusOK
}
