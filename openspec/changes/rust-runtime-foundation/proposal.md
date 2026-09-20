## Why

The target architecture names Rust as the owner of shared runtime contracts, but the active roadmap previously started with Godot features and had no foundation change. Establish a compatibility substrate before moving either online authority or client state.

## Status and prerequisites

F1 in [`docs/architecture-target.md`](../../../docs/architecture-target.md) is implemented in part and is **not accepted**. The review of `60c476645ee6dae1f6392336a7f3c593d2163ae3` found incompatible decoding, invalid encoding, missing compiled coverage, and an oracle that hashes files without executing behavior. See [review.md](review.md). Crate registration remains complete; inventory evidence, replay, domain ownership, protocol and storage acceptance are reopened in `tasks.md`. This planning revision does not implement repairs or start the kernel stage.

Completion requires the implementation SHA, executed non-empty behavioral tests, corpus coverage, failure-path results and rollback evidence in `ledger.md`; file existence, passing self-round-trips or OpenSpec status is insufficient. The controller owns architecture, compatibility rulings and task admission. Workers implement the complete linked contracts in `design.md`, `execution-contract.md` and `plans/01-evidence.md` through `plans/06-acceptance.md`; they do not choose shared ownership, compatibility policy or acceptance criteria.

## What Changes

- Freeze language-neutral wire/save/input/event/replay identities against the verified Go baseline and record a complete capability inventory.
- Add Rust domain, protocol and storage contract crates; reuse the existing `mornlea_engine` numerical implementation through Rust library APIs.
- Build offline golden, malformed-input and differential replay evidence. Keep all production startup and server authority unchanged.
- Publish explicit completion evidence consumed by F2 and F3; a planning artifact is not a completed prerequisite.
- Use controller-owned Superpowers design and planning for every unstarted node:59 individual packet tasks, explicit domain/event fields, seven save families, ten native numerical APIs, pathfinding and acceptance dependencies.
- Correct the evidence model before accepting additional migrations: executable cases must identify the compiled Rust consumer and the Go-produced value or failure, including asymmetric login validation and historical save associations.

## Capabilities

### New Capabilities

- `rust-runtime-foundation`: Versioned shared contracts and offline compatibility evidence for the Rust runtime.

### Modified Capabilities

None. Existing wire, save and gameplay requirements remain the compatibility oracle; any discovered need to change observable behavior requires a scoped delta before implementation.

## Impact

- Affected areas: `packages/engine/` workspace and scoped guides; `packages/shared/network/`, `packages/server/storage/`, `packages/contracts/` as current contract sources; offline oracle tooling and `testdata/runtime-migration/`. No game presentation or tracked visual changes.
- Compatibility: preserve current protocol/save versions and semantics, including the login driver's explicit rejection responses, raw metadata/armor fidelity and the kernel failure-publication corrections explicitly specified in plan05. Inventory every version from verified code at implementation start. The existing user ruling permits different zstd-compressed bytes for `save.chunk` and `protocol.server.ChunkSnapshot`; decoded values, logical payloads, integrity checks and cross-decoder compatibility remain exact. No wire/save version bump or silent conversion is authorized. The incomplete tooling trace format may be versioned without changing game formats.
- Concurrency/performance: bounded queues, batches and tick work; no blocking I/O on hot paths. Report performance measurements; overflow, data loss, identity gaps and I/O failures remain hard failures.
- User outcome: the target Rust owner becomes independently testable with equivalent behavior; default startup remains the current Go application plus Rust renderer until P14.
- Non-goals: new gameplay, a default-client switch, production GDScript, Python authority, or simultaneous Go/Rust online writers.
- Rollback: retain the previous runtime and its compatible data; select one authority and never rely on a shadow writer or implicit save downgrade.

## Deferred and abandoned

Production Godot features and visual handoffs remain in P8–P12; distribution and default retirement remain P13–P14. Full authoritative gameplay replay belongs to the consuming runtime stages; F1 must execute its codecs, semantic transformations and numerical kernels, and cannot claim gameplay parity from fixture hashes. No runtime fix is claimed by this planning revision.
