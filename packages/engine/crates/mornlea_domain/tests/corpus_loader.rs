//! Tests for the strict fail-closed Rust corpus loader.

use std::fs;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicU64, Ordering};

#[path = "../../../tests/runtime_corpus.rs"]
mod runtime_corpus;

use runtime_corpus::{CorpusConsumer, InputFormat, try_load_cases_from_root};

static CORPUS_COUNTER: AtomicU64 = AtomicU64::new(0);

struct TempCorpusDir {
    path: PathBuf,
}

impl TempCorpusDir {
    fn new() -> Self {
        let pid = std::process::id();
        let count = CORPUS_COUNTER.fetch_add(1, Ordering::Relaxed);
        let dir_name = format!("mornlea-corpus-loader-{pid}-{count}");
        let path = std::env::temp_dir().join(dir_name);
        fs::create_dir_all(&path).expect("create temp corpus dir");
        Self { path }
    }

    fn path(&self) -> &Path {
        &self.path
    }
}

impl Drop for TempCorpusDir {
    fn drop(&mut self) {
        if let Ok(canonical) = self.path.canonicalize() {
            let is_match = canonical
                .file_name()
                .and_then(|n| n.to_str())
                .is_some_and(|name| name.starts_with("mornlea-corpus-loader-"));
            if is_match {
                let _ = fs::remove_dir_all(&canonical);
            }
        }
    }
}

fn sha256_digest(bytes: &[u8]) -> String {
    use sha2::{Digest, Sha256};
    let mut hasher = Sha256::new();
    hasher.update(bytes);
    format!("sha256:{:x}", hasher.finalize())
}

struct MinimalCorpusConfig<'a> {
    family_id: &'a str,
    family_cases: Vec<&'a str>,
    supported_versions: Vec<&'a str>,
    case_id: &'a str,
    case_family: &'a str,
    case_version: &'a str,
    case_operation: &'a str,
    input_format: &'a str,
    input_bytes: Vec<u8>,
    expected_bytes: Vec<u8>,
    checkpoints: Vec<&'a str>,
    rust_consumer: &'a str,
}

impl Default for MinimalCorpusConfig<'static> {
    fn default() -> Self {
        Self {
            family_id: "domain.test",
            family_cases: vec!["domain.test/v1/case-1"],
            supported_versions: vec!["v1"],
            case_id: "domain.test/v1/case-1",
            case_family: "domain.test",
            case_version: "v1",
            case_operation: "decode",
            input_format: "json",
            input_bytes: br#"{"valid":true}"#.to_vec(),
            expected_bytes: br#"{"category":"test","kind":"ok"}"#.to_vec(),
            checkpoints: vec!["0"],
            rust_consumer: "mornlea_domain",
        }
    }
}

fn write_corpus(temp: &TempCorpusDir, config: &MinimalCorpusConfig) {
    let base = temp.path();
    let cases_dir = base.join("testdata/runtime-migration/cases/test");
    fs::create_dir_all(&cases_dir).expect("create cases dir");

    let input_rel = "testdata/runtime-migration/cases/test/case-1.input.json";
    let expected_rel = "testdata/runtime-migration/cases/test/case-1.expected.json";

    fs::write(base.join(input_rel), &config.input_bytes).expect("write input");
    fs::write(base.join(expected_rel), &config.expected_bytes).expect("write expected");

    let input_sha = sha256_digest(&config.input_bytes);
    let expected_sha = sha256_digest(&config.expected_bytes);

    let manifest_json = serde_json::json!({
        "schema_version": 2,
        "source_revision": "60c476645ee6dae1f6392336a7f3c593d2163ae3",
        "identities": {
            "protocol": 45,
            "chunk_schema": 9,
            "player_schema": 9,
            "world_metadata": 6,
            "companions_ai_schema": 5,
            "hostile_mobs_schema": 2,
            "passive_mobs_schema": 1,
            "engine_abi": 11,
            "region_format": 1,
            "agent_http": "v1",
            "agent_mcp": "v1"
        },
        "families": [
            {
                "id": config.family_id,
                "kind": "domain",
                "role": "semantic",
                "current_version": "v1",
                "supported_versions": config.supported_versions,
                "source": "packages/contracts/domain/test.json",
                "eventual_owner": "mornlea_domain",
                "numeric_semantics": "exact",
                "sources": [],
                "cases": config.family_cases
            }
        ],
        "cases": [
            {
                "id": config.case_id,
                "family": config.case_family,
                "version": config.case_version,
                "operation": config.case_operation,
                "input": {
                    "path": input_rel,
                    "sha256": input_sha
                },
                "input_format": config.input_format,
                "expected": {
                    "path": expected_rel,
                    "sha256": expected_sha
                },
                "checkpoints": config.checkpoints,
                "rust_consumer": config.rust_consumer
            }
        ]
    });

    let manifest_bytes = serde_json::to_vec_pretty(&manifest_json).expect("serialize manifest");
    let manifest_path = base.join("testdata/runtime-migration/contracts.json");
    fs::write(manifest_path, manifest_bytes).expect("write manifest");
}

