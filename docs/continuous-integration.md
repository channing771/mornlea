---
doc_id: continuous-integration
doc_revision: 2026-09-23.2
language: en
counterpart: continuous-integration.zh.md
---
# Continuous integration

The required workflow is [`.github/workflows/ci.yml`](../.github/workflows/ci.yml), named `Required CI`. It validates the checked-out candidate SHA for pull requests and pushes to `main`; a newer candidate cancels unfinished work for the older one. Run `make ci-preflight` from the repository root to reproduce the early policy checks locally. This checks tool prerequisites, formatting, OpenSpec, agent-hook policy, comment language, six-module package inventory, and repository audits before native artifacts are needed. A missing required tool fails the entry point.

## Required layers

`preflight`, `frontend`, `rust-quality`, `native-linux`, and `native-macos` start independently. The native jobs produce separate Linux and macOS bundles with manifests containing the candidate SHA, platform, ordered file paths, sizes, and SHA-256 digests. Every dependent job downloads its platform's bundle and runs `ci-verify-linux-artifact` or `ci-verify-macos-artifact` before testing. Artifact verification rejects a missing, mismatched, or corrupt bundle; caches do not establish artifact identity and consumers do not rebuild a missing bundle.

| Platform | Jobs after native artifact verification | Local entry points |
| --- | --- | --- |
| Linux (`ubuntu-24.04`) | `linux-quality`, `race-server`, `race-rest`, `integration-server` | `make ci-linux-quality`, `make ci-race-server`, `make ci-race-rest`, `make ci-integration-server` |
| macOS (`macos-15`) | `race-client`, `integration-client` | `make ci-race-client`, `make ci-integration-client` |

The package inventory checks that the disjoint `client`, `server`, and `rest` race slices cover all packages across the six `go.work` modules. The macOS client slice includes `packages/tools/gfxspike`; server and rest run on Linux. Linux quality compiles and vets its supported source set, while the macOS client jobs cover graphical source that depends on Darwin. The independent server timing probe remains outside race testing.

Each local native producer and dependent entry point requires the same `CI_CANDIDATE_SHA` for the candidate being checked. The producer targets are `make ci-native-linux CI_CANDIDATE_SHA=<sha>` and `make ci-native-macos CI_CANDIDATE_SHA=<sha>` on their respective supported platforms. Downstream targets accept the same variable and verify the corresponding manifest before running their checks. See the `Makefile` and `scripts/ci/` for the executable contract.

`Required CI / merge-gate` is the only intended branch-protection status. It succeeds only if every required predecessor succeeds for the candidate; failed, skipped, cancelled, and timed-out work cannot authorize a merge. A failed required job is repaired and rerun explicitly, using GitHub's failed-job rerun when appropriate. Required validation does not automatically retry a failed command or turn failure into success. Repository branch-protection settings are a separate rollout step; the workflow alone does not configure them.

The initial feedback targets are five minutes for actionable preflight results and twenty minutes for the required critical path on a warm runner. Job durations and runner identities are recorded for diagnosis. These timings and numeric performance measurements are informational; incomplete reports, overflow, data loss, invalid identity, and I/O errors remain failures.

## Optional Godot qualification

[`Godot CI`](../.github/workflows/godot.yml) runs for relevant paths and supports manual dispatch. It is outside `merge-gate`, but its own failures remain visible and fail the optional workflow. `godot-static` checks project closure on Linux. After it succeeds, `godot-runtime` runs on macOS 26 arm64 with Xcode 26.5: it prefetches the external Go dependencies of all six workspace modules on a cold runner, builds the native engine before cgo-backed deterministic asset generation, and fetches the checksum-verified Godot editor. It then verifies the embedded Python runtime, builds and verifies the Go client-core/GDExtension distribution, checks Python tooling, runs the unchanged 100-cycle headless lifecycle smoke, and exports and probes a macOS application offline. The export probe uses the fetched editor through the repository resolver. This workflow remains optional until a separate approved cutover changes merge authority.
