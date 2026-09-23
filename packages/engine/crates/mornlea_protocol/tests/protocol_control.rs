//! Control packet families: the common fallible encode surface and the decode
//! boundaries the seven handshake, login and keepalive records share.
//!
//! Every control record is a concrete packet with public mutable fields, so the
//! group pins the design's surface order (`validate` → checked `encoded_len` →
//! capacity check → private `publish_packet`) for each family: a short or
//! invalid `encode_into` leaves the caller's buffer untouched, an invalid value
//! wins over a short destination, and every proper truncation of a canonical
//! payload rejects. The corpus evidence these families publish is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.

use mornlea_protocol::{
    Disconnect, HandshakeReject, KeepAlive, KeepAliveReply, LoginReject, LoginSuccess,
    ProtocolError, ServerHello,
};

/// Identity the login success cases carry: version nibble 4 and variant bits 10.
const CONTROL_PLAYER_ID: [u8; 16] = [
    0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
];

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

fn control_player_id() -> mornlea_protocol::PlayerId {
    mornlea_protocol::PlayerId::try_from_bytes(CONTROL_PLAYER_ID).expect("uuid v4 identity")
}

/// One type-erased control record for the family-generic assertions.
///
/// Each variant carries the record by value, so the surface tests run the same
/// sequence for every family without generics over the packet types.
#[derive(Debug, PartialEq)]
enum ControlRecord {
    ServerHello(ServerHello),
    HandshakeReject(HandshakeReject),
    LoginSuccess(LoginSuccess),
    LoginReject(LoginReject),
    KeepAlive(KeepAlive),
    KeepAliveReply(KeepAliveReply),
    Disconnect(Disconnect),
}

impl ControlRecord {
    fn validate(&self) -> Result<(), ProtocolError> {
        match self {
            ControlRecord::ServerHello(record) => record.validate(),
            ControlRecord::HandshakeReject(record) => record.validate(),
            ControlRecord::LoginSuccess(record) => record.validate(),
            ControlRecord::LoginReject(record) => record.validate(),
            ControlRecord::KeepAlive(record) => record.validate(),
            ControlRecord::KeepAliveReply(record) => record.validate(),
            ControlRecord::Disconnect(record) => record.validate(),
        }
    }

    fn encoded_len(&self) -> Result<usize, ProtocolError> {
        match self {
            ControlRecord::ServerHello(record) => record.encoded_len(),
            ControlRecord::HandshakeReject(record) => record.encoded_len(),
            ControlRecord::LoginSuccess(record) => record.encoded_len(),
            ControlRecord::LoginReject(record) => record.encoded_len(),
            ControlRecord::KeepAlive(record) => record.encoded_len(),
            ControlRecord::KeepAliveReply(record) => record.encoded_len(),
            ControlRecord::Disconnect(record) => record.encoded_len(),
        }
    }

    fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        match self {
            ControlRecord::ServerHello(record) => record.encode_into(dst),
            ControlRecord::HandshakeReject(record) => record.encode_into(dst),
            ControlRecord::LoginSuccess(record) => record.encode_into(dst),
            ControlRecord::LoginReject(record) => record.encode_into(dst),
            ControlRecord::KeepAlive(record) => record.encode_into(dst),
            ControlRecord::KeepAliveReply(record) => record.encode_into(dst),
            ControlRecord::Disconnect(record) => record.encode_into(dst),
        }
    }

    fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        match self {
            ControlRecord::ServerHello(record) => record.encode(),
            ControlRecord::HandshakeReject(record) => record.encode(),
            ControlRecord::LoginSuccess(record) => record.encode(),
            ControlRecord::LoginReject(record) => record.encode(),
            ControlRecord::KeepAlive(record) => record.encode(),
            ControlRecord::KeepAliveReply(record) => record.encode(),
            ControlRecord::Disconnect(record) => record.encode(),
        }
    }
}

/// One control record family's canonical valid instance and wire payload.
///
/// The wire literals are the Go encoder's output for these fields, so the
/// round-trip tests pin byte-level parity rather than self-agreement.
struct ControlVector {
    label: &'static str,
    payload: Vec<u8>,
    build: fn() -> ControlRecord,
}

