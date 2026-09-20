package main

import (
	"sync"
	"testing"
	"unsafe"
)

// The lifecycle tests pin the identity/lifecycle export surface: the
// fixed-capacity handle table, the create/destroy semantics, and the
// validation-order contract (ABI identity before requested families before
// handle and pointer arguments). Every test owns a fresh table through
// `resetSessionTable` because package tests run sequentially; replacing the
// `clientSessions` variable would race only if tests ran in parallel, which
// these tests deliberately avoid.

// resetSessionTable installs a fresh, empty handle table for one test.
func resetSessionTable(t *testing.T) {
	t.Helper()
	clientSessions = newSessionTable()
}

// pilotRequestWords builds the valid pilot negotiation request: one 64-bit
// word per family with the family identifier in the low 32 bits and the
// registered contract version in the high 32 bits.
func pilotRequestWords() []uint64 {
	registry := ClientCoreRegistry()
	view := make([]RegistryDescriptor, registry.Len())
	if copied := registry.Descriptors(view); copied != len(view) {
		panic("test helper could not enumerate the registry")
	}
	words := make([]uint64, 0, len(view))
	for _, descriptor := range view {
		words = append(words, uint64(descriptor.Version)<<32|uint64(descriptor.Family))
	}
	return words
}

// requestWordsPointer adapts a request-word slice to the raw pointer shape of
// `coreCreate`; an empty request is the null pointer a C caller would pass.
func requestWordsPointer(words []uint64) *uint64 {
	if len(words) == 0 {
		return nil
	}
	return &words[0]
}

// createSessionOK performs a fully valid create and fails the test on any
// unexpected status.
func createSessionOK(t *testing.T) uint64 {
	t.Helper()
	words := pilotRequestWords()
	var handle uint64
	if status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), &handle); status != StatusOK {
		t.Fatalf("valid create failed with status %d", status)
	}
	if handle == 0 {
		t.Fatal("valid create returned handle zero")
	}
	return handle
}

// identityStatusRequiredBytes is the exact two-phase size of the identity
// status record: the fixed header plus one descriptor per pilot family.
func identityStatusRequiredBytes() uint32 {
	return IdentityHeaderBytes + uint32(FamilyCount)*FamilyDescriptorBytes
}

// TestLifecycleCreateDestroyStatusHappyPath covers one full session walk:
// create succeeds with a nonzero handle, status identity succeeds on the live
// handle, destroy succeeds exactly once, and later use of the destroyed
// handle reports the destroyed lifecycle state instead of a live result.
func TestLifecycleCreateDestroyStatusHappyPath(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)

	required := identityStatusRequiredBytes()
	buffer := make([]byte, int(required))
	if status := coreStatusIdentity(handle, &buffer[0], uint32(len(buffer)), new(uint32)); status != StatusOK {
		t.Fatalf("status identity on a live handle failed with status %d", status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy of a live handle failed with status %d", status)
	}
	if live := clientSessions.live(); live != 0 {
		t.Fatalf("table reports %d live sessions after destroy, want 0", live)
	}
}

