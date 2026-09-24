//! The chat event packet family: the common fallible surface and the closed
//! semantic chat union one reused wire text slot carries.
//!
//! `ChatEvent` (Play S→C 16) is the only publication whose text slot is reused
//! by kind: a companion speech event carries a model-generated line bounded by
//! `CHAT_SPEECH_TEXT_MAX_BYTES`, and every other kind restates the player's
//! original command bounded by the planner instruction limit. The decoder
//! therefore reads the kind first and then decides which text to read, and the
//! value gate validates the complete combination before any byte is sized or
//! published, which is the Go `ChatEvent.Validate` precedence: the global
//! identity and name gates, then the speech-slot exclusivity, then the
//! per-branch reason, identity, name and text rules.
//!
//! The record is a concrete packet with public mutable fields, so this suite
//! pins the design's surface order (`validate` → checked `encoded_len` →
//! capacity check → private `publish_packet`): a short or invalid `encode_into`
//! leaves the caller's buffer untouched, an invalid value wins over a short
//! destination, and every proper truncation of a canonical payload rejects.
//! The wire literals are the Go encoder's output, so the round-trip tests pin
//! byte-level parity rather than self-agreement.
//!
//! The bidirectional domain conversion is the other half of the contract: the
//! closed `ChatBody` union (seven variants unfolding into the sixteen legal
//! branch shapes) is the acceptance target, and the raw zero companion
//! identity maps to semantic absence in the two permitted rejection branches
//! alone. No case in this suite declares a session recipient, a publish tick or
//! a command sequence: routing lives in the publication envelope and the chat
//! FIFO, and `/warp` handling, targeting and event-id allocation stay outside
//! the protocol contract.
//!
//! The corpus evidence this family publishes is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.

use mornlea_domain::{ChatBody, CompanionSpeaker, TaskFailure, TaskState};
use mornlea_protocol::{
    CHAT_EVENT_ACCEPTED, CHAT_EVENT_COMPANION_SPEECH, CHAT_EVENT_MAX_WIRE_BYTES,
    CHAT_EVENT_REJECTED, CHAT_EVENT_TASK_COMPLETED, CHAT_EVENT_TASK_FAILED,
    CHAT_EVENT_TASK_PROGRESS, CHAT_EVENT_TASK_STARTED, CHAT_EVENT_TASK_STOPPED,
    CHAT_EVENT_TASK_TIMED_OUT, CHAT_REJECT_INVALID_FORMAT, CHAT_REJECT_NONE,
    CHAT_REJECT_NOT_FOLLOWING, CHAT_REJECT_QUEUE_FULL, CHAT_REJECT_UNKNOWN_COMPANION,
    CHAT_SPEECH_TEXT_MAX_BYTES, ChatEvent, ProtocolError, TASK_FAIL_INVALID_PLAN,
    TASK_FAIL_INVENTORY_FULL, TASK_FAIL_PATH_UNREACHABLE, TASK_FAIL_PLANNER_UNAVAILABLE,
    TASK_FAIL_WORLD_CHANGED,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The reviewed event identity the canonical vectors carry.
const EVENT_ID: u64 = 0x0102_0304_0506_0708;

/// The reviewed player identity: a valid UUIDv4 whose last byte differs from
/// the companion identity, so the two cannot be confused on the wire.
const PLAYER_ID: [u8; 16] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
];

/// The reviewed companion identity, absent only in the two rejection branches
/// that never addressed a companion.
const COMPANION_ID: [u8; 16] = [
    0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x46, 0x07, 0x88, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x10,
];

/// The reviewed player display name.
const PLAYER_NAME: &str = "Alice";

/// The reviewed companion name.
const COMPANION_NAME: &str = "Mira";

/// The reviewed original command every non-speech branch restates.
const COMMAND: &str = "@mira follow";

/// The reviewed model-generated speech line the speech branch carries.
const SPEECH: &str = "On it.";

/// One chat event vector: the reviewed record, the exact wire payload the Go
/// encoder publishes for it, and the closed domain body it maps to.
struct ChatEventVector {
    label: &'static str,
    event: ChatEvent,
    payload: Vec<u8>,
    body: ChatBody,
}

