# Protocol contracts

`packages/engine/crates/mornlea_protocol` owns versioned framing, negotiation,
and packet-family codecs. It is a windowless rlib. Production code may depend
only on `mornlea_domain` and must not depend on `mornlea_storage`,
`mornlea_engine`, `mornlea_client`, or `mornlea_godot`. Direction is enforced
by `tests/runtime_contract.rs` (`production_manifest_depends_only_on_domain`
and `domain_does_not_depend_on_protocol`).

## Inventory freeze (`src/lib.rs`, `tests/runtime_contract.rs`)

- Eventual owner of every `protocol.*` inventory row, including framing.
- Registration tests fail if the frozen corpus drops protocol rows or if
  production dependencies reverse onto domain or reach storage/kernel/host
  crates.
- Packet families are ported one inventory row at a time with byte-preserving
  and malformed-input cases; this crate must not infer parity from a covered
  subset.

## Framing (`src/frame.rs`, `src/varint.rs`, `tests/runtime_contract.rs`)

- `write_frame` / `read_frame` are the length-prefixed packet boundary.
  The length is a canonical uvarint and does not include itself.
- Empty, oversized, truncated, overlong, and non-canonical length prefixes
  fail before a payload is published
  (`frame_read_rejects_invalid_lengths_before_payload`,
  `frame_read_rejects_truncated_and_noncanonical_packet_id`,
  `frame_write_enforces_maximum_payload`).
- Capacity is `MAX_FRAME_BYTES`; do not copy that number here.
- Canonical uvarint vectors are pinned by
  `canonical_uvarint_round_trips_and_rejects_malformed`.
- Coalesced frames consume only one record
  (`frame_round_trip_preserves_packet_id_and_payload`).

## Client hello (`src/client_hello.rs`, `tests/runtime_contract.rs`)

- Handshake packet ID 0 payload is a canonical protocol-version uvarint.
- `ClientHello::new` / `decode` accept only `Identities::current().protocol`.
  Other versions are `UnsupportedVersion`; trailing bytes and truncated
  varints fail before publication
  (`client_hello_round_trip_preserves_current_version_bytes`,
  `client_hello_rejects_unknown_version_and_malformed_payload`).

## Server hello (`src/server_hello.rs`, `tests/runtime_contract.rs`)

- Handshake packet ID 0 payload is the same canonical protocol-version
  uvarint as ClientHello, on the server-to-client handshake ID space.
- `ServerHello::new` / `decode` accept only `Identities::current().protocol`.
  Other versions are `UnsupportedVersion`; trailing bytes and truncated
  varints fail before publication
  (`server_hello_round_trip_preserves_current_version_bytes`,
  `server_hello_rejects_unknown_version_and_malformed_payload`).

## Handshake reject (`src/handshake_reject.rs`, `src/bytes.rs`, `tests/runtime_contract.rs`)

- Handshake packet ID 1 payload is a canonical uvarint server protocol
  version, a one-byte reject code, and a length-prefixed UTF-8 message.
- Only `HANDSHAKE_VERSION_MISMATCH` (`1`) is a published code. The server
  version is informational and is not required to match the current
  protocol. Unknown codes are `InvalidEnum`; invalid UTF-8, oversized
  declared lengths, and oversized messages are `InvalidString`; trailing
  bytes fail before publication
  (`handshake_reject_round_trip_preserves_golden_bytes`,
  `handshake_reject_round_trip_preserves_empty_message`,
  `handshake_reject_rejects_unknown_code_and_malformed_payload`).
- `ByteEncoder` / `ByteDecoder` are crate-private payload primitives shared
  by later packet families. They are not a public codec surface.

## Login start (`src/login_start.rs`, `src/player_id.rs`, `tests/runtime_contract.rs`)

- Login packet ID 0 payload is a 16-byte UUIDv4, a length-prefixed display
  name, and a trailing view-distance byte.
