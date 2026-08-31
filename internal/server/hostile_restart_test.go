package server

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/network"
	"github.com/channing771/mornlea/internal/storage"
)

// 夜行者跨重启恢复的端到端测试：两段完整宿主生命周期（Memory 与 Disk 两条
// 存储路径）验证——首 tick 前逐字段恢复、路径与槽位派生物绝不跨重启、首段
// 运行内路径按当前世界重算、关服屏障写入最新权威快照、第二段启动恢复与第
// 一段落盘逐字段一致（重启不清怪）。

// hostileRestartNightTicks 把世界时间钉在夜间相位（13000..23000）：灼烧静默，
// 运行内状态漂移只来自追逐与（视野内的）夜间生成，边界比对保持确定性。
const hostileRestartNightTicks = uint64(13000)

// hostileRestartChaserID 是种子中带目标的夜行者：恢复后首个到期 tick 重规划。
const hostileRestartChaserID = uint64(3)

func hostileRestartSeedRecords(target core.PlayerID) []storage.StoredHostileMob {
	return []storage.StoredHostileMob{
		{
			ID: hostileRestartChaserID, Dimension: core.Overworld,
			Position: [3]float32{2.5, 1, 2.5}, Velocity: [3]float32{0, 0, 0},
			OnGround: true, Yaw: 1.25,
			Health: 17, AttackCooldown: 3, HurtCooldown: 1, BurnCooldown: 20,
			HasTarget: true, PlayerID: target,
			NextRepathTicks: 5, DistantTicks: 42,
		},
		{
			ID: 9, Dimension: core.Overworld,
			Position: [3]float32{-4.5, 1, 3.5}, Velocity: [3]float32{0, 0, 0},
			OnGround: true, Yaw: -2.5,
			Health: 9, BurnCooldown: 20,
		},
	}
}

func hostileRestartConfig() Config {
	config := hostTestConfig()
	// 视野半径 2：覆盖追逐路径窗口的 3×3 区块，恢复后的重规划能在测试内
	// 真正走到 A* 结果应用。
	config.ViewRadius = 2
	return config
}

func hostileRestartMetadata() storage.Metadata {
	return storage.Metadata{
		FormatVersion:  3,
		Seed:           42,
		SpawnDimension: core.Overworld,
		WorldTimeTicks: hostileRestartNightTicks,
	}
}

