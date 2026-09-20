//! Semantic command payloads and the replay-ordering test facade.
//!
//! The grouped payloads live in `control` and carry no sequence: intake
//! metadata belongs to the ordering layer, so a payload cannot pretend to know
//! when it was admitted. `SemanticInput` below is a temporary compatibility
//! facade that pairs one payload with a sequence, which keeps the existing
//! replay-order cases exercising `order_inputs` until the ordering layer
//! replaces it. The facade performs no validation of its own.

mod control;

pub use control::{
    Command, HeldActions, Movement, PlacementIntent, PlayerControl, PlayerControlParts,
    ResyncIntent,
};

use crate::values::HotbarSlot;

/// Language-neutral input family used by replay ordering.
///
/// This is a compatibility facade rather than a production record: it wraps
/// the new payloads together with the sequence the old positional constructors
/// used to carry, so ordering stays testable without duplicating any rule the
/// payload constructors already enforce.
#[derive(Clone, Debug, PartialEq)]
pub enum SemanticInput {
    Player {
        sequence: u64,
        control: PlayerControl,
    },
    Place {
        sequence: u64,
        intent: PlacementIntent,
    },
    SelectHotbar {
        sequence: u64,
        slot: HotbarSlot,
    },
}

impl SemanticInput {
    pub fn sequence(&self) -> u64 {
        match self {
            Self::Player { sequence, .. } => *sequence,
            Self::Place { sequence, .. } => *sequence,
            Self::SelectHotbar { sequence, .. } => *sequence,
        }
    }

    pub fn kind(&self) -> &'static str {
        match self {
            Self::Player { .. } => "player_input",
            Self::Place { .. } => "place_block",
            Self::SelectHotbar { .. } => "select_hotbar",
        }
    }
}

/// Orders inputs by sequence, then by kind name, so two independent runtimes
/// emit the same checkpoint schedule from the same bag of records.
pub fn order_inputs(inputs: impl IntoIterator<Item = SemanticInput>) -> Vec<SemanticInput> {
    let mut ordered: Vec<_> = inputs.into_iter().collect();
    ordered.sort_by(|left, right| {
        left.sequence()
            .cmp(&right.sequence())
            .then(left.kind().cmp(right.kind()))
    });
    ordered
}
