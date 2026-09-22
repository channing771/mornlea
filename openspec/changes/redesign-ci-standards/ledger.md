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

## Execution preflight

| Check | Producer / consumer contract | Finding |
| --- | --- | --- |
| Node 1.1 | Linux compile regression, platform-neutral `AtlasPixels`, then committed-source provenance refresh | Internally consistent; source and derived manifest are deliberately separate ordered commits. |
| Node 2.1 | Missing-`rg` audit regression and validator prerequisite guard | Internally consistent; the guard precedes repository discovery and all validator modes. |
| Node 2.2 | Fake Cargo producer and `build-extension.sh` consumer for unset, relative, absolute, and traversal cases | Internally consistent; repository-relative resolution is frozen by the brief. |
| Node 3.1 | Manifest packager, verifier, platform identity, mutation suite, and `scripts/ci` ownership guide | Internally consistent; legacy workflow callers remain intentionally broken only until their atomic Node 5.1 migration. |
| Node 4.1 | Dependency profiles, package inventory, partition proof, race launcher, and `ci-preflight` | Internally consistent; package counts remain observations rather than acceptance constants. |
| Node 4.2 | Native packaging, artifact verification, Linux quality, race, and split integration entry points | Internally consistent; host-only limits are explicit and workflow acceptance owns the other platform. |
| Node 5.1 | Audit mutations and the exact pinned required workflow graph | Internally consistent; the workflow consumes only repository-owned `ci-*` semantics and removes the obsolete `test` aggregator. |
| Node 5.2 | Required/optional workflow audit, rollback policy, and path-scoped Godot workflow | Internally consistent; optional status never suppresses command failures or enters `merge-gate`. |
| Node 6.1 | Documentation, full validation, review, exact-head evidence, synchronization, archive, and PR handoff | Internally consistent; archive happens only after exact-head workflow evidence and is revalidated on the archived head. |
| Nodes 1.1 -> 4.2 | Platform-neutral atlas API is compiled by Linux quality and asset generation | Matching signature and unchanged byte contract; no GPU ownership moves. |
| Nodes 1.1 -> 5.2 | Committed source provenance is consumed by optional Godot asset validation | Matching generated-manifest ownership; Node 1.1 owns the refresh. |
| Nodes 2.1 -> 4.1 | Validator fail-closed dependency semantics are mirrored by the CI doctor | Matching missing-tool policy; validator and environment checks remain independently defensive. |
| Nodes 2.1/2.2 -> 5.2 | Repaired validator and Cargo target contracts are consumed by optional Godot jobs | Matching command interfaces; no workflow-only fallback is permitted. |
| Nodes 3.1 -> 4.2 | Platform/SHA/path/size/digest manifest API is consumed by native Make targets | Matching named options, manifest paths, and expected platform file sets. |
| Nodes 3.1/4.2 -> 5.1 | Native producers and consumers are transferred through same-SHA workflow artifacts | Matching artifact roots and manifest names; no downstream rebuild fallback. |
| Nodes 4.1 -> 4.2 | Doctor, package inventory, and race launcher are composed by remaining CI targets | Matching profile and slice names; dependencies are acyclic. |
| Nodes 4.1/4.2 -> 5.1 | Repository-owned entry points replace inline workflow validation semantics | Matching job commands and platform ownership; workflow YAML retains setup and transport only. |
| Nodes 5.1 -> 5.2 | `.github/AGENTS.md` and required graph constrain optional workflow extraction | Matching ownership guide; Godot remains outside required job blocks and merge dependencies. |
| Nodes 5.1/5.2 -> 6.1 | Implemented topology and command names feed documentation and exact-head acceptance | Matching stable statuses `Required CI / merge-gate` and `Godot CI`; branch protection remains external. |

