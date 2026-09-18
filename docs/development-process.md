---
doc_id: development-process
doc_revision: 2026-09-16.3
language: en
counterpart: development-process.zh.md
---
# Mornlea development process

This is the single current process document. `docs/feature-backlog.md`, role cards in `docs/agents/`, GitHub Discussion #71, and task briefs reference it. Code, tests, and `openspec/specs/` are authoritative.

## Workflow policy

OpenAI-native orchestration is isolation-first. A verified OpenAI ChatGPT/Codex controller has standing authorization to choose direct, delegated, or mixed execution. Prefer a fresh agent for bounded repository discovery, multi-file reasoning, specialized review, or a long trace whose main-context retention cost exceeds its handoff cost; keep only tiny, tightly coupled, or cheaper-to-finish work in the controller. Parallel speed and unused capacity are not sufficient by themselves, and no more than three subagents may run concurrently. Give each worker a concise task brief and a fresh or minimal context. Before every new delegation, use the project-owned `adaptive-model-router` with live host capabilities. Evaluate difficulty, consequence, context breadth, tool horizon, validation strength, worker bootstrap cost, and user priorities, then select the lowest-cost model and effort credibly sufficient for the work. Code quality and token efficiency are coequal goals: neither routine work on an excessive tier nor risky work on an inadequate tier is acceptable. A non-OpenAI or unknown-provider controller must use strict `subagent-driven-development`, including independent implementation and review. Every mode preserves scope, ownership, test-first work, validation, and authorization boundaries.

The router may use a native subagent or the external Z Code `GLM-5.3` bridge packaged in `adaptive-model-router/scripts/`. Resolve the selected skill root and run its `scripts/zcode-agent.mjs probe` before selecting Z Code, start a fresh isolated session with `run`, and use the returned session ID with `send` for follow-up turns. The bridge is not a native Codex model registration. Editing sessions require an isolated worktree or exclusive files and remain subject to controller integration and validation.

At the end of each implementation round, promote only stable cross-task architectural conventions into `mornlea-architecture`; otherwise record `Architecture skill: no change`. Also review routing for over-routing, under-routing, retries, escalation, context-transfer cost, validation quality, and token use. Update both project `adaptive-model-router` copies only for verified reusable improvements; otherwise record `Model router: no change`.

## Stages

### Roles and claim discipline

The controller coordinates and rules on work; it may implement directly under the provider policy, but must not bypass required review. A planner maintains planning material and does not claim tasks or modify feature code. A reviewer independently checks the task’s changed behavior. Claim only one `ready` backlog row, change it to `claimed`, record `<agent> @ <branch>` and the exclusive file set, and do not transfer a claim without controller ruling. `queued` and `design candidate` rows are not claimable.

### 0. Claim
Read `docs/feature-backlog.md` and `openspec/config.yaml`; claim only a `ready` row. Record the owner and exclusive file set, claim one row at a time, and preserve unrelated dirty worktree changes.

### 1. Clarify
Classify the work as `spike`, `bounded`, or `architectural`; inspect source, tests, history, and the task source; clarify purpose, boundaries, success criteria, and constraints one question at a time; and present a short design for explicit approval before implementation. The confirmation channel is device-first (`confirm.sh ask` → Feishu reply → `feishu-listener.js`/`AGENT_RESUME`); if unavailable or timed out, use the structured GitHub Discussion fallback and stop at the confirmation point. If requirements change, update OpenSpec artifacts first.

### 2. Isolate and specify
Substantial work uses an isolated worktree/branch. Complex features, new modules, cross-package refactors, save/protocol, concurrency, or performance-contract changes require `proposal.md`, delta specs, `design.md`, `tasks.md`, and `ledger.md`, validated with:

```bash
openspec validate --all --strict --no-interactive
```

Spelling, formatting, and disposable experiments may be direct with proportionate validation.

### 3. Implement
Follow `tasks.md` and red → green → refactor. Keep tests with code, one topic per test file, one shared-helper center per package, and synchronize cross-language constants in one task. For delegation, provide a concise brief containing only the task, necessary evidence and paths, baseline SHA, relevant change artifacts, constraints, ownership, integration point, and exact validation; do not copy the whole controller transcript. An OpenAI ChatGPT/Codex controller decides whether a separate reviewer adds enough context isolation or risk reduction; it is not required to use a one-round implementation/one-round review pattern. Non-OpenAI or unknown-provider controllers retain the strict fresh-implementer and independent-review pattern. Record progress, review when performed, evidence, and `Ruling: <decision> — <reason> — <mistake addressed>` in `ledger.md`; copy unresolved items into proposal.md’s “Deferred and abandoned” section. Reuse evidence by baseline SHA only when unchanged; focused review checks changed behavior, while full race remains a gate.

### 4. Branch review and gates

```bash
make rust
make test-race
go vet ./packages/contracts/... ./packages/shared/... ./packages/server/... ./packages/client/... ./packages/tools/... ./packages/audit/...
test -z "$(gofmt -l .)"
openspec validate --all --strict --no-interactive
```

Add applicable benchmark, fuzz/golden, visual, and platform gates. Benchmarks are informational; overflow, data loss, report identity, and I/O errors are hard failures. Removed automatic Hooks remain removed; maintain only `scripts/agent-hooks/guard.mjs` and its tests.

### 5. Closeout
Confirm `go test -list` sets when splitting files; sync delta specs and archive each change; update scoped guidance and progress only with verified facts. Behavior changes use PR/CI (`gh pr create`, `gh pr checks --watch`, repair and repeat until green, then `gh pr merge --merge`); pure sync/archive documentation may merge directly after local gates. Preserve historical evidence and unresolved items.

## Parallelism and conflicts
Protocol, save-schema, engine/client ABI, and benchmark-scenario upgrades are mutually exclusive. Versioned core gameplay is serial. Parallel work is allowed only when file ownership and version impact do not overlap. Freeze scope after claiming and reconcile OpenSpec artifacts before changing it.

## Quick reference

| Stage | Action | Key output |
|---|---|---|
| 0 | Claim | owner and exclusive file set |
| 1 | Clarify | approved design decision |
| 2 | Specify | validated OpenSpec change |
| 3 | Implement | tested change and ledger |
| 4 | Gate | review and validation evidence |
| 5 | Close | synchronized and archived change |
