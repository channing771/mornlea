//! The stable identity of one authoritative item drop.
//!
//! The identity is the sort key of both drop families, so its ordering lives
//! once in the domain value, re-exported here: the protocol reads the fixed
//! wire stride and maps the domain rejection into the protocol error. The
//! dimension is deliberately not validated beyond the domain rule — the Go
//! `core.DropID.Valid` rule checks only the slot range and the generation, so
//! a drop that names an unusual dimension is still a publishable identity and
//! must not be rejected by a stricter Rust rule.
//!
//! A slot past the fixed per-chunk array and a zero generation are the one
//! identity rejection this wire edge publishes, and they answer at the
//! identity boundary rather than the range boundary: the Go decoder reports
//! both with `network: invalid item drop ID` (and `network: item drop remove
//! %d: invalid ID`), which is the same boundary the domain publication corpus
//! freezes for drop-ID slot and generation errors, so the protocol error
//! vocabulary keeps `InvalidIdentity` for them.

use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
pub use mornlea_domain::DropId;
use mornlea_domain::{ChunkPos, DomainError};

/// Maximum drop records one payload may carry, copied from the Go
/// `MaxItemDropBatch` pin. Both the upsert and the remove batch share this
/// ceiling because they describe the same bounded drop set.
pub const MAX_ITEM_DROP_BATCH: u32 = 32;

/// Fixed encoded length of one drop identity: an `i32` dimension, two `i32`
/// chunk coordinates, a `u8` slot, and a `u32` generation.
pub const DROP_ID_WIRE_BYTES: usize = 4 + 4 + 4 + 1 + 4;

/// Reads one 17-byte identity and runs the domain rule. A slot past the fixed
/// per-chunk array and a zero generation are the identity boundary the Go
/// decoder names with its invalid-drop-ID message, which is the mapping the
/// packet families publish.
pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<DropId, ProtocolError> {
    let dimension = decoder.i32()?;
    let chunk_x = decoder.i32()?;
    let chunk_z = decoder.i32()?;
    let slot = decoder.u8()?;
    let generation = decoder.u32()?;
    DropId::try_new(dimension, ChunkPos::new(chunk_x, chunk_z), slot, generation).map_err(|error| {
        match error {
            DomainError::InvalidDropSlot | DomainError::InvalidDropGeneration => {
                ProtocolError::InvalidIdentity
            }
            _ => ProtocolError::InvalidRange,
        }
    })
}

/// Publishes one identity into a caller-owned publication window.
///
/// The bytes are the fixed wire field order with the raw dimension verbatim,
/// so the caller-owned window cannot drift from the decoded form.
pub(crate) fn write_into(id: DropId, writer: &mut SliceWriter<'_>) {
    writer.i32(id.dimension());
    writer.i32(id.chunk().x());
    writer.i32(id.chunk().z());
    writer.u8(id.slot());
    writer.u32(id.generation());
}
