package main

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	goruntime "runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/presentation"
	clientruntime "github.com/channing771/mornlea/packages/client/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// The step tests pin the step family surface: the frozen MCS1 request decoded
// from an owned copy, the validation order from handle lifecycle through
// session state, pointer shape, and record identity/content to exactly one
// bounded `runtime.Runtime.Step` call, the step-result retention ruling (the
// latest completed step result stays on the session for the world/frame pull
// exports), the receiver-failure mapping, and the no-network-wait contract.
// Online sessions come from the same scripted transport seam the connect tests
// use, so no test opens a socket; the `stepRuntime` seam records arguments and
// scripts failures and panics while defaulting to the real bounded step.

// stepReadyStateMessage is the authoritative ready-state message that makes
// the predictor ready and the phase playable, mirroring the runtime package's
// own step fixture.
func stepReadyStateMessage(tick uint64) network.PlayerState {
	return network.PlayerState{
		ServerTick: tick, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true, Health: 15, Hunger: 12, Oxygen: 240,
		DayPhaseOffset: 321, WorldTimeTicks: 98_765, WeatherKind: core.WeatherRain,
		Season: core.SeasonAutumn, SeasonProgress: 91, Temperature: -7,
	}
}

// stepScriptEndpoint is the fake logged-in endpoint. Its send can be switched
// to block until the caller's context is canceled, mirroring the runtime
// package's blocking step sender so the export can prove a step never waits
// for a wedged transport write.
type stepScriptEndpoint struct {
	mu        sync.Mutex
	blocking  bool
	started   int
	completed int
	closed    int
	notify    chan struct{}
}

func newStepScriptEndpoint() *stepScriptEndpoint {
	return &stepScriptEndpoint{notify: make(chan struct{}, 1)}
}

func (endpoint *stepScriptEndpoint) Send(ctx context.Context, _ protocol.ClientMessage) error {
	endpoint.mu.Lock()
	if !endpoint.blocking {
		endpoint.mu.Unlock()
		return nil
	}
	endpoint.started++
	endpoint.mu.Unlock()
	select {
	case endpoint.notify <- struct{}{}:
	default:
	}
	<-ctx.Done()
	endpoint.mu.Lock()
	endpoint.completed++
	endpoint.mu.Unlock()
	return ctx.Err()
}

func (endpoint *stepScriptEndpoint) Recv(context.Context) (network.ServerMessage, error) {
	return nil, errors.New("unexpected receive")
}

func (endpoint *stepScriptEndpoint) Close() error {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	endpoint.closed++
	return nil
}

func (endpoint *stepScriptEndpoint) setBlocking(blocking bool) {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	endpoint.blocking = blocking
}

func (endpoint *stepScriptEndpoint) startedCount() int {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	return endpoint.started
}

func (endpoint *stepScriptEndpoint) closeCalls() int {
	endpoint.mu.Lock()
	defer endpoint.mu.Unlock()
	return endpoint.closed
}

func (endpoint *stepScriptEndpoint) waitStarted(timeout time.Duration) bool {
	if endpoint.startedCount() > 0 {
		return true
	}
	select {
	case <-endpoint.notify:
		return true
	case <-time.After(timeout):
		return false
	}
}

// stepScriptReceiver serves scripted server messages and can flip to a
// terminal receiver error mid-session; it owns the endpoint like the real
// bounded receiver.
type stepScriptReceiver struct {
	endpoint *stepScriptEndpoint
	mu       sync.Mutex
	messages []network.ServerMessage
	err      error
	closed   int
}

func newStepScriptReceiver(endpoint *stepScriptEndpoint, messages ...network.ServerMessage) *stepScriptReceiver {
	return &stepScriptReceiver{endpoint: endpoint, messages: append([]network.ServerMessage(nil), messages...)}
}

func (receiver *stepScriptReceiver) TryRecv() (network.ServerMessage, bool) {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	if len(receiver.messages) == 0 {
		return nil, false
	}
	message := receiver.messages[0]
	receiver.messages = receiver.messages[1:]
	return message, true
}

func (receiver *stepScriptReceiver) Err() error {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	return receiver.err
}

func (receiver *stepScriptReceiver) setErr(err error) {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	receiver.err = err
}

