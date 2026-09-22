//! Chat event contracts for `mornlea_domain`.
//!
//! The Go baseline is the `protocol.ChatEvent` validator in
//! `packages/shared/network/protocol`, which publishes the chat addressing
//! facts an authoritative session confirms for one player. The validator
//! admits exactly sixteen legal branch shapes: the accepted command, the
//! four rejections (malformed format, unknown companion, queue full, not
//! following), the five plain task facts (started, progress, completed,
//! timed out, stopped), the five failure reasons inside the failed task
//! fact, and the companion speech line.
//!
//! This crate closes that surface as a semantic union: every accepted wire
//! combination has exactly one `ChatBody` variant, and every illegal
//! cross-field combination the Go validator rejects — speech on a
//! non-speech branch, a command on speech or malformed format, a leaked
//! companion identity on malformed format or unknown companion — has no
//! constructible state at all, because the variant field lists are the
//! rule. Raw invalid kind and reason bytes stay corpus-adapter concerns:
//! no type here exposes an enum number or an `Unknown` member.
//!
//! Absence is the variant shape, never a zero identity: the malformed
//! format branch carries nothing, the unknown companion branch keeps only
//! the target name, and `CompanionId` itself rejects the zero UUID. The
//! event carries no routing recipient, publish tick, reason byte or
//! command sequence; its event id names the chat acknowledgment itself,
//! and a chat command travels through its own FIFO with no sequence.

use mornlea_domain::{
    ChatBody, ChatEvent, ChatEventParts, CommandText, CompanionId, CompanionName, CompanionSpeaker,
    DisplayName, DomainError, PlayerId, SpeechText, TaskFailure, TaskState,
};

/// The seed player identity: the same UUIDv4 the frozen chat corpus carries.
fn player_id() -> PlayerId {
    PlayerId::try_from_bytes([
        0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee,
        0xff,
    ])
    .expect("the identity is a valid UUIDv4")
}

/// The seed companion identity: the same UUIDv4 the frozen chat corpus
/// carries.
fn companion_id() -> CompanionId {
    CompanionId::try_from_bytes([
        0x22, 0x33, 0x44, 0x55, 0x46, 0x67, 0x48, 0x89, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00,
        0xff,
    ])
    .expect("the identity is a valid UUIDv4")
}

fn player_name() -> DisplayName {
    DisplayName::try_from_canonical("Alice".into()).expect("the name is canonical")
}

fn companion_name() -> CompanionName {
    CompanionName::try_from_canonical("Buddy".into()).expect("the name is canonical")
}

fn command() -> CommandText {
    CommandText::try_from_canonical("Buddy fetch the sword".into()).expect("the command is bounded")
}

fn speech() -> SpeechText {
    SpeechText::try_from_canonical("The forge is warm tonight.".into())
        .expect("the speech is bounded")
}

fn speaker() -> CompanionSpeaker {
    CompanionSpeaker::new(companion_id(), companion_name())
}

/// One admitted chat event around a body.
fn chat_event(body: ChatBody) -> ChatEvent {
    ChatEvent::try_new(ChatEventParts {
        event_id: 7,
        player_id: player_id(),
        player_name: player_name(),
        body,
    })
    .expect("a nonzero event with checked parts is admitted")
}

/// The task fact body for one lifecycle state.
fn task_body(state: TaskState) -> ChatBody {
    ChatBody::Task {
        companion: speaker(),
        command: command(),
        state,
    }
}

/// The sixteen legal branch shapes as one closed classification, so the
/// exhaustive match over `ChatBody`, `TaskState` and `TaskFailure` below is
/// forced by the compiler and a variant added later fails to compile here.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum Branch {
    Accepted,
    InvalidFormat,
    UnknownCompanion,
    QueueFull,
    NotFollowing,
    TaskStarted,
    TaskProgress,
    TaskCompleted,
    TaskTimedOut,
    TaskStopped,
    FailedPlannerUnavailable,
    FailedInvalidPlan,
    FailedPathUnreachable,
    FailedWorldChanged,
    FailedInventoryFull,
    Speech,
}

