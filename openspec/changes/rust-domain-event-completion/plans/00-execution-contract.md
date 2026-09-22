# Rust Domain Event Completion Frozen Execution Contract

> **For agentic workers:** this packet freezes the interfaces, ownership,
> evidence model and limits consumed by every node in `tasks.md`. A worker may
> choose equivalent private local organization inside its owned files, but it
> may not change a public name, field, validation order, case count, corpus
> identity or downstream boundary here. A conflict returns to the controller
> and reconciles the OpenSpec artifacts before implementation continues.

**Goal:** add the missing semantic publication values and one complete event
surface while keeping the current Go runtime, protocol v45 and all persistence
formats untouched.

**Architecture:** package-local Go tests execute current protocol DTO
validators and produce normalized immutable evidence. Public Rust domain types
use checked existing primitives and closed enums. Corpus adapters parse raw
evidence, independently classify the Go rule in its documented precedence,
exercise the applicable Rust constructor, and compare normalized output.

**Tech Stack:** Go 1.26, Rust 1.97.1, the existing runtime-oracle JSON schema,
Serde test support and OpenSpec.

**Spec:** `../proposal.md`, `../specs/rust-runtime-foundation/spec.md` and
`../design.md`.

## Baseline and workflow

The planning baseline is `3401fa7a12c97844791822f185071a3641998773`, which
descends from `d241611316fb683d5eee99dc194a24d33fd8cd52` and contains
merged-main commit `effd8a247427d2ab5710a8f481349c4cd4676721` from PR
#184. The intervening commit adds only unrelated `redesign-ci-standards`
planning packets. At execution start the controller records the actual branch
HEAD, confirms it still descends from that merge, checks the four-point orphan
state (`tasks.md`, ledger frontier, untracked in-flight files and recent
mtimes), and records any later compatible baseline in `ledger.md` without
rewriting this historical planning fact.

Use an isolated worktree at execution time when the selected Superpowers
workflow requires one. On a clean checkout run `make rust` before the first
focused Go command because this change spans Rust. Every node follows: add the
named failing behavioral test; run it and record the intended red verdict;
implement only the frozen contract; run focused green gates; inspect derived
consumers; request independent review; record the ruling in `ledger.md`; create
the scoped commit named by the packet. A failed required gate leaves its task
open.

## Exact public Rust interface

All fields are private after construction. `Parts` fields are public inputs;
records expose read-only getters with the same names. Boxed batches expose
borrowed slices. Total constructors are named `new`; a constructor that checks
a relation is named `try_new` and returns `Result<Self, DomainError>`.

### Mob values in `src/event/mobs.rs`

```rust
pub enum HostileKind { Nightwalker, BoneThrower }

pub struct HostileSpawnRecordParts {
    pub id: HostileId,
    pub dimension: Dimension,
    pub position: FiniteVec3,
    pub yaw: f32,
    pub health: u8,
    pub kind: HostileKind,
}
pub struct HostileSpawnRecord { /* exact fields above */ }
impl HostileSpawnRecord {
    pub fn try_new(parts: HostileSpawnRecordParts) -> Result<Self, DomainError>;
}

pub struct HostileStateRecordParts {
    pub id: HostileId,
    pub position: FiniteVec3,
    pub velocity: FiniteVec3,
    pub yaw: f32,
    pub health: u8,
    pub kind: HostileKind,
}
pub struct HostileStateRecord { /* exact fields above */ }
impl HostileStateRecord {
    pub fn try_new(parts: HostileStateRecordParts) -> Result<Self, DomainError>;
}

pub struct HostileSpawnParts {
    pub server_tick: u64,
    pub spawns: Box<[HostileSpawnRecord]>,
}
pub struct HostileSpawn { /* exact fields above */ }
pub struct HostileStateParts {
    pub server_tick: u64,
    pub states: Box<[HostileStateRecord]>,
}
pub struct HostileState { /* exact fields above */ }
pub struct HostileDespawnParts {
    pub server_tick: u64,
    pub ids: Box<[HostileId]>,
}
pub struct HostileDespawn { /* exact fields above */ }
```

