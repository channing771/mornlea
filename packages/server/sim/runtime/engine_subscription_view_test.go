package runtime_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/server/sim/runtime"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/world"
)

// viewSquareKeys 构造以 center 为圆心、radius 为半径的方形视距键集合，
// 与 sessionWantedSnapshot 的循环边界同口径。
func viewSquareKeys(
	dimension core.DimensionID,
	center core.ChunkPos,
	radius int,
) map[core.ChunkKey]struct{} {
	keys := make(map[core.ChunkKey]struct{})
	for dz := -radius; dz <= radius; dz++ {
		for dx := -radius; dx <= radius; dx++ {
			keys[core.ChunkKey{
				Dimension: dimension,
				Pos:       core.ChunkPos{X: center.X + int32(dx), Z: center.Z + int32(dz)},
			}] = struct{}{}
		}
	}
	return keys
}

func acquireKeySet(t *testing.T, result runtime.TickResult) map[core.ChunkKey]struct{} {
	t.Helper()
	keys := make(map[core.ChunkKey]struct{}, len(result.Acquire))
	for _, key := range result.Acquire {
		if _, duplicate := keys[key]; duplicate {
			t.Fatalf("Acquire 含重复键 %+v", key)
		}
		keys[key] = struct{}{}
	}
	return keys
}

func assertKeySetsEqual(
	t *testing.T,
	got map[core.ChunkKey]struct{},
	want map[core.ChunkKey]struct{},
) {
	t.Helper()
	for key := range want {
		if _, ok := got[key]; !ok {
			t.Fatalf("缺少期望键 %+v（got %d 个 / want %d 个）", key, len(got), len(want))
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			t.Fatalf("出现期望外键 %+v（got %d 个 / want %d 个）", key, len(got), len(want))
		}
	}
}

// TestSingleSessionDeclaredViewDistanceWantedSquare 钉住单会话声明视距的
// 方形语义：声明 2 的会话订阅范围恰为 5×5（视距 +1 为半径），不随引擎上界
// 扩张；引擎上界仍是缺省（未声明）与钳制上界。
func TestSingleSessionDeclaredViewDistanceWantedSquare(t *testing.T) {
	engine := runtime.NewEngine(33, 0, 0)
	const session = runtime.SessionID(1)
	engine.RegisterPlayer(session, runtime.PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{},
		ViewDistance:   2,
	})

	result := engine.Step()
	assertKeySetsEqual(t, acquireKeySet(t, result), viewSquareKeys(core.Overworld, core.ChunkPos{}, 2+1))

	inside := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 3, Z: -3}}
	outside := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 3, Z: -4}}
	if !engine.SessionWantsChunk(session, inside) {
		t.Fatalf("会话应订阅方形内区块 %+v", inside)
	}
	if engine.SessionWantsChunk(session, outside) {
		t.Fatalf("会话不应订阅方形外区块 %+v", outside)
	}
}

// TestDeclaredViewDistanceClampedToEngineUpperBound 对应规格
// 「超出服务端上界被钳制」：声明 64、引擎上界 33（换算后生效视距 32）时
// 会话生效半径必须恰为 33，且登录注册路径不被拒绝。
func TestDeclaredViewDistanceClampedToEngineUpperBound(t *testing.T) {
	engine := runtime.NewEngine(33, 0, 0)
	const session = runtime.SessionID(1)
	engine.RegisterPlayer(session, runtime.PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{},
		ViewDistance:   64,
	})

	result := engine.Step()
	assertKeySetsEqual(t, acquireKeySet(t, result), viewSquareKeys(core.Overworld, core.ChunkPos{}, 33))
}

