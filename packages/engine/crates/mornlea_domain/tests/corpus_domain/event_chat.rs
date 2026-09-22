//! Topic module for the domain.event chat corpus cases.
//!
//! The one rule here is the chat-addressing fact an authoritative session
//! confirms for the one player that caused it. The executor parses every raw
//! field of the frozen envelope, replays the Go `ChatEvent.Validate`
//! precedence to select the first broken rule, and then proves the applicable
//! Rust constructor agrees: a classified acceptance must construct the exact
//! closed `ChatBody` and `ChatEvent`, and a classified rejection with a
//! constructible culprit must fail the mapped checked constructor with the
//! exact `DomainError`. Accepted fields are read back through getters, and a
//! rejected value has no getters, so its projection retains the raw inputs
//! exactly as the case named them, including an unknown numeric kind or
//! reason.
//!
//! The frozen precedence runs in three stages. The global stage checks the
//! event identity, the player identity and the canonical player name, then
//! rejects a non-speech payload carrying speech before any kind dispatch.
//! The kind-local stage then applies exactly the order the Go validator
//! froze: the reason slot, the companion identity, the companion name, and
//! finally the command or speech text. A zero event identity reaches
//! `ChatEvent::try_new` itself — the adapter constructs an otherwise valid
//! event around the zero id and requires `DomainError::InvalidIdentity`, so
//! the domain proof stays in the domain type rather than in an adapter
//! precheck.
//!
//! Raw kind, reason and cross-field combinations that cannot form any
//! `ChatBody` stay classifier-only: an unknown kind byte, the reserved
//! reject-reason domain, a failure reason outside 16..=20, a non-speech
//! branch carrying speech, a reason outside its branch, a command on the
//! speech or unaddressed branches, and a leaked companion identity on the
//! branches that forbid one. The adapter still emits the exact Go category
//! and rule for them, but no constructor verdict exists to check and no
//! `Unknown` enum variant is invented.

use super::support::{
    DispatchError, JsonMap, assert_domain_normalized, execute_topic, input_object, invalid_case,
    normalize_u64, normalize_uuid, normalized_error, normalized_ok, parse_uuid_hex,
    required_string, required_u64,
};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};
use mornlea_domain::{
    ChatBody, ChatEvent, ChatEventParts, CommandText, CompanionId, CompanionName, CompanionSpeaker,
    DisplayName, DomainError, PlayerId, SpeechText, TaskFailure, TaskState,
};
use serde_json::Value;

pub const EXPECTED_COUNT: usize = 44;

/// The sole rule this topic owns; the manifest's rule name is the only
/// discriminator the shared `domain.event` family carries.
const RULE: &str = "chat";

/// Raw wire kind bytes, mirroring the Go `ChatEventKind` constants.
const KIND_ACCEPTED: u64 = 1;
const KIND_REJECTED: u64 = 2;
const KIND_TASK_STARTED: u64 = 3;
const KIND_TASK_PROGRESS: u64 = 4;
const KIND_TASK_COMPLETED: u64 = 5;
const KIND_TASK_FAILED: u64 = 6;
const KIND_TASK_TIMED_OUT: u64 = 7;
const KIND_TASK_STOPPED: u64 = 8;
const KIND_SPEECH: u64 = 9;

/// Raw reject-reason bytes, mirroring the Go `ChatRejectReason` constants.
/// Value 3 is reserved and unpublished, so no named constant exists for it.
const REASON_NONE: u64 = 0;
const REASON_INVALID_FORMAT: u64 = 1;
const REASON_UNKNOWN_COMPANION: u64 = 2;
const REASON_QUEUE_FULL: u64 = 4;
const REASON_NOT_FOLLOWING: u64 = 5;

/// The closed `TaskFailReason` interval the failed-task branch admits.
const TASK_FAIL_MIN: u64 = 16;
const TASK_FAIL_MAX: u64 = 20;

/// The wire byte the kind and reason slots carry; a raw value above it
/// cannot be a wire record at all and is an invalid case, exactly as the Go
/// producer's decode bound reports.
const MAX_WIRE_BYTE: u64 = 255;

pub fn owns(case: &FrozenCase) -> bool {
    if case.consumer != CorpusConsumer::Domain
        || case.family != "domain.event"
        || case.version != "1"
        || case.operation != "admit"
    {
        return false;
    }
    match case
        .input_json
        .as_ref()
        .and_then(|v| v.get("rule"))
        .and_then(|v| v.as_str())
    {
        Some(rule) => rule == RULE,
        None => false,
    }
}

pub fn execute(case: &FrozenCase) -> Result<serde_json::Value, DispatchError> {
    let input = input_object(case)?;
    let rule = required_string(case, input, "rule")?;
    if rule != RULE {
        return Err(invalid_case(
            case,
            format!("unknown chat event rule '{rule}'"),
        ));
    }
    let raw = parse_raw(case, input)?;
    match classify(&raw) {
        Some(rejection) => {
            verify_rejection(case, &raw, &rejection)?;
            chat_error(case, &raw, &rejection)
        }
        None => execute_accepted(case, &raw),
    }
}

/// One classified rejection: its normalized category, its exact Go rule
/// name, and the checked constructor the culprit field must fail.
///
/// `proof` is `None` for a classifier-only rejection whose bad raw
/// combination cannot enter any closed Rust type, so no constructor verdict
/// exists to check.
struct Rejection {
    category: &'static str,
    rule: &'static str,
    proof: Option<Proof>,
}

/// The checked constructor a classified rejection must fail, with the exact
/// `DomainError` the failure must return. Every variant names one culprit
/// field, so a verdict that succeeds or fails differently is a contract
/// conflict rather than a corpus expectation.
enum Proof {
    /// `ChatEvent::try_new` must reject the zero event identity. The rest
    /// of the probe event is synthetic and valid, so the zero id is the
    /// only relation that can fail.
    ZeroEventId,
    /// `PlayerId::try_from_bytes` must reject the raw player identity.
    PlayerIdentity,
    /// `CompanionId::try_from_bytes` must reject the raw companion
    /// identity.
    CompanionIdentity,
    /// `DisplayName::try_from_canonical` must reject the raw player name.
    PlayerName,
    /// `CompanionName::try_from_canonical` must reject the raw companion
    /// name.
    CompanionName,
    /// `CommandText::try_from_canonical` must reject the raw command.
    CommandText,
    /// `SpeechText::try_from_canonical` must reject the raw speech.
    SpeechText,
}

impl Proof {
    /// The exact constructor error this proof requires.
    fn expected_error(&self) -> DomainError {
        match self {
            Proof::ZeroEventId | Proof::PlayerIdentity | Proof::CompanionIdentity => {
                DomainError::InvalidIdentity
            }
            Proof::PlayerName | Proof::CompanionName | Proof::CommandText | Proof::SpeechText => {
                DomainError::InvalidText
            }
        }
    }
}

/// The parsed raw shape of one frozen chat envelope. The numeric and byte
/// identities stay beside their raw text spellings because a rejection
/// publishes the raw values exactly as the case named them.
struct RawChat {
    event_id: u64,
    event_id_text: String,
    player_id: [u8; 16],
    player_id_text: String,
    companion_id: [u8; 16],
    companion_id_text: String,
    player_name: String,
    companion_name: String,
    kind: u64,
    reason: u64,
    command: String,
    speech: String,
}

/// Parses every raw field of the frozen envelope. The UUID hex parser keeps
/// the zero companion sentinel decodable, the kind and reason stay bounded
/// to the wire byte, and the event identity keeps its raw decimal spelling
/// for the rejection projection.
fn parse_raw(case: &FrozenCase, input: &JsonMap) -> Result<RawChat, DispatchError> {
    let event_id_text = required_string(case, input, "event_id")?.to_string();
    let event_id = event_id_text
        .parse::<u64>()
        .map_err(|_| invalid_case(case, "event_id is not a decimal u64 token"))?;
    let player_id_text = required_string(case, input, "player_id")?.to_string();
    let player_id = parse_uuid_hex(case, "player_id", &player_id_text)?;
    let companion_id_text = required_string(case, input, "companion_id")?.to_string();
    let companion_id = parse_uuid_hex(case, "companion_id", &companion_id_text)?;
    let kind = required_u64(case, input, "kind")?;
    if kind > MAX_WIRE_BYTE {
        return Err(invalid_case(
            case,
            format!("kind {kind} is outside the wire byte range"),
        ));
    }
    let reason = required_u64(case, input, "reason")?;
    if reason > MAX_WIRE_BYTE {
        return Err(invalid_case(
            case,
            format!("reason {reason} is outside the wire byte range"),
        ));
    }
    Ok(RawChat {
        event_id,
        event_id_text,
        player_id,
        player_id_text,
        companion_id,
        companion_id_text,
        player_name: required_string(case, input, "player_name")?.to_string(),
        companion_name: required_string(case, input, "companion_name")?.to_string(),
        kind,
        reason,
        command: required_string(case, input, "command")?.to_string(),
        speech: required_string(case, input, "speech")?.to_string(),
    })
}

/// Selects the first rule one raw envelope breaks, in the exact order the
/// Go `ChatEvent.Validate` checks them: the global identity and name gates,
/// the speech slot's kind exclusivity, then inside the kind switch the
/// reason, the companion identity, the companion name, and the command or
/// speech text.
///
/// Identity and text validity consult the checked Rust constructors
/// directly, because those constructors are the ported Go rules; a raw
/// combination no closed type can carry classifies without a proof.
fn classify(raw: &RawChat) -> Option<Rejection> {
    if raw.event_id == 0 {
        return Some(Rejection {
            category: "invalid-identity",
            rule: "chat_event.event_id",
            proof: Some(Proof::ZeroEventId),
        });
    }
    if PlayerId::try_from_bytes(raw.player_id).is_err() {
        return Some(Rejection {
            category: "invalid-identity",
            rule: "chat_event.player_id",
            proof: Some(Proof::PlayerIdentity),
        });
    }
    if DisplayName::try_from_canonical(raw.player_name.clone()).is_err() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "chat_event.player_name",
            proof: Some(Proof::PlayerName),
        });
    }
    // The speech slot belongs to the speech branch alone, so any other kind
    // carrying speech fails before the kind-specific switch.
    if raw.kind != KIND_SPEECH && !raw.speech.is_empty() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "chat_event.speech",
            proof: None,
        });
    }
    match raw.kind {
        KIND_ACCEPTED => {
            if raw.reason != REASON_NONE {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.reason",
                    proof: None,
                });
            }
            classify_companion_name_command(raw)
        }
        KIND_REJECTED => classify_rejected_branch(raw),
        KIND_TASK_STARTED | KIND_TASK_PROGRESS | KIND_TASK_COMPLETED | KIND_TASK_TIMED_OUT
        | KIND_TASK_STOPPED => {
            if raw.reason != REASON_NONE {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.reason",
                    proof: None,
                });
            }
            classify_companion_name_command(raw)
        }
        KIND_TASK_FAILED => {
            if !(TASK_FAIL_MIN..=TASK_FAIL_MAX).contains(&raw.reason) {
                return Some(Rejection {
                    category: "invalid-enum",
                    rule: "chat_event.task_failed.reason",
                    proof: None,
                });
            }
            classify_companion_name_command(raw)
        }
        KIND_SPEECH => {
            if raw.reason != REASON_NONE {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.reason",
                    proof: None,
                });
            }
            if !raw.command.is_empty() {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.command",
                    proof: None,
                });
            }
            if CompanionId::try_from_bytes(raw.companion_id).is_err() {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.companion_id",
                    proof: Some(Proof::CompanionIdentity),
                });
            }
            if CompanionName::try_from_canonical(raw.companion_name.clone()).is_err() {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.companion_name",
                    proof: Some(Proof::CompanionName),
                });
            }
            if SpeechText::try_from_canonical(raw.speech.clone()).is_err() {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.speech",
                    proof: Some(Proof::SpeechText),
                });
            }
            None
        }
        _ => Some(Rejection {
            category: "invalid-enum",
            rule: "chat_event.kind",
            proof: None,
        }),
    }
}

/// The shared kind-local walk of every companion-bearing branch after its
/// reason slot has been cleared: the full companion identity, the companion
/// name, and the command text decide in that order. The rejected branch's
/// queue-full and not-following reasons and the failed task's failure
/// reason carry their branch's own reason value, so the reason check belongs
/// to the call sites that require the None sentinel.
fn classify_companion_name_command(raw: &RawChat) -> Option<Rejection> {
    if CompanionId::try_from_bytes(raw.companion_id).is_err() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "chat_event.companion_id",
            proof: Some(Proof::CompanionIdentity),
        });
    }
    if CompanionName::try_from_canonical(raw.companion_name.clone()).is_err() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "chat_event.companion_name",
            proof: Some(Proof::CompanionName),
        });
    }
    if CommandText::try_from_canonical(raw.command.clone()).is_err() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "chat_event.command",
            proof: Some(Proof::CommandText),
        });
    }
    None
}

/// The rejected branch's own reason-local walk. The malformed-format and
/// unknown-companion reasons forbid a companion identity and a command
/// outright, and disagree about whether the name must be empty or valid;
/// the queue-full and not-following reasons keep the full speaker and the
/// command so the player can match the rejection to its instruction. Every
/// other reason value, including the reserved 3, is an enum rejection.
fn classify_rejected_branch(raw: &RawChat) -> Option<Rejection> {
    match raw.reason {
        REASON_INVALID_FORMAT => {
            if raw.companion_id != [0u8; 16] {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.companion_id",
                    proof: None,
                });
            }
            if !raw.command.is_empty() {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.command",
                    proof: None,
                });
            }
            if !raw.companion_name.is_empty() {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.companion_name",
                    proof: None,
                });
            }
            None
        }
        REASON_UNKNOWN_COMPANION => {
            if raw.companion_id != [0u8; 16] {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.companion_id",
                    proof: None,
                });
            }
            if !raw.command.is_empty() {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.command",
                    proof: None,
                });
            }
            if CompanionName::try_from_canonical(raw.companion_name.clone()).is_err() {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "chat_event.companion_name",
                    proof: Some(Proof::CompanionName),
                });
            }
            None
        }
        REASON_QUEUE_FULL | REASON_NOT_FOLLOWING => classify_companion_name_command(raw),
        _ => Some(Rejection {
            category: "invalid-enum",
            rule: "chat_event.rejected.reason",
            proof: None,
        }),
    }
}

