//! Remote-player and companion observation contracts for `mornlea_domain`.
//!
//! The Go baseline is the `protocol.RemotePlayerSpawn`, `RemotePlayerDespawn`,
//! `RemotePlayerStates`, `CompanionSpawn`, `CompanionStates` and
//! `CompanionDespawn` records in `packages/shared/network/protocol`, which are
//! the visibility-derived observations an authoritative session publishes
//! about the other players and the companions a subscriber can see. Every
//! value rule below is the Go validator's rule for the same record, so a
//! record this crate admits is a record the protocol layer admits and the
//! other way around.
//!
//! Three Go wire rules deliberately stay out of these records, because the
//! domain value they describe does not carry them and a case pinning one could
//! not be replayed against the Rust consumer. The 7-record remote-player batch
//! maximum and the 4-record companion batch maximum are transport budgets the
//! protocol layer applies, so the domain requires only a nonempty batch whose
//! identities are strictly increasing. A record's own tick is the enclosing
//! publish tick, which the domain names `server_tick` everywhere; and the
//! flattened yaw and pitch are one validated `LookAngles` named `look`.
//!
//! The per-record rules differ by subject. A remote-player record accepts
//! either playable dimension and any finite pitch, because a peer session is
//! not the local player and the Go validator publishes no vertical look bound
//! for it. A companion record accepts the overworld alone and a pitch inside
//! the inclusive vertical look range, which is the Go `validCompanionPose`
//! rule. Neither record carries a profile, a persona, mining progress or a
//! velocity: those are authority-side quantities the companion and remote
//! publications never put on the wire.

use mornlea_domain::{
    CompanionDespawn, CompanionId, CompanionName, CompanionSpawn, CompanionSpawnParts,
    CompanionState, CompanionStateParts, CompanionStates, CompanionStatesParts, Dimension,
    DisplayName, DomainError, FiniteVec3, LookAngles, PlayerId, RemotePlayerDespawn,
    RemotePlayerSpawn, RemotePlayerSpawnParts, RemotePlayerState, RemotePlayerStateParts,
    RemotePlayerStates, RemotePlayerStatesParts,
};
use std::f32::consts::FRAC_PI_2;

/// One player identity ending `fe`, which sorts before the `ff` one.
const PLAYER_ID_FE: [u8; 16] = [
    0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xfe,
];

/// One player identity ending `ff`, which sorts after the `fe` one.
const PLAYER_ID_FF: [u8; 16] = [
    0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
];

/// One companion identity ending `fe`, which sorts before the `ff` one.
const COMPANION_ID_FE: [u8; 16] = [
    0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x48, 0x89, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0xfe,
];

/// One companion identity ending `ff`, which sorts after the `fe` one.
const COMPANION_ID_FF: [u8; 16] = [
    0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x48, 0x89, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00, 0xff,
];

/// The first player identity in the batch's canonical order.
fn player_id_first() -> PlayerId {
    PlayerId::try_from_bytes(PLAYER_ID_FE).expect("the identity is a nonzero UUIDv4")
}

/// The second player identity in the batch's canonical order.
fn player_id_second() -> PlayerId {
    PlayerId::try_from_bytes(PLAYER_ID_FF).expect("the identity is a nonzero UUIDv4")
}

/// The first companion identity in the batch's canonical order.
fn companion_id_first() -> CompanionId {
    CompanionId::try_from_bytes(COMPANION_ID_FE).expect("the identity is a nonzero UUIDv4")
}

/// The second companion identity in the batch's canonical order.
fn companion_id_second() -> CompanionId {
    CompanionId::try_from_bytes(COMPANION_ID_FF).expect("the identity is a nonzero UUIDv4")
}

/// One canonical player display name.
fn display_name() -> DisplayName {
    DisplayName::try_from_canonical("Alice".to_string()).expect("the name is canonical")
}

/// One canonical companion name.
fn companion_name() -> CompanionName {
    CompanionName::try_from_canonical("Buddy".to_string()).expect("the name is canonical")
}

