//! Client control packet families: the common fallible encode surface and the
//! decode boundaries the four Play client-to-server records share.
//!
//! Every client control record is a concrete packet with public mutable fields,
//! so the group pins the design's surface order (`validate` → checked
//! `encoded_len` → capacity check → private `publish_packet`) for each family:
//! a short or invalid `encode_into` leaves the caller's buffer untouched, an
//! invalid value wins over a short destination, and every proper truncation of
//! a canonical payload rejects. The wire literals are the Go encoder's output
//! for these fields, so the round-trip tests pin byte-level parity rather than
//! self-agreement. The corpus evidence these families publish is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.

use mornlea_protocol::{PlaceBlock, PlayerInput, ProtocolError, RequestChunkResync, SelectHotbar};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// One type-erased client control record for the family-generic assertions.
///
/// Each variant carries the record by value, so the surface tests run the same
/// sequence for every family without generics over the packet types.
#[derive(Debug, PartialEq)]
enum ClientControlRecord {
    PlayerInput(PlayerInput),
    PlaceBlock(PlaceBlock),
    RequestChunkResync(RequestChunkResync),
    SelectHotbar(SelectHotbar),
}

impl ClientControlRecord {
    fn validate(&self) -> Result<(), ProtocolError> {
        match self {
            ClientControlRecord::PlayerInput(record) => record.validate(),
            ClientControlRecord::PlaceBlock(record) => record.validate(),
            ClientControlRecord::RequestChunkResync(record) => record.validate(),
            ClientControlRecord::SelectHotbar(record) => record.validate(),
        }
    }

    fn encoded_len(&self) -> Result<usize, ProtocolError> {
        match self {
            ClientControlRecord::PlayerInput(record) => record.encoded_len(),
            ClientControlRecord::PlaceBlock(record) => record.encoded_len(),
            ClientControlRecord::RequestChunkResync(record) => record.encoded_len(),
            ClientControlRecord::SelectHotbar(record) => record.encoded_len(),
        }
    }

    fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        match self {
            ClientControlRecord::PlayerInput(record) => record.encode_into(dst),
            ClientControlRecord::PlaceBlock(record) => record.encode_into(dst),
            ClientControlRecord::RequestChunkResync(record) => record.encode_into(dst),
            ClientControlRecord::SelectHotbar(record) => record.encode_into(dst),
        }
    }

    fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        match self {
            ClientControlRecord::PlayerInput(record) => record.encode(),
            ClientControlRecord::PlaceBlock(record) => record.encode(),
            ClientControlRecord::RequestChunkResync(record) => record.encode(),
            ClientControlRecord::SelectHotbar(record) => record.encode(),
        }
    }
}

/// One client control record family's canonical valid instance and wire
/// payload.
///
/// The wire literals are the Go encoder's output for these fields, so the
/// round-trip tests pin byte-level parity rather than self-agreement.
struct ClientControlVector {
    label: &'static str,
    payload: Vec<u8>,
    build: fn() -> ClientControlRecord,
}

/// Four canonical client control records: the reviewed valid instance of each
/// family beside the exact wire payload the Go encoder publishes for it.
fn client_control_vectors() -> Vec<ClientControlVector> {
    vec![
        ClientControlVector {
            label: "player input",
            payload: vec![
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x7f, 0x01, 0x00, 0x00, 0x00,
                0x80, 0x00, 0x00, 0x00, 0x40, 0x01, 0x00, 0x01, 0x00,
            ],
            build: || {
                ClientControlRecord::PlayerInput(
                    PlayerInput::new(0, -128, 127, true, -0.0, 2.0, true, false, true, false)
                        .expect("valid input"),
                )
            },
        },
        ClientControlVector {
            label: "place block",
            payload: vec![
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xa0, 0x3f, 0x00, 0x00,
                0x00, 0x80, 0x08,
            ],
            build: || {
                ClientControlRecord::PlaceBlock(
                    PlaceBlock::new(0, 1.25, -0.0, 8).expect("valid placement"),
                )
            },
        },
        ClientControlVector {
            label: "request chunk resync",
            payload: vec![
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0xff, 0xff,
                0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            ],
            build: || {
                ClientControlRecord::RequestChunkResync(RequestChunkResync::new(
                    0,
                    mornlea_domain::Dimension::DEPTHS,
                    -1,
                    0,
                    0,
                ))
            },
        },
        ClientControlVector {
            label: "select hotbar",
            payload: vec![0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08],
            build: || {
                ClientControlRecord::SelectHotbar(SelectHotbar::new(0, 8).expect("valid slot"))
            },
        },
    ]
}

