//! The hostile and passive mob observations.
//!
//! These are the visibility-derived publications an authoritative session
//! sends about the mobs a subscriber can see: a spawn batch when mobs enter
//! the subscribed chunks, a per-tick state batch while they stay visible, and
//! a despawn batch when they leave or die. Both families are projections of
//! authority-side state, not mirrors of it, and the record shapes are the Go
//! `protocol` mob packets, so every rule below is the Go validator's rule for
//! the same record: a record this crate admits is a record the protocol
//! layer admits and the other way around.
//!
//! Two kinds of wire rule deliberately stay out of this file. The packet
//! record maxima — 64 records for each of the hostile and passive batches —
//! are transport budgets the protocol layer applies when it splits a batch
//! for a frame, so the domain bounds semantic work with the shared
//! `MAX_SEMANTIC_BATCH_RECORDS` cap and then requires only a nonempty batch
//! whose identities are strictly increasing. And the raw kind, grazing and
//! despawn-reason bytes belong to the evidence and protocol adapters: the
//! domain stores the closed `HostileKind` and `PassiveDespawnReason` enums
//! and a `bool`, and no type here exposes an enum number.
//!
//! A state record carries no dimension because a dimension change always
//! goes through a despawn/spawn pair, so the mirror keeps the body by
//! identity alone. A hostile despawn carries no reason: the Go packet
//! publishes identity removal only, while the passive despawn publishes the
//! closed vanished/died pair.
//!
//! No record carries cooldown, target, path, AI or capacity state. Those are
//! authority-side simulation quantities the mob publications never put on
//! the wire, and a record that grew one back would turn a visibility
//! observation into a second authority mirror.

use super::player::MAX_HEALTH;
use crate::identity::{DomainError, HostileId, PassiveId};
use crate::values::{Dimension, FiniteVec3};

/// Closed hostile kind set, mirroring the Go kind byte values {0, 1}.
///
/// The mapping from the wire number to the variant belongs to the evidence
/// and protocol adapters; the domain never exposes the number.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum HostileKind {
    Nightwalker,
    BoneThrower,
}

/// Closed passive despawn reason set, mirroring the Go reason byte values
/// {0, 1}.
///
/// The mapping from the wire number to the variant belongs to the evidence
/// and protocol adapters; the domain never exposes the number.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum PassiveDespawnReason {
    Vanished,
    Died,
}

/// One hostile the subscriber can newly see.
///
/// The record is the complete body of one mob at the tick it entered the
/// subscribed chunks: the spawn names the dimension the state batch omits,
/// and the kind because a mob keeps its behaviour category for its whole
/// life.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct HostileSpawnRecord {
    id: HostileId,
    dimension: Dimension,
    position: FiniteVec3,
    yaw: f32,
    health: u8,
    kind: HostileKind,
}

/// Parts of one hostile spawn record.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct HostileSpawnRecordParts {
    pub id: HostileId,
    pub dimension: Dimension,
    pub position: FiniteVec3,
    pub yaw: f32,
    pub health: u8,
    pub kind: HostileKind,
}

impl HostileSpawnRecord {
    /// Wraps one hostile spawn, checking dimension, yaw and health in that
    /// order.
    ///
    /// A hostile spawns in the overworld alone, which is the Go dimension
    /// rule; the yaw is a raw `f32` on this record, so its finiteness is
    /// checked here as `NonFiniteRotation` because a rotation is what the
    /// value names; and health has to sit inside `1..=MAX_HEALTH`, because a
    /// mob at zero health is removed rather than published. Identity and
    /// position finiteness are already enforced by the checked parts. The
    /// order is the Go validator's, so a record violating several rules
    /// reports the first one.
    pub fn try_new(parts: HostileSpawnRecordParts) -> Result<Self, DomainError> {
        if parts.dimension != Dimension::OVERWORLD {
            return Err(DomainError::InvalidDimension);
        }
        if !parts.yaw.is_finite() {
            return Err(DomainError::NonFiniteRotation);
        }
        if parts.health == 0 || parts.health > MAX_HEALTH {
            return Err(DomainError::InvalidSurvivalValue);
        }
        Ok(Self {
            id: parts.id,
            dimension: parts.dimension,
            position: parts.position,
            yaw: parts.yaw,
            health: parts.health,
            kind: parts.kind,
        })
    }

    pub fn id(self) -> HostileId {
        self.id
    }

    pub fn dimension(self) -> Dimension {
        self.dimension
    }

    pub fn position(self) -> FiniteVec3 {
        self.position
    }

    pub fn yaw(self) -> f32 {
        self.yaw
    }

    pub fn health(self) -> u8 {
        self.health
    }

    pub fn kind(self) -> HostileKind {
        self.kind
    }
}