/// The seed remote-player spawn: the `ff` identity, a canonical name, tick 41,
/// the overworld, a finite pose and a finite look.
///
/// The literal lists every field of the parts struct, so a field a later node
/// adds to the record fails to compile here rather than silently widening the
/// publication.
fn remote_player_spawn_seed() -> RemotePlayerSpawnParts {
    RemotePlayerSpawnParts {
        player_id: player_id_second(),
        display_name: display_name(),
        server_tick: 41,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([1.5, 64.0, -3.25]).expect("the pose is finite"),
        look: LookAngles::try_new(0.5, -0.25).expect("the angles are finite"),
    }
}

/// The seed remote-player state record: the `fe` identity, the overworld, a
/// finite pose, a finite look and a false reset.
fn remote_player_state_seed() -> RemotePlayerStateParts {
    RemotePlayerStateParts {
        player_id: player_id_first(),
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([2.5, 65.0, -4.25]).expect("the pose is finite"),
        look: LookAngles::try_new(-1.0, 0.75).expect("the angles are finite"),
        reset: false,
    }
}

/// The seed companion spawn: the `ff` identity, a canonical name, tick 41, the
/// overworld, a finite pose and a finite look.
fn companion_spawn_seed() -> CompanionSpawnParts {
    CompanionSpawnParts {
        id: companion_id_second(),
        name: companion_name(),
        server_tick: 41,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([-8.5, 70.0, 12.25]).expect("the pose is finite"),
        look: LookAngles::try_new(3.0, -0.5).expect("the angles are finite"),
    }
}

/// The seed companion state record: the `fe` identity, the overworld, a finite
/// pose, a finite look and a false reset.
fn companion_state_seed() -> CompanionStateParts {
    CompanionStateParts {
        id: companion_id_first(),
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([-7.5, 70.0, 11.25]).expect("the pose is finite"),
        look: LookAngles::try_new(-3.0, 0.5).expect("the angles are finite"),
        reset: false,
    }
}

/// One remote-player batch of two records: the canonical `fe` before `ff`
/// order, so a boundary row is always the seed batch plus one change.
fn remote_player_batch(records: [RemotePlayerState; 2]) -> RemotePlayerStatesParts {
    RemotePlayerStatesParts {
        server_tick: 7,
        states: Box::new(records),
    }
}

/// One companion batch of two records: the canonical `fe` before `ff` order.
fn companion_batch(records: [CompanionState; 2]) -> CompanionStatesParts {
    CompanionStatesParts {
        server_tick: 7,
        states: Box::new(records),
    }
}

#[test]
fn event_people_remote_player_spawn_seed_is_admitted_and_keeps_every_field() {
    let spawn = RemotePlayerSpawn::new(remote_player_spawn_seed());

    assert_eq!(spawn.player_id(), player_id_second());
    assert_eq!(spawn.display_name().as_str(), "Alice");
    assert_eq!(spawn.server_tick(), 41);
    assert_eq!(spawn.dimension(), Dimension::OVERWORLD);
    assert_eq!(spawn.position().get(), [1.5, 64.0, -3.25]);
    assert_eq!(spawn.look().yaw(), 0.5);
    assert_eq!(spawn.look().pitch(), -0.25);
}

#[test]
fn event_people_remote_player_spawn_accepts_a_zero_server_tick() {
    // The Go validator admits a zero tick on every remote-player record: the
    // tick is the publish instant, and a world that has not advanced yet still
    // publishes a spawn.
    let mut parts = remote_player_spawn_seed();
    parts.server_tick = 0;

    let spawn = RemotePlayerSpawn::new(parts);

    assert_eq!(spawn.server_tick(), 0);
}

#[test]
fn event_people_remote_player_spawn_accepts_the_depths_dimension() {
    // A peer session plays in either dimension, so a remote player is a
    // two-dimension observation rather than an overworld-only one.
    let mut parts = remote_player_spawn_seed();
    parts.dimension = Dimension::DEPTHS;

    let spawn = RemotePlayerSpawn::new(parts);

    assert_eq!(spawn.dimension(), Dimension::DEPTHS);
}

