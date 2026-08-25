# 攻击疲劳（B-13 攻击半边）

## Why

v25 服务端权威近战上线后，成功命中是玩家动作里唯一没有饥饿代价的高频成功路径：跳跃、采掘、翻地、回血都已在疲劳固定表上计价，唯独打人不收费。这与 authoritative-hunger「每个成功动作都有代价、比例关系即玩法」的曲线不一致。`docs/feature-backlog.md` B-13 明确「攻击疲劳半边已可先行」；冲刺半边依赖 B-30（协议升版），不在本 change。

## What Changes

- 疲劳来源固定表追加一行：一次成功近战命中累积固定疲劳量（参考实现攻击疲劳 0.1 × 1000 = 100 千分位，与既有五行同一换算口径）。
- 判定点在权威近战的意图冻结处：落空、被固体方块遮挡、目标处于受击冷却免疫窗口的输入不形成命中，不累积疲劳。
- 无协议、存档、ABI、benchmark scenario 变更；饱和度与疲劳值仍不上线，客户端零改动。

## Capabilities

### New Capabilities

（无）

### Modified Capabilities

- `authoritative-hunger`: 「疲劳来源是固定表」Requirement 的动作集合从五项扩展为六项——新增「近战命中成功」，并补齐对应可判定 Scenario。

## Impact

- 受影响代码：`internal/sim/hunger.go`（固定表加行）、`internal/sim/combat.go`（意图冻结处调用 `applyExhaustion`）、同包测试（新增聚焦测试文件；既有共享 melee 测试 helper 按测试组织纪律迁入中心 `helpers_test.go`）。
- 兼容性：无 wire/schema 变更；三层状态推进仍是纯整数确定性运算，Memory 与 TCP 复用同一权威路径；既有玩家在满饥饿/满饱和初值下单次命中疲劳远低于阈值，对既有测试与存档行为无观察差异。
- 非目标：冲刺疲劳（依赖 B-30 协议升版）、武器伤害差异化结算（A-06 统一战斗接通）、伙伴任何疲劳语义。

## 延期与放弃

- 冲刺与攻击疲劳的冲刺半边：依赖 B-30 冲刺输入位（协议升版），待其落地后另行认领。
