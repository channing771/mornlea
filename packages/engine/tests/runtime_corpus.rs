//! Shared test-only corpus helpers for offline evidence verification.

use sha2::{Digest, Sha256};
use std::fs;
use std::path::{Path, PathBuf};

const MAX_MANIFEST_BYTES: u64 = 4 * 1024 * 1024;
const MAX_CASE_JSON_BYTES: u64 = 256 * 1024;
const MAX_BINARY_BYTES: u64 = 4 * 1024 * 1024;
const MAX_CASES: usize = 8192;

#[derive(Debug, Clone)]
#[allow(dead_code)]
pub struct FrozenCase {
    pub id: String,
    pub family: String,
    pub input: Vec<u8>,
    pub normalized: serde_json::Value,
    pub encoded: Option<Vec<u8>>,
    pub category: String,
}

pub fn find_repo_root() -> PathBuf {
    let manifest_dir = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    let mut current = manifest_dir.as_path();
    loop {
        let candidate = current.join("testdata/runtime-migration/contracts.json");
        if candidate.is_file() {
            return current.to_path_buf();
        }
        match current.parent() {
            Some(parent) => current = parent,
            None => panic!(
                "could not find testdata/runtime-migration/contracts.json walking up from {}",
                manifest_dir.display()
            ),
        }
    }
}

pub fn validate_relative_path(path_str: &str) {
    assert!(!path_str.is_empty(), "path cannot be empty");
    assert!(
        !path_str.starts_with('/'),
        "path cannot be absolute: {path_str}"
    );
    assert!(
        !path_str.contains('\\'),
        "path must use forward slashes: {path_str}"
    );
    for component in path_str.split('/') {
        assert!(component != "..", "path cannot contain '..': {path_str}");
        assert!(component != ".", "path cannot contain '.': {path_str}");
    }
}

pub fn read_bounded_file(path: &Path, max_bytes: u64) -> (Vec<u8>, String) {
    let meta =
        fs::symlink_metadata(path).unwrap_or_else(|e| panic!("stat {}: {e}", path.display()));
    assert!(
        !meta.file_type().is_symlink(),
        "symlinks forbidden: {}",
        path.display()
    );
    assert!(
        meta.file_type().is_file(),
        "must be a regular file: {}",
        path.display()
    );
    assert!(
        meta.len() <= max_bytes,
        "file {} size {} exceeds budget {}",
        path.display(),
        meta.len(),
        max_bytes
    );
    let bytes = fs::read(path).unwrap_or_else(|e| panic!("read {}: {e}", path.display()));
    let mut hasher = Sha256::new();
    hasher.update(&bytes);
    let digest = format!("sha256:{:x}", hasher.finalize());
    (bytes, digest)
}

pub fn load_case(id: &str) -> FrozenCase {
    let root = find_repo_root();
    let manifest_path = root.join("testdata/runtime-migration/contracts.json");
    let (manifest_bytes, _manifest_digest) = read_bounded_file(&manifest_path, MAX_MANIFEST_BYTES);

    let manifest: serde_json::Value = serde_json::from_slice(&manifest_bytes)
        .unwrap_or_else(|e| panic!("parse contracts.json: {e}"));

    let schema_version = manifest["schema_version"]
        .as_i64()
        .expect("schema_version integer");
    assert_eq!(schema_version, 2, "contracts.json schema_version must be 2");

    let cases = manifest["cases"]
        .as_array()
        .expect("cases array in contracts.json");
    assert!(
        cases.len() <= MAX_CASES,
        "cases count {} exceeds maximum {}",
        cases.len(),
        MAX_CASES
    );

    let mut seen_ids = std::collections::HashSet::new();
    let mut matched_case: Option<&serde_json::Value> = None;
    for c in cases {
        let cid = c["id"].as_str().expect("case id string");
        assert!(seen_ids.insert(cid), "duplicate case ID: {cid}");
        if cid == id {
            matched_case = Some(c);
        }
    }

    let c = matched_case.unwrap_or_else(|| panic!("case '{id}' not found in contracts.json"));
    let family = c["family"].as_str().expect("family string").to_string();
    let rust_consumer = c["rust_consumer"].as_str().expect("rust_consumer string");
    assert!(!rust_consumer.is_empty(), "rust_consumer cannot be empty");

    let input_path_str = c["input"]["path"].as_str().expect("input.path string");
    let input_sha = c["input"]["sha256"].as_str().expect("input.sha256 string");
    validate_relative_path(input_path_str);
    let input_format = c["input_format"].as_str().unwrap_or("binary");
    let max_input_bytes = if input_format == "json" {
        MAX_CASE_JSON_BYTES
    } else {
        MAX_BINARY_BYTES
    };
    let (input_bytes, actual_input_sha) =
        read_bounded_file(&root.join(input_path_str), max_input_bytes);
    assert_eq!(
        input_sha, actual_input_sha,
        "input sha256 mismatch for case {id}"
    );

    let expected_path_str = c["expected"]["path"]
        .as_str()
        .expect("expected.path string");
    let expected_sha = c["expected"]["sha256"]
        .as_str()
        .expect("expected.sha256 string");
    validate_relative_path(expected_path_str);
    let (expected_bytes, actual_expected_sha) =
        read_bounded_file(&root.join(expected_path_str), MAX_CASE_JSON_BYTES);
    assert_eq!(
        expected_sha, actual_expected_sha,
        "expected sha256 mismatch for case {id}"
    );
    let normalized: serde_json::Value = serde_json::from_slice(&expected_bytes)
        .unwrap_or_else(|e| panic!("parse expected json for {id}: {e}"));

    let category = normalized["category"]
        .as_str()
        .unwrap_or_default()
        .to_string();

    let encoded = if let Some(encoded_obj) = c.get("encoded") {
        if encoded_obj.is_null() {
            None
        } else {
            let enc_path = encoded_obj["path"].as_str().expect("encoded.path string");
            let enc_sha = encoded_obj["sha256"]
                .as_str()
                .expect("encoded.sha256 string");
            validate_relative_path(enc_path);
            let (enc_bytes, actual_enc_sha) =
                read_bounded_file(&root.join(enc_path), MAX_BINARY_BYTES);
            assert_eq!(
                enc_sha, actual_enc_sha,
                "encoded sha256 mismatch for case {id}"
            );
            Some(enc_bytes)
        }
    } else {
        None
    };

    FrozenCase {
        id: id.to_string(),
        family,
        input: input_bytes,
        normalized,
        encoded,
        category,
    }
}

pub fn assert_normalized(case: &FrozenCase, actual: serde_json::Value) {
    assert_eq!(
        case.normalized, actual,
        "case {} normalized JSON mismatch",
        case.id
    );
}

#[allow(dead_code)]
pub fn assert_rejected_unchanged<T: PartialEq + std::fmt::Debug>(
    before: &T,
    after: &T,
    error: bool,
) {
    assert!(error, "expected operation to fail");
    assert_eq!(before, after, "state changed on rejected operation");
}