#[test]
fn event_people_remote_player_spawn_accepts_a_pitch_outside_the_vertical_look_range() {
    // The companion vertical look bound is a companion rule. A remote player
    // carries no pitch bound at all, so the same value a companion record
    // rejects is publishable here.
    let mut parts = remote_player_spawn_seed();
    parts.look = LookAngles::try_new(0.0, 4.0).expect("4.0 is finite");

    let spawn = RemotePlayerSpawn::new(parts);

    assert_eq!(spawn.look().pitch(), 4.0);
}

#[test]
fn event_people_remote_player_spawn_names_a_checked_identity() {
    // The spawn and the despawn name a checked identity rather than raw bytes,
    // so the zero and the wrong-version forms fail before a record exists.
    let zero = PlayerId::try_from_bytes([0; 16]);
    let wrong_version = PlayerId::try_from_bytes([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x36, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ]);

    assert_eq!(zero, Err(DomainError::InvalidIdentity));
    assert_eq!(wrong_version, Err(DomainError::InvalidIdentity));
}

#[test]
fn event_people_remote_player_despawn_keeps_the_checked_identity() {
    let despawn = RemotePlayerDespawn::new(player_id_first());

    assert_eq!(despawn.player_id(), player_id_first());
}

#[test]
fn event_people_remote_player_states_seed_is_admitted_and_keeps_every_record() {
    // A one-record batch with a zero tick, which the Go validator admits: the
    // tick is the publish instant rather than a liveness marker.
    let mut parts = remote_player_state_seed();
    parts.reset = true;
    let states = RemotePlayerStates::try_new(RemotePlayerStatesParts {
        server_tick: 0,
        states: Box::new([RemotePlayerState::new(parts)]),
    })
    .expect("a one-record batch with a zero tick is admitted");

    assert_eq!(states.server_tick(), 0);
    assert_eq!(states.states().len(), 1);
    assert_eq!(states.states()[0].player_id(), player_id_first());
    assert_eq!(states.states()[0].dimension(), Dimension::OVERWORLD);
    assert_eq!(states.states()[0].position().get(), [2.5, 65.0, -4.25]);
    assert_eq!(states.states()[0].look().yaw(), -1.0);
    assert_eq!(states.states()[0].look().pitch(), 0.75);
    assert!(states.states()[0].reset());
}

#[test]
fn event_people_remote_player_states_accept_the_depths_dimension_and_an_unbounded_pitch() {
    // A remote-player record accepts either playable dimension and any finite
    // pitch, so both rules that narrow a companion record stay open here.
    let mut first = remote_player_state_seed();
    first.dimension = Dimension::DEPTHS;
    first.look = LookAngles::try_new(0.0, 4.0).expect("4.0 is finite");
    let mut second = remote_player_state_seed();
    second.player_id = player_id_second();
    second.dimension = Dimension::DEPTHS;

    let states = RemotePlayerStates::try_new(remote_player_batch([
        RemotePlayerState::new(first),
        RemotePlayerState::new(second),
    ]))
    .expect("the depths dimension and an unbounded pitch are admitted");

    assert_eq!(states.states()[0].dimension(), Dimension::DEPTHS);
    assert_eq!(states.states()[0].look().pitch(), 4.0);
    assert_eq!(states.states()[1].dimension(), Dimension::DEPTHS);
}

#[test]
fn event_people_remote_player_states_reject_an_empty_batch() {
    // An empty batch names nothing to apply, which no authority publishes: the
    // Go validator rejects a count below one.
    let result = RemotePlayerStates::try_new(RemotePlayerStatesParts {
        server_tick: 7,
        states: Box::new([]),
    });

    assert_eq!(result, Err(DomainError::EmptyStateBatch));
}

#[test]
fn event_people_remote_player_states_reject_reversed_identities() {
    // The two identities in the opposite order, so `ff` precedes `fe`: the Go
    // validator rejects a batch that is not strictly sorted by raw UUID bytes.
    let mut second = remote_player_state_seed();
    second.player_id = player_id_second();
    let first = RemotePlayerState::new(remote_player_state_seed());

    let result =
        RemotePlayerStates::try_new(remote_player_batch([RemotePlayerState::new(second), first]));

    assert_eq!(result, Err(DomainError::InvalidStateOrder));
}

