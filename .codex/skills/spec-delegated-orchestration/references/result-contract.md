# Result Storage Contract

This contract defines where a user-controlled worker runtime places outputs. It deliberately does not define how the worker performs the assigned task.

## Per-Change Layout

    openspec/changes/example-change/
      tasks.md
      delegation/
        assignments.yaml
        rounds/
          round-001.md
        results/
          task-001/
            status.yaml
            progress/
              001.md
              002.md
            final.md

Use the real OpenSpec change name and task ID. The result store travels with the change so it remains available when the change is reviewed, resumed, or archived.

## Assignment Index Fields

assignments.yaml is written by the main model before workers are launched. It contains one entry per task:

    change: example-change
    round: 1
    tasks:
      - id: task-001
        title: bounded task description
        model: GLM 5.3 Flash
        reasoning: low
        result_dir: delegation/results/task-001/
        depends_on: []

Required fields are id, title, model, reasoning, and result_dir. model identifies a worker other than the current controller; the controller's own spec and integration work is not a worker assignment. depends_on is required when ordering matters and may be an empty list. Optional resolved_model and resolved_reasoning fields record host-specific values without changing the user-facing assignment.

## Status File

status.yaml is the small navigation record the main model reads first:

    task_id: task-001
    assigned_model: GLM 5.3 Flash
    assigned_reasoning: low
    state: complete
    latest_final: final.md
    updated_at: 2026-09-19T10:30:00+08:00

Allowed states are planned, running, complete, partial, blocked, and failed. A runtime may add non-semantic metadata, but it must not remove the task identity, assignment, state, or latest-final pointer. Do not put credentials, private account responses, or secrets in this file.

## Progress Files

Each meaningful stage can produce the next numbered Markdown file under progress/. A stage file should identify the task and state what was produced at that point. It may contain code excerpts, decisions, discovered constraints, or links to generated artifacts. Stage files are immutable history: a correction creates the next number rather than editing the old file.

The main model normally does not need to read every progress file. It reads them when final.md is absent, marked partial/blocked/failed, or leaves an important contradiction unresolved.

## Final File

final.md is the required terminal handoff. It should contain, in any clear format:

- task ID and assigned model;
- terminal state;
- concise final output or conclusion;
- paths to any implementation or generated artifacts;
- unresolved questions or known limitations.

The runtime controls the wording and technical details. The storage rule only requires that the final output be present at the indexed path and remain readable by the later main model.

For a retry, preserve the old final file and add a revision suffix:

    final-r01.md
    final-r02.md

Update latest_final to the newest file. This preserves the evolution of the result without requiring the main model to guess which output is current.

## Round Record

rounds/round-001.md is maintained by the main model after reading results. It should list each task ID, assigned model, latest final path, terminal state, and the OpenSpec documents changed during reconciliation. It may link to files; it should not duplicate their contents.
