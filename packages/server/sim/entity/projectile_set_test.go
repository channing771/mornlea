package entity

import (
	"math"
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定投射物集合契约：按 ID 严格升序的定容切片（容量 128，无 map）、
// 集合满时「ID 最旧（最小）先清」腾位且新投射物正常存在，以及生成 ID 的
// 确定性派生——非零、相同输入逐位一致、冲突沿哈希链重散列不碰撞。

func TestProjectileSetKeepsIDsStrictlySortedWithOldestEviction(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	for i := range maxProjectiles {
		id, ok := engine.spawnProjectile(
			projectileKindShard,
			core.Overworld,
			mgl32.Vec3{0.5, float32(i) + 1, 0.5},
			mgl32.Vec3{0, 0, 0},
			21,
			projectileShardDamage,
		)
		if !ok || id == 0 {
			t.Fatalf("第 %d 条生成失败：ok=%v id=%d", i, ok, id)
		}
	}
	if len(engine.projectiles.entries) != maxProjectiles {
		t.Fatalf("集合长度=%d，想要 %d", len(engine.projectiles.entries), maxProjectiles)
	}
	for index := 1; index < len(engine.projectiles.entries); index++ {
		if engine.projectiles.entries[index-1].id >= engine.projectiles.entries[index].id {
			t.Fatalf("集合未按 ID 严格升序：下标 %d/%d", index-1, index)
		}
	}

	oldest := engine.projectiles.entries[0].id
	spawned := make(map[uint64]struct{}, maxProjectiles)
	for _, entry := range engine.projectiles.entries {
		spawned[entry.id] = struct{}{}
	}
	newID, ok := engine.spawnProjectile(
		projectileKindShard,
		core.Overworld,
		mgl32.Vec3{0.5, 200, 0.5},
		mgl32.Vec3{0, 0, 0},
		21,
		projectileShardDamage,
	)
	if !ok || newID == 0 {
		t.Fatalf("集合满后再生成失败：ok=%v id=%d", ok, newID)
	}
	if engine.projectiles.findIndex(oldest) >= 0 {
		t.Fatalf("ID 最旧（最小）的投射物 %d 未被挤出", oldest)
	}
	if engine.projectiles.findIndex(newID) < 0 {
		t.Fatalf("挤位后的新投射物 %d 不在集合中", newID)
	}
	if len(engine.projectiles.entries) != maxProjectiles {
		t.Fatalf("挤位后集合长度=%d，想要仍为 %d", len(engine.projectiles.entries), maxProjectiles)
	}
	// 被挤出的恰好一条、其余条目原样保留。
	removed := 0
	for _, id := range idsOfProjectileEntries(engine) {
		if _, known := spawned[id]; !known {
			removed++
		}
	}
	if removed != 1 {
		t.Fatalf("挤位移除的既有条目数=%d，想要 1", removed)
	}
}

func idsOfProjectileEntries(engine *Engine) []uint64 {
	ids := make([]uint64, 0, len(engine.projectiles.entries))
	for index := range engine.projectiles.entries {
		ids = append(ids, engine.projectiles.entries[index].id)
	}
	return ids
}

func TestProjectileSpawnIDIsDeterministicAcrossRuns(t *testing.T) {
	const seed = int64(42)
	build := func() []uint64 {
		engine := NewEngine(0, 0, seed)
		ids := make([]uint64, 0, 3)
		id, ok := engine.spawnProjectile(
			projectileKindShard, core.Overworld,
			mgl32.Vec3{0.5, 2.5, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
		)
		if !ok {
			t.Fatal("骨刺生成失败")
		}
		ids = append(ids, id)
		engine.tick.Store(77)
		id, ok = engine.spawnProjectile(
			projectileKindArrow, core.Overworld,
			mgl32.Vec3{1.5, 3.5, 1.5}, mgl32.Vec3{-16, 4, 2}, 1, projectileArrowFullDamage,
		)
		if !ok {
			t.Fatal("箭生成失败")
		}
		ids = append(ids, id)
		engine.tick.Store(78)
		id, ok = engine.spawnProjectile(
			projectileKindShard, core.Overworld,
			mgl32.Vec3{0.5, 2.5, 0.5}, mgl32.Vec3{22, 0, 0}, 33, projectileShardDamage,
		)
		if !ok {
			t.Fatal("同位异主骨刺生成失败")
		}
		return append(ids, id)
	}
	first, second := build(), build()
	for index := range first {
		if first[index] == 0 {
			t.Fatalf("第 %d 个 ID 为零", index)
		}
		if first[index] != second[index] {
			t.Fatalf("第 %d 个 ID 两次运行不一致：%d vs %d", index, first[index], second[index])
		}
	}
	// 不同输入（tick、弹种、发射者）必须派生不同 ID，集合内无重复。
	if first[0] == first[1] || first[0] == first[2] || first[1] == first[2] {
		t.Fatalf("不同输入派生出重复 ID：%v", first)
	}
}

func TestProjectileSpawnRejectsInvalidFacts(t *testing.T) {
	engine := NewEngine(0, 0, 0)
	if _, ok := engine.spawnProjectile(
		7, core.Overworld,
		mgl32.Vec3{0.5, 1, 0.5}, mgl32.Vec3{1, 0, 0}, 21, projectileShardDamage,
	); ok {
		t.Fatal("非法弹种被受理")
	}
	if _, ok := engine.spawnProjectile(
		projectileKindShard, core.DimensionID(9),
		mgl32.Vec3{0.5, 1, 0.5}, mgl32.Vec3{1, 0, 0}, 21, projectileShardDamage,
	); ok {
		t.Fatal("未知维度被受理")
	}
	if _, ok := engine.spawnProjectile(
		projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 1, 0.5}, mgl32.Vec3{1, 0, 0}, 21, 0,
	); ok {
		t.Fatal("零伤害被受理")
	}
	if _, ok := engine.spawnProjectile(
		projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 1, float32(math.Inf(1))}, mgl32.Vec3{1, 0, 0}, 21, projectileShardDamage,
	); ok {
		t.Fatal("非有限出生位置被受理")
	}
	if len(engine.projectiles.entries) != 0 {
		t.Fatalf("被拒生成留下了条目：%d", len(engine.projectiles.entries))
	}
}
