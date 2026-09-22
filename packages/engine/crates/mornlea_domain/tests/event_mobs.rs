//! Hostile and passive mob observation contracts for `mornlea_domain`.
//!
//! The Go baseline is the `protocol.HostileSpawn`, `HostileState`,
//! `HostileDespawn`, `PassiveSpawn`, `PassiveState` and `PassiveDespawn`
//! records in `packages/shared/network/protocol`, which are the
//! visibility-derived observations an authoritative session publishes about
//! the mobs a subscriber can see. Every value rule below is the Go
//! validator's rule for the same record, so a record this crate admits is a
//! record the protocol layer admits and the other way around.
//!
//! The packet record maxima are evidence of the wire budget, not domain
//! rules: one packet carries at most 64 mob records of one kind, but the
//! domain bounds semantic work with the shared `MAX_SEMANTIC_BATCH_RECORDS`
//! cap and then requires only a nonempty batch whose identities are strictly
//! increasing. The raw kind, grazing and despawn-reason bytes belong to the
//! evidence and protocol adapters; the domain stores the closed
//! `HostileKind` and `PassiveDespawnReason` enums and a `bool`.
//!
//! No record carries cooldown, target, path, AI or capacity state. Those are
//! authority-side simulation quantities the mob publications never put on
//! the wire, and a record that grew one back would turn a visibility
//! observation into a second authority mirror.

use mornlea_domain::{
    Dimension, DomainError, FiniteVec3, HostileDespawn, HostileDespawnParts, HostileId,
    HostileKind, HostileSpawn, HostileSpawnParts, HostileSpawnRecord, HostileSpawnRecordParts,
    HostileState, HostileStateParts, HostileStateRecord, HostileStateRecordParts, PassiveDespawn,
    PassiveDespawnParts, PassiveDespawnReason, PassiveDespawnRecord, PassiveId, PassiveSpawn,
    PassiveSpawnParts, PassiveSpawnRecord, PassiveSpawnRecordParts, PassiveState,
    PassiveStateParts, PassiveStateRecord, PassiveStateRecordParts,
};

/// The seed hostile spawn: identity 9, the overworld, a finite pose, a finite
/// yaw, health 12 and the nightwalker kind.
///
/// The literal lists every field of the parts struct, so a field a later node
/// adds to the record fails to compile here rather than silently widening the
/// publication.
fn hostile_spawn_seed() -> HostileSpawnRecordParts {
    HostileSpawnRecordParts {
        id: HostileId::try_new(9).expect("the identity is nonzero"),
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([12.5, 64.0, -7.25]).expect("the pose is finite"),
        yaw: 0.75,
        health: 12,
        kind: HostileKind::Nightwalker,
    }
}

/// The seed hostile state record: identity 9, a finite pose and velocity, a
/// finite yaw, health 12 and the bone-thrower kind.
fn hostile_state_seed() -> HostileStateRecordParts {
    HostileStateRecordParts {
        id: HostileId::try_new(9).expect("the identity is nonzero"),
        position: FiniteVec3::try_new([12.5, 64.0, -7.25]).expect("the pose is finite"),
        velocity: FiniteVec3::try_new([0.5, 0.0, -0.25]).expect("the velocity is finite"),
        yaw: 0.75,
        health: 12,
        kind: HostileKind::BoneThrower,
    }
}

/// The seed passive spawn: identity 5, the overworld, a finite pose, a finite
/// yaw and health 10.
fn passive_spawn_seed() -> PassiveSpawnRecordParts {
    PassiveSpawnRecordParts {
        id: PassiveId::try_new(5).expect("the identity is nonzero"),
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([-3.5, 70.0, 8.25]).expect("the pose is finite"),
        yaw: -1.25,
        health: 10,
    }
}

/// The seed passive state record: identity 5, a finite pose and velocity, a
/// finite yaw, health 10 and a false grazing bit.
fn passive_state_seed() -> PassiveStateRecordParts {
    PassiveStateRecordParts {
        id: PassiveId::try_new(5).expect("the identity is nonzero"),
        position: FiniteVec3::try_new([-3.5, 70.0, 8.25]).expect("the pose is finite"),
        velocity: FiniteVec3::try_new([-0.5, 0.0, 0.75]).expect("the velocity is finite"),
        yaw: -1.25,
        health: 10,
        grazing: false,
    }
}

