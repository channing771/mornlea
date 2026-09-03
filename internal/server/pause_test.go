package server

// pause_test.go：权威暂停门的行为契约测试。契约（OpenSpec pause-menu）：
// 暂停期间整个权威 tick 不存在——世界时间、随机 tick 与持久化调度全部停走；
// Resume 后模拟从冻结值确定性续跑，暂停段不改变任何后续结算结果。

import (
	"context"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

const (
	// pauseObserverDwell 是暂停窗口的最小观察时长。RunTicks 的调度周期为
	// 50ms，该值覆盖多个周期；只要期间观察到的已执行 tick 数不增加，就证明
	// 「整个 tick 不存在」而非「tick 空转」。
	pauseObserverDwell = 300 * time.Millisecond
)

// TestPauseFreezesWorldTimeAcrossExplicitSteps 验证显式推进点在暂停期被
// 短路（真实调度循环的对应性质由 RunTicks 观察者测试覆盖）。
func TestPauseFreezesWorldTimeAcrossExplicitSteps(t *testing.T) {
	running := newDefaultTestServer(t)
	warmup := running.StepForTest()
	if warmup.WorldTimeTicks == 0 {
		t.Fatalf("预热 tick 世界时间 = %d，想要非零", warmup.WorldTimeTicks)
	}
	frozenAt := running.TickCount()

	running.Pause()
	for range 4 {
		result := running.StepForTest()
		if !reflect.DeepEqual(result, contract.TickResult{}) {
			t.Fatalf("暂停期返回了非空 tick 结果: %+v", result)
		}
		if got := running.TickCount(); got != frozenAt {
			t.Fatalf("暂停期世界推进: 冻结于 %d，实际 %d", frozenAt, got)
		}
	}
}

func TestPauseIsIdempotent(t *testing.T) {
	running := newDefaultTestServer(t)
	warmup := running.StepForTest()
	frozenAt := running.TickCount()
	if warmup.Tick != frozenAt {
		t.Fatalf("预热后 tick 计数 = %d，与世界计数 %d 不一致", warmup.Tick, frozenAt)
	}

	running.Pause()
	beforeRepeat := running.TickCount()
	running.Pause()
	running.Pause()
	// 幂等意味着重复置位不排队、不计数：不会出现“双重暂停”或额外状态迁移，
	// 世界仍停在同一个冻结点。
	if got := running.TickCount(); got != beforeRepeat {
		t.Fatalf("重复 Pause 改变了状态: %d -> %d", beforeRepeat, got)
	}
	if result := running.StepForTest(); !reflect.DeepEqual(result, contract.TickResult{}) {
		t.Fatalf("重复 Pause 后 tick 未被跳过: %+v", result)
	}

	running.Resume()
	resumed := running.StepForTest()
	if resumed.Tick != beforeRepeat+1 {
		t.Fatalf("恢复后首个 tick = %d，想要 %d", resumed.Tick, beforeRepeat+1)
	}
}

func TestResumeContinuesWorldTimeFromFrozenValue(t *testing.T) {
	running := newDefaultTestServer(t)
	const warmups = 3
	var lastWorldTime uint64
	for range warmups {
		lastWorldTime = running.StepForTest().WorldTimeTicks
	}
	frozenTick := running.TickCount()

	running.Pause()
	for range 3 {
		running.StepForTest()
	}
	running.Resume()

	for offset := uint64(1); offset <= 4; offset++ {
		result := running.StepForTest()
		wantTick, wantWorldTime := frozenTick+offset, lastWorldTime+offset
		if result.Tick != wantTick || result.WorldTimeTicks != wantWorldTime {
			t.Fatalf("恢复后第 %d 个 tick = (%d,%d)，想要 (%d,%d)",
				offset, result.Tick, result.WorldTimeTicks, wantTick, wantWorldTime)
		}
	}
}

// TestPausedIntervalPreservesDeterministicContinuation 验证暂停段不改变
// 确定性续跑：同种子构造两个孪生世界，一个经历 Pause→Resume、一个不暂停，
// 在相同绝对 tick 处的逐 tick 结算与区块状态必须一致。
func TestPausedIntervalPreservesDeterministicContinuation(t *testing.T) {
	const (
		warmupSteps   = 2
		pausedCycles  = 4
		replaySteps   = 6
		inputSequence = uint64(7)
	)
	paused, unpaused := newDeterminismTwinServers(t)
	for _, twin := range []*Server{paused, unpaused} {
		readySpawnChunkForDeterministicReplay(t, twin)
	}
	var baselineHash [32]byte
	var baselineRevision uint64
	for index, twin := range []*Server{paused, unpaused} {
		for range warmupSteps {
			twin.StepForTest()
		}
		hash, revision, ok := twin.ChunkHash(core.Overworld, core.ChunkPos{})
		if !ok {
			t.Fatalf("孪生 %d 出生区块未就绪", index)
		}
		if index == 0 {
			baselineHash, baselineRevision = hash, revision
			continue
		}
		if hash != baselineHash || revision != baselineRevision {
			t.Fatalf("起点状态不一致: hash=%x/%x rev=%d/%d",
				baselineHash, hash, baselineRevision, revision)
		}
	}

	paused.Pause()
	frozenTick := paused.TickCount()
	for range pausedCycles {
		skipped := paused.StepForTest()
		if !reflect.DeepEqual(skipped, contract.TickResult{}) {
			t.Fatalf("暂停期返回了非空 tick 结果: %+v", skipped)
		}
	}
	if got := paused.TickCount(); got != frozenTick {
		t.Fatalf("暂停期世界推进: 冻结于 %d，实际 %d", frozenTick, got)
	}
	paused.Resume()

	// 续跑段两侧注入同序玩家输入并逐 tick 对照：结算结果给出位置这类连续
	// 模拟量，冻结缺口不得在任一侧留下可观测漂移。
	for step := range replaySteps {
		command := contract.Command{
			Session:  testSessionID,
			Sequence: inputSequence + uint64(step),
			Kind:     contract.CommandPlayerInput,
			MoveZ:    1,
			Yaw:      0,
			Pitch:    -0.25,
		}
		paused.engine.Enqueue(command)
		unpaused.engine.Enqueue(command)
		wantTick := frozenTick + uint64(step) + 1
		fromPaused, fromUnpaused := paused.StepForTest(), unpaused.StepForTest()
		if fromPaused.Tick != wantTick || fromUnpaused.Tick != wantTick {
			t.Fatalf("第 %d 步 tick=(%d,%d)，想要 %d",
				step, fromPaused.Tick, fromUnpaused.Tick, wantTick)
		}
		if fromPaused.WorldTimeTicks != fromUnpaused.WorldTimeTicks {
			t.Fatalf("第 %d 步世界时间分叉: %d != %d",
				step, fromPaused.WorldTimeTicks, fromUnpaused.WorldTimeTicks)
		}
		assertSinglePlayerAligned(t, step, fromPaused.Players, fromUnpaused.Players)
	}

	pausedHash, pausedRevision, ok := paused.ChunkHash(core.Overworld, core.ChunkPos{})
	unpausedHash, unpausedRevision, okToo :=
		unpaused.ChunkHash(core.Overworld, core.ChunkPos{})
	if !ok || !okToo {
		t.Fatalf("终态区块读取失败 ok=%v/%v", ok, okToo)
	}
	if pausedHash != unpausedHash || pausedRevision != unpausedRevision {
		t.Fatalf("相同绝对 tick 处区块状态不一致: hash=%x/%x rev=%d/%d",
			pausedHash, unpausedHash, pausedRevision, unpausedRevision)
	}
	if gotPaused, gotUnpaused := paused.TickCount(), unpaused.TickCount(); gotPaused != gotUnpaused {
		t.Fatalf("终点 tick 计数分叉: %d != %d", gotPaused, gotUnpaused)
	}
}

// TestRunTicksSchedulerDoesNotExecuteTickWhilePaused 在真实调度循环上验证：
// 暂停窗口内 ScheduledTickObserver 不再收到样本——本周期连编排入口都未进入。
func TestRunTicksSchedulerDoesNotExecuteTickWhilePaused(t *testing.T) {
	_, endpoint := network.NewMemoryPair(32)
	config := DefaultConfig(42)
	config.ViewRadius = 0
	config.Workers = 1
	var scheduled atomic.Uint64
	config.ScheduledTickObserver = func(time.Time, time.Duration) {
		scheduled.Add(1)
	}
	running := newMemoryAttachedWorldForTest(config, endpoint, playerTestGenerator{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = running.RunTicks(ctx) }()
	t.Cleanup(func() { shutdownServerForTest(t, running) })

	waitUntilScheduledCount(t, &scheduled, 2, "暂停前未积累足够已调度 tick")
	running.Pause()
	beforeFreeze := scheduled.Load()
	time.Sleep(pauseObserverDwell)
	if got := scheduled.Load(); got != beforeFreeze {
		t.Fatalf("暂停窗口内调度 tick 继续执行: %d -> %d", beforeFreeze, got)
	}

	running.Resume()
	waitUntilScheduledCount(t, &scheduled, beforeFreeze+1, "Resume 后调度未继续")
	cancel()
}

func assertSinglePlayerAligned(
	t *testing.T,
	step int,
	fromPaused []contract.PlayerUpdate,
	fromUnpaused []contract.PlayerUpdate,
) {
	t.Helper()
	if len(fromPaused) != 1 || len(fromUnpaused) != 1 {
		t.Fatalf("第 %d 步玩家更新数量 = %d/%d，想要各 1",
			step, len(fromPaused), len(fromUnpaused))
	}
	left, right := fromPaused[0], fromUnpaused[0]
	if left.Session != testSessionID || right.Session != testSessionID {
		t.Fatalf("第 %d 步会话标识 = %d/%d", step, left.Session, right.Session)
	}
	if left.State != right.State {
		t.Fatalf("第 %d 步玩家运动状态分叉: %+v != %+v", step, left.State, right.State)
	}
	if left.LastInputSequence != right.LastInputSequence {
		t.Fatalf("第 %d 步输入确认分叉: %d != %d",
			step, left.LastInputSequence, right.LastInputSequence)
	}
}

func waitUntilScheduledCount(t *testing.T, counter *atomic.Uint64, want uint64, message string) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for counter.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("%s: 实际 %d，想要 >= %d", message, counter.Load(), want)
		}
		time.Sleep(integrationPollInterval)
	}
}