/// One hostile's body at one tick inside a visibility batch.
///
/// The record omits the enclosing tick because the batch owns it, and it
/// omits the dimension because a dimension change always goes through a
/// despawn/spawn pair. The velocity is the per-tick delta the mirror
/// interpolates with; the kind repeats so a state batch alone names the
/// behaviour category of every record it moves.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct HostileStateRecord {
    id: HostileId,
    position: FiniteVec3,
    velocity: FiniteVec3,
    yaw: f32,
    health: u8,
    kind: HostileKind,
}

/// Parts of one hostile state record.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct HostileStateRecordParts {
    pub id: HostileId,
    pub position: FiniteVec3,
    pub velocity: FiniteVec3,
    pub yaw: f32,
    pub health: u8,
    pub kind: HostileKind,
}

impl HostileStateRecord {
    /// Wraps one hostile state record, checking yaw then health.
    ///
    /// The rules are the spawn's minus the dimension one, which the spawn
    /// alone carries, and the order stays the Go validator's: a non-finite
    /// yaw reports `NonFiniteRotation` before an out-of-range health reports
    /// `InvalidSurvivalValue`. Identity and both vector finiteness rules are
    /// already enforced by the checked parts.
    pub fn try_new(parts: HostileStateRecordParts) -> Result<Self, DomainError> {
        if !parts.yaw.is_finite() {
            return Err(DomainError::NonFiniteRotation);
        }
        if parts.health == 0 || parts.health > MAX_HEALTH {
            return Err(DomainError::InvalidSurvivalValue);
        }
        Ok(Self {
            id: parts.id,
            position: parts.position,
            velocity: parts.velocity,
            yaw: parts.yaw,
            health: parts.health,
            kind: parts.kind,
        })
    }

    pub fn id(self) -> HostileId {
        self.id
    }

    pub fn position(self) -> FiniteVec3 {
        self.position
    }

    pub fn velocity(self) -> FiniteVec3 {
        self.velocity
    }

    pub fn yaw(self) -> f32 {
        self.yaw
    }

    pub fn health(self) -> u8 {
        self.health
    }

    pub fn kind(self) -> HostileKind {
        self.kind
    }
}

/// Parts of one hostile spawn batch.
#[derive(Clone, Debug, PartialEq)]
pub struct HostileSpawnParts {
    pub server_tick: u64,
    pub spawns: Box<[HostileSpawnRecord]>,
}

/// The spawn batch for the hostiles entering a subscriber's visibility.
#[derive(Clone, Debug, PartialEq)]
pub struct HostileSpawn {
    server_tick: u64,
    spawns: Box<[HostileSpawnRecord]>,
}

impl HostileSpawn {
    /// Wraps one batch, checking the work cap, emptiness and strict identity
    /// order in that order.
    ///
    /// Every record is already a validated `HostileSpawnRecord`, so this
    /// constructor owns only the batch relations. The 64-record packet
    /// maximum is a transport budget the protocol layer applies, not a
    /// domain rule: the domain bounds work with the shared
    /// `MAX_SEMANTIC_BATCH_RECORDS` cap first, then rejects an empty batch
    /// (`EmptyStateBatch`) and a non-strictly-increasing identity sequence
    /// (`InvalidStateOrder`). Submitted order is preserved with no copy or
    /// sort, and a zero tick is admitted because the tick is the publish
    /// instant rather than a liveness marker.
    pub fn try_new(parts: HostileSpawnParts) -> Result<Self, DomainError> {
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
    pub fn spawns(&self) -> &[HostileSpawnRecord] {
        &self.spawns
    }
}

/// Parts of one hostile state batch.
#[derive(Clone, Debug, PartialEq)]
pub struct HostileStateParts {
    pub server_tick: u64,
    pub states: Box<[HostileStateRecord]>,
}

/// The per-tick body batch for the hostiles one subscriber can see.
#[derive(Clone, Debug, PartialEq)]
pub struct HostileState {
    server_tick: u64,
    states: Box<[HostileStateRecord]>,
}

impl HostileState {
    /// Wraps one batch, applying the same cap, emptiness and strict identity
    /// order sequence as the spawn batch.
    ///
    /// Every record is already a validated `HostileStateRecord`, and the
    /// 64-record packet maximum stays a transport budget in the protocol
    /// layer, so the relations this constructor owns are exactly the shared
    /// batch ones. Submitted order is preserved with no copy or sort, and a
    /// zero tick is admitted.
    pub fn try_new(parts: HostileStateParts) -> Result<Self, DomainError> {
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
    pub fn states(&self) -> &[HostileStateRecord] {
        &self.states
    }
}

/// Parts of one hostile despawn batch.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct HostileDespawnParts {
    pub server_tick: u64,
    pub ids: Box<[HostileId]>,
}

/// The despawn batch for the hostiles leaving a subscriber's visibility.
///
/// The batch carries identities alone because the Go packet publishes
/// identity removal only: a hostile despawn names no reason, no position and
/// no cause, so the mirror has nothing to forget but the body.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct HostileDespawn {
    server_tick: u64,
    ids: Box<[HostileId]>,
}

