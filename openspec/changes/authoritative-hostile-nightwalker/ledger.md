# Ledger：authoritative-hostile-nightwalker（A-04）

> 记录：控制会话裁决（ruling）、每 Task 评审结论与修复循环、最终验证输出摘要。

## 内容确认记录（brainstorming 硬门禁，2026-08-25）

- **分类**：architectural（新实体子系统 + 新存档 schema + 新协议消息 + 客户端 ABI 升版 + Rust 呈现改动）。
- **探索**：批次设计 `docs/superpowers/specs/2026-08-23-first-night-survival-parallel-wave-design.md` 任务四、计划 `docs/superpowers/plans/2026-08-23-authoritative-hostile-nightwalker.md`；全仓现状核对（Explore 子代理报告）：无物理批处理 API（per-actor `physics.Step`）、发光/衰减表在 `assets`/`mesh` 且 `sim` 不可导入、`splitmix64` 在 `internal/sim/crop.go`、avatar `maxAvatars=11`（66 实例，每帧 GPU 上限）、client ABI v8、协议 v26、`WorldTimeTicks` 无偏移、后端存储原语/CRC/原子写先例齐全。
- **Ruling（控制会话既有裁决，经 A-02-q1/q2 卡片取得并采用）**：批次各分支自建 PR 不合并（待集成），A-06 按固定顺序合流；分支可对两份基线文档做最小同步（只改本人负责 ABI 的版本行、两份逐字节一致），其余基线归集成。
- **Ruling: A-04-q1（2026-08-25T10:45:00Z，approve）** — client ABI 本分支直接升 v9（Rust/Go 常量与容量拒绝同步），并按 A-02-q2 先例对两份基线仅同步 client ABI 版本行 — 为什么：批次设计要求「旧动态库不得在运行到敌怪帧后才迟发拒绝」，主基线已 v8（egui 占用）故实际新值 v9；唯一例外的合理载体是实际改动的分支。与 A-07 的版本基线独占不冲突（A-07 只补其余行）。
- **Ruling: A-04-q2（2026-08-25T10:52:33Z，answer A）** — 本分支在 `internal/core` 引入 `BlockEmission`/`BlockLightAttenuation` 单一表（按现有 assets/mesh 值迁移，二者改为委托 core；若 A-02 契约先行则消费其表不重复创建） — 为什么：`sim` 只能依赖 `core`/`companion`/`fluid`/`physics`/`world`，暗度判定规则必须落 core；批次设计任务二把 `core.BlockEmission` 单一表归 A-02，故本分支仅在 A-02 未落地时创建并保持值一致。
- **批准轮 A-04-approval（2026-08-25T10:54:16Z，approve「批准」）** — 按节呈现的设计（§1 范围 / §2 数据所有权 / §3 关键裁决 / §4 固定上限 / §5 验证 / §6 不做）经用户显式批准；结论已誊入本 change 的 proposal/design 与 tasks。

## 重定基线裁决（2026-08-28，控制会话 brainstorming）

- **Ruling: A-04-rebase-1（approve）** — 批次合流模式正式弃用（原 A-06/A-07 集成职责已拆回各功能行并标记取消）：本行改为自包含直接合并，协议 v29→v30、client ABI v9→v10、`hostile_mobs` v1、golden（21→22 张口径）与两份基线文档版本行由本行自带同步；benchmark scenario 不动。为什么：批次模式的「PR 不合并、版本不动、golden 延后」前提随 A-06/A-07 取消失效，现行约定以 A-02（协议 v29 内自带 engine ABI v8、torch-night 场景与 golden）为先例。
- **Ruling: A-04-rebase-2（approve）** — 消息编号改取 S→C 22/23/24（21 已被 A-01 `CraftingState` 实占；原设计预留 21/22/23 已撞号）；实现期以注册表实占空闲位为准，与并行行撞号由后合并者重订（A-02 撞号重订先例）。
- **Ruling: A-04-rebase-3（approve）** — `core.BlockEmission`/`core.BlockLightAttenuation` 已由现行 `internal/core/block_properties.go` 提供（A-02 落地）：按原 D2 预设判据走「直接消费、不重复创建」路径，本行只新增 `core.BlockOpaque` 单一表并把 assets/mesh 不透明谓词改为委托。
- **Ruling: A-04-rebase-4（approve）** — 与并行行 A-05（床与睡眠）解耦互动：睡眠不查询夜行者，跳夜后白昼灼烧规则自然生效；两行唯一共享契约为 `core.DisplayDayPhase(ticks, offset)`（本行交付并消费、offset 恒 0，A-05 后续提供 offset 生产端）。战斗 seam 保留，待 A-03 统一战斗落地后收编删除。
- **分支操作**：分支 `feat/A-04-hostile-nightwalker` 已 rebase 到 `origin/main`（`fe3890ed`），原两提交（proposal 23df0525 + rulings c96a6851）重放为 69e5c1f0 + 717cd3e7；重定基线文档修订以新提交追加。

## 变更产物

- [x] `openspec new change authoritative-hostile-nightwalker`；proposal/7 delta specs/design/tasks/ledger 已建。
- Ruling: 本分支产物提交到功能分支（本 worktree）而非 main — 为什么：控制会话裁决（A-02-q1 路径 A）批次分支自包含；claims 类 docs-only 提交才上 main。

## 评审记录（Task 1 起，逐 Task 追加）

- （待逐 Task 填：SPEC 合规结论 / QUALITY 结论 / 修复轮 R1..Rn / 对应 Ruling）

## 最终验证输出摘要（收尾补）

- （待整分支终审后补：make rust、focused -race、archcheck、vet、gofmt、openspec strict 的数值摘要；benchmark 数值只记录）
