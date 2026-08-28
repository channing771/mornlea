package server

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/storage"
)

// respawn_persistence_test.go：个人重生点在服务端持久化路径上的接线——快照 →
// 存档字节 → 磁盘 → 恢复。重生点是「睡一觉」唯一的状态变化，变更检测漏掉
// 任何一个字段，这次入睡就会被当成无变化而永不落盘。

// respawnTestPosition 是接线用例的床尾格坐标：三个分量全部非零，任何一段路径
// 漏写都会让读回值落在零值上，与这里的取值不同，用例因此承重。
var respawnTestPosition = [3]float32{7, 1, 5}

// respawnTestSnapshot 在既有快照夹具之上带上一个非零重生点。
func respawnTestSnapshot() contract.PlayerSnapshot {
	snapshot := testPlayerSnapshot(10)
	snapshot.RespawnPresent = true
	snapshot.RespawnPosition = respawnTestPosition
	snapshot.RespawnDimension = core.Overworld
	return snapshot
}

// TestRespawnPointSurvivesDiskRestart 覆盖重生点跨重启保留的服务端全程：
// 权威快照经 save() → 玩家 schema v8 字节 → 真实磁盘 → LoadPlayer → restore()
// 之后，重生点必须逐字段原值返回。
func TestRespawnPointSurvivesDiskRestart(t *testing.T) {
	root := t.TempDir()
	id := playerID(0x74)
	const name = "Sleeper"
	ctx := context.Background()
	want := respawnTestSnapshot()
	seedHungerPlayer(t, root, id, name, want)

	// 第一次开服：观察到带重生点的权威快照并落盘。
	store := openHungerDiskStore(t, root, false)
	persistence := newPlayerPersistence(store, playerPersistenceTestConfig())
	if _, err := persistence.Prepare(ctx, id, name, testMetadata()); err != nil {
		t.Fatalf("首次登录: %v", err)
	}
	if err := persistence.Observe(id, name, want, 0, false); err != nil {
		t.Fatalf("观察权威快照: %v", err)
	}
	// 落盘前先自匹配：漏写任何一个重生点字段会让 matchesSave 恒假，Flush
	// 因此永远重派保存直到包级超时才收场——先把该类变异钉在值断言上。
	persistence.mu.Lock()
	cached := persistence.cache[id]
	selfMatches := cached.matchesSave(cached.save(1))
	persistence.mu.Unlock()
	if !selfMatches {
		t.Fatal("落盘前缓存快照与自身存档不匹配")
	}
	flushHungerPersistence(t, persistence)
	persistence.CloseWorker()
	if err := store.Close(); err != nil {
		t.Fatalf("关闭世界: %v", err)
	}

	// 落盘字节本身必须携带重生点：把「写盘漏字段」与「读盘漏字段」分开。
	stored := loadStoredPlayerForTest(t, root, id)
	if !stored.RespawnPresent || stored.RespawnPosition != respawnTestPosition ||
		stored.RespawnDimension != core.Overworld {
		t.Fatalf("落盘的重生点 = (present %v, %+v, %d)，想要 (true, %+v, %d)",
			stored.RespawnPresent, stored.RespawnPosition, stored.RespawnDimension,
			respawnTestPosition, core.Overworld)
	}

	// 重启：同一份磁盘世界重开并重连，重生点必须原值恢复。
	restarted := openHungerDiskStore(t, root, false)
	defer func() {
		if err := restarted.Close(); err != nil {
			t.Fatalf("关闭重启后的世界: %v", err)
		}
	}()
	reopened := newPlayerPersistence(restarted, playerPersistenceTestConfig())
	defer reopened.CloseWorker()
	restore, err := reopened.Prepare(ctx, id, name, testMetadata())
	if err != nil {
		t.Fatalf("重连: %v", err)
	}
	if !restore.RespawnPresent {
		t.Fatal("重启后的 PlayerRestore 缺少重生点")
	}
	if restore.RespawnPosition != respawnTestPosition ||
		restore.RespawnDimension != core.Overworld {
		t.Fatalf("重启后恢复的重生点 = (%+v, %d)，想要 (%+v, %d)",
			restore.RespawnPosition, restore.RespawnDimension,
			respawnTestPosition, core.Overworld)
	}
}

