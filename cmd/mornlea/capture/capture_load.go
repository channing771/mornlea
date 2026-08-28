package capture

import (
	"fmt"
	"time"

	"github.com/channing771/mornlea/internal/physics"
)

// capture_load.go 承载 capture 域的固定场景加载等待。它与 benchmark 域
// `benchmark_measure.go` 中的同名函数族是同一套判据的两个副本：函数族原本
// 住在 main 包被两条无头路径共用，capture 独立成包后不得再引用 main，而
// 副本落地时 app 包的导出面已冻结，因此暂时各持一份。收敛判据（视距目标
// 列数、mesher 四项归零、pending 归零）必须在两个副本间逐字保持一致——
// golden update control 的预加载与正式抓帧若对「近环就绪」出现第二套定义，
// control 的保护语义就会失效。benchmark 迁入独立包时，本函数族与其 drain
// 上限常量统一下沉 app 包导出，两份副本随之收敛为单一实现。

// waitUntilLoaded 推进帧循环直到初始区块快照与近环网格上传全部收敛，
// 返回快照首次到齐的时刻（用于 benchmark 的加载耗时记录；capture 只消费
// 错误与收敛本身）。
func waitUntilLoaded(app SceneApplication, timeout time.Duration) (time.Duration, error) {
	deadline := time.Now().Add(timeout)
	started := time.Now()
	var snapshotDuration time.Duration
	wantedChunks := loadedChunkTarget(app)
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
		rendered, err := app.Frame(captureDrainMax, captureDrainMax, physics.FixedDelta)
		if err != nil {
			return 0, err
		}
		if !rendered {
			continue
		}
		if snapshotDuration == 0 && len(app.LoadedChunks()) == wantedChunks {
			snapshotDuration = time.Since(started)
		}
		if applicationLoadComplete(app, wantedChunks) {
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

// loadedChunkTarget 返回当前视距完整初始快照应包含的列数。服务端额外发送
// 一圈边界列，故半径是 `ViewDistance + 1`，这个定义与 `waitUntilLoaded`
// 的历史收敛判据保持完全一致。
func loadedChunkTarget(app SceneApplication) int {
	viewDistance := app.Render().ViewDistance
	return (2*(viewDistance+1) + 1) * (2*(viewDistance+1) + 1)
}

// applicationLoadComplete 判断初始快照与近环网格上传是否全部收敛。远环
// LOD 的单独收敛仍由 `captureSceneImage` 负责；这里刻意保持 benchmark 的
// 原判据，避免 control 的预加载与正式抓帧有不同的近环就绪定义。
func applicationLoadComplete(app SceneApplication, wantedChunks int) bool {
	stats := app.Mesher().Stats()
	return len(app.LoadedChunks()) == wantedChunks &&
		stats.QueuedJobs == 0 &&
		stats.InFlightJobs == 0 &&
		stats.ReadyResults == 0 &&
		stats.DirtySections == 0 &&
		app.Scheduler().PendingUploads() == 0
}

// waitUntilLoadedPair 在同一 goroutine 交错推进 LOD on/off control，直到
// 两者的既有加载判据都成立。即便一侧先完成，也继续推进它直到另一侧完成，
// 因为其内嵌 Host 仍会持续发送消息，停止 drain 会让 `client.Receiver`
// 的有界 inbox 溢出。这里不并发调用 renderer，控制次序固定为 on 后 off。
func waitUntilLoadedPair(lodOn, lodOff SceneApplication, timeout time.Duration) error {
	return waitUntilLoadedPairWithStep(
		lodOn, lodOff, timeout,
		func(app SceneApplication) (bool, error) {
			rendered, err := app.Frame(
				captureDrainMax, captureDrainMax, physics.FixedDelta,
			)
			if err != nil || !rendered {
				return false, err
			}
			return applicationLoadComplete(app, loadedChunkTarget(app)), nil
		},
	)
}

// waitUntilLoadedPairWithStep 是 paired control 的最小调度内核。`step` 保留
// 仅用于无 GPU 单元测试的 seam；生产路径始终由 `waitUntilLoadedPair` 注入
// 真实 `app.Frame`，从而完整应用服务端消息和网格上传。
func waitUntilLoadedPairWithStep(
	lodOn, lodOff SceneApplication,
	timeout time.Duration,
	step func(SceneApplication) (bool, error),
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
