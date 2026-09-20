use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use mornlea_domain::Dimension;

/// Maximum chunk coordinates one payload may carry, copied from the Go
/// `ForgetChunks.Validate` bound.
pub const MAX_FORGET_CHUNKS: u32 = 4096;

/// Fixed stride of one encoded chunk coordinate: two little-endian `i32`.
const CHUNK_COORD_WIRE_BYTES: usize = 8;

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
        if chunks.is_empty() || chunks.len() > MAX_FORGET_CHUNKS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        for (index, chunk) in chunks.iter().enumerate() {
            if chunks[..index].contains(chunk) {
                return Err(ProtocolError::InvalidRange);
            }
        }
        Ok(Self { dimension, chunks })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.i32(i32::from(self.dimension.get()));
        encoder.uvarint(self.chunks.len() as u32);
        for (x, z) in &self.chunks {
            encoder.i32(*x);
            encoder.i32(*z);
        }
        encoder
            .finish()
            .expect("validated forget chunks are encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
            .map_err(|_| ProtocolError::InvalidEnum)?;
        let count = decoder.uvarint()?;
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
