# Node 5.1: Layered required workflow

## Identity and readiness

- Planning baseline: `d2416113`; execute from the controller-provided verified baseline.
- Direct predecessors: Nodes 1.1, 3.1, 4.1, and 4.2.
- Deliverable: `.github/workflows/ci.yml` contains only the required layered graph, uses immutable environment identities, and exposes `Required CI / merge-gate` as the single merge-authority result.
- Required sub-skills: `superpowers:test-driven-development` and `superpowers:verification-before-completion`.

## File ownership

- Modify: `.github/workflows/ci.yml`.
- Create: `.github/AGENTS.md` covering both required and optional workflows.
- Create: `packages/audit/ci_workflow_standard_test.go`.
- Modify: `packages/audit/companion_agent_service_test.go` (structured workflow contract and shared YAML structs).
- Modify: `packages/audit/companion_agent_service_repair_test.go` (new integration-server fixture and mutations).
- Modify: `packages/audit/identity_test.go` (`requireLinuxServerBundleIdentity`).
- Modify: `packages/audit/comment_gate_integration_test.go` (comment gate now reached through `ci-preflight`).
- Read-only authority: all `ci-*` entry points from Nodes 4.1/4.2; `scripts/godot/rollback-check.sh`; `README.md` badge; active OpenSpec specs/design.
- Excluded: `.github/workflows/godot.yml`, Godot entrypoint audit conversion, branch-protection mutation, workflow retries, and changing any repository-owned test selector.

## Immutable environment identities

Use these exact action commit SHAs, with a trailing YAML comment naming the reviewed major tag:

```text
actions/checkout@fbc6f3992d24b796d5a048ff273f7fcc4a7b6c09        # v5
actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16       # v6
actions/setup-node@a0853c24544627f65ddf259abe73b1d18a591444     # v5
actions/setup-python@ece7cb06caefa5fff74198d8649806c4678c61a1   # v6
actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02 # v4
actions/download-artifact@634f93cb2916e3fdff6788551b99b062d0335ce0 # v5
astral-sh/setup-uv@37802adc94f370d6bfd71619e3f0bf239e1f3b78       # v7
```

These are the reviewed lightweight-tag commits (or the annotated-tag commit for setup-uv), not tag-object IDs. `ci_workflow_standard_test.go` owns the allowlist and rejects every other external `uses:` value unless it is a repository-local action.

Use only `ubuntu-24.04` x64 and `macos-15` arm64. Set workflow-level `permissions: contents: read` and:

```yaml
env:
  CARGO_TARGET_DIR: ${{ github.workspace }}/packages/engine/target/cargo
```

Preserve PR plus `main` push triggers and concurrency group `required-ci-${{ github.event.pull_request.number || github.ref }}` with cancellation.

## Exact job graph

| Job | Runner | Timeout | Needs | Repository command |
| --- | --- | ---: | --- | --- |
| `preflight` | `ubuntu-24.04` | 10 | none | `make ci-preflight` |
| `frontend` | `ubuntu-24.04` | 15 | none | `make ci-frontend` |
| `rust-quality` | `ubuntu-24.04` | 30 | none | `make ci-rust-quality` |
| `native-linux` | `ubuntu-24.04` | 30 | none | `make ci-native-linux CI_CANDIDATE_SHA="$GITHUB_SHA"` |
| `native-macos` | `macos-15` | 30 | none | `make ci-native-macos CI_CANDIDATE_SHA="$GITHUB_SHA"` |
| `linux-quality` | `ubuntu-24.04` | 20 | `native-linux` | `make ci-linux-quality CI_CANDIDATE_SHA="$GITHUB_SHA"` |
| `race-server` | `ubuntu-24.04` | 30 | `native-linux` | `make ci-race-server CI_CANDIDATE_SHA="$GITHUB_SHA"` |
| `race-rest` | `ubuntu-24.04` | 30 | `native-linux` | `make ci-race-rest CI_CANDIDATE_SHA="$GITHUB_SHA"` |
| `race-client` | `macos-15` | 45 | `native-macos` | `make ci-race-client CI_CANDIDATE_SHA="$GITHUB_SHA"` |
| `integration-server` | `ubuntu-24.04` | 30 | `native-linux` | `make ci-integration-server CI_CANDIDATE_SHA="$GITHUB_SHA"` |
| `integration-client` | `macos-15` | 45 | `native-macos` | `make ci-integration-client CI_CANDIDATE_SHA="$GITHUB_SHA"` |
| `merge-gate` | `ubuntu-24.04` | 5 | all eleven jobs above | result assertions only |

