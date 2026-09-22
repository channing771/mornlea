# Node 5.2: Optional Godot workflow extraction

## Identity and readiness

- Planning baseline: `d2416113`; execute from the controller-provided verified baseline.
- Direct predecessors: Nodes 1.1, 2.1, 2.2, and 5.1.
- Deliverable: relevant changes run complete Godot static and headless-runtime evidence in a separate red/green workflow that has no path into `Required CI / merge-gate`.
- Required sub-skills: `superpowers:test-driven-development` and `superpowers:verification-before-completion`.

## File ownership

- Create: `.github/workflows/godot.yml`.
- Modify: `packages/audit/godot_entrypoints_test.go`.
- Modify: `scripts/godot/rollback-check.sh`.
- Read-only authority: `.github/AGENTS.md`; `.github/workflows/ci.yml`; all existing `godot-*` Make targets; `scripts/godot/validate-project.sh`; `scripts/godot/build-extension.sh`.
- Excluded: adding Godot to branch protection or `merge-gate`, allowing failure, reducing the 100-cycle smoke, adding foreground/manual UI tests, or changing the target architecture.

## Workflow contract

Name the workflow `Godot CI`. It runs on `workflow_dispatch`, relevant pull-request paths, and relevant pushes to `main`. Use concurrency `godot-ci-${{ github.event.pull_request.number || github.ref }}` with cancellation and workflow-level `permissions: contents: read`.

The path list is explicit and includes:

```text
.github/workflows/godot.yml
Makefile
apps/mornlea-godot/**
packages/audit/godot_*_test.go
packages/audit/ci_workflow_standard_test.go
packages/client/assets/**
packages/client/cmd/mornlea-godot-assets/**
packages/client/cmd/mornlea-godot-core/**
packages/client/presentation/**
packages/client/runtime/**
packages/contracts/**
packages/engine/Cargo.lock
packages/engine/Cargo.toml
packages/engine/crates/mornlea_domain/**
packages/engine/crates/mornlea_godot/**
packages/engine/crates/mornlea_protocol/**
packages/engine/rust-toolchain.toml
packages/shared/**
scripts/ci/**
scripts/godot/**
```

The two jobs are:

- `godot-static`: `ubuntu-24.04`, timeout 20. It uses the pinned checkout, setup-go, and setup-uv actions from Node 5.1; setup-uv pins `0.12.5`. It runs `scripts/ci/doctor.sh godot-static`, `make godot-project-check`, `make godot-asset-check`, and `make godot-python-check` in that order.
- `godot-runtime`: needs `godot-static`, runs on `macos-15`, timeout 90. It uses pinned checkout and setup-uv, installs ripgrep explicitly with `brew install ripgrep`, runs `scripts/ci/doctor.sh godot-runtime`, then `scripts/godot/fetch.sh`, `scripts/godot/build-python-runtime.sh --verify`, and `make godot-smoke`. The smoke remains `--iterations 100 --isolated-python` through the Make target and stays headless.

Both jobs record duration and runner identity. Neither job uses `continue-on-error`, retries, a required-workflow artifact, or a `merge-gate` job.

## Test-first steps

1. Refactor `TestGodotIsOptionalForLegacyBuild` to read both workflow files. Keep the Make legacy-edge checks. Required workflow violations assert no Godot job, command, script path, need, or path-filter indirection. Optional workflow violations assert the exact name/triggers/path set/jobs/runners/timeouts/actions/order and the three static plus one smoke Make gates.

2. Replace old mutations that assumed `godot` sat directly before `test`. Add mutations that delete a required path, move smoke into required CI, add `continue-on-error`, remove the 100-cycle isolated smoke through the Makefile fixture, remove manual dispatch, change a runner to `latest`, or make an optional job depend on/produce `merge-gate`.

3. Update `rollback-check.sh` to read both workflows. It must reject any non-comment Godot reference in required job blocks, require the optional workflow file, and keep all existing legacy Make, authority, project-root, and offline-replay checks. Change obsolete `required CI test job` wording to `required CI merge gate`.

4. Run the red suite before adding the workflow:

   ```bash
   go test ./packages/audit -run '^TestGodotIsOptionalForLegacyBuild$' -count=1
   scripts/godot/rollback-check.sh
   ```

   Expected: fail because the optional workflow is missing.

5. Add `godot.yml` with the exact contract. Keep setup/fetch logic in workflow YAML but all validation semantics in repository commands.

6. Re-run the tests plus the repaired failure entry points:

   ```bash
   go test ./packages/audit -run 'TestGodot(IsOptionalForLegacyBuild|ProjectValidator|BuildExtension|AssetSync)' -count=1
   scripts/godot/rollback-check.sh
   scripts/godot/validate-project.sh
   scripts/godot/sync-assets.sh --check
   ```

   `make godot-smoke` is not required locally when it would download or run the full Godot runtime; the exact optional workflow run on the PR is its acceptance environment.

7. Restore the paired transition gates and prove the required/optional split is green together:

   ```bash
   go test ./packages/audit -count=1
   make test-race-changed RACE_BASE="<Node-5.1-task-base>"
   ```

   Use the immutable base recorded by Node 5.1 so changed-scope validation covers both workflow commits and all migrated audit/rollback consumers.

## Closure

- Re-enumerate every Godot/GDExtension/Python/asset consumer with `rg -n 'godot-|scripts/godot|mornlea_godot|mornlea-godot' .github Makefile scripts packages apps docs`; confirm every source input is either in the path list or has a documented non-effect.
- The paired changed-scope command above replaces a Node-5.2-only base; it must cover both Nodes 5.1 and 5.2.
- Commit only owned files with `chore(godot): isolate optional migration validation`.
- Rollback unit: this commit plus Node 5.1. Recombining Godot into required CI violates the approved design; rollback means restoring the prior pair only as a coordinated emergency revert.
- Report path inventory, mutation evidence, repaired-script evidence, and commit SHA. The controller updates `tasks.md` and `ledger.md`.
