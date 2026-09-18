#!/usr/bin/env node

import { spawn } from "node:child_process";
import { createServer, connect } from "node:net";
import { closeSync, existsSync, mkdirSync, openSync, statSync, unlinkSync, writeFileSync } from "node:fs";
import { chmod } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import os from "node:os";
import path from "node:path";
import { randomUUID } from "node:crypto";

const MODULE_PATH = fileURLToPath(import.meta.url);
const MAX_FRAME = 1024 * 1024;
const MAX_EVENTS = 256;
const MAX_EVENT_BYTES = 64 * 1024;
const PROVIDER_ID = "opencode-go";
const MODEL_ID = "muse-spark-1.3-contributor";
const VARIANT = "xhigh";
const MODES = new Set(["plan", "build", "edit", "yolo"]);
const RETRYABLE_SOCKET_ERRORS = new Set(["ENOENT", "ECONNREFUSED"]);
const WORKER_ID = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;
const SECRET_KEY = /api[-_]?key|token|secret|authorization|password|baseurl|base_url/i;

export function defaultRuntimeDir() {
  const override = process.env.MORNLEA_OPENCODE_RUNTIME_DIR;
  if (override && path.isAbsolute(override)) return path.resolve(override);
  return path.join(os.tmpdir(), `mornlea-opencode-live-${process.getuid?.() ?? "unknown"}`);
}

function redact(value) {
  return String(value)
    .replace(/(?:Bearer\s+)?(?:sk-|AIza)[A-Za-z0-9._-]+/gi, "[redacted]")
    .replace(/secret[-_a-z0-9.]*/gi, "[redacted]");
}

function safe(value, depth = 0) {
  if (depth > 4) return "[truncated]";
  if (typeof value === "string") return redact(value).slice(0, 2048);
  if (value === null || typeof value !== "object") return value;
  if (Array.isArray(value)) return value.slice(0, 20).map((item) => safe(item, depth + 1));
  return Object.fromEntries(
    Object.entries(value)
      .filter(([key]) => !SECRET_KEY.test(key))
      .slice(0, 40)
      .map(([key, item]) => [key, safe(item, depth + 1)]),
  );
}

function error(code, message) {
  const cause = new Error(redact(message));
  cause.code = code;
  return cause;
}

function validateCwd(cwd) {
  const resolved = path.resolve(cwd ?? "");
  if (!path.isAbsolute(cwd ?? "") || !existsSync(resolved) || !statSync(resolved).isDirectory()) {
    throw error("cwd_invalid", "cwd must be an existing absolute directory");
  }
  return resolved;
}

function cliPath() {
  return process.env.MORNLEA_OPENCODE_CLI_PATH ?? "opencode";
}

function modelRef() {
  return { providerID: PROVIDER_ID, id: MODEL_ID, variant: VARIANT };
}

function modeAgent(mode) {
  return mode === "plan" ? "plan" : "build";
}

async function allocatePort() {
  return new Promise((resolve, reject) => {
    const probe = createServer();
    probe.once("error", reject);
    probe.listen(0, "127.0.0.1", () => {
      const port = probe.address().port;
      probe.close(() => resolve(port));
    });
  });
}

