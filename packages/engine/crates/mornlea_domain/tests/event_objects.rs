//! Projectile and item-drop observation contracts for `mornlea_domain`.
//!
//! The Go baseline is the `protocol.ProjectileSpawn`, `ProjectileState`,
//! `ProjectileDespawn`, `ItemDropUpserts` and `ItemDropRemoves` records in
//! `packages/shared/network/protocol`, which are the object publications an
//! authoritative session sends about the projectiles in flight and the
//! dropped item stacks one subscriber can see. Every value rule below is the
//! Go validator's rule for the same record, so a record this crate admits is
//! a record the protocol layer admits and the other way around.
//!
//! The packet record maxima are evidence of the wire budget, not domain
//! rules: one packet carries at most 128 projectile records or 32 drops, but
//! the domain bounds semantic work with the shared `MAX_SEMANTIC_BATCH_RECORDS`
//! cap and then requires only a nonempty batch whose identities are strictly
//! increasing. The raw kind byte belongs to the evidence and protocol
//! adapters; the domain stores the closed `ProjectileKind` enum.
//!
//! A projectile state record carries no kind, dimension or velocity because
//! all three are fixed for the projectile's whole life: the mirror records
//! them at spawn and the state batch only moves the body. An item drop
//! carries no world position and no authority lifecycle state: the block
//! index is the chunk-local cell the stack sits in, and pickup or despawn
//! timing stays authority-side simulation state the publication never puts
//! on the wire.

use mornlea_domain::{
    ChunkPos, Dimension, DomainError, DropId, FiniteVec3, ItemDrop, ItemDropParts, ItemDropRemoves,
    ItemDropRemovesParts, ItemDropUpserts, ItemDropUpsertsParts, ItemStack, ProjectileDespawn,
    ProjectileDespawnParts, ProjectileId, ProjectileKind, ProjectileSpawn, ProjectileSpawnParts,
    ProjectileSpawnRecord, ProjectileSpawnRecordParts, ProjectileState, ProjectileStateParts,
    ProjectileStateRecord, ProjectileStateRecordParts,
};

/// The seed projectile spawn: identity 11, the shard kind, the overworld, a
/// finite pose and a finite velocity.
///
/// The literal lists every field of the parts struct, so a field a later node
/// adds to the record fails to compile here rather than silently widening the
/// publication.
fn projectile_spawn_seed() -> ProjectileSpawnRecordParts {
    ProjectileSpawnRecordParts {
        id: ProjectileId::try_new(11).expect("the identity is nonzero"),
        kind: ProjectileKind::Shard,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([4.5, 66.0, -2.25]).expect("the pose is finite"),
        velocity: FiniteVec3::try_new([0.25, 0.0, -0.5]).expect("the velocity is finite"),
    }
}

/// The seed projectile state record: identity 11 and a finite pose.
fn projectile_state_seed() -> ProjectileStateRecordParts {
    ProjectileStateRecordParts {
        id: ProjectileId::try_new(11).expect("the identity is nonzero"),
        position: FiniteVec3::try_new([4.5, 66.0, -2.25]).expect("the pose is finite"),
    }
}

/// Strictly increasing projectile spawn records with the identities 1..=count.
fn projectile_spawn_records(count: u64) -> Vec<ProjectileSpawnRecord> {
    (1..=count)
        .map(|n| {
            let mut parts = projectile_spawn_seed();
            parts.id = ProjectileId::try_new(n).expect("each identity is nonzero");
            ProjectileSpawnRecord::new(parts)
        })
        .collect()
}

/// Strictly increasing projectile state records with the identities 1..=count.
fn projectile_state_records(count: u64) -> Vec<ProjectileStateRecord> {
    (1..=count)
        .map(|n| {
            let mut parts = projectile_state_seed();
            parts.id = ProjectileId::try_new(n).expect("each identity is nonzero");
            ProjectileStateRecord::new(parts)
        })
        .collect()
}

/// The strictly increasing projectile identities 1..=count.
fn projectile_ids(count: u64) -> Vec<ProjectileId> {
    (1..=count)
        .map(|n| ProjectileId::try_new(n).expect("each identity is nonzero"))
        .collect()
}

