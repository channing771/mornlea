use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Play PlaceBlockSucceeded payload. A zero sequence is legal and is copied
/// as-is; the packet only acknowledges that a place command settled.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PlaceBlockSucceeded {
    pub sequence: u64,
}

impl PlaceBlockSucceeded {
    pub const PACKET_ID: u32 = 20;

    pub fn new(sequence: u64) -> Self {
        Self { sequence }
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder
            .finish()
            .expect("validated place-block success is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        decoder.done()?;
        Ok(Self::new(sequence))
    }
}
