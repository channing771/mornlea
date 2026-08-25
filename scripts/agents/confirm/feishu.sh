#!/usr/bin/env bash
# 飞书应用消息发送器：把确认请求以交互卡片推送到你的设备。
# 配置: ~/.mornlea/confirm/feishu.json（见 docs/agents/confirmation-channel.md）
# 用法: feishu.sh send <请求ID>
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
  resp="$(curl -sS --max-time 15 -X POST "https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal" \
    -H "Content-Type: application/json" \
    -d "$(jq -nc --arg a "$APP_ID" --arg s "$APP_SECRET" '{app_id:$a, app_secret:$s}')")"
  tok="$(echo "$resp" | jq -r '.tenant_access_token // empty')"
  [ -n "$tok" ] || die "获取 tenant_access_token 失败: $(echo "$resp" | jq -c .)"
  echo "$resp" | jq --arg tok "$tok" --argjson exp "$(( $(date +%s) + 7000 ))" '{tenant_access_token: $tok, expiresAt: $exp}' > "$TOKEN_FILE"
  echo "$tok"
}

send_card() { # $1 = 卡片 JSON（jq -nc 生成）——interactive 卡片比纯文本清晰
  local tok recv_id resp msg_id
  tok="$(get_token)" || return 1
  recv_id="$RECV_ID"
  resp="$(curl -sS --max-time 15 -X POST "https://open.feishu.cn/open-apis/im/v1/messages?receive_id_type=$RECV_TYPE" \
    -H "Authorization: Bearer $tok" -H "Content-Type: application/json" \
    -d "$(jq -nc --arg rid "$recv_id" --arg c "$1" '{receive_id:$rid, msg_type:"interactive", content:$c}')")"
  msg_id="$(echo "$resp" | jq -r '.data.message_id // empty')"
  [ -n "$msg_id" ] || die "发送消息失败: $(echo "$resp" | jq -c .)"
  echo "ok message_id=$msg_id"
}

# 把 agent 写的大段短设计按句拆成条目行，卡片里更易读
fmt_design() {
  python3 -c '
import sys, re
text = sys.stdin.read()
parts = [p.strip() for p in re.split(r"[；。]|\n", text) if p.strip()]
if len(parts) > 8:
    parts = parts[:8] + ["…（更多细节见请求文件）"]
for p in parts:
    print("- " + p)
' | sed 's/^- - /- /' || true
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
    KIND="$(jq -r '.kind // "approval"' "$REQ")"
    if [ "$KIND" = "question" ]; then
      HEAD_PREFIX="澄清提问："
      NOTE="请直接回复你的答案（带 #$ID 可精确指定该提问）；回答后实现者会继续分析，可能还有后续提问"
    else
      HEAD_PREFIX="内容确认："
      NOTE="请回复：✅ 批准（或直接回复修改意见；带 #$ID 可精确指定该任务）"
    fi
    DESIGN_BODY="$(printf '%s' "$DESIGN" | fmt_design)"
    # 选项数组原样传入（每个选项渲染为独立编号行；无选项则不渲染该区）
    CARD="$(jq -nc --arg id "$ID" --arg t "$TITLE" --arg c "$CATEGORY" --arg q "$QUESTION" --argjson o "$(jq -c '.options // []' "$REQ")" --arg d "$DESIGN_BODY" --arg hp "$HEAD_PREFIX" --arg note "$NOTE" '{
  config: { wide_screen_mode: true },
  header: { template: "blue", title: { tag: "plain_text", content: ($hp + $id + " " + $t) } },
  elements: [
    { tag: "div", text: { tag: "lark_md", content: (
      "**分类**：" + $c + "\n\n**问题**：\n" + $q
      + ($o | if length == 0 then "" else "\n\n**选项**：\n" + (to_entries | map("- " + (.key + 1 | tostring) + ". " + .value) | join("\n")) end)
      + ($d | if . == "" then "" else "\n\n**短设计**：\n" + . end)
    ) } },
    { tag: "hr" },
    { tag: "note", elements: [ { tag: "plain_text", content: $note } ] }
  ]
}')"
    send_card "$CARD"
    ;;
  *)
    echo "用法: feishu.sh send <请求ID>" >&2; exit 2 ;;
esac