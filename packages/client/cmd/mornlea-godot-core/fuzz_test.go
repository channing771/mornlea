package main

import (
	"encoding/binary"
	goruntime "runtime"
	"testing"
	"unsafe"

	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
)

// FuzzClientCoreABI drives every versioned client-core export with
// mutation-derived handle words, request bytes, lengths, and capacities, and
// asserts the cross-export invariants inside every iteration: the returned
// status stays inside the frozen vocabulary, a failed call writes no output
// byte (canary buffers; the two-phase size signal is the one documented
// exception), a written record starts with its family magic and layout
// version, and the teardown disconnects and destroys every session this
// iteration created so the table ends empty for the next seed.
//
// Honest boundary design: a Go fuzz harness cannot hand raw C pointers to the
// mutation engine, so the target fuzzes the byte level instead — request
// buffers, lengths within allocation bounds, capacities, handle words, and
// the ABI identity words — against the same `core*` functions the cgo
// wrappers call, building real Go buffers whose declared length always
// equals their allocation size so no mutated value can overread the harness.
// Misalignment is injected by shifting a buffer base one byte off the ABI
// alignment, reusing the C-caller-frame discipline of the unit tests. Online
// sessions come from the seam-based scripted transport (no sockets, no wall
// clock), and every iteration resets the session table and restores the
// transport provider, so iterations are deterministic-safe and independent
// whether they run as seed subtests or inside fuzz workers.

// Fuzz allocation and spin bounds. The length bound covers the maximal
// well-formed input batch (2064 bytes) plus its over-max shape rejection;
// the capacity bound covers every exact record size the identity, status,
// and frame families can require under the current contracts. Both bounds
// exist only to keep a mutated word from driving a multi-gigabyte harness
// allocation; the exact boundary values are pinned by the matrix tests.
const (
	fuzzLengthBound   = 2065
	fuzzCapacityBound = 4096
	fuzzSpinBound     = 1 << 20
)

// fuzzRequestBuffer builds one C-caller-shaped request buffer of exactly
// `size` bytes with content derived from the fuzz payload (repeated then
// truncated), carved from a uint64 storage array so the base honors
// `ABIAlignment`; the misaligned flag shifts the base one byte off exactly
// like the unit tests' defect buffers. A zero size is the null pointer a C
// caller would pass. The declared length always equals the real buffer size,
// so no mutated value can make the export overread the harness.
func fuzzRequestBuffer(payload []byte, size int, misaligned bool) *byte {
	if size == 0 {
		return nil
	}
	storage := make([]uint64, (size+15)/8+1)
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&storage[0])), size+8)
	if misaligned {
		buffer = buffer[1:]
	}
	for index := 0; index < size; index++ {
		if len(payload) > 0 {
			buffer[index] = payload[index%len(payload)]
		}
	}
	return &buffer[0]
}

// fuzzAssertPull checks the pull-family invariants inside the fuzz body:
// every non-OK status leaves the write buffer untouched, only the capacity
// signal may report a size (positive, matching a fresh query, and never
// written by any other failure), a zero-size success reports exactly the
// zero size word, and a successful call with content writes a record that
// starts with the family magic and layout version. The poison byte 0xA5 can
// never open a real record because every family magic starts with the ASCII
// 'M' byte. `query` performs one fresh zero-capacity size query against the
// same handle so the capacity signal's reported size stays consistent with
// the idempotent two-phase protocol.
func fuzzAssertPull(t *testing.T, status Status, buffer []byte, sizeOut *uint32, magic, version uint32, query func() (uint32, Status)) {
	t.Helper()
	switch status {
	case StatusOK:
		if len(buffer) == 0 || buffer[0] == 0xA5 {
			matrixAssertPoisoned(t, buffer, "zero-size pull")
			if *sizeOut != 0 {
				t.Fatalf("zero-size pull reported required size %d, want zero", *sizeOut)
			}
			return
		}
		if len(buffer) < 8 {
			t.Fatalf("pull wrote into a %d-byte buffer that cannot hold a header", len(buffer))
		}
		if got := binary.LittleEndian.Uint32(buffer[0:4]); got != magic {
			t.Fatalf("written record starts with magic %#x, want the family magic %#x", got, magic)
		}
		if got := binary.LittleEndian.Uint32(buffer[4:8]); got != version {
			t.Fatalf("written record carries layout %#x, want the family version %d", got, version)
		}
	case StatusInsufficientCapacity:
		matrixAssertPoisoned(t, buffer, "capacity-signal pull")
		if *sizeOut == 0 {
			t.Fatal("capacity signal reported required size zero")
		}
		querySize, queryStatus := query()
		if queryStatus != StatusInsufficientCapacity || querySize != *sizeOut {
			t.Fatalf("capacity signal reported required size %d, but a fresh query reports (%d, %d)",
				*sizeOut, querySize, queryStatus)
		}
	default:
		matrixAssertPoisoned(t, buffer, "failed pull")
		if *sizeOut != 0xDEADBEEF {
			t.Fatalf("status %d wrote the size word %d despite failure", status, *sizeOut)
		}
	}
}

