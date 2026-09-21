# Exact Rust domain corpus adapter contract

> **For agentic workers:** this is the frozen adapter contract consumed by
> nodes 3.1–3.10 in [`03-domain-corpus.md`](03-domain-corpus.md). Implement the
> tables exactly; a source conflict is reported to the controller and is not a
> worker-owned design decision.

**Goal:** execute exactly 376 Go-produced cases through public
`mornlea_domain` APIs and compare independently normalized Rust outcomes.

**Architecture:** shared support owns strict JSON parsing, primitive conversion,
normalization, and comparison. Eight topic modules own closed rule sets. A topic
selects the Go rejection rule from parsed input in the producer's stated
precedence, then verifies that the applicable Rust constructor returns the
mapped `DomainError`; accepted output is read from constructed Rust values.

**Compatibility source:** Go producer code and tests first, then the current
Rust APIs. The input schemas and Go precedence are defined at the cited
`packages/tools/cmd/runtime-oracle` lines. The Rust types and errors are defined
at the cited `packages/engine/crates/mornlea_domain` lines.

## Closed ownership and counts

| Module | Closed input family/rules | Exact cases |
| --- | --- | ---: |
| `identity_text` | all `domain.identity_values` rules | 31 |
| `values` | all `domain.values` rules | 99 |
| `command_control` | all `domain.command_control` rules | 28 |
| `command_inventory` | all `domain.command_inventory` rules | 54 |
| `event_player` | four player/outcome `domain.event` rules | 47 |
| `event_world` | three world `domain.event` rules | 38 |
| `event_inventory` | five inventory `domain.event` rules | 33 |
| `event_people` | six remote-player/companion `domain.event` rules | 46 |
| **Total** | exact single ownership; no catch-all | **376** |

The existing `domain.input/45/session-sequence-arrival` case is not in this
partition. Node 3.1 changes only its manifest consumer from `mornlea_domain` to
`external:runtime-authority`. Its expectation contains authoritative admission,
same-sequence deduplication, inventory/world effects, and reports that
`mornlea_domain::order_commands` cannot produce. The handwritten Rust
`command_order` tests remain the domain ordering proof. Do not execute, copy,
or approximate this case in any topic module. The authoritative producer is
`packages/server/sim/runtime/command_order_oracle_test.go:105-163,340-382,447-577`;
the narrower Rust orderer is
`packages/engine/crates/mornlea_domain/src/input/order.rs:126-177`.

## Shared parsing, normalization, and failure contract

### Strict input boundary

- Every file is one JSON object with required `consumer: string` and
  `rule: string`. The topic accepts only its exact consumer and listed rules.
  Hard `DispatchError::InvalidCase` is limited to schema/syntax failures:
  unknown consumers/rules, duplicate keys, malformed JSON, trailing content,
  missing rule-required fields, wrong JSON types, values outside a field's
  declared integer width, wrong fixed-array lengths unless a table explicitly
  maps that length to a normalized rule, malformed float tokens, and malformed
  fixed-width identity text. These are never normalized as `kind: "error"`.
- A value that is valid for the declared input field but outside a semantic
  enum or union is not a hard failure when the Go classifier names a normalized
  rejection. Select that category/rule from raw input before construction even
  when a closed Rust enum cannot represent the bad value. This applies only to
  the table-reachable unknown dimensions, container kinds, stack views,
  crafting sizes, registered rejection reasons, non-container views carrying a
  reference, and raw batch-count cases listed below; an explicitly unreachable
  classifier such as `domain.values` `container_ref.known_kind` remains hard.
- A `u64` accepted field is a base-10 string. A `u32`, `u16`, `u8`, `i32`, or
  `i8` accepted field is a JSON number. A `f32` accepted field is the exact
  eight-lowercase-hex-digit `f32::to_bits()` value, including negative zero.
  `parse_f32_token` accepts ordinary decimal text plus exactly `NaN`, `Inf`,
  `+Inf`, and `-Inf`; other non-decimal spellings are hard failures. The
  unprefixed `Inf` form is required by two frozen control-command cases. A
  decimal token that overflows or underflows with a parser error is also hard;
  only the four explicit spellings may intentionally produce non-finite bits.
- An identity input is exactly 32 lowercase hexadecimal characters decoded to
  `[u8; 16]`. Wrong length, non-hex, or uppercase is a hard failure. A decoded
  zero/non-v4/non-RFC-4122 value reaches `PlayerId::try_from_bytes` or
  `CompanionId::try_from_bytes` and is a normalized semantic rejection.
- Ordered input arrays are never sorted for output. Position vectors are
  `[x,y,z]`; chunk positions are `[x,z]`. Section, palette, packed-word,
  change, chunk, inventory-slot, and state-batch order is preserved exactly.
- Optionality is rule-specific below. A field ignored by the selected rule does
  not become an implicit input to another constructor.

The representative strict Go decoders are
`domain_identity_values_test.go:604-615`,
`domain_values_test.go:712-724`,
`domain_command_control_test.go:897-909`, and the event decoders at
`domain_event_player_test.go:1001-1013`,
`domain_event_world_test.go:1140-1152`,
`domain_event_inventory_test.go:1127-1139`, and
`domain_event_people_test.go:1353-1365`.

### Outcome construction and comparison

- Accepted output is exactly
  `{"kind":"ok","category":<table category>,"fields":<table fields>}`.
  Construct fields from Rust getters or an exhaustively matched Rust enum;
  never read the expectation in an executor.
