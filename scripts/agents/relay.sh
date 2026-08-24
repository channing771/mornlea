#!/usr/bin/env bash
# 实现者接力器：当前实现者会话完成（或放弃）后，自动启动下一个实现者认领下一行任务。
# 由 implementer.md 收尾清单调用；也可手动启动整个循环：AGENT_LOOP=1 make agent-implementer
# 环境变量: MORNLEA_LOOP_GUARD(默认 ~/.mornlea/loop.guard) / MORNLEA_LOOP_LOG
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
GUARD="${MORNLEA_LOOP_GUARD:-$HOME/.mornlea/loop.guard}"
LOCK="$GUARD.lock"
LOGDIR="$HOME/Library/Logs"
LOG="${MORNLEA_LOOP_LOG:-$LOGDIR/mornlea-implementer-loop.log}"
mkdir -p "$LOGDIR" "$(dirname "$GUARD")"

log() { echo "[relay $(date '+%F %T')] $*" >> "$LOG"; echo "[relay] $*"; }

# 原子锁：防止两个会话同时接力（mkdir 原子性）
if ! mkdir "$LOCK" 2>/dev/null; then
  log "已有接力在排队，本会话退出"
  exit 0
fi
trap 'rmdir "$LOCK" 2>/dev/null || true' EXIT

# 保护：guard 里若还有存活 pid（其它活动循环会话）则不应接力——正常流程里收尾已 rm guard，
# 这里的检查兜住「忘记释放」的情况。
if [ -f "$GUARD" ]; then
  OPID="$(cat "$GUARD" 2>/dev/null || echo 0)"
  if [ "${OPID:-0}" -gt 0 ] 2>/dev/null && kill -0 "$OPID" 2>/dev/null; then
    log "活动实现者仍在运行（pid=$OPID），不接力"
    exit 0
  fi
fi

# 终结判据：规划表不再有「未认领」行（认领人在「未认领」状态列之后）
if ! grep -E '^\| [A-F]-[0-9]{2} \|' "$ROOT/docs/feature-backlog.md" | grep -q '未认领'; then
  rm -f "$GUARD"
  log "规划表已无未认领任务，循环终结"
  exit 0
fi

# 启动下一个实现者（detached）；成功交给它接管循环
if [ -x "$ROOT/scripts/agents/run-agent.sh" ]; then
  (cd "$ROOT" && AGENT_LOOP=1 nohup scripts/agents/run-agent.sh implementer >> "$LOG" 2>&1 &)
  log "已接力启动下一个实现者（AGENT_LOOP=1，日志 $LOG）"
  exit 0
fi
rm -f "$GUARD"
log "run-agent.sh 缺失，循环终结"
exit 1