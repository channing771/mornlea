//! Foundation registration and dependency-direction contracts for `mornlea_storage`.
//! Save-family ports remain recorded in the change ledger until each family
//! has a failing behavioral case of its own.

use mornlea_storage::{
    BANK_A_START_SECTOR, BANK_B_START_SECTOR, BANK_SIZE, ChunkKey, DATA_START_SECTOR,
    HOSTILE_CURRENT_SCHEMA, HOSTILE_ENVELOPE_VERSION, HOSTILE_MAX_FILE_LENGTH, HOSTILE_SCHEMA_V1,
    HostileMob, HostileMobsSave, MAX_COMPRESSED_CHUNK, MAX_HOSTILE_MOBS, MAX_PASSIVE_MOBS,
    PASSIVE_CURRENT_SCHEMA, PASSIVE_ENVELOPE_VERSION, PASSIVE_MAX_FILE_LENGTH, PassiveMob,
    PassiveMobsSave, PlayerId, REGION_SLOTS, RegionBank, RegionEntry, RegionKey, SECTOR_SIZE,
    StorageError, crc32c, crc32c_join, decode_hostile_mobs, decode_passive_mobs,
    decode_region_bank, decode_superblock, encode_hostile_mobs, encode_passive_mobs,
    encode_region_bank, encode_superblock, region_for, select_region_bank,
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

#[test]
fn hostile_current_schema_round_trip_preserves_bytes() {
    let save = HostileMobsSave {
        revision: 31,
        records: vec![
            hostile_bone_thrower(0x8000_0000_0000_0002),
            hostile_nightcrawler(0x4000_0000_0000_0001),
            hostile_nightcrawler(1),
        ],
    };
    let encoded = encode_hostile_mobs(&save).expect("encode hostile mobs");
    assert_eq!(encoded.len(), 32 + 3 * 73);
    let decoded = decode_hostile_mobs(&encoded).expect("decode hostile mobs");
    let mut expected = save.records.clone();
    expected.sort_by_key(|record| record.id);
    assert_eq!(decoded.revision, 31);
    assert_eq!(decoded.records, expected);
    let reencoded = encode_hostile_mobs(&HostileMobsSave {
        revision: decoded.revision,
        records: decoded.records.clone(),
    })
    .expect("re-encode hostile mobs");
    assert_eq!(reencoded, encoded, "canonical form must be byte-stable");
    assert_eq!(HOSTILE_MAX_FILE_LENGTH, 4704);
}

#[test]
fn hostile_encode_is_canonical_regardless_of_input_order() {
    let forward = HostileMobsSave {
        revision: 5,
        records: vec![hostile_nightcrawler(1), hostile_nightcrawler(2)],
    };
    let mut reversed = forward.clone();
    reversed.records.reverse();
    assert_eq!(
        encode_hostile_mobs(&forward).expect("encode forward"),
        encode_hostile_mobs(&reversed).expect("encode reversed"),
    );
}

#[test]
fn hostile_empty_collection_round_trips() {
    let encoded = encode_hostile_mobs(&HostileMobsSave {
        revision: 4,
        records: Vec::new(),
    })
    .expect("encode empty hostile mobs");
    assert_eq!(encoded.len(), 32);
    let decoded = decode_hostile_mobs(&encoded).expect("decode empty hostile mobs");
    assert_eq!(decoded.revision, 4);
    assert!(decoded.records.is_empty());
}

#[test]
fn hostile_maximum_records_fit_the_file_ceiling() {
    let records: Vec<HostileMob> = (1..=MAX_HOSTILE_MOBS as u64)
        .map(hostile_nightcrawler)
        .collect();
    let encoded = encode_hostile_mobs(&HostileMobsSave {
        revision: 23,
        records,
    })
    .expect("encode maximum hostile mobs");
    assert_eq!(encoded.len(), HOSTILE_MAX_FILE_LENGTH);
    let decoded = decode_hostile_mobs(&encoded).expect("decode maximum hostile mobs");
    assert_eq!(decoded.records.len(), MAX_HOSTILE_MOBS);

    let mut trailing = encoded.clone();
    trailing.push(0);
    assert!(decode_hostile_mobs(&trailing).is_err());

    let too_many: Vec<HostileMob> = (1..=MAX_HOSTILE_MOBS as u64 + 1)
        .map(hostile_nightcrawler)
        .collect();
    assert!(
        encode_hostile_mobs(&HostileMobsSave {
            revision: 1,
            records: too_many,
        })
        .is_err()
    );

    let oversized = vec![0x5au8; HOSTILE_MAX_FILE_LENGTH + 1];
    assert!(decode_hostile_mobs(&oversized).is_err());
}

#[test]
fn hostile_v1_fixture_migrates_to_kind_zero_and_rewrites_as_v2() {
    let golden = read_go_fixture("server/storage/hostile/testdata/hostile-mobs-v1.bin");
    assert_eq!(golden.len(), 32 + 3 * 72);
    assert_eq!(
        u32::from_le_bytes(golden[8..12].try_into().unwrap()),
        HOSTILE_SCHEMA_V1,
        "the frozen v1 golden must stay on schema 1"
    );
    let before = golden.clone();
    let decoded = decode_hostile_mobs(&golden).expect("decode committed hostile v1 fixture");
    assert_eq!(
        golden, before,
        "v1 migration must not rewrite the input bytes"
    );
    assert_eq!(decoded.revision, 19);
    let expected = [
        hostile_far(1),
        hostile_idle(0x4000_0000_0000_0001),
        hostile_bone_thrower(0x8000_0000_0000_0002),
    ];
    for (record, want) in decoded.records.iter().zip(expected.iter()) {
        let mut migrated = want.clone();
        migrated.kind = 0;
        assert_eq!(
            record, &migrated,
            "v1 migration must keep every field but kind"
        );
    }

    let rewritten = encode_hostile_mobs(&HostileMobsSave {
        revision: decoded.revision,
        records: decoded.records.clone(),
    })
    .expect("rewrite migrated hostile mobs");
    assert_eq!(
        u32::from_le_bytes(rewritten[8..12].try_into().unwrap()),
        HOSTILE_CURRENT_SCHEMA,
    );
    assert_eq!(
        &rewritten[0..8],
        &golden[0..8],
        "envelope magic and version"
    );
    assert_eq!(&rewritten[12..24], &golden[12..24], "revision and count");
    assert_eq!(
        u32::from_le_bytes(rewritten[24..28].try_into().unwrap()) as usize,
        (golden.len() - 32) + expected.len(),
    );
    for index in 0..expected.len() {
        let v1_record = &golden[32 + index * 72..32 + (index + 1) * 72];
        let v2_record = &rewritten[32 + index * 73..32 + (index + 1) * 73];
        assert_eq!(
            &v2_record[..72],
            v1_record,
            "v1 record {index} must be preserved byte-for-byte as the v2 prefix"
        );
        assert_eq!(v2_record[72], 0, "migrated kind must stay zero");
    }
}

#[test]
fn hostile_v2_fixture_round_trips_byte_for_byte() {
    let golden = read_go_fixture("server/storage/hostile/testdata/hostile-mobs-v2.bin");
    assert_eq!(golden.len(), 32 + 3 * 73);
    assert_eq!(
        u32::from_le_bytes(golden[8..12].try_into().unwrap()),
        HOSTILE_CURRENT_SCHEMA,
    );
    let decoded = decode_hostile_mobs(&golden).expect("decode committed hostile v2 fixture");
    let reencoded = encode_hostile_mobs(&HostileMobsSave {
        revision: decoded.revision,
        records: decoded.records.clone(),
    })
    .expect("re-encode committed hostile v2 fixture");
    assert_eq!(
        reencoded, golden,
        "the Rust encoder must reproduce the committed Go bytes exactly"
    );
}

#[test]
fn hostile_decode_rejects_future_versions() {
    let encoded = encode_hostile_mobs(&HostileMobsSave {
        revision: 1,
        records: vec![hostile_nightcrawler(1)],
    })
    .expect("encode hostile mobs");

    let mut future_schema = encoded.clone();
    future_schema[8..12].copy_from_slice(&(HOSTILE_CURRENT_SCHEMA + 1).to_le_bytes());
    assert_eq!(
        decode_hostile_mobs(&future_schema).unwrap_err(),
        StorageError::FutureVersion("hostile schema: 3".to_owned()),
    );

    let mut future_envelope = encoded.clone();
    future_envelope[4..8].copy_from_slice(&(HOSTILE_ENVELOPE_VERSION + 1).to_le_bytes());
    assert_eq!(
        decode_hostile_mobs(&future_envelope).unwrap_err(),
        StorageError::FutureVersion("hostile envelope version: 2".to_owned()),
    );
}

#[test]
fn hostile_decode_rejects_corrupt_and_partial_records() {
    let encoded = encode_hostile_mobs(&HostileMobsSave {
        revision: 3,
        records: vec![hostile_nightcrawler(1), hostile_bone_thrower(2)],
    })
    .expect("encode hostile mobs");

    for length in [0, 4, 31, 32, 40, 33, encoded.len() - 1] {
        assert!(
            decode_hostile_mobs(&encoded[..length]).is_err(),
            "truncated hostile file of {length} bytes must be rejected"
        );
    }

    let mut oversized_count = encoded.clone();
    oversized_count[20..24].copy_from_slice(&(MAX_HOSTILE_MOBS as u32 + 1).to_le_bytes());
    assert!(decode_hostile_mobs(&oversized_count).is_err());

    let mut bad_crc = encoded.clone();
    bad_crc[60] ^= 0xff;
    assert!(decode_hostile_mobs(&bad_crc).is_err());

    let mut unsorted = encoded.clone();
    unsorted[32..40].copy_from_slice(&9u64.to_le_bytes());
    reseal_hostile(&mut unsorted);
    assert!(decode_hostile_mobs(&unsorted).is_err());

    let mut bad_cooldown = encoded.clone();
    bad_cooldown[32 + 42] = 21;
    reseal_hostile(&mut bad_cooldown);
    assert!(decode_hostile_mobs(&bad_cooldown).is_err());

    let mut bad_distant = encoded.clone();
    bad_distant[32 + 70..32 + 72].copy_from_slice(&601u16.to_le_bytes());
    reseal_hostile(&mut bad_distant);
    assert!(decode_hostile_mobs(&bad_distant).is_err());

    let mut bad_kind = encoded.clone();
    bad_kind[32 + 72] = 2;
    reseal_hostile(&mut bad_kind);
    assert!(decode_hostile_mobs(&bad_kind).is_err());

    let mut bad_bool = encoded.clone();
    bad_bool[32 + 36] = 2;
    reseal_hostile(&mut bad_bool);
    assert!(decode_hostile_mobs(&bad_bool).is_err());

    let mut bad_target = encoded.clone();
    bad_target[32 + 73 + 45] = 0;
    reseal_hostile(&mut bad_target);
    assert!(decode_hostile_mobs(&bad_target).is_err());
}

#[test]
fn hostile_encode_rejects_invalid_saves() {
    assert!(
        encode_hostile_mobs(&HostileMobsSave {
            revision: 0,
            records: vec![hostile_nightcrawler(1)],
        })
        .is_err()
    );

    let duplicate = HostileMobsSave {
        revision: 1,
        records: vec![hostile_nightcrawler(1), hostile_nightcrawler(1)],
    };
    assert!(encode_hostile_mobs(&duplicate).is_err());

    let mut zero_id = hostile_nightcrawler(0);
    assert!(
        encode_hostile_mobs(&HostileMobsSave {
            revision: 1,
            records: vec![zero_id.clone()],
        })
        .is_err()
    );
    zero_id.id = 1;
    zero_id.health = 0;
    assert!(
        encode_hostile_mobs(&HostileMobsSave {
            revision: 1,
            records: vec![zero_id],
        })
        .is_err()
    );

    let mut stale_target = hostile_nightcrawler(1);
    stale_target.player_id =
        PlayerId::from_bytes([0x6f, 0xce, 0x82, 0x77, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0]);
    assert!(
        encode_hostile_mobs(&HostileMobsSave {
            revision: 1,
            records: vec![stale_target],
        })
        .is_err()
    );
}

/// Recomputes the hostile envelope checksum in place so a semantic mutation is
/// rejected by the record validator rather than the CRC gate.
fn reseal_hostile(bytes: &mut [u8]) {
    let checksum = crc32c_join(&[&bytes[8..28], &bytes[32..]]);
    bytes[28..32].copy_from_slice(&checksum.to_le_bytes());
}

fn hostile_nightcrawler(id: u64) -> HostileMob {
    HostileMob {
        id,
        dimension: 0,
        position: [0.0, 0.0, 0.0],
        velocity: [0.0, 0.0, 0.0],
        on_ground: false,
        yaw: 0.0,
        health: 1,
        attack_cooldown: 0,
        hurt_cooldown: 0,
        burn_cooldown: 0,
        has_target: false,
        player_id: PlayerId::from_bytes([0; 16]),
        next_repath_ticks: 0,
        distant_ticks: 0,
        kind: 0,
    }
}

/// The committed fixture's "idle" nightcrawler.
fn hostile_idle(id: u64) -> HostileMob {
    HostileMob {
        position: [0.5, 64.0, -9.75],
        velocity: [0.0, -3.25, 0.0],
        yaw: -2.5,
        health: 20,
        ..hostile_nightcrawler(id)
    }
}

/// The committed fixture's "far" nightcrawler.
fn hostile_far(id: u64) -> HostileMob {
    HostileMob {
        position: [8.5, 65.5, 9.75],
        velocity: [2.0, 0.0, -2.0],
        on_ground: true,
        yaw: 3.0,
        health: 1,
        burn_cooldown: 19,
        distant_ticks: 600,
        ..hostile_nightcrawler(id)
    }
}

fn hostile_bone_thrower(id: u64) -> HostileMob {
    HostileMob {
        id,
        dimension: 0,
        position: [-12.5, 70.25, 3.5],
        velocity: [-1.25, 0.0, 0.5],
        on_ground: true,
        yaw: 1.25,
        health: 17,
        attack_cooldown: 3,
        hurt_cooldown: 1,
        burn_cooldown: 5,
        has_target: true,
        player_id: PlayerId::from_bytes([
            0x6f, 0xce, 0x82, 0x77, 0xa9, 0x33, 0x46, 0xcb, 0x9a, 0x1f, 0xda, 0x13, 0xb7, 0xee,
            0x56, 0x44,
        ]),
        next_repath_ticks: 905,
        distant_ticks: 120,
        kind: 1,
    }
}

#[test]
fn region_superblock_exact_layout_and_round_trip() {
    let key = RegionKey {
        dimension: -7,
        x: -2,
        z: 3,
    };
    let encoded = encode_superblock(key);
    assert_eq!(&encoded[0..4], b"MCGR");
    let fields: [(usize, u32); 9] = [
        (4, 1),
        (8, 4096),
        (12, 0xffff_fff9),
        (16, 0xffff_fffe),
        (20, 3),
        (24, 1),
        (28, 8),
        (32, 7),
        (36, 15),
    ];
    for (offset, want) in fields {
        assert_eq!(
            u32_at(&encoded, offset),
            want,
            "superblock field at {offset}"
        );
    }
    assert!(encoded[40..4092].iter().all(|byte| *byte == 0));
    assert_eq!(u32_at(&encoded, 4092), crc32c(&encoded[..4092]));
    // Frozen layout constants the format contract depends on.
    assert_eq!(BANK_A_START_SECTOR, 1);
    assert_eq!(BANK_B_START_SECTOR, 8);
    assert_eq!(BANK_SIZE, 7 * SECTOR_SIZE as usize);
    assert_eq!(DATA_START_SECTOR, 15);
    assert_eq!(REGION_SLOTS, 1024);
    assert_eq!(MAX_COMPRESSED_CHUNK, 1 << 20);
    decode_superblock(key, &encoded).expect("decode superblock");
}

#[test]
fn region_superblock_rejects_corruption() {
    let key = RegionKey {
        dimension: 0,
        x: -2,
        z: 3,
    };
    let valid = encode_superblock(key);

    let mut trailing = valid.to_vec();
    trailing.push(0);
    let cases: Vec<(&str, Vec<u8>, RegionKey)> = vec![
        (
            "wrong magic",
            mutate_superblock(&valid, 0, 0xff, false),
            key,
        ),
        ("past version", put_super_u32(&valid, 4, 0), key),
        ("future version", put_super_u32(&valid, 4, 2), key),
        ("wrong sector size", put_super_u32(&valid, 8, 2048), key),
        (
            "wrong dimension",
            valid.to_vec(),
            RegionKey {
                dimension: 1,
                x: -2,
                z: 3,
            },
        ),
        (
            "wrong x",
            valid.to_vec(),
            RegionKey {
                dimension: 0,
                x: -1,
                z: 3,
            },
        ),
        (
            "wrong z",
            valid.to_vec(),
            RegionKey {
                dimension: 0,
                x: -2,
                z: 4,
            },
        ),
        ("wrong bank A sector", put_super_u32(&valid, 24, 2), key),
        ("wrong bank B sector", put_super_u32(&valid, 28, 9), key),
        ("wrong bank size", put_super_u32(&valid, 32, 6), key),
        ("wrong data start", put_super_u32(&valid, 36, 14), key),
        (
            "nonzero reserved byte",
            mutate_superblock(&valid, 40, 1, true),
            key,
        ),
        ("invalid CRC", mutate_superblock(&valid, 100, 1, false), key),
        ("short", valid[..4095].to_vec(), key),
        ("trailing", trailing, key),
    ];
    for (name, bytes, key) in cases {
        let err = decode_superblock(key, &bytes)
            .err()
            .unwrap_or_else(|| panic!("{name}: expected rejection"));
        assert_eq!(
            matches!(err, StorageError::FutureVersion(_)),
            name == "future version",
            "{name}: unexpected error class {err:?}"
        );
    }
}

#[test]
fn region_bank_round_trip_and_selection() {
    let key = region_bank_key();
    let mut want = RegionBank::empty();
    want.generation = 9;
    want.entries[31] = RegionEntry {
        offset_sector: 15,
        sector_count: 2,
        payload_length: 5000,
        revision: 7,
        payload_crc32c: 0x1234_5678,
    };
    let encoded = encode_region_bank(key, &want).expect("encode region bank");
    let got =
        decode_region_bank(key, &encoded, 17 * SECTOR_SIZE as i64).expect("decode region bank");
    assert_eq!(got, want);

    let mut older = RegionBank::empty();
    older.generation = 8;
    let (selected, index) = select_region_bank(Ok(older), Ok(got)).expect("select region bank");
    assert_eq!(index, 1);
    assert_eq!(selected.generation, 9);
}

#[test]
fn region_bank_exact_layout() {
    let key = RegionKey {
        dimension: -7,
        x: -2,
        z: 3,
    };
    let mut bank = RegionBank::empty();
    bank.generation = 9;
    bank.entries[31] = RegionEntry {
        offset_sector: 15,
        sector_count: 2,
        payload_length: 5000,
        revision: 7,
        payload_crc32c: 0x1234_5678,
    };
    let encoded = encode_region_bank(key, &bank).expect("encode region bank");
    assert_eq!(&encoded[0..4], b"MCGB");
    let fields: [(usize, u32); 9] = [
        (4, 1),
        (8, 4096),
        (12, 0xffff_fff9),
        (16, 0xffff_fffe),
        (20, 3),
        (32, 1024),
        (36, 24),
        (40, 7),
        (44, 15),
    ];
    for (offset, want) in fields {
        assert_eq!(u32_at(&encoded, offset), want, "bank field at {offset}");
    }
    assert_eq!(u64_at(&encoded, 24), 9);
    assert!(encoded[48..60].iter().all(|byte| *byte == 0));

    let entry = 64 + 31 * 24;
    assert_eq!(u32_at(&encoded, entry), 15);
    assert_eq!(u32_at(&encoded, entry + 4), 2);
    assert_eq!(u32_at(&encoded, entry + 8), 5000);
    assert_eq!(u64_at(&encoded, entry + 12), 7);
    assert_eq!(u32_at(&encoded, entry + 20), 0x1234_5678);
    assert!(encoded[64 + 1024 * 24..].iter().all(|byte| *byte == 0));

    let mut checksum_input = encoded.to_vec();
    checksum_input[60..64].copy_from_slice(&[0, 0, 0, 0]);
    assert_eq!(u32_at(&encoded, 60), crc32c(&checksum_input));
}

#[test]
fn region_bank_accepts_empty_generation_zero_and_max_generation() {
    let key = region_bank_key();
    for generation in [0u64, u64::MAX] {
        let mut bank = RegionBank::empty();
        bank.generation = generation;
        let encoded = encode_region_bank(key, &bank).expect("encode bank");
        let got = decode_region_bank(key, &encoded, DATA_START_SECTOR as i64 * SECTOR_SIZE as i64)
            .expect("decode bank");
        assert_eq!(got, bank, "generation {generation}");
    }
}

#[test]
fn region_bank_rejects_corruption() {
    let key = region_bank_key();
    let file_size = 17 * SECTOR_SIZE as i64;
    let mut base = RegionBank::empty();
    base.generation = 9;
    base.entries[0] = RegionEntry {
        offset_sector: 15,
        sector_count: 2,
        payload_length: 5000,
        revision: 7,
        payload_crc32c: 0x1234_5678,
    };
    let valid = encode_region_bank(key, &base).expect("encode region bank");

    let mut trailing = valid.to_vec();
    trailing.push(0);
    let cases: Vec<(&str, Vec<u8>, i64)> = vec![
        ("wrong magic", mutate_bank(&valid, 0, 1, false), file_size),
        ("past version", put_bank_u32(&valid, 4, 0), file_size),
        ("future version", put_bank_u32(&valid, 4, 2), file_size),
        (
            "wrong sector size",
            put_bank_u32(&valid, 8, 2048),
            file_size,
        ),
        (
            "wrong entry count",
            put_bank_u32(&valid, 32, 1023),
            file_size,
        ),
        ("wrong entry size", put_bank_u32(&valid, 36, 20), file_size),
        ("wrong bank sectors", put_bank_u32(&valid, 40, 6), file_size),
        ("wrong data start", put_bank_u32(&valid, 44, 14), file_size),
        (
            "nonzero header reserved byte",
            mutate_bank(&valid, 48, 1, true),
            file_size,
        ),
        ("invalid CRC", mutate_bank(&valid, 100, 1, false), file_size),
        (
            "generation zero with entry",
            put_bank_u64(&valid, 24, 0),
            file_size,
        ),
        (
            "offset inside headers",
            put_bank_entry_u32(&valid, 0, 0, 14),
            file_size,
        ),
        (
            "zero sector count",
            put_bank_entry_u32(&valid, 0, 4, 0),
            file_size,
        ),
        (
            "payload over one MiB",
            put_bank_entry_u32(&valid, 0, 8, (1 << 20) + 1),
            272 * SECTOR_SIZE as i64,
        ),
        (
            "payload exceeds extent",
            put_bank_entry_u32(&valid, 0, 4, 1),
            file_size,
        ),
        (
            "extent past EOF",
            put_bank_entry_u32(&valid, 0, 0, 16),
            file_size,
        ),
        (
            "uint32 extent overflow",
            put_bank_entry_u32(&valid, 0, 0, u32::MAX),
            i64::MAX,
        ),
        (
            "absent entry with nonzero tail",
            put_bank_entry_u32(&valid, 1, 0, 1),
            file_size,
        ),
        (
            "zero revision",
            put_bank_entry_u64(&valid, 0, 12, 0),
            file_size,
        ),
        (
            "nonzero trailing padding",
            mutate_bank(&valid, 64 + 1024 * 24, 1, true),
            file_size,
        ),
        ("short", valid[..valid.len() - 1].to_vec(), file_size),
        ("trailing", trailing, file_size),
    ];
    for (name, bytes, size) in cases {
        let err = decode_region_bank(key, &bytes, size)
            .err()
            .unwrap_or_else(|| panic!("{name}: expected rejection"));
        assert_eq!(
            matches!(err, StorageError::FutureVersion(_)),
            name == "future version",
            "{name}: unexpected error class {err:?}"
        );
    }

    // Overlapping extents need a second populated slot.
    let mut overlap = put_bank_entry_u32(&valid, 1, 0, 16);
    overlap = put_bank_entry_u32(&overlap, 1, 4, 1);
    overlap = put_bank_entry_u32(&overlap, 1, 8, 1);
    overlap = put_bank_entry_u64(&overlap, 1, 12, 8);
    assert!(decode_region_bank(key, &overlap, file_size).is_err());
}

#[test]
fn region_encode_bank_rejects_invalid_structure() {
    let key = region_bank_key();
    let empty = RegionBank::empty();
    let valid_entry = RegionEntry {
        offset_sector: 15,
        sector_count: 1,
        payload_length: 1,
        revision: 1,
        payload_crc32c: 0,
    };

    let mut generation_zero_with_entry = empty.clone();
    generation_zero_with_entry.entries[0] = valid_entry;
    assert!(encode_region_bank(key, &generation_zero_with_entry).is_err());

    let mut absent_with_tail = empty.clone();
    absent_with_tail.generation = 1;
    absent_with_tail.entries[0] = RegionEntry {
        sector_count: 1,
        ..RegionEntry::default()
    };
    assert!(encode_region_bank(key, &absent_with_tail).is_err());

    let mut zero_sector_count = empty.clone();
    zero_sector_count.generation = 1;
    zero_sector_count.entries[0] = RegionEntry {
        offset_sector: 15,
        sector_count: 0,
        payload_length: 1,
        revision: 1,
        payload_crc32c: 0,
    };
    assert!(encode_region_bank(key, &zero_sector_count).is_err());

    let mut oversized = empty.clone();
    oversized.generation = 1;
    oversized.entries[0] = RegionEntry {
        offset_sector: 15,
        sector_count: 257,
        payload_length: (1 << 20) + 1,
        revision: 1,
        payload_crc32c: 0,
    };
    assert!(encode_region_bank(key, &oversized).is_err());

    let mut zero_revision = empty.clone();
    zero_revision.generation = 1;
    zero_revision.entries[0] = RegionEntry {
        offset_sector: 15,
        sector_count: 1,
        payload_length: 1,
        revision: 0,
        payload_crc32c: 0,
    };
    assert!(encode_region_bank(key, &zero_revision).is_err());

    let mut overlapping = empty.clone();
    overlapping.generation = 1;
    overlapping.entries[0] = RegionEntry {
        offset_sector: 15,
        sector_count: 2,
        payload_length: 1,
        revision: 1,
        payload_crc32c: 0,
    };
    overlapping.entries[1] = RegionEntry {
        offset_sector: 16,
        sector_count: 1,
        payload_length: 1,
        revision: 2,
        payload_crc32c: 0,
    };
    assert!(encode_region_bank(key, &overlapping).is_err());

    let mut committed = empty.clone();
    committed.generation = 9;
    assert!(encode_region_bank(key, &committed).is_ok());
}

#[test]
fn region_select_bank_validity_and_ties() {
    let mut committed = RegionBank::empty();
    committed.generation = 9;
    let mut newer = RegionBank::empty();
    newer.generation = 10;
    let mut different = RegionBank::empty();
    different.generation = 9;
    different.entries[0] = RegionEntry {
        offset_sector: 15,
        sector_count: 1,
        payload_length: 1,
        revision: 1,
        payload_crc32c: 0,
    };
    let invalid = || Err(StorageError::Corrupt("region bank: injected".to_owned()));

    assert!(select_region_bank(invalid(), invalid()).is_err());
    let (bank, index) = select_region_bank(Ok(committed.clone()), invalid()).expect("only A valid");
    assert_eq!(index, 0);
    assert_eq!(bank, committed);
    let (bank, index) = select_region_bank(invalid(), Ok(committed.clone())).expect("only B valid");
    assert_eq!(index, 1);
    assert_eq!(bank, committed);
    let (bank, index) =
        select_region_bank(Ok(newer.clone()), Ok(committed.clone())).expect("newer A");
    assert_eq!(index, 0);
    assert_eq!(bank, newer);
    let (bank, index) =
        select_region_bank(Ok(committed.clone()), Ok(newer.clone())).expect("newer B");
    assert_eq!(index, 1);
    assert_eq!(bank, newer);
    let (bank, index) =
        select_region_bank(Ok(committed.clone()), Ok(committed.clone())).expect("identical tie");
    assert_eq!(index, 0);
    assert_eq!(bank, committed);
    assert!(select_region_bank(Ok(committed.clone()), Ok(different)).is_err());

    // A structurally valid generation-zero bank is an uncommitted standby: it
    // loses against a committed peer but is an error when the peer also fails.
    let standby = RegionBank::empty();
    assert!(select_region_bank(Ok(standby.clone()), invalid()).is_err());
    assert!(select_region_bank(Ok(standby.clone()), Ok(standby.clone())).is_err());
    let (bank, index) =
        select_region_bank(Ok(standby), Ok(committed.clone())).expect("standby A and committed B");
    assert_eq!(index, 1);
    assert_eq!(bank, committed);
}

#[test]
fn region_for_uses_floor_division() {
    let cases: [(i32, i32, usize); 9] = [
        (-33, -2, 31 * 32 + 31),
        (-32, -1, 0),
        (-31, -1, 32 + 1),
        (-1, -1, 31 * 32 + 31),
        (0, 0, 0),
        (1, 0, 32 + 1),
        (31, 0, 31 * 32 + 31),
        (32, 1, 0),
        (33, 1, 32 + 1),
    ];
    for (chunk, region, slot) in cases {
        let (key, got_slot) = region_for(ChunkKey {
            dimension: 0,
            x: chunk,
            z: chunk,
        });
        assert_eq!((key.x, key.z), (region, region), "chunk {chunk}");
        assert_eq!(got_slot, slot, "chunk {chunk} slot");
    }
}

#[test]
fn region_for_handles_min_int32() {
    let (key, slot) = region_for(ChunkKey {
        dimension: 0,
        x: i32::MIN,
        z: i32::MIN,
    });
    assert_eq!((key.x, key.z), (-67_108_864, -67_108_864));
    assert_eq!(slot, 0);
}

fn region_bank_key() -> RegionKey {
    RegionKey {
        dimension: 0,
        x: -2,
        z: 3,
    }
}

fn mutate_superblock(block: &[u8; 4096], offset: usize, mask: u8, reseal: bool) -> Vec<u8> {
    let mut mutated = block.to_vec();
    mutated[offset] ^= mask;
    if reseal {
        reseal_superblock(&mut mutated);
    }
    mutated
}

fn put_super_u32(block: &[u8; 4096], offset: usize, value: u32) -> Vec<u8> {
    let mut mutated = block.to_vec();
    mutated[offset..offset + 4].copy_from_slice(&value.to_le_bytes());
    reseal_superblock(&mut mutated);
    mutated
}

fn reseal_superblock(block: &mut [u8]) {
    let checksum = crc32c(&block[..4092]);
    block[4092..4096].copy_from_slice(&checksum.to_le_bytes());
}

fn mutate_bank(bank: &[u8], offset: usize, mask: u8, reseal: bool) -> Vec<u8> {
    let mut mutated = bank.to_vec();
    mutated[offset] ^= mask;
    if reseal {
        reseal_bank(&mut mutated);
    }
    mutated
}

fn put_bank_u32(bank: &[u8], offset: usize, value: u32) -> Vec<u8> {
    let mut mutated = bank.to_vec();
    mutated[offset..offset + 4].copy_from_slice(&value.to_le_bytes());
    reseal_bank(&mut mutated);
    mutated
}

fn put_bank_u64(bank: &[u8], offset: usize, value: u64) -> Vec<u8> {
    let mut mutated = bank.to_vec();
    mutated[offset..offset + 8].copy_from_slice(&value.to_le_bytes());
    reseal_bank(&mut mutated);
    mutated
}

fn put_bank_entry_u32(bank: &[u8], slot: usize, field: usize, value: u32) -> Vec<u8> {
    put_bank_u32(bank, 64 + slot * 24 + field, value)
}

fn put_bank_entry_u64(bank: &[u8], slot: usize, field: usize, value: u64) -> Vec<u8> {
    put_bank_u64(bank, 64 + slot * 24 + field, value)
}

fn reseal_bank(bank: &mut [u8]) {
    bank[60..64].copy_from_slice(&[0, 0, 0, 0]);
    let checksum = crc32c(bank);
    bank[60..64].copy_from_slice(&checksum.to_le_bytes());
}

fn u32_at(encoded: &[u8], offset: usize) -> u32 {
    u32::from_le_bytes(encoded[offset..offset + 4].try_into().expect("four bytes"))
}

fn u64_at(encoded: &[u8], offset: usize) -> u64 {
    u64::from_le_bytes(encoded[offset..offset + 8].try_into().expect("eight bytes"))
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
