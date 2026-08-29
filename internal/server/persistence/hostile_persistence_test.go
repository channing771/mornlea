// 夜行者持久化 worker 的非阻塞与故障注入测试：慢保存不持锁不阻 tick、
// in-flight 期间的观察合并 latest、失败按既有退避重试且重试载荷不与存储
// 输入别名、关服 `Flush` 收敛最新快照、调用方取消只中断等待且保留 worker
// 与重试状态。并发形状与 companion_persistence_test.go 同构。
package persistence

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
	"github.com/channing771/mornlea/internal/physics"
	"github.com/channing771/mornlea/internal/sim"
	"github.com/channing771/mornlea/internal/storage"
)

func TestHostilePersistenceCoalescesToOneInflightSave(t *testing.T) {
	store := newControllableHostileStore()
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(p.Close)

	// 首次保存派发后 store 挂起不返回：Poll 立即返回（tick 不等落盘），
	// 期间不会并发第二份保存。
	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 10)})
	if err := p.Poll(10); err != nil {
		t.Fatal(err)
	}
	first := receiveHostileSave(t, store)
	if first.Revision != 1 || first.Records[0].Position[0] != 10 {
		t.Fatalf("first save=%+v", first)
	}
	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 20)})
	if err := p.Poll(20); err != nil {
		t.Fatal(err)
	}
	assertNoHostileSave(t, store)

	// 保存完成后，in-flight 期间合并的 latest 随下一份保存整体落盘。
	store.complete(nil)
	pollHostilePersistenceUntil(t, p, 20, func() bool { return len(store.started) != 0 })
	second := receiveHostileSave(t, store)
	if second.Revision != 2 || second.Records[0].Position[0] != 20 {
		t.Fatalf("coalesced save=%+v，想要 revision 2 且位置 20", second)
	}
	store.complete(nil)
}

func TestHostilePersistenceDoesNotHoldMutexDuringStoreSave(t *testing.T) {
	store := newControllableHostileStore()
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(p.Close)
	mutexFree := make(chan bool, 1)
	store.setOnSave(func() {
		free := p.mu.TryLock()
		if free {
			p.mu.Unlock()
		}
		mutexFree <- free
	})
	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 10)})
	if err := p.Poll(10); err != nil {
		t.Fatal(err)
	}
	_ = receiveHostileSave(t, store)
	free := <-mutexFree
	store.complete(nil)
	if !free {
		t.Fatal("worker held mutex during SaveHostileMobs")
	}
}

func TestHostilePersistenceSaveFailureRetainsDirtyAndRetriesAtTick(t *testing.T) {
	store := newControllableHostileStore()
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(p.Close)
	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 10)})
	_ = p.Poll(10)
	first := receiveHostileSave(t, store)
	store.complete(errors.New("disk full"))
	if err := pollHostilePersistenceError(t, p, 10); err == nil {
		t.Fatal("save failure was not surfaced")
	}
	// 退避窗口内不重试，到期才以冻结的原载荷重派。
	if err := p.Poll(11); err != nil {
		t.Fatal(err)
	}
	assertNoHostileSave(t, store)
	if err := p.Poll(12); err != nil {
		t.Fatal(err)
	}
	retry := receiveHostileSave(t, store)
	if !reflect.DeepEqual(retry, first) {
		t.Fatalf("retry=%+v, want frozen %+v", retry, first)
	}
	store.complete(nil)
}

func TestHostilePersistenceRetryDoesNotAliasStoreSaveInput(t *testing.T) {
	store := newControllableHostileStore()
	store.mutateNextSave()
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(p.Close)
	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 10)})
	_ = p.Poll(10)
	mutated := receiveHostileSave(t, store)
	if mutated.Records[0].Position[0] != 999 {
		t.Fatal("test store did not mutate input")
	}
	store.complete(errors.New("disk full"))
	_ = pollHostilePersistenceError(t, p, 10)
	_ = p.Poll(12)
	retry := receiveHostileSave(t, store)
	if retry.Records[0].Position[0] != 10 {
		t.Fatalf("retry aliased store input: %+v", retry)
	}
	store.complete(nil)
}

func TestHostilePersistenceChangeDuringInflightRemainsDirty(t *testing.T) {
	store := newControllableHostileStore()
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(p.Close)
	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 10)})
	_ = p.Poll(10)
	_ = receiveHostileSave(t, store)
	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 20)})
	store.complete(nil)
	pollHostilePersistenceUntil(t, p, 11, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.dirty && !p.inFlight
	})
}

