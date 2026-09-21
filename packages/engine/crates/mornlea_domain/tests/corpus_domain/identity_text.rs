//! Topic module for domain.identity_values corpus cases.

use super::support::{DispatchError, assert_domain_normalized, execute_topic};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};

pub const EXPECTED_COUNT: usize = 31;

const RULES: &[&str] = &[
    "player-id",
    "display-name",
    "companion-name",
    "command-text",
    "speech-text",
];

pub fn owns(case: &FrozenCase) -> bool {
    if case.consumer != CorpusConsumer::Domain || case.family != "domain.identity_values" {
        return false;
    }
    let rule = match case
        .input_json
        .as_ref()
        .and_then(|v| v.get("rule"))
        .and_then(|v| v.as_str())
    {
        Some(r) => r,
        None => return false,
    };
    RULES.contains(&rule)
}

pub fn execute(case: &FrozenCase) -> Result<serde_json::Value, DispatchError> {
    let rule = case
        .input_json
        .as_ref()
        .and_then(|v| v.get("rule"))
        .and_then(|v| v.as_str())
        .unwrap_or("")
        .to_string();
    Err(DispatchError::NotImplemented {
        case_id: case.id.clone(),
        rule,
    })
}

#[test]
#[ignore = "node 3.2 unignores this test"]
fn identity_text_execute_31_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}
