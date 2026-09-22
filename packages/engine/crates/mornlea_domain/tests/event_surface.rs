//! Event-surface contracts for `mornlea_domain`.
//!
//! The public surface this file pins is the closed `Event` enum, the
//! two-form `EventRecipient` and the `RoutedEvent` envelope. The
//! construction test builds one legal value of every variant in the frozen
//! order and passes each through an exhaustive match without a wildcard, so
//! an unplanned future variant is a compile failure rather than a silently
//! accepted shape. The routing tests pin that the envelope stores the
//! recipient and the event unchanged, that session zero is a legal
//! recipient because session existence and broadcast policy are runtime
//! concerns, and that a broadcast is its own shape rather than a sentinel
//! session. The source test keeps the retired digest-era helper names out
//! of the two files that define the crate's public API.

use mornlea_domain::{
    BlockChanges, BlockChangesParts, BlockPos, ChatBody, ChatEvent, ChatEventParts, ChestState,
    ChestStateParts, ChunkPos, ChunkSnapshot, ChunkSnapshotParts, CombatHit, CombatTarget,
    CommandRejection, CommandText, CompanionDespawn, CompanionId, CompanionName, CompanionSpawn,
    CompanionSpawnParts, CompanionSpeaker, CompanionState, CompanionStateParts, CompanionStates,
    CompanionStatesParts, ContainerClosed, ContainerKind, ContainerRef, CraftingSize,
    CraftingState, CraftingStateParts, Dimension, DisplayName, DropId, Event, EventRecipient,
    FiniteVec3, ForgetChunks, ForgetChunksParts, FurnaceState, FurnaceStateParts, HostileDespawn,
    HostileDespawnParts, HostileId, HostileKind, HostileSpawn, HostileSpawnParts,
    HostileSpawnRecord, HostileSpawnRecordParts, HostileState, HostileStateParts,
    HostileStateRecord, HostileStateRecordParts, HotbarSlot, InventoryState, InventoryStateParts,
    ItemDrop, ItemDropParts, ItemDropRemoves, ItemDropRemovesParts, ItemDropUpserts,
    ItemDropUpsertsParts, ItemStack, LookAngles, MiningState, MiningStateParts, MotionState,
    MotionStateParts, PalettedSection, PassiveDespawn, PassiveDespawnParts, PassiveDespawnReason,
    PassiveDespawnRecord, PassiveId, PassiveSpawn, PassiveSpawnParts, PassiveSpawnRecord,
    PassiveSpawnRecordParts, PassiveState, PassiveStateParts, PassiveStateRecord,
    PassiveStateRecordParts, PlacementSuccess, PlayerId, PlayerState, PlayerStateParts,
    ProjectileDespawn, ProjectileDespawnParts, ProjectileId, ProjectileKind, ProjectileSpawn,
    ProjectileSpawnParts, ProjectileSpawnRecord, ProjectileSpawnRecordParts, ProjectileState,
    ProjectileStateParts, ProjectileStateRecord, ProjectileStateRecordParts, RejectReason,
    RemotePlayerDespawn, RemotePlayerSpawn, RemotePlayerSpawnParts, RemotePlayerState,
    RemotePlayerStateParts, RemotePlayerStates, RemotePlayerStatesParts, RoutedEvent, Season,
    SurvivalState, SurvivalStateParts, Weather, WorldState, WorldStateParts,
};

/// Sections one chunk column holds, from the Go `core.SectionsPerChunk`.
const CHUNK_SECTIONS: usize = 24;

/// The frozen variant-name list, in the order the enum declares them.
const EXPECTED_VARIANT_NAMES: [&str; 30] = [
    "ChunkSnapshot",
    "BlockChanges",
    "ForgetChunks",
    "PlayerState",
    "CommandRejected",
    "RemotePlayerSpawn",
    "RemotePlayerDespawn",
    "RemotePlayerStates",
    "InventoryState",
    "ItemDropUpserts",
    "ItemDropRemoves",
    "FurnaceState",
    "ContainerClosed",
    "ChestState",
    "Chat",
    "CompanionSpawn",
    "CompanionStates",
    "CompanionDespawn",
    "PlaceBlockSucceeded",
    "CraftingState",
    "HostileSpawn",
    "HostileState",
    "HostileDespawn",
    "CombatHit",
    "PassiveSpawn",
    "PassiveState",
    "PassiveDespawn",
    "ProjectileSpawn",
    "ProjectileState",
    "ProjectileDespawn",
];

