use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::handshake_reject::{
    control_message_len, message_length_prefix, read_control_message, valid_control_message,
};
use crate::server_hello::publish_packet;

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
        let disconnect = Self {
            code,
            message: message.into(),
        };
        disconnect.valid()?;
        Ok(disconnect)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// Both fields are public, so the gate runs on every encode instead of
    /// only at construction: a record mutated into an unpublished code or an
    /// oversized message after construction is refused rather than silently
    /// published.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if !valid_disconnect_code(self.code) {
            return Err(ProtocolError::InvalidEnum);
        }
        valid_control_message(&self.message)
    }

    /// The exact encoded length: the code byte followed by the message slot.
    ///
    /// The value gate runs first, so an invalid record reports its value error
    /// here instead of reaching the capacity check.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        control_message_len(1, &self.message)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's, and the destination is tested
    /// before the first byte is written, so a short or invalid call leaves
    /// every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u8(self.code);
            writer.uvarint(message_length_prefix(&self.message));
            writer.bytes(self.message.as_bytes());
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
        let mut decoder = ByteDecoder::new(payload);
        let code = decoder.u8()?;
        let message = read_control_message(&mut decoder)?;
        decoder.done()?;
        Self::new(code, message)
    }
}