- Rejected output is exactly
  `{"kind":"error","category":<mapped category>,"fields":{<the rule's
  parsed semantic projection>,"rule":<exact table rule>}}`. The projection is
  built from parsed input because a rejected Rust value has no getters.
- The Go command producers append codec-only `fields.wire` after a production
  round trip (`domain_command_control_test.go:646-666` and
  `domain_command_inventory_test.go:770-790`). For an accepted
  `domain.command_control` or `domain.command_inventory` expectation, the
  shared comparator removes exactly `fields.wire` from the expected clone.
  It removes no field from an error, no field from actual output, and no other
  accepted field. No domain executor implements a Go wire encoder.
- Semantic error selection is input-aware. Apply each row's listed precedence
  before mapping a Rust error. One `DomainError` may map to multiple rules; the
  field and stage select the exact rule. A closed Rust enum or fixed array that
  cannot represent the rejected raw value uses the explicitly named
  pre-construction classifier. No generic `DomainError -> rule` switch is
  sufficient.
- `FiniteVec3::try_new` currently returns
  `DomainError::NonFiniteRotation`
  (`mornlea_domain/src/values.rs:43-57`). After node 4.1 it returns
  `DomainError::NonFiniteValue` while `LookAngles::try_new` retains
  `NonFiniteRotation` (`plans/04-domain-bounds.md:49-61`). Every table entry
  marked **vector error** accepts the current variant before 4.1 and requires
  `NonFiniteValue` after 4.1; its normalized category/rule never changes.
- For every sequenced command, construct the payload variant, then
  `CommandEnvelope::try_new(CommandEnvelopeParts { tick: 0, session: 0,
  sequence, arrival_index: 0, command })`. These synthetic values are absent
  from normalized output. `chat-intent` is `CommandText -> ChatIntent` and is
  not a `Command` variant. See `input/control.rs:165-199`,
  `input/order.rs:22-92`, and `input/chat.rs:13-33`.

## `identity_text`: 31 cases

Input schema: `uuid: string` defaults to `""` when absent; `name?: string` and
`text?: string` preserve absent versus explicit empty. The selected rule
requires the field named below. Source: Go schema/routing/precedence at
`domain_identity_values_test.go:112-126,312-334,469-546`; Rust APIs at
`mornlea_domain/src/identity.rs:141-195` and `src/text.rs:94-167`.

| Rule (cases) | Required input and Rust API chain | Accepted category and exact fields | Normalized rejection precedence and mapping |
| --- | --- | --- | --- |
| `player-id` (4) | `uuid: string` -> strict hex decoder -> `PlayerId::try_from_bytes([u8;16])` | `player-id`; `uuid: string` from `PlayerId::bytes`, lowercase hex | `invalid-identity`: `player_id.nonzero` -> `InvalidIdentity`; then `.version_v4` -> `InvalidIdentity`; then `.variant_rfc4122` -> `InvalidIdentity`. Raw byte tests select the suffix in that order. |
| `display-name` (12) | required `name: string` -> `DisplayName::try_from_canonical(String)` | `display-name`; `name: string` | `invalid-value`, all `InvalidText`, in order: `display_name.utf8`, `.length_range`, `.control`, `.canonical_trim`. Invalid UTF-8 cannot occur in a Rust `String` or frozen JSON but remains the closed producer rule. |
| `companion-name` (5) | required `name: string` -> `CompanionName::try_from_canonical(String)` | `companion-name`; `name: string` | `invalid-value`, all `InvalidText`: first the four `display_name.*` rules above, then `companion_name.embedded_whitespace`. |
| `command-text` (5) | required `text: string` -> `CommandText::try_from_canonical(String)` | `command-text`; `text: string` | `invalid-value`, all `InvalidText`, in order: `command_text.byte_range`, `.utf8`, `.untrimmed`, `.control`. |
| `speech-text` (5) | required `text: string` -> `SpeechText::try_from_canonical(String)` | `speech-text`; `text: string` | `invalid-value`, all `InvalidText`, in order: `speech_text.byte_range`, `.utf8`, `.untrimmed`, `.control`. |

## `values`: 99 cases

Input schema is `item?: int`, `count?: int`, `durability?: int`,
`dimension?: i32`, `chunk_x?: i32`, `chunk_z?: i32`, `slot?: int`,
`generation?: i64`, `kind: string` (absent string becomes `""`). Source: Go
schema/routing/normalization/precedence at
`domain_values_test.go:93-112,365-535,545-653`; Rust APIs at
`mornlea_domain/src/items.rs:58-179` and `src/locations.rs:88-215`.

