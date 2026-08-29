package server

import (
	"context"
	"errors"
	"testing"

	"github.com/channing771/mornlea/internal/storage"
)

func setMetadataRespond(store *shutdownTestStore, respond func(int, storage.Metadata) error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	store.metadataRespond = respond
}

func shutdownMetadataSaves(store *shutdownTestStore) []storage.Metadata {
	store.mu.Lock()
	defer store.mu.Unlock()
	return append([]storage.Metadata(nil), store.metadataSaves...)
}

func TestShutdownFlushesFinalWorldTimeBeforeSync(t *testing.T) {
	store := newShutdownTestStore()
	running, _ := newShutdownTestServer(t, store)
	for range 5 {
		running.StepForTest()
	}

	if err := shutdownWithDeadline(running, waitDeadline); err != nil {
		t.Fatalf("Shutdown 错误 = %v", err)
	}
	want := running.engine.WorldTime()
	saves := shutdownMetadataSaves(store)
	if len(saves) == 0 {
		t.Fatal("关服没有保存世界 metadata")
	}
	if got := saves[len(saves)-1].WorldTimeTicks; got != want {
		t.Fatalf("关服保存的世界时间 = %d，想要冻结后的 %d", got, want)
	}
	if got := store.eventLog(); len(got) < 2 || got[len(got)-2] != "sync" || got[len(got)-1] != "close" {
		t.Fatalf("关服事件顺序 = %v，想要以 sync、close 结束", got)
	}
}

func TestShutdownMetadataFailureIsRetryableAndKeepsOwnership(t *testing.T) {
	store := newShutdownTestStore()
	injected := errors.New("injected shutdown metadata failure")
	setMetadataRespond(store, func(int, storage.Metadata) error { return injected })
	running, _ := newShutdownTestServer(t, store)
	running.StepForTest()

	if err := shutdownWithDeadline(running, waitDeadline); !errors.Is(err, injected) {
		t.Fatalf("首次 Shutdown 错误 = %v，想要注入的 metadata 失败", err)
	}
	if syncCalls, closeCalls := store.lifecycleCalls(); syncCalls != 0 || closeCalls != 0 {
		t.Fatalf("metadata 失败后 Sync=%d Close=%d，想要 0,0", syncCalls, closeCalls)
	}
	if !store.worldOwned() {
		t.Fatal("metadata 失败释放了 Store 所有权")
	}

	setMetadataRespond(store, nil)
	if err := shutdownWithDeadline(running, waitDeadline); err != nil {
		t.Fatalf("重试 Shutdown 错误 = %v", err)
	}
	want := running.engine.WorldTime()
	saves := shutdownMetadataSaves(store)
	if got := saves[len(saves)-1].WorldTimeTicks; got != want {
		t.Fatalf("重试保存的世界时间 = %d，想要 %d", got, want)
	}
	if syncCalls, closeCalls := store.lifecycleCalls(); syncCalls != 1 || closeCalls != 1 {
		t.Fatalf("重试后 Sync=%d Close=%d，想要 1,1", syncCalls, closeCalls)
	}
}

func TestShutdownMetadataContextTimeoutDefersToRetry(t *testing.T) {
	store := newShutdownTestStore()
	running, _ := newShutdownTestServer(t, store)
	running.StepForTest()

	ctx, cancel := context.WithCancel(context.Background())
	setMetadataRespond(store, func(int, storage.Metadata) error {
		cancel()
		return context.Canceled
	})
	if err := running.Shutdown(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown 错误 = %v，想要 context.Canceled", err)
	}
	if syncCalls, closeCalls := store.lifecycleCalls(); syncCalls != 0 || closeCalls != 0 {
		t.Fatalf("取消后 Sync=%d Close=%d，想要 0,0", syncCalls, closeCalls)
	}
	if !store.worldOwned() {
		t.Fatal("取消的关服释放了 Store 所有权")
	}

	setMetadataRespond(store, nil)
	recoverShutdownAfterExpectedFailure(t, running, context.Canceled)
	if store.worldOwned() {
		t.Fatal("成功关服仍保留 Store 所有权")
	}
}
