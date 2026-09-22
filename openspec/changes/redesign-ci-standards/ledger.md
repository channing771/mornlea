# Change ledger

This append-only ledger records controller decisions, task routing, evidence, and review rulings. `tasks.md` is the only task-status source.

## Planning

| Date | Node | Record |
| --- | --- | --- |
| 2026-09-22 | design | Ruling: use a layered required workflow plus a separate optional Godot workflow — PR #184 showed that platform-neutral failures were delayed behind a 13-minute macOS build and that optional migration evidence was mixed with merge authority. |
| 2026-09-22 | design | Evidence: Linux compilation failed on Darwin-only `Registry.AtlasPixels`; Godot validation lacked a fail-closed `rg` prerequisite; the extension builder inherited `CARGO_TARGET_DIR` but searched `packages/engine/target`. |
| 2026-09-22 | planning | Execution shape: sequential subagent-driven nodes with fresh bounded implementers and proportional independent review — tasks share one worktree and several serial integration files, so concurrent editors or concurrent Git commits would create unnecessary ownership risk. |
| 2026-09-22 | planning | Isolation: Nodes 1.1, 2.1, 2.2, and 3.1 have disjoint editable files but will still integrate one verified commit at a time; Nodes 4.1 through 6.1 consume those frozen interfaces in dependency order. |
| 2026-09-22 | planning | External state: workflow and repository documentation may declare `Required CI / merge-gate`; changing GitHub branch protection remains outside implementation authority until explicitly approved. |
| 2026-09-22 | planning | Architecture skill: no change — the plan applies the existing optional-Godot and platform-ownership rules but has not yet produced a new stable cross-task rule backed by implementation evidence. |
| 2026-09-22 | planning | Discovery: workflow/script consumers, six-module package enumeration, three failing seams, generated atlas provenance, and Godot rollback checks were enumerated before task ownership was frozen. The atlas source node owns its generated manifest refresh; the optional-workflow node owns rollback-check migration. |
| 2026-09-22 | planning | Readiness review: passed — nine nodes have independently reviewable deliverables, exact editable/read-only files, frozen interfaces, concrete baseline failures, red/green commands, exclusions, scoped commits, rollback units, and acyclic predecessors. All continuous-integration requirements and five Review Focus cases trace to named nodes; task briefs contain no competing status checkboxes. |
