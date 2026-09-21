//! Closed domain corpus dispatcher integration test.

#[path = "../../../tests/runtime_corpus.rs"]
pub(crate) mod runtime_corpus;

#[path = "corpus_domain/support.rs"]
pub mod support;

#[path = "corpus_domain/identity_text.rs"]
pub mod identity_text;

#[path = "corpus_domain/values.rs"]
pub mod values;

#[path = "corpus_domain/command_control.rs"]
pub mod command_control;

#[path = "corpus_domain/command_inventory.rs"]
pub mod command_inventory;

#[path = "corpus_domain/event_player.rs"]
pub mod event_player;

#[path = "corpus_domain/event_world.rs"]
pub mod event_world;

#[path = "corpus_domain/event_inventory.rs"]
pub mod event_inventory;

#[path = "corpus_domain/event_people.rs"]
pub mod event_people;

use runtime_corpus::{CorpusConsumer, try_load_cases_from_root};
use std::collections::HashSet;

#[test]
fn corpus_structure() {
    let root = runtime_corpus::find_repo_root();

    // 1. Verify consumer counts
    let domain_cases =
        try_load_cases_from_root(&root, CorpusConsumer::Domain).expect("load domain cases");
    assert_eq!(
        domain_cases.len(),
        376,
        "expected exactly 376 mornlea_domain cases"
    );

    let authority_cases = try_load_cases_from_root(&root, CorpusConsumer::ExternalRuntimeAuthority)
        .expect("load authority cases");
    assert_eq!(
        authority_cases.len(),
        1,
        "expected exactly 1 external:runtime-authority case"
    );
    assert_eq!(
        authority_cases[0].id, "domain.input/45/session-sequence-arrival",
        "external runtime authority case must be domain.input/45/session-sequence-arrival"
    );

    let agent_cases = try_load_cases_from_root(&root, CorpusConsumer::ExternalAgentContract)
        .expect("load agent contract cases");
    assert_eq!(
        agent_cases.len(),
        154,
        "expected exactly 154 external:agent-contract cases"
    );

    let frame_cases =
        try_load_cases_from_root(&root, CorpusConsumer::Frame).expect("load frame cases");
    assert_eq!(
        frame_cases.len(),
        2,
        "expected exactly 2 corpus_frame cases"
    );

    // 2. Verify all domain cases are json format and operation admit
    for case in &domain_cases {
        assert_eq!(
            case.operation, "admit",
            "case {} operation must be 'admit', got '{}'",
            case.id, case.operation
        );
        assert_eq!(
            case.input_format,
            runtime_corpus::InputFormat::Json,
            "case {} input_format must be json",
            case.id
        );
    }

    // 3. Verify external runtime case does not match any domain owner
    let authority_case = &authority_cases[0];
    assert!(!identity_text::owns(authority_case));
    assert!(!values::owns(authority_case));
    assert!(!command_control::owns(authority_case));
    assert!(!command_inventory::owns(authority_case));
    assert!(!event_player::owns(authority_case));
    assert!(!event_world::owns(authority_case));
    assert!(!event_inventory::owns(authority_case));
    assert!(!event_people::owns(authority_case));

    // 4. Exact single ownership of every domain case
    let mut seen_ids = HashSet::new();
    let mut identity_text_count = 0;
    let mut values_count = 0;
    let mut command_control_count = 0;
    let mut command_inventory_count = 0;
    let mut event_player_count = 0;
    let mut event_world_count = 0;
    let mut event_inventory_count = 0;
    let mut event_people_count = 0;

    for case in &domain_cases {
        assert!(
            seen_ids.insert(case.id.clone()),
            "duplicate domain case id: {}",
            case.id
        );

        let mut matches = 0;
        if identity_text::owns(case) {
            matches += 1;
            identity_text_count += 1;
        }
        if values::owns(case) {
            matches += 1;
            values_count += 1;
        }
        if command_control::owns(case) {
            matches += 1;
            command_control_count += 1;
        }
        if command_inventory::owns(case) {
            matches += 1;
            command_inventory_count += 1;
        }
        if event_player::owns(case) {
            matches += 1;
            event_player_count += 1;
        }
        if event_world::owns(case) {
            matches += 1;
            event_world_count += 1;
        }
        if event_inventory::owns(case) {
            matches += 1;
            event_inventory_count += 1;
        }
        if event_people::owns(case) {
            matches += 1;
            event_people_count += 1;
        }

        assert_eq!(
            matches, 1,
            "case {} owned by {} modules (expected exactly 1)",
            case.id, matches
        );
    }

    assert_eq!(identity_text_count, 31, "identity_text count mismatch");
    assert_eq!(values_count, 99, "values count mismatch");
    assert_eq!(command_control_count, 28, "command_control count mismatch");
    assert_eq!(
        command_inventory_count, 54,
        "command_inventory count mismatch"
    );
    assert_eq!(event_player_count, 47, "event_player count mismatch");
    assert_eq!(event_world_count, 38, "event_world count mismatch");
    assert_eq!(event_inventory_count, 33, "event_inventory count mismatch");
    assert_eq!(event_people_count, 46, "event_people count mismatch");
}
