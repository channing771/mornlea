# Protocol contracts

`packages/engine/crates/mornlea_protocol` owns versioned framing, negotiation,
and packet-family codecs. It is a windowless rlib. Production code may depend
on `mornlea_domain` and exactly one compression dependency, `zstd`, and must
not depend on `mornlea_storage`, `mornlea_engine`, `mornlea_client`, or
`mornlea_godot`. Direction is enforced by `tests/runtime_contract.rs`
(`production_manifest_depends_only_on_domain` and
`domain_does_not_depend_on_protocol`), which pins the permitted set exactly
rather than checking that it omits a few names.

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
  domain type `mornlea_domain::LookAngles` is the single owner of that
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
- `require_minimum_records` is the budget check for the families whose Go
  decoder rejects a short payload but accepts a long one, leaving the
  remainder to the end-of-payload check. Using the exact-length rule there
  would report a padded batch as truncated rather than as trailing bytes,
  which is a different failure than the Go side publishes. The item drop and
  remote player state batches use it; the mob and companion batches use
  `require_records`.
- `src/block.rs` owns registered block numbering, the world vertical span,
  the chunk-ordered block index that sorted block-change batches compare, and
  `MAX_CHUNK_BLOCK_INDEX`, the exclusive upper bound an item drop's block
  index must stay below.

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
- `valid_display_name` is the plain canonical display-name rule, shared by
  the chat event and the remote player spawn. It is a different rule from the
  login start's, which trims before validating, because the Go side applies
  `NormalizeDisplayName` differently in those two places.
- `CompanionId::NONE` is the absent identity and `from_bytes` the
  unvalidated reader; see "Shared value rules".

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

## Furnace state (`src/furnace_state.rs`, `tests/runtime_contract.rs`)

- Play packet ID 13 payload is the 18-byte furnace reference, the fixed
  input, fuel, and output stacks, a single progress byte, and a
  little-endian `u16` burn time. The reference must name a furnace
  (`validate_furnace`); the timers are bounded by `FURNACE_SMELT_TICKS` and
  `FURNACE_BURN_TICKS`; the input slot only accepts an empty stack or a
  registered smelting input, the fuel slot only an empty stack or coal, and
  the output slot only a fixed smelting product
  (`furnace_state_round_trip_preserves_golden_bytes`,
  `furnace_state_rejects_invalid_slots_timers_and_malformed_payload`).

## Crafting state (`src/crafting_state.rs`, `tests/runtime_contract.rs`)

- Play packet ID 21 payload is the grid size, the fixed nine grid slots, and
  the derived output slot, so the encoding never takes a variable-length
  branch. The only published sizes are `CRAFTING_GRID_SIZE_PERSONAL` (`2`) and
  `CRAFTING_GRID_SIZE_WORKBENCH` (`3`); a personal grid may not carry residue
  beyond its own size (`crafting_state_round_trip_preserves_golden_bytes`,
  `crafting_state_rejects_unknown_size_residue_and_malformed_payload`).

## Inventory state (`src/inventory_state.rs`, `tests/runtime_contract.rs`)

- Play packet ID 10 payload is the selected hotbar byte, the fixed nine
  hotbar slots, and the fixed `BACKPACK_SLOTS` (`27`) backpack slots, which
  makes the payload stride `INVENTORY_STATE_WIRE_BYTES` (`181`). The selected
  index is a domain `HotbarSlot`; every slot is a validated item stack
  (`inventory_state_round_trip_preserves_golden_bytes`,
  `inventory_state_rejects_unknown_selected_and_malformed_payload`).

## Hostile spawn (`src/hostile_spawn.rs`, `tests/runtime_contract.rs`)

- Play packet ID 22 payload is a `u64` server tick, a one-byte record count,
  and the fixed 30-byte spawn records: ID, dimension, position, yaw, health,
  and kind. The count is bounded by `HOSTILE_SPAWN_MAX_RECORDS` (`64`); IDs
  must be non-zero and strictly ascending; only the overworld dimension is
  published; health is `1..=MAX_HEALTH`; and the kind is
  `HOSTILE_KIND_NIGHTWALKER` or `HOSTILE_KIND_BONE_THROWER`
  (`hostile_spawn_round_trip_preserves_batch_bytes`,
  `hostile_spawn_rejects_invalid_records_and_malformed_payload`).

## Hostile state (`src/hostile_state.rs`, `tests/runtime_contract.rs`)

- Play packet ID 23 payload is a `u64` server tick, a one-byte record count,
  and the fixed 38-byte state records: ID, position, velocity, yaw, health,
  and kind. The dimension is not on the wire because a dimension change
  always goes through a despawn/spawn pair. The same count, ordering, health,
  and kind bounds as the spawn batch apply
  (`hostile_state_round_trip_preserves_batch_bytes`,
  `hostile_state_rejects_invalid_records_and_malformed_payload`).