| Rule (cases) | Required/defaulted input and Rust API chain | Accepted category and exact fields | Normalized rejection precedence and mapping |
| --- | --- | --- | --- |
| `item-table` (67) | required `item`, checked `u16`; call `item_stack_limit(item)`, `durability_max(item)`, `smelting_output(item)`, `is_smelting_product(item)` | `item-table`; always `item`, `registered`, `smelting_product`; add `stack_limit` only when registered, `durability_max` only when durable, `smelting_output` only when smeltable | No normalized rejection. Missing/out-of-`u16` item is hard `InvalidCase`. |
| `item-stack` (18) | required `item: u16`; absent `count` -> `0`, absent `durability` -> `0`; checked `u8`/`u16`; `ItemStack::try_new` | `item-stack`; `item`, `count`, `durability` | In order: `invalid-value/item_stack.absent_item_count` -> `InvalidCount`; `invalid-value/item_stack.absent_item_durability` -> `InvalidDurability`; `invalid-enum/item_stack.registered_item` -> `InvalidItem`; `invalid-value/item_stack.count_range` -> `InvalidCount`; `invalid-value/item_stack.durability_range` -> `InvalidDurability`; `invalid-value/item_stack.nondurable_durability` -> `InvalidDurability`. |
| `drop-id` (7) | required `dimension:i32`, `chunk_x:i32`, `chunk_z:i32`, `slot:u8`; absent `generation` -> `0`, checked `u32`; `ChunkPos::new` -> `DropId::try_new` | `drop-id`; `dimension`, `chunk_x`, `chunk_z`, `slot`, `generation` | `invalid-value/drop_id.slot_range` -> `InvalidDropSlot`; then `invalid-value/drop_id.generation_nonzero` -> `InvalidDropGeneration`. Dimension is deliberately unchecked. |
| `container-ref` (7) | required chunks and `slot:u8`; absent/blank/case- or space-normalized `kind` -> furnace, `chest` -> chest; absent generation -> `0`; input `dimension` is ignored; `ChunkPos::new` -> `ContainerRef::try_new` | `container-ref`; `kind` (`furnace`/`chest`), `chunk_x`, `chunk_z`, `slot`, constant `dimension:0`, `generation` | `invalid-value/container_ref.furnace_slot_range` or `.chest_slot_range` -> `InvalidContainerSlot` by kind; then `invalid-value/container_ref.generation_nonzero` -> `InvalidContainerGeneration`. Classifier-only `invalid-enum/container_ref.known_kind` is unreachable and has no frozen row: `domainValuesContainerKind` rejects unknown text as hard `InvalidCase` before classification (`domain_values_test.go:642-652`); do not fabricate a normalized result. |

## `command_control`: 28 cases

All payload fields are optional in the shared JSON struct but every field named
by a row is required; explicit zero/false remains a value. `yaw`/`pitch` are
float-token strings. Source: Go schema/routing/normalization/precedence at
`domain_command_control_test.go:106-134,404-643,727-837,846-892`; Rust APIs at
`mornlea_domain/src/input/control.rs:18-199`, `src/values.rs:24-92`, and
`src/input/order.rs:22-92`.

Every accepted row below has the listed semantic fields after the shared
comparator removes only the Go expectation's `fields.wire`.

| Rule (cases) | Required input and Rust API chain | Accepted category and exact fields | Normalized rejection precedence and mapping |
| --- | --- | --- | --- |
| `player-input` (9) | `sequence:u64`, `move_x:i8`, `move_z:i8`, `jump:bool`, `yaw/pitch:f32 token`, `mining/eating/sprinting/sneaking:bool`; `LookAngles::try_new` -> `PlayerControl::new(PlayerControlParts { Movement, HeldActions })` -> `Command::PlayerInput` -> envelope | `player-input`; `sequence`, `move_x`, `move_z`, `jump`, `yaw`, `pitch`, `mining`, `eating`, `sprinting`, `sneaking` | `invalid-value/player_input.finite_rotation` -> `NonFiniteRotation`. |
| `place-block` (4) | `sequence`, `yaw`, `pitch`, `slot:u8`; `LookAngles::try_new` -> `PlacementIntent::try_new` -> `Command::PlaceBlock` -> envelope | `place-block`; `sequence`, `yaw`, `pitch`, `slot` | `invalid-value/place_block.finite_rotation` -> `NonFiniteRotation`; then `invalid-value/place_block.slot_range` -> `InvalidHotbarSlot`. |
| `select-hotbar` (2) | `sequence`, `slot:u8`; `HotbarSlot::new` -> `Command::SelectHotbar` -> envelope | `select-hotbar`; `sequence`, `slot` | `invalid-value/select_hotbar.slot_range` -> `InvalidHotbarSlot`. |
| `chunk-resync` (3) | `sequence`, `dimension:i32`, `chunk_x:i32`, `chunk_z:i32`, `have_revision:u64`; semantic dimension classification, then checked `u8` for admitted `0` or `1`; `ChunkPos::new` -> `ResyncIntent::try_new` -> `Command::Resync` -> envelope | `chunk-resync`; `sequence`, `dimension`, `chunk_x`, `chunk_z`, `have_revision` | `invalid-enum/chunk_resync.dimension`: fitting unknown `u8` -> `ResyncIntent::try_new`/`Dimension::new` returns `InvalidDimension`; negative or `>255` remains valid `i32` input and uses the same normalized rule through the pre-construction classifier, never a hard failure. |
| `till-soil` (2) | `sequence`, `yaw`, `pitch`; `LookAngles::try_new` -> `Command::TillSoil` -> envelope | `till-soil`; `sequence`, `yaw`, `pitch` | `invalid-value/till-soil.finite_rotation` -> `NonFiniteRotation`. |
| `bone-meal` (2) | same payload; `LookAngles::try_new` -> `Command::BoneMeal` -> envelope | `bone-meal`; `sequence`, `yaw`, `pitch` | `invalid-value/bone-meal.finite_rotation` -> `NonFiniteRotation`. |
| `collect-water` (2) | same payload; `LookAngles::try_new` -> `Command::CollectWater` -> envelope | `collect-water`; `sequence`, `yaw`, `pitch` | `invalid-value/collect-water.finite_rotation` -> `NonFiniteRotation`. |
| `place-water` (2) | same payload; `LookAngles::try_new` -> `Command::PlaceWater` -> envelope | `place-water`; `sequence`, `yaw`, `pitch` | `invalid-value/place-water.finite_rotation` -> `NonFiniteRotation`. |
| `open-container` (2) | same payload; `LookAngles::try_new` -> `Command::OpenContainer` -> envelope | `open-container`; `sequence`, `yaw`, `pitch` | `invalid-value/open-container.finite_rotation` -> `NonFiniteRotation`. |

