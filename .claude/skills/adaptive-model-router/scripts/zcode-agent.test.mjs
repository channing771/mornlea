import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const testDir = path.dirname(fileURLToPath(import.meta.url));
const bridgePath = path.join(testDir, "zcode-agent.mjs");
const providerId = "builtin:bigmodel-coding-plan";
const modelId = "GLM-5.3";
const secret = "fixture-secret-must-not-leak";

function createFixture() {
  const root = mkdtempSync(path.join(tmpdir(), "mornlea-zcode-agent-"));
  const workspace = path.join(root, "workspace");
  const runtimeDir = path.join(root, "runtime");
  const configPath = path.join(root, "config.json");
  const cliPath = path.join(root, "fake-zcode.mjs");
  mkdirSync(workspace);
  writeFileSync(
    configPath,
    JSON.stringify({
      provider: {
        [providerId]: {
          enabled: true,
          kind: "anthropic",
          models: {
            [modelId]: {
              reasoning: {
                defaultVariant: "max",
                enabled: true,
                variants: ["low", "high", "max"],
              },
            },
          },
          options: {
            apiKey: secret,
            baseURL: "https://example.invalid/api/anthropic",
          },
        },
      },
    }),
  );
  writeFileSync(
    cliPath,
    `
const args = process.argv.slice(2);
const key = process.env.ZCODE_API_KEY;
if (process.env.FAKE_ZCODE_FAIL === "1") {
  console.error("provider failure: " + key);
  process.exit(7);
}
if (process.env.ZCODE_MODEL !== ${JSON.stringify(`${providerId}/${modelId}`)}) {
  console.error("unexpected model");
  process.exit(8);
}
if (process.env.ZCODE_BASE_URL !== "https://example.invalid/api/anthropic") {
  console.error("unexpected base URL");
  process.exit(9);
}
if (key !== ${JSON.stringify(secret)}) {
  console.error("missing credential");
  process.exit(10);
}
if (args[0] === "app-server") {
  let buffer = "";
  let sequence = 0;
  const output = (frame) => process.stdout.write(JSON.stringify(frame) + "\\n");
  process.stdin.setEncoding("utf8");
  process.stdin.on("data", (chunk) => {
    buffer += chunk;
    while (buffer.includes("\\n")) {
      const point = buffer.indexOf("\\n");
      const line = buffer.slice(0, point);
      buffer = buffer.slice(point + 1);
      if (!line) continue;
      const request = JSON.parse(line);
      if (request.method === "session/list") {
        output({ id: request.id, result: { sessions: [] } });
      } else if (request.method === "v4/command" && request.params.type === "invalid") {
        output({ id: request.id, error: { code: -32602, message: "invalid command" } });
      } else if (request.method === "session/subscribe" && !request.params.sessionId) {
        output({ id: request.id, error: { code: -32602, message: "sessionId required" } });
      } else if (request.method === "session/create") {
        output({ id: request.id, result: { sessionId: "live_session_fixture" } });
      } else if (request.method === "session/subscribe") {
        output({ id: request.id, result: { sessionId: request.params.sessionId, eventSeq: sequence, events: [], snapshot: { state: "idle" } } });
      } else if (request.method === "v4/command") {
        output({ id: request.id, result: { status: "accepted", result: { type: "inputAccepted", delivery: request.params.payload.requestedDelivery } } });
      } else if (request.method === "session/read") {
        output({ id: request.id, result: { sessionId: request.params.sessionId, eventSeq: sequence, state: "idle" } });
      } else if (request.method === "session/stop") {
        output({ id: request.id, result: {} });
      } else if (request.method === "session/close") {
        output({ id: request.id, result: { closed: true } });
      } else {
        output({ id: request.id, error: { code: -32601, message: "unsupported" } });
      }
    }
  });
} else {
  const prompt = args[args.indexOf("--prompt") + 1];
  const resumeIndex = args.indexOf("--resume");
  const sessionId = resumeIndex >= 0 ? args[resumeIndex + 1] : "sess_fixture_new";
  console.log(JSON.stringify({
    eventCount: 4,
    projection: { contextUsed: 12, contextWindow: 1000000, status: "idle", turnCount: 1 },
    response: JSON.stringify({ prompt, resume: resumeIndex >= 0 }),
    sessionId,
    traceId: "trace_fixture",
    turnId: "turn_fixture",
    usage: { inputTokens: 10, modelRequestCount: 1, outputTokens: 2, totalTokens: 12 },
  }));
}
`,
  );

  return {
    configPath,
    env: {
      ...process.env,
      MORNLEA_ZCODE_CLI_PATH: cliPath,
      MORNLEA_ZCODE_CONFIG_PATH: configPath,
      MORNLEA_ZCODE_RUNTIME_DIR: runtimeDir,
      MORNLEA_ZCODE_IDLE_SHUTDOWN_MS: "25",
    },
    runtimeDir,
    workspace,
  };
}

function invoke(fixture, args, options = {}) {
  return spawnSync(process.execPath, [bridgePath, ...args], {
    encoding: "utf8",
    env: { ...fixture.env, ...options.env },
    input: options.input,
  });
}

test("probe reports the configured GLM worker without exposing credentials", () => {
  const fixture = createFixture();
  const result = invoke(fixture, ["probe"]);

  assert.equal(result.status, 0, result.stderr);
  const report = JSON.parse(result.stdout);
  assert.equal(report.available, true);
  assert.equal(report.backend, "zcode-cli");
  assert.equal(report.providerId, providerId);
  assert.equal(report.modelId, modelId);
  assert.equal(report.credentialPresent, true);
  assert.deepEqual(report.reasoningLevels, ["low", "high", "max"]);
  assert.doesNotMatch(result.stdout, new RegExp(secret));
});

