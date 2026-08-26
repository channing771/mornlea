# 新机器落地指南（代理工作链套装）

> 把「规划者＋实现者继电器＋飞书确认通道」整套机制搬到另一台电脑的完整步骤。机制设计为**仓库自带、机器自描述**：所有路径均按 `$HOME`/脚本位置动态解析，仓库内没有任何本机绝对路径硬编码；新机器只需：clone → 装依赖 → 生成本地配置 → 填飞书凭据 → 登录 CLI。

## 0. 一句话结论

git clone 后运行 `scripts/agents/confirm/install-listener.sh`（macOS）即可生成全部本地配置骨架；**唯一必须在新机器人工配置的是飞书应用凭据与 claude/codex/gh 的登录态**。不要把 `~/.mornlea/` 整体拷贝过去（详见第 5 节）。

## 1. 随仓库走 vs 本机私有

### 1.1 随仓库走（clone 即得，零修改）

| 项 | 说明 |
|---|---|
| 全部工作者脚本 | `scripts/agents/`：`run-agent.sh`、`relay.sh`、`pr-finalize.sh`、`gates.sh`、`refresh-discussion.py`、`install-*.sh` 与 `scripts/agents/confirm/`（`confirm.sh`、`feishu.sh`、`feishu-listener.js`） |
| 确认通道依赖 | `scripts/agents/confirm/package.json` + **`package-lock.json`**（`npm ci` 可复现 `@larksuiteoapi/node-sdk`）；`node_modules` 被 gitignore，**不随库走** |
| 仓库 hooks | `.codex/hooks.json` / `.claude/settings.json` → `scripts/agent-hooks/guard.mjs`（已入库，跨机一致生效） |
| 文档与规划表 | `docs/feature-backlog.md`、`docs/development-process.md`、`docs/agents/*`、本文件 |
| CI | `.github/workflows/ci.yml` 在 GitHub 侧执行，与机器无关 |
| 仓库身份 | git remote、Discussion #71 id（`D_kwDOToJS8M4Aou6G`，硬编码在 `refresh-discussion.py` 与评论模板）——clone 同一个仓库即有效 |

### 1.2 本机私有（重新生成，不要搬运）

| 内容 | 迁移方式 |
|---|---|
| **`~/.mornlea/confirm/feishu.json`**（appId / appSecret / receive.open_id / autoResume / resumeCmd） | 唯一需要迁移的配置。**复用同一个飞书应用**：拷贝 appId/appSecret 即可（open_id 是「应用×用户」维度，同应用同用户不变）；**新建应用**：填凭据后跑一次 `--bootstrap` 重捕 open_id |
| `~/.mornlea/confirm/feishu-token.json`（token 缓存） | 可删，自动重新获取 |
| `~/.mornlea/loop.guard` / `loop.guard.<WORKER_ID>`（链守卫 pid） | **别拷贝**——新机上全是死 pid；实现者会话启动时会识别存活 pid 并重新登记 |
| `~/.mornlea/confirm/*.json` / `*.reply.json` / `resume-*.log`（历史请求/回复/续跑日志） | **别拷贝**——listener 会把旧 pending 请求当作待答问题，匹配与续跑都会混乱；从零开始 |
| `~/Library/LaunchAgents/com.mornlea.*.plist` | 由 `install-listener.sh` / `install-launchd.sh` 按新机真实路径（node、仓库、日志）**重新生成**，绝不拷贝 |
| CLI 登录态 | `gh auth`、`~/.claude`、`~/.codex` 每台机器各自登录；claude/codex 二进制另行安装 |
| 定时调度 | macOS 用 launchd（`install-launchd.sh`）；Linux 用 `install-cron.sh`（通用）；Listener 常驻 Linux 用 systemd/cron 兜底（见第 4 节） |

## 2. 前置依赖

| 用途 | 依赖 | 本机实测 |
|---|---|---|
| 所有机器 | git、node ≥ 18、npm、jq、python3、curl（系统自带）、gh CLI | node v26.5.0 / npm 11.17.0 / macOS 26（Apple Silicon） |
| 跑实现者链（额外） | claude CLI（`~/.local/bin` 或 PATH）＋登录；codex CLI ＋登录；Go/Rust 工具链（`gates.sh` 用） | claude/codex 登录态在 `~/.claude`/`~/.codex`（独立于仓库）；其他 CLI（zcode/glm-cli 等）按需在 `run-agent.sh` 加 `TOOL` 分支后同样可开链（见 §3.8） |
| 只跑规划者＋确认通道 | 不需要 Go/Rust、不需要 claude/codex | — |

检查命令：`node --version && jq --version && gh --version`。

## 3. macOS 落地步骤（推荐，约 15 分钟）

