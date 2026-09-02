//go:build darwin

package app

import (
	"github.com/channing771/mornlea/internal/client"
)

// dev_capture.go 定义开发捕获服务在 app 侧的消费端契约与帧循环捕获泵。协调
// 器实现住在 `cmd/mornlea/devcapture`（consumer 侧接口模式，同
// `capture.SceneApplication` 先例的反向）：本包只声明接口，不感知也不得
// import 具体实现，依赖方向由 `internal/archcheck` 强制。

// captureSource 是捕获泵对像素来源的最小依赖，方法集与 `client.Window` 的
// 窗口合成捕获桥（client ABI v13）同形。收窄成私有接口让泵可以无头测试
// （测试注入计数替身）；生产交互路径由 `*client.Window` 隐式满足，无需任何
// 适配代码。
type captureSource interface {
	Capture() (pixels []byte, width, height int, err error)
}

// CaptureOutcome 是一次窗口捕获的终态：`Pixels` 为自上而下、无行 padding 的
// BGRA8 原始字节，长度恰为 `Width*Height*4`；`Err` 非 nil 时其余字段无意义。
//
// 所有权契约：经 `CaptureRequest.Done` 发送成功后，`Pixels` 即归协调器侧
// （服务 goroutine）所有，发送方（帧循环）不再持有或改写——遵循「跨
// goroutine 发送成功后的消息及其切片视为不可变」的全局约定；PNG/zip/GIF
// 编码因此可以安全地全部留在帧循环之外。
type CaptureOutcome struct {
	Pixels []byte
	Width  int
	Height int
	Err    error
}

// CaptureRequest 是一帧待执行捕获的凭据：协调器侧生成并持有待办语义，泵
// 消费后把结果经 `Done` 交回。`Done` 由协调器侧创建并拥有，泵只发送、不
// 关闭、不缓冲。
type CaptureRequest struct {
	Done chan<- CaptureOutcome
}

// CaptureCoordinator 是开发捕获服务注入给交互客户端的可空协调器。契约：
//
//   - `PendingCapture` MUST 非阻塞：帧循环在有协调器时每帧至多调用一次，
//     无待办时立即返回（零值， false），不做任何等待；
//   - 请求单 outstanding：同一时刻至多一个未交付请求，新请求只在上一个
//     交付完成后才产生，帧循环侧因此「每帧至多一次捕获、永不排队」；
//   - `CompleteCapture` MUST 非阻塞：调用即完成像素所有权移交（见
//     `CaptureOutcome`），实现不得在帧循环线程上做编码、落盘或任何有界
//     等待之外的工作。
//
// `err` 参数承载捕获桥的原始失败（含 `client.ErrCaptureUnavailable` 这类
// 运行期预期条件），实现必须原样上报，不得静默吞掉或伪造画面。
type CaptureCoordinator interface {
	PendingCapture() (CaptureRequest, bool)
	CompleteCapture(req CaptureRequest, pixels []byte, width, height int, err error)
}

// SetCaptureCoordinator 注入（或以 nil 清除）开发捕获协调器。供 main 在
// app 构造后、进入交互循环前装配：服务要消费 app 状态访问器，天然晚于
// app 存在。写入发生在启动序列的同一 goroutine 上，与帧循环的每帧读取
// 构成 happens-before；循环运行中并发改写是调用方编程错误，不加锁。
//
// 注入同时播种一次状态快照（`publishDevCaptureStatus`）：服务的 HTTP 面
// 在 `Start` 之后随时可能收到 `/status`，播种保证首个请求读到的是注入时刻
// 的相位与尺寸而不是构造零值。以 nil 清除后快照不再更新，但此时服务随
// 之不可达（main 侧仅在启动失败清除协调器，监听并未建立），没有观察者。
func (a *Application) SetCaptureCoordinator(coordinator CaptureCoordinator) {
	a.startupDeps.CaptureCoordinator = coordinator
	a.publishDevCaptureStatus()
}

