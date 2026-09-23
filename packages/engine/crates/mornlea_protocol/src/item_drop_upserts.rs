//! The item drop batch that adds or replaces whole drop stacks.
//!
//! The batch carries a canonical uvarint count, so it shares the uvarint-count
//! header with the companion and remote-player families. Unlike those
//! families it allows the payload to be longer than the records need, and the
//! trailing-byte check at the end of decoding rejects the remainder: the Go
//! decoder validates the record budget before it allocates and applies the
//! end-of-payload check afterwards, so a short payload is a truncation and a
//! padded one is a trailing byte. Using the exact-remaining-length rule here
//! would report the padded batch at the wrong boundary.
//!
//! The identity is the domain's checked `DropId`, whose field order is the Go
//! `core.DropID.Compare` total order — dimension first, as the raw wire `i32`
//! the Go validity rule never narrows. A drop naming an unusual dimension is a
//! publishable identity on both sides, so the ordering compares the raw
//! dimension rather than a validated enum. The per-record stack goes through
//! the shared `ItemStack` rule, so the exact empty triple `(0,0,0)` is
//! wire-valid and a durable item at durability zero is not.
//!
//! One boundary is latent across the two implementations and therefore pinned
//! in the group test rather than in the corpus: the Go validator folds an
//! unregistered item number, a count and a durability violation into the
//! single message `network: invalid item drop stack`, while the Rust rule
//! answers an unregistered number with `InvalidEnum` and the count and
//! durability boundaries with `InvalidRange`. The frozen corpus freezes the
//! count and stack-limit violations, where both sides publish the value
//! boundary, and the group test pins the unregistered number at the Rust
//! variant beside the Go message coarseness.

use crate::batch::UvarintCountBatch;
use crate::block::MAX_CHUNK_BLOCK_INDEX;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::drop_id::{self, DROP_ID_WIRE_BYTES, DropId, MAX_ITEM_DROP_BATCH};
use crate::error::ProtocolError;
use crate::item_stack::{self, ItemStack};
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;

/// Fixed encoded length of one drop record: the drop identity, a `u32` block
/// index, and the fixed 5-byte item stack.
pub const ITEM_DROP_WIRE_BYTES: usize = DROP_ID_WIRE_BYTES + 4 + 5;

/// Fixed header stride before the records: the eight-byte server tick.
const BATCH_TICK_WIRE_BYTES: usize = 8;

/// One authoritative drop stack value.
///
/// The fields are public, so the value gate runs on every encode instead of
/// only at construction: a record mutated into an out-of-range block index or
/// an invalid stack after construction is refused rather than silently
/// published. The identity is the checked domain `DropId`, so a slot past the
/// fixed per-chunk array and a zero generation cannot be constructed at all.
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
        let batch = Self { server_tick, drops };
        batch.valid()?;
        Ok(batch)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate order is the Go `ItemDropUpserts.Validate` order: the batch
    /// count bound, then per record the block-index bound and the shared stack
    /// rule with the strict identity order checked against the previous
    /// record. The identity comparison is the domain order, which compares the
    /// raw dimension first exactly as the Go `Compare` does.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.drops.is_empty() || self.drops.len() > MAX_ITEM_DROP_BATCH as usize {
            return Err(ProtocolError::InvalidRange);
        }
        let mut previous: Option<DropId> = None;
        for drop in &self.drops {
            if drop.block_index >= MAX_CHUNK_BLOCK_INDEX {
                return Err(ProtocolError::InvalidRange);
            }
            item_stack::checked(drop.item, drop.count, drop.durability)?;
            if previous.is_some_and(|last| last >= drop.id) {
                return Err(ProtocolError::InvalidRange);
            }
            previous = Some(drop.id);
        }
        Ok(())
    }

    /// The exact encoded length: the eight-byte server tick, the canonical
    /// uvarint count and one fixed stride per record.
    ///
    /// The value gate runs first, so an invalid batch reports its count,
    /// record or order error here instead of reaching a size or capacity
    /// decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        let count = self.drops.len();
        let records = count
            .checked_mul(ITEM_DROP_WIRE_BYTES)
            .ok_or(ProtocolError::Allocation)?;
        BATCH_TICK_WIRE_BYTES
            .checked_add(canonical_uvarint_length(count as u32))
            .and_then(|length| length.checked_add(records))
            .ok_or(ProtocolError::Allocation)
    }

    /// Publishes the batch into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the server tick, the canonical
    /// uvarint count and the per-record identity, block index and item stack —
    /// and the destination is tested before the first byte is written, so a
    /// short or invalid call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.server_tick);
            writer.uvarint(self.drops.len() as u32);
            for drop in &self.drops {
                drop_id::write_into(drop.id, writer);
                writer.u32(drop.block_index);
                item_stack::write_into(
                    item_stack::checked(drop.item, drop.count, drop.durability)
                        .expect("validated item drop is encodable"),
                    writer,
                );
            }
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
        let batch = UvarintCountBatch::read(&mut decoder, MAX_ITEM_DROP_BATCH)?;
        batch.require_minimum_records(&decoder, ITEM_DROP_WIRE_BYTES)?;
        let mut drops = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = drop_id::read(&mut decoder)?;
            let block_index = decoder.u32()?;
            let stack = item_stack::read(&mut decoder)?;
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
