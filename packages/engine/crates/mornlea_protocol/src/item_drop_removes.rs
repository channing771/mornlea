//! The item drop batch that removes drop stacks.
//!
//! This is the remove half of the drop family and shares the identity space,
//! the batch ceiling, and the count header with `item_drop_upserts`. Keeping
//! the two halves in separate modules matches the Go side, where each half has
//! its own wire entry point and packet ID.
//!
//! The identity is the domain's checked `DropId` ordered by the Go
//! `core.DropID.Compare` total order, which compares the raw wire dimension
//! first: the Go validity rule checks only the slot range and the generation,
//! so an unusual dimension stays a publishable identity and the batch order
//! never narrows it. The batch applies the same minimum-records rule and
//! end-of-payload check as the upsert half.

use crate::batch::{UvarintCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::drop_id::{self, DROP_ID_WIRE_BYTES, DropId, MAX_ITEM_DROP_BATCH};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;

/// Fixed header stride before the records: the eight-byte server tick.
const BATCH_TICK_WIRE_BYTES: usize = 8;

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
        let batch = Self { server_tick, ids };
        batch.valid()?;
        Ok(batch)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate order is the Go `ItemDropRemoves.Validate` order: the batch
    /// count bound, then the strictly increasing identity order. A slot past
    /// the fixed per-chunk array and a zero generation are refused where the
    /// identity is read, because `DropId` is a checked newtype.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.ids.is_empty() || self.ids.len() > MAX_ITEM_DROP_BATCH as usize {
            return Err(ProtocolError::InvalidRange);
        }
        if !strictly_increasing(&self.ids) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The exact encoded length: the eight-byte server tick, the canonical
    /// uvarint count and one fixed identity stride per record.
    ///
    /// The value gate runs first, so an invalid batch reports its count or
    /// order error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        let count = self.ids.len();
        let records = count
            .checked_mul(DROP_ID_WIRE_BYTES)
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
    /// uvarint count and the ordered identities — and the destination is
    /// tested before the first byte is written, so a short or invalid call
    /// leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.server_tick);
            writer.uvarint(self.ids.len() as u32);
            for id in &self.ids {
                drop_id::write_into(*id, writer);
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
        batch.require_minimum_records(&decoder, DROP_ID_WIRE_BYTES)?;
        let mut ids = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            ids.push(drop_id::read(&mut decoder)?);
        }
        decoder.done()?;
        Self::new(batch.server_tick, ids)
    }
}
