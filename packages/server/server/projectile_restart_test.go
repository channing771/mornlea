// 投射物与掷骨者跨重启的集成测试：掷骨者（kind v2）跨重启逐字段保值、射击
// 冷却重启后回到就绪态、在飞投射物不持久化（重启后消失）、以及 v1 敌怪存档
// 的整链迁移（旧档读入 kind 恒 0 → 触发保存后文件升级 v2）。
package server

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

// projectileRestartHurlerID 是跨重启夹具中的掷骨者 ID。
const projectileRestartHurlerID = uint64(21)

// projectileRestartHurlerSeed 返回一条 kind=掷骨者的种子记录：无追逐目标、
// 重规划 tick 推到远端（管理器不派发 A*）、灼烧冷却就绪推进后即产生存档差异。
func projectileRestartHurlerSeed() storage.StoredHostileMob {
	return storage.StoredHostileMob{
		ID:              projectileRestartHurlerID,
		Dimension:       core.Overworld,
		Position:        [3]float32{2.5, 1, 2.5},
		OnGround:        true,
		Yaw:             1.25,
		Health:          17,
		BurnCooldown:    20,
		NextRepathTicks: ^uint64(0),
		Kind:            contract.HostileKindBoneThrower,
	}
}

func TestProjectileRestartKeepsHurlerAndDropsInFlight(t *testing.T) {
	target := playerIdentity(7)
	var memoryStore storage.WorldStore
	for _, variant := range []struct {
		name              string
		makeWorld         func(t *testing.T) string
		openLifetimeStore func(t *testing.T, root string) storage.WorldStore
		loadSaved         func(t *testing.T, root string) storage.StoredHostileMobs
	}{
		{
			name: "memory",
			makeWorld: func(t *testing.T) string {
				store := &reusableHostileMemoryStore{hostTestStore: &hostTestStore{
					MemoryStore: storage.NewMemory(hostileRestartMetadata()),
				}}
				seedProjectileRestartHurler(t, store)
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
				seedProjectileRestartHurler(t, store)
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
			want := []storage.StoredHostileMob{projectileRestartHurlerSeed()}

			// ---------- 生命周期 1：恢复 → 发射一条在飞骨刺 → 关服落盘 ----------
			store1 := variant.openLifetimeStore(t, root)
			host1 := mustNewHost(t, hostileRestartConfig(), flatTestGenerator{}, store1)
			closeHostileLifetime(t, host1)

			if tick := host1.world.TickCount(); tick != 0 {
				t.Fatalf("恢复发生在 tick %d 之后，想要首 tick 前", tick)
			}
			assertHostilesRestored(t, host1.world.engine.HostileMobs(), want)

			// 真实玩家登录并就绪：引擎视图建立后投射物才能在权威集合中存活
			//（离开全部会话订阅区的投射物会被同 tick 移除）。
			login := startMemoryLogin(t, host1, target)
			session := activeLoginForPlayer(t, host1, target.PlayerID).Session
			for {
				result := host1.world.StepForTest()
				drainHostileTickMessages(t, login.Client, result.Tick)
				if player, ok := host1.world.engine.Player(session); ok && player.Ready {
					break
				}
			}

			// 发射一条骨刺并推进一个权威 tick：在飞 1 条、冷却置满。
			if !host1.world.engine.EnqueueHostileAction(contract.HostileAction{
				ID:           projectileRestartHurlerID,
				RangedAttack: true,
				AimX:         1, AimY: 0, AimZ: 0,
			}) {
				t.Fatal("射击意图入队失败")
			}
			result := host1.world.StepForTest()
			drainHostileTickMessages(t, login.Client, result.Tick)
			if got := len(host1.world.engine.ProjectilesForTest()); got != 1 {
				t.Fatalf("发射后权威投射物=%d 条，想要 1 条", got)
			}
			if got := host1.world.engine.HostileMobs()[0].ShootCooldown; got == 0 {
				t.Fatal("发射后射击冷却仍为就绪，想要已置满")
			}

			if err := host1.Shutdown(shutdownContextForHostileRestart(t)); err != nil {
				t.Fatalf("首次关服: %v", err)
			}
			// 关服屏障写最新权威快照：掷骨者 kind 随 v2 记录落盘，射击冷却与
			// 在飞投射物都是瞬态，不出现在任何持久化载荷里。
			final := hostileRestartStorageRecords(host1)
			saved := variant.loadSaved(t, root)
			if !reflect.DeepEqual(saved.Records, final) {
				t.Fatalf("关服落盘=%+v，想要引擎终态 %+v", saved.Records, final)
			}

			// ---------- 生命周期 2：重启恢复，kind 保值、瞬态清零 ----------
			store2 := variant.openLifetimeStore(t, root)
			host2 := mustNewHost(t, hostileRestartConfig(), flatTestGenerator{}, store2)
			closeHostileLifetime(t, host2)

			if tick := host2.world.TickCount(); tick != 0 {
				t.Fatalf("第二段恢复发生在 tick %d 之后，想要首 tick 前", tick)
			}
			mobs := host2.world.engine.HostileMobs()
			assertHostilesRestored(t, mobs, saved.Records)
			if got := mobs[0].Kind; got != contract.HostileKindBoneThrower {
				t.Fatalf("重启后掷骨者 kind=%d，想要 %d", got, contract.HostileKindBoneThrower)
			}
			if got := mobs[0].ShootCooldown; got != 0 {
				t.Fatalf("重启后射击冷却=%d，想要就绪 0", got)
			}
			if got := len(host2.world.engine.ProjectilesForTest()); got != 0 {
				t.Fatalf("重启后在飞投射物=%d 条，想要 0（瞬态不持久化）", got)
			}
		})
	}
}

