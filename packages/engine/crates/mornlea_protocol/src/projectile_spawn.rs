//! The projectile spawn payload that publishes a projectile's flight state.
//!
//! The record carries a kind byte because a projectile's behaviour differs by
//! what fired it, but unlike a mob it has no yaw or health: a projectile is a
//! point-like transient entity whose orientation the client derives from its
//! velocity. Both playable dimensions are legal because a player's bow works
//! in either one, while the kind-by-dimension policy is an authority concern
//! this codec deliberately does not enforce — a shard in the depths and an
//! arrow in the overworld are both publishable records at this boundary.
//!
//! The order is the raw numeric `u64` identity, held here by the checked domain
//! `ProjectileId`, and the count is bounded before the records are read. The
//! batch applies the exact-remaining-length rule, not the minimum-records rule,
//! because the Go decoder rejects a payload whose remaining length is not
//! exactly `count` records before it reads one, so a short and a padded payload
//! both answer at the truncation boundary.
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
use mornlea_domain::Dimension;

/// Fixed stride of one projectile spawn record: `u64` ID, `u8` kind, `i32`
/// dimension, three `f32` position components, and three `f32` velocity
/// components.
pub const PROJECTILE_SPAWN_WIRE_BYTES: usize = PROJECTILE_ID_WIRE_BYTES + 1 + 4 + 12 + 12;

/// Fixed wire upper bound of the whole payload, derived from the stride and the
/// record bound the same way the Go declaration derives its maximum.
pub const PROJECTILE_SPAWN_MAX_WIRE_BYTES: usize =
    9 + MAX_PROJECTILE_RECORDS as usize * PROJECTILE_SPAWN_WIRE_BYTES;

/// Projectile kinds on the wire: a bone shard thrown by a ranged hostile and
/// an arrow loosed by a player's bow.
pub const PROJECTILE_KIND_SHARD: u8 = 0;
pub const PROJECTILE_KIND_ARROW: u8 = 1;

/// Fixed header stride before the records: the eight-byte server tick and the
/// one-byte record count.
const BATCH_HEADER_WIRE_BYTES: usize = 9;

/// One projectile birth fact: a non-zero ID, a known kind, a reachable
/// dimension, and a finite pose and velocity.
///
/// The fields are public, so the value gate runs on every encode instead of
/// only at construction: a record mutated into an unknown kind or a non-finite
/// component after construction is refused rather than silently published. The
/// identity is the checked domain `ProjectileId`, so a zero identity cannot be
/// constructed at all.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ProjectileSpawnRecord {
    pub id: ProjectileId,
    pub kind: u8,
    pub dimension: Dimension,
    pub position: [f32; 3],
    pub velocity: [f32; 3],
}

impl ProjectileSpawnRecord {
    /// The record's value gate, in the Go validator's order.
    ///
    /// The identity is already checked by `ProjectileId`, so the record gate
    /// restates no identity rule; the kind and the dimension are separate
    /// closed wire sets whose combination this layer does not constrain; and
    /// the pose and the velocity share one finiteness check.
    fn valid(&self) -> Result<(), ProtocolError> {
        if self.kind != PROJECTILE_KIND_SHARD && self.kind != PROJECTILE_KIND_ARROW {
            return Err(ProtocolError::InvalidEnum);
        }
        if !self.position.iter().all(|value| value.is_finite())
            || !self.velocity.iter().all(|value| value.is_finite())
        {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(())
    }
}

/// Play ProjectileSpawn payload: the projectiles that entered the session's
/// subscribed chunks, strictly ascending by ID.
#[derive(Clone, Debug, PartialEq)]
pub struct ProjectileSpawn {
    pub server_tick: u64,
    pub spawns: Vec<ProjectileSpawnRecord>,
}

impl ProjectileSpawn {
    pub const PACKET_ID: u32 = 29;

    /// Builds a validated batch.
    ///
    /// The gate order is the Go `ProjectileSpawn.Validate` order: the batch
    /// count bound, then each record, then the strictly increasing identity
    /// order.
    pub fn new(
        server_tick: u64,
        spawns: Vec<ProjectileSpawnRecord>,
    ) -> Result<Self, ProtocolError> {
        let batch = Self {
            server_tick,
            spawns,
        };
        batch.valid()?;
        Ok(batch)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.spawns.is_empty() || self.spawns.len() > MAX_PROJECTILE_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        for (index, record) in self.spawns.iter().enumerate() {
            record.valid()?;
            if index > 0 && self.spawns[index - 1].id >= record.id {
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
            .spawns
            .len()
            .checked_mul(PROJECTILE_SPAWN_WIRE_BYTES)
            .ok_or(ProtocolError::Allocation)?;
        BATCH_HEADER_WIRE_BYTES
            .checked_add(records)
            .ok_or(ProtocolError::Allocation)
    }

    /// Publishes the batch into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the server tick, the one-byte
    /// count and, per record, the identity, the kind, the dimension, the
    /// position and the velocity — and the destination is tested before the
    /// first byte is written, so a short or invalid call leaves every
    /// destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.server_tick);
            writer.u8(self.spawns.len() as u8);
            for record in &self.spawns {
                projectile_id::write_into(record.id, writer);
                writer.u8(record.kind);
                writer.i32(i32::from(record.dimension.get()));
                for value in record.position {
                    writer.f32(value);
                }
                for value in record.velocity {
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
        if payload.len() > PROJECTILE_SPAWN_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let mut decoder = ByteDecoder::new(payload);
        let batch = ByteCountBatch::read(&mut decoder, MAX_PROJECTILE_RECORDS)?;
        // The exact-remaining-length rule runs before any record is read, so a
        // short and a padded payload both answer at the truncation boundary
        // the Go decoder publishes.
        batch.require_records(&decoder, PROJECTILE_SPAWN_WIRE_BYTES)?;
        let mut spawns = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = projectile_id::read(&mut decoder)?;
            let kind = decoder.u8()?;
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
            let mut velocity = [0f32; 3];
            for value in &mut velocity {
                *value = decoder.f32()?;
            }
            spawns.push(ProjectileSpawnRecord {
                id,
                kind,
                dimension,
                position,
                velocity,
            });
        }
        decoder.done()?;
        Self::new(batch.server_tick, spawns)
    }
}
