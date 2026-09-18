---
name: adaptive-model-router
description: Select a sufficient live native, Z Code, or OpenCode agent backend, model, and reasoning effort for isolated Mornlea work while coordinating controller-worker handoffs, token efficiency, and provider quotas.
---

# Adaptive Model Router

Use this project-owned skill before every new Mornlea subagent delegation. It is a required project governance rule and complements `mornlea-implementation-orchestration`: orchestration applies the isolation-first execution policy; this skill selects the backend, model, and reasoning effort for that isolated work.

The routing objectives are coequal: protect code quality and token efficiency. The main agent is the controller for the whole task; a delegated worker owns a bounded execution node. Do not downshift below credible sufficiency merely to save tokens, and do not use the highest model or effort by default when a lower tier can produce a verifiable result.

## Controller/Worker Operating Contract

The controller (main agent) owns the global picture and the final result:

- Translate the user request into scope, dependencies, ownership, acceptance criteria, and the required validation tier.
- Freeze the contract and file boundary for each delegated node before dispatching it.
- Integrate worker output, resolve cross-node conflicts, run the final gates, update the change ledger, and make any commit or external handoff.

The worker owns the concrete bounded node described in its brief. A worker brief MUST state the evidence, owned paths or isolated worktree, non-goals, integration point, and acceptance command. The controller must not silently expand the brief while the worker is running.

Use this execution state machine for every delegated node:

```text
DECOMPOSE -> DISPATCH -> WAIT -> HANDOFF -> INTEGRATE
                         |          |
                         +-> NEEDS-INPUT / FAILED
```

After `DISPATCH`, the controller enters `WAIT` for that node. While a worker is active, the controller MUST use a cursor-aware bounded wait or status operation (`wait_threads`/worker `wait`, normally 30-60 seconds) and wake only for a phase change, checkpoint, test/tool failure, permission or user-input request, or terminal state. A timeout with unchanged state is not permission to redo the task; retain the cursor, back off, and wait again.

The controller MUST NOT, while the worker is active:

- repeat repository searches, file reads, planning, implementation, or tests over the worker's owned paths;
- start a replacement worker for the same node merely because commentary is delayed;
- inspect or rewrite an equivalent implementation in the main worktree;
- poll token-level output or issue repeated status calls without a cursor.

During `WAIT`, permitted work is limited to waiting/status/steering, answering an explicit worker request, handling a real failure, or progressing a separately scoped non-overlapping node. Any independent work MUST be recorded as such and MUST NOT consume the worker's files or acceptance evidence. If the controller notices a possible issue, send one targeted steer or wait for the worker's terminal report; take over only after `FAILED`, `NEEDS-INPUT`, an explicit scope change, or a verified worker exit.

At `HANDOFF`, read the worker's terminal summary once, inspect only its owned diff, and run the focused acceptance command. Then return to `INTEGRATE` for cross-boundary checks and final validation. Record the worker id, exact route, state transitions, ownership boundary, wait outcome, and any correction in the change ledger. This handoff record prevents the controller from rediscovering the same context on the next turn.

## Delegate-First Threshold

To honor the controller/worker split without creating trivial coordination overhead, use a fresh Grok worker as the first eligible candidate for a bounded implementation, test, debugging, exploration, or review node when any of these is true:

- the node spans two or more files or packages;
- the expected tool horizon is at least three meaningful calls;
- it needs independent repository discovery or review;
- keeping the trace in the controller would materially enlarge context or invite repeated work.

Keep tiny one-file deterministic edits, a change that is inseparable from the controller's current integration step, and final cross-node validation in the controller. Do not dispatch overlapping editing workers. The higher Grok dispatch frequency is a default for eligible bounded nodes, not a waiver of the high-level OpenAI design gate, file ownership, validation, or the two-worker concurrency ceiling.

## Discover the Live Capability Set

Treat the current invocation surface as authoritative. Read the native agent tool's model and reasoning-effort enums or another read-only, account-scoped capability source. The repository's Z Code bridge is a second verified invocation surface for its one configured worker. Do not route from a remembered or copied general model catalog.

When availability or effort support is unclear, read [capability discovery](references/capability-discovery.md). Keep discovery read-only: do not modify configuration, install providers, buy capacity, redeem credits, or send paid probes merely to compare options.

## Route Native, Z Code, and OpenCode Workers

