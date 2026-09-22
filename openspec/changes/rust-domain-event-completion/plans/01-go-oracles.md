# Independently Executed Go Event Oracles

This packet owns tasks 1.1–1.3. Each node adds one package-local Go producer
and its reviewed case assets, but does not register those cases in the canonical
manifest; Rust registration occurs only when the matching adapter exists in
tasks 4.1–4.3. Workers do not edit `tasks.md`, `ledger.md`, Rust code or
`testdata/runtime-migration/contracts.json` in this packet.

All three producers use the shared strict decoder and runner conventions in
`domain_event_people_test.go`: lowercase-slug case labels; decimal-string
`u64`; JSON-number `u32/u16/u8/i32`; exact float tokens including `NaN`, `Inf`,
`+Inf`, `-Inf` and signed zero; explicit arrays; and normalized finite `f32`
as eight lowercase hexadecimal bits. Accepted dimensions, enum kinds and
despawn reasons retain their current Go numeric values as JSON integers;
grazing becomes a JSON Boolean. Entity IDs and ticks remain decimal strings,
UUID identities remain 32 lowercase hexadecimal characters, `DropID` remains
an object of its five ordered key fields, and item stacks remain numeric
`item/count/durability` objects. Rejected outcomes retain the raw input value,
including an unknown numeric enum. The producer receives only `CaseSpec` and
input bytes, constructs the current Go DTO, calls
`protocol.ValidateServerPacket`, and normalizes from that DTO. It never reads
the expected asset.

The shared family router remains `runDomainEvent` in
`domain_event_world_test.go`. Every node adds only its own closed rules to that
switch and begins with one full valid-input routing regression that fails on
the baseline with `unknown rule`, rather than treating a missing symbol as red
evidence.

## Node 1.1: Produce the 68-case hostile/passive event oracle

**Deliverable and prerequisites**

This node has no implementation predecessor. It creates one exact producer for
the six Go packet validators and 68 reviewed input/expected pairs. It does not
change the canonical manifest or any production package.

**Editable files**

- Create `packages/tools/cmd/runtime-oracle/domain_event_mobs_test.go`.
- Modify `packages/tools/cmd/runtime-oracle/domain_event_world_test.go` only to
  add the six router cases and the initial routing regression.
- Modify `packages/tools/cmd/runtime-oracle/AGENTS.md` with the producer,
  external-only publication and six-rule ownership entry.
- Create generated pairs under
  `testdata/runtime-migration/cases/domain/event_mobs/` after external review.

Read-only authorities are
`packages/shared/network/protocol/message_hostile.go`,
`packages/shared/network/protocol/message_passive.go`,
`packages/shared/network/protocol/registry.go`,
`packages/shared/core/block.go` and `packages/shared/core/health.go`. No other
node edits the shared router until this node is reviewed and committed.

**Frozen producer interface**

Declare constants for family `domain.event`, version `1`, operation `admit`,
consumer `mornlea_domain`, corpus directory `event_mobs`, report name
`runtime-corpus-domain-event-mobs.json`, producer test path and the six rules:

```text
hostile-spawn
hostile-state
hostile-despawn
passive-spawn
passive-state
passive-despawn
```

`domainEventMobsInput` carries `consumer`, `rule`, `server_tick`, and explicit
`spawns`, `states`, `ids` and `despawns` arrays. Hostile/passive record structs
carry only the exact Go DTO fields. IDs and ticks are decimal strings in the
frozen JSON so their full `u64` range is lossless. The provenance list is
exactly:

```text
packages/shared/network/protocol/message_hostile.go
packages/shared/network/protocol/message_passive.go
packages/shared/network/protocol/registry.go
packages/shared/core/block.go
packages/shared/core/health.go
```

