package benchmark

import (
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"github.com/channing771/mornlea/packages/shared/config"
)

func TestScenarioV12GPUCompletionAmortizesOverFixedBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("短模式:重型测试由 CI 全量门禁运行")
	}
	app := application.NewOffscreenRenderApplicationForTest(t, &application.IntegrationGlyphSource{}, 64, 64, config.Render{})
	probe, err := newMultiplayerClientProbe(app)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(probe.Close)

	// 每次读钟推进 1ms：一批的总耗时因此恒为 1ms，样本值必须是 1ms / 批次数量。
	clockReads := 0
	batchCallsBefore := app.Renderer().BenchmarkBatchCalls()
	probe.now = func() time.Time {
		if got, want := app.Renderer().BenchmarkBatchCalls(), batchCallsBefore+clockReads+1; got != want {
			t.Fatalf("读钟前 benchmark batch FFI=%d,想要 %d", got, want)
		}
		clockReads++
		return time.Unix(0, int64(clockReads)*int64(time.Millisecond))
	}
	framesBefore := app.Renderer().FrameCalls()
	if err := probe.measureGPUCompletion(app); err != nil {
		t.Fatal(err)
	}

	summary := probe.gpuComplete.Summary()
	if summary.Samples != client.ScenarioV12GPUCompletionSamples {
		t.Fatalf("样本数 = %d，想要 %d", summary.Samples, client.ScenarioV12GPUCompletionSamples)
	}
	// 样本按 time.Duration 的整数纳秒摊薄，期望值同样取整。
	wantMS := float64(time.Millisecond/client.ScenarioV12GPUCompletionBatch) / float64(time.Millisecond)
	if summary.P50MS != wantMS {
		t.Fatalf("样本 p50 = %v ms，想要摊薄后的 %v ms", summary.P50MS, wantMS)
	}

	// 每个样本在首个读钟前 prepare、两个读钟之间 submit，一共两次 batch FFI。
	wantBatchCalls := client.ScenarioV12GPUCompletionSamples * 2
	if got := app.Renderer().BenchmarkBatchCalls() - batchCallsBefore; got != wantBatchCalls {
		t.Fatalf("GPU completion benchmark batch FFI=%d,想要 %d", got, wantBatchCalls)
	}
	if got := app.Renderer().FrameCalls() - framesBefore; got != 0 {
		t.Fatalf("GPU completion 不应调用 render_frame，实际=%d", got)
	}
	if clockReads != client.ScenarioV12GPUCompletionSamples*2 {
		t.Fatalf("读钟=%d,想要每样本一对(%d)", clockReads, client.ScenarioV12GPUCompletionSamples*2)
	}
}

func TestScenarioV12GPUCompletionBatchIsLargeEnoughToAmortizePollTick(t *testing.T) {
	// 实测 Poll 的固定节拍约为 1.28ms。批次数量必须让节拍摊薄到
	// 远小于 20% 判定阈值，否则分位数会重新被量化。
	const observedPollTickMS = 1.28
	const observedPerDrawMS = 0.09
	const relativeGateThreshold = 0.20

	amortized := observedPollTickMS / float64(client.ScenarioV12GPUCompletionBatch)
	// 摊薄后的节拍必须远小于相对判定阈值，否则分位数会重新被它主导。
	if share := amortized / observedPerDrawMS; share > relativeGateThreshold/2 {
		t.Fatalf(
			"节拍摊薄后占每次绘制成本的 %.1f%%，想要不超过判定阈值的一半（批次数量 = %d）",
			share*100, client.ScenarioV12GPUCompletionBatch,
		)
	}
}

func TestScenarioV12GPUCompletionBatchIsRecordedInReport(t *testing.T) {
	probe := &multiplayerClientProbe{
		encode:       client.NewLatencyRecorder(1),
		decode:       client.NewLatencyRecorder(1),
		rosterApply:  client.NewLatencyRecorder(1),
		interpolate:  client.NewLatencyRecorder(1),
		renderTiming: application.NewMultiplayerRenderTiming(1),
		gpuComplete:  client.NewLatencyRecorder(1),
	}
	summary := probe.Summary()
	if got := summary.RemoteGPUCompleteBatch; got != client.ScenarioV12GPUCompletionBatch {
		t.Fatalf("报告中的批次数量 = %d，想要 %d", got, client.ScenarioV12GPUCompletionBatch)
	}
}
