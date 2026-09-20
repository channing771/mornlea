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

## Focused Verification

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_storage --test runtime_contract --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_storage --test runtime_contract --locked
```