/// Sixty-five hostile spawn records with the identities 1..=65.
///
/// One packet carries at most 64 records, so the strictly increasing 65 form
/// is the packet-separation fact: a domain batch above the wire maximum is
/// still one publishable semantic batch.
fn hostile_spawn_records(count: u64) -> Vec<HostileSpawnRecord> {
    (1..=count)
        .map(|n| {
            let mut parts = hostile_spawn_seed();
            parts.id = HostileId::try_new(n).expect("each identity is nonzero");
            HostileSpawnRecord::try_new(parts).expect("each seed record is admitted")
        })
        .collect()
}

/// Sixty-five hostile state records with the identities 1..=65.
fn hostile_state_records(count: u64) -> Vec<HostileStateRecord> {
    (1..=count)
        .map(|n| {
            let mut parts = hostile_state_seed();
            parts.id = HostileId::try_new(n).expect("each identity is nonzero");
            HostileStateRecord::try_new(parts).expect("each seed record is admitted")
        })
        .collect()
}

/// The identities 1..=65 as checked hostile identities.
fn hostile_ids(count: u64) -> Vec<HostileId> {
    (1..=count)
        .map(|n| HostileId::try_new(n).expect("each identity is nonzero"))
        .collect()
}

/// Sixty-five passive spawn records with the identities 1..=65.
fn passive_spawn_records(count: u64) -> Vec<PassiveSpawnRecord> {
    (1..=count)
        .map(|n| {
            let mut parts = passive_spawn_seed();
            parts.id = PassiveId::try_new(n).expect("each identity is nonzero");
            PassiveSpawnRecord::try_new(parts).expect("each seed record is admitted")
        })
        .collect()
}

/// Sixty-five passive state records with the identities 1..=65.
fn passive_state_records(count: u64) -> Vec<PassiveStateRecord> {
    (1..=count)
        .map(|n| {
            let mut parts = passive_state_seed();
            parts.id = PassiveId::try_new(n).expect("each identity is nonzero");
            PassiveStateRecord::try_new(parts).expect("each seed record is admitted")
        })
        .collect()
}

/// Sixty-five passive despawn records with the identities 1..=65.
fn passive_despawn_records(count: u64) -> Vec<PassiveDespawnRecord> {
    (1..=count)
        .map(|n| {
            PassiveDespawnRecord::new(
                PassiveId::try_new(n).expect("each identity is nonzero"),
                PassiveDespawnReason::Vanished,
            )
        })
        .collect()
}

#[test]
fn hostile_spawn_seed_keeps_every_field_and_zero_tick() {
    let record =
        HostileSpawnRecord::try_new(hostile_spawn_seed()).expect("the seed record is admitted");

    assert_eq!(
        record.id(),
        HostileId::try_new(9).expect("the identity is nonzero")
    );
    assert_eq!(record.dimension(), Dimension::OVERWORLD);
    assert_eq!(record.position().get(), [12.5, 64.0, -7.25]);
    assert_eq!(record.yaw(), 0.75);
    assert_eq!(record.health(), 12);
    assert_eq!(record.kind(), HostileKind::Nightwalker);

    // The Go validator admits a zero tick on every mob batch: the tick is
    // the publish instant, and a world that has not advanced yet still
    // publishes a spawn.
    let batch = HostileSpawn::try_new(HostileSpawnParts {
        server_tick: 0,
        spawns: [record].into(),
    })
    .expect("a one-record batch with a zero tick is admitted");

    assert_eq!(batch.server_tick(), 0);
    assert_eq!(batch.spawns().len(), 1);
}

#[test]
fn hostile_spawn_rejects_depths_before_yaw_and_health() {
    // The record carries three violations at once: the depths dimension, a
    // non-finite yaw and health zero. The dimension rule fires first, which
    // is the Go validator's order, so the other two are never reached.
    let mut parts = hostile_spawn_seed();
    parts.dimension = Dimension::DEPTHS;
    parts.yaw = f32::NAN;
    parts.health = 0;

    let result = HostileSpawnRecord::try_new(parts);

    assert_eq!(result, Err(DomainError::InvalidDimension));
}