/// Nine canonical control records: the reviewed valid instance of each family
/// beside the exact wire payload the Go encoder publishes for it.
fn control_vectors() -> Vec<ControlVector> {
    vec![
        ControlVector {
            label: "server hello",
            payload: vec![0x2d],
            build: || ControlRecord::ServerHello(ServerHello::new(45).expect("current hello")),
        },
        ControlVector {
            label: "handshake reject",
            payload: vec![0x2a, 0x01, 0x00],
            build: || {
                ControlRecord::HandshakeReject(
                    HandshakeReject::new(42, 1, "").expect("current reject"),
                )
            },
        },
        ControlVector {
            label: "handshake reject with message",
            payload: vec![0x2a, 0x01, 0x02, b'n', b'o'],
            build: || {
                ControlRecord::HandshakeReject(
                    HandshakeReject::new(42, 1, "no").expect("reject with message"),
                )
            },
        },
        ControlVector {
            label: "login success zero seed",
            payload: {
                let mut payload = CONTROL_PLAYER_ID.to_vec();
                payload.extend_from_slice(&[0u8; 8]);
                payload
            },
            build: || ControlRecord::LoginSuccess(LoginSuccess::new(control_player_id(), 0)),
        },
        ControlVector {
            label: "login success seeded",
            payload: {
                let mut payload = CONTROL_PLAYER_ID.to_vec();
                payload.extend_from_slice(&0x1122_3344_5566_7788u64.to_le_bytes());
                payload
            },
            build: || {
                ControlRecord::LoginSuccess(LoginSuccess::new(
                    control_player_id(),
                    0x1122_3344_5566_7788,
                ))
            },
        },
        ControlVector {
            label: "login reject",
            payload: vec![0x01, 0x00],
            build: || ControlRecord::LoginReject(LoginReject::new(1, "").expect("current reject")),
        },
        ControlVector {
            label: "keep alive",
            payload: vec![0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
            build: || ControlRecord::KeepAlive(KeepAlive::new(8).expect("current keep alive")),
        },
        ControlVector {
            label: "keep alive reply",
            payload: vec![0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
            build: || ControlRecord::KeepAliveReply(KeepAliveReply::new(6).expect("current reply")),
        },
        ControlVector {
            label: "disconnect",
            payload: vec![0x02, 0x03, b'b', b'y', b'e'],
            build: || {
                ControlRecord::Disconnect(Disconnect::new(2, "bye").expect("current disconnect"))
            },
        },
    ]
}

fn decode_of(label: &str, payload: &[u8]) -> Result<ControlRecord, ProtocolError> {
    let decoded = match label {
        "server hello" => ControlRecord::ServerHello(ServerHello::decode(payload)?),
        "handshake reject" | "handshake reject with message" => {
            ControlRecord::HandshakeReject(HandshakeReject::decode(payload)?)
        }
        "login success zero seed" | "login success seeded" => {
            ControlRecord::LoginSuccess(LoginSuccess::decode(payload)?)
        }
        "login reject" => ControlRecord::LoginReject(LoginReject::decode(payload)?),
        "keep alive" => ControlRecord::KeepAlive(KeepAlive::decode(payload)?),
        "keep alive reply" => ControlRecord::KeepAliveReply(KeepAliveReply::decode(payload)?),
        "disconnect" => ControlRecord::Disconnect(Disconnect::decode(payload)?),
        other => panic!("no decoder is registered for {other}"),
    };
    Ok(decoded)
}

#[test]
fn control_records_round_trip_through_the_fallible_surface() {
    for vector in control_vectors() {
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
fn control_encode_into_leaves_a_short_destination_unchanged() {
    for vector in control_vectors() {
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
struct ControlMutation {
    label: &'static str,
    build: fn() -> ControlRecord,
    mutate: fn(&mut ControlRecord),
    error: ProtocolError,
}

fn control_mutations() -> Vec<ControlMutation> {
    vec![
        ControlMutation {
            label: "server hello",
            build: || ControlRecord::ServerHello(ServerHello::new(45).expect("current hello")),
            mutate: |record| {
                if let ControlRecord::ServerHello(hello) = record {
                    hello.protocol_version = 44;
                }
            },
            error: ProtocolError::UnsupportedVersion,
        },
        ControlMutation {
            label: "handshake reject",
            build: || {
                ControlRecord::HandshakeReject(
                    HandshakeReject::new(42, 1, "").expect("current reject"),
                )
            },
            mutate: |record| {
                if let ControlRecord::HandshakeReject(reject) = record {
                    reject.code = 2;
                }
            },
            error: ProtocolError::InvalidEnum,
        },
        ControlMutation {
            label: "handshake reject with message",
            build: || {
                ControlRecord::HandshakeReject(
                    HandshakeReject::new(42, 1, "no").expect("reject with message"),
                )
            },
            mutate: |record| {
                if let ControlRecord::HandshakeReject(reject) = record {
                    reject.message = "a".repeat(257);
                }
            },
            error: ProtocolError::InvalidString,
        },
        ControlMutation {
            label: "login reject",
            build: || ControlRecord::LoginReject(LoginReject::new(1, "").expect("current reject")),
            mutate: |record| {
                if let ControlRecord::LoginReject(reject) = record {
                    reject.code = 0;
                }
            },
            error: ProtocolError::InvalidEnum,
        },
        ControlMutation {
            label: "login reject",
            build: || ControlRecord::LoginReject(LoginReject::new(1, "").expect("current reject")),
            mutate: |record| {
                if let ControlRecord::LoginReject(reject) = record {
                    reject.code = 8;
                }
            },
            error: ProtocolError::InvalidEnum,
        },
        ControlMutation {
            label: "login reject",
            build: || {
                ControlRecord::LoginReject(LoginReject::new(1, "no").expect("reject with message"))
            },
            mutate: |record| {
                if let ControlRecord::LoginReject(reject) = record {
                    reject.message = "a".repeat(257);
                }
            },
            error: ProtocolError::InvalidString,
        },
        ControlMutation {
            label: "keep alive",
            build: || ControlRecord::KeepAlive(KeepAlive::new(8).expect("current keep alive")),
            mutate: |record| {
                if let ControlRecord::KeepAlive(keep_alive) = record {
                    keep_alive.token = 0;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ControlMutation {
            label: "keep alive reply",
            build: || ControlRecord::KeepAliveReply(KeepAliveReply::new(6).expect("current reply")),
            mutate: |record| {
                if let ControlRecord::KeepAliveReply(reply) = record {
                    reply.token = 0;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ControlMutation {
            label: "disconnect",
            build: || {
                ControlRecord::Disconnect(Disconnect::new(2, "bye").expect("current disconnect"))
            },
            mutate: |record| {
                if let ControlRecord::Disconnect(disconnect) = record {
                    disconnect.code = 0;
                }
            },
            error: ProtocolError::InvalidEnum,
        },
        ControlMutation {
            label: "disconnect",
            build: || {
                ControlRecord::Disconnect(Disconnect::new(2, "bye").expect("current disconnect"))
            },
            mutate: |record| {
                if let ControlRecord::Disconnect(disconnect) = record {
                    disconnect.code = 6;
                }
            },
            error: ProtocolError::InvalidEnum,
        },
        ControlMutation {
            label: "disconnect",
            build: || {
                ControlRecord::Disconnect(Disconnect::new(2, "bye").expect("current disconnect"))
            },
            mutate: |record| {
                if let ControlRecord::Disconnect(disconnect) = record {
                    disconnect.message = "a".repeat(257);
                }
            },
            error: ProtocolError::InvalidString,
        },
    ]
}

#[test]
fn control_invalid_value_wins_over_short_capacity() {
    for mutation in control_mutations() {
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
fn control_login_success_has_no_invalid_value_to_mutate_into() {
    // The login success record carries a checked identity and a seed with no
    // value rule, so no field mutation can make it invalid. The family still
    // exposes the common surface: the gate exists, stays total, and both fields
    // can be rewritten without breaking the record.
    let mut success = LoginSuccess::new(control_player_id(), 0);
    success.validate().expect("a zero seed is legal");
    success.world_seed = u64::MAX;
    success.validate().expect("a maximum seed is legal");
    let other = mornlea_protocol::PlayerId::try_from_bytes([
        0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x4f, 0xff, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ])
    .expect("another uuid v4 identity");
    success.player_id = other;
    success.validate().expect("every checked identity is legal");
    let payload = success.encode().expect("encode the mutated record");
    assert_eq!(payload.len(), 24);
    assert_eq!(&payload[..16], &other.bytes());
    assert_eq!(&payload[16..], &u64::MAX.to_le_bytes());
}

#[test]
fn control_decode_rejects_every_proper_truncation() {
    for vector in control_vectors() {
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
fn control_decode_rejects_one_trailing_byte() {
    for vector in control_vectors() {
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
fn control_decode_rejects_the_pinned_invalid_values() {
    assert_eq!(
        ServerHello::decode(&[0x2c]),
        Err(ProtocolError::UnsupportedVersion)
    );

    assert_eq!(
        HandshakeReject::decode(&[0x2a, 0x02, 0x00]),
        Err(ProtocolError::InvalidEnum)
    );
    assert_eq!(
        HandshakeReject::decode(&[0x2a, 0x01, 0x01, 0xff]),
        Err(ProtocolError::InvalidString)
    );
    let mut oversized = vec![0x2a, 0x01];
    oversized.extend(mornlea_protocol::encode_uvarint(257));
    oversized.extend(std::iter::repeat_n(b'a', 257));
    assert_eq!(
        HandshakeReject::decode(&oversized),
        Err(ProtocolError::InvalidString)
    );

    assert_eq!(
        LoginSuccess::decode(&[0; 24]),
        Err(ProtocolError::InvalidIdentity)
    );
    assert_eq!(
        LoginSuccess::decode(&[0; 15]),
        Err(ProtocolError::Truncated)
    );

    assert_eq!(
        LoginReject::decode(&[0x00, 0x00]),
        Err(ProtocolError::InvalidEnum)
    );
    assert_eq!(
        LoginReject::decode(&[0x08, 0x00]),
        Err(ProtocolError::InvalidEnum)
    );
    assert_eq!(
        LoginReject::decode(&[0x02, 0x01, 0xff]),
        Err(ProtocolError::InvalidString)
    );
    let mut oversized = vec![0x02];
    oversized.extend(mornlea_protocol::encode_uvarint(257));
    oversized.extend(std::iter::repeat_n(b'a', 257));
    assert_eq!(
        LoginReject::decode(&oversized),
        Err(ProtocolError::InvalidString)
    );

    assert_eq!(KeepAlive::decode(&[0; 8]), Err(ProtocolError::InvalidRange));
    assert_eq!(
        KeepAliveReply::decode(&[0; 8]),
        Err(ProtocolError::InvalidRange)
    );

    assert_eq!(
        Disconnect::decode(&[0x00, 0x00]),
        Err(ProtocolError::InvalidEnum)
    );
    assert_eq!(
        Disconnect::decode(&[0x06, 0x00]),
        Err(ProtocolError::InvalidEnum)
    );
    assert_eq!(
        Disconnect::decode(&[0x02, 0x01, 0xff]),
        Err(ProtocolError::InvalidString)
    );
    let mut oversized = vec![0x02];
    oversized.extend(mornlea_protocol::encode_uvarint(257));
    oversized.extend(std::iter::repeat_n(b'a', 257));
    assert_eq!(
        Disconnect::decode(&oversized),
        Err(ProtocolError::InvalidString)
    );
}

#[test]
fn control_an_incomplete_message_payload_is_truncated() {
    // A declared message length the remaining payload cannot complete is an
    // incomplete payload, so the control message reader reports `Truncated`
    // rather than the `InvalidString` the shared string primitive reports for
    // the same bytes: the Go decoder answers that condition with the same
    // sentinel as a malformed UTF-8 message, and the frozen corpus category for
    // an incomplete payload is `truncated`. The reader owns that boundary for
    // the three message-carrying control families, so the corpus and the Rust
    // consumer publish one category.
    assert_eq!(
        HandshakeReject::decode(&[0x2a, 0x01, 0x02, b'n']),
        Err(ProtocolError::Truncated)
    );
    assert_eq!(
        HandshakeReject::decode(&[0x2a, 0x01, 0x02]),
        Err(ProtocolError::Truncated)
    );
    assert_eq!(
        LoginReject::decode(&[0x01, 0x02, b'n']),
        Err(ProtocolError::Truncated)
    );
    assert_eq!(
        Disconnect::decode(&[0x06, 0x02, b'b']),
        Err(ProtocolError::Truncated)
    );
}

#[test]
fn control_packet_ids_and_reject_code_tables_are_pinned() {
    assert_eq!(ServerHello::PACKET_ID, 0);
    assert_eq!(HandshakeReject::PACKET_ID, 1);
    assert_eq!(LoginSuccess::PACKET_ID, 0);
    assert_eq!(LoginReject::PACKET_ID, 1);
    assert_eq!(KeepAlive::PACKET_ID, 5);
    assert_eq!(KeepAliveReply::PACKET_ID, 4);
    assert_eq!(Disconnect::PACKET_ID, 6);

    assert_eq!(mornlea_protocol::HANDSHAKE_VERSION_MISMATCH, 1);
    assert_eq!(mornlea_protocol::LOGIN_SERVER_FULL, 1);
    assert_eq!(mornlea_protocol::LOGIN_INVALID_IDENTITY, 2);
    assert_eq!(mornlea_protocol::LOGIN_PLAYER_DATA_CORRUPT, 3);
    assert_eq!(mornlea_protocol::LOGIN_STORE_UNAVAILABLE, 4);
    assert_eq!(mornlea_protocol::LOGIN_PROTOCOL_VIOLATION, 5);
    assert_eq!(mornlea_protocol::LOGIN_INTERNAL_ERROR, 6);
    assert_eq!(mornlea_protocol::LOGIN_ALREADY_ONLINE, 7);
    assert_eq!(mornlea_protocol::DISCONNECT_PROTOCOL_VIOLATION, 1);
    assert_eq!(mornlea_protocol::DISCONNECT_TIMEOUT, 2);
    assert_eq!(mornlea_protocol::DISCONNECT_SERVER_SHUTDOWN, 3);
    assert_eq!(mornlea_protocol::DISCONNECT_SLOW_CLIENT, 4);
    assert_eq!(mornlea_protocol::DISCONNECT_INTERNAL_ERROR, 5);
}

#[test]
fn control_wide_values_keep_their_wire_shapes() {
    // The version varint grows past one byte, the token and seed stay
    // little-endian, and the version a reject answers with is informational,
    // so it does not have to be the current protocol version.
    let hello_payload = ServerHello::new(45)
        .expect("hello")
        .encode()
        .expect("encode");
    assert_eq!(hello_payload, vec![45]);

    let reject = HandshakeReject::new(128, 1, "").expect("reject");
    let payload = reject.encode().expect("encode");
    assert_eq!(payload, vec![0x80, 0x01, 0x01, 0x00]);
    assert_eq!(HandshakeReject::decode(&payload), Ok(reject));

    let keep_alive = KeepAlive::new(0x0102).expect("keep alive");
    let payload = keep_alive.encode().expect("encode");
    assert_eq!(
        payload,
        vec![0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]
    );
    assert_eq!(KeepAlive::decode(&payload), Ok(keep_alive));

    let reply = KeepAliveReply::new(u64::MAX).expect("reply");
    let payload = reply.encode().expect("encode");
    assert_eq!(payload, vec![0xff; 8]);
    assert_eq!(KeepAliveReply::decode(&payload), Ok(reply));

    let success = LoginSuccess::new(control_player_id(), 1);
    let payload = success.encode().expect("encode");
    let mut want = CONTROL_PLAYER_ID.to_vec();
    want.extend_from_slice(&1u64.to_le_bytes());
    assert_eq!(payload, want);
    assert_eq!(LoginSuccess::decode(&payload), Ok(success));
}
