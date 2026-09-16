import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import {
  createLiveSupervisor,
  defaultRuntimeDir,
  serveSupervisor,
  requestLiveSupervisor,
} from "./zcode-live-agent.mjs";

const node = process.execPath;

async function fixture(t, behavior = {}) {
  const root = await mkdtemp(path.join(os.tmpdir(), "mornlea-zcode-live-test-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const cwd = path.join(root, "worktree");
  await mkdir(cwd);
  const configPath = path.join(root, "config.json");
  await writeFile(configPath, JSON.stringify({ provider: { "builtin:bigmodel-coding-plan": { enabled: true, options: { apiKey: "unrecognizable-configured-credential", baseURL: "https://secret.example" }, models: { "GLM-5.3": { reasoning: { defaultVariant: "max", variants: ["low", "high", "max"] } } } } } }));
  const behaviorPath = path.join(root, "behavior.json");
  await writeFile(behaviorPath, JSON.stringify(behavior));
  const cliPath = path.join(root, "fake-zcode.mjs");
  await writeFile(cliPath, `
import { readFileSync } from "node:fs";
const b = JSON.parse(readFileSync(process.env.FAKE_BEHAVIOR)); let buffer = ""; let seq = 0; let sendCount = 0; const commandIds = new Set();
	function out(x) { process.stdout.write(JSON.stringify(x) + "\\n"); }
	function event(type, extra = {}) { const value={ seq: ++seq, type, ...extra }; out({ method: "session/event", params: b.nested ? {event:value} : value }); }
	if (b.exitEarly) process.exit(3);
	if (b.ignoreTerm) process.on("SIGTERM", () => {});
	if (b.delayTerm) process.on("SIGTERM", () => setTimeout(() => process.exit(0), b.delayTerm));
	process.stdin.on("data", chunk => { buffer += chunk; while (buffer.includes("\\n")) { const i = buffer.indexOf("\\n"); const line = buffer.slice(0, i); buffer = buffer.slice(i + 1); if (!line) continue; let r; try { r = JSON.parse(line); } catch { out({ broken: true }); continue; }
 if (b.malformed) { process.stdout.write("not-json\\n"); continue; }
 if (b.unsupported && r.method === "session/list") { out({ id:r.id,error:{code:-32601,message:"unsupported secret-key"} }); continue; }
 if (r.method === "session/list") out({ id:r.id,result:{sessions:[]} });
 else if (r.method === "v4/command" && r.params.type === "invalid") out({ id:r.id,error:{code:-32602,message:"invalid"} });
 else if (r.method === "session/create") { if (b.strictReasoning && r.params.thoughtLevel !== "high") out({id:r.id,error:{code:-32602,message:"thoughtLevel missing"}}); else out({ id:r.id,result:{sessionId:"upstream-1",eventSeq:seq} }); }
 else if (r.method === "session/subscribe") { if (!r.params.sessionId) out({id:r.id,error:{code:-32602,message:"invalid subscribe"}}); else { const events=b.gap?[{seq:2,type:"turn.completed"}]:b.backfill?[{seq:1,type:"turn.started"},{seq:2,type:"turn.completed"}]:[]; out({ id:r.id,result:{sessionId:"upstream-1",eventSeq:(b.backfill||b.gap)?2:seq,events,snapshot:{sessionId:"upstream-1",state:"idle"}} }); if(b.backfill) seq=2; if (b.deltas) { event("part.delta",{delta:"one"}); event("part.delta",{delta:"two"}); event("turn.completed",{result:"ok"}); } if (b.noisy) { event("part.upserted",{text:"draft"}); event("message.upserted",{text:"draft"}); event("reasoning_delta",{text:"draft"}); event("tool_input_delta",{text:"draft"}); event("tool.updated",{status:"running"}); event("tool.updated",{status:"failed",error:"boom"}); } if (b.bigEvents) for(let n=0;n<40;n++) event("checkpoint.created",{summary:"x".repeat(3000)}); if (b.secretEvent) event("tool.updated",{status:"failed",message:"unrecognizable-configured-credential"}); if (b.regress) { event("turn.started"); out({method:"session/event",params:{seq:0,type:"turn.completed"}}); } } }
 else if (r.method === "session/read") { if (b.strictRead && Object.keys(r.params).join(",") !== "sessionId") out({id:r.id,error:{code:-32602,message:"read params wrong"}}); else out({ id:r.id,result:{sessionId:"upstream-1",eventSeq:b.readBehind?0:b.readAhead?2:seq,state:"idle"} }); }
 else if (r.method === "session/events") { out({ id:r.id,result:{sessionId:"upstream-1",eventSeq:2,events:[{seq:1,type:"turn.started"},{seq:2,type:"turn.completed"}] } }); }
 else if (r.method === "session/stop" || r.method === "session/close") out({ id:r.id,result:r.method === "session/close" ? {closed:true} : {} });
 else if (r.method === "v4/command") { if (r.params.type === "sendText") sendCount++; if (b.rejectInitialSend && r.params.type === "sendText") out({id:r.id,error:{code:-32000,message:"initial send failed unrecognizable-configured-credential"}}); else if (b.rejectSteerSecret && sendCount > 1) out({id:r.id,error:{code:-32000,message:"steer failed unrecognizable-configured-credential"}}); else { const duplicate=commandIds.has(r.params.commandId); commandIds.add(r.params.commandId); const reply=()=>out({ id:r.id,result:{commandId:r.params.commandId,status:duplicate?"duplicate":"accepted",result:{type:"inputAccepted",delivery:r.params.payload.requestedDelivery}} }); if (b.delaySteer && sendCount > 1) setTimeout(reply,25); else reply(); if (b.exitAfterCommand) process.exit(3); } }
 else out({ id:r.id,error:{code:-32601,message:"unknown"} });
 }});`);
  return { root, cwd, cliPath, configPath, behaviorPath };
}

async function supervisor(t, behavior = {}) {
  const f = await fixture(t, behavior);
  const runtimeDir = path.join(f.root, "runtime");
  const live = createLiveSupervisor({ runtimeDir, cliPath: f.cliPath, configPath: f.configPath, env: { ...process.env, FAKE_BEHAVIOR: f.behaviorPath } });
  t.after(() => live.close());
  return { ...f, live, runtimeDir };
}

test("starts a legacy session, subscribes, and uses v4 sendText for delivery", async (t) => {
  const { live, cwd } = await supervisor(t);
  const started = await live.handle({ op: "start", cwd, mode: "plan", brief: "safe fake only", workerId: "worker-a", provider: "builtin:bigmodel-coding-plan", model: "GLM-5.3" });
  assert.equal(started.workerId, "worker-a");
  assert.equal(started.sessionId, "upstream-1");
  const steered = await live.handle({ op: "steer", workerId: "worker-a", delivery: "queue", text: "continue", commandId: "queue-1" });
  assert.equal(steered.delivery, "queue");
  assert.equal(steered.commandId, "queue-1");
  const duplicate = await live.handle({ op: "steer", workerId: "worker-a", delivery: "queue", text: "continue", commandId: "queue-1" });
  assert.equal(duplicate.duplicate, true);
  const stopped = await live.handle({ op: "stop", workerId: "worker-a" });
  assert.equal(stopped.lifecycle, "stopped");
  assert.equal((await live.handle({ op: "close", workerId: "worker-a" })).closed, true);
});

test("passes and returns a supported resolved reasoning level", async (t) => {
  const { live, cwd } = await supervisor(t, { strictReasoning: true });
  const started = await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "reason-a", reasoning: "high" });
  assert.equal(started.reasoning, "high");
  await assert.rejects(
    () => live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "reason-b", reasoning: "medium" }),
    (cause) => cause.code === "reasoning_invalid",
  );
});