- `PlayerId::new` accepts only non-zero UUIDv4 values. Display names must
  remain valid after trimming (`1..=32` runes, `<=128` bytes, no control
  characters). View distance is the closed interval `2..=64`. Failures are
  `InvalidIdentity`, `InvalidString`, or `InvalidRange` before publication
  (`login_start_round_trip_preserves_golden_bytes`,
  `login_start_rejects_invalid_identity_name_range_and_malformed_payload`).

## Login success (`src/login_success.rs`, `tests/runtime_contract.rs`)

- Login packet ID 0 payload is a 16-byte UUIDv4 followed by a little-endian
  `u64` world seed. Zero seeds are legal. Non-v4 identities, truncated
  payloads, and trailing bytes fail before publication
  (`login_success_round_trip_preserves_golden_bytes`,
  `login_success_rejects_invalid_identity_and_malformed_payload`).

## Login reject (`src/login_reject.rs`, `tests/runtime_contract.rs`)

- Login packet ID 1 payload is a one-byte reject code and a length-prefixed
  UTF-8 message (max 256 bytes/runes). Published codes are the closed
  interval `1..=7` copied from Go `LoginRejectCode`. Unknown codes are
  `InvalidEnum`; invalid UTF-8 and oversized declared lengths are
  `InvalidString`; trailing bytes fail before publication
  (`login_reject_round_trip_preserves_golden_bytes`,
  `login_reject_round_trip_preserves_empty_message_codes`,
  `login_reject_rejects_unknown_code_and_malformed_payload`).

## Disconnect (`src/disconnect.rs`, `tests/runtime_contract.rs`)

- Play packet ID 6 payload is a one-byte disconnect code and a
  length-prefixed UTF-8 message (max 256 bytes/runes). Published codes are
  the closed interval `1..=5` copied from Go `DisconnectCode`. Unknown
  codes are `InvalidEnum`; invalid UTF-8 and oversized declared lengths are
  `InvalidString`; trailing bytes fail before publication
  (`disconnect_round_trip_preserves_golden_bytes`,
  `disconnect_round_trip_preserves_empty_message_codes`,
  `disconnect_rejects_unknown_code_and_malformed_payload`).

## Keep alive (`src/keep_alive.rs`, `tests/runtime_contract.rs`)

- Play packet ID 5 payload is a little-endian `u64` token. Zero tokens are
  `InvalidRange`; truncated payloads and trailing bytes fail before
  publication (`keep_alive_round_trip_preserves_golden_bytes`,
  `keep_alive_rejects_zero_token_and_malformed_payload`).

## Keep alive reply (`src/keep_alive_reply.rs`, `tests/runtime_contract.rs`)

- Play packet ID 4 payload is a little-endian `u64` token. Zero tokens are
  `InvalidRange`; truncated payloads and trailing bytes fail before
  publication (`keep_alive_reply_round_trip_preserves_golden_bytes`,
  `keep_alive_reply_rejects_zero_token_and_malformed_payload`).

## Place block succeeded (`src/place_block_succeeded.rs`, `tests/runtime_contract.rs`)

- Play packet ID 20 payload is a little-endian `u64` sequence. Zero
  sequences are legal. Truncated payloads and trailing bytes fail before
  publication (`place_block_succeeded_round_trip_preserves_golden_bytes`,
  `place_block_succeeded_rejects_malformed_payload_and_accepts_zero_sequence`).

## Command rejected (`src/command_rejected.rs`, `tests/runtime_contract.rs`)

- Play packet ID 4 payload is a little-endian `u64` sequence followed by a
  one-byte reject reason. Published reasons are the closed interval
  `1..=15` copied from Go `CommandRejectReasonID`. Unknown IDs are
  `InvalidEnum`; trailing bytes fail before publication
  (`command_rejected_round_trip_preserves_golden_bytes`,
  `command_rejected_round_trip_preserves_frozen_reason_ids`,
  `command_rejected_rejects_unknown_reason_and_malformed_payload`).

## Select hotbar (`src/select_hotbar.rs`, `tests/runtime_contract.rs`)

- Play packet ID 5 payload is a little-endian `u64` sequence followed by a
  hotbar slot u8. Slot must be inside domain `HotbarSlot` (`0..=8`).
  Out-of-range slots are `InvalidRange`; trailing bytes fail before
  publication (`select_hotbar_round_trip_preserves_golden_bytes`,
  `select_hotbar_rejects_invalid_slot_and_malformed_payload`).