impl HostileDespawn {
    /// Wraps one batch, applying the same cap, emptiness and strict identity
    /// order sequence as the other hostile batches.
    ///
    /// Every identity is already a checked `HostileId`, so the relations this
    /// constructor owns are exactly the shared batch ones. The 64-record
    /// packet maximum stays a transport budget in the protocol layer, the
    /// submitted order is preserved with no copy or sort, and a zero tick is
    /// admitted.
    pub fn try_new(parts: HostileDespawnParts) -> Result<Self, DomainError> {
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
    pub fn ids(&self) -> &[HostileId] {
        &self.ids
    }
}

/// One passive mob the subscriber can newly see.
///
/// The record is the complete body of one passive mob at the tick it entered
/// the subscribed chunks. It carries no kind because a passive mob has no
/// behaviour category to publish, which is why the hostile record names one
/// and this one does not.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PassiveSpawnRecord {
    id: PassiveId,
    dimension: Dimension,
    position: FiniteVec3,
    yaw: f32,
    health: u8,
}

/// Parts of one passive spawn record.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PassiveSpawnRecordParts {
    pub id: PassiveId,
    pub dimension: Dimension,
    pub position: FiniteVec3,
    pub yaw: f32,
    pub health: u8,
}

impl PassiveSpawnRecord {
    /// Wraps one passive spawn, checking dimension, yaw and health in that
    /// order.
    ///
    /// The rules and the order are the hostile spawn's: the overworld alone,
    /// then a finite yaw (`NonFiniteRotation`), then health inside
    /// `1..=MAX_HEALTH` (`InvalidSurvivalValue`). Identity and position
    /// finiteness are already enforced by the checked parts.
    pub fn try_new(parts: PassiveSpawnRecordParts) -> Result<Self, DomainError> {
        if parts.dimension != Dimension::OVERWORLD {
            return Err(DomainError::InvalidDimension);
        }
        if !parts.yaw.is_finite() {
            return Err(DomainError::NonFiniteRotation);
        }
        if parts.health == 0 || parts.health > MAX_HEALTH {
            return Err(DomainError::InvalidSurvivalValue);
        }
        Ok(Self {
            id: parts.id,
            dimension: parts.dimension,
            position: parts.position,
            yaw: parts.yaw,
            health: parts.health,
        })
    }

    pub fn id(self) -> PassiveId {
        self.id
    }

    pub fn dimension(self) -> Dimension {
        self.dimension
    }

    pub fn position(self) -> FiniteVec3 {
        self.position
    }

    pub fn yaw(self) -> f32 {
        self.yaw
    }

    pub fn health(self) -> u8 {
        self.health
    }
}

/// One passive mob's body at one tick inside a visibility batch.
///
/// The record omits the enclosing tick and the dimension for the same
/// reasons the hostile state record does, and it carries the grazing bit
/// because grazing is a transient presentation observation the authority
/// projects at publish time and never persists.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PassiveStateRecord {
    id: PassiveId,
    position: FiniteVec3,
    velocity: FiniteVec3,
    yaw: f32,
    health: u8,
    grazing: bool,
}

/// Parts of one passive state record.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PassiveStateRecordParts {
    pub id: PassiveId,
    pub position: FiniteVec3,
    pub velocity: FiniteVec3,
    pub yaw: f32,
    pub health: u8,
    pub grazing: bool,
}

impl PassiveStateRecord {
    /// Wraps one passive state record, checking yaw then health.
    ///
    /// The rules and the order are the hostile state's: a finite yaw reports
    /// `NonFiniteRotation` before an out-of-range health reports
    /// `InvalidSurvivalValue`. The grazing bit is stored as the `bool` the
    /// domain owns; the wire's 0/1 conversion belongs to the adapters.
    pub fn try_new(parts: PassiveStateRecordParts) -> Result<Self, DomainError> {
        if !parts.yaw.is_finite() {
            return Err(DomainError::NonFiniteRotation);
        }
        if parts.health == 0 || parts.health > MAX_HEALTH {
            return Err(DomainError::InvalidSurvivalValue);
        }
        Ok(Self {
            id: parts.id,
            position: parts.position,
            velocity: parts.velocity,
            yaw: parts.yaw,
            health: parts.health,
            grazing: parts.grazing,
        })
    }

    pub fn id(self) -> PassiveId {
        self.id
    }

    pub fn position(self) -> FiniteVec3 {
        self.position
    }

    pub fn velocity(self) -> FiniteVec3 {
        self.velocity
    }

    pub fn yaw(self) -> f32 {
        self.yaw
    }

    pub fn health(self) -> u8 {
        self.health
    }

