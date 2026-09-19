# Godot Client Pilot Baseline

## Scope and identity

This document freezes the implementation baseline for the `pilot-godot-client-migration` change. It is a planning record, not a declaration that the Godot client is already production-ready.

Forward architecture is defined by [`docs/architecture-target.md`](../architecture-target.md): embedded Python remains the final Godot feature language, while Rust replaces the Go real-time server/client-core path. P0–P7 evidence below describes the migration pilot and must not be read as approval to expand the pilot's Go data path or to introduce production GDScript features.

| Field | Baseline |
|---|---|
| Source commit | `84f3e0e75dff6987e47cf2aa50f50d888e85eafb` |
| Worktree | Dirty by design: the governance change and unrelated user-owned renderer work are not part of this snapshot identity |
| Protocol | protocol v44 |
| Numerical engine | engine ABI v11 |
| Existing graphical client | client ABI v19 |
| Benchmark | benchmark scenario v23 |
| Existing graphical host | Darwin `mornlea_client`, using winit/wgpu and WKWebView |
| Pilot host scope | Godot 4 Standard on macOS desktop; Windows/Linux desktop are future candidates |
| Explicit exclusions | Android, iOS, Web, console, touch, sensors, and mobile lifecycle |

Current behavior remains defined by code and tests. The version identities above are checked by `TestBaselineVersionsMatchCode`; the P0 manifest records run-specific machine and scenario identity separately.

## Current client inventory

The Go client module currently exposes twelve packages. Counts below describe the frozen source tree and are navigation aids rather than architecture limits.

| Area | Frozen source shape | Principal function | Representative tests | Pilot disposition |
|---|---:|---|---|---|
| `packages/client/assets` | 10 production / 16 test Go files | Registry, procedural textures, atlas, item icons and geometry | `atlas_test.go`, `pack_test.go`, `procedural_test.go` | Retain as the source of truth; generate deterministic Godot derivatives |
| `packages/client/audio` | 2 / 3 | Semantic cue derivation and Darwin playback | `cue_test.go`, `player_darwin_test.go` | Keep cue semantics; pilot audio remains disabled |
| `packages/client/client` | 40 / 54 | Mirror, receiver, predictor, mesher scheduling, entity and UI mirrors, client ABI bridge | `mirror_test.go`, `predictor_*_test.go`, `mesher_*_test.go`, `remote_players_test.go` | Reuse and compose through the platform-independent runtime |
| `packages/client/lod` | 4 / 4 | Far-ring scheduling and LOD descriptions | `scheduler_test.go`, `oracle_test.go` | Not migrated in P0–P7 |
| `packages/client/mesh` | 6 / 15 | Section input validation and calls to engine ABI v11 | `native_abi_test.go`, `native_parity_test.go`, `greedy_oracle_test.go` | Reuse without a second meshing implementation |
| `packages/client/render` | 21 / 45 | Host-neutral presentation parameters plus current GPU encodings | `section_scheduler_test.go`, `daylight_test.go`, `weather_test.go`, `block_outline_test.go` | Reuse semantic calculations; replace only Godot presentation resources |
| `packages/client/render/hud` | 12 / 11 | HUD layout and current renderer data | `layout_test.go`, `renderer_test.go`, `container_test.go` | Pilot exposes only confirmed basic HUD values |
| `packages/client/cmd/mornlea` | 2 / 5 | Existing command and mode selection | `run_test.go`, `options_test.go` | Remains the default; must not probe Godot |
| `packages/client/cmd/mornlea/app` | 36 / 57 | Local/remote assembly, message routing, frame lifecycle, input, UI and audio coordination | `app_connection_test.go`, `app_input_prediction_test.go`, `app_render_test.go`, `app_loading_test.go` | Characterize first; extract portable ownership one loop at a time |
| `packages/client/cmd/mornlea/benchmark` | 8 / 11 | Fixed benchmark scenario v23 and report production | `benchmark_scenario_test.go`, `benchmark_report_test.go` | Remains the comparison producer |
| `packages/client/cmd/mornlea/capture` | 26 / 38 | Windowless visual scenes and comparisons | `capture_scene_test.go`, `visual_compare_test.go` | Keeps tracked visual-producer ownership during the pilot |
| `packages/client/cmd/mornlea/devcapture` | 5 / 6 | Interactive capture coordination | `service_test.go`, `record_test.go` | Not migrated in P0–P7 |

The existing Rust graphical host is `packages/engine/crates/mornlea_client`. Its stable ownership includes:

- `window.rs`, `input.rs`, and `camera.rs`: Darwin window, device events, and host camera adaptation;
- `render/`: wgpu terrain, entity, weather, viewmodel, shader, and resource-pool implementation;
- `overlay.rs` and `webview.rs`: the current WKWebView/React interface;
- `capture.rs`: current client capture support;
- `ffi.rs` and `bridge.rs`: client ABI v19.

The pilot does not modify those ownership facts. It adds a second host and preserves the old one as the default comparison path.

## Existing verification entry points

| Concern | Existing entry point | Baseline meaning |
|---|---|---|
| Version and architecture identity | `go test ./packages/audit -run TestBaselineVersionsMatchCode -count=1` | Current protocol/ABI/scenario claims match code-owned constants |
| Client mirror/predictor/mesher | `go test ./packages/client/client -race -count=1` | Portable client facts remain race-clean |
| Existing app behavior | `go test ./packages/client/cmd/mornlea/app -race -count=1` | Current app assembly and frame behavior remain available |
| Mesh numerical parity | `go test ./packages/client/mesh -race -count=1` | Go inputs continue to use `mornlea_engine` results |
| Rust workspaces | `make rust-check` | Existing numerical and graphical Rust clients still build and test |
| Visual evidence | `make visual-check` | Existing `ui/`, `world/`, and human-review `motion/` producer contract remains active |
| Scenario v23 benchmark | `make bench-multiplayer` | Fixed multiplayer microbenchmarks remain available |
| Full client compatibility | `make test-race` and `make frontend-check` | The pilot does not replace the old Go/Rust/WebView path |

Tracked visual evidence currently contains 31 `ui` PNG files, 31 `world` PNG files, and 11 `motion` GIF files. Godot does not own or update any of them during P0–P7; its evidence goes under `build/visual/godot-pilot/<run-id>/`.

## Explicitly non-migrated in P0–P7

- The authoritative Go server, simulation, persistence, world metadata, and companion Agent service.
- Local Memory single-player assembly and save access.
- Protocol v44 packet definitions, login semantics, and strict codecs.
- The engine ABI v11 numerical implementations for mesh, lighting, collision, raycast, physics, world generation, LOD, and fluids.
- Complete menus, inventory/container/crafting/chest/furnace/chat/settings UI.
- Full React/WKWebView replacement or embedding decision.
- Audio playback, far-ring LOD, viewmodel, complete actors/effects, particles, name tags, and advanced weather.
- Production Windows/Linux packaging and every mobile, Web, or console target.
- Default-client switching, client ABI v19 retirement, and removal of `mornlea_client`.

Unimplemented pilot capabilities must be absent, disabled, or explicitly identified as unavailable. Empty features must not advertise support or send guessed protocol commands.

## P0–P14 mapping

| Stage | Scope | Exit evidence | Rollback boundary |
|---|---|---|---|
| P0 | Freeze identities, client inventory, visual/benchmark references, baseline manifest, and dependency provenance | Complete, validated baseline inputs | Remove only new planning/provenance data |
| P1 | Stable `apps/mornlea-godot/` root, pure Bootstrap, feature host/catalog, desktop tooling, and minimal GDExtension probe | Project opens without native libraries and repeatedly loads/unloads with them | Delete the additive project/crate/scripts |
| P2 | Platform-independent presentation values, transcripts, runtime lifecycle, remote session, prediction, camera, and mesh scheduling | Old app and extracted runtime produce equivalent semantic summaries | Keep old app ownership and revert extraction wiring |
| P3 | Client-core ABI v1 plus Rust/Godot bridge, feature negotiation, bounded inputs, step, world/frame pulls, status, and lifecycle | Cross-language errors are atomic and repeated destruction is safe | Remove ABI/bridge without changing client ABI v19 |
| P4 | Near-ring terrain, bounded quad expansion, RID resources, chunk delta/reset, and TCP terrain smoke | Correct content/transparency with bounded preparation and upload | Preserve report as No-Go evidence or delete pilot terrain |
| P5 | Desktop input, prediction, first-person camera, target feedback, and disconnect paths | Protocol/input/prediction terminal state matches the old client | Old client remains the default |
| P6 | Remote player, basic environment, confirmed HUD values, and disabled future capability identities | Minimum remote presentation loop uses confirmed state only | Disable optional manifests |
| P7 | Headless capture, scenario v23 benchmark report, visual classification, and Go/No-Go review | Every hard condition has complete identity and an explicit decision | No-Go removes pilot runtime entry points but preserves evidence |
| F1 | Rust domain, protocol, storage contracts, deterministic kernels, and replay reference | Separate Rust foundation change after P7 | Keep Go as offline oracle; no dual writer |
| F2 | Rust authoritative server, validation, persistence, and shared local/remote session path | F1 replay and migration evidence | Keep Go authority until cutover; never dual-write |
| F3 | Rust client-core and typed Godot bridge for session, mirror, prediction, reconciliation, and semantic frames | F1/F2 protocol contract | Keep pilot Go core without adding features |
| P8 | Production terrain, LOD, water, atmosphere, and resource pools | F3 plus separate post-Go OpenSpec change | Catalog can select the earlier world feature |
| P9 | Complete actors, effects, and viewmodel | F3 plus parity evidence | Disable features independently |
| P10 | Godot Control UI driven by embedded Python and Rust view-models | F3 plus separate route change | Keep old UI client as rollback producer |
| P11 | Desktop audio, device input, and optional controller | F3 plus separate desktop-only change | Disable device adapters |
| P12 | Rust replay/perf contracts and Godot/Python capture, benchmark, devcapture, import, and CI tooling | F1–P9 as applicable | Continue the old toolchain |
| P13 | Rust server-core local play and macOS/Windows/Linux desktop packaging | F2–P12 | Return to remote-only pilot or old client |
| P14 | Default-client switch and retirement of Go real-time runtime/client ABI | F1–P13 plus rollback release | Restore the previous release; no partial retirement |