func seedProjectileRestartHurler(t *testing.T, store storage.WorldStore) {
	t.Helper()
	if err := store.SaveHostileMobs(context.Background(), storage.HostileMobsSave{
		Revision: 2,
		Records:  []storage.StoredHostileMob{projectileRestartHurlerSeed()},
	}); err != nil {
		t.Fatalf("seed SaveHostileMobs: %v", err)
	}
}

// hostileRestoreRecordList 已并入断言路径：关服落盘与引擎终态直接经
// `hostileRestartStorageRecords` 逐字段比对，无需再经中间形态。

// 敌怪存档头的冻结布局常量：与 storage/hostile codec 的固定字节布局一一对应
// （头 32 字节；v1 记录 72 字节，v2 记录在尾部追加 1 字节 kind 共 73 字节）。
// 本文件只读写这些偏移做版本迁移整链，字段级语义由 storage 侧测试锁定。
const (
	hostileArchiveHeaderLength          = 32
	hostileArchiveRecordLengthV1        = 72
	hostileArchiveRecordLengthV2        = 73
	hostileArchiveSchemaV1       uint32 = 1
	hostileArchiveSchemaV2       uint32 = 2
)

// downgradeHostileArchiveToV1 把当前编码器写出的 v2 存档降级为等价的 v1 旧档：
// 逐条剥掉记录尾部的 kind 字节、schema 改写 1、重算 payload 长度与 CRC。输入
// 必须是合法 v2 文件；降级产物经真实解码入口读回即触发只读迁移分支。
func downgradeHostileArchiveToV1(t *testing.T, v2 []byte) []byte {
	t.Helper()
	if schema := binary.LittleEndian.Uint32(v2[8:12]); schema != hostileArchiveSchemaV2 {
		t.Fatalf("降级输入 schema=%d，想要 v2", schema)
	}
	count := int(binary.LittleEndian.Uint32(v2[20:24]))
	if len(v2) != hostileArchiveHeaderLength+count*hostileArchiveRecordLengthV2 {
		t.Fatalf("降级输入长度=%d，与 count=%d 不符", len(v2), count)
	}
	payload := make([]byte, 0, count*hostileArchiveRecordLengthV1)
	for index := 0; index < count; index++ {
		start := hostileArchiveHeaderLength + index*hostileArchiveRecordLengthV2
		payload = append(payload, v2[start:start+hostileArchiveRecordLengthV1]...)
	}
	legacy := make([]byte, hostileArchiveHeaderLength, hostileArchiveHeaderLength+len(payload))
	copy(legacy, v2[:hostileArchiveHeaderLength])
	binary.LittleEndian.PutUint32(legacy[8:12], hostileArchiveSchemaV1)
	binary.LittleEndian.PutUint32(legacy[24:28], uint32(len(payload)))
	hasher := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	_, _ = hasher.Write(legacy[8:28])
	_, _ = hasher.Write(payload)
	binary.LittleEndian.PutUint32(legacy[28:32], hasher.Sum32())
	return append(legacy, payload...)
}