## Drop selected item (`src/drop_selected_item.rs`, `tests/runtime_contract.rs`)

- Play packet ID 11 payload is a little-endian `u64` sequence. Zero
  sequences are legal. The selected slot and drop position stay
  server-owned. Truncated payloads and trailing bytes fail before
  publication (`drop_selected_item_round_trip_preserves_golden_bytes`,
  `drop_selected_item_rejects_malformed_payload_and_accepts_zero_sequence`).

## Equip armor (`src/equip_armor.rs`, `tests/runtime_contract.rs`)

- Play packet ID 18 payload is a little-endian `u64` sequence, the same
  shape as DropSelectedItem. Zero sequences are legal. The selected item
  and destination armor slot stay server-owned. Truncated payloads and
  trailing bytes fail before publication
  (`equip_armor_round_trip_preserves_golden_bytes`,
  `equip_armor_rejects_malformed_payload_and_accepts_zero_sequence`).

## Take crafting output (`src/take_crafting_output.rs`, `tests/runtime_contract.rs`)

- Play packet ID 15 payload is a little-endian `u64` sequence. Zero
  sequences are `InvalidRange` because they cannot take part in command
  acknowledgement. Output contents stay server-owned. Truncated payloads
  and trailing bytes fail before publication
  (`take_crafting_output_round_trip_preserves_golden_bytes`,
  `take_crafting_output_rejects_zero_sequence_and_malformed_payload`).

## Move inventory stack (`src/move_inventory_stack.rs`, `tests/runtime_contract.rs`)

- Play packet ID 6 payload is a little-endian `u64` sequence plus source
  and target slot bytes. Slots must be distinct and inside
  `0..INVENTORY_SLOTS-1` (`36`, copied from Go `InventorySlots`). Same-slot
  and out-of-range pairs are `InvalidRange`; trailing bytes fail before
  publication (`move_inventory_stack_round_trip_preserves_golden_bytes`,
  `move_inventory_stack_rejects_invalid_slots_and_malformed_payload`).

## Move crafting stack (`src/move_crafting_stack.rs`, `tests/runtime_contract.rs`)

- Play packet ID 7 payload is a little-endian `u64` sequence plus unified
  view slots. Grid is `0..CRAFTING_GRID_SLOTS-1`; inventory is
  `CRAFTING_GRID_SLOTS..GRID_CRAFTING_VIEW_SLOTS-1`. Same-slot, out-of-range,
  and inventory-to-inventory pairs are `InvalidRange`; trailing bytes fail
  before publication (`move_crafting_stack_round_trip_preserves_golden_bytes`,
  `move_crafting_stack_rejects_invalid_slots_and_malformed_payload`).

## Close container (`src/close_container.rs`, `tests/runtime_contract.rs`)

- Play packet ID 10 payload is a little-endian `u64` sequence. Zero
  sequences are legal. The viewed container identity stays server-owned.
  Truncated payloads and trailing bytes fail before publication
  (`close_container_round_trip_preserves_golden_bytes`,
  `close_container_rejects_malformed_payload_and_accepts_zero_sequence`).

## Place block (`src/place_block.rs`, `src/bytes.rs`, `tests/runtime_contract.rs`)

- Play packet ID 2 payload is a little-endian `u64` sequence, two
  little-endian `f32` look angles, and a hotbar slot byte. Slot must be
  inside domain `HotbarSlot` (`0..=8`). Non-finite yaw/pitch are
  `InvalidFloat`; out-of-range slots are `InvalidRange`; trailing bytes
  fail before publication (`place_block_round_trip_preserves_golden_bytes`,
  `place_block_rejects_invalid_slot_non_finite_and_malformed_payload`).
- `ByteEncoder` / `ByteDecoder` `f32` helpers copy the Go primitive: NaN
  and Inf fail before a payload is published.

## Open container (`src/open_container.rs`, `tests/runtime_contract.rs`)