#[test]
fn event_people_remote_player_states_reject_duplicate_identities() {
    // The same identity twice: strict ordering already rules out a duplicate,
    // which would otherwise apply one record on top of itself.
    let first = RemotePlayerState::new(remote_player_state_seed());
    let second = RemotePlayerState::new(remote_player_state_seed());

    let result = RemotePlayerStates::try_new(remote_player_batch([first, second]));

    assert_eq!(result, Err(DomainError::InvalidStateOrder));
}

#[test]
fn event_people_remote_player_states_admit_more_records_than_the_wire_batch_maximum() {
    // The 7-record wire maximum is a transport budget, so a domain batch of
    // eight strictly increasing identities stays publishable: a protocol
    // adapter enforces the wire cap when it splits a batch for the frame.
    let mut states = Vec::with_capacity(8);
    for index in 0..8u8 {
        let mut parts = remote_player_state_seed();
        parts.player_id = PlayerId::try_from_bytes([
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0x00, index,
        ])
        .expect("each identity is a nonzero UUIDv4");
        states.push(RemotePlayerState::new(parts));
    }

    let batch = RemotePlayerStates::try_new(RemotePlayerStatesParts {
        server_tick: 9,
        states: states.into_boxed_slice(),
    })
    .expect("eight strictly increasing identities are a publishable domain batch");

    assert_eq!(batch.states().len(), 8);
}

#[test]
fn event_people_companion_spawn_seed_is_admitted_and_keeps_every_field() {
    let spawn = CompanionSpawn::try_new(companion_spawn_seed())
        .expect("the overworld, a finite pose and a bounded pitch are admitted");

    assert_eq!(spawn.id(), companion_id_second());
    assert_eq!(spawn.name().as_str(), "Buddy");
    assert_eq!(spawn.server_tick(), 41);
    assert_eq!(spawn.dimension(), Dimension::OVERWORLD);
    assert_eq!(spawn.position().get(), [-8.5, 70.0, 12.25]);
    assert_eq!(spawn.look().yaw(), 3.0);
    assert_eq!(spawn.look().pitch(), -0.5);
}

#[test]
fn event_people_companion_spawn_accepts_both_ends_of_the_vertical_look_range() {
    // The Go bound is inclusive at both ends, so the two limit values are the
    // published extremes rather than rejections.
    for pitch in [-FRAC_PI_2, FRAC_PI_2] {
        let mut parts = companion_spawn_seed();
        parts.look = LookAngles::try_new(0.0, pitch).expect("the limit is finite");

        let spawn =
            CompanionSpawn::try_new(parts).expect("the inclusive vertical look limit is admitted");

        assert_eq!(spawn.look().pitch(), pitch);
    }
}

#[test]
fn event_people_companion_spawn_rejects_the_depths_dimension() {
    // A companion lives in the overworld alone: the Go validator accepts only
    // `core.Overworld`, and a companion in the depths would be a record the
    // authority never publishes.
    let mut parts = companion_spawn_seed();
    parts.dimension = Dimension::DEPTHS;

    let result = CompanionSpawn::try_new(parts);

    assert_eq!(result, Err(DomainError::InvalidCompanionDimension));
}

#[test]
fn event_people_companion_spawn_rejects_the_next_float_above_the_vertical_look_limit() {
    // The value one representable step above the inclusive limit, so the bound
    // is pinned at the exact float the Go `math.Pi/2` comparison rejects rather
    // than at a nearby decimal.
    let mut parts = companion_spawn_seed();
    parts.look = LookAngles::try_new(0.0, FRAC_PI_2.next_up()).expect("the value is finite");

    let result = CompanionSpawn::try_new(parts);

    assert_eq!(result, Err(DomainError::InvalidCompanionPitch));
}

#[test]
fn event_people_companion_spawn_names_a_checked_identity() {
    // The spawn and the despawn name a checked companion identity, so the
    // absent form a chat event uses for a rejection never reaches a companion
    // publication.
    let zero = CompanionId::try_from_bytes([0; 16]);

    assert_eq!(zero, Err(DomainError::InvalidIdentity));
}

