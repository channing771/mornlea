# Node 4.1: Preflight, dependency profiles, and race inventory

## Identity and readiness

- Planning baseline: `d2416113`; execute from the controller-provided verified baseline.
- Direct predecessors: Node 3.1 for artifact-script command names; Node 2.1 for the fail-closed validator contract.
- Deliverable: platform-neutral preflight and the three race slices are repository-owned, locally callable, complete, pairwise disjoint, and independently testable.
- Required sub-skills: `superpowers:test-driven-development` and `superpowers:verification-before-completion`.

## File ownership

- Create: `scripts/ci/doctor.sh`.
- Create: `scripts/ci/check-package-partitions.sh`.
- Create: `scripts/ci/package-inventory.sh`.
- Create: `scripts/ci/run-go-race.sh`.
- Modify: `Makefile` to add `ci-preflight` only in this node; Node 4.2 owns the remaining CI targets.
- Create: `packages/audit/ci_local_contracts_test.go`.
- Read-only authority: `go.work`; `packages/audit/helpers_test.go` (`workspaceModules`, `listWorkspacePackages`); `packages/audit/unit_boundary_test.go`; `scripts/agents/gates.sh`; `scripts/agents/race-changed.sh`; `scripts/agent-hooks/guard.mjs`; `docs/notes/test-quickstart.md`; `scripts/ci/AGENTS.md` from Node 3.1.
- Excluded: changing the committed module set, race test semantics, the independent server-probe identity, native builds, or workflow YAML.

## Interfaces and algorithms

`scripts/ci/doctor.sh <profile>` supports these exact profiles and command sets:

```text
preflight: bash git go gofmt node npx rg
frontend: bash corepack git node
rust: bash cargo rustc rustup
go: bash go gofmt
native-linux: bash cargo cc go ldd make nm readelf rustc rustup shasum
native-macos: bash cargo codesign go install_name_tool make nm rustc rustup shasum
agent: bash go python3 uv
godot-static: bash make rg
godot-runtime: bash cargo cc clang++ codesign curl ditto git go install_name_tool make nm patch perl pgrep rg rustc rustup sandbox-exec shasum tar unzip uv xcrun
```

It checks every command, reports all missing names in lexical order as `missing required executable for <profile>: <name>`, and exits 1 when any are absent. Unknown profiles exit 2 with a usage line. A success prints `CI dependency profile passed: <profile>`.

`scripts/ci/check-package-partitions.sh <all> <client> <server> <rest>` accepts four newline-delimited package files. It requires each input to be nonempty, lexically sorted, and internally unique. It rejects pairwise intersections and then compares the sorted union with `all`. Diagnostics use `duplicate package in <slice>:`, `overlapping package:`, `missing package:`, and `unexpected package:` followed by the import path.

`scripts/ci/package-inventory.sh` supports `--all`, `--slice client|server|rest`, and `--check`:

- `--all` is the sorted union of Linux/amd64 and Darwin/arm64 `go list -e` results for all module directories parsed from `go work edit -json`. Both supported source-set queries set `CGO_ENABLED=1`; otherwise a cross-platform query from macOS silently omits the established Linux `packages/shared/nativeabi` package before compilation.
- `client` uses the cgo-enabled Darwin/arm64 client module package set plus `github.com/channing771/mornlea/packages/tools/gfxspike`.
- `server` uses the cgo-enabled Linux/amd64 server module package set.
- `rest` uses the cgo-enabled Linux/amd64 contracts, shared, tools, and audit package sets; Linux excludes the Darwin-only `gfxspike` package while retaining `packages/shared/nativeabi`.
- `--check` materializes those four lists and invokes `check-package-partitions.sh`.

The script verifies that the parsed module directory set is exactly `packages/audit`, `packages/client`, `packages/contracts`, `packages/server`, `packages/shared`, and `packages/tools` before listing packages. It does not keep a second package-count constant. Planning evidence is 57 packages on Darwin and 54 on Linux; counts are informational, not acceptance constants.

`scripts/ci/run-go-race.sh <client|server|rest>` reads a slice with `mapfile`, rejects an empty result, and runs one command:

```text
client: go test "${packages[@]}" -race -p=1 -skip '^TestScenarioV7EightSessionServerProbeIsRealAndBounded$'
server/rest: go test "${packages[@]}" -race -p=1
```

`make ci-preflight` runs, in order: `doctor.sh preflight`, gofmt cleanliness, `openspec validate --all --strict --no-interactive` through pinned `npx --yes @fission-ai/openspec@1.7.0`, `node --test scripts/agent-hooks/guard.test.mjs`, `make comment-language-check`, `package-inventory.sh --check`, and `go test ./packages/audit -skip '^TestGodotAssetSyncIsDeterministicAndRejectsManualFiles$' -count=1`. The exact asset-sync test is deferred because its generator links the native engine. The full audit suite runs after Linux artifact verification in `linux-quality`. Preflight never invokes Cargo, `make rust`, artifact download/verification, or a Godot runtime.

## Test-first steps

1. Add audit tests that invoke `doctor.sh` with a temporary `PATH`: one success fixture per profile, one fixture missing two commands and asserting lexical diagnostics, and one unknown profile. The baseline fails because the script is absent.

2. Add partition fixtures for a valid four-file set and mutations with a duplicate, client/server overlap, missing package, unexpected package, unsorted line, and empty slice. Each mutation asserts its stable diagnostic and nonzero status.

3. Add a repository test that runs `package-inventory.sh --check`, asserts the exact six module directories through `go work edit -json`, verifies `gfxspike` occurs exactly once in `client` and never in `rest`, and verifies `packages/shared/nativeabi` occurs exactly once in `rest`. Its fake-command/source assertions pin `CGO_ENABLED=1` for both supported platform queries.

4. Run the red suite:

   ```bash
   go test ./packages/audit -run '^TestCI(Doctor|PackagePartition|RepositoryPackageInventory)' -count=1
   ```

   Expected: fail because the new scripts do not exist.

5. Implement `doctor.sh`, then run only `TestCIDoctor`; expected: all profile and negative cases pass.

6. Implement `check-package-partitions.sh` and `package-inventory.sh`, then run the partition and repository inventory tests. Confirm `package-inventory.sh --check` succeeds and each slice output is sorted and nonempty.

7. Implement `run-go-race.sh`. Add source/argument tests asserting the exact client skip and that unknown/empty slices exit before `go test`; use a fake `go` command to capture argv without running the full race suite.

8. Add `ci-preflight` to `.PHONY`, Make help, and the recipe. Extend the audit test to assert exact command order and forbidden native/Godot tokens. Run:

   ```bash
   make ci-preflight
   bash -n scripts/ci/doctor.sh scripts/ci/check-package-partitions.sh scripts/ci/package-inventory.sh scripts/ci/run-go-race.sh
   ```

## Closure

- Re-enumerate module/package definitions with `rg -n 'GO_TEST_MODULES|packages/contracts/.+packages/shared|workspaceModules|go work edit' Makefile scripts packages/audit`. Record duplicates that remain for non-CI local tooling; do not rewrite `scripts/agents` or hooks in this node.
- Run `make test-race-changed RACE_BASE="$task_base"`; do not use it as proof of the new full partitions.
- Commit only owned files with `feat(ci): add preflight and race inventory entrypoints`.
- Rollback unit: this commit. No workflow consumes these entry points until Node 5.1.
- Report fixture mutation results, real repository inventory evidence, remaining definition consumers, and commit SHA. The controller updates `tasks.md` and `ledger.md`.
