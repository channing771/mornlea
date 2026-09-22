---
doc_id: architecture-target
doc_revision: 2026-09-20.1
language: en
counterpart: architecture-target.zh.md
status: target-not-current
---
# Mornlea target architecture

This document is the design authority for new architecture decisions. It describes the intended product runtime after migration, not the implementation that currently exists. The current implementation remains documented in [`architecture.md`](architecture.md); a transition exception in an approved OpenSpec change does not redefine this target.

## 1. Target decision

The final real-time product runtime has one Rust domain and runtime core, a Godot presentation host driven by embedded Python, and an independent Python service for companion AI. Rust owns authoritative state, protocol, persistence contracts, client session state, prediction, and numerical kernels. Godot/Python owns scene composition and presentation behavior. Python does not own the authoritative game loop merely because it is the Godot scripting language.

The final product does not use production GDScript as its feature language. A pure-GDScript bootstrap may remain during migration only to open an unprepared project and report missing native/Python artifacts. It is a transition mechanism and must not receive gameplay features.

## 2. Target topology

```text
                       presentation and devices
  +---------------------------------------------------------------+
  | Godot + qualified embedded Python                            |
  | scenes / UI / input / audio / animation / resource lifecycle |
  +-----------------------------+---------------------------------+
                                | typed semantic bridge
                                v
  +---------------------------------------------------------------+
  | Rust client-core                                             |
  | protocol session / mirror / prediction / reconciliation      |
  | client state / mesh preparation / presentation snapshots     |
  +-----------------------------+---------------------------------+
                                | versioned protocol
                                v
  +---------------------------------------------------------------+
  | Rust authoritative server                                    |
  | tick / world / entities / rules / validation / persistence   |
  +-----------------------------+---------------------------------+
                                | shared deterministic crates
                                v
  +---------------------------------------------------------------+
  | Rust domain and numerical kernel                             |
  | physics / collision / worldgen / fluids / pathfinding        |
  | voxel transforms / lighting / meshing                        |
  +---------------------------------------------------------------+

  Independent Python Agent
  Planner / Dialogue / Memory / tools
             -- candidate intent over a versioned service contract -->
             Rust server -- revalidate at a tick boundary --> world state

  Go during migration only
  legacy runtime / replay oracle / differential tests / conversion tools
  (never a second online authority)
  ```

The Rust client-core and Rust server share domain and protocol crates, but never share mutable authoritative state. The Godot host sees semantic snapshots and semantic input; it does not see packet bytes, storage records, or raw numerical buffers.

## 3. Ownership by language and boundary

| Area | Final owner | Boundary rule |
|---|---|---|
| Authoritative tick and gameplay outcomes | Rust server/domain | The server is the only writer of world, player, entity, inventory, and companion outcomes. |
| Deterministic numerical work | Rust kernel | Physics, collision, raycast, worldgen, fluids, pathfinding, mesh, and light have one production implementation. |
| Network protocol and compatibility | Rust protocol | Packet schemas, codecs, framing, version negotiation, and replay identities are owned together. |
| Persistence and migrations | Rust storage/server | Save schemas and migrations are server-owned and never implemented by a client host. |
| Client session and state | Rust client-core | Login, packet receipt, mirror updates, prediction, reconciliation, budgets, and semantic frame assembly live here. |
| Scene and presentation features | Godot plus embedded Python | Python maps typed semantic values to scenes, controls, audio cues, animation, and resources within explicit budgets. |
| Godot native adapter | Rust GDExtension | One typed bridge owns lifecycle, conversion, buffer validation, and the client-core link. Python never loads a raw C symbol. |
| Companion AI | Independent Python service | It returns bounded candidates and summaries; the Rust server validates every world effect. |
| Migration and offline tooling | Go temporarily, then Rust/tooling | Go may compare old and new runtimes offline, but it is not a final real-time product dependency. |

The two Python environments are intentionally separate. The embedded Godot Python runtime is a product dependency with a pinned, reproducible distribution closure. The companion Agent Python environment is an independent service with its own process, lockfile, contracts, and failure policy. Neither imports the other.

## 4. What Python does and does not mean

Using Python as the Godot scripting language is a presentation decision, not a decision to put the game simulation in an interpreter. Godot Python may own feature assembly, scene lifecycle, UI state mapping, input adaptation, audio routing, animation triggers, and other bounded presentation orchestration. It may not own authoritative rules, protocol codecs, client prediction, save handling, physics integration, per-cell numerical loops, or direct server writes.

If a future feature needs designer-authored behavior, its data and execution model must remain deterministic and server-validatable. A Python callback cannot become an implicit authority. A separate sandboxed scripting or data-driven change would be required for extensible gameplay behavior; it is not created by adding Python code to a Godot feature.