/// Builds a record through its public fields instead of the constructor, which
/// is how a mutated record reaches the surface: the constructor gate is not
/// the only thing that has to hold.
#[allow(clippy::too_many_arguments)]
fn record(
    event_id: u64,
    player_id: [u8; 16],
    player_name: &str,
    companion_id: [u8; 16],
    companion_name: &str,
    kind: u8,
    reject_reason: u8,
    command: &str,
    speech: &str,
) -> ChatEvent {
    ChatEvent {
        event_id,
        player_id: mornlea_protocol::PlayerId::try_from_bytes(player_id).expect("player identity"),
        player_name: player_name.to_owned(),
        companion_id,
        companion_name: companion_name.to_owned(),
        kind,
        reject_reason,
        command: command.to_owned(),
        speech: speech.to_owned(),
    }
}

/// The canonical companion speaker both directions carry.
fn speaker() -> CompanionSpeaker {
    CompanionSpeaker::new(
        mornlea_domain::CompanionId::try_from_bytes(COMPANION_ID).expect("companion identity"),
        mornlea_domain::CompanionName::try_from_canonical(COMPANION_NAME.to_owned())
            .expect("companion name"),
    )
}

/// The canonical original command every non-speech branch restates.
fn command_text() -> mornlea_domain::CommandText {
    mornlea_domain::CommandText::try_from_canonical(COMMAND.to_owned()).expect("command text")
}

/// The canonical speech line the speech branch carries.
fn speech_text() -> mornlea_domain::SpeechText {
    mornlea_domain::SpeechText::try_from_canonical(SPEECH.to_owned()).expect("speech text")
}

/// Renders one branch's wire payload: the event identity, the player identity
/// and name, the companion identity and name, the kind, the reason, and the
/// one text slot the kind selects.
///
/// The payload is built from the same field order the Go encoder uses, so the
/// literal is the encoder's own bytes rather than a self-agreement.
fn branch_payload(
    event_id: u64,
    player_id: [u8; 16],
    companion_id: [u8; 16],
    companion_name: &str,
    kind: u8,
    reject_reason: u8,
    text: &str,
) -> Vec<u8> {
    let mut wire = Vec::new();
    wire.extend_from_slice(&event_id.to_le_bytes());
    wire.extend_from_slice(&player_id);
    wire.push(PLAYER_NAME.len() as u8);
    wire.extend_from_slice(PLAYER_NAME.as_bytes());
    wire.extend_from_slice(&companion_id);
    wire.push(companion_name.len() as u8);
    wire.extend_from_slice(companion_name.as_bytes());
    wire.push(kind);
    wire.push(reject_reason);
    wire.push(text.len() as u8);
    wire.extend_from_slice(text.as_bytes());
    wire
}

/// The reviewed literals of all sixteen legal branch shapes.
///
/// Every shape is carried in both directions: the wire payload the Go encoder
/// publishes, the record the Rust decoder reproduces, and the closed domain
/// body the bidirectional conversion produces. The five plain task facts and
/// the five failure reasons share one payload shape and differ only in the
/// kind and reason bytes, so the table enumerates them rather than deriving
/// them from a loop.
fn chat_event_vectors() -> Vec<ChatEventVector> {
    let mut vectors = Vec::new();
    let mut push = |label: &'static str,
                    kind: u8,
                    reject_reason: u8,
                    companion_id: [u8; 16],
                    companion_name: &str,
                    command: &str,
                    speech: &str,
                    body: ChatBody| {
        let payload = branch_payload(
            EVENT_ID,
            PLAYER_ID,
            companion_id,
            companion_name,
            kind,
            reject_reason,
            if kind == CHAT_EVENT_COMPANION_SPEECH {
                speech
            } else {
                command
            },
        );
        vectors.push(ChatEventVector {
            label,
            event: record(
                EVENT_ID,
                PLAYER_ID,
                PLAYER_NAME,
                companion_id,
                companion_name,
                kind,
                reject_reason,
                command,
                speech,
            ),
            payload,
            body,
        });
    };

    push(
        "accepted",
        CHAT_EVENT_ACCEPTED,
        CHAT_REJECT_NONE,
        COMPANION_ID,
        COMPANION_NAME,
        COMMAND,
        "",
        ChatBody::Accepted {
            companion: speaker(),
            command: command_text(),
        },
    );
    // A malformed command never addressed a companion, so the branch carries
    // the absent identity, no name and no restated command.
    push(
        "invalid format",
        CHAT_EVENT_REJECTED,
        CHAT_REJECT_INVALID_FORMAT,
        [0u8; 16],
        "",
        "",
        "",
        ChatBody::InvalidFormat,
    );
    // Only the target name survives, so the player can check the spelling.
    push(
        "unknown companion",
        CHAT_EVENT_REJECTED,
        CHAT_REJECT_UNKNOWN_COMPANION,
        [0u8; 16],
        COMPANION_NAME,
        "",
        "",
        ChatBody::UnknownCompanion {
            name: mornlea_domain::CompanionName::try_from_canonical(COMPANION_NAME.to_owned())
                .expect("companion name"),
        },
    );
    push(
        "queue full",
        CHAT_EVENT_REJECTED,
        CHAT_REJECT_QUEUE_FULL,
        COMPANION_ID,
        COMPANION_NAME,
        COMMAND,
        "",
        ChatBody::QueueFull {
            companion: speaker(),
            command: command_text(),
        },
    );
    push(
        "not following",
        CHAT_EVENT_REJECTED,
        CHAT_REJECT_NOT_FOLLOWING,
        COMPANION_ID,
        COMPANION_NAME,
        COMMAND,
        "",
        ChatBody::NotFollowing {
            companion: speaker(),
            command: command_text(),
        },
    );
    for (label, kind, state) in [
        ("task started", CHAT_EVENT_TASK_STARTED, TaskState::Started),
        (
            "task progress",
            CHAT_EVENT_TASK_PROGRESS,
            TaskState::Progress,
        ),
        (
            "task completed",
            CHAT_EVENT_TASK_COMPLETED,
            TaskState::Completed,
        ),
        (
            "task timed out",
            CHAT_EVENT_TASK_TIMED_OUT,
            TaskState::TimedOut,
        ),
        ("task stopped", CHAT_EVENT_TASK_STOPPED, TaskState::Stopped),
    ] {
        push(
            label,
            kind,
            CHAT_REJECT_NONE,
            COMPANION_ID,
            COMPANION_NAME,
            COMMAND,
            "",
            ChatBody::Task {
                companion: speaker(),
                command: command_text(),
                state,
            },
        );
    }
    for (label, reason, failure) in [
        (
            "task failed planner unavailable",
            TASK_FAIL_PLANNER_UNAVAILABLE,
            TaskFailure::PlannerUnavailable,
        ),
        (
            "task failed invalid plan",
            TASK_FAIL_INVALID_PLAN,
            TaskFailure::InvalidPlan,
        ),
        (
            "task failed path unreachable",
            TASK_FAIL_PATH_UNREACHABLE,
            TaskFailure::PathUnreachable,
        ),
        (
            "task failed world changed",
            TASK_FAIL_WORLD_CHANGED,
            TaskFailure::WorldChanged,
        ),
        (
            "task failed inventory full",
            TASK_FAIL_INVENTORY_FULL,
            TaskFailure::InventoryFull,
        ),
    ] {
        push(
            label,
            CHAT_EVENT_TASK_FAILED,
            reason,
            COMPANION_ID,
            COMPANION_NAME,
            COMMAND,
            "",
            ChatBody::Task {
                companion: speaker(),
                command: command_text(),
                state: TaskState::Failed(failure),
            },
        );
    }
    push(
        "companion speech",
        CHAT_EVENT_COMPANION_SPEECH,
        CHAT_REJECT_NONE,
        COMPANION_ID,
        COMPANION_NAME,
        "",
        SPEECH,
        ChatBody::Speech {
            companion: speaker(),
            text: speech_text(),
        },
    );
    vectors
}

