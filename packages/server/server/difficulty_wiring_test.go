package server

import (
	"context"
	"errors"
	"testing"

	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/network"
)

// 难度装配接线的集成测试：创建/重启把 `storage.Metadata.Difficulty` 送达
// `runtime.NewEngine`、权威周期不回读磁盘、同一难度经 Memory 与 TCP 两条
// 传输装配后夜间生成门控行为一致。难度的规则语义（饥饿、回血、门控短路
// 本体）由 sim 侧测试钉住，本文件只钉「服务端装配把正确的域值交给 Engine」。

// difficultyDiskWorldConfig 是难度磁盘装配测试的服务端配置：零视距、单
// worker，与 metadata 重启夹具同形——本组测试不依赖区块装载。
func difficultyDiskWorldConfig(seed int64) Config {
	config := DefaultConfig(seed)
	config.ViewRadius = 0
	config.Workers = 1
	config.SaveWorkers = 1
	return config
}

// TestServerWiresDiskDifficultyIntoEngineAcrossRestart 覆盖难度规约「重启与
// 跨传输同难度结果」的重启半边：hard 世界的创建快照必须压过 Engine 构造的
// 缺省 normal 并直达权威侧；正常关服把难度随 v6 metadata 落盘，重开不带
// Create（难度唯一来源是磁盘文件）后 Engine 读回仍为 hard，且推进若干 tick
// 后依旧只读——保存的难度优先于构造默认值。
func TestServerWiresDiskDifficultyIntoEngineAcrossRestart(t *testing.T) {
	root := t.TempDir()
	const seed int64 = 4242
	config := difficultyDiskWorldConfig(seed)

	firstStore, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{
		Create: storage.Metadata{
			FormatVersion: 6, Seed: seed, SpawnDimension: core.Overworld,
			Difficulty:        core.DifficultyHard,
			DepthsSpawnAnchor: core.ChunkPos{},
			DepthsSeedSalt:    0x9E3779B97F4A7C15,
		},
	})
	if err != nil {
		t.Fatalf("OpenDisk create: %v", err)
	}
	first := NewWorld(config, playerTestGenerator{}, firstStore)
	if got := first.engine.DifficultyForTest(); got != core.DifficultyHard {
		t.Fatalf("首段 engine 难度 = %v，想要 metadata 的 hard", got)
	}
	for range 3 {
		first.StepForTest()
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := first.Shutdown(ctx); err != nil {
		t.Fatalf("首段关服: %v", err)
	}

	secondStore, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenDisk reopen: %v", err)
	}
	if got := secondStore.Metadata().Difficulty; got != core.DifficultyHard {
		t.Fatalf("重开 metadata 难度 = %v，想要落盘的 hard", got)
	}
	second := NewWorld(config, playerTestGenerator{}, secondStore)
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), waitDeadline)
		defer shutdownCancel()
		if err := second.Shutdown(shutdownCtx); err != nil {
			t.Errorf("清理关服: %v", err)
		}
	})
	if got := second.engine.DifficultyForTest(); got != core.DifficultyHard {
		t.Fatalf("重开 engine 难度 = %v，想要 metadata 的 hard", got)
	}
	for range 2 {
		second.StepForTest()
	}
	if got := second.engine.DifficultyForTest(); got != core.DifficultyHard {
		t.Fatalf("推进后 engine 难度 = %v，想要生命周期内只读的 hard", got)
	}
}

// TestEngineDifficultySnapshotIgnoresMetadataSavesAfterAssembly 覆盖难度规约
// 「权威 tick 不读磁盘」：装配后再对同一 store 保存一份 peaceful metadata
// （等价于任一并发保存者写入不同难度），本周期 Engine 快照必须不受影响——
// 难度在构造时一次性注入，tick 路径不回读 storage。
func TestEngineDifficultySnapshotIgnoresMetadataSavesAfterAssembly(t *testing.T) {
	root := t.TempDir()
	const seed int64 = 77
	store, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{
		Create: storage.Metadata{
			FormatVersion: 6, Seed: seed, SpawnDimension: core.Overworld,
			Difficulty:        core.DifficultyHard,
			DepthsSpawnAnchor: core.ChunkPos{},
			DepthsSeedSalt:    0x9E3779B97F4A7C15,
		},
	})
	if err != nil {
		t.Fatalf("OpenDisk: %v", err)
	}
	running := NewWorld(difficultyDiskWorldConfig(seed), playerTestGenerator{}, store)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		if err := running.Shutdown(ctx); err != nil {
			t.Errorf("清理关服: %v", err)
		}
	})
	if got := running.engine.DifficultyForTest(); got != core.DifficultyHard {
		t.Fatalf("engine 难度 = %v，想要 metadata 的 hard", got)
	}

	mutated := store.Metadata()
	mutated.Difficulty = core.DifficultyPeaceful
	if err := store.SaveMetadata(context.Background(), mutated); err != nil {
		t.Fatalf("改写 metadata: %v", err)
	}
	for range 5 {
		running.StepForTest()
	}
	if got := running.engine.DifficultyForTest(); got != core.DifficultyHard {
		t.Fatalf("磁盘 metadata 改写后 engine 难度 = %v，想要构造快照的 hard", got)
	}
}

// difficultyProbeTicks 是夜窗探针在登录就绪后固定推进的 tick 数。夜间候选流
// 由（种子、世界时间、锚点）完全确定，无运行间抖动；该窗口对 normal 控制腿
// 足以完成至少一次夜间生成，夹具自证由此成立。
const difficultyProbeTicks = 512

