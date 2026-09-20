use crate::batch::{ByteCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::hostile_spawn::{HOSTILE_KIND_BONE_THROWER, HOSTILE_KIND_NIGHTWALKER};

/// Maximum hostile state records one payload may carry.
pub const HOSTILE_STATE_MAX_RECORDS: u8 = 64;

/// Fixed stride of one hostile state record.
pub const HOSTILE_STATE_WIRE_BYTES: usize = 8 + 12 + 12 + 4 + 1 + 1;

/// One hostile state record: a non-zero ID, finite position and velocity,
/// health inside `1..=MAX_HEALTH`, and a known kind. Dimension is not on the
/// wire because a dimension change always goes through a despawn/spawn pair.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct HostileStateRecord {
    pub id: u64,
    pub position: [f32; 3],
    pub velocity: [f32; 3],
    pub yaw: f32,
    pub health: u8,
    pub kind: u8,
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

    fn valid(record: &HostileStateRecord) -> Result<(), ProtocolError> {
        if record.id == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        if !record
            .position
            .iter()
            .chain(record.velocity.iter())
            .all(|value| value.is_finite())
            || !record.yaw.is_finite()
        {
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

    pub fn new(server_tick: u64, states: Vec<HostileStateRecord>) -> Result<Self, ProtocolError> {
        if states.is_empty() || states.len() > HOSTILE_STATE_MAX_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        for (index, record) in states.iter().enumerate() {
            Self::valid(record)?;
            if index > 0 && states[index - 1].id >= record.id {
                return Err(ProtocolError::InvalidRange);
            }
        }
        Ok(Self {
            server_tick,
            states,
        })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        ByteCountBatch::write(&mut encoder, self.server_tick, self.states.len() as u8);
        for record in &self.states {
            encoder.u64(record.id);
            for value in record.position {
                encoder.f32(value);
            }
            for value in record.velocity {
                encoder.f32(value);
            }
            encoder.f32(record.yaw);
            encoder.u8(record.health);
            encoder.u8(record.kind);
        }
        encoder
            .finish()
            .expect("validated hostile state is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let batch = ByteCountBatch::read(&mut decoder, HOSTILE_STATE_MAX_RECORDS)?;
        batch.require_records(&decoder, HOSTILE_STATE_WIRE_BYTES)?;
        let mut states = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = decoder.u64()?;
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
