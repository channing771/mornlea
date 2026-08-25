# Ledger: crop-random-drop-count

> 记录本 change 的进度、评审结论与全部裁决（Ruling）。格式：`Ruling: <决定什么> — <为什么> — <错在哪>`。

## 裁决记录（认领与设计阶段，2026-08-25）

- **Ruling: B-01 拆分，本 change 只承接其作物半边的姊妹行 B-10** — 用户（控制需求方）裁决：B-01 的肉类与熟食熔炉食谱并入 B-27（被动生物）随其落地交付；作物半边因与第一批次 A-02 独占文件集（`internal/assets`、`internal/mesh`、engine crate）重叠且 mesh registry 容量仅剩 3/48 条与批次编号契约耦合，按积压表「换行或延迟」规则延迟。原选 B-01 全量的路径违反「依赖行已满足才可认领」（肉类依赖未开工的 B-27）。已回写 `docs/feature-backlog.md` B-01/B-10 两行（提交 `6a06dd12`）。
- **Ruling: 认领 B-10 并冻结独占文件集** — `internal/sim/mining.go`、`internal/sim/crop.go`、同包 `*_test.go` 与本 change 产物；刻意不触碰 `tunables.go`/`drop.go`/`hunger.go` 与 `internal/core` 编号段（A-01/A-04 已认领）。核实这些文件不在任何已认领行的独占集内。
- **Ruling: server 测试文件与 E-11 的边界** — 本 change 需改 `internal/server/farming_loop_e2e_test.go` 的收获数量断言，该文件在 E-11 独占的 `internal/server/*_test.go` 内；按 A-04 行先例记录「仅改行为断言、不碰等待助手/helper，关注点不相交」，合并序如遇 E-11 重排由集成裁决。
- **Ruling: 掉落数量范围采用「小麦 1–3、种子 1–3，两次独立抽取」** — 用户在三个候选（小麦固定 1 / 组合表四选一）中选定；经济均值从每株 3 件升到 4 件被显式接受。
- **Ruling: 数量范围不进 tunable** — 固定常量表先例（饥饿疲劳表）；同时避开 A-04 独占的 `tunables.go`。
- **Ruling: 设计批准** — bounded 短设计（阶段 0.5）经用户显式批准后开工。

## 工具链事件

- 机器迁移丢失 node/npm 与全局 `openspec` CLI；经用户确认后经 fnm 提供的 node v24.19.0 重装钉死版本 `@fission-ai/openspec@1.7.0`（docs/openspec.md:18）。

## Task 1

（待实现派发后填写）

## Task 2

（待实现派发后填写）

## Task 3 / 整分支终审

（待收尾填写）
