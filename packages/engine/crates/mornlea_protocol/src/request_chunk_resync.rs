use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use mornlea_domain::Dimension;

/// Play RequestChunkResync payload: a little-endian `u64` sequence, the
/// target dimension and chunk coordinates, and the revision the client
/// already holds. Only overworld and depths are known dimensions; the
/// server remains the owner of chunk state.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct RequestChunkResync {
    pub sequence: u64,
    pub dimension: Dimension,
    pub chunk_x: i32,
    pub chunk_z: i32,
    pub have_revision: u64,
}

impl RequestChunkResync {
    pub const PACKET_ID: u32 = 3;

    pub fn new(
        sequence: u64,
        dimension: Dimension,
        chunk_x: i32,
        chunk_z: i32,
        have_revision: u64,
    ) -> Self {
        Self {
            sequence,
            dimension,
            chunk_x,
            chunk_z,
            have_revision,
        }
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder.i32(i32::from(self.dimension.get()));
        encoder.i32(self.chunk_x);
        encoder.i32(self.chunk_z);
        encoder.u64(self.have_revision);
        encoder
            .finish()
            .expect("validated chunk resync request is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
            .map_err(|_| ProtocolError::InvalidEnum)?;
        let chunk_x = decoder.i32()?;
        let chunk_z = decoder.i32()?;
        let have_revision = decoder.u64()?;
        decoder.done()?;
        Ok(Self::new(
            sequence,
            dimension,
            chunk_x,
            chunk_z,
            have_revision,
        ))
    }
}
