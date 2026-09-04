//go:build darwin

package benchmark

import (
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/client/render"
	"github.com/channing771/mornlea/packages/server/server"
)

// BenchmarkApplication 是 benchmark 对宿主应用状态所需能力的消费端接口：
// 预热、阶段测量、GPU 完成时间采样与传输关闭只经由这里声明的方法驱动应用，
// 不感知 `application.Application` 的具体类型。`*application.Application`
// 隐式实现本接口；方法集以 benchmark 生产代码的实际引用为准，不为对称性
// 添加方法，也不得反向扩散 app 的内部字段。装配入口 `RunBenchmark` 接收
// 具体类型：固定场景的加载等待判据（app 包导出的 `WaitUntilLoaded`）以
// 具体应用的加载管线为契约，测试侧则可用 `NewOffscreenRenderApplicationForTest`
// 返回的具体应用直接装配。
type BenchmarkApplication interface {
	// 帧驱动与无头渲染契约。
	Frame(drainMax, meshWorkMax int, elapsed time.Duration) (bool, error)
	Window() application.Window
	Renderer() *client.Renderer
	Scheduler() *render.SectionScheduler
	NameTagRenderer() *render.NameTagRenderer

	// 阶段测量读取的状态与镜像面。
	Camera() *client.Camera
	Server() *server.Server
	Mirror() *client.Mirror
	RemotePlayers() *client.RemotePlayers
	LastFrameStats() render.FrameStats

	// 多人探针的渲染计时注入与传输关闭链路。
	MultiplayerRenderTiming() *application.MultiplayerRenderTiming
	SetMultiplayerRenderTiming(timing *application.MultiplayerRenderTiming)
	SetMultiplayerRenderNow(now func() time.Time)
	CloseClientSession(cause error)
	ClientCloseErr() error
}
