//! The passive mob despawn batch that drops passive bodies from a mirror.
//!
//! The batch carries only identities and removal reasons, so its record is the
//! fixed 9-byte pair the spawn and state records start their identity with,
//! and it shares the byte-count header and the exact-remaining-length rule
//! with both of them. The batch therefore restates no identity rule: a zero
//! identity is refused where the identity is read, and only the count bound,
//! the closed reason match and the strict numeric order are left for this gate.
//!
//! The order comparison is over the typed identity rather than over the wire
//! bytes, which is the same total order the Go ascending batch applies, so a
//! duplicate and a descending pair are both refused.

use crate::batch::ByteCountBatch;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::passive_id::{self, PASSIVE_ID_WIRE_BYTES, PassiveId};
use crate::server_hello::publish_packet;

/// Maximum passive mob records one payload may carry, copied from the Go
/// `MaxPassiveRecords` pin.
pub const MAX_PASSIVE_RECORDS: u8 = 64;

/// Fixed stride of one passive despawn record: the identity plus the reason
/// byte.
pub const PASSIVE_DESPAWN_WIRE_BYTES: usize = PASSIVE_ID_WIRE_BYTES + 1;

/// Fixed wire upper bound of the whole payload.
pub const PASSIVE_DESPAWN_MAX_WIRE_BYTES: usize =
    9 + MAX_PASSIVE_RECORDS as usize * PASSIVE_DESPAWN_WIRE_BYTES;

/// Fixed header stride before the records: the eight-byte server tick and the
/// one-byte record count.
const BATCH_HEADER_WIRE_BYTES: usize = 9;

/// Removal reasons: the mob left the subscription range or it died.
pub const PASSIVE_DESPAWN_VANISHED: u8 = 0;
pub const PASSIVE_DESPAWN_DIED: u8 = 1;

/// One passive mob removal fact: a nonzero identity and one of the two
/// published removal reasons.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PassiveDespawnRecord {
    pub id: PassiveId,
    pub reason: u8,
}

impl PassiveDespawnRecord {
    /// The record's value gate, in the Go validator's order.
    ///
    /// The identity is already checked by the domain `PassiveId`, so only the
    /// closed reason match is left: the wire publishes exactly the vanished
    /// and died pair.
    fn valid(&self) -> Result<(), ProtocolError> {
        if self.reason != PASSIVE_DESPAWN_VANISHED && self.reason != PASSIVE_DESPAWN_DIED {
            return Err(ProtocolError::InvalidEnum);
        }
        Ok(())
    }
}

/// Play PassiveDespawn payload: the passive mob bodies a session must drop.
/// Records carry the ID and the removal reason, are strictly ascending by ID,
/// and a zero ID or an unknown reason is rejected.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PassiveDespawn {
    pub server_tick: u64,
    pub despawns: Vec<PassiveDespawnRecord>,
}

impl PassiveDespawn {
    pub const PACKET_ID: u32 = 28;

    /// Builds a validated batch.
    ///
    /// Identity validity is already enforced by the checked domain `PassiveId`,
    /// so this gate only checks the batch bounds, the reason pair and the
    /// order.
    pub fn new(
        server_tick: u64,
        despawns: Vec<PassiveDespawnRecord>,
    ) -> Result<Self, ProtocolError> {
        let batch = Self {
            server_tick,
            despawns,
        };
        batch.valid()?;
        Ok(batch)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate order is the Go `PassiveDespawn.Validate` order: the batch
    /// count bound, then per record the reason gate with the strict identity
    /// order checked against the previous record. A zero ID is refused where
    /// the identity is read, because `PassiveId` is a checked newtype whose
    /// zero form does not exist.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.despawns.is_empty() || self.despawns.len() > MAX_PASSIVE_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        let mut previous: Option<PassiveId> = None;
        for record in &self.despawns {
            record.valid()?;
            if previous.is_some_and(|last| last >= record.id) {
                return Err(ProtocolError::InvalidRange);
            }
            previous = Some(record.id);
        }
        Ok(())
    }

    /// The exact encoded length: the server tick, the one-byte count and one
    /// fixed record stride per record.
    ///
    /// The value gate runs first, so an invalid batch reports its count,
    /// record or order error here instead of reaching a size or capacity
    /// decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        let records = self
            .despawns
            .len()
            .checked_mul(PASSIVE_DESPAWN_WIRE_BYTES)
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
            writer.u8(self.despawns.len() as u8);
            for record in &self.despawns {
                passive_id::write_into(record.id, writer);
                writer.u8(record.reason);
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
        let batch = ByteCountBatch::read(&mut decoder, MAX_PASSIVE_RECORDS)?;
        // The exact-remaining-length rule runs before any record is read, so a
        // short and a padded payload both answer at the truncation boundary
        // the Go decoder publishes.
        batch.require_records(&decoder, PASSIVE_DESPAWN_WIRE_BYTES)?;
        let mut despawns = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = passive_id::read(&mut decoder)?;
            let reason = decoder.u8()?;
            despawns.push(PassiveDespawnRecord { id, reason });
        }
        decoder.done()?;
        Self::new(batch.server_tick, despawns)
    }
}
