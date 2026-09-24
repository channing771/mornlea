//! The three projectile publication families: the common fallible surface, the
//! closed kind rule, the two-dimension rule, and the pre-parse wire ceiling.
//!
//! `ProjectileSpawn` (S/Play/29), `ProjectileState` (S/Play/30) and
//! `ProjectileDespawn` (S/Play/31) publish the projectiles one subscriber can
//! see. All three carry a `u64` server tick and a one-byte record count, all
//! three admit exactly `1..=MAX_PROJECTILE_RECORDS` records ordered by the
//! checked domain `ProjectileId`, which is a nonzero `u64` newtype: a zero ID
//! is the absent form, so it cannot be constructed on the outbound surface and
//! is refused at the identity boundary where it is read. Trajectory, collision
//! and spawn eligibility are authority/kernel concerns that never reach this
//! wire.
//!
//! The three records differ, and the difference is the Go one: a spawn record
//! carries the kind, the dimension, the position and the velocity; a state
//! record carries the identity and the position alone, because a projectile's
//! kind, dimension and velocity are fixed for its whole life; and a despawn
//! record carries the bare eight-byte identity. The fixed strides are
//! therefore 37, 20 and 8 bytes.
//!
//! The kind and the dimension are independent on this wire: all four
//! combinations admit, because a player's bow works in either dimension and the
//! narrower kind-by-dimension policy is an authority rule the codec does not
//! publish. The server may refuse a combination later; this layer must not.
//!
//! The batch length rule is the exact-remaining-length rule on both sides: the
//! Go decoder rejects a payload whose remaining length is not exactly `count`
//! records before it reads one, so a short and a padded payload answer at the
//! same truncation boundary.
//!
//! Unlike the hostile and passive batches, these three decoders also apply the
//! family's fixed wire ceiling before they read a byte, because the Go decode
//! path applies the per-family maximum before the family decoder runs. An
//! over-ceiling payload therefore answers the capacity boundary on both sides
//! instead of the truncation boundary the count and length rules would report.
//!
//! The corpus evidence these families publish is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.

