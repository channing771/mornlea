# 工作者调度（Planner / Implementer）

本目录是两个常驻工作者的角色卡：

| 工作者 | 角色卡 | 职责 | 调度 |
|---|---|---|---|
| 规划者 Planner | `docs/agents/planner.md`（角色卡）/ `docs/agents/planner-prompt.md`（实际投喂的提示词）| 每日固定时间读取规划进度与 MC 缺口请求，扩展/校对 `docs/feature-backlog.md` 与 Discussion #71 | 每天固定时间（默认 09:00）|
| 实现者 Implementer | `docs/agents/implementer.md` | 从规划认领任务，按 `docs/development-process.md` 开发并自动收尾 | 手动触发或规划者/控制会话点名 |

开发流程的唯一说明在 `docs/development-process.md`，两个工作者卡与 `docs/feature-backlog.md` 均只引用它。

## 前置条件

- `claude`（`~/.local/bin/claude`）或 `codex`（homebrew）任一已安装并登录。
- 仓库已启用 GitHub Discussions（`channing771/mornlea`）且有本机 `gh auth login`。
- 调度机器为 macOS 时推荐 launchd；Linux 用 cron。

## 运行入口

```bash
# 手动运行（默认 claude，可用 AGENT_TOOL=codex / CLAUDE_BIN=... 覆盖）
./scripts/agents/run-agent.sh planner
./scripts/agents/run-agent.sh implementer

# 或通过 Makefile
make agent-planner
make agent-implementer

# 完整提交前门禁（两项都必须通过）
scripts/agents/gates.sh
make rust-check
```

`scripts/agents/gates.sh` 汇总 gofmt、vet、archcheck、OpenSpec、`make rust` 和未跳过时的 full race；它不包含 `make rust-check`，两者都通过才构成完整提交前门禁。

## 执行状态看板（可选）

这是一个只读的本地开发环境状态看板，展示当前哪些 AI（planner/implementer 工作者）正在执行及其执行状态。Go 后端默认监听 `http://127.0.0.1:8787`，React 前端位于 `web/agent-board/`。

```bash
# 安装锁定依赖、构建前端并启动 Go 后端（默认地址 127.0.0.1:8787）
make agent-dashboard

# 只启动 Vite 开发服务器（127.0.0.1:5173，/api 代理到 8787）
# 需另开终端运行已构建的 agent-dashboard 或 Go 后端
make agent-ui-dev
```

- 前端改动流程：在 `web/agent-board/` 修改 TypeScript/Tailwind/shadcn/ui 源码，运行 `npm --prefix web/agent-board test` 与 `npm --prefix web/agent-board run build`；`package-lock.json` 入库，`node_modules/` 与 `dist/` 不入库。
- Go 后端从 `web/agent-board/dist/` 读盘提供 `/` 与 `/assets/*`。若前端尚未构建，`/` 返回指引页并提示运行 `make agent-dashboard`，`/api/status` 仍可用。
- 地址覆盖：`BOARD_ADDR` 提供默认值、flag `--addr` 再覆盖，例如 `BOARD_ADDR=:9000 go run ./cmd/mornlea-agent-board` 或 `go run ./cmd/mornlea-agent-board --addr 127.0.0.1:9000`。
- 数据范围：全部为本机采集（`ps`/`git`/`gh`/日志文件），无远程依赖；刷新间隔固定 5 秒。
- gh 未登录或不可用时，PR 区自动降级为说明文字，不影响其它小节；该看板不会启动或影响任何 agent。

## 每天固定时间：cron

```bash
scripts/agents/install-cron.sh planner
crontab -l
```

`scripts/agents/install-cron.sh planner` 是 canonical 安装入口；脚本自动解析当前仓库根并把绝对路径写入 crontab。默认每天 09:00，可用 `CRON_HOUR` / `CRON_MIN` 覆盖；安装后用 `crontab -l` 检查结果。

## 每天固定时间：launchd（macOS 推荐）

```bash
scripts/agents/install-launchd.sh
```

`scripts/agents/install-launchd.sh` 是 canonical 安装入口；脚本自动解析当前仓库根、写入绝对路径并生成和加载 `~/Library/LaunchAgents/com.mornlea.planner.plist`。默认每天 09:00，可用 `PLANNER_AT_HOUR` / `PLANNER_AT_MINUTE` 覆盖；安装后直接检查生成的 plist。

## 环境变量

