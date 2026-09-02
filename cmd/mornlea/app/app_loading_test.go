//go:build darwin

package app

// app_loading_test.go：世界加载相位的交互循环测试。加载循环的收敛/退出/错误
// 三分支与 RunInteractive 的 loading 路由都在这里钉住；复用 testkit 的离屏渲染
// 装配（排空桥事件需要真实渲染器）与交互测试的内存流对夹具。

import (
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
)

// loadingTestWindow 是加载相位测试的窗口替身：Poll 计数并在达到给定帧数后置
// ShouldClose，让各测试精确控制循环推进的帧数。
type loadingTestWindow struct {
	fakeInteractiveWindow
	closeAfterPolls int
	polls           int
}

func (window *loadingTestWindow) ShouldClose() bool    { return window.polls >= window.closeAfterPolls }
func (window *loadingTestWindow) Poll()                { window.polls++ }
func (*loadingTestWindow) FramebufferSize() (int, int) { return 64, 64 }

// newLoadingPhaseTestApp 构造加载相位的最小交互替身：真实离屏渲染器、视距 0
// （目标列数 9，见 `LoadedChunkTarget`）与指定数量的已就绪区块列镜像。mesher
// 与上传调度器均为新建零积压状态，因此填满目标列数即满足
// `ApplicationLoadComplete`，反之永远不收敛。
func newLoadingPhaseTestApp(t *testing.T, window Window, readyChunks int) *Application {
	t.Helper()
	app := NewOffscreenRenderApplicationForTest(t, &IntegrationGlyphSource{}, 64, 64, config.Render{})
	app.window = window
	app.menu.phase = MenuPhaseLoading
	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			if len(app.loadedChunks) >= readyChunks {
				return app
			}
			app.loadedChunks[core.ChunkPos{X: x, Z: z}] = struct{}{}
		}
	}
	if len(app.loadedChunks) < readyChunks {
		t.Fatalf("填充已就绪区块列越界: want %d", readyChunks)
	}
	return app
}

// TestRunLoadingPhaseConvergesToGameAndCapturesCursor 锁定加载收敛点：完成判据
// 首次成立即置游戏相位、捕获光标并返回 nil——光标捕获从装配成功迁移至此。
func TestRunLoadingPhaseConvergesToGameAndCapturesCursor(t *testing.T) {
	window := &loadingTestWindow{closeAfterPolls: 1}
	app := newLoadingPhaseTestApp(t, window, 9)

	if err := runLoadingPhase(app); err != nil {
		t.Fatalf("runLoadingPhase: %v", err)
	}
	if app.menu.phase != MenuPhaseGame {
		t.Fatalf("收敛后 phase = %v，want game", app.menu.phase)
	}
	if !window.CursorCaptured() {
		t.Fatal("加载收敛应捕获光标")
	}
}

// TestRunLoadingPhaseWindowCloseReturnsNil 锁定加载期的正常退出：窗口关闭返回
// nil，相位保持 loading 且光标未被捕获（判据未满不进入世界）。
func TestRunLoadingPhaseWindowCloseReturnsNil(t *testing.T) {
	window := &loadingTestWindow{closeAfterPolls: 1}
	app := newLoadingPhaseTestApp(t, window, 0)

	if err := runLoadingPhase(app); err != nil {
		t.Fatalf("runLoadingPhase: %v", err)
	}
	if app.menu.phase != MenuPhaseLoading {
		t.Fatalf("窗口关闭后 phase = %v，want 仍为 loading", app.menu.phase)
	}
	if window.CursorCaptured() {
		t.Fatal("判据未满不应捕获光标")
	}
}

// TestRunLoadingPhasePropagatesReceiverError 锁定加载期的接收器错误路径：`Frame`
// 内 `receiver.Err()` 成立时走既有 `CloseClientSession` 收摊并把错误上抛，与
// 游戏相位同一语义。
func TestRunLoadingPhasePropagatesReceiverError(t *testing.T) {
	window := &loadingTestWindow{closeAfterPolls: 8}
	app := newLoadingPhaseTestApp(t, window, 0)
	clientEndpoint, serverEndpoint := network.NewMemoryPair(4)
	app.clientEndpoint = clientEndpoint
	app.receiver = client.NewReceiver(clientEndpoint, 4)
	t.Cleanup(func() { _ = clientEndpoint.Close() })
	// 服务端断开：接收器读 goroutine 记录错误；轮询等待落地避免竞态。
	if err := serverEndpoint.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for app.receiver.Err() == nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if app.receiver.Err() == nil {
		t.Fatal("等待接收器错误超时")
	}

	if err := runLoadingPhase(app); err == nil {
		t.Fatal("接收器错误应从加载循环上抛")
	}
	if !app.clientSessionClosed {
		t.Fatal("接收器错误应经 CloseClientSession 收摊")
	}
}

// TestRunInteractiveRoutesLoadingPhaseToLoadingLoop 锁定相位路由：loading 相位
// 交给加载循环（而非菜单/游戏循环），收敛后相位为 game 并正常退出。
func TestRunInteractiveRoutesLoadingPhaseToLoadingLoop(t *testing.T) {
	window := &loadingTestWindow{closeAfterPolls: 1}
	app := newLoadingPhaseTestApp(t, window, 9)

	if err := RunInteractive(app); err != nil {
		t.Fatalf("RunInteractive: %v", err)
	}
	if app.menu.phase != MenuPhaseGame {
		t.Fatalf("收敛后 phase = %v，want game", app.menu.phase)
	}
	if !window.CursorCaptured() {
		t.Fatal("加载收敛应捕获光标")
	}
}
