#!/usr/bin/env node

import { spawnSync } from "node:child_process";
import { existsSync, readFileSync, statSync } from "node:fs";
import os from "node:os";
import path from "node:path";

import { requestLiveSupervisor } from "./zcode-live-agent.mjs";

const providerId = "builtin:bigmodel-coding-plan";
const modelId = "GLM-5.3";
const defaultCliPath = "/Applications/ZCode.app/Contents/Resources/glm/zcode.cjs";
const defaultConfigPath = path.join(os.homedir(), ".zcode", "v2", "config.json");
const supportedModes = new Set(["plan", "build", "edit", "yolo"]);

function fail(message, code = 1) {
  process.stderr.write(`${message}\n`);
  process.exit(code);
}

function parseArguments(argv) {
  const [command, ...rest] = argv;
  const options = {};
  for (let index = 0; index < rest.length; index += 1) {
    const flag = rest[index];
    if (!flag.startsWith("--")) {
      fail(`Unexpected argument: ${flag}`, 2);
    }
    const name = flag.slice(2);
    const value = rest[index + 1];
    if (value === undefined || value.startsWith("--")) {
      fail(`Missing value for ${flag}`, 2);
    }
    options[name] = value;
    index += 1;
  }
  return { command, options };
}

function loadConfiguration() {
  const cliPath = process.env.MORNLEA_ZCODE_CLI_PATH ?? defaultCliPath;
  const configPath = process.env.MORNLEA_ZCODE_CONFIG_PATH ?? defaultConfigPath;
  let config;
  try {
    config = JSON.parse(readFileSync(configPath, "utf8"));
  } catch (error) {
    fail(`Cannot read Z Code desktop configuration at ${configPath}: ${error.message}`);
  }

  const providers = config.provider ?? config.providers ?? {};
  const provider = providers[providerId];
  const model = provider?.models?.[modelId];
  const apiKey = provider?.options?.apiKey;
  const baseURL = provider?.options?.baseURL ?? provider?.options?.baseUrl;
  const variants = model?.reasoning?.variants;
  const reasoningLevels = Array.isArray(variants)
    ? variants
    : variants && typeof variants === "object"
      ? Object.keys(variants)
      : [];
  const cliPresent = existsSync(cliPath) && statSync(cliPath).isFile();

  return {
    apiKey,
    baseURL,
    cliPath,
    cliPresent,
    configPath,
    model,
    provider,
    reasoningLevels,
  };
}

function probe(configuration) {
  const credentialPresent =
    typeof configuration.apiKey === "string" && configuration.apiKey.length > 0;
  const available = Boolean(
    configuration.cliPresent &&
      configuration.provider?.enabled !== false &&
      configuration.model &&
      credentialPresent &&
      configuration.baseURL,
  );
  process.stdout.write(
    `${JSON.stringify({
      available,
      backend: "zcode-cli",
      providerId,
      modelId,
      reasoningDefault: configuration.model?.reasoning?.defaultVariant ?? null,
      reasoningLevels: configuration.reasoningLevels,
      credentialPresent,
      cliPath: configuration.cliPath,
      configPath: configuration.configPath,
    })}\n`,
  );
  if (!available) {
    process.exitCode = 1;
  }
}

function redact(value, secrets) {
  let sanitized = value;
  for (const secret of secrets) {
    if (typeof secret === "string" && secret.length > 0) {
      sanitized = sanitized.split(secret).join("[redacted]");
    }
  }
  return sanitized;
}

function parseProviderOutput(stdout, secrets) {
  const sanitized = redact(stdout.trim(), secrets);
  try {
    return JSON.parse(sanitized);
  } catch {
    const lines = sanitized.split(/\r?\n/).filter(Boolean);
    for (let index = lines.length - 1; index >= 0; index -= 1) {
      try {
        return JSON.parse(lines[index]);
      } catch {
        continue;
      }
    }
    fail(`Z Code returned invalid JSON: ${sanitized || "<empty>"}`);
  }
}

