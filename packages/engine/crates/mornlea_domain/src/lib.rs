//! Shared identifiers, value rules, and semantic input/event records.
//!
//! Production code in this crate must not depend on protocol codecs, save
//! codecs, the numerical kernel, a graphical host, or an online authority.
//! Incomplete identity, unknown family IDs, out-of-range values, non-finite
//! vectors and rotations, oversized text, and semantic batches above the
//! shared work cap fail before a record is published.

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
    ActiveMining, ActiveMiningParts, BlockChange, BlockChanges, BlockChangesParts, ChatBody,
    ChatEvent, ChatEventParts, ChestState, ChestStateParts, ChunkSnapshot, ChunkSnapshotParts,
    CombatHit, CombatTarget, CommandRejection, CompanionDespawn, CompanionSpawn,
    CompanionSpawnParts, CompanionSpeaker, CompanionState, CompanionStateParts, CompanionStates,
    CompanionStatesParts, ContainerClosed, CraftingSize, CraftingState, CraftingStateParts, Event,
    EventRecipient, ForgetChunks, ForgetChunksParts, FurnaceState, FurnaceStateParts,
    HostileDespawn, HostileDespawnParts, HostileKind, HostileSpawn, HostileSpawnParts,
    HostileSpawnRecord, HostileSpawnRecordParts, HostileState, HostileStateParts,
    HostileStateRecord, HostileStateRecordParts, InventoryState, InventoryStateParts, ItemDrop,
    ItemDropParts, ItemDropRemoves, ItemDropRemovesParts, ItemDropUpserts, ItemDropUpsertsParts,
    MiningState, MiningStateParts, MotionState, MotionStateParts, PassiveDespawn,
    PassiveDespawnParts, PassiveDespawnReason, PassiveDespawnRecord, PassiveSpawn,
    PassiveSpawnParts, PassiveSpawnRecord, PassiveSpawnRecordParts, PassiveState,
    PassiveStateParts, PassiveStateRecord, PassiveStateRecordParts, PlacementSuccess, PlayerState,
    PlayerStateParts, ProjectileDespawn, ProjectileDespawnParts, ProjectileKind, ProjectileSpawn,
    ProjectileSpawnParts, ProjectileSpawnRecord, ProjectileSpawnRecordParts, ProjectileState,
    ProjectileStateParts, ProjectileStateRecord, ProjectileStateRecordParts, RejectReason,
    RemotePlayerDespawn, RemotePlayerSpawn, RemotePlayerSpawnParts, RemotePlayerState,
    RemotePlayerStateParts, RemotePlayerStates, RemotePlayerStatesParts, RoutedEvent, Season,
    SurvivalState, SurvivalStateParts, TaskFailure, TaskState, Weather, WorldState,
    WorldStateParts,
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

/// Shared semantic work cap for the record batches this crate admits.
///
/// A batch constructor checks this cap at its first line and reports
/// `BatchTooLarge` before any per-record scan, copy, or sort, so an oversized
/// input costs a length compare instead of work proportional to its size.
/// The cap bounds semantic work per record batch; it is not a wire budget.
/// The protocol packet caps (the 4096-change and 4096-chunk frame ceilings
/// and the 7-record remote-player and 4-record companion batch maxima) stay
/// transport budgets in `mornlea_protocol` and are applied there in addition.
pub const MAX_SEMANTIC_BATCH_RECORDS: usize = 4096;

/// Workspace crate identity consumed by the foundation registration tests.
pub const CRATE_NAME: &str = "mornlea_domain";
