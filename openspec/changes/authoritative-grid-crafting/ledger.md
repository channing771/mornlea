# Ledger — authoritative-grid-crafting（A-01）

## 2026-08-25 认领与规划

- 18:31 `zcode-implementer` 认领 `docs/feature-backlog.md` A-01 行（main 提交 `540bbe17`），分支 `feat/A-01-authoritative-grid-crafting`，worktree `.worktrees/A-01-grid-crafting` 自 `main`（`16ac2fe7`）创建。
- 阶段 0.5 内容确认：分类 architectural（跨 core/sim/network/server/client/HUD 的批次基础功能），设计蓝本为批次设计文档与 `docs/superpowers/plans/2026-08-23-authoritative-grid-crafting.md`；用户于 2026-08-25 显式批准开工（「确认」）。
- 本 change 规划产物（proposal / 5 份 delta specs / design / tasks）在分支内创建，独立执行模式（crafting 计划 Task 1 的「独立执行本计划时才在本分支创建 Task 1 产物」分支）。

## Rulings

- Ruling: A-01 在无共享契约的重置批次下按 append-only 段独立执行 — 三条功能线（A-01/A-02/A-04）已各自从 `main` fork，任何一线强制他人 rebase 新契约 SHA 都违反「已认领行不得抢」；批次计划已规定合并序锁定终值 — 原共享契约提交随机器迁移丢失是根因，重置裁决「从当前 main 重做」已接受该事实。
- Ruling: in-branch 消息临时编号 C→S `MoveCraftingStack=14`、`TakeCraftingOutput=15`、S→C `CraftingState=21`，协议版本保持 v26 — 批次终值 `MoveCraftingStack=7` 依赖 `network.CraftRecipe` 删除（A-06）与 v27 升版（A-07），均为集成分支独占；分支内同构建客户端/服务端一致 — 不这么做会在功能分支触碰版本基线（集成任务独占文件集）。
- Ruling: 本分支只登记 recipe `1..13` 与 `ItemStick=37`/`ItemWorkbench=38`/`WorkbenchID=45` — recipe `14..18` 的产物（火把/剑/床）物品归 A-02/A-03/A-05 的 append-only 段，A-03 认领行已注明「recipe 15..17 依赖 A-01 合流」；backlog A-01 行的「七条新配方」描述批次终态，跨线实现 — 若在本分支预登记他人物品会在合并时产生重复编号冲突。
- Ruling: `CraftRecipe` 保留线上注册与 codec、ingress 稳定拒绝 — 类型删除与编号 7 释放归 A-06（backlog A-06 行明示「删除 network.CraftRecipe 过渡类型」）；保留注册让 fuzz/round-trip 在过渡期继续全覆盖。
- Ruling: `container-ui-presentation` 的「最大打开态 266 quad」改为「布局边界测试锁定精确值且 ≤267」 — 十条配方行被网格+产物格替换后精确最坏组合必然变化，功能分支不能改 `AGENTS.md` 基线表述（A-07 独占），精确值必须仍由测试锁定而非漂移。

## 基线证据

- 待补：首个 implementer 任务开始前在 worktree 运行 `make rust` 与 `go test ./internal/core ./internal/sim ./internal/network ./internal/render/hud ./cmd/mornlea -race -count=1`，命令与结果誊入本节。

## 评审与执行记录

- 待任务派发后逐条记录（implementer 提交、SPEC/QUALITY 结论、修复循环轮次）。
