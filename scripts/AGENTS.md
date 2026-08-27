# 脚本指南

## 开发契约

Hook、gate、发布和 agent 自动化脚本改变的是仓库开发契约，而不只是本地便利命令。修改前应核对调用方、运行环境、失败语义和现有文档。

失败时修复根因；不得删除步骤、吞掉错误、放宽真实 overflow 或数据丢失门禁，也不得使用无 spec 绕过变量规避 Hook。

## Hook 生命周期

`.codex/hooks.json` 与 `.claude/settings.json` 共用 `scripts/agent-hooks/guard.mjs`，但生命周期配置不同：两者都运行 `PreToolUse` 和 `PostToolUse`，当前只有 Codex 配置 `Stop`。修改共享实现时必须分别核对两套调用协议，不能假定生命周期相同。

## Gate 现状

`scripts/agents/gates.sh` 当前依次运行 gofmt 检查、`go vet ./...`、archcheck、OpenSpec strict 校验、`make rust`，并在未设置 `GATES_SKIP_RACE=1` 时运行全量 race。它不包含 `make rust-check`，文档和输出不得宣称已经执行该门禁。

修改 shell 脚本时运行对应的 focused shell check；修改 Node 脚本时运行对应的 focused Node check。本指南只记录现状，不修改任何脚本。
