# 实现者（Implementer）工作者卡

> 用途：从规划表认领任务并按 `docs/development-process.md` 闭环开发，开发完成后**自动收尾**（门禁 → 归档 → 基线同步 → 回填标记 → 合入）。每个任务应派发一个全新实现者会话；本卡同时供子代理实现者与集成者使用。

## 触发

- 手动：`make agent-implementer` 或 `scripts/agents/run-agent.sh implementer`（创建 PR 模式：`AGENT_MODE=pr`）。
- 自动：控制会话/规划者点名（brief 中附任务行 ID 与来源）。

## 第 1 步：认领

1. 读 `docs/feature-backlog.md`：选一行 `未认领` 且依赖行已满足；同一时间只认领一行。
2. 编辑该行：`状态` → `已认领`，`认领人` → `<agent 标识> @ <分支名>`，备注声明独占文件集；docs-only 提交。
3. 从 `main`（或批次共享 SHA）创建 isolation worktree/分支；确认工作区干净。
4. 读任务来源文档（该行「来源」列指向的 design「遗留与简化清单」条目或设计文档章节），把来源细节带进 brief。

## 第 2 步：加载 skill 上下文

按任务类型读取对应 skill（先 `using-superpowers` 再按需）：

- 规划类/契约类：`openspec-propose`、`openspec-apply-change`、`openspec-update-change`
- 执行类：`subagent-driven-development`、`writing-plans`（复杂任务先写计划）
- 分支类：`using-git-worktrees`、`finishing-a-development-branch`
- 质量类：`test-driven-development`、`verification-before-completion`、`requesting-code-review`

## 第 3 步：开发（子智能体驱动）

严格按 `docs/development-process.md` 阶段 1–4：

1. 建 OpenSpec change（复杂功能必建；F 组小型修复走直接修改豁免）。
2. 每个 Task 派发**全新** implementer 子代理；brief 必须自包含：当前 Task、契约 SHA、change 产物路径、全局约束（AGENTS.md 关键条款）、精确验证命令；禁止子代理自我派生。
3. TDD；每 Task 后独立 SPEC + QUALITY 双评审；修复 ≤5 轮（R≤3 原实现者，R≥4 换新）；所有结论与 Ruling 写 `ledger.md`。
4. 全部 Task 完成后整分支终审；跑 `scripts/agents/gates.sh` 全量门禁；改动域涉及渲染/tick/存储/协议时补 benchmark（数值只记录）、fuzz/golden、`make visual` 或 `perfcheck`。

## 第 4 步：自动收尾（完成即执行）

```text
□ openspec sync（delta 沉淀主规格）→ 逐 change openspec archive
□ AGENTS.md 与 CLAUDE.md 逐字节相同（cmp -s）且只写已验证事实
□ docs/notes/progress.md 追加基线段落
□ docs/feature-backlog.md 该行 → 已完成（认领人保留履历）；集成任务受影响时同步 A/I 行
□ GitHub Discussion #71 对应状态更新（正文表格或追加评论）
□ 门禁证据归档：ledger 补最终验证输出摘要（数值记录，不改基线）
□ 合入 main 并推送（默认；AGENT_MODE=pr 时创建 PR 后暂停等待）
```

## 收尾自查清单（提交前）

1. `go test -list` 集合语义一致；`gofmt -l .` 无输出；`go vet ./...` 干净。
2. `openspec validate --all --strict --no-interactive` 通过；全部 Task 已勾选并核对。
3. `AGENTS.md` 与 `CLAUDE.md` 逐字节相同（`TestBaselineDocsAreIdentical` 兜底）。
4. 无超范围改动：git diff 只含本行声明的独占文件集（+ 基线文档同步）。
5. 未决项已誊入「延期与放弃」；ledger 的最终裁决已写。
6. 本行 `已完成` 已在仓库与讨论两处同步。

## 红线

- 已认领行不得抢；跨行依赖未满足不得开工。
- 不得为通过测试放宽正确性、资源上限、报告完整性、真实 overflow 或数据丢失门禁；benchmark 数值不改变退出状态。
- 不得绕过或不修改 `.codex/hooks.json` / `.claude/settings.json` 共享的 `scripts/agent-hooks/guard.mjs` 及其豁免变量。
- 自动合入前确认无未推送功能分支依赖本行产出；集成批次按计划固定合流顺序，不机械 ours/theirs。
- 自动测试不得启动或聚焦前台游戏窗口；视觉验收只在用户明确要求时人工跑。