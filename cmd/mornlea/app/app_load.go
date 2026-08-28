//go:build darwin

package app

import (
	"fmt"
	"time"

	"github.com/channing771/mornlea/internal/client"
	"github.com/channing771/mornlea/internal/config"
	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/render"
)

// MessageDrainMax 是无头路径每帧从服务端接收管线处理的消息上限。capture
// 抓帧与 benchmark 测量共用这一个常量：两侧的帧节奏都由本包 `Frame` 驱动，
// drain 预算若分叉，两侧对同一固定场景的加载耗时与消息吞吐就不再可比。
const MessageDrainMax = 4096

// LoadingApplication 是加载等待函数族的参数面：声明「加载完成判据」所需的
// 最小帧驱动与状态读取能力。`*Application` 隐式实现本接口；capture 的
// `SceneApplication` 方法集是本接口的超集，接口值可直接传入而无需类型断言；
// benchmark 则以具体 `*application.Application` 调用，两个消费域因此都不
// 感知超出本接口的加载判据面。
type LoadingApplication interface {
	// Frame 应用服务端消息并绘制一帧，是无头路径推进加载的唯一入口。
	Frame(drainMax, meshWorkMax int, elapsed time.Duration) (bool, error)
	// Window 返回窗口接口；无头形态为 nil，此时跳过事件泵。
	Window() Window
	// LoadedChunks 返回已就绪区块列的集合，用于统计初始快照进度。
	LoadedChunks() map[core.ChunkPos]struct{}
	// Mesher 返回网格化 worker 句柄，判据读取其排队与积压统计。
	Mesher() *client.Mesher
	// Scheduler 返回 mesh 上传调度器，判据读取待上传段数。
	Scheduler() *render.SectionScheduler
	// Render 返回生效配置快照，判据由其中的视距推导目标列数。
	Render() config.Render
}

// WaitUntilLoaded 推进帧循环直到初始区块快照与近环网格上传全部收敛，
// 返回快照首次到齐的时刻（benchmark 记录加载耗时；capture 只消费错误与
// 收敛本身）。该判据由 capture 的 golden 预加载与 LOD on/off control、
// benchmark 的固定场景加载共用：三方若对「近环就绪」出现第二套定义，
// golden update control 的保护语义就会失效。
func WaitUntilLoaded(app LoadingApplication, timeout time.Duration) (time.Duration, error) {
	deadline := time.Now().Add(timeout)
	started := time.Now()
	var snapshotDuration time.Duration
	wantedChunks := LoadedChunkTarget(app)
	lastLog := time.Time{}
	for {
		if time.Now().After(deadline) {
			return 0, fmt.Errorf("固定场景在 %s 内未完成加载：chunks=%d/%d mesher=%+v pending=%d",
				timeout, len(app.LoadedChunks()), wantedChunks, app.Mesher().Stats(),
				app.Scheduler().PendingUploads())
		}
		if app.Window() != nil {
			app.Window().Poll()
			if app.Window().ShouldClose() {
				app.Window().CancelClose()
			}
		}
		rendered, err := app.Frame(MessageDrainMax, MessageDrainMax, physics.FixedDelta)
		if err != nil {
			return 0, err
		}
		if !rendered {
			continue
		}
		if snapshotDuration == 0 && len(app.LoadedChunks()) == wantedChunks {
			snapshotDuration = time.Since(started)
		}
		if ApplicationLoadComplete(app, wantedChunks) {
			return snapshotDuration, nil
		}
		if time.Since(lastLog) >= 5*time.Second {
			stats := app.Mesher().Stats()
			fmt.Printf("加载中：chunks=%d/%d queued=%d active=%d ready=%d pending=%d\n",
				len(app.LoadedChunks()), wantedChunks, stats.QueuedJobs, stats.InFlightJobs,
				stats.ReadyResults, app.Scheduler().PendingUploads())
			lastLog = time.Now()
		}
	}
}

// LoadedChunkTarget 返回当前视距完整初始快照应包含的列数。服务端额外发送
// 一圈边界列，故半径是 `ViewDistance + 1`，这个定义与 `WaitUntilLoaded`
// 的历史收敛判据保持完全一致。
func LoadedChunkTarget(app LoadingApplication) int {
	viewDistance := app.Render().ViewDistance
	return (2*(viewDistance+1) + 1) * (2*(viewDistance+1) + 1)
}

// ApplicationLoadComplete 判断初始快照与近环网格上传是否全部收敛。远环
// LOD 的单独收敛仍由 capture 的场景成像负责；这里刻意保持 benchmark 的
// 原判据，避免 control 的预加载与正式抓帧有不同的近环就绪定义。
func ApplicationLoadComplete(app LoadingApplication, wantedChunks int) bool {
	stats := app.Mesher().Stats()
	return len(app.LoadedChunks()) == wantedChunks &&
		stats.QueuedJobs == 0 &&
		stats.InFlightJobs == 0 &&
		stats.ReadyResults == 0 &&
		stats.DirtySections == 0 &&
		app.Scheduler().PendingUploads() == 0
}

// WaitUntilLoadedPair 在同一 goroutine 交错推进 LOD on/off control，直到
// 两者的既有加载判据都成立。即便一侧先完成，也继续推进它直到另一侧完成，
// 因为其内嵌 Host 仍会持续发送消息，停止 drain 会让 `client.Receiver`
// 的有界 inbox 溢出。这里不并发调用 renderer，控制次序固定为 on 后 off。
func WaitUntilLoadedPair(lodOn, lodOff LoadingApplication, timeout time.Duration) error {
	return WaitUntilLoadedPairWithStep(
		lodOn, lodOff, timeout,
		func(app LoadingApplication) (bool, error) {
			rendered, err := app.Frame(
				MessageDrainMax, MessageDrainMax, physics.FixedDelta,
			)
			if err != nil || !rendered {
				return false, err
			}
			return ApplicationLoadComplete(app, LoadedChunkTarget(app)), nil
		},
	)
}

// WaitUntilLoadedPairWithStep 是 paired control 的最小调度内核。`step` 保留
// 仅用于无 GPU 单元测试的 seam；生产路径始终由 `WaitUntilLoadedPair` 注入
// 真实 `Frame` 方法，从而完整应用服务端消息和网格上传。
func WaitUntilLoadedPairWithStep(
	lodOn, lodOff LoadingApplication,
	timeout time.Duration,
	step func(LoadingApplication) (bool, error),
) error {
	deadline := time.Now().Add(timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("LOD on/off control 在 %s 内未完成加载", timeout)
		}
		lodOnReady, err := step(lodOn)
		if err != nil {
			return fmt.Errorf("LOD-on control: %w", err)
		}
		lodOffReady, err := step(lodOff)
		if err != nil {
			return fmt.Errorf("LOD-off control: %w", err)
		}
		if lodOnReady && lodOffReady {
			return nil
		}
	}
}
