package main

import (
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/channing771/mornlea/packages/client/presentation"
	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
)

// The error-precedence matrix tests pin, for every versioned client-core
// export, the documented validation order as one table: each cell injects one
// or more defect classes (ABI identity, feature families, handle lifecycle,
// session state, pointer/length shape, record content, capacity) into a
// single call and asserts that the highest-precedence phase wins. Around the
// status itself every cell re-proves the failure-atomicity invariants: the
// status stays inside the frozen vocabulary, no forbidden output byte is
// written (canary buffers; the two-phase size signal is the one documented
// exception), and the session and table survive so a subsequent valid call
// succeeds. Cells marked as recorded behavior document known conflations and
// transient classifications rather than aspirational contracts: queue-full
// backpressure currently conflates into `StatusInternal`, and a
// receiver-driven self-close racing an in-flight submission or step can
// transiently classify one call as `StatusInputRejected` or `StatusInternal`
// instead of `StatusDisconnected` until the session observes the terminal
// phase.

// matrixCell is one cell of the error-precedence matrix: a call carrying one
// or more injected defect classes, the highest-precedence status the
// documented validation order must report, and the failure-atomicity probes
// around it.
type matrixCell struct {
	// name identifies the cell in subtest output.
	name string
	// defects lists the combined defect classes the call carries.
	defects string
	// note marks recorded behavior — a documented conflation or transient
	// classification — rather than an aspirational contract.
	note string
	// invoke performs the call and returns its status.
	invoke func(t *testing.T) Status
	// canary proves no forbidden output byte was written on the call.
	canary func(t *testing.T)
	// recovery proves the session and table survived: a subsequent valid
	// call still succeeds.
	recovery func(t *testing.T)
	// want is the highest-precedence status the cell must report.
	want Status
}

// runMatrixCells executes the cells and asserts the shared invariants: the
// reported status stays inside the frozen vocabulary, the highest-precedence
// status wins, no forbidden output is written, and the surviving state still
// serves a valid call.
func runMatrixCells(t *testing.T, cells []matrixCell) {
	t.Helper()
	for _, cell := range cells {
		t.Run(cell.name, func(t *testing.T) {
			status := cell.invoke(t)
			if uint32(status) >= uint32(StatusCount) {
				t.Fatalf("status %d leaves the frozen vocabulary 0..%d", uint32(status), uint32(StatusCount)-1)
			}
			if status != cell.want {
				t.Fatalf("call with defects [%s] = %d, want the highest-precedence status %d (note: %s)",
					cell.defects, status, cell.want, cell.note)
			}
			if cell.canary != nil {
				cell.canary(t)
			}
			if cell.recovery != nil {
				cell.recovery(t)
			}
		})
	}
}

// assertStatusVocabulary fails when a status leaves the frozen word range;
// the matrix harness and the fuzz target share it.
func assertStatusVocabulary(t *testing.T, status Status) {
	t.Helper()
	if uint32(status) >= uint32(StatusCount) {
		t.Fatalf("status %d leaves the frozen vocabulary 0..%d", uint32(status), uint32(StatusCount)-1)
	}
}

// matrixCallerFrame models one C caller's frame for a pull export with the
// same discipline as the world and frame test frames: a poisoned write buffer
// at the head of one caller-owned allocation and the size word at an aligned
// offset beyond every declared span, so the two pointers can never drift into
// accidental numeric overlap with unrelated Go allocations.
func matrixCallerFrame(capacity uint32) (buffer []byte, sizeOut *uint32) {
	storage := poisonedBuffer(int(capacity) + 40)
	buffer = storage[:int(capacity):int(capacity)]
	sizeOffset := (int(capacity)+7)&^7 + 32
	return buffer, (*uint32)(unsafe.Pointer(&storage[sizeOffset]))
}

// matrixMisalignedBuffer returns a poisoned `capacity`-byte view whose base is
// one byte off `ABIAlignment`, carved from one padded allocation so the
// misaligned pointer stays inside live memory exactly like a C caller's
// defect buffer.
func matrixMisalignedBuffer(capacity uint32) []byte {
	storage := poisonedBuffer(int(capacity) + 8)
	return storage[1 : int(capacity)+1 : int(capacity)+1]
}

// matrixAliasedFrame builds one caller frame whose size word deliberately
// points inside the write buffer's declared span, the pull exports' one
// aliasing defect. The offset stays four-byte aligned so the pointer itself
// is valid; only the containment is wrong.
func matrixAliasedFrame(capacity uint32) (buffer []byte, sizeOut *uint32) {
	storage := poisonedBuffer(int(capacity) + 40)
	buffer = storage[:int(capacity):int(capacity)]
	return buffer, (*uint32)(unsafe.Pointer(&storage[8]))
}

// matrixAssertPoisoned fails when any byte of a canary buffer changed.
func matrixAssertPoisoned(t *testing.T, buffer []byte, context string) {
	t.Helper()
	for index, value := range buffer {
		if value != 0xA5 {
			t.Fatalf("%s wrote buffer byte %d despite writing nothing", context, index)
		}
	}
}

// TestMatrixCreateErrorPrecedence pins the create validation order — ABI
// identity before requested families (zero count judged as content before
// array shape), family content before the out-handle pointer, capacity last —
// and that every rejection leaves the table and the out-handle word
// untouched.
func TestMatrixCreateErrorPrecedence(t *testing.T) {
	resetSessionTable(t)
	unknownFamily := func() []uint64 {
		words := pilotRequestWords()
		words[0] = words[0]&0xFFFFFFFF00000000 | 99
		return words
	}
	liveProbe := func(t *testing.T) {
		t.Helper()
		if live := clientSessions.live(); live != 0 {
			t.Fatalf("rejected create left %d live sessions, want 0", live)
		}
		if status := coreDestroy(createSessionOK(t)); status != StatusOK {
			t.Fatalf("valid create after rejections failed to destroy with status %d", status)
		}
	}
	runMatrixCells(t, []matrixCell{
		{
			name:    "newer major outranks families shape and out handle",
			defects: "abi major mismatch + unknown family + count over limit + null out handle",
			invoke: func(t *testing.T) Status {
				return coreCreate(ABIMajor+1, ABIMinor, requestWordsPointer(unknownFamily()), uint32(FamilyCount)+1, nil)
			},
			recovery: liveProbe,
			want:     StatusABIMismatch,
		},
		{
			name:    "older major outranks families shape and out handle",
			defects: "abi major mismatch + null request array + zero count + null out handle",
			invoke: func(t *testing.T) Status {
				return coreCreate(ABIMajor-1, ABIMinor, nil, 0, nil)
			},
			recovery: liveProbe,
			want:     StatusABIMismatch,
		},
		{
			name:    "newer minor outranks families shape and out handle",
			defects: "abi minor mismatch + unknown family + null out handle",
			invoke: func(t *testing.T) Status {
				return coreCreate(ABIMajor, ABIMinor+1, requestWordsPointer(unknownFamily()), uint32(FamilyCount), nil)
			},
			recovery: liveProbe,
			want:     StatusABIMismatch,
		},
		{
			name:    "empty request is judged as family content before array shape",
			defects: "zero count + null request array + null out handle",
			invoke: func(t *testing.T) Status {
				return coreCreate(ABIMajor, ABIMinor, nil, 0, nil)
			},
			note:     "the zero-count request cannot cover the required families, so family content outranks the null-array shape defect",
			recovery: liveProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "count over the family limit outranks the null array",
			defects: "count above family count + null request array + null out handle",
			invoke: func(t *testing.T) Status {
				return coreCreate(ABIMajor, ABIMinor, nil, uint32(FamilyCount)+1, nil)
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "null request array with a valid count",
			defects: "null request array + valid count + real out handle",
			invoke: func(t *testing.T) Status {
				return coreCreate(ABIMajor, ABIMinor, nil, uint32(FamilyCount), new(uint64))
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "misaligned request array",
			defects: "request array one word-misaligned + real out handle",
			invoke: func(t *testing.T) Status {
				words := pilotRequestWords()
				storage := make([]uint64, len(words)+1)
				copy(storage[1:], words)
				misaligned := (*uint64)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), 4)))
				return coreCreate(ABIMajor, ABIMinor, misaligned, uint32(len(words)), new(uint64))
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "unknown family content outranks the null out handle",
			defects: "unknown family + null out handle",
			invoke: func(t *testing.T) Status {
				return coreCreate(ABIMajor, ABIMinor, requestWordsPointer(unknownFamily()), uint32(FamilyCount), nil)
			},
			recovery: liveProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "missing required family outranks the null out handle",
			defects: "seven of eight required families + null out handle",
			invoke: func(t *testing.T) Status {
				words := pilotRequestWords()[:7]
				return coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), nil)
			},
			recovery: liveProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "duplicate family combined with a missing family",
			defects: "identity listed twice + connection missing + null out handle",
			invoke: func(t *testing.T) Status {
				words := pilotRequestWords()
				words[1] = words[0]
				return coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), nil)
			},
			recovery: liveProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "requested family version too new outranks the null out handle",
			defects: "family version above the registered contract + null out handle",
			invoke: func(t *testing.T) Status {
				words := pilotRequestWords()
				words[0] = ((words[0] >> 32) + 1) << 32
				return coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), nil)
			},
			recovery: liveProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "family rejection leaves the out-handle word untouched",
			defects: "unknown family + poisoned out handle",
			invoke: func(t *testing.T) Status {
				out := uint64(0xCAFEF00D)
				status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(unknownFamily()), uint32(FamilyCount), &out)
				if out != 0xCAFEF00D {
					t.Errorf("rejected create wrote the out handle %d despite failure", out)
				}
				return status
			},
			recovery: liveProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "valid families still require the out-handle pointer",
			defects: "null out handle",
			invoke: func(t *testing.T) Status {
				words := pilotRequestWords()
				return coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), nil)
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
	})
	// The capacity cell runs last because it fills the fixed table.
	var filled []uint64
	runMatrixCells(t, []matrixCell{
		{
			name:    "full table reports state and writes nothing",
			defects: "table at capacity + poisoned out handle",
			invoke: func(t *testing.T) Status {
				for index := 0; index < pilotSessionCapacity; index++ {
					filled = append(filled, createSessionOK(t))
				}
				out := uint64(0xDEADBEEF)
				words := pilotRequestWords()
				status := coreCreate(ABIMajor, ABIMinor, requestWordsPointer(words), uint32(len(words)), &out)
				if out != 0xDEADBEEF {
					t.Errorf("create at capacity wrote the out handle %d despite failure", out)
				}
				return status
			},
			recovery: func(t *testing.T) {
				for _, handle := range filled {
					if status := coreDestroy(handle); status != StatusOK {
						t.Fatalf("drain destroy of %d = %d, want StatusOK", handle, status)
					}
				}
				if live := clientSessions.live(); live != 0 {
					t.Fatalf("table holds %d live sessions after draining, want 0", live)
				}
			},
			want: StatusInvalidState,
		},
	})
}