    pub fn grazing(self) -> bool {
        self.grazing
    }
}

/// One passive mob that left the subscriber's visibility.
///
/// Unlike a hostile despawn this record carries a reason, because the Go
/// packet publishes the closed vanished/died pair: a client shows a death
/// differently from a quiet removal.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PassiveDespawnRecord {
    id: PassiveId,
    reason: PassiveDespawnReason,
}

impl PassiveDespawnRecord {
    /// Wraps one passive despawn record.
    ///
    /// Construction is total because every part is already a checked domain
    /// value: the identity is a nonzero `PassiveId` and the reason one of
    /// the two closed variants, which together are the whole Go
    /// `PassiveDespawnRecord.validate` rule.
    pub fn new(id: PassiveId, reason: PassiveDespawnReason) -> Self {
        Self { id, reason }
    }

    pub fn id(self) -> PassiveId {
        self.id
    }

    pub fn reason(self) -> PassiveDespawnReason {
        self.reason
    }
}

/// Parts of one passive spawn batch.
#[derive(Clone, Debug, PartialEq)]
pub struct PassiveSpawnParts {
    pub server_tick: u64,
    pub spawns: Box<[PassiveSpawnRecord]>,
}

/// The spawn batch for the passive mobs entering a subscriber's visibility.
#[derive(Clone, Debug, PartialEq)]
pub struct PassiveSpawn {
    server_tick: u64,
    spawns: Box<[PassiveSpawnRecord]>,
}

impl PassiveSpawn {
    /// Wraps one batch, checking the work cap, emptiness and strict identity
    /// order in that order.
    ///
    /// Every record is already a validated `PassiveSpawnRecord`, so this
    /// constructor owns only the shared batch relations. The 64-record packet
    /// maximum is a transport budget the protocol layer applies, the
    /// submitted order is preserved with no copy or sort, and a zero tick is
    /// admitted.
    pub fn try_new(parts: PassiveSpawnParts) -> Result<Self, DomainError> {
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
    pub fn spawns(&self) -> &[PassiveSpawnRecord] {
        &self.spawns
    }
}

/// Parts of one passive state batch.
#[derive(Clone, Debug, PartialEq)]
pub struct PassiveStateParts {
    pub server_tick: u64,
    pub states: Box<[PassiveStateRecord]>,
}

/// The per-tick body batch for the passive mobs one subscriber can see.
#[derive(Clone, Debug, PartialEq)]
pub struct PassiveState {
    server_tick: u64,
    states: Box<[PassiveStateRecord]>,
}

impl PassiveState {
    /// Wraps one batch, applying the same cap, emptiness and strict identity
    /// order sequence as the other passive batches.
    ///
    /// Every record is already a validated `PassiveStateRecord`, and the
    /// 64-record packet maximum stays a transport budget in the protocol
    /// layer, so the relations this constructor owns are exactly the shared
    /// batch ones. Submitted order is preserved with no copy or sort, and a
    /// zero tick is admitted.
    pub fn try_new(parts: PassiveStateParts) -> Result<Self, DomainError> {
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
    pub fn states(&self) -> &[PassiveStateRecord] {
        &self.states
    }
}

/// Parts of one passive despawn batch.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PassiveDespawnParts {
    pub server_tick: u64,
    pub despawns: Box<[PassiveDespawnRecord]>,
}

/// The despawn batch for the passive mobs leaving a subscriber's visibility.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PassiveDespawn {
    server_tick: u64,
    despawns: Box<[PassiveDespawnRecord]>,
}

impl PassiveDespawn {
    /// Wraps one batch, applying the same cap, emptiness and strict identity
    /// order sequence as the other passive batches.
    ///
    /// Every record is already a checked `PassiveDespawnRecord`, and the
    /// 64-record packet maximum stays a transport budget in the protocol
    /// layer, so the relations this constructor owns are exactly the shared
    /// batch ones. Submitted order is preserved with no copy or sort, and a
    /// zero tick is admitted.
    pub fn try_new(parts: PassiveDespawnParts) -> Result<Self, DomainError> {
        if parts.despawns.len() > crate::MAX_SEMANTIC_BATCH_RECORDS {
            return Err(DomainError::BatchTooLarge);
        }
        if parts.despawns.is_empty() {
            return Err(DomainError::EmptyStateBatch);
        }
        if parts
            .despawns
            .windows(2)
            .any(|pair| pair[0].id >= pair[1].id)
        {
            return Err(DomainError::InvalidStateOrder);
        }
        Ok(Self {
            server_tick: parts.server_tick,
            despawns: parts.despawns,
        })
    }

    pub fn server_tick(&self) -> u64 {
        self.server_tick
    }

    /// The batch's records in the order the authority published them.
    pub fn despawns(&self) -> &[PassiveDespawnRecord] {
        &self.despawns
    }
}
