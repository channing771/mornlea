use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Play SelectHotbar payload. Slot must be inside the domain hotbar range
/// `0..HotbarSlot::COUNT-1`; unknown slots fail before publication.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct SelectHotbar {
    pub sequence: u64,
    pub slot: u8,
}

impl SelectHotbar {
    pub const PACKET_ID: u32 = 5;

    pub fn new(sequence: u64, slot: u8) -> Result<Self, ProtocolError> {
        let slot = mornlea_domain::HotbarSlot::new(slot)
            .map_err(|_| ProtocolError::InvalidRange)?
            .get();
        Ok(Self { sequence, slot })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder.u8(self.slot);
        encoder
            .finish()
            .expect("validated hotbar selection is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        let slot = decoder.u8()?;
        decoder.done()?;
        Self::new(sequence, slot)
    }
}