fn decode_of(label: &str, payload: &[u8]) -> Result<ClientControlRecord, ProtocolError> {
    let decoded = match label {
        "player input" => ClientControlRecord::PlayerInput(PlayerInput::decode(payload)?),
        "place block" => ClientControlRecord::PlaceBlock(PlaceBlock::decode(payload)?),
        "request chunk resync" => {
            ClientControlRecord::RequestChunkResync(RequestChunkResync::decode(payload)?)
        }
        "select hotbar" => ClientControlRecord::SelectHotbar(SelectHotbar::decode(payload)?),
        other => panic!("no decoder is registered for {other}"),
    };
    Ok(decoded)
}

#[test]
fn client_control_records_round_trip_through_the_fallible_surface() {
    for vector in client_control_vectors() {
        let record = (vector.build)();
        record.validate().unwrap_or_else(|err| {
            panic!("{}: valid record fails validation: {err:?}", vector.label)
        });
        let length = record
            .encoded_len()
            .unwrap_or_else(|err| panic!("{}: encoded_len fails: {err:?}", vector.label));
        assert_eq!(
            length,
            vector.payload.len(),
            "{}: encoded_len disagrees with the reviewed payload",
            vector.label
        );

        // An exact window publishes the reviewed bytes and reports the length.
        let mut exact = vec![0u8; length];
        let written = record
            .encode_into(&mut exact)
            .unwrap_or_else(|err| panic!("{}: encode_into fails: {err:?}", vector.label));
        assert_eq!(
            written, length,
            "{}: encode_into reports a short write",
            vector.label
        );
        assert_eq!(
            exact, vector.payload,
            "{}: encode_into published unexpected bytes",
            vector.label
        );

        // The allocating wrapper agrees byte for byte with the caller-owned one.
        let allocated = record
            .encode()
            .unwrap_or_else(|err| panic!("{}: encode fails: {err:?}", vector.label));
        assert_eq!(
            allocated, vector.payload,
            "{}: encode disagrees with encode_into",
            vector.label
        );

        // A larger destination is written only in dst[..length].
        let mut padded = vec![SENTINEL; length + 3];
        let written = record
            .encode_into(&mut padded)
            .unwrap_or_else(|err| panic!("{}: padded encode_into fails: {err:?}", vector.label));
        assert_eq!(
            written, length,
            "{}: padded write reports a short length",
            vector.label
        );
        assert_eq!(
            &padded[..length],
            vector.payload.as_slice(),
            "{}: padded prefix differs from the reviewed payload",
            vector.label
        );
        assert!(
            padded[length..].iter().all(|byte| *byte == SENTINEL),
            "{}: padded write touched bytes beyond the record",
            vector.label
        );

        // The decoded record equals the encoded one, and re-encoding it
        // reproduces the reviewed bytes, so both directions share one canonical
        // form.
        let decoded = decode_of(vector.label, &vector.payload)
            .unwrap_or_else(|err| panic!("{}: decode fails: {err:?}", vector.label));
        assert_eq!(
            decoded, record,
            "{}: decoded record differs from the encoded one",
            vector.label
        );
        let reencoded = decoded
            .encode()
            .unwrap_or_else(|err| panic!("{}: re-encode fails: {err:?}", vector.label));
        assert_eq!(
            reencoded, vector.payload,
            "{}: re-encoded payload differs from the reviewed bytes",
            vector.label
        );
    }
}

