//go:build darwin

package app

import (
	"errors"
	"testing"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/packages/shared/network"
)

// fakeDevCaptureCoordinator 是捕获泵测试的计数协调器替身：按接口契约模拟
// 「单 outstanding 请求」——待办被消费一次后即回落为无待办。替身记录全部
// 调用计数与最后一次交付内容，供空闲零开销与逐字段交付断言复用。
type fakeDevCaptureCoordinator struct {
	pendingReq       CaptureRequest
	hasPending       bool
	pendingCalls     int
	completeCalls    int
	completedReq     CaptureRequest
	completedOutcome CaptureOutcome
}

// PendingCapture 返回并消费当前待办请求，非阻塞。
func (c *fakeDevCaptureCoordinator) PendingCapture() (CaptureRequest, bool) {
	c.pendingCalls++
	if c.hasPending {
		c.hasPending = false
		return c.pendingReq, true
	}
	return CaptureRequest{}, false
}

// CompleteCapture 记录一次交付；像素按约定归协调器侧所有，替身只保存引用
// 供断言，不触碰内容。
func (c *fakeDevCaptureCoordinator) CompleteCapture(
	req CaptureRequest, pixels []byte, width, height int, err error,
) {
	c.completeCalls++
	c.completedReq = req
	c.completedOutcome = CaptureOutcome{Pixels: pixels, Width: width, Height: height, Err: err}
}

// capturePumpWindow 是带捕获能力的单帧窗口替身：第一次 `Poll` 后即报告
// `ShouldClose`，使交互循环恰好执行一帧；`Capture` 交付预设结果并计数。
type capturePumpWindow struct {
	fakeInteractiveWindow
	polled       bool
	captureCalls int
	pixels       []byte
	width        int
	height       int
	captureErr   error
}

func (w *capturePumpWindow) ShouldClose() bool { return w.polled }

func (w *capturePumpWindow) Poll() { w.polled = true }

func (w *capturePumpWindow) Capture() ([]byte, int, int, error) {
	w.captureCalls++
	return w.pixels, w.width, w.height, w.captureErr
}

// newCapturePumpApplication 构造能无头跑通恰好一帧交互循环的离屏渲染应用，
// 替换窗口为单帧捕获替身并注入带一个待办请求的计数协调器；无 GPU 适配器时
// 随装配入口跳过。
func newCapturePumpApplication(
	t *testing.T,
	phase MenuPhase,
) (*Application, *capturePumpWindow, *fakeDevCaptureCoordinator) {
	t.Helper()
	application := newRemoteRenderApplication(t, &IntegrationGlyphSource{})
	application.menu.phase = phase
	window := &capturePumpWindow{
		pixels: []byte{1, 2, 3, 4, 5, 6, 7, 8}, width: 2, height: 1,
	}
	application.window = window
	clientEndpoint, serverEndpoint := network.NewMemoryPair(4)
	application.clientEndpoint = clientEndpoint
	application.receiver = client.NewReceiver(clientEndpoint, 4)
	t.Cleanup(func() { _ = serverEndpoint.Close() })
	coordinator := &fakeDevCaptureCoordinator{
		hasPending: true,
		pendingReq: CaptureRequest{Done: make(chan CaptureOutcome, 1)},
	}
	application.SetCaptureCoordinator(coordinator)
	return application, window, coordinator
}

// TestPumpDevCaptureWithoutCoordinatorKeepsFrameIdle 钉住「未注入协调器即
// 整体零参与」：帧循环唯一允许的额外开销是一次判空，不得触碰窗口。
func TestPumpDevCaptureWithoutCoordinatorKeepsFrameIdle(t *testing.T) {
	window := &capturePumpWindow{}
	application := &Application{window: window}

	application.pumpDevCapture()

	if window.captureCalls != 0 {
		t.Fatalf("无协调器时捕获调用 %d 次，want 0", window.captureCalls)
	}
}

// TestPumpDevCaptureIdleFrameChecksPendingOnly 钉住空闲帧零捕获调用：已注入
// 协调器但无待办时，每帧只做一次非阻塞待办检查，捕获源与交付通道都零调用。
func TestPumpDevCaptureIdleFrameChecksPendingOnly(t *testing.T) {
	coordinator := &fakeDevCaptureCoordinator{}
	window := &capturePumpWindow{}
	application := &Application{window: window}
	application.SetCaptureCoordinator(coordinator)

	application.pumpDevCapture()

	if coordinator.pendingCalls != 1 {
		t.Fatalf("空闲帧待办检查 %d 次，want 1", coordinator.pendingCalls)
	}
	if window.captureCalls != 0 || coordinator.completeCalls != 0 {
		t.Fatalf("空闲帧 capture=%d complete=%d，want 均 0",
			window.captureCalls, coordinator.completeCalls)
	}
}

