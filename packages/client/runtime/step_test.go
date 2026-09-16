package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/presentation"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

func TestStepBoundedMessageDrainPublishesOneImmutableFrame(t *testing.T) {
	receiver := &stepTestReceiver{messages: []network.ServerMessage{
		stepReadyState(1),
		network.InventoryState{},
		network.CraftingState{Size: 2},
	}}
	runtime := newLoggedInRuntime(receiver, nil, 0)
	t.Cleanup(func() { _ = runtime.Close() })

	first, err := runtime.Step(0, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if first.MessagesProcessed != 1 || receiver.remaining() != 2 {
		t.Fatalf("first Step processed/remaining = %d/%d, want 1/2", first.MessagesProcessed, receiver.remaining())
	}
	if first.Frame.Version != FrameSnapshotVersion || first.Frame.Revision != 1 || first.Frame.Epoch != 1 {
		t.Fatalf("first frame identity = version %d revision %d epoch %d", first.Frame.Version, first.Frame.Revision, first.Frame.Epoch)
	}
	if first.Frame.Phase != ConnectionPhasePlay || !first.Frame.Camera.Ready || !first.Frame.HUD.Ready {
		t.Fatalf("first frame readiness = phase %v camera %v HUD %v", first.Frame.Phase, first.Frame.Camera.Ready, first.Frame.HUD.Ready)
	}
	if first.HasWorld {
		t.Fatal("Step without meshing published a world batch")
	}
	if err := first.Frame.Validate(); err != nil {
		t.Fatalf("published frame is invalid: %v", err)
	}

	second, err := runtime.Step(0, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if second.MessagesProcessed != 1 || receiver.remaining() != 1 || second.Frame.Revision != 2 {
		t.Fatalf("second Step processed/remaining/revision = %d/%d/%d, want 1/1/2", second.MessagesProcessed, receiver.remaining(), second.Frame.Revision)
	}
	if first.Frame.HUD != (presentation.HUDSnapshot{Ready: true, Health: 15, Hunger: 12, Oxygen: 240}) {
		t.Fatalf("retained first frame changed after later step: %+v", first.Frame.HUD)
	}
}

func TestStepFrameAggregatesAuthoritativeEnvironmentEntitiesAndExplicitInterpolation(t *testing.T) {
	remoteID := core.PlayerID{0x40, 0x34, 0x4f, 0xa8, 0x94, 0x20, 0x4c, 0xd1, 0x82, 0xaa, 0x1a, 0x62, 0x58, 0xe0, 0x5f, 0x44}
	receiver := &stepTestReceiver{messages: []network.ServerMessage{
		stepReadyState(7),
		network.RemotePlayerSpawn{
			PlayerID: remoteID, DisplayName: "Remote", ServerTick: 5,
			Dimension: core.Overworld, Position: mgl32.Vec3{1, 2, 3},
		},
		network.RemotePlayerStates{ServerTick: 6, Players: []network.RemotePlayerState{{
			PlayerID: remoteID, Dimension: core.Overworld, Position: mgl32.Vec3{5, 2, 3},
		}}},
	}}
	runtime := newLoggedInRuntime(receiver, nil, 0)
	t.Cleanup(func() { _ = runtime.Close() })

	result, err := runtime.Step(50*time.Millisecond, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	wantEnvironment := presentation.EnvironmentSnapshot{
		Ready: true, ServerTick: 7, WorldTimeTicks: 98_765, DayPhaseOffset: 321,
		Weather: core.WeatherRain, Season: core.SeasonAutumn, SeasonProgress: 91, Temperature: -7,
	}
	if result.Frame.Environment != wantEnvironment {
		t.Fatalf("environment = %+v, want %+v", result.Frame.Environment, wantEnvironment)
	}
	records := result.Frame.Entities.Records()
	if len(records) != 1 || records[0].PlayerID != remoteID || records[0].Position[0] <= 1 || records[0].Position[0] >= 5 {
		t.Fatalf("interpolated entities = %+v, want one bounded pose between snapshots", records)
	}
}

func TestStepNoWallClockAndNoSynchronousNetworkWait(t *testing.T) {
	sender := newBlockingStepSender()
	runtime := newLoggedInRuntime(&stepTestReceiver{}, sender, 0)
	if _, err := runtime.ApplyMessage(stepReadyState(1)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	time.Sleep(20 * time.Millisecond)
	if _, err := runtime.Step(0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if sender.startedCount() != 0 {
		t.Fatal("zero elapsed Step advanced prediction from wall-clock time")
	}

	returned := make(chan error, 1)
	go func() {
		_, err := runtime.Step(physics.FixedDelta, 0, 0)
		returned <- err
	}()
	select {
	case err := <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Step waited for the blocking endpoint Send")
	}
	if !sender.waitStarted(time.Second) {
		t.Fatal("outbound worker did not begin the queued send")
	}
}

func TestStepOrdersNumberedResyncBeforePrediction(t *testing.T) {
	sender := &recordingStepSender{notify: make(chan struct{}, 4)}
	receiver := &stepTestReceiver{}
	runtime := newLoggedInRuntime(receiver, sender, 0)
	t.Cleanup(func() { _ = runtime.Close() })
	if _, err := runtime.ApplyMessage(stepReadyState(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ApplyMessage(meshSnapshot(3, core.AirID)); err != nil {
		t.Fatal(err)
	}
	receiver.messages = []network.ServerMessage{network.BlockChanges{
		// Base 4 against the revision-3 mirror chunk reveals a revision gap: the protocol
		// requires new == base+1, so this valid delta cannot apply and must trigger resync.
		Dimension: core.Overworld, Chunk: core.ChunkPos{}, BaseRevision: 4, NewRevision: 5,
		Changes: []network.BlockChange{{Position: core.BlockPos{Y: core.MinY}, Block: core.StoneID}},
	}}

	if _, err := runtime.Step(physics.FixedDelta, 1, 0); err != nil {
		t.Fatal(err)
	}
	if !sender.waitMessages(2, time.Second) {
		t.Fatalf("outbound messages = %#v, want resync plus input", sender.snapshot())
	}
	got := sender.snapshot()
	resync, resyncOK := got[0].(network.RequestChunkResync)
	input, inputOK := got[1].(network.PlayerInput)
	if !resyncOK || !inputOK || resync.Sequence != 1 || input.Sequence != 2 {
		t.Fatalf("ordered outbound = %#v, want resync sequence 1 then input sequence 2", got)
	}
}

func TestStepInvalidAndZeroBudgetsAreAtomic(t *testing.T) {
	receiver := &stepTestReceiver{messages: []network.ServerMessage{stepReadyState(1)}}
	runtime := newLoggedInRuntime(receiver, nil, 0)
	t.Cleanup(func() { _ = runtime.Close() })

	invalid := []struct {
		elapsed                   time.Duration
		messageBudget, meshBudget int
	}{
		{elapsed: -time.Nanosecond},
		{messageBudget: -1},
		{messageBudget: MaxStepMessageBudget + 1},
		{meshBudget: -1},
		{meshBudget: presentation.MaxWorldBatchOperations + 1},
	}
	for _, test := range invalid {
		if _, err := runtime.Step(test.elapsed, test.messageBudget, test.meshBudget); err == nil {
			t.Fatalf("Step(%v,%d,%d) accepted invalid arguments", test.elapsed, test.messageBudget, test.meshBudget)
		}
		if receiver.remaining() != 1 || runtime.Prediction().Ready {
			t.Fatal("invalid Step partially mutated receiver or prediction state")
		}
	}

	zero, err := runtime.Step(0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if zero.MessagesProcessed != 0 || receiver.remaining() != 1 || zero.HasWorld {
		t.Fatalf("zero-budget Step = %+v remaining=%d", zero, receiver.remaining())
	}
}

func TestStepFailureDoesNotConsumeResetTargetLatch(t *testing.T) {
	runtime := newCameraRuntime(t)
	eye := mgl32.Vec3{0.5, 3.5, 2.5}
	readyCameraRuntime(t, runtime, eye.Sub(mgl32.Vec3{0, physics.ActiveTunables().EyeHeight, 0}), 0, 0)
	loadCameraChunks(t, runtime, core.ChunkPos{}, core.ChunkPos{Z: -1})
	setCameraMirrorBlock(t, runtime, core.BlockPos{X: 0, Y: 3, Z: -3}, core.BrickID)
	if _, err := runtime.ApplyMessage(network.PlayerState{
		ServerTick: 2, Dimension: core.Overworld, Position: eye.Sub(mgl32.Vec3{0, physics.ActiveTunables().EyeHeight, 0}),
		OnGround: true, Ready: true, Reset: true, Health: 15, Hunger: 12, Oxygen: 240,
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := runtime.Step(0, -1, 0); err == nil {
		t.Fatal("invalid Step unexpectedly succeeded")
	}
	resetFrame, err := runtime.Step(0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if resetFrame.Frame.Target.Visible {
		t.Fatalf("first successfully published reset frame retained target: %+v", resetFrame.Frame.Target)
	}
	after, err := runtime.Step(0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Frame.Target.Visible {
		t.Fatalf("target did not return after published reset frame: %+v", after.Frame.Target)
	}
}

func TestStepReceiverFailurePublishesOneTerminalFrame(t *testing.T) {
	receiverErr := errors.New("receive failed")
	receiver := &stepTestReceiver{err: receiverErr}
	runtime := newLoggedInRuntime(receiver, nil, 0)

	result, err := runtime.Step(0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Frame.Phase != ConnectionPhaseDisconnected || result.Frame.Error.Code != presentation.ErrorConnection {
		t.Fatalf("terminal frame = %+v", result.Frame)
	}
	if _, err := runtime.Step(0, 0, 0); err == nil {
		t.Fatal("second Step after terminal publication succeeded")
	}
	if !errors.Is(runtime.Err(), receiverErr) {
		t.Fatalf("runtime error = %v, want receiver error", runtime.Err())
	}
}

func TestStepCloseCancelsInflightAndDiscardsQueuedOutbound(t *testing.T) {
	sender := newBlockingStepSender()
	runtime := newLoggedInRuntime(&stepTestReceiver{}, sender, 0)
	if _, err := runtime.ApplyMessage(stepReadyState(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Step(5*physics.FixedDelta, 0, 0); err != nil {
		t.Fatal(err)
	}
	if !sender.waitStarted(time.Second) {
		t.Fatal("outbound worker did not begin an in-flight send")
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if sender.startedCount() != 1 || sender.completedCount() != 0 {
		t.Fatalf("close outbound started/completed = %d/%d, want 1/0", sender.startedCount(), sender.completedCount())
	}
}

func stepReadyState(tick uint64) network.PlayerState {
	return network.PlayerState{
		ServerTick: tick, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true, Health: 15, Hunger: 12, Oxygen: 240,
		DayPhaseOffset: 321, WorldTimeTicks: 98_765, WeatherKind: core.WeatherRain,
		Season: core.SeasonAutumn, SeasonProgress: 91, Temperature: -7,
	}
}

type stepTestReceiver struct {
	mu       sync.Mutex
	messages []network.ServerMessage
	err      error
	closed   bool
}

func (receiver *stepTestReceiver) TryRecv() (network.ServerMessage, bool) {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	if len(receiver.messages) == 0 {
		return nil, false
	}
	message := receiver.messages[0]
	receiver.messages = receiver.messages[1:]
	return message, true
}

func (receiver *stepTestReceiver) Err() error {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	return receiver.err
}

func (receiver *stepTestReceiver) Close() error {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	receiver.closed = true
	return nil
}

func (receiver *stepTestReceiver) remaining() int {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	return len(receiver.messages)
}

type recordingStepSender struct {
	mu       sync.Mutex
	messages []network.ClientMessage
	notify   chan struct{}
}

func (sender *recordingStepSender) Send(_ context.Context, message network.ClientMessage) error {
	sender.mu.Lock()
	sender.messages = append(sender.messages, message)
	sender.mu.Unlock()
	select {
	case sender.notify <- struct{}{}:
	default:
	}
	return nil
}

func (sender *recordingStepSender) snapshot() []network.ClientMessage {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return append([]network.ClientMessage(nil), sender.messages...)
}

func (sender *recordingStepSender) waitMessages(count int, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for len(sender.snapshot()) < count {
		select {
		case <-sender.notify:
		case <-deadline.C:
			return false
		}
	}
	return true
}

type blockingStepSender struct {
	mu        sync.Mutex
	started   int
	completed int
	notify    chan struct{}
}

func newBlockingStepSender() *blockingStepSender {
	return &blockingStepSender{notify: make(chan struct{}, 1)}
}

func (sender *blockingStepSender) Send(ctx context.Context, _ network.ClientMessage) error {
	sender.mu.Lock()
	sender.started++
	sender.mu.Unlock()
	select {
	case sender.notify <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func (sender *blockingStepSender) waitStarted(timeout time.Duration) bool {
	if sender.startedCount() > 0 {
		return true
	}
	select {
	case <-sender.notify:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (sender *blockingStepSender) startedCount() int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.started
}

func (sender *blockingStepSender) completedCount() int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return sender.completed
}