func (receiver *stepScriptReceiver) Close() error {
	receiver.mu.Lock()
	receiver.closed++
	receiver.mu.Unlock()
	// The real receiver owns the endpoint and releases it on close.
	return receiver.endpoint.Close()
}

func (receiver *stepScriptReceiver) closeCalls() int {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	return receiver.closed
}

// stepTransportScript completes dial and login immediately and hands the
// session the scripted receiver and endpoint.
type stepTransportScript struct {
	endpoint *stepScriptEndpoint
	receiver *stepScriptReceiver
}

func newStepTransportScript(messages ...network.ServerMessage) *stepTransportScript {
	endpoint := newStepScriptEndpoint()
	return &stepTransportScript{endpoint: endpoint, receiver: newStepScriptReceiver(endpoint, messages...)}
}

func (script *stepTransportScript) dependencies() clientruntime.RemoteDependencies {
	return clientruntime.RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) {
			return &connectTestPacketStream{}, nil
		},
		Login: func(context.Context, network.ClientPacketStream, network.Identity, uint8) (network.ClientEndpoint, uint64, error) {
			return script.endpoint, 42, nil
		},
		NewReceiver: func(network.ClientEndpoint, int) (clientruntime.Receiver, error) {
			return script.receiver, nil
		},
	}
}

// connectSessionOnlineStep installs the step transport script and drives one
// session to the online phase, returning its handle.
func connectSessionOnlineStep(t *testing.T, script *stepTransportScript) uint64 {
	t.Helper()
	resetSessionTable(t)
	handle := createSessionOK(t)
	installConnectTransport(t, script.dependencies())
	beginConnecting(t, handle, "127.0.0.1:25565")
	connectPollUntilPhase(t, handle, ConnectPhaseLoading)
	return handle
}

// stepRequestPointer encodes one well-formed step request into fresh
// 8-byte-aligned storage, as a C caller's fixed request record would be
// aligned.
func stepRequestPointer(elapsed uint64, messageBudget, meshBudget uint32) (*byte, uint32) {
	storage := make([]uint64, 4)
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&storage[0])), int(StepRequestBytes))
	binary.LittleEndian.PutUint32(buffer[0:4], uint32(MagicStep))
	binary.LittleEndian.PutUint32(buffer[4:8], StepVersion)
	binary.LittleEndian.PutUint64(buffer[8:16], elapsed)
	binary.LittleEndian.PutUint32(buffer[16:20], messageBudget)
	binary.LittleEndian.PutUint32(buffer[20:24], meshBudget)
	return &buffer[0], StepRequestBytes
}

// patchStepWord overwrites one little-endian 32-bit word of a built request so
// a test can corrupt exactly one record field.
func patchStepWord(buffer *byte, offset int, value uint32) {
	view := unsafe.Slice(buffer, offset+4)
	binary.LittleEndian.PutUint32(view[offset:offset+4], value)
}

// stepCall is one recorded step execution's arguments.
type stepCall struct {
	elapsed       time.Duration
	messageBudget int
	meshBudget    int
}

// stepCallLog records every execution that reached the step seam.
type stepCallLog struct {
	mu    sync.Mutex
	calls []stepCall
}

func (log *stepCallLog) record(call stepCall) {
	log.mu.Lock()
	defer log.mu.Unlock()
	log.calls = append(log.calls, call)
}

func (log *stepCallLog) count() int {
	log.mu.Lock()
	defer log.mu.Unlock()
	return len(log.calls)
}

func (log *stepCallLog) recorded() []stepCall {
	log.mu.Lock()
	defer log.mu.Unlock()
	return append([]stepCall(nil), log.calls...)
}

// errStepSeamScripted distinguishes the scripted seam failure from real
// runtime step errors in the mapping tests.
var errStepSeamScripted = errors.New("step seam: scripted failure")

// installStepSeam replaces `stepRuntime` for one test with a recording body
// and restores the production execution afterwards; package tests run
// sequentially, so the swap is race-free. A nil `body` delegates to the real
// bounded step.
func installStepSeam(t *testing.T, body func(established *clientruntime.Runtime, elapsed time.Duration, messageBudget, meshBudget int) error) *stepCallLog {
	t.Helper()
	log := &stepCallLog{}
	previous := stepRuntime
	stepRuntime = func(established *clientruntime.Runtime, elapsed time.Duration, messageBudget, meshBudget int) (clientruntime.StepResult, error) {
		log.record(stepCall{elapsed: elapsed, messageBudget: messageBudget, meshBudget: meshBudget})
		if body != nil {
			if err := body(established, elapsed, messageBudget, meshBudget); err != nil {
				return clientruntime.StepResult{}, err
			}
		}
		return established.Step(elapsed, messageBudget, meshBudget)
	}
	t.Cleanup(func() { stepRuntime = previous })
	return log
}

