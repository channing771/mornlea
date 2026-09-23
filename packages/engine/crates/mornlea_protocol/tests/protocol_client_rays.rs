//! Client ray action packet families: the common fallible encode surface and
//! the decode boundaries the five Play client-to-server ray records share.
//!
//! Every client ray record is a concrete packet with public mutable fields, so
//! the group pins the design's surface order (`validate` → checked
//! `encoded_len` → capacity check → private `publish_packet`) for each family:
//! a short or invalid `encode_into` leaves the caller's buffer untouched, an
//! invalid value wins over a short destination, and every proper truncation of
//! a canonical payload rejects. The wire literals are the Go encoder's output
//! for these fields, so the round-trip tests pin byte-level parity rather than
//! self-agreement. The corpus evidence these families publish is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.
//!
//! All five families share one wire shape: a sequence and the two look angles.
//! The ray-cast target, the held item, the container kind and the resulting
//! world write stay server-owned, so the 16-byte payload carries none of them.

use mornlea_protocol::{
    BoneMeal, CollectWater, OpenContainer, PlaceWater, ProtocolError, TillSoil,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The fixed wire stride every ray payload occupies: the 8-byte sequence and
/// the two four-byte look angles, with no target, item or result field.
const RAY_WIRE_BYTES: usize = 16;

/// One type-erased client ray record for the family-generic assertions.
///
/// Each variant carries the record by value, so the surface tests run the same
/// sequence for every family without generics over the packet types.
#[derive(Debug, PartialEq)]
enum ClientRayRecord {
    OpenContainer(OpenContainer),
    TillSoil(TillSoil),
    BoneMeal(BoneMeal),
    CollectWater(CollectWater),
    PlaceWater(PlaceWater),
}

impl ClientRayRecord {
    fn validate(&self) -> Result<(), ProtocolError> {
        match self {
            ClientRayRecord::OpenContainer(record) => record.validate(),
            ClientRayRecord::TillSoil(record) => record.validate(),
            ClientRayRecord::BoneMeal(record) => record.validate(),
            ClientRayRecord::CollectWater(record) => record.validate(),
            ClientRayRecord::PlaceWater(record) => record.validate(),
        }
    }

    fn encoded_len(&self) -> Result<usize, ProtocolError> {
        match self {
            ClientRayRecord::OpenContainer(record) => record.encoded_len(),
            ClientRayRecord::TillSoil(record) => record.encoded_len(),
            ClientRayRecord::BoneMeal(record) => record.encoded_len(),
            ClientRayRecord::CollectWater(record) => record.encoded_len(),
            ClientRayRecord::PlaceWater(record) => record.encoded_len(),
        }
    }

    fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        match self {
            ClientRayRecord::OpenContainer(record) => record.encode_into(dst),
            ClientRayRecord::TillSoil(record) => record.encode_into(dst),
            ClientRayRecord::BoneMeal(record) => record.encode_into(dst),
            ClientRayRecord::CollectWater(record) => record.encode_into(dst),
            ClientRayRecord::PlaceWater(record) => record.encode_into(dst),
        }
    }

    fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        match self {
            ClientRayRecord::OpenContainer(record) => record.encode(),
            ClientRayRecord::TillSoil(record) => record.encode(),
            ClientRayRecord::BoneMeal(record) => record.encode(),
            ClientRayRecord::CollectWater(record) => record.encode(),
            ClientRayRecord::PlaceWater(record) => record.encode(),
        }
    }

    fn yaw_bits(&self) -> u32 {
        match self {
            ClientRayRecord::OpenContainer(record) => record.yaw.to_bits(),
            ClientRayRecord::TillSoil(record) => record.yaw.to_bits(),
            ClientRayRecord::BoneMeal(record) => record.yaw.to_bits(),
            ClientRayRecord::CollectWater(record) => record.yaw.to_bits(),
            ClientRayRecord::PlaceWater(record) => record.yaw.to_bits(),
        }
    }
}

/// One client ray record family's canonical valid instance and wire payload.
///
/// The wire literals are the Go encoder's output for these fields, so the
/// round-trip tests pin byte-level parity rather than self-agreement.
struct ClientRayVector {
    label: &'static str,
    payload: Vec<u8>,
    build: fn() -> ClientRayRecord,
}