func TestHostilePersistenceFlushWaitsForInflightAndWritesLatestOnce(t *testing.T) {
	for _, test := range []struct {
		name      string
		inherited bool
	}{
		{name: "inherited", inherited: true},
		{name: "self dispatched"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newControllableHostileStore()
			p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
			t.Cleanup(p.Close)
			p.Observe([]sim.HostileMob{hostileObserveFixture(5, 10)})
			flushed := make(chan error, 1)
			if test.inherited {
				if err := p.Poll(10); err != nil {
					t.Fatal(err)
				}
				_ = receiveHostileSave(t, store)
				go func() { flushed <- p.Flush(context.Background()) }()
			} else {
				go func() { flushed <- p.Flush(context.Background()) }()
				first := receiveHostileSaveBeforeFlushReturns(t, store, flushed)
				if first.Revision != 1 || first.Records[0].Position[0] != 10 {
					t.Fatalf("first save=%+v", first)
				}
			}

			// 关服屏障写的是最新权威快照：继承或自派的首次保存之后发生的
			// 观察变化，由同一屏障的收尾保存补齐。
			p.Observe([]sim.HostileMob{hostileObserveFixture(5, 20)})
			store.complete(nil)
			followup := receiveHostileSaveBeforeFlushReturns(t, store, flushed)
			if followup.Revision != 2 || followup.Records[0].Position[0] != 20 {
				t.Fatalf("follow-up save=%+v, want revision 2 position 20", followup)
			}
			// 收尾保存等待期间的新观察不被同一屏障吞掉：单次 Flush 至多补
			// 一份 follow-up，随后返回（第三次变化留给下一次 Flush）。
			p.Observe([]sim.HostileMob{hostileObserveFixture(5, 30)})
			store.complete(nil)
			select {
			case save := <-store.started:
				t.Fatalf("same Flush wrote more than one follow-up: %+v", save)
			case err := <-flushed:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(hostilePersistenceTestDeadline):
				t.Fatal("Flush did not return after its single follow-up")
			}

			// 下一次 Flush 持久化剩余的最新快照，此后集合干净。
			next := make(chan error, 1)
			go func() { next <- p.Flush(context.Background()) }()
			latest := receiveHostileSaveBeforeFlushReturns(t, store, next)
			if latest.Revision != 3 || latest.Records[0].Position[0] != 30 {
				t.Fatalf("next Flush save=%+v, want revision 3 position 30", latest)
			}
			store.complete(nil)
			select {
			case err := <-next:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(hostilePersistenceTestDeadline):
				t.Fatal("next Flush did not return")
			}
			assertNoHostileSave(t, store)
		})
	}
}

func TestHostilePersistenceFlushFailureCanBeRetried(t *testing.T) {
	store := newControllableHostileStore()
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(p.Close)
	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 10)})
	firstDone := make(chan error, 1)
	go func() { firstDone <- p.Flush(context.Background()) }()
	first := receiveHostileSave(t, store)
	wantErr := errors.New("disk full")
	store.complete(wantErr)
	if err := <-firstDone; !errors.Is(err, wantErr) {
		t.Fatalf("first Flush error=%v", err)
	}
	secondDone := make(chan error, 1)
	go func() { secondDone <- p.Flush(context.Background()) }()
	retry := receiveHostileSaveBeforeFlushReturns(t, store, secondDone)
	if !reflect.DeepEqual(retry, first) {
		t.Fatalf("Flush retry=%+v, want %+v", retry, first)
	}
	store.complete(nil)
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	assertNoHostileSave(t, store)
}

