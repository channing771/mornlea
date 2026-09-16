//go:build cgo

package main

import (
	"sync"
	"unsafe"
)

// This file owns the identity/lifecycle half of the client-core export
// surface: a fixed-capacity session handle table plus the create logic with
// the validation-order contract ABI identity -> requested feature families ->
// handle and pointer arguments -> content. Every function here is
// deterministic: no clock, no I/O, no network, and no unbounded allocation
// participates in the handle path.

// pilotSessionCapacity is the fixed capacity of the sole session handle
// table. The pilot data plane has exactly one consumer, the mornlea_godot
// bridge, holding one live session; the margin to eight covers a bridge
// restart or diagnostic session overlapping the old one and keeps the table
// a fixed-size allocation. The bound is producer-internal bookkeeping rather
// than wire vocabulary, so it deliberately lives here and not in
// include/mornlea_client_core.h; exhaustion reports `StatusInvalidState`
// instead of growing the table.
const pilotSessionCapacity = 8

// Handle value layout: the low `handleSlotBits` bits name a table slot and
// the remaining bits hold the slot's session generation. Generations start at
// one, so the zero value is never an issued handle and a reused slot never
// re-issues a previously destroyed handle value. The slot field exactly
// covers the table, so every 64-bit value decodes to a slot index; validity
// comes only from the slot state and generation match.
const (
	handleSlotBits = 3
	handleSlotMask = pilotSessionCapacity - 1
)

// sessionState is one slot's lifecycle phase.
type sessionState uint8

const (
	// sessionSlotFree was never used or its generation was fully superseded.
	sessionSlotFree sessionState = iota
	// sessionSlotLive holds an active session created by `sessionTable.create`.
	sessionSlotLive
	// sessionSlotRetired holds a destroyed session kept as a tombstone so a
	// repeated destroy of the same handle value stays idempotent.
	sessionSlotRetired
)

// sessionSlot is one fixed table entry.
type sessionSlot struct {
	state      sessionState
	generation uint64
}

// sessionTable is the bounded, race-safe owner of every live session. All
// state is the fixed slot array guarded by one mutex; operations are O(capacity)
// with no allocation, so concurrent exports stay bounded under `-race` and in
// production.
type sessionTable struct {
	mu        sync.Mutex
	slots     [pilotSessionCapacity]sessionSlot
	liveCount int
}

// newSessionTable returns an empty table with every slot free at generation
// zero; the first use of a slot moves it to generation one.
func newSessionTable() *sessionTable {
	return &sessionTable{}
}

// makeSessionHandle packs a slot index and generation into one handle value.
func makeSessionHandle(slot int, generation uint64) uint64 {
	return generation<<handleSlotBits | uint64(slot)
}

// sessionSlotIndex extracts the slot index a handle names.
func sessionSlotIndex(handle uint64) int {
	return int(handle & handleSlotMask)
}

// sessionHandleGeneration extracts the generation a handle claims.
func sessionHandleGeneration(handle uint64) uint64 {
	return handle >> handleSlotBits
}

// create claims the first free or retired slot and returns its new handle.
// The generation is bumped on every claim, so a value destroyed earlier never
// aliases the new session. A full table returns `StatusInvalidState` and
// consumes nothing.
func (table *sessionTable) create() (uint64, Status) {
	table.mu.Lock()
	defer table.mu.Unlock()
	for index := range table.slots {
		if table.slots[index].state == sessionSlotLive {
			continue
		}
		table.slots[index].generation++
		table.slots[index].state = sessionSlotLive
		table.liveCount++
		return makeSessionHandle(index, table.slots[index].generation), StatusOK
	}
	return 0, StatusInvalidState
}

// destroy releases a session. The destroy ruling: destroying a handle this
// producer issued is idempotent, so a live slot with a matching generation
// retires and a retired slot with a matching generation answers `StatusOK`
// again; any other value (never-issued slot, free slot, or stale generation
// after the slot was reused) was never a live identity of the current
// generation and reports `StatusInvalidHandle`.
func (table *sessionTable) destroy(handle uint64) Status {
	table.mu.Lock()
	defer table.mu.Unlock()
	slot := &table.slots[sessionSlotIndex(handle)]
	switch {
	case slot.state == sessionSlotLive && slot.generation == sessionHandleGeneration(handle):
		slot.state = sessionSlotRetired
		table.liveCount--
		return StatusOK
	case slot.state == sessionSlotRetired && slot.generation == sessionHandleGeneration(handle):
		return StatusOK
	default:
		return StatusInvalidHandle
	}
}

// requireLive reports whether a handle names a live session. A retired
// tombstone with a matching generation is a destroyed session used after
// destroy, reported as `StatusInvalidState` (correct handle, wrong lifecycle
// phase); anything else is `StatusInvalidHandle`.
func (table *sessionTable) requireLive(handle uint64) Status {
	table.mu.Lock()
	defer table.mu.Unlock()
	slot := &table.slots[sessionSlotIndex(handle)]
	switch {
	case slot.state == sessionSlotLive && slot.generation == sessionHandleGeneration(handle):
		return StatusOK
	case slot.state == sessionSlotRetired && slot.generation == sessionHandleGeneration(handle):
		return StatusInvalidState
	default:
		return StatusInvalidHandle
	}
}