#[test]
fn hostile_spawn_rejects_non_finite_yaw_before_health() {
    // The dimension rule is satisfied, so the non-finite yaw reports first
    // and the out-of-range health is never reached: the finiteness rule
    // precedes the survival range in the Go validator's order.
    let mut parts = hostile_spawn_seed();
    parts.yaw = f32::NAN;
    parts.health = 0;

    let result = HostileSpawnRecord::try_new(parts);

    assert_eq!(result, Err(DomainError::NonFiniteRotation));
}

#[test]
fn hostile_spawn_health_accepts_one_and_twenty_and_rejects_zero_and_twenty_one() {
    // Health inside `1..=20` is publishable at both ends, while zero names a
    // mob the authority removes rather than publishes and twenty-one is
    // above the Go `core.MaxHealth` a mob can hold.
    for health in [1u8, 20] {
        let mut parts = hostile_spawn_seed();
        parts.health = health;

        let record = HostileSpawnRecord::try_new(parts).expect("the boundary health is admitted");

        assert_eq!(record.health(), health);
    }

    for health in [0u8, 21] {
        let mut parts = hostile_spawn_seed();
        parts.health = health;

        let result = HostileSpawnRecord::try_new(parts);

        assert_eq!(result, Err(DomainError::InvalidSurvivalValue));
    }
}

#[test]
fn hostile_state_keeps_position_velocity_kind_and_no_dimension() {
    // The parts literal names every field, so a dimension a later node adds
    // fails to compile here: a dimension change always goes through a
    // despawn/spawn pair, so the state record carries no dimension to keep.
    let record =
        HostileStateRecord::try_new(hostile_state_seed()).expect("the seed record is admitted");

    assert_eq!(
        record.id(),
        HostileId::try_new(9).expect("the identity is nonzero")
    );
    assert_eq!(record.position().get(), [12.5, 64.0, -7.25]);
    assert_eq!(record.velocity().get(), [0.5, 0.0, -0.25]);
    assert_eq!(record.yaw(), 0.75);
    assert_eq!(record.health(), 12);
    assert_eq!(record.kind(), HostileKind::BoneThrower);
}

#[test]
fn hostile_state_rejects_non_finite_yaw_before_health() {
    // The state record checks yaw then health, so the non-finite yaw reports
    // first and the out-of-range health is never reached.
    let mut parts = hostile_state_seed();
    parts.yaw = f32::NEG_INFINITY;
    parts.health = 0;

    let result = HostileStateRecord::try_new(parts);

    assert_eq!(result, Err(DomainError::NonFiniteRotation));
}

