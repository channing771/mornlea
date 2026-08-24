# 设备确认通道（飞书优先）

brainstorm 内容确认（docs/development-process.md 阶段 0.5）要求「呈现设计 → 获得显式批准 → 才开工」。在 headless 定时调度下，实现者不能对着终端提问，本通道把确认请求**推送到你的手机**，你回复后**自动续跑**任务；无通道时降级为 GitHub Discussion 评论协议。

## 架构

```
实现者（run-agent.sh implementer）
  └─ 阶段 0.5 内容确认
       │ confirm.sh ask --id B-02 --title ...     （写 ~/.mornlea/confirm/<id>.json）
       │ └─ feishu.sh send <id>                   （飞书应用消息 API → 你的飞书）
       ▼
你的飞书收到：【Mornlea 内容确认】... 请回复：✅ 批准 或 修改意见
       ▼
你的回复（文本，可带 #B-02 指定请求）
       ▼
feishu-listener.js（长连接事件订阅，常驻 daemon）
       │ 解析动作（approve/edit/reject）→ 写 <id>.reply.json
       │ 发 ack「已收到确认…」
       └─ 自动续跑：AGENT_RESUME=<id> 后台执行 run-agent.sh implementer
       ▼
实现者恢复：读 <id>.reply.json → 结论写入 OpenSpec 产物 → 继续开发
```

## 通道与降级链

| 优先级 | 通道 | 触发 | 说明 |
|---|---|---|---|
| 1 | feishu | 配置了 ~/.mornlea/confirm/feishu.json（AGENT_CONFIRM_CHANNEL=feishu 或 auto 探测到） | 推送设备，回复即续跑 |
| 2 | discussion | 飞书发送失败 / 显式指定 / 等待超时 | 发布到 GitHub Discussion #71 对应评论，用户回复后 confirm.sh reply 或下次调度恢复 |
| 3 | none | 未配置任何通道 | 本地记录；confirm.sh reply 模拟回复（测试用） |

## 配置飞书（一次性，约 10 分钟）

1. 打开 <https://open.feishu.cn> → 开发者后台 → **创建企业自建应用**（个人可用企业版飞书免费注册）。
2. 应用详情：添加**机器人**能力；权限管理添加 im:message（获取与发送单聊/群聊消息）。
3. 「事件与回调」→ 事件订阅：选 **通过 WebSocket 接收事件**，添加 im.message.receive_v1（接收消息）。
4. 创建版本并**发布**（自建应用发布后 API 才生效）。
5. 把凭证与基础信息里的 **App ID / App Secret** 填进配置（见下）。
6. **捕获 receive**：启动监听器并给你的机器人发一条消息：

   ```
   node scripts/agents/confirm/feishu-listener.js --bootstrap
   # 然后在飞书里找到你的应用（搜索应用名）→ 私聊发任意一句
   # 监听器会打印并写入 receive（open_id/chat_id）
   ```

   不想走 bootstrap 时，也可把机器人在的群聊 chat_id 填入 receive.id（type=chat_id）。

## 配置文件 ~/.mornlea/confirm/feishu.json

```
{
  "appId": "cli_xxxxxxxxxxxx",
  "appSecret": "xxxxxxxxxxxxxxxx",
  "receive": { "type": "open_id", "id": "ou_xxxxxxxxxxxx" },
  "autoResume": true,
  "resumeCmd": "cd /Users/you/minecraft-go && scripts/agents/run-agent.sh implementer"
}
```

- receive：type 取 open_id（bootstrap 自动写入）或 chat_id；id 为对应值。
- autoResume：收到回复后是否自动后台续跑实现者（默认 true；关闭时只写回复文件，需手动 make agent-implementer）。
- resumeCmd：续跑命令（默认 = 当前仓库的 run-agent.sh implementer，可覆盖）。

## 常用命令