/// Five canonical client ray records: the reviewed valid instance of each
/// family beside the exact wire payload the Go encoder publishes for it.
///
/// Every family carries sequence 0, a `-0.0` yaw and a 1.5 pitch, so the five
/// literals are byte-identical: the packet ID is the only thing that tells the
/// families apart on the wire.
fn client_ray_vectors() -> Vec<ClientRayVector> {
    vec![
        ClientRayVector {
            label: "open container",
            payload: vec![
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00,
                0xc0, 0x3f,
            ],
            build: || {
                ClientRayRecord::OpenContainer(
                    OpenContainer::new(0, -0.0, 1.5).expect("valid open container"),
                )
            },
        },
        ClientRayVector {
            label: "till soil",
            payload: vec![
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00,
                0xc0, 0x3f,
            ],
            build: || ClientRayRecord::TillSoil(TillSoil::new(0, -0.0, 1.5).expect("valid till")),
        },
        ClientRayVector {
            label: "bone meal",
            payload: vec![
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00,
                0xc0, 0x3f,
            ],
            build: || {
                ClientRayRecord::BoneMeal(BoneMeal::new(0, -0.0, 1.5).expect("valid bone meal"))
            },
        },
        ClientRayVector {
            label: "collect water",
            payload: vec![
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00,
                0xc0, 0x3f,
            ],
            build: || {
                ClientRayRecord::CollectWater(
                    CollectWater::new(0, -0.0, 1.5).expect("valid collect water"),
                )
            },
        },
        ClientRayVector {
            label: "place water",
            payload: vec![
                0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00,
                0xc0, 0x3f,
            ],
            build: || {
                ClientRayRecord::PlaceWater(
                    PlaceWater::new(0, -0.0, 1.5).expect("valid place water"),
                )
            },
        },
    ]
}

fn decode_of(label: &str, payload: &[u8]) -> Result<ClientRayRecord, ProtocolError> {
    let decoded = match label {
        "open container" => ClientRayRecord::OpenContainer(OpenContainer::decode(payload)?),
        "till soil" => ClientRayRecord::TillSoil(TillSoil::decode(payload)?),
        "bone meal" => ClientRayRecord::BoneMeal(BoneMeal::decode(payload)?),
        "collect water" => ClientRayRecord::CollectWater(CollectWater::decode(payload)?),
        "place water" => ClientRayRecord::PlaceWater(PlaceWater::decode(payload)?),
        other => panic!("no decoder is registered for {other}"),
    };
    Ok(decoded)
}

/// The offset each look angle occupies in the shared 16-byte payload.
const YAW_OFFSET: usize = 8;
const PITCH_OFFSET: usize = 12;