// TestMatrixDestroyErrorPrecedence pins the destroy handle ruling: never-
// issued values are `StatusInvalidHandle` regardless of every other property,
// an issued handle destroys idempotently, and the surviving live session
// stays usable after any rejection.
func TestMatrixDestroyErrorPrecedence(t *testing.T) {
	resetSessionTable(t)
	live := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	liveProbe := func(t *testing.T) {
		t.Helper()
		required := identityStatusRequiredBytes()
		buffer := poisonedBuffer(int(required))
		if status := coreStatusIdentity(live, &buffer[0], uint32(len(buffer)), new(uint32)); status != StatusOK {
			t.Fatalf("identity on the surviving session = %d, want StatusOK", status)
		}
	}
	runMatrixCells(t, []matrixCell{
		{
			name:     "zero handle",
			defects:  "never-issued handle 0",
			invoke:   func(t *testing.T) Status { return coreDestroy(0) },
			recovery: liveProbe,
			want:     StatusInvalidHandle,
		},
		{
			name:     "maximal handle word",
			defects:  "never-issued handle ^uint64(0)",
			invoke:   func(t *testing.T) Status { return coreDestroy(^uint64(0)) },
			recovery: liveProbe,
			want:     StatusInvalidHandle,
		},
		{
			name:     "never-used slot at generation one",
			defects:  "free slot + claimed generation",
			invoke:   func(t *testing.T) Status { return coreDestroy(makeSessionHandle(3, 1)) },
			recovery: liveProbe,
			want:     StatusInvalidHandle,
		},
		{
			name:     "live slot at a future generation",
			defects:  "live slot + stale generation",
			invoke:   func(t *testing.T) Status { return coreDestroy(makeSessionHandle(0, 2)) },
			recovery: liveProbe,
			want:     StatusInvalidHandle,
		},
		{
			name:     "retired tombstone is idempotent",
			defects:  "destroyed handle + repeated destroy",
			note:     "recorded ruling: destroying a handle this producer issued is idempotent",
			invoke:   func(t *testing.T) Status { return coreDestroy(destroyed) },
			recovery: liveProbe,
			want:     StatusOK,
		},
		{
			name:    "live handle destroys",
			defects: "live handle",
			invoke:  func(t *testing.T) Status { return coreDestroy(live) },
			recovery: func(t *testing.T) {
				// The tombstone answers later calls with the destroyed
				// lifecycle state and repeated destroy stays idempotent.
				if status := coreStatusIdentity(live, nil, 0, new(uint32)); status != StatusInvalidState {
					t.Fatalf("identity on the destroyed session = %d, want StatusInvalidState", status)
				}
				if status := coreDestroy(live); status != StatusOK {
					t.Fatalf("repeated destroy = %d, want StatusOK (idempotent)", status)
				}
			},
			want: StatusOK,
		},
	})
	if status := coreStatusIdentity(live, nil, 0, new(uint32)); status != StatusInvalidState {
		t.Fatalf("identity on the destroyed session = %d, want StatusInvalidState", status)
	}
}

// TestMatrixStatusIdentityErrorPrecedence pins the identity pull order —
// handle lifecycle before pointer shape before content — with the two-phase
// capacity signal as the one documented output-writing failure.
func TestMatrixStatusIdentityErrorPrecedence(t *testing.T) {
	resetSessionTable(t)
	live := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	required := identityStatusRequiredBytes()
	validPull := func(t *testing.T) {
		t.Helper()
		buffer := poisonedBuffer(int(required))
		if status := coreStatusIdentity(live, &buffer[0], uint32(len(buffer)), new(uint32)); status != StatusOK {
			t.Fatalf("valid identity pull after rejections = %d, want StatusOK", status)
		}
	}
	runMatrixCells(t, []matrixCell{
		{
			name:     "unknown handle outranks the null size word",
			defects:  "never-issued handle + null buffer + null size word",
			invoke:   func(t *testing.T) Status { return coreStatusIdentity(0, nil, 0, nil) },
			recovery: validPull,
			want:     StatusInvalidHandle,
		},
		{
			name:     "destroyed handle outranks the null size word",
			defects:  "destroyed handle + null buffer + null size word",
			invoke:   func(t *testing.T) Status { return coreStatusIdentity(destroyed, nil, 0, nil) },
			recovery: validPull,
			want:     StatusInvalidState,
		},
		{
			name:    "live handle requires the size word",
			defects: "null size word + valid buffer",
			invoke: func(t *testing.T) Status {
				buffer := poisonedBuffer(int(required))
				status := coreStatusIdentity(live, &buffer[0], uint32(len(buffer)), nil)
				matrixAssertPoisoned(t, buffer, "null-size-word identity pull")
				return status
			},
			recovery: validPull,
			want:     StatusInvalidArgument,
		},
		{
			name:    "positive capacity requires the buffer",
			defects: "null buffer + valid size word",
			invoke: func(t *testing.T) Status {
				size := uint32(0xDEADBEEF)
				status := coreStatusIdentity(live, nil, required, &size)
				if size != 0xDEADBEEF {
					t.Errorf("shape rejection wrote the size word %d despite failure", size)
				}
				return status
			},
			recovery: validPull,
			want:     StatusInvalidArgument,
		},
		{
			name:    "misaligned buffer",
			defects: "buffer one byte off the ABI alignment",
			invoke: func(t *testing.T) Status {
				buffer := matrixMisalignedBuffer(required)
				return coreStatusIdentity(live, &buffer[0], required, new(uint32))
			},
			recovery: validPull,
			want:     StatusInvalidArgument,
		},
		{
			name:    "short capacity is the two-phase signal",
			defects: "capacity below the required size",
			invoke: func(t *testing.T) Status {
				buffer := poisonedBuffer(int(required) - 8)
				size := uint32(0xDEADBEEF)
				status := coreStatusIdentity(live, &buffer[0], uint32(len(buffer)), &size)
				matrixAssertPoisoned(t, buffer, "short-capacity identity pull")
				if size != required {
					t.Errorf("short-capacity pull reported required %d, want %d", size, required)
				}
				return status
			},
			note:     "the size word is the one documented output the failure path may write",
			recovery: validPull,
			want:     StatusInsufficientCapacity,
		},
		{
			name:    "exact capacity succeeds",
			defects: "none (positive control)",
			invoke: func(t *testing.T) Status {
				buffer := poisonedBuffer(int(required))
				return coreStatusIdentity(live, &buffer[0], uint32(len(buffer)), new(uint32))
			},
			recovery: validPull,
			want:     StatusOK,
		},
		{
			name:    "encoder rejection maps to internal without output",
			defects: "registry enumeration invariant break",
			invoke: func(t *testing.T) Status {
				previous := identityStatusEncode
				identityStatusEncode = func(Registry) []byte { return nil }
				defer func() { identityStatusEncode = previous }()
				buffer := poisonedBuffer(int(required))
				size := uint32(0xDEADBEEF)
				status := coreStatusIdentity(live, &buffer[0], uint32(len(buffer)), &size)
				matrixAssertPoisoned(t, buffer, "encoder-rejected identity pull")
				if size != 0xDEADBEEF {
					t.Errorf("encoder rejection wrote the size word %d despite failure", size)
				}
				return status
			},
			recovery: validPull,
			want:     StatusInternal,
		},
	})
	if status := coreDestroy(live); status != StatusOK {
		t.Fatalf("final destroy = %d, want StatusOK", status)
	}
}

