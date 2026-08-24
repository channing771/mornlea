#!/usr/bin/env bash
# 安装每日定时 launchd 任务（macOS；默认 09:00，PLANNER_AT_HOUR/PLANNER_AT_MINUTE 可覆盖）。
set -euo pipefail
ROLE="${1:-planner}"
HOUR="${PLANNER_AT_HOUR:-9}"
MINUTE="${PLANNER_AT_MINUTE:-0}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PLIST="$HOME/Library/LaunchAgents/com.mornlea.$ROLE.plist"
mkdir -p "$HOME/Library/Logs" "$HOME/Library/LaunchAgents"
cat > "$PLIST" <<EOF2
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
  <dict>
    <key>Label</key><string>com.mornlea.$ROLE</string>
    <key>ProgramArguments</key>
    <array>
      <string>/bin/bash</string>
      <string>-lc</string>
      <string>$ROOT/scripts/agents/run-agent.sh $ROLE</string>
    </array>
    <key>StartCalendarInterval</key>
    <dict><key>Hour</key><integer>$HOUR</integer><key>Minute</key><integer>$MINUTE</integer></dict>
    <key>StandardOutPath</key><string>$HOME/Library/Logs/mornlea-$ROLE.log</string>
    <key>StandardErrorPath</key><string>$HOME/Library/Logs/mornlea-$ROLE.err.log</string>
  </dict>
</plist>
EOF2
launchctl unload "$PLIST" 2>/dev/null || true
launchctl load "$PLIST"
echo "已安装 launchd（$ROLE 每日 $HOUR:$MINUTE）: $PLIST"
echo "查看: launchctl list | grep com.mornlea ; 日志: $HOME/Library/Logs/mornlea-$ROLE.log"