/// Fails closed when the Go-rule classification and a checked Rust
/// constructor disagree about one case input: one side's rule moved, so the
/// conflict is reported instead of publishing whichever side was consulted
/// first.
fn classification_conflict(case: &FrozenCase, rule: &str) -> DispatchError {
    invalid_case(
        case,
        format!("classification '{rule}' disagrees with the Rust constructor"),
    )
}

/// Runs the constructor a classified rejection's proof names and requires
/// the exact mapped `DomainError`.
fn verify_rejection(
    case: &FrozenCase,
    raw: &RawChat,
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(proof) = &rejection.proof else {
        return Ok(());
    };
    let verdict: Result<(), DomainError> = match proof {
        Proof::ZeroEventId => {
            // The zero id must reach the domain constructor itself: the
            // probe event is otherwise fully valid, so the constructor's
            // `InvalidIdentity` is the domain proof rather than an adapter
            // precheck.
            ChatEvent::try_new(ChatEventParts {
                event_id: 0,
                player_id: PlayerId::try_from_bytes(SYNTHETIC_PLAYER_ID)
                    .expect("synthetic player identity is a valid UUIDv4"),
                player_name: DisplayName::try_from_canonical("Probe".to_string())
                    .expect("synthetic player name is canonical"),
                body: ChatBody::InvalidFormat,
            })
            .map(|_| ())
        }
        Proof::PlayerIdentity => PlayerId::try_from_bytes(raw.player_id).map(|_| ()),
        Proof::CompanionIdentity => CompanionId::try_from_bytes(raw.companion_id).map(|_| ()),
        Proof::PlayerName => DisplayName::try_from_canonical(raw.player_name.clone()).map(|_| ()),
        Proof::CompanionName => {
            CompanionName::try_from_canonical(raw.companion_name.clone()).map(|_| ())
        }
        Proof::CommandText => CommandText::try_from_canonical(raw.command.clone()).map(|_| ()),
        Proof::SpeechText => SpeechText::try_from_canonical(raw.speech.clone()).map(|_| ()),
    };
    match verdict {
        Err(error) if error == proof.expected_error() => Ok(()),
        _ => Err(classification_conflict(case, rejection.rule)),
    }
}

/// A valid synthetic UUIDv4 player identity for the zero-event-id probe:
/// version nibble 4 and RFC-4122 variant, exactly as the checked
/// constructor requires.
const SYNTHETIC_PLAYER_ID: [u8; 16] = [
    0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
];

