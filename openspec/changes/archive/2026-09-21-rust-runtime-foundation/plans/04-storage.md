# Storage compatibility and bounded encoding plan

**Goal:** preserve all supported save versions and repair data loss/panic/aggregate gaps before accepting Rust storage. **Architecture:** raw schema DTO → complete format validation → explicit migration → current value; gameplay admission is a separate later boundary. **Tech stack:** Rust std and existing zstd; Go pure codec producers. **Spec:** migration associations, complete aggregate validation, invalid publication and resource bounds. **Global constraints:** `../execution-contract.md` and all exact shapes/version/defaults in `../storage-contracts.md`. **Review focus:** legacy queues, fixed bank cardinality, container-block association, raw armor/weather fidelity and allocation before rejection.

`STORAGE` is `packages/engine/crates/mornlea_storage`; Go paths start `packages/server/storage/`; ORACLE/CORPUS follow01-evidence. Every node owns its named Rust source and a topic test under `STORAGE/tests/runtime_contract/`, preserving existing integration root. Controller integrates `lib.rs`, test registration and manifest fragments serially. No file-store, recovery, save scheduling or disk writer is implemented.

All seven read-version sets are frozen: chunk1..9,player1..9,companions1..5,hostile1..2,passive1,region1,metadata1..6. Current writers are9/9/5/2/1/1/6. A missing historical case fails acceptance. Do not generate old-schema expected results using a new Rust helper. Existing historical binary files remain read-only; private Go legacy builders are invoked only from package-local tests with temporary export.

## Common output and corpus contract

Noncompressed APIs retain existing fallible Vec-returning names and add `<family>_encoded_len(&Save)->StorageResult<usize>` and `encode_<family>_into(&Save,&mut[u8])->StorageResult<usize>`, with family tokens `player`, `companions`, `hostile_mobs`, `passive_mobs`, `world_metadata`. Existing Save/Metadata types remain the format DTOs specified in storage-contracts. Add `StorageError::OutputTooSmall {needed,available}`. Validation precedes checked length, destination check, and writes; error leaves destination/suffix unchanged. At most one bounded sorted-index allocation is permitted for canonicalizing entity collections; do not clone large task/summary strings to sort records. Full record validity is checked before index allocation. Decode validates count/minimum input bytes before allocating and returns only a completely validated/migrated result.

Chunk compression has its own context in4.10. Region has fixed sizes and a signature in4.12. Public low-level `encode_chunk_logical`/`encode_chunk_at_schema` are subject to the same invariants as full encode; they are not an escape from aggregate validation. Historical format outputs from diagnostic helpers must honor that schema's layout/table, and cannot be used by the current production writer to silently downgrade.

For each family add Go `TestStorageOracle<Family>` in ORACLE using the exact exported codec from storage-contracts, except metadata/private migration builders use package-local `_test.go` producers. Case IDs are the existing inventory family plus `/version/label`. Execute current encode, all historical decodes and applicable migrations; normalize all fields/flags, including record ownership. Binary-decode negatives include truncated header/body, trailing byte, unsupported version, oversized declared lengths and bad CRC; where testing semantic invalidity, reseal CRC in the Go test helper so the semantic path is actually reached. Current noncompressed Go bytes are exact; historical re-encode compares the Go current-migration result, not old bytes. Historical companion bootstrap is the explicit exception in4.11.

Each node runs `rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_storage --test runtime_contract --locked <topic>` and the listed Go command. Discover the same Rust filter with `-- --list`; nonzero tests required. Follow red→implementation→focused tests→controller evidence→scoped commit. Commit subject names that node's concrete repair; rollback includes API consumers in the same unit and never rewrites source fixtures.

<a id="node-4-1"></a>

## 4.1 — Legacy companion queue ownership

Dependencies:1.5. Own `src/companion.rs`, tests `companion_legacy.rs`, `ORACLE/storage_companion_legacy_test.go`, `CORPUS/cases/save.companion/{2,3,4}/`. Use Go companion.Decode on `companions-v2/v3/v4.bin` and record every body's actual UUID, each task command/steps/state/index/deadline/failure, FIFO order and summary bytes. No guessed constant expected IDs.

