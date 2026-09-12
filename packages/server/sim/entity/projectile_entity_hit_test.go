package entity

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/packages/shared/core"
	"github.com/channing771/mornlea/packages/shared/physics"
)

// 本文件锁定命中结算契约：只经既有伤害入口结算（玩家 `applyDamage`、敌怪
// `applyDamage`、被动 `DamagePassive`）；玩家目标先按命中时点冻结的护甲点数
// 减免、实际减免才耗全件耐久、沿弹速水平分量击退；玩家所有的箭命中实体向
// 持有者会话追加既有近战命中私有确认；骨刺只命中玩家、箭不命中持有者本人；
// 掠过实体未命中则继续飞行；弹击致死与近战致死同 tick 完成掉落与移除。

func TestArrowHitsHostileAppliesDamageKnockbackAndConfirmation(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(21)
	mob.State.Position = mgl32.Vec3{5.5, 1, 0.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	id := spawnTestProjectile(
		t, engine, projectileKindArrow, core.Overworld,
		mgl32.Vec3{0.5, 1.9, 0.5}, mgl32.Vec3{22, 0, 0}, 1, projectileArrowFullDamage,
	)
	var result TickResult
	for range 4 {
		engine.advanceProjectiles(&result)
	}
	if _, ok := projectileAt(engine, id); !ok {
		t.Fatal("第 4 tick 未抵达夜行者，不应提前消失")
	}
	engine.advanceProjectiles(&result)
	if _, ok := projectileAt(engine, id); ok {
		t.Fatal("命中实体的箭未消失")
	}
	hostile := &engine.hostiles.entries[0]
	if hostile.health != core.MaxHealth-uint8(projectileArrowFullDamage) {
		t.Fatalf("夜行者生命=%d，想要 %d", hostile.health, core.MaxHealth-uint8(projectileArrowFullDamage))
	}
	if hostile.state.Velocity != (mgl32.Vec3{0.35, 0, 0}) {
		t.Fatalf("夜行者击退=%v，想要沿弹速水平分量 (0.35,0,0)", hostile.state.Velocity)
	}
	if len(result.CombatHits) != 1 {
		t.Fatalf("命中确认数=%d，想要 1", len(result.CombatHits))
	}
	hit := result.CombatHits[0]
	if hit.Session != SessionID(1) || hit.Damage != uint8(projectileArrowFullDamage) ||
		hit.TargetKind != core.CombatTargetHostile {
		t.Fatalf("命中确认=%+v，想要持有者会话 1 / 伤害 %d / 敌怪目标", hit, projectileArrowFullDamage)
	}
}

func TestArrowKillSettlesHostileDeathSameTick(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(21)
	mob.Health = 1
	mob.State.Position = mgl32.Vec3{5.5, 1, 0.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	spawnTestProjectile(
		t, engine, projectileKindArrow, core.Overworld,
		mgl32.Vec3{0.5, 1.9, 0.5}, mgl32.Vec3{22, 0, 0}, 1, projectileArrowFullDamage,
	)
	// 弹道飞行 5 tick 后命中；完整 hostile 阶段在投射物阶段之后紧随死亡结算。
	var result TickResult
	for range 5 {
		result = advanceHostilesTick(engine, nil)
	}
	if len(engine.hostiles.entries) != 0 {
		t.Fatalf("0 血夜行者存活到了阶段结束：%+v", engine.hostiles.entries)
	}
	if countLoadedDrops(t, engine, core.ItemRottenFlesh) != 1 {
		t.Fatal("弹击致死未在同一 tick 完成掉落结算")
	}
	found := false
	for _, hit := range result.CombatHits {
		if hit.Session == SessionID(1) && hit.TargetKind == core.CombatTargetHostile {
			found = true
		}
	}
	if !found {
		t.Fatalf("致死命中未产生持有者确认：%+v", result.CombatHits)
	}
}

func TestArrowHitsOtherPlayerButNotOwner(t *testing.T) {
	engine, owner := readyMovementPlayer(t)
	const other = SessionID(2)
	engine.RegisterSession(other, core.Overworld, core.ChunkPos{})
	advanceActorsTick(engine)
	engine.SetPlayerPositionForTest(owner, mgl32.Vec3{0.5, 1, 0.5})
	engine.SetPlayerPositionForTest(other, mgl32.Vec3{9.5, 1, 0.5})
	// 出生高度 2.8：8 tick 飞行期间弹道保持离地（1.9 高度会在第 6 tick 触地）。
	id := spawnTestProjectile(
		t, engine, projectileKindArrow, core.Overworld,
		mgl32.Vec3{0.5, 2.8, 0.5}, mgl32.Vec3{22, 0, 0}, uint64(owner), projectileArrowFullDamage,
	)
	var result TickResult
	for range 8 {
		engine.advanceProjectiles(&result)
	}
	if _, ok := projectileAt(engine, id); ok {
		t.Fatal("命中玩家后箭未消失")
	}
	ownerPlayer := engine.sessions[owner].player
	otherPlayer := engine.sessions[other].player
	if ownerPlayer.health != core.MaxHealth {
		t.Fatalf("持有者生命=%d，想要不被自己的箭命中", ownerPlayer.health)
	}
	if otherPlayer.health != core.MaxHealth-uint8(projectileArrowFullDamage) {
		t.Fatalf("另一名玩家生命=%d，想要 %d", otherPlayer.health, core.MaxHealth-uint8(projectileArrowFullDamage))
	}
	if len(result.CombatHits) != 1 || result.CombatHits[0].Session != owner ||
		result.CombatHits[0].TargetKind != core.CombatTargetPlayer {
		t.Fatalf("命中确认=%+v，想要持有者会话的玩家目标确认", result.CombatHits)
	}
}

func TestShardHitsPlayerWithArmorReductionAndDurabilityDrain(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	player := engine.sessions[session].player
	player.armor = [core.ArmorSlotCount]core.ItemStack{
		core.ArmorSlotHead:  {Item: core.ItemIronHelmet, Count: 1, Durability: 165},
		core.ArmorSlotChest: {Item: core.ItemIronChestplate, Count: 1, Durability: 240},
		core.ArmorSlotLegs:  {Item: core.ItemIronLeggings, Count: 1, Durability: 225},
		core.ArmorSlotFeet:  {Item: core.ItemIronBoots, Count: 1, Durability: 195},
	}
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{9.5, 1, 0.5})
	// 出生高度 2.8：8 tick 飞行期间弹道保持离地（1.9 高度会在第 6 tick 触地）。
	id := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 2.8, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)
	var result TickResult
	for range 8 {
		engine.advanceProjectiles(&result)
	}
	if _, ok := projectileAt(engine, id); ok {
		t.Fatal("命中玩家后骨刺未消失")
	}
	// 15 点护甲按既有减免公式折算：3×40/100=1，向下取整后有效伤害 1。
	if got := player.health; got != core.MaxHealth-1 {
		t.Fatalf("玩家生命=%d，想要 %d", got, core.MaxHealth-1)
	}
	// 实际产生减免：参与护甲件各消耗 1 点耐久。
	if got := player.armor[core.ArmorSlotHead].Durability; got != 164 {
		t.Fatalf("头盔耐久=%d，想要 164", got)
	}
	if got := player.armor[core.ArmorSlotChest].Durability; got != 239 {
		t.Fatalf("胸甲耐久=%d，想要 239", got)
	}
	if got := player.armor[core.ArmorSlotLegs].Durability; got != 224 {
		t.Fatalf("护腿耐久=%d，想要 224", got)
	}
	if got := player.armor[core.ArmorSlotFeet].Durability; got != 194 {
		t.Fatalf("靴子耐久=%d，想要 194", got)
	}
	if player.state.Velocity != (mgl32.Vec3{0.35, 0, 0}) {
		t.Fatalf("玩家击退=%v，想要沿弹速水平方向 (0.35,0,0)", player.state.Velocity)
	}
	// 骨刺不是玩家所有的箭：不产生私有命中确认。
	if len(result.CombatHits) != 0 {
		t.Fatalf("骨刺命中不应产生确认：%+v", result.CombatHits)
	}
}