fn write_raw_manifest(temp: &TempCorpusDir, content: &[u8]) {
    let manifest_path = temp
        .path()
        .join("testdata/runtime-migration/contracts.json");
    if let Some(parent) = manifest_path.parent() {
        fs::create_dir_all(parent).expect("create manifest dir");
    }
    fs::write(manifest_path, content).expect("write raw manifest");
}

#[test]
fn loads_minimal_valid_corpus_successfully() {
    let temp = TempCorpusDir::new();
    let config = MinimalCorpusConfig::default();
    write_corpus(&temp, &config);

    let cases = try_load_cases_from_root(temp.path(), CorpusConsumer::Domain)
        .expect("should load valid corpus");
    assert_eq!(cases.len(), 1);
    let c = &cases[0];
    assert_eq!(c.id, "domain.test/v1/case-1");
    assert_eq!(c.family, "domain.test");
    assert_eq!(c.version, "v1");
    assert_eq!(c.consumer, CorpusConsumer::Domain);
    assert_eq!(c.operation, "decode");
    assert_eq!(c.input_format, InputFormat::Json);
    assert!(c.input_json.is_some());
    assert_eq!(c.category, "test");
}

#[test]
fn rejects_duplicate_manifest_key() {
    let temp = TempCorpusDir::new();
    let config = MinimalCorpusConfig::default();
    write_corpus(&temp, &config);

    let raw = br#"{
        "schema_version": 2,
        "schema_version": 2,
        "source_revision": "60c476645ee6dae1f6392336a7f3c593d2163ae3",
        "families": [
            {
                "id": "domain.test",
                "kind": "domain",
                "role": "semantic",
                "current_version": "v1",
                "supported_versions": ["v1"],
                "source": "packages/contracts/domain/test.json",
                "eventual_owner": "mornlea_domain",
                "numeric_semantics": "exact",
                "sources": [],
                "cases": ["domain.test/v1/case-1"]
            }
        ],
        "cases": [
            {
                "id": "domain.test/v1/case-1",
                "family": "domain.test",
                "version": "v1",
                "operation": "decode",
                "input": {
                    "path": "testdata/runtime-migration/cases/test/case-1.input.json",
                    "sha256": "sha256:d12c9cb00cb8abeb6c43c16a3bc5cf635a90e38eb45d341938fc294eb88c1b75"
                },
                "input_format": "json",
                "expected": {
                    "path": "testdata/runtime-migration/cases/test/case-1.expected.json",
                    "sha256": "sha256:b1d8e124806a88b5ec585c5b410eb7058cc697ea3ca671c6ae797ad89487c674"
                },
                "checkpoints": ["0"],
                "rust_consumer": "mornlea_domain"
            }
        ]
    }"#;
    write_raw_manifest(&temp, raw);

    let err = try_load_cases_from_root(temp.path(), CorpusConsumer::Domain)
        .expect_err("should reject duplicate manifest key");
    assert!(
        err.message.contains("contracts.json"),
        "expected error to name manifest, got: {}",
        err.message
    );
    assert!(
        err.message.to_lowercase().contains("duplicate"),
        "expected error to mention duplicate, got: {}",
        err.message
    );
}