/// One checked drop identity for a strictly increasing sequence position.
///
/// Positions below 32 fill the slots of chunk (0, 0); position 32 and above
/// continue in chunk (1, 0), so the helper derives thirty-three distinct
/// increasing identities without leaving the per-chunk slot budget.
fn drop_id_at(position: u32) -> DropId {
    let (chunk, slot) = if position < 32 {
        (ChunkPos::new(0, 0), position as u8)
    } else {
        (ChunkPos::new(1, 0), (position - 32) as u8)
    };
    DropId::try_new(0, chunk, slot, 1).expect("the slot is in range and the generation is nonzero")
}

/// One valid item drop at a strictly increasing sequence position, with the
/// block index kept inside the chunk for every position the helpers use.
fn item_drop_at(position: u32) -> ItemDrop {
    ItemDrop::try_new(ItemDropParts {
        id: drop_id_at(position),
        block_index: position * 3_000,
        stack: ItemStack::try_new(1, 3, 0).expect("the stack is an ordinary valid stack"),
    })
    .expect("the block index is inside the chunk")
}

#[test]
fn projectile_spawn_keeps_all_four_kind_dimension_combinations() {
    // A player's bow works in either playable dimension while the hostile
    // shard is currently an overworld phenomenon, but that policy belongs to
    // the authority: the wire and this domain publish all four combinations,
    // and no kind-by-dimension match is rejected here.
    for kind in [ProjectileKind::Shard, ProjectileKind::Arrow] {
        for dimension in [Dimension::OVERWORLD, Dimension::DEPTHS] {
            let mut parts = projectile_spawn_seed();
            parts.kind = kind;
            parts.dimension = dimension;

            let record = ProjectileSpawnRecord::new(parts);

            assert_eq!(record.kind(), kind);
            assert_eq!(record.dimension(), dimension);
        }
    }
}

#[test]
fn projectile_spawn_keeps_position_and_velocity() {
    // The spawn record is the complete flight state at the tick the
    // projectile entered the subscribed chunks, and it carries no yaw or
    // health: a projectile is a point-like transient whose orientation the
    // client derives from its velocity.
    let record = ProjectileSpawnRecord::new(projectile_spawn_seed());

    assert_eq!(record.id().get(), 11);
    assert_eq!(record.kind(), ProjectileKind::Shard);
    assert_eq!(record.dimension(), Dimension::OVERWORLD);
    assert_eq!(record.position().get(), [4.5, 66.0, -2.25]);
    assert_eq!(record.velocity().get(), [0.25, 0.0, -0.5]);
}

#[test]
fn projectile_state_keeps_only_identity_and_position() {
    // The parts literal names only the identity and the position, so a kind,
    // dimension or velocity a later node adds fails to compile here: all
    // three are fixed for the projectile's whole life and the state batch
    // only moves the body.
    let record = ProjectileStateRecord::new(projectile_state_seed());

    assert_eq!(record.id().get(), 11);
    assert_eq!(record.position().get(), [4.5, 66.0, -2.25]);
}

#[test]
fn projectile_despawn_keeps_only_tick_and_ids() {
    // The parts literal names only the tick and the identities, so a reason
    // or a position a later node adds fails to compile here: the projectile
    // despawn publishes identity removal only, exactly as the Go packet
    // does.
    let ids = [
        ProjectileId::try_new(4).expect("the identity is nonzero"),
        ProjectileId::try_new(9).expect("the identity is nonzero"),
    ];
    let despawns = ProjectileDespawn::try_new(ProjectileDespawnParts {
        server_tick: 21,
        ids: ids.into(),
    })
    .expect("two strictly increasing identities are admitted");

    assert_eq!(despawns.server_tick(), 21);
    assert_eq!(despawns.ids(), ids.as_slice());
}