func TestShardIgnoresPassiveCowEvenWhenSegmentCrossesBoth(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	cow := PassiveMob{
		ID:        7,
		Dimension: core.Overworld,
		State:     physicsStateAt(mgl32.Vec3{5.5, 1, 0.5}),
		Health:    core.MaxHealth,
	}
	if err := engine.RestorePassive(cow); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{9.5, 1, 0.5})
	// 出生高度 2.8：8 tick 飞行期间弹道保持离地，且第 5 tick 仍穿过牛的身体。
	spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 2.8, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)
	var result TickResult
	for range 8 {
		engine.advanceProjectiles(&result)
	}
	passive := &engine.passives.entries[0]
	if passive.health != core.MaxHealth || passive.fleeTicks != 0 {
		t.Fatalf("被动牛被骨刺命中：生命=%d 逃跑=%d", passive.health, passive.fleeTicks)
	}
	if got := engine.sessions[session].player.health; got != core.MaxHealth-uint8(projectileShardDamage) {
		t.Fatalf("玩家生命=%d，想要被骨刺命中减 %d（无护甲全额伤害）", got, projectileShardDamage)
	}
	if len(result.CombatHits) != 0 {
		t.Fatalf("骨刺命中不应产生确认：%+v", result.CombatHits)
	}
}

