//! The four owner-private packet families: the common fallible surface, the
//! explicit reject-reason translation, and the value unions that make the
//! player-state record publishable.
//!
//! `PlayerState` (S/Play/3), `CommandRejected` (S/Play/4),
//! `PlaceBlockSucceeded` (S/Play/20) and `CombatHit` (S/Play/25) are the
//! records an authoritative tick addresses to the session that caused them:
//! the 93-byte body, survival and world-time publication, the refused-command
//! answer, the placement acknowledgement and the melee confirmation. Routing
//! and damage settlement are later authority concerns; the wire carries no
//! recipient, so this suite pins the payloads alone.
//!
//! All four records are concrete packets with public mutable fields, so the
//! group pins the design's surface order (`validate` → checked `encoded_len`
//! → capacity check → `publish_packet`): a short or invalid `encode_into`
//! leaves the caller's buffer untouched, an invalid value wins over a short
//! destination, and every proper truncation of a canonical payload rejects.
//! The wire literals are the Go encoder's output, so the round-trip tests pin
//! byte-level parity rather than self-agreement.
//!
//! `CommandRejected` carries the one translation this node owns: the Go
//! internal reject-reason enum runs `0..14` while the wire enum runs `1..15`,
//! so the reason is never a discriminant cast. `reject_reason_to_wire` and
//! `reject_reason_from_wire` are closed matches over the fifteen matrix rows,
//! compile-time exhaustive over the domain enum, and the group pins every row
//! in both directions against the Go `CommandRejectReasonID` table.
//!
//! The category split this group pins follows the Go validators: the mining
//! union, the survival and world-time ranges, the non-finite vectors and the
//! combat tick and damage bounds report `InvalidRange`, while the weather,
//! season, reject-reason and combat-target-kind boundaries report
//! `InvalidEnum`. Both sides publish one category for the same bytes. The
//! corpus evidence these families publish is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.

use mornlea_domain::{Dimension, RejectReason};
use mornlea_protocol::{
    BlockPos, COMBAT_TARGET_HOSTILE, COMBAT_TARGET_PASSIVE, COMBAT_TARGET_PLAYER, CombatHit,
    CommandRejected, MAX_ARMOR_POINTS, MAX_HEALTH, MAX_HUNGER, MAX_OXYGEN_TICKS,
    PLAYER_STATE_WIRE_BYTES, PlaceBlockSucceeded, ProtocolError, SEASON_WINTER, WEATHER_THUNDER,
    reject_reason_from_wire, reject_reason_to_wire,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// Copies the canonical player-state literal and hands it to one boundary
/// mutator, so a single-violation wire mutation is named by its mutator.
fn mutate_player_state_wire(mutator: impl FnOnce(&mut [u8])) -> Vec<u8> {
    let mut wire = PLAYER_STATE_WIRE.to_vec();
    mutator(&mut wire);
    wire
}

/// Copies the canonical combat-hit literal and hands it to one boundary
/// mutator, the same single-violation discipline.
fn mutate_combat_hit_wire(mutator: impl FnOnce(&mut [u8])) -> Vec<u8> {
    let mut wire = COMBAT_HIT_WIRE.to_vec();
    mutator(&mut wire);
    wire
}

/// The reviewed 93-byte PlayerState literal: zero server tick, input sequence
/// and world time, dimension 1 (Depths), a negative-zero position X and
/// pitch, an exact-zero inactive mining block, and every named boundary
/// maximum — health 20, oxygen 300, hunger 20, day phase offset 23999,
/// weather 2, season 3, in-season progress 255, temperature −128 and armor
/// points 20.
const PLAYER_STATE_WIRE: [u8; PLAYER_STATE_WIRE_BYTES] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // server tick
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // last input sequence
    0x01, 0x00, 0x00, 0x00, // dimension 1 (Depths)
    0x00, 0x00, 0x00, 0x80, // position X = -0.0
    0x00, 0x00, 0x00, 0x00, // position Y
    0x00, 0x00, 0x00, 0x00, // position Z
    0x00, 0x00, 0x00, 0x00, // velocity X
    0x00, 0x00, 0x00, 0x00, // velocity Y
    0x00, 0x00, 0x00, 0x00, // velocity Z
    0x00, 0x00, 0x00, 0x00, // yaw
    0x00, 0x00, 0x00, 0x80, // pitch = -0.0
    0x00, 0x00, 0x00, 0x00, // on ground, ready, reset, mining inactive
    0x00, 0x00, 0x00, 0x00, // mining target X
    0x00, 0x00, 0x00, 0x00, // mining target Y
    0x00, 0x00, 0x00, 0x00, // mining target Z
    0x00, 0x00, // mining progress
    0x00, 0x00, // mining requirement
    0x00, // mining harvestable
    0x14, // health 20
    0x2c, 0x01, // oxygen 300
    0x14, // hunger 20
    0x00, // saturation zero
    0xbf, 0x5d, // day phase offset 23999 (0x5dbf, little-endian)
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // world time ticks
    0x02, // weather 2 (thunder)
    0x03, // season 3 (winter)
    0xff, // season progress 255
    0x80, // temperature -128
    0x14, // armor points 20
];