// sessionRetainedStep reads the session's retained step result for retention
// assertions.
func sessionRetainedStep(t *testing.T, handle uint64) (clientruntime.StepResult, bool) {
	t.Helper()
	session, status := clientSessions.sessionFor(handle)
	if status != StatusOK {
		t.Fatalf("session lookup = %d, want StatusOK", status)
	}
	return session.retainedStep()
}

// TestStepZeroSizeAndBoundaryBudgetsAdvanceRuntimeOnce pins the zero-size
// ruling: elapsed zero with both budgets zero is one valid step that drains
// nothing and advances no prediction but still publishes one frame, the exact
// budget boundary values stay valid, every call performs exactly one runtime
// step with the decoded arguments, and each later step overwrites the retained
// result so the pull families always read the latest completed step.
func TestStepZeroSizeAndBoundaryBudgetsAdvanceRuntimeOnce(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	log := installStepSeam(t, nil)

	pointer, length := stepRequestPointer(0, 0, 0)
	if status := coreStep(handle, pointer, length); status != StatusOK {
		t.Fatalf("zero-size step = %d, want StatusOK", status)
	}
	retained, ok := sessionRetainedStep(t, handle)
	if !ok {
		t.Fatal("zero-size step retained no result")
	}
	if retained.Frame.Revision != 1 || retained.Frame.Phase != clientruntime.ConnectionPhaseLoading {
		t.Fatalf("zero-size retained frame = revision %d phase %v, want 1 and loading", retained.Frame.Revision, retained.Frame.Phase)
	}
	if retained.MessagesProcessed != 0 || retained.HasWorld {
		t.Fatalf("zero-size step = processed %d hasWorld %v, want 0 and false", retained.MessagesProcessed, retained.HasWorld)
	}
	if retained.Frame.HUD.Ready {
		t.Fatal("zero-size step fabricated a ready HUD from no message")
	}

	boundaryPointer, boundaryLength := stepRequestPointer(0, MaxStepMessageBudget, MaxStepMeshBudget)
	if status := coreStep(handle, boundaryPointer, boundaryLength); status != StatusOK {
		t.Fatalf("boundary-budget step = %d, want StatusOK", status)
	}
	retained, ok = sessionRetainedStep(t, handle)
	if !ok || retained.Frame.Revision != 2 {
		t.Fatalf("retained result after the boundary step = (%+v, %v), want revision 2 (latest completed step)", retained, ok)
	}

	calls := log.recorded()
	if len(calls) != 2 {
		t.Fatalf("two accepted steps executed %d runtime steps, want 2", len(calls))
	}
	if calls[0] != (stepCall{elapsed: 0, messageBudget: 0, meshBudget: 0}) ||
		calls[1] != (stepCall{elapsed: 0, messageBudget: int(MaxStepMessageBudget), meshBudget: int(MaxStepMeshBudget)}) {
		t.Fatalf("decoded step arguments = %+v, want the wire values passed through unchanged", calls)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after steps = %d, want StatusOK", status)
	}
}