## Player state (`src/player_state.rs`, `tests/runtime_contract.rs`)

- Play packet ID 3 payload is the fixed 93-byte body, survival, and
  world-time record. It is the only family that carries health, oxygen,
  hunger, the day phase offset, weather, season, in-season progress,
  temperature, and armor points, so those value ranges live in this module
  rather than in a shared value module no other family consumes.
- Out-of-range weather, season, and armor values are rejected outright
  instead of being clamped: a wire value outside the authoritative domain is
  a protocol violation. Season progress and temperature have no sub-range
  because they are a whole `u8` and an `i8`.
- The mining block is validated as one unit. An inactive block must be
  entirely empty so a client never has to guess whether a stale target still
  applies; an active block must report progress strictly below the
  requirement, so a completed swing is published as inactive
  (`player_state_round_trip_preserves_golden_bytes`,
  `player_state_rejects_out_of_range_fields_and_malformed_payload`).

## Companion spawn (`src/companion_spawn.rs`, `tests/runtime_contract.rs`)

- Play packet ID 17 payload is the 16-byte companion identity, the
  length-prefixed name, the `u64` tick, the dimension, the position, and the
  yaw and pitch. Only the overworld dimension is published, and the pitch
  stays inside the vertical look range so a wrapped angle is rejected instead
  of being normalized on the client. `COMPANION_SPAWN_MAX_WIRE_BYTES` is the
  fixed payload ceiling
  (`companion_spawn_round_trip_preserves_golden_bytes`,
  `companion_spawn_rejects_invalid_identity_name_and_pose`).

## Companion states (`src/companion_states.rs`, `tests/runtime_contract.rs`)

- Play packet ID 18 payload is a `u64` tick, a canonical uvarint count, and
  the fixed 41-byte records of identity, dimension, position, yaw, pitch, and
  reset. The count is bounded by `MAX_COMPANION_STATES` (`4`), which is the
  companion activity limit; identities are ordered by unsigned byte order
  through `strictly_increasing_ids`, and the name rule does not apply because
  this family carries no name
  (`companion_states_round_trip_preserves_batch_bytes`,
  `companion_states_rejects_unsorted_invalid_and_malformed_payload`).

## Drop identity (`src/drop_id.rs`, `src/item_drop_upserts.rs`, `src/item_drop_removes.rs`, `tests/runtime_contract.rs`)

- `DropId` is the stable identity of one authoritative drop: an `i32`
  dimension, two `i32` chunk coordinates, a `u8` slot, and a `u32`
  generation, in the 17-byte wire order. The slot must fit
  `DROPS_PER_CHUNK` (`32`) and the generation must be non-zero.
- The dimension is deliberately not validated. The Go `DropID.Valid` rule
  checks only the slot range and the generation, so a drop naming an unusual
  dimension is still a publishable identity and must not be rejected by a
  stricter Rust rule.
- `DropId` is a validated newtype, so an out-of-range slot or a zero
  generation cannot be constructed at all. The batch families therefore only
  assert the batch bounds and the identity order, and the identity rejections
  are asserted at `DropId::new`.
- `MAX_ITEM_DROP_BATCH` (`32`) lives with the identity because both drop
  halves describe the same bounded drop set.

## Item drop upserts (`src/item_drop_upserts.rs`, `tests/runtime_contract.rs`)

- Play packet ID 11 payload is a `u64` server tick, a canonical uvarint
  count, and the fixed 26-byte records of identity, `u32` block index, and
  the five-byte item stack. The block index must stay below
  `MAX_CHUNK_BLOCK_INDEX` and every stack goes through the shared `ItemStack`
  rule, so a drop cannot publish a slot value the inventory families reject
  (`item_drop_upserts_round_trip_preserves_batch_bytes`,
  `item_drop_upserts_rejects_invalid_records_and_malformed_payload`).

## Item drop removes (`src/item_drop_removes.rs`, `tests/runtime_contract.rs`)

- Play packet ID 12 payload is a `u64` server tick, a canonical uvarint
  count, and the fixed 17-byte drop identities. The two drop halves are
  separate modules because each has its own wire entry point and packet ID,
  and they share the identity space, the batch ceiling, and the count header
  (`item_drop_removes_round_trip_preserves_batch_bytes`,
  `item_drop_removes_rejects_invalid_ids_and_malformed_payload`).

## Remote player spawn (`src/remote_player_spawn.rs`, `tests/runtime_contract.rs`)

