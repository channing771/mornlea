//! The three companion publication families: the common fallible surface, the
//! shared identity gate, and the bounded batch.
//!
//! `CompanionSpawn` (S/Play/17), `CompanionStates` (S/Play/18) and
//! `CompanionDespawn` (S/Play/19) are the publications an authoritative
//! session sends about the companions one subscriber can see: the first full
//! body of a companion, its per-tick state batch, and its removal. Viewer
//! routing and companion scheduling stay with the authority, so the wire
//! carries no recipient, no visibility set and no task or persona state.
//!
//! All three records are concrete packets with public mutable fields, so the
//! group pins the design's surface order (`validate` → checked `encoded_len`
//! → capacity check → `publish_packet`): a short or invalid `encode_into`
//! leaves the caller's buffer untouched, an invalid value wins over a short
//! destination, and every proper truncation of a canonical payload rejects.
//! The wire literals are the Go encoder's output, so the round-trip tests pin
//! byte-level parity rather than self-agreement.
//!
//! The value gates keep the Go validator orders: the spawn's companion name,
//! then its overworld-only dimension, then the finite pose; the batch's count
//! bound, per-record pose and identity order; and the despawn's identity. A
//! companion is a member of the player's own party rather than a mirror of a
//! peer session, so unlike a remote player it appears in the overworld alone
//! and its pitch is bounded by the inclusive half-turn limit — exactly
//! ±pi/2 admits bit-exact and the next float above or below rejects, while the
//! yaw stays unrestricted in range.
//!
//! The identity is the domain's checked `CompanionId`, whose private
//! representation makes the absent zero form unconstructible on this surface:
//! `decode` refuses it at `InvalidIdentity`, and no public constructor accepts
//! raw bytes. The corpus evidence these families publish is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.

use mornlea_protocol::{
    COMPANION_SPAWN_MAX_WIRE_BYTES, COMPANION_STATE_WIRE_BYTES, COMPANION_STATES_MAX_WIRE_BYTES,
    CompanionDespawn, CompanionId, CompanionSpawn, CompanionState, CompanionStates,
    MAX_COMPANION_STATES, ProtocolError,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The reviewed 53-byte CompanionSpawn literal: the control companion
/// identity, the canonical companion name `Mira`, tick 0, the overworld, a
/// position carrying a negative zero, yaw 0 and the pitch at exactly half a
/// turn.
const COMPANION_SPAWN_WIRE: [u8; 53] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
    0x04, 0x4d, 0x69, 0x72, 0x61, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00,
    0x00, 0xdb, 0x0f, 0xc9, 0x3f,
];

/// The reviewed 16-byte CompanionDespawn literal: the control companion
/// identity alone.
const COMPANION_DESPAWN_WIRE: [u8; 16] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
];

/// The reviewed 50-byte CompanionStates literal: tick 0, one record at the
/// overworld, a position carrying a negative zero, the pitch at exactly half a
/// turn and the reset flag clear.
const COMPANION_STATES_WIRE: [u8; 50] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46,
    0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x00, 0xdb, 0x0f, 0xc9,
    0x3f, 0x00,
];

/// The reviewed 173-byte CompanionStates literal: the full companion-activity
/// batch of four ascending identities, which is exactly the fixed wire ceiling
/// the Go decoder applies before it allocates.
const COMPANION_STATES_FOUR_WIRE: [u8; 173] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40,
    0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80,
    0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x00, 0x00, 0xdb, 0x0f, 0xc9,
    0x3f, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x41, 0x00, 0x81, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
    0x40, 0x40, 0x00, 0x00, 0x00, 0x00, 0xdb, 0x0f, 0xc9, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x42, 0x00, 0x82, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x00, 0x00, 0xdb,
    0x0f, 0xc9, 0x3f, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x43, 0x00, 0x83, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40,
    0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x00, 0x00, 0xdb, 0x0f, 0xc9, 0x3f, 0x00,
];

/// The control companion identity used by every reviewed vector.
fn control_companion() -> CompanionId {
    CompanionId::try_from_bytes([
        0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e,
        0x0f,
    ])
    .expect("uuid v4")
}

