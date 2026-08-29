package core_test

import (
	"testing"

	"github.com/channing771/mornlea/internal/core"
)

func TestCombatTargetKindUsesStableValues(t *testing.T) {
	if core.CombatTargetPlayer != 1 || core.CombatTargetHostile != 2 {
		t.Fatalf("战斗目标类别 = player %d / hostile %d，想要 1 / 2",
			core.CombatTargetPlayer, core.CombatTargetHostile)
	}
	for _, kind := range []core.CombatTargetKind{core.CombatTargetPlayer, core.CombatTargetHostile} {
		if !kind.Valid() {
			t.Fatalf("合法战斗目标类别 %d 被拒绝", kind)
		}
	}
	for _, kind := range []core.CombatTargetKind{0, 3} {
		if kind.Valid() {
			t.Fatalf("非法战斗目标类别 %d 被接受", kind)
		}
	}
}
