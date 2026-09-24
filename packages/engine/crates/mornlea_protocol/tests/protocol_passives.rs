//! The three passive-mob publication families: the common fallible surface,
//! the closed grazing and reason rules, and the exact-record batch rule.
//!
//! `PassiveSpawn` (S/Play/26), `PassiveState` (S/Play/27) and
//! `PassiveDespawn` (S/Play/28) publish the passive mob bodies one subscriber
//! can see. All three carry a `u64` server tick and a one-byte record count,
//! and all three admit exactly `1..=MAX_PASSIVE_RECORDS` records ordered by
//! the checked domain `PassiveId`, which is a nonzero `u64` newtype: a zero ID
//! is the absent form, so it cannot be constructed on the outbound surface and
//! is refused at the identity boundary where it is read. No grazing behaviour,
//! spawning rule or death-reason derivation is on this surface — those stay
//! authority concerns — so the wire carries the observation, not the rule.
//!
//! The three records differ in exactly one field each, and the difference is
//! the Go one: a spawn record carries the dimension and omits the velocity,
//! because a dimension change always goes through a despawn/spawn pair, while
//! a state record carries the velocity and omits the dimension, because the
//! mirror already holds it from the spawn. Unlike a hostile record, neither
//! carries a kind byte, because a passive mob is the only passive kind; the
//! fixed strides are therefore 29, 38 and 9 bytes.
//!
//! The 64-record wire bound is the protocol budget and is admitted on all
//! three families. The authority converges on a smaller live capacity of
//! passive actors, which is a separate concern that never enters this packet
//! layer: a future tightening of the live cap is an authority change and must
//! not narrow what the wire admits.
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

use mornlea_domain::PassiveId;
use mornlea_protocol::{
    MAX_PASSIVE_RECORDS, MAX_PASSIVE_SPAWN_RECORDS, MAX_PASSIVE_STATE_RECORDS,
    PASSIVE_DESPAWN_DIED, PASSIVE_DESPAWN_VANISHED, PASSIVE_DESPAWN_WIRE_BYTES,
    PASSIVE_SPAWN_WIRE_BYTES, PASSIVE_STATE_WIRE_BYTES, PassiveDespawn, PassiveDespawnRecord,
    PassiveSpawn, PassiveSpawnRecord, PassiveState, PassiveStateRecord, ProtocolError,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The reviewed 67-byte PassiveSpawn literal: tick 0, two records strictly
/// ordered by ID, the overworld dimension, the health boundaries 1 and 20, and
/// no kind byte. The first position carries a negative zero.
const PASSIVE_SPAWN_WIRE: [u8; 67] = [
    // tick 0 and the declared count of two records
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
    // first record: ID 1, dimension 0, position (-0.0, 1.0, 2.0), yaw 0.0,
    // health 1
    0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80,
    0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x00, 0x01,
    // second record: ID 2, dimension 0, position (3.0, 4.0, 5.0), yaw -2.5,
    // health 20
    0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40,
    0x00, 0x00, 0x80, 0x40, 0x00, 0x00, 0xa0, 0x40, 0x00, 0x00, 0x20, 0xc0, 0x14,
];

/// The reviewed 85-byte PassiveState literal: tick 0, two records strictly
/// ordered by ID, finite velocities, the health values 13 and 7, and the
/// grazing boundaries 1 and 0. No dimension byte is present, so the record
/// stride is 38.
const PASSIVE_STATE_WIRE: [u8; 85] = [
    // tick 0 and the declared count of two records
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
    // first record: ID 1, position (-0.0, 1.0, 2.0), velocity (-0.0, 0.25,
    // 3.0), yaw 0.5, health 13, grazing 1
    0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f,
    0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3e, 0x00, 0x00, 0x40, 0x40,
    0x00, 0x00, 0x00, 0x3f, 0x0d, 0x01,
    // second record: ID 2, position (3.0, 4.0, 5.0), velocity (0.5, -1.25,
    // 0.0), yaw -2.5, health 7, grazing 0
    0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x80, 0x40,
    0x00, 0x00, 0xa0, 0x40, 0x00, 0x00, 0x00, 0x3f, 0x00, 0x00, 0xa0, 0xbf, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x20, 0xc0, 0x07, 0x00,
];

/// The reviewed 27-byte PassiveDespawn literal: tick 0 and the two strictly
/// ordered identities with the two published removal reasons.
const PASSIVE_DESPAWN_WIRE: [u8; 27] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
];

/// Wraps one nonzero passive identity, which the domain rule already admits.
fn passive_id(id: u64) -> PassiveId {
    PassiveId::try_new(id).expect("reviewed nonzero passive identity")
}

/// The first reviewed spawn record.
fn spawn_record_one() -> PassiveSpawnRecord {
    PassiveSpawnRecord {
        id: passive_id(1),
        dimension: mornlea_domain::Dimension::OVERWORLD,
        position: [-0.0, 1.0, 2.0],
        yaw: 0.0,
        health: 1,
    }
}

