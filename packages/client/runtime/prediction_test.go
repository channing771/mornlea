package runtime

import (
	"context"
	"errors"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/physics"
)

func TestPredictInputParityWithDirectPredictor(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	direct := client.NewPredictor()
	authority := predictionReadyState()
	if _, err := runtime.ApplyMessage(authority); err != nil {
		t.Fatal(err)
	}
	if err := direct.Begin(authority); err != nil {
		t.Fatal(err)
	}

	input := SemanticInput{
		MoveX: 1, MoveZ: -1, Jump: true, Yaw: 0.25, Pitch: -0.5,
		Mining: true, Eating: true, Sprinting: true, Sneaking: false,
	}
	directControl := client.Control{
		MoveX: input.MoveX, MoveZ: input.MoveZ, Jump: input.Jump,
		Yaw: input.Yaw, Pitch: input.Pitch, Mining: input.Mining,
		Eating: input.Eating, Sprinting: input.Sprinting, Sneaking: input.Sneaking,
	}
	directMirror := client.NewMirror()
	directSource := client.MirrorCollisionSource{Mirror: directMirror, Dimension: core.Overworld}
	directSequence := uint64(0)
	var directSent []network.PlayerInput
	if err := runtime.SubmitInput(input); err != nil {
		t.Fatal(err)
	}

	for _, elapsed := range []time.Duration{
		physics.FixedDelta - time.Millisecond,
		time.Millisecond,
		2 * physics.FixedDelta,
	} {
		got, gotErr := runtime.AdvancePrediction(context.Background(), elapsed)
		wantErr := direct.Advance(elapsed, directControl, directSource, func() uint64 {
			directSequence++
			return directSequence
		}, func(message network.PlayerInput) error {
			directSent = append(directSent, message)
			return nil
		})
		if !errors.Is(gotErr, wantErr) {
			t.Fatalf("AdvancePrediction(%v) error = %v, direct = %v", elapsed, gotErr, wantErr)
		}
		assertPredictionMatchesDirect(t, got, direct, elapsed)
	}

	if got := sender.playerInputs(); !reflect.DeepEqual(got, directSent) {
		t.Fatalf("runtime sent inputs = %+v, direct = %+v", got, directSent)
	}
}

func TestPredictReconcileParityReplaysUnconfirmedInput(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	direct := client.NewPredictor()
	authority := predictionReadyState()
	if _, err := runtime.ApplyMessage(authority); err != nil {
		t.Fatal(err)
	}
	if err := direct.Begin(authority); err != nil {
		t.Fatal(err)
	}

	input := SemanticInput{MoveZ: 1, Yaw: 0.4, Pitch: -0.2}
	directControl := client.Control{MoveZ: 1, Yaw: 0.4, Pitch: -0.2}
	directMirror := client.NewMirror()
	directSource := client.MirrorCollisionSource{Mirror: directMirror, Dimension: core.Overworld}
	directSequence := uint64(0)
	if err := runtime.SubmitInput(input); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if _, err := runtime.AdvancePrediction(context.Background(), physics.FixedDelta); err != nil {
			t.Fatal(err)
		}
		if err := direct.Advance(physics.FixedDelta, directControl, directSource, func() uint64 {
			directSequence++
			return directSequence
		}, func(network.PlayerInput) error { return nil }); err != nil {
			t.Fatal(err)
		}
	}

	corrected := authority
	corrected.ServerTick++
	corrected.LastInputSequence = 1
	corrected.Position = mgl32.Vec3{0.75, 10, 0.25}
	wantResult, err := direct.ApplyPlayerState(corrected, directSource)
	if err != nil {
		t.Fatal(err)
	}
	sentBefore := len(sender.playerInputs())
	outcome, err := runtime.ApplyMessage(corrected)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Handled || !outcome.PredictionChanged {
		t.Fatalf("PlayerState outcome = %+v, want handled prediction change", outcome)
	}
	wantRuntimeResult := ReconcileResult{
		ResetView: wantResult.ResetView,
		Yaw:       wantResult.Yaw,
		Pitch:     wantResult.Pitch,
	}
	if outcome.Reconcile != wantRuntimeResult {
		t.Fatalf("runtime reconcile = %+v, direct = %+v", outcome.Reconcile, wantRuntimeResult)
	}
	assertPredictionMatchesDirect(t, outcome.Prediction, direct, 0)
	if got := len(sender.playerInputs()); got != sentBefore {
		t.Fatalf("reconciliation sent %d extra inputs", got-sentBefore)
	}
	const smoothingElapsed = 25 * time.Millisecond
	got, gotErr := runtime.AdvancePrediction(context.Background(), smoothingElapsed)
	wantErr := direct.Advance(smoothingElapsed, directControl, directSource, func() uint64 {
		directSequence++
		return directSequence
	}, func(network.PlayerInput) error { return nil })
	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("post-reconcile advance error = %v, direct = %v", gotErr, wantErr)
	}
	assertPredictionMatchesDirect(t, got, direct, smoothingElapsed)
}