// difficultyProbeViewRadius 是探针的最小订阅半径：夜间候选列落在锚点玩家
// 24..48 格外、至多 3 个区块外，半径 3 的方形视界恰好完整覆盖候选区块，
// 「候选区块未加载」不再成为不生成的借口。
const difficultyProbeViewRadius = 3

// TestPeacefulNightSpawnGateParityAcrossTransports 覆盖难度规约「重启与跨
// 传输同难度结果」的跨传输半边：同一份 peaceful 世界 metadata 经 Memory 与
// TCP 两条传输装配后，Engine 难度读回一致，且同一夜间窗口内零夜行者生成。
// normal 控制腿先在相同夹具（同种子、同夜间锚相位、同推进窗口）下证明「该
// 夹具会生成夜行者」，peaceful 的零生成才是门控经装配生效的证据，而不是
// 夹具本身不生成的假一致。
func TestPeacefulNightSpawnGateParityAcrossTransports(t *testing.T) {
	if spawned := runNightSpawnDifficultyProbe(t, core.DifficultyNormal, "memory"); spawned == 0 {
		t.Fatal("normal 控制腿夜窗内零生成：夹具不足以证明 peaceful 门控")
	}
	for _, transport := range []string{"memory", "tcp"} {
		if spawned := runNightSpawnDifficultyProbe(t, core.DifficultyPeaceful, transport); spawned != 0 {
			t.Fatalf("%s 装配的 peaceful 世界夜窗内生成 %d 只夜行者，想要 0", transport, spawned)
		}
	}
}

// runNightSpawnDifficultyProbe 以指定难度与传输装配一个内存世界（世界时间
// 从跨季节稳定的夜间锚起步），登录就绪后推进固定夜窗，返回窗口结束时权威
// 侧夜行者总数；同时就地断言 Engine 难度与 metadata 一致。两条传输只差
// 链路形态，难度与候选流逐位同源，结果是可比的。
func runNightSpawnDifficultyProbe(t *testing.T, difficulty core.Difficulty, transport string) int {
	t.Helper()
	store := storage.NewMemory(storage.Metadata{
		FormatVersion:     6,
		Seed:              42,
		SpawnDimension:    core.Overworld,
		WorldTimeTicks:    hostileRestartNightTicks,
		Difficulty:        difficulty,
		DepthsSpawnAnchor: core.ChunkPos{},
		DepthsSeedSalt:    0x9E3779B97F4A7C15,
	})
	config := hostTestConfig()
	config.ViewRadius = difficultyProbeViewRadius
	config.OutboxCapacity = 512
	// 关掉自动存盘：本探针只观察权威生成路径，不制造落盘噪声。
	config.AutosaveTicks = 1 << 30
	host := mustNewHost(t, config, flatTestGenerator{}, store)
	// mustNewHost 不注册清理钩子：host 生命周期由本 runner 显式收口（与
	// 既有 parity runner 同纪律），否则 chunk/save worker 会驻留整个测试
	// 进程，污染后续测试的进程级 goroutine 泄漏判定。
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		if err := host.Shutdown(ctx); err != nil {
			t.Fatalf("%s 探针 Host.Shutdown: %v", transport, err)
		}
	}()
	identity := integrationIdentity(0xB3, "DifficultyProbe")
	endpoint, acceptDone, closeTransport := openParityTransport(t, host, transport, identity)
	defer closeTransport()

	ready := false
	for range integrationLoginTickBudget {
		if state := stepDifficultyProbeTick(t, host, endpoint); state.Ready {
			ready = true
			break
		}
	}
	if !ready {
		t.Fatalf("%s %v 探针登录未就绪", transport, difficulty)
	}
	for range difficultyProbeTicks {
		stepDifficultyProbeTick(t, host, endpoint)
	}

	// 引擎读取与既有 tick 路径同一锁界：随后的 endpoint 关闭会经 reader 触发
	// 异步会话回收，无锁读与该写入构成数据竞争。
	host.world.stepMu.Lock()
	engineDifficulty := host.world.engine.DifficultyForTest()
	spawned := len(host.world.engine.HostileMobs())
	host.world.stepMu.Unlock()
	if engineDifficulty != difficulty {
		t.Fatalf("%s 装配 engine 难度 = %v，想要 metadata 的 %v", transport, engineDifficulty, difficulty)
	}

	if err := endpoint.Close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	select {
	case err := <-acceptDone:
		if err != nil && !errors.Is(err, network.ErrClosed) {
			t.Fatalf("%s 探针 accept worker: %v", transport, err)
		}
	case <-ctx.Done():
		t.Fatalf("%s 探针 accept worker 未退出: %v", transport, ctx.Err())
	}
	return spawned
}

// stepDifficultyProbeTick 推进一个权威 tick 并把服务端消息排空到本 tick 的
// PlayerState 为止（与 `parityStep` 同一「PlayerState 收尾每 tick 消息流」
// 契约），返回该状态供调用方检查就绪标志。
func stepDifficultyProbeTick(
	t *testing.T,
	host *Host,
	endpoint network.ClientEndpoint,
) network.PlayerState {
	t.Helper()
	result := host.world.StepForTest()
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	for {
		message, err := endpoint.Recv(ctx)
		if err != nil {
			t.Fatalf("探针 tick %d Recv: %v", result.Tick, err)
		}
		if state, ok := message.(network.PlayerState); ok {
			if state.ServerTick != result.Tick {
				t.Fatalf("探针 PlayerState tick=%d, want %d", state.ServerTick, result.Tick)
			}
			return state
		}
	}
}