```
# 发起确认（实现者第 3 步使用）
confirm.sh ask --id B-02 --title '水桶（可搬运流体）' --category bounded \
  --question '是否按短设计实施？' --design '舀水/倒水物品……'

# 等待回复（明确 approve/edit/reject；超时退出码 3 → 降级 Discussion 协议）
confirm.sh wait --id B-02 --timeout-min 30

# 手工回复（测试 / discussion 降级后的恢复入口）
confirm.sh reply --id B-02 --action approve --text '✅ 批准'

# 状态与列表
confirm.sh status B-02
confirm.sh list
```

路径约定：scripts/agents/confirm/（CLI 与监听器）；状态目录默认 ~/.mornlea/confirm/（MORNLEA_CONFIRM_DIR 可覆盖）。

## 数据文件

| 文件 | 含义 |
|---|---|
| <id>.json | 请求：{id, title, category, question, design, status(pending/answered), channel, createdAt…} |
| <id>.reply.json | 回复：{id, action(approve/edit/reject), text, repliedAt, senderOpenId, chatId, messageId} |
| feishu-token.json | tenant_access_token 缓存（过期自动刷新） |
| resume-<id>.log | 续跑实现者的后台日志 |

## 回复动作判定（listener）

- 批准：文本含 批准/同意/认可/ok/approve/继续/可以/确认/✅/👍，且不含 不/别/勿/修改；
- 驳回：文本含 驳回/拒绝/取消/reject/不行/不要/别做/❌；
- 其他一律视为「修改意见」（action=edit，文本进 reply.text 作为设计修订输入）。

## 监听器常驻（launchd 保活推荐）

**一步到位：** 直接运行仓库内的一键安装器（自动生成真实路径的配置骨架 + launchd plist）：

```bash
scripts/agents/confirm/install-listener.sh
```

它做的事：生成本机路径（repo 根、node 路径 `$(which node)`）的 `com.mornlea.feishu-listener.plist`（KeepAlive=true，断线/崩溃自动拉起）与 `~/.mornlea/confirm/feishu.json` 骨架（mode 600；`resumeCmd` 已填真实仓库路径），有凭据时直接 launchctl load。等价的手工 plist 模板：

```
cat > ~/Library/LaunchAgents/com.mornlea.feishu-listener.plist <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.mornlea.feishu-listener</string>
  <key>ProgramArguments</key><array>
    <string>/opt/homebrew/bin/node</string>
    <string>/Users/chen/chenwork/minecraft-go/scripts/agents/confirm/feishu-listener.js</string>
  </array>
  <key>KeepAlive</key><true/>
  <key>RunAtLoad</key><true/>
  <key>WorkingDirectory</key><string>/Users/chen/chenwork/minecraft-go</string>
  <key>StandardOutPath</key><string>/Users/chen/Library/Logs/mornlea-listener.log</string>
  <key>StandardErrorPath</key><string>/Users/chen/Library/Logs/mornlea-listener.err.log</string>
</dict></plist>
PLIST
launchctl load ~/Library/LaunchAgents/com.mornlea.feishu-listener.plist
```

（本机实测 node 在 `/opt/homebrew/bin/node`；换机器时以 `which node` 为准。）

## 微信 / 企业微信备选

- **个人微信**：无官方开放收消息接口，不做。
- **企业微信自建应用**：可发送应用消息；接收回复需要**回调 URL（公网或内网穿透）**，事件签名验证后把消息写回同一套 <id>.reply.json 即可复用本机制——实现同一契约的另一 adapter，可照搬本 listener 的回复解析（默认最新 pending / #ID 精确指定）。

## 降级与失败恢复

- confirm.sh ask 在 feishu 不可用时**自动降级**并在请求文件里记录 channel=discussion，打印发布与恢复命令。
- confirm.sh wait 超时（默认 30 分钟）退出码 3：实现者按 docs/development-process.md 阶段 0.5 的 Discussion 协议发评论并停在确认点。
- listener 掉线：SDK 自动重连；launchd KeepAlive 兜底；回复文件是唯一的续跑事实源，任何时刻重跑 make agent-implementer 都可从既有 reply 恢复。
