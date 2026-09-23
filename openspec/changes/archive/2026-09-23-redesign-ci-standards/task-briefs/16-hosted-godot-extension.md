# Node 6.3.4: Qualify the Godot extension from a cold project

## Identity and readiness

- Baseline: PR #185 Godot runtime run 35816085873, job 107037846972. The pinned Godot 4.7.2 runtime completed native/asset/toolchain preparation and failed in `make godot-build` at `MornleaClientBridge is not registered`.
- Deliverable: headless verification succeeds from a fresh project with no `.godot/extension_list.cfg`, while both debug/editor and release/distribution binaries are built and checked.
- Required sub-skills: `superpowers:systematic-debugging`, `superpowers:test-driven-development`, and `superpowers:verification-before-completion`.

## Proven cause and frozen contract

A read-only local reproduction with the real release library first failed with the same unregistered class. Headless editor startup then wrote `.godot/extension_list.cfg`, but selected `macos.debug.arm64` and failed because only `bin/macos-universal/release/libmornlea_godot.dylib` existed. Supplying the debug-selected path made the identity probe pass. The effective Cargo target path was correct.

`make godot-build` must run the pinned offline Python runtime check, build the debug GDExtension without verification, then build the release GDExtension with verification, then build the release Go client core with its existing verification. In `build-extension.sh --verify`, after copying the selected profile's library, require the matching debug library path, perform a headless `--editor --quit` import via `godot.sh`, reject command failure or Godot `ERROR:`/`SCRIPT ERROR:` output, and only then run the existing identity and bridge-host probes. Both profiles use the same import/verification path; a direct release `--verify` with no debug library must fail with a specific prerequisite diagnostic. Do not copy release bytes into the debug path, suppress editor errors, or change the `.gdextension` profile mapping. The exported-app probe remains the release qualification.

## File ownership and implementation

- Edit only `Makefile`, `scripts/godot/build-extension.sh`, `packages/audit/godot_build_extension_test.go`, `packages/audit/godot_entrypoints_test.go`, and the bilingual `docs/continuous-integration.md` / `.zh.md` pair plus `docs/documentation-manifest.json`. Read root, scripts, audit, and docs `AGENTS.md` files. The controller owns OpenSpec reconciliation and integration.
- Add a test fixture with fake Cargo/Godot commands and a fresh temporary project. It must fail first on the current script because no editor import occurs. The fake Godot checks that the debug library exists during import and records call order; identity and bridge-host probes require an import marker. Exercise debug `--verify`, release `--verify` after a debug build, and explicit failure of a cold release `--verify` without debug. Do not launch a foreground window.
- Pin the exact `godot-build` recipe ordering in the existing Makefile audit, red before implementation. Keep the existing effective `CARGO_TARGET_DIR` and profile tests green.
- Update both CI guide languages and manifest to state the debug/editor import prerequisite and release export ownership.

## Validation and closure

- Run `go test ./packages/audit -run '^(TestGodotBuildExtension|TestGodotIsOptionalForLegacyBuild|TestGodotProject)' -count=1`, `bash -n scripts/godot/build-extension.sh`, `scripts/godot/rollback-check.sh`, bilingual documentation audit, strict change validation, and `git diff --check`. Run real headless cold-project reproduction if local pinned editor and runtime are available; otherwise Node 6.3's new exact-head Godot runtime job supplies proof.
- Do not edit workflow YAML, Godot plugin descriptor, engine source, or runtime gameplay. Commit only the owned files as `fix(godot): qualify cold extension import`; this is the rollback unit. The controller reviews test evidence and hosted outcome.