- Play packet ID 8 payload is a little-endian `u64` sequence followed by
  two little-endian `f32` look angles. The server ray-casts the
  authoritative world and decides whether the hit block is a furnace or a
  chest, so the client never declares a container kind. A zero sequence is
  legal. Non-finite yaw/pitch are `InvalidFloat`; truncated payloads and
  trailing bytes fail before publication
  (`open_container_round_trip_preserves_golden_bytes`,
  `open_container_rejects_non_finite_and_malformed_payload`).

## Request chunk resync (`src/request_chunk_resync.rs`, `tests/runtime_contract.rs`)

- Play packet ID 3 payload is a little-endian `u64` sequence, the target
  dimension, two little-endian chunk coordinates, and the revision the
  client already holds. Only domain `Dimension::OVERWORLD` and
  `Dimension::DEPTHS` are known; any other dimension is `InvalidEnum`.
  Chunk state stays server-owned; negative coordinates are legal.
  Truncated payloads and trailing bytes fail before publication
  (`request_chunk_resync_round_trip_preserves_golden_bytes`,
  `request_chunk_resync_rejects_unknown_dimension_and_malformed_payload`).
- `ByteEncoder` / `ByteDecoder` `i32` helpers use two's-complement
  little-endian encoding, matching the Go primitive.

## Till soil (`src/till_soil.rs`, `tests/runtime_contract.rs`)

- Play packet ID 13 payload is a little-endian `u64` sequence followed by
  two little-endian `f32` look angles, with no slot byte. The server
  validates that the ray-cast target is fluid-adjacent dirt and owns the
  resulting block write. A zero sequence is legal. Non-finite yaw/pitch
  are `InvalidFloat`; truncated payloads and trailing bytes fail before
  publication (`till_soil_round_trip_preserves_golden_bytes`,
  `till_soil_rejects_non_finite_and_malformed_payload`).

## Bone meal (`src/bone_meal.rs`, `tests/runtime_contract.rs`)

- Play packet ID 14 payload is a little-endian `u64` sequence followed by
  two little-endian `f32` look angles, with no slot byte. The server
  validates that the ray-cast target is a fertilizable plant block and
  owns the resulting block write. A zero sequence is legal. Non-finite
  yaw/pitch are `InvalidFloat`; truncated payloads and trailing bytes
  fail before publication (`bone_meal_round_trip_preserves_golden_bytes`,
  `bone_meal_rejects_non_finite_and_malformed_payload`).

## Collect water (`src/collect_water.rs`, `tests/runtime_contract.rs`)

- Play packet ID 16 payload is a little-endian `u64` sequence followed by
  two little-endian `f32` look angles, with no slot byte. The server
  validates that the ray-cast target is a water source the held bucket
  can collect from and owns the resulting item change. A zero sequence is
  legal. Non-finite yaw/pitch are `InvalidFloat`; truncated payloads and
  trailing bytes fail before publication
  (`collect_water_round_trip_preserves_golden_bytes`,
  `collect_water_rejects_non_finite_and_malformed_payload`).

## Place water (`src/place_water.rs`, `tests/runtime_contract.rs`)

- Play packet ID 17 payload is a little-endian `u64` sequence followed by
  two little-endian `f32` look angles, with no slot byte. The server
  validates that the held bucket is water-filled and owns the resulting
  block write. A zero sequence is legal. Non-finite yaw/pitch are
  `InvalidFloat`; truncated payloads and trailing bytes fail before
  publication (`place_water_round_trip_preserves_golden_bytes`,
  `place_water_rejects_non_finite_and_malformed_payload`).

## Container reference (`src/container_ref.rs`, `tests/runtime_contract.rs`)

- `ContainerRef` is the shared 18-byte wire value that fleet and chest
  commands carry: a little-endian `i32` dimension, two chunk coordinates,
  a one-byte kind, a slot byte, and a little-endian `u32` generation.
  Both container kinds live in the overworld's fixed per-chunk arrays, so
  a foreign dimension, out-of-range slot, or zero generation is
  `InvalidRange`, and an unknown kind is `InvalidEnum`. `write`/`read`
  keep furnaces and chests on one encoding.

