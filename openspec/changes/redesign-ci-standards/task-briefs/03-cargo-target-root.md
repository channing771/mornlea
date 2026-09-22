# Node 2.2: Shared Cargo target-root resolution

## Identity and readiness

- Planning baseline: `d2416113`; execute from the controller-provided verified baseline.
- Direct predecessors: none.
- Deliverable: the Godot extension build locates the library under the exact target root Cargo uses for unset, relative, and absolute `CARGO_TARGET_DIR` values, independent of caller working directory.
- Required sub-skills: `superpowers:test-driven-development`, `superpowers:systematic-debugging`, and `superpowers:verification-before-completion`.

## File ownership

- Modify: `scripts/godot/build-extension.sh`.
- Create: `packages/audit/godot_build_extension_test.go`.
- Read-only authority: `scripts/AGENTS.md`; `packages/engine/AGENTS.md`; `Makefile`; `scripts/godot/build-core.sh`; all callers from `rg -n 'build-extension\.sh' scripts apps packages docs`.
- Excluded: Rust crate behavior, extension ABI, cross-compilation enablement, Godot descriptor paths, and release/debug semantics.

## Interfaces and behavior

- Preserve `--target`, `--profile debug|release`, and `--verify` behavior and existing unsupported-target/host failures.
- Add testability-only root overrides used by the audit test: `MORNLEA_REPOSITORY_ROOT` defaults to the detected repository root, and `MORNLEA_GODOT_PROJECT_ROOT` defaults to `<repository-root>/apps/mornlea-godot`. Production callers do not set them.
- Resolve and export `CARGO_TARGET_DIR` before invoking Cargo:
  - unset or empty: `<repository-root>/packages/engine/target`;
  - absolute: the provided absolute path unchanged after physical parent normalization;
  - relative: `<repository-root>/<provided-relative-path>`.
- Cargo and `source_library` use the same exported absolute directory. The source remains `<target-root>/<target-triple>/<debug|release>/<library-name>`.
- Reject a relative target directory containing a path component equal to `..`; do not allow output outside the repository through a relative override. Exit 2 with `relative CARGO_TARGET_DIR must not contain '..'`.
- Do not fall back to `<engine-root>/target` after Cargo succeeds.

## Test-first steps

1. In `godot_build_extension_test.go`, create a table test with `unset`, `relative`, and `absolute` cases. Each case creates a fake repository containing an empty `packages/engine/Cargo.toml` and Godot destination, plus fake `rustup` and `uname` commands. The fake `rustup` must:
   - print `host: aarch64-apple-darwin` for `rustc -vV`;
   - for `cargo build`, assert exported `CARGO_TARGET_DIR` equals the case's expected absolute root and create `libmornlea_godot.dylib` at `<expected>/aarch64-apple-darwin/debug/`.

   Run the real script without `--verify`, from a temporary working directory outside the fake repository, using both root overrides. Assert exit 0 and byte-identical copy to `addons/mornlea_bridge/bin/macos-universal/debug/libmornlea_godot.dylib`.

2. Add a negative case with `CARGO_TARGET_DIR=../escape`; assert exit 2, the exact diagnostic above, and no fake Cargo invocation marker.

3. Run:

   ```bash
   go test ./packages/audit -run '^TestGodotBuildExtensionUsesEffectiveCargoTargetDirectory$' -count=1
   ```

   Expected baseline result: at least relative and absolute cases fail with `built GDExtension is missing` because the current consumer searches `<engine-root>/target`.

4. Implement the root overrides, validation, normalization, export, and shared `source_library` calculation. Use English comments for the ownership and relative-path decision.

5. Re-run the table and negative cases; expected: all pass.

6. Run existing structural coverage:

   ```bash
   go test ./packages/audit -run 'TestGodot(BridgeHost|ProjectLayout|BuildExtension)' -count=1
   bash -n scripts/godot/build-extension.sh
   ```

## Closure

- Re-enumerate all build-extension callers. Confirm none independently constructs the Cargo output path; `scripts/godot/build-core.sh` may document its own distinct output, but must not override extension lookup.
- Run `make test-race-changed RACE_BASE="$task_base"`.
- Commit only owned files with `fix(godot): honor the effective cargo target directory`.
- Rollback unit: this commit alone. A rollback restores the known producer/consumer mismatch and is not acceptable as a CI workaround.
- Report table-case evidence, the rejected traversal case, caller inventory, and commit SHA. The controller updates `tasks.md` and `ledger.md`.
