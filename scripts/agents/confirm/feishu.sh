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
      HEAD_PREFIX="澄清提问"; TPL="orange"
      NOTE=""
    else
      HEAD_PREFIX="内容确认"; TPL="blue"
      NOTE=""
    fi
    # 底部说明默认不展示（卡片保持干净）；仅选项超过 5 个时显示提示
    DESIGN_BODY="$(printf '%s' "$DESIGN" | fmt_design)"
    # 选项渲染为按钮 + 正文完整列表：按钮只作快捷路径（label 去掉编号前缀、截断加省略号，
    # value 仍携带完整选项文本与请求 ID，listener 按 id 精确匹配），**全部**选项永远在正文
    # 「选项」段完整列出——之前按钮截断加省略号、且隐藏的选项正文也没有，用户看不到全貌。
    if [ "$KIND" = "question" ]; then
      # 正文完整列表：每行「编号. 完整文本」，不截断（选项可能很长）
      OPT_TEXT="$(jq -r 'if (.options // []) | length == 0 then "" else (.options | to_entries | map((.key + 1 | tostring) + ". " + .value) | join("\n")) end' "$REQ")"
      ACTIONS="$(jq -nc --arg id "$ID" --argjson o "$(jq -c '.options // []' "$REQ")" '
        [ (($o | to_entries | map({
            tag: "button",
            text: { tag: "plain_text", content: ((.key + 1 | tostring) + ". " + (.value | sub("^[A-Za-z0-9]\\s*[.、]\\s*"; "") | .[0:22]) + (if (.value | length) > 24 then "…" else "" end)) },
            type: (if .key == 0 then "primary" else "default" end),
            value: { id: $id, action: "answer", text: .value }
          })) | .[0:5])[],
          { tag: "button", text: { tag: "plain_text", content: "驳回" }, type: "danger", value: { id: $id, action: "reject", text: "驳回" } }
        ]')"
      [ "$(jq -r '.options | length // 0' "$REQ")" -gt 5 ] && NOTE="选项共 $(jq -r '.options | length' "$REQ") 项，已完整列出在上方；点对应按钮，或回复「编号」即可"
    else
      ACTIONS="$(jq -nc --arg id "$ID" '[
        { tag: "button", text: { tag: "plain_text", content: "✅ 批准" }, type: "primary", value: { id: $id, action: "approve", text: "批准" } },
        { tag: "button", text: { tag: "plain_text", content: "❌ 驳回" }, type: "danger", value: { id: $id, action: "reject", text: "驳回" } }
      ]')"
    fi
    # 卡片结构（分层，不再堆一个 div）：标题行 → 问题/短设计 → 按钮 → 手动输入区 → 底部说明；
    # form 提交按钮为 Card 2.0 写法（name + form_action_type=submit），回调带 form_value（JSON 字符串，键=note）。
    CARD="$(jq -nc --arg id "$ID" --arg t "$TITLE" --arg q "$QUESTION" --argjson o "$(jq -c '.options // []' "$REQ")" --argjson actions "$ACTIONS" --arg d "$DESIGN_BODY" --arg hp "$HEAD_PREFIX" --arg tpl "$TPL" --arg note "$NOTE" --arg opt "${OPT_TEXT:-}" '{
  config: { wide_screen_mode: true },
  header: { template: $tpl, title: { tag: "plain_text", content: ($hp + " · " + $id) } },
  elements: [
    { tag: "div", text: { tag: "lark_md", content: (
      "**" + $t + "**\n\n**问题**\n" + $q
      + ($d | if . == "" then "" else "\n\n**短设计**\n" + . end)
      + ($opt | if . == "" then "" else "\n\n**选项**\n" + . end)
    ) } },
    { tag: "action", actions: $actions },
    { tag: "form", name: "manual", elements: [
      { tag: "input", name: "note",
        label: { tag: "plain_text", content: "修改意见 / 其他答案" },
        placeholder: { tag: "plain_text", content: "写在这里，点「发送」提交" },
        is_required: false, max_length: 512 },
      { tag: "button", name: "submit", form_action_type: "submit", text: { tag: "plain_text", content: "发送" }, type: "primary" }
    ] },
    (($note | if . == "" then [] else [ { tag: "note", elements: [ { tag: "plain_text", content: $note } ] } ] end)[])
  ]
}')"
    if SEND_OUT="$(send_card "$CARD")"; then
      # 记录下发消息的 message_id：listener 用「回复该消息」的 parent_id 精确反查请求
      MID="$(printf '%s' "$SEND_OUT" | sed -n 's/^ok message_id=//p')"
      [ -n "$MID" ] && jq --arg mid "$MID" '.feishuMessageId=$mid' "$REQ" > "$REQ.tmp" && mv "$REQ.tmp" "$REQ"
      echo "$SEND_OUT"
    else
      exit 1
    fi
    ;;
  *)
    echo "用法: feishu.sh send <请求ID>" >&2; exit 2 ;;
esac