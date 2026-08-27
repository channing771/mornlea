# Ledger：authoritative-bed-sleep

> 记录：控制会话裁决（ruling）、每 Task 评审结论与修复循环、最终验证输出摘要。

## 内容确认记录（brainstorming 硬门禁，2026-08-28）

- **分类**：architectural（新方块子系统 + 玩家 schema v8 + 世界 metadata v3 + 协议字段追加 + 显示相位语义扩展）。
- **探索**：backlog 行「床与睡眠」（原 `codex-implementer @ feat/A-05-authoritative-bed-sleep` 履历与共享契约 SHA `785ea07b` 均已丢失、本无实现损失）；现行主规格核对：`authoritative-daylight`（绝对时间 + metadata v2 + 客户端相位）、`authoritative-health`（死亡重生回出生锚点）、`internal/sim/door.go`（双格原子放置/交互/采掘先例）、`internal/sim/death.go`（`beginReset` 重生路径）、`internal/storage/metadata.go`（v2 定长布局 + v1 迁移先例）、`internal/storage/player_types.go`（v7 无重生点字段）、`internal/render/daylight.go`（`DayLengthTicks=24000`）。
- **Ruling: 并行互动（2026-08-28，用户裁决「解耦：无条件可睡」）** — 睡眠不检查夜行者；跳夜后白昼灼烧按夜行者既有规则自然结算；两行唯一共享契约为 `core.DisplayDayPhase(ticks, offset)`（A-04 交付、本行提供 offset 生产端）。为什么：契约面最小、两线真并行；靠近拒睡等耦合玩法留待后续行。
- **Ruling: A-05-approval（2026-08-28，approve）** — 按节呈现的设计（共享契约 / A-04 重定基线 / A-05 范围与配方 / 编排与门禁）经用户显式批准；床配方定为「顶排 3 小麦 + 下排 3 橡木木板 → 床 ×1」（麦秸床垫，材料可再生，与门 2×3 形状不冲突）。
- **Ruling: 合并序（2026-08-28）** — A-04 先合并（交付 `DisplayDayPhase` 与 S→C 22/23/24），本行 rebase 后合并；协议版本号由本行合并时基于届时 `main` 取下一空闲（A-04 取 v30 则本行 v31）；`bed-night` 场景插在 `torch-night` 之后、`ai-companion` 之前，与 A-04 的 `hostile-mob` 插入点互不冲突。

## 变更产物

- [x] `openspec/changes/authoritative-bed-sleep/`：proposal/5 delta specs/design/tasks/ledger 已建于本 worktree 功能分支。

## 评审记录（Task 1 起，逐 Task 追加）

- （待逐 Task 填：SPEC 合规结论 / QUALITY 结论 / 修复轮 R1..Rn / 对应 Ruling）

## 最终验证输出摘要（收尾补）

- （待整分支终审后补：make rust、focused -race、archcheck、vet、gofmt、openspec strict、visual-check 的数值摘要；benchmark 数值只记录）
