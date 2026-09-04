// 被动牛持久化 worker 的非阻塞与故障注入测试：慢保存不持锁不阻 tick、
// in-flight 期间的观察合并 latest、失败按既有退避重试且重试载荷不与存储
// 输入别名、关服 `Flush` 收敛最新快照。并发形状与 hostile_persistence_test.go 同构，
// 但收敛到被动牛必需的最小覆盖。
package persistence

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/contract"
	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

func TestPassivePersistenceRoundTripViaMemoryStore(t *testing.T) {
	// 走真实 MemoryStore 的完整编码路径：观察 → Flush → 重新加载必须逐字段
	// 一致，不经任何假存档。
	store := storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: 42})
	p := NewPassives(store, storage.StoredPassiveMobs{}, passivePersistenceTestOptions())
	t.Cleanup(p.Close)
	p.Observe([]contract.PassiveMob{
		passiveObserveFixture(5, 10),
		passiveObserveFixture(6, 20),
	})
	if err := p.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	loaded, err := store.LoadPassiveMobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []storage.StoredPassiveMob{
		passiveStorageRecord(passiveObserveFixture(5, 10)),
		passiveStorageRecord(passiveObserveFixture(6, 20)),
	}
	if loaded.Revision != 1 || !reflect.DeepEqual(loaded.Records, want) {
		t.Fatalf("round-trip=%+v，想要 revision 1 records %+v", loaded, want)
	}
}

func TestPassivePersistenceRestoreIsSortedDeepCopy(t *testing.T) {
	store := newControllablePassiveStore()
	loaded := storage.StoredPassiveMobs{
		Revision: 4,
		Records: []storage.StoredPassiveMob{
			passiveStorageRecord(passiveObserveFixture(9, 30)),
			passiveStorageRecord(passiveObserveFixture(3, 10)),
		},
	}
	p := NewPassives(store, loaded, passivePersistenceTestOptions())
	t.Cleanup(p.Close)
	restored := p.Restore()
	if len(restored) != 2 || restored[0].ID != 3 || restored[1].ID != 9 {
		t.Fatalf("Restore=%+v，想要按 ID 升序", restored)
	}
	restored[0].Health = 1
	again := p.Restore()
	if again[0].Health == 1 {
		t.Fatal("Restore 返回了内部快照的别名")
	}
}

func TestPassivePersistenceCoalescesToOneInflightSave(t *testing.T) {
	store := newControllablePassiveStore()
	p := NewPassives(store, storage.StoredPassiveMobs{}, passivePersistenceTestOptions())
	t.Cleanup(p.Close)

	// 首次保存派发后 store 挂起不返回：Poll 立即返回（tick 不等落盘），
	// 期间不会并发第二份保存。
	p.Observe([]contract.PassiveMob{passiveObserveFixture(5, 10)})
	if err := p.Poll(10); err != nil {
		t.Fatal(err)
	}
	first := receivePassiveSave(t, store)
	if first.Revision != 1 || first.Records[0].Position[0] != 10 {
		t.Fatalf("first save=%+v", first)
	}
	p.Observe([]contract.PassiveMob{passiveObserveFixture(5, 20)})
	if err := p.Poll(20); err != nil {
		t.Fatal(err)
	}
	assertNoPassiveSave(t, store)

	// 保存完成后，in-flight 期间合并的 latest 随下一份保存整体落盘。
	store.complete(nil)
	pollPassivePersistenceUntil(t, p, 20, func() bool { return len(store.started) != 0 })
	second := receivePassiveSave(t, store)
	if second.Revision != 2 || second.Records[0].Position[0] != 20 {
		t.Fatalf("coalesced save=%+v，想要 revision 2 且位置 20", second)
	}
	store.complete(nil)
}

func TestPassivePersistenceSaveFailureRetainsDirtyAndRetriesAtTick(t *testing.T) {
	store := newControllablePassiveStore()
	p := NewPassives(store, storage.StoredPassiveMobs{}, passivePersistenceTestOptions())
	t.Cleanup(p.Close)
	p.Observe([]contract.PassiveMob{passiveObserveFixture(5, 10)})
	_ = p.Poll(10)
	first := receivePassiveSave(t, store)
	store.complete(errors.New("disk full"))
	if err := pollPassivePersistenceError(t, p, 10); err == nil {
		t.Fatal("save failure was not surfaced")
	}
	// 退避窗口内不重试，到期才以冻结的原载荷重派。
	if err := p.Poll(11); err != nil {
		t.Fatal(err)
	}
	assertNoPassiveSave(t, store)
	if err := p.Poll(12); err != nil {
		t.Fatal(err)
	}
	retry := receivePassiveSave(t, store)
	if !reflect.DeepEqual(retry, first) {
		t.Fatalf("retry=%+v, want frozen %+v", retry, first)
	}
	store.complete(nil)
}

