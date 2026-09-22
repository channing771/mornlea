//! The remote-player and companion observations.
//!
//! These are the records an authoritative session publishes about the other
//! players and the companions one subscriber can see. A remote player appears
//! when it enters the subscriber's visibility, continues with a per-tick state
//! batch while it stays visible, and disappears when it leaves; a companion
//! follows the same three-shape life cycle. Both are visibility projections of
//! authority-side state, not mirrors of it.
//!
//! Every rule below is the Go `protocol` validator's rule for the same record,
//! so a record this crate admits is a record the protocol layer admits and the
//! other way around. Four Go wire rules deliberately stay out of this file.
//! The 7-record remote-player batch maximum and the 4-record companion batch
//! maximum are transport budgets the protocol layer applies; the domain bounds
//! only semantic work with the shared `MAX_SEMANTIC_BATCH_RECORDS` cap, then
//! requires a nonempty batch whose identities are strictly increasing.
//! The publish tick a record carries is the enclosing batch's tick, renamed
//! `server_tick` so the one name means the one concept across the crate. And
//! the flattened yaw and pitch are one already-validated `LookAngles` named
//! `look`, so no record carries a rotation the finiteness rule has not seen.
//!
//! The per-record rules differ by subject, and the difference is the Go one
//! rather than a symmetry: a remote-player record accepts either playable
//! dimension and any finite pitch, because a peer session is not the local
//! player and no vertical look bound is published for it, while a companion
//! record accepts the overworld alone and a pitch inside the inclusive
//! vertical look range.
//!
//! No record in this file carries a profile, a persona, mining progress or a
//! velocity. Those are authority-side quantities the Go publications drop:
//! `CompanionUpdate` keeps `velocity`, `ground` and `mining` for the
//! simulation, and the remote-player publication drops the survival,
//! inventory and mining fields the private player publication owns. A record
//! that grew one of them back would turn a visibility observation into a
//! second authority mirror.

use crate::identity::{CompanionId, DomainError, PlayerId};
use crate::text::{CompanionName, DisplayName};
use crate::values::{Dimension, FiniteVec3, LookAngles};

/// Inclusive vertical look limit of a companion record, from the Go
/// `validCompanionPose` bound `math.Pi/2`.
///
/// The bound is inclusive at both ends, and it is a companion rule only: a
/// remote-player record publishes no pitch bound at all, so a pitch a
/// companion record refuses is publishable for a peer session.
const COMPANION_PITCH_LIMIT: f32 = core::f32::consts::FRAC_PI_2;

/// Reports whether one pitch is inside the inclusive companion vertical look
/// range.
///
/// The comparison is on the exact `f32` the wire carries, so the limit itself
/// is admitted and the next representable value above it is not.
fn companion_pitch_admits(pitch: f32) -> bool {
    (-COMPANION_PITCH_LIMIT..=COMPANION_PITCH_LIMIT).contains(&pitch)
}

/// One remote player the subscriber can newly see.
///
/// The record is the complete identity and body of a peer session at the tick
/// it became visible, which is why it carries the name a spawn needs and the
/// state batch does not repeat.
#[derive(Clone, Debug, PartialEq)]
pub struct RemotePlayerSpawn {
    player_id: PlayerId,
    display_name: DisplayName,
    server_tick: u64,
    dimension: Dimension,
    position: FiniteVec3,
    look: LookAngles,
}

/// Parts of one remote-player spawn.
#[derive(Clone, Debug, PartialEq)]
pub struct RemotePlayerSpawnParts {
    pub player_id: PlayerId,
    pub display_name: DisplayName,
    pub server_tick: u64,
    pub dimension: Dimension,
    pub position: FiniteVec3,
    pub look: LookAngles,
}

impl RemotePlayerSpawn {
    /// Wraps one remote-player spawn.
    ///
    /// Construction is total and named `new` rather than `try_new` because
    /// every part is an already-validated domain value: the identity is a
    /// checked `PlayerId`, the name a canonical `DisplayName`, the dimension
    /// one of the two playable ones, and the pose and the look already finite.
    /// Together those are the exact Go `RemotePlayerSpawn.Validate` rule,
    /// including the absent pitch bound and the admitted zero tick.
    pub fn new(parts: RemotePlayerSpawnParts) -> Self {
        Self {
            player_id: parts.player_id,
            display_name: parts.display_name,
            server_tick: parts.server_tick,
            dimension: parts.dimension,
            position: parts.position,
            look: parts.look,
        }
    }

    pub fn player_id(&self) -> PlayerId {
        self.player_id
    }

    pub fn display_name(&self) -> &DisplayName {
        &self.display_name
    }

    pub fn server_tick(&self) -> u64 {
        self.server_tick
    }

    pub fn dimension(&self) -> Dimension {
        self.dimension
    }

    pub fn position(&self) -> FiniteVec3 {
        self.position
    }

    pub fn look(&self) -> LookAngles {
        self.look
    }
}

/// One remote player that left the subscriber's visibility.
///
/// The Go packet is derived from the visibility difference rather than from a
/// liveness event, so it carries no reason and no position: the identity alone
/// names what the mirror has to forget.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct RemotePlayerDespawn {
    player_id: PlayerId,
}

