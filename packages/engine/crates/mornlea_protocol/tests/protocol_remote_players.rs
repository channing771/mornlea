//! The three remote-player publication families: the common fallible surface,
//! the shared identity gate, and the batch bounds.
//!
//! `RemotePlayerSpawn` (S/Play/7), `RemotePlayerDespawn` (S/Play/8) and
//! `RemotePlayerStates` (S/Play/9) are the peer-session publications an
//! authoritative session sends about the other players one subscriber can
//! see: the first full body of a peer, its removal, and the per-tick state
//! batch. Viewer routing and subscription gating stay with the authority, so
//! the wire carries no recipient, no visibility set and no private survival
//! state.
//!
//! All three records are concrete packets with public mutable fields, so the
//! group pins the design's surface order (`validate` → checked `encoded_len`
//! → capacity check → `publish_packet`): a short or invalid `encode_into`
//! leaves the caller's buffer untouched, an invalid value wins over a short
//! destination, and every proper truncation of a canonical payload rejects.
//! The wire literals are the Go encoder's output, so the round-trip tests pin
//! byte-level parity rather than self-agreement.
//!
//! The value gates keep the Go validator orders: the spawn's canonical
//! display name before the combined pose finiteness, the despawn's identity,
//! and the batch's count bound, per-record finiteness and identity order.
//! Pitch is unrestricted here — a peer session may mirror any finite look
//! angle, so the companion vertical-look rule is deliberately not imported.
//! The identity is the domain's checked `PlayerId`, whose private
//! representation makes a zero or non-UUIDv4 value unconstructible on this
//! surface.
//!
//! The category split this group pins follows the Go validators: a padded or
//! otherwise non-canonical name and a non-finite pose report
//! `InvalidString`/`InvalidFloat`, an unknown dimension reports `InvalidEnum`,
//! and a count, duplicate or descending identity reports `InvalidRange`. The
//! corpus evidence these families publish is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.

use mornlea_protocol::{
    MAX_REMOTE_PLAYER_STATES, PlayerId, ProtocolError, REMOTE_PLAYER_STATE_WIRE_BYTES,
    REMOTE_PLAYER_STATES_MAX_WIRE_BYTES, RemotePlayerDespawn, RemotePlayerSpawn, RemotePlayerState,
    RemotePlayerStates,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The reviewed 54-byte RemotePlayerSpawn literal: the control player
/// identity, the canonical name `Alice`, tick 0, the depths dimension, a
/// position carrying a negative zero, yaw 0 and the wire-valid pitch 2.0.
const REMOTE_PLAYER_SPAWN_WIRE: [u8; 54] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
    0x05, 0x41, 0x6c, 0x69, 0x63, 0x65, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x40,
];

/// The reviewed 16-byte RemotePlayerDespawn literal: the control player
/// identity alone.
const REMOTE_PLAYER_DESPAWN_WIRE: [u8; 16] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
];

/// The reviewed 91-byte RemotePlayerStates literal: tick 0, two strictly
/// increasing identities, the overworld and the depths, both poses carrying a
/// negative zero, and the two reset flags.
const REMOTE_PLAYER_STATES_WIRE: [u8; 91] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46,
    0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x40, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d,
    0x0e, 0x10, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0xc0, 0xbf, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x01,
];

/// The reviewed 296-byte RemotePlayerStates literal: the full peer-session
/// batch of seven ascending identities, which is exactly the fixed wire
/// ceiling the Go decoder applies before it allocates.
const REMOTE_PLAYER_STATES_SEVEN_WIRE: [u8; 296] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x07, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40,
    0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80,
    0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x40, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x41, 0x00, 0x81, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
    0x40, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x42, 0x00, 0x82, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x40, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x43, 0x00, 0x83, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40,
    0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x44, 0x00, 0x84, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x40, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x00, 0x85, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00,
    0x00, 0x40, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x46, 0x00, 0x86, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x40, 0x40, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x01,
];

/// The control player identity used since the negotiation nodes.
fn control_player() -> PlayerId {
    PlayerId::try_from_bytes([
        0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e,
        0x0f,
    ])
    .expect("uuid v4")
}

