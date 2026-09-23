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
//! `LoginStart::decode_inbound` and the strict outbound record encoders. The
//! second group is the seven non-gameplay control families, executed here
//! through their common fallible surface (`validate` → checked `encoded_len`
//! → capacity check → `encode_into`). The third group is the four Play
//! client-to-server control families (`PlayerInput`, `PlaceBlock`,
//! `RequestChunkResync` and `SelectHotbar`), executed here through the same
//! fallible surface, with the look angles published as their exact IEEE-754
//! bits. The fourth group is the five Play client-to-server ray action
//! families (`OpenContainer`, `TillSoil`, `BoneMeal`, `CollectWater` and
//! `PlaceWater`), executed through that same surface; their payload carries
//! only the sequence and the two look angles, because the ray-cast target, the
//! held item, the container kind and the resulting write stay server-owned.
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
/// The seven packet families the control producer group registers.
const SERVER_HELLO_FAMILY: &str = "protocol.server.ServerHello";
const HANDSHAKE_REJECT_FAMILY: &str = "protocol.server.HandshakeReject";
const LOGIN_SUCCESS_FAMILY: &str = "protocol.server.LoginSuccess";
const LOGIN_REJECT_FAMILY: &str = "protocol.server.LoginReject";
const KEEP_ALIVE_FAMILY: &str = "protocol.server.KeepAlive";
const KEEP_ALIVE_REPLY_FAMILY: &str = "protocol.client.KeepAliveReply";
const DISCONNECT_FAMILY: &str = "protocol.server.Disconnect";
/// The four packet families the client control producer group registers.
const PLAYER_INPUT_FAMILY: &str = "protocol.client.PlayerInput";
const PLACE_BLOCK_FAMILY: &str = "protocol.client.PlaceBlock";
const REQUEST_CHUNK_RESYNC_FAMILY: &str = "protocol.client.RequestChunkResync";
const SELECT_HOTBAR_FAMILY: &str = "protocol.client.SelectHotbar";
/// The five packet families the client ray producer group registers.
const OPEN_CONTAINER_FAMILY: &str = "protocol.client.OpenContainer";
const TILL_SOIL_FAMILY: &str = "protocol.client.TillSoil";
const BONE_MEAL_FAMILY: &str = "protocol.client.BoneMeal";
const COLLECT_WATER_FAMILY: &str = "protocol.client.CollectWater";
const PLACE_WATER_FAMILY: &str = "protocol.client.PlaceWater";
/// The packet families' protocol version, matching the manifest family rows.
const PACKET_VERSION: &str = "45";
/// The category label every accepted control packet outcome publishes.
const PACKET_OUTCOME_CATEGORY: &str = "packet";

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

/// Reads one unsigned numeric field one encode case carries.
fn unsigned_field(case: &FrozenCase, name: &str) -> u64 {
    case.input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_u64())
        .unwrap_or_else(|| panic!("case {} names no {name}", case.id))
}

/// Reads one small unsigned field one encode case carries.
fn byte_field(case: &FrozenCase, name: &str) -> u8 {
    u8::try_from(unsigned_field(case, name))
        .unwrap_or_else(|_| panic!("case {} field {name} exceeds u8", case.id))
}

/// Reads one text field one encode case carries.
fn text_field(case: &FrozenCase, name: &str) -> String {
    case.input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_str().map(str::to_owned))
        .unwrap_or_else(|| panic!("case {} names no {name}", case.id))
}

/// Renders one f32 as its eight-digit lowercase-hexadecimal bit string.
///
/// The corpus's canonical float encoding is the bit pattern rather than the
/// numeric value, so a negative zero and a positive zero stay distinguishable
/// after a JSON round trip.
fn float_bits_text(value: f32) -> String {
    format!("{:08x}", value.to_bits())
}

