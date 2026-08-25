#!/usr/bin/env bash
# PR 收尾守护：detached 运行——监听 PR CI（失败自动 failed-only 重跑，最多 3 轮），全绿后合并。
# 实现者收尾时以 nohup 方式启动，自身退出也不影响 CI 盯守：
#   nohup scripts/agents/pr-finalize.sh <PR号> >> <log> 2>&1 &
set -euo pipefail

PR="${1:?用法: pr-finalize.sh <PR号>}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOGDIR="$HOME/Library/Logs"
LOG="${MORNLEA_PR_FINALIZE_LOG:-$LOGDIR/mornlea-pr-finalize.log}"
mkdir -p "$LOGDIR"

log() { echo "[pr-finalize $(date '+%F %T')] $*" >> "$LOG"; echo "[pr-finalize] $*"; }

latest_run() { gh run list --branch "$(gh pr view "$PR" --json headRefName -q .headRefName 2>/dev/null)" --limit 1 --json databaseId -q '.[0].databaseId' 2>/dev/null || true; }

for ROUND in 1 2 3; do
  log "第 ${ROUND} 轮：监听 PR #${PR} 的 CI…"
  gh pr checks "$PR" --watch --interval 30 >/dev/null 2>&1 || true
  ST="$(gh pr view "$PR" --json state -q .state 2>/dev/null || echo OPEN)"
  if [ "$ST" = "MERGED" ]; then log "PR #${PR} 已合并（可能由他人完成）"; exit 0; fi
  FAILS="$(gh pr checks "$PR" --json name,state 2>/dev/null | jq '[.[] | select(.state=="FAILURE")] | length' 2>/dev/null || echo 0)"
  if [ "${FAILS:-0}" -eq 0 ] && [ "$ST" = "OPEN" ]; then
    if gh pr merge "$PR" --merge >> "$LOG" 2>&1; then log "CI 全绿，已合并 PR #${PR}"; exit 0; fi
    log "合并命令失败（可能被分支保护/并发修改拦截），停止"; exit 1
  fi
  log "存在 ${FAILS} 个失败 job（第 ${ROUND} 轮）——flaky 的话重跑即可；若重跑后仍失败则需修复"
  if [ "$ROUND" -lt 3 ]; then
    RUN="$(latest_run)"
    if [ -n "$RUN" ]; then gh run rerun "$RUN" --failed >> "$LOG" 2>&1 || true; fi
    sleep 30
    continue
  fi
done
log "重跑 3 轮后仍有失败：需要修复（查看 gh run list 或 PR #${PR} 检查详情）——保持 OPEN，等待实现者/人工接管"
exit 1