//! Topic module for domain.command_inventory corpus cases.

use super::support::{DispatchError, assert_domain_normalized, execute_topic};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};

pub const EXPECTED_COUNT: usize = 54;

const RULES: &[&str] = &[
    "move-inventory",
    "move-crafting",
    "move-container",
    "close-container",
    "drop-selected-item",
    "take-crafting-output",
    "equip-armor",
    "move-partial",
    "quick-move",
    "drop-stack",
    "chat-intent",
];

pub fn owns(case: &FrozenCase) -> bool {
    if case.consumer != CorpusConsumer::Domain || case.family != "domain.command_inventory" {
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
#[ignore = "node 3.5 unignores this test"]
fn command_inventory_execute_54_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}
