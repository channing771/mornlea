//! Foundation registration, dependency-direction, and framing contracts for
//! `mornlea_protocol`. Remaining packet-family ports land one inventory row
//! at a time.

use std::fs;
use std::path::PathBuf;

#[path = "../../../tests/runtime_corpus.rs"]
mod runtime_corpus;

const FORBIDDEN_PRODUCTION_DEPS: &[&str] = &[
    "mornlea_storage",
    "mornlea_engine",
    "mornlea_client",
    "mornlea_godot",
];

#[test]
fn crate_identity_matches_workspace_name() {
    assert_eq!(mornlea_protocol::CRATE_NAME, env!("CARGO_PKG_NAME"));
}

#[test]
fn production_manifest_depends_only_on_domain() {
    // Exactly one compression dependency is permitted: the ChunkSnapshot
    // envelope is a zstd frame with an xxhash-64 content checksum, so the
    // codec needs a real encoder rather than a decode-only crate. Every other
    // production dependency stays forbidden, including the other workspace
    // crates listed below, so this is an exact-set assertion and not a
    // "does not contain" check.
    let keys = production_dependency_keys(&read_manifest(env!("CARGO_MANIFEST_DIR")));
    assert_eq!(keys, ["mornlea_domain", "zstd"]);
    for forbidden in FORBIDDEN_PRODUCTION_DEPS {
        assert!(
            !keys.iter().any(|key| key == forbidden),
            "mornlea_protocol must not depend on {forbidden}"
        );
    }
}

#[test]
fn domain_does_not_depend_on_protocol() {
    let domain_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../mornlea_domain");
    let keys = production_dependency_keys(&read_manifest(domain_dir.to_str().unwrap()));
    assert!(
        !keys.iter().any(|key| key == "mornlea_protocol"),
        "mornlea_domain must not depend on mornlea_protocol"
    );
}

#[test]
fn inventory_assigns_protocol_families_to_this_crate() {
    let json = read_inventory();
    let owner = format!("\"eventual_owner\": \"{}\"", env!("CARGO_PKG_NAME"));
    let count = json.matches(&owner).count();
    assert_eq!(
        count, 60,
        "protocol inventory rows drifted; update intended family ports before implementing them"
    );
    assert!(json.contains("\"id\": \"protocol.frame\""));
}

#[test]
fn frame_round_trip_preserves_packet_id_and_payload() {
    let first = mornlea_protocol::write_frame(3, &[1, 2, 3]).expect("first frame");
    let second = mornlea_protocol::write_frame(4, &[5, 6]).expect("second frame");
    let mut wire = first;
    wire.extend_from_slice(&second);

    let (id, payload, used) = mornlea_protocol::read_frame(&wire).expect("first decode");
    assert_eq!(id, 3);
    assert_eq!(payload, [1, 2, 3]);
    let (id, payload, used_second) =
        mornlea_protocol::read_frame(&wire[used..]).expect("second decode");
    assert_eq!(id, 4);
    assert_eq!(payload, [5, 6]);
    assert_eq!(used + used_second, wire.len());
}

#[test]
fn frame_write_uses_canonical_length_prefix() {
    let wire = mornlea_protocol::write_frame(128, &[1, 2, 3]).expect("frame");
    assert_eq!(wire, [5, 0x80, 0x01, 1, 2, 3]);
}

#[test]
fn frame_read_accepts_packet_id_only() {
    let (id, payload, used) = mornlea_protocol::read_frame(&[1, 7]).expect("packet id frame");
    assert_eq!(id, 7);
    assert_eq!(payload, [] as [u8; 0]);
    assert_eq!(used, 2);
}

#[test]
fn frame_read_rejects_invalid_lengths_before_payload() {
    for wire in [
        &[0u8][..],
        &[0x81, 0x80, 0x80, 0x01],
        &[0x80],
        &[0x81, 0x00],
        &[0xff, 0xff, 0xff, 0xff, 0x1f],
        &[0x80, 0x80, 0x80, 0x80, 0x80, 0],
    ] {
        assert!(
            mornlea_protocol::read_frame(wire).is_err(),
            "accepted invalid length {wire:?}"
        );
    }
}

#[test]
fn frame_read_rejects_truncated_and_noncanonical_packet_id() {
    for wire in [
        &[2u8, 1][..],
        &[1, 0x80],
        &[5, 0xff, 0xff, 0xff, 0xff, 0x1f],
        &[2, 0x81, 0x00],
    ] {
        assert!(
            mornlea_protocol::read_frame(wire).is_err(),
            "accepted malformed frame {wire:?}"
        );
    }
}

#[test]
fn frame_write_enforces_maximum_payload() {
    let max = vec![0u8; (mornlea_protocol::MAX_FRAME_BYTES - 1) as usize];
    mornlea_protocol::write_frame(0, &max).expect("maximum payload");
    let oversized = vec![0u8; mornlea_protocol::MAX_FRAME_BYTES as usize];
    assert!(mornlea_protocol::write_frame(0, &oversized).is_err());
}

#[test]
fn canonical_uvarint_round_trips_and_rejects_malformed() {
    for (value, encoded) in [
        (0u32, &[0x00][..]),
        (1, &[0x01]),
        (127, &[0x7f]),
        (128, &[0x80, 0x01]),
        (u32::MAX, &[0xff, 0xff, 0xff, 0xff, 0x0f]),
    ] {
        assert_eq!(mornlea_protocol::encode_uvarint(value), encoded);
        let (got, used) = mornlea_protocol::decode_uvarint(encoded).expect("canonical uvarint");
        assert_eq!(got, value);
        assert_eq!(used, encoded.len());
    }
    for bad in [&[0x80][..], &[0x81, 0x00], &[0xff, 0xff, 0xff, 0xff, 0x1f]] {
        assert!(
            mornlea_protocol::decode_uvarint(bad).is_err(),
            "accepted malformed uvarint {bad:?}"
        );
    }
}

#[test]
fn client_hello_round_trip_preserves_current_version_bytes() {
    let hello = mornlea_protocol::ClientHello::new(45).expect("current hello");
    let payload = hello.encode();
    assert_eq!(payload, [0x2d]);
    assert_eq!(mornlea_protocol::ClientHello::PACKET_ID, 0);
    let decoded = mornlea_protocol::ClientHello::decode(&payload).expect("decode hello");
    assert_eq!(decoded, hello);
    assert_eq!(decoded.protocol_version, 45);
}

#[test]
fn client_hello_rejects_unknown_version_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::ClientHello::new(0),
        Err(mornlea_protocol::ProtocolError::UnsupportedVersion)
    );
    assert_eq!(
        mornlea_protocol::ClientHello::new(46),
        Err(mornlea_protocol::ProtocolError::UnsupportedVersion)
    );
    assert_eq!(
        mornlea_protocol::ClientHello::decode(&[0x2e]),
        Err(mornlea_protocol::ProtocolError::UnsupportedVersion)
    );
    assert!(mornlea_protocol::ClientHello::decode(&[]).is_err());
    assert!(mornlea_protocol::ClientHello::decode(&[0x80]).is_err());
    assert!(mornlea_protocol::ClientHello::decode(&[0x81, 0x00]).is_err());
    assert_eq!(
        mornlea_protocol::ClientHello::decode(&[0x2d, 0x00]),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn server_hello_round_trip_preserves_current_version_bytes() {
    let hello = mornlea_protocol::ServerHello::new(45).expect("current hello");
    let payload = hello.encode();
    assert_eq!(payload, [0x2d]);
    assert_eq!(mornlea_protocol::ServerHello::PACKET_ID, 0);
    let decoded = mornlea_protocol::ServerHello::decode(&payload).expect("decode hello");
    assert_eq!(decoded, hello);
    assert_eq!(decoded.protocol_version, 45);
}

#[test]
fn server_hello_rejects_unknown_version_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::ServerHello::new(0),
        Err(mornlea_protocol::ProtocolError::UnsupportedVersion)
    );
    assert_eq!(
        mornlea_protocol::ServerHello::decode(&[0x2e]),
        Err(mornlea_protocol::ProtocolError::UnsupportedVersion)
    );
    assert!(mornlea_protocol::ServerHello::decode(&[]).is_err());
    assert_eq!(
        mornlea_protocol::ServerHello::decode(&[0x2d, 0x00]),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn handshake_reject_round_trip_preserves_golden_bytes() {
    let reject = mornlea_protocol::HandshakeReject::new(42, 1, "no").expect("reject");
    let payload = reject.encode();
    assert_eq!(payload, [0x2a, 0x01, 0x02, b'n', b'o']);
    assert_eq!(mornlea_protocol::HandshakeReject::PACKET_ID, 1);
    let decoded = mornlea_protocol::HandshakeReject::decode(&payload).expect("decode");
    assert_eq!(decoded, reject);
    assert_eq!(decoded.server_protocol_version, 42);
    assert_eq!(decoded.code, 1);
    assert_eq!(decoded.message, "no");
}

#[test]
fn handshake_reject_round_trip_preserves_empty_message() {
    let reject = mornlea_protocol::HandshakeReject::new(8, 1, "").expect("empty message");
    let payload = reject.encode();
    assert_eq!(payload, [0x08, 0x01, 0x00]);
    let decoded = mornlea_protocol::HandshakeReject::decode(&payload).expect("decode");
    assert_eq!(decoded, reject);
}

#[test]
fn handshake_reject_rejects_unknown_code_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::HandshakeReject::new(45, 0, ""),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::HandshakeReject::new(45, 2, ""),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::HandshakeReject::decode(&[0x2a, 0x00, 0x00]),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert!(mornlea_protocol::HandshakeReject::decode(&[]).is_err());
    assert_eq!(
        mornlea_protocol::HandshakeReject::decode(&[0x2a, 0x01, 0x02, b'n', b'o', 0x00]),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    assert_eq!(
        mornlea_protocol::HandshakeReject::decode(&[0x2a, 0x01, 0x01, 0xff]),
        Err(mornlea_protocol::ProtocolError::InvalidString)
    );
    let mut oversized = vec![0x2a, 0x01];
    oversized.extend(mornlea_protocol::encode_uvarint(257));
    oversized.extend(std::iter::repeat_n(b'a', 257));
    assert_eq!(
        mornlea_protocol::HandshakeReject::decode(&oversized),
        Err(mornlea_protocol::ProtocolError::InvalidString)
    );
}

const GOLDEN_PLAYER_ID: [u8; 16] = [
    0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
];

#[test]
fn login_start_round_trip_preserves_golden_bytes() {
    let id = mornlea_protocol::PlayerId::new(GOLDEN_PLAYER_ID).expect("uuid v4");
    let login = mornlea_protocol::LoginStart::new(id, "Chen", 32).expect("login");
    let payload = login.encode();
    assert_eq!(
        payload,
        [
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff, 0x04, b'C', b'h', b'e', b'n', 0x20,
        ]
    );
    assert_eq!(mornlea_protocol::LoginStart::PACKET_ID, 0);
    let decoded = mornlea_protocol::LoginStart::decode(&payload).expect("decode");
    assert_eq!(decoded, login);
    assert_eq!(decoded.player_id, id);
    assert_eq!(decoded.display_name, "Chen");
    assert_eq!(decoded.view_distance, 32);
}

#[test]
fn login_start_rejects_invalid_identity_name_range_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::PlayerId::new([0; 16]),
        Err(mornlea_protocol::ProtocolError::InvalidIdentity)
    );
    let mut not_v4 = GOLDEN_PLAYER_ID;
    not_v4[6] = 0x55;
    assert_eq!(
        mornlea_protocol::PlayerId::new(not_v4),
        Err(mornlea_protocol::ProtocolError::InvalidIdentity)
    );
    let id = mornlea_protocol::PlayerId::new(GOLDEN_PLAYER_ID).expect("uuid v4");
    assert_eq!(
        mornlea_protocol::LoginStart::new(id, "Chen\nName", 32),
        Err(mornlea_protocol::ProtocolError::InvalidString)
    );
    assert_eq!(
        mornlea_protocol::LoginStart::new(id, "Chen", 1),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::LoginStart::new(id, "Chen", 65),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    let mut truncated = GOLDEN_PLAYER_ID.to_vec();
    truncated.extend_from_slice(&[0x04, b'C', b'h', b'e', b'n']);
    assert!(mornlea_protocol::LoginStart::decode(&truncated).is_err());
    let mut trailing = GOLDEN_PLAYER_ID.to_vec();
    trailing.extend_from_slice(&[0x04, b'C', b'h', b'e', b'n', 0x20, 0x00]);
    assert_eq!(
        mornlea_protocol::LoginStart::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn login_success_round_trip_preserves_golden_bytes() {
    let id = mornlea_protocol::PlayerId::new(GOLDEN_PLAYER_ID).expect("uuid v4");
    let success = mornlea_protocol::LoginSuccess::new(id, 0x1122_3344_5566_7788);
    let payload = success.encode();
    assert_eq!(
        payload,
        [
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff, 0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11,
        ]
    );
    assert_eq!(mornlea_protocol::LoginSuccess::PACKET_ID, 0);
    let decoded = mornlea_protocol::LoginSuccess::decode(&payload).expect("decode");
    assert_eq!(decoded, success);
    assert_eq!(decoded.player_id, id);
    assert_eq!(decoded.world_seed, 0x1122_3344_5566_7788);
}

#[test]
fn login_success_rejects_invalid_identity_and_malformed_payload() {
    let zero = mornlea_protocol::LoginSuccess::decode(&[0; 24]);
    assert_eq!(zero, Err(mornlea_protocol::ProtocolError::InvalidIdentity));
    assert!(mornlea_protocol::LoginSuccess::decode(&[0; 15]).is_err());
    let mut trailing = GOLDEN_PLAYER_ID.to_vec();
    trailing.extend_from_slice(&[0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11, 0x00]);
    assert_eq!(
        mornlea_protocol::LoginSuccess::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let id = mornlea_protocol::PlayerId::new(GOLDEN_PLAYER_ID).expect("uuid v4");
    let zero_seed = mornlea_protocol::LoginSuccess::new(id, 0);
    assert_eq!(zero_seed.world_seed, 0);
    assert_eq!(
        mornlea_protocol::LoginSuccess::decode(&zero_seed.encode()).expect("decode zero seed"),
        zero_seed
    );
}

#[test]
fn login_reject_round_trip_preserves_golden_bytes() {
    let reject = mornlea_protocol::LoginReject::new(2, "no").expect("reject");
    let payload = reject.encode();
    assert_eq!(payload, [0x02, 0x02, b'n', b'o']);
    assert_eq!(mornlea_protocol::LoginReject::PACKET_ID, 1);
    let decoded = mornlea_protocol::LoginReject::decode(&payload).expect("decode");
    assert_eq!(decoded, reject);
    assert_eq!(decoded.code, 2);
    assert_eq!(decoded.message, "no");
}

#[test]
fn login_reject_round_trip_preserves_empty_message_codes() {
    for (code, wire) in [
        (1u8, [0x01, 0x00]),
        (2, [0x02, 0x00]),
        (3, [0x03, 0x00]),
        (4, [0x04, 0x00]),
        (5, [0x05, 0x00]),
        (6, [0x06, 0x00]),
        (7, [0x07, 0x00]),
    ] {
        let reject = mornlea_protocol::LoginReject::new(code, "").expect("empty message");
        let payload = reject.encode();
        assert_eq!(payload, wire, "code {code}");
        let decoded = mornlea_protocol::LoginReject::decode(&payload).expect("decode");
        assert_eq!(decoded, reject);
        assert_eq!(decoded.code, code);
        assert_eq!(decoded.message, "");
    }
}

#[test]
fn login_reject_rejects_unknown_code_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::LoginReject::new(0, ""),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::LoginReject::new(8, ""),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::LoginReject::decode(&[0x00, 0x00]),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert!(mornlea_protocol::LoginReject::decode(&[]).is_err());
    assert_eq!(
        mornlea_protocol::LoginReject::decode(&[0x02, 0x02, b'n', b'o', 0x00]),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    assert_eq!(
        mornlea_protocol::LoginReject::decode(&[0x02, 0x01, 0xff]),
        Err(mornlea_protocol::ProtocolError::InvalidString)
    );
    let mut oversized = vec![0x02];
    oversized.extend(mornlea_protocol::encode_uvarint(257));
    oversized.extend(std::iter::repeat_n(b'a', 257));
    assert_eq!(
        mornlea_protocol::LoginReject::decode(&oversized),
        Err(mornlea_protocol::ProtocolError::InvalidString)
    );
}

#[test]
fn disconnect_round_trip_preserves_golden_bytes() {
    let disconnect = mornlea_protocol::Disconnect::new(2, "bye").expect("disconnect");
    let payload = disconnect.encode();
    assert_eq!(payload, [0x02, 0x03, b'b', b'y', b'e']);
    assert_eq!(mornlea_protocol::Disconnect::PACKET_ID, 6);
    let decoded = mornlea_protocol::Disconnect::decode(&payload).expect("decode");
    assert_eq!(decoded, disconnect);
    assert_eq!(decoded.code, 2);
    assert_eq!(decoded.message, "bye");
}

#[test]
fn disconnect_round_trip_preserves_empty_message_codes() {
    for (code, wire) in [
        (1u8, [0x01, 0x00]),
        (2, [0x02, 0x00]),
        (3, [0x03, 0x00]),
        (4, [0x04, 0x00]),
        (5, [0x05, 0x00]),
    ] {
        let disconnect = mornlea_protocol::Disconnect::new(code, "").expect("empty message");
        let payload = disconnect.encode();
        assert_eq!(payload, wire, "code {code}");
        let decoded = mornlea_protocol::Disconnect::decode(&payload).expect("decode");
        assert_eq!(decoded, disconnect);
        assert_eq!(decoded.code, code);
        assert_eq!(decoded.message, "");
    }
}

#[test]
fn disconnect_rejects_unknown_code_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::Disconnect::new(0, ""),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::Disconnect::new(6, ""),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::Disconnect::decode(&[0x00, 0x00]),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert!(mornlea_protocol::Disconnect::decode(&[]).is_err());
    assert_eq!(
        mornlea_protocol::Disconnect::decode(&[0x02, 0x03, b'b', b'y', b'e', 0x00]),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    assert_eq!(
        mornlea_protocol::Disconnect::decode(&[0x02, 0x01, 0xff]),
        Err(mornlea_protocol::ProtocolError::InvalidString)
    );
    let mut oversized = vec![0x02];
    oversized.extend(mornlea_protocol::encode_uvarint(257));
    oversized.extend(std::iter::repeat_n(b'a', 257));
    assert_eq!(
        mornlea_protocol::Disconnect::decode(&oversized),
        Err(mornlea_protocol::ProtocolError::InvalidString)
    );
}

#[test]
fn keep_alive_round_trip_preserves_golden_bytes() {
    let keep_alive = mornlea_protocol::KeepAlive::new(8).expect("keep alive");
    let payload = keep_alive.encode();
    assert_eq!(payload, [0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]);
    assert_eq!(mornlea_protocol::KeepAlive::PACKET_ID, 5);
    let decoded = mornlea_protocol::KeepAlive::decode(&payload).expect("decode");
    assert_eq!(decoded, keep_alive);
    assert_eq!(decoded.token, 8);
}

#[test]
fn keep_alive_rejects_zero_token_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::KeepAlive::new(0),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::KeepAlive::decode(&[0; 8]),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::KeepAlive::decode(&[0x08]).is_err());
    let mut trailing = vec![0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::KeepAlive::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn keep_alive_reply_round_trip_preserves_golden_bytes() {
    let reply = mornlea_protocol::KeepAliveReply::new(6).expect("keep alive reply");
    let payload = reply.encode();
    assert_eq!(payload, [0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]);
    assert_eq!(mornlea_protocol::KeepAliveReply::PACKET_ID, 4);
    let decoded = mornlea_protocol::KeepAliveReply::decode(&payload).expect("decode");
    assert_eq!(decoded, reply);
    assert_eq!(decoded.token, 6);
}

#[test]
fn keep_alive_reply_rejects_zero_token_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::KeepAliveReply::new(0),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::KeepAliveReply::decode(&[0; 8]),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::KeepAliveReply::decode(&[0x06]).is_err());
    let mut trailing = vec![0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::KeepAliveReply::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn place_block_succeeded_round_trip_preserves_golden_bytes() {
    let ack = mornlea_protocol::PlaceBlockSucceeded::new(0x1122_3344_5566_7788);
    let payload = ack.encode();
    assert_eq!(payload, [0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11]);
    assert_eq!(mornlea_protocol::PlaceBlockSucceeded::PACKET_ID, 20);
    let decoded = mornlea_protocol::PlaceBlockSucceeded::decode(&payload).expect("decode");
    assert_eq!(decoded, ack);
    assert_eq!(decoded.sequence, 0x1122_3344_5566_7788);
}

#[test]
fn place_block_succeeded_rejects_malformed_payload_and_accepts_zero_sequence() {
    assert!(mornlea_protocol::PlaceBlockSucceeded::decode(&[0x88]).is_err());
    let mut trailing = vec![0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::PlaceBlockSucceeded::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let zero = mornlea_protocol::PlaceBlockSucceeded::new(0);
    assert_eq!(zero.sequence, 0);
    assert_eq!(
        mornlea_protocol::PlaceBlockSucceeded::decode(&zero.encode()).expect("decode zero"),
        zero
    );
}

#[test]
fn command_rejected_round_trip_preserves_golden_bytes() {
    let rejected = mornlea_protocol::CommandRejected::new(7, 6).expect("occupied");
    let payload = rejected.encode();
    assert_eq!(
        payload,
        [0x07, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x06]
    );
    assert_eq!(mornlea_protocol::CommandRejected::PACKET_ID, 4);
    let decoded = mornlea_protocol::CommandRejected::decode(&payload).expect("decode");
    assert_eq!(decoded, rejected);
    assert_eq!(decoded.sequence, 7);
    assert_eq!(decoded.reason, 6);
}

#[test]
fn command_rejected_round_trip_preserves_frozen_reason_ids() {
    for reason in 1u8..=15 {
        let rejected = mornlea_protocol::CommandRejected::new(1, reason).expect("reason");
        let mut want = vec![0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, reason];
        let payload = rejected.encode();
        assert_eq!(payload, want, "reason {reason}");
        let decoded = mornlea_protocol::CommandRejected::decode(&payload).expect("decode");
        assert_eq!(decoded.reason, reason);
        want.push(0x00);
        assert_eq!(
            mornlea_protocol::CommandRejected::decode(&want),
            Err(mornlea_protocol::ProtocolError::TrailingBytes)
        );
    }
}

#[test]
fn command_rejected_rejects_unknown_reason_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::CommandRejected::new(1, 0),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::CommandRejected::new(1, 16),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::CommandRejected::decode(&[
            0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
        ]),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert!(mornlea_protocol::CommandRejected::decode(&[0x01]).is_err());
}

#[test]
fn select_hotbar_round_trip_preserves_golden_bytes() {
    let select = mornlea_protocol::SelectHotbar::new(9, 8).expect("select");
    let payload = select.encode();
    assert_eq!(
        payload,
        [0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08]
    );
    assert_eq!(mornlea_protocol::SelectHotbar::PACKET_ID, 5);
    let decoded = mornlea_protocol::SelectHotbar::decode(&payload).expect("decode");
    assert_eq!(decoded, select);
    assert_eq!(decoded.sequence, 9);
    assert_eq!(decoded.slot, 8);
}

#[test]
fn select_hotbar_rejects_invalid_slot_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::SelectHotbar::new(1, 9),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::SelectHotbar::decode(&[
            0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09
        ]),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::SelectHotbar::decode(&[0x09]).is_err());
    let mut trailing = vec![0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::SelectHotbar::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let first = mornlea_protocol::SelectHotbar::new(0, 0).expect("slot zero");
    assert_eq!(first.slot, 0);
    assert_eq!(
        mornlea_protocol::SelectHotbar::decode(&first.encode()).expect("decode slot zero"),
        first
    );
}

#[test]
fn drop_selected_item_round_trip_preserves_golden_bytes() {
    let drop = mornlea_protocol::DropSelectedItem::new(0x1122_3344_5566_7788);
    let payload = drop.encode();
    assert_eq!(payload, [0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11]);
    assert_eq!(mornlea_protocol::DropSelectedItem::PACKET_ID, 11);
    let decoded = mornlea_protocol::DropSelectedItem::decode(&payload).expect("decode");
    assert_eq!(decoded, drop);
    assert_eq!(decoded.sequence, 0x1122_3344_5566_7788);
}

#[test]
fn drop_selected_item_rejects_malformed_payload_and_accepts_zero_sequence() {
    assert!(mornlea_protocol::DropSelectedItem::decode(&[0x88]).is_err());
    let mut trailing = vec![0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::DropSelectedItem::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let zero = mornlea_protocol::DropSelectedItem::new(0);
    assert_eq!(zero.sequence, 0);
    assert_eq!(
        mornlea_protocol::DropSelectedItem::decode(&zero.encode()).expect("decode zero"),
        zero
    );
}

#[test]
fn equip_armor_round_trip_preserves_golden_bytes() {
    let equip = mornlea_protocol::EquipArmor::new(18);
    let payload = equip.encode();
    assert_eq!(payload, [0x12, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]);
    assert_eq!(mornlea_protocol::EquipArmor::PACKET_ID, 18);
    let decoded = mornlea_protocol::EquipArmor::decode(&payload).expect("decode");
    assert_eq!(decoded, equip);
    assert_eq!(decoded.sequence, 18);
}

#[test]
fn equip_armor_rejects_malformed_payload_and_accepts_zero_sequence() {
    assert!(mornlea_protocol::EquipArmor::decode(&[0x12]).is_err());
    let mut trailing = vec![0x12, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::EquipArmor::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let zero = mornlea_protocol::EquipArmor::new(0);
    assert_eq!(zero.sequence, 0);
    assert_eq!(
        mornlea_protocol::EquipArmor::decode(&zero.encode()).expect("decode zero"),
        zero
    );
}

#[test]
fn take_crafting_output_round_trip_preserves_golden_bytes() {
    let take = mornlea_protocol::TakeCraftingOutput::new(0x1122_3344_5566_7788).expect("take");
    let payload = take.encode();
    assert_eq!(payload, [0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11]);
    assert_eq!(mornlea_protocol::TakeCraftingOutput::PACKET_ID, 15);
    let decoded = mornlea_protocol::TakeCraftingOutput::decode(&payload).expect("decode");
    assert_eq!(decoded, take);
    assert_eq!(decoded.sequence, 0x1122_3344_5566_7788);
}

#[test]
fn take_crafting_output_rejects_zero_sequence_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::TakeCraftingOutput::new(0),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::TakeCraftingOutput::decode(&[
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
        ]),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::TakeCraftingOutput::decode(&[0x88]).is_err());
    let mut trailing = vec![0x88, 0x77, 0x66, 0x55, 0x44, 0x33, 0x22, 0x11];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::TakeCraftingOutput::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn move_inventory_stack_round_trip_preserves_golden_bytes() {
    let mov = mornlea_protocol::MoveInventoryStack::new(10, 3, 35).expect("move");
    let payload = mov.encode();
    assert_eq!(
        payload,
        [0x0a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x23]
    );
    assert_eq!(mornlea_protocol::MoveInventoryStack::PACKET_ID, 6);
    assert_eq!(mornlea_protocol::INVENTORY_SLOTS, 36);
    let decoded = mornlea_protocol::MoveInventoryStack::decode(&payload).expect("decode");
    assert_eq!(decoded, mov);
    assert_eq!(decoded.sequence, 10);
    assert_eq!(decoded.from, 3);
    assert_eq!(decoded.to, 35);
}

#[test]
fn move_inventory_stack_rejects_invalid_slots_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::MoveInventoryStack::new(1, 36, 0),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveInventoryStack::new(1, 0, 36),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveInventoryStack::new(1, 2, 2),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveInventoryStack::decode(&[
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 36
        ]),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::MoveInventoryStack::decode(&[0x0a]).is_err());
    let mut trailing = vec![0x0a, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x23];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::MoveInventoryStack::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let first = mornlea_protocol::MoveInventoryStack::new(0, 0, 35).expect("boundary");
    assert_eq!(first.from, 0);
    assert_eq!(first.to, 35);
    assert_eq!(
        mornlea_protocol::MoveInventoryStack::decode(&first.encode()).expect("decode boundary"),
        first
    );
}

#[test]
fn move_crafting_stack_round_trip_preserves_golden_bytes() {
    let mov = mornlea_protocol::MoveCraftingStack::new(11, 9, 0).expect("move");
    let payload = mov.encode();
    assert_eq!(
        payload,
        [0x0b, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09, 0x00]
    );
    assert_eq!(mornlea_protocol::MoveCraftingStack::PACKET_ID, 7);
    assert_eq!(mornlea_protocol::CRAFTING_GRID_SLOTS, 9);
    assert_eq!(mornlea_protocol::GRID_CRAFTING_VIEW_SLOTS, 45);
    let decoded = mornlea_protocol::MoveCraftingStack::decode(&payload).expect("decode");
    assert_eq!(decoded, mov);
    assert_eq!(decoded.sequence, 11);
    assert_eq!(decoded.from, 9);
    assert_eq!(decoded.to, 0);
}

#[test]
fn move_crafting_stack_rejects_invalid_slots_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::MoveCraftingStack::new(1, 45, 0),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveCraftingStack::new(1, 0, 45),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveCraftingStack::new(1, 2, 2),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveCraftingStack::new(1, 9, 10),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveCraftingStack::decode(&[
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 9, 10
        ]),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::MoveCraftingStack::decode(&[0x0b]).is_err());
    let mut trailing = vec![0x0b, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x09, 0x00];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::MoveCraftingStack::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let grid_to_inv = mornlea_protocol::MoveCraftingStack::new(0, 0, 44).expect("grid to inv");
    assert_eq!(grid_to_inv.from, 0);
    assert_eq!(grid_to_inv.to, 44);
    assert_eq!(
        mornlea_protocol::MoveCraftingStack::decode(&grid_to_inv.encode()).expect("decode"),
        grid_to_inv
    );
}