// TestPlayerPersistenceDirtyDetectionIncludesRespawn 覆盖「只有重生点变化也必须
// 被持久化」：快照比较与存档比较都必须把重生点三个字段逐个算进去。
func TestPlayerPersistenceDirtyDetectionIncludesRespawn(t *testing.T) {
	base := respawnTestSnapshot()
	cases := []struct {
		name  string
		apply func(*contract.PlayerSnapshot)
	}{
		{"重生点出现", func(s *contract.PlayerSnapshot) {
			s.RespawnPresent = false
		}},
		{"重生点坐标", func(s *contract.PlayerSnapshot) {
			s.RespawnPosition = [3]float32{8, 1, 6}
		}},
		{"重生点维度", func(s *contract.PlayerSnapshot) {
			s.RespawnDimension = 1
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			changed := base
			testCase.apply(&changed)
			if playerSnapshotsEqual(base, changed) {
				t.Fatalf("快照比较忽略了%s", testCase.name)
			}
			player := &cachedPlayer{hasSnapshot: true, snapshot: base}
			save := player.save(1)
			if !player.matchesSave(save) {
				t.Fatal("同值存档必须匹配缓存快照")
			}
			player.snapshot = changed
			if player.matchesSave(save) {
				t.Fatalf("存档比较忽略了%s", testCase.name)
			}
		})
	}
}

// TestRespawnOnlyChangeGetsPersisted 锁定真实调度行为：玩家原地入睡（只有重生点
// 变化）后的快照必须被判脏并真正写盘，磁盘上的 schema v8 记录带重生点。
func TestRespawnOnlyChangeGetsPersisted(t *testing.T) {
	root := t.TempDir()
	id := playerID(0x75)
	const name = "Dreamer"
	ctx := context.Background()
	seedHungerPlayer(t, root, id, name, testPlayerSnapshot(10))

	store := openHungerDiskStore(t, root, false)
	persistence := newPlayerPersistence(store, playerPersistenceTestConfig())
	defer persistence.CloseWorker()
	if _, err := persistence.Prepare(ctx, id, name, testMetadata()); err != nil {
		t.Fatalf("登录: %v", err)
	}
	// 除重生点外与存档完全相同：变更检测漏掉重生点，这次入睡就会被当成
	// 「无变化」，磁盘上永远没有重生点。
	if err := persistence.Observe(id, name, respawnTestSnapshot(), 0, false); err != nil {
		t.Fatalf("观察入睡后的权威快照: %v", err)
	}
	flushHungerPersistence(t, persistence)
	if err := store.Close(); err != nil {
		t.Fatalf("关闭世界: %v", err)
	}

	stored := loadStoredPlayerForTest(t, root, id)
	if !stored.RespawnPresent || stored.RespawnPosition != respawnTestPosition {
		t.Fatalf("入睡后落盘的重生点 = (present %v, %+v)，想要 (true, %+v)",
			stored.RespawnPresent, stored.RespawnPosition, respawnTestPosition)
	}
}

// TestNewWorldRestoresDayPhaseOffsetFromMetadata 覆盖宿主装配的一半：开服时
// 引擎的显示相位偏移必须从世界 metadata 恢复，而不是恒从 0 起步。
func TestNewWorldRestoresDayPhaseOffsetFromMetadata(t *testing.T) {
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld,
		DayPhaseOffset: 12399,
	})
	config := DefaultConfig(42)
	config.ViewRadius = 0
	config.Workers = 1
	config.SaveWorkers = 1
	running := NewWorld(config, playerTestGenerator{}, store)
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := running.Shutdown(ctx); err != nil {
		t.Fatalf("关服: %v", err)
	}
	if got := running.engine.DayPhaseOffset(); got != 12399 {
		t.Fatalf("开服恢复的偏移 = %d，想要 12399", got)
	}
}

// TestShutdownPersistsDayPhaseOffset 覆盖关服屏障的一半：跳夜设置的非零偏移
// 必须随关服屏障落盘，重开世界时从保存值继续（metadata v3 偏移持久化）。
func TestShutdownPersistsDayPhaseOffset(t *testing.T) {
	root := t.TempDir()
	const seed int64 = 4242
	running, _ := openRestartWorld(t, root, seed)
	// 模拟跳夜结算后的偏移状态：经测试专用写入口置值（生产写者是
	// settleSleepThroughNight，这里只关心持久化接线）。
	running.engine.SetDayPhaseOffsetForTest(23999)
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := running.Shutdown(ctx); err != nil {
		t.Fatalf("关服: %v", err)
	}

	reopened, reopenedStore := openRestartWorld(t, root, seed)
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), waitDeadline)
		defer shutdownCancel()
		if err := reopened.Shutdown(shutdownCtx); err != nil {
			t.Errorf("清理关服: %v", err)
		}
	})
	if got := reopenedStore.Metadata().DayPhaseOffset; got != 23999 {
		t.Fatalf("关服落盘的偏移 = %d，想要 23999", got)
	}
	if got := reopened.engine.DayPhaseOffset(); got != 23999 {
		t.Fatalf("重开恢复的偏移 = %d，想要 23999", got)
	}
}
