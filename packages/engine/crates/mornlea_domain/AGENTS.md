# Domain contracts

`packages/engine/crates/mornlea_domain` owns shared identifiers, value rules,
and semantic input/event records for the Rust runtime foundation. It is a
windowless rlib. Production code must not depend on protocol codecs, save
codecs, `mornlea_engine`, `mornlea_client`, or `mornlea_godot`. Direction is
enforced by `tests/runtime_contract.rs` (`production_manifest_has_no_codec_kernel_or_host_dependencies`).

## Inventory freeze (`src/lib.rs`, `tests/runtime_contract.rs`)

- Eventual owner of the `domain.input` and `domain.event` inventory rows.
- Registration tests fail if the frozen corpus drops those rows or assigns
  this crate a production dependency on a codec, kernel, or graphical host.
- Family semantics are ported behind `runtime_contract` cases; this crate
  must not infer parity from an unimplemented subset.

## Identity completeness (`src/identity.rs`, `tests/runtime_contract.rs`)

- `Identities::current` is the Rust pin for supported protocol, save, ABI,
  and agent versions. `current_identities_match_frozen_inventory` compares it
  with `testdata/runtime-migration/contracts.json`; do not keep a second
  matrix in this guide.
- `Identities::validate` and `ReplayIdentity::new` reject a zero version,
  empty agent label, empty source revision, or empty corpus digest as
  `IncompleteIdentity` before a replay identity is published
  (`incomplete_identity_is_rejected`).

## Value and input bounds (`src/values.rs`, `src/input.rs`, `tests/runtime_contract.rs`)

- `Dimension` accepts only overworld and depths; any other ID is
  `InvalidDimension` (`invalid_dimension_and_hotbar_ranges_are_rejected`).
- `HotbarSlot` accepts `0..COUNT-1`; `PlaceBlock` and `SelectHotbar` reuse
  that range (`invalid_dimension_and_hotbar_ranges_are_rejected`).
- `PlayerInput` and `PlaceBlock` reject NaN or infinite yaw/pitch as
  `NonFiniteRotation` (`non_finite_input_rotation_is_rejected`).
- `order_inputs` sorts by sequence, then kind name
  (`semantic_inputs_order_by_sequence_then_kind`).

## Observations (`src/event.rs`, `tests/runtime_contract.rs`)

- `Observation::new` publishes only `domain.input` and `domain.event`
  family IDs; any other family is `UnknownId`
  (`unknown_observation_family_is_rejected`).
- `order_observations` sorts by tick, then family id
  (`observations_order_by_tick_then_family`).

## Focused Verification

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract --locked
```
