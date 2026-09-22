## Context

The accepted `rust-runtime-foundation-baseline` change established the evidence
harness and a bounded subset of `mornlea_domain`; it explicitly did not complete
the original F1 migration. The current crate has checked player, world,
inventory, remote-player and companion publication values, but it has no mob,
projectile, item-drop or chat event modules. Its production replay surface is
still the digest-based `Observation`, while the target server and client need
typed semantic publications.

This change is the first F1 successor. It closes the shared event-value boundary
only. The Go server remains authoritative and the current startup path remains
unchanged. Protocol conversion, save conversion, complete corpus coverage,
public numerical APIs, pathfinding and final F1 acceptance remain later
successors and continue to block `rust-authoritative-server`.

## Goals / Non-Goals

Goals:

- complete the checked semantic values for all currently missing server
  publications;
- replace the production digest surrogate with an exhaustive typed event union
  and a separate routing envelope;
- prove the new values through independently executed Go producer and Rust
  consumer cases; and
- keep construction bounded, atomic, dependency-free and headless.

Non-goals:

- implement packet encoding or decoding, save conversion, server authority,
  subscriptions, visibility, transport sessions or worker lifecycle;
- expose new numerical kernels or pathfinding;
- change protocol v45 or any save, ABI or benchmark version;
- add gameplay, presentation behavior, a runtime thread, or a default-path
  switch; or
- claim complete F1 acceptance from this event-only slice.

## Decisions

### Keep the domain boundary typed, checked and dependency-free

Add `event/mobs.rs`, `event/objects.rs` and `event/chat.rs` under
`mornlea_domain`. Values with invariants use private fields, a public `Parts`
input where a record has several fields, `try_new` construction and read-only
accessors. Closed concepts use Rust enums rather than retaining raw Go/wire
numbers. The crate keeps no production dependencies, denies unsafe code and
does not import protocol, storage, server, client, Godot, Python or GPU types.

Existing `HostileId`, `PassiveId`, `ProjectileId`, `DropId`, `Dimension`,
`FiniteVec3`, `ItemStack` and bounded text/identity values remain the shared
primitives. Source-side raw enum conversion belongs to the Go producer or a
later protocol adapter; the domain never exposes an `Unknown` enum member.
Validation uses `InvalidDimension`, `InvalidIdentity`, `NonFiniteRotation`,
`NonFiniteValue`, `InvalidSurvivalValue`, `EmptyStateBatch`,
`InvalidStateOrder` and `BatchTooLarge` for their existing exact meanings. Add
`DomainError::InvalidBlockIndex` for the 98,304-cell item-drop bound; no generic
error or silent clamping substitutes for it.

### Model hostile and passive publications without simulation state

`HostileKind` has only `Nightwalker` and `BoneThrower`.
`HostileSpawnRecord` contains hostile ID, overworld dimension, finite position,
finite yaw, health in `1..=20` and kind. `HostileStateRecord` contains hostile
ID, finite position and velocity, finite yaw, health in `1..=20` and kind.
Hostile despawn records contain only the typed ID.

`PassiveSpawnRecord` contains passive ID, overworld dimension, finite position,
finite yaw and health in `1..=20`. `PassiveStateRecord` contains passive ID,
finite position and velocity, finite yaw, health in `1..=20` and a Boolean
`grazing` value. `PassiveDespawnRecord` contains passive ID and
`PassiveDespawnReason::{Vanished,Died}`.

Each spawn, state and despawn publication owns `server_tick` and boxed records.
Zero server tick remains valid because the current Go validators permit it.
No cooldown, target, path, AI or authority-capacity state is added.

### Model projectile and item-drop publications without wire policy

`ProjectileKind` has only `Shard` and `Arrow`.
`ProjectileSpawnRecord` contains projectile ID, kind, either supported
dimension, finite position and finite velocity. The semantic constructor accepts
all kind-by-dimension combinations; current generation policy belongs to the
authority. `ProjectileStateRecord` contains only projectile ID and finite
position because kind, dimension and velocity are not part of the current state
publication. Projectile despawn records contain only the typed ID.

`ItemDrop` contains `DropId`, `block_index` and `ItemStack`. `block_index` is
valid in `0..98_304`. `DropId` retains its existing arbitrary raw dimension,
slot and generation rules. `ItemStack::EMPTY` remains valid because the current
Go publication accepts it. `ItemDropUpserts` owns ordered drops and
`ItemDropRemoves` owns ordered IDs; neither invents position, velocity or
authority state.

### Represent chat as a closed legal union

`ChatEventParts` contains nonzero `event_id`, `PlayerId`, `DisplayName` and a
`ChatBody`. `CompanionSpeaker` contains `CompanionId` and `CompanionName`.
`ChatBody` has exactly these legal shapes:

- `Accepted { companion, command }`;
- `InvalidFormat`;
- `UnknownCompanion { name }`;
- `QueueFull { companion, command }`;
- `NotFollowing { companion, command }`;
- `Task { companion, command, state }`; and
- `Speech { companion, text }`.

`TaskState` is `Started`, `Progress`, `Completed`, `TimedOut`, `Stopped` or
`Failed(TaskFailure)`. `TaskFailure` is `PlannerUnavailable`, `InvalidPlan`,
`PathUnreachable`, `WorldChanged` or `InventoryFull`. This structure exhausts
the current `ChatEvent.Validate` table: absent companions are represented by the
two variants that permit absence, not by a zero ID; commands and generated
speech cannot coexist; task-failure reasons cannot leak into other variants.
The later protocol adapter alone maps these legal values to raw kind/reason
numbers and zero wire UUIDs.

