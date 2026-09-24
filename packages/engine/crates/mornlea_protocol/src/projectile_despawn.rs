//! The projectile despawn batch that removes projectile bodies from a mirror.
//!
//! The batch carries only identities, so its record is the fixed eight-byte
//! `ProjectileId` the spawn and state records start with, and it shares the
//! byte-count header and the exact-remaining-length rule with both of them.
//! The batch therefore restates no identity rule: a zero identity is refused
//! where the identity is read, and only the count bound and the strict numeric
//! order are left for this gate.
//!
//! The order comparison is over the typed identity rather than over the wire
//! bytes, which is the same total order the Go ascending batch applies, so a
//! duplicate and a descending pair are both refused.
//!
//! The payload also carries a fixed wire ceiling this decoder applies before it
//! reads a byte, because the Go decode path refuses an over-ceiling payload
//! before the family decoder runs. An over-ceiling payload therefore answers
//! the capacity boundary on both sides instead of the truncation boundary the
//! count and length rules would report.

use crate::batch::ByteCountBatch;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::projectile_id::{self, PROJECTILE_ID_WIRE_BYTES, ProjectileId};
use crate::server_hello::publish_packet;

/// Maximum projectile records one payload may carry, copied from the Go
/// `MaxProjectileRecords` pin.
pub const MAX_PROJECTILE_RECORDS: u8 = 128;

/// Fixed stride of one projectile despawn record: a little-endian `u64` ID.
pub const PROJECTILE_DESPAWN_WIRE_BYTES: usize = PROJECTILE_ID_WIRE_BYTES;

/// Fixed wire upper bound of the whole payload, derived from the stride and the
/// record bound the same way the Go declaration derives its maximum.
pub const PROJECTILE_DESPAWN_MAX_WIRE_BYTES: usize =
    9 + MAX_PROJECTILE_RECORDS as usize * PROJECTILE_DESPAWN_WIRE_BYTES;

/// Fixed header stride before the records: the eight-byte server tick and the
/// one-byte record count.
const BATCH_HEADER_WIRE_BYTES: usize = 9;

/// Play ProjectileDespawn payload: the authoritative projectile bodies a
/// session must drop. Records carry only the ID, are strictly ascending, and
/// a zero ID is never a live projectile.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ProjectileDespawn {
    pub server_tick: u64,
    pub ids: Vec<ProjectileId>,
}

impl ProjectileDespawn {
    pub const PACKET_ID: u32 = 31;

    /// Builds a validated batch.
    ///
    /// Identity validity is already enforced by the checked domain
    /// `ProjectileId`, so this gate only checks the batch bounds and the order.
    pub fn new(server_tick: u64, ids: Vec<ProjectileId>) -> Result<Self, ProtocolError> {
        let batch = Self { server_tick, ids };
        batch.valid()?;
        Ok(batch)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate order is the Go `ProjectileDespawn.Validate` order: the batch
    /// count bound, then the strictly increasing identity order. A zero ID is
    /// refused where the identity is read, because `ProjectileId` is a checked
    /// newtype whose zero form does not exist.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.ids.is_empty() || self.ids.len() > MAX_PROJECTILE_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        if !crate::batch::strictly_increasing(&self.ids) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The exact encoded length: the server tick, the one-byte count and one
    /// fixed identity stride per record.
    ///
    /// The value gate runs first, so an invalid batch reports its count or
    /// order error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        let records = self
            .ids
            .len()
            .checked_mul(PROJECTILE_DESPAWN_WIRE_BYTES)
            .ok_or(ProtocolError::Allocation)?;
        BATCH_HEADER_WIRE_BYTES
            .checked_add(records)
            .ok_or(ProtocolError::Allocation)
    }

    /// Publishes the batch into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the server tick, the one-byte
    /// count and the ordered identities — and the destination is tested before
    /// the first byte is written, so a short or invalid call leaves every
    /// destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.server_tick);
            writer.u8(self.ids.len() as u8);
            for id in &self.ids {
                projectile_id::write_into(*id, writer);
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
        if payload.len() > PROJECTILE_DESPAWN_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let mut decoder = ByteDecoder::new(payload);
        let batch = ByteCountBatch::read(&mut decoder, MAX_PROJECTILE_RECORDS)?;
        // The exact-remaining-length rule runs before any record is read, so a
        // short and a padded payload both answer at the truncation boundary
        // the Go decoder publishes.
        batch.require_records(&decoder, PROJECTILE_DESPAWN_WIRE_BYTES)?;
        let mut ids = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            ids.push(projectile_id::read(&mut decoder)?);
        }
        decoder.done()?;
        Self::new(batch.server_tick, ids)
    }
}
