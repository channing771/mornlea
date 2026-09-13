package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定方块命中契约：以「上一位置到新位置」的线段经既有生产射线出口
//（`core.RaycastBlocks` + `core.InteractionTarget` 谓词）求首个命中；命中方
// 块的投射物同 tick 消失且不掉落；流体不是目标；墙体遮挡时后方的实体不受击。

func TestProjectileDespawnsWhenSegmentEntersSolidBlock(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	// 夹具玩家站在出生点，会被同位的弹体在 t=0 命中：挪出飞行线。
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{12.5, 1, 12.5})
	for _, y := range []int32{1, 2, 3} {
		engine.SetBlockForTest(core.BlockPos{X: 3, Y: y, Z: 0}, core.StoneID)
	}
	id := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 1.5, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)

	// 前两步在墙前飞行（0.5→1.6→2.7），不得命中。
	for tick := range 2 {
		engine.advanceProjectiles(&TickResult{})
		if _, ok := projectileAt(engine, id); !ok {
			t.Fatalf("第 %d tick 提前命中方块", tick)
		}
	}
	// 第三步线段 2.7→3.8 穿过 x=3 的石墙：同 tick 消失、不掉落、无命中确认。
	var result TickResult
	engine.advanceProjectiles(&result)
	if _, ok := projectileAt(engine, id); ok {
		t.Fatal("穿墙投射物未在命中 tick 消失")
	}
	if len(result.CombatHits) != 0 {
		t.Fatalf("方块命中不应产生命中确认：%+v", result.CombatHits)
	}
	if countLoadedDrops(t, engine, core.ItemArrow) != 0 {
		t.Fatal("方块命中不应掉落箭物品")
	}
}

func TestProjectileFliesThroughFluidBlocks(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{12.5, 1, 12.5})
	for _, y := range []int32{1, 2} {
		engine.SetBlockForTest(core.BlockPos{X: 3, Y: y, Z: 0}, core.WaterSourceID)
	}
	id := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 1.5, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)
	engine.advanceProjectiles(&TickResult{})
	engine.advanceProjectiles(&TickResult{})
	var result TickResult
	engine.advanceProjectiles(&result)
	entry, ok := projectileAt(engine, id)
	if !ok {
		t.Fatal("流体被当作命中目标，投射物提前消失")
	}
	if entry.position.X() <= 3 {
		t.Fatalf("投射物未穿过流体墙：x=%v", entry.position.X())
	}
	if len(result.CombatHits) != 0 {
		t.Fatalf("流体命中不应产生命中确认：%+v", result.CombatHits)
	}
}

func TestProjectileBlockOccludesEntityBehindWall(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{12.5, 1, 12.5})
	for _, y := range []int32{1, 2} {
		engine.SetBlockForTest(core.BlockPos{X: 3, Y: y, Z: 0}, core.StoneID)
	}
	mob := validTestHostile(21)
	mob.State.Position = mgl32.Vec3{3.5, 1, 0.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	id := spawnTestProjectile(
		t, engine, projectileKindArrow, core.Overworld,
		mgl32.Vec3{0.5, 1.9, 0.5}, mgl32.Vec3{22, 0, 0}, 1, projectileArrowFullDamage,
	)
	var result TickResult
	engine.advanceProjectiles(&result)
	engine.advanceProjectiles(&result)
	engine.advanceProjectiles(&result)
	if _, ok := projectileAt(engine, id); ok {
		t.Fatal("箭未在命中墙体的 tick 消失")
	}
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth {
		t.Fatalf("墙后夜行者生命=%d，想要不受弹击", got)
	}
	if len(result.CombatHits) != 0 {
		t.Fatalf("被墙遮挡的命中不应产生确认：%+v", result.CombatHits)
	}
}