func TestPassivePersistenceFlushWaitsForInflightAndWritesLatestOnce(t *testing.T) {
	store := newControllablePassiveStore()
	p := NewPassives(store, storage.StoredPassiveMobs{}, passivePersistenceTestOptions())
	t.Cleanup(p.Close)
	p.Observe([]contract.PassiveMob{passiveObserveFixture(5, 10)})
	if err := p.Poll(10); err != nil {
		t.Fatal(err)
	}
	_ = receivePassiveSave(t, store)

	// 关服屏障等待在途保存，再把期间观察到的最新快照补写一份后返回。
	flushed := make(chan error, 1)
	go func() { flushed <- p.Flush(context.Background()) }()
	p.Observe([]contract.PassiveMob{passiveObserveFixture(5, 20)})
	store.complete(nil)
	followup := receivePassiveSaveBeforeFlushReturns(t, store, flushed)
	if followup.Revision != 2 || followup.Records[0].Position[0] != 20 {
		t.Fatalf("follow-up save=%+v, want revision 2 position 20", followup)
	}
	store.complete(nil)
	select {
	case err := <-flushed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(passivePersistenceTestDeadline):
		t.Fatal("Flush did not return after its single follow-up")
	}
	assertNoPassiveSave(t, store)
}

func TestPassivePersistenceObserveBeyondLimitPanics(t *testing.T) {
	store := newControllablePassiveStore()
	p := NewPassives(store, storage.StoredPassiveMobs{}, passivePersistenceTestOptions())
	t.Cleanup(p.Close)
	oversized := make([]contract.PassiveMob, storage.MaxPassiveMobs+1)
	for index := range oversized {
		oversized[index] = passiveObserveFixture(uint64(index)+1, float32(index))
	}
	defer func() {
		if recovered := recover(); recovered == nil {
			t.Fatal("超过上限的 Observe 没有 panic")
		}
	}()
	p.Observe(oversized)
}

// passiveObserveFixture 返回一条位置可变的合法权威被动牛快照。
func passiveObserveFixture(id uint64, x float32) contract.PassiveMob {
	return contract.PassiveMob{
		ID:        id,
		Dimension: core.Overworld,
		State: physics.State{
			Position: mgl32.Vec3{x, 1, 0.5},
			OnGround: true,
		},
		Yaw:    0.5,
		Health: core.MaxHealth,
	}
}

type controllablePassiveStore struct {
	mu      sync.Mutex
	started chan storage.PassiveMobsSave
	results chan error
	onSave  func()
}

const passivePersistenceTestDeadline = time.Second

func newControllablePassiveStore() *controllablePassiveStore {
	return &controllablePassiveStore{
		started: make(chan storage.PassiveMobsSave, 4),
		results: make(chan error),
	}
}

func (store *controllablePassiveStore) LoadPassiveMobs(context.Context) (storage.StoredPassiveMobs, error) {
	return storage.StoredPassiveMobs{}, storage.ErrPassiveMobsNotFound
}

func (store *controllablePassiveStore) SavePassiveMobs(ctx context.Context, save storage.PassiveMobsSave) error {
	store.mu.Lock()
	onSave := store.onSave
	store.mu.Unlock()
	snapshot := clonePassiveSaveForTest(save)
	select {
	case store.started <- snapshot:
	case <-ctx.Done():
		return ctx.Err()
	}
	if onSave != nil {
		onSave()
	}
	select {
	case err := <-store.results:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (store *controllablePassiveStore) complete(err error) { store.results <- err }

func clonePassiveSaveForTest(save storage.PassiveMobsSave) storage.PassiveMobsSave {
	snapshot := save
	snapshot.Records = append([]storage.StoredPassiveMob(nil), save.Records...)
	return snapshot
}

func passivePersistenceTestOptions() Options {
	return Options{
		AutosaveTicks:  10,
		RetryBaseTicks: 2,
		RetryMaxTicks:  8,
	}
}

func receivePassiveSave(t *testing.T, store *controllablePassiveStore) storage.PassiveMobsSave {
	t.Helper()
	select {
	case save := <-store.started:
		return save
	case <-time.After(waitDeadline):
		t.Fatal("SavePassiveMobs was not started")
		return storage.PassiveMobsSave{}
	}
}

func receivePassiveSaveBeforeFlushReturns(
	t *testing.T,
	store *controllablePassiveStore,
	flushed <-chan error,
) storage.PassiveMobsSave {
	t.Helper()
	select {
	case save := <-store.started:
		return save
	case err := <-flushed:
		t.Fatalf("Flush returned before required save: %v", err)
		return storage.PassiveMobsSave{}
	case <-time.After(passivePersistenceTestDeadline):
		t.Fatal("SavePassiveMobs was not started before Flush deadline")
		return storage.PassiveMobsSave{}
	}
}

func assertNoPassiveSave(t *testing.T, store *controllablePassiveStore) {
	t.Helper()
	select {
	case save := <-store.started:
		t.Fatalf("unexpected SavePassiveMobs(%+v)", save)
	case <-time.After(50 * time.Millisecond):
	}
}

func pollPassivePersistenceUntil(t *testing.T, p *Passives, tick uint64, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for !done() {
		if err := p.Poll(tick); err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("passive persistence did not reach expected state")
		}
		time.Sleep(time.Millisecond)
	}
}

func pollPassivePersistenceError(t *testing.T, p *Passives, tick uint64) error {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for {
		if err := p.Poll(tick); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			t.Fatal("passive persistence did not surface SavePassiveMobs error")
		}
		time.Sleep(time.Millisecond)
	}
}

var _ storage.PassiveMobStore = (*controllablePassiveStore)(nil)
