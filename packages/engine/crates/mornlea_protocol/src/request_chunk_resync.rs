use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use mornlea_domain::Dimension;

/// Fixed wire stride of one chunk resync request: the 8-byte sequence, the
/// raw dimension, the two chunk coordinates and the 8-byte held revision.
const REQUEST_CHUNK_RESYNC_WIRE_BYTES: usize = 28;

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

    /// The single value gate shared by `encode_into` and `decode`.
    ///
    /// The gate is total: the dimension is checked by `Dimension` itself, so it
    /// cannot be constructed invalid, and the Go validator places no rule on
    /// the coordinates or the held revision — negative coordinates are legal
    /// and a zero revision names a chunk the client holds nothing for. No
    /// field mutation can therefore make a resync record unpublishable, which
    /// is why the family keeps the surface without a rejection branch.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.validate()?;
        Ok(REQUEST_CHUNK_RESYNC_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — sequence, the dimension as the raw
    /// `i32` the domain value carries, the two chunk coordinates, then the held
    /// revision — and the destination is tested before the first byte is
    /// written, so a short call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
            writer.i32(i32::from(self.dimension.get()));
            writer.i32(self.chunk_x);
            writer.i32(self.chunk_z);
            writer.u64(self.have_revision);
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
        // The raw `i32` is matched against the known dimension IDs instead of
        // being narrowed to a `u8`: a value such as 256 fits the byte port but
        // is not a known dimension, and reinterpreting it would publish a
        // record the Go validator refuses.
        let raw_dimension = decoder.i32()?;
        let dimension = match raw_dimension {
            0 => Dimension::OVERWORLD,
            1 => Dimension::DEPTHS,
            _ => return Err(ProtocolError::InvalidEnum),
        };
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
