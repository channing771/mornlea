#!/usr/bin/env bash
# Mornlea 设备确认 CLI：把确认请求推送到你的设备（优先飞书），收到回复后写入本地。
# 用法:
#   confirm.sh ask   --id X-xx --title "..." [--category bounded|architectural] [--question "..."] [--design "..."] [--channel feishu|discussion|none|auto]
#   confirm.sh wait  --id X-xx [--timeout-min 30] [--sleep-sec 5]
#   confirm.sh reply --id X-xx --action approve|edit|reject [--text "..."]
#   confirm.sh status X-xx | confirm.sh list
# 环境变量: AGENT_CONFIRM_CHANNEL / MORNLEA_CONFIRM_DIR
set -euo pipefail

DIR="${MORNLEA_CONFIRM_DIR:-$HOME/.mornlea/confirm}"
CMD="${1:-help}"; shift || true
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

usage() {
  cat >&2 <<USAGE
用法: confirm.sh ask|wait|reply|status|list
  ask   --id X-xx --title "..." [--category ...] [--question "..."] [--design "..."] [--channel auto|feishu|discussion|none]
  wait  --id X-xx [--timeout-min 30] [--sleep-sec 5]
  reply --id X-xx --action approve|edit|reject [--text "..."]
  status X-xx / list
USAGE
  exit 2
}

require_id() { [ -n "${1:-}" ] || { echo "缺少 --id" >&2; usage; }; }
now_iso() { date -u +%Y-%m-%dT%H:%M:%SZ; }

feishu_ready() {
  [ -f "$DIR/feishu.json" ] || return 1
  jq -e '.appId and .appSecret and .receive and .receive.id' "$DIR/feishu.json" >/dev/null 2>&1 || return 1
  return 0
}

