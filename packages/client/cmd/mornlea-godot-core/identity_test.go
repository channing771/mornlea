package main

import (
	"encoding/binary"
	"testing"
	"unsafe"
)

// The identity tests pin the two-phase status identity export: the
// required-size query, the no-write guarantee on insufficient capacity, the
// exact header-plus-descriptor wire record, and the validation order from
// handle lifecycle through pointer arguments to content. The panic tests pin
// the export boundary's panic conversion: an injected encoder panic becomes
// `StatusPanic` with no output byte written.

// poisonedBuffer returns a byte slice filled with a marker byte so any
// forbidden write is detectable.
func poisonedBuffer(size int) []byte {
	buffer := make([]byte, size)
	for index := range buffer {
		buffer[index] = 0xA5
	}
	return buffer
}

// TestIdentityStatusQueryReturnsRequiredBytes covers the two-phase query: a
// zero-capacity call (null buffer) reports the exact required size with
// `StatusInsufficientCapacity` and writes nothing else.
func TestIdentityStatusQueryReturnsRequiredBytes(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	required := uint32(0xDEADBEEF)
	if status := coreStatusIdentity(handle, nil, 0, &required); status != StatusInsufficientCapacity {
		t.Fatalf("zero-capacity query = %d, want StatusInsufficientCapacity", status)
	}
	want := IdentityHeaderBytes + uint32(FamilyCount)*FamilyDescriptorBytes
	if required != want {
		t.Fatalf("required bytes = %d, want %d (header %d plus %d descriptors of %d bytes)",
			required, want, IdentityHeaderBytes, FamilyCount, FamilyDescriptorBytes)
	}
	if want != 216 {
		t.Fatalf("identity record size = %d, want the pinned literal 216", want)
	}
}

// TestIdentityStatusShortBufferWritesNothing proves a buffer below the
// required size is never partially written: the poisoned content survives,
// the required size is reported, and no session state is consumed.
func TestIdentityStatusShortBufferWritesNothing(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	required := identityStatusRequiredBytes()
	for _, capacity := range []uint32{1, required - 1} {
		buffer := poisonedBuffer(int(capacity))
		requiredOut := uint32(0xDEADBEEF)
		status := coreStatusIdentity(handle, &buffer[0], capacity, &requiredOut)
		if status != StatusInsufficientCapacity {
			t.Fatalf("capacity %d status = %d, want StatusInsufficientCapacity", capacity, status)
		}
		if requiredOut != required {
			t.Fatalf("capacity %d reported required %d, want %d", capacity, requiredOut, required)
		}
		for index, value := range buffer {
			if value != 0xA5 {
				t.Fatalf("capacity %d: short buffer wrote byte %d despite failure", capacity, index)
			}
		}
	}
	// The failed queries left the session usable.
	full := poisonedBuffer(int(required))
	if status := coreStatusIdentity(handle, &full[0], uint32(len(full)), new(uint32)); status != StatusOK {
		t.Fatalf("status identity after failed queries = %d, want StatusOK", status)
	}
}

