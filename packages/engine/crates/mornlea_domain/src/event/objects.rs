//! The projectile and item-drop observations.
//!
//! These are the object publications an authoritative session sends about
//! the projectiles in flight and the dropped item stacks one subscriber can
//! see: a projectile spawn batch when a projectile enters the subscribed
//! chunks, a per-tick state batch while it stays visible, and a despawn
//! batch when it hits, expires or leaves; an upserts batch when a drop
//! appears or its stack is replaced wholesale, and a removes batch when it
//! leaves. Both families are projections of authority-side state, and the
//! record shapes are the Go `protocol` object packets, so every rule below
//! is the Go validator's rule for the same record.
//!
//! Two kinds of wire rule deliberately stay out of this file. The packet
//! record maxima — 128 projectile records and 32 drops — are transport
//! budgets the protocol layer applies when it splits a batch for a frame,
//! so the domain bounds semantic work with the shared
//! `MAX_SEMANTIC_BATCH_RECORDS` cap and then requires only a nonempty batch
//! whose identities are strictly increasing. And the raw kind byte belongs
//! to the evidence and protocol adapters: the domain stores the closed
//! `ProjectileKind` enum and no type here exposes an enum number.
//!
//! A projectile spawn record carries the kind because a projectile's
//! behaviour differs by what fired it, and no yaw or health because a
//! projectile is a point-like transient whose orientation the client
//! derives from its velocity. The state record carries neither kind,
//! dimension nor velocity because all three are fixed for the projectile's
//! whole life: the mirror records them at spawn and the state batch only
//! moves the body.
//!
//! An item drop is the complete value of one authoritative dropped stack:
//! the identity, the chunk-local block cell it sits in, and the stack
//! itself through the shared `ItemStack` rule, so a drop cannot publish a
//! slot value the inventory families reject. The identity keeps an
//! arbitrary raw dimension because the Go `core.DropID.Valid` rule checks
//! only the slot range and the generation. No drop record carries a world
//! position, a pickup timer or a despawn countdown: those are
//! authority-side lifecycle quantities the publication never puts on the
//! wire.

use crate::identity::{DomainError, ProjectileId};
use crate::items::ItemStack;
use crate::locations::DropId;
use crate::values::{Dimension, FiniteVec3};

/// Exclusive upper bound of a chunk-local block index, from the Go
/// `protocol` chunk geometry `SectionsPerChunk * BlocksPerSection`: 24
/// sections of 4096 cells. An item drop sits inside the chunk its identity
/// names, so its index has to stay below this bound and the first value
/// outside it names no cell at all.
const MAX_CHUNK_BLOCK_INDEX: u32 = 98_304;

/// Closed projectile kind set, mirroring the Go kind byte values {0, 1}.
///
/// The mapping from the wire number to the variant belongs to the evidence
/// and protocol adapters; the domain never exposes the number.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum ProjectileKind {
    Shard,
    Arrow,
}

/// One projectile the subscriber can newly see.
///
/// The record is the complete flight state at the tick the projectile
/// entered the subscribed chunks: it names the kind because a projectile's
/// behaviour differs by what fired it, the dimension because a player's bow
/// works in either playable dimension, and both the position and the
/// velocity because a projectile is a point-like transient whose
/// orientation the client derives from its velocity, so no yaw or health
/// exists to publish.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ProjectileSpawnRecord {
    id: ProjectileId,
    kind: ProjectileKind,
    dimension: Dimension,
    position: FiniteVec3,
    velocity: FiniteVec3,
}

/// Parts of one projectile spawn record.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ProjectileSpawnRecordParts {
    pub id: ProjectileId,
    pub kind: ProjectileKind,
    pub dimension: Dimension,
    pub position: FiniteVec3,
    pub velocity: FiniteVec3,
}

impl ProjectileSpawnRecord {
    /// Wraps one projectile spawn.
    ///
    /// Construction is total because every part is already a checked domain
    /// value: the identity is a nonzero `ProjectileId`, the kind one of the
    /// two closed variants, the dimension one of the two playable ones, and
    /// both vectors finite `FiniteVec3` values. Both kinds are legal in
    /// both dimensions — the kind-by-dimension policy is an authority
    /// concern the wire and this domain do not enforce.
    pub fn new(parts: ProjectileSpawnRecordParts) -> Self {
        Self {
            id: parts.id,
            kind: parts.kind,
            dimension: parts.dimension,
            position: parts.position,
            velocity: parts.velocity,
        }
    }

