# Node 3.1: Candidate-bound native artifact manifests

## Identity and readiness

- Planning baseline: `d2416113`; execute from the controller-provided verified baseline.
- Direct predecessors: none.
- Deliverable: one portable packager and verifier enforce exact platform, candidate SHA, path set, byte size, digest, ordering, and safe extraction for macOS and Linux artifacts.
- Required sub-skills: `superpowers:test-driven-development` and `superpowers:verification-before-completion`.

## File ownership

- Create: `scripts/ci/package-native-artifact.sh`.
- Modify: `scripts/ci/verify-native-artifact.sh`.
- Create: `scripts/ci/platform-id.sh`.
- Create: `scripts/ci/AGENTS.md` covering the complete planned `scripts/ci` boundary so later nodes do not rewrite it.
- Create: `packages/audit/native_artifact_test.go`.
- Remove no artifact producer from `.github/workflows/ci.yml` in this node; workflow conversion belongs to Node 5.1.
- Read-only authority: `scripts/AGENTS.md`; `packages/engine/AGENTS.md`; `scripts/engine/deploy-dylib.sh`; `packages/shared/nativeabi/native.go`; `packages/client/client/window.go`; current native producer and consumers in `.github/workflows/ci.yml`.
- Excluded: building Rust or Go products, changing loader/rpath behavior, caches, workflow topology, or accepting legacy manifests after the new consumers land.

## Interfaces

`scripts/ci/platform-id.sh` has no arguments and prints exactly one of:

- `linux-amd64` for Linux/x86_64;
- `macos-arm64` for Darwin/arm64;
- `macos-x86_64` for Darwin/x86_64.

All other pairs exit 1 with `unsupported CI platform: <uname-s>/<uname-m>`.

`scripts/ci/package-native-artifact.sh` accepts:

```text
--platform <platform-id> --sha <40-or-64-lowercase-hex> \
--root <absolute-repository-root> --manifest <repository-relative-path> \
-- followed by one or more repository-relative artifact path arguments
```

`scripts/ci/verify-native-artifact.sh` accepts the same four named options without `-- <paths>`; the platform selects the exact expected set:

- `macos-arm64` or `macos-x86_64`: `packages/engine/target/release/libmornlea_client.dylib`, `packages/engine/target/release/libmornlea_engine.dylib`.
- `linux-amd64`: `bin/libmornlea_engine.so`, `bin/mornlea-server`, `packages/engine/target/release/libmornlea_engine.so`.

Manifest v1 is UTF-8 text with exactly these ordered records:

```text
version 1
sha <candidate-sha>
platform <platform-id>
file <sorted-repository-relative-path> <decimal-byte-size> <lowercase-sha256>
```

The packager sorts artifact paths bytewise under `LC_ALL=C`. It rejects duplicate paths, non-regular files, symlinks, absolute paths, empty components, `.`/`..` components, control characters, and a manifest path that overlaps an artifact path. It writes through a temporary file in the manifest directory and renames only after every record succeeds.

The verifier rejects every malformed condition above plus missing/extra/reordered records, unexpected platform file sets, SHA/platform mismatch, size/digest mismatch, and resolved paths outside `--root`. It validates everything before copying libraries. After success, it creates `packages/engine/target/release/deps` and copies only the verified dynamic libraries for that platform.

Both scripts use `wc -c` for portable byte size and `shasum -a 256` for digest. They require `bash`, `realpath`, `shasum`, `sort`, `wc`, `mktemp`, `mkdir`, `mv`, and `cp` through an upfront command check.

## Test-first steps

1. Build table-driven Go helpers in `native_artifact_test.go` that create a temporary fake repository with regular files and invoke the real scripts. The positive table covers all three platform IDs and asserts the exact manifest bytes above plus the verified library copies.

2. Add independent mutation subtests for wrong SHA, wrong platform, missing file, extra file record, duplicate record, reversed file order, non-decimal size, size mismatch, uppercase/incorrect digest, absolute path, `../` traversal, symlink artifact, symlink escape under the root, and trailing fields. Each mutation must produce nonzero status before `deps` exists.

3. Run:

   ```bash
   go test ./packages/audit -run '^TestNativeArtifact(ManifestRoundTrip|ManifestMutations|PlatformIdentity)$' -count=1
   ```

   Expected baseline result: fail because the packager and platform command are absent and the current verifier has no required interface.

4. Implement `platform-id.sh` and make its tests green, including an injectable `MORNLEA_CI_UNAME_S`/`MORNLEA_CI_UNAME_M` used only to exercise unsupported and x86_64 cases. Normal execution uses `uname`.

5. Implement the packager exactly as specified. Re-run only `ManifestRoundTrip`; expected: packager assertions pass while verifier-dependent assertions remain red.

6. Replace the fixed macOS verifier with the argument-driven v1 verifier. Re-run the full mutation suite; expected: all positive cases pass and every mutation fails before publication.

7. Run syntax and real-path focused checks:

   ```bash
   bash -n scripts/ci/platform-id.sh scripts/ci/package-native-artifact.sh scripts/ci/verify-native-artifact.sh
   go test ./packages/audit -run '^TestNativeArtifact' -count=1
   ```

## Closure

- Re-run `rg -n 'verify-native-artifact|native-artifact-manifest|native-source-sha' .github scripts packages docs openspec` and report every legacy caller. Do not edit workflow callers in this node; Node 5.1 owns their atomic migration.
- Run `make test-race-changed RACE_BASE="$task_base"`.
- Commit only owned files with `feat(ci): verify platform-bound native artifacts`.
- Rollback unit: this commit plus the later workflow consumer migration. The controller must not land a new consumer with the old verifier or an old consumer with the new interface.
- Report mutation counts, syntax evidence, legacy caller inventory, and commit SHA. The controller updates `tasks.md` and `ledger.md`.
