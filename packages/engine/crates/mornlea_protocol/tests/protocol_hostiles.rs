//! The three hostile-mob publication families: the common fallible surface,
//! the closed kind rule, and the exact-record batch rule.
//!
//! `HostileSpawn` (S/Play/22), `HostileState` (S/Play/23) and
//! `HostileDespawn` (S/Play/24) publish the hostile bodies one subscriber can
//! see. All three carry a `u64` server tick and a one-byte record count, and
//! all three admit exactly `1..=MAX_HOSTILE_RECORDS` records ordered by the
//! checked domain `HostileId`, which is a nonzero `u64` newtype: a zero ID is
//! the absent form, so it cannot be constructed on the outbound surface and is
//! refused at the identity boundary where it is read. No AI state — target,
//! cooldown, path or capacity — is on the wire, because those stay F2 concerns
//! of the authority.
//!
//! The three records differ in exactly one field each, and the difference is
//! the Go one: a spawn record carries the dimension and omits the velocity,
//! because a dimension change always goes through a despawn/spawn pair, while a
//! state record carries the velocity and omits the dimension, because the
//! mirror already holds it from the spawn. The fixed strides are therefore 30
//! and 38 bytes, and the despawn record is the bare 8-byte identity.
//!
//! The batch length rule is the exact-remaining-length rule on both sides: the
//! Go decoder rejects a payload whose remaining length is not exactly `count`
//! records before it reads one, so a short and a padded payload answer at the
//! same truncation boundary. This differs from the item drop batch, whose Go
//! decoder accepts a long payload and answers the remainder as trailing bytes.
//!
//! The corpus evidence these families publish is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.

use mornlea_domain::HostileId;
use mornlea_protocol::{
    HOSTILE_DESPAWN_WIRE_BYTES, HOSTILE_KIND_BONE_THROWER, HOSTILE_KIND_NIGHTWALKER,
    HOSTILE_SPAWN_MAX_RECORDS, HOSTILE_SPAWN_WIRE_BYTES, HOSTILE_STATE_MAX_RECORDS,
    HOSTILE_STATE_WIRE_BYTES, HostileDespawn, HostileSpawn, HostileSpawnRecord, HostileState,
    HostileStateRecord, MAX_HOSTILE_RECORDS, ProtocolError,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The reviewed 69-byte HostileSpawn literal: tick 0, two records strictly
/// ordered by ID, the overworld dimension, the health boundaries 1 and 20, and
/// the two kinds. The first position carries a negative zero.
const HOSTILE_SPAWN_WIRE: [u8; 69] = [
    // tick 0 and the declared count of two records
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
    // first record: ID 1, dimension 0, position (-0.0, 1.0, 2.0), yaw 0.0,
    // health 1, kind 0
    0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80,
    0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
    // second record: ID 2, dimension 0, position (3.0, 4.0, 5.0), yaw -2.5,
    // health 20, kind 1
    0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40,
    0x00, 0x00, 0x80, 0x40, 0x00, 0x00, 0xa0, 0x40, 0x00, 0x00, 0x20, 0xc0, 0x14, 0x01,
];

/// The reviewed 85-byte HostileState literal: tick 0, two records strictly
/// ordered by ID, finite velocities, the health values 13 and 7, and the two
/// kinds. No dimension byte is present, so the record stride is 38.
const HOSTILE_STATE_WIRE: [u8; 85] = [
    // tick 0 and the declared count of two records
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
    // first record: ID 1, position (-0.0, 1.0, 2.0), velocity (-0.0, 0.25, 3.0),
    // yaw 0.5, health 13, kind 1
    0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f,
    0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3e, 0x00, 0x00, 0x40, 0x40,
    0x00, 0x00, 0x00, 0x3f, 0x0d, 0x01,
    // second record: ID 2, position (3.0, 4.0, 5.0), velocity (0.5, -1.25, 0.0),
    // yaw -2.5, health 7, kind 0
    0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x80, 0x40,
    0x00, 0x00, 0xa0, 0x40, 0x00, 0x00, 0x00, 0x3f, 0x00, 0x00, 0xa0, 0xbf, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x20, 0xc0, 0x07, 0x00,
];

/// The reviewed 25-byte HostileDespawn literal: tick 0 and the two strictly
/// ordered identities.
const HOSTILE_DESPAWN_WIRE: [u8; 25] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
];

/// Wraps one nonzero hostile identity, which the domain rule already admits.
fn hostile_id(id: u64) -> HostileId {
    HostileId::try_new(id).expect("reviewed nonzero hostile identity")
}

/// The first reviewed spawn record.
fn spawn_record_one() -> HostileSpawnRecord {
    HostileSpawnRecord {
        id: hostile_id(1),
        dimension: mornlea_domain::Dimension::OVERWORLD,
        position: [-0.0, 1.0, 2.0],
        yaw: 0.0,
        health: 1,
        kind: HOSTILE_KIND_NIGHTWALKER,
    }
}