func TestHostilePersistenceFlushCancellationKeepsWorkerAndRetry(t *testing.T) {
	store := newControllableHostileStore()
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(p.Close)
	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 10)})
	if err := p.Poll(10); err != nil {
		t.Fatal(err)
	}
	first := receiveHostileSave(t, store)

	// 调用方取消只中断等待：内部 worker 与在途保存原样存活。
	ctx, cancel := context.WithCancel(context.Background())
	canceled := make(chan error, 1)
	go func() { canceled <- p.Flush(ctx) }()
	waitForHostileFlushWait(t, p)
	cancel()
	select {
	case err := <-canceled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Flush error=%v, want context.Canceled", err)
		}
	case <-time.After(hostilePersistenceTestDeadline):
		t.Fatal("Flush ignored caller cancellation")
	}
	if err := p.ctx.Err(); err != nil {
		t.Fatalf("caller cancellation canceled internal worker: %v", err)
	}

	// 新的 Flush 先收回失败的 in-flight，再以冻结载荷重试。
	wantErr := errors.New("disk full after caller cancellation")
	store.complete(wantErr)
	collected := make(chan error, 1)
	go func() { collected <- p.Flush(context.Background()) }()
	select {
	case save := <-store.started:
		t.Fatalf("collecting failed in-flight dispatched another save: %+v", save)
	case err := <-collected:
		if !errors.Is(err, wantErr) {
			t.Fatalf("collecting Flush error=%v, want %v", err, wantErr)
		}
	case <-time.After(hostilePersistenceTestDeadline):
		t.Fatal("new Flush did not collect failed in-flight save")
	}

	replayed := make(chan error, 1)
	go func() { replayed <- p.Flush(context.Background()) }()
	retry := receiveHostileSaveBeforeFlushReturns(t, store, replayed)
	if !reflect.DeepEqual(retry, first) {
		t.Fatalf("retry=%+v, want frozen %+v", retry, first)
	}
	store.complete(nil)
	select {
	case err := <-replayed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(hostilePersistenceTestDeadline):
		t.Fatal("replayed Flush did not return")
	}
}

func TestHostilePersistenceSurvivesRealStoreRoundTrip(t *testing.T) {
	// 走真实 MemoryStore 的完整编码路径：观察 → Flush → 重新加载必须逐字段
	// 一致，不经任何假存档。
	store := storage.NewMemory(storage.Metadata{FormatVersion: 3, Seed: 42})
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(p.Close)
	p.Observe([]sim.HostileMob{
		hostileObserveFixture(5, 10),
		hostileObserveFixture(6, 20),
	})
	if err := p.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	loaded, err := store.LoadHostileMobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []storage.StoredHostileMob{
		hostileStorageRecord(hostileObserveFixture(5, 10)),
		hostileStorageRecord(hostileObserveFixture(6, 20)),
	}
	if loaded.Revision != 1 || !reflect.DeepEqual(loaded.Records, want) {
		t.Fatalf("round-trip=%+v，想要 revision 1 records %+v", loaded, want)
	}
}

func TestHostilePersistenceSaveFailurePreservesArchiveFile(t *testing.T) {
	// 走真实磁盘原子写路径的故障注入：世界目录只读使临时文件创建失败，
	// Flush 必须返回错误且正式文件保持上一次成功保存的内容（Sync/rename
	// 阶段的同类注入由 storage 包的 replace hooks 测试钉死，此处覆盖
	// worker 层的「失败即报错、旧文件不动」闭环）。
	root := t.TempDir()
	store, err := storage.OpenDisk(context.Background(), root, storage.OpenOptions{
		Create: storage.Metadata{FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld},
	})
	if err != nil {
		t.Fatalf("OpenDisk: %v", err)
	}
	p := NewHostiles(store, storage.StoredHostileMobs{}, hostilePersistenceTestOptions())
	t.Cleanup(func() {
		// 先恢复目录权限再关 store：TempDir 清理与 DiskStore.Close 都需要
		// 可写目录。
		if err := os.Chmod(root, 0o755); err != nil {
			t.Errorf("restore directory mode: %v", err)
		}
		p.Close()
		if err := store.Close(); err != nil {
			t.Errorf("store Close: %v", err)
		}
	})
	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 10)})
	if err := p.Flush(context.Background()); err != nil {
		t.Fatalf("first Flush: %v", err)
	}
	path := filepath.Join(root, "hostile_mobs.bin")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	p.Observe([]sim.HostileMob{hostileObserveFixture(5, 20)})
	if err := os.Chmod(root, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := p.Flush(context.Background()); err == nil {
		t.Fatal("保存失败被 Flush 吞掉，想要错误返回")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("失败的保存改写了旧正式文件")
	}

	// 恢复可写后重试同一屏障：最新快照落盘，rev 推进。
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := p.Flush(context.Background()); err != nil {
		t.Fatalf("retry Flush: %v", err)
	}
	loaded, err := store.LoadHostileMobs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Revision != 2 || loaded.Records[0].Position[0] != 20 {
		t.Fatalf("retry save=%+v，想要 revision 2 且位置 20", loaded.Records)
	}
}

// hostileObserveFixture 返回一条位置可变的合法权威夜行者快照（含目标、
// 冷却与远离累计，供脏判定与逐字段比对使用）。
func hostileObserveFixture(id uint64, x float32) sim.HostileMob {
	return sim.HostileMob{
		ID:        id,
		Dimension: core.Overworld,
		State: physics.State{
			Position: mgl32.Vec3{x, 1, 0.5},
			OnGround: true,
		},
		Yaw:             0.5,
		Health:          core.MaxHealth,
		AttackCooldown:  3,
		BurnCooldown:    20,
		HasTarget:       true,
		PlayerID:        hostileTestPlayerID(),
		NextRepathTicks: 30,
		DistantTicks:    7,
	}
}