/// One ascending identity of the full four-record batch.
fn ascending_companion(index: u8) -> CompanionId {
    CompanionId::try_from_bytes([
        0,
        0,
        0,
        0,
        0,
        0,
        0x40 + index,
        0,
        0x80 + index,
        0,
        0,
        0,
        0,
        0,
        0,
        0,
    ])
    .expect("uuid v4")
}

/// The canonical spawn the reviewed literal carries.
fn canonical_spawn() -> CompanionSpawn {
    CompanionSpawn::new(
        control_companion(),
        "Mira".to_owned(),
        0,
        mornlea_domain::Dimension::OVERWORLD,
        [-0.0, 1.0, 2.0],
        0.0,
        std::f32::consts::FRAC_PI_2,
    )
    .expect("the canonical spawn is valid")
}

/// The canonical despawn the reviewed literal carries.
fn canonical_despawn() -> CompanionDespawn {
    CompanionDespawn::new(control_companion())
}

/// One companion state record for the canonical one-record batch.
fn canonical_record() -> CompanionState {
    CompanionState {
        companion_id: control_companion(),
        dimension: mornlea_domain::Dimension::OVERWORLD,
        position: [-0.0, 1.0, 2.0],
        yaw: 0.0,
        pitch: std::f32::consts::FRAC_PI_2,
        reset: false,
    }
}

/// The canonical one-record batch the reviewed literal carries.
fn canonical_states() -> CompanionStates {
    CompanionStates::new(0, vec![canonical_record()]).expect("the canonical batch is valid")
}

/// The full four-record batch, the fixed companion-activity ceiling.
fn full_states() -> CompanionStates {
    let states: Vec<CompanionState> = (0..MAX_COMPANION_STATES as u8)
        .map(|index| CompanionState {
            companion_id: ascending_companion(index),
            dimension: mornlea_domain::Dimension::OVERWORLD,
            position: [1.0, 2.0, 3.0],
            yaw: 0.0,
            pitch: std::f32::consts::FRAC_PI_2,
            reset: index % 2 == 0,
        })
        .collect();
    CompanionStates::new(0, states).expect("the full batch is valid")
}

/// Asserts the shared surface contract for one canonical record: the reviewed
/// length, the exact window, the padded prefix, the allocating wrapper and the
/// decode/re-encode round trip.
///
/// The families differ in their record type but not in the surface, so the
/// helper takes the pieces it needs instead of duplicating the assertions.
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
    validate().unwrap_or_else(|err| panic!("{label}: valid record fails validation: {err:?}"));
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
        "{label}: decoded record differs from the encoded one"
    );
    let reencoded = re_encode().unwrap_or_else(|err| panic!("{label}: re-encode fails: {err:?}"));
    assert_eq!(
        reencoded, payload,
        "{label}: re-encoded payload differs from the reviewed bytes"
    );
}

#[test]
fn companions_spawn_round_trips_through_the_fallible_surface() {
    let spawn = canonical_spawn();
    let decoded = CompanionSpawn::decode(&COMPANION_SPAWN_WIRE).expect("decode");
    assert_round_trip(
        "companion spawn",
        &COMPANION_SPAWN_WIRE,
        || spawn.validate(),
        || spawn.encoded_len(),
        |dst| spawn.encode_into(dst),
        || spawn.encode(),
        |payload| CompanionSpawn::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == spawn,
    );
    assert_eq!(COMPANION_SPAWN_WIRE.len(), 16 + 1 + 4 + 8 + 4 + 12 + 4 + 4);
    assert_eq!(decoded.companion_id(), control_companion());
    assert_eq!(decoded.name, "Mira");
    assert_eq!(decoded.tick, 0);
    assert_eq!(decoded.dimension, mornlea_domain::Dimension::OVERWORLD);
    // The negative-zero component and the boundary pitch survive the round
    // trip bit for bit.
    assert_eq!(decoded.position[0].to_bits(), (-0.0f32).to_bits());
    assert_eq!(
        decoded.pitch.to_bits(),
        std::f32::consts::FRAC_PI_2.to_bits()
    );
}

#[test]
fn companions_despawn_round_trips_through_the_fallible_surface() {
    let despawn = canonical_despawn();
    let decoded = CompanionDespawn::decode(&COMPANION_DESPAWN_WIRE).expect("decode");
    assert_round_trip(
        "companion despawn",
        &COMPANION_DESPAWN_WIRE,
        || despawn.validate(),
        || despawn.encoded_len(),
        |dst| despawn.encode_into(dst),
        || despawn.encode(),
        |payload| CompanionDespawn::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == despawn,
    );
    assert_eq!(COMPANION_DESPAWN_WIRE.len(), 16);
    assert_eq!(decoded.companion, control_companion());
}

