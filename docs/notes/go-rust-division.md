---
doc_id: go-rust-ownership
doc_revision: 2026-09-19.1
language: en
counterpart: go-rust-division.zh.md
---
# Mornlea language ownership and migration policy

This document is a compact routing guide for new work. The final architecture is defined by [`docs/architecture-target.md`](../architecture-target.md). [`docs/architecture.md`](../architecture.md) describes the current implementation. Go ownership listed below is a transition exception unless the target document explicitly assigns it to Go.

## 1. Final ownership in one table

| Concern | Final owner | Notes |
|---|---|---|
| Authoritative gameplay rules and tick | Rust server/domain | Rust owns the state transition and the only world write path. |
| Protocol and network session | Rust protocol/server/client-core | The wire contract and its replay identity are one Rust-owned boundary. |
| Persistence and schema migration | Rust storage/server | Clients and Godot hosts never write authoritative saves. |
| Physics, collision, raycast, worldgen, fluids | Rust kernel | One deterministic production implementation; no Go or Python fallback. |
| Mesh, light, voxel and bulk transforms | Rust kernel/client-core | Prepare bulk data on the Rust side and cross the bridge in batches. |
| Client mirror, prediction, reconciliation | Rust client-core | Godot receives semantic snapshots rather than protocol records. |
| Window, scene, UI, input, audio, animation | Godot with embedded Python | Python is the final Godot feature language; work remains bounded and presentation-only. |
| Companion planning, dialogue, memory, AI tools | Independent Python service | Candidates cross a versioned service contract and are revalidated by Rust server. |
| Migration oracle and conversion tools | Go temporarily | Go compares recorded transcripts and helps migrate data; it is not a final real-time dependency. |

## 2. Current-to-target transition

| Area | Current production or pilot | Target direction | Rule for new work |
|---|---|---|---|
| Server | Go authoritative server | Rust authoritative server | Do not add new Go authority; add a Rust contract or a time-bounded adapter. |
| Godot client core | Go runtime behind a Rust/Godot adapter | Rust client-core linked to the typed Godot bridge | Python features may consume semantic views, but must not grow Go wire/data logic. |
| Godot scripting | Embedded Python pilot plus pure-GDScript bootstrap | Embedded Python feature host | Do not add production GDScript; bootstrap is migration-only. |
| Numerical engine | Rust engine called from Go | Rust domain/kernel consumed by Rust server and client-core | Do not create a second numerical implementation in Go, Python, or Godot. |
| Renderer | Rust `mornlea_client` | Godot presentation plus Rust preparation/bridge | Keep the old renderer as a baseline until an approved producer handoff. |
| AI | Separate Python Agent | Separate Python Agent | Never embed the Agent into the client or let it write world state. |

The embedded Godot Python runtime and the standalone Agent Python runtime are different products. They have separate processes or runtime closures, dependency locks, lifecycle owners, and contracts. Similar syntax does not justify shared imports or state.

## 3. Decision rules

1. Does the code decide an authoritative world outcome, persist it, or define a protocol/schema? Write it in the Rust server/domain/protocol/storage boundary.
2. Does it perform deterministic numerical work over many cells, vertices, samples, entities, or bytes? Write it in the Rust kernel or client-core.
3. Does it maintain a client mirror, prediction, correction, network session, or bounded semantic snapshot? Write it in Rust client-core.
4. Does it compose Godot scenes, controls, input actions, audio cues, animation, or resource lifecycles? Write it in embedded Godot Python through the typed bridge.
5. Does it plan, converse, summarize memory, or provide an AI tool outside the real-time loop? Write it in the independent Python Agent.
6. Is it only comparing old and new behavior, converting data, or operating a migration gate? Go is allowed temporarily, but the result must be replayable and cannot define a new final contract.
7. If a task matches more than one rule, place the state and computation together on the side that owns the larger bounded operation. Do not create a high-frequency FFI shuttle to preserve an old package boundary.

## 4. Non-negotiable boundaries

- There is one online authority. A Go/Rust dual writer or a real-time shadow authority is prohibited.
- Godot/Python consumes typed semantic values. It does not parse packets, load save files, call raw engine/client ABIs, or submit unvalidated world actions.
- Python callbacks are bounded and non-blocking. They do not perform tick-critical numerical loops, network waits, disk I/O, runtime installation, or unbounded scene scans.
- Rust server validation is required for human input, local input, and AI proposals alike.
- Local Memory and remote TCP reuse the same login, packet, validation, and server-core path.
- Cross-language buffers are null or valid owned buffers, and batches are failure-atomic. Per-cell calls and retained foreign pointers are prohibited.
- The current Go and Python pilot seams must be named in the relevant OpenSpec change and must have a removal or replacement condition.

## 5. Test placement

Rust owns authoritative rule, protocol, persistence, numerical, replay, property, fuzz, and client-core tests. Godot/Python owns typed-bridge, lifecycle, input, UI, audio, animation, and semantic visual tests. The standalone Agent owns HTTP/MCP, candidate validation, cancellation, and memory-isolation tests. Go tests remain valid migration evidence only when they compare recorded behavior or validate conversion/tooling; a Go-only gameplay test is not evidence that Go should remain the final owner.

Moving a behavior between language boundaries requires an OpenSpec change with a replay corpus, compatibility plan, failure-atomicity tests, and a rollback path. Performance measurements inform the decision but do not excuse a duplicated production implementation or an unbounded bridge.
