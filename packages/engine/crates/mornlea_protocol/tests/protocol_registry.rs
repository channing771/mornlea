//! The closed v45 packet registry: typed dispatch over exactly the 59 Go
//! packet keys, and the refusals that keep every other key out.
//!
//! `mornlea_protocol::registry` is the crate's public dispatch surface:
//! [`Direction`], [`State`] and [`PacketKey`] form the three-part key, and
//! [`ClientPacket`] / [`ServerPacket`] are exhaustive enums over the 23 client
//! and 36 server keys the Go `registry.go` table freezes. This suite is the
//! evidence for that closure.
//!
//! The dispatch table carries one reviewed Go-produced payload per key. Every
//! payload is the `.input.bin` of the corresponding family's valid corpus
//! decode case, so the bytes are the Go encoder's output rather than a Rust
//! re-encoding, and the round-trip assertion is byte-level parity instead of
//! self-agreement. Each key's test decodes through the real boundary, checks
//! the dispatched key and variant, re-encodes and compares, refuses a
//! destination one byte short, and then mutates only the direction and the
//! state to prove the dispatcher answers the requested key and nothing else.
//!
//! The one family whose re-encoding is not byte-identical is the compressed
//! snapshot: the Go and Rust zstd encoders legitimately publish different
//! compressed blocks, so that key's acceptance is a semantic round trip
//! through the owned context rather than a byte comparison.
//!
//! The second table is the test-owned expectation of the key map, written
//! independently of the dispatch table. The comparison between them fails by
//! key and variant rather than by count, and three tamper cases pin that:
//! dropping a row, duplicating a complete key, and swapping two server IDs.

use mornlea_protocol::{
    ClientPacket, Direction, HandshakeRejection, MAX_COMPRESSED_SNAPSHOT, MAX_SMALL_PAYLOAD_BYTES,
    PacketKey, ProtocolCodec, ProtocolError, SNAPSHOT_ENVELOPE_LENGTH, ServerPacket, State,
    decode_client, encode_client_into, validate_hello,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// How one key's re-encoded payload is compared against the reviewed literal.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum Reencode {
    /// The re-encoded payload must equal the Go literal byte for byte.
    Bytes,
    /// The re-encoded payload must decode back to the same record, which is
    /// the acceptance rule for the one compressed family.
    Logical,
}

/// One registered key beside the reviewed Go payload it dispatches.
struct RegistryCase {
    /// The test function that owns this row.
    name: &'static str,
    /// The complete three-part key the row registers.
    key: PacketKey,
    /// The reviewed Go-produced payload for that key.
    payload: &'static [u8],
    /// The enum variant the dispatcher must produce.
    variant: &'static str,
    /// How the re-encoded payload is compared.
    reencode: Reencode,
}

/// Looks up one row of the dispatch table by the test that owns it.
fn case(name: &str) -> &'static RegistryCase {
    CASES
        .iter()
        .find(|candidate| candidate.name == name)
        .unwrap_or_else(|| panic!("the dispatch table carries no row for {name}"))
}

/// Reports whether a complete key is one of the registered keys.
fn is_registered(key: &PacketKey) -> bool {
    CASES.iter().any(|candidate| candidate.key == *key)
}

/// The three keys that differ from `key` in direction or state only.
///
/// The numeric ID is held fixed, because the ID is the part that collides
/// across direction and state by design; mutating anything else would not be a
/// direction-or-state mutation.
fn mutated_keys(key: PacketKey) -> Vec<PacketKey> {
    let mut keys = Vec::new();
    for direction in [Direction::ClientToServer, Direction::ServerToClient] {
        for state in [State::Handshake, State::Login, State::Play] {
            let mutated = PacketKey {
                direction,
                state,
                id: key.id,
            };
            if mutated != key {
                keys.push(mutated);
            }
        }
    }
    keys
}

/// The variant name of one dispatched client packet.
///
/// The match is exhaustive and names every variant, so a variant added to the
/// enum without a dispatch row fails here rather than being silently unnamed.
fn client_variant(packet: &ClientPacket) -> &'static str {
    match packet {
        ClientPacket::ClientHello(_) => "ClientHello",
        ClientPacket::LoginStart(_) => "LoginStart",
        ClientPacket::PlayerInput(_) => "PlayerInput",
        ClientPacket::PlaceBlock(_) => "PlaceBlock",
        ClientPacket::RequestChunkResync(_) => "RequestChunkResync",
        ClientPacket::KeepAliveReply(_) => "KeepAliveReply",
        ClientPacket::SelectHotbar(_) => "SelectHotbar",
        ClientPacket::MoveInventoryStack(_) => "MoveInventoryStack",
        ClientPacket::MoveCraftingStack(_) => "MoveCraftingStack",
        ClientPacket::OpenContainer(_) => "OpenContainer",
        ClientPacket::MoveContainerStack(_) => "MoveContainerStack",
        ClientPacket::CloseContainer(_) => "CloseContainer",
        ClientPacket::DropSelectedItem(_) => "DropSelectedItem",
        ClientPacket::ChatCommand(_) => "ChatCommand",
        ClientPacket::TillSoil(_) => "TillSoil",
        ClientPacket::BoneMeal(_) => "BoneMeal",
        ClientPacket::TakeCraftingOutput(_) => "TakeCraftingOutput",
        ClientPacket::CollectWater(_) => "CollectWater",
        ClientPacket::PlaceWater(_) => "PlaceWater",
        ClientPacket::EquipArmor(_) => "EquipArmor",
        ClientPacket::MoveStackPartial(_) => "MoveStackPartial",
        ClientPacket::QuickMoveStack(_) => "QuickMoveStack",
        ClientPacket::DropStack(_) => "DropStack",
    }
}

/// The variant name of one dispatched server packet.
fn server_variant(packet: &ServerPacket) -> &'static str {
    match packet {
        ServerPacket::ServerHello(_) => "ServerHello",
        ServerPacket::HandshakeReject(_) => "HandshakeReject",
        ServerPacket::LoginSuccess(_) => "LoginSuccess",
        ServerPacket::LoginReject(_) => "LoginReject",
        ServerPacket::ChunkSnapshot(_) => "ChunkSnapshot",
        ServerPacket::BlockChanges(_) => "BlockChanges",
        ServerPacket::ForgetChunks(_) => "ForgetChunks",
        ServerPacket::PlayerState(_) => "PlayerState",
        ServerPacket::CommandRejected(_) => "CommandRejected",
        ServerPacket::KeepAlive(_) => "KeepAlive",
        ServerPacket::Disconnect(_) => "Disconnect",
        ServerPacket::RemotePlayerSpawn(_) => "RemotePlayerSpawn",
        ServerPacket::RemotePlayerDespawn(_) => "RemotePlayerDespawn",
        ServerPacket::RemotePlayerStates(_) => "RemotePlayerStates",
        ServerPacket::InventoryState(_) => "InventoryState",
        ServerPacket::ItemDropUpserts(_) => "ItemDropUpserts",
        ServerPacket::ItemDropRemoves(_) => "ItemDropRemoves",
        ServerPacket::FurnaceState(_) => "FurnaceState",
        ServerPacket::ContainerClosed(_) => "ContainerClosed",
        ServerPacket::ChestState(_) => "ChestState",
        ServerPacket::ChatEvent(_) => "ChatEvent",
        ServerPacket::CompanionSpawn(_) => "CompanionSpawn",
        ServerPacket::CompanionStates(_) => "CompanionStates",
        ServerPacket::CompanionDespawn(_) => "CompanionDespawn",
        ServerPacket::PlaceBlockSucceeded(_) => "PlaceBlockSucceeded",
        ServerPacket::CraftingState(_) => "CraftingState",
        ServerPacket::HostileSpawn(_) => "HostileSpawn",
        ServerPacket::HostileState(_) => "HostileState",
        ServerPacket::HostileDespawn(_) => "HostileDespawn",
        ServerPacket::CombatHit(_) => "CombatHit",
        ServerPacket::PassiveSpawn(_) => "PassiveSpawn",
        ServerPacket::PassiveState(_) => "PassiveState",
        ServerPacket::PassiveDespawn(_) => "PassiveDespawn",
        ServerPacket::ProjectileSpawn(_) => "ProjectileSpawn",
        ServerPacket::ProjectileState(_) => "ProjectileState",
        ServerPacket::ProjectileDespawn(_) => "ProjectileDespawn",
    }
}

/// Refuses a destination one byte short of the validated record.
///
/// Every family publishes through the crate-wide capacity check, so the
/// refusal is `OutputTooSmall` with the exact needed and available counts and
/// every caller byte left at the sentinel.
fn assert_short_destination_is_refused(
    row: &RegistryCase,
    needed: usize,
    encode: impl FnOnce(&mut [u8]) -> Result<usize, ProtocolError>,
) {
    assert!(
        needed > 0,
        "{}: every packet publishes at least one byte",
        row.name
    );
    let mut short = vec![SENTINEL; needed - 1];
    let error = match encode(&mut short) {
        Ok(written) => panic!(
            "{}: a destination one byte short published {written} bytes",
            row.name
        ),
        Err(error) => error,
    };
    assert_eq!(
        error,
        ProtocolError::OutputTooSmall {
            needed,
            available: needed - 1,
        },
        "{}: a short destination must report the exact capacity refusal",
        row.name
    );
    assert!(
        short.iter().all(|byte| *byte == SENTINEL),
        "{}: a short destination must leave every caller byte unchanged",
        row.name
    );
}

/// Requires that no key the registry does not publish, and no payload a
/// published key cannot carry, produces a packet.
///
/// The invariant is stated over the requested key rather than over the
/// original one: a mutated key that is itself registered may legitimately
/// dispatch — C/Handshake/0 and S/Handshake/0 share one payload shape and one
/// numeric ID — but then it must answer under the mutated key, never under the
/// key the payload was reviewed for. A mutated key that is not registered must
/// be refused with exactly `UnknownPacket`.
fn assert_unreachable(key: PacketKey, payload: &[u8]) {
    for mutated in mutated_keys(key) {
        let outcome = match mutated.direction {
            Direction::ClientToServer => {
                decode_client(mutated.state, mutated.id, payload).map(|packet| packet.key())
            }
            Direction::ServerToClient => {
                let mut codec = ProtocolCodec::new().expect("one owned snapshot context");
                codec
                    .decode_server(mutated.state, mutated.id, payload)
                    .map(|packet| packet.key())
            }
        };
        match outcome {
            Ok(dispatched) => assert_eq!(
                dispatched, mutated,
                "{key:?} mutated into {mutated:?} dispatched {dispatched:?}: \
                 the dispatcher must answer the requested key"
            ),
            Err(error) => {
                if !is_registered(&mutated) {
                    assert_eq!(
                        error,
                        ProtocolError::UnknownPacket,
                        "{mutated:?} is not one of the registered keys, so the refusal \
                         must name the key rather than a field"
                    );
                }
            }
        }
    }
}

/// Decodes, re-encodes and checks one registered client key.
fn dispatch_client_case(row: &RegistryCase) {
    let packet = decode_client(row.key.state, row.key.id, row.payload).unwrap_or_else(|error| {
        panic!(
            "{}: the dispatcher refused a reviewed Go payload: {error:?}",
            row.name
        )
    });
    assert_eq!(
        packet.key(),
        row.key,
        "{}: the dispatched key is not the registry key",
        row.name
    );
    assert_eq!(
        client_variant(&packet),
        row.variant,
        "{}: the dispatched variant is not the mapped variant",
        row.name
    );

    let mut dst = vec![SENTINEL; row.payload.len()];
    let written = encode_client_into(&packet, &mut dst).unwrap_or_else(|error| {
        panic!(
            "{}: the encoder refused a dispatched packet: {error:?}",
            row.name
        )
    });
    assert_eq!(
        written,
        row.payload.len(),
        "{}: the encoded length is not the reviewed payload length",
        row.name
    );
    assert_eq!(
        dst, row.payload,
        "{}: the re-encoded payload is not the reviewed Go literal",
        row.name
    );
    assert_short_destination_is_refused(row, written, |dst| encode_client_into(&packet, dst));
    assert_unreachable(row.key, row.payload);
}

/// Decodes, re-encodes and checks one registered server key.
///
/// The destination is sized by the compressed ceiling for the one family whose
/// payload length is not precomputed and by the reviewed length for every
/// other family, so the buffer is never sized by a guess.
fn dispatch_server_case(row: &RegistryCase, codec: &mut ProtocolCodec) {
    let packet = codec
        .decode_server(row.key.state, row.key.id, row.payload)
        .unwrap_or_else(|error| {
            panic!(
                "{}: the dispatcher refused a reviewed Go payload: {error:?}",
                row.name
            )
        });
    assert_eq!(
        packet.key(),
        row.key,
        "{}: the dispatched key is not the registry key",
        row.name
    );
    assert_eq!(
        server_variant(&packet),
        row.variant,
        "{}: the dispatched variant is not the mapped variant",
        row.name
    );

    let capacity = if row.reencode == Reencode::Logical {
        MAX_COMPRESSED_SNAPSHOT + SNAPSHOT_ENVELOPE_LENGTH
    } else {
        row.payload.len()
    };
    let mut dst = vec![SENTINEL; capacity];
    let written = codec
        .encode_server_into(&packet, &mut dst)
        .unwrap_or_else(|error| {
            panic!(
                "{}: the encoder refused a dispatched packet: {error:?}",
                row.name
            )
        });
    dst.truncate(written);
    match row.reencode {
        Reencode::Bytes => assert_eq!(
            dst, row.payload,
            "{}: the re-encoded payload is not the reviewed Go literal",
            row.name
        ),
        Reencode::Logical => {
            let again = codec
                .decode_server(row.key.state, row.key.id, &dst)
                .unwrap_or_else(|error| {
                    panic!(
                        "{}: the re-encoded snapshot did not decode: {error:?}",
                        row.name
                    )
                });
            assert_eq!(
                again, packet,
                "{}: the re-encoded snapshot does not decode to the same record",
                row.name
            );
            assert_eq!(
                server_variant(&again),
                row.variant,
                "{}: the re-decoded snapshot is not the mapped variant",
                row.name
            );
        }
    }
    assert_short_destination_is_refused(row, written, |dst| codec.encode_server_into(&packet, dst));
    assert_unreachable(row.key, row.payload);
}

/// Runs one dispatch-table row end to end.
fn dispatch_case(row: &RegistryCase) {
    if row.key.direction == Direction::ServerToClient {
        let mut codec = ProtocolCodec::new().expect("one owned snapshot context");
        dispatch_server_case(row, &mut codec);
    } else {
        dispatch_client_case(row);
    }
}

/// The reviewed 1-byte ClientHello payload: the Go encoder's output for the
/// `decode-current-version` corpus case.
const CLIENT_HELLO_WIRE: [u8; 1] = [0x2d];

/// The reviewed 23-byte LoginStart payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const LOGIN_START_WIRE: [u8; 23] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
    0x05, 0x41, 0x6c, 0x69, 0x63, 0x65, 0x02,
];

/// The reviewed 23-byte PlayerInput payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PLAYER_INPUT_WIRE: [u8; 23] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x7f, 0x01, 0x00, 0x00, 0x00, 0x80, 0x00,
    0x00, 0x00, 0x40, 0x01, 0x00, 0x01, 0x00,
];

/// The reviewed 17-byte PlaceBlock payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PLACE_BLOCK_WIRE: [u8; 17] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xa0, 0x3f, 0x00, 0x00, 0x00, 0x80,
    0x08,
];

/// The reviewed 28-byte RequestChunkResync payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const REQUEST_CHUNK_RESYNC_WIRE: [u8; 28] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
];

/// The reviewed 8-byte KeepAliveReply payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const KEEP_ALIVE_REPLY_WIRE: [u8; 8] = [0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];

/// The reviewed 9-byte SelectHotbar payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const SELECT_HOTBAR_WIRE: [u8; 9] = [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08];

/// The reviewed 10-byte MoveInventoryStack payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const MOVE_INVENTORY_STACK_WIRE: [u8; 10] =
    [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x23];

/// The reviewed 10-byte MoveCraftingStack payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const MOVE_CRAFTING_STACK_WIRE: [u8; 10] =
    [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x2c];

/// The reviewed 16-byte OpenContainer payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const OPEN_CONTAINER_WIRE: [u8; 16] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0xc0, 0x3f,
];

/// The reviewed 28-byte MoveContainerStack payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const MOVE_CONTAINER_STACK_WIRE: [u8; 28] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff,
    0x02, 0x00, 0x00, 0x00, 0x00, 0x1f, 0x01, 0x00, 0x00, 0x00, 0x00, 0x25,
];