The embedded Python runtime must pass editor, headless, export, isolation, lifecycle, and repeated teardown qualification. The current Py4Godot derivative is evidence for the migration path, not an unconditional final dependency. If it cannot meet those requirements, the project must qualify a successor or stop the Godot product cutover; it must not silently introduce system Python, runtime installation, or GDScript gameplay fallback.

## 5. Runtime data flow

```text
Godot input
  -> Python semantic InputBatch
  -> Rust client-core
  -> Rust protocol/session
  -> Rust authoritative server tick
  -> validated world mutation and persistence observation
  -> Rust protocol snapshot
  -> Rust client-core mirror/reconciliation/presentation snapshot
  -> Python typed feature views
  -> Godot scene, UI, audio, and rendering resources
```

The server is the sole authority. Client prediction is reversible and is replaced or corrected by confirmed snapshots. AI proposals enter the same validated command path as human input. Local Memory and remote TCP are transport choices over the same login, packet, validation, and server-core path; local mode must not become a privileged simulation implementation.

Cross-language calls are coarse-grained and bounded. A frame, input batch, world publication, or entity set crosses a boundary as an owned semantic value or immutable snapshot. Per-cell FFI, raw pointer retention, and repeated byte shuffling between Python, Go, and Rust are prohibited.

## 6. Target Rust crate direction

The eventual Rust workspace should converge toward independently testable crates with one-way dependencies similar to:

```text
mornlea-godot -> mornlea-client-core -> mornlea-protocol
                                      -> mornlea-domain
                                      -> mornlea-kernel

mornlea-server -> mornlea-storage
               -> mornlea-protocol
               -> mornlea-domain
               -> mornlea-kernel
```

Names are implementation guidance, not an ABI promise. The important property is ownership: the server and client-core consume shared Rust contracts and kernels, while Godot/Python consumes only a typed presentation surface. A new crate or bridge must not recreate a second rule implementation merely to preserve a transitional language boundary.

## 7. Test ownership

| Test concern | Final home | Required evidence |
|---|---|---|
| Rules, tick ordering, authority, persistence | Rust server/domain/storage | Unit, property, fuzz, failure-injection, and deterministic replay tests |
| Physics, worldgen, fluid, mesh, light | Rust kernel | Numerical oracles, property tests, benchmark records, and cross-platform determinism checks |
| Protocol and save compatibility | Rust protocol/storage | Golden wire/save fixtures, version migration tests, malformed-input tests, and replay corpus |
| Client mirror, prediction, reconciliation | Rust client-core | Transcript parity, correction/replay tests, bounded-queue and race tests |
| Godot presentation | Godot/Python test harness | Typed-bridge contract tests, scene lifecycle tests, input/UI/audio behavior, and semantic visual evidence |
| AI service | Standalone Python service | HTTP/MCP contract tests, candidate validation tests, cancellation and persistence-isolation tests |
| Migration parity | Go temporarily | Offline differential replay against the Rust reference; no Go-only behavior is accepted as a final contract |

Tests follow the final owner. A Go test that exists only because the current implementation is Go is migration evidence, not a reason to keep gameplay ownership in Go.

## 8. Migration sequence

1. Freeze language-neutral protocol, save, event, input, and replay identities. Publish this target and mark current Go/Python seams as explicit transition exceptions.
2. Extract or reimplement the domain, protocol, storage contracts, and numerical kernels in Rust. Keep Go as an offline oracle and adapter; do not run two online writers.
3. Build the Rust authoritative server and validate it against recorded Go replays. Cut over server authority only after deterministic replay, persistence migration, and failure-path parity pass.
4. Build Rust client-core and a typed Godot bridge. The existing Go client-core may continue to feed the pilot, but no new feature may add to it. Move ownership one bounded capability at a time.
5. Keep Python as the final Godot feature language while replacing the Go client-core and Python wire/data work with Rust client-core semantic views. Retire the pure-GDScript bootstrap once the product distribution can diagnose its own missing runtime through the native launcher.
6. Migrate terrain, actors, UI, audio, local play, tooling, and release packaging as independent changes. Every feature consumes the Rust semantic contract and remains disableable until parity is proven.
7. Switch the default product entry only after the Rust server, Rust client-core, Godot/Python presentation stack, local/remote paths, and release rollback package have passed multiple release cycles. Then retire Go from the real-time product path and retain only approved tools until their Rust replacements are complete.

Each phase has a reversible boundary. Rollback selects the previous release or replay/oracle path; it does not keep two authorities live and does not silently rewrite saves.

## 9. Explicit no-go directions

