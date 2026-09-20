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