- Play packet ID 7 payload is the 16-byte player identity, the
  length-prefixed display name, the `u64` server tick, the dimension, the
  position, and the yaw and pitch. Unlike a companion or a mob, a remote
  player may appear in either playable dimension because it mirrors a peer
  session. Its name rule is the plain canonical display name, not the
  companion rule that also rejects embedded whitespace, because a player
  display name may legitimately contain spaces
  (`remote_player_spawn_round_trip_preserves_golden_bytes`,
  `remote_player_spawn_rejects_invalid_identity_name_and_pose`).

## Remote player states (`src/remote_player_states.rs`, `tests/runtime_contract.rs`)

- Play packet ID 9 payload is a `u64` server tick, a canonical uvarint
  count, and the fixed 41-byte records of identity, dimension, position,
  yaw, pitch, and reset. The count is bounded by
  `MAX_REMOTE_PLAYER_STATES` (`7`), the peer-session budget, which is a
  different ceiling from the companion activity limit even though the record
  stride is the same.
- This is the only uvarint-count family that also carries a fixed wire
  ceiling, `REMOTE_PLAYER_STATES_MAX_WIRE_BYTES` (`296`), which the Go
  decoder applies before it allocates
  (`remote_player_states_round_trip_preserves_golden_bytes`,
  `remote_player_states_rejects_unsorted_invalid_and_malformed_payload`).

## Passive spawn (`src/passive_spawn.rs`, `tests/runtime_contract.rs`)

- Play packet ID 26 payload is a `u64` server tick, a one-byte record count,
  and the fixed 29-byte spawn records of ID, dimension, position, yaw, and
  health. The record has the same field face as a hostile spawn record minus
  the kind byte, because a passive mob has no category to publish. The count
  is bounded by `MAX_PASSIVE_SPAWN_RECORDS` (`64`), which is the protocol
  budget the decoder accepts, not the smaller live capacity the authority
  converges on
  (`passive_spawn_round_trip_preserves_batch_bytes`,
  `passive_spawn_rejects_invalid_records_and_malformed_payload`).

## Passive state (`src/passive_state.rs`, `tests/runtime_contract.rs`)

- Play packet ID 27 payload is a `u64` server tick, a one-byte record count,
  and the fixed 38-byte state records of ID, position, velocity, yaw, health,
  and the grazing bit. The dimension is not on the wire for the same reason
  as the hostile state batch. Grazing is a transient presentation observation
  that is never persisted, so it is validated as a 0/1 value rather than
  reinterpreted
  (`passive_state_round_trip_preserves_batch_bytes`,
  `passive_state_rejects_invalid_records_and_malformed_payload`).

## Projectile spawn (`src/projectile_spawn.rs`, `tests/runtime_contract.rs`)

- Play packet ID 29 payload is a `u64` server tick, a one-byte record count,
  and the fixed 37-byte spawn records of ID, kind, dimension, position, and
  velocity. The record carries a kind byte because a projectile's behaviour
  differs by what fired it, but no yaw or health: a projectile is a
  point-like transient whose orientation the client derives from its
  velocity. Both playable dimensions are legal because a player's bow works
  in either one, while the kind-by-dimension policy is an authority concern
  this codec does not enforce
  (`projectile_spawn_round_trip_preserves_batch_bytes`,
  `projectile_spawn_rejects_invalid_records_and_malformed_payload`).

## Projectile state (`src/projectile_state.rs`, `tests/runtime_contract.rs`)

- Play packet ID 30 payload is a `u64` server tick, a one-byte record count,
  and the fixed 20-byte state records of ID and position. This is the
  narrowest record in the crate: a projectile's kind, dimension, and velocity
  are fixed for its whole life, so the mirror records them at spawn and the
  state batch only moves the body
  (`projectile_state_round_trip_preserves_batch_bytes`,
  `projectile_state_rejects_invalid_records_and_malformed_payload`).

## Chat event (`src/chat_event.rs`, `tests/runtime_contract.rs`)

- Play packet ID 16 payload is the event ID, the player identity and name,
  the companion identity and name, the kind, the reason, and one text slot.
- The text slot is reused by kind. A companion speech event carries a
  model-generated line bounded by `CHAT_SPEECH_TEXT_MAX_BYTES` (`256`), and
  every other kind restates the player's original command bounded by
  `CHAT_COMMAND_TEXT_MAX_BYTES` (`1024`). The decoder therefore reads the
  kind first and then decides which text to read, and the two fields are
  mutually exclusive on the wire as well as in validation.
- The combination rules are atomic. A speech field on any other kind, a
  command on a speech event, an empty or non-canonical name, an out-of-range
  kind, a reserved rejection reason, or a chat rejection reason on a failed
  task rejects the whole event before any field is applied.
