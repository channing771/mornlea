#!/usr/bin/env bash
# 飞书应用消息发送器：把确认请求推送到你的设备。
# 配置: ~/.mornlea/confirm/feishu.json（见 docs/agents/confirmation-channel.md）
# 用法: feishu.sh send <请求ID>    （文本由 confirm.sh 从请求文件组装）
set -euo pipefail

DIR="${MORNLEA_CONFIRM_DIR:-$HOME/.mornlea/confirm}"
CONFIG="$DIR/feishu.json"

die() { echo "[feishu] $*" >&2; exit 1; }
[ -f "$CONFIG" ] || die "缺少配置 ${CONFIG}（先按 docs/agents/confirmation-channel.md 配置飞书应用）"

APP_ID="$(jq -r '.appId // .app_id // empty' "$CONFIG")"
APP_SECRET="$(jq -r '.appSecret // .app_secret // empty' "$CONFIG")"
RECV_TYPE="$(jq -r '.receive.type // .receive_type // empty' "$CONFIG")"
RECV_ID="$(jq -r '.receive.id // .receive_id // empty' "$CONFIG")"
[ -n "$APP_ID" ] && [ -n "$APP_SECRET" ] || die "feishu.json 缺 appId/appSecret"
[ -n "$RECV_TYPE" ] && [ -n "$RECV_ID" ] || die "feishu.json 缺 receive.type/receive.id（先用 feishu-listener.js --bootstrap 或按文档填写）"

TOKEN_FILE="$DIR/feishu-token.json"
get_token() {
  # 缓存有效期内直接复用；空/损坏缓存视为过期（jq 失败时按 0 处理）
  if [ -f "$TOKEN_FILE" ]; then
    local exp
    exp="$(jq -r '.expiresAt // 0' "$TOKEN_FILE" 2>/dev/null || echo 0)"
    if [ "$exp" -gt "$(date +%s)" ]; then
      local cached
      cached="$(jq -r '.tenant_access_token // .token // empty' "$TOKEN_FILE" 2>/dev/null || true)"
      if [ -n "$cached" ]; then echo "$cached"; return; fi
    fi
  fi
  local resp tok
  resp="$(curl -sS --max-time 15 -X POST 'https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal'     -H 'Content-Type: application/json'     -d "$(jq -nc --arg a "$APP_ID" --arg s "$APP_SECRET" '{app_id:$a, app_secret:$s}')")"
  tok="$(echo "$resp" | jq -r '.tenant_access_token // empty')"
  [ -n "$tok" ] || die "获取 tenant_access_token 失败: $(echo "$resp" | jq -c .)"
  echo "$resp" | jq --arg tok "$tok" --argjson exp "$(( $(date +%s) + 7000 ))" '{tenant_access_token: $tok, expiresAt: $exp}' > "$TOKEN_FILE"
  echo "$tok"
}

send_text() { # $1 = 消息文本
  local tok recv_id resp msg_id
  tok="$(get_token)" || return 1
  recv_id="$RECV_ID"
  resp="$(curl -sS --max-time 15 -X POST "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=$RECV_TYPE" \
    -H "Authorization: Bearer $tok" -H 'Content-Type: application/json' \
    -d "$(jq -nc --arg rid "$recv_id" --arg c "$(jq -nc --arg t "$1" '{text:$t}')" '{receive_id:$rid, msg_type:"text", content:$c}')")"
  msg_id="$(echo "$resp" | jq -r '.data.message_id // empty')"
  [ -n "$msg_id" ] || die "发送消息失败: $(echo "$resp" | jq -c .)"
  echo "ok message_id=$msg_id"
}

case "${1:-}" in
  send)
    ID="${2:?用法: feishu.sh send <请求ID>}"
    REQ="$DIR/$ID.json"
    [ -f "$REQ" ] || die "找不到请求文件 ${REQ}（先 confirm.sh ask）"
    TITLE="$(jq -r '.title // .id // ""' "$REQ")"
    CATEGORY="$(jq -r '.category // ""' "$REQ")"
    QUESTION="$(jq -r '.question // ""' "$REQ")"
    DESIGN="$(jq -r '.design // ""' "$REQ")"
    # 用 printf 生成真实换行：字面 \n 会被 jq 再转义成 \\n，飞书端显示为「\n」
    TEXT="$(printf '【Mornlea 内容确认】%s\n分类：%s\n问题：%s\n短设计：%s\n请回复：✅ 批准（或回复修改意见；精确指定请加 #%s）' "$TITLE" "$CATEGORY" "$QUESTION" "$DESIGN" "$ID")"
    send_text "$TEXT"
    ;;
  *)
    echo "用法: feishu.sh send <请求ID>" >&2; exit 2 ;;
esac
