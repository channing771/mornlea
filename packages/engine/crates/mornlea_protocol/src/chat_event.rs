//! The chat event payload that confirms a chat addressing outcome.
//!
//! The event is the only message whose text slot is reused by kind: a
//! companion speech event carries a model-generated line, and every other kind
//! restates the player's original command. The two slots share one wire slot
//! with different bounds, so the decoder reads the kind first and then decides
//! which text to read. The combination rules are atomic: any field that does
//! not belong to the current kind and reason rejects the whole event before
//! any field is applied, matching the Go decision that a partially valid event
//! is worse than a rejected one.
//!
//! The record carries the common fallible surface (`validate` → checked
//! `encoded_len` → capacity check → private `publish_packet`), so a public
//! field mutated into an illegal combination after construction is refused
//! instead of silently published, and a short destination is left byte for
//! byte unchanged. The value gate's error variants are the ones the frozen
//! corpus categories are resolved from: a zero event identity or an invalid
//! player identity is `InvalidIdentity`, an unknown kind or an unknown reason
//! is `InvalidEnum`, and every text boundary and illegal cross-field
//! combination is `InvalidString`.
//!
//! The bidirectional domain conversion closes the seam between the wire record
//! and the closed semantic union: the checked `mornlea_domain::ChatEvent`
//! owns the seven `ChatBody` variants that unfold into the sixteen legal
//! branch shapes, so a combination the Go validator rejects has no
//! constructible domain state either. The raw zero companion identity is a
//! wire-only absent form the two permitted rejection branches carry; the
//! domain expresses that absence by the variant shape, and no other branch
//! admits it.

use crate::bytes::{ByteDecoder, SliceWriter};
use crate::chat_command::{
    CHAT_COMMAND_TEXT_MAX_BYTES, read_bounded_text, valid_bounded_text, valid_command_text,
};
use crate::entity_id::{CompanionId, valid_companion_name, valid_display_name};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;
use mornlea_domain::{
    ChatBody, ChatEvent as DomainChatEvent, ChatEventParts, CommandText, CompanionName,
    CompanionSpeaker, DisplayName, SpeechText, TaskFailure, TaskState,
};

/// Companion display-name byte ceiling shared with the other companion
/// name-carrying families.
const COMPANION_NAME_MAX_BYTES: usize = 128;

/// Companion display-name rune ceiling shared with the other companion
/// name-carrying families.
const COMPANION_NAME_MAX_RUNES: usize = 32;

/// Player display-name byte ceiling.
const PLAYER_NAME_MAX_BYTES: usize = 128;

/// Player display-name rune ceiling.
const PLAYER_NAME_MAX_RUNES: usize = 32;

/// Maximum UTF-8 byte length of a companion speech line. The line is a
/// model-generated bounded expression, not a player input channel, so its
/// bound is tighter than the command bound.
pub const CHAT_SPEECH_TEXT_MAX_BYTES: usize = 256;

/// Fixed wire upper bound of the whole payload, derived from the Go
/// `ChatEventMaxWireBytes`: the command slot decides the bound because it is
/// the wider of the two text slots.
pub const CHAT_EVENT_MAX_WIRE_BYTES: usize =
    8 + 16 + 130 + 16 + 130 + 1 + 1 + CHAT_COMMAND_TEXT_MAX_BYTES + 2;

/// Chat event kinds. The values are appended in order, so an existing kind
/// keeps its wire number.
pub const CHAT_EVENT_ACCEPTED: u8 = 1;
pub const CHAT_EVENT_REJECTED: u8 = 2;
pub const CHAT_EVENT_TASK_STARTED: u8 = 3;
pub const CHAT_EVENT_TASK_PROGRESS: u8 = 4;
pub const CHAT_EVENT_TASK_COMPLETED: u8 = 5;
pub const CHAT_EVENT_TASK_FAILED: u8 = 6;
pub const CHAT_EVENT_TASK_TIMED_OUT: u8 = 7;
pub const CHAT_EVENT_TASK_STOPPED: u8 = 8;
pub const CHAT_EVENT_COMPANION_SPEECH: u8 = 9;

/// Chat addressing rejection reasons. Reason 3 is reserved and unassigned.
pub const CHAT_REJECT_NONE: u8 = 0;
pub const CHAT_REJECT_INVALID_FORMAT: u8 = 1;
pub const CHAT_REJECT_UNKNOWN_COMPANION: u8 = 2;
pub const CHAT_REJECT_QUEUE_FULL: u8 = 4;
pub const CHAT_REJECT_NOT_FOLLOWING: u8 = 5;

