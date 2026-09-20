//! The bounded passive mob state batch published every tick.
//!
//! The state record has the same face as a hostile state record plus the
//! transient grazing bit. Grazing is a presentation-only observation of an
//! authoritative transient, so it is validated as a 0/1 value and never
//! persisted or reinterpreted by this codec.

use crate::batch::{ByteCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::combat_hit::MAX_HEALTH;
use crate::error::ProtocolError;
use crate::passive_despawn::MAX_PASSIVE_RECORDS;

/// Fixed stride of one passive state record: `u64` ID, three `f32` position
/// components, three `f32` velocity components, `f32` yaw, a `u8` health, and
/// a `u8` grazing bit.
pub const PASSIVE_STATE_WIRE_BYTES: usize = 8 + 12 + 12 + 4 + 1 + 1;

/// Fixed wire upper bound of the whole payload.
pub const PASSIVE_STATE_MAX_WIRE_BYTES: usize = 9 + MAX_PASSIVE_RECORDS as usize * 38;

/// One passive mob body state for one tick.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PassiveStateRecord {
    pub id: u64,
    pub position: [f32; 3],
    pub velocity: [f32; 3],
    pub yaw: f32,
    pub health: u8,
    pub grazing: u8,
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

    fn valid(record: &PassiveStateRecord) -> Result<(), ProtocolError> {
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
        if record.health == 0 || record.health > MAX_HEALTH {
            return Err(ProtocolError::InvalidRange);
        }
        if record.grazing > 1 {
            return Err(ProtocolError::InvalidEnum);
        }
        Ok(())
    }

    /// Builds a validated batch.
    pub fn new(server_tick: u64, states: Vec<PassiveStateRecord>) -> Result<Self, ProtocolError> {
        if states.is_empty() || states.len() > MAX_PASSIVE_RECORDS as usize {
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
            encoder.u8(record.grazing);
        }
        encoder
            .finish()
            .expect("validated passive state is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        if payload.len() > PASSIVE_STATE_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let batch = ByteCountBatch::read(&mut decoder, MAX_PASSIVE_RECORDS)?;
        batch.require_records(&decoder, PASSIVE_STATE_WIRE_BYTES)?;
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