## Move container stack (`src/move_container_stack.rs`, `tests/runtime_contract.rs`)

- Play packet ID 9 payload is a `u64` sequence, an 18-byte container
  reference, and unified source and target slot bytes. Furnace unified
  slots are `0..FURNACE_VIEW_SLOTS-1` with `FURNACE_OUTPUT_SLOT` legal
  only as a source; chest unified slots are `0..CHEST_VIEW_SLOTS-1`.
  Same-slot and out-of-range pairs are `InvalidRange`; unknown kinds and
  malformed references are `InvalidEnum`; truncated payloads and trailing
  bytes fail before publication
  (`move_container_stack_round_trip_preserves_golden_bytes`,
  `move_container_stack_rejects_invalid_container_and_malformed_payload`).

## Move stack partial (`src/move_stack_partial.rs`, `tests/runtime_contract.rs`)

- Play packet ID 19 payload is a `u64` sequence, an 18-byte container
  reference, a view byte, source and target bytes, and a single-item
  flag. The moved amount is derived by the server from the source stack,
  so the wire carries no count field. The three view domains are
  `STACK_VIEW_INVENTORY` (`0`), `STACK_VIEW_CRAFTING` (`1`), and
  `STACK_VIEW_CONTAINER` (`2`); the container view bounds the index by the
  referenced container kind, while the inventory and crafting views must
  carry the zero container reference. Same-slot and out-of-range pairs are
  `InvalidRange`; unknown views and kinds are `InvalidEnum`; a `single`
  flag outside 0/1 is `InvalidEnum`; truncated payloads and trailing bytes
  fail before publication
  (`move_stack_partial_round_trip_preserves_golden_bytes`,
  `move_stack_partial_rejects_invalid_view_and_malformed_payload`).

## Quick move stack (`src/move_stack_partial.rs`, `tests/runtime_contract.rs`)

- Play packet ID 20 payload is the `MoveStackPartial` prefix without the
  target and single-item flag: a `u64` sequence, an 18-byte container
  reference, a view byte, and one source byte. The destination is a fixed
  deterministic contract the server derives, so the wire carries no target
  slot and there is no same-slot rejection. The same static view,
  container-reference, and index bounds apply; unknown views and kinds are
  `InvalidEnum`; truncated payloads and trailing bytes fail before
  publication (`quick_move_stack_round_trip_preserves_golden_bytes`,
  `quick_move_stack_rejects_invalid_view_and_malformed_payload`).

## Drop stack (`src/move_stack_partial.rs`, `tests/runtime_contract.rs`)

- Play packet ID 21 payload is a `u64` sequence, an 18-byte container
  reference, a view byte, and a unified slot byte. The drop position is
  derived by the server from the authoritative player state, so the wire
  carries no coordinates. The static view, container-reference, and index
  bounds match the other view-addressed commands; unknown views and kinds
  are `InvalidEnum`; truncated payloads and trailing bytes fail before
  publication (`drop_stack_round_trip_preserves_golden_bytes`,
  `drop_stack_rejects_invalid_view_and_malformed_payload`).

## Chat command (`src/chat_command.rs`, `tests/runtime_contract.rs`)

- Play packet ID 12 payload is a length-prefixed UTF-8 instruction bounded
  by `CHAT_COMMAND_TEXT_MAX_BYTES` (`1024`, shared with the planner
  instruction limit). The text must be 1..=max bytes, valid UTF-8, free of
  NUL and Unicode control characters, and untrimmed whitespace is
  rejected. Failures are `InvalidString`; oversized declared lengths and
  truncated payloads fail before publication; trailing bytes fail after the
  last field (`chat_command_round_trip_preserves_golden_bytes`,
  `chat_command_rejects_blank_control_and_malformed_payload`).

## Player input (`src/player_input.rs`, `tests/runtime_contract.rs`)

