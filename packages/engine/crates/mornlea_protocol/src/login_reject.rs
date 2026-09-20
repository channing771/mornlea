use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

const MESSAGE_MAX_BYTES: usize = 256;
const MESSAGE_MAX_RUNES: usize = 256;

/// Login reject codes copied from the Go `LoginRejectCode` pin.
pub const LOGIN_SERVER_FULL: u8 = 1;
pub const LOGIN_INVALID_IDENTITY: u8 = 2;
pub const LOGIN_PLAYER_DATA_CORRUPT: u8 = 3;
pub const LOGIN_STORE_UNAVAILABLE: u8 = 4;
pub const LOGIN_PROTOCOL_VIOLATION: u8 = 5;
pub const LOGIN_INTERNAL_ERROR: u8 = 6;
pub const LOGIN_ALREADY_ONLINE: u8 = 7;

fn valid_login_reject_code(code: u8) -> bool {
    matches!(
        code,
        LOGIN_SERVER_FULL
            | LOGIN_INVALID_IDENTITY
            | LOGIN_PLAYER_DATA_CORRUPT
            | LOGIN_STORE_UNAVAILABLE
            | LOGIN_PROTOCOL_VIOLATION
            | LOGIN_INTERNAL_ERROR
            | LOGIN_ALREADY_ONLINE
    )
}

/// Login LoginReject payload. Unknown reject codes, oversized messages,
/// and malformed UTF-8 fail before the record is published.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct LoginReject {
    pub code: u8,
    pub message: String,
}

impl LoginReject {
    pub const PACKET_ID: u32 = 1;

    pub fn new(code: u8, message: impl Into<String>) -> Result<Self, ProtocolError> {
        if !valid_login_reject_code(code) {
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
        encoder
            .finish()
            .expect("validated login reject is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let code = decoder.u8()?;
        let message = decoder.string(MESSAGE_MAX_BYTES, MESSAGE_MAX_RUNES)?;
        decoder.done()?;
        Self::new(code, message)
    }
}