/// Classifies one body into the sixteen legal branch shapes.
///
/// The match is exhaustive over all three closed enums without a wildcard,
/// so it is the complete proof that the union unfolds into exactly these
/// branches.
fn classify(body: &ChatBody) -> Branch {
    match body {
        ChatBody::Accepted { .. } => Branch::Accepted,
        ChatBody::InvalidFormat => Branch::InvalidFormat,
        ChatBody::UnknownCompanion { .. } => Branch::UnknownCompanion,
        ChatBody::QueueFull { .. } => Branch::QueueFull,
        ChatBody::NotFollowing { .. } => Branch::NotFollowing,
        ChatBody::Task { state, .. } => match state {
            TaskState::Started => Branch::TaskStarted,
            TaskState::Progress => Branch::TaskProgress,
            TaskState::Completed => Branch::TaskCompleted,
            TaskState::TimedOut => Branch::TaskTimedOut,
            TaskState::Stopped => Branch::TaskStopped,
            TaskState::Failed(failure) => match failure {
                TaskFailure::PlannerUnavailable => Branch::FailedPlannerUnavailable,
                TaskFailure::InvalidPlan => Branch::FailedInvalidPlan,
                TaskFailure::PathUnreachable => Branch::FailedPathUnreachable,
                TaskFailure::WorldChanged => Branch::FailedWorldChanged,
                TaskFailure::InventoryFull => Branch::FailedInventoryFull,
            },
        },
        ChatBody::Speech { .. } => Branch::Speech,
    }
}

/// Builds the sixteen legal branch shapes the Go chat validator admits, one
/// event each, in the order the frozen corpus enumerates them.
fn sixteen_legal_events() -> Vec<ChatEvent> {
    vec![
        chat_event(ChatBody::Accepted {
            companion: speaker(),
            command: command(),
        }),
        chat_event(ChatBody::InvalidFormat),
        chat_event(ChatBody::UnknownCompanion {
            name: companion_name(),
        }),
        chat_event(ChatBody::QueueFull {
            companion: speaker(),
            command: command(),
        }),
        chat_event(ChatBody::NotFollowing {
            companion: speaker(),
            command: command(),
        }),
        chat_event(task_body(TaskState::Started)),
        chat_event(task_body(TaskState::Progress)),
        chat_event(task_body(TaskState::Completed)),
        chat_event(task_body(TaskState::TimedOut)),
        chat_event(task_body(TaskState::Stopped)),
        chat_event(task_body(TaskState::Failed(
            TaskFailure::PlannerUnavailable,
        ))),
        chat_event(task_body(TaskState::Failed(TaskFailure::InvalidPlan))),
        chat_event(task_body(TaskState::Failed(TaskFailure::PathUnreachable))),
        chat_event(task_body(TaskState::Failed(TaskFailure::WorldChanged))),
        chat_event(task_body(TaskState::Failed(TaskFailure::InventoryFull))),
        chat_event(ChatBody::Speech {
            companion: speaker(),
            text: speech(),
        }),
    ]
}

#[test]
fn chat_event_rejects_zero_event_id() {
    // A zero event id names no chat acknowledgment at all, exactly as the
    // Go `ChatEvent.Validate` gate rejects `EventID == 0` before any branch
    // rule runs. The rejection leaves the checked player fields and the
    // body untouched because there is nothing to rewrite.
    let rejected = ChatEvent::try_new(ChatEventParts {
        event_id: 0,
        player_id: player_id(),
        player_name: player_name(),
        body: ChatBody::InvalidFormat,
    });

    assert_eq!(rejected, Err(DomainError::InvalidIdentity));
}

#[test]
fn chat_event_keeps_player_identity_name_and_nonzero_id() {
    // The event owns the issuing player's checked identity and name plus
    // its own acknowledgment id, reading them back exactly as submitted:
    // no field is normalized, reordered or re-derived, and the largest id
    // stays publishable because only zero is the absent form.
    let event = chat_event(ChatBody::Accepted {
        companion: speaker(),
        command: command(),
    });

    assert_eq!(event.event_id(), 7);
    assert_eq!(event.player_id(), player_id());
    assert_eq!(event.player_name().as_str(), "Alice");
    assert!(matches!(event.body(), ChatBody::Accepted { .. }));

    let boundary = ChatEvent::try_new(ChatEventParts {
        event_id: u64::MAX,
        player_id: player_id(),
        player_name: player_name(),
        body: ChatBody::InvalidFormat,
    })
    .expect("the largest event id is a publishable acknowledgment");

    assert_eq!(boundary.event_id(), u64::MAX);
}