func TestHostileRestartRestoresFullAuthorityAcrossLifetimes(t *testing.T) {
	target := playerIdentity(7)
	// memory 变体跨两段生命周期复用同一存档实例（`MemoryStore.Close` 为
	// 无操作，重复 Shutdown 不会使其失效）。
	var memoryStore storage.WorldStore
	for _, variant := range []struct {
		name string
		// makeWorld 建档并写入种子夜行者，返回世界根（memory 形态不使用）。
		makeWorld func(t *testing.T) string
		// openLifetimeStore 打开一段生命周期使用的存储；关闭由该生命周期的
		// `Host.Shutdown` 负责（与伙伴验收测试的 openLifetime 纪律一致）。
		openLifetimeStore func(t *testing.T, root string) storage.WorldStore
		// loadSaved 在一段生命周期关服后读取落盘记录。
		loadSaved func(t *testing.T, root string) storage.StoredHostileMobs
	}{
		{
			name: "memory",
			makeWorld: func(t *testing.T) string {
				store := &reusableHostileMemoryStore{hostTestStore: &hostTestStore{
					MemoryStore: storage.NewMemory(hostileRestartMetadata()),
				}}
				seedHostileMobsForRestart(t, store)
				memoryStore = store
				return ""
			},
			openLifetimeStore: func(t *testing.T, _ string) storage.WorldStore {
				return memoryStore
			},
			loadSaved: func(t *testing.T, _ string) storage.StoredHostileMobs {
				return loadHostileRecordsForRestart(t, memoryStore)
			},
		},
		{
			name: "disk",
			makeWorld: func(t *testing.T) string {
				root := t.TempDir()
				store, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{
					Create: hostileRestartMetadata(),
				})
				if err != nil {
					t.Fatalf("OpenDisk 种子存档: %v", err)
				}
				seedHostileMobsForRestart(t, store)
				if err := store.Close(); err != nil {
					t.Fatalf("close seed store: %v", err)
				}
				return root
			},
			openLifetimeStore: func(t *testing.T, root string) storage.WorldStore {
				store, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{})
				if err != nil {
					t.Fatalf("OpenDisk: %v", err)
				}
				return store
			},
			loadSaved: func(t *testing.T, root string) storage.StoredHostileMobs {
				// 生命周期关服已关闭 store：读取走临时句柄并在返回前关闭，
				// 世界锁不得跨越到下一段生命周期的 OpenDisk。
				store, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{})
				if err != nil {
					t.Fatalf("OpenDisk after shutdown: %v", err)
				}
				defer func() {
					if err := store.Close(); err != nil {
						t.Errorf("read store Close: %v", err)
					}
				}()
				return loadHostileRecordsForRestart(t, store)
			},
		},
	} {
		t.Run(variant.name, func(t *testing.T) {
			root := variant.makeWorld(t)

			// ---------- 生命周期 1：恢复 → 路径重算 → 关服落盘 ----------
			store1 := variant.openLifetimeStore(t, root)
			host1 := mustNewHost(t, hostileRestartConfig(), flatTestGenerator{}, store1)
			closeHostileLifetime(t, host1)

			assertHostileRestartPreTick(t, host1, hostileRestartSeedRecords(target.PlayerID))

			login := startMemoryLogin(t, host1, target)
			assertHostileReplanAfterRestore(t, host1, target.PlayerID, login.Client)

			if err := host1.Shutdown(shutdownContextForHostileRestart(t)); err != nil {
				t.Fatalf("首次关服: %v", err)
			}
			// 关服屏障写最新权威快照：磁盘内容与引擎终态逐字段一致。
			final := hostileRestartStorageRecords(host1)
			saved := variant.loadSaved(t, root)
			if !reflect.DeepEqual(saved.Records, final) {
				t.Fatalf("关服落盘=%+v，想要引擎终态 %+v", saved.Records, final)
			}

			// ---------- 生命周期 2：重启恢复，重启不清怪 ----------
			store2 := variant.openLifetimeStore(t, root)
			host2 := mustNewHost(t, hostileRestartConfig(), flatTestGenerator{}, store2)
			closeHostileLifetime(t, host2)

			if tick := host2.world.TickCount(); tick != 0 {
				t.Fatalf("第二段恢复发生在 tick %d 之后，想要首 tick 前", tick)
			}
			// 恢复集合与第一段落盘逐字段一致（含全部种子个体 → 重启不清怪）。
			assertHostilesRestored(t, host2.world.engine.HostileMobs(), saved.Records)
			// 路径与规划槽位是运行时派生物：重启后不得携带任何派生状态。
			host2.world.stepMu.Lock()
			slots := len(host2.world.hostileManager.slots)
			host2.world.stepMu.Unlock()
			if slots != 0 {
				t.Fatalf("重启后管理器携带 %d 个槽位，想要空", slots)
			}

			login2 := startMemoryLogin(t, host2, target)
			assertHostileReplanAfterRestore(t, host2, target.PlayerID, login2.Client)
		})
	}
}

// reusableHostileMemoryStore 模拟两段生命周期共享同一内存 archive；宿主的
// Close 屏障不销毁该测试夹具，Disk 变体仍以真实重开覆盖关闭语义。
type reusableHostileMemoryStore struct {
	*hostTestStore
}

func (*reusableHostileMemoryStore) Close() error { return nil }

// seedHostileMobsForRestart 把种子夜行者写入指定世界存档。
func seedHostileMobsForRestart(t *testing.T, store storage.WorldStore) {
	t.Helper()
	if err := store.SaveHostileMobs(context.Background(), storage.HostileMobsSave{
		Revision: 2,
		Records:  hostileRestartSeedRecords(playerIdentity(7).PlayerID),
	}); err != nil {
		t.Fatalf("seed SaveHostileMobs: %v", err)
	}
}

