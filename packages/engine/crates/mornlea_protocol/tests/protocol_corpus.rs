//! Executable protocol corpus routes for `mornlea_protocol`.
//!
//! This suite is the Rust half of the evidence rule in the change design: a
//! corpus case is executed through the real production codec, never compared
//! by file name. The framing family publishes a decode case and an encode
//! case, so the suite executes `read_frame` on the decode vectors and
//! `write_frame` on the encode vector and compares the complete result
//! against the outcome the independent Go producer recorded.
//!
//! The packet cases this crate owns arrive one producer group at a time. The
//! first group is the inbound negotiation pair: `protocol.client.ClientHello`
//! and `protocol.client.LoginStart` each publish a decode and an encode
//! operation, executed here through `ClientHello::decode_inbound` /
//! `LoginStart::decode_inbound` and the strict outbound record encoders.
//! `CorpusConsumer::Protocol` is registered here as `mornlea_protocol` so a
//! packet case can name it, while the framing cases stay with the separate
//! `corpus_frame` consumer the frame regression suite keeps using.
//!
//! A packet case whose assets the controller has not integrated yet fails
//! here as a missing corpus case rather than as a silently empty selection,
//! which is what keeps the staged state visible until the merge lands.

use sha2::{Digest, Sha256};

#[path = "../../../tests/runtime_corpus.rs"]
mod runtime_corpus;

use runtime_corpus::{
    CorpusConsumer, FrozenCase, assert_normalized, load_case, load_cases_for_consumer,
};

/// The reviewed frame the encode case publishes: canonical length prefix 5,
/// the two-byte canonical uvarint packet ID 128, and the three-byte payload.
const FRAME_ENCODE_WIRE: [u8; 6] = [5, 0x80, 0x01, 1, 2, 3];

/// The two packet families the negotiation producer group registers.
const CLIENT_HELLO_FAMILY: &str = "protocol.client.ClientHello";
const LOGIN_START_FAMILY: &str = "protocol.client.LoginStart";
/// The packet families' protocol version, matching the manifest family rows.
const PACKET_VERSION: &str = "45";

/// The case identities the negotiation group registers. They mirror the Go
/// producer's registration, so a case that only one side names is a mismatch
/// rather than a shared name.
const NEGOTIATION_CASE_IDS: [&str; 10] = [
    "protocol.client.ClientHello/45/decode-current-version",
    "protocol.client.ClientHello/45/encode-current-version",
    "protocol.client.ClientHello/45/decode-trailing-byte",
    "protocol.client.ClientHello/45/decode-noncanonical-version",
    "protocol.client.LoginStart/45/decode-valid",
    "protocol.client.LoginStart/45/encode-valid",
    "protocol.client.LoginStart/45/decode-missing-distance",
    "protocol.client.LoginStart/45/decode-trailing-byte",
    "protocol.client.LoginStart/45/decode-invalid-utf8-name",
    "protocol.client.LoginStart/45/decode-oversized-payload",
];

fn hex_lower(bytes: &[u8]) -> String {
    let mut rendered = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        rendered.push_str(&format!("{byte:02x}"));
    }
    rendered
}

/// Renders one payload hex field as bytes, so a producer and a consumer read
/// the same canonical field encoding.
fn payload_bytes(case: &FrozenCase) -> Vec<u8> {
    let text = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get("payload")
        .and_then(|value| value.as_str())
        .unwrap_or_else(|| panic!("case {} names no payload", case.id));
    payload_bytes_from_text(case, text)
}

