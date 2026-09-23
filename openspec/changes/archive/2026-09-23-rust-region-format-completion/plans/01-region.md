# Rust region implementation packet

This packet supplies execution detail for [tasks.md](../tasks.md); only `tasks.md` records status. Read the [proposal](../proposal.md), [delta specification](../specs/rust-runtime-foundation/spec.md), [design](../design.md), root `AGENTS.md`, `packages/engine/AGENTS.md` and `packages/engine/crates/mornlea_storage/AGENTS.md` before implementation. Baseline is `0f5ff747`; recheck HEAD and the protocol task's owned paths before editing. All commands run from the repository root in an isolated worktree. Workers report contract drift to the controller before choosing a new policy.

## Shared contract and exclusion

The producer is Rust `mornlea_storage::RegionBank`, aliased from `region::Bank`. Consumers are its existing `runtime_contract` suite and later Rust server code. `RegionKey { dimension: i32, x: i32, z: i32 }` retains raw signed format bits; `Entry { offset_sector:u32, sector_count:u32, payload_length:u32, revision:u64, payload_crc32c:u32 }` is unchanged. `StorageResult<T> = Result<T, StorageError>` remains the return convention. The Go `packages/server/storage/region/region_format.go` and test are read-only format authorities. This change neither updates `testdata/runtime-migration/contracts.json` nor claims complete `save.region` corpus/F1 acceptance. The two Rust nodes own separate behavior but share `region.rs` serially; no concurrent editing within this change.

`RegionBank` is the only owner of bank cardinality. It contains `pub generation: u64` and `pub entries: Box<[Entry; REGION_SLOTS]>`, where `REGION_SLOTS == 1024`. The constructor signature is `RegionBank::try_from_entries(generation: u64, entries: Vec<RegionEntry>) -> StorageResult<RegionBank>`. `RegionBank::empty() -> RegionBank`, `encode_region_bank(key, &bank) -> StorageResult<[u8; BANK_SIZE]>`, `decode_region_bank(key, bytes, file_size) -> StorageResult<RegionBank>` and `select_region_bank(Result<Bank,StorageError>, Result<Bank,StorageError>) -> StorageResult<(Bank,usize)>` remain public. Node 1.2 exports `encode_superblock_into(RegionKey, &mut [u8]) -> StorageResult<usize>` and `encode_region_bank_into(RegionKey, &RegionBank, &mut [u8]) -> StorageResult<usize>`. The former returns 4096, the latter 28672. `StorageError::OutputTooSmall { needed: usize, available: usize }` is the capacity failure; `Corrupt` remains for format/invalid-bank errors and `FutureVersion` for version >1.

The entire editable implementation set is `packages/engine/crates/mornlea_storage/src/{region,error,lib}.rs`, only region tests in `packages/engine/crates/mornlea_storage/tests/runtime_contract.rs`, and `packages/engine/crates/mornlea_storage/AGENTS.md`. This change directory is controller-owned for status and evidence. Read-only integration consumers: the rest of `mornlea_storage`, `packages/audit/identity_test.go`, the existing Go region tests, and `packages/engine/crates/mornlea_domain/`. Do not edit the protocol task's crates, `packages/tools/cmd/runtime-oracle/`, shared corpus, workspace manifest, lockfile, other storage families, live saves, or generated artifacts. Before dispatch and review, search `rg -n 'RegionBank|encode_region_bank|encode_superblock' packages/engine` and `rg -n 'mornlea_storage/src/(region|lib|error)\.rs|RegionBank' packages/audit scripts testdata` for newly added consumers; escalate a changed ownership set. No known derived artifact hashes or embeds the editable files.

<a id="node-1-1"></a>

## 1.1 — Fixed-shape bank

**Deliverable and predecessor:** from accepted baseline `0f5ff747`, a bank whose cardinality cannot change after construction, with unchanged Go v1 layout. No implementation predecessor. The independent review may reject this node without affecting protocol work.

