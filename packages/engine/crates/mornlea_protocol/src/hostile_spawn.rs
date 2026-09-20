use crate::batch::{ByteCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use mornlea_domain::Dimension;

/// Maximum hostile spawn records one payload may carry.
pub const HOSTILE_SPAWN_MAX_RECORDS: u8 = 64;

/// Fixed stride of one hostile spawn record.
pub const HOSTILE_SPAWN_WIRE_BYTES: usize = 8 + 4 + 12 + 4 + 1 + 1;

/// Hostile kinds on the wire: nightwalker (`0`) and bone thrower (`1`).
pub const HOSTILE_KIND_NIGHTWALKER: u8 = 0;
pub const HOSTILE_KIND_BONE_THROWER: u8 = 1;

/// One hostile spawn record: a non-zero ID, an overworld dimension, a finite
/// pose, health inside `1..=MAX_HEALTH`, and a known kind.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct HostileSpawnRecord {
    pub id: u64,
    pub dimension: Dimension,
    pub position: [f32; 3],
    pub yaw: f32,
    pub health: u8,
    pub kind: u8,
}

/// Play HostileSpawn payload: the authoritative hostile bodies that entered
/// the session's subscribed chunks, strictly ascending by ID.
#[derive(Clone, Debug, PartialEq)]
pub struct HostileSpawn {
    pub server_tick: u64,
    pub spawns: Vec<HostileSpawnRecord>,
}

impl HostileSpawn {
    pub const PACKET_ID: u32 = 22;

    fn valid(record: &HostileSpawnRecord) -> Result<(), ProtocolError> {
        if record.id == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        if record.dimension != Dimension::OVERWORLD {
            return Err(ProtocolError::InvalidEnum);
        }
        if !record.position.iter().all(|value| value.is_finite()) || !record.yaw.is_finite() {
            return Err(ProtocolError::InvalidFloat);
        }
        if record.health == 0 || record.health > super::combat_hit::MAX_HEALTH {
            return Err(ProtocolError::InvalidRange);
        }
        if record.kind != HOSTILE_KIND_NIGHTWALKER && record.kind != HOSTILE_KIND_BONE_THROWER {
            return Err(ProtocolError::InvalidEnum);
        }
        Ok(())
    }

    pub fn new(server_tick: u64, spawns: Vec<HostileSpawnRecord>) -> Result<Self, ProtocolError> {
        if spawns.is_empty() || spawns.len() > HOSTILE_SPAWN_MAX_RECORDS as usize {
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
            encoder.u8(record.kind);
        }
        encoder
            .finish()
            .expect("validated hostile spawn is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let batch = ByteCountBatch::read(&mut decoder, HOSTILE_SPAWN_MAX_RECORDS)?;
        batch.require_records(&decoder, HOSTILE_SPAWN_WIRE_BYTES)?;
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
            let kind = decoder.u8()?;
            spawns.push(HostileSpawnRecord {
                id,
                dimension,
                position,
                yaw,
                health,
                kind,
            });
        }
        decoder.done()?;
        Self::new(batch.server_tick, spawns)
    }
}