/// Executes one framing case through the real Rust framing path named by its
/// operation and returns the normalized outcome.
///
/// The dispatch is a match over the actual codec entry points: `decode` runs
/// `read_frame` and `encode` runs `write_frame`. A rejection is classified from
/// the error the codec actually returned, so the category stays the signal both
/// implementations publish rather than a message string.
fn dispatch_frame(case: &FrozenCase) -> serde_json::Value {
    assert_eq!(case.family, "protocol.frame", "unexpected corpus family");
    assert!(
        case.arguments.is_null(),
        "framing carries no typed arguments, got {}",
        case.arguments
    );
    match case.operation.as_str() {
        "decode" => match mornlea_protocol::read_frame(&case.input) {
            Ok((packet_id, payload, used)) => {
                assert_eq!(
                    used,
                    case.input.len(),
                    "frame reader left {} of {} bytes unconsumed",
                    case.input.len() - used,
                    case.input.len()
                );
                serde_json::json!({
                    "category": "frame",
                    "fields": {
                        "packet_id": packet_id,
                        "payload": hex_lower(&payload),
                        "payload_len": payload.len()
                    },
                    "kind": "ok"
                })
            }
            Err(err) => {
                let category = match err {
                    mornlea_protocol::ProtocolError::NonCanonicalUvarint => "invalid-varint",
                    other => panic!("unclassified framing rejection for {}: {other:?}", case.id),
                };
                serde_json::json!({
                    "category": category,
                    "kind": "error"
                })
            }
        },
        "encode" => {
            let packet_id = case
                .input_json
                .as_ref()
                .expect("encode case carries JSON fields")
                .get("packet_id")
                .and_then(|value| value.as_u64())
                .unwrap_or_else(|| panic!("case {} names no packet_id", case.id));
            let packet_id = u32::try_from(packet_id).expect("packet_id fits u32");
            let payload = payload_bytes(case);
            let wire = mornlea_protocol::write_frame(packet_id, &payload)
                .unwrap_or_else(|err| panic!("case {} encode failed: {err:?}", case.id));
            let mut hasher = Sha256::new();
            hasher.update(&wire);
            let digest = format!("sha256:{:x}", hasher.finalize());
            serde_json::json!({
                "category": "frame",
                "encoded_payload_digest": digest,
                "fields": {
                    "packet_id": packet_id,
                    "payload": hex_lower(&payload),
                    "payload_len": payload.len()
                },
                "kind": "ok"
            })
        }
        other => panic!("unsupported framing operation for {}: {other}", case.id),
    }
}

/// Resolves the language-neutral rejection category for one real Rust codec
/// failure.
///
/// The category names the wire condition the Go producer publishes for the same
/// input, so the mapping is over the error variants this crate can return for a
/// packet payload and never over a message string. A variant with no mapping is
/// a panic, because an unclassified rejection must never become corpus
/// evidence.
fn packet_rejection_category(err: mornlea_protocol::ProtocolError) -> String {
    let category = match err {
        mornlea_protocol::ProtocolError::NonCanonicalUvarint
        | mornlea_protocol::ProtocolError::InvalidUvarint => "invalid-varint",
        mornlea_protocol::ProtocolError::Truncated
        | mornlea_protocol::ProtocolError::EmptyFrame => "truncated",
        mornlea_protocol::ProtocolError::TrailingBytes => "trailing",
        mornlea_protocol::ProtocolError::InvalidEnum
        | mornlea_protocol::ProtocolError::UnknownPacket => "invalid-enum",
        mornlea_protocol::ProtocolError::InvalidIdentity => "invalid-identity",
        mornlea_protocol::ProtocolError::UnsupportedVersion => "unsupported-version",
        mornlea_protocol::ProtocolError::InvalidString
        | mornlea_protocol::ProtocolError::InvalidRange
        | mornlea_protocol::ProtocolError::InvalidFloat => "invalid-value",
        mornlea_protocol::ProtocolError::FrameTooLarge
        | mornlea_protocol::ProtocolError::Allocation
        | mornlea_protocol::ProtocolError::OutputTooSmall { .. } => "capacity",
    };
    category.to_owned()
}

/// Publishes one rejection outcome in the corpus's normalized shape.
fn packet_error(err: mornlea_protocol::ProtocolError) -> serde_json::Value {
    serde_json::json!({
        "category": packet_rejection_category(err),
        "kind": "error"
    })
}

/// Reads the canonical `player_id` hex field one encode case carries.
fn player_id_bytes(case: &FrozenCase) -> [u8; 16] {
    let text = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get("player_id")
        .and_then(|value| value.as_str())
        .unwrap_or_else(|| panic!("case {} names no player_id", case.id));
    let bytes = payload_bytes_from_text(case, text);
    bytes
        .try_into()
        .unwrap_or_else(|_| panic!("case {} player_id is not 16 bytes", case.id))
}

/// Reads the canonical `display_name` field one encode case carries.
fn display_name_field(case: &FrozenCase) -> String {
    case.input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get("display_name")
        .and_then(|value| value.as_str().map(str::to_owned))
        .unwrap_or_else(|| panic!("case {} names no display_name", case.id))
}