Accepted fields use category equal to the rule and retain submitted array
order. Rejection categories stay inside the frozen corpus vocabulary the
baseline closes: `invalid-identity` for zero entity ID, `invalid-enum` for
dimension/kind/grazing/reason and every enum-domain value, and `invalid-value`
for non-finite pose or health, an empty batch, duplicate/reversed IDs, and
every other illegal field or cross-field value. Rule names are
`<rule_with_underscores>.<field>`; batch records add `.record_<index>` before
the field, and the two aggregate rules are `.count_range` and
`.strictly_increasing_ids`.

The seed values are tick `7`, IDs `1,2`, finite positions `[1.5,64,-3.25]` and
`[2.5,65,-4.25]`, finite velocity `[0.25,0,-0.5]`, yaw `0.5`, health `10`,
overworld dimension `0`, and canonical ascending order. Hostile seed records
use kind `0`; the `hostile-spawn-both-kinds` case alone changes the second
record to kind `1`. Passive-state seed records use both grazing bytes `0,1`;
passive-despawn seed records use both reasons `0,1`.

**Exact 68-case table**

| Rule | Labels and the one seed-relative change | Count |
| --- | --- | ---: |
| hostile-spawn | `hostile-spawn-seed`; `-zero-tick`; `-health-one`; `-health-twenty`; `-health-zero`; `-health-twenty-one`; `-depths-dimension`; `-zero-id`; `-non-finite-position`; `-non-finite-yaw`; `-empty`; `-reversed`; `-duplicate`; `-both-kinds` (two records, kinds 0 then 1) | 14 |
| hostile-state | `hostile-state-seed`; `-zero-tick`; `-health-one`; `-health-twenty`; `-health-zero`; `-health-twenty-one`; `-unknown-kind`; `-zero-id`; `-non-finite-position`; `-non-finite-velocity`; `-non-finite-yaw`; `-empty`; `-reversed`; `-duplicate` | 14 |
| hostile-despawn | `hostile-despawn-seed`; `-zero-tick`; `-zero-id`; `-empty`; `-reversed`; `-duplicate` | 6 |
| passive-spawn | `passive-spawn-seed`; `-zero-tick`; `-health-one`; `-health-twenty`; `-health-zero`; `-health-twenty-one`; `-depths-dimension`; `-zero-id`; `-non-finite-position`; `-non-finite-yaw`; `-empty`; `-reversed`; `-duplicate` | 13 |
| passive-state | `passive-state-seed`; `-zero-tick`; `-health-one`; `-health-twenty`; `-health-zero`; `-health-twenty-one`; `-unknown-grazing`; `-zero-id`; `-non-finite-position`; `-non-finite-velocity`; `-non-finite-yaw`; `-empty`; `-reversed`; `-duplicate` | 14 |
| passive-despawn | `passive-despawn-seed`; `-zero-tick`; `-unknown-reason`; `-zero-id`; `-empty`; `-reversed`; `-duplicate` | 7 |

`unknown-kind`, `unknown-grazing` and `unknown-reason` use raw value `2`.
Non-finite position mutates X to `NaN`; non-finite velocity mutates Z to
`+Inf`; non-finite yaw uses `-Inf`. Reverse swaps IDs 1 and 2; duplicate changes
the second ID to 1. Every other suffix mutates the first record only. Empty
arrays remain present in JSON.

**Red/green implementation sequence**

1. Add `TestDomainEventMobsRouterExecutesHostileSpawnSeed` to the shared router
   file with the full seed JSON and assert category `hostile-spawn`, two ordered
   records and no error. Run it and record the baseline `unknown rule` failure.
2. Add the producer file with strict decode, six DTO builders, first-failure
   classifiers, `runDomainEventMobs`, the exact case table, normalized field
   builders, working-manifest selection, runner/trace tests and the topic entry
   `TestDomainOracle_event_mobs`.
3. Add these named guards:

```text
TestDomainEventMobsOracleExecutesEveryCase
TestDomainEventMobsOracleCaseIdentitiesAreTheRustDomainConsumer
TestDomainEventMobsOracleWorkingManifestDescribesItself
TestDomainEventMobsOracleOutcomesDistinguishAcceptedAndRejected
TestDomainEventMobsOracleRunnerRejectsUnregisteredFamily
TestDomainEventMobsOracleRunnerRejectsOperationFamilyMismatch
TestDomainEventMobsOracleRunnerRejectsTamperedInput
TestDomainEventMobsOracleReportPublishesAndValidates
TestDomainEventMobsOracleRejectsMissingProducerTest
TestDomainEventMobsExportRejectsRepositoryAndSymlinkTargets
```

   The outcomes test requires exactly 68 unique labels, both `ok` and `error`
   for every rejectable rule, both accepted hostile kinds, both accepted
   grazing values, both passive despawn reasons, health 1 and 20, zero tick,
   and every exact aggregate/error rule above.
4. Build generated assets before comparing committed bytes when
   `RUNTIME_ORACLE_EXPORT_DIR` is set. The initial export run must create 136
   files externally and may then fail only because the tracked directory is not
   present. Review the external files, copy them mechanically into the tracked
   directory, and rerun normally.
5. Update the router and guide, run `gofmt`, and verify the external candidate
   and tracked directory are byte-identical.

**Validation and closure**

```bash
mornlea_export_dir=$(mktemp -d)
RUNTIME_ORACLE_EXPORT_DIR="$mornlea_export_dir" go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventMobs|TestDomainOracle_event_mobs)'
find "$mornlea_export_dir/runtime-oracle/domain-event-mobs" -type f | sort
diff -ru testdata/runtime-migration/cases/domain/event_mobs "$mornlea_export_dir/runtime-oracle/domain-event-mobs"
go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventMobs|TestDomainOracle_event_mobs)'
go test ./packages/audit -count=1 -run '^(TestRuntimeOracleInternalDependencies|TestCorpusTestFlagsCannotRewriteFrozenEvidence|TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --exit-code -- testdata/runtime-migration/contracts.json
git diff --check
```

Require 136 external/tracked files, an empty `diff`, exact 68 execution and no
canonical manifest change. Proposed commit:
`test(runtime-oracle): add mob event evidence`.

Rollback removes the producer, its six router cases, its guide entry and only
the `event_mobs` asset directory.

## Node 1.2: Produce the 45-case projectile/drop event oracle

**Deliverable and prerequisites**

After node 1.1, create one exact producer for projectile and item-drop DTOs and
45 reviewed pairs. The mob producer and assets are read-only.

**Editable files**

- Create `packages/tools/cmd/runtime-oracle/domain_event_objects_test.go`.
- Modify
  `packages/tools/cmd/runtime-oracle/domain_event_world_test.go` only for five
  new router cases and the valid projectile seed routing regression.
- Modify the runtime-oracle guide with this producer's ownership.
- Create reviewed pairs under
  `testdata/runtime-migration/cases/domain/event_objects/`.

Read-only authorities are
`packages/shared/network/protocol/message_projectile.go`,
`packages/shared/network/protocol/message_drop.go`,
`packages/shared/network/protocol/message_chunk.go`,
`packages/shared/network/protocol/registry.go`,
`packages/shared/core/block.go`, `packages/shared/core/drop.go`,
`packages/shared/core/item.go`, `packages/shared/core/armor.go` and
`packages/shared/core/pos.go`.

**Frozen producer interface**

Rules are `projectile-spawn`, `projectile-state`, `projectile-despawn`,
`item-drop-upserts` and `item-drop-removes`. `domainEventObjectsInput` carries
explicit `spawns`, `states`, `ids` and `drops` arrays. Projectile records use
decimal-string IDs and exact float tokens. Drop IDs are objects with raw i32
dimension, chunk `[x,z]`, u8 slot and u32 generation; stacks are `{item,count,
durability}`.

The provenance list is exactly:

```text
packages/shared/network/protocol/message_projectile.go
packages/shared/network/protocol/message_drop.go
packages/shared/network/protocol/message_chunk.go
packages/shared/network/protocol/registry.go
packages/shared/core/block.go
packages/shared/core/drop.go
packages/shared/core/item.go
packages/shared/core/armor.go
packages/shared/core/pos.go
```

Use the same normalized categories as node 1.1. Drop ID slot/generation errors
are `invalid-identity`; block index and stack errors are `invalid-value`;
dimension/kind are `invalid-enum`. Exact rule names follow the same indexed
form, including `item_drop_upserts.drop_<index>.block_index` and
`.stack.{item,count,durability}`.

Projectile seed tick is 7 and contains IDs 1..4 in order, covering all four
`(Shard|Arrow) × (Overworld|Depths)` combinations with finite position and
velocity. Its first record uses position `[1.5,64,-3.25]` and velocity
`[0.25,0,-0.5]`, so normalized velocity X is exactly `"3e800000"`.
Projectile state/despawn seeds contain IDs 1,2. Drop ID order is
dimension, chunk X, chunk Z, slot, generation. The upsert and remove seeds each
contain two strictly ordered IDs in dimension 0 and chunk `[0,0]`, with slots
1 then 2 and generation 1. The upsert records use the ordinary stone stack
`{item:1,count:1,durability:0}` at block indices 17 then 18. This two-record
seed is also the exact order-mutation fixture required by node 4.3.

**Exact 45-case table**

| Rule | Labels and the one seed-relative change | Count |
| --- | --- | ---: |
| projectile-spawn | `projectile-spawn-all-kind-dimension-combinations`; `-zero-tick`; `-unknown-kind`; `-unknown-dimension`; `-zero-id`; `-non-finite-position`; `-non-finite-velocity`; `-empty`; `-reversed`; `-duplicate` | 10 |
| projectile-state | `projectile-state-seed`; `-zero-tick`; `-zero-id`; `-non-finite-position`; `-empty`; `-reversed`; `-duplicate` | 7 |
| projectile-despawn | `projectile-despawn-seed`; `-zero-tick`; `-zero-id`; `-empty`; `-reversed`; `-duplicate` | 6 |
| item-drop-upserts | `item-drop-upserts-seed`; `-zero-tick`; `-raw-dimension-negative-one`; `-block-index-98303`; `-block-index-98304`; `-empty-stack`; `-slot-32`; `-zero-generation`; `-unregistered-item-66`; `-zero-count`; `-nondurable-with-durability`; `-empty`; `-reversed`; `-duplicate` | 14 |
| item-drop-removes | `item-drop-removes-seed`; `-zero-tick`; `-raw-dimension-negative-one`; `-slot-32`; `-zero-generation`; `-empty`; `-reversed`; `-duplicate` | 8 |

Unknown projectile kind/dimension use raw `2`; non-finite position uses `NaN`
and velocity uses `Inf`. Empty stack is exactly `{0,0,0}` and must be admitted.
`raw-dimension-negative-one` must remain accepted. Nondurable durability uses
stone with durability 1. The two ordering failures preserve otherwise valid
records: reverse swaps the two seed records and duplicate replaces the second
ID with the first. Every other suffix mutates the first record only.

**Red/green implementation sequence**

1. Add `TestDomainEventObjectsRouterExecutesProjectileSpawnSeed` with the full
   four-record seed and record the baseline `unknown rule` failure.
2. Implement strict parsing, five DTO builders/classifiers, normalized output,
   the exact table, external-first asset generation, working-manifest and
   independent runner/trace checks in the new file.
3. Add the same ten guard shapes as node 1.1 with `Mobs` replaced by `Objects`,
   plus `TestDomainEventObjectsProjectileKindDimensionMatrixIsComplete` and
   `TestDomainEventObjectsDropOrderingUsesRawDimensionFirst`. Require exactly
   45 cases and 90 generated files.