// TestLifecycleDestroyIsIdempotentForIssuedHandles pins the destroy ruling:
// destroying a handle this producer issued is idempotent (the tombstone state
// answers repeated destroy calls with `StatusOK`), status identity on the
// destroyed handle reports `StatusInvalidState`, and once the slot is reused
// the superseded handle value becomes `StatusInvalidHandle` again because its
// generation no longer matches.
func TestLifecycleDestroyIsIdempotentForIssuedHandles(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("first destroy failed with status %d", status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("second destroy of the same handle = %d, want StatusOK (idempotent)", status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("third destroy of the same handle = %d, want StatusOK (idempotent)", status)
	}
	required := identityStatusRequiredBytes()
	buffer := make([]byte, int(required))
	if status := coreStatusIdentity(handle, &buffer[0], uint32(len(buffer)), new(uint32)); status != StatusInvalidState {
		t.Fatalf("status identity on a destroyed handle = %d, want StatusInvalidState", status)
	}
	for index := range buffer {
		buffer[index] = 0xA5
	}
	if status := coreStatusIdentity(handle, &buffer[0], uint32(len(buffer)), new(uint32)); status != StatusInvalidState {
		t.Fatalf("repeated status on a destroyed handle = %d, want StatusInvalidState", status)
	}
	for index, value := range buffer {
		if value != 0xA5 {
			t.Fatalf("destroyed-handle status wrote buffer byte %d despite failure", index)
		}
	}

	// After the slot is reused the old handle value is superseded: destroy of
	// a generation that no longer matches the slot is an invalid handle, not
	// an idempotent success.
	if replacement := createSessionOK(t); replacement == handle {
		t.Fatal("slot reuse returned the superseded handle value")
	}
	if status := coreDestroy(handle); status != StatusInvalidHandle {
		t.Fatalf("destroy of a superseded handle = %d, want StatusInvalidHandle", status)
	}
}

// TestLifecycleDestroyRejectsUnknownHandleValues proves an arbitrary handle
// value that was never issued by this producer is `StatusInvalidHandle`: the
// zero handle, huge generation values, and generation mismatches against
// live, retired, and never-used slots. The slot field is as wide as the
// table, so every 64-bit value decodes to a slot index; validity comes only
// from the state and generation match.
func TestLifecycleDestroyRejectsUnknownHandleValues(t *testing.T) {
	resetSessionTable(t)
	live := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy failed with status %d", status)
	}
	unknown := []uint64{
		0,
		0xFFFFFFFFFFFFFFFF,
		0x0000000100000000,
		// Slot 3 was never used, so its first generation does not exist.
		makeSessionHandle(3, 1),
		// Slot 0 holds a live first-generation session; a later generation is
		// unknown until the slot is actually reused.
		makeSessionHandle(0, 2),
		// Slot 1 holds a retired first-generation tombstone; later and earlier
		// generations are both unknown.
		makeSessionHandle(1, 2),
		makeSessionHandle(1, 0),
	}
	for index, handle := range unknown {
		if status := coreDestroy(handle); status != StatusInvalidHandle {
			t.Fatalf("destroy of unknown handle %d (case %d) = %d, want StatusInvalidHandle",
				handle, index, status)
		}
	}
	if status := coreDestroy(live); status != StatusOK {
		t.Fatalf("destroy of the still-live handle %d = %d, want StatusOK", live, status)
	}
}

// TestLifecycleCreateAtCapacityFailsWithInvalidState pins the fixed-capacity
// ruling: the table never grows, create at capacity reports
// `StatusInvalidState` without writing the out handle, and freeing one slot
// makes create succeed again.
func TestLifecycleCreateAtCapacityFailsWithInvalidState(t *testing.T) {
	resetSessionTable(t)
	words := pilotRequestWords()
	handles := make([]uint64, 0, pilotSessionCapacity)
	for index := 0; index < pilotSessionCapacity; index++ {
		var handle uint64
		if status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), &handle); status != StatusOK {
			t.Fatalf("create %d of %d failed with status %d", index+1, pilotSessionCapacity, status)
		}
		handles = append(handles, handle)
	}
	var overflow uint64 = 0xDEADBEEF
	if status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), &overflow); status != StatusInvalidState {
		t.Fatalf("create at capacity = %d, want StatusInvalidState", status)
	}
	if overflow != 0xDEADBEEF {
		t.Fatalf("create at capacity wrote the out handle %d despite failure", overflow)
	}
	if status := coreDestroy(handles[0]); status != StatusOK {
		t.Fatalf("destroy freeing one slot failed with status %d", status)
	}
	var handle uint64
	if status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), &handle); status != StatusOK {
		t.Fatalf("create after freeing a slot = %d, want StatusOK", status)
	}
	if handle == handles[0] {
		t.Fatal("slot reuse returned the superseded handle value")
	}
}

// TestLifecycleCreateRejectsABIMismatchBeforeFamilyAndPointerChecks proves
// the first validation phase: a producer-wide ABI identity mismatch wins over
// any requested-family defect and any out-handle pointer defect, and leaves
// the table untouched.
func TestLifecycleCreateRejectsABIMismatchBeforeFamilyAndPointerChecks(t *testing.T) {
	resetSessionTable(t)
	unknownFamily := pilotRequestWords()
	unknownFamily[0] = unknownFamily[0]&0xFFFFFFFF00000000 | 99
	for _, identity := range []struct {
		major uint32
		minor uint32
	}{
		{major: ABIMajor + 1, minor: ABIMinor},
		{major: ABIMajor, minor: ABIMinor + 1},
		{major: ABIMajor - 1, minor: ABIMinor},
	} {
		if status := coreCreate(identity.major, identity.minor, requestWordsPointer(unknownFamily), uint32(len(unknownFamily)), nil); status != StatusABIMismatch {
			t.Fatalf("create with ABI %d.%d plus invalid families and null out handle = %d, want StatusABIMismatch",
				identity.major, identity.minor, status)
		}
	}
	// The exact producer minor must create successfully. An older requested
	// minor stays compatible once the minor rises above zero; with the
	// current minor of zero it is unrepresentable.
	if status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(pilotRequestWords()), uint32(len(pilotRequestWords())), new(uint64)); status != StatusOK {
		t.Fatalf("create at the exact producer ABI failed with status %d", status)
	}
	if live := clientSessions.live(); live != 1 {
		t.Fatalf("table holds %d live sessions after mismatch probes, want 1 (the one valid create)", live)
	}
}

