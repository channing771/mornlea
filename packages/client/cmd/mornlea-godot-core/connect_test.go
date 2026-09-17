package main

import (
	"context"
	"errors"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"unsafe"

	"github.com/channing771/mornlea/packages/client/presentation"
	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

// The connection tests pin the asynchronous connect surface: connect begin
// returns before establishment completes, the poll reports the phase mapping
// and the terminal disconnect status, and disconnect plus destroy cancel and
// join an in-flight establishment. Transport behavior is scripted through the
// same `runtime.RemoteDependencies` injection the runtime tests use, so no
// test opens a real socket and every wait is channel- or spin-synchronized
// without a wall clock. Swapping the dependency seam is safe only because
// package tests run sequentially.

// connectPollSpinBound bounds the clock-free poll spins: every awaited
// transition is produced by a goroutine that only needs scheduler rounds to
// finish, so an exhausted spin means a real defect, not slow hardware.
const connectPollSpinBound = 1 << 22

// installConnectTransport pins the dependency seam for one test and restores
// the production provider afterwards.
func installConnectTransport(t *testing.T, dependencies clientruntime.RemoteDependencies) {
	t.Helper()
	previous := remoteSessionDependencies
	remoteSessionDependencies = func() clientruntime.RemoteDependencies { return dependencies }
	t.Cleanup(func() { remoteSessionDependencies = previous })
}

// connectPollUntilPhase spins until poll reports the wanted phase with
// `StatusOK`, yielding between attempts so the connect goroutine finishes
// without any sleep.
func connectPollUntilPhase(t *testing.T, handle uint64, want uint32) {
	t.Helper()
	for iteration := 0; iteration < connectPollSpinBound; iteration++ {
		phase := uint32(0xDEADBEEF)
		if status := coreConnectPoll(handle, &phase); status == StatusOK && phase == want {
			return
		}
		goruntime.Gosched()
	}
	t.Fatalf("connect poll never reported phase %d with StatusOK within the bounded spin", want)
}

// connectPollUntilStatus spins until poll reports the wanted status and
// returns the untouched phase word for no-write assertions.
func connectPollUntilStatus(t *testing.T, handle uint64, want Status) uint32 {
	t.Helper()
	for iteration := 0; iteration < connectPollSpinBound; iteration++ {
		phase := uint32(0xDEADBEEF)
		if status := coreConnectPoll(handle, &phase); status == want {
			return phase
		}
		goruntime.Gosched()
	}
	t.Fatalf("connect poll never reported status %d within the bounded spin", want)
	return 0
}

// connectAddressPointer adapts an address string to the raw pointer shape of
// `coreConnectBegin`. The storage is carved from a uint64 array so the
// payload pointer honors the `ABIAlignment` record-buffer rule the way a C
// caller's header-plus-payload buffer does; Go byte slices may come from the
// tiny allocator at byte granularity, which the export rightly rejects. An
// empty address is the null pointer a C caller would pass.
func connectAddressPointer(address string) *byte {
	if address == "" {
		return nil
	}
	storage := make([]uint64, (len(address)+7)/8+1)
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&storage[0])), len(address))
	copy(buffer, address)
	return &buffer[0]
}

// connectTestEndpoint is the fake logged-in endpoint; only close accounting
// matters to the connect surface.
type connectTestEndpoint struct {
	mu        sync.Mutex
	closeCall int
}

func (endpoint *connectTestEndpoint) Send(context.Context, protocol.ClientMessage) error {
	return nil
}

func (endpoint *connectTestEndpoint) Recv(context.Context) (network.ServerMessage, error) {
	return nil, errors.New("unexpected receive")
}

func (endpoint *connectTestEndpoint) Close() error {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	endpoint.closeCall++
	return nil
}

func (endpoint *connectTestEndpoint) closeCalls() int {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	return endpoint.closeCall
}

// connectTestReceiver is the fake bounded receiver; its `Err` flips to a
// terminal error on demand so a poll can observe a receiver-driven
// disconnect.
type connectTestReceiver struct {
	endpoint *connectTestEndpoint
	mu       sync.Mutex
	err      error
	closed   int
}