/// The seed player identity: a valid UUIDv4.
fn player_id() -> PlayerId {
    PlayerId::try_from_bytes([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ])
    .expect("the identity is a valid UUIDv4")
}

/// The seed companion identity: a valid UUIDv4.
fn companion_id() -> CompanionId {
    CompanionId::try_from_bytes([
        0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x48, 0x89, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00,
        0xff,
    ])
    .expect("the identity is a valid UUIDv4")
}

/// One canonical player display name.
fn display_name() -> DisplayName {
    DisplayName::try_from_canonical("Alice".into()).expect("the name is canonical")
}

/// One canonical companion name.
fn companion_name() -> CompanionName {
    CompanionName::try_from_canonical("Buddy".into()).expect("the name is canonical")
}

/// One bounded chat command.
fn command_text() -> CommandText {
    CommandText::try_from_canonical("Buddy fetch the sword".into()).expect("the command is bounded")
}

/// One single-storage air section, the empty value of a column.
fn air_section() -> PalettedSection {
    PalettedSection::single(0).expect("air is the registered zero block")
}

/// The all-air column a snapshot carries.
fn air_column() -> Box<[PalettedSection; CHUNK_SECTIONS]> {
    vec![air_section(); CHUNK_SECTIONS]
        .try_into()
        .expect("exactly one section per chunk column index")
}

/// The seed snapshot: the overworld origin chunk at revision one.
fn chunk_snapshot() -> ChunkSnapshot {
    ChunkSnapshot::try_new(ChunkSnapshotParts {
        dimension: Dimension::OVERWORLD,
        chunk: ChunkPos::new(0, 0),
        revision: 1,
        sections: air_column(),
    })
    .expect("a nonzero revision with a full column is admitted")
}

/// The seed change batch: the empty revision barrier from revision one.
fn block_changes() -> BlockChanges {
    BlockChanges::try_new(BlockChangesParts {
        dimension: Dimension::OVERWORLD,
        chunk: ChunkPos::new(0, 0),
        base_revision: 1,
        new_revision: 2,
        changes: Vec::new().into_boxed_slice(),
    })
    .expect("an empty batch is a legal revision barrier")
}

/// The seed forget batch: one retired chunk.
fn forget_chunks() -> ForgetChunks {
    ForgetChunks::try_new(ForgetChunksParts {
        dimension: Dimension::OVERWORLD,
        chunks: vec![ChunkPos::new(0, 0)].into_boxed_slice(),
    })
    .expect("one distinct chunk is a legal batch")
}

/// The seed private player publication at tick nine.
fn player_state() -> PlayerState {
    PlayerState::new(PlayerStateParts {
        server_tick: 9,
        last_input_sequence: 0,
        dimension: Dimension::OVERWORLD,
        motion: MotionState::new(MotionStateParts {
            position: FiniteVec3::try_new([0.5, 64.0, 0.5]).expect("the pose is finite"),
            velocity: FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("the velocity is finite"),
            on_ground: true,
        }),
        look: LookAngles::try_new(0.0, 0.0).expect("the angles are finite"),
        ready: true,
        reset: false,
        mining: MiningState::try_new(MiningStateParts {
            active: false,
            target: BlockPos::ORIGIN,
            progress: 0,
            required: 0,
            harvestable: false,
        })
        .expect("the idle block is exactly empty"),
        survival: SurvivalState::try_new(SurvivalStateParts {
            health: 20,
            oxygen: 300,
            hunger: 20,
            saturation_zero: false,
            armor_points: 0,
        })
        .expect("the scalars are inside their maxima"),
        world: WorldState::try_new(WorldStateParts {
            day_phase_offset: 0,
            world_time_ticks: 9,
            weather: Weather::Clear,
            season: Season::Spring,
            season_progress: 0,
            temperature: 0,
        })
        .expect("the offset is inside one display day"),
    })
}

/// The seed command rejection: the zero sequence the wire accepts.
fn command_rejection() -> CommandRejection {
    CommandRejection::new(0, RejectReason::InvalidRay)
}

/// The seed placement confirmation: the zero sequence the wire accepts.
fn placement_success() -> PlacementSuccess {
    PlacementSuccess::new(0)
}