/// The second reviewed spawn record: the health upper boundary.
fn spawn_record_two() -> PassiveSpawnRecord {
    PassiveSpawnRecord {
        id: passive_id(2),
        dimension: mornlea_domain::Dimension::OVERWORLD,
        position: [3.0, 4.0, 5.0],
        yaw: -2.5,
        health: 20,
    }
}

/// The canonical two-record spawn batch the reviewed literal carries.
fn canonical_spawn() -> PassiveSpawn {
    PassiveSpawn::new(0, vec![spawn_record_one(), spawn_record_two()])
        .expect("the canonical batch is valid")
}

/// The first reviewed state record, whose velocity carries a negative zero.
fn state_record_one() -> PassiveStateRecord {
    PassiveStateRecord {
        id: passive_id(1),
        position: [-0.0, 1.0, 2.0],
        velocity: [-0.0, 0.25, 3.0],
        yaw: 0.5,
        health: 13,
        grazing: 1,
    }
}

/// The second reviewed state record.
fn state_record_two() -> PassiveStateRecord {
    PassiveStateRecord {
        id: passive_id(2),
        position: [3.0, 4.0, 5.0],
        velocity: [0.5, -1.25, 0.0],
        yaw: -2.5,
        health: 7,
        grazing: 0,
    }
}

/// The canonical two-record state batch the reviewed literal carries.
fn canonical_state() -> PassiveState {
    PassiveState::new(0, vec![state_record_one(), state_record_two()])
        .expect("the canonical batch is valid")
}

/// The canonical despawn batch the reviewed literal carries.
fn canonical_despawn() -> PassiveDespawn {
    PassiveDespawn::new(
        0,
        vec![
            PassiveDespawnRecord {
                id: passive_id(1),
                reason: PASSIVE_DESPAWN_VANISHED,
            },
            PassiveDespawnRecord {
                id: passive_id(2),
                reason: PASSIVE_DESPAWN_DIED,
            },
        ],
    )
    .expect("the canonical batch is valid")
}

/// The full 64-record spawn batch, the fixed record ceiling, with ascending
/// identities on every entry point.
fn full_spawn() -> PassiveSpawn {
    let spawns: Vec<PassiveSpawnRecord> = (1..=MAX_PASSIVE_SPAWN_RECORDS as u64)
        .map(|id| PassiveSpawnRecord {
            id: passive_id(id),
            ..spawn_record_one()
        })
        .collect();
    PassiveSpawn::new(0, spawns).expect("the full batch is valid")
}

/// The full 64-record state batch, the fixed record ceiling.
fn full_state() -> PassiveState {
    let states: Vec<PassiveStateRecord> = (1..=MAX_PASSIVE_STATE_RECORDS as u64)
        .map(|id| PassiveStateRecord {
            id: passive_id(id),
            ..state_record_one()
        })
        .collect();
    PassiveState::new(0, states).expect("the full batch is valid")
}

/// The full 64-record despawn batch, the fixed record ceiling.
fn full_despawn() -> PassiveDespawn {
    let despawns: Vec<PassiveDespawnRecord> = (1..=MAX_PASSIVE_RECORDS as u64)
        .map(|id| PassiveDespawnRecord {
            id: passive_id(id),
            reason: PASSIVE_DESPAWN_VANISHED,
        })
        .collect();
    PassiveDespawn::new(0, despawns).expect("the full batch is valid")
}

/// Renders one spawn payload: the tick, the one-byte count and the records.
fn passive_spawn_bytes(tick: u64, records: &[PassiveSpawnRecord]) -> Vec<u8> {
    let mut wire = Vec::with_capacity(9 + records.len() * PASSIVE_SPAWN_WIRE_BYTES);
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
    }
    wire
}

/// Renders one state payload: the tick, the one-byte count and the records.
fn passive_state_bytes(tick: u64, records: &[PassiveStateRecord]) -> Vec<u8> {
    let mut wire = Vec::with_capacity(9 + records.len() * PASSIVE_STATE_WIRE_BYTES);
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
        wire.push(record.grazing);
    }
    wire
}