// fuzzSeedWords encodes the valid pilot family-request words as raw bytes for
// the create-path seed corpus.
func fuzzSeedWords() []byte {
	words := pilotRequestWords()
	buffer := make([]byte, 8*len(words))
	for index, word := range words {
		binary.LittleEndian.PutUint64(buffer[index*8:], word)
	}
	return buffer
}

// fuzzSeedStepRequest encodes one well-formed step request record.
func fuzzSeedStepRequest() []byte {
	pointer, _ := stepRequestPointer(0, 1, 1)
	return unsafe.Slice(pointer, int(StepRequestBytes))
}

// fuzzSeedInputBatch encodes one well-formed single-event input batch.
func fuzzSeedInputBatch() []byte {
	pointer, length := inputBatchPointer(inputMoveEvent(0, 1))
	return unsafe.Slice(pointer, int(length))
}

// FuzzClientCoreABI is the fuzz gate for the whole versioned export surface;
// see the file comment for the honest boundary design and the per-iteration
// invariants. The seed corpus covers one valid request shape per export so
// the mutation engine starts from every deep path: a valid create request,
// live and fuzzed handle words, the pilot address, a valid input batch, a
// valid step request, and pull capacities at the exact family record sizes.
func FuzzClientCoreABI(f *testing.F) {
	addressSeed := []byte("127.0.0.1:25565")
	// Packed exactly like the `coreAbiVersion` word: major high, minor low,
	// so this seed performs a genuinely valid create.
	abiSeed := uint64(ABIMajor)<<32 | uint64(ABIMinor)
	f.Add(uint8(0x00), uint64(0), fuzzSeedWords(), uint32(7), uint32(0), abiSeed)
	f.Add(uint8(0x01), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0x81), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0x02), uint64(0), addressSeed, uint32(len(addressSeed)), uint32(0), uint64(0))
	f.Add(uint8(0x82), uint64(0), addressSeed, uint32(len(addressSeed)), uint32(0), uint64(0))
	f.Add(uint8(0x12), uint64(0), addressSeed, uint32(len(addressSeed)), uint32(0), uint64(0))
	f.Add(uint8(0x03), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0x83), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0x04), ^uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0x84), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0x05), uint64(0), fuzzSeedInputBatch(), uint32(InputHeaderBytes+InputEventBytes), uint32(0), uint64(0))
	f.Add(uint8(0x95), uint64(0), fuzzSeedInputBatch(), uint32(InputHeaderBytes+InputEventBytes), uint32(0), uint64(0))
	f.Add(uint8(0x06), uint64(0), fuzzSeedStepRequest(), uint32(StepRequestBytes), uint32(0), uint64(0))
	f.Add(uint8(0x96), uint64(0), fuzzSeedStepRequest(), uint32(StepRequestBytes), uint32(0), uint64(0))
	f.Add(uint8(0x07), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0x87), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0xC7), uint64(0), []byte(nil), uint32(0), uint32(128), uint64(0))
	f.Add(uint8(0x08), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0x28), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0xA8), uint64(0), []byte(nil), uint32(0), uint32(576), uint64(0))
	f.Add(uint8(0x09), uint64(0), []byte(nil), uint32(0), uint32(80), uint64(0))
	f.Add(uint8(0x89), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Add(uint8(0x0A), uint64(0), []byte(nil), uint32(0), uint32(192), uint64(0))
	f.Add(uint8(0x8A), uint64(0), []byte(nil), uint32(0), uint32(0), uint64(0))
	f.Fuzz(func(t *testing.T, selector uint8, handle uint64, payload []byte, length uint32, capacity uint32, abiWord uint64) {
		// Reset every piece of shared producer state through the same seams
		// and helpers the unit tests use, so no iteration observes another's
		// session table or transport script; the deferred restore keeps the
		// production transport provider for whatever runs next in the same
		// process.
		clientSessions = newSessionTable()
		script := newStepTransportScript()
		previousDeps := remoteSessionDependencies
		remoteSessionDependencies = func() clientruntime.RemoteDependencies { return script.dependencies() }
		defer func() { remoteSessionDependencies = previousDeps }()

		live := createSessionOK(t)
		liveRetired := false
		var created []uint64
		online := false
		bringOnline := func() {
			if online {
				return
			}
			address := "127.0.0.1:25565"
			pointer := connectAddressPointer(address)
			if status := coreConnectBegin(live, pointer, uint32(len(address))); status != StatusOK {
				t.Fatalf("bring-online connect begin = %d, want StatusOK", status)
			}
			for iteration := 0; iteration < fuzzSpinBound; iteration++ {
				phase := uint32(0xDEADBEEF)
				if status := coreConnectPoll(live, &phase); status == StatusOK && phase == ConnectPhaseLoading {
					online = true
					return
				}
				goruntime.Gosched()
			}
			t.Fatal("bring-online spin never reached the loading phase")
		}

		target := handle
		if selector&0x80 != 0 {
			target = live
		}
		misaligned := selector&0x10 != 0
		size := int(length % fuzzLengthBound)
		outCapacity := capacity % fuzzCapacityBound

		switch selector & 0x0f {
		case 0: // create
			count := int(length % (uint32(FamilyCount) + 1))
			words := make([]uint64, count)
			for index := range words {
				if len(payload) >= (index+1)*8 {
					words[index] = binary.LittleEndian.Uint64(payload[index*8 : (index+1)*8])
				}
			}
			out := ^uint64(0)
			// The ABI identity word packs major in the high half and minor
			// in the low half, the same packing `coreAbiVersion` returns, so
			// the seed corpus genuinely reaches the create-success path.
			status := coreCreate(uint32(abiWord>>32), uint32(abiWord), requestWordsPointer(words), uint32(count), &out)
			assertStatusVocabulary(t, status)
			if status != StatusOK {
				if out != ^uint64(0) {
					t.Fatalf("failed create wrote the out handle %#x despite failure", out)
				}
			} else {
				created = append(created, out)
			}
		case 1: // destroy
			status := coreDestroy(target)
			assertStatusVocabulary(t, status)
			if target == live && status == StatusOK {
				liveRetired = true
			}
		case 2: // connect begin
			pointer := fuzzRequestBuffer(payload, size, misaligned)
			assertStatusVocabulary(t, coreConnectBegin(target, pointer, uint32(size)))
		case 3, 11, 12, 13, 14, 15: // connect poll
			phase := uint32(0xDEADBEEF)
			status := coreConnectPoll(target, &phase)
			assertStatusVocabulary(t, status)
			if status != StatusOK && phase != 0xDEADBEEF {
				t.Fatalf("failed poll wrote the phase word %d despite failure", phase)
			}
		case 4: // disconnect
			assertStatusVocabulary(t, coreDisconnect(target))
		case 5: // submit input
			bringOnline()
			pointer := fuzzRequestBuffer(payload, size, misaligned)
			assertStatusVocabulary(t, coreSubmitInput(target, pointer, uint32(size)))
		case 6: // step
			bringOnline()
			pointer := fuzzRequestBuffer(payload, size, misaligned)
			assertStatusVocabulary(t, coreStep(target, pointer, uint32(size)))
		case 7: // world pull
			if selector&0x40 != 0 {
				// Retain one world batch through the same step-result seam
				// the world tests use, so the record-write and capacity
				// paths of this family become reachable; the seam restores
				// the production step when this iteration's test ends.
				bringOnline()
				installWorldResultSeam(t, worldSeamResult(1, worldSeamDropBatch(t, 11, 12), true))
				pointer, stepLength := stepRequestPointer(0, 0, 0)
				if status := coreStep(live, pointer, stepLength); status != StatusOK {
					t.Fatalf("retaining world step = %d, want StatusOK", status)
				}
			}
			buffer, sizeOut := matrixCallerFrame(outCapacity)
			*sizeOut = 0xDEADBEEF
			var pointer *byte
			if outCapacity > 0 {
				pointer = &buffer[0]
			}
			status := coreWorldPull(target, pointer, outCapacity, sizeOut)
			assertStatusVocabulary(t, status)
			query := func() (uint32, Status) {
				size := uint32(0xDEADBEEF)
				status := coreWorldPull(target, nil, 0, &size)
				return size, status
			}
			fuzzAssertPull(t, status, buffer, sizeOut, uint32(MagicWorld), WorldVersion, query)
		case 8: // frame pull
			if selector&0x20 != 0 {
				bringOnline()
				pointer, stepLength := stepRequestPointer(0, 0, 0)
				if status := coreStep(live, pointer, stepLength); status != StatusOK {
					t.Fatalf("retaining zero step = %d, want StatusOK", status)
				}
			}
			buffer, sizeOut := matrixCallerFrame(outCapacity)
			*sizeOut = 0xDEADBEEF
			var pointer *byte
			if outCapacity > 0 {
				pointer = &buffer[0]
			}
			status := coreFramePull(target, pointer, outCapacity, sizeOut)
			assertStatusVocabulary(t, status)
			query := func() (uint32, Status) {
				size := uint32(0xDEADBEEF)
				status := coreFramePull(target, nil, 0, &size)
				return size, status
			}
			fuzzAssertPull(t, status, buffer, sizeOut, uint32(MagicFrame), FrameVersion, query)
		case 9: // status pull
			buffer, sizeOut := matrixCallerFrame(outCapacity)
			*sizeOut = 0xDEADBEEF
			var pointer *byte
			if outCapacity > 0 {
				pointer = &buffer[0]
			}
			status := coreStatusPull(target, pointer, outCapacity, sizeOut)
			assertStatusVocabulary(t, status)
			query := func() (uint32, Status) {
				size := uint32(0xDEADBEEF)
				status := coreStatusPull(target, nil, 0, &size)
				return size, status
			}
			fuzzAssertPull(t, status, buffer, sizeOut, uint32(MagicStatus), StatusVersion, query)
		case 10: // status identity
			buffer, sizeOut := matrixCallerFrame(outCapacity)
			*sizeOut = 0xDEADBEEF
			var pointer *byte
			if outCapacity > 0 {
				pointer = &buffer[0]
			}
			status := coreStatusIdentity(target, pointer, outCapacity, sizeOut)
			assertStatusVocabulary(t, status)
			query := func() (uint32, Status) {
				size := uint32(0xDEADBEEF)
				status := coreStatusIdentity(target, nil, 0, &size)
				return size, status
			}
			fuzzAssertPull(t, status, buffer, sizeOut, uint32(MagicIdentity), IdentityVersion, query)
		}

		// Teardown: disconnect joins any connect goroutine, destroy retires
		// every issued handle, and the table must end empty so no iteration
		// leaks session state into the next. Destroy of an issued handle is
		// idempotent, so an export call that already destroyed `live` or a
		// created handle still answers `StatusOK`.
		if !liveRetired {
			if status := coreDisconnect(live); status != StatusOK {
				t.Errorf("teardown disconnect = %d, want StatusOK", status)
			}
			if status := coreDestroy(live); status != StatusOK {
				t.Errorf("teardown destroy = %d, want StatusOK", status)
			}
		}
		for _, createdHandle := range created {
			if status := coreDestroy(createdHandle); status != StatusOK {
				t.Errorf("teardown destroy of the created handle = %d, want StatusOK", status)
			}
		}
		if count := clientSessions.live(); count != 0 {
			t.Errorf("table holds %d live sessions after teardown, want 0", count)
		}
	})
}
