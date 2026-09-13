package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定投射物消失契约：寿命 100 tick（第 101 个存活 tick 的阶段进入点
// 移除）、越出世界边界消失、离开全部会话订阅区消失。消失即从权威集合移除，
// 不保留半移除状态。

func TestProjectileDespawnsWhenLifetimeExhausted(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	refreshViewsForTest(engine)
	id := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 30, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)
	entry, ok := projectileAt(engine, id)
	if !ok {
		t.Fatal("投射物丢失")
	}
	// 直接把寿命推进到临界点：已完成 99 步。
	entry.age = projectileMaxLifetimeTicks - 1
	engine.projectiles.entries[engine.projectiles.findIndex(id)] = entry

	// 第 100 个存活 tick：照常推进一步并存活。
	engine.advanceProjectiles(&TickResult{})
	entry, ok = projectileAt(engine, id)
	if !ok || entry.age != projectileMaxLifetimeTicks {
		t.Fatalf("第 100 步后存在=%v 步数=%d，想要存活且 100", ok, entry.age)
	}
	// 第 101 个存活 tick 的阶段进入点：寿命耗尽，消失且不再推进。
	engine.advanceProjectiles(&TickResult{})
	if _, ok := projectileAt(engine, id); ok {
		t.Fatal("寿命耗尽的投射物未消失")
	}
}

func TestProjectileDespawnsOutsideWorldBounds(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	refreshViewsForTest(engine)
	below := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, float32(core.MinY) - 0.5, 0.5}, mgl32.Vec3{0, -5, 0}, 21, projectileShardDamage,
	)
	above := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, float32(core.MaxY) - 0.05, 0.5}, mgl32.Vec3{0, 5, 0}, 22, projectileShardDamage,
	)
	engine.advanceProjectiles(&TickResult{})
	if _, ok := projectileAt(engine, below); ok {
		t.Fatal("越出世界下界的投射物未消失")
	}
	if _, ok := projectileAt(engine, above); ok {
		t.Fatal("越出世界上界的投射物未消失")
	}
}

func TestProjectileDespawnsWhenNoSessionSubscribesChunk(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.setSessionViewForTest(session, SessionView{
		Ready:  true,
		Center: core.ChunkPos{},
		Radius: 1,
	}, true)
	inside := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 30, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)
	outside := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 30, 60.5}, mgl32.Vec3{22, 0, 0}, 22, projectileShardDamage,
	)
	engine.advanceProjectiles(&TickResult{})
	if _, ok := projectileAt(engine, outside); ok {
		t.Fatal("没有任何会话订阅所在 chunk 的投射物未消失")
	}
	if _, ok := projectileAt(engine, inside); !ok {
		t.Fatal("订阅区内的投射物被误删")
	}
}

func TestProjectileDespawnRuleIgnoresOtherDimensionSessions(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	engine.RegisterSession(1, core.Overworld, core.ChunkPos{})
	engine.realm.EnsureDimension(core.Depths)
	refreshViewsForTest(engine)
	// 主世界会话在订阅判定中只覆盖主世界：Depths 中的投射物没有任何会话订阅。
	id := spawnTestProjectile(
		t, engine, projectileKindShard, core.Depths,
		mgl32.Vec3{0.5, 30, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)
	engine.advanceProjectiles(&TickResult{})
	if _, ok := projectileAt(engine, id); ok {
		t.Fatal("无会话维度的投射物未按订阅区规则消失")
	}
}
