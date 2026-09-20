//! The projectile spawn payload that publishes a projectile's flight state.
//!
//! The record carries a kind byte because a projectile's behaviour differs by
//! what fired it, but unlike a mob it has no yaw or health: a projectile is a
//! point-like transient entity whose orientation the client derives from its
//! velocity. Both playable dimensions are legal because a player's bow works
//! in either one, while the kind-by-dimension policy is an authority concern
//! this codec deliberately does not enforce.

use crate::batch::{ByteCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::projectile_despawn::MAX_PROJECTILE_RECORDS;
use mornlea_domain::Dimension;

/// Fixed stride of one projectile spawn record: `u64` ID, `u8` kind, `i32`
/// dimension, three `f32` position components, and three `f32` velocity
/// components.
pub const PROJECTILE_SPAWN_WIRE_BYTES: usize = 8 + 1 + 4 + 12 + 12;

/// Fixed wire upper bound of the whole payload.
pub const PROJECTILE_SPAWN_MAX_WIRE_BYTES: usize =
    9 + MAX_PROJECTILE_RECORDS as usize * PROJECTILE_SPAWN_WIRE_BYTES;

/// Projectile kinds on the wire: a bone shard thrown by a ranged hostile and
/// an arrow loosed by a player's bow.
pub const PROJECTILE_KIND_SHARD: u8 = 0;
pub const PROJECTILE_KIND_ARROW: u8 = 1;

/// One projectile birth fact: a non-zero ID, a known kind, a reachable
/// dimension, and a finite pose and velocity.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ProjectileSpawnRecord {
    pub id: u64,
    pub kind: u8,
    pub dimension: Dimension,
    pub position: [f32; 3],
    pub velocity: [f32; 3],
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

    fn valid(record: &ProjectileSpawnRecord) -> Result<(), ProtocolError> {
        if record.id == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        if record.kind != PROJECTILE_KIND_SHARD && record.kind != PROJECTILE_KIND_ARROW {
            return Err(ProtocolError::InvalidEnum);
        }
        if !record.position.iter().all(|value| value.is_finite())
            || !record.velocity.iter().all(|value| value.is_finite())
        {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(())
    }

    /// Builds a validated batch.
    pub fn new(
        server_tick: u64,
        spawns: Vec<ProjectileSpawnRecord>,
    ) -> Result<Self, ProtocolError> {
        if spawns.is_empty() || spawns.len() > MAX_PROJECTILE_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        for (index, record) in spawns.iter().enumerate() {
            Self::valid(record)?;
            if index > 0 && spawns[index - 1].id >= record.id {
                return Err(ProtocolError::InvalidRange);
            }
        }
        Ok(Self {
            server_tick,
            spawns,
        })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        ByteCountBatch::write(&mut encoder, self.server_tick, self.spawns.len() as u8);
        for record in &self.spawns {
            encoder.u64(record.id);
            encoder.u8(record.kind);
            encoder.i32(i32::from(record.dimension.get()));
            for value in record.position {
                encoder.f32(value);
            }
            for value in record.velocity {
                encoder.f32(value);
            }
        }
        encoder
            .finish()
            .expect("validated projectile spawn is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        if payload.len() > PROJECTILE_SPAWN_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let batch = ByteCountBatch::read(&mut decoder, MAX_PROJECTILE_RECORDS)?;
        batch.require_records(&decoder, PROJECTILE_SPAWN_WIRE_BYTES)?;
        let mut spawns = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = decoder.u64()?;
            let kind = decoder.u8()?;
            let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
                .map_err(|_| ProtocolError::InvalidEnum)?;
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