// TestMatrixConnectBeginErrorPrecedence pins the connect begin order — handle
// lifecycle before pointer/length shape before address content before
// connection state — and that no rejection starts an establishment.
func TestMatrixConnectBeginErrorPrecedence(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())
	address := "127.0.0.1:25565"
	idleProbe := func(t *testing.T) {
		t.Helper()
		phase := uint32(0xDEADBEEF)
		if status := coreConnectPoll(idle, &phase); status != StatusOK || phase != ConnectPhaseNotReady {
			t.Fatalf("poll after rejected begins = (%d, %d), want StatusOK and not-ready", status, phase)
		}
	}
	oversized := strings.Repeat("a", int(MaxConnectionAddressBytes)+1)
	runMatrixCells(t, []matrixCell{
		{
			name:     "unknown handle outranks every argument defect",
			defects:  "never-issued handle + zero length + null address",
			invoke:   func(t *testing.T) Status { return coreConnectBegin(0, nil, 0) },
			recovery: idleProbe,
			want:     StatusInvalidHandle,
		},
		{
			name:     "destroyed handle outranks every argument defect",
			defects:  "destroyed handle + zero length + null address",
			invoke:   func(t *testing.T) Status { return coreConnectBegin(destroyed, nil, 16) },
			recovery: idleProbe,
			want:     StatusInvalidState,
		},
		{
			name:    "zero length is a shape defect",
			defects: "zero length + valid address pointer",
			invoke: func(t *testing.T) Status {
				return coreConnectBegin(idle, connectAddressPointer(address), 0)
			},
			recovery: idleProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "length over the address limit is a shape defect",
			defects: "length above the connection address bound",
			invoke: func(t *testing.T) Status {
				pointer := connectAddressPointer(oversized)
				return coreConnectBegin(idle, pointer, uint32(len(oversized)))
			},
			recovery: idleProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:     "null address with a positive length is a shape defect",
			defects:  "null address + positive length",
			invoke:   func(t *testing.T) Status { return coreConnectBegin(idle, nil, uint32(len(address))) },
			recovery: idleProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "misaligned address is a shape defect",
			defects: "address one byte off the ABI alignment",
			invoke: func(t *testing.T) Status {
				storage := make([]byte, len(address)+8)
				copy(storage[1:], address)
				misaligned := (*byte)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), 1)))
				return coreConnectBegin(idle, misaligned, uint32(len(address)))
			},
			recovery: idleProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "invalid UTF-8 content is judged after shape",
			defects: "non-UTF-8 address bytes",
			invoke: func(t *testing.T) Status {
				return coreConnectBegin(idle, connectAddressPointer("\xff\xfe"), 2)
			},
			recovery: idleProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "ASCII control byte content is judged after shape",
			defects: "NUL and control bytes inside the address",
			invoke: func(t *testing.T) Status {
				return coreConnectBegin(idle, connectAddressPointer("127.0.0.1\n:25565"), uint32(len("127.0.0.1\n:25565")))
			},
			recovery: idleProbe,
			want:     StatusInputRejected,
		},
	})
	// The begun-state cells mutate the session, so they run last.
	runMatrixCells(t, []matrixCell{
		{
			name:    "begun connection state outranks a fully valid request",
			defects: "online session + valid address",
			note:    "connection state is the lowest-precedence phase for begin; a retry creates a new session",
			invoke: func(t *testing.T) Status {
				beginConnecting(t, idle, address)
				<-script.dialEntered
				close(script.dialRelease)
				connectPollUntilPhase(t, idle, ConnectPhaseLoading)
				return coreConnectBegin(idle, connectAddressPointer(address), uint32(len(address)))
			},
			recovery: func(t *testing.T) {
				phase := uint32(0xDEADBEEF)
				if status := coreConnectPoll(idle, &phase); status != StatusOK || phase != ConnectPhaseLoading {
					t.Fatalf("poll after the rejected repeat begin = (%d, %d), want StatusOK and loading", status, phase)
				}
			},
			want: StatusInvalidState,
		},
		{
			name:    "address content outranks the begun connection state",
			defects: "online session + non-UTF-8 address",
			note:    "content is judged before the connection-state phase, so a malformed retry address reports content, not state",
			invoke: func(t *testing.T) Status {
				return coreConnectBegin(idle, connectAddressPointer("\xff\xfe"), 2)
			},
			recovery: func(t *testing.T) {
				phase := uint32(0xDEADBEEF)
				if status := coreConnectPoll(idle, &phase); status != StatusOK || phase != ConnectPhaseLoading {
					t.Fatalf("poll after the rejected content retry = (%d, %d), want StatusOK and loading: the online connection is unchanged", status, phase)
				}
			},
			want: StatusInputRejected,
		},
	})
	if status := coreDestroy(idle); status != StatusOK {
		t.Fatalf("final destroy = %d, want StatusOK", status)
	}
}

// TestMatrixConnectPollErrorPrecedence pins the poll order — handle lifecycle
// before the out-pointer before the phase observation — and that no failure
// path writes the phase word, including the terminal disconnect status.
func TestMatrixConnectPollErrorPrecedence(t *testing.T) {
	resetSessionTable(t)
	live := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	runMatrixCells(t, []matrixCell{
		{
			name:    "unknown handle outranks the null out-pointer",
			defects: "never-issued handle + null phase word",
			invoke:  func(t *testing.T) Status { return coreConnectPoll(0, nil) },
			want:    StatusInvalidHandle,
		},
		{
			name:    "destroyed handle outranks the null out-pointer",
			defects: "destroyed handle + null phase word",
			invoke:  func(t *testing.T) Status { return coreConnectPoll(destroyed, nil) },
			want:    StatusInvalidState,
		},
		{
			name:    "live handle requires the out-pointer",
			defects: "null phase word",
			invoke:  func(t *testing.T) Status { return coreConnectPoll(live, nil) },
			want:    StatusInvalidArgument,
		},
	})
	// The terminal cell needs a begun connection — an idle session's
	// disconnect changes nothing, by the idempotent-idle ruling — so it drives
	// one establishment first, runs last, and proves the status carries the
	// disconnect signal without writing the phase word.
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())
	runMatrixCells(t, []matrixCell{
		{
			name:    "terminal session reports disconnect without writing",
			defects: "terminal session + poisoned phase word",
			invoke: func(t *testing.T) Status {
				address := "127.0.0.1:25565"
				beginConnecting(t, live, address)
				<-script.dialEntered
				close(script.dialRelease)
				connectPollUntilPhase(t, live, ConnectPhaseLoading)
				if status := coreDisconnect(live); status != StatusOK {
					t.Fatalf("setup disconnect = %d, want StatusOK", status)
				}
				phase := uint32(0xDEADBEEF)
				status := coreConnectPoll(live, &phase)
				if phase != 0xDEADBEEF {
					t.Errorf("terminal poll wrote the phase word %d despite the non-OK status", phase)
				}
				return status
			},
			want: StatusDisconnected,
		},
	})
	probe := createSessionOK(t)
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(probe, &phase); status != StatusOK || phase != ConnectPhaseNotReady {
		t.Fatalf("poll on a fresh session = (%d, %d), want StatusOK and not-ready", status, phase)
	}
	if status := coreDestroy(probe); status != StatusOK {
		t.Fatalf("final destroy = %d, want StatusOK", status)
	}
}