Prefer the native delegation surface when its eligible model is sufficient and native lifecycle, tool, or review integration materially reduces coordination cost. For architecture, feature-design, and other high-level system tasks, apply the mandatory OpenAI design gate before considering any non-OpenAI score. For ordinary bounded work, Z Code may compete only when its probe passes, validation is strong, quota is not confirmed exhausted, and the local time is outside the disabled window. A model absent from the native tool enum cannot be injected by this skill.

The native delegation surface exposes `grok-4.6` with `xhigh` reasoning. Treat the exact pair as one eligible configuration and pass both values explicitly when the native tool accepts overrides. Do not infer Grok availability from a public catalog or substitute another Grok variant when the live native surface does not expose this exact pair.

Z Code `GLM-5.3` is available through the `scripts/zcode-agent.mjs` runtime packaged with this skill as an external isolated agent, not as a model on the native delegation surface. The bridge reads the enabled desktop provider locally, passes its credential to the child process only, returns machine-readable JSON, and never copies the credential into project configuration or task output. Resolve `router_skill_dir` to the absolute directory containing the selected `adaptive-model-router/SKILL.md`; do not depend on a project-root bridge. Verify the route immediately before routing:

```bash
router_skill_dir=/absolute/path/to/selected/adaptive-model-router
node "$router_skill_dir/scripts/zcode-agent.mjs" probe
node "$router_skill_dir/scripts/zcode-agent.mjs" live-probe --cwd /absolute/worktree
```

The first command verifies the desktop provider and model without inference. The second verifies the private live app-server contract without creating a session or sending a model prompt; require it before selecting the live supervisor.

For a short, completion-only task, start a synchronous worker with a concise brief on stdin:

```bash
node "$router_skill_dir/scripts/zcode-agent.mjs" run --cwd /absolute/worktree --mode plan
```

The response includes a session ID. Continue the same worker, preserving native-like multi-turn communication without replaying its context:

```bash
node "$router_skill_dir/scripts/zcode-agent.mjs" send --session <session-id> --cwd /absolute/worktree --mode plan
```

For a long-running task that benefits from progress filtering or intervention, prefer the live supervisor:

```bash
node "$router_skill_dir/scripts/zcode-agent.mjs" start --cwd /absolute/worktree --mode plan --reasoning high
node "$router_skill_dir/scripts/zcode-agent.mjs" wait --worker <worker-id> --cursor <cursor> --timeout-ms 30000
node "$router_skill_dir/scripts/zcode-agent.mjs" status --worker <worker-id>
node "$router_skill_dir/scripts/zcode-agent.mjs" steer --worker <worker-id> --delivery guide --command-id <stable-id>
node "$router_skill_dir/scripts/zcode-agent.mjs" stop --worker <worker-id>
node "$router_skill_dir/scripts/zcode-agent.mjs" close --worker <worker-id>
```

Pass the start brief and steering text through stdin unless a short non-sensitive `--prompt` is more practical. Pass the exact selected `low`, `high`, or `max` reasoning level to live `start` and record the resolved value returned by the bridge; synchronous `run` and `send` inherit the configured desktop default. `guide` targets the active turn at a safe boundary, `queue` preserves the active turn and schedules later work, and `startNow` preempts active work before starting the replacement instruction. Give every steering intent a stable command ID and reuse it after an ambiguous retry so the upstream protocol can deduplicate it. Treat acceptance as an acknowledgement rather than proof the instruction has already been consumed. Keep a returned cursor and wait from it; wake the controller only for phase changes, checkpoints, tool or test failures, permission or input requests, and terminal states. Do not forward token-level text, reasoning, or tool-input deltas into the controller context.

Use `plan` for read-only discovery and review. Use an editing mode only when the worker has an isolated worktree or an exclusive, non-overlapping file set and pass the bridge's explicit ownership assertion; the controller still owns integration and validation. Count the Z Code worker against the two-agent concurrency ceiling. Its startup context is substantial, so select it for bounded work that benefits from isolation and enough repository reasoning to repay that bootstrap cost, not for trivial extraction or one-line edits. The live supervisor can only wake a controller with a pending wait; it cannot inject an unsolicited turn into an idle Codex task.

## OpenCode allocation policy

OpenCode is a second external isolated-agent backend exposed only through the skill-local `scripts/opencode-agent.mjs` bridge. Keep its route exact and immutable:

```text
backend: opencode
provider: opencode-go
model: muse-spark-1.3-contributor
reasoning: xhigh
```

