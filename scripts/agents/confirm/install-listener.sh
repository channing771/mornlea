#!/usr/bin/env bash
# 设备确认通道一键安装：生成真实路径的配置与 launchd 保活任务，尽量替你"填好"。
# 剩余人工项（无法自动化，涉及你的飞书账号）：
#   1) 创立/配置飞书自建应用（见 docs/agents/confirmation-channel.md 第 4 节）
#   2) 把 App ID / App Secret 填进 ~/.mornlea/confirm/feishu.json
#   3) 运行: node scripts/agents/confirm/feishu-listener.js --bootstrap（捕获你的 open_id）
set -euo pipefail

NODE_BIN="${NODE_BIN:-$(which node || echo /opt/homebrew/bin/node)}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
DIR="${MORNLEA_CONFIRM_DIR:-$HOME/.mornlea/confirm}"
PLIST="$HOME/Library/LaunchAgents/com.mornlea.feishu-listener.plist"
CFG="$DIR/feishu.json"

mkdir -p "$DIR" "$HOME/Library/LaunchAgents"
chmod 700 "$DIR"

# 1) 配置骨架（已有则不覆盖，保留你的填写）
if [ ! -f "$CFG" ]; then
  cat > "$CFG" <<CONF
{
  "appId": "",
  "appSecret": "",
  "receive": { "type": "open_id", "id": "" },
  "autoResume": true,
  "resumeCmd": "cd $ROOT && scripts/agents/run-agent.sh implementer"
}
CONF
  chmod 600 "$CFG"
  echo "[install] 已生成配置: $CFG"
else
  echo "[install] 配置已存在，保留原内容: $CFG"
fi

# 2) launchd 保活 plist（真实 node/仓库路径）
cat > "$PLIST" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>com.mornlea.feishu-listener</string>
  <key>ProgramArguments</key><array>
    <string>$NODE_BIN</string>
    <string>$ROOT/scripts/agents/confirm/feishu-listener.js</string>
  </array>
  <key>KeepAlive</key><true/>
  <key>RunAtLoad</key><true/>
  <key>WorkingDirectory</key><string>$ROOT</string>
  <key>StandardOutPath</key><string>$HOME/Library/Logs/mornlea-listener.log</string>
  <key>StandardErrorPath</key><string>$HOME/Library/Logs/mornlea-listener.err.log</string>
</dict></plist>
PLIST
echo "[install] 已生成 launchd plist: $PLIST"

# 3) 有凭据才启动；否则提示剩余人工项
if [ -n "$(jq -r '.appId // empty' "$CFG")" ] && [ -n "$(jq -r '.appSecret // empty' "$CFG")" ]; then
  launchctl unload "$PLIST" 2>/dev/null || true
  launchctl load "$PLIST"
  echo "[install] 已加载 listener（launchctl list | grep mornlea 可查）"
else
  launchctl unload "$PLIST" 2>/dev/null || true
  echo "[install] 待你填写 appId/appSecret 后运行一次: launchctl load $PLIST"
fi

echo
echo "== 剩余 3 步（涉及你的飞书账号，无法代填）=="
echo "1. 创建企业自建应用：https://open.feishu.cn -> 开发者后台（机器人能力 + im:message 权限 + 事件订阅 WebSocket 接收 im.message.receive_v1 + 发布版本）"
echo "2. 编辑 $CFG 填写 appId / appSecret"
echo "3. 运行: node $ROOT/scripts/agents/confirm/feishu-listener.js --bootstrap，然后给你的机器人发一句，自动写入 receive"
echo "4. 启动监听: launchctl load $PLIST （或直接执行上一步后 Ctrl-C，再 load）"