impl RemotePlayerDespawn {
    /// Wraps one remote-player despawn.
    ///
    /// Construction is total because the identity is already a checked
    /// `PlayerId`, which is the whole Go `RemotePlayerDespawn.Validate` rule.
    pub fn new(player_id: PlayerId) -> Self {
        Self { player_id }
    }

    pub fn player_id(self) -> PlayerId {
        self.player_id
    }
}

/// One remote player's body at one tick inside a visibility batch.
///
/// The record omits the enclosing tick because the batch owns it: a batch is
/// one publish instant, so a record carrying its own tick could disagree with
/// the batch that holds it.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct RemotePlayerState {
    player_id: PlayerId,
    dimension: Dimension,
    position: FiniteVec3,
    look: LookAngles,
    reset: bool,
}

/// Parts of one remote-player state record.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct RemotePlayerStateParts {
    pub player_id: PlayerId,
    pub dimension: Dimension,
    pub position: FiniteVec3,
    pub look: LookAngles,
    pub reset: bool,
}

impl RemotePlayerState {
    /// Wraps one remote-player state record.
    ///
    /// Construction is total for the same reason as the spawn: every part is
    /// already validated, and the Go `RemotePlayerState.validate` rule adds
    /// nothing beyond that. The reset bit is the per-record replay marker a
    /// mirror needs, so the record keeps it rather than the batch collapsing
    /// it into one flag.
    pub fn new(parts: RemotePlayerStateParts) -> Self {
        Self {
            player_id: parts.player_id,
            dimension: parts.dimension,
            position: parts.position,
            look: parts.look,
            reset: parts.reset,
        }
    }

    pub fn player_id(self) -> PlayerId {
        self.player_id
    }

    pub fn dimension(self) -> Dimension {
        self.dimension
    }

    pub fn position(self) -> FiniteVec3 {
        self.position
    }

    pub fn look(self) -> LookAngles {
        self.look
    }

    pub fn reset(self) -> bool {
        self.reset
    }
}

/// Parts of one remote-player state batch.
#[derive(Clone, Debug, PartialEq)]
pub struct RemotePlayerStatesParts {
    pub server_tick: u64,
    pub states: Box<[RemotePlayerState]>,
}

/// The per-tick body batch for the remote players one subscriber can see.
#[derive(Clone, Debug, PartialEq)]
pub struct RemotePlayerStates {
    server_tick: u64,
    states: Box<[RemotePlayerState]>,
}

impl RemotePlayerStates {
    /// Wraps one batch, requiring a nonempty batch of strictly increasing
    /// identities.
    ///
    /// The order is the Go rule: the authority publishes the batch sorted by
    /// raw UUID bytes, so a mirror that applied the records in the order given
    /// would reach the same end state only if the order is the published one.
    /// Strictness rules out a duplicate as well, which would otherwise apply
    /// one record on top of itself. A zero tick is admitted because the tick is
    /// the publish instant rather than a liveness marker.
    ///
    /// The 7-record wire maximum is not checked here: it is a transport budget
    /// the protocol layer applies when it splits a batch for a frame, and a
    /// domain rule refusing eight records would disagree with an authority
    /// that publishes them in two frames. The shared
    /// `MAX_SEMANTIC_BATCH_RECORDS` work cap is the domain's own bound and is
    /// checked before the emptiness and identity-order relations.
    pub fn try_new(parts: RemotePlayerStatesParts) -> Result<Self, DomainError> {
        if parts.states.len() > crate::MAX_SEMANTIC_BATCH_RECORDS {
            return Err(DomainError::BatchTooLarge);
        }
        if parts.states.is_empty() {
            return Err(DomainError::EmptyStateBatch);
        }
        if parts
            .states
            .windows(2)
            .any(|pair| pair[0].player_id >= pair[1].player_id)
        {
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
    pub fn states(&self) -> &[RemotePlayerState] {
        &self.states
    }
}

/// One companion the subscriber can newly see.
///
/// The Go spawn joins `companion.Definition` with the body, so the record
/// carries the name and the identity and nothing else from the definition: the
/// profile and the persona stay in the Agent service and never reach the wire.
#[derive(Clone, Debug, PartialEq)]
pub struct CompanionSpawn {
    id: CompanionId,
    name: CompanionName,
    server_tick: u64,
    dimension: Dimension,
    position: FiniteVec3,
    look: LookAngles,
}

/// Parts of one companion spawn.
#[derive(Clone, Debug, PartialEq)]
pub struct CompanionSpawnParts {
    pub id: CompanionId,
    pub name: CompanionName,
    pub server_tick: u64,
    pub dimension: Dimension,
    pub position: FiniteVec3,
    pub look: LookAngles,
}

impl CompanionSpawn {
    /// Wraps one companion spawn, checking the two companion-only rules.
    ///
    /// A companion lives in the overworld alone and looks inside the inclusive
    /// vertical range, which are the Go `CompanionSpawn.Validate` dimension
    /// rule and its `validCompanionPose` pitch rule. Everything else is
    /// already validated: the identity is a checked `CompanionId`, the name a
    /// canonical `CompanionName`, and the pose and the look already finite. A
    /// zero tick is admitted, exactly as on the remote-player records.
    pub fn try_new(parts: CompanionSpawnParts) -> Result<Self, DomainError> {
        if parts.dimension != Dimension::OVERWORLD {
            return Err(DomainError::InvalidCompanionDimension);
        }
        if !companion_pitch_admits(parts.look.pitch()) {
            return Err(DomainError::InvalidCompanionPitch);
        }
        Ok(Self {
            id: parts.id,
            name: parts.name,
            server_tick: parts.server_tick,
            dimension: parts.dimension,
            position: parts.position,
            look: parts.look,
        })
    }

    pub fn id(&self) -> CompanionId {
        self.id
    }

    pub fn name(&self) -> &CompanionName {
        &self.name
    }

    pub fn server_tick(&self) -> u64 {
        self.server_tick
    }

    pub fn dimension(&self) -> Dimension {
        self.dimension
    }

    pub fn position(&self) -> FiniteVec3 {
        self.position
    }

    pub fn look(&self) -> LookAngles {
        self.look
    }
}

/// One companion that left the subscriber's visibility.
///
/// Like the remote-player despawn this is derived from the visibility
/// difference, so it carries the identity alone.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CompanionDespawn {
    id: CompanionId,
}

impl CompanionDespawn {
    /// Wraps one companion despawn.
    ///
    /// Construction is total because the identity is already a checked
    /// `CompanionId`, which is the whole Go `CompanionDespawn.Validate` rule.
    pub fn new(id: CompanionId) -> Self {
        Self { id }
    }