async function waitForHealth(baseURL, child) {
  const deadline = Date.now() + 8000;
  let lastError = "";
  while (Date.now() < deadline) {
    if (child.exitCode !== null || child.signalCode !== null) {
      throw error("server_exit", `OpenCode server exited (${child.exitCode ?? child.signalCode ?? "unknown"})`);
    }
    try {
      const response = await fetch(`${baseURL}/global/health`);
      if (response.ok) return await response.json();
      lastError = `HTTP ${response.status}`;
    } catch (cause) {
      lastError = cause.message;
    }
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw error("server_start_timeout", `OpenCode server health check timed out: ${lastError}`);
}

class OpenCodeServer {
  constructor(child, baseURL) {
    this.child = child;
    this.baseURL = baseURL;
    this.closed = false;
  }

  async request(method, endpoint, body, timeoutMs = 30000) {
    if (this.closed) throw error("server_closed", "OpenCode server is closed");
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), timeoutMs);
    try {
      const response = await fetch(`${this.baseURL}${endpoint}`, {
        method,
        headers: body === undefined ? undefined : { "content-type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
        signal: controller.signal,
      });
      const text = await response.text();
      let payload = null;
      if (text.trim()) {
        try {
          payload = JSON.parse(text);
        } catch {
          payload = { raw: text.slice(0, 2048) };
        }
      }
      if (!response.ok) {
        throw error("opencode_http_error", `OpenCode ${method} ${endpoint} failed (${response.status}): ${payload?.message ?? payload?.data?.message ?? response.statusText}`);
      }
      return payload;
    } catch (cause) {
      if (cause.name === "AbortError") throw error("opencode_timeout", `${method} ${endpoint} timed out`);
      throw cause;
    } finally {
      clearTimeout(timer);
    }
  }

  async close() {
    if (this.closed) return;
    this.closed = true;
    this.eventAbortController?.abort();
    if (this.child.exitCode !== null || this.child.signalCode !== null) return;
    this.child.kill("SIGTERM");
    const exited = await Promise.race([
      new Promise((resolve) => this.child.once("exit", () => resolve(true))),
      new Promise((resolve) => setTimeout(() => resolve(false), 1200)),
    ]);
    if (!exited && this.child.exitCode === null) this.child.kill("SIGKILL");
  }
}

async function startOpenCodeServer(options = {}) {
  if (typeof options.serverFactory === "function") return options.serverFactory();
  const port = await allocatePort();
  const child = spawn(cliPath(), ["serve", "--hostname=127.0.0.1", `--port=${port}`, "--log-level=ERROR"], {
    stdio: ["ignore", "ignore", "ignore"],
    env: process.env,
  });
  const runtime = new OpenCodeServer(child, `http://127.0.0.1:${port}`);
  try {
    await waitForHealth(runtime.baseURL, child);
    return runtime;
  } catch (cause) {
    await runtime.close();
    throw cause;
  }
}

async function consumeEvents(runtime, onEvent, onFailure) {
  const controller = new AbortController();
  runtime.eventAbortController = controller;
  try {
    const response = await fetch(`${runtime.baseURL}/event`, { headers: { accept: "text/event-stream" }, signal: controller.signal });
    if (!response.ok || !response.body) throw error("event_stream_failed", `OpenCode event stream failed (${response.status})`);
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    while (!runtime.closed) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      let boundary;
      while (true) {
        const match = /\r?\n\r?\n/.exec(buffer);
        if (!match) break;
        boundary = match.index;
        const frame = buffer.slice(0, boundary);
        buffer = buffer.slice(boundary + match[0].length);
        const data = frame.split(/\r?\n/).filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trim()).join("\n");
        if (!data) continue;
        try { onEvent(JSON.parse(data)); } catch (cause) { onFailure(error("event_malformed", `OpenCode event was malformed: ${cause.message}`)); }
      }
    }
    if (!runtime.closed) onFailure(error("event_stream_closed", "OpenCode event stream closed unexpectedly"));
  } catch (cause) {
    if (cause.name === "AbortError") return;
    if (!runtime.closed) onFailure(cause);
  }
}

function eventSessionId(input) {
  const properties = input?.properties ?? input?.data?.properties ?? {};
  return properties.sessionID ?? properties.info?.sessionID ?? properties.part?.sessionID ?? properties.message?.sessionID ?? null;
}

function isFailureEvent(input) {
  return String(input?.type ?? "").includes("error") || String(input?.type ?? "").includes("failed");
}