#[test]
fn chat_event_records_round_trip_through_the_fallible_surface() {
    for vector in chat_event_vectors() {
        let record = ChatEvent::new(vector.event.clone()).unwrap_or_else(|err| {
            panic!(
                "{}: constructor refuses a valid record: {err:?}",
                vector.label
            )
        });
        record.validate().unwrap_or_else(|err| {
            panic!("{}: valid record fails validation: {err:?}", vector.label)
        });
        let length = record
            .encoded_len()
            .unwrap_or_else(|err| panic!("{}: encoded_len fails: {err:?}", vector.label));
        assert_eq!(
            length,
            vector.payload.len(),
            "{}: encoded_len disagrees with the reviewed payload",
            vector.label
        );
        // The fixed strides are 8 event ID, 16 player identity, 16 companion
        // identity, one kind and one reason; the three text slots are
        // length-prefixed.
        let text = if vector.event.kind == CHAT_EVENT_COMPANION_SPEECH {
            &vector.event.speech
        } else {
            &vector.event.command
        };
        let expected = 8
            + 16
            + 1
            + PLAYER_NAME.len()
            + 16
            + 1
            + vector.event.companion_name.len()
            + 1
            + 1
            + 1
            + text.len();
        assert_eq!(
            length, expected,
            "{}: the stride decomposition disagrees",
            vector.label
        );

        let mut exact = vec![0u8; length];
        let written = record
            .encode_into(&mut exact)
            .unwrap_or_else(|err| panic!("{}: encode_into fails: {err:?}", vector.label));
        assert_eq!(
            written, length,
            "{}: encode_into reports a short write",
            vector.label
        );
        assert_eq!(
            exact, vector.payload,
            "{}: encode_into published unexpected bytes",
            vector.label
        );
        let allocated = record
            .encode()
            .unwrap_or_else(|err| panic!("{}: encode fails: {err:?}", vector.label));
        assert_eq!(
            allocated, vector.payload,
            "{}: encode disagrees with encode_into",
            vector.label
        );

        let mut padded = vec![SENTINEL; length + 3];
        let written = record
            .encode_into(&mut padded)
            .unwrap_or_else(|err| panic!("{}: padded encode_into fails: {err:?}", vector.label));
        assert_eq!(
            written, length,
            "{}: padded write reports a short length",
            vector.label
        );
        assert_eq!(
            &padded[..length],
            vector.payload.as_slice(),
            "{}: padded prefix differs from the reviewed payload",
            vector.label
        );
        assert!(
            padded[length..].iter().all(|byte| *byte == SENTINEL),
            "{}: padded write touched bytes beyond the record",
            vector.label
        );

        let decoded = ChatEvent::decode(&vector.payload)
            .unwrap_or_else(|err| panic!("{}: decode fails: {err:?}", vector.label));
        assert_eq!(
            decoded, record,
            "{}: decoded record differs from the encoded one",
            vector.label
        );
        assert_eq!(
            decoded
                .encode()
                .unwrap_or_else(|err| panic!("{}: re-encode fails: {err:?}", vector.label)),
            vector.payload,
            "{}: re-encoded payload differs from the reviewed bytes",
            vector.label
        );
    }
}

#[test]
fn chat_event_every_branch_shape_maps_to_the_closed_domain_union() {
    for vector in chat_event_vectors() {
        let record = ChatEvent::new(vector.event.clone()).unwrap_or_else(|err| {
            panic!(
                "{}: constructor refuses a valid record: {err:?}",
                vector.label
            )
        });
        let domain = mornlea_protocol::DomainChatEvent::try_from(record).unwrap_or_else(|err| {
            panic!(
                "{}: the domain conversion refuses it: {err:?}",
                vector.label
            )
        });
        assert_eq!(
            domain.body(),
            &vector.body,
            "{}: the domain body is not the reviewed union member",
            vector.label
        );
        assert_eq!(domain.event_id(), EVENT_ID);
        // The reverse conversion reproduces the reviewed record exactly, so the
        // two directions are one lossless pair.
        let back = ChatEvent::try_from(domain).unwrap_or_else(|err| {
            panic!(
                "{}: the reverse conversion refuses it: {err:?}",
                vector.label
            )
        });
        assert_eq!(
            back, vector.event,
            "{}: the reverse conversion rewrote the wire record",
            vector.label
        );
        assert_eq!(
            back.encode()
                .unwrap_or_else(|err| panic!("{}: re-encode fails: {err:?}", vector.label)),
            vector.payload,
            "{}: the reverse conversion does not re-encode to the reviewed bytes",
            vector.label
        );
    }
}