```bash
# 1. clone 并进入仓库
git clone https://github.com/channing771/mornlea.git && cd mornlea

# 2. 安装确认通道依赖（用 lock 文件保证与开发机一致的 SDK 版本）
cd scripts/agents/confirm && npm ci && cd ../..

# 3. 生成确认通道配置骨架 + launchd 保活任务
scripts/agents/confirm/install-listener.sh
# 产物：~/.mornlea/confirm/feishu.json（mode 600）与
#       ~/Library/LaunchAgents/com.mornlea.feishu-listener.plist（真实 node/仓库路径）
```

**4. 飞书应用（二选一）**

- *方案 A：复用现有应用*（推荐，最省事）
  把当前机器的 `~/.mornlea/confirm/feishu.json` 里的 `appId`/`appSecret`（以及 `receive`，同应用同用户 open_id 不变）复制到新机器同一路径；`resumeCmd` 保留 installer 生成的新仓库路径，不要拷贝旧值。
- *方案 B：新建应用*（若想隔离或轮换凭据，推荐含 appSecret 曾外泄的场景）
  1. <https://open.feishu.cn> → 开发者后台 → 创建企业自建应用，开启**机器人**能力；权限添加 `im:message`；事件订阅选 **WebSocket 长连接接收**，添加 `im.message.receive_v1`；
  2. **「事件与回调 → 回调配置」启用**（卡片按钮 `card.action.trigger` 回传依赖它，不启用则按钮无响应，文本回复仍可用）；
  3. 创建版本并发布；把 App ID / App Secret 填入 `~/.mornlea/confirm/feishu.json`；
  4. 捕获接收对象：`node scripts/agents/confirm/feishu-listener.js --bootstrap`，在飞书里给机器人发任意一句，监听器自动写入 `receive`（open_id/chat_id）。

**5. 启动并验证监听器**

```bash
launchctl load ~/Library/LaunchAgents/com.mornlea.feishu-listener.plist
tail -f ~/Library/Logs/mornlea-listener.log   # 看到「长连接已就绪」
# 发一张测试卡，确认按钮/输入区可达：
scripts/agents/confirm/confirm.sh ask --id T-01 --title '新机验证' --category bounded \
  --kind question --question '看到此卡即通道 OK，点按钮或回复即可' \
  --option 'A. 通道正常' --channel feishu
```

**6. GitHub 登录**：`gh auth login`（需对该仓库有读＋写权限：推送/PR/CI/讨论评论都走 gh）。

**7. 安装定时规划者（每天 09:00，可 `CRON_HOUR`/`CRON_MIN` 覆盖）**

```bash
scripts/agents/install-cron.sh planner      # macOS/Linux 通用
# macOS 也行（更接近 launchd 家族）：
scripts/agents/install-launchd.sh planner
```

**8.（可选）启动实现者继电器链**

```bash
# 主链（默认 claude，full-permission 模式；AGENT_SAFE=1 可回受限模式）
AGENT_LOOP=1 make agent-implementer
# 任意多条并行链：WORKER_ID 唯一即可（守卫 = ~/.mornlea/loop.guard.<WORKER_ID>）
WORKER_ID=codex AGENT_TOOL=codex AGENT_LOOP=1 make agent-implementer
WORKER_ID=codex2 AGENT_TOOL=codex AGENT_LOOP=1 make agent-implementer   # 第三/四条同理
# 其他 CLI 工具（如 zcode/glm-cli）接入：run-agent.sh 加 TOOL 分支后
#   AGENT_TOOL=zcode WORKER_ID=zcode AGENT_LOOP=1 … 同样开链；飞书确认与 relay 自动继承
```

继电器行为：`relay.sh` 用原子锁 + `~/.mornlea/loop.guard[.$WORKER_ID]` 防止同链并发；下一棒以 **setsid 独立会话**拉起（脱离宿主 agent 会话进程组，收尾退出不会连带杀它）；实现者收尾时自动接力下一行，无「未认领」任务自动终结。**守卫由 `run-agent.sh` 统一以真实会话 pid 登记**（启动/接力/续跑都会检查存活 pid，防同一链双开）；链身份（WORKER_ID + AGENT_TOOL）经 relay 与 listener 续跑自动继承。

> ⚠️ **claude 账号有用量上限**：会话撞限时以 `session limit` 提示退出（exit 1）——等 reset 时间后用 `AGENT_RESUME=<该行最近确认请求ID> … run-agent.sh implementer` 续跑**同一行**（详见 `docs/agents/implementer.md` 故障恢复）。建议 codex 作主力多链，claude 只留 1 条。

## 4. Linux 落地差异

- **Listener 常驻**：仓库只提供 macOS launchd 方案；Linux 二选一：
  - *systemd*：写一个 `~/.config/systemd/user/mornlea-listener.service`，`ExecStart=/usr/bin/node <repo>/scripts/agents/confirm/feishu-listener.js`，`Restart=always`；
  - *cron 兜底*：`* * * * * pgrep -f feishu-listener.js >/dev/null || (cd <repo> && nohup node scripts/agents/confirm/feishu-listener.js >> ~/.mornlea/listener.log 2>&1 &)`。