Regression asserts **each nonempty legacy queue ID equals its containing body's ID** and normalized queue content equals Go. Baseline Rust yields zero ID and drops/misassociates queue evidence. In the schema2/3/4 body loop, assign parsed queue.id from body.id immediately after queue parsing, before checking emptiness/retention; retain source_schema and migration flags. Validate membership after migration without synthesizing extra bodies/lifecycles. Include body without queue and multiple bodies with distinct work so a count-only assertion cannot pass. No v5 memory bootstrap here. Topic `companion_legacy`; Go `go test ./packages/tools/cmd/runtime-oracle -run '^TestStorageOracleCompanionLegacy$' -count=1`.

<a id="node-4-2"></a>

## 4.2 — Companion cardinality and membership

Dependencies:4.1. Own `src/companion.rs`, tests `companion_bounds.rs`, producer `storage_companion_bounds_test.go`. Validate body count<=64 before clones/sorts, lifecycle count equals bodies for v5, unique body/lifecycle IDs, lifecycle/body join, active<=4, queues<=active, one queue per active body, no orphan/inactive queue. Retain exact active/memory/tombstone alternatives from current Go canonicalV5Parts. Check command<=1024 bytes,plan<=5000 steps,FIFO<=16,summary<=2048 and file<=393904 before proportional work. Record count limits are independent of file byte limit.

Cases:64 inactive bodies with valid tombstones accepted,65 rejected;4/5 active;missing/extra lifecycle;duplicate body;orphan queue;inactive queue;17 FIFO entries;5001 plan steps;2049-byte summary. Oversized count rejection must precede sorting/copying: instrument test-only allocation counting around the rejecting call with data allocated beforehand. Topic `companion_bounds`; Go filter `^TestStorageOracleCompanionBounds$` under ORACLE. No ordinary byte-format or memory-service behavior change.

<a id="node-4-3"></a>

## 4.3 — Chunk aggregate validation on every encoder

Dependencies:1.5. Own `src/chunk.rs`, tests `chunk_aggregate.rs`, `ORACLE/storage_chunk_aggregate_test.go`. Factor the **existing decode-side** `validate_chunk` into the one aggregate validator invoked by full encode, logical encode, schema encode and decode. Validate exactly24 sections/32 drops/32 furnaces/16 chests, then individual values, then active container associations. Active index<98304, no two active furnaces at one block, no two active chests at one block, actual block kind matches furnace/chest. Cross-kind overlap necessarily fails actual block matching; never make it valid by checking each collection in isolation. Inactive slots retain the Go-required canonical empty shape and generations.

Cases: current regression active furnace on air; chest on air; duplicate same-kind index;98303 correct block accepted/98304 rejected; furnace at chest block; canonical inactive entry;mixed valid furnace/chest; last drop slot valid/invalid; oversized shape and palette rejected before compression. Test all four public encode layers so fixing only full encode cannot close this node. Preserve generation and inventory values. Topic `chunk_aggregate`; Go `go test ./packages/tools/cmd/runtime-oracle -run '^TestStorageOracleChunkAggregate$' -count=1`.

<a id="node-4-4"></a>

## 4.4 — Fixed region-bank representation

Dependencies:1.5. Own `src/region.rs`, tests `region_shape.rs`, `ORACLE/storage_region_test.go`. `RegionBank` becomes private `generation:u64, entries:Box<[Entry;1024]>`; `try_from_entries(generation:u64,entries:Vec<Entry>)->StorageResult<Self>` checks len first and uses boxed-slice TryInto, avoiding a 24KiB stack copy. Read-only `generation()` and `entries()->&[Entry;1024]`; checked constructor/replace-entry method if mutation is needed by existing tests, revalidating whole bank. Raw RegionKey dimension stays i32. Valid occupied payload_length0 remains allowed. Standby generation0 is legal only for an all-empty bank and is not selectable as committed state.

