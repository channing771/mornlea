import assert from "node:assert/strict";
import { createServer } from "node:http";
import { mkdtempSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { createLiveSupervisor } from "./opencode-live-agent.mjs";

const provider = "opencode-go";
const model = "muse-spark-1.3-contributor";

async function fakeRuntime() {
  let eventResponse;
  let sessionCounter = 0;
  const requests = [];
  const sockets = new Set();
  const server = createServer(async (request, response) => {
    const url = new URL(request.url, "http://127.0.0.1");
    if (request.method === "GET" && url.pathname === "/global/health") {
      response.setHeader("content-type", "application/json"); response.end(JSON.stringify({ healthy: true })); return;
    }
    if (request.method === "GET" && url.pathname === "/config/providers") {
      response.setHeader("content-type", "application/json");
      response.end(JSON.stringify({ providers: [{ id: provider, models: { [model]: { variants: { xhigh: { reasoningEffort: "xhigh" } } } } }] })); return;
    }
    if (request.method === "GET" && url.pathname === "/event") {
      response.writeHead(200, { "content-type": "text/event-stream", connection: "keep-alive" });
      eventResponse = response;
      return;
    }
    let body = "";
    for await (const chunk of request) body += chunk;
    const parsed = body ? JSON.parse(body) : null;
    requests.push({ method: request.method, path: url.pathname, body: parsed });
    if (request.method === "POST" && url.pathname === "/api/session") {
      sessionCounter += 1;
      response.setHeader("content-type", "application/json"); response.end(JSON.stringify({ data: { id: `session-${sessionCounter}` } })); return;
    }
    if (request.method === "POST" && url.pathname.endsWith("/prompt")) {
      response.setHeader("content-type", "application/json"); response.end(JSON.stringify({ data: { admittedSeq: requests.length } }));
      if (eventResponse) {
        eventResponse.write(`data: ${JSON.stringify({ type: "session.status", properties: { sessionID: url.pathname.split("/")[3], status: { type: "busy" } } })}\r\n\r\n`);
        eventResponse.write(`data: ${JSON.stringify({ type: "session.status", properties: { sessionID: url.pathname.split("/")[3], status: { type: "idle" } } })}\r\n\r\n`);
      }
      return;
    }
    response.setHeader("content-type", "application/json"); response.end(JSON.stringify({ data: {} }));
  });
  server.on("connection", (socket) => { sockets.add(socket); socket.on("close", () => sockets.delete(socket)); });
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  const runtime = {
    baseURL: `http://127.0.0.1:${address.port}`,
    closed: false,
    requests,
    async request(method, endpoint, body) {
      const response = await fetch(`${this.baseURL}${endpoint}`, {
        method,
        headers: body === undefined ? undefined : { "content-type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      const text = await response.text();
      if (!response.ok) throw new Error(`fake OpenCode HTTP ${response.status}`);
      return text ? JSON.parse(text) : null;
    },
    async close() {
      this.closed = true;
      eventResponse?.end();
      for (const socket of sockets) socket.destroy();
      server.closeAllConnections?.();
      await new Promise((resolve) => server.close(() => resolve()));
    },
  };
  return runtime;
}

function workspace() {
  const root = mkdtempSync(path.join(tmpdir(), "mornlea-opencode-live-"));
  const directory = path.join(root, "workspace");
  mkdirSync(directory);
  return directory;
}

async function supervisorFixture() {
  const runtime = await fakeRuntime();
  const supervisor = createLiveSupervisor({ runtimeDir: mkdtempSync(path.join(tmpdir(), "mornlea-opencode-runtime-")), serverFactory: async () => runtime });
  return { runtime, supervisor, cwd: workspace() };
}

test("diagnose and start lock the OpenCode route to Muse Spark xhigh", async () => {
  const item = await supervisorFixture();
  try {
    const diagnosis = await item.supervisor.handle({ op: "diagnose", cwd: item.cwd });
    assert.deepEqual({ compatible: diagnosis.compatible, provider: diagnosis.provider, model: diagnosis.model, reasoning: diagnosis.reasoning }, { compatible: true, provider, model, reasoning: "xhigh" });
    const started = await item.supervisor.handle({ op: "start", cwd: item.cwd, mode: "plan", brief: "inspect", provider, model, reasoning: "xhigh" });
    assert.equal(started.provider, provider);
    assert.equal(started.model, model);
    assert.equal(started.reasoning, "xhigh");
    const status = await item.supervisor.handle({ op: "status", workerId: started.workerId });
    assert.equal(status.reasoning, "xhigh");
    const wait = await item.supervisor.handle({ op: "wait", workerId: started.workerId, cursor: 0, timeoutMs: 100 });
    assert.ok(wait.events.some((event) => event.type === "worker.started"));
  } finally {
    await item.supervisor.close();
  }
});

test("steering maps guide, queue, and startNow to OpenCode delivery semantics", async () => {
  const item = await supervisorFixture();
  try {
    const started = await item.supervisor.handle({ op: "start", cwd: item.cwd, mode: "plan", brief: "inspect" });
    await item.supervisor.handle({ op: "steer", workerId: started.workerId, delivery: "guide", commandId: "guide-1", text: "guide" });
    await item.supervisor.handle({ op: "steer", workerId: started.workerId, delivery: "queue", commandId: "queue-1", text: "queue" });
    await item.supervisor.handle({ op: "steer", workerId: started.workerId, delivery: "startNow", commandId: "now-1", text: "now" });
    const prompts = item.runtime.requests.filter((request) => request.path.endsWith("/prompt"));
    assert.deepEqual(prompts.slice(-3).map((request) => request.body.delivery), ["steer", "queue", "steer"]);
    assert.ok(item.runtime.requests.some((request) => request.path.endsWith("/interrupt")));
  } finally {
    await item.supervisor.close();
  }
});

test("editing and unsupported provider or reasoning requests are rejected", async () => {
  const item = await supervisorFixture();
  try {
    await assert.rejects(item.supervisor.handle({ op: "start", cwd: item.cwd, mode: "edit", brief: "edit" }), { code: "ownership_required" });
    await assert.rejects(item.supervisor.handle({ op: "start", cwd: item.cwd, mode: "plan", brief: "bad", reasoning: "high" }), { code: "reasoning_invalid" });
    await assert.rejects(item.supervisor.handle({ op: "start", cwd: item.cwd, mode: "plan", brief: "bad", provider: "other" }), { code: "provider_invalid" });
  } finally {
    await item.supervisor.close();
    await item.runtime.close();
  }
});