- **定时**：`install-cron.sh` 通用（它只写 crontab，不依赖 launchd）。
- **日志目录**：`relay.sh`/`pr-finalize.sh` 默认写 `$HOME/Library/Logs`（历史遗留路径）；Linux 上会自建该目录，无害，或设 `MORNLEA_LOOP_LOG` 改到 `~/.mornlea/logs`。
- **Node 路径**：`install-listener.sh` 用 `NODE_BIN`（默认 `$(which node)`），Linux 无需特殊处理。

## 5. 不要迁移的目录（`~/.mornlea/`）

正确姿势：新机器上**什么都不拷**，全由 installer 与 agent 自然重建。若强拷，会产生：死 pid 的 guard、被误认成待答的旧请求文件、指向旧仓库路径的 `resumeCmd`。唯一可迁移的是 `feishu.json`（敏感，建议安全通道拷贝；若怀疑凭据曾外泄，直接走方案 B 新建应用）。

## 6. 常见坑

| 坑 | 说明 |
|---|---|
| 双机同时跑继电器链 | 不同机器的 guard 互不知情，会**抢同一张 backlog**。约定：循环链只在一台机器上跑；另一台按需手动 `make agent-implementer` |
| 旧 pending 请求 | 新机跑 `confirm.sh list`，确认无历史 pending 再启用确认流 |
| 卡片按钮无回传 | 「事件与回调 → 回调配置」未启用（macOS 安装步骤 4.B.2）。文本「回复」路径不受影响 |
| `open_id` 变了 | 换了飞书应用或换用户账号——重跑 `--bootstrap` |
| 飞书 SDK 版本漂移 | 用 `npm ci`（有 lock 文件）；不要 `npm install` 升级到未验证的大版本（卡片回调/表单 schema 依赖当前线） |
| Card 2.0 表单 schema | 脚本已处理（form 内元素必须有 `name`、提交按钮必须 `form_action_type: submit`）；手工改卡片时注意 |
| `AGENT_SAFE` | 默认 full-permission（claude `--dangerously-skip-permissions`；codex `--dangerously-bypass-approvals-and-sandbox`）；要受限模式设 `AGENT_SAFE=1`。仓库 hooks（`guard.mjs`）始终独立生效 |
| claude 用量上限 | 撞限会话以 `session limit` 退出（exit 1）——等 reset 后用 `AGENT_RESUME=<确认ID>` 续跑同一行；不要重新认领（见第 3.8 节与 implementer 卡「故障恢复」） |
| 守卫不要手动写 | `~/.mornlea/loop.guard*` 由 `run-agent.sh` 登记**真实会话 pid**；不要再 `echo $$`（旧写法写的是 bash 工具临时 shell 的 pid，命令一返回即死，防重入形同虚设） |
| 讨论区纪律 | 卡片已送达但用户未回复 ≠ 通道降级——不往 Discussion #71 写卡片转录，只等 listener 续跑；实现者收尾用 `python3 scripts/agents/refresh-discussion.py --update` 同步正文（状态评论 + 正文同时到位） |
| 卡片样式 | 新版面：头部按类型分色（澄清=橙/确认=蓝）、选项只保留按钮、底部说明默认不显示；手工改 `feishu.sh` 时注意 Card 2.0（form 内元素必须有 `name`、提交按钮 `form_action_type: submit`） |
| 时间与 CI | 本机不用管 CI（GitHub 侧）；`pr-finalize.sh` 收尾守护依赖 `gh` 已登录 |

## 7. 新机验收清单

```text
□ node/jq/gh/python3 版本命令均可用
□ npm ci 成功（node_modules/@larksuiteoapi 存在）
□ install-listener.sh 生成 feishu.json 与 plist；launchctl list | grep mornlea 可见
□ 监听日志「长连接已就绪」
□ 测试卡 T-01 在飞书可达；按钮（或输入区/文本回复）回传后 confirm.sh status T-01 显示 answered
□ gh auth status 显示已登录且对该仓库有写权限
□ crontab -l / launchctl list 里有 planner 定时
□ confirm.sh list 无历史 pending（失败时先清理）
□ （可选）AGENT_LOOP=1 链已启动，backlog 出现「已认领」行
□ 守卫真实 pid：`cat ~/.mornlea/loop.guard[.<WORKER_ID>]` 的 pid 用 `ps -p <pid>` 查得到（防双开生效）
□ 收尾链路：状态评论发出后 `refresh-discussion.py --update` 正文同步（可人工跑一次验证）
```

> 相关文档：`docs/agents/confirmation-channel.md`（确认通道机制与飞书应用配置）、`docs/agents/planner.md` / `implementer.md`（角色卡）、`docs/development-process.md`（开发流程）。
