#!/usr/bin/env bash
# 通用工作者入口：读取 docs/agents/<role>-prompt.md（缺省回退 role 角色卡）作为提示词，调用 claude/codex 执行。
# 用法: run-agent.sh <planner|implementer> [claude|codex]
# 环境变量: AGENT_TOOL(默认 claude) / AGENT_MODEL(默认 CLI 配置中的模型) / CLAUDE_BIN / CODEX_BIN / AGENT_EXTRA_ARGS
set -euo pipefail

ROLE="${1:-}"
TOOL="${AGENT_TOOL:-claude}"
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PROMPT_FILE="$ROOT/docs/agents/$ROLE-prompt.md"
[ -f "$PROMPT_FILE" ] || PROMPT_FILE="$ROOT/docs/agents/$ROLE.md"

usage() { echo "用法: $(basename "$0") <planner|implementer> [claude|codex]" >&2; exit 2; }

case "$ROLE" in planner|implementer) ;; *) usage ;; esac
[ -f "$PROMPT_FILE" ] || { echo "缺少提示词/角色卡: $PROMPT_FILE" >&2; exit 3; }

PROMPT="$(cat "$PROMPT_FILE")"
cd "$ROOT"

# 模型：AGENT_MODEL 未设置时用各 CLI 自身配置(claude: ~/.claude/settings.json 的 model;
# codex: ~/.codex/config.toml 的 model)。需要固定模型时设置 AGENT_MODEL。
# 注意：用字符串而非数组——macOS 自带 bash 3.2 下空数组 + set -u 会报 unbound variable。
MODEL_ARGS=""
[ -n "${AGENT_MODEL:-}" ] && MODEL_ARGS="--model $AGENT_MODEL"

case "$TOOL" in
  claude)
    CLAUDE_BIN="${CLAUDE_BIN:-claude}"
    # 默认 headless 打印模式；需要无人值守自动批准时用
    # AGENT_EXTRA_ARGS='--permission-mode acceptEdits'；需要终端交互问答时
    # 手动运行 claude（不带 -p）并粘贴提示词，或设置 AGENT_INTERACTIVE=1。
    if [ "${AGENT_INTERACTIVE:-0}" = "1" ]; then
      exec "$CLAUDE_BIN" "$PROMPT" $MODEL_ARGS ${AGENT_EXTRA_ARGS:-}
    fi
    exec "$CLAUDE_BIN" -p "$PROMPT" $MODEL_ARGS ${AGENT_EXTRA_ARGS:-}
    ;;
  codex)
    CODEX_BIN="${CODEX_BIN:-codex}"
    # codex 的 exec 为 headless；终端交互请直接运行 `codex`(交互式) 并粘贴提示词。
    exec "$CODEX_BIN" exec --full-auto $MODEL_ARGS "$PROMPT" ${AGENT_EXTRA_ARGS:-}
    ;;
  *) usage ;;
esac
