use crate::batch::strictly_increasing;
use crate::block::{chunk_block_index, chunk_of, inside_world, registered_block};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use mornlea_domain::Dimension;

/// Maximum block changes one payload may carry. Zero changes stay legal
/// because a revision barrier for an item-only tick carries no blocks.
pub const MAX_BLOCK_CHANGES: u32 = 4096;

/// Fixed stride of one encoded block change: three little-endian `i32`
/// coordinates and a little-endian `u16` block.
const BLOCK_CHANGE_WIRE_BYTES: usize = 4 + 4 + 4 + 2;

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
        if base_revision == 0
            || base_revision == u64::MAX
            || new_revision != base_revision + 1
            || changes.len() > MAX_BLOCK_CHANGES as usize
        {
            return Err(ProtocolError::InvalidRange);
        }
        let mut indices = Vec::with_capacity(changes.len());
        for change in &changes {
            if !registered_block(change.block) || !inside_world(change.y) {
                return Err(ProtocolError::InvalidEnum);
            }
            if chunk_of(change.x, change.z) != (chunk_x, chunk_z) {
                return Err(ProtocolError::InvalidRange);
            }
            indices.push(chunk_block_index(change.x, change.y, change.z));
        }
        if !strictly_increasing(&indices) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self {
            dimension,
            chunk_x,
            chunk_z,
            base_revision,
            new_revision,
            changes,
        })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.i32(i32::from(self.dimension.get()));
        encoder.i32(self.chunk_x);
        encoder.i32(self.chunk_z);
        encoder.u64(self.base_revision);
        encoder.u64(self.new_revision);
        encoder.uvarint(self.changes.len() as u32);
        for change in &self.changes {
            encoder.i32(change.x);
            encoder.i32(change.y);
            encoder.i32(change.z);
            encoder.u16(change.block);
        }
        encoder
            .finish()
            .expect("validated block changes are encodable")
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