// TestStepRejectsOverBudgetAndUnrepresentableElapsedWithoutStateChange pins
// the content domain: a message or mesh budget above the frozen limits and an
// elapsed value above the maximal representable int64 nanosecond count
// (unrepresentable as a non-negative Go duration) each reject the request
// with `StatusInputRejected` before the runtime is called, no result is
// retained, and the representable maximum elapsed stays valid, proving the
// bound is exactly representability.
func TestStepRejectsOverBudgetAndUnrepresentableElapsedWithoutStateChange(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	log := installStepSeam(t, nil)

	cases := []struct {
		name         string
		elapsed      uint64
		messageValue uint32
		meshValue    uint32
	}{
		{"message budget over limit", 0, MaxStepMessageBudget + 1, 0},
		{"mesh budget over limit", 0, 0, MaxStepMeshBudget + 1},
		{"first unrepresentable elapsed", uint64(1) << 63, 0, 0},
		{"maximal word elapsed", math.MaxUint64, 0, 0},
	}
	for _, testCase := range cases {
		pointer, length := stepRequestPointer(testCase.elapsed, testCase.messageValue, testCase.meshValue)
		if status := coreStep(handle, pointer, length); status != StatusInputRejected {
			t.Fatalf("step with %s = %d, want StatusInputRejected", testCase.name, status)
		}
	}
	if count := log.count(); count != 0 {
		t.Fatalf("rejected requests executed %d runtime steps, want 0", count)
	}
	if _, ok := sessionRetainedStep(t, handle); ok {
		t.Fatal("rejected request retained a step result")
	}

	pointer, length := stepRequestPointer(0, 0, 0)
	if status := coreStep(handle, pointer, length); status != StatusOK {
		t.Fatalf("first accepted step after rejections = %d, want StatusOK", status)
	}
	retained, _ := sessionRetainedStep(t, handle)
	if retained.Frame.Revision != 1 {
		t.Fatalf("first accepted step revision = %d, want 1 (rejections consumed no step)", retained.Frame.Revision)
	}

	maximumPointer, maximumLength := stepRequestPointer(math.MaxInt64, 1, 1)
	if status := coreStep(handle, maximumPointer, maximumLength); status != StatusOK {
		t.Fatalf("step with the representable maximum elapsed = %d, want StatusOK", status)
	}
	retained, _ = sessionRetainedStep(t, handle)
	if retained.Frame.Revision != 2 {
		t.Fatalf("maximum-elapsed step revision = %d, want 2", retained.Frame.Revision)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after content rejections = %d, want StatusOK", status)
	}
}

// TestStepValidatesHandleAndStateBeforeShapeAndContent pins the validation
// order: an unknown or destroyed handle outranks every argument defect, the
// session state (idle, connecting, terminal) outranks the buffer shape, and
// only an online session judges the pointer shape (wrong length, null,
// misaligned) as `StatusInvalidArgument`.
func TestStepValidatesHandleAndStateBeforeShapeAndContent(t *testing.T) {
	resetSessionTable(t)
	idle := createSessionOK(t)
	destroyed := createSessionOK(t)
	if status := coreDestroy(destroyed); status != StatusOK {
		t.Fatalf("setup destroy = %d, want StatusOK", status)
	}
	log := installStepSeam(t, nil)
	validPointer, validLength := stepRequestPointer(0, 1, 1)

	if status := coreStep(0, nil, 0); status != StatusInvalidHandle {
		t.Fatalf("step on the zero handle with a null request = %d, want StatusInvalidHandle", status)
	}
	if status := coreStep(makeSessionHandle(3, 1), nil, 0); status != StatusInvalidHandle {
		t.Fatalf("step on a never-issued handle = %d, want StatusInvalidHandle", status)
	}
	if status := coreStep(destroyed, nil, 0); status != StatusInvalidState {
		t.Fatalf("step on a destroyed handle = %d, want StatusInvalidState", status)
	}
	if status := coreStep(idle, validPointer, validLength); status != StatusInvalidState {
		t.Fatalf("step on an idle session with a valid request = %d, want StatusInvalidState", status)
	}
	if status := coreStep(idle, nil, 0); status != StatusInvalidState {
		t.Fatalf("step on an idle session with a null request = %d, want StatusInvalidState (state precedes shape)", status)
	}
	if status := coreDestroy(idle); status != StatusOK {
		t.Fatalf("destroy of the idle session = %d, want StatusOK", status)
	}

	// The connecting helper installs a fresh table, so every earlier handle
	// assertion must run before it.
	connecting := connectSessionOnlineInputConnecting(t)
	if status := coreStep(connecting, validPointer, validLength); status != StatusInvalidState {
		t.Fatalf("step while connecting with a valid request = %d, want StatusInvalidState", status)
	}
	if status := coreDisconnect(connecting); status != StatusOK {
		t.Fatalf("disconnect of the connecting session = %d, want StatusOK", status)
	}

	script := newStepTransportScript()
	online := connectSessionOnlineStep(t, script)
	for _, length := range []uint32{0, StepRequestBytes - 8, StepRequestBytes + 8} {
		if status := coreStep(online, validPointer, length); status != StatusInvalidArgument {
			t.Fatalf("step with length %d = %d, want StatusInvalidArgument", length, status)
		}
	}
	if status := coreStep(online, nil, StepRequestBytes); status != StatusInvalidArgument {
		t.Fatalf("step with a null request = %d, want StatusInvalidArgument", status)
	}
	storage := make([]byte, int(StepRequestBytes)+8)
	copy(storage[1:], unsafe.Slice(validPointer, int(StepRequestBytes)))
	misaligned := (*byte)(unsafe.Pointer(unsafe.Add(unsafe.Pointer(&storage[0]), 1)))
	if status := coreStep(online, misaligned, StepRequestBytes); status != StatusInvalidArgument {
		t.Fatalf("step with a misaligned request = %d, want StatusInvalidArgument", status)
	}
	if count := log.count(); count != 0 {
		t.Fatalf("lifecycle and shape rejections executed %d runtime steps, want 0", count)
	}
	if _, ok := sessionRetainedStep(t, online); ok {
		t.Fatal("rejected requests retained a step result")
	}
	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(online, &phase); status != StatusOK || phase != ConnectPhaseLoading {
		t.Fatalf("poll after rejections = (%d, %d), want StatusOK and loading", status, phase)
	}

	if status := coreDisconnect(online); status != StatusOK {
		t.Fatalf("disconnect of the online session = %d, want StatusOK", status)
	}
	if status := coreStep(online, validPointer, validLength); status != StatusDisconnected {
		t.Fatalf("step on a terminal session with a valid request = %d, want StatusDisconnected", status)
	}
	if status := coreDestroy(online); status != StatusOK {
		t.Fatalf("destroy after the state walk = %d, want StatusOK", status)
	}
}

