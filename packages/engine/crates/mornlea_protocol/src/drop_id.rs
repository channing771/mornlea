//! The stable identity of one authoritative item drop.
//!
//! The identity is the sort key of both drop families, so its ordering lives
//! once in the domain value, re-exported here: the protocol reads the fixed
//! wire stride and maps the domain rejection into the protocol error. The
//! dimension is deliberately not validated beyond the domain rule — the Go
//! `core.DropID.Valid` rule checks only the slot range and the generation, so
//! a drop that names an unusual dimension is still a publishable identity and
//! must not be rejected by a stricter Rust rule.

use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
pub use mornlea_domain::{ChunkPos, DropId};

/// Maximum drop records one payload may carry, copied from the Go
/// `MaxItemDropBatch` pin. Both the upsert and the remove batch share this
/// ceiling because they describe the same bounded drop set.
pub const MAX_ITEM_DROP_BATCH: u32 = 32;

/// Fixed encoded length of one drop identity: an `i32` dimension, two `i32`
/// chunk coordinates, a `u8` slot, and a `u32` generation.
pub const DROP_ID_WIRE_BYTES: usize = 4 + 4 + 4 + 1 + 4;

/// Reads one 17-byte identity and runs the domain rule. A slot past the fixed
/// per-chunk array and a zero generation are `InvalidRange`, which is the
/// mapping the Go decoder publishes.
pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<DropId, ProtocolError> {
    let dimension = decoder.i32()?;
    let chunk_x = decoder.i32()?;
    let chunk_z = decoder.i32()?;
    let slot = decoder.u8()?;
    let generation = decoder.u32()?;
    DropId::try_new(dimension, ChunkPos::new(chunk_x, chunk_z), slot, generation)
        .map_err(|_| ProtocolError::InvalidRange)
}

/// Writes one identity in the wire field order.
pub(crate) fn write(id: DropId, encoder: &mut ByteEncoder) {
    encoder.i32(id.dimension());
    encoder.i32(id.chunk().x());
    encoder.i32(id.chunk().z());
    encoder.u8(id.slot());
    encoder.u32(id.generation());
}
