//! The item drop batch that adds or replaces whole drop stacks.
//!
//! The batch carries a canonical uvarint count, so it shares the uvarint-count
//! header with the companion and remote-player families. Unlike those
//! families it allows the payload to be longer than the records need, and the
//! trailing-byte check at the end of decoding rejects the remainder, matching
//! the Go decoder that validates the record budget before allocating and the
//! exact length afterwards.

use crate::batch::{UvarintCountBatch, strictly_increasing};
use crate::block::MAX_CHUNK_BLOCK_INDEX;
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::drop_id::{DROP_ID_WIRE_BYTES, DropId};
use crate::error::ProtocolError;
use crate::item_stack::ItemStack;

/// Maximum drops one payload may carry, copied from the Go `MaxItemDropBatch`
/// pin.
pub const MAX_ITEM_DROP_BATCH: u32 = 32;

/// Fixed encoded length of one drop record: the drop identity, a `u32` block
/// index, and the fixed 5-byte item stack.
pub const ITEM_DROP_WIRE_BYTES: usize = DROP_ID_WIRE_BYTES + 4 + 5;

/// One authoritative drop stack value.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ItemDrop {
    pub id: DropId,
    pub block_index: u32,
    pub item: u16,
    pub count: u8,
    pub durability: u16,
}

/// Play ItemDropUpserts payload: the drop stacks a session must add or replace,
/// strictly ascending by identity.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ItemDropUpserts {
    pub server_tick: u64,
    pub drops: Vec<ItemDrop>,
}

impl ItemDropUpserts {
    pub const PACKET_ID: u32 = 11;

    /// Builds a validated batch.
    ///
    /// The stack is validated through the shared `ItemStack` rule so a drop
    /// cannot publish a slot value the inventory families would reject.
    pub fn new(server_tick: u64, drops: Vec<ItemDrop>) -> Result<Self, ProtocolError> {
        if drops.is_empty() || drops.len() > MAX_ITEM_DROP_BATCH as usize {
            return Err(ProtocolError::InvalidRange);
        }
        for drop in &drops {
            if drop.block_index >= MAX_CHUNK_BLOCK_INDEX {
                return Err(ProtocolError::InvalidRange);
            }
            ItemStack::new(drop.item, drop.count, drop.durability)?;
        }
        let ids: Vec<DropId> = drops.iter().map(|drop| drop.id).collect();
        if !strictly_increasing(&ids) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self { server_tick, drops })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        UvarintCountBatch::write(&mut encoder, self.server_tick, self.drops.len() as u32);
        for drop in &self.drops {
            drop.id.write(&mut encoder);
            encoder.u32(drop.block_index);
            ItemStack::new(drop.item, drop.count, drop.durability)
                .expect("validated item drop is encodable")
                .write(&mut encoder);
        }
        encoder
            .finish()
            .expect("validated item drop upserts are encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let batch = UvarintCountBatch::read(&mut decoder, MAX_ITEM_DROP_BATCH)?;
        batch.require_minimum_records(&decoder, ITEM_DROP_WIRE_BYTES)?;
        let mut drops = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = DropId::read(&mut decoder)?;
            let block_index = decoder.u32()?;
            let stack = ItemStack::read(&mut decoder)?;
            drops.push(ItemDrop {
                id,
                block_index,
                item: stack.item(),
                count: stack.count(),
                durability: stack.durability(),
            });
        }
        decoder.done()?;
        Self::new(batch.server_tick, drops)
    }
}
