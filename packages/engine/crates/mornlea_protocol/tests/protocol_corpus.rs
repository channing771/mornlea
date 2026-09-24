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
//! The fifth group is the six simple inventory and crafting command families
//! (`MoveInventoryStack`, `MoveCraftingStack`, `CloseContainer`,
//! `DropSelectedItem`, `EquipArmor` and `TakeCraftingOutput`), executed
//! through that same surface; the two move payloads carry the sequence and
//! two slot bytes, and the four sequence-only payloads carry the sequence
//! alone, because inventory contents, the moved count, the output recipe,
//! the drop position and the equipped slot stay server-owned. The sixth group
//! is the four container and view-addressed stack command families
//! (`MoveContainerStack`, `MoveStackPartial`, `QuickMoveStack` and
//! `DropStack`), executed through that same surface; every one of them carries
//! the 18-byte container reference, which the container view validates as a
//! real reference and the inventory and crafting views require to be the exact
//! all-zero sentinel. The moved amount, the transfer destination and the world
//! drop position stay server-owned, so no payload carries any of them. The
//! seventh group is the unsequenced chat command family (`ChatCommand`),
//! executed through that same surface; it is the one variable-length client
//! payload, so its gate is the domain `CommandText` rule, its decoder applies
//! the Go payload ceiling before any parse, and a declared length the payload
//! cannot complete reports `Truncated` the way the control message reader
//! does. No case in the group declares a session, a deadline, a FIFO entry or
//! a send: addressing, `/warp` handling and chat routing stay outside the
//! protocol contract. The eighth group is the two server-to-client world delta
//! families (`BlockChanges` and `ForgetChunks`), the first variable-count
//! batch families this suite executes: both carry a canonical uvarint record
//! count ahead of fixed-stride records, the decode side applies the count
//! bound before the record-length rule, an empty block-change batch stays
//! legal as the revision barrier while a zero-count forget batch is refused,
//! and the forget batch keeps its submitted wire order because uniqueness is
//! the only relation the wire publishes. The block-change gate's variant
//! split is a pinned ruling: an unregistered block is `InvalidEnum` and an
//! out-of-world Y is `InvalidRange`, which are the two categories the Go
//! producer records for the same bytes. The ninth group is the five inventory
//! and container publication families (`InventoryState`, `CraftingState`,
//! `FurnaceState`, `ChestState` and `ContainerClosed`), executed through the
//! same fallible surface; every slot value goes through the domain
//! `ItemStack` rule, the furnace and chest references keep their kind-first
//! gates, and the container-neutral closure family refuses the exact all-zero
//! record through its zero generation rather than treating it as absence.
//! `CorpusConsumer::Protocol` is registered here as `mornlea_protocol` so a
//! packet case can name it, while the framing cases stay with the separate
//! `corpus_frame` consumer the frame regression suite keeps using. The tenth
//! group is the three companion publication families (`CompanionSpawn`,
//! `CompanionStates` and `CompanionDespawn`), executed through the same
//! fallible surface; a companion is a member of the player's own party, so it
//! appears in the overworld alone and its pitch is bounded by the inclusive
//! half-turn limit, the batch applies the exact-remaining-length rule the Go
//! decoder compares against, and the absent zero companion identity stays
//! unconstructible on this surface because the identity is the checked domain
//! `CompanionId`. The eleventh group is the two item-drop publication
//! families (`ItemDropUpserts` and `ItemDropRemoves`), executed through the
//! same fallible surface; both order their records by the domain `DropId`
//! total order, where the raw wire dimension comes first and is never
//! narrowed, both apply the minimum-records batch rule so a padded payload
//! answers at the trailing-byte boundary, and the exact empty stack triple
//! stays wire-valid. The twelfth group is the three hostile-mob publication
//! families (`HostileSpawn`, `HostileState` and `HostileDespawn`), executed
//! through the same fallible surface; all three order their records by the
//! checked domain `HostileId`, all three apply the exact-remaining-length
//! batch rule so a padded payload answers at the truncation boundary, and the
//! spawn record carries the dimension the state record omits while the state
//! record carries the velocity the spawn record omits. The thirteenth group is
//! the three passive-mob publication families (`PassiveSpawn`,
//! `PassiveState` and `PassiveDespawn`), executed through the same fallible
//! surface; all three order their records by the checked domain `PassiveId`,
//! all three apply the exact-remaining-length batch rule so a padded payload
//! answers at the truncation boundary, neither record carries a kind byte
//! because a passive mob has no category to publish, and the 64-record wire
//! bound is admitted while the smaller live capacity the authority converges
//! on stays out of this packet layer.
//! The fourteenth group is the three projectile publication families
//! (`ProjectileSpawn`, `ProjectileState` and `ProjectileDespawn`), executed
//! through the same fallible surface; all three order their records by the
//! checked domain `ProjectileId`, all three apply the exact-remaining-length
//! batch rule so a padded payload answers at the truncation boundary, and all
//! three apply the family's fixed wire ceiling as a pre-parse size check so an
//! over-ceiling payload answers the capacity boundary the Go decoder's fixed
//! maximum publishes. The spawn record carries the kind before the dimension
//! and both playable dimensions are legal, because the kind-by-dimension
//! policy is an authority rule this wire does not enforce; the state record
//! carries the identity and the position alone, and the despawn record the
//! bare eight-byte identity.
//!
//! The fifteenth group is the single chat event publication family
//! (`ChatEvent`), executed through the same fallible surface. It is the one
//! family whose text slot is reused by kind: a companion speech event carries a
//! model-generated line bounded by the speech slot, and every other kind
//! restates the player's original command bounded by the planner instruction
//! limit, so the decoder reads the kind first and then decides which text to
//! read. The value gate's error variants publish the frozen categories the Go
//! producer records: a zero event identity or an invalid player identity is
//! `invalid-identity`, an unknown kind, the reserved reject reason 3 and a
//! failure reason outside 16..=20 are `invalid-enum`, and every text boundary
//! and illegal cross-field combination is `invalid-value`. The raw zero
//! companion identity is the wire-only absent form the two permitted rejection
//! branches carry; every other branch refuses it, and the domain union maps it
//! to variant-shaped absence rather than to an identity.
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
/// The six packet families the simple inventory and crafting command producer
/// group registers.
const MOVE_INVENTORY_STACK_FAMILY: &str = "protocol.client.MoveInventoryStack";
const MOVE_CRAFTING_STACK_FAMILY: &str = "protocol.client.MoveCraftingStack";
const CLOSE_CONTAINER_FAMILY: &str = "protocol.client.CloseContainer";
const DROP_SELECTED_ITEM_FAMILY: &str = "protocol.client.DropSelectedItem";
const EQUIP_ARMOR_FAMILY: &str = "protocol.client.EquipArmor";
const TAKE_CRAFTING_OUTPUT_FAMILY: &str = "protocol.client.TakeCraftingOutput";
/// The four packet families the container and view-addressed stack command
/// producer group registers.
const MOVE_CONTAINER_STACK_FAMILY: &str = "protocol.client.MoveContainerStack";
const MOVE_STACK_PARTIAL_FAMILY: &str = "protocol.client.MoveStackPartial";
const QUICK_MOVE_STACK_FAMILY: &str = "protocol.client.QuickMoveStack";
const DROP_STACK_FAMILY: &str = "protocol.client.DropStack";
/// The packet family the unsequenced chat command producer group registers.
const CHAT_COMMAND_FAMILY: &str = "protocol.client.ChatCommand";
/// The packet family the chat event producer group registers.
const CHAT_EVENT_FAMILY: &str = "protocol.server.ChatEvent";
/// The two packet families the world delta producer group registers.
const BLOCK_CHANGES_FAMILY: &str = "protocol.server.BlockChanges";
const FORGET_CHUNKS_FAMILY: &str = "protocol.server.ForgetChunks";
const CHUNK_SNAPSHOT_FAMILY: &str = "protocol.server.ChunkSnapshot";
/// The four packet families the player and private outcome producer group
/// registers.
const PLAYER_STATE_FAMILY: &str = "protocol.server.PlayerState";
const COMMAND_REJECTED_FAMILY: &str = "protocol.server.CommandRejected";
const PLACE_BLOCK_SUCCEEDED_FAMILY: &str = "protocol.server.PlaceBlockSucceeded";
const COMBAT_HIT_FAMILY: &str = "protocol.server.CombatHit";
/// The five packet families the inventory and container publication producer
/// group registers.
const INVENTORY_STATE_FAMILY: &str = "protocol.server.InventoryState";
const CRAFTING_STATE_FAMILY: &str = "protocol.server.CraftingState";
const FURNACE_STATE_FAMILY: &str = "protocol.server.FurnaceState";
const CHEST_STATE_FAMILY: &str = "protocol.server.ChestState";
const CONTAINER_CLOSED_FAMILY: &str = "protocol.server.ContainerClosed";
/// The three packet families the remote player producer group registers.
const REMOTE_PLAYER_SPAWN_FAMILY: &str = "protocol.server.RemotePlayerSpawn";
const REMOTE_PLAYER_DESPAWN_FAMILY: &str = "protocol.server.RemotePlayerDespawn";
const REMOTE_PLAYER_STATES_FAMILY: &str = "protocol.server.RemotePlayerStates";
/// The three packet families the companion producer group registers.
const COMPANION_SPAWN_FAMILY: &str = "protocol.server.CompanionSpawn";
const COMPANION_DESPAWN_FAMILY: &str = "protocol.server.CompanionDespawn";
const COMPANION_STATES_FAMILY: &str = "protocol.server.CompanionStates";
/// The two packet families the item drop producer group registers.
const ITEM_DROP_UPSERTS_FAMILY: &str = "protocol.server.ItemDropUpserts";
const ITEM_DROP_REMOVES_FAMILY: &str = "protocol.server.ItemDropRemoves";
/// The three packet families the hostile mob producer group registers.
const HOSTILE_SPAWN_FAMILY: &str = "protocol.server.HostileSpawn";
const HOSTILE_STATE_FAMILY: &str = "protocol.server.HostileState";
const HOSTILE_DESPAWN_FAMILY: &str = "protocol.server.HostileDespawn";
/// The three packet families the passive mob producer group registers.
const PASSIVE_SPAWN_FAMILY: &str = "protocol.server.PassiveSpawn";
const PASSIVE_STATE_FAMILY: &str = "protocol.server.PassiveState";
const PASSIVE_DESPAWN_FAMILY: &str = "protocol.server.PassiveDespawn";
/// The three packet families the projectile producer group registers.
const PROJECTILE_SPAWN_FAMILY: &str = "protocol.server.ProjectileSpawn";
const PROJECTILE_STATE_FAMILY: &str = "protocol.server.ProjectileState";
const PROJECTILE_DESPAWN_FAMILY: &str = "protocol.server.ProjectileDespawn";
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
///
/// `Integrity` is the compressed-stream boundary only the chunk-snapshot family
/// can publish: the envelope's length checks already passed, so the failure the
/// zstd layer reports is the frame's own content-checksum (or equivalent
/// frame-level) corruption. It is deliberately distinct from `Truncated`, which
/// stays the incomplete-bytes boundary the envelope's remaining-length check
/// and a declared length the frame cannot back both publish.
fn packet_rejection_category(err: mornlea_protocol::ProtocolError) -> String {
    let category = match err {
        mornlea_protocol::ProtocolError::NonCanonicalUvarint
        | mornlea_protocol::ProtocolError::InvalidUvarint => "invalid-varint",
        mornlea_protocol::ProtocolError::Truncated
        | mornlea_protocol::ProtocolError::EmptyFrame => "truncated",
        mornlea_protocol::ProtocolError::TrailingBytes => "trailing",
        mornlea_protocol::ProtocolError::InvalidEnum
        | mornlea_protocol::ProtocolError::UnknownPacket => "invalid-enum",
        mornlea_protocol::ProtocolError::Integrity => "integrity",
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

/// Reads one move encode case's canonical request fields: the sequence and the
/// two slot bytes.
fn client_move_request(case: &FrozenCase) -> (u64, u8, u8) {
    (
        unsigned_field(case, "sequence"),
        byte_field(case, "from"),
        byte_field(case, "to"),
    )
}

/// Renders the semantic fields one move command packet publishes.
///
/// The sequence renders as a decimal string so the full u64 range stays
/// lossless, and the two slots render as JSON numbers, which is the canonical
/// field encoding the Go producer records.
fn client_move_fields(sequence: u64, from: u8, to: u8) -> serde_json::Value {
    serde_json::json!({
        "sequence": sequence.to_string(),
        "from": from,
        "to": to
    })
}

/// Renders the semantic fields one sequence-only command packet publishes.
fn client_sequence_fields(sequence: u64) -> serde_json::Value {
    serde_json::json!({
        "sequence": sequence.to_string()
    })
}

/// Renders one 18-byte container reference as the nested JSON object both
/// directions publish.
///
/// The reference stays the raw wire value: the dimension and the two chunk
/// coordinates are plain JSON integers, so a negative chunk coordinate and a
/// dimension the authority would refuse both survive the round trip, and the
/// physical slot and generation publish as the byte and number they are.
fn client_container_fields(container: &mornlea_protocol::ContainerRef) -> serde_json::Value {
    serde_json::json!({
        "dimension": container.dimension,
        "chunk_x": container.chunk_x,
        "chunk_z": container.chunk_z,
        "kind": container.kind,
        "slot": container.slot,
        "generation": container.generation
    })
}

/// Reads one stack-view command encode case's container reference object.
fn client_container_request(case: &FrozenCase) -> mornlea_protocol::ContainerRef {
    let raw = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get("container")
        .and_then(|value| value.as_object())
        .unwrap_or_else(|| panic!("case {} names no container reference", case.id));
    let field = |name: &str| -> i64 {
        raw.get(name)
            .and_then(|value| value.as_i64())
            .unwrap_or_else(|| panic!("case {} reference names no {name}", case.id))
    };
    let byte = |name: &str| -> u8 {
        u8::try_from(field(name))
            .unwrap_or_else(|_| panic!("case {} reference field {name} exceeds u8", case.id))
    };
    let generation = u32::try_from(field("generation"))
        .unwrap_or_else(|_| panic!("case {} reference generation exceeds u32", case.id));
    mornlea_protocol::ContainerRef {
        dimension: i32::try_from(field("dimension"))
            .unwrap_or_else(|_| panic!("case {} reference dimension exceeds i32", case.id)),
        chunk_x: i32::try_from(field("chunk_x"))
            .unwrap_or_else(|_| panic!("case {} reference chunk_x exceeds i32", case.id)),
        chunk_z: i32::try_from(field("chunk_z"))
            .unwrap_or_else(|_| panic!("case {} reference chunk_z exceeds i32", case.id)),
        kind: byte("kind"),
        slot: byte("slot"),
        generation,
    }
}

/// Renders the semantic fields the four stack-view command families publish,
/// shared by the decode and encode arms so both publish the same canonical
/// field encoding.
fn client_stack_view_fields(record: &mornlea_protocol::MoveContainerStack) -> serde_json::Value {
    serde_json::json!({
        "sequence": record.sequence.to_string(),
        "container": client_container_fields(&record.container),
        "from": record.from,
        "to": record.to
    })
}

/// Renders the partial move's semantic fields, which add the view byte and the
/// single-item flag to the shared sequence, reference and index fields.
fn client_stack_partial_fields(record: &mornlea_protocol::MoveStackPartial) -> serde_json::Value {
    serde_json::json!({
        "sequence": record.sequence.to_string(),
        "container": client_container_fields(&record.container),
        "view": record.view,
        "from": record.from,
        "to": record.to,
        "single": record.single
    })
}

/// Renders the quick move's semantic fields, which carry one index.
fn client_quick_move_fields(record: &mornlea_protocol::QuickMoveStack) -> serde_json::Value {
    serde_json::json!({
        "sequence": record.sequence.to_string(),
        "container": client_container_fields(&record.container),
        "view": record.view,
        "from": record.from
    })
}

/// Renders the stack drop's semantic fields, which carry one slot.
fn client_drop_stack_fields(record: &mornlea_protocol::DropStack) -> serde_json::Value {
    serde_json::json!({
        "sequence": record.sequence.to_string(),
        "container": client_container_fields(&record.container),
        "view": record.view,
        "slot": record.slot
    })
}

/// Renders the semantic field one chat command publishes, shared by the decode
/// and encode arms so both publish the same canonical field encoding.
///
/// The text is published verbatim, including a leading mention prefix: the
/// codec performs no addressing, so the wire value and the published value are
/// the same string and no routing decision rides in the payload.
fn client_chat_fields(text: &str) -> serde_json::Value {
    serde_json::json!({
        "text": text
    })
}

/// Renders one block change's semantic fields, shared by the decode and encode
/// arms so both publish the same canonical field encoding.
///
/// The position is the absolute world coordinate the record carries and the
/// block is the plain registered number, so a negative coordinate and a block
/// above the last registered number both survive the round trip as the
/// integers they are.
fn block_change_fields(change: &mornlea_protocol::BlockChange) -> serde_json::Value {
    serde_json::json!({
        "x": change.x,
        "y": change.y,
        "z": change.z,
        "block": change.block
    })
}

/// Renders one chunk coordinate's semantic fields.
fn chunk_pos_fields(x: i32, z: i32) -> serde_json::Value {
    serde_json::json!({
        "x": x,
        "z": z
    })
}

/// Renders the semantic fields one block-change batch publishes, shared by the
/// decode and encode arms so both publish the same canonical field encoding.
///
/// The revisions render as decimal strings so the full u64 range stays
/// lossless, and the change list renders in the submitted order the
/// strictly-increasing rule admits, so an empty revision barrier publishes an
/// empty array rather than a null.
fn block_changes_fields(changes: &mornlea_protocol::BlockChanges) -> serde_json::Value {
    let records: Vec<serde_json::Value> = changes.changes.iter().map(block_change_fields).collect();
    serde_json::json!({
        "dimension": changes.dimension.get(),
        "chunk_x": changes.chunk_x,
        "chunk_z": changes.chunk_z,
        "base_revision": changes.base_revision.to_string(),
        "new_revision": changes.new_revision.to_string(),
        "changes": records
    })
}

/// Renders the semantic fields one forget batch publishes.
///
/// The chunk list renders in the submitted wire order, never sorted: the
/// uniqueness rule is the only relation the wire publishes, so a normalized
/// comparison that sorted the chunks would hide a reordering the authority
/// replayed.
fn forget_chunks_fields(forget: &mornlea_protocol::ForgetChunks) -> serde_json::Value {
    let records: Vec<serde_json::Value> = forget
        .chunks
        .iter()
        .map(|(x, z)| chunk_pos_fields(*x, *z))
        .collect();
    serde_json::json!({
        "dimension": forget.dimension.get(),
        "chunks": records
    })
}

/// Resolves one dimension field the checked domain value admits.
///
/// The record's dimension field is the checked domain `Dimension`, so an
/// unknown raw dimension cannot be constructed at all: the observable
/// rejection is published directly, which keeps the category identical to the
/// Go validator's.
fn dimension_field(
    case: &FrozenCase,
) -> Result<mornlea_domain::Dimension, mornlea_protocol::ProtocolError> {
    match u8::try_from(unsigned_field(case, "dimension"))
        .ok()
        .and_then(|value| mornlea_domain::Dimension::new(value).ok())
    {
        Some(dimension) => Ok(dimension),
        None => Err(mornlea_protocol::ProtocolError::InvalidEnum),
    }
}

/// Reads one block change's canonical request fields from the ordered record
/// array the encode case carries.
fn block_change_request(
    entry: &serde_json::Value,
    case: &FrozenCase,
) -> mornlea_protocol::BlockChange {
    let field = |name: &str| -> i64 {
        entry
            .get(name)
            .and_then(|value| value.as_i64())
            .unwrap_or_else(|| panic!("case {} change names no {name}", case.id))
    };
    mornlea_protocol::BlockChange {
        x: i32::try_from(field("x"))
            .unwrap_or_else(|_| panic!("case {} change x exceeds i32", case.id)),
        y: i32::try_from(field("y"))
            .unwrap_or_else(|_| panic!("case {} change y exceeds i32", case.id)),
        z: i32::try_from(field("z"))
            .unwrap_or_else(|_| panic!("case {} change z exceeds i32", case.id)),
        block: u16::try_from(field("block"))
            .unwrap_or_else(|_| panic!("case {} change block exceeds u16", case.id)),
    }
}

/// Reads one chunk coordinate pair's canonical request fields.
fn chunk_pos_request(entry: &serde_json::Value, case: &FrozenCase) -> (i32, i32) {
    let field = |name: &str| -> i32 {
        i32::try_from(
            entry
                .get(name)
                .and_then(|value| value.as_i64())
                .unwrap_or_else(|| panic!("case {} chunk names no {name}", case.id)),
        )
        .unwrap_or_else(|_| panic!("case {} chunk {name} exceeds i32", case.id))
    };
    (field("x"), field("z"))
}

/// Reads one record array an encode case carries, preserving its order.
fn record_array<'a>(case: &'a FrozenCase, name: &str) -> Vec<&'a serde_json::Value> {
    case.input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_array())
        .unwrap_or_else(|| panic!("case {} names no {name} array", case.id))
        .iter()
        .collect()
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

