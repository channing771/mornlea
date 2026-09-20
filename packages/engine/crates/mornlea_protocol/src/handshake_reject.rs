use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

const MESSAGE_MAX_BYTES: usize = 256;
const MESSAGE_MAX_RUNES: usize = 256;

/// Handshake reject code copied from the Go `HandshakeVersionMismatch` pin.
pub const HANDSHAKE_VERSION_MISMATCH: u8 = 1;

/// Handshake HandshakeReject payload. Unknown reject codes, oversized
/// messages, and malformed UTF-8 fail before the record is published.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct HandshakeReject {
    pub server_protocol_version: u32,
    pub code: u8,
    pub message: String,
}

impl HandshakeReject {
    pub const PACKET_ID: u32 = 1;

    pub fn new(
        server_protocol_version: u32,
        code: u8,
        message: impl Into<String>,
    ) -> Result<Self, ProtocolError> {
        if code != HANDSHAKE_VERSION_MISMATCH {
            return Err(ProtocolError::InvalidEnum);
        }
        let message = message.into();
        if message.len() > MESSAGE_MAX_BYTES || message.chars().count() > MESSAGE_MAX_RUNES {
            return Err(ProtocolError::InvalidString);
        }
        Ok(Self {
            server_protocol_version,
            code,
            message,
        })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.uvarint(self.server_protocol_version);
        encoder.u8(self.code);
        encoder.string(&self.message, MESSAGE_MAX_BYTES);
        encoder
            .finish()
            .expect("validated handshake reject is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let server_protocol_version = decoder.uvarint()?;
        let code = decoder.u8()?;
        let message = decoder.string(MESSAGE_MAX_BYTES, MESSAGE_MAX_RUNES)?;
        decoder.done()?;
        Self::new(server_protocol_version, code, message)
    }
}