use mornlea_domain::{Dimension, ProjectileId};
use mornlea_protocol::{
    MAX_PROJECTILE_RECORDS, PROJECTILE_DESPAWN_MAX_WIRE_BYTES, PROJECTILE_DESPAWN_WIRE_BYTES,
    PROJECTILE_KIND_ARROW, PROJECTILE_KIND_SHARD, PROJECTILE_SPAWN_MAX_WIRE_BYTES,
    PROJECTILE_SPAWN_WIRE_BYTES, PROJECTILE_STATE_MAX_WIRE_BYTES, PROJECTILE_STATE_WIRE_BYTES,
    ProjectileDespawn, ProjectileSpawn, ProjectileSpawnRecord, ProjectileState,
    ProjectileStateRecord, ProtocolError,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The reviewed 83-byte ProjectileSpawn literal: tick 0, two records strictly
/// ordered by ID, the two kinds across the two playable dimensions, and one
/// negative-zero word in each of the position and the velocity.
const PROJECTILE_SPAWN_WIRE: [u8; 83] = [
    // tick 0 and the declared count of two records
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
    // first record: ID 1, kind 0 (shard), dimension 0 (overworld),
    // position (-0.0, 1.0, 2.0), velocity (0.5, -1.25, 0.0)
    0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x3f, 0x00, 0x00, 0xa0,
    0xbf, 0x00, 0x00, 0x00, 0x00,
    // second record: ID 2, kind 1 (arrow), dimension 1 (depths),
    // position (3.0, 4.0, 5.0), velocity (-0.0, 2.0, 3.0)
    0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40,
    0x40, 0x00, 0x00, 0x80, 0x40, 0x00, 0x00, 0xa0, 0x40, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00,
    0x40, 0x00, 0x00, 0x40, 0x40,
];

/// The reviewed 49-byte ProjectileState literal: tick 0, two records strictly
/// ordered by ID, and nothing but the position, so the record stride is 20.
const PROJECTILE_STATE_WIRE: [u8; 49] = [
    // tick 0 and the declared count of two records
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
    // first record: ID 1, position (-0.0, 1.0, 2.0)
    0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f,
    0x00, 0x00, 0x00, 0x40, // second record: ID 2, position (3.0, 4.0, 5.0)
    0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x80, 0x40,
    0x00, 0x00, 0xa0, 0x40,
];

/// The reviewed 25-byte ProjectileDespawn literal: tick 0 and the two strictly
/// ordered identities.
const PROJECTILE_DESPAWN_WIRE: [u8; 25] = [
    // tick 0, the declared count of two records, and the identities 1 and 2
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
];

/// Wraps one nonzero projectile identity, which the domain rule already admits.
fn projectile_id(id: u64) -> ProjectileId {
    ProjectileId::try_new(id).expect("reviewed nonzero projectile identity")
}

/// The first reviewed spawn record: a shard in the overworld.
fn spawn_record_one() -> ProjectileSpawnRecord {
    ProjectileSpawnRecord {
        id: projectile_id(1),
        kind: PROJECTILE_KIND_SHARD,
        dimension: Dimension::OVERWORLD,
        position: [-0.0, 1.0, 2.0],
        velocity: [0.5, -1.25, 0.0],
    }
}

/// The second reviewed spawn record: an arrow in the depths, which is the
/// combination the authority never publishes but this wire admits.
fn spawn_record_two() -> ProjectileSpawnRecord {
    ProjectileSpawnRecord {
        id: projectile_id(2),
        kind: PROJECTILE_KIND_ARROW,
        dimension: Dimension::DEPTHS,
        position: [3.0, 4.0, 5.0],
        velocity: [-0.0, 2.0, 3.0],
    }
}

/// The canonical two-record spawn batch the reviewed literal carries.
fn canonical_spawn() -> ProjectileSpawn {
    ProjectileSpawn::new(0, vec![spawn_record_one(), spawn_record_two()])
        .expect("the canonical batch is valid")
}

/// The first reviewed state record, whose position carries a negative zero.
fn state_record_one() -> ProjectileStateRecord {
    ProjectileStateRecord {
        id: projectile_id(1),
        position: [-0.0, 1.0, 2.0],
    }
}

/// The second reviewed state record.
fn state_record_two() -> ProjectileStateRecord {
    ProjectileStateRecord {
        id: projectile_id(2),
        position: [3.0, 4.0, 5.0],
    }
}

/// The canonical two-record state batch the reviewed literal carries.
fn canonical_state() -> ProjectileState {
    ProjectileState::new(0, vec![state_record_one(), state_record_two()])
        .expect("the canonical batch is valid")
}

/// The canonical despawn batch the reviewed literal carries.
fn canonical_despawn() -> ProjectileDespawn {
    ProjectileDespawn::new(0, vec![projectile_id(1), projectile_id(2)])
        .expect("the canonical batch is valid")
}

/// The full 128-record spawn batch, the fixed record ceiling, with ascending
/// identities on every entry point.
fn full_spawn() -> ProjectileSpawn {
    let spawns: Vec<ProjectileSpawnRecord> = (1..=MAX_PROJECTILE_RECORDS as u64)
        .map(|id| ProjectileSpawnRecord {
            id: projectile_id(id),
            ..spawn_record_one()
        })
        .collect();
    ProjectileSpawn::new(0, spawns).expect("the full batch is valid")
}

/// The full 128-record state batch, the fixed record ceiling.
fn full_state() -> ProjectileState {
    let states: Vec<ProjectileStateRecord> = (1..=MAX_PROJECTILE_RECORDS as u64)
        .map(|id| ProjectileStateRecord {
            id: projectile_id(id),
            ..state_record_one()
        })
        .collect();
    ProjectileState::new(0, states).expect("the full batch is valid")
}

/// The full 128-record despawn batch, the fixed record ceiling.
fn full_despawn() -> ProjectileDespawn {
    let ids: Vec<ProjectileId> = (1..=MAX_PROJECTILE_RECORDS as u64)
        .map(projectile_id)
        .collect();
    ProjectileDespawn::new(0, ids).expect("the full batch is valid")
}

/// Renders one spawn payload: the tick, the one-byte count and the records in
/// the Go field order — identity, kind, dimension, position, velocity.
fn projectile_spawn_bytes(tick: u64, records: &[ProjectileSpawnRecord]) -> Vec<u8> {
    let mut wire = Vec::with_capacity(9 + records.len() * PROJECTILE_SPAWN_WIRE_BYTES);
    wire.extend_from_slice(&tick.to_le_bytes());
    wire.push(records.len() as u8);
    for record in records {
        wire.extend_from_slice(&record.id.get().to_le_bytes());
        wire.push(record.kind);
        wire.extend_from_slice(&i32::from(record.dimension.get()).to_le_bytes());
        for value in record.position {
            wire.extend_from_slice(&value.to_bits().to_le_bytes());
        }
        for value in record.velocity {
            wire.extend_from_slice(&value.to_bits().to_le_bytes());
        }
    }
    wire
}

/// Renders one state payload: the tick, the one-byte count and the records,
/// which carry the identity and the position alone.
fn projectile_state_bytes(tick: u64, records: &[ProjectileStateRecord]) -> Vec<u8> {
    let mut wire = Vec::with_capacity(9 + records.len() * PROJECTILE_STATE_WIRE_BYTES);
    wire.extend_from_slice(&tick.to_le_bytes());
    wire.push(records.len() as u8);
    for record in records {
        wire.extend_from_slice(&record.id.get().to_le_bytes());
        for value in record.position {
            wire.extend_from_slice(&value.to_bits().to_le_bytes());
        }
    }
    wire
}

/// Renders one despawn payload: the tick, the one-byte count and the identities.
fn projectile_despawn_bytes(tick: u64, ids: &[ProjectileId]) -> Vec<u8> {
    let mut wire = Vec::with_capacity(9 + ids.len() * PROJECTILE_DESPAWN_WIRE_BYTES);
    wire.extend_from_slice(&tick.to_le_bytes());
    wire.push(ids.len() as u8);
    for id in ids {
        wire.extend_from_slice(&id.get().to_le_bytes());
    }
    wire
}

/// Asserts the shared surface contract for one canonical batch: the reviewed
/// length, the exact window, the padded prefix, the allocating wrapper and the
/// decode/re-encode round trip.
fn assert_round_trip(
    label: &str,
    payload: &[u8],
    validate: impl FnOnce() -> Result<(), ProtocolError>,
    encoded_len: impl FnOnce() -> Result<usize, ProtocolError>,
    encode_into: impl Fn(&mut [u8]) -> Result<usize, ProtocolError>,
    encode: impl FnOnce() -> Result<Vec<u8>, ProtocolError>,
    decode: impl Fn(&[u8]) -> Result<(), ProtocolError>,
    re_encode: impl FnOnce() -> Result<Vec<u8>, ProtocolError>,
    equals: impl FnOnce() -> bool,
) {
    validate().unwrap_or_else(|err| panic!("{label}: valid batch fails validation: {err:?}"));
    let length = encoded_len().unwrap_or_else(|err| panic!("{label}: encoded_len fails: {err:?}"));
    assert_eq!(
        length,
        payload.len(),
        "{label}: encoded_len disagrees with the reviewed payload"
    );

    let mut exact = vec![0u8; length];
    let written =
        encode_into(&mut exact).unwrap_or_else(|err| panic!("{label}: encode_into fails: {err:?}"));
    assert_eq!(
        written, length,
        "{label}: encode_into reports a short write"
    );
    assert_eq!(
        exact, payload,
        "{label}: encode_into published unexpected bytes"
    );

    let mut padded = vec![SENTINEL; length + 3];
    let written = encode_into(&mut padded)
        .unwrap_or_else(|err| panic!("{label}: padded encode_into fails: {err:?}"));
    assert_eq!(
        written, length,
        "{label}: padded write reports a short length"
    );
    assert_eq!(
        &padded[..length],
        payload,
        "{label}: padded prefix differs from the reviewed payload"
    );
    assert!(
        padded[length..].iter().all(|byte| *byte == SENTINEL),
        "{label}: padded write touched bytes beyond the record"
    );

    let allocated = encode().unwrap_or_else(|err| panic!("{label}: encode fails: {err:?}"));
    assert_eq!(
        allocated, payload,
        "{label}: encode disagrees with encode_into"
    );

    decode(payload).unwrap_or_else(|err| panic!("{label}: decode fails: {err:?}"));
    assert!(
        equals(),
        "{label}: decoded batch differs from the encoded one"
    );
    let reencoded = re_encode().unwrap_or_else(|err| panic!("{label}: re-encode fails: {err:?}"));
    assert_eq!(
        reencoded, payload,
        "{label}: re-encoded payload differs from the reviewed bytes"
    );
}

#[test]
fn projectile_spawn_round_trips_through_the_fallible_surface() {
    let spawn = canonical_spawn();
    let decoded = ProjectileSpawn::decode(&PROJECTILE_SPAWN_WIRE).expect("decode");
    assert_round_trip(
        "projectile spawn",
        &PROJECTILE_SPAWN_WIRE,
        || spawn.validate(),
        || spawn.encoded_len(),
        |dst| spawn.encode_into(dst),
        || spawn.encode(),
        |payload| ProjectileSpawn::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == spawn,
    );
    assert_eq!(
        PROJECTILE_SPAWN_WIRE.len(),
        8 + 1 + 2 * PROJECTILE_SPAWN_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.spawns.len(), 2);
    assert_eq!(decoded.spawns[0].id.get(), 1);
    assert_eq!(decoded.spawns[1].id.get(), 2);
    // The two records publish the two kinds across the two dimensions.
    assert_eq!(decoded.spawns[0].kind, PROJECTILE_KIND_SHARD);
    assert_eq!(decoded.spawns[0].dimension, Dimension::OVERWORLD);
    assert_eq!(decoded.spawns[1].kind, PROJECTILE_KIND_ARROW);
    assert_eq!(decoded.spawns[1].dimension, Dimension::DEPTHS);
}

#[test]
fn projectile_state_round_trips_through_the_fallible_surface() {
    let state = canonical_state();
    let decoded = ProjectileState::decode(&PROJECTILE_STATE_WIRE).expect("decode");
    assert_round_trip(
        "projectile state",
        &PROJECTILE_STATE_WIRE,
        || state.validate(),
        || state.encoded_len(),
        |dst| state.encode_into(dst),
        || state.encode(),
        |payload| ProjectileState::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == state,
    );
    assert_eq!(
        PROJECTILE_STATE_WIRE.len(),
        8 + 1 + 2 * PROJECTILE_STATE_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.states.len(), 2);
    assert_eq!(decoded.states[0].id.get(), 1);
    assert_eq!(decoded.states[1].id.get(), 2);
}

#[test]
fn projectile_despawn_round_trips_through_the_fallible_surface() {
    let despawn = canonical_despawn();
    let decoded = ProjectileDespawn::decode(&PROJECTILE_DESPAWN_WIRE).expect("decode");
    assert_round_trip(
        "projectile despawn",
        &PROJECTILE_DESPAWN_WIRE,
        || despawn.validate(),
        || despawn.encoded_len(),
        |dst| despawn.encode_into(dst),
        || despawn.encode(),
        |payload| ProjectileDespawn::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == despawn,
    );
    assert_eq!(
        PROJECTILE_DESPAWN_WIRE.len(),
        8 + 1 + 2 * PROJECTILE_DESPAWN_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.ids, vec![projectile_id(1), projectile_id(2)]);
}

#[test]
fn projectile_kind_and_dimension_are_independent_on_the_wire() {
    // All four combinations admit: a shard in the depths and an arrow in the
    // overworld are both publishable records at this boundary. The narrower
    // kind-by-dimension policy is an authority rule this codec deliberately
    // does not enforce, so no combination is refused here.
    for kind in [PROJECTILE_KIND_SHARD, PROJECTILE_KIND_ARROW] {
        for dimension in [Dimension::OVERWORLD, Dimension::DEPTHS] {
            let record = ProjectileSpawnRecord {
                kind,
                dimension,
                ..spawn_record_one()
            };
            assert!(
                ProjectileSpawn::new(0, vec![record]).is_ok(),
                "kind {kind} in dimension {dimension:?} is refused"
            );
            let wire = projectile_spawn_bytes(0, &[record]);
            assert!(
                ProjectileSpawn::decode(&wire).is_ok(),
                "kind {kind} in dimension {dimension:?} does not decode"
            );
        }
    }
}

#[test]
fn projectile_state_carries_only_identity_and_position() {
    // The state record is the narrowest record in the crate: the identity and
    // the position, with no kind, no dimension and no velocity, because all
    // three are fixed for the projectile's whole life and the mirror recorded
    // them at spawn.
    assert_eq!(PROJECTILE_STATE_WIRE_BYTES, 8 + 12);
    assert_eq!(
        PROJECTILE_SPAWN_WIRE_BYTES - PROJECTILE_STATE_WIRE_BYTES,
        1 + 4 + 12,
        "the state record is the spawn record minus kind, dimension and velocity"
    );
    assert_eq!(PROJECTILE_DESPAWN_WIRE_BYTES, 8);
    assert_eq!(PROJECTILE_STATE_WIRE.len(), 9 + 2 * 20);

    // The kind byte is absent, so the first position word follows the identity
    // immediately, five bytes earlier than in a spawn record, where the kind
    // byte and the dimension word precede it.
    assert_eq!(&PROJECTILE_STATE_WIRE[17..21], &[0x00, 0x00, 0x00, 0x80]);
    assert_eq!(&PROJECTILE_SPAWN_WIRE[22..26], &[0x00, 0x00, 0x00, 0x80]);
    // The state record ends at the fixed stride: the last word is the second
    // record's position Z, and no trailing field exists.
    assert_eq!(&PROJECTILE_STATE_WIRE[45..49], &[0x00, 0x00, 0xa0, 0x40]);
}

#[test]
fn projectile_kind_is_a_closed_match() {
    // The two published kinds are admitted and any other byte is refused at
    // the enum boundary, which is the category the Go validator publishes for
    // the same byte.
    for kind in [PROJECTILE_KIND_SHARD, PROJECTILE_KIND_ARROW] {
        let mut record = spawn_record_one();
        record.kind = kind;
        assert!(ProjectileSpawn::new(0, vec![record]).is_ok());
    }
    for kind in [2u8, 3, 0xff] {
        let mut record = spawn_record_one();
        record.kind = kind;
        assert_eq!(
            ProjectileSpawn::new(0, vec![record]),
            Err(ProtocolError::InvalidEnum),
            "kind {kind} is refused at the enum boundary"
        );
    }

    // The decode path answers at the same boundary: the kind is the byte after
    // the identity, before the dimension word.
    let mut kind_two = PROJECTILE_SPAWN_WIRE;
    kind_two[9 + 8] = 2;
    assert_eq!(
        ProjectileSpawn::decode(&kind_two),
        Err(ProtocolError::InvalidEnum)
    );
}

#[test]
fn projectile_records_preserve_negative_zero_pose_bits() {
    // A negative zero component survives the round trip in both records, so
    // neither pose nor velocity is normalized on either side.
    let spawn = canonical_spawn();
    assert_eq!(spawn.spawns[0].position[0].to_bits(), 0x8000_0000);
    assert_eq!(spawn.spawns[1].velocity[0].to_bits(), 0x8000_0000);
    let decoded = ProjectileSpawn::decode(&spawn.encode().expect("encode")).expect("decode");
    assert_eq!(decoded.spawns[0].position[0].to_bits(), 0x8000_0000);
    assert_eq!(decoded.spawns[1].velocity[0].to_bits(), 0x8000_0000);

    let state = canonical_state();
    assert_eq!(state.states[0].position[0].to_bits(), 0x8000_0000);
    let decoded = ProjectileState::decode(&state.encode().expect("encode")).expect("decode");
    assert_eq!(decoded.states[0].position[0].to_bits(), 0x8000_0000);

    // A non-finite component is refused where it is written and where it is
    // read, so neither side publishes a NaN or an infinity.
    let mut spawn_pose = projectile_spawn_bytes(0, &[spawn_record_one()]);
    // The first position word follows the identity, the kind and the dimension.
    spawn_pose[22..26].copy_from_slice(&f32::NAN.to_bits().to_le_bytes());
    assert_eq!(
        ProjectileSpawn::decode(&spawn_pose),
        Err(ProtocolError::InvalidFloat)
    );
    let mut spawn_velocity = projectile_spawn_bytes(0, &[spawn_record_one()]);
    // The first velocity word follows the position.
    spawn_velocity[34..38].copy_from_slice(&f32::INFINITY.to_bits().to_le_bytes());
    assert_eq!(
        ProjectileSpawn::decode(&spawn_velocity),
        Err(ProtocolError::InvalidFloat)
    );
    let mut state_pose = projectile_state_bytes(0, &[state_record_one()]);
    // The first position word follows the identity immediately.
    state_pose[17..21].copy_from_slice(&f32::NAN.to_bits().to_le_bytes());
    assert_eq!(
        ProjectileState::decode(&state_pose),
        Err(ProtocolError::InvalidFloat)
    );
    let mut non_finite = canonical_spawn();
    non_finite.spawns[0].velocity = [f32::NAN, 0.0, 0.0];
    assert_eq!(non_finite.encode(), Err(ProtocolError::InvalidFloat));
    let mut non_finite_state = canonical_state();
    non_finite_state.states[0].position = [f32::NAN, 1.0, 2.0];
    assert_eq!(non_finite_state.encode(), Err(ProtocolError::InvalidFloat));
}

#[test]
fn projectile_count_bound_fires_before_the_record_rule() {
    // A declared count outside 1..=128 is answered at the count bound even when
    // the payload cannot back it, which is the order the Go decoder applies:
    // the count message precedes its remaining-length check.
    for count in [0u8, 129, 0xff] {
        let mut short = PROJECTILE_SPAWN_WIRE.to_vec();
        short[8] = count;
        short.truncate(20);
        assert_eq!(
            ProjectileSpawn::decode(&short),
            Err(ProtocolError::InvalidRange),
            "spawn count {count} is refused at the count bound"
        );
        let mut short_state = PROJECTILE_STATE_WIRE.to_vec();
        short_state[8] = count;
        short_state.truncate(20);
        assert_eq!(
            ProjectileState::decode(&short_state),
            Err(ProtocolError::InvalidRange),
            "state count {count} is refused at the count bound"
        );
        let mut short_despawn = PROJECTILE_DESPAWN_WIRE.to_vec();
        short_despawn[8] = count;
        short_despawn.truncate(20);
        assert_eq!(
            ProjectileDespawn::decode(&short_despawn),
            Err(ProtocolError::InvalidRange),
            "despawn count {count} is refused at the count bound"
        );
        let mut full = PROJECTILE_SPAWN_WIRE;
        full[8] = count;
        assert_eq!(
            ProjectileSpawn::decode(&full),
            Err(ProtocolError::InvalidRange)
        );
    }

    // A declared count the payload cannot back is the truncation boundary,
    // because both sides apply the exact-remaining-length rule.
    let mut mismatched = PROJECTILE_DESPAWN_WIRE;
    mismatched[8] = 3;
    assert_eq!(
        ProjectileDespawn::decode(&mismatched),
        Err(ProtocolError::Truncated)
    );
    let mut mismatched_state = PROJECTILE_STATE_WIRE;
    mismatched_state[8] = 3;
    assert_eq!(
        ProjectileState::decode(&mismatched_state),
        Err(ProtocolError::Truncated)
    );
    let mut mismatched_spawn = PROJECTILE_SPAWN_WIRE;
    mismatched_spawn[8] = 3;
    assert_eq!(
        ProjectileSpawn::decode(&mismatched_spawn),
        Err(ProtocolError::Truncated)
    );

    // The constructors refuse the empty and the over-full batches.
    assert_eq!(
        ProjectileSpawn::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(
        ProjectileState::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(
        ProjectileDespawn::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    let over: Vec<ProjectileId> = (1..=MAX_PROJECTILE_RECORDS as u64 + 1)
        .map(projectile_id)
        .collect();
    assert_eq!(
        ProjectileDespawn::new(0, over),
        Err(ProtocolError::InvalidRange)
    );
}

#[test]
fn projectile_batches_admit_the_full_record_ceiling() {
    // One hundred twenty-eight records is the count and record ceiling of all
    // three families: each batch admits, and the exact length is the tick, the
    // one-byte count and the records.
    let spawn = full_spawn();
    assert_eq!(spawn.spawns.len(), MAX_PROJECTILE_RECORDS as usize);
    assert_eq!(
        spawn.encoded_len().expect("encoded_len"),
        8 + 1 + MAX_PROJECTILE_RECORDS as usize * PROJECTILE_SPAWN_WIRE_BYTES
    );
    let spawn_wire = spawn.encode().expect("encode the full spawn batch");
    assert_eq!(spawn_wire.len(), 9 + 128 * 37);
    assert_eq!(
        ProjectileSpawn::decode(&spawn_wire).expect("decode the full spawn batch"),
        spawn
    );
    // The one-hundred-twenty-ninth record is refused by the count bound.
    let mut over = spawn.clone();
    over.spawns.push(ProjectileSpawnRecord {
        id: projectile_id(129),
        ..spawn_record_one()
    });
    assert_eq!(over.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(over.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(over.encode(), Err(ProtocolError::InvalidRange));

    let state = full_state();
    assert_eq!(state.states.len(), MAX_PROJECTILE_RECORDS as usize);
    assert_eq!(
        state.encoded_len().expect("encoded_len"),
        8 + 1 + MAX_PROJECTILE_RECORDS as usize * PROJECTILE_STATE_WIRE_BYTES
    );
    let state_wire = state.encode().expect("encode the full state batch");
    assert_eq!(state_wire.len(), 9 + 128 * 20);
    assert_eq!(
        ProjectileState::decode(&state_wire).expect("decode the full state batch"),
        state
    );
    let mut over_state = state.clone();
    over_state.states.push(ProjectileStateRecord {
        id: projectile_id(129),
        ..state_record_one()
    });
    assert_eq!(over_state.validate(), Err(ProtocolError::InvalidRange));

    let despawn = full_despawn();
    assert_eq!(despawn.ids.len(), MAX_PROJECTILE_RECORDS as usize);
    assert_eq!(
        despawn.encoded_len().expect("encoded_len"),
        8 + 1 + MAX_PROJECTILE_RECORDS as usize * PROJECTILE_DESPAWN_WIRE_BYTES
    );
    let despawn_wire = despawn.encode().expect("encode the full despawn batch");
    assert_eq!(despawn_wire.len(), 9 + 128 * 8);
    assert_eq!(
        ProjectileDespawn::decode(&despawn_wire).expect("decode the full despawn batch"),
        despawn
    );
    let mut over_despawn = despawn.clone();
    over_despawn.ids.push(projectile_id(129));
    assert_eq!(over_despawn.validate(), Err(ProtocolError::InvalidRange));

    // The derived ceilings are exactly the full batch lengths.
    assert_eq!(PROJECTILE_SPAWN_MAX_WIRE_BYTES, 9 + 128 * 37);
    assert_eq!(PROJECTILE_STATE_MAX_WIRE_BYTES, 9 + 128 * 20);
    assert_eq!(PROJECTILE_DESPAWN_MAX_WIRE_BYTES, 9 + 128 * 8);
}

#[test]
fn projectile_records_are_strictly_ordered_by_identity() {
    // Duplicate and descending identity orders are refused by every batch, and
    // the decode path answers at the same rule.
    let duplicate_spawn = ProjectileSpawn::new(0, vec![spawn_record_one(), spawn_record_one()]);
    assert_eq!(
        duplicate_spawn,
        Err(ProtocolError::InvalidRange),
        "a duplicate identity is refused"
    );
    let reversed_spawn = ProjectileSpawn::new(0, vec![spawn_record_two(), spawn_record_one()]);
    assert_eq!(reversed_spawn, Err(ProtocolError::InvalidRange));

    let duplicate_state = ProjectileState::new(0, vec![state_record_one(), state_record_one()]);
    assert_eq!(duplicate_state, Err(ProtocolError::InvalidRange));
    let reversed_state = ProjectileState::new(0, vec![state_record_two(), state_record_one()]);
    assert_eq!(reversed_state, Err(ProtocolError::InvalidRange));

    let duplicate_despawn = ProjectileDespawn::new(0, vec![projectile_id(1), projectile_id(1)]);
    assert_eq!(duplicate_despawn, Err(ProtocolError::InvalidRange));
    let reversed_despawn = ProjectileDespawn::new(0, vec![projectile_id(2), projectile_id(1)]);
    assert_eq!(reversed_despawn, Err(ProtocolError::InvalidRange));

    let mut duplicate_wire = PROJECTILE_DESPAWN_WIRE;
    duplicate_wire[17..25].copy_from_slice(&1u64.to_le_bytes());
    assert_eq!(
        ProjectileDespawn::decode(&duplicate_wire),
        Err(ProtocolError::InvalidRange)
    );
    let reversed_wire = projectile_despawn_bytes(0, &[projectile_id(2), projectile_id(1)]);
    assert_eq!(
        ProjectileDespawn::decode(&reversed_wire),
        Err(ProtocolError::InvalidRange)
    );
    // A large identity pair proves the comparison is numeric rather than
    // lexicographic over the byte string.
    let wide = projectile_despawn_bytes(0, &[projectile_id(u64::MAX - 1), projectile_id(u64::MAX)]);
    assert_eq!(
        ProjectileDespawn::decode(&wide).map(|batch| batch.ids.len()),
        Ok(2)
    );
}

#[test]
fn projectile_spawn_dimension_is_matched_against_the_known_ids() {
    // The dimension is the raw wire `i32` matched against the two known IDs
    // rather than narrowed to a `u8`: both playable dimensions are admitted and
    // a foreign raw value is an enum violation instead of a reinterpreted
    // dimension.
    for (raw, expect_ok) in [
        (0i32, true),
        (1, true),
        (2, false),
        (256, false),
        (-1, false),
    ] {
        let mut wire = PROJECTILE_SPAWN_WIRE;
        wire[18..22].copy_from_slice(&raw.to_le_bytes());
        let decoded = ProjectileSpawn::decode(&wire);
        if expect_ok {
            assert!(decoded.is_ok(), "dimension {raw} is admitted");
        } else {
            assert_eq!(
                decoded,
                Err(ProtocolError::InvalidEnum),
                "dimension {raw} is refused at the enum boundary"
            );
        }
    }
}

#[test]
fn projectile_spawn_invalid_value_wins_over_short_capacity() {
    let mut over_kind = spawn_record_one();
    over_kind.kind = 2;
    let mut reversed = canonical_spawn();
    reversed.spawns.reverse();
    let mut duplicate = canonical_spawn();
    duplicate.spawns.push(spawn_record_one());
    let mut empty = canonical_spawn();
    empty.spawns.clear();

    let cases: Vec<(
        &str,
        ProtocolError,
        usize,
        Vec<Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>>,
    )> = vec![
        (
            "spawn kind is outside the closed set",
            ProtocolError::InvalidEnum,
            PROJECTILE_SPAWN_WIRE.len(),
            vec![Box::new(move |dst: &mut [u8]| {
                ProjectileSpawn {
                    server_tick: 0,
                    spawns: vec![over_kind],
                }
                .encode_into(dst)
            })],
        ),
        (
            "spawn records are descending",
            ProtocolError::InvalidRange,
            PROJECTILE_SPAWN_WIRE.len(),
            vec![Box::new(move |dst: &mut [u8]| reversed.encode_into(dst))],
        ),
        (
            "spawn batch carries a duplicate",
            ProtocolError::InvalidRange,
            8 + 1 + 3 * PROJECTILE_SPAWN_WIRE_BYTES,
            vec![Box::new(move |dst: &mut [u8]| duplicate.encode_into(dst))],
        ),
        (
            "spawn batch is empty",
            ProtocolError::InvalidRange,
            8 + 1,
            vec![Box::new(move |dst: &mut [u8]| empty.encode_into(dst))],
        ),
    ];

    for (label, error, needed, encoders) in cases {
        for encode_into in &encoders {
            for available in [0usize, needed - 1, needed] {
                let mut dst = vec![SENTINEL; available];
                assert_eq!(
                    encode_into(&mut dst),
                    Err(error),
                    "{label}: a destination of {available} bytes masks the value error"
                );
                assert!(
                    dst.iter().all(|byte| *byte == SENTINEL),
                    "{label}: a destination of {available} bytes was modified on a value refusal"
                );
            }
            let mut exact = vec![0u8; needed];
            assert_eq!(
                encode_into(&mut exact),
                Err(error),
                "{label}: the exact destination still refuses the value error"
            );
        }
    }
}

#[test]
fn projectile_state_invalid_value_wins_over_short_capacity() {
    let mut non_finite = state_record_one();
    non_finite.position = [f32::NAN, 1.0, 2.0];
    let mut duplicate = canonical_state();
    duplicate.states.push(state_record_one());
    let mut reversed = canonical_state();
    reversed.states.reverse();
    let mut empty = canonical_state();
    empty.states.clear();

    let cases: Vec<(
        &str,
        ProtocolError,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "state position is not finite",
            ProtocolError::InvalidFloat,
            PROJECTILE_STATE_WIRE.len(),
            Box::new(move |dst: &mut [u8]| {
                let mut batch = canonical_state();
                batch.states[0] = non_finite;
                batch.encode_into(dst)
            }),
        ),
        (
            "state batch carries a duplicate",
            ProtocolError::InvalidRange,
            8 + 1 + 3 * PROJECTILE_STATE_WIRE_BYTES,
            Box::new(move |dst: &mut [u8]| duplicate.encode_into(dst)),
        ),
        (
            "state batch is descending",
            ProtocolError::InvalidRange,
            8 + 1 + 2 * PROJECTILE_STATE_WIRE_BYTES,
            Box::new(move |dst: &mut [u8]| reversed.encode_into(dst)),
        ),
        (
            "state batch is empty",
            ProtocolError::InvalidRange,
            8 + 1,
            Box::new(move |dst: &mut [u8]| empty.encode_into(dst)),
        ),
    ];

    for (label, error, needed, encode_into) in cases {
        for available in [0usize, needed - 1, needed] {
            let mut dst = vec![SENTINEL; available];
            assert_eq!(
                encode_into(&mut dst),
                Err(error),
                "{label}: a destination of {available} bytes masks the value error"
            );
            assert!(
                dst.iter().all(|byte| *byte == SENTINEL),
                "{label}: a destination of {available} bytes was modified on a value refusal"
            );
        }
        let mut exact = vec![0u8; needed];
        assert_eq!(
            encode_into(&mut exact),
            Err(error),
            "{label}: the exact destination still refuses the value error"
        );
    }
}

#[test]
fn projectile_despawn_invalid_value_wins_over_short_capacity() {
    let mut duplicate = canonical_despawn();
    duplicate.ids.push(projectile_id(2));
    let mut reversed = canonical_despawn();
    reversed.ids.reverse();
    let mut empty = canonical_despawn();
    empty.ids.clear();

    let cases: Vec<(
        &str,
        ProtocolError,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "despawn batch carries a duplicate",
            ProtocolError::InvalidRange,
            8 + 1 + 3 * PROJECTILE_DESPAWN_WIRE_BYTES,
            Box::new(move |dst: &mut [u8]| duplicate.encode_into(dst)),
        ),
        (
            "despawn batch is descending",
            ProtocolError::InvalidRange,
            8 + 1 + 2 * PROJECTILE_DESPAWN_WIRE_BYTES,
            Box::new(move |dst: &mut [u8]| reversed.encode_into(dst)),
        ),
        (
            "despawn batch is empty",
            ProtocolError::InvalidRange,
            8 + 1,
            Box::new(move |dst: &mut [u8]| empty.encode_into(dst)),
        ),
    ];

    for (label, error, needed, encode_into) in cases {
        for available in [0usize, needed - 1, needed] {
            let mut dst = vec![SENTINEL; available];
            assert_eq!(
                encode_into(&mut dst),
                Err(error),
                "{label}: a destination of {available} bytes masks the value error"
            );
            assert!(
                dst.iter().all(|byte| *byte == SENTINEL),
                "{label}: a destination of {available} bytes was modified on a value refusal"
            );
        }
        let mut exact = vec![0u8; needed];
        assert_eq!(
            encode_into(&mut exact),
            Err(error),
            "{label}: the exact destination still refuses the value error"
        );
    }
}

/// The silent-encode red this node closes, quoted from the previous surface.
///
/// A batch whose fields were public and mutable with no value gate on the
/// encode path published whatever the caller wrote: the previous infallible
/// `encode` trusted the constructor, so a spawn record mutated into kind 2, a
/// zero identity or an unordered pair after construction was published byte for
/// byte. The kind-two record published as
/// `[00, 00, 00, 00, 00, 00, 00, 00, 01, 01, 00, 00, 00, 00, 00, 00, 00, 02,
/// 00, 00, 00, 00, 00, 00, 80, 3f, 00, 00, 00, 40, 00, 00, 40, 40, 00, 00, 00,
/// 3f, 00, 00, 00, 00, 00, 00, 00, 00]` — the kind byte `0x02` at offset 17 —
/// the zero-identity spawn published `00` in the identity word, the duplicate
/// pair published the same identity twice, and the zero-identity despawn
/// published `0` where the identity belongs. A non-finite component reached the
/// primitive's own refusal, which the previous surface turned into a panic at
/// its `expect`. The fallible surface turns every one of them into a value
/// error before any capacity decision.
#[test]
fn projectile_mutated_public_fields_are_never_published() {
    let mut kind_two = canonical_spawn();
    kind_two.spawns[0].kind = 2;
    assert_eq!(kind_two.validate(), Err(ProtocolError::InvalidEnum));
    assert_eq!(kind_two.encoded_len(), Err(ProtocolError::InvalidEnum));
    assert_eq!(kind_two.encode(), Err(ProtocolError::InvalidEnum));

    let mut non_finite = canonical_spawn();
    non_finite.spawns[0].velocity = [f32::NAN, 0.0, 0.0];
    assert_eq!(non_finite.validate(), Err(ProtocolError::InvalidFloat));
    assert_eq!(non_finite.encoded_len(), Err(ProtocolError::InvalidFloat));
    assert_eq!(non_finite.encode(), Err(ProtocolError::InvalidFloat));

    let mut reversed = canonical_spawn();
    reversed.spawns.reverse();
    assert_eq!(reversed.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(reversed.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(reversed.encode(), Err(ProtocolError::InvalidRange));

    let mut duplicate = canonical_state();
    duplicate.states.push(state_record_one());
    assert_eq!(duplicate.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(duplicate.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(duplicate.encode(), Err(ProtocolError::InvalidRange));

    let mut non_finite_state = canonical_state();
    non_finite_state.states[0].position = [f32::NAN, 1.0, 2.0];
    assert_eq!(non_finite_state.encode(), Err(ProtocolError::InvalidFloat));

    let mut over = full_despawn();
    over.ids.push(projectile_id(129));
    assert_eq!(over.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(over.encode(), Err(ProtocolError::InvalidRange));

    let mut empty = canonical_despawn();
    empty.ids.clear();
    assert_eq!(empty.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(empty.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(empty.encode(), Err(ProtocolError::InvalidRange));
}

#[test]
fn projectile_decode_rejects_every_proper_truncation() {
    // All three canonical payloads are short enough for a full sweep, and every
    // cut answers at the truncation boundary: the tick, the one-byte count
    // prefix and the record region all report the same failure the Go decoder
    // reports for the same bytes.
    for cut in 0..PROJECTILE_SPAWN_WIRE.len() {
        let error = ProjectileSpawn::decode(&PROJECTILE_SPAWN_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("spawn: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "spawn: truncation at {cut} bytes reports {error:?}"
        );
    }
    for cut in 0..PROJECTILE_STATE_WIRE.len() {
        let error = ProjectileState::decode(&PROJECTILE_STATE_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("state: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "state: truncation at {cut} bytes reports {error:?}"
        );
    }
    for cut in 0..PROJECTILE_DESPAWN_WIRE.len() {
        let error = ProjectileDespawn::decode(&PROJECTILE_DESPAWN_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("despawn: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "despawn: truncation at {cut} bytes reports {error:?}"
        );
    }

    // The full batches are swept inside their record regions rather than byte
    // by byte, and the cut inside a record reports the same boundary.
    let spawn = full_spawn().encode().expect("encode the full spawn batch");
    for cut in [10usize, 9 + 37, 9 + 2 * 37 - 1, 9 + 128 * 37 - 1] {
        let error = ProjectileSpawn::decode(&spawn[..cut])
            .err()
            .unwrap_or_else(|| panic!("full spawn: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "full spawn: truncation at {cut} bytes reports {error:?}"
        );
    }
    let state = full_state().encode().expect("encode the full state batch");
    for cut in [10usize, 9 + 20, 9 + 2 * 20 - 1, 9 + 128 * 20 - 1] {
        let error = ProjectileState::decode(&state[..cut])
            .err()
            .unwrap_or_else(|| panic!("full state: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "full state: truncation at {cut} bytes reports {error:?}"
        );
    }
}

#[test]
fn projectile_decode_rejects_one_trailing_byte() {
    // All three families apply the exact-remaining-length rule, so a one-byte
    // extra payload is answered by the length check before any record is read.
    // The Go decoder applies the same rule — its remaining-length message
    // precedes the record loop — so a padded payload reports the truncation
    // boundary on both sides rather than a trailing byte.
    let mut spawn = PROJECTILE_SPAWN_WIRE.to_vec();
    spawn.push(0x00);
    assert_eq!(
        ProjectileSpawn::decode(&spawn),
        Err(ProtocolError::Truncated),
        "a padded spawn payload reports the truncation boundary"
    );
    let mut state = PROJECTILE_STATE_WIRE.to_vec();
    state.push(0x00);
    assert_eq!(
        ProjectileState::decode(&state),
        Err(ProtocolError::Truncated),
        "a padded state payload reports the truncation boundary"
    );
    let mut despawn = PROJECTILE_DESPAWN_WIRE.to_vec();
    despawn.push(0x00);
    assert_eq!(
        ProjectileDespawn::decode(&despawn),
        Err(ProtocolError::Truncated),
        "a padded despawn payload reports the truncation boundary"
    );
}

#[test]
fn projectile_decode_refuses_an_over_ceiling_payload_before_any_read() {
    // The Go decode path applies the family's fixed payload maximum before the
    // family decoder runs, so these three decoders apply the same ceiling as a
    // pre-parse size check: one byte above the derived ceiling is refused at
    // the capacity boundary before the count is read, which is what makes the
    // over-ceiling corpus case publish one category on both sides.
    let mut spawn = full_spawn().encode().expect("encode the full spawn batch");
    assert_eq!(spawn.len(), PROJECTILE_SPAWN_MAX_WIRE_BYTES);
    spawn.push(0x00);
    assert_eq!(
        ProjectileSpawn::decode(&spawn),
        Err(ProtocolError::FrameTooLarge),
        "an over-ceiling spawn payload is refused at the capacity boundary"
    );

    let mut state = full_state().encode().expect("encode the full state batch");
    assert_eq!(state.len(), PROJECTILE_STATE_MAX_WIRE_BYTES);
    state.push(0x00);
    assert_eq!(
        ProjectileState::decode(&state),
        Err(ProtocolError::FrameTooLarge),
        "an over-ceiling state payload is refused at the capacity boundary"
    );

    let mut despawn = full_despawn()
        .encode()
        .expect("encode the full despawn batch");
    assert_eq!(despawn.len(), PROJECTILE_DESPAWN_MAX_WIRE_BYTES);
    despawn.push(0x00);
    assert_eq!(
        ProjectileDespawn::decode(&despawn),
        Err(ProtocolError::FrameTooLarge),
        "an over-ceiling despawn payload is refused at the capacity boundary"
    );

    // The ceiling fires before every other rule, including the count bound: a
    // payload that is simultaneously over the ceiling and declares a count the
    // batch cannot back still answers at the capacity boundary, exactly as the
    // Go decoder's fixed maximum precedes its family decoder.
    let mut mutilated = spawn.clone();
    mutilated[8] = 0;
    assert_eq!(
        ProjectileSpawn::decode(&mutilated),
        Err(ProtocolError::FrameTooLarge)
    );
}

#[test]
fn projectile_encode_into_leaves_a_short_destination_unchanged() {
    let cases: Vec<(
        &str,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "projectile spawn",
            PROJECTILE_SPAWN_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_spawn().encode_into(dst)),
        ),
        (
            "projectile spawn at the ceiling",
            PROJECTILE_SPAWN_MAX_WIRE_BYTES,
            Box::new(|dst: &mut [u8]| full_spawn().encode_into(dst)),
        ),
        (
            "projectile state",
            PROJECTILE_STATE_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_state().encode_into(dst)),
        ),
        (
            "projectile state at the ceiling",
            PROJECTILE_STATE_MAX_WIRE_BYTES,
            Box::new(|dst: &mut [u8]| full_state().encode_into(dst)),
        ),
        (
            "projectile despawn",
            PROJECTILE_DESPAWN_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_despawn().encode_into(dst)),
        ),
        (
            "projectile despawn at the ceiling",
            PROJECTILE_DESPAWN_MAX_WIRE_BYTES,
            Box::new(|dst: &mut [u8]| full_despawn().encode_into(dst)),
        ),
    ];
    for (label, length, encode_into) in cases {
        for available in [0usize, length - 1, length - 3] {
            let mut dst = vec![SENTINEL; available];
            let error = encode_into(&mut dst).expect_err("a short destination must be refused");
            assert_eq!(
                error,
                ProtocolError::OutputTooSmall {
                    needed: length,
                    available
                },
                "{label}: short destination of {available} bytes reports the wrong error"
            );
            assert!(
                dst.iter().all(|byte| *byte == SENTINEL),
                "{label}: short destination of {available} bytes was modified"
            );
        }
    }
}

#[test]
fn projectile_packet_ids_are_pinned() {
    assert_eq!(ProjectileSpawn::PACKET_ID, 29);
    assert_eq!(ProjectileState::PACKET_ID, 30);
    assert_eq!(ProjectileDespawn::PACKET_ID, 31);
}
