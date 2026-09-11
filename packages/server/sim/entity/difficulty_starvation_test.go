package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定饥饿归零结算的难度分档（spec authoritative-hunger「饥饿归零按
// 固定间隔扣血且致死性由难度决定」）：normal 的间隔边界与一点生命硬地板由
// hunger_test.go 的既有用例逐位钉住（缺省夹具即 normal），这里只补 hard 与
// peaceful 两档，并复用同一套 `readyRegenPlayerAtDifficulty` 夹具保证三档
// 差异只来自难度本身。

// TestStarvationHardCrossesOneHealthFloor 覆盖 Scenario「困难难度饥饿可致死」
// 的伤害半边：hard 从 2 点生命起，第一个间隔照常扣到 1，第二个间隔必须
// **穿过** normal 的硬地板把生命打到 0——只推进到 1 的实现与 normal 不可
// 区分，因此第二个间隔的断言不可省。
func TestStarvationHardCrossesOneHealthFloor(t *testing.T) {
	const id = SessionID(50)
	engine := readyRegenPlayerAtDifficulty(t, id, core.DifficultyHard, 2)
	player := engine.sessions[id].player
	player.hunger = 0
	player.saturationMilli = 0

	stepRegen(t, engine, id, 80)
	if player.health != 1 {
		t.Fatalf("hard 第一个间隔后 health=%d，想要 1", player.health)
	}
	// 第二个间隔刻意不走 stepRegen：硬地板已取消，生命会在归零当 tick 立即
	// 进入死亡结算，helper 的 Ready 检查会把红点错报成「失去 Ready」。
	for range 80 {
		advanceActorsTick(engine)
	}
	if player.health != 0 {
		t.Fatalf("hard 第二个间隔后 health=%d，想要 0（硬地板已取消）", player.health)
	}
}

// TestStarvationHardDiesThroughUnifiedDeathSettlement 覆盖 Scenario「困难
// 难度饥饿可致死」的死亡半边：hard 下 1 点生命的玩家挨过一次饥饿伤害间隔，
// 必须经既有死亡结算死亡——生命回满、饥饿回到固定初值、进入待重生——
// 死亡反馈、快照与持久化因此复用统一路径，不存在饥饿专用的第二条死亡分支。
func TestStarvationHardDiesThroughUnifiedDeathSettlement(t *testing.T) {
	const id = SessionID(51)
	engine := readyRegenPlayerAtDifficulty(t, id, core.DifficultyHard, 1)
	player := engine.sessions[id].player
	player.hunger = 0
	player.saturationMilli = 0

	// 间隔边界与 normal 逐位一致：第 79 tick 不扣血。
	for range 79 {
		advanceActorsTick(engine)
	}
	if player.health != 1 {
		t.Fatalf("hard 第 79 tick health=%d，想要保持 1", player.health)
	}
	// 第 80 tick 扣血归零；死亡结算挂在 hostile 阶段（settleDeaths），与
	// hunger_test.go 的重生用例同一编排。
	tick := engine.beginTick()
	tick.context.AdvanceActors()
	tick.context.AdvanceHostiles(nil, &tick.result)
	commitMutation(tick.mutation, &tick.result)
	publishFixture(engine, &tick)
	if player.lifecycle != PlayerPendingSpawn {
		t.Fatalf("hard 饥饿归零后 lifecycle=%v，想要 PendingSpawn（经既有死亡结算）",
			player.lifecycle)
	}
	if player.health != core.MaxHealth {
		t.Fatalf("死亡结算后 health=%d，想要回满 %d", player.health, core.MaxHealth)
	}
	if player.hunger != core.MaxHunger || player.saturationMilli != core.InitialSaturationMilli {
		t.Fatalf("死亡结算后 (饥饿,饱和)=(%d,%d)，想要固定初值 (%d,%d)",
			player.hunger, player.saturationMilli, core.MaxHunger, core.InitialSaturationMilli)
	}
}

// TestStarvationPeacefulNeverDamages 覆盖 Scenario「和平难度饥饿归零不扣血」：
// peaceful 玩家满血且饥饿归零——满血让回血短路（饥饿不会被回血恢复），饥饿
// 伤害间隔走满三个也没有任何一扣，更没有死亡。
func TestStarvationPeacefulNeverDamages(t *testing.T) {
	const id = SessionID(52)
	engine := readyRegenPlayerAtDifficulty(t, id, core.DifficultyPeaceful, core.MaxHealth)
	player := engine.sessions[id].player
	player.hunger = 0
	player.saturationMilli = 0

	for range 240 {
		advanceActorsTick(engine)
	}
	if player.health != core.MaxHealth {
		t.Fatalf("peaceful 饿满三个间隔后 health=%d，想要保持 %d",
			player.health, core.MaxHealth)
	}
	if player.hunger != 0 {
		t.Fatalf("peaceful 满血短路下饥饿=%d，想要保持 0（未被恢复）", player.hunger)
	}
	if player.lifecycle != PlayerActive {
		t.Fatalf("peaceful 饿满三个间隔后 lifecycle=%v，想要 Active", player.lifecycle)
	}
}

// TestStarvationPeacefulDoesNotResetRegenTimer 钉住「不重置回血计时」这一
// 半句：非满血玩家的回血计时应照常累积到 100——normal 下第 80 tick 的饥饿
// 伤害会经 `applyDamage` 把计时清零，读数只会停在 20。夹具停在 100 tick
// 刻意避开第 140 tick 的首次 peaceful 回血（那次回血会把饥饿恢复到满）。
func TestStarvationPeacefulDoesNotResetRegenTimer(t *testing.T) {
	const id = SessionID(53)
	engine := readyRegenPlayerAtDifficulty(t, id, core.DifficultyPeaceful, 10)
	player := engine.sessions[id].player
	player.hunger = 0
	player.saturationMilli = 0

	for range 100 {
		advanceActorsTick(engine)
	}
	if player.health != 10 {
		t.Fatalf("peaceful 饥饿归零推进 100 tick 后 health=%d，想要保持 10", player.health)
	}
	if player.ticksSinceDamage != 100 {
		t.Fatalf("peaceful 回血计时=%d，想要 100（不因饥饿伤害被重置）",
			player.ticksSinceDamage)
	}
}
