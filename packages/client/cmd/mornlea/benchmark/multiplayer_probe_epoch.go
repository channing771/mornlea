//go:build darwin

package benchmark

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/channing771/mornlea/packages/client/client"
	"github.com/channing771/mornlea/packages/shared/core"
)

const (
	benchmarkServerWarmupTicks     = 20
	benchmarkServerMeasuredTicks   = 200
	benchmarkServerInterestSamples = 8 * benchmarkServerMeasuredTicks
	benchmarkServerSignalCapacity  = benchmarkServerWarmupTicks + benchmarkServerMeasuredTicks + 16
	// benchmarkStreamingLatencyCapacity 是区块加载时延环形缓冲容量：视距
	// 梯度最大会话（视距 8 → 半径 9）的订阅方形 361 区块，容量取 8192 为
	// 重载（失败重试等）留出充裕余量；溢出时保留最新样本，报告仍完整。
	benchmarkStreamingLatencyCapacity = 8192
)

type benchmarkServerEpochPhase uint32

const (
	benchmarkServerEpochIdle benchmarkServerEpochPhase = iota
	benchmarkServerEpochWarmup
	benchmarkServerEpochMeasuring
	benchmarkServerEpochDone
)

type benchmarkServerTickSignal struct {
	measured  bool
	scheduled time.Time
	// published 是 observeScheduledTick 回调内打的时间戳，用于把
	// "服务端侧耗时"与"信号在缓冲里排队的耗时"分开。只在失败信息里读取。
	published time.Time
	// duration 是该 tick 自身的执行耗时，服务端已作为回调参数给出，
	// 此前未向下传递。只在失败信息里读取。
	duration time.Duration
}

type benchmarkServerInputBoundary func(context.Context, uint64, func() error) error

type benchmarkServerEpoch struct {
	phase         atomic.Uint32
	observedTicks atomic.Uint64
	measuredTicks atomic.Int64
	overflow      atomic.Bool
	signals       chan benchmarkServerTickSignal
	ticks         *client.LatencyRecorder
	interest      *client.LatencyRecorder
	// streaming 三件套只经 `StreamingObserver` 在服务端 step goroutine 上
	// 串行触达；探针侧读取唯一发生在服务端 run 结束（cleanup join）之后，
	// 因此无锁。与 `ticks`/`interest` 的串行纪律一致。
	//
	// streaming 与它们有一处刻意不同：不在 `beginMeasurement` 里 Reset——
	// 区块装载从会话登录即开始，多数发生在 warm-up 之前，指标族覆盖探针
	// 整个生命周期而非仅 measured 窗口。
	streaming    *client.LatencyRecorder
	loadBegins   map[core.ChunkKey]time.Time
	readyKeys    map[core.ChunkKey]struct{}
	streamingNow func() time.Time
}

func newBenchmarkServerEpoch() *benchmarkServerEpoch {
	return &benchmarkServerEpoch{
		signals:      make(chan benchmarkServerTickSignal, benchmarkServerSignalCapacity),
		ticks:        client.NewLatencyRecorder(512),
		interest:     client.NewLatencyRecorder(4096),
		streaming:    client.NewLatencyRecorder(benchmarkStreamingLatencyCapacity),
		loadBegins:   make(map[core.ChunkKey]time.Time),
		readyKeys:    make(map[core.ChunkKey]struct{}),
		streamingNow: time.Now,
	}
}

func (epoch *benchmarkServerEpoch) beginWarmup() {
	epoch.abortMeasurement()
	epoch.drainSignals()
	epoch.overflow.Store(false)
	epoch.phase.Store(uint32(benchmarkServerEpochWarmup))
}

