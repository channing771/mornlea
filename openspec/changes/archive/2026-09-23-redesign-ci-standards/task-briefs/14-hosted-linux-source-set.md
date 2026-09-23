# Node 6.3.2: Select the Linux-compilable package set

## Identity and readiness

- Baseline: PR #185 run 35816085845, Linux quality job 107038383000; implement after Node 6.3.1 commit `1cd51524`.
- Deliverable: Linux quality compiles and vets every supported Linux package from the six-module union while all Darwin-owned exclusions remain in the macOS client race slice.
- Required sub-skills: `superpowers:systematic-debugging`, `superpowers:test-driven-development`, and `superpowers:verification-before-completion`.

## Root cause and frozen selection

`package-inventory.sh --all` is the Linux/Darwin union. The current four-path exclusion leaves five Linux compile-invalid packages: `client/cmd/mornlea` has no Linux Go files; `client/render` references `Glyph` and `growEncodeBuffer` defined in Darwin-tagged files; `client/render/hud`, `client/cmd/mornlea/benchmark`, and `client/cmd/mornlea-godot-core` depend on render. `go list -e` accepts some of them, so its loadability difference is not a compile oracle. Do not move GPU/render ownership into Linux or shrink the union/race partition.

Exclude exactly these repository-relative package paths in `run-linux-quality.sh`: `client/cmd/mornlea`, `client/cmd/mornlea/app`, `client/cmd/mornlea/benchmark`, `client/cmd/mornlea/capture`, `client/cmd/mornlea/devcapture`, `client/cmd/mornlea-godot-core`, `client/render`, `client/render/hud`, `tools/gfxspike`. Keep every other package in the compile and vet argv. The compiler remains the authoritative hosted proof; do not substitute `go list` for compilation.

## File ownership and implementation

- Edit only `scripts/ci/run-linux-quality.sh` and `packages/audit/ci_entrypoints_test.go`. Read `scripts/ci/package-inventory.sh`, `packages/audit/AGENTS.md`, and `scripts/ci/AGENTS.md` as authority. The controller owns OpenSpec and documentation edits.
- Replace the old `TestLinuxQualityPlatformExclusionsAreExact` loadability-difference oracle with a real selector test: call `package-inventory.sh --all` and `--slice client` using `exec.Command.Output()` so a module-download notice on stderr never becomes a package; assert every one of the nine literals is present in the union and client slice; execute `run-linux-quality.sh` with the existing recorder and real inventory; compare its compile and vet package argv to sorted union minus exactly those nine paths. Preserve the exact full-audit assertion in `TestCILinuxQualityOrderedCommandsAndFailures`.
- Add one fixture that emits a Go module-download notice on stderr while `go list` writes valid import paths to stdout; the selector test must ignore the notice as package data. A file-output fixture is acceptable if it invokes the real script boundary rather than comparing a mock to itself.
- First run the changed selector test against the old script: it must fail because five excluded paths remain in compile/vet argv. Then update only the `case` exclusions in `run-linux-quality.sh`; repeat the focused suite and Bash syntax check.

## Validation and closure

- Run `go test ./packages/audit -run '^(TestLinuxQualityPlatformExclusionsAreExact|TestCILinuxQualityOrderedCommandsAndFailures)$' -count=1`, `bash -n scripts/ci/run-linux-quality.sh`, `scripts/ci/package-inventory.sh --check`, and `git diff --check`.
- A macOS host cannot claim Linux cgo compilation without a Linux compiler; Node 6.3's new exact-head `linux-quality` job must provide that evidence.
- Do not edit workflow YAML, native production code, race partition identities, or any OpenSpec artifact. Commit only owned files as `fix(ci): select linux-compilable source set`. The controller reviews the diff and records evidence; this commit is the rollback unit.
