package persistence

import (
	"errors"
	"testing"
	"time"

	"github.com/channing771/mornlea/internal/storage"
)

func waitMetadataSaves(t *testing.T, store *persistenceTestStore, want int) []storage.Metadata {
	t.Helper()
	deadline := time.Now().Add(waitDeadline)
	for {
		store.mu.Lock()
		saves := append([]storage.Metadata(nil), store.metadataSaves...)
		store.mu.Unlock()
		if len(saves) >= want {
			return saves
		}
		if time.Now().After(deadline) {
			t.Fatalf("metadata 提交次数 = %d，想要至少 %d", len(saves), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func metadataSaveCount(store *persistenceTestStore) int {
	store.mu.Lock()
	defer store.mu.Unlock()
	return len(store.metadataSaves)
}

func stepToAutosaveBoundary(t *testing.T, running *persistenceTestWorld) uint64 {
	t.Helper()
	for range int(running.config.AutosaveTicks) + 1 {
		result := running.StepForTest()
		if result.Tick%running.config.AutosaveTicks == 0 {
			return result.WorldTimeTicks
		}
	}
	t.Fatal("没有到达自动保存边界")
	return 0
}

func TestMetadataAutosaveCommitsLatestWorldTimeAtBoundary(t *testing.T) {
	store := newPersistenceTestStore()
	running := newPersistenceServer(t, store)
	running.config.AutosaveTicks = 4

	running.StepForTest()
	running.StepForTest()
	if got := metadataSaveCount(store); got != 0 {
		t.Fatalf("边界前 metadata 提交次数 = %d，想要 0", got)
	}

	wantTime := stepToAutosaveBoundary(t, running)
	saves := waitMetadataSaves(t, store, 1)
	if saves[0].WorldTimeTicks != wantTime {
		t.Fatalf("提交世界时间 = %d，想要 %d", saves[0].WorldTimeTicks, wantTime)
	}
	if saves[0].Seed != store.metadata.Seed {
		t.Fatalf("提交种子 = %d，想要 %d", saves[0].Seed, store.metadata.Seed)
	}
}

func TestMetadataAutosaveKeepsAtMostOneInFlightAndMergesLatest(t *testing.T) {
	store := newPersistenceTestStore()
	release := make(chan struct{})
	entered := make(chan struct{}, 8)
	store.metadataRespond = func(call int, _ storage.Metadata) error {
		entered <- struct{}{}
		if call == 0 {
			<-release
		}
		return nil
	}
	running := newPersistenceServer(t, store)
	running.config.AutosaveTicks = 4

	first := stepToAutosaveBoundary(t, running)
	select {
	case <-entered:
	case <-time.After(waitDeadline):
		t.Fatal("首个 metadata 保存没有开始")
	}

	var latest uint64
	for range int(running.config.AutosaveTicks) * 3 {
		latest = running.StepForTest().WorldTimeTicks
	}
	if got := metadataSaveCount(store); got != 1 {
		t.Fatalf("in-flight 期间 metadata 提交次数 = %d，想要 1", got)
	}

	close(release)
	deadline := time.Now().Add(waitDeadline)
	for metadataSaveCount(store) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("合并后的 metadata 没有被提交，次数 = %d", metadataSaveCount(store))
		}
		running.StepForTest()
	}
	saves := waitMetadataSaves(t, store, 2)
	if saves[0].WorldTimeTicks != first {
		t.Fatalf("首次提交世界时间 = %d，想要 %d", saves[0].WorldTimeTicks, first)
	}
	if saves[1].WorldTimeTicks < latest {
		t.Fatalf("合并后提交世界时间 = %d，想要至少 %d", saves[1].WorldTimeTicks, latest)
	}
	if len(saves) > 2 {
		t.Fatalf("跨过 3 个边界产生了 %d 次提交，想要合并为 2 次", len(saves))
	}
}

func TestMetadataAutosaveFailureRetriesWithBoundedBackoff(t *testing.T) {
	store := newPersistenceTestStore()
	injected := errors.New("injected metadata failure")
	store.metadataRespond = func(call int, _ storage.Metadata) error {
		if call == 0 {
			return injected
		}
		return nil
	}
	running := newPersistenceServer(t, store)
	running.config.AutosaveTicks = 4

	stepToAutosaveBoundary(t, running)
	waitMetadataSaves(t, store, 1)

	deadline := time.Now().Add(waitDeadline)
	for {
		status := running.PersistenceStatus()
		if status.MetadataLastError != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("metadata 保存失败没有被记录")
		}
		running.StepForTest()
	}

	for range int(running.config.RetryBaseTicks) * 2 {
		running.StepForTest()
	}
	saves := waitMetadataSaves(t, store, 2)
	if len(saves) < 2 {
		t.Fatalf("重试后 metadata 提交次数 = %d，想要至少 2", len(saves))
	}
	deadline = time.Now().Add(waitDeadline)
	for {
		if running.PersistenceStatus().MetadataLastError == "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("成功重试后仍保留失败状态")
		}
		running.StepForTest()
	}
}

func TestMetadataAutosaveDoesNotBlockStepWhenQueueIsFull(t *testing.T) {
	store := newPersistenceTestStore()
	release := make(chan struct{})
	defer close(release)
	store.metadataRespond = func(int, storage.Metadata) error {
		<-release
		return nil
	}
	running := newPersistenceServer(t, store)
	running.config.AutosaveTicks = 2

	for len(running.world.saveJobs) < cap(running.world.saveJobs) {
		running.world.saveJobs <- saveJob{Kind: saveKindMetadata}
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 8 {
			running.StepForTest()
		}
	}()
	select {
	case <-done:
	case <-time.After(waitDeadline):
		t.Fatal("save 队列满时 Step 被阻塞")
	}
	if !running.PersistenceStatus().MetadataPending {
		t.Fatal("队列满后没有保留待提交的 metadata")
	}
}