test("requires ownership for editing modes and keeps metadata credential-free", async (t) => {
  const { live, cwd, runtimeDir } = await supervisor(t);
  await assert.rejects(() => live.handle({ op: "start", cwd, mode: "edit", brief: "x", workerId: "edit-a" }), /ownership/);
  await live.handle({ op: "start", cwd, mode: "edit", brief: "x", workerId: "edit-a", ownership: { isolatedWorktree: true } });
  const metadata = await readFile(path.join(runtimeDir, "workers", "edit-a.json"), "utf8");
  assert.equal(metadata.includes("secret-key"), false);
  assert.equal(metadata.includes("secret.example"), false);
  await assert.rejects(
    () => live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "../escape" }),
    (cause) => cause.code === "worker_id_invalid",
  );
});

test("requires the desktop provider map's exact GLM-5.3 model and rejects undocumented modes", async (t) => {
  const { live, cwd, configPath } = await supervisor(t);
  await assert.rejects(() => live.handle({ op: "start", cwd, mode: "auto", brief: "x", workerId: "auto-a" }), /mode/);
  await writeFile(configPath, JSON.stringify({ provider: { "builtin:bigmodel-coding-plan": { options: { apiKey: "unrecognizable-configured-credential", baseURL: "https://secret.example" }, models: {} } } }));
  await assert.rejects(() => live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "wrong-model" }), /model/);
});

