## Context

See [proposal.md](proposal.md) for the failure and [the delta specification](specs/rust-runtime-foundation/spec.md) for the required behavior. Go `packages/server/storage/region` remains the format authority during this offline migration. Its `Bank.Entries` is a `[1024]Entry`; Rust `mornlea_storage::RegionBank.entries` is currently a public `Vec<Entry>`. The current Rust encoder iterates that vector without a cardinality check, while its output is fixed at 28,672 bytes. Existing Rust `runtime_contract` and Go `region_format_test.go` cover much of the valid layout and corrupt-input surface; this change adds the missing safe-caller and caller-buffer boundaries.

## Goals / Non-Goals

**Goals:** make invalid slot cardinality unrepresentable after construction; preserve v1 bytes and bank selection; expose bounded, atomic caller-buffer output; retain focused Go/Rust compatibility evidence.

**Non-Goals:** disk I/O, durability scheduling, automatic repair, a new storage format, other save families, a live Rust server, or full F1 corpus acceptance. No protocol code or shared runtime corpus file is edited.

## Decisions

### Fixed shape and construction

`RegionBank` remains the public alias of `region::Bank`. Replace `pub entries: Vec<Entry>` with `pub entries: Box<[Entry; REGION_SLOTS]>`; keep `pub generation: u64` and element indexing for existing callers. `Bank::empty()` creates 1,024 default entries on the heap, then converts a boxed slice to the fixed boxed array, avoiding a 24 KiB stack array. Add `Bank::try_from_entries(generation: u64, entries: Vec<Entry>) -> StorageResult<Self>`: check `entries.len() == REGION_SLOTS` before scanning or conversion; reject any other size as `StorageError::Corrupt`, convert the owned vector without copying its entries, then validate canonical entry rules. This constructor is an optional caller convenience; direct field mutation remains possible and every encoder revalidates it.

The region validator scans exactly 1,024 entries. It preserves the existing rules: an absent entry is all zero; an occupied entry requires nonzero generation, sector count and revision, starts at or after sector 15, has `payload_length <= 1,048,576` and within its allocated extent, and has a checked end sector no greater than `u32::MAX`. Sort at most 1,024 `(start,end)` ranges and reject overlap. Decode additionally checks `file_size >= 61,440` bytes and each end sector against `file_size / 4,096`. A zero-generation all-empty bank remains encodable and decodable as standby but is not selectable as committed.

**Rejected:** a length-checked public `Vec`, because callers could mutate its cardinality after validation; `[Entry; 1024]` by value, because cloning or returning a bank would create a large stack frame; unchecked array conversion, because a wrong length must be a typed failure.

### Atomic output and error order

Add `encode_superblock_into(key: RegionKey, dst: &mut [u8]) -> StorageResult<usize>` and `encode_region_bank_into(key: RegionKey, bank: &Bank, dst: &mut [u8]) -> StorageResult<usize>`, exported from `src/lib.rs`. Add `StorageError::OutputTooSmall { needed: usize, available: usize }` and its English display text in `src/error.rs`. The bank encoder validates before checking `dst.len()`, reports `OutputTooSmall` only for a valid bank, then writes only `dst[..BANK_SIZE]`. The superblock encoder checks capacity before writing. Fixed offsets, complete preflight and infallible slice writes after preflight ensure no later error can leave partial output. Existing owned-array encoders become wrappers that allocate their fixed output and call the new operations; they retain their signatures and byte outputs. The wrapper's known-size destination makes `OutputTooSmall` unreachable.

For the bank, write the magic, fields, entries, reserved bytes and padding to `dst[..BANK_SIZE]` with explicit zero fill, then compute CRC32C with the checksum field zeroed. The direct caller-buffer path uses no bank-sized temporary allocation. A destination longer than the format length keeps its suffix untouched. No mutable global codec state or cross-thread publication is added.

**Rejected:** encode into a temporary `Vec` and copy at the end, because it defeats the caller-buffer and allocation boundary; use `Corrupt` for short output, because it conflates caller capacity with disk corruption; report short capacity before bank validation, because it hides invalid records.

### Evidence and file ownership

Extend the region section of the existing `tests/runtime_contract.rs` rather than opening a parallel integration-test mirror. Test a 1,023/1,025-entry constructor rejection, a malformed occupied entry, the 28,671-byte canary case, exact-length and oversized destinations, negative keys, fixed geometry, CRC, file-size and selection boundaries. For key `{dimension:-3,x:-1,z:2}` and a generation-1 bank with slot 0 at sector 15, one sector, payload length 0 and revision 1, the existing Go encoder yields superblock CRC32C `0xef554c52` and bank CRC32C `0x24d271a5`; pin these independent values in Rust. The Go comparison uses the existing `packages/server/storage/region/region_format_test.go` tests; no generated corpus or shared manifest changes. No new crate dependency or `Cargo.lock` change is needed.

The editable source set is `packages/engine/crates/mornlea_storage/src/{region,error,lib}.rs`, the region section of `packages/engine/crates/mornlea_storage/tests/runtime_contract.rs`, this change directory and the existing crate `AGENTS.md` for focused-validation guidance. `packages/server/storage/region/`, `mornlea_protocol/`, `mornlea_domain/`, `packages/tools/cmd/runtime-oracle/`, `testdata/runtime-migration/`, `packages/engine/Cargo.toml` and `Cargo.lock` are read-only. `src/lib.rs` exports are consumed by the Rust contract test and future Rust server; no generated or embedded artifact is derived from the editable files. Other Rust storage families and their tests are regression consumers through `cargo test -p mornlea_storage`.

## Risks / Trade-offs

- **Public Rust field type changes** → compile the whole storage crate and search workspace consumers before the node closes. Current source search found no external bank literal or direct type annotation beyond the crate's integration tests.
- **Bank validation or CRC drift** → compare fixed fields and checksum against the existing Go layout tests, run all Rust storage contracts and keep the Go codec unchanged.
- **Parallel checkout contention** → run implementation in an isolated worktree. The protocol task owns protocol/domain/oracle/corpus files; this task owns only storage paths and its OpenSpec change. Any shared gate or integration conflict returns to the controller for serial resolution.

## Migration Plan

This is an offline Rust API and validation change. No save files are rewritten and no runtime selection changes. Implement fixed shape first, then caller-buffer encoding and focused evidence. If qualification fails, revert the scoped storage change without touching live worlds; F2 and final F1 acceptance remain blocked until their other prerequisites complete.
