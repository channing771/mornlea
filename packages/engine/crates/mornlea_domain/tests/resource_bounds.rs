//! Resource-bound precedence contracts for semantic batch construction.
//!
//! Every public batch constructor must reject an oversized input with
//! `BatchTooLarge` at its first line, before any per-record content relation
//! runs: the revision, span, membership and order scans, the duplicate
//! detection, and the sorted uniqueness scratch. The 4097-record inputs below
//! deliberately also violate a later content rule, so a `BatchTooLarge`
//! result proves the length gate precedes the content scan rather than merely
//! existing. The admitted 4096-record inputs use sorted or unique records and
//! must construct successfully, proving the cap is a work bound and not a
//! disguised protocol packet budget.

use mornlea_domain::{
    BlockChange, BlockChanges, BlockChangesParts, BlockPos, ChunkPos, CompanionId, CompanionState,
    CompanionStateParts, CompanionStates, CompanionStatesParts, Dimension, DomainError, DropId,
    FiniteVec3, ForgetChunks, ForgetChunksParts, HostileDespawn, HostileDespawnParts, HostileId,
    HostileKind, HostileSpawn, HostileSpawnParts, HostileSpawnRecord, HostileSpawnRecordParts,
    HostileState, HostileStateParts, HostileStateRecord, HostileStateRecordParts, ItemDrop,
    ItemDropParts, ItemDropRemoves, ItemDropRemovesParts, ItemDropUpserts, ItemDropUpsertsParts,
    ItemStack, LookAngles, MAX_SEMANTIC_BATCH_RECORDS, PassiveDespawn, PassiveDespawnParts,
    PassiveDespawnReason, PassiveDespawnRecord, PassiveId, PassiveSpawn, PassiveSpawnParts,
    PassiveSpawnRecord, PassiveSpawnRecordParts, PassiveState, PassiveStateParts,
    PassiveStateRecord, PassiveStateRecordParts, PlayerId, ProjectileDespawn,
    ProjectileDespawnParts, ProjectileId, ProjectileKind, ProjectileSpawn, ProjectileSpawnParts,
    ProjectileSpawnRecord, ProjectileSpawnRecordParts, ProjectileState, ProjectileStateParts,
    ProjectileStateRecord, ProjectileStateRecordParts, RemotePlayerState, RemotePlayerStateParts,
    RemotePlayerStates, RemotePlayerStatesParts,
};

/// Builds a distinct RFC-4122 v4 UUID from a counter, in wire byte order.
fn uuid_v4(counter: u128) -> [u8; 16] {
    let mut bytes = counter.to_be_bytes();
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    bytes
}

/// Builds `count` strictly increasing player identities.
fn ascending_player_ids(count: usize) -> Vec<PlayerId> {
    (1..=count as u128)
        .map(|counter| PlayerId::try_from_bytes(uuid_v4(counter)).expect("countered uuid v4"))
        .collect()
}

/// Builds one remote-player state record around `player_id`.
fn remote_state(player_id: PlayerId) -> RemotePlayerState {
    RemotePlayerState::new(RemotePlayerStateParts {
        player_id,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([1.0, 64.0, 2.0]).expect("finite seed position"),
        look: LookAngles::try_new(0.25, -0.5).expect("finite seed look"),
        reset: false,
    })
}

/// Builds one companion state record around `id`.
fn companion_state(id: CompanionId) -> CompanionState {
    CompanionState::try_new(CompanionStateParts {
        id,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([3.0, 65.0, 4.0]).expect("finite seed position"),
        look: LookAngles::try_new(0.5, 0.25).expect("finite seed look"),
        reset: false,
    })
    .expect("overworld companion state inside the vertical look range")
}

/// Builds `count` block changes inside chunk `(0, 0)` whose chunk-ordered
/// block indexes are strictly increasing from zero.
fn ascending_block_changes(count: usize) -> Vec<BlockChange> {
    (0..count)
        .map(|index| {
            let position = BlockPos::new(
                (index % 16) as i32,
                -64 + (index / 256) as i32,
                ((index / 16) % 16) as i32,
            );
            BlockChange::try_new(position, 1).expect("registered seed block")
        })
        .collect()
}

