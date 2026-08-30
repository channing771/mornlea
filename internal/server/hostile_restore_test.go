// 夜行者持久化的启动矩阵与跨重启恢复测试：missing→空集合、valid→首 tick
// 前逐字段恢复、corrupt/future/read error→`NewHost` 以错误启动失败且绝不
// 以空集合覆盖旧文件、重复/超上限→拒绝且不截断；Memory 与 Disk 两条存储
// 路径都覆盖完整重启闭环。本文件只测装配与恢复语义，持久化 worker 的
// 并发行为见 hostile_persistence_test.go。
package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/sim/contract"
	"github.com/channing771/mornlea/internal/storage"
)

// hostileRestoreFixture 返回三条字段各异的合法存档记录（含追逐目标、冷却
// 与远离累计），供恢复用例逐字段比对。目标玩家 ID 使用合法 UUIDv4 形状。
func hostileRestoreFixture() []storage.StoredHostileMob {
	target := playerIdentity(7).PlayerID
	return []storage.StoredHostileMob{
		{
			ID: 3, Dimension: core.Overworld,
			Position: [3]float32{2.5, 1, 2.5}, Velocity: [3]float32{0.5, -1.25, 0},
			OnGround: true, Yaw: 1.25,
			Health: 14, AttackCooldown: 3, HurtCooldown: 1, BurnCooldown: 20,
			HasTarget: true, PlayerID: target,
			NextRepathTicks: 37, DistantTicks: 42,
		},
		{
			ID: 9, Dimension: core.Overworld,
			Position: [3]float32{-8.5, 65.5, 12.75}, Velocity: [3]float32{0, 0, 0},
			OnGround: false, Yaw: -2.5,
			Health: core.MaxHealth, BurnCooldown: 20,
			NextRepathTicks: 5, DistantTicks: 600,
		},
		{
			ID: 12, Dimension: core.Overworld,
			Position: [3]float32{30.5, 70, -3.25}, Velocity: [3]float32{-2, 0.25, 3},
			OnGround: true, Yaw: 3,
			Health: 1, AttackCooldown: 20, HurtCooldown: 20, BurnCooldown: 1,
			DistantTicks: 0,
		},
	}
}

// assertHostilesRestored 逐字段断言权威侧夜行者集合与存档记录一致（按 ID
// 升序），覆盖位置/速度/生命/冷却/目标/重规划节奏/远离累计的全部持久化
// 字段。路径是运行时派生物，不存在于记录或权威侧，因此天然不参与比对。
func assertHostilesRestored(t *testing.T, mobs []contract.HostileMob, want []storage.StoredHostileMob) {
	t.Helper()
	if len(mobs) != len(want) {
		t.Fatalf("夜行者数量=%d，想要 %d（记录=%v）", len(mobs), len(want), mobs)
	}
	for index, record := range want {
		mob := mobs[index]
		got := storage.StoredHostileMob{
			ID: mob.ID, Dimension: mob.Dimension,
			Position: [3]float32(mob.State.Position), Velocity: [3]float32(mob.State.Velocity),
			OnGround: mob.State.OnGround, Yaw: mob.Yaw,
			Health: mob.Health, AttackCooldown: mob.AttackCooldown,
			HurtCooldown: mob.HurtCooldown, BurnCooldown: mob.BurnCooldown,
			HasTarget: mob.HasTarget, PlayerID: mob.PlayerID,
			NextRepathTicks: mob.NextRepathTicks, DistantTicks: mob.DistantTicks,
		}
		if got != record {
			t.Fatalf("第 %d 只夜行者=%+v，想要存档记录 %+v", index, got, record)
		}
	}
}

func TestHostileStartupMissingArchiveStartsEmpty(t *testing.T) {
	host := newTestHost(t)
	// 缺失存档视同空集合：首 tick 前权威侧没有任何夜行者，宿主照常可用。
	if got := host.world.engine.HostileMobs(); len(got) != 0 {
		t.Fatalf("缺失存档时夜行者=%v，想要空集合", got)
	}
}