Cases lengths0/1023/1024/1025/1200; only1024 proceeds. Invalid lengths return error without panic or zero padding; accepted last slot encodes correctly and all padding bytes are zero. Retain extent offset>=15, checked sector end,<=1MiB payload,capacity/revision/overlap rules and file length checks. Selection case:equal generations identical→A;equal generations different→corrupt;higher valid generation wins;one corrupted bank uses other;both standby fail. Topic `region_shape`; Go `go test ./packages/tools/cmd/runtime-oracle -run '^TestStorageOracleRegion$' -count=1`.

<a id="node-4-5"></a>

## 4.5 — Share current values while keeping raw historical DTOs

Dependencies:2.3,2.8,4.3,4.4. Own `src/{identity,items,player,chunk,companion,hostile,passive}.rs` mechanical value conversion sites and tests `storage_values.rs`. Use domain IDs/current ordinary stacks/compact sections, delete duplicate current item/durability/smelting tables. Keep format DTOs for raw historical triples, absent UUIDs and unchecked player armor. `RawItemStack {item:u16,count:u8,durability:u16}` is storage-private except as the field type of the public fidelity armor record; make that type public with documented raw semantics if needed by PlayerRecord's public API. Convert ordinary current inventory with ItemStack::try_new; historical migrations run **before** current validation. Player armor `[RawItemStack;4]` is copied exactly and never passed through ItemStack. Hostile absent target is Option<PlayerId>, serialized zero when None.

Chunk keeps existing format collections at the schema boundary, with exact-size aggregate validation from4.3; compact sections move into current domain sections after migration. This node does not reorder palettes or convert all chunks to dense buffers. Tests: ID65/66; broken armor durability0 and raw armor(4242,65,999) preserved; legacy tool durability synthesized exactly; raw region dimension−3 preserved; no companion NONE ID in valid records. Topic `storage_values`; run full foundation three-crate runtime_contract after integration. Controller owns simultaneous export/caller updates; no split commit with broken producers/consumers.

<a id="node-4-6"></a>

## 4.6 — Player codec and all nine migrations

Dependencies:4.5,1.5. Own `src/player.rs`, private bounded byte-writer helpers in `src/bytes.rs`/`error.rs` (first storage-buffer owner), tests `player_corpus.rs`, `ORACLE/storage_player_test.go`, `CORPUS/cases/save.player/`. Implement common player_encoded_len/encode_player_into and retain encode_player convenience. Preserve schema-specific defaults from storage-contracts exactly; no authority respawn/health correction. Save UUID matches requested ID,revision>0,canonical name,finite positions/yaw,pitch±pi/2,health/hunger<=20,saturation<=hunger*1000. Full u16 exhaustion and raw armor remain legal. Absent respawn decode ignores16 trailing location bytes, while current encode writes the entire17-byte absent field zero.

Cases every player-v1..v9 fixture with expected NeedsRewrite and default inventory/health/hunger/respawn/armor; mismatched UUID/revision0; health21;saturation one above hunger*1000;exhaustion65535;pitch boundary;raw armor(4242,65,999);CRC-resealed absent respawn with garbage location accepted and re-encoded zeros;present flag2 rejected. Short destination unchanged, including invalid value+short precedence. Max payload1MiB checked before reserve. Topic `player_corpus`; Go `go test ./packages/tools/cmd/runtime-oracle -run '^TestStorageOraclePlayer$' -count=1`.

<a id="node-4-7"></a>

## 4.7 — Metadata byte fidelity and six versions

Dependencies:4.6,1.5. Own `src/world_metadata.rs`, tests `metadata_corpus.rs`, new `packages/server/storage/metadata_oracle_test.go`, `CORPUS/cases/save.world-metadata/`. Metadata pure Go functions are private: use this package-local producer with encodeMetadata/decodeMetadata and existing v1..v5 builders. **Do not add production exports or invoke DiskStore/MemoryStore.** Add world_metadata_encoded_len/encode_world_metadata_into.