#[test]
fn chat_event_the_absent_companion_identity_is_wire_only_in_two_branches() {
    // The two rejection branches that never addressed a companion carry the
    // exact zero form on the wire, and the domain expresses that absence by
    // the variant shape rather than by a zero identity.
    let zero = [0u8; 16];
    let malformed = chat_event_vectors()
        .into_iter()
        .find(|vector| vector.label == "invalid format")
        .expect("the invalid-format vector");
    assert_eq!(malformed.event.companion_id, zero);
    assert!(malformed.event.companion_name.is_empty());
    assert!(malformed.event.command.is_empty());
    let unknown = chat_event_vectors()
        .into_iter()
        .find(|vector| vector.label == "unknown companion")
        .expect("the unknown-companion vector");
    assert_eq!(unknown.event.companion_id, zero);
    assert_eq!(unknown.event.companion_name, COMPANION_NAME);

    // Every other branch names a companion, so the zero form is refused there
    // by the value gate and never becomes a domain identity.
    for vector in chat_event_vectors() {
        if vector.label == "invalid format" || vector.label == "unknown companion" {
            continue;
        }
        let mut anonymous = vector.event.clone();
        anonymous.companion_id = zero;
        assert_eq!(
            anonymous.validate(),
            Err(ProtocolError::InvalidString),
            "{}: an absent companion identity is admitted",
            vector.label
        );
        assert!(
            mornlea_protocol::DomainChatEvent::try_from(anonymous).is_err(),
            "{}: an absent companion identity became a domain identity",
            vector.label
        );
    }
    // The two absent branches refuse a real identity, so the zero form is the
    // only accepted one there.
    for label in ["invalid format", "unknown companion"] {
        let mut named = chat_event_vectors()
            .into_iter()
            .find(|vector| vector.label == label)
            .expect("the absent branch vector");
        named.event.companion_id = COMPANION_ID;
        assert_eq!(
            named.event.validate(),
            Err(ProtocolError::InvalidString),
            "{label}: a named companion identity leaks into the absent branch"
        );
    }
}

#[test]
fn chat_event_the_text_slot_is_reused_by_kind_alone() {
    // The payload carries exactly one text slot, so a non-speech branch that
    // carries speech bytes and a speech branch that restates the command are
    // both refused before any byte is published. The wire literal is the
    // discriminator: the accepted branch's slot holds the command and the
    // speech branch's slot holds the line.
    let accepted = chat_event_vectors()
        .into_iter()
        .find(|vector| vector.label == "accepted")
        .expect("the accepted vector");
    let speech = chat_event_vectors()
        .into_iter()
        .find(|vector| vector.label == "companion speech")
        .expect("the speech vector");
    let text_offset = accepted.payload.len() - COMMAND.len() - 1;
    assert_eq!(accepted.payload[text_offset], COMMAND.len() as u8);
    assert_eq!(&accepted.payload[text_offset + 1..], COMMAND.as_bytes());
    let text_offset = speech.payload.len() - SPEECH.len() - 1;
    assert_eq!(speech.payload[text_offset], SPEECH.len() as u8);
    assert_eq!(&speech.payload[text_offset + 1..], SPEECH.as_bytes());

    let mut non_speech_with_line = accepted.event.clone();
    non_speech_with_line.speech = SPEECH.to_owned();
    assert_eq!(
        non_speech_with_line.validate(),
        Err(ProtocolError::InvalidString),
        "a non-speech branch carries a model-generated line"
    );
    let mut speech_with_command = speech.event.clone();
    speech_with_command.command = COMMAND.to_owned();
    assert_eq!(
        speech_with_command.validate(),
        Err(ProtocolError::InvalidString),
        "the speech branch restates the player command"
    );
}