// newDeterminismTwinServers 构造同种子的两个孪生世界（ViewRadius=0 仅覆盖
// 出生锚，出生区块经手动投递推进到 Ready，保证相对 tick 序不含异步竞态）。
func newDeterminismTwinServers(t *testing.T) (*Server, *Server) {
	t.Helper()
	// 孪生世界对照的硬性要求只有共用同一种子，具体取值无关紧要。
	const twinWorldSeed = int64(42)
	newTwin := func() *Server {
		_, endpoint := network.NewMemoryPair(32)
		config := DefaultConfig(twinWorldSeed)
		config.ViewRadius = 0
		config.Workers = 1
		return newMemoryAttachedWorldForTest(config, endpoint, playerTestGenerator{})
	}
	first, second := newTwin(), newTwin()
	t.Cleanup(func() {
		shutdownServerForTest(t, first)
		shutdownServerForTest(t, second)
	})
	return first, second
}

// readySpawnChunkForDeterministicReplay 把出生区块同步推进到 Ready：
// acquire 缺失 → 引擎直推一步产出生成请求 → 注入同种子生成结果。全程不经
// 异步 worker，两个孪生世界的引擎步进序列因此完全镜像。
func readySpawnChunkForDeterministicReplay(t *testing.T, running *Server) {
	t.Helper()
	spawnKey := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{}}
	requested := running.StepForTest()
	if len(requested.Acquire) != 1 || requested.Acquire[0] != spawnKey {
		t.Fatalf("首 tick Acquire = %+v，想要 [%+v]", requested.Acquire, spawnKey)
	}
	submitServerAcquiredMisses(running, requested.Acquire)
	generated := running.engine.Step()
	if len(generated.Generate) != 1 || generated.Generate[0] != spawnKey {
		t.Fatalf("Generate = %+v，想要 [%+v]", generated.Generate, spawnKey)
	}
	running.engine.SubmitGenerated(contract.GeneratedChunk{
		Dimension: spawnKey.Dimension,
		Pos:       spawnKey.Pos,
		Chunk:     playerTestGenerator{}.GenerateChunk(spawnKey.Pos),
	})
	ready := running.StepForTest()
	if len(ready.Ready) != 1 || ready.Ready[0] != spawnKey {
		t.Fatalf("Ready = %+v，想要 [%+v]", ready.Ready, spawnKey)
	}
}

