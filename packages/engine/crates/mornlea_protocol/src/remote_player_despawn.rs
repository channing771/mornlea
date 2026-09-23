use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};

/// Play RemotePlayerDespawn payload: a 16-byte UUIDv4 identity of the
/// remote player the authoritative world removed. The remaining players
/// stay server-owned.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct RemotePlayerDespawn {
    pub player: PlayerId,
}

impl RemotePlayerDespawn {
    pub const PACKET_ID: u32 = 8;

    pub fn new(player: PlayerId) -> Self {
        Self { player }
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.bytes(&self.player.bytes());
        encoder.finish().expect("validated identity is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let player = player_id::read(&mut decoder)?;
        decoder.done()?;
        Ok(Self::new(player))
    }
}