## `command_inventory`: 54 cases

Input fields are `sequence?:u64`, `from?:u8`, `to?:u8`, `slot?:u8`,
`view?:u8`, `single?:bool`, `kind?:string`, `chunk_x?:i32`,
`chunk_z?:i32`, `container_slot?:u8`, `generation?:u32`, and `text?:string`.
Every field named by the selected row is required. A reference is absent only
when `kind` is absent; a present `kind` requires both chunks, container slot,
and generation. Source: Go schema/routing/normalization/precedence at
`domain_command_inventory_test.go:113-140,522-767,855-925,941-1107,1109-1193`;
Rust APIs at `mornlea_domain/src/input/inventory.rs:42-290`,
`src/locations.rs:148-215`, `src/input/chat.rs:13-33`, and
`src/input/order.rs:22-92`.

For view-bearing accepted output, the common fields are `view`, `chunk_x`,
`chunk_z`, `dimension`, `kind`, `container_slot`, and `generation`. Inventory
or crafting view emits the deterministic Go sentinel
`chunk_x:0, chunk_z:0, dimension:0, kind:"furnace", container_slot:0,
generation:0`; container view emits its `ContainerRef` getters and constant
`dimension:0`. This is normalization only; do not construct a zero Rust
`ContainerRef`.

| Rule (cases) | Required input and Rust API chain | Accepted category and exact fields | Normalized rejection precedence and mapping |
| --- | --- | --- | --- |
| `move-inventory` (5) | `sequence`, `from`, `to`; `InventoryMove::try_new` -> `Command::MoveInventory` -> envelope | `move-inventory`; `sequence`, `from`, `to` | `invalid-value/move_inventory.slot_range` -> `InvalidSlot`; then `.same_slot` -> `SourceEqualsTarget`. |
| `move-crafting` (5) | `sequence`, `from`, `to`; `CraftingMove::try_new` -> `Command::MoveCrafting` -> envelope | `move-crafting`; `sequence`, `from`, `to` | `invalid-value/move_crafting.slot_range` -> `InvalidSlot`; then `.same_slot` -> `SourceEqualsTarget`; then `.both_ends_in_inventory` -> `CraftingMoveInsideInventory`. |
| `move-container` (10) | `sequence`, present reference, `from`, `to`; raw kind -> `ContainerKind`; `ChunkPos::new` -> `ContainerMove::try_new(chunk,kind,slot,generation,from,to)` -> `Command::MoveContainer` -> envelope | `move-container`; `sequence`, flattened reference fields, `from`, `to` | Reference first: `invalid-enum/container_ref.kind` -> pre-construction classifier (closed Rust enum); `invalid-value/container_ref.slot_range` -> `InvalidContainerSlot`; `.generation` -> `InvalidContainerGeneration`. Then `invalid-value/move_container.same_slot` -> `SourceEqualsTarget`; `.slot_range` -> `InvalidSlot`; `.furnace_output_target` -> `FurnaceOutputAsTarget`. Classifier-only `container_ref.dimension` is unreachable because input has no dimension. |
| `close-container` (1) | `sequence`; `Command::CloseContainer` -> envelope | `close-container`; `sequence` | No normalized rejection. |
| `drop-selected-item` (1) | `sequence`; `Command::DropSelectedItem` -> envelope | `drop-selected-item`; `sequence` | No normalized rejection. |
| `take-crafting-output` (2) | `sequence`; `Command::TakeCraftingOutput` -> envelope | `take-crafting-output`; `sequence` | `invalid-value/take_crafting_output.zero_sequence` -> `CommandEnvelope::try_new` returns `InvalidSequence`. |
| `equip-armor` (1) | `sequence`; `Command::EquipArmor` -> envelope | `equip-armor`; `sequence` | No normalized rejection. |
| `move-partial` (12) | `sequence`, `view`, reference shape, `from`, `to`, `single`; raw shape -> `StackView`; `PartialMove::try_new` -> `Command::MovePartial` -> envelope | `move-partial`; `sequence`, common view fields, `from`, `to`, `single` | Shape first: `invalid-enum/stack_split.view`, `invalid-value/stack_split.inventory_view_carries_container`, and `.crafting_view_carries_container` -> pre-construction classifiers. Container view then maps `container_ref.kind` pre-construction, `.slot_range` -> `InvalidContainerSlot`, `.generation` -> `InvalidContainerGeneration`. Then `invalid-value/stack_split.slot_range` -> `InvalidSlot`; finally `invalid-value/move_stack_partial.same_slot` -> `SourceEqualsTarget`. |
| `quick-move` (6) | `sequence`, `view`, reference shape, input key `from`; raw shape -> `StackView`; `StackSource::try_new(view,from)` -> `Command::QuickMove` -> envelope | `quick-move`; `sequence`, common view fields, output field `slot` (not `from`) | Same shape/reference precedence as `move-partial`, then `invalid-value/stack_split.slot_range` -> `InvalidSlot`; no same-slot rule. |
| `drop-stack` (5) | `sequence`, `view`, reference shape, `slot`; raw shape -> `StackView`; `StackSource::try_new` -> `Command::DropStack` -> envelope | `drop-stack`; `sequence`, common view fields, `slot` | Same shape/reference precedence as `quick-move`, then `invalid-value/stack_split.slot_range` -> `InvalidSlot`. |
| `chat-intent` (6) | required `text`; `CommandText::try_from_canonical` -> `ChatIntent::new`; no envelope | `chat-intent`; `text` | `invalid-value/chat_command.text_bounds` -> `InvalidText`; then `invalid-value/chat_command.control_character` -> `InvalidText`. |