/// The reviewed 8-byte CloseContainer payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const CLOSE_CONTAINER_WIRE: [u8; 8] = [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];

/// The reviewed 8-byte DropSelectedItem payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const DROP_SELECTED_ITEM_WIRE: [u8; 8] = [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];

/// The reviewed 13-byte ChatCommand payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const CHAT_COMMAND_WIRE: [u8; 13] = [
    0x0c, 0x40, 0x6d, 0x69, 0x72, 0x61, 0x20, 0x66, 0x6f, 0x6c, 0x6c, 0x6f, 0x77,
];

/// The reviewed 16-byte TillSoil payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const TILL_SOIL_WIRE: [u8; 16] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0xc0, 0x3f,
];

/// The reviewed 16-byte BoneMeal payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const BONE_MEAL_WIRE: [u8; 16] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0xc0, 0x3f,
];

/// The reviewed 8-byte TakeCraftingOutput payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const TAKE_CRAFTING_OUTPUT_WIRE: [u8; 8] = [0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];

/// The reviewed 16-byte CollectWater payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const COLLECT_WATER_WIRE: [u8; 16] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0xc0, 0x3f,
];

/// The reviewed 16-byte PlaceWater payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PLACE_WATER_WIRE: [u8; 16] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0xc0, 0x3f,
];

/// The reviewed 8-byte EquipArmor payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const EQUIP_ARMOR_WIRE: [u8; 8] = [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];

/// The reviewed 30-byte MoveStackPartial payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const MOVE_STACK_PARTIAL_WIRE: [u8; 30] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff,
    0x02, 0x00, 0x00, 0x00, 0x01, 0x0f, 0x01, 0x00, 0x00, 0x00, 0x02, 0x24, 0x3e, 0x01,
];

/// The reviewed 28-byte QuickMoveStack payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const QUICK_MOVE_STACK_WIRE: [u8; 28] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x2c,
];

/// The reviewed 28-byte DropStack payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const DROP_STACK_WIRE: [u8; 28] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x23,
];

/// The reviewed 1-byte ServerHello payload: the Go encoder's output for the
/// `decode-current-version` corpus case.
const SERVER_HELLO_WIRE: [u8; 1] = [0x2d];

/// The reviewed 3-byte HandshakeReject payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const HANDSHAKE_REJECT_WIRE: [u8; 3] = [0x2a, 0x01, 0x00];

/// The reviewed 24-byte LoginSuccess payload: the Go encoder's output for the
/// `decode-zero-seed` corpus case.
const LOGIN_SUCCESS_WIRE: [u8; 24] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
];

/// The reviewed 2-byte LoginReject payload: the Go encoder's output for the
/// `decode-code-one-empty-message` corpus case.
const LOGIN_REJECT_WIRE: [u8; 2] = [0x01, 0x00];

/// The reviewed 440-byte ChunkSnapshot payload: the Go encoder's output for the
/// `decode-valid-mixed` corpus case.
const CHUNK_SNAPSHOT_WIRE: [u8; 440] = [
    0xe3, 0x54, 0x01, 0x00, 0xb0, 0x01, 0x00, 0x00, 0x28, 0xb5, 0x2f, 0xfd, 0xa4, 0xe3, 0x54, 0x01,
    0x00, 0x05, 0x0d, 0x00, 0x12, 0x56, 0x48, 0x34, 0x40, 0x89, 0xa1, 0x00, 0x3c, 0x30, 0x0f, 0xcc,
    0x03, 0xf3, 0xc0, 0x3c, 0x30, 0x0f, 0xcc, 0x7a, 0x22, 0xb6, 0x08, 0x68, 0x8b, 0xd8, 0x22, 0xb6,
    0x88, 0x2d, 0x62, 0x2b, 0xdd, 0xa0, 0x4a, 0x75, 0x83, 0x2a, 0xd5, 0x0d, 0xaa, 0x54, 0x37, 0xa8,
    0x52, 0xdd, 0x08, 0xf1, 0x21, 0xbd, 0xf1, 0x48, 0xfb, 0xff, 0xfb, 0x1e, 0xa3, 0x98, 0x8b, 0xa5,
    0x42, 0xf9, 0xd0, 0xc1, 0x55, 0x7b, 0xaa, 0x29, 0x39, 0xf9, 0x49, 0x7c, 0xb7, 0x97, 0xf2, 0x22,
    0x10, 0xd4, 0xd1, 0x38, 0x19, 0x0b, 0x85, 0x59, 0x92, 0x76, 0x89, 0x08, 0x88, 0x07, 0x87, 0x06,
    0x86, 0x05, 0x85, 0x04, 0x84, 0x03, 0x83, 0x02, 0x82, 0x01, 0x81, 0x00, 0x80, 0xa3, 0xb1, 0x48,
    0x1c, 0x0a, 0x83, 0xc0, 0x9f, 0xaf, 0xc7, 0xdb, 0xe9, 0x72, 0xb8, 0x9b, 0xad, 0x46, 0x9b, 0xc9,
    0x62, 0xb0, 0x97, 0xab, 0xc5, 0x5a, 0xa9, 0x52, 0xa8, 0x93, 0xa9, 0x44, 0x1a, 0x89, 0x42, 0xa0,
    0x8f, 0xa7, 0xc3, 0xd9, 0x68, 0x32, 0x98, 0x8b, 0xa5, 0x42, 0x99, 0x48, 0x6e, 0x1a, 0x81, 0x3c,
    0x4c, 0x75, 0x89, 0x23, 0x86, 0xf8, 0x61, 0x87, 0x1b, 0x66, 0x78, 0x61, 0x85, 0x13, 0x46, 0xf8,
    0x60, 0x83, 0x0b, 0x26, 0x78, 0x60, 0x81, 0x03, 0x06, 0xf8, 0xb8, 0xf1, 0xe2, 0xc4, 0x87, 0x0b,
    0x0f, 0x0e, 0xfc, 0xb7, 0xef, 0xde, 0xbc, 0x77, 0xeb, 0xce, 0x8d, 0xfb, 0xb6, 0xed, 0xda, 0xb4,
    0x67, 0xcb, 0x8e, 0x0d, 0xfb, 0xb5, 0xeb, 0xd6, 0xac, 0x57, 0xab, 0x4e, 0x8d, 0xfa, 0xb4, 0xe9,
    0xd2, 0xa4, 0x47, 0x8b, 0x0e, 0x0d, 0xfa, 0xb3, 0xe7, 0xce, 0x9c, 0x37, 0x6b, 0xce, 0x8c, 0xf9,
    0xb2, 0xe5, 0xca, 0x94, 0x27, 0x4b, 0xbe, 0xa7, 0x47, 0x86, 0xfc, 0x18, 0xdd, 0x71, 0xe3, 0x73,
    0xc6, 0x8b, 0xf2, 0x89, 0xd3, 0x3e, 0xac, 0x45, 0xef, 0x75, 0x58, 0xdb, 0xb8, 0xf7, 0xb4, 0x8c,
    0x4b, 0xaa, 0x93, 0x5b, 0xd2, 0xb4, 0x8b, 0x15, 0x2b, 0x36, 0x37, 0x2f, 0x2f, 0x27, 0x27, 0xb7,
    0x99, 0xed, 0xff, 0xba, 0xae, 0xeb, 0x6f, 0x03, 0x21, 0x00, 0xff, 0x0f, 0x7a, 0x09, 0x00, 0x80,
    0xbf, 0x00, 0x00, 0x78, 0x16, 0x00, 0x00, 0x2f, 0x01, 0x00, 0x01, 0x54, 0xfe, 0xaa, 0xff, 0x95,
    0x5e, 0xf9, 0x0f, 0x7c, 0x60, 0xeb, 0x85, 0x43, 0x21, 0xff, 0x81, 0x1f, 0x6c, 0xbd, 0x70, 0x28,
    0xe4, 0x3f, 0xf0, 0x83, 0xad, 0x17, 0x0e, 0x85, 0xfc, 0xe7, 0x85, 0xe3, 0x87, 0xad, 0x17, 0x0e,
    0x75, 0x48, 0x03, 0xea, 0x14, 0xa2, 0x17, 0x83, 0xe3, 0x83, 0xad, 0x17, 0x8e, 0xba, 0x08, 0x46,
    0xdf, 0x05, 0x3f, 0x51, 0xb0, 0x0a, 0xbb, 0x82, 0xf5, 0xda, 0xfd, 0xc1, 0x0f, 0x0d, 0x80, 0xda,
    0xec, 0x0f, 0x3e, 0x68, 0x00, 0xd4, 0x66, 0x7f, 0xf0, 0x41, 0x03, 0xa0, 0x36, 0xfb, 0x57, 0xc0,
    0xf9, 0xd0, 0x00, 0xa8, 0x1d, 0xff, 0xb8, 0xcf, 0x1a, 0x00, 0x35, 0x11, 0xff, 0xb8, 0xef, 0x30,
    0xf1, 0xc5, 0xe0, 0x94, 0xb8, 0xd2, 0xc6, 0x63,
];

