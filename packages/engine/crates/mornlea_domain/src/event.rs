use crate::identity::DomainError;

mod outcome;
mod player;

pub use outcome::{CombatHit, CombatTarget, CommandRejection, PlacementSuccess, RejectReason};
pub use player::{
    ActiveMining, ActiveMiningParts, MiningState, MiningStateParts, MotionState, MotionStateParts,
    PlayerState, PlayerStateParts, Season, SurvivalState, SurvivalStateParts, Weather, WorldState,
    WorldStateParts,
};

/// Inventory family owned by this crate for semantic player inputs.
pub const FAMILY_INPUT: &str = "domain.input";
/// Inventory family owned by this crate for replay observations.
pub const FAMILY_EVENT: &str = "domain.event";

/// Normalized checkpoint observation for offline differential comparison.
///
/// Family IDs outside the domain inventory fail before the observation is
/// published. Digest emptiness is incomplete evidence, not a repairable gap.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Observation {
    pub tick: u64,
    family_id: String,
    digest: String,
}

impl Observation {
    pub fn new(
        tick: u64,
        family_id: impl Into<String>,
        digest: impl Into<String>,
    ) -> Result<Self, DomainError> {
        let family_id = family_id.into();
        if family_id != FAMILY_INPUT && family_id != FAMILY_EVENT {
            return Err(DomainError::UnknownId);
        }
        let digest = digest.into();
        if digest.trim().is_empty() {
            return Err(DomainError::IncompleteIdentity);
        }
        Ok(Self {
            tick,
            family_id,
            digest,
        })
    }

    pub fn family_id(&self) -> &str {
        &self.family_id
    }

    pub fn digest(&self) -> &str {
        &self.digest
    }
}

/// Orders observations by tick, then by family id, matching the replay
/// identity requirement for deterministic checkpoint comparison.
pub fn order_observations(rows: impl IntoIterator<Item = Observation>) -> Vec<Observation> {
    let mut ordered: Vec<_> = rows.into_iter().collect();
    ordered.sort_by(|left, right| {
        left.tick
            .cmp(&right.tick)
            .then(left.family_id.cmp(&right.family_id))
    });
    ordered
}