/// Reads one signed numeric field a section entry carries.
fn section_i32_field(entry: &serde_json::Value, name: &str) -> i32 {
    i32::try_from(
        entry
            .get(name)
            .and_then(|value| value.as_i64())
            .unwrap_or_else(|| panic!("section names no {name}")),
    )
    .unwrap_or_else(|_| panic!("section field {name} exceeds i32"))
}

/// Reads one unsigned numeric field a section entry carries.
fn section_u16_field(entry: &serde_json::Value, name: &str) -> u16 {
    u16::try_from(
        entry
            .get(name)
            .and_then(|value| value.as_i64())
            .unwrap_or_else(|| panic!("section names no {name}")),
    )
    .unwrap_or_else(|_| panic!("section field {name} exceeds u16"))
}

/// Reads one paletted section's ordered palette.
fn section_palette_field(entry: &serde_json::Value) -> Vec<u16> {
    entry
        .get("palette")
        .and_then(|value| value.as_array())
        .unwrap_or_else(|| panic!("paletted section names no palette"))
        .iter()
        .map(|id| {
            u16::try_from(
                id.as_i64()
                    .unwrap_or_else(|| panic!("palette names a non-integer")),
            )
            .unwrap_or_else(|_| panic!("palette entry exceeds u16"))
        })
        .collect()
}

/// Reads one packed section's exact word bits from their fixed-width hex text.
fn section_words_field(entry: &serde_json::Value, case: &FrozenCase) -> Vec<u64> {
    entry
        .get("words")
        .and_then(|value| value.as_array())
        .unwrap_or_else(|| panic!("case {} packed section names no words", case.id))
        .iter()
        .map(|word| {
            let text = word
                .as_str()
                .unwrap_or_else(|| panic!("case {} packed word is not text", case.id));
            u64::from_str_radix(text, 16)
                .unwrap_or_else(|_| panic!("case {} names invalid packed word {text}", case.id))
        })
        .collect()
}

/// Renders one section's semantic fields, the shape the Go producer publishes
/// for the same record.
///
/// The container kind implies the bits-per-slot, so the wire field is not
/// restated. The palette renders as plain numbers, which preserves its order,
/// and the packed words render as fixed-width lowercase hexadecimal, which
/// preserves their exact bits.
fn snapshot_section_fields(section: &mornlea_protocol::SectionData) -> serde_json::Value {
    let words: Vec<String> = section
        .packed
        .iter()
        .map(|word| format!("{word:016x}"))
        .collect();
    match section.storage {
        mornlea_protocol::SectionStorage::Single => serde_json::json!({
            "y": section.y,
            "kind": "single",
            "block": section.single
        }),
        mornlea_protocol::SectionStorage::Indexed => serde_json::json!({
            "y": section.y,
            "kind": if section.bits == 4 { "indexed4" } else { "indexed8" },
            "palette": section.palette,
            "words": words
        }),
        mornlea_protocol::SectionStorage::Direct => serde_json::json!({
            "y": section.y,
            "kind": "direct",
            "words": words
        }),
    }
}

/// Renders the semantic fields one chunk snapshot publishes.
///
/// The revision renders as a decimal string so the full u64 range stays
/// lossless, and the section list renders in the column order the wire pins.
fn snapshot_fields(snapshot: &mornlea_protocol::ChunkSnapshot) -> serde_json::Value {
    let sections: Vec<serde_json::Value> = snapshot
        .sections
        .iter()
        .map(snapshot_section_fields)
        .collect();
    serde_json::json!({
        "dimension": snapshot.dimension.get(),
        "chunk_x": snapshot.chunk_x,
        "chunk_z": snapshot.chunk_z,
        "revision": snapshot.revision.to_string(),
        "sections": sections
    })
}

/// Builds the record one snapshot encode case names from its typed fields, so a
/// mutated or invalid case is refused by the production validation rather than
/// by a constructor guard.
fn snapshot_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::ChunkSnapshot, mornlea_protocol::ProtocolError> {
    let dimension = dimension_field(case)?;
    let mut sections = Vec::new();
    for entry in record_array(case, "sections") {
        let kind = entry
            .get("kind")
            .and_then(|value| value.as_str())
            .unwrap_or_else(|| panic!("case {} section names no kind", case.id))
            .to_owned();
        let y = section_i32_field(entry, "y");
        let section = match kind.as_str() {
            "single" => mornlea_protocol::SectionData::single(y, section_u16_field(entry, "block")),
            "indexed4" => mornlea_protocol::SectionData::indexed(
                y,
                4,
                section_palette_field(entry),
                section_words_field(entry, case),
            ),
            "indexed8" => mornlea_protocol::SectionData::indexed(
                y,
                8,
                section_palette_field(entry),
                section_words_field(entry, case),
            ),
            "direct" => mornlea_protocol::SectionData::direct(y, section_words_field(entry, case)),
            other => panic!("case {} section names unknown kind {other}", case.id),
        };
        sections.push(section);
    }
    mornlea_protocol::ChunkSnapshot::new(
        dimension,
        i32_field(case, "chunk_x"),
        i32_field(case, "chunk_z"),
        unsigned_field(case, "revision"),
        sections,
    )
}

/// Executes one snapshot encode case through the owned context and records the
/// logical payload's digest.
///
/// The digest is taken over the logical bytes of the codec's own frame, never
/// over the compressed bytes: the Go encoder legitimately publishes a different
/// compressed block for the same logical payload, while the logical layer is
/// byte-identical, which is what makes the two implementations' digests agree.
fn snapshot_encode_outcome(case: &FrozenCase) -> serde_json::Value {
    let snapshot = match snapshot_request(case) {
        Ok(snapshot) => snapshot,
        Err(err) => return packet_error(err),
    };
    let mut codec = match mornlea_protocol::ProtocolCodec::new() {
        Ok(codec) => codec,
        Err(err) => return packet_error(err),
    };
    let mut dst = vec![0u8; mornlea_protocol::MAX_COMPRESSED_SNAPSHOT];
    match codec.encode_snapshot_into(&snapshot, &mut dst) {
        Ok(written) => {
            dst.truncate(written);
            let logical = match mornlea_protocol::ChunkSnapshot::decode_envelope(&dst)
                .and_then(|envelope| envelope.decompress())
            {
                Ok(logical) => logical,
                Err(err) => return packet_error(err),
            };
            serde_json::json!({
                "category": PACKET_OUTCOME_CATEGORY,
                "encoded_payload_digest": payload_digest(&logical),
                "fields": snapshot_fields(&snapshot),
                "kind": "ok"
            })
        }
        Err(err) => packet_error(err),
    }
}

/// Reads one unsigned 16-bit field one encode case carries.
fn u16_field(case: &FrozenCase, name: &str) -> u16 {
    u16::try_from(unsigned_field(case, name))
        .unwrap_or_else(|_| panic!("case {} field {name} exceeds u16", case.id))
}

/// Reads one signed 8-bit field one encode case carries, so the full range
/// stays lossless in both directions.
fn i8_field(case: &FrozenCase, name: &str) -> i8 {
    let value = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_i64())
        .unwrap_or_else(|| panic!("case {} names no {name}", case.id));
    i8::try_from(value).unwrap_or_else(|_| panic!("case {} field {name} exceeds i8", case.id))
}

/// Reads one three-component bit-string vector field one encode case
/// carries.
fn vec3_bits_field(case: &FrozenCase, name: &str) -> [f32; 3] {
    let entries = record_array(case, name);
    if entries.len() != 3 {
        panic!(
            "case {} field {name} carries {} components",
            case.id,
            entries.len()
        );
    }
    let mut vector = [0f32; 3];
    for (slot, entry) in vector.iter_mut().zip(entries) {
        let text = entry
            .as_str()
            .unwrap_or_else(|| panic!("case {} field {name} is not a bit string", case.id));
        let bits = u32::from_str_radix(text, 16)
            .unwrap_or_else(|_| panic!("case {} field {name} is not hexadecimal", case.id));
        *slot = f32::from_bits(bits);
    }
    vector
}

/// Reads one integer-triple object field one encode case carries.
fn triple_i32_field(case: &FrozenCase, name: &str) -> [i32; 3] {
    let object = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_object())
        .unwrap_or_else(|| panic!("case {} names no {name} object", case.id));
    let mut triple = [0i32; 3];
    for (index, axis) in ["x", "y", "z"].iter().enumerate() {
        triple[index] = i32::try_from(
            object
                .get(*axis)
                .and_then(|value| value.as_i64())
                .unwrap_or_else(|| panic!("case {} field {name} names no {axis}", case.id)),
        )
        .unwrap_or_else(|_| panic!("case {} field {name}.{axis} exceeds i32", case.id));
    }
    triple
}

/// Renders the semantic fields one player state publishes, shared by the
/// decode and encode arms so both publish the same canonical field encoding.
///
/// The ticks and the input sequence render as decimal strings so the full u64
/// range stays lossless, the position, velocity and look angles render as
/// their eight-digit hexadecimal bit strings so a negative zero survives the
/// round trip, and the temperature renders as the signed integer the wire
/// carries: the full i8 range is legal and never clipped.
fn player_state_fields(state: &mornlea_protocol::PlayerState) -> serde_json::Value {
    let vector = |values: &[f32; 3]| -> Vec<String> {
        values.iter().map(|value| float_bits_text(*value)).collect()
    };
    serde_json::json!({
        "server_tick": state.server_tick.to_string(),
        "last_input_sequence": state.last_input_sequence.to_string(),
        "dimension": state.dimension.get(),
        "position": vector(&state.position),
        "velocity": vector(&state.velocity),
        "yaw": float_bits_text(state.yaw),
        "pitch": float_bits_text(state.pitch),
        "on_ground": state.on_ground,
        "ready": state.ready,
        "reset": state.reset,
        "mining_active": state.mining_active,
        "mining_target": {
            "x": state.mining_target.x,
            "y": state.mining_target.y,
            "z": state.mining_target.z
        },
        "mining_progress_ticks": state.mining_progress_ticks,
        "mining_required_ticks": state.mining_required_ticks,
        "mining_harvestable": state.mining_harvestable,
        "health": state.health,
        "oxygen": state.oxygen,
        "hunger": state.hunger,
        "saturation_zero": state.saturation_zero,
        "day_phase_offset": state.day_phase_offset,
        "world_time_ticks": state.world_time_ticks.to_string(),
        "weather_kind": state.weather_kind,
        "season": state.season,
        "season_progress": state.season_progress,
        "temperature": state.temperature,
        "armor_points": state.armor_points
    })
}

/// Renders the semantic fields one command rejection publishes: the sequence
/// as a decimal string and the reason as its frozen wire number, which is the
/// value both implementations answer with rather than an internal cast.
fn command_rejected_fields(rejected: &mornlea_protocol::CommandRejected) -> serde_json::Value {
    serde_json::json!({
        "sequence": rejected.sequence.to_string(),
        "reason": rejected.reason
    })
}

/// Renders the semantic fields one placement acknowledgement publishes.
fn place_block_succeeded_fields(ack: &mornlea_protocol::PlaceBlockSucceeded) -> serde_json::Value {
    serde_json::json!({
        "sequence": ack.sequence.to_string()
    })
}

/// Renders the semantic fields one combat hit publishes: the server tick as a
/// decimal string and the damage and target kind as the plain integers the
/// wire carries.
fn combat_hit_fields(hit: &mornlea_protocol::CombatHit) -> serde_json::Value {
    serde_json::json!({
        "server_tick": hit.server_tick.to_string(),
        "damage": hit.damage,
        "target_kind": hit.target_kind
    })
}

/// Renders one item stack as the nested JSON object both directions publish.
///
/// All three fields are published, because the empty slot is the zero triple
/// and a missing field would not prove the record carried exactly that.
fn inventory_stack_fields(stack: mornlea_protocol::ItemStack) -> serde_json::Value {
    serde_json::json!({
        "item": stack.item(),
        "count": stack.count(),
        "durability": stack.durability()
    })
}

/// Renders one ordered stack array, preserving wire order.
fn inventory_stack_array(stacks: &[mornlea_protocol::ItemStack]) -> Vec<serde_json::Value> {
    stacks
        .iter()
        .map(|stack| inventory_stack_fields(*stack))
        .collect()
}

/// Reads one stack object an encode case carries through the domain rule.
///
/// The corpus never names an invalid stack in an encode request, so a value
/// the domain rule refuses is a case-authoring failure rather than an outcome
/// this consumer publishes.
fn inventory_stack_request(
    entry: &serde_json::Value,
    case: &FrozenCase,
) -> mornlea_protocol::ItemStack {
    let field = |name: &str| -> i64 {
        entry
            .get(name)
            .and_then(|value| value.as_i64())
            .unwrap_or_else(|| panic!("case {} stack names no {name}", case.id))
    };
    mornlea_protocol::ItemStack::try_new(
        u16::try_from(field("item"))
            .unwrap_or_else(|_| panic!("case {} stack item exceeds u16", case.id)),
        u8::try_from(field("count"))
            .unwrap_or_else(|_| panic!("case {} stack count exceeds u8", case.id)),
        u16::try_from(field("durability"))
            .unwrap_or_else(|_| panic!("case {} stack durability exceeds u16", case.id)),
    )
    .unwrap_or_else(|_| panic!("case {} names an invalid item stack", case.id))
}

/// Reads one ordered stack array an encode case carries.
fn inventory_stack_array_request(
    case: &FrozenCase,
    name: &str,
    want: usize,
) -> Vec<mornlea_protocol::ItemStack> {
    let stacks: Vec<mornlea_protocol::ItemStack> = record_array(case, name)
        .into_iter()
        .map(|entry| inventory_stack_request(entry, case))
        .collect();
    assert_eq!(
        stacks.len(),
        want,
        "case {} field {name} carries {} stacks, want {want}",
        case.id,
        stacks.len()
    );
    stacks
}

/// Reads one container reference object an encode case carries, naming the
/// field the family publishes it under.
fn inventory_container_request(case: &FrozenCase, name: &str) -> mornlea_protocol::ContainerRef {
    let raw = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_object())
        .unwrap_or_else(|| panic!("case {} names no {name} reference", case.id));
    let field = |key: &str| -> i64 {
        raw.get(key)
            .and_then(|value| value.as_i64())
            .unwrap_or_else(|| panic!("case {} reference names no {key}", case.id))
    };
    let byte = |key: &str| -> u8 {
        u8::try_from(field(key))
            .unwrap_or_else(|_| panic!("case {} reference field {key} exceeds u8", case.id))
    };
    let generation = u32::try_from(field("generation"))
        .unwrap_or_else(|_| panic!("case {} reference generation exceeds u32", case.id));
    mornlea_protocol::ContainerRef {
        dimension: i32::try_from(field("dimension"))
            .unwrap_or_else(|_| panic!("case {} reference dimension exceeds i32", case.id)),
        chunk_x: i32::try_from(field("chunk_x"))
            .unwrap_or_else(|_| panic!("case {} reference chunk_x exceeds i32", case.id)),
        chunk_z: i32::try_from(field("chunk_z"))
            .unwrap_or_else(|_| panic!("case {} reference chunk_z exceeds i32", case.id)),
        kind: byte("kind"),
        slot: byte("slot"),
        generation,
    }
}

/// Renders the semantic fields one inventory state publishes: the selected
/// index and the two ordered slot arrays.
fn inventory_state_fields(state: &mornlea_protocol::InventoryState) -> serde_json::Value {
    serde_json::json!({
        "selected": state.selected,
        "hotbar": inventory_stack_array(&state.hotbar),
        "backpack": inventory_stack_array(&state.backpack)
    })
}

/// Renders the semantic fields one crafting state publishes.
fn crafting_state_fields(state: &mornlea_protocol::CraftingState) -> serde_json::Value {
    serde_json::json!({
        "size": state.size,
        "slots": inventory_stack_array(&state.slots),
        "output": inventory_stack_fields(state.output)
    })
}

/// Renders the semantic fields one furnace state publishes.
fn furnace_state_fields(state: &mornlea_protocol::FurnaceState) -> serde_json::Value {
    serde_json::json!({
        "furnace": client_container_fields(&state.furnace),
        "input": inventory_stack_fields(state.input),
        "fuel": inventory_stack_fields(state.fuel),
        "output": inventory_stack_fields(state.output),
        "progress_ticks": state.progress_ticks,
        "burn_ticks": state.burn_ticks
    })
}

/// Renders the semantic fields one chest state publishes.
fn chest_state_fields(state: &mornlea_protocol::ChestState) -> serde_json::Value {
    serde_json::json!({
        "chest": client_container_fields(&state.chest),
        "items": inventory_stack_array(&state.items)
    })
}