/// Constructs the exact closed `ChatBody` an admitted raw envelope carries,
/// building every checked field from the raw values the classifier cleared.
/// A construction failure here is a contract conflict, because the
/// classifier already proved every part admissible.
fn construct_body(case: &FrozenCase, raw: &RawChat) -> Result<ChatBody, DispatchError> {
    let conflict = |error: DomainError| construction_conflict(case, error);
    let companion = || -> Result<CompanionSpeaker, DomainError> {
        Ok(CompanionSpeaker::new(
            CompanionId::try_from_bytes(raw.companion_id)?,
            CompanionName::try_from_canonical(raw.companion_name.clone())?,
        ))
    };
    let command = || CommandText::try_from_canonical(raw.command.clone());
    Ok(match (raw.kind, raw.reason) {
        (KIND_ACCEPTED, REASON_NONE) => ChatBody::Accepted {
            companion: companion().map_err(conflict)?,
            command: command().map_err(conflict)?,
        },
        (KIND_REJECTED, REASON_INVALID_FORMAT) => ChatBody::InvalidFormat,
        (KIND_REJECTED, REASON_UNKNOWN_COMPANION) => ChatBody::UnknownCompanion {
            name: CompanionName::try_from_canonical(raw.companion_name.clone())
                .map_err(conflict)?,
        },
        (KIND_REJECTED, REASON_QUEUE_FULL) => ChatBody::QueueFull {
            companion: companion().map_err(conflict)?,
            command: command().map_err(conflict)?,
        },
        (KIND_REJECTED, REASON_NOT_FOLLOWING) => ChatBody::NotFollowing {
            companion: companion().map_err(conflict)?,
            command: command().map_err(conflict)?,
        },
        (KIND_TASK_STARTED, REASON_NONE) => ChatBody::Task {
            companion: companion().map_err(conflict)?,
            command: command().map_err(conflict)?,
            state: TaskState::Started,
        },
        (KIND_TASK_PROGRESS, REASON_NONE) => ChatBody::Task {
            companion: companion().map_err(conflict)?,
            command: command().map_err(conflict)?,
            state: TaskState::Progress,
        },
        (KIND_TASK_COMPLETED, REASON_NONE) => ChatBody::Task {
            companion: companion().map_err(conflict)?,
            command: command().map_err(conflict)?,
            state: TaskState::Completed,
        },
        (KIND_TASK_TIMED_OUT, REASON_NONE) => ChatBody::Task {
            companion: companion().map_err(conflict)?,
            command: command().map_err(conflict)?,
            state: TaskState::TimedOut,
        },
        (KIND_TASK_STOPPED, REASON_NONE) => ChatBody::Task {
            companion: companion().map_err(conflict)?,
            command: command().map_err(conflict)?,
            state: TaskState::Stopped,
        },
        (KIND_TASK_FAILED, reason) => {
            let failure = match reason {
                16 => TaskFailure::PlannerUnavailable,
                17 => TaskFailure::InvalidPlan,
                18 => TaskFailure::PathUnreachable,
                19 => TaskFailure::WorldChanged,
                20 => TaskFailure::InventoryFull,
                other => {
                    return Err(classification_conflict(
                        case,
                        &format!("failure reason {other} passed classification"),
                    ));
                }
            };
            ChatBody::Task {
                companion: companion().map_err(conflict)?,
                command: command().map_err(conflict)?,
                state: TaskState::Failed(failure),
            }
        }
        (KIND_SPEECH, REASON_NONE) => ChatBody::Speech {
            companion: companion().map_err(conflict)?,
            text: SpeechText::try_from_canonical(raw.speech.clone()).map_err(conflict)?,
        },
        (kind, reason) => {
            return Err(classification_conflict(
                case,
                &format!("kind {kind} reason {reason} passed classification"),
            ));
        }
    })
}

/// Maps one constructor error onto a classification conflict so an admitted
/// construction failure names the contract conflict instead of panicking.
fn construction_conflict(case: &FrozenCase, error: DomainError) -> DispatchError {
    classification_conflict(case, &format!("{error:?}"))
}

/// Names the exact semantic branch one admitted body carries, which is the
/// accepted outcome's category. The match is exhaustive over the closed
/// union, so a new variant is a compile error here rather than a silent
/// renumbering.
fn body_category(body: &ChatBody) -> &'static str {
    match body {
        ChatBody::Accepted { .. } => "accepted",
        ChatBody::InvalidFormat => "invalid-format",
        ChatBody::UnknownCompanion { .. } => "unknown-companion",
        ChatBody::QueueFull { .. } => "queue-full",
        ChatBody::NotFollowing { .. } => "not-following",
        ChatBody::Task { state, .. } => match state {
            TaskState::Started => "task-started",
            TaskState::Progress => "task-progress",
            TaskState::Completed => "task-completed",
            TaskState::TimedOut => "task-timed-out",
            TaskState::Stopped => "task-stopped",
            TaskState::Failed(failure) => match failure {
                TaskFailure::PlannerUnavailable => "task-failed-planner-unavailable",
                TaskFailure::InvalidPlan => "task-failed-invalid-plan",
                TaskFailure::PathUnreachable => "task-failed-path-unreachable",
                TaskFailure::WorldChanged => "task-failed-world-changed",
                TaskFailure::InventoryFull => "task-failed-inventory-full",
            },
        },
        ChatBody::Speech { .. } => "speech",
    }
}