test("coalesces deltas, preserves material cursor ordering, and times out with a snapshot", async (t) => {
  const { live, cwd } = await supervisor(t, { deltas: true });
  const started = await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "delta-a" });
  const waited = await live.handle({ op: "wait", workerId: "delta-a", cursor: 0, timeoutMs: 10 });
  assert.equal(waited.events.some((event) => event.type === "part.delta"), false);
  assert.equal(waited.events.some((event) => event.type === "turn.completed"), true);
  assert.equal(waited.snapshot.activity.deltaCount, 2);
  const timeout = await live.handle({ op: "wait", workerId: "delta-a", cursor: waited.cursor, timeoutMs: 1 });
  assert.equal(timeout.status, "timeout");
  assert.equal(started.workerId, "delta-a");
});

test("retains session subscribe backfill events and accepts a nested event envelope", async (t) => {
  const { live, cwd } = await supervisor(t, { backfill: true, deltas: true, nested: true });
  await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "backfill-a" });
  const waited = await live.handle({ op: "wait", workerId: "backfill-a", cursor: 0, timeoutMs: 10 });
  assert.equal(waited.events.some((event) => event.type === "turn.completed"), true);
  assert.equal(waited.snapshot.activity.deltaCount, 2);
});

test("rejects an upstream sequence gap instead of silently losing progress", async (t) => {
  const { live, cwd } = await supervisor(t, { gap: true });
  await assert.rejects(
    () => live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "gap-a" }),
    (cause) => cause.code === "protocol_sequence_gap",
  );
});

test("replays events when a status snapshot reports an advanced sequence", async (t) => {
  const { live, cwd } = await supervisor(t, { readAhead: true });
  const started = await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "replay-a" });
  await live.handle({ op: "status", workerId: "replay-a" });
  const waited = await live.handle({ op: "wait", workerId: "replay-a", cursor: started.cursor, timeoutMs: 10 });
  assert.deepEqual(waited.events.map((event) => event.type), ["turn.started", "turn.completed"]);
});

test("fails status when a snapshot sequence regresses", async (t) => {
  const { live, cwd } = await supervisor(t, { backfill: true, readBehind: true });
  await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "read-regress" });
  const status = await live.handle({ op: "status", workerId: "read-regress" });
  assert.equal(status.lifecycle, "failed");
  assert.equal(status.lastError.code, "protocol_sequence_regression");
});

test("redacts exact credentials from material events and later steering failures", async (t) => {
  const eventFixture = await supervisor(t, { secretEvent: true });
  await eventFixture.live.handle({ op: "start", cwd: eventFixture.cwd, mode: "plan", brief: "x", workerId: "secret-event" });
  const waited = await eventFixture.live.handle({ op: "wait", workerId: "secret-event", cursor: 0, timeoutMs: 10 });
  assert.equal(JSON.stringify(waited).includes("unrecognizable-configured-credential"), false);

  const steerFixture = await supervisor(t, { rejectSteerSecret: true });
  await steerFixture.live.handle({ op: "start", cwd: steerFixture.cwd, mode: "plan", brief: "x", workerId: "secret-steer" });
  await assert.rejects(
    () => steerFixture.live.handle({ op: "steer", workerId: "secret-steer", delivery: "guide", text: "x" }),
    (cause) => !String(cause).includes("unrecognizable-configured-credential"),
  );
});

