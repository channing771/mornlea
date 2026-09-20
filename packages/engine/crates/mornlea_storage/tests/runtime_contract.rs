//! Foundation registration and dependency-direction contracts for `mornlea_storage`.
//! Save-family ports remain recorded in the change ledger until each family
//! has a failing behavioral case of its own.

use mornlea_storage::{
    MAX_PASSIVE_MOBS, PASSIVE_CURRENT_SCHEMA, PASSIVE_ENVELOPE_VERSION, PASSIVE_MAX_FILE_LENGTH,
    PassiveMob, PassiveMobsSave, StorageError, crc32c_join, decode_passive_mobs,
    encode_passive_mobs,
};
use std::fs;
use std::path::PathBuf;

const FORBIDDEN_PRODUCTION_DEPS: &[&str] = &[
    "mornlea_protocol",
    "mornlea_engine",
    "mornlea_client",
    "mornlea_godot",
];

#[test]
fn crate_identity_matches_workspace_name() {
    assert_eq!(mornlea_storage::CRATE_NAME, env!("CARGO_PKG_NAME"));
}

#[test]
fn production_manifest_depends_only_on_domain() {
    let keys = production_dependency_keys(&read_manifest(env!("CARGO_MANIFEST_DIR")));
    assert_eq!(keys, ["mornlea_domain"]);
    for forbidden in FORBIDDEN_PRODUCTION_DEPS {
        assert!(
            !keys.iter().any(|key| key == forbidden),
            "mornlea_storage must not depend on {forbidden}"
        );
    }
}

#[test]
fn domain_does_not_depend_on_storage() {
    let domain_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR")).join("../mornlea_domain");
    let keys = production_dependency_keys(&read_manifest(domain_dir.to_str().unwrap()));
    assert!(
        !keys.iter().any(|key| key == "mornlea_storage"),
        "mornlea_domain must not depend on mornlea_storage"
    );
}

#[test]
fn inventory_assigns_save_families_to_this_crate() {
    let json = read_inventory();
    let owner = format!("\"eventual_owner\": \"{}\"", env!("CARGO_PKG_NAME"));
    let count = json.matches(&owner).count();
    assert_eq!(
        count, 7,
        "save inventory rows drifted; update intended family ports before implementing them"
    );
    for family in [
        "save.chunk",
        "save.companion",
        "save.hostile",
        "save.passive",
        "save.player",
        "save.region",
        "save.world-metadata",
    ] {
        assert!(
            json.contains(&format!("\"id\": \"{family}\"")),
            "missing save family {family}"
        );
    }
}

#[test]
fn passive_current_schema_round_trip_preserves_bytes() {
    let save = PassiveMobsSave {
        revision: 41,
        records: vec![
            passive_mob(7, 1.5, -3.25, 12.0),
            passive_mob(3, -64.0, 0.0, 319.999),
        ],
    };
    let encoded = encode_passive_mobs(&save).expect("encode passive mobs");
    assert_eq!(encoded.len(), 32 + 2 * 72);
    let decoded = decode_passive_mobs(&encoded).expect("decode passive mobs");
    let mut expected = save.records.clone();
    expected.sort_by_key(|record| record.id);
    assert_eq!(decoded.revision, 41);
    assert_eq!(decoded.records, expected);
    let reencoded = encode_passive_mobs(&PassiveMobsSave {
        revision: decoded.revision,
        records: decoded.records.clone(),
    })
    .expect("re-encode passive mobs");
    assert_eq!(reencoded, encoded, "canonical form must be byte-stable");
}

#[test]
fn passive_encode_is_canonical_regardless_of_input_order() {
    let forward = PassiveMobsSave {
        revision: 5,
        records: vec![passive_mob(1, 0.0, 0.0, 0.0), passive_mob(2, 1.0, 1.0, 1.0)],
    };
    let mut reversed = forward.clone();
    reversed.records.reverse();
    assert_eq!(
        encode_passive_mobs(&forward).expect("encode forward"),
        encode_passive_mobs(&reversed).expect("encode reversed"),
    );
}