/// The reviewed 9-byte CommandRejected literal: sequence 0 and reject reason
/// 1, which is the wire value the Go encoder publishes for `RejectInvalidRay`.
const COMMAND_REJECTED_WIRE: [u8; 9] = [0, 0, 0, 0, 0, 0, 0, 0, 0x01];

/// The reviewed 8-byte PlaceBlockSucceeded literal: the acknowledged
/// sequence 0, which this family admits.
const PLACE_BLOCK_SUCCEEDED_WIRE: [u8; 8] = [0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11];

/// The reviewed 10-byte CombatHit literal: server tick 1, damage 1 and
/// target kind 1, every field at its admitted lower boundary.
const COMBAT_HIT_WIRE: [u8; 10] = [0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01];

/// The canonical player state the reviewed literal carries.
fn canonical_player_state() -> mornlea_protocol::PlayerState {
    mornlea_protocol::PlayerState::new(
        0,
        0,
        Dimension::DEPTHS,
        [-0.0, 0.0, 0.0],
        [0.0, 0.0, 0.0],
        0.0,
        -0.0,
        false,
        false,
        false,
        false,
        BlockPos::ZERO,
        0,
        0,
        false,
        MAX_HEALTH,
        MAX_OXYGEN_TICKS,
        MAX_HUNGER,
        false,
        23999,
        0,
        WEATHER_THUNDER,
        SEASON_WINTER,
        255,
        -128,
        MAX_ARMOR_POINTS,
    )
    .expect("the canonical player state is valid")
}

/// The canonical command rejection the reviewed literal carries.
fn canonical_command_rejected() -> CommandRejected {
    CommandRejected::new(0, 1).expect("wire reason 1 is the admitted boundary")
}

/// The canonical placement acknowledgement the reviewed literal carries.
fn canonical_place_block_succeeded() -> PlaceBlockSucceeded {
    PlaceBlockSucceeded::new(0x1122_3344_5566_7788)
}

/// The canonical combat hit the reviewed literal carries.
fn canonical_combat_hit() -> CombatHit {
    CombatHit::new(1, 1, COMBAT_TARGET_PLAYER).expect("the canonical combat hit is valid")
}

