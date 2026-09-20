//! Foundation registration, dependency-direction, and framing contracts for
//! `mornlea_protocol`. Remaining packet-family ports land one inventory row
//! at a time.

use std::fs;
use std::path::PathBuf;

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
    let keys = production_dependency_keys(&read_manifest(env!("CARGO_MANIFEST_DIR")));
    assert_eq!(keys, ["mornlea_domain"]);
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