// TestMatrixDisconnectErrorPrecedence pins the disconnect handle ruling:
// never-issued values are `StatusInvalidHandle`, destroyed sessions
// `StatusInvalidState`, and every live session phase disconnects
// idempotently.
func TestMatrixDisconnectErrorPrecedence(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	runMatrixCells(t, []matrixCell{
		{
			name:    "never-issued handle",
			defects: "free slot + claimed generation",
			invoke:  func(t *testing.T) Status { return coreDisconnect(makeSessionHandle(5, 1<<40)) },
			want:    StatusInvalidHandle,
		},
		{
			name:    "destroyed session",
			defects: "destroyed handle",
			invoke:  func(t *testing.T) Status { return coreDisconnect(destroyed) },
			want:    StatusInvalidState,
		},
		{
			name:    "idle session disconnects idempotently",
			defects: "idle session + repeated disconnect",
			invoke: func(t *testing.T) Status {
				if status := coreDisconnect(idle); status != StatusOK {
					t.Fatalf("first disconnect = %d, want StatusOK", status)
				}
				return coreDisconnect(idle)
			},
			want: StatusOK,
		},
	})
	// The online cell installs its own transport and mutates its session, so
	// it runs last.
	runMatrixCells(t, []matrixCell{
		{
			name:    "online session disconnects",
			defects: "online session",
			invoke: func(t *testing.T) Status {
				handle := connectSessionOnlineInput(t)
				return coreDisconnect(handle)
			},
			recovery: func(t *testing.T) {
				// `connectSessionOnlineInput` resets the table, so a fresh
				// probe session proves the disconnect left the table usable.
				probe := createSessionOK(t)
				if status := coreDisconnect(probe); status != StatusOK {
					t.Fatalf("probe disconnect = %d, want StatusOK", status)
				}
			},
			want: StatusOK,
		},
	})
}

// TestMatrixSubmitInputErrorPrecedence pins the submission order — handle
// lifecycle before session state before buffer shape before batch identity
// and content — plus the whole-batch atomicity and the recorded outcome
// conflations.
func TestMatrixSubmitInputErrorPrecedence(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	log := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error { return nil })
	validBatch := func() (*byte, uint32) { return inputBatchPointer(inputMoveEvent(0, 1)) }
	idleProbe := func(t *testing.T) {
		t.Helper()
		pointer, length := validBatch()
		if status := coreSubmitInput(idle, pointer, length); status != StatusInvalidState {
			t.Fatalf("idle probe submit = %d, want StatusInvalidState (the session survived)", status)
		}
	}
	runMatrixCells(t, []matrixCell{
		{
			name:     "unknown handle outranks shape and content",
			defects:  "never-issued handle + null buffer + zero length",
			invoke:   func(t *testing.T) Status { return coreSubmitInput(0, nil, 0) },
			recovery: idleProbe,
			want:     StatusInvalidHandle,
		},
		{
			name:     "destroyed handle outranks shape and content",
			defects:  "destroyed handle + null buffer + zero length",
			invoke:   func(t *testing.T) Status { return coreSubmitInput(destroyed, nil, 0) },
			recovery: idleProbe,
			want:     StatusInvalidState,
		},
		{
			name:    "idle session state outranks shape and content",
			defects: "idle session + valid batch",
			invoke: func(t *testing.T) Status {
				pointer, length := validBatch()
				return coreSubmitInput(idle, pointer, length)
			},
			recovery: idleProbe,
			want:     StatusInvalidState,
		},
		{
			name:     "idle session state outranks the null buffer",
			defects:  "idle session + null buffer + zero length",
			invoke:   func(t *testing.T) Status { return coreSubmitInput(idle, nil, 0) },
			recovery: idleProbe,
			want:     StatusInvalidState,
		},
	})
	connecting := connectSessionOnlineInputConnecting(t)
	runMatrixCells(t, []matrixCell{
		{
			name:    "connecting session state outranks shape and content",
			defects: "connecting session + valid batch",
			invoke: func(t *testing.T) Status {
				pointer, length := validBatch()
				return coreSubmitInput(connecting, pointer, length)
			},
			want: StatusInvalidState,
		},
	})
	if status := coreDisconnect(connecting); status != StatusOK {
		t.Fatalf("disconnect of the connecting session = %d, want StatusOK", status)
	}
	if status := coreDestroy(connecting); status != StatusOK {
		t.Fatalf("destroy of the connecting session = %d, want StatusOK", status)
	}

	online := connectSessionOnlineInput(t)
	onlineProbe := func(t *testing.T) {
		t.Helper()
		before := log.count()
		pointer, length := validBatch()
		if status := coreSubmitInput(online, pointer, length); status != StatusOK {
			t.Fatalf("valid probe submit after rejections = %d, want StatusOK", status)
		}
		if count := log.count(); count != before+1 {
			t.Fatalf("valid probe submit reached the runtime %d times, want exactly once more", count-before)
		}
	}
	runMatrixCells(t, []matrixCell{
		{
			name:     "online zero length is a shape defect",
			defects:  "zero length + null buffer",
			invoke:   func(t *testing.T) Status { return coreSubmitInput(online, nil, 0) },
			recovery: onlineProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "online length below the header is a shape defect",
			defects: "length below the frozen input header",
			invoke: func(t *testing.T) Status {
				pointer, _ := validBatch()
				return coreSubmitInput(online, pointer, InputHeaderBytes-8)
			},
			recovery: onlineProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "online length over the batch bound is a shape defect",
			defects: "length above the maximal well-formed batch",
			invoke: func(t *testing.T) Status {
				storage := make([]uint64, int(InputBatchMaxBytes)/8+2)
				buffer := unsafe.Slice((*byte)(unsafe.Pointer(&storage[0])), int(InputBatchMaxBytes)+8)
				return coreSubmitInput(online, &buffer[0], uint32(len(buffer)))
			},
			recovery: onlineProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "online misaligned buffer is a shape defect",
			defects: "buffer one byte off the ABI alignment",
			invoke: func(t *testing.T) Status {
				pointer, length := validBatch()
				storage := make([]byte, int(length)+8)
				copy(storage[1:], unsafe.Slice(pointer, int(length)))
				misaligned := (*byte)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), 1)))
				return coreSubmitInput(online, misaligned, length)
			},
			recovery: onlineProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "shape outranks batch identity",
			defects: "length below the header + wrong family magic",
			note:    "the length bound is judged before any buffer byte is read, so the lying short length wins over the corrupt identity",
			invoke: func(t *testing.T) Status {
				pointer, _ := validBatch()
				patchInputWord(pointer, 0, uint32(MagicWorld))
				return coreSubmitInput(online, pointer, InputHeaderBytes-8)
			},
			recovery: onlineProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "batch identity outranks domain content",
			defects: "wrong family magic + over-limit event count",
			invoke: func(t *testing.T) Status {
				pointer, length := validBatch()
				patchInputWord(pointer, 0, uint32(MagicWorld))
				patchInputWord(pointer, 8, MaxInputEvents+1)
				return coreSubmitInput(online, pointer, length)
			},
			recovery: onlineProbe,
			want:     StatusABIMismatch,
		},
		{
			name:    "layout word outranks domain content",
			defects: "wrong layout version + unknown action code",
			invoke: func(t *testing.T) Status {
				pointer, length := validBatch()
				patchInputWord(pointer, 4, InputVersion+1)
				return coreSubmitInput(online, pointer, length)
			},
			recovery: onlineProbe,
			want:     StatusABIMismatch,
		},
		{
			name:    "nonzero reserved header word is rejected",
			defects: "nonzero header reserved word",
			invoke: func(t *testing.T) Status {
				pointer, length := validBatch()
				patchInputWord(pointer, 12, 1)
				return coreSubmitInput(online, pointer, length)
			},
			recovery: onlineProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "one invalid record rejects the whole batch",
			defects: "valid move event followed by an out-of-domain move axis",
			invoke: func(t *testing.T) Status {
				pointer, length := inputBatchPointer(inputMoveEvent(0, 1), inputMoveEvent(7, 0))
				before := log.count()
				status := coreSubmitInput(online, pointer, length)
				if count := log.count(); count != before {
					t.Errorf("rejected batch reached the runtime %d times, want 0", count-before)
				}
				return status
			},
			recovery: onlineProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "unknown action code rejects the whole batch",
			defects: "unknown action code in the middle event",
			invoke: func(t *testing.T) Status {
				pointer, length := inputBatchPointer(inputMoveEvent(0, 1), inputEventWords{action: 99}, inputStateEvent(InputActionJump, 1))
				return coreSubmitInput(online, pointer, length)
			},
			recovery: onlineProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "non-finite look pose rejects the whole batch",
			defects: "NaN yaw bits in the look event",
			invoke: func(t *testing.T) Status {
				pointer, length := inputBatchPointer(inputLookEvent(0, 0))
				patchInputWord(pointer, int(InputHeaderBytes)+4, 0x7FC00000)
				return coreSubmitInput(online, pointer, length)
			},
			recovery: onlineProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "runtime rejection maps to input rejected",
			defects: "scripted runtime rejection on a healthy session",
			note:    "recorded transient: a receiver-driven self-close racing an in-flight submission reports `StatusInputRejected` for one call until the session observes the terminal phase",
			invoke: func(t *testing.T) Status {
				calls := 0
				previous := submitSemanticInput
				submitSemanticInput = func(*clientruntime.Runtime, clientruntime.SemanticInput) error {
					calls++
					return errInputSeamRejected
				}
				defer func() { submitSemanticInput = previous }()
				pointer, length := validBatch()
				status := coreSubmitInput(online, pointer, length)
				if calls != 1 {
					t.Errorf("rejecting seam saw %d submissions, want 1", calls)
				}
				return status
			},
			recovery: onlineProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "submission panic converts to the panic status",
			defects: "panicking submission seam",
			invoke: func(t *testing.T) Status {
				calls := 0
				previous := submitSemanticInput
				submitSemanticInput = func(*clientruntime.Runtime, clientruntime.SemanticInput) error {
					calls++
					panic("matrix submit seam panic")
				}
				defer func() { submitSemanticInput = previous }()
				pointer, length := validBatch()
				status := coreSubmitInput(online, pointer, length)
				if calls != 1 {
					t.Errorf("panicking seam saw %d submissions, want 1", calls)
				}
				return status
			},
			recovery: onlineProbe,
			want:     StatusPanic,
		},
	})
	// The terminal cell disconnects the online session, so it runs last.
	runMatrixCells(t, []matrixCell{
		{
			name:    "terminal session outranks shape and content",
			defects: "terminal session + valid batch",
			invoke: func(t *testing.T) Status {
				if status := coreDisconnect(online); status != StatusOK {
					t.Fatalf("setup disconnect = %d, want StatusOK", status)
				}
				pointer, length := validBatch()
				status := coreSubmitInput(online, pointer, length)
				if destroy := coreDestroy(online); destroy != StatusOK {
					t.Errorf("destroy of the terminal session = %d, want StatusOK", destroy)
				}
				return status
			},
			want: StatusDisconnected,
		},
		{
			name:    "rejection racing teardown reports disconnected",
			defects: "runtime rejection + disconnect during the submission",
			note:    "recorded ruling: the terminal status wins once the teardown published the terminal state",
			invoke: func(t *testing.T) Status {
				terminal := connectSessionOnlineInput(t)
				racing := installInputSubmitSeam(t, func(*clientruntime.Runtime, clientruntime.SemanticInput) error {
					if status := coreDisconnect(terminal); status != StatusOK {
						t.Errorf("disconnect inside the seam = %d, want StatusOK", status)
					}
					return errInputSeamRejected
				})
				pointer, length := validBatch()
				status := coreSubmitInput(terminal, pointer, length)
				if count := racing.count(); count != 1 {
					t.Errorf("racing seam saw %d submissions, want 1", count)
				}
				if destroy := coreDestroy(terminal); destroy != StatusOK {
					t.Errorf("destroy of the racing session = %d, want StatusOK", destroy)
				}
				return status
			},
			want: StatusDisconnected,
		},
	})
}

