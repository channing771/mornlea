use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;

/// Fixed wire stride of one bone meal payload: the 8-byte sequence and the two
/// look angles.
const BONE_MEAL_WIRE_BYTES: usize = 16;

/// Play BoneMeal payload. The server validates that the ray-cast target is
/// a fertilizable plant block and owns the resulting block write; the
/// client only carries the look direction.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct BoneMeal {
    pub sequence: u64,
    pub yaw: f32,
    pub pitch: f32,
}

impl BoneMeal {
    pub const PACKET_ID: u32 = 14;

    pub fn new(sequence: u64, yaw: f32, pitch: f32) -> Result<Self, ProtocolError> {
        let command = Self {
            sequence,
            yaw,
            pitch,
        };
        command.valid()?;
        Ok(command)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into a non-finite rotation after
    /// construction is refused rather than silently published, which is what
    /// keeps the admitted set identical to the Go validator's.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if !self.yaw.is_finite() || !self.pitch.is_finite() {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its rotation
    /// error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(BONE_MEAL_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — sequence, then the two look
    /// angles — and the destination is tested before the first byte is written,
    /// so a short call leaves every destination byte unchanged. The angles are
    /// published as their exact IEEE-754 bits, so a negative zero survives the
    /// round trip.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
            writer.f32(self.yaw);
            writer.f32(self.pitch);
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
        let sequence = decoder.u64()?;
        let yaw = decoder.f32()?;
        let pitch = decoder.f32()?;
        decoder.done()?;
        Self::new(sequence, yaw, pitch)
    }
}