Every job checks out the candidate with the pinned checkout action. Add only the setup actions required by its command. `integration-server` uses Python `3.12` and setup-uv input `version: '0.12.5'`, `enable-cache: true`, and both Agent dependency files. `frontend` uses Node `24` and the locked pnpm cache input. Go jobs use Go `1.26` and the existing module/header cache dependencies.

`native-linux` uploads `native-linux-${{ github.sha }}` with the Linux manifest and three platform files. `native-macos` uploads `native-macos-${{ github.sha }}` with the macOS manifest and two dylibs. Retention is one day and missing files are errors. Each consumer downloads its exact same-SHA artifact to `.` before invoking its Make command; the Make target performs verification.

`merge-gate` uses `if: ${{ always() }}` and an explicit `needs` sequence containing every other job. Its only run step contains one equality assertion per need; failure, cancellation, or skip therefore fails. It has no checkout or artifact step.

Each non-aggregator job records elapsed seconds and `${RUNNER_OS}/$(uname -m)` to `GITHUB_STEP_SUMMARY` in an `always()` step. Duration is informational.

## Test-first steps

1. In `ci_workflow_standard_test.go`, implement a structured YAML parser and `requiredWorkflowViolations`. Assert workflow name, events, concurrency, permissions, env, exact job set, runners, timeouts, needs, command, setup versions, action allowlist, artifact names/paths, `merge-gate` assertions, and the absence of Godot commands/jobs, mutable action tags, `*-latest`, retries, and `continue-on-error`.

2. Add mutation cases for every Review Focus failure: delete a required job from merge needs; change `always()`; skip an assertion; add Godot; use `ubuntu-latest`; replace an action SHA with `@v5`; remove a timeout; widen permissions; change an artifact SHA name; download to the wrong root; add `continue-on-error`; and duplicate an inline package/test selector instead of a `make ci-*` command.

3. Rewrite the companion workflow fixture around `integration-server`, `native-linux`, pinned setup-python/setup-uv/download actions, Linux runner, exact same-SHA Linux artifact, `make ci-integration-server`, and `merge-gate`. Preserve Python/uv version and cache assertions. Update identity/comment tests to inspect the new job/entry-point names rather than obsolete inline commands.

4. Run the red tests against the old workflow:

   ```bash
   go test ./packages/audit -run 'Test(RequiredCIWorkflow|CompanionAgentCI|EnglishCommentGateIntegration|MornleaCurrentIdentity)' -count=1
   ```

   Expected: fail on mutable actions/runners, absent timeouts/permissions/new jobs, old integration topology, and missing merge gate.

5. Create `.github/AGENTS.md` before rewriting YAML. It states that workflow files are thin orchestration, required status is `merge-gate`, actions/runners/timeouts are explicit, Godot is optional, and focused validation is the audit command above plus a YAML parse.

6. Rewrite `ci.yml` to the exact graph. Remove the `godot` job entirely; Node 5.2 adds the optional replacement. Do not leave a compatibility `test` aggregator.

7. Run:

   ```bash
   go test ./packages/audit -run 'Test(RequiredCIWorkflow|CompanionAgentCI|EnglishCommentGateIntegration|MornleaCurrentIdentity)' -count=1
   ```

   Also run `TestGodotIsOptionalForLegacyBuild` and `scripts/godot/rollback-check.sh` to record the expected transition failures: both old gates still require the in-file Godot job until Node 5.2 atomically creates the optional workflow and migrates those consumers. Do not weaken or edit those gates in this node. Full audit, rollback, and changed-scope race acceptance are deferred only across this one ordered commit boundary and become mandatory in Node 5.2.

## Closure

- Inventory all `.github/workflows/ci.yml` readers with `rg -n '\.github/workflows/ci\.yml|ci\.yml' . --glob '!openspec/changes/archive/**'`; every live consumer must either be updated here, explicitly assigned to Node 5.2, or shown to be path-only and still correct.
- Do not claim full-audit or changed-scope-race green at this transient boundary. Node 5.2 immediately consumes this commit, replaces the missing optional workflow, and runs full audit plus `make test-race-changed` from this node's pre-change base so the pair is accepted together.
- Commit only owned files with `feat(ci): layer required validation by platform`.
- Rollback unit: this commit together with Nodes 3.1/4.1/4.2. Reverting only the workflow after removing the legacy verifier interface is invalid.
- Report all policy mutation results, updated consumers, workflow parse evidence, and commit SHA. The controller updates `tasks.md` and `ledger.md`.
