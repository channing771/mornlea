use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Play TakeCraftingOutput payload. A zero sequence is rejected because it
/// cannot take part in the command-ack protocol; output contents stay
/// server-owned.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct TakeCraftingOutput {
    pub sequence: u64,
}

impl TakeCraftingOutput {
    pub const PACKET_ID: u32 = 15;

    pub fn new(sequence: u64) -> Result<Self, ProtocolError> {
        if sequence == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self { sequence })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder
            .finish()
            .expect("validated take-crafting-output command is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        decoder.done()?;
        Self::new(sequence)
    }
}
