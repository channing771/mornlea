## ADDED Requirements

### Requirement: Remaining publication families use checked semantic values

The Rust domain SHALL provide checked values for hostile mobs, passive mobs,
projectiles, item drops and chat publications. Accepted values MUST preserve
every semantic field and the validated record order of the current Go source
contract without carrying packet identifiers, wire padding, persistence policy,
authority-only state or presentation state. Hostile and passive spawns MUST be
overworld values; projectile spawns MUST accept both supported dimensions and
MUST NOT impose an authority-only kind-by-dimension rule. Item drops MUST retain
the raw dimension semantics of `DropId`, MUST accept the valid empty
`ItemStack`, and MUST reject a block index outside the 98,304-cell chunk. Chat
MUST be a closed tagged union in which invalid kind, reason, identity, command
and speech combinations are unrepresentable after construction.

#### Scenario: A valid remaining publication is constructed

- **WHEN** a Go-valid hostile, passive, projectile, item-drop or chat
  publication is converted into its Rust domain value
- **THEN** every semantic field and record position MUST be preserved
- **AND** the result MUST contain no packet ID, opaque wire bytes, digest-only
  surrogate or authority-only state

#### Scenario: Projectile policy stays with the authority

- **WHEN** a projectile spawn contains either supported projectile kind in
  either the overworld or depths
- **THEN** the domain constructor MUST accept the combination when its identity,
  position and velocity are otherwise valid
- **AND** any narrower kind-by-dimension rule MUST remain outside the shared
  semantic value

#### Scenario: A wire-shaped chat combination is semantically illegal

- **WHEN** a chat source carries an unknown kind or reason, a missing required
  identity, command text on speech, speech text on a non-speech event, or a
  task-failure reason on another event
- **THEN** conversion MUST reject the complete event before publication

### Requirement: The semantic event surface is exhaustive and wire-independent

The production domain SHALL expose an exhaustive `Event` enum with exactly
these 30 variants: `ChunkSnapshot`, `BlockChanges`, `ForgetChunks`,
`PlayerState`, `CommandRejected`, `RemotePlayerSpawn`,
`RemotePlayerDespawn`, `RemotePlayerStates`, `InventoryState`,
`ItemDropUpserts`, `ItemDropRemoves`, `FurnaceState`, `ContainerClosed`,
`ChestState`, `Chat`, `CompanionSpawn`, `CompanionStates`,
`CompanionDespawn`, `PlaceBlockSucceeded`, `CraftingState`, `HostileSpawn`,
`HostileState`, `HostileDespawn`, `CombatHit`, `PassiveSpawn`, `PassiveState`,
`PassiveDespawn`, `ProjectileSpawn`, `ProjectileState` and
`ProjectileDespawn`. Publication routing SHALL be a separate `RoutedEvent`
envelope with `EventRecipient::Session(u64)` and
`EventRecipient::Broadcast`. The surface MUST exclude transport lifecycle
messages and runtime worker lifecycle messages, and MUST NOT provide an opaque,
digest or catch-all variant.

#### Scenario: Every semantic publication is represented

- **WHEN** the event-surface contract suite enumerates the public `Event` union
- **THEN** each of the 30 named semantic variants MUST be constructed and
  observed exactly once
- **AND** the contained value MUST be the corresponding checked domain value

#### Scenario: Routing does not alter semantic values

- **WHEN** a checked event is addressed to one runtime session or broadcast
- **THEN** the recipient MUST be carried only by `RoutedEvent`
- **AND** the domain MUST NOT validate session existence, subscription state or
  visibility policy

#### Scenario: A lifecycle message reaches the domain boundary

- **WHEN** a handshake, login, rejection, keepalive, disconnect, chunk-worker
  acquisition, generation, readiness or resynchronization message is considered
- **THEN** it MUST remain outside `Event`
- **AND** it MUST NOT be hidden behind a generic byte or digest variant

### Requirement: Remaining semantic batches are bounded and atomic

Every hostile, passive, projectile and item-drop semantic batch SHALL own its
records, SHALL contain between 1 and 4,096 records, and SHALL preserve strictly
increasing typed identity order. The 4,096-record semantic cap MUST be checked
before any per-record scan, copy or sort. A duplicate, reversed, invalid or
oversized record MUST reject the complete batch without publishing a partial
value. Protocol packet maxima such as 32, 64 and 128 records MUST remain
protocol-owned and MUST NOT narrow the domain batch type.

#### Scenario: An oversized remaining batch is submitted

- **WHEN** a safe Rust caller submits 4,097 hostile, passive, projectile or
  item-drop records
- **THEN** construction MUST return the bounded-batch failure before inspecting
  an individual record
- **AND** no partial batch may be published

#### Scenario: Record identity order is not canonical

- **WHEN** a nonempty batch contains a duplicate identity or a later record
  whose typed identity is lower than its predecessor
- **THEN** construction MUST reject the complete batch without reordering it

#### Scenario: A domain-valid batch exceeds one packet limit

- **WHEN** a batch contains no more than 4,096 valid records but exceeds its
  protocol packet maximum
- **THEN** the domain value MAY be constructed
- **AND** the protocol adapter MUST remain responsible for rejecting or
  partitioning it according to the separately versioned wire contract

### Requirement: Remaining event evidence executes independently in Go and Rust

The frozen runtime-migration corpus SHALL obtain expected remaining-event
outcomes from current package-local Go validators, and the Rust consumer SHALL
execute the corresponding checked constructors and exhaustive event union
independently. Wire codec evidence remains owned by the later protocol
successor. Every selected case MUST execute exactly once;
zero-case discovery MUST fail qualification. The evidence SHALL cover every
legal chat branch, boundary and rejection cases for every new record family,
strict ordering, semantic-versus-wire limits, and mutations of individual
semantic fields. Producers and tests MUST write only to harness-owned temporary
or external export storage and MUST NOT rewrite tracked corpus evidence.

#### Scenario: Go and Rust agree on the remaining families

- **WHEN** the recorded remaining-event corpus is executed at its bound source
  identity
- **THEN** each selected Go producer and Rust consumer case MUST run exactly
  once
- **AND** every normalized accepted value or typed rejection MUST match

#### Scenario: One semantic field is mutated

- **WHEN** one expected kind, health, grazing state, despawn reason, dimension,
  velocity, block index, identity order or chat branch differs from the value
  produced by Rust
- **THEN** the event evidence gate MUST fail even if every file and ownership
  identifier remains present

#### Scenario: A corpus generator is aimed at tracked evidence

- **WHEN** the remaining-event producer receives an output path inside the
  repository or through a symbolic-link ancestor
- **THEN** it MUST reject the request without changing a tracked manifest or
  case asset
