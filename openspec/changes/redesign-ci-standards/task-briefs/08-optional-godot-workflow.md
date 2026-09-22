# Node 5.2: Optional Godot workflow extraction

## Identity and readiness

- Planning baseline: `d2416113`; execute from the controller-provided verified baseline.
- Direct predecessors: Nodes 1.1, 2.1, 2.2, and 5.1.
- Deliverable: relevant changes run complete Godot static and headless-runtime evidence in a separate red/green workflow that has no path into `Required CI / merge-gate`.
- Required sub-skills: `superpowers:test-driven-development` and `superpowers:verification-before-completion`.

## File ownership

- Create: `.github/workflows/godot.yml`.
- Modify: `packages/audit/godot_entrypoints_test.go`.
- Modify: `packages/audit/ci_local_contracts_test.go` for the corrected Godot dependency profiles.
- Modify: `packages/audit/godot_toolchain_test.go` for cold-cache editor materialization fixtures.
- Modify: `scripts/ci/doctor.sh` for the corrected static/runtime executable sets.
- Modify: `scripts/godot/fetch.sh` so a verified archive becomes the cache-owned editor consumed by headless checks.
- Modify: `scripts/godot/rollback-check.sh`.
- Read-only authority: `.github/AGENTS.md`; `.github/workflows/ci.yml`; all existing `godot-*` Make targets; `scripts/godot/validate-project.sh`; `scripts/godot/build-extension.sh`; `scripts/godot/smoke.sh`; `scripts/godot/python-version.env`; `scripts/godot/py4godot/build-inputs.env`; `go.work` and all six module manifests.
- Excluded: adding Godot to branch protection or `merge-gate`, allowing failure, reducing the 100-cycle smoke, adding foreground/manual UI tests, or changing the target architecture.

## Workflow contract

Name the workflow `Godot CI`. It runs on `workflow_dispatch`, relevant pull-request paths, and relevant pushes to `main`. Use concurrency `godot-ci-${{ github.event.pull_request.number || github.ref }}` with cancellation and workflow-level `permissions: contents: read`.

The path list is explicit and includes:

```text
.github/workflows/godot.yml
Makefile
apps/mornlea-godot/**
go.work
go.work.sum
packages/audit/godot_*_test.go
packages/audit/ci_workflow_standard_test.go
packages/audit/go.mod
packages/audit/go.sum
packages/client/assets/**
packages/client/cmd/mornlea-godot-assets/**
packages/client/cmd/mornlea-godot-core/**
packages/client/go.mod
packages/client/go.sum
packages/client/mesh/**
packages/client/presentation/**
packages/client/render/assets/**
packages/client/runtime/**
packages/contracts/**
packages/engine/**
packages/server/go.mod
packages/server/go.sum
packages/shared/**
packages/tools/go.mod
packages/tools/go.sum
scripts/ci/**
scripts/engine/**
scripts/godot/**
```

The two jobs are:

- `godot-static`: `ubuntu-24.04`, timeout 20. It uses only the pinned checkout action from Node 5.1. It explicitly installs ripgrep with `sudo apt-get update` and `sudo apt-get install --yes ripgrep` before running `scripts/ci/doctor.sh godot-static` and `make godot-project-check` in that order. Dependency installation is workflow setup, not validation semantics.
- `godot-runtime`: needs `godot-static`, runs on the arm64 `macos-26` label, timeout 90, and sets only `DEVELOPER_DIR=/Applications/Xcode_26.5.app/Contents/Developer` at job scope. It uses pinned checkout, setup-go, and setup-uv; Go is `1.26`, uv is `0.12.5`, and setup-go's cache key covers `go.work.sum`, every committed module `go.sum`, and both native ABI headers. After `brew install ripgrep`, it activates the repository-pinned Rust toolchain by changing to `packages/engine` and running `rustup show active-toolchain`, `rustc --version`, and `cargo --version`, then runs `scripts/ci/doctor.sh godot-runtime`. It downloads external dependencies for each of the six committed Go modules with `GOWORK=off go mod download`, runs `make rust`, and then runs `make godot-asset-check`. It continues with `scripts/godot/fetch.sh`, `scripts/godot/build-python-runtime.sh --verify`, `make godot-python-check`, and `make godot-smoke` in that order. Asset validation is runtime-owned because its Go closure reaches `packages/shared/nativeabi`; Python validation is runtime-owned because it deliberately uses the materialized Darwin embedded Python with locked/offline/no-download semantics. The smoke remains `--iterations 100 --isolated-python` through the Make target and stays headless.

