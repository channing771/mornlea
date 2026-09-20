use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Command reject reason IDs copied from the Go `CommandRejectReasonID` pin.
pub const REJECT_INVALID_RAY: u8 = 1;
pub const REJECT_NO_TARGET: u8 = 2;
pub const REJECT_CHUNK_NOT_READY: u8 = 3;
pub const REJECT_PROTECTED_BLOCK: u8 = 4;
pub const REJECT_INVALID_BLOCK: u8 = 5;
pub const REJECT_OCCUPIED: u8 = 6;
pub const REJECT_INVALID_INPUT: u8 = 7;
pub const REJECT_PLAYER_NOT_READY: u8 = 8;
pub const REJECT_INVALID_SLOT: u8 = 9;
pub const REJECT_HOTBAR_FULL: u8 = 10;
pub const REJECT_DROP_CAPACITY: u8 = 11;
pub const REJECT_CONTAINER_CAPACITY: u8 = 12;
pub const REJECT_NOT_FLUID_SOURCE: u8 = 13;
pub const REJECT_BUCKET_MISMATCH: u8 = 14;
pub const REJECT_NOT_ARMOR: u8 = 15;

fn valid_reject_reason(reason: u8) -> bool {
    (REJECT_INVALID_RAY..=REJECT_NOT_ARMOR).contains(&reason)
}

/// Play CommandRejected payload. Unknown reason IDs fail before publication.
/// A zero sequence is legal and is copied as-is.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CommandRejected {
    pub sequence: u64,
    pub reason: u8,
}

impl CommandRejected {
    pub const PACKET_ID: u32 = 4;

    pub fn new(sequence: u64, reason: u8) -> Result<Self, ProtocolError> {
        if !valid_reject_reason(reason) {
            return Err(ProtocolError::InvalidEnum);
        }
        Ok(Self { sequence, reason })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder.u8(self.reason);
        encoder
            .finish()
            .expect("validated command rejection is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        let reason = decoder.u8()?;
        decoder.done()?;
        Self::new(sequence, reason)
    }
}