#[test]
fn block_changes_admit_4096_records_and_reject_4097_before_revision_rules() {
    let admitted = BlockChanges::try_new(BlockChangesParts {
        dimension: Dimension::OVERWORLD,
        chunk: ChunkPos::new(0, 0),
        base_revision: 7,
        new_revision: 8,
        changes: ascending_block_changes(MAX_SEMANTIC_BATCH_RECORDS).into_boxed_slice(),
    })
    .expect("a cap-sized batch is a legal semantic batch");
    assert_eq!(admitted.changes().len(), MAX_SEMANTIC_BATCH_RECORDS);

    // The revision pair (base zero) also violates the later revision rule, so
    // `BatchTooLarge` proves the length gate precedes the revision check.
    let oversized = BlockChangesParts {
        dimension: Dimension::OVERWORLD,
        chunk: ChunkPos::new(0, 0),
        base_revision: 0,
        new_revision: 1,
        changes: ascending_block_changes(MAX_SEMANTIC_BATCH_RECORDS + 1).into_boxed_slice(),
    };
    assert_eq!(
        BlockChanges::try_new(oversized),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn empty_block_changes_remain_a_valid_revision_barrier() {
    let barrier = BlockChanges::try_new(BlockChangesParts {
        dimension: Dimension::OVERWORLD,
        chunk: ChunkPos::new(-3, 5),
        base_revision: 1,
        new_revision: 2,
        changes: Vec::new().into_boxed_slice(),
    })
    .expect("the empty revision barrier stays admitted");
    assert!(barrier.changes().is_empty());
}

#[test]
fn forget_chunks_admit_4096_chunks_and_reject_4097_before_duplicate_scan() {
    let admitted: Vec<ChunkPos> = (0..MAX_SEMANTIC_BATCH_RECORDS as i32)
        .map(|x| ChunkPos::new(x, 0))
        .collect();
    let wrapped = ForgetChunks::try_new(ForgetChunksParts {
        dimension: Dimension::OVERWORLD,
        chunks: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized forget batch is a legal semantic batch");
    assert_eq!(wrapped.chunks().len(), MAX_SEMANTIC_BATCH_RECORDS);

    // Every chunk is the same column, so the later duplicate scan would also
    // reject; `BatchTooLarge` proves the length gate precedes that scan.
    let oversized: Vec<ChunkPos> = vec![ChunkPos::new(0, 0); MAX_SEMANTIC_BATCH_RECORDS + 1];
    assert_eq!(
        ForgetChunks::try_new(ForgetChunksParts {
            dimension: Dimension::OVERWORLD,
            chunks: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn remote_player_states_admit_4096_records_and_reject_4097_before_order_scan() {
    let ids = ascending_player_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let admitted: Vec<RemotePlayerState> = ids.iter().copied().map(remote_state).collect();
    let wrapped = RemotePlayerStates::try_new(RemotePlayerStatesParts {
        server_tick: 0,
        states: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized state batch is a legal semantic batch");
    assert_eq!(wrapped.states().len(), MAX_SEMANTIC_BATCH_RECORDS);

    // The oversized batch carries one record more than the cap with the first
    // two swapped, so the later order scan would also reject; `BatchTooLarge`
    // proves the length gate precedes the identity-order scan.
    let oversized_ids = ascending_player_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    let mut oversized: Vec<RemotePlayerState> =
        oversized_ids.iter().copied().map(remote_state).collect();
    oversized.swap(0, 1);
    assert_eq!(
        RemotePlayerStates::try_new(RemotePlayerStatesParts {
            server_tick: 0,
            states: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn companion_states_admit_4096_records_and_reject_4097_before_order_scan() {
    let ids: Vec<CompanionId> = (1..=MAX_SEMANTIC_BATCH_RECORDS as u128)
        .map(|counter| CompanionId::try_from_bytes(uuid_v4(counter)).expect("countered uuid v4"))
        .collect();
    let admitted: Vec<CompanionState> = ids.iter().copied().map(companion_state).collect();
    let wrapped = CompanionStates::try_new(CompanionStatesParts {
        server_tick: 0,
        states: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized companion batch is a legal semantic batch");
    assert_eq!(wrapped.states().len(), MAX_SEMANTIC_BATCH_RECORDS);

    let oversized_ids: Vec<CompanionId> = (1..=(MAX_SEMANTIC_BATCH_RECORDS + 1) as u128)
        .map(|counter| CompanionId::try_from_bytes(uuid_v4(counter)).expect("countered uuid v4"))
        .collect();
    let mut oversized: Vec<CompanionState> =
        oversized_ids.iter().copied().map(companion_state).collect();
    oversized.swap(0, 1);
    assert_eq!(
        CompanionStates::try_new(CompanionStatesParts {
            server_tick: 0,
            states: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

/// Builds `count` strictly increasing hostile identities from one.
fn ascending_hostile_ids(count: usize) -> Vec<HostileId> {
    (1..=count as u64)
        .map(|raw| HostileId::try_new(raw).expect("nonzero seed identity"))
        .collect()
}

/// Builds `count` strictly increasing passive identities from one.
fn ascending_passive_ids(count: usize) -> Vec<PassiveId> {
    (1..=count as u64)
        .map(|raw| PassiveId::try_new(raw).expect("nonzero seed identity"))
        .collect()
}

/// Builds `count` strictly increasing projectile identities from one.
fn ascending_projectile_ids(count: usize) -> Vec<ProjectileId> {
    (1..=count as u64)
        .map(|raw| ProjectileId::try_new(raw).expect("nonzero seed identity"))
        .collect()
}

/// Builds one hostile spawn record around `id`.
fn hostile_spawn_record(id: HostileId) -> HostileSpawnRecord {
    HostileSpawnRecord::try_new(HostileSpawnRecordParts {
        id,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([5.0, 66.0, 6.0]).expect("finite seed position"),
        yaw: 0.5,
        health: 20,
        kind: HostileKind::Nightwalker,
    })
    .expect("overworld hostile spawn inside the health range")
}

/// Builds one hostile state record around `id`.
fn hostile_state_record(id: HostileId) -> HostileStateRecord {
    HostileStateRecord::try_new(HostileStateRecordParts {
        id,
        position: FiniteVec3::try_new([5.0, 66.0, 6.0]).expect("finite seed position"),
        velocity: FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite seed velocity"),
        yaw: 0.5,
        health: 20,
        kind: HostileKind::Nightwalker,
    })
    .expect("hostile state inside the health range")
}

/// Builds one passive spawn record around `id`.
fn passive_spawn_record(id: PassiveId) -> PassiveSpawnRecord {
    PassiveSpawnRecord::try_new(PassiveSpawnRecordParts {
        id,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([7.0, 67.0, 8.0]).expect("finite seed position"),
        yaw: 0.25,
        health: 20,
    })
    .expect("overworld passive spawn inside the health range")
}

/// Builds one passive state record around `id`.
fn passive_state_record(id: PassiveId) -> PassiveStateRecord {
    PassiveStateRecord::try_new(PassiveStateRecordParts {
        id,
        position: FiniteVec3::try_new([7.0, 67.0, 8.0]).expect("finite seed position"),
        velocity: FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite seed velocity"),
        yaw: 0.25,
        health: 20,
        grazing: true,
    })
    .expect("passive state inside the health range")
}

/// Builds one passive despawn record around `id`.
fn passive_despawn_record(id: PassiveId) -> PassiveDespawnRecord {
    PassiveDespawnRecord::new(id, PassiveDespawnReason::Vanished)
}

/// Builds one projectile spawn record around `id`.
fn projectile_spawn_record(id: ProjectileId) -> ProjectileSpawnRecord {
    ProjectileSpawnRecord::new(ProjectileSpawnRecordParts {
        id,
        kind: ProjectileKind::Arrow,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([9.0, 68.0, 10.0]).expect("finite seed position"),
        velocity: FiniteVec3::try_new([1.0, 0.0, 0.0]).expect("finite seed velocity"),
    })
}

/// Builds one projectile state record around `id`.
fn projectile_state_record(id: ProjectileId) -> ProjectileStateRecord {
    ProjectileStateRecord::new(ProjectileStateRecordParts {
        id,
        position: FiniteVec3::try_new([9.0, 68.0, 10.0]).expect("finite seed position"),
    })
}

/// Builds one drop identity in the chunk column `x`: dimension zero, chunk Z
/// zero, slot zero and generation one, so an increasing `x` preserves the
/// derived `DropId` order while staying far inside `i32`.
fn drop_id(x: i32) -> DropId {
    DropId::try_new(0, ChunkPos::new(x, 0), 0, 1).expect("valid seed drop identity")
}

/// Builds one dropped-stack record around `id`.
fn item_drop(id: DropId) -> ItemDrop {
    ItemDrop::try_new(ItemDropParts {
        id,
        block_index: 0,
        stack: ItemStack::EMPTY,
    })
    .expect("drop with an empty stack and an in-chunk index")
}

#[test]
fn hostile_spawn_admits_4096_and_rejects_4097_before_order_scan() {
    let ids = ascending_hostile_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let admitted: Vec<HostileSpawnRecord> = ids.iter().copied().map(hostile_spawn_record).collect();
    let wrapped = HostileSpawn::try_new(HostileSpawnParts {
        server_tick: 0,
        spawns: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized hostile spawn batch is a legal semantic batch");
    assert_eq!(wrapped.spawns().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.spawns()[0].id().get(), 1);
    assert_eq!(
        wrapped.spawns()[MAX_SEMANTIC_BATCH_RECORDS - 1].id().get(),
        MAX_SEMANTIC_BATCH_RECORDS as u64
    );

    // The oversized batch also swaps the first two records, so the later
    // identity-order scan would reject too; `BatchTooLarge` proves the
    // length gate precedes that scan.
    let oversized_ids = ascending_hostile_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    let mut oversized: Vec<HostileSpawnRecord> = oversized_ids
        .iter()
        .copied()
        .map(hostile_spawn_record)
        .collect();
    oversized.swap(0, 1);
    assert_eq!(
        HostileSpawn::try_new(HostileSpawnParts {
            server_tick: 0,
            spawns: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn hostile_state_admits_4096_and_rejects_4097_before_order_scan() {
    let ids = ascending_hostile_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let admitted: Vec<HostileStateRecord> = ids.iter().copied().map(hostile_state_record).collect();
    let wrapped = HostileState::try_new(HostileStateParts {
        server_tick: 0,
        states: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized hostile state batch is a legal semantic batch");
    assert_eq!(wrapped.states().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.states()[0].id().get(), 1);
    assert_eq!(
        wrapped.states()[MAX_SEMANTIC_BATCH_RECORDS - 1].id().get(),
        MAX_SEMANTIC_BATCH_RECORDS as u64
    );

    // The oversized batch also swaps the first two records, so the later
    // identity-order scan would reject too; `BatchTooLarge` proves the
    // length gate precedes that scan.
    let oversized_ids = ascending_hostile_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    let mut oversized: Vec<HostileStateRecord> = oversized_ids
        .iter()
        .copied()
        .map(hostile_state_record)
        .collect();
    oversized.swap(0, 1);
    assert_eq!(
        HostileState::try_new(HostileStateParts {
            server_tick: 0,
            states: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn hostile_despawn_admits_4096_and_rejects_4097_before_order_scan() {
    let admitted = ascending_hostile_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let wrapped = HostileDespawn::try_new(HostileDespawnParts {
        server_tick: 0,
        ids: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized hostile despawn batch is a legal semantic batch");
    assert_eq!(wrapped.ids().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.ids()[0].get(), 1);
    assert_eq!(
        wrapped.ids()[MAX_SEMANTIC_BATCH_RECORDS - 1].get(),
        MAX_SEMANTIC_BATCH_RECORDS as u64
    );

    // The oversized batch also swaps the first two identities, so the later
    // order scan would reject too; `BatchTooLarge` proves the length gate
    // precedes that scan.
    let mut oversized = ascending_hostile_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    oversized.swap(0, 1);
    assert_eq!(
        HostileDespawn::try_new(HostileDespawnParts {
            server_tick: 0,
            ids: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn passive_spawn_admits_4096_and_rejects_4097_before_order_scan() {
    let ids = ascending_passive_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let admitted: Vec<PassiveSpawnRecord> = ids.iter().copied().map(passive_spawn_record).collect();
    let wrapped = PassiveSpawn::try_new(PassiveSpawnParts {
        server_tick: 0,
        spawns: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized passive spawn batch is a legal semantic batch");
    assert_eq!(wrapped.spawns().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.spawns()[0].id().get(), 1);
    assert_eq!(
        wrapped.spawns()[MAX_SEMANTIC_BATCH_RECORDS - 1].id().get(),
        MAX_SEMANTIC_BATCH_RECORDS as u64
    );

    // The oversized batch also swaps the first two records, so the later
    // identity-order scan would reject too; `BatchTooLarge` proves the
    // length gate precedes that scan.
    let oversized_ids = ascending_passive_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    let mut oversized: Vec<PassiveSpawnRecord> = oversized_ids
        .iter()
        .copied()
        .map(passive_spawn_record)
        .collect();
    oversized.swap(0, 1);
    assert_eq!(
        PassiveSpawn::try_new(PassiveSpawnParts {
            server_tick: 0,
            spawns: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn passive_state_admits_4096_and_rejects_4097_before_order_scan() {
    let ids = ascending_passive_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let admitted: Vec<PassiveStateRecord> = ids.iter().copied().map(passive_state_record).collect();
    let wrapped = PassiveState::try_new(PassiveStateParts {
        server_tick: 0,
        states: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized passive state batch is a legal semantic batch");
    assert_eq!(wrapped.states().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.states()[0].id().get(), 1);
    assert_eq!(
        wrapped.states()[MAX_SEMANTIC_BATCH_RECORDS - 1].id().get(),
        MAX_SEMANTIC_BATCH_RECORDS as u64
    );

    // The oversized batch also swaps the first two records, so the later
    // identity-order scan would reject too; `BatchTooLarge` proves the
    // length gate precedes that scan.
    let oversized_ids = ascending_passive_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    let mut oversized: Vec<PassiveStateRecord> = oversized_ids
        .iter()
        .copied()
        .map(passive_state_record)
        .collect();
    oversized.swap(0, 1);
    assert_eq!(
        PassiveState::try_new(PassiveStateParts {
            server_tick: 0,
            states: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn passive_despawn_admits_4096_and_rejects_4097_before_order_scan() {
    let ids = ascending_passive_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let admitted: Vec<PassiveDespawnRecord> =
        ids.iter().copied().map(passive_despawn_record).collect();
    let wrapped = PassiveDespawn::try_new(PassiveDespawnParts {
        server_tick: 0,
        despawns: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized passive despawn batch is a legal semantic batch");
    assert_eq!(wrapped.despawns().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.despawns()[0].id().get(), 1);
    assert_eq!(
        wrapped.despawns()[MAX_SEMANTIC_BATCH_RECORDS - 1]
            .id()
            .get(),
        MAX_SEMANTIC_BATCH_RECORDS as u64
    );

    // The oversized batch also swaps the first two records, so the later
    // identity-order scan would reject too; `BatchTooLarge` proves the
    // length gate precedes that scan.
    let oversized_ids = ascending_passive_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    let mut oversized: Vec<PassiveDespawnRecord> = oversized_ids
        .iter()
        .copied()
        .map(passive_despawn_record)
        .collect();
    oversized.swap(0, 1);
    assert_eq!(
        PassiveDespawn::try_new(PassiveDespawnParts {
            server_tick: 0,
            despawns: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn projectile_spawn_admits_4096_and_rejects_4097_before_order_scan() {
    let ids = ascending_projectile_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let admitted: Vec<ProjectileSpawnRecord> =
        ids.iter().copied().map(projectile_spawn_record).collect();
    let wrapped = ProjectileSpawn::try_new(ProjectileSpawnParts {
        server_tick: 0,
        spawns: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized projectile spawn batch is a legal semantic batch");
    assert_eq!(wrapped.spawns().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.spawns()[0].id().get(), 1);
    assert_eq!(
        wrapped.spawns()[MAX_SEMANTIC_BATCH_RECORDS - 1].id().get(),
        MAX_SEMANTIC_BATCH_RECORDS as u64
    );

    // The oversized batch also swaps the first two records, so the later
    // identity-order scan would reject too; `BatchTooLarge` proves the
    // length gate precedes that scan.
    let oversized_ids = ascending_projectile_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    let mut oversized: Vec<ProjectileSpawnRecord> = oversized_ids
        .iter()
        .copied()
        .map(projectile_spawn_record)
        .collect();
    oversized.swap(0, 1);
    assert_eq!(
        ProjectileSpawn::try_new(ProjectileSpawnParts {
            server_tick: 0,
            spawns: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn projectile_state_admits_4096_and_rejects_4097_before_order_scan() {
    let ids = ascending_projectile_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let admitted: Vec<ProjectileStateRecord> =
        ids.iter().copied().map(projectile_state_record).collect();
    let wrapped = ProjectileState::try_new(ProjectileStateParts {
        server_tick: 0,
        states: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized projectile state batch is a legal semantic batch");
    assert_eq!(wrapped.states().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.states()[0].id().get(), 1);
    assert_eq!(
        wrapped.states()[MAX_SEMANTIC_BATCH_RECORDS - 1].id().get(),
        MAX_SEMANTIC_BATCH_RECORDS as u64
    );

    // The oversized batch also swaps the first two records, so the later
    // identity-order scan would reject too; `BatchTooLarge` proves the
    // length gate precedes that scan.
    let oversized_ids = ascending_projectile_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    let mut oversized: Vec<ProjectileStateRecord> = oversized_ids
        .iter()
        .copied()
        .map(projectile_state_record)
        .collect();
    oversized.swap(0, 1);
    assert_eq!(
        ProjectileState::try_new(ProjectileStateParts {
            server_tick: 0,
            states: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn projectile_despawn_admits_4096_and_rejects_4097_before_order_scan() {
    let admitted = ascending_projectile_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let wrapped = ProjectileDespawn::try_new(ProjectileDespawnParts {
        server_tick: 0,
        ids: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized projectile despawn batch is a legal semantic batch");
    assert_eq!(wrapped.ids().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.ids()[0].get(), 1);
    assert_eq!(
        wrapped.ids()[MAX_SEMANTIC_BATCH_RECORDS - 1].get(),
        MAX_SEMANTIC_BATCH_RECORDS as u64
    );

    // The oversized batch also swaps the first two identities, so the later
    // order scan would reject too; `BatchTooLarge` proves the length gate
    // precedes that scan.
    let mut oversized = ascending_projectile_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    oversized.swap(0, 1);
    assert_eq!(
        ProjectileDespawn::try_new(ProjectileDespawnParts {
            server_tick: 0,
            ids: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn item_drop_upserts_admit_4096_and_reject_4097_before_order_scan() {
    let admitted: Vec<ItemDrop> = (1..=MAX_SEMANTIC_BATCH_RECORDS as i32)
        .map(drop_id)
        .map(item_drop)
        .collect();
    let wrapped = ItemDropUpserts::try_new(ItemDropUpsertsParts {
        server_tick: 0,
        drops: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized drop upserts batch is a legal semantic batch");
    assert_eq!(wrapped.drops().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.drops()[0].id(), drop_id(1));
    assert_eq!(
        wrapped.drops()[MAX_SEMANTIC_BATCH_RECORDS - 1].id(),
        drop_id(MAX_SEMANTIC_BATCH_RECORDS as i32)
    );

    // The oversized batch also swaps the first two drops, so the later
    // identity-order scan would reject too; `BatchTooLarge` proves the
    // length gate precedes that scan.
    let mut oversized: Vec<ItemDrop> = (1..=(MAX_SEMANTIC_BATCH_RECORDS + 1) as i32)
        .map(drop_id)
        .map(item_drop)
        .collect();
    oversized.swap(0, 1);
    assert_eq!(
        ItemDropUpserts::try_new(ItemDropUpsertsParts {
            server_tick: 0,
            drops: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn item_drop_removes_admit_4096_and_reject_4097_before_order_scan() {
    let admitted: Vec<DropId> = (1..=MAX_SEMANTIC_BATCH_RECORDS as i32)
        .map(drop_id)
        .collect();
    let wrapped = ItemDropRemoves::try_new(ItemDropRemovesParts {
        server_tick: 0,
        ids: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized drop removes batch is a legal semantic batch");
    assert_eq!(wrapped.ids().len(), MAX_SEMANTIC_BATCH_RECORDS);
    assert_eq!(wrapped.ids()[0], drop_id(1));
    assert_eq!(
        wrapped.ids()[MAX_SEMANTIC_BATCH_RECORDS - 1],
        drop_id(MAX_SEMANTIC_BATCH_RECORDS as i32)
    );

    // The oversized batch also swaps the first two identities, so the later
    // order scan would reject too; `BatchTooLarge` proves the length gate
    // precedes that scan.
    let mut oversized: Vec<DropId> = (1..=(MAX_SEMANTIC_BATCH_RECORDS + 1) as i32)
        .map(drop_id)
        .collect();
    oversized.swap(0, 1);
    assert_eq!(
        ItemDropRemoves::try_new(ItemDropRemovesParts {
            server_tick: 0,
            ids: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}