// TestPumpDevCaptureDeliversPendingFrameExactlyOnce 钉住「有待办时每帧至多
// 一次捕获、像素宽高原样交付」：第二帧不得重复捕获或重复交付；泵自身无
// 状态，交付后不保留像素引用（所有权随交付整体移交协调器侧）。
func TestPumpDevCaptureDeliversPendingFrameExactlyOnce(t *testing.T) {
	coordinator := &fakeDevCaptureCoordinator{
		hasPending: true,
		pendingReq: CaptureRequest{Done: make(chan CaptureOutcome, 1)},
	}
	window := &capturePumpWindow{pixels: []byte{1, 2, 3, 4, 5, 6, 7, 8}, width: 2, height: 1}
	application := &Application{window: window}
	application.SetCaptureCoordinator(coordinator)

	application.pumpDevCapture()
	application.pumpDevCapture()

	if window.captureCalls != 1 {
		t.Fatalf("捕获调用 %d 次，want 每帧至多 1 次", window.captureCalls)
	}
	if coordinator.completeCalls != 1 {
		t.Fatalf("交付调用 %d 次，want 1", coordinator.completeCalls)
	}
	if coordinator.completedReq != coordinator.pendingReq {
		t.Fatalf("交付凭据与待办请求不是同一个")
	}
	outcome := coordinator.completedOutcome
	if string(outcome.Pixels) != string(window.pixels) ||
		outcome.Width != 2 || outcome.Height != 1 || outcome.Err != nil {
		t.Fatalf("交付结果 %+v，want 原样像素 2×1 且无错误", outcome)
	}
}

// TestPumpDevCaptureDeliversCaptureUnavailableAsIs 钉住错误语义：捕获桥报告
// `client.ErrCaptureUnavailable` 时必须原样进入交付结果，泵不得吞掉、重试
// 或伪造空图。
func TestPumpDevCaptureDeliversCaptureUnavailableAsIs(t *testing.T) {
	coordinator := &fakeDevCaptureCoordinator{
		hasPending: true,
		pendingReq: CaptureRequest{Done: make(chan CaptureOutcome, 1)},
	}
	window := &capturePumpWindow{captureErr: client.ErrCaptureUnavailable}
	application := &Application{window: window}
	application.SetCaptureCoordinator(coordinator)

	application.pumpDevCapture()

	if coordinator.completeCalls != 1 {
		t.Fatalf("交付调用 %d 次，want 1", coordinator.completeCalls)
	}
	outcome := coordinator.completedOutcome
	if !errors.Is(outcome.Err, client.ErrCaptureUnavailable) {
		t.Fatalf("交付错误 %v，want `client.ErrCaptureUnavailable` 原样", outcome.Err)
	}
	if outcome.Pixels != nil {
		t.Fatalf("失败交付不应携带像素，got %d 字节", len(outcome.Pixels))
	}
}

// TestPumpDevCaptureDeliversUnavailableWithoutCaptureSource 钉住收窄边界的
// 降级语义：窗口不具备捕获能力（如无头替身）时，待办请求必须以可观察的
// 「捕获不可用」失败交付收敛，而不是悬挂或 panic。
func TestPumpDevCaptureDeliversUnavailableWithoutCaptureSource(t *testing.T) {
	coordinator := &fakeDevCaptureCoordinator{
		hasPending: true,
		pendingReq: CaptureRequest{Done: make(chan CaptureOutcome, 1)},
	}
	application := &Application{window: &fakeInteractiveWindow{}}
	application.SetCaptureCoordinator(coordinator)

	application.pumpDevCapture()

	if coordinator.completeCalls != 1 {
		t.Fatalf("交付调用 %d 次，want 1", coordinator.completeCalls)
	}
	if !errors.Is(coordinator.completedOutcome.Err, client.ErrCaptureUnavailable) {
		t.Fatalf("无捕获源时交付错误 %v，want `client.ErrCaptureUnavailable`",
			coordinator.completedOutcome.Err)
	}
}

// TestRunInteractiveGameLoopPumpsPendingCaptureOnce 是循环接线集成断言：游戏
// 相位的单帧循环恰好走完「一次待办检查 → 一次捕获 → 立即交付」。
func TestRunInteractiveGameLoopPumpsPendingCaptureOnce(t *testing.T) {
	application, window, coordinator := newCapturePumpApplication(t, MenuPhaseGame)

	if err := RunInteractive(application); err != nil {
		t.Fatalf("RunInteractive: %v", err)
	}
	if coordinator.pendingCalls != 1 || window.captureCalls != 1 || coordinator.completeCalls != 1 {
		t.Fatalf("单帧调用计数 pending=%d capture=%d complete=%d，want 均 1",
			coordinator.pendingCalls, window.captureCalls, coordinator.completeCalls)
	}
	if coordinator.completedReq != coordinator.pendingReq {
		t.Fatalf("交付凭据与待办请求不是同一个")
	}
	outcome := coordinator.completedOutcome
	if string(outcome.Pixels) != string(window.pixels) ||
		outcome.Width != 2 || outcome.Height != 1 || outcome.Err != nil {
		t.Fatalf("交付结果 %+v，want 原样像素 2×1 且无错误", outcome)
	}
}

