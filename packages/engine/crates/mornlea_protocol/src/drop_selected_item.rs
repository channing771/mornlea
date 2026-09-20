use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Play DropSelectedItem payload. A zero sequence is legal and is copied
/// as-is; the selected hotbar slot and drop position are server-owned.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct DropSelectedItem {
    pub sequence: u64,
}

impl DropSelectedItem {
    pub const PACKET_ID: u32 = 11;

    pub fn new(sequence: u64) -> Self {
        Self { sequence }
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder
            .finish()
            .expect("validated drop-selected command is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        decoder.done()?;
        Ok(Self::new(sequence))
    }
}
