use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

const MESSAGE_MAX_BYTES: usize = 256;
const MESSAGE_MAX_RUNES: usize = 256;

/// Disconnect codes copied from the Go `DisconnectCode` pin.
pub const DISCONNECT_PROTOCOL_VIOLATION: u8 = 1;
pub const DISCONNECT_TIMEOUT: u8 = 2;
pub const DISCONNECT_SERVER_SHUTDOWN: u8 = 3;
pub const DISCONNECT_SLOW_CLIENT: u8 = 4;
pub const DISCONNECT_INTERNAL_ERROR: u8 = 5;

fn valid_disconnect_code(code: u8) -> bool {
    matches!(
        code,
        DISCONNECT_PROTOCOL_VIOLATION
            | DISCONNECT_TIMEOUT
            | DISCONNECT_SERVER_SHUTDOWN
            | DISCONNECT_SLOW_CLIENT
            | DISCONNECT_INTERNAL_ERROR
    )
}

/// Play Disconnect payload. Unknown codes, oversized messages, and
/// malformed UTF-8 fail before the record is published.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct Disconnect {
    pub code: u8,
    pub message: String,
}

impl Disconnect {
    pub const PACKET_ID: u32 = 6;

    pub fn new(code: u8, message: impl Into<String>) -> Result<Self, ProtocolError> {
        if !valid_disconnect_code(code) {
            return Err(ProtocolError::InvalidEnum);
        }
        let message = message.into();
        if message.len() > MESSAGE_MAX_BYTES || message.chars().count() > MESSAGE_MAX_RUNES {
            return Err(ProtocolError::InvalidString);
        }
        Ok(Self { code, message })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u8(self.code);
        encoder.string(&self.message, MESSAGE_MAX_BYTES);
        encoder.finish().expect("validated disconnect is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let code = decoder.u8()?;
        let message = decoder.string(MESSAGE_MAX_BYTES, MESSAGE_MAX_RUNES)?;
        decoder.done()?;
        Self::new(code, message)
    }
}