## `event_player`: 47 cases

Source: Go input/routing/construction/normalization/precedence at
`domain_event_player_test.go:116-163,487-581,607-778,818-888`; Rust APIs at
`mornlea_domain/src/event/player.rs:43-498`, `src/event/outcome.rs:15-205`, and
`src/values.rs:1-92`.

| Rule (cases) | Required input and Rust API chain | Accepted category and exact fields | Normalized rejection precedence and mapping |
| --- | --- | --- | --- |
| `player-state` (21) | Required: `server_tick:u64`, `last_input_sequence:u64`, `dimension:i32`, `position:[f32 token;3]`, `velocity:[f32 token;3]`, `yaw/pitch:f32 token`, `on_ground/ready/reset/mining_active/mining_harvestable/saturation_zero:bool`, `mining_target:[i32;3]`, `mining_progress/mining_required/oxygen/day_phase_offset:u16`, `health/hunger/weather_kind/season/season_progress/armor_points:u8`, `world_time_ticks:u64`, `temperature:i8`. Classify raw dimension, convert admitted `0` or `1` to `u8`, then checked chain: `Dimension::new`; two `FiniteVec3::try_new`; `LookAngles::try_new`; `MotionState::new`; `MiningState::try_new`; `SurvivalState::try_new`; `Weather::try_new`; `Season::try_new`; `WorldState::try_new`; **`PlayerState::new(PlayerStateParts)`** (not `try_new`). | `player-state`; `server_tick`, `last_input_sequence`, `dimension`, `position`, `velocity`, `yaw`, `pitch`, `on_ground`, `ready`, `reset`, `mining_active`, `mining_target`, `mining_progress`, `mining_required`, `mining_harvestable`, `health`, `oxygen`, `hunger`, `saturation_zero`, `day_phase_offset`, `world_time_ticks`, `weather_kind`, `season`, `season_progress`, `temperature`, `armor_points`. Both vectors and target preserve XYZ order. | Exact Go precedence: `invalid-enum/player_state.dimension`: fitting unknown `u8` -> `InvalidDimension`, negative or `>255` -> the same pre-construction normalized rule; `invalid-value/.finite_position` -> vector error; `.finite_velocity` -> vector error; `.finite_rotation` -> `NonFiniteRotation`; `.health_range`, `.oxygen_range`, `.hunger_range` -> `InvalidSurvivalValue` selected by raw field; `.day_phase_offset_range` -> `InvalidDayPhaseOffset`; `invalid-enum/.weather_kind` -> `InvalidWeather`; `invalid-enum/.season` -> `InvalidSeason`; `invalid-value/.armor_points_range` -> `InvalidSurvivalValue`; `.mining_inactive_residue` then `.mining_progress_range` -> `InvalidMiningState`. Use this raw precedence to disambiguate shared variants even though aggregate construction groups fields differently. |
| `command-rejected` (17) | required `sequence:u64`, `reason:string`; closed raw match to one of `invalid_ray`, `no_target`, `chunk_not_ready`, `protected_block`, `invalid_block`, `occupied`, `invalid_input`, `player_not_ready`, `invalid_slot`, `hotbar_full`, `drop_capacity`, `container_capacity`, `not_fluid_source`, `bucket_mismatch`, `not_armor`; then `CommandRejection::new` | `command-rejected`; `sequence`, `reason` | `invalid-enum/command_rejected.registered_reason` -> pre-construction classifier; Rust exposes no raw-string `RejectReason` parser. Zero sequence is accepted. |
| `place-block-succeeded` (2) | required `sequence:u64` -> `PlacementSuccess::new` | `place-block-succeeded`; `sequence` | No normalized rejection; zero is accepted. |
| `combat-hit` (7) | required `server_tick:u64`, checked `damage:u8`, checked `target_kind:u8`; `CombatTarget::try_new` -> `CombatHit::try_new` | `combat-hit`; `server_tick`, `damage`, `target_kind` | Go precedence is `invalid-value/combat_hit.server_tick_nonzero` -> `InvalidCombatHit`; then `.damage_range` -> `InvalidCombatHit`; then `invalid-enum/.target_kind` -> `InvalidCombatTarget`. Select tick/damage from raw input before attempting the closed target when inputs are multiply invalid. |

## `event_world`: 38 cases

Input schema and absent/empty batch distinction are at
`domain_event_world_test.go:135-219`; routing, builders, normalization, and
precedence are at `:620-815,842-1047`. Rust APIs are
`mornlea_domain/src/sections.rs:137-269` and
`src/event/world.rs:27-249`.