// ---------------------------------------------------------------------------
// Host 暂停门直通测试。单机交互客户端经 `app.Host` 只持有 `*Host`，
// 因此宿主必须把暂停门转发到内持 `*Server` 的同一原子位；这里用行为断言
// 钉住转发落点与幂等性（冻结语义本身以 Server 侧 pause 测试为准，不重复
// 孪生世界对照）。
// ---------------------------------------------------------------------------

func TestHostPausePassthroughFreezesWorldUntilResume(t *testing.T) {
	host := newTestHost(t)

	warmup := host.world.StepForTest()
	if warmup.WorldTimeTicks == 0 {
		t.Fatalf("预热 tick 世界时间 = %d，想要非零", warmup.WorldTimeTicks)
	}
	frozenAt := host.world.TickCount()

	host.Pause()
	if result := host.world.StepForTest(); !reflect.DeepEqual(result, contract.TickResult{}) {
		t.Fatalf("宿主暂停后 tick 未被跳过: %+v", result)
	}
	if got := host.world.TickCount(); got != frozenAt {
		t.Fatalf("宿主暂停后世界推进: 冻结于 %d，实际 %d", frozenAt, got)
	}

	host.Resume()
	resumed := host.world.StepForTest()
	wantTick, wantWorldTime := frozenAt+1, warmup.WorldTimeTicks+1
	if resumed.Tick != wantTick || resumed.WorldTimeTicks != wantWorldTime {
		t.Fatalf("宿主恢复后首个 tick=(%d,%d)，想要 (%d,%d)",
			resumed.Tick, resumed.WorldTimeTicks, wantTick, wantWorldTime)
	}
}

