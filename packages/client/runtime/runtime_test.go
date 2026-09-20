package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
	"github.com/channing771/mornlea/packages/shared/network/protocol"
)

func TestLifecyclePartialConstructionFailureReleasesInStrictReverseOrder(t *testing.T) {
	constructErr := errors.New("construct third resource")
	secondCloseErr := errors.New("close second resource")
	firstCloseErr := errors.New("close first resource")
	var events []string

	runtime, err := New(Options{Dependencies: Dependencies{Resources: []ResourceFactory{
		resourceFactoryFunc(func() (Resource, error) {
			events = append(events, "open:first")
			return &recordingResource{name: "first", events: &events, closeErr: firstCloseErr}, nil
		}),
		resourceFactoryFunc(func() (Resource, error) {
			events = append(events, "open:second")
			return &recordingResource{name: "second", events: &events, closeErr: secondCloseErr}, nil
		}),
		resourceFactoryFunc(func() (Resource, error) {
			events = append(events, "open:third")
			return nil, constructErr
		}),
	}}})
	if runtime != nil {
		t.Fatal("New returned a runtime after construction failure")
	}
	if !errors.Is(err, constructErr) || !errors.Is(err, secondCloseErr) || !errors.Is(err, firstCloseErr) {
		t.Fatalf("New error does not preserve construction and release errors: %v", err)
	}
	if want := []string{"open:first", "open:second", "open:third", "close:second", "close:first"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

func TestLifecycleInvalidOptionsHaveNoSideEffects(t *testing.T) {
	if runtime, err := New(Options{}); runtime != nil || err == nil {
		t.Fatalf("New(Options{}) = (%v, %v), want nil runtime and error", runtime, err)
	}

	called := false
	runtime, err := New(Options{Dependencies: Dependencies{Resources: []ResourceFactory{
		resourceFactoryFunc(func() (Resource, error) {
			called = true
			return &recordingResource{}, nil
		}),
		nil,
	}}})
	if runtime != nil || err == nil {
		t.Fatalf("New with nil factory = (%v, %v), want nil runtime and error", runtime, err)
	}
	if called {
		t.Fatal("New invoked a factory before rejecting invalid options")
	}
}

func TestLifecycleStartsNotReady(t *testing.T) {
	runtime, err := New(Options{Dependencies: Dependencies{Resources: []ResourceFactory{
		resourceFactoryFunc(func() (Resource, error) { return &recordingResource{}, nil }),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = runtime.Close() }()

	if got := runtime.Phase(); got != ConnectionPhaseNotReady {
		t.Fatalf("Phase() = %v, want %v", got, ConnectionPhaseNotReady)
	}
}

func TestCloseIsIdempotentAndConcurrent(t *testing.T) {
	closeErr := errors.New("close second resource")
	var events []string
	var eventsMu sync.Mutex
	newResource := func(name string, err error) ResourceFactory {
		return resourceFactoryFunc(func() (Resource, error) {
			return &recordingResource{name: name, events: &events, eventsMu: &eventsMu, closeErr: err}, nil
		})
	}
	runtime, err := New(Options{Dependencies: Dependencies{Resources: []ResourceFactory{
		newResource("first", nil),
		newResource("second", closeErr),
	}}})
	if err != nil {
		t.Fatal(err)
	}

	const callers = 32
	errorsByCaller := make(chan error, callers)
	var callersWG sync.WaitGroup
	callersWG.Add(callers)
	for range callers {
		go func() {
			defer callersWG.Done()
			errorsByCaller <- runtime.Close()
		}()
	}
	callersWG.Wait()
	close(errorsByCaller)
	for closeResult := range errorsByCaller {
		if !errors.Is(closeResult, closeErr) {
			t.Fatalf("Close() error = %v, want joined %v", closeResult, closeErr)
		}
	}

	eventsMu.Lock()
	defer eventsMu.Unlock()
	if want := []string{"close:second", "close:first"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("close events = %v, want %v", events, want)
	}
	if got := runtime.Phase(); got != ConnectionPhaseDisconnected {
		t.Fatalf("Phase() after Close = %v, want %v", got, ConnectionPhaseDisconnected)
	}
}

func TestRemoteLoginOwnsEndpointThroughReceiverAndStartsLoading(t *testing.T) {
	identity := testRemoteIdentity(t)
	stream := &remoteTestStream{}
	endpoint := &remoteTestEndpoint{}
	receiver := &remoteTestReceiver{endpoint: endpoint}
	var events []string
	endpoint.events = &events
	receiver.events = &events

	runtime, err := NewRemote(context.Background(), Options{
		RemoteAddress:    "127.0.0.1:25565",
		Identity:         identity,
		ViewDistance:     protocol.LoginViewDistanceMin,
		ReceiverCapacity: 3,
		RemoteDependencies: RemoteDependencies{
			Dial: func(ctx context.Context, address string) (network.ClientPacketStream, error) {
				if ctx == nil || address != "127.0.0.1:25565" {
					t.Fatalf("Dial arguments = (%v, %q)", ctx, address)
				}
				events = append(events, "dial")
				return stream, nil
			},
			Login: func(ctx context.Context, gotStream network.ClientPacketStream, gotIdentity network.Identity, gotViewDistance uint8) (network.ClientEndpoint, uint64, error) {
				if ctx == nil || gotStream != stream || gotIdentity != identity || gotViewDistance != protocol.LoginViewDistanceMin {
					t.Fatalf("Login arguments do not match the remote options")
				}
				events = append(events, "login")
				return endpoint, 42, nil
			},
			NewReceiver: func(gotEndpoint network.ClientEndpoint, capacity int) (Receiver, error) {
				if gotEndpoint != endpoint || capacity != 3 {
					t.Fatalf("NewReceiver arguments = (%v, %d)", gotEndpoint, capacity)
				}
				events = append(events, "receiver")
				return receiver, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := runtime.Phase(); got != ConnectionPhaseLoading {
		t.Fatalf("Phase() = %v, want %v", got, ConnectionPhaseLoading)
	}
	if got := runtime.WorldSeed(); got != 42 {
		t.Fatalf("WorldSeed() = %d, want 42", got)
	}
	if want := []string{"dial", "login", "receiver"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("construction events = %v, want %v", events, want)
	}

	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"dial", "login", "receiver", "close:receiver", "close:endpoint"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("close events = %v, want %v", events, want)
	}
	if endpoint.closeCalls != 1 {
		t.Fatalf("endpoint close calls = %d, want 1", endpoint.closeCalls)
	}
}

func TestRemoteDialFailureDoesNotAttemptLogin(t *testing.T) {
	dialErr := errors.New("dial failed")
	calledLogin := false
	runtime, err := NewRemote(context.Background(), validRemoteOptions(RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) {
			return nil, dialErr
		},
		Login: func(context.Context, network.ClientPacketStream, network.Identity, uint8) (network.ClientEndpoint, uint64, error) {
			calledLogin = true
			return nil, 0, nil
		},
		NewReceiver: func(network.ClientEndpoint, int) (Receiver, error) {
			return nil, nil
		},
	}))
	if runtime != nil || !errors.Is(err, dialErr) {
		t.Fatalf("NewRemote() = (%v, %v), want nil runtime and dial error", runtime, err)
	}
	if calledLogin {
		t.Fatal("NewRemote attempted login after dialing failed")
	}
}

func TestRemoteCanceledContextPassesToDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runtime, err := NewRemote(ctx, validRemoteOptions(RemoteDependencies{
		Dial: func(got context.Context, _ string) (network.ClientPacketStream, error) {
			if got != ctx || !errors.Is(got.Err(), context.Canceled) {
				t.Fatalf("Dial context = %v, want canceled caller context", got)
			}
			return nil, got.Err()
		},
	}))
	if runtime != nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("NewRemote() = (%v, %v), want canceled context error", runtime, err)
	}
}

func TestRemoteLoginRejectionReliesOnLoginToCloseStream(t *testing.T) {
	rejectErr := &network.RemoteError{State: protocol.StateHandshake, Message: "version mismatch"}
	stream := &remoteTestStream{}
	runtime, err := NewRemote(context.Background(), validRemoteOptions(RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) { return stream, nil },
		Login: func(context.Context, network.ClientPacketStream, network.Identity, uint8) (network.ClientEndpoint, uint64, error) {
			// The established login contract closes a failed stream before returning.
			_ = stream.Close()
			return nil, 0, rejectErr
		},
		NewReceiver: func(network.ClientEndpoint, int) (Receiver, error) { return nil, nil },
	}))
	if runtime != nil || !errors.Is(err, rejectErr) {
		t.Fatalf("NewRemote() = (%v, %v), want nil runtime and login rejection", runtime, err)
	}
	if stream.closeCalls != 1 {
		t.Fatalf("stream close calls = %d, want login-owned close exactly once", stream.closeCalls)
	}
}

func TestRemoteV44LoginRejectsVersionAndClosesStreamOnce(t *testing.T) {
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
	if runtime != nil {
		t.Fatal("NewRemote returned a runtime after handshake rejection")
	}
	var remoteErr *network.RemoteError
	if !errors.As(err, &remoteErr) {
		t.Fatalf("NewRemote() error = %v, want network remote rejection", err)
	}
	if remoteErr.State != protocol.StateHandshake || remoteErr.Code != uint8(protocol.HandshakeVersionMismatch) {
		t.Fatalf("remote rejection = %+v, want handshake version mismatch", remoteErr)
	}
	if stream.closeCalls != 1 {
		t.Fatalf("stream close calls = %d, want 1", stream.closeCalls)
	}
	if len(stream.sent) != 1 {
		t.Fatalf("sent packets = %d, want one v44 handshake", len(stream.sent))
	}
	if stream.sent[0].state != protocol.StateHandshake {
		t.Fatalf("handshake state = %v, want %v", stream.sent[0].state, protocol.StateHandshake)
	}
	if hello, ok := stream.sent[0].packet.(protocol.ClientHello); !ok || hello.ProtocolVersion != protocol.ProtocolVersion {
		t.Fatalf("handshake packet = %#v, want current protocol hello", stream.sent[0].packet)
	}
}

func TestRemoteReceiverTerminalErrorDisconnectsAndReleasesEndpoint(t *testing.T) {
	terminalErr := errors.New("receiver terminal error")
	endpoint := &remoteTestEndpoint{}
	receiver := &remoteTestReceiver{endpoint: endpoint, err: terminalErr}
	runtime, err := NewRemote(context.Background(), validRemoteOptions(RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) { return &remoteTestStream{}, nil },
		Login: func(context.Context, network.ClientPacketStream, network.Identity, uint8) (network.ClientEndpoint, uint64, error) {
			return endpoint, 0, nil
		},
		NewReceiver: func(network.ClientEndpoint, int) (Receiver, error) { return receiver, nil },
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := runtime.Err(); !errors.Is(got, terminalErr) {
		t.Fatalf("Err() = %v, want receiver terminal error", got)
	}
	if got := runtime.Phase(); got != ConnectionPhaseDisconnected {
		t.Fatalf("Phase() = %v, want %v", got, ConnectionPhaseDisconnected)
	}
	if receiver.closeCalls != 1 || endpoint.closeCalls != 1 {
		t.Fatalf("receiver/endpoint closes = %d/%d, want 1/1", receiver.closeCalls, endpoint.closeCalls)
	}
}

func TestRemoteReceiverTerminalErrorPreservesCloseFailure(t *testing.T) {
	terminalErr := errors.New("receiver terminal error")
	closeErr := errors.New("receiver close error")
	receiver := &remoteTestReceiver{endpoint: &remoteTestEndpoint{}, err: terminalErr, closeErr: closeErr}
	runtime, err := NewRemote(context.Background(), validRemoteOptions(RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) { return &remoteTestStream{}, nil },
		Login: func(context.Context, network.ClientPacketStream, network.Identity, uint8) (network.ClientEndpoint, uint64, error) {
			return receiver.endpoint, 0, nil
		},
		NewReceiver: func(network.ClientEndpoint, int) (Receiver, error) { return receiver, nil },
	}))
	if err != nil {
		t.Fatal(err)
	}
	if got := runtime.Err(); !errors.Is(got, terminalErr) || !errors.Is(got, closeErr) {
		t.Fatalf("Err() = %v, want terminal and close errors", got)
	}
}

func TestRemoteOptionsRejectInvalidValuesBeforeDial(t *testing.T) {
	calledDial := false
	dependencies := RemoteDependencies{
		Dial: func(context.Context, string) (network.ClientPacketStream, error) {
			calledDial = true
			return nil, nil
		},
	}
	options := validRemoteOptions(dependencies)
	options.RemoteAddress = ""
	if runtime, err := NewRemote(context.Background(), options); runtime != nil || err == nil {
		t.Fatalf("NewRemote(empty address) = (%v, %v), want error", runtime, err)
	}
	options = validRemoteOptions(dependencies)
	options.ViewDistance = protocol.LoginViewDistanceMin - 1
	if runtime, err := NewRemote(context.Background(), options); runtime != nil || err == nil {
		t.Fatalf("NewRemote(invalid view distance) = (%v, %v), want error", runtime, err)
	}
	options = validRemoteOptions(dependencies)
	options.ReceiverCapacity = 0
	if runtime, err := NewRemote(context.Background(), options); runtime != nil || err == nil {
		t.Fatalf("NewRemote(zero receiver capacity) = (%v, %v), want error", runtime, err)
	}
	options = validRemoteOptions(dependencies)
	options.Identity.PlayerID = core.PlayerID{}
	if runtime, err := NewRemote(context.Background(), options); runtime != nil || err == nil {
		t.Fatalf("NewRemote(invalid identity) = (%v, %v), want error", runtime, err)
	}
	options = validRemoteOptions(dependencies)
	options.Identity.DisplayName = "not\nvalid"
	if runtime, err := NewRemote(context.Background(), options); runtime != nil || err == nil {
		t.Fatalf("NewRemote(invalid display name) = (%v, %v), want error", runtime, err)
	}
	if calledDial {
		t.Fatal("NewRemote opened a stream for invalid options")
	}
}

type resourceFactoryFunc func() (Resource, error)

func (factory resourceFactoryFunc) Open() (Resource, error) {
	return factory()
}

type recordingResource struct {
	name     string
	events   *[]string
	eventsMu *sync.Mutex
	closeErr error
}

func testRemoteIdentity(t *testing.T) network.Identity {
	t.Helper()
	playerID, err := core.ParsePlayerID("00112233-4455-4677-8899-aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	return network.Identity{PlayerID: playerID, DisplayName: "Chen"}
}

func validRemoteOptions(dependencies RemoteDependencies) Options {
	return Options{
		RemoteAddress:      "127.0.0.1:25565",
		Identity:           network.Identity{PlayerID: core.PlayerID{0, 1, 2, 3, 4, 5, 0x46, 7, 0x88, 9, 10, 11, 12, 13, 14, 15}, DisplayName: "Chen"},
		ViewDistance:       protocol.LoginViewDistanceMin,
		ReceiverCapacity:   1,
		RemoteDependencies: dependencies,
	}
}

type remoteTestStream struct{ closeCalls int }

func (stream *remoteTestStream) Send(context.Context, protocol.State, protocol.ClientPacket) error {
	return nil
}

func (stream *remoteTestStream) Recv(context.Context, protocol.State) (protocol.ServerPacket, error) {
	return nil, errors.New("unexpected receive")
}

func (stream *remoteTestStream) Close() error {
	stream.closeCalls++
	return nil
}

type sentClientPacket struct {
	state  protocol.State
	packet protocol.ClientPacket
}

type scriptedClientPacketStream struct {
	packets    []protocol.ServerPacket
	sent       []sentClientPacket
	closeCalls int
}

func (stream *scriptedClientPacketStream) Send(_ context.Context, state protocol.State, packet protocol.ClientPacket) error {
	stream.sent = append(stream.sent, sentClientPacket{state: state, packet: packet})
	return nil
}

func (stream *scriptedClientPacketStream) Recv(_ context.Context, _ protocol.State) (protocol.ServerPacket, error) {
	if len(stream.packets) == 0 {
		return nil, errors.New("unexpected receive")
	}
	packet := stream.packets[0]
	stream.packets = stream.packets[1:]
	return packet, nil
}

func (stream *scriptedClientPacketStream) Close() error {
	stream.closeCalls++
	return nil
}

type remoteTestEndpoint struct {
	closeCalls int
	events     *[]string
}

func (endpoint *remoteTestEndpoint) Send(context.Context, protocol.ClientMessage) error { return nil }

func (endpoint *remoteTestEndpoint) Recv(context.Context) (network.ServerMessage, error) {
	return nil, errors.New("unexpected receive")
}

func (endpoint *remoteTestEndpoint) Close() error {
	endpoint.closeCalls++
	if endpoint.events != nil {
		*endpoint.events = append(*endpoint.events, "close:endpoint")
	}
	return nil
}

type remoteTestReceiver struct {
	endpoint   *remoteTestEndpoint
	err        error
	closeErr   error
	closeCalls int
	events     *[]string
}

func (receiver *remoteTestReceiver) TryRecv() (network.ServerMessage, bool) { return nil, false }

func (receiver *remoteTestReceiver) Err() error { return receiver.err }

func (receiver *remoteTestReceiver) Close() error {
	receiver.closeCalls++
	if receiver.events != nil {
		*receiver.events = append(*receiver.events, "close:receiver")
	}
	return errors.Join(receiver.closeErr, receiver.endpoint.Close())
}

func (resource *recordingResource) Close() error {
	if resource.events != nil {
		if resource.eventsMu != nil {
			resource.eventsMu.Lock()
			defer resource.eventsMu.Unlock()
		}
		*resource.events = append(*resource.events, "close:"+resource.name)
	}
	return resource.closeErr
}