    pub fn id(self) -> CompanionId {
        self.id
    }
}

/// One companion's body at one tick inside a visibility batch.
///
/// The record omits the enclosing tick for the same reason as the
/// remote-player record, and it omits the velocity, the ground bit and the
/// mining block the Go `CompanionUpdate` keeps for the simulation: those are
/// authority-side quantities the companion wire never carries.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct CompanionState {
    id: CompanionId,
    dimension: Dimension,
    position: FiniteVec3,
    look: LookAngles,
    reset: bool,
}

/// Parts of one companion state record.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct CompanionStateParts {
    pub id: CompanionId,
    pub dimension: Dimension,
    pub position: FiniteVec3,
    pub look: LookAngles,
    pub reset: bool,
}

impl CompanionState {
    /// Wraps one companion state record, checking the same two companion-only
    /// rules the spawn checks.
    ///
    /// The rules are shared with `CompanionSpawn::try_new` because the Go
    /// `CompanionState.validate` reads the same `validCompanionPose` predicate
    /// and the same overworld-only dimension rule, so a record the batch
    /// publishes can never be one the spawn would have refused.
    pub fn try_new(parts: CompanionStateParts) -> Result<Self, DomainError> {
        if parts.dimension != Dimension::OVERWORLD {
            return Err(DomainError::InvalidCompanionDimension);
        }
        if !companion_pitch_admits(parts.look.pitch()) {
            return Err(DomainError::InvalidCompanionPitch);
        }
        Ok(Self {
            id: parts.id,
            dimension: parts.dimension,
            position: parts.position,
            look: parts.look,
            reset: parts.reset,
        })
    }

    pub fn id(self) -> CompanionId {
        self.id
    }

    pub fn dimension(self) -> Dimension {
        self.dimension
    }

    pub fn position(self) -> FiniteVec3 {
        self.position
    }

    pub fn look(self) -> LookAngles {
        self.look
    }

    pub fn reset(self) -> bool {
        self.reset
    }
}

/// Parts of one companion state batch.
#[derive(Clone, Debug, PartialEq)]
pub struct CompanionStatesParts {
    pub server_tick: u64,
    pub states: Box<[CompanionState]>,
}

/// The per-tick body batch for the companions one subscriber can see.
#[derive(Clone, Debug, PartialEq)]
pub struct CompanionStates {
    server_tick: u64,
    states: Box<[CompanionState]>,
}

impl CompanionStates {
    /// Wraps one companion batch, requiring a nonempty batch of strictly
    /// increasing identities.
    ///
    /// Every record is already a validated `CompanionState`, so the two
    /// companion-only rules were checked when the record was built and this
    /// constructor owns only the batch relations: the Go
    /// `CompanionStates.Validate` count and order rules. A zero tick is
    /// admitted, exactly as on the remote-player batch.
    ///
    /// The 4-record wire maximum is not checked here: it is the companion
    /// activity limit the protocol layer applies, and a domain rule refusing a
    /// fifth record would disagree with an authority that publishes the
    /// companions it has in more than one frame. The shared
    /// `MAX_SEMANTIC_BATCH_RECORDS` work cap is the domain's own bound and is
    /// checked before the emptiness and identity-order relations.
    pub fn try_new(parts: CompanionStatesParts) -> Result<Self, DomainError> {
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
    pub fn states(&self) -> &[CompanionState] {
        &self.states
    }
}
