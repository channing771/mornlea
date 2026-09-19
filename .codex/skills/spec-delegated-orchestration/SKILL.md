---
name: spec-delegated-orchestration
description: Define model assignments for spec-driven task breakdowns and a durable per-task result store that the later main model can read. Use when one main model owns specs while separately launched models produce task results.
---

# Spec Task Model Allocation

This skill is a planning and handoff protocol only. It defines which available model and reasoning band belongs to each decomposed task, and where that task's intermediate and final results are stored. It does not prescribe how a worker is launched, what tools it uses, how it edits code, how it waits, or how it validates its work. Those decisions remain with the user and the worker runtime.

## Manual Dispatch Boundary

The controller is plan-only for worker execution. The controller MUST NOT call `spawn_agent` or any equivalent dispatch, launch, scheduling, polling, cancellation, or worker-control API. The controller MUST NOT start a worker because an assignment is eligible, because capacity is available, or because project policy permits delegation.

The user or a runtime explicitly controlled by the user owns selecting the concrete worker, starting it, sequencing it after dependencies, stopping it, and writing its observable result. The controller may write the assignment protocol, create result directories, and create honest `planned` handoffs. A later, explicitly requested integration pass may read returned results and reconcile the OpenSpec artifacts; it must not manufacture worker state, output, validation evidence, or changed-file claims.

## Main Model Contract

The main model is responsible for the control plane:

1. Read the applicable repository guidance and current OpenSpec artifacts.
2. Decompose the request into leaf requirements, then group them into independently addressable work packages with stable assignment IDs. Choose the package size from shared ownership, context, dependencies, and acceptance gates; do not mechanically assign one worker per checkbox.
3. Write or update the relevant spec documents and tasks.md before the user-controlled launch, then assign exactly one non-controller candidate model and one reasoning band to each pending work package in the current round.
4. Create the result directory and an honest planned handoff before the user-controlled runtime starts the task.
5. Only after the user explicitly requests integration following a user-controlled runtime result, read the result index, each task's final result, and any needed checkpoints.
6. During that explicitly requested integration pass, reconcile verified conclusions into proposal.md, delta specs, design.md, tasks.md, and the change ledger as appropriate.

The main model remains the owner of spec meaning and cross-task conclusions. A worker result is an input to that process, not an automatic change to a spec.

## Assignment Record

For an OpenSpec change, keep the assignment index at:

    openspec/changes/<change-name>/delegation/assignments.yaml

Each pending task or grouped work package in the current delegation round must have a matching assignment entry. Completed tasks from earlier rounds are historical evidence and MUST NOT be retroactively assigned or given fabricated worker results. The index SHOULD declare its scope as `pending-frontier` when it covers only unfinished tasks. The entry records:

    change: example-change
    round: 1
    dispatch_owner: user
    dispatch_mode: manual
    controller_dispatch: prohibited
    tasks:
      - id: block-11
        title: complete one cohesive architecture work package
        covers: ["11.1", "11.2"]
        model: GLM 5.3
        reasoning: medium
        result_dir: delegation/results/block-11/
        depends_on: []

Use the exact candidate labels supplied by the user: GLM 5.3, GLM 5.3 Flash, Muse spark, a separately launched ChatGPT worker model, or Grok 4.6. The current main model is never assigned as its own worker. If the execution host maps a label to a different provider ID, add resolved_model beside model; do not replace the user-facing assignment.

The assignment index is planning data, not a worker instruction sheet. Keep leaf-task scope and acceptance intent in tasks.md; use `covers` in assignments.yaml to show which leaf tasks a larger work package handles. Keep model allocation and result locations in assignments.yaml so the later main model has one predictable index to read. Controller-owned spec writing and final integration are recorded as controller work, not as a self-assigned worker task.

Prefer one work package when tasks share the same owner, repository context, lifecycle, and validation chain. Split a package only when independent dependencies, ownership boundaries, failure recovery, or review value justify the extra handoff. A package may cover adjacent numbered tasks and still requires one coherent final handoff.

An assignment with `state: planned` is a reservation the user may dispatch, not evidence that a worker has run or been dispatched. Create its result directory and an honest planned handoff before the user-controlled launch. The controller MUST NOT change the status or final handoff to claim execution; only the worker runtime may publish execution state, and it must never pre-fill a successful result, validation claim, or changed-file list.

## Result Locations

Every assignment gets its own directory:

    openspec/changes/<change-name>/delegation/results/<assignment-id>/
      status.yaml
      progress/
        001.md
        002.md
      final.md

The worker or user-controlled runtime may choose the contents of each result, but these locations are fixed:

- status.yaml identifies the task or work package, assigned model, reasoning band, current state, and the path of the latest final output.
- progress/NNN.md stores numbered stage outputs or checkpoints. Files are append-only snapshots; do not overwrite an earlier stage.
- final.md stores the worker's final output for the current attempt, including whether the task is complete, partial, blocked, or failed.

If a task is retried or its assignment changes, preserve the previous output as final-r01.md, final-r02.md, and so on, and point status.yaml at the newest file. Do not erase an earlier result. A task that produces no useful result still gets a status.yaml and a final.md stating why.

## Main Model Read Order

When the user explicitly requests an integration pass after workers finish, the main model reads work-package results in this order:

1. delegation/assignments.yaml to discover task IDs, assigned models, reasoning bands, dependencies, and result directories.
2. Each task's status.yaml to identify the latest state and final-output path.
3. The latest final*.md named by status.yaml.
4. The numbered progress/ files only when the final output is incomplete, contradictory, or needs historical context.
5. The relevant OpenSpec artifacts and repository evidence before updating any spec.

This read order lets the main model resume from a known index without searching the repository for worker output. Results must not be stored only in chat, an untracked temporary directory, or a model-specific private location.

The controller may record a future assignment before its dependencies are complete, but MUST record those dependencies and keep the task `planned` until the prerequisite result is verified. The user-controlled runtime decides when to run it within the host's permitted concurrency limit; a queued assignment is not a running worker.

## Iterative Rounds

When the main model's reconciliation exposes another task or a different grouping, create a new task/package ID or round-specific assignment and a new result directory. Do not silently reuse an old directory for a different scope. Keep a short round record at:

    openspec/changes/<change-name>/delegation/rounds/round-001.md

The round record links assigned tasks to their latest result files and states which spec documents the main model updated. It is a navigation record, not a second copy of the worker outputs.

For the detailed file shapes and minimal status fields, read [result-contract.md](references/result-contract.md). For model and reasoning allocation, read [model-allocation.md](references/model-allocation.md).
