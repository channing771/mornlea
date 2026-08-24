#!/usr/bin/env bash
# 通用工作者入口：读取 docs/agents/<role>.md 作为提示词，调用 claude/codex 执行。
# 用法: run-agent.sh <planner|implementer> [claude|codex]
# 环境变量: AGENT_TOOL / CLAUDE_BIN / CODEX_BIN / AGENT_EXTRA_ARGS
set -euo pipefail

ROLE="${1:-}"
TOOL="${AGENT_TOOL:-claude}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROMPT_FILE="$ROOT/docs/agents/$ROLE.md"

usage() { echo "用法: $(basename "$0") <planner|implementer> [claude|codex]" >&2; exit 2; }

case "$ROLE" in planner|implementer) ;; *) usage ;; esac
[ -f "$PROMPT_FILE" ] || { echo "缺少角色卡: $PROMPT_FILE" >&2; exit 3; }

PROMPT="$(cat "$PROMPT_FILE")"
cd "$ROOT"

case "$TOOL" in
  claude)
    CLAUDE_BIN="${CLAUDE_BIN:-claude}"
    # 默认打印模式；需要自动批准时用 AGENT_EXTRA_ARGS='--permission-mode acceptEdits'
    exec "$CLAUDE_BIN" -p "$PROMPT" ${AGENT_EXTRA_ARGS:-}
    ;;
  codex)
    CODEX_BIN="${CODEX_BIN:-codex}"
    exec "$CODEX_BIN" exec --full-auto "$PROMPT" ${AGENT_EXTRA_ARGS:-}
    ;;
  *) usage ;;
esac
