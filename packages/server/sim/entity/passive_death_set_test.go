package entity

import (
	"slices"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// TestSettlePassiveDeathsRecordsDeathSet 锁定死亡原因的事实来源：同 tick
// 结算移除的个体 ID 必须留在当 tick 死亡集合里，供发布侧投影原因位；下一次
// 无死亡的结算必须清空集合，不跨 tick 累积。
func TestSettlePassiveDeathsRecordsDeathSet(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	first := validTestPassive(21)
	first.State = physics.State{Position: mgl32.Vec3{2.5, 1, 2.5}, OnGround: true}
	second := validTestPassive(22)
	second.State = physics.State{Position: mgl32.Vec3{4.5, 1, 4.5}, OnGround: true}
	if err := engine.RestorePassive(first); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	if err := engine.RestorePassive(second); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	engine.DamagePassive(21, int32(core.MaxHealth), mgl32.Vec3{10.5, 1, 2.5})
	engine.settlePassiveDeaths(engine.newMutation())
	if got := engine.PassiveDeaths(); !slices.Equal(got, []uint64{21}) {
		t.Fatalf("死亡集合=%v，想要 [21]（存活个体不进集合）", got)
	}
	engine.settlePassiveDeaths(engine.newMutation())
	if got := engine.PassiveDeaths(); len(got) != 0 {
		t.Fatalf("无死亡结算后集合=%v，想要空（不跨 tick 累积）", got)
	}
}
