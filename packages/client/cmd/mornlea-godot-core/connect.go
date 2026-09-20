//go:build cgo

package main

import (
	"context"
	"fmt"
	"sync"
	"unicode/utf8"
	"unsafe"

	"github.com/channing771/mornlea/packages/client/assets"
	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/client/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// This file owns the connection family export surface: asynchronous connect
// begin (`coreConnectBegin`), bounded phase polling (`coreConnectPoll`), and
// the idempotent disconnect (`coreDisconnect`). Establishment (DNS, TCP,
// login) runs on one goroutine owned by the session's connection state, so
// the calling thread never waits; the exports below only lock bounded state
// and never block on the network. A poll performs bounded work only: one
// session lock acquisition and, while online, one runtime phase read. The
// phase read takes the runtime's own phase lock, and on the terminal path
// the runtime's self-close of already-failed resources runs inside it —
// bounded socket-close work, never a network wait.

// Producer-side connection phase words reported by `coreConnectPoll`. The
// mapping is the identity cast of `presentation.SessionPhase` through
// `runtime.ConnectionPhase`, so the vocabulary stays 0..5 and moves only with
// the presentation contract that owns it. These are producer-side constants
// documented here rather than new header defines: the frozen
// include/mornlea_client_core.h gains wire vocabulary only through a
// reviewed compatible addition, and a phase word that today only mirrors an
// existing Go contract does not need one.
const (
	// ConnectPhaseNotReady marks a live session that never began connecting.
	ConnectPhaseNotReady = uint32(runtime.ConnectionPhaseNotReady)
	// ConnectPhaseConnecting covers the whole establishment (dial and login);
	// `runtime.NewRemote` performs both as one synchronous step and first
	// publishes Loading, so the core reports Connecting until the runtime
	// exists.
	ConnectPhaseConnecting = uint32(runtime.ConnectionPhaseConnecting)
	// ConnectPhaseLogin exists in the presentation vocabulary but is never
	// reported by this producer generation; see `ConnectPhaseConnecting`.
	ConnectPhaseLogin = uint32(runtime.ConnectionPhaseLogin)
	// ConnectPhaseLoading means login succeeded and the runtime is live.
	ConnectPhaseLoading = uint32(runtime.ConnectionPhaseLoading)
	// ConnectPhasePlay means the runtime published playable state; the step
	// family drives the readiness transition, and the poll forwards the
	// runtime phase unchanged.
	ConnectPhasePlay = uint32(runtime.ConnectionPhasePlay)
	// ConnectPhaseDisconnected is the implied phase whenever the poll returns
	// `StatusDisconnected`; the terminal status itself carries it because no
	// output byte is written on a non-OK status.
	ConnectPhaseDisconnected = uint32(runtime.ConnectionPhaseDisconnected)
)

// Pilot login inputs sent with every establishment. The pilot connection
// family carries no identity or profile negotiation yet: the connect begin
// export takes only the address, so a fixed UUIDv4 pilot identity, the
// minimum login view distance, and the legacy receiver bound are producer
// constants. Identity and view-distance configuration arrives as a
// compatible connection-family addition in a later change.
const (
	// pilotConnectPlayerIDText is the fixed pilot UUIDv4 login identity.
	pilotConnectPlayerIDText = "6a3f2c1e-9b4d-4e8a-a1c2-5d7e8f9a0b1c"
	// pilotConnectDisplayName is the fixed pilot display name.
	pilotConnectDisplayName = "GodotPilot"
	// pilotConnectViewDistance declares the minimum pilot view distance.
	pilotConnectViewDistance = protocol.LoginViewDistanceMin
	// pilotConnectReceiverCapacity mirrors the legacy application receiver
	// capacity so the pilot inbound queue keeps the same backpressure bound
	// as the existing remote frame loop.
	pilotConnectReceiverCapacity = 8192
)

// pilotConnectIdentity parses the fixed pilot login identity. The literal is
// a test-pinned UUIDv4, so parsing cannot fail; the error path exists only to
// keep the connect goroutine panic-free if that invariant ever breaks.
func pilotConnectIdentity() (network.Identity, error) {
	playerID, err := core.ParsePlayerID(pilotConnectPlayerIDText)
	if err != nil {
		return network.Identity{}, fmt.Errorf("client core: pilot login identity: %w", err)
	}
	return network.Identity{PlayerID: playerID, DisplayName: pilotConnectDisplayName}, nil
}

// remoteSessionDependencies supplies the transport seams handed to
// `runtime.NewRemote`. Production returns the zero value so the runtime uses
// the real TCP dial, the v44 login state machine, and the bounded receiver.
// It is a variable so tests script dial, login, and receiver behavior
// without network I/O, reusing the runtime package's own injection seam; it
// is never redefined outside tests.
var remoteSessionDependencies = func() runtime.RemoteDependencies {
	return runtime.RemoteDependencies{}
}

// Pilot meshing inputs. The world family publishes section batches only when
// the runtime owns a mesher (`runtime.DrainWorldBatch` reports no batch for a
// mesher-less runtime), so every online pilot session assembles one over the
// placeholder-material registry — the mesher consumes only the registry's
// atlas-layer assignments, never its generated pixels, so the pilot renders
// with the real generated atlas the presentation host owns. Two workers keep
// the near-ring pilot's section expansion off the caller thread while staying
// a fixed, deterministic assembly cost, and the ready capacity is the frozen
// maximal world batch so a full subscription burst never blocks publication.
// The atlas revision is the neutral positive identity `AdoptMeshing` pins
// when a host supplies no atlas of its own.
const (
	pilotMeshWorkers       = 2
	pilotMeshReadyCapacity = presentation.MaxWorldBatchOperations
)

// pilotMeshOptions returns the runtime-owned mesh configuration every online
// pilot session connects with. The registry is built per establishment: it is
// bounded, deterministic, and owned by the runtime once handed over.
func pilotMeshOptions() *runtime.MeshOptions {
	return &runtime.MeshOptions{
		Registry:      assets.NewRegistry(),
		Workers:       pilotMeshWorkers,
		ReadyCapacity: pilotMeshReadyCapacity,
		AtlasRevision: presentation.AtlasRevision(1),
	}
}

// openRemoteSession assembles one remote session through the platform
// independent runtime. All blocking establishment work (DNS/TCP dial, v44
// handshake, login, receiver start, mesh worker start) happens inside
// `runtime.NewRemote` under the caller's context, which is the sole
// cancellation path.
func openRemoteSession(ctx context.Context, address string) (*runtime.Runtime, error) {
	identity, err := pilotConnectIdentity()
	if err != nil {
		return nil, err
	}
	return runtime.NewRemote(ctx, runtime.Options{
		RemoteAddress:      address,
		Identity:           identity,
		ViewDistance:       pilotConnectViewDistance,
		ReceiverCapacity:   pilotConnectReceiverCapacity,
		Mesh:               pilotMeshOptions(),
		RemoteDependencies: remoteSessionDependencies(),
	})
}

// connectState is one session's connection lifecycle.
type connectState uint8

const (
	// connectStateIdle means no establishment was ever begun.
	connectStateIdle connectState = iota
	// connectStateEstablishing means the connect goroutine is inside
	// `runtime.NewRemote`.
	connectStateEstablishing
	// connectStateOnline means the runtime exists and the poll delegates the
	// phase to it.
	connectStateOnline
	// connectStateTerminal is the reached-disconnect state, clean or failed.
	connectStateTerminal
)

// clientSession is the per-session connection state owned by one session
// table slot. All fields are guarded by `mu` except `stepMu`, which is its own
// leaf lock owned by the step family (see step.go); the object identity is
// fixed at slot claim, so exports resolve the pointer through the table and
// then synchronize here. The connect goroutine locks only `mu` (never the
// table lock), so the lock order is table -> session everywhere and teardown
// never deadlocks.
type clientSession struct {
	mu          sync.Mutex
	state       connectState
	runtime     *runtime.Runtime
	terminalErr error
	cancel      context.CancelFunc
	done        chan struct{}
	// `stepMu` serializes the step family's single-driver discipline. It is
	// acquired only by the step export and never while `mu` is held, so the
	// step -> session lock order cannot invert against table -> session.
	stepMu sync.Mutex
	// `worldMu` serializes the world pull family's two-phase drain (see
	// world.go). It is acquired only by the world pull export and never while
	// `mu` or `stepMu` is held, mirroring `stepMu`'s discipline so the
	// pull -> session lock order cannot invert against table -> session.
	worldMu sync.Mutex
	// The latest completed step result plus its presence flag are retained
	// under `mu` for the world and frame pull families; see step.go for the
	// single-producer retention ruling. Retention deliberately survives
	// teardown so a terminal frame stays pullable after a disconnect.
	stepResult    runtime.StepResult
	hasStepResult bool
	// `terminalClass` is the status family's terminal-cause classification of
	// `terminalErr`, recorded at the two terminal record sites (establishment
	// failure and receiver death) and guarded by `mu`; a clean user disconnect
	// records neither error nor class, which is the none classification.
	terminalClass uint32
	// `messagesProcessedTotal` accumulates the inbound message counts of
	// every retained step result for the status family's counter record; it
	// is guarded by `mu` and updated with each retention.
	messagesProcessedTotal uint64
	// World-pull bookkeeping guarded by `mu`: `stepGeneration` increments on
	// every step-result retention and `worldPulledGeneration` names the
	// generation whose world batch the world pull already consumed. The
	// generation pair gives the pull family a monotonic identity for the
	// retained result without borrowing frame-revision semantics; see
	// world.go for the consumption protocol.
	stepGeneration        uint64
	worldPulledGeneration uint64
}

// begin starts one establishment. Only an idle session may begin: a second
// begin in any begun phase (establishing, online, or terminal) reports
// `StatusInvalidState` and a retry creates a new session.
func (session *clientSession) begin(address string) Status {
	session.mu.Lock()
	if session.state != connectStateIdle {
		session.mu.Unlock()
		return StatusInvalidState
	}
	ctx, cancel := context.WithCancel(context.Background())
	session.state = connectStateEstablishing
	session.cancel = cancel
	session.done = make(chan struct{})
	session.mu.Unlock()
	go session.establish(ctx, address)
	return StatusOK
}

// establish is the connect goroutine body: it drives `runtime.NewRemote` to
// completion and publishes the outcome. When teardown already claimed the
// terminal transition (a disconnect or destroy raced the establishment), the
// goroutine owns releasing a runtime that slipped through despite the
// cancellation; otherwise a failure records the terminal cause (dial error,
// v44 version rejection, receiver construction failure) with its status-family
// classification, and a success publishes the runtime for polling.
// `establishSession` converts any producer-internal panic into the terminal
// error so no panic escapes a session goroutine.
func (session *clientSession) establish(ctx context.Context, address string) {
	defer close(session.done)
	established, err := establishSession(ctx, address)
	session.mu.Lock()
	switch {
	case session.state != connectStateEstablishing:
		session.mu.Unlock()
		if established != nil {
			_ = established.Close()
		}
	case err != nil:
		session.terminalErr = err
		session.terminalClass = classifyEstablishmentTerminal(err)
		session.state = connectStateTerminal
		session.cancel()
		session.mu.Unlock()
	default:
		session.runtime = established
		session.state = connectStateOnline
		session.mu.Unlock()
	}
}

// establishSession runs one establishment with goroutine-local panic
// conversion; the `withPanicGuard` export guard cannot see this goroutine.
func establishSession(ctx context.Context, address string) (established *runtime.Runtime, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			established = nil
			err = fmt.Errorf("client core: connect goroutine recovered panic: %v", recovered)
		}
	}()
	return openRemoteSession(ctx, address)
}

