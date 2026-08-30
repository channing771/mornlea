# 工作者调度（Planner）

本目录是常驻规划者的角色卡：

| 工作者 | 角色卡 | 职责 | 调度 |
|---|---|---|---|
| 规划者 Planner | `docs/agents/planner.md`（角色卡）/ `docs/agents/planner-prompt.md`（实际投喂的提示词）| 每日固定时间读取规划进度与 MC 缺口请求，扩展/校对 `docs/feature-backlog.md` 与 Discussion #71 | 每天固定时间（默认 09:00）|

开发流程的唯一说明在 `docs/development-process.md`，工作者卡与 `docs/feature-backlog.md` 均只引用它。

规划表只允许 `就绪|已认领|开发中|待集成|排队|设计候选|已完成|已取消` 八种状态。

## 前置条件

- `claude`（`~/.local/bin/claude`）或 `codex`（homebrew）任一已安装并登录。
- 仓库已启用 GitHub Discussions（`channing771/mornlea`）且有本机 `gh auth login`。
- 调度机器为 macOS 时推荐 launchd；Linux 用 cron。

## 运行入口

```bash
# 手动运行（默认 claude，可用 AGENT_TOOL=codex / CLAUDE_BIN=... 覆盖）
./scripts/agents/run-agent.sh planner

# 或通过 Makefile
make agent-planner

# 完整提交前门禁（两项都必须通过）
scripts/agents/gates.sh
make rust-check
```

`scripts/agents/gates.sh` 汇总 gofmt、vet、archcheck、OpenSpec、`make rust` 和未跳过时的 full race；它不包含 `make rust-check`，两者都通过才构成完整提交前门禁。

## 执行状态看板（可选）

这是一个只读的本地开发环境状态看板，展示当前哪些 AI（planner 工作者）正在执行及其执行状态。Go 后端默认监听 `http://127.0.0.1:8787`，React 前端位于 `web/agent-board/`。

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

## 开发捕获服务（可选）

客户端以 `--dev-capture` 启动时会内嵌一个仅绑定回环地址的本地 HTTP 捕获服务，供 agent 观察运行中的游戏画面（世界 + HUD + WebView 菜单层）：直接 curl 拉取 PNG 截图或短录屏帧序列，无需人工截图。服务默认关闭；无头的 `--benchmark`/`--capture` 路径不可用。

```bash
# 启动交互式客户端并打开捕获服务（可与 --connect 组合；实际地址打印到 stdout）
go run ./cmd/mornlea --dev-capture

# 服务实际端口优先读发现文件（字段 pid/port/started_at，进程退出时删除）
cat ~/.mornlea/dev-capture.json

# 截图
curl -s -o /tmp/mornlea-shot.png http://127.0.0.1:17790/screenshot

# 录制 2s @ 8fps 并解包逐帧查看
curl -s -o /tmp/mornlea-rec.zip 'http://127.0.0.1:17790/record?seconds=2&fps=8'
unzip -d /tmp/mornlea-rec /tmp/mornlea-rec.zip
```

- 首次捕获可能触发 macOS「屏幕录制」授权弹窗；画面不含游戏窗口时检查系统设置的屏幕录制授权并重试。
- 端点契约、录制参数上限与失败语义见 `docs/notes/dev-capture.md`。

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
| `AGENT_EXTRA_ARGS` | 空 | 透传给 agent CLI 的附加参数 |
| `AGENT_SAFE` | `0` | 权限模式：`0`（默认）= 最高权限（claude `--dangerously-skip-permissions`；codex `--dangerously-bypass-approvals-and-sandbox`，经用户明确要求）；`1` = 受限（claude 逐项批准 / codex `--approve-for-me`）。仓库 hooks 始终独立生效 |

## 日志与追溯

- 日志：`~/Library/Logs/mornlea-planner.log`（可配 `PLANNER_LOG`）。
- 每次运行把「本次变更行 ID 摘要」追加到 `docs/notes/agent-runs.md`（不存在则创建），便于回溯两处同步。
- 失败处理：`run-agent.sh` 返回非零时调度器不改状态，当天可手动补跑。

## 常见问题

- **讨论与仓库不一致**：以 `docs/feature-backlog.md` 为准，改讨论正文。
- **模型如何选择**：不设 `AGENT_MODEL` 时用各 CLI 配置的默认模型（本机：claude → `claude-fable-5[1m]`；codex → `gpt-5.6-sol`）。需要更高能力或更快模型时用 `AGENT_MODEL=<模型名> make agent-planner` 覆盖；模型名称以对应 CLI 支持列表为准。
- **版本号冲突**：认领前按 `docs/development-process.md`「版本号互斥」检查所有 `就绪` 行的契约影响列。