The bridge must reject every other provider, model, or reasoning value before invoking OpenCode. `probe` is read-only and validates the installed OpenCode model metadata; `run` and `send` always pass `--model opencode-go/muse-spark-1.3-contributor`, `--variant xhigh`, `--format json`, `--agent plan|build`, and the selected absolute `--dir`. For long-running work, use `live-probe --cwd` before `start`; the live supervisor preserves the same fixed route and supports `wait`, `status`, `steer`, `stop`, and `close`. `guide` maps to an active-turn `steer`, `queue` preserves the active turn, and `startNow` interrupts before steering the replacement brief. Use stable command IDs and retain the returned cursor exactly as for the native and Z Code supervisors.

OpenCode's Muse Spark 1.3 contributor model has a verified `1,048,576` token context limit, `131,072` maximum output, reasoning, tool-call, and attachment support. Historical local OpenCode operational data contains about 13.6k assistant messages for this model, about 13.1k with tool calls and about 99.8% with a normal stop or tool-call finish; the small failure tail is dominated by user cancellation, connection closure, regional availability, and rate limiting. These are reliability and workflow observations, not a benchmark claim or proof of quality superiority. Route it as a strong bounded explorer and implementer for long-context repository search, multi-step tool work, decomposition into child sessions, code review, ordinary implementation, and debugging with a clear validation oracle. Treat it as weaker for cross-boundary architecture, protocol/ABI/storage contracts, durable ownership or lifecycle design, high-level feature design, and work with a weak oracle or irreversible consequences. Those tasks remain behind the OpenAI design gate and OpenCode does not compete on quota or score.

For ordinary eligible tasks, give OpenCode a bounded selection prior that reflects its lower contributor quota without treating it as exhausted:

```text
OpenCode prior = 0.92
quota factor = 0.90 + 0.20 × quota ratio
OpenCode score = base score × OpenCode prior × quota factor × time factor
quota ratio = clamp(remaining / limit, 0, 1)
time factor = 1.00 outside the Z Code disabled window
```

Use a fresh, read-only, non-secret OpenCode quota snapshot when the host exposes one. A valid snapshot has `used`, `limit`, or `remaining`, an `observed_at`, and an optional `reset_at`; derive missing `remaining` from `limit - used`. A malformed, missing, or more-than-15-minute-old snapshot is unknown quota and uses the neutral ratio `0.50`. Confirmed zero remaining or an actual OpenCode rate-limit response removes OpenCode until reset or a fresh positive snapshot. Missing quota telemetry alone does not mean zero quota. Apply the existing local `14:00–18:00` Z Code disabled-window filter before scoring; the window disables Z Code only, so OpenCode may still be considered outside the high-level OpenAI gate when its own probe is healthy and quota is not confirmed exhausted. Record the route, quota observation time, and eligibility reason without recording provider output or credentials.

OpenCode live workers count against the same two-agent concurrency ceiling. Use `plan` for read-only exploration and review. Use `build`, `edit`, or `yolo` only with an isolated worktree or explicit exclusive-file ownership, and let the controller own integration and final validation. Preserve the worker session ID and use `send --session` for synchronous multi-turn continuation; use the live supervisor when the controller needs cursor-based progress, intervention, or queued follow-up work. The OpenCode session API exposes parent and child sessions and the bridge's `steer`/`queue` operations, which provide native-like multi-turn subagent communication without replaying prior context.

## Grok 4.6 allocation policy

Grok 4.6 is a native worker configuration with a fixed reasoning level:

```text
backend: native
model: grok-4.6
reasoning: xhigh
```

Use it for complex bounded repository exploration, multi-step tool work, ordinary implementation, debugging, code review, test diagnosis, and synthesis across several packages when the validation oracle is clear. Its high reasoning setting makes it suitable for ambiguous bounded work that benefits from sustained investigation. Prefer a lower native effort or a smaller eligible model for extraction, formatting, trivial deterministic edits, and short checks where `xhigh` would add cost without improving the result. Require an independent review or stronger validation for security-sensitive, irreversible, weak-oracle, or cross-boundary changes. Architecture, feature design, package ownership, lifecycle, concurrency, protocol, ABI, storage, and other durable system decisions still go through the OpenAI design gate before Grok can be considered.

For an ordinary bounded node that satisfies the delegate-first threshold, Grok is the default first dispatch. Keeping that node in the controller requires an explicit reason such as a one-file deterministic edit, inseparable integration ownership, or a missing isolated execution surface. This raises useful Grok delegation frequency while preserving the high-level design gate and final controller accountability.

After eligibility filters and the OpenAI design gate, give Grok a higher but bounded selection prior:

```text
Grok prior = 1.12
Grok score = base score × Grok prior
```