#[test]
fn client_control_encode_into_leaves_a_short_destination_unchanged() {
    for vector in client_control_vectors() {
        let record = (vector.build)();
        let needed = vector.payload.len();

        for available in [0usize, needed - 1, needed.saturating_sub(3)] {
            let mut dst = vec![SENTINEL; available];
            let error = record
                .encode_into(&mut dst)
                .expect_err("a short destination must be refused");
            assert_eq!(
                error,
                ProtocolError::OutputTooSmall { needed, available },
                "{}: short destination of {available} bytes reports the wrong error",
                vector.label
            );
            assert!(
                dst.iter().all(|byte| *byte == SENTINEL),
                "{}: short destination of {available} bytes was modified",
                vector.label
            );
        }
    }
}

/// One family's mutate-after-construction case: the canonical instance, the
/// invalid value a public mutable field is set to after construction, and the
/// error the surface has to report for it before any size or capacity decision.
struct ClientControlMutation {
    label: &'static str,
    build: fn() -> ClientControlRecord,
    mutate: fn(&mut ClientControlRecord),
    error: ProtocolError,
}

fn client_control_mutations() -> Vec<ClientControlMutation> {
    vec![
        ClientControlMutation {
            label: "player input",
            build: || {
                ClientControlRecord::PlayerInput(
                    PlayerInput::new(0, 0, 0, false, 0.0, 0.0, false, false, false, false)
                        .expect("valid input"),
                )
            },
            mutate: |record| {
                if let ClientControlRecord::PlayerInput(input) = record {
                    input.yaw = f32::NAN;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientControlMutation {
            label: "place block slot",
            build: || {
                ClientControlRecord::PlaceBlock(
                    PlaceBlock::new(0, 0.0, 0.0, 0).expect("valid placement"),
                )
            },
            mutate: |record| {
                if let ClientControlRecord::PlaceBlock(place) = record {
                    place.slot = 9;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientControlMutation {
            label: "place block pitch",
            build: || {
                ClientControlRecord::PlaceBlock(
                    PlaceBlock::new(0, 0.0, 0.0, 0).expect("valid placement"),
                )
            },
            mutate: |record| {
                if let ClientControlRecord::PlaceBlock(place) = record {
                    place.pitch = f32::INFINITY;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientControlMutation {
            label: "select hotbar",
            build: || {
                ClientControlRecord::SelectHotbar(SelectHotbar::new(0, 0).expect("valid slot"))
            },
            mutate: |record| {
                if let ClientControlRecord::SelectHotbar(select) = record {
                    select.slot = 9;
                }
            },
            error: ProtocolError::InvalidRange,
        },
    ]
}

#[test]
fn client_control_invalid_value_wins_over_short_capacity() {
    for mutation in client_control_mutations() {
        let mut record = (mutation.build)();
        // The unmutated record sizes the destinations, so the capacity refusal
        // and the value refusal are compared on buffers of the same shape.
        let needed = record.encoded_len().unwrap_or_else(|err| {
            panic!(
                "{}: the valid record fails to size: {err:?}",
                mutation.label
            )
        });
        (mutation.mutate)(&mut record);

        // The value gate runs before the size and capacity decisions, so every
        // entry point reports the same value error for the mutated record.
        assert_eq!(
            record.validate(),
            Err(mutation.error),
            "{}: validate disagrees with the mutation",
            mutation.label
        );
        assert_eq!(
            record.encoded_len(),
            Err(mutation.error),
            "{}: encoded_len disagrees with the mutation",
            mutation.label
        );
        assert_eq!(
            record.encode(),
            Err(mutation.error),
            "{}: encode disagrees with the mutation",
            mutation.label
        );

        // A short destination refuses with the value error, not a capacity
        // error, and is left untouched.
        let mut short = vec![SENTINEL; needed - 1];
        assert_eq!(
            record.encode_into(&mut short),
            Err(mutation.error),
            "{}: a short destination masks the value error",
            mutation.label
        );
        assert!(
            short.iter().all(|byte| *byte == SENTINEL),
            "{}: a short destination was modified on a value refusal",
            mutation.label
        );

        // An exact destination refuses the same way and is left untouched too.
        let mut exact = vec![SENTINEL; needed];
        assert_eq!(
            record.encode_into(&mut exact),
            Err(mutation.error),
            "{}: an exact destination masks the value error",
            mutation.label
        );
        assert!(
            exact.iter().all(|byte| *byte == SENTINEL),
            "{}: an exact destination was modified on a value refusal",
            mutation.label
        );
    }
}

#[test]
fn client_control_request_chunk_resync_has_no_invalid_value_to_mutate_into() {
    // The resync record carries a checked dimension and free coordinates and
    // revision values, so no field mutation can make it invalid. The family
    // still exposes the common surface: the gate exists, stays total, and every
    // field can be rewritten to its extreme legal value without breaking the
    // record.
    let mut resync = RequestChunkResync::new(0, mornlea_domain::Dimension::OVERWORLD, 0, 0, 0);
    resync
        .validate()
        .expect("a zero revision and overworld dimension are legal");
    resync.sequence = u64::MAX;
    resync.dimension = mornlea_domain::Dimension::DEPTHS;
    resync.chunk_x = i32::MIN;
    resync.chunk_z = i32::MAX;
    resync.have_revision = u64::MAX;
    resync.validate().expect("extreme legal values stay legal");
    let payload = resync.encode().expect("encode the mutated record");
    assert_eq!(payload.len(), 28);
    assert_eq!(&payload[..8], &u64::MAX.to_le_bytes());
    assert_eq!(&payload[8..12], &1i32.to_le_bytes());
    assert_eq!(&payload[12..16], &i32::MIN.to_le_bytes());
    assert_eq!(&payload[16..20], &i32::MAX.to_le_bytes());
    assert_eq!(&payload[20..], &u64::MAX.to_le_bytes());
}

#[test]
fn client_control_decode_rejects_every_proper_truncation() {
    for vector in client_control_vectors() {
        for cut in 0..vector.payload.len() {
            let truncated = &vector.payload[..cut];
            let error = decode_of(vector.label, truncated)
                .expect_err("a proper truncation must never decode");
            assert_eq!(
                error,
                ProtocolError::Truncated,
                "{}: truncation at {cut} bytes reports {error:?}",
                vector.label
            );
        }
    }
}

#[test]
fn client_control_decode_rejects_one_trailing_byte() {
    for vector in client_control_vectors() {
        let mut trailing = vector.payload.clone();
        trailing.push(0x00);
        let error =
            decode_of(vector.label, &trailing).expect_err("one trailing byte must never decode");
        assert_eq!(
            error,
            ProtocolError::TrailingBytes,
            "{}: one trailing byte reports {error:?}",
            vector.label
        );
    }
}

#[test]
fn client_control_decode_rejects_the_pinned_invalid_values() {
    let mut nan_yaw = vec![0u8; 8];
    nan_yaw.extend_from_slice(&[0, 0, 0]);
    nan_yaw.extend_from_slice(&0x7fc0_0000u32.to_le_bytes());
    nan_yaw.extend_from_slice(&0f32.to_le_bytes());
    nan_yaw.extend_from_slice(&[0, 0, 0, 0]);
    assert_eq!(
        PlayerInput::decode(&nan_yaw),
        Err(ProtocolError::InvalidFloat)
    );
    let mut bool_two = vec![0u8; 8];
    bool_two.extend_from_slice(&[0, 0, 0x02]);
    bool_two.extend_from_slice(&0f32.to_le_bytes());
    bool_two.extend_from_slice(&0f32.to_le_bytes());
    bool_two.extend_from_slice(&[0, 0, 0, 0]);
    assert_eq!(
        PlayerInput::decode(&bool_two),
        Err(ProtocolError::InvalidEnum)
    );

    let mut place_slot_nine = vec![0u8; 8];
    place_slot_nine.extend_from_slice(&0f32.to_le_bytes());
    place_slot_nine.extend_from_slice(&0f32.to_le_bytes());
    place_slot_nine.push(0x09);
    assert_eq!(
        PlaceBlock::decode(&place_slot_nine),
        Err(ProtocolError::InvalidRange)
    );
    let mut place_inf_pitch = vec![0u8; 8];
    place_inf_pitch.extend_from_slice(&0f32.to_le_bytes());
    place_inf_pitch.extend_from_slice(&f32::INFINITY.to_le_bytes());
    place_inf_pitch.push(0x00);
    assert_eq!(
        PlaceBlock::decode(&place_inf_pitch),
        Err(ProtocolError::InvalidFloat)
    );

    let mut dimension_two = vec![0u8; 8];
    dimension_two.extend_from_slice(&2i32.to_le_bytes());
    dimension_two.extend_from_slice(&0i32.to_le_bytes());
    dimension_two.extend_from_slice(&0i32.to_le_bytes());
    dimension_two.extend_from_slice(&0u64.to_le_bytes());
    assert_eq!(
        RequestChunkResync::decode(&dimension_two),
        Err(ProtocolError::InvalidEnum)
    );
    // The dimension is never narrowed to a u8: 256 fits a u8 port but is not a
    // known dimension, so it is refused as an unknown enum rather than
    // reinterpreted.
    let mut dimension_256 = vec![0u8; 8];
    dimension_256.extend_from_slice(&256i32.to_le_bytes());
    dimension_256.extend_from_slice(&0i32.to_le_bytes());
    dimension_256.extend_from_slice(&0i32.to_le_bytes());
    dimension_256.extend_from_slice(&0u64.to_le_bytes());
    assert_eq!(
        RequestChunkResync::decode(&dimension_256),
        Err(ProtocolError::InvalidEnum)
    );

    let mut select_slot_nine = vec![0u8; 8];
    select_slot_nine.push(0x09);
    assert_eq!(
        SelectHotbar::decode(&select_slot_nine),
        Err(ProtocolError::InvalidRange)
    );
}

#[test]
fn client_control_negative_zero_and_wide_values_keep_their_wire_shapes() {
    // The review literals carry -0.0 yaw (PlayerInput) and -0.0 pitch
    // (PlaceBlock), and f32 `==` cannot distinguish -0.0 from +0.0, so the bit
    // pattern is pinned explicitly. The full i8 axes and the finite 2.0 pitch
    // ride along: the protocol boundary must not clamp them.
    let input = PlayerInput::new(0, -128, 127, true, -0.0, 2.0, true, false, true, false)
        .expect("valid input");
    let payload = input.encode().expect("encode input");
    assert_eq!(payload.len(), 23);
    assert_eq!(input.move_x, i8::MIN);
    assert_eq!(input.move_z, i8::MAX);
    assert_eq!(input.yaw.to_bits(), 0x8000_0000);
    assert_eq!(input.pitch, 2.0);
    let decoded_input = PlayerInput::decode(&payload).expect("decode input");
    assert_eq!(decoded_input.yaw.to_bits(), 0x8000_0000);
    assert_eq!(decoded_input.pitch.to_bits(), 2.0f32.to_bits());
    assert_eq!(decoded_input.move_x, i8::MIN);
    assert_eq!(decoded_input.move_z, i8::MAX);

    let place = PlaceBlock::new(0, 1.25, -0.0, 8).expect("valid placement");
    let payload = place.encode().expect("encode place");
    assert_eq!(payload.len(), 17);
    assert_eq!(place.yaw, 1.25);
    assert_eq!(place.pitch.to_bits(), 0x8000_0000);
    let decoded_place = PlaceBlock::decode(&payload).expect("decode place");
    assert_eq!(decoded_place.yaw.to_bits(), 1.25f32.to_bits());
    assert_eq!(decoded_place.pitch.to_bits(), 0x8000_0000);
    assert_eq!(decoded_place.slot, 8);
}

#[test]
fn client_control_packet_ids_are_pinned() {
    assert_eq!(PlayerInput::PACKET_ID, 0);
    assert_eq!(PlaceBlock::PACKET_ID, 2);
    assert_eq!(RequestChunkResync::PACKET_ID, 3);
    assert_eq!(SelectHotbar::PACKET_ID, 5);
}