#[test]
fn event_people_companion_spawn_rejects_a_name_with_embedded_whitespace() {
    // The companion name rule is the display-name rule plus a rejection of any
    // Unicode whitespace, so a spaced name fails while a player display name
    // carrying the same space would be admitted.
    let spaced = CompanionName::try_from_canonical("Best Buddy".to_string());

    assert_eq!(spaced, Err(DomainError::InvalidText));
    assert!(DisplayName::try_from_canonical("Best Buddy".to_string()).is_ok());
}

#[test]
fn event_people_companion_despawn_keeps_the_checked_identity() {
    let despawn = CompanionDespawn::new(companion_id_first());

    assert_eq!(despawn.id(), companion_id_first());
}

#[test]
fn event_people_companion_state_seed_is_admitted_and_keeps_every_field() {
    let state = CompanionState::try_new(companion_state_seed())
        .expect("the overworld, a finite pose and a bounded pitch are admitted");

    assert_eq!(state.id(), companion_id_first());
    assert_eq!(state.dimension(), Dimension::OVERWORLD);
    assert_eq!(state.position().get(), [-7.5, 70.0, 11.25]);
    assert_eq!(state.look().yaw(), -3.0);
    assert_eq!(state.look().pitch(), 0.5);
    assert!(!state.reset());
}

#[test]
fn event_people_companion_state_rejects_the_depths_dimension() {
    // The per-record companion rule applies inside a batch as well, so the
    // record itself refuses a dimension the authority never publishes for a
    // companion and no batch can carry one.
    let mut parts = companion_state_seed();
    parts.dimension = Dimension::DEPTHS;

    let result = CompanionState::try_new(parts);

    assert_eq!(result, Err(DomainError::InvalidCompanionDimension));
}

#[test]
fn event_people_companion_state_rejects_the_next_float_above_the_vertical_look_limit() {
    let mut parts = companion_state_seed();
    parts.look = LookAngles::try_new(0.0, FRAC_PI_2.next_up()).expect("the value is finite");

    let result = CompanionState::try_new(parts);

    assert_eq!(result, Err(DomainError::InvalidCompanionPitch));
}

#[test]
fn event_people_companion_state_rejects_the_pitch_a_remote_player_record_accepts() {
    // The corresponding remote-player pitch is accepted, so the two subjects
    // keep their own rule: 4.0 is a legal remote pitch and an illegal
    // companion one.
    let mut parts = companion_state_seed();
    parts.look = LookAngles::try_new(0.0, 4.0).expect("4.0 is finite");

    assert_eq!(
        CompanionState::try_new(parts),
        Err(DomainError::InvalidCompanionPitch)
    );

    let mut remote = remote_player_state_seed();
    remote.look = LookAngles::try_new(0.0, 4.0).expect("4.0 is finite");
    let batch = RemotePlayerStates::try_new(RemotePlayerStatesParts {
        server_tick: 7,
        states: Box::new([RemotePlayerState::new(remote)]),
    });

    assert_eq!(
        batch.expect("the remote pitch is unbounded").states()[0]
            .look()
            .pitch(),
        4.0
    );
}

#[test]
fn event_people_companion_states_seed_is_admitted_and_keeps_every_record() {
    let mut parts = companion_state_seed();
    parts.reset = true;
    let states = CompanionStates::try_new(CompanionStatesParts {
        server_tick: 0,
        states: Box::new([
            CompanionState::try_new(parts).expect("the seed companion record is admitted")
        ]),
    })
    .expect("a one-record batch with a zero tick is admitted");

    assert_eq!(states.server_tick(), 0);
    assert_eq!(states.states().len(), 1);
    assert_eq!(states.states()[0].id(), companion_id_first());
    assert_eq!(states.states()[0].dimension(), Dimension::OVERWORLD);
    assert_eq!(states.states()[0].position().get(), [-7.5, 70.0, 11.25]);
    assert_eq!(states.states()[0].look().yaw(), -3.0);
    assert_eq!(states.states()[0].look().pitch(), 0.5);
    assert!(states.states()[0].reset());
}

#[test]
fn event_people_companion_states_reject_an_empty_batch() {
    let result = CompanionStates::try_new(CompanionStatesParts {
        server_tick: 7,
        states: Box::new([]),
    });

    assert_eq!(result, Err(DomainError::EmptyStateBatch));
}

