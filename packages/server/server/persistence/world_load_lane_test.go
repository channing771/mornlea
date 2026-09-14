package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

type stubLoadStore struct{ storage.Store }

func (stubLoadStore) LoadChunk(_ context.Context, key core.ChunkKey) (storage.StoredChunk, error) {
	return storage.StoredChunk{}, storage.ErrChunkNotFound
}

func TestSubmitLoadMissingKeyDrainsNotFound(t *testing.T) {
	t.Parallel()
	store := stubLoadStore{}
	world := NewWorld(store, nil, Options{SaveWorkers: 1, LoadWorkers: 1})
	defer world.Close()
	var zero core.ChunkKey
	if !world.SubmitLoad(LoadRequest{Keys: []core.ChunkKey{zero}}) {
		t.Fatalf("SubmitLoad = false, want true")
	}
	deadline := 1000
	for world.LoadPending() > 0 && deadline > 0 {
		// 轮询等待 worker 推进：无休眠的空转不等 worker，只能原地消耗计数。
		time.Sleep(time.Millisecond)
		deadline--
	}
	results := world.DrainLoad()
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Err == nil {
		t.Fatalf("results[0].Err = nil, want non-nil")
	}
}

// TestCloseWithoutSubmitIsClean 钉住懒启动路径：未提交即关闭必须
// 立刻返回，不挂起，不残留计数与结果。
func TestCloseWithoutSubmitIsClean(t *testing.T) {
	t.Parallel()
	world := NewWorld(stubLoadStore{}, nil, Options{SaveWorkers: 1, LoadWorkers: 1})
	world.Close()
	if got := world.LoadPending(); got != 0 {
		t.Fatalf("LoadPending() = %d, want 0", got)
	}
	if results := world.DrainLoad(); len(results) != 0 {
		t.Fatalf("len(DrainLoad()) = %d, want 0", len(results))
	}
}

// TestSubmitAfterCloseReturnsFalse 钉住关闭后不再接受提交：
// 从未提交的世界关闭后，`SubmitLoad` 返回 false 且计数为零。
func TestSubmitAfterCloseReturnsFalse(t *testing.T) {
	t.Parallel()
	world := NewWorld(stubLoadStore{}, nil, Options{SaveWorkers: 1, LoadWorkers: 1})
	world.Close()
	var zero core.ChunkKey
	if world.SubmitLoad(LoadRequest{Keys: []core.ChunkKey{zero}}) {
		t.Fatalf("SubmitLoad after Close = true, want false")
	}
	if got := world.LoadPending(); got != 0 {
		t.Fatalf("LoadPending() = %d, want 0", got)
	}
}
