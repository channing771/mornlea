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
func (a *Application) SetCaptureCoordinator(coordinator CaptureCoordinator) {
	a.startupDeps.CaptureCoordinator = coordinator
}

// captureCoordinator 读取当前生效的协调器。存储复用 `NewWithDependencies`
// 保存的 `Dependencies` 注入载体：帧循环只读该槽位，未注入（零值 nil）时
// 整个泵只剩这一次字段读取与判空，即空闲零开销的全部成本。
func (a *Application) captureCoordinator() CaptureCoordinator {
	return a.startupDeps.CaptureCoordinator
}

// pumpDevCapture 是交互帧循环的捕获泵，由菜单与游戏两处循环在
// `Window.Poll` 之后、渲染之前各调用一次。未注入协调器时早退；注入后每帧
// 也只做一次非阻塞待办检查，有待办时在当前线程（与 `Window.Poll` 同线程，
// 沿用窗口 FFI 的线程约束）同步执行恰好一次捕获并立即交付结果。泵不做
// 任何缓冲、不吞错、不重试：编码与打包都发生在协调器侧的帧循环之外，
// 帧循环只承受捕获本身的一次窗口合成拷贝开销。
func (a *Application) pumpDevCapture() {
	coordinator := a.captureCoordinator()
	if coordinator == nil {
		return
	}
	request, pending := coordinator.PendingCapture()
	if !pending {
		return
	}
	pixels, width, height, err := a.captureFrame()
	coordinator.CompleteCapture(request, pixels, width, height, err)
}

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
