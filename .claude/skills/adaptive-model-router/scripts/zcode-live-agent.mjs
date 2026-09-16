import { spawn, spawnSync } from "node:child_process";
import { createServer, connect } from "node:net";
import { closeSync, existsSync, mkdirSync, openSync, readFileSync, readdirSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import { chmod } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import os from "node:os";
import path from "node:path";
import { randomUUID } from "node:crypto";

const modulePath = fileURLToPath(import.meta.url);

const MAX_FRAME = 1024 * 1024;
const MAX_EVENTS = 256;
const MAX_EVENT_BYTES = 64 * 1024;
const PROVIDER_ID = "builtin:bigmodel-coding-plan";
const MODEL_ID = "GLM-5.3";
const MODES = new Set(["plan", "build", "edit", "yolo"]);
const RETRYABLE_SOCKET_ERRORS = new Set(["ENOENT", "ECONNREFUSED"]);
const COALESCED_EVENTS = new Set([
  "message.removed",
  "message.upserted",
  "model.streaming",
  "part.delta",
  "part.removed",
  "part.started",
  "part.upserted",
]);
const SECRET = /api[-_]?key|token|secret|authorization|password|baseurl|base_url/i;
const WORKER_ID = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;

export function defaultRuntimeDir() {
  const override = process.env.MORNLEA_ZCODE_RUNTIME_DIR;
  if (override && path.isAbsolute(override)) return path.resolve(override);
  return path.join(os.tmpdir(), `mornlea-zcode-live-${process.getuid?.() ?? "unknown"}`);
}

function safe(value, depth = 0, secrets = []) {
  if (depth > 4) return "[truncated]";
  if (typeof value === "string") return redact(value, secrets).slice(0, 2048);
  if (value === null || typeof value !== "object") return value;
  if (Array.isArray(value)) return value.slice(0, 20).map((item) => safe(item, depth + 1, secrets));
  return Object.fromEntries(Object.entries(value).filter(([key]) => !SECRET.test(key)).slice(0, 40).map(([key, item]) => [key, safe(item, depth + 1, secrets)]));
}

function redact(value, secrets = []) {
  let result = String(value)
    .replace(/(?:Bearer\s+)?(?:sk-|AIza)[A-Za-z0-9._-]+/gi, "[redacted]")
    .replace(/secret[-_a-z0-9.]*/gi, "[redacted]");
  for (const secret of secrets) if (typeof secret === "string" && secret) result = result.split(secret).join("[redacted]");
  return result;
}

function error(code, message, secrets = []) {
  const result = new Error(redact(message, secrets));
  result.code = code;
  return result;
}

function provesMethodPresence(cause) {
  const code = cause?.remoteCode;
  return code !== undefined && code !== -32601 && code !== "-32601";
}

function isToolFailure(input) {
  const states = [
    input?.status,
    input?.state,
    input?.tool?.status,
    input?.tool?.state,
    input?.result?.status,
  ];
  return states.some((value) => /fail|error|denied|cancel/i.test(String(value ?? "")));
}

function extractCredentials(configPath) {
  let config;
  try { config = JSON.parse(readFileSync(configPath, "utf8")); } catch { throw error("config_invalid", "ZCode config is unavailable"); }
  const providers = config.provider ?? config.providers ?? {};
  const provider = providers[PROVIDER_ID];
  const options = provider?.options ?? {};
  const baseURL = options.baseURL ?? options.baseUrl;
  const model = provider?.models?.[MODEL_ID];
  if (provider?.enabled === false || !model) throw error("provider_invalid", `ZCode provider ${PROVIDER_ID} or model ${MODEL_ID} is unavailable`);
  if (!options.apiKey || !baseURL) throw error("credentials_missing", "ZCode credentials or base URL are unavailable");
  const variants = model.reasoning?.variants;
  const reasoningLevels = Array.isArray(variants) ? variants : variants && typeof variants === "object" ? Object.keys(variants) : [];
  const reasoningDefault = model.reasoning?.defaultVariant ?? reasoningLevels[0];
  if (!reasoningDefault || !reasoningLevels.includes(reasoningDefault)) throw error("reasoning_invalid", "ZCode model reasoning variants are unavailable");
  return { apiKey: options.apiKey, baseURL, provider: PROVIDER_ID, model: MODEL_ID, reasoningDefault, reasoningLevels };
}

class AppServer {
  constructor(child, onEvent, onFailure, secrets = []) {
    this.child = child; this.onEvent = onEvent; this.onFailure = onFailure; this.secrets = secrets; this.pending = new Map(); this.buffer = ""; this.next = 1;
    this.exited = new Promise((resolve) => child.once("exit", resolve));
    child.stdout.setEncoding("utf8");
    child.stdout.on("data", (chunk) => this.#read(chunk));
    child.on("error", (cause) => this.#fail(error("child_error", cause.message)));
    child.on("exit", (code, signal) => this.#fail(error("child_exit", `app-server exited (${code ?? signal ?? "unknown"})`)));
  }
  #read(chunk) {
    this.buffer += chunk;
    if (this.buffer.length > MAX_FRAME) return this.#fail(error("protocol_frame_too_large", "app-server frame exceeds limit"));
    while (this.buffer.includes("\n")) {
      const point = this.buffer.indexOf("\n"); const line = this.buffer.slice(0, point); this.buffer = this.buffer.slice(point + 1);
      if (!line.trim()) continue;
      let frame; try { frame = JSON.parse(line); } catch { this.#fail(error("protocol_malformed", "app-server sent malformed JSON")); return; }
      if (frame.id !== undefined) { const pending = this.pending.get(String(frame.id)); if (!pending) continue; this.pending.delete(String(frame.id)); if (frame.error) { const cause = error("upstream_error", frame.error.message ?? "app-server error", this.secrets); cause.remoteCode = frame.error.code; pending.reject(cause); } else pending.resolve(frame.result); }
      else if (frame.method === "session/event") this.onEvent(frame.params?.event ?? frame.params ?? {});
    }
  }
  #fail(cause) { if (this.failed) return; this.failed = cause; for (const pending of this.pending.values()) pending.reject(cause); this.pending.clear(); this.onFailure(cause); }
  request(method, params, timeoutMs = 5000) {
    if (this.failed) return Promise.reject(this.failed);
    const id = String(this.next++); const frame = JSON.stringify({ id, method, params });
    if (frame.length > MAX_FRAME) return Promise.reject(error("request_too_large", "app-server request exceeds limit"));
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { this.pending.delete(id); reject(error("upstream_timeout", `${method} timed out`)); }, timeoutMs);
      this.pending.set(id, { resolve: (value) => { clearTimeout(timer); resolve(value); }, reject: (cause) => { clearTimeout(timer); reject(cause); } });
      this.child.stdin.write(`${frame}\n`);
    });
  }
  async close() {
    if (this.closing) return this.closing;
    const closing = (async () => {
      if (this.child.exitCode !== null || this.child.signalCode !== null) return;
      this.child.kill("SIGTERM");
      const graceful = await Promise.race([this.exited.then(() => true), new Promise((resolve) => setTimeout(() => resolve(false), 1200))]);
      if (graceful) return;
      this.child.kill("SIGKILL");
      const forced = await Promise.race([this.exited.then(() => true), new Promise((resolve) => setTimeout(() => resolve(false), 500))]);
      if (!forced) throw error("process_cleanup_timeout", "owned ZCode worker did not exit after termination");
    })();
    this.closing = closing.catch((cause) => { this.closing = undefined; throw cause; });
    return this.closing;
  }
}

export function createLiveSupervisor(options = {}) {
  const runtimeDir = options.runtimeDir ?? defaultRuntimeDir();
  const workersDir = path.join(runtimeDir, "workers");
  mkdirSync(workersDir, { recursive: true, mode: 0o700 });
  try { chmod(workersDir, 0o700); } catch { /* best effort before socket exists */ }
  const workers = new Map(); const stale = new Map();
  for (const name of readdirSync(workersDir)) {
    if (!name.endsWith(".json")) continue;
    try { const metadataPath = path.join(workersDir, name); const metadata = JSON.parse(readFileSync(metadataPath, "utf8")); if (WORKER_ID.test(metadata.workerId) && name === `${metadata.workerId}.json`) stale.set(metadata.workerId, { ...metadata, metadataPath }); } catch { /* corrupt metadata is ignored */ }
  }
  function metadata(worker) {
    return { workerId: worker.workerId, sessionId: worker.sessionId, cwd: worker.cwd, mode: worker.mode, provider: worker.provider, model: worker.model, reasoning: worker.reasoning, lifecycle: worker.lifecycle, cursor: worker.cursor, upstreamSeq: worker.upstreamSeq, bornAt: worker.bornAt, processPid: worker.processPid, processIdentityToken: worker.processIdentityToken };
  }
  function persist(worker) {
    // A closing process may race test/runtime cleanup; losing transient metadata is safe.
    try { writeFileSync(path.join(workersDir, `${worker.workerId}.json`), JSON.stringify(metadata(worker)), { mode: 0o600 }); } catch {}
  }
  function snapshot(worker) { return { lifecycle: worker.lifecycle, sessionId: worker.sessionId, upstreamSeq: worker.upstreamSeq, activity: { deltaCount: worker.deltas, lastAt: worker.lastDeltaAt }, lastError: worker.lastError }; }
  function material(worker, type, payload = {}) {
    const item = { cursor: ++worker.cursor, type, upstreamSeq: worker.upstreamSeq, at: Date.now(), payload: safe(payload, 0, worker.secrets) };
    let itemBytes = Buffer.byteLength(JSON.stringify(item));
    if (itemBytes > MAX_EVENT_BYTES) { item.payload = { truncated: true }; itemBytes = Buffer.byteLength(JSON.stringify(item)); }
    worker.events.push(item); worker.eventBytes += itemBytes;
    while (worker.events.length > MAX_EVENTS || worker.eventBytes > MAX_EVENT_BYTES) { const removed = worker.events.shift(); worker.eventBytes -= Buffer.byteLength(JSON.stringify(removed)); }
    persist(worker);
    for (const waiter of worker.waiters.splice(0)) { clearTimeout(waiter.timer); waiter.resolve(waitResult(worker, waiter.cursor, "event")); }
  }
  function fail(worker, cause) { if (worker.closed || worker.closing || worker.lifecycle === "failed") return; worker.lifecycle = "failed"; worker.lastError = { code: cause.code ?? "upstream_failure", message: redact(cause.message, worker.secrets) }; material(worker, "protocol.failure", worker.lastError); }
  function event(worker, input) {
    const sequence = input.seq;
    if (!Number.isInteger(sequence) || sequence < 0) return fail(worker, error("protocol_sequence", "app-server event lacks a valid sequence"));
    if (sequence < worker.upstreamSeq) return fail(worker, error("protocol_sequence_regression", "app-server event sequence regressed"));
    if (sequence === worker.upstreamSeq) return;
    if (sequence > worker.upstreamSeq + 1) return fail(worker, error("protocol_sequence_gap", "app-server event sequence skipped progress"));
    worker.upstreamSeq = sequence;
    if (COALESCED_EVENTS.has(input.type) || /(?:^|[._])(?:text|reasoning|tool_input)?_?(?:start|delta|end)$/i.test(String(input.type ?? "")) || input.type === "tool.updated" && !isToolFailure(input)) { worker.deltas++; worker.lastDeltaAt = Date.now(); return; }
    if (input.type === "turn.failed") worker.lifecycle = "failed";
    else if (input.type === "turn.completed") worker.lifecycle = "idle";
    else if (input.type === "turn.started") worker.lifecycle = "running";
    material(worker, input.type ?? "session.event", input);
  }
  function requireWorker(id) { const worker = workers.get(id); if (worker) return worker; if (stale.has(id)) return null; throw error("worker_not_found", `unknown worker ${id}`); }
  function processCommand(pid) { const result = spawnSync("ps", ["-ww", "-p", String(pid), "-o", "command="], { encoding: "utf8" }); return result.status === 0 ? result.stdout.trim() : ""; }
  function ownedWorkerProcess(metadata) {
    const command = processCommand(metadata.processPid);
    return command.includes(modulePath) && command.includes(" worker ") && command.includes(String(metadata.processIdentityToken ?? ""));
  }
  async function closeStale(metadata) {
    if (!Number.isInteger(metadata.processPid) || metadata.processPid <= 0 || !metadata.processIdentityToken) {
      return { workerId: metadata.workerId, available: false, lifecycle: "unavailable", reason: "process_identity_unverified" };
    }
    let alive = false;
    try { process.kill(metadata.processPid, 0); alive = true; } catch (cause) { if (cause.code !== "ESRCH") return { workerId: metadata.workerId, available: false, lifecycle: "unavailable", reason: "process_identity_unverified" }; }
    let orphanTerminated = false;
    if (alive) {
      if (!ownedWorkerProcess(metadata)) return { workerId: metadata.workerId, available: false, lifecycle: "unavailable", reason: "process_identity_mismatch" };
      try { process.kill(metadata.processPid, "SIGTERM"); orphanTerminated = true; } catch (cause) { if (cause.code !== "ESRCH") return { workerId: metadata.workerId, available: false, lifecycle: "unavailable", reason: "process_cleanup_failed" }; }
      const deadline = Date.now() + Math.max(100, Number(options.staleCleanupMs) || 1000);
      let exited = false;
      while (Date.now() < deadline) {
        await new Promise((resolve) => setTimeout(resolve, 20));
        try {
          process.kill(metadata.processPid, 0);
          if (!ownedWorkerProcess(metadata)) { exited = true; break; }
        } catch (cause) {
          if (cause.code === "ESRCH") { exited = true; break; }
          return { workerId: metadata.workerId, available: false, lifecycle: "unavailable", reason: "process_cleanup_unverified" };
        }
      }
      if (!exited) return { workerId: metadata.workerId, available: false, lifecycle: "unavailable", reason: "process_cleanup_timeout" };
    }
    try { unlinkSync(metadata.metadataPath); } catch {}
    stale.delete(metadata.workerId);
    return { workerId: metadata.workerId, closed: true, stale: true, orphanTerminated };
  }
  function mutate(worker, operation) { const result = worker.mutations.then(operation, operation); worker.mutations = result.catch(() => {}); return result; }
  function waitResult(worker, cursor, status) { const events = worker.events.filter((item) => item.cursor > cursor); return { status, cursor: worker.cursor, events, snapshot: snapshot(worker) }; }
  async function start(request) {
    if (!path.isAbsolute(request.cwd ?? "") || !existsSync(request.cwd) || !statSync(request.cwd).isDirectory()) throw error("cwd_invalid", "cwd must be an existing absolute directory");
    if (!MODES.has(request.mode) || !String(request.brief ?? "").trim()) throw error("request_invalid", "mode and non-empty brief are required");
    if (["edit", "build", "yolo"].includes(request.mode) && !(request.ownership?.isolatedWorktree || request.ownership?.exclusiveFiles?.length)) throw error("ownership_required", "editing mode requires isolated worktree or exclusive ownership");
    const workerId = request.workerId ?? randomUUID(); if (!WORKER_ID.test(workerId)) throw error("worker_id_invalid", "workerId contains unsupported characters"); if (workers.has(workerId) || stale.has(workerId)) throw error("worker_exists", "workerId already exists");
    const credentials = extractCredentials(options.configPath ?? path.join(os.homedir(), ".zcode", "v2", "config.json"));
    if (request.provider && request.provider !== PROVIDER_ID || request.model && request.model !== MODEL_ID) throw error("provider_invalid", `only ${PROVIDER_ID}/${MODEL_ID} is supported`);
    const reasoning = request.reasoning ?? credentials.reasoningDefault;
    if (!credentials.reasoningLevels.includes(reasoning)) throw error("reasoning_invalid", `unsupported ZCode reasoning level: ${reasoning}`);
    const cliPath = options.cliPath ?? "/Applications/ZCode.app/Contents/Resources/glm/zcode.cjs";
    if (!existsSync(cliPath) || !statSync(cliPath).isFile()) throw error("cli_missing", "ZCode app-server CLI is unavailable");
    const env = { ...process.env, ...options.env, ZCODE_API_KEY: credentials.apiKey, ZCODE_BASE_URL: credentials.baseURL, ZCODE_MODEL: `${PROVIDER_ID}/${MODEL_ID}` };
    const processIdentityToken = randomUUID();
    const child = spawn(process.execPath, [modulePath, "worker", "--token", processIdentityToken, "--cli-path", cliPath, "--cwd", request.cwd], { stdio: ["pipe", "pipe", "ignore"], env });
    const worker = { workerId, cwd: request.cwd, mode: request.mode, provider: credentials.provider, model: credentials.model, reasoning, secrets: [credentials.apiKey, credentials.baseURL], lifecycle: "starting", sessionId: null, upstreamSeq: 0, cursor: 0, events: [], eventBytes: 0, pendingEvents: [], subscribing: true, waiters: [], mutations: Promise.resolve(), deltas: 0, lastDeltaAt: null, bornAt: Date.now(), processPid: child.pid, processIdentityToken, closed: false };
    worker.connection = new AppServer(child, (item) => worker.subscribing ? worker.pendingEvents.push(item) : event(worker, item), (cause) => fail(worker, cause), worker.secrets); workers.set(workerId, worker);
    try {
      await worker.connection.request("session/list", {});
      try { await worker.connection.request("v4/command", { commandId: randomUUID(), clientId: "mornlea-live-supervisor", sessionId: null, type: "invalid", payload: {}, issuedAt: Date.now() }); } catch (cause) { if (!provesMethodPresence(cause)) throw error("v4_missing", "v4 command validation is unavailable"); }
      const created = await worker.connection.request("session/create", { workspace: { workspacePath: request.cwd, workspaceKey: request.cwd }, mode: request.mode, persistence: "immediate", thoughtLevel: reasoning });
      worker.sessionId = created?.sessionId ?? created?.snapshot?.sessionId;
      if (!worker.sessionId) throw error("protocol_create", "session/create did not return sessionId");
      const subscribed = await worker.connection.request("session/subscribe", { sessionId: worker.sessionId, deliveryKind: "desktop-continuous", afterSeq: 0, includeSnapshot: true });
      for (const item of subscribed?.events ?? []) event(worker, item);
      if ((subscribed?.eventSeq ?? worker.upstreamSeq) !== worker.upstreamSeq) throw error("protocol_sequence_gap", "session subscription omitted progress events");
      worker.subscribing = false;
      for (const item of worker.pendingEvents.splice(0)) event(worker, item);
      if (worker.lifecycle === "failed") throw error(worker.lastError.code, worker.lastError.message);
      worker.lifecycle = "running"; persist(worker); material(worker, "worker.started", { sessionId: worker.sessionId });
      const delivery = request.initialDelivery ?? "startNow";
      await steer(worker, request.brief, delivery, true, `${workerId}:initial`);
      return { workerId, sessionId: worker.sessionId, lifecycle: worker.lifecycle, cursor: worker.cursor, reasoning };
    } catch (cause) { workers.delete(workerId); await worker.connection.close(); try { unlinkSync(path.join(workersDir, `${workerId}.json`)); } catch {} throw error(cause.code ?? "start_failed", redact(cause.message, [credentials.apiKey, credentials.baseURL])); }
  }
  async function steer(worker, text, delivery, initial = false, commandId) {
    if (!["guide", "queue", "startNow"].includes(delivery) || !String(text ?? "").trim()) throw error("steer_invalid", "delivery and non-empty text are required");
    if (typeof commandId !== "string" || commandId.length === 0 || commandId.length > 128) throw error("command_id_invalid", "a stable commandId of at most 128 characters is required");
    const result = await worker.connection.request("v4/command", { commandId, clientId: "mornlea-live-supervisor", sessionId: worker.sessionId, type: "sendText", payload: { text, requestedDelivery: delivery }, issuedAt: Date.now() });
    if (!["accepted", "duplicate"].includes(result?.status)) throw error("steer_rejected", result?.message ?? "v4 command was not accepted");
    const actual = result?.result?.delivery;
    if ((delivery === "queue" || delivery === "startNow") && actual && actual !== delivery) throw error("delivery_mismatch", `requested ${delivery}, received ${actual}`);
    if (!initial) material(worker, result.status === "duplicate" ? "steer.duplicate" : "steer.accepted", { commandId, delivery, upstreamDelivery: actual });
    return { accepted: true, duplicate: result.status === "duplicate", commandId, delivery, cursor: worker.cursor };
  }
  async function diagnose(request) {
    if (!path.isAbsolute(request.cwd ?? "") || !existsSync(request.cwd) || !statSync(request.cwd).isDirectory()) throw error("cwd_invalid", "cwd must be an existing absolute directory");
    const credentials = extractCredentials(options.configPath ?? path.join(os.homedir(), ".zcode", "v2", "config.json"));
    const cliPath = options.cliPath ?? "/Applications/ZCode.app/Contents/Resources/glm/zcode.cjs";
    if (!existsSync(cliPath) || !statSync(cliPath).isFile()) throw error("cli_missing", "ZCode app-server CLI is unavailable");
    const child = spawn(process.execPath, [cliPath, "app-server", "--cwd", request.cwd, "--surface", "terminal"], { stdio: ["pipe", "pipe", "ignore"], env: { ...process.env, ...options.env, ZCODE_API_KEY: credentials.apiKey, ZCODE_BASE_URL: credentials.baseURL, ZCODE_MODEL: `${PROVIDER_ID}/${MODEL_ID}` } });
    const connection = new AppServer(child, () => {}, () => {}, [credentials.apiKey, credentials.baseURL]);
    try {
      await connection.request("session/list", {});
      for (const [method, params] of [["session/subscribe", {}], ["v4/command", { commandId: randomUUID(), clientId: "mornlea-live-supervisor", sessionId: null, type: "invalid", payload: {}, issuedAt: Date.now() }]]) {
        try { await connection.request(method, params); } catch (cause) { if (!provesMethodPresence(cause)) throw error("compatibility_missing", `${method} validation is unavailable`); }
      }
      return { compatible: true, provider: PROVIDER_ID, model: MODEL_ID, cliPath, cliIdentity: path.basename(cliPath) };
    } catch (cause) { throw error(cause.code ?? "compatibility_failed", redact(cause.message, [credentials.apiKey, credentials.baseURL])); } finally { await connection.close(); }
  }
  async function handle(request) {
    const op = request?.op;
    if (op === "ping") return { ok: true };
    if (op === "diagnose") return diagnose(request);
    if (op === "start") return start(request);
    const worker = requireWorker(request?.workerId);
    if (!worker) { const metadata = stale.get(request?.workerId); if (op === "close") return closeStale(metadata); return { workerId: request?.workerId, available: false, lifecycle: "unavailable", reason: "supervisor_restart_recovery_required" }; }
    if (op === "status") { try { const read = await worker.connection.request("session/read", { sessionId: worker.sessionId }); if (Number.isInteger(read?.eventSeq) && read.eventSeq < worker.upstreamSeq) fail(worker, error("protocol_sequence_regression", "session snapshot sequence regressed")); else if (read?.eventSeq > worker.upstreamSeq) { const replay = await worker.connection.request("session/events", { sessionId: worker.sessionId, afterSeq: worker.upstreamSeq, limit: MAX_EVENTS }); for (const item of replay?.events ?? []) event(worker, item); if ((replay?.eventSeq ?? worker.upstreamSeq) !== worker.upstreamSeq) fail(worker, error("protocol_sequence_gap", "session replay omitted progress events")); } } catch (cause) { fail(worker, cause); } return { workerId: worker.workerId, available: true, cursor: worker.cursor, ...snapshot(worker) }; }
    if (op === "wait") { const cursor = request.cursor; if (!Number.isInteger(cursor) || cursor < 0) throw error("cursor_invalid", "cursor must be a non-negative integer"); const first = worker.events[0]?.cursor ?? worker.cursor + 1; if (cursor < first - 1) throw error("cursor_expired", `cursor expired; earliest is ${first}`); if (worker.events.some((item) => item.cursor > cursor)) return waitResult(worker, cursor, "event"); const timeoutMs = Math.max(1, Math.min(Number(request.timeoutMs) || 30000, 60000)); return new Promise((resolve) => { const waiter = { cursor, resolve, timer: setTimeout(() => { worker.waiters = worker.waiters.filter((item) => item !== waiter); resolve(waitResult(worker, cursor, "timeout")); }, timeoutMs) }; worker.waiters.push(waiter); }); }
    if (op === "steer") return mutate(worker, () => steer(worker, request.text, request.delivery, false, request.commandId));
    if (op === "stop") return mutate(worker, async () => { await worker.connection.request("session/stop", { sessionId: worker.sessionId }); worker.lifecycle = "stopped"; material(worker, "worker.stopped"); return { workerId: worker.workerId, lifecycle: worker.lifecycle, cursor: worker.cursor }; });
    if (op === "close") return mutate(worker, async () => {
      if (worker.closed) return { workerId: worker.workerId, closed: true, cursor: worker.cursor };
      try {
        await worker.connection.request("session/close", { sessionId: worker.sessionId });
      } finally {
        worker.closing = true; worker.lifecycle = "closing"; persist(worker);
        try { await worker.connection.close(); }
        catch (cause) { worker.closing = false; worker.lifecycle = "cleanup-pending"; persist(worker); throw cause; }
        worker.lifecycle = "closed"; material(worker, "worker.closed"); worker.closed = true; worker.closing = false;
        workers.delete(worker.workerId); try { unlinkSync(path.join(workersDir, `${worker.workerId}.json`)); } catch {}
      }
      return { workerId: worker.workerId, closed: true, cursor: worker.cursor };
    });
    throw error("operation_invalid", "unknown supervisor operation");
  }
  return { runtimeDir, handle, hasWorkers: () => workers.size > 0, async close() { await Promise.all([...workers.values()].map((worker) => worker.connection.close())); workers.clear(); } };
}

function socketPath(runtimeDir) { return path.join(runtimeDir, "supervisor.sock"); }

async function acquireStartupLock(runtimeDir, file) {
  const lockPath = path.join(runtimeDir, "supervisor.start.lock");
  const token = randomUUID();
  for (let attempt = 0; attempt < 200; attempt++) {
    let descriptor;
    try {
      descriptor = openSync(lockPath, "wx", 0o600);
      writeFileSync(descriptor, JSON.stringify({ pid: process.pid, token, createdAt: Date.now() }));
      closeSync(descriptor); descriptor = undefined;
      return {
        release() {
          try {
            const current = JSON.parse(readFileSync(lockPath, "utf8"));
            if (current.token === token) unlinkSync(lockPath);
          } catch {}
        },
      };
    } catch (cause) {
      if (descriptor !== undefined) try { closeSync(descriptor); } catch {}
      if (cause.code !== "EEXIST") throw cause;
      try {
        await requestSocket(file, { op: "ping" });
        throw error("supervisor_already_running", "a live ZCode supervisor already owns the socket");
      } catch (probe) {
        if (probe.code === "supervisor_already_running") throw probe;
        if (!RETRYABLE_SOCKET_ERRORS.has(probe.code)) throw probe;
      }
      try {
        const owner = JSON.parse(readFileSync(lockPath, "utf8"));
        try { process.kill(owner.pid, 0); } catch (ownerError) { if (ownerError.code === "ESRCH") { unlinkSync(lockPath); continue; } }
      } catch {}
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
  }
  throw error("supervisor_start_timeout", "timed out waiting for the ZCode supervisor startup lock");
}

export async function serveSupervisor(options = {}) {
  const runtimeDir = options.runtimeDir ?? defaultRuntimeDir(); const supervisor = options.supervisor ?? createLiveSupervisor({ ...options, runtimeDir });
  mkdirSync(runtimeDir, { recursive: true, mode: 0o700 }); await chmod(runtimeDir, 0o700);
  const file = socketPath(runtimeDir);
  const startupLock = await acquireStartupLock(runtimeDir, file);
  const idleMs = Math.max(1, Number(options.idleShutdownMs ?? process.env.MORNLEA_ZCODE_IDLE_SHUTDOWN_MS) || 30000); let idleTimer; let shutdownPromise;
  const shutdown = () => {
    if (shutdownPromise) return shutdownPromise;
    shutdownPromise = new Promise((resolve) => {
      clearTimeout(idleTimer); try { unlinkSync(file); } catch {}
      server.close(async () => { await supervisor.close(); resolve(); });
      server.closeAllConnections?.();
    });
    return shutdownPromise;
  };
  const scheduleIdle = () => { clearTimeout(idleTimer); idleTimer = setTimeout(() => { if (!supervisor.hasWorkers?.()) shutdown(); }, idleMs); };
  const server = createServer({ allowHalfOpen: true }, (socket) => { clearTimeout(idleTimer); let buffer = ""; socket.setEncoding("utf8"); socket.on("data", async (chunk) => { buffer += chunk; if (buffer.length > MAX_FRAME) { socket.once("finish", scheduleIdle); socket.end(JSON.stringify({ ok: false, error: { code: "request_too_large" } }) + "\n"); return; } const end = buffer.indexOf("\n"); if (end < 0) return; try { const request = JSON.parse(buffer.slice(0, end)); const result = await supervisor.handle(request); if (!supervisor.hasWorkers?.()) socket.once("finish", scheduleIdle); socket.end(JSON.stringify({ ok: true, result: safe(result) }) + "\n"); } catch (cause) { if (!supervisor.hasWorkers?.()) socket.once("finish", scheduleIdle); socket.end(JSON.stringify({ ok: false, error: { code: cause.code ?? "supervisor_error", message: redact(cause.message) } }) + "\n"); } }); });
  try {
    if (existsSync(file)) {
      try { await requestSocket(file, { op: "ping" }); throw error("supervisor_already_running", "a live ZCode supervisor already owns the socket"); }
      catch (cause) { if (cause.code === "supervisor_already_running") throw cause; if (!RETRYABLE_SOCKET_ERRORS.has(cause.code)) throw cause; try { unlinkSync(file); } catch {} }
    }
    await new Promise((resolve, reject) => server.once("error", reject).listen(file, resolve)); await chmod(file, 0o600); scheduleIdle();
    return { path: file, close: shutdown, supervisor };
  } finally {
    startupLock.release();
  }
}

function requestSocket(file, request) { return new Promise((resolve, reject) => { const socket = connect(file); let text = ""; socket.setEncoding("utf8"); socket.once("error", reject); socket.on("data", (chunk) => { text += chunk; }); socket.on("end", () => { try { const reply = JSON.parse(text.trim()); if (!reply.ok) reject(error(reply.error?.code ?? "supervisor_error", reply.error?.message ?? "supervisor error")); else resolve(reply.result); } catch { reject(error("supervisor_protocol", "invalid supervisor response")); } }); socket.on("connect", () => socket.end(`${JSON.stringify(request)}\n`)); }); }

export async function requestLiveSupervisor(request, options = {}) {
  const runtimeDir = options.runtimeDir ?? defaultRuntimeDir(); const file = socketPath(runtimeDir);
  try { return await requestSocket(file, request); } catch (cause) {
    if (options.startDaemon === false || !RETRYABLE_SOCKET_ERRORS.has(cause.code)) throw cause;
    const env = { ...process.env, ...(options.env ?? {}), ...(options.cliPath ? { MORNLEA_ZCODE_CLI_PATH: options.cliPath } : {}), ...(options.configPath ? { MORNLEA_ZCODE_CONFIG_PATH: options.configPath } : {}) };
    const child = spawn(process.execPath, [modulePath, "serve", "--runtime-dir", runtimeDir], { detached: true, stdio: "ignore", env }); child.unref();
    for (let attempt = 0; attempt < 30; attempt++) { await new Promise((resolve) => setTimeout(resolve, 50)); try { return await requestSocket(file, request); } catch (retry) { if (!RETRYABLE_SOCKET_ERRORS.has(retry.code) || attempt === 29) throw retry; } }
  }
}

function argument(name) { const index = process.argv.indexOf(name); return index >= 0 ? process.argv[index + 1] : undefined; }

function runWorkerProcess() {
  const cliPath = argument("--cli-path"); const cwd = argument("--cwd");
  if (!cliPath || !cwd || !argument("--token")) process.exit(2);
  const child = spawn(process.execPath, [cliPath, "app-server", "--cwd", cwd, "--surface", "terminal"], { stdio: "inherit", env: process.env });
  let forceTimer;
  const terminate = () => {
    if (child.exitCode !== null || child.signalCode !== null) return;
    child.kill("SIGTERM");
    clearTimeout(forceTimer);
    forceTimer = setTimeout(() => { if (child.exitCode === null && child.signalCode === null) child.kill("SIGKILL"); }, 500);
    forceTimer.unref();
  };
  process.on("SIGTERM", terminate); process.on("SIGINT", terminate);
  child.on("error", () => process.exit(1)); child.on("exit", (code) => { clearTimeout(forceTimer); process.exit(code ?? 1); });
}

if (process.argv[1] === modulePath && process.argv[2] === "serve") {
  const index = process.argv.indexOf("--runtime-dir"); const runtimeDir = index >= 0 ? process.argv[index + 1] : defaultRuntimeDir();
  serveSupervisor({ runtimeDir, cliPath: process.env.MORNLEA_ZCODE_CLI_PATH, configPath: process.env.MORNLEA_ZCODE_CONFIG_PATH }).catch(() => process.exitCode = 1);
} else if (process.argv[1] === modulePath && process.argv[2] === "worker") {
  runWorkerProcess();
}