/// The control identity's byte-successor: the same prefix with the last byte
/// incremented, so the pair is strictly ascending in unsigned byte order.
fn next_player() -> PlayerId {
    PlayerId::try_from_bytes([
        0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e,
        0x10,
    ])
    .expect("uuid v4")
}

/// The canonical spawn the reviewed literal carries.
fn canonical_spawn() -> RemotePlayerSpawn {
    RemotePlayerSpawn::new(
        control_player(),
        "Alice".to_owned(),
        0,
        mornlea_domain::Dimension::DEPTHS,
        [-0.0, 1.0, 2.0],
        0.0,
        2.0,
    )
    .expect("the canonical spawn is valid")
}

/// The canonical despawn the reviewed literal carries.
fn canonical_despawn() -> RemotePlayerDespawn {
    RemotePlayerDespawn::new(control_player())
}

/// One peer state record for the canonical two-record batch.
fn canonical_record(index: usize) -> RemotePlayerState {
    let (player_id, dimension, position, reset) = match index {
        0 => (
            control_player(),
            mornlea_domain::Dimension::OVERWORLD,
            [-0.0, 1.0, 2.0],
            false,
        ),
        _ => (
            next_player(),
            mornlea_domain::Dimension::DEPTHS,
            [-0.0, -1.5, 0.0],
            true,
        ),
    };
    RemotePlayerState {
        player_id,
        dimension,
        position,
        yaw: 0.0,
        pitch: 2.0,
        reset,
    }
}

/// The canonical two-record batch the reviewed literal carries.
fn canonical_states() -> RemotePlayerStates {
    RemotePlayerStates::new(0, vec![canonical_record(0), canonical_record(1)])
        .expect("the canonical batch is valid")
}