/// The reviewed 43-byte BlockChanges payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const BLOCK_CHANGES_WIRE: [u8; 43] = [
    0x01, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xff, 0xff, 0xff,
    0xff, 0xc0, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
];

/// The reviewed 21-byte ForgetChunks payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const FORGET_CHUNKS_WIRE: [u8; 21] = [
    0x01, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff,
    0xff, 0x00, 0x00, 0x00, 0x00,
];

/// The reviewed 93-byte PlayerState payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PLAYER_STATE_WIRE: [u8; 93] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x14, 0x2c, 0x01, 0x14, 0x00, 0xbf, 0x5d,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x03, 0xff, 0x80, 0x14,
];

/// The reviewed 9-byte CommandRejected payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const COMMAND_REJECTED_WIRE: [u8; 9] = [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01];

/// The reviewed 8-byte KeepAlive payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const KEEP_ALIVE_WIRE: [u8; 8] = [0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];

/// The reviewed 2-byte Disconnect payload: the Go encoder's output for the
/// `decode-code-one-empty-message` corpus case.
const DISCONNECT_WIRE: [u8; 2] = [0x01, 0x00];

/// The reviewed 54-byte RemotePlayerSpawn payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const REMOTE_PLAYER_SPAWN_WIRE: [u8; 54] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
    0x05, 0x41, 0x6c, 0x69, 0x63, 0x65, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x40,
];

/// The reviewed 16-byte RemotePlayerDespawn payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const REMOTE_PLAYER_DESPAWN_WIRE: [u8; 16] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
];

/// The reviewed 91-byte RemotePlayerStates payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const REMOTE_PLAYER_STATES_WIRE: [u8; 91] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46,
    0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x40, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d,
    0x0e, 0x10, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0xc0, 0xbf, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x01,
];

/// The reviewed 181-byte InventoryState payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const INVENTORY_STATE_WIRE: [u8; 181] = [
    0x08, 0x01, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x00, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0a, 0x00, 0x01, 0x3c, 0x00, 0x02, 0x00,
    0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x05,
    0x00, 0x0c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x01, 0x00, 0x09, 0x00, 0x00,
];

/// The reviewed 61-byte ItemDropUpserts payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const ITEM_DROP_UPSERTS_WIRE: [u8; 61] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xff, 0xff, 0xff, 0x07, 0x00, 0x00,
    0x00, 0xfd, 0xff, 0xff, 0xff, 0x1f, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
    0x04, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0xfc, 0xff, 0xff, 0xff, 0x09, 0x00, 0x00, 0x00, 0x00,
    0x01, 0x00, 0x00, 0x00, 0xff, 0x7f, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
];

/// The reviewed 26-byte ItemDropRemoves payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const ITEM_DROP_REMOVES_WIRE: [u8; 26] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00,
    0x00, 0xfe, 0xff, 0xff, 0xff, 0x03, 0x07, 0x00, 0x00, 0x00,
];

/// The reviewed 36-byte FurnaceState payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const FURNACE_STATE_WIRE: [u8; 36] = [
    0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x02, 0x00, 0x00, 0x00, 0x00, 0x1f, 0x01, 0x00,
    0x00, 0x00, 0x35, 0x00, 0x01, 0x00, 0x00, 0x05, 0x00, 0x01, 0x00, 0x00, 0x36, 0x00, 0x01, 0x00,
    0x00, 0xc7, 0x40, 0x06,
];

/// The reviewed 18-byte ContainerClosed payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const CONTAINER_CLOSED_WIRE: [u8; 18] = [
    0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x02, 0x00, 0x00, 0x00, 0x00, 0x1f, 0x01, 0x00,
    0x00, 0x00,
];

/// The reviewed 153-byte ChestState payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const CHEST_STATE_WIRE: [u8; 153] = [
    0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x02, 0x00, 0x00, 0x00, 0x01, 0x0f, 0x01, 0x00,
    0x00, 0x00, 0x01, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0a, 0x00, 0x01,
    0x3c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x01, 0x00, 0x00,
];

/// The reviewed 66-byte ChatEvent payload: the Go encoder's output for the
/// `decode-accepted` corpus case.
const CHAT_EVENT_WIRE: [u8; 66] = [
    0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07,
    0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x05, 0x41, 0x6c, 0x69, 0x63, 0x65, 0x00, 0x01,
    0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x10, 0x04, 0x4d,
    0x69, 0x72, 0x61, 0x01, 0x00, 0x0c, 0x40, 0x6d, 0x69, 0x72, 0x61, 0x20, 0x66, 0x6f, 0x6c, 0x6c,
    0x6f, 0x77,
];

/// The reviewed 53-byte CompanionSpawn payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const COMPANION_SPAWN_WIRE: [u8; 53] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
    0x04, 0x4d, 0x69, 0x72, 0x61, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00,
    0x00, 0xdb, 0x0f, 0xc9, 0x3f,
];

/// The reviewed 50-byte CompanionStates payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const COMPANION_STATES_WIRE: [u8; 50] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46,
    0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00, 0x00, 0xdb, 0x0f, 0xc9,
    0x3f, 0x00,
];

/// The reviewed 16-byte CompanionDespawn payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const COMPANION_DESPAWN_WIRE: [u8; 16] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
];

/// The reviewed 8-byte PlaceBlockSucceeded payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PLACE_BLOCK_SUCCEEDED_WIRE: [u8; 8] = [0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];

/// The reviewed 51-byte CraftingState payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const CRAFTING_STATE_WIRE: [u8; 51] = [
    0x02, 0x01, 0x00, 0x02, 0x00, 0x00, 0x01, 0x00, 0x02, 0x00, 0x00, 0x25, 0x00, 0x01, 0x00, 0x00,
    0x25, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x00,
    0x01, 0x00, 0x00,
];

/// The reviewed 69-byte HostileSpawn payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const HOSTILE_SPAWN_WIRE: [u8; 69] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00,
    0x40, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x80, 0x40, 0x00, 0x00, 0xa0, 0x40, 0x00,
    0x00, 0x20, 0xc0, 0x14, 0x01,
];

/// The reviewed 85-byte HostileState payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const HOSTILE_STATE_WIRE: [u8; 85] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00,
    0x80, 0x00, 0x00, 0x80, 0x3e, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x00, 0x3f, 0x0d, 0x01, 0x02,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x80, 0x40, 0x00,
    0x00, 0xa0, 0x40, 0x00, 0x00, 0x00, 0x3f, 0x00, 0x00, 0xa0, 0xbf, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x20, 0xc0, 0x07, 0x00,
];

/// The reviewed 25-byte HostileDespawn payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const HOSTILE_DESPAWN_WIRE: [u8; 25] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
];

/// The reviewed 10-byte CombatHit payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const COMBAT_HIT_WIRE: [u8; 10] = [0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01];

/// The reviewed 67-byte PassiveSpawn payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PASSIVE_SPAWN_WIRE: [u8; 67] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00,
    0x40, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x80, 0x40, 0x00, 0x00, 0xa0, 0x40, 0x00, 0x00,
    0x20, 0xc0, 0x14,
];

/// The reviewed 85-byte PassiveState payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PASSIVE_STATE_WIRE: [u8; 85] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x00,
    0x80, 0x00, 0x00, 0x80, 0x3e, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x00, 0x3f, 0x0d, 0x01, 0x02,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x80, 0x40, 0x00,
    0x00, 0xa0, 0x40, 0x00, 0x00, 0x00, 0x3f, 0x00, 0x00, 0xa0, 0xbf, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x20, 0xc0, 0x07, 0x00,
];

/// The reviewed 27-byte PassiveDespawn payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PASSIVE_DESPAWN_WIRE: [u8; 27] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
];