The six-module prefetch command is workflow setup and MUST be exactly the committed module set from `go.work`; using `GOWORK=off` prevents setup from rewriting `go.work.sum`. The subsequent asset gate retains `GOPROXY=off`, so a missing prefetch remains a hard cold-cache failure rather than a network fallback. `make rust` materializes the canonical `packages/engine/target/release` ABI consumed by the cgo link. `scripts/godot/fetch.sh` verifies the pinned archive, extracts it into a temporary directory under the external cache, requires an executable `Godot.app/Contents/MacOS/Godot`, and only then replaces the cache-owned `Godot.app`; `--verify-only` continues to verify local bytes or the pinned remote metadata without extraction.

Both jobs record duration and runner identity. Neither job uses `continue-on-error`, retries, a required-workflow artifact, or a `merge-gate` job.

## Test-first steps

1. Refactor `TestGodotIsOptionalForLegacyBuild` to read both workflow files. Keep the Make legacy-edge checks. Required workflow violations assert no Godot job, command, script path, need, or path-filter indirection. Optional workflow violations assert the exact name/triggers/path set/jobs/runners/timeouts/actions/order, the static project gate, repository-pinned Rust activation, the runtime-owned prefetch/native/asset/Python/smoke sequence, setup-go cache inputs, and the exact Xcode selection.

2. Replace old mutations that assumed `godot` sat directly before `test`. Add mutations that delete a required path, move smoke into required CI, add `continue-on-error`, remove the 100-cycle isolated smoke through the Makefile fixture, remove manual dispatch, change a runner to `latest`, change or remove `DEVELOPER_DIR`, remove the Rust activation step, omit one module from prefetch, omit `make rust` or the runtime asset gate, weaken setup-go cache inputs, or make an optional job depend on/produce `merge-gate`.

3. Update `rollback-check.sh` to read both workflows. It must reject any non-comment Godot reference in required job blocks, require the optional workflow file, and keep all existing legacy Make, authority, project-root, and offline-replay checks. Change obsolete `required CI test job` wording to `required CI merge gate`.

4. Run the red suite before adding the workflow:

   ```bash
   go test ./packages/audit -run '^TestGodotIsOptionalForLegacyBuild$' -count=1
   scripts/godot/rollback-check.sh
   ```

   Expected: fail because the optional workflow is missing.

5. Add `godot.yml` with the exact contract. Keep environment selection, dependency prefetch, and dependency installation in workflow YAML but all validation semantics in repository commands.

6. Add a failing fetch fixture proving that a verified downloaded archive without an executable materialized editor is rejected and not published. Implement transactional editor materialization in `fetch.sh`, then prove the fixture publishes the expected cache path and that `--verify-only` does not extract. Inject a failure when the staged editor is moved into its public path after the previous editor has been retained; require nonzero exit and successful restoration. Inject a second failure during restoration; require nonzero exit, an exact recovery-location diagnostic, and preservation of the previous editor inside the staging directory for manual recovery.

7. Re-run the tests plus the repaired failure entry points:

   ```bash
   go test ./packages/audit -run 'TestGodot(IsOptionalForLegacyBuild|ProjectValidator|BuildExtension|AssetSync)' -count=1
   scripts/godot/rollback-check.sh
   scripts/godot/validate-project.sh
   scripts/godot/sync-assets.sh --check
   ```

   `make godot-smoke` is not required locally when it would download or run the full Godot runtime; the exact optional workflow run on the PR is its acceptance environment.

8. Prove cold-cache module setup without modifying checksums: use a fresh `GOMODCACHE`, run `GOWORK=off go mod download` once in each of the six module directories, then run `GOPROXY=off go list -deps ./packages/client/cmd/mornlea-godot-assets`. Run `make rust` before the direct asset check so the cgo link consumes a candidate-built engine library.

9. Restore the paired transition gates and prove the required/optional split is green together:

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
