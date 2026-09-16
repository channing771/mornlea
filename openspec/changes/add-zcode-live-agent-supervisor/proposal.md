## Why

Mornlea can invoke the desktop-configured Z Code `GLM-5.3` worker, but the current bridge blocks until a turn finishes and returns only a final JSON result. That prevents the Codex controller from observing meaningful progress, mediating approval or input requests, or steering a long-running isolated task before completion, which limits the context-isolation and token-efficiency benefits of external delegation.

## What Changes

- Add a persistent local Z Code supervisor that owns one `app-server` child process per delegated session and preserves its event cursor, lifecycle, and sanitized diagnostics.
- Expose machine-readable commands to start, wait for, inspect, steer, stop, and close an isolated Z Code task without replaying its accumulated context.
- Coalesce raw streaming deltas into bounded progress snapshots and wake the controller only for meaningful lifecycle, checkpoint, failure, approval, input, and completion events.
- Support three explicit intervention semantics: safe-boundary guidance, next-turn queuing, and active-turn preemption.
- Keep the existing synchronous `probe`, `run`, and `send` interface compatible while documenting when the live supervisor should be preferred.
- Package the bridge, supervisor, and focused tests inside each synchronized `adaptive-model-router` skill so the skill owns its executable runtime and remains self-contained.
- Update the synchronized model-router policy with a conservative `GLM-5.3` capability prior and a validation-aware scoring rubric, while treating project evaluation results rather than vendor benchmarks as the promotion gate.

The change does not make `GLM-5.3` a native Codex model, provide unsolicited delivery into an idle model turn, alter Z Code desktop configuration, or permit overlapping editing ownership.

## Capabilities

### New Capabilities

- `zcode-live-agent-supervision`: Observable lifecycle, progress waiting, bounded status, intervention, recovery, and credential-safe operation for an external Z Code worker controlled by Codex.

### Modified Capabilities

None.

## Impact

- Affected code: synchronized `scripts/` resources inside `.codex/skills/adaptive-model-router/` and `.claude/skills/adaptive-model-router/`; no root-level agent bridge remains under `scripts/agents/`.
- Affected policy: synchronized `.codex` and `.claude` `adaptive-model-router` skills and their capability-discovery guidance.
- Dependencies: Node.js standard library and the locally installed Z Code CLI protocol; no new package dependency or checked-in credential.
- Compatibility: existing synchronous bridge commands remain supported; the live protocol is additive and version-gated because the Z Code `app-server` surface is not a stable Codex-native API.
- Saves and game/network protocols: not applicable; no world save, gameplay protocol, schema, ABI, or benchmark version changes.
- Concurrency: the supervisor serializes commands per Z Code session, persists an event cursor, and counts each live worker against the existing two-subagent ceiling.
- Performance: progress output is bounded and coalesced; long polling avoids busy polling, and small tasks remain ineligible when Z Code bootstrap cost exceeds the isolation benefit.
