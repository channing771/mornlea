---
name: spec-delegated-orchestration
description: Define model assignments for spec-driven task breakdowns and a durable per-task result store that the later main model can read. Use when one main model owns specs while separately launched models produce task results.
---

# Spec Task Model Allocation

This skill defines only two things: which available model and reasoning band belongs to each decomposed task, and where that task's intermediate and final results are stored. It does not prescribe how a worker is launched, what tools it uses, how it edits code, how it waits, or how it validates its work. Those decisions remain with the user and the worker runtime.

## Main Model Contract

The main model is responsible for the control plane:

1. Read the applicable repository guidance and current OpenSpec artifacts.
2. Decompose the request into independently addressable tasks with stable IDs.
3. Write or update the relevant spec documents and tasks.md before launch, then assign exactly one non-controller candidate model and one reasoning band to each worker task in the change documents.
4. Create the result directory before the task starts.
5. After the user launches the workers and they finish, read the result index, each task's final result, and any needed checkpoints.
6. Reconcile the verified conclusions into proposal.md, delta specs, design.md, tasks.md, and the change ledger as appropriate.

The main model remains the owner of spec meaning and cross-task conclusions. A worker result is an input to that process, not an automatic change to a spec.

## Assignment Record

For an OpenSpec change, keep the assignment index at:

    openspec/changes/<change-name>/delegation/assignments.yaml

Each task in tasks.md must have a matching assignment entry. The entry records:

    change: example-change
    round: 1
    tasks:
      - id: task-001
        title: bounded task description
        model: GLM 5.3
        reasoning: medium
        result_dir: delegation/results/task-001/
        depends_on: []

Use the exact candidate labels supplied by the user: GLM 5.3, GLM 5.3 Flash, Muse spark, a separately launched ChatGPT worker model, or Grok 4.6. The current main model is never assigned as its own worker. If the execution host maps a label to a different provider ID, add resolved_model beside model; do not replace the user-facing assignment.

The assignment index is planning data, not a worker instruction sheet. Keep task scope, acceptance intent, and dependencies in tasks.md; keep model allocation and result locations in assignments.yaml so the later main model has one predictable index to read. Controller-owned spec writing and final integration are recorded as controller work, not as a self-assigned worker task.

## Result Locations

Every assignment gets its own directory:

    openspec/changes/<change-name>/delegation/results/<task-id>/
      status.yaml
      progress/
        001.md
        002.md
      final.md

The worker or user-controlled runtime may choose the contents of each result, but these locations are fixed:

- status.yaml identifies the task, assigned model, reasoning band, current state, and the path of the latest final output.
- progress/NNN.md stores numbered stage outputs or checkpoints. Files are append-only snapshots; do not overwrite an earlier stage.
- final.md stores the worker's final output for the current attempt, including whether the task is complete, partial, blocked, or failed.

If a task is retried or its assignment changes, preserve the previous output as final-r01.md, final-r02.md, and so on, and point status.yaml at the newest file. Do not erase an earlier result. A task that produces no useful result still gets a status.yaml and a final.md stating why.

## Main Model Read Order

When the main model is started for integration, it reads results in this order:

1. delegation/assignments.yaml to discover task IDs, assigned models, reasoning bands, dependencies, and result directories.
2. Each task's status.yaml to identify the latest state and final-output path.
3. The latest final*.md named by status.yaml.
4. The numbered progress/ files only when the final output is incomplete, contradictory, or needs historical context.
5. The relevant OpenSpec artifacts and repository evidence before updating any spec.

This read order lets the main model resume from a known index without searching the repository for worker output. Results must not be stored only in chat, an untracked temporary directory, or a model-specific private location.

## Iterative Rounds

When the main model's reconciliation exposes another task, create a new task ID or round-specific assignment and a new result directory. Do not silently reuse an old directory for a different scope. Keep a short round record at:

    openspec/changes/<change-name>/delegation/rounds/round-001.md

The round record links assigned tasks to their latest result files and states which spec documents the main model updated. It is a navigation record, not a second copy of the worker outputs.

For the detailed file shapes and minimal status fields, read [result-contract.md](references/result-contract.md). For model and reasoning allocation, read [model-allocation.md](references/model-allocation.md).