func (receiver *connectTestReceiver) TryRecv() (network.ServerMessage, bool) { return nil, false }

func (receiver *connectTestReceiver) Err() error {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	return receiver.err
}

func (receiver *connectTestReceiver) setErr(err error) {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	receiver.err = err
}

func (receiver *connectTestReceiver) Close() error {
	receiver.mu.Lock()
	receiver.closed++
	receiver.mu.Unlock()
	// The real receiver owns the endpoint and releases it on close.
	return receiver.endpoint.Close()
}

func (receiver *connectTestReceiver) closeCalls() int {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	return receiver.closed
}

// connectTestPacketStream scripts server packets for the real login state
// machine, mirroring the runtime-test scripted stream.
type connectTestPacketStream struct {
	mu         sync.Mutex
	packets    []protocol.ServerPacket
	sentStates []protocol.State
	closed     int
}

func (stream *connectTestPacketStream) Send(_ context.Context, state protocol.State, _ protocol.ClientPacket) error {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	stream.sentStates = append(stream.sentStates, state)
	return nil
}

func (stream *connectTestPacketStream) Recv(_ context.Context, _ protocol.State) (protocol.ServerPacket, error) {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if len(stream.packets) == 0 {
		return nil, errors.New("unexpected receive")
	}
	packet := stream.packets[0]
	stream.packets = stream.packets[1:]
	return packet, nil
}

func (stream *connectTestPacketStream) Close() error {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	stream.closed++
	return nil
}

func (stream *connectTestPacketStream) closeCalls() int {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return stream.closed
}

// connectTransportScript scripts one establishment attempt: the dial blocks
// until the test releases it or the connect context is canceled, the login
// fake succeeds with the fake endpoint, and the receiver factory returns the
// fake receiver. A non-nil `stream` switches the script to the real login
// state machine consuming scripted packets (for version rejection).
// `loginIgnoresCancel` swaps the login fake for one that parks until the
// connect context is canceled and then succeeds anyway, reproducing an
// establishment that completes despite cancellation (the slip-through
// branch).
type connectTransportScript struct {
	dialEntered        chan struct{}
	dialRelease        chan struct{}
	dialCanceled       chan struct{}
	dialExited         chan struct{}
	loginEntered       chan struct{}
	loginIgnoresCancel bool
	stream             *connectTestPacketStream
	endpoint           *connectTestEndpoint
	receiver           *connectTestReceiver
}

func newConnectTransportScript() *connectTransportScript {
	endpoint := &connectTestEndpoint{}
	return &connectTransportScript{
		dialEntered:  make(chan struct{}),
		dialRelease:  make(chan struct{}),
		dialCanceled: make(chan struct{}),
		dialExited:   make(chan struct{}),
		loginEntered: make(chan struct{}),
		endpoint:     endpoint,
		receiver:     &connectTestReceiver{endpoint: endpoint},
	}
}

// dependencies builds the injected seam value for `runtime.NewRemote`.
func (script *connectTransportScript) dependencies() clientruntime.RemoteDependencies {
	dependencies := clientruntime.RemoteDependencies{
		Dial: func(ctx context.Context, _ string) (network.ClientPacketStream, error) {
			close(script.dialEntered)
			select {
			case <-script.dialRelease:
				close(script.dialExited)
				if script.stream != nil {
					return script.stream, nil
				}
				return &connectTestPacketStream{}, nil
			case <-ctx.Done():
				close(script.dialCanceled)
				close(script.dialExited)
				return nil, ctx.Err()
			}
		},
		NewReceiver: func(network.ClientEndpoint, int) (clientruntime.Receiver, error) {
			return script.receiver, nil
		},
	}
	if script.loginIgnoresCancel {
		dependencies.Login = func(ctx context.Context, _ network.ClientPacketStream, _ network.Identity, _ uint8) (network.ClientEndpoint, uint64, error) {
			close(script.loginEntered)
			// Park until the teardown cancellation, then succeed anyway: the
			// disconnect path publishes the terminal state before it cancels,
			// and the context package guarantees the cancel happens before
			// `Done` is observed closed, so the goroutine can only observe
			// the terminal state when it later publishes this success.
			<-ctx.Done()
			return script.endpoint, 42, nil
		}
		return dependencies
	}
	if script.stream == nil {
		dependencies.Login = func(context.Context, network.ClientPacketStream, network.Identity, uint8) (network.ClientEndpoint, uint64, error) {
			return script.endpoint, 42, nil
		}
	}
	return dependencies
}

