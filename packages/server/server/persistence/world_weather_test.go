package persistence

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// TestMetadataAutosaveCommitsLatestWeatherAtBoundary 覆盖自动保存的天气一半：
// 保存边界派发的 metadata 必须携带调度时刻的最新权威天气（种类与剩余时长），
// 与世界时间同一快照语义；I/O 仍在 worker 内，tick 只做有界非阻塞调度。
func TestMetadataAutosaveCommitsLatestWeatherAtBoundary(t *testing.T) {
	store := newPersistenceTestStore()
	running := newPersistenceServer(t, store)
	running.config.AutosaveTicks = 4

	running.StepForTest()
	running.StepForTest()
	if got := metadataSaveCount(store); got != 0 {
		t.Fatalf("边界前 metadata 提交次数 = %d，想要 0", got)
	}

	running.engine.SetWeatherForTest(core.WeatherRain, 5000)
	wantTime := stepToAutosaveBoundary(t, running)
	wantKind := running.engine.WeatherKind()
	wantRemaining := running.engine.WeatherTicksRemaining()
	saves := waitMetadataSaves(t, store, 1)
	if saves[0].WorldTimeTicks != wantTime {
		t.Fatalf("提交世界时间 = %d，想要 %d", saves[0].WorldTimeTicks, wantTime)
	}
	if saves[0].WeatherKind != wantKind || wantKind != core.WeatherRain {
		t.Fatalf("提交天气 = %d，想要边界最新值 %d", saves[0].WeatherKind, wantKind)
	}
	if saves[0].WeatherTicksRemaining != wantRemaining {
		t.Fatalf("提交剩余时长 = %d，想要边界最新值 %d", saves[0].WeatherTicksRemaining, wantRemaining)
	}
	if saves[0].Seed != store.metadata.Seed {
		t.Fatalf("提交种子 = %d，想要 %d", saves[0].Seed, store.metadata.Seed)
	}
}
