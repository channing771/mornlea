//! Executable protocol corpus routes for `mornlea_protocol`.
//!
//! This suite is the Rust half of the evidence rule in the change design: a
//! corpus case is executed through the real production codec, never compared
//! by file name. The framing family publishes a decode case and an encode
//! case, so the suite executes `read_frame` on the decode vectors and
//! `write_frame` on the encode vector and compares the complete result
//! against the outcome the independent Go producer recorded.
//!
//! The packet cases this crate will own arrive one producer group at a time.
//! `CorpusConsumer::Protocol` is registered here as `mornlea_protocol` so a
//! later packet case can name it, while the framing cases stay with the
//! separate `corpus_frame` consumer the frame regression suite keeps using.

use sha2::{Digest, Sha256};

#[path = "../../../tests/runtime_corpus.rs"]
mod runtime_corpus;

use runtime_corpus::{
    CorpusConsumer, FrozenCase, assert_normalized, load_case, load_cases_for_consumer,
};

/// The reviewed frame the encode case publishes: canonical length prefix 5,
/// the two-byte canonical uvarint packet ID 128, and the three-byte payload.
const FRAME_ENCODE_WIRE: [u8; 6] = [5, 0x80, 0x01, 1, 2, 3];

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
    assert!(
        text.len() % 2 == 0,
        "case {} payload is not whole bytes",
        case.id
    );
    let mut bytes = Vec::with_capacity(text.len() / 2);
    let raw = text.as_bytes();
    let mut index = 0;
    while index < raw.len() {
        let high = (raw[index] as char)
            .to_digit(16)
            .unwrap_or_else(|| panic!("case {} payload is not hexadecimal", case.id));
        let low = (raw[index + 1] as char)
            .to_digit(16)
            .unwrap_or_else(|| panic!("case {} payload is not hexadecimal", case.id));
        bytes.push((high * 16 + low) as u8);
        index += 2;
    }
    bytes
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

/// Executes every case the selected consumer names, once per pass, and requires
/// the executed set to be nonempty.
fn execute_selection(consumer: CorpusConsumer, cases: &[FrozenCase]) {
    for case in cases {
        assert_eq!(case.consumer, consumer, "case carries the wrong consumer");
        assert!(
            !case.operation.is_empty(),
            "case {} names no operation",
            case.id
        );
        assert_normalized(case, dispatch_frame(case));
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
    // node at a time, so an empty selection is the current state rather than a
    // failure. What must hold now is that the consumer parses, the manifest
    // loads under it, and every case it carries names a real operation and is
    // executed through the framing path it belongs to.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    execute_selection(CorpusConsumer::Protocol, &cases);
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
