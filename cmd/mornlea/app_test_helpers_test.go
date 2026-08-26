//go:build darwin

package main

// app_test_helpers_test.go：交互式窗口测试的共享替身、消息收发/镜像夹具与
// capture 跨主题夹具，供 `cmd/mornlea` 测试复用。

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/companion"
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

// `captureSceneByName` 为 capture 场景主题测试提供唯一名称查找，并同时钉住
// 场景表不存在重名项。
func captureSceneByName(t *testing.T, name string) captureScene {
	t.Helper()
	var matches []captureScene
	for _, scene := range captureScenes {
		if scene.Name == name {
			matches = append(matches, scene)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("capture scene %q count=%d，想要 1", name, len(matches))
	}
	return matches[0]
}

// `newCaptureAICompanionState` 为场景顺序与 AI 伙伴主题测试构造共同的最小应用状态。
func newCaptureAICompanionState() *application {
	return &application{
		remotePlayers:   client.NewRemotePlayers(),
		companions:      &client.Companions{},
		chatEvents:      &client.ChatEvents{},
		itemDrops:       client.NewItemDrops(),
		inventorySource: -1,
		panel:           &panelState{},
	}
}

// `assertCaptureAICompanionState` 为场景顺序与 AI 伙伴主题测试断言共同的确定性状态。
func assertCaptureAICompanionState(t *testing.T, app *application) {
	t.Helper()
	wantCamera := client.Camera{
		Pos: mgl32.Vec3{5.5, 3.2, 9.5}, Yaw: 0, Pitch: -0.05,
		FovY: mgl32.DegToRad(70), Aspect: float32(captureWidth) / captureHeight,
		Near: 0.1, Far: 2000,
	}
	if app.worldTimeTicks != 6000 || app.camera != wantCamera {
		t.Fatalf("固定环境 time=%d camera=%+v，想要 6000/%+v",
			app.worldTimeTicks, app.camera, wantCamera)
	}
	presentations := app.companions.AppendPresentations(nil)
	wantID := companion.ID{0: 0x42, 6: 0x40, 8: 0x80, 15: 0x14}
	if len(presentations) != 1 || presentations[0] != (client.CompanionPresentation{
		ID: wantID, Name: "阿木", Dimension: core.Overworld,
		Position: mgl32.Vec3{5.5, 1, 4}, Yaw: 3.1415927,
	}) {
		t.Fatalf("companion presentations=%+v", presentations)
	}
	events := app.chatEvents.Events(nil)
	wantEvent := network.ChatEvent{
		EventID: 1, PlayerID: core.PlayerID{0: 0x23, 6: 0x40, 8: 0x80, 15: 0x11},
		PlayerName: "旅人", CompanionID: wantID, CompanionName: "阿木",
		Kind: network.ChatEventAccepted, Command: "挖石头",
	}
	if len(events) != 1 || events[0] != wantEvent {
		t.Fatalf("chat events=%+v，想要 [%+v]", events, wantEvent)
	}
	if !app.chatInput.open || app.chatInput.text != "@阿木 挖石头" || app.chatInput.overflow {
		t.Fatalf("chat input=%+v", app.chatInput)
	}
}

// `audioPlayerState` 为音频与水花主题测试构造最小合法的权威玩家状态。
func audioPlayerState(tick uint64, health, hunger uint8, reset bool) network.PlayerState {
	return network.PlayerState{
		ServerTick: tick, Dimension: core.Overworld, Position: mgl32.Vec3{0.5, 10, 0.5},
		OnGround: true, Ready: true, Reset: reset, Health: health, Hunger: hunger,
	}
}