Remove Rust weather0..2 codec rejection. Current format accepts every u8 weather, arbitrary raw i32 spawn dimension,full u64 phase/remaining fields of their declared widths. Only Difficulty0..2 is semantic format admission. Preserve seed i64 and all anchors/salts. Decode v1..v6 to format_version6 with exact defaults; two-dimension count must be2. Exact total bytes are36/44/52/57/77/78. Runtime weather clamp and phase modulo stay out of codec.

Primary red value: current Metadata with weather_kind7,spawn_dimension−3,day_phase_offset=u64::MAX,difficulty0; Go encode/decode preserve fields, current Rust fails. Add weather255; difficulty3 fails; every historical default (including Depths anchor=spawn and salt0x9E3779B97F4A7C15); wrong dimension count,CRC/trailing/header failures. Topic `metadata_corpus`; Go `go test ./packages/server/storage -run '^TestMetadataOracle$' -count=1`. This is a newly source-inspected finding; record a real red run before fixing it.

<a id="node-4-8"></a>

## 4.8 — Hostile records and canonical output

Dependencies:4.6. Own `src/hostile.rs`, tests `hostile_corpus.rs`, `ORACLE/storage_hostile_test.go`. Add hostile_mobs_encoded_len/encode_hostile_mobs_into. Validate<=64 records before sorting; use sorted record indices, no record clones. Exact fields in storage-contracts; current record73 bytes,legacy72 defaults kind0. Count0 withrevision1 accepted. Decode strictly increasing IDs; encode canonicalizes unsorted valid values but rejects duplicates. Format-only fields retained:cooldowns<=20,distant<=600,target presence relation,next_repath_ticks full u64,kind0/1.

Cases v1/v2 fixtures;64/65 count; IDs2,1 encode sorted then decode1,2;duplicate ID;positionY−64/319 accepted,320 rejected;health0/21;cooldown21;distant601;target false with nonzero bytes;target true bad UUID;kind2;reserved/header/CRC errors. Topic `hostile_corpus`; Go `go test ./packages/tools/cmd/runtime-oracle -run '^TestStorageOracleHostile$' -count=1`.

<a id="node-4-9"></a>

## 4.9 — Passive records and reserved bytes

Dependencies:4.6. Own `src/passive.rs`, tests `passive_corpus.rs`, `ORACLE/storage_passive_test.go`. Add passive_mobs_encoded_len/encode_passive_mobs_into. Same canonical sorted-index strategy, count<=32,max2336 bytes; record72 with30 zero reserved bytes. Do not add transient grazing/flee/birth fields from runtime or wire. Count0 is a valid save, distinct from missing-file handling.

Cases passive-v1;32/33 count;ID order/duplicate;dimension1;positionY320;health0/21;bool2;each reserved-tail byte changed with CRC resealed;empty collection revision1 accepted/revision0 rejected. Destination canary/short/invalid precedence. Topic `passive_corpus`; Go `go test ./packages/tools/cmd/runtime-oracle -run '^TestStorageOraclePassive$' -count=1`.

<a id="node-4-10"></a>

## 4.10 — Reusable chunk codec with bounded compression

Dependencies:4.3,4.5,4.6. Own `src/chunk.rs`, tests `chunk_corpus.rs`, `ORACLE/storage_chunk_test.go`, `CORPUS/cases/save.chunk/`. `ChunkCodec::try_new()->StorageResult<Self>` owns compressor,decompressor,logical Vec and compressed Vec. `encode_into(&mut self,save:&ChunkSave,dst:&mut[u8])->StorageResult<usize>`; `decode(&mut self,key:ChunkKey,revision:u64,bytes:&[u8])->StorageResult<DecodedChunk>`. Existing free encode/decode create a context as explicitly allocating conveniences. No compressed encoded_len promise; `chunk_logical_len(&ChunkSave,schema:u32)` validates and returns exact logical size for diagnostic callers.