/// Executes one admitted envelope: construct the checked player fields and
/// the exact closed body, wrap them in `ChatEvent`, and normalize the event
/// identity, the player identity and name, and exactly the branch-legal
/// fields back through getters.
fn execute_accepted(case: &FrozenCase, raw: &RawChat) -> Result<serde_json::Value, DispatchError> {
    let player_id = PlayerId::try_from_bytes(raw.player_id)
        .map_err(|error| construction_conflict(case, error))?;
    let player_name = DisplayName::try_from_canonical(raw.player_name.clone())
        .map_err(|error| construction_conflict(case, error))?;
    let body = construct_body(case, raw)?;
    let event = ChatEvent::try_new(ChatEventParts {
        event_id: raw.event_id,
        player_id,
        player_name,
        body,
    })
    .map_err(|error| construction_conflict(case, error))?;

    let mut fields = JsonMap::new();
    fields.insert("event_id".to_string(), normalize_u64(event.event_id()));
    fields.insert(
        "player_id".to_string(),
        normalize_uuid(event.player_id().bytes()),
    );
    fields.insert(
        "player_name".to_string(),
        Value::String(event.player_name().as_str().to_string()),
    );
    match event.body() {
        ChatBody::Accepted { companion, command }
        | ChatBody::QueueFull { companion, command }
        | ChatBody::NotFollowing { companion, command }
        | ChatBody::Task {
            companion, command, ..
        } => {
            fields.insert(
                "companion_id".to_string(),
                normalize_uuid(companion.id().bytes()),
            );
            fields.insert(
                "companion_name".to_string(),
                Value::String(companion.name().as_str().to_string()),
            );
            fields.insert(
                "command".to_string(),
                Value::String(command.as_str().to_string()),
            );
        }
        ChatBody::InvalidFormat => {}
        ChatBody::UnknownCompanion { name } => {
            fields.insert(
                "companion_name".to_string(),
                Value::String(name.as_str().to_string()),
            );
        }
        ChatBody::Speech { companion, text } => {
            fields.insert(
                "companion_id".to_string(),
                normalize_uuid(companion.id().bytes()),
            );
            fields.insert(
                "companion_name".to_string(),
                Value::String(companion.name().as_str().to_string()),
            );
            fields.insert(
                "speech".to_string(),
                Value::String(text.as_str().to_string()),
            );
        }
    }
    Ok(normalized_ok(body_category(event.body()), fields))
}

/// Publishes one classified rejection from the raw envelope, retaining every
/// raw input exactly as the case named it — including an unknown numeric
/// kind or reason — so a replay consumer sees the value the record carried
/// rather than a normalized one.
fn chat_error(
    case: &FrozenCase,
    raw: &RawChat,
    rejection: &Rejection,
) -> Result<serde_json::Value, DispatchError> {
    let mut fields = JsonMap::new();
    fields.insert(
        "event_id".to_string(),
        Value::String(raw.event_id_text.clone()),
    );
    fields.insert(
        "player_id".to_string(),
        Value::String(raw.player_id_text.clone()),
    );
    fields.insert(
        "companion_id".to_string(),
        Value::String(raw.companion_id_text.clone()),
    );
    fields.insert(
        "player_name".to_string(),
        Value::String(raw.player_name.clone()),
    );
    fields.insert(
        "companion_name".to_string(),
        Value::String(raw.companion_name.clone()),
    );
    fields.insert("kind".to_string(), Value::from(raw.kind));
    fields.insert("reason".to_string(), Value::from(raw.reason));
    fields.insert("command".to_string(), Value::String(raw.command.clone()));
    fields.insert("speech".to_string(), Value::String(raw.speech.clone()));
    normalized_error(case, rejection.category, rejection.rule, fields)
}

#[test]
fn event_chat_execute_44_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}

/// The exact label, category and rule of every boundary rejection the frozen
/// table pins, in the table's boundary-row order. The two admitted
/// exact-bound rows are deliberately absent, and the focused test reads
/// this table so the executed evidence and the pinned expectations cannot
/// drift apart.
const BOUNDARY_REJECTIONS: &[(&str, &str, &str)] = &[
    (
        "chat-event-id-zero",
        "invalid-identity",
        "chat_event.event_id",
    ),
    (
        "chat-player-id-zero",
        "invalid-identity",
        "chat_event.player_id",
    ),
    (
        "chat-player-name-untrimmed",
        "invalid-value",
        "chat_event.player_name",
    ),
    (
        "chat-accepted-reason-not-none",
        "invalid-value",
        "chat_event.reason",
    ),
    (
        "chat-accepted-zero-companion",
        "invalid-value",
        "chat_event.companion_id",
    ),
    (
        "chat-accepted-spaced-companion-name",
        "invalid-value",
        "chat_event.companion_name",
    ),
    (
        "chat-accepted-empty-command",
        "invalid-value",
        "chat_event.command",
    ),
    (
        "chat-accepted-speech-leak",
        "invalid-value",
        "chat_event.speech",
    ),
    (
        "chat-invalid-format-leaks-companion-id",
        "invalid-value",
        "chat_event.companion_id",
    ),
    (
        "chat-invalid-format-leaks-companion-name",
        "invalid-value",
        "chat_event.companion_name",
    ),
    (
        "chat-invalid-format-leaks-command",
        "invalid-value",
        "chat_event.command",
    ),
    (
        "chat-unknown-companion-invalid-name",
        "invalid-value",
        "chat_event.companion_name",
    ),
    (
        "chat-unknown-companion-leaks-id",
        "invalid-value",
        "chat_event.companion_id",
    ),
    (
        "chat-queue-full-missing-companion",
        "invalid-value",
        "chat_event.companion_id",
    ),
    (
        "chat-not-following-empty-command",
        "invalid-value",
        "chat_event.command",
    ),
    (
        "chat-rejected-reserved-reason-three",
        "invalid-enum",
        "chat_event.rejected.reason",
    ),
    (
        "chat-task-started-reason-not-none",
        "invalid-value",
        "chat_event.reason",
    ),
    (
        "chat-task-failed-reason-fifteen",
        "invalid-enum",
        "chat_event.task_failed.reason",
    ),
    (
        "chat-task-failed-reason-twenty-one",
        "invalid-enum",
        "chat_event.task_failed.reason",
    ),
    (
        "chat-task-event-speech-leak",
        "invalid-value",
        "chat_event.speech",
    ),
    (
        "chat-speech-has-command",
        "invalid-value",
        "chat_event.command",
    ),
    ("chat-speech-empty", "invalid-value", "chat_event.speech"),
    (
        "chat-speech-reason-not-none",
        "invalid-value",
        "chat_event.reason",
    ),
    ("chat-kind-unknown", "invalid-enum", "chat_event.kind"),
    (
        "chat-accepted-command-1025-bytes",
        "invalid-value",
        "chat_event.command",
    ),
    (
        "chat-speech-257-bytes",
        "invalid-value",
        "chat_event.speech",
    ),
];