// TestUndeclaredSessionUsesEngineBound pins 未声明路径（视距 0，如既有探针
// 夹具）：会话沿用引擎视界为缺省，ViewRadius=0 的引擎只经出生路径取锚点
// 区块，方形订阅为空——多人探针现状行为不变。
func TestUndeclaredSessionUsesEngineBound(t *testing.T) {
	engine := runtime.NewEngine(2, 0, 0)
	const session = runtime.SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})

	result := engine.Step()
	assertKeySetsEqual(t, acquireKeySet(t, result), viewSquareKeys(core.Overworld, core.ChunkPos{}, 2))

	probe := runtime.NewEngine(0, 0, 0)
	const probeSession = runtime.SessionID(2)
	probe.RegisterSession(probeSession, core.Overworld, core.ChunkPos{})
	probeResult := probe.Step()
	anchor := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{}}
	assertKeySetsEqual(t, acquireKeySet(t, probeResult), map[core.ChunkKey]struct{}{anchor: {}})
}

// TestTrustedObserverSubscribesEngineUpperBound pins trusted observer 不参与
// 登录协商：订阅范围恒为引擎上界方形，与会话声明无关。
func TestTrustedObserverSubscribesEngineUpperBound(t *testing.T) {
	engine := runtime.NewEngine(2, 0, 0)
	engine.RegisterObserverSession(1)
	engine.Enqueue(runtime.Command{
		Session: 1, Sequence: 1, Kind: runtime.CommandTrustedObserverCenter,
		Dimension: core.Overworld, Center: core.ChunkPos{X: 4, Z: -2},
	})

	result := engine.Step()
	assertKeySetsEqual(t, acquireKeySet(t, result), viewSquareKeys(core.Overworld, core.ChunkPos{X: 4, Z: -2}, 2))
}

// loadUnionClean 把当前 Acquire 集合以干净已持久化区块完成加载并断言全部
// Ready，返回加载完成的键集合。
func loadUnionClean(
	t *testing.T,
	engine *runtime.Engine,
	acquired []core.ChunkKey,
) map[core.ChunkKey]struct{} {
	t.Helper()
	for _, key := range acquired {
		engine.SubmitAcquired(runtime.AcquiredChunk{
			Key: key, Chunk: world.NewChunk(key.Pos), Revision: 7, PersistedRevision: 7,
		})
	}
	loaded := engine.Step()
	if len(loaded.Ready) != len(acquired) {
		t.Fatalf("Ready = %d 个，想要 %d 个全部就绪", len(loaded.Ready), len(acquired))
	}
	ready := make(map[core.ChunkKey]struct{}, len(acquired))
	for _, key := range acquired {
		info, ok := engine.ChunkInfo(key)
		if !ok || info.State != runtime.ChunkReady {
			t.Fatalf("区块 %+v info=%+v ok=%v，想要 Ready", key, info, ok)
		}
		ready[key] = struct{}{}
	}
	return ready
}

// TestUnionSubscriptionDifferentViewDistances 对应规格「两会话不同视距按
// 并集加载」：视距 4 与 16、中心相邻的两会话，已加载集合必须等于两方形
// 并集；大会话视距内的区块不因小会话而在并存期间被卸载。
func TestUnionSubscriptionDifferentViewDistances(t *testing.T) {
	engine := runtime.NewEngine(33, 0, 0)
	const small, large = runtime.SessionID(1), runtime.SessionID(2)
	engine.RegisterPlayer(small, runtime.PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 0, Z: 0},
		ViewDistance:   4,
	})
	engine.RegisterPlayer(large, runtime.PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 1, Z: 0},
		ViewDistance:   16,
	})

	first := engine.Step()
	wantUnion := viewSquareKeys(core.Overworld, core.ChunkPos{X: 0, Z: 0}, 5)
	for key := range viewSquareKeys(core.Overworld, core.ChunkPos{X: 1, Z: 0}, 17) {
		wantUnion[key] = struct{}{}
	}
	assertKeySetsEqual(t, acquireKeySet(t, first), wantUnion)
	loaded := loadUnionClean(t, engine, first.Acquire)

	// 并存期间：小会话不使大会话视距内的已加载区块被卸载（仍全部 Ready）。
	for key := range viewSquareKeys(core.Overworld, core.ChunkPos{X: 1, Z: 0}, 17) {
		if _, ready := loaded[key]; !ready {
			t.Fatalf("大会话方形内区块 %+v 未随并集加载", key)
		}
	}
}