function readPrompt(options) {
  const prompt = options.prompt ?? readFileSync(0, "utf8");
  if (prompt.trim().length === 0) {
    fail("A non-empty task brief is required via --prompt or stdin.", 2);
  }
  return prompt.trim();
}

function liveError(code, message) {
  const result = new Error(message);
  result.code = code;
  return result;
}

function readLiveText(options) {
  const text = options.prompt ?? readFileSync(0, "utf8");
  if (text.trim().length === 0) {
    throw liveError("prompt_required", "A non-empty task brief is required via --prompt or stdin.");
  }
  return text.trim();
}

function requireOption(options, name) {
  if (!options[name]) {
    throw liveError("option_required", `--${name} is required.`);
  }
  return options[name];
}

function parseIntegerOption(options, name, defaultValue) {
  if (options[name] === undefined) {
    return defaultValue;
  }
  const value = Number(options[name]);
  if (!Number.isInteger(value) || value < 0) {
    throw liveError("option_invalid", `--${name} must be a non-negative integer.`);
  }
  return value;
}

function parseOwnership(options, cwd) {
  if (!options.ownership) {
    return undefined;
  }
  if (options.ownership === "isolated-worktree") {
    return { isolatedWorktree: true };
  }
  if (options.ownership === "exclusive-files") {
    const files = (options["exclusive-files"] ?? "")
      .split(",")
      .map((value) => value.trim())
      .filter(Boolean)
      .map((value) => path.resolve(cwd, value));
    if (files.length === 0) {
      throw liveError(
        "ownership_invalid",
        "--ownership exclusive-files requires --exclusive-files with a comma-separated list.",
      );
    }
    return { exclusiveFiles: files };
  }
  throw liveError(
    "ownership_invalid",
    "--ownership must be isolated-worktree or exclusive-files.",
  );
}

async function invokeLive(command, options) {
  if (command === "live-probe") {
    const result = await requestLiveSupervisor({
      op: "diagnose",
      cwd: path.resolve(options.cwd ?? process.cwd()),
    });
    return {
      available: result.compatible === true,
      backend: "zcode-live",
      providerId: result.provider,
      modelId: result.model,
      cliIdentity: result.cliIdentity,
    };
  }

  if (command === "start") {
    const cwd = path.resolve(options.cwd ?? process.cwd());
    const mode = options.mode ?? "plan";
    if (!supportedModes.has(mode)) {
      throw liveError("mode_invalid", `Unsupported Z Code mode: ${mode}`);
    }
    const result = await requestLiveSupervisor({
      op: "start",
      cwd,
      mode,
      brief: readLiveText(options),
      provider: providerId,
      model: modelId,
      reasoning: options.reasoning,
      ownership: parseOwnership(options, cwd),
    });
    return {
      backend: "zcode-live",
      providerId,
      modelId,
      mode,
      ...result,
    };
  }

  const workerId = requireOption(options, "worker");
  if (command === "wait") {
    return requestLiveSupervisor({
      op: "wait",
      workerId,
      cursor: parseIntegerOption(options, "cursor", undefined),
      timeoutMs: parseIntegerOption(options, "timeout-ms", 30000),
    });
  }
  if (command === "status") {
    return requestLiveSupervisor({ op: "status", workerId });
  }
  if (command === "steer") {
    return requestLiveSupervisor({
      op: "steer",
      workerId,
      delivery: requireOption(options, "delivery"),
      commandId: requireOption(options, "command-id"),
      text: readLiveText(options),
    });
  }
  if (command === "stop") {
    return requestLiveSupervisor({ op: "stop", workerId });
  }
  if (command === "close") {
    return requestLiveSupervisor({ op: "close", workerId });
  }
  throw liveError("command_invalid", `Unsupported live command: ${command}`);
}

function printLiveFailure(cause) {
  process.stdout.write(
    `${JSON.stringify({
      ok: false,
      error: {
        code: cause.code ?? "zcode_live_error",
        message: redact(cause.message ?? String(cause), []),
      },
    })}\n`,
  );
  process.exitCode = 1;
}