/// Task failure reasons, carried in the reason slot of a failed-task event
/// only. They start at 16 so they cannot collide with the rejection reasons.
pub const TASK_FAIL_PLANNER_UNAVAILABLE: u8 = 16;
pub const TASK_FAIL_INVALID_PLAN: u8 = 17;
pub const TASK_FAIL_PATH_UNREACHABLE: u8 = 18;
pub const TASK_FAIL_WORLD_CHANGED: u8 = 19;
pub const TASK_FAIL_INVENTORY_FULL: u8 = 20;

/// Reports whether a reason value belongs to the task failure enum. The
/// rejection range and every other out-of-range value are rejected.
fn valid_task_fail_reason(reason: u8) -> bool {
    (TASK_FAIL_PLANNER_UNAVAILABLE..=TASK_FAIL_INVENTORY_FULL).contains(&reason)
}

/// Reports whether the raw wire identity names a companion that is present
/// and well formed, as opposed to the absent form a never-addressed event
/// carries. The absent form is the exact zero bytes; the domain companion
/// identity has no zero member, so this predicate is the only place the raw
/// form is interpreted and it never becomes a domain identity.
fn names_companion(id: &[u8; 16]) -> bool {
    *id != [0; 16] && CompanionId::try_from_bytes(*id).is_ok()
}

/// Play ChatEvent payload: the confirmed outcome of one chat addressing.
///
/// The companion identity is the raw 16 wire bytes rather than a checked
/// identity: the two rejection branches that never addressed a companion
/// carry the exact zero form, and the domain deliberately publishes no zero
/// companion identity. `names_companion` interprets the raw form, so absence
/// stays a wire-only representation.
#[derive(Clone, Debug, PartialEq)]
pub struct ChatEvent {
    pub event_id: u64,
    pub player_id: PlayerId,
    pub player_name: String,
    pub companion_id: [u8; 16],
    pub companion_name: String,
    pub kind: u8,
    pub reject_reason: u8,
    pub command: String,
    pub speech: String,
}

impl ChatEvent {
    pub const PACKET_ID: u32 = 16;

    /// Builds a validated chat event. The player identity is validated by
    /// `PlayerId` itself, so only the outcome fields are checked here.
    pub fn new(event: ChatEvent) -> Result<Self, ProtocolError> {
        event.validate()?;
        Ok(event)
    }

