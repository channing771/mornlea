package sim

import (
	"testing"

	"github.com/go-gl/mathgl/mgl32"

	"github.com/channing771/mornlea/internal/core"
)

// 本文件只证一个性质组：近战命中的疲劳代价（OpenSpec change attack-exhaustion）。
//
// 规则：成功近战命中时攻击者经 `playerState.applyExhaustion` 累积恰好 100 千分位
// 疲劳；未命中的每一条路径都不累积。断言一律使用字面量数值而非引用常量——这是
// 为让 RED 阶段的失败是行为缺失而非符号未定义；如今 `exhaustionMeleeMilli`
// 已落地，保留字面量以维持「规格钉死值」的显式性。

// TestCombatHitChargesAttackerExhaustionExactlyOnce 覆盖 Scenario「近战命中累积
// 攻击者疲劳」：按住 primary input（v25 语义下 `miningHeld` 同时是近战意图）推进
// 至发生命中，攻击者的 `exhaustionMilli` 必须恰好 +100，目标的疲劳一字不动。
//
// 命中已发生由目标生命值从 20 降到 18 自证：没有这次命中，「疲劳不变色」的断言
// 会在一个根本没打中人的夹具上空绿。
func TestCombatHitChargesAttackerExhaustionExactlyOnce(t *testing.T) {
	engine, sessions := readyMeleePlayers(t, 2)
	setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
	setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
	attacker := engine.sessions[sessions[0]].player
	target := engine.sessions[sessions[1]].player
	if attacker.exhaustionMilli != 0 || target.exhaustionMilli != 0 {
		t.Fatalf("夹具起始疲劳 (攻击者=%d, 目标=%d)，想要双方都是 0",
			attacker.exhaustionMilli, target.exhaustionMilli)
	}
	attacker.miningHeld = true

	engine.Step()
	if target.health != 18 {
		t.Fatalf("首 tick 目标 health=%d，想要 18（夹具必须真的命中一次）", target.health)
	}
	if got := attacker.exhaustionMilli; got != 100 {
		t.Fatalf("命中后攻击者疲劳=%d，想要恰好 100", got)
	}
	if got := target.exhaustionMilli; got != 0 {
		t.Fatalf("被命中一方的疲劳=%d，想要保持 0（挨打不是疲劳来源）", got)
	}
}

// TestCombatExhaustionThresholdCrossSettlesSaturationThenHunger 覆盖 Scenario
// 「累积跨过阈值走完饱和度→饥饿值整条结算链路」：把攻击者 `exhaustionMilli`
// 预置到 3950，一次命中累积 100 后总数 4050 恰好跨过一次 4000 阈值。
//
//   - 饱和度充足时：结算扣一点饱和度（1000 千分位），疲劳余量 4050−4000=50，
//     饥饿值不动；
//   - 饱和度为 0 时：同一笔累积改为扣一点饥饿值，饱和度保持 0。
func TestCombatExhaustionThresholdCrossSettlesSaturationThenHunger(t *testing.T) {
	t.Run("跨阈值扣饱和度", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		attacker := engine.sessions[sessions[0]].player
		target := engine.sessions[sessions[1]].player
		// 注册初值即满状态：饥饿 20、饱和度 5000。预置疲劳到距阈值一步之遥。
		attacker.exhaustionMilli = 3950
		attacker.miningHeld = true

		engine.Step()
		if target.health != 18 {
			t.Fatalf("目标 health=%d，想要 18（夹具必须真的命中一次）", target.health)
		}
		if got := attacker.exhaustionMilli; got != 50 {
			t.Fatalf("跨阈值后攻击者疲劳余量=%d，想要 50（4050 减去一次 4000 阈值）", got)
		}
		if got := attacker.saturationMilli; got != 4000 {
			t.Fatalf("跨阈值后攻击者饱和度=%d，想要 4000（5000 扣一点）", got)
		}
		if got := attacker.hunger; got != 20 {
			t.Fatalf("跨阈值后攻击者饥饿值=%d，想要保持 20（饱和度尚未耗尽）", got)
		}
	})

	t.Run("饱和度耗尽改扣饥饿值", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		attacker := engine.sessions[sessions[0]].player
		target := engine.sessions[sessions[1]].player
		attacker.exhaustionMilli = 3950
		attacker.saturationMilli = 0
		attacker.miningHeld = true

		engine.Step()
		if target.health != 18 {
			t.Fatalf("目标 health=%d，想要 18（夹具必须真的命中一次）", target.health)
		}
		if got := attacker.hunger; got != 19 {
			t.Fatalf("饱和度为零时攻击者饥饿值=%d，想要 19（改扣饥饿值）", got)
		}
		if got := attacker.exhaustionMilli; got != 50 {
			t.Fatalf("跨阈值后攻击者疲劳余量=%d，想要 50", got)
		}
		if got := attacker.saturationMilli; got != 0 {
			t.Fatalf("攻击者饱和度=%d，想要保持 0", got)
		}
	})
}

