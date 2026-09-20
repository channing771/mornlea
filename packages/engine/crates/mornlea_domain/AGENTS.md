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

## Identity and text values (`src/identity.rs`, `src/text.rs`, `tests/identity_values.rs`)

- `PlayerId` and `CompanionId` are distinct checked UUIDv4 newtypes over the
  same bits (`try_from_bytes`/`bytes`); `HostileId`, `PassiveId` and
  `ProjectileId` are checked nonzero `u64` wrappers (`try_new`/`get`). No
  unchecked public constructor and no valid NONE companion identity exists in
  this crate.
- `DisplayName`, `CompanionName`, `CommandText` and `SpeechText` are owned
  `String` newtypes built through `try_from_canonical`, which performs no trim
  and no normalization: admission owns the trim, exactly as Go's normalizer
  returns its trimmed output. `CompanionName` additionally rejects embedded
  Unicode whitespace.
- The whitespace and control predicates are explicit character ranges
  (U+0009..000D, 0020, 0085, 00A0, 1680, 2000..200A, 2028, 2029, 202F, 205F,
  3000 and U+0000..001F, U+007F..009F), never a Unicode crate, so behavior
  cannot drift with a Unicode-version bump and the crate's empty production
  dependency set holds. U+200B stays accepted, as in Go; there is no NFC.

## Item, drop and container values (`src/items.rs`, `src/locations.rs`, `tests/items_locations.rs`)

- Exactly one authoritative Rust copy of the item tables (stack limits,
  durability maxima, smelting output and product predicates) is ported from the
  verified Go core and compared exhaustively by item ID across 0..66; do not
  fork a second copy into another module.
- `ItemStack::try_new` reproduces the Go `Valid` precedence (registration,
  count, durability), admits the empty triple as exactly `(0,0,0)`, and rejects
  a durable item at durability 0. Equipped broken armor is deliberately
  outside this type: armor-owned slots are raw fidelity data and must not
  relax ordinary stack rules.
- `DropId::try_new` retains an arbitrary raw dimension because Go's
  `DropID.Valid` checks only the slot range and the generation; a stricter Rust
  rule is a parity break. Ordering is dimension, chunk x, chunk z, slot,
  generation.
- `ContainerRef::try_new` is inherently overworld and takes no dimension: the
  protocol conversion validates the raw wire dimension before construction.
  Absence is `Option<ContainerRef>`; there is no invalid domain reference.
- `ChunkPos` and `FiniteVec3`/`LookAngles` are plain coordinates and finite
  vectors: private fields, getters, no normalization, no clamping, and exact
  `f32` bit preservation including negative zero.

## Value and input bounds (`src/values.rs`, `src/input.rs`, `src/input/control.rs`, `tests/runtime_contract.rs`, `tests/command_control.rs`)

- `Dimension` accepts only overworld and depths; any other ID is
  `InvalidDimension` (`invalid_dimension_and_hotbar_ranges_are_rejected`).
- `HotbarSlot` accepts `0..COUNT-1`; `PlacementIntent::try_new` and
  `Command::SelectHotbar` reuse that range
  (`invalid_dimension_and_hotbar_ranges_are_rejected`,
  `command_control_placement_slot_eight_succeeds_and_nine_fails`).
- `LookAngles` is the single owner of the finite-rotation rule: NaN and
  infinities are `NonFiniteRotation`, and the bits are preserved exactly
  including negative zero. The grouped payloads in `input/control.rs` carry an
  already-validated `LookAngles`, so `PlayerControl::new` is total and named
  `new` rather than `try_new` (`non_finite_input_rotation_is_rejected`,
  `command_control_non_finite_rotation_fails_look_angles`).
- `Movement` and `HeldActions` are plain grouped controls with public fields:
  the move axes keep their full `i8` range because the −1..1 rule belongs to
  the authority, and `HeldActions::primary` maps exactly to the Go `Mining`
  bit without implying that mining wins over combat. No payload names a
  target cell, a hit entity, a placed block, a consumed item or an outcome.
- `Command` is the extensible intent enum introduced with the movement and
  ray variants (`PlayerInput`, `PlaceBlock`, `Resync`, `SelectHotbar`,
  `OpenContainer`, `TillSoil`, `BoneMeal`, `CollectWater`, `PlaceWater`);
  later inventory, container and chat intents are added as new variants.
  `ResyncIntent` accepts a zero `have_revision` because it names a chunk the
  client holds nothing for.
- `SemanticInput` and `order_inputs` are a temporary replay-ordering test
  facade that pairs a payload with a sequence and performs no validation of
  its own; `order_inputs` sorts by sequence, then kind name
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
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test identity_values --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test items_locations --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test command_control --locked
```