/// The active mining variant: a non-zero target, progress 1 below requirement
/// 2 and a true harvestable flag, on the same canonical survival values.
fn active_mining_player_state() -> mornlea_protocol::PlayerState {
    let mut state = canonical_player_state();
    state.mining_active = true;
    state.mining_target = BlockPos { x: 1, y: 2, z: 3 };
    state.mining_progress_ticks = 1;
    state.mining_required_ticks = 2;
    state.mining_harvestable = true;
    state
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
fn player_outcomes_player_state_round_trips_through_the_fallible_surface() {
    let record = canonical_player_state();
    let decoded = mornlea_protocol::PlayerState::decode(&PLAYER_STATE_WIRE)
        .expect("decode the canonical payload");
    assert_round_trip(
        "player state",
        &PLAYER_STATE_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| mornlea_protocol::PlayerState::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(PLAYER_STATE_WIRE.len(), 93);
    // The negative-zero words keep their exact bits through the round trip.
    assert_eq!(decoded.position[0].to_bits(), 0x8000_0000);
    assert_eq!(decoded.pitch.to_bits(), 0x8000_0000);
    assert_eq!(decoded.health, MAX_HEALTH);
    assert_eq!(decoded.oxygen, MAX_OXYGEN_TICKS);
    assert_eq!(decoded.temperature, -128);
    assert_eq!(decoded.armor_points, MAX_ARMOR_POINTS);
}

#[test]
fn player_outcomes_player_state_active_mining_round_trips() {
    let record = active_mining_player_state();
    let decoded = mornlea_protocol::PlayerState::decode(&PLAYER_STATE_WIRE)
        .expect("decode the canonical payload");
    assert!(record.validate().is_ok());
    assert_eq!(record.mining_target, BlockPos { x: 1, y: 2, z: 3 });
    assert_eq!(record.mining_progress_ticks, 1);
    assert_eq!(record.mining_required_ticks, 2);
    assert!(record.mining_harvestable);
    // The inactive canonical vector stays publishable beside the active one,
    // and the active record re-encodes to its own reviewed bytes.
    let active_wire = record.encode().expect("encode the active record");
    let active_decoded =
        mornlea_protocol::PlayerState::decode(&active_wire).expect("decode the active payload");
    assert_eq!(active_decoded, record);
    assert_eq!(
        active_decoded
            .encode()
            .expect("re-encode the active record"),
        active_wire
    );
    // The canonical literal stays the inactive vector, so the two records
    // differ exactly in the mining block.
    assert!(!decoded.mining_active);
    assert_ne!(active_wire, PLAYER_STATE_WIRE.to_vec());
}

#[test]
fn player_outcomes_command_rejected_round_trips_through_the_fallible_surface() {
    let record = canonical_command_rejected();
    let decoded = CommandRejected::decode(&COMMAND_REJECTED_WIRE).expect("decode");
    assert_round_trip(
        "command rejected",
        &COMMAND_REJECTED_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| CommandRejected::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(COMMAND_REJECTED_WIRE.len(), 9);
    assert_eq!(decoded.sequence, 0);
    assert_eq!(decoded.reason, 1);
}

#[test]
fn player_outcomes_place_block_succeeded_round_trips_through_the_fallible_surface() {
    let record = canonical_place_block_succeeded();
    let decoded = PlaceBlockSucceeded::decode(&PLACE_BLOCK_SUCCEEDED_WIRE).expect("decode");
    assert_round_trip(
        "place block succeeded",
        &PLACE_BLOCK_SUCCEEDED_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| PlaceBlockSucceeded::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(PLACE_BLOCK_SUCCEEDED_WIRE.len(), 8);
    assert_eq!(decoded.sequence, 0x1122_3344_5566_7788);
}

#[test]
fn player_outcomes_combat_hit_round_trips_through_the_fallible_surface() {
    let record = canonical_combat_hit();
    let decoded = CombatHit::decode(&COMBAT_HIT_WIRE).expect("decode");
    assert_round_trip(
        "combat hit",
        &COMBAT_HIT_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| CombatHit::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(COMBAT_HIT_WIRE.len(), 10);
    assert_eq!(decoded.server_tick, 1);
    assert_eq!(decoded.damage, 1);
    assert_eq!(decoded.target_kind, COMBAT_TARGET_PLAYER);
}

/// The fifteen reviewed matrix rows: every domain reason beside the wire value
/// the Go `CommandRejectReasonID` table publishes for it.
fn reject_reason_matrix() -> [(RejectReason, u8); 15] {
    [
        (RejectReason::InvalidRay, 1),
        (RejectReason::NoTarget, 2),
        (RejectReason::ChunkNotReady, 3),
        (RejectReason::ProtectedBlock, 4),
        (RejectReason::InvalidBlock, 5),
        (RejectReason::Occupied, 6),
        (RejectReason::InvalidInput, 7),
        (RejectReason::PlayerNotReady, 8),
        (RejectReason::InvalidSlot, 9),
        (RejectReason::HotbarFull, 10),
        (RejectReason::DropCapacity, 11),
        (RejectReason::ContainerCapacity, 12),
        (RejectReason::NotFluidSource, 13),
        (RejectReason::BucketMismatch, 14),
        (RejectReason::NotArmor, 15),
    ]
}

#[test]
fn player_outcomes_the_reject_reason_matrix_translates_both_ways() {
    // The explicit translation is closed over the fifteen matrix rows, so a
    // new domain variant has to change the match itself rather than ride a
    // discriminant cast or a catch-all arm.
    for (reason, wire) in reject_reason_matrix() {
        assert_eq!(
            reject_reason_to_wire(reason),
            Ok(wire),
            "{reason:?}: the outbound translation publishes the wrong wire value"
        );
        assert_eq!(
            reject_reason_from_wire(wire),
            Ok(reason),
            "wire {wire}: the inbound translation names the wrong reason"
        );
        // The domain table is the same mapping, not a second one: the two
        // closed matches agree on every row.
        assert_eq!(
            reject_reason_to_wire(reason),
            Ok(reason.wire_id()),
            "{reason:?}: the translation disagrees with the domain wire_id"
        );
        // Each row survives the packet round trip, so the translation is the
        // packet surface's own reason rather than a side table.
        let packet = CommandRejected::new(1, wire).expect("reason");
        assert_eq!(packet.reason, wire);
        let decoded = CommandRejected::decode(&packet.encode().expect("encode"))
            .expect("decode the reason row");
        assert_eq!(decoded.reason, wire);
    }
}

#[test]
fn player_outcomes_the_reject_reason_interval_boundaries_are_refused() {
    for wire in [0u8, 16, 255] {
        assert_eq!(
            reject_reason_from_wire(wire),
            Err(ProtocolError::InvalidEnum),
            "wire {wire} is outside the closed interval"
        );
        assert_eq!(
            CommandRejected::new(1, wire),
            Err(ProtocolError::InvalidEnum),
            "wire {wire} cannot be published on the packet"
        );
        let mut payload = COMMAND_REJECTED_WIRE.to_vec();
        payload[8] = wire;
        assert_eq!(
            CommandRejected::decode(&payload),
            Err(ProtocolError::InvalidEnum),
            "wire {wire} is refused on the decode path too"
        );
    }
}

#[test]
fn player_outcomes_temperature_and_pitch_are_never_clipped() {
    // The full i8 range is legal in both directions, and the pitch carries no
    // protocol-level clamp: a wide angle keeps its exact bits.
    for temperature in [i8::MIN, -1, 0, 1, i8::MAX] {
        let mut state = canonical_player_state();
        state.temperature = temperature;
        assert!(
            state.validate().is_ok(),
            "temperature {temperature} is a legal i8"
        );
        let wire = state.encode().expect("encode the temperature variant");
        let decoded = mornlea_protocol::PlayerState::decode(&wire).expect("decode the variant");
        assert_eq!(decoded.temperature, temperature);
        assert_eq!(wire[91], temperature as u8);
    }
    let mut wide = canonical_player_state();
    wide.pitch = 100.0;
    assert!(
        wide.validate().is_ok(),
        "the pitch carries no protocol clamp"
    );
    let wire = wide.encode().expect("encode the wide pitch");
    let decoded = mornlea_protocol::PlayerState::decode(&wire).expect("decode the wide pitch");
    assert_eq!(decoded.pitch.to_bits(), 100.0f32.to_bits());
}

#[test]
fn player_outcomes_the_mining_union_is_validated_as_one_unit() {
    // An inactive block must be entirely empty, so any single non-zero
    // residue field refuses the whole record.
    for (label, mutate) in [
        (
            "non-zero target",
            Box::new(|state: &mut mornlea_protocol::PlayerState| {
                state.mining_target = BlockPos { x: 1, y: 0, z: 0 };
            }) as Box<dyn Fn(&mut mornlea_protocol::PlayerState)>,
        ),
        (
            "non-zero progress",
            Box::new(|state: &mut mornlea_protocol::PlayerState| {
                state.mining_progress_ticks = 1;
            }),
        ),
        (
            "non-zero requirement",
            Box::new(|state: &mut mornlea_protocol::PlayerState| {
                state.mining_required_ticks = 1;
            }),
        ),
        (
            "true harvestable",
            Box::new(|state: &mut mornlea_protocol::PlayerState| {
                state.mining_harvestable = true;
            }),
        ),
    ] {
        let mut state = canonical_player_state();
        mutate(&mut state);
        assert_eq!(
            state.validate(),
            Err(ProtocolError::InvalidRange),
            "{label}: an inactive mining block must be entirely empty"
        );
        assert_eq!(
            state.encoded_len(),
            Err(ProtocolError::InvalidRange),
            "{label}: the size decision never sees an invalid record"
        );
        assert_eq!(
            state.encode(),
            Err(ProtocolError::InvalidRange),
            "{label}: encode refuses the residue"
        );
    }

    // An active block reports progress strictly below the requirement, so a
    // completed swing has to be published as inactive.
    for (progress, required) in [(0u16, 2u16), (1, 1), (2, 1)] {
        let mut state = canonical_player_state();
        state.mining_active = true;
        state.mining_target = BlockPos { x: 1, y: 2, z: 3 };
        state.mining_progress_ticks = progress;
        state.mining_required_ticks = required;
        state.mining_harvestable = true;
        assert_eq!(
            state.validate(),
            Err(ProtocolError::InvalidRange),
            "active progress {progress} of required {required} is not publishable"
        );
    }
    // The admitted active window is 0 < progress < required.
    for (progress, required) in [(1u16, 2u16), (1, u16::MAX)] {
        let mut state = canonical_player_state();
        state.mining_active = true;
        state.mining_target = BlockPos { x: 1, y: 2, z: 3 };
        state.mining_progress_ticks = progress;
        state.mining_required_ticks = required;
        state.mining_harvestable = true;
        assert!(
            state.validate().is_ok(),
            "active progress {progress} below required {required} is publishable"
        );
    }
}

/// One mutate-after-construction case per family: the invalid value a public
/// mutable field is set to after construction, and the error the surface has
/// to report for it before any size or capacity decision.
#[test]
fn player_outcomes_invalid_value_wins_over_short_capacity() {
    let mut bad_health = canonical_player_state();
    bad_health.health = MAX_HEALTH + 1;
    let mut bad_position = canonical_player_state();
    bad_position.position[0] = f32::NAN;
    let mut bad_weather = canonical_player_state();
    bad_weather.weather_kind = WEATHER_THUNDER + 1;
    let mut bad_reason = canonical_command_rejected();
    bad_reason.reason = 0;
    let mut bad_tick = canonical_combat_hit();
    bad_tick.server_tick = 0;
    let mut bad_kind = canonical_combat_hit();
    bad_kind.target_kind = 0;

    let cases: Vec<(
        &str,
        ProtocolError,
        Vec<Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>>,
        usize,
    )> = vec![
        (
            "player state health above the maximum",
            ProtocolError::InvalidRange,
            {
                let record = bad_health.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
            PLAYER_STATE_WIRE_BYTES,
        ),
        (
            "player state non-finite position",
            ProtocolError::InvalidFloat,
            {
                let record = bad_position.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
            PLAYER_STATE_WIRE_BYTES,
        ),
        (
            "player state weather above the maximum",
            ProtocolError::InvalidEnum,
            {
                let record = bad_weather.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
            PLAYER_STATE_WIRE_BYTES,
        ),
        (
            "command rejected reason zero",
            ProtocolError::InvalidEnum,
            {
                let record = bad_reason;
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
            COMMAND_REJECTED_WIRE.len(),
        ),
        (
            "combat hit zero server tick",
            ProtocolError::InvalidRange,
            {
                let record = bad_tick;
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
            COMBAT_HIT_WIRE.len(),
        ),
        (
            "combat hit unknown target kind",
            ProtocolError::InvalidEnum,
            {
                let record = bad_kind;
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
            COMBAT_HIT_WIRE.len(),
        ),
    ];

    for (label, error, encoders, needed) in cases {
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
            // The allocating wrapper refuses the same record, so a mutated
            // field cannot bypass the gate by choosing the other entry point.
            let mut exact = vec![0u8; needed];
            assert_eq!(
                encode_into(&mut exact),
                Err(error),
                "{label}: the exact destination still refuses the value error"
            );
        }
    }

    assert_eq!(bad_health.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_position.validate(), Err(ProtocolError::InvalidFloat));
    assert_eq!(bad_weather.validate(), Err(ProtocolError::InvalidEnum));
    assert_eq!(
        canonical_command_rejected().validate(),
        Ok(()),
        "the unmutated rejection record stays valid"
    );
    assert_eq!(bad_tick.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_kind.validate(), Err(ProtocolError::InvalidEnum));
    let mut zero_reason = bad_reason;
    assert_eq!(zero_reason.validate(), Err(ProtocolError::InvalidEnum));
    zero_reason.reason = 1;
    assert_eq!(zero_reason.validate(), Ok(()));
}

#[test]
fn player_outcomes_decode_rejects_every_proper_truncation() {
    let cases: Vec<(&str, Vec<u8>, fn(&[u8]) -> Result<(), ProtocolError>)> = vec![
        ("player state", PLAYER_STATE_WIRE.to_vec(), |payload| {
            mornlea_protocol::PlayerState::decode(payload).map(|_| ())
        }),
        (
            "command rejected",
            COMMAND_REJECTED_WIRE.to_vec(),
            |payload| CommandRejected::decode(payload).map(|_| ()),
        ),
        (
            "place block succeeded",
            PLACE_BLOCK_SUCCEEDED_WIRE.to_vec(),
            |payload| PlaceBlockSucceeded::decode(payload).map(|_| ()),
        ),
        ("combat hit", COMBAT_HIT_WIRE.to_vec(), |payload| {
            CombatHit::decode(payload).map(|_| ())
        }),
    ];
    for (label, payload, decode) in cases {
        // Cut 0 is the empty payload, where the first fixed-width read reports
        // a missing byte. Every cut is the same boundary the Go decoder
        // answers with its short-input sentinel.
        for cut in 0..payload.len() {
            let error = decode(&payload[..cut])
                .err()
                .unwrap_or_else(|| panic!("{label}: truncation at {cut} bytes decoded"));
            assert_eq!(
                error,
                ProtocolError::Truncated,
                "{label}: truncation at {cut} bytes reports {error:?}"
            );
        }
    }
}

#[test]
fn player_outcomes_decode_rejects_one_trailing_byte() {
    let cases: Vec<(&str, Vec<u8>, fn(&[u8]) -> Result<(), ProtocolError>)> = vec![
        (
            "player state",
            {
                let mut wire = PLAYER_STATE_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| mornlea_protocol::PlayerState::decode(payload).map(|_| ()),
        ),
        (
            "command rejected",
            {
                let mut wire = COMMAND_REJECTED_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| CommandRejected::decode(payload).map(|_| ()),
        ),
        (
            "place block succeeded",
            {
                let mut wire = PLACE_BLOCK_SUCCEEDED_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| PlaceBlockSucceeded::decode(payload).map(|_| ()),
        ),
        (
            "combat hit",
            {
                let mut wire = COMBAT_HIT_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| CombatHit::decode(payload).map(|_| ()),
        ),
    ];
    for (label, payload, decode) in cases {
        let error = decode(&payload)
            .err()
            .unwrap_or_else(|| panic!("{label}: one trailing byte decoded"));
        assert_eq!(
            error,
            ProtocolError::TrailingBytes,
            "{label}: one trailing byte reports {error:?}"
        );
    }
}

#[test]
fn player_outcomes_decode_rejects_the_pinned_invalid_values() {
    // The matrix mirrors the corpus negatives. Every entry is a single
    // violation, and the variant the Rust decoder reports is the one the Go
    // producer records for the same bytes.
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&mutate_player_state_wire(|wire| {
            wire[20..24].copy_from_slice(&0x7fc0_0000u32.to_le_bytes());
        })),
        Err(ProtocolError::InvalidFloat),
        "a NaN position word"
    );
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&mutate_player_state_wire(|wire| {
            wire[73] = 21;
        })),
        Err(ProtocolError::InvalidRange),
        "health above the maximum"
    );
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&mutate_player_state_wire(|wire| {
            wire[74..76].copy_from_slice(&301u16.to_le_bytes());
        })),
        Err(ProtocolError::InvalidRange),
        "oxygen above the maximum"
    );
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&mutate_player_state_wire(|wire| {
            wire[76] = 21;
        })),
        Err(ProtocolError::InvalidRange),
        "hunger above the maximum"
    );
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&mutate_player_state_wire(|wire| {
            wire[78..80].copy_from_slice(&24000u16.to_le_bytes());
        })),
        Err(ProtocolError::InvalidRange),
        "day phase offset at the bound"
    );
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&mutate_player_state_wire(|wire| {
            wire[88] = 3;
        })),
        Err(ProtocolError::InvalidEnum),
        "weather above the maximum"
    );
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&mutate_player_state_wire(|wire| {
            wire[89] = 4;
        })),
        Err(ProtocolError::InvalidEnum),
        "season above the maximum"
    );
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&mutate_player_state_wire(|wire| {
            wire[92] = 21;
        })),
        Err(ProtocolError::InvalidRange),
        "armor points above the maximum"
    );
    // An inactive block carrying nonzero progress is the union violation.
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&mutate_player_state_wire(|wire| {
            wire[68..70].copy_from_slice(&1u16.to_le_bytes());
        })),
        Err(ProtocolError::InvalidRange),
        "inactive mining block with nonzero progress"
    );
    // The active vector with progress at the requirement refuses, because a
    // completed swing publishes as inactive.
    assert_eq!(
        active_mining_player_state()
            .encode()
            .map(|wire| {
                let mut wire = wire;
                wire[70..72].copy_from_slice(&1u16.to_le_bytes());
                wire
            })
            .map(|wire| mornlea_protocol::PlayerState::decode(&wire)),
        Ok(Err(ProtocolError::InvalidRange)),
        "active mining progress at the requirement"
    );

    // The combat-hit boundaries report their own variants: the tick and
    // damage ranges are invalid-value, the kind is invalid-enum.
    assert_eq!(
        CombatHit::decode(&mutate_combat_hit_wire(|wire| wire[0] = 0)),
        Err(ProtocolError::InvalidRange),
        "zero server tick"
    );
    assert_eq!(
        CombatHit::decode(&mutate_combat_hit_wire(|wire| wire[8] = 0)),
        Err(ProtocolError::InvalidRange),
        "zero damage"
    );
    assert_eq!(
        CombatHit::decode(&mutate_combat_hit_wire(|wire| wire[8] = MAX_HEALTH + 1)),
        Err(ProtocolError::InvalidRange),
        "damage above the maximum"
    );
    assert_eq!(
        CombatHit::decode(&mutate_combat_hit_wire(|wire| wire[9] = 0)),
        Err(ProtocolError::InvalidEnum),
        "unknown target kind zero"
    );
    assert_eq!(
        CombatHit::decode(&mutate_combat_hit_wire(
            |wire| wire[9] = COMBAT_TARGET_PASSIVE + 1
        )),
        Err(ProtocolError::InvalidEnum),
        "unknown target kind above three"
    );
    // The three published kinds each admit, and the kind boundary decode
    // case is the third one.
    for kind in [
        COMBAT_TARGET_PLAYER,
        COMBAT_TARGET_HOSTILE,
        COMBAT_TARGET_PASSIVE,
    ] {
        let hit = CombatHit::new(1, 1, kind).expect("kind");
        assert_eq!(
            CombatHit::decode(&hit.encode().expect("encode")),
            Ok(hit),
            "target kind {kind} round-trips"
        );
    }
}

#[test]
fn player_outcomes_encode_into_leaves_a_short_destination_unchanged() {
    let cases: Vec<(
        &str,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "player state",
            PLAYER_STATE_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_player_state().encode_into(dst)),
        ),
        (
            "command rejected",
            COMMAND_REJECTED_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_command_rejected().encode_into(dst)),
        ),
        (
            "place block succeeded",
            PLACE_BLOCK_SUCCEEDED_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_place_block_succeeded().encode_into(dst)),
        ),
        (
            "combat hit",
            COMBAT_HIT_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_combat_hit().encode_into(dst)),
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
fn player_outcomes_packet_ids_are_pinned() {
    assert_eq!(mornlea_protocol::PlayerState::PACKET_ID, 3);
    assert_eq!(CommandRejected::PACKET_ID, 4);
    assert_eq!(PlaceBlockSucceeded::PACKET_ID, 20);
    assert_eq!(CombatHit::PACKET_ID, 25);
}