func TestHostPausePassthroughIsIdempotent(t *testing.T) {
	host := newTestHost(t)
	warmup := host.world.StepForTest()
	frozenAt := host.world.TickCount()

	// 置位侧幂等：重复 Pause 只写同一原子位，不排队、不计数。
	host.Pause()
	host.Pause()
	if result := host.world.StepForTest(); !reflect.DeepEqual(result, contract.TickResult{}) {
		t.Fatalf("重复 Pause 后 tick 未被跳过: %+v", result)
	}
	if got := host.world.TickCount(); got != frozenAt {
		t.Fatalf("重复 Pause 改变了状态: 冻结于 %d，实际 %d", frozenAt, got)
	}

	// 恢复侧幂等：对刚恢复的世界再调一次 Resume 同样无害，直接续接增量。
	host.Resume()
	host.Resume()
	for offset := uint64(1); offset <= 2; offset++ {
		resumed := host.world.StepForTest()
		wantTick, wantWorldTime := frozenAt+offset, warmup.WorldTimeTicks+offset
		if resumed.Tick != wantTick || resumed.WorldTimeTicks != wantWorldTime {
			t.Fatalf("恢复后第 %d 个 tick=(%d,%d)，想要 (%d,%d)",
				offset, resumed.Tick, resumed.WorldTimeTicks, wantTick, wantWorldTime)
		}
	}
}
