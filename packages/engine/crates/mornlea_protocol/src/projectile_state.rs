//! The bounded projectile state batch published every tick.
//!
//! The state record is the narrowest record in the crate: an identity and a
//! position. A projectile's kind, dimension, and velocity are fixed for its
//! whole life, so the client mirror records them at spawn and the state batch
//! only moves the body. The batch therefore restates no kind or dimension
//! rule, and no velocity is inferred on this wire.
//!
//! The order is the raw numeric `u64` identity, held here by the checked domain
//! `ProjectileId`, and the count is bounded before the records are read. The
//! batch applies the exact-remaining-length rule, not the minimum-records
//! rule, because the Go decoder rejects a payload whose remaining length is
//! not exactly `count` records before it reads one, so a short and a padded
//! payload both answer at the truncation boundary.
//!
//! The payload also carries a fixed wire ceiling this decoder applies before it
//! reads a byte, because the Go decode path refuses an over-ceiling payload
//! before the family decoder runs. An over-ceiling payload therefore answers
//! the capacity boundary on both sides instead of the truncation boundary the
//! count and length rules would report.

use crate::batch::ByteCountBatch;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::projectile_despawn::MAX_PROJECTILE_RECORDS;
use crate::projectile_id::{self, PROJECTILE_ID_WIRE_BYTES, ProjectileId};
use crate::server_hello::publish_packet;

/// Fixed stride of one projectile state record: `u64` ID and three `f32`
/// position components.
pub const PROJECTILE_STATE_WIRE_BYTES: usize = PROJECTILE_ID_WIRE_BYTES + 12;

/// Fixed wire upper bound of the whole payload, derived from the stride and the
/// record bound the same way the Go declaration derives its maximum.
pub const PROJECTILE_STATE_MAX_WIRE_BYTES: usize =
    9 + MAX_PROJECTILE_RECORDS as usize * PROJECTILE_STATE_WIRE_BYTES;

/// Fixed header stride before the records: the eight-byte server tick and the
/// one-byte record count.
const BATCH_HEADER_WIRE_BYTES: usize = 9;

/// One projectile body state for one tick.
///
/// The fields are public, so the value gate runs on every encode instead of
/// only at construction: a record mutated into a non-finite position after
/// construction is refused rather than silently published. The identity is the
/// checked domain `ProjectileId`, so a zero identity cannot be constructed at
/// all.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ProjectileStateRecord {
    pub id: ProjectileId,
    pub position: [f32; 3],
}

impl ProjectileStateRecord {
    /// The record's value gate, in the Go validator's order.
    ///
    /// The identity is already checked by `ProjectileId`, so the record gate
    /// restates no identity rule and the position carries the one finiteness
    /// check the wire has.
    fn valid(&self) -> Result<(), ProtocolError> {
        if !self.position.iter().all(|value| value.is_finite()) {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(())
    }
}

/// Play ProjectileState payload: the projectile bodies of one tick, strictly
/// ascending by ID, for latest-wins mirroring and interpolation.
#[derive(Clone, Debug, PartialEq)]
pub struct ProjectileState {
    pub server_tick: u64,
    pub states: Vec<ProjectileStateRecord>,
}

impl ProjectileState {
    pub const PACKET_ID: u32 = 30;

    /// Builds a validated batch.
    ///
    /// The gate order is the Go `ProjectileState.Validate` order: the batch
    /// count bound, then each record, then the strictly increasing identity
    /// order.
    pub fn new(
        server_tick: u64,
        states: Vec<ProjectileStateRecord>,
    ) -> Result<Self, ProtocolError> {
        let batch = Self {
            server_tick,
            states,
        };
        batch.valid()?;
        Ok(batch)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.states.is_empty() || self.states.len() > MAX_PROJECTILE_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        for (index, record) in self.states.iter().enumerate() {
            record.valid()?;
            if index > 0 && self.states[index - 1].id >= record.id {
                return Err(ProtocolError::InvalidRange);
            }
        }
        Ok(())
    }

    /// The exact encoded length: the server tick, the one-byte count and one
    /// fixed record stride per record.
    ///
    /// The value gate runs first, so an invalid batch reports its count, record
    /// or order error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        let records = self
            .states
            .len()
            .checked_mul(PROJECTILE_STATE_WIRE_BYTES)
            .ok_or(ProtocolError::Allocation)?;
        BATCH_HEADER_WIRE_BYTES
            .checked_add(records)
            .ok_or(ProtocolError::Allocation)
    }

    /// Publishes the batch into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the server tick, the one-byte
    /// count and, per record, the identity and the position — and the
    /// destination is tested before the first byte is written, so a short or
    /// invalid call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.server_tick);
            writer.u8(self.states.len() as u8);
            for record in &self.states {
                projectile_id::write_into(record.id, writer);
                for value in record.position {
                    writer.f32(value);
                }
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
        // The fixed wire bound is a pre-allocation guard that runs before the
        // header is read: the Go decode path refuses an over-ceiling payload
        // with its fixed-maximum check before the family decoder runs, so an
        // over-ceiling payload answers the capacity boundary on both sides
        // rather than the truncation boundary the length rule would report.
        if payload.len() > PROJECTILE_STATE_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let mut decoder = ByteDecoder::new(payload);
        let batch = ByteCountBatch::read(&mut decoder, MAX_PROJECTILE_RECORDS)?;
        // The exact-remaining-length rule runs before any record is read, so a
        // short and a padded payload both answer at the truncation boundary
        // the Go decoder publishes.
        batch.require_records(&decoder, PROJECTILE_STATE_WIRE_BYTES)?;
        let mut states = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = projectile_id::read(&mut decoder)?;
            let mut position = [0f32; 3];
            for value in &mut position {
                *value = decoder.f32()?;
            }
            states.push(ProjectileStateRecord { id, position });
        }
        decoder.done()?;
        Self::new(batch.server_tick, states)
    }
}