#[test]
fn client_ray_records_round_trip_through_the_fallible_surface() {
    for vector in client_ray_vectors() {
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
        assert_eq!(
            length, RAY_WIRE_BYTES,
            "{}: a ray payload is exactly the sequence and the two angles",
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
fn client_ray_negative_zero_yaw_survives_the_round_trip() {
    // The reviewed literals carry a -0.0 yaw, and f32 `==` cannot distinguish
    // -0.0 from +0.0, so the bit pattern is pinned explicitly across the
    // round trip: the payload purity assertion is the exact 16-byte literal,
    // which carries the sequence and the two angles and nothing else.
    for vector in client_ray_vectors() {
        let record = (vector.build)();
        assert_eq!(
            record.yaw_bits(),
            0x8000_0000,
            "{}: the canonical yaw is negative zero",
            vector.label
        );
        let decoded = decode_of(vector.label, &vector.payload)
            .unwrap_or_else(|err| panic!("{}: decode fails: {err:?}", vector.label));
        assert_eq!(
            decoded.yaw_bits(),
            0x8000_0000,
            "{}: negative zero yaw lost its bit pattern",
            vector.label
        );
        assert_eq!(
            vector.payload.len(),
            RAY_WIRE_BYTES,
            "{}: the payload carries only the sequence and the two angles",
            vector.label
        );
    }
}

#[test]
fn client_ray_encode_into_leaves_a_short_destination_unchanged() {
    for vector in client_ray_vectors() {
        let record = (vector.build)();
        let needed = vector.payload.len();

        for available in [0usize, 15, 13] {
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
struct ClientRayMutation {
    label: &'static str,
    build: fn() -> ClientRayRecord,
    mutate: fn(&mut ClientRayRecord),
    error: ProtocolError,
}

fn client_ray_mutations() -> Vec<ClientRayMutation> {
    vec![
        ClientRayMutation {
            label: "open container yaw",
            build: || {
                ClientRayRecord::OpenContainer(OpenContainer::new(0, 0.0, 0.0).expect("open"))
            },
            mutate: |record| {
                if let ClientRayRecord::OpenContainer(open) = record {
                    open.yaw = f32::NAN;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientRayMutation {
            label: "open container pitch",
            build: || {
                ClientRayRecord::OpenContainer(OpenContainer::new(0, 0.0, 0.0).expect("open"))
            },
            mutate: |record| {
                if let ClientRayRecord::OpenContainer(open) = record {
                    open.pitch = f32::INFINITY;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientRayMutation {
            label: "till soil yaw",
            build: || ClientRayRecord::TillSoil(TillSoil::new(0, 0.0, 0.0).expect("till")),
            mutate: |record| {
                if let ClientRayRecord::TillSoil(till) = record {
                    till.yaw = f32::NAN;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientRayMutation {
            label: "till soil pitch",
            build: || ClientRayRecord::TillSoil(TillSoil::new(0, 0.0, 0.0).expect("till")),
            mutate: |record| {
                if let ClientRayRecord::TillSoil(till) = record {
                    till.pitch = f32::INFINITY;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientRayMutation {
            label: "bone meal yaw",
            build: || ClientRayRecord::BoneMeal(BoneMeal::new(0, 0.0, 0.0).expect("meal")),
            mutate: |record| {
                if let ClientRayRecord::BoneMeal(meal) = record {
                    meal.yaw = f32::NAN;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientRayMutation {
            label: "bone meal pitch",
            build: || ClientRayRecord::BoneMeal(BoneMeal::new(0, 0.0, 0.0).expect("meal")),
            mutate: |record| {
                if let ClientRayRecord::BoneMeal(meal) = record {
                    meal.pitch = f32::INFINITY;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientRayMutation {
            label: "collect water yaw",
            build: || {
                ClientRayRecord::CollectWater(CollectWater::new(0, 0.0, 0.0).expect("collect"))
            },
            mutate: |record| {
                if let ClientRayRecord::CollectWater(collect) = record {
                    collect.yaw = f32::NAN;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientRayMutation {
            label: "collect water pitch",
            build: || {
                ClientRayRecord::CollectWater(CollectWater::new(0, 0.0, 0.0).expect("collect"))
            },
            mutate: |record| {
                if let ClientRayRecord::CollectWater(collect) = record {
                    collect.pitch = f32::INFINITY;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientRayMutation {
            label: "place water yaw",
            build: || ClientRayRecord::PlaceWater(PlaceWater::new(0, 0.0, 0.0).expect("place")),
            mutate: |record| {
                if let ClientRayRecord::PlaceWater(place) = record {
                    place.yaw = f32::NAN;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
        ClientRayMutation {
            label: "place water pitch",
            build: || ClientRayRecord::PlaceWater(PlaceWater::new(0, 0.0, 0.0).expect("place")),
            mutate: |record| {
                if let ClientRayRecord::PlaceWater(place) = record {
                    place.pitch = f32::INFINITY;
                }
            },
            error: ProtocolError::InvalidFloat,
        },
    ]
}

#[test]
fn client_ray_invalid_value_wins_over_short_capacity() {
    for mutation in client_ray_mutations() {
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
fn client_ray_decode_rejects_every_proper_truncation() {
    for vector in client_ray_vectors() {
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
fn client_ray_decode_rejects_one_trailing_byte() {
    for vector in client_ray_vectors() {
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
fn client_ray_decode_rejects_the_pinned_invalid_values() {
    // A quiet NaN yaw and a positive-infinite pitch are refused by the f32
    // primitive before publication, for every family.
    let mut nan_yaw = vec![0u8; YAW_OFFSET];
    nan_yaw.extend_from_slice(&0x7fc0_0000u32.to_le_bytes());
    nan_yaw.extend_from_slice(&1.5f32.to_le_bytes());
    let mut inf_pitch = vec![0u8; YAW_OFFSET];
    inf_pitch.extend_from_slice(&(-0.0f32).to_le_bytes());
    inf_pitch.extend_from_slice(&0x7f80_0000u32.to_le_bytes());

    for vector in client_ray_vectors() {
        let error = decode_of(vector.label, &nan_yaw).expect_err("a NaN yaw must never decode");
        assert_eq!(
            error,
            ProtocolError::InvalidFloat,
            "{}: NaN yaw reports {error:?}",
            vector.label
        );
        let error =
            decode_of(vector.label, &inf_pitch).expect_err("an infinite pitch must never decode");
        assert_eq!(
            error,
            ProtocolError::InvalidFloat,
            "{}: infinite pitch reports {error:?}",
            vector.label
        );
    }
}

#[test]
fn client_ray_packet_ids_are_pinned() {
    assert_eq!(OpenContainer::PACKET_ID, 8);
    assert_eq!(TillSoil::PACKET_ID, 13);
    assert_eq!(BoneMeal::PACKET_ID, 14);
    assert_eq!(CollectWater::PACKET_ID, 16);
    assert_eq!(PlaceWater::PACKET_ID, 17);
}