// TestMatrixStepErrorPrecedence pins the step order — handle lifecycle before
// session state before request shape before record identity and content —
// plus the recorded outcome conflations (`StatusInternal` for producer-side
// failures including queue-full backpressure, and the transient terminal
// race).
func TestMatrixStepErrorPrecedence(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	log := installStepSeam(t, nil)
	validRequest := func() (*byte, uint32) { return stepRequestPointer(0, 1, 1) }
	idleProbe := func(t *testing.T) {
		t.Helper()
		pointer, length := validRequest()
		if status := coreStep(idle, pointer, length); status != StatusInvalidState {
			t.Fatalf("idle probe step = %d, want StatusInvalidState (the session survived)", status)
		}
	}
	runMatrixCells(t, []matrixCell{
		{
			name:     "unknown handle outranks shape and content",
			defects:  "never-issued handle + null request + zero length",
			invoke:   func(t *testing.T) Status { return coreStep(0, nil, 0) },
			recovery: idleProbe,
			want:     StatusInvalidHandle,
		},
		{
			name:     "destroyed handle outranks shape and content",
			defects:  "destroyed handle + null request + zero length",
			invoke:   func(t *testing.T) Status { return coreStep(destroyed, nil, 0) },
			recovery: idleProbe,
			want:     StatusInvalidState,
		},
		{
			name:    "idle session state outranks shape and content",
			defects: "idle session + valid request",
			invoke: func(t *testing.T) Status {
				pointer, length := validRequest()
				return coreStep(idle, pointer, length)
			},
			recovery: idleProbe,
			want:     StatusInvalidState,
		},
		{
			name:     "idle session state outranks the null request",
			defects:  "idle session + null request + zero length",
			invoke:   func(t *testing.T) Status { return coreStep(idle, nil, 0) },
			recovery: idleProbe,
			want:     StatusInvalidState,
		},
	})
	connecting := connectSessionOnlineInputConnecting(t)
	runMatrixCells(t, []matrixCell{
		{
			name:    "connecting session state outranks shape and content",
			defects: "connecting session + valid request",
			invoke: func(t *testing.T) Status {
				pointer, length := validRequest()
				return coreStep(connecting, pointer, length)
			},
			want: StatusInvalidState,
		},
	})
	if status := coreDisconnect(connecting); status != StatusOK {
		t.Fatalf("disconnect of the connecting session = %d, want StatusOK", status)
	}
	if status := coreDestroy(connecting); status != StatusOK {
		t.Fatalf("destroy of the connecting session = %d, want StatusOK", status)
	}

	script := newStepTransportScript()
	online := connectSessionOnlineStep(t, script)
	onlineProbe := func(t *testing.T) {
		t.Helper()
		before := log.count()
		pointer, length := stepRequestPointer(0, 0, 0)
		if status := coreStep(online, pointer, length); status != StatusOK {
			t.Fatalf("valid probe step after rejections = %d, want StatusOK", status)
		}
		if count := log.count(); count != before+1 {
			t.Fatalf("valid probe step executed %d runtime steps, want exactly once more", count-before)
		}
		retained, ok := sessionRetainedStep(t, online)
		if !ok {
			t.Fatal("valid probe step retained no result")
		}
		if retained.Frame.Revision == 0 {
			t.Fatal("retained revision is zero; rejections consumed a step")
		}
	}
	runMatrixCells(t, []matrixCell{
		{
			name:    "online wrong length is a shape defect",
			defects: "length below and above the fixed request size",
			invoke: func(t *testing.T) Status {
				pointer, _ := validRequest()
				if status := coreStep(online, pointer, StepRequestBytes-8); status != StatusInvalidArgument {
					t.Errorf("short length = %d, want StatusInvalidArgument", status)
				}
				return coreStep(online, pointer, StepRequestBytes+8)
			},
			recovery: onlineProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:     "online null request with the exact length is a shape defect",
			defects:  "null request + exact length",
			invoke:   func(t *testing.T) Status { return coreStep(online, nil, StepRequestBytes) },
			recovery: onlineProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "online misaligned request is a shape defect",
			defects: "request one byte off the ABI alignment",
			invoke: func(t *testing.T) Status {
				pointer, length := validRequest()
				storage := make([]byte, int(length)+8)
				copy(storage[1:], unsafe.Slice(pointer, int(length)))
				misaligned := (*byte)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), 1)))
				return coreStep(online, misaligned, length)
			},
			recovery: onlineProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "shape outranks record identity",
			defects: "length off the fixed request size + wrong family magic",
			note:    "the exact-length rule is judged before any buffer byte is read, so the wrong length wins over the corrupt identity",
			invoke: func(t *testing.T) Status {
				pointer, _ := validRequest()
				patchStepWord(pointer, 0, uint32(MagicWorld))
				return coreStep(online, pointer, StepRequestBytes-8)
			},
			recovery: onlineProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "record identity outranks domain content",
			defects: "wrong family magic + over-limit budgets",
			invoke: func(t *testing.T) Status {
				pointer, length := stepRequestPointer(0, MaxStepMessageBudget+1, MaxStepMeshBudget+1)
				patchStepWord(pointer, 0, uint32(MagicWorld))
				return coreStep(online, pointer, length)
			},
			recovery: onlineProbe,
			want:     StatusABIMismatch,
		},
		{
			name:    "layout word outranks domain content",
			defects: "wrong layout version + unrepresentable elapsed",
			invoke: func(t *testing.T) Status {
				pointer, length := stepRequestPointer(^uint64(0), 0, 0)
				patchStepWord(pointer, 4, StepVersion+1)
				return coreStep(online, pointer, length)
			},
			recovery: onlineProbe,
			want:     StatusABIMismatch,
		},
		{
			name:    "unrepresentable elapsed is domain content",
			defects: "elapsed above the maximal representable nanosecond count",
			invoke: func(t *testing.T) Status {
				pointer, length := stepRequestPointer(uint64(1)<<63, 0, 0)
				return coreStep(online, pointer, length)
			},
			recovery: onlineProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "over-limit budgets are domain content",
			defects: "message and mesh budgets above the frozen limits",
			invoke: func(t *testing.T) Status {
				pointer, length := stepRequestPointer(0, MaxStepMessageBudget+1, MaxStepMeshBudget+1)
				return coreStep(online, pointer, length)
			},
			recovery: onlineProbe,
			want:     StatusInputRejected,
		},
		{
			name:    "producer-side failure conflates into internal",
			defects: "step failure on a healthy session",
			note:    "recorded conflation: queue-full backpressure on the outbound queue surfaces exactly here as `StatusInternal`; a compatible new status code is the designated future split",
			invoke: func(t *testing.T) Status {
				calls := 0
				previous := stepRuntime
				stepRuntime = func(established *clientruntime.Runtime, elapsed time.Duration, messageBudget, meshBudget int) (clientruntime.StepResult, error) {
					calls++
					_ = established
					_ = elapsed
					_ = messageBudget
					_ = meshBudget
					return clientruntime.StepResult{}, errStepSeamScripted
				}
				defer func() { stepRuntime = previous }()
				pointer, length := validRequest()
				status := coreStep(online, pointer, length)
				if calls != 1 {
					t.Errorf("failing seam executed %d steps, want 1", calls)
				}
				return status
			},
			recovery: onlineProbe,
			want:     StatusInternal,
		},
		{
			name:    "execution panic converts to the panic status",
			defects: "panicking step execution",
			invoke: func(t *testing.T) Status {
				calls := 0
				previous := stepRuntime
				stepRuntime = func(established *clientruntime.Runtime, elapsed time.Duration, messageBudget, meshBudget int) (clientruntime.StepResult, error) {
					calls++
					_ = established
					_ = elapsed
					_ = messageBudget
					_ = meshBudget
					panic("matrix step seam panic")
				}
				defer func() { stepRuntime = previous }()
				pointer, length := validRequest()
				status := coreStep(online, pointer, length)
				if calls != 1 {
					t.Errorf("panicking seam executed %d steps, want 1", calls)
				}
				return status
			},
			recovery: onlineProbe,
			want:     StatusPanic,
		},
		{
			name:    "failure racing teardown reports disconnected",
			defects: "step failure + user disconnect during the execution",
			note:    "recorded transient: a receiver-driven self-close racing an in-flight step classifies one call as `StatusInternal` instead of `StatusDisconnected` until the session observes the terminal phase; benign and recorded",
			invoke: func(t *testing.T) Status {
				racing := installStepSeam(t, func(*clientruntime.Runtime, time.Duration, int, int) error {
					if status := coreDisconnect(online); status != StatusOK {
						t.Errorf("disconnect inside the seam = %d, want StatusOK", status)
					}
					return errStepSeamScripted
				})
				pointer, length := validRequest()
				status := coreStep(online, pointer, length)
				if count := racing.count(); count != 1 {
					t.Errorf("racing seam executed %d steps, want 1", count)
				}
				return status
			},
			want: StatusDisconnected,
		},
	})
	// The terminal row needs a fresh online session because the racing row
	// already tore `online` down.
	runMatrixCells(t, []matrixCell{
		{
			name:    "terminal session outranks shape and content",
			defects: "terminal session + valid request",
			invoke: func(t *testing.T) Status {
				terminal := connectSessionOnlineStep(t, script)
				if status := coreDisconnect(terminal); status != StatusOK {
					t.Fatalf("setup disconnect = %d, want StatusOK", status)
				}
				pointer, length := validRequest()
				status := coreStep(terminal, pointer, length)
				if destroy := coreDestroy(terminal); destroy != StatusOK {
					t.Errorf("destroy of the terminal session = %d, want StatusOK", destroy)
				}
				return status
			},
			want: StatusDisconnected,
		},
		{
			name:    "valid request steps and retains",
			defects: "none (positive control)",
			invoke: func(t *testing.T) Status {
				handle := connectSessionOnlineStep(t, script)
				pointer, length := stepRequestPointer(0, 0, 0)
				status := coreStep(handle, pointer, length)
				if _, ok := sessionRetainedStep(t, handle); !ok && status == StatusOK {
					t.Error("successful step retained no result")
				}
				if destroy := coreDestroy(handle); destroy != StatusOK {
					t.Errorf("destroy after the control step = %d, want StatusOK", destroy)
				}
				return status
			},
			want: StatusOK,
		},
	})
}