// live reports the number of live sessions; tests use it to prove failed
// calls consume no table state.
func (table *sessionTable) live() int {
	table.mu.Lock()
	defer table.mu.Unlock()
	return table.liveCount
}

// clientSessions is the sole handle table behind the exported lifecycle
// surface. Exports never retain caller pointers; this table owns every piece
// of session state.
var clientSessions = newSessionTable()

// withPanicGuard runs one export's core logic and converts any recovered
// panic into `StatusPanic`, honoring the boundary rule that a panic never
// unwinds across the ABI. A converted panic writes no output byte: every
// guarded path writes caller memory only as its final successful step, so a
// panic can only fire before any write. The create and status paths exercise
// this guard end to end; the identity encoder seam lets tests inject a panic
// without breaking the real encoder.
func withPanicGuard(work func() Status) (status Status) {
	defer func() {
		if recovered := recover(); recovered != nil {
			status = StatusPanic
		}
	}()
	return work()
}

// coreCreate validates a session creation request and allocates one session
// handle. It is the testable core behind the exported create symbol; the raw
// pointer arguments mirror the C surface so every validation decision lives
// here rather than in the cgo wrapper.
//
// Request words: the request pointer addresses `requestCount` little-endian
// 64-bit words, each packing a feature-family identifier in the low 32 bits
// and the requested family contract version in the high 32 bits (the first
// two fields of a family descriptor record). Decoding reads the words with
// the host's native byte order, which is little-endian on every supported
// desktop target and therefore matches the wire rule.
//
// Validation order, each phase returning before the next runs:
//  1. ABI identity: the requested major must equal the producer major and
//     the requested minor must not be newer than the producer minor; older
//     minors stay compatible because the minor only rises additively. Any
//     mismatch is `StatusABIMismatch`.
//  2. Requested families: the request set must be a readable, bounded array
//     whose entries all negotiate through `Registry.Negotiate` and together
//     cover every required pilot family. A zero-count request is judged as
//     content (it cannot cover the required families) and reports
//     `StatusInputRejected`; a count above `FamilyCount`, a null or
//     misaligned array with a positive count, or a buffer whose content
//     fails negotiation (unknown family, duplicate family, version newer
//     than the registered contract, missing required family) leaves the
//     table untouched and reports `StatusInvalidArgument` for the argument
//     shape or `StatusInputRejected` for the content.
//  3. Output pointer: the out-handle argument must be non-null
//     (`StatusInvalidArgument`).
//  4. Capacity: a full table reports `StatusInvalidState` and writes nothing.
//
// Only full success writes the out handle.
func coreCreate(abiMajor, abiMinor uint32, requested *uint64, requestCount uint32, outHandle *uint64) Status {
	return withPanicGuard(func() Status {
		return createSession(abiMajor, abiMinor, requested, requestCount, outHandle)
	})
}

// createSession is the unguarded body of `coreCreate`; see the `coreCreate`
// contract for the validation order and status mapping.
func createSession(abiMajor, abiMinor uint32, requested *uint64, requestCount uint32, outHandle *uint64) Status {
	if abiMajor != ABIMajor || abiMinor > ABIMinor {
		return StatusABIMismatch
	}
	if requestCount == 0 {
		return StatusInputRejected
	}
	if requestCount > uint32(FamilyCount) {
		return StatusInvalidArgument
	}
	if requested == nil {
		return StatusInvalidArgument
	}
	if uintptr(unsafe.Pointer(requested))%uintptr(ABIAlignment) != 0 {
		return StatusInvalidArgument
	}
	registry := ClientCoreRegistry()
	words := unsafe.Slice(requested, int(requestCount))
	var seenMask uint32
	for _, word := range words {
		family := Family(uint32(word))
		version := uint32(word >> 32)
		// An accepted negotiation implies a registered family, and the sole
		// registry pins registered identifiers to the pilot range, so the
		// mask shift below stays inside a uint32.
		if !registry.Negotiate(family, ABIMajor, version).Accepted {
			return StatusInputRejected
		}
		bit := uint32(1) << uint32(family)
		if seenMask&bit != 0 {
			return StatusInputRejected
		}
		seenMask |= bit
	}
	if seenMask != requiredFamilyMask() {
		return StatusInputRejected
	}
	if outHandle == nil {
		return StatusInvalidArgument
	}
	handle, status := clientSessions.create()
	if status != StatusOK {
		return status
	}
	*outHandle = handle
	return StatusOK
}

// requiredFamilyMask returns the request-presence bitmask of every required
// pilot family, in the same bit encoding `createSession` builds from request
// words.
func requiredFamilyMask() uint32 {
	var mask uint32
	for _, family := range pilotFamilyIDs() {
		mask |= uint32(1) << uint32(family)
	}
	return mask
}

// coreDestroy releases a session handle idempotently; see the destroy ruling
// on `sessionTable.destroy` for the invalid-handle boundary.
func coreDestroy(handle uint64) Status {
	return withPanicGuard(func() Status {
		return clientSessions.destroy(handle)
	})
}