#[test]
fn passive_maximum_records_fit_the_file_ceiling() {
    let records: Vec<PassiveMob> = (1..=MAX_PASSIVE_MOBS as u64)
        .map(|id| passive_mob(id, 0.5, 64.0, 100.0))
        .collect();
    let save = PassiveMobsSave {
        revision: 9,
        records,
    };
    let encoded = encode_passive_mobs(&save).expect("encode maximum passive mobs");
    assert_eq!(encoded.len(), PASSIVE_MAX_FILE_LENGTH);
    let decoded = decode_passive_mobs(&encoded).expect("decode maximum passive mobs");
    assert_eq!(decoded.records.len(), MAX_PASSIVE_MOBS);
}

#[test]
fn passive_decode_rejects_future_versions() {
    let encoded = encode_passive_mobs(&PassiveMobsSave {
        revision: 1,
        records: vec![passive_mob(1, 0.0, 0.0, 0.0)],
    })
    .expect("encode passive mobs");

    let mut future_schema = encoded.clone();
    future_schema[8..12].copy_from_slice(&(PASSIVE_CURRENT_SCHEMA + 1).to_le_bytes());
    assert_eq!(
        decode_passive_mobs(&future_schema).unwrap_err(),
        StorageError::FutureVersion("passive schema: 2".to_owned()),
    );

    let mut future_envelope = encoded.clone();
    future_envelope[4..8].copy_from_slice(&(PASSIVE_ENVELOPE_VERSION + 1).to_le_bytes());
    assert_eq!(
        decode_passive_mobs(&future_envelope).unwrap_err(),
        StorageError::FutureVersion("passive envelope version: 2".to_owned()),
    );
}

#[test]
fn passive_decode_rejects_corrupt_and_partial_records() {
    let encoded = encode_passive_mobs(&PassiveMobsSave {
        revision: 3,
        records: vec![passive_mob(1, 0.0, 0.0, 0.0), passive_mob(2, 1.0, 1.0, 1.0)],
    })
    .expect("encode passive mobs");

    for length in [0, 4, 31, 32, 40, encoded.len() - 1] {
        assert!(
            decode_passive_mobs(&encoded[..length]).is_err(),
            "truncated passive file of {length} bytes must be rejected"
        );
    }

    let mut oversized_count = encoded.clone();
    oversized_count[20..24].copy_from_slice(&(MAX_PASSIVE_MOBS as u32 + 1).to_le_bytes());
    assert!(decode_passive_mobs(&oversized_count).is_err());

    let mut oversized_file = encoded.clone();
    oversized_file.extend_from_slice(&[0u8; 8]);
    assert!(decode_passive_mobs(&oversized_file).is_err());

    let mut nonzero_reserved = encoded.clone();
    nonzero_reserved[32 + 42] = 1;
    reseal_passive(&mut nonzero_reserved);
    assert!(decode_passive_mobs(&nonzero_reserved).is_err());

    let mut broken_crc = encoded.clone();
    broken_crc[60] ^= 0xff;
    assert!(decode_passive_mobs(&broken_crc).is_err());

    let mut unsorted = encoded.clone();
    unsorted[32..40].copy_from_slice(&9u64.to_le_bytes());
    reseal_passive(&mut unsorted);
    assert!(decode_passive_mobs(&unsorted).is_err());

    let mut bad_bool = encoded.clone();
    bad_bool[32 + 36] = 2;
    reseal_passive(&mut bad_bool);
    assert!(decode_passive_mobs(&bad_bool).is_err());

    let mut bad_dimension = encoded.clone();
    bad_dimension[32 + 8..32 + 12].copy_from_slice(&1u32.to_le_bytes());
    reseal_passive(&mut bad_dimension);
    assert!(decode_passive_mobs(&bad_dimension).is_err());

    let mut bad_health = encoded.clone();
    bad_health[32 + 41] = 21;
    reseal_passive(&mut bad_health);
    assert!(decode_passive_mobs(&bad_health).is_err());
}