| Rule (cases) | Required input and Rust API chain | Accepted category and exact fields | Normalized rejection precedence and mapping |
| --- | --- | --- | --- |
| `chunk-snapshot` (19) | Required `dimension:i32`, `chunk:[i32;2]`, `revision:u64`, explicit `sections` array. Each section has `storage:int`, `single?:int`, `bits?:int`, `palette:[int]`, `packed:[u64]`; storage `0` requires `single:u16`, `1` requires `bits:u8` plus non-null palette, `2` is direct-15. Classify dimension then convert admitted `0` or `1`; `Dimension::new` -> `ChunkPos::new`; each `PalettedSection::{single,indexed,direct}`; exact conversion to `Box<[PalettedSection;24]>`; `ChunkSnapshot::try_new`. Unknown storage has no Go normalized rule and is a schema `InvalidCase`. | `chunk-snapshot`; `dimension`, `chunk`, `revision`, `sections`. Section order preserved. Single fields: `storage:"single"`, `single`; indexed: `storage:"indexed"`, `bits`, ordered `palette`, ordered decimal-string `packed`; direct: `storage:"direct"`, ordered decimal-string `packed`. | Exact order: `invalid-enum/chunk_snapshot.dimension`: fitting unknown `u8` -> `InvalidDimension`, other unknown `i32` -> same pre-construction normalized rule; `invalid-value/.revision_nonzero` -> `InvalidRevision`; `.section_count` -> pre-construction fixed-array classifier; then first section in input order: `invalid-enum/.section_single_registered` -> `InvalidBlock`; `.section_indexed_bits` -> `InvalidSectionBits`; `invalid-value/.section_palette_bounds` -> `InvalidSectionPalette`; `invalid-enum/.section_palette_registered` -> `InvalidBlock`; `invalid-enum/.section_palette_unique` -> `InvalidSectionPalette`; `invalid-value/.section_words_exact` -> `InvalidSectionWords`; `invalid-enum/.section_slot_in_palette` -> `InvalidSectionSlot`; `invalid-value/.section_direct_high_bits` -> `InvalidSectionHighBits`; `invalid-enum/.section_direct_block_registered` -> `InvalidBlock`. The raw precedence decides multiply-invalid diagnostics; constructors decide admission. |
| `block-changes` (14) | Required `dimension:i32`, `chunk:[i32;2]`, `base_revision:u64`, `new_revision:u64`, explicit ordered `changes:[{position:[i32;3],block:u16}]`; empty is accepted. Classify dimension, convert admitted `0` or `1`; `Dimension::new` -> `ChunkPos::new`; per row `BlockPos::new` -> `BlockChange::try_new`; ordered boxed slice -> `BlockChanges::try_new`. | `block-changes`; `dimension`, `chunk`, `base_revision`, `new_revision`, ordered `changes`, each with XYZ `position` and `block`. | `invalid-enum/block_changes.dimension`: fitting unknown -> `InvalidDimension`, other unknown `i32` -> pre-construction normalized rule; `invalid-value/.revision_transition` -> `InvalidRevision`; for each change in order `invalid-enum/.block_registered` -> `InvalidBlock`, `invalid-value/.position_y_span` -> `InvalidBlockY`, `.position_chunk` -> `InvalidBlockChunk`, `.strictly_increasing_index` -> `InvalidChangeOrder`. |
| `forget-chunks` (5) | Required `dimension:i32`, explicit `chunks:[[i32;2]]`; classify dimension, convert admitted `0` or `1`; `Dimension::new`; ordered `ChunkPos::new` list -> `ForgetChunks::try_new`. | `forget-chunks`; `dimension`, ordered `chunks` | `invalid-enum/forget_chunks.dimension`: fitting unknown -> `InvalidDimension`, other unknown `i32` -> pre-construction normalized rule; then `invalid-value/.nonempty_positions` -> `InvalidForgetChunks`; then `.unique_positions` -> `InvalidForgetChunks`. Preserve submitted order; the constructor's uniqueness copy is not output. |

## `event_inventory`: 33 cases

Common stack input is `{item:int,count:int,durability:int}` checked to
`u16/u8/u16` then passed to `ItemStack::try_new`. Common container input is
`{kind:int,chunk:[i32;2],slot:int,generation:u32}`; raw `0` means furnace and
`1` chest, with no dimension field. Source: Go schema/builders/normalization/
precedence at `domain_event_inventory_test.go:154-210,547-790,858-927,959-1098`;
Rust APIs at `mornlea_domain/src/event/inventory.rs:66-361`,
`src/items.rs:106-179`, and `src/locations.rs:148-215`.

Fixed-array length mismatch, missing required arrays/objects, or primitive width
failure is hard `InvalidCase`. Error precedence is the listed raw-field order;
this is required when a closed enum or an earlier outer rule cannot be reached
through the natural nested-constructor order.