#[test]
fn rejects_duplicate_input_key() {
    let temp = TempCorpusDir::new();
    let config = MinimalCorpusConfig {
        input_bytes: br#"{"valid":true,"valid":false}"#.to_vec(),
        ..Default::default()
    };
    write_corpus(&temp, &config);

    let err = try_load_cases_from_root(temp.path(), CorpusConsumer::Domain)
        .expect_err("should reject duplicate input key");
    assert!(
        err.message.contains(config.case_id) || err.message.contains("case-1.input.json"),
        "expected error to name case ID or asset path, got: {}",
        err.message
    );
    assert!(
        err.message.to_lowercase().contains("duplicate"),
        "expected error to mention duplicate, got: {}",
        err.message
    );
}

#[test]
fn rejects_duplicate_expected_key() {
    let temp = TempCorpusDir::new();
    let config = MinimalCorpusConfig {
        expected_bytes: br#"{"category":"test","category":"test","kind":"ok"}"#.to_vec(),
        ..Default::default()
    };
    write_corpus(&temp, &config);

    let err = try_load_cases_from_root(temp.path(), CorpusConsumer::Domain)
        .expect_err("should reject duplicate expected key");
    assert!(
        err.message.contains(config.case_id) || err.message.contains("case-1.expected.json"),
        "expected error to name case ID or asset path, got: {}",
        err.message
    );
    assert!(
        err.message.to_lowercase().contains("duplicate"),
        "expected error to mention duplicate, got: {}",
        err.message
    );
}

#[test]
fn rejects_unknown_input_format() {
    let temp = TempCorpusDir::new();
    let config = MinimalCorpusConfig {
        input_format: "yaml",
        ..Default::default()
    };
    write_corpus(&temp, &config);

    let err = try_load_cases_from_root(temp.path(), CorpusConsumer::Domain)
        .expect_err("should reject unknown input format");
    assert!(
        err.message.to_lowercase().contains("input_format") || err.message.contains("yaml"),
        "expected error to mention input_format, got: {}",
        err.message
    );
}

#[test]
fn rejects_unknown_consumer() {
    let temp = TempCorpusDir::new();
    let config = MinimalCorpusConfig {
        rust_consumer: "unknown_consumer",
        ..Default::default()
    };
    write_corpus(&temp, &config);

    let err = try_load_cases_from_root(temp.path(), CorpusConsumer::Domain)
        .expect_err("should reject unknown consumer");
    assert!(
        err.message.to_lowercase().contains("consumer") || err.message.contains("unknown_consumer"),
        "expected error to mention consumer, got: {}",
        err.message
    );
}

#[test]
fn rejects_trailing_json_input() {
    let temp = TempCorpusDir::new();
    let config = MinimalCorpusConfig {
        input_bytes: br#"{"valid":true} trailing_extra"#.to_vec(),
        ..Default::default()
    };
    write_corpus(&temp, &config);

    let err = try_load_cases_from_root(temp.path(), CorpusConsumer::Domain)
        .expect_err("should reject trailing json input");
    assert!(
        err.message.to_lowercase().contains("trailing"),
        "expected error to mention trailing content, got: {}",
        err.message
    );
}

#[test]
fn rejects_symlinked_ancestor() {
    let temp = TempCorpusDir::new();
    let config = MinimalCorpusConfig::default();
    write_corpus(&temp, &config);

    #[cfg(unix)]
    {
        let cases_dir = temp.path().join("testdata/runtime-migration/cases");
        let real_cases_dir = temp.path().join("testdata/runtime-migration/real_cases");
        fs::rename(&cases_dir, &real_cases_dir).expect("rename cases dir");
        std::os::unix::fs::symlink(&real_cases_dir, &cases_dir).expect("create symlink to cases");

        let err = try_load_cases_from_root(temp.path(), CorpusConsumer::Domain)
            .expect_err("should reject symlinked ancestor");
        assert!(
            err.message.to_lowercase().contains("symlink"),
            "expected error to mention symlink, got: {}",
            err.message
        );
    }
}

