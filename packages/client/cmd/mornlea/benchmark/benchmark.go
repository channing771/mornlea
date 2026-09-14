//go:build darwin

package benchmark

import (
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/server/server"
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	benchmarkSeed = application.BenchmarkSeed
	// benchmarkMessageDrainMax 是每帧服务端消息 drain 预算，单一取值住在
	// app 包（`MessageDrainMax`），与 capture 共用同一无头帧节奏契约。
	benchmarkMessageDrainMax = application.MessageDrainMax
	// scenarioVersion 是 benchmark producer 的场景身份。v22 → v23 的判定与
	// 历代同源：benchmark 的固定输入（七名远端玩家、零伙伴、不注入聊天）与
	// 被测世界（不注水、同一 seed、不含农业方块、固定世界方块与 v22 逐格
	// 一致）不变，但统一方块更新调度器与世界流式收尾又一次改变了**被测进程
	// 与被测服务端行为本身**——耕地湿度积压消费从 FIFO 改为统一调度器确定
	// 性全序，流体定时队列与随机抽样哈希链迁入统一调度器，多玩家服务端探
	// 针按会话视距（v40 登录协商的 2/4/6/8 梯度）启用真实区块订阅与流式
	// 加载，存档 I/O 按类别与 region 并行化并新增 region 句柄有界治理，报
	// 告随之新增 `streaming` 记录性指标族（只记录，不设阈值）。
	//
	// 本代叠加在 v22（自然短草）基线之上：分辨率、阶段时长、运动、样本、
	// 指标、绝对阈值与 `20%` 相对阈值全部不动；still/flying 主场景输入与阈
	// 值不受探针流式化的影响，跨 workload 报告只能经比较器显式 `22:23` 迁移
	// 并跳过相对回归判定。
	scenarioVersion = 23
)

var (
	warmupDuration = 10 * time.Second
	stillDuration  = 60 * time.Second
	flyDuration    = 120 * time.Second
	// benchmarkCooldown 是阶段之间的固定冷却，让 GPU 从满载回落。
	// 它只影响阶段之间的时间轴，不改变任何被采集指标的定义、样本数或阶段时长。
	benchmarkCooldown = 30 * time.Second
)