// beginConnecting performs a valid connect begin against the scripted
// transport and fails the test on any unexpected status.
func beginConnecting(t *testing.T, handle uint64, address string) {
	t.Helper()
	pointer := connectAddressPointer(address)
	if pointer == nil {
		t.Fatal("test address must be non-empty")
	}
	if status := coreConnectBegin(handle, pointer, uint32(len(address))); status != StatusOK {
		t.Fatalf("connect begin of %q = %d, want StatusOK", address, status)
	}
}

// TestConnectPollOnIdleSessionReportsNotReadyPhase pins the pre-connect
// ruling: a live session that never began connecting polls `StatusOK` with
// the not-ready phase word.
func TestConnectPollOnIdleSessionReportsNotReadyPhase(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(handle, &phase); status != StatusOK {
		t.Fatalf("poll on idle session = %d, want StatusOK", status)
	}
	if phase != 0 {
		t.Fatalf("idle phase = %d, want the pinned literal 0 (not-ready)", phase)
	}
}

// TestConnectBeginReturnsBeforeEstablishmentAndPollWalksToLoading proves the
// core async contract: begin returns while the dial is still blocked, poll
// reports connecting, releasing the dial walks the session to loading without
// blocking any poll, disconnect tears the receiver down exactly once, and the
// terminal poll reports the disconnect status without writing the phase word.
func TestConnectBeginReturnsBeforeEstablishmentAndPollWalksToLoading(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())

	beginConnecting(t, handle, "127.0.0.1:25565")
	<-script.dialEntered
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(handle, &phase); status != StatusOK || phase != ConnectPhaseConnecting {
		t.Fatalf("poll during blocked dial = (%d, %d), want StatusOK and connecting", status, phase)
	}
	if phase != 1 {
		t.Fatalf("connecting phase = %d, want the pinned literal 1", phase)
	}

	close(script.dialRelease)
	connectPollUntilPhase(t, handle, ConnectPhaseLoading)
	if got := ConnectPhaseLoading; got != 3 {
		t.Fatalf("loading phase = %d, want the pinned literal 3", got)
	}

	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("disconnect of an online session = %d, want StatusOK", status)
	}
	phase = uint32(0xDEADBEEF)
	if status := coreConnectPoll(handle, &phase); status != StatusDisconnected {
		t.Fatalf("poll after disconnect = %d, want StatusDisconnected", status)
	}
	if phase != 0xDEADBEEF {
		t.Fatalf("terminal poll wrote the phase word %d despite the non-OK status", phase)
	}
	if calls := script.receiver.closeCalls(); calls != 1 {
		t.Fatalf("receiver close calls after disconnect = %d, want 1", calls)
	}
	if calls := script.endpoint.closeCalls(); calls != 1 {
		t.Fatalf("endpoint close calls after disconnect = %d, want 1", calls)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after disconnect = %d, want StatusOK", status)
	}
}

