# Node 1.1: Portable CPU atlas export

## Identity and readiness

- Planning baseline: `d2416113`; execute from the controller-provided verified baseline.
- Direct predecessors: none.
- Deliverable: `assets.Registry.AtlasPixels` and its mip helpers compile and behave identically on Linux and Darwin, while no GPU ownership moves out of the existing client boundary.
- Required sub-skills: `superpowers:test-driven-development` and `superpowers:verification-before-completion`.

## File ownership

- Modify: `packages/client/assets/atlas.go`.
- Modify: `packages/client/assets/atlas_test.go`.
- Modify: `packages/client/assets/default_pack_test.go` (`atlasPixelsForTest`).
- Modify after the source commit: `apps/mornlea-godot/assets/generated/manifest.json`.
- Create: `packages/audit/linux_asset_source_set_test.go`.
- Read-only authority: `packages/client/AGENTS.md`; `packages/client/cmd/mornlea-godot-assets/main.go`; `packages/client/cmd/mornlea/app/app_startup.go`; `packages/client/client/render.go`; `packages/engine/crates/mornlea_client/src/ffi.rs`; `packages/engine/crates/mornlea_client/src/render/mod.rs`.
- Excluded: generated Godot assets, Rust upload semantics, texture bytes, layer ordering, `atlasMips`, and ABI values.

## Interfaces and behavior

- Preserve the existing public signature exactly: `func (r *Registry) AtlasPixels() (int, []byte)`.
- Preserve `atlasMips = 5`, layer-major then mip-major byte ordering, `texSize`-derived lengths, box averaging for opaque layers, and max-alpha coverage for cutout layers.
- Remove the Darwin build constraint from the CPU implementation and its tests. Do not add a Linux stub or copy of the algorithm.
- `atlasPixelsForTest` must call `registry.AtlasPixels()` directly; it must no longer use an interface assertion or skip on non-Darwin platforms.
- Rewrite any comment substantively touched by this move in English. The function comment must say that the bytes are shared by native renderer upload and deterministic asset generation; it must not claim ownership by a removed `UploadTo` method.

## Test-first steps

1. Create `TestGodotAssetGeneratorCompilesForLinux` in `packages/audit/linux_asset_source_set_test.go`. It runs from the repository root:

   ```go
   command := exec.Command("go", "test",
       "./packages/client/assets",
       "./packages/client/cmd/mornlea-godot-assets",
       "-run", "^$",
       "-count=1",
   )
   command.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
   ```

   The test fails on the planning baseline with `registry.AtlasPixels undefined`. Its error output must include the full command output.

2. Remove `//go:build darwin` from `atlas_test.go` and replace the conditional `atlasPixelsForTest` helper with a direct call. Run:

   ```bash
   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test ./packages/client/assets ./packages/client/cmd/mornlea-godot-assets -run '^$' -count=1
   ```

   Confirm the same undefined-method failure before changing production code.

3. Remove the build constraint from `atlas.go`; keep one implementation and update only the ownership comments described above.

4. Run the atlas behavior tests on the host:

   ```bash
   go test ./packages/client/assets -run 'Test(Downsample|WheatLayers|CutoutMip|AtlasPixels|CrackLayers|DefaultRegistryAtlas)' -count=1
   ```

   Expected: pass, including the exact existing byte-length and cutout-path assertions.

5. Run the Linux source-set test and focused audit:

   ```bash
   go test ./packages/audit -run 'TestGodotAssetGeneratorCompilesForLinux' -count=1
   GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go test ./packages/client/assets ./packages/client/cmd/mornlea-godot-assets -run '^$' -count=1
   ```

   Expected: both pass; no test is skipped because of the host platform.

6. Record `shasum -a 256 apps/mornlea-godot/assets/generated/atlas.rgba8`, then commit the source/test repair. Run `scripts/godot/sync-assets.sh` from that committed source tree so `assets_git_tree` names the new `packages/client/assets` Git tree. The generator is authoritative because it scans every regular asset source, including the two modified tests.

7. Assert the atlas hash is unchanged, `git diff --exit-code -- apps/mornlea-godot/assets/generated/atlas.rgba8` succeeds, and only `manifest.json` changes in the generated tree. Run:

   ```bash
   scripts/godot/sync-assets.sh --check
   go test ./packages/audit -run '^TestGodotAssetSyncIsDeterministicAndRejectsManualFiles$' -count=1
   ```

   Commit the derived manifest separately after both commands pass.

## Closure

- Inspect `rg -n 'AtlasPixels|layerMipChain|atlasMips' packages/client packages/engine` and confirm there is one CPU implementation and every consumer retains the same signature and byte contract.
- Run `make test-race-changed RACE_BASE="$task_base"`.
- Use two ordered commits: `fix(assets): make atlas export platform neutral`, then `chore(godot): refresh atlas provenance` for the generated manifest.
- Rollback unit: both commits together. Reverting only the provenance commit leaves `godot-asset-check` red; reverting both restores the Linux compile failure but does not affect later CI interfaces.
- Report the red command/output, green commands/output, final commit SHA, and any unexpected platform consumer to the controller. The controller updates `tasks.md` and `ledger.md`.