This prior is a routing preference, not a measured accuracy claim. It is deliberately higher than the current Z Code prior (`1.06`) and the maximum unknown/full-quota OpenCode modifier (`0.92 × 1.10`), so Grok wins when the remaining candidates have equivalent task fit, validation strength, lifecycle support, isolation benefit, quota efficiency, and latency. It does not override explicit user constraints, native availability, provider health, validation requirements, or the mandatory OpenAI gate. Do not invent a Grok quota value; apply a quota modifier only when the live native host reports a current, non-secret quota signal.

## High-level OpenAI design gate

Before applying any quota or backend score, classify the task from its requested outcome and affected boundaries. Code architecture, package or module ownership, dependency direction, lifecycle or concurrency design, protocol/ABI/storage contracts, multi-component feature design, cross-component user behavior, OpenSpec proposal/design, and other durable system decisions are high-level tasks. If the brief is ambiguous and could change a durable boundary or multiple components, classify it as high-level.

For a high-level task, filter the candidate set to native OpenAI-backed models and select the highest eligible OpenAI model under the project ceiling, currently `gpt-5.6-sol`, with `high` or `max` reasoning. Z Code and every other non-OpenAI backend MUST NOT compete through quota or routing scores. If no compliant advanced OpenAI configuration is exposed by the live native surface, report an unavailable route and MUST NOT silently substitute Z Code.

## ZCode allocation policy

Keep Z Code in normal rotation outside the local 14:00–18:00 disabled window, with a modestly higher overall share, while retaining native fallback for high-consequence or weak-oracle work. This is a bounded routing preference, not a guarantee that Z Code wins every task.

Immediately before scoring an ordinary Z Code candidate, first apply the high-level gate and the local-time filter. During `14:00–18:00` in the host's local timezone, remove Z Code from the candidate set and do not spend quota on it. Outside that disabled window, obtain a fresh, non-secret quota snapshot from a read-only account-scoped usage source exposed by the host or provider. Accept `used`, `limit`, and `remaining` values plus `observed_at` and optional `reset_at`; when only `used` and `limit` are available, derive `remaining = limit - used`. Do not send an inference request just to discover quota, print credentials, or treat a missing snapshot as zero quota. A snapshot older than 15 minutes, malformed, or missing a positive `limit` is `stale quota` and becomes `unknown quota` for routing.

Normalize the live value as `quota ratio = clamp(remaining / limit, 0, 1)`. Apply the following deterministic `score modifier` after the six-axis base score:

```text
ZCode score = base score × Z Code prior × quota factor × time factor (outside the disabled window)
Z Code prior = 1.06
quota factor = 0.90 + 0.20 × quota ratio
time factor = 1.00 outside the disabled window
```

The neutral `unknown quota` case uses a ratio of `0.50`, so it keeps the modest Z Code prior without pretending that capacity is either full or exhausted. A confirmed zero `remaining` value or an actual provider rate-limit response removes Z Code from that decision until the next reset or a fresh positive snapshot; an unavailable quota source does not. This keeps the total Z Code preference slightly higher while still conserving a genuinely depleted entitlement.

Use the host's local timezone for the time check. During `14:00–18:00`, Z Code MUST be disabled and omitted before scoring, regardless of probe health or quota. Outside the disabled window, Z Code remains eligible only when its probe is healthy and quota is not confirmed exhausted; fall back for an actual capability, quota, provider, user, or validation constraint. Record the local timezone, disabled-window classification, and quota observation time in the routing decision, never the credential or raw account response.

## Apply Constraints and Project Ceilings

Honor explicit user constraints for provider, model, effort, budget, latency, privacy, region, and tools before optimizing. An explicit user selection is not silently replaced.

For an OpenAI-backed model, the highest eligible model is `gpt-5.6-sol` and the highest eligible reasoning effort is `max`. Options above either boundary may be visible but remain ineligible unless the user changes this project rule. A compatible gateway does not remove the ceiling. This ceiling does not apply to non-OpenAI providers; route those options within their own verified constraints.

If provider identity or model ordering cannot be verified, retain the current eligible configuration rather than guessing. Do not describe inheritance as adaptive optimization.

## Route from Multiple Angles

Assess the isolated task on independent axes:

- reasoning difficulty, ambiguity, and novelty;
- consequence of an incorrect result and ease of rollback;
- context breadth, dependency depth, and architectural coupling;
- expected tool-call horizon and autonomy;
- required modalities and tool support;
- strength and speed of the available validation oracle;
- user preferences for speed, cost, and quality.

