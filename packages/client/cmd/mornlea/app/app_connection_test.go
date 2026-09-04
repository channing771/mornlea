//go:build darwin

package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/server/server"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/companion"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

func TestBenchmarkTCPDialFailureClosesListenerBeforeWaitingForAccept(t *testing.T) {
	dialErr := errors.New("injected benchmark dial failure")
	listener := newBenchmarkDialFailureListener()
	dial := func(context.Context, string) (network.ClientPacketStream, error) {
		<-listener.accepted
		return nil, dialErr
	}
	type result struct {
		endpoint network.ClientEndpoint
		err      error
	}
	returned := make(chan result, 1)
	go func() {
		endpoint, err := assembleBenchmarkObserverConnection(
			context.Background(), nil, "tcp", 0,
			func(string) (network.Listener, error) { return listener, nil },
			dial,
		)
		returned <- result{endpoint: endpoint, err: err}
	}()

	select {
	case got := <-returned:
		if got.endpoint != nil || !errors.Is(got.err, dialErr) {
			t.Fatalf("dial failure result = (%T, %v), want (nil, dial error)", got.endpoint, got.err)
		}
	case <-time.After(500 * time.Millisecond):
		_ = listener.Close()
		<-returned
		t.Fatal("benchmark TCP dial failure waited on Accept before closing listener")
	}
	if got := listener.closeCalls.Load(); got != 1 {
		t.Fatalf("listener Close calls = %d, want 1", got)
	}
	select {
	case <-listener.acceptDone:
	default:
		t.Fatal("benchmark TCP dial failure returned before Accept goroutine exited")
	}
}

type benchmarkDialFailureListener struct {
	accepted   chan struct{}
	closed     chan struct{}
	acceptDone chan struct{}
	closeOnce  sync.Once
	closeCalls atomic.Int32
}

func newBenchmarkDialFailureListener() *benchmarkDialFailureListener {
	return &benchmarkDialFailureListener{
		accepted:   make(chan struct{}),
		closed:     make(chan struct{}),
		acceptDone: make(chan struct{}),
	}
}

func (listener *benchmarkDialFailureListener) Accept(ctx context.Context) (network.ServerPacketStream, error) {
	close(listener.accepted)
	defer close(listener.acceptDone)
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-listener.closed:
		return nil, network.ErrClosed
	}
}

func (*benchmarkDialFailureListener) Addr() string { return "benchmark.invalid:1" }

func (listener *benchmarkDialFailureListener) Close() error {
	listener.closeOnce.Do(func() {
		listener.closeCalls.Add(1)
		close(listener.closed)
	})
	return nil
}

func TestApplicationConnectionRemoteAssemblyNeverOpensLocalStore(t *testing.T) {
	dialErr := errors.New("dial failed")
	storeCalls := 0
	dependencies := NewConnectionTestDependencies(t)
	dependencies.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
		storeCalls++
		return nil, errors.New("remote opened local store")
	}
	dependencies.DialTCP = func(context.Context, string) (network.ClientPacketStream, error) {
		return nil, dialErr
	}

	_, err := NewWithDependencies(remoteConnectionOptions(), dependencies)
	if !errors.Is(err, dialErr) {
		t.Fatalf("New error=%v, want dial failure", err)
	}
	if storeCalls != 0 {
		t.Fatalf("remote OpenStore calls=%d, want 0", storeCalls)
	}
}

func TestApplicationConnectionLocalNeverDialsTCP(t *testing.T) {
	openErr := errors.New("open local store failed")
	dialCalls := 0
	dependencies := NewConnectionTestDependencies(t)
	dependencies.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
		return nil, openErr
	}
	dependencies.DialTCP = func(context.Context, string) (network.ClientPacketStream, error) {
		dialCalls++
		return nil, errors.New("local dialed TCP")
	}

	_, err := NewWithDependencies(LocalConnectionTestOptions(), dependencies)
	if !errors.Is(err, openErr) {
		t.Fatalf("New error=%v, want local store failure", err)
	}
	if dialCalls != 0 {
		t.Fatalf("local DialTCP calls=%d, want 0", dialCalls)
	}
}