test("coalesces message, part, and non-failing tool updates", async (t) => {
  const { live, cwd } = await supervisor(t, { noisy: true });
  await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "noise-a" });
  const waited = await live.handle({ op: "wait", workerId: "noise-a", cursor: 0, timeoutMs: 10 });
  assert.equal(waited.events.some((event) => event.type === "part.upserted"), false);
  assert.equal(waited.events.some((event) => event.type === "message.upserted"), false);
  assert.equal(waited.events.some((event) => event.type === "reasoning_delta"), false);
  assert.equal(waited.events.some((event) => event.type === "tool_input_delta"), false);
  assert.equal(waited.events.filter((event) => event.type === "tool.updated").length, 1);
  assert.equal(waited.events.find((event) => event.type === "tool.updated").payload.status, "failed");
});

test("bounds the retained material event ring by aggregate bytes", async (t) => {
  const { live, cwd } = await supervisor(t, { bigEvents: true });
  await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "bytes-a" });
  await assert.rejects(
    () => live.handle({ op: "wait", workerId: "bytes-a", cursor: 0, timeoutMs: 10 }),
    (cause) => cause.code === "cursor_expired",
  );
});

test("serializes worker mutations so stop cannot overtake steering", async (t) => {
  const { live, cwd } = await supervisor(t, { delaySteer: true });
  await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "serial-a" });
  const order = [];
  const steering = live.handle({ op: "steer", workerId: "serial-a", delivery: "guide", text: "x", commandId: "serial-guide" }).then(() => order.push("steer"));
  const stopping = live.handle({ op: "stop", workerId: "serial-a" }).then(() => order.push("stop"));
  await Promise.all([steering, stopping]);
  assert.deepEqual(order, ["steer", "stop"]);
});

test("closing a worker resolves a pending wait with an ordered terminal event", async (t) => {
  const { live, cwd } = await supervisor(t);
  const started = await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "close-wait" });
  const waiting = live.handle({ op: "wait", workerId: "close-wait", cursor: started.cursor, timeoutMs: 1000 });
  await live.handle({ op: "close", workerId: "close-wait" });
  const result = await waiting;
  assert.equal(result.status, "event");
  assert.equal(result.events.at(-1).type, "worker.closed");
});

test("normal close retains metadata until the owned wrapper exits", async (t) => {
  const { live, cwd, runtimeDir } = await supervisor(t, { delayTerm: 100 });
  await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "close-owned" });
  const closing = live.handle({ op: "close", workerId: "close-owned" });
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(JSON.parse(await readFile(path.join(runtimeDir, "workers", "close-owned.json"), "utf8")).workerId, "close-owned");
  assert.equal((await closing).closed, true);
  await assert.rejects(readFile(path.join(runtimeDir, "workers", "close-owned.json"), "utf8"));
});

test("normal close force-terminates an owned child that ignores SIGTERM", async (t) => {
  const { live, cwd, runtimeDir } = await supervisor(t, { ignoreTerm: true });
  await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "close-forced" });
  const startedAt = Date.now();
  assert.equal((await live.handle({ op: "close", workerId: "close-forced" })).closed, true);
  assert.ok(Date.now() - startedAt >= 400);
  await assert.rejects(readFile(path.join(runtimeDir, "workers", "close-forced.json"), "utf8"));
});

test("rejects unsupported and malformed protocol frames without credential disclosure", async (t) => {
  for (const behavior of [{ unsupported: true }, { malformed: true }]) {
    const { live, cwd } = await supervisor(t, behavior);
    await assert.rejects(() => live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: `bad-${Boolean(behavior.unsupported)}` }), (error) => !String(error).includes("secret-key"));
  }
});

test("rejects a missing CLI before creating a child", async (t) => {
  const f = await fixture(t);
  const live = createLiveSupervisor({ runtimeDir: path.join(f.root, "missing-cli-runtime"), cliPath: path.join(f.root, "missing-zcode.mjs"), configPath: f.configPath, env: { ...process.env, FAKE_BEHAVIOR: f.behaviorPath } });
  t.after(() => live.close());
  await assert.rejects(
    () => live.handle({ op: "start", cwd: f.cwd, mode: "plan", brief: "x", workerId: "missing-cli" }),
    (cause) => cause.code === "cli_missing",
  );
});