/// Reads one f32 bit-string field one encode case carries.
fn float_bits_field(case: &FrozenCase, name: &str) -> f32 {
    let text = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_str())
        .unwrap_or_else(|| panic!("case {} names no {name}", case.id));
    let bits = u32::from_str_radix(text, 16)
        .unwrap_or_else(|_| panic!("case {} field {name} is not hexadecimal bits", case.id));
    f32::from_bits(bits)
}

/// Reads one ray encode case's canonical request fields: the sequence and the
/// two look-angle bit strings.
fn client_ray_request(case: &FrozenCase) -> (u64, f32, f32) {
    (
        unsigned_field(case, "sequence"),
        float_bits_field(case, "yaw"),
        float_bits_field(case, "pitch"),
    )
}

/// Renders the semantic fields one client ray packet publishes, shared by the
/// decode and encode arms so both publish the same canonical field encoding.
fn client_ray_fields(sequence: u64, yaw: f32, pitch: f32) -> serde_json::Value {
    serde_json::json!({
        "sequence": sequence.to_string(),
        "yaw": float_bits_text(yaw),
        "pitch": float_bits_text(pitch)
    })
}

/// Reads one signed move-axis field one encode case carries.
fn signed_axis_field(case: &FrozenCase, name: &str) -> i8 {
    let value = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_i64())
        .unwrap_or_else(|| panic!("case {} names no {name}", case.id));
    i8::try_from(value).unwrap_or_else(|_| panic!("case {} field {name} exceeds i8", case.id))
}

/// Reads one boolean field one encode case carries.
fn bool_field(case: &FrozenCase, name: &str) -> bool {
    case.input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_bool())
        .unwrap_or_else(|| panic!("case {} names no {name}", case.id))
}

/// Reads one signed 32-bit chunk coordinate field one encode case carries.
fn i32_field(case: &FrozenCase, name: &str) -> i32 {
    let value = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_i64())
        .unwrap_or_else(|| panic!("case {} names no {name}", case.id));
    i32::try_from(value).unwrap_or_else(|_| panic!("case {} field {name} exceeds i32", case.id))
}

/// Renders one player input record's semantic fields, shared by the decode and
/// encode arms so both publish the same canonical field encoding.
fn player_input_fields(input: &mornlea_protocol::PlayerInput) -> serde_json::Value {
    serde_json::json!({
        "sequence": input.sequence.to_string(),
        "move_x": input.move_x,
        "move_z": input.move_z,
        "jump": input.jump,
        "yaw": float_bits_text(input.yaw),
        "pitch": float_bits_text(input.pitch),
        "mining": input.mining,
        "eating": input.eating,
        "sprinting": input.sprinting,
        "sneaking": input.sneaking
    })
}

/// Digests one produced payload the way the corpus records an encode outcome.
fn payload_digest(payload: &[u8]) -> String {
    let mut hasher = Sha256::new();
    hasher.update(payload);
    format!("sha256:{:x}", hasher.finalize())
}