#[test]
fn chat_event_decode_cuts_inside_the_name_and_text_regions_are_truncated() {
    // The payload is variable-length, so the sweep cuts inside the record
    // regions rather than at every prefix: the shared prefix, both name slots
    // and the text slot. Every cut reports `Truncated`, including the string
    // length prefixes the payload cannot complete, which is the boundary the
    // Go producer resolves through its own derivation base.
    let accepted = chat_event_vectors()
        .into_iter()
        .find(|vector| vector.label == "accepted")
        .expect("the accepted vector");
    for cut in 0..accepted.payload.len() {
        let error = ChatEvent::decode(&accepted.payload[..cut])
            .expect_err("a proper truncation must never decode");
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "truncation at {cut} bytes reports {error:?}"
        );
    }
    // The speech branch reuses the same slot with the tighter bound, so its own
    // cuts are swept too.
    let speech = chat_event_vectors()
        .into_iter()
        .find(|vector| vector.label == "companion speech")
        .expect("the speech vector");
    for cut in 0..speech.payload.len() {
        let error = ChatEvent::decode(&speech.payload[..cut])
            .expect_err("a proper truncation must never decode");
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "speech truncation at {cut} bytes reports {error:?}"
        );
    }
}

#[test]
fn chat_event_decode_rejects_one_trailing_byte() {
    let accepted = chat_event_vectors()
        .into_iter()
        .find(|vector| vector.label == "accepted")
        .expect("the accepted vector");
    let mut trailing = accepted.payload.clone();
    trailing.push(0x00);
    assert_eq!(
        ChatEvent::decode(&trailing),
        Err(ProtocolError::TrailingBytes)
    );
}

#[test]
fn chat_event_decode_refuses_the_wire_ceiling_before_any_parse() {
    // The Go decoder applies the fixed payload ceiling before it reads a single
    // field, so a payload one byte above it answers the capacity boundary. The
    // guard is a strict comparison, so a payload of exactly the ceiling length
    // is parsed and refused, if at all, by a field rule instead.
    let mut above = vec![0u8; CHAT_EVENT_MAX_WIRE_BYTES + 1];
    above[0] = EVENT_ID as u8;
    assert_eq!(
        ChatEvent::decode(&above),
        Err(ProtocolError::FrameTooLarge),
        "a payload above the ceiling is parsed instead of refused"
    );
    assert_ne!(
        ChatEvent::decode(&vec![0u8; CHAT_EVENT_MAX_WIRE_BYTES]),
        Err(ProtocolError::FrameTooLarge),
        "a payload of exactly the ceiling length is refused by the ceiling"
    );
    assert_eq!(CHAT_EVENT_MAX_WIRE_BYTES, 1328);
}

#[test]
fn chat_event_decode_admits_a_maximum_record_at_the_ceiling() {
    // The command slot decides the ceiling, and the widest admissible record
    // reaches it exactly: both names at their 128-byte bound (32 four-byte
    // runes) with their two-byte length prefixes, plus the maximum-length
    // command with its own two-byte prefix.
    let name = "\u{1f600}".repeat(32);
    assert_eq!(name.len(), 128);
    let event = record(
        EVENT_ID,
        PLAYER_ID,
        &name,
        COMPANION_ID,
        &name,
        CHAT_EVENT_ACCEPTED,
        CHAT_REJECT_NONE,
        &"a".repeat(1024),
        "",
    );
    let payload = event.encode().expect("the widest record encodes");
    assert_eq!(payload.len(), CHAT_EVENT_MAX_WIRE_BYTES);
    assert_eq!(ChatEvent::decode(&payload), Ok(event));
}

