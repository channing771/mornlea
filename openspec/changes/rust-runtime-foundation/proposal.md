## Why

The target architecture names Rust as the owner of shared runtime contracts, but the active roadmap previously started with Godot features and had no foundation change. Establish a compatibility substrate before moving either online authority or client state.

## Status and prerequisites

Planning only: F1 in [`docs/architecture-target.md`](../../../docs/architecture-target.md). The archived pilot decision permits foundation planning; implementation starts with the contract inventory. Completion requires the implementation SHA, executed non-empty tests, corpus coverage, failure-path results and rollback evidence in `ledger.md`; file existence or OpenSpec status is insufficient.

## What Changes

- Freeze language-neutral wire/save/input/event/replay identities against the verified Go baseline and record a complete capability inventory.
- Add Rust domain, protocol and storage contract crates; reuse the existing `mornlea_engine` numerical implementation through Rust library APIs.
- Build offline golden, malformed-input and differential replay evidence. Keep all production startup and server authority unchanged.
- Publish explicit completion evidence consumed by F2 and F3; a planning artifact is not a completed prerequisite.

## Capabilities

### New Capabilities

- `rust-runtime-foundation`: Versioned shared contracts and offline compatibility evidence for the Rust runtime.

### Modified Capabilities

None. Existing wire, save and gameplay requirements remain the compatibility oracle; any discovered need to change observable behavior requires a scoped delta before implementation.

## Impact

- Affected areas: `packages/engine/` workspace and scoped guides; `packages/shared/network/`, `packages/server/storage/`, `packages/contracts/` as current contract sources; offline oracle tooling and `testdata/runtime-migration/`. No game presentation or tracked visual changes.
- Compatibility: preserve current protocol/save versions and semantics. Inventory every version from verified code at implementation start. No version bump or silent conversion is authorized by this plan.
- Concurrency/performance: bounded queues, batches and tick work; no blocking I/O on hot paths. Report performance measurements; overflow, data loss, identity gaps and I/O failures remain hard failures.
- User outcome: the target Rust owner becomes independently testable with equivalent behavior; default startup remains the current Go application plus Rust renderer until P14.
- Non-goals: new gameplay, a default-client switch, production GDScript, Python authority, or simultaneous Go/Rust online writers.
- Rollback: retain the previous runtime and its compatible data; select one authority and never rely on a shadow writer or implicit save downgrade.

## Deferred and abandoned

Production Godot features and visual handoffs remain in P8–P12; distribution and default retirement remain P13–P14. No runtime implementation is claimed by this planning synchronization.
