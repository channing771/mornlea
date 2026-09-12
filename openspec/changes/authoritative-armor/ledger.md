# Ledger：权威护甲

## 基线

- 分支 `feat/B-24-armor`（worktree `.worktrees/B-24-armor`），基于 main `f10fbb57`。
- 版本槽位持有：协议 v41→v42、玩家 schema v8→v9、client ABI v18→v19。

## 内容确认（阶段 1）

- 2026-09-12 控制会话以 brainstorming 分类为 **bounded**（沿门/床/夜行者行先例的既有流程改动），短设计经用户**显式批准**（AskUserQuestion 四项裁决全取推荐项）：仅铁质一套、4 槽、点数 + 4%/点减免、耐久随本行交付。
- 已核实的代码事实：`playerState.applyDamage` 为全部伤害来源唯一入口（`combat.go:384`、`player.go:646`、`oxygen.go:36`、`hunger.go:158`）；近战结算单点 `settleCombatIntent`；`PlayerState` 尾部追加先例与协议 v41；HUD 状态行已迁 WebView 组件；`ItemIDMax = 58`、C→S 命令 ID 下一空闲 18（ID 1 已废止不复用）。

## Rulings

（实现期登记，格式：`Ruling: <决定什么> — <为什么> — <错在哪>`）

## 验证证据（按 SHA）

（每任务组的验证命令与输出摘要登记处；同一 SHA 且工作区未再改动时后续任务直接引用）