**Test first:** in the existing region section of `tests/runtime_contract.rs`, add `region_bank_rejects_wrong_slot_count` with two vectors of 1,023 and 1,025 default entries. Call `RegionBank::try_from_entries(1, entries)` and assert `matches!(result, Err(StorageError::Corrupt(_)))`. Add `region_bank_constructor_preserves_valid_shape`: create 1,024 default entries, set slot 0 to `{offset_sector:15,sector_count:1,payload_length:0,revision:1,payload_crc32c:0}`, construct generation 1, assert `entries.len()==1024`, encode, then decode with file size 65,536 and compare the entire bank. Before adding the constructor, reproduce the behavioral baseline with a temporary test that truncates `RegionBank::empty().entries` to 1,023 and asserts `encode_region_bank` fails; it currently returns `Ok` (silent zero fill). Record that red result, then replace the temporary test with the final constructor table. The final test initially fails to compile because the constructor is absent; the earlier red establishes the actual unsafe behavior.

The final wrong-size table is:

```rust
for count in [REGION_SLOTS - 1, REGION_SLOTS + 1] {
    let entries = vec![RegionEntry::default(); count];
    assert!(matches!(
        RegionBank::try_from_entries(1, entries),
        Err(StorageError::Corrupt(_))
    ));
}
```

**Implementation:** preserve public element indexing. Create `Bank::empty` from `vec![Entry::default(); REGION_SLOTS].into_boxed_slice().try_into()` after the known-length invariant, so no `[Entry;1024]` stack value is built. In `try_from_entries`, compare `len` to 1024 before scanning; return `Corrupt` for either wrong length; convert ownership to `Box<[Entry;1024]>` without an element clone; run `validate_region_bank(&bank, 0, false)` before returning. In `decode_region_bank`, start from `Bank::empty()`, set its generation and fill 1,024 indexed entries, then validate with `file_size`. Keep the existing checksum, entry and selection algorithms; `validate_region_bank` now iterates an always-fixed array and sorts at most 1,024 extent pairs. Do not change the Go codec, disk bytes, versions or other storage data types.

The constructor's non-obvious conversion is:

```rust
if entries.len() != REGION_SLOTS {
    return Err(corrupt("region bank entries", format!("{} entries, want {REGION_SLOTS}", entries.len())));
}
let entries: Box<[Entry; REGION_SLOTS]> = entries
    .into_boxed_slice()
    .try_into()
    .map_err(|_| corrupt("region bank entries", "invalid fixed length"))?;
let bank = Self { generation, entries };
validate_region_bank(&bank, 0, false)?;
Ok(bank)
```

**Expected red/green:** red is the demonstrated 1,023-entry silent acceptance and then the missing constructor. Green is the two new tests, every existing `region_*` case and Go region test. Discover cases with `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_storage --test runtime_contract --locked -- --list`; the filtered region count must be nonzero. Run the focused commands in task 1.1. The controller records source SHA, red/green output, review finding and `Architecture skill: no change` or a verified promotion decision in `ledger.md`, checks task 1.1 and creates a scoped `fix(storage): enforce fixed region bank cardinality` commit. Roll back only this node's Rust storage edits if its gate fails.

<a id="node-1-2"></a>

## 1.2 — Atomic region output

**Deliverable and predecessor:** follows accepted/committed node 1.1; exact public caller-buffer output without partial writes. Node 1.1's `Box<[Entry;1024]>` and existing v1 layout are prerequisites.

**Test first:** extend the same region section with `region_bank_into_is_atomic`, `region_superblock_into_preserves_tail`, and `region_go_crc_reference`. Use the valid key `{dimension:-3,x:-1,z:2}` and the node 1.1 bank. For a `vec![0xA5; BANK_SIZE-1]`, assert `encode_region_bank_into` yields exactly `OutputTooSmall {needed:BANK_SIZE,available:BANK_SIZE-1}` and the vector is unchanged. Mutate slot 0 to `offset_sector=14`, repeat with the same short buffer, assert `Corrupt` wins and bytes remain unchanged. For `BANK_SIZE+7` canary bytes, assert written size `BANK_SIZE`, encoded prefix equals `encode_region_bank(key,&bank)`, and seven tail bytes remain `0xA5`; repeat for the 4096-byte superblock, including a 4095-byte short buffer. Assert little-endian bytes at superblock offsets 12/16/20 reflect -3/-1/2, CRC at 4092 is `0xef554c52`, and bank CRC at 60 is `0x24d271a5`. Those constants were produced from the current Go `region.EncodeSuperblock` and `region.EncodeRegionBank` for this exact input. The test initially fails with unresolved new exports; record the red command. Run the existing selection and corrupt-record table unchanged for extent, padding, future-version and divergent tie coverage.