// TestRunInteractiveMenuLoopPumpsPendingCaptureOnce 是循环接线集成断言：主菜单
// 相位同样每帧恰好一次泵检查，像素在渲染前取自当前窗口合成图。
func TestRunInteractiveMenuLoopPumpsPendingCaptureOnce(t *testing.T) {
	application, window, coordinator := newCapturePumpApplication(t, MenuPhaseMenu)

	if err := RunInteractive(application); err != nil {
		t.Fatalf("RunInteractive: %v", err)
	}
	if coordinator.pendingCalls != 1 || window.captureCalls != 1 || coordinator.completeCalls != 1 {
		t.Fatalf("单帧调用计数 pending=%d capture=%d complete=%d，want 均 1",
			coordinator.pendingCalls, window.captureCalls, coordinator.completeCalls)
	}
	outcome := coordinator.completedOutcome
	if string(outcome.Pixels) != string(window.pixels) || outcome.Width != 2 || outcome.Height != 1 {
		t.Fatalf("交付结果 %+v，want 原样像素 2×1", outcome)
	}
}

// TestRunInteractiveLoadingLoopPumpsPendingCaptureOnce 是循环接线集成断言：世界
// 加载相位与菜单/游戏相位同位泵检查——每帧恰好一次「待办检查 → 捕获 → 交付」，
// 加载判据未满时单帧后随窗口关闭退出。
func TestRunInteractiveLoadingLoopPumpsPendingCaptureOnce(t *testing.T) {
	application, window, coordinator := newCapturePumpApplication(t, MenuPhaseLoading)

	if err := RunInteractive(application); err != nil {
		t.Fatalf("RunInteractive: %v", err)
	}
	if coordinator.pendingCalls != 1 || window.captureCalls != 1 || coordinator.completeCalls != 1 {
		t.Fatalf("单帧调用计数 pending=%d capture=%d complete=%d，want 均 1",
			coordinator.pendingCalls, window.captureCalls, coordinator.completeCalls)
	}
	outcome := coordinator.completedOutcome
	if string(outcome.Pixels) != string(window.pixels) || outcome.Width != 2 || outcome.Height != 1 {
		t.Fatalf("交付结果 %+v，want 原样像素 2×1", outcome)
	}
	if application.menu.phase != MenuPhaseLoading {
		t.Fatalf("判据未满相位应保持 loading，got %v", application.menu.phase)
	}
}

// TestSetCaptureCoordinatorSeedsStatusSnapshot 钉住播种语义：注入发生在服务
// `Start` 之前，首个 `/status` 读到的必须是注入时刻的相位与窗口尺寸，而不是
// 构造零值（game / 0×0）。
func TestSetCaptureCoordinatorSeedsStatusSnapshot(t *testing.T) {
	application := &Application{window: &fakeInteractiveWindow{}}
	application.menu.phase = MenuPhaseSettings

	application.SetCaptureCoordinator(&fakeDevCaptureCoordinator{})

	if application.Phase() != "settings" {
		t.Fatalf("注入后 Phase() = %q，want settings", application.Phase())
	}
	if application.WindowWidth() != 1 || application.WindowHeight() != 1 {
		t.Fatalf("注入后尺寸 (%d,%d)，want 替身报告的 (1,1)",
			application.WindowWidth(), application.WindowHeight())
	}
}

// TestPumpDevCapturePublishesStatusSnapshot 钉住发布语义：泵把当前帧的相位
// 同步进 /status 快照，导出访问器读到的是最近一次泵经过时的值。
func TestPumpDevCapturePublishesStatusSnapshot(t *testing.T) {
	window := &capturePumpWindow{}
	application := &Application{window: window}
	application.SetCaptureCoordinator(&fakeDevCaptureCoordinator{})

	application.menu.phase = menuPhasePaused
	application.pumpDevCapture()

	if application.Phase() != "paused" {
		t.Fatalf("泵后 Phase() = %q，want paused", application.Phase())
	}
	if application.WindowWidth() != 1 || application.WindowHeight() != 1 {
		t.Fatalf("泵后尺寸 (%d,%d)，want 替身报告的 (1,1)",
			application.WindowWidth(), application.WindowHeight())
	}
}

// TestDevCaptureStatusAccessorsDegradeWithoutWindow 钉住无头降级：无窗口的
// Application 注入协调器后，尺寸快照保持非正值（StatusSource 契约的「未知」），
// 相位仍可安全读取，发布路径不触碰空窗口。
func TestDevCaptureStatusAccessorsDegradeWithoutWindow(t *testing.T) {
	application := &Application{}
	application.SetCaptureCoordinator(&fakeDevCaptureCoordinator{})
	application.pumpDevCapture()

	if application.WindowWidth() != 0 || application.WindowHeight() != 0 {
		t.Fatalf("无窗口时尺寸 (%d,%d)，want 保持 0 表示未知",
			application.WindowWidth(), application.WindowHeight())
	}
	if application.Phase() != "game" {
		t.Fatalf("无窗口时 Phase() = %q，want 零值相位 game", application.Phase())
	}
}