export function createLiveSupervisor(options = {}) {
  const runtimeDir = options.runtimeDir ?? defaultRuntimeDir();
  const workersDir = path.join(runtimeDir, "workers");
  mkdirSync(workersDir, { recursive: true, mode: 0o700 });
  const workers = new Map();
  let runtimePromise;

  function metadata(worker) {
    return {
      workerId: worker.workerId,
      sessionId: worker.sessionId,
      cwd: worker.cwd,
      mode: worker.mode,
      provider: PROVIDER_ID,
      model: MODEL_ID,
      reasoning: VARIANT,
      lifecycle: worker.lifecycle,
      cursor: worker.cursor,
      bornAt: worker.bornAt,
    };
  }

  function persist(worker) {
    try { writeFileSync(path.join(workersDir, `${worker.workerId}.json`), JSON.stringify(metadata(worker)), { mode: 0o600 }); } catch {}
  }

  function snapshot(worker) {
    return {
      lifecycle: worker.lifecycle,
      sessionId: worker.sessionId,
      provider: PROVIDER_ID,
      model: MODEL_ID,
      reasoning: VARIANT,
      activity: { deltaCount: worker.deltas, lastAt: worker.lastDeltaAt },
      lastError: worker.lastError,
    };
  }

  function material(worker, type, payload = {}) {
    const item = { cursor: ++worker.cursor, type, at: Date.now(), payload: safe(payload) };
    let size = Buffer.byteLength(JSON.stringify(item));
    if (size > MAX_EVENT_BYTES) { item.payload = { truncated: true }; size = Buffer.byteLength(JSON.stringify(item)); }
    worker.events.push(item);
    worker.eventBytes += size;
    while (worker.events.length > MAX_EVENTS || worker.eventBytes > MAX_EVENT_BYTES) {
      const removed = worker.events.shift();
      worker.eventBytes -= Buffer.byteLength(JSON.stringify(removed));
    }
    persist(worker);
    for (const waiter of worker.waiters.splice(0)) {
      clearTimeout(waiter.timer);
      waiter.resolve(waitResult(worker, waiter.cursor, "event"));
    }
  }

  function fail(worker, cause) {
    if (worker.closed || worker.lifecycle === "failed") return;
    worker.lifecycle = "failed";
    worker.lastError = { code: cause.code ?? "opencode_failure", message: redact(cause.message) };
    material(worker, "protocol.failure", worker.lastError);
  }

  function applyEvent(input) {
    const sessionId = eventSessionId(input);
    if (!sessionId) return;
    const worker = [...workers.values()].find((candidate) => candidate.sessionId === sessionId);
    if (!worker) return;
    const type = String(input.type ?? "session.event");
    const properties = input.properties ?? {};
    if (type === "session.status") {
      const state = properties.status?.type;
      if (state === "busy") worker.lifecycle = "running";
      if (state === "idle") worker.lifecycle = "idle";
      material(worker, type, { status: state });
      return;
    }
    if (type === "session.idle") {
      worker.lifecycle = "idle";
      material(worker, type);
      return;
    }
    if (isFailureEvent(input)) {
      worker.lifecycle = "failed";
      worker.lastError = { code: "opencode_event_failure", message: redact(JSON.stringify(safe(input))) };
      material(worker, type, worker.lastError);
      return;
    }
    if (type === "message.part.updated") {
      const partType = properties.part?.type;
      if (partType === "text" || partType === "reasoning") {
        worker.deltas += 1;
        worker.lastDeltaAt = Date.now();
        return;
      }
    }
    material(worker, type, { part: properties.part?.type, messageID: properties.messageID ?? properties.part?.messageID });
  }

  async function ensureRuntime() {
    if (!runtimePromise) {
      runtimePromise = startOpenCodeServer(options).then((runtime) => {
        runtime.eventsTask = consumeEvents(runtime, applyEvent, (cause) => {
          for (const worker of workers.values()) fail(worker, cause);
        });
        return runtime;
      }).catch((cause) => {
        runtimePromise = undefined;
        throw cause;
      });
    }
    return runtimePromise;
  }

  function requireWorker(workerId) {
    const worker = workers.get(workerId);
    if (!worker) throw error("worker_not_found", `unknown worker ${workerId}`);
    return worker;
  }

  function waitResult(worker, cursor, status) {
    return { status, cursor: worker.cursor, events: worker.events.filter((item) => item.cursor > cursor), snapshot: snapshot(worker) };
  }

  async function start(request) {
    const cwd = validateCwd(request.cwd);
    const mode = request.mode ?? "plan";
    if (!MODES.has(mode) || !String(request.brief ?? "").trim()) throw error("request_invalid", "mode and non-empty brief are required");
    if ((request.provider && request.provider !== PROVIDER_ID) || (request.model && request.model !== MODEL_ID)) throw error("provider_invalid", `only ${PROVIDER_ID}/${MODEL_ID} is supported`);
    if (request.reasoning !== undefined && request.reasoning !== VARIANT) throw error("reasoning_invalid", `OpenCode only permits reasoning variant ${VARIANT}`);
    if (["edit", "build", "yolo"].includes(mode) && !(request.ownership?.isolatedWorktree || request.ownership?.exclusiveFiles?.length)) throw error("ownership_required", "editing mode requires isolated worktree or exclusive ownership");
    const workerId = request.workerId ?? randomUUID();
    if (!WORKER_ID.test(workerId) || workers.has(workerId)) throw error("worker_id_invalid", "workerId is invalid or already exists");
    const runtime = await ensureRuntime();
    const created = await runtime.request("POST", "/api/session", { agent: modeAgent(mode), model: modelRef(), location: { directory: cwd } });
    const sessionId = created?.data?.id;
    if (!sessionId) throw error("session_create_failed", "OpenCode did not return a session ID");
    const worker = { workerId, sessionId, cwd, mode, lifecycle: "starting", cursor: 0, events: [], eventBytes: 0, waiters: [], deltas: 0, lastDeltaAt: null, lastError: null, bornAt: Date.now(), closed: false, mutations: Promise.resolve() };
    workers.set(workerId, worker);
    persist(worker);
    material(worker, "worker.started", { sessionId, provider: PROVIDER_ID, model: MODEL_ID, reasoning: VARIANT });
    try {
      await runtime.request("POST", `/api/session/${encodeURIComponent(sessionId)}/prompt`, { id: `${workerId}:initial`, prompt: { text: String(request.brief).trim() }, delivery: "steer" });
      worker.lifecycle = "running";
      persist(worker);
      material(worker, "prompt.accepted", { commandId: `${workerId}:initial`, delivery: "steer" });
      return { workerId, sessionId, lifecycle: worker.lifecycle, cursor: worker.cursor, provider: PROVIDER_ID, model: MODEL_ID, reasoning: VARIANT };
    } catch (cause) {
      workers.delete(workerId);
      try { await runtime.request("DELETE", `/session/${encodeURIComponent(sessionId)}?directory=${encodeURIComponent(cwd)}`); } catch {}
      try { unlinkSync(path.join(workersDir, `${workerId}.json`)); } catch {}
      throw cause;
    }
  }

  function mutate(worker, operation) {
    const result = worker.mutations.then(operation, operation);
    worker.mutations = result.catch(() => {});
    return result;
  }

  async function steer(worker, text, delivery, commandId) {
    if (!String(text ?? "").trim() || !["guide", "queue", "startNow"].includes(delivery)) throw error("steer_invalid", "delivery and non-empty text are required");
    if (!commandId || commandId.length > 128) throw error("command_id_invalid", "a stable commandId of at most 128 characters is required");
    const runtime = await ensureRuntime();
    let upstreamDelivery = delivery === "queue" ? "queue" : "steer";
    if (delivery === "startNow") {
      try { await runtime.request("POST", `/api/session/${encodeURIComponent(worker.sessionId)}/interrupt`); } catch (cause) { if (!/not found|idle/i.test(cause.message)) throw cause; }
    }
    const result = await runtime.request("POST", `/api/session/${encodeURIComponent(worker.sessionId)}/prompt`, { id: commandId, prompt: { text: String(text).trim() }, delivery: upstreamDelivery });
    worker.lifecycle = "running";
    material(worker, "prompt.accepted", { commandId, delivery, upstreamDelivery, admitted: result?.data?.admittedSeq ?? null });
    return { accepted: true, commandId, delivery, upstreamDelivery, cursor: worker.cursor };
  }

  async function diagnose(request) {
    validateCwd(request.cwd ?? process.cwd());
    const runtime = await ensureRuntime();
    const providers = await runtime.request("GET", "/config/providers");
    const model = providers?.providers?.find((provider) => provider.id === PROVIDER_ID)?.models?.[MODEL_ID];
    const supported = model?.variants?.[VARIANT]?.reasoningEffort === VARIANT;
    if (!supported) throw error("model_unavailable", `OpenCode model ${PROVIDER_ID}/${MODEL_ID} with ${VARIANT} is unavailable`);
    return { compatible: true, backend: "opencode", provider: PROVIDER_ID, model: MODEL_ID, reasoning: VARIANT, cliPath: cliPath(), serverVersion: null };
  }

  async function handle(request) {
    const operation = request?.op;
    if (operation === "ping") return { ok: true };
    if (operation === "diagnose") return diagnose(request);
    if (operation === "start") return start(request);
    const worker = requireWorker(request?.workerId);
    if (operation === "status") return { workerId: worker.workerId, available: true, cursor: worker.cursor, ...snapshot(worker) };
    if (operation === "wait") {
      const cursor = request.cursor;
      if (!Number.isInteger(cursor) || cursor < 0) throw error("cursor_invalid", "cursor must be a non-negative integer");
      const first = worker.events[0]?.cursor ?? worker.cursor + 1;
      if (cursor < first - 1) throw error("cursor_expired", `cursor expired; earliest is ${first}`);
      if (worker.events.some((item) => item.cursor > cursor)) return waitResult(worker, cursor, "event");
      const timeoutMs = Math.max(1, Math.min(Number(request.timeoutMs) || 30000, 60000));
      return new Promise((resolve) => {
        const waiter = { cursor, resolve, timer: setTimeout(() => { worker.waiters = worker.waiters.filter((item) => item !== waiter); resolve(waitResult(worker, cursor, "timeout")); }, timeoutMs) };
        worker.waiters.push(waiter);
      });
    }
    if (operation === "steer") return mutate(worker, () => steer(worker, request.text, request.delivery, request.commandId));
    const runtime = await ensureRuntime();
    if (operation === "stop") return mutate(worker, async () => { await runtime.request("POST", `/api/session/${encodeURIComponent(worker.sessionId)}/interrupt`); worker.lifecycle = "stopped"; material(worker, "worker.stopped"); return { workerId: worker.workerId, lifecycle: worker.lifecycle, cursor: worker.cursor }; });
    if (operation === "close") return mutate(worker, async () => {
      try { await runtime.request("DELETE", `/session/${encodeURIComponent(worker.sessionId)}?directory=${encodeURIComponent(worker.cwd)}`); }
      finally {
        worker.closed = true;
        worker.lifecycle = "closed";
        material(worker, "worker.closed");
        workers.delete(worker.workerId);
        try { unlinkSync(path.join(workersDir, `${worker.workerId}.json`)); } catch {}
      }
      return { workerId: worker.workerId, closed: true, cursor: worker.cursor };
    });
    throw error("operation_invalid", `unknown OpenCode supervisor operation: ${operation}`);
  }

  return {
    runtimeDir,
    handle,
    hasWorkers: () => workers.size > 0,
    async close() {
      for (const worker of workers.values()) worker.closed = true;
      workers.clear();
      if (runtimePromise) {
        const runtime = await runtimePromise.catch(() => null);
        if (runtime) {
          runtime.eventAbortController?.abort();
          await runtime.close();
        }
      }
    },
  };
}

