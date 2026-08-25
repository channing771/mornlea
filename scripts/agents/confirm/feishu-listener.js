#!/usr/bin/env node
'use strict';
// 飞书长连接监听器：接收用户对确认请求的回复，写入本地回复文件，并按配置自动续跑实现者。
// 用法: node feishu-listener.js           常驻监听（launchd 保活，见 docs/agents/confirmation-channel.md）
//       node feishu-listener.js --bootstrap  接收一条消息后把 open_id/chat_id 写入配置
// 依赖: @larksuiteoapi/node-sdk（scripts/agents/confirm/package.json，工具链依赖，与 Go 模块无关）

const fs = require('fs');
const os = require('os');
const path = require('path');
const { spawn } = require('child_process');
const { WSClient, Client, EventDispatcher } = require('@larksuiteoapi/node-sdk');

const DIR = process.env.MORNLEA_CONFIRM_DIR || path.join(os.homedir(), '.mornlea', 'confirm');
const CONFIG = path.join(DIR, 'feishu.json');
const BOOTSTRAP = process.argv.includes('--bootstrap');
const NL = String.fromCharCode(10);

function log(...a) { console.log(new Date().toISOString(), ...a); }
function die(msg) { console.error('[feishu-listener] ' + msg); process.exit(1); }

function loadConfig() {
  if (!fs.existsSync(CONFIG)) die('缺少配置 ' + CONFIG + '（按 docs/agents/confirmation-channel.md 配置飞书应用）');
  return JSON.parse(fs.readFileSync(CONFIG, 'utf8'));
}

function saveConfig(cfg) {
  fs.writeFileSync(CONFIG, JSON.stringify(cfg, null, 2) + NL, { mode: 0o600 });
}

function pendingRequests() {
  const files = fs.readdirSync(DIR).filter((f) => f.endsWith('.json') && f !== 'feishu.json' && f !== 'feishu-token.json');
  const list = [];
  for (const f of files) {
    try {
      const req = JSON.parse(fs.readFileSync(path.join(DIR, f), 'utf8'));
      if (req.status === 'pending') {
        req._file = path.join(DIR, f);
        list.push(req);
      }
    } catch (_) { /* 忽略损坏文件 */ }
  }
  list.sort((a, b) => (b.updatedAt || b.createdAt || '').localeCompare(a.updatedAt || a.createdAt || ''));
  return list;
}

function classifyReply(text, kind) {
  const t = text.toLowerCase();
  if (/驳回|拒绝|取消|reject|不行|不要|别做|❌/.test(t)) return 'reject';
  // 澄清提问轮（kind=question）：除驳回外一律视为答案（answer），由实现者继续分析；
  // 批准/同意同样算「答案」，避免在提问轮被误判成最终批准
  if (kind === 'question') return 'answer';
  if (/批准|同意|认可|ok|lgtm|approve|继续|可以|确认|✅|👍/.test(t) && !/不|别|勿|修改/.test(t)) return 'approve';
  return 'edit';
}

function writeReply(requestId, action, text, meta) {
  const reply = Object.assign({ id: requestId, action, text: text || '', repliedAt: new Date().toISOString() }, meta || {});
  const file = path.join(DIR, requestId + '.reply.json');
  fs.writeFileSync(file, JSON.stringify(reply, null, 2) + NL, { mode: 0o600 });
  const reqFile = path.join(DIR, requestId + '.json');
  if (fs.existsSync(reqFile)) {
    try {
      const req = JSON.parse(fs.readFileSync(reqFile, 'utf8'));
      req.status = 'answered';
      req.repliedAt = reply.repliedAt;
      fs.writeFileSync(reqFile, JSON.stringify(req, null, 2) + NL, { mode: 0o600 });
    } catch (_) { /* 忽略 */ }
  }
  log('已写入回复', file);
  return reply;
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// 回执 ack 是用户在飞书侧感知「已收到」的唯一反馈面：单次失败常见于瞬时限流/网络抖动，
// 有限重试（0.5s/1.5s 退避，共 3 次）并把真实原因写日志替代静默吞掉——
// 之前「一次丢 + catch 忽略」会让按钮点了没反馈，用户只能反复点按。
async function sendAck(cfg, openId, text) {
  const client = new Client({ appId: cfg.appId, appSecret: cfg.appSecret });
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      await client.im.message.create({
        params: { receive_id_type: 'open_id' },
        data: { receive_id: openId, msg_type: 'text', content: JSON.stringify({ text }) },
      });
      return;
    } catch (e) {
      if (attempt < 2) { await sleep(attempt === 0 ? 500 : 1500); continue; }
      log('ack 发送失败:', e && e.message);
    }
  }
}