func TestApplicationConnectionRemoteDialFailurePrecedesWindow(t *testing.T) {
	dialErr := errors.New("dial failed")
	windowCalls := 0
	loginCalls := 0
	dependencies := NewConnectionTestDependencies(t)
	dependencies.DialTCP = func(context.Context, string) (network.ClientPacketStream, error) {
		return nil, dialErr
	}
	dependencies.LoginClient = func(
		context.Context, network.ClientPacketStream, network.Identity,
	) (network.ClientEndpoint, uint64, error) {
		loginCalls++
		return nil, 0, errors.New("login called after dial failure")
	}
	dependencies.NewWindow = connectionTestWindowFactory(&windowCalls)

	_, err := NewWithDependencies(remoteConnectionOptions(), dependencies)
	if !errors.Is(err, dialErr) {
		t.Fatalf("New error=%v, want dial failure", err)
	}
	if loginCalls != 0 || windowCalls != 0 {
		t.Fatalf("after dial failure login calls=%d window calls=%d, want 0/0", loginCalls, windowCalls)
	}
}

func TestApplicationConnectionRemoteLoginFailureClosesStreamBeforeWindow(t *testing.T) {
	loginErr := errors.New("login failed")
	stream := &connectionTestClientStream{}
	windowCalls := 0
	dependencies := NewConnectionTestDependencies(t)
	dependencies.DialTCP = func(context.Context, string) (network.ClientPacketStream, error) {
		return stream, nil
	}
	dependencies.LoginClient = func(
		context.Context, network.ClientPacketStream, network.Identity,
	) (network.ClientEndpoint, uint64, error) {
		return nil, 0, loginErr
	}
	dependencies.NewWindow = connectionTestWindowFactory(&windowCalls)

	_, err := NewWithDependencies(remoteConnectionOptions(), dependencies)
	if !errors.Is(err, loginErr) {
		t.Fatalf("New error=%v, want login failure", err)
	}
	if got := stream.closeCalls.Load(); got != 1 {
		t.Fatalf("remote stream Close calls=%d, want 1", got)
	}
	if windowCalls != 0 {
		t.Fatalf("remote login failure window calls=%d, want 0", windowCalls)
	}
}

func TestApplicationConnectionRemoteLoginSuccessReturnsOwnedApplicationAfterGraphics(t *testing.T) {
	rawEndpoint, _ := network.NewMemoryPair(1)
	endpoint := &ConnectionTestEndpoint{ClientEndpoint: rawEndpoint}
	t.Cleanup(func() { _ = rawEndpoint.Close() })
	stream := &connectionTestClientStream{}
	window := &connectionTestWindow{}
	loginComplete := false
	windowCalls := 0
	windowTitle := ""
	rendererCalls := 0
	dependencies := NewConnectionTestDependencies(t)
	dependencies.DialTCP = func(context.Context, string) (network.ClientPacketStream, error) {
		return stream, nil
	}
	dependencies.LoginClient = func(
		_ context.Context,
		got network.ClientPacketStream,
		_ network.Identity,
	) (network.ClientEndpoint, uint64, error) {
		if got != stream {
			t.Fatalf("LoginClient stream=%T, want dialed stream", got)
		}
		loginComplete = true
		return endpoint, 0, nil
	}
	dependencies.NewWindow = func(_, _ int, title string) (Window, error) {
		windowCalls++
		windowTitle = title
		if !loginComplete {
			t.Fatal("window created before remote login completed")
		}
		return window, nil
	}
	dependencies.NewWindowedRenderer = func(Window) (*client.Renderer, error) {
		rendererCalls++
		if !loginComplete || windowCalls != 1 {
			t.Fatal("renderer created before remote login and window")
		}
		renderer, err := client.NewRenderer(64, 64)
		if errors.Is(err, client.ErrNoGPUAdapter) {
			t.Skip("无 GPU 适配器")
		}
		return renderer, err
	}

	app, err := NewWithDependencies(remoteConnectionOptions(), dependencies)
	if err != nil {
		t.Fatalf("New remote success: %v", err)
	}
	if app == nil {
		t.Fatal("remote success returned nil Application")
	}
	if app.clientEndpoint != endpoint || app.receiver == nil {
		t.Fatalf("remote Application ownership endpoint=%T receiver=%p", app.clientEndpoint, app.receiver)
	}
	if app.host != nil || app.serverCancel != nil || app.serverDone != nil {
		t.Fatalf("remote Application acquired local Host lifecycle: host=%v cancel=%v done=%v", app.host, app.serverCancel, app.serverDone)
	}
	if len(app.remoteNameTags) != 0 || cap(app.remoteNameTags) != MaxFrameNameTags {
		t.Fatalf("remote target feedback tags=%d/%d，想要 0/%d",
			len(app.remoteNameTags), cap(app.remoteNameTags), MaxFrameNameTags)
	}
	if windowCalls != 1 || rendererCalls != 1 {
		t.Fatalf("remote success graphics calls window=%d renderer=%d, want 1/1", windowCalls, rendererCalls)
	}
	if windowTitle != applicationWindowTitle {
		t.Fatalf("interactive window title = %q, want %q", windowTitle, applicationWindowTitle)
	}
	if got := endpoint.closeCalls.Load(); got != 0 {
		t.Fatalf("live remote Application endpoint Close calls=%d, want 0", got)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("remote Application Close: %v", err)
	}
	if got := endpoint.closeCalls.Load(); got != 1 {
		t.Fatalf("remote Application endpoint Close calls=%d, want 1", got)
	}
	if got := window.closeCalls.Load(); got != 1 {
		t.Fatalf("remote Application window Close calls=%d, want 1", got)
	}
}

// stubApplicationHost 是 TestApplicationRemoteReflectsHostAndServerPresence
// 用的最小 Host 实现：三个方法都不会被调用，只用来在测试里制造
// 一个非 nil 的 host 值。
type stubApplicationHost struct{}

func (stubApplicationHost) Run(context.Context, network.Listener) error { return nil }
func (stubApplicationHost) AcceptStream(context.Context, network.ServerPacketStream) error {
	return nil
}
func (stubApplicationHost) Shutdown(context.Context) error { return nil }

// TestApplicationRemoteReflectsHostAndServerPresence 锁住 a.remote() 的判定：
// 它决定面板 physics/sim 组能不能写，取反会让单机变只读、真联机反而能写权威
// 参数，这条谓词必须有测试守着，不能只靠代码走查。
func TestApplicationRemoteReflectsHostAndServerPresence(t *testing.T) {
	tests := []struct {
		name string
		app  *Application
		want bool
	}{
		{name: "本地内嵌 Host（单机）", app: &Application{host: stubApplicationHost{}}, want: false},
		{name: "benchmark 内嵌可信 server", app: &Application{server: &server.Server{}}, want: false},
		{name: "host 与 server 均为 nil（真远程联机）", app: &Application{}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.app.remote(); got != test.want {
				t.Fatalf("remote() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestApplicationConnectionLocalHostFailureClosesStoreBeforeWindow(t *testing.T) {
	hostErr := errors.New("construct host failed")
	store := NewConnectionTestStore(42)
	windowCalls := 0
	memoryCalls := 0
	dependencies := NewConnectionTestDependencies(t)
	dependencies.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
		return store, nil
	}
	dependencies.NewHost = func(context.Context, server.Config, server.Generator, storage.WorldStore) (Host, error) {
		return nil, hostErr
	}
	dependencies.NewMemoryStreamPair = func(int) (network.ClientPacketStream, network.ServerPacketStream, error) {
		memoryCalls++
		return nil, nil, errors.New("memory pair called after host failure")
	}
	dependencies.NewWindow = connectionTestWindowFactory(&windowCalls)

	_, err := NewWithDependencies(LocalConnectionTestOptions(), dependencies)
	if !errors.Is(err, hostErr) {
		t.Fatalf("New error=%v, want host failure", err)
	}
	if got := store.closeCalls.Load(); got != 1 {
		t.Fatalf("local store Close calls=%d, want 1", got)
	}
	if memoryCalls != 0 || windowCalls != 0 {
		t.Fatalf("after host failure memory calls=%d window calls=%d, want 0/0", memoryCalls, windowCalls)
	}
}

func TestApplicationConnectionLocalMemoryFailureStopsHostAndClosesStoreBeforeWindow(t *testing.T) {
	memoryErr := errors.New("memory stream assembly failed")
	store := NewConnectionTestStore(42)
	host := newConnectionTestHost(store)
	windowCalls := 0
	dependencies := NewConnectionTestDependencies(t)
	dependencies.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
		return store, nil
	}
	dependencies.NewHost = func(context.Context, server.Config, server.Generator, storage.WorldStore) (Host, error) {
		return host, nil
	}
	dependencies.NewMemoryStreamPair = func(int) (network.ClientPacketStream, network.ServerPacketStream, error) {
		return nil, nil, memoryErr
	}
	dependencies.NewWindow = connectionTestWindowFactory(&windowCalls)

	_, err := NewWithDependencies(LocalConnectionTestOptions(), dependencies)
	if !errors.Is(err, memoryErr) {
		t.Fatalf("New error=%v, want memory assembly failure", err)
	}
	if got := host.shutdownCalls.Load(); got != 1 {
		t.Fatalf("local host Shutdown calls=%d, want 1", got)
	}
	if got := store.closeCalls.Load(); got != 1 {
		t.Fatalf("local store Close calls=%d, want 1", got)
	}
	if windowCalls != 0 {
		t.Fatalf("local memory failure window calls=%d, want 0", windowCalls)
	}
}

func TestApplicationConnectionLocalLoginFailureCleansStreamsHostAndStoreBeforeWindow(t *testing.T) {
	loginErr := errors.New("local login failed")
	store := NewConnectionTestStore(42)
	host := newConnectionTestHost(store)
	clientStream := &connectionTestClientStream{}
	serverStream := &connectionTestServerStream{}
	windowCalls := 0
	dependencies := NewConnectionTestDependencies(t)
	dependencies.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
		return store, nil
	}
	dependencies.NewHost = func(context.Context, server.Config, server.Generator, storage.WorldStore) (Host, error) {
		return host, nil
	}
	dependencies.NewMemoryStreamPair = func(int) (network.ClientPacketStream, network.ServerPacketStream, error) {
		return clientStream, serverStream, nil
	}
	dependencies.LoginClient = func(
		context.Context, network.ClientPacketStream, network.Identity,
	) (network.ClientEndpoint, uint64, error) {
		return nil, 0, loginErr
	}
	dependencies.NewWindow = connectionTestWindowFactory(&windowCalls)

	_, err := NewWithDependencies(LocalConnectionTestOptions(), dependencies)
	if !errors.Is(err, loginErr) {
		t.Fatalf("New error=%v, want local login failure", err)
	}
	if got := clientStream.closeCalls.Load(); got != 1 {
		t.Errorf("local client stream Close calls=%d, want 1", got)
	}
	if got := serverStream.closeCalls.Load(); got != 1 {
		t.Errorf("local server stream Close calls=%d, want 1", got)
	}
	if got := host.shutdownCalls.Load(); got != 1 {
		t.Errorf("local host Shutdown calls=%d, want 1", got)
	}
	if got := store.closeCalls.Load(); got != 1 {
		t.Errorf("local store Close calls=%d, want 1", got)
	}
	if windowCalls != 0 {
		t.Errorf("local login failure window calls=%d, want 0", windowCalls)
	}
}

func TestApplicationConnectionLocalAttachmentFailureCleansOwnedResourcesBeforeWindow(t *testing.T) {
	attachmentErr := errors.New("local attachment failed")
	store := NewConnectionTestStore(42)
	store.loadPlayerErr = attachmentErr
	windowCalls := 0
	dependencies := defaultDependencies()
	dependencies.OpenStore = func(context.Context, Options) (storage.WorldStore, error) {
		return store, nil
	}
	dependencies.NewWindow = connectionTestWindowFactory(&windowCalls)

	_, err := NewWithDependencies(LocalConnectionTestOptions(), dependencies)
	if err == nil {
		t.Fatal("New accepted local attachment failure")
	}
	if got := store.closeCalls.Load(); got != 1 {
		t.Fatalf("local attachment failure store Close calls=%d, want 1", got)
	}
	if windowCalls != 0 {
		t.Fatalf("local attachment failure window calls=%d, want 0", windowCalls)
	}
}

func remoteConnectionOptions() Options {
	identity := connectionTestIdentity()
	return Options{
		Connect: "example.invalid:25565", Identity: &identity, Render: config.Defaults().Render,
	}
}

func connectionTestIdentity() network.Identity {
	return network.Identity{
		PlayerID:    core.PlayerID{6: 0x40, 8: 0x80, 15: 1},
		DisplayName: "Test Player",
	}
}

type connectionTestHost struct {
	store         storage.WorldStore
	shutdownCalls atomic.Int32
}

func newConnectionTestHost(store storage.WorldStore) *connectionTestHost {
	return &connectionTestHost{store: store}
}

func (host *connectionTestHost) Run(ctx context.Context, _ network.Listener) error {
	<-ctx.Done()
	return ctx.Err()
}

func (*connectionTestHost) AcceptStream(ctx context.Context, _ network.ServerPacketStream) error {
	<-ctx.Done()
	return ctx.Err()
}

func (host *connectionTestHost) Shutdown(context.Context) error {
	host.shutdownCalls.Add(1)
	return host.store.Close()
}

type connectionTestClientStream struct{ closeCalls atomic.Int32 }

func (*connectionTestClientStream) Send(context.Context, network.State, network.ClientPacket) error {
	return nil
}
func (*connectionTestClientStream) Recv(context.Context, network.State) (network.ServerPacket, error) {
	return nil, network.ErrClosed
}
func (stream *connectionTestClientStream) Close() error {
	stream.closeCalls.Add(1)
	return nil
}

type connectionTestWindow struct {
	fakeInteractiveWindow
	closeCalls atomic.Int32
}

func (window *connectionTestWindow) Close() {
	window.closeCalls.Add(1)
}

type connectionTestServerStream struct{ closeCalls atomic.Int32 }

func (*connectionTestServerStream) Send(context.Context, network.State, network.ServerPacket) error {
	return nil
}
func (*connectionTestServerStream) Recv(context.Context, network.State) (network.ClientPacket, error) {
	return nil, network.ErrClosed
}
func (*connectionTestServerStream) Peer() string { return "test" }
func (stream *connectionTestServerStream) Close() error {
	stream.closeCalls.Add(1)
	return nil
}

func TestApplicationCloseReturnsPersistenceErrorAndReleasesOnce(t *testing.T) {
	persistenceErr := errors.New("持久化刷盘失败")
	serverDone := make(chan error, 1)
	serverDone <- persistenceErr

	cancelCalls := 0
	releaseCalls := 0
	app := &Application{
		serverCancel: func() { cancelCalls++ },
		serverDone:   serverDone,
		releaseResources: func() {
			releaseCalls++
		},
	}

	first := app.Close()
	second := app.Close()
	if !errors.Is(first, persistenceErr) {
		t.Fatalf("Close error=%v，想要包含 %v", first, persistenceErr)
	}
	if first != second {
		t.Fatalf("第二次 Close error=%v，不是缓存的第一次结果 %v", second, first)
	}
	if cancelCalls != 1 || releaseCalls != 1 {
		t.Fatalf("Close 调用次数 cancel=%d release=%d，想要各 1 次", cancelCalls, releaseCalls)
	}
}

func TestApplicationCloseCancelsBeforeWaitingAndSharesSuccessfulResult(t *testing.T) {
	serverDone := make(chan error)
	cancelObserved := make(chan struct{}, 1)
	releaseObserved := make(chan struct{}, 1)
	cancelCalls := 0
	releaseCalls := 0
	app := &Application{
		serverCancel: func() {
			cancelCalls++
			cancelObserved <- struct{}{}
		},
		serverDone: serverDone,
		releaseResources: func() {
			releaseCalls++
			releaseObserved <- struct{}{}
		},
	}

	const callers = 8
	start := make(chan struct{})
	results := make(chan error, callers)
	var callersDone sync.WaitGroup
	callersDone.Add(callers)
	for range callers {
		go func() {
			defer callersDone.Done()
			<-start
			results <- app.Close()
		}()
	}
	close(start)

	select {
	case <-cancelObserved:
	case <-time.After(time.Second):
		t.Fatal("Close 未在等待 serverDone 前调用 serverCancel")
	}
	select {
	case err := <-results:
		t.Fatalf("serverDone 前 Close 已返回: %v", err)
	case <-releaseObserved:
		t.Fatal("serverDone 前已释放资源")
	default:
	}
	select {
	case serverDone <- context.Canceled:
	case <-time.After(time.Second):
		t.Fatal("Close 未等待 serverDone")
	}

	callersDone.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Errorf("concurrent Close error=%v，plain context.Canceled 应为 nil", err)
		}
	}
	if err := app.Close(); err != nil {
		t.Fatalf("repeated Close error=%v", err)
	}
	if cancelCalls != 1 || releaseCalls != 1 {
		t.Fatalf("Close 调用次数 cancel=%d release=%d，想要各 1 次", cancelCalls, releaseCalls)
	}
}

func TestRunInteractiveReturnsReceiverDisconnectWithoutRendering(t *testing.T) {
	endpoint, _ := network.NewMemoryPair(1)
	receiver := client.NewReceiver(endpoint, 1)
	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for receiver.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	app := &Application{
		window:    &fakeInteractiveWindow{},
		receiver:  receiver,
		mirror:    client.NewMirror(),
		predictor: client.NewPredictor(),
	}
	if err := RunInteractive(app); !errors.Is(err, network.ErrClosed) {
		t.Fatalf("RunInteractive error=%v, want network.ErrClosed", err)
	}
}

func TestApplicationRoutesCompanionAndChatMessagesAndResetsOnDisconnect(t *testing.T) {
	app, serverEndpoint, endpoint, cancelCount := newRemoteProtocolApplication(t)
	app.companions = &client.Companions{}
	app.chatEvents = &client.ChatEvents{}
	spawn := applicationCompanionSpawn(1, 1, mgl32.Vec3{1, 64, 3})
	event := applicationChatEvent(1)
	sendInteractiveServerMessage(t, serverEndpoint, spawn)
	sendInteractiveServerMessage(t, serverEndpoint, event)
	app.DrainServerMessages(2)
	if got := app.companions.AppendPresentations(nil); len(got) != 1 || got[0].ID != spawn.ID {
		t.Fatalf("companion presentations = %+v", got)
	}
	if got := app.chatEvents.Events(nil); len(got) != 1 || got[0] != event {
		t.Fatalf("chat events = %+v", got)
	}

	// Apply 协议错误必须只关闭客户端会话，并原子清空两份镜像。
	sendInteractiveServerMessage(t, serverEndpoint, spawn)
	app.DrainServerMessages(1)
	if got := endpoint.closeCalls.Load(); got != 1 {
		t.Fatalf("protocol close count = %d, want 1", got)
	}
	if got := cancelCount(); got != 0 {
		t.Fatalf("server cancel count = %d, want 0", got)
	}
	if got := app.companions.AppendPresentations(nil); len(got) != 0 {
		t.Fatalf("companions after close = %+v", got)
	}
	if got := app.chatEvents.Events(nil); len(got) != 0 {
		t.Fatalf("chat events after close = %+v", got)
	}

	// transport 断开走同一清理路径。
	app, serverEndpoint, _, _ = newRemoteProtocolApplication(t)
	app.companions = &client.Companions{}
	app.chatEvents = &client.ChatEvents{}
	if err := app.companions.ApplySpawn(spawn); err != nil {
		t.Fatal(err)
	}
	if err := app.chatEvents.Apply(event); err != nil {
		t.Fatal(err)
	}
	if err := serverEndpoint.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for app.receiver.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if _, err := app.Frame(0, 0, 0); !errors.Is(err, network.ErrClosed) {
		t.Fatalf("frame error = %v, want network.ErrClosed", err)
	}
	if len(app.companions.AppendPresentations(nil)) != 0 || len(app.chatEvents.Events(nil)) != 0 {
		t.Fatal("disconnect left companion or chat state")
	}
}