Filter incompatible or ineligible options first. From the remaining live host capability set, choose the lowest-cost configuration that is credibly sufficient. Model capability and reasoning effort are separate decisions: a larger model does not automatically require maximum effort.

For two otherwise compatible candidates, calculate a deterministic routing score rather than sampling by weight:

- task and capability fit: 35%;
- validation strength: 20%;
- lifecycle and intervention integration: 15%;
- main-context isolation benefit: 15%;
- total token or quota efficiency: 10%;
- startup and expected completion latency: 5%.

Score each axis from current task evidence. These weights are routing policy, not measured model accuracy or selection probability. Strong automated validation increases the eligibility of a lower-cost isolated implementer; weak validation and high consequence increase the capability requirement even when isolation value is high.

Use this conservative prior for the configured Z Code worker until repository-specific evaluations justify a change:

- `GLM-5.3 low`: roughly the routing band from a highest-effort fast native worker through a low-to-medium balanced native worker; use for read-heavy exploration and narrow mechanical work only when bootstrap cost is repaid.
- `GLM-5.3 high`: roughly the ordinary balanced-native medium-to-high band; use for bounded implementation, debugging, and review with a strong validation oracle.
- `GLM-5.3 max`: roughly the balanced-native high-to-max through flagship-native medium band; use for long-context implementation and agentic tool work with strong tests.

These bands are conservative task-routing judgments, not assertions of model equivalence. Do not treat `GLM-5.3` as the default substitute for an eligible flagship native model on cross-boundary architecture, security, irreversible operations, or weak-oracle work. For those tasks, prefer the eligible native flagship or use Z Code as an isolated implementer followed by an independent native review. Prefer Z Code initially for long-context repository exploration and bounded implementation with strong tests; prefer a direct or low-bootstrap native path for tiny deterministic work.

Use semantic bands only as routing guidance, then select exact values exposed by the host:

- Lowest available effort: extraction, formatting, classification, deterministic edits, and narrow checks with strong validation.
- Low: bounded research, small isolated code changes, and straightforward transformations.
- Medium: ordinary implementation, synthesis across several sources, debugging with a plausible hypothesis, and multi-step tool use.
- High: ambiguous diagnosis, architecture, security or correctness review, and consequential work with interacting constraints.
- Above high: only the hardest eligible work when failure is costly, evaluation is difficult, and the added token and latency cost is justified.

## Apply, Verify, and Adjust

For a native worker, pass the exact selected model and effort through the native delegation surface. For Grok, that means `grok-4.6` with `xhigh`; if the host returns resolved values, treat them as truth. Use a fresh or minimal context fork and a concise task brief; preserve only the evidence, paths, constraints, ownership, integration point, and acceptance criteria needed for a correct result. For the external Z Code and OpenCode workers, record the bridge probe, exact provider/model, mode, reasoning, and returned session ID as the resolved route.

Escalate only one eligible capability or effort step after observable insufficiency, such as failed validation, unresolved contradictions, repeated planning failure, or material uncertainty. Prefer targeted verification and follow-up over restarting completed expensive work. Downshift later independent tasks when the work becomes repetitive or mechanically verifiable.

Record a compact routing decision in the change ledger when delegation occurs:

```text
model: <exact resolved identifier or inherited>
effort: <exact supported value or provider default>
backend: <native | zcode-cli | opencode-cli>
basis: <task axes and capability source>
fallback: <next eligible configuration if validation fails>
```

Distinguish host-reported facts from routing judgment.

## Round-End Router Retrospective

At the end of every implementation round, evaluate routing alongside the architecture retrospective. Review both over-routing and under-routing:

- Was a model or effort tier higher than the task and its validation oracle required?
- Did a lower choice cause retries, contradictions, missing context, weak review, or avoidable escalation?
- Did context transfer, tool support, latency, or token use materially affect quality?
- Were eligible bounded nodes dispatched to Grok instead of being redundantly re-executed by the controller?
- After dispatch, did the controller actually remain in cursor-aware `WAIT` until a wake condition, without repeating the worker's searches, reads, implementation, or tests?
- Did the handoff summary and ownership record prevent the controller from rediscovering the worker's context?
- Would a reusable axis, constraint, or escalation rule improve future routing?

Update both project-owned skill copies only when verified evidence supports a stable cross-task rule that changes future routing. Do not add task history, volatile availability lists, one-off model anecdotes, or unverified preferences. Validate and synchronize both copies after an update. If no reusable improvement qualifies, record `Model router: no change` with a short reason in the change ledger.