/// The second reviewed spawn record: the health and kind upper boundaries.
fn spawn_record_two() -> HostileSpawnRecord {
    HostileSpawnRecord {
        id: hostile_id(2),
        dimension: mornlea_domain::Dimension::OVERWORLD,
        position: [3.0, 4.0, 5.0],
        yaw: -2.5,
        health: 20,
        kind: HOSTILE_KIND_BONE_THROWER,
    }
}

/// The canonical two-record spawn batch the reviewed literal carries.
fn canonical_spawn() -> HostileSpawn {
    HostileSpawn::new(0, vec![spawn_record_one(), spawn_record_two()])
        .expect("the canonical batch is valid")
}

/// The first reviewed state record, whose velocity carries a negative zero.
fn state_record_one() -> HostileStateRecord {
    HostileStateRecord {
        id: hostile_id(1),
        position: [-0.0, 1.0, 2.0],
        velocity: [-0.0, 0.25, 3.0],
        yaw: 0.5,
        health: 13,
        kind: HOSTILE_KIND_BONE_THROWER,
    }
}

/// The second reviewed state record.
fn state_record_two() -> HostileStateRecord {
    HostileStateRecord {
        id: hostile_id(2),
        position: [3.0, 4.0, 5.0],
        velocity: [0.5, -1.25, 0.0],
        yaw: -2.5,
        health: 7,
        kind: HOSTILE_KIND_NIGHTWALKER,
    }
}

/// The canonical two-record state batch the reviewed literal carries.
fn canonical_state() -> HostileState {
    HostileState::new(0, vec![state_record_one(), state_record_two()])
        .expect("the canonical batch is valid")
}

/// The canonical despawn batch the reviewed literal carries.
fn canonical_despawn() -> HostileDespawn {
    HostileDespawn::new(0, vec![hostile_id(1), hostile_id(2)])
        .expect("the canonical batch is valid")
}

/// The full 64-record spawn batch, the fixed record ceiling, with ascending
/// identities on every entry point.
fn full_spawn() -> HostileSpawn {
    let spawns: Vec<HostileSpawnRecord> = (1..=HOSTILE_SPAWN_MAX_RECORDS as u64)
        .map(|id| HostileSpawnRecord {
            id: hostile_id(id),
            ..spawn_record_one()
        })
        .collect();
    HostileSpawn::new(0, spawns).expect("the full batch is valid")
}

/// The full 64-record state batch, the fixed record ceiling.
fn full_state() -> HostileState {
    let states: Vec<HostileStateRecord> = (1..=HOSTILE_STATE_MAX_RECORDS as u64)
        .map(|id| HostileStateRecord {
            id: hostile_id(id),
            ..state_record_one()
        })
        .collect();
    HostileState::new(0, states).expect("the full batch is valid")
}

/// The full 64-record despawn batch, the fixed record ceiling.
fn full_despawn() -> HostileDespawn {
    let ids: Vec<HostileId> = (1..=MAX_HOSTILE_RECORDS as u64).map(hostile_id).collect();
    HostileDespawn::new(0, ids).expect("the full batch is valid")
}

/// Renders one spawn payload: the tick, the one-byte count and the records.
fn hostile_spawn_bytes(tick: u64, records: &[HostileSpawnRecord]) -> Vec<u8> {
    let mut wire = Vec::with_capacity(9 + records.len() * HOSTILE_SPAWN_WIRE_BYTES);
    wire.extend_from_slice(&tick.to_le_bytes());
    wire.push(records.len() as u8);
    for record in records {
        wire.extend_from_slice(&record.id.get().to_le_bytes());
        wire.extend_from_slice(&i32::from(record.dimension.get()).to_le_bytes());
        for value in record.position {
            wire.extend_from_slice(&value.to_bits().to_le_bytes());
        }
        wire.extend_from_slice(&record.yaw.to_bits().to_le_bytes());
        wire.push(record.health);
        wire.push(record.kind);
    }
    wire
}

/// Renders one state payload: the tick, the one-byte count and the records.
fn hostile_state_bytes(tick: u64, records: &[HostileStateRecord]) -> Vec<u8> {
    let mut wire = Vec::with_capacity(9 + records.len() * HOSTILE_STATE_WIRE_BYTES);
    wire.extend_from_slice(&tick.to_le_bytes());
    wire.push(records.len() as u8);
    for record in records {
        wire.extend_from_slice(&record.id.get().to_le_bytes());
        for value in record.position {
            wire.extend_from_slice(&value.to_bits().to_le_bytes());
        }
        for value in record.velocity {
            wire.extend_from_slice(&value.to_bits().to_le_bytes());
        }
        wire.extend_from_slice(&record.yaw.to_bits().to_le_bytes());
        wire.push(record.health);
        wire.push(record.kind);
    }
    wire
}