func TestHostileStartupRestoresAllRecordsBeforeFirstTick(t *testing.T) {
	store := newHostTestStore()
	if err := store.SaveHostileMobs(context.Background(), storage.HostileMobsSave{
		Revision: 4,
		Records:  hostileRestoreFixture(),
	}); err != nil {
		t.Fatalf("seed SaveHostileMobs: %v", err)
	}
	host := mustNewHost(t, hostTestConfig(), flatTestGenerator{}, store)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
		defer cancel()
		if err := host.Shutdown(ctx); err != nil {
			t.Errorf("Host cleanup Shutdown: %v", err)
		}
	})

	// 恢复必须发生在第一个权威 tick 之前：尚未推进任何 tick，全部记录已
	// 逐字段回到权威侧（含目标与冷却），顺序按 ID 升序。
	if tick := host.world.TickCount(); tick != 0 {
		t.Fatalf("恢复发生在 tick %d 之后，想要首 tick 前", tick)
	}
	assertHostilesRestored(t, host.world.engine.HostileMobs(), hostileRestoreFixture())
}

func TestHostileStartupRejectsCorruptArchiveWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	seeded := seedHostileDiskWorld(t, root)

	// 破坏 payload 的最后一个字节：CRC 不再匹配，整份文件按损坏拒绝。
	corrupt := bytes.Clone(seeded)
	corrupt[len(corrupt)-1] ^= 0xff
	writeHostileArchive(t, root, corrupt)

	store := openHostileWorldStore(t, root)
	if _, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store); err == nil {
		t.Fatal("损坏的夜行者存档被接受，想要 NewHost 启动失败")
	}
	// 「原样保留」的基线是损坏字节本身：启动失败不得触发任何保存（包括把
	// 空集合或修复版写回磁盘）。
	assertHostileArchiveUnchanged(t, root, corrupt)
}

func TestHostileStartupRejectsFutureSchemaWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	seedHostileDiskWorld(t, root)

	// 手工构造 schema=2 的未来版本文件：头布局与 `hostileChecksum` 的覆盖
	// 范围镜像（[8:28] 段加 payload），CRC 因此合法，拒绝必须来自版本门禁
	// 而不是损坏。
	future := make([]byte, 32)
	copy(future, "MHST")
	binary.LittleEndian.PutUint32(future[4:], 1)
	binary.LittleEndian.PutUint32(future[8:], 2)
	binary.LittleEndian.PutUint64(future[12:], 1)
	binary.LittleEndian.PutUint32(future[20:], 0)
	binary.LittleEndian.PutUint32(future[24:], 0)
	table := crc32.MakeTable(crc32.Castagnoli)
	hasher := crc32.New(table)
	_, _ = hasher.Write(future[8:28])
	binary.LittleEndian.PutUint32(future[28:], hasher.Sum32())
	writeHostileArchive(t, root, future)

	store := openHostileWorldStore(t, root)
	if _, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store); err == nil {
		t.Fatal("未来版本的夜行者存档被接受，想要 NewHost 启动失败")
	}
	assertHostileArchiveUnchanged(t, root, future)
}

func TestHostileStartupRejectsArchiveReadError(t *testing.T) {
	root := t.TempDir()
	seedHostileDiskWorld(t, root)

	// 用同名目录替换正式文件：读取路径必然失败（读错误与损坏不同，文件
	// 内容无从谈起，只要求启动失败）。
	if err := os.Remove(filepath.Join(root, "hostile_mobs.bin")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "hostile_mobs.bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := openHostileWorldStore(t, root)
	if _, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store); err == nil {
		t.Fatal("读取失败的夜行者存档被容忍，想要 NewHost 启动失败")
	}
}

