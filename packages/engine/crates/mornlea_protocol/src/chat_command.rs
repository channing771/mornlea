use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Maximum UTF-8 byte length of a chat command text, shared with the
/// planner instruction limit so the two bounds cannot drift apart.
pub const CHAT_COMMAND_TEXT_MAX_BYTES: usize = 1024;

/// Reports whether a bounded text slot is publishable: at least one byte, at
/// most `max_bytes` bytes, no surrounding whitespace, and no control
/// character. The rule is the Go `validateCommandText` and
/// `validateSpeechText` shape with the bound supplied by the caller, so the
/// two text slots cannot drift apart.
pub(crate) fn valid_bounded_text(text: &str, max_bytes: usize) -> bool {
    !text.is_empty()
        && text.len() <= max_bytes
        && text.trim() == text
        && !text.chars().any(char::is_control)
}

/// Reports whether a chat command text slot is publishable.
pub(crate) fn valid_command_text(text: &str) -> bool {
    valid_bounded_text(text, CHAT_COMMAND_TEXT_MAX_BYTES)
}

/// Play ChatCommand payload: a length-prefixed UTF-8 instruction for the
/// authority. The server decides what the text may do; this codec only
/// keeps the transfer bounded and well-formed.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ChatCommand {
    pub text: String,
}

impl ChatCommand {
    pub const PACKET_ID: u32 = 12;

    pub fn new(text: String) -> Result<Self, ProtocolError> {
        if !valid_command_text(&text) {
            return Err(ProtocolError::InvalidString);
        }
        Ok(Self { text })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.string(&self.text, CHAT_COMMAND_TEXT_MAX_BYTES);
        encoder
            .finish()
            .expect("validated chat command is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let text = decoder.string(CHAT_COMMAND_TEXT_MAX_BYTES, CHAT_COMMAND_TEXT_MAX_BYTES)?;
        decoder.done()?;
        Self::new(text)
    }
}
