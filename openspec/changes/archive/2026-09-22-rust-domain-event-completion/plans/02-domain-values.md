# Checked Rust Event Values

This packet owns tasks 2.1–2.4. Every node consumes the exact public interface
and algorithm in `00-execution-contract.md`; a worker does not choose alternate
field names, error variants or limits. Go producer code, corpus assets and the
canonical manifest are read-only here.

For a new public type, the first compile failure is only interface setup
evidence. Each node must then create a minimal compiling constructor that
preserves fields but intentionally has no relation check, add the named invalid
behavior test, and record that second test failing because the permissive value
was admitted. Only that behavioral failure is the task's TDD red evidence.

## Node 2.1: Implement hostile and passive semantic publications

**Deliverable and prerequisites**

After Go node 1.1, add the six checked mob publication types and their record
enums/parts/getters. Every field and omission is frozen by the common contract.

**Editable files**

- Create `packages/engine/crates/mornlea_domain/src/event/mobs.rs`.
- Create `packages/engine/crates/mornlea_domain/tests/event_mobs.rs`.
- Modify `packages/engine/crates/mornlea_domain/src/event.rs` and
  `packages/engine/crates/mornlea_domain/src/lib.rs` only to declare/re-export
  the new mob API.
- Modify `packages/engine/crates/mornlea_domain/AGENTS.md` with a mob-event
  ownership, validation and focused-test section.

`packages/engine/crates/mornlea_domain/src/identity.rs`,
`packages/engine/crates/mornlea_domain/src/values.rs`, every existing event
module and all corpus code are read-only. Node 2.2 waits for the shared export
files.

**Exact constructor behavior**

Implement the public signatures from the common packet. For spawn records,
check overworld dimension, finite yaw and health `1..=20` in that order. For
state records, check finite yaw then health. Despawn record construction is
total after typed identity/reason conversion. Batch constructors check 4,096,
empty and strict numeric ID order in that order. A batch owns its boxed slice,
accepts tick zero and performs no copy or sort.

`HostileKind`, `PassiveDespawnReason` and every parts/record type derive the
value traits their fields support (`Clone`, `Copy` where every field is copy,
`Debug`, `Eq` where no float exists, and `PartialEq`). No type exposes raw enum
numbers, cooldown, target, path, ground, dimension on state or despawn reason
for hostiles.

**Behavioral red and exact test set**

After the public skeleton compiles, add
`hostile_spawn_rejects_depths_before_yaw_and_health`; submit depths plus
non-finite yaw and health zero and require `InvalidDimension`. The permissive
skeleton must fail this test by returning `Ok`.

Complete `event_mobs.rs` with these named tests:

```text
hostile_spawn_seed_keeps_every_field_and_zero_tick
hostile_spawn_rejects_depths_before_yaw_and_health
hostile_spawn_rejects_non_finite_yaw_before_health
hostile_spawn_health_accepts_one_and_twenty_and_rejects_zero_and_twenty_one
hostile_state_keeps_position_velocity_kind_and_no_dimension
hostile_state_rejects_non_finite_yaw_before_health
hostile_batches_reject_empty_reversed_and_duplicate_ids
hostile_batches_accept_sixty_five_records_beyond_one_packet
hostile_despawn_keeps_only_tick_and_ids
passive_spawn_seed_keeps_every_field_and_zero_tick
passive_spawn_rejects_depths_before_yaw_and_health
passive_spawn_health_accepts_one_and_twenty_and_rejects_zero_and_twenty_one
passive_state_keeps_velocity_and_boolean_grazing_without_dimension
passive_state_rejects_non_finite_yaw_before_health
passive_despawn_preserves_vanished_and_died
passive_batches_reject_empty_reversed_and_duplicate_ids
passive_batches_accept_sixty_five_records_beyond_one_packet
mob_records_require_checked_nonzero_identity_and_finite_vectors
```

The seed literals name every parts field so an added field fails to compile.
The 65-record tests build IDs 1..=65 and assert unchanged first/last records.
Identity zero and non-finite vectors must fail in `HostileId`/`PassiveId` and
`FiniteVec3` before a record can be assembled. Direct tests never reproduce
raw kind/grazing/reason conversion; corpus adapters own that boundary.

**Implementation sequence**

1. Add the test file with seed helpers and the full public interface imports;
   record the expected unresolved-import setup failure.
2. Add module declarations, exports, private fields, getters and permissive
   relation constructors sufficient to compile.
3. Add and run the behavioral red above; record `Ok` versus
   `Err(InvalidDimension)`.
4. Implement record precedence and all six batch constructors. Add concise
   English module/constructor comments explaining semantic-versus-wire bounds
   and omitted simulation state.
5. Add the remaining named tests, update the guide and format.

**Validation and closure**

