package sim

import (
	"testing"

	"github.com/channing771/mornlea/internal/sim/tuning"
)

// TestEngineRefreshesSnapshotAtTickStart 证明快照在 tick 入口刷新，
// 且同一 tick 内不再变化。
func TestEngineRefreshesSnapshotAtTickStart(t *testing.T) {
	t.Cleanup(func() { tuning.SetTunables(tuning.DefaultTunables()) })

	engine := NewEngine(0, 0, 0)

	changed := tuning.DefaultTunables()
	changed.InteractionReach = 3
	tuning.SetTunables(changed)

	engine.Step()
	if engine.tunables.InteractionReach != 3 {
		t.Fatalf("tick 后引擎快照 InteractionReach = %v，want 3",
			engine.tunables.InteractionReach)
	}

	// tick 之间修改，在下一次 Step 之前引擎快照不应改变。
	again := tuning.DefaultTunables()
	again.InteractionReach = 5
	tuning.SetTunables(again)
	if engine.tunables.InteractionReach != 3 {
		t.Fatal("引擎快照必须只在 tick 入口刷新")
	}
	engine.Step()
	if engine.tunables.InteractionReach != 5 {
		t.Fatalf("下一次 tick 后应刷新为 5，实际 %v", engine.tunables.InteractionReach)
	}
}