// TestIdentityStatusExactCapacityWritesHeaderAndDescriptors decodes the
// complete record written at exact capacity and pins every field: the header
// magic, layout version, producer ABI identity, family count, and zero
// reserved words, followed by the pinned descriptor table in family-ID
// order.
func TestIdentityStatusExactCapacityWritesHeaderAndDescriptors(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	required := identityStatusRequiredBytes()
	buffer := poisonedBuffer(int(required))
	if status := coreStatusIdentity(handle, &buffer[0], uint32(len(buffer)), new(uint32)); status != StatusOK {
		t.Fatalf("exact-capacity status = %d, want StatusOK", status)
	}
	if magic := binary.LittleEndian.Uint32(buffer[0:4]); magic != uint32(MagicIdentity) {
		t.Fatalf("header magic = %#x, want identity magic %#x", magic, uint32(MagicIdentity))
	}
	if layout := binary.LittleEndian.Uint32(buffer[4:8]); layout != IdentityVersion {
		t.Fatalf("header layout = %d, want identity family version %d", layout, IdentityVersion)
	}
	if major := binary.LittleEndian.Uint32(buffer[8:12]); major != ABIMajor {
		t.Fatalf("header abi major = %d, want %d", major, ABIMajor)
	}
	if minor := binary.LittleEndian.Uint32(buffer[12:16]); minor != ABIMinor {
		t.Fatalf("header abi minor = %d, want %d", minor, ABIMinor)
	}
	if count := binary.LittleEndian.Uint32(buffer[16:20]); count != uint32(FamilyCount) {
		t.Fatalf("header family count = %d, want %d", count, FamilyCount)
	}
	if reserved := binary.LittleEndian.Uint32(buffer[20:24]); reserved != 0 {
		t.Fatalf("header reserved = %#x, want zero", reserved)
	}
	want := pinnedPilotDescriptors()
	for index, descriptor := range want {
		offset := int(IdentityHeaderBytes) + index*int(FamilyDescriptorBytes)
		got := RegistryDescriptor{
			Family:      Family(binary.LittleEndian.Uint32(buffer[offset : offset+4])),
			Version:     binary.LittleEndian.Uint32(buffer[offset+4 : offset+8]),
			RecordLimit: binary.LittleEndian.Uint32(buffer[offset+8 : offset+12]),
			RecordBytes: binary.LittleEndian.Uint32(buffer[offset+12 : offset+16]),
		}
		if got != descriptor {
			t.Fatalf("descriptor %d = %+v, want pinned %+v", index, got, descriptor)
		}
		for reserved, value := range []uint32{
			binary.LittleEndian.Uint32(buffer[offset+16 : offset+20]),
			binary.LittleEndian.Uint32(buffer[offset+20 : offset+24]),
		} {
			if value != 0 {
				t.Fatalf("descriptor %d reserved word %d = %#x, want zero", index, reserved, value)
			}
		}
	}
}

// TestIdentityStatusOversizedBufferLeavesTailUntouched proves an oversized
// buffer receives exactly the required bytes and the surplus tail survives
// untouched.
func TestIdentityStatusOversizedBufferLeavesTailUntouched(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	required := identityStatusRequiredBytes()
	buffer := poisonedBuffer(int(required) + 13)
	if status := coreStatusIdentity(handle, &buffer[0], uint32(len(buffer)), new(uint32)); status != StatusOK {
		t.Fatalf("oversized-capacity status = %d, want StatusOK", status)
	}
	exact := poisonedBuffer(int(required))
	if status := coreStatusIdentity(handle, &exact[0], uint32(len(exact)), new(uint32)); status != StatusOK {
		t.Fatalf("exact-capacity status = %d, want StatusOK", status)
	}
	for index := 0; index < int(required); index++ {
		if buffer[index] != exact[index] {
			t.Fatalf("oversized write diverges from the exact record at byte %d", index)
		}
	}
	for index := int(required); index < len(buffer); index++ {
		if buffer[index] != 0xA5 {
			t.Fatalf("oversized write touched tail byte %d beyond the required size", index)
		}
	}
}

// TestIdentityStatusValidatesHandleBeforePointerArguments proves the
// lifecycle phase precedes pointer validation: an unknown handle with null
// output arguments is `StatusInvalidHandle`, and a destroyed handle is
// `StatusInvalidState` even before its buffer is considered.
func TestIdentityStatusValidatesHandleBeforePointerArguments(t *testing.T) {
	resetSessionTable(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy failed with status %d", status)
	}
	if status := coreStatusIdentity(0, nil, 0, nil); status != StatusInvalidHandle {
		t.Fatalf("status on zero handle with null arguments = %d, want StatusInvalidHandle", status)
	}
	if status := coreStatusIdentity(makeSessionHandle(2, 1<<30), nil, 0, nil); status != StatusInvalidHandle {
		t.Fatalf("status on never-issued handle with null arguments = %d, want StatusInvalidHandle", status)
	}
	if status := coreStatusIdentity(destroyed, nil, 0, nil); status != StatusInvalidState {
		t.Fatalf("status on destroyed handle with null arguments = %d, want StatusInvalidState", status)
	}
}