// TestStepRejectsMagicAndLayoutBeforeBudgetContent pins the record-identity
// precedence: a wrong family magic or a wrong layout word reports
// `StatusABIMismatch` even when the budgets are also over-limit, and no
// runtime step runs.
func TestStepRejectsMagicAndLayoutBeforeBudgetContent(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	log := installStepSeam(t, nil)

	identityCases := []struct {
		name   string
		offset int
		value  uint32
	}{
		{"wrong family magic", 0, uint32(MagicConnection)},
		{"zero magic", 0, 0},
		{"newer layout", 4, StepVersion + 1},
		{"zero layout", 4, 0},
	}
	for _, testCase := range identityCases {
		pointer, length := stepRequestPointer(0, MaxStepMessageBudget+1, MaxStepMeshBudget+1)
		patchStepWord(pointer, testCase.offset, testCase.value)
		if status := coreStep(handle, pointer, length); status != StatusABIMismatch {
			t.Fatalf("step with %s before over-limit budgets = %d, want StatusABIMismatch", testCase.name, status)
		}
	}
	if count := log.count(); count != 0 {
		t.Fatalf("identity rejections executed %d runtime steps, want 0", count)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after identity rejections = %d, want StatusOK", status)
	}
}

// TestStepReceiverFailureRetainsTerminalFrameAndDisconnects pins the
// receiver-failure mapping: a receiver that dies mid-session makes the bounded
// step publish exactly one terminal frame, the export reports `StatusOK`,
// retains the terminal frame for the frame family to pull, and moves the
// session to its terminal state so later steps report `StatusDisconnected`
// while the retained terminal frame stays readable.
func TestStepReceiverFailureRetainsTerminalFrameAndDisconnects(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)

	terminalErr := errors.New("step receiver died")
	script.receiver.setErr(terminalErr)
	pointer, length := stepRequestPointer(0, 0, 0)
	if status := coreStep(handle, pointer, length); status != StatusOK {
		t.Fatalf("step with a dead receiver = %d, want StatusOK (the runtime publishes one terminal frame)", status)
	}
	retained, ok := sessionRetainedStep(t, handle)
	if !ok {
		t.Fatal("terminal step retained no result")
	}
	if retained.Frame.Phase != clientruntime.ConnectionPhaseDisconnected ||
		retained.Frame.Error.Code != presentation.ErrorConnection {
		t.Fatalf("retained terminal frame = phase %v error %+v, want disconnected connection error", retained.Frame.Phase, retained.Frame.Error)
	}

	phase := uint32(0xDEADBEEF)
	if status := coreConnectPoll(handle, &phase); status != StatusDisconnected {
		t.Fatalf("poll after the terminal step = %d, want StatusDisconnected", status)
	}
	if status := coreStep(handle, pointer, length); status != StatusDisconnected {
		t.Fatalf("second step after the terminal frame = %d, want StatusDisconnected", status)
	}
	retained, ok = sessionRetainedStep(t, handle)
	if !ok || retained.Frame.Phase != clientruntime.ConnectionPhaseDisconnected {
		t.Fatalf("retained result after the session turned terminal = (%+v, %v), want the terminal frame still readable", retained, ok)
	}
	session, status := clientSessions.sessionFor(handle)
	if status != StatusOK {
		t.Fatalf("session lookup after receiver death = %d, want StatusOK", status)
	}
	if err := session.terminalError(); !errors.Is(err, terminalErr) {
		t.Fatalf("terminal error = %v, want the receiver terminal error", err)
	}
	if calls := script.receiver.closeCalls(); calls != 1 {
		t.Fatalf("receiver close calls after the terminal step = %d, want 1", calls)
	}
	if calls := script.endpoint.closeCalls(); calls != 1 {
		t.Fatalf("endpoint close calls after the terminal step = %d, want 1", calls)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the terminal step = %d, want StatusOK", status)
	}
}