test("uses exact session/read params and removes metadata after initial-send failure", async (t) => {
  const { live, cwd, runtimeDir } = await supervisor(t, { strictRead: true });
  await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "read-a" });
  await live.handle({ op: "status", workerId: "read-a" });
  const rejecting = await supervisor(t, { rejectInitialSend: true });
  await assert.rejects(() => rejecting.live.handle({ op: "start", cwd: rejecting.cwd, mode: "plan", brief: "x", workerId: "cleanup-a" }), /initial send failed/);
  await assert.rejects(readFile(path.join(rejecting.runtimeDir, "workers", "cleanup-a.json"), "utf8"));
  assert.equal(runtimeDir.includes("runtime"), true);
});

test("reports cursor expiry, sequence regression, stale metadata, and child exit conservatively", async (t) => {
  const { live, cwd, runtimeDir } = await supervisor(t, { regress: true });
  const started = await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "regress-a" });
  await new Promise((resolve) => setTimeout(resolve, 15));
  const status = await live.handle({ op: "status", workerId: "regress-a" });
  assert.equal(status.lifecycle, "failed");
  await assert.rejects(() => live.handle({ op: "wait", workerId: "regress-a", cursor: -1 }), /cursor/);
  await writeFile(path.join(runtimeDir, "workers", "old.json"), JSON.stringify({ workerId: "old", sessionId: "s", lifecycle: "running" }));
  const recovered = createLiveSupervisor({ runtimeDir, cliPath: "/missing", configPath: "/missing" });
  t.after(() => recovered.close());
  const stale = await recovered.handle({ op: "status", workerId: "old" });
  assert.equal(stale.available, false);
  assert.equal(started.workerId, "regress-a");
});

test("cleans an orphaned owned worker only after its birth token matches", async (t) => {
  const { live, cwd, runtimeDir, cliPath, configPath, behaviorPath } = await supervisor(t, { delayTerm: 100 });
  await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "orphan-a" });
  const saved = JSON.parse(await readFile(path.join(runtimeDir, "workers", "orphan-a.json"), "utf8"));
  assert.equal(Number.isInteger(saved.processPid), true);
  assert.equal(typeof saved.processIdentityToken, "string");
  const recovered = createLiveSupervisor({ runtimeDir, cliPath, configPath, env: { ...process.env, FAKE_BEHAVIOR: behaviorPath } });
  t.after(() => recovered.close());
  assert.equal((await recovered.handle({ op: "status", workerId: "orphan-a" })).available, false);
  const closing = recovered.handle({ op: "close", workerId: "orphan-a" });
  await new Promise((resolve) => setTimeout(resolve, 20));
  assert.equal(JSON.parse(await readFile(path.join(runtimeDir, "workers", "orphan-a.json"), "utf8")).workerId, "orphan-a");
  const closed = await closing;
  assert.equal(closed.closed, true);
  assert.equal(closed.stale, true);
  assert.equal(closed.orphanTerminated, true);
  await assert.rejects(readFile(path.join(runtimeDir, "workers", "orphan-a.json"), "utf8"));
});

test("turns an app-server child exit into a bounded failed state", async (t) => {
  const { live, cwd } = await supervisor(t, { exitAfterCommand: true });
  await live.handle({ op: "start", cwd, mode: "plan", brief: "x", workerId: "exit-a" });
  await new Promise((resolve) => setTimeout(resolve, 15));
  const status = await live.handle({ op: "status", workerId: "exit-a" });
  assert.equal(status.lifecycle, "failed");
  assert.equal(status.lastError.code, "child_exit");
});

test("serves a private Unix socket and the client reuses an existing daemon", async (t) => {
  const { live, runtimeDir } = await supervisor(t);
  const server = await serveSupervisor({ supervisor: live, runtimeDir });
  t.after(() => server.close());
  const result = await requestLiveSupervisor({ op: "ping" }, { runtimeDir, startDaemon: false });
  assert.equal(result.ok, true);
  assert.equal(defaultRuntimeDir().includes(String(process.getuid?.() ?? "unknown")), true);
});