/// The seed remote-player spawn.
fn remote_player_spawn() -> RemotePlayerSpawn {
    RemotePlayerSpawn::new(RemotePlayerSpawnParts {
        player_id: player_id(),
        display_name: display_name(),
        server_tick: 9,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([1.5, 64.0, -3.25]).expect("the pose is finite"),
        look: LookAngles::try_new(0.5, -0.25).expect("the angles are finite"),
    })
}

/// The seed remote-player state batch: one record.
fn remote_player_states() -> RemotePlayerStates {
    RemotePlayerStates::try_new(RemotePlayerStatesParts {
        server_tick: 9,
        states: vec![RemotePlayerState::new(RemotePlayerStateParts {
            player_id: player_id(),
            dimension: Dimension::OVERWORLD,
            position: FiniteVec3::try_new([2.5, 65.0, -4.25]).expect("the pose is finite"),
            look: LookAngles::try_new(-1.0, 0.75).expect("the angles are finite"),
            reset: false,
        })]
        .into_boxed_slice(),
    })
    .expect("one checked record is a legal batch")
}

/// The seed inventory: everything empty, the first slot selected.
fn inventory_state() -> InventoryState {
    InventoryState::new(InventoryStateParts {
        selected: HotbarSlot::new(0).expect("slot zero is inside the range"),
        hotbar: [ItemStack::EMPTY; 9],
        backpack: [ItemStack::EMPTY; 27],
    })
}

/// The seed furnace reference.
fn furnace_ref() -> ContainerRef {
    ContainerRef::try_new(ChunkPos::new(0, 0), ContainerKind::Furnace, 0, 1)
        .expect("the slot and generation are inside the bounds")
}

/// The seed chest reference.
fn chest_ref() -> ContainerRef {
    ContainerRef::try_new(ChunkPos::new(0, 0), ContainerKind::Chest, 0, 1)
        .expect("the slot and generation are inside the bounds")
}

/// The seed furnace publication: idle timers and empty slots.
fn furnace_state() -> FurnaceState {
    FurnaceState::try_new(FurnaceStateParts {
        container: furnace_ref(),
        input: ItemStack::EMPTY,
        fuel: ItemStack::EMPTY,
        output: ItemStack::EMPTY,
        progress_ticks: 0,
        burn_ticks: 0,
    })
    .expect("idle timers and empty slots are admitted")
}

/// The seed chest publication: every slot empty.
fn chest_state() -> ChestState {
    ChestState::try_new(ChestStateParts {
        container: chest_ref(),
        items: [ItemStack::EMPTY; 27],
    })
    .expect("a chest reference with empty slots is admitted")
}

/// The seed crafting publication: an empty workbench grid.
fn crafting_state() -> CraftingState {
    CraftingState::try_new(CraftingStateParts {
        size: CraftingSize::Workbench,
        slots: [ItemStack::EMPTY; 9],
        output: ItemStack::EMPTY,
    })
    .expect("an empty workbench grid is admitted")
}

/// The seed chat event: one accepted command.
fn chat_event() -> ChatEvent {
    ChatEvent::try_new(ChatEventParts {
        event_id: 7,
        player_id: player_id(),
        player_name: display_name(),
        body: ChatBody::Accepted {
            companion: CompanionSpeaker::new(companion_id(), companion_name()),
            command: command_text(),
        },
    })
    .expect("a nonzero event with checked parts is admitted")
}

/// The seed companion spawn: the overworld, pitch inside the vertical range.
fn companion_spawn() -> CompanionSpawn {
    CompanionSpawn::try_new(CompanionSpawnParts {
        id: companion_id(),
        name: companion_name(),
        server_tick: 9,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([-8.5, 70.0, 12.25]).expect("the pose is finite"),
        look: LookAngles::try_new(3.0, -0.5).expect("the angles are finite and inside the range"),
    })
    .expect("the overworld pose is admitted")
}

/// The seed companion state batch: one record.
fn companion_states() -> CompanionStates {
    CompanionStates::try_new(CompanionStatesParts {
        server_tick: 9,
        states: vec![
            CompanionState::try_new(CompanionStateParts {
                id: companion_id(),
                dimension: Dimension::OVERWORLD,
                position: FiniteVec3::try_new([-7.5, 70.0, 11.25]).expect("the pose is finite"),
                look: LookAngles::try_new(-3.0, 0.5).expect("the angles are finite"),
                reset: false,
            })
            .expect("the overworld pose is admitted"),
        ]
        .into_boxed_slice(),
    })
    .expect("one checked record is a legal batch")
}