4. Import the reviewed external candidate, add the five routes, update the
   guide, format and prove byte identity. Do not add a protocol packet-cap
   rejection case; 128/32 are tested as separation facts in Rust node 2.2.

**Validation and closure**

```bash
mornlea_export_dir=$(mktemp -d)
RUNTIME_ORACLE_EXPORT_DIR="$mornlea_export_dir" go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventObjects|TestDomainOracle_event_objects)'
find "$mornlea_export_dir/runtime-oracle/domain-event-objects" -type f | sort
diff -ru testdata/runtime-migration/cases/domain/event_objects "$mornlea_export_dir/runtime-oracle/domain-event-objects"
go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventObjects|TestDomainOracle_event_objects)'
go test ./packages/audit -count=1 -run '^(TestRuntimeOracleInternalDependencies|TestCorpusTestFlagsCannotRewriteFrozenEvidence|TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --exit-code -- testdata/runtime-migration/contracts.json
git diff --check
```

Require 90 files, exact 45 execution and no manifest edit. Proposed commit:
`test(runtime-oracle): add object event evidence`.

Rollback removes only the object producer/routes/guide entry and
`event_objects` assets.

## Node 1.3: Produce the 44-case closed-chat event oracle

**Deliverable and prerequisites**

After node 1.2, create the exact current `protocol.ChatEvent.Validate` oracle,
cover every legal semantic branch and 28 invalid raw combinations, and close
the Go evidence stage without changing runtime behavior.

**Editable files**

- Create `packages/tools/cmd/runtime-oracle/domain_event_chat_test.go`.
- Modify
  `packages/tools/cmd/runtime-oracle/domain_event_world_test.go` only for the
  `chat` route and valid accepted seed regression.
- Modify the runtime-oracle guide with the chat union and final three-producer
  counts.
- Create reviewed pairs under
  `testdata/runtime-migration/cases/domain/event_chat/`.

Read-only authorities are
`packages/shared/network/protocol/message_companion.go`,
`packages/shared/network/protocol/registry.go`,
`packages/shared/core/player_id.go` and
`packages/shared/companion/identity.go`.

**Frozen producer interface**

The sole rule is `chat`. The raw input fields are `event_id` decimal string,
32-lowercase-hex `player_id` and `companion_id`, `player_name`,
`companion_name`, numeric `kind` and `reason`, `command`, and `speech`.
Every key is present, including empty strings and the zero companion UUID, so
the union decision never depends on an absent JSON field.

The normalized accepted output category is the exact semantic branch:
`accepted`, `invalid-format`, `unknown-companion`, `queue-full`,
`not-following`, `task-started`, `task-progress`, `task-completed`,
`task-timed-out`, `task-stopped`, `task-failed-<reason>` or `speech`.
Accepted fields contain event/player identity plus exactly the branch's legal
companion/name/command/speech data; they do not retain empty wire sentinels.
Rejected fields retain the raw semantic inputs and add exact `rule`.

Global identity/name failures precede kind dispatch. Non-speech carrying
speech fails before the kind-specific switch. Inside the switch use the exact
order in `ChatEvent.Validate`: reason, companion identity/name, then
command/speech text. Categories stay inside the frozen corpus vocabulary:
`invalid-identity` for zero event/player identity, `invalid-enum` for unknown
kinds and reserved/out-of-domain reasons, and `invalid-value` for every
text-boundary and illegal cross-field combination. Rule names begin
`chat_event.` and name the
failing field or combination; reserved reason 3 is
`chat_event.rejected.reason`, failure reasons 15/21 are
`chat_event.task_failed.reason`, and unknown kind is `chat_event.kind`.

Use a valid UUIDv4 player `00112233445546778899aabbccddeeff`, name `Alice`,
companion `2233445546674889aabbccddeeff00ff`, name `Buddy`, event ID 7,
command `gather stone`, and speech `Ready.`. Kind/reason numbers remain the Go
wire values 1..9 and 0/1/2/4/5/16..20.