func hostileTestPlayerID() core.PlayerID {
	return core.PlayerID{0, 0, 0, 0, 0, 0, 0x40, 0, 0x80, 0, 0, 0, 0, 0, 0, 7}
}

type controllableHostileStore struct {
	mu      sync.Mutex
	started chan storage.HostileMobsSave
	results chan error
	mutate  bool
	onSave  func()
}

const hostilePersistenceTestDeadline = time.Second

func newControllableHostileStore() *controllableHostileStore {
	return &controllableHostileStore{
		started: make(chan storage.HostileMobsSave, 4),
		results: make(chan error),
	}
}

func (store *controllableHostileStore) LoadHostileMobs(context.Context) (storage.StoredHostileMobs, error) {
	return storage.StoredHostileMobs{}, storage.ErrHostileMobsNotFound
}

func (store *controllableHostileStore) SaveHostileMobs(ctx context.Context, save storage.HostileMobsSave) error {
	store.mu.Lock()
	mutate, onSave := store.mutate, store.onSave
	store.mutate = false
	store.mu.Unlock()
	if mutate && len(save.Records) != 0 {
		save.Records[0].Position[0] = 999
	}
	snapshot := cloneHostileSaveForTest(save)
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

func (store *controllableHostileStore) complete(err error) { store.results <- err }

func (store *controllableHostileStore) mutateNextSave() {
	store.mu.Lock()
	store.mutate = true
	store.mu.Unlock()
}

func (store *controllableHostileStore) setOnSave(onSave func()) {
	store.mu.Lock()
	store.onSave = onSave
	store.mu.Unlock()
}

func cloneHostileSaveForTest(save storage.HostileMobsSave) storage.HostileMobsSave {
	snapshot := save
	snapshot.Records = append([]storage.StoredHostileMob(nil), save.Records...)
	return snapshot
}

func hostilePersistenceTestOptions() Options {
	return Options{
		AutosaveTicks:  10,
		RetryBaseTicks: 2,
		RetryMaxTicks:  8,
	}
}

func receiveHostileSave(t *testing.T, store *controllableHostileStore) storage.HostileMobsSave {
	t.Helper()
	select {
	case save := <-store.started:
		return save
	case <-time.After(waitDeadline):
		t.Fatal("SaveHostileMobs was not started")
		return storage.HostileMobsSave{}
	}
}

func receiveHostileSaveBeforeFlushReturns(
	t *testing.T,
	store *controllableHostileStore,
	flushed <-chan error,
) storage.HostileMobsSave {
	t.Helper()
	select {
	case save := <-store.started:
		return save
	case err := <-flushed:
		t.Fatalf("Flush returned before required save: %v", err)
		return storage.HostileMobsSave{}
	case <-time.After(hostilePersistenceTestDeadline):
		t.Fatal("SaveHostileMobs was not started before Flush deadline")
		return storage.HostileMobsSave{}
	}
}

func waitForHostileFlushWait(t *testing.T, p *Hostiles) {
	t.Helper()
	deadline := time.Now().Add(hostilePersistenceTestDeadline)
	for {
		if p.completionMu.TryLock() {
			p.completionMu.Unlock()
		} else {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Flush did not start waiting for in-flight save")
		}
		time.Sleep(time.Millisecond)
	}
}

func assertNoHostileSave(t *testing.T, store *controllableHostileStore) {
	t.Helper()
	select {
	case save := <-store.started:
		t.Fatalf("unexpected SaveHostileMobs(%+v)", save)
	case <-time.After(50 * time.Millisecond):
	}
}

func pollHostilePersistenceUntil(t *testing.T, p *Hostiles, tick uint64, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for !done() {
		if err := p.Poll(tick); err != nil {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("hostile persistence did not reach expected state")
		}
		time.Sleep(time.Millisecond)
	}
}

func pollHostilePersistenceError(t *testing.T, p *Hostiles, tick uint64) error {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for {
		if err := p.Poll(tick); err != nil {
			return err
		}
		if time.Now().After(deadline) {
			t.Fatal("hostile persistence did not surface SaveHostileMobs error")
		}
		time.Sleep(time.Millisecond)
	}
}

var _ storage.HostileMobStore = (*controllableHostileStore)(nil)
