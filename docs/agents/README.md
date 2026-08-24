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

# 标准门禁汇总（实现者收尾前必跑）
scripts/agents/gates.sh
```

## 每天固定时间：cron

```cron
# 每天 09:00 运行规划者（改第一行即可调时间）
0 9 * * * /bin/bash -lc '/Users/chen/chenwork/minecraft-go/scripts/agents/run-agent.sh planner >> /Users/chen/Library/Logs/mornlea-planner.log 2>&1'
```

安装：`crontab -e` 粘贴上行；或 `scripts/agents/install-cron.sh planner` 自动写入 crontab。

## 每天固定时间：launchd（macOS 推荐）

```bash
scripts/agents/install-launchd.sh   # 生成 ~/Library/LaunchAgents/com.mornlea.planner.plist 并 load
```

生成的 plist 等价于（Hour/Minute 即固定时间）：

```xml
<plist version="1.0">
  <dict>
    <key>Label</key><string>com.mornlea.planner</string>
    <key>ProgramArguments</key>
    <array>
      <string>/bin/bash</string>
      <string>-lc</string>
      <string>/Users/chen/chenwork/minecraft-go/scripts/agents/run-agent.sh planner</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict><key>Hour</key><integer>9</integer><key>Minute</key><integer>0</integer></dict>
    <key>StandardOutPath</key><string>/Users/chen/Library/Logs/mornlea-planner.log</string>
    <key>StandardErrorPath</key><string>/Users/chen/Library/Logs/mornlea-planner.err.log</string>
  </dict>
</plist>
```

## 环境变量

| 变量 | 默认 | 作用 |
|---|---|---|
| `AGENT_TOOL` | `claude` | 用 `claude` 或 `codex` 执行 |
| `AGENT_MODEL` | CLI 各自默认 | 固定模型，未设置时用 CLI 配置（claude：`~/.claude/settings.json` 的 `model`；codex：`~/.codex/config.toml` 的 `model`）|
| `CLAUDE_BIN` / `CODEX_BIN` | PATH 查找 | 覆盖 CLI 路径 |
| `AGENT_MODE` | `pr` | 实现者收尾模式（默认）：`pr` = 创建 PR → 监听 CI 至全绿（失败自动修复重推）→ merge 到 main；`merge` = 本地全绿后直接合并推送 |
| `AGENT_INTERACTIVE` | `0` | `1` 时 claude 以终端交互模式运行（可用 `ask_user_question` 直接问你；仅 claude 支持）|
| `AGENT_CONFIRM_CHANNEL` | `auto` | 内容确认通道：`feishu`（推送设备）/ `discussion`（GitHub 评论）/ `none`（本地记录）/ `auto`（有飞书配置则 feishu，否则 none）|
| `MORNLEA_CONFIRM_DIR` | `~/.mornlea/confirm` | 确认请求/回复/飞书配置文件目录 |
| `AGENT_EXTRA_ARGS` | 空 | 透传给 agent CLI 的附加参数（如 claude 的 `--permission-mode acceptEdits`）|

## 日志与追溯

- 日志：`~/Library/Logs/mornlea-planner.log`（可配 `PLANNER_LOG`）。
- 每次运行把「本次变更行 ID 摘要」追加到 `docs/notes/agent-runs.md`（不存在则创建），便于回溯两处同步。
- 失败处理：`run-agent.sh` 返回非零时调度器不改状态，当天可手动补跑，不影响实现者。

## 常见问题

- **讨论与仓库不一致**：以 `docs/feature-backlog.md` 为准，改讨论正文。
- **模型如何选择**：不设 `AGENT_MODEL` 时用各 CLI 配置的默认模型（本机：claude → `claude-fable-5[1m]`；codex → `gpt-5.6-sol`）。需要更高能力或更快模型时用 `AGENT_MODEL=<模型名> make agent-planner` 覆盖；模型名称以对应 CLI 支持列表为准。
- **brainstorm 需要用户选择怎么办**：**设备优先**——实现者 `confirm.sh ask` 把内容确认请求推送到你的飞书，回复「✅ 批准」或修改意见后，常驻 `feishu-listener.js` 写回复文件并自动续跑任务（`AGENT_RESUME`）；通道未配置/发送失败/超时则降级为 GitHub Discussion #71 评论协议（发评论 + 停止等待，回复后 `confirm.sh reply` 恢复）。完整机制与飞书配置（约 10 分钟）见 `docs/agents/confirmation-channel.md`。想终端即时问答：`AGENT_INTERACTIVE=1 scripts/agents/run-agent.sh implementer`。
- **版本号冲突**：认领前按 `docs/development-process.md`「版本号互斥」检查所有 `未认领` 行的契约影响列。
- **多个实现者同时跑**：不同行在独立 worktree 互不干扰；同一行只有一个认领人；冲突时后到者让位。
- **钩子拦截**：`.codex/hooks.json` 与 `.claude/settings.json` 共用 `scripts/agent-hooks/guard.mjs`，不得绕过；修复根因。
