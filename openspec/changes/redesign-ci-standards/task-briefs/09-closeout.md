# Node 6.1: Documentation, review, exact-head evidence, and archive

## Identity and readiness

- Planning baseline: `d2416113`; execute from the controller-provided verified baseline.
- Direct predecessors: every node from 1.1 through 5.2.
- Deliverable: repository guidance matches the implemented CI contract, all local gates and independent review pass, the exact PR head is green in both workflows, and the completed OpenSpec change is archived and revalidated on the final PR head.
- Required sub-skills: `superpowers:requesting-code-review`, `superpowers:verification-before-completion`, and `superpowers:finishing-a-development-branch` when preparing the PR handoff.

## File ownership

- Modify: `docs/notes/test-quickstart.md`.
- Modify: `docs/development-process.md`.
- Modify: `docs/notes/progress.md`.
- Read-only authority: `README.md` CI badge; `docs/AGENTS.md`; `openspec/config.yaml`; all implemented workflows, Make targets, scripts, tests, and active change artifacts.
- Controller-only status/evidence writes: `openspec/changes/redesign-ci-standards/tasks.md` and `ledger.md` before archive.
- Archive output after exact-head evidence: `openspec/specs/continuous-integration/spec.md`, updated `openspec/specs/test-timing-discipline/spec.md`, and the dated `redesign-ci-standards` directory path returned by the OpenSpec archive command.
- Excluded: changing test semantics to obtain green, editing benchmark thresholds, branch-protection mutation, direct merge, force-push, or bypass variables.

## Documentation contract

`test-quickstart.md` must describe:

- `make ci-preflight` as the local early-policy reproduction command;
- the platform routing of `race-client`, `race-server`, and `race-rest`, including `gfxspike` in the macOS client slice;
- the separate Linux and macOS artifact manifests and same-SHA verification;
- `Required CI / merge-gate` as the only intended protected result;
- `Godot CI` as optional, path-scoped, manually dispatchable, and still fail-closed;
- the five-minute preflight and twenty-minute critical-path service targets as informational measurements.

`development-process.md` must replace old job names/topology with the stable merge gate and state that a failed required job is repaired and rerun without silent automatic retry. `progress.md` records only verified implementation and CI evidence, not planned success. The existing README badge remains valid because the required workflow file stays `.github/workflows/ci.yml`.

## Review and validation steps

1. Update the three documentation files from the implemented commands, not from the planning brief. Run `rg -n 'native-macos|linux-server|go-race|final test job|macos-latest|ubuntu-latest' docs README.md openspec/specs` and classify every match as current, historical, or superseded by the active delta. Do not rewrite archived changes.

2. Run focused policy and regression gates:

   ```bash
   make ci-preflight
   go test ./packages/audit -count=1
   scripts/godot/validate-project.sh
   scripts/godot/sync-assets.sh --check
   scripts/godot/rollback-check.sh
   bash -n scripts/ci/*.sh scripts/godot/build-extension.sh scripts/godot/validate-project.sh
   openspec validate redesign-ci-standards --strict --no-interactive
   ```

3. Ask a fresh reviewer to compare the full branch against `proposal.md`, both delta specs, `design.md`, `tasks.md`, and every task brief. The review must enumerate producer/consumer paths again and report findings by severity with file/line evidence. The controller rules on each finding; an accepted finding is fixed in the owning node and revalidated before proceeding.

4. Run the complete stage-boundary suite from a clean status except the planned documentation/OpenSpec files:

   ```bash
   make rust-check
   make frontend-check
   make companion-agent-check
   make companion-agent-integration
   make dev-check
   make test-race-short
   make test-race
   openspec validate --all --strict --no-interactive
   git diff --check origin/main...HEAD
   ```

   `make test-race` covers all six modules and is mandatory. If a command fails, keep Node 6.1 open and fix the root cause; no failure may be labelled inherited without reproducing and recording it.

5. Commit documentation with `docs(ci): document layered validation workflow`. The controller then marks completed tasks and appends exact commands/results, review rulings, orchestration decisions, and the round-end architecture-skill decision to `ledger.md`, validates the change again, and commits the planning evidence with `docs(ci): record validation redesign evidence`.

6. With the user's standing request to submit the repair PR still applicable, push `codex/redesign-ci-standards` and create or update a PR titled `refactor(ci): layer platform-owned validation`. Use `.github/PULL_REQUEST_TEMPLATE.md`; list actual validation results one command per line and state that protocol/save/ABI/scenario/visual versions are unchanged. Do not merge.

7. Watch the exact head until `Required CI / merge-gate` is green. Also require the relevant `Godot CI / godot-static` and `Godot CI / godot-runtime` run to be green as implementation acceptance even though neither is merge authority. Record job durations, runner identities, run URLs, and the exact SHA. Repair any failure with a scoped commit and restart exact-head validation; do not retry commands silently.

8. After that exact head is green, archive with the repository OpenSpec archive workflow and synchronize the delta specs. Run:

   ```bash
   openspec validate --all --strict --no-interactive
   git diff --check
   ```

   Commit the archive/synchronized specs with `docs(ci): archive layered validation standard`, push the updated head, and watch both workflows again. Final acceptance requires the archived exact head to be green.

9. Report branch protection as external state. The intended required check is exactly `Required CI / merge-gate`; do not change repository settings without a new explicit authorization. Leave the green PR open unless the user explicitly asks to merge.

## Closure

- The controller confirms every `tasks.md` checkbox is backed by a ledger evidence row before archive and that the archive preserved those records.
- Re-run `git status --short`, `git log --oneline origin/main..HEAD`, and PR check status immediately before any completion claim.
- Rollback unit: workflow/entry-point commits may be reverted together; portable atlas, fail-closed validator, and target-root fixes remain unless separately disproved. Never restore the known fail-open validator or stale artifact lookup as rollback.
- Report local commands, independent review result, both exact-head workflow outcomes, final PR URL, archive commit, and any explicitly external branch-protection step.