// TestStepReturnsWhileOutboundSendIsBlocked pins the no-network-wait contract
// through the whole export: after a drained ready state, one step with real
// elapsed enqueues prediction input and returns while the outbound worker's
// transport send is still parked, because the step path only performs
// non-blocking queue operations.
func TestStepReturnsWhileOutboundSendIsBlocked(t *testing.T) {
	script := newStepTransportScript(stepReadyStateMessage(1))
	handle := connectSessionOnlineStep(t, script)

	zeroPointer, zeroLength := stepRequestPointer(0, 1, 0)
	if status := coreStep(handle, zeroPointer, zeroLength); status != StatusOK {
		t.Fatalf("zero-elapsed drain step = %d, want StatusOK", status)
	}
	retained, ok := sessionRetainedStep(t, handle)
	if !ok || retained.Frame.Phase != clientruntime.ConnectionPhasePlay || retained.MessagesProcessed != 1 {
		t.Fatalf("drain step = (%+v, %v), want one processed message and a playable frame", retained, ok)
	}
	if count := script.endpoint.startedCount(); count != 0 {
		t.Fatalf("zero-elapsed step started %d transport sends, want 0", count)
	}

	script.endpoint.setBlocking(true)
	returned := make(chan Status, 1)
	advancePointer, advanceLength := stepRequestPointer(uint64(physics.FixedDelta), 0, 0)
	go func() {
		returned <- coreStep(handle, advancePointer, advanceLength)
	}()
	select {
	case status := <-returned:
		if status != StatusOK {
			t.Fatalf("advancing step with a blocked send = %d, want StatusOK", status)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("step waited for the blocking transport send")
	}
	if !script.endpoint.waitStarted(time.Second) {
		t.Fatal("outbound worker did not begin the parked send")
	}
	retained, ok = sessionRetainedStep(t, handle)
	if !ok || retained.Frame.Revision != 2 {
		t.Fatalf("retained result after the advancing step = (%+v, %v), want revision 2", retained, ok)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy with a parked send = %d, want StatusOK", status)
	}
}

// TestStepMapsSeamFailurePanicAndDisconnectRace pins the step outcome mapping:
// a runtime step failure with a healthy session maps to `StatusInternal` (the
// request was valid, the failure is producer-side), a panic inside the
// execution converts to `StatusPanic`, and a failure that races a disconnect
// maps to `StatusDisconnected` because the teardown closed the session.
func TestStepMapsSeamFailurePanicAndDisconnectRace(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)
	pointer, length := stepRequestPointer(0, 0, 0)

	failing := installStepSeam(t, func(*clientruntime.Runtime, time.Duration, int, int) error {
		return errStepSeamScripted
	})
	if status := coreStep(handle, pointer, length); status != StatusInternal {
		t.Fatalf("step with a producer-side failure = %d, want StatusInternal", status)
	}
	if count := failing.count(); count != 1 {
		t.Fatalf("failing seam executed %d steps, want 1", count)
	}
	if _, ok := sessionRetainedStep(t, handle); ok {
		t.Fatal("failed step retained a result")
	}

	panicking := installStepSeam(t, func(*clientruntime.Runtime, time.Duration, int, int) error {
		panic("step seam panic")
	})
	if status := coreStep(handle, pointer, length); status != StatusPanic {
		t.Fatalf("step with a panicking execution = %d, want StatusPanic", status)
	}
	if count := panicking.count(); count != 1 {
		t.Fatalf("panicking seam executed %d steps, want 1", count)
	}

	racing := installStepSeam(t, func(*clientruntime.Runtime, time.Duration, int, int) error {
		if status := coreDisconnect(handle); status != StatusOK {
			t.Errorf("disconnect inside the seam = %d, want StatusOK", status)
		}
		return errStepSeamScripted
	})
	if status := coreStep(handle, pointer, length); status != StatusDisconnected {
		t.Fatalf("step whose failure raced a disconnect = %d, want StatusDisconnected", status)
	}
	if count := racing.count(); count != 1 {
		t.Fatalf("racing seam executed %d steps, want 1", count)
	}
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the mapping cases = %d, want StatusOK", status)
	}
}

