#!/usr/bin/env bash
# 安装每日定时任务到 crontab（macOS/Linux 通用；默认 09:00，CRON_HOUR/CRON_MIN 可覆盖）。
set -euo pipefail
ROLE="${1:-planner}"
case "$ROLE" in planner|implementer) ;; *) echo "用法: install-cron.sh <planner|implementer>"; exit 2 ;; esac
HOUR="${CRON_HOUR:-9}"
MINUTE="${CRON_MIN:-0}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
mkdir -p "$HOME/Library/Logs"
LOG="$HOME/Library/Logs/mornlea-$ROLE.log"
LINE="$MINUTE $HOUR * * * /bin/bash -lc '$ROOT/scripts/agents/run-agent.sh $ROLE >> $LOG 2>&1'"
( crontab -l 2>/dev/null | grep -v "run-agent.sh $ROLE" || true; echo "$LINE" ) | crontab -
echo "已安装 cron（${ROLE}）: $LINE"