**Exact 44-case table**

The 16 legal cases are:

```text
chat-accepted
chat-rejected-invalid-format
chat-rejected-unknown-companion
chat-rejected-queue-full
chat-rejected-not-following
chat-task-started
chat-task-progress
chat-task-completed
chat-task-timed-out
chat-task-stopped
chat-task-failed-planner-unavailable
chat-task-failed-invalid-plan
chat-task-failed-path-unreachable
chat-task-failed-world-changed
chat-task-failed-inventory-full
chat-speech
```

The 28 boundary/rejection cases are:

```text
chat-event-id-zero
chat-player-id-zero
chat-player-name-untrimmed
chat-accepted-reason-not-none
chat-accepted-zero-companion
chat-accepted-spaced-companion-name
chat-accepted-empty-command
chat-accepted-speech-leak
chat-invalid-format-leaks-companion-id
chat-invalid-format-leaks-companion-name
chat-invalid-format-leaks-command
chat-unknown-companion-invalid-name
chat-unknown-companion-leaks-id
chat-queue-full-missing-companion
chat-not-following-empty-command
chat-rejected-reserved-reason-three
chat-task-started-reason-not-none
chat-task-failed-reason-fifteen
chat-task-failed-reason-twenty-one
chat-task-event-speech-leak
chat-speech-has-command
chat-speech-empty
chat-speech-reason-not-none
chat-kind-unknown
chat-accepted-command-1024-bytes
chat-accepted-command-1025-bytes
chat-speech-256-bytes
chat-speech-257-bytes
```

The 1,024/256 boundaries are accepted and the plus-one cases rejected. The
invalid-format branch requires exact zero ID plus empty name/command/speech;
unknown-companion requires exact zero ID, valid name and empty texts. No case
interprets `/warp` as chat output.

**Red/green implementation sequence**

1. Add `TestDomainEventChatRouterExecutesAcceptedSeed` with the complete raw
   accepted event and record the baseline `unknown rule` failure.
2. Implement strict raw parsing, the legal-branch classifier, the exact case
   table, normalization, external-first asset generation, working-manifest and
   independent runner/trace checks.
3. Add the ten standard guard shapes named
   `TestDomainEventChat...`, `TestDomainOracle_event_chat`,
   `TestDomainEventChatEveryLegalBranchIsExecuted`, and
   `TestDomainEventChatIllegalFieldCombinationsAreRejected`. Require exactly 44
   cases, 16 legal semantic branches and 88 generated files.
4. Import the reviewed external candidate, add the route, update the guide,
   format and prove byte identity. Then run the full runtime-oracle package so
   the shared router proves all old and new rules remain single-owned.

**Validation and closure**

```bash
mornlea_export_dir=$(mktemp -d)
RUNTIME_ORACLE_EXPORT_DIR="$mornlea_export_dir" go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventChat|TestDomainOracle_event_chat)'
find "$mornlea_export_dir/runtime-oracle/domain-event-chat" -type f | sort
diff -ru testdata/runtime-migration/cases/domain/event_chat "$mornlea_export_dir/runtime-oracle/domain-event-chat"
go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventChat|TestDomainOracle_event_chat)'
go test ./packages/tools/cmd/runtime-oracle -race -count=1
go test ./packages/audit -count=1 -run '^(TestRuntimeOracleInternalDependencies|TestCorpusTestFlagsCannotRewriteFrozenEvidence|TestInternalDependenciesAreOneWay|TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --exit-code -- testdata/runtime-migration/contracts.json
git diff --check
```

Require 88 files, exact 44 execution and a green full package. Proposed commit:
`test(runtime-oracle): add chat event evidence`.

Rollback removes only the chat producer/route/guide entry and `event_chat`
assets; it does not touch the two earlier evidence families.