// poll reports the current connection state into the phase out-parameter.
// Live states write the phase word and return `StatusOK`; the terminal state
// returns `StatusDisconnected` and writes nothing, so the status itself is
// the disconnect signal (failure atomicity). The observation itself is shared
// with the status family through `phaseWord`.
func (session *clientSession) poll(outPhase *uint32) Status {
	phase, status := session.phaseWord()
	if status != StatusOK {
		return status
	}
	*outPhase = phase
	return StatusOK
}

// phaseWord reports the session's current connection phase word using the
// connect family's `ConnectPhase*` vocabulary. It is the single connection
// observation shared by the connect poll and the status family's phase record
// (see status.go), so both families observe one lifecycle. While online the
// word delegates to `runtime.Runtime.Phase`, and a runtime that died from a
// receiver terminal error transitions the session to terminal here. The
// terminal outcome returns `StatusDisconnected` with the implied
// `ConnectPhaseDisconnected` word: the connect poll keeps its no-write failure
// atomicity, while the status family publishes the same word as a record.
func (session *clientSession) phaseWord() (uint32, Status) {
	session.mu.Lock()
	state := session.state
	established := session.runtime
	session.mu.Unlock()
	switch state {
	case connectStateIdle:
		return ConnectPhaseNotReady, StatusOK
	case connectStateEstablishing:
		return ConnectPhaseConnecting, StatusOK
	case connectStateOnline:
		phase := established.Phase()
		if phase != runtime.ConnectionPhaseDisconnected {
			return uint32(phase), StatusOK
		}
		session.recordRuntimeTerminal(established)
		return ConnectPhaseDisconnected, StatusDisconnected
	default:
		return ConnectPhaseDisconnected, StatusDisconnected
	}
}

