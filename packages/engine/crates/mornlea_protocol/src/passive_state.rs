//! The bounded passive mob state batch published every tick.
//!
//! The batch carries the same one-byte record count and the same
//! exact-remaining-length rule as the spawn and despawn batches. Its record is
//! the spawn record with the dimension exchanged for the velocity: a dimension
//! change always goes through a despawn/spawn pair, so the mirror already
//! holds the dimension from the spawn and the state record carries the motion
//! instead. The transient grazing bit closes the record, because a passive mob
//! has no category to publish either.
//!
//! The record gate keeps the Go `PassiveStateRecord.validate` order: the
//! finiteness of the position, the velocity and the yaw as one message, the
//! health span, and the closed grazing match. The identity is already checked
//! by the domain `PassiveId`, so no identity rule is restated here.

use crate::batch::ByteCountBatch;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::passive_despawn::MAX_PASSIVE_RECORDS;
use crate::passive_id::{self, PASSIVE_ID_WIRE_BYTES, PassiveId};
use crate::server_hello::publish_packet;

/// Maximum passive state records one payload may carry, the same protocol
/// budget the spawn and despawn batches publish.
pub const MAX_PASSIVE_STATE_RECORDS: u8 = MAX_PASSIVE_RECORDS;

/// Fixed stride of one passive state record: the identity, the position, the
/// velocity, the yaw, the health and the grazing bit. The dimension is not on
/// the wire.
pub const PASSIVE_STATE_WIRE_BYTES: usize = PASSIVE_ID_WIRE_BYTES + 12 + 12 + 4 + 1 + 1;

/// Fixed wire upper bound of the whole payload.
pub const PASSIVE_STATE_MAX_WIRE_BYTES: usize =
    9 + MAX_PASSIVE_STATE_RECORDS as usize * PASSIVE_STATE_WIRE_BYTES;

/// Fixed header stride before the records: the eight-byte server tick and the
/// one-byte record count.
const BATCH_HEADER_WIRE_BYTES: usize = 9;

/// One passive state record: a nonzero identity, finite position and velocity,
/// health inside `1..=MAX_HEALTH`, and a grazing bit inside the closed 0/1
/// pair.
///
/// The fields are public, so the value gate runs on every encode instead of
/// only at construction, and a record mutated into a non-finite motion, an
/// out-of-range health or a grazing byte above one after construction is
/// refused rather than silently published.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PassiveStateRecord {
    pub id: PassiveId,
    pub position: [f32; 3],
    pub velocity: [f32; 3],
    pub yaw: f32,
    pub health: u8,
    pub grazing: u8,
}

impl PassiveStateRecord {
    /// The record's value gate, in the Go validator's order.
    fn valid(&self) -> Result<(), ProtocolError> {
        if !self
            .position
            .iter()
            .chain(self.velocity.iter())
            .all(|value| value.is_finite())
            || !self.yaw.is_finite()
        {
            return Err(ProtocolError::InvalidFloat);
        }
        if self.health == 0 || self.health > super::combat_hit::MAX_HEALTH {
            return Err(ProtocolError::InvalidRange);
        }
        if self.grazing > 1 {
            return Err(ProtocolError::InvalidEnum);
        }
        Ok(())
    }
}

/// Play PassiveState payload: the passive mob bodies of one tick, strictly
/// ascending by ID. Dimension is not carried: a dimension change always goes
/// through a despawn and spawn pair.
#[derive(Clone, Debug, PartialEq)]
pub struct PassiveState {
    pub server_tick: u64,
    pub states: Vec<PassiveStateRecord>,
}

impl PassiveState {
    pub const PACKET_ID: u32 = 27;

    /// Builds a validated batch.
    ///
    /// The record ceiling is the protocol budget the decoder accepts; the
    /// authority's smaller live capacity of passive actors is a separate
    /// concern this codec deliberately does not enforce.
    pub fn new(server_tick: u64, states: Vec<PassiveStateRecord>) -> Result<Self, ProtocolError> {
        let batch = Self {
            server_tick,
            states,
        };
        batch.valid()?;
        Ok(batch)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate order is the Go `PassiveState.Validate` order: the batch count
    /// bound, then per record the record gate with the strict identity order
    /// checked against the previous record.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.states.is_empty() || self.states.len() > MAX_PASSIVE_STATE_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        let mut previous: Option<PassiveId> = None;
        for record in &self.states {
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
            .states
            .len()
            .checked_mul(PASSIVE_STATE_WIRE_BYTES)
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
            writer.u8(self.states.len() as u8);
            for record in &self.states {
                passive_id::write_into(record.id, writer);
                for value in record.position {
                    writer.f32(value);
                }
                for value in record.velocity {
                    writer.f32(value);
                }
                writer.f32(record.yaw);
                writer.u8(record.health);
                writer.u8(record.grazing);
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
        let batch = ByteCountBatch::read(&mut decoder, MAX_PASSIVE_STATE_RECORDS)?;
        // The exact-remaining-length rule runs before any record is read,
        // exactly as the spawn and despawn batches apply it.
        batch.require_records(&decoder, PASSIVE_STATE_WIRE_BYTES)?;
        let mut states = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = passive_id::read(&mut decoder)?;
            let mut position = [0f32; 3];
            for value in &mut position {
                *value = decoder.f32()?;
            }
            let mut velocity = [0f32; 3];
            for value in &mut velocity {
                *value = decoder.f32()?;
            }
            let yaw = decoder.f32()?;
            let health = decoder.u8()?;
            let grazing = decoder.u8()?;
            states.push(PassiveStateRecord {
                id,
                position,
                velocity,
                yaw,
                health,
                grazing,
            });
        }
        decoder.done()?;
        Self::new(batch.server_tick, states)
    }
}
