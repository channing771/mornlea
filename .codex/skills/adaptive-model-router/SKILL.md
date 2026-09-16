---
name: adaptive-model-router
description: Select the lowest-cost live native or Z Code agent backend, model, and reasoning effort sufficient for isolated Mornlea work while protecting code quality and token efficiency.
---

# Adaptive Model Router

Use this project-owned skill before every new Mornlea subagent delegation. It is a required project governance rule and complements `mornlea-implementation-orchestration`: orchestration applies the isolation-first execution policy; this skill selects the backend, model, and reasoning effort for that isolated work.

The routing objectives are coequal: protect code quality and token efficiency. Do not downshift below credible sufficiency merely to save tokens, and do not use the highest model or effort by default when a lower tier can produce a verifiable result.

## Discover the Live Capability Set

Treat the current invocation surface as authoritative. Read the native agent tool's model and reasoning-effort enums or another read-only, account-scoped capability source. The repository's Z Code bridge is a second verified invocation surface for its one configured worker. Do not route from a remembered or copied general model catalog.

When availability or effort support is unclear, read [capability discovery](references/capability-discovery.md). Keep discovery read-only: do not modify configuration, install providers, buy capacity, redeem credits, or send paid probes merely to compare options.

## Route Native and Z Code Workers

Prefer the native delegation surface when its eligible model is sufficient and native lifecycle, tool, or review integration materially reduces coordination cost. A model absent from the native tool enum cannot be injected by this skill.

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

For a native worker, pass the exact selected model and effort through the native delegation surface. If the host returns resolved values, treat them as truth. Use a fresh or minimal context fork and a concise task brief; preserve only the evidence, paths, constraints, ownership, integration point, and acceptance criteria needed for a correct result. For the external Z Code worker, record the bridge probe, exact provider/model, mode, and returned session ID as the resolved route.

Escalate only one eligible capability or effort step after observable insufficiency, such as failed validation, unresolved contradictions, repeated planning failure, or material uncertainty. Prefer targeted verification and follow-up over restarting completed expensive work. Downshift later independent tasks when the work becomes repetitive or mechanically verifiable.

Record a compact routing decision in the change ledger when delegation occurs:

```text
model: <exact resolved identifier or inherited>
effort: <exact supported value or provider default>
backend: <native or zcode-cli>
basis: <task axes and capability source>
fallback: <next eligible configuration if validation fails>
```

Distinguish host-reported facts from routing judgment.

## Round-End Router Retrospective

At the end of every implementation round, evaluate routing alongside the architecture retrospective. Review both over-routing and under-routing:

- Was a model or effort tier higher than the task and its validation oracle required?
- Did a lower choice cause retries, contradictions, missing context, weak review, or avoidable escalation?
- Did context transfer, tool support, latency, or token use materially affect quality?
- Would a reusable axis, constraint, or escalation rule improve future routing?

Update both project-owned skill copies only when verified evidence supports a stable cross-task rule that changes future routing. Do not add task history, volatile availability lists, one-off model anecdotes, or unverified preferences. Validate and synchronize both copies after an update. If no reusable improvement qualifies, record `Model router: no change` with a short reason in the change ledger.
