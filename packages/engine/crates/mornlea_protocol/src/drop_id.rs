//! The stable identity of one authoritative item drop.
//!
//! The drop identity is the sort key of both drop families, so its ordering
//! lives here once instead of being restated by the upsert and remove batches.
//! The dimension is deliberately not validated: the Go `DropID.Valid` rule
//! checks only the slot range and the generation, so a drop that names an
//! unusual dimension is still a publishable identity and must not be rejected
//! by a stricter Rust rule.

use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Fixed authoritative drop slots per chunk, copied from the Go
/// `DropsPerChunk` pin.
pub const DROPS_PER_CHUNK: u8 = 32;

/// Fixed encoded length of one drop identity: an `i32` dimension, two `i32`
/// chunk coordinates, a `u8` slot, and a `u32` generation.
pub const DROP_ID_WIRE_BYTES: usize = 4 + 4 + 4 + 1 + 4;

/// Identity of one drop stack for its whole lifetime. A reused slot raises the
/// generation, so an old identity never collides with a new stack.
#[derive(Clone, Copy, Debug, Eq, Ord, PartialEq, PartialOrd)]
pub struct DropId {
    dimension: i32,
    chunk_x: i32,
    chunk_z: i32,
    slot: u8,
    generation: u32,
}

impl DropId {
    /// Builds a publishable identity.
    ///
    /// The slot must fit the fixed per-chunk array and the generation must be
    /// non-zero, matching the Go rule that an unused slot has no identity.
    pub fn new(
        dimension: i32,
        chunk_x: i32,
        chunk_z: i32,
        slot: u8,
        generation: u32,
    ) -> Result<Self, ProtocolError> {
        if slot >= DROPS_PER_CHUNK || generation == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self {
            dimension,
            chunk_x,
            chunk_z,
            slot,
            generation,
        })
    }

    pub fn dimension(self) -> i32 {
        self.dimension
    }

    pub fn chunk_x(self) -> i32 {
        self.chunk_x
    }

    pub fn chunk_z(self) -> i32 {
        self.chunk_z
    }

    pub fn slot(self) -> u8 {
        self.slot
    }

    pub fn generation(self) -> u32 {
        self.generation
    }

    pub(crate) fn write(&self, encoder: &mut ByteEncoder) {
        encoder.i32(self.dimension);
        encoder.i32(self.chunk_x);
        encoder.i32(self.chunk_z);
        encoder.u8(self.slot);
        encoder.u32(self.generation);
    }

    pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<Self, ProtocolError> {
        let dimension = decoder.i32()?;
        let chunk_x = decoder.i32()?;
        let chunk_z = decoder.i32()?;
        let slot = decoder.u8()?;
        let generation = decoder.u32()?;
        Self::new(dimension, chunk_x, chunk_z, slot, generation)
    }
}