/// The seed item drop: the empty stack in the first cell of the origin slot.
fn item_drop() -> ItemDrop {
    ItemDrop::try_new(ItemDropParts {
        id: DropId::try_new(0, ChunkPos::new(0, 0), 0, 1)
            .expect("the slot and generation are inside the bounds"),
        block_index: 0,
        stack: ItemStack::EMPTY,
    })
    .expect("the index names the first cell of the chunk")
}

/// The seed drop upserts batch: one drop.
fn item_drop_upserts() -> ItemDropUpserts {
    ItemDropUpserts::try_new(ItemDropUpsertsParts {
        server_tick: 9,
        drops: vec![item_drop()].into_boxed_slice(),
    })
    .expect("one checked drop is a legal batch")
}

/// The seed drop removes batch: one identity.
fn item_drop_removes() -> ItemDropRemoves {
    ItemDropRemoves::try_new(ItemDropRemovesParts {
        server_tick: 9,
        ids: vec![item_drop().id()].into_boxed_slice(),
    })
    .expect("one checked identity is a legal batch")
}

/// The seed hostile spawn batch: one nightwalker at full health.
fn hostile_spawn() -> HostileSpawn {
    HostileSpawn::try_new(HostileSpawnParts {
        server_tick: 9,
        spawns: vec![
            HostileSpawnRecord::try_new(HostileSpawnRecordParts {
                id: HostileId::try_new(1).expect("the identity is nonzero"),
                dimension: Dimension::OVERWORLD,
                position: FiniteVec3::try_new([3.5, 64.0, 3.5]).expect("the pose is finite"),
                yaw: 0.0,
                health: 20,
                kind: HostileKind::Nightwalker,
            })
            .expect("the overworld pose is admitted"),
        ]
        .into_boxed_slice(),
    })
    .expect("one checked record is a legal batch")
}

/// The seed hostile state batch: one record.
fn hostile_state() -> HostileState {
    HostileState::try_new(HostileStateParts {
        server_tick: 9,
        states: vec![
            HostileStateRecord::try_new(HostileStateRecordParts {
                id: HostileId::try_new(1).expect("the identity is nonzero"),
                position: FiniteVec3::try_new([3.5, 64.0, 3.5]).expect("the pose is finite"),
                velocity: FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("the velocity is finite"),
                yaw: 0.0,
                health: 20,
                kind: HostileKind::Nightwalker,
            })
            .expect("the body is admitted"),
        ]
        .into_boxed_slice(),
    })
    .expect("one checked record is a legal batch")
}

/// The seed hostile despawn batch: one identity.
fn hostile_despawn() -> HostileDespawn {
    HostileDespawn::try_new(HostileDespawnParts {
        server_tick: 9,
        ids: vec![HostileId::try_new(1).expect("the identity is nonzero")].into_boxed_slice(),
    })
    .expect("one checked identity is a legal batch")
}

/// The seed combat hit: one damage against a player at tick nine.
fn combat_hit() -> CombatHit {
    CombatHit::try_new(9, 1, CombatTarget::Player).expect("the tick and damage are inside range")
}

/// The seed passive spawn batch: one mob at full health.
fn passive_spawn() -> PassiveSpawn {
    PassiveSpawn::try_new(PassiveSpawnParts {
        server_tick: 9,
        spawns: vec![
            PassiveSpawnRecord::try_new(PassiveSpawnRecordParts {
                id: PassiveId::try_new(1).expect("the identity is nonzero"),
                dimension: Dimension::OVERWORLD,
                position: FiniteVec3::try_new([4.5, 64.0, 4.5]).expect("the pose is finite"),
                yaw: 0.0,
                health: 20,
            })
            .expect("the overworld pose is admitted"),
        ]
        .into_boxed_slice(),
    })
    .expect("one checked record is a legal batch")
}