The atomic error-order assertion uses a separate invalid copy so the valid prefix case still runs:

```rust
let mut short = vec![0xA5; BANK_SIZE - 1];
assert_eq!(
    encode_region_bank_into(key, &good, &mut short),
    Err(StorageError::OutputTooSmall { needed: BANK_SIZE, available: BANK_SIZE - 1 })
);
assert!(short.iter().all(|byte| *byte == 0xA5));
let mut invalid = good.clone();
invalid.entries[0].offset_sector = DATA_START_SECTOR - 1;
assert!(matches!(encode_region_bank_into(key, &invalid, &mut short), Err(StorageError::Corrupt(_))));
assert!(short.iter().all(|byte| *byte == 0xA5));
```

**Implementation:** add the `OutputTooSmall` enum variant and `Display` arm in `src/error.rs`. In `src/region.rs`, add `encode_superblock_into` and `encode_region_bank_into` with the exact signatures above. Bank order is `validate_region_bank` → destination length check → `dst[..BANK_SIZE].fill(0)` → fixed-field/entry writes → CRC32C after checksum field is zero; no fallible operation occurs after the first write. Superblock order is length check → zero only `dst[..4096]` → fields → checksum. Make existing array-returning functions call these new functions on their own fixed arrays, preserving signatures and bytes. For the fallible bank wrapper, propagate validation failures; the known-size output makes capacity failure impossible. Export both functions from `src/lib.rs`. Keep all source comments in English and explain validation-before-publication intent at the new boundary.

The direct bank path starts with complete preflight and then passes the exact prefix to the existing fixed-offset field writer:

```rust
validate_region_bank(bank, 0, false)?;
if dst.len() < BANK_SIZE {
    return Err(StorageError::OutputTooSmall { needed: BANK_SIZE, available: dst.len() });
}
let encoded = &mut dst[..BANK_SIZE];
encoded.fill(0);
```

Move the current header and entry writes from `encode_region_bank` into that path without changing their offsets. After those writes, call `put_u32(encoded, BANK_CRC_OFFSET, region_bank_checksum(encoded))` and return `Ok(BANK_SIZE)`. Because `encoded.fill(0)` precedes the checksum, the checksum field and every reserved/padding byte have their required zero value during the CRC calculation.

**Expected red/green:** the concrete caller-buffer tests fail to compile on the baseline because the boundary is absent; the canary, precedence, Go CRC and existing format tests pass after implementation. The 28,672-byte direct API must not allocate a second bank-sized temporary. Run the focused commands in task 1.2 and discover the new test names with `-- --list`. The controller reviews exact output, buffer tail, error order, dependency direction and Go CRC evidence, records source/result SHA in `ledger.md`, checks task 1.2 and creates `feat(storage): add atomic region buffer encoding`. Rollback reverts this node's error/export/encoder changes but leaves the committed fixed-shape node intact if independent acceptance remains valid.

<a id="node-2-1"></a>

## 2.1 — Guidance and stage acceptance

**Deliverable and predecessor:** follows accepted nodes 1.1 and 1.2; records whole-change compatibility and stage evidence. Update only the region and focused-verification paragraphs in `packages/engine/crates/mornlea_storage/AGENTS.md` to describe the fixed representation and caller-buffer contract. Confirm source search still finds no protocol-owned or derived-consumer overlap; if it does, stop and reconcile the task packet before editing that new area. Discover the Rust test target and capture the nonzero region case list. Run exact task 2.1 gate commands, plus `go test ./packages/server/storage/region -count=1` and `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_storage --test runtime_contract --locked`. Record each result, SHA, reviewer ruling, format/version unchanged and rollback feasibility in `ledger.md`. Failed required gates keep 2.1 open; no exemption or assertion of an inherited failure substitutes for a passed gate. After acceptance, the controller checks 2.1 and creates `docs(storage): record region format acceptance` with only its owned guide and OpenSpec evidence. Do not sync canonical specs or archive until this change's accepted implementation and strict validation match the delta.
