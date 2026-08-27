# Ledger

## 历史段（旧基线 `b2115a64`，已丢失，仅作参考）

| 日期 | 阶段 | 结论 | 验证 |
| --- | --- | --- | --- |
| 2026-08-25 | 0.5 确认 Q1 | **Ruling:** 原批次共享契约提交随机器迁移丢失，不存在全仓 `reserve first-night survival contracts`；本轮由本链重建——先产出 Task 1 冻结内容（OpenSpec 全套产物、docs-only、无行为），A-02 完成后推分支开 PR 但**不合并**（待集成），由 A-06 统一合流。用户选 A。 | 全仓 grep 无预留提交；worktree 干净 @ ca8e9d5c。 |
| 2026-08-25 | 0.5 确认 Q2 | **Ruling:** engine ABI v6 → v7 由本分支实际升版；`AGENTS.md`/`CLAUDE.md` 只做「engine ABI 版本表述」一项的最小同步（两份逐字节相同），其余基线内容归集成任务；以 `TestBaselineVersionsMatchCode` 转绿所需的最小集合为准。用户选 A。 | `internal/nativeabi` 读 `C.MORNLEA_ENGINE_ABI_VERSION` 单处。 |
| 2026-08-25 | 0.5 approval | **Ruling:** bounded 短设计（契约重建 + Task 2–6 概要 + 验证命令 + 待集成路径）经飞书卡片获用户显式批准（`A-02-approval` approve @ 2026-08-25T10:39:28Z）。 | `~/.mornlea/confirm/A-02-approval.reply.json`：`approve`。 |
| 2026-08-25 | 1 契约产物 | 创建 `placeable-torches` 全套产物：proposal/design/tasks/ledger + 五份 delta specs；冻结互斥契约（火把方块 46..50、物品 37/39、mesh 19 bytes/48→64 entries/model offset 18、engine ABI v7、配方契约 19 号：煤炭在木棍上方 → 4 火把）与方向映射、支撑复核、属性唯一事实源、`torch-night` 场景契约（golden 归 A-07）。 | `openspec validate --all --strict --no-interactive`；`git diff --check`。 |

> 以上编号与职责切分基于旧基线（协议 v26、engine ABI v6、A-06/A-07 待办、配方归 A-01、golden 归 A-07），对应提交 `b2115a64` 已丢失；留档仅作决策脉络参考，不构成当前契约。

## 执行记录段（新基线）

| 日期 | 阶段 | 结论 | 验证 |
| --- | --- | --- | --- |
| 2026-08-27 | 0 重做裁决 | **Ruling（用户）:** ① 原 change 产物写于 A-01 合流前的旧基线，stash 留档后从当前 main 重做；A-06/A-07 已取消，职责回收各功能行。② A-02 自带 `torch-night` 场景 + golden（19 → 20 张），火把配方由 A-02 在 A-01 格子合成系统上追加（`RecipeTorch`=14）。 | 分支 `feat/A-02-placeable-torches` reset 至 origin/main=`cc385d22`；原产物以暂存区留档。 |
| 2026-08-27 | 0.5 契约修订 | 按 `cc385d22` 重写 proposal/design/tasks/ledger 与五份 delta：编号最终锁定（方块 62..66、`ItemTorch`=43、`RecipeTorch`=14）、engine ABI v7 → v8、mesh entry 19→20 bytes（model offset 19，`blockTopRaw` 保持 offset 18）、条目上限 64→80（67 > 64 所迫）、`torch-night` 自带 golden（场景表第 12 位，golden 19→20 张）。与指令给定的「19 bytes/offset 18/上限 64」冲突处按代码为准（`internal/mesh/native_input.go`、`engine/crates/mornlea_engine/src/input.rs`）。 | `openspec validate placeable-torches --strict --no-interactive`；`openspec validate --all --strict --no-interactive`。 |

### 版本矩阵（基线 `cc385d22`）

| 项 | 基线 | 本变更后 |
| --- | --- | --- |
| 协议 | v27 | v27（不变，无 wire 变更、无新命令） |
| 玩家 schema | v7 | v7 |
| 区块 schema | v9 | v9 |
| 世界 metadata | v2 | v2 |
| `companions.ai` schema | v4 | v4 |
| engine ABI | v7 | **v8**（registry entry 19→20 bytes） |
| client ABI | v9 | v9 |
| benchmark scenario | v19 | v19 |
| 方块编号 | 0..61（`BlockIDMax`=62） | 0..66（火把 62..66，`BlockIDMax`=67） |
| 物品编号 | 0..42（`ItemIDMax`=43） | 0..43（`ItemTorch`=43，`ItemIDMax`=44） |
| recipe 表 | 1..13 | 1..14（`RecipeTorch`=14） |
| mesh registry entry | 19 bytes / 上限 64 | 20 bytes / 上限 80 |
| capture 场景表 | 20 项（含 `workbench-crafting`） | 21 项（`torch-night` 第 12 位） |
| capture golden | 19 张 | 20 张（新增 `torch-night.png`） |

### 待跑基线命令清单

- `make rust`
- `go test ./internal/core ./internal/assets -race -count=1`
- `go test ./internal/sim ./internal/world -race -count=1`
- `go test ./internal/mesh ./internal/nativeabi -race -count=1`
- `go test ./internal/archcheck -count=1`

### 已知既有缺口（不属本变更，收尾上报）

- `workbench-crafting` 场景无 golden PNG：A-01 交付场景构造时把 golden 生成挂给已取消的批次集成任务；场景表 20 项与 golden 19 张的差即此。capture 非更新模式下该场景会因 golden 缺失失败，需对应功能行或独立基线任务补齐。
- 主规格滞后于代码：`openspec/specs/authoritative-crafting` 仍写 11 条配方与旧聚合语义（代码为 13 条形状配方）、`openspec/specs/visual-verification` 场景清单仍不含 `workbench-crafting`、`openspec/specs/rust-engine-mesh` 仍写「ABI version MUST 保持 1」（代码 v7）——A-01 归档未把这些 MODIFIED 合入主规格；本变更的 delta 按代码事实书写，归档时一并弥合所触及条目。

### Task 1 执行记录

- 实现 commit：`b77b4dde feat: register torch blocks, items and recipe`；评审修复 commit：`cf358dd4 fix: raise mesh registry entry cap to 80`；产物裁决 commit：`26c20c62`（design 方向表 Z 行笔误修正 + Task 2 边界补 physics 零碰撞）。
- RED：core/assets 双包 build failure（TorchStandingID、PlaceableBlockAtFace、ItemTorch、RecipeTorch 等未定义）。
- GREEN：core/assets race `ok 4.214s/4.057s`；archcheck `ok 4.335s`；QUALITY repair 后 core/assets/mesh/client 四包 race 全绿、cargo workspace 159+166 通过。
- 超清单同步（裁决接受）：`farming_test.go`/`block_name_test.go` 为既定枚举守护的最小机械同步；repair 中 `native_input_test.go` 容量夹具按每行 2 字联动。
- Ruling: registry 条目上限 64→80 提前至 Task 1 收口（QUALITY C1）——保留每提交点套件全绿，19→20 bytes 布局与 ABI v8 仍归 Task 3；Task 1 验证面扩为 core/assets/mesh/client 四包。
- SPEC 评审：PASS（无 Critical/Important；2 条 Minor 记录）。QUALITY 评审：初审 FAIL（C1 越界回归 + I1/M1/M2/M3），repair round 1 后复审 PASS（含线格式 count 推导逐链核对与跨 FFI 80 条容量钉子）。