func (epoch *benchmarkServerEpoch) beginMeasurement(
	ctx context.Context,
	inputBoundary benchmarkServerInputBoundary,
	armInput func() error,
) error {
	epoch.abortMeasurement()
	epoch.phase.Store(uint32(benchmarkServerEpochIdle))
	epoch.drainSignals()
	epoch.ticks.Reset()
	epoch.interest.Reset()
	epoch.measuredTicks.Store(0)
	epoch.overflow.Store(false)
	if ctx == nil {
		epoch.phase.Store(uint32(benchmarkServerEpochDone))
		return errors.New("缺少 measurement context")
	}
	armBoundary := epoch.observedTicks.Load()
	if inputBoundary == nil {
		epoch.phase.Store(uint32(benchmarkServerEpochDone))
		return errors.New("缺少服务端 input boundary")
	}
	err := inputBoundary(ctx, 1, func() error {
		if epoch.observedTicks.Load() != armBoundary {
			return errors.New("服务端 tick 在首组输入 arm 前完成")
		}
		if armInput != nil {
			if err := armInput(); err != nil {
				return err
			}
		}
		if epoch.observedTicks.Load() != armBoundary {
			return errors.New("服务端 tick 在首组输入 arm 期间完成")
		}
		epoch.phase.Store(uint32(benchmarkServerEpochMeasuring))
		return nil
	})
	if err != nil {
		epoch.phase.Store(uint32(benchmarkServerEpochDone))
		return err
	}
	if err := ctx.Err(); err != nil {
		epoch.phase.Store(uint32(benchmarkServerEpochDone))
		return err
	}
	return nil
}

func (epoch *benchmarkServerEpoch) abortMeasurement() {
	epoch.phase.CompareAndSwap(
		uint32(benchmarkServerEpochMeasuring),
		uint32(benchmarkServerEpochDone),
	)
}

func (epoch *benchmarkServerEpoch) measuring() bool {
	return benchmarkServerEpochPhase(epoch.phase.Load()) == benchmarkServerEpochMeasuring
}

func (epoch *benchmarkServerEpoch) observeInterest(duration time.Duration) {
	if epoch.measuring() {
		epoch.interest.Add(duration)
	}
}

// observeChunkStreaming 消费服务端 `StreamingObserver` 的当 tick 装载/就绪
// 键：装载请求记为 BeginLoading 起点，就绪事件与起点配对产出加载时延样本
// （CancelUnload 直达就绪的区块没有起点，只计入加载计数）。就绪键按区块
// 去重，稳态下即订阅并集实际驻留的区块数。
func (epoch *benchmarkServerEpoch) observeChunkStreaming(
	acquired []core.ChunkKey,
	ready []core.ChunkKey,
) {
	now := epoch.streamingNow()
	for _, key := range acquired {
		epoch.loadBegins[key] = now
	}
	for _, key := range ready {
		if begin, pending := epoch.loadBegins[key]; pending {
			delete(epoch.loadBegins, key)
			epoch.streaming.Add(now.Sub(begin))
		}
		epoch.readyKeys[key] = struct{}{}
	}
}

// streamingSummary 只能在服务端 run 结束后调用（见结构体注释的串行纪律）。
func (epoch *benchmarkServerEpoch) streamingSummary() client.StreamingSummary {
	return client.StreamingSummary{
		LoadedChunks: len(epoch.readyKeys),
		LoadLatency:  epoch.streaming.Summary(),
	}
}

func (epoch *benchmarkServerEpoch) observeTick(duration time.Duration) {
	epoch.observeScheduledTick(time.Now(), duration)
}

func (epoch *benchmarkServerEpoch) observeScheduledTick(
	scheduled time.Time,
	duration time.Duration,
) {
	epoch.observedTicks.Add(1)
	phase := benchmarkServerEpochPhase(epoch.phase.Load())
	if phase != benchmarkServerEpochWarmup && phase != benchmarkServerEpochMeasuring {
		return
	}
	measured := phase == benchmarkServerEpochMeasuring
	final := false
	if measured {
		epoch.ticks.Add(duration)
		final = epoch.measuredTicks.Add(1) == benchmarkServerMeasuredTicks
	}
	select {
	case epoch.signals <- benchmarkServerTickSignal{
		measured:  measured,
		scheduled: scheduled,
		published: time.Now(),
		duration:  duration,
	}:
	default:
		epoch.overflow.Store(true)
	}
	if final {
		epoch.phase.CompareAndSwap(
			uint32(benchmarkServerEpochMeasuring),
			uint32(benchmarkServerEpochDone),
		)
	}
}

func (epoch *benchmarkServerEpoch) drainSignals() {
	for {
		select {
		case <-epoch.signals:
		default:
			return
		}
	}
}
