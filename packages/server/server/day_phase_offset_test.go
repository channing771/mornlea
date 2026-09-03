package server

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestNewWorldRestoresDayPhaseOffsetFromMetadata 覆盖宿主装配的一半：开服时
// 引擎的显示相位偏移必须从世界 metadata 恢复，而不是恒从 0 起步。
func TestNewWorldRestoresDayPhaseOffsetFromMetadata(t *testing.T) {
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 3, Seed: 42, SpawnDimension: core.Overworld,
		DayPhaseOffset: 12399,
	})
	config := DefaultConfig(42)
	config.ViewRadius = 0
	config.Workers = 1
	config.SaveWorkers = 1
	running := NewWorld(config, playerTestGenerator{}, store)
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := running.Shutdown(ctx); err != nil {
		t.Fatalf("关服: %v", err)
	}
	if got := running.engine.DayPhaseOffset(); got != 12399 {
		t.Fatalf("开服恢复的偏移 = %d，想要 12399", got)
	}
}

// TestShutdownPersistsDayPhaseOffset 覆盖关服屏障的一半：跳夜设置的非零偏移
// 必须随关服屏障落盘，重开世界时从保存值继续（metadata v3 偏移持久化）。
func TestShutdownPersistsDayPhaseOffset(t *testing.T) {
	root := t.TempDir()
	const seed int64 = 4242
	running, _ := openRestartWorld(t, root, seed)
	// 模拟跳夜结算后的偏移状态：经测试专用写入口置值（生产写者是
	// settleSleepThroughNight，这里只关心持久化接线）。
	running.engine.SetDayPhaseOffsetForTest(23999)
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := running.Shutdown(ctx); err != nil {
		t.Fatalf("关服: %v", err)
	}

	reopened, reopenedStore := openRestartWorld(t, root, seed)
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), waitDeadline)
		defer shutdownCancel()
		if err := reopened.Shutdown(shutdownCtx); err != nil {
			t.Errorf("清理关服: %v", err)
		}
	})
	if got := reopenedStore.Metadata().DayPhaseOffset; got != 23999 {
		t.Fatalf("关服落盘的偏移 = %d，想要 23999", got)
	}
	if got := reopened.engine.DayPhaseOffset(); got != 23999 {
		t.Fatalf("重开恢复的偏移 = %d，想要 23999", got)
	}
}