/// The reviewed 83-byte ProjectileSpawn payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PROJECTILE_SPAWN_WIRE: [u8; 83] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00,
    0x00, 0x40, 0x00, 0x00, 0x00, 0x3f, 0x00, 0x00, 0xa0, 0xbf, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x00,
    0x00, 0x80, 0x40, 0x00, 0x00, 0xa0, 0x40, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x00, 0x40, 0x00,
    0x00, 0x40, 0x40,
];

/// The reviewed 49-byte ProjectileState payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PROJECTILE_STATE_WIRE: [u8; 49] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x80, 0x00, 0x00, 0x80, 0x3f, 0x00, 0x00, 0x00, 0x40, 0x02, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x40, 0x00, 0x00, 0x80, 0x40, 0x00, 0x00, 0xa0,
    0x40,
];

/// The reviewed 25-byte ProjectileDespawn payload: the Go encoder's output for the
/// `decode-valid` corpus case.
const PROJECTILE_DESPAWN_WIRE: [u8; 25] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
];

/// Declares the dispatch table and one test per registered key.
///
/// The macro is the single source of the table: each row names the test
/// that owns it, the complete key, the reviewed Go payload, the variant the
/// dispatcher must produce and how the re-encoding is compared. Generating
/// the table and the tests together is what keeps a new key from being
/// registered without a named case in `-- --list`.
macro_rules! registry_cases {
    (
        $(
            $test:ident => {
                key: $key:expr,
                payload: $payload:expr,
                variant: $variant:literal,
                reencode: $reencode:expr,
            }
        )*
    ) => {
        /// Every reviewed Go-produced payload the registry dispatches.
        const CASES: &[RegistryCase] = &[
            $(
                RegistryCase {
                    name: stringify!($test),
                    key: $key,
                    payload: $payload,
                    variant: $variant,
                    reencode: $reencode,
                },
            )*
        ];

        $(
            #[test]
            fn $test() {
                dispatch_case(case(stringify!($test)));
            }
        )*
    };
}

registry_cases! {
    dispatch_client_handshake_zero_client_hello => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Handshake,
            id: 0,
        },
        payload: &CLIENT_HELLO_WIRE,
        variant: "ClientHello",
        reencode: Reencode::Bytes,
    }
    dispatch_client_login_zero_login_start => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Login,
            id: 0,
        },
        payload: &LOGIN_START_WIRE,
        variant: "LoginStart",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_zero_player_input => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 0,
        },
        payload: &PLAYER_INPUT_WIRE,
        variant: "PlayerInput",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_two_place_block => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 2,
        },
        payload: &PLACE_BLOCK_WIRE,
        variant: "PlaceBlock",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_three_request_chunk_resync => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 3,
        },
        payload: &REQUEST_CHUNK_RESYNC_WIRE,
        variant: "RequestChunkResync",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_four_keep_alive_reply => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 4,
        },
        payload: &KEEP_ALIVE_REPLY_WIRE,
        variant: "KeepAliveReply",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_five_select_hotbar => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 5,
        },
        payload: &SELECT_HOTBAR_WIRE,
        variant: "SelectHotbar",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_six_move_inventory_stack => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 6,
        },
        payload: &MOVE_INVENTORY_STACK_WIRE,
        variant: "MoveInventoryStack",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_seven_move_crafting_stack => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 7,
        },
        payload: &MOVE_CRAFTING_STACK_WIRE,
        variant: "MoveCraftingStack",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_eight_open_container => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 8,
        },
        payload: &OPEN_CONTAINER_WIRE,
        variant: "OpenContainer",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_nine_move_container_stack => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 9,
        },
        payload: &MOVE_CONTAINER_STACK_WIRE,
        variant: "MoveContainerStack",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_ten_close_container => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 10,
        },
        payload: &CLOSE_CONTAINER_WIRE,
        variant: "CloseContainer",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_eleven_drop_selected_item => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 11,
        },
        payload: &DROP_SELECTED_ITEM_WIRE,
        variant: "DropSelectedItem",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_twelve_chat_command => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 12,
        },
        payload: &CHAT_COMMAND_WIRE,
        variant: "ChatCommand",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_thirteen_till_soil => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 13,
        },
        payload: &TILL_SOIL_WIRE,
        variant: "TillSoil",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_fourteen_bone_meal => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 14,
        },
        payload: &BONE_MEAL_WIRE,
        variant: "BoneMeal",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_fifteen_take_crafting_output => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 15,
        },
        payload: &TAKE_CRAFTING_OUTPUT_WIRE,
        variant: "TakeCraftingOutput",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_sixteen_collect_water => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 16,
        },
        payload: &COLLECT_WATER_WIRE,
        variant: "CollectWater",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_seventeen_place_water => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 17,
        },
        payload: &PLACE_WATER_WIRE,
        variant: "PlaceWater",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_eighteen_equip_armor => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 18,
        },
        payload: &EQUIP_ARMOR_WIRE,
        variant: "EquipArmor",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_nineteen_move_stack_partial => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 19,
        },
        payload: &MOVE_STACK_PARTIAL_WIRE,
        variant: "MoveStackPartial",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_twenty_quick_move_stack => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 20,
        },
        payload: &QUICK_MOVE_STACK_WIRE,
        variant: "QuickMoveStack",
        reencode: Reencode::Bytes,
    }
    dispatch_client_play_twenty_one_drop_stack => {
        key: PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 21,
        },
        payload: &DROP_STACK_WIRE,
        variant: "DropStack",
        reencode: Reencode::Bytes,
    }
    dispatch_server_handshake_zero_server_hello => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Handshake,
            id: 0,
        },
        payload: &SERVER_HELLO_WIRE,
        variant: "ServerHello",
        reencode: Reencode::Bytes,
    }
    dispatch_server_handshake_one_handshake_reject => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Handshake,
            id: 1,
        },
        payload: &HANDSHAKE_REJECT_WIRE,
        variant: "HandshakeReject",
        reencode: Reencode::Bytes,
    }
    dispatch_server_login_zero_login_success => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Login,
            id: 0,
        },
        payload: &LOGIN_SUCCESS_WIRE,
        variant: "LoginSuccess",
        reencode: Reencode::Bytes,
    }
    dispatch_server_login_one_login_reject => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Login,
            id: 1,
        },
        payload: &LOGIN_REJECT_WIRE,
        variant: "LoginReject",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_zero_chunk_snapshot => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 0,
        },
        payload: &CHUNK_SNAPSHOT_WIRE,
        variant: "ChunkSnapshot",
        reencode: Reencode::Logical,
    }
    dispatch_server_play_one_block_changes => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 1,
        },
        payload: &BLOCK_CHANGES_WIRE,
        variant: "BlockChanges",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_two_forget_chunks => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 2,
        },
        payload: &FORGET_CHUNKS_WIRE,
        variant: "ForgetChunks",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_three_player_state => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 3,
        },
        payload: &PLAYER_STATE_WIRE,
        variant: "PlayerState",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_four_command_rejected => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 4,
        },
        payload: &COMMAND_REJECTED_WIRE,
        variant: "CommandRejected",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_five_keep_alive => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 5,
        },
        payload: &KEEP_ALIVE_WIRE,
        variant: "KeepAlive",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_six_disconnect => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 6,
        },
        payload: &DISCONNECT_WIRE,
        variant: "Disconnect",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_seven_remote_player_spawn => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 7,
        },
        payload: &REMOTE_PLAYER_SPAWN_WIRE,
        variant: "RemotePlayerSpawn",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_eight_remote_player_despawn => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 8,
        },
        payload: &REMOTE_PLAYER_DESPAWN_WIRE,
        variant: "RemotePlayerDespawn",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_nine_remote_player_states => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 9,
        },
        payload: &REMOTE_PLAYER_STATES_WIRE,
        variant: "RemotePlayerStates",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_ten_inventory_state => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 10,
        },
        payload: &INVENTORY_STATE_WIRE,
        variant: "InventoryState",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_eleven_item_drop_upserts => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 11,
        },
        payload: &ITEM_DROP_UPSERTS_WIRE,
        variant: "ItemDropUpserts",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twelve_item_drop_removes => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 12,
        },
        payload: &ITEM_DROP_REMOVES_WIRE,
        variant: "ItemDropRemoves",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_thirteen_furnace_state => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 13,
        },
        payload: &FURNACE_STATE_WIRE,
        variant: "FurnaceState",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_fourteen_container_closed => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 14,
        },
        payload: &CONTAINER_CLOSED_WIRE,
        variant: "ContainerClosed",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_fifteen_chest_state => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 15,
        },
        payload: &CHEST_STATE_WIRE,
        variant: "ChestState",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_sixteen_chat_event => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 16,
        },
        payload: &CHAT_EVENT_WIRE,
        variant: "ChatEvent",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_seventeen_companion_spawn => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 17,
        },
        payload: &COMPANION_SPAWN_WIRE,
        variant: "CompanionSpawn",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_eighteen_companion_states => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 18,
        },
        payload: &COMPANION_STATES_WIRE,
        variant: "CompanionStates",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_nineteen_companion_despawn => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 19,
        },
        payload: &COMPANION_DESPAWN_WIRE,
        variant: "CompanionDespawn",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twenty_place_block_succeeded => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 20,
        },
        payload: &PLACE_BLOCK_SUCCEEDED_WIRE,
        variant: "PlaceBlockSucceeded",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twenty_one_crafting_state => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 21,
        },
        payload: &CRAFTING_STATE_WIRE,
        variant: "CraftingState",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twenty_two_hostile_spawn => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 22,
        },
        payload: &HOSTILE_SPAWN_WIRE,
        variant: "HostileSpawn",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twenty_three_hostile_state => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 23,
        },
        payload: &HOSTILE_STATE_WIRE,
        variant: "HostileState",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twenty_four_hostile_despawn => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 24,
        },
        payload: &HOSTILE_DESPAWN_WIRE,
        variant: "HostileDespawn",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twenty_five_combat_hit => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 25,
        },
        payload: &COMBAT_HIT_WIRE,
        variant: "CombatHit",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twenty_six_passive_spawn => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 26,
        },
        payload: &PASSIVE_SPAWN_WIRE,
        variant: "PassiveSpawn",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twenty_seven_passive_state => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 27,
        },
        payload: &PASSIVE_STATE_WIRE,
        variant: "PassiveState",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twenty_eight_passive_despawn => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 28,
        },
        payload: &PASSIVE_DESPAWN_WIRE,
        variant: "PassiveDespawn",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_twenty_nine_projectile_spawn => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 29,
        },
        payload: &PROJECTILE_SPAWN_WIRE,
        variant: "ProjectileSpawn",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_thirty_projectile_state => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 30,
        },
        payload: &PROJECTILE_STATE_WIRE,
        variant: "ProjectileState",
        reencode: Reencode::Bytes,
    }
    dispatch_server_play_thirty_one_projectile_despawn => {
        key: PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 31,
        },
        payload: &PROJECTILE_DESPAWN_WIRE,
        variant: "ProjectileDespawn",
        reencode: Reencode::Bytes,
    }
}

