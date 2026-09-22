use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Play OpenContainer payload. The server ray-casts the authoritative world
/// and decides whether the hit block is a furnace or a chest, so the client
/// never declares a container kind. A zero sequence is legal and is copied
/// as-is; yaw/pitch must be finite.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct OpenContainer {
    pub sequence: u64,
    pub yaw: f32,
    pub pitch: f32,
}

impl OpenContainer {
    pub const PACKET_ID: u32 = 8;

    pub fn new(sequence: u64, yaw: f32, pitch: f32) -> Result<Self, ProtocolError> {
        if !yaw.is_finite() || !pitch.is_finite() {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(Self {
            sequence,
            yaw,
            pitch,
        })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder.f32(self.yaw);
        encoder.f32(self.pitch);
        encoder
            .finish()
            .expect("validated open-container command is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        let yaw = decoder.f32()?;
        let pitch = decoder.f32()?;
        decoder.done()?;
        Self::new(sequence, yaw, pitch)
    }
}
