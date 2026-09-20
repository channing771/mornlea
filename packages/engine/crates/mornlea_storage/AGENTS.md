# Storage contracts

`packages/engine/crates/mornlea_storage` owns versioned save records and
supported migration codecs, including standalone entity families. It is a
windowless rlib. Production code may depend only on `mornlea_domain` and must
not depend on `mornlea_protocol`, `mornlea_engine`, `mornlea_client`, or
`mornlea_godot`. Direction is enforced by `tests/runtime_contract.rs`
(`production_manifest_depends_only_on_domain` and
`domain_does_not_depend_on_storage`).

## Inventory freeze (`src/lib.rs`, `tests/runtime_contract.rs`)

- Eventual owner of every `save.*` inventory row.
- Registration tests fail if the frozen corpus drops a save family or if
  production dependencies reverse onto domain or reach protocol/kernel/host
  crates.
- Save families are ported with current-schema round-trips, supported
  migrations, and corrupt/partial rejection; this crate must not repair
  invalid records.

## Shared primitives (`src/bytes.rs`, `src/crc32c.rs`, `src/error.rs`)

- `ByteReader`/`ByteWriter` are the single little-endian byte layer for every
  family; fixed-width integers match the on-disk layout exactly.
- `crc32c`/`crc32c_join` are the Castagnoli CRC-32C used by every envelope.
  They hash header slices plus payload without materializing the
  concatenation, and are public so contract tests can reseal a mutated
  fixture.
- `StorageError::Corrupt` and `StorageError::FutureVersion` keep the two Go
  storage sentinels distinct: both reject, neither repairs.

## Ported families

Each family is one module re-exported from `src/lib.rs`, and each is verified
against the committed Go binary fixture where one exists (byte-for-byte
re-encode equality).

| Family | Module | Current schema | Notes |
| --- | --- | --- | --- |
| `save.passive` | `src/passive.rs` | v1 | 32-byte header + fixed 72-byte records, 30-byte zero reserved tail, canonical ascending-ID order |
| `save.hostile` | `src/hostile.rs` | v2 | v1 records lack the trailing `kind` byte and migrate to nightcrawler; re-encode keeps each v1 record as the v2 prefix |
| `save.region` | `src/region.rs` | v1 | fixed 4096-byte superblock plus two 28672-byte banks; newest valid generation wins, ties break to bank A |
| `save.player` identity | `src/identity.rs` | — | `PlayerId` UUIDv4 wrapper shared by the entity families |

## Focused Verification

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_storage --test runtime_contract --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_storage --test runtime_contract --locked
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml -p mornlea_storage -- --check
```
