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
| `CLAUDE_BIN` / `CODEX_BIN` | PATH 查找 | 覆盖 CLI 路径 |
| `AGENT_MODE` | `merge` | 实现者收尾模式：`merge` 自动合入 main 并推送；`pr` 创建 PR 后暂停 |
| `AGENT_EXTRA_ARGS` | 空 | 透传给 agent CLI 的附加参数 |

## 日志与追溯

- 日志：`~/Library/Logs/mornlea-planner.log`（可配 `PLANNER_LOG`）。
- 每次运行把「本次变更行 ID 摘要」追加到 `docs/notes/agent-runs.md`（不存在则创建），便于回溯两处同步。
- 失败处理：`run-agent.sh` 返回非零时调度器不改状态，当天可手动补跑，不影响实现者。

## 常见问题

- **讨论与仓库不一致**：以 `docs/feature-backlog.md` 为准，改讨论正文。
- **版本号冲突**：认领前按 `docs/development-process.md`「版本号互斥」检查所有 `未认领` 行的契约影响列。
- **多个实现者同时跑**：不同行在独立 worktree 互不干扰；同一行只有一个认领人；冲突时后到者让位。
- **钩子拦截**：`.codex/hooks.json` 与 `.claude/settings.json` 共用 `scripts/agent-hooks/guard.mjs`，不得绕过；修复根因。