// TestConnectBeginRejectsRepeatedConnectInEveryBegunPhase pins the
// repeated-connect ruling: after any begun connection (establishing, online,
// or terminal) a second begin on the same session reports
// `StatusInvalidState`; a retry creates a new session.
func TestConnectBeginRejectsRepeatedConnectInEveryBegunPhase(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())
	address := "127.0.0.1:25565"

	beginConnecting(t, handle, address)
	<-script.dialEntered
	if status := coreConnectBegin(handle, connectAddressPointer(address), uint32(len(address))); status != StatusInvalidState {
		t.Fatalf("repeated begin while establishing = %d, want StatusInvalidState", status)
	}

	close(script.dialRelease)
	connectPollUntilPhase(t, handle, ConnectPhaseLoading)
	if status := coreConnectBegin(handle, connectAddressPointer(address), uint32(len(address))); status != StatusInvalidState {
		t.Fatalf("repeated begin while online = %d, want StatusInvalidState", status)
	}

	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("disconnect = %d, want StatusOK", status)
	}
	connectPollUntilStatus(t, handle, StatusDisconnected)
	if status := coreConnectBegin(handle, connectAddressPointer(address), uint32(len(address))); status != StatusInvalidState {
		t.Fatalf("repeated begin after terminal = %d, want StatusInvalidState", status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy = %d, want StatusOK", status)
	}
}

// TestConnectBeginValidatesHandleBeforePointerShapeAndContent proves the
// validation order of `coreConnectBegin`: an unknown or destroyed handle wins
// over every argument defect, then the pointer and length shape is judged,
// and none of the rejections starts a connection.
func TestConnectBeginValidatesHandleBeforePointerShapeAndContent(t *testing.T) {
	resetSessionTable(t)
	live := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}

	if status := coreConnectBegin(0, nil, 0); status != StatusInvalidHandle {
		t.Fatalf("begin on zero handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreConnectBegin(makeSessionHandle(3, 1), nil, 0); status != StatusInvalidHandle {
		t.Fatalf("begin on never-issued handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreConnectBegin(destroyed, nil, 16); status != StatusInvalidState {
		t.Fatalf("begin on destroyed handle with null address = %d, want StatusInvalidState", status)
	}

	if status := coreConnectBegin(live, nil, 16); status != StatusInvalidArgument {
		t.Fatalf("begin with null address and positive length = %d, want StatusInvalidArgument", status)
	}
	if status := coreConnectBegin(live, connectAddressPointer("127.0.0.1:25565"), 0); status != StatusInvalidArgument {
		t.Fatalf("begin with zero length = %d, want StatusInvalidArgument", status)
	}
	oversized := strings.Repeat("a", int(MaxConnectionAddressBytes)+1)
	if status := coreConnectBegin(live, connectAddressPointer(oversized), uint32(len(oversized))); status != StatusInvalidArgument {
		t.Fatalf("begin with length %d = %d, want StatusInvalidArgument", len(oversized), status)
	}
	storage := make([]byte, len(oversized)+8)
	copy(storage[1:], oversized)
	misaligned := (*byte)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), 1)))
	if status := coreConnectBegin(live, misaligned, 16); status != StatusInvalidArgument {
		t.Fatalf("begin with misaligned address = %d, want StatusInvalidArgument", status)
	}

	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(live, &phase); status != StatusOK || phase != ConnectPhaseNotReady {
		t.Fatalf("poll after rejected begins = (%d, %d), want StatusOK and not-ready: no rejection may start a connection",
			status, phase)
	}
	if live := clientSessions.live(); live != 1 {
		t.Fatalf("table holds %d live sessions after rejections, want 1", live)
	}
}

// TestConnectBeginRejectsInvalidAddressContentAndLeavesSessionIdle pins the
// content ruling: the address must be valid UTF-8 without NUL or ASCII
// control bytes; rejections are `StatusInputRejected` and leave the session
// idle, and the full 256-byte limit remains acceptable.
func TestConnectBeginRejectsInvalidAddressContentAndLeavesSessionIdle(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())

	for index, address := range []string{
		"\xff\xfe",
		"127.0.0.1:\x0025565",
		"127.0.0.1\n:25565",
		"127.0.0.1:\x7f25565",
	} {
		pointer := connectAddressPointer(address)
		if status := coreConnectBegin(handle, pointer, uint32(len(address))); status != StatusInputRejected {
			t.Fatalf("begin with content case %d (%q) = %d, want StatusInputRejected", index, address, status)
		}
	}
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(handle, &phase); status != StatusOK || phase != ConnectPhaseNotReady {
		t.Fatalf("poll after content rejections = (%d, %d), want StatusOK and not-ready", status, phase)
	}

	limitAddress := strings.Repeat("a", int(MaxConnectionAddressBytes))
	beginConnecting(t, handle, limitAddress)
	<-script.dialEntered
	close(script.dialRelease)
	connectPollUntilPhase(t, handle, ConnectPhaseLoading)
	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("disconnect = %d, want StatusOK", status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy = %d, want StatusOK", status)
	}
}

