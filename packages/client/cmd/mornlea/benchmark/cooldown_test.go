package benchmark

import (
	application "github.com/channing771/mornlea/packages/client/cmd/mornlea/app"
	"testing"

	"github.com/channing771/mornlea/packages/shared/config"
	"time"
)

func TestBenchmarkCooldownIsFixedAndNonZero(t *testing.T) {
	if benchmarkCooldown <= 0 {
		t.Fatalf("冷却时长 = %v，想要正值", benchmarkCooldown)
	}
	// 冷却只在阶段之间发生，不得长到改变 benchmark 的整体量级。
	if benchmarkCooldown > time.Minute {
		t.Fatalf("冷却时长 = %v，想要不超过 1 分钟", benchmarkCooldown)
	}
}

func TestBenchmarkCooldownDoesNotSubmitRenderWork(t *testing.T) {
	app := application.NewOffscreenRenderApplicationForTest(t, &application.IntegrationGlyphSource{}, 64, 64, config.Render{})
	framesBefore := app.Renderer().FrameCalls()

	// 冷却期间只允许窗口事件泵，不得提交任何渲染工作。
	runBenchmarkCooldown(app, 10*time.Millisecond)

	if got := app.Renderer().FrameCalls(); got != framesBefore {
		t.Fatalf("冷却期间触发 %d 次 render FFI,想要 %d", got, framesBefore)
	}
}

func TestBenchmarkCooldownIsRecordedInReport(t *testing.T) {
	report := benchmarkReportSkeleton()
	if got := report.CooldownSeconds; got != benchmarkCooldown.Seconds() {
		t.Fatalf("报告中的冷却秒数 = %v，想要 %v", got, benchmarkCooldown.Seconds())
	}
}
