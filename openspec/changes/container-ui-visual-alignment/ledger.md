# container-ui-visual-alignment 执行 ledger

共享基线：`cff1133f62782b04a36a0461549edb64be877de2`。本文件只记录已经发生且可复验的事实；`待派发`、`待评审` 和 `待执行` 不代表通过。每个执行 Task 的 finding 最多进入 5 轮追加修复/同一 reviewer 复审，超限后由 controller 逐项裁决。

| Task | Implementer | Candidate / fix commits | Reviewer | SPEC | QUALITY | 修复轮次 | 验证证据 | Controller ruling |
|---|---|---|---|---|---|---:|---|---|
| 1 OpenSpec change | `/root/hud_merge_impl`（本轮 fresh implementer） | `f36eb5bb3686403fa1e70cadb90688d4a27276bc`; `93a2d8d81a48777997d5ee11d3bb31e520d6fe3f` | `/root/container_task1_review`（`task-1-review.md`；修复轮次 1 复审见 `task-1-review-round-1.md`） | PASS（修复轮次 1 复审） | FAIL（修复轮次 1 复审） | 2/5 进行中 | change 创建前结构 RED；fix 1 strict/diff 通过；修复轮次 1 复审为 Spec PASS / Quality FAIL，1 finding | controller 只接受补齐 fix SHA 与 reviewer 审计事实，进入修复轮次 2；本轮复审待结论 |
| 2 程序化容器 atlas | 待派发 fresh implementer | 待执行 | 待派发 fresh task reviewer | 待评审 | 待评审 | 0/5 | 待执行 | 待裁决 |
| 3 overlay/interaction redlines | 待派发 fresh implementer | 待执行 | 待派发 fresh task reviewer | 待评审 | 待评审 | 0/5 | 待执行 | 待裁决 |
| 4 capture/golden | 待派发 fresh implementer | 待执行 | 待派发 fresh task reviewer | 待评审 | 待评审 | 0/5 | 待执行 | 待裁决 |
| 5 closeout | 待派发 fresh implementer | 待执行 | 待派发 fresh whole-branch reviewer | 待评审 | 待评审 | 0/5 | 待执行 | 待裁决 |

## Task 1 现状与决策记录

- `2026-08-23`：worktree `codex/container-ui-visual-alignment` 在共享基线 `cff1133f62782b04a36a0461549edb64be877de2` 上 clean；唯一 implementer 为 `/root/hud_merge_impl`，未派生子代理或 reviewer。
- 结构 RED：change 创建前，`openspec status --change container-ui-visual-alignment --json` 与 `openspec instructions apply --change container-ui-visual-alignment --json` 均 EXIT 1 并报告 change not found；`openspec validate container-ui-visual-alignment --strict --no-interactive` EXIT 1 并报告 unknown item。
- 当前行为红线由代码/测试核实：`InventorySlotAt` 为 36 格 `0..35`，`FurnaceSlotAt` 为 39 格 `0..38`，`ChestSlotAt` 为 63 格 `0..62`；UI 固定配方为 10 条；第一次点击只选来源，第二次才发送一次整堆移动且确认前不改镜像。
- 当前固定资源为 267 quad、700 glyph、13312-byte glyph offset、46912-byte 总容量、48-byte instance、256-byte 对齐；三种打开态互斥，当前合法最大为 265 quad。每 overlay 只新增一个零 glyph 标题后最大为 266，无需版本或容量迁移。
- 当前正式 capture 恰好 15 项。本 change 把新场景命名为 `chest-container`、`furnace-container` 并依次插在 `inventory-crafting` 后，最终恰好 17 项；其余 15 项相对顺序不变，末三项继续是 `water-surface-slope`、`far-horizon`、`water-underwater`，两张 diagnostic controls 不计入正式场景。
- `2026-08-23`：`openspec validate container-ui-visual-alignment --strict --no-interactive` 通过，`git diff --check` 零输出；apply instructions 确认规划产物 complete、后续实现进度 `0/23`，本 Task 不提前勾选。
- `2026-08-23`：首轮独立 review 在 `task-1-review.md` 记录 SPEC FAIL / QUALITY FAIL；报告未署 reviewer 身份，ledger 不猜测。Controller ruling 只修正场景顺序、火焰/箭头 atlas cell 与裁剪接线、20px header 几何边界和 candidate SHA，未修改产品代码或 golden。
- `2026-08-23`：修复轮次 1 的定向 strict validate 通过，全量 strict validate 为 58 passed / 0 failed，`git diff --check` 零输出；此证据不代表独立复审结论。
- `2026-08-23`：独立 reviewer `/root/container_task1_review` 在 `task-1-review-round-1.md` 确认修复轮次 1 为 Spec PASS / Quality FAIL，唯一 finding 是 ledger 遗漏 fix commit `93a2d8d81a48777997d5ee11d3bb31e520d6fe3f` 与 reviewer 身份；修复轮次 2 只补这两项已发生事实，不声称尚未发生的复审 PASS。