### Publish one exhaustive event union and a separate recipient envelope

Replace the production `Observation` abstraction with an `Event` enum containing
exactly the 30 variants named in the delta specification. Every variant carries
its checked semantic value. The enum derives value-oriented traits that its
owned data supports and does not expose packet numbers, byte payloads, digests
or a catch-all variant.

Routing is `RoutedEvent { recipient, event }`, where `EventRecipient` is
`Session(u64)` or `Broadcast`. The session number is opaque at this layer,
including zero: runtime session existence, subscription and visibility checks
belong to later server ownership. Values that already carry server ticks retain
them; inventory, container-close and chat events do not receive fabricated
ticks.

Handshake, login, rejection, keepalive and disconnect are transport lifecycle,
not semantic events. Chunk acquisition, generation, readiness,
resynchronization and generated-chunk pointers are runtime worker messages, not
missing event variants. The digest `Observation` and `order_observations`
helpers move to test-only support or are deleted when no test needs them.

### Apply the semantic budget before record inspection

Every new variable-size batch accepts `1..=MAX_SEMANTIC_BATCH_RECORDS` records,
where the existing shared value is 4,096. Construction checks length before
per-record validation, copying or ordering work, then validates each record and
strictly increasing typed IDs without sorting. Any error rejects the complete
input. Boxed ownership avoids retaining caller aliases and supports immutable
cross-thread publication later.

Wire maxima remain deliberately separate: hostile/passive packets allow 64,
projectile packets 128 and item-drop packets 32. A semantic batch above one of
those limits is not invalid merely because a future protocol adapter must reject
or partition it.

### Extend evidence without letting fixtures become the oracle

Add focused Rust suites for `event_mobs`, `event_objects`, `event_chat` and
`event_surface`. Add matching package-local Go producers under
`packages/tools/cmd/runtime-oracle/` that call the current protocol validation
paths, normalize semantic fields, and export only to temporary or fresh
external storage through the established containment checks. Event wire codecs
remain owned by the later protocol successor. Reviewed case assets live under
`testdata/runtime-migration/cases/domain/`; tests never rewrite them.

The corpus binds source identity and assigns each Rust-owned case to the closed
`mornlea_domain` consumer. Each selected case runs exactly once in Go and Rust;
zero discovery is a hard failure. Boundary coverage includes valid and invalid
health, enum values, grazing conversion, dimensions, finite values, block index,
empty stack, ID order and every chat branch. Mutation tests change individual
semantic fields and require comparison failure. The event-surface suite
constructs all 30 variants once so source-file counts cannot masquerade as
coverage.

### Keep downstream migration ordered by accepted boundaries

This change does not satisfy F1 as a whole. The next successors separately close
protocol evidence, storage evidence, safe public numerical APIs and pathfinding,
then run a final F1 acceptance that rejects uncovered families. F2 may be
replanned against those accepted contracts, but no `mornlea_server` task may
start merely because this event change or the earlier baseline exists.

## Rejected Alternatives

- Extending F2 now would force the server to depend on incomplete protocol,
  storage and kernel boundaries and would duplicate compatibility decisions.
- Reusing protocol DTOs in the domain would reverse the target dependency and
  preserve wire sentinels as application values.
- Keeping digest-only production observations would prove file agreement rather
  than an exhaustive typed publication surface.
- Applying packet maxima to domain batches would couple semantic values to one
  transport version and prevent later bounded aggregation or partitioning.
- Adding an opaque event variant would hide missing ownership and let coverage
  pass without semantic comparison.

## Risks / Trade-offs

- A 30-variant enum is intentionally explicit and makes additions compile-time
  breaking; that cost is preferred because every new semantic publication must
  receive an owner, evidence and downstream handling.
- The 4,096 semantic cap permits values larger than one packet; later adapters
  must make their packet behavior explicit instead of relying on type size.
- Current Go validators are compatibility sources, not permanent owners. Frozen
  evidence can still become stale, so source identity and mutation tests remain
  required.
- Moving digest helpers may disturb existing tests. The change preserves any
  still-useful digest machinery in test-only support, not production API.

## Migration Plan

1. Freeze focused Go producer cases for mobs, objects and chat without changing
   tracked assets during test execution.
2. Add failing Rust constructor tests for each value family and its resource,
   ordering and invalid-input boundaries.
3. Implement the checked modules and independently execute the matching corpus
   cases.
4. Add the exhaustive `Event` and `RoutedEvent` surface, migrate existing
   semantic publication tests, and move digest-only helpers out of production.
5. Run focused dependency, corpus, mutation and full OpenSpec validation; record
   discovered and executed counts plus rollback evidence in the change ledger.

Rollback removes the three new event modules, the exhaustive envelope and their
new corpus cases together, then restores the baseline test-only observation
support. The current Go runtime, wire protocol, saves and startup paths remain
unchanged throughout, so rollback requires no data conversion.

## Validation and Completion Evidence

Implementation must follow failing tests, minimum construction logic and
refactoring. Focused Rust suites must enumerate nonzero test and corpus case
counts for every new family and all 30 variants. Go producers must exercise the
current package APIs, not copy expected JSON. Corpus integrity, mutation,
dependency-direction and oversized-input-before-scan cases are hard gates.

The implementation ledger must bind evidence to the exact source SHA and record
the command, discovered cases, executed cases, result and relevant failure-path
proof. Planning validation proves only artifact coherence. Closing this change
does not authorize F2 and does not qualify complete F1 acceptance.
