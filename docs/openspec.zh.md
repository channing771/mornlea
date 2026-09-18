---
doc_id: openspec-workflow
doc_revision: 2026-09-15.1
language: zh-CN
counterpart: openspec.md
---
# OpenSpec 工作流

Mornlea 使用 OpenSpec 定义变更范围、可观察需求、设计、实施任务和归档上下文。编码前先达成“做什么、如何验证”的共识，完成后将稳定行为沉淀到主规格。

## 初始化

仓库使用 `spec-driven` schema 和 core profile。项目上下文及产物规则在 `openspec/config.yaml`，项目规则在 `AGENTS.md`，CI 使用严格校验。

```bash
npm install -g @fission-ai/openspec@1.7.0
openspec --version
```

## 何时使用

新里程碑/系统、跨包功能或重构、协议/存档/并发/资源所有权变化、兼容性或性能边界变化，以及预计超过两天的工作都使用 OpenSpec。拼写、格式和一次性验证脚本可直接处理。

## 标准流程

需求或现状不清时先用 `explore`。proposal 必须说明背景、目标、非目标、可观察结果、影响范围以及兼容性/并发/性能影响；规格使用英文规范性的 `SHALL`/`MUST` 和可判定 Given/When/Then 场景；design 说明所有权、依赖方向、边界、风险、回滚、否决方案和验证；tasks 写明精确文件与命令并可独立验证。

1. 需求不清时用 `$openspec-explore` 探索代码、测试和历史。
2. 用 `$openspec-propose <change-name>` 生成聚焦的 proposal、delta specs、design 和 tasks。
3. 用 `$openspec-apply-change` 按 `tasks.md` 测试优先实施。默认主会话直接执行；经验证的 OpenAI ChatGPT/Codex 有委派授权。仅为上下文隔离委派，最多三个子代理；每次新委派先用实时 `adaptive-model-router` 选择最低够用模型/effort。非 OpenAI 或未知提供方严格使用 `subagent-driven-development` 并独立实现和评审。
4. 实现符合规格后运行：

```bash
openspec status --change <change-name>
openspec validate --all --strict --no-interactive
```

需要提前合并 delta 时使用 `$openspec-sync-specs`，完成后使用 `$openspec-archive-change`。每轮实现结束提升稳定架构发现，否则记录 `Architecture skill: no change`。

归档会将 delta 合并到 `openspec/specs/<capability>/spec.md`，并把完整 change 移到 `openspec/changes/archive/`；不得批量改写历史 change 或计划。

## 日常命令

```bash
openspec list
openspec list --specs
openspec status --change <change-name>
openspec show <change-name>
openspec validate --all --strict --no-interactive
openspec doctor
```

活跃及主规划产物使用英文；当前说明文档使用英文 `*.md` 和同步中文 `*.zh.md`。归档变更保留为历史证据。

## Hook 状态

Claude Code 与 Codex 的自动 Hooks 已移除。维护的实现和测试是 `scripts/agent-hooks/guard.mjs` 与 `node --test scripts/agent-hooks/guard.test.mjs`，不得描述为已安装的 Hook 配置。