/// Publishes one encode case's outcome: the produced bytes' digest beside the
/// semantic fields on success, or the rejection category on failure.
///
/// The record is built through the packet's public fields, so a mutated or
/// invalid encode case is refused by the same production validation the
/// positive path applies, and the outcome stays a record of execution.
fn encode_ok_outcome(
    encoded: Result<Vec<u8>, mornlea_protocol::ProtocolError>,
    fields: serde_json::Value,
) -> serde_json::Value {
    match encoded {
        Ok(wire) => serde_json::json!({
            "category": PACKET_OUTCOME_CATEGORY,
            "encoded_payload_digest": payload_digest(&wire),
            "fields": fields,
            "kind": "ok"
        }),
        Err(err) => packet_error(err),
    }
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
        SERVER_HELLO_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ServerHello::decode(&case.input) {
                Ok(hello) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {"protocol_version": hello.protocol_version},
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let version = u32::try_from(unsigned_field(case, "protocol_version"))
                    .unwrap_or_else(|_| panic!("case {} protocol_version exceeds u32", case.id));
                // The record is built through its public field, so an invalid
                // version is refused by the production validation rather than
                // by a constructor guard.
                let hello = mornlea_protocol::ServerHello {
                    protocol_version: version,
                };
                encode_ok_outcome(
                    hello.encode(),
                    serde_json::json!({"protocol_version": version}),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        HANDSHAKE_REJECT_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::HandshakeReject::decode(&case.input) {
                Ok(reject) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {
                        "server_protocol_version": reject.server_protocol_version,
                        "code": reject.code,
                        "message": reject.message
                    },
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let reject = mornlea_protocol::HandshakeReject {
                    server_protocol_version: u32::try_from(unsigned_field(
                        case,
                        "server_protocol_version",
                    ))
                    .unwrap_or_else(|_| {
                        panic!("case {} server_protocol_version exceeds u32", case.id)
                    }),
                    code: byte_field(case, "code"),
                    message: text_field(case, "message"),
                };
                encode_ok_outcome(
                    reject.encode(),
                    serde_json::json!({
                        "server_protocol_version": reject.server_protocol_version,
                        "code": reject.code,
                        "message": reject.message
                    }),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        LOGIN_SUCCESS_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::LoginSuccess::decode(&case.input) {
                Ok(success) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {
                        "player_id": hex_lower(&success.player_id.bytes()),
                        "world_seed": success.world_seed.to_string()
                    },
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let player_id = mornlea_protocol::PlayerId::try_from_bytes(player_id_bytes(case))
                    .unwrap_or_else(|_| panic!("case {} names an invalid identity", case.id));
                let world_seed = unsigned_field(case, "world_seed");
                let success = mornlea_protocol::LoginSuccess {
                    player_id,
                    world_seed,
                };
                encode_ok_outcome(
                    success.encode(),
                    serde_json::json!({
                        "player_id": hex_lower(&success.player_id.bytes()),
                        "world_seed": success.world_seed.to_string()
                    }),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        LOGIN_REJECT_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::LoginReject::decode(&case.input) {
                Ok(reject) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {"code": reject.code, "message": reject.message},
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let reject = mornlea_protocol::LoginReject {
                    code: byte_field(case, "code"),
                    message: text_field(case, "message"),
                };
                encode_ok_outcome(
                    reject.encode(),
                    serde_json::json!({"code": reject.code, "message": reject.message}),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        KEEP_ALIVE_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::KeepAlive::decode(&case.input) {
                Ok(keep_alive) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {"token": keep_alive.token.to_string()},
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let keep_alive = mornlea_protocol::KeepAlive {
                    token: unsigned_field(case, "token"),
                };
                encode_ok_outcome(
                    keep_alive.encode(),
                    serde_json::json!({"token": keep_alive.token.to_string()}),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        KEEP_ALIVE_REPLY_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::KeepAliveReply::decode(&case.input) {
                Ok(reply) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {"token": reply.token.to_string()},
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let reply = mornlea_protocol::KeepAliveReply {
                    token: unsigned_field(case, "token"),
                };
                encode_ok_outcome(
                    reply.encode(),
                    serde_json::json!({"token": reply.token.to_string()}),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        DISCONNECT_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::Disconnect::decode(&case.input) {
                Ok(disconnect) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {"code": disconnect.code, "message": disconnect.message},
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let disconnect = mornlea_protocol::Disconnect {
                    code: byte_field(case, "code"),
                    message: text_field(case, "message"),
                };
                encode_ok_outcome(
                    disconnect.encode(),
                    serde_json::json!({"code": disconnect.code, "message": disconnect.message}),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PLAYER_INPUT_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::PlayerInput::decode(&case.input) {
                Ok(input) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {
                        "sequence": input.sequence.to_string(),
                        "move_x": input.move_x,
                        "move_z": input.move_z,
                        "jump": input.jump,
                        "yaw": float_bits_text(input.yaw),
                        "pitch": float_bits_text(input.pitch),
                        "mining": input.mining,
                        "eating": input.eating,
                        "sprinting": input.sprinting,
                        "sneaking": input.sneaking
                    },
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The record is built through its public fields, so an invalid
                // angle is refused by the production validation rather than by
                // a constructor guard.
                let input = mornlea_protocol::PlayerInput {
                    sequence: unsigned_field(case, "sequence"),
                    move_x: signed_axis_field(case, "move_x"),
                    move_z: signed_axis_field(case, "move_z"),
                    jump: bool_field(case, "jump"),
                    yaw: float_bits_field(case, "yaw"),
                    pitch: float_bits_field(case, "pitch"),
                    mining: bool_field(case, "mining"),
                    eating: bool_field(case, "eating"),
                    sprinting: bool_field(case, "sprinting"),
                    sneaking: bool_field(case, "sneaking"),
                };
                encode_ok_outcome(input.encode(), player_input_fields(&input))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PLACE_BLOCK_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::PlaceBlock::decode(&case.input) {
                Ok(place) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {
                        "sequence": place.sequence.to_string(),
                        "yaw": float_bits_text(place.yaw),
                        "pitch": float_bits_text(place.pitch),
                        "slot": place.slot
                    },
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let place = mornlea_protocol::PlaceBlock {
                    sequence: unsigned_field(case, "sequence"),
                    yaw: float_bits_field(case, "yaw"),
                    pitch: float_bits_field(case, "pitch"),
                    slot: byte_field(case, "slot"),
                };
                encode_ok_outcome(
                    place.encode(),
                    serde_json::json!({
                        "sequence": place.sequence.to_string(),
                        "yaw": float_bits_text(place.yaw),
                        "pitch": float_bits_text(place.pitch),
                        "slot": place.slot
                    }),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        REQUEST_CHUNK_RESYNC_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::RequestChunkResync::decode(&case.input) {
                Ok(resync) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {
                        "sequence": resync.sequence.to_string(),
                        "dimension": resync.dimension.get(),
                        "chunk_x": resync.chunk_x,
                        "chunk_z": resync.chunk_z,
                        "have_revision": resync.have_revision.to_string()
                    },
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The dimension field is the checked domain value, so an
                // unknown raw dimension cannot be constructed at all: the
                // observable rejection is published directly, which keeps the
                // category identical to the Go validator's.
                let dimension = match u8::try_from(unsigned_field(case, "dimension"))
                    .ok()
                    .and_then(|value| mornlea_domain::Dimension::new(value).ok())
                {
                    Some(dimension) => dimension,
                    None => return packet_error(mornlea_protocol::ProtocolError::InvalidEnum),
                };
                let resync = mornlea_protocol::RequestChunkResync::new(
                    unsigned_field(case, "sequence"),
                    dimension,
                    i32_field(case, "chunk_x"),
                    i32_field(case, "chunk_z"),
                    unsigned_field(case, "have_revision"),
                );
                encode_ok_outcome(
                    resync.encode(),
                    serde_json::json!({
                        "sequence": resync.sequence.to_string(),
                        "dimension": resync.dimension.get(),
                        "chunk_x": resync.chunk_x,
                        "chunk_z": resync.chunk_z,
                        "have_revision": resync.have_revision.to_string()
                    }),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        SELECT_HOTBAR_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::SelectHotbar::decode(&case.input) {
                Ok(select) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": {
                        "sequence": select.sequence.to_string(),
                        "slot": select.slot
                    },
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let select = mornlea_protocol::SelectHotbar {
                    sequence: unsigned_field(case, "sequence"),
                    slot: byte_field(case, "slot"),
                };
                encode_ok_outcome(
                    select.encode(),
                    serde_json::json!({
                        "sequence": select.sequence.to_string(),
                        "slot": select.slot
                    }),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        OPEN_CONTAINER_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::OpenContainer::decode(&case.input) {
                Ok(open) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_ray_fields(open.sequence, open.yaw, open.pitch),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let (sequence, yaw, pitch) = client_ray_request(case);
                // The record is built through its public fields, so an invalid
                // angle is refused by the production validation rather than by
                // a constructor guard.
                let open = mornlea_protocol::OpenContainer {
                    sequence,
                    yaw,
                    pitch,
                };
                encode_ok_outcome(
                    open.encode(),
                    client_ray_fields(open.sequence, open.yaw, open.pitch),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        TILL_SOIL_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::TillSoil::decode(&case.input) {
                Ok(till) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_ray_fields(till.sequence, till.yaw, till.pitch),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let (sequence, yaw, pitch) = client_ray_request(case);
                let till = mornlea_protocol::TillSoil {
                    sequence,
                    yaw,
                    pitch,
                };
                encode_ok_outcome(
                    till.encode(),
                    client_ray_fields(till.sequence, till.yaw, till.pitch),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        BONE_MEAL_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::BoneMeal::decode(&case.input) {
                Ok(meal) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_ray_fields(meal.sequence, meal.yaw, meal.pitch),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let (sequence, yaw, pitch) = client_ray_request(case);
                let meal = mornlea_protocol::BoneMeal {
                    sequence,
                    yaw,
                    pitch,
                };
                encode_ok_outcome(
                    meal.encode(),
                    client_ray_fields(meal.sequence, meal.yaw, meal.pitch),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        COLLECT_WATER_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::CollectWater::decode(&case.input) {
                Ok(collect) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_ray_fields(collect.sequence, collect.yaw, collect.pitch),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let (sequence, yaw, pitch) = client_ray_request(case);
                let collect = mornlea_protocol::CollectWater {
                    sequence,
                    yaw,
                    pitch,
                };
                encode_ok_outcome(
                    collect.encode(),
                    client_ray_fields(collect.sequence, collect.yaw, collect.pitch),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PLACE_WATER_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::PlaceWater::decode(&case.input) {
                Ok(place) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_ray_fields(place.sequence, place.yaw, place.pitch),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let (sequence, yaw, pitch) = client_ray_request(case);
                let place = mornlea_protocol::PlaceWater {
                    sequence,
                    yaw,
                    pitch,
                };
                encode_ok_outcome(
                    place.encode(),
                    client_ray_fields(place.sequence, place.yaw, place.pitch),
                )
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
        CLIENT_HELLO_FAMILY
        | LOGIN_START_FAMILY
        | SERVER_HELLO_FAMILY
        | HANDSHAKE_REJECT_FAMILY
        | LOGIN_SUCCESS_FAMILY
        | LOGIN_REJECT_FAMILY
        | KEEP_ALIVE_FAMILY
        | KEEP_ALIVE_REPLY_FAMILY
        | DISCONNECT_FAMILY
        | PLAYER_INPUT_FAMILY
        | PLACE_BLOCK_FAMILY
        | REQUEST_CHUNK_RESYNC_FAMILY
        | SELECT_HOTBAR_FAMILY
        | OPEN_CONTAINER_FAMILY
        | TILL_SOIL_FAMILY
        | BONE_MEAL_FAMILY
        | COLLECT_WATER_FAMILY
        | PLACE_WATER_FAMILY => dispatch_packet(case),
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

/// The case identities the control group registers. They mirror the Go
/// producer's registration, so a case that only one side names is a mismatch
/// rather than a shared name. The merged manifest sorts case IDs, so the
/// comparison sorts this list too.
const CONTROL_CASE_IDS: [&str; 34] = [
    "protocol.client.KeepAliveReply/45/decode-valid",
    "protocol.client.KeepAliveReply/45/decode-zero-token",
    "protocol.client.KeepAliveReply/45/encode-valid",
    "protocol.client.KeepAliveReply/45/encode-zero-token",
    "protocol.server.Disconnect/45/decode-code-one-empty-message",
    "protocol.server.Disconnect/45/decode-code-six",
    "protocol.server.Disconnect/45/decode-code-zero",
    "protocol.server.Disconnect/45/decode-declared-length-above-bound",
    "protocol.server.Disconnect/45/decode-message-length-exceeds-payload",
    "protocol.server.Disconnect/45/decode-trailing-byte",
    "protocol.server.Disconnect/45/encode-code-five-maximum-message",
    "protocol.server.Disconnect/45/encode-message-above-bound",
    "protocol.server.HandshakeReject/45/decode-message-length-exceeds-payload",
    "protocol.server.HandshakeReject/45/decode-unknown-code",
    "protocol.server.HandshakeReject/45/decode-valid",
    "protocol.server.HandshakeReject/45/encode-unknown-code",
    "protocol.server.HandshakeReject/45/encode-valid",
    "protocol.server.KeepAlive/45/decode-valid",
    "protocol.server.KeepAlive/45/decode-zero-token",
    "protocol.server.KeepAlive/45/encode-valid",
    "protocol.server.LoginReject/45/decode-code-eight",
    "protocol.server.LoginReject/45/decode-code-one-empty-message",
    "protocol.server.LoginReject/45/decode-code-zero",
    "protocol.server.LoginReject/45/decode-declared-length-above-bound",
    "protocol.server.LoginReject/45/decode-message-length-exceeds-payload",
    "protocol.server.LoginReject/45/encode-code-seven-maximum-message",
    "protocol.server.LoginReject/45/encode-message-above-bound",
    "protocol.server.LoginSuccess/45/decode-invalid-uuid",
    "protocol.server.LoginSuccess/45/decode-trailing-byte",
    "protocol.server.LoginSuccess/45/decode-zero-seed",
    "protocol.server.LoginSuccess/45/encode-zero-seed",
    "protocol.server.ServerHello/45/decode-current-version",
    "protocol.server.ServerHello/45/decode-previous-version",
    "protocol.server.ServerHello/45/encode-current-version",
];

/// Reports whether one family belongs to the control producer group.
fn is_control_family(family: &str) -> bool {
    matches!(
        family,
        SERVER_HELLO_FAMILY
            | HANDSHAKE_REJECT_FAMILY
            | LOGIN_SUCCESS_FAMILY
            | LOGIN_REJECT_FAMILY
            | KEEP_ALIVE_FAMILY
            | KEEP_ALIVE_REPLY_FAMILY
            | DISCONNECT_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_control_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before that merge this test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let control: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_control_family(&case.family))
        .collect();
    let executed: Vec<&str> = control.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = CONTROL_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the control selection does not carry the reviewed case set"
    );
    for case in control {
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

/// The case identities the client control group registers. They mirror the Go
/// producer's registration, so a case that only one side names is a mismatch
/// rather than a shared name. The merged manifest sorts case IDs, so the
/// comparison sorts this list too.
const CLIENT_CONTROL_CASE_IDS: [&str; 19] = [
    "protocol.client.PlaceBlock/45/decode-infinite-pitch",
    "protocol.client.PlaceBlock/45/decode-slot-nine",
    "protocol.client.PlaceBlock/45/decode-valid",
    "protocol.client.PlaceBlock/45/encode-slot-nine",
    "protocol.client.PlaceBlock/45/encode-valid",
    "protocol.client.PlayerInput/45/decode-bool-two",
    "protocol.client.PlayerInput/45/decode-nan-yaw",
    "protocol.client.PlayerInput/45/decode-trailing-byte",
    "protocol.client.PlayerInput/45/decode-valid",
    "protocol.client.PlayerInput/45/encode-nan-yaw",
    "protocol.client.PlayerInput/45/encode-valid",
    "protocol.client.RequestChunkResync/45/decode-dimension-two",
    "protocol.client.RequestChunkResync/45/decode-valid",
    "protocol.client.RequestChunkResync/45/encode-dimension-two",
    "protocol.client.RequestChunkResync/45/encode-valid",
    "protocol.client.SelectHotbar/45/decode-slot-nine",
    "protocol.client.SelectHotbar/45/decode-valid",
    "protocol.client.SelectHotbar/45/encode-slot-nine",
    "protocol.client.SelectHotbar/45/encode-valid",
];

/// Reports whether one family belongs to the client control producer group.
fn is_client_control_family(family: &str) -> bool {
    matches!(
        family,
        PLAYER_INPUT_FAMILY
            | PLACE_BLOCK_FAMILY
            | REQUEST_CHUNK_RESYNC_FAMILY
            | SELECT_HOTBAR_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_client_control_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before that merge this test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let client_control: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_client_control_family(&case.family))
        .collect();
    let executed: Vec<&str> = client_control.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = CLIENT_CONTROL_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the client control selection does not carry the reviewed case set"
    );
    assert!(
        !client_control.is_empty(),
        "the client control selection executed zero cases"
    );
    for case in client_control {
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

/// The case identities the client ray group registers. They mirror the Go
/// producer's registration, so a case that only one side names is a mismatch
/// rather than a shared name. The merged manifest sorts case IDs, so the
/// comparison sorts this list too.
const CLIENT_RAYS_CASE_IDS: [&str; 25] = [
    "protocol.client.BoneMeal/45/decode-infinite-pitch",
    "protocol.client.BoneMeal/45/decode-nan-yaw",
    "protocol.client.BoneMeal/45/decode-valid",
    "protocol.client.BoneMeal/45/encode-nan-yaw",
    "protocol.client.BoneMeal/45/encode-valid",
    "protocol.client.CollectWater/45/decode-infinite-pitch",
    "protocol.client.CollectWater/45/decode-nan-yaw",
    "protocol.client.CollectWater/45/decode-valid",
    "protocol.client.CollectWater/45/encode-nan-yaw",
    "protocol.client.CollectWater/45/encode-valid",
    "protocol.client.OpenContainer/45/decode-infinite-pitch",
    "protocol.client.OpenContainer/45/decode-nan-yaw",
    "protocol.client.OpenContainer/45/decode-valid",
    "protocol.client.OpenContainer/45/encode-nan-yaw",
    "protocol.client.OpenContainer/45/encode-valid",
    "protocol.client.PlaceWater/45/decode-infinite-pitch",
    "protocol.client.PlaceWater/45/decode-nan-yaw",
    "protocol.client.PlaceWater/45/decode-valid",
    "protocol.client.PlaceWater/45/encode-nan-yaw",
    "protocol.client.PlaceWater/45/encode-valid",
    "protocol.client.TillSoil/45/decode-infinite-pitch",
    "protocol.client.TillSoil/45/decode-nan-yaw",
    "protocol.client.TillSoil/45/decode-valid",
    "protocol.client.TillSoil/45/encode-nan-yaw",
    "protocol.client.TillSoil/45/encode-valid",
];

/// Reports whether one family belongs to the client ray producer group.
fn is_client_rays_family(family: &str) -> bool {
    matches!(
        family,
        OPEN_CONTAINER_FAMILY
            | TILL_SOIL_FAMILY
            | BONE_MEAL_FAMILY
            | COLLECT_WATER_FAMILY
            | PLACE_WATER_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_client_rays_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before that merge this test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let client_rays: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_client_rays_family(&case.family))
        .collect();
    let executed: Vec<&str> = client_rays.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = CLIENT_RAYS_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the client ray selection does not carry the reviewed case set"
    );
    assert!(
        !client_rays.is_empty(),
        "the client ray selection executed zero cases"
    );
    for case in client_rays {
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
