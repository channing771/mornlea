# flood-destroys-crops Ledger

> 记录执行进度、评审结论与全部裁决（Ruling）。格式：`Ruling: <决定什么> — <为什么> — <错在哪>`。

## 认领与确认（阶段 0–1）

- 2026-08-25：B-07 由 ox-alpha-implementer 认领（backlog 提交 `755b2228`），分支 `feat/B-07-flood-destroys-crops`。
- 内容确认：bounded 路径，短设计经需求方显式批准。裁决记录：
  - `Ruling: 掉落槽满时拒绝破坏并重试 — 对齐「背包无空位不破坏」纪律与数据丢失门禁 — 否决照常破坏丢产物与有界重试（上限无依据、单格重试成本有界无害）`

## 阶段 2

- worktree `.worktrees/feat/B-07-flood-destroys-crops` 自 main `bcc900fb` 创建；基线 `make rust` + `go test ./internal/fluid ./internal/sim -short -count=1` 全绿。
- change 四产物 + 本 ledger 已建；独占文件集见 backlog B-07 行（不触碰 A-01/A-04/B-10 独占文件）。

## 执行（阶段 3）

（待记录）

## 终审与收尾（阶段 4–5）

（待记录）