#[test]
fn chat_body_constructs_accepted_invalid_format_unknown_companion_queue_full_and_not_following() {
    // The addressing branches. Accepted, QueueFull and NotFollowing carry
    // the full companion speaker plus the command so the issuing player can
    // match the outcome to the exact instruction; the malformed-format
    // branch carries nothing at all because no addressing happened; and the
    // unknown-companion branch keeps only the target name so the player can
    // check the spelling. Each shape is read back through the variant's own
    // fields, so a field a later node adds or removes fails to compile here.
    let accepted = chat_event(ChatBody::Accepted {
        companion: speaker(),
        command: command(),
    });
    let ChatBody::Accepted {
        companion,
        command: restated,
    } = accepted.body()
    else {
        panic!("the accepted body must destructure as the accepted branch")
    };
    assert_eq!(companion.id(), companion_id());
    assert_eq!(companion.name().as_str(), "Buddy");
    assert_eq!(restated.as_str(), "Buddy fetch the sword");

    let invalid_format = chat_event(ChatBody::InvalidFormat);
    assert!(matches!(invalid_format.body(), ChatBody::InvalidFormat));

    let unknown = chat_event(ChatBody::UnknownCompanion {
        name: companion_name(),
    });
    let ChatBody::UnknownCompanion { name } = unknown.body() else {
        panic!("the unknown-companion body must destructure as the name-only branch")
    };
    assert_eq!(name.as_str(), "Buddy");

    for body in [
        ChatBody::QueueFull {
            companion: speaker(),
            command: command(),
        },
        ChatBody::NotFollowing {
            companion: speaker(),
            command: command(),
        },
    ] {
        let event = chat_event(body);
        let (companion, command) = match event.body() {
            ChatBody::QueueFull { companion, command }
            | ChatBody::NotFollowing { companion, command } => (companion, command),
            _ => panic!("the rejection must stay one of the two companion-bearing branches"),
        };
        assert_eq!(companion.id(), companion_id());
        assert_eq!(companion.name().as_str(), "Buddy");
        assert_eq!(command.as_str(), "Buddy fetch the sword");
    }
}

#[test]
fn chat_task_state_constructs_started_progress_completed_timed_out_and_stopped() {
    // The five plain task facts restate the player's original command with
    // the full companion speaker, exactly as the Go task fact kinds admit
    // them: each state round-trips through the `Task` variant unchanged.
    for state in [
        TaskState::Started,
        TaskState::Progress,
        TaskState::Completed,
        TaskState::TimedOut,
        TaskState::Stopped,
    ] {
        let event = chat_event(task_body(state));
        let ChatBody::Task {
            companion,
            command,
            state: observed,
        } = event.body()
        else {
            panic!("the task body must destructure as the task branch")
        };
        assert_eq!(companion.id(), companion_id());
        assert_eq!(command.as_str(), "Buddy fetch the sword");
        assert_eq!(*observed, state);
    }
}

#[test]
fn chat_task_failure_constructs_all_five_reasons() {
    // The five failure reasons the Go `validTaskFailReason` predicate
    // admits, which ride only inside `TaskState::Failed`: each reason
    // round-trips with the companion and the restated command intact, and
    // the exhaustive match over the reason has no wildcard arm.
    for failure in [
        TaskFailure::PlannerUnavailable,
        TaskFailure::InvalidPlan,
        TaskFailure::PathUnreachable,
        TaskFailure::WorldChanged,
        TaskFailure::InventoryFull,
    ] {
        let event = chat_event(task_body(TaskState::Failed(failure)));
        let ChatBody::Task {
            state: TaskState::Failed(observed),
            ..
        } = event.body()
        else {
            panic!("the failed task body must destructure as the failed state")
        };
        assert_eq!(*observed, failure);
    }
}

#[test]
fn chat_body_constructs_speech_separately_from_command() {
    // Speech is the only branch carrying model-generated text, and its
    // slot is a different checked type from the command: the two cannot be
    // swapped by construction, and the branch binds `text` alone with no
    // command field to read.
    let event = chat_event(ChatBody::Speech {
        companion: speaker(),
        text: speech(),
    });
    let ChatBody::Speech { companion, text } = event.body() else {
        panic!("the speech body must destructure as the speech branch")
    };
    assert_eq!(companion.id(), companion_id());
    assert_eq!(companion.name().as_str(), "Buddy");
    assert_eq!(text.as_str(), "The forge is warm tonight.");
}