#[test]
fn rejects_containment_escape() {
    let temp = TempCorpusDir::new();
    let base = temp.path();
    let cases_dir = base.join("testdata/runtime-migration/cases/test");
    fs::create_dir_all(&cases_dir).expect("create cases dir");

    let input_bytes = br#"{"valid":true}"#;
    let expected_bytes = br#"{"category":"test","kind":"ok"}"#;
    fs::write(cases_dir.join("case-1.input.json"), input_bytes).expect("write input");
    fs::write(cases_dir.join("case-1.expected.json"), expected_bytes).expect("write expected");

    let manifest_json = serde_json::json!({
        "schema_version": 2,
        "source_revision": "60c476645ee6dae1f6392336a7f3c593d2163ae3",
        "identities": {
            "protocol": 45,
            "chunk_schema": 9,
            "player_schema": 9,
            "world_metadata": 6,
            "companions_ai_schema": 5,
            "hostile_mobs_schema": 2,
            "passive_mobs_schema": 1,
            "engine_abi": 11,
            "region_format": 1,
            "agent_http": "v1",
            "agent_mcp": "v1"
        },
        "families": [
            {
                "id": "domain.test",
                "kind": "domain",
                "role": "semantic",
                "current_version": "v1",
                "supported_versions": ["v1"],
                "source": "packages/contracts/domain/test.json",
                "eventual_owner": "mornlea_domain",
                "numeric_semantics": "exact",
                "sources": [],
                "cases": ["domain.test/v1/case-1"]
            }
        ],
        "cases": [
            {
                "id": "domain.test/v1/case-1",
                "family": "domain.test",
                "version": "v1",
                "operation": "decode",
                "input": {
                    "path": "testdata/runtime-migration/cases/test/../../../../etc/passwd",
                    "sha256": "sha256:0000000000000000000000000000000000000000000000000000000000000000"
                },
                "input_format": "json",
                "expected": {
                    "path": "testdata/runtime-migration/cases/test/case-1.expected.json",
                    "sha256": sha256_digest(expected_bytes)
                },
                "checkpoints": ["0"],
                "rust_consumer": "mornlea_domain"
            }
        ]
    });
    write_raw_manifest(&temp, &serde_json::to_vec_pretty(&manifest_json).unwrap());

    let err = try_load_cases_from_root(temp.path(), CorpusConsumer::Domain)
        .expect_err("should reject escaping path");
    assert!(
        err.message.contains("..")
            || err.message.to_lowercase().contains("containment")
            || err.message.to_lowercase().contains("path"),
        "expected error to mention path or containment, got: {}",
        err.message
    );
}

#[test]
fn rejects_table_cases() {
    struct TableTestCase {
        name: &'static str,
        mutate: fn(&mut MinimalCorpusConfig),
    }

    let cases = [
        TableTestCase {
            name: "unknown_family",
            mutate: |c| {
                c.case_family = "domain.unknown";
                c.case_id = "domain.unknown/v1/case-1";
            },
        },
        TableTestCase {
            name: "unsupported_version",
            mutate: |c| {
                c.case_version = "v2";
                c.case_id = "domain.test/v2/case-1";
                c.family_cases = vec!["domain.test/v2/case-1"];
            },
        },
        TableTestCase {
            name: "bad_id_prefix",
            mutate: |c| {
                c.case_id = "wrong_prefix/v1/case-1";
                c.family_cases = vec!["wrong_prefix/v1/case-1"];
            },
        },
        TableTestCase {
            name: "family_case_list_mismatch",
            mutate: |c| {
                c.family_cases = vec!["domain.test/v1/other-case"];
            },
        },
        TableTestCase {
            name: "invalid_operation",
            mutate: |c| {
                c.case_operation = "invalid_op";
            },
        },
        TableTestCase {
            name: "empty_checkpoints",
            mutate: |c| {
                c.checkpoints = vec![];
            },
        },
        TableTestCase {
            name: "nondecimal_checkpoints",
            mutate: |c| {
                c.checkpoints = vec!["0x10"];
            },
        },
    ];

    for tc in cases {
        let temp = TempCorpusDir::new();
        let mut config = MinimalCorpusConfig::default();
        (tc.mutate)(&mut config);
        write_corpus(&temp, &config);

        let res = try_load_cases_from_root(temp.path(), CorpusConsumer::Domain);
        assert!(
            res.is_err(),
            "table case '{}' should have failed, but succeeded",
            tc.name
        );
    }
}
