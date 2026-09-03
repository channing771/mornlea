package companion

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/shared/core"
)

const testNamespaceUUID = "4d5e6f70-8192-4aa3-8b4f-3a4b5c6d7e8f"

func TestSnapshotRegistryCapacityExpiryAndLifecycle(t *testing.T) {
	clock := newSnapshotFakeClock(time.Unix(1_800_000_000, 0))
	registry := newSnapshotRegistry(clock, snapshotTestEntropy())
	snapshot := testSnapshot()
	registrations := make([]SnapshotRegistration, 0, SnapshotRegistryCapacity)
	for generation := uint64(1); generation <= SnapshotRegistryCapacity; generation++ {
		registration, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, generation, snapshot, clock.Now().Add(time.Second))
		if err != nil {
			t.Fatalf("Register(%d): %v", generation, err)
		}
		registrations = append(registrations, registration)
	}
	started := time.Now()
	if _, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, 9, snapshot, clock.Now().Add(time.Second)); !errors.Is(err, ErrSnapshotRegistryFull) {
		t.Fatalf("第五槽错误=%v，want ErrSnapshotRegistryFull", err)
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("第五槽等待了 %v", elapsed)
	}

	first, err := registry.Lookup(registrations[0].Capability)
	if err != nil {
		t.Fatalf("Lookup(first): %v", err)
	}
	if first.SnapshotID != registrations[0].SnapshotID || first.Generation != 1 ||
		first.NamespaceID != testNamespaceUUID || first.Digest != registrations[0].Digest {
		t.Fatalf("lookup identity=%+v", first)
	}
	if !registry.Complete(registrations[0].SnapshotID) {
		t.Fatal("Complete(first)=false")
	}
	wantSnapshotCanceled(t, first.Done())
	if _, err := registry.Lookup(registrations[0].Capability); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("complete 后 Lookup=%v", err)
	}
	if _, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, 10, snapshot, clock.Now().Add(time.Second)); err != nil {
		t.Fatalf("complete 未释放容量: %v", err)
	}

	clock.Advance(6 * time.Second)
	for _, registration := range registrations[1:] {
		if _, err := registry.Lookup(registration.Capability); !errors.Is(err, ErrSnapshotUnavailable) {
			t.Fatalf("TTL 后 Lookup(%s)=%v", registration.SnapshotID, err)
		}
	}
	for generation := uint64(20); generation < 20+SnapshotRegistryCapacity; generation++ {
		if _, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, generation, snapshot, clock.Now().Add(time.Second)); err != nil {
			t.Fatalf("无人 lookup 的 TTL 未释放第 %d 槽: %v", generation, err)
		}
	}

	registry.Close()
	registry.Close()
	if _, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, 99, snapshot, clock.Now().Add(time.Second)); !errors.Is(err, ErrSnapshotRegistryClosed) {
		t.Fatalf("Close 后 Register=%v", err)
	}
	if _, err := registry.Lookup("missing"); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("Close 后 Lookup=%v", err)
	}
}

func TestSnapshotRegistryDeepCopyCapabilityAndCancel(t *testing.T) {
	clock := newSnapshotFakeClock(time.Unix(1_800_000_100, 0))
	entropy := snapshotTestEntropy()
	registry := newSnapshotRegistry(clock, entropy)
	t.Cleanup(registry.Close)

	snapshot := testSnapshot()
	registration, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, 7, snapshot, clock.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if registration.SnapshotID == "" || registration.Capability == "" || len(registration.Digest) != 64 ||
		registration.SnapshotID == registration.Capability {
		t.Fatalf("registration identity=%+v", registration)
	}
	if _, err := registry.Lookup(registration.SnapshotID); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("snapshot ID 不得充当 capability: %v", err)
	}
	if _, err := registry.Lookup(registration.Capability + "x"); !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("错误 capability 泄露 record: %v", err)
	}

	snapshot.Command = "已被调用方修改"
	snapshot.ExposedBlocks[0].Block = core.BedrockID
	snapshot.Heights[0].Height = core.MinY - 1
	snapshot.OnlinePlayers[0].Position[0] = 999
	snapshot.ChunkRevisions[0].Revision = 999
	snapshot.Terrain.SetBlock(core.BlockPos{X: 6, Y: 64, Z: 0}, core.DirtID)
	first, err := registry.Lookup(registration.Capability)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if first.Snapshot.Command != "去那棵橡树旁边" || first.Snapshot.ExposedBlocks[0].Block != core.GrassID ||
		first.Snapshot.OnlinePlayers[0].Position[0] == 999 || first.Snapshot.ChunkRevisions[0].Revision == 999 {
		t.Fatal("registry 没有深拷贝注册输入")
	}
	block, _, ok := first.Snapshot.Terrain.Lookup(core.BlockPos{X: 6, Y: 64, Z: 0})
	if !ok || block != core.StoneID {
		t.Fatalf("registry terrain=%d/%v，want stone", block, ok)
	}

	first.Snapshot.ExposedBlocks[0].Block = core.BedrockID
	first.Snapshot.OnlinePlayers[0].Position[0] = 777
	second, err := registry.Lookup(registration.Capability)
	if err != nil || second.Snapshot.ExposedBlocks[0].Block != core.GrassID || second.Snapshot.OnlinePlayers[0].Position[0] == 777 {
		t.Fatalf("Lookup view 不是独立深拷贝: err=%v", err)
	}
	if !registry.Cancel(registration.SnapshotID) {
		t.Fatal("Cancel=false")
	}
	wantSnapshotCanceled(t, first.Done())
	if first.Checkpoint() == nil {
		t.Fatal("取消后 Checkpoint=nil")
	}
}