#[test]
fn chat_union_has_exactly_sixteen_legal_branch_shapes() {
    // The sixteen events built from the frozen corpus shapes classify into
    // the sixteen closed branches, each occurring exactly once: the union
    // unfolds into exactly the accepted wire surface with no extra branch
    // and no missing one.
    let events = sixteen_legal_events();
    assert_eq!(events.len(), 16);

    let all_branches = [
        Branch::Accepted,
        Branch::InvalidFormat,
        Branch::UnknownCompanion,
        Branch::QueueFull,
        Branch::NotFollowing,
        Branch::TaskStarted,
        Branch::TaskProgress,
        Branch::TaskCompleted,
        Branch::TaskTimedOut,
        Branch::TaskStopped,
        Branch::FailedPlannerUnavailable,
        Branch::FailedInvalidPlan,
        Branch::FailedPathUnreachable,
        Branch::FailedWorldChanged,
        Branch::FailedInventoryFull,
        Branch::Speech,
    ];
    for branch in all_branches {
        let occurrences = events
            .iter()
            .filter(|event| classify(event.body()) == branch)
            .count();
        assert_eq!(
            occurrences, 1,
            "the {branch:?} branch occurs exactly once across the sixteen legal shapes"
        );
    }
}

#[test]
fn chat_union_never_uses_zero_companion_identity_as_absence() {
    // Absence is the variant shape, never a zero identity: the two branches
    // that speak to an absent companion construct with no companion value
    // at all, and the zero UUID cannot produce a `CompanionId` in the first
    // place, so no constructor can smuggle an absent speaker into a
    // companion-bearing branch.
    assert_eq!(
        CompanionId::try_from_bytes([0; 16]),
        Err(DomainError::InvalidIdentity)
    );

    let invalid_format = chat_event(ChatBody::InvalidFormat);
    match invalid_format.body() {
        ChatBody::Accepted { .. }
        | ChatBody::QueueFull { .. }
        | ChatBody::NotFollowing { .. }
        | ChatBody::Task { .. }
        | ChatBody::Speech { .. } => {
            panic!("the malformed format carries no companion-bearing branch");
        }
        ChatBody::InvalidFormat => {}
        ChatBody::UnknownCompanion { .. } => {
            panic!("the malformed format is not the unknown-companion branch");
        }
    }

    let unknown = chat_event(ChatBody::UnknownCompanion {
        name: companion_name(),
    });
    match unknown.body() {
        ChatBody::Accepted { .. }
        | ChatBody::QueueFull { .. }
        | ChatBody::NotFollowing { .. }
        | ChatBody::Task { .. }
        | ChatBody::Speech { .. } => {
            panic!("the unknown companion carries no companion-bearing branch");
        }
        ChatBody::InvalidFormat => {
            panic!("the unknown companion is not the malformed-format branch");
        }
        ChatBody::UnknownCompanion { name } => {
            assert_eq!(name.as_str(), "Buddy");
        }
    }
}

#[test]
fn chat_union_cannot_store_command_and_speech_together() {
    // The union carries exactly one text slot per branch, which the
    // compiler enforces because no variant declares both fields: across the
    // sixteen legal shapes, thirteen command branches bind `command` and
    // can see no `text`, the single speech branch binds `text` and can see
    // no `command`, and the two bare branches carry no text at all.
    let events = sixteen_legal_events();
    let mut command_slots = 0;
    let mut speech_slots = 0;
    let mut bare_slots = 0;

    for event in &events {
        match event.body() {
            ChatBody::Accepted { command, .. }
            | ChatBody::QueueFull { command, .. }
            | ChatBody::NotFollowing { command, .. }
            | ChatBody::Task { command, .. } => {
                assert_eq!(command.as_str(), "Buddy fetch the sword");
                command_slots += 1;
            }
            ChatBody::Speech { text, .. } => {
                assert_eq!(text.as_str(), "The forge is warm tonight.");
                speech_slots += 1;
            }
            ChatBody::InvalidFormat | ChatBody::UnknownCompanion { .. } => {
                bare_slots += 1;
            }
        }
    }

    assert_eq!(command_slots, 13);
    assert_eq!(speech_slots, 1);
    assert_eq!(bare_slots, 2);
}

#[test]
fn chat_event_has_no_recipient_tick_reason_byte_or_command_sequence() {
    // The parts literal names exactly the event id, the player identity,
    // the player name and the body, so a routing recipient, a publish tick,
    // a raw reason byte or a command sequence a later node adds fails to
    // compile here. Reading back exercises exactly the four frozen
    // getters, and the event id is the chat acknowledgment itself rather
    // than a `CommandEnvelope` sequence, because a chat command travels
    // through its own FIFO with no sequence at all.
    let event = chat_event(ChatBody::Accepted {
        companion: speaker(),
        command: command(),
    });

    assert_eq!(event.event_id(), 7);
    assert_eq!(event.player_id(), player_id());
    assert_eq!(event.player_name().as_str(), "Alice");
    assert!(matches!(event.body(), ChatBody::Accepted { .. }));
}