Each batch implements `try_new(parts)`. Hostile spawn rejects a non-overworld
dimension as `InvalidDimension`, then a non-finite yaw as
`NonFiniteRotation`, then health outside `1..=20` as
`InvalidSurvivalValue`. Hostile state checks yaw then health. Typed IDs and
`FiniteVec3` values have already enforced identity and vector finiteness.

```rust
pub enum PassiveDespawnReason { Vanished, Died }

pub struct PassiveSpawnRecordParts {
    pub id: PassiveId,
    pub dimension: Dimension,
    pub position: FiniteVec3,
    pub yaw: f32,
    pub health: u8,
}
pub struct PassiveStateRecordParts {
    pub id: PassiveId,
    pub position: FiniteVec3,
    pub velocity: FiniteVec3,
    pub yaw: f32,
    pub health: u8,
    pub grazing: bool,
}
pub struct PassiveDespawnRecord {
    id: PassiveId,
    reason: PassiveDespawnReason,
}
impl PassiveDespawnRecord {
    pub fn new(id: PassiveId, reason: PassiveDespawnReason) -> Self;
}

pub struct PassiveSpawnParts {
    pub server_tick: u64,
    pub spawns: Box<[PassiveSpawnRecord]>,
}
pub struct PassiveStateParts {
    pub server_tick: u64,
    pub states: Box<[PassiveStateRecord]>,
}
pub struct PassiveDespawnParts {
    pub server_tick: u64,
    pub despawns: Box<[PassiveDespawnRecord]>,
}
```

`PassiveSpawnRecord::try_new` applies dimension, yaw and health in that order;
`PassiveStateRecord::try_new` applies yaw then health. The three enclosing
types are `PassiveSpawn`, `PassiveState` and `PassiveDespawn` with matching
`try_new`, tick and slice getters. Raw grazing `0/1` and despawn reason `0/1`
conversion belongs to evidence/protocol adapters; the domain stores `bool` and
the closed enum only.

### Object values in `src/event/objects.rs`

```rust
pub enum ProjectileKind { Shard, Arrow }

pub struct ProjectileSpawnRecordParts {
    pub id: ProjectileId,
    pub kind: ProjectileKind,
    pub dimension: Dimension,
    pub position: FiniteVec3,
    pub velocity: FiniteVec3,
}
pub struct ProjectileSpawnRecord { /* exact fields above */ }
impl ProjectileSpawnRecord { pub fn new(parts: ProjectileSpawnRecordParts) -> Self; }

pub struct ProjectileStateRecordParts {
    pub id: ProjectileId,
    pub position: FiniteVec3,
}
pub struct ProjectileStateRecord { /* exact fields above */ }
impl ProjectileStateRecord { pub fn new(parts: ProjectileStateRecordParts) -> Self; }

pub struct ProjectileSpawnParts {
    pub server_tick: u64,
    pub spawns: Box<[ProjectileSpawnRecord]>,
}
pub struct ProjectileStateParts {
    pub server_tick: u64,
    pub states: Box<[ProjectileStateRecord]>,
}
pub struct ProjectileDespawnParts {
    pub server_tick: u64,
    pub ids: Box<[ProjectileId]>,
}
```

The enclosing types are `ProjectileSpawn`, `ProjectileState` and
`ProjectileDespawn`, each with `try_new`. The spawn record is total because
both supported kinds and both checked `Dimension` values are legal in every
combination. State has no kind, dimension or velocity field.