    /// The single validation gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The speech field belongs to the companion speech kind alone, so any
    /// other kind carrying a line is rejected before its own combination is
    /// examined. A queue-full or not-following rejection keeps the same
    /// identity and command requirements as an acceptance, because the player
    /// must be able to match the rejection to the command that caused it.
    ///
    /// The gate's error variants carry the frozen corpus category: the global
    /// identity gate reports `InvalidIdentity` for a zero event identity and
    /// an invalid player identity, and the player name rule reports
    /// `InvalidString` because a name boundary is a text boundary. An unknown
    /// kind and an unknown reason are `InvalidEnum`, and every remaining
    /// combination failure — including the two absent-branch leaks — is
    /// `InvalidString`.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        if self.event_id == 0 {
            return Err(ProtocolError::InvalidIdentity);
        }
        // The identity type is already the rule, and the decode-side reader
        // applies it before a record exists, so this recheck only guards the
        // public field against a future change of that type.
        if PlayerId::try_from_bytes(self.player_id.bytes()).is_err() {
            return Err(ProtocolError::InvalidIdentity);
        }
        if !valid_display_name(&self.player_name) {
            return Err(ProtocolError::InvalidString);
        }
        if self.kind != CHAT_EVENT_COMPANION_SPEECH && !self.speech.is_empty() {
            return Err(ProtocolError::InvalidString);
        }
        match self.kind {
            CHAT_EVENT_ACCEPTED => {
                if self.reject_reason != CHAT_REJECT_NONE
                    || !names_companion(&self.companion_id)
                    || !valid_companion_name(&self.companion_name)
                    || !valid_command_text(&self.command)
                {
                    return Err(ProtocolError::InvalidString);
                }
            }
            CHAT_EVENT_REJECTED => match self.reject_reason {
                CHAT_REJECT_INVALID_FORMAT => {
                    // A malformed command never addressed a companion, so no
                    // identity, name, or command may survive.
                    if self.companion_id != [0; 16]
                        || !self.command.is_empty()
                        || !self.companion_name.is_empty()
                    {
                        return Err(ProtocolError::InvalidString);
                    }
                }
                CHAT_REJECT_UNKNOWN_COMPANION => {
                    // Only a legal target name survives, so the player can
                    // check the spelling.
                    if self.companion_id != [0; 16]
                        || !self.command.is_empty()
                        || !valid_companion_name(&self.companion_name)
                    {
                        return Err(ProtocolError::InvalidString);
                    }
                }
                CHAT_REJECT_QUEUE_FULL | CHAT_REJECT_NOT_FOLLOWING => {
                    if !names_companion(&self.companion_id)
                        || !valid_companion_name(&self.companion_name)
                        || !valid_command_text(&self.command)
                    {
                        return Err(ProtocolError::InvalidString);
                    }
                }
                _ => return Err(ProtocolError::InvalidEnum),
            },
            CHAT_EVENT_TASK_STARTED
            | CHAT_EVENT_TASK_PROGRESS
            | CHAT_EVENT_TASK_COMPLETED
            | CHAT_EVENT_TASK_TIMED_OUT
            | CHAT_EVENT_TASK_STOPPED => {
                // Task progress events restate the original command and keep
                // the reason slot empty.
                if self.reject_reason != CHAT_REJECT_NONE
                    || !names_companion(&self.companion_id)
                    || !valid_companion_name(&self.companion_name)
                    || !valid_command_text(&self.command)
                {
                    return Err(ProtocolError::InvalidString);
                }
            }
            CHAT_EVENT_TASK_FAILED => {
                // The reason slot carries a task failure reason here and a
                // rejection reason nowhere else, so the reason enum is checked
                // first and its refusal is the enum boundary rather than a
                // combination failure.
                if !valid_task_fail_reason(self.reject_reason) {
                    return Err(ProtocolError::InvalidEnum);
                }
                if !names_companion(&self.companion_id)
                    || !valid_companion_name(&self.companion_name)
                    || !valid_command_text(&self.command)
                {
                    return Err(ProtocolError::InvalidString);
                }
            }
            CHAT_EVENT_COMPANION_SPEECH => {
                if self.reject_reason != CHAT_REJECT_NONE
                    || !self.command.is_empty()
                    || !names_companion(&self.companion_id)
                    || !valid_companion_name(&self.companion_name)
                    || !valid_bounded_text(&self.speech, CHAT_SPEECH_TEXT_MAX_BYTES)
                {
                    return Err(ProtocolError::InvalidString);
                }
            }
            _ => return Err(ProtocolError::InvalidEnum),
        }
        Ok(())
    }

    /// The exact encoded length: the fixed fields, the three length-prefixed
    /// text slots and the one kind-selected text slot.
    ///
    /// The value gate runs first, so an invalid record reports its value error
    /// here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.validate()?;
        let text = self.text_slot();
        let mut length: usize = 8 + 16 + 16 + 1 + 1;
        length = length
            .checked_add(prefix_len(self.player_name.len()))
            .and_then(|sum| sum.checked_add(self.player_name.len()))
            .and_then(|sum| sum.checked_add(prefix_len(self.companion_name.len())))
            .and_then(|sum| sum.checked_add(self.companion_name.len()))
            .and_then(|sum| sum.checked_add(prefix_len(text.len())))
            .and_then(|sum| sum.checked_add(text.len()))
            .ok_or(ProtocolError::Allocation)?;
        Ok(length)
    }
    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the event identity, the player
    /// identity and name, the companion identity and name, the kind, the
    /// reason, and the one text slot the kind selects — and the destination is
    /// tested before the first byte is written, so a short or invalid call
    /// leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.event_id);
            writer.bytes(&self.player_id.bytes());
            writer.uvarint(prefix_value(self.player_name.len()));
            writer.bytes(self.player_name.as_bytes());
            writer.bytes(&self.companion_id);
            writer.uvarint(prefix_value(self.companion_name.len()));
            writer.bytes(self.companion_name.as_bytes());
            writer.u8(self.kind);
            writer.u8(self.reject_reason);
            writer.uvarint(prefix_value(self.text_slot().len()));
            writer.bytes(self.text_slot().as_bytes());
        })
    }

    /// The allocating compatibility wrapper.
    ///
    /// It reserves exactly the validated length and publishes through
    /// `encode_into`, so the two entry points always agree byte for byte.
    pub fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        let length = self.encoded_len()?;
        let mut wire = vec![0u8; length];
        let written = self.encode_into(&mut wire)?;
        wire.truncate(written);
        Ok(wire)
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        // The Go decoder applies the payload ceiling before it reads a single
        // field, so an oversized payload reports the ceiling refusal rather
        // than a field error.
        if payload.len() > CHAT_EVENT_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let mut decoder = ByteDecoder::new(payload);
        let event_id = decoder.u64()?;
        let player_id = player_id::read(&mut decoder)?;
        let player_name =
            read_bounded_text(&mut decoder, PLAYER_NAME_MAX_BYTES, PLAYER_NAME_MAX_RUNES)?;
        // The companion identity is read as raw wire bytes: a rejection that
        // never addressed a companion carries the absent zero form, and the
        // kind-specific validation decides whether that form is acceptable.
        // The domain companion identity has no zero member, so the raw form
        // never becomes a domain identity here.
        let companion_id = decoder.bytes::<16>()?;
        let companion_name = read_bounded_text(
            &mut decoder,
            COMPANION_NAME_MAX_BYTES,
            COMPANION_NAME_MAX_RUNES,
        )?;
        let kind = decoder.u8()?;
        let reject_reason = decoder.u8()?;
        let (command, speech) = if kind == CHAT_EVENT_COMPANION_SPEECH {
            (
                String::new(),
                read_bounded_text(
                    &mut decoder,
                    CHAT_SPEECH_TEXT_MAX_BYTES,
                    CHAT_SPEECH_TEXT_MAX_BYTES,
                )?,
            )
        } else {
            (
                read_bounded_text(
                    &mut decoder,
                    CHAT_COMMAND_TEXT_MAX_BYTES,
                    CHAT_COMMAND_TEXT_MAX_BYTES,
                )?,
                String::new(),
            )
        };
        decoder.done()?;
        Self::new(ChatEvent {
            event_id,
            player_id,
            player_name,
            companion_id,
            companion_name,
            kind,
            reject_reason,
            command,
            speech,
        })
    }

    /// The one text slot the kind selects.
    ///
    /// The wire carries exactly one slot, so the encoder and the size
    /// computation read the same one the decoder wrote into.
    fn text_slot(&self) -> &str {
        if self.kind == CHAT_EVENT_COMPANION_SPEECH {
            &self.speech
        } else {
            &self.command
        }
    }
}

