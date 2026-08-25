# fix-hoe-harvest-durability ledger

规划表行：E-09 作物×锄头耐久豁免（`docs/feature-backlog.md`）。
分支：`fix/E-09-hoe-harvest-durability`（基于 main@ff80a690）。

## 内容确认（brainstorming）

| 轮次 | 时间（UTC） | 呈现 | 用户回复 |
|---|---|---|---|
| 1 | 2026-08-25T05:00 | bounded 短设计（豁免位置 = 玩家完成分叉；谓词 = `core.IsCrop` × `core.TillingTool`；伙伴/翻地/疲劳不动；MODIFIED tool-durability + 5 组测试） | `approve`（05:01，「批准」） |

结论已写入 `proposal.md` 与 `design.md`；批准来源：用户飞书显式 approve（请求 `E-09-approval`）。无澄清轮——来源（farming 遗留 16）已钉死范围，`core.TillingTool` 已显式排除损坏形态。

## Task 执行与评审

（随执行追加）
