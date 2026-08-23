# container-ui-visual-alignment 执行 ledger

共享基线：`cff1133f62782b04a36a0461549edb64be877de2`。本文件只记录已经发生且可复验的事实；`待派发`、`待评审` 和 `待执行` 不代表通过。每个执行 Task 的 finding 最多进入 5 轮追加修复/同一 reviewer 复审，超限后由 controller 逐项裁决。

| Task | Implementer | Candidate / fix commits | Reviewer | SPEC | QUALITY | 修复轮次 | 验证证据 | Controller ruling |
|---|---|---|---|---|---|---:|---|---|
| 1 OpenSpec change | `/root/hud_merge_impl`（本轮 fresh implementer） | `f36eb5bb3686403fa1e70cadb90688d4a27276bc`; `93a2d8d81a48777997d5ee11d3bb31e520d6fe3f` | `/root/container_task1_review`（`task-1-review.md`；修复轮次 1 复审见 `task-1-review-round-1.md`） | PASS（修复轮次 1 复审） | FAIL（修复轮次 1 复审） | 2/5 进行中 | change 创建前结构 RED；fix 1 strict/diff 通过；修复轮次 1 复审为 Spec PASS / Quality FAIL，1 finding | controller 只接受补齐 fix SHA 与 reviewer 审计事实，进入修复轮次 2；本轮复审待结论 |
| 2 程序化容器 atlas | `/root/container_task2_impl`（本轮 fresh implementer） | `506395c95595b7187377888d4c914fe2608fd2d9`; `07f28e079cb2c8bfc5ef6981afcbaf3dcca5c485` | `/root/container_task2_review`（`task-2-review.md`） | PASS | FAIL | 1/5 进行中 | RED、HUD race、archcheck、vet、format、diff check；fix 1 focused 与 strict 已通过 | 仅接受 P2 注释修复；同一 reviewer 复审待发生 |
| 3 overlay/interaction redlines | `/root/container_task3_impl`（本轮 fresh implementer） | `79a98947b45bdb2818871ee01846a7564452ca85`; `c7598752835af837d923ecc4bfacab11759fe6e3` | 待派发 fresh task reviewer | 待评审 | 待评审 | 1/5 复审待发生 | RED、HUD focused/race、archcheck、vet、format、diff check 通过；cmd race 未完成 | 待裁决 |
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

## Task 2 已发生事实

- `2026-08-23`：fresh implementer `/root/container_task2_impl` 在 Task 1 的 `a87b5cc47dbf017fbe8fc4f61f40cfaf16a0c935` 基线上开始；先运行 `make rust`，完成后不派生子代理或 reviewer。
- RED：`make rust && go test ./internal/render/hud -run 'TestHotbarAtlas.*(Container|Item)|TestContainerPixelCells' -count=1` 以 six undefined atlas cell constants 编译失败，符合新增固定列尚未实现的预期。
- GREEN：追加六个固定 cell（列 7..12）后，`hotbarBlockColumnOffset=13`；七个 survival cell 的逐像素 SHA-256、所有可放置 `core.ItemID` 的注册表顶面逐像素比较、六个 cell 的非空/互异/二值 alpha/重复构建与全部 UV 界内均由 HUD 测试覆盖。
- GREEN：背包、合成、熔炉与箱子的栏位 surface 都采同一凹槽 UV；背包栏位的真实生产入口在 `layout.go`，故该最小 root-cause 修改与 `container.go` 同时提交，未改变坐标、hit-test、item tile、数量或耐久。
- 验证：`go test ./internal/render/hud -run 'TestHotbarAtlas|TestContainer' -race -count=1`、`go test ./internal/render/hud -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./internal/render/hud`、`gofmt -l`（目标文件）与 `git diff --check` 均通过。
- 候选提交：`506395c95595b7187377888d4c914fe2608fd2d9`（`feat: add original container pixel primitives`）。Task reviewer 尚未派发；不得将本段视为 SPEC/QUALITY PASS。
- `2026-08-23`：独立 reviewer `/root/container_task2_review` 在 `task-2-review.md` 对 `a87b5cc..fc4ea55` 给出 SPEC PASS / QUALITY FAIL；唯一 P2 是 `atlas.go` 的列说明遗漏六个 container cell。修复轮次 1 只改该注释，复审结论尚未发生。
- 修复提交：`07f28e079cb2c8bfc5ef6981afcbaf3dcca5c485`（`docs: correct container atlas column comment`）；`gofmt`、brief HUD focused、strict OpenSpec 与 `git diff --check` 均通过。

## Task 3 已发生事实

- `2026-08-23`：fresh implementer `/root/container_task3_impl` 在 Task 2 修复提交基线上开始，先运行 `make rust`；未派生子代理或 reviewer。
- RED：`go test ./internal/render/hud -run 'Test(ContainerTitles|ContainerSlotGeometry|Furnace.*Composition|RecipeButton)' -count=1` 因三个共享 header 常量尚不存在而编译失败。
- GREEN：三种互斥 overlay 各追加一个 atlas 标题 quad；panel 只向上扩 20px，36/39/63 格及十条配方的 origin/hit-test 保持原路径；熔炉两项填充复用既有 bar quad，并分别按火焰自下向上、箭头自左向右同步裁剪实例与 UV。
- 验证：同一 focused 命令、`go test ./internal/render/hud -race -count=1`、`go test ./internal/archcheck -count=1`、`go vet ./internal/render/hud`、目标 `gofmt -l` 与 `git diff --check` 均通过。三次 `cmd/mornlea` race 被工具 30 秒回合限制留下重复运行，已精确停止；它们没有通过结论。
- 候选提交：`79a98947b45bdb2818871ee01846a7564452ca85`（`feat: align container UI presentation`）。未进行 reviewer 双裁决，不得视为 SPEC/QUALITY PASS。
- `2026-08-23`：独立审查 `task-3-review.md` 对候选范围给出 SPEC PASS / QUALITY FAIL；唯一 P2 指出三尺寸集缺少 panel 四边与 `InventorySlotAt`/`FurnaceSlotAt`/`ChestSlotAt` 半开命中证明。
- 修复轮次 1：只扩展 `layout_test.go` 的既有表驱动测试。三种 framebuffer 直接读取三类 overlay panel，锁定旧 X/Width/底边不变及仅向上扩 `containerHeaderHeight*scale`；同一尺寸集穷举 36/39/63 格的左上闭、中心与右/下开命中。未改生产代码，复审结论尚未发生。
- 修复轮次 1 验证：focused HUD、HUD race、archcheck、目标 `gofmt -l`、`git diff --check` 与 strict OpenSpec 均通过。
- 修复提交：`c7598752835af837d923ecc4bfacab11759fe6e3`（`test: lock container header boundaries`）。
