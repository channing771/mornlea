#!/usr/bin/env bash
# 实现者接力器：当前实现者会话完成（或放弃）后，自动启动下一个实现者认领下一行任务。
# 由 implementer.md 收尾清单调用；也可手动启动整个循环：AGENT_LOOP=1 make agent-implementer
# 环境变量: MORNLEA_LOOP_GUARD(默认 ~/.mornlea/loop.guard) / MORNLEA_LOOP_LOG
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# 多工作者并行：WORKER_ID 唯一时各自独立守卫链（loop.guard.<id>），互不排斥；
# 未设置 WORKER_ID 时用主链 ~/.mornlea/loop.guard（默认 claude 工作者的向后兼容路径）。
WORKER_ID="${WORKER_ID:-}"
if [ -n "$WORKER_ID" ]; then
  GUARD="${MORNLEA_LOOP_GUARD:-$HOME/.mornlea/loop.guard.$WORKER_ID}"
else
  GUARD="${MORNLEA_LOOP_GUARD:-$HOME/.mornlea/loop.guard}"
fi
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

# 暂停认领：有人工暂停标志（~/.mornlea/claims.paused）时，本链在此终结，不再接力下一行；
# 在飞/已认领任务可继续完成，但不会有新行被认领（run-agent.sh 同样拒绝启动新循环）。
if [ -f "$HOME/.mornlea/claims.paused" ]; then
  log "检测到暂停认领标志（~/.mornlea/claims.paused），本链终结——继续完成的已认领任务不受影响"
  rm -f "$GUARD"
  exit 0
fi

# 保护：guard 里若还有存活 pid（其它活动循环会话）则不应接力——正常流程里收尾已 rm guard，
# 这里的检查兜住「忘记释放」的情况。
if [ -f "$GUARD" ]; then
  OPID="$(cat "$GUARD" 2>/dev/null || echo 0)"
  if [ "${OPID:-0}" -gt 0 ] 2>/dev/null && kill -0 "$OPID" 2>/dev/null; then
    log "活动实现者仍在运行（pid=${OPID}），不接力"
    exit 0
  fi
fi

# 终结判据：规划表没有状态单元格为「就绪」的任务行。
if ! awk -F '|' '
  /^\| A-[0-9][0-9] \|/ && $5 ~ /^[[:space:]]*就绪[[:space:]]*$/ { ready = 1 }
  /^\| [B-F]-[0-9][0-9] \|/ && $6 ~ /^[[:space:]]*就绪[[:space:]]*$/ { ready = 1 }
  END { exit !ready }
' "$ROOT/docs/feature-backlog.md"; then
  rm -f "$GUARD"
  log "规划表已无就绪任务，循环终结"
  exit 0
fi

# 链名迁移映射（接力落点改名）：主链（无 WORKER_ID）→ opus、claude2 → opus-b。
# 只在 relay 启动下一棒的落点替换，不改变在飞当前行的旧名（如 A-02 以 claude2 身份
# 走到收尾，收尾 relay 时这一棒才以 opus-b 接力）。MORNLEA_CHAIN_RENAME 可整体覆盖
# 映射表（逗号分隔 old=new，如 "=opus,opus=main"；k 为空串匹配主链），未设置用内置表。
CHAIN_ID="${WORKER_ID:-}"
if [ -n "${MORNLEA_CHAIN_RENAME:-}" ]; then
  for PAIR in $(printf '%s' "$MORNLEA_CHAIN_RENAME" | tr ',' ' '); do
    [ -n "$PAIR" ] || continue
    OLD="${PAIR%%=*}"; NEW="${PAIR#*=}"
    if [ "$OLD" = "$CHAIN_ID" ]; then CHAIN_ID="$NEW"; break; fi
  done
else
  case "$CHAIN_ID" in
    "") CHAIN_ID="opus" ;;
    claude2) CHAIN_ID="opus-b" ;;
  esac
fi

# 启动下一个实现者（detached）；成功交给它接管循环
if [ -x "$ROOT/scripts/agents/run-agent.sh" ]; then
  # 保持链身份：同 WORKER_ID、同工具（WORKER_TOOL 默认 claude）
  # 链身份：WORKER_TOOL 未设时回退 AGENT_TOOL（链路通常只带 AGENT_TOOL=codex/claude），再回退 claude。
  # 关键：用 setsid 让下一棒脱离宿主 agent 会话的进程组——实现者收尾退出时会清理整组，
  # nohup 只能挡 SIGHUP、挡不住组杀（listener 续跑用 detached:true 同理）；脱离后接力会话才能存活。
  CHAIN_TOOL="${WORKER_TOOL:-${AGENT_TOOL:-claude}}"
  (cd "$ROOT" && AGENT_LOOP=1 WORKER_ID="$CHAIN_ID" AGENT_TOOL="$CHAIN_TOOL" SPAWN_ROOT="$ROOT" SPAWN_LOG="$LOG" \
    python3 -c 'import os,subprocess; os.setsid(); env=os.environ.copy(); f=open(env.get("SPAWN_LOG","/dev/null"),"a"); subprocess.Popen(["/bin/bash","-lc","cd \\\"%s\\\" && scripts/agents/run-agent.sh implementer" % env.get("SPAWN_ROOT",".")], env=env, stdout=f, stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL)' >/dev/null 2>&1 &)
  log "已接力启动下一个实现者（AGENT_LOOP=1，日志 ${LOG}）"
  exit 0
fi
rm -f "$GUARD"
log "run-agent.sh 缺失，循环终结"
exit 1