func TestHostileStartupRejectsDuplicateAndOverlimitWithoutTruncation(t *testing.T) {
	for name, records := range map[string][]storage.StoredHostileMob{
		// 重复 ID：恢复侧按「拒绝整份」处理，绝不静默去重。
		"duplicate": {
			hostileRestoreFixture()[0], hostileRestoreFixture()[1],
			{ID: 3, Dimension: core.Overworld, Position: [3]float32{9.5, 1, 9.5},
				Health: core.MaxHealth, BurnCooldown: 20},
		},
		// 65 条合法记录：超过权威容量，拒绝且绝不截断保留前 64 条。
		"over-limit": hostileOverlimitFixture(65),
	} {
		t.Run(name, func(t *testing.T) {
			store := &hostileLoadOverrideStore{hostTestStore: newHostTestStore()}
			store.loaded = storage.StoredHostileMobs{Revision: 1, Records: records}
			if _, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store); err == nil {
				t.Fatalf("%s 存档被接受，想要 NewHost 启动失败", name)
			}
		})
	}
}

// 构造失败发生在夜行者持久化 worker 启动之后：恢复阶段拒绝必须在返回错误前
// 停掉 worker，否则每次启动失败都泄漏一个永不退出的 goroutine。
func TestNewHostConstructionErrorStopsHostilePersistenceWorker(t *testing.T) {
	baseline := runtime.NumGoroutine()
	store := &hostileLoadOverrideStore{hostTestStore: newHostTestStore()}
	store.loaded = storage.StoredHostileMobs{Revision: 1, Records: []storage.StoredHostileMob{
		hostileRestoreFixture()[0],
		// 与首条重复的 ID：加载边界不拦截（注入），恢复阶段整体拒绝，
		// `newWorld` 以错误返回——这正是 worker 泄漏的错误路径。
		hostileRestoreFixture()[0],
	}}
	if _, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store); err == nil {
		t.Fatal("重复 ID 存档被接受，想要 NewHost 启动失败")
	}
	waitForGoroutineCeiling(t, baseline)
}

// hostileOverlimitFixture 返回 count 条逐字段合法、ID 严格升序的记录。
func hostileOverlimitFixture(count int) []storage.StoredHostileMob {
	records := make([]storage.StoredHostileMob, 0, count)
	for id := 1; id <= count; id++ {
		records = append(records, storage.StoredHostileMob{
			ID: uint64(id), Dimension: core.Overworld,
			Position: [3]float32{float32(id), 1, float32(-id)},
			OnGround: true,
			Health:   core.MaxHealth, BurnCooldown: 20,
		})
	}
	return records
}

// seedHostileDiskWorld 在 root 建档并写入一份合法夜行者存档，返回磁盘上
// 的正式文件字节（供损坏用例与不变断言使用）。
func seedHostileDiskWorld(t *testing.T, root string) []byte {
	t.Helper()
	store, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{
		Create: storage.Metadata{FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld},
	})
	if err != nil {
		t.Fatalf("OpenDisk 种子存档: %v", err)
	}
	if err := store.SaveHostileMobs(context.Background(), storage.HostileMobsSave{
		Revision: 2, Records: hostileRestoreFixture(),
	}); err != nil {
		t.Fatalf("seed SaveHostileMobs: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}
	seeded, err := os.ReadFile(filepath.Join(root, "hostile_mobs.bin"))
	if err != nil {
		t.Fatal(err)
	}
	return seeded
}

func writeHostileArchive(t *testing.T, root string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "hostile_mobs.bin"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func openHostileWorldStore(t *testing.T, root string) storage.WorldStore {
	t.Helper()
	store, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{})
	if err != nil {
		t.Fatalf("OpenDisk: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("cleanup store Close: %v", err)
		}
	})
	return store
}

func assertHostileArchiveUnchanged(t *testing.T, root string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, "hostile_mobs.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("启动失败后夜行者存档被改写，想要原样保留（不得以空集合覆盖）")
	}
}

// hostileLoadOverrideStore 在既有 Memory 测试存档上覆盖夜行者加载结果：
// 存储层 codec 自身会拒绝重复与超限集合，只有在加载边界注入才能把这类
// 非 conforming 载荷送达恢复路径，验证「拒绝且不截断」由恢复侧兜底。
type hostileLoadOverrideStore struct {
	*hostTestStore
	loaded storage.StoredHostileMobs
}

func (store *hostileLoadOverrideStore) LoadHostileMobs(context.Context) (storage.StoredHostileMobs, error) {
	return store.loaded, nil
}