/// The seed passive state batch: one grazing mob.
fn passive_state() -> PassiveState {
    PassiveState::try_new(PassiveStateParts {
        server_tick: 9,
        states: vec![
            PassiveStateRecord::try_new(PassiveStateRecordParts {
                id: PassiveId::try_new(1).expect("the identity is nonzero"),
                position: FiniteVec3::try_new([4.5, 64.0, 4.5]).expect("the pose is finite"),
                velocity: FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("the velocity is finite"),
                yaw: 0.0,
                health: 20,
                grazing: true,
            })
            .expect("the body is admitted"),
        ]
        .into_boxed_slice(),
    })
    .expect("one checked record is a legal batch")
}

/// The seed passive despawn batch: one vanished mob.
fn passive_despawn() -> PassiveDespawn {
    PassiveDespawn::try_new(PassiveDespawnParts {
        server_tick: 9,
        despawns: vec![PassiveDespawnRecord::new(
            PassiveId::try_new(1).expect("the identity is nonzero"),
            PassiveDespawnReason::Vanished,
        )]
        .into_boxed_slice(),
    })
    .expect("one checked record is a legal batch")
}

/// The seed projectile spawn batch: one shard in the overworld.
fn projectile_spawn() -> ProjectileSpawn {
    ProjectileSpawn::try_new(ProjectileSpawnParts {
        server_tick: 9,
        spawns: vec![ProjectileSpawnRecord::new(ProjectileSpawnRecordParts {
            id: ProjectileId::try_new(1).expect("the identity is nonzero"),
            kind: ProjectileKind::Shard,
            dimension: Dimension::OVERWORLD,
            position: FiniteVec3::try_new([5.5, 66.0, 5.5]).expect("the pose is finite"),
            velocity: FiniteVec3::try_new([1.0, 0.0, 0.0]).expect("the velocity is finite"),
        })]
        .into_boxed_slice(),
    })
    .expect("one checked record is a legal batch")
}

/// The seed projectile state batch: one record.
fn projectile_state() -> ProjectileState {
    ProjectileState::try_new(ProjectileStateParts {
        server_tick: 9,
        states: vec![ProjectileStateRecord::new(ProjectileStateRecordParts {
            id: ProjectileId::try_new(1).expect("the identity is nonzero"),
            position: FiniteVec3::try_new([6.5, 66.0, 5.5]).expect("the pose is finite"),
        })]
        .into_boxed_slice(),
    })
    .expect("one checked record is a legal batch")
}

/// The seed projectile despawn batch: one identity.
fn projectile_despawn() -> ProjectileDespawn {
    ProjectileDespawn::try_new(ProjectileDespawnParts {
        server_tick: 9,
        ids: vec![ProjectileId::try_new(1).expect("the identity is nonzero")].into_boxed_slice(),
    })
    .expect("one checked identity is a legal batch")
}

/// One legal value of every variant, in the order the enum declares them.
///
/// Every payload comes from a seed helper that uses the existing public
/// constructors with minimal registered values, checked identities and text,
/// and one-record batches; no validation logic is duplicated here.
fn all_variants() -> Vec<Event> {
    vec![
        Event::ChunkSnapshot(chunk_snapshot()),
        Event::BlockChanges(block_changes()),
        Event::ForgetChunks(forget_chunks()),
        Event::PlayerState(player_state()),
        Event::CommandRejected(command_rejection()),
        Event::RemotePlayerSpawn(remote_player_spawn()),
        Event::RemotePlayerDespawn(RemotePlayerDespawn::new(player_id())),
        Event::RemotePlayerStates(remote_player_states()),
        Event::InventoryState(inventory_state()),
        Event::ItemDropUpserts(item_drop_upserts()),
        Event::ItemDropRemoves(item_drop_removes()),
        Event::FurnaceState(furnace_state()),
        Event::ContainerClosed(ContainerClosed::new(furnace_ref())),
        Event::ChestState(chest_state()),
        Event::Chat(chat_event()),
        Event::CompanionSpawn(companion_spawn()),
        Event::CompanionStates(companion_states()),
        Event::CompanionDespawn(CompanionDespawn::new(companion_id())),
        Event::PlaceBlockSucceeded(placement_success()),
        Event::CraftingState(crafting_state()),
        Event::HostileSpawn(hostile_spawn()),
        Event::HostileState(hostile_state()),
        Event::HostileDespawn(hostile_despawn()),
        Event::CombatHit(combat_hit()),
        Event::PassiveSpawn(passive_spawn()),
        Event::PassiveState(passive_state()),
        Event::PassiveDespawn(passive_despawn()),
        Event::ProjectileSpawn(projectile_spawn()),
        Event::ProjectileState(projectile_state()),
        Event::ProjectileDespawn(projectile_despawn()),
    ]
}