#[test]
fn projectile_batches_reject_empty_reversed_and_duplicate_ids() {
    // All three projectile batch shapes own the same three relations: a
    // batch above the shared work cap is refused before any scan, an empty
    // batch names nothing to apply, and the identities have to be strictly
    // increasing so a reversed or duplicated pair cannot apply one record on
    // top of itself.
    assert_eq!(
        ProjectileSpawn::try_new(ProjectileSpawnParts {
            server_tick: 7,
            spawns: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );
    assert_eq!(
        ProjectileState::try_new(ProjectileStateParts {
            server_tick: 7,
            states: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );
    assert_eq!(
        ProjectileDespawn::try_new(ProjectileDespawnParts {
            server_tick: 7,
            ids: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );

    let reversed_spawns = [
        ProjectileSpawnRecord::new({
            let mut parts = projectile_spawn_seed();
            parts.id = ProjectileId::try_new(2).expect("the identity is nonzero");
            parts
        }),
        ProjectileSpawnRecord::new({
            let mut parts = projectile_spawn_seed();
            parts.id = ProjectileId::try_new(1).expect("the identity is nonzero");
            parts
        }),
    ];
    assert_eq!(
        ProjectileSpawn::try_new(ProjectileSpawnParts {
            server_tick: 7,
            spawns: reversed_spawns.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );

    let duplicate_states = [
        ProjectileStateRecord::new(projectile_state_seed()),
        ProjectileStateRecord::new(projectile_state_seed()),
    ];
    assert_eq!(
        ProjectileState::try_new(ProjectileStateParts {
            server_tick: 7,
            states: duplicate_states.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );

    let reversed_ids = [
        ProjectileId::try_new(2).expect("the identity is nonzero"),
        ProjectileId::try_new(1).expect("the identity is nonzero"),
    ];
    assert_eq!(
        ProjectileDespawn::try_new(ProjectileDespawnParts {
            server_tick: 7,
            ids: reversed_ids.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );
}

#[test]
fn projectile_batches_accept_one_hundred_twenty_nine_records_beyond_one_packet() {
    // One packet carries at most 128 projectile records, so the strictly
    // increasing 129-record form proves the packet maximum is a transport
    // budget rather than a domain rule; every batch keeps its first and last
    // record unchanged, with no copy or sort.
    let spawns = ProjectileSpawn::try_new(ProjectileSpawnParts {
        server_tick: 3,
        spawns: projectile_spawn_records(129).into(),
    })
    .expect("129 strictly increasing records are a publishable domain batch");
    assert_eq!(spawns.spawns().len(), 129);
    assert_eq!(spawns.spawns()[0], projectile_spawn_records(1)[0]);
    assert_eq!(spawns.spawns()[128], projectile_spawn_records(129)[128]);

    let states = ProjectileState::try_new(ProjectileStateParts {
        server_tick: 3,
        states: projectile_state_records(129).into(),
    })
    .expect("129 strictly increasing records are a publishable domain batch");
    assert_eq!(states.states().len(), 129);
    assert_eq!(states.states()[0], projectile_state_records(1)[0]);
    assert_eq!(states.states()[128], projectile_state_records(129)[128]);

    let despawns = ProjectileDespawn::try_new(ProjectileDespawnParts {
        server_tick: 3,
        ids: projectile_ids(129).into(),
    })
    .expect("129 strictly increasing identities are a publishable domain batch");
    assert_eq!(despawns.ids().len(), 129);
    assert_eq!(despawns.ids()[0].get(), 1);
    assert_eq!(despawns.ids()[128].get(), 129);
}

#[test]
fn projectile_records_require_checked_nonzero_identity_and_finite_vectors() {
    // Identity zero and a non-finite vector fail inside the checked parts
    // before a record can be assembled, so no projectile constructor ever
    // sees them: `ProjectileId` rejects the absent form and `FiniteVec3`
    // rejects a NaN or infinite component for both the pose and the
    // velocity.
    assert_eq!(ProjectileId::try_new(0), Err(DomainError::InvalidIdentity));

    assert_eq!(
        FiniteVec3::try_new([f32::NAN, 66.0, 0.0]),
        Err(DomainError::NonFiniteValue)
    );
    assert_eq!(
        FiniteVec3::try_new([0.0, f32::NEG_INFINITY, 0.0]),
        Err(DomainError::NonFiniteValue)
    );

    // The finite seed pose and velocity still construct, so the rejections
    // above are the finiteness rule rather than an over-tight gate.
    assert!(FiniteVec3::try_new([4.5, 66.0, -2.25]).is_ok());
    assert!(FiniteVec3::try_new([0.25, 0.0, -0.5]).is_ok());
}

#[test]
fn item_drop_keeps_raw_negative_dimension_and_stack_fields() {
    // The Go `core.DropID.Valid` rule checks only the slot range and the
    // generation, so a drop naming an unusual dimension is still a
    // publishable identity and the domain keeps the raw -1 instead of
    // normalizing it.
    let id = DropId::try_new(-1, ChunkPos::new(3, -4), 5, 2).expect("the slot and generation hold");
    let drop = ItemDrop::try_new(ItemDropParts {
        id,
        block_index: 4_000,
        stack: ItemStack::try_new(1, 3, 0).expect("the stack is an ordinary valid stack"),
    })
    .expect("the raw dimension is publishable");

    assert_eq!(drop.id().dimension(), -1);
    assert_eq!(drop.id().chunk(), ChunkPos::new(3, -4));
    assert_eq!(drop.id().slot(), 5);
    assert_eq!(drop.id().generation(), 2);
    assert_eq!(drop.block_index(), 4_000);
    assert_eq!(drop.stack().item(), 1);
    assert_eq!(drop.stack().count(), 3);
    assert_eq!(drop.stack().durability(), 0);
}

#[test]
fn item_drop_accepts_empty_stack_and_block_index_98303() {
    // The empty stack is the value the authority publishes for an empty
    // slot, and 98_303 is the last cell of the 24-section chunk, so both
    // boundaries of the legal range are publishable.
    let drop = ItemDrop::try_new(ItemDropParts {
        id: drop_id_at(0),
        block_index: 98_303,
        stack: ItemStack::EMPTY,
    })
    .expect("the last chunk cell with an empty stack is admitted");

    assert_eq!(drop.block_index(), 98_303);
    assert_eq!(drop.stack(), ItemStack::EMPTY);
}

#[test]
fn item_drop_rejects_block_index_98304_without_clamping() {
    // 98_304 is the first value past the chunk's 24 sections of 4096 cells,
    // which the Go `ItemDrop.validate` bound rejects outright: the index is
    // reported as `InvalidBlockIndex` rather than clamped to the last cell,
    // so a caller can never mistake a rejected drop for one inside the
    // chunk.
    let rejected = |block_index| {
        ItemDrop::try_new(ItemDropParts {
            id: drop_id_at(0),
            block_index,
            stack: ItemStack::EMPTY,
        })
    };

    assert_eq!(rejected(98_304), Err(DomainError::InvalidBlockIndex));
    assert_eq!(rejected(u32::MAX), Err(DomainError::InvalidBlockIndex));
}

#[test]
fn item_drop_uses_existing_stack_registration_count_and_durability_rules() {
    // The drop record owns no second stack rule: the parts carry the shared
    // validated `ItemStack`, so registration, count and durability are the
    // inventory families' exact rules and a drop cannot publish a slot value
    // those families reject.
    assert_eq!(ItemStack::try_new(66, 1, 0), Err(DomainError::InvalidItem));
    assert_eq!(ItemStack::try_new(1, 0, 0), Err(DomainError::InvalidCount));
    assert_eq!(ItemStack::try_new(1, 65, 0), Err(DomainError::InvalidCount));
    assert_eq!(
        ItemStack::try_new(10, 1, 0),
        Err(DomainError::InvalidDurability)
    );
    assert_eq!(
        ItemStack::try_new(10, 1, 132),
        Err(DomainError::InvalidDurability)
    );

    // A durable stack at the top of its budget survives the drop constructor
    // unchanged, so the shared rule admits exactly what the drop keeps.
    let intact = ItemStack::try_new(10, 1, 131).expect("the durability is at the budget");
    let drop = ItemDrop::try_new(ItemDropParts {
        id: drop_id_at(0),
        block_index: 0,
        stack: intact,
    })
    .expect("a valid durable stack is admitted");

    assert_eq!(drop.stack(), intact);
}

#[test]
fn item_drop_upserts_reject_empty_reversed_and_duplicate_ids() {
    // The upserts batch owns the three shared batch relations, and the order
    // key is the derived `DropId` ordering — dimension, chunk column, slot,
    // generation — so a drop from chunk (1, 0) ahead of chunk (0, 0) is
    // reversed even though both slots sit inside their own chunks.
    assert_eq!(
        ItemDropUpserts::try_new(ItemDropUpsertsParts {
            server_tick: 7,
            drops: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );

    let reversed_drops = [item_drop_at(32), item_drop_at(31)];
    assert_eq!(
        ItemDropUpserts::try_new(ItemDropUpsertsParts {
            server_tick: 7,
            drops: reversed_drops.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );

    let duplicate_drops = [item_drop_at(0), item_drop_at(0)];
    assert_eq!(
        ItemDropUpserts::try_new(ItemDropUpsertsParts {
            server_tick: 7,
            drops: duplicate_drops.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );
}

#[test]
fn item_drop_removes_reject_empty_reversed_and_duplicate_ids() {
    // The removes batch owns the same three shared relations over bare
    // `DropId` values, with the same derived ordering as the upserts batch.
    assert_eq!(
        ItemDropRemoves::try_new(ItemDropRemovesParts {
            server_tick: 7,
            ids: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );

    let reversed_ids = [drop_id_at(32), drop_id_at(31)];
    assert_eq!(
        ItemDropRemoves::try_new(ItemDropRemovesParts {
            server_tick: 7,
            ids: reversed_ids.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );

    let duplicate_ids = [drop_id_at(0), drop_id_at(0)];
    assert_eq!(
        ItemDropRemoves::try_new(ItemDropRemovesParts {
            server_tick: 7,
            ids: duplicate_ids.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );
}

#[test]
fn item_drop_batches_accept_thirty_three_records_beyond_one_packet() {
    // One packet carries at most 32 drops, so the strictly increasing
    // 33-record form proves the packet maximum is a transport budget rather
    // than a domain rule; both batches keep their first and last record
    // unchanged, with no copy or sort.
    let drops = ItemDropUpserts::try_new(ItemDropUpsertsParts {
        server_tick: 3,
        drops: (0..33).map(item_drop_at).collect(),
    })
    .expect("33 strictly increasing drops are a publishable domain batch");
    assert_eq!(drops.drops().len(), 33);
    assert_eq!(drops.drops()[0], item_drop_at(0));
    assert_eq!(drops.drops()[32], item_drop_at(32));

    let removes = ItemDropRemoves::try_new(ItemDropRemovesParts {
        server_tick: 3,
        ids: (0..33).map(drop_id_at).collect(),
    })
    .expect("33 strictly increasing identities are a publishable domain batch");
    assert_eq!(removes.ids().len(), 33);
    assert_eq!(removes.ids()[0], drop_id_at(0));
    assert_eq!(removes.ids()[32], drop_id_at(32));
}

#[test]
fn object_batches_accept_zero_server_tick() {
    // The Go validators admit a zero tick on every object batch: the tick is
    // the publish instant, and a world that has not advanced yet still
    // publishes a spawn or a removal.
    let spawns = ProjectileSpawn::try_new(ProjectileSpawnParts {
        server_tick: 0,
        spawns: [ProjectileSpawnRecord::new(projectile_spawn_seed())].into(),
    })
    .expect("a one-record batch with a zero tick is admitted");
    assert_eq!(spawns.server_tick(), 0);

    let states = ProjectileState::try_new(ProjectileStateParts {
        server_tick: 0,
        states: [ProjectileStateRecord::new(projectile_state_seed())].into(),
    })
    .expect("a one-record batch with a zero tick is admitted");
    assert_eq!(states.server_tick(), 0);

    let despawns = ProjectileDespawn::try_new(ProjectileDespawnParts {
        server_tick: 0,
        ids: [ProjectileId::try_new(11).expect("the identity is nonzero")].into(),
    })
    .expect("a one-record batch with a zero tick is admitted");
    assert_eq!(despawns.server_tick(), 0);

    let upserts = ItemDropUpserts::try_new(ItemDropUpsertsParts {
        server_tick: 0,
        drops: [item_drop_at(0)].into(),
    })
    .expect("a one-record batch with a zero tick is admitted");
    assert_eq!(upserts.server_tick(), 0);

    let removes = ItemDropRemoves::try_new(ItemDropRemovesParts {
        server_tick: 0,
        ids: [drop_id_at(0)].into(),
    })
    .expect("a one-record batch with a zero tick is admitted");
    assert_eq!(removes.server_tick(), 0);
}

#[test]
fn object_seed_literals_expose_no_authority_only_fields() {
    // The state parts literal names only the identity and the position, so
    // a kind, dimension or velocity a later node adds fails to compile
    // here: all three are fixed for a projectile's whole life and the
    // mirror keeps them from the spawn. Reading back exercises exactly the
    // two frozen getters.
    let state = ProjectileStateRecord::new(projectile_state_seed());
    assert_eq!(state.id().get(), 11);
    assert_eq!(state.position().get(), [4.5, 66.0, -2.25]);

    // The drop parts literal names only the identity, the chunk-local block
    // index and the stack, so a world position or a pickup timer, despawn
    // countdown or velocity a later node adds fails to compile here: those
    // are authority-side lifecycle quantities the drop publication never
    // carries.
    let drop = item_drop_at(0);
    assert_eq!(drop.id(), drop_id_at(0));
    assert_eq!(drop.block_index(), 0);
    assert_eq!(
        drop.stack(),
        ItemStack::try_new(1, 3, 0).expect("the stack is valid")
    );
}