```rust
pub struct ItemDropParts {
    pub id: DropId,
    pub block_index: u32,
    pub stack: ItemStack,
}
pub struct ItemDrop { /* exact fields above */ }
impl ItemDrop { pub fn try_new(parts: ItemDropParts) -> Result<Self, DomainError>; }

pub struct ItemDropUpsertsParts {
    pub server_tick: u64,
    pub drops: Box<[ItemDrop]>,
}
pub struct ItemDropRemovesParts {
    pub server_tick: u64,
    pub ids: Box<[DropId]>,
}
```

The enclosing types are `ItemDropUpserts` and `ItemDropRemoves`, each with
`try_new`. `ItemDrop::try_new` accepts `0..=98_303`, reports
`DomainError::InvalidBlockIndex` for `98_304` and above, accepts
`ItemStack::EMPTY`, and does not tighten `DropId`'s raw dimension.

### Chat values in `src/event/chat.rs`

```rust
pub struct CompanionSpeaker { id: CompanionId, name: CompanionName }
impl CompanionSpeaker {
    pub fn new(id: CompanionId, name: CompanionName) -> Self;
    pub fn id(&self) -> CompanionId;
    pub fn name(&self) -> &CompanionName;
}

pub enum TaskFailure {
    PlannerUnavailable,
    InvalidPlan,
    PathUnreachable,
    WorldChanged,
    InventoryFull,
}
pub enum TaskState {
    Started,
    Progress,
    Completed,
    TimedOut,
    Stopped,
    Failed(TaskFailure),
}
pub enum ChatBody {
    Accepted { companion: CompanionSpeaker, command: CommandText },
    InvalidFormat,
    UnknownCompanion { name: CompanionName },
    QueueFull { companion: CompanionSpeaker, command: CommandText },
    NotFollowing { companion: CompanionSpeaker, command: CommandText },
    Task { companion: CompanionSpeaker, command: CommandText, state: TaskState },
    Speech { companion: CompanionSpeaker, text: SpeechText },
}
pub struct ChatEventParts {
    pub event_id: u64,
    pub player_id: PlayerId,
    pub player_name: DisplayName,
    pub body: ChatBody,
}
pub struct ChatEvent { /* exact fields above */ }
impl ChatEvent {
    pub fn try_new(parts: ChatEventParts) -> Result<Self, DomainError>;
    pub fn event_id(&self) -> u64;
    pub fn player_id(&self) -> PlayerId;
    pub fn player_name(&self) -> &DisplayName;
    pub fn body(&self) -> &ChatBody;
}
```

Only `event_id == 0` remains rejectable after typed parts construction and maps
to `DomainError::InvalidIdentity`. No `Unknown` enum, absent companion ID,
reason byte, command/speech sentinel, routing recipient or command sequence is
part of these types.

### Event surface in `src/event.rs`

```rust
pub enum Event {
    ChunkSnapshot(ChunkSnapshot),
    BlockChanges(BlockChanges),
    ForgetChunks(ForgetChunks),
    PlayerState(PlayerState),
    CommandRejected(CommandRejection),
    RemotePlayerSpawn(RemotePlayerSpawn),
    RemotePlayerDespawn(RemotePlayerDespawn),
    RemotePlayerStates(RemotePlayerStates),
    InventoryState(InventoryState),
    ItemDropUpserts(ItemDropUpserts),
    ItemDropRemoves(ItemDropRemoves),
    FurnaceState(FurnaceState),
    ContainerClosed(ContainerClosed),
    ChestState(ChestState),
    Chat(ChatEvent),
    CompanionSpawn(CompanionSpawn),
    CompanionStates(CompanionStates),
    CompanionDespawn(CompanionDespawn),
    PlaceBlockSucceeded(PlacementSuccess),
    CraftingState(CraftingState),
    HostileSpawn(HostileSpawn),
    HostileState(HostileState),
    HostileDespawn(HostileDespawn),
    CombatHit(CombatHit),
    PassiveSpawn(PassiveSpawn),
    PassiveState(PassiveState),
    PassiveDespawn(PassiveDespawn),
    ProjectileSpawn(ProjectileSpawn),
    ProjectileState(ProjectileState),
    ProjectileDespawn(ProjectileDespawn),
}

pub enum EventRecipient { Session(u64), Broadcast }
pub struct RoutedEvent { recipient: EventRecipient, event: Event }
impl RoutedEvent {
    pub fn new(recipient: EventRecipient, event: Event) -> Self;
    pub fn recipient(&self) -> EventRecipient;
    pub fn event(&self) -> &Event;
}
```