func TestSnapshotRegistryRejectsInvalidDeadlineAndIdentity(t *testing.T) {
	clock := newSnapshotFakeClock(time.Unix(1_800_000_200, 0))
	registry := newSnapshotRegistry(clock, snapshotTestEntropy())
	t.Cleanup(registry.Close)
	snapshot := testSnapshot()

	for name, testCase := range map[string]struct {
		namespace  string
		id         ID
		generation uint64
		deadline   time.Time
	}{
		"past":            {testNamespaceUUID, snapshot.Companion.ID, 1, clock.Now().Add(-time.Nanosecond)},
		"equal":           {testNamespaceUUID, snapshot.Companion.ID, 1, clock.Now()},
		"namespace":       {"not-a-uuid", snapshot.Companion.ID, 1, clock.Now().Add(time.Second)},
		"companion":       {testNamespaceUUID, ID{}, 1, clock.Now().Add(time.Second)},
		"generation":      {testNamespaceUUID, snapshot.Companion.ID, 0, clock.Now().Add(time.Second)},
		"expiry overflow": {testNamespaceUUID, snapshot.Companion.ID, 1, time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := registry.Register(testCase.namespace, testCase.id, testCase.generation, snapshot, testCase.deadline); !errors.Is(err, ErrSnapshotInvalid) {
				t.Fatalf("Register=%v，want ErrSnapshotInvalid", err)
			}
		})
	}
}

func TestSnapshotRegistryRejectsDeadlineThatExpiresWhileFreezing(t *testing.T) {
	clock := newSnapshotFakeClock(time.Unix(1_800_000_225, 0))
	entropy := &snapshotAdvancingEntropy{
		clock:  clock,
		reader: snapshotTestEntropy(),
		delta:  snapshotExpiryGrace + time.Second,
	}
	registry := newSnapshotRegistry(clock, entropy)
	t.Cleanup(registry.Close)
	snapshot := testSnapshot()

	if _, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, 1, snapshot, clock.Now().Add(time.Second)); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatalf("冻结期间已过期的 Register=%v，want ErrSnapshotInvalid", err)
	}
	if _, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, 2, snapshot, clock.Now().Add(time.Second)); err != nil {
		t.Fatalf("失败注册占用了容量: %v", err)
	}
}

func TestSnapshotRegistryRejectsUnrepresentableTimerDeadline(t *testing.T) {
	clock := newSnapshotFakeClock(time.Unix(1_800_000_230, 0))
	registry := newSnapshotRegistry(clock, snapshotTestEntropy())
	t.Cleanup(registry.Close)
	snapshot := testSnapshot()
	deadline := time.Date(3000, time.January, 1, 0, 0, 0, 0, time.UTC)

	if _, err := registry.Register(testNamespaceUUID, snapshot.Companion.ID, 1, snapshot, deadline); !errors.Is(err, ErrSnapshotInvalid) {
		t.Fatalf("无法表示的 timer deadline Register=%v，want ErrSnapshotInvalid", err)
	}
}

func wantSnapshotCanceled(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("registry cancellation 未发出")
	}
}

func snapshotTestEntropy() *bytes.Reader {
	data := make([]byte, 4096)
	for index := range data {
		data[index] = byte(index)
	}
	return bytes.NewReader(data)
}

type snapshotAdvancingEntropy struct {
	clock  *snapshotFakeClock
	reader io.Reader
	delta  time.Duration
	once   sync.Once
}

func (r *snapshotAdvancingEntropy) Read(data []byte) (int, error) {
	r.once.Do(func() { r.clock.Advance(r.delta) })
	return r.reader.Read(data)
}

type snapshotFakeClock struct {
	mu     sync.Mutex
	now    time.Time
	nextID uint64
	timers map[uint64]*snapshotFakeTimer
}

type snapshotFakeTimer struct {
	clock   *snapshotFakeClock
	id      uint64
	due     time.Time
	fn      func()
	stopped bool
}

func newSnapshotFakeClock(now time.Time) *snapshotFakeClock {
	return &snapshotFakeClock{now: now, timers: make(map[uint64]*snapshotFakeTimer)}
}

func (c *snapshotFakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *snapshotFakeClock) AfterFunc(delay time.Duration, fn func()) snapshotTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	timer := &snapshotFakeTimer{clock: c, id: c.nextID, due: c.now.Add(delay), fn: fn}
	c.timers[timer.id] = timer
	return timer
}

func (c *snapshotFakeClock) Advance(delta time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(delta)
	var callbacks []func()
	for id, timer := range c.timers {
		if !timer.stopped && !timer.due.After(c.now) {
			timer.stopped = true
			delete(c.timers, id)
			callbacks = append(callbacks, timer.fn)
		}
	}
	c.mu.Unlock()
	for _, callback := range callbacks {
		callback()
	}
}

func (t *snapshotFakeTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if t.stopped {
		return false
	}
	t.stopped = true
	delete(t.clock.timers, t.id)
	return true
}