function socketPath(runtimeDir) { return path.join(runtimeDir, "supervisor.sock"); }

function requestSocket(file, request) {
  return new Promise((resolve, reject) => {
    const socket = connect(file);
    let text = "";
    socket.setEncoding("utf8");
    socket.once("error", reject);
    socket.on("data", (chunk) => { text += chunk; });
    socket.on("end", () => {
      try {
        const reply = JSON.parse(text.trim());
        if (!reply.ok) reject(error(reply.error?.code ?? "supervisor_error", reply.error?.message ?? "supervisor error"));
        else resolve(reply.result);
      } catch { reject(error("supervisor_protocol", "invalid OpenCode supervisor response")); }
    });
    socket.on("connect", () => socket.end(`${JSON.stringify(request)}\n`));
  });
}

async function acquireStartupLock(runtimeDir) {
  const lockPath = path.join(runtimeDir, "supervisor.start.lock");
  for (let attempt = 0; attempt < 100; attempt += 1) {
    try {
      const descriptor = openSync(lockPath, "wx", 0o600);
      closeSync(descriptor);
      return { release: () => { try { unlinkSync(lockPath); } catch {} }, descriptor };
    } catch (cause) {
      if (cause.code !== "EEXIST") throw cause;
      await new Promise((resolve) => setTimeout(resolve, 25));
    }
  }
  throw error("supervisor_start_timeout", "timed out waiting for OpenCode supervisor startup lock");
}

