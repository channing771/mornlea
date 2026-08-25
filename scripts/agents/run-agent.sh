#!/usr/bin/env bash
# 通用工作者入口：读取 docs/agents/<role>-prompt.md（缺省回退 role 角色卡）作为提示词，调用 claude/codex 执行。
# 用法: run-agent.sh <planner|implementer> [claude|codex]
# 环境变量: AGENT_TOOL(默认 claude) / AGENT_MODEL(默认 CLI 配置中的模型) / CLAUDE_BIN / CODEX_BIN / AGENT_EXTRA_ARGS
set -euo pipefail

# launchd(listener 续跑)下 PATH 极简，claude/codex 常在 ~/.local/bin 或 /opt/homebrew/bin 等位置；
# 按 PATH → 常见绝对路径依次解析，保证 headless 续跑能找到 CLI。
resolve_cli() { # $1 = CLI 名称
  local n="$1" p
  if command -v "$n" >/dev/null 2>&1; then command -v "$n"; return 0; fi
  for p in "$HOME/.local/bin/$n" "/opt/homebrew/bin/$n" "/usr/local/bin/$n" "/usr/bin/$n"; do
    if [ -x "$p" ]; then echo "$p"; return 0; fi
  done
  return 1
}

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

# 权限模式：默认最高权限（用户明确要求 full pass）——claude --dangerously-skip-permissions，
# codex --dangerously-bypass-approvals-and-sandbox。仓库 hooks(guard.mjs) 仍独立生效，不受此影响。
# 若要回到受限模式：AGENT_SAFE=1（claude 回到默认 per-request 批准；codex 回到 --approve-for-me）。
BYpass=""
if [ "${AGENT_SAFE:-0}" = "1" ]; then
  case "$TOOL" in
    claude) : ;;                    # 默认交互批准模式
    codex) BYpass="--approve-for-me" ;;
  esac
else
  case "$TOOL" in
    claude) BYpass="--dangerously-skip-permissions" ;;
    codex) BYpass="--dangerously-bypass-approvals-and-sandbox" ;;
  esac
fi

# 接力循环守卫：AGENT_LOOP=1 时以**本进程 pid** 登记链守卫（exec 后 CLI 与守卫同 pid，
# kill -0 判活准确——旧做法让 agent 自己 echo $$ 写入的是临时 shell pid，命令一返回即失效，
# 防重入形同虚设）。若本链 guard 已有存活 pid 则直接退出，杜绝并发双开。
if [ "${AGENT_LOOP:-0}" = "1" ]; then
  # 暂停认领：人工暂停标志存在时拒绝启动任何新循环（含手动/cron/接力触发的新会话）
  if [ -f "$HOME/.mornlea/claims.paused" ]; then
    echo "[run-agent] 已在暂停认领（~/.mornlea/claims.paused）——不启动新实现者循环；删除该文件可恢复" >&2
    exit 0
  fi
  if [ -n "${WORKER_ID:-}" ]; then LIVE_GUARD="$HOME/.mornlea/loop.guard.$WORKER_ID"; else LIVE_GUARD="$HOME/.mornlea/loop.guard"; fi
  mkdir -p "$HOME/.mornlea"
  if [ -f "$LIVE_GUARD" ]; then
    OPID="$(cat "$LIVE_GUARD" 2>/dev/null || echo 0)"
    if [ "${OPID:-0}" -gt 0 ] 2>/dev/null && [ "$OPID" != "$$" ] && kill -0 "$OPID" 2>/dev/null; then
      echo "[run-agent] 链守卫 ${LIVE_GUARD} 已有存活 pid=${OPID}，本会话退出（防双开）" >&2
      exit 0
    fi
  fi
  echo "$$" > "$LIVE_GUARD"
  echo "[run-agent] 已登记链守卫 ${LIVE_GUARD} (pid=$$)"
fi

case "$TOOL" in
  claude)
    CLAUDE_BIN="${CLAUDE_BIN:-$(resolve_cli claude || echo claude)}"
    # 默认 headless 打印模式；需要终端交互问答时设置 AGENT_INTERACTIVE=1。
    if [ "${AGENT_INTERACTIVE:-0}" = "1" ]; then
      exec "$CLAUDE_BIN" "$PROMPT" $MODEL_ARGS $BYpass ${AGENT_EXTRA_ARGS:-}
    fi
    exec "$CLAUDE_BIN" -p "$PROMPT" $MODEL_ARGS $BYpass ${AGENT_EXTRA_ARGS:-}
    ;;
  codex)
    CODEX_BIN="${CODEX_BIN:-$(resolve_cli codex || echo codex)}"
    # 新版 codex exec：--full-auto 已移除；最高权限用 --dangerously-bypass-approvals-and-sandbox
    exec "$CODEX_BIN" exec $BYpass $MODEL_ARGS "$PROMPT" ${AGENT_EXTRA_ARGS:-}
    ;;
  *) usage ;;
esac
