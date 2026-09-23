## Why

The Rust foundation has checked semantic values and individual packet codecs, but it cannot yet serve as the shared protocol owner for a Rust server and client core. The v45 inventory has 59 packet families with no executed packet corpus cases; Rust also lacks a closed direction/state dispatcher, a distinct inbound admission boundary, and a fallible caller-buffer encoding path. Finish this one protocol compatibility surface before F2 or F3 consumes it.

## What Changes

- Complete the Rust v45 frame, packet, negotiation and typed dispatch surface over the accepted Rust domain values. Raw wire representation stays in `mornlea_protocol`; semantic commands and events stay in `mornlea_domain`.
- Produce independently executed, source-bound Go codec cases and Rust consumers for every frozen packet key, including malformed input, validation, encoding atomicity and the logical equivalence exception for compressed snapshots.
- Separate structural handshake/login decoding from version rejection and identity admission, preserving the Go login decision order without moving session lifecycle into the protocol crate.
- Close the gap between individual packet modules and one usable Rust client/server API, with bounded work, explicit errors and no partially published output.
- Keep F2 blocked until this protocol successor, the other F1 successors and final zero-gap F1 acceptance have all passed.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `rust-runtime-foundation`: Complete the Rust protocol contract and its independent offline compatibility evidence.

## Impact

- **Affected areas:** `packages/engine/crates/mornlea_protocol/`, a narrow shared text helper in `mornlea_domain`, test-only `packages/tools/cmd/runtime-oracle/`, reviewed `testdata/runtime-migration/` assets, and the relevant scoped `AGENTS.md` guides. Go protocol/codec and login behavior remain compatibility sources; production Go paths are not migrated here.
- **Compatibility:** protocol stays v45, including all existing packet IDs, state/direction keys, wire layouts and rejection behavior. Player/chunk schema v9, world metadata v6, `companions.ai` v5, hostile v2, passive v1, engine ABI v11, client ABI v19 and benchmark scenario v23 do not change. Historical save migration is outside this change.
- **Concurrency and performance:** no online authority, transport thread, queue or blocking hot-path work is added. The future Rust server and client receive bounded, fallible codec APIs; packet and corpus budgets are checked before proportional allocation or decompression. Performance measurements remain informational; overflow, malformed input, lost output and I/O failures remain hard failures.
- **User-visible outcome:** current Go startup and gameplay remain unchanged. This is an offline-qualified prerequisite for the single Rust authority and Rust client core.
- **Rollback:** revert this Rust protocol surface and its matching test-only corpus additions as one scoped change. No live world, save, default entry point or dual online writer is touched.