// recordRuntimeTerminal records a receiver-driven disconnect observed through
// `runtime.Runtime.Phase`, which closes the runtime itself before reporting
// the disconnected phase. A teardown that claimed the terminal transition
// first keeps its own (clean) cause. The terminal class is recorded by
// provenance: this site exists only on the receiver-death path, so the class
// is `TerminalCauseReceiver` regardless of the receiver's own error value.
func (session *clientSession) recordRuntimeTerminal(established *runtime.Runtime) {
	err := established.Err()
	session.mu.Lock()
	if session.state == connectStateOnline {
		session.state = connectStateTerminal
		session.runtime = nil
		session.terminalErr = err
		session.terminalClass = TerminalCauseReceiver
		if session.cancel != nil {
			session.cancel()
		}
	}
	session.mu.Unlock()
}

// disconnect tears the session's connection down: it cancels the connect
// context (unblocking dial/login through `runtime.NewRemote`), closes a
// published runtime, and joins the connect goroutine before returning, so a
// returned disconnect leaves no establishment work behind. It is idempotent
// and safe on idle sessions (nothing begun, nothing to join); `coreDestroy`
// routes slot retirement through the same path so a destroy during an
// in-flight connect cancels and joins too.
func (session *clientSession) disconnect() Status {
	session.mu.Lock()
	if session.state == connectStateIdle {
		session.mu.Unlock()
		return StatusOK
	}
	cancel := session.cancel
	established := session.runtime
	done := session.done
	session.runtime = nil
	session.state = connectStateTerminal
	session.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if established != nil {
		_ = established.Close()
	}
	if done != nil {
		<-done
	}
	return StatusOK
}