export async function serveSupervisor(options = {}) {
  const runtimeDir = options.runtimeDir ?? defaultRuntimeDir();
  mkdirSync(runtimeDir, { recursive: true, mode: 0o700 });
  await chmod(runtimeDir, 0o700);
  const supervisor = options.supervisor ?? createLiveSupervisor({ ...options, runtimeDir });
  const file = socketPath(runtimeDir);
  const lock = await acquireStartupLock(runtimeDir);
  const idleMs = Math.max(1, Number(options.idleShutdownMs ?? process.env.MORNLEA_OPENCODE_IDLE_SHUTDOWN_MS) || 30000);
  let idleTimer;
  let shuttingDown;
  const server = createServer({ allowHalfOpen: true }, (socket) => {
    clearTimeout(idleTimer);
    let buffer = "";
    socket.setEncoding("utf8");
    socket.on("data", async (chunk) => {
      buffer += chunk;
      if (buffer.length > MAX_FRAME) { socket.end(JSON.stringify({ ok: false, error: { code: "request_too_large", message: "request exceeds limit" } }) + "\n"); return; }
      const point = buffer.indexOf("\n");
      if (point < 0) return;
      try {
        const result = await supervisor.handle(JSON.parse(buffer.slice(0, point)));
        socket.end(JSON.stringify({ ok: true, result: safe(result) }) + "\n");
      } catch (cause) {
        socket.end(JSON.stringify({ ok: false, error: { code: cause.code ?? "supervisor_error", message: redact(cause.message) } }) + "\n");
      }
      if (!supervisor.hasWorkers()) scheduleIdle();
    });
  });
  function scheduleIdle() { clearTimeout(idleTimer); idleTimer = setTimeout(() => { if (!supervisor.hasWorkers()) shutdown(); }, idleMs); }
  function shutdown() {
    if (shuttingDown) return shuttingDown;
    shuttingDown = new Promise((resolve) => {
      clearTimeout(idleTimer);
      try { unlinkSync(file); } catch {}
      server.close(async () => { await supervisor.close(); resolve(); });
    });
    return shuttingDown;
  }
  try {
    if (existsSync(file)) {
      try { await requestSocket(file, { op: "ping" }); throw error("supervisor_already_running", "an OpenCode supervisor already owns the socket"); }
      catch (cause) { if (cause.code === "supervisor_already_running") throw cause; try { unlinkSync(file); } catch {} }
    }
    await new Promise((resolve, reject) => server.once("error", reject).listen(file, resolve));
    await chmod(file, 0o600);
    scheduleIdle();
    return { path: file, close: shutdown, supervisor };
  } finally {
    lock.release();
  }
}

