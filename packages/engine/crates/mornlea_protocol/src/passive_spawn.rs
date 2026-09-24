//! The passive mob spawn batch that publishes newly visible passive bodies.
//!
//! The batch carries a one-byte record count and records of a fixed 29-byte
//! stride, so it shares the byte-count header with the passive state and
//! despawn batches. Unlike a hostile spawn record it has no kind byte: a
//! passive mob is the only passive kind, so the health field is the trailing
//! byte of the record. The order is the raw numeric `u64` identity, held here
//! by the checked domain `PassiveId`, and the count is bounded before the
//! records are read.
//!
//! The batch applies the exact-remaining-length rule, not the
//! minimum-records rule: the Go decoder rejects a payload whose remaining
//! length is not exactly `count` records before it reads one, so a short and a
//! padded payload both answer at the truncation boundary. Using the
//! minimum-records rule here would report a padded batch as a trailing byte,
//! which is a different failure than the Go side publishes.
//!
//! The record gate keeps the Go `PassiveSpawnRecord.validate` order: the
//! dimension, the pose finiteness and the health span. The identity is already
//! checked by `PassiveId`, so the record gate restates no identity rule, and
//! the dimension is the checked domain value, so only the overworld is
//! admissible — which is also why no kind match exists here.

use crate::batch::ByteCountBatch;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::passive_id::{self, PASSIVE_ID_WIRE_BYTES, PassiveId};
use crate::server_hello::publish_packet;
use mornlea_domain::Dimension;

/// Maximum passive spawn records one payload may carry, copied from the Go
/// `MaxPassiveRecords` pin.
pub const MAX_PASSIVE_SPAWN_RECORDS: u8 = 64;

/// Fixed stride of one passive spawn record: the identity, the dimension, the
/// position, the yaw and the health. No kind byte is carried, because a
/// passive mob has no category to publish.
pub const PASSIVE_SPAWN_WIRE_BYTES: usize = PASSIVE_ID_WIRE_BYTES + 4 + 12 + 4 + 1;

/// Fixed wire upper bound of the whole payload.
pub const PASSIVE_SPAWN_MAX_WIRE_BYTES: usize =
    9 + MAX_PASSIVE_SPAWN_RECORDS as usize * PASSIVE_SPAWN_WIRE_BYTES;

/// Fixed header stride before the records: the eight-byte server tick and the
/// one-byte record count.
const BATCH_HEADER_WIRE_BYTES: usize = 9;

/// One passive spawn record: a nonzero identity, the overworld dimension, a
/// finite pose, and health inside `1..=MAX_HEALTH`.
///
/// The fields are public, so the value gate runs on every encode instead of
/// only at construction: a record mutated into a foreign dimension, a
/// non-finite pose or an out-of-range health after construction is refused
/// rather than silently published. The identity is the checked domain
/// `PassiveId`, so a zero identity cannot be constructed at all.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PassiveSpawnRecord {
    pub id: PassiveId,
    pub dimension: Dimension,
    pub position: [f32; 3],
    pub yaw: f32,
    pub health: u8,
}

impl PassiveSpawnRecord {
    /// The record's value gate, in the Go validator's order.
    fn valid(&self) -> Result<(), ProtocolError> {
        if self.dimension != Dimension::OVERWORLD {
            return Err(ProtocolError::InvalidEnum);
        }
        if !self.position.iter().all(|value| value.is_finite()) || !self.yaw.is_finite() {
            return Err(ProtocolError::InvalidFloat);
        }
        if self.health == 0 || self.health > super::combat_hit::MAX_HEALTH {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }
}

/// Play PassiveSpawn payload: the passive mob bodies that entered the
/// session's subscribed chunks, strictly ascending by ID.
#[derive(Clone, Debug, PartialEq)]
pub struct PassiveSpawn {
    pub server_tick: u64,
    pub spawns: Vec<PassiveSpawnRecord>,
}

impl PassiveSpawn {
    pub const PACKET_ID: u32 = 26;

    /// Builds a validated batch.
    ///
    /// The record ceiling is the protocol budget the decoder accepts. The
    /// authority converges on a smaller live capacity of passive actors, which
    /// is a separate concern this codec deliberately does not enforce: the
    /// 64-record wire bound is the protocol budget, and the 32-actor live cap
    /// never enters the packet layer.
    pub fn new(server_tick: u64, spawns: Vec<PassiveSpawnRecord>) -> Result<Self, ProtocolError> {
        let batch = Self {
            server_tick,
            spawns,
        };
        batch.valid()?;
        Ok(batch)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate order is the Go `PassiveSpawn.Validate` order: the batch count
    /// bound, then per record the record gate with the strict identity order
    /// checked against the previous record. A batch mutated into an empty or
    /// over-full record set, an unordered identity pair or an invalid record
    /// after construction is refused instead of silently published.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.spawns.is_empty() || self.spawns.len() > MAX_PASSIVE_SPAWN_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        let mut previous: Option<PassiveId> = None;
        for record in &self.spawns {
            record.valid()?;
            if previous.is_some_and(|last| last >= record.id) {
                return Err(ProtocolError::InvalidRange);
            }
            previous = Some(record.id);
        }
        Ok(())
    }

    /// The exact encoded length: the server tick, the one-byte count and one
    /// fixed stride per record.
    ///
    /// The value gate runs first, so an invalid batch reports its count,
    /// record or order error here instead of reaching a size or capacity
    /// decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        let records = self
            .spawns
            .len()
            .checked_mul(PASSIVE_SPAWN_WIRE_BYTES)
            .ok_or(ProtocolError::Allocation)?;
        BATCH_HEADER_WIRE_BYTES
            .checked_add(records)
            .ok_or(ProtocolError::Allocation)
    }

    /// Publishes the batch into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the server tick, the one-byte
    /// count and the ordered records — and the destination is tested before
    /// the first byte is written, so a short or invalid call leaves every
    /// destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.server_tick);
            writer.u8(self.spawns.len() as u8);
            for record in &self.spawns {
                passive_id::write_into(record.id, writer);
                writer.i32(i32::from(record.dimension.get()));
                for value in record.position {
                    writer.f32(value);
                }
                writer.f32(record.yaw);
                writer.u8(record.health);
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
        let batch = ByteCountBatch::read(&mut decoder, MAX_PASSIVE_SPAWN_RECORDS)?;
        // The exact-remaining-length rule runs before any record is read, so
        // a short and a padded payload fail at the same boundary the Go
        // decoder answers with its remaining-length message.
        batch.require_records(&decoder, PASSIVE_SPAWN_WIRE_BYTES)?;
        let mut spawns = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = passive_id::read(&mut decoder)?;
            // The raw `i32` is matched against the known dimension IDs instead
            // of being narrowed to a `u8`: a value such as 256 fits the byte
            // port but is not a known dimension, and reinterpreting it would
            // publish a record the Go validator refuses.
            let raw_dimension = decoder.i32()?;
            let dimension = match raw_dimension {
                0 => Dimension::OVERWORLD,
                1 => Dimension::DEPTHS,
                _ => return Err(ProtocolError::InvalidEnum),
            };
            let mut position = [0f32; 3];
            for value in &mut position {
                *value = decoder.f32()?;
            }
            let yaw = decoder.f32()?;
            let health = decoder.u8()?;
            spawns.push(PassiveSpawnRecord {
                id,
                dimension,
                position,
                yaw,
                health,
            });
        }
        decoder.done()?;
        Self::new(batch.server_tick, spawns)
    }
}
