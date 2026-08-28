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
  # --watch 在检查尚未注册或 API 抖动时会提前退出，退出后必须重新全量核对，
  # 不能信任本轮 watch 已看到全部检查完成。
  gh pr checks "$PR" --watch --interval 30 >/dev/null 2>&1 || true
  ST="$(gh pr view "$PR" --json state -q .state 2>/dev/null || echo OPEN)"
  if [ "$ST" = "MERGED" ]; then log "PR #${PR} 已合并（可能由他人完成）"; exit 0; fi
  # 合并硬门禁：检查总数 > 0 且每个检查都为成功态（pass/success/skipping/
  # skipped/neutral）。只数 FAILURE 会把 PENDING 当成 0 失败而在 CI 未完成时
  # 抢跑合并（PR #115 实测）。
  STATES="$(gh pr checks "$PR" --json state 2>/dev/null | jq -r '[.[].state | ascii_downcase] | join(" ")' 2>/dev/null || echo "")"
  TOTAL="$(echo "$STATES" | wc -w | tr -d ' ')"
  BAD="$(echo "$STATES" | tr ' ' '\n' | grep -cvE '^(pass|success|skipping|skipped|neutral)$' || true)"
  BAD="$([ -z "${STATES// /}" ] && echo 1 || echo "$BAD")"
  if [ "$ST" = "OPEN" ] && [ "$TOTAL" -gt 0 ] && [ "$BAD" -eq 0 ]; then
    if gh pr merge "$PR" --merge >> "$LOG" 2>&1; then log "CI 全绿（${TOTAL} 项检查全部成功），已合并 PR #${PR}"; exit 0; fi
    log "合并命令失败（可能被分支保护/并发修改拦截），停止"; exit 1
  fi
  if [ "$TOTAL" -eq 0 ]; then
    log "检查尚未注册，30s 后重查（不消耗重跑轮次）"
    sleep 30
    continue
  fi
  log "存在 ${BAD} 个非成功检查（第 ${ROUND} 轮）——flaky 的话重跑即可；若重跑后仍失败则需修复"
  if [ "$ROUND" -lt 3 ]; then
    RUN="$(latest_run)"
    if [ -n "$RUN" ]; then gh run rerun "$RUN" --failed >> "$LOG" 2>&1 || true; fi
    sleep 30
    continue
  fi
done
log "重跑 3 轮后仍有失败：需要修复（查看 gh run list 或 PR #${PR} 检查详情）——保持 OPEN，等待实现者/人工接管"
exit 1