package core_test

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

func TestCombatTargetKindUsesStableValues(t *testing.T) {
	if core.CombatTargetPlayer != 1 || core.CombatTargetHostile != 2 || core.CombatTargetPassive != 3 {
		t.Fatalf("战斗目标类别 = player %d / hostile %d / passive %d，想要 1 / 2 / 3",
			core.CombatTargetPlayer, core.CombatTargetHostile, core.CombatTargetPassive)
	}
	for _, kind := range []core.CombatTargetKind{core.CombatTargetPlayer, core.CombatTargetHostile, core.CombatTargetPassive} {
		if !kind.Valid() {
			t.Fatalf("合法战斗目标类别 %d 被拒绝", kind)
		}
	}
	for _, kind := range []core.CombatTargetKind{0, 4} {
		if kind.Valid() {
			t.Fatalf("非法战斗目标类别 %d 被接受", kind)
		}
	}
}