/// Renders one despawn payload: the tick, the one-byte count and the identities.
fn hostile_despawn_bytes(tick: u64, ids: &[HostileId]) -> Vec<u8> {
    let mut wire = Vec::with_capacity(9 + ids.len() * HOSTILE_DESPAWN_WIRE_BYTES);
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
fn hostile_spawn_round_trips_through_the_fallible_surface() {
    let spawn = canonical_spawn();
    let decoded = HostileSpawn::decode(&HOSTILE_SPAWN_WIRE).expect("decode");
    assert_round_trip(
        "hostile spawn",
        &HOSTILE_SPAWN_WIRE,
        || spawn.validate(),
        || spawn.encoded_len(),
        |dst| spawn.encode_into(dst),
        || spawn.encode(),
        |payload| HostileSpawn::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == spawn,
    );
    assert_eq!(
        HOSTILE_SPAWN_WIRE.len(),
        8 + 1 + 2 * HOSTILE_SPAWN_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.spawns.len(), 2);
    assert_eq!(decoded.spawns[0].id.get(), 1);
    assert_eq!(decoded.spawns[1].id.get(), 2);
    // Both records name the overworld, and the health and kind boundaries 1
    // and 20 / 0 and 1 survive the round trip.
    for record in &decoded.spawns {
        assert_eq!(record.dimension, mornlea_domain::Dimension::OVERWORLD);
    }
    assert_eq!(decoded.spawns[0].health, 1);
    assert_eq!(decoded.spawns[0].kind, HOSTILE_KIND_NIGHTWALKER);
    assert_eq!(decoded.spawns[1].health, 20);
    assert_eq!(decoded.spawns[1].kind, HOSTILE_KIND_BONE_THROWER);
}

#[test]
fn hostile_state_round_trips_through_the_fallible_surface() {
    let state = canonical_state();
    let decoded = HostileState::decode(&HOSTILE_STATE_WIRE).expect("decode");
    assert_round_trip(
        "hostile state",
        &HOSTILE_STATE_WIRE,
        || state.validate(),
        || state.encoded_len(),
        |dst| state.encode_into(dst),
        || state.encode(),
        |payload| HostileState::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == state,
    );
    assert_eq!(
        HOSTILE_STATE_WIRE.len(),
        8 + 1 + 2 * HOSTILE_STATE_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.states.len(), 2);
    assert_eq!(decoded.states[0].id.get(), 1);
    assert_eq!(decoded.states[1].id.get(), 2);
    assert_eq!(decoded.states[0].health, 13);
    assert_eq!(decoded.states[0].kind, HOSTILE_KIND_BONE_THROWER);
    assert_eq!(decoded.states[1].health, 7);
    assert_eq!(decoded.states[1].kind, HOSTILE_KIND_NIGHTWALKER);
}

#[test]
fn hostile_despawn_round_trips_through_the_fallible_surface() {
    let despawn = canonical_despawn();
    let decoded = HostileDespawn::decode(&HOSTILE_DESPAWN_WIRE).expect("decode");
    assert_round_trip(
        "hostile despawn",
        &HOSTILE_DESPAWN_WIRE,
        || despawn.validate(),
        || despawn.encoded_len(),
        |dst| despawn.encode_into(dst),
        || despawn.encode(),
        |payload| HostileDespawn::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == despawn,
    );
    assert_eq!(
        HOSTILE_DESPAWN_WIRE.len(),
        8 + 1 + 2 * HOSTILE_DESPAWN_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.ids, vec![hostile_id(1), hostile_id(2)]);
}

#[test]
fn hostile_state_omits_dimension_and_spawn_omits_velocity() {
    // The two records differ by exactly one field, and the difference is the
    // Go one: a dimension change goes through a despawn/spawn pair, so the
    // state record carries the velocity the spawn record never carries.
    assert_eq!(HOSTILE_SPAWN_WIRE_BYTES, 8 + 4 + 12 + 4 + 1 + 1);
    assert_eq!(HOSTILE_STATE_WIRE_BYTES, 8 + 12 + 12 + 4 + 1 + 1);
    assert_eq!(
        HOSTILE_STATE_WIRE_BYTES - HOSTILE_SPAWN_WIRE_BYTES,
        8,
        "the state record is the spawn record minus dimension plus velocity"
    );
    assert_eq!(HOSTILE_DESPAWN_WIRE_BYTES, 8);

    // The spawn's dimension is the four bytes after the identity, and no
    // velocity word exists: the record ends at the fixed 30-byte stride.
    assert_eq!(&HOSTILE_SPAWN_WIRE[17..21], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(HOSTILE_SPAWN_WIRE.len(), 9 + 2 * 30);
    // The state record begins its pose immediately after the identity, so the
    // first position word is the same negative zero the spawn carries.
    assert_eq!(&HOSTILE_STATE_WIRE[17..21], &[0x00, 0x00, 0x00, 0x80]);
    assert_eq!(HOSTILE_STATE_WIRE.len(), 9 + 2 * 38);
}

#[test]
fn hostile_kind_is_a_closed_match() {
    // The two published kinds are admitted on both record shapes and any other
    // byte is refused at the enum boundary, which is the category the Go
    // validator publishes for the same byte.
    for kind in [HOSTILE_KIND_NIGHTWALKER, HOSTILE_KIND_BONE_THROWER] {
        let mut spawn_record = spawn_record_one();
        spawn_record.kind = kind;
        assert!(HostileSpawn::new(0, vec![spawn_record]).is_ok());
        let mut state_record = state_record_one();
        state_record.kind = kind;
        assert!(HostileState::new(0, vec![state_record]).is_ok());
    }
    for kind in [2u8, 3, 0xff] {
        let mut spawn_record = spawn_record_one();
        spawn_record.kind = kind;
        assert_eq!(
            HostileSpawn::new(0, vec![spawn_record]),
            Err(ProtocolError::InvalidEnum),
            "kind {kind} is refused at the enum boundary"
        );
        let mut state_record = state_record_one();
        state_record.kind = kind;
        assert_eq!(
            HostileState::new(0, vec![state_record]),
            Err(ProtocolError::InvalidEnum)
        );
    }

    // The decode path answers at the same boundary: the kind is the last byte
    // of each record.
    let mut spawn_kind_two = HOSTILE_SPAWN_WIRE;
    spawn_kind_two[9 + 30 - 1] = 2;
    assert_eq!(
        HostileSpawn::decode(&spawn_kind_two),
        Err(ProtocolError::InvalidEnum)
    );
    let mut state_kind_two = HOSTILE_STATE_WIRE;
    state_kind_two[9 + 38 - 1] = 2;
    assert_eq!(
        HostileState::decode(&state_kind_two),
        Err(ProtocolError::InvalidEnum)
    );
}

#[test]
fn hostile_health_span_is_one_to_twenty() {
    // The Go `core.MaxHealth` span is inclusive at both ends, and the two
    // outside values are refused on every entry point.
    for health in [1u8, 20] {
        let mut spawn_record = spawn_record_one();
        spawn_record.health = health;
        assert!(HostileSpawn::new(0, vec![spawn_record]).is_ok());
        let mut state_record = state_record_one();
        state_record.health = health;
        assert!(HostileState::new(0, vec![state_record]).is_ok());
    }
    for health in [0u8, 21, 0xff] {
        let mut spawn_record = spawn_record_one();
        spawn_record.health = health;
        assert_eq!(
            HostileSpawn::new(0, vec![spawn_record]),
            Err(ProtocolError::InvalidRange)
        );
        let mut state_record = state_record_one();
        state_record.health = health;
        assert_eq!(
            HostileState::new(0, vec![state_record]),
            Err(ProtocolError::InvalidRange)
        );
    }

    // The decode path answers at the same boundary: the health byte precedes
    // the kind byte in both records.
    let mut spawn_health_zero = HOSTILE_SPAWN_WIRE;
    spawn_health_zero[9 + 30 - 2] = 0;
    assert_eq!(
        HostileSpawn::decode(&spawn_health_zero),
        Err(ProtocolError::InvalidRange)
    );
    let mut state_health_above = HOSTILE_STATE_WIRE;
    state_health_above[9 + 38 - 2] = 21;
    assert_eq!(
        HostileState::decode(&state_health_above),
        Err(ProtocolError::InvalidRange)
    );
}

#[test]
fn hostile_records_preserve_negative_zero_pose_bits() {
    // A negative zero component survives the round trip in both records, so
    // the pose is never normalized on either side.
    let spawn = canonical_spawn();
    assert_eq!(spawn.spawns[0].position[0].to_bits(), 0x8000_0000);
    let decoded = HostileSpawn::decode(&spawn.encode().expect("encode")).expect("decode");
    assert_eq!(decoded.spawns[0].position[0].to_bits(), 0x8000_0000);

    let state = canonical_state();
    assert_eq!(state.states[0].position[0].to_bits(), 0x8000_0000);
    assert_eq!(state.states[0].velocity[0].to_bits(), 0x8000_0000);
    let decoded = HostileState::decode(&state.encode().expect("encode")).expect("decode");
    assert_eq!(decoded.states[0].position[0].to_bits(), 0x8000_0000);
    assert_eq!(decoded.states[0].velocity[0].to_bits(), 0x8000_0000);

    // A non-finite component is refused where it is written and where it is
    // read, so neither side publishes a NaN or an infinity.
    let mut spawn_pose = hostile_spawn_bytes(0, &[spawn_record_one()]);
    // The first position word follows the identity and the dimension.
    spawn_pose[21..25].copy_from_slice(&f32::NAN.to_bits().to_le_bytes());
    assert_eq!(
        HostileSpawn::decode(&spawn_pose),
        Err(ProtocolError::InvalidFloat)
    );
    let mut state_pose = hostile_state_bytes(0, &[state_record_one()]);
    // The first velocity word follows the identity and the position.
    state_pose[29..33].copy_from_slice(&f32::INFINITY.to_bits().to_le_bytes());
    assert_eq!(
        HostileState::decode(&state_pose),
        Err(ProtocolError::InvalidFloat)
    );
    let mut non_finite = canonical_state();
    non_finite.states[0].velocity = [f32::NAN, 0.25, 3.0];
    assert_eq!(non_finite.encode(), Err(ProtocolError::InvalidFloat));
}

#[test]
fn hostile_count_bound_fires_before_the_record_rule() {
    // A declared count outside 1..=64 is answered at the count bound even when
    // the payload cannot back it, which is the order the Go decoder applies:
    // the count message precedes its remaining-length check.
    for count in [0u8, 65, 0xff] {
        let mut short = HOSTILE_SPAWN_WIRE.to_vec();
        short[8] = count;
        short.truncate(20);
        assert_eq!(
            HostileSpawn::decode(&short),
            Err(ProtocolError::InvalidRange),
            "spawn count {count} is refused at the count bound"
        );
        let mut short_state = HOSTILE_STATE_WIRE.to_vec();
        short_state[8] = count;
        short_state.truncate(20);
        assert_eq!(
            HostileState::decode(&short_state),
            Err(ProtocolError::InvalidRange),
            "state count {count} is refused at the count bound"
        );
        let mut short_despawn = HOSTILE_DESPAWN_WIRE.to_vec();
        short_despawn[8] = count;
        short_despawn.truncate(20);
        assert_eq!(
            HostileDespawn::decode(&short_despawn),
            Err(ProtocolError::InvalidRange),
            "despawn count {count} is refused at the count bound"
        );
        let mut full = HOSTILE_SPAWN_WIRE;
        full[8] = count;
        assert_eq!(
            HostileSpawn::decode(&full),
            Err(ProtocolError::InvalidRange)
        );
    }

    // A declared count the payload cannot back is the truncation boundary,
    // because both sides apply the exact-remaining-length rule.
    let mut mismatched = HOSTILE_DESPAWN_WIRE;
    mismatched[8] = 3;
    assert_eq!(
        HostileDespawn::decode(&mismatched),
        Err(ProtocolError::Truncated)
    );
    let mut mismatched_state = HOSTILE_STATE_WIRE;
    mismatched_state[8] = 3;
    assert_eq!(
        HostileState::decode(&mismatched_state),
        Err(ProtocolError::Truncated)
    );

    // The constructors refuse the empty and the over-full batches.
    assert_eq!(
        HostileSpawn::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(
        HostileState::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(
        HostileDespawn::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    let over: Vec<HostileId> = (1..=MAX_HOSTILE_RECORDS as u64 + 1)
        .map(hostile_id)
        .collect();
    assert_eq!(
        HostileDespawn::new(0, over),
        Err(ProtocolError::InvalidRange)
    );
}

#[test]
fn hostile_batches_admit_the_full_record_ceiling() {
    // Sixty-four records is the count and record ceiling of all three
    // families: each batch admits, and the exact length is the tick, the
    // one-byte count and the records.
    let spawn = full_spawn();
    assert_eq!(spawn.spawns.len(), HOSTILE_SPAWN_MAX_RECORDS as usize);
    assert_eq!(
        spawn.encoded_len().expect("encoded_len"),
        8 + 1 + HOSTILE_SPAWN_MAX_RECORDS as usize * HOSTILE_SPAWN_WIRE_BYTES
    );
    let spawn_wire = spawn.encode().expect("encode the full spawn batch");
    assert_eq!(spawn_wire.len(), 9 + 64 * 30);
    assert_eq!(
        HostileSpawn::decode(&spawn_wire).expect("decode the full spawn batch"),
        spawn
    );
    // The sixty-fifth record is refused by the count bound.
    let mut over = spawn.clone();
    over.spawns.push(HostileSpawnRecord {
        id: hostile_id(65),
        ..spawn_record_one()
    });
    assert_eq!(over.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(over.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(over.encode(), Err(ProtocolError::InvalidRange));

    let state = full_state();
    assert_eq!(state.states.len(), HOSTILE_STATE_MAX_RECORDS as usize);
    assert_eq!(
        state.encoded_len().expect("encoded_len"),
        8 + 1 + HOSTILE_STATE_MAX_RECORDS as usize * HOSTILE_STATE_WIRE_BYTES
    );
    let state_wire = state.encode().expect("encode the full state batch");
    assert_eq!(state_wire.len(), 9 + 64 * 38);
    assert_eq!(
        HostileState::decode(&state_wire).expect("decode the full state batch"),
        state
    );
    let mut over_state = state.clone();
    over_state.states.push(HostileStateRecord {
        id: hostile_id(65),
        ..state_record_one()
    });
    assert_eq!(over_state.validate(), Err(ProtocolError::InvalidRange));

    let despawn = full_despawn();
    assert_eq!(despawn.ids.len(), MAX_HOSTILE_RECORDS as usize);
    assert_eq!(
        despawn.encoded_len().expect("encoded_len"),
        8 + 1 + MAX_HOSTILE_RECORDS as usize * HOSTILE_DESPAWN_WIRE_BYTES
    );
    let despawn_wire = despawn.encode().expect("encode the full despawn batch");
    assert_eq!(despawn_wire.len(), 9 + 64 * 8);
    assert_eq!(
        HostileDespawn::decode(&despawn_wire).expect("decode the full despawn batch"),
        despawn
    );
    let mut over_despawn = despawn.clone();
    over_despawn.ids.push(hostile_id(65));
    assert_eq!(over_despawn.validate(), Err(ProtocolError::InvalidRange));

    // The declared count is a single byte, so the ceiling is the byte range
    // the family can carry at all.
    assert_eq!(HOSTILE_SPAWN_MAX_RECORDS, 64);
    assert_eq!(HOSTILE_STATE_MAX_RECORDS, 64);
    assert_eq!(MAX_HOSTILE_RECORDS, 64);
}

#[test]
fn hostile_records_are_strictly_ordered_by_identity() {
    // Duplicate and descending identity orders are refused by every batch, and
    // the decode path answers at the same rule.
    let duplicate_spawn = HostileSpawn::new(0, vec![spawn_record_one(), spawn_record_one()]);
    assert_eq!(
        duplicate_spawn,
        Err(ProtocolError::InvalidRange),
        "a duplicate identity is refused"
    );
    let reversed_spawn = HostileSpawn::new(0, vec![spawn_record_two(), spawn_record_one()]);
    assert_eq!(reversed_spawn, Err(ProtocolError::InvalidRange));

    let duplicate_state = HostileState::new(0, vec![state_record_one(), state_record_one()]);
    assert_eq!(duplicate_state, Err(ProtocolError::InvalidRange));
    let reversed_state = HostileState::new(0, vec![state_record_two(), state_record_one()]);
    assert_eq!(reversed_state, Err(ProtocolError::InvalidRange));

    let duplicate_despawn = HostileDespawn::new(0, vec![hostile_id(1), hostile_id(1)]);
    assert_eq!(duplicate_despawn, Err(ProtocolError::InvalidRange));
    let reversed_despawn = HostileDespawn::new(0, vec![hostile_id(2), hostile_id(1)]);
    assert_eq!(reversed_despawn, Err(ProtocolError::InvalidRange));

    let mut duplicate_wire = HOSTILE_DESPAWN_WIRE;
    duplicate_wire[17..25].copy_from_slice(&1u64.to_le_bytes());
    assert_eq!(
        HostileDespawn::decode(&duplicate_wire),
        Err(ProtocolError::InvalidRange)
    );
    let reversed_wire = hostile_despawn_bytes(0, &[hostile_id(2), hostile_id(1)]);
    assert_eq!(
        HostileDespawn::decode(&reversed_wire),
        Err(ProtocolError::InvalidRange)
    );
    // A large identity pair proves the comparison is numeric rather than
    // lexicographic over the byte string.
    let wide = hostile_despawn_bytes(0, &[hostile_id(u64::MAX - 1), hostile_id(u64::MAX)]);
    assert_eq!(
        HostileDespawn::decode(&wide).map(|batch| batch.ids.len()),
        Ok(2)
    );
}

#[test]
fn hostile_spawn_dimension_is_matched_against_the_known_ids() {
    // The dimension is the raw wire `i32` matched against the two known IDs
    // rather than narrowed to a `u8`: the spawn record accepts only the
    // overworld, and a foreign raw value is an enum violation instead of a
    // reinterpreted dimension.
    for (raw, expect_ok) in [(0i32, true), (1, false), (256, false), (-1, false)] {
        let mut wire = HOSTILE_SPAWN_WIRE;
        wire[17..21].copy_from_slice(&raw.to_le_bytes());
        let decoded = HostileSpawn::decode(&wire);
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
    // The depths dimension is refused on the outbound surface too.
    let mut foreign = spawn_record_one();
    foreign.dimension = mornlea_domain::Dimension::DEPTHS;
    assert_eq!(
        HostileSpawn::new(0, vec![foreign]),
        Err(ProtocolError::InvalidEnum)
    );
}

#[test]
fn hostile_spawn_invalid_value_wins_over_short_capacity() {
    let mut over_kind = spawn_record_one();
    over_kind.kind = 2;
    let mut over_dimension = spawn_record_one();
    over_dimension.dimension = mornlea_domain::Dimension::DEPTHS;
    let mut over_health = spawn_record_one();
    over_health.health = 21;
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
            HOSTILE_SPAWN_WIRE.len(),
            vec![Box::new(move |dst: &mut [u8]| {
                HostileSpawn {
                    server_tick: 0,
                    spawns: vec![over_kind],
                }
                .encode_into(dst)
            })],
        ),
        (
            "spawn dimension is foreign",
            ProtocolError::InvalidEnum,
            HOSTILE_SPAWN_WIRE.len(),
            vec![Box::new(move |dst: &mut [u8]| {
                HostileSpawn {
                    server_tick: 0,
                    spawns: vec![over_dimension],
                }
                .encode_into(dst)
            })],
        ),
        (
            "spawn health is above the maximum",
            ProtocolError::InvalidRange,
            HOSTILE_SPAWN_WIRE.len(),
            vec![Box::new(move |dst: &mut [u8]| {
                HostileSpawn {
                    server_tick: 0,
                    spawns: vec![over_health],
                }
                .encode_into(dst)
            })],
        ),
        (
            "spawn records are descending",
            ProtocolError::InvalidRange,
            HOSTILE_SPAWN_WIRE.len(),
            vec![Box::new(move |dst: &mut [u8]| reversed.encode_into(dst))],
        ),
        (
            "spawn batch carries a duplicate",
            ProtocolError::InvalidRange,
            8 + 1 + 3 * HOSTILE_SPAWN_WIRE_BYTES,
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
fn hostile_state_invalid_value_wins_over_short_capacity() {
    let mut over_kind = state_record_one();
    over_kind.kind = 2;
    let mut over_velocity = state_record_one();
    over_velocity.velocity = [f32::NAN, 0.25, 3.0];
    let mut over_health = state_record_one();
    over_health.health = 0;

    let cases: Vec<(
        &str,
        ProtocolError,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "state kind is outside the closed set",
            ProtocolError::InvalidEnum,
            HOSTILE_STATE_WIRE.len(),
            Box::new(move |dst: &mut [u8]| {
                let mut batch = canonical_state();
                batch.states[0] = over_kind;
                batch.encode_into(dst)
            }),
        ),
        (
            "state velocity is not finite",
            ProtocolError::InvalidFloat,
            HOSTILE_STATE_WIRE.len(),
            Box::new(move |dst: &mut [u8]| {
                let mut batch = canonical_state();
                batch.states[0] = over_velocity;
                batch.encode_into(dst)
            }),
        ),
        (
            "state health is zero",
            ProtocolError::InvalidRange,
            HOSTILE_STATE_WIRE.len(),
            Box::new(move |dst: &mut [u8]| {
                let mut batch = canonical_state();
                batch.states[0] = over_health;
                batch.encode_into(dst)
            }),
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
fn hostile_despawn_invalid_value_wins_over_short_capacity() {
    let mut duplicate = canonical_despawn();
    duplicate.ids.push(hostile_id(2));
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
            8 + 1 + 3 * HOSTILE_DESPAWN_WIRE_BYTES,
            Box::new(move |dst: &mut [u8]| duplicate.encode_into(dst)),
        ),
        (
            "despawn batch is descending",
            ProtocolError::InvalidRange,
            8 + 1 + 2 * HOSTILE_DESPAWN_WIRE_BYTES,
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
/// A record whose fields were public and mutable with no value gate on the
/// encode path published whatever the caller wrote: the previous infallible
/// `encode` trusted the constructor, so a spawn record mutated into kind 2, a
/// foreign dimension, health 21 or a zero identity after construction was
/// published byte for byte. The kind-two record published as
/// `[00, 00, 00, 00, 00, 00, 00, 00, 01, 07, 00, 00, 00, 00, 00, 00, 00, 00,
/// 00, 00, 00, 00, 00, 80, 3f, 00, 00, 00, 40, 00, 00, 40, 40, 00, 00, 00,
/// 3f, 0a, 02]` — health 10 and kind 2 — and the depths spawn as the same bytes
/// with the dimension word `01, 00, 00, 00` and the health-21 spawn with
/// `0x15` in the health byte. A zero-identity spawn published `0` in the
/// identity word, a descending pair published `09` before `07`, and the empty
/// batch published the bare `00` count. A non-finite pose reached the
/// primitive's own refusal, which the previous surface turned into a panic at
/// its `expect`. The fallible surface turns every one of them into a value
/// error before any capacity decision.
#[test]
fn hostile_mutated_public_fields_are_never_published() {
    for kind in [2u8, 0xff] {
        let mut batch = canonical_spawn();
        batch.spawns[0].kind = kind;
        assert_eq!(batch.validate(), Err(ProtocolError::InvalidEnum));
        assert_eq!(batch.encoded_len(), Err(ProtocolError::InvalidEnum));
        assert_eq!(batch.encode(), Err(ProtocolError::InvalidEnum));
    }

    let mut foreign = canonical_spawn();
    foreign.spawns[0].dimension = mornlea_domain::Dimension::DEPTHS;
    assert_eq!(foreign.validate(), Err(ProtocolError::InvalidEnum));
    assert_eq!(foreign.encoded_len(), Err(ProtocolError::InvalidEnum));
    assert_eq!(foreign.encode(), Err(ProtocolError::InvalidEnum));

    let mut over_health = canonical_spawn();
    over_health.spawns[1].health = 21;
    assert_eq!(over_health.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(over_health.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(over_health.encode(), Err(ProtocolError::InvalidRange));

    let mut non_finite = canonical_state();
    non_finite.states[0].velocity = [f32::NAN, 0.25, 3.0];
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

    let mut empty = canonical_despawn();
    empty.ids.clear();
    assert_eq!(empty.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(empty.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(empty.encode(), Err(ProtocolError::InvalidRange));
}

#[test]
fn hostile_decode_rejects_every_proper_truncation() {
    // All three canonical payloads are short enough for a full sweep, and every
    // cut answers at the truncation boundary: the tick, the one-byte count
    // prefix and the record region all report the same failure the Go decoder
    // reports for the same bytes.
    for cut in 0..HOSTILE_SPAWN_WIRE.len() {
        let error = HostileSpawn::decode(&HOSTILE_SPAWN_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("spawn: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "spawn: truncation at {cut} bytes reports {error:?}"
        );
    }
    for cut in 0..HOSTILE_STATE_WIRE.len() {
        let error = HostileState::decode(&HOSTILE_STATE_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("state: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "state: truncation at {cut} bytes reports {error:?}"
        );
    }
    for cut in 0..HOSTILE_DESPAWN_WIRE.len() {
        let error = HostileDespawn::decode(&HOSTILE_DESPAWN_WIRE[..cut])
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
    for cut in [10usize, 9 + 30, 9 + 2 * 30 - 1, 9 + 64 * 30 - 1] {
        let error = HostileSpawn::decode(&spawn[..cut])
            .err()
            .unwrap_or_else(|| panic!("full spawn: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "full spawn: truncation at {cut} bytes reports {error:?}"
        );
    }
    let state = full_state().encode().expect("encode the full state batch");
    for cut in [10usize, 9 + 38, 9 + 2 * 38 - 1, 9 + 64 * 38 - 1] {
        let error = HostileState::decode(&state[..cut])
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
fn hostile_decode_rejects_one_trailing_byte() {
    // All three families apply the exact-remaining-length rule, so a one-byte
    // extra payload is answered by the length check before any record is read.
    // The Go decoder applies the same rule — its remaining-length message
    // precedes the record loop — so a padded payload reports the truncation
    // boundary on both sides rather than a trailing byte.
    let mut spawn = HOSTILE_SPAWN_WIRE.to_vec();
    spawn.push(0x00);
    assert_eq!(
        HostileSpawn::decode(&spawn),
        Err(ProtocolError::Truncated),
        "a padded spawn payload reports the truncation boundary"
    );
    let mut state = HOSTILE_STATE_WIRE.to_vec();
    state.push(0x00);
    assert_eq!(
        HostileState::decode(&state),
        Err(ProtocolError::Truncated),
        "a padded state payload reports the truncation boundary"
    );
    let mut despawn = HOSTILE_DESPAWN_WIRE.to_vec();
    despawn.push(0x00);
    assert_eq!(
        HostileDespawn::decode(&despawn),
        Err(ProtocolError::Truncated),
        "a padded despawn payload reports the truncation boundary"
    );
    let mut full = full_despawn().encode().expect("encode the full batch");
    full.push(0x00);
    assert_eq!(HostileDespawn::decode(&full), Err(ProtocolError::Truncated));
}

#[test]
fn hostile_encode_into_leaves_a_short_destination_unchanged() {
    let cases: Vec<(
        &str,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "hostile spawn",
            HOSTILE_SPAWN_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_spawn().encode_into(dst)),
        ),
        (
            "hostile spawn at the ceiling",
            8 + 1 + HOSTILE_SPAWN_MAX_RECORDS as usize * HOSTILE_SPAWN_WIRE_BYTES,
            Box::new(|dst: &mut [u8]| full_spawn().encode_into(dst)),
        ),
        (
            "hostile state",
            HOSTILE_STATE_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_state().encode_into(dst)),
        ),
        (
            "hostile state at the ceiling",
            8 + 1 + HOSTILE_STATE_MAX_RECORDS as usize * HOSTILE_STATE_WIRE_BYTES,
            Box::new(|dst: &mut [u8]| full_state().encode_into(dst)),
        ),
        (
            "hostile despawn",
            HOSTILE_DESPAWN_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_despawn().encode_into(dst)),
        ),
        (
            "hostile despawn at the ceiling",
            8 + 1 + MAX_HOSTILE_RECORDS as usize * HOSTILE_DESPAWN_WIRE_BYTES,
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
fn hostile_packet_ids_are_pinned() {
    assert_eq!(HostileSpawn::PACKET_ID, 22);
    assert_eq!(HostileState::PACKET_ID, 23);
    assert_eq!(HostileDespawn::PACKET_ID, 24);
}
