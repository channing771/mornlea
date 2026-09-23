## Context

Pull request #184 exposed three deterministic defects, but the current CI topology made them slow and noisy to diagnose:

- the required macOS native build ran for more than thirteen minutes before most policy and race jobs could start;
- Linux compilation failed because a Linux-buildable asset command consumed CPU atlas data hidden behind a Darwin build tag;
- Godot project validation depended on `rg` without checking or provisioning it, so the validator printed a success message after prerequisite failures and the mutation tests caught the fail-open behavior;
- the Godot extension build inherited the repository `CARGO_TARGET_DIR` but searched a different hard-coded output tree.

The current workflow also mixes required product validation with an optional Godot migration leaf. This conflicts with the approved architecture rule that Godot CI remains outside the required pipeline until a separate cutover decision. Existing audit tests encode parts of the old workflow layout, so workflow changes and their executable policy tests must move together.

The repository has six Go modules, Rust native libraries for Linux and macOS consumers, a macOS graphical client, and an optional Godot presentation path. CI must preserve complete race coverage, candidate-bound native artifacts, hard failures for incomplete or lossy results, and local reproduction of every required gate.

## Goals

- Surface platform-neutral policy, configuration, and dependency failures before expensive native builds complete.
- Provide one stable, fail-closed merge result that represents every required validation job for the exact candidate commit.
- Build and verify native artifacts on the platform that consumes them, with explicit provenance and integrity metadata.
- Keep all six Go modules covered by a complete, disjoint race-test partition.
- Make CI jobs thin wrappers around versioned local entry points with explicit tool prerequisites.
- Run Godot validation as an honest, visible optional leaf until a future approved cutover promotes it.
- Repair the three deterministic failures found in pull request #184 without changing runtime contracts.

## Non-goals

- This change does not relax tests, convert hard failures into warnings, add automatic retries, or use `continue-on-error` for required work.
- This change does not skip core validation based on changed-file heuristics.
- This change does not alter protocol, save, ABI, benchmark scenario, visual-baseline, gameplay, or performance semantics.
- This change does not promote Godot to a required merge gate or approve the final Godot architecture cutover.
- This change does not require GitHub merge queues or silently mutate repository branch-protection settings.

## Decisions

### 1. Keep one required workflow and move Godot to a separate optional workflow

`.github/workflows/ci.yml` remains the repository's required workflow file so existing repository tooling and external references retain a stable path. Its workflow name becomes the stable required-CI identity, and its only externally required job is `merge-gate`.

Godot validation moves to `.github/workflows/godot.yml`. It runs on explicit Godot-related path changes and by manual dispatch, but it is not a dependency of `merge-gate`. A failing Godot run remains visible and actionable; optional means excluded from merge authority, not allowed to lie or suppress failures.

The workflow split is covered by audit tests. Required-CI tests assert that no Godot job participates in the required graph, while Godot tests assert that the optional workflow retains its static validation, extension build, runtime smoke, and qualification responsibilities.

### 2. Use a layered required job graph

The required workflow uses the following dependency graph:

```text
preflight   frontend   rust-quality   native-linux   native-macos
                                           |              |
                                           +-- linux-quality
                                           +-- race-server +-- race-client
                                           +-- race-rest   +-- integration-client
                                           +-- integration-server
                         all required results
                                  |
                             merge-gate
```

The independent first wave starts immediately:

- `preflight` checks repository policy, OpenSpec validity, workflow invariants, script syntax, package inventory, and required tool availability without requiring native artifacts. It runs the audit suite except the deterministic Godot asset-sync test, whose generator reaches the cgo native engine ABI. The full audit suite, including that test and generated-file consistency, runs in `linux-quality` after the candidate-bound Linux native artifact is verified;
- `frontend` runs the frontend formatting, lint, type, and unit gates;
- `rust-quality` runs Rust formatting, linting, and unit tests that do not require platform-specific artifact handoff;
- `native-linux` builds the Linux server/native bundle and publishes its manifest;
- `native-macos` builds the macOS engine and client libraries and publishes its manifest.

Downstream jobs download and verify the artifact for their platform and candidate commit before running tests. Server-owned integration and Linux quality work consume the Linux artifact. Client race and graphical integration work consume the macOS artifact. No downstream job rebuilds a native library as a fallback.

`merge-gate` uses `if: always()` and fails unless every declared required predecessor completed successfully. It is a pure result aggregator and does not rerun tests. Its predecessor list is explicit and audit-tested so newly added required jobs cannot be accidentally omitted.

Initial service targets are five minutes to actionable preflight feedback and twenty minutes for the required critical path on a warm runner. CI records per-job and critical-path durations and reviews the targets after twenty representative pull-request runs. These targets guide optimization and capacity decisions; exceeding them does not change a correct validation result.

### 3. Make native artifacts candidate-bound and platform-bound