    pub fn id(self) -> ProjectileId {
        self.id
    }

    pub fn kind(self) -> ProjectileKind {
        self.kind
    }

    pub fn dimension(self) -> Dimension {
        self.dimension
    }

    pub fn position(self) -> FiniteVec3 {
        self.position
    }

    pub fn velocity(self) -> FiniteVec3 {
        self.velocity
    }
}

/// One projectile's position at one tick inside a visibility batch.
///
/// The record omits the enclosing tick because the batch owns it, and it
/// carries no kind, dimension or velocity because all three are fixed for
/// the projectile's whole life: the mirror records them at spawn and the
/// state batch only moves the body.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ProjectileStateRecord {
    id: ProjectileId,
    position: FiniteVec3,
}

/// Parts of one projectile state record.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ProjectileStateRecordParts {
    pub id: ProjectileId,
    pub position: FiniteVec3,
}

impl ProjectileStateRecord {
    /// Wraps one projectile state record.
    ///
    /// Construction is total because every part is already a checked domain
    /// value: the identity is a nonzero `ProjectileId` and the position a
    /// finite `FiniteVec3`.
    pub fn new(parts: ProjectileStateRecordParts) -> Self {
        Self {
            id: parts.id,
            position: parts.position,
        }
    }

    pub fn id(self) -> ProjectileId {
        self.id
    }

    pub fn position(self) -> FiniteVec3 {
        self.position
    }
}

/// Parts of one projectile spawn batch.
#[derive(Clone, Debug, PartialEq)]
pub struct ProjectileSpawnParts {
    pub server_tick: u64,
    pub spawns: Box<[ProjectileSpawnRecord]>,
}

/// The spawn batch for the projectiles entering a subscriber's visibility.
#[derive(Clone, Debug, PartialEq)]
pub struct ProjectileSpawn {
    server_tick: u64,
    spawns: Box<[ProjectileSpawnRecord]>,
}

impl ProjectileSpawn {
    /// Wraps one batch, checking the work cap, emptiness and strict identity
    /// order in that order.
    ///
    /// Every record is already a validated `ProjectileSpawnRecord`, so this
    /// constructor owns only the batch relations. The 128-record packet
    /// maximum is a transport budget the protocol layer applies, not a
    /// domain rule: the domain bounds work with the shared
    /// `MAX_SEMANTIC_BATCH_RECORDS` cap first, then rejects an empty batch
    /// (`EmptyStateBatch`) and a non-strictly-increasing identity sequence
    /// (`InvalidStateOrder`). Submitted order is preserved with no copy or
    /// sort, and a zero tick is admitted because the tick is the publish
    /// instant rather than a liveness marker.
    pub fn try_new(parts: ProjectileSpawnParts) -> Result<Self, DomainError> {
        if parts.spawns.len() > crate::MAX_SEMANTIC_BATCH_RECORDS {
            return Err(DomainError::BatchTooLarge);
        }
        if parts.spawns.is_empty() {
            return Err(DomainError::EmptyStateBatch);
        }
        if parts.spawns.windows(2).any(|pair| pair[0].id >= pair[1].id) {
            return Err(DomainError::InvalidStateOrder);
        }
        Ok(Self {
            server_tick: parts.server_tick,
            spawns: parts.spawns,
        })
    }

    pub fn server_tick(&self) -> u64 {
        self.server_tick
    }

    /// The batch's records in the order the authority published them.
    pub fn spawns(&self) -> &[ProjectileSpawnRecord] {
        &self.spawns
    }
}

/// Parts of one projectile state batch.
#[derive(Clone, Debug, PartialEq)]
pub struct ProjectileStateParts {
    pub server_tick: u64,
    pub states: Box<[ProjectileStateRecord]>,
}

/// The per-tick position batch for the projectiles one subscriber can see.
#[derive(Clone, Debug, PartialEq)]
pub struct ProjectileState {
    server_tick: u64,
    states: Box<[ProjectileStateRecord]>,
}

