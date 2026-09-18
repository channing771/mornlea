#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { existsSync, readFileSync, statSync } from "node:fs";
import path from "node:path";

import { requestOpenCodeSupervisor } from "./opencode-live-agent.mjs";

const PROVIDER_ID = "opencode-go";
const MODEL_ID = "muse-spark-1.3-contributor";
const REASONING = "xhigh";
const MODEL_REF = `${PROVIDER_ID}/${MODEL_ID}`;
const MODES = new Set(["plan", "build", "edit", "yolo"]);
const LIVE_COMMANDS = new Set(["live-probe", "start", "wait", "status", "steer", "stop", "close"]);
const SECRET_KEY = /api[-_]?key|token|secret|authorization|password|baseurl|base_url/i;

function cliPath() {
  return process.env.MORNLEA_OPENCODE_CLI_PATH ?? "opencode";
}

function redact(value) {
  let result = String(value)
    .replace(/(?:Bearer\s+)?(?:sk-|AIza)[A-Za-z0-9._-]+/gi, "[redacted]")
    .replace(/secret[-_a-z0-9.]*/gi, "[redacted]");
  for (const [key, secret] of Object.entries(process.env)) {
    if (SECRET_KEY.test(key) && typeof secret === "string" && secret.length > 0) result = result.split(secret).join("[redacted]");
  }
  return result.slice(0, 12000);
}

function safe(value, depth = 0) {
  if (depth > 4) return "[truncated]";
  if (typeof value === "string") return redact(value).slice(0, 4096);
  if (value === null || typeof value !== "object") return value;
  if (Array.isArray(value)) return value.slice(0, 40).map((item) => safe(item, depth + 1));
  return Object.fromEntries(Object.entries(value)
    .filter(([key]) => !SECRET_KEY.test(key))
    .slice(0, 80)
    .map(([key, item]) => [key, safe(item, depth + 1)]));
}

function failure(code, message) {
  const cause = new Error(redact(message));
  cause.code = code;
  return cause;
}

function parseArguments(argv) {
  const [command, ...rest] = argv;
  const options = {};
  for (let index = 0; index < rest.length; index += 1) {
    const flag = rest[index];
    if (!flag.startsWith("--")) throw failure("argument_invalid", `unexpected argument: ${flag}`);
    const name = flag.slice(2);
    const value = rest[index + 1];
    if (value === undefined || value.startsWith("--")) throw failure("argument_invalid", `missing value for --${name}`);
    options[name] = value;
    index += 1;
  }
  return { command, options };
}

function cwdFor(options) {
  const cwd = path.resolve(options.cwd ?? process.cwd());
  if (!path.isAbsolute(options.cwd ?? process.cwd()) || !existsSync(cwd) || !statSync(cwd).isDirectory()) throw failure("cwd_invalid", "cwd must be an existing absolute directory");
  return cwd;
}

function modeFor(options) {
  const mode = options.mode ?? "plan";
  if (!MODES.has(mode)) throw failure("mode_invalid", `unsupported OpenCode mode: ${mode}`);
  return mode;
}

function agentFor(mode) {
  return mode === "plan" ? "plan" : "build";
}

function requireReasoning(value) {
  if (value !== undefined && value !== REASONING) throw failure("reasoning_invalid", `OpenCode only permits reasoning variant ${REASONING}`);
  return REASONING;
}

function readPrompt(options) {
  const prompt = options.prompt ?? readFileSync(0, "utf8");
  if (!String(prompt).trim()) throw failure("prompt_required", "a non-empty task brief is required via --prompt or stdin");
  return String(prompt).trim();
}

function extractJsonObjects(text) {
  const objects = [];
  let start = -1;
  let depth = 0;
  let quote = false;
  let escaped = false;
  for (let index = 0; index < text.length; index += 1) {
    const character = text[index];
    if (quote) {
      if (escaped) escaped = false;
      else if (character === "\\") escaped = true;
      else if (character === '"') quote = false;
      continue;
    }
    if (character === '"') { quote = true; continue; }
    if (character === "{" && depth === 0) start = index;
    if (character === "{" && start >= 0) depth += 1;
    if (character === "}" && start >= 0) {
      depth -= 1;
      if (depth === 0) {
        try { objects.push(JSON.parse(text.slice(start, index + 1))); } catch {}
        start = -1;
      }
    }
  }
  return objects;
}

function probe() {
  const result = spawnSync(cliPath(), ["models", PROVIDER_ID, "--verbose"], { encoding: "utf8", env: process.env, maxBuffer: 8 * 1024 * 1024 });
  const models = extractJsonObjects(`${result.stdout ?? ""}\n${result.stderr ?? ""}`);
  const model = models.find((candidate) => candidate.providerID === PROVIDER_ID && candidate.id === MODEL_ID);
  const variant = model?.variants?.[REASONING];
  const available = result.status === 0 && Boolean(model) && variant?.reasoningEffort === REASONING;
  const report = {
    available,
    backend: "opencode-cli",
    providerId: PROVIDER_ID,
    modelId: MODEL_ID,
    model: available ? {
      contextWindow: model.limit?.context ?? null,
      maxOutput: model.limit?.output ?? null,
      reasoning: Boolean(model.capabilities?.reasoning),
      toolcall: Boolean(model.capabilities?.toolcall),
      attachments: Boolean(model.capabilities?.attachment),
      variant: REASONING,
    } : null,
    cliPath: cliPath(),
  };
  process.stdout.write(`${JSON.stringify(report)}\n`);
  if (!available) process.exitCode = 1;
  return report;
}

