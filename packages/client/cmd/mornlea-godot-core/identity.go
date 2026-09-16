//go:build cgo

package main

import (
	"encoding/binary"
	"unsafe"
)

// This file owns the identity-status output record and the trivial ABI
// version accessor. The status path follows the two-phase capacity protocol
// from the header orientation: encode completely into owned scratch space,
// then perform one exact write into the caller buffer, reporting the required
// byte count instead of ever writing a partial record.

// identityStatusEncode produces the identity status record for one registry.
// It is a variable so tests can inject a panic through the seam and prove the
// export boundary converts it to `StatusPanic` without writing output; it is
// never redefined outside tests.
var identityStatusEncode = encodeIdentityStatus

// encodeIdentityStatus builds the wire record: one identity header (magic,
// layout, ABI major and minor, family count, zero reserved) followed by the
// registry's descriptor table in ascending family-ID order, each descriptor
// carrying family, version, record limit, record bytes, and two zero reserved
// words. Offsets are the pinned header offsets (magic 0, layout 4, abi_major
// 8, abi_minor 12, family_count 16, reserved 20; descriptor fields at 0, 4,
// 8, 12 with reserved words at 16 and 20). A registry whose enumeration
// cannot fill an exact-capacity view violates a registry invariant and
// yields nil, which callers map to `StatusInternal` rather than publishing a
// partial record.
func encodeIdentityStatus(registry Registry) []byte {
	descriptors := make([]RegistryDescriptor, registry.Len())
	if copied := registry.Descriptors(descriptors); copied != len(descriptors) || copied > int(FamilyCount) {
		return nil
	}
	record := make([]byte, int(IdentityHeaderBytes)+len(descriptors)*int(FamilyDescriptorBytes))
	binary.LittleEndian.PutUint32(record[0:4], uint32(MagicIdentity))
	binary.LittleEndian.PutUint32(record[4:8], IdentityVersion)
	binary.LittleEndian.PutUint32(record[8:12], ABIMajor)
	binary.LittleEndian.PutUint32(record[12:16], ABIMinor)
	binary.LittleEndian.PutUint32(record[16:20], uint32(len(descriptors)))
	// The reserved word at record[20:24] and every descriptor reserved word
	// stay zero because the record is freshly allocated.
	offset := int(IdentityHeaderBytes)
	for _, descriptor := range descriptors {
		binary.LittleEndian.PutUint32(record[offset:offset+4], uint32(descriptor.Family))
		binary.LittleEndian.PutUint32(record[offset+4:offset+8], descriptor.Version)
		binary.LittleEndian.PutUint32(record[offset+8:offset+12], descriptor.RecordLimit)
		binary.LittleEndian.PutUint32(record[offset+12:offset+16], descriptor.RecordBytes)
		offset += int(FamilyDescriptorBytes)
	}
	return record
}

// coreStatusIdentity writes the producer identity record for one live
// session handle. It is the testable core behind the exported status
// identity symbol; the raw pointer arguments mirror the C surface.
//
// Validation order, each phase returning before the next runs:
//  1. Handle lifecycle: a live handle passes, a destroyed handle reports
//     `StatusInvalidState`, and any other value reports
//     `StatusInvalidHandle`.
//  2. Pointer arguments: the required-size out-parameter must be non-null
//     because every call may need to report the required size, and a
//     positive `capacity` needs a non-null, `ABIAlignment`-aligned `out`
//     buffer; violations are `StatusInvalidArgument`.
//  3. Content capacity: the record is encoded completely into owned scratch
//     first, then either the required byte count is reported with
//     `StatusInsufficientCapacity` and no buffer write, or the exact record
//     is written into `out`, leaving any surplus tail untouched.
func coreStatusIdentity(handle uint64, out *byte, capacity uint32, requiredOut *uint32) Status {
	return withPanicGuard(func() Status {
		return statusIdentity(handle, out, capacity, requiredOut)
	})
}

// statusIdentity is the unguarded body of `coreStatusIdentity`; see the
// `coreStatusIdentity` contract for the validation order and status mapping.
func statusIdentity(handle uint64, out *byte, capacity uint32, requiredOut *uint32) Status {
	if status := clientSessions.requireLive(handle); status != StatusOK {
		return status
	}
	if requiredOut == nil {
		return StatusInvalidArgument
	}
	if capacity > 0 {
		if out == nil {
			return StatusInvalidArgument
		}
		if uintptr(unsafe.Pointer(out))%uintptr(ABIAlignment) != 0 {
			return StatusInvalidArgument
		}
	}
	record := identityStatusEncode(ClientCoreRegistry())
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

// coreAbiVersion is the trivial identity accessor behind the exported ABI
// version symbol: it packs the producer major (high 32 bits) and minor (low
// 32 bits) so the bridge and the shared-library symbol verification can
// confirm the loaded producer identity before any session exists. It takes
// no pointer arguments, cannot fail, and performs only constant arithmetic.
func coreAbiVersion() uint64 {
	return uint64(ABIMajor)<<32 | uint64(ABIMinor)
}