/// Pins the executed evidence's exact shape: every one of the sixteen legal
/// branch categories (the accepted and speech branches twice, once for the
/// branch row and once for the exact-bound row), every boundary rejection's
/// category and rule, the reserved reason values a rejection retains, the
/// zero companion sentinel semantics, the illegal command/speech leakage
/// rules, and the zero event identity outcome.
#[test]
fn event_chat_pins_branches_and_boundaries() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    let by_label = |label: &str| {
        executed
            .iter()
            .find(|case| case.case.id == format!("domain.event/1/{label}"))
            .unwrap_or_else(|| panic!("missing case chat-{label}"))
    };

    // Every legal branch executes, with the exact-bound rows admitted
    // beside their branch rows.
    let mut branches = std::collections::BTreeMap::new();
    for case in &executed {
        if case.actual["kind"] == "ok" {
            *branches
                .entry(case.actual["category"].as_str().unwrap().to_string())
                .or_insert(0) += 1;
        }
    }
    let mut expected_branches = std::collections::BTreeMap::new();
    for branch in [
        "accepted",
        "invalid-format",
        "unknown-companion",
        "queue-full",
        "not-following",
        "task-started",
        "task-progress",
        "task-completed",
        "task-timed-out",
        "task-stopped",
        "task-failed-planner-unavailable",
        "task-failed-invalid-plan",
        "task-failed-path-unreachable",
        "task-failed-world-changed",
        "task-failed-inventory-full",
        "speech",
    ] {
        expected_branches.insert(branch.to_string(), 1);
    }
    *expected_branches.get_mut("accepted").unwrap() = 2;
    *expected_branches.get_mut("speech").unwrap() = 2;
    assert_eq!(branches, expected_branches);

    // The exact-bound rows carry their full texts.
    let bound_command = by_label("chat-accepted-command-1024-bytes");
    assert_eq!(bound_command.actual["kind"], "ok");
    assert_eq!(
        bound_command.actual["fields"]["command"]
            .as_str()
            .unwrap()
            .len(),
        1024
    );
    let bound_speech = by_label("chat-speech-256-bytes");
    assert_eq!(bound_speech.actual["kind"], "ok");
    assert_eq!(
        bound_speech.actual["fields"]["speech"]
            .as_str()
            .unwrap()
            .len(),
        256
    );

    // Every boundary rejection publishes its exact category and rule, and
    // the rejections the table names are exactly the rejections that ran.
    let mut rejected_labels = std::collections::BTreeSet::new();
    for case in &executed {
        if case.actual["kind"] == "error" {
            rejected_labels.insert(case.case.id.clone());
        }
    }
    assert_eq!(rejected_labels.len(), BOUNDARY_REJECTIONS.len());
    for (label, category, rule) in BOUNDARY_REJECTIONS {
        let rejected = by_label(label);
        assert_eq!(rejected.actual["kind"], "error", "{label}");
        assert_eq!(rejected.actual["category"], *category, "{label}");
        assert_eq!(rejected.actual["fields"]["rule"], *rule, "{label}");
        assert!(
            rejected_labels.contains(&format!("domain.event/1/{label}")),
            "{label} must be one of the executed rejections"
        );
    }

    // The reserved reject reason 3 and the failure reasons 15 and 21 stay
    // in the raw rejection fields exactly as the record carried them.
    assert_eq!(
        by_label("chat-rejected-reserved-reason-three").actual["fields"]["reason"],
        3
    );
    assert_eq!(
        by_label("chat-task-failed-reason-fifteen").actual["fields"]["reason"],
        15
    );
    assert_eq!(
        by_label("chat-task-failed-reason-twenty-one").actual["fields"]["reason"],
        21
    );

    // The zero companion sentinel: a companion-bearing branch rejects the
    // zero identity while retaining the raw zero UUID, and the unaddressed
    // branches publish no companion fields at all.
    let zero_companion = by_label("chat-accepted-zero-companion");
    assert_eq!(
        zero_companion.actual["fields"]["companion_id"],
        "00000000000000000000000000000000"
    );
    let invalid_format = by_label("chat-rejected-invalid-format");
    assert_eq!(invalid_format.actual["kind"], "ok");
    assert!(
        invalid_format.actual["fields"]
            .get("companion_id")
            .is_none()
    );
    assert!(
        invalid_format.actual["fields"]
            .get("companion_name")
            .is_none()
    );
    assert!(invalid_format.actual["fields"].get("command").is_none());
    let unknown = by_label("chat-rejected-unknown-companion");
    assert_eq!(unknown.actual["kind"], "ok");
    assert!(unknown.actual["fields"].get("companion_id").is_none());
    assert_eq!(unknown.actual["fields"]["companion_name"], "Buddy");

    // The illegal text-slot leakage: speech on a non-speech branch and a
    // command on the speech branch both reject before the text rules.
    assert_eq!(
        by_label("chat-accepted-speech-leak").actual["fields"]["rule"],
        "chat_event.speech"
    );
    assert_eq!(
        by_label("chat-speech-has-command").actual["fields"]["rule"],
        "chat_event.command"
    );

    // The zero event identity: the rejection retains the raw zero id and
    // the domain constructor proof happened inside `execute`.
    let zero_id = by_label("chat-event-id-zero");
    assert_eq!(zero_id.actual["category"], "invalid-identity");
    assert_eq!(zero_id.actual["fields"]["event_id"], "0");

    // An unknown kind stays classifier-only and retains its raw value.
    assert_eq!(by_label("chat-kind-unknown").actual["fields"]["kind"], 10);
}