- A format rejection must not leak a companion identity or the command,
  because a malformed command never addressed a companion; a queue-full or
  not-following rejection keeps the same identity and command requirements as
  an acceptance so the player can match the rejection to its command. The
  absent companion identity for those cases is `CompanionId::NONE`
  (`chat_event_round_trip_preserves_golden_bytes`,
  `chat_event_rejects_invalid_kind_combinations_and_malformed_payload`).

## Chunk snapshot (`src/chunk_snapshot.rs`, `tests/runtime_contract.rs`)

- Play packet ID 0 payload is an eight-byte envelope — the declared decoded
  length, then the declared compressed length — followed by a zstd frame
  carrying the logical snapshot: dimension, chunk, revision, a section count,
  and one paletted container per section. This is the only family whose wire
  payload is compressed, and it is the only reason this crate is allowed the
  `zstd` dependency.
- `MAX_COMPRESSED_SNAPSHOT` and `MAX_DECODED_SNAPSHOT` are the two ceilings,
  and both are checked before the frame is touched. Every declared palette and
  word count is checked against the bytes that remain before the matching
  buffer is allocated, so a corrupt count cannot become a large allocation.
- Section containers are a closed set of three kinds. A `Single` section
  carries one block ID, an `Indexed` section a palette plus 4- or 8-bit slots,
  and a `Direct` section 15-bit slots with no palette. A section must not carry
  a field belonging to another kind, every slot must resolve inside the palette
  or the registered block range, and a direct word must not carry bits above
  its 15-bit slot. Rejecting the combination instead of ignoring it keeps a
  partially described section from being silently reinterpreted.
- The section list is the whole ordered column: the count must be exactly
  `SECTIONS_PER_CHUNK` and each section's Y must equal its index. The decoder
  does not repair a missing or reordered section.
- Encoder output is intentionally not byte-identical to the Go encoder and
  must not be "fixed". The Go payload is built by
  `github.com/klauspost/compress/zstd`, a pure-Go zstd implementation whose
  compressed block payload differs from the reference libzstd bound here at
  every compression level, even though the frame header and the trailing
  xxhash-64 content checksum are byte-identical and the total frame length can
  match. A standalone Go program re-encodes the committed fixture exactly, so
  the divergence is exclusively a Rust-versus-Go encoder difference.
- Cross-implementation compatibility is defined at the logical/decode level:
  zstd frames are self-describing, so the Go decoder reads what this encoder
  writes and this decoder reads what the Go encoder wrote. Acceptance is
  semantic round-trip — exact decode of the committed fixture, lossless
  encode/decode, round-trip through the fixture, and rejection without
  implicit repair — never byte-identical re-encode. Do not add an assertion
  that a frame produced here equals a committed fixture's compressed bytes.
  The frame magic, the frame header descriptor, the four-byte content size, and
  the trailing content checksum are pinned separately because those parts are
  identical across the two implementations.
- Intermediate layers (`encode_logical`, `decode_logical`, `decode_envelope`,
  `SnapshotEnvelope::decompress`, `compress_logical`) are public so contract
  tests can prove decode exactness byte for byte and rejection before
  allocation without reaching into private state
  (`chunk_snapshot_round_trip_preserves_golden_bytes`,
  `chunk_snapshot_round_trips_through_committed_fixture`,
  `chunk_snapshot_rejects_malformed_envelope_and_bounds`,
  `chunk_snapshot_rejects_malformed_logical_payload`).

## Shared value rules

- `src/entity_id.rs` owns both the companion identity and the display-name
  rule. `valid_display_name` is the plain canonical rule shared by the chat
  event and the remote player spawn; `valid_companion_name` adds the
  Unicode-whitespace rejection for companion names.
- `CompanionId::NONE` is the absent identity a never-addressed chat event
  carries. It is deliberately unreachable through `CompanionId::new`, which
  rejects the zero value, and `CompanionId::from_bytes` is the unvalidated
  reader decoding needs so the kind-specific validation can decide whether
  the absent form is acceptable.
- `src/chat_command.rs` owns the bounded text rule. `valid_bounded_text`
  takes the bound from the caller so the command and speech slots cannot
  drift apart, and `valid_command_text` binds it to the command bound.
- A family whose Go `Validate` checks fewer fields than the Rust newtype
  enforces is a parity break. Where the Go rule is narrower, as with
  `DropID.Valid` and the dimension, the Rust rule is narrowed to match rather
  than the Go rule being treated as incomplete.

## Focused Verification

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_protocol --test runtime_contract --locked
```