impl ProjectileState {
    /// Wraps one batch, applying the same cap, emptiness and strict identity
    /// order sequence as the spawn batch.
    ///
    /// Every record is already a validated `ProjectileStateRecord`, and the
    /// 128-record packet maximum stays a transport budget in the protocol
    /// layer, so the relations this constructor owns are exactly the shared
    /// batch ones. Submitted order is preserved with no copy or sort, and a
    /// zero tick is admitted.
    pub fn try_new(parts: ProjectileStateParts) -> Result<Self, DomainError> {
        if parts.states.len() > crate::MAX_SEMANTIC_BATCH_RECORDS {
            return Err(DomainError::BatchTooLarge);
        }
        if parts.states.is_empty() {
            return Err(DomainError::EmptyStateBatch);
        }
        if parts.states.windows(2).any(|pair| pair[0].id >= pair[1].id) {
            return Err(DomainError::InvalidStateOrder);
        }
        Ok(Self {
            server_tick: parts.server_tick,
            states: parts.states,
        })
    }

    pub fn server_tick(&self) -> u64 {
        self.server_tick
    }

    /// The batch's records in the order the authority published them.
    pub fn states(&self) -> &[ProjectileStateRecord] {
        &self.states
    }
}

/// Parts of one projectile despawn batch.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ProjectileDespawnParts {
    pub server_tick: u64,
    pub ids: Box<[ProjectileId]>,
}

/// The despawn batch for the projectiles leaving a subscriber's visibility.
///
/// The batch carries identities alone because the Go packet publishes
/// identity removal only: a projectile despawn names no reason, no position
/// and no cause, so the mirror has nothing to forget but the body.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ProjectileDespawn {
    server_tick: u64,
    ids: Box<[ProjectileId]>,
}

impl ProjectileDespawn {
    /// Wraps one batch, applying the same cap, emptiness and strict identity
    /// order sequence as the other projectile batches.
    ///
    /// Every identity is already a checked `ProjectileId`, so the relations
    /// this constructor owns are exactly the shared batch ones. The
    /// 128-record packet maximum stays a transport budget in the protocol
    /// layer, the submitted order is preserved with no copy or sort, and a
    /// zero tick is admitted.
    pub fn try_new(parts: ProjectileDespawnParts) -> Result<Self, DomainError> {
        if parts.ids.len() > crate::MAX_SEMANTIC_BATCH_RECORDS {
            return Err(DomainError::BatchTooLarge);
        }
        if parts.ids.is_empty() {
            return Err(DomainError::EmptyStateBatch);
        }
        if parts.ids.windows(2).any(|pair| pair[0] >= pair[1]) {
            return Err(DomainError::InvalidStateOrder);
        }
        Ok(Self {
            server_tick: parts.server_tick,
            ids: parts.ids,
        })
    }

    pub fn server_tick(&self) -> u64 {
        self.server_tick
    }

    /// The batch's identities in the order the authority published them.
    pub fn ids(&self) -> &[ProjectileId] {
        &self.ids
    }
}

/// Parts of one dropped authoritative item stack.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ItemDropParts {
    pub id: DropId,
    pub block_index: u32,
    pub stack: ItemStack,
}

/// The complete value of one authoritative dropped stack.
///
/// The record names the identity, the chunk-local block cell the stack sits
/// in and the stack itself. It carries no world position — the block index
/// is the cell inside the chunk the identity names, not a world coordinate —
/// and no pickup timer, despawn countdown or velocity, which are
/// authority-side lifecycle quantities the publication never puts on the
/// wire.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ItemDrop {
    id: DropId,
    block_index: u32,
    stack: ItemStack,
}

impl ItemDrop {
    /// Wraps one dropped stack, checking the block index.
    ///
    /// The index has to stay below the chunk's cell count: a drop sits
    /// inside the chunk its identity names, and the first value outside the
    /// bound is rejected as `InvalidBlockIndex` rather than clamped to the
    /// last cell, so a caller can never mistake a rejected drop for one
    /// inside the chunk. The identity and the stack are already checked
    /// domain values — the raw dimension stays untouched and the stack rule
    /// is exactly the shared `ItemStack` one — so the index bound is the
    /// only relation left to check.
    pub fn try_new(parts: ItemDropParts) -> Result<Self, DomainError> {
        if parts.block_index >= MAX_CHUNK_BLOCK_INDEX {
            return Err(DomainError::InvalidBlockIndex);
        }
        Ok(Self {
            id: parts.id,
            block_index: parts.block_index,
            stack: parts.stack,
        })
    }