var LOOP_GUARD = process.env.MORNLEA_LOOP_GUARD || path.join(os.homedir(), '.mornlea', 'loop.guard');

function spawnResume(cfg, request) {
  if (cfg.autoResume === false) { log('autoResume 已关闭，不自动续跑'); return; }
  const requestId = request && request.id;
  const repo = path.resolve(__dirname, '..', '..', '..');
  const cmd = cfg.resumeCmd || ('cd ' + repo + ' && scripts/agents/run-agent.sh implementer');
  // 接力循环：guard 存在说明有活动循环 → 继承 AGENT_LOOP=1，否则续跑会话不会在收尾触发 relay，链条会断。
  // 链守卫按链身份取：有 workerId 用 loop.guard.<workerId>，否则主链 loop.guard。
  const chainGuard = request && request.workerId ? path.join(os.homedir(), '.mornlea', 'loop.guard.' + request.workerId) : LOOP_GUARD;
  const inLoop = fs.existsSync(chainGuard);
  // 链身份：确认请求记录了发起链条的 tool/workerId（confirm.sh ask 写入），续跑必须保持同一链
  const chainTool = (request && request.workerTool) || 'claude';
  const chainId = (request && request.workerId) || '';
  const logFile = fs.openSync(path.join(DIR, 'resume-' + requestId + '.log'), 'a');
  const child = spawn('/bin/bash', ['-lc', cmd], {
    detached: true,
    stdio: ['ignore', logFile, logFile],
    env: Object.assign({}, process.env, { AGENT_RESUME: requestId, MORNLEA_CONFIRM_DIR: DIR, AGENT_LOOP: inLoop ? '1' : (process.env.AGENT_LOOP || '0'), AGENT_TOOL: chainTool, WORKER_ID: chainId }),
  });
  child.unref();
  log('已触发续跑(pid=' + child.pid + ')：' + cmd + ' (AGENT_RESUME=' + requestId + ', tool=' + chainTool + ', worker=' + chainId + ')');
}