func TestApplicationClosedSessionDoesNotDrainQueuedCompanionMessages(t *testing.T) {
	app, serverEndpoint, endpoint, _ := newRemoteProtocolApplication(t)
	app.companions = &client.Companions{}
	app.chatEvents = &client.ChatEvents{}
	first := applicationCompanionSpawn(1, 1, mgl32.Vec3{1, 64, 1})
	tailSpawn := applicationCompanionSpawn(2, 1, mgl32.Vec3{2, 64, 2})
	tailEvent := applicationChatEvent(1)
	for _, message := range []network.ServerMessage{first, first, tailSpawn, tailEvent} {
		sendInteractiveServerMessage(t, serverEndpoint, message)
	}

	app.DrainServerMessages(4)
	if !app.clientSessionClosed || endpoint.closeCalls.Load() != 1 {
		t.Fatalf("protocol close state=%v calls=%d", app.clientSessionClosed, endpoint.closeCalls.Load())
	}
	if len(app.companions.AppendPresentations(nil)) != 0 || len(app.chatEvents.Events(nil)) != 0 {
		t.Fatal("protocol close did not reset mirrors")
	}

	app.DrainServerMessages(4)
	if len(app.companions.AppendPresentations(nil)) != 0 || len(app.chatEvents.Events(nil)) != 0 {
		t.Fatal("closed session applied queued tail messages")
	}
	message, ok := app.receiver.TryRecv()
	if !ok || message != tailSpawn {
		t.Fatalf("queued tail head = %#v, %v; want untouched tail Spawn", message, ok)
	}
	message, ok = app.receiver.TryRecv()
	if !ok || message != tailEvent {
		t.Fatalf("queued tail second = %#v, %v; want untouched ChatEvent", message, ok)
	}
}

