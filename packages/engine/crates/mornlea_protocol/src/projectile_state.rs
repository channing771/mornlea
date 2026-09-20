//! The bounded projectile state batch published every tick.
//!
//! The state record is the narrowest record in the crate: an identity and a
//! position. A projectile's kind, dimension, and velocity are fixed for its
//! whole life, so the client mirror records them at spawn and the state batch
//! only moves the body.

use crate::batch::{ByteCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::projectile_despawn::MAX_PROJECTILE_RECORDS;

/// Fixed stride of one projectile state record: `u64` ID and three `f32`
/// position components.
pub const PROJECTILE_STATE_WIRE_BYTES: usize = 8 + 12;

/// Fixed wire upper bound of the whole payload.
pub const PROJECTILE_STATE_MAX_WIRE_BYTES: usize =
    9 + MAX_PROJECTILE_RECORDS as usize * PROJECTILE_STATE_WIRE_BYTES;

/// One projectile body state for one tick.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ProjectileStateRecord {
    pub id: u64,
    pub position: [f32; 3],
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

    fn valid(record: &ProjectileStateRecord) -> Result<(), ProtocolError> {
        if record.id == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        if !record.position.iter().all(|value| value.is_finite()) {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(())
    }

    /// Builds a validated batch.
    pub fn new(
        server_tick: u64,
        states: Vec<ProjectileStateRecord>,
    ) -> Result<Self, ProtocolError> {
        if states.is_empty() || states.len() > MAX_PROJECTILE_RECORDS as usize {
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
        }
        encoder
            .finish()
            .expect("validated projectile state is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        if payload.len() > PROJECTILE_STATE_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let batch = ByteCountBatch::read(&mut decoder, MAX_PROJECTILE_RECORDS)?;
        batch.require_records(&decoder, PROJECTILE_STATE_WIRE_BYTES)?;
        let mut states = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = decoder.u64()?;
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