func TestShardIgnoresItsThrowerAndKeepsFlying(t *testing.T) {
	engine, session := readyMovementPlayer(t)
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{12.5, 1, 12.5})
	mob := validTestHostile(21)
	mob.State.Position = mgl32.Vec3{0.5, 1, 0.5}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	id := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 1.9, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)
	engine.advanceProjectiles(&TickResult{})
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth {
		t.Fatalf("掷骨者生命=%d，想要不被自己的骨刺命中", got)
	}
	if _, ok := projectileAt(engine, id); !ok {
		t.Fatal("被发射者阻挡判定命中的骨刺不应消失")
	}
}

func TestArrowHitsPassiveCowThroughDamagePassive(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	cow := PassiveMob{
		ID:        7,
		Dimension: core.Overworld,
		State:     physicsStateAt(mgl32.Vec3{5.5, 1, 0.5}),
		Health:    core.MaxHealth,
	}
	if err := engine.RestorePassive(cow); err != nil {
		t.Fatalf("恢复被动牛：%v", err)
	}
	id := spawnTestProjectile(
		t, engine, projectileKindArrow, core.Overworld,
		mgl32.Vec3{0.5, 1.9, 0.5}, mgl32.Vec3{22, 0, 0}, 1, projectileArrowFullDamage,
	)
	var result TickResult
	for range 4 {
		engine.advanceProjectiles(&result)
	}
	entry, _ := projectileAt(engine, id)
	source := entry.position
	engine.advanceProjectiles(&result)
	if _, ok := projectileAt(engine, id); ok {
		t.Fatal("命中被动牛后箭未消失")
	}
	passive := &engine.passives.entries[0]
	if passive.health != core.MaxHealth-uint8(projectileArrowFullDamage) {
		t.Fatalf("被动牛生命=%d，想要 %d", passive.health, core.MaxHealth-uint8(projectileArrowFullDamage))
	}
	if passive.fleeTicks != passiveFleeDurationTicks {
		t.Fatalf("逃跑计时=%d，想要 %d", passive.fleeTicks, passiveFleeDurationTicks)
	}
	if passive.fleeFrom != source {
		t.Fatalf("逃跑来源=%v，想要命中时刻的弹体位置 %v", passive.fleeFrom, source)
	}
	if passive.state.Velocity != (mgl32.Vec3{0.35, 0, 0}) {
		t.Fatalf("被动牛击退=%v，想要 (0.35,0,0)", passive.state.Velocity)
	}
	if len(result.CombatHits) != 1 || result.CombatHits[0].TargetKind != core.CombatTargetPassive {
		t.Fatalf("命中确认=%+v，想要被动目标确认", result.CombatHits)
	}
}

func TestProjectileGrazeKeepsFlyingWithoutSettlement(t *testing.T) {
	engine, _ := readyMovementPlayer(t)
	mob := validTestHostile(21)
	// 身体 z 区间 [2.2,2.8]：弹道 z=1.8 与其最近距离 0.4，小于半格但不相交。
	mob.State.Position = mgl32.Vec3{5.5, 1, 2.4}
	if err := engine.RestoreHostile(mob); err != nil {
		t.Fatalf("恢复夜行者：%v", err)
	}
	id := spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 1.9, 1.8}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)
	for range 5 {
		engine.advanceProjectiles(&TickResult{})
	}
	if _, ok := projectileAt(engine, id); !ok {
		t.Fatal("掠过未命中的投射物不应消失")
	}
	if got := engine.hostiles.entries[0].health; got != core.MaxHealth {
		t.Fatalf("被掠过的夜行者生命=%d，想要不变", got)
	}
}

func TestProjectileSettlementUnaffectedByPeacefulDifficulty(t *testing.T) {
	engine := NewEngine(0, 0, 0, core.DifficultyPeaceful)
	session := SessionID(1)
	engine.RegisterSession(session, core.Overworld, core.ChunkPos{})
	loadMovementChunk(t, engine.dimension(core.Overworld), movementFlatChunk(core.ChunkPos{}))
	advanceActorsTick(engine)
	engine.SetPlayerPositionForTest(session, mgl32.Vec3{9.5, 1, 0.5})
	// 出生高度 2.8：8 tick 飞行期间弹道保持离地（1.9 高度会在第 6 tick 触地）。
	spawnTestProjectile(
		t, engine, projectileKindShard, core.Overworld,
		mgl32.Vec3{0.5, 2.8, 0.5}, mgl32.Vec3{22, 0, 0}, 21, projectileShardDamage,
	)
	for range 8 {
		engine.advanceProjectiles(&TickResult{})
	}
	// 和平档只门控敌怪生成，不给伤害路径增加第二套分支：命中照常全额结算
	//（无护甲 → 骨刺 3 点）。
	if got := engine.sessions[session].player.health; got != core.MaxHealth-uint8(projectileShardDamage) {
		t.Fatalf("和平档玩家生命=%d，想要 %d", got, core.MaxHealth-uint8(projectileShardDamage))
	}
}

// physicsStateAt 返回给定脚底位置的最小合法物理体。
func physicsStateAt(position mgl32.Vec3) physics.State {
	return physics.State{Position: position, OnGround: true}
}
