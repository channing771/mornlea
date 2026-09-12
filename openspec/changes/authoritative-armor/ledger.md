# Ledger：权威护甲

## 基线

- 分支 `feat/B-24-armor`（worktree `.worktrees/B-24-armor`），基于 main `f10fbb57`。
- 版本槽位持有：协议 v41→v42、玩家 schema v8→v9、client ABI v18→v19。

## 内容确认（阶段 1）

- 2026-09-12 控制会话以 brainstorming 分类为 **bounded**（沿门/床/夜行者行先例的既有流程改动），短设计经用户**显式批准**（AskUserQuestion 四项裁决全取推荐项）：仅铁质一套、4 槽、点数 + 4%/点减免、耐久随本行交付。
- 已核实的代码事实：`playerState.applyDamage` 为全部伤害来源唯一入口（`combat.go:384`、`player.go:646`、`oxygen.go:36`、`hunger.go:158`）；近战结算单点 `settleCombatIntent`；`PlayerState` 尾部追加先例与协议 v41；HUD 状态行已迁 WebView 组件；`ItemIDMax = 58`、C→S 命令 ID 下一空闲 18（ID 1 已废止不复用）。

## Rulings

- `Ruling: 护甲四件的快捷栏/HUD 物品 sprite 由顺延改为任务组 6 交付 — 实现者实测客户端存在「所有注册物品必须有图标」的守护测试（`packages/client/cmd/mornlea/app` 21 例因物品 58 缺图标变红），顺延将使分支长期带红测试 — 2026-09-12 proposal 延期节原决策被推翻，proposal 与 tasks.md 已同步修订。`
- `Ruling: 损坏形态护甲以「数量 1、耐久 0」原地表达，不新增损坏护甲物品编号 — 规格只允许 58..61 四个编号；`ArmorPoints` 完好判定 = Count≥1 且耐久 1..上限，越界 fail closed 计 0 点 — 与 design D6 一致，测试已钉住。`
- `Ruling: `ReducedDamage` 对非正伤害返回 0 — 沿 core 值域函数不 panic 惯例；中间量 int64 收敛 int32 防极端回绕 — 实现者裁量，测试已钉住。`
- 待办登记：`packages/shared/core/recipe_shape_internal_test.go` 头注释「recipe 1..19」陈旧（现 1..24，断言仍成立），后续任务组顺手修正。

## 验证证据（按 SHA）

- SHA `0fb77e74`（任务组 1）：`go test ./packages/shared/core -race -count=1` → ok 1.461s，verbose 235 PASS / 0 FAIL；`gofmt -l packages/shared/core` 无输出；`go vet ./packages/shared/core/...` 干净；worktree 首跑前已执行 `make rust`（成功），六模块 `go build ./...` 通过。已知下游影响：`packages/client/cmd/mornlea/app` 21 例因护甲缺图标红（任务组 6 交付 sprite 后清偿）。
