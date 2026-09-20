use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Play CollectWater payload. The server validates that the ray-cast target
/// is a water source the held bucket can collect from and owns the
/// resulting item change; the client only carries the look direction.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct CollectWater {
    pub sequence: u64,
    pub yaw: f32,
    pub pitch: f32,
}

impl CollectWater {
    pub const PACKET_ID: u32 = 16;

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
            .expect("validated collect-water command is encodable")
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