/// One row of the test-owned expected key table.
///
/// The table is written independently of the dispatch table, so a change to
/// the registry has to be made in two places and the comparison below is a
/// real check rather than a restatement.
struct ExpectedKey {
    /// The dispatch case the row expects to own the key.
    name: &'static str,
    /// The complete key the row expects that case to publish under.
    key: PacketKey,
    /// The variant the row expects the dispatcher to produce.
    variant: &'static str,
}

/// The closed v45 registry as this suite expects it: 23 client keys and 36
/// server keys, mirroring the Go `registry.go` tables.
fn expected_registry() -> Vec<ExpectedKey> {
    vec![
        ExpectedKey {
            name: "dispatch_client_handshake_zero_client_hello",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Handshake,
                id: 0,
            },
            variant: "ClientHello",
        },
        ExpectedKey {
            name: "dispatch_client_login_zero_login_start",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Login,
                id: 0,
            },
            variant: "LoginStart",
        },
        ExpectedKey {
            name: "dispatch_client_play_zero_player_input",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 0,
            },
            variant: "PlayerInput",
        },
        ExpectedKey {
            name: "dispatch_client_play_two_place_block",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 2,
            },
            variant: "PlaceBlock",
        },
        ExpectedKey {
            name: "dispatch_client_play_three_request_chunk_resync",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 3,
            },
            variant: "RequestChunkResync",
        },
        ExpectedKey {
            name: "dispatch_client_play_four_keep_alive_reply",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 4,
            },
            variant: "KeepAliveReply",
        },
        ExpectedKey {
            name: "dispatch_client_play_five_select_hotbar",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 5,
            },
            variant: "SelectHotbar",
        },
        ExpectedKey {
            name: "dispatch_client_play_six_move_inventory_stack",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 6,
            },
            variant: "MoveInventoryStack",
        },
        ExpectedKey {
            name: "dispatch_client_play_seven_move_crafting_stack",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 7,
            },
            variant: "MoveCraftingStack",
        },
        ExpectedKey {
            name: "dispatch_client_play_eight_open_container",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 8,
            },
            variant: "OpenContainer",
        },
        ExpectedKey {
            name: "dispatch_client_play_nine_move_container_stack",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 9,
            },
            variant: "MoveContainerStack",
        },
        ExpectedKey {
            name: "dispatch_client_play_ten_close_container",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 10,
            },
            variant: "CloseContainer",
        },
        ExpectedKey {
            name: "dispatch_client_play_eleven_drop_selected_item",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 11,
            },
            variant: "DropSelectedItem",
        },
        ExpectedKey {
            name: "dispatch_client_play_twelve_chat_command",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 12,
            },
            variant: "ChatCommand",
        },
        ExpectedKey {
            name: "dispatch_client_play_thirteen_till_soil",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 13,
            },
            variant: "TillSoil",
        },
        ExpectedKey {
            name: "dispatch_client_play_fourteen_bone_meal",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 14,
            },
            variant: "BoneMeal",
        },
        ExpectedKey {
            name: "dispatch_client_play_fifteen_take_crafting_output",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 15,
            },
            variant: "TakeCraftingOutput",
        },
        ExpectedKey {
            name: "dispatch_client_play_sixteen_collect_water",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 16,
            },
            variant: "CollectWater",
        },
        ExpectedKey {
            name: "dispatch_client_play_seventeen_place_water",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 17,
            },
            variant: "PlaceWater",
        },
        ExpectedKey {
            name: "dispatch_client_play_eighteen_equip_armor",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 18,
            },
            variant: "EquipArmor",
        },
        ExpectedKey {
            name: "dispatch_client_play_nineteen_move_stack_partial",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 19,
            },
            variant: "MoveStackPartial",
        },
        ExpectedKey {
            name: "dispatch_client_play_twenty_quick_move_stack",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 20,
            },
            variant: "QuickMoveStack",
        },
        ExpectedKey {
            name: "dispatch_client_play_twenty_one_drop_stack",
            key: PacketKey {
                direction: Direction::ClientToServer,
                state: State::Play,
                id: 21,
            },
            variant: "DropStack",
        },
        ExpectedKey {
            name: "dispatch_server_handshake_zero_server_hello",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Handshake,
                id: 0,
            },
            variant: "ServerHello",
        },
        ExpectedKey {
            name: "dispatch_server_handshake_one_handshake_reject",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Handshake,
                id: 1,
            },
            variant: "HandshakeReject",
        },
        ExpectedKey {
            name: "dispatch_server_login_zero_login_success",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Login,
                id: 0,
            },
            variant: "LoginSuccess",
        },
        ExpectedKey {
            name: "dispatch_server_login_one_login_reject",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Login,
                id: 1,
            },
            variant: "LoginReject",
        },
        ExpectedKey {
            name: "dispatch_server_play_zero_chunk_snapshot",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 0,
            },
            variant: "ChunkSnapshot",
        },
        ExpectedKey {
            name: "dispatch_server_play_one_block_changes",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 1,
            },
            variant: "BlockChanges",
        },
        ExpectedKey {
            name: "dispatch_server_play_two_forget_chunks",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 2,
            },
            variant: "ForgetChunks",
        },
        ExpectedKey {
            name: "dispatch_server_play_three_player_state",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 3,
            },
            variant: "PlayerState",
        },
        ExpectedKey {
            name: "dispatch_server_play_four_command_rejected",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 4,
            },
            variant: "CommandRejected",
        },
        ExpectedKey {
            name: "dispatch_server_play_five_keep_alive",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 5,
            },
            variant: "KeepAlive",
        },
        ExpectedKey {
            name: "dispatch_server_play_six_disconnect",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 6,
            },
            variant: "Disconnect",
        },
        ExpectedKey {
            name: "dispatch_server_play_seven_remote_player_spawn",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 7,
            },
            variant: "RemotePlayerSpawn",
        },
        ExpectedKey {
            name: "dispatch_server_play_eight_remote_player_despawn",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 8,
            },
            variant: "RemotePlayerDespawn",
        },
        ExpectedKey {
            name: "dispatch_server_play_nine_remote_player_states",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 9,
            },
            variant: "RemotePlayerStates",
        },
        ExpectedKey {
            name: "dispatch_server_play_ten_inventory_state",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 10,
            },
            variant: "InventoryState",
        },
        ExpectedKey {
            name: "dispatch_server_play_eleven_item_drop_upserts",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 11,
            },
            variant: "ItemDropUpserts",
        },
        ExpectedKey {
            name: "dispatch_server_play_twelve_item_drop_removes",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 12,
            },
            variant: "ItemDropRemoves",
        },
        ExpectedKey {
            name: "dispatch_server_play_thirteen_furnace_state",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 13,
            },
            variant: "FurnaceState",
        },
        ExpectedKey {
            name: "dispatch_server_play_fourteen_container_closed",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 14,
            },
            variant: "ContainerClosed",
        },
        ExpectedKey {
            name: "dispatch_server_play_fifteen_chest_state",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 15,
            },
            variant: "ChestState",
        },
        ExpectedKey {
            name: "dispatch_server_play_sixteen_chat_event",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 16,
            },
            variant: "ChatEvent",
        },
        ExpectedKey {
            name: "dispatch_server_play_seventeen_companion_spawn",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 17,
            },
            variant: "CompanionSpawn",
        },
        ExpectedKey {
            name: "dispatch_server_play_eighteen_companion_states",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 18,
            },
            variant: "CompanionStates",
        },
        ExpectedKey {
            name: "dispatch_server_play_nineteen_companion_despawn",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 19,
            },
            variant: "CompanionDespawn",
        },
        ExpectedKey {
            name: "dispatch_server_play_twenty_place_block_succeeded",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 20,
            },
            variant: "PlaceBlockSucceeded",
        },
        ExpectedKey {
            name: "dispatch_server_play_twenty_one_crafting_state",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 21,
            },
            variant: "CraftingState",
        },
        ExpectedKey {
            name: "dispatch_server_play_twenty_two_hostile_spawn",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 22,
            },
            variant: "HostileSpawn",
        },
        ExpectedKey {
            name: "dispatch_server_play_twenty_three_hostile_state",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 23,
            },
            variant: "HostileState",
        },
        ExpectedKey {
            name: "dispatch_server_play_twenty_four_hostile_despawn",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 24,
            },
            variant: "HostileDespawn",
        },
        ExpectedKey {
            name: "dispatch_server_play_twenty_five_combat_hit",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 25,
            },
            variant: "CombatHit",
        },
        ExpectedKey {
            name: "dispatch_server_play_twenty_six_passive_spawn",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 26,
            },
            variant: "PassiveSpawn",
        },
        ExpectedKey {
            name: "dispatch_server_play_twenty_seven_passive_state",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 27,
            },
            variant: "PassiveState",
        },
        ExpectedKey {
            name: "dispatch_server_play_twenty_eight_passive_despawn",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 28,
            },
            variant: "PassiveDespawn",
        },
        ExpectedKey {
            name: "dispatch_server_play_twenty_nine_projectile_spawn",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 29,
            },
            variant: "ProjectileSpawn",
        },
        ExpectedKey {
            name: "dispatch_server_play_thirty_projectile_state",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 30,
            },
            variant: "ProjectileState",
        },
        ExpectedKey {
            name: "dispatch_server_play_thirty_one_projectile_despawn",
            key: PacketKey {
                direction: Direction::ServerToClient,
                state: State::Play,
                id: 31,
            },
            variant: "ProjectileDespawn",
        },
    ]
}