- Do not add new authoritative gameplay logic to Go because the current server is still Go.
- Do not add new real-time gameplay, protocol, prediction, or persistence logic to Python because Godot supports Python.
- Do not make Godot Physics or scene state authoritative.
- Do not let Godot/Python call raw protocol, engine, client-core, or storage ABIs.
- Do not maintain Go and Rust as concurrent online authorities or use a shadow writer.
- Do not move data one cell at a time across FFI boundaries.
- Do not make the standalone Agent service a library embedded in the game client.
- Do not interpret a successful Godot pilot as permission to skip the Rust server/client-core convergence stages.

When a new task conflicts with this document, the task must either be redesigned to fit the target or explicitly record an approved, time-bounded transition exception in its OpenSpec change.

## 10. Rust foundation stages, later features, and No-Go rollback

P7 decided Go for the remote-TCP pilot only. That decision does not authorize a default-client switch or new Go real-time ownership. Later work is blocked on independently proposed Rust foundation stages:

| Stage | Owner | Prerequisite | Exit condition | Rollback |
|---|---|---|---|---|
| [F1](../openspec/changes/archive/2026-09-21-rust-runtime-foundation/proposal.md) | Rust domain, protocol, storage contracts, and numerical kernels | P7 Go | Replay/oracle agreement with Go; no second online writer | Keep the Go production path |
| [F2](../openspec/changes/rust-authoritative-server/proposal.md) | Rust authoritative server | [F1](../openspec/changes/archive/2026-09-21-rust-runtime-foundation/proposal.md) | Deterministic replay, save migration, failure-path parity, shared Memory/TCP semantics | Keep Go authority; never dual-write |
| [F3](../openspec/changes/rust-client-core/proposal.md) | Rust client-core and typed Godot bridge | F1; F2 protocol | Transcript parity, correction/replay, bounded bridge, repeated lifecycle | Keep the pilot Go core without adding features |
| [P8](../openspec/changes/godot-production-terrain/proposal.md) | Production terrain presentation | [F3](../openspec/changes/rust-client-core/proposal.md) | Independent world feature against Rust semantic families | Disable the catalog entry; keep the old client default |
| [P9](../openspec/changes/godot-complete-actors/proposal.md) | Complete entities and effects | [F3](../openspec/changes/rust-client-core/proposal.md) | Independently disableable actor features | Disable by catalog |
| [P10](../openspec/changes/godot-ui-migration/proposal.md) | UI migration | [F3](../openspec/changes/rust-client-core/proposal.md) | Godot Control plus embedded Python; no production GDScript or new WebView ownership | Keep the old UI client |
| [P11](../openspec/changes/godot-desktop-audio/proposal.md) | Audio and desktop devices | [F3](../openspec/changes/rust-client-core/proposal.md) | Semantic cues; headless touches no device | Disable the adapter |
| [P12](../openspec/changes/godot-production-tooling/proposal.md) | Tooling | F1–F3 as applicable | Offline replay and presentation tests; no dual online authority | Continue the old toolchain |
| [P13](../openspec/changes/godot-desktop-packaging/proposal.md) | Local play and desktop release | F2–F3 | Local/remote share one Rust path; desktop-only closure | Return to remote-only or the old client |
| [P14](../openspec/changes/godot-default-client-switch/proposal.md) | Default switch and retirement | F1–P13 complete | Two release cycles and a usable rollback package | Restore the previous release; prohibit partial deletion |

These links identify active planning changes, not completed implementations. F1 freezes and validates shared contracts; F2 establishes Rust authority; F3 consumes the accepted F2 protocol/session contract and must pass Rust-server integration before its own acceptance. Every prerequisite needs implementation-SHA, corpus coverage, non-empty executed tests, failure-path and rollback evidence in its ledger. OpenSpec artifact status and text searches cannot prove completion.

P12 first supplies the capture/identity/coverage infrastructure needed by P8–P11, then accepts individual producer handoffs after their feature evidence exists. This ordering avoids a tooling/feature dependency cycle. A handoff changes only named semantic cases; remaining cases keep their old producer and regression checks. P13 qualifies exported desktop evidence; P14 consumes two complete release cycles and the rollback package before switching defaults or retiring transition components. The stable project root survives Bootstrap retirement; qualified native diagnostics must replace the migration Bootstrap first.

No-Go rollback for the current pilot remains additive: removing `apps/mornlea-godot/`, both GDExtensions, the bundled Python runtime, the Go client-core ABI, and optional `scripts/godot` entry points restores pre-pilot production behavior. Pilot failure must not rewrite saves or default configuration. After a P7 Go, later features stay independently reversible without deleting the stable project root. Rust migration uses offline replay rather than a dual online writer; it never runs two online authorities.