function handleMessage(cfg, data) {
  const m = data.message;
  if (!m || m.message_type !== 'text') { log('忽略非文本消息:', m && m.message_type); return; }
  let text = '';
  try { text = JSON.parse(m.content).text || ''; } catch (e) { log('文本解析失败:', e.message); return; }
  const openId = data.sender && data.sender.sender_id && data.sender.sender_id.open_id;
  if (BOOTSTRAP) {
    if (!text || !openId) { log('等待一条来自你的消息以便记录 open_id/chat_id'); return; }
    const recv = cfg.receive && cfg.receive.id ? cfg.receive : { type: 'open_id', id: openId };
    cfg.receive = recv;
    cfg.chatId = m.chat_id;
    saveConfig(cfg);
    log('已捕获 receive=' + recv.type + ':' + recv.id + ' chat_id=' + m.chat_id + '，请检查 ' + CONFIG + ' 后重启监听');
    process.exit(0);
  }
  // 匹配请求优先级：①「回复」某条卡片消息（parent_id 精确锁定，最不易错）
  // ② 文本带 #ID ③ 只有一个 pending ④ 多个 pending 且无法精确匹配 → 不猜，提示用户指明
  const pending = pendingRequests();
  let target = null;
  if (m.parent_id) target = pending.find((p) => p.feishuMessageId === m.parent_id) || null;
  if (!target) {
    const m2 = text.match(/#([A-F]-\d{2})/);
    if (m2) target = pending.find((p) => p.id === m2[1]) || null;
  }
  if (!target && pending.length === 1) target = pending[0];
  if (!target) {
    if (pending.length > 1) {
      log('多 pending 且无精确匹配（parent_id/#ID），忽略并提示（文本:' + text + '）');
      if (openId) sendAck(cfg, openId, '有 ' + pending.length + ' 个待确认请求：' + pending.map((p) => p.id).join('、') + '。请点按对应卡片的按钮，或「回复」那条卡片消息，或回复时带 #' + pending[0].id + ' 之类的编号。');
    } else { log('没有匹配的待确认请求（文本:' + text + '），忽略'); }
    return;
  }
  if (target.status !== 'pending') {
    log('请求 ' + target.id + ' 已答复，忽略回复（文本:' + text + '）');
    // 重复点按也要有反馈——「已答复过」提示避免用户以为丢了、继续点
    if (openId) sendAck(cfg, openId, '「' + target.id + '」已答复过，无需重复；实现者正在继续。');
    return;
  }
  const action = classifyReply(text, target.kind || 'approval');
  const reply = writeReply(target.id, action, text, { senderOpenId: openId, chatId: m.chat_id, messageId: m.message_id });
  log('#HANDLED ' + target.id + ' action=' + action);
  // 先本地续跑（快、关键路径），再回执 ack（外部网络、可慢可失败）——
  // 顺序反了会让「续跑」被 ack 的网络延迟拖着
  spawnResume(cfg, target);
  if (openId) { const label = (target.kind === 'question' ? '提问' : '确认') + '已收到（' + action + '）'; sendAck(cfg, openId, label + '，实现者将从 ' + target.id + '.reply.json 继续。'); }
}

// 卡片按钮回调：Feishu 回调配置（事件与回调→回调配置）开启后，按钮事件经同一长连接以
// card.action.trigger 送达。按钮 value（{id, action, text}）在回调里是 JSON 字符串。
function cardEventData(data) {
  const ev = (data && data.event) || data || {};
  const action = ev.action || {};
  const ctx = ev.context || {};
  const raw = action.value !== undefined ? action.value : (ev.action_value !== undefined ? ev.action_value : '');
  let value = raw;
  if (typeof raw === 'string') { try { value = JSON.parse(raw); } catch (_) { /* 保留原串 */ } }
  // 手动输入区（form 容器）：点「发送」时回调携带 form_value（JSON 字符串，键为 input 的 name）
  const rawForm = action.form_value !== undefined ? action.form_value : (ev.form_value !== undefined ? ev.form_value : '');
  let formValue = rawForm;
  if (typeof rawForm === 'string' && rawForm) { try { formValue = JSON.parse(rawForm); } catch (_) { /* 保留原串 */ } }
  return {
    operator: (ev.operator && ev.operator.open_id) || ev.operator_id || '',
    messageId: ctx.message_id || ctx.open_message_id || ev.message_id || '',
    chatId: ctx.chat_id || ev.chat_id || '',
    value,
    formValue: (formValue && typeof formValue === 'object') ? formValue : {},
  };
}

function handleCardAction(cfg, data) {
  const ev = cardEventData(data);
  const value = (ev.value && typeof ev.value === 'object') ? ev.value : {};
  const pending = pendingRequests();
  // 按 value.id 精确匹配；无 id 时用 message_id 反查该卡片对应的请求
  let target = value.id ? pending.find((p) => p.id === value.id) || null : null;
  if (!target && ev.messageId) target = pending.find((p) => p.feishuMessageId === ev.messageId) || null;
  if (!target) {
    log('卡片按钮无匹配请求（value=' + JSON.stringify(value) + '），忽略');
    // 按钮回调到达但匹配不到（重复点击时请求已从 pending 移除、或消息 id 未记录）→
    // 给点按者一个有反馈的提示，否则点按后无声无息，用户只会再点
    if (ev.operator) sendAck(cfg, ev.operator, '未能匹配待确认请求：请点按对应卡片的按钮，或回复「#编号」；已答复过的请忽略。');
    return;
  }
  if (target.status !== 'pending') {
    log('请求 ' + target.id + ' 已答复，忽略按钮（value=' + JSON.stringify(value) + '）');
    if (ev.operator) sendAck(cfg, ev.operator, '「' + target.id + '」已答复过，无需重复点按；实现者正在继续。');
    return;
  }
  // 手动输入区提交：以输入框文本为准，按文本规则判定动作（批准类：关键词→批准，其他→修改意见；提问类→答案）
  const manualText = (ev.formValue && ev.formValue.note) ? String(ev.formValue.note).trim() : '';
  let action = value.action || classifyReply(String(value.text || ''), target.kind || 'approval');
  let text = manualText || (value.text !== undefined ? String(value.text) : '');
  if (manualText && action === 'manual') action = classifyReply(manualText, target.kind || 'approval');
  const reply = writeReply(target.id, action, text, { senderOpenId: ev.operator, chatId: ev.chatId, messageId: ev.messageId });
  log('#HANDLED-CARD ' + target.id + ' action=' + action);
  // 同 handleMessage：先本地续跑再回执（ack 网络延迟不拖实现者）
  spawnResume(cfg, target);
  if (ev.operator) { const label = (target.kind === 'question' ? '提问' : '确认') + '已收到（' + action + '）'; sendAck(cfg, ev.operator, label + '，实现者将从 ' + target.id + '.reply.json 继续。'); }
}

async function main() {
  // let 而非 const：事件回调里会 cfg = loadConfig() 按事件重载（见下方注释），const 会在首条消息即抛 TypeError。
  let cfg = loadConfig();
  if (!cfg.appId || !cfg.appSecret) die('配置缺 appId/appSecret');
  log('启动监听（bootstrap=' + BOOTSTRAP + '；receive=' + JSON.stringify(cfg.receive || '未设置') + '）');
  const ws = new WSClient({
    appId: cfg.appId,
    appSecret: cfg.appSecret,
    onReady: () => log('长连接已就绪'),
    onError: (e) => log('长连接错误:', e && e.message),
    onReconnecting: () => log('断线重连中…'),
    onReconnected: () => log('重连成功'),
  });
  // SDK 要求 EventDispatcher 实例（不能传裸对象）。im.message.receive_v1 收文本回复；
  // card.action.trigger 收卡片按钮点按（需在飞书开发者后台「事件与回调→回调配置」开启）。
  const dispatcher = new EventDispatcher({}).register({
    // 每个事件重载配置（loadConfig）：autoResume 等键可能被人工/脚本中途修改，
    // 缓存启动时快照会导致「配置已改但行为不变」的滞留（曾吞掉两次续跑）。
    'im.message.receive_v1': (data) => { try { cfg = loadConfig(); handleMessage(cfg, data); } catch (e) { log('处理消息异常:', e && e.stack || e); } },
    'card.action.trigger': (data) => { try { cfg = loadConfig(); handleCardAction(cfg, data); } catch (e) { log('处理卡片回调异常:', e && e.stack || e); } },
  });
  process.on('SIGINT', () => { log('收到 SIGINT，关闭'); try { ws.close(); } catch (e) {} process.exit(0); });
  process.on('SIGTERM', () => { log('收到 SIGTERM，关闭'); try { ws.close(); } catch (e) {} process.exit(0); });
  await ws.start({ eventDispatcher: dispatcher });
}

main().catch((e) => die(e && e.stack || e));