#[test]
fn event_people_companion_states_reject_reversed_identities() {
    // The two companion identities in the opposite order, so `ff` precedes
    // `fe`: the same strict-order rule the remote-player batch applies.
    let mut second = companion_state_seed();
    second.id = companion_id_second();
    let first = CompanionState::try_new(companion_state_seed())
        .expect("the seed companion record is admitted");

    let result = CompanionStates::try_new(companion_batch([
        CompanionState::try_new(second).expect("the seed companion record is admitted"),
        first,
    ]));

    assert_eq!(result, Err(DomainError::InvalidStateOrder));
}

#[test]
fn event_people_companion_states_reject_duplicate_identities() {
    let first = CompanionState::try_new(companion_state_seed())
        .expect("the seed companion record is admitted");
    let second = CompanionState::try_new(companion_state_seed())
        .expect("the seed companion record is admitted");

    let result = CompanionStates::try_new(companion_batch([first, second]));

    assert_eq!(result, Err(DomainError::InvalidStateOrder));
}

#[test]
fn event_people_companion_states_admit_more_records_than_the_wire_batch_maximum() {
    // The 4-record wire maximum is the companion activity limit the protocol
    // layer applies, so a domain batch of five strictly increasing companion
    // identities stays publishable.
    let mut states = Vec::with_capacity(5);
    for index in 0..5u8 {
        let mut parts = companion_state_seed();
        parts.id = CompanionId::try_from_bytes([
            0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x48, 0x89, 0xaa, 0xbb, 0xcc, 0xdd, 0x00, 0x00,
            0x00, index,
        ])
        .expect("each identity is a nonzero UUIDv4");
        states
            .push(CompanionState::try_new(parts).expect("each seed companion record is admitted"));
    }

    let batch = CompanionStates::try_new(CompanionStatesParts {
        server_tick: 9,
        states: states.into_boxed_slice(),
    })
    .expect("five strictly increasing companion identities are a publishable domain batch");

    assert_eq!(batch.states().len(), 5);
}

#[test]
fn event_people_records_carry_no_profile_persona_mining_or_velocity() {
    // Every field of the six records is named by these literals, so a field a
    // later node adds fails to compile here instead of quietly widening the
    // publication. The companion literals carry no profile, no persona, no
    // mining progress and no velocity, which is the Go `CompanionUpdate`
    // projection: `velocity`, `ground` and `mining` are authority-side
    // quantities the companion wire never carries. The remote-player literals
    // carry the same absence beside the survival, inventory and mining fields
    // the private player publication owns.
    let spawn = CompanionSpawn::try_new(CompanionSpawnParts {
        id: companion_id_first(),
        name: companion_name(),
        server_tick: 41,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([0.0, 64.0, 0.0]).expect("the pose is finite"),
        look: LookAngles::try_new(0.0, 0.0).expect("the angles are finite"),
    })
    .expect("the seed companion spawn is admitted");
    let state = CompanionState::try_new(CompanionStateParts {
        id: companion_id_first(),
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([0.0, 64.0, 0.0]).expect("the pose is finite"),
        look: LookAngles::try_new(0.0, 0.0).expect("the angles are finite"),
        reset: false,
    })
    .expect("the seed companion state is admitted");
    let remote_spawn = RemotePlayerSpawn::new(RemotePlayerSpawnParts {
        player_id: player_id_first(),
        display_name: display_name(),
        server_tick: 41,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([0.0, 64.0, 0.0]).expect("the pose is finite"),
        look: LookAngles::try_new(0.0, 0.0).expect("the angles are finite"),
    });
    let remote_state = RemotePlayerState::new(RemotePlayerStateParts {
        player_id: player_id_first(),
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([0.0, 64.0, 0.0]).expect("the pose is finite"),
        look: LookAngles::try_new(0.0, 0.0).expect("the angles are finite"),
        reset: false,
    });

    assert_eq!(spawn.position().get(), [0.0, 64.0, 0.0]);
    assert_eq!(state.position().get(), [0.0, 64.0, 0.0]);
    assert_eq!(remote_spawn.position().get(), [0.0, 64.0, 0.0]);
    assert_eq!(remote_state.position().get(), [0.0, 64.0, 0.0]);
}