Algorithm: current aggregate preflight → logical-size<=2MiB → fill retained logical scratch → compress with current parameters and enforce compressed **frame**<=1MiB → construct44-byte envelope with exact lengths/checksums → destination preflight → one publication copy. Region's total-payload1MiB cap is separate. Decode envelope/version/identity/declared-size, bounded decompression stopping atlimit+1, exact length/CRC, schema parse, migration, then validate current aggregate before return. No partial decoded chunk, no global codec lock, no runtime-thread compression scheduling here.

Cases all 9 fixtures; current representations and worst legal palette; frame/bomb/CRC/key/revision mismatches; short output unchanged; scratch reuse after error. Legacy multistack tools split into available drop slots with full durability/count/generation preserved; fixture with insufficient slots rejects without data loss. No retroactive water insertion. For private logical Go migration builders add only `packages/server/storage/chunk/chunk_oracle_test.go`, reusing existing testEnvelopeForSchema; run `go test ./packages/server/storage/chunk -run '^TestChunkMigrationOracle$' -count=1` plus `go test ./packages/tools/cmd/runtime-oracle -run '^TestStorageOracleChunk$' -count=1`. Topic `chunk_corpus`; record cold/warm memory, logical parity and both cross-decodes.

<a id="node-4-11"></a>

## 4.11 — Companion writer and complete version corpus

Dependencies:4.1,4.2,4.5,4.6. Own `src/companion.rs`, tests `companion_corpus.rs`, `ORACLE/storage_companion_test.go`, corpus family. Add companions_encoded_len/encode_companions_into with complete preflight, sorted indices and no string clones. All count,task/step,text/lifecycle invariants use the established Go v5 canonical rules; encode nonempty legacy Queue.Summary in v5 fails. Empty queues are omitted canonically; nonempty ownership is preserved.

Read all v1..v5 fixtures and compare SourceSchema,revision,namespace,bodies,lifecycles,queues and full step variants. **Legacy decode is not automatically a valid v5 save**: absent namespace/lifecycles and legacy summary require MergeV5 policy, which is outside this codec node. Do not invent IDs/tombstones or drop summary just to re-encode. Record legacy decode/migration outcome exactly; compare current re-encode only for cases with an explicit valid v5 input supplied by Go. Add each step discriminant go_to/mine/place/follow,NextStep bounds,FIFO order, task failed reason,legacy summaries,v5 empty/nonempty queue canonicalization and all 4.2 bounds. Topic `companion_corpus`; Go `go test ./packages/tools/cmd/runtime-oracle -run '^TestStorageOracleCompanion$' -count=1`.

<a id="node-4-12"></a>

## 4.12 — Region caller buffers and corruption closure

Dependencies:4.4,4.6. Own `src/region.rs`, tests `region_corpus.rs`, extend `ORACLE/storage_region_test.go` serially. `encode_region_bank_into(key:RegionKey,bank:&RegionBank,dst:&mut[u8])->StorageResult<usize>` returns28672; existing array-returning wrapper delegates. `encode_superblock_into(key:RegionKey,dst:&mut[u8])->StorageResult<usize>` returns4096; all i32 keys valid. Validate occupied extents and exact fixed representation before any destination writes, zero reserved/padding bytes explicitly, compute CRC32C on the same logical spans as Go.

Golden seeds: key{dimension−3,x−1,z2},all-empty standby generation0;committed bankgeneration1 withentry0 offset15,sectors1,payload_length0,revision1,crc0;valid lastslot extent atsector16. Cases superblock key mismatch,wrongmagic/version,length,checksum,reserved padding;overlap/end overflow,beyondfile size61440,entry offset14,noncanonical empty fields;equal-generation conflict. Exact bytes and selection normalized outcome against Go. Topic `region_corpus`; Go `go test ./packages/tools/cmd/runtime-oracle -run '^TestStorageOracleRegion$' -count=1`.

## Acceptance boundary

These nodes close storage codec equivalence, not disk durability or recovery. Run all storage runtime_contract once after integration and let6.2 verify all seven families/versions and public encoders. A Go codec accepting raw values is not permission for a runtime owner to accept them as valid gameplay; the later owner must apply its own documented restoration/admission rule.