/// The canonical uvarint length prefix one text slot carries.
///
/// The value gate has already admitted the slot, so the length is inside the
/// slot's byte bound and the conversion cannot fail.
fn prefix_value(length: usize) -> u32 {
    u32::try_from(length).expect("validated chat event text fits its length prefix")
}

/// The exact encoded length of one canonical uvarint length prefix.
fn prefix_len(length: usize) -> usize {
    canonical_uvarint_length(u32::try_from(length).unwrap_or(u32::MAX))
}

/// Builds the checked companion speaker every companion-bearing branch
/// carries.
///
/// The value gate has already admitted the identity and the name, so the
/// checked constructors cannot fail here; a failure is therefore a crate bug
/// rather than a record the authority could have published.
fn companion_speaker(id: &[u8; 16], name: String) -> Result<CompanionSpeaker, ProtocolError> {
    let identity = CompanionId::try_from_bytes(*id).map_err(|_| ProtocolError::InvalidIdentity)?;
    let name = CompanionName::try_from_canonical(name).map_err(|_| ProtocolError::InvalidString)?;
    Ok(CompanionSpeaker::new(identity, name))
}

/// Wraps the restated command through the domain rule the gate already
/// applied.
fn command_text(command: String) -> Result<CommandText, ProtocolError> {
    CommandText::try_from_canonical(command).map_err(|_| ProtocolError::InvalidString)
}

/// Wraps the model-generated line through the domain rule the gate already
/// applied.
fn speech_text(speech: String) -> Result<SpeechText, ProtocolError> {
    SpeechText::try_from_canonical(speech).map_err(|_| ProtocolError::InvalidString)
}

