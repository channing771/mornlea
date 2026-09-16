//go:build cgo

package main

/*
#cgo CFLAGS: -I${SRCDIR}/include
#include "mornlea_client_core.h"
*/
import "C"

import "unsafe"

// The exported C surface of the client core. Each wrapper only converts raw
// C arguments to the Go views its core function expects; every validation
// decision, panic conversion, and output write lives in the testable core
// functions (`coreCreate`, `coreDestroy`, `coreConnectBegin`, `coreConnectPoll`,
// `coreDisconnect`, `coreSubmitInput`, `coreStep`, `coreStatusIdentity`, and
// `coreAbiVersion`). The wrappers never retain a caller pointer after the
// call returns: the handle table owns all producer state.
//
// Build note: this package is built with -buildmode=c-shared by the
// shared-library build script; the exported symbols below are that library's
// sole entry points. The declarations are intentionally not repeated in
// include/mornlea_client_core.h yet; the header gains export declarations as
// a reviewed compatible addition when the shared-library build lands.

//export mornlea_client_core_create
func mornlea_client_core_create(abiMajor C.uint32_t, abiMinor C.uint32_t, requestedFamilies *C.uint64_t, familyCount C.uint32_t, outHandle *C.uint64_t) C.uint32_t {
	return C.uint32_t(coreCreate(
		uint32(abiMajor),
		uint32(abiMinor),
		(*uint64)(unsafe.Pointer(requestedFamilies)),
		uint32(familyCount),
		(*uint64)(unsafe.Pointer(outHandle)),
	))
}

//export mornlea_client_core_destroy
func mornlea_client_core_destroy(handle C.uint64_t) C.uint32_t {
	return C.uint32_t(coreDestroy(uint64(handle)))
}

//export mornlea_client_core_connect_begin
func mornlea_client_core_connect_begin(handle C.uint64_t, address *C.uint8_t, addressLen C.uint32_t) C.uint32_t {
	return C.uint32_t(coreConnectBegin(
		uint64(handle),
		(*byte)(unsafe.Pointer(address)),
		uint32(addressLen),
	))
}

//export mornlea_client_core_connect_poll
func mornlea_client_core_connect_poll(handle C.uint64_t, outPhase *C.uint32_t) C.uint32_t {
	return C.uint32_t(coreConnectPoll(
		uint64(handle),
		(*uint32)(unsafe.Pointer(outPhase)),
	))
}

//export mornlea_client_core_disconnect
func mornlea_client_core_disconnect(handle C.uint64_t) C.uint32_t {
	return C.uint32_t(coreDisconnect(uint64(handle)))
}

//export mornlea_client_core_submit_input
func mornlea_client_core_submit_input(handle C.uint64_t, buffer *C.uint8_t, length C.uint32_t) C.uint32_t {
	return C.uint32_t(coreSubmitInput(
		uint64(handle),
		(*byte)(unsafe.Pointer(buffer)),
		uint32(length),
	))
}

//export mornlea_client_core_step
func mornlea_client_core_step(handle C.uint64_t, request *C.uint8_t, length C.uint32_t) C.uint32_t {
	return C.uint32_t(coreStep(
		uint64(handle),
		(*byte)(unsafe.Pointer(request)),
		uint32(length),
	))
}

//export mornlea_client_core_status_identity
func mornlea_client_core_status_identity(handle C.uint64_t, out *C.uint8_t, capacity C.uint32_t, outRequiredBytes *C.uint32_t) C.uint32_t {
	return C.uint32_t(coreStatusIdentity(
		uint64(handle),
		(*byte)(unsafe.Pointer(out)),
		uint32(capacity),
		(*uint32)(unsafe.Pointer(outRequiredBytes)),
	))
}

//export mornlea_client_core_abi_version
func mornlea_client_core_abi_version() C.uint64_t {
	return C.uint64_t(coreAbiVersion())
}
