## Why

The reviewed Rust runtime baseline deliberately stopped before hostile, passive,
projectile, item-drop, and chat publications, leaving the shared domain event
surface incomplete. Complete that semantic boundary before protocol/storage
acceptance, native numerical APIs, or the Rust authoritative server proceeds,
so later owners consume validated values instead of wire DTOs or ad hoc copies.

## What Changes

- Add checked Rust domain records for hostile and passive mobs, projectiles,
  item drops, and the closed chat outcome union while preserving the current Go
  authority's observable fields, validation precedence, and ordering.
- Replace the production digest-only replay observation with an exhaustive
  semantic event enum and a separate recipient envelope; transport lifecycle
  packets and runtime worker messages remain outside the domain event surface.
- Extend the frozen runtime-migration corpus with independently executed Go
  producers and Rust consumers for the new event families, including mutation
  cases that fail when one semantic field, variant, or record order drifts.
- Apply the shared 4096-record semantic work cap before proportional scans or
  copies while retaining each protocol packet's smaller wire limit separately.
- Keep `rust-authoritative-server` blocked on complete F1 evidence. This change
  closes only the domain-event successor; protocol/save corpus completion,
  safe native numerical APIs, pathfinding, and final F1 acceptance remain
  separate successor changes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `rust-runtime-foundation`: Complete the shared semantic event model and its
  independently executed Go/Rust evidence without expanding online authority.

## Impact

- Affected areas: `packages/engine/crates/mornlea_domain/`, test-only Go
  producers in `packages/tools/cmd/runtime-oracle/`, and reviewed additions
  under `testdata/runtime-migration/`. Existing Go protocol DTOs are read-only
  compatibility sources.
- Architecture: Rust domain remains dependency-free and windowless. Protocol,
  storage, server, client, Godot, Python, GPU, and online authority ownership do
  not move in this change.
- Compatibility: protocol remains v45; player schema v9, chunk schema v9,
  world metadata v6, standalone entity schemas, engine ABI v11, client ABI v19,
  and benchmark scenario v23 remain unchanged. No save or wire conversion is
  introduced.
- Concurrency and performance: no runtime thread or queue is added. Public
  variable-size domain batches reject more than 4096 records before
  proportional work; packet-specific limits remain protocol-owned.
- User-visible outcome: none. The current Go runtime and all default startup
  paths remain unchanged.
- Rollback: remove the new event modules, exports, corpus cases, and test-only
  producers together; the accepted baseline domain values and current Go
  production path remain intact.
