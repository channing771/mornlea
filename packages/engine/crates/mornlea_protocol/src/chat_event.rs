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

use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::chat_command::{CHAT_COMMAND_TEXT_MAX_BYTES, valid_bounded_text, valid_command_text};
use crate::entity_id::{CompanionId, valid_companion_name, valid_display_name};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};

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

    /// The single validation gate shared by `new` and `decode`.
    ///
    /// The speech field belongs to the companion speech kind alone, so any
    /// other kind carrying a line is rejected before its own combination is
    /// examined. A queue-full or not-following rejection keeps the same
    /// identity and command requirements as an acceptance, because the player
    /// must be able to match the rejection to the command that caused it.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        if self.event_id == 0 || !valid_display_name(&self.player_name) {
            return Err(ProtocolError::InvalidIdentity);
        }
        if self.kind != CHAT_EVENT_COMPANION_SPEECH && !self.speech.is_empty() {
            return Err(ProtocolError::InvalidRange);
        }
        match self.kind {
            CHAT_EVENT_ACCEPTED => {
                if self.reject_reason != CHAT_REJECT_NONE
                    || !names_companion(&self.companion_id)
                    || !valid_companion_name(&self.companion_name)
                    || !valid_command_text(&self.command)
                {
                    return Err(ProtocolError::InvalidEnum);
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
                        return Err(ProtocolError::InvalidRange);
                    }
                }
                CHAT_REJECT_UNKNOWN_COMPANION => {
                    // Only a legal target name survives, so the player can
                    // check the spelling.
                    if self.companion_id != [0; 16]
                        || !self.command.is_empty()
                        || !valid_companion_name(&self.companion_name)
                    {
                        return Err(ProtocolError::InvalidRange);
                    }
                }
                CHAT_REJECT_QUEUE_FULL | CHAT_REJECT_NOT_FOLLOWING => {
                    if !names_companion(&self.companion_id)
                        || !valid_companion_name(&self.companion_name)
                        || !valid_command_text(&self.command)
                    {
                        return Err(ProtocolError::InvalidEnum);
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
                    return Err(ProtocolError::InvalidEnum);
                }
            }
            CHAT_EVENT_TASK_FAILED => {
                if !valid_task_fail_reason(self.reject_reason)
                    || !names_companion(&self.companion_id)
                    || !valid_companion_name(&self.companion_name)
                    || !valid_command_text(&self.command)
                {
                    return Err(ProtocolError::InvalidEnum);
                }
            }
            CHAT_EVENT_COMPANION_SPEECH => {
                if self.reject_reason != CHAT_REJECT_NONE
                    || !self.command.is_empty()
                    || !names_companion(&self.companion_id)
                    || !valid_companion_name(&self.companion_name)
                    || !valid_bounded_text(&self.speech, CHAT_SPEECH_TEXT_MAX_BYTES)
                {
                    return Err(ProtocolError::InvalidEnum);
                }
            }
            _ => return Err(ProtocolError::InvalidEnum),
        }
        Ok(())
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.event_id);
        encoder.bytes(&self.player_id.bytes());
        encoder.string(&self.player_name, PLAYER_NAME_MAX_BYTES);
        encoder.bytes(&self.companion_id);
        encoder.string(&self.companion_name, COMPANION_NAME_MAX_BYTES);
        encoder.u8(self.kind);
        encoder.u8(self.reject_reason);
        if self.kind == CHAT_EVENT_COMPANION_SPEECH {
            encoder.string(&self.speech, CHAT_SPEECH_TEXT_MAX_BYTES);
        } else {
            encoder.string(&self.command, CHAT_COMMAND_TEXT_MAX_BYTES);
        }
        encoder.finish().expect("validated chat event is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        if payload.len() > CHAT_EVENT_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let event_id = decoder.u64()?;
        let player_id = player_id::read(&mut decoder)?;
        let player_name = decoder.string(PLAYER_NAME_MAX_BYTES, PLAYER_NAME_MAX_RUNES)?;
        // The companion identity is read as raw wire bytes: a rejection that
        // never addressed a companion carries the absent zero form, and the
        // kind-specific validation decides whether that form is acceptable.
        // The domain companion identity has no zero member, so the raw form
        // never becomes a domain identity here.
        let companion_id = decoder.bytes::<16>()?;
        let companion_name = decoder.string(COMPANION_NAME_MAX_BYTES, COMPANION_NAME_MAX_RUNES)?;
        let kind = decoder.u8()?;
        let reject_reason = decoder.u8()?;
        let (command, speech) = if kind == CHAT_EVENT_COMPANION_SPEECH {
            (
                String::new(),
                decoder.string(CHAT_SPEECH_TEXT_MAX_BYTES, CHAT_SPEECH_TEXT_MAX_BYTES)?,
            )
        } else {
            (
                decoder.string(CHAT_COMMAND_TEXT_MAX_BYTES, CHAT_COMMAND_TEXT_MAX_BYTES)?,
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
}