/// Renders the semantic fields one container closure publishes.
fn container_closed_fields(closed: &mornlea_protocol::ContainerClosed) -> serde_json::Value {
    serde_json::json!({
        "container": client_container_fields(&closed.container)
    })
}

/// Renders one 16-byte identity as the 32-lowercase-hexadecimal text both
/// directions publish.
///
/// The wire form is published as text rather than as a nested object, so a
/// zero or non-UUIDv4 byte sequence stays observable instead of being
/// pre-validated into a number the wire never carries.
fn remote_player_id_text(player_id: &mornlea_protocol::PlayerId) -> String {
    hex_lower(&player_id.bytes())
}

/// Renders one companion identity as its 32-lowercase-hexadecimal text.
///
/// The wire form is published as text rather than as a nested object, so a
/// zero or non-UUIDv4 byte sequence stays observable instead of being
/// pre-validated into a number the wire never carries.
fn companion_id_text(companion_id: &mornlea_protocol::CompanionId) -> String {
    hex_lower(&companion_id.bytes())
}

/// Renders one remote player spawn's semantic fields.
///
/// The tick is a decimal string so the full `u64` range stays lossless, the
/// dimension is the plain wire integer, the pose publishes as the eight-digit
/// hexadecimal bit strings that keep a negative zero distinct, and the name
/// is verbatim because the codec performs no trimming.
fn remote_player_spawn_fields(spawn: &mornlea_protocol::RemotePlayerSpawn) -> serde_json::Value {
    serde_json::json!({
        "player_id": remote_player_id_text(&spawn.player_id),
        "display_name": spawn.display_name,
        "server_tick": spawn.server_tick.to_string(),
        "dimension": spawn.dimension.get(),
        "position": [
            float_bits_text(spawn.position[0]),
            float_bits_text(spawn.position[1]),
            float_bits_text(spawn.position[2])
        ],
        "yaw": float_bits_text(spawn.yaw),
        "pitch": float_bits_text(spawn.pitch)
    })
}

/// Renders one remote player state record's semantic fields.
fn remote_player_state_fields(record: &mornlea_protocol::RemotePlayerState) -> serde_json::Value {
    serde_json::json!({
        "player_id": remote_player_id_text(&record.player_id),
        "dimension": record.dimension.get(),
        "position": [
            float_bits_text(record.position[0]),
            float_bits_text(record.position[1]),
            float_bits_text(record.position[2])
        ],
        "yaw": float_bits_text(record.yaw),
        "pitch": float_bits_text(record.pitch),
        "reset": record.reset
    })
}

/// Renders one remote player despawn's semantic fields.
fn remote_player_despawn_fields(
    despawn: &mornlea_protocol::RemotePlayerDespawn,
) -> serde_json::Value {
    serde_json::json!({
        "player_id": remote_player_id_text(&despawn.player)
    })
}

/// Renders one remote player state batch's semantic fields.
///
/// The records publish in wire order, never sorted, so a batch the authority
/// ordered is observed in the order it carried.
fn remote_player_states_fields(states: &mornlea_protocol::RemotePlayerStates) -> serde_json::Value {
    let players: Vec<serde_json::Value> = states
        .players
        .iter()
        .map(remote_player_state_fields)
        .collect();
    serde_json::json!({
        "server_tick": states.server_tick.to_string(),
        "players": players
    })
}

/// Reads one identity field an encode case carries as its 32-hex text.
fn remote_player_id_request(case: &FrozenCase, name: &str) -> mornlea_protocol::PlayerId {
    let text = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_str())
        .unwrap_or_else(|| panic!("case {} names no {name}", case.id));
    let bytes: [u8; 16] = payload_bytes_from_text(case, text)
        .try_into()
        .unwrap_or_else(|_| panic!("case {} field {name} is not 16 bytes", case.id));
    mornlea_protocol::PlayerId::try_from_bytes(bytes)
        .unwrap_or_else(|_| panic!("case {} field {name} is not a player identity", case.id))
}

/// Reads one f32 bit-string array an encode case carries.
fn float_bits_array_request(case: &FrozenCase, name: &str) -> [f32; 3] {
    let entries = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_array())
        .unwrap_or_else(|| panic!("case {} names no {name}", case.id));
    let mut values = [0f32; 3];
    for (index, slot) in values.iter_mut().enumerate() {
        let text = entries
            .get(index)
            .and_then(|value| value.as_str())
            .unwrap_or_else(|| panic!("case {} {name}[{index}] is not text", case.id));
        let bits = u32::from_str_radix(text, 16)
            .unwrap_or_else(|_| panic!("case {} {name}[{index}] is not hexadecimal bits", case.id));
        *slot = f32::from_bits(bits);
    }
    values
}

/// Builds the remote player spawn one encode case names from its typed
/// fields.
///
/// The record is built through its public fields, so a mutated or invalid
/// case is refused by the production validation rather than by a constructor
/// guard.
fn remote_player_spawn_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::RemotePlayerSpawn, mornlea_protocol::ProtocolError> {
    let player_id = remote_player_id_request(case, "player_id");
    let display_name = display_name_field(case);
    let server_tick = unsigned_field(case, "server_tick");
    let dimension = remote_player_dimension_request(case, "dimension")?;
    let position = float_bits_array_request(case, "position");
    let yaw = float_bits_field(case, "yaw");
    let pitch = float_bits_field(case, "pitch");
    Ok(mornlea_protocol::RemotePlayerSpawn {
        player_id,
        display_name,
        server_tick,
        dimension,
        position,
        yaw,
        pitch,
    })
}

/// Builds the remote player despawn one encode case names from its typed
/// fields.
fn remote_player_despawn_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::RemotePlayerDespawn, mornlea_protocol::ProtocolError> {
    Ok(mornlea_protocol::RemotePlayerDespawn::new(
        remote_player_id_request(case, "player_id"),
    ))
}

/// Reads one dimension field an encode case carries, mapping an unknown value
/// to the enum boundary the decoder publishes for the same bytes.
fn remote_player_dimension_request(
    case: &FrozenCase,
    name: &str,
) -> Result<mornlea_domain::Dimension, mornlea_protocol::ProtocolError> {
    let dimension = i32_field(case, name);
    let narrowed = u8::try_from(dimension).unwrap_or(u8::MAX);
    mornlea_domain::Dimension::new(narrowed)
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidEnum)
}

/// Builds one remote player state record its JSON object names.
fn remote_player_state_request(
    case: &FrozenCase,
    name: &str,
    index: usize,
) -> Result<mornlea_protocol::RemotePlayerState, mornlea_protocol::ProtocolError> {
    let entries = record_array(case, name);
    let entry = entries
        .get(index)
        .unwrap_or_else(|| panic!("case {} {name}[{index}] is missing", case.id));
    let player_id = {
        let text = entry
            .get("player_id")
            .and_then(|value| value.as_str())
            .unwrap_or_else(|| panic!("case {} {name}[{index}] names no player_id", case.id));
        let bytes: [u8; 16] = payload_bytes_from_text(case, text)
            .try_into()
            .unwrap_or_else(|_| {
                panic!("case {} {name}[{index}] player_id is not 16 bytes", case.id)
            });
        mornlea_protocol::PlayerId::try_from_bytes(bytes)
            .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?
    };
    let dimension = match entry.get("dimension").and_then(|value| value.as_i64()) {
        Some(dimension) => {
            let narrowed = u8::try_from(dimension).unwrap_or(u8::MAX);
            mornlea_domain::Dimension::new(narrowed)
                .map_err(|_| mornlea_protocol::ProtocolError::InvalidEnum)?
        }
        None => panic!("case {} {name}[{index}] names no dimension", case.id),
    };
    let position = {
        let values = entry
            .get("position")
            .and_then(|value| value.as_array())
            .unwrap_or_else(|| panic!("case {} {name}[{index}] names no position", case.id));
        let mut parsed = [0f32; 3];
        for (index, slot) in parsed.iter_mut().enumerate() {
            let text = values
                .get(index)
                .and_then(|value| value.as_str())
                .unwrap_or_else(|| {
                    panic!(
                        "case {} {name}[{index}] position[{index}] is not text",
                        case.id
                    )
                });
            let bits = u32::from_str_radix(text, 16).unwrap_or_else(|_| {
                panic!(
                    "case {} {name}[{index}] position[{index}] is not hexadecimal bits",
                    case.id
                )
            });
            *slot = f32::from_bits(bits);
        }
        parsed
    };
    let angle = |field: &str| -> f32 {
        let text = entry
            .get(field)
            .and_then(|value| value.as_str())
            .unwrap_or_else(|| panic!("case {} {name}[{index}] names no {field}", case.id));
        let bits = u32::from_str_radix(text, 16).unwrap_or_else(|_| {
            panic!(
                "case {} {name}[{index}] {field} is not hexadecimal bits",
                case.id
            )
        });
        f32::from_bits(bits)
    };
    let reset = entry
        .get("reset")
        .and_then(|value| value.as_bool())
        .unwrap_or_else(|| panic!("case {} {name}[{index}] names no reset", case.id));
    Ok(mornlea_protocol::RemotePlayerState {
        player_id,
        dimension,
        position,
        yaw: angle("yaw"),
        pitch: angle("pitch"),
        reset,
    })
}

/// Builds the remote player state batch one encode case names from its typed
/// fields.
fn remote_player_states_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::RemotePlayerStates, mornlea_protocol::ProtocolError> {
    let server_tick = unsigned_field(case, "server_tick");
    let records = record_array(case, "players");
    let mut players = Vec::with_capacity(records.len());
    for index in 0..records.len() {
        players.push(remote_player_state_request(case, "players", index)?);
    }
    Ok(mornlea_protocol::RemotePlayerStates {
        server_tick,
        players,
    })
}

/// Renders one companion spawn's semantic fields.
///
/// The tick is a decimal string so the full `u64` range stays lossless, the
/// dimension is the plain wire integer, the pose publishes as the eight-digit
/// hexadecimal bit strings that keep a negative zero distinct, and the name is
/// verbatim because the codec performs no trimming.
fn companion_spawn_fields(spawn: &mornlea_protocol::CompanionSpawn) -> serde_json::Value {
    serde_json::json!({
        "companion_id": companion_id_text(&spawn.companion_id),
        "name": spawn.name,
        "tick": spawn.tick.to_string(),
        "dimension": spawn.dimension.get(),
        "position": [
            float_bits_text(spawn.position[0]),
            float_bits_text(spawn.position[1]),
            float_bits_text(spawn.position[2])
        ],
        "yaw": float_bits_text(spawn.yaw),
        "pitch": float_bits_text(spawn.pitch)
    })
}

/// Renders one companion state record's semantic fields.
fn companion_state_fields(record: &mornlea_protocol::CompanionState) -> serde_json::Value {
    serde_json::json!({
        "companion_id": companion_id_text(&record.companion_id),
        "dimension": record.dimension.get(),
        "position": [
            float_bits_text(record.position[0]),
            float_bits_text(record.position[1]),
            float_bits_text(record.position[2])
        ],
        "yaw": float_bits_text(record.yaw),
        "pitch": float_bits_text(record.pitch),
        "reset": record.reset
    })
}

/// Renders one companion despawn's semantic fields.
fn companion_despawn_fields(despawn: &mornlea_protocol::CompanionDespawn) -> serde_json::Value {
    serde_json::json!({
        "companion_id": companion_id_text(&despawn.companion)
    })
}

/// Renders one companion state batch's semantic fields.
///
/// The records publish in wire order, never sorted, so a batch the authority
/// ordered is observed in the order it carried.
fn companion_states_fields(states: &mornlea_protocol::CompanionStates) -> serde_json::Value {
    let records: Vec<serde_json::Value> =
        states.states.iter().map(companion_state_fields).collect();
    serde_json::json!({
        "tick": states.tick.to_string(),
        "states": records
    })
}

/// Reads one identity field an encode case carries as its 32-hex text.
fn companion_id_request(case: &FrozenCase, name: &str) -> mornlea_protocol::CompanionId {
    let text = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .and_then(|value| value.as_str())
        .unwrap_or_else(|| panic!("case {} names no {name}", case.id));
    let bytes: [u8; 16] = payload_bytes_from_text(case, text)
        .try_into()
        .unwrap_or_else(|_| panic!("case {} field {name} is not 16 bytes", case.id));
    mornlea_protocol::CompanionId::try_from_bytes(bytes)
        .unwrap_or_else(|_| panic!("case {} field {name} is not a companion identity", case.id))
}

/// Reads one dimension field an encode case carries, mapping an unknown value
/// to the enum boundary the decoder publishes for the same bytes.
fn companion_dimension_request(
    case: &FrozenCase,
    name: &str,
) -> Result<mornlea_domain::Dimension, mornlea_protocol::ProtocolError> {
    let dimension = i32_field(case, name);
    let narrowed = u8::try_from(dimension).unwrap_or(u8::MAX);
    mornlea_domain::Dimension::new(narrowed)
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidEnum)
}

/// Reads one companion name field an encode case carries.
fn companion_name_field(case: &FrozenCase) -> String {
    case.input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get("name")
        .and_then(|value| value.as_str().map(str::to_owned))
        .unwrap_or_else(|| panic!("case {} names no name", case.id))
}

/// Builds the companion spawn one encode case names from its typed fields.
///
/// The record is built through its public fields, so a mutated or invalid
/// case is refused by the production validation rather than by a constructor
/// guard.
fn companion_spawn_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::CompanionSpawn, mornlea_protocol::ProtocolError> {
    let companion_id = companion_id_request(case, "companion_id");
    let name = companion_name_field(case);
    let tick = unsigned_field(case, "tick");
    let dimension = companion_dimension_request(case, "dimension")?;
    let position = float_bits_array_request(case, "position");
    let yaw = float_bits_field(case, "yaw");
    let pitch = float_bits_field(case, "pitch");
    Ok(mornlea_protocol::CompanionSpawn {
        companion_id,
        name,
        tick,
        dimension,
        position,
        yaw,
        pitch,
    })
}

/// Builds the companion despawn one encode case names from its typed fields.
fn companion_despawn_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::CompanionDespawn, mornlea_protocol::ProtocolError> {
    Ok(mornlea_protocol::CompanionDespawn::new(
        companion_id_request(case, "companion_id"),
    ))
}

/// Builds one companion state record its JSON object names.
///
/// The identity is read as the checked domain value, so a zero or non-UUIDv4
/// identity an encode case names is refused at the identity boundary rather
/// than reaching the record gate.
fn companion_state_request(
    case: &FrozenCase,
    name: &str,
    index: usize,
) -> Result<mornlea_protocol::CompanionState, mornlea_protocol::ProtocolError> {
    let entry = record_array(case, name)
        .get(index)
        .copied()
        .unwrap_or_else(|| panic!("case {} {name}[{index}] is missing", case.id));
    let companion_id = {
        let text = entry
            .get("companion_id")
            .and_then(|value| value.as_str())
            .unwrap_or_else(|| panic!("case {} {name}[{index}] names no companion_id", case.id));
        let bytes: [u8; 16] = payload_bytes_from_text(case, text)
            .try_into()
            .unwrap_or_else(|_| {
                panic!(
                    "case {} {name}[{index}] companion_id is not 16 bytes",
                    case.id
                )
            });
        mornlea_protocol::CompanionId::try_from_bytes(bytes)
            .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?
    };
    let dimension = match entry.get("dimension").and_then(|value| value.as_i64()) {
        Some(dimension) => {
            let narrowed = u8::try_from(dimension).unwrap_or(u8::MAX);
            mornlea_domain::Dimension::new(narrowed)
                .map_err(|_| mornlea_protocol::ProtocolError::InvalidEnum)?
        }
        None => panic!("case {} {name}[{index}] names no dimension", case.id),
    };
    let position = {
        let values = entry
            .get("position")
            .and_then(|value| value.as_array())
            .unwrap_or_else(|| panic!("case {} {name}[{index}] names no position", case.id));
        let mut parsed = [0f32; 3];
        for (slot_index, slot) in parsed.iter_mut().enumerate() {
            let text = values
                .get(slot_index)
                .and_then(|value| value.as_str())
                .unwrap_or_else(|| {
                    panic!(
                        "case {} {name}[{index}] position[{slot_index}] is not text",
                        case.id
                    )
                });
            let bits = u32::from_str_radix(text, 16).unwrap_or_else(|_| {
                panic!(
                    "case {} {name}[{index}] position[{slot_index}] is not hexadecimal bits",
                    case.id
                )
            });
            *slot = f32::from_bits(bits);
        }
        parsed
    };
    let angle = |field: &str| -> f32 {
        let text = entry
            .get(field)
            .and_then(|value| value.as_str())
            .unwrap_or_else(|| panic!("case {} {name}[{index}] names no {field}", case.id));
        let bits = u32::from_str_radix(text, 16).unwrap_or_else(|_| {
            panic!(
                "case {} {name}[{index}] {field} is not hexadecimal bits",
                case.id
            )
        });
        f32::from_bits(bits)
    };
    let reset = entry
        .get("reset")
        .and_then(|value| value.as_bool())
        .unwrap_or_else(|| panic!("case {} {name}[{index}] names no reset", case.id));
    Ok(mornlea_protocol::CompanionState {
        companion_id,
        dimension,
        position,
        yaw: angle("yaw"),
        pitch: angle("pitch"),
        reset,
    })
}

/// Builds the companion state batch one encode case names from its typed
/// fields.
fn companion_states_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::CompanionStates, mornlea_protocol::ProtocolError> {
    let tick = unsigned_field(case, "tick");
    let records = record_array(case, "states");
    let mut states = Vec::with_capacity(records.len());
    for index in 0..records.len() {
        states.push(companion_state_request(case, "states", index)?);
    }
    Ok(mornlea_protocol::CompanionStates { tick, states })
}

/// Renders one drop identity as its ordered five-field object.
///
/// The dimension publishes as the plain wire integer, verbatim and never
/// narrowed: the Go validity rule checks only the slot range and the
/// generation, so a −1 or 256 dimension is a publishable identity both
/// directions carry unchanged.
fn drop_id_fields(id: mornlea_protocol::DropId) -> serde_json::Value {
    serde_json::json!({
        "dimension": id.dimension(),
        "chunk_x": id.chunk().x(),
        "chunk_z": id.chunk().z(),
        "slot": id.slot(),
        "generation": id.generation()
    })
}

/// Renders one drop record's stack as its ordered triple, where the exact
/// empty triple `(0,0,0)` is wire-valid.
fn item_drop_stack_fields(item: u16, count: u8, durability: u16) -> serde_json::Value {
    serde_json::json!({
        "item": item,
        "count": count,
        "durability": durability
    })
}

/// Renders one drop record's semantic fields.
fn item_drop_fields(drop: &mornlea_protocol::ItemDrop) -> serde_json::Value {
    serde_json::json!({
        "id": drop_id_fields(drop.id),
        "block_index": drop.block_index,
        "stack": item_drop_stack_fields(drop.item, drop.count, drop.durability)
    })
}