#[test]
fn companions_states_round_trips_through_the_fallible_surface() {
    let states = canonical_states();
    let decoded = CompanionStates::decode(&COMPANION_STATES_WIRE).expect("decode");
    assert_round_trip(
        "companion states",
        &COMPANION_STATES_WIRE,
        || states.validate(),
        || states.encoded_len(),
        |dst| states.encode_into(dst),
        || states.encode(),
        |payload| CompanionStates::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == states,
    );
    assert_eq!(
        COMPANION_STATES_WIRE.len(),
        8 + 1 + COMPANION_STATE_WIRE_BYTES
    );
    assert_eq!(decoded.tick, 0);
    assert_eq!(decoded.states.len(), 1);
    assert_eq!(
        decoded.states[0].dimension,
        mornlea_domain::Dimension::OVERWORLD
    );
    assert!(!decoded.states[0].reset);
    assert_eq!(decoded.states[0].position[0].to_bits(), (-0.0f32).to_bits());
    assert_eq!(
        decoded.states[0].pitch.to_bits(),
        std::f32::consts::FRAC_PI_2.to_bits()
    );
}

#[test]
fn companions_states_admit_the_full_batch_at_the_fixed_wire_ceiling() {
    // Four records of the fixed stride behind the eight-byte tick and the
    // one-byte count is exactly the ceiling, so the boundary admits on both
    // the size and the count side.
    let states = full_states();
    assert_eq!(states.states.len(), MAX_COMPANION_STATES as usize);
    assert_eq!(
        states.encoded_len().expect("encoded_len"),
        COMPANION_STATES_MAX_WIRE_BYTES
    );
    let wire = states.encode().expect("encode the full batch");
    assert_eq!(wire.len(), 173);
    assert_eq!(wire, COMPANION_STATES_FOUR_WIRE);
    let mut exact = vec![0u8; COMPANION_STATES_MAX_WIRE_BYTES];
    let written = states.encode_into(&mut exact).expect("encode_into");
    assert_eq!(written, COMPANION_STATES_MAX_WIRE_BYTES);
    assert_eq!(exact, COMPANION_STATES_FOUR_WIRE);
    let decoded = CompanionStates::decode(&COMPANION_STATES_FOUR_WIRE).expect("decode");
    assert_eq!(decoded, states);
    assert_eq!(
        COMPANION_STATES_MAX_WIRE_BYTES,
        8 + 1 + MAX_COMPANION_STATES as usize * COMPANION_STATE_WIRE_BYTES
    );
}

#[test]
fn companions_the_fixed_ceilings_refuse_before_any_field_is_read() {
    // The ceilings are pre-allocation guards, so a payload above them reports
    // the capacity refusal rather than the truncation or trailing boundary the
    // same payload would answer with below the ceiling. The Go decoder refuses
    // all three with its own fixed-maximum message, so the two sides publish
    // one boundary for the same bytes.
    let above_spawn_ceiling = vec![0u8; COMPANION_SPAWN_MAX_WIRE_BYTES + 1];
    let far_above_spawn_ceiling = vec![0u8; COMPANION_SPAWN_MAX_WIRE_BYTES + 64];
    assert_eq!(
        CompanionSpawn::decode(&above_spawn_ceiling),
        Err(ProtocolError::FrameTooLarge)
    );
    assert_eq!(
        CompanionSpawn::decode(&far_above_spawn_ceiling),
        Err(ProtocolError::FrameTooLarge)
    );
    let mut states_overshoot = COMPANION_STATES_FOUR_WIRE.to_vec();
    states_overshoot.push(0x00);
    assert_eq!(
        CompanionStates::decode(&states_overshoot),
        Err(ProtocolError::FrameTooLarge)
    );
    assert_eq!(
        CompanionStates::decode(&vec![0u8; COMPANION_STATES_MAX_WIRE_BYTES + 64]),
        Err(ProtocolError::FrameTooLarge)
    );
    // The despawn's bound equals its stride, so the boundary admits the exact
    // record and refuses one byte more.
    assert_eq!(
        CompanionDespawn::decode(&COMPANION_DESPAWN_WIRE).map(|_| ()),
        Ok(())
    );
    assert_eq!(
        CompanionDespawn::decode(&vec![0u8; COMPANION_DESPAWN_WIRE.len() + 1]),
        Err(ProtocolError::FrameTooLarge)
    );
}

