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

## Value and input bounds (`src/values.rs`, `src/input.rs`, `src/input/control.rs`, `src/input/inventory.rs`, `src/input/chat.rs`, `tests/runtime_contract.rs`, `tests/command_control.rs`, `tests/command_inventory.rs`)

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
- `Command` is the extensible intent enum over all 19 sequenced play
  variants: the movement and ray variants (`PlayerInput`, `PlaceBlock`,
  `Resync`, `SelectHotbar`, `OpenContainer`, `TillSoil`, `BoneMeal`,
  `CollectWater`, `PlaceWater`) plus the inventory and container variants
  (`MoveInventory`, `MoveCrafting`, `MoveContainer`, `CloseContainer`,
  `DropSelectedItem`, `TakeCraftingOutput`, `EquipArmor`, `MovePartial`,
  `QuickMove`, `DropStack`). No payload carries a sequence, an envelope, a
  target cell, a hit entity, a placed block, a consumed item or an outcome.
  `ResyncIntent` accepts a zero `have_revision` because it names a chunk the
  client holds nothing for.
- The inventory and container payloads in `input/inventory.rs` pin the exact
  Go bounds per command. `StackView {Inventory, Crafting, Container(ContainerRef)}`
  replaces the raw view numbers and the all-zero sentinel reference: absence of
  a container is expressed by the variant itself, so a zero `ContainerRef`
  never enters the domain type. `MoveCrafting` alone rejects two
  inventory-region indices, and `MoveContainer` alone rejects the furnace
  output slot as a destination; `MovePartial`, `QuickMove` and `DropStack`
  share the view and reference bounds without those stricter rules because
  the protocol does not reject them and the authority applies item and slot
  rules later. A malformed real container reference is rejected before slot
  checks. `TakeCraftingOutput` carries nothing but its variant because its
  nonzero-sequence rule belongs to envelope construction.
- `ChatIntent { text: CommandText }` in `input/chat.rs` is deliberately
  outside `Command`: chat has no wire sequence and is consumed through its own
  FIFO. It performs no addressing, warp, stop or queue policy, and the text is
  retained verbatim including a leading mention.

## Command envelope and ordering (`src/input/order.rs`, `tests/command_order.rs`)

- `CommandEnvelopeParts { tick, session, sequence, arrival_index: u64, command: Command }`
  is the only place intake metadata meets a payload. `CommandEnvelope::try_new`
  keeps the fields verbatim and rejects a zero sequence for
  `TakeCraftingOutput` alone, because that command has to take part in command
  acknowledgement; every other command accepts sequence zero, which is
  wire-valid. No payload carries a sequence of its own.
- `CommandOrderScratch::try_with_capacity` owns one `(u64, u64, u64)` key slot
  per command and never grows: `capacity` is the caller's budget, and an
  unreservable request fails closed with `InsufficientScratch` rather than
  aborting. `order_commands` refuses a batch larger than the scratch before it
  fills a single key.
- `order_commands` orders in place by `(tick, session, sequence,
  arrival_index)`. No key consults the command kind, so a same-sequence pair of
  different kinds resolves to the earliest arrival — the Go authority's
  behavior in `packages/server/sim/runtime/engine_step.go`, which has no
  kind-name tiebreaker. Session generation stays runtime ingress lifecycle data
  and is neither filtered nor checked here.
- The arrival key `(tick, session, arrival_index)` is validated before any
  command moves: a duplicate is `DuplicateArrival` and leaves the caller's
  slice byte for byte unchanged, because a caller cannot otherwise tell which
  of the two it submitted first. A warm call allocates nothing — the slice is
  sorted in place and the keys come from the scratch.
- Focused entry: `cargo test -p mornlea_domain --test command_order --locked`
  (12 cases: the red input, reverse arrival, same-kind duplicates, the same
  sequence across sessions, two ticks, duplicate arrival with unchanged input,
  an empty batch, exact and short scratch, scratch reuse after failure, the
  zero-sequence rule, the unreservable capacity, and the envelope carrying
  every payload family unchanged).

## Command outcomes and player publications (`src/event/outcome.rs`, `src/event/player.rs`, `tests/event_player.rs`)

- `CommandRejection`, `PlacementSuccess` and `CombatHit` in `event/outcome.rs` are
  the three results an authoritative tick publishes to the session that caused
  them. `CommandRejection::new` and `PlacementSuccess::new` are total and keep a
  zero sequence, which the Go wire accepts and the `/warp` rejects publish;
  `CombatHit::try_new` rejects a zero server tick and a damage value outside
  `1..=MAX_HEALTH`, which is the Go `CombatHit.Validate` rule.
- `RejectReason` is the closed set of the fifteen published reasons. Its wire
  value comes from the explicit `wire_id` match, never from a discriminant cast,
  so reordering the variants cannot change what the wire carries; the Go
  internal enum runs `0..14` while the wire enum runs `1..15`.
  `CombatTarget` is the same shape for the three published kinds
  (`try_new`/`wire_id`), and `Weather` and `Season` pin their wire IDs the same
  way.
- `PlayerState` in `event/player.rs` is the private per-session publication and
  nothing else: body, survival and world scalars. It carries no inventory, no
  crafting, no container and no equipped-armor field, because those are
  separate records owned by later nodes; the seed literal in the test lists
  every field explicitly, so an added field fails to compile.
- The grouped records follow the crate's parts convention.
  `MotionState::new` is total because its parts carry already-validated
  `FiniteVec3`s; `SurvivalState::try_new` rejects a health, oxygen, hunger or
  armor value above the Go `core` maximum rather than clamping it;
  `WorldState::try_new` rejects a day phase offset at or above one display day;
  season progress and temperature are the two full-range scalars and carry no
  sub-range rule.
- `MiningState::try_new` is the checked conversion of one wire mining block, not
  an inference. An inactive block has to be entirely empty — exact zero target,
  zero progress and requirement, false harvestable flag — because the Go
  validator rejects an inactive block that still carries part of a swing, and a
  client would otherwise have to guess whether a stale target still applies. An
  active block goes through `ActiveMining::try_new`, which requires
  `0 < progress < required` so a completed swing is published as inactive.
- `BlockPos` in `src/locations.rs` is the world block coordinate triple. Like
  `ChunkPos` it carries no invariant and constructs totally, because the Go
  `core.BlockPos` imposes no coordinate rule on the player-state mining target;
  `BlockPos::ORIGIN` is the exact zero target an inactive block has to carry.
- Focused entry: `cargo test -p mornlea_domain --test event_player --locked`
  (13 cases: the seed record and its field map, the absent inventory and
  armor, the four survival maxima plus one, the offset, weather, season and
  dimension boundaries, the full-range scalars, the non-finite pose and
  angles, the inactive mining residue, the active mining range, the zero
  sequences, the frozen reject-reason wire table, and the combat hit and
  target-kind boundaries).

## Observations (`src/event.rs`, `tests/runtime_contract.rs`)

- `Observation::new` publishes only `domain.input` and `domain.event`
  family IDs; any other family is `UnknownId`
  (`unknown_observation_family_is_rejected`).
- `order_observations` sorts by tick, then family id
  (`observations_order_by_tick_then_family`).

## Focused Verification

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test command_order --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test identity_values --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test items_locations --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test command_control --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test command_inventory --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_player --locked
```
