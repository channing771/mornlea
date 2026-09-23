# Node 4.2: Native, quality, and integration entry points

## Identity and readiness

- Planning baseline: `d2416113`; execute from the controller-provided verified baseline.
- Direct predecessors: Nodes 1.1, 3.1, and 4.1.
- Deliverable: every required workflow stage has one local Make entry point, and Linux bundle/load plus server/client integration semantics no longer live inline in workflow YAML.
- Required sub-skills: `superpowers:test-driven-development` and `superpowers:verification-before-completion`.

## File ownership

- Create: `scripts/ci/verify-linux-bundle.sh`.
- Create: `scripts/ci/run-linux-quality.sh`.
- Create: `scripts/ci/run-integration-server.sh`.
- Create: `scripts/ci/run-integration-client.sh`.
- Modify: `Makefile` (`CI_CANDIDATE_SHA`, manifest paths, `.PHONY`, help, and all remaining `ci-*` targets).
- Create: `packages/audit/ci_entrypoints_test.go`.
- Read-only authority: `scripts/ci/AGENTS.md`; current `.github/workflows/ci.yml`; `packages/audit/companion_agent_service_test.go`; `packages/audit/identity_test.go`; `packages/shared/nativeabi/native.go`; `packages/client/client/window.go`.
- Excluded: workflow runner/action selection, changing test selectors, performance thresholds, protocol behavior, or swallowing a benchmark/report structural failure.

## Make interfaces

`CI_CANDIDATE_SHA` defaults to `git rev-parse HEAD` and must match `^[0-9a-f]{40}$|^[0-9a-f]{64}$`. These manifest locations are fixed:

```text
build/ci/native-linux.manifest
build/ci/native-macos.manifest
```

Add these phony targets and exact ownership:

- `ci-rust-quality`: `doctor.sh rust`, then `$(MAKE) rust-check`.
- `ci-frontend`: `doctor.sh frontend`, then `$(MAKE) frontend-check`.
- `ci-native-linux`: `doctor.sh native-linux`; `$(MAKE) build-linux-server`; focused native collision race tests; `verify-linux-bundle.sh --root "$(CURDIR)"`; package the three `linux-amd64` paths from Node 3.1.
- `ci-native-macos`: `doctor.sh native-macos`; `$(MAKE) rust`; derive `macos-arm64|macos-x86_64` through `platform-id.sh`; package the two macOS paths.
- `ci-verify-linux-artifact`: verify `linux-amd64`, current SHA, repository root, and Linux manifest.
- `ci-verify-macos-artifact`: derive the host macOS ID and verify current SHA, root, and macOS manifest.
- `ci-linux-quality`: depends on `ci-verify-linux-artifact`, then invokes `run-linux-quality.sh`.
- `ci-race-server` and `ci-race-rest`: depend on Linux verification and invoke `run-go-race.sh server|rest`.
- `ci-race-client`: depends on macOS verification and invokes `run-go-race.sh client`.
- `ci-integration-server`: depends on Linux verification and invokes `run-integration-server.sh`.
- `ci-integration-client`: depends on macOS verification and invokes `run-integration-client.sh`.

Every target is listed in `make help`. Do not make legacy `build`, `test`, `run`, or `companion-agent-check` depend on a CI or Godot target.

## Script behavior

`verify-linux-bundle.sh --root <absolute-root>` moves `<root>/packages/engine/target` to a fresh temporary backup only for the detached-load probe and restores it in an EXIT trap. Before the move it checks:

- the server dependency closure excludes client rendering/WebGPU packages;
- `readelf -d bin/mornlea-server` names `libmornlea_engine.so` and `$ORIGIN`;
- `nm -D --defined-only bin/libmornlea_engine.so` exports exactly the required four checked symbols: `mornlea_engine_abi_version`, `mornlea_mesh_section`, `mornlea_collision_resolve`, and `mornlea_raycast_batch`;
- after the move, `ldd` resolves the adjacent `bin/libmornlea_engine.so`, `bin/mornlea-server -h` exits 1, output contains `flag: help requested`, and no loader failure text occurs.