- Play packet ID 0 payload is the highest-frequency record: a `u64`
  sequence, two `i8` move axes, four action flags around two `f32` look
  angles, in field order sequence, move X, move Z, jump, yaw, pitch,
  mining, eating, sprinting, sneaking. Rotation must be finite; the
  domain type `mornlea_domain::PlayerInput` is the single owner of that
  rule, so a non-finite yaw or pitch is `InvalidFloat`. A non-0/1 flag
  byte is `InvalidEnum`; truncated payloads and trailing bytes fail
  before publication (`player_input_round_trip_preserves_golden_bytes`,
  `player_input_rejects_non_finite_and_malformed_payload`).

## Combat hit (`src/combat_hit.rs`, `tests/runtime_contract.rs`)

- Play packet ID 25 payload is a fixed 10-byte confirmation: a
  little-endian `u64` server tick, a damage byte, and a target kind byte.
  The server tick must be non-zero, damage must be inside `1..=MAX_HEALTH`
  (`20`, copied from Go `core.MaxHealth`), and the kind must be inside
  `COMBAT_TARGET_PLAYER..=COMBAT_TARGET_PASSIVE` (`1..=3`). Out-of-range
  values are `InvalidRange`; unknown kinds are `InvalidEnum`; truncated
  payloads and trailing bytes fail before publication
  (`combat_hit_round_trip_preserves_golden_bytes`,
  `combat_hit_rejects_invalid_range_and_malformed_payload`).

## Remote player despawn (`src/remote_player_despawn.rs`, `tests/runtime_contract.rs`)

- Play packet ID 8 payload is the 16-byte UUIDv4 identity of a remote
  player the authoritative world removed. `PlayerId` stays the single
  identity gate: zero and non-v4 values are `InvalidIdentity`. Truncated
  payloads and trailing bytes fail before publication
  (`remote_player_despawn_round_trip_preserves_golden_bytes`,
  `remote_player_despawn_rejects_invalid_identity_and_malformed_payload`).
- `ByteEncoder` / `ByteDecoder` `i8` helpers use two's-complement
  little-endian encoding, matching the Go primitive.
- `ByteEncoder` / `ByteDecoder` `boolean` helpers copy the Go primitive:
  only 0 and 1 are accepted, and anything else is `InvalidEnum`.

## Item stack (`src/item_stack.rs`, `tests/runtime_contract.rs`)

- `ItemStack` is the fixed 5-byte slot value every inventory-carrying family
  shares: `u16` item, `u8` count, `u16` durability. The empty stack is the
  zero value. This module is the single owner of the registered item table
  the Go side consults for `ItemStack.Valid`: stack limits, tool and armor
  durability maxima, and the smelting input/output whitelists. Families must
  not fork a second copy of those rules.
- Unregistered item numbers are `InvalidEnum`; a non-canonical empty stack, a
  zero or over-limit count, and a durability outside the item budget are
  `InvalidRange`.

## Record arrays and batch headers (`src/batch.rs`, `src/block.rs`)

- `read_fixed` / `write_fixed` encode fixed-count record arrays, and
  `ByteCountBatch` / `UvarintCountBatch` are the two batch headers the
  entity families share: a server tick plus a one-byte or canonical-uvarint
  record count. A zero count and a count above the family's fixed maximum are
  `InvalidRange`; a remaining length that is not exactly `count` records of
  the family stride is `Truncated` before any record is published.
- `src/block.rs` owns registered block numbering, the world vertical span,
  and the chunk-ordered block index that sorted block-change batches compare.

## Block changes (`src/block_changes.rs`, `tests/runtime_contract.rs`)

- Play packet ID 1 payload is the dimension, two chunk coordinates, the
  base and new revision, a canonical uvarint change count, and the
  fixed-stride changes. Zero changes stay legal as a revision barrier for an
  item-only tick; the count is bounded by `MAX_BLOCK_CHANGES` (`4096`).
- The revision transition must be exactly `base + 1` from a non-zero,
  non-saturated base; every change must name a registered block, stay inside
  the world span and the announced chunk, and keep the batch strictly
  increasing by chunk-ordered block index. Failures are `InvalidEnum`,
  `InvalidRange`, or `Truncated` before publication; trailing bytes fail
  after the last change
  (`block_changes_round_trip_preserves_golden_bytes`,
  `block_changes_rejects_invalid_revision_position_and_malformed_payload`).

