//! The semantic publication surface and the routing envelope this crate
//! owns.
//!
//! The leaf modules beside this file own the checked payload values; this
//! module owns the two shapes above them: the closed [`Event`] set, which
//! names exactly the semantic publications an authority routes to sessions,
//! and the [`RoutedEvent`] envelope, which pairs one publication with its
//! explicit destination.
//!
//! The surface is deliberately exclusive. No variant carries a packet ID,
//! raw wire bytes, a digest, a handshake, login, rejection, keepalive or
//! disconnect fact, a chunk-worker acquire, generate, ready or resync
//! lifecycle message, or a generated-chunk pointer: those are transport and
//! runtime-owner concerns that stay outside the domain value set, and the
//! enum has no catch-all member that could smuggle one back in. Values that
//! already carry a publish tick retain it; the envelope adds none, because
//! routing is not a publication instant.

mod chat;
mod inventory;
mod mobs;
mod objects;
mod outcome;
mod people;
mod player;
mod world;

pub use chat::{ChatBody, ChatEvent, ChatEventParts, CompanionSpeaker, TaskFailure, TaskState};
pub use inventory::{
    ChestState, ChestStateParts, ContainerClosed, CraftingSize, CraftingState, CraftingStateParts,
    FurnaceState, FurnaceStateParts, InventoryState, InventoryStateParts,
};
pub use mobs::{
    HostileDespawn, HostileDespawnParts, HostileKind, HostileSpawn, HostileSpawnParts,
    HostileSpawnRecord, HostileSpawnRecordParts, HostileState, HostileStateParts,
    HostileStateRecord, HostileStateRecordParts, PassiveDespawn, PassiveDespawnParts,
    PassiveDespawnReason, PassiveDespawnRecord, PassiveSpawn, PassiveSpawnParts,
    PassiveSpawnRecord, PassiveSpawnRecordParts, PassiveState, PassiveStateParts,
    PassiveStateRecord, PassiveStateRecordParts,
};
pub use objects::{
    ItemDrop, ItemDropParts, ItemDropRemoves, ItemDropRemovesParts, ItemDropUpserts,
    ItemDropUpsertsParts, ProjectileDespawn, ProjectileDespawnParts, ProjectileKind,
    ProjectileSpawn, ProjectileSpawnParts, ProjectileSpawnRecord, ProjectileSpawnRecordParts,
    ProjectileState, ProjectileStateParts, ProjectileStateRecord, ProjectileStateRecordParts,
};
pub use outcome::{CombatHit, CombatTarget, CommandRejection, PlacementSuccess, RejectReason};
pub use people::{
    CompanionDespawn, CompanionSpawn, CompanionSpawnParts, CompanionState, CompanionStateParts,
    CompanionStates, CompanionStatesParts, RemotePlayerDespawn, RemotePlayerSpawn,
    RemotePlayerSpawnParts, RemotePlayerState, RemotePlayerStateParts, RemotePlayerStates,
    RemotePlayerStatesParts,
};
pub use player::{
    ActiveMining, ActiveMiningParts, MiningState, MiningStateParts, MotionState, MotionStateParts,
    PlayerState, PlayerStateParts, Season, SurvivalState, SurvivalStateParts, Weather, WorldState,
    WorldStateParts,
};
pub use world::{
    BlockChange, BlockChanges, BlockChangesParts, ChunkSnapshot, ChunkSnapshotParts, ForgetChunks,
    ForgetChunksParts,
};

/// The closed set of semantic publications an authority routes to sessions.
///
/// Every payload is an already-checked leaf value, so wrapping is total and
/// the enum adds no rule of its own. Consumers are expected to match
/// exhaustively: the set is closed, so an unplanned variant is a compile
/// failure at every consumer rather than a silently accepted shape.
#[derive(Clone, Debug, PartialEq)]
pub enum Event {
    ChunkSnapshot(ChunkSnapshot),
    BlockChanges(BlockChanges),
    ForgetChunks(ForgetChunks),
    PlayerState(PlayerState),
    CommandRejected(CommandRejection),
    RemotePlayerSpawn(RemotePlayerSpawn),
    RemotePlayerDespawn(RemotePlayerDespawn),
    RemotePlayerStates(RemotePlayerStates),
    InventoryState(InventoryState),
    ItemDropUpserts(ItemDropUpserts),
    ItemDropRemoves(ItemDropRemoves),
    FurnaceState(FurnaceState),
    ContainerClosed(ContainerClosed),
    ChestState(ChestState),
    Chat(ChatEvent),
    CompanionSpawn(CompanionSpawn),
    CompanionStates(CompanionStates),
    CompanionDespawn(CompanionDespawn),
    PlaceBlockSucceeded(PlacementSuccess),
    CraftingState(CraftingState),
    HostileSpawn(HostileSpawn),
    HostileState(HostileState),
    HostileDespawn(HostileDespawn),
    CombatHit(CombatHit),
    PassiveSpawn(PassiveSpawn),
    PassiveState(PassiveState),
    PassiveDespawn(PassiveDespawn),
    ProjectileSpawn(ProjectileSpawn),
    ProjectileState(ProjectileState),
    ProjectileDespawn(ProjectileDespawn),
}

/// The destination one publication addresses.
///
/// `Session` deliberately admits zero, because whether a session number
/// names a live session is a runtime concern this value does not own.
/// `Broadcast` is an explicit shape of its own and is never encoded as a
/// sentinel session number, so the two destinations cannot be confused.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum EventRecipient {
    Session(u64),
    Broadcast,
}

/// One publication paired with its explicit destination.
///
/// The envelope stores both parts unchanged and adds nothing of its own: no
/// tick, because payloads that carry a publish tick keep it and routing is
/// not a publication instant, and no priority or ordering, because those
/// belong to the runtime owner that drains the envelopes.
#[derive(Clone, Debug, PartialEq)]
pub struct RoutedEvent {
    recipient: EventRecipient,
    event: Event,
}

impl RoutedEvent {
    /// Stores the recipient and the event exactly as supplied.
    pub fn new(recipient: EventRecipient, event: Event) -> Self {
        Self { recipient, event }
    }

    pub fn recipient(&self) -> EventRecipient {
        self.recipient
    }

    pub fn event(&self) -> &Event {
        &self.event
    }
}
