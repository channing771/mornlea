use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};

/// Login LoginSuccess payload. The player identity must already be a
/// published UUIDv4; a zero world seed is legal and is copied as-is.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct LoginSuccess {
    pub player_id: PlayerId,
    pub world_seed: u64,
}

impl LoginSuccess {
    pub const PACKET_ID: u32 = 0;

    pub fn new(player_id: PlayerId, world_seed: u64) -> Self {
        Self {
            player_id,
            world_seed,
        }
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.bytes(&self.player_id.bytes());
        encoder.u64(self.world_seed);
        encoder
            .finish()
            .expect("validated login success is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let player_id = player_id::read(&mut decoder)?;
        let world_seed = decoder.u64()?;
        decoder.done()?;
        Ok(Self::new(player_id, world_seed))
    }
}
