// 敌怪 kind 的持久化契约：`contract.HostileMob.Kind` 随存档记录双向携带
// （v2 记录尾部 kind 字节，真实 MemoryStore 编码路径往返）、观察差异把
// kind 变化判脏、射击冷却作为瞬态不进存档（恢复恒就绪）。
package persistence

import (
	"context"
	"reflect"
	"testing"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

func TestHostilePersistenceCarriesKindRoundTrip(t *testing.T) {
	// 走真实 MemoryStore 的完整编码路径：掷骨者观察 → Flush → 重新加载必须
	// 原样携带 kind，恢复接线回到权威值快照且射击冷却就绪。
	store := storage.NewMemory(storage.Metadata{FormatVersion: 6, Seed: 42, DepthsSpawnAnchor: core.ChunkPos{}, DepthsSeedSalt: 0x9E3779B97F4A7C15})
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(p.Close)

	mob := hostileObserveFixture(5, 10)
	mob.Kind = contract.HostileKindBoneThrower
	p.Observe([]contract.HostileMob{mob})
	if err := p.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadHostileMobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.Records[0].Kind; got != contract.HostileKindBoneThrower {
		t.Fatalf("存档记录 kind=%d，想要掷骨者 %d", got, contract.HostileKindBoneThrower)
	}
	restored := p.Restore()
	if len(restored) != 1 || restored[0].Kind != contract.HostileKindBoneThrower {
		t.Fatalf("恢复快照=%+v，想要 kind=掷骨者", restored)
	}
	if restored[0].ShootCooldown != 0 {
		t.Fatalf("恢复快照射击冷却=%d，想要就绪态 0", restored[0].ShootCooldown)
	}
}

func TestHostilePersistenceObserveDirtiesOnKindChange(t *testing.T) {
	store := newControllableHostileStore()
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(p.Close)

	p.Observe([]contract.HostileMob{hostileObserveFixture(5, 10)})
	pollHostilePersistenceUntil(t, p, 10, func() bool { return len(store.started) != 0 })
	first := receiveHostileSave(t, store)
	if got := first.Records[0].Kind; got != contract.HostileKindNightwalker {
		t.Fatalf("首存 kind=%d，想要夜行者", got)
	}
	store.complete(nil)

	// 同一个体只变 kind（夜行者 → 掷骨者）：字段差异必须判脏并再次落盘。
	// 完成回收与重派发按既有节奏经 Poll 收敛（completion 送达与派发是两次
	// 独立的 Poll 步，轮询至 started 再次出现）。
	mob := hostileObserveFixture(5, 10)
	mob.Kind = contract.HostileKindBoneThrower
	p.Observe([]contract.HostileMob{mob})
	pollHostilePersistenceUntil(t, p, 20, func() bool { return len(store.started) != 0 })
	second := receiveHostileSave(t, store)
	if got := second.Records[0].Kind; got != contract.HostileKindBoneThrower {
		t.Fatalf("kind 变化未落盘：存档 kind=%d，想要掷骨者", got)
	}
	store.complete(nil)
}

func TestHostileStorageRecordIgnoresShootCooldown(t *testing.T) {
	// 射击冷却瞬态不入存档：storage record 转换不携带它，恢复侧恒就绪。
	mob := hostileObserveFixture(7, 3)
	mob.Kind = contract.HostileKindBoneThrower
	mob.ShootCooldown = 17
	record := hostileStorageRecord(mob)
	restored := hostileRestoreRecord(record)
	if restored.ShootCooldown != 0 {
		t.Fatalf("存档往返后的射击冷却=%d，想要就绪态 0", restored.ShootCooldown)
	}
	if restored.Kind != contract.HostileKindBoneThrower {
		t.Fatalf("存档往返后的 kind=%d，想要掷骨者", restored.Kind)
	}
	// 存档记录本身不含冷却字段（结构面对齐 storage.StoredHostileMob）。
	want := hostileStorageRecord(hostileObserveFixture(7, 3))
	want.Kind = contract.HostileKindBoneThrower
	if !reflect.DeepEqual(record, want) {
		t.Fatalf("含冷却的记录=%+v，想要与冷却无关的 %+v", record, want)
	}
}