// TestMatrixWorldPullErrorPrecedence pins the world pull order — handle
// lifecycle before pointer shape (including the aliased size word) before the
// two-phase drain — with the encoder rejection mapping and the
// retention-survives-teardown ruling.
func TestMatrixWorldPullErrorPrecedence(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	installWorldResultSeam(t, worldSeamResult(1, worldSeamMixedBatch(t, 7, 900), true))
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("setup step = %d, want StatusOK", status)
	}
	required, status := worldQuerySize(t, handle)
	if status != StatusInsufficientCapacity || required == 0 {
		t.Fatalf("setup query = (%d, %d), want a positive size and the capacity signal", required, status)
	}
	liveProbe := func(t *testing.T) {
		t.Helper()
		again, status := worldQuerySize(t, handle)
		if status != StatusInsufficientCapacity || again != required {
			t.Fatalf("query after rejections = (%d, %d), want (%d, the capacity signal): the batch must stay consumable", again, status, required)
		}
	}
	invokeLive := func(buffer []byte, sizeOut *uint32) Status {
		var pointer *byte
		var capacity uint32
		if buffer != nil {
			pointer = &buffer[0]
			capacity = uint32(len(buffer))
		}
		return coreWorldPull(handle, pointer, capacity, sizeOut)
	}
	runMatrixCells(t, []matrixCell{
		{
			name:    "unknown handle outranks the null size word",
			defects: "never-issued handle + null buffer + null size word",
			invoke: func(t *testing.T) Status {
				var pointer *byte
				return coreWorldPull(0, pointer, 0, nil)
			},
			recovery: liveProbe,
			want:     StatusInvalidHandle,
		},
		{
			name:    "destroyed handle outranks the null size word",
			defects: "destroyed handle + null buffer + null size word",
			invoke: func(t *testing.T) Status {
				var pointer *byte
				return coreWorldPull(destroyed, pointer, 0, nil)
			},
			recovery: liveProbe,
			want:     StatusInvalidState,
		},
		{
			name:    "live handle requires the size word",
			defects: "null size word + valid buffer",
			invoke: func(t *testing.T) Status {
				buffer, _ := matrixCallerFrame(required)
				status := invokeLive(buffer, nil)
				matrixAssertPoisoned(t, buffer, "null-size-word world pull")
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "positive capacity requires the buffer",
			defects: "null buffer + valid size word",
			invoke: func(t *testing.T) Status {
				size := uint32(0xDEADBEEF)
				status := coreWorldPull(handle, nil, required, &size)
				if size != 0xDEADBEEF {
					t.Errorf("shape rejection wrote the size word %d despite failure", size)
				}
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "misaligned buffer",
			defects: "buffer one byte off the ABI alignment",
			invoke: func(t *testing.T) Status {
				buffer := matrixMisalignedBuffer(required)
				status := coreWorldPull(handle, &buffer[0], required, new(uint32))
				matrixAssertPoisoned(t, buffer, "misaligned world pull")
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "size word inside the buffer span",
			defects: "aliased size word pointing into the write buffer",
			invoke: func(t *testing.T) Status {
				buffer, sizeOut := matrixAliasedFrame(required)
				status := coreWorldPull(handle, &buffer[0], required, sizeOut)
				matrixAssertPoisoned(t, buffer, "aliased world pull")
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "aliased size word outranks the capacity signal",
			defects: "size word inside the buffer span + capacity below the required size",
			invoke: func(t *testing.T) Status {
				// The aliased size word lives inside the span, so its bytes
				// are part of the buffer: the full-span poison check proves
				// neither the record nor a size write landed anywhere.
				buffer, sizeOut := matrixAliasedFrame(required - 8)
				status := invokeLive(buffer, sizeOut)
				matrixAssertPoisoned(t, buffer, "aliased short-capacity world pull")
				return status
			},
			note:     "alias containment precedes the capacity signal; a reorder would corrupt the caller buffer by writing the size word inside its span",
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "short capacity is the two-phase signal",
			defects: "capacity below the required size",
			invoke: func(t *testing.T) Status {
				buffer, sizeOut := matrixCallerFrame(required - 8)
				*sizeOut = 0xDEADBEEF
				status := invokeLive(buffer, sizeOut)
				matrixAssertPoisoned(t, buffer, "short-capacity world pull")
				if *sizeOut != required {
					t.Errorf("short-capacity pull reported required %d, want %d", *sizeOut, required)
				}
				return status
			},
			note:     "the size word is the one documented output the failure path may write",
			recovery: liveProbe,
			want:     StatusInsufficientCapacity,
		},
		{
			name:    "encoder rejection maps to internal without consuming",
			defects: "retained batch fails presentation validation",
			invoke: func(t *testing.T) Status {
				previous := worldBatchEncode
				worldBatchEncode = func(presentation.WorldBatch) []byte { return nil }
				defer func() { worldBatchEncode = previous }()
				buffer, sizeOut := matrixCallerFrame(required)
				*sizeOut = 0xDEADBEEF
				status := invokeLive(buffer, sizeOut)
				matrixAssertPoisoned(t, buffer, "encoder-rejected world pull")
				if *sizeOut != 0xDEADBEEF {
					t.Errorf("encoder rejection wrote the size word %d despite failure", *sizeOut)
				}
				return status
			},
			recovery: liveProbe,
			want:     StatusInternal,
		},
		{
			name:    "sufficient capacity consumes exactly once",
			defects: "none (positive control)",
			invoke: func(t *testing.T) Status {
				buffer, sizeOut := matrixCallerFrame(required)
				*sizeOut = 0xDEADBEEF
				status := invokeLive(buffer, sizeOut)
				if status == StatusOK {
					if magic := buffer[0]; magic == 0xA5 {
						t.Error("successful consume wrote nothing")
					}
					again, queryStatus := worldQuerySize(t, handle)
					if queryStatus != StatusOK || again != 0 {
						t.Errorf("query after consume = (%d, %d), want (0, StatusOK): consumption commits exactly once", again, queryStatus)
					}
				}
				return status
			},
			want: StatusOK,
		},
	})
	// The terminal row needs retained content, so it re-arms the seam after
	// the positive control consumed the first batch.
	runMatrixCells(t, []matrixCell{
		{
			name:    "terminal session still serves the retained batch",
			defects: "terminal session + sufficient capacity",
			note:    "recorded ruling: retention deliberately survives teardown so the terminal batch stays pullable",
			invoke: func(t *testing.T) Status {
				installWorldResultSeam(t, worldSeamResult(2, worldSeamDropBatch(t, 11, 12), true))
				if status := worldStepOnce(t, handle); status != StatusOK {
					t.Fatalf("setup step = %d, want StatusOK", status)
				}
				if status := coreDisconnect(handle); status != StatusOK {
					t.Fatalf("setup disconnect = %d, want StatusOK", status)
				}
				required, status := worldQuerySize(t, handle)
				if status != StatusInsufficientCapacity {
					t.Fatalf("query on the terminal session = %d, want the capacity signal", status)
				}
				buffer, _, status := worldConsume(handle, required)
				if status == StatusOK && len(buffer) < int(WorldHeaderBytes) {
					t.Error("terminal consume wrote no header")
				}
				return status
			},
			want: StatusOK,
		},
	})
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("final destroy = %d, want StatusOK", status)
	}
}

// TestMatrixFramePullErrorPrecedence pins the frame pull order — handle
// lifecycle before pointer shape (including the aliased size word) before the
// non-consuming serve — with the encoder rejection mapping and the
// terminal-frame ruling.
func TestMatrixFramePullErrorPrecedence(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	installWorldResultSeam(t, clientruntime.StepResult{Frame: worldSeamFrame(1)})
	if status := worldStepOnce(t, handle); status != StatusOK {
		t.Fatalf("setup step = %d, want StatusOK", status)
	}
	required, status := frameQuerySize(t, handle)
	if status != StatusInsufficientCapacity || required == 0 {
		t.Fatalf("setup query = (%d, %d), want a positive size and the capacity signal", required, status)
	}
	liveProbe := func(t *testing.T) {
		t.Helper()
		again, status := frameQuerySize(t, handle)
		if status != StatusInsufficientCapacity || again != required {
			t.Fatalf("query after rejections = (%d, %d), want (%d, the capacity signal): the frame must stay servable", again, status, required)
		}
	}
	runMatrixCells(t, []matrixCell{
		{
			name:    "unknown handle outranks the null size word",
			defects: "never-issued handle + null buffer + null size word",
			invoke: func(t *testing.T) Status {
				var pointer *byte
				return coreFramePull(0, pointer, 0, nil)
			},
			recovery: liveProbe,
			want:     StatusInvalidHandle,
		},
		{
			name:    "destroyed handle outranks the null size word",
			defects: "destroyed handle + null buffer + null size word",
			invoke: func(t *testing.T) Status {
				var pointer *byte
				return coreFramePull(destroyed, pointer, 0, nil)
			},
			recovery: liveProbe,
			want:     StatusInvalidState,
		},
		{
			name:    "live handle requires the size word",
			defects: "null size word + valid buffer",
			invoke: func(t *testing.T) Status {
				buffer, _ := matrixCallerFrame(required)
				status := coreFramePull(handle, &buffer[0], required, nil)
				matrixAssertPoisoned(t, buffer, "null-size-word frame pull")
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "positive capacity requires the buffer",
			defects: "null buffer + valid size word",
			invoke: func(t *testing.T) Status {
				size := uint32(0xDEADBEEF)
				status := coreFramePull(handle, nil, required, &size)
				if size != 0xDEADBEEF {
					t.Errorf("shape rejection wrote the size word %d despite failure", size)
				}
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "misaligned buffer",
			defects: "buffer one byte off the ABI alignment",
			invoke: func(t *testing.T) Status {
				buffer := matrixMisalignedBuffer(required)
				status := coreFramePull(handle, &buffer[0], required, new(uint32))
				matrixAssertPoisoned(t, buffer, "misaligned frame pull")
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "size word inside the buffer span",
			defects: "aliased size word pointing into the write buffer",
			invoke: func(t *testing.T) Status {
				buffer, sizeOut := matrixAliasedFrame(required)
				status := coreFramePull(handle, &buffer[0], required, sizeOut)
				matrixAssertPoisoned(t, buffer, "aliased frame pull")
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "aliased size word outranks the capacity signal",
			defects: "size word inside the buffer span + capacity below the required size",
			invoke: func(t *testing.T) Status {
				// The aliased size word lives inside the span, so the
				// full-span poison check proves neither the record nor a
				// size write landed anywhere.
				buffer, sizeOut := matrixAliasedFrame(required - 8)
				status := coreFramePull(handle, &buffer[0], uint32(len(buffer)), sizeOut)
				matrixAssertPoisoned(t, buffer, "aliased short-capacity frame pull")
				return status
			},
			note:     "alias containment precedes the capacity signal; a reorder would corrupt the caller buffer by writing the size word inside its span",
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "short capacity is the two-phase signal",
			defects: "capacity below the required size",
			invoke: func(t *testing.T) Status {
				buffer, sizeOut := matrixCallerFrame(required - 8)
				*sizeOut = 0xDEADBEEF
				status := coreFramePull(handle, &buffer[0], uint32(len(buffer)), sizeOut)
				matrixAssertPoisoned(t, buffer, "short-capacity frame pull")
				if *sizeOut != required {
					t.Errorf("short-capacity pull reported required %d, want %d", *sizeOut, required)
				}
				return status
			},
			note:     "the size word is the one documented output the failure path may write",
			recovery: liveProbe,
			want:     StatusInsufficientCapacity,
		},
		{
			name:    "encoder rejection maps to internal without output",
			defects: "retained frame fails presentation validation",
			invoke: func(t *testing.T) Status {
				previous := frameEncode
				frameEncode = func(presentation.FrameSnapshot) []byte { return nil }
				defer func() { frameEncode = previous }()
				buffer, sizeOut := matrixCallerFrame(required)
				*sizeOut = 0xDEADBEEF
				status := coreFramePull(handle, &buffer[0], required, sizeOut)
				matrixAssertPoisoned(t, buffer, "encoder-rejected frame pull")
				if *sizeOut != 0xDEADBEEF {
					t.Errorf("encoder rejection wrote the size word %d despite failure", *sizeOut)
				}
				return status
			},
			recovery: liveProbe,
			want:     StatusInternal,
		},
		{
			name:    "sufficient capacity serves without consuming",
			defects: "none (positive control)",
			invoke: func(t *testing.T) Status {
				buffer, sizeOut := matrixCallerFrame(required)
				*sizeOut = 0xDEADBEEF
				status := coreFramePull(handle, &buffer[0], required, sizeOut)
				if status == StatusOK {
					if buffer[0] == 0xA5 {
						t.Error("successful serve wrote nothing")
					}
					again, queryStatus := frameQuerySize(t, handle)
					if queryStatus != StatusInsufficientCapacity || again != required {
						t.Errorf("query after serve = (%d, %d), want (%d, the capacity signal): serving consumes nothing", again, queryStatus, required)
					}
				}
				return status
			},
			want: StatusOK,
		},
	})
	runMatrixCells(t, []matrixCell{
		{
			name:    "terminal session still serves the retained frame",
			defects: "terminal session + sufficient capacity",
			note:    "recorded ruling: retention deliberately survives teardown so the terminal frame stays pullable",
			invoke: func(t *testing.T) Status {
				if status := coreDisconnect(handle); status != StatusOK {
					t.Fatalf("setup disconnect = %d, want StatusOK", status)
				}
				required, status := frameQuerySize(t, handle)
				if status != StatusInsufficientCapacity {
					t.Fatalf("query on the terminal session = %d, want the capacity signal", status)
				}
				buffer, _, status := frameConsume(handle, required)
				if status == StatusOK && len(buffer) < int(FrameHeaderBytes) {
					t.Error("terminal serve wrote no header")
				}
				return status
			},
			want: StatusOK,
		},
	})
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("final destroy = %d, want StatusOK", status)
	}
}

// TestMatrixStatusPullErrorPrecedence pins the status pull order — handle
// lifecycle before pointer shape (including the aliased size word) before the
// non-consuming serve — with the encoder rejection mapping. The family always
// has content, so a query never reports the zero size.
func TestMatrixStatusPullErrorPrecedence(t *testing.T) {
	resetSessionTable(t)
	live := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	required, status := metricsQuerySize(t, live)
	if status != StatusInsufficientCapacity || required == 0 {
		t.Fatalf("setup query = (%d, %d), want a positive size and the capacity signal", required, status)
	}
	liveProbe := func(t *testing.T) {
		t.Helper()
		again, status := metricsQuerySize(t, live)
		if status != StatusInsufficientCapacity || again != required {
			t.Fatalf("query after rejections = (%d, %d), want (%d, the capacity signal)", again, status, required)
		}
	}
	runMatrixCells(t, []matrixCell{
		{
			name:    "unknown handle outranks the null size word",
			defects: "never-issued handle + null buffer + null size word",
			invoke: func(t *testing.T) Status {
				var pointer *byte
				return coreStatusPull(0, pointer, 0, nil)
			},
			recovery: liveProbe,
			want:     StatusInvalidHandle,
		},
		{
			name:    "destroyed handle outranks the null size word",
			defects: "destroyed handle + null buffer + null size word",
			invoke: func(t *testing.T) Status {
				var pointer *byte
				return coreStatusPull(destroyed, pointer, 0, nil)
			},
			recovery: liveProbe,
			want:     StatusInvalidState,
		},
		{
			name:    "live handle requires the size word",
			defects: "null size word + valid buffer",
			invoke: func(t *testing.T) Status {
				buffer, _ := matrixCallerFrame(required)
				status := coreStatusPull(live, &buffer[0], required, nil)
				matrixAssertPoisoned(t, buffer, "null-size-word status pull")
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "positive capacity requires the buffer",
			defects: "null buffer + valid size word",
			invoke: func(t *testing.T) Status {
				size := uint32(0xDEADBEEF)
				status := coreStatusPull(live, nil, required, &size)
				if size != 0xDEADBEEF {
					t.Errorf("shape rejection wrote the size word %d despite failure", size)
				}
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "misaligned buffer",
			defects: "buffer one byte off the ABI alignment",
			invoke: func(t *testing.T) Status {
				buffer := matrixMisalignedBuffer(required)
				status := coreStatusPull(live, &buffer[0], required, new(uint32))
				matrixAssertPoisoned(t, buffer, "misaligned status pull")
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "size word inside the buffer span",
			defects: "aliased size word pointing into the write buffer",
			invoke: func(t *testing.T) Status {
				buffer, sizeOut := matrixAliasedFrame(required)
				status := coreStatusPull(live, &buffer[0], required, sizeOut)
				matrixAssertPoisoned(t, buffer, "aliased status pull")
				return status
			},
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "aliased size word outranks the capacity signal",
			defects: "size word inside the buffer span + capacity below the required size",
			invoke: func(t *testing.T) Status {
				// The aliased size word lives inside the span, so the
				// full-span poison check proves neither the record nor a
				// size write landed anywhere.
				buffer, sizeOut := matrixAliasedFrame(required - 8)
				status := coreStatusPull(live, &buffer[0], uint32(len(buffer)), sizeOut)
				matrixAssertPoisoned(t, buffer, "aliased short-capacity status pull")
				return status
			},
			note:     "alias containment precedes the capacity signal; a reorder would corrupt the caller buffer by writing the size word inside its span",
			recovery: liveProbe,
			want:     StatusInvalidArgument,
		},
		{
			name:    "short capacity is the two-phase signal",
			defects: "capacity below the required size",
			invoke: func(t *testing.T) Status {
				buffer, sizeOut := matrixCallerFrame(required - 8)
				*sizeOut = 0xDEADBEEF
				status := coreStatusPull(live, &buffer[0], uint32(len(buffer)), sizeOut)
				matrixAssertPoisoned(t, buffer, "short-capacity status pull")
				if *sizeOut != required {
					t.Errorf("short-capacity pull reported required %d, want %d", *sizeOut, required)
				}
				return status
			},
			note:     "the size word is the one documented output the failure path may write",
			recovery: liveProbe,
			want:     StatusInsufficientCapacity,
		},
		{
			name:    "encoder rejection maps to internal without output",
			defects: "status encoder invariant break",
			invoke: func(t *testing.T) Status {
				previous := statusEncode
				statusEncode = func(statusSnapshot) []byte { return nil }
				defer func() { statusEncode = previous }()
				buffer, sizeOut := matrixCallerFrame(required)
				*sizeOut = 0xDEADBEEF
				status := coreStatusPull(live, &buffer[0], required, sizeOut)
				matrixAssertPoisoned(t, buffer, "encoder-rejected status pull")
				if *sizeOut != 0xDEADBEEF {
					t.Errorf("encoder rejection wrote the size word %d despite failure", *sizeOut)
				}
				return status
			},
			recovery: liveProbe,
			want:     StatusInternal,
		},
		{
			name:    "sufficient capacity serves without consuming",
			defects: "none (positive control)",
			invoke: func(t *testing.T) Status {
				buffer, sizeOut := matrixCallerFrame(required)
				*sizeOut = 0xDEADBEEF
				status := coreStatusPull(live, &buffer[0], required, sizeOut)
				if status == StatusOK && buffer[0] == 0xA5 {
					t.Error("successful serve wrote nothing")
				}
				return status
			},
			recovery: liveProbe,
			want:     StatusOK,
		},
		{
			name:    "terminal session still serves its record set",
			defects: "terminal session + sufficient capacity",
			note:    "recorded ruling: the pull observes session state, so a terminal session publishes the disconnected phase record",
			invoke: func(t *testing.T) Status {
				if status := coreDisconnect(live); status != StatusOK {
					t.Fatalf("setup disconnect = %d, want StatusOK", status)
				}
				required, status := metricsQuerySize(t, live)
				if status != StatusInsufficientCapacity {
					t.Fatalf("query on the terminal session = %d, want the capacity signal", status)
				}
				buffer, _, status := metricsConsume(live, required)
				if status == StatusOK && len(buffer) < int(StatusHeaderBytes) {
					t.Error("terminal serve wrote no header")
				}
				return status
			},
			want: StatusOK,
		},
	})
	if status := coreDestroy(live); status != StatusOK {
		t.Fatalf("final destroy = %d, want StatusOK", status)
	}
}

// TestMatrixAbiVersionAccessorCannotFail pins the one export without a status
// return: the packed identity word is pure constant arithmetic, so it cannot
// fail and carries the producer major and minor verbatim.
func TestMatrixAbiVersionAccessorCannotFail(t *testing.T) {
	packed := coreAbiVersion()
	if major := uint32(packed >> 32); major != ABIMajor {
		t.Fatalf("packed ABI major = %d, want %d", major, ABIMajor)
	}
	if minor := uint32(packed); minor != ABIMinor {
		t.Fatalf("packed ABI minor = %d, want %d", minor, ABIMinor)
	}
}
