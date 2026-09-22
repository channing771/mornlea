---
doc_id: godot-client-pilot-report
doc_revision: 2026-09-19.1
language: en
counterpart: godot-client-pilot-report.zh.md
---

# Godot client pilot dual-client report

This document records the P7 dual-client review for `pilot-godot-client-migration`. It compares the existing Rust/WebView client with the Godot Python-primary pilot. The Godot path is not the default entry point.

Decision: GO

## Run identity

| Field | Legacy Rust client | Godot pilot |
|---|---|---|
| Machine-readable report | `testdata/godot-pilot/legacy-rust-memory-v23.json` | `testdata/godot-pilot/godot-pilot-v23.json` |
| Git commit | `84f3e0e75dff6987e47cf2aa50f50d888e85eafb` (P0 baseline) | `376f435a262bfc8fa611866dedfee8c56225444f` |
| Worktree | dirty (P0) | dirty |
| Protocol | protocol v44 | protocol v44 |
| Engine ABI | engine ABI v11 | engine ABI v11 |
| Client ABI | client ABI v19 | client ABI v19 (unused by the pilot) |
| Client-core ABI | n/a | v1 |
| Benchmark | benchmark scenario v23 | benchmark scenario v23 windows |
| Godot | n/a | 4.7.2-stable |
| Py4Godot | n/a | 4.7-alpha21, source `d8e17428deeb0428587349b663f6da26cd71ef3a` |
| CPython | n/a | 3.14.4 embedded |
| Catalog | n/a | `apps/mornlea-godot/config/feature_catalog.tres` |
| Platform | darwin/arm64, macOS 26.6.2, Apple M2, Metal 4 | darwin/arm64, macOS 26.6.2, Apple M2 (Apple8), API 4.0 |
| Resolution | 2560×1440 | 2560×1440 requested |
| View distance | 32 | 2 (login minimum; connect family is address-only) |
| Seed | 20260726 | 20260726 |
| Sample windows | warmup 10s, still 60s, flying 120s, cooldown 30s, min 128 GPU samples | same requested windows; 25,062 CPU samples |
| Visual evidence | tracked `testdata/visual-golden/{ui,world,motion}` | untracked `build/visual/godot-pilot/<run-id>/` |

## Visual evidence

The Godot pilot captured one fixed-camera world frame at 640×360 from the deterministic terrain transcript (`world/terrain-settled.png`) with complete identity. Godot 4.7 headless display only exposes the dummy renderer, so capture used a no-focus minimized macOS Metal window instead of a foreground window.

`make godot-visual-compare` classified every tracked baseline against the existing dual threshold (max channel delta 2, diff-pixel ratio 0.0001) without writing `testdata/visual-golden/` or creating `testdata/visual-golden/godot/`:

- 31 `ui/` fixtures: not-covered (WebView remains the UI producer)
- 31 `world/` scenes: not-covered (offscreen Rust capture remains the world producer; the pilot frame is transcript terrain, not a `captureScenes` world)
- 11 `motion/` GIFs: human-review-only (no automated pixel comparison)

## Performance

Values below are recorded only. Overflow and failed uploads remain hard failures and were both 0.

| Metric | Legacy Rust v23 | Godot pilot | Decision method |
|---|---|---|---|
| CPU frame P50/P95/P99 (ms) | still 5.998 / 6.431 / 7.257; flying 1.676 / 8.007 / 20.211 | 6.896 / 6.944 / 16.667 (25,062 samples) | record; flying P99 is informational on the legacy path too |
| GPU frame P50/P95/P99 | 128 GPU-completion samples | non-comparable: minimized Metal window did not expose GPU timestamps | do not zero |
| Python apply P50/P95/P99 (ms) | n/a | 0.506 / 0.769 / 1.206 | bounded main-thread apply |
| Python process P50/P95/P99 (ms) | n/a | 1.074 / 1.712 / 2.746 | bounded host `_process` |
| Allocation pressure | n/a | P50/P95/P99 = 53/56/58 allocated-block deltas per frame (stored in the quantile fields) | record interpreter pressure |
| Cold start login / visible world | load_seconds 43.807 | 419.7 ms to Play | different loaded criterion; do not claim a faster fake load |
| Packed / expanded bytes | packed upload | 760,056 packed, 19,001,400 expanded (~25×) | expected CPU expansion |
| Prepare / upload (ms, cumulative) | n/a | 108.0 prepare, 1003.5 upload | record |
| RID / mesh | n/a | 184 live RIDs, 92 sections, peak 184 | no per-block nodes |
| RSS peak (bytes) | still 1,614,807,040; flying 1,735,655,424 | 581,484,544 | record; includes eagerly loaded `libmornlea_client.dylib` frameworks |
| Input-to-presentation P50/P95/P99 (ms) | explicit probe | 6.896 / 6.944 / 7.407 (17,265 look samples) | do not count server tick as an engine difference |
| Overflow / failed uploads | hard failure | 0 / 0 | hard failure if nonzero |

Fields that cannot be compared with the Memory v23 observer (authoritative tick P99, protocol encode/decode, player persistence, eight-session probe, GPU completions, view distance 32) are omitted from the Godot JSON rather than written as zero.

## Feature coverage

Covered by the pilot: remote TCP login, bounded step, near-ring terrain, desktop keyboard/mouse semantic input, predicted camera, target outline, remote-player pool, confirmed HUD values, environment color mapping, disconnect/reset.

Not covered: local Memory assembly, complete menus/containers, viewmodel, particles, far ring, advanced weather, audio devices, production capture/benchmark producers, default-client switch.

## Difference classification

1. Must be equivalent: protocol, mirrors, prediction, overflow fail-closed. Held by runtime/transcript tests and zero overflow.
2. May differ within approved bounds: antialiasing, tone mapping, font rasterization. Not pixel-compared; no producer handoff.
3. Not covered by the pilot: listed in Feature coverage. All tracked UI/world goldens remain on the existing producers.

## Final review

| Hard condition | Result | Evidence |
|---|---|---|
| Exact hardened Py4Godot/CPython artifact | GO | Pinned revision, patch series, offline qualification |
| No system Python or runtime install | GO | Isolated `PyConfig`, poisoned-environment tests |
| No authority/protocol/prediction fork | GO | Shared Go runtime and protocol v44 |
| No silent loss or partial batch | GO | overflow_count 0, uploads_failed 0 |
| Terrain and minimum entity stable | GO | terrain check, 300s playable smoke, 92 live sections in the v23-window run |
| Repeated GDExtension/Go core close | GO | 100-cycle isolated smoke |
| Python main-thread cost bounded | GO | apply P99 1.21 ms, process P99 2.75 ms |
| Complete report identity | GO | `testdata/godot-pilot/godot-pilot-v23.json` validates |
| GPU timestamp comparison | Evidence Missing | non-comparable field, not zeroed |
| View-distance 32 parity | Evidence Missing | connect family is address-only; login uses view distance 2 |
| Lifecycle crash | GO | no crash in smoke, capture, or the v23-window run |

No unqualified runtime, system Python, unbounded Python hot path, data loss, authority fork, missing identity, or lifecycle crash was observed. GPU and view-distance gaps are recorded follow-on work, not hidden zeros.

## Follow-on

P8–P14 are split into candidate OpenSpec changes. The default `mornlea` entry point is unchanged.