#[test]
fn companions_count_bound_fires_before_the_record_scan() {
    // A declared count above the ceiling is answered at the count bound even
    // when the payload carries fewer records than declared, which is the order
    // the Go decoder applies: the count message precedes its length check.
    let mut count_five = COMPANION_STATES_WIRE.to_vec();
    count_five[8] = 5;
    assert_eq!(
        CompanionStates::decode(&count_five),
        Err(ProtocolError::InvalidRange)
    );
    let mut count_zero = COMPANION_STATES_WIRE.to_vec();
    count_zero[8] = 0;
    assert_eq!(
        CompanionStates::decode(&count_zero),
        Err(ProtocolError::InvalidRange)
    );
    // A declared count the payload cannot back is the exact-length boundary,
    // and a fifth record would exceed the fixed wire ceiling first, so the
    // count bound is the only place that boundary is observable.
    let mut mismatched = COMPANION_STATES_WIRE.to_vec();
    mismatched[8] = 2;
    assert_eq!(
        CompanionStates::decode(&mismatched),
        Err(ProtocolError::Truncated)
    );
}

#[test]
fn companions_pitch_limit_is_inclusive_on_both_ends() {
    // The half-turn limit is inclusive: exactly +pi/2 and -pi/2 publish their
    // exact bits, while the next float above or below is refused. The yaw
    // carries no vertical rule, so the full finite range stays wire-valid.
    let limit = std::f32::consts::FRAC_PI_2;
    let next_above = f32::from_bits(limit.to_bits() + 1);
    let next_below = f32::from_bits((-limit).to_bits() + 1);
    assert_eq!(limit.to_bits(), 0x3fc9_0fdb);
    assert_eq!((-limit).to_bits(), 0xbfc9_0fdb);
    assert_eq!(next_above.to_bits(), 0x3fc9_0fdc);
    assert_eq!(next_below.to_bits(), 0xbfc9_0fdc);

    for pitch in [limit, -limit, 0.0, -0.0, 1.0, -1.0] {
        let spawn = CompanionSpawn::new(
            control_companion(),
            "Mira".to_owned(),
            0,
            mornlea_domain::Dimension::OVERWORLD,
            [-0.0, 1.0, 2.0],
            0.0,
            pitch,
        )
        .expect("a pitch inside the inclusive limit is wire-valid");
        let decoded = CompanionSpawn::decode(&spawn.encode().expect("encode")).expect("decode");
        assert_eq!(decoded.pitch.to_bits(), pitch.to_bits());
        let states = CompanionStates::new(
            0,
            vec![CompanionState {
                pitch,
                ..canonical_record()
            }],
        )
        .expect("a pitch inside the inclusive limit is wire-valid");
        let decoded = CompanionStates::decode(&states.encode().expect("encode")).expect("decode");
        assert_eq!(decoded.states[0].pitch.to_bits(), pitch.to_bits());
    }

    for pitch in [next_above, next_below, 2.0, -2.0, f32::NAN, f32::INFINITY] {
        let spawn = CompanionSpawn::new(
            control_companion(),
            "Mira".to_owned(),
            0,
            mornlea_domain::Dimension::OVERWORLD,
            [-0.0, 1.0, 2.0],
            0.0,
            pitch,
        );
        assert!(
            spawn.is_err(),
            "pitch {pitch} was admitted by the spawn constructor"
        );
        assert!(
            CompanionStates::new(
                0,
                vec![CompanionState {
                    pitch,
                    ..canonical_record()
                }]
            )
            .is_err()
        );
    }

    // The yaw keeps its full finite range, including the two zeros and the
    // extreme magnitudes, and the batch carries the same unrestricted rotation
    // and tick span.
    let wide = CompanionSpawn::new(
        control_companion(),
        "Mira".to_owned(),
        u64::MAX,
        mornlea_domain::Dimension::OVERWORLD,
        [f32::MIN, f32::MAX, -0.0],
        -0.0,
        -std::f32::consts::FRAC_PI_2,
    )
    .expect("wide finite values are wire-valid");
    let wire = wide.encode().expect("encode");
    assert_eq!(&wire[21..29], &u64::MAX.to_le_bytes());
    let decoded = CompanionSpawn::decode(&wire).expect("decode");
    assert_eq!(decoded.tick, u64::MAX);
    assert_eq!(decoded.position[0].to_bits(), f32::MIN.to_bits());
    assert_eq!(decoded.position[1].to_bits(), f32::MAX.to_bits());
    assert_eq!(decoded.position[2].to_bits(), (-0.0f32).to_bits());
    assert_eq!(decoded.yaw.to_bits(), (-0.0f32).to_bits());
    let states = CompanionStates::new(
        u64::MAX,
        vec![CompanionState {
            yaw: -0.0,
            ..canonical_record()
        }],
    )
    .expect("wide finite values are wire-valid");
    let wire = states.encode().expect("encode");
    assert_eq!(&wire[..8], &u64::MAX.to_le_bytes());
    let decoded = CompanionStates::decode(&wire).expect("decode");
    assert_eq!(decoded.states[0].yaw.to_bits(), (-0.0f32).to_bits());
}

