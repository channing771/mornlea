# Node 6.3.3: Provision Linux audit tools

## Identity and readiness

- Baseline: PR #185 run 35816085845; implement after Node 6.3.2 because Linux quality's script is serially owned there.
- Deliverable: both Linux jobs that run the audit suite have `rg` before invoking it, and local entry points report the absent prerequisite before partial validation.
- Required sub-skills: `superpowers:systematic-debugging`, `superpowers:test-driven-development`, and `superpowers:verification-before-completion`.

## Contract and files

- `race-rest` job 107038383009 failed in the Godot project audit with `missing required executable: rg`; `preflight` installs it, but job environments are isolated. `linux-quality` runs the same full audit after native verification and must provision it too.
- Edit `.github/workflows/ci.yml`, `packages/audit/ci_workflow_standard_test.go`, `scripts/ci/doctor.sh`, `scripts/ci/run-go-race.sh`, `scripts/ci/run-linux-quality.sh`, `packages/audit/ci_local_contracts_test.go`, and `packages/audit/ci_entrypoints_test.go`. The controller owns planning/doc reconciliation. No other workflow, package inventory, native artifact or Godot validator changes.
- Add an `audit` doctor profile requiring exactly `bash go gofmt rg`. In `run-go-race.sh`, invoke it for `rest` before inventory/test, not for `server` or `client`. In `run-linux-quality.sh`, invoke it before package inventory; remove its weaker standalone `go` check. Preserve failure-on-missing-tool and no partial `go test` behavior.
- In each of `linux-quality` and `race-rest` workflows, add the same unconditional `sudo apt-get update` then `sudo apt-get install --yes ripgrep` setup used by `preflight`, before native artifact download and `make` command. `preflight` remains unchanged.

## Test-first and acceptance

1. Extend doctor profile and missing-`rg` fixtures; add script-entrypoint fake-PATH tests proving that an absent `rg` stops `rest`/Linux quality before inventory or Go test, while server/client race retain their existing tool set. The baseline must fail for the missing `audit` profile.
2. Extend `requiredWorkflowViolations` so the exact install sequence is required for `preflight`, `linux-quality`, and `race-rest`; add a removal mutation for each new job. The baseline must fail with missing setup in both jobs.
3. Implement the profile, script calls, and workflow setup; run `go test ./packages/audit -run '^(TestCIDoctorProfilesAndFailures|TestCILinuxQualityOrderedCommandsAndFailures|TestRequiredCIWorkflow|TestRequiredCIWorkflowMutations)' -count=1`, `bash -n scripts/ci/doctor.sh scripts/ci/run-go-race.sh scripts/ci/run-linux-quality.sh`, and `git diff --check`. Run full `go test ./packages/audit -count=1` and `make ci-preflight` at integration boundary.
4. The new exact-head `race-rest` and `linux-quality` hosted jobs supply cold-run proof. Commit only the owned files as `fix(ci): provision linux audit tools`; keep every failure hard and do not use retries, bypasses, or cached-tool assumptions. Rollback unit is this scoped commit.