func TestApplicationAdvancesCompanionsExactlyOnceInFrameAndInteractiveLoops(t *testing.T) {
	t.Run("frame", func(t *testing.T) {
		app, serverEndpoint, _, _ := newRemoteProtocolApplication(t)
		app.companions = &client.Companions{}
		app.chatEvents = &client.ChatEvents{}
		spawn := applicationCompanionSpawn(1, 1, mgl32.Vec3{0, 64, 0})
		seedApplicationCompanion(t, app.companions, spawn)
		sendInteractiveServerMessage(t, serverEndpoint, network.CompanionStates{
			Tick: 3, States: []network.CompanionState{{
				ID: spawn.ID, Dimension: core.Overworld, Position: mgl32.Vec3{8, 64, 0},
			}},
		})
		rendered, err := app.Frame(1, 1, 25*time.Millisecond)
		if err != nil || rendered {
			t.Fatalf("frame = (%v,%v), want (false,nil)", rendered, err)
		}
		if got := app.companions.AppendPresentations(nil)[0].Position; got != (mgl32.Vec3{2, 64, 0}) {
			t.Fatalf("frame companion position = %v, want [2 64 0]", got)
		}
	})

	t.Run("interactive", func(t *testing.T) {
		app := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
		app.companions = &client.Companions{}
		app.chatEvents = &client.ChatEvents{}
		clientEndpoint, serverEndpoint := network.NewMemoryPair(4)
		app.clientEndpoint = clientEndpoint
		app.receiver = client.NewReceiver(clientEndpoint, 4)
		t.Cleanup(func() { _ = serverEndpoint.Close() })
		spawn := applicationCompanionSpawn(1, 1, mgl32.Vec3{0, 64, 0})
		seedApplicationCompanion(t, app.companions, spawn)
		if err := app.companions.ApplyStates(network.CompanionStates{
			Tick: 3, States: []network.CompanionState{{
				ID: spawn.ID, Dimension: core.Overworld, Position: mgl32.Vec3{8, 64, 0},
			}},
		}); err != nil {
			t.Fatal(err)
		}
		app.window = &oneFrameInteractiveWindow{delay: 25 * time.Millisecond}
		if err := RunInteractive(app); err != nil {
			t.Fatalf("RunInteractive: %v", err)
		}
		x := app.companions.AppendPresentations(nil)[0].Position[0]
		if x < 1.5 || x > 3 {
			t.Fatalf("interactive companion x = %f, want one elapsed advance", x)
		}
	})
}