| Rule (cases) | Required/defaulted input and Rust API chain | Accepted category and exact fields | Normalized rejection precedence and mapping |
| --- | --- | --- | --- |
| `inventory-state` (3) | required `selected:u8`, exact `hotbar[9]`, exact `backpack[27]`; `HotbarSlot::new`; each `ItemStack::try_new`; fixed arrays -> `InventoryState::new` | `inventory-state`; `selected`, ordered `hotbar`, ordered `backpack`; every stack has `item`, `count`, `durability` | `invalid-value/inventory_state.selected_range` -> `InvalidHotbarSlot`; then each `inventory_state.hotbar_slot_<i>` and `backpack_slot_<i>` in order: `InvalidItem` -> `invalid-enum`, `InvalidCount` or `InvalidDurability` -> `invalid-value`. |
| `crafting-state` (4) | required `size:u8`, exact `slots[9]`, required `output`; raw `2` -> `CraftingSize::Personal`, `3` -> `Workbench`; stacks -> fixed array -> `CraftingState::try_new` | `crafting-state`; semantic size `"personal"` or `"workbench"`, ordered `slots`, `output`; stacks have all three fields | `invalid-enum/crafting_state.size` -> pre-construction closed-enum classifier; then each `crafting_state.slot_<i>` (`InvalidItem` -> `invalid-enum`, `InvalidCount`/`InvalidDurability` -> `invalid-value`); `invalid-value/crafting_state.personal_residue` -> `InvalidCraftingResidue` at the first nonempty residue; then `crafting_state.output` with the same stack-category mapping. |
| `furnace-state` (19) | required container, `progress_ticks:u8`, `burn_ticks:u16`; `input?`, `fuel?`, `output?` default to `ItemStack::EMPTY` when absent. Raw kind -> `ContainerKind`; `ChunkPos::new` -> `ContainerRef::try_new`; three `ItemStack::try_new`; `FurnaceState::try_new`. | `furnace-state`; `container:{kind,chunk,slot,generation}`, `input`, `fuel`, `output`, `progress_ticks`, `burn_ticks` | Precedence: `invalid-enum/furnace_state.ref_kind`: known chest -> `InvalidContainerKind`, unknown raw byte -> pre-construction classifier; `invalid-value/.ref_slot` -> `InvalidContainerSlot`; `.ref_generation` -> `InvalidContainerGeneration`; `.progress_range` then `.burn_range` -> `InvalidFurnaceTimers` selected by raw timer; `.input`, `.fuel`, `.output`: nested `InvalidItem` -> `invalid-enum`, nested `InvalidCount`/`InvalidDurability` -> `invalid-value`, or outer whitelist `InvalidFurnaceSlot` -> `invalid-value`, with the field selecting the rule. |
| `chest-state` (4) | required container and exact `items[27]`; raw kind -> `ContainerKind`; `ContainerRef::try_new`; items -> fixed array; `ChestState::try_new` | `chest-state`; `container`, ordered `items`; every stack has all three fields | `invalid-enum/chest_state.ref_kind`: known furnace -> `InvalidContainerKind`, unknown raw byte -> pre-construction classifier; `invalid-value/.ref_slot` -> `InvalidContainerSlot`; `.ref_generation` -> `InvalidContainerGeneration`; then `chest_state.item_<i>` in order with `InvalidItem` -> `invalid-enum`, `InvalidCount`/`InvalidDurability` -> `invalid-value`. |
| `container-closed` (3) | required container; raw kind -> `ContainerKind`; `ChunkPos::new` -> `ContainerRef::try_new` -> `ContainerClosed::new` | `container-closed`; `container:{kind,chunk,slot,generation}` | `invalid-enum/container_closed.ref_kind` -> pre-construction classifier for unknown raw byte; then `invalid-value/.ref_slot` -> `InvalidContainerSlot`; `.ref_generation` -> `InvalidContainerGeneration`. Both known kinds are accepted and `ContainerClosed::new` is total. |

## `event_people`: 46 cases

Input schema and record shapes are at
`domain_event_people_test.go:160-220`; routing/builders/normalization/precedence
are at `:696-950,976-1212`. Rust APIs are
`mornlea_domain/src/event/people.rs:57-489`,
`src/identity.rs:141-195`, `src/text.rs:94-134`, and `src/values.rs:1-92`.

All positions are exact three-token XYZ arrays; all yaw/pitch values are float
tokens. A batch output keeps submitted record order. Do not sort in the adapter:
the Rust batch constructor verifies that submitted IDs are already strictly
increasing.