// TestStepConcurrentWithPollSubmitAndDisconnectStaysWithinVocabulary drives
// concurrent steps, polls, and submits on one online session through a racing
// disconnect: every observed status stays inside the documented vocabulary,
// and the finally retained frame revision equals the number of steps that
// returned `StatusOK`, because the export serializes steps and retains the
// latest completed result.
func TestStepConcurrentWithPollSubmitAndDisconnectStaysWithinVocabulary(t *testing.T) {
	script := newStepTransportScript()
	handle := connectSessionOnlineStep(t, script)

	steppers := 3
	iterations := 100
	var okSteps atomic.Uint64
	stepPointer, stepLength := stepRequestPointer(0, 0, 0)
	inputPointer, inputLength := inputBatchPointer(inputMoveEvent(1, 0))
	var group sync.WaitGroup
	for worker := 0; worker < steppers; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				switch status := coreStep(handle, stepPointer, stepLength); status {
				case StatusOK:
					okSteps.Add(1)
				case StatusDisconnected:
					return
				default:
					t.Errorf("step %d = %d, want StatusOK or StatusDisconnected", iteration, status)
					return
				}
			}
		}()
	}
	for worker := 0; worker < 2; worker++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for iteration := 0; iteration < iterations; iteration++ {
				switch status := coreSubmitInput(handle, inputPointer, inputLength); status {
				case StatusOK, StatusDisconnected:
				default:
					t.Errorf("submit = %d, want StatusOK or StatusDisconnected", status)
					return
				}
			}
		}()
	}
	group.Add(1)
	go func() {
		defer group.Done()
		for iteration := 0; iteration < iterations; iteration++ {
			phase := uint32(0xDEADBEEF)
			switch status := coreConnectPoll(handle, &phase); status {
			case StatusOK:
				if phase != ConnectPhaseLoading && phase != ConnectPhasePlay {
					t.Errorf("poll observed phase %d, want loading or play", phase)
				}
			case StatusDisconnected:
			default:
				t.Errorf("poll = %d, want StatusOK or StatusDisconnected", status)
				return
			}
		}
	}()
	group.Add(1)
	go func() {
		defer group.Done()
		// Let at least one step publish before tearing down so the retention
		// assertion keeps a deterministic lower bound; the spin needs only
		// scheduler rounds because the steppers wait for no external event.
		for okSteps.Load() == 0 {
			goruntime.Gosched()
		}
		if status := coreDisconnect(handle); status != StatusOK {
			t.Errorf("racing disconnect = %d, want StatusOK", status)
		}
	}()
	group.Wait()

	published := okSteps.Load()
	if published == 0 {
		t.Fatal("no step returned StatusOK before the disconnect won")
	}
	session, status := clientSessions.sessionFor(handle)
	if status != StatusOK {
		t.Fatalf("session lookup after the race = %d, want StatusOK", status)
	}
	retained, ok := session.retainedStep()
	if !ok || uint64(retained.Frame.Revision) != published {
		t.Fatalf("retained revision = %d (ok %v), want %d published frames", retained.Frame.Revision, ok, published)
	}
	connectPollUntilStatus(t, handle, StatusDisconnected)
	if status := coreDestroy(handle); status != StatusOK {
		t.Fatalf("destroy after the race = %d, want StatusOK", status)
	}
}
