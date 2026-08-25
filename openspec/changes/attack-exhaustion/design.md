# 攻击疲劳（B-13 攻击半边）设计

## Context

v25 近战的权威路径在 `internal/sim/combat.go` 的 `advancePlayerMelee`：同一份已排序 active 会话快照上先收集全部有效意图（目标存在、无方块遮挡、目标不在 `meleeCooldownTicks` 免疫窗口），再统一经 `applyDamage` 结算。疲劳机制在 `internal/sim/hunger.go`：六项判定所需的 `applyExhaustion(milli, thresholdMilli)` 与五项固定表已就位，其注释明确「新增疲劳来源是给这张表加一行，并在对应的成功路径上调用 applyExhaustion」。既有五行数值与参考实现严格同口径（参考值 ×1000：采掘 5、起跳 50、游泳每格 10、翻地 5、回血每点 6000）。

## Goals / Non-Goals

**Goals:**

- 成功近战命中按固定量累积攻击者疲劳，复用既有三层状态结算链（饱和度 → 饥饿值），零新概念。
- 判定点语义与「意图冻结后统一结算」的既有近战结构一致。

**Non-Goals:**

- 冲刺疲劳（依赖 B-30）；武器差异化疲劳或伤害（A-06 统一战斗）；伙伴任何疲劳语义（伙伴不在 `advancePlayerMelee` 的玩家会话遍历内，天然不涉及）。
- 把攻击疲劳做成 tunable——固定表纪律明确拒绝逐项可调。

## Decisions

1. **数值 `exhaustionMeleeMilli = 100`。** 参考实现攻击疲劳为 0.1，×1000 与既有五行同口径，维持「比例关系即玩法」的可解释性。被否决的替代：50（介于采掘与起跳之间、看似折中，但破坏与参考实现的逐行对应关系，且没有任何验收标准支撑）；做成 tunable（固定表纪律禁止，见 proposal）。
2. **判定点在意图冻结处，而非结算循环。** 通过目标冷却检查、即将写入 `intents` 时对攻击者调用 `attacker.applyExhaustion(exhaustionMeleeMilli, engine.tunables.ExhaustionThresholdMilli)`。理由：(a) 「冻结即收费」与该处已有的 `meleeSuppressedMining`/冷却写入同属一次成功的副作用聚合点；(b) 同 tick 被反击致死的攻击者仍支付已成功的命中代价，与「死亡交给 settleDeaths、意图照常结算」的对称性一致；(c) 结算循环保持只做 `applyDamage` 单一职责。被否决的替代：放在结算循环里收费——会让资源语义寄生在伤害结算顺序上，且若未来结算顺序调整（如 A-06 统一战斗改排序）疲劳语义被动漂移。
3. **阈值继续走 tunable 快照传参。** `applyExhaustion` 签名不变，调用点传 `engine.tunables.ExhaustionThresholdMilli`，与其他五个判定点完全同形；`exhaustionMeleeMilli` 本身是固定表常量，不新增 Tunables 字段。
4. **测试组织：先迁移共享 helper 再写新测试文件。** 现状核实：`readyMeleePlayers`/`setMeleePlayer` 定义在 `combat_test.go`，已被 `combat_test.go`、`player_health_test.go`、`mining_test.go` 三个文件引用，而包内没有 `helpers_test.go` 中心——本 change 新增第 4 个消费者，按测试组织纪律必须先迁入新建的 `helpers_test.go` 中心（零行为变化重组：测试函数与子测试名不动）。随后新建聚焦测试文件 `combat_exhaustion_test.go`，一个文件证一条性质组：命中累积（含跨阈值扣饱和→扣饥饿整链路）、落空/遮挡不累积、免疫窗口不累积。

## Risks / Trade-offs

- [新疲劳源影响既有近战测试断言] → 单次命中 100 千分位远低于阈值 4000，满初值玩家不发生饱和度/饥饿值变化；实现后全量跑 `./internal/sim -race -count=1` 验证零观察差异，若有测试显式断言疲劳字段则按新语义修正。
- [意图冻结处收费使「命中但攻击者当 tick 死亡」也付费] → 这是刻意的：命中已经发生，代价不应因后续结算顺序豁免；由聚焦测试锁定该语义。

## Migration Plan

无协议、存档、ABI、scenario 变更，无需迁移。回退即还原两处代码改动与新增测试，无数据兼容负担。

## Open Questions

（无）