// TestLifecycleCreateRejectsFamilyNegotiationBeforeOutHandlePointer proves
// the second validation phase: every requested-family defect reports
// `StatusInputRejected` even when the out-handle pointer is also null, so
// family negotiation precedes output pointer validation. A duplicate word is
// necessarily combined with a missing family at the maximum valid count, so
// the combined word covers the duplicate branch.
func TestLifecycleCreateRejectsFamilyNegotiationBeforeOutHandlePointer(t *testing.T) {
	resetSessionTable(t)
	full := func() []uint64 { return pilotRequestWords() }

	unknownFamily := full()
	unknownFamily[0] = (unknownFamily[0] & 0xFFFFFFFF00000000) | 99

	tooNewVersion := full()
	tooNewVersion[0] = ((tooNewVersion[0]>>32)+1)<<32 | (tooNewVersion[0] & 0xFFFFFFFF)

	duplicateIdentity := full()
	duplicateIdentity[1] = duplicateIdentity[0]

	cases := []struct {
		name    string
		words   []uint64
		details string
	}{
		{name: "unknown family", words: unknownFamily, details: "family 99 is not registered"},
		{name: "missing required", words: full()[:7], details: "dropping one family leaves seven of eight"},
		{name: "version too new", words: tooNewVersion, details: "requested version above the registered contract"},
		{name: "duplicate family", words: duplicateIdentity, details: "identity listed twice, connection missing"},
		{name: "empty request", words: nil, details: "zero families cannot cover the required eight"},
	}
	for _, testCase := range cases {
		count := uint32(len(testCase.words))
		if status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(testCase.words), count, nil); status != StatusInputRejected {
			t.Fatalf("create with %s (%s) and null out handle = %d, want StatusInputRejected",
				testCase.name, testCase.details, status)
		}
	}
	// The same family defects must not touch a real out handle either.
	var handle uint64 = 0xCAFEF00D
	if status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(unknownFamily), uint32(len(unknownFamily)), &handle); status != StatusInputRejected {
		t.Fatalf("create with unknown family and real out handle = %d, want StatusInputRejected", status)
	}
	if handle != 0xCAFEF00D {
		t.Fatalf("family rejection wrote the out handle %d despite failure", handle)
	}
	if live := clientSessions.live(); live != 0 {
		t.Fatalf("table holds %d live sessions after rejected requests, want 0", live)
	}
}

// TestLifecycleCreateValidatesRequestPointerArguments covers the argument
// shape of the request array itself: a positive count with a null pointer, a
// count above the family-count limit, and a misaligned request array are all
// `StatusInvalidArgument`.
func TestLifecycleCreateValidatesRequestPointerArguments(t *testing.T) {
	resetSessionTable(t)
	words := pilotRequestWords()
	if status := coreCreate(ABIMajor, ABIMinor, nil, uint32(len(words)), new(uint64)); status != StatusInvalidArgument {
		t.Fatalf("create with null request pointer and count %d = %d, want StatusInvalidArgument",
			len(words), status)
	}
	// The oversized count must be rejected before the array is read, so the
	// pointer below deliberately addresses only seven valid words.
	if status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words))+1, new(uint64)); status != StatusInvalidArgument {
		t.Fatalf("create with count %d = %d, want StatusInvalidArgument", len(words)+1, status)
	}
	storage := make([]uint64, len(words)+1)
	copy(storage[1:], words)
	misaligned := (*uint64)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), 4)))
	if status := coreCreate(ABIMajor, ABIMinor, misaligned, uint32(len(words)), new(uint64)); status != StatusInvalidArgument {
		t.Fatalf("create with misaligned request array = %d, want StatusInvalidArgument", status)
	}
	if live := clientSessions.live(); live != 0 {
		t.Fatalf("table holds %d live sessions after pointer rejections, want 0", live)
	}
}

