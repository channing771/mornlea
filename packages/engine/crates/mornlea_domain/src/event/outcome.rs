//! Command-outcome and combat-hit publication records.
//!
//! These are the results an authoritative tick publishes to the session that
//! caused them: a command the authority refused, a placement it confirmed, and
//! a melee hit that landed. Each record names one command or one hit and
//! nothing else. No record carries a target identity, a placed block, a
//! consumed item or any world state, because those stay server-owned; the
//! records only say what happened to what the session asked for.

use crate::event::player::MAX_HEALTH;
use crate::identity::DomainError;

/// One command the authority refused, published to the session that sent it.
///
/// The Go publication strips the routing session before the packet is written,
/// so the record carries the sequence and the reason alone.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CommandRejection {
    sequence: u64,
    reason: RejectReason,
}

impl CommandRejection {
    /// Wraps one rejection.
    ///
    /// A zero sequence is preserved rather than rejected. The Go protocol
    /// admits it, and the `/warp` rejects are published with sequence zero
    /// outside the tick result, so a domain rule refusing zero would disagree
    /// with the authority. The reason is already one of the fifteen published
    /// variants, so nothing is left for this constructor to check.
    pub fn new(sequence: u64, reason: RejectReason) -> Self {
        Self { sequence, reason }
    }

    pub fn sequence(self) -> u64 {
        self.sequence
    }

    pub fn reason(self) -> RejectReason {
        self.reason
    }
}

/// Confirmation that one `PlaceBlock` command completed atomically.
///
/// The Go packet confirms that the world write and the exactly-one inventory
/// decrement of the same sequence landed in one authoritative tick, and it is
/// routed to the issuing session only. It therefore expresses no other
/// command's success and no prediction state.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PlacementSuccess {
    sequence: u64,
}

impl PlacementSuccess {
    /// Wraps one placement success, zero sequence included: the Go wire
    /// accepts a zero sequence on this packet.
    pub fn new(sequence: u64) -> Self {
        Self { sequence }
    }

    pub fn sequence(self) -> u64 {
        self.sequence
    }
}

/// One melee hit the authority confirmed to the attacking session.
///
/// The Go packet carries no target identity and no sequence: the publication
/// injects the enclosing tick's number and routes the hit to the attacker
/// only, after that session's own player state.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CombatHit {
    server_tick: u64,
    damage: u8,
    target: CombatTarget,
}

impl CombatHit {
    /// Wraps one hit, rejecting a zero server tick and a damage value outside
    /// `1..=MAX_HEALTH`, which are the exact Go `CombatHit.Validate` rules.
    ///
    /// The damage bound is the health maximum rather than a combat-specific
    /// constant, because that is the Go rule: a hit cannot confirm more damage
    /// than a body can hold.
    pub fn try_new(
        server_tick: u64,
        damage: u8,
        target: CombatTarget,
    ) -> Result<Self, DomainError> {
        if server_tick == 0 {
            return Err(DomainError::InvalidCombatHit);
        }
        if damage == 0 || damage > MAX_HEALTH {
            return Err(DomainError::InvalidCombatHit);
        }
        Ok(Self {
            server_tick,
            damage,
            target,
        })
    }

    pub fn server_tick(self) -> u64 {
        self.server_tick
    }

    pub fn damage(self) -> u8 {
        self.damage
    }

    pub fn target(self) -> CombatTarget {
        self.target
    }
}

/// The three published combat target kinds.
///
/// The kinds are a closed set with an explicit wire mapping, so a target this
/// crate publishes always names one of the three victims the Go registry
/// knows.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum CombatTarget {
    Player,
    Hostile,
    Passive,
}

impl CombatTarget {
    /// Wraps one raw wire kind, rejecting anything outside the three
    /// published values.
    pub fn try_new(id: u8) -> Result<Self, DomainError> {
        match id {
            1 => Ok(Self::Player),
            2 => Ok(Self::Hostile),
            3 => Ok(Self::Passive),
            _ => Err(DomainError::InvalidCombatTarget),
        }
    }

    /// Returns the wire value this kind publishes.
    ///
    /// The value comes from this explicit match rather than from a
    /// discriminant cast, so reordering the variants cannot change what the
    /// wire carries.
    pub fn wire_id(self) -> u8 {
        match self {
            Self::Player => 1,
            Self::Hostile => 2,
            Self::Passive => 3,
        }
    }
}

/// The fifteen published command rejection reasons.
///
/// The variants are named exactly as the Go `protocol.RejectReason` strings.
/// The internal Go enum runs `0..14` while the wire enum runs `1..15`, so the
/// wire value is never the discriminant.
#[derive(Clone, Copy, Debug, Eq, Hash, PartialEq)]
pub enum RejectReason {
    InvalidRay,
    NoTarget,
    ChunkNotReady,
    ProtectedBlock,
    InvalidBlock,
    Occupied,
    InvalidInput,
    PlayerNotReady,
    InvalidSlot,
    HotbarFull,
    DropCapacity,
    ContainerCapacity,
    NotFluidSource,
    BucketMismatch,
    NotArmor,
}

impl RejectReason {
    /// Returns the frozen wire ID one rejection reason publishes.
    ///
    /// The value comes from this explicit match rather than from a
    /// discriminant cast, so reordering or inserting a variant cannot change
    /// what the wire carries. The mapping is the Go
    /// `CommandRejectReasonID` table, in which every reason is one above its
    /// internal number.
    pub fn wire_id(self) -> u8 {
        match self {
            Self::InvalidRay => 1,
            Self::NoTarget => 2,
            Self::ChunkNotReady => 3,
            Self::ProtectedBlock => 4,
            Self::InvalidBlock => 5,
            Self::Occupied => 6,
            Self::InvalidInput => 7,
            Self::PlayerNotReady => 8,
            Self::InvalidSlot => 9,
            Self::HotbarFull => 10,
            Self::DropCapacity => 11,
            Self::ContainerCapacity => 12,
            Self::NotFluidSource => 13,
            Self::BucketMismatch => 14,
            Self::NotArmor => 15,
        }
    }
}
