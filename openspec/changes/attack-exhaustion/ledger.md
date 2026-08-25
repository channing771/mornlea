# attack-exhaustion Ledger

## 2026-08-25 认领与确认

- 认领：`docs/feature-backlog.md` B-13 攻击疲劳半边，认领人 `ox-alpha-implementer @ feat/B-13-attack-exhaustion`，docs-only 提交 `bcc900fb`（主仓 main）。
- 路径分类：**bounded**（仓库既有流程改动——既有近战成功路径加一行固定疲劳表项），对话内短设计。
- 设计确认：用户显式批准（「批准」）。设计要点：`exhaustionMeleeMilli = 100`（参考实现 0.1 × 1000 同口径）；判定点在 `advancePlayerMelee` 意图冻结处；共享 melee 测试 helper 先迁 `helpers_test.go` 中心再写新测试文件。

## Rulings

- Ruling: 判定点选在意图冻结处而非伤害结算循环 — 冻结即收费与该处既有副作用聚合点（采掘抑制、目标冷却写入）同位，且结算循环保持 `applyDamage` 单一职责，未来 A-06 统一战斗调整结算顺序时疲劳语义不被动漂移 — 放结算循环会让资源语义寄生在伤害结算顺序上。

## 任务进度

- Task 1.1：待执行
- Task 1.2：待执行
- Task 2.1：待执行
- Task 3.1：待执行
- Task 3.2：待执行

## 评审结论

- （尚未进入评审）