// TestConnectDisconnectCancelsInFlightDialAndJoinsGoroutine proves the
// cancellation path: disconnect during a blocked dial cancels the connect
// context, the dial observes the cancellation, disconnect returns only after
// the connect goroutine finished, and the session polls disconnected.
func TestConnectDisconnectCancelsInFlightDialAndJoinsGoroutine(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())

	beginConnecting(t, handle, "127.0.0.1:25565")
	<-script.dialEntered
	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("disconnect during establishing = %d, want StatusOK", status)
	}
	<-script.dialCanceled
	<-script.dialExited
	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("repeated disconnect = %d, want StatusOK", status)
	}
	connectPollUntilStatus(t, handle, StatusDisconnected)
	if calls := script.receiver.closeCalls(); calls != 0 {
		t.Fatalf("receiver close calls after a canceled dial = %d, want 0", calls)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after disconnect = %d, want StatusOK", status)
	}
}

// TestConnectDisconnectDuringLoginClosesSlippedThroughRuntimeOnce pins the
// slip-through branch deterministically: an establishment that succeeds even
// though the disconnect cancellation lands during login publishes no runtime
// (teardown already claimed the terminal state), so the connect goroutine
// itself owns the release and closes the receiver and endpoint exactly once
// — never both the goroutine and the disconnect path.
func TestConnectDisconnectDuringLoginClosesSlippedThroughRuntimeOnce(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	script.loginIgnoresCancel = true
	installConnectTransport(t, script.dependencies())

	beginConnecting(t, handle, "127.0.0.1:25565")
	<-script.dialEntered
	close(script.dialRelease)
	<-script.loginEntered
	// The login fake returns success only after this disconnect published the
	// terminal state and canceled the context, and disconnect joins the
	// goroutine, so a returned disconnect implies the slip-through close ran.
	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("disconnect during login = %d, want StatusOK", status)
	}

	if calls := script.receiver.closeCalls(); calls != 1 {
		t.Fatalf("receiver close calls after the slip-through = %d, want exactly 1 (single owner, no double close)", calls)
	}
	if calls := script.endpoint.closeCalls(); calls != 1 {
		t.Fatalf("endpoint close calls after the slip-through = %d, want exactly 1", calls)
	}
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(handle, &phase); status != StatusDisconnected {
		t.Fatalf("poll after the slip-through = %d, want StatusDisconnected", status)
	}
	if phase != 0xDEADBEEF {
		t.Fatalf("terminal poll wrote the phase word %d despite the non-OK status", phase)
	}
	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("repeated disconnect after the slip-through = %d, want StatusOK", status)
	}
	if calls := script.receiver.closeCalls(); calls != 1 {
		t.Fatalf("receiver close calls after repeated disconnect = %d, want still exactly 1", calls)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the slip-through = %d, want StatusOK", status)
	}
}

