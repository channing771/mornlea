# container-ui-visual-alignment 执行 ledger

共享基线：`cff1133f62782b04a36a0461549edb64be877de2`。本文件只记录已经发生且可复验的事实；`待派发`、`待评审` 和 `待执行` 不代表通过。每个执行 Task 的 finding 最多进入 5 轮追加修复/同一 reviewer 复审，超限后由 controller 逐项裁决。

| Task | Implementer | Candidate / fix commits | Reviewer | SPEC | QUALITY | 修复轮次 | 验证证据 | Controller ruling |
|---|---|---|---|---|---|---:|---|---|
| 1 OpenSpec change | `/root/hud_merge_impl`（本轮 fresh implementer） | 待本任务提交 | 待派发 fresh task reviewer | 待评审 | 待评审 | 0/5 | change 创建前 `openspec status`、apply instructions 与 strict validate 均因 change 不存在而预期失败；GREEN 为 strict validate 与 `git diff --check` 均成功 | Task brief 是唯一需求与精确值来源；本任务只创建 7 个 OpenSpec 规划文件，不改产品代码/golden，不派生子代理 |
| 2 程序化容器 atlas | 待派发 fresh implementer | 待执行 | 待派发 fresh task reviewer | 待评审 | 待评审 | 0/5 | 待执行 | 待裁决 |
| 3 overlay/interaction redlines | 待派发 fresh implementer | 待执行 | 待派发 fresh task reviewer | 待评审 | 待评审 | 0/5 | 待执行 | 待裁决 |
| 4 capture/golden | 待派发 fresh implementer | 待执行 | 待派发 fresh task reviewer | 待评审 | 待评审 | 0/5 | 待执行 | 待裁决 |
| 5 closeout | 待派发 fresh implementer | 待执行 | 待派发 fresh whole-branch reviewer | 待评审 | 待评审 | 0/5 | 待执行 | 待裁决 |

## Task 1 现状与决策记录

- `2026-08-23`：worktree `codex/container-ui-visual-alignment` 在共享基线 `cff1133f62782b04a36a0461549edb64be877de2` 上 clean；唯一 implementer 为 `/root/hud_merge_impl`，未派生子代理或 reviewer。
- 结构 RED：change 创建前，`openspec status --change container-ui-visual-alignment --json` 与 `openspec instructions apply --change container-ui-visual-alignment --json` 均 EXIT 1 并报告 change not found；`openspec validate container-ui-visual-alignment --strict --no-interactive` EXIT 1 并报告 unknown item。
- 当前行为红线由代码/测试核实：`InventorySlotAt` 为 36 格 `0..35`，`FurnaceSlotAt` 为 39 格 `0..38`，`ChestSlotAt` 为 63 格 `0..62`；UI 固定配方为 10 条；第一次点击只选来源，第二次才发送一次整堆移动且确认前不改镜像。
- 当前固定资源为 267 quad、700 glyph、13312-byte glyph offset、46912-byte 总容量、48-byte instance、256-byte 对齐；三种打开态互斥，当前合法最大为 265 quad。每 overlay 只新增一个零 glyph 标题后最大为 266，无需版本或容量迁移。
- 当前正式 capture 恰好 15 项。本 change 把新场景命名为 `furnace-container`、`chest-container` 并依次插在 `inventory-crafting` 后，最终恰好 17 项；末三项继续是 `water-surface-slope`、`far-horizon`、`water-underwater`，两张 diagnostic controls 不计入正式场景。
- `2026-08-23`：`openspec validate container-ui-visual-alignment --strict --no-interactive` 通过，`git diff --check` 零输出；apply instructions 确认规划产物 complete、后续实现进度 `0/23`，本 Task 不提前勾选。