case "$CMD" in
  ask)
    ID=""; TITLE=""; CATEGORY=""; QUESTION=""; DESIGN=""; CH=""
    while [ $# -gt 0 ]; do
      case "$1" in
        --id) ID="${2:-}"; shift 2 ;; --title) TITLE="${2:-}"; shift 2 ;;
        --category) CATEGORY="${2:-}"; shift 2 ;; --question) QUESTION="${2:-}"; shift 2 ;;
        --design) DESIGN="${2:-}"; shift 2 ;; --channel) CH="${2:-}"; shift 2 ;;
        *) echo "未知参数: $1" >&2; usage ;;
      esac
    done
    require_id "$ID"; [ -n "$TITLE" ] || { echo "缺少 --title" >&2; usage; }
    [ -n "$QUESTION" ] || QUESTION="是否按上述短设计实施？"
    [ -n "$CATEGORY" ] || CATEGORY="bounded"
    CH="${CH:-${AGENT_CONFIRM_CHANNEL:-auto}}"
    mkdir -p "$DIR"
    case "$ID" in feishu|feishu-token|pending) echo "ID 与通道保留名冲突，请换 ID" >&2; exit 2 ;; esac
    if [ -f "$DIR/$ID.reply.json" ]; then echo "该行已有回复文件（$DIR/$ID.reply.json），请先清理或等待续跑" >&2; exit 3; fi
    NOW="$(now_iso)"
    jq -nc --arg id "$ID" --arg t "$TITLE" --arg c "$CATEGORY" --arg q "$QUESTION" --arg d "$DESIGN" --arg ch "$CH" --arg n "$NOW"       '{id:$id, title:$t, category:$c, question:$q, design:$d, status:"pending", channel:$ch, createdAt:$n, updatedAt:$n}' > "$DIR/$ID.json"
    echo "[confirm] 已登记请求: $DIR/${ID}.json（channel=${CH}）"
    case "$CH" in
      feishu|auto)
        if [ "$CH" = "auto" ] && ! feishu_ready; then
          echo "[confirm] 未配置飞书（$DIR/feishu.json）→ 降级为本地记录（channel=none）"
          jq '.channel="none"' "$DIR/$ID.json" > "$DIR/$ID.json.tmp" && mv "$DIR/$ID.json.tmp" "$DIR/$ID.json"
        else
          if "$HERE/feishu.sh" send "$ID"; then
            echo "[confirm] 已推送到飞书。等待回复: confirm.sh wait --id $ID"
          else
            echo "[confirm] 飞书发送失败 → 降级为 GitHub Discussion 协议（见 docs/development-process.md 阶段 0.5）"
            jq '.channel="discussion"' "$DIR/$ID.json" > "$DIR/$ID.json.tmp" && mv "$DIR/$ID.json.tmp" "$DIR/$ID.json"
            echo "[confirm] 请把请求内容发到 Discussion #71 对应评论；用户回复后运行: confirm.sh reply --id $ID --action <approve|edit|reject> --text "...""
          fi
        fi
        ;;
      discussion)
        echo "[confirm] 请把请求内容（$DIR/$ID.json）发到 Discussion #71 对应评论；用户回复后运行: confirm.sh reply --id $ID --action <approve|edit|reject> --text "...""
        ;;
      none) echo "[confirm] 本地记录模式；模拟回复: confirm.sh reply --id $ID --action approve" ;;
      *) echo "未知 channel: $CH" >&2; exit 2 ;;
    esac
    ;;
  wait)
    ID=""; TIMEOUT_MIN=30; SLEEP=5
    while [ $# -gt 0 ]; do
      case "$1" in --id) ID="${2:-}"; shift 2 ;; --timeout-min) TIMEOUT_MIN="${2:-30}"; shift 2 ;; --sleep-sec) SLEEP="${2:-5}"; shift 2 ;; *) echo "未知参数: $1" >&2; usage ;; esac
    done
    require_id "$ID"
    REPLY_FILE="$DIR/$ID.reply.json"
    i=0; LIMIT=$(( TIMEOUT_MIN * 60 / SLEEP )); [ "$LIMIT" -lt 1 ] && LIMIT=1
    while [ $i -lt "$LIMIT" ]; do
      if [ -f "$REPLY_FILE" ]; then
        echo "[confirm] 收到回复：" >&2
        cat "$REPLY_FILE"
        exit 0
      fi
      sleep "$SLEEP"; i=$((i+1))
    done
    echo "[confirm] 等待超时（${TIMEOUT_MIN} 分钟），尚无回复。可继续等：confirm.sh wait --id ${ID}；或降级 Discussion 协议并停在这里。" >&2
    exit 3
    ;;
  reply)
    ID=""; ACTION=""; TEXT=""
    while [ $# -gt 0 ]; do
      case "$1" in --id) ID="${2:-}"; shift 2 ;; --action) ACTION="${2:-}"; shift 2 ;; --text) TEXT="${2:-}"; shift 2 ;; *) echo "未知参数: $1" >&2; usage ;; esac
    done
    require_id "$ID"
    case "$ACTION" in approve|edit|reject) ;; *) echo "--action 必须为 approve|edit|reject" >&2; usage ;; esac
    [ -f "$DIR/$ID.json" ] || { echo "请求不存在: $DIR/$ID.json" >&2; exit 4; }
    NOW="$(now_iso)"
    jq -nc --arg id "$ID" --arg a "$ACTION" --arg t "$TEXT" --arg n "$NOW"       '{id:$id, action:$a, text:$t, repliedAt:$n, source:"cli"}' > "$DIR/$ID.reply.json"
    jq '.status="answered" | .repliedAt=$n' --arg n "$NOW" "$DIR/$ID.json" > "$DIR/$ID.json.tmp" && mv "$DIR/$ID.json.tmp" "$DIR/$ID.json"
    echo "[confirm] 已写入回复: $DIR/$ID.reply.json"
    ;;
  status)
    ID="${1:-}"; require_id "$ID"
    echo "== $DIR/$ID.json"; cat "$DIR/$ID.json"
    echo; [ -f "$DIR/$ID.reply.json" ] && { echo "== $DIR/$ID.reply.json"; cat "$DIR/$ID.reply.json"; } || echo "(尚无回复)"
    ;;
  list)
    mkdir -p "$DIR"
    for f in "$DIR"/*.json; do
      [ -f "$f" ] || continue
      base="$(basename "$f")"
      case "$base" in feishu.json|feishu-token.json) continue ;; esac
      case "$base" in *.reply.json) continue ;; esac
      id="$(jq -r '.id // empty' "$f" 2>/dev/null)"
      [ -n "$id" ] || continue
      st="$(jq -r '.status // "?"' "$f" 2>/dev/null)"
      printf '%-8s %-10s %s\n' "$id" "$st" "$(basename "$f")"
    done
    ;;
  *) usage ;;
esac