/// Renders one item drop upsert batch's semantic fields.
///
/// The records publish in wire order, never sorted, so a batch the authority
/// ordered is observed in the order it carried.
fn item_drop_upserts_fields(upserts: &mornlea_protocol::ItemDropUpserts) -> serde_json::Value {
    let drops: Vec<serde_json::Value> = upserts.drops.iter().map(item_drop_fields).collect();
    serde_json::json!({
        "server_tick": upserts.server_tick.to_string(),
        "drops": drops
    })
}

/// Renders one item drop remove batch's semantic fields.
fn item_drop_removes_fields(removes: &mornlea_protocol::ItemDropRemoves) -> serde_json::Value {
    let ids: Vec<serde_json::Value> = removes.ids.iter().map(|id| drop_id_fields(*id)).collect();
    serde_json::json!({
        "server_tick": removes.server_tick.to_string(),
        "ids": ids
    })
}

/// Reads one drop identity object an encode case carries.
fn drop_id_request(entry: &serde_json::Value, case: &FrozenCase) -> mornlea_protocol::DropId {
    let field = |name: &str| -> i64 {
        entry
            .get(name)
            .and_then(|value| value.as_i64())
            .unwrap_or_else(|| panic!("case {} drop identity names no {name}", case.id))
    };
    let i32_field = |name: &str| -> i32 {
        i32::try_from(field(name))
            .unwrap_or_else(|_| panic!("case {} drop identity field {name} exceeds i32", case.id))
    };
    let u32_field = |name: &str| -> u32 {
        u32::try_from(field(name))
            .unwrap_or_else(|_| panic!("case {} drop identity field {name} exceeds u32", case.id))
    };
    mornlea_protocol::DropId::try_new(
        i32_field("dimension"),
        mornlea_domain::ChunkPos::new(i32_field("chunk_x"), i32_field("chunk_z")),
        u8::try_from(field("slot"))
            .unwrap_or_else(|_| panic!("case {} drop identity slot exceeds u8", case.id)),
        u32_field("generation"),
    )
    .expect("the reviewed drop identity satisfies the domain rule")
}

/// Reads one nested object field an encode case carries.
fn nested_object_field<'a>(
    entry: &'a serde_json::Value,
    name: &str,
    case: &FrozenCase,
) -> &'a serde_json::Value {
    entry
        .get(name)
        .unwrap_or_else(|| panic!("case {} drop record names no {name}", case.id))
}

/// Reads one numeric field of one nested object.
fn nested_int_field(entry: &serde_json::Value, name: &str, case: &FrozenCase) -> i64 {
    entry
        .get(name)
        .and_then(|value| value.as_i64())
        .unwrap_or_else(|| panic!("case {} nested object names no {name}", case.id))
}

/// Builds one drop record its JSON object names.
fn item_drop_record_request(
    entry: &serde_json::Value,
    case: &FrozenCase,
) -> mornlea_protocol::ItemDrop {
    let id = drop_id_request(nested_object_field(entry, "id", case), case);
    let block_index = u32::try_from(nested_int_field(entry, "block_index", case))
        .unwrap_or_else(|_| panic!("case {} drop record block_index exceeds u32", case.id));
    let stack = nested_object_field(entry, "stack", case);
    mornlea_protocol::ItemDrop {
        id,
        block_index,
        item: u16::try_from(nested_int_field(stack, "item", case))
            .unwrap_or_else(|_| panic!("case {} drop stack item exceeds u16", case.id)),
        count: u8::try_from(nested_int_field(stack, "count", case))
            .unwrap_or_else(|_| panic!("case {} drop stack count exceeds u8", case.id)),
        durability: u16::try_from(nested_int_field(stack, "durability", case))
            .unwrap_or_else(|_| panic!("case {} drop stack durability exceeds u16", case.id)),
    }
}

/// Builds the item drop upsert batch one encode case names from its typed
/// fields.
///
/// The record is built through its public fields, so a mutated or invalid
/// case is refused by the production validation rather than by a constructor
/// guard.
fn item_drop_upserts_request(case: &FrozenCase) -> mornlea_protocol::ItemDropUpserts {
    let server_tick = unsigned_field(case, "server_tick");
    let drops: Vec<mornlea_protocol::ItemDrop> = record_array(case, "drops")
        .into_iter()
        .map(|entry| item_drop_record_request(entry, case))
        .collect();
    mornlea_protocol::ItemDropUpserts { server_tick, drops }
}

/// Builds the item drop remove batch one encode case names from its typed
/// fields.
fn item_drop_removes_request(case: &FrozenCase) -> mornlea_protocol::ItemDropRemoves {
    let server_tick = unsigned_field(case, "server_tick");
    let ids: Vec<mornlea_protocol::DropId> = record_array(case, "ids")
        .into_iter()
        .map(|entry| drop_id_request(entry, case))
        .collect();
    mornlea_protocol::ItemDropRemoves { server_tick, ids }
}

/// Renders one hostile spawn record's semantic fields.
///
/// The identity publishes as a decimal string so the full `u64` range stays
/// lossless, the dimension as the plain wire integer, the pose as bit strings so
/// a negative zero stays distinct, and the health and kind as JSON numbers.
fn hostile_spawn_record_fields(record: &mornlea_protocol::HostileSpawnRecord) -> serde_json::Value {
    serde_json::json!({
        "id": record.id.get().to_string(),
        "dimension": i32::from(record.dimension.get()),
        "position": [
            float_bits_text(record.position[0]),
            float_bits_text(record.position[1]),
            float_bits_text(record.position[2])
        ],
        "yaw": float_bits_text(record.yaw),
        "health": record.health,
        "kind": record.kind
    })
}

/// Renders one hostile spawn batch's semantic fields.
///
/// The records publish in wire order, never sorted, so a batch the authority
/// ordered is observed in the order it carried.
fn hostile_spawn_fields(spawn: &mornlea_protocol::HostileSpawn) -> serde_json::Value {
    let records: Vec<serde_json::Value> = spawn
        .spawns
        .iter()
        .map(hostile_spawn_record_fields)
        .collect();
    serde_json::json!({
        "server_tick": spawn.server_tick.to_string(),
        "spawns": records
    })
}

/// Renders one hostile state record's semantic fields, which carry the velocity
/// the spawn record never carries and no dimension at all.
fn hostile_state_record_fields(record: &mornlea_protocol::HostileStateRecord) -> serde_json::Value {
    serde_json::json!({
        "id": record.id.get().to_string(),
        "position": [
            float_bits_text(record.position[0]),
            float_bits_text(record.position[1]),
            float_bits_text(record.position[2])
        ],
        "velocity": [
            float_bits_text(record.velocity[0]),
            float_bits_text(record.velocity[1]),
            float_bits_text(record.velocity[2])
        ],
        "yaw": float_bits_text(record.yaw),
        "health": record.health,
        "kind": record.kind
    })
}

/// Renders one hostile state batch's semantic fields.
fn hostile_state_fields(state: &mornlea_protocol::HostileState) -> serde_json::Value {
    let records: Vec<serde_json::Value> = state
        .states
        .iter()
        .map(hostile_state_record_fields)
        .collect();
    serde_json::json!({
        "server_tick": state.server_tick.to_string(),
        "states": records
    })
}

/// Renders one hostile despawn batch's semantic fields.
fn hostile_despawn_fields(despawn: &mornlea_protocol::HostileDespawn) -> serde_json::Value {
    let ids: Vec<String> = despawn.ids.iter().map(|id| id.get().to_string()).collect();
    serde_json::json!({
        "server_tick": despawn.server_tick.to_string(),
        "ids": ids
    })
}

/// Reads one hostile identity an encode case carries.
///
/// The identity is the checked domain `HostileId`, so a zero value is refused at
/// the identity boundary rather than being reinterpreted.
fn hostile_id_field(value: &serde_json::Value) -> Result<mornlea_protocol::HostileId, ()> {
    let id = value.as_u64().ok_or(())?;
    mornlea_protocol::HostileId::try_new(id).map_err(|_| ())
}

/// Reads one dimension an encode case carries, mapping an unknown value to the
/// enum boundary the decoder publishes for the same bytes.
fn record_dimension_field(
    value: &serde_json::Value,
) -> Result<mornlea_domain::Dimension, mornlea_protocol::ProtocolError> {
    let dimension = value
        .as_i64()
        .ok_or(mornlea_protocol::ProtocolError::InvalidEnum)?;
    let narrowed = u8::try_from(dimension).unwrap_or(u8::MAX);
    mornlea_domain::Dimension::new(narrowed)
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidEnum)
}

/// Reads one three-component bit-string array an encode case carries.
fn record_bits_array(entry: &serde_json::Value, name: &str) -> Result<[f32; 3], ()> {
    let values = entry
        .get(name)
        .and_then(|value| value.as_array())
        .ok_or(())?;
    let mut parsed = [0f32; 3];
    for (index, slot) in parsed.iter_mut().enumerate() {
        let text = values
            .get(index)
            .and_then(|value| value.as_str())
            .ok_or(())?;
        let bits = u32::from_str_radix(text, 16).map_err(|_| ())?;
        *slot = f32::from_bits(bits);
    }
    Ok(parsed)
}

/// Reads one single bit-string field an encode case carries.
fn record_bits_text(entry: &serde_json::Value, name: &str) -> Result<f32, ()> {
    let text = entry.get(name).and_then(|value| value.as_str()).ok_or(())?;
    let bits = u32::from_str_radix(text, 16).map_err(|_| ())?;
    Ok(f32::from_bits(bits))
}

/// Reads one unsigned record byte an encode case carries.
fn record_byte(entry: &serde_json::Value, name: &str) -> Result<u8, ()> {
    u8::try_from(entry.get(name).and_then(|value| value.as_u64()).ok_or(())?).map_err(|_| ())
}

/// Builds one hostile spawn record its JSON object names.
fn hostile_spawn_record_request(
    entry: &serde_json::Value,
) -> Result<mornlea_protocol::HostileSpawnRecord, mornlea_protocol::ProtocolError> {
    let id = hostile_id_field(
        entry
            .get("id")
            .ok_or(mornlea_protocol::ProtocolError::InvalidIdentity)?,
    )
    .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?;
    let dimension = record_dimension_field(
        entry
            .get("dimension")
            .ok_or(mornlea_protocol::ProtocolError::InvalidEnum)?,
    )?;
    let position = record_bits_array(entry, "position")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let yaw = record_bits_text(entry, "yaw")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let health =
        record_byte(entry, "health").map_err(|_| mornlea_protocol::ProtocolError::InvalidRange)?;
    let kind =
        record_byte(entry, "kind").map_err(|_| mornlea_protocol::ProtocolError::InvalidEnum)?;
    Ok(mornlea_protocol::HostileSpawnRecord {
        id,
        dimension,
        position,
        yaw,
        health,
        kind,
    })
}

/// Builds one hostile state record its JSON object names.
fn hostile_state_record_request(
    entry: &serde_json::Value,
) -> Result<mornlea_protocol::HostileStateRecord, mornlea_protocol::ProtocolError> {
    let id = hostile_id_field(
        entry
            .get("id")
            .ok_or(mornlea_protocol::ProtocolError::InvalidIdentity)?,
    )
    .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?;
    let position = record_bits_array(entry, "position")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let velocity = record_bits_array(entry, "velocity")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let yaw = record_bits_text(entry, "yaw")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let health =
        record_byte(entry, "health").map_err(|_| mornlea_protocol::ProtocolError::InvalidRange)?;
    let kind =
        record_byte(entry, "kind").map_err(|_| mornlea_protocol::ProtocolError::InvalidEnum)?;
    Ok(mornlea_protocol::HostileStateRecord {
        id,
        position,
        velocity,
        yaw,
        health,
        kind,
    })
}

/// Builds the hostile spawn batch one encode case names from its typed fields.
///
/// The record is built through its public fields, so a mutated or invalid case
/// is refused by the production validation rather than by a constructor guard.
fn hostile_spawn_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::HostileSpawn, mornlea_protocol::ProtocolError> {
    let server_tick = unsigned_field(case, "server_tick");
    let mut spawns = Vec::new();
    for entry in record_array(case, "spawns") {
        spawns.push(hostile_spawn_record_request(entry)?);
    }
    Ok(mornlea_protocol::HostileSpawn {
        server_tick,
        spawns,
    })
}

/// Builds the hostile state batch one encode case names from its typed fields.
fn hostile_state_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::HostileState, mornlea_protocol::ProtocolError> {
    let server_tick = unsigned_field(case, "server_tick");
    let mut states = Vec::new();
    for entry in record_array(case, "states") {
        states.push(hostile_state_record_request(entry)?);
    }
    Ok(mornlea_protocol::HostileState {
        server_tick,
        states,
    })
}

/// Builds the hostile despawn batch one encode case names from its typed fields.
fn hostile_despawn_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::HostileDespawn, mornlea_protocol::ProtocolError> {
    let server_tick = unsigned_field(case, "server_tick");
    let mut ids = Vec::new();
    for entry in record_array(case, "ids") {
        let id = hostile_id_field(entry)
            .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?;
        ids.push(id);
    }
    Ok(mornlea_protocol::HostileDespawn { server_tick, ids })
}

/// Renders one passive spawn record's semantic fields.
///
/// The identity publishes as a decimal string so the full `u64` range stays
/// lossless, the dimension as the plain wire integer, the pose as bit strings so
/// a negative zero stays distinct, and the health as a JSON number. No kind
/// byte exists, because a passive mob has no category to publish.
fn passive_spawn_record_fields(record: &mornlea_protocol::PassiveSpawnRecord) -> serde_json::Value {
    serde_json::json!({
        "id": record.id.get().to_string(),
        "dimension": i32::from(record.dimension.get()),
        "position": [
            float_bits_text(record.position[0]),
            float_bits_text(record.position[1]),
            float_bits_text(record.position[2])
        ],
        "yaw": float_bits_text(record.yaw),
        "health": record.health
    })
}

/// Renders one passive spawn batch's semantic fields.
///
/// The records publish in wire order, never sorted, so a batch the authority
/// ordered is observed in the order it carried.
fn passive_spawn_fields(spawn: &mornlea_protocol::PassiveSpawn) -> serde_json::Value {
    let records: Vec<serde_json::Value> = spawn
        .spawns
        .iter()
        .map(passive_spawn_record_fields)
        .collect();
    serde_json::json!({
        "server_tick": spawn.server_tick.to_string(),
        "spawns": records
    })
}

/// Renders one passive state record's semantic fields, which carry the velocity
/// the spawn record never carries, the transient grazing bit, and no dimension
/// at all.
fn passive_state_record_fields(record: &mornlea_protocol::PassiveStateRecord) -> serde_json::Value {
    serde_json::json!({
        "id": record.id.get().to_string(),
        "position": [
            float_bits_text(record.position[0]),
            float_bits_text(record.position[1]),
            float_bits_text(record.position[2])
        ],
        "velocity": [
            float_bits_text(record.velocity[0]),
            float_bits_text(record.velocity[1]),
            float_bits_text(record.velocity[2])
        ],
        "yaw": float_bits_text(record.yaw),
        "health": record.health,
        "grazing": record.grazing
    })
}

/// Renders one passive state batch's semantic fields.
fn passive_state_fields(state: &mornlea_protocol::PassiveState) -> serde_json::Value {
    let records: Vec<serde_json::Value> = state
        .states
        .iter()
        .map(passive_state_record_fields)
        .collect();
    serde_json::json!({
        "server_tick": state.server_tick.to_string(),
        "states": records
    })
}

/// Renders one passive despawn record's semantic fields: the identity as a
/// decimal string and the removal reason as a plain JSON number.
fn passive_despawn_record_fields(
    record: &mornlea_protocol::PassiveDespawnRecord,
) -> serde_json::Value {
    serde_json::json!({
        "id": record.id.get().to_string(),
        "reason": record.reason
    })
}

/// Renders one passive despawn batch's semantic fields.
fn passive_despawn_fields(despawn: &mornlea_protocol::PassiveDespawn) -> serde_json::Value {
    let records: Vec<serde_json::Value> = despawn
        .despawns
        .iter()
        .map(passive_despawn_record_fields)
        .collect();
    serde_json::json!({
        "server_tick": despawn.server_tick.to_string(),
        "despawns": records
    })
}

/// Reads one passive identity an encode case carries.
///
/// The identity is the checked domain `PassiveId`, so a zero value is refused at
/// the identity boundary rather than being reinterpreted.
fn passive_id_field(value: &serde_json::Value) -> Result<mornlea_protocol::PassiveId, ()> {
    let id = value.as_u64().ok_or(())?;
    mornlea_protocol::PassiveId::try_new(id).map_err(|_| ())
}

/// Builds one passive spawn record its JSON object names.
fn passive_spawn_record_request(
    entry: &serde_json::Value,
) -> Result<mornlea_protocol::PassiveSpawnRecord, mornlea_protocol::ProtocolError> {
    let id = passive_id_field(
        entry
            .get("id")
            .ok_or(mornlea_protocol::ProtocolError::InvalidIdentity)?,
    )
    .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?;
    let dimension = record_dimension_field(
        entry
            .get("dimension")
            .ok_or(mornlea_protocol::ProtocolError::InvalidEnum)?,
    )?;
    let position = record_bits_array(entry, "position")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let yaw = record_bits_text(entry, "yaw")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let health =
        record_byte(entry, "health").map_err(|_| mornlea_protocol::ProtocolError::InvalidRange)?;
    Ok(mornlea_protocol::PassiveSpawnRecord {
        id,
        dimension,
        position,
        yaw,
        health,
    })
}

/// Builds one passive state record its JSON object names.
fn passive_state_record_request(
    entry: &serde_json::Value,
) -> Result<mornlea_protocol::PassiveStateRecord, mornlea_protocol::ProtocolError> {
    let id = passive_id_field(
        entry
            .get("id")
            .ok_or(mornlea_protocol::ProtocolError::InvalidIdentity)?,
    )
    .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?;
    let position = record_bits_array(entry, "position")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let velocity = record_bits_array(entry, "velocity")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let yaw = record_bits_text(entry, "yaw")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let health =
        record_byte(entry, "health").map_err(|_| mornlea_protocol::ProtocolError::InvalidRange)?;
    let grazing =
        record_byte(entry, "grazing").map_err(|_| mornlea_protocol::ProtocolError::InvalidEnum)?;
    Ok(mornlea_protocol::PassiveStateRecord {
        id,
        position,
        velocity,
        yaw,
        health,
        grazing,
    })
}

/// Builds one passive despawn record its JSON object names.
fn passive_despawn_record_request(
    entry: &serde_json::Value,
) -> Result<mornlea_protocol::PassiveDespawnRecord, mornlea_protocol::ProtocolError> {
    let id = passive_id_field(
        entry
            .get("id")
            .ok_or(mornlea_protocol::ProtocolError::InvalidIdentity)?,
    )
    .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?;
    let reason =
        record_byte(entry, "reason").map_err(|_| mornlea_protocol::ProtocolError::InvalidEnum)?;
    Ok(mornlea_protocol::PassiveDespawnRecord { id, reason })
}

