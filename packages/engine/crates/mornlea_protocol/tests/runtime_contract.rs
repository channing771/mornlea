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
