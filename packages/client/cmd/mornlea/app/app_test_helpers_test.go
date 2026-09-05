//go:build darwin

package app

// app_test_helpers_test.go：交互式窗口测试的共享替身与消息收发/镜像夹具，
// 供本包测试复用。跨包（capture/benchmark）共用的白盒装配入口收敛在
// testkit.go；本文件只保留 app 域测试私有、但被多个测试文件引用的夹具。

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/shared/config"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

type fakeInteractiveWindow struct {
	captured bool
	// pushedUIStates 记录 PushUIState 收到的下行状态原文,供「状态变化才
	// 推送」断言复用。
	pushedUIStates [][]byte
}

// PushUIState 记录一份下行 UI 状态(桥下行;替身不做任何呈现)。
func (window *fakeInteractiveWindow) PushUIState(payload []byte) {
	window.pushedUIStates = append(window.pushedUIStates, append([]byte(nil), payload...))
}

func (window *fakeInteractiveWindow) SetCursorCaptured(captured bool) {
	window.captured = captured
}
func (*fakeInteractiveWindow) CursorPos() (float64, float64) { return 0, 0 }
func (*fakeInteractiveWindow) ShouldClose() bool             { return false }
func (*fakeInteractiveWindow) Poll()                         {}
func (*fakeInteractiveWindow) DrainTextInput(dst []rune) ([]rune, bool) {
	return dst, false
}
func (*fakeInteractiveWindow) KeyDown(client.Key) bool     { return false }
func (*fakeInteractiveWindow) PrimaryButtonDown() bool     { return false }
func (*fakeInteractiveWindow) SecondaryButtonDown() bool   { return false }
func (window *fakeInteractiveWindow) CursorCaptured() bool { return window.captured }
func (*fakeInteractiveWindow) FramebufferSize() (int, int) { return 1, 1 }
func (*fakeInteractiveWindow) ContentSize() (int, int)     { return 1, 1 }
func (*fakeInteractiveWindow) SetContentSize(int, int)     {}
func (*fakeInteractiveWindow) CancelClose()                {}
func (*fakeInteractiveWindow) Close()                      {}
func (*fakeInteractiveWindow) Focus()                      {}

// newRemoteRenderApplication 构造 64×64 离屏渲染替身，供本包渲染主题测试
// 使用与既有用例相同的尺寸与零值渲染配置；跨包消费者直接使用 testkit 的
// 可参数化装配入口。
func newRemoteRenderApplication(t *testing.T, glyphs render.GlyphSource) *Application {
	t.Helper()
	application := NewOffscreenRenderApplicationForTest(t, glyphs, 64, 64, config.Render{})
	application.SetHostiles(&client.Hostiles{})
	return application
}

func newInteractiveTestApplication(
	t *testing.T,
) (*Application, network.ServerEndpoint) {
	t.Helper()
	clientEndpoint, serverEndpoint := network.NewMemoryPair(8)
	t.Cleanup(func() { _ = clientEndpoint.Close() })
	return &Application{
		clientEndpoint: clientEndpoint,
		receiver:       client.NewReceiver(clientEndpoint, 8),
		mirror:         client.NewMirror(),
		itemDrops:      client.NewItemDrops(),

		predictor:    client.NewPredictor(),
		serverCancel: func() {},
	}, serverEndpoint
}
func sendInteractiveServerMessage(
	t *testing.T,
	endpoint network.ServerEndpoint,
	message network.ServerMessage,
) {
	t.Helper()
	if err := endpoint.Send(context.Background(), message); err != nil {
		t.Fatalf("发送服务端消息: %v", err)
	}
	// The Application intentionally drains a non-blocking Receiver; let its sole
	// blocking reader hand this test message to the inbox before the frame drains.
	// 分包后本包测试会与 GPU 重型测试并行执行，1ms 在 -race 高负载下不足以
	// 保证交接，这里放宽到 50ms（交接本身通常在微秒到低毫秒量级）。
	time.Sleep(50 * time.Millisecond)
}

func loadInteractiveBlock(
	t *testing.T,
	app *Application,
	position core.BlockPos,
	block core.BlockID,
) {
	t.Helper()
	sections := make([]network.SectionData, core.SectionsPerChunk)
	for index := range sections {
		sections[index] = network.SectionData{
			Y: int32(index), Storage: network.SectionSingle, Single: core.AirID,
		}
	}
	chunk := position.Chunk()
	if _, err := app.mirror.Apply(network.ChunkSnapshot{
		Dimension: core.Overworld, Chunk: chunk, Revision: 1, Sections: sections,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.mirror.Apply(network.BlockChanges{
		Dimension: core.Overworld, Chunk: chunk, BaseRevision: 1, NewRevision: 2,
		Changes: []network.BlockChange{{Position: position, Block: block}},
	}); err != nil {
		t.Fatal(err)
	}
}

func receiveInteractiveClientMessage(
	t *testing.T,
	endpoint network.ServerEndpoint,
) network.ClientMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	message, err := endpoint.Recv(ctx)
	if err != nil {
		t.Fatalf("接收客户端消息: %v", err)
	}
	return message
}

func assertNoInteractiveClientMessage(t *testing.T, endpoint network.ServerEndpoint) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	message, err := endpoint.Recv(ctx)
	if err == nil {
		t.Fatalf("意外客户端消息: %#v", message)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("检查无客户端消息: %v", err)
	}
}

// `audioPlayerState` 为音频与水花主题测试构造最小合法的权威玩家状态。
func audioPlayerState(tick uint64, health, hunger uint8, reset bool) network.PlayerState {
	return network.PlayerState{
		ServerTick: tick, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true, Reset: reset, Health: health, Hunger: hunger,
	}
}

// gameTestAction 保留真实视图身份与生产守卫，测试只提供语义操作。
func gameTestAction(a *Application, op, area string, index int) {
	a.handleGameAction(client.UIGameAction{Token: a.buildGameUIState().Token, Op: op, Area: area, Index: index})
}

type gameEventDrainer struct {
	events []client.UIEvent
	drains int
}

func (d *gameEventDrainer) DrainUIEvents() []client.UIEvent {
	d.drains++
	events := d.events
	d.events = nil
	return events
}