// TestConnectDisconnectValidatesHandlesAndIsIdempotent pins the disconnect
// handle mapping: disconnect of a never-issued handle is
// `StatusInvalidHandle`, of a destroyed session `StatusInvalidState`, and of
// a live session (idle, establishing, online, or terminal) `StatusOK`
// repeatedly.
func TestConnectDisconnectValidatesHandlesAndIsIdempotent(t *testing.T) {
	resetSessionTable(t)
	if status := coreDisconnect(0); status != StatusInvalidHandle {
		t.Fatalf("disconnect of zero handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreDisconnect(makeSessionHandle(5, 1<<40)); status != StatusInvalidHandle {
		t.Fatalf("disconnect of never-issued handle = %d, want StatusInvalidHandle", status)
	}

	idle := createSessionOK(t)
	if status := coreDisconnect(idle); status != StatusOK {
		t.Fatalf("disconnect of idle session = %d, want StatusOK", status)
	}
	if status := coreDisconnect(idle); status != StatusOK {
		t.Fatalf("repeated disconnect of idle session = %d, want StatusOK", status)
	}
	if status := coreDestroy(idle); status != StatusOK {
		t.Fatalf("destroy of idle session = %d, want StatusOK", status)
	}
	if status := coreDisconnect(idle); status != StatusInvalidState {
		t.Fatalf("disconnect of destroyed session = %d, want StatusInvalidState", status)
	}
}

// TestConnectVersionRejectionReachesPollAsDisconnectedTerminal proves the
// v44 rejection path: a server that rejects the handshake with a newer
// protocol version drives the real login state machine to a remote error,
// the poll reports the terminal disconnect status, the session records the
// remote rejection as its terminal cause, and the login closes the stream
// exactly once.
func TestConnectVersionRejectionReachesPollAsDisconnectedTerminal(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	script.stream = &connectTestPacketStream{packets: []protocol.ServerPacket{
		protocol.HandshakeReject{
			ServerProtocolVersion: protocol.ProtocolVersion + 1,
			Code:                  protocol.HandshakeVersionMismatch,
			Message:               "version mismatch",
		},
	}}
	installConnectTransport(t, script.dependencies())

	beginConnecting(t, handle, "127.0.0.1:25565")
	<-script.dialEntered
	close(script.dialRelease)
	connectPollUntilStatus(t, handle, StatusDisconnected)

	session, status := clientSessions.sessionFor(handle)
	if status != StatusOK {
		t.Fatalf("session lookup after rejection = %d, want StatusOK", status)
	}
	var remoteErr *network.RemoteError
	if err := session.terminalError(); !errors.As(err, &remoteErr) {
		t.Fatalf("terminal error = %v, want a network remote rejection", err)
	}
	if remoteErr.State != protocol.StateHandshake || remoteErr.Code != uint8(protocol.HandshakeVersionMismatch) {
		t.Fatalf("remote rejection = %+v, want handshake version mismatch", remoteErr)
	}
	if calls := script.stream.closeCalls(); calls != 1 {
		t.Fatalf("stream close calls after login rejection = %d, want 1 (login-owned close)", calls)
	}
	retryAddress := "127.0.0.1:25565"
	if status := coreConnectBegin(handle, connectAddressPointer(retryAddress), uint32(len(retryAddress))); status != StatusInvalidState {
		t.Fatalf("begin after terminal rejection = %d, want StatusInvalidState", status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after terminal rejection = %d, want StatusOK", status)
	}
}

// TestConnectDestroyDuringDialCancelsAndJoinsWithoutLeak pins the close
// race: destroying a session with a blocked dial retires the slot, cancels
// the connect context, joins the connect goroutine before returning, and the
// destroyed handle answers later polls with `StatusInvalidState`.
func TestConnectDestroyDuringDialCancelsAndJoinsWithoutLeak(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())

	beginConnecting(t, handle, "127.0.0.1:25565")
	<-script.dialEntered
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy during establishing = %d, want StatusOK", status)
	}
	<-script.dialCanceled
	<-script.dialExited
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(handle, &phase); status != StatusInvalidState {
		t.Fatalf("poll on destroyed handle = %d, want StatusInvalidState", status)
	}
	if phase != 0xDEADBEEF {
		t.Fatalf("poll on destroyed handle wrote the phase word %d despite failure", phase)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("repeated destroy = %d, want StatusOK", status)
	}
	if live := clientSessions.live(); live != 0 {
		t.Fatalf("table holds %d live sessions after destroy, want 0", live)
	}
}

