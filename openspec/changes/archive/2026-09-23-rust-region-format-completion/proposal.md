## Why

The Rust region codec currently represents a fixed 1,024-slot bank with an unrestricted `Vec`. Its encoder can silently zero-fill a short bank or index beyond the 28,672-byte output for a long bank. The future Rust server needs a fixed-shape, fallible region format boundary before it can own persistence.

## What Changes

- Make the Rust region bank's slot cardinality fixed by construction and preserve the existing v1 bank and superblock bytes and selection rules.
- Add caller-buffer region encoders that validate before writing and leave the destination unchanged on invalid input or insufficient capacity.
- Qualify valid and corrupt region layouts against the existing Go codec tests with focused Rust cases, including extent overflow, overlap, reserved bytes and bank selection.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `rust-runtime-foundation`: Require a bounded, atomic Rust region-format boundary with fixed cardinality and preserved Go v1 compatibility.

## Impact

- **Affected code:** `packages/engine/crates/mornlea_storage/src/{region,error,lib}.rs`, the region section of its existing Rust contract test and the crate's scoped guide. The Go `packages/server/storage/region` codec and tests are read-only compatibility sources.
- **Compatibility:** region format remains v1, with the same 4,096-byte superblock, two 28,672-byte banks, CRC32C and bank-selection behavior. All save schemas, protocol v45, engine ABI v11, client ABI v19 and benchmark scenario v23 remain unchanged. No save migration or live-world write occurs.
- **Concurrency and performance:** no online worker or blocking hot-path operation is introduced. Encoding scans at most 1,024 slots, uses bounded scratch and writes only after validation and capacity checks.
- **User-visible outcome:** current Go gameplay and startup remain unchanged. This is an independently verifiable Rust storage prerequisite for the future single Rust authority.
- **Non-goals and rollback:** this change does not complete the other six save families or F1 acceptance. Revert the Rust region code/tests as one scoped unit if its format qualification fails; no live data rollback is needed.