/// Compares an expected key table against the dispatch table and the real
/// dispatcher.
///
/// Every failure names a row, a key or a variant. A count is never the
/// signal, so a dropped, duplicated or swapped key is reported as the key it
/// is rather than as a difference in size.
fn compare_registry(expected: &[ExpectedKey]) -> Result<(), String> {
    let mut problems: Vec<String> = Vec::new();
    for row in CASES {
        match expected.iter().find(|candidate| candidate.name == row.name) {
            None => problems.push(format!(
                "the expected key table dropped {} under {:?}",
                row.name, row.key
            )),
            Some(wanted) => {
                if wanted.key != row.key {
                    problems.push(format!(
                        "{}: the expected key {:?} is not the registry key {:?}",
                        row.name, wanted.key, row.key
                    ));
                }
                if wanted.variant != row.variant {
                    problems.push(format!(
                        "{}: the expected variant {} is not the registry variant {}",
                        row.name, wanted.variant, row.variant
                    ));
                }
            }
        }
    }
    for row in expected {
        if !CASES.iter().any(|candidate| candidate.name == row.name) {
            problems.push(format!(
                "the expected key table carries {} under {:?}, which no dispatched case names",
                row.name, row.key
            ));
        }
    }
    for (index, row) in expected.iter().enumerate() {
        if let Some(earlier) = expected[..index]
            .iter()
            .find(|candidate| candidate.key == row.key)
        {
            problems.push(format!(
                "the expected key table publishes {:?} twice, first as {} and again as {}",
                row.key, earlier.name, row.name
            ));
        }
    }
    if problems.is_empty() {
        Ok(())
    } else {
        Err(problems.join("; "))
    }
}

/// Verifies the expected key table, turning a rejection into a panic.
fn verify_registry(expected: &[ExpectedKey]) {
    compare_registry(expected).unwrap_or_else(|message| panic!("{message}"));
}

/// The registry covers exactly the 59 Go v45 keys and nothing else.
///
/// The count is asserted here rather than inside `compare_registry`, so the
/// key-by-key comparison can never be satisfied or broken by a count alone.
#[test]
fn registry_tables_cover_exactly_the_fifty_nine_keys() {
    let expected = expected_registry();
    assert_eq!(
        CASES.len(),
        59,
        "the dispatch table must carry one row per registered key"
    );
    assert_eq!(
        expected.len(),
        59,
        "the expected key table must carry one row per registered key"
    );
    let client = CASES
        .iter()
        .filter(|row| row.key.direction == Direction::ClientToServer)
        .count();
    let server = CASES
        .iter()
        .filter(|row| row.key.direction == Direction::ServerToClient)
        .count();
    assert_eq!(client, 23, "the Go registry publishes 23 client keys");
    assert_eq!(server, 36, "the Go registry publishes 36 server keys");
    verify_registry(&expected);
}

/// Dropping one row from the expected key table fails by key and variant.
#[test]
fn registry_expected_table_dropping_one_key_fails_by_key() {
    let mut expected = expected_registry();
    expected.retain(|row| row.name != "dispatch_server_play_twenty_four_hostile_despawn");
    let message = compare_registry(&expected)
        .expect_err("a dropped key must be reported rather than accepted");
    assert!(
        message.contains("hostile_despawn"),
        "the rejection must name the dropped case: {message}"
    );
    assert!(
        message.contains("24"),
        "the rejection must name the dropped key: {message}"
    );
}

/// Duplicating one complete key in the expected table fails by key.
#[test]
fn registry_expected_table_duplicating_one_key_fails_by_key() {
    let mut expected = expected_registry();
    let duplicated = expected
        .iter()
        .find(|row| row.name == "dispatch_server_play_twenty_one_crafting_state")
        .expect("the crafting state case is registered");
    expected.push(ExpectedKey {
        name: duplicated.name,
        key: duplicated.key,
        variant: duplicated.variant,
    });
    let message =
        compare_registry(&expected).expect_err("a duplicated key must be reported, not counted");
    assert!(
        message.contains("crafting_state"),
        "the rejection must name the duplicated case: {message}"
    );
    assert!(
        message.contains("21"),
        "the rejection must name the duplicated key: {message}"
    );
}