function invoke(command, options, configuration) {
  if (!configuration.cliPresent) {
    fail(`Z Code CLI was not found at ${configuration.cliPath}.`);
  }
  if (!configuration.provider || configuration.provider.enabled === false) {
    fail(`Z Code provider ${providerId} is not enabled.`);
  }
  if (!configuration.model) {
    fail(`Z Code model ${modelId} is not configured.`);
  }
  if (!configuration.apiKey || !configuration.baseURL) {
    fail("Z Code provider credentials or base URL are missing.");
  }

  const cwd = path.resolve(options.cwd ?? process.cwd());
  if (!existsSync(cwd) || !statSync(cwd).isDirectory()) {
    fail(`Working directory does not exist: ${cwd}`, 2);
  }
  const mode = options.mode ?? "plan";
  if (!supportedModes.has(mode)) {
    fail(`Unsupported Z Code mode: ${mode}`, 2);
  }
  if (command === "send" && !options.session) {
    fail("send requires --session with a Z Code session ID.", 2);
  }

  const args = [
    configuration.cliPath,
    "--prompt",
    readPrompt(options),
    "--cwd",
    cwd,
    "--mode",
    mode,
    "--output-format",
    "json",
  ];
  if (command === "send") {
    args.push("--resume", options.session);
  }

  const child = spawnSync(process.execPath, args, {
    encoding: "utf8",
    env: {
      ...process.env,
      ZCODE_API_KEY: configuration.apiKey,
      ZCODE_BASE_URL: configuration.baseURL,
      ZCODE_MODEL: `${providerId}/${modelId}`,
    },
    maxBuffer: 64 * 1024 * 1024,
  });
  const secrets = [configuration.apiKey];
  if (child.error) {
    fail(`Cannot start Z Code: ${redact(child.error.message, secrets)}`);
  }
  if (child.status !== 0) {
    const detail = redact(`${child.stderr}\n${child.stdout}`.trim(), secrets);
    fail(`Z Code failed with exit code ${child.status}: ${detail}`, child.status ?? 1);
  }

  const result = parseProviderOutput(child.stdout, secrets);
  process.stdout.write(
    `${JSON.stringify({
      backend: "zcode-cli",
      providerId,
      modelId,
      mode,
      sessionId: result.sessionId ?? options.session ?? null,
      turnId: result.turnId ?? null,
      traceId: result.traceId ?? null,
      response: result.response ?? null,
      usage: result.usage ?? null,
      projection: result.projection ?? null,
      eventCount: result.eventCount ?? null,
    })}\n`,
  );
}

const { command, options } = parseArguments(process.argv.slice(2));
const synchronousCommands = new Set(["probe", "run", "send"]);
const liveCommands = new Set([
  "live-probe",
  "start",
  "wait",
  "status",
  "steer",
  "stop",
  "close",
]);
if (!command || (!synchronousCommands.has(command) && !liveCommands.has(command))) {
  fail(
    "Usage: zcode-agent.mjs probe | live-probe [--cwd PATH] | run [--cwd PATH] [--mode MODE] [--prompt TEXT] | send --session ID [--cwd PATH] [--mode MODE] [--prompt TEXT] | start [--cwd PATH] [--mode MODE] [--reasoning low|high|max] [--ownership isolated-worktree|exclusive-files] [--exclusive-files PATHS] [--prompt TEXT] | wait --worker ID --cursor N [--timeout-ms N] | status --worker ID | steer --worker ID --delivery guide|queue|startNow --command-id ID [--prompt TEXT] | stop --worker ID | close --worker ID",
    2,
  );
}

if (synchronousCommands.has(command)) {
  const configuration = loadConfiguration();
  if (command === "probe") {
    probe(configuration);
  } else {
    invoke(command, options, configuration);
  }
} else {
  try {
    process.stdout.write(`${JSON.stringify(await invokeLive(command, options))}\n`);
  } catch (cause) {
    printLiveFailure(cause);
  }
}
