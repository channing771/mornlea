# Node 6.1: Documentation and active-plan reconciliation

## Identity and readiness

- Direct predecessors: every implementation node from 1.1 through 5.2.
- Deliverable: current developer guidance and the still-active Godot tooling plan describe the implemented CI topology and no longer route optional work through the required workflow.
- Required sub-skills: `superpowers:executing-plans` and `superpowers:verification-before-completion`.

## File ownership

- Create: `docs/continuous-integration.md` and synchronized `docs/continuous-integration.zh.md` as the current CI authority.
- Modify together: `docs/development-process.md` and `docs/development-process.zh.md`.
- Modify together: `docs/README.md` and `docs/README.zh.md` to add the navigation entry.
- Modify: `docs/documentation-manifest.json` to register the new bilingual pair and matching revision bumps.
- Modify: `openspec/changes/godot-production-tooling/tasks.md` only to replace the superseded optional-workflow location and job ownership.
- Read-only authority: root `README.md` CI badge; `docs/AGENTS.md`; legacy `docs/notes/test-quickstart.md` and `docs/notes/progress.md`; `.github/workflows/ci.yml`; `.github/workflows/godot.yml`; implemented Make targets and scripts; both active OpenSpec changes.
- Excluded: changing validation behavior, rewriting archived changes, claiming hosted evidence that has not run, changing branch protection, or starting a foreground game window.

## Documentation contract

`continuous-integration.md` and its synchronized Chinese counterpart describe:

- `make ci-preflight` as the local early-policy reproduction command;
- platform routing for `race-client`, `race-server`, and `race-rest`, including `gfxspike` in the macOS client slice;
- separate Linux/macOS artifact manifests and same-SHA verification;
- `Required CI / merge-gate` as the only intended protected result;
- `Godot CI` as path-scoped, manually dispatchable, outside merge authority, and still fail-closed;
- the optional static/runtime split, including macOS 26/Xcode 26.5, cold six-module Go prefetch, native-before-cgo assets, and the unchanged 100-cycle headless smoke;
- five-minute preflight and twenty-minute required critical-path targets as informational measurements.

The development-process pair replaces old job names/topology with a concise link to the CI authority and states that a failed required job is repaired and rerun explicitly, never hidden by automatic validation retry. The documentation-map pair links the new authority instead of duplicating it. Legacy `test-quickstart.md` and `progress.md` remain unchanged: their first substantive revision requires a separate atomic English/Chinese migration, and implementation history is already preserved in this change's ledger/archive. The root README badge remains unchanged because the required workflow file remains `.github/workflows/ci.yml`.

The active `godot-production-tooling` task 3.4 must name `.github/workflows/godot.yml`, not `ci.yml`, for future optional tooling jobs. No other scope, acceptance, or status in that change moves.

## Validation and closure

1. Read implemented workflows/entry points before editing. Run:

   ```bash
   rg -n 'native-macos|linux-server|go-race|final test job|macos-latest|ubuntu-latest|workflows/ci.yml' docs README.md openspec/specs openspec/changes/godot-production-tooling
   ```

   Classify every remaining match as current, historical, or superseded; do not rewrite archived evidence.

2. Update only the owned files. Run:

   ```bash
   git diff --check
   go test ./packages/audit -run '^TestDocumentation(Manifest|Pair)' -count=1
   openspec validate redesign-ci-standards --strict --no-interactive
   openspec validate godot-production-tooling --strict --no-interactive
   make ci-preflight
   ```

3. Commit only owned files with `docs(ci): document layered validation workflow`. Report the match classification, bilingual revision identities, commands, and commit SHA. The controller updates `tasks.md` and `ledger.md` separately.

4. Rollback unit: the documentation commit. Reverting it must not affect workflow behavior.
