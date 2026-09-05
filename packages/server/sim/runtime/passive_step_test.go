package runtime

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/server/sim/entity"
	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// TestStepAdvancesPassiveMovement 锁定被动牛推进已接入权威 tick：恢复一头
// 健康个体后推进一个夜间 tick，位置必须因漫游积分改变；夜间可排除昼间生成
// 对计数的干扰，若接线缺失则位置纹丝不动。牛放在玩家 10 格外：6 格内会触发
// 闲时看人（含 1.5 格止步），那是 `passive_idle_test.go` 的领地，本测试只锁
// 推进接线本身。
func TestStepAdvancesPassiveMovement(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	// 夜间拨时让昼间生成早退，只观察已存在个体的移动。
	engine.SetWorldTimeForTest(13000)
	mob := entity.PassiveMob{
		ID:        7,
		Dimension: core.Overworld,
		State: physics.State{
			Position: mgl32.Vec3{10.5, 1, 0.5},
			OnGround: true,
		},
		Health: core.MaxHealth,
	}
	if err := engine.entities.RestorePassive(mob, engine.realm); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	before := engine.entities.PassiveMobs()[0].State.Position

	engine.Step()

	mobs := engine.entities.PassiveMobs()
	if len(mobs) != 1 {
		t.Fatalf("被动牛数量=%d，想要 1", len(mobs))
	}
	after := mobs[0].State.Position
	if after == before {
		t.Fatalf("被动牛位置未推进：before=%v after=%v", before, after)
	}
}