Each native artifact contains a deterministic manifest with:

- the candidate Git SHA;
- a normalized platform identifier;
- an ordered record for every required file containing its relative path, byte size, and SHA-256 digest.

`scripts/ci/verify-native-artifact.sh` accepts the expected platform, expected SHA, and artifact root explicitly. It rejects missing files, extra required records, mismatched SHA or platform, invalid ordering, duplicate paths, size mismatch, and digest mismatch. Verification never searches another target directory or triggers a rebuild.

Build scripts resolve Rust output paths from the effective `CARGO_TARGET_DIR`. When the variable is relative, it is interpreted from the invoking workspace consistently; when it is absolute, it is used directly. Consumer lookup and manifest generation use the same resolved root as Cargo.

### 4. Put executable CI contracts behind local entry points

The `Makefile` exposes named CI entry points for the workflow layers, including preflight, race inventory, Linux quality, server integration, and client integration. Workflow YAML selects runners, installs declared dependencies, restores caches, transfers artifacts, and invokes those entry points; it does not duplicate package-selection or validation logic inline.

`scripts/ci/doctor.sh` checks the commands required by a selected profile before validation begins. Missing prerequisites fail immediately with a single actionable diagnostic. Validators may still check their own mandatory dependencies, but they must exit before doing partial work and must never print a success message after a prerequisite failure.

The full repository audit consumes `rg` through Godot project checks. Linux quality and the `rest` race slice each run that audit in a separate clean job, so both must install ripgrep and invoke an `audit` doctor profile before inventory or test execution; installing it only in preflight does not populate those runners.

`scripts/ci/package-inventory.sh` derives the six-module package universe and compares it with the race partitions. The check fails if a package is missing, duplicated, or assigned to more than one partition. The package inventory, not hand-maintained counts, defines completeness.

Because this change makes `.github/workflows` and `scripts/ci` explicit policy and lifecycle boundaries, concise directory-scoped `AGENTS.md` guidance is added or updated beside them.

### 5. Route validation by supported platform instead of runner convenience

Platform-neutral and server-owned work uses a pinned supported Linux runner. Linux quality compiles and vets the complete supported Linux package set. The six-module race inventory remains the Linux/Darwin union, but Linux quality excludes exactly nine Darwin-owned client packages through one audit-checked list: graphical command, app, capture, developer-capture, benchmark, Godot client-core command, render, render/hud, and gfxspike. Linux `go list -e` loadability alone is insufficient because some render files refer to symbols defined only in Darwin-tagged files; the hosted Linux compiler decides support. All excluded packages remain in the complete Darwin client race slice. macOS is reserved for artifacts and tests that consume Darwin libraries or exercise the graphical client. Runner labels are explicit supported versions rather than floating `*-latest` aliases.

Third-party actions are pinned to immutable commit SHAs and selected from revisions compatible with the runner's supported Node runtime. Every job has an explicit timeout and least-privilege permissions. Caches may accelerate a job but are never treated as validation evidence or a substitute for candidate-bound artifacts.

### 6. Keep Godot optional but complete

The optional Godot workflow has two layers:

- platform-neutral project closure, descriptor, import, and contract checks;
- deterministic-asset generation after six-module Go dependency prefetch and native-engine materialization, followed by pinned Godot/Python runtime materialization, embedded-Python tooling checks, extension build, runtime smoke, and the existing repeated lifecycle qualification on the macOS 26 arm64/Xcode 26.5 environment required by the checked-in runtime inputs.

The deterministic asset command is runtime-owned even though its emitted bytes are platform neutral: its current Go dependency closure reaches the cgo-only native engine ABI. Dependency caches may accelerate this layer, but a cold runner explicitly activates the repository-pinned Rust toolchain and downloads every committed Go module's external requirements before the gate switches back to its locked offline mode. The pinned Godot fetch command materializes the verified editor application at the cache path consumed by headless checks; a downloaded archive alone is not runtime evidence.

The cold GDExtension qualification must build the editor-selected debug library as well as the release distribution library. Verification performs a headless editor import after the debug library is present and before script/scene probes; this discovers the extension in a fresh project's ignored `.godot` cache. A direct release verification without the debug prerequisite fails explicitly. The release build and exported-app probe still qualify the shipped release artifact rather than substituting the debug library for it.

Its path filters include the Godot project, Godot bridge crate, extension build and validation scripts, asset generator and relevant inputs, workflow and Makefile entry points, and executable audit tests. Manual dispatch is always available to diagnose filter mistakes or validate a candidate before cutover.

Promotion of the Godot result into `merge-gate` requires a separate OpenSpec decision with measured reliability evidence; it is not implicit in this redesign.

### 7. Repair the first observed failures at their ownership boundaries

The implementation starts with regression tests and then makes these narrow repairs:

- CPU atlas packing, mip generation, and exported atlas pixels move into platform-neutral asset code; only GPU upload and WebGPU ownership remain Darwin-specific. The generated bytes and asset contract stay unchanged.
- Godot project validation checks `rg` before any scan, and the CI doctor provisions or rejects the selected environment before invoking the validator. A missing `rg` must produce a non-zero exit and no success output.
- the Godot extension builder and consumer derive their output path from the same effective Cargo target root, including target-triple and build-profile components.
- the Godot fetcher atomically materializes the verified editor archive at the cache path used by headless runtime commands instead of relying on a preinstalled application or a warm runner.

These fixes are independently testable and remain useful even if workflow orchestration is rolled back.

The first hosted Linux candidate also exposed a two-ULP mismatch between the Go
encoder's displacement envelope and the Rust integrator's fixed-step result.
The native diagnostic identified displacement rejection, not malformed bytes.
The repair keeps Rust as the sole production integrator and retains its one-ULP
rejection contract and frozen output vectors. The Go envelope mirrors the
Rust `vec3_len` square-and-add order with explicit fused operations only along
the fixed-step target, acceleration, and airborne-clamp path. The shared
movement-direction helper is also consumed by sneak-edge probing; its resulting
direction must continue to track the production Rust target and be regression
tested. This is a parity correction, not a new movement or ABI policy. The
temporary native stderr probe is removed after the Go-side regression passes,
and the frozen source-provenance digest is restored before PR acceptance.

### 8. Treat repository settings as an explicit migration step

The repository-owned contract names `merge-gate` as the required check. After an exact-head required workflow is green, a repository owner may configure branch protection to require that check. Changing GitHub repository settings is an external state change and is performed only with explicit authorization. If permissions are unavailable, the missing protection is recorded as an external rollout blocker rather than hidden by workflow logic.

## Validation strategy

Implementation follows test-driven development at each boundary:

1. Audit tests first reject the old mixed workflow, missing `merge-gate` dependencies, floating runner labels, mutable action references, absent timeouts, and accidental Godot inclusion in the required graph.
2. Shell regression tests first demonstrate that missing `rg` fails closed and that an overridden `CARGO_TARGET_DIR` is honored end to end.
3. A platform-neutral audit first demonstrates that the atlas implementation belongs to the Linux source set; on Linux it also compiles the asset generator, while the required Linux quality entry point provides the real compile acceptance for every candidate.
4. Native artifact tests cover SHA, platform, ordering, path, size, and digest mutations.
5. Race inventory tests prove that the union of partitions is the complete six-module package universe and that intersections are empty.
6. Focused package and script tests run during each repair, followed by the repository's proportionate T1/T2 gates.
7. Before push, the full required local gates and strict OpenSpec validation run against the same commit.
8. After push, the exact PR head must produce a green `merge-gate`. The optional Godot workflow is also expected to pass for relevant changes, but it is not merge authority.

No test may launch or focus a foreground game window. Godot runtime smoke remains headless.

## Risks and trade-offs

- Building native artifacts on both Linux and macOS increases total compute. Parallel startup and removal of redundant downstream builds reduce elapsed time and make ownership correct.
- Workflow separation can allow path filters to drift. Audit tests cover the owned path set, and manual dispatch provides a recovery path.
- Existing audit tests are coupled to the old workflow file. They are changed before the workflow so a topology regression is visible as a failing test rather than a silent policy gap.
- Optional Godot failures may receive less attention than required failures. The workflow remains visible, fail-closed, and complete, and promotion requires a deliberate reliability review.
- Pinning runners and actions creates explicit maintenance work. Upgrades are deliberate reviewed changes with audit fixtures, rather than ambient changes from floating aliases.

## Migration and rollback

1. Add failing workflow-policy, script, artifact, race-inventory, and Linux asset-consumer tests.
2. Repair portable atlas ownership, prerequisite checks, and Cargo target-root resolution.
3. Add the local CI entry points, artifact manifest contract, package inventory, and scoped agent guidance.
4. Rewire `.github/workflows/ci.yml` into the required layered graph and move Godot to `.github/workflows/godot.yml`.
5. Reconcile audit fixtures, canonical specifications, architecture documentation, test documentation, and the change ledger.
6. Run focused gates, strict OpenSpec validation, and the full pre-push suite; then verify both workflows on the exact PR head.
7. With separate explicit authorization, configure branch protection to require the stable `merge-gate` check.

If orchestration must be rolled back, revert the workflow and CI-entry-point changes as one coherent unit. Keep the portable atlas, fail-closed prerequisite, and target-root fixes unless a regression is demonstrated; restoring the known fail-open or platform-mismatched behavior is not an acceptable rollback.

## Open questions

None. The implementation plan may refine filenames and command composition without changing these ownership, gating, or failure-policy decisions.