#[test]
fn hostile_batches_reject_empty_reversed_and_duplicate_ids() {
    // All three hostile batch shapes own the same three relations: a batch
    // above the shared work cap is refused before any scan, an empty batch
    // names nothing to apply, and the identities have to be strictly
    // increasing so a reversed or duplicated pair cannot apply one record on
    // top of itself.
    assert_eq!(
        HostileSpawn::try_new(HostileSpawnParts {
            server_tick: 7,
            spawns: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );
    assert_eq!(
        HostileState::try_new(HostileStateParts {
            server_tick: 7,
            states: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );
    assert_eq!(
        HostileDespawn::try_new(HostileDespawnParts {
            server_tick: 7,
            ids: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );

    let reversed_spawns = [
        HostileSpawnRecord::try_new({
            let mut parts = hostile_spawn_seed();
            parts.id = HostileId::try_new(2).expect("the identity is nonzero");
            parts
        })
        .expect("the seed record is admitted"),
        HostileSpawnRecord::try_new({
            let mut parts = hostile_spawn_seed();
            parts.id = HostileId::try_new(1).expect("the identity is nonzero");
            parts
        })
        .expect("the seed record is admitted"),
    ];
    assert_eq!(
        HostileSpawn::try_new(HostileSpawnParts {
            server_tick: 7,
            spawns: reversed_spawns.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );

    let duplicate_states = [
        HostileStateRecord::try_new(hostile_state_seed()).expect("the seed record is admitted"),
        HostileStateRecord::try_new(hostile_state_seed()).expect("the seed record is admitted"),
    ];
    assert_eq!(
        HostileState::try_new(HostileStateParts {
            server_tick: 7,
            states: duplicate_states.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );

    let reversed_ids = [
        HostileId::try_new(2).expect("the identity is nonzero"),
        HostileId::try_new(1).expect("the identity is nonzero"),
    ];
    assert_eq!(
        HostileDespawn::try_new(HostileDespawnParts {
            server_tick: 7,
            ids: reversed_ids.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );
}

#[test]
fn hostile_batches_accept_sixty_five_records_beyond_one_packet() {
    // One packet carries at most 64 records, so the strictly increasing
    // 65-record form proves the packet maximum is a transport budget rather
    // than a domain rule; every batch keeps its first and last record
    // unchanged, with no copy or sort.
    let expected_first_spawn = hostile_spawn_records(1)[0];
    let expected_last_spawn = hostile_spawn_records(65)[64];
    let spawns = HostileSpawn::try_new(HostileSpawnParts {
        server_tick: 3,
        spawns: hostile_spawn_records(65).into(),
    })
    .expect("sixty-five strictly increasing records are a publishable domain batch");
    assert_eq!(spawns.spawns().len(), 65);
    assert_eq!(spawns.spawns()[0], expected_first_spawn);
    assert_eq!(spawns.spawns()[64], expected_last_spawn);

    let expected_first_state = hostile_state_records(1)[0];
    let expected_last_state = hostile_state_records(65)[64];
    let states = HostileState::try_new(HostileStateParts {
        server_tick: 3,
        states: hostile_state_records(65).into(),
    })
    .expect("sixty-five strictly increasing records are a publishable domain batch");
    assert_eq!(states.states().len(), 65);
    assert_eq!(states.states()[0], expected_first_state);
    assert_eq!(states.states()[64], expected_last_state);

    let despawns = HostileDespawn::try_new(HostileDespawnParts {
        server_tick: 3,
        ids: hostile_ids(65).into(),
    })
    .expect("sixty-five strictly increasing identities are a publishable domain batch");
    assert_eq!(despawns.ids().len(), 65);
    assert_eq!(despawns.ids()[0].get(), 1);
    assert_eq!(despawns.ids()[64].get(), 65);
}

#[test]
fn hostile_despawn_keeps_only_tick_and_ids() {
    // The parts literal names only the tick and the identities, so a reason
    // a later node adds fails to compile here: the Go hostile despawn
    // publishes identity removal only, unlike the passive despawn's closed
    // vanished/died pair.
    let ids = [
        HostileId::try_new(4).expect("the identity is nonzero"),
        HostileId::try_new(9).expect("the identity is nonzero"),
    ];
    let despawns = HostileDespawn::try_new(HostileDespawnParts {
        server_tick: 21,
        ids: ids.into(),
    })
    .expect("two strictly increasing identities are admitted");

    assert_eq!(despawns.server_tick(), 21);
    assert_eq!(despawns.ids(), ids.as_slice());
}

#[test]
fn passive_spawn_seed_keeps_every_field_and_zero_tick() {
    let record =
        PassiveSpawnRecord::try_new(passive_spawn_seed()).expect("the seed record is admitted");

    assert_eq!(
        record.id(),
        PassiveId::try_new(5).expect("the identity is nonzero")
    );
    assert_eq!(record.dimension(), Dimension::OVERWORLD);
    assert_eq!(record.position().get(), [-3.5, 70.0, 8.25]);
    assert_eq!(record.yaw(), -1.25);
    assert_eq!(record.health(), 10);

    // The Go validator admits a zero tick on every mob batch, exactly as on
    // the hostile one.
    let batch = PassiveSpawn::try_new(PassiveSpawnParts {
        server_tick: 0,
        spawns: [record].into(),
    })
    .expect("a one-record batch with a zero tick is admitted");

    assert_eq!(batch.server_tick(), 0);
    assert_eq!(batch.spawns().len(), 1);
}

#[test]
fn passive_spawn_rejects_depths_before_yaw_and_health() {
    // The record carries three violations at once, and the dimension rule
    // fires first, matching the hostile spawn's Go validator order.
    let mut parts = passive_spawn_seed();
    parts.dimension = Dimension::DEPTHS;
    parts.yaw = f32::NAN;
    parts.health = 0;

    let result = PassiveSpawnRecord::try_new(parts);

    assert_eq!(result, Err(DomainError::InvalidDimension));
}

#[test]
fn passive_spawn_health_accepts_one_and_twenty_and_rejects_zero_and_twenty_one() {
    // The passive health range is the hostile one: `1..=20` with both ends
    // publishable, zero removed rather than published and twenty-one above
    // the Go `core.MaxHealth`.
    for health in [1u8, 20] {
        let mut parts = passive_spawn_seed();
        parts.health = health;

        let record = PassiveSpawnRecord::try_new(parts).expect("the boundary health is admitted");

        assert_eq!(record.health(), health);
    }

    for health in [0u8, 21] {
        let mut parts = passive_spawn_seed();
        parts.health = health;

        let result = PassiveSpawnRecord::try_new(parts);

        assert_eq!(result, Err(DomainError::InvalidSurvivalValue));
    }
}

#[test]
fn passive_state_keeps_velocity_and_boolean_grazing_without_dimension() {
    // The parts literal names every field, so a dimension a later node adds
    // fails to compile here. Grazing is stored as the `bool` the domain
    // owns; the wire's 0/1 conversion belongs to the adapters.
    let record =
        PassiveStateRecord::try_new(passive_state_seed()).expect("the seed record is admitted");

    assert_eq!(
        record.id(),
        PassiveId::try_new(5).expect("the identity is nonzero")
    );
    assert_eq!(record.position().get(), [-3.5, 70.0, 8.25]);
    assert_eq!(record.velocity().get(), [-0.5, 0.0, 0.75]);
    assert_eq!(record.yaw(), -1.25);
    assert_eq!(record.health(), 10);
    assert!(!record.grazing());

    let mut grazing_parts = passive_state_seed();
    grazing_parts.grazing = true;
    let grazing = PassiveStateRecord::try_new(grazing_parts).expect("the seed record is admitted");

    assert!(grazing.grazing());
}

#[test]
fn passive_state_rejects_non_finite_yaw_before_health() {
    // The state record checks yaw then health, so the non-finite yaw reports
    // first and the out-of-range health is never reached.
    let mut parts = passive_state_seed();
    parts.yaw = f32::NAN;
    parts.health = 0;

    let result = PassiveStateRecord::try_new(parts);

    assert_eq!(result, Err(DomainError::NonFiniteRotation));
}

#[test]
fn passive_despawn_preserves_vanished_and_died() {
    // The closed reason pair survives construction unchanged, and the domain
    // stores the enum rather than the wire byte; the 0/1 conversion itself
    // belongs to the adapters and is never reproduced here.
    let vanished = PassiveDespawnRecord::new(
        PassiveId::try_new(5).expect("the identity is nonzero"),
        PassiveDespawnReason::Vanished,
    );
    let died = PassiveDespawnRecord::new(
        PassiveId::try_new(7).expect("the identity is nonzero"),
        PassiveDespawnReason::Died,
    );

    assert_eq!(
        vanished.id(),
        PassiveId::try_new(5).expect("the identity is nonzero")
    );
    assert_eq!(vanished.reason(), PassiveDespawnReason::Vanished);
    assert_eq!(
        died.id(),
        PassiveId::try_new(7).expect("the identity is nonzero")
    );
    assert_eq!(died.reason(), PassiveDespawnReason::Died);
}

#[test]
fn passive_batches_reject_empty_reversed_and_duplicate_ids() {
    // All three passive batch shapes own the same three relations the
    // hostile ones do: the work cap first, then emptiness, then strict
    // identity order.
    assert_eq!(
        PassiveSpawn::try_new(PassiveSpawnParts {
            server_tick: 7,
            spawns: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );
    assert_eq!(
        PassiveState::try_new(PassiveStateParts {
            server_tick: 7,
            states: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );
    assert_eq!(
        PassiveDespawn::try_new(PassiveDespawnParts {
            server_tick: 7,
            despawns: Box::new([]),
        }),
        Err(DomainError::EmptyStateBatch)
    );

    let reversed_spawns = [
        PassiveSpawnRecord::try_new({
            let mut parts = passive_spawn_seed();
            parts.id = PassiveId::try_new(2).expect("the identity is nonzero");
            parts
        })
        .expect("the seed record is admitted"),
        PassiveSpawnRecord::try_new({
            let mut parts = passive_spawn_seed();
            parts.id = PassiveId::try_new(1).expect("the identity is nonzero");
            parts
        })
        .expect("the seed record is admitted"),
    ];
    assert_eq!(
        PassiveSpawn::try_new(PassiveSpawnParts {
            server_tick: 7,
            spawns: reversed_spawns.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );

    let duplicate_states = [
        PassiveStateRecord::try_new(passive_state_seed()).expect("the seed record is admitted"),
        PassiveStateRecord::try_new(passive_state_seed()).expect("the seed record is admitted"),
    ];
    assert_eq!(
        PassiveState::try_new(PassiveStateParts {
            server_tick: 7,
            states: duplicate_states.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );

    let reversed_despawns = [
        PassiveDespawnRecord::new(
            PassiveId::try_new(2).expect("the identity is nonzero"),
            PassiveDespawnReason::Vanished,
        ),
        PassiveDespawnRecord::new(
            PassiveId::try_new(1).expect("the identity is nonzero"),
            PassiveDespawnReason::Died,
        ),
    ];
    assert_eq!(
        PassiveDespawn::try_new(PassiveDespawnParts {
            server_tick: 7,
            despawns: reversed_despawns.into(),
        }),
        Err(DomainError::InvalidStateOrder)
    );
}

#[test]
fn passive_batches_accept_sixty_five_records_beyond_one_packet() {
    // The passive packet maximum is the hostile one's 64, so the strictly
    // increasing 65-record form proves it is a transport budget rather than
    // a domain rule; every batch keeps its first and last record unchanged,
    // with no copy or sort.
    let expected_first_spawn = passive_spawn_records(1)[0];
    let expected_last_spawn = passive_spawn_records(65)[64];
    let spawns = PassiveSpawn::try_new(PassiveSpawnParts {
        server_tick: 3,
        spawns: passive_spawn_records(65).into(),
    })
    .expect("sixty-five strictly increasing records are a publishable domain batch");
    assert_eq!(spawns.spawns().len(), 65);
    assert_eq!(spawns.spawns()[0], expected_first_spawn);
    assert_eq!(spawns.spawns()[64], expected_last_spawn);

    let expected_first_state = passive_state_records(1)[0];
    let expected_last_state = passive_state_records(65)[64];
    let states = PassiveState::try_new(PassiveStateParts {
        server_tick: 3,
        states: passive_state_records(65).into(),
    })
    .expect("sixty-five strictly increasing records are a publishable domain batch");
    assert_eq!(states.states().len(), 65);
    assert_eq!(states.states()[0], expected_first_state);
    assert_eq!(states.states()[64], expected_last_state);

    let despawns = PassiveDespawn::try_new(PassiveDespawnParts {
        server_tick: 3,
        despawns: passive_despawn_records(65).into(),
    })
    .expect("sixty-five strictly increasing records are a publishable domain batch");
    assert_eq!(despawns.despawns().len(), 65);
    assert_eq!(
        despawns.despawns()[0],
        PassiveDespawnRecord::new(
            PassiveId::try_new(1).expect("the identity is nonzero"),
            PassiveDespawnReason::Vanished,
        )
    );
    assert_eq!(
        despawns.despawns()[64],
        PassiveDespawnRecord::new(
            PassiveId::try_new(65).expect("the identity is nonzero"),
            PassiveDespawnReason::Vanished,
        )
    );
}

#[test]
fn mob_records_require_checked_nonzero_identity_and_finite_vectors() {
    // Identity zero and a non-finite vector fail inside the checked parts
    // before a record can be assembled, so no mob constructor ever sees
    // them: `HostileId` and `PassiveId` reject the absent form and
    // `FiniteVec3` rejects a NaN or infinite component for both the pose
    // and the velocity.
    assert_eq!(HostileId::try_new(0), Err(DomainError::InvalidIdentity));
    assert_eq!(PassiveId::try_new(0), Err(DomainError::InvalidIdentity));

    assert_eq!(
        FiniteVec3::try_new([f32::NAN, 64.0, 0.0]),
        Err(DomainError::NonFiniteValue)
    );
    assert_eq!(
        FiniteVec3::try_new([0.0, f32::NEG_INFINITY, 0.0]),
        Err(DomainError::NonFiniteValue)
    );

    // The finite seed pose and velocity still construct, so the rejection
    // above is the finiteness rule rather than an over-tight gate.
    assert!(FiniteVec3::try_new([12.5, 64.0, -7.25]).is_ok());
    assert!(FiniteVec3::try_new([0.5, 0.0, -0.25]).is_ok());
}