// TestIdentityStatusValidatesBufferPointerArguments covers the pointer shape:
// the required-size out-parameter is mandatory, a positive capacity with a
// null buffer is rejected, and a misaligned buffer is rejected, all with
// `StatusInvalidArgument` and no writes.
func TestIdentityStatusValidatesBufferPointerArguments(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	required := identityStatusRequiredBytes()
	if status := coreStatusIdentity(handle, &poisonedBuffer(int(required))[0], required, nil); status != StatusInvalidArgument {
		t.Fatalf("status with null size out-parameter = %d, want StatusInvalidArgument", status)
	}
	if status := coreStatusIdentity(handle, nil, required, new(uint32)); status != StatusInvalidArgument {
		t.Fatalf("status with null buffer and capacity %d = %d, want StatusInvalidArgument",
			required, status)
	}
	misalignedStorage := poisonedBuffer(int(required) + 8)
	misaligned := (*byte)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&misalignedStorage[0]), 1)))
	requiredOut := uint32(0xDEADBEEF)
	if status := coreStatusIdentity(handle, misaligned, required, &requiredOut); status != StatusInvalidArgument {
		t.Fatalf("status with misaligned buffer = %d, want StatusInvalidArgument", status)
	}
	if requiredOut != 0xDEADBEEF {
		t.Fatalf("pointer rejection wrote the size out-parameter %d despite failure", requiredOut)
	}
	for index, value := range misalignedStorage {
		if value != 0xA5 {
			t.Fatalf("pointer rejection wrote buffer byte %d despite failure", index)
		}
	}
}

// TestIdentityAbiVersionAccessorMatchesHeader pins the trivial ABI version
// accessor used by symbol verification before any session exists: it packs
// the producer major and minor without pointer arguments.
func TestIdentityAbiVersionAccessorMatchesHeader(t *testing.T) {
	if got, want := coreAbiVersion(), uint64(ABIMajor)<<32|uint64(ABIMinor); got != want {
		t.Fatalf("abi version accessor = %#x, want %#x", got, want)
	}
	if got, want := coreAbiVersion(), uint64(0x0000000100000001); got != want {
		t.Fatalf("abi version accessor = %#x, want the pinned literal %#x", got, want)
	}
}

// TestPanicStatusIdentityReturnsPanicStatusWithoutOutput injects a panic
// through the `identityStatusEncode` seam and proves the export boundary
// converts it to `StatusPanic` while leaving both the record buffer and the
// size out-parameter untouched. Swapping the seam is safe only because every
// test in this package runs sequentially; a parallel test reading the seam
// while it is swapped would race.
func TestPanicStatusIdentityReturnsPanicStatusWithoutOutput(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	required := identityStatusRequiredBytes()
	buffer := poisonedBuffer(int(required))
	requiredOut := uint32(0xDEADBEEF)
	previous := identityStatusEncode
	identityStatusEncode = func(Registry) []byte { panic("injected identity encoder panic") }
	defer func() { identityStatusEncode = previous }()
	if status := coreStatusIdentity(handle, &buffer[0], uint32(len(buffer)), &requiredOut); status != StatusPanic {
		t.Fatalf("status identity during injected panic = %d, want StatusPanic", status)
	}
	if requiredOut != 0xDEADBEEF {
		t.Fatalf("panic wrote the size out-parameter %d", requiredOut)
	}
	for index, value := range buffer {
		if value != 0xA5 {
			t.Fatalf("panic wrote buffer byte %d despite the failure", index)
		}
	}
}

// TestPanicGuardConvertsInjectedPanicToPanicStatus pins the shared guard
// every export routes through: a recovered panic becomes `StatusPanic` and a
// normal return passes the underlying status through unchanged. The status
// identity path exercises the same guard end to end in
// `TestPanicStatusIdentityReturnsPanicStatusWithoutOutput`.
func TestPanicGuardConvertsInjectedPanicToPanicStatus(t *testing.T) {
	if status := withPanicGuard(func() Status { return StatusOK }); status != StatusOK {
		t.Fatalf("guard changed a passing status to %d", status)
	}
	if status := withPanicGuard(func() Status { return StatusInvalidHandle }); status != StatusInvalidHandle {
		t.Fatalf("guard changed a failing status to %d", status)
	}
	if status := withPanicGuard(func() Status { panic("injected guard panic") }); status != StatusPanic {
		t.Fatalf("guard converted the injected panic to %d, want StatusPanic", status)
	}
}