| Rule (cases) | Required input and Rust API chain | Accepted category and exact fields | Normalized rejection precedence and mapping |
| --- | --- | --- | --- |
| `remote-player-spawn` (10) | required `player_id`, `display_name`, `server_tick:u64`, `dimension:i32`, `position`, `yaw`, `pitch`; strict UUID decode; `DisplayName::try_from_canonical` -> `PlayerId::try_from_bytes`; classify dimension and convert admitted `0` or `1`; `Dimension::new` -> `FiniteVec3::try_new` -> `LookAngles::try_new` -> `RemotePlayerSpawn::new` | `remote-player-spawn`; `player_id`, `display_name`, `server_tick`, `dimension`, XYZ `position`, `look:{yaw,pitch}` | Go order: `invalid-value/remote_player_spawn.display_name.{utf8,length_range,control,canonical_trim}` -> `InvalidText`; `invalid-identity/remote_player_spawn.player_id.{nonzero,version_v4,variant_rfc4122}` -> `InvalidIdentity`; `invalid-enum/.dimension`: fitting unknown -> `InvalidDimension`, other unknown `i32` -> pre-construction normalized rule; `invalid-value/.position_finite` -> vector error; `.rotation_finite` -> `NonFiniteRotation`. |
| `remote-player-despawn` (2) | required `player_id`; strict decode -> `PlayerId::try_from_bytes` -> `RemotePlayerDespawn::new` | `remote-player-despawn`; `player_id` | `invalid-identity/player_id.{nonzero,version_v4,variant_rfc4122}` -> `InvalidIdentity` in that order. |
| `remote-player-states` (10) | required `server_tick:u64`, explicit non-null `players`; each record requires `player_id`, `dimension:i32`, `position`, `yaw`, `pitch`, `reset:bool`; decode -> `PlayerId`; classify and convert dimension -> `Dimension`; `FiniteVec3` -> `LookAngles` -> `RemotePlayerState::new`; ordered boxed slice -> `RemotePlayerStates::try_new`. | `remote-player-states`; `server_tick`, ordered `states`; each state has `player_id`, `dimension`, XYZ `position`, `look:{yaw,pitch}`, `reset` | `invalid-value/remote_player_states.count_range`: empty -> `EmptyStateBatch`. The Go upper arm `>7` is a protocol cap the domain explicitly does not adopt; no frozen over-max case exists. A future over-max corpus row is a contract conflict to escalate, not a rejection this adapter fabricates. Then for each index in input order: `invalid-identity/remote_player_states.player_<i>.player_id.{nonzero,version_v4,variant_rfc4122}` -> `InvalidIdentity`; `invalid-enum/.dimension`: fitting unknown -> `InvalidDimension`, other unknown `i32` -> pre-construction normalized rule; `invalid-value/.position_finite` -> vector error; `.rotation_finite` -> `NonFiniteRotation`; after each valid record, `invalid-value/remote_player_states.strictly_increasing_ids` -> `InvalidStateOrder`. |
| `companion-spawn` (13) | required `id`, `name`, `tick:u64`, `dimension:i32`, `position`, `yaw`, `pitch`; strict decode -> `CompanionId::try_from_bytes` -> `CompanionName::try_from_canonical`; classify dimension and convert admitted `0` or `1`; `Dimension::new` -> `FiniteVec3::try_new` -> `LookAngles::try_new` -> `CompanionSpawn::try_new` | `companion-spawn`; `id`, `name`, output `server_tick` (from input `tick`), `dimension`, XYZ `position`, `look:{yaw,pitch}` | `invalid-identity/companion_spawn.player_id.{nonzero,version_v4,variant_rfc4122}` -> `InvalidIdentity`; then `invalid-value/companion_spawn.display_name.{utf8,length_range,control,canonical_trim}` and `.companion_name.embedded_whitespace` -> `InvalidText`; `invalid-enum/.dimension`: raw depths `1` -> `InvalidCompanionDimension`, fitting other unknown -> `InvalidDimension`, negative or `>255` -> pre-construction normalized rule; `invalid-value/.position_finite` -> vector error; `.rotation_finite` -> `NonFiniteRotation`; `.pitch_vertical_look_range` -> `InvalidCompanionPitch`. |
| `companion-states` (9) | required `tick:u64`, explicit non-null `states`; each requires `id`, `dimension`, `position`, `yaw`, `pitch`, `reset`; decode -> `CompanionId`; classify and convert dimension -> `Dimension`; `FiniteVec3` -> `LookAngles` -> `CompanionState::try_new`; ordered boxed slice -> `CompanionStates::try_new`. | `companion-states`; output `server_tick`, ordered `states`; each has `id`, `dimension`, XYZ `position`, `look:{yaw,pitch}`, `reset` | `invalid-value/companion_states.count_range`: empty -> `EmptyStateBatch`. The Go upper arm `>4` is a protocol cap the domain explicitly does not adopt; no frozen over-max case exists. A future over-max corpus row is a contract conflict to escalate, not a rejection this adapter fabricates. Then for each index: `invalid-identity/companion_states.state_<i>.player_id.{nonzero,version_v4,variant_rfc4122}` -> `InvalidIdentity`; `invalid-enum/.dimension`: depths -> `InvalidCompanionDimension`, fitting other unknown -> `InvalidDimension`, negative or `>255` -> pre-construction normalized rule; `invalid-value/.position_finite` -> vector error; `.rotation_finite` -> `NonFiniteRotation`; `.pitch_vertical_look_range` -> `InvalidCompanionPitch`; after each valid record, `invalid-value/companion_states.strictly_increasing_ids` -> `InvalidStateOrder`. |
| `companion-despawn` (2) | required `id`; strict decode -> `CompanionId::try_from_bytes` -> `CompanionDespawn::new` | `companion-despawn`; `id` | `invalid-identity/player_id.{nonzero,version_v4,variant_rfc4122}` -> `InvalidIdentity` in that order. The producer intentionally reuses the shared identity rule name. |

## Worker implementation and acceptance invariants

- Each topic `owns` predicate is an exhaustive match over the exact rules in its
  table. Unknown rules are `InvalidCase`; no topic has a fallback owner.
- Each topic test first fails with `NotImplemented`, then passes exactly its
  stated nonzero case count. Accepted fields come from Rust values. Rejected
  fields come from parsed input plus the rule selected above. Executors never
  inspect expected JSON.
- Node 3.1 support tests cover structured-value boundaries: negative zero, all
  four non-finite spellings (`NaN`, `Inf`, `+Inf`, `-Inf`), malformed UUIDs,
  required/optional absence semantics, numeric width failures, and wrong
  fixed-array length. Duplicate keys and trailing JSON are raw-byte concerns
  that cannot survive into `FrozenCase.input_json`; node 2.2's strict loader
  suite owns and independently tests those two boundaries for JSON inputs.
- The comparator test proves that an accepted command `wire`-only difference is
  ignored and that a mutation to any other semantic field fails. A rejected
  command's fields are compared without removal.
- The final gate requires exact counts `31,99,28,54,47,38,33,46`, total 376,
  unique IDs, no overlap/gap, and explicit exclusion of
  `domain.input/45/session-sequence-arrival` as
  `external:runtime-authority`.

Focused validation commands are the per-node commands in
[`03-domain-corpus.md`](03-domain-corpus.md). The integrated acceptance command
is:

```bash
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml \
  -p mornlea_domain --test corpus_domain --locked
```

It must execute 376 unique cases; a zero-test result, ignored topic, skipped
case, expectation-derived actual, or extra normalized-field omission is a
failure.
