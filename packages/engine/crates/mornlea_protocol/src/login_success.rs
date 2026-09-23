use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};
use crate::server_hello::publish_packet;

/// Fixed wire stride of one login success payload: the 16 identity bytes
/// followed by the little-endian `u64` world seed.
const LOGIN_SUCCESS_WIRE_BYTES: usize = 24;

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

    /// The single value gate shared by `encode_into` and `decode`.
    ///
    /// The gate is total: the identity is checked by `PlayerId` itself, so it
    /// cannot be constructed invalid, and the Go validator places no rule on
    /// the seed — a zero seed is a legal world seed and is copied verbatim. No
    /// field mutation can therefore make a login success record unpublishable,
    /// which is why the family keeps the surface without a rejection branch.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.validate()?;
        Ok(LOGIN_SUCCESS_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — identity bytes first, then the
    /// little-endian seed — and the destination is tested before the first
    /// byte is written, so a short call leaves every destination byte
    /// unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.bytes(&self.player_id.bytes());
            writer.u64(self.world_seed);
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
        let player_id = player_id::read(&mut decoder)?;
        let world_seed = decoder.u64()?;
        decoder.done()?;
        Ok(Self::new(player_id, world_seed))
    }
}
