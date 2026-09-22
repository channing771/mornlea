//! Foundation registration, identity, value-range, and semantic input/event
//! contracts for `mornlea_domain`.

use std::fs;
use std::path::PathBuf;

const FORBIDDEN_PRODUCTION_DEPS: &[&str] = &[
    "mornlea_protocol",
    "mornlea_storage",
    "mornlea_engine",
    "mornlea_client",
    "mornlea_godot",
];

#[test]
fn crate_identity_matches_workspace_name() {
    assert_eq!(mornlea_domain::CRATE_NAME, env!("CARGO_PKG_NAME"));
}

#[test]
fn production_manifest_has_no_codec_kernel_or_host_dependencies() {
    let keys = production_dependency_keys(&read_manifest(env!("CARGO_MANIFEST_DIR")));
    assert!(
        keys.is_empty(),
        "mornlea_domain production dependencies must stay empty, got {keys:?}"
    );
    for forbidden in FORBIDDEN_PRODUCTION_DEPS {
        assert!(
            !keys.iter().any(|key| key == forbidden),
            "mornlea_domain must not depend on {forbidden}"
        );
    }
}

#[test]
fn inventory_assigns_domain_families_to_this_crate() {
    let json = read_inventory();
    let owner = format!("\"eventual_owner\": \"{}\"", env!("CARGO_PKG_NAME"));
    let count = json.matches(&owner).count();
    // The frozen corpus grows one domain family per landed node, so the pinned
    // set is the reconciliation point the assertion message names: a family the
    // list does not name still fails, which is what keeps an unmerged row from
    // passing silently.
    let families = [
        "domain.event",
        "domain.identity_values",
        "domain.command_control",
        "domain.command_inventory",
        "domain.values",
        "domain.input",
    ];
    assert_eq!(
        count,
        families.len(),
        "domain inventory rows drifted; update intended domain ports before implementing them"
    );
    for family in families {
        assert!(
            json.contains(&format!("\"id\": \"{family}\"")),
            "missing domain family {family}"
        );
    }
}

#[test]
fn current_identities_match_frozen_inventory() {
    let json = read_inventory();
    let live = mornlea_domain::Identities::current();
    live.validate()
        .expect("current identities must be complete");
    assert!(json.contains(&format!("\"protocol\": {}", live.protocol)));
    assert!(json.contains(&format!("\"chunk_schema\": {}", live.chunk_schema)));
    assert!(json.contains(&format!("\"player_schema\": {}", live.player_schema)));
    assert!(json.contains(&format!("\"world_metadata\": {}", live.world_metadata)));
    assert!(json.contains(&format!(
        "\"companions_ai_schema\": {}",
        live.companions_ai_schema
    )));
    assert!(json.contains(&format!(
        "\"hostile_mobs_schema\": {}",
        live.hostile_mobs_schema
    )));
    assert!(json.contains(&format!(
        "\"passive_mobs_schema\": {}",
        live.passive_mobs_schema
    )));
    assert!(json.contains(&format!("\"engine_abi\": {}", live.engine_abi)));
    assert!(json.contains(&format!("\"region_format\": {}", live.region_format)));
    assert!(json.contains(&format!("\"agent_http\": \"{}\"", live.agent_http)));
    assert!(json.contains(&format!("\"agent_mcp\": \"{}\"", live.agent_mcp)));
}

#[test]
fn incomplete_identity_is_rejected() {
    let mut identities = mornlea_domain::Identities::current();
    identities.protocol = 0;
    identities.agent_http.clear();
    let err = identities
        .validate()
        .expect_err("incomplete identity must fail");
    assert_eq!(err, mornlea_domain::DomainError::IncompleteIdentity);

    let err = mornlea_domain::ReplayIdentity::new("", "sha256:abc", identities.clone(), 1)
        .expect_err("empty source revision must fail");
    assert_eq!(err, mornlea_domain::DomainError::IncompleteIdentity);
}

#[test]
fn invalid_dimension_and_hotbar_ranges_are_rejected() {
    assert_eq!(
        mornlea_domain::Dimension::new(2),
        Err(mornlea_domain::DomainError::InvalidDimension)
    );
    assert_eq!(
        mornlea_domain::HotbarSlot::new(mornlea_domain::HotbarSlot::COUNT),
        Err(mornlea_domain::DomainError::InvalidHotbarSlot)
    );
    assert!(mornlea_domain::Dimension::new(0).is_ok());
    assert!(mornlea_domain::Dimension::new(1).is_ok());
    assert!(mornlea_domain::HotbarSlot::new(0).is_ok());
    assert!(mornlea_domain::HotbarSlot::new(8).is_ok());
}

#[test]
fn non_finite_input_rotation_is_rejected() {
    let err = mornlea_domain::LookAngles::try_new(f32::NAN, 0.0).expect_err("NaN yaw must fail");
    assert_eq!(err, mornlea_domain::DomainError::NonFiniteRotation);

    let err = mornlea_domain::LookAngles::try_new(0.0, f32::INFINITY)
        .expect_err("infinite pitch must fail");
    assert_eq!(err, mornlea_domain::DomainError::NonFiniteRotation);

    // The grouped payload cannot be built around a non-finite rotation,
    // because its parts carry an already-validated `LookAngles`.
    let err = mornlea_domain::LookAngles::try_new(0.0, f32::NAN)
        .expect_err("NaN pitch must fail before any payload exists");
    assert_eq!(err, mornlea_domain::DomainError::NonFiniteRotation);
}

#[test]
fn observations_order_by_tick_then_family() {
    let late = mornlea_domain::Observation::new(2, "domain.input", "sha256:a").unwrap();
    let early_event = mornlea_domain::Observation::new(1, "domain.event", "sha256:b").unwrap();
    let early_input = mornlea_domain::Observation::new(1, "domain.input", "sha256:c").unwrap();
    let ordered = mornlea_domain::order_observations([
        late.clone(),
        early_input.clone(),
        early_event.clone(),
    ]);
    assert_eq!(
        ordered
            .iter()
            .map(|row| (row.tick, row.family_id()))
            .collect::<Vec<_>>(),
        vec![
            (1, "domain.event"),
            (1, "domain.input"),
            (2, "domain.input")
        ]
    );
}

#[test]
fn unknown_observation_family_is_rejected() {
    let err = mornlea_domain::Observation::new(1, "protocol.frame", "sha256:a")
        .expect_err("unknown family must fail");
    assert_eq!(err, mornlea_domain::DomainError::UnknownId);
    assert!(mornlea_domain::Observation::new(1, "domain.event", "sha256:b").is_ok());
    assert!(mornlea_domain::Observation::new(1, "domain.input", "sha256:c").is_ok());
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
