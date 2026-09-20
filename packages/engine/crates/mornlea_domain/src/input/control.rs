//! Grouped movement, held-action and ray command payloads.
//!
//! These records carry only what a client can legitimately claim about its own
//! intent: which way it wants to move, which buttons it is holding, and where
//! it is looking. No field names a world object, because the authoritative
//! server owns the target cell, the hit entity, the placed block, the consumed
//! item and the outcome. Intake metadata such as the sequence, tick or arrival
//! index is deliberately absent: it belongs to the ordering layer, so a
//! payload stays comparable across two runtimes that admit it differently.

use crate::identity::DomainError;
use crate::locations::ChunkPos;
use crate::values::{Dimension, HotbarSlot, LookAngles};

/// Locomotion request of one tick: two raw move axes and the jump bit.
///
/// The axes keep their full `i8` range. This crate validates only what the
/// wire already guarantees, so the −1..1 rule stays with the authority that
/// judges the movement; clamping here would silently rewrite a client claim
/// before the authority ever sees it, turning a movement the server should
/// reject into one it never learns about.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct Movement {
    pub move_x: i8,
    pub move_z: i8,
    pub jump: bool,
}

/// Buttons the client reports as held for one tick.
///
/// `primary` maps exactly to the Go `Mining` bit and carries no more meaning
/// than that: it does not imply that mining wins over combat, because which
/// action a held button resolves to is an authoritative decision over the
/// target and the held item. `eating`, `sprinting` and `sneaking` are the
/// remaining held bits in wire order. None of them names a target cell, a hit
/// entity, a placed block, a consumed item or an outcome.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct HeldActions {
    pub primary: bool,
    pub eating: bool,
    pub sprinting: bool,
    pub sneaking: bool,
}

/// Parts of one player-control payload.
///
/// The grouped plain controls are public fields because they carry no
/// invariant of their own; the one checked value, the rotation, is already a
/// `LookAngles` by the time the parts exist.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PlayerControlParts {
    pub movement: Movement,
    pub look: LookAngles,
    pub actions: HeldActions,
}

/// Semantic player locomotion and held-action record.
///
/// The field grouping replaces the former positional argument list so a
/// caller cannot transpose a move axis into a rotation. Construction is total
/// and named `new` rather than `try_new`: the only rule the record carries is
/// a finite rotation, and `LookAngles` enforces that before the parts can be
/// assembled, so there is nothing left for this constructor to reject.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PlayerControl {
    movement: Movement,
    look: LookAngles,
    actions: HeldActions,
}

impl PlayerControl {
    pub fn new(parts: PlayerControlParts) -> Self {
        Self {
            movement: parts.movement,
            look: parts.look,
            actions: parts.actions,
        }
    }

    pub fn movement(self) -> Movement {
        self.movement
    }

    pub fn look(self) -> LookAngles {
        self.look
    }

    pub fn actions(self) -> HeldActions {
        self.actions
    }
}

/// Place-block intent: where the client is looking and which hotbar slot it
/// means to place from.
///
/// The slot goes through the shared hotbar range, so the domain and the
/// protocol layer cannot disagree about which slots exist. The record names no
/// target cell and no placed block: the authority ray-casts the world and
/// decides whether the placement succeeds at all.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PlacementIntent {
    look: LookAngles,
    slot: HotbarSlot,
}

impl PlacementIntent {
    pub fn try_new(look: LookAngles, slot: u8) -> Result<Self, DomainError> {
        Ok(Self {
            look,
            slot: HotbarSlot::new(slot)?,
        })
    }

    pub fn look(self) -> LookAngles {
        self.look
    }

    pub fn slot(self) -> HotbarSlot {
        self.slot
    }
}

/// Chunk-resync intent: the dimension and chunk column the client wants
/// resent, plus the revision it already holds.
///
/// A zero revision is legal and means the client holds nothing for that chunk,
/// so it is accepted rather than rejected. The chunk contents stay
/// server-owned: this record asks for a resync and carries no snapshot.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ResyncIntent {
    dimension: Dimension,
    chunk: ChunkPos,
    have_revision: u64,
}

impl ResyncIntent {
    pub fn try_new(
        dimension: u8,
        chunk: ChunkPos,
        have_revision: u64,
    ) -> Result<Self, DomainError> {
        Ok(Self {
            dimension: Dimension::new(dimension)?,
            chunk,
            have_revision,
        })
    }

    pub fn dimension(self) -> Dimension {
        self.dimension
    }

    pub fn chunk(self) -> ChunkPos {
        self.chunk
    }

    pub fn have_revision(self) -> u64 {
        self.have_revision
    }
}

/// One client intent the authoritative runtime admits for ordering.
///
/// The variants are the movement and ray families. Each carries only the
/// grouped payloads above, and none carries a target cell, a hit entity, a
/// placed block, a consumed item or an outcome. The enum is deliberately open
/// for extension: later inventory, container and chat intents are added as new
/// variants beside these, so an existing match stays a compile error rather
/// than a silent fallthrough.
#[derive(Clone, Copy, Debug, PartialEq)]
pub enum Command {
    PlayerInput(PlayerControl),
    PlaceBlock(PlacementIntent),
    Resync(ResyncIntent),
    SelectHotbar(HotbarSlot),
    OpenContainer(LookAngles),
    TillSoil(LookAngles),
    BoneMeal(LookAngles),
    CollectWater(LookAngles),
    PlaceWater(LookAngles),
}