#[test]
fn chat_event_invalid_value_wins_over_short_capacity() {
    // Every mutation is a single violation, and every entry point reports the
    // same value error before any size or capacity decision. The unmutated
    // record sizes the destinations, so the capacity refusal and the value
    // refusal are compared on buffers of the same shape.
    let needed = ChatEvent::new(
        chat_event_vectors()
            .into_iter()
            .find(|vector| vector.label == "accepted")
            .expect("the accepted vector")
            .event,
    )
    .expect("the valid record")
    .encoded_len()
    .expect("the valid record sizes");

    let cases: Vec<(&str, ChatEvent, ProtocolError)> = vec![
        (
            "unknown kind after construction",
            record(
                EVENT_ID,
                PLAYER_ID,
                PLAYER_NAME,
                COMPANION_ID,
                COMPANION_NAME,
                200,
                CHAT_REJECT_NONE,
                COMMAND,
                "",
            ),
            ProtocolError::InvalidEnum,
        ),
        (
            "reserved reject reason three",
            record(
                EVENT_ID,
                PLAYER_ID,
                PLAYER_NAME,
                COMPANION_ID,
                COMPANION_NAME,
                CHAT_EVENT_REJECTED,
                3,
                "",
                "",
            ),
            ProtocolError::InvalidEnum,
        ),
        (
            "task failed reason zero",
            record(
                EVENT_ID,
                PLAYER_ID,
                PLAYER_NAME,
                COMPANION_ID,
                COMPANION_NAME,
                CHAT_EVENT_TASK_FAILED,
                0,
                COMMAND,
                "",
            ),
            ProtocolError::InvalidEnum,
        ),
        (
            "task failed reason twenty-one",
            record(
                EVENT_ID,
                PLAYER_ID,
                PLAYER_NAME,
                COMPANION_ID,
                COMPANION_NAME,
                CHAT_EVENT_TASK_FAILED,
                21,
                COMMAND,
                "",
            ),
            ProtocolError::InvalidEnum,
        ),
        (
            "zero event id",
            record(
                0,
                PLAYER_ID,
                PLAYER_NAME,
                COMPANION_ID,
                COMPANION_NAME,
                CHAT_EVENT_ACCEPTED,
                CHAT_REJECT_NONE,
                COMMAND,
                "",
            ),
            ProtocolError::InvalidIdentity,
        ),
        (
            "noncanonical player name",
            record(
                EVENT_ID,
                PLAYER_ID,
                " Alice",
                COMPANION_ID,
                COMPANION_NAME,
                CHAT_EVENT_ACCEPTED,
                CHAT_REJECT_NONE,
                COMMAND,
                "",
            ),
            ProtocolError::InvalidString,
        ),
        (
            "command above the byte bound",
            record(
                EVENT_ID,
                PLAYER_ID,
                PLAYER_NAME,
                COMPANION_ID,
                COMPANION_NAME,
                CHAT_EVENT_ACCEPTED,
                CHAT_REJECT_NONE,
                &"a".repeat(1025),
                "",
            ),
            ProtocolError::InvalidString,
        ),
        (
            "speech above the byte bound",
            record(
                EVENT_ID,
                PLAYER_ID,
                PLAYER_NAME,
                COMPANION_ID,
                COMPANION_NAME,
                CHAT_EVENT_COMPANION_SPEECH,
                CHAT_REJECT_NONE,
                "",
                &"a".repeat(CHAT_SPEECH_TEXT_MAX_BYTES + 1),
            ),
            ProtocolError::InvalidString,
        ),
        (
            "non-speech branch carries a line",
            record(
                EVENT_ID,
                PLAYER_ID,
                PLAYER_NAME,
                COMPANION_ID,
                COMPANION_NAME,
                CHAT_EVENT_TASK_STARTED,
                CHAT_REJECT_NONE,
                COMMAND,
                SPEECH,
            ),
            ProtocolError::InvalidString,
        ),
        (
            "speech branch restates the command",
            record(
                EVENT_ID,
                PLAYER_ID,
                PLAYER_NAME,
                COMPANION_ID,
                COMPANION_NAME,
                CHAT_EVENT_COMPANION_SPEECH,
                CHAT_REJECT_NONE,
                COMMAND,
                SPEECH,
            ),
            ProtocolError::InvalidString,
        ),
    ];

    for (label, mutation, error) in cases {
        assert_eq!(
            mutation.validate(),
            Err(error),
            "{label}: validate disagrees with the mutation"
        );
        assert_eq!(
            mutation.encoded_len(),
            Err(error),
            "{label}: encoded_len disagrees with the mutation"
        );
        assert_eq!(
            mutation.encode(),
            Err(error),
            "{label}: encode disagrees with the mutation"
        );

        let mut short = vec![SENTINEL; needed - 1];
        assert_eq!(
            mutation.encode_into(&mut short),
            Err(error),
            "{label}: a short destination masks the value error"
        );
        assert!(
            short.iter().all(|byte| *byte == SENTINEL),
            "{label}: a short destination was modified on a value refusal"
        );

        let mut exact = vec![SENTINEL; needed];
        assert_eq!(
            mutation.encode_into(&mut exact),
            Err(error),
            "{label}: an exact destination masks the value error"
        );
        assert!(
            exact.iter().all(|byte| *byte == SENTINEL),
            "{label}: an exact destination was modified on a value refusal"
        );
    }
}

