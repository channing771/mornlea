package entity

import (
	"reflect"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// 本文件锁定弹道推进契约：每权威 tick 恰好一步、先施加重力再按位移积分
// （f32 与玩家/敌怪物理同语义，`physics.FixedDeltaSeconds` 同源步长），以及
// 相同世界种子与相同输入序列的两次独立推进逐位一致（生成、轨迹全程）。

func TestProjectileIntegratesGravityBeforeStepEachTick(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	refreshViewsForTest(engine)
	id := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 30, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)

	step := physics.FixedDeltaSeconds
	velocity := mgl32.Vec3{22, 0, 0}
	position := mgl32.Vec3{0.5, 30, 0.5}
	for tick := range 3 {
		// 文档化的积分公式：速度先减 gravity*dt，位置再加 velocity*dt。
		velocity = velocity.Sub(mgl32.Vec3{0, projectileGravity * step, 0})
		position = position.Add(velocity.Mul(step))
		engine.advanceProjectiles(&TickResult{})
		entry, ok := projectileAt(engine, id)
		if !ok {
			t.Fatalf("第 %d tick 投射物消失", tick)
		}
		if entry.velocity != velocity {
			t.Fatalf("第 %d tick 速度=%v，想要 %v", tick, entry.velocity, velocity)
		}
		if entry.position != position {
			t.Fatalf("第 %d tick 位置=%v，想要 %v", tick, entry.position, position)
		}
	}
	// 重力逐 tick 累积：水平分量不变，竖直分量严格递减。
	entry, _ := projectileAt(engine, id)
	if entry.velocity.X() != 22 || entry.velocity.Z() != 0 {
		t.Fatalf("水平速度被意外修改：%v", entry.velocity)
	}
	if entry.age != 3 {
		t.Fatalf("推进步数计数=%d，想要 3", entry.age)
	}
}

func TestProjectileReplayIsBitIdentical(t *testing.T) {
	run := func() []projectileState {
		engine := NewEngine(0, 0, 7)
		engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
		refreshViewsForTest(engine)
		for step := range 60 {
			if step == 5 {
				spawnTestProjectile(
					t, engine, projectileKindShard, core.Overworld,
					mgl32.Vec3{0.5, 20, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
				)
			}
			if step == 25 {
				spawnTestProjectile(
					t, engine, projectileKindArrow, core.Overworld,
					mgl32.Vec3{2.5, 25, 1.5}, mgl32.Vec3{-16, 4, 2}, 1, projectileArrowFullDamage,
				)
			}
			engine.advanceProjectiles(&TickResult{})
		}
		return append([]projectileState(nil), engine.projectiles.entries...)
	}
	first, second := run(), run()
	if len(first) != 2 {
		t.Fatalf("重放结束时在飞投射物=%d，想要 2", len(first))
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("两次独立推进的投射物状态不一致：\n%+v\n%+v", first, second)
	}
}

func TestProjectileFirstMovementHappensInSpawnTick(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	refreshViewsForTest(engine)
	id := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 30, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)
	engine.advanceProjectiles(&TickResult{})
	entry, ok := projectileAt(engine, id)
	if !ok {
		t.Fatal("出生 tick 未推进投射物")
	}
	step := physics.FixedDeltaSeconds
	if entry.position != (mgl32.Vec3{0.5 + 22*step, 30 - projectileGravity*step*step, 0.5}) {
		t.Fatalf("出生 tick 首步位置=%v，与固定步长积分不符", entry.position)
	}
	if entry.age != 1 {
		t.Fatalf("出生 tick 步数计数=%d，想要 1", entry.age)
	}
}
