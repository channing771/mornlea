//! Semantic command payloads and the envelope that orders them.
//!
//! The grouped payloads live in `control` (movement and ray), `inventory`
//! (slot-addressed moves) and `chat` (bounded text), and they carry no
//! sequence: intake metadata belongs to the ordering layer, so a payload cannot
//! pretend to know when it was admitted. `order` owns that layer, pairing a
//! payload with its tick, session, sequence and arrival index and sorting one
//! tick's commands by them.

mod chat;
mod control;
mod inventory;
mod order;

pub use chat::ChatIntent;
pub use control::{
    Command, HeldActions, Movement, PlacementIntent, PlayerControl, PlayerControlParts,
    ResyncIntent,
};
pub use inventory::{
    ContainerMove, CraftingMove, InventoryMove, PartialMove, StackSource, StackView,
};
pub use order::{CommandEnvelope, CommandEnvelopeParts, CommandOrderScratch, order_commands};
