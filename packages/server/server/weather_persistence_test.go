package server

import (
	"context"
	"testing"

	"github.com/channing771/mornlea/packages/server/storage"
	"github.com/channing771/mornlea/packages/shared/core"
)

// TestNewWorldRestoresWeatherFromMetadata 覆盖宿主装配的天气一半：开服时引擎的
// 天气种类与剩余时长必须从世界 metadata 恢复，而不是恒从晴天起步。
func TestNewWorldRestoresWeatherFromMetadata(t *testing.T) {
	store := storage.NewMemory(storage.Metadata{
		FormatVersion: 4, Seed: 42, SpawnDimension: core.Overworld,
		WeatherKind: core.WeatherRain, WeatherTicksRemaining: 5000,
	})
	config := DefaultConfig(42)
	config.ViewRadius = 0
	config.Workers = 1
	config.SaveWorkers = 1
	running := NewWorld(config, playerTestGenerator{}, store)
	if got := running.engine.WeatherKind(); got != core.WeatherRain {
		t.Fatalf("开服恢复的天气 = %d，想要雨 %d", got, core.WeatherRain)
	}
	if got := running.engine.WeatherTicksRemaining(); got != 5000 {
		t.Fatalf("开服恢复的剩余时长 = %d，想要 5000", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := running.Shutdown(ctx); err != nil {
		t.Fatalf("关服: %v", err)
	}
}

// TestShutdownPersistsWeather 覆盖关服屏障的天气一半与 Scenario「重启延续天气」：
// 冻结后的最终天气必须随关服屏障落盘，重开世界后首份有效权威状态的天气与
// 关服前一致。
func TestShutdownPersistsWeather(t *testing.T) {
	root := t.TempDir()
	const seed int64 = 4242
	running, _ := openRestartWorld(t, root, seed)
	// 模拟推进后的天气状态：经测试专用写入口置值（生产写者是权威 tick 尾部的
	// 天气推进，这里只关心持久化接线）。
	running.engine.SetWeatherForTest(core.WeatherThunder, 7777)
	ctx, cancel := context.WithTimeout(context.Background(), waitDeadline)
	defer cancel()
	if err := running.Shutdown(ctx); err != nil {
		t.Fatalf("关服: %v", err)
	}
	// 关服屏障会再走一个最终 tick：落盘的必须是冻结后的最终值，
	// 而不是设置时的旧值（7777 经最终 tick 递减为 7776）。
	wantKind := running.engine.WeatherKind()
	wantRemaining := running.engine.WeatherTicksRemaining()
	if wantKind != core.WeatherThunder || wantRemaining != 7776 {
		t.Fatalf("关服后引擎天气 = (%d, %d)，想要 (雷暴, 7776)", wantKind, wantRemaining)
	}

	reopened, reopenedStore := openRestartWorld(t, root, seed)
	t.Cleanup(func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), waitDeadline)
		defer shutdownCancel()
		if err := reopened.Shutdown(shutdownCtx); err != nil {
			t.Errorf("清理关服: %v", err)
		}
	})
	if got := reopenedStore.Metadata().WeatherKind; got != wantKind {
		t.Fatalf("关服落盘的天气 = %d，想要冻结后的 %d", got, wantKind)
	}
	if got := reopenedStore.Metadata().WeatherTicksRemaining; got != wantRemaining {
		t.Fatalf("关服落盘的剩余时长 = %d，想要冻结后的 %d", got, wantRemaining)
	}
	if got := reopened.engine.WeatherKind(); got != wantKind {
		t.Fatalf("重开恢复的天气 = %d，想要 %d", got, wantKind)
	}
	// 剩余时长远大于 1，首个 tick 只递减、种类不变：首份有效权威状态延续关服前。
	if got := reopened.StepForTest().WeatherKind; got != wantKind {
		t.Fatalf("重开后首个 tick 天气 = %d，想要 %d", got, wantKind)
	}
}
