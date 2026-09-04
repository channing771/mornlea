// 被动牛持久化的启动矩阵与跨重启恢复测试：missing→空集合、valid→首 tick
// 前逐字段恢复、corrupt/future/read error→`NewHost` 以错误启动失败且绝不
// 以空集合覆盖旧文件、重复/超上限→拒绝且不截断；Memory 与 Disk 两条存储
// 路径都覆盖完整重启闭环。本文件只测装配与恢复语义，持久化 worker 的
// 并发行为见 persistence 子包的被动测试。
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

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

// passiveRestoreFixture 返回三条字段各异的合法存档记录，供恢复用例逐字段
// 比对。
func passiveRestoreFixture() []storage.StoredPassiveMob {
	return []storage.StoredPassiveMob{
		{
			ID: 5, Dimension: core.Overworld,
			Position: [3]float32{1.5, 2, 2.5}, Velocity: [3]float32{0.25, -0.5, 0},
			OnGround: true, Yaw: 0.75, Health: 10,
		},
		{
			ID: 8, Dimension: core.Overworld,
			Position: [3]float32{-4.5, 63.5, 6.25}, Velocity: [3]float32{0, 0, 0},
			OnGround: false, Yaw: -1.5, Health: core.MaxHealth,
		},
		{
			ID: 11, Dimension: core.Overworld,
			Position: [3]float32{12.5, 68, -6.5}, Velocity: [3]float32{-1, 0.5, 1.5},
			OnGround: true, Yaw: 2, Health: 1,
		},
	}
}

// assertPassivesRestored 逐字段断言权威侧被动牛集合与存档记录一致（按 ID
// 升序），覆盖位置/速度/生命/朝向的全部持久化字段。逃跑计时与出生区块是
// 运行时派生物，不存在于记录或权威快照中，因此天然不参与比对。
func assertPassivesRestored(t *testing.T, mobs []contract.PassiveMob, want []storage.StoredPassiveMob) {
	t.Helper()
	if len(mobs) != len(want) {
		t.Fatalf("被动牛数量=%d，想要 %d（记录=%v）", len(mobs), len(want), mobs)
	}
	for index, record := range want {
		mob := mobs[index]
		got := storage.StoredPassiveMob{
			ID: mob.ID, Dimension: mob.Dimension,
			Position: [3]float32(mob.State.Position), Velocity: [3]float32(mob.State.Velocity),
			OnGround: mob.State.OnGround, Yaw: mob.Yaw, Health: mob.Health,
		}
		if got != record {
			t.Fatalf("第 %d 头被动牛=%+v，想要存档记录 %+v", index, got, record)
		}
	}
}

func TestPassiveStartupMissingArchiveStartsEmpty(t *testing.T) {
	host := newTestHost(t)
	// 缺失存档视同空集合：首 tick 前权威侧没有任何被动牛，宿主照常可用。
	if got := host.world.engine.PassiveMobs(); len(got) != 0 {
		t.Fatalf("缺失存档时被动牛=%v，想要空集合", got)
	}
}

func TestPassiveStartupRestoresAllRecordsBeforeFirstTick(t *testing.T) {
	store := newHostTestStore()
	if err := store.SavePassiveMobs(context.Background(), storage.PassiveMobsSave{
		Revision: 4,
		Records:  passiveRestoreFixture(),
	}); err != nil {
		t.Fatalf("seed SavePassiveMobs: %v", err)
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
	// 逐字段回到权威侧，顺序按 ID 升序。
	if tick := host.world.TickCount(); tick != 0 {
		t.Fatalf("恢复发生在 tick %d 之后，想要首 tick 前", tick)
	}
	assertPassivesRestored(t, host.world.engine.PassiveMobs(), passiveRestoreFixture())
}

func TestPassiveStartupRejectsCorruptArchiveWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	seeded := seedPassiveDiskWorld(t, root)

	// 破坏 payload 的最后一个字节：CRC 不再匹配，整份文件按损坏拒绝。
	corrupt := bytes.Clone(seeded)
	corrupt[len(corrupt)-1] ^= 0xff
	writePassiveArchive(t, root, corrupt)

	store := openHostileWorldStore(t, root)
	if _, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store); err == nil {
		t.Fatal("损坏的被动牛存档被接受，想要 NewHost 启动失败")
	}
	// 「原样保留」的基线是损坏字节本身：启动失败不得触发任何保存（包括把
	// 空集合或修复版写回磁盘）。
	assertPassiveArchiveUnchanged(t, root, corrupt)
}

func TestPassiveStartupRejectsFutureSchemaWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	seedPassiveDiskWorld(t, root)

	// 手工构造 schema=2 的未来版本文件：头布局与被动存档 CRC 的覆盖范围镜
	// 像（[8:28] 段加 payload），CRC 因此合法，拒绝必须来自版本门禁而不
	// 是损坏。
	future := make([]byte, 32)
	copy(future, "PMST")
	binary.LittleEndian.PutUint32(future[4:], 1)
	binary.LittleEndian.PutUint32(future[8:], 2)
	binary.LittleEndian.PutUint64(future[12:], 1)
	binary.LittleEndian.PutUint32(future[20:], 0)
	binary.LittleEndian.PutUint32(future[24:], 0)
	table := crc32.MakeTable(crc32.Castagnoli)
	hasher := crc32.New(table)
	_, _ = hasher.Write(future[8:28])
	binary.LittleEndian.PutUint32(future[28:], hasher.Sum32())
	writePassiveArchive(t, root, future)

	store := openHostileWorldStore(t, root)
	if _, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store); err == nil {
		t.Fatal("未来版本的被动牛存档被接受，想要 NewHost 启动失败")
	}
	assertPassiveArchiveUnchanged(t, root, future)
}