/// Reads the canonical `view_distance` field one encode case carries.
fn view_distance_field(case: &FrozenCase) -> u8 {
    case.input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get("view_distance")
        .and_then(|value| value.as_u64())
        .and_then(|value| u8::try_from(value).ok())
        .unwrap_or_else(|| panic!("case {} names no view_distance", case.id))
}

/// Decodes one hexadecimal field, reporting the case it belongs to.
fn payload_bytes_from_text(case: &FrozenCase, text: &str) -> Vec<u8> {
    assert!(
        text.len().is_multiple_of(2),
        "case {} field is not whole bytes",
        case.id
    );
    let raw = text.as_bytes();
    let mut bytes = Vec::with_capacity(raw.len() / 2);
    let mut index = 0;
    while index < raw.len() {
        let high = (raw[index] as char)
            .to_digit(16)
            .unwrap_or_else(|| panic!("case {} field is not hexadecimal", case.id));
        let low = (raw[index + 1] as char)
            .to_digit(16)
            .unwrap_or_else(|| panic!("case {} field is not hexadecimal", case.id));
        bytes.push((high * 16 + low) as u8);
        index += 2;
    }
    bytes
}

/// Executes one packet case through the real Rust path its operation names.
///
/// A decode case runs the family's inbound decoder, which applies only the
/// structural rules; an encode case builds the strict outbound record and
/// encodes it. Both directions publish the same semantic field names the Go
/// producer records, and an encode case carries the encoded digest so the
/// comparison covers the exact bytes rather than their length.
fn dispatch_packet(case: &FrozenCase) -> serde_json::Value {
    assert_eq!(case.version, PACKET_VERSION, "unexpected packet version");
    match case.family.as_str() {
        CLIENT_HELLO_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ClientHello::decode_inbound(&case.input) {
                Ok(hello) => serde_json::json!({
                    "category": "packet",
                    "fields": {"protocol_version": hello.protocol_version()},
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let version = case
                    .input_json
                    .as_ref()
                    .expect("encode case carries JSON fields")
                    .get("protocol_version")
                    .and_then(|value| value.as_u64())
                    .and_then(|value| u32::try_from(value).ok())
                    .unwrap_or_else(|| panic!("case {} names no protocol_version", case.id));
                let hello = mornlea_protocol::ClientHello::new(version)
                    .unwrap_or_else(|err| panic!("case {} encode refused: {err:?}", case.id));
                let wire = hello.encode();
                let mut hasher = Sha256::new();
                hasher.update(&wire);
                serde_json::json!({
                    "category": "packet",
                    "encoded_payload_digest": format!("sha256:{:x}", hasher.finalize()),
                    "fields": {"protocol_version": version},
                    "kind": "ok"
                })
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        LOGIN_START_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::LoginStart::decode_inbound(&case.input) {
                Ok(start) => serde_json::json!({
                    "category": "packet",
                    "fields": {
                        "display_name": start.display_name(),
                        "player_id": hex_lower(start.player_id()),
                        "view_distance": start.view_distance()
                    },
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let player_id = mornlea_protocol::PlayerId::try_from_bytes(player_id_bytes(case))
                    .unwrap_or_else(|_| panic!("case {} names an invalid identity", case.id));
                let start = mornlea_protocol::LoginStart::new(
                    player_id,
                    display_name_field(case),
                    view_distance_field(case),
                )
                .unwrap_or_else(|err| panic!("case {} encode refused: {err:?}", case.id));
                let wire = start.encode();
                let mut hasher = Sha256::new();
                hasher.update(&wire);
                serde_json::json!({
                    "category": "packet",
                    "encoded_payload_digest": format!("sha256:{:x}", hasher.finalize()),
                    "fields": {
                        "display_name": start.display_name,
                        "player_id": hex_lower(&start.player_id.bytes()),
                        "view_distance": start.view_distance
                    },
                    "kind": "ok"
                })
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        other => panic!("unsupported packet family for {}: {other}", case.id),
    }
}

/// Executes one case of any registered protocol family.
///
/// The dispatch is a match over the case's own family and operation, so a case
/// naming a family this suite does not execute fails loudly instead of being
/// reported as covered.
fn dispatch_case(case: &FrozenCase) -> serde_json::Value {
    match case.family.as_str() {
        "protocol.frame" => dispatch_frame(case),
        CLIENT_HELLO_FAMILY | LOGIN_START_FAMILY => dispatch_packet(case),
        other => panic!("unregistered protocol family for {}: {other}", case.id),
    }
}

/// Executes every case the selected consumer names, once per pass.
fn execute_selection(consumer: CorpusConsumer, cases: &[FrozenCase]) {
    for case in cases {
        assert_eq!(case.consumer, consumer, "case carries the wrong consumer");
        assert!(
            !case.operation.is_empty(),
            "case {} names no operation",
            case.id
        );
        assert_normalized(case, dispatch_case(case));
    }
}

#[test]
fn protocol_corpus_executes_every_frame_case() {
    let cases = load_cases_for_consumer(CorpusConsumer::Frame);
    assert!(
        !cases.is_empty(),
        "the framing consumer selection executed zero cases"
    );
    execute_selection(CorpusConsumer::Frame, &cases);
}

#[test]
fn protocol_corpus_packet_consumer_selection_is_executable() {
    // The packet consumer is registered for the packet groups that arrive one
    // node at a time. What must hold is that the consumer parses, the manifest
    // loads under it, and every case it carries names a real operation and is
    // executed through the codec path it belongs to.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    execute_selection(CorpusConsumer::Protocol, &cases);
}

/// The negotiation group's cases execute through the real inbound decoders and
/// the strict outbound encoders.
///
/// The case assets are exported by the Go producer and integrated by the
/// controller, so before that merge this suite reports the missing corpus case
/// instead of an empty selection that would look like a passing run.
#[test]
fn protocol_corpus_packet_negotiation_cases_are_executed() {
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let negotiation: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| case.family == CLIENT_HELLO_FAMILY || case.family == LOGIN_START_FAMILY)
        .collect();
    let executed: Vec<&str> = negotiation.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = NEGOTIATION_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the negotiation selection does not carry the reviewed case set"
    );
    assert!(
        !negotiation.is_empty(),
        "the negotiation selection executed zero cases"
    );
    for case in negotiation {
        assert_eq!(
            case.consumer,
            CorpusConsumer::Protocol,
            "case {} carries the wrong consumer",
            case.id
        );
        assert!(
            !case.operation.is_empty(),
            "case {} names no operation",
            case.id
        );
        assert_normalized(case, dispatch_packet(case));
    }
}

#[test]
fn protocol_corpus_frame_encode_matches_the_reviewed_wire() {
    let case = load_case("protocol.frame/45/encode-id-128");
    assert_eq!(case.family, "protocol.frame");
    assert_eq!(case.operation, "encode");

    let packet_id = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get("packet_id")
        .and_then(|value| value.as_u64())
        .expect("encode case names a packet_id");
    assert_eq!(packet_id, 128, "encode case names packet id {packet_id}");
    let payload = payload_bytes(&case);
    assert_eq!(
        payload,
        vec![1, 2, 3],
        "encode case names payload {payload:?}"
    );

    // The full output is compared, not only its length: the writer has to
    // publish the canonical length prefix and the two-byte uvarint ID exactly
    // as the reviewed frame carries them.
    let wire =
        mornlea_protocol::write_frame(u32::try_from(packet_id).expect("id fits u32"), &payload)
            .expect("frame encode");
    assert_eq!(
        wire, FRAME_ENCODE_WIRE,
        "encoded frame is {wire:?}, want {FRAME_ENCODE_WIRE:?}"
    );

    // The produced frame has to decode back through the production reader,
    // so an encoding the reader rejects can never be recorded as evidence.
    let (decoded_id, decoded_payload, used) =
        mornlea_protocol::read_frame(&wire).expect("read back the encoded frame");
    assert_eq!(decoded_id, u32::try_from(packet_id).expect("id fits u32"));
    assert_eq!(decoded_payload, payload);
    assert_eq!(used, wire.len(), "frame reader left bytes unconsumed");

    let normalized = dispatch_frame(&case);
    assert_normalized(&case, normalized.clone());

    // A field mutation has to fail comparison: replacing the expected packet id
    // with 127, or the encoded digest with the digest of that different frame,
    // is a different expectation than the one this case records.
    let mut mutated_id = normalized.clone();
    mutated_id["fields"]["packet_id"] = serde_json::json!(127);
    assert_ne!(case.normalized, mutated_id);

    let mut mutated_digest = normalized;
    mutated_digest["encoded_payload_digest"] = serde_json::json!("sha256:0");
    assert_ne!(case.normalized, mutated_digest);
}