test("run creates a fresh isolated ZCode session from a stdin brief", () => {
  const fixture = createFixture();
  const result = invoke(
    fixture,
    ["run", "--cwd", fixture.workspace, "--mode", "plan"],
    { input: "Inspect the bounded subsystem and report evidence." },
  );

  assert.equal(result.status, 0, result.stderr);
  const report = JSON.parse(result.stdout);
  assert.equal(report.backend, "zcode-cli");
  assert.equal(report.providerId, providerId);
  assert.equal(report.modelId, modelId);
  assert.equal(report.sessionId, "sess_fixture_new");
  assert.equal(report.mode, "plan");
  assert.deepEqual(JSON.parse(report.response), {
    prompt: "Inspect the bounded subsystem and report evidence.",
    resume: false,
  });
  assert.doesNotMatch(result.stdout, new RegExp(secret));
});

test("send resumes the named ZCode session", () => {
  const fixture = createFixture();
  const result = invoke(
    fixture,
    [
      "send",
      "--cwd",
      fixture.workspace,
      "--mode",
      "plan",
      "--session",
      "sess_existing-123",
      "--prompt",
      "Verify the failing assertion only.",
    ],
  );

  assert.equal(result.status, 0, result.stderr);
  const report = JSON.parse(result.stdout);
  assert.equal(report.sessionId, "sess_existing-123");
  assert.deepEqual(JSON.parse(report.response), {
    prompt: "Verify the failing assertion only.",
    resume: true,
  });
});

test("provider failures redact the desktop credential", () => {
  const fixture = createFixture();
  const result = invoke(
    fixture,
    ["run", "--cwd", fixture.workspace, "--prompt", "Fail safely."],
    { env: { FAKE_ZCODE_FAIL: "1" } },
  );

  assert.notEqual(result.status, 0);
  assert.doesNotMatch(`${result.stdout}\n${result.stderr}`, new RegExp(secret));
  assert.match(`${result.stdout}\n${result.stderr}`, /\[redacted\]/);
});

test("live commands expose start, status, wait, steering, stop, and close", () => {
  const fixture = createFixture();
  const startedResult = invoke(
    fixture,
    ["start", "--cwd", fixture.workspace, "--mode", "plan", "--reasoning", "high"],
    { input: "Inspect the bounded fake workspace." },
  );

  assert.equal(startedResult.status, 0, startedResult.stderr);
  const started = JSON.parse(startedResult.stdout);
  assert.equal(started.backend, "zcode-live");
  assert.equal(started.providerId, providerId);
  assert.equal(started.modelId, modelId);
  assert.equal(started.reasoning, "high");
  assert.equal(started.sessionId, "live_session_fixture");
  assert.equal(typeof started.workerId, "string");
  assert.equal(Number.isInteger(started.cursor), true);

  const statusResult = invoke(fixture, ["status", "--worker", started.workerId]);
  assert.equal(statusResult.status, 0, statusResult.stderr);
  assert.equal(JSON.parse(statusResult.stdout).workerId, started.workerId);

  const steerResult = invoke(
    fixture,
    ["steer", "--worker", started.workerId, "--delivery", "guide", "--command-id", "guide-fixture-1"],
    { input: "Report only material progress." },
  );
  assert.equal(steerResult.status, 0, steerResult.stderr);
  assert.equal(JSON.parse(steerResult.stdout).delivery, "guide");
  assert.equal(JSON.parse(steerResult.stdout).commandId, "guide-fixture-1");

  const waitResult = invoke(fixture, [
    "wait",
    "--worker",
    started.workerId,
    "--cursor",
    String(JSON.parse(steerResult.stdout).cursor),
    "--timeout-ms",
    "5",
  ]);
  assert.equal(waitResult.status, 0, waitResult.stderr);
  assert.equal(JSON.parse(waitResult.stdout).status, "timeout");

  const stopResult = invoke(fixture, ["stop", "--worker", started.workerId]);
  assert.equal(stopResult.status, 0, stopResult.stderr);
  assert.equal(JSON.parse(stopResult.stdout).lifecycle, "stopped");

  const closeResult = invoke(fixture, ["close", "--worker", started.workerId]);
  assert.equal(closeResult.status, 0, closeResult.stderr);
  assert.equal(JSON.parse(closeResult.stdout).closed, true);
});

test("live-probe validates app-server compatibility without creating a session", () => {
  const fixture = createFixture();
  const result = invoke(fixture, ["live-probe", "--cwd", fixture.workspace]);

  assert.equal(result.status, 0, result.stderr);
  const report = JSON.parse(result.stdout);
  assert.equal(report.available, true);
  assert.equal(report.backend, "zcode-live");
  assert.equal(report.providerId, providerId);
  assert.equal(report.modelId, modelId);
  assert.doesNotMatch(result.stdout, new RegExp(secret));
});

test("live editing requires an explicit ownership assertion", () => {
  const fixture = createFixture();
  const rejected = invoke(
    fixture,
    ["start", "--cwd", fixture.workspace, "--mode", "edit"],
    { input: "Do not run without ownership." },
  );

  assert.notEqual(rejected.status, 0);
  const failure = JSON.parse(rejected.stdout);
  assert.equal(failure.ok, false);
  assert.equal(failure.error.code, "ownership_required");
  assert.doesNotMatch(`${rejected.stdout}\n${rejected.stderr}`, new RegExp(secret));
});