// TestHostileV1ArchiveMigratesToV2FullChain 覆盖 v1 旧档的整链迁移：真实 v1
// 文件读入 → 恢复后 kind 恒 0（夜行者）→ 首次落盘（关服屏障）把文件升级为
// v2，每条记录的迁移 kind 字节为 0。降级输入由当前编码器现场生成再手工降级，
// 保证字段布局始终跟随权威编码器。
func TestHostileV1ArchiveMigratesToV2FullChain(t *testing.T) {
	root := t.TempDir()
	seed := []storage.StoredHostileMob{
		{
			ID: 3, Dimension: core.Overworld,
			Position: [3]float32{2.5, 1, 2.5}, OnGround: true,
			Health: 15, BurnCooldown: 20,
		},
		{
			ID: 9, Dimension: core.Overworld,
			Position: [3]float32{-4.5, 1, 3.5}, OnGround: true,
			Health: 9, BurnCooldown: 20,
		},
	}
	writeStore, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{
		Create: hostileRestartMetadata(),
	})
	if err != nil {
		t.Fatalf("OpenDisk 种子存档: %v", err)
	}
	if err := writeStore.SaveHostileMobs(context.Background(), storage.HostileMobsSave{
		Revision: 5,
		Records:  seed,
	}); err != nil {
		t.Fatalf("seed SaveHostileMobs: %v", err)
	}
	if err := writeStore.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}
	v2Path := filepath.Join(root, "hostile_mobs.bin")
	encoded, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(v2Path, downgradeHostileArchiveToV1(t, encoded), 0o600); err != nil {
		t.Fatalf("写入 v1 旧档: %v", err)
	}

	// 读入：v1 只读迁移，全部恢复为夜行者（kind 恒 0）。
	host := mustNewHost(t, hostTestConfig(), flatTestGenerator{}, openHostileWorldStore(t, root))
	if tick := host.world.TickCount(); tick != 0 {
		t.Fatalf("恢复发生在 tick %d 之后，想要首 tick 前", tick)
	}
	mobs := host.world.engine.HostileMobs()
	assertHostilesRestored(t, mobs, seed)
	for index := range mobs {
		if got := mobs[index].Kind; got != contract.HostileKindNightwalker {
			t.Fatalf("v1 迁移记录 %d kind=%d，想要恒 0", index, got)
		}
	}

	// 制造存档差异（灼烧冷却递减）并关服：Flush 屏障把最新快照落盘，写侧只
	// 写当前 schema，文件随之升级 v2 且每条记录尾字节 kind=0。
	host.world.StepForTest()
	if err := host.Shutdown(shutdownContextForHostileRestart(t)); err != nil {
		t.Fatalf("关服: %v", err)
	}
	migrated, err := os.ReadFile(v2Path)
	if err != nil {
		t.Fatal(err)
	}
	if schema := binary.LittleEndian.Uint32(migrated[8:12]); schema != hostileArchiveSchemaV2 {
		t.Fatalf("迁移后文件 schema=%d，想要 v2", schema)
	}
	count := int(binary.LittleEndian.Uint32(migrated[20:24]))
	for index := 0; index < count; index++ {
		kind := migrated[hostileArchiveHeaderLength+index*hostileArchiveRecordLengthV2+hostileArchiveRecordLengthV1]
		if kind != 0 {
			t.Fatalf("迁移后记录 %d 的 kind 字节=%d，想要 0", index, kind)
		}
	}
	verify := openHostileWorldStore(t, root)
	loaded, err := verify.LoadHostileMobs(context.Background())
	if err != nil {
		t.Fatalf("LoadHostileMobs: %v", err)
	}
	if len(loaded.Records) != len(seed) {
		t.Fatalf("迁移后记录数=%d，想要 %d", len(loaded.Records), len(seed))
	}
	for index := range loaded.Records {
		if got := loaded.Records[index].Kind; got != contract.HostileKindNightwalker {
			t.Fatalf("迁移后解码记录 %d kind=%d，想要 0", index, got)
		}
	}
}