func TestPredictInputSendFailurePreservesStateAndMonotonicSequence(t *testing.T) {
	sendErr := errors.New("send failed")
	sender := &predictionTestSender{failures: []error{sendErr, nil}}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	if _, err := runtime.ApplyMessage(predictionReadyState()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SubmitInput(SemanticInput{MoveX: 1}); err != nil {
		t.Fatal(err)
	}
	before := runtime.Prediction()

	if _, err := runtime.AdvancePrediction(context.Background(), physics.FixedDelta); !errors.Is(err, sendErr) {
		t.Fatalf("first AdvancePrediction error = %v, want send failure", err)
	}
	afterFailure := runtime.Prediction()
	if afterFailure.State != before.State || afterFailure.HistoryLength != 0 {
		t.Fatalf("send failure changed prediction: before=%+v after=%+v", before, afterFailure)
	}
	if _, err := runtime.AdvancePrediction(context.Background(), 0); err != nil {
		t.Fatal(err)
	}
	got := sender.playerInputs()
	if len(got) != 2 || got[0].Sequence != 1 || got[1].Sequence != 2 {
		t.Fatalf("send attempts = %+v, want monotonic sequences 1,2", got)
	}
	if snapshot := runtime.Prediction(); snapshot.HistoryLength != 1 || snapshot.State == before.State {
		t.Fatalf("successful retry did not advance prediction: %+v", snapshot)
	}
}

func TestPredictInputUsesCallerContext(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	if _, err := runtime.ApplyMessage(predictionReadyState()); err != nil {
		t.Fatal(err)
	}
	type contextKey struct{}
	ctx := context.WithValue(context.Background(), contextKey{}, "caller")
	if _, err := runtime.AdvancePrediction(ctx, physics.FixedDelta); err != nil {
		t.Fatal(err)
	}
	contexts := sender.sendContexts()
	if len(contexts) != 1 || contexts[0] != ctx || contexts[0].Value(contextKey{}) != "caller" {
		t.Fatalf("sender contexts = %v, want exact caller context", contexts)
	}
}

func TestPredictInputRemoteEndpointIsSenderButReceiverRemainsCloseOwner(t *testing.T) {
	endpoint := &predictionTestEndpoint{}
	receiver := &predictionTestEndpointReceiver{endpoint: endpoint}
	runtime, err := NewRemote(context.Background(), validRemoteOptions(RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) {
			return &remoteTestStream{}, nil
		},
		Login: func(context.Context, network.ClientPacketStream, network.Identity, uint8) (network.ClientEndpoint, uint64, error) {
			return endpoint, 0, nil
		},
		NewReceiver: func(network.ClientEndpoint, int) (Receiver, error) {
			return receiver, nil
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.ApplyMessage(predictionReadyState()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AdvancePrediction(context.Background(), physics.FixedDelta); err != nil {
		t.Fatal(err)
	}
	if got := endpoint.playerInputs(); len(got) != 1 || got[0].Sequence != 1 {
		t.Fatalf("remote endpoint inputs = %+v, want sequence 1", got)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if receiver.closeCalls != 1 || endpoint.closeCalls != 1 {
		t.Fatalf("receiver/endpoint close calls = %d/%d, want 1/1", receiver.closeCalls, endpoint.closeCalls)
	}
}

func TestPredictInputHistoryCapacitySuspensionMatchesDirectPredictor(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	direct := client.NewPredictor()
	authority := predictionReadyState()
	if _, err := runtime.ApplyMessage(authority); err != nil {
		t.Fatal(err)
	}
	if err := direct.Begin(authority); err != nil {
		t.Fatal(err)
	}
	input := SemanticInput{MoveX: 1, Yaw: 0.3, Pitch: -0.2}
	if err := runtime.SubmitInput(input); err != nil {
		t.Fatal(err)
	}
	directControl := client.Control{MoveX: 1, Yaw: 0.3, Pitch: -0.2}
	directSource := client.MirrorCollisionSource{Mirror: client.NewMirror(), Dimension: core.Overworld}
	directSequence := uint64(0)
	var directSent []network.PlayerInput
	for range 257 {
		got, gotErr := runtime.AdvancePrediction(context.Background(), physics.FixedDelta)
		wantErr := direct.Advance(physics.FixedDelta, directControl, directSource, func() uint64 {
			directSequence++
			return directSequence
		}, func(message network.PlayerInput) error {
			directSent = append(directSent, message)
			return nil
		})
		if !errors.Is(gotErr, wantErr) {
			t.Fatalf("runtime error = %v, direct = %v", gotErr, wantErr)
		}
		assertPredictionMatchesDirect(t, got, direct, physics.FixedDelta)
	}
	if !runtime.Prediction().Suspended {
		t.Fatal("history capacity did not suspend runtime prediction")
	}
	if got := sender.playerInputs(); !reflect.DeepEqual(got, directSent) {
		t.Fatalf("suspension inputs differ: runtime=%+v direct=%+v", got, directSent)
	}
	last := directSent[len(directSent)-1]
	if last.MoveX != 0 || last.MoveZ != 0 || last.Jump || last.Sequence != 257 {
		t.Fatalf("suspension input = %+v, want sequence 257 neutral input", last)
	}
}

func TestPredictInputSequenceRemainsMonotonicAcrossAuthoritativeReset(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	initial := predictionReadyState()
	if _, err := runtime.ApplyMessage(initial); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SubmitInput(SemanticInput{MoveZ: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AdvancePrediction(context.Background(), physics.FixedDelta); err != nil {
		t.Fatal(err)
	}
	reset := initial
	reset.ServerTick++
	reset.LastInputSequence = 1
	reset.Reset = true
	if _, err := runtime.ApplyMessage(reset); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AdvancePrediction(context.Background(), physics.FixedDelta); err != nil {
		t.Fatal(err)
	}
	got := sender.playerInputs()
	if len(got) != 2 || got[0].Sequence != 1 || got[1].Sequence != 2 {
		t.Fatalf("authoritative reset sequences = %+v, want monotonic 1,2", got)
	}
}

func TestPredictInputAndResyncShareMonotonicSequence(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	if _, err := runtime.ApplyMessage(predictionReadyState()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SubmitInput(SemanticInput{MoveZ: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AdvancePrediction(context.Background(), physics.FixedDelta); err != nil {
		t.Fatal(err)
	}
	outcome, err := runtime.ApplyMessage(network.BlockChanges{
		Dimension:    core.Overworld,
		Chunk:        core.ChunkPos{},
		BaseRevision: 1,
		NewRevision:  2,
		Changes: []network.BlockChange{{
			Position: core.BlockPos{Y: core.MinY},
			Block:    core.StoneID,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.World.Resync == nil || outcome.World.Resync.Sequence != 2 {
		t.Fatalf("numbered resync = %+v, want sequence 2", outcome.World.Resync)
	}
	if _, err := runtime.AdvancePrediction(context.Background(), physics.FixedDelta); err != nil {
		t.Fatal(err)
	}
	got := sender.playerInputs()
	if len(got) != 2 || got[0].Sequence != 1 || got[1].Sequence != 3 {
		t.Fatalf("player input sequences around resync = %+v, want 1,3", got)
	}
}

func TestPredictReconcilePreservesStaleResetAndReadinessSemantics(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	initial := predictionReadyState()
	initial.Yaw = -0.8
	initial.Pitch = 0.3
	outcome, err := runtime.ApplyMessage(initial)
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Reconcile.ResetView || !outcome.Prediction.Ready {
		t.Fatalf("first ready outcome = %+v", outcome)
	}
	if got := runtime.Phase(); got != ConnectionPhasePlay {
		t.Fatalf("phase after ready state = %v, want %v", got, ConnectionPhasePlay)
	}
	retained := outcome.Prediction

	stale := initial
	stale.Position = mgl32.Vec3{99, 99, 99}
	stale.Ready = false
	staleOutcome, err := runtime.ApplyMessage(stale)
	if err != nil {
		t.Fatal(err)
	}
	if !staleOutcome.Handled || staleOutcome.PredictionChanged || runtime.Prediction() != retained {
		t.Fatalf("stale state changed prediction: outcome=%+v snapshot=%+v", staleOutcome, runtime.Prediction())
	}

	notReady := initial
	notReady.ServerTick++
	notReady.Ready = false
	notReady.LastInputSequence = 0
	if _, err := runtime.ApplyMessage(notReady); err != nil {
		t.Fatal(err)
	}
	if snapshot := runtime.Prediction(); snapshot.Ready || snapshot.State != (physics.State{}) {
		t.Fatalf("Ready=false snapshot = %+v", snapshot)
	}
	if got := runtime.Phase(); got != ConnectionPhaseLoading {
		t.Fatalf("phase after not-ready state = %v, want %v", got, ConnectionPhaseLoading)
	}

	reset := initial
	reset.ServerTick += 2
	reset.Reset = true
	reset.Position = mgl32.Vec3{4, 5, 6}
	reset.Yaw = 1.1
	reset.Pitch = -0.25
	resetOutcome, err := runtime.ApplyMessage(reset)
	if err != nil {
		t.Fatal(err)
	}
	if !resetOutcome.Reconcile.ResetView || resetOutcome.Reconcile.Yaw != reset.Yaw || resetOutcome.Reconcile.Pitch != reset.Pitch {
		t.Fatalf("reset reconcile = %+v", resetOutcome.Reconcile)
	}
	if got := runtime.Phase(); got != ConnectionPhasePlay {
		t.Fatalf("phase after ready reset = %v, want %v", got, ConnectionPhasePlay)
	}
	if retained.State.Position != initial.Position || !retained.Ready {
		t.Fatalf("retained snapshot was mutated by later messages: %+v", retained)
	}
}

func TestPredictInputSessionResetRecreatesSequenceAndPrediction(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	if _, err := runtime.ApplyMessage(predictionReadyState()); err != nil {
		t.Fatal(err)
	}
	if err := runtime.SubmitInput(SemanticInput{MoveX: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AdvancePrediction(context.Background(), physics.FixedDelta); err != nil {
		t.Fatal(err)
	}
	retained := runtime.Prediction()
	runtime.ResetMirrors()
	if snapshot := runtime.Prediction(); snapshot.Ready || snapshot.State != (physics.State{}) || snapshot.HistoryLength != 0 {
		t.Fatalf("reset prediction = %+v", snapshot)
	}
	if retained.HistoryLength != 1 {
		t.Fatalf("retained snapshot changed after reset: %+v", retained)
	}
	if _, err := runtime.ApplyMessage(predictionReadyState()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AdvancePrediction(context.Background(), physics.FixedDelta); err != nil {
		t.Fatal(err)
	}
	got := sender.playerInputs()
	if len(got) != 2 || got[0].Sequence != 1 || got[1].Sequence != 1 {
		t.Fatalf("session sequences = %+v, want reset to 1", got)
	}
}

func TestPredictInputRejectsNilContextAndNonFiniteSemanticInput(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	if _, err := runtime.ApplyMessage(predictionReadyState()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.AdvancePrediction(nil, physics.FixedDelta); err == nil {
		t.Fatal("AdvancePrediction accepted nil context")
	}
	before := runtime.Prediction()
	if _, err := runtime.AdvancePrediction(context.Background(), -time.Nanosecond); err == nil {
		t.Fatal("AdvancePrediction accepted negative elapsed time")
	}
	if after := runtime.Prediction(); after != before {
		t.Fatalf("negative elapsed changed prediction: before=%+v after=%+v", before, after)
	}
	if err := runtime.SubmitInput(SemanticInput{Yaw: float32(math.NaN())}); err == nil {
		t.Fatal("SubmitInput accepted non-finite semantic input")
	}
	if len(sender.playerInputs()) != 0 {
		t.Fatal("invalid semantic input reached sender")
	}
}

func TestPredictInputResetCloseConcurrency(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	if _, err := runtime.ApplyMessage(predictionReadyState()); err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(3)
		go func() {
			defer wait.Done()
			_, _ = runtime.AdvancePrediction(context.Background(), physics.FixedDelta)
		}()
		go func() {
			defer wait.Done()
			runtime.ResetMirrors()
		}()
		go func() {
			defer wait.Done()
			_ = runtime.Close()
		}()
	}
	wait.Wait()
	if runtime.Prediction().Ready {
		t.Fatal("closed runtime retained ready prediction")
	}
}

func assertPredictionMatchesDirect(
	t *testing.T,
	got PredictionSnapshot,
	direct *client.Predictor,
	presentationElapsed time.Duration,
) {
	t.Helper()
	wantState, wantReady := direct.State()
	wantPosition, wantPresentationReady := direct.PresentationPosition(presentationElapsed)
	if got.State != wantState || got.Ready != wantReady || got.HistoryLength != direct.HistoryLen() || got.Suspended != direct.Suspended() {
		t.Fatalf("runtime prediction = %+v, direct state=%+v ready=%v history=%d suspended=%v",
			got, wantState, wantReady, direct.HistoryLen(), direct.Suspended())
	}
	if got.PresentationPosition != wantPosition || got.PresentationReady != wantPresentationReady {
		t.Fatalf("runtime presentation = (%v,%v), direct = (%v,%v)",
			got.PresentationPosition, got.PresentationReady, wantPosition, wantPresentationReady)
	}
}

func predictionReadyState() network.PlayerState {
	return network.PlayerState{
		ServerTick: 1,
		Dimension:  core.Overworld,
		Position:   mgl32.Vec3{0.5, 10, 0.5},
		Yaw:        0.2,
		Pitch:      -0.1,
		OnGround:   true,
		Ready:      true,
	}
}

type predictionTestSender struct {
	mu       sync.Mutex
	messages []network.ClientMessage
	contexts []context.Context
	failures []error
}

type predictionTestEndpoint struct {
	predictionTestSender
	closeCalls int
}

func (*predictionTestEndpoint) Recv(context.Context) (network.ServerMessage, error) {
	return nil, errors.New("unexpected receive")
}

func (endpoint *predictionTestEndpoint) Close() error {
	endpoint.closeCalls++
	return nil
}

type predictionTestEndpointReceiver struct {
	endpoint   *predictionTestEndpoint
	closeCalls int
}

func (*predictionTestEndpointReceiver) TryRecv() (network.ServerMessage, bool) { return nil, false }
func (*predictionTestEndpointReceiver) Err() error                             { return nil }
func (receiver *predictionTestEndpointReceiver) Close() error {
	receiver.closeCalls++
	return receiver.endpoint.Close()
}

func (sender *predictionTestSender) Send(ctx context.Context, message network.ClientMessage) error {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	sender.messages = append(sender.messages, message)
	sender.contexts = append(sender.contexts, ctx)
	if len(sender.failures) == 0 {
		return nil
	}
	err := sender.failures[0]
	sender.failures = sender.failures[1:]
	return err
}

func (sender *predictionTestSender) sendContexts() []context.Context {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return append([]context.Context(nil), sender.contexts...)
}

func (sender *predictionTestSender) playerInputs() []network.PlayerInput {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	inputs := make([]network.PlayerInput, 0, len(sender.messages))
	for _, message := range sender.messages {
		input, ok := message.(network.PlayerInput)
		if ok {
			inputs = append(inputs, input)
		}
	}
	return inputs
}
