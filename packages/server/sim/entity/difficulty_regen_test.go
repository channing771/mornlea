package entity

import (
	"testing"

	"github.com/channing771/mornlea/packages/shared/core"
)

// 本文件锁定自然回血的难度分档（spec authoritative-hunger「自然回血按难度
// 条件化门控并消耗疲劳」）：normal 的门控与疲劳读数由 health_regen_test.go
// 的既有用例逐位钉住（缺省夹具即 normal），这里补 peaceful 的无门控 + 回血
// 后恢复完整饥饿，以及 hard 沿用同一门控阈值的分档证据。

// TestHealthRegenPeacefulIgnoresGateAndRestoresHunger 覆盖 Scenario「和平
// 难度无门控且回血后恢复完整饥饿」：饥饿 5 远低于阈值 18，回血照常在第 140
// tick 发生；饥饿与饱和度恢复到上限对应的完整值，疲劳仍按同一回血疲劳规则
// 累积（6000 跨一次 4000 阈值、消耗一点饱和度，余 2000——恢复覆盖消耗效果，
// 残值保留）。
func TestHealthRegenPeacefulIgnoresGateAndRestoresHunger(t *testing.T) {
	const id = SessionID(54)
	engine := readyRegenPlayerAtDifficulty(t, id, core.DifficultyPeaceful, 10)
	player := engine.sessions[id].player
	player.hunger = 5
	player.saturationMilli = 3000
	player.exhaustionMilli = 0

	stepRegen(t, engine, id, 139)
	if player.health != 10 {
		t.Fatalf("peaceful 第 139 tick health=%d，想要保持 10（回复不应提前）", player.health)
	}
	if player.hunger != 5 || player.saturationMilli != 3000 {
		t.Fatalf("peaceful 回血前 (饥饿,饱和)=(%d,%d)，想要保持 (5,3000)",
			player.hunger, player.saturationMilli)
	}
	stepRegen(t, engine, id, 1)
	if player.health != 11 {
		t.Fatalf("peaceful 饥饿 5 时第 140 tick health=%d，想要 11（门控取消）", player.health)
	}
	if player.hunger != core.MaxHunger {
		t.Fatalf("peaceful 回血后饥饿=%d，想要 %d", player.hunger, core.MaxHunger)
	}
	wantSaturation := uint16(core.MaxHunger) * core.SaturationMilliPerPoint
	if player.saturationMilli != wantSaturation {
		t.Fatalf("peaceful 回血后饱和=%d，想要上限对应最大值 %d",
			player.saturationMilli, wantSaturation)
	}
	if player.exhaustionMilli != 2000 {
		t.Fatalf("peaceful 回血后疲劳=%d，想要 2000（同一回血疲劳规则累积）",
			player.exhaustionMilli)
	}
	if player.saturationZero {
		t.Fatal("peaceful 回血后饱和度不应标记为零")
	}
}

// TestHealthRegenPeacefulFullHealthLeavesHungerUntouched 覆盖 Scenario「和平
// 难度未实际回血不改饥饿状态」的满血半边：满血玩家推进任意 tick 都不回血，
// 饥饿与饱和度必须原样保持——「无条件每 tick 恢复完整饥饿」的实现会在这里
// 当场变红。
func TestHealthRegenPeacefulFullHealthLeavesHungerUntouched(t *testing.T) {
	const id = SessionID(55)
	engine := readyRegenPlayerAtDifficulty(t, id, core.DifficultyPeaceful, core.MaxHealth)
	player := engine.sessions[id].player
	player.hunger = 5
	player.saturationMilli = 3000

	stepRegen(t, engine, id, 220)
	if player.health != core.MaxHealth {
		t.Fatalf("peaceful 满血推进后 health=%d，想要保持 %d", player.health, core.MaxHealth)
	}
	if player.hunger != 5 || player.saturationMilli != 3000 || player.exhaustionMilli != 0 {
		t.Fatalf("peaceful 满血推进后 (饥饿,饱和,疲劳)=(%d,%d,%d)，想要保持 (5,3000,0)",
			player.hunger, player.saturationMilli, player.exhaustionMilli)
	}
}

// TestHealthRegenHardKeepsHungerGate 钉住 hard 与 normal 共用同一门控阈值：
// 饥饿 17 不回血、饥饿 18 恰好在第 140 tick 回血并累积同量疲劳——hard 只改
// 饥饿伤害的致死性，不动回血门控与疲劳表。
func TestHealthRegenHardKeepsHungerGate(t *testing.T) {
	const id = SessionID(56)
	blocked := readyRegenPlayerAtDifficulty(t, id, core.DifficultyHard, 10)
	blocked.sessions[id].player.hunger = 17
	stepRegen(t, blocked, id, 140)
	if player := blocked.sessions[id].player; player.health != 10 {
		t.Fatalf("hard 饥饿 17 第 140 tick health=%d，想要保持 10（门控沿用）", player.health)
	}

	const other = SessionID(57)
	allowed := readyRegenPlayerAtDifficulty(t, other, core.DifficultyHard, 10)
	allowed.sessions[other].player.hunger = 18
	stepRegen(t, allowed, other, 140)
	player := allowed.sessions[other].player
	if player.health != 11 {
		t.Fatalf("hard 饥饿 18 第 140 tick health=%d，想要 11（阈值处放行）", player.health)
	}
	if player.exhaustionMilli != 2000 || player.saturationMilli != core.InitialSaturationMilli-1000 {
		t.Fatalf("hard 回血后 (疲劳,饱和)=(%d,%d)，想要 (2000,%d)（同一疲劳表）",
			player.exhaustionMilli, player.saturationMilli, core.InitialSaturationMilli-1000)
	}
}