// TestShrunkUnionUnloadsExcessChunksAfterLargeSessionLeaves 对应规格
// 「视距缩小后多余区块卸载」：视距 16 会话断开、相邻视距 4 会话仍在时，
// 不再属于任何会话订阅范围的区块走既有卸载路径——干净区块立即卸载，
// 脏区块保留 Unloading 并进入持久化快照，保存确认后完成卸载。
func TestShrunkUnionUnloadsExcessChunksAfterLargeSessionLeaves(t *testing.T) {
	engine := runtime.NewEngine(33, 0, 0)
	const small, large = runtime.SessionID(1), runtime.SessionID(2)
	engine.RegisterPlayer(small, runtime.PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 0, Z: 0},
		ViewDistance:   4,
	})
	engine.RegisterPlayer(large, runtime.PlayerRestore{
		SpawnDimension: core.Overworld,
		SpawnAnchor:    core.ChunkPos{X: 1, Z: 0},
		ViewDistance:   16,
	})

	first := engine.Step()
	loadUnionClean(t, engine, first.Acquire)
	retained := viewSquareKeys(core.Overworld, core.ChunkPos{X: 0, Z: 0}, 5)

	// 大会话断开后，脏区块（生成后从未持久化或本地已改）必须先保存再卸载。
	dirtyKey := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: 17, Z: 0}}
	if _, retainedDirty := retained[dirtyKey]; retainedDirty {
		t.Fatalf("脏区块 %+v 应位于缩小后订阅范围之外", dirtyKey)
	}
	engine.TouchChunkForTest(dirtyKey)

	// 待出生玩家不可持久化，注销只保证订阅记录移除（快照 ok 位为假）。
	engine.UnregisterSession(large)
	shrunk := engine.Step()

	for key := range retained {
		info, ok := engine.ChunkInfo(key)
		if !ok || info.State != runtime.ChunkReady {
			t.Fatalf("仍被订阅的区块 %+v info=%+v ok=%v，想要 Ready", key, info, ok)
		}
	}
	if info, ok := engine.ChunkInfo(dirtyKey); !ok || info.State != runtime.ChunkUnloading {
		t.Fatalf("脏区块 %+v info=%+v ok=%v，想要 Unloading", dirtyKey, info, ok)
	}
	cleanKey := core.ChunkKey{Dimension: core.Overworld, Pos: core.ChunkPos{X: -6, Z: 0}}
	if _, retainedClean := retained[cleanKey]; retainedClean {
		t.Fatalf("干净区块 %+v 应位于缩小后订阅范围之外", cleanKey)
	}
	if _, ok := engine.ChunkInfo(cleanKey); ok {
		t.Fatalf("干净区块 %+v 应立即卸载", cleanKey)
	}
	if len(shrunk.Ready) != 0 {
		t.Fatalf("缩小对账不应产生新的 Ready: %+v", shrunk.Ready)
	}

	// 保存确认后脏区块完成卸载（既有「脏则保存后卸」闭环）。
	snapshots := engine.PersistenceSnapshots(16, 1<<20, runtime.SaveUrgent)
	dirtyRevision := uint64(0)
	for _, snapshot := range snapshots {
		if snapshot.Key == dirtyKey {
			dirtyRevision = snapshot.Revision
		}
	}
	if dirtyRevision == 0 {
		t.Fatalf("Unloading 区块 %+v 未进入持久化快照: %+v", dirtyKey, snapshots)
	}
	engine.ApplyPersisted([]runtime.PersistedChunk{{Key: dirtyKey, Revision: dirtyRevision}})
	if _, ok := engine.ChunkInfo(dirtyKey); ok {
		t.Fatalf("保存确认后区块 %+v 应完成卸载", dirtyKey)
	}
}