// TestConnectPollValidatesHandleAndOutPointerWithoutWrites pins the poll
// argument contract: the handle lifecycle precedes the out-pointer check and
// no failure path writes the phase word.
func TestConnectPollValidatesHandleAndOutPointerWithoutWrites(t *testing.T) {
	resetSessionTable(t)
	live := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}

	if status := coreConnectPoll(0, nil); status != StatusInvalidHandle {
		t.Fatalf("poll on zero handle with null out = %d, want StatusInvalidHandle", status)
	}
	if status := coreConnectPoll(destroyed, nil); status != StatusInvalidState {
		t.Fatalf("poll on destroyed handle with null out = %d, want StatusInvalidState", status)
	}
	if status := coreConnectPoll(live, nil); status != StatusInvalidArgument {
		t.Fatalf("poll on live handle with null out = %d, want StatusInvalidArgument", status)
	}
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(destroyed, &phase); status != StatusInvalidState {
		t.Fatalf("poll on destroyed handle = %d, want StatusInvalidState", status)
	}
	if phase != 0xDEADBEEF {
		t.Fatalf("failed poll wrote the phase word %d", phase)
	}
}

// TestConnectConcurrentPollAndDisconnectStayWithinStatusVocabulary drives
// poll from several goroutines during establishment and through a racing
// disconnect: every observed result stays inside the documented vocabulary
// and no call panics or blocks forever.
func TestConnectConcurrentPollAndDisconnectStayWithinStatusVocabulary(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())

	beginConnecting(t, handle, "127.0.0.1:25565")
	<-script.dialEntered

	const pollers = 6
	const iterations = 200
	var group sync.WaitGroup
	stop := make(chan struct{})
	for worker := 0; worker < pollers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				phase := uint32(0xDEADBEEF)
				switch status := coreConnectPoll(handle, &phase); status {
				case StatusOK:
					if phase != ConnectPhaseConnecting && phase != ConnectPhaseLoading {
						t.Errorf("poll observed phase %d, want connecting or loading", phase)
					}
				case StatusDisconnected:
				default:
					t.Errorf("poll = %d, want StatusOK or StatusDisconnected", status)
				}
				select {
				case <-stop:
					return
				default:
				}
			}
		}()
	}
	close(script.dialRelease)
	group.Add(1)
	go func() {
		defer group.Done()
		if status := coreDisconnect(handle); status != StatusOK {
			t.Errorf("racing disconnect = %d, want StatusOK", status)
		}
	}()
	group.Wait()
	close(stop)
	connectPollUntilStatus(t, handle, StatusDisconnected)
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after racing disconnect = %d, want StatusOK", status)
	}
}

// TestConnectReceiverTerminalErrorTransitionsPollToDisconnected proves the
// online delegation: a receiver that reports a terminal error flips the
// runtime to disconnected, the poll transitions the session to terminal, and
// the receiver and endpoint are closed exactly once.
func TestConnectReceiverTerminalErrorTransitionsPollToDisconnected(t *testing.T) {
	resetSessionTable(t)
	handle := createSessionOK(t)
	script := newConnectTransportScript()
	installConnectTransport(t, script.dependencies())

	beginConnecting(t, handle, "127.0.0.1:25565")
	<-script.dialEntered
	close(script.dialRelease)
	connectPollUntilPhase(t, handle, ConnectPhaseLoading)

	terminalErr := errors.New("receiver terminal error")
	script.receiver.setErr(terminalErr)
	connectPollUntilStatus(t, handle, StatusDisconnected)

	session, status := clientSessions.sessionFor(handle)
	if status != StatusOK {
		t.Fatalf("session lookup after receiver death = %d, want StatusOK", status)
	}
	if err := session.terminalError(); !errors.Is(err, terminalErr) {
		t.Fatalf("terminal error = %v, want the receiver terminal error", err)
	}
	if calls := script.receiver.closeCalls(); calls != 1 {
		t.Fatalf("receiver close calls after terminal error = %d, want 1", calls)
	}
	if calls := script.endpoint.closeCalls(); calls != 1 {
		t.Fatalf("endpoint close calls after terminal error = %d, want 1", calls)
	}
	if status := coreDisconnect(handle); status != StatusOK {
		t.Fatalf("disconnect after receiver death = %d, want StatusOK", status)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after receiver death = %d, want StatusOK", status)
	}
}