/// Renders one despawn payload: the tick, the one-byte count and the records.
fn passive_despawn_bytes(tick: u64, records: &[PassiveDespawnRecord]) -> Vec<u8> {
    let mut wire = Vec::with_capacity(9 + records.len() * PASSIVE_DESPAWN_WIRE_BYTES);
    wire.extend_from_slice(&tick.to_le_bytes());
    wire.push(records.len() as u8);
    for record in records {
        wire.extend_from_slice(&record.id.get().to_le_bytes());
        wire.push(record.reason);
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
fn passive_spawn_round_trips_through_the_fallible_surface() {
    let spawn = canonical_spawn();
    let decoded = PassiveSpawn::decode(&PASSIVE_SPAWN_WIRE).expect("decode");
    assert_round_trip(
        "passive spawn",
        &PASSIVE_SPAWN_WIRE,
        || spawn.validate(),
        || spawn.encoded_len(),
        |dst| spawn.encode_into(dst),
        || spawn.encode(),
        |payload| PassiveSpawn::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == spawn,
    );
    assert_eq!(
        PASSIVE_SPAWN_WIRE.len(),
        8 + 1 + 2 * PASSIVE_SPAWN_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.spawns.len(), 2);
    assert_eq!(decoded.spawns[0].id.get(), 1);
    assert_eq!(decoded.spawns[1].id.get(), 2);
    // Both records name the overworld, and the health boundaries 1 and 20
    // survive the round trip.
    for record in &decoded.spawns {
        assert_eq!(record.dimension, mornlea_domain::Dimension::OVERWORLD);
    }
    assert_eq!(decoded.spawns[0].health, 1);
    assert_eq!(decoded.spawns[1].health, 20);
}

#[test]
fn passive_state_round_trips_through_the_fallible_surface() {
    let state = canonical_state();
    let decoded = PassiveState::decode(&PASSIVE_STATE_WIRE).expect("decode");
    assert_round_trip(
        "passive state",
        &PASSIVE_STATE_WIRE,
        || state.validate(),
        || state.encoded_len(),
        |dst| state.encode_into(dst),
        || state.encode(),
        |payload| PassiveState::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == state,
    );
    assert_eq!(
        PASSIVE_STATE_WIRE.len(),
        8 + 1 + 2 * PASSIVE_STATE_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.states.len(), 2);
    assert_eq!(decoded.states[0].id.get(), 1);
    assert_eq!(decoded.states[1].id.get(), 2);
    assert_eq!(decoded.states[0].health, 13);
    assert_eq!(decoded.states[0].grazing, 1);
    assert_eq!(decoded.states[1].health, 7);
    assert_eq!(decoded.states[1].grazing, 0);
}

#[test]
fn passive_despawn_round_trips_through_the_fallible_surface() {
    let despawn = canonical_despawn();
    let decoded = PassiveDespawn::decode(&PASSIVE_DESPAWN_WIRE).expect("decode");
    assert_round_trip(
        "passive despawn",
        &PASSIVE_DESPAWN_WIRE,
        || despawn.validate(),
        || despawn.encoded_len(),
        |dst| despawn.encode_into(dst),
        || despawn.encode(),
        |payload| PassiveDespawn::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == despawn,
    );
    assert_eq!(
        PASSIVE_DESPAWN_WIRE.len(),
        8 + 1 + 2 * PASSIVE_DESPAWN_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.despawns.len(), 2);
    assert_eq!(decoded.despawns[0].reason, PASSIVE_DESPAWN_VANISHED);
    assert_eq!(decoded.despawns[1].reason, PASSIVE_DESPAWN_DIED);
}

#[test]
fn passive_spawn_omits_kind_and_state_omits_dimension() {
    // The three records differ by exactly their carried fields, and the
    // difference is the Go one: the spawn record carries the dimension the
    // state record never carries, the state record carries the velocity the
    // spawn record never carries, and neither carries a kind byte because a
    // passive mob has no category to publish.
    assert_eq!(PASSIVE_SPAWN_WIRE_BYTES, 8 + 4 + 12 + 4 + 1);
    assert_eq!(PASSIVE_STATE_WIRE_BYTES, 8 + 12 + 12 + 4 + 1 + 1);
    assert_eq!(PASSIVE_DESPAWN_WIRE_BYTES, 8 + 1);
    assert_eq!(
        PASSIVE_STATE_WIRE_BYTES - PASSIVE_SPAWN_WIRE_BYTES,
        9,
        "the state record is the spawn record minus the dimension word plus the velocity words and the grazing bit, with no kind byte on either"
    );

    // The spawn's dimension is the four bytes after the identity, and the
    // record ends at the fixed 29-byte stride with the health byte — no kind
    // byte follows it.
    assert_eq!(&PASSIVE_SPAWN_WIRE[17..21], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(PASSIVE_SPAWN_WIRE.len(), 9 + 2 * 29);
    assert_eq!(PASSIVE_SPAWN_WIRE[9 + 29 - 1], 0x01);
    // The state record begins its pose immediately after the identity, so the
    // first position word is the same negative zero the spawn carries, and the
    // record ends with the grazing bit rather than a kind byte.
    assert_eq!(&PASSIVE_STATE_WIRE[17..21], &[0x00, 0x00, 0x00, 0x80]);
    assert_eq!(PASSIVE_STATE_WIRE.len(), 9 + 2 * 38);
    assert_eq!(PASSIVE_STATE_WIRE[9 + 38 - 1], 0x01);
    // The despawn record ends with the reason byte.
    assert_eq!(PASSIVE_DESPAWN_WIRE.len(), 9 + 2 * 9);
    assert_eq!(PASSIVE_DESPAWN_WIRE[9 + 9 - 1], 0x00);
    assert_eq!(PASSIVE_DESPAWN_WIRE[18 + 9 - 1], 0x01);
}

#[test]
fn passive_grazing_and_reason_are_closed_pairs() {
    // The grazing bit and the removal reason are both 0/1 wire tags. The two
    // published values are admitted on every entry point and any other byte is
    // refused at the enum boundary, which is the category the Go validator
    // publishes for the same byte.
    for grazing in [0u8, 1] {
        let mut state_record = state_record_one();
        state_record.grazing = grazing;
        assert!(PassiveState::new(0, vec![state_record]).is_ok());
    }
    for grazing in [2u8, 3, 0xff] {
        let mut state_record = state_record_one();
        state_record.grazing = grazing;
        assert_eq!(
            PassiveState::new(0, vec![state_record]),
            Err(ProtocolError::InvalidEnum),
            "grazing {grazing} is refused at the enum boundary"
        );
        let mut batch = canonical_state();
        batch.states[0].grazing = grazing;
        assert_eq!(
            batch.validate(),
            Err(ProtocolError::InvalidEnum),
            "grazing {grazing} survives into the encode path"
        );
        assert_eq!(batch.encoded_len(), Err(ProtocolError::InvalidEnum));
        assert_eq!(batch.encode(), Err(ProtocolError::InvalidEnum));
    }

    for reason in [PASSIVE_DESPAWN_VANISHED, PASSIVE_DESPAWN_DIED] {
        let record = PassiveDespawnRecord {
            id: passive_id(1),
            reason,
        };
        assert!(PassiveDespawn::new(0, vec![record]).is_ok());
    }
    for reason in [2u8, 0xff] {
        let record = PassiveDespawnRecord {
            id: passive_id(1),
            reason,
        };
        assert_eq!(
            PassiveDespawn::new(0, vec![record]),
            Err(ProtocolError::InvalidEnum),
            "reason {reason} is refused at the enum boundary"
        );
        let mut batch = canonical_despawn();
        batch.despawns[0].reason = reason;
        assert_eq!(batch.validate(), Err(ProtocolError::InvalidEnum));
        assert_eq!(batch.encoded_len(), Err(ProtocolError::InvalidEnum));
        assert_eq!(batch.encode(), Err(ProtocolError::InvalidEnum));
    }

    // The decode path answers at the same boundary: the grazing byte and the
    // reason byte are the last byte of each record.
    let mut state_grazing_two = PASSIVE_STATE_WIRE;
    state_grazing_two[9 + 38 - 1] = 2;
    assert_eq!(
        PassiveState::decode(&state_grazing_two),
        Err(ProtocolError::InvalidEnum)
    );
    let mut despawn_reason_two = PASSIVE_DESPAWN_WIRE;
    despawn_reason_two[9 + 9 - 1] = 2;
    assert_eq!(
        PassiveDespawn::decode(&despawn_reason_two),
        Err(ProtocolError::InvalidEnum)
    );
}

#[test]
fn passive_health_span_is_one_to_twenty() {
    // The Go `core.MaxHealth` span is inclusive at both ends, and the two
    // outside values are refused on every entry point.
    for health in [1u8, 20] {
        let mut spawn_record = spawn_record_one();
        spawn_record.health = health;
        assert!(PassiveSpawn::new(0, vec![spawn_record]).is_ok());
        let mut state_record = state_record_one();
        state_record.health = health;
        assert!(PassiveState::new(0, vec![state_record]).is_ok());
    }
    for health in [0u8, 21, 0xff] {
        let mut spawn_record = spawn_record_one();
        spawn_record.health = health;
        assert_eq!(
            PassiveSpawn::new(0, vec![spawn_record]),
            Err(ProtocolError::InvalidRange)
        );
        let mut state_record = state_record_one();
        state_record.health = health;
        assert_eq!(
            PassiveState::new(0, vec![state_record]),
            Err(ProtocolError::InvalidRange)
        );
    }

    // The decode path answers at the same boundary: the health byte precedes
    // the grazing byte in the state record and closes the spawn record.
    let mut spawn_health_zero = PASSIVE_SPAWN_WIRE;
    spawn_health_zero[9 + 29 - 1] = 0;
    assert_eq!(
        PassiveSpawn::decode(&spawn_health_zero),
        Err(ProtocolError::InvalidRange)
    );
    let mut state_health_above = PASSIVE_STATE_WIRE;
    state_health_above[9 + 38 - 2] = 21;
    assert_eq!(
        PassiveState::decode(&state_health_above),
        Err(ProtocolError::InvalidRange)
    );
}

#[test]
fn passive_records_preserve_negative_zero_pose_bits() {
    // A negative zero component survives the round trip in both records, so
    // the pose is never normalized on either side.
    let spawn = canonical_spawn();
    assert_eq!(spawn.spawns[0].position[0].to_bits(), 0x8000_0000);
    let decoded = PassiveSpawn::decode(&spawn.encode().expect("encode")).expect("decode");
    assert_eq!(decoded.spawns[0].position[0].to_bits(), 0x8000_0000);

    let state = canonical_state();
    assert_eq!(state.states[0].position[0].to_bits(), 0x8000_0000);
    assert_eq!(state.states[0].velocity[0].to_bits(), 0x8000_0000);
    let decoded = PassiveState::decode(&state.encode().expect("encode")).expect("decode");
    assert_eq!(decoded.states[0].position[0].to_bits(), 0x8000_0000);
    assert_eq!(decoded.states[0].velocity[0].to_bits(), 0x8000_0000);

    // A non-finite component is refused where it is written and where it is
    // read, so neither side publishes a NaN or an infinity.
    let mut spawn_pose = passive_spawn_bytes(0, &[spawn_record_one()]);
    // The first position word follows the identity and the dimension.
    spawn_pose[21..25].copy_from_slice(&f32::NAN.to_bits().to_le_bytes());
    assert_eq!(
        PassiveSpawn::decode(&spawn_pose),
        Err(ProtocolError::InvalidFloat)
    );
    let mut state_pose = passive_state_bytes(0, &[state_record_one()]);
    // The first velocity word follows the identity and the position.
    state_pose[29..33].copy_from_slice(&f32::INFINITY.to_bits().to_le_bytes());
    assert_eq!(
        PassiveState::decode(&state_pose),
        Err(ProtocolError::InvalidFloat)
    );
    let mut non_finite = canonical_state();
    non_finite.states[0].velocity = [f32::NAN, 0.25, 3.0];
    assert_eq!(non_finite.encode(), Err(ProtocolError::InvalidFloat));
}

#[test]
fn passive_count_bound_fires_before_the_record_rule() {
    // A declared count outside 1..=64 is answered at the count bound even when
    // the payload cannot back it, which is the order the Go decoder applies:
    // the count message precedes its remaining-length check.
    for count in [0u8, 65, 0xff] {
        let mut short = PASSIVE_SPAWN_WIRE.to_vec();
        short[8] = count;
        short.truncate(20);
        assert_eq!(
            PassiveSpawn::decode(&short),
            Err(ProtocolError::InvalidRange),
            "spawn count {count} is refused at the count bound"
        );
        let mut short_state = PASSIVE_STATE_WIRE.to_vec();
        short_state[8] = count;
        short_state.truncate(20);
        assert_eq!(
            PassiveState::decode(&short_state),
            Err(ProtocolError::InvalidRange),
            "state count {count} is refused at the count bound"
        );
        let mut short_despawn = PASSIVE_DESPAWN_WIRE.to_vec();
        short_despawn[8] = count;
        short_despawn.truncate(20);
        assert_eq!(
            PassiveDespawn::decode(&short_despawn),
            Err(ProtocolError::InvalidRange),
            "despawn count {count} is refused at the count bound"
        );
        let mut full = PASSIVE_SPAWN_WIRE;
        full[8] = count;
        assert_eq!(
            PassiveSpawn::decode(&full),
            Err(ProtocolError::InvalidRange)
        );
    }

    // A declared count the payload cannot back is the truncation boundary,
    // because both sides apply the exact-remaining-length rule.
    let mut mismatched = PASSIVE_DESPAWN_WIRE;
    mismatched[8] = 3;
    assert_eq!(
        PassiveDespawn::decode(&mismatched),
        Err(ProtocolError::Truncated)
    );
    let mut mismatched_state = PASSIVE_STATE_WIRE;
    mismatched_state[8] = 3;
    assert_eq!(
        PassiveState::decode(&mismatched_state),
        Err(ProtocolError::Truncated)
    );

    // The constructors refuse the empty and the over-full batches.
    assert_eq!(
        PassiveSpawn::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(
        PassiveState::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(
        PassiveDespawn::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    let over: Vec<PassiveSpawnRecord> = (1..=MAX_PASSIVE_SPAWN_RECORDS as u64 + 1)
        .map(|id| PassiveSpawnRecord {
            id: passive_id(id),
            ..spawn_record_one()
        })
        .collect();
    assert_eq!(PassiveSpawn::new(0, over), Err(ProtocolError::InvalidRange));
}

#[test]
fn passive_batches_admit_the_full_record_ceiling() {
    // Sixty-four records is the count and record ceiling of all three
    // families: each batch admits, and the exact length is the tick, the
    // one-byte count and the records.
    //
    // The ceiling is the protocol budget the wire publishes. The authority
    // converges on a smaller live capacity of passive actors, which is a
    // separate concern: the 32-actor live cap never enters this packet layer,
    // so a future tightening of the live cap must not narrow what a payload
    // admits here.
    let spawn = full_spawn();
    assert_eq!(spawn.spawns.len(), MAX_PASSIVE_SPAWN_RECORDS as usize);
    assert_eq!(
        spawn.encoded_len().expect("encoded_len"),
        8 + 1 + MAX_PASSIVE_SPAWN_RECORDS as usize * PASSIVE_SPAWN_WIRE_BYTES
    );
    let spawn_wire = spawn.encode().expect("encode the full spawn batch");
    assert_eq!(spawn_wire.len(), 9 + 64 * 29);
    assert_eq!(
        PassiveSpawn::decode(&spawn_wire).expect("decode the full spawn batch"),
        spawn
    );

    let state = full_state();
    assert_eq!(state.states.len(), MAX_PASSIVE_STATE_RECORDS as usize);
    assert_eq!(
        state.encoded_len().expect("encoded_len"),
        8 + 1 + MAX_PASSIVE_STATE_RECORDS as usize * PASSIVE_STATE_WIRE_BYTES
    );
    let state_wire = state.encode().expect("encode the full state batch");
    assert_eq!(state_wire.len(), 9 + 64 * 38);
    assert_eq!(
        PassiveState::decode(&state_wire).expect("decode the full state batch"),
        state
    );

    let despawn = full_despawn();
    assert_eq!(despawn.despawns.len(), MAX_PASSIVE_RECORDS as usize);
    assert_eq!(
        despawn.encoded_len().expect("encoded_len"),
        8 + 1 + MAX_PASSIVE_RECORDS as usize * PASSIVE_DESPAWN_WIRE_BYTES
    );
    let despawn_wire = despawn.encode().expect("encode the full despawn batch");
    assert_eq!(despawn_wire.len(), 9 + 64 * 9);
    assert_eq!(
        PassiveDespawn::decode(&despawn_wire).expect("decode the full despawn batch"),
        despawn
    );

    // The declared count is a single byte, so the ceiling is the byte range
    // the family can carry at all.
    assert_eq!(MAX_PASSIVE_SPAWN_RECORDS, 64);
    assert_eq!(MAX_PASSIVE_STATE_RECORDS, 64);
    assert_eq!(MAX_PASSIVE_RECORDS, 64);
    // The fixed wire ceilings are the tick, the count and the full batch.
    assert_eq!(mornlea_protocol::PASSIVE_SPAWN_MAX_WIRE_BYTES, 9 + 64 * 29);
    assert_eq!(mornlea_protocol::PASSIVE_STATE_MAX_WIRE_BYTES, 9 + 64 * 38);
    assert_eq!(mornlea_protocol::PASSIVE_DESPAWN_MAX_WIRE_BYTES, 9 + 64 * 9);
}

#[test]
fn passive_records_are_strictly_ordered_by_identity() {
    // Duplicate and descending identity orders are refused by every batch, and
    // the decode path answers at the same rule.
    let duplicate_spawn = PassiveSpawn::new(0, vec![spawn_record_one(), spawn_record_one()]);
    assert_eq!(
        duplicate_spawn,
        Err(ProtocolError::InvalidRange),
        "a duplicate identity is refused"
    );
    let reversed_spawn = PassiveSpawn::new(0, vec![spawn_record_two(), spawn_record_one()]);
    assert_eq!(reversed_spawn, Err(ProtocolError::InvalidRange));

    let duplicate_state = PassiveState::new(0, vec![state_record_one(), state_record_one()]);
    assert_eq!(duplicate_state, Err(ProtocolError::InvalidRange));
    let reversed_state = PassiveState::new(0, vec![state_record_two(), state_record_one()]);
    assert_eq!(reversed_state, Err(ProtocolError::InvalidRange));

    let duplicate_despawn = PassiveDespawn::new(
        0,
        vec![
            PassiveDespawnRecord {
                id: passive_id(1),
                reason: PASSIVE_DESPAWN_VANISHED,
            },
            PassiveDespawnRecord {
                id: passive_id(1),
                reason: PASSIVE_DESPAWN_DIED,
            },
        ],
    );
    assert_eq!(duplicate_despawn, Err(ProtocolError::InvalidRange));
    let reversed_despawn = passive_despawn_bytes(
        0,
        &[
            PassiveDespawnRecord {
                id: passive_id(2),
                reason: PASSIVE_DESPAWN_VANISHED,
            },
            PassiveDespawnRecord {
                id: passive_id(1),
                reason: PASSIVE_DESPAWN_VANISHED,
            },
        ],
    );
    assert_eq!(
        PassiveDespawn::decode(&reversed_despawn),
        Err(ProtocolError::InvalidRange)
    );
    // A large identity pair proves the comparison is numeric rather than
    // lexicographic over the byte string.
    let wide = passive_despawn_bytes(
        0,
        &[
            PassiveDespawnRecord {
                id: passive_id(u64::MAX - 1),
                reason: PASSIVE_DESPAWN_VANISHED,
            },
            PassiveDespawnRecord {
                id: passive_id(u64::MAX),
                reason: PASSIVE_DESPAWN_DIED,
            },
        ],
    );
    assert_eq!(
        PassiveDespawn::decode(&wide).map(|batch| batch.despawns.len()),
        Ok(2)
    );
}

#[test]
fn passive_spawn_dimension_is_matched_against_the_known_ids() {
    // The dimension is the raw wire `i32` matched against the two known IDs
    // rather than narrowed to a `u8`: the spawn record accepts only the
    // overworld, and a foreign raw value is an enum violation instead of a
    // reinterpreted dimension.
    for (raw, expect_ok) in [(0i32, true), (1, false), (256, false), (-1, false)] {
        let mut wire = PASSIVE_SPAWN_WIRE;
        wire[17..21].copy_from_slice(&raw.to_le_bytes());
        let decoded = PassiveSpawn::decode(&wire);
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
        PassiveSpawn::new(0, vec![foreign]),
        Err(ProtocolError::InvalidEnum)
    );
}

#[test]
fn passive_spawn_invalid_value_wins_over_short_capacity() {
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
            "spawn dimension is foreign",
            ProtocolError::InvalidEnum,
            PASSIVE_SPAWN_WIRE.len(),
            vec![Box::new(move |dst: &mut [u8]| {
                PassiveSpawn {
                    server_tick: 0,
                    spawns: vec![over_dimension],
                }
                .encode_into(dst)
            })],
        ),
        (
            "spawn health is above the maximum",
            ProtocolError::InvalidRange,
            PASSIVE_SPAWN_WIRE.len(),
            vec![Box::new(move |dst: &mut [u8]| {
                PassiveSpawn {
                    server_tick: 0,
                    spawns: vec![over_health],
                }
                .encode_into(dst)
            })],
        ),
        (
            "spawn records are descending",
            ProtocolError::InvalidRange,
            PASSIVE_SPAWN_WIRE.len(),
            vec![Box::new(move |dst: &mut [u8]| reversed.encode_into(dst))],
        ),
        (
            "spawn batch carries a duplicate",
            ProtocolError::InvalidRange,
            8 + 1 + 3 * PASSIVE_SPAWN_WIRE_BYTES,
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
fn passive_state_invalid_value_wins_over_short_capacity() {
    let mut over_grazing = state_record_one();
    over_grazing.grazing = 2;
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
            "state grazing is outside the closed pair",
            ProtocolError::InvalidEnum,
            PASSIVE_STATE_WIRE.len(),
            Box::new(move |dst: &mut [u8]| {
                let mut batch = canonical_state();
                batch.states[0] = over_grazing;
                batch.encode_into(dst)
            }),
        ),
        (
            "state velocity is not finite",
            ProtocolError::InvalidFloat,
            PASSIVE_STATE_WIRE.len(),
            Box::new(move |dst: &mut [u8]| {
                let mut batch = canonical_state();
                batch.states[0] = over_velocity;
                batch.encode_into(dst)
            }),
        ),
        (
            "state health is zero",
            ProtocolError::InvalidRange,
            PASSIVE_STATE_WIRE.len(),
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
fn passive_despawn_invalid_value_wins_over_short_capacity() {
    let mut over_reason = canonical_despawn();
    over_reason.despawns[0].reason = 2;
    let mut duplicate = canonical_despawn();
    duplicate.despawns.push(PassiveDespawnRecord {
        id: passive_id(2),
        reason: PASSIVE_DESPAWN_DIED,
    });
    let mut reversed = canonical_despawn();
    reversed.despawns.reverse();
    let mut empty = canonical_despawn();
    empty.despawns.clear();

    let cases: Vec<(
        &str,
        ProtocolError,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "despawn reason is outside the closed pair",
            ProtocolError::InvalidEnum,
            PASSIVE_DESPAWN_WIRE.len(),
            Box::new(move |dst: &mut [u8]| over_reason.encode_into(dst)),
        ),
        (
            "despawn batch carries a duplicate",
            ProtocolError::InvalidRange,
            8 + 1 + 3 * PASSIVE_DESPAWN_WIRE_BYTES,
            Box::new(move |dst: &mut [u8]| duplicate.encode_into(dst)),
        ),
        (
            "despawn batch is descending",
            ProtocolError::InvalidRange,
            8 + 1 + 2 * PASSIVE_DESPAWN_WIRE_BYTES,
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
/// `encode` trusted the constructor, so a spawn record mutated into a foreign
/// dimension, health 21 or a zero identity after construction was published
/// byte for byte, as were a descending pair and an empty batch. The health-21
/// record published as
/// `[00, 00, 00, 00, 00, 00, 00, 00, 01, 07, 00, 00, 00, 00, 00, 00, 00, 00,
/// 00, 00, 00, 00, 00, 80, 3f, 00, 00, 00, 40, 00, 00, 40, 40, 00, 00, 00,
/// 3f, 15]` — health 21 in the trailing byte — and the grazing-2 state record
/// published `0a, 02` in its health and grazing bytes. A non-finite pose
/// reached the primitive's own refusal, which the previous surface turned into
/// a panic at its `expect`. The fallible surface turns every one of them into
/// a value error before any capacity decision.
#[test]
fn passive_mutated_public_fields_are_never_published() {
    let mut over_grazing = canonical_state();
    over_grazing.states[0].grazing = 2;
    assert_eq!(over_grazing.validate(), Err(ProtocolError::InvalidEnum));
    assert_eq!(over_grazing.encoded_len(), Err(ProtocolError::InvalidEnum));
    assert_eq!(over_grazing.encode(), Err(ProtocolError::InvalidEnum));

    let mut over_reason = canonical_despawn();
    over_reason.despawns[1].reason = 2;
    assert_eq!(over_reason.validate(), Err(ProtocolError::InvalidEnum));
    assert_eq!(over_reason.encoded_len(), Err(ProtocolError::InvalidEnum));
    assert_eq!(over_reason.encode(), Err(ProtocolError::InvalidEnum));

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
    empty.despawns.clear();
    assert_eq!(empty.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(empty.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(empty.encode(), Err(ProtocolError::InvalidRange));
}

#[test]
fn passive_decode_rejects_every_proper_truncation() {
    // All three canonical payloads are short enough for a full sweep, and every
    // cut answers at the truncation boundary: the tick, the one-byte count
    // prefix and the record region all report the same failure the Go decoder
    // reports for the same bytes.
    for cut in 0..PASSIVE_SPAWN_WIRE.len() {
        let error = PassiveSpawn::decode(&PASSIVE_SPAWN_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("spawn: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "spawn: truncation at {cut} bytes reports {error:?}"
        );
    }
    for cut in 0..PASSIVE_STATE_WIRE.len() {
        let error = PassiveState::decode(&PASSIVE_STATE_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("state: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "state: truncation at {cut} bytes reports {error:?}"
        );
    }
    for cut in 0..PASSIVE_DESPAWN_WIRE.len() {
        let error = PassiveDespawn::decode(&PASSIVE_DESPAWN_WIRE[..cut])
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
    for cut in [10usize, 9 + 29, 9 + 2 * 29 - 1, 9 + 64 * 29 - 1] {
        let error = PassiveSpawn::decode(&spawn[..cut])
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
        let error = PassiveState::decode(&state[..cut])
            .err()
            .unwrap_or_else(|| panic!("full state: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "full state: truncation at {cut} bytes reports {error:?}"
        );
    }
    let despawn = full_despawn()
        .encode()
        .expect("encode the full despawn batch");
    for cut in [10usize, 9 + 9, 9 + 2 * 9 - 1, 9 + 64 * 9 - 1] {
        let error = PassiveDespawn::decode(&despawn[..cut])
            .err()
            .unwrap_or_else(|| panic!("full despawn: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "full despawn: truncation at {cut} bytes reports {error:?}"
        );
    }
}

#[test]
fn passive_decode_rejects_one_trailing_byte() {
    // All three families apply the exact-remaining-length rule, so a one-byte
    // extra payload is answered by the length check before any record is read.
    // The Go decoder applies the same rule — its remaining-length message
    // precedes the record loop — so a padded payload reports the truncation
    // boundary on both sides rather than a trailing byte.
    let mut spawn = PASSIVE_SPAWN_WIRE.to_vec();
    spawn.push(0x00);
    assert_eq!(
        PassiveSpawn::decode(&spawn),
        Err(ProtocolError::Truncated),
        "a padded spawn payload reports the truncation boundary"
    );
    let mut state = PASSIVE_STATE_WIRE.to_vec();
    state.push(0x00);
    assert_eq!(
        PassiveState::decode(&state),
        Err(ProtocolError::Truncated),
        "a padded state payload reports the truncation boundary"
    );
    let mut despawn = PASSIVE_DESPAWN_WIRE.to_vec();
    despawn.push(0x00);
    assert_eq!(
        PassiveDespawn::decode(&despawn),
        Err(ProtocolError::Truncated),
        "a padded despawn payload reports the truncation boundary"
    );
    let mut full = full_despawn().encode().expect("encode the full batch");
    full.push(0x00);
    assert_eq!(PassiveDespawn::decode(&full), Err(ProtocolError::Truncated));
}

#[test]
fn passive_encode_into_leaves_a_short_destination_unchanged() {
    let cases: Vec<(
        &str,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "passive spawn",
            PASSIVE_SPAWN_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_spawn().encode_into(dst)),
        ),
        (
            "passive spawn at the ceiling",
            8 + 1 + MAX_PASSIVE_SPAWN_RECORDS as usize * PASSIVE_SPAWN_WIRE_BYTES,
            Box::new(|dst: &mut [u8]| full_spawn().encode_into(dst)),
        ),
        (
            "passive state",
            PASSIVE_STATE_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_state().encode_into(dst)),
        ),
        (
            "passive state at the ceiling",
            8 + 1 + MAX_PASSIVE_STATE_RECORDS as usize * PASSIVE_STATE_WIRE_BYTES,
            Box::new(|dst: &mut [u8]| full_state().encode_into(dst)),
        ),
        (
            "passive despawn",
            PASSIVE_DESPAWN_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_despawn().encode_into(dst)),
        ),
        (
            "passive despawn at the ceiling",
            8 + 1 + MAX_PASSIVE_RECORDS as usize * PASSIVE_DESPAWN_WIRE_BYTES,
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
fn passive_packet_ids_are_pinned() {
    assert_eq!(PassiveSpawn::PACKET_ID, 26);
    assert_eq!(PassiveState::PACKET_ID, 27);
    assert_eq!(PassiveDespawn::PACKET_ID, 28);
}