`run-linux-quality.sh` uses `package-inventory.sh --all` and `mapfile` to compile every supported package under `GOOS=linux GOARCH=amd64 CGO_ENABLED=1` with `go test "${packages[@]}" -run '^$' -count=1`, then runs `go vet` over that same filtered package list, and finally runs the current audit/storage/network/physics focused tests. This is six-module vet coverage because the filtered list is derived from the complete workspace inventory, not six broad `./...` patterns that reintroduce unsupported Darwin packages. The hosted correction in Node 6.3.2 excludes exactly nine verified Darwin-owned paths: `client/cmd/mornlea`, `client/cmd/mornlea/app`, `client/cmd/mornlea/benchmark`, `client/cmd/mornlea/capture`, `client/cmd/mornlea/devcapture`, `client/cmd/mornlea-godot-core`, `client/render`, `client/render/hud`, and `tools/gfxspike`. The audit test checks the exact filtered selector against the union and requires every excluded path in the Darwin client slice. `go list -e` loadability is not a compile oracle for untagged render files referring to Darwin-only symbols.

`run-integration-server.sh` runs, in order:

```bash
scripts/ci/doctor.sh agent
make companion-agent-check
make companion-agent-integration
go test ./packages/server/server -run 'TestTCPPlayerAndWorld|TestMemoryTCPParity' -race -count=10
```

`run-integration-client.sh` runs the unchanged independent probe first, then the existing M3C/report selector, all-package one-shot benchmarks, and the existing three multiplayer benchmarks. Preserve the exact probe command:

```bash
go test ./packages/client/cmd/mornlea/benchmark -run '^TestScenarioV7EightSessionServerProbeIsRealAndBounded$' -count=1
```

Performance numbers remain informational, while command failures and incomplete reports remain hard failures.

## Test-first steps

1. Add `TestCIEntrypointsOwnRequiredCommands` to parse Make rules and assert every target, prerequisite, manifest argument, script call, and help entry above. Add forbidden-edge assertions for legacy targets. Baseline fails because targets are absent.

2. Add `TestLinuxQualityPlatformExclusionsAreExact`, which computes the Darwin union and cgo-enabled Linux package/error inventory and requires the unsupported set to equal the four explicit paths above. Assert that compile and vet consume the same filtered list. This guards future build-tag and dependency-boundary changes.

3. Add script mutation/source tests for the Linux bundle restore trap and all loader/symbol assertions, plus integration tests that use fake `make`/`go` binaries to capture the exact ordered argv. Run:

   ```bash
   go test ./packages/audit -run '^TestCI(Entrypoints|LinuxQuality|LinuxBundle|Integration)' -count=1
   ```

   Expected baseline result: fail because the scripts/targets are absent.

4. Implement `verify-linux-bundle.sh`; run `bash -n` and its focused audit tests. The real detached-load behavior is accepted only on Linux and will run through `ci-native-linux`; non-Linux tests verify the command contract and trap restoration fixture.

5. Implement `run-linux-quality.sh` and make its fake-command/order tests green. On the current macOS host, validate the package selection with `GOOS=linux GOARCH=amd64 CGO_ENABLED=1 scripts/ci/package-inventory.sh --all` but do not claim a Linux compile pass.

6. Implement both integration scripts and make their capture tests green. Preserve every exact selector from the current workflow; no test moves between race and non-race except the approved server/client split.

7. Add all Make targets and variables. Run `make -n` for every target and verify candidate SHA, platform, manifest path, and prerequisites are passed exactly once.

8. Run available host-focused gates:

   ```bash
   make ci-rust-quality
   make ci-frontend
   bash -n scripts/ci/verify-linux-bundle.sh scripts/ci/run-linux-quality.sh scripts/ci/run-integration-server.sh scripts/ci/run-integration-client.sh
   go test ./packages/audit -run '^TestCI' -count=1
   ```

   Do not run `ci-native-*` merely to satisfy planning; the worker must run the host-compatible native target if its dependencies are present and report the other platform as workflow acceptance owned by Node 5.1.

## Closure

- Re-enumerate every inline command in the old workflow and map it to exactly one new entry point; report any orphan before Node 5.1.
- Run `make test-race-changed RACE_BASE="$task_base"`.
- Commit only owned files with `feat(ci): add native and integration entrypoints`.
- Rollback unit: this commit. The old workflow remains functional until Node 5.1 atomically switches consumers.
- Report focused evidence, host-only limitations, the old-command ownership map, and commit SHA. The controller updates `tasks.md` and `ledger.md`.
