//! The hostile state batch that publishes one tick of hostile bodies.
//!
//! The batch carries the same one-byte record count and the same
//! exact-remaining-length rule as the spawn and despawn batches. Its record is
//! the spawn record with the dimension exchanged for the velocity: a dimension
//! change always goes through a despawn/spawn pair, so the mirror already
//! holds the dimension from the spawn and the state record carries the motion
//! instead. The record stride is therefore 38 bytes rather than the spawn's 30.
//!
//! The record gate keeps the Go `HostileStateRecord.validate` order: the
//! identity, the finiteness of the position, the velocity and the yaw, the
//! health span and the closed kind match. The identity is already checked by
//! the domain `HostileId`, so no identity rule is restated here.

use crate::batch::ByteCountBatch;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::hostile_id::{self, HOSTILE_ID_WIRE_BYTES, HostileId};
use crate::hostile_spawn::{HOSTILE_KIND_BONE_THROWER, HOSTILE_KIND_NIGHTWALKER};
use crate::server_hello::publish_packet;

/// Maximum hostile state records one payload may carry.
pub const HOSTILE_STATE_MAX_RECORDS: u8 = 64;

/// Fixed stride of one hostile state record: the identity, the position, the
/// velocity, the yaw, the health and the kind. The dimension is not on the
/// wire.
pub const HOSTILE_STATE_WIRE_BYTES: usize = HOSTILE_ID_WIRE_BYTES + 12 + 12 + 4 + 1 + 1;

/// Fixed header stride before the records: the eight-byte server tick and the
/// one-byte record count.
const BATCH_HEADER_WIRE_BYTES: usize = 9;

/// One hostile state record: a nonzero identity, finite position and velocity,
/// health inside `1..=MAX_HEALTH`, and a known kind.
///
/// The fields are public, so the value gate runs on every encode instead of
/// only at construction, and a record mutated into a non-finite motion, an
/// out-of-range health or an unknown kind after construction is refused rather
/// than silently published.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct HostileStateRecord {
    pub id: HostileId,
    pub position: [f32; 3],
    pub velocity: [f32; 3],
    pub yaw: f32,
    pub health: u8,
    pub kind: u8,
}

impl HostileStateRecord {
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
        if self.kind != HOSTILE_KIND_NIGHTWALKER && self.kind != HOSTILE_KIND_BONE_THROWER {
            return Err(ProtocolError::InvalidEnum);
        }
        Ok(())
    }
}

/// Play HostileState payload: the authoritative hostile bodies of one tick,
/// strictly ascending by ID.
#[derive(Clone, Debug, PartialEq)]
pub struct HostileState {
    pub server_tick: u64,
    pub states: Vec<HostileStateRecord>,
}

impl HostileState {
    pub const PACKET_ID: u32 = 23;

    /// Builds a validated batch.
    pub fn new(server_tick: u64, states: Vec<HostileStateRecord>) -> Result<Self, ProtocolError> {
        let batch = Self {
            server_tick,
            states,
        };
        batch.valid()?;
        Ok(batch)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate order is the Go `HostileState.Validate` order: the batch count
    /// bound, then per record the record gate with the strict identity order
    /// checked against the previous record.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.states.is_empty() || self.states.len() > HOSTILE_STATE_MAX_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        let mut previous: Option<HostileId> = None;
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
            .checked_mul(HOSTILE_STATE_WIRE_BYTES)
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
                hostile_id::write_into(record.id, writer);
                for value in record.position {
                    writer.f32(value);
                }
                for value in record.velocity {
                    writer.f32(value);
                }
                writer.f32(record.yaw);
                writer.u8(record.health);
                writer.u8(record.kind);
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
        let batch = ByteCountBatch::read(&mut decoder, HOSTILE_STATE_MAX_RECORDS)?;
        // The exact-remaining-length rule runs before any record is read,
        // exactly as the spawn and despawn batches apply it.
        batch.require_records(&decoder, HOSTILE_STATE_WIRE_BYTES)?;
        let mut states = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = hostile_id::read(&mut decoder)?;
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
            let kind = decoder.u8()?;
            states.push(HostileStateRecord {
                id,
                position,
                velocity,
                yaw,
                health,
                kind,
            });
        }
        decoder.done()?;
        Self::new(batch.server_tick, states)
    }
}
