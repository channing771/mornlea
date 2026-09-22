//! The item drop batch that removes drop stacks.
//!
//! This is the remove half of the drop family and shares the identity space,
//! the batch ceiling, and the count header with `item_drop_upserts`. Keeping
//! the two halves in separate modules matches the Go side, where each half has
//! its own wire entry point and packet ID.

use crate::batch::{UvarintCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::drop_id::{DROP_ID_WIRE_BYTES, DropId, MAX_ITEM_DROP_BATCH};
use crate::error::ProtocolError;

/// Play ItemDropRemoves payload: the drop identities a session must drop,
/// strictly ascending.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ItemDropRemoves {
    pub server_tick: u64,
    pub ids: Vec<DropId>,
}

impl ItemDropRemoves {
    pub const PACKET_ID: u32 = 12;

    /// Builds a validated batch. Identity validity is already enforced by
    /// `DropId`, so this gate only checks the batch bounds and the order.
    pub fn new(server_tick: u64, ids: Vec<DropId>) -> Result<Self, ProtocolError> {
        if ids.is_empty() || ids.len() > MAX_ITEM_DROP_BATCH as usize {
            return Err(ProtocolError::InvalidRange);
        }
        if !strictly_increasing(&ids) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self { server_tick, ids })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        UvarintCountBatch::write(&mut encoder, self.server_tick, self.ids.len() as u32);
        for id in &self.ids {
            id.write(&mut encoder);
        }
        encoder
            .finish()
            .expect("validated item drop removes are encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let batch = UvarintCountBatch::read(&mut decoder, MAX_ITEM_DROP_BATCH)?;
        batch.require_minimum_records(&decoder, DROP_ID_WIRE_BYTES)?;
        let mut ids = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            ids.push(DropId::read(&mut decoder)?);
        }
        decoder.done()?;
        Self::new(batch.server_tick, ids)
    }
}