/// Names one event by its variant.
///
/// The match is exhaustive without a wildcard, so a variant added to the
/// enum later fails to compile here instead of silently widening the
/// surface.
fn variant_name(event: &Event) -> &'static str {
    match event {
        Event::ChunkSnapshot(_) => "ChunkSnapshot",
        Event::BlockChanges(_) => "BlockChanges",
        Event::ForgetChunks(_) => "ForgetChunks",
        Event::PlayerState(_) => "PlayerState",
        Event::CommandRejected(_) => "CommandRejected",
        Event::RemotePlayerSpawn(_) => "RemotePlayerSpawn",
        Event::RemotePlayerDespawn(_) => "RemotePlayerDespawn",
        Event::RemotePlayerStates(_) => "RemotePlayerStates",
        Event::InventoryState(_) => "InventoryState",
        Event::ItemDropUpserts(_) => "ItemDropUpserts",
        Event::ItemDropRemoves(_) => "ItemDropRemoves",
        Event::FurnaceState(_) => "FurnaceState",
        Event::ContainerClosed(_) => "ContainerClosed",
        Event::ChestState(_) => "ChestState",
        Event::Chat(_) => "Chat",
        Event::CompanionSpawn(_) => "CompanionSpawn",
        Event::CompanionStates(_) => "CompanionStates",
        Event::CompanionDespawn(_) => "CompanionDespawn",
        Event::PlaceBlockSucceeded(_) => "PlaceBlockSucceeded",
        Event::CraftingState(_) => "CraftingState",
        Event::HostileSpawn(_) => "HostileSpawn",
        Event::HostileState(_) => "HostileState",
        Event::HostileDespawn(_) => "HostileDespawn",
        Event::CombatHit(_) => "CombatHit",
        Event::PassiveSpawn(_) => "PassiveSpawn",
        Event::PassiveState(_) => "PassiveState",
        Event::PassiveDespawn(_) => "PassiveDespawn",
        Event::ProjectileSpawn(_) => "ProjectileSpawn",
        Event::ProjectileState(_) => "ProjectileState",
        Event::ProjectileDespawn(_) => "ProjectileDespawn",
    }
}

#[test]
fn domain_public_api_has_no_digest_observation_exports() {
    let event_source = include_str!("../src/event.rs");
    let lib_source = include_str!("../src/lib.rs");
    for removed in [
        "Observation",
        "order_observations",
        "FAMILY_EVENT",
        "FAMILY_INPUT",
    ] {
        assert!(
            !event_source.contains(removed),
            "`src/event.rs` must no longer name `{removed}`"
        );
        assert!(
            !lib_source.contains(removed),
            "`src/lib.rs` must no longer name `{removed}`"
        );
    }
}

#[test]
fn event_surface_constructs_exactly_30_semantic_variants() {
    let events = all_variants();
    assert_eq!(
        events.len(),
        EXPECTED_VARIANT_NAMES.len(),
        "the surface is exactly the thirty declared variants"
    );
    let names: Vec<&'static str> = events.iter().map(variant_name).collect();
    assert_eq!(
        names, EXPECTED_VARIANT_NAMES,
        "every variant constructs and names itself in the declared order"
    );
    let unique: std::collections::BTreeSet<&'static str> = names.iter().copied().collect();
    assert_eq!(unique.len(), 30, "every variant name is unique");
}

#[test]
fn routed_event_preserves_session_zero_and_event() {
    let event = Event::Chat(chat_event());
    let routed = RoutedEvent::new(EventRecipient::Session(0), event.clone());
    assert_eq!(
        routed.recipient(),
        EventRecipient::Session(0),
        "session zero is a legal recipient and is stored unchanged"
    );
    assert_eq!(routed.event(), &event);
}

#[test]
fn routed_event_preserves_broadcast_and_event() {
    let event = Event::PlayerState(player_state());
    let routed = RoutedEvent::new(EventRecipient::Broadcast, event.clone());
    assert_eq!(
        routed.recipient(),
        EventRecipient::Broadcast,
        "a broadcast is its own shape and is stored unchanged"
    );
    assert_eq!(routed.event(), &event);
}
