//go:build darwin

package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/shared/core"
)

func runBenchmarkTestInputBoundary(_ context.Context, _ uint64, action func() error) error {
	return action()
}

func observeMeasuredBenchmarkTick(
	t *testing.T,
	epoch *benchmarkServerEpoch,
	duration time.Duration,
) benchmarkServerTickSignal {
	t.Helper()
	epoch.observeTick(duration)
	signal := <-epoch.signals
	return signal
}

func TestBenchmarkServerEpochIgnoresWarmupAndStopsAtExactWindow(t *testing.T) {
	epoch := newBenchmarkServerEpoch()
	epoch.beginWarmup()
	for range benchmarkServerWarmupTicks {
		for range 8 {
			epoch.observeInterest(9 * time.Millisecond)
		}
		epoch.observeTick(9 * time.Millisecond)
		if signal := <-epoch.signals; signal.measured {
			t.Fatal("warm-up tick marked measured")
		}
	}
	if err := epoch.beginMeasurement(context.Background(), runBenchmarkTestInputBoundary, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	for tick := 1; tick <= benchmarkServerMeasuredTicks; tick++ {
		for range 8 {
			epoch.observeInterest(time.Duration(tick) * time.Microsecond)
		}
		if signal := observeMeasuredBenchmarkTick(
			t, epoch, time.Duration(tick)*time.Microsecond,
		); !signal.measured {
			t.Fatalf("tick %d not marked measured", tick)
		}
	}

	epoch.observeInterest(time.Second)
	epoch.observeTick(time.Second)
	if got := epoch.ticks.Summary().Samples; got != benchmarkServerMeasuredTicks {
		t.Fatalf("tick samples=%d want=%d", got, benchmarkServerMeasuredTicks)
	}
	if got := epoch.interest.Summary().Samples; got != benchmarkServerInterestSamples {
		t.Fatalf("interest samples=%d want=%d", got, benchmarkServerInterestSamples)
	}
	select {
	case signal := <-epoch.signals:
		t.Fatalf("done epoch emitted signal: %+v", signal)
	default:
	}
	if epoch.overflow.Load() {
		t.Fatal("complete epoch reported overflow")
	}
}

func TestBenchmarkServerEpochDropsStaleWarmupSignalsBeforeMeasurement(t *testing.T) {
	epoch := newBenchmarkServerEpoch()
	epoch.beginWarmup()
	epoch.observeTick(time.Millisecond)
	if err := epoch.beginMeasurement(context.Background(), runBenchmarkTestInputBoundary, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if signal := observeMeasuredBenchmarkTick(t, epoch, 2*time.Millisecond); !signal.measured {
		t.Fatalf("stale warm-up signal survived reset: %+v", signal)
	}
	if got := epoch.ticks.Summary().Samples; got != 1 {
		t.Fatalf("measured samples=%d want=1", got)
	}
}

func TestBenchmarkServerEpochReportsSignalOverflowWithoutBlocking(t *testing.T) {
	epoch := newBenchmarkServerEpoch()
	epoch.beginWarmup()
	for range cap(epoch.signals) + 1 {
		epoch.observeTick(time.Microsecond)
	}
	if !epoch.overflow.Load() {
		t.Fatal("signal overflow not reported")
	}
}

func TestBenchmarkServerEpochArmsInputBeforeMeasurementGate(t *testing.T) {
	epoch := newBenchmarkServerEpoch()
	epoch.beginWarmup()
	armed := false
	err := epoch.beginMeasurement(context.Background(), runBenchmarkTestInputBoundary, func() error {
		if epoch.measuring() {
			t.Fatal("measurement gate opened before input arm")
		}
		epoch.observeInterest(time.Second)
		armed = true
		return nil
	})
	if err != nil || !armed || !epoch.measuring() {
		t.Fatalf("beginMeasurement err=%v armed=%v measuring=%v", err, armed, epoch.measuring())
	}
	if signal := observeMeasuredBenchmarkTick(t, epoch, time.Millisecond); !signal.measured {
		t.Fatalf("first post-arm tick not measured: %+v", signal)
	}
	if got := epoch.ticks.Summary().Samples; got != 1 {
		t.Fatalf("post-arm samples=%d want=1", got)
	}
}

func TestBenchmarkServerEpochRejectsTickCompletedWhileArmingFirstInput(t *testing.T) {
	epoch := newBenchmarkServerEpoch()
	epoch.beginWarmup()
	err := epoch.beginMeasurement(context.Background(), runBenchmarkTestInputBoundary, func() error {
		epoch.observeTick(time.Millisecond)
		return nil
	})
	if err == nil {
		t.Fatal("beginMeasurement accepted a tick completed while arming input 1")
	}
	if epoch.measuring() {
		t.Fatal("failed input arm left measurement enabled")
	}
}

func TestBenchmarkServerEpochObserverDoesNotWaitForController(t *testing.T) {
	epoch := newBenchmarkServerEpoch()
	if err := epoch.beginMeasurement(context.Background(), runBenchmarkTestInputBoundary, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	returned := make(chan struct{})
	go func() {
		epoch.observeTick(time.Millisecond)
		close(returned)
	}()
	if signal := <-epoch.signals; !signal.measured {
		t.Fatalf("first tick signal=%+v", signal)
	}
	select {
	case <-returned:
	case <-time.After(25 * time.Millisecond):
		t.Fatal("tick observer waited for benchmark controller")
	}
}

func TestBenchmarkServerEpochPreservesScheduledTickTime(t *testing.T) {
	epoch := newBenchmarkServerEpoch()
	epoch.beginWarmup()
	scheduled := time.Now().Add(-25 * time.Millisecond)
	epoch.observeScheduledTick(scheduled, time.Millisecond)
	if signal := <-epoch.signals; !signal.scheduled.Equal(scheduled) {
		t.Fatalf("scheduled tick=%s want=%s", signal.scheduled, scheduled)
	}
}

func TestBenchmarkServerEpochPairsChunkStreamingBeginWithReady(t *testing.T) {
	epoch := newBenchmarkServerEpoch()
	// 固定时钟让时延断言有确定值：每次观测推进 5ms。
	tick := 0
	epoch.streamingNow = func() time.Time {
		tick++
		return time.Unix(0, int64(tick)*int64(5*time.Millisecond))
	}
	near := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 1, Z: -1}}
	far := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 30, Z: 30}}
	// t=5ms：两个区块进入 BeginLoading。
	epoch.observeChunkStreaming([]core.ChunkKey{near, far}, nil)
	// t=10ms：near 就绪 → 5ms 时延样本。
	epoch.observeChunkStreaming(nil, []core.ChunkKey{near})
	// t=15ms：far 就绪（10ms 样本），near 重复就绪不得重复计数。
	epoch.observeChunkStreaming(nil, []core.ChunkKey{far, near})
	summary := epoch.streamingSummary()
	if summary.LoadedChunks != 2 {
		t.Fatalf("loaded chunks=%d want=2（重复就绪须去重）", summary.LoadedChunks)
	}
	latency := summary.LoadLatency
	if latency.Samples != 2 {
		t.Fatalf("load latency samples=%d want=2", latency.Samples)
	}
	if latency.P50MS != 5 || latency.P95MS != 10 || latency.P99MS != 10 || latency.MaxMS != 10 {
		t.Fatalf("load latency=%+v want p50=5 p95/p99/max=10ms", latency)
	}
}

func TestBenchmarkServerEpochStreamingSkipsReadyWithoutBegin(t *testing.T) {
	epoch := newBenchmarkServerEpoch()
	base := time.Unix(0, 0)
	epoch.streamingNow = func() time.Time { return base }
	// CancelUnload 直接就绪的区块没有 BeginLoading 前置，不得产出零值时延。
	epoch.observeChunkStreaming(nil, []core.ChunkKey{{Pos: core.ChunkPos{X: 7}}})
	summary := epoch.streamingSummary()
	if summary.LoadedChunks != 1 {
		t.Fatalf("loaded chunks=%d want=1（未配对的就绪仍计入加载计数）", summary.LoadedChunks)
	}
	if summary.LoadLatency.Samples != 0 {
		t.Fatalf("无前置装载的就绪产出时延样本=%+v", summary.LoadLatency)
	}
}