// runBenchmarkCooldown 在阶段之间等待固定时长，期间只泵送窗口事件，
// 不提交任何渲染工作，也不推进相机脚本。
func runBenchmarkCooldown(app BenchmarkApplication, duration time.Duration) {
	// 冷却是让系统回落的窗口：顺带回收上一阶段产生的对象，
	// 避免它们把后续阶段的 RSS 峰值推高。
	runtime.GC()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if app.Window() != nil {
			app.Window().Poll()
			if app.Window().ShouldClose() {
				app.Window().CancelClose()
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// printMemoryBreakdown 打印进程 RSS 峰值与 Go 运行时的内存构成。
//
// RSS 取自 ru_maxrss，是进程生命周期的历史峰值，回收之后不会下降；
// 把它与 Go 堆分开显示，才能判断峰值来自 Go 堆还是原生分配。
func printMemoryBreakdown(label string) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	const mib = 1 << 20
	rss, err := client.ProcessRSSBytes()
	if err != nil {
		return
	}
	fmt.Printf(
		"%s 内存：RSS 峰值 %.1fMiB｜Go 堆在用 %.1fMiB｜Go 堆保留 %.1fMiB｜Go 运行时合计 %.1fMiB｜非 Go %.1fMiB\n",
		label,
		float64(rss)/mib,
		float64(stats.HeapAlloc)/mib,
		float64(stats.HeapSys)/mib,
		float64(stats.Sys)/mib,
		(float64(rss)-float64(stats.Sys))/mib,
	)
}

// benchmarkReportSkeleton 返回只含固定运行参数的报告骨架，供测试断言这些参数被记录。
func benchmarkReportSkeleton() client.PerfReport {
	return client.PerfReport{
		ScenarioVersion: scenarioVersion,
		CooldownSeconds: benchmarkCooldown.Seconds(),
	}
}

// gpuCompletionMinSamples 返回该场景下 remote_gpu_complete 的最小样本数。
// v8–v11 逐次计时取 2048；v12–v14 改为批量分摊，样本数相应减少。
func gpuCompletionMinSamples(scenario int) int {
	switch {
	case scenario >= 12:
		return client.ScenarioV12GPUCompletionSamples
	case scenario >= 8:
		return client.ScenarioV8GPUCompletionSamples
	default:
		return 256
	}
}

// RunBenchmark 是 benchmark 性能测量的导出入口：加载固定场景后依次执行
// 预热、still/flying 两个测量阶段、GPU 完成时间采样与八会话服务端探针，
// 并把完整 `client.PerfReport` 写入 outputPath。app 由 main 装配后传入；
// 内部的帧循环与探针只依赖包内消费端接口 `BenchmarkApplication`。
func RunBenchmark(app *application.Application, outputPath string) error {
	width, height := app.FramebufferSize()
	if width != 2560 || height != 1440 {
		return fmt.Errorf("benchmark framebuffer=%dx%d，要求精确 2560x1440", width, height)
	}

	loadStarted := time.Now()
	snapshotDuration, err := application.WaitUntilLoaded(app, 5*time.Minute)
	if err != nil {
		return err
	}
	loadSeconds := time.Since(loadStarted).Seconds()
	fmt.Printf("固定场景加载完成，用时 %.2f 秒；开始预热\n", loadSeconds)

	if err := runWarmup(app, warmupDuration); err != nil {
		return err
	}
	runBenchmarkCooldown(app, benchmarkCooldown)
	multiplayerProbe, err := newMultiplayerClientProbe(app)
	if err != nil {
		return fmt.Errorf("创建多人客户端性能探针: %w", err)
	}
	defer multiplayerProbe.Close()
	app.Ticks().Reset()
	app.Saves().Reset()
	still, err := measurePhase(app, multiplayerProbe, "still", stillDuration, nil)
	if err != nil {
		return err
	}
	printMemoryBreakdown("still 后")
	runBenchmarkCooldown(app, benchmarkCooldown)
	flyingStart := app.Camera().Pos
	probe := server.NewTerrainProbe(benchmarkSeed)
	flying, err := measurePhase(app, multiplayerProbe, "flying", flyDuration, func(elapsed time.Duration) {
		seconds := float32(elapsed.Seconds())
		app.Camera().Pos[0] = flyingStart[0] + seconds*48
		app.Camera().Pos[2] = flyingStart[2] + float32(math.Sin(float64(seconds)*0.1))*96
		x := int32(math.Floor(float64(app.Camera().Pos[0])))
		z := int32(math.Floor(float64(app.Camera().Pos[2])))
		app.Camera().Pos[1] = float32(probe.HeightAt(x, z)) + 3.5
		app.Camera().Pitch = -float32(math.Pi)/2 + 0.02
		app.UpdateCenter()
	})
	if err != nil {
		return err
	}
	printMemoryBreakdown("flying 后")
	finalCenter := app.Center()
	if err := waitForBenchmarkCenterConsistency(
		app,
		finalCenter,
		app.ObserverFloor(),
		10*time.Second,
	); err != nil {
		return err
	}
	authoritativeHash, authoritativeRevision, authoritativeOK := app.Server().ChunkHash(
		core.Overworld, finalCenter,
	)
	mirrorHash, mirrorRevision, mirrorOK := app.Mirror().Hash(
		core.Overworld, finalCenter,
	)
	if !authoritativeOK || !mirrorOK || authoritativeRevision != mirrorRevision ||
		authoritativeHash != mirrorHash {
		return fmt.Errorf("最终 trusted observer 中心权威/镜像不一致: center=%+v server=(%x,%d,%v) mirror=(%x,%d,%v)",
			finalCenter,
			authoritativeHash, authoritativeRevision, authoritativeOK,
			mirrorHash, mirrorRevision, mirrorOK)
	}
	// GPU 采样不得紧接 flying 的满载尾部。
	runBenchmarkCooldown(app, benchmarkCooldown)
	if err := multiplayerProbe.measureGPUCompletionAfterTransportClose(app); err != nil {
		return fmt.Errorf("测量远端 GPU 完成时间: %w", err)
	}
	printMemoryBreakdown("GPU 采样后")
	// GPU 采样同样是满载阶段，其后也要冷却并回收，才轮到服务端探针。
	runBenchmarkCooldown(app, benchmarkCooldown)
	serverMultiplayer, ticks, streaming, err := measureMultiplayerServerProbe(10 * time.Second)
	if err != nil {
		return fmt.Errorf("测量八会话服务端: %w", err)
	}
	fmt.Printf(
		"streaming: loaded=%d load p50=%.3fms p95=%.3fms p99=%.3fms max=%.3fms RSS=%.1fMiB\n",
		streaming.LoadedChunks,
		streaming.LoadLatency.P50MS, streaming.LoadLatency.P95MS,
		streaming.LoadLatency.P99MS, streaming.LoadLatency.MaxMS,
		float64(streaming.PeakRSSBytes)/(1<<20),
	)
	multiplayer := multiplayerProbe.Summary()
	multiplayer.InterestDiff = serverMultiplayer.InterestDiff
	multiplayer.ServerOutboundBytes = serverMultiplayer.ServerOutboundBytes
	multiplayer.OutboxHighWater = serverMultiplayer.OutboxHighWater
	multiplayer.PlayerJobsHighWater = serverMultiplayer.PlayerJobsHighWater
	multiplayer.PlayerDoneHighWater = serverMultiplayer.PlayerDoneHighWater
	multiplayer.PeakRSSBytes = serverMultiplayer.PeakRSSBytes
	persistence := app.Saves().Summary()
	protocol, err := measureProtocolSummary()
	if err != nil {
		return err
	}
	playerPersistence, err := measurePlayerPersistenceSummary()
	if err != nil {
		return err
	}

	report := client.PerfReport{
		ScenarioVersion: scenarioVersion,
		Transport:       app.BenchmarkTransport(),
		Hardware:        hardwareID(),
		OS:              osID(),
		GoVersion:       runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH,
		GitCommit:       commandOutput("git", "rev-parse", "HEAD"),
		Framebuffer:     app.FramebufferLabel(),
		LoadSeconds:     loadSeconds,
		CooldownSeconds: benchmarkCooldown.Seconds(),
		SnapshotSeconds: snapshotDuration.Seconds(),
		Phases: map[string]client.PhaseSummary{
			"still":  still,
			"flying": flying,
		},
		Ticks:             ticks,
		Persistence:       persistence,
		Protocol:          protocol,
		PlayerPersistence: playerPersistence,
		Multiplayer:       multiplayer,
		Streaming:         streaming,
	}
	if err := writeBenchmarkReport(outputPath, report); err != nil {
		return err
	}
	for _, record := range benchmarkPerformanceRecords(report) {
		fmt.Println("性能记录:", record)
	}
	fmt.Printf("性能报告已写入 %s\n", outputPath)
	return nil
}
