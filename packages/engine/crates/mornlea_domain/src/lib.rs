//! Shared identifiers, value rules, and semantic input/event records.
//!
//! Production code in this crate must not depend on protocol codecs, save
//! codecs, the numerical kernel, a graphical host, or an online authority.
//! Incomplete identity, unknown family IDs, out-of-range values, and
//! non-finite rotations fail before a record is published.

#![deny(unsafe_code)]

mod event;
mod identity;
mod input;
mod items;
mod locations;
mod sections;
mod text;
mod values;

pub use event::{
    ActiveMining, ActiveMiningParts, BlockChange, BlockChanges, BlockChangesParts, ChestState,
    ChestStateParts, ChunkSnapshot, ChunkSnapshotParts, CombatHit, CombatTarget, CommandRejection,
    ContainerClosed, CraftingSize, CraftingState, CraftingStateParts, FAMILY_EVENT, FAMILY_INPUT,
    ForgetChunks, ForgetChunksParts, FurnaceState, FurnaceStateParts, InventoryState,
    InventoryStateParts, MiningState, MiningStateParts, MotionState, MotionStateParts, Observation,
    PlacementSuccess, PlayerState, PlayerStateParts, RejectReason, Season, SurvivalState,
    SurvivalStateParts, Weather, WorldState, WorldStateParts, order_observations,
};
pub use identity::{
    CompanionId, DomainError, HostileId, Identities, PassiveId, PlayerId, ProjectileId,
    ReplayIdentity,
};
pub use input::{
    ChatIntent, Command, CommandEnvelope, CommandEnvelopeParts, CommandOrderScratch, ContainerMove,
    CraftingMove, HeldActions, InventoryMove, Movement, PartialMove, PlacementIntent,
    PlayerControl, PlayerControlParts, ResyncIntent, StackSource, StackView, order_commands,
};
pub use items::{
    ItemStack, durability_max, is_smelting_product, item_stack_limit, smelting_output,
};
pub use locations::{BlockPos, ChunkPos, ContainerKind, ContainerRef, DropId};
pub use sections::{PalettedSection, chunk_block_index, registered_block};
pub use text::{CommandText, CompanionName, DisplayName, SpeechText};
pub use values::{Dimension, FiniteVec3, HotbarSlot, LookAngles};

/// Workspace crate identity consumed by the foundation registration tests.
pub const CRATE_NAME: &str = "mornlea_domain";