// TestCombatMissPathsAccumulateNoExhaustion 覆盖否定面：三条未命中路径都不给
// 攻击者累积任何疲劳。疲劳只挂在**成功命中**上，这是本性质组的承重边界——
// 「只要挥拳就计费」的错误实现会在这三条分支上全部现形：
//
//   - 视线内无目标：目标站在攻击者背后，射线前方没有任何候选；
//   - 固体方块遮挡：两名玩家之间放置石块挡住射线（与既有近战阻挡测试同位形）;
//   - 受击冷却免疫：目标 `hurtCooldownTicks` 为正，意图在冻结处被丢弃。
//
// 推进 9 个 tick：短于裸手采掘完成的 30 tick，避免夹具里可能存在的采掘进度
// 干扰疲劳读数；也短于路径 (c) 预置的 20 tick 冷却，保证整段推进都在免疫窗口内。
// 每条分支都以「目标生命值保持 20」自证确实一次都没命中。
func TestCombatMissPathsAccumulateNoExhaustion(t *testing.T) {
	t.Run("视线内无目标", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		// 攻击者朝 -Z 看，目标放在背后的 +Z 侧：射线扫不到的候选。
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 6.5}, 0)
		attacker := engine.sessions[sessions[0]].player
		target := engine.sessions[sessions[1]].player
		attacker.miningHeld = true

		for range 9 {
			engine.Step()
		}
		if target.health != 20 {
			t.Fatalf("背后目标 health=%d，想要 20（夹具必须一次未命中）", target.health)
		}
		if got := attacker.exhaustionMilli; got != 0 {
			t.Fatalf("无目标挥拳 9 tick 后疲劳=%d，想要精确 0", got)
		}
	})

	t.Run("固体方块遮挡", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		engine.SetBlockForTest(core.BlockPos{X: 0, Y: 2, Z: 3}, core.StoneID)
		attacker := engine.sessions[sessions[0]].player
		target := engine.sessions[sessions[1]].player
		attacker.miningHeld = true

		for range 9 {
			engine.Step()
		}
		if target.health != 20 {
			t.Fatalf("被遮挡目标 health=%d，想要 20（夹具必须一次未命中）", target.health)
		}
		if got := attacker.exhaustionMilli; got != 0 {
			t.Fatalf("射线被遮挡挥拳 9 tick 后疲劳=%d，想要精确 0", got)
		}
	})

	t.Run("受击冷却免疫", func(t *testing.T) {
		engine, sessions := readyMeleePlayers(t, 2)
		setMeleePlayer(engine, sessions[0], mgl32.Vec3{0.5, 1, 4.5}, 0)
		setMeleePlayer(engine, sessions[1], mgl32.Vec3{0.5, 1, 2.5}, 0)
		attacker := engine.sessions[sessions[0]].player
		target := engine.sessions[sessions[1]].player
		// 预置正冷却：意图收集要求目标 `hurtCooldownTicks` 恰为 0，
		// 冷却窗口内的每一次挥拳都必须在冻结处被丢弃。
		target.hurtCooldownTicks = 20
		attacker.miningHeld = true

		for range 9 {
			engine.Step()
		}
		if target.health != 20 {
			t.Fatalf("冷却中的目标 health=%d，想要 20（夹具必须一次未命中）", target.health)
		}
		if got := attacker.exhaustionMilli; got != 0 {
			t.Fatalf("目标处于受击冷却时挥拳 9 tick 后疲劳=%d，想要精确 0", got)
		}
	})
}