/// Builds the passive spawn batch one encode case names from its typed fields.
///
/// The record is built through its public fields, so a mutated or invalid case
/// is refused by the production validation rather than by a constructor guard.
fn passive_spawn_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::PassiveSpawn, mornlea_protocol::ProtocolError> {
    let server_tick = unsigned_field(case, "server_tick");
    let mut spawns = Vec::new();
    for entry in record_array(case, "spawns") {
        spawns.push(passive_spawn_record_request(entry)?);
    }
    Ok(mornlea_protocol::PassiveSpawn {
        server_tick,
        spawns,
    })
}

/// Builds the passive state batch one encode case names from its typed fields.
fn passive_state_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::PassiveState, mornlea_protocol::ProtocolError> {
    let server_tick = unsigned_field(case, "server_tick");
    let mut states = Vec::new();
    for entry in record_array(case, "states") {
        states.push(passive_state_record_request(entry)?);
    }
    Ok(mornlea_protocol::PassiveState {
        server_tick,
        states,
    })
}

/// Builds the passive despawn batch one encode case names from its typed
/// fields.
fn passive_despawn_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::PassiveDespawn, mornlea_protocol::ProtocolError> {
    let server_tick = unsigned_field(case, "server_tick");
    let mut despawns = Vec::new();
    for entry in record_array(case, "despawns") {
        despawns.push(passive_despawn_record_request(entry)?);
    }
    Ok(mornlea_protocol::PassiveDespawn {
        server_tick,
        despawns,
    })
}

/// Renders one projectile spawn record's semantic fields.
///
/// The identity publishes as a decimal string so the full `u64` range stays
/// lossless, the kind and the dimension as the plain wire integers, and the
/// position and velocity as bit-string arrays so a negative zero stays
/// distinct. The record carries no yaw and no health, because a projectile is
/// a point-like transient whose orientation the client derives from its
/// velocity.
fn projectile_spawn_record_fields(
    record: &mornlea_protocol::ProjectileSpawnRecord,
) -> serde_json::Value {
    serde_json::json!({
        "id": record.id.get().to_string(),
        "kind": record.kind,
        "dimension": i32::from(record.dimension.get()),
        "position": [
            float_bits_text(record.position[0]),
            float_bits_text(record.position[1]),
            float_bits_text(record.position[2])
        ],
        "velocity": [
            float_bits_text(record.velocity[0]),
            float_bits_text(record.velocity[1]),
            float_bits_text(record.velocity[2])
        ]
    })
}

/// Renders one projectile spawn batch's semantic fields.
///
/// The records publish in wire order, never sorted, so a batch the authority
/// ordered is observed in the order it carried.
fn projectile_spawn_fields(spawn: &mornlea_protocol::ProjectileSpawn) -> serde_json::Value {
    let records: Vec<serde_json::Value> = spawn
        .spawns
        .iter()
        .map(projectile_spawn_record_fields)
        .collect();
    serde_json::json!({
        "server_tick": spawn.server_tick.to_string(),
        "spawns": records
    })
}

/// Renders one projectile state record's semantic fields, which carry the
/// identity and the position alone: the kind, the dimension and the velocity
/// are fixed for the projectile's whole life, so the mirror records them at
/// spawn and the state batch only moves the body.
fn projectile_state_record_fields(
    record: &mornlea_protocol::ProjectileStateRecord,
) -> serde_json::Value {
    serde_json::json!({
        "id": record.id.get().to_string(),
        "position": [
            float_bits_text(record.position[0]),
            float_bits_text(record.position[1]),
            float_bits_text(record.position[2])
        ]
    })
}

/// Renders one projectile state batch's semantic fields.
fn projectile_state_fields(state: &mornlea_protocol::ProjectileState) -> serde_json::Value {
    let records: Vec<serde_json::Value> = state
        .states
        .iter()
        .map(projectile_state_record_fields)
        .collect();
    serde_json::json!({
        "server_tick": state.server_tick.to_string(),
        "states": records
    })
}

/// Renders one projectile despawn batch's semantic fields.
fn projectile_despawn_fields(despawn: &mornlea_protocol::ProjectileDespawn) -> serde_json::Value {
    let ids: Vec<String> = despawn.ids.iter().map(|id| id.get().to_string()).collect();
    serde_json::json!({
        "server_tick": despawn.server_tick.to_string(),
        "ids": ids
    })
}

/// Reads one projectile identity an encode case carries.
///
/// The identity is the checked domain `ProjectileId`, so a zero value is
/// refused at the identity boundary rather than being reinterpreted.
fn projectile_id_field(value: &serde_json::Value) -> Result<mornlea_protocol::ProjectileId, ()> {
    let id = value.as_u64().ok_or(())?;
    mornlea_protocol::ProjectileId::try_new(id).map_err(|_| ())
}

/// Builds one projectile spawn record its JSON object names.
fn projectile_spawn_record_request(
    entry: &serde_json::Value,
) -> Result<mornlea_protocol::ProjectileSpawnRecord, mornlea_protocol::ProtocolError> {
    let id = projectile_id_field(
        entry
            .get("id")
            .ok_or(mornlea_protocol::ProtocolError::InvalidIdentity)?,
    )
    .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?;
    let dimension = record_dimension_field(
        entry
            .get("dimension")
            .ok_or(mornlea_protocol::ProtocolError::InvalidEnum)?,
    )?;
    let position = record_bits_array(entry, "position")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let velocity = record_bits_array(entry, "velocity")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    let kind =
        record_byte(entry, "kind").map_err(|_| mornlea_protocol::ProtocolError::InvalidEnum)?;
    Ok(mornlea_protocol::ProjectileSpawnRecord {
        id,
        kind,
        dimension,
        position,
        velocity,
    })
}

/// Builds one projectile state record its JSON object names.
fn projectile_state_record_request(
    entry: &serde_json::Value,
) -> Result<mornlea_protocol::ProjectileStateRecord, mornlea_protocol::ProtocolError> {
    let id = projectile_id_field(
        entry
            .get("id")
            .ok_or(mornlea_protocol::ProtocolError::InvalidIdentity)?,
    )
    .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?;
    let position = record_bits_array(entry, "position")
        .map_err(|_| mornlea_protocol::ProtocolError::InvalidFloat)?;
    Ok(mornlea_protocol::ProjectileStateRecord { id, position })
}

/// Builds the projectile spawn batch one encode case names from its typed
/// fields.
///
/// The record is built through its public fields, so a mutated or invalid case
/// is refused by the production validation rather than by a constructor guard.
fn projectile_spawn_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::ProjectileSpawn, mornlea_protocol::ProtocolError> {
    let server_tick = unsigned_field(case, "server_tick");
    let mut spawns = Vec::new();
    for entry in record_array(case, "spawns") {
        spawns.push(projectile_spawn_record_request(entry)?);
    }
    Ok(mornlea_protocol::ProjectileSpawn {
        server_tick,
        spawns,
    })
}

/// Builds the projectile state batch one encode case names from its typed
/// fields.
fn projectile_state_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::ProjectileState, mornlea_protocol::ProtocolError> {
    let server_tick = unsigned_field(case, "server_tick");
    let mut states = Vec::new();
    for entry in record_array(case, "states") {
        states.push(projectile_state_record_request(entry)?);
    }
    Ok(mornlea_protocol::ProjectileState {
        server_tick,
        states,
    })
}

/// Builds the projectile despawn batch one encode case names from its typed
/// fields.
fn projectile_despawn_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::ProjectileDespawn, mornlea_protocol::ProtocolError> {
    let server_tick = unsigned_field(case, "server_tick");
    let mut ids = Vec::new();
    for entry in record_array(case, "ids") {
        let id = projectile_id_field(entry)
            .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)?;
        ids.push(id);
    }
    Ok(mornlea_protocol::ProjectileDespawn { server_tick, ids })
}

/// Builds the inventory state one encode case names from its typed fields.
///
/// The record is built through its public fields, so a mutated or invalid
/// case is refused by the production validation rather than by a constructor
/// guard.
fn inventory_state_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::InventoryState, mornlea_protocol::ProtocolError> {
    let selected = byte_field(case, "selected");
    let mut hotbar = [mornlea_protocol::ItemStack::EMPTY; 9];
    let mut requested = inventory_stack_array_request(case, "hotbar", 9).into_iter();
    for slot in &mut hotbar {
        *slot = requested
            .next()
            .expect("the request carries nine hotbar slots");
    }
    let mut backpack = [mornlea_protocol::ItemStack::EMPTY; 27];
    let mut requested = inventory_stack_array_request(case, "backpack", 27).into_iter();
    for slot in &mut backpack {
        *slot = requested
            .next()
            .expect("the request carries twenty-seven backpack slots");
    }
    Ok(mornlea_protocol::InventoryState {
        selected,
        hotbar,
        backpack,
    })
}

/// Reads one stack object field an encode case carries.
fn inventory_stack_object_field<'a>(case: &'a FrozenCase, name: &str) -> &'a serde_json::Value {
    case.input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get(name)
        .unwrap_or_else(|| panic!("case {} names no {name} stack", case.id))
}

/// Builds the crafting state one encode case names from its typed fields.
fn crafting_state_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::CraftingState, mornlea_protocol::ProtocolError> {
    let size = byte_field(case, "size");
    let mut slots = [mornlea_protocol::ItemStack::EMPTY; 9];
    let mut requested = inventory_stack_array_request(case, "slots", 9).into_iter();
    for slot in &mut slots {
        *slot = requested
            .next()
            .expect("the request carries nine grid slots");
    }
    let output = inventory_stack_request(inventory_stack_object_field(case, "output"), case);
    Ok(mornlea_protocol::CraftingState {
        size,
        slots,
        output,
    })
}

/// Builds the furnace state one encode case names from its typed fields.
fn furnace_state_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::FurnaceState, mornlea_protocol::ProtocolError> {
    let stack = |name: &str| -> mornlea_protocol::ItemStack {
        inventory_stack_request(inventory_stack_object_field(case, name), case)
    };
    Ok(mornlea_protocol::FurnaceState {
        furnace: inventory_container_request(case, "furnace"),
        input: stack("input"),
        fuel: stack("fuel"),
        output: stack("output"),
        progress_ticks: byte_field(case, "progress_ticks"),
        burn_ticks: u16_field(case, "burn_ticks"),
    })
}

/// Builds the chest state one encode case names from its typed fields.
fn chest_state_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::ChestState, mornlea_protocol::ProtocolError> {
    let mut items = [mornlea_protocol::ItemStack::EMPTY; 27];
    let mut requested = inventory_stack_array_request(case, "items", 27).into_iter();
    for slot in &mut items {
        *slot = requested
            .next()
            .expect("the request carries twenty-seven chest slots");
    }
    Ok(mornlea_protocol::ChestState {
        chest: inventory_container_request(case, "chest"),
        items,
    })
}

/// Builds the container closure one encode case names from its typed fields.
fn container_closed_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::ContainerClosed, mornlea_protocol::ProtocolError> {
    Ok(mornlea_protocol::ContainerClosed {
        container: inventory_container_request(case, "container"),
    })
}