// TestLifecycleCreateRequiresOutHandlePointer proves the out-handle pointer
// is validated after family negotiation: a fully valid request with a null
// out handle reports `StatusInvalidArgument` and consumes no table slot.
func TestLifecycleCreateRequiresOutHandlePointer(t *testing.T) {
	resetSessionTable(t)
	words := pilotRequestWords()
	if status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), nil); status != StatusInvalidArgument {
		t.Fatalf("valid create with null out handle = %d, want StatusInvalidArgument", status)
	}
	if live := clientSessions.live(); live != 0 {
		t.Fatalf("table holds %d live sessions after a null out handle, want 0", live)
	}
}

// TestLifecycleConcurrentCreateStatusDestroy drives the table from several
// goroutines under the race detector: interleaved create, status identity,
// and destroy calls, plus destruction of arbitrary handle values, must stay
// within the documented status vocabulary and leave the table empty. The
// workload uses no clock, randomness, or I/O so it stays deterministic.
func TestLifecycleConcurrentCreateStatusDestroy(t *testing.T) {
	resetSessionTable(t)
	const workers = 8
	const iterations = 200
	required := identityStatusRequiredBytes()
	var group sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				words := pilotRequestWords()
				var handle uint64
				status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), &handle)
				switch status {
				case StatusOK:
					buffer := make([]byte, int(required))
					if status := coreStatusIdentity(handle, &buffer[0], uint32(len(buffer)), new(uint32)); status != StatusOK {
						t.Errorf("worker %d: status identity on live handle %d = %d, want StatusOK",
							worker, handle, status)
					}
					if status := coreDestroy(handle); status != StatusOK {
						t.Errorf("worker %d: destroy of live handle %d = %d, want StatusOK",
							worker, handle, status)
					}
				case StatusInvalidState:
					// The fixed table may be momentarily full; retry on the
					// next iteration.
				default:
					t.Errorf("worker %d: create = %d, want StatusOK or StatusInvalidState", worker, status)
				}
				// Probe handles use a generation far above anything the
				// bounded workload can issue, so they stay never-issued
				// values while still exercising concurrent destroy against
				// the shared table.
				probe := uint64(1)<<40 | uint64(iteration%int(pilotSessionCapacity))
				if status := coreDestroy(probe); status != StatusInvalidHandle {
					t.Errorf("worker %d: destroy of probe handle %d = %d, want StatusInvalidHandle",
						worker, probe, status)
				}
				if status := coreDestroy(0); status != StatusInvalidHandle {
					t.Errorf("worker %d: destroy of zero handle = %d, want StatusInvalidHandle", worker, status)
				}
			}
		}(worker)
	}
	group.Wait()
	if live := clientSessions.live(); live != 0 {
		t.Fatalf("table holds %d live sessions after the race workload, want 0", live)
	}
}

// TestLifecycleHandleEncodingPinsSlotGeometry pins the handle value layout:
// the slot field exactly covers the fixed capacity, the first session on a
// fresh table is the first slot at generation one, and the encoding helpers
// round-trip slot and generation for representative values.
func TestLifecycleHandleEncodingPinsSlotGeometry(t *testing.T) {
	if want := uint64(pilotSessionCapacity); uint64(1)<<handleSlotBits != want {
		t.Fatalf("slot bits %d cover %d slots, want capacity %d",
			handleSlotBits, uint64(1)<<handleSlotBits, want)
	}
	if handleSlotMask != pilotSessionCapacity-1 {
		t.Fatalf("slot mask = %d, want %d", handleSlotMask, pilotSessionCapacity-1)
	}
	resetSessionTable(t)
	handle := createSessionOK(t)
	if want := uint64(1) << handleSlotBits; handle != want {
		t.Fatalf("first handle = %d, want first slot at generation one (%d)", handle, want)
	}
	for slot := 0; slot < pilotSessionCapacity; slot++ {
		for _, generation := range []uint64{1, 2, 1 << 40} {
			handle := makeSessionHandle(slot, generation)
			if got := sessionSlotIndex(handle); got != slot {
				t.Fatalf("handle %d decodes to slot %d, want %d", handle, got, slot)
			}
			if got := sessionHandleGeneration(handle); got != generation {
				t.Fatalf("handle %d decodes to generation %d, want %d", handle, got, generation)
			}
		}
	}
}