## Forget chunks (`src/forget_chunks.rs`, `tests/runtime_contract.rs`)

- Play packet ID 2 payload is the dimension, a canonical uvarint chunk
  count, and the fixed-stride chunk coordinates. The count is bounded by
  `MAX_FORGET_CHUNKS` (`4096`), a zero count is `InvalidRange`, duplicate
  coordinates are `InvalidRange`, and a remaining length shorter than the
  declared records is `Truncated` before publication; trailing bytes fail
  after the last coordinate
  (`forget_chunks_round_trip_preserves_golden_bytes`,
  `forget_chunks_rejects_empty_duplicate_and_malformed_payload`).

## Companion identity (`src/entity_id.rs`)

- `CompanionId` is the 16-byte UUIDv4 companion identity the companion
  families share; zero and non-v4 values are `InvalidIdentity`.
  `valid_companion_name` is the companion name rule: the canonical display
  name rule plus a rejection of Unicode whitespace, so a publishable companion
  name never contains an embedded space.

## Companion despawn (`src/companion_despawn.rs`, `tests/runtime_contract.rs`)

- Play packet ID 19 payload is the 16-byte UUIDv4 companion identity the
  authoritative world removed. `InvalidIdentity` rejects zero and non-v4
  values; truncated payloads and trailing bytes fail before publication
  (`companion_despawn_round_trip_preserves_identity_bytes`,
  `companion_despawn_rejects_invalid_identity_and_malformed_payload`).

## Hostile despawn (`src/hostile_despawn.rs`, `tests/runtime_contract.rs`)

- Play packet ID 24 payload is a `u64` server tick, a one-byte record count,
  and the fixed 8-byte hostile IDs. The count is bounded by
  `MAX_HOSTILE_RECORDS` (`64`), a zero count and zero IDs are
  `InvalidRange`, records must be strictly ascending, and a remaining length
  that is not exactly `count` records is `Truncated` before publication;
  trailing bytes fail after the last ID
  (`hostile_despawn_round_trip_preserves_batch_bytes`,
  `hostile_despawn_rejects_unsorted_zero_and_malformed_payload`).

## Projectile despawn (`src/projectile_despawn.rs`, `tests/runtime_contract.rs`)

- Play packet ID 31 payload is a `u64` server tick, a one-byte record count,
  and the fixed 8-byte projectile IDs. The count is bounded by
  `MAX_PROJECTILE_RECORDS` (`128`), a zero count and zero IDs are
  `InvalidRange`, records must be strictly ascending, and a remaining length
  that is not exactly `count` records is `Truncated` before publication
  (`projectile_despawn_round_trip_preserves_batch_bytes`,
  `projectile_despawn_rejects_unsorted_zero_and_malformed_payload`).

## Passive despawn (`src/passive_despawn.rs`, `tests/runtime_contract.rs`)

- Play packet ID 28 payload is a `u64` server tick, a one-byte record count,
  and the fixed 9-byte records of an ID plus the removal reason. The count is
  bounded by `MAX_PASSIVE_RECORDS` (`64`); the only published reasons are
  `PASSIVE_DESPAWN_VANISHED` (`0`) and `PASSIVE_DESPAWN_DIED` (`1`), anything
  else is `InvalidEnum`; records must be strictly ascending and non-zero
  (`passive_despawn_round_trip_preserves_batch_bytes`,
  `passive_despawn_rejects_unsorted_zero_reason_and_malformed_payload`).

## Chest state (`src/chest_state.rs`, `src/batch.rs`, `tests/runtime_contract.rs`)

- Play packet ID 15 payload is the 18-byte chest container reference plus the
  fixed `CHEST_SLOTS` (`27`) item stacks. `read_fixed` decodes the fixed-count
  array, so a truncated payload fails before any slot is published; the chest
  reference must name a chest with a legal slot and generation
  (`chest_state_round_trip_preserves_slot_bytes`,
  `chest_state_rejects_wrong_reference_and_malformed_payload`).

## Focused Verification

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked
```
