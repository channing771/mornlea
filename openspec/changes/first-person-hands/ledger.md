# first-person-hands Ledger

基线 SHA：`da148639`（分支 `feat/first-person-hands`，干净起步；worktree `.worktrees/first-person-hands`）。

## 内容确认（brainstorming architectural 路径）

- 分类：architectural（新渲染子系统 + client ABI 升版），已按重路径走。
- 用户澄清结论：双手同时呈现（左手空手占位 + 编码保留副手字段，不加协议/存档字段）；动作按工具微调（六档参数表，斧铲取镐默认占位，B-36 落地后只改表）。
- 用户已显式批准短设计（2026-09-06），批准结论已折入 proposal/design/brief。
- Ruling: 不在 `docs/feature-backlog.md` 自建新行 — 新行注册留给 planner（backlog 是 planner 的调度面，控制会话不抢）；独占文件集已在 design.md 声明。

## Change 产物

- `proposal.md` / `specs/first-person-viewmodel/spec.md`（ADDED×6）/ `specs/rust-client-render-cutover/spec.md`（ADDED×1）/ `design.md` / `tasks.md`（5 组）已建，待 `openspec validate --all --strict --no-interactive`。

## Task 1：Go viewmodel 编码（子代理开发轮 + 审查轮）

- Red→Green：`4f4ca8dc`，`go test ./packages/client/render -race -count=1` 全绿。
- 任务评审 Spec ❌：I-1（tick 回退不清 `lastAttackTick` 致新会话首挥丢失）/ I-2（确认切换无显式测试）/ I-3（缺 `EncodeRenderFrame` 字节一致回归）+ M-1..M-4。
- 修复轮 1/5：`10901e37` 全闭合，scoped re-review 全 ADDRESSED，无新 breakage。
- Task 1: complete (commits 4a5adffd..10901e37, review clean)

## Task 2：跨语言 client ABI v17 同步（子代理开发轮 + 审查轮）

- Green：`6c88a315`，header/Rust/Go 三端 17 + TLV tag 11 + 解码 + 三端一致/v16 拒绝测试；client race、`cargo test -p mornlea_client`（viewmodel 6/6，全 crate 197/197）、render 全绿。
- 任务评审 Spec ✅；Quality 有条件 ✅：Important-1（Task 1 遗留两处注释反引号致 audit 红）+ Minor-1（ffi 白名单注释过期）。
- Ruling: audit 全绿是硬门禁，纯注释修复零语义变化，不违反文件所有权——允许 fix loop 修 Task 1 注释行。
- 修复轮 1/5：`d60ffa6d`，re-review 全 ADDRESSED；audit/render/client/cargo 全绿。
- Task 2: complete (commits 10901e37..d60ffa6d, review clean)