// TestConnectPhaseMappingPinsPresentationSessionPhaseValues pins the phase
// mapping ruling: the producer-side poll vocabulary is the identity cast of
// the presentation session phases, stable at the pinned literals 0 through 5.
func TestConnectPhaseMappingPinsPresentationSessionPhaseValues(t *testing.T) {
	cases := []struct {
		value uint32
		want  uint32
		phase presentation.SessionPhase
	}{
		{ConnectPhaseNotReady, 0, presentation.SessionPhaseNotReady},
		{ConnectPhaseConnecting, 1, presentation.SessionPhaseConnecting},
		{ConnectPhaseLogin, 2, presentation.SessionPhaseLogin},
		{ConnectPhaseLoading, 3, presentation.SessionPhaseLoading},
		{ConnectPhasePlay, 4, presentation.SessionPhasePlay},
		{ConnectPhaseDisconnected, 5, presentation.SessionPhaseDisconnected},
	}
	for _, testCase := range cases {
		if testCase.value != testCase.want || uint32(testCase.phase) != testCase.want {
			t.Fatalf("phase mapping = (%d, presentation %d), want the pinned literal %d",
				testCase.value, testCase.phase, testCase.want)
		}
	}
}

// TestConnectPilotLoginConfigurationIsValid pins the pilot login inputs the
// connection family sends with every establishment: a valid UUIDv4 identity,
// a display name inside the login domain, a view distance inside the
// negotiated login range, and a positive receiver capacity.
func TestConnectPilotLoginConfigurationIsValid(t *testing.T) {
	identity, err := pilotConnectIdentity()
	if err != nil {
		t.Fatalf("pilot identity error = %v, want none", err)
	}
	if !identity.PlayerID.Valid() {
		t.Fatalf("pilot player ID %s is not a valid UUIDv4", identity.PlayerID)
	}
	if identity.DisplayName != pilotConnectDisplayName {
		t.Fatalf("pilot display name = %q, want %q", identity.DisplayName, pilotConnectDisplayName)
	}
	if pilotConnectViewDistance < protocol.LoginViewDistanceMin || pilotConnectViewDistance > protocol.LoginViewDistanceMax {
		t.Fatalf("pilot view distance = %d, want inside %d..%d",
			pilotConnectViewDistance, protocol.LoginViewDistanceMin, protocol.LoginViewDistanceMax)
	}
	if pilotConnectReceiverCapacity < 1 {
		t.Fatalf("pilot receiver capacity = %d, want positive", pilotConnectReceiverCapacity)
	}
	if _, err := core.ParsePlayerID(pilotConnectPlayerIDText); err != nil {
		t.Fatalf("pilot player ID literal %q does not parse: %v", pilotConnectPlayerIDText, err)
	}
}

// TestConnectPilotMeshOptionsAreValid pins the meshing inputs every online
// pilot session assembles with: the world family publishes section batches
// only through a runtime-owned mesher, so the options must validate against
// the runtime contract (registry present, positive worker count, ready
// capacity inside the frozen world-batch bound, positive atlas revision).
func TestConnectPilotMeshOptionsAreValid(t *testing.T) {
	options := pilotMeshOptions()
	if options == nil {
		t.Fatal("pilot mesh options are missing")
	}
	if options.Registry == nil {
		t.Fatal("pilot mesh options carry no block registry")
	}
	if err := options.Validate(); err != nil {
		t.Fatalf("pilot mesh options are invalid: %v", err)
	}
	if options.Workers < 1 {
		t.Fatalf("pilot mesh workers = %d, want positive", options.Workers)
	}
	if options.ReadyCapacity != presentation.MaxWorldBatchOperations {
		t.Fatalf("pilot mesh ready capacity = %d, want the frozen maximal batch %d",
			options.ReadyCapacity, presentation.MaxWorldBatchOperations)
	}
	if options.AtlasRevision == 0 {
		t.Fatal("pilot mesh atlas revision must be positive")
	}
}
