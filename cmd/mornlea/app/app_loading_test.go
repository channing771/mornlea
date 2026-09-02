//go:build darwin

package app

// app_loading_test.go：世界加载相位的交互循环测试。加载循环的收敛/退出/错误
// 三分支与 RunInteractive 的 loading 路由都在这里钉住；复用 testkit 的离屏渲染
// 装配（排空桥事件需要真实渲染器）与交互测试的内存流对夹具。

import (
	"strings"
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
	seedLoadingChunks(app, readyChunks)
	return app
}

// seedLoadingChunks 把已就绪区块列镜像填到 count 个（上界 9，即视距 0 的目标
// 列数）。`startWorld` 装配成功会整体重建镜像，因此装配之后再播种才能让完成
// 判据在加载循环内成立。
func seedLoadingChunks(app *Application, count int) {
	for x := int32(-1); x <= 1; x++ {
		for z := int32(-1); z <= 1; z++ {
			if len(app.loadedChunks) >= count {
				return
			}
			app.loadedChunks[core.ChunkPos{X: x, Z: z}] = struct{}{}
		}
	}
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

// menuHandoffWindow 是「菜单循环→加载循环」交接缝测试的窗口替身：首次 Poll
// 时经注入回调模拟「进入游戏」点击走真实装配路径。桥动作事件无法注入
// （renderer 为具体类型），Poll 是菜单循环内测试可控的最早注入点。
type menuHandoffWindow struct {
	loadingTestWindow
	onFirstPoll func()
	firstPolled bool
}

func (window *menuHandoffWindow) Poll() {
	window.loadingTestWindow.Poll()
	if !window.firstPolled {
		window.firstPolled = true
		window.onFirstPoll()
	}
}

// TestRunInteractiveMenuLoopHandsOffToLoadingOnAssembly 锁定菜单→加载的交接缝：
// 菜单相位中装配成功（相位切到 loading）后菜单循环必须返回，外层路由把控制权
// 交给加载循环并在收敛后进入游戏相位。若菜单循环的 loading 返回条件被回退为
// 只判 game（或缺失），装配成功后客户端会滞留菜单循环空转到窗口关闭，本测试
// 以「收敛到 game」失败的形式变红。
func TestRunInteractiveMenuLoopHandsOffToLoadingOnAssembly(t *testing.T) {
	window := &menuHandoffWindow{loadingTestWindow: loadingTestWindow{closeAfterPolls: 6}}
	app := NewOffscreenRenderApplicationForTest(t, &IntegrationGlyphSource{}, 64, 64, config.Render{})
	identity := connectionTestIdentity()
	ticks, _ := newPerformanceRecorders(false)
	app.window = window
	app.menu = menuState{phase: MenuPhaseMenu, title: "Mornlea", version: menuVersion()}
	// startWorld 的完整装配依赖（内存 store/Host/登录流对），关闭 LOD 使远环
	// 接线零参与；视距 0 使目标列数为 9，装配后播种镜像即可在加载循环收敛。
	render := config.Render{}
	app.startupOptions = Options{
		Seed: 42, WorldPath: "unused", Identity: &identity,
		Render: render, StartAtMenu: true,
	}
	app.startupDeps = startWorldSuccessDeps(t)
	app.ticks = ticks

	startRequestedQuit := false
	window.onFirstPoll = func() {
		startRequestedQuit = app.handleMenuEvent(menuActionStart)
		// 装配成功已整体重建镜像：此刻播种，加载循环首帧即满足完成判据。
		seedLoadingChunks(app, 9)
	}

	if err := RunInteractive(app); err != nil {
		t.Fatalf("RunInteractive: %v", err)
	}
	if startRequestedQuit {
		t.Fatal("进入游戏不应请求退出")
	}
	if app.menu.phase != MenuPhaseGame {
		t.Fatalf("交接后 phase = %v，want game（菜单循环应把控制权交给加载循环）", app.menu.phase)
	}
	if !window.CursorCaptured() {
		t.Fatal("加载收敛应捕获光标")
	}
	if len(window.pushedUIStates) == 0 || !strings.Contains(string(window.pushedUIStates[0]), `"phase":"loading"`) {
		t.Fatalf("加载循环应至少呈现一帧 loading 下行文档，首份 = %v", window.pushedUIStates)
	}
}