#[test]
fn close_container_round_trip_preserves_golden_bytes() {
    let close = mornlea_protocol::CloseContainer::new(5);
    let payload = close.encode();
    assert_eq!(payload, [0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]);
    assert_eq!(mornlea_protocol::CloseContainer::PACKET_ID, 10);
    let decoded = mornlea_protocol::CloseContainer::decode(&payload).expect("decode");
    assert_eq!(decoded, close);
    assert_eq!(decoded.sequence, 5);
}

#[test]
fn close_container_rejects_malformed_payload_and_accepts_zero_sequence() {
    assert!(mornlea_protocol::CloseContainer::decode(&[0x05]).is_err());
    let mut trailing = vec![0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::CloseContainer::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let zero = mornlea_protocol::CloseContainer::new(0);
    assert_eq!(zero.sequence, 0);
    assert_eq!(
        mornlea_protocol::CloseContainer::decode(&zero.encode()).expect("decode zero"),
        zero
    );
}

#[test]
fn place_block_round_trip_preserves_golden_bytes() {
    let place = mornlea_protocol::PlaceBlock::new(3, 2.0, -1.0, 4).expect("place");
    let payload = place.encode();
    assert_eq!(
        payload,
        [
            0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
            0x80, 0xbf, 0x04
        ]
    );
    assert_eq!(mornlea_protocol::PlaceBlock::PACKET_ID, 2);
    let decoded = mornlea_protocol::PlaceBlock::decode(&payload).expect("decode");
    assert_eq!(decoded, place);
    assert_eq!(decoded.sequence, 3);
    assert_eq!(decoded.yaw, 2.0);
    assert_eq!(decoded.pitch, -1.0);
    assert_eq!(decoded.slot, 4);
}

#[test]
fn place_block_rejects_invalid_slot_non_finite_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::PlaceBlock::new(1, 0.0, 0.0, 9),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::PlaceBlock::new(1, f32::NAN, 0.0, 0),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert_eq!(
        mornlea_protocol::PlaceBlock::new(1, 0.0, f32::INFINITY, 0),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    let mut nan_yaw = vec![0u8; 8];
    nan_yaw.extend_from_slice(&0x7fc0_0000u32.to_le_bytes());
    nan_yaw.extend_from_slice(&0f32.to_le_bytes());
    nan_yaw.push(0);
    assert_eq!(
        mornlea_protocol::PlaceBlock::decode(&nan_yaw),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert_eq!(
        mornlea_protocol::PlaceBlock::decode(&[
            0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
            0x80, 0xbf, 0x09
        ]),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::PlaceBlock::decode(&[0x03]).is_err());
    let mut trailing = vec![
        0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x80,
        0xbf, 0x04,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::PlaceBlock::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let first = mornlea_protocol::PlaceBlock::new(0, 0.0, 0.0, 0).expect("slot zero");
    let last = mornlea_protocol::PlaceBlock::new(0, 0.0, 0.0, 8).expect("slot eight");
    assert_eq!(first.slot, 0);
    assert_eq!(last.slot, 8);
    assert_eq!(
        mornlea_protocol::PlaceBlock::decode(&first.encode()).expect("decode slot zero"),
        first
    );
}

#[test]
fn open_container_round_trip_preserves_golden_bytes() {
    let open = mornlea_protocol::OpenContainer::new(3, 1.5, -0.5).expect("open");
    let payload = open.encode();
    assert_eq!(
        payload,
        [
            0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xc0, 0x3f, 0x00, 0x00,
            0x00, 0xbf
        ]
    );
    assert_eq!(mornlea_protocol::OpenContainer::PACKET_ID, 8);
    let decoded = mornlea_protocol::OpenContainer::decode(&payload).expect("decode");
    assert_eq!(decoded, open);
    assert_eq!(decoded.sequence, 3);
    assert_eq!(decoded.yaw, 1.5);
    assert_eq!(decoded.pitch, -0.5);
}

#[test]
fn open_container_rejects_non_finite_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::OpenContainer::new(1, f32::NAN, 0.0),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert_eq!(
        mornlea_protocol::OpenContainer::new(1, 0.0, f32::NEG_INFINITY),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    let mut inf_pitch = vec![0u8; 8];
    inf_pitch.extend_from_slice(&0f32.to_le_bytes());
    inf_pitch.extend_from_slice(&0x7f80_0000u32.to_le_bytes());
    assert_eq!(
        mornlea_protocol::OpenContainer::decode(&inf_pitch),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert!(mornlea_protocol::OpenContainer::decode(&[0x03]).is_err());
    let mut trailing = vec![
        0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xc0, 0x3f, 0x00, 0x00, 0x00,
        0xbf,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::OpenContainer::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let zero = mornlea_protocol::OpenContainer::new(0, 0.0, 0.0).expect("zero");
    assert_eq!(zero.sequence, 0);
    assert_eq!(
        mornlea_protocol::OpenContainer::decode(&zero.encode()).expect("decode zero"),
        zero
    );
}

#[test]
fn request_chunk_resync_round_trip_preserves_golden_bytes() {
    let resync = mornlea_protocol::RequestChunkResync::new(
        4,
        mornlea_domain::Dimension::OVERWORLD,
        -2,
        3,
        5,
    );
    let payload = resync.encode();
    assert_eq!(
        payload,
        [
            0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff,
            0xff, 0xff, 0x03, 0x00, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
        ]
    );
    assert_eq!(mornlea_protocol::RequestChunkResync::PACKET_ID, 3);
    let decoded = mornlea_protocol::RequestChunkResync::decode(&payload).expect("decode");
    assert_eq!(decoded, resync);
    assert_eq!(decoded.chunk_x, -2);
    assert_eq!(decoded.chunk_z, 3);
    assert_eq!(decoded.have_revision, 5);
    let depths =
        mornlea_protocol::RequestChunkResync::new(0, mornlea_domain::Dimension::DEPTHS, -1, -1, 0);
    assert_eq!(
        mornlea_protocol::RequestChunkResync::decode(&depths.encode()).expect("decode depths"),
        depths
    );
}

#[test]
fn request_chunk_resync_rejects_unknown_dimension_and_malformed_payload() {
    let mut unknown = vec![0u8; 8];
    unknown.extend_from_slice(&2i32.to_le_bytes());
    unknown.extend_from_slice(&0i32.to_le_bytes());
    unknown.extend_from_slice(&0i32.to_le_bytes());
    unknown.extend_from_slice(&0u64.to_le_bytes());
    assert_eq!(
        mornlea_protocol::RequestChunkResync::decode(&unknown),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    let mut far = vec![0u8; 8];
    far.extend_from_slice(&0x0000_0100i32.to_le_bytes());
    far.extend_from_slice(&0i32.to_le_bytes());
    far.extend_from_slice(&0i32.to_le_bytes());
    far.extend_from_slice(&0u64.to_le_bytes());
    assert!(mornlea_protocol::RequestChunkResync::decode(&far).is_err());
    assert!(mornlea_protocol::RequestChunkResync::decode(&[0x04]).is_err());
    let mut trailing = vec![
        0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfe, 0xff, 0xff,
        0xff, 0x03, 0x00, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::RequestChunkResync::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn till_soil_round_trip_preserves_golden_bytes() {
    let till = mornlea_protocol::TillSoil::new(12, 2.0, -1.0).expect("till");
    let payload = till.encode();
    assert_eq!(
        payload,
        [
            0x0c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
            0x80, 0xbf
        ]
    );
    assert_eq!(mornlea_protocol::TillSoil::PACKET_ID, 13);
    let decoded = mornlea_protocol::TillSoil::decode(&payload).expect("decode");
    assert_eq!(decoded, till);
    assert_eq!(decoded.yaw, 2.0);
    assert_eq!(decoded.pitch, -1.0);
}

#[test]
fn till_soil_rejects_non_finite_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::TillSoil::new(1, f32::NAN, 0.0),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert_eq!(
        mornlea_protocol::TillSoil::new(1, 0.0, f32::NEG_INFINITY),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert!(mornlea_protocol::TillSoil::decode(&[0x0c]).is_err());
    let mut trailing = vec![
        0x0c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x80,
        0xbf,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::TillSoil::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let zero = mornlea_protocol::TillSoil::new(0, 0.0, 0.0).expect("zero");
    assert_eq!(
        mornlea_protocol::TillSoil::decode(&zero.encode()).expect("decode zero"),
        zero
    );
}

#[test]
fn bone_meal_round_trip_preserves_golden_bytes() {
    let packet = mornlea_protocol::BoneMeal::new(12, 2.0, -1.0).expect("codec");
    let payload = packet.encode();
    assert_eq!(
        payload,
        [
            0x0c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
            0x80, 0xbf
        ]
    );
    assert_eq!(mornlea_protocol::BoneMeal::PACKET_ID, 14);
    let decoded = mornlea_protocol::BoneMeal::decode(&payload).expect("decode");
    assert_eq!(decoded, packet);
    assert_eq!(decoded.sequence, 12);
    assert_eq!(decoded.yaw, 2.0);
    assert_eq!(decoded.pitch, -1.0);
}

#[test]
fn bone_meal_rejects_non_finite_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::BoneMeal::new(1, f32::NAN, 0.0),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert_eq!(
        mornlea_protocol::BoneMeal::new(1, 0.0, f32::NEG_INFINITY),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    let mut inf_pitch = vec![0u8; 8];
    inf_pitch.extend_from_slice(&0f32.to_le_bytes());
    inf_pitch.extend_from_slice(&0x7f80_0000u32.to_le_bytes());
    assert_eq!(
        mornlea_protocol::BoneMeal::decode(&inf_pitch),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert!(mornlea_protocol::BoneMeal::decode(&[0x03]).is_err());
    let mut trailing = vec![
        0x0c, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x80,
        0xbf,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::BoneMeal::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let zero = mornlea_protocol::BoneMeal::new(0, 0.0, 0.0).expect("zero");
    assert_eq!(
        mornlea_protocol::BoneMeal::decode(&zero.encode()).expect("decode zero"),
        zero
    );
}

#[test]
fn collect_water_round_trip_preserves_golden_bytes() {
    let packet = mornlea_protocol::CollectWater::new(16, 2.0, -1.0).expect("codec");
    let payload = packet.encode();
    assert_eq!(
        payload,
        [
            0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
            0x80, 0xbf
        ]
    );
    assert_eq!(mornlea_protocol::CollectWater::PACKET_ID, 16);
    let decoded = mornlea_protocol::CollectWater::decode(&payload).expect("decode");
    assert_eq!(decoded, packet);
    assert_eq!(decoded.sequence, 16);
    assert_eq!(decoded.yaw, 2.0);
    assert_eq!(decoded.pitch, -1.0);
}

#[test]
fn collect_water_rejects_non_finite_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::CollectWater::new(1, f32::NAN, 0.0),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert_eq!(
        mornlea_protocol::CollectWater::new(1, 0.0, f32::NEG_INFINITY),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    let mut inf_pitch = vec![0u8; 8];
    inf_pitch.extend_from_slice(&0f32.to_le_bytes());
    inf_pitch.extend_from_slice(&0x7f80_0000u32.to_le_bytes());
    assert_eq!(
        mornlea_protocol::CollectWater::decode(&inf_pitch),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert!(mornlea_protocol::CollectWater::decode(&[0x03]).is_err());
    let mut trailing = vec![
        0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x80,
        0xbf,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::CollectWater::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let zero = mornlea_protocol::CollectWater::new(0, 0.0, 0.0).expect("zero");
    assert_eq!(
        mornlea_protocol::CollectWater::decode(&zero.encode()).expect("decode zero"),
        zero
    );
}

#[test]
fn place_water_round_trip_preserves_golden_bytes() {
    let packet = mornlea_protocol::PlaceWater::new(17, 2.0, -1.0).expect("codec");
    let payload = packet.encode();
    assert_eq!(
        payload,
        [
            0x11, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00,
            0x80, 0xbf
        ]
    );
    assert_eq!(mornlea_protocol::PlaceWater::PACKET_ID, 17);
    let decoded = mornlea_protocol::PlaceWater::decode(&payload).expect("decode");
    assert_eq!(decoded, packet);
    assert_eq!(decoded.sequence, 17);
    assert_eq!(decoded.yaw, 2.0);
    assert_eq!(decoded.pitch, -1.0);
}

#[test]
fn place_water_rejects_non_finite_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::PlaceWater::new(1, f32::NAN, 0.0),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert_eq!(
        mornlea_protocol::PlaceWater::new(1, 0.0, f32::NEG_INFINITY),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    let mut inf_pitch = vec![0u8; 8];
    inf_pitch.extend_from_slice(&0f32.to_le_bytes());
    inf_pitch.extend_from_slice(&0x7f80_0000u32.to_le_bytes());
    assert_eq!(
        mornlea_protocol::PlaceWater::decode(&inf_pitch),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert!(mornlea_protocol::PlaceWater::decode(&[0x03]).is_err());
    let mut trailing = vec![
        0x11, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x00, 0x00, 0x80,
        0xbf,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::PlaceWater::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let zero = mornlea_protocol::PlaceWater::new(0, 0.0, 0.0).expect("zero");
    assert_eq!(
        mornlea_protocol::PlaceWater::decode(&zero.encode()).expect("decode zero"),
        zero
    );
}

#[test]
fn move_container_stack_round_trip_preserves_golden_bytes() {
    let container = mornlea_protocol::ContainerRef::new(0, -3, 7, 0, 5, 9).expect("container");
    let move_stack = mornlea_protocol::MoveContainerStack::new(4, container, 0, 36).expect("move");
    let payload = move_stack.encode();
    assert_eq!(
        payload,
        [
            0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfd, 0xff,
            0xff, 0xff, 0x07, 0x00, 0x00, 0x00, 0x00, 0x05, 0x09, 0x00, 0x00, 0x00, 0x00, 0x24
        ]
    );
    assert_eq!(mornlea_protocol::MoveContainerStack::PACKET_ID, 9);
    assert_eq!(payload.len(), 28);
    let decoded = mornlea_protocol::MoveContainerStack::decode(&payload).expect("decode");
    assert_eq!(decoded, move_stack);
    assert_eq!(decoded.container, container);
    assert_eq!(decoded.from, 0);
    assert_eq!(decoded.to, 36);
    let chest = mornlea_protocol::ContainerRef::new(0, 1, -2, 1, 3, 4).expect("chest");
    let chest_move =
        mornlea_protocol::MoveContainerStack::new(5, chest, 62, 3).expect("chest move");
    assert_eq!(
        mornlea_protocol::MoveContainerStack::decode(&chest_move.encode()).expect("decode chest"),
        chest_move
    );
}

#[test]
fn move_container_stack_rejects_invalid_container_and_malformed_payload() {
    let container = mornlea_protocol::ContainerRef::new(0, -3, 7, 0, 5, 9).expect("container");
    assert_eq!(
        mornlea_protocol::MoveContainerStack::new(1, container, 0, 0),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveContainerStack::new(1, container, 0, 38),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveContainerStack::new(1, container, 39, 0),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    let chest = mornlea_protocol::ContainerRef::new(0, 1, -2, 1, 3, 4).expect("chest");
    assert_eq!(
        mornlea_protocol::MoveContainerStack::new(1, chest, 0, 63),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::ContainerRef::new(0, 1, 2, 2, 0, 1),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::ContainerRef::new(0, 1, 2, 0, 32, 1),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::ContainerRef::new(0, 1, 2, 1, 16, 1),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::ContainerRef::new(0, 1, 2, 0, 0, 0),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::ContainerRef::new(1, 1, 2, 0, 0, 1),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::MoveContainerStack::decode(&[0x04]).is_err());
    let mut trailing = vec![
        0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfd, 0xff, 0xff,
        0xff, 0x07, 0x00, 0x00, 0x00, 0x00, 0x05, 0x09, 0x00, 0x00, 0x00, 0x00, 0x24,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::MoveContainerStack::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn move_stack_partial_round_trip_preserves_golden_bytes() {
    let container = mornlea_protocol::ContainerRef::new(0, -3, 7, 1, 5, 9).expect("chest");
    let partial =
        mornlea_protocol::MoveStackPartial::new(19, container, 2, 10, 34, true).expect("partial");
    let payload = partial.encode();
    assert_eq!(
        payload,
        [
            0x13, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfd, 0xff,
            0xff, 0xff, 0x07, 0x00, 0x00, 0x00, 0x01, 0x05, 0x09, 0x00, 0x00, 0x00, 0x02, 0x0a,
            0x22, 0x01
        ]
    );
    assert_eq!(mornlea_protocol::MoveStackPartial::PACKET_ID, 19);
    assert_eq!(payload.len(), 30);
    let decoded = mornlea_protocol::MoveStackPartial::decode(&payload).expect("decode");
    assert_eq!(decoded, partial);
    assert!(decoded.single);
    assert_eq!(decoded.view, 2);
    let inventory = mornlea_protocol::MoveStackPartial::new(
        7,
        mornlea_protocol::ContainerRef::NONE,
        mornlea_protocol::STACK_VIEW_INVENTORY,
        0,
        35,
        false,
    )
    .expect("inventory split");
    assert_eq!(
        mornlea_protocol::MoveStackPartial::decode(&inventory.encode()).expect("decode inventory"),
        inventory
    );
}

#[test]
fn move_stack_partial_rejects_invalid_view_and_malformed_payload() {
    let container = mornlea_protocol::ContainerRef::new(0, -3, 7, 1, 5, 9).expect("chest");
    assert_eq!(
        mornlea_protocol::MoveStackPartial::new(1, container, 2, 0, 0, true),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveStackPartial::new(1, container, 2, 0, 63, true),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveStackPartial::new(
            1,
            mornlea_protocol::ContainerRef::NONE,
            0,
            0,
            36,
            true
        ),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveStackPartial::new(1, container, 0, 0, 1, true),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveStackPartial::new(
            1,
            mornlea_protocol::ContainerRef::NONE,
            1,
            0,
            45,
            true
        ),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::MoveStackPartial::new(
            1,
            mornlea_protocol::ContainerRef::NONE,
            3,
            0,
            1,
            true
        ),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    let mut bad_single = vec![
        0x13, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfd, 0xff, 0xff,
        0xff, 0x07, 0x00, 0x00, 0x00, 0x01, 0x05, 0x09, 0x00, 0x00, 0x00, 0x02, 0x0a, 0x22, 0x02,
    ];
    assert_eq!(
        mornlea_protocol::MoveStackPartial::decode(&bad_single),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert!(mornlea_protocol::MoveStackPartial::decode(&[0x13]).is_err());
    bad_single.truncate(bad_single.len() - 1);
    assert!(mornlea_protocol::MoveStackPartial::decode(&bad_single).is_err());
    let mut trailing = vec![
        0x13, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfd, 0xff, 0xff,
        0xff, 0x07, 0x00, 0x00, 0x00, 0x01, 0x05, 0x09, 0x00, 0x00, 0x00, 0x02, 0x0a, 0x22, 0x01,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::MoveStackPartial::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn quick_move_stack_round_trip_preserves_golden_bytes() {
    let container = mornlea_protocol::ContainerRef::new(0, 4, -2, 0, 3, 5).expect("furnace");
    let quick = mornlea_protocol::QuickMoveStack::new(20, container, 2, 38).expect("quick");
    let payload = quick.encode();
    assert_eq!(
        payload,
        [
            0x14, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x00,
            0x00, 0x00, 0xfe, 0xff, 0xff, 0xff, 0x00, 0x03, 0x05, 0x00, 0x00, 0x00, 0x02, 0x26
        ]
    );
    assert_eq!(mornlea_protocol::QuickMoveStack::PACKET_ID, 20);
    assert_eq!(payload.len(), 28);
    let decoded = mornlea_protocol::QuickMoveStack::decode(&payload).expect("decode");
    assert_eq!(decoded, quick);
    assert_eq!(decoded.from, 38);
    let crafting = mornlea_protocol::QuickMoveStack::new(
        8,
        mornlea_protocol::ContainerRef::NONE,
        mornlea_protocol::STACK_VIEW_CRAFTING,
        44,
    )
    .expect("crafting quick");
    assert_eq!(
        mornlea_protocol::QuickMoveStack::decode(&crafting.encode()).expect("decode crafting"),
        crafting
    );
}

#[test]
fn quick_move_stack_rejects_invalid_view_and_malformed_payload() {
    let container = mornlea_protocol::ContainerRef::new(0, 4, -2, 0, 3, 5).expect("furnace");
    assert_eq!(
        mornlea_protocol::QuickMoveStack::new(1, container, 2, 39),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::QuickMoveStack::new(1, mornlea_protocol::ContainerRef::NONE, 0, 36),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::QuickMoveStack::new(1, mornlea_protocol::ContainerRef::NONE, 1, 45),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::QuickMoveStack::new(1, mornlea_protocol::ContainerRef::NONE, 9, 0),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::QuickMoveStack::new(1, container, 0, 0),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::QuickMoveStack::decode(&[0x14]).is_err());
    let mut trailing = vec![
        0x14, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00,
        0x00, 0xfe, 0xff, 0xff, 0xff, 0x00, 0x03, 0x05, 0x00, 0x00, 0x00, 0x02, 0x26,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::QuickMoveStack::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn drop_stack_round_trip_preserves_golden_bytes() {
    let container = mornlea_protocol::ContainerRef::new(0, -3, 7, 1, 5, 9).expect("chest");
    let drop = mornlea_protocol::DropStack::new(21, container, 2, 62).expect("drop");
    let payload = drop.encode();
    assert_eq!(
        payload,
        [
            0x15, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfd, 0xff,
            0xff, 0xff, 0x07, 0x00, 0x00, 0x00, 0x01, 0x05, 0x09, 0x00, 0x00, 0x00, 0x02, 0x3e
        ]
    );
    assert_eq!(mornlea_protocol::DropStack::PACKET_ID, 21);
    assert_eq!(payload.len(), 28);
    let decoded = mornlea_protocol::DropStack::decode(&payload).expect("decode");
    assert_eq!(decoded, drop);
    assert_eq!(decoded.slot, 62);
    assert_eq!(decoded.view, 2);
    let inventory =
        mornlea_protocol::DropStack::new(1, mornlea_protocol::ContainerRef::NONE, 0, 35)
            .expect("drop");
    assert_eq!(
        mornlea_protocol::DropStack::decode(&inventory.encode()).expect("decode inventory"),
        inventory
    );
}

#[test]
fn drop_stack_rejects_invalid_view_and_malformed_payload() {
    let container = mornlea_protocol::ContainerRef::new(0, -3, 7, 1, 5, 9).expect("chest");
    assert_eq!(
        mornlea_protocol::DropStack::new(1, container, 2, 63),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::DropStack::new(1, mornlea_protocol::ContainerRef::NONE, 0, 36),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::DropStack::new(1, mornlea_protocol::ContainerRef::NONE, 1, 45),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::DropStack::new(1, mornlea_protocol::ContainerRef::NONE, 7, 0),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::DropStack::new(1, container, 1, 0),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::DropStack::decode(&[0x15]).is_err());
    let mut trailing = vec![
        0x15, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfd, 0xff, 0xff,
        0xff, 0x07, 0x00, 0x00, 0x00, 0x01, 0x05, 0x09, 0x00, 0x00, 0x00, 0x02, 0x3e,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::DropStack::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn chat_command_round_trip_preserves_golden_bytes() {
    let command = mornlea_protocol::ChatCommand::new("chop oak".to_owned()).expect("command");
    let payload = command.encode();
    assert_eq!(
        payload,
        [0x08, 0x63, 0x68, 0x6f, 0x70, 0x20, 0x6f, 0x61, 0x6b]
    );
    assert_eq!(mornlea_protocol::ChatCommand::PACKET_ID, 12);
    let decoded = mornlea_protocol::ChatCommand::decode(&payload).expect("decode");
    assert_eq!(decoded, command);
    assert_eq!(decoded.text, "chop oak");
}

#[test]
fn chat_command_rejects_blank_control_and_malformed_payload() {
    assert!(mornlea_protocol::ChatCommand::new(String::new()).is_err());
    assert!(mornlea_protocol::ChatCommand::new("  ".to_owned()).is_err());
    assert!(mornlea_protocol::ChatCommand::new(" chop".to_owned()).is_err());
    assert!(mornlea_protocol::ChatCommand::new("chop ".to_owned()).is_err());
    assert!(mornlea_protocol::ChatCommand::new("ch\u{7}op".to_owned()).is_err());
    let oversized = "a".repeat(mornlea_protocol::CHAT_COMMAND_TEXT_MAX_BYTES + 1);
    assert!(mornlea_protocol::ChatCommand::new(oversized).is_err());
    let maximum = "a".repeat(mornlea_protocol::CHAT_COMMAND_TEXT_MAX_BYTES);
    assert_eq!(
        mornlea_protocol::ChatCommand::decode(
            &mornlea_protocol::ChatCommand::new(maximum.clone())
                .expect("maximum")
                .encode()
        )
        .expect("decode maximum")
        .text,
        maximum
    );
    assert_eq!(
        mornlea_protocol::ChatCommand::decode(&[0x00]),
        Err(mornlea_protocol::ProtocolError::InvalidString)
    );
    assert_eq!(
        mornlea_protocol::ChatCommand::decode(&[0x08, 0x63, 0x68, 0x6f]),
        Err(mornlea_protocol::ProtocolError::InvalidString)
    );
    assert_eq!(
        mornlea_protocol::ChatCommand::decode(&[
            0x08, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff
        ]),
        Err(mornlea_protocol::ProtocolError::InvalidString)
    );
    let mut trailing = vec![0x08, 0x63, 0x68, 0x6f, 0x70, 0x20, 0x6f, 0x61, 0x6b];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::ChatCommand::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn player_input_round_trip_preserves_golden_bytes() {
    let input =
        mornlea_protocol::PlayerInput::new(1, -1, 1, true, 1.5, -0.5, true, false, false, false)
            .expect("input");
    let payload = input.encode();
    assert_eq!(
        payload,
        [
            0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0x01, 0x01, 0x00, 0x00, 0xc0,
            0x3f, 0x00, 0x00, 0x00, 0xbf, 0x01, 0x00, 0x00, 0x00
        ]
    );
    assert_eq!(mornlea_protocol::PlayerInput::PACKET_ID, 0);
    let decoded = mornlea_protocol::PlayerInput::decode(&payload).expect("decode");
    assert_eq!(decoded, input);
    assert_eq!(decoded.move_x, -1);
    assert_eq!(decoded.move_z, 1);
    assert!(decoded.jump);
    assert!(decoded.mining);
    assert!(!decoded.eating);
    assert!(!decoded.sprinting);
    assert!(!decoded.sneaking);
    let eating =
        mornlea_protocol::PlayerInput::new(2, 0, 0, false, 0.0, 0.0, false, true, false, false)
            .expect("eating input");
    assert_eq!(
        mornlea_protocol::PlayerInput::decode(&eating.encode()).expect("decode eating"),
        eating
    );
}

#[test]
fn player_input_rejects_non_finite_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::PlayerInput::new(
            1,
            0,
            0,
            false,
            f32::NAN,
            0.0,
            false,
            false,
            false,
            false
        ),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert_eq!(
        mornlea_protocol::PlayerInput::new(
            1,
            0,
            0,
            false,
            0.0,
            f32::INFINITY,
            false,
            false,
            false,
            false
        ),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    let mut nan_yaw = vec![0u8; 8];
    nan_yaw.extend_from_slice(&0i8.to_le_bytes());
    nan_yaw.extend_from_slice(&0i8.to_le_bytes());
    nan_yaw.push(0);
    nan_yaw.extend_from_slice(&0x7fc0_0000u32.to_le_bytes());
    nan_yaw.extend_from_slice(&0f32.to_le_bytes());
    nan_yaw.extend_from_slice(&[0, 0, 0, 0]);
    assert_eq!(
        mornlea_protocol::PlayerInput::decode(&nan_yaw),
        Err(mornlea_protocol::ProtocolError::InvalidFloat)
    );
    assert!(mornlea_protocol::PlayerInput::decode(&[0x01]).is_err());
    let mut trailing = vec![
        0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0x01, 0x01, 0x00, 0x00, 0xc0, 0x3f,
        0x00, 0x00, 0x00, 0xbf, 0x01, 0x00, 0x00, 0x00,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::PlayerInput::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let mut bad_flag = vec![
        0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    ];
    assert_eq!(
        mornlea_protocol::PlayerInput::decode(&bad_flag),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    bad_flag.truncate(bad_flag.len() - 1);
    assert!(mornlea_protocol::PlayerInput::decode(&bad_flag).is_err());
}

#[test]
fn combat_hit_round_trip_preserves_golden_bytes() {
    let hit = mornlea_protocol::CombatHit::new(0x0102_0304_0506_0708, 6, 2).expect("hit");
    let payload = hit.encode();
    assert_eq!(
        payload,
        [0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0x06, 0x02]
    );
    assert_eq!(mornlea_protocol::CombatHit::PACKET_ID, 25);
    assert_eq!(payload.len(), 10);
    let decoded = mornlea_protocol::CombatHit::decode(&payload).expect("decode");
    assert_eq!(decoded, hit);
    assert_eq!(decoded.damage, 6);
    assert_eq!(decoded.target_kind, 2);
}

#[test]
fn combat_hit_rejects_invalid_range_and_malformed_payload() {
    assert_eq!(
        mornlea_protocol::CombatHit::new(0, 6, 2),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::CombatHit::new(1, 0, 2),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::CombatHit::new(1, mornlea_protocol::MAX_HEALTH + 1, 2),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::CombatHit::new(1, 6, 0),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    assert_eq!(
        mornlea_protocol::CombatHit::new(1, 6, 4),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    for kind in [
        mornlea_protocol::COMBAT_TARGET_PLAYER,
        mornlea_protocol::COMBAT_TARGET_HOSTILE,
        mornlea_protocol::COMBAT_TARGET_PASSIVE,
    ] {
        let full = mornlea_protocol::CombatHit::new(1, 1, kind).expect("kind");
        assert_eq!(
            mornlea_protocol::CombatHit::decode(&full.encode()).expect("decode full"),
            full
        );
    }
    assert!(
        mornlea_protocol::CombatHit::decode(&[
            0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0x06
        ])
        .is_err()
    );
    let mut trailing = vec![0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0x06, 0x02];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::CombatHit::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn remote_player_despawn_round_trip_preserves_golden_bytes() {
    let player = mornlea_protocol::PlayerId::new([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ])
    .expect("player");
    let despawn = mornlea_protocol::RemotePlayerDespawn::new(player);
    let payload = despawn.encode();
    assert_eq!(
        payload,
        [
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff
        ]
    );
    assert_eq!(mornlea_protocol::RemotePlayerDespawn::PACKET_ID, 8);
    assert_eq!(payload.len(), 16);
    let decoded = mornlea_protocol::RemotePlayerDespawn::decode(&payload).expect("decode");
    assert_eq!(decoded, despawn);
}

#[test]
fn remote_player_despawn_rejects_invalid_identity_and_malformed_payload() {
    let mut not_v4 = [
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ];
    not_v4[6] = 0x06;
    assert_eq!(
        mornlea_protocol::RemotePlayerDespawn::decode(&not_v4),
        Err(mornlea_protocol::ProtocolError::InvalidIdentity)
    );
    assert_eq!(
        mornlea_protocol::RemotePlayerDespawn::decode(&[0u8; 16]),
        Err(mornlea_protocol::ProtocolError::InvalidIdentity)
    );
    assert!(
        mornlea_protocol::RemotePlayerDespawn::decode(&[
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee
        ])
        .is_err()
    );
    let mut trailing = vec![
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::RemotePlayerDespawn::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

fn read_manifest(dir: &str) -> String {
    fs::read_to_string(PathBuf::from(dir).join("Cargo.toml")).expect("crate manifest")
}

fn read_inventory() -> String {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../../../testdata/runtime-migration/contracts.json");
    fs::read_to_string(&path).unwrap_or_else(|err| panic!("read {}: {err}", path.display()))
}

fn production_dependency_keys(manifest: &str) -> Vec<String> {
    let Some(rest) = manifest.split("[dependencies]\n").nth(1) else {
        return Vec::new();
    };
    let section = rest.split("\n[").next().unwrap_or(rest);
    section
        .lines()
        .filter_map(|line| {
            let line = line.trim();
            if line.is_empty() || line.starts_with('#') {
                return None;
            }
            line.split('=')
                .next()
                .map(str::trim)
                .filter(|key| !key.is_empty())
                .map(str::to_string)
        })
        .collect()
}

#[test]
fn block_changes_round_trip_preserves_golden_bytes() {
    let changes = mornlea_protocol::BlockChanges::new(
        mornlea_domain::Dimension::OVERWORLD,
        1,
        -1,
        1,
        2,
        vec![mornlea_protocol::BlockChange {
            x: 16,
            y: -64,
            z: -1,
            block: 2,
        }],
    )
    .expect("changes");
    let payload = changes.encode();
    assert_eq!(
        payload,
        [
            0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x01, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x01, 0x10, 0x00, 0x00, 0x00, 0xc0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x02,
            0x00,
        ]
    );
    assert_eq!(mornlea_protocol::BlockChanges::PACKET_ID, 1);
    assert_eq!(payload.len(), 43);
    let decoded = mornlea_protocol::BlockChanges::decode(&payload).expect("decode");
    assert_eq!(decoded, changes);
}

#[test]
fn block_changes_rejects_invalid_revision_position_and_malformed_payload() {
    let change = mornlea_protocol::BlockChange {
        x: 16,
        y: -64,
        z: -1,
        block: 2,
    };
    let valid = mornlea_protocol::BlockChanges::new(
        mornlea_domain::Dimension::OVERWORLD,
        1,
        -1,
        1,
        2,
        vec![change],
    )
    .expect("valid");
    assert!(
        mornlea_protocol::BlockChanges::new(
            mornlea_domain::Dimension::OVERWORLD,
            1,
            -1,
            0,
            1,
            vec![change],
        )
        .is_err()
    );
    assert!(
        mornlea_protocol::BlockChanges::new(
            mornlea_domain::Dimension::OVERWORLD,
            1,
            -1,
            1,
            3,
            vec![change],
        )
        .is_err()
    );
    assert!(
        mornlea_protocol::BlockChanges::new(
            mornlea_domain::Dimension::OVERWORLD,
            1,
            -1,
            u64::MAX,
            0,
            vec![change],
        )
        .is_err()
    );
    // The block must stay inside the announced chunk.
    assert!(
        mornlea_protocol::BlockChanges::new(
            mornlea_domain::Dimension::OVERWORLD,
            1,
            -1,
            1,
            2,
            vec![mornlea_protocol::BlockChange {
                x: 32,
                y: -64,
                z: -1,
                block: 2,
            }],
        )
        .is_err()
    );
    // World span ends below the highest Y.
    assert!(
        mornlea_protocol::BlockChanges::new(
            mornlea_domain::Dimension::OVERWORLD,
            1,
            -1,
            1,
            2,
            vec![mornlea_protocol::BlockChange {
                x: 16,
                y: 320,
                z: -1,
                block: 2,
            }],
        )
        .is_err()
    );
    // Unregistered block numbers are rejected.
    assert!(
        mornlea_protocol::BlockChanges::new(
            mornlea_domain::Dimension::OVERWORLD,
            1,
            -1,
            1,
            2,
            vec![mornlea_protocol::BlockChange {
                x: 16,
                y: -64,
                z: -1,
                block: 90,
            }],
        )
        .is_err()
    );
    // Records must be sorted by chunk-ordered block index.
    assert!(
        mornlea_protocol::BlockChanges::new(
            mornlea_domain::Dimension::OVERWORLD,
            1,
            -1,
            1,
            2,
            vec![
                mornlea_protocol::BlockChange {
                    x: 17,
                    y: -64,
                    z: -1,
                    block: 2,
                },
                change,
            ],
        )
        .is_err()
    );
    // Truncated and trailing payloads fail before publication.
    assert!(mornlea_protocol::BlockChanges::decode(&valid.encode()[..42]).is_err());
    let mut trailing = valid.encode();
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::BlockChanges::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn forget_chunks_round_trip_preserves_golden_bytes() {
    let forget = mornlea_protocol::ForgetChunks::new(
        mornlea_domain::Dimension::OVERWORLD,
        vec![(1, -1), (2, 3)],
    )
    .expect("forget");
    let payload = forget.encode();
    assert_eq!(
        payload,
        [
            0x00, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x02,
            0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00,
        ]
    );
    assert_eq!(mornlea_protocol::ForgetChunks::PACKET_ID, 2);
    assert_eq!(payload.len(), 21);
    let decoded = mornlea_protocol::ForgetChunks::decode(&payload).expect("decode");
    assert_eq!(decoded, forget);
}

#[test]
fn forget_chunks_rejects_empty_duplicate_and_malformed_payload() {
    let forget = mornlea_protocol::ForgetChunks::new(
        mornlea_domain::Dimension::OVERWORLD,
        vec![(1, -1), (2, 3)],
    )
    .expect("forget");
    assert_eq!(
        mornlea_protocol::ForgetChunks::new(mornlea_domain::Dimension::OVERWORLD, Vec::new()),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(
        mornlea_protocol::ForgetChunks::new(
            mornlea_domain::Dimension::OVERWORLD,
            vec![(1, -1), (1, -1)],
        )
        .is_err()
    );
    assert!(
        mornlea_protocol::ForgetChunks::new(
            mornlea_domain::Dimension::DEPTHS,
            vec![(1, -1), (2, 3)],
        )
        .is_ok()
    );
    // A truncated batch never publishes partial coordinates.
    assert!(mornlea_protocol::ForgetChunks::decode(&forget.encode()[..17]).is_err());
    // A zero count carries no observable meaning.
    assert!(mornlea_protocol::ForgetChunks::decode(&[0, 0, 0, 0, 0]).is_err());
    let mut trailing = forget.encode();
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::ForgetChunks::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn companion_despawn_round_trip_preserves_identity_bytes() {
    let companion = mornlea_protocol::CompanionId::new([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ])
    .expect("companion");
    let despawn = mornlea_protocol::CompanionDespawn::new(companion);
    let payload = despawn.encode();
    assert_eq!(
        payload,
        [
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff
        ]
    );
    assert_eq!(mornlea_protocol::CompanionDespawn::PACKET_ID, 19);
    assert_eq!(payload.len(), 16);
    assert_eq!(
        mornlea_protocol::CompanionDespawn::decode(&payload).expect("decode"),
        despawn
    );
}

#[test]
fn companion_despawn_rejects_invalid_identity_and_malformed_payload() {
    let mut not_v4 = [
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ];
    not_v4[6] = 0x06;
    assert_eq!(
        mornlea_protocol::CompanionDespawn::decode(&not_v4),
        Err(mornlea_protocol::ProtocolError::InvalidIdentity)
    );
    assert!(mornlea_protocol::CompanionDespawn::decode(&[0u8; 16]).is_err());
    assert!(mornlea_protocol::CompanionDespawn::decode(&[0u8; 15]).is_err());
    let mut trailing = vec![
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ];
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::CompanionDespawn::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn hostile_despawn_round_trip_preserves_batch_bytes() {
    let despawn = mornlea_protocol::HostileDespawn::new(0x0102_0304_0506_0708, vec![7, 9, 12])
        .expect("batch");
    let payload = despawn.encode();
    assert_eq!(
        payload,
        [
            0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0x03, 0x07, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0c, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00,
        ]
    );
    assert_eq!(mornlea_protocol::HostileDespawn::PACKET_ID, 24);
    assert_eq!(payload.len(), 9 + 3 * 8);
    assert_eq!(
        mornlea_protocol::HostileDespawn::decode(&payload).expect("decode"),
        despawn
    );
}

#[test]
fn hostile_despawn_rejects_unsorted_zero_and_malformed_payload() {
    assert!(mornlea_protocol::HostileDespawn::new(1, Vec::new()).is_err());
    assert!(mornlea_protocol::HostileDespawn::new(1, vec![0]).is_err());
    assert!(mornlea_protocol::HostileDespawn::new(1, vec![7, 7]).is_err());
    assert!(mornlea_protocol::HostileDespawn::new(1, vec![9, 7]).is_err());
    let mut over = Vec::new();
    for id in 1..=mornlea_protocol::MAX_HOSTILE_RECORDS {
        over.push(u64::from(id));
    }
    over.push(u64::from(mornlea_protocol::MAX_HOSTILE_RECORDS) + 1);
    assert!(mornlea_protocol::HostileDespawn::new(1, over).is_err());
    // A payload whose length disagrees with the count is rejected whole.
    let mut short = vec![1, 0, 0, 0, 0, 0, 0, 0, 0x02];
    short.extend_from_slice(&[7, 0, 0, 0, 0, 0, 0, 0]);
    assert!(mornlea_protocol::HostileDespawn::decode(&short).is_err());
    // A payload longer than the declared records fails the exact-length
    // check before any record is published.
    let padded = vec![1, 0, 0, 0, 0, 0, 0, 0, 0x01, 7, 0, 0, 0, 0, 0, 0, 0, 0];
    assert_eq!(
        mornlea_protocol::HostileDespawn::decode(&padded),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let exact = vec![1, 0, 0, 0, 0, 0, 0, 0, 0x01, 7, 0, 0, 0, 0, 0, 0, 0];
    assert!(mornlea_protocol::HostileDespawn::decode(&exact).is_ok());
    assert!(mornlea_protocol::HostileDespawn::decode(&[1, 0, 0, 0, 0, 0, 0, 0, 0]).is_err());
}

#[test]
fn projectile_despawn_round_trip_preserves_batch_bytes() {
    let despawn = mornlea_protocol::ProjectileDespawn::new(0x0102_0304_0506_0708, vec![3, 200])
        .expect("batch");
    let payload = despawn.encode();
    assert_eq!(
        payload,
        [
            0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0x02, 0x03, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0xc8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        ]
    );
    assert_eq!(mornlea_protocol::ProjectileDespawn::PACKET_ID, 31);
    assert_eq!(payload.len(), 9 + 2 * 8);
    assert_eq!(
        mornlea_protocol::ProjectileDespawn::decode(&payload).expect("decode"),
        despawn
    );
}

#[test]
fn projectile_despawn_rejects_unsorted_zero_and_malformed_payload() {
    assert!(mornlea_protocol::ProjectileDespawn::new(1, Vec::new()).is_err());
    assert!(mornlea_protocol::ProjectileDespawn::new(1, vec![0]).is_err());
    assert!(mornlea_protocol::ProjectileDespawn::new(1, vec![4, 4]).is_err());
    assert!(mornlea_protocol::ProjectileDespawn::new(1, vec![9, 4]).is_err());
    let over: Vec<u64> = (1..=(u64::from(mornlea_protocol::MAX_PROJECTILE_RECORDS) + 1)).collect();
    assert!(mornlea_protocol::ProjectileDespawn::new(1, over).is_err());
    let short = vec![1, 0, 0, 0, 0, 0, 0, 0, 0x02, 3, 0, 0, 0, 0, 0, 0, 0];
    assert!(mornlea_protocol::ProjectileDespawn::decode(&short).is_err());
    assert!(mornlea_protocol::ProjectileDespawn::decode(&[1, 0, 0, 0, 0, 0, 0, 0, 0]).is_err());
}

#[test]
fn passive_despawn_round_trip_preserves_batch_bytes() {
    let despawn = mornlea_protocol::PassiveDespawn::new(
        0x0102_0304_0506_0708,
        vec![
            mornlea_protocol::PassiveDespawnRecord {
                id: 4,
                reason: mornlea_protocol::PASSIVE_DESPAWN_VANISHED,
            },
            mornlea_protocol::PassiveDespawnRecord {
                id: 9,
                reason: mornlea_protocol::PASSIVE_DESPAWN_DIED,
            },
        ],
    )
    .expect("batch");
    let payload = despawn.encode();
    assert_eq!(
        payload,
        [
            0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0x02, 0x04, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
        ]
    );
    assert_eq!(mornlea_protocol::PassiveDespawn::PACKET_ID, 28);
    assert_eq!(payload.len(), 9 + 2 * 9);
    assert_eq!(
        mornlea_protocol::PassiveDespawn::decode(&payload).expect("decode"),
        despawn
    );
}

#[test]
fn passive_despawn_rejects_unsorted_zero_reason_and_malformed_payload() {
    let vanished = mornlea_protocol::PassiveDespawnRecord {
        id: 4,
        reason: mornlea_protocol::PASSIVE_DESPAWN_VANISHED,
    };
    let died = mornlea_protocol::PassiveDespawnRecord {
        id: 9,
        reason: mornlea_protocol::PASSIVE_DESPAWN_DIED,
    };
    assert!(mornlea_protocol::PassiveDespawn::new(1, Vec::new()).is_err());
    assert!(mornlea_protocol::PassiveDespawn::new(1, vec![died, vanished]).is_err());
    assert!(
        mornlea_protocol::PassiveDespawn::new(
            1,
            vec![mornlea_protocol::PassiveDespawnRecord { id: 0, reason: 1 }],
        )
        .is_err()
    );
    assert_eq!(
        mornlea_protocol::PassiveDespawn::new(
            1,
            vec![mornlea_protocol::PassiveDespawnRecord { id: 4, reason: 2 }],
        ),
        Err(mornlea_protocol::ProtocolError::InvalidEnum)
    );
    // A payload whose length disagrees with the count is rejected whole.
    let short = vec![1, 0, 0, 0, 0, 0, 0, 0, 0x02, 4, 0, 0, 0, 0, 0, 0, 0, 0];
    assert!(mornlea_protocol::PassiveDespawn::decode(&short).is_err());
    assert!(mornlea_protocol::PassiveDespawn::decode(&[1, 0, 0, 0, 0, 0, 0, 0, 0]).is_err());
}

#[test]
fn chest_state_round_trip_preserves_slot_bytes() {
    let chest = mornlea_protocol::ContainerRef::new(
        0,
        5,
        -6,
        mornlea_protocol::CONTAINER_KIND_CHEST,
        3,
        11,
    )
    .expect("chest");
    let mut items = [mornlea_protocol::ItemStack::EMPTY; mornlea_protocol::CHEST_SLOTS];
    items[0] = mornlea_protocol::ItemStack::new(1, 5, 0).expect("stone");
    items[26] = mornlea_protocol::ItemStack::new(2, 1, 0).expect("dirt");
    let state = mornlea_protocol::ChestState::new(chest, items).expect("state");
    let payload = state.encode();
    assert_eq!(mornlea_protocol::ChestState::PACKET_ID, 15);
    assert_eq!(payload.len(), 18 + mornlea_protocol::CHEST_SLOTS * 5);
    assert_eq!(&payload[18..23], &[0x01, 0x00, 0x05, 0x00, 0x00]);
    assert_eq!(
        &payload[payload.len() - 5..],
        &[0x02, 0x00, 0x01, 0x00, 0x00]
    );
    let decoded = mornlea_protocol::ChestState::decode(&payload).expect("decode");
    assert_eq!(decoded, state);
}

#[test]
fn chest_state_rejects_wrong_reference_and_malformed_payload() {
    let furnace = mornlea_protocol::ContainerRef::new(
        0,
        5,
        -6,
        mornlea_protocol::CONTAINER_KIND_FURNACE,
        3,
        11,
    )
    .expect("furnace");
    let items = [mornlea_protocol::ItemStack::EMPTY; mornlea_protocol::CHEST_SLOTS];
    // A chest state must name a chest, not a furnace.
    assert!(mornlea_protocol::ChestState::new(furnace, items).is_err());
    assert!(
        mornlea_protocol::ChestState::new(mornlea_protocol::ContainerRef::NONE, items).is_err()
    );
    let chest = mornlea_protocol::ContainerRef::new(
        0,
        5,
        -6,
        mornlea_protocol::CONTAINER_KIND_CHEST,
        3,
        11,
    )
    .expect("chest");
    let state = mornlea_protocol::ChestState::new(chest, items).expect("state");
    let payload = state.encode();
    // A registered chest reference is accepted and round-trips.
    assert_eq!(
        mornlea_protocol::ChestState::decode(&payload).expect("decode"),
        state
    );
    // Truncated and trailing payloads fail before publication.
    assert!(mornlea_protocol::ChestState::decode(&payload[..payload.len() - 1]).is_err());
    let mut trailing = payload.clone();
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::ChestState::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn furnace_state_round_trip_preserves_golden_bytes() {
    let furnace = mornlea_protocol::ContainerRef::new(
        0,
        1,
        -1,
        mornlea_protocol::CONTAINER_KIND_FURNACE,
        5,
        7,
    )
    .expect("furnace");
    let state = mornlea_protocol::FurnaceState::new(
        furnace,
        mornlea_protocol::ItemStack::new(6, 3, 0).expect("raw iron"),
        mornlea_protocol::ItemStack::EMPTY,
        mornlea_protocol::ItemStack::new(7, 1, 0).expect("iron ingot"),
        120,
        1600,
    )
    .expect("state");
    let payload = state.encode();
    assert_eq!(mornlea_protocol::FurnaceState::PACKET_ID, 13);
    assert_eq!(payload.len(), 18 + 3 * 5 + 3);
    // The furnace reference stays the shared 18-byte layout.
    assert_eq!(
        &payload[..18],
        &[
            0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x00, 0x05,
            0x07, 0x00, 0x00, 0x00,
        ]
    );
    // Input, fuel, and output follow in that fixed order.
    assert_eq!(&payload[18..23], &[0x06, 0x00, 0x03, 0x00, 0x00]);
    assert_eq!(&payload[23..28], &[0x00, 0x00, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[28..33], &[0x07, 0x00, 0x01, 0x00, 0x00]);
    // Progress is a single byte and burn time a little-endian u16.
    assert_eq!(payload[33], 120);
    assert_eq!(&payload[34..36], &[0x40, 0x06]);
    assert_eq!(mornlea_protocol::FurnaceState::PACKET_ID, 13);
    assert_eq!(payload.len(), 18 + 3 * 5 + 3);
    assert_eq!(
        mornlea_protocol::FurnaceState::decode(&payload).expect("decode"),
        state
    );
}

#[test]
fn furnace_state_rejects_invalid_slots_timers_and_malformed_payload() {
    let furnace = mornlea_protocol::ContainerRef::new(
        0,
        1,
        -1,
        mornlea_protocol::CONTAINER_KIND_FURNACE,
        5,
        7,
    )
    .expect("furnace");
    let chest =
        mornlea_protocol::ContainerRef::new(0, 1, -1, mornlea_protocol::CONTAINER_KIND_CHEST, 5, 7)
            .expect("chest");
    let base = mornlea_protocol::FurnaceState::new(
        furnace,
        mornlea_protocol::ItemStack::new(6, 3, 0).expect("raw iron"),
        mornlea_protocol::ItemStack::EMPTY,
        mornlea_protocol::ItemStack::new(7, 1, 0).expect("iron ingot"),
        120,
        1600,
    )
    .expect("state");
    // A furnace state must name a furnace.
    assert!(
        mornlea_protocol::FurnaceState::new(
            chest,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::EMPTY,
            0,
            0,
        )
        .is_err()
    );
    // Timer bounds come from the authoritative furnace contract.
    assert_eq!(
        mornlea_protocol::FurnaceState::new(
            furnace,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::FURNACE_SMELT_TICKS,
            0,
        ),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert_eq!(
        mornlea_protocol::FurnaceState::new(
            furnace,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::EMPTY,
            0,
            mornlea_protocol::FURNACE_BURN_TICKS + 1,
        ),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    // The input slot only accepts a registered smelting input, the fuel slot
    // only empty or coal, and the output slot only a fixed smelting product.
    assert!(
        mornlea_protocol::FurnaceState::new(
            furnace,
            mornlea_protocol::ItemStack::new(1, 1, 0).expect("stone"),
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::EMPTY,
            0,
            0,
        )
        .is_err()
    );
    assert!(
        mornlea_protocol::FurnaceState::new(
            furnace,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::new(1, 1, 0).expect("stone"),
            mornlea_protocol::ItemStack::EMPTY,
            0,
            0,
        )
        .is_err()
    );
    assert!(
        mornlea_protocol::FurnaceState::new(
            furnace,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::new(1, 1, 0).expect("stone"),
            0,
            0,
        )
        .is_err()
    );
    // Coal is a legal fuel.
    assert!(
        mornlea_protocol::FurnaceState::new(
            furnace,
            mornlea_protocol::ItemStack::EMPTY,
            mornlea_protocol::ItemStack::new(5, 1, 0).expect("coal"),
            mornlea_protocol::ItemStack::EMPTY,
            0,
            0,
        )
        .is_ok()
    );
    let payload = base.encode();
    assert!(mornlea_protocol::FurnaceState::decode(&payload[..payload.len() - 1]).is_err());
    let mut trailing = payload.clone();
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::FurnaceState::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn crafting_state_round_trip_preserves_golden_bytes() {
    let mut slots = [mornlea_protocol::ItemStack::EMPTY; 9];
    slots[0] = mornlea_protocol::ItemStack::new(1, 2, 0).expect("stone");
    slots[4] = mornlea_protocol::ItemStack::new(37, 1, 0).expect("stick");
    let state = mornlea_protocol::CraftingState::new(
        3,
        slots,
        mornlea_protocol::ItemStack::new(4, 4, 0).expect("stone brick"),
    )
    .expect("state");
    let payload = state.encode();
    assert_eq!(mornlea_protocol::CraftingState::PACKET_ID, 21);
    assert_eq!(payload.len(), 1 + 9 * 5 + 5);
    assert_eq!(payload[0], 3);
    // Grid slot 0 carries the stone stack, slot 4 the stick, the rest empty.
    assert_eq!(&payload[1..6], &[0x01, 0x00, 0x02, 0x00, 0x00]);
    assert_eq!(&payload[6..21], &[0x00; 15]);
    assert_eq!(&payload[21..26], &[0x25, 0x00, 0x01, 0x00, 0x00]);
    assert_eq!(&payload[26..46], &[0x00; 20]);
    // The derived output slot is always present.
    assert_eq!(&payload[46..51], &[0x04, 0x00, 0x04, 0x00, 0x00]);
    assert_eq!(mornlea_protocol::CraftingState::PACKET_ID, 21);
    assert_eq!(payload.len(), 1 + 9 * 5 + 5);
    assert_eq!(
        mornlea_protocol::CraftingState::decode(&payload).expect("decode"),
        state
    );
}

#[test]
fn crafting_state_rejects_unknown_size_residue_and_malformed_payload() {
    let empty = [mornlea_protocol::ItemStack::EMPTY; 9];
    for size in [0u8, 1, 4, 255] {
        assert!(
            mornlea_protocol::CraftingState::new(size, empty, mornlea_protocol::ItemStack::EMPTY)
                .is_err()
        );
    }
    let mut personal = empty;
    personal[0] = mornlea_protocol::ItemStack::new(1, 1, 0).expect("stone");
    assert!(
        mornlea_protocol::CraftingState::new(
            mornlea_protocol::CRAFTING_GRID_SIZE_PERSONAL,
            personal,
            mornlea_protocol::ItemStack::EMPTY
        )
        .is_ok()
    );
    personal[4] = mornlea_protocol::ItemStack::new(1, 1, 0).expect("stone");
    // A personal grid may not carry residue beyond its own size.
    assert!(
        mornlea_protocol::CraftingState::new(
            mornlea_protocol::CRAFTING_GRID_SIZE_PERSONAL,
            personal,
            mornlea_protocol::ItemStack::EMPTY
        )
        .is_err()
    );
    let mut slots = personal;
    slots[4] = mornlea_protocol::ItemStack::EMPTY;
    let state = mornlea_protocol::CraftingState::new(
        mornlea_protocol::CRAFTING_GRID_SIZE_WORKBENCH,
        slots,
        mornlea_protocol::ItemStack::EMPTY,
    )
    .expect("state");
    let payload = state.encode();
    assert!(mornlea_protocol::CraftingState::decode(&payload[..payload.len() - 1]).is_err());
    let mut trailing = payload.clone();
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::CraftingState::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
}

#[test]
fn inventory_state_round_trip_preserves_golden_bytes() {
    let mut hotbar = [mornlea_protocol::ItemStack::EMPTY; mornlea_protocol::HOTBAR_SLOTS];
    hotbar[0] = mornlea_protocol::ItemStack::new(1, 5, 0).expect("stone");
    hotbar[4] = mornlea_protocol::ItemStack::new(3, 64, 0).expect("grass");
    let mut backpack = [mornlea_protocol::ItemStack::EMPTY; mornlea_protocol::BACKPACK_SLOTS];
    backpack[0] = mornlea_protocol::ItemStack::new(2, 1, 0).expect("dirt");
    backpack[mornlea_protocol::BACKPACK_SLOTS - 1] =
        mornlea_protocol::ItemStack::new(1, 9, 0).expect("stone");
    let state = mornlea_protocol::InventoryState::new(2, hotbar, backpack).expect("state");
    let payload = state.encode();
    assert_eq!(mornlea_protocol::InventoryState::PACKET_ID, 10);
    assert_eq!(payload.len(), mornlea_protocol::INVENTORY_STATE_WIRE_BYTES);
    assert_eq!(payload[0], 2);
    assert_eq!(&payload[1..6], &[0x01, 0x00, 0x05, 0x00, 0x00]);
    assert_eq!(&payload[21..26], &[0x03, 0x00, 0x40, 0x00, 0x00]);
    let backpack_start = 1 + mornlea_protocol::HOTBAR_SLOTS * 5;
    assert_eq!(
        &payload[backpack_start..backpack_start + 5],
        &[0x02, 0x00, 0x01, 0x00, 0x00]
    );
    let tail = payload.len() - 5;
    assert_eq!(&payload[tail..], &[0x01, 0x00, 0x09, 0x00, 0x00]);
    assert_eq!(
        mornlea_protocol::InventoryState::decode(&payload).expect("decode"),
        state
    );
}

#[test]
fn inventory_state_rejects_unknown_selected_and_malformed_payload() {
    let empty_hotbar = [mornlea_protocol::ItemStack::EMPTY; mornlea_protocol::HOTBAR_SLOTS];
    let empty_backpack = [mornlea_protocol::ItemStack::EMPTY; mornlea_protocol::BACKPACK_SLOTS];
    assert_eq!(
        mornlea_protocol::InventoryState::new(9, empty_hotbar, empty_backpack),
        Err(mornlea_protocol::ProtocolError::InvalidRange)
    );
    assert!(mornlea_protocol::InventoryState::new(8, empty_hotbar, empty_backpack).is_ok());
    let state =
        mornlea_protocol::InventoryState::new(0, empty_hotbar, empty_backpack).expect("state");
    let payload = state.encode();
    assert!(mornlea_protocol::InventoryState::decode(&payload[..payload.len() - 1]).is_err());
    let mut trailing = payload.clone();
    trailing.push(0x00);
    assert_eq!(
        mornlea_protocol::InventoryState::decode(&trailing),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    // An unregistered item number in any slot is rejected.
    // Item 66 is the exclusive upper bound of registered item numbers.
    assert!(mornlea_protocol::ItemStack::new(66, 1, 0).is_err());
    let mut hotbar = empty_hotbar;
    hotbar[3] = mornlea_protocol::ItemStack::new(65, 1, 0).expect("item");
    let encoded = mornlea_protocol::InventoryState::new(0, hotbar, empty_backpack)
        .expect("state")
        .encode();
    let slot = 1 + 3 * 5;
    let mut corrupt = encoded.clone();
    corrupt[slot] = 0xff;
    corrupt[slot + 1] = 0xff;
    assert!(mornlea_protocol::InventoryState::decode(&corrupt).is_err());
}

#[test]
fn hostile_spawn_round_trip_preserves_batch_bytes() {
    let spawn = mornlea_protocol::HostileSpawn::new(
        0x0102_0304_0506_0708,
        vec![
            mornlea_protocol::HostileSpawnRecord {
                id: 7,
                dimension: mornlea_domain::Dimension::OVERWORLD,
                position: [2.5, 1.0, -3.25],
                yaw: 1.25,
                health: 14,
                kind: mornlea_protocol::HOSTILE_KIND_BONE_THROWER,
            },
            mornlea_protocol::HostileSpawnRecord {
                id: 9,
                dimension: mornlea_domain::Dimension::OVERWORLD,
                position: [-8.5, 65.5, 12.75],
                yaw: -2.5,
                health: 20,
                kind: mornlea_protocol::HOSTILE_KIND_NIGHTWALKER,
            },
        ],
    )
    .expect("spawn");
    let payload = spawn.encode();
    assert_eq!(mornlea_protocol::HostileSpawn::PACKET_ID, 22);
    assert_eq!(payload.len(), 9 + 2 * 30);
    assert_eq!(
        &payload[..9],
        &[0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0x02]
    );
    assert_eq!(&payload[9..17], &[0x07, 0, 0, 0, 0, 0, 0, 0]);
    assert_eq!(&payload[17..21], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[21..25], &[0x00, 0x00, 0x20, 0x40]);
    assert_eq!(&payload[25..29], &[0x00, 0x00, 0x80, 0x3f]);
    assert_eq!(&payload[29..33], &[0x00, 0x00, 0x50, 0xc0]);
    assert_eq!(&payload[33..37], &[0x00, 0x00, 0xa0, 0x3f]);
    assert_eq!(payload[37], 14);
    assert_eq!(payload[38], 1);
    // The second record starts at the fixed 30-byte stride after the first.
    assert_eq!(&payload[39..43], &[0x09, 0, 0, 0]);
    assert_eq!(payload.len(), 9 + 2 * 30);
    assert_eq!(
        mornlea_protocol::HostileSpawn::decode(&payload).expect("decode"),
        spawn
    );
}

#[test]
fn hostile_spawn_rejects_invalid_records_and_malformed_payload() {
    let record = mornlea_protocol::HostileSpawnRecord {
        id: 7,
        dimension: mornlea_domain::Dimension::OVERWORLD,
        position: [1.0, 2.0, 3.0],
        yaw: 0.5,
        health: 10,
        kind: mornlea_protocol::HOSTILE_KIND_NIGHTWALKER,
    };
    let base = |record: mornlea_protocol::HostileSpawnRecord| {
        mornlea_protocol::HostileSpawnRecord { ..record }
    };
    let mut zero_id = base(record);
    zero_id.id = 0;
    let mut foreign = base(record);
    foreign.dimension = mornlea_domain::Dimension::DEPTHS;
    let mut bad_health = base(record);
    bad_health.health = 0;
    let mut over_health = base(record);
    over_health.health = 21;
    let mut bad_kind = base(record);
    bad_kind.kind = 2;
    let mut bad_yaw = base(record);
    bad_yaw.yaw = f32::NAN;
    for bad in [zero_id, foreign, bad_health, over_health, bad_kind, bad_yaw] {
        assert!(mornlea_protocol::HostileSpawn::new(1, vec![bad]).is_err());
    }
    assert!(mornlea_protocol::HostileSpawn::new(1, vec![base(record), base(record)]).is_err());
    assert!(mornlea_protocol::HostileSpawn::new(1, Vec::new()).is_err());
    let valid = mornlea_protocol::HostileSpawn::new(1, vec![record]).expect("spawn");
    let payload = valid.encode();
    assert!(mornlea_protocol::HostileSpawn::decode(&payload[..payload.len() - 1]).is_err());
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::HostileSpawn::decode(&padded),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
}

#[test]
fn hostile_state_round_trip_preserves_batch_bytes() {
    let state = mornlea_protocol::HostileState::new(
        0x0102_0304_0506_0708,
        vec![mornlea_protocol::HostileStateRecord {
            id: 7,
            position: [2.5, 1.0, -3.25],
            velocity: [0.5, -1.25, 0.0],
            yaw: 1.25,
            health: 13,
            kind: mornlea_protocol::HOSTILE_KIND_BONE_THROWER,
        }],
    )
    .expect("state");
    let payload = state.encode();
    assert_eq!(mornlea_protocol::HostileState::PACKET_ID, 23);
    assert_eq!(payload.len(), 9 + 38);
    assert_eq!(
        &payload[..9],
        &[0x08, 0x07, 0x06, 0x05, 0x04, 0x03, 0x02, 0x01, 0x01]
    );
    assert_eq!(&payload[9..17], &[0x07, 0, 0, 0, 0, 0, 0, 0]);
    // Position is 2.5, 1.0, -3.25 and velocity 0.5, -1.25, 0.0 in that order.
    assert_eq!(&payload[17..21], &[0x00, 0x00, 0x20, 0x40]);
    assert_eq!(&payload[21..25], &[0x00, 0x00, 0x80, 0x3f]);
    assert_eq!(&payload[25..29], &[0x00, 0x00, 0x50, 0xc0]);
    assert_eq!(&payload[29..33], &[0x00, 0x00, 0x00, 0x3f]);
    assert_eq!(&payload[33..37], &[0x00, 0x00, 0xa0, 0xbf]);
    assert_eq!(&payload[37..41], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(payload.len(), 9 + 38);
    assert_eq!(
        mornlea_protocol::HostileState::decode(&payload).expect("decode"),
        state
    );
}

#[test]
fn hostile_state_rejects_invalid_records_and_malformed_payload() {
    let record = mornlea_protocol::HostileStateRecord {
        id: 7,
        position: [1.0, 2.0, 3.0],
        velocity: [0.0, 0.0, 0.0],
        yaw: 0.5,
        health: 10,
        kind: mornlea_protocol::HOSTILE_KIND_NIGHTWALKER,
    };
    let mut zero_id = record;
    zero_id.id = 0;
    let mut bad_velocity = record;
    bad_velocity.velocity = [f32::INFINITY, 0.0, 0.0];
    let mut bad_health = record;
    bad_health.health = 21;
    let mut bad_kind = record;
    bad_kind.kind = 2;
    for bad in [zero_id, bad_velocity, bad_health, bad_kind] {
        assert!(mornlea_protocol::HostileState::new(1, vec![bad]).is_err());
    }
    assert!(mornlea_protocol::HostileState::new(1, vec![record, record]).is_err());
    assert!(mornlea_protocol::HostileState::new(1, Vec::new()).is_err());
    let valid = mornlea_protocol::HostileState::new(1, vec![record]).expect("state");
    let payload = valid.encode();
    assert!(mornlea_protocol::HostileState::decode(&payload[..payload.len() - 1]).is_err());
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::HostileState::decode(&padded),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
}

#[test]
fn player_state_round_trip_preserves_golden_bytes() {
    let active = mornlea_protocol::PlayerState::new(
        0,
        0,
        mornlea_domain::Dimension::OVERWORLD,
        [0.0, 0.0, 0.0],
        [0.0, 0.0, 0.0],
        0.0,
        0.0,
        false,
        false,
        false,
        true,
        mornlea_protocol::BlockPos { x: 1, y: 2, z: 3 },
        6,
        15,
        true,
        15,
        300,
        0,
        false,
        0,
        24000,
        0,
        0,
        0,
        0,
        0,
    )
    .expect("active player state");
    let payload = active.encode();
    assert_eq!(mornlea_protocol::PlayerState::PACKET_ID, 3);
    assert_eq!(payload.len(), 93);
    // Mining state sits after the four boolean phase bits.
    assert_eq!(&payload[52..56], &[0x00, 0x00, 0x00, 0x01]);
    assert_eq!(&payload[56..60], &[0x01, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[60..64], &[0x02, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[64..68], &[0x03, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[68..70], &[0x06, 0x00]);
    assert_eq!(&payload[70..72], &[0x0f, 0x00]);
    assert_eq!(payload[72], 0x01);
    // Health, then the u16 oxygen, lock the survival field order.
    assert_eq!(payload[73], 0x0f);
    assert_eq!(&payload[74..76], &[0x2c, 0x01]);
    assert_eq!(&payload[80..88], &24000u64.to_le_bytes());
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&payload).expect("decode"),
        active
    );

    // A mid-range oxygen and day phase offset prove both are u16 little-endian
    // in their own slots rather than adjacent zero-value neighbours.
    let drowned = mornlea_protocol::PlayerState::new(
        0,
        0,
        mornlea_domain::Dimension::OVERWORLD,
        [0.0, 0.0, 0.0],
        [0.0, 0.0, 0.0],
        0.0,
        0.0,
        false,
        false,
        false,
        false,
        mornlea_protocol::BlockPos { x: 0, y: 0, z: 0 },
        0,
        0,
        false,
        15,
        0x0101,
        0,
        false,
        0x0101,
        24000,
        0,
        0,
        0,
        0,
        0,
    )
    .expect("drowned player state");
    let payload = drowned.encode();
    assert_eq!(payload.len(), 93);
    assert_eq!(&payload[74..76], &[0x01, 0x01]);
    assert_eq!(&payload[78..80], &[0x01, 0x01]);
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&payload).expect("decode"),
        drowned
    );

    // Weather, season, in-season progress, temperature and armor points are the
    // five trailing bytes; a mid-value sample per byte pins the order.
    let seasonal = mornlea_protocol::PlayerState::new(
        0,
        0,
        mornlea_domain::Dimension::OVERWORLD,
        [0.0, 0.0, 0.0],
        [0.0, 0.0, 0.0],
        0.0,
        0.0,
        false,
        false,
        false,
        false,
        mornlea_protocol::BlockPos { x: 0, y: 0, z: 0 },
        0,
        0,
        false,
        15,
        300,
        12,
        false,
        0,
        24000,
        mornlea_protocol::WEATHER_RAIN,
        mornlea_protocol::SEASON_AUTUMN,
        128,
        -8,
        15,
    )
    .expect("seasonal player state");
    let payload = seasonal.encode();
    assert_eq!(payload.len(), 93);
    assert_eq!(payload[76], 12);
    assert_eq!(payload[88], mornlea_protocol::WEATHER_RAIN);
    assert_eq!(payload[89], mornlea_protocol::SEASON_AUTUMN);
    assert_eq!(payload[90], 128);
    assert_eq!(payload[91], 0xf8);
    assert_eq!(payload[92], 15);
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&payload).expect("decode"),
        seasonal
    );
}

#[test]
fn player_state_rejects_out_of_range_fields_and_malformed_payload() {
    let active = mornlea_protocol::PlayerState::new(
        7,
        9,
        mornlea_domain::Dimension::OVERWORLD,
        [1.0, 2.0, 3.0],
        [0.5, 0.0, 0.0],
        0.25,
        -0.5,
        true,
        true,
        false,
        true,
        mornlea_protocol::BlockPos { x: 4, y: 5, z: 6 },
        6,
        15,
        true,
        20,
        300,
        20,
        false,
        23999,
        24000,
        2,
        3,
        255,
        45,
        20,
    )
    .expect("active player state");

    let mut bad_health = active.clone();
    bad_health.health = 21;
    let mut bad_oxygen = active.clone();
    bad_oxygen.oxygen = 301;
    let mut bad_hunger = active.clone();
    bad_hunger.hunger = 21;
    let mut bad_phase = active.clone();
    bad_phase.day_phase_offset = 24000;
    let mut bad_weather = active.clone();
    bad_weather.weather_kind = 3;
    let mut bad_season = active.clone();
    bad_season.season = 4;
    let mut bad_armor = active.clone();
    bad_armor.armor_points = 21;
    let mut bad_position = active.clone();
    bad_position.position = [f32::NAN, 0.0, 0.0];
    let mut bad_yaw = active.clone();
    bad_yaw.yaw = f32::INFINITY;
    for bad in [
        bad_health,
        bad_oxygen,
        bad_hunger,
        bad_phase,
        bad_weather,
        bad_season,
        bad_armor,
        bad_position,
        bad_yaw,
    ] {
        assert!(
            bad.validate().is_err(),
            "accepted out-of-range player state"
        );
    }

    // An inactive mining block must be entirely empty, and an active one must
    // carry progress strictly below the requirement.
    let mut dirty_inactive = active.clone();
    dirty_inactive.mining_active = false;
    dirty_inactive.mining_target = mornlea_protocol::BlockPos { x: 1, y: 0, z: 0 };
    assert!(dirty_inactive.validate().is_err());
    let mut dirty_progress = active.clone();
    dirty_progress.mining_progress_ticks = 0;
    assert!(dirty_progress.validate().is_err());
    let mut finished_progress = active.clone();
    finished_progress.mining_progress_ticks = 15;
    assert!(finished_progress.validate().is_err());

    let payload = active.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::PlayerState::decode(&payload[..length]).is_err(),
            "accepted truncated player state at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::PlayerState::decode(&padded),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    // An out-of-range weather byte is rejected by the same validation the Go
    // decoder applies after the last field.
    let mut bad_weather_payload = payload.clone();
    bad_weather_payload[88] = 3;
    assert!(mornlea_protocol::PlayerState::decode(&bad_weather_payload).is_err());
}

#[test]
fn companion_spawn_round_trip_preserves_golden_bytes() {
    let spawn = mornlea_protocol::CompanionSpawn::new(
        mornlea_protocol::CompanionId::new([
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff,
        ])
        .expect("companion id"),
        "Mira".to_owned(),
        0x0102_0304_0506_0708,
        mornlea_domain::Dimension::OVERWORLD,
        [2.5, 1.0, -3.25],
        1.25,
        -0.5,
    )
    .expect("spawn");
    let payload = spawn.encode();
    assert_eq!(mornlea_protocol::CompanionSpawn::PACKET_ID, 17);
    // 16 ID + (1 length prefix + 4 name) + 8 tick + 4 dimension + 12 position
    // + 4 yaw + 4 pitch.
    assert_eq!(payload.len(), 53);
    assert_eq!(&payload[..16], &spawn.companion_id().bytes());
    assert_eq!(payload[16], 4);
    assert_eq!(&payload[17..21], b"Mira");
    assert_eq!(&payload[21..29], &0x0102_0304_0506_0708u64.to_le_bytes());
    assert_eq!(&payload[29..33], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[33..37], &[0x00, 0x00, 0x20, 0x40]);
    assert_eq!(&payload[37..41], &[0x00, 0x00, 0x80, 0x3f]);
    assert_eq!(&payload[41..45], &[0x00, 0x00, 0x50, 0xc0]);
    assert_eq!(&payload[45..49], &[0x00, 0x00, 0xa0, 0x3f]);
    assert_eq!(&payload[49..53], &[0x00, 0x00, 0x00, 0xbf]);
    assert_eq!(
        mornlea_protocol::CompanionSpawn::decode(&payload).expect("decode"),
        spawn
    );
}

#[test]
fn companion_spawn_rejects_invalid_identity_name_and_pose() {
    let spawn = mornlea_protocol::CompanionSpawn::new(
        mornlea_protocol::CompanionId::new([
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff,
        ])
        .expect("companion id"),
        "Mira".to_owned(),
        1,
        mornlea_domain::Dimension::OVERWORLD,
        [1.0, 2.0, 3.0],
        0.0,
        0.0,
    )
    .expect("spawn");

    let mut foreign = spawn.clone();
    foreign.dimension = mornlea_domain::Dimension::DEPTHS;
    let mut bad_name = spawn.clone();
    bad_name.name = "has space".to_owned();
    let mut padded_name = spawn.clone();
    padded_name.name = " padded".to_owned();
    let mut long_name = spawn.clone();
    long_name.name = "x".repeat(33);
    let mut bad_pose = spawn.clone();
    bad_pose.position = [f32::NAN, 0.0, 0.0];
    let mut bad_pitch = spawn.clone();
    bad_pitch.pitch = 1.6;
    for bad in [
        foreign,
        bad_name,
        padded_name,
        long_name,
        bad_pose,
        bad_pitch,
    ] {
        assert!(
            bad.validate().is_err(),
            "accepted invalid companion spawn field"
        );
    }
    assert!(
        mornlea_protocol::CompanionId::new([0xff; 16]).is_err(),
        "accepted non-UUIDv4 companion identity"
    );

    let payload = spawn.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::CompanionSpawn::decode(&payload[..length]).is_err(),
            "accepted truncated companion spawn at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::CompanionSpawn::decode(&padded),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    // A name length prefix that overruns the remaining payload is rejected
    // before any bytes are copied.
    let mut bad_length = payload.clone();
    bad_length[16] = 200;
    assert!(mornlea_protocol::CompanionSpawn::decode(&bad_length).is_err());
}

#[test]
fn companion_states_round_trip_preserves_batch_bytes() {
    let first = mornlea_protocol::CompanionId::new([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xf0,
    ])
    .expect("first companion id");
    let second = mornlea_protocol::CompanionId::new([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ])
    .expect("second companion id");
    let states = mornlea_protocol::CompanionStates::new(
        0x0102_0304_0506_0708,
        vec![
            mornlea_protocol::CompanionState {
                companion_id: first,
                dimension: mornlea_domain::Dimension::OVERWORLD,
                position: [2.5, 1.0, -3.25],
                yaw: 1.25,
                pitch: -0.5,
                reset: true,
            },
            mornlea_protocol::CompanionState {
                companion_id: second,
                dimension: mornlea_domain::Dimension::OVERWORLD,
                position: [-8.5, 65.5, 12.75],
                yaw: -2.5,
                pitch: 0.0,
                reset: false,
            },
        ],
    )
    .expect("states");
    let payload = states.encode();
    assert_eq!(mornlea_protocol::CompanionStates::PACKET_ID, 18);
    assert_eq!(payload.len(), 8 + 1 + 2 * 41);
    assert_eq!(&payload[..8], &0x0102_0304_0506_0708u64.to_le_bytes());
    assert_eq!(payload[8], 2);
    assert_eq!(&payload[9..25], &first.bytes());
    assert_eq!(&payload[25..29], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[29..33], &[0x00, 0x00, 0x20, 0x40]);
    assert_eq!(&payload[41..45], &[0x00, 0x00, 0xa0, 0x3f]);
    assert_eq!(&payload[45..49], &[0x00, 0x00, 0x00, 0xbf]);
    assert_eq!(payload[49], 1);
    // The second record starts one fixed 41-byte stride later.
    assert_eq!(&payload[50..66], &second.bytes());
    assert_eq!(payload[90], 0);
    assert_eq!(
        mornlea_protocol::CompanionStates::decode(&payload).expect("decode"),
        states
    );
}

#[test]
fn companion_states_rejects_unsorted_invalid_and_malformed_payload() {
    let first = mornlea_protocol::CompanionId::new([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xf0,
    ])
    .expect("first companion id");
    let second = mornlea_protocol::CompanionId::new([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ])
    .expect("second companion id");
    let record = mornlea_protocol::CompanionState {
        companion_id: first,
        dimension: mornlea_domain::Dimension::OVERWORLD,
        position: [1.0, 2.0, 3.0],
        yaw: 0.5,
        pitch: 0.0,
        reset: false,
    };

    let mut foreign = record;
    foreign.dimension = mornlea_domain::Dimension::DEPTHS;
    let mut bad_pitch = record;
    bad_pitch.pitch = 1.6;
    let mut bad_position = record;
    bad_position.position = [f32::INFINITY, 0.0, 0.0];
    for bad in [foreign, bad_pitch, bad_position] {
        assert!(
            mornlea_protocol::CompanionStates::new(1, vec![bad]).is_err(),
            "accepted invalid companion state record"
        );
    }
    assert!(mornlea_protocol::CompanionStates::new(1, Vec::new()).is_err());
    assert!(
        mornlea_protocol::CompanionStates::new(1, vec![record, record]).is_err(),
        "accepted duplicate companion state records"
    );
    assert!(
        mornlea_protocol::CompanionStates::new(
            1,
            vec![
                mornlea_protocol::CompanionState {
                    companion_id: second,
                    ..record
                },
                record,
            ]
        )
        .is_err(),
        "accepted descending companion state records"
    );
    // The batch ceiling is the shared companion activity limit.
    let mut full = Vec::new();
    for index in 0..mornlea_protocol::MAX_COMPANION_STATES {
        full.push(mornlea_protocol::CompanionState {
            companion_id: mornlea_protocol::CompanionId::new([
                0,
                0,
                0,
                0,
                0,
                0,
                0x40 + index as u8,
                0,
                0x80 + index as u8,
                0,
                0,
                0,
                0,
                0,
                0,
                0,
            ])
            .expect("companion id"),
            ..record
        });
    }
    assert!(
        mornlea_protocol::CompanionStates::new(1, full).is_ok(),
        "rejected a full companion state batch"
    );

    let valid = mornlea_protocol::CompanionStates::new(1, vec![record]).expect("states");
    let payload = valid.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::CompanionStates::decode(&payload[..length]).is_err(),
            "accepted truncated companion states at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::CompanionStates::decode(&padded),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    // A count that does not match the remaining record bytes is rejected
    // before any record is read.
    let mut mismatched = payload.clone();
    mismatched[8] = 2;
    assert_eq!(
        mornlea_protocol::CompanionStates::decode(&mismatched),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut empty = payload.clone();
    empty[8] = 0;
    assert!(mornlea_protocol::CompanionStates::decode(&empty).is_err());
    let mut over = payload.clone();
    over[8] = (mornlea_protocol::MAX_COMPANION_STATES + 1) as u8;
    assert!(mornlea_protocol::CompanionStates::decode(&over).is_err());
}

#[test]
fn item_drop_upserts_round_trip_preserves_batch_bytes() {
    let upserts = mornlea_protocol::ItemDropUpserts::new(
        0x0102_0304_0506_0708,
        vec![
            mornlea_protocol::ItemDrop {
                id: mornlea_protocol::DropId::new(0, 0, 0, 1, 1).expect("first drop id"),
                block_index: 100,
                item: mornlea_protocol::ITEM_STONE,
                count: 5,
                durability: 0,
            },
            mornlea_protocol::ItemDrop {
                id: mornlea_protocol::DropId::new(0, 0, 0, 2, 1).expect("second drop id"),
                block_index: 200,
                item: mornlea_protocol::ITEM_COAL,
                count: 10,
                durability: 0,
            },
        ],
    )
    .expect("upserts");
    let payload = upserts.encode();
    assert_eq!(mornlea_protocol::ItemDropUpserts::PACKET_ID, 11);
    assert_eq!(payload.len(), 8 + 1 + 2 * 26);
    assert_eq!(&payload[..8], &0x0102_0304_0506_0708u64.to_le_bytes());
    assert_eq!(payload[8], 2);
    // One drop is a 17-byte identity, a u32 block index, and a 5-byte stack.
    assert_eq!(&payload[9..13], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(payload[21], 1);
    assert_eq!(&payload[22..26], &1u32.to_le_bytes());
    assert_eq!(&payload[26..30], &100u32.to_le_bytes());
    assert_eq!(
        &payload[30..32],
        &mornlea_protocol::ITEM_STONE.to_le_bytes()
    );
    assert_eq!(payload[32], 5);
    assert_eq!(&payload[33..35], &[0x00, 0x00]);
    // The second record starts one fixed 26-byte stride later.
    assert_eq!(payload[47], 2);
    assert_eq!(&payload[56..58], &mornlea_protocol::ITEM_COAL.to_le_bytes());
    assert_eq!(payload[58], 10);
    assert_eq!(
        mornlea_protocol::ItemDropUpserts::decode(&payload).expect("decode"),
        upserts
    );
}

#[test]
fn item_drop_upserts_rejects_invalid_records_and_malformed_payload() {
    let drop = mornlea_protocol::ItemDrop {
        id: mornlea_protocol::DropId::new(0, 0, 0, 1, 1).expect("drop id"),
        block_index: 100,
        item: mornlea_protocol::ITEM_STONE,
        count: 5,
        durability: 0,
    };

    assert!(
        mornlea_protocol::DropId::new(0, 0, 0, 32, 1).is_err(),
        "accepted a drop slot outside the fixed per-chunk array"
    );
    assert!(
        mornlea_protocol::DropId::new(0, 0, 0, 1, 0).is_err(),
        "accepted a drop identity with a zero generation"
    );
    let bad_block_index = mornlea_protocol::ItemDrop {
        block_index: 98304,
        ..drop
    };
    let unregistered_item = mornlea_protocol::ItemDrop {
        item: mornlea_protocol::ITEM_ID_MAX,
        ..drop
    };
    let zero_count = mornlea_protocol::ItemDrop { count: 0, ..drop };
    let over_count = mornlea_protocol::ItemDrop { count: 65, ..drop };
    let bad_durability = mornlea_protocol::ItemDrop {
        item: mornlea_protocol::ITEM_STONE_PICKAXE,
        durability: 200,
        ..drop
    };
    // A `DropId` is a validated newtype, so an out-of-range slot or a zero
    // generation cannot be built at all; the two rejections are asserted at
    // the identity constructor above instead of through the batch.
    for bad in [
        bad_block_index,
        unregistered_item,
        zero_count,
        over_count,
        bad_durability,
    ] {
        assert!(
            mornlea_protocol::ItemDropUpserts::new(1, vec![bad]).is_err(),
            "accepted invalid item drop"
        );
    }
    assert!(mornlea_protocol::ItemDropUpserts::new(1, Vec::new()).is_err());
    assert!(
        mornlea_protocol::ItemDropUpserts::new(1, vec![drop, drop]).is_err(),
        "accepted duplicate item drop identities"
    );
    let later = mornlea_protocol::ItemDrop {
        id: mornlea_protocol::DropId::new(0, 0, 0, 2, 1).expect("drop id"),
        ..drop
    };
    assert!(
        mornlea_protocol::ItemDropUpserts::new(1, vec![later, drop]).is_err(),
        "accepted descending item drop identities"
    );

    let valid = mornlea_protocol::ItemDropUpserts::new(1, vec![drop]).expect("upserts");
    let payload = valid.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::ItemDropUpserts::decode(&payload[..length]).is_err(),
            "accepted truncated item drop upserts at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::ItemDropUpserts::decode(&padded),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    // A count that does not match the remaining record bytes is rejected
    // before any record is read.
    let mut mismatched = payload.clone();
    mismatched[8] = 2;
    assert_eq!(
        mornlea_protocol::ItemDropUpserts::decode(&mismatched),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut empty = payload.clone();
    empty[8] = 0;
    assert!(mornlea_protocol::ItemDropUpserts::decode(&empty).is_err());
    let mut over = payload.clone();
    over[8] = (mornlea_protocol::MAX_ITEM_DROP_BATCH + 1) as u8;
    assert!(mornlea_protocol::ItemDropUpserts::decode(&over).is_err());
}

#[test]
fn item_drop_removes_round_trip_preserves_batch_bytes() {
    let removes = mornlea_protocol::ItemDropRemoves::new(
        0x0102_0304_0506_0708,
        vec![
            mornlea_protocol::DropId::new(0, 0, 0, 1, 1).expect("first drop id"),
            mornlea_protocol::DropId::new(0, 0, 0, 2, 7).expect("second drop id"),
        ],
    )
    .expect("removes");
    let payload = removes.encode();
    assert_eq!(mornlea_protocol::ItemDropRemoves::PACKET_ID, 12);
    assert_eq!(payload.len(), 8 + 1 + 2 * 17);
    assert_eq!(&payload[..8], &0x0102_0304_0506_0708u64.to_le_bytes());
    assert_eq!(payload[8], 2);
    assert_eq!(&payload[9..13], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(payload[21], 1);
    assert_eq!(&payload[22..26], &1u32.to_le_bytes());
    // The second identity starts one fixed 17-byte stride later.
    assert_eq!(payload[38], 2);
    assert_eq!(&payload[39..43], &7u32.to_le_bytes());
    assert_eq!(
        mornlea_protocol::ItemDropRemoves::decode(&payload).expect("decode"),
        removes
    );
}

#[test]
fn item_drop_removes_rejects_invalid_ids_and_malformed_payload() {
    let first = mornlea_protocol::DropId::new(0, 0, 0, 1, 1).expect("first drop id");
    let second = mornlea_protocol::DropId::new(0, 0, 0, 2, 1).expect("second drop id");

    assert!(mornlea_protocol::ItemDropRemoves::new(1, Vec::new()).is_err());
    assert!(
        mornlea_protocol::ItemDropRemoves::new(1, vec![first, first]).is_err(),
        "accepted duplicate drop identities"
    );
    assert!(
        mornlea_protocol::ItemDropRemoves::new(1, vec![second, first]).is_err(),
        "accepted descending drop identities"
    );
    // The same identity space as the upsert batch, so the slot and generation
    // rules are enforced by `DropId` itself.
    assert!(
        mornlea_protocol::DropId::new(0, 0, 0, 32, 1).is_err(),
        "accepted a drop slot outside the fixed per-chunk array"
    );

    let valid = mornlea_protocol::ItemDropRemoves::new(1, vec![first]).expect("removes");
    let payload = valid.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::ItemDropRemoves::decode(&payload[..length]).is_err(),
            "accepted truncated item drop removes at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::ItemDropRemoves::decode(&padded),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let mut mismatched = payload.clone();
    mismatched[8] = 2;
    assert_eq!(
        mornlea_protocol::ItemDropRemoves::decode(&mismatched),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut empty = payload.clone();
    empty[8] = 0;
    assert!(mornlea_protocol::ItemDropRemoves::decode(&empty).is_err());
    let mut over = payload.clone();
    over[8] = (mornlea_protocol::MAX_ITEM_DROP_BATCH + 1) as u8;
    assert!(mornlea_protocol::ItemDropRemoves::decode(&over).is_err());
}

#[test]
fn remote_player_spawn_round_trip_preserves_golden_bytes() {
    let spawn = mornlea_protocol::RemotePlayerSpawn::new(
        mornlea_protocol::PlayerId::new([
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff,
        ])
        .expect("player id"),
        "陈".to_owned(),
        1,
        mornlea_domain::Dimension::OVERWORLD,
        [1.0, 2.0, 3.0],
        4.0,
        -5.0,
    )
    .expect("spawn");
    let payload = spawn.encode();
    assert_eq!(mornlea_protocol::RemotePlayerSpawn::PACKET_ID, 7);
    // 16 identity + (1 length prefix + 3 name bytes) + 8 tick + 4 dimension
    // + 12 position + 4 yaw + 4 pitch.
    assert_eq!(payload.len(), 52);
    assert_eq!(
        &payload[..16],
        &[
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff,
        ]
    );
    assert_eq!(payload[16], 3);
    assert_eq!(&payload[17..20], &[0xe9, 0x99, 0x88]);
    assert_eq!(&payload[20..28], &1u64.to_le_bytes());
    assert_eq!(&payload[28..32], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[32..36], &1.0f32.to_bits().to_le_bytes());
    assert_eq!(&payload[36..40], &2.0f32.to_bits().to_le_bytes());
    assert_eq!(&payload[40..44], &3.0f32.to_bits().to_le_bytes());
    assert_eq!(&payload[44..48], &4.0f32.to_bits().to_le_bytes());
    assert_eq!(&payload[48..52], &(-5.0f32).to_bits().to_le_bytes());
    assert_eq!(
        mornlea_protocol::RemotePlayerSpawn::decode(&payload).expect("decode"),
        spawn
    );
    // Remote players, unlike companions and mobs, may appear in the depths.
    let depths = mornlea_protocol::RemotePlayerSpawn::new(
        spawn.player_id(),
        "陈".to_owned(),
        1,
        mornlea_domain::Dimension::DEPTHS,
        [1.0, 2.0, 3.0],
        4.0,
        -5.0,
    )
    .expect("depths spawn");
    assert_eq!(
        mornlea_protocol::RemotePlayerSpawn::decode(&depths.encode()).expect("decode"),
        depths
    );
}

#[test]
fn remote_player_spawn_rejects_invalid_identity_name_and_pose() {
    let spawn = mornlea_protocol::RemotePlayerSpawn::new(
        mornlea_protocol::PlayerId::new([
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff,
        ])
        .expect("player id"),
        "陈".to_owned(),
        1,
        mornlea_domain::Dimension::OVERWORLD,
        [1.0, 2.0, 3.0],
        4.0,
        -5.0,
    )
    .expect("spawn");

    let mut padded_name = spawn.clone();
    padded_name.display_name = " 陈".to_owned();
    let mut long_name = spawn.clone();
    long_name.display_name = "x".repeat(33);
    let mut control_name = spawn.clone();
    control_name.display_name = "a\u{0}b".to_owned();
    let mut empty_name = spawn.clone();
    empty_name.display_name = String::new();
    let mut bad_position = spawn.clone();
    bad_position.position = [f32::NAN, 0.0, 0.0];
    let mut bad_yaw = spawn.clone();
    bad_yaw.yaw = f32::INFINITY;
    let mut bad_pitch = spawn.clone();
    bad_pitch.pitch = f32::NEG_INFINITY;
    for bad in [
        padded_name,
        long_name,
        control_name,
        empty_name,
        bad_position,
        bad_yaw,
        bad_pitch,
    ] {
        assert!(
            bad.validate().is_err(),
            "accepted invalid remote player spawn field"
        );
    }
    assert!(
        mornlea_protocol::PlayerId::new([0xff; 16]).is_err(),
        "accepted non-UUIDv4 player identity"
    );

    let payload = spawn.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::RemotePlayerSpawn::decode(&payload[..length]).is_err(),
            "accepted truncated remote player spawn at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::RemotePlayerSpawn::decode(&padded),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    // A name length prefix that overruns the remaining payload is rejected
    // before any bytes are copied.
    let mut bad_length = payload.clone();
    bad_length[16] = 200;
    assert!(mornlea_protocol::RemotePlayerSpawn::decode(&bad_length).is_err());
}

#[test]
fn remote_player_states_round_trip_preserves_golden_bytes() {
    let states = mornlea_protocol::RemotePlayerStates::new(
        2,
        vec![mornlea_protocol::RemotePlayerState {
            player_id: mornlea_protocol::PlayerId::new([
                0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
                0xee, 0xff,
            ])
            .expect("player id"),
            dimension: mornlea_domain::Dimension::OVERWORLD,
            position: [1.0, 2.0, 3.0],
            yaw: 4.0,
            pitch: -5.0,
            reset: true,
        }],
    )
    .expect("states");
    let payload = states.encode();
    assert_eq!(mornlea_protocol::RemotePlayerStates::PACKET_ID, 9);
    assert_eq!(payload.len(), 8 + 1 + 41);
    assert_eq!(&payload[..8], &2u64.to_le_bytes());
    assert_eq!(payload[8], 1);
    assert_eq!(
        &payload[9..25],
        &[
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff,
        ]
    );
    assert_eq!(&payload[25..29], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[29..33], &1.0f32.to_bits().to_le_bytes());
    assert_eq!(&payload[41..45], &4.0f32.to_bits().to_le_bytes());
    assert_eq!(&payload[45..49], &(-5.0f32).to_bits().to_le_bytes());
    assert_eq!(payload[49], 1);
    assert_eq!(
        mornlea_protocol::RemotePlayerStates::decode(&payload).expect("decode"),
        states
    );
    // The depths are a legal dimension for a peer session, as for its spawn.
    let depths = mornlea_protocol::RemotePlayerStates::new(
        2,
        vec![mornlea_protocol::RemotePlayerState {
            dimension: mornlea_domain::Dimension::DEPTHS,
            ..states.players[0]
        }],
    )
    .expect("depths states");
    assert_eq!(
        mornlea_protocol::RemotePlayerStates::decode(&depths.encode()).expect("decode"),
        depths
    );
}

#[test]
fn remote_player_states_rejects_unsorted_invalid_and_malformed_payload() {
    let first = mornlea_protocol::PlayerId::new([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xf0,
    ])
    .expect("first player id");
    let second = mornlea_protocol::PlayerId::new([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ])
    .expect("second player id");
    let record = mornlea_protocol::RemotePlayerState {
        player_id: first,
        dimension: mornlea_domain::Dimension::OVERWORLD,
        position: [1.0, 2.0, 3.0],
        yaw: 4.0,
        pitch: -5.0,
        reset: false,
    };

    let mut bad_position = record;
    bad_position.position = [f32::NAN, 0.0, 0.0];
    let mut bad_yaw = record;
    bad_yaw.yaw = f32::INFINITY;
    for bad in [bad_position, bad_yaw] {
        assert!(
            mornlea_protocol::RemotePlayerStates::new(1, vec![bad]).is_err(),
            "accepted invalid remote player state record"
        );
    }
    assert!(mornlea_protocol::RemotePlayerStates::new(1, Vec::new()).is_err());
    assert!(
        mornlea_protocol::RemotePlayerStates::new(1, vec![record, record]).is_err(),
        "accepted duplicate remote player state records"
    );
    assert!(
        mornlea_protocol::RemotePlayerStates::new(
            1,
            vec![
                mornlea_protocol::RemotePlayerState {
                    player_id: second,
                    ..record
                },
                record,
            ]
        )
        .is_err(),
        "accepted descending remote player state records"
    );
    // The batch ceiling is the fixed peer-session budget, not the companion
    // activity limit.
    let full: Vec<_> = (0..mornlea_protocol::MAX_REMOTE_PLAYER_STATES)
        .map(|index| mornlea_protocol::RemotePlayerState {
            player_id: mornlea_protocol::PlayerId::new([
                0,
                0,
                0,
                0,
                0,
                0,
                0x40 + index as u8,
                0,
                0x80 + index as u8,
                0,
                0,
                0,
                0,
                0,
                0,
                0,
            ])
            .expect("player id"),
            ..record
        })
        .collect();
    assert!(
        mornlea_protocol::RemotePlayerStates::new(1, full).is_ok(),
        "rejected a full remote player state batch"
    );

    let valid = mornlea_protocol::RemotePlayerStates::new(1, vec![record]).expect("states");
    let payload = valid.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::RemotePlayerStates::decode(&payload[..length]).is_err(),
            "accepted truncated remote player states at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::RemotePlayerStates::decode(&padded),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    let mut mismatched = payload.clone();
    mismatched[8] = 2;
    assert_eq!(
        mornlea_protocol::RemotePlayerStates::decode(&mismatched),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut empty = payload.clone();
    empty[8] = 0;
    assert!(mornlea_protocol::RemotePlayerStates::decode(&empty).is_err());
    let mut over = payload.clone();
    over[8] = (mornlea_protocol::MAX_REMOTE_PLAYER_STATES + 1) as u8;
    assert!(mornlea_protocol::RemotePlayerStates::decode(&over).is_err());
    // A payload above the fixed wire ceiling is rejected before any record is
    // read, which is the pre-allocation guard the Go decoder applies.
    let oversized = vec![0u8; mornlea_protocol::REMOTE_PLAYER_STATES_MAX_WIRE_BYTES + 1];
    assert_eq!(
        mornlea_protocol::RemotePlayerStates::decode(&oversized),
        Err(mornlea_protocol::ProtocolError::FrameTooLarge)
    );
}

#[test]
fn passive_spawn_round_trip_preserves_batch_bytes() {
    let spawn = mornlea_protocol::PassiveSpawn::new(
        0x0102_0304_0506_0708,
        vec![
            mornlea_protocol::PassiveSpawnRecord {
                id: 7,
                dimension: mornlea_domain::Dimension::OVERWORLD,
                position: [2.5, 1.0, -3.25],
                yaw: 1.25,
                health: 14,
            },
            mornlea_protocol::PassiveSpawnRecord {
                id: 9,
                dimension: mornlea_domain::Dimension::OVERWORLD,
                position: [-8.5, 65.5, 12.75],
                yaw: -2.5,
                health: 20,
            },
        ],
    )
    .expect("spawn");
    let payload = spawn.encode();
    assert_eq!(mornlea_protocol::PassiveSpawn::PACKET_ID, 26);
    assert_eq!(payload.len(), 8 + 1 + 2 * 29);
    assert_eq!(&payload[..8], &0x0102_0304_0506_0708u64.to_le_bytes());
    assert_eq!(payload[8], 2);
    assert_eq!(&payload[9..17], &[0x07, 0, 0, 0, 0, 0, 0, 0]);
    assert_eq!(&payload[17..21], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[21..25], &[0x00, 0x00, 0x20, 0x40]);
    assert_eq!(&payload[33..37], &[0x00, 0x00, 0xa0, 0x3f]);
    assert_eq!(payload[37], 14);
    // The second record starts one fixed 29-byte stride later.
    assert_eq!(&payload[38..46], &[0x09, 0, 0, 0, 0, 0, 0, 0]);
    assert_eq!(payload[66], 20);
    assert_eq!(
        mornlea_protocol::PassiveSpawn::decode(&payload).expect("decode"),
        spawn
    );
}

#[test]
fn passive_spawn_rejects_invalid_records_and_malformed_payload() {
    let record = mornlea_protocol::PassiveSpawnRecord {
        id: 7,
        dimension: mornlea_domain::Dimension::OVERWORLD,
        position: [1.0, 2.0, 3.0],
        yaw: 0.5,
        health: 10,
    };

    let mut zero_id = record;
    zero_id.id = 0;
    let mut foreign = record;
    foreign.dimension = mornlea_domain::Dimension::DEPTHS;
    let mut bad_health = record;
    bad_health.health = 0;
    let mut over_health = record;
    over_health.health = 21;
    let mut bad_yaw = record;
    bad_yaw.yaw = f32::NAN;
    for bad in [zero_id, foreign, bad_health, over_health, bad_yaw] {
        assert!(
            mornlea_protocol::PassiveSpawn::new(1, vec![bad]).is_err(),
            "accepted invalid passive spawn record"
        );
    }
    assert!(mornlea_protocol::PassiveSpawn::new(1, vec![record, record]).is_err());
    assert!(mornlea_protocol::PassiveSpawn::new(1, Vec::new()).is_err());
    // The protocol ceiling is the record budget, not the smaller live capacity
    // the authority converges on.
    let full: Vec<_> = (1..=mornlea_protocol::MAX_PASSIVE_SPAWN_RECORDS as u64)
        .map(|id| mornlea_protocol::PassiveSpawnRecord { id, ..record })
        .collect();
    assert!(
        mornlea_protocol::PassiveSpawn::new(1, full).is_ok(),
        "rejected a full passive spawn batch"
    );

    let valid = mornlea_protocol::PassiveSpawn::new(1, vec![record]).expect("spawn");
    let payload = valid.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::PassiveSpawn::decode(&payload[..length]).is_err(),
            "accepted truncated passive spawn at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::PassiveSpawn::decode(&padded),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut mismatched = payload.clone();
    mismatched[8] = 2;
    assert_eq!(
        mornlea_protocol::PassiveSpawn::decode(&mismatched),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut empty = payload.clone();
    empty[8] = 0;
    assert!(mornlea_protocol::PassiveSpawn::decode(&empty).is_err());
    let mut over = payload.clone();
    over[8] = mornlea_protocol::MAX_PASSIVE_SPAWN_RECORDS + 1;
    assert!(mornlea_protocol::PassiveSpawn::decode(&over).is_err());
}

#[test]
fn passive_state_round_trip_preserves_batch_bytes() {
    let state = mornlea_protocol::PassiveState::new(
        0x0102_0304_0506_0708,
        vec![
            mornlea_protocol::PassiveStateRecord {
                id: 7,
                position: [2.5, 1.0, -3.25],
                velocity: [0.5, -1.25, 0.0],
                yaw: 1.25,
                health: 13,
                grazing: 1,
            },
            mornlea_protocol::PassiveStateRecord {
                id: 9,
                position: [-8.5, 65.5, 12.75],
                velocity: [0.0, 0.0, 0.0],
                yaw: -2.5,
                health: 20,
                grazing: 0,
            },
        ],
    )
    .expect("state");
    let payload = state.encode();
    assert_eq!(mornlea_protocol::PassiveState::PACKET_ID, 27);
    assert_eq!(payload.len(), 8 + 1 + 2 * 38);
    assert_eq!(&payload[..8], &0x0102_0304_0506_0708u64.to_le_bytes());
    assert_eq!(payload[8], 2);
    assert_eq!(&payload[9..17], &[0x07, 0, 0, 0, 0, 0, 0, 0]);
    // Position is 2.5, 1.0, -3.25 and velocity 0.5, -1.25, 0.0 in that order.
    assert_eq!(&payload[17..21], &[0x00, 0x00, 0x20, 0x40]);
    assert_eq!(&payload[21..25], &[0x00, 0x00, 0x80, 0x3f]);
    assert_eq!(&payload[25..29], &[0x00, 0x00, 0x50, 0xc0]);
    assert_eq!(&payload[29..33], &[0x00, 0x00, 0x00, 0x3f]);
    assert_eq!(&payload[33..37], &[0x00, 0x00, 0xa0, 0xbf]);
    assert_eq!(&payload[37..41], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[41..45], &[0x00, 0x00, 0xa0, 0x3f]);
    assert_eq!(payload[45], 13);
    assert_eq!(payload[46], 1);
    // The second record starts one fixed 38-byte stride later.
    assert_eq!(&payload[47..55], &[0x09, 0, 0, 0, 0, 0, 0, 0]);
    assert_eq!(payload[83], 20);
    assert_eq!(payload[84], 0);
    assert_eq!(
        mornlea_protocol::PassiveState::decode(&payload).expect("decode"),
        state
    );
}

#[test]
fn passive_state_rejects_invalid_records_and_malformed_payload() {
    let record = mornlea_protocol::PassiveStateRecord {
        id: 7,
        position: [1.0, 2.0, 3.0],
        velocity: [0.0, 0.0, 0.0],
        yaw: 0.5,
        health: 10,
        grazing: 0,
    };

    let mut zero_id = record;
    zero_id.id = 0;
    let mut bad_velocity = record;
    bad_velocity.velocity = [f32::INFINITY, 0.0, 0.0];
    let mut bad_health = record;
    bad_health.health = 0;
    let mut over_health = record;
    over_health.health = 21;
    let mut bad_grazing = record;
    bad_grazing.grazing = 2;
    for bad in [zero_id, bad_velocity, bad_health, over_health, bad_grazing] {
        assert!(
            mornlea_protocol::PassiveState::new(1, vec![bad]).is_err(),
            "accepted invalid passive state record"
        );
    }
    assert!(mornlea_protocol::PassiveState::new(1, vec![record, record]).is_err());
    assert!(mornlea_protocol::PassiveState::new(1, Vec::new()).is_err());

    let valid = mornlea_protocol::PassiveState::new(1, vec![record]).expect("state");
    let payload = valid.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::PassiveState::decode(&payload[..length]).is_err(),
            "accepted truncated passive state at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::PassiveState::decode(&padded),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut mismatched = payload.clone();
    mismatched[8] = 2;
    assert_eq!(
        mornlea_protocol::PassiveState::decode(&mismatched),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut empty = payload.clone();
    empty[8] = 0;
    assert!(mornlea_protocol::PassiveState::decode(&empty).is_err());
    let mut over = payload.clone();
    over[8] = mornlea_protocol::MAX_PASSIVE_RECORDS + 1;
    assert!(mornlea_protocol::PassiveState::decode(&over).is_err());
    // A grazing byte outside 0/1 is a range violation the Go decoder reports
    // after the last field, not a silently accepted boolean.
    let mut bad_grazing_payload = payload.clone();
    bad_grazing_payload[46] = 2;
    assert!(mornlea_protocol::PassiveState::decode(&bad_grazing_payload).is_err());
}

#[test]
fn projectile_spawn_round_trip_preserves_batch_bytes() {
    let spawn = mornlea_protocol::ProjectileSpawn::new(
        0x0102_0304_0506_0708,
        vec![
            mornlea_protocol::ProjectileSpawnRecord {
                id: 7,
                kind: mornlea_protocol::PROJECTILE_KIND_ARROW,
                dimension: mornlea_domain::Dimension::OVERWORLD,
                position: [2.5, 1.0, -3.25],
                velocity: [0.5, -1.25, 0.0],
            },
            mornlea_protocol::ProjectileSpawnRecord {
                id: 9,
                kind: mornlea_protocol::PROJECTILE_KIND_SHARD,
                dimension: mornlea_domain::Dimension::DEPTHS,
                position: [-8.5, 65.5, 12.75],
                velocity: [1.0, 2.0, 3.0],
            },
        ],
    )
    .expect("spawn");
    let payload = spawn.encode();
    assert_eq!(mornlea_protocol::ProjectileSpawn::PACKET_ID, 29);
    assert_eq!(payload.len(), 8 + 1 + 2 * 37);
    assert_eq!(&payload[..8], &0x0102_0304_0506_0708u64.to_le_bytes());
    assert_eq!(payload[8], 2);
    assert_eq!(&payload[9..17], &[0x07, 0, 0, 0, 0, 0, 0, 0]);
    assert_eq!(payload[17], mornlea_protocol::PROJECTILE_KIND_ARROW);
    assert_eq!(&payload[18..22], &[0x00, 0x00, 0x00, 0x00]);
    assert_eq!(&payload[22..26], &[0x00, 0x00, 0x20, 0x40]);
    assert_eq!(&payload[34..38], &[0x00, 0x00, 0x00, 0x3f]);
    // The second record starts one fixed 37-byte stride later.
    assert_eq!(&payload[46..54], &[0x09, 0, 0, 0, 0, 0, 0, 0]);
    assert_eq!(payload[54], mornlea_protocol::PROJECTILE_KIND_SHARD);
    assert_eq!(&payload[55..59], &[0x01, 0x00, 0x00, 0x00]);
    assert_eq!(
        mornlea_protocol::ProjectileSpawn::decode(&payload).expect("decode"),
        spawn
    );
}

#[test]
fn projectile_spawn_rejects_invalid_records_and_malformed_payload() {
    let record = mornlea_protocol::ProjectileSpawnRecord {
        id: 7,
        kind: mornlea_protocol::PROJECTILE_KIND_SHARD,
        dimension: mornlea_domain::Dimension::OVERWORLD,
        position: [1.0, 2.0, 3.0],
        velocity: [0.5, 0.0, 0.0],
    };

    let mut zero_id = record;
    zero_id.id = 0;
    let mut bad_kind = record;
    bad_kind.kind = 2;
    let mut bad_velocity = record;
    bad_velocity.velocity = [f32::NAN, 0.0, 0.0];
    for bad in [zero_id, bad_kind, bad_velocity] {
        assert!(
            mornlea_protocol::ProjectileSpawn::new(1, vec![bad]).is_err(),
            "accepted invalid projectile spawn record"
        );
    }
    assert!(mornlea_protocol::ProjectileSpawn::new(1, vec![record, record]).is_err());
    assert!(mornlea_protocol::ProjectileSpawn::new(1, Vec::new()).is_err());
    // A projectile is a peer-reachable entity, so the depths are a legal
    // dimension here even though mob spawns are overworld-only.
    let depths = mornlea_protocol::ProjectileSpawnRecord {
        dimension: mornlea_domain::Dimension::DEPTHS,
        ..record
    };
    assert!(mornlea_protocol::ProjectileSpawn::new(1, vec![depths]).is_ok());

    let valid = mornlea_protocol::ProjectileSpawn::new(1, vec![record]).expect("spawn");
    let payload = valid.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::ProjectileSpawn::decode(&payload[..length]).is_err(),
            "accepted truncated projectile spawn at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::ProjectileSpawn::decode(&padded),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut mismatched = payload.clone();
    mismatched[8] = 2;
    assert_eq!(
        mornlea_protocol::ProjectileSpawn::decode(&mismatched),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut empty = payload.clone();
    empty[8] = 0;
    assert!(mornlea_protocol::ProjectileSpawn::decode(&empty).is_err());
    let mut over = payload.clone();
    over[8] = mornlea_protocol::MAX_PROJECTILE_RECORDS + 1;
    assert!(mornlea_protocol::ProjectileSpawn::decode(&over).is_err());
}

#[test]
fn projectile_state_round_trip_preserves_batch_bytes() {
    let state = mornlea_protocol::ProjectileState::new(
        0x0102_0304_0506_0708,
        vec![
            mornlea_protocol::ProjectileStateRecord {
                id: 7,
                position: [2.5, 1.0, -3.25],
            },
            mornlea_protocol::ProjectileStateRecord {
                id: 9,
                position: [-8.5, 65.5, 12.75],
            },
        ],
    )
    .expect("state");
    let payload = state.encode();
    assert_eq!(mornlea_protocol::ProjectileState::PACKET_ID, 30);
    assert_eq!(payload.len(), 8 + 1 + 2 * 20);
    assert_eq!(&payload[..8], &0x0102_0304_0506_0708u64.to_le_bytes());
    assert_eq!(payload[8], 2);
    assert_eq!(&payload[9..17], &[0x07, 0, 0, 0, 0, 0, 0, 0]);
    assert_eq!(&payload[17..21], &[0x00, 0x00, 0x20, 0x40]);
    assert_eq!(&payload[21..25], &[0x00, 0x00, 0x80, 0x3f]);
    assert_eq!(&payload[25..29], &[0x00, 0x00, 0x50, 0xc0]);
    // The second record starts one fixed 20-byte stride later.
    assert_eq!(&payload[29..37], &[0x09, 0, 0, 0, 0, 0, 0, 0]);
    assert_eq!(
        mornlea_protocol::ProjectileState::decode(&payload).expect("decode"),
        state
    );
}

#[test]
fn projectile_state_rejects_invalid_records_and_malformed_payload() {
    let record = mornlea_protocol::ProjectileStateRecord {
        id: 7,
        position: [1.0, 2.0, 3.0],
    };

    let mut zero_id = record;
    zero_id.id = 0;
    let mut bad_position = record;
    bad_position.position = [f32::INFINITY, 0.0, 0.0];
    for bad in [zero_id, bad_position] {
        assert!(
            mornlea_protocol::ProjectileState::new(1, vec![bad]).is_err(),
            "accepted invalid projectile state record"
        );
    }
    assert!(mornlea_protocol::ProjectileState::new(1, vec![record, record]).is_err());
    assert!(mornlea_protocol::ProjectileState::new(1, Vec::new()).is_err());

    let valid = mornlea_protocol::ProjectileState::new(1, vec![record]).expect("state");
    let payload = valid.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::ProjectileState::decode(&payload[..length]).is_err(),
            "accepted truncated projectile state at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::ProjectileState::decode(&padded),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut mismatched = payload.clone();
    mismatched[8] = 2;
    assert_eq!(
        mornlea_protocol::ProjectileState::decode(&mismatched),
        Err(mornlea_protocol::ProtocolError::Truncated)
    );
    let mut empty = payload.clone();
    empty[8] = 0;
    assert!(mornlea_protocol::ProjectileState::decode(&empty).is_err());
    let mut over = payload.clone();
    over[8] = mornlea_protocol::MAX_PROJECTILE_RECORDS + 1;
    assert!(mornlea_protocol::ProjectileState::decode(&over).is_err());
}

#[test]
fn chat_event_round_trip_preserves_golden_bytes() {
    let accepted = mornlea_protocol::ChatEvent::new(mornlea_protocol::ChatEvent {
        event_id: 0x0102_0304_0506_0708,
        player_id: mornlea_protocol::PlayerId::new([
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff,
        ])
        .expect("player id"),
        player_name: "陈".to_owned(),
        companion_id: mornlea_protocol::CompanionId::new([
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xfe,
        ])
        .expect("companion id"),
        companion_name: "Mira".to_owned(),
        kind: mornlea_protocol::CHAT_EVENT_ACCEPTED,
        reject_reason: mornlea_protocol::CHAT_REJECT_NONE,
        command: "follow me".to_owned(),
        speech: String::new(),
    })
    .expect("accepted chat event");
    let payload = accepted.encode();
    assert_eq!(mornlea_protocol::ChatEvent::PACKET_ID, 16);
    // 8 event ID + 16 player ID + (1 + 3 name) + 16 companion ID
    // + (1 + 4 name) + 1 kind + 1 reason + (1 + 9 command).
    assert_eq!(payload.len(), 61);
    assert_eq!(&payload[..8], &0x0102_0304_0506_0708u64.to_le_bytes());
    assert_eq!(&payload[8..24], &accepted.player_id.bytes());
    assert_eq!(payload[24], 3);
    assert_eq!(&payload[25..28], &[0xe9, 0x99, 0x88]);
    assert_eq!(&payload[28..44], &accepted.companion_id.bytes());
    assert_eq!(payload[44], 4);
    assert_eq!(&payload[45..49], b"Mira");
    assert_eq!(payload[49], mornlea_protocol::CHAT_EVENT_ACCEPTED);
    assert_eq!(payload[50], mornlea_protocol::CHAT_REJECT_NONE);
    assert_eq!(payload[51], 9);
    assert_eq!(&payload[52..61], b"follow me");
    assert_eq!(
        mornlea_protocol::ChatEvent::decode(&payload).expect("decode"),
        accepted
    );

    // A companion speech event reuses the same text slot with a tighter bound,
    // and must not restate the player command.
    let speech = mornlea_protocol::ChatEvent::new(mornlea_protocol::ChatEvent {
        kind: mornlea_protocol::CHAT_EVENT_COMPANION_SPEECH,
        command: String::new(),
        speech: "hello there".to_owned(),
        ..accepted.clone()
    })
    .expect("speech chat event");
    let payload = speech.encode();
    assert_eq!(payload.len(), 63);
    assert_eq!(payload[49], mornlea_protocol::CHAT_EVENT_COMPANION_SPEECH);
    assert_eq!(payload[51], 11);
    assert_eq!(&payload[52..63], b"hello there");
    assert_eq!(
        mornlea_protocol::ChatEvent::decode(&payload).expect("decode"),
        speech
    );

    // A rejection that never addressed a companion carries the absent identity
    // and no text at all, which is why the identity needs an absent form.
    let unaddressed = mornlea_protocol::ChatEvent::new(mornlea_protocol::ChatEvent {
        companion_id: mornlea_protocol::CompanionId::NONE,
        companion_name: String::new(),
        kind: mornlea_protocol::CHAT_EVENT_REJECTED,
        reject_reason: mornlea_protocol::CHAT_REJECT_INVALID_FORMAT,
        command: String::new(),
        speech: String::new(),
        ..accepted.clone()
    })
    .expect("unaddressed chat event");
    let payload = unaddressed.encode();
    assert_eq!(payload.len(), 48);
    assert_eq!(&payload[28..44], &[0u8; 16]);
    assert_eq!(payload[44], 0);
    assert_eq!(payload[45], mornlea_protocol::CHAT_EVENT_REJECTED);
    assert_eq!(payload[46], mornlea_protocol::CHAT_REJECT_INVALID_FORMAT);
    assert_eq!(payload[47], 0);
    assert_eq!(
        mornlea_protocol::ChatEvent::decode(&payload).expect("decode"),
        unaddressed
    );
}

#[test]
fn chat_event_rejects_invalid_kind_combinations_and_malformed_payload() {
    let base = mornlea_protocol::ChatEvent {
        event_id: 0x0102_0304_0506_0708,
        player_id: mornlea_protocol::PlayerId::new([
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xff,
        ])
        .expect("player id"),
        player_name: "陈".to_owned(),
        companion_id: mornlea_protocol::CompanionId::new([
            0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd,
            0xee, 0xfe,
        ])
        .expect("companion id"),
        companion_name: "Mira".to_owned(),
        kind: mornlea_protocol::CHAT_EVENT_ACCEPTED,
        reject_reason: mornlea_protocol::CHAT_REJECT_NONE,
        command: "follow me".to_owned(),
        speech: String::new(),
    };

    let mut zero_event_id = base.clone();
    zero_event_id.event_id = 0;
    let mut padded_player_name = base.clone();
    padded_player_name.player_name = " 陈".to_owned();
    let mut unknown_kind = base.clone();
    unknown_kind.kind = 10;
    let mut zero_kind = base.clone();
    zero_kind.kind = 0;
    let mut accepted_with_reason = base.clone();
    accepted_with_reason.reject_reason = mornlea_protocol::CHAT_REJECT_QUEUE_FULL;
    let mut accepted_with_speech = base.clone();
    accepted_with_speech.speech = "hello".to_owned();
    let mut spaced_companion_name = base.clone();
    spaced_companion_name.companion_name = "has space".to_owned();
    let mut blank_command = base.clone();
    blank_command.command = String::new();
    let mut padded_command = base.clone();
    padded_command.command = " follow me".to_owned();
    let mut control_command = base.clone();
    control_command.command = "follow\u{0} me".to_owned();
    for bad in [
        zero_event_id,
        padded_player_name,
        unknown_kind,
        zero_kind,
        accepted_with_reason,
        accepted_with_speech,
        spaced_companion_name,
        blank_command,
        padded_command,
        control_command,
    ] {
        assert!(
            bad.validate().is_err(),
            "accepted invalid accepted chat event"
        );
    }

    // A format rejection must not leak a companion identity or the command.
    let mut leaking_format = base.clone();
    leaking_format.kind = mornlea_protocol::CHAT_EVENT_REJECTED;
    leaking_format.reject_reason = mornlea_protocol::CHAT_REJECT_INVALID_FORMAT;
    leaking_format.companion_name = String::new();
    leaking_format.command = String::new();
    assert!(
        leaking_format.validate().is_err(),
        "accepted a format rejection that keeps a companion identity"
    );
    let clean_format = mornlea_protocol::ChatEvent {
        companion_id: mornlea_protocol::CompanionId::NONE,
        kind: mornlea_protocol::CHAT_EVENT_REJECTED,
        reject_reason: mornlea_protocol::CHAT_REJECT_INVALID_FORMAT,
        companion_name: String::new(),
        command: String::new(),
        ..base.clone()
    };
    assert!(clean_format.validate().is_ok());
    // Reason 3 is reserved and unassigned.
    let mut reserved_reason = clean_format.clone();
    reserved_reason.reject_reason = 3;
    assert!(reserved_reason.validate().is_err());

    // A queue-full rejection must carry the full companion identity.
    let mut anonymous_queue_full = base.clone();
    anonymous_queue_full.kind = mornlea_protocol::CHAT_EVENT_REJECTED;
    anonymous_queue_full.reject_reason = mornlea_protocol::CHAT_REJECT_QUEUE_FULL;
    anonymous_queue_full.companion_id = mornlea_protocol::CompanionId::NONE;
    assert!(anonymous_queue_full.validate().is_err());

    // Task events restate the command and keep the reason slot empty.
    let mut task_with_reason = base.clone();
    task_with_reason.kind = mornlea_protocol::CHAT_EVENT_TASK_STARTED;
    task_with_reason.reject_reason = mornlea_protocol::CHAT_REJECT_QUEUE_FULL;
    assert!(task_with_reason.validate().is_err());
    let task = mornlea_protocol::ChatEvent {
        kind: mornlea_protocol::CHAT_EVENT_TASK_STARTED,
        ..base.clone()
    };
    assert!(task.validate().is_ok());

    // A failed task carries a task failure reason, not a chat rejection reason.
    let mut failed_with_chat_reason = base.clone();
    failed_with_chat_reason.kind = mornlea_protocol::CHAT_EVENT_TASK_FAILED;
    failed_with_chat_reason.reject_reason = mornlea_protocol::CHAT_REJECT_QUEUE_FULL;
    assert!(failed_with_chat_reason.validate().is_err());
    let failed = mornlea_protocol::ChatEvent {
        kind: mornlea_protocol::CHAT_EVENT_TASK_FAILED,
        reject_reason: mornlea_protocol::TASK_FAIL_WORLD_CHANGED,
        ..base.clone()
    };
    assert!(failed.validate().is_ok());

    // A speech event must not restate the command and must carry bounded text.
    let mut speech_with_command = base.clone();
    speech_with_command.kind = mornlea_protocol::CHAT_EVENT_COMPANION_SPEECH;
    speech_with_command.speech = "hello".to_owned();
    assert!(speech_with_command.validate().is_err());
    let mut empty_speech = base.clone();
    empty_speech.kind = mornlea_protocol::CHAT_EVENT_COMPANION_SPEECH;
    assert!(empty_speech.validate().is_err());
    let mut over_long_speech = base.clone();
    over_long_speech.kind = mornlea_protocol::CHAT_EVENT_COMPANION_SPEECH;
    over_long_speech.speech = "x".repeat(257);
    assert!(over_long_speech.validate().is_err());
    let mut padded_speech = base.clone();
    padded_speech.kind = mornlea_protocol::CHAT_EVENT_COMPANION_SPEECH;
    padded_speech.speech = " hello".to_owned();
    assert!(padded_speech.validate().is_err());
    let speech = mornlea_protocol::ChatEvent {
        kind: mornlea_protocol::CHAT_EVENT_COMPANION_SPEECH,
        command: String::new(),
        speech: "hello".to_owned(),
        ..base.clone()
    };
    assert!(speech.validate().is_ok());

    let payload = base.encode();
    for length in 0..payload.len() {
        assert!(
            mornlea_protocol::ChatEvent::decode(&payload[..length]).is_err(),
            "accepted truncated chat event at {length}"
        );
    }
    let mut padded = payload.clone();
    padded.push(0x00);
    assert_eq!(
        mornlea_protocol::ChatEvent::decode(&padded),
        Err(mornlea_protocol::ProtocolError::TrailingBytes)
    );
    // An unknown kind is rejected by the same validation the Go decoder
    // applies after the last field, before the text slot is interpreted.
    let mut bad_kind = payload.clone();
    bad_kind[49] = 10;
    assert!(mornlea_protocol::ChatEvent::decode(&bad_kind).is_err());
}

/// Section field offsets inside one logical snapshot payload, walked with the
/// same layout the encoder writes so mutation tests target real bytes.
struct SnapshotSectionOffsets {
    y: usize,
    storage: usize,
    bits: usize,
    palette_count: usize,
    first_palette: usize,
    word_count: usize,
    first_word: usize,
}

fn snapshot_section_offsets(logical: &[u8]) -> Vec<SnapshotSectionOffsets> {
    let mut offset = 20;
    let (count, used) =
        mornlea_protocol::decode_uvarint(&logical[offset..]).expect("section count");
    offset += used;
    let mut offsets = Vec::with_capacity(count as usize);
    for _ in 0..count {
        let y = offset;
        let storage = offset + 1;
        offset += 2;
        let mut entry = SnapshotSectionOffsets {
            y,
            storage,
            bits: 0,
            palette_count: 0,
            first_palette: 0,
            word_count: 0,
            first_word: 0,
        };
        match logical[storage] {
            0 => offset += 2,
            1 => {
                entry.bits = offset;
                offset += 1;
                entry.palette_count = offset;
                let (palette_count, used) =
                    mornlea_protocol::decode_uvarint(&logical[offset..]).expect("palette count");
                offset += used;
                entry.first_palette = offset;
                offset += palette_count as usize * 2;
                entry.word_count = offset;
                let (word_count, used) =
                    mornlea_protocol::decode_uvarint(&logical[offset..]).expect("word count");
                offset += used;
                entry.first_word = offset;
                offset += word_count as usize * 8;
            }
            _ => {
                entry.bits = offset;
                offset += 1;
                entry.word_count = offset;
                let (word_count, used) =
                    mornlea_protocol::decode_uvarint(&logical[offset..]).expect("word count");
                offset += used;
                entry.first_word = offset;
                offset += word_count as usize * 8;
            }
        }
        offsets.push(entry);
    }
    offsets
}

/// Packs a section's block slots the way the Go fixture builder does, so the
/// golden snapshot is the same logical value the committed fixture carries.
fn snapshot_packed_words(bits: u8, modulus: usize, seed: usize) -> Vec<u64> {
    let per_word = 64 / bits as usize;
    let words = (mornlea_protocol::BLOCKS_PER_SECTION + per_word - 1) / per_word;
    let mut packed = vec![0u64; words];
    for index in 0..mornlea_protocol::BLOCKS_PER_SECTION {
        let value = ((index + seed) % modulus) as u64;
        packed[index / per_word] |= value << ((index % per_word) * bits as usize);
    }
    packed
}

/// The committed fixture's logical snapshot: overworld chunk (-3, 7) at
/// revision 19, cycling through all three section storages.
fn golden_chunk_snapshot() -> mornlea_protocol::ChunkSnapshot {
    let mut sections = Vec::with_capacity(mornlea_protocol::SECTIONS_PER_CHUNK);
    for y in 0..mornlea_protocol::SECTIONS_PER_CHUNK {
        let section = match y % 4 {
            0 => mornlea_protocol::SectionData::single(y as i32, (y % 6) as u16),
            1 => mornlea_protocol::SectionData::indexed(
                y as i32,
                4,
                vec![0, 2, 3],
                snapshot_packed_words(4, 3, y),
            ),
            2 => mornlea_protocol::SectionData::indexed(
                y as i32,
                8,
                vec![0, 1, 2, 3, 4, 5, 27, 34],
                snapshot_packed_words(8, 8, y),
            ),
            _ => mornlea_protocol::SectionData::direct(y as i32, snapshot_packed_words(15, 35, y)),
        };
        sections.push(section);
    }
    mornlea_protocol::ChunkSnapshot::new(mornlea_domain::Dimension::OVERWORLD, -3, 7, 19, sections)
        .expect("golden snapshot")
}

fn read_go_snapshot_fixture() -> Vec<u8> {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../../../packages/shared/network/codec/testdata/chunk-snapshot-v1.bin");
    fs::read(&path).unwrap_or_else(|err| panic!("read {}: {err}", path.display()))
}

fn snapshot_envelope(decoded_length: u32, compressed: &[u8]) -> Vec<u8> {
    let mut payload = Vec::with_capacity(8 + compressed.len());
    payload.extend_from_slice(&decoded_length.to_le_bytes());
    payload.extend_from_slice(&(compressed.len() as u32).to_le_bytes());
    payload.extend_from_slice(compressed);
    payload
}

#[test]
fn chunk_snapshot_round_trip_preserves_golden_bytes() {
    let snapshot = golden_chunk_snapshot();
    let logical = snapshot.encode_logical();
    // The Go fixture's logical payload length, derived from its envelope.
    assert_eq!(logical.len(), 86_295);
    assert_eq!(snapshot.logical_size(), 86_295);
    // Identity fields lead the logical payload: dimension, chunk, revision,
    // then the 24-section count.
    assert_eq!(&logical[0..4], &0i32.to_le_bytes());
    assert_eq!(&logical[4..8], &(-3i32).to_le_bytes());
    assert_eq!(&logical[8..12], &7i32.to_le_bytes());
    assert_eq!(&logical[12..20], &19u64.to_le_bytes());
    assert_eq!(logical[20], 24);
    assert_eq!(mornlea_protocol::ChunkSnapshot::PACKET_ID, 0);

    // Decode exactness: the committed fixture's logical payload must be
    // reproduced byte for byte by decoding it, which the logical layer can
    // prove because it is uncompressed and deterministic.
    let fixture = read_go_snapshot_fixture();
    let envelope =
        mornlea_protocol::ChunkSnapshot::decode_envelope(&fixture).expect("fixture envelope");
    assert_eq!(envelope.decoded_length, 86_295);
    assert_eq!(envelope.compressed.len(), 439);
    assert_eq!(envelope.decompress().expect("fixture logical"), logical);
    assert_eq!(
        mornlea_protocol::ChunkSnapshot::decode(&fixture).expect("fixture decode"),
        snapshot
    );
    assert_eq!(
        mornlea_protocol::ChunkSnapshot::decode_logical(&logical).expect("logical decode"),
        snapshot
    );
}

#[test]
fn chunk_snapshot_round_trips_through_committed_fixture() {
    let fixture = read_go_snapshot_fixture();
    let decoded = mornlea_protocol::ChunkSnapshot::decode(&fixture).expect("fixture decode");
    let reencoded = decoded.encode();
    let again = mornlea_protocol::ChunkSnapshot::decode(&reencoded).expect("re-encoded decode");
    assert_eq!(again, decoded);

    // The compressed block payload legitimately differs from the Go encoder's,
    // so only the self-describing frame envelope and the logical snapshot are
    // compared. The zstd magic, the frame header descriptor, the four-byte
    // content size, and the trailing xxhash-64 content checksum are the parts
    // that are byte-identical across the two implementations.
    let fixture_frame = &fixture[8..];
    let rust_frame = &reencoded[8..];
    assert_eq!(&fixture_frame[0..4], &rust_frame[0..4]);
    assert_eq!(&fixture_frame[0..4], &[0x28, 0xb5, 0x2f, 0xfd]);
    assert_eq!(fixture_frame[4], rust_frame[4]);
    assert_eq!(fixture_frame[4], 0xa4);
    assert_eq!(&fixture_frame[5..9], &rust_frame[5..9]);
    assert_eq!(
        &fixture_frame[5..9],
        &(86_295u32).to_le_bytes(),
        "frame content size must equal the logical length"
    );
    assert_eq!(
        &fixture_frame[fixture_frame.len() - 4..],
        &rust_frame[rust_frame.len() - 4..],
        "xxhash-64 content checksum must match across implementations"
    );
    assert_eq!(&reencoded[0..4], &(86_295u32).to_le_bytes());
}

#[test]
fn chunk_snapshot_rejects_malformed_envelope_and_bounds() {
    let snapshot = golden_chunk_snapshot();
    let logical = snapshot.encode_logical();
    let valid = snapshot.encode();

    for length in 0..8 {
        assert!(
            mornlea_protocol::ChunkSnapshot::decode(&valid[..length]).is_err(),
            "accepted truncated snapshot envelope at {length}"
        );
    }

    let mut mismatched = valid.clone();
    let declared = u32::from_le_bytes(mismatched[4..8].try_into().unwrap());
    mismatched[4..8].copy_from_slice(&(declared + 1).to_le_bytes());
    assert!(mornlea_protocol::ChunkSnapshot::decode(&mismatched).is_err());

    let mut trailing = valid.clone();
    trailing.push(0);
    assert!(mornlea_protocol::ChunkSnapshot::decode(&trailing).is_err());

    let mut short_declared = valid.clone();
    short_declared[0..4].copy_from_slice(&(logical.len() as u32 + 1).to_le_bytes());
    assert!(mornlea_protocol::ChunkSnapshot::decode(&short_declared).is_err());

    let mut long_declared = valid.clone();
    long_declared[0..4].copy_from_slice(&(logical.len() as u32 - 1).to_le_bytes());
    assert!(mornlea_protocol::ChunkSnapshot::decode(&long_declared).is_err());

    // The frame's own content checksum is enforced, so a corrupted tail is a
    // corrupt payload rather than a silently accepted one.
    let mut bad_checksum = valid.clone();
    let last = bad_checksum.len() - 1;
    bad_checksum[last] ^= 0xff;
    assert!(mornlea_protocol::ChunkSnapshot::decode(&bad_checksum).is_err());

    // Bounds are rejected before decompression: a non-zstd payload of the
    // declared size still yields the bound failure rather than a frame failure.
    for size in [
        mornlea_protocol::MAX_COMPRESSED_SNAPSHOT - 1,
        mornlea_protocol::MAX_COMPRESSED_SNAPSHOT,
    ] {
        let payload = snapshot_envelope(1, &vec![0u8; size]);
        assert_eq!(
            mornlea_protocol::ChunkSnapshot::decode(&payload),
            Err(mornlea_protocol::ProtocolError::Truncated),
            "compressed length {size} must reach the zstd decoder"
        );
    }
    let oversized = snapshot_envelope(1, &vec![0u8; mornlea_protocol::MAX_COMPRESSED_SNAPSHOT + 1]);
    assert_eq!(
        mornlea_protocol::ChunkSnapshot::decode(&oversized),
        Err(mornlea_protocol::ProtocolError::FrameTooLarge)
    );
    let oversized_decoded =
        snapshot_envelope(mornlea_protocol::MAX_DECODED_SNAPSHOT as u32 + 1, &[0u8]);
    assert_eq!(
        mornlea_protocol::ChunkSnapshot::decode(&oversized_decoded),
        Err(mornlea_protocol::ProtocolError::FrameTooLarge)
    );

    // A frame that decodes past the declared ceiling is an expansion bomb and
    // must be rejected rather than decompressed into a larger buffer.
    let bomb = vec![0u8; mornlea_protocol::MAX_DECODED_SNAPSHOT + 1];
    let frame = mornlea_protocol::compress_logical(&bomb).expect("compress bomb");
    let payload = snapshot_envelope(mornlea_protocol::MAX_DECODED_SNAPSHOT as u32, &frame);
    assert!(mornlea_protocol::ChunkSnapshot::decode(&payload).is_err());
}

#[test]
fn chunk_snapshot_rejects_malformed_logical_payload() {
    let snapshot = golden_chunk_snapshot();
    let logical = snapshot.encode_logical();
    let offsets = snapshot_section_offsets(&logical);

    let mut cases: Vec<(&str, Box<dyn Fn(&mut Vec<u8>)>)> = Vec::new();
    cases.push((
        "section count",
        Box::new(|data: &mut Vec<u8>| data[20] = mornlea_protocol::SECTIONS_PER_CHUNK as u8 - 1),
    ));
    cases.push((
        "section order",
        Box::new(|data: &mut Vec<u8>| data[offsets[4].y] = 5),
    ));
    // An unknown storage byte is the future-version rejection for this family:
    // the wire defines exactly three storage kinds and this version has no
    // repair for a fourth.
    cases.push((
        "unknown storage",
        Box::new(|data: &mut Vec<u8>| data[offsets[0].storage] = 3),
    ));
    cases.push((
        "indexed bits",
        Box::new(|data: &mut Vec<u8>| data[offsets[1].bits] = 5),
    ));
    cases.push((
        "palette count before allocation",
        Box::new(|data: &mut Vec<u8>| data[offsets[1].palette_count] = 17),
    ));
    cases.push((
        "duplicate palette block ID",
        Box::new(|data: &mut Vec<u8>| {
            let at = offsets[1].first_palette;
            let first = u16::from_le_bytes(data[at..at + 2].try_into().unwrap());
            data[at + 2..at + 4].copy_from_slice(&first.to_le_bytes());
        }),
    ));
    cases.push((
        "invalid palette block ID",
        Box::new(|data: &mut Vec<u8>| {
            let at = offsets[1].first_palette;
            data[at..at + 2].copy_from_slice(&(1u16 << 15).to_le_bytes());
        }),
    ));
    cases.push((
        "invalid palette slot",
        Box::new(|data: &mut Vec<u8>| {
            let at = offsets[1].first_word;
            let word = u64::from_le_bytes(data[at..at + 8].try_into().unwrap());
            data[at..at + 8].copy_from_slice(&(word | 0xf).to_le_bytes());
        }),
    ));
    cases.push((
        "indexed word count before allocation",
        Box::new(|data: &mut Vec<u8>| data[offsets[1].word_count] = 0),
    ));
    cases.push((
        "direct bits",
        Box::new(|data: &mut Vec<u8>| data[offsets[3].bits] = 14),
    ));
    cases.push((
        "direct word count before allocation",
        Box::new(|data: &mut Vec<u8>| data[offsets[3].word_count] = 0),
    ));
    cases.push((
        "direct unused high bits",
        Box::new(|data: &mut Vec<u8>| {
            let at = offsets[3].first_word;
            let word = u64::from_le_bytes(data[at..at + 8].try_into().unwrap());
            data[at..at + 8].copy_from_slice(&(word | 1 << 60).to_le_bytes());
        }),
    ));
    cases.push((
        "logical trailing byte",
        Box::new(|data: &mut Vec<u8>| data.push(0)),
    ));

    for (name, mutate) in cases {
        let mut malformed = logical.clone();
        mutate(&mut malformed);
        let frame = mornlea_protocol::compress_logical(&malformed).expect("compress malformed");
        let payload = snapshot_envelope(malformed.len() as u32, &frame);
        assert!(
            mornlea_protocol::ChunkSnapshot::decode(&payload).is_err(),
            "malformed logical payload accepted: {name}"
        );
    }

    // No prefix of a valid logical payload decodes as a shorter snapshot. The
    // stride keeps the case count bounded while still walking every section
    // boundary, and the leading window is exhaustive because the header and
    // the first section descriptors are where a partial read is most likely to
    // be mistaken for a whole field.
    let lengths: Vec<usize> = (0..48).chain((48..logical.len()).step_by(256)).collect();
    for length in lengths {
        let frame =
            mornlea_protocol::compress_logical(&logical[..length]).expect("compress prefix");
        let payload = snapshot_envelope(length as u32, &frame);
        assert!(
            mornlea_protocol::ChunkSnapshot::decode(&payload).is_err(),
            "accepted truncated logical snapshot at {length}"
        );
    }
}

/// Renders bytes as lowercase hexadecimal, matching the Go producer's
/// `encoding/hex` output so both sides publish the same payload text.
fn hex_lower(bytes: &[u8]) -> String {
    let mut rendered = String::with_capacity(bytes.len() * 2);
    for byte in bytes {
        rendered.push_str(&format!("{byte:02x}"));
    }
    rendered
}

/// Dispatches one framing case through the real Rust consumer named by the
/// case's parsed operation.
///
/// The dispatch is a match over real consumers rather than a name count: the
/// accepted branch runs `read_frame` on the case input and the rejected branch
/// classifies the error the consumer actually returned. A consumer that decodes
/// a different result, or a mutation of the decoded packet ID, changes the
/// normalized outcome and fails the assertion instead of merely renaming a
/// case. An unclassified rejection is a hard failure because the corpus only
/// records categories both languages can name.
fn dispatch_frame(case: &runtime_corpus::FrozenCase) -> serde_json::Value {
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
        other => panic!("unsupported framing operation for {}: {other}", case.id),
    }
}

#[test]
fn corpus_frame() {
    let root = runtime_corpus::find_repo_root();

    let valid = runtime_corpus::load_case("protocol.frame/45/valid");
    assert_eq!(valid.family, "protocol.frame");
    assert_eq!(valid.operation, "decode");
    runtime_corpus::assert_normalized(&valid, dispatch_frame(&valid));

    // Negative mutation check: mutating the decoded ID must fail assertion.
    let (decoded_id, decoded_payload, _) = mornlea_protocol::read_frame(&valid.input)
        .expect("read_frame should succeed on valid framing case");
    let mutated = serde_json::json!({
        "category": "frame",
        "fields": {
            "packet_id": decoded_id + 1,
            "payload": hex_lower(&decoded_payload),
            "payload_len": decoded_payload.len()
        },
        "kind": "ok"
    });
    assert_ne!(valid.normalized, mutated);

    // The noncanonical length vector is executed but its manifest merge is a
    // separate controller step, so the consumer names the case entry by path
    // until the frozen manifest carries it. The expected outcome is the one the
    // independent Go producer recorded; the Rust consumer has to reproduce it
    // from the same bytes rather than agreeing with a Rust-generated value.
    let noncanonical_entry = serde_json::json!({
        "id": "protocol.frame/45/noncanonical-length",
        "family": "protocol.frame",
        "version": "45",
        "operation": "decode",
        "input": {
            "path": "testdata/runtime-migration/cases/frame/noncanonical-length.bin",
            "sha256": "sha256:431c114d525bb04ddd1b7434509fc9360452448041b2921b02b7852b85cc2d6c"
        },
        "input_format": "binary",
        "expected": {
            "path": "testdata/runtime-migration/cases/frame/noncanonical-length.expected.json",
            "sha256": "sha256:322f8e8aa056952c6e2990cc1f0b361ae868fe46c66ed2f644eb2ce120289d14"
        },
        "checkpoints": ["0"],
        "rust_consumer": "corpus_frame"
    });
    let noncanonical = runtime_corpus::load_case_file(
        &root,
        &noncanonical_entry,
        "protocol.frame/45/noncanonical-length",
    );
    assert_eq!(noncanonical.operation, "decode");
    let rejected = dispatch_frame(&noncanonical);
    assert_eq!(
        rejected["kind"], "error",
        "noncanonical length vector was accepted: {rejected}"
    );
    runtime_corpus::assert_normalized(&noncanonical, rejected);
}