    pub fn id(self) -> DropId {
        self.id
    }

    pub fn block_index(self) -> u32 {
        self.block_index
    }

    pub fn stack(self) -> ItemStack {
        self.stack
    }
}

/// Parts of one item-drop upserts batch.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ItemDropUpsertsParts {
    pub server_tick: u64,
    pub drops: Box<[ItemDrop]>,
}

/// The batch that adds or wholly replaces the drops one subscriber can see.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ItemDropUpserts {
    server_tick: u64,
    drops: Box<[ItemDrop]>,
}

impl ItemDropUpserts {
    /// Wraps one batch, checking the work cap, emptiness and strict identity
    /// order in that order.
    ///
    /// Every drop is already a validated `ItemDrop`, so this constructor
    /// owns only the batch relations. The 32-record packet maximum is a
    /// transport budget the protocol layer applies, not a domain rule: the
    /// domain bounds work with the shared `MAX_SEMANTIC_BATCH_RECORDS` cap
    /// first, then rejects an empty batch (`EmptyStateBatch`) and a
    /// non-strictly-increasing identity sequence (`InvalidStateOrder`). The
    /// order key is the derived `DropId` ordering — dimension, chunk column,
    /// slot, generation — compared exactly as submitted, with no copy, sort
    /// or dimension normalization. A zero tick is admitted because the tick
    /// is the publish instant rather than a liveness marker.
    pub fn try_new(parts: ItemDropUpsertsParts) -> Result<Self, DomainError> {
        if parts.drops.len() > crate::MAX_SEMANTIC_BATCH_RECORDS {
            return Err(DomainError::BatchTooLarge);
        }
        if parts.drops.is_empty() {
            return Err(DomainError::EmptyStateBatch);
        }
        if parts.drops.windows(2).any(|pair| pair[0].id >= pair[1].id) {
            return Err(DomainError::InvalidStateOrder);
        }
        Ok(Self {
            server_tick: parts.server_tick,
            drops: parts.drops,
        })
    }

    pub fn server_tick(&self) -> u64 {
        self.server_tick
    }

    /// The batch's drops in the order the authority published them.
    pub fn drops(&self) -> &[ItemDrop] {
        &self.drops
    }
}

/// Parts of one item-drop removes batch.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ItemDropRemovesParts {
    pub server_tick: u64,
    pub ids: Box<[DropId]>,
}

/// The batch that removes the drops one subscriber can no longer see.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ItemDropRemoves {
    server_tick: u64,
    ids: Box<[DropId]>,
}

impl ItemDropRemoves {
    /// Wraps one batch, applying the same cap, emptiness and strict identity
    /// order sequence as the upserts batch.
    ///
    /// Every identity is already a checked `DropId`, so the relations this
    /// constructor owns are exactly the shared batch ones, with the same
    /// derived `DropId` ordering the upserts batch compares. The 32-record
    /// packet maximum stays a transport budget in the protocol layer, the
    /// submitted order is preserved with no copy or sort, and a zero tick is
    /// admitted.
    pub fn try_new(parts: ItemDropRemovesParts) -> Result<Self, DomainError> {
        if parts.ids.len() > crate::MAX_SEMANTIC_BATCH_RECORDS {
            return Err(DomainError::BatchTooLarge);
        }
        if parts.ids.is_empty() {
            return Err(DomainError::EmptyStateBatch);
        }
        if parts.ids.windows(2).any(|pair| pair[0] >= pair[1]) {
            return Err(DomainError::InvalidStateOrder);
        }
        Ok(Self {
            server_tick: parts.server_tick,
            ids: parts.ids,
        })
    }

    pub fn server_tick(&self) -> u64 {
        self.server_tick
    }

    /// The batch's identities in the order the authority published them.
    pub fn ids(&self) -> &[DropId] {
        &self.ids
    }
}