test("a second supervisor cannot unlink an active supervisor socket", async (t) => {
  const firstFixture = await supervisor(t);
  const first = await serveSupervisor({ supervisor: firstFixture.live, runtimeDir: firstFixture.runtimeDir });
  t.after(() => first.close());
  const secondLive = createLiveSupervisor({ runtimeDir: firstFixture.runtimeDir, cliPath: firstFixture.cliPath, configPath: firstFixture.configPath, env: { ...process.env, FAKE_BEHAVIOR: firstFixture.behaviorPath } });
  t.after(() => secondLive.close());
  await assert.rejects(
    () => serveSupervisor({ supervisor: secondLive, runtimeDir: firstFixture.runtimeDir }),
    (cause) => cause.code === "supervisor_already_running",
  );
  assert.equal((await requestLiveSupervisor({ op: "ping" }, { runtimeDir: firstFixture.runtimeDir, startDaemon: false })).ok, true);
});

test("concurrent supervisors serialize stale socket recovery", async (t) => {
  const runtimeDir = await mkdtemp(path.join(os.tmpdir(), "zl-race-"));
  t.after(() => rm(runtimeDir, { recursive: true, force: true }));
  const socket = path.join(runtimeDir, "supervisor.sock");
  const stale = spawn(node, ["-e", "require('node:net').createServer().listen(process.argv[1],()=>process.stdout.write('ready'));setInterval(()=>{},1000)", socket], { stdio: ["ignore", "pipe", "pipe"] });
  await new Promise((resolve, reject) => {
    let stderr = "";
    stale.stderr.setEncoding("utf8"); stale.stderr.on("data", (chunk) => { stderr += chunk; });
    stale.once("error", reject);
    stale.once("exit", (code) => reject(new Error(`stale socket fixture exited ${code}: ${stderr}`)));
    stale.stdout.once("data", resolve);
  });
  stale.kill("SIGKILL");
  await new Promise((resolve) => stale.once("exit", resolve));
  const mock = () => ({ handle: async () => ({ ok: true }), hasWorkers: () => false, close() {} });
  const outcomes = await Promise.allSettled([
    serveSupervisor({ supervisor: mock(), runtimeDir }),
    serveSupervisor({ supervisor: mock(), runtimeDir }),
  ]);
  const started = outcomes.find((outcome) => outcome.status === "fulfilled");
  const rejected = outcomes.find((outcome) => outcome.status === "rejected");
  try {
    assert.ok(started);
    assert.equal(rejected?.reason?.code, "supervisor_already_running");
    assert.equal((await requestLiveSupervisor({ op: "ping" }, { runtimeDir, startDaemon: false })).ok, true);
  } finally {
    if (started) await started.value.close();
  }
});

test("shuts down an idle socket after its final worker closes", async (t) => {
  const { live, runtimeDir, cwd } = await supervisor(t);
  const server = await serveSupervisor({ supervisor: live, runtimeDir, idleShutdownMs: 10 });
  t.after(() => server.close());
  await requestLiveSupervisor({ op: "start", cwd, mode: "plan", brief: "x", workerId: "idle-a" }, { runtimeDir, startDaemon: false });
  await requestLiveSupervisor({ op: "close", workerId: "idle-a" }, { runtimeDir, startDaemon: false });
  await new Promise((resolve) => setTimeout(resolve, 30));
  await assert.rejects(() => requestLiveSupervisor({ op: "ping" }, { runtimeDir, startDaemon: false }));
});

test("diagnoses compatibility without creating a session or sending model input", async (t) => {
  const { live, cwd } = await supervisor(t);
  const result = await live.handle({ op: "diagnose", cwd });
  assert.equal(result.compatible, true);
  assert.equal(result.provider, "builtin:bigmodel-coding-plan");
  assert.equal(result.model, "GLM-5.3");
});

test("shuts down an idle diagnostic-only daemon", async (t) => {
  const { live, runtimeDir, cwd } = await supervisor(t);
  const server = await serveSupervisor({ supervisor: live, runtimeDir, idleShutdownMs: 10 });
  t.after(() => server.close());
  await requestLiveSupervisor({ op: "diagnose", cwd }, { runtimeDir, startDaemon: false });
  await new Promise((resolve) => setTimeout(resolve, 30));
  await assert.rejects(() => requestLiveSupervisor({ op: "ping" }, { runtimeDir, startDaemon: false }));
});