`Event` and `RoutedEvent` derive `Clone`, `Debug` and `PartialEq`;
`EventRecipient` additionally derives `Copy` and `Eq`. Delete production
`Observation`, `order_observations`, `FAMILY_INPUT` and `FAMILY_EVENT` and the
two observation-only runtime-contract tests. No repository consumer other than
those tests currently uses those Rust items, so no test-only digest helper is
retained. The Go runtime-oracle `Observation` is an unrelated harness type and
remains unchanged.

## Batch algorithm and error precedence

Every new batch constructor uses this exact order without allocating or
sorting:

```rust
if records.len() > crate::MAX_SEMANTIC_BATCH_RECORDS {
    return Err(DomainError::BatchTooLarge);
}
if records.is_empty() {
    return Err(DomainError::EmptyStateBatch);
}
if records.windows(2).any(|pair| key(&pair[0]) >= key(&pair[1])) {
    return Err(DomainError::InvalidStateOrder);
}
```

The concrete field (`spawns`, `states`, `ids`, `despawns` or `drops`) replaces
`records`; the key is the typed numeric entity ID or `DropId`. Submitted order
is preserved. Zero server tick is accepted. Individual typed records are
constructed before entering the batch; corpus adapters must therefore classify
raw count overflow before constructing individual records when a raw test ever
exercises the semantic cap.

Packet maxima are evidence, not domain rules: direct tests construct and admit
65 hostile records, 65 passive records, 129 projectile records and 33 drops.
The shared cap remains 4,096 for all eleven new batch types.

## Corpus ownership, identity and exact counts

All new cases retain family `domain.event`, version `1`, operation `admit`,
consumer `mornlea_domain`, JSON input, checkpoint `"0"` and normalized
`kind: "ok" | "error"` output. Inputs always carry
`{"consumer":"mornlea_domain","rule":...}`. Rule ownership is closed:

| Rust topic | Exact rules | New cases | Integrated total |
| --- | --- | ---: | ---: |
| `event_mobs` | hostile spawn/state/despawn; passive spawn/state/despawn | 68 | 444 |
| `event_objects` | projectile spawn/state/despawn; item-drop upserts/removes | 45 | 489 |
| `event_chat` | chat | 44 | 533 |

The starting partition is exactly 376 cases across eight topics; starting
`domain.event` coverage is 164 cases. Final `domain.event` coverage is 321 and
the final Rust-owned domain partition is 533. `corpus_domain.rs` owns the
single `TOPICS` table and `TOTAL_DOMAIN_CASES`. Node 4.1 renames the numeric
test function to `corpus_domain_executes_exact_unique_partition`, so later
count changes touch the constant and guide rather than a test symbol.

Go producers read no expected JSON. They call `protocol.ValidateServerPacket`
on the constructed current DTO, normalize fields from the DTO itself, and map
the first Go validation failure to the exact rule named in their tables. An
unknown raw kind, dimension, grazing byte, despawn reason or chat combination
is a normalized semantic rejection, not a malformed-input dispatch failure.
Malformed JSON, missing required fields, wrong JSON types, integer-width
overflow, invalid float-token grammar and malformed identity hex remain hard
case errors.

Rust topic adapters use the existing strict support readers and normalize
accepted fields only from Rust getters or exhaustive enum matches. For a raw
value that cannot enter a closed Rust type, the adapter verifies the
pre-construction classifier and emits the Go rule; for representable invalid
values it also asserts the exact `DomainError`. It never reads the expected
outcome inside its executor. Every topic has an exact `EXPECTED_COUNT`, closed
`RULES`, `owns`, `execute` and `execute_all_cases_match_frozen_go` test.