/// Recomputes the passive envelope checksum in place so a semantic mutation
/// is rejected by the record validator rather than the CRC gate.
fn reseal_passive(bytes: &mut [u8]) {
    let checksum = crc32c_join(&[&bytes[8..28], &bytes[32..]]);
    bytes[28..32].copy_from_slice(&checksum.to_le_bytes());
}

#[test]
fn passive_encode_rejects_invalid_saves() {
    assert!(
        encode_passive_mobs(&PassiveMobsSave {
            revision: 0,
            records: vec![passive_mob(1, 0.0, 0.0, 0.0)],
        })
        .is_err()
    );

    let too_many: Vec<PassiveMob> = (1..=MAX_PASSIVE_MOBS as u64 + 1)
        .map(|id| passive_mob(id, 0.0, 0.0, 0.0))
        .collect();
    assert!(
        encode_passive_mobs(&PassiveMobsSave {
            revision: 1,
            records: too_many,
        })
        .is_err()
    );

    let duplicate = PassiveMobsSave {
        revision: 1,
        records: vec![passive_mob(1, 0.0, 0.0, 0.0), passive_mob(1, 1.0, 1.0, 1.0)],
    };
    assert!(encode_passive_mobs(&duplicate).is_err());

    let mut outside_world = passive_mob(1, 0.0, 320.0, 0.0);
    assert!(
        encode_passive_mobs(&PassiveMobsSave {
            revision: 1,
            records: vec![outside_world.clone()],
        })
        .is_err()
    );
    outside_world.position[1] = -64.001;
    assert!(
        encode_passive_mobs(&PassiveMobsSave {
            revision: 1,
            records: vec![outside_world],
        })
        .is_err()
    );

    let mut zero_id = passive_mob(0, 0.0, 0.0, 0.0);
    assert!(
        encode_passive_mobs(&PassiveMobsSave {
            revision: 1,
            records: vec![zero_id.clone()],
        })
        .is_err()
    );
    zero_id.health = 0;
    zero_id.id = 1;
    assert!(
        encode_passive_mobs(&PassiveMobsSave {
            revision: 1,
            records: vec![zero_id],
        })
        .is_err()
    );
}

#[test]
fn passive_committed_go_fixture_round_trips_byte_for_byte() {
    let golden = read_go_fixture("server/storage/passive/testdata/passive-mobs-v1.bin");
    assert_eq!(golden.len(), 32 + 3 * 72);
    assert_eq!(&golden[0..4], b"PMST");
    let before = golden.clone();
    let decoded = decode_passive_mobs(&golden).expect("decode committed passive fixture");
    assert_eq!(golden, before, "decoding must not rewrite the input bytes");
    assert_eq!(decoded.revision, 11);
    assert_eq!(decoded.records.len(), 3);
    assert_eq!(decoded.records[0].id, 1);
    assert_eq!(decoded.records[0].position, [8.5, 65.5, 9.75]);
    assert_eq!(decoded.records[0].health, 1);
    let reencoded = encode_passive_mobs(&PassiveMobsSave {
        revision: decoded.revision,
        records: decoded.records.clone(),
    })
    .expect("re-encode committed passive fixture");
    assert_eq!(
        reencoded, golden,
        "the Rust encoder must reproduce the committed Go bytes exactly"
    );
}

fn passive_mob(id: u64, x: f32, y: f32, z: f32) -> PassiveMob {
    PassiveMob {
        id,
        dimension: 0,
        position: [x, y, z],
        velocity: [0.0, 0.0, 0.0],
        on_ground: true,
        yaw: 1.25,
        health: 20,
    }
}

fn read_go_fixture(relative: &str) -> Vec<u8> {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../../../packages/")
        .join(relative);
    fs::read(&path).unwrap_or_else(|err| panic!("read {}: {err}", path.display()))
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