func seedApplicationCompanion(t *testing.T, companions *client.Companions, spawn network.CompanionSpawn) {
	t.Helper()
	if err := companions.ApplySpawn(spawn); err != nil {
		t.Fatal(err)
	}
	if err := companions.ApplyStates(network.CompanionStates{
		Tick: 2, States: []network.CompanionState{{
			ID: spawn.ID, Dimension: core.Overworld, Position: mgl32.Vec3{4, 64, 0},
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func applicationCompanionSpawn(last byte, tick uint64, position mgl32.Vec3) network.CompanionSpawn {
	return network.CompanionSpawn{
		ID:   companion.ID{0: 0x12, 6: 0x40, 8: 0x80, 15: last},
		Name: "阿木", Tick: tick, Dimension: core.Overworld, Position: position,
	}
}

func applicationChatEvent(eventID uint64) network.ChatEvent {
	return network.ChatEvent{
		EventID:       eventID,
		PlayerID:      core.PlayerID{0: 0x12, 6: 0x40, 8: 0x80, 15: 9},
		PlayerName:    "Chen",
		CompanionID:   companion.ID{0: 0x12, 6: 0x40, 8: 0x80, 15: 1},
		CompanionName: "阿木",
		Kind:          network.ChatEventAccepted,
		Command:       "挖石头",
	}
}
