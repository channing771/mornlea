use crate::batch::strictly_increasing;
use crate::block::{chunk_block_index, chunk_of, inside_world, registered_block};
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;
use mornlea_domain::Dimension;

/// Maximum block changes one payload may carry. Zero changes stay legal
/// because a revision barrier for an item-only tick carries no blocks.
pub const MAX_BLOCK_CHANGES: u32 = 4096;

/// Fixed stride of one encoded block change: three little-endian `i32`
/// coordinates and a little-endian `u16` block.
const BLOCK_CHANGE_WIRE_BYTES: usize = 4 + 4 + 4 + 2;

/// Fixed header stride before the count: the dimension, the two chunk
/// coordinates and the two revisions.
const BLOCK_CHANGES_HEADER_BYTES: usize = 4 + 4 + 4 + 8 + 8;

/// One authoritative block write inside a chunk column.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct BlockChange {
    pub x: i32,
    pub y: i32,
    pub z: i32,
    pub block: u16,
}

/// Play BlockChanges payload: the chunk-scoped block writes of one revision
/// step. Positions are absolute world coordinates and must stay inside the
/// announced chunk, be sorted by chunk-ordered block index, and name a
/// registered block.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BlockChanges {
    pub dimension: Dimension,
    pub chunk_x: i32,
    pub chunk_z: i32,
    pub base_revision: u64,
    pub new_revision: u64,
    pub changes: Vec<BlockChange>,
}

impl BlockChanges {
    pub const PACKET_ID: u32 = 1;

    /// Builds a validated batch.
    ///
    /// The revision transition must be exactly `base + 1` from a non-zero,
    /// non-saturated base; changes must be sorted by chunk-ordered block
    /// index and stay inside the world span and the announced chunk.
    pub fn new(
        dimension: Dimension,
        chunk_x: i32,
        chunk_z: i32,
        base_revision: u64,
        new_revision: u64,
        changes: Vec<BlockChange>,
    ) -> Result<Self, ProtocolError> {
        let changes = Self {
            dimension,
            chunk_x,
            chunk_z,
            base_revision,
            new_revision,
            changes,
        };
        changes.valid()?;
        Ok(changes)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into an invalid revision, an
    /// unregistered block or an out-of-span position after construction is
    /// refused rather than silently published. The order is the Go
    /// `BlockChanges.Validate` order — the revision transition, then the count
    /// bound, then each change in submitted order — and the block-registration
    /// and world-span checks are split so each field reports the boundary that
    /// owns it: an unregistered block is the registered-numbering boundary
    /// (`InvalidEnum`) and an out-of-span Y is the geometry boundary
    /// (`InvalidRange`), which are the two categories the Go validator
    /// publishes for the same bytes.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.base_revision == 0
            || self.base_revision == u64::MAX
            || self.new_revision != self.base_revision + 1
            || self.changes.len() > MAX_BLOCK_CHANGES as usize
        {
            return Err(ProtocolError::InvalidRange);
        }
        let mut indices = Vec::with_capacity(self.changes.len());
        for change in &self.changes {
            if !registered_block(change.block) {
                return Err(ProtocolError::InvalidEnum);
            }
            if !inside_world(change.y) {
                return Err(ProtocolError::InvalidRange);
            }
            if chunk_of(change.x, change.z) != (self.chunk_x, self.chunk_z) {
                return Err(ProtocolError::InvalidRange);
            }
            indices.push(chunk_block_index(change.x, change.y, change.z));
        }
        if !strictly_increasing(&indices) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The exact encoded length: the fixed header, the canonical uvarint count
    /// and one stride per change.
    ///
    /// The value gate runs first, so an invalid record reports its own error
    /// here instead of reaching a size decision. The count is bounded by the
    /// gate, so the length arithmetic cannot overflow the platform's usize.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        let count = self.changes.len();
        let records = count
            .checked_mul(BLOCK_CHANGE_WIRE_BYTES)
            .ok_or(ProtocolError::Allocation)?;
        let length = BLOCK_CHANGES_HEADER_BYTES
            .checked_add(canonical_uvarint_length(count as u32))
            .and_then(|length| length.checked_add(records))
            .ok_or(ProtocolError::Allocation)?;
        Ok(length)
    }

    /// Publishes the batch into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the dimension, the chunk
    /// coordinates, the two revisions, the canonical uvarint count and the
    /// per-change records — and the destination is tested before the first
    /// byte is written, so a short call leaves every destination byte
    /// unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer: &mut SliceWriter<'_>| {
            writer.i32(i32::from(self.dimension.get()));
            writer.i32(self.chunk_x);
            writer.i32(self.chunk_z);
            writer.u64(self.base_revision);
            writer.u64(self.new_revision);
            writer.uvarint(self.changes.len() as u32);
            for change in &self.changes {
                writer.i32(change.x);
                writer.i32(change.y);
                writer.i32(change.z);
                writer.u16(change.block);
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
        let chunk_x = decoder.i32()?;
        let chunk_z = decoder.i32()?;
        let base_revision = decoder.u64()?;
        let new_revision = decoder.u64()?;
        let count = decoder.uvarint()?;
        // The count bound fires before the record-length rule, which is the
        // order the Go decode arm applies: a count above the ceiling is
        // refused as a range violation even when the remaining payload is also
        // too short for the records it names.
        if count > MAX_BLOCK_CHANGES {
            return Err(ProtocolError::InvalidRange);
        }
        if decoder.remaining() < count as usize * BLOCK_CHANGE_WIRE_BYTES {
            return Err(ProtocolError::Truncated);
        }
        let mut changes = Vec::with_capacity(count as usize);
        for _ in 0..count {
            let x = decoder.i32()?;
            let y = decoder.i32()?;
            let z = decoder.i32()?;
            let block = decoder.u16()?;
            changes.push(BlockChange { x, y, z, block });
        }
        decoder.done()?;
        Self::new(
            dimension,
            chunk_x,
            chunk_z,
            base_revision,
            new_revision,
            changes,
        )
    }
}
