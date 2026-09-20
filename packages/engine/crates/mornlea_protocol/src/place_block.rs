use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Play PlaceBlock payload. Slot must be inside the domain hotbar range
/// `0..HotbarSlot::COUNT-1`, and yaw/pitch must be finite. The targeted
/// block stays server-owned.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PlaceBlock {
    pub sequence: u64,
    pub yaw: f32,
    pub pitch: f32,
    pub slot: u8,
}

impl PlaceBlock {
    pub const PACKET_ID: u32 = 2;

    pub fn new(sequence: u64, yaw: f32, pitch: f32, slot: u8) -> Result<Self, ProtocolError> {
        if !yaw.is_finite() || !pitch.is_finite() {
            return Err(ProtocolError::InvalidFloat);
        }
        let slot = mornlea_domain::HotbarSlot::new(slot)
            .map_err(|_| ProtocolError::InvalidRange)?
            .get();
        Ok(Self {
            sequence,
            yaw,
            pitch,
            slot,
        })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder.f32(self.yaw);
        encoder.f32(self.pitch);
        encoder.u8(self.slot);
        encoder
            .finish()
            .expect("validated place-block command is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        let yaw = decoder.f32()?;
        let pitch = decoder.f32()?;
        let slot = decoder.u8()?;
        decoder.done()?;
        Self::new(sequence, yaw, pitch, slot)
    }
}
