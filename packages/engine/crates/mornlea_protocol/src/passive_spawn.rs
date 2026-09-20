//! The passive mob spawn payload that publishes a passive body.
//!
//! The record has the same field face as a hostile spawn record minus the
//! kind byte: a passive mob has no category to publish because it is the only
//! passive kind. The health field is therefore the trailing byte of the fixed
//! 29-byte stride.

use crate::batch::{ByteCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::combat_hit::MAX_HEALTH;
use crate::error::ProtocolError;
use mornlea_domain::Dimension;

/// Maximum passive spawn records one payload may carry, copied from the Go
/// `MaxPassiveRecords` pin.
pub const MAX_PASSIVE_SPAWN_RECORDS: u8 = 64;

/// Fixed stride of one passive spawn record: `u64` ID, `i32` dimension, three
/// `f32` position components, `f32` yaw, and a `u8` health.
pub const PASSIVE_SPAWN_WIRE_BYTES: usize = 8 + 4 + 12 + 4 + 1;

/// Fixed wire upper bound of the whole payload.
pub const PASSIVE_SPAWN_MAX_WIRE_BYTES: usize = 9 + MAX_PASSIVE_SPAWN_RECORDS as usize * 29;

/// One passive mob birth fact: a non-zero ID, an overworld dimension, a finite
/// pose, and health inside `1..=MAX_HEALTH`.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PassiveSpawnRecord {
    pub id: u64,
    pub dimension: Dimension,
    pub position: [f32; 3],
    pub yaw: f32,
    pub health: u8,
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

    fn valid(record: &PassiveSpawnRecord) -> Result<(), ProtocolError> {
        if record.id == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        if record.dimension != Dimension::OVERWORLD {
            return Err(ProtocolError::InvalidEnum);
        }
        if !record.position.iter().all(|value| value.is_finite()) || !record.yaw.is_finite() {
            return Err(ProtocolError::InvalidFloat);
        }
        if record.health == 0 || record.health > MAX_HEALTH {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// Builds a validated batch.
    ///
    /// The record ceiling is the protocol budget the decoder accepts; the
    /// authority converges on a smaller live capacity, which is a separate
    /// concern this codec deliberately does not enforce.
    pub fn new(server_tick: u64, spawns: Vec<PassiveSpawnRecord>) -> Result<Self, ProtocolError> {
        if spawns.is_empty() || spawns.len() > MAX_PASSIVE_SPAWN_RECORDS as usize {
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
            encoder.i32(i32::from(record.dimension.get()));
            for value in record.position {
                encoder.f32(value);
            }
            encoder.f32(record.yaw);
            encoder.u8(record.health);
        }
        encoder
            .finish()
            .expect("validated passive spawn is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        if payload.len() > PASSIVE_SPAWN_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let batch = ByteCountBatch::read(&mut decoder, MAX_PASSIVE_SPAWN_RECORDS)?;
        batch.require_records(&decoder, PASSIVE_SPAWN_WIRE_BYTES)?;
        let mut spawns = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = decoder.u64()?;
            let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
                .map_err(|_| ProtocolError::InvalidEnum)?;
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
