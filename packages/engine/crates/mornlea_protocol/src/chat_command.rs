use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Maximum UTF-8 byte length of a chat command text, shared with the
/// planner instruction limit so the two bounds cannot drift apart.
pub const CHAT_COMMAND_TEXT_MAX_BYTES: usize = 1024;

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
        if text.is_empty()
            || text.len() > CHAT_COMMAND_TEXT_MAX_BYTES
            || text.trim() != text
            || text.chars().any(char::is_control)
        {
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
