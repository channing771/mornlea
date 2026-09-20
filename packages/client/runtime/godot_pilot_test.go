package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// TestGodotMovementParity is the fixed-input/correction transcript consumed by
// the pilot gate. It compares the host-facing runtime with the existing
// predictor at every frame, including command sequence numbers and reset data.
func TestGodotMovementParity(t *testing.T) {
	sender := &predictionTestSender{}
	runtime := newLoggedInRuntime(&messageTestReceiver{}, sender, 0)
	t.Cleanup(func() { _ = runtime.Close() })
	direct := client.NewPredictor()
	authority := predictionReadyState()
	if _, err := runtime.ApplyMessage(authority); err != nil {
		t.Fatal(err)
	}
	if err := direct.Begin(authority); err != nil {
		t.Fatal(err)
	}
	input := SemanticInput{MoveX: 1, MoveZ: -1, Jump: true, Yaw: 0.25, Pitch: -0.5, Sprinting: true}
	if err := runtime.SubmitInput(input); err != nil {
		t.Fatal(err)
	}
	directControl := input.control()
	directSource := client.MirrorCollisionSource{Mirror: client.NewMirror(), Dimension: core.Overworld}
	var directInputs []network.PlayerInput
	var directSequence uint64
	for _, elapsed := range []time.Duration{physics.FixedDelta - time.Millisecond, time.Millisecond, 2 * physics.FixedDelta} {
		got, err := runtime.AdvancePrediction(context.Background(), elapsed)
		if err != nil {
			t.Fatal(err)
		}
		if err := direct.Advance(elapsed, directControl, directSource, func() uint64 {
			directSequence++
			return directSequence
		}, func(message network.PlayerInput) error {
			directInputs = append(directInputs, message)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		assertPredictionMatchesDirect(t, got, direct, elapsed)
	}
	corrected := authority
	corrected.ServerTick++
	corrected.LastInputSequence = 1
	corrected.Position = corrected.Position.Add(mgl32.Vec3{0.25, 0, -0.25})
	if _, err := direct.ApplyPlayerState(corrected, directSource); err != nil {
		t.Fatal(err)
	}
	outcome, err := runtime.ApplyMessage(corrected)
	if err != nil || !outcome.Handled || !outcome.PredictionChanged {
		t.Fatalf("correction outcome = %+v, err = %v", outcome, err)
	}
	assertPredictionMatchesDirect(t, outcome.Prediction, direct, 0)
	if got := sender.playerInputs(); !reflect.DeepEqual(got, directInputs) {
		t.Fatalf("pilot command sequence = %+v, baseline = %+v", got, directInputs)
	}
}

// TestGodotDisconnectProtocolRejection pins the terminal path before any
// runtime is published: a rejected protocol closes the stream exactly once.
func TestGodotDisconnectProtocolRejection(t *testing.T) {
	stream := &scriptedClientPacketStream{packets: []protocol.ServerPacket{
		protocol.HandshakeReject{
			ServerProtocolVersion: protocol.ProtocolVersion + 1,
			Code:                  protocol.HandshakeVersionMismatch,
			Message:               "version mismatch",
		},
	}}
	runtime, err := NewRemote(context.Background(), validRemoteOptions(RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) { return stream, nil },
	}))
	if runtime != nil || err == nil || stream.closeCalls != 1 {
		t.Fatalf("protocol rejection runtime/error/close = %v/%v/%d", runtime, err, stream.closeCalls)
	}
}

// TestGodotDisconnectReceiverErrorPins the receiver overflow/error boundary
// and confirms reverse ownership release after terminal publication.
func TestGodotDisconnectReceiverError(t *testing.T) {
	receiverErr := errors.New("receiver overflow")
	endpoint := &remoteTestEndpoint{}
	receiver := &remoteTestReceiver{endpoint: endpoint, err: receiverErr}
	runtime, err := NewRemote(context.Background(), validRemoteOptions(RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) { return &remoteTestStream{}, nil },
		Login: func(context.Context, network.ClientPacketStream, network.Identity, uint8) (network.ClientEndpoint, uint64, error) {
			return endpoint, 0, nil
		},
		NewReceiver: func(network.ClientEndpoint, int) (Receiver, error) { return receiver, nil },
	}))
	if err != nil || runtime == nil {
		t.Fatal(err)
	}
	if runtime.Phase() != ConnectionPhaseDisconnected || !errors.Is(runtime.Err(), receiverErr) {
		t.Fatalf("terminal phase/error = %v/%v", runtime.Phase(), runtime.Err())
	}
	if receiver.closeCalls != 1 || endpoint.closeCalls != 1 {
		t.Fatalf("receiver/endpoint closes = %d/%d", receiver.closeCalls, endpoint.closeCalls)
	}
}

// TestGodotShutdownReleasesResourcesReverseOrder pins client-requested exit
// after a partially constructed host. No terminal failure is fabricated.
func TestGodotShutdownReleasesResourcesReverseOrder(t *testing.T) {
	var events []string
	var eventsMu sync.Mutex
	runtime, err := New(Options{Dependencies: Dependencies{Resources: []ResourceFactory{
		resourceFactoryFunc(func() (Resource, error) {
			return &recordingResource{name: "first", events: &events, eventsMu: &eventsMu}, nil
		}),
		resourceFactoryFunc(func() (Resource, error) {
			return &recordingResource{name: "second", events: &events, eventsMu: &eventsMu}, nil
		}),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if got, want := events, []string{"close:second", "close:first"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shutdown release order = %v, want %v", got, want)
	}
	if runtime.Phase() != ConnectionPhaseDisconnected || runtime.Err() != nil {
		t.Fatalf("shutdown terminal state = %v/%v, want clean disconnect", runtime.Phase(), runtime.Err())
	}
}