// captureCoordinator 读取当前生效的协调器。存储复用 `NewWithDependencies`
// 保存的 `Dependencies` 注入载体：帧循环只读该槽位，未注入（零值 nil）时
// 整个泵只剩这一次字段读取与判空，即空闲零开销的全部成本。
func (a *Application) captureCoordinator() CaptureCoordinator {
	return a.startupDeps.CaptureCoordinator
}

// pumpDevCapture 是交互帧循环的捕获泵，由菜单、加载与游戏三处循环在
// `Window.Poll` 之后、渲染之前各调用一次。未注入协调器时早退；注入后每帧
// 先把相位与窗口尺寸发布进 /status 原子快照，再做一次非阻塞待办检查，
// 有待办时在当前线程（与 `Window.Poll` 同线程，
// 沿用窗口 FFI 的线程约束）同步执行恰好一次捕获并立即交付结果。泵不做
// 任何缓冲、不吞错、不重试：编码与打包都发生在协调器侧的帧循环之外，
// 帧循环只承受捕获本身的一次窗口合成拷贝开销。
func (a *Application) pumpDevCapture() {
	coordinator := a.captureCoordinator()
	if coordinator == nil {
		return
	}
	// 状态发布在待办检查之前：/status 读到的相位与尺寸对应本次待办捕获
	// 即将取样的同一帧窗口，观察面与画面不脱节。
	a.publishDevCaptureStatus()
	request, pending := coordinator.PendingCapture()
	if !pending {
		return
	}
	pixels, width, height, err := a.captureFrame()
	coordinator.CompleteCapture(request, pixels, width, height, err)
}

// publishDevCaptureStatus 在帧循环 goroutine 上把当前相位与窗口逻辑尺寸写入
// 原子快照，供 devcapture 的 `StatusSource` 适配器跨 goroutine 读取。发布只
// 在协调器已注入时发生（泵入口判空先行），未启用捕获时帧循环对这里零调用；
// 启用后的空闲帧成本是三次原子存储——无捕获桥调用、无分配，满足 spec 的
// 空闲零开销硬约束。快照是三个独立原子整数，观察者可能读到相邻两帧的组合；
// 对 /status 这种人类可读的观察面足够，不值得为此引入每帧分配的一致性结构。
// 无窗口（无头替身）时尺寸保持非正值，按 StatusSource 契约表示未知。
func (a *Application) publishDevCaptureStatus() {
	a.devCapturePhase.Store(int32(a.menu.phase))
	if a.window != nil {
		width, height := a.window.ContentSize()
		a.devCaptureWidth.Store(int32(width))
		a.devCaptureHeight.Store(int32(height))
	}
}

// Phase 返回快照中的菜单相位字符串，取值与 UI 桥 schema 的 `$defs/phase`
// 枚举一致（menu/settings/starting/paused/game）。它是 `devcapture.StatusSource`
// 的相位注入面：快照仅在注入捕获协调器后由帧循环维护，未注入时读到构造
// 零值——零值相位即游戏相位（`MenuPhaseGame`），与「非 StartAtMenu 路径
// 零值直接进游戏循环」的既有约定一致。并发语义：原子读取，可在任意
// goroutine 调用，读到的值至多落后帧循环一帧。
func (a *Application) Phase() string {
	return MenuPhase(a.devCapturePhase.Load()).uiPhase()
}

// WindowWidth 返回快照中的窗口逻辑点宽度；非正值（含未注入协调器时的零值）
// 表示未知，观察方按 StatusSource 契约呈现 unknown 而不是猜测尺寸。
func (a *Application) WindowWidth() int { return int(a.devCaptureWidth.Load()) }

// WindowHeight 返回快照中的窗口逻辑点高度；非正值表示未知。
func (a *Application) WindowHeight() int { return int(a.devCaptureHeight.Load()) }

// captureFrame 从当前窗口同步抓取一帧合成画面。窗口不具备捕获能力（无头
// 装配没有窗口、测试替身未实现捕获）时按 `client.ErrCaptureUnavailable`
// 交付，让协调器以可观察的失败收敛，而不是悬挂等待或 panic。
func (a *Application) captureFrame() ([]byte, int, int, error) {
	source, ok := a.window.(captureSource)
	if !ok {
		return nil, 0, 0, client.ErrCaptureUnavailable
	}
	return source.Capture()
}
