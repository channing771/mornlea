# first-person-hands Ledger

基线 SHA：`da148639`（分支 `feat/first-person-hands`，干净起步；worktree `.worktrees/first-person-hands`）。

## 内容确认（brainstorming architectural 路径）

- 分类：architectural（新渲染子系统 + client ABI 升版），已按重路径走。
- 用户澄清结论：双手同时呈现（左手空手占位 + 编码保留副手字段，不加协议/存档字段）；动作按工具微调（六档参数表，斧铲取镐默认占位，B-36 落地后只改表）。
- 用户已显式批准短设计（2026-09-06），批准结论已折入 proposal/design/brief。
- Ruling: 不在 `docs/feature-backlog.md` 自建新行 — 新行注册留给 planner（backlog 是 planner 的调度面，控制会话不抢）；独占文件集已在 design.md 声明。

## Change 产物

- `proposal.md` / `specs/first-person-viewmodel/spec.md`（ADDED×6）/ `specs/rust-client-render-cutover/spec.md`（ADDED×1）/ `design.md` / `tasks.md`（5 组）已建，待 `openspec validate --all --strict --no-interactive`。