// terminalError reports the recorded terminal cause for the session. It is
// producer-internal state today (tests pin it and the later status/metrics
// family publishes it); a clean user disconnect records no error.
func (session *clientSession) terminalError() error {
	session.mu.Lock()
	defer session.mu.Unlock()
	return session.terminalErr
}

// coreConnectBegin validates a connection request and starts the async
// establishment. It is the testable core behind the exported connect begin
// symbol; the raw pointer arguments mirror the C surface.
//
// Validation order, each phase returning before the next runs:
//  1. Handle lifecycle (table): an unknown value reports
//     `StatusInvalidHandle` and a destroyed session `StatusInvalidState`,
//     both before any argument is read.
//  2. Pointer and length shape: a zero length, a length above
//     `MaxConnectionAddressBytes`, a null address, or an address not aligned
//     to `ABIAlignment` (the connection record buffer rule) reports
//     `StatusInvalidArgument`.
//  3. Content: the address bytes must be valid UTF-8 without NUL or ASCII
//     control bytes (every byte at least 0x20 and not 0x7F); violations
//     report `StatusInputRejected`. Semantic host:port validity is delegated
//     to the dialer and surfaces through the poll terminal path.
//  4. Connection state: a session that already began connecting reports
//     `StatusInvalidState`; a retry creates a new session.
//
// Only full success spawns the connect goroutine and returns `StatusOK`
// immediately; establishment never blocks the calling thread. The address is
// copied into the establishment, so no caller pointer is retained after the
// call returns.
func coreConnectBegin(handle uint64, address *byte, addressLen uint32) Status {
	return withPanicGuard(func() Status {
		return clientSessions.beginSessionConnect(handle, func(session *clientSession) Status {
			if addressLen == 0 {
				return StatusInvalidArgument
			}
			if addressLen > MaxConnectionAddressBytes {
				return StatusInvalidArgument
			}
			if address == nil {
				return StatusInvalidArgument
			}
			if uintptr(unsafe.Pointer(address))%uintptr(ABIAlignment) != 0 {
				return StatusInvalidArgument
			}
			text := string(unsafe.Slice(address, int(addressLen)))
			if !utf8.ValidString(text) {
				return StatusInputRejected
			}
			for index := 0; index < len(text); index++ {
				if text[index] < 0x20 || text[index] == 0x7F {
					return StatusInputRejected
				}
			}
			return session.begin(text)
		})
	})
}

// coreConnectPoll reports the current connection phase of one live session.
// It is the testable core behind the exported connect poll symbol.
//
// Validation order: the handle lifecycle first (unknown values report
// `StatusInvalidHandle`, destroyed sessions `StatusInvalidState`), then the
// non-null phase out-pointer (`StatusInvalidArgument`). A live session polls
// `StatusOK` with the phase word while idle, establishing, or online, and
// `StatusDisconnected` with no output byte once the connection reached its
// terminal disconnect; the implied phase is `ConnectPhaseDisconnected`. The
// call is O(1) and never blocks on the network.
func coreConnectPoll(handle uint64, outPhase *uint32) Status {
	return withPanicGuard(func() Status {
		session, status := clientSessions.sessionFor(handle)
		if status != StatusOK {
			return status
		}
		if outPhase == nil {
			return StatusInvalidArgument
		}
		return session.poll(outPhase)
	})
}

// coreDisconnect tears one live session's connection down; see
// `clientSession.disconnect` for the cancel-and-join discipline and the
// idempotency ruling. Handle validation mirrors the other exports: an
// unknown value reports `StatusInvalidHandle`, a destroyed session
// `StatusInvalidState` (destroy already performed the teardown), and every
// live session phase reports `StatusOK`.
func coreDisconnect(handle uint64) Status {
	return withPanicGuard(func() Status {
		session, status := clientSessions.sessionFor(handle)
		if status != StatusOK {
			return status
		}
		return session.disconnect()
	})
}
