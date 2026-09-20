//go:build cgo

package main

import (
	"encoding/binary"
	"errors"
	"strings"
	"unsafe"

	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file owns the status/metrics family export surface: the two-phase pull
// of the pilot status record set under the MCM1 header. Like the frame pull,
// a status pull is deliberately NON-consuming: it observes session state (the
// connection phase word, the recorded terminal cause, the step counters) and
// changes nothing, so repeated pulls return identical bytes until a step, a
// connect transition, or a teardown changes that state. The pilot record set
// is minimal and additive: later compatible families of metrics append record
// kinds, never repurpose them.

// StatusRecordBytes is the fixed wire size of every status record. Like the
// world and frame body layouts, the record layout is producer-side wire
// vocabulary defined here, not new header defines: the frozen include/
// mornlea_client_core.h owns the MCM1 header and the record limit, and the
// Rust consumer pins this layout when its status bridge lands. The registry
// descriptor for the status family deliberately keeps `RecordBytes` at zero in
// this generation because both language pin suites pin that table; switching
// the descriptor to this fixed size is a coordinated cross-language update
// owned by the consumer task.
//
// Wire layout of one record, in little-endian words:
//
//	offset 0   kind      u32   one `StatusRecord*` code below
//	offset 4   reserved  u32   must be zero
//	offset 8   value     u64   the record's single 64-bit payload
const StatusRecordBytes = 16

// Status record kind codes. Codes are frozen: new kinds append and none is
// repurposed. The pilot set carries one record per observable: the session
// phase, the terminal-cause classification, and the two step counters.
const (
	// StatusRecordPhase carries the session's connection phase word using the
	// connect family's `ConnectPhase*` vocabulary verbatim, so one word
	// domain serves both families.
	StatusRecordPhase uint32 = 1
	// StatusRecordTerminalCause carries the terminal-cause classification of
	// the session's recorded terminal error, or `TerminalCauseNone`.
	StatusRecordTerminalCause uint32 = 2
	// StatusRecordStepsCompleted carries the number of step results the
	// session retained, the step family's completed-step count.
	StatusRecordStepsCompleted uint32 = 3
	// StatusRecordMessagesProcessed carries the session-total inbound server
	// messages the step family processed across every retained step.
	StatusRecordMessagesProcessed uint32 = 4
)

// Terminal-cause classification words. The mapping from runtime error classes
// to this small stable enum is documented per code; the words are frozen and
// new classes append. A clean user-requested disconnect records no error, so
// its class is `TerminalCauseNone` — the phase record's disconnected word plus
// a none cause is the clean-close signal, which is why no separate clean-close
// code exists.
const (
	// TerminalCauseNone means no terminal cause is recorded: the session is
	// live, or it closed cleanly at the user's request.
	TerminalCauseNone uint32 = 0
	// TerminalCauseDial means establishment failed at the dial step: the
	// runtime wraps every dial failure as "runtime: dial remote ...".
	TerminalCauseDial uint32 = 1
	// TerminalCauseHandshake means the server explicitly rejected the v44
	// handshake, including a version-mismatch rejection, surfaced as a
	// `*network.RemoteError` in the handshake state.
	TerminalCauseHandshake uint32 = 2
	// TerminalCauseLogin means the server explicitly rejected the login,
	// surfaced as a `*network.RemoteError` in the login state.
	TerminalCauseLogin uint32 = 3
	// TerminalCauseProtocol means the client detected incompatible or
	// malformed protocol traffic, including a client-side handshake version
	// mismatch, surfaced as a "network: protocol violation ..." error.
	TerminalCauseProtocol uint32 = 4
	// TerminalCauseReceiver means the post-login receiver died; the class is
	// recorded by provenance at the runtime-terminal record site, because the
	// receiver's own error value carries no stable marker.
	TerminalCauseReceiver uint32 = 5
	// TerminalCauseInternal means a recorded cause no narrower class covers,
	// such as a receiver-construction failure.
	TerminalCauseInternal uint32 = 6
)

// classifyEstablishmentTerminal maps one establishment failure recorded by the
// connect goroutine to the stable terminal-cause word. Structural checks come
// first: a server rejection keeps its `*network.RemoteError` state through the
// runtime's wrapping, so unwrapping through the chain identifies handshake
// versus login rejections without string coupling. The dial and protocol
// classes match the producer's documented wrapper markers instead, because
// the read-only runtime and network packages wrap those failures without
// sentinels to match structurally; the dial marker is the whole error's
// prefix while the protocol marker sits nested under the login wrapper, so it
// is matched by containment. The pinning tests catch classifier-side drift;
// a coordinated wording change inside the producer packages themselves would
// silently downgrade those causes to `TerminalCauseInternal` (the safe
// direction). Everything else, including the receiver
// factory failure, classifies as `TerminalCauseInternal`. A nil error is
// `TerminalCauseNone`, the live and clean-close word.
func classifyEstablishmentTerminal(err error) uint32 {
	if err == nil {
		return TerminalCauseNone
	}
	var remote *network.RemoteError
	if errors.As(err, &remote) {
		if remote.State == protocol.StateHandshake {
			return TerminalCauseHandshake
		}
		if remote.State == protocol.StateLogin {
			return TerminalCauseLogin
		}
	}
	message := err.Error()
	switch {
	case strings.HasPrefix(message, "runtime: dial remote"):
		return TerminalCauseDial
	case strings.Contains(message, "network: protocol violation"):
		return TerminalCauseProtocol
	default:
		return TerminalCauseInternal
	}
}

// metricsWireBytes returns the exact wire size of one status record set: the
// frozen header plus one fixed record per entry.
func metricsWireBytes(records int) int {
	return int(StatusHeaderBytes) + records*int(StatusRecordBytes)
}

// statusSnapshot is one consistent-enough view of the status observables: each
// field is individually consistent under the session mutex (or the runtime's
// phase lock for the phase word), and the pull publishes them as one record
// set.
type statusSnapshot struct {
	phase             uint32
	terminalCause     uint32
	stepsCompleted    uint64
	messagesProcessed uint64
}

// statusEncode produces the wire record of one status snapshot. It is a
// variable so tests can inject a panic through the seam and prove the export
// boundary converts it to `StatusPanic` without writing output; it is never
// redefined outside tests. Like the frame encoder it is a pure function of
// its value, never of the family registry.
var statusEncode = encodeStatus

// encodeStatus builds the wire record: the MCM1 header (magic, layout, record
// count, zero reserved) followed by the four pilot records in the fixed order
// phase, terminal cause, steps completed, messages processed. Determinism
// rests on the snapshot value alone, so the same session state always encodes
// to the same bytes.
func encodeStatus(snapshot statusSnapshot) []byte {
	record := make([]byte, metricsWireBytes(4))
	binary.LittleEndian.PutUint32(record[0:4], uint32(MagicStatus))
	binary.LittleEndian.PutUint32(record[4:8], StatusVersion)
	binary.LittleEndian.PutUint32(record[8:12], 4)
	// The reserved word at record[12:16] and every record reserved word stay
	// zero because the record is freshly allocated.
	statusPutRecord(record[16:32], StatusRecordPhase, uint64(snapshot.phase))
	statusPutRecord(record[32:48], StatusRecordTerminalCause, uint64(snapshot.terminalCause))
	statusPutRecord(record[48:64], StatusRecordStepsCompleted, snapshot.stepsCompleted)
	statusPutRecord(record[64:80], StatusRecordMessagesProcessed, snapshot.messagesProcessed)
	return record
}

// statusPutRecord writes one 16-byte kind-prefixed record with its single
// 64-bit value payload.
func statusPutRecord(record []byte, kind uint32, value uint64) {
	binary.LittleEndian.PutUint32(record[0:4], kind)
	binary.LittleEndian.PutUint64(record[8:16], value)
}

// statusSnapshot captures the pullable observables. The phase word comes from
// the shared `phaseWord` observation the connect poll uses, so both families
// see one connection lifecycle: while online the read delegates to the
// runtime's bounded phase observation, and a runtime that died from a
// receiver error transitions the session to terminal here, recording the
// receiver cause. The terminal outcome that the connect poll reports as
// `StatusDisconnected` (no output byte) is published by this family as the
// implied disconnected phase word in a record. The counters then read under
// the session mutex, so a terminal transition caused by the phase observation
// is already visible to them.
func (session *clientSession) statusSnapshot() statusSnapshot {
	phase, status := session.phaseWord()
	if status != StatusOK {
		phase = ConnectPhaseDisconnected
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return statusSnapshot{
		phase:             phase,
		terminalCause:     session.terminalClass,
		stepsCompleted:    session.stepGeneration,
		messagesProcessed: session.messagesProcessedTotal,
	}
}

// pullStatus serves the status record set through the two-phase capacity
// protocol. The family always has content — the phase record exists for every
// live session — so a query never reports size zero. Outcomes:
//   - Encoder failure (only the defensive nil today): `StatusInternal` with
//     no output.
//   - Capacity below the encoded size: `StatusInsufficientCapacity` and the
//     required size in the size word; no buffer byte is written.
//   - Sufficient capacity: the whole record is encoded into owned scratch and
//     copied once, exactly; nothing is consumed, so repeated pulls return
//     identical bytes while the session state is unchanged.
//
// Session ruling: the pull observes session and retained state, not a live
// connection requirement, so it never reports `StatusDisconnected`; a
// terminal session still pulls its record set (the phase and terminal-cause
// records exist precisely to publish that state), and only a destroyed handle
// is rejected with `StatusInvalidState`.
func (session *clientSession) pullStatus(out *byte, capacity uint32, requiredOut *uint32) Status {
	record := statusEncode(session.statusSnapshot())
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

// coreStatusPull serves the status record set through the two-phase capacity
// protocol. It is the testable core behind the exported status pull symbol;
// the raw pointer arguments mirror the C surface. A zero-capacity call is the
// required-size query; a positive capacity below the required size is the
// capacity signal; a sufficient capacity performs the one exact write. The
// pull is non-consuming (see pullStatus), so queries and consumes are freely
// repeatable.
//
// Overlap ruling: this export holds exactly two caller pointers, the write
// buffer and the size out-parameter, and rejects a size word pointing inside
// the write buffer's declared capacity span with `StatusInvalidArgument`,
// mirroring `coreWorldPull` and `coreFramePull`.
//
// Validation order, each phase returning before the next runs:
//  1. Handle lifecycle (table): an unknown value reports
//     `StatusInvalidHandle` and a destroyed session `StatusInvalidState`,
//     both before any argument is read.
//  2. Pointer and length shape: the size out-parameter is mandatory because
//     every call reports a size; a positive capacity needs a non-null,
//     `ABIAlignment`-aligned buffer that does not contain the size word.
//     Violations report `StatusInvalidArgument` before any session state is
//     read.
//  3. Content: the non-consuming serve above (observe, encode, capacity, one
//     exact write).
//
// A recovered panic reports `StatusPanic` with no output byte; the guarded
// path writes caller memory only as its final successful step, so a panic can
// only fire before any write. The caller's pointers are never retained after
// the call returns.
func coreStatusPull(handle uint64, out *byte, capacity uint32, requiredOut *uint32) Status {
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
		return session.pullStatus(out, capacity, requiredOut)
	})
}
