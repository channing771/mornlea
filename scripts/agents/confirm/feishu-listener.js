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

async function sendAck(cfg, openId, text) {
  try {
    const client = new Client({ appId: cfg.appId, appSecret: cfg.appSecret });
    await client.im.message.create({
      params: { receive_id_type: 'open_id' },
      data: { receive_id: openId, msg_type: 'text', content: JSON.stringify({ text }) },
    });
  } catch (e) { log('ack 发送失败(忽略):', e && e.message); }
}

var LOOP_GUARD = process.env.MORNLEA_LOOP_GUARD || path.join(os.homedir(), '.mornlea', 'loop.guard');

function spawnResume(cfg, requestId) {
  if (cfg.autoResume === false) { log('autoResume 已关闭，不自动续跑'); return; }
  const repo = path.resolve(__dirname, '..', '..', '..');
  const cmd = cfg.resumeCmd || ('cd ' + repo + ' && scripts/agents/run-agent.sh implementer');
  // 接力循环：guard 存在说明有活动循环 → 继承 AGENT_LOOP=1，否则续跑会话不会在收尾触发 relay，链条会断
  const inLoop = fs.existsSync(LOOP_GUARD);
  const logFile = fs.openSync(path.join(DIR, 'resume-' + requestId + '.log'), 'a');
  const child = spawn('/bin/bash', ['-lc', cmd], {
    detached: true,
    stdio: ['ignore', logFile, logFile],
    env: Object.assign({}, process.env, { AGENT_RESUME: requestId, MORNLEA_CONFIRM_DIR: DIR, AGENT_LOOP: inLoop ? '1' : (process.env.AGENT_LOOP || '0') }),
  });
  child.unref();
  log('已触发续跑(pid=' + child.pid + ')：' + cmd + ' (AGENT_RESUME=' + requestId + ')');
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
  // 匹配请求：文本带 #ID 则精确指定；否则取最新 pending
  const m2 = text.match(/#([A-F]-\d{2})/);
  const pending = pendingRequests();
  const target = m2 ? pending.find((p) => p.id === m2[1]) : pending[0];
  if (!target) { log('没有匹配的待确认请求（文本:' + text + '），忽略'); return; }
  const action = classifyReply(text, target.kind || 'approval');
  const reply = writeReply(target.id, action, text, { senderOpenId: openId, chatId: m.chat_id, messageId: m.message_id });
  log('#HANDLED ' + target.id + ' action=' + action);
  if (openId) { const label = (target.kind === 'question' ? '提问' : '确认') + '已收到（' + action + '）'; sendAck(cfg, openId, label + '，实现者将从 ' + target.id + '.reply.json 继续。'); }
  spawnResume(cfg, target.id);
}

async function main() {
  const cfg = loadConfig();
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
  // SDK 要求 EventDispatcher 实例（不能传裸对象），事件键为 im.message.receive_v1
  const dispatcher = new EventDispatcher({}).register({
    'im.message.receive_v1': (data) => { try { handleMessage(cfg, data); } catch (e) { log('处理消息异常:', e && e.stack || e); } },
  });
  process.on('SIGINT', () => { log('收到 SIGINT，关闭'); try { ws.close(); } catch (e) {} process.exit(0); });
  process.on('SIGTERM', () => { log('收到 SIGTERM，关闭'); try { ws.close(); } catch (e) {} process.exit(0); });
  await ws.start({ eventDispatcher: dispatcher });
}

main().catch((e) => die(e && e.stack || e));