| 变量 | 默认 | 作用 |
|---|---|---|
| `AGENT_TOOL` | `claude` | 用 `claude` 或 `codex` 执行 |
| `AGENT_MODEL` | CLI 各自默认 | 固定模型，未设置时用 CLI 配置（claude：`~/.claude/settings.json` 的 `model`；codex：`~/.codex/config.toml` 的 `model`）|
| `CLAUDE_BIN` / `CODEX_BIN` | PATH 查找 | 覆盖 CLI 路径 |
| `AGENT_MODE` | `pr` | 实现者收尾模式（默认）：`pr` = 创建 PR → 监听 CI 至全绿（失败自动修复重推）→ merge 到 main；`merge` = 本地全绿后直接合并推送 |
| `AGENT_LOOP` | `0` | 实现者接力循环：`1` 时每个实现者完成即经 `scripts/agents/relay.sh` 自动启动下一个，直到规划表无「未认领」任务 |
| `WORKER_ID` | 空 | 链身份（如 `codex`）。空=主链（`loop.guard`）；非空=独立链（`loop.guard.<WORKER_ID>`），多工作者并行互不排斥 |
| `WORKER_TOOL` | `claude` | relay 接力保持的工具（与启动时 `AGENT_TOOL` 一致，如 `codex`）|
| `AGENT_INTERACTIVE` | `0` | `1` 时 claude 以终端交互模式运行（可用 `ask_user_question` 直接问你；仅 claude 支持）|
| `AGENT_CONFIRM_CHANNEL` | `auto` | 内容确认通道：`feishu`（推送设备）/ `discussion`（GitHub 评论）/ `none`（本地记录）/ `auto`（有飞书配置则 feishu，否则 none）|
| `MORNLEA_CONFIRM_DIR` | `~/.mornlea/confirm` | 确认请求/回复/飞书配置文件目录 |
| `AGENT_EXTRA_ARGS` | 空 | 透传给 agent CLI 的附加参数 |
| `AGENT_SAFE` | `0` | 权限模式：`0`（默认）= 最高权限（claude `--dangerously-skip-permissions`；codex `--dangerously-bypass-approvals-and-sandbox`，经用户明确要求）；`1` = 受限（claude 逐项批准 / codex `--approve-for-me`）。仓库 hooks 始终独立生效 |

## 日志与追溯

- 日志：`~/Library/Logs/mornlea-planner.log`（可配 `PLANNER_LOG`）。
- 每次运行把「本次变更行 ID 摘要」追加到 `docs/notes/agent-runs.md`（不存在则创建），便于回溯两处同步。
- 失败处理：`run-agent.sh` 返回非零时调度器不改状态，当天可手动补跑，不影响实现者。

## 常见问题

- **讨论与仓库不一致**：以 `docs/feature-backlog.md` 为准，改讨论正文。
- **模型如何选择**：不设 `AGENT_MODEL` 时用各 CLI 配置的默认模型（本机：claude → `claude-fable-5[1m]`；codex → `gpt-5.6-sol`）。需要更高能力或更快模型时用 `AGENT_MODEL=<模型名> make agent-planner` 覆盖；模型名称以对应 CLI 支持列表为准。
- **brainstorm 需要用户选择怎么办**：**设备优先**——实现者 `confirm.sh ask` 把内容确认请求推送到你的飞书，回复「✅ 批准」或修改意见后，常驻 `feishu-listener.js` 写回复文件并自动续跑任务（`AGENT_RESUME`）；通道未配置/发送失败/超时则降级为 GitHub Discussion #71 评论协议（发评论 + 停止等待，回复后 `confirm.sh reply` 恢复）。完整机制与飞书配置（约 10 分钟）见 `docs/agents/confirmation-channel.md`。想终端即时问答：`AGENT_INTERACTIVE=1 scripts/agents/run-agent.sh implementer`。
- **如何让实现者循环领取任务**：`AGENT_LOOP=1 make agent-implementer`（或首次 `scripts/agents/relay.sh`）——实现者完成后自动接力下一个，循环直到无未认领任务；每个任务仍要经过 brainstorm 确认（推你的飞书）。
- **多工作者并行（如 claude + codex 各一条链）**：第二条链用 `WORKER_ID=codex AGENT_TOOL=codex AGENT_LOOP=1 scripts/agents/run-agent.sh implementer` 启动——`WORKER_ID` 唯一即独立守卫，两条链互不排斥；接力时用 `WORKER_TOOL` 保持各自工具。**认领安全**：两链可能同时选中同一行——以仓库 git 提交先后为准，被拒/冲突的一方让位并改选其它行。
- **版本号冲突**：认领前按 `docs/development-process.md`「版本号互斥」检查所有 `未认领` 行的契约影响列。
- **多个实现者同时跑**：不同行在独立 worktree 互不干扰；同一行只有一个认领人；冲突时后到者让位。
- **钩子拦截**：`.codex/hooks.json` 与 `.claude/settings.json` 共用 `scripts/agent-hooks/guard.mjs`，两者都配置 `PreToolUse` 与 `PostToolUse`，当前只有 Codex 配置 `Stop`；不得绕过，失败时修复根因。
