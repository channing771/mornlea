//go:build darwin

package main

// app_test_helpers_test.go：交互式窗口测试的共享替身与消息收发/镜像夹具，供 cmd/mornlea 交互类测试复用。

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
)

type fakeInteractiveWindow struct {
	captured bool
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

func newInteractiveTestApplication(
	t *testing.T,
) (*application, network.ServerEndpoint) {
	t.Helper()
	clientEndpoint, serverEndpoint := network.NewMemoryPair(8)
	t.Cleanup(func() { _ = clientEndpoint.Close() })
	return &application{
		clientEndpoint:  clientEndpoint,
		receiver:        client.NewReceiver(clientEndpoint, 8),
		mirror:          client.NewMirror(),
		itemDrops:       client.NewItemDrops(),
		inventorySource: -1,
		predictor:       client.NewPredictor(),
		serverCancel:    func() {},
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
	// The application intentionally drains a non-blocking Receiver; let its sole
	// blocking reader hand this test message to the inbox before the frame drains.
	time.Sleep(time.Millisecond)
}

func loadInteractiveBlock(
	t *testing.T,
	app *application,
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
