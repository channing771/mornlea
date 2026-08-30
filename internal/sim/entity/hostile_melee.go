package entity

// 夜行者近战伤害的固定数值契约：3 点伤害、1.8 格水平攻击距离、20 tick 冷却
// （冷却周期复用 `hostileCooldownPeriodTicks`）。距离常量导出给 server 编排层
// 复用——停移裁决与结算重验必须使用同一个边界，两侧各自抄写数字必然漂移。
const (
	// hostileMeleeDamage 是一次近战命中的伤害点数。
	hostileMeleeDamage = int32(3)
)

// hostileAttackRangeSquared 是攻击距离边界的平方。刻意用变量承载运行时乘积而
// 非常量折算：1.8 不能被 float32 精确表示，先乘后舍入与常量折叠的结果在边界
// 附近可能相差一个 ULP，server 侧的停移裁决必须与这里的表达式逐位一致。
var hostileAttackRangeSquared = HostileAttackRange * HostileAttackRange

// advanceHostileMelee 推进夜行者近战：先把全集合的攻击冷却逐 tick 递减（固定
// 顺序），再按 ID 升序统一结算本 tick 已冻结的攻击意图。意图冻结发生在更早
// 的 applyHostileActions（同 tick、任何结算之前），因此结算循环开始后不再收
// 集新意图——「先冻结全部意图、再统一结算」的同 tick 纪律由阶段次序保证。
// 结算顺序即切片顺序（ID 升序）；每个意图经 damageHostileTarget 做权威重验
// 与伤害落地，受击保护与距离不达的意图被确定性跳过。
func (engine *Engine) advanceHostileMelee() {
	for index := range engine.hostiles.entries {
		entry := &engine.hostiles.entries[index]
		if entry.attackCooldown > 0 {
			entry.attackCooldown--
		}
	}
	var intents [maxHostiles]int
	count := 0
	for index := range engine.hostiles.entries {
		entry := &engine.hostiles.entries[index]
		if entry.attackIntent && entry.health > 0 {
			intents[count] = index
			count++
		}
	}
	for _, index := range intents[:count] {
		entry := &engine.hostiles.entries[index]
		if entry.attackCooldown != 0 {
			// 冷却中的夜行者不结算：意图即使被编排层重复冻结也在此拦下。
			continue
		}
		engine.damageHostileTarget(entry)
	}
}

// damageHostileTarget 是夜行者近战伤害的收编接缝：对一次已冻结的攻击意图做
// 全部权威重验（目标会话存在且玩家激活、同维、存活、未处受击保护期、水平距
// 离在攻击边界内），全部通过才经既有 `applyDamage` 唯一伤害入口结算 3 点伤
// 命——回血计时重置、受伤中断进食与归零后的同 tick 死亡结算都由该入口的既
// 有语义承载，这里绝不复制它们。命中后攻击者进入满冷却、目标进入受击保护期
// （复用玩家近战的同一份保护计时，玩家近战与敌怪近战共享同一受击免疫语义）。
//
// 这条窄通道是统一近战结算（同维最近命中、等距按身份、击退与冷却）落地前的
// 临时形态：统一战斗收编时，本函数与 advanceHostileMelee 的意图收集一并迁移，
// 数值用例随行。
func (engine *Engine) damageHostileTarget(attacker *hostileState) {
	session := engine.sessions[attacker.attackTargetSession]
	if session == nil || session.player == nil ||
		session.player.lifecycle != PlayerActive {
		return
	}
	if session.dimension != attacker.dimension {
		return
	}
	target := session.player
	if target.health == 0 {
		return
	}
	if target.meleeCooldownTicks != 0 {
		return
	}
	if horizontalDistanceSq(attacker.state.Position, target.state.Position) > hostileAttackRangeSquared {
		return
	}
	target.applyDamage(hostileMeleeDamage)
	target.meleeCooldownTicks = hostileCooldownPeriodTicks
	attacker.attackCooldown = hostileCooldownPeriodTicks
}