/// One ascending identity of the full seven-record batch.
fn ascending_player(index: u8) -> PlayerId {
    PlayerId::try_from_bytes([
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

/// The full seven-record batch, the fixed peer-session ceiling.
fn full_states() -> RemotePlayerStates {
    let players: Vec<RemotePlayerState> = (0..MAX_REMOTE_PLAYER_STATES as u8)
        .map(|index| RemotePlayerState {
            player_id: ascending_player(index),
            dimension: mornlea_domain::Dimension::OVERWORLD,
            position: [1.0, 2.0, 3.0],
            yaw: 0.0,
            pitch: 2.0,
            reset: index % 2 == 0,
        })
        .collect();
    RemotePlayerStates::new(0, players).expect("the full batch is valid")
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
fn remote_players_spawn_round_trips_through_the_fallible_surface() {
    let spawn = canonical_spawn();
    let decoded = RemotePlayerSpawn::decode(&REMOTE_PLAYER_SPAWN_WIRE).expect("decode");
    assert_round_trip(
        "remote player spawn",
        &REMOTE_PLAYER_SPAWN_WIRE,
        || spawn.validate(),
        || spawn.encoded_len(),
        |dst| spawn.encode_into(dst),
        || spawn.encode(),
        |payload| RemotePlayerSpawn::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == spawn,
    );
    assert_eq!(REMOTE_PLAYER_SPAWN_WIRE.len(), 54);
    assert_eq!(decoded.player_id(), control_player());
    assert_eq!(decoded.display_name, "Alice");
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.dimension, mornlea_domain::Dimension::DEPTHS);
    // The negative-zero component survives the round trip bit for bit.
    assert_eq!(decoded.position[0].to_bits(), (-0.0f32).to_bits());
    assert_eq!(decoded.pitch.to_bits(), 2.0f32.to_bits());
}

#[test]
fn remote_players_despawn_round_trips_through_the_fallible_surface() {
    let despawn = canonical_despawn();
    let decoded = RemotePlayerDespawn::decode(&REMOTE_PLAYER_DESPAWN_WIRE).expect("decode");
    assert_round_trip(
        "remote player despawn",
        &REMOTE_PLAYER_DESPAWN_WIRE,
        || despawn.validate(),
        || despawn.encoded_len(),
        |dst| despawn.encode_into(dst),
        || despawn.encode(),
        |payload| RemotePlayerDespawn::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == despawn,
    );
    assert_eq!(REMOTE_PLAYER_DESPAWN_WIRE.len(), 16);
    assert_eq!(decoded.player, control_player());
}

#[test]
fn remote_players_states_round_trips_through_the_fallible_surface() {
    let states = canonical_states();
    let decoded = RemotePlayerStates::decode(&REMOTE_PLAYER_STATES_WIRE).expect("decode");
    assert_round_trip(
        "remote player states",
        &REMOTE_PLAYER_STATES_WIRE,
        || states.validate(),
        || states.encoded_len(),
        |dst| states.encode_into(dst),
        || states.encode(),
        |payload| RemotePlayerStates::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == states,
    );
    assert_eq!(
        REMOTE_PLAYER_STATES_WIRE.len(),
        8 + 1 + 2 * REMOTE_PLAYER_STATE_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.players.len(), 2);
    assert_eq!(
        decoded.players[0].dimension,
        mornlea_domain::Dimension::OVERWORLD
    );
    assert_eq!(
        decoded.players[1].dimension,
        mornlea_domain::Dimension::DEPTHS
    );
    assert!(!decoded.players[0].reset);
    assert!(decoded.players[1].reset);
    assert_eq!(
        decoded.players[0].position[0].to_bits(),
        (-0.0f32).to_bits()
    );
}

#[test]
fn remote_players_states_admit_the_full_batch_at_the_fixed_wire_ceiling() {
    // Seven records of the fixed stride behind the eight-byte tick and the
    // one-byte count is exactly the ceiling, so the boundary admits on both
    // the size and the count side.
    let states = full_states();
    assert_eq!(states.players.len(), MAX_REMOTE_PLAYER_STATES as usize);
    assert_eq!(
        states.encoded_len().expect("encoded_len"),
        REMOTE_PLAYER_STATES_MAX_WIRE_BYTES
    );
    let wire = states.encode().expect("encode the full batch");
    assert_eq!(wire.len(), 296);
    assert_eq!(wire, REMOTE_PLAYER_STATES_SEVEN_WIRE);
    let mut exact = vec![0u8; REMOTE_PLAYER_STATES_MAX_WIRE_BYTES];
    let written = states.encode_into(&mut exact).expect("encode_into");
    assert_eq!(written, REMOTE_PLAYER_STATES_MAX_WIRE_BYTES);
    assert_eq!(exact, REMOTE_PLAYER_STATES_SEVEN_WIRE);
    let decoded = RemotePlayerStates::decode(&REMOTE_PLAYER_STATES_SEVEN_WIRE).expect("decode");
    assert_eq!(decoded, states);
    assert_eq!(
        REMOTE_PLAYER_STATES_MAX_WIRE_BYTES,
        8 + 1 + MAX_REMOTE_PLAYER_STATES as usize * REMOTE_PLAYER_STATE_WIRE_BYTES
    );
}

#[test]
fn remote_players_states_the_fixed_ceiling_refuses_before_any_record_is_read() {
    // The ceiling is a pre-allocation guard, so one byte above it reports the
    // capacity refusal rather than the trailing-byte boundary the same payload
    // would answer with below the ceiling.
    let mut overshoot = REMOTE_PLAYER_STATES_SEVEN_WIRE.to_vec();
    overshoot.push(0x00);
    assert_eq!(
        RemotePlayerStates::decode(&overshoot),
        Err(ProtocolError::FrameTooLarge)
    );
    let far = vec![0u8; REMOTE_PLAYER_STATES_MAX_WIRE_BYTES + 64];
    assert_eq!(
        RemotePlayerStates::decode(&far),
        Err(ProtocolError::FrameTooLarge)
    );
    // A valid count whose records the payload cannot back refuses at the
    // count bound, not at the record-length rule: the Go decoder answers the
    // count first, so the two sides publish one boundary for the same bytes.
    let mut count_eight = vec![0u8; 8];
    count_eight.push(8);
    count_eight.extend_from_slice(&REMOTE_PLAYER_STATES_WIRE[9..]);
    assert_eq!(
        RemotePlayerStates::decode(&count_eight),
        Err(ProtocolError::InvalidRange)
    );
    let mut count_zero = vec![0u8; 8];
    count_zero.push(0);
    assert_eq!(
        RemotePlayerStates::decode(&count_zero),
        Err(ProtocolError::InvalidRange)
    );
}

#[test]
fn remote_players_pitch_and_the_finite_range_stay_wire_valid() {
    // Pitch 2.0 is the reviewed vector, and a peer pose carries no vertical
    // look rule: the companion limit is a different family's rule and is
    // deliberately not imported here.
    for pitch in [
        2.0f32,
        100.0,
        -100.0,
        std::f32::consts::FRAC_PI_2,
        0.0,
        -0.0,
    ] {
        let spawn = RemotePlayerSpawn::new(
            control_player(),
            "Alice".to_owned(),
            0,
            mornlea_domain::Dimension::OVERWORLD,
            [-0.0, 1.0, 2.0],
            0.0,
            pitch,
        )
        .expect("a finite pitch is wire-valid");
        let decoded = RemotePlayerSpawn::decode(&spawn.encode().expect("encode")).expect("decode");
        assert_eq!(decoded.pitch.to_bits(), pitch.to_bits());
    }
    // The full finite magnitude range and the two zeros keep their exact bits.
    let wide = RemotePlayerSpawn::new(
        control_player(),
        "Alice".to_owned(),
        u64::MAX,
        mornlea_domain::Dimension::DEPTHS,
        [f32::MIN, f32::MAX, -0.0],
        -0.0,
        2.0,
    )
    .expect("wide finite values are wire-valid");
    let wire = wide.encode().expect("encode");
    assert_eq!(&wire[22..30], &u64::MAX.to_le_bytes());
    let decoded = RemotePlayerSpawn::decode(&wire).expect("decode");
    assert_eq!(decoded.server_tick, u64::MAX);
    assert_eq!(decoded.position[0].to_bits(), f32::MIN.to_bits());
    assert_eq!(decoded.position[1].to_bits(), f32::MAX.to_bits());
    assert_eq!(decoded.position[2].to_bits(), (-0.0f32).to_bits());
    assert_eq!(decoded.yaw.to_bits(), (-0.0f32).to_bits());
    // The batch carries the same unrestricted rotation and tick span.
    let states = RemotePlayerStates::new(
        u64::MAX,
        vec![RemotePlayerState {
            yaw: -0.0,
            pitch: -100.0,
            ..canonical_record(0)
        }],
    )
    .expect("wide finite values are wire-valid");
    let wire = states.encode().expect("encode");
    assert_eq!(&wire[..8], &u64::MAX.to_le_bytes());
    let decoded = RemotePlayerStates::decode(&wire).expect("decode");
    assert_eq!(decoded.players[0].yaw.to_bits(), (-0.0f32).to_bits());
    assert_eq!(decoded.players[0].pitch.to_bits(), (-100.0f32).to_bits());
}

#[test]
fn remote_players_identity_order_is_unsigned_byte_order() {
    // The sort key is the raw 16-byte identity, not a display or map
    // ordering: the pair below is byte-ascending while its last-byte pair
    // (0x09, 0x10) reads as nine-then-sixteen, and its descending twin is
    // refused by both the constructor and the public-field surface.
    let low = PlayerId::try_from_bytes([
        0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e,
        0x09,
    ])
    .expect("uuid v4");
    let high = PlayerId::try_from_bytes([
        0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e,
        0x10,
    ])
    .expect("uuid v4");
    let low_record = RemotePlayerState {
        player_id: low,
        ..canonical_record(0)
    };
    let high_record = RemotePlayerState {
        player_id: high,
        ..canonical_record(1)
    };
    let ascending = RemotePlayerStates::new(0, vec![low_record, high_record])
        .expect("byte-ascending identities are admitted");
    let decoded = RemotePlayerStates::decode(&ascending.encode().expect("encode")).expect("decode");
    assert_eq!(decoded, ascending);
    // The descending twin is refused at the constructor and through the
    // public fields after construction.
    assert!(
        RemotePlayerStates::new(0, vec![high_record, low_record]).is_err(),
        "byte-descending identities must be refused"
    );
    let mut descending = RemotePlayerStates {
        server_tick: 0,
        players: vec![high_record, low_record],
    };
    assert_eq!(descending.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(descending.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(descending.encode(), Err(ProtocolError::InvalidRange));
    descending.players.reverse();
    assert!(descending.validate().is_ok());
}

/// One mutate-after-construction case: the invalid value a public mutable
/// field is set to after construction, and the error the surface reports for
/// it before any size or capacity decision.
#[test]
fn remote_players_invalid_value_wins_over_short_capacity() {
    let mut bad_pitch = canonical_spawn();
    bad_pitch.pitch = f32::NAN;
    let mut bad_position = canonical_spawn();
    bad_position.position = [f32::INFINITY, 0.0, 0.0];
    let mut bad_yaw = canonical_spawn();
    bad_yaw.yaw = f32::NEG_INFINITY;
    let mut padded_name = canonical_spawn();
    padded_name.display_name = " Alice ".to_owned();
    let mut empty_name = canonical_spawn();
    empty_name.display_name = String::new();
    let mut control_name = canonical_spawn();
    control_name.display_name = "a\u{0}b".to_owned();
    let mut long_name = canonical_spawn();
    long_name.display_name = "x".repeat(33);

    let mut duplicate = canonical_states();
    duplicate.players[1].player_id = control_player();
    let mut reversed = canonical_states();
    reversed.players.reverse();
    let mut bad_coordinate = canonical_states();
    bad_coordinate.players[0].position = [f32::NAN, 0.0, 0.0];
    let mut bad_pitch_record = canonical_states();
    bad_pitch_record.players[0].pitch = f32::INFINITY;
    let mut empty_batch = canonical_states();
    empty_batch.players.clear();
    let mut over_full = full_states();
    let extra = over_full.players[0];
    over_full.players.push(extra);

    let cases: Vec<(
        &str,
        ProtocolError,
        usize,
        Vec<Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>>,
    )> = vec![
        (
            "spawn pitch is NaN",
            ProtocolError::InvalidFloat,
            REMOTE_PLAYER_SPAWN_WIRE.len(),
            {
                let record = bad_pitch.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn position is infinite",
            ProtocolError::InvalidFloat,
            REMOTE_PLAYER_SPAWN_WIRE.len(),
            {
                let record = bad_position.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn yaw is negative infinity",
            ProtocolError::InvalidFloat,
            REMOTE_PLAYER_SPAWN_WIRE.len(),
            {
                let record = bad_yaw.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn name is padded",
            ProtocolError::InvalidString,
            REMOTE_PLAYER_SPAWN_WIRE.len(),
            {
                let record = padded_name.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn name is empty",
            ProtocolError::InvalidString,
            REMOTE_PLAYER_SPAWN_WIRE.len(),
            {
                let record = empty_name.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn name carries a control character",
            ProtocolError::InvalidString,
            REMOTE_PLAYER_SPAWN_WIRE.len(),
            {
                let record = control_name.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "spawn name exceeds the rune bound",
            ProtocolError::InvalidString,
            REMOTE_PLAYER_SPAWN_WIRE.len(),
            {
                let record = long_name;
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "states carry a duplicate identity",
            ProtocolError::InvalidRange,
            REMOTE_PLAYER_STATES_WIRE.len(),
            {
                let record = duplicate.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "states are descending",
            ProtocolError::InvalidRange,
            REMOTE_PLAYER_STATES_WIRE.len(),
            {
                let record = reversed.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "states carry a non-finite coordinate",
            ProtocolError::InvalidFloat,
            REMOTE_PLAYER_STATES_WIRE.len(),
            {
                let record = bad_coordinate.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "states carry a non-finite pitch",
            ProtocolError::InvalidFloat,
            REMOTE_PLAYER_STATES_WIRE.len(),
            {
                let record = bad_pitch_record.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "states batch is empty",
            ProtocolError::InvalidRange,
            REMOTE_PLAYER_STATES_WIRE.len(),
            {
                let record = empty_batch;
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "states batch exceeds the ceiling",
            ProtocolError::InvalidRange,
            REMOTE_PLAYER_STATES_SEVEN_WIRE.len(),
            {
                let record = over_full;
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
    // publishes a mutated record. The NaN pitch is the behavioral red this
    // surface closes: the previous infallible encoder panicked on it, and the
    // padded name and the duplicate or descending identities were published
    // silently, because their bytes crossed no encoder-side rule.
    assert_eq!(bad_pitch.encode(), Err(ProtocolError::InvalidFloat));
    assert_eq!(bad_position.encode(), Err(ProtocolError::InvalidFloat));
    assert_eq!(bad_yaw.encode(), Err(ProtocolError::InvalidFloat));
    assert_eq!(padded_name.encode(), Err(ProtocolError::InvalidString));
    assert_eq!(duplicate.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(reversed.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_coordinate.encode(), Err(ProtocolError::InvalidFloat));
    assert_eq!(bad_pitch_record.encode(), Err(ProtocolError::InvalidFloat));
}

/// The silent-encode red this node closes, quoted from the previous surface.
///
/// A spawn mutated into the non-canonical name `" Alice "` after construction
/// published 56 bytes whose name slot was `07 20 41 6c 69 63 65 20`
/// (`" Alice "`), and a two-record batch mutated into duplicate identities
/// published 91 bytes whose leading 25 bytes were
/// `000000000000000002000102030405460788090a0b0c0d0e0f0001020304054607`
/// — the same identity twice behind a count of two. Both payloads are
/// refused by the Go validator, so the value gate now runs on every encode
/// instead of only at construction. A NaN pitch did not publish silently: the
/// previous encoder panicked at its `expect` instead of reporting a value
/// error, which the fallible surface turns into `InvalidFloat`.
#[test]
fn remote_players_mutated_public_fields_are_never_published() {
    let mut padded = canonical_spawn();
    padded.display_name = " Alice ".to_owned();
    assert_eq!(padded.validate(), Err(ProtocolError::InvalidString));
    assert_eq!(padded.encoded_len(), Err(ProtocolError::InvalidString));
    assert_eq!(padded.encode(), Err(ProtocolError::InvalidString));
    assert_eq!(padded.encode(), Err(ProtocolError::InvalidString));

    let mut duplicate = canonical_states();
    duplicate.players[1].player_id = control_player();
    assert_eq!(duplicate.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(duplicate.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(duplicate.encode(), Err(ProtocolError::InvalidRange));
}

/// The latent cross-implementation boundary class.
///
/// The Go `RemotePlayerSpawn.Validate` folds the identity, the name, the
/// dimension and the pose finiteness into one predicate with the single
/// message `network: invalid remote player spawn`, so those boundaries cannot
/// publish distinct categories on the Go side. This group therefore pins them
/// at the Rust variants instead of registering corpus cases for them: a zero
/// or non-UUIDv4 identity is `InvalidIdentity` and an unknown dimension is
/// `InvalidEnum`, both on the decode path. The encode path cannot express
/// either value at all — `PlayerId` and `Dimension` are checked domain
/// newtypes, so the Go DTO's construct-then-validate shape has no Rust
/// counterpart — which is the same latent class the furnace family records
/// for its unregistered-item boundary.
#[test]
fn remote_players_spawn_latent_boundaries_are_pinned_here_not_in_the_corpus() {
    let mut zero_uuid = REMOTE_PLAYER_SPAWN_WIRE;
    for byte in &mut zero_uuid[..16] {
        *byte = 0;
    }
    assert_eq!(
        RemotePlayerSpawn::decode(&zero_uuid),
        Err(ProtocolError::InvalidIdentity),
        "a zero identity is the identity boundary"
    );

    let mut wrong_version = REMOTE_PLAYER_SPAWN_WIRE;
    wrong_version[6] &= 0x0f;
    assert_eq!(
        RemotePlayerSpawn::decode(&wrong_version),
        Err(ProtocolError::InvalidIdentity),
        "a non-UUIDv4 identity is the identity boundary"
    );

    let mut dimension_two = REMOTE_PLAYER_SPAWN_WIRE;
    dimension_two[30] = 2;
    assert_eq!(
        RemotePlayerSpawn::decode(&dimension_two),
        Err(ProtocolError::InvalidEnum),
        "an unknown dimension is the enum boundary"
    );

    // The same values are unconstructible on the outbound surface, so the
    // latent class is decode-only on this side.
    assert!(PlayerId::try_from_bytes([0; 16]).is_err());
    assert!(
        PlayerId::try_from_bytes({
            let mut not_v4: [u8; 16] = REMOTE_PLAYER_SPAWN_WIRE[..16]
                .to_owned()
                .try_into()
                .unwrap();
            not_v4[6] = REMOTE_PLAYER_SPAWN_WIRE[6] & 0x0f;
            not_v4
        })
        .is_err()
    );
    assert!(mornlea_domain::Dimension::new(2).is_err());
}

#[test]
fn remote_players_decode_rejects_every_proper_truncation() {
    // The spawn's name slot is the one variable field: a declared length the
    // payload cannot complete is the string boundary on both sides, matching
    // the Go `byteDecoder.string` sentinel, while every other cut is the
    // short-input boundary.
    for cut in 0..REMOTE_PLAYER_SPAWN_WIRE.len() {
        let error = RemotePlayerSpawn::decode(&REMOTE_PLAYER_SPAWN_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("spawn: truncation at {cut} bytes decoded"));
        let want = if (17..=21).contains(&cut) {
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
    for cut in 0..REMOTE_PLAYER_DESPAWN_WIRE.len() {
        let error = RemotePlayerDespawn::decode(&REMOTE_PLAYER_DESPAWN_WIRE[..cut])
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
    for cut in 0..REMOTE_PLAYER_STATES_WIRE.len() {
        let error = RemotePlayerStates::decode(&REMOTE_PLAYER_STATES_WIRE[..cut])
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
fn remote_players_decode_rejects_one_trailing_byte() {
    let mut spawn = REMOTE_PLAYER_SPAWN_WIRE.to_vec();
    spawn.push(0x00);
    assert_eq!(
        RemotePlayerSpawn::decode(&spawn),
        Err(ProtocolError::TrailingBytes)
    );
    let mut despawn = REMOTE_PLAYER_DESPAWN_WIRE.to_vec();
    despawn.push(0x00);
    assert_eq!(
        RemotePlayerDespawn::decode(&despawn),
        Err(ProtocolError::TrailingBytes)
    );
    let mut states = REMOTE_PLAYER_STATES_WIRE.to_vec();
    states.push(0x00);
    assert_eq!(
        RemotePlayerStates::decode(&states),
        Err(ProtocolError::TrailingBytes)
    );
}

#[test]
fn remote_players_encode_into_leaves_a_short_destination_unchanged() {
    let cases: Vec<(
        &str,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "remote player spawn",
            REMOTE_PLAYER_SPAWN_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_spawn().encode_into(dst)),
        ),
        (
            "remote player despawn",
            REMOTE_PLAYER_DESPAWN_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_despawn().encode_into(dst)),
        ),
        (
            "remote player states",
            REMOTE_PLAYER_STATES_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_states().encode_into(dst)),
        ),
        (
            "remote player states at the ceiling",
            REMOTE_PLAYER_STATES_SEVEN_WIRE.len(),
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
fn remote_players_packet_ids_are_pinned() {
    assert_eq!(RemotePlayerSpawn::PACKET_ID, 7);
    assert_eq!(RemotePlayerDespawn::PACKET_ID, 8);
    assert_eq!(RemotePlayerStates::PACKET_ID, 9);
}