## Frozen evidence publication and source identity

Ordinary producer runs compare committed assets read-only. With a fresh
external `RUNTIME_ORACLE_EXPORT_DIR`, each producer publishes its complete
candidate below its own child:

```text
runtime-oracle/domain-event-mobs/
runtime-oracle/domain-event-objects/
runtime-oracle/domain-event-chat/
```

The candidate is reviewed, then copied mechanically into the matching tracked
directory under `testdata/runtime-migration/cases/domain/`. Tests must reject a
repository-contained export root, any symlink component, duplicate target,
file-as-parent collision or preexisting export child. No update flag or direct
tracked writer is introduced.

The final canonical `domain.event` source set is the existing 25 paths plus
exactly these five paths, sorted and freshly hashed:

```text
packages/shared/core/drop.go
packages/shared/network/protocol/message_drop.go
packages/shared/network/protocol/message_hostile.go
packages/shared/network/protocol/message_passive.go
packages/shared/network/protocol/message_projectile.go
```

Before node 4.3 edits the manifest, the controller records
`CORPUS_SOURCE_SHA=$(git rev-parse HEAD)`. That full 40-character lowercase SHA
must contain all Go producer code, Rust production values, resource bounds and
the 30-variant event surface. Node 4.3 writes the same value to
`testdata/runtime-migration/contracts.json` `source_revision` and
`packages/tools/cmd/runtime-oracle/inventory.go`
`BaselineSourceRevision`. The later adapter/manifest/evidence commits may
follow it; no compatibility source changes after the checkpoint. Every source
digest is computed from repository bytes at the checkpoint, and duplicated
paths are forbidden.

## Derived consumers and exclusive ownership

- Go producer files are consumed by the runtime-oracle Go test package,
  `TestRuntimeOracleInternalDependencies`, the frozen-evidence writer guard,
  English-comment migration and task-ID comment scan.
- The new Rust modules are re-exported by `src/event.rs` and `src/lib.rs`,
  consumed by focused tests, corpus adapters and the production-manifest
  dependency gate. No protocol/storage source is edited in this change.
- `contracts.json` and every case asset are consumed by runtime-oracle
  reconciliation/trace tests, Rust `runtime_corpus.rs`, `corpus_loader`,
  `corpus_domain`, and source/digest validation. Its source revision is also
  mirrored by `BaselineSourceRevision`.
- `packages/engine/crates/mornlea_domain/AGENTS.md` owns the public-value,
  batch, event-surface and exact corpus-count guide. The runtime-oracle guide
  owns producer routes, external-only publication and final source/case counts.
- `tasks.md` is the only checkbox source. Workers never edit it or the ledger;
  the controller updates both after review.

The Go oracle nodes serially own the shared `runDomainEvent` router. Rust value
nodes serially own `event.rs`, `lib.rs` and the domain guide. Corpus nodes
serially own `contracts.json`, `corpus_domain.rs`, the exact partition counts
and source hashes. No two editing workers may run over one of those ownership
chains concurrently.

## Completion boundary and rollback

Completion means: all three Go producers execute nonzero exact case sets;
every focused Rust constructor and resource test passes; all 30 `Event`
variants and two recipient forms construct; exactly 533 unique domain cases
execute once; nine named semantic mutations fail comparison; dependency,
source, tracked-writer and comment gates pass; and the delta is reviewed,
synced and archived. It does not mean complete F1 or F2 readiness.

Each node is a rollback unit with its scoped commit. Before canonical sync, a
failed node reverts only its own reviewed commit through a new revert commit if
necessary; never reset or force-push. After sync/archive starts, use the
recovery sequence in `05-closeout.md` so the active change and canonical spec
cannot disagree silently.