export async function requestOpenCodeSupervisor(request, options = {}) {
  const runtimeDir = options.runtimeDir ?? defaultRuntimeDir();
  const file = socketPath(runtimeDir);
  try {
    return await requestSocket(file, request);
  } catch (cause) {
    if (options.startDaemon === false || !RETRYABLE_SOCKET_ERRORS.has(cause.code)) throw cause;
    mkdirSync(runtimeDir, { recursive: true, mode: 0o700 });
    const child = spawn(process.execPath, [MODULE_PATH, "serve", "--runtime-dir", runtimeDir], { detached: true, stdio: "ignore", env: process.env });
    child.unref();
    for (let attempt = 0; attempt < 40; attempt += 1) {
      await new Promise((resolve) => setTimeout(resolve, 50));
      try { return await requestSocket(file, request); }
      catch (retry) { if (!RETRYABLE_SOCKET_ERRORS.has(retry.code) || attempt === 39) throw retry; }
    }
  }
}

function argument(name) { const index = process.argv.indexOf(name); return index >= 0 ? process.argv[index + 1] : undefined; }

if (process.argv[1] === MODULE_PATH && process.argv[2] === "serve") {
  const runtimeDir = argument("--runtime-dir") ?? defaultRuntimeDir();
  serveSupervisor({ runtimeDir, idleShutdownMs: process.env.MORNLEA_OPENCODE_IDLE_SHUTDOWN_MS }).catch((cause) => { process.stderr.write(`${redact(cause.message)}\n`); process.exitCode = 1; });
}