#[test]
fn chat_event_encode_into_leaves_a_short_destination_unchanged() {
    let event = chat_event_vectors()
        .into_iter()
        .find(|vector| vector.label == "accepted")
        .expect("the accepted vector")
        .event;
    let needed = event.encoded_len().expect("the valid record sizes");
    for available in [0usize, needed - 1, needed - 3] {
        let mut dst = vec![SENTINEL; available];
        assert_eq!(
            event.encode_into(&mut dst),
            Err(ProtocolError::OutputTooSmall { needed, available }),
            "short destination of {available} bytes reports the wrong error"
        );
        assert!(
            dst.iter().all(|byte| *byte == SENTINEL),
            "short destination of {available} bytes was modified"
        );
    }
}

#[test]
fn chat_event_decode_rejects_the_pinned_invalid_values() {
    // The matrix mirrors the corpus negatives. Every entry is a single
    // violation, and the boundary each rejection reports is the one the Go
    // producer records for the same bytes.
    let mut vectors = chat_event_vectors().into_iter();
    let accepted = vectors.next().expect("the accepted vector").payload.clone();
    let mut cases: Vec<(&str, Vec<u8>, ProtocolError)> = vec![
        (
            "one trailing byte",
            {
                let mut trailing = accepted.clone();
                trailing.push(0x00);
                trailing
            },
            ProtocolError::TrailingBytes,
        ),
        (
            "declared player name length the payload cannot complete",
            { accepted[..24].to_vec() },
            ProtocolError::Truncated,
        ),
    ];
    // An unknown kind is refused by the same validation the Go decoder applies
    // after the last field, before the text slot is interpreted.
    let mut unknown_kind = accepted.clone();
    let kind_offset = 8 + 16 + 1 + PLAYER_NAME.len() + 16 + 1 + COMPANION_NAME.len();
    unknown_kind[kind_offset] = 10;
    cases.push(("unknown kind", unknown_kind, ProtocolError::InvalidEnum));

    for (label, payload, error) in cases {
        assert_eq!(
            ChatEvent::decode(&payload),
            Err(error),
            "{label}: the decoder reports a different boundary"
        );
    }
}

#[test]
fn chat_event_packet_id_is_pinned() {
    assert_eq!(ChatEvent::PACKET_ID, 16);
}

#[test]
fn chat_event_the_two_text_bounds_are_distinct() {
    // The speech slot is tighter than the command slot, and the wire ceiling is
    // the wider command slot plus its maximum two-byte length prefix.
    assert_eq!(CHAT_SPEECH_TEXT_MAX_BYTES, 256);
    assert_eq!(
        CHAT_EVENT_MAX_WIRE_BYTES,
        8 + 16 + 130 + 16 + 130 + 1 + 1 + 1024 + 2
    );
}
