import assert from "node:assert/strict";
import { chmodSync, mkdtempSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import { fileURLToPath } from "node:url";

const testDir = path.dirname(fileURLToPath(import.meta.url));
const bridgePath = path.join(testDir, "opencode-agent.mjs");
const providerId = "opencode-go";
const modelId = "muse-spark-1.3-contributor";
const secret = "opencode-test-secret";

function fixture() {
  const root = mkdtempSync(path.join(tmpdir(), "mornlea-opencode-agent-"));
  const workspace = path.join(root, "workspace");
  const argsFile = path.join(root, "args.json");
  const cli = path.join(root, "fake-opencode");
  mkdirSync(workspace);
  writeFileSync(cli, `#!/usr/bin/env node
const fs = require("node:fs");
const args = process.argv.slice(2);
if (process.env.FAKE_OPENCODE_FAIL === "1") {
  process.stderr.write(process.env.OPENCODE_API_KEY);
  process.exit(7);
}
fs.writeFileSync(process.env.FAKE_OPENCODE_ARGS_FILE, JSON.stringify(args));
if (args[0] === "models") {
  process.stdout.write("opencode-go/${modelId}\\n");
  process.stdout.write(JSON.stringify({
    id: "${modelId}", providerID: "${providerId}", limit: { context: 1048576, output: 131072 },
    capabilities: { reasoning: true, toolcall: true, attachment: true }, variants: { xhigh: { reasoningEffort: "xhigh" } }
  }) + "\\n");
} else if (args[0] === "run") {
  process.stdout.write(JSON.stringify({ type: "session.idle", sessionID: "session-fixture" }) + "\\n");
} else process.exit(8);
`);
  chmodSync(cli, 0o700);
  return { root, workspace, argsFile, cli, env: { ...process.env, MORNLEA_OPENCODE_CLI_PATH: cli, FAKE_OPENCODE_ARGS_FILE: argsFile, OPENCODE_API_KEY: secret } };
}

function invoke(item, args, input) {
  return spawnSync(process.execPath, [bridgePath, ...args], { encoding: "utf8", env: item.env, input });
}

test("probe accepts only the exact Muse Spark route and reports its capabilities", () => {
  const item = fixture();
  const result = invoke(item, ["probe"]);
  assert.equal(result.status, 0, result.stderr);
  const report = JSON.parse(result.stdout);
  assert.deepEqual({ backend: report.backend, providerId: report.providerId, modelId: report.modelId, variant: report.model.variant }, {
    backend: "opencode-cli", providerId, modelId, variant: "xhigh",
  });
  assert.equal(report.model.contextWindow, 1048576);
  assert.doesNotMatch(result.stdout, new RegExp(secret));
});

test("run and send force the exact model, xhigh, and agent mapping", () => {
  const item = fixture();
  const run = invoke(item, ["run", "--cwd", item.workspace, "--mode", "plan", "--prompt", "inspect"]);
  assert.equal(run.status, 0, run.stderr);
  const runArgs = JSON.parse(readFileSync(item.argsFile, "utf8"));
  assert.deepEqual(runArgs.slice(0, 12), ["run", "--model", `${providerId}/${modelId}`, "--variant", "xhigh", "--format", "json", "--agent", "plan", "--dir", item.workspace, "inspect"]);
  const report = JSON.parse(run.stdout);
  assert.equal(report.sessionId, "session-fixture");
  const send = invoke(item, ["send", "--cwd", item.workspace, "--mode", "edit", "--session", "session-existing", "--reasoning", "xhigh", "--prompt", "continue"]);
  assert.equal(send.status, 0, send.stderr);
  const sendArgs = JSON.parse(readFileSync(item.argsFile, "utf8"));
  assert.deepEqual(sendArgs.slice(0, 14), ["run", "--model", `${providerId}/${modelId}`, "--variant", "xhigh", "--format", "json", "--agent", "build", "--dir", item.workspace, "--session", "session-existing", "continue"]);
});

test("non-xhigh reasoning is rejected before invoking OpenCode", () => {
  const item = fixture();
  const result = invoke(item, ["run", "--cwd", item.workspace, "--reasoning", "high", "--prompt", "reject"]);
  assert.notEqual(result.status, 0);
  const failure = JSON.parse(result.stdout);
  assert.equal(failure.error.code, "reasoning_invalid");
  assert.doesNotMatch(`${result.stdout}\n${result.stderr}`, new RegExp(secret));
});

test("provider failures redact environment credentials", () => {
  const item = fixture();
  item.env.FAKE_OPENCODE_FAIL = "1";
  const result = invoke(item, ["run", "--cwd", item.workspace, "--prompt", "fail"]);
  assert.notEqual(result.status, 0);
  assert.doesNotMatch(`${result.stdout}\n${result.stderr}`, new RegExp(secret));
  assert.match(result.stdout, /\[redacted\]/);
});