```bash
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_mobs --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_mobs --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract production_manifest_has_no_codec_kernel_or_host_dependencies --locked
go test ./packages/audit -count=1 -run '^(TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --check
```

Require all 18 names discovered and green. Proposed commit:
`feat(domain): add mob event values`.

Rollback removes the module/test/exports and its guide section without touching
Go evidence.

## Node 2.2: Implement projectile and item-drop semantic publications

**Deliverable and prerequisites**

After nodes 1.2 and 2.1, add the projectile/drop types, exact block-index error
and direct separation tests. Mob values and their exports are read-only except
for adjacent additions in the two shared export files.

**Editable files**

- Create `packages/engine/crates/mornlea_domain/src/event/objects.rs` and
  `packages/engine/crates/mornlea_domain/tests/event_objects.rs`.
- Modify `packages/engine/crates/mornlea_domain/src/identity.rs` only to add
  `DomainError::InvalidBlockIndex` with an English semantic-bound comment.
- Modify `packages/engine/crates/mornlea_domain/src/event.rs`,
  `packages/engine/crates/mornlea_domain/src/lib.rs` and
  `packages/engine/crates/mornlea_domain/AGENTS.md` for exports and the
  object-event contract.

**Exact constructor behavior**

Projectile record constructors are total after typed identity/dimension/vector
construction. No kind-by-dimension match is rejected. The three projectile
batches apply only cap, nonempty and strict numeric ID order. Item-drop
construction checks `block_index < 98_304`; `DropId` and `ItemStack` retain
their existing validators. Upsert/removal batches order by derived `DropId`
ordering and do not normalize raw dimensions.

**Behavioral red and exact test set**

After a compiling skeleton exists, add
`item_drop_rejects_block_index_98304_without_clamping`; the permissive skeleton
must fail by admitting it. Then implement these exact tests:

```text
projectile_spawn_keeps_all_four_kind_dimension_combinations
projectile_spawn_keeps_position_and_velocity
projectile_state_keeps_only_identity_and_position
projectile_despawn_keeps_only_tick_and_ids
projectile_batches_reject_empty_reversed_and_duplicate_ids
projectile_batches_accept_one_hundred_twenty_nine_records_beyond_one_packet
projectile_records_require_checked_nonzero_identity_and_finite_vectors
item_drop_keeps_raw_negative_dimension_and_stack_fields
item_drop_accepts_empty_stack_and_block_index_98303
item_drop_rejects_block_index_98304_without_clamping
item_drop_uses_existing_stack_registration_count_and_durability_rules
item_drop_upserts_reject_empty_reversed_and_duplicate_ids
item_drop_removes_reject_empty_reversed_and_duplicate_ids
item_drop_batches_accept_thirty_three_records_beyond_one_packet
object_batches_accept_zero_server_tick
object_seed_literals_expose_no_authority_only_fields
```

The 129/33 cases use strictly increasing typed IDs and verify first/last record
preservation. The absent-field test destructures/accesses only the frozen
getters and proves projectile state has no kind/dimension/velocity and item
drops have no world position or authority lifecycle state.

**Implementation sequence**

1. Add direct tests and record the unresolved-import setup failure.
2. Add exports, fields/getters and permissive relation constructors.
3. Run the 98,304 test and record the semantic red.
4. Add `InvalidBlockIndex`, exact item-drop check and five batch constructors.
5. Add the matrix, empty-stack, raw-dimension, ordering and packet-separation
   tests; update the guide and format.

**Validation and closure**

```bash
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_objects --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_objects --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test items_locations --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract production_manifest_has_no_codec_kernel_or_host_dependencies --locked
go test ./packages/audit -count=1 -run '^(TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --check
```

Require 16 named object tests green. Proposed commit:
`feat(domain): add object event values`.

Rollback removes object values/exports/tests and the single error variant; it
does not alter existing `DropId` or `ItemStack` behavior.

## Node 2.3: Implement the closed chat semantic union

**Deliverable and prerequisites**

After nodes 1.3 and 2.2, add a semantic chat representation in which every
accepted wire combination has one variant and every illegal combination has no
constructible state.

**Editable files**

- Create `packages/engine/crates/mornlea_domain/src/event/chat.rs` and
  `packages/engine/crates/mornlea_domain/tests/event_chat.rs`.
- Modify `packages/engine/crates/mornlea_domain/src/event.rs`,
  `packages/engine/crates/mornlea_domain/src/lib.rs` and
  `packages/engine/crates/mornlea_domain/AGENTS.md` for exports and the
  closed-union contract.

All text/identity implementations and Go chat code are read-only.

**Exact constructor behavior**

Implement the common contract verbatim. `CompanionSpeaker` is total from
checked types. The body/state/failure enums expose no raw kind/reason getters.
`ChatEvent::try_new` rejects only zero event ID as `InvalidIdentity`, then owns
the checked player fields and body without normalization. Event ID is not a
command sequence and no recipient belongs to the value.

