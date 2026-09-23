use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;
use mornlea_domain::Dimension;

/// Maximum chunk coordinates one payload may carry, copied from the Go
/// `ForgetChunks.Validate` bound.
pub const MAX_FORGET_CHUNKS: u32 = 4096;

/// Fixed stride of one encoded chunk coordinate: two little-endian `i32`.
const CHUNK_COORD_WIRE_BYTES: usize = 8;

/// Fixed header stride before the count: the dimension alone.
const FORGET_CHUNKS_HEADER_BYTES: usize = 4;

/// Play ForgetChunks payload: the chunk columns a session must drop. The
/// count is bounded, the coordinates must be unique, and a zero-count batch
/// carries no observable meaning.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ForgetChunks {
    pub dimension: Dimension,
    pub chunks: Vec<(i32, i32)>,
}

impl ForgetChunks {
    pub const PACKET_ID: u32 = 2;

    pub fn new(dimension: Dimension, chunks: Vec<(i32, i32)>) -> Result<Self, ProtocolError> {
        let forget = Self { dimension, chunks };
        forget.valid()?;
        Ok(forget)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into an empty batch or a duplicate
    /// chunk after construction is refused rather than silently published. The
    /// order is the Go `ForgetChunks.Validate` order — the count bound, then
    /// the duplicate rejection — and the submitted order is preserved because
    /// the protocol does not demand a sorted batch: the authority groups and
    /// sorts when it publishes, so a replay has to observe the sequence the
    /// record carried. The uniqueness check therefore sorts a fallible
    /// reserved scratch copy rather than the batch itself.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.chunks.is_empty() || self.chunks.len() > MAX_FORGET_CHUNKS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        let mut sorted = reserve_unique_scratch(self.chunks.len())?;
        sorted.extend_from_slice(&self.chunks);
        sorted.sort_unstable();
        if sorted.windows(2).any(|pair| pair[0] == pair[1]) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The exact encoded length: the fixed header, the canonical uvarint count
    /// and one stride per chunk.
    ///
    /// The value gate runs first, so an invalid record reports its own error
    /// here instead of reaching a size decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        let count = self.chunks.len();
        let records = count
            .checked_mul(CHUNK_COORD_WIRE_BYTES)
            .ok_or(ProtocolError::Allocation)?;
        let length = FORGET_CHUNKS_HEADER_BYTES
            .checked_add(canonical_uvarint_length(count as u32))
            .and_then(|length| length.checked_add(records))
            .ok_or(ProtocolError::Allocation)?;
        Ok(length)
    }

    /// Publishes the batch into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the dimension, the canonical
    /// uvarint count and the per-chunk coordinates — and the destination is
    /// tested before the first byte is written, so a short call leaves every
    /// destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer: &mut SliceWriter<'_>| {
            writer.i32(i32::from(self.dimension.get()));
            writer.uvarint(self.chunks.len() as u32);
            for (x, z) in &self.chunks {
                writer.i32(*x);
                writer.i32(*z);
            }
        })
    }

    /// The allocating compatibility wrapper.
    ///
    /// It reserves exactly the validated length and publishes through
    /// `encode_into`, so the two entry points always agree byte for byte and
    /// an invalid record fails instead of panicking.
    pub fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        let length = self.encoded_len()?;
        let mut wire = vec![0u8; length];
        let written = self.encode_into(&mut wire)?;
        wire.truncate(written);
        Ok(wire)
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
            .map_err(|_| ProtocolError::InvalidEnum)?;
        let count = decoder.uvarint()?;
        // The count bound — zero and above the ceiling alike — fires before
        // the record-length rule, which is the order the Go decode arm
        // applies.
        if count < 1 || count > MAX_FORGET_CHUNKS {
            return Err(ProtocolError::InvalidRange);
        }
        if decoder.remaining() < count as usize * CHUNK_COORD_WIRE_BYTES {
            return Err(ProtocolError::Truncated);
        }
        let mut chunks = Vec::with_capacity(count as usize);
        for _ in 0..count {
            let x = decoder.i32()?;
            let z = decoder.i32()?;
            chunks.push((x, z));
        }
        decoder.done()?;
        Self::new(dimension, chunks)
    }
}

/// Reserves the fallible scratch buffer the uniqueness scan sorts into, one
/// slot per chunk.
///
/// The reservation is checked before any copy or sort: a failed reserve maps
/// to `Allocation` and the caller publishes nothing, so an admitted batch
/// never pays proportional work for a capacity the process cannot back. The
/// borrowed input is never mutated and the scratch is not returned to
/// production callers, which is what keeps the recorded wire order intact.
fn reserve_unique_scratch(count: usize) -> Result<Vec<(i32, i32)>, ProtocolError> {
    let mut scratch: Vec<(i32, i32)> = Vec::new();
    scratch
        .try_reserve_exact(count)
        .map_err(|_| ProtocolError::Allocation)?;
    Ok(scratch)
}