/// Maps one task kind, with the failed kind's reason, to the closed lifecycle
/// state, or reports that the kind is not a task fact.
fn task_state(kind: u8, reason: u8) -> Option<TaskState> {
    Some(match kind {
        CHAT_EVENT_TASK_STARTED => TaskState::Started,
        CHAT_EVENT_TASK_PROGRESS => TaskState::Progress,
        CHAT_EVENT_TASK_COMPLETED => TaskState::Completed,
        CHAT_EVENT_TASK_TIMED_OUT => TaskState::TimedOut,
        CHAT_EVENT_TASK_STOPPED => TaskState::Stopped,
        CHAT_EVENT_TASK_FAILED => TaskState::Failed(task_failure(reason)?),
        _ => return None,
    })
}

/// Maps one reason value to the closed failure reason, or reports that the
/// value is outside the published enum.
fn task_failure(reason: u8) -> Option<TaskFailure> {
    Some(match reason {
        TASK_FAIL_PLANNER_UNAVAILABLE => TaskFailure::PlannerUnavailable,
        TASK_FAIL_INVALID_PLAN => TaskFailure::InvalidPlan,
        TASK_FAIL_PATH_UNREACHABLE => TaskFailure::PathUnreachable,
        TASK_FAIL_WORLD_CHANGED => TaskFailure::WorldChanged,
        TASK_FAIL_INVENTORY_FULL => TaskFailure::InventoryFull,
        _ => return None,
    })
}

/// Maps one closed lifecycle state back to its kind and reason slot.
fn task_kind_and_reason(state: &TaskState) -> (u8, u8) {
    match state {
        TaskState::Started => (CHAT_EVENT_TASK_STARTED, CHAT_REJECT_NONE),
        TaskState::Progress => (CHAT_EVENT_TASK_PROGRESS, CHAT_REJECT_NONE),
        TaskState::Completed => (CHAT_EVENT_TASK_COMPLETED, CHAT_REJECT_NONE),
        TaskState::TimedOut => (CHAT_EVENT_TASK_TIMED_OUT, CHAT_REJECT_NONE),
        TaskState::Stopped => (CHAT_EVENT_TASK_STOPPED, CHAT_REJECT_NONE),
        TaskState::Failed(failure) => {
            let reason = match failure {
                TaskFailure::PlannerUnavailable => TASK_FAIL_PLANNER_UNAVAILABLE,
                TaskFailure::InvalidPlan => TASK_FAIL_INVALID_PLAN,
                TaskFailure::PathUnreachable => TASK_FAIL_PATH_UNREACHABLE,
                TaskFailure::WorldChanged => TASK_FAIL_WORLD_CHANGED,
                TaskFailure::InventoryFull => TASK_FAIL_INVENTORY_FULL,
            };
            (CHAT_EVENT_TASK_FAILED, reason)
        }
    }
}

/// Converts one admitted wire record into the closed domain chat event.
///
/// The value gate runs first, so every combination the Go validator rejects
/// is refused before any domain constructor runs and the checked domain
/// values cannot fail afterwards. The raw zero companion identity never
/// reaches this conversion as a present identity: the two branches that carry
/// it map to variant-shaped absence, and the gate refuses it everywhere else.
impl TryFrom<ChatEvent> for DomainChatEvent {
    type Error = ProtocolError;