| Date | Node | Record |
| --- | --- | --- |
| 2026-09-22 | execution | Orphan check: no completed checkbox without evidence, no in-flight ledger row, no untracked or modified file, and no stale implementation artifact existed in the isolated worktree at `3401fa7a`. |
| 2026-09-22 | execution | Ruling: keep `tasks.md` and this ledger as the only durable execution state, using existing OpenSpec task briefs and ephemeral reports/diff packages outside the repository — the project orchestration skill explicitly forbids a `.superpowers/sdd` progress store; if wrong, recovery loses disposable reviewer packets but not task status, commits, or durable evidence. |
| 2026-09-22 | execution | Ruling: resolve each node's `task_base` to an immutable SHA and record/pass that exact value rather than relying on a worker shell variable surviving separate commands — this preserves changed-scope validation semantics; if wrong, a worker may rerun a broader gate but cannot omit committed changes. |
| 2026-09-22 | execution | Baseline: isolated branch `codex/redesign-ci-standards-impl` started from `3401fa7a`; `make rust`, the seven existing focused CI/Godot audit tests, and strict change validation passed before implementation. |
| 2026-09-22 | 1.1 | NEEDS_CONTEXT evidence: the prescribed `CGO_ENABLED=0` red test failed first because `packages/shared/nativeabi` correctly has no non-cgo implementation, so it could not observe the Darwin-only atlas defect. |
| 2026-09-22 | 1.1 | Ruling: replace the impossible cgo-disabled cross-compile with a `go list -json` Linux source-set assertion on every host, a real compile branch on Linux with cgo enabled, and required Linux quality compilation in Node 4.2 — this isolates the intended build-tag regression without weakening the existing ABI boundary; if wrong, non-Linux local runs could miss a compiler-only Linux defect until the required Linux job. |
| 2026-09-22 | 1.1 | NEEDS_CONTEXT evidence: after both planned product commits, all focused gates passed but `make test-race-changed RACE_BASE=9970aa52` correctly required the English-comment migration baseline to ratchet from seven to two comments because the brief mandated rewriting the stale atlas comment. |
| 2026-09-22 | 1.1 | Ruling: add the existing English-comment migration baseline to Node 1.1 ownership and permit a dedicated third test commit instead of rewriting the two verified product commits — the ratchet is a deterministic consequence of the required comment change; if wrong, the audit baseline could bless an unrelated debt change, so the worker must use update mode and inspect the diff before committing. |
| 2026-09-22 | 1.1 | Evidence: `TestGodotAssetGeneratorUsesPortableLinuxSourceSet` failed with `atlas.go` in `IgnoredGoFiles`, then the focused atlas/source-set tests, deterministic asset check, comment-migration audit, and `make test-race-changed RACE_BASE=9970aa52` passed. `atlas.rgba8` remained `f0de32dcbc3023fb66a06f0a73435eaa578422bcfbb1902c3da7ddece4075a43`. |
| 2026-09-22 | 1.1 | Review: spec PASS and task quality Approved with no Critical, Important, or Minor findings. The reviewer could not execute the Linux-only compiler branch from Darwin; this is not a present gap because the revised task contract makes Linux source selection the portable proof and Node 4.2 owns required real Linux compilation. |
| 2026-09-22 | 1.1 | Complete: implementation commits `2459ccff`, `600a558e`, and `e4ef702a`; independent review clean. |
| 2026-09-22 | 1.1 | Architecture skill: no change — the cgo-aware source-set testing correction is specific to this task and does not establish a new cross-task architecture rule beyond existing platform ownership. |
| 2026-09-22 | 2.1 | Routing correction: the controller initially transcribed an invalid full `task_base` after the clean orphan check; the worker stopped without edits, received the actual `efa7454b3808b9db6ea8f7567c0dcce8674ff64b`, and then resumed normally. |
| 2026-09-22 | 2.1 | Evidence: the missing-`rg` regression failed on the old fail-open behavior, then the prerequisite test, existing Godot validator coverage, full audit package, direct validator, shell syntax, and `make test-race-changed RACE_BASE=efa7454b3808b9db6ea8f7567c0dcce8674ff64b` passed. |
| 2026-09-22 | 2.1 | Minor (deferred): the regression test combines stdout/stderr and does not separately assert exact stderr-only output or exit code 1; implementation inspection confirms both and the test matches the frozen brief. Final review must triage whether this needs strengthening before merge. |
| 2026-09-22 | 2.1 | Complete: commit `74fdd290`; independent review found spec compliant and task quality Approved with no Critical or Important findings. |
| 2026-09-22 | 2.1 | Architecture skill: no change — fail-closed executable prerequisites are already an approved CI rule, and this node adds no broader ownership decision. |
