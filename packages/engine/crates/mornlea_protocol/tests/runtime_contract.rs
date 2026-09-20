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