func loadHostileRecordsForRestart(t *testing.T, store storage.WorldStore) storage.StoredHostileMobs {
	t.Helper()
	loaded, err := store.LoadHostileMobs(context.Background())
	if err != nil {
		t.Fatalf("LoadHostileMobs: %v", err)
	}
	return loaded
}

func closeHostileLifetime(t *testing.T, host *Host) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), longWaitDeadline)
		defer cancel()
		if err := host.Shutdown(ctx); err != nil {
			t.Errorf("Host cleanup Shutdown: %v", err)
		}
	})
}

func shutdownContextForHostileRestart(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), longWaitDeadline)
	t.Cleanup(cancel)
	return ctx
}

// hostileRestartStorageRecords 在关服后读取引擎终态并转换为存档记录形态。
func hostileRestartStorageRecords(host *Host) []storage.StoredHostileMob {
	mobs := host.world.engine.HostileMobs()
	records := make([]storage.StoredHostileMob, 0, len(mobs))
	for _, mob := range mobs {
		records = append(records, storage.StoredHostileMob{
			ID:              mob.ID,
			Dimension:       mob.Dimension,
			Position:        [3]float32(mob.State.Position),
			Velocity:        [3]float32(mob.State.Velocity),
			OnGround:        mob.State.OnGround,
			Yaw:             mob.Yaw,
			Health:          mob.Health,
			AttackCooldown:  mob.AttackCooldown,
			HurtCooldown:    mob.HurtCooldown,
			BurnCooldown:    mob.BurnCooldown,
			HasTarget:       mob.HasTarget,
			PlayerID:        mob.PlayerID,
			NextRepathTicks: mob.NextRepathTicks,
			DistantTicks:    mob.DistantTicks,
		})
	}
	return records
}

// assertHostileRestartPreTick 断言首 tick 前的恢复结果：记录逐字段一致且
// 管理器槽位为空（派生物不存在于存档，也不得凭空预建）。
func assertHostileRestartPreTick(t *testing.T, host *Host, want []storage.StoredHostileMob) {
	t.Helper()
	if tick := host.world.TickCount(); tick != 0 {
		t.Fatalf("恢复发生在 tick %d 之后，想要首 tick 前", tick)
	}
	assertHostilesRestored(t, host.world.engine.HostileMobs(), want)
	host.world.stepMu.Lock()
	slots := len(host.world.hostileManager.slots)
	host.world.stepMu.Unlock()
	if slots != 0 {
		t.Fatalf("首 tick 前管理器已有 %d 个槽位，想要空", slots)
	}
}

// assertHostileReplanAfterRestore 等目标玩家激活并推进权威 tick，直到恢复
// 个体的追逐路径被重新计算并应用：路径不落盘，运行中出现的任何路径都来自
// 按当前世界的重算，这正是「path 不恢复且首 tick 重算」的可观察面。
func assertHostileReplanAfterRestore(
	t *testing.T,
	host *Host,
	target core.PlayerID,
	client network.ClientEndpoint,
) {
	t.Helper()
	session := activeLoginForPlayer(t, host, target).Session
	deadline := time.Now().Add(longWaitDeadline)
	for {
		result := host.world.StepForTest()
		drainHostileTickMessages(t, client, result.Tick)
		host.world.stepMu.Lock()
		ready := false
		if player, ok := host.world.engine.Player(session); ok {
			ready = player.Ready
		}
		slot := host.world.hostileManager.slots[hostileRestartChaserID]
		replanned := slot != nil && slot.path != nil
		host.world.stepMu.Unlock()
		if ready && replanned {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("恢复夜行者 %d 未在预算内重算路径（ready=%v）", hostileRestartChaserID, ready)
		}
		time.Sleep(time.Millisecond)
	}
}

func drainHostileTickMessages(t *testing.T, endpoint network.ClientEndpoint, tick uint64) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	for {
		message, err := endpoint.Recv(ctx)
		if err != nil {
			t.Fatalf("接收服务端消息: %v", err)
		}
		if state, ok := message.(network.PlayerState); ok && state.ServerTick == tick {
			return
		}
	}
}