/// Swapping two server IDs in the expected table fails by key and variant.
#[test]
fn registry_expected_table_swapping_two_server_ids_fails_by_key() {
    let mut expected = expected_registry();
    let spawn = expected
        .iter()
        .position(|row| row.name == "dispatch_server_play_twenty_two_hostile_spawn")
        .expect("the hostile spawn case is registered");
    let state = expected
        .iter()
        .position(|row| row.name == "dispatch_server_play_twenty_three_hostile_state")
        .expect("the hostile state case is registered");
    let swapped_key = expected[spawn].key;
    let swapped_variant = expected[spawn].variant;
    expected[spawn].key = expected[state].key;
    expected[spawn].variant = expected[state].variant;
    expected[state].key = swapped_key;
    expected[state].variant = swapped_variant;
    let message =
        compare_registry(&expected).expect_err("a swapped key pair must be reported by key");
    assert!(
        message.contains("hostile_spawn") && message.contains("hostile_state"),
        "the rejection must name both swapped cases: {message}"
    );
    assert!(
        message.contains("22") && message.contains("23"),
        "the rejection must name both swapped keys: {message}"
    );
}

/// No ID outside the registered set dispatches, in either direction and in all
/// three states.
///
/// The sweep is what makes an unknown state byte a Rust-side refusal as well:
/// the closed `State` enum has exactly three members, so the fourth state a Go
/// peer could name has no representation here and every ID under the three real
/// states is either registered or `UnknownPacket`.
#[test]
fn registry_every_unregistered_key_is_unknown_packet() {
    let states = [State::Handshake, State::Login, State::Play];
    assert_eq!(
        states.len(),
        3,
        "the closed state set is exactly the three Go states"
    );
    let mut codec = ProtocolCodec::new().expect("one owned snapshot context");
    for direction in [Direction::ClientToServer, Direction::ServerToClient] {
        for state in states {
            for id in 0u32..=u8::MAX as u32 {
                let key = PacketKey {
                    direction,
                    state,
                    id,
                };
                let outcome = match direction {
                    Direction::ClientToServer => {
                        decode_client(state, id, &PLAYER_INPUT_WIRE).map(|packet| packet.key())
                    }
                    Direction::ServerToClient => codec
                        .decode_server(state, id, &PLAYER_INPUT_WIRE)
                        .map(|packet| packet.key()),
                };
                match outcome {
                    Ok(dispatched) => {
                        assert_eq!(
                            dispatched, key,
                            "{key:?} dispatched {dispatched:?}: the dispatcher must answer \
                             the requested key"
                        );
                        assert!(
                            is_registered(&key),
                            "{key:?} dispatched but is not one of the 59 registered keys"
                        );
                    }
                    Err(error) => {
                        if !is_registered(&key) {
                            assert_eq!(
                                error,
                                ProtocolError::UnknownPacket,
                                "{key:?} is not registered, so the refusal must name the key"
                            );
                        }
                    }
                }
            }
        }
    }
}

/// The retired and unassigned Play IDs never reach a decoder.
#[test]
fn registry_refuses_reserved_and_unassigned_play_ids() {
    for key in [
        PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 1,
        },
        PacketKey {
            direction: Direction::ClientToServer,
            state: State::Play,
            id: 22,
        },
        PacketKey {
            direction: Direction::ServerToClient,
            state: State::Play,
            id: 32,
        },
    ] {
        let error = match key.direction {
            Direction::ClientToServer => {
                decode_client(key.state, key.id, &PLAYER_INPUT_WIRE).expect_err("an unassigned ID")
            }
            Direction::ServerToClient => {
                let mut codec = ProtocolCodec::new().expect("one owned snapshot context");
                codec
                    .decode_server(key.state, key.id, &PLAYER_INPUT_WIRE)
                    .expect_err("an unassigned ID")
            }
        };
        assert_eq!(
            error,
            ProtocolError::UnknownPacket,
            "{key:?} must be refused as an unknown key before any field is read"
        );
    }
}

/// A login payload presented under a Play ID is refused by the Play record's
/// own field rules.
///
/// The reviewed login start and a player input share the 23-byte stride, so
/// the refusal cannot come from a length check: the login payload's third byte
/// is not a 0/1 flag, which is the boundary the play record publishes.
#[test]
fn registry_a_login_payload_under_a_play_id_is_refused() {
    let error = decode_client(State::Play, 0, &LOGIN_START_WIRE)
        .expect_err("a login payload is not a player input");
    assert_eq!(
        error,
        ProtocolError::InvalidEnum,
        "the login payload's jump byte is not a 0/1 flag"
    );
}

/// An old-version hello decodes structurally and is refused by admission.
///
/// The structural path is what lets a peer running another version receive the
/// negotiated mismatch answer instead of a bare decode failure, so the
/// dispatcher must publish the raw record and `validate_hello` must be the
/// function that refuses it.
#[test]
fn registry_an_old_version_hello_reaches_validate_hello() {
    const PREVIOUS_PROTOCOL_VERSION_WIRE: [u8; 1] = [0x2c];
    let packet = decode_client(State::Handshake, 0, &PREVIOUS_PROTOCOL_VERSION_WIRE)
        .expect("the structural inbound path keeps a peer's own version");
    let ClientPacket::ClientHello(hello) = &packet else {
        panic!("C/Handshake/0 dispatches the raw inbound hello");
    };
    assert_eq!(hello.protocol_version(), 44);
    assert_eq!(
        validate_hello(*hello),
        Err(HandshakeRejection::VersionMismatch { server_version: 45 }),
        "admission owns the version decision, not the dispatcher"
    );
}

/// The complete three-part key resolves the direction collision.
///
/// C/Handshake/0 and S/Handshake/0 carry the same payload shape under the same
/// numeric ID, so the same reviewed bytes are a valid record in both
/// directions. What tells them apart is the direction in the requested key,
/// which is why a per-struct packet ID cannot serve as the registry key.
#[test]
fn registry_the_complete_key_resolves_a_direction_collision() {
    let client = decode_client(State::Handshake, 0, &CLIENT_HELLO_WIRE)
        .expect("the reviewed client hello decodes");
    assert_eq!(
        client.key(),
        PacketKey {
            direction: Direction::ClientToServer,
            state: State::Handshake,
            id: 0,
        }
    );
    assert_eq!(client_variant(&client), "ClientHello");

    let mut codec = ProtocolCodec::new().expect("one owned snapshot context");
    let server = codec
        .decode_server(State::Handshake, 0, &SERVER_HELLO_WIRE)
        .expect("the reviewed server hello decodes");
    assert_eq!(
        server.key(),
        PacketKey {
            direction: Direction::ServerToClient,
            state: State::Handshake,
            id: 0,
        }
    );
    assert_eq!(server_variant(&server), "ServerHello");

    let echoed = codec
        .decode_server(State::Handshake, 0, &CLIENT_HELLO_WIRE)
        .expect("the same bytes are a structurally valid server hello");
    assert_eq!(
        server_variant(&echoed),
        "ServerHello",
        "the server dispatcher answers the server key, not the client one"
    );
    let mirrored = decode_client(State::Handshake, 0, &SERVER_HELLO_WIRE)
        .expect("the same bytes are a structurally valid client hello");
    assert_eq!(
        client_variant(&mirrored),
        "ClientHello",
        "the client dispatcher answers the client key, not the server one"
    );
}

/// The per-packet payload ceiling is applied before any field is read.
///
/// The Go decoder refuses an oversized payload before it parses one, so the
/// refusal is a size answer rather than a field answer.
#[test]
fn registry_refuses_a_payload_above_the_small_payload_ceiling() {
    let oversized = vec![0u8; MAX_SMALL_PAYLOAD_BYTES + 1];
    let error = decode_client(State::Play, 0, &oversized)
        .expect_err("a payload above the ceiling is refused before any field is read");
    assert_eq!(error, ProtocolError::Allocation);

    let mut codec = ProtocolCodec::new().expect("one owned snapshot context");
    let error = codec
        .decode_server(State::Play, 5, &oversized)
        .expect_err("a payload above the ceiling is refused before any field is read");
    assert_eq!(error, ProtocolError::Allocation);
}

/// The compressed snapshot is bounded by its own ceilings, not the small one.
///
/// The Go decoder routes the snapshot away from the control decoder's 64 KiB
/// check, so a payload above that ceiling must be answered by the envelope's
/// own boundary rather than by the small-payload refusal.
#[test]
fn registry_the_snapshot_family_is_not_bounded_by_the_small_payload_ceiling() {
    let oversized = vec![0u8; MAX_SMALL_PAYLOAD_BYTES + 1];
    let mut codec = ProtocolCodec::new().expect("one owned snapshot context");
    let error = codec
        .decode_server(State::Play, 0, &oversized)
        .expect_err("an all-zero envelope is not a snapshot");
    assert_eq!(
        error,
        ProtocolError::Truncated,
        "the compressed family's own envelope boundary answers, not the small ceiling"
    );
}