    fn try_from(value: ChatEvent) -> Result<Self, Self::Error> {
        value.validate()?;
        let body = match value.kind {
            CHAT_EVENT_ACCEPTED => ChatBody::Accepted {
                companion: companion_speaker(&value.companion_id, value.companion_name)?,
                command: command_text(value.command)?,
            },
            CHAT_EVENT_REJECTED => match value.reject_reason {
                CHAT_REJECT_INVALID_FORMAT => ChatBody::InvalidFormat,
                CHAT_REJECT_UNKNOWN_COMPANION => ChatBody::UnknownCompanion {
                    name: CompanionName::try_from_canonical(value.companion_name)
                        .map_err(|_| ProtocolError::InvalidString)?,
                },
                CHAT_REJECT_QUEUE_FULL => ChatBody::QueueFull {
                    companion: companion_speaker(&value.companion_id, value.companion_name)?,
                    command: command_text(value.command)?,
                },
                CHAT_REJECT_NOT_FOLLOWING => ChatBody::NotFollowing {
                    companion: companion_speaker(&value.companion_id, value.companion_name)?,
                    command: command_text(value.command)?,
                },
                _ => return Err(ProtocolError::InvalidEnum),
            },
            CHAT_EVENT_TASK_STARTED
            | CHAT_EVENT_TASK_PROGRESS
            | CHAT_EVENT_TASK_COMPLETED
            | CHAT_EVENT_TASK_TIMED_OUT
            | CHAT_EVENT_TASK_STOPPED
            | CHAT_EVENT_TASK_FAILED => {
                let state = task_state(value.kind, value.reject_reason)
                    .ok_or(ProtocolError::InvalidEnum)?;
                ChatBody::Task {
                    companion: companion_speaker(&value.companion_id, value.companion_name)?,
                    command: command_text(value.command)?,
                    state,
                }
            }
            CHAT_EVENT_COMPANION_SPEECH => ChatBody::Speech {
                companion: companion_speaker(&value.companion_id, value.companion_name)?,
                text: speech_text(value.speech)?,
            },
            _ => return Err(ProtocolError::InvalidEnum),
        };
        let player_name = DisplayName::try_from_canonical(value.player_name)
            .map_err(|_| ProtocolError::InvalidString)?;
        DomainChatEvent::try_new(ChatEventParts {
            event_id: value.event_id,
            player_id: value.player_id,
            player_name,
            body,
        })
        .map_err(|_| ProtocolError::InvalidIdentity)
    }
}

/// Converts one closed domain chat event back into its wire record.
///
/// The conversion is total — every part is already a checked domain value, so
/// no combination the gate would refuse can be assembled — but it keeps the
/// fallible `TryFrom` shape and re-runs the gate, so the produced record is
/// admitted by the same validation an independently decoded one faces. The two
/// absent branches publish the exact zero companion identity, which is the one
/// wire form the domain cannot express as an identity.
impl TryFrom<DomainChatEvent> for ChatEvent {
    type Error = ProtocolError;

    fn try_from(event: DomainChatEvent) -> Result<Self, Self::Error> {
        let (kind, reject_reason, companion_id, companion_name, command, speech) =
            match event.body() {
                ChatBody::Accepted { companion, command } => (
                    CHAT_EVENT_ACCEPTED,
                    CHAT_REJECT_NONE,
                    companion.id().bytes(),
                    companion.name().as_str().to_owned(),
                    command.as_str().to_owned(),
                    String::new(),
                ),
                ChatBody::InvalidFormat => (
                    CHAT_EVENT_REJECTED,
                    CHAT_REJECT_INVALID_FORMAT,
                    [0u8; 16],
                    String::new(),
                    String::new(),
                    String::new(),
                ),
                ChatBody::UnknownCompanion { name } => (
                    CHAT_EVENT_REJECTED,
                    CHAT_REJECT_UNKNOWN_COMPANION,
                    [0u8; 16],
                    name.as_str().to_owned(),
                    String::new(),
                    String::new(),
                ),
                ChatBody::QueueFull { companion, command } => (
                    CHAT_EVENT_REJECTED,
                    CHAT_REJECT_QUEUE_FULL,
                    companion.id().bytes(),
                    companion.name().as_str().to_owned(),
                    command.as_str().to_owned(),
                    String::new(),
                ),
                ChatBody::NotFollowing { companion, command } => (
                    CHAT_EVENT_REJECTED,
                    CHAT_REJECT_NOT_FOLLOWING,
                    companion.id().bytes(),
                    companion.name().as_str().to_owned(),
                    command.as_str().to_owned(),
                    String::new(),
                ),
                ChatBody::Task {
                    companion,
                    command,
                    state,
                } => {
                    let (kind, reason) = task_kind_and_reason(state);
                    (
                        kind,
                        reason,
                        companion.id().bytes(),
                        companion.name().as_str().to_owned(),
                        command.as_str().to_owned(),
                        String::new(),
                    )
                }
                ChatBody::Speech { companion, text } => (
                    CHAT_EVENT_COMPANION_SPEECH,
                    CHAT_REJECT_NONE,
                    companion.id().bytes(),
                    companion.name().as_str().to_owned(),
                    String::new(),
                    text.as_str().to_owned(),
                ),
            };
        let record = ChatEvent {
            event_id: event.event_id(),
            player_id: event.player_id(),
            player_name: event.player_name().as_str().to_owned(),
            companion_id,
            companion_name,
            kind,
            reject_reason,
            command,
            speech,
        };
        record.validate()?;
        Ok(record)
    }
}