func TestPassiveStartupRejectsArchiveReadError(t *testing.T) {
	root := t.TempDir()
	seedPassiveDiskWorld(t, root)

	// 用同名目录替换正式文件：读取路径必然失败（读错误与损坏不同，文件
	// 内容无从谈起，只要求启动失败）。
	if err := os.Remove(filepath.Join(root, "passive_mobs.bin")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "passive_mobs.bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	store := openHostileWorldStore(t, root)
	if _, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store); err == nil {
		t.Fatal("读取失败的被动牛存档被容忍，想要 NewHost 启动失败")
	}
}

func TestPassiveStartupRejectsDuplicateAndOverlimitWithoutTruncation(t *testing.T) {
	for name, records := range map[string][]storage.StoredPassiveMob{
		// 重复 ID：恢复侧按「拒绝整份」处理，绝不静默去重。
		"duplicate": {
			passiveRestoreFixture()[0], passiveRestoreFixture()[1],
			{ID: 5, Dimension: core.Overworld, Position: [3]float32{9.5, 1, 9.5},
				Health: core.MaxHealth},
		},
		// 33 条合法记录：超过权威容量，拒绝且绝不截断保留前 32 条。
		"over-limit": passiveOverlimitFixture(33),
	} {
		t.Run(name, func(t *testing.T) {
			store := &passiveLoadOverrideStore{hostTestStore: newHostTestStore()}
			store.loaded = storage.StoredPassiveMobs{Revision: 1, Records: records}
			if _, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store); err == nil {
				t.Fatalf("%s 存档被接受，想要 NewHost 启动失败", name)
			}
		})
	}
}

// 构造失败发生在被动牛持久化 worker 启动之后：恢复阶段拒绝必须在返回错误前
// 停掉 worker，否则每次启动失败都泄漏一个永不退出的 goroutine。
func TestNewHostConstructionErrorStopsPassivePersistenceWorker(t *testing.T) {
	baseline := runtime.NumGoroutine()
	store := &passiveLoadOverrideStore{hostTestStore: newHostTestStore()}
	store.loaded = storage.StoredPassiveMobs{Revision: 1, Records: []storage.StoredPassiveMob{
		passiveRestoreFixture()[0],
		// 与首条重复的 ID：加载边界不拦截（注入），恢复阶段整体拒绝，
		// `newWorld` 以错误返回——这正是 worker 泄漏的错误路径。
		passiveRestoreFixture()[0],
	}}
	if _, err := NewHost(context.Background(), hostTestConfig(), flatTestGenerator{}, store); err == nil {
		t.Fatal("重复 ID 存档被接受，想要 NewHost 启动失败")
	}
	waitForGoroutineCeiling(t, baseline)
}

// passiveOverlimitFixture 返回 count 条逐字段合法、ID 严格升序的记录。
func passiveOverlimitFixture(count int) []storage.StoredPassiveMob {
	records := make([]storage.StoredPassiveMob, 0, count)
	for id := 1; id <= count; id++ {
		records = append(records, storage.StoredPassiveMob{
			ID: uint64(id), Dimension: core.Overworld,
			Position: [3]float32{float32(id), 1, float32(-id)},
			OnGround: true,
			Health:   core.MaxHealth,
		})
	}
	return records
}

// seedPassiveDiskWorld 在 root 建档并写入一份合法被动牛存档，返回磁盘上
// 的正式文件字节（供损坏用例与不变断言使用）。
func seedPassiveDiskWorld(t *testing.T, root string) []byte {
	t.Helper()
	store, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{
		Create: storage.Metadata{FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld},
	})
	if err != nil {
		t.Fatalf("OpenDisk 种子存档: %v", err)
	}
	if err := store.SavePassiveMobs(context.Background(), storage.PassiveMobsSave{
		Revision: 2, Records: passiveRestoreFixture(),
	}); err != nil {
		t.Fatalf("seed SavePassiveMobs: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close seed store: %v", err)
	}
	seeded, err := os.ReadFile(filepath.Join(root, "passive_mobs.bin"))
	if err != nil {
		t.Fatal(err)
	}
	return seeded
}

func writePassiveArchive(t *testing.T, root string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "passive_mobs.bin"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertPassiveArchiveUnchanged(t *testing.T, root string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(root, "passive_mobs.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("启动失败后被动牛存档被改写，想要原样保留（不得以空集合覆盖）")
	}
}

// passiveLoadOverrideStore 在既有 Memory 测试存档上覆盖被动牛加载结果：
// 存储层 codec 自身会拒绝重复与超限集合，只有在加载边界注入才能把这类
// 非 conforming 载荷送达恢复路径，验证「拒绝且不截断」由恢复侧兜底。
type passiveLoadOverrideStore struct {
	*hostTestStore
	loaded storage.StoredPassiveMobs
}

func (store *passiveLoadOverrideStore) LoadPassiveMobs(context.Context) (storage.StoredPassiveMobs, error) {
	return store.loaded, nil
}