/// Builds the player state one encode case names from its typed fields, so a
/// mutated or invalid case is refused by the production validation rather
/// than by a constructor guard.
fn player_state_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::PlayerState, mornlea_protocol::ProtocolError> {
    let dimension = dimension_field(case)?;
    let mining_target = triple_i32_field(case, "mining_target");
    mornlea_protocol::PlayerState::new(
        unsigned_field(case, "server_tick"),
        unsigned_field(case, "last_input_sequence"),
        dimension,
        vec3_bits_field(case, "position"),
        vec3_bits_field(case, "velocity"),
        float_bits_field(case, "yaw"),
        float_bits_field(case, "pitch"),
        bool_field(case, "on_ground"),
        bool_field(case, "ready"),
        bool_field(case, "reset"),
        bool_field(case, "mining_active"),
        mornlea_protocol::BlockPos {
            x: mining_target[0],
            y: mining_target[1],
            z: mining_target[2],
        },
        u16_field(case, "mining_progress_ticks"),
        u16_field(case, "mining_required_ticks"),
        bool_field(case, "mining_harvestable"),
        byte_field(case, "health"),
        u16_field(case, "oxygen"),
        byte_field(case, "hunger"),
        bool_field(case, "saturation_zero"),
        u16_field(case, "day_phase_offset"),
        unsigned_field(case, "world_time_ticks"),
        byte_field(case, "weather_kind"),
        byte_field(case, "season"),
        byte_field(case, "season_progress"),
        i8_field(case, "temperature"),
        byte_field(case, "armor_points"),
    )
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
        MOVE_INVENTORY_STACK_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::MoveInventoryStack::decode(&case.input) {
                Ok(stack) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_move_fields(stack.sequence, stack.from, stack.to),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let (sequence, from, to) = client_move_request(case);
                // The record is built through its public fields, so an
                // out-of-range or same-slot pair is refused by the production
                // validation rather than by a constructor guard.
                let stack = mornlea_protocol::MoveInventoryStack { sequence, from, to };
                encode_ok_outcome(
                    stack.encode(),
                    client_move_fields(stack.sequence, stack.from, stack.to),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        MOVE_CRAFTING_STACK_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::MoveCraftingStack::decode(&case.input) {
                Ok(stack) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_move_fields(stack.sequence, stack.from, stack.to),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let (sequence, from, to) = client_move_request(case);
                // The record is built through its public fields, so an
                // out-of-range, same-slot or both-in-inventory pair is refused
                // by the production validation rather than by a constructor
                // guard.
                let stack = mornlea_protocol::MoveCraftingStack { sequence, from, to };
                encode_ok_outcome(
                    stack.encode(),
                    client_move_fields(stack.sequence, stack.from, stack.to),
                )
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        CLOSE_CONTAINER_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::CloseContainer::decode(&case.input) {
                Ok(close) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_sequence_fields(close.sequence),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The value gate is total, so the record is built through its
                // public field and the surface stays uniform with the families
                // that do carry a rule.
                let close = mornlea_protocol::CloseContainer {
                    sequence: unsigned_field(case, "sequence"),
                };
                encode_ok_outcome(close.encode(), client_sequence_fields(close.sequence))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        DROP_SELECTED_ITEM_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::DropSelectedItem::decode(&case.input) {
                Ok(drop) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_sequence_fields(drop.sequence),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let drop = mornlea_protocol::DropSelectedItem {
                    sequence: unsigned_field(case, "sequence"),
                };
                encode_ok_outcome(drop.encode(), client_sequence_fields(drop.sequence))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        EQUIP_ARMOR_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::EquipArmor::decode(&case.input) {
                Ok(equip) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_sequence_fields(equip.sequence),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let equip = mornlea_protocol::EquipArmor {
                    sequence: unsigned_field(case, "sequence"),
                };
                encode_ok_outcome(equip.encode(), client_sequence_fields(equip.sequence))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        TAKE_CRAFTING_OUTPUT_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::TakeCraftingOutput::decode(&case.input) {
                Ok(take) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_sequence_fields(take.sequence),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The record is built through its public field, so a zero
                // sequence is refused by the production validation rather than
                // by a constructor guard.
                let take = mornlea_protocol::TakeCraftingOutput {
                    sequence: unsigned_field(case, "sequence"),
                };
                encode_ok_outcome(take.encode(), client_sequence_fields(take.sequence))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        MOVE_CONTAINER_STACK_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::MoveContainerStack::decode(&case.input) {
                Ok(command) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_stack_view_fields(&command),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The record is built through its public fields, so a malformed
                // reference, a same-slot pair or the furnace output target is
                // refused by the production validation rather than by a
                // constructor guard.
                let command = mornlea_protocol::MoveContainerStack {
                    sequence: unsigned_field(case, "sequence"),
                    container: client_container_request(case),
                    from: byte_field(case, "from"),
                    to: byte_field(case, "to"),
                };
                encode_ok_outcome(command.encode(), client_stack_view_fields(&command))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        MOVE_STACK_PARTIAL_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::MoveStackPartial::decode(&case.input) {
                Ok(command) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_stack_partial_fields(&command),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The record is built through its public fields, so an unknown
                // view, a reference that does not match its view, an
                // out-of-range index or a same-slot pair is refused by the
                // production validation rather than by a constructor guard.
                let command = mornlea_protocol::MoveStackPartial {
                    sequence: unsigned_field(case, "sequence"),
                    container: client_container_request(case),
                    view: byte_field(case, "view"),
                    from: byte_field(case, "from"),
                    to: byte_field(case, "to"),
                    single: bool_field(case, "single"),
                };
                encode_ok_outcome(command.encode(), client_stack_partial_fields(&command))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        QUICK_MOVE_STACK_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::QuickMoveStack::decode(&case.input) {
                Ok(command) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_quick_move_fields(&command),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let command = mornlea_protocol::QuickMoveStack {
                    sequence: unsigned_field(case, "sequence"),
                    container: client_container_request(case),
                    view: byte_field(case, "view"),
                    from: byte_field(case, "from"),
                };
                encode_ok_outcome(command.encode(), client_quick_move_fields(&command))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        DROP_STACK_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::DropStack::decode(&case.input) {
                Ok(command) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_drop_stack_fields(&command),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let command = mornlea_protocol::DropStack {
                    sequence: unsigned_field(case, "sequence"),
                    container: client_container_request(case),
                    view: byte_field(case, "view"),
                    slot: byte_field(case, "slot"),
                };
                encode_ok_outcome(command.encode(), client_drop_stack_fields(&command))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        CHAT_COMMAND_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ChatCommand::decode(&case.input) {
                Ok(command) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": client_chat_fields(&command.text),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The record is built through its public field, so an untrimmed,
                // empty, control-carrying or oversized text is refused by the
                // production validation rather than by a constructor guard.
                let command = mornlea_protocol::ChatCommand {
                    text: text_field(case, "text"),
                };
                encode_ok_outcome(command.encode(), client_chat_fields(&command.text))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        BLOCK_CHANGES_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::BlockChanges::decode(&case.input) {
                Ok(changes) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": block_changes_fields(&changes),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The record is built through its public fields, so a mutated
                // revision, an unregistered block or an out-of-span position
                // is refused by the production validation rather than by a
                // constructor guard. The dimension is the checked domain
                // value, so an unknown raw dimension publishes its rejection
                // directly instead of being constructible.
                let dimension = match dimension_field(case) {
                    Ok(dimension) => dimension,
                    Err(err) => return packet_error(err),
                };
                let changes: Vec<mornlea_protocol::BlockChange> = record_array(case, "changes")
                    .into_iter()
                    .map(|entry| block_change_request(entry, case))
                    .collect();
                let record = mornlea_protocol::BlockChanges {
                    dimension,
                    chunk_x: i32_field(case, "chunk_x"),
                    chunk_z: i32_field(case, "chunk_z"),
                    base_revision: unsigned_field(case, "base_revision"),
                    new_revision: unsigned_field(case, "new_revision"),
                    changes,
                };
                encode_ok_outcome(record.encode(), block_changes_fields(&record))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        FORGET_CHUNKS_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ForgetChunks::decode(&case.input) {
                Ok(forget) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": forget_chunks_fields(&forget),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The record is built through its public fields, so a mutated
                // empty batch or a duplicate chunk is refused by the
                // production validation, and the submitted order is preserved
                // from the request into the outcome.
                let dimension = match dimension_field(case) {
                    Ok(dimension) => dimension,
                    Err(err) => return packet_error(err),
                };
                let chunks: Vec<(i32, i32)> = record_array(case, "chunks")
                    .into_iter()
                    .map(|entry| chunk_pos_request(entry, case))
                    .collect();
                let record = mornlea_protocol::ForgetChunks { dimension, chunks };
                encode_ok_outcome(record.encode(), forget_chunks_fields(&record))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        CHUNK_SNAPSHOT_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ChunkSnapshot::decode(&case.input) {
                Ok(snapshot) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": snapshot_fields(&snapshot),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => snapshot_encode_outcome(case),
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PLAYER_STATE_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::PlayerState::decode(&case.input) {
                Ok(state) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": player_state_fields(&state),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => match player_state_request(case) {
                Ok(state) => encode_ok_outcome(state.encode(), player_state_fields(&state)),
                Err(err) => packet_error(err),
            },
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        COMMAND_REJECTED_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::CommandRejected::decode(&case.input) {
                Ok(rejected) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": command_rejected_fields(&rejected),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The record is built through its public fields, so a reason
                // outside the frozen interval is refused by the production
                // validation rather than by a constructor guard.
                let rejected = mornlea_protocol::CommandRejected {
                    sequence: unsigned_field(case, "sequence"),
                    reason: byte_field(case, "reason"),
                };
                encode_ok_outcome(rejected.encode(), command_rejected_fields(&rejected))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PLACE_BLOCK_SUCCEEDED_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::PlaceBlockSucceeded::decode(&case.input) {
                Ok(ack) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": place_block_succeeded_fields(&ack),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let ack = mornlea_protocol::PlaceBlockSucceeded {
                    sequence: unsigned_field(case, "sequence"),
                };
                encode_ok_outcome(ack.encode(), place_block_succeeded_fields(&ack))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        COMBAT_HIT_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::CombatHit::decode(&case.input) {
                Ok(hit) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": combat_hit_fields(&hit),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                // The record is built through its public fields, so a zero
                // tick, an out-of-range damage or an unknown kind is refused
                // by the production validation rather than by a constructor
                // guard.
                let hit = mornlea_protocol::CombatHit {
                    server_tick: unsigned_field(case, "server_tick"),
                    damage: byte_field(case, "damage"),
                    target_kind: byte_field(case, "target_kind"),
                };
                encode_ok_outcome(hit.encode(), combat_hit_fields(&hit))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        INVENTORY_STATE_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::InventoryState::decode(&case.input) {
                Ok(state) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": inventory_state_fields(&state),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let state = match inventory_state_request(case) {
                    Ok(state) => state,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(state.encode(), inventory_state_fields(&state))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        CRAFTING_STATE_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::CraftingState::decode(&case.input) {
                Ok(state) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": crafting_state_fields(&state),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let state = match crafting_state_request(case) {
                    Ok(state) => state,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(state.encode(), crafting_state_fields(&state))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        FURNACE_STATE_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::FurnaceState::decode(&case.input) {
                Ok(state) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": furnace_state_fields(&state),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let state = match furnace_state_request(case) {
                    Ok(state) => state,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(state.encode(), furnace_state_fields(&state))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        CHEST_STATE_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ChestState::decode(&case.input) {
                Ok(state) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": chest_state_fields(&state),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let state = match chest_state_request(case) {
                    Ok(state) => state,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(state.encode(), chest_state_fields(&state))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        CONTAINER_CLOSED_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ContainerClosed::decode(&case.input) {
                Ok(closed) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": container_closed_fields(&closed),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let closed = match container_closed_request(case) {
                    Ok(closed) => closed,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(closed.encode(), container_closed_fields(&closed))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        REMOTE_PLAYER_SPAWN_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::RemotePlayerSpawn::decode(&case.input) {
                Ok(spawn) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": remote_player_spawn_fields(&spawn),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let spawn = match remote_player_spawn_request(case) {
                    Ok(spawn) => spawn,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(spawn.encode(), remote_player_spawn_fields(&spawn))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        REMOTE_PLAYER_DESPAWN_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::RemotePlayerDespawn::decode(&case.input) {
                Ok(despawn) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": remote_player_despawn_fields(&despawn),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let despawn = match remote_player_despawn_request(case) {
                    Ok(despawn) => despawn,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(despawn.encode(), remote_player_despawn_fields(&despawn))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        REMOTE_PLAYER_STATES_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::RemotePlayerStates::decode(&case.input) {
                Ok(states) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": remote_player_states_fields(&states),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let states = match remote_player_states_request(case) {
                    Ok(states) => states,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(states.encode(), remote_player_states_fields(&states))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        COMPANION_SPAWN_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::CompanionSpawn::decode(&case.input) {
                Ok(spawn) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": companion_spawn_fields(&spawn),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let spawn = match companion_spawn_request(case) {
                    Ok(spawn) => spawn,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(spawn.encode(), companion_spawn_fields(&spawn))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        COMPANION_DESPAWN_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::CompanionDespawn::decode(&case.input) {
                Ok(despawn) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": companion_despawn_fields(&despawn),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let despawn = match companion_despawn_request(case) {
                    Ok(despawn) => despawn,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(despawn.encode(), companion_despawn_fields(&despawn))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        CHAT_EVENT_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ChatEvent::decode(&case.input) {
                Ok(event) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": chat_event_fields(&event),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let event = match chat_event_request(case) {
                    Ok(event) => event,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(event.encode(), chat_event_fields(&event))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        COMPANION_STATES_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::CompanionStates::decode(&case.input) {
                Ok(states) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": companion_states_fields(&states),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let states = match companion_states_request(case) {
                    Ok(states) => states,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(states.encode(), companion_states_fields(&states))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        ITEM_DROP_UPSERTS_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ItemDropUpserts::decode(&case.input) {
                Ok(upserts) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": item_drop_upserts_fields(&upserts),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let upserts = item_drop_upserts_request(case);
                encode_ok_outcome(upserts.encode(), item_drop_upserts_fields(&upserts))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        ITEM_DROP_REMOVES_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ItemDropRemoves::decode(&case.input) {
                Ok(removes) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": item_drop_removes_fields(&removes),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let removes = item_drop_removes_request(case);
                encode_ok_outcome(removes.encode(), item_drop_removes_fields(&removes))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        HOSTILE_SPAWN_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::HostileSpawn::decode(&case.input) {
                Ok(spawn) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": hostile_spawn_fields(&spawn),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let spawn = match hostile_spawn_request(case) {
                    Ok(spawn) => spawn,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(spawn.encode(), hostile_spawn_fields(&spawn))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        HOSTILE_STATE_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::HostileState::decode(&case.input) {
                Ok(state) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": hostile_state_fields(&state),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let state = match hostile_state_request(case) {
                    Ok(state) => state,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(state.encode(), hostile_state_fields(&state))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        HOSTILE_DESPAWN_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::HostileDespawn::decode(&case.input) {
                Ok(despawn) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": hostile_despawn_fields(&despawn),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let despawn = match hostile_despawn_request(case) {
                    Ok(despawn) => despawn,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(despawn.encode(), hostile_despawn_fields(&despawn))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PASSIVE_SPAWN_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::PassiveSpawn::decode(&case.input) {
                Ok(spawn) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": passive_spawn_fields(&spawn),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let spawn = match passive_spawn_request(case) {
                    Ok(spawn) => spawn,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(spawn.encode(), passive_spawn_fields(&spawn))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PASSIVE_STATE_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::PassiveState::decode(&case.input) {
                Ok(state) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": passive_state_fields(&state),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let state = match passive_state_request(case) {
                    Ok(state) => state,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(state.encode(), passive_state_fields(&state))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PASSIVE_DESPAWN_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::PassiveDespawn::decode(&case.input) {
                Ok(despawn) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": passive_despawn_fields(&despawn),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let despawn = match passive_despawn_request(case) {
                    Ok(despawn) => despawn,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(despawn.encode(), passive_despawn_fields(&despawn))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PROJECTILE_SPAWN_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ProjectileSpawn::decode(&case.input) {
                Ok(spawn) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": projectile_spawn_fields(&spawn),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let spawn = match projectile_spawn_request(case) {
                    Ok(spawn) => spawn,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(spawn.encode(), projectile_spawn_fields(&spawn))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PROJECTILE_STATE_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ProjectileState::decode(&case.input) {
                Ok(state) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": projectile_state_fields(&state),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let state = match projectile_state_request(case) {
                    Ok(state) => state,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(state.encode(), projectile_state_fields(&state))
            }
            other => panic!("unsupported packet operation for {}: {other}", case.id),
        },
        PROJECTILE_DESPAWN_FAMILY => match case.operation.as_str() {
            "decode" => match mornlea_protocol::ProjectileDespawn::decode(&case.input) {
                Ok(despawn) => serde_json::json!({
                    "category": PACKET_OUTCOME_CATEGORY,
                    "fields": projectile_despawn_fields(&despawn),
                    "kind": "ok"
                }),
                Err(err) => packet_error(err),
            },
            "encode" => {
                let despawn = match projectile_despawn_request(case) {
                    Ok(despawn) => despawn,
                    Err(err) => return packet_error(err),
                };
                encode_ok_outcome(despawn.encode(), projectile_despawn_fields(&despawn))
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
        | PLACE_WATER_FAMILY
        | MOVE_INVENTORY_STACK_FAMILY
        | MOVE_CRAFTING_STACK_FAMILY
        | CLOSE_CONTAINER_FAMILY
        | DROP_SELECTED_ITEM_FAMILY
        | EQUIP_ARMOR_FAMILY
        | TAKE_CRAFTING_OUTPUT_FAMILY
        | MOVE_CONTAINER_STACK_FAMILY
        | MOVE_STACK_PARTIAL_FAMILY
        | QUICK_MOVE_STACK_FAMILY
        | DROP_STACK_FAMILY
        | CHAT_COMMAND_FAMILY
        | BLOCK_CHANGES_FAMILY
        | FORGET_CHUNKS_FAMILY
        | CHUNK_SNAPSHOT_FAMILY
        | PLAYER_STATE_FAMILY
        | COMMAND_REJECTED_FAMILY
        | PLACE_BLOCK_SUCCEEDED_FAMILY
        | COMBAT_HIT_FAMILY
        | INVENTORY_STATE_FAMILY
        | CRAFTING_STATE_FAMILY
        | FURNACE_STATE_FAMILY
        | CHEST_STATE_FAMILY
        | CONTAINER_CLOSED_FAMILY
        | REMOTE_PLAYER_SPAWN_FAMILY
        | REMOTE_PLAYER_DESPAWN_FAMILY
        | REMOTE_PLAYER_STATES_FAMILY
        | COMPANION_SPAWN_FAMILY
        | COMPANION_DESPAWN_FAMILY
        | COMPANION_STATES_FAMILY
        | ITEM_DROP_UPSERTS_FAMILY
        | ITEM_DROP_REMOVES_FAMILY
        | HOSTILE_SPAWN_FAMILY
        | HOSTILE_STATE_FAMILY
        | HOSTILE_DESPAWN_FAMILY
        | PASSIVE_SPAWN_FAMILY
        | PASSIVE_STATE_FAMILY
        | PASSIVE_DESPAWN_FAMILY
        | PROJECTILE_SPAWN_FAMILY
        | PROJECTILE_STATE_FAMILY
        | PROJECTILE_DESPAWN_FAMILY => dispatch_packet(case),
        CHAT_EVENT_FAMILY => dispatch_packet(case),
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

/// The case identities the simple inventory and crafting command group
/// registers. They mirror the Go producer's registration, so a case that only
/// one side names is a mismatch rather than a shared name. The merged manifest
/// sorts case IDs, so the comparison sorts this list too.
const CLIENT_INVENTORY_CASE_IDS: [&str; 28] = [
    "protocol.client.CloseContainer/45/decode-trailing-byte",
    "protocol.client.CloseContainer/45/decode-truncated-byte",
    "protocol.client.CloseContainer/45/decode-valid",
    "protocol.client.CloseContainer/45/encode-valid",
    "protocol.client.DropSelectedItem/45/decode-trailing-byte",
    "protocol.client.DropSelectedItem/45/decode-truncated-byte",
    "protocol.client.DropSelectedItem/45/decode-valid",
    "protocol.client.DropSelectedItem/45/encode-valid",
    "protocol.client.EquipArmor/45/decode-trailing-byte",
    "protocol.client.EquipArmor/45/decode-truncated-byte",
    "protocol.client.EquipArmor/45/decode-valid",
    "protocol.client.EquipArmor/45/encode-valid",
    "protocol.client.MoveCraftingStack/45/decode-above-view",
    "protocol.client.MoveCraftingStack/45/decode-inventory-to-inventory",
    "protocol.client.MoveCraftingStack/45/decode-same-slot",
    "protocol.client.MoveCraftingStack/45/decode-valid",
    "protocol.client.MoveCraftingStack/45/encode-inventory-to-inventory",
    "protocol.client.MoveCraftingStack/45/encode-valid",
    "protocol.client.MoveInventoryStack/45/decode-from-above-range",
    "protocol.client.MoveInventoryStack/45/decode-same-slot",
    "protocol.client.MoveInventoryStack/45/decode-to-above-range",
    "protocol.client.MoveInventoryStack/45/decode-valid",
    "protocol.client.MoveInventoryStack/45/encode-same-slot",
    "protocol.client.MoveInventoryStack/45/encode-valid",
    "protocol.client.TakeCraftingOutput/45/decode-valid",
    "protocol.client.TakeCraftingOutput/45/decode-zero-sequence",
    "protocol.client.TakeCraftingOutput/45/encode-valid",
    "protocol.client.TakeCraftingOutput/45/encode-zero-sequence",
];

/// Reports whether one family belongs to the simple inventory and crafting
/// command producer group.
fn is_client_inventory_family(family: &str) -> bool {
    matches!(
        family,
        MOVE_INVENTORY_STACK_FAMILY
            | MOVE_CRAFTING_STACK_FAMILY
            | CLOSE_CONTAINER_FAMILY
            | DROP_SELECTED_ITEM_FAMILY
            | EQUIP_ARMOR_FAMILY
            | TAKE_CRAFTING_OUTPUT_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_client_inventory_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before that merge this test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let client_inventory: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_client_inventory_family(&case.family))
        .collect();
    let executed: Vec<&str> = client_inventory
        .iter()
        .map(|case| case.id.as_str())
        .collect();
    let mut expected: Vec<&str> = CLIENT_INVENTORY_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the client inventory selection does not carry the reviewed case set"
    );
    assert!(
        !client_inventory.is_empty(),
        "the client inventory selection executed zero cases"
    );
    for case in client_inventory {
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

/// The case identities the container and view-addressed stack command group
/// registers. They mirror the Go producer's registration, so a case that only
/// one side names is a mismatch rather than a shared name. The merged manifest
/// sorts case IDs, so the comparison sorts this list too.
const CLIENT_STACK_VIEWS_CASE_IDS: [&str; 29] = [
    "protocol.client.DropStack/45/decode-inventory-index-above",
    "protocol.client.DropStack/45/decode-nonzero-ref-inventory-view",
    "protocol.client.DropStack/45/decode-unknown-view",
    "protocol.client.DropStack/45/decode-valid",
    "protocol.client.DropStack/45/encode-valid",
    "protocol.client.MoveContainerStack/45/decode-chest-index-above",
    "protocol.client.MoveContainerStack/45/decode-foreign-dimension",
    "protocol.client.MoveContainerStack/45/decode-output-target",
    "protocol.client.MoveContainerStack/45/decode-ref-slot-above-range",
    "protocol.client.MoveContainerStack/45/decode-same-slot",
    "protocol.client.MoveContainerStack/45/decode-unknown-kind",
    "protocol.client.MoveContainerStack/45/decode-valid",
    "protocol.client.MoveContainerStack/45/decode-zero-generation",
    "protocol.client.MoveContainerStack/45/encode-output-target",
    "protocol.client.MoveContainerStack/45/encode-valid",
    "protocol.client.MoveStackPartial/45/decode-furnace-index-above",
    "protocol.client.MoveStackPartial/45/decode-nonzero-ref-inventory-view",
    "protocol.client.MoveStackPartial/45/decode-same-slot",
    "protocol.client.MoveStackPartial/45/decode-single-tag-two",
    "protocol.client.MoveStackPartial/45/decode-unknown-view",
    "protocol.client.MoveStackPartial/45/decode-valid",
    "protocol.client.MoveStackPartial/45/decode-zero-generation-container-view",
    "protocol.client.MoveStackPartial/45/encode-nonzero-ref-inventory-view",
    "protocol.client.MoveStackPartial/45/encode-valid",
    "protocol.client.QuickMoveStack/45/decode-crafting-index-above",
    "protocol.client.QuickMoveStack/45/decode-nonzero-ref-crafting-view",
    "protocol.client.QuickMoveStack/45/decode-unknown-view",
    "protocol.client.QuickMoveStack/45/decode-valid",
    "protocol.client.QuickMoveStack/45/encode-valid",
];

/// Reports whether one family belongs to the container and view-addressed
/// stack command producer group.
fn is_client_stack_views_family(family: &str) -> bool {
    matches!(
        family,
        MOVE_CONTAINER_STACK_FAMILY
            | MOVE_STACK_PARTIAL_FAMILY
            | QUICK_MOVE_STACK_FAMILY
            | DROP_STACK_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_client_stack_views_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before that merge this test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let client_stack_views: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_client_stack_views_family(&case.family))
        .collect();
    let executed: Vec<&str> = client_stack_views
        .iter()
        .map(|case| case.id.as_str())
        .collect();
    let mut expected: Vec<&str> = CLIENT_STACK_VIEWS_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the client stack view selection does not carry the reviewed case set"
    );
    assert!(
        !client_stack_views.is_empty(),
        "the client stack view selection executed zero cases"
    );
    for case in client_stack_views {
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

/// The case identities the unsequenced chat command group registers. They
/// mirror the Go producer's registration, so a case that only one side names is
/// a mismatch rather than a shared name. The merged manifest sorts case IDs,
/// so the comparison sorts this list too.
const CLIENT_CHAT_CASE_IDS: [&str; 13] = [
    "protocol.client.ChatCommand/45/decode-control-text",
    "protocol.client.ChatCommand/45/decode-empty-text",
    "protocol.client.ChatCommand/45/decode-invalid-utf8",
    "protocol.client.ChatCommand/45/decode-max-text",
    "protocol.client.ChatCommand/45/decode-noncanonical-length",
    "protocol.client.ChatCommand/45/decode-payload-above-wire-ceiling",
    "protocol.client.ChatCommand/45/decode-trailing-byte",
    "protocol.client.ChatCommand/45/decode-truncated",
    "protocol.client.ChatCommand/45/decode-untrimmed-leading-nbsp",
    "protocol.client.ChatCommand/45/decode-valid",
    "protocol.client.ChatCommand/45/encode-empty-text",
    "protocol.client.ChatCommand/45/encode-text-above-bound",
    "protocol.client.ChatCommand/45/encode-valid",
];

/// Reports whether one family belongs to the unsequenced chat command
/// producer group.
fn is_client_chat_family(family: &str) -> bool {
    family == CHAT_COMMAND_FAMILY
}
#[test]
fn protocol_corpus_packet_client_chat_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before that merge this test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let client_chat: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_client_chat_family(&case.family))
        .collect();
    let executed: Vec<&str> = client_chat.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = CLIENT_CHAT_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the client chat selection does not carry the reviewed case set"
    );
    assert!(
        !client_chat.is_empty(),
        "the client chat selection executed zero cases"
    );
    for case in client_chat {
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

/// The case identities the world delta group registers. They mirror the Go
/// producer's registration, so a case that only one side names is a mismatch
/// rather than a shared name. The merged manifest sorts case IDs, so the
/// comparison sorts this list too.
///
/// The count is the reviewed table's enumerated labels: fourteen block-change
/// cases beside the eight forget-chunk cases.
const WORLD_DELTA_CASE_IDS: [&str; 22] = [
    "protocol.server.BlockChanges/45/decode-base-zero",
    "protocol.server.BlockChanges/45/decode-count-above-max",
    "protocol.server.BlockChanges/45/decode-empty-barrier",
    "protocol.server.BlockChanges/45/decode-revision-gap",
    "protocol.server.BlockChanges/45/decode-trailing-byte",
    "protocol.server.BlockChanges/45/decode-truncated",
    "protocol.server.BlockChanges/45/decode-unregistered-block",
    "protocol.server.BlockChanges/45/decode-unsorted-index",
    "protocol.server.BlockChanges/45/decode-valid",
    "protocol.server.BlockChanges/45/decode-wrong-chunk",
    "protocol.server.BlockChanges/45/decode-y-above-world",
    "protocol.server.BlockChanges/45/encode-empty-barrier",
    "protocol.server.BlockChanges/45/encode-valid",
    "protocol.server.BlockChanges/45/encode-y-above-world",
    "protocol.server.ForgetChunks/45/decode-dimension-two",
    "protocol.server.ForgetChunks/45/decode-duplicate-chunk",
    "protocol.server.ForgetChunks/45/decode-trailing-byte",
    "protocol.server.ForgetChunks/45/decode-truncated",
    "protocol.server.ForgetChunks/45/decode-valid",
    "protocol.server.ForgetChunks/45/decode-zero-count",
    "protocol.server.ForgetChunks/45/encode-duplicate-chunk",
    "protocol.server.ForgetChunks/45/encode-valid",
];

/// Reports whether one family belongs to the world delta producer group.
fn is_world_delta_family(family: &str) -> bool {
    family == BLOCK_CHANGES_FAMILY || family == FORGET_CHUNKS_FAMILY
}
#[test]
fn protocol_corpus_packet_world_delta_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before that merge this test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let world_delta: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_world_delta_family(&case.family))
        .collect();
    let executed: Vec<&str> = world_delta.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = WORLD_DELTA_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the world delta selection does not carry the reviewed case set"
    );
    assert!(
        !world_delta.is_empty(),
        "the world delta selection executed zero cases"
    );
    for case in world_delta {
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

/// The case identities the chunk-snapshot group registers. They mirror the Go
/// producer's registration, so a case that only one side names is a mismatch
/// rather than a shared name. The merged manifest sorts case IDs, so the
/// comparison sorts this list too.
///
/// The count is the reviewed table's enumerated labels: the committed fixture
/// and the mixed vector as decode cases, the mixed and all-single vectors as
/// encode cases, the five logical-layer encode negatives, and the four
/// compressed-layer decode negatives.
const SNAPSHOT_CASE_IDS: [&str; 13] = [
    "protocol.server.ChunkSnapshot/45/decode-checksum-failure",
    "protocol.server.ChunkSnapshot/45/decode-compressed-length-above-cap",
    "protocol.server.ChunkSnapshot/45/decode-decoded-length-above-cap",
    "protocol.server.ChunkSnapshot/45/decode-fixture",
    "protocol.server.ChunkSnapshot/45/decode-truncated-zstd-frame",
    "protocol.server.ChunkSnapshot/45/decode-valid-mixed",
    "protocol.server.ChunkSnapshot/45/encode-23-sections",
    "protocol.server.ChunkSnapshot/45/encode-direct-high-bits",
    "protocol.server.ChunkSnapshot/45/encode-palette-index-out-of-range",
    "protocol.server.ChunkSnapshot/45/encode-revision-zero",
    "protocol.server.ChunkSnapshot/45/encode-valid-all-single",
    "protocol.server.ChunkSnapshot/45/encode-valid-mixed",
    "protocol.server.ChunkSnapshot/45/encode-y-order-swap",
];

/// Reports whether one family belongs to the chunk-snapshot producer group.
fn is_chunk_snapshot_family(family: &str) -> bool {
    family == CHUNK_SNAPSHOT_FAMILY
}

#[test]
fn protocol_corpus_packet_chunk_snapshot_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before that merge this test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let snapshot: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_chunk_snapshot_family(&case.family))
        .collect();
    let executed: Vec<&str> = snapshot.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = SNAPSHOT_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the chunk snapshot selection does not carry the reviewed case set"
    );
    assert!(
        !snapshot.is_empty(),
        "the chunk snapshot selection executed zero cases"
    );
    for case in snapshot {
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

/// The case identities the player and private outcome group registers. They
/// mirror the Go producer's registration, so a case that only one side names
/// is a mismatch rather than a shared name. The merged manifest sorts case
/// IDs, so the comparison sorts this list too.
///
/// The count is the reviewed table's enumerated labels: the canonical player
/// state vector and its active mining variant, the nine one-boundary player
/// state rejections beside their encode twin, the frozen reject-reason
/// interval boundaries, the acknowledgement's structural pair and the combat
/// hit's range and kind boundaries.
const PLAYER_OUTCOMES_CASE_IDS: [&str; 32] = [
    "protocol.server.PlayerState/45/decode-valid",
    "protocol.server.PlayerState/45/encode-valid",
    "protocol.server.PlayerState/45/decode-active-mining",
    "protocol.server.PlayerState/45/decode-mining-combination-invalid",
    "protocol.server.PlayerState/45/decode-nan-position",
    "protocol.server.PlayerState/45/decode-health-above",
    "protocol.server.PlayerState/45/encode-health-above",
    "protocol.server.PlayerState/45/decode-oxygen-above",
    "protocol.server.PlayerState/45/decode-hunger-above",
    "protocol.server.PlayerState/45/decode-day-offset-above",
    "protocol.server.PlayerState/45/decode-weather-three",
    "protocol.server.PlayerState/45/decode-season-four",
    "protocol.server.PlayerState/45/decode-armor-above",
    "protocol.server.PlayerState/45/decode-trailing-byte",
    "protocol.server.CommandRejected/45/decode-valid",
    "protocol.server.CommandRejected/45/encode-valid",
    "protocol.server.CommandRejected/45/decode-reason-fifteen",
    "protocol.server.CommandRejected/45/decode-reason-zero",
    "protocol.server.CommandRejected/45/encode-reason-zero",
    "protocol.server.CommandRejected/45/decode-reason-sixteen",
    "protocol.server.PlaceBlockSucceeded/45/decode-valid",
    "protocol.server.PlaceBlockSucceeded/45/encode-valid",
    "protocol.server.PlaceBlockSucceeded/45/decode-truncated",
    "protocol.server.PlaceBlockSucceeded/45/decode-trailing-byte",
    "protocol.server.CombatHit/45/decode-valid",
    "protocol.server.CombatHit/45/encode-valid",
    "protocol.server.CombatHit/45/decode-kind-three",
    "protocol.server.CombatHit/45/decode-tick-zero",
    "protocol.server.CombatHit/45/decode-damage-zero",
    "protocol.server.CombatHit/45/decode-damage-above",
    "protocol.server.CombatHit/45/decode-kind-zero",
    "protocol.server.CombatHit/45/decode-kind-four",
];

/// Reports whether one family belongs to the player and private outcome
/// producer group.
fn is_player_outcomes_family(family: &str) -> bool {
    family == PLAYER_STATE_FAMILY
        || family == COMMAND_REJECTED_FAMILY
        || family == PLACE_BLOCK_SUCCEEDED_FAMILY
        || family == COMBAT_HIT_FAMILY
}

#[test]
fn protocol_corpus_packet_player_outcomes_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before that merge this test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let outcomes: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_player_outcomes_family(&case.family))
        .collect();
    let executed: Vec<&str> = outcomes.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = PLAYER_OUTCOMES_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the player and private outcome selection does not carry the reviewed case set"
    );
    assert!(
        !outcomes.is_empty(),
        "the player and private outcome selection executed zero cases"
    );
    for case in outcomes {
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

/// The case identities the inventory and container publication group
/// registers. They mirror the Go producer's registration, so a case that only
/// one side names is a mismatch rather than a shared name. The merged manifest
/// sorts case IDs, so the comparison sorts this list too.
const INVENTORY_PUBLICATION_CASE_IDS: [&str; 37] = [
    "protocol.server.InventoryState/45/decode-valid",
    "protocol.server.InventoryState/45/encode-valid",
    "protocol.server.InventoryState/45/decode-selected-nine",
    "protocol.server.InventoryState/45/encode-selected-nine",
    "protocol.server.InventoryState/45/decode-tool-at-durability-zero",
    "protocol.server.InventoryState/45/decode-trailing-byte",
    "protocol.server.InventoryState/45/decode-truncated",
    "protocol.server.CraftingState/45/decode-valid",
    "protocol.server.CraftingState/45/encode-valid",
    "protocol.server.CraftingState/45/decode-size-three",
    "protocol.server.CraftingState/45/decode-size-zero",
    "protocol.server.CraftingState/45/decode-size-four",
    "protocol.server.CraftingState/45/decode-personal-residue-slot-four",
    "protocol.server.CraftingState/45/encode-personal-residue-slot-four",
    "protocol.server.CraftingState/45/decode-trailing-byte",
    "protocol.server.FurnaceState/45/decode-valid",
    "protocol.server.FurnaceState/45/encode-valid",
    "protocol.server.FurnaceState/45/decode-progress-at-limit",
    "protocol.server.FurnaceState/45/decode-burn-above",
    "protocol.server.FurnaceState/45/decode-wrong-kind",
    "protocol.server.FurnaceState/45/decode-zero-generation",
    "protocol.server.FurnaceState/45/decode-invalid-fuel",
    "protocol.server.FurnaceState/45/decode-invalid-output",
    "protocol.server.FurnaceState/45/encode-invalid-fuel",
    "protocol.server.FurnaceState/45/decode-trailing-byte",
    "protocol.server.ChestState/45/decode-valid",
    "protocol.server.ChestState/45/encode-valid",
    "protocol.server.ChestState/45/decode-wrong-kind",
    "protocol.server.ChestState/45/decode-ref-slot-above",
    "protocol.server.ChestState/45/decode-invalid-stack",
    "protocol.server.ChestState/45/decode-truncated",
    "protocol.server.ChestState/45/decode-trailing-byte",
    "protocol.server.ContainerClosed/45/decode-valid",
    "protocol.server.ContainerClosed/45/encode-valid",
    "protocol.server.ContainerClosed/45/decode-exact-none",
    "protocol.server.ContainerClosed/45/decode-foreign-dimension",
    "protocol.server.ContainerClosed/45/decode-kind-two",
];

/// Reports whether one family belongs to the inventory and container
/// publication producer group.
fn is_inventory_publication_family(family: &str) -> bool {
    matches!(
        family,
        INVENTORY_STATE_FAMILY
            | CRAFTING_STATE_FAMILY
            | FURNACE_STATE_FAMILY
            | CHEST_STATE_FAMILY
            | CONTAINER_CLOSED_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_inventory_publication_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before this merge the test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let publications: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_inventory_publication_family(&case.family))
        .collect();
    let executed: Vec<&str> = publications.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = INVENTORY_PUBLICATION_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the inventory and container publication selection does not carry the reviewed case set"
    );
    assert!(
        !publications.is_empty(),
        "the inventory and container publication selection executed zero cases"
    );
    for case in publications {
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

/// The case identities the remote player producer group registers. They
/// mirror the Go producer's registration, so a case that only one side names
/// is a mismatch rather than a shared name. The merged manifest sorts case
/// IDs, so the comparison sorts this list too.
///
/// The count is the reviewed table's enumerated labels: the spawn's canonical
/// vector pair with its two name-bound refusals and the non-finite pitch the
/// Go float primitive answers before the validator, the despawn's pair with
/// its identity refusal and its trailing byte, and the batch's pair with the
/// two boundary-admitting counts, the count-bound refusals, the two order
/// refusals, the dimension, the non-finite coordinate and the trailing byte.
/// The spawn's zero and wrong-version identity and its unknown dimension are
/// deliberately absent: the Go validator folds them into one message, so they
/// are pinned as Rust group-test boundaries instead of corpus cases.
const REMOTE_PLAYERS_CASE_IDS: [&str; 20] = [
    "protocol.server.RemotePlayerSpawn/45/decode-valid",
    "protocol.server.RemotePlayerSpawn/45/encode-valid",
    "protocol.server.RemotePlayerSpawn/45/decode-padded-name",
    "protocol.server.RemotePlayerSpawn/45/encode-padded-name",
    "protocol.server.RemotePlayerSpawn/45/decode-nan-pitch",
    "protocol.server.RemotePlayerDespawn/45/decode-valid",
    "protocol.server.RemotePlayerDespawn/45/encode-valid",
    "protocol.server.RemotePlayerDespawn/45/decode-zero-uuid",
    "protocol.server.RemotePlayerDespawn/45/decode-trailing-byte",
    "protocol.server.RemotePlayerStates/45/decode-valid",
    "protocol.server.RemotePlayerStates/45/encode-valid",
    "protocol.server.RemotePlayerStates/45/decode-count-one",
    "protocol.server.RemotePlayerStates/45/decode-count-seven",
    "protocol.server.RemotePlayerStates/45/decode-count-zero",
    "protocol.server.RemotePlayerStates/45/decode-count-eight",
    "protocol.server.RemotePlayerStates/45/decode-duplicate-uuids",
    "protocol.server.RemotePlayerStates/45/decode-reversed-uuids",
    "protocol.server.RemotePlayerStates/45/decode-dimension-two",
    "protocol.server.RemotePlayerStates/45/decode-infinite-coordinate",
    "protocol.server.RemotePlayerStates/45/decode-trailing-byte",
];

/// Reports whether one family belongs to the remote player producer group.
fn is_remote_players_family(family: &str) -> bool {
    matches!(
        family,
        REMOTE_PLAYER_SPAWN_FAMILY | REMOTE_PLAYER_DESPAWN_FAMILY | REMOTE_PLAYER_STATES_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_remote_players_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before that merge the test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let remote_players: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_remote_players_family(&case.family))
        .collect();
    let executed: Vec<&str> = remote_players.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = REMOTE_PLAYERS_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the remote player selection does not carry the reviewed case set"
    );
    assert!(
        !remote_players.is_empty(),
        "the remote player selection executed zero cases"
    );
    for case in remote_players {
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

/// The case identities the companion producer group registers. They mirror
/// the Go producer's registration, so a case that only one side names is a
/// mismatch rather than a shared name. The merged manifest sorts case IDs, so
/// the comparison sorts this list too.
///
/// The count is the reviewed table's enumerated labels: the spawn's canonical
/// vector pair with its embedded-space name pair and its pitch and yaw
/// boundaries, the despawn's pair with its identity refusal and its fixed
/// ceiling, and the batch's canonical pair with the full four-record boundary,
/// the two count-bound refusals, the two order refusals, the pitch boundary
/// and the length refusal. The spawn's zero and wrong-version identity and its
/// depths dimension are deliberately absent: the Go validator folds them into
/// one message, so they are pinned as Rust group-test boundaries instead of
/// corpus cases.
const COMPANIONS_CASE_IDS: [&str; 19] = [
    "protocol.server.CompanionSpawn/45/decode-valid",
    "protocol.server.CompanionSpawn/45/encode-valid",
    "protocol.server.CompanionSpawn/45/decode-embedded-space-name",
    "protocol.server.CompanionSpawn/45/encode-embedded-space-name",
    "protocol.server.CompanionSpawn/45/decode-pitch-above-limit",
    "protocol.server.CompanionSpawn/45/decode-nan-yaw",
    "protocol.server.CompanionDespawn/45/decode-valid",
    "protocol.server.CompanionDespawn/45/encode-valid",
    "protocol.server.CompanionDespawn/45/decode-zero-id",
    "protocol.server.CompanionDespawn/45/decode-trailing-byte",
    "protocol.server.CompanionStates/45/decode-valid",
    "protocol.server.CompanionStates/45/encode-valid",
    "protocol.server.CompanionStates/45/decode-count-four",
    "protocol.server.CompanionStates/45/decode-count-zero",
    "protocol.server.CompanionStates/45/decode-count-five",
    "protocol.server.CompanionStates/45/decode-duplicate-ids",
    "protocol.server.CompanionStates/45/decode-reversed-ids",
    "protocol.server.CompanionStates/45/decode-pitch-above-limit",
    "protocol.server.CompanionStates/45/decode-trailing-byte",
];

/// Reports whether one family belongs to the companion producer group.
fn is_companions_family(family: &str) -> bool {
    matches!(
        family,
        COMPANION_SPAWN_FAMILY | COMPANION_DESPAWN_FAMILY | COMPANION_STATES_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_companions_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before this merge the test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let companions: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_companions_family(&case.family))
        .collect();
    let executed: Vec<&str> = companions.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = COMPANIONS_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the companion selection does not carry the reviewed case set"
    );
    assert!(
        !companions.is_empty(),
        "the companion selection executed zero cases"
    );
    for case in companions {
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

/// The case identities the item drop producer group registers. They mirror
/// the Go producer's registration, so a case that only one side names is a
/// mismatch rather than a shared name. The merged manifest sorts case IDs, so
/// the comparison sorts this list too.
///
/// The count is the reviewed table's enumerated labels: each family's
/// canonical vector pair, the upsert's block-index, identity and stack
/// boundaries with its encode twin, the upsert's two order refusals, the
/// remove batch's count boundaries and its two order refusals. The
/// unregistered item number is deliberately absent from the upsert table:
/// the Go validator folds it into the same stack message as the count
/// boundary, so a corpus case could not distinguish them and the Rust group
/// test pins it at its own variant instead.
const ITEM_DROPS_CASE_IDS: [&str; 19] = [
    "protocol.server.ItemDropUpserts/45/decode-valid",
    "protocol.server.ItemDropUpserts/45/encode-valid",
    "protocol.server.ItemDropUpserts/45/decode-block-index-above",
    "protocol.server.ItemDropUpserts/45/decode-generation-zero",
    "protocol.server.ItemDropUpserts/45/decode-slot-above",
    "protocol.server.ItemDropUpserts/45/decode-count-above-stack-limit",
    "protocol.server.ItemDropUpserts/45/decode-duplicate-ids",
    "protocol.server.ItemDropUpserts/45/decode-reversed-ids",
    "protocol.server.ItemDropUpserts/45/encode-block-index-above",
    "protocol.server.ItemDropUpserts/45/decode-trailing-byte",
    "protocol.server.ItemDropRemoves/45/decode-valid",
    "protocol.server.ItemDropRemoves/45/encode-valid",
    "protocol.server.ItemDropRemoves/45/decode-count-one",
    "protocol.server.ItemDropRemoves/45/decode-count-thirty-two",
    "protocol.server.ItemDropRemoves/45/decode-count-zero",
    "protocol.server.ItemDropRemoves/45/decode-count-thirty-three",
    "protocol.server.ItemDropRemoves/45/decode-duplicate-ids",
    "protocol.server.ItemDropRemoves/45/decode-reversed-ids",
    "protocol.server.ItemDropRemoves/45/decode-bad-generation",
];

/// Reports whether one family belongs to the item drop producer group.
fn is_item_drops_family(family: &str) -> bool {
    matches!(family, ITEM_DROP_UPSERTS_FAMILY | ITEM_DROP_REMOVES_FAMILY)
}

#[test]
fn protocol_corpus_packet_item_drops_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before this merge the test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let drops: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_item_drops_family(&case.family))
        .collect();
    let executed: Vec<&str> = drops.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = ITEM_DROPS_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the item drop selection does not carry the reviewed case set"
    );
    assert!(
        !drops.is_empty(),
        "the item drop selection executed zero cases"
    );
    for case in drops {
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

/// The case identities the hostile mob producer group registers. They mirror
/// the Go producer's registration, so a case that only one side names is a
/// mismatch rather than a shared name. The merged manifest sorts case IDs, so
/// the comparison sorts this list too.
///
/// The count is the reviewed table's enumerated labels: each family's canonical
/// vector pair, the spawn's identity, dimension, health and kind boundaries
/// with its encode twin, the spawn's nonfinite-pose and order refusals, the
/// state's identity, velocity, health and kind boundaries with its duplicate and
/// count refusals, the despawn's identity and order refusals, and one
/// full-record admit and one trailing-byte refusal per family.
const HOSTILES_CASE_IDS: [&str; 30] = [
    "protocol.server.HostileDespawn/45/decode-valid",
    "protocol.server.HostileDespawn/45/encode-valid",
    "protocol.server.HostileDespawn/45/decode-id-zero",
    "protocol.server.HostileDespawn/45/decode-duplicate-ids",
    "protocol.server.HostileDespawn/45/decode-reversed-ids",
    "protocol.server.HostileDespawn/45/decode-count-above",
    "protocol.server.HostileDespawn/45/decode-count-sixty-four",
    "protocol.server.HostileDespawn/45/decode-trailing-byte",
    "protocol.server.HostileSpawn/45/decode-valid",
    "protocol.server.HostileSpawn/45/encode-valid",
    "protocol.server.HostileSpawn/45/decode-count-sixty-four",
    "protocol.server.HostileSpawn/45/decode-id-zero",
    "protocol.server.HostileSpawn/45/decode-dimension-depths",
    "protocol.server.HostileSpawn/45/decode-health-zero",
    "protocol.server.HostileSpawn/45/decode-health-above",
    "protocol.server.HostileSpawn/45/decode-kind-two",
    "protocol.server.HostileSpawn/45/encode-kind-two",
    "protocol.server.HostileSpawn/45/decode-nan-position",
    "protocol.server.HostileSpawn/45/decode-reversed-ids",
    "protocol.server.HostileSpawn/45/decode-trailing-byte",
    "protocol.server.HostileState/45/decode-valid",
    "protocol.server.HostileState/45/encode-valid",
    "protocol.server.HostileState/45/decode-id-zero",
    "protocol.server.HostileState/45/decode-nan-velocity",
    "protocol.server.HostileState/45/decode-health-above",
    "protocol.server.HostileState/45/decode-kind-two",
    "protocol.server.HostileState/45/decode-duplicate-ids",
    "protocol.server.HostileState/45/decode-count-above",
    "protocol.server.HostileState/45/decode-count-sixty-four",
    "protocol.server.HostileState/45/decode-trailing-byte",
];

/// Reports whether one family belongs to the hostile mob producer group.
fn is_hostiles_family(family: &str) -> bool {
    matches!(
        family,
        HOSTILE_SPAWN_FAMILY | HOSTILE_STATE_FAMILY | HOSTILE_DESPAWN_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_hostiles_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before this merge the test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let hostiles: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_hostiles_family(&case.family))
        .collect();
    let executed: Vec<&str> = hostiles.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = HOSTILES_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the hostile selection does not carry the reviewed case set"
    );
    assert!(
        !hostiles.is_empty(),
        "the hostile selection executed zero cases"
    );
    for case in hostiles {
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

/// The case identities the passive mob producer group registers. They mirror
/// the Go producer's registration, so a case that only one side names is a
/// mismatch rather than a shared name. The merged manifest sorts case IDs, so
/// the comparison sorts this list too.
///
/// The count is the reviewed table's enumerated labels: each family's canonical
/// vector pair, the spawn's identity, dimension and two health boundaries with
/// its health-above encode twin, the spawn's nonfinite-pose and order refusals,
/// the state's grazing pair boundary with its grazing-two encode twin, the
/// state's nonfinite-velocity, identity, duplicate and count refusals, the
/// despawn's reason pair boundary with its reason-two encode twin, the
/// despawn's identity and order refusals, and one full-record admit and one
/// trailing-byte refusal per family. Neither family carries a kind-byte case,
/// because a passive mob has no category to publish.
const PASSIVES_CASE_IDS: [&str; 30] = [
    "protocol.server.PassiveDespawn/45/decode-valid",
    "protocol.server.PassiveDespawn/45/encode-valid",
    "protocol.server.PassiveDespawn/45/decode-reason-two",
    "protocol.server.PassiveDespawn/45/encode-reason-two",
    "protocol.server.PassiveDespawn/45/decode-id-zero",
    "protocol.server.PassiveDespawn/45/decode-duplicate-ids",
    "protocol.server.PassiveDespawn/45/decode-reversed-ids",
    "protocol.server.PassiveDespawn/45/decode-count-above",
    "protocol.server.PassiveDespawn/45/decode-trailing-byte",
    "protocol.server.PassiveSpawn/45/decode-valid",
    "protocol.server.PassiveSpawn/45/encode-valid",
    "protocol.server.PassiveSpawn/45/decode-count-sixty-four",
    "protocol.server.PassiveSpawn/45/decode-id-zero",
    "protocol.server.PassiveSpawn/45/decode-dimension-depths",
    "protocol.server.PassiveSpawn/45/decode-health-zero",
    "protocol.server.PassiveSpawn/45/decode-health-above",
    "protocol.server.PassiveSpawn/45/encode-health-above",
    "protocol.server.PassiveSpawn/45/decode-nan-position",
    "protocol.server.PassiveSpawn/45/decode-reversed-ids",
    "protocol.server.PassiveSpawn/45/decode-trailing-byte",
    "protocol.server.PassiveState/45/decode-valid",
    "protocol.server.PassiveState/45/encode-valid",
    "protocol.server.PassiveState/45/decode-grazing-two",
    "protocol.server.PassiveState/45/encode-grazing-two",
    "protocol.server.PassiveState/45/decode-nan-velocity",
    "protocol.server.PassiveState/45/decode-id-zero",
    "protocol.server.PassiveState/45/decode-duplicate-ids",
    "protocol.server.PassiveState/45/decode-count-above",
    "protocol.server.PassiveState/45/decode-count-sixty-four",
    "protocol.server.PassiveState/45/decode-trailing-byte",
];

/// Reports whether one family belongs to the passive mob producer group.
fn is_passives_family(family: &str) -> bool {
    matches!(
        family,
        PASSIVE_SPAWN_FAMILY | PASSIVE_STATE_FAMILY | PASSIVE_DESPAWN_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_passives_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before this merge the test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let passives: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_passives_family(&case.family))
        .collect();
    let executed: Vec<&str> = passives.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = PASSIVES_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the passive selection does not carry the reviewed case set"
    );
    assert!(
        !passives.is_empty(),
        "the passive selection executed zero cases"
    );
    for case in passives {
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

/// The case identities the projectile producer group registers. They mirror
/// the Go producer's registration, so a case that only one side names is a
/// mismatch rather than a shared name. The merged manifest sorts case IDs, so
/// the comparison sorts this list too.
///
/// The count is the reviewed table's enumerated labels: each family's canonical
/// vector pair, one full-record ceiling admit per family, the spawn's identity,
/// kind and dimension boundaries with its kind encode twin, the spawn's
/// nonfinite-pose and order refusals, the state's identity, nonfinite, order
/// and count refusals with its nonfinite encode twin, the despawn's identity
/// and order refusals, the count-bound refusal per family, the padding refusal
/// the exact-length rule answers at the truncation category, and the
/// despawn's over-ceiling refusal the pre-parse wire ceiling answers at the
/// capacity category.
const PROJECTILES_CASE_IDS: [&str; 29] = [
    "protocol.server.ProjectileDespawn/45/decode-valid",
    "protocol.server.ProjectileDespawn/45/encode-valid",
    "protocol.server.ProjectileDespawn/45/decode-id-zero",
    "protocol.server.ProjectileDespawn/45/decode-duplicate-ids",
    "protocol.server.ProjectileDespawn/45/decode-reversed-ids",
    "protocol.server.ProjectileDespawn/45/decode-count-above",
    "protocol.server.ProjectileDespawn/45/decode-count-one-hundred-twenty-eight",
    "protocol.server.ProjectileDespawn/45/decode-trailing-byte",
    "protocol.server.ProjectileDespawn/45/decode-over-ceiling",
    "protocol.server.ProjectileSpawn/45/decode-valid",
    "protocol.server.ProjectileSpawn/45/encode-valid",
    "protocol.server.ProjectileSpawn/45/decode-count-one-hundred-twenty-eight",
    "protocol.server.ProjectileSpawn/45/decode-id-zero",
    "protocol.server.ProjectileSpawn/45/decode-kind-two",
    "protocol.server.ProjectileSpawn/45/encode-kind-two",
    "protocol.server.ProjectileSpawn/45/decode-dimension-two",
    "protocol.server.ProjectileSpawn/45/decode-nan-position",
    "protocol.server.ProjectileSpawn/45/decode-reversed-ids",
    "protocol.server.ProjectileSpawn/45/decode-count-above",
    "protocol.server.ProjectileSpawn/45/decode-trailing-byte",
    "protocol.server.ProjectileState/45/decode-valid",
    "protocol.server.ProjectileState/45/encode-valid",
    "protocol.server.ProjectileState/45/decode-id-zero",
    "protocol.server.ProjectileState/45/decode-nan-position",
    "protocol.server.ProjectileState/45/encode-nan-position",
    "protocol.server.ProjectileState/45/decode-duplicate-ids",
    "protocol.server.ProjectileState/45/decode-count-above",
    "protocol.server.ProjectileState/45/decode-count-one-hundred-twenty-eight",
    "protocol.server.ProjectileState/45/decode-trailing-byte",
];

/// Renders the semantic fields one chat event publishes, shared by the decode
/// and encode arms so both publish the same canonical field encoding.
///
/// The event identity renders as a decimal string so the full `u64` range stays
/// lossless, both identities render as 32-lowercase-hexadecimal text, and the
/// names, command and speech render verbatim. The companion identity stays the
/// raw wire form: the two rejection branches that never addressed a companion
/// carry the exact zero bytes, which is the wire-only absent form the domain
/// union cannot express as an identity, so publishing it raw keeps the
/// distinction observable instead of pre-validating it away. The kind and the
/// reason render as the plain integers the wire carries, and the one text slot
/// the kind selected is published beside the empty one, because the wire
/// carries exactly one slot and both fields are part of the reviewed record.
fn chat_event_fields(event: &mornlea_protocol::ChatEvent) -> serde_json::Value {
    serde_json::json!({
        "event_id": event.event_id.to_string(),
        "player_id": hex_lower(&event.player_id.bytes()),
        "player_name": event.player_name,
        "companion_id": hex_lower(&event.companion_id),
        "companion_name": event.companion_name,
        "kind": event.kind,
        "reason": event.reject_reason,
        "command": event.command,
        "speech": event.speech
    })
}

/// Builds the chat event one encode case names from its typed fields.
///
/// The record is built through its public fields, so a mutated or invalid case
/// is refused by the production validation rather than by a constructor guard.
/// The kind decides which text slot the record carries, exactly as the wire
/// does, and the companion identity is read as the raw hexadecimal text the
/// case publishes so the absent zero form stays constructible for the two
/// branches that carry it.
fn chat_event_request(
    case: &FrozenCase,
) -> Result<mornlea_protocol::ChatEvent, mornlea_protocol::ProtocolError> {
    let identity =
        |name: &str| -> Result<mornlea_protocol::PlayerId, mornlea_protocol::ProtocolError> {
            let text = case
                .input_json
                .as_ref()
                .expect("encode case carries JSON fields")
                .get(name)
                .and_then(|value| value.as_str())
                .unwrap_or_else(|| panic!("case {} names no {name}", case.id));
            let bytes: [u8; 16] = payload_bytes_from_text(case, text)
                .try_into()
                .unwrap_or_else(|_| panic!("case {} field {name} is not 16 bytes", case.id));
            mornlea_protocol::PlayerId::try_from_bytes(bytes)
                .map_err(|_| mornlea_protocol::ProtocolError::InvalidIdentity)
        };
    let companion = case
        .input_json
        .as_ref()
        .expect("encode case carries JSON fields")
        .get("companion_id")
        .and_then(|value| value.as_str())
        .unwrap_or_else(|| panic!("case {} names no companion_id", case.id));
    let companion_id: [u8; 16] = payload_bytes_from_text(case, companion)
        .try_into()
        .unwrap_or_else(|_| panic!("case {} companion_id is not 16 bytes", case.id));
    Ok(mornlea_protocol::ChatEvent {
        event_id: unsigned_field(case, "event_id"),
        player_id: identity("player_id")?,
        player_name: text_field(case, "player_name"),
        companion_id,
        companion_name: text_field(case, "companion_name"),
        kind: byte_field(case, "kind"),
        reject_reason: byte_field(case, "reason"),
        command: text_field(case, "command"),
        speech: text_field(case, "speech"),
    })
}

/// Reports whether one family belongs to the projectile producer group.
fn is_projectiles_family(family: &str) -> bool {
    matches!(
        family,
        PROJECTILE_SPAWN_FAMILY | PROJECTILE_STATE_FAMILY | PROJECTILE_DESPAWN_FAMILY
    )
}

#[test]
fn protocol_corpus_packet_projectiles_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before this merge the test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let projectiles: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_projectiles_family(&case.family))
        .collect();
    let executed: Vec<&str> = projectiles.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = PROJECTILES_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the projectile selection does not carry the reviewed case set"
    );
    assert!(
        !projectiles.is_empty(),
        "the projectile selection executed zero cases"
    );
    for case in projectiles {
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

/// The case identities the chat event producer group registers. They mirror the
/// Go producer's registration, so a case that only one side names is a mismatch
/// rather than a shared name.
const CHAT_EVENT_CASE_IDS: [&str; 24] = [
    "protocol.server.ChatEvent/45/decode-accepted",
    "protocol.server.ChatEvent/45/decode-invalid-format",
    "protocol.server.ChatEvent/45/decode-unknown-companion",
    "protocol.server.ChatEvent/45/decode-queue-full",
    "protocol.server.ChatEvent/45/decode-not-following",
    "protocol.server.ChatEvent/45/decode-task-started",
    "protocol.server.ChatEvent/45/decode-task-progress",
    "protocol.server.ChatEvent/45/decode-task-completed",
    "protocol.server.ChatEvent/45/decode-task-failed",
    "protocol.server.ChatEvent/45/decode-task-timed-out",
    "protocol.server.ChatEvent/45/decode-task-stopped",
    "protocol.server.ChatEvent/45/decode-speech",
    "protocol.server.ChatEvent/45/encode-accepted",
    "protocol.server.ChatEvent/45/encode-speech",
    "protocol.server.ChatEvent/45/decode-reserved-reason-three",
    "protocol.server.ChatEvent/45/decode-failed-reason-zero",
    "protocol.server.ChatEvent/45/decode-non-speech-with-speech",
    "protocol.server.ChatEvent/45/decode-speech-with-command",
    "protocol.server.ChatEvent/45/decode-zero-player-uuid",
    "protocol.server.ChatEvent/45/decode-noncanonical-player-name",
    "protocol.server.ChatEvent/45/encode-command-above-bound",
    "protocol.server.ChatEvent/45/decode-speech-above-bound",
    "protocol.server.ChatEvent/45/decode-zero-event-id",
    "protocol.server.ChatEvent/45/decode-trailing-byte",
];

/// Reports whether one family belongs to the chat event producer group.
fn is_chat_event_family(family: &str) -> bool {
    family == CHAT_EVENT_FAMILY
}

#[test]
fn protocol_corpus_packet_chat_event_cases_are_executed() {
    // The case assets are exported by the Go producer and integrated by the
    // controller, so before this merge the test reports the missing corpus
    // cases instead of an empty selection that would look like a passing run.
    let cases = load_cases_for_consumer(CorpusConsumer::Protocol);
    let chat_event: Vec<&FrozenCase> = cases
        .iter()
        .filter(|case| is_chat_event_family(&case.family))
        .collect();
    let executed: Vec<&str> = chat_event.iter().map(|case| case.id.as_str()).collect();
    let mut expected: Vec<&str> = CHAT_EVENT_CASE_IDS.to_vec();
    // The merged manifest sorts case IDs; compare as the reviewed set, not in
    // the authoring order of this suite's constant.
    expected.sort_unstable();
    assert_eq!(
        executed, expected,
        "the chat event selection does not carry the reviewed case set"
    );
    assert!(
        !chat_event.is_empty(),
        "the chat event selection executed zero cases"
    );
    for case in chat_event {
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