/// Proves the classifier-only enum domains with direct raw probes: a reject
/// reason the published set does not name (including the None sentinel on
/// the rejected branch) rejects as the reason enum, and an unknown kind
/// above the known domain rejects as the kind enum. Neither combination
/// constructs anything, so the outcome is the classifier's alone.
#[test]
fn event_chat_probes_unfrozen_enum_domains() {
    let seed = serde_json::json!({
        "consumer": "mornlea_domain",
        "rule": "chat",
        "event_id": "7",
        "player_id": "00112233445546778899aabbccddeeff",
        "companion_id": "2233445546674889aabbccddeeff00ff",
        "player_name": "Alice",
        "companion_name": "Buddy",
        "kind": 2,
        "reason": 0,
        "command": "gather stone",
        "speech": "",
    });
    let reason_zero = probe_case(
        "domain.event/1/probe-chat-rejected-reason-none",
        seed.clone(),
    );
    let outcome = execute(&reason_zero).expect("execute rejected-reason-none probe");
    assert_eq!(outcome["kind"], "error");
    assert_eq!(outcome["category"], "invalid-enum");
    assert_eq!(outcome["fields"]["rule"], "chat_event.rejected.reason");
    assert_eq!(outcome["fields"]["reason"], 0);

    let mut kind_high = seed.clone();
    kind_high["kind"] = serde_json::json!(255);
    let kind_high = probe_case("domain.event/1/probe-chat-kind-255", kind_high);
    let outcome = execute(&kind_high).expect("execute kind-255 probe");
    assert_eq!(outcome["kind"], "error");
    assert_eq!(outcome["category"], "invalid-enum");
    assert_eq!(outcome["fields"]["rule"], "chat_event.kind");
    assert_eq!(outcome["fields"]["kind"], 255);
}

/// Builds one synthetic case around a raw input object, the shape the raw
/// probes execute. The frozen `normalized` value stays null because the
/// probes compare the executed outcome directly rather than through the
/// frozen comparator.
fn probe_case(id: &str, input: serde_json::Value) -> FrozenCase {
    FrozenCase {
        id: id.to_string(),
        family: "domain.event".to_string(),
        version: "1".to_string(),
        consumer: CorpusConsumer::Domain,
        operation: "admit".to_string(),
        arguments: serde_json::Value::Null,
        input_format: crate::runtime_corpus::InputFormat::Json,
        input: Vec::new(),
        input_json: Some(input),
        normalized: serde_json::Value::Null,
        encoded: None,
        category: String::new(),
    }
}
