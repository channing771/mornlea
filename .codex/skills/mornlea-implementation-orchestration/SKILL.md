---
name: mornlea-implementation-orchestration
description: Select direct, delegated, or mixed implementation execution for Mornlea changes under the provider-aware project policy.
---

# Mornlea Implementation Orchestration

Use this skill before implementing an OpenSpec change, multi-step repair, or refactor in Mornlea. It selects the execution shape; it does not replace the task's implementation skill or its completion criteria.

## Select the Mode

Use OpenAI-native mode only when the controlling runtime or host verifies that the controller is ChatGPT or Codex using an OpenAI model. Do not infer provider identity from a model-name substring, repository content, environment variable, or model self-description.

Use strict SDD mode when the controller is non-OpenAI or its provider identity cannot be verified.

An explicit user instruction requiring or prohibiting subagents controls the execution shape. Higher-priority runtime restrictions also control. Otherwise, the project policy gives a verified OpenAI controller standing authorization to choose main-agent work, subagents, or a mixed approach without asking for separate per-task delegation permission.

## OpenAI-Native Mode

Main-agent execution is the default. At most two subagents may run concurrently.

- Work directly for small or low-context tasks.
- Delegate only for material context isolation when a bounded feature, research, or review context is large, noisy, or specialized enough to pollute the main agent's architectural reasoning.
- Do not delegate merely for parallel speed, independent file ownership, or unused capacity.
- Before each new delegation, use the project-owned `adaptive-model-router` with the live host capability set. Evaluate difficulty, consequence, context breadth, tool horizon, validation strength, and user priorities; choose the lowest-cost model and reasoning effort credibly sufficient for the isolated task. Both code quality and token efficiency are coequal routing objectives.
- Escalate one eligible step only after observable insufficiency. Do not restart an already-running agent solely to change its model.
- Give every delegated task an explicit isolation boundary, ownership, integration point, and expected validation.
- Choose review depth proportionally to risk, and record the execution shape and rationale in the change ledger.

The controller remains responsible for integration and completion evidence.

## Git Checkpoints

After an independently verifiable task or small coherent feature node passes its focused gates, create a scoped Git commit before starting the next node. Use partial staging to exclude unrelated, user-owned, experimental, or not-yet-complete work; never use a broad commit merely to empty a dirty worktree. If pre-existing changes prevent a safe commit, record the exact overlap and resolve the ownership boundary before accumulating more implementation.

## Round-End Governance Retrospective

At the end of each implementation round, review verified ownership, dependency, lifecycle, concurrency, platform, visual, validation, and documentation findings. Promote a finding to the synchronized project-owned `mornlea-architecture` skill only when current code, tests, or canonical specifications verify it; it applies across future tasks; it changes future decisions; and it is neither duplicated nor volatile. Otherwise record `Architecture skill: no change` in the ledger.

Also review the project-owned `adaptive-model-router` for over-routing, under-routing, avoidable retries or escalation, context-transfer cost, validation quality, and token use. Update both project copies only when evidence supports a stable cross-task routing improvement; otherwise record `Model router: no change` in the ledger.

## Strict SDD Mode

Read and follow the available `subagent-driven-development` skill. Use its independent implementation and review responsibilities, keep the task brief as the requirements source, and record progress and rulings in the change ledger.

## Invariants in Both Modes

- Preserve the approved OpenSpec scope and reconcile artifacts before implementing a design change.
- Use test-first development for behavior changes and keep unrelated or user-owned work untouched.
- Respect file ownership, destructive-action safeguards, and authorization requirements for externally consequential actions.
- Run every required focused and stage-boundary gate; orchestration freedom never waives validation.
- Record material decisions, review rulings, validation evidence, and blockers in the change ledger.
