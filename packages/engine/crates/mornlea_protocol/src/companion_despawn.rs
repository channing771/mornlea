use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::entity_id::CompanionId;
use crate::error::ProtocolError;

/// Play CompanionDespawn payload: the 16-byte companion identity the
/// authoritative world removed. One message names exactly one companion.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CompanionDespawn {
    pub companion: CompanionId,
}

impl CompanionDespawn {
    pub const PACKET_ID: u32 = 19;

    pub fn new(companion: CompanionId) -> Self {
        Self { companion }
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.bytes(&self.companion.bytes());
        encoder
            .finish()
            .expect("validated companion identity is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let companion = CompanionId::new(decoder.bytes()?)?;
        decoder.done()?;
        Ok(Self::new(companion))
    }
}