function parseEvents(stdout) {
  return stdout.split(/\r?\n/).map((line) => line.trim()).filter(Boolean).flatMap((line) => {
    try { return [JSON.parse(line)]; } catch { return []; }
  });
}

function runSync(command, options) {
  const cwd = cwdFor(options);
  const mode = modeFor(options);
  const reasoning = requireReasoning(options.reasoning);
  const prompt = readPrompt(options);
  const args = ["run", "--model", MODEL_REF, "--variant", reasoning, "--format", "json", "--agent", agentFor(mode), "--dir", cwd];
  if (command === "send") {
    if (!options.session) throw failure("session_required", "send requires --session with an OpenCode session ID");
    args.push("--session", options.session);
  }
  args.push(prompt);
  const result = spawnSync(cliPath(), args, { cwd, env: process.env, encoding: "utf8", maxBuffer: 32 * 1024 * 1024 });
  const events = parseEvents(result.stdout ?? "");
  if (result.status !== 0) throw failure("provider_failed", redact(result.stderr || result.stdout || `OpenCode exited with ${result.status ?? "unknown"}`));
  const last = events.at(-1) ?? null;
  return {
    backend: "opencode-cli",
    providerId: PROVIDER_ID,
    modelId: MODEL_ID,
    reasoning,
    mode,
    sessionId: last?.sessionID ?? last?.sessionId ?? null,
    eventCount: events.length,
    lastEvent: safe(last),
  };
}

function ownership(options, cwd) {
  if (!options.ownership) return undefined;
  if (options.ownership === "isolated-worktree") return { isolatedWorktree: true };
  if (options.ownership !== "exclusive-files") throw failure("ownership_invalid", "--ownership must be isolated-worktree or exclusive-files");
  const files = String(options["exclusive-files"] ?? "").split(",").map((item) => item.trim()).filter(Boolean).map((item) => path.resolve(cwd, item));
  if (files.length === 0) throw failure("ownership_invalid", "exclusive-files requires --exclusive-files");
  return { exclusiveFiles: files };
}

function nonNegativeInteger(options, name, fallback) {
  if (options[name] === undefined) return fallback;
  const value = Number(options[name]);
  if (!Number.isInteger(value) || value < 0) throw failure("option_invalid", `--${name} must be a non-negative integer`);
  return value;
}

async function live(command, options) {
  if (command === "live-probe") return requestOpenCodeSupervisor({ op: "diagnose", cwd: cwdFor(options) });
  if (command === "start") {
    const cwd = cwdFor(options);
    return requestOpenCodeSupervisor({
      op: "start",
      cwd,
      mode: modeFor(options),
      brief: readPrompt(options),
      provider: PROVIDER_ID,
      model: MODEL_ID,
      reasoning: requireReasoning(options.reasoning),
      ownership: ownership(options, cwd),
    });
  }
  if (!options.worker) throw failure("worker_required", `--worker is required for ${command}`);
  if (command === "wait") return requestOpenCodeSupervisor({ op: "wait", workerId: options.worker, cursor: nonNegativeInteger(options, "cursor", 0), timeoutMs: nonNegativeInteger(options, "timeout-ms", 30000) });
  if (command === "status") return requestOpenCodeSupervisor({ op: "status", workerId: options.worker });
  if (command === "steer") return requestOpenCodeSupervisor({ op: "steer", workerId: options.worker, delivery: options.delivery, commandId: options["command-id"], text: readPrompt(options) });
  if (command === "stop") return requestOpenCodeSupervisor({ op: "stop", workerId: options.worker });
  if (command === "close") return requestOpenCodeSupervisor({ op: "close", workerId: options.worker });
  throw failure("command_invalid", `unsupported OpenCode command: ${command}`);
}

export { MODEL_ID, MODEL_REF, PROVIDER_ID, REASONING, extractJsonObjects, parseArguments, probe, runSync };

async function main() {
  const { command, options } = parseArguments(process.argv.slice(2));
  if (command === "probe") { probe(); return; }
  if (command === "run" || command === "send") { process.stdout.write(`${JSON.stringify(runSync(command, options))}\n`); return; }
  if (LIVE_COMMANDS.has(command)) { process.stdout.write(`${JSON.stringify(await live(command, options))}\n`); return; }
  throw failure("command_invalid", `unsupported OpenCode command: ${command}`);
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  main().catch((cause) => {
    process.stdout.write(`${JSON.stringify({ ok: false, error: { code: cause.code ?? "opencode_error", message: redact(cause.message ?? String(cause)) } })}\n`);
    process.exitCode = 1;
  });
}