**Behavioral red and exact test set**

After the compiling skeleton, add `chat_event_rejects_zero_event_id`; record
the permissive constructor returning `Ok`. Implement these exact tests:

```text
chat_event_rejects_zero_event_id
chat_event_keeps_player_identity_name_and_nonzero_id
chat_body_constructs_accepted_invalid_format_unknown_companion_queue_full_and_not_following
chat_task_state_constructs_started_progress_completed_timed_out_and_stopped
chat_task_failure_constructs_all_five_reasons
chat_body_constructs_speech_separately_from_command
chat_union_has_exactly_sixteen_legal_branch_shapes
chat_union_never_uses_zero_companion_identity_as_absence
chat_union_cannot_store_command_and_speech_together
chat_event_has_no_recipient_tick_reason_byte_or_command_sequence
```

`chat_union_has_exactly_sixteen_legal_branch_shapes` builds the 16 cases from
node 1.3 and exhaustively matches the body/task enums. Compile-time construction
proves illegal cross-field combinations have no variant; raw invalid kind,
reason and text combinations remain corpus-adapter tests rather than adding an
`Unknown` member.

**Implementation sequence**

1. Add the direct test file and record unresolved imports as setup only.
2. Add the exact structs/enums/getters and permissive chat-event skeleton.
3. Run the zero-ID test and record the behavioral failure.
4. Implement the nonzero check, derive only supported traits, add exhaustive
   matches and English ownership comments.
5. Update exports/guide, format and run all named tests.

**Validation and closure**

```bash
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_chat --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_chat --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test identity_values --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract production_manifest_has_no_codec_kernel_or_host_dependencies --locked
go test ./packages/audit -count=1 -run '^(TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --check
```

Require all ten tests and all 16 matched branch shapes. Proposed commit:
`feat(domain): add chat event union`.

Rollback removes only chat values/exports/tests and its guide section.

## Node 2.4: Prove all eleven new batch resource bounds

**Deliverable and prerequisites**

After mob and object values exist, extend the central resource suite so every
new batch proves the same 4,096-first contract. This node changes tests and the
guide only; production constructor fixes are returned to the owning node before
this task can close.

**Editable files**

- Modify `packages/engine/crates/mornlea_domain/tests/resource_bounds.rs`.
- Modify the resource-bounds section of the domain guide.

**Exact tests and data**

Add one test for each batch:

```text
hostile_spawn_admits_4096_and_rejects_4097_before_order_scan
hostile_state_admits_4096_and_rejects_4097_before_order_scan
hostile_despawn_admits_4096_and_rejects_4097_before_order_scan
passive_spawn_admits_4096_and_rejects_4097_before_order_scan
passive_state_admits_4096_and_rejects_4097_before_order_scan
passive_despawn_admits_4096_and_rejects_4097_before_order_scan
projectile_spawn_admits_4096_and_rejects_4097_before_order_scan
projectile_state_admits_4096_and_rejects_4097_before_order_scan
projectile_despawn_admits_4096_and_rejects_4097_before_order_scan
item_drop_upserts_admit_4096_and_reject_4097_before_order_scan
item_drop_removes_admit_4096_and_reject_4097_before_order_scan
```

Each admitted vector uses IDs 1..=4,096 in canonical order and asserts length,
first key and last key. Each oversized vector uses IDs 1..=4,097, swaps the
first two otherwise valid records, and requires exactly `BatchTooLarge`. This
simultaneous later order defect proves precedence. Drop IDs use dimension 0,
chunk X increasing, chunk Z 0, slot 0 and generation 1; when X spans 4,097 it
remains inside i32 and preserves `DropId` order. All records are constructed
before calling the batch and no allocation-failure injection is added because
the batch owns the provided box and allocates no scratch.

**Red/green sequence**

1. Add all eleven tests before changing any constructor. Any constructor whose
   limit/order precedence is wrong must fail with its actual result recorded.
2. If all eleven pass because nodes 2.1/2.2 implemented the frozen algorithm,
   the tests are still new regression coverage; do not invent a production
   change to manufacture red. Record that the behavior was already introduced
   test-first in the owning node.
3. Correct only an actual contract discrepancy in the owning module, rerun its
   focused suite, then complete all eleven central tests and the guide count.

**Validation and closure**

```bash
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test resource_bounds --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test resource_bounds --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --locked
go test ./packages/audit -count=1 -run '^(TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --check
```

Require the eleven new names plus every pre-existing resource test. Proposed
commit: `test(domain): cover event batch bounds`.

Rollback removes only these eleven tests and their guide count; production
limits remain owned by nodes 2.1/2.2.
