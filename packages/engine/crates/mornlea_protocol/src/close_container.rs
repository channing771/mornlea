use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Play CloseContainer payload. A zero sequence is legal and is copied
/// as-is; the viewed container identity stays server-owned.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CloseContainer {
    pub sequence: u64,
}

impl CloseContainer {
    pub const PACKET_ID: u32 = 10;

    pub fn new(sequence: u64) -> Self {
        Self { sequence }
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder
            .finish()
            .expect("validated close-container command is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        decoder.done()?;
        Ok(Self::new(sequence))
    }
}