Only P0–P7 are implementation scope for the current change. A P7 Go decision authorizes proposals for P8–P14, not their implementation and not a default switch.

## Baseline evidence status

| Evidence | Canonical location | P0 action |
|---|---|---|
| Existing visual indexes and thresholds | `testdata/visual-golden/README.md` and code-owned comparison settings | Reference and run non-update checks only |
| Existing historical performance reports | `docs/notes/perf-baseline.json` and `docs/notes/perf-baseline-m5.json` | Preserve unchanged; these files contain scenario v15 and v14 respectively and are not valid v23 pilot inputs |
| Current legacy-client v23 report | `testdata/godot-pilot/legacy-rust-memory-v23.json` | Use as the machine-readable P0 comparison source, bound by SHA-256 in the manifest |
| New run identity | `testdata/godot-pilot/baseline-manifest.json` | Validate strictly before pilot comparisons |
| Pilot visual output | `build/visual/godot-pilot/<run-id>/` | Keep untracked; never promote during this change |

The worktree began with unrelated render changes and one modified tracked world golden. A failing visual comparison is recorded as external evidence and must not be converted into a baseline update by this change.

## P0 execution record

All commands below ran on Darwin/arm64 with Apple M2 / 16 GiB, macOS 26.6.2, Metal 4, source commit `84f3e0e75dff6987e47cf2aa50f50d888e85eafb`, and the explicitly dirty worktree recorded in the manifest. The frozen render target is 2560×1440 and the benchmark view distance is 32.

| Command | Output | Result |
|---|---|---|
| `make visual-check` | Captures in `build/visual/`; tracked references remain under `testdata/visual-golden/{ui,world,motion}` | Comparison-only run completed all 31 world scenes. Twenty-eight matched. `grass-closeup` differed at 1,287/230,400 pixels (maximum channel difference 127), `mining-crack-early` at 620/230,400 (48), and `mining-crack-heavy` at 576/230,400 (48), so the command returned failure. No golden-update command ran and no tracked golden was written by the pilot. UI PNGs and motion GIFs are referenced unchanged rather than regenerated. |
| `make bench-multiplayer` | Standard benchmark output only; no report file | Passed three runs each of `RemotePlayerStateCodec`, `EightPlayerInterest`, and `RemoteAvatarNameTag` on Darwin/arm64 and Apple M2. |
| `go run ./packages/client/cmd/mornlea --benchmark --benchmark-transport memory --perf-output build/perf/godot-pilot/legacy-rust-memory-v23.json` | Promoted unchanged to `testdata/godot-pilot/legacy-rust-memory-v23.json` for reproducible comparison; SHA-256 `e588b40e268f317fee1acaa4ff5c1e0ae035cc2ec98dc170e9dc90ce6750339e` | Passed report completeness and wrote scenario v23 with 9,835 still frames, 46,245 flying frames, 128 GPU-completion samples, 200 authoritative tick samples, and 296 streaming samples. The recorded flying P99 of 20.211 ms exceeded its informational threshold but did not change exit status. |

The new manifest references only the current scenario v23 JSON. It deliberately does not relabel the historical v14/v15 JSON files as v23 evidence. The visual failure is a frozen pre-pilot variance on the dirty worktree, not an accepted producer handoff; later Godot evidence must classify it separately and may not overwrite or relax the tracked baseline.
