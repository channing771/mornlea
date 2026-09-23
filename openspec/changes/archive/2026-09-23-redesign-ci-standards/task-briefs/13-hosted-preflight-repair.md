# Node 6.3.1: Keep cold preflight artifact-free

## Identity and readiness

- Baseline: pushed PR #185 candidate `887eaa88`; Node 6.3 remains open.
- Deliverable: cold Linux preflight completes without native libraries while the verified-artifact Linux quality job still runs the complete audit suite.
- Required sub-skills: `superpowers:systematic-debugging`, `superpowers:test-driven-development`, and `superpowers:verification-before-completion`.

## Evidence and contract

- Hosted `preflight` failed in [run 35816085845, job 107037787246](https://github.com/channing771/mornlea/actions/runs/35816085845/job/107037787246): `TestGodotAssetSyncIsDeterministicAndRejectsManualFiles` invoked `sync-assets.sh --check`, whose Go command links `libmornlea_engine` before any native artifact exists.
- `go list -deps ./cmd/mornlea-godot-assets` includes `packages/shared/nativeabi`. Linux quality already verifies the candidate-bound Linux artifact before running full `go test ./packages/audit`.
- Keep all other audit tests in artifact-free preflight. Skip only `^TestGodotAssetSyncIsDeterministicAndRejectsManualFiles$` there, using Go's `-skip` flag. Do not skip it in `linux-quality` or Godot runtime, build a fallback library, or change the asset generator.

## Files and steps

- Modify `packages/audit/ci_local_contracts_test.go`: add a contract test asserting that the preflight audit command excludes exactly the native-backed asset-sync test and the Linux quality script retains full audit execution. The baseline must fail because preflight has no exclusion.
- Modify `Makefile`: change only the final `ci-preflight` audit invocation to `$(GO) test ./packages/audit -skip '^TestGodotAssetSyncIsDeterministicAndRejectsManualFiles$' -count=1`.
- Modify the bilingual `docs/continuous-integration.md` and `.zh.md` pair (and manifest metadata if required): explain that preflight runs artifact-free audits and Linux quality runs the complete suite after artifact verification.
- Reconcile the active design, completed Node 4.1 brief, and ledger. No workflow graph, native production source, or merge authority change.

## Red, green, and closure

1. Run `go test ./packages/audit -run '^TestCIPreflightRecipeOrderAndBoundary$' -count=1` after adding the new assertion; expect a missing `-skip` failure.
2. Apply the one-line Make repair; rerun the focused audit and `go test ./packages/audit -count=1` with a native artifact available locally. Run `make ci-preflight` and strict OpenSpec/documentation audits. `make ci-linux-quality CI_CANDIDATE_SHA=<exact local SHA>` is a Linux-only local gate when a matching verified artifact is available; otherwise the hosted Linux quality job supplies that evidence.
3. Commit the bounded repair using `fix(ci): defer native-backed asset audit` and push without force. Review the PR's next exact SHA; a green rerun on the old SHA does not count.
4. Record runner, duration, run URL, and the red-to-green evidence in `ledger.md`. Rollback unit is this scoped commit; do not close Node 6.3 until all three hosted acceptance statuses pass on one exact candidate.
