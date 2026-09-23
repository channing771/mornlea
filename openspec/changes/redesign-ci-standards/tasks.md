# Layered Continuous Integration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the slow mixed pull-request workflow with a layered, platform-owned, fail-closed CI system and repair the three deterministic defects exposed by PR #184.

**Architecture:** Repository-owned scripts and Make targets define validation semantics; GitHub workflows only select pinned environments, transfer candidate-bound artifacts, and invoke those entry points. Linux and macOS native products have separate manifests, every required result converges on one `merge-gate`, and Godot remains a complete but optional workflow until a later cutover.

**Tech Stack:** GitHub Actions, Bash, GNU Make, Go 1.26, Rust 1.97.1/Cargo, Node.js 24/Corepack pnpm, Python 3.12/uv 0.12.5, Godot 4.7, OpenSpec 1.7.0.

**Spec:** `openspec/changes/redesign-ci-standards/design.md` and `openspec/changes/redesign-ci-standards/specs/continuous-integration/spec.md`

## Global Constraints

- Preserve protocol v45, player schema v9, chunk schema v9, world metadata v6, companions.ai v5, hostile_mobs v2, passive_mobs v1, engine ABI v11, client ABI v19, and benchmark scenario v23.
- Required runners are `ubuntu-24.04` x64 and `macos-15` arm64; the optional pinned-runtime job uses `macos-26` arm64 with Xcode 26.5 because its checked-in Python runtime inputs require Clang 21 and the macOS 26.5 SDK. `*-latest` labels are forbidden.
- External actions use the immutable SHAs listed in `task-briefs/07-required-workflow.md`; mutable major tags are forbidden.
- Every job has `contents: read` or narrower permissions and an explicit timeout; required commands never use `continue-on-error`, allow-failure, or automatic retries.
- The exact candidate SHA and normalized platform are manifest inputs; caches never satisfy artifact identity.
- The six committed Go modules remain the complete race universe, partitioned into pairwise-disjoint `client`, `server`, and `rest` slices.
- Godot remains outside the required merge graph; optional failures remain red and diagnosable.
- New or substantively rewritten source comments are English and do not contain task identifiers.
- No test may launch or focus a foreground game window; Godot runtime validation is headless.
- `tasks.md` is the sole task-status source. Workers report evidence; the controller alone updates checkboxes and `ledger.md`.
- At the start of each node, the implementer records `task_base="$(git rev-parse HEAD)"`, confirms the worktree contains no unassigned changes, and uses that immutable SHA for the node's changed-scope validation.

## Review Focus

- A manifest containing a duplicate, reordered, absolute, parent-traversing, symlink-escaping, missing, or digest-mismatched path must fail before any native-dependent command; Task 3.1 owns these cases.
- An absent executable, especially `rg`, must fail before partial validation and must never emit a success line; Tasks 2.1 and 4.1 own these cases.
- Unset, relative, and absolute `CARGO_TARGET_DIR` values invoked outside the repository root must resolve to the same producer/consumer output; Task 2.2 owns these cases.
- A missing, duplicated, or newly added workspace package must make the race-inventory check fail; Task 4.1 owns these cases.
- A failed, cancelled, or skipped required job, an omitted merge dependency, or accidental Godot dependency must make the relevant policy test fail; Tasks 5.1 and 5.2 own these cases.

---

## 1. Portable asset source set

- [x] 1.1 Make CPU atlas export platform-neutral, prove Linux source-set ownership, and retain real Linux compilation in the Linux quality gate (`task-briefs/01-portable-atlas.md`).

## 2. Godot script failure contracts

- [x] 2.1 Make project validation reject a missing `rg` before any scan or success output (`task-briefs/02-validator-prerequisites.md`).
- [x] 2.2 Make the Godot extension producer and consumer share the effective Cargo target root (`task-briefs/03-cargo-target-root.md`).

## 3. Native artifact trust

- [x] 3.1 Implement deterministic platform/SHA/path/size/digest manifests and fail-closed verification (`task-briefs/04-native-artifacts.md`).

## 4. Repository-owned CI entry points

- [x] 4.1 Implement dependency profiles, six-module race inventory, preflight, and race-slice entry points (`task-briefs/05-preflight-and-race.md`).
- [x] 4.2 Implement native build, Linux bundle, quality, and split integration entry points (`task-briefs/06-native-and-integration.md`).

## 5. Workflow topology

- [x] 5.1 Replace the required workflow with the pinned layered job graph and stable fail-closed `merge-gate` (`task-briefs/07-required-workflow.md`).
- [x] 5.2 Move complete Godot validation into a path-scoped optional workflow with no merge authority (`task-briefs/08-optional-godot-workflow.md`).

## 6. Documentation and acceptance

- [x] 6.1 Reconcile current CI documentation and the active Godot tooling plan with the implemented topology (`task-briefs/09-documentation-reconciliation.md`).
- [x] 6.2.1 Remove the cold-Linux native link from artifact-free preflight while retaining Linux quality compilation (`task-briefs/10-local-acceptance-review.md`).
- [x] 6.2.2 Fail closed on ripgrep execution errors in the Godot project validator (`task-briefs/10-local-acceptance-review.md`).
- [x] 6.2.3 Complete optional Godot path routing, client-core build, and exported-runtime qualification (`task-briefs/10-local-acceptance-review.md`).
- [x] 6.2.4 Re-review the repaired branch and run the complete local acceptance suite (`task-briefs/10-local-acceptance-review.md`).
- [x] 6.3.1 Defer the native-backed asset-sync audit from cold preflight while preserving full post-artifact coverage (`task-briefs/13-hosted-preflight-repair.md`).
- [x] 6.3.2 Select the exact Linux-compilable package set without dropping Darwin race coverage (`task-briefs/14-hosted-linux-source-set.md`).
- [x] 6.3.3 Provision and fail closed on `rg` for both Linux audit consumers (`task-briefs/15-hosted-audit-tools.md`).
- [x] 6.3.4 Discover the Godot extension on a cold project and provide its editor-selected debug library (`task-briefs/16-hosted-godot-extension.md`).
- [x] 6.3.5 Distinguish Linux native physics-step rejection causes on a candidate-bound runner before selecting a repair (`task-briefs/17-hosted-native-step-diagnostic.md`).
- [ ] 6.3.6 Match Go sweep-bound vector length to the Rust fixed-step FMA order without changing the native displacement policy (`task-briefs/18-hosted-physics-sweep-parity.md`).
- [ ] 6.3.7 Remove the temporary native rejection probe and restore the frozen source-provenance digest (`task-briefs/19-remove-physics-probe.md`).
- [ ] 6.3 Push the PR and qualify one exact candidate head in required and optional workflows (`task-briefs/11-pr-hosted-acceptance.md`).
- [ ] 6.4 Synchronize delta specs, archive the change, and qualify the final PR head (`task-briefs/12-archive-final-head.md`).