#[test]
fn companions_identity_order_is_unsigned_byte_order() {
    // The sort key is the raw 16-byte identity, not a display or map
    // ordering: the pair below is byte-ascending while its last-byte pair
    // (0x09, 0x10) reads as nine-then-sixteen, and its descending twin is
    // refused by both the constructor and the public-field surface.
    let low = CompanionId::try_from_bytes([
        0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e,
        0x09,
    ])
    .expect("uuid v4");
    let high = CompanionId::try_from_bytes([
        0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e,
        0x10,
    ])
    .expect("uuid v4");
    let low_record = CompanionState {
        companion_id: low,
        ..canonical_record()
    };
    let high_record = CompanionState {
        companion_id: high,
        ..canonical_record()
    };
    let ascending = CompanionStates::new(0, vec![low_record, high_record])
        .expect("byte-ascending identities are admitted");
    let decoded = CompanionStates::decode(&ascending.encode().expect("encode")).expect("decode");
    assert_eq!(decoded, ascending);
    assert!(
        CompanionStates::new(0, vec![high_record, low_record]).is_err(),
        "byte-descending identities must be refused"
    );
    let mut descending = CompanionStates {
        tick: 0,
        states: vec![high_record, low_record],
    };
    assert_eq!(descending.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(descending.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(descending.encode(), Err(ProtocolError::InvalidRange));
    descending.states.reverse();
    assert!(descending.validate().is_ok());
}

/// One mutate-after-construction case: the invalid value a public mutable
/// field is set to after construction, and the error the surface reports for
/// it before any size or capacity decision.
#[test]
fn companions_invalid_value_wins_over_short_capacity() {
    let mut spaced_name = canonical_spawn();
    spaced_name.name = "Mira Bell".to_owned();
    let mut padded_name = canonical_spawn();
    padded_name.name = " padded".to_owned();
    let mut empty_name = canonical_spawn();
    empty_name.name = String::new();
    let mut long_name = canonical_spawn();
    long_name.name = "x".repeat(33);
    let mut control_name = canonical_spawn();
    control_name.name = "a\u{0}b".to_owned();
    let mut foreign = canonical_spawn();
    foreign.dimension = mornlea_domain::Dimension::DEPTHS;
    let mut bad_pitch = canonical_spawn();
    bad_pitch.pitch = f32::from_bits(std::f32::consts::FRAC_PI_2.to_bits() + 1);
    let mut nan_yaw = canonical_spawn();
    nan_yaw.yaw = f32::NAN;
    let mut bad_position = canonical_spawn();
    bad_position.position = [f32::INFINITY, 0.0, 0.0];

    let mut record_foreign = canonical_states();
    record_foreign.states[0].dimension = mornlea_domain::Dimension::DEPTHS;
    let mut record_pitch = canonical_states();
    record_pitch.states[0].pitch = f32::from_bits(std::f32::consts::FRAC_PI_2.to_bits() + 1);
    let mut record_position = canonical_states();
    record_position.states[0].position = [f32::NAN, 0.0, 0.0];
    let mut duplicate = full_states();
    duplicate.states[1].companion_id = duplicate.states[0].companion_id;
    let mut reversed = full_states();
    reversed.states.reverse();
    let mut empty_batch = canonical_states();
    empty_batch.states.clear();

    let cases: Vec<(
        &str,
        ProtocolError,
        usize,
        Vec<Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>>,
    )> = vec![
        (
            "spawn name carries an embedded space",
            ProtocolError::InvalidString,
            COMPANION_SPAWN_WIRE.len(),
            {
                let record = spaced_name.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn name is padded",
            ProtocolError::InvalidString,
            COMPANION_SPAWN_WIRE.len(),
            {
                let record = padded_name.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn name is empty",
            ProtocolError::InvalidString,
            COMPANION_SPAWN_WIRE.len(),
            {
                let record = empty_name.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn name exceeds the rune bound",
            ProtocolError::InvalidString,
            COMPANION_SPAWN_WIRE.len(),
            {
                let record = long_name;
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn name carries a control character",
            ProtocolError::InvalidString,
            COMPANION_SPAWN_WIRE.len(),
            {
                let record = control_name;
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn names a foreign dimension",
            ProtocolError::InvalidEnum,
            COMPANION_SPAWN_WIRE.len(),
            {
                let record = foreign.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn pitch is above the inclusive limit",
            ProtocolError::InvalidFloat,
            COMPANION_SPAWN_WIRE.len(),
            {
                let record = bad_pitch.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn yaw is NaN",
            ProtocolError::InvalidFloat,
            COMPANION_SPAWN_WIRE.len(),
            {
                let record = nan_yaw.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn position is infinite",
            ProtocolError::InvalidFloat,
            COMPANION_SPAWN_WIRE.len(),
            {
                let record = bad_position.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "state names a foreign dimension",
            ProtocolError::InvalidEnum,
            COMPANION_STATES_WIRE.len(),
            {
                let record = record_foreign.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "state pitch is above the inclusive limit",
            ProtocolError::InvalidFloat,
            COMPANION_STATES_WIRE.len(),
            {
                let record = record_pitch.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "state position is NaN",
            ProtocolError::InvalidFloat,
            COMPANION_STATES_WIRE.len(),
            {
                let record = record_position.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "states carry a duplicate identity",
            ProtocolError::InvalidRange,
            COMPANION_STATES_FOUR_WIRE.len(),
            {
                let record = duplicate.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "states are descending",
            ProtocolError::InvalidRange,
            COMPANION_STATES_FOUR_WIRE.len(),
            {
                let record = reversed.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "states batch is empty",
            ProtocolError::InvalidRange,
            COMPANION_STATES_WIRE.len(),
            {
                let record = empty_batch;
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
    ];

    for (label, error, needed, encoders) in cases {
        for encode_into in &encoders {
            // The value error is reported for every destination shape, so an
            // invalid record never reaches the capacity decision.
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
            // The exact destination still refuses the value error.
            let mut exact = vec![0u8; needed];
            assert_eq!(
                encode_into(&mut exact),
                Err(error),
                "{label}: the exact destination still refuses the value error"
            );
            assert!(
                exact.iter().all(|byte| *byte == 0),
                "{label}: the exact destination was modified on a value refusal"
            );
        }
    }

    // Every allocating wrapper refuses the same mutations, so no entry point
    // publishes a mutated record.
    assert_eq!(spaced_name.encode(), Err(ProtocolError::InvalidString));
    assert_eq!(padded_name.encode(), Err(ProtocolError::InvalidString));
    assert_eq!(foreign.encode(), Err(ProtocolError::InvalidEnum));
    assert_eq!(bad_pitch.encode(), Err(ProtocolError::InvalidFloat));
    assert_eq!(nan_yaw.encode(), Err(ProtocolError::InvalidFloat));
    assert_eq!(bad_position.encode(), Err(ProtocolError::InvalidFloat));
    assert_eq!(record_foreign.encode(), Err(ProtocolError::InvalidEnum));
    assert_eq!(record_pitch.encode(), Err(ProtocolError::InvalidFloat));
    assert_eq!(record_position.encode(), Err(ProtocolError::InvalidFloat));
    assert_eq!(duplicate.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(reversed.encode(), Err(ProtocolError::InvalidRange));
}

/// The silent-encode red this node closes, quoted from the previous surface.
///
/// A spawn mutated into the embedded-space name `"Mira Bell"` after
/// construction published its name bytes silently, and a spawn mutated into
/// the next float above the half-turn pitch published that angle silently,
/// because the previous encoder applied no value gate of its own — it trusted
/// the constructor and only panicked at its `expect` when the name slot
/// overflowed the 128-byte primitive bound. Both payloads are refused by the
/// Go validator, so the value gate now runs on every encode instead of only
/// at construction.
#[test]
fn companions_mutated_public_fields_are_never_published() {
    let mut spaced = canonical_spawn();
    spaced.name = "Mira Bell".to_owned();
    assert_eq!(spaced.validate(), Err(ProtocolError::InvalidString));
    assert_eq!(spaced.encoded_len(), Err(ProtocolError::InvalidString));
    assert_eq!(spaced.encode(), Err(ProtocolError::InvalidString));

    let mut bad_pitch = canonical_spawn();
    bad_pitch.pitch = f32::from_bits(std::f32::consts::FRAC_PI_2.to_bits() + 1);
    assert_eq!(bad_pitch.validate(), Err(ProtocolError::InvalidFloat));
    assert_eq!(bad_pitch.encoded_len(), Err(ProtocolError::InvalidFloat));
    assert_eq!(bad_pitch.encode(), Err(ProtocolError::InvalidFloat));
}

/// The absent companion identity, on the wire and on this surface.
///
/// The plan's named red: `decode` refuses the zero UUID at `InvalidIdentity`
/// on all three families, and the Go DTO shape has no Rust counterpart because
/// `CompanionId` is a checked domain newtype — `try_from_bytes` refuses both
/// the exact zero record and a non-UUIDv4 value, so no public construction
/// path accepts the absent form and the divergence cannot be reintroduced by
/// a later caller. The zero identity a never-addressed chat event carries
/// stays a raw wire value in that family and never becomes a packet here.
#[test]
fn companions_absent_identity_is_unconstructible_and_decode_refuses_it() {
    let mut zero_spawn = COMPANION_SPAWN_WIRE;
    for byte in &mut zero_spawn[..16] {
        *byte = 0;
    }
    assert_eq!(
        CompanionSpawn::decode(&zero_spawn),
        Err(ProtocolError::InvalidIdentity)
    );
    let mut zero_despawn = COMPANION_DESPAWN_WIRE;
    for byte in &mut zero_despawn[..16] {
        *byte = 0;
    }
    assert_eq!(
        CompanionDespawn::decode(&zero_despawn),
        Err(ProtocolError::InvalidIdentity)
    );
    let mut zero_states = COMPANION_STATES_WIRE;
    for byte in &mut zero_states[9..25] {
        *byte = 0;
    }
    assert_eq!(
        CompanionStates::decode(&zero_states),
        Err(ProtocolError::InvalidIdentity)
    );

    assert!(CompanionId::try_from_bytes([0; 16]).is_err());
    let mut not_v4: [u8; 16] = COMPANION_DESPAWN_WIRE;
    not_v4[6] &= 0x0f;
    assert!(CompanionId::try_from_bytes(not_v4).is_err());
}

/// The latent cross-implementation boundary class.
///
/// The Go `CompanionSpawn.Validate` and `CompanionState.validate` fold the
/// identity, the dimension and the pose into one predicate each with the
/// single messages `network: invalid companion spawn` and `invalid companion
/// state`, so a zero or non-UUIDv4 identity and the depths dimension cannot
/// publish distinct categories on the Go side. This group therefore pins them
/// at the Rust variants instead of registering corpus cases for them, and
/// records them as the same latent class the remote-player and furnace
/// families carry: a spawn or record naming dimension 1 is `InvalidEnum`,
/// because companions are overworld-only. The depths value itself stays a
/// constructible domain value, so unlike the identity this boundary is
/// expressible on the outbound surface and is pinned there as well; the zero
/// identity is decode-only because `CompanionId` refuses it.
#[test]
fn companions_spawn_latent_boundaries_are_pinned_here_not_in_the_corpus() {
    let mut dimension_one = COMPANION_SPAWN_WIRE;
    dimension_one[29] = 1;
    assert_eq!(
        CompanionSpawn::decode(&dimension_one),
        Err(ProtocolError::InvalidEnum),
        "a companion in the depths is the enum boundary"
    );
    let mut record_dimension_one = COMPANION_STATES_WIRE;
    record_dimension_one[25] = 1;
    assert_eq!(
        CompanionStates::decode(&record_dimension_one),
        Err(ProtocolError::InvalidEnum),
        "a companion state in the depths is the enum boundary"
    );
    // The outbound surface can name the depths, and the packet gate refuses it
    // on both families while the identity has no such value to name.
    let depths = mornlea_domain::Dimension::new(1).expect("the depths is a real dimension");
    let mut foreign = canonical_spawn();
    foreign.dimension = depths;
    assert_eq!(foreign.encode(), Err(ProtocolError::InvalidEnum));
    let mut foreign_record = canonical_states();
    foreign_record.states[0].dimension = depths;
    assert_eq!(foreign_record.encode(), Err(ProtocolError::InvalidEnum));
    assert!(mornlea_domain::Dimension::new(2).is_err());
}

#[test]
fn companions_decode_rejects_every_proper_truncation() {
    // The spawn's name slot is the one variable field: a declared length the
    // payload cannot complete is the string boundary on both sides, matching
    // the Go `byteDecoder.string` sentinel, while every other cut is the
    // short-input boundary.
    for cut in 0..COMPANION_SPAWN_WIRE.len() {
        let error = CompanionSpawn::decode(&COMPANION_SPAWN_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("spawn: truncation at {cut} bytes decoded"));
        let want = if (17..=20).contains(&cut) {
            ProtocolError::InvalidString
        } else {
            ProtocolError::Truncated
        };
        assert_eq!(
            error, want,
            "spawn: truncation at {cut} bytes reports {error:?}"
        );
    }
    // The despawn is a fixed stride, so every cut is the short-input boundary.
    for cut in 0..COMPANION_DESPAWN_WIRE.len() {
        let error = CompanionDespawn::decode(&COMPANION_DESPAWN_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("despawn: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "despawn: truncation at {cut} bytes reports {error:?}"
        );
    }
    // The batch's count fires before the record scan, and a cut inside the
    // record region is the short-payload boundary the Go decoder answers with
    // its own remaining-length message.
    for cut in 0..COMPANION_STATES_WIRE.len() {
        let error = CompanionStates::decode(&COMPANION_STATES_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("states: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "states: truncation at {cut} bytes reports {error:?}"
        );
    }
}

#[test]
fn companions_decode_rejects_one_trailing_byte() {
    let mut spawn = COMPANION_SPAWN_WIRE.to_vec();
    spawn.push(0x00);
    assert_eq!(
        CompanionSpawn::decode(&spawn),
        Err(ProtocolError::TrailingBytes)
    );
    let mut despawn = COMPANION_DESPAWN_WIRE.to_vec();
    despawn.push(0x00);
    assert_eq!(
        CompanionDespawn::decode(&despawn),
        Err(ProtocolError::FrameTooLarge)
    );
    // The companion batch is the one family whose Go decoder applies an exact
    // remaining-length rule rather than a minimum-records rule, so a trailing
    // byte reports the truncation boundary on both sides instead of the
    // trailing-byte boundary a remote-player batch would publish.
    let mut states = COMPANION_STATES_WIRE.to_vec();
    states.push(0x00);
    assert_eq!(
        CompanionStates::decode(&states),
        Err(ProtocolError::Truncated)
    );
}

#[test]
fn companions_encode_into_leaves_a_short_destination_unchanged() {
    let cases: Vec<(
        &str,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "companion spawn",
            COMPANION_SPAWN_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_spawn().encode_into(dst)),
        ),
        (
            "companion despawn",
            COMPANION_DESPAWN_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_despawn().encode_into(dst)),
        ),
        (
            "companion states",
            COMPANION_STATES_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_states().encode_into(dst)),
        ),
        (
            "companion states at the ceiling",
            COMPANION_STATES_FOUR_WIRE.len(),
            Box::new(|dst: &mut [u8]| full_states().encode_into(dst)),
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
fn companions_packet_ids_are_pinned() {
    assert_eq!(CompanionSpawn::PACKET_ID, 17);
    assert_eq!(CompanionStates::PACKET_ID, 18);
    assert_eq!(CompanionDespawn::PACKET_ID, 19);
}
