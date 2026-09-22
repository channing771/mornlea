//! The bounded companion state batch published every tick.
//!
//! The batch reuses the shared uvarint-count header and the shared
//! unsigned-byte identity ordering so the companion batch, the hostile batch,
//! and the remote-player batch cannot drift apart in their count bounds or
//! their sort rule.

use crate::batch::{UvarintCountBatch, strictly_increasing_ids};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::entity_id::CompanionId;
use crate::error::ProtocolError;
use mornlea_domain::Dimension;

/// Maximum companion states one payload may carry, copied from the Go
/// `MaxCompanionStates` pin that is itself sourced from the domain limit.
pub const MAX_COMPANION_STATES: u32 = 4;

/// Fixed stride of one companion state record: 16-byte identity, `i32`
/// dimension, three `f32` position components, `f32` yaw, `f32` pitch, and a
/// reset boolean.
pub const COMPANION_STATE_WIRE_BYTES: usize = 16 + 4 + 12 + 4 + 4 + 1;

/// Fixed wire upper bound of the whole payload.
pub const COMPANION_STATES_MAX_WIRE_BYTES: usize =
    8 + 1 + MAX_COMPANION_STATES as usize * COMPANION_STATE_WIRE_BYTES;

/// Half turn in radians, the pitch ceiling of a companion pose.
const PITCH_LIMIT: f32 = std::f32::consts::FRAC_PI_2;

/// One companion body state for one tick.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct CompanionState {
    pub companion_id: CompanionId,
    pub dimension: Dimension,
    pub position: [f32; 3],
    pub yaw: f32,
    pub pitch: f32,
    pub reset: bool,
}

/// Play CompanionStates payload: the companion bodies of one tick, strictly
/// ascending by identity.
#[derive(Clone, Debug, PartialEq)]
pub struct CompanionStates {
    pub tick: u64,
    pub states: Vec<CompanionState>,
}

impl CompanionStates {
    pub const PACKET_ID: u32 = 18;

    fn valid(record: &CompanionState) -> Result<(), ProtocolError> {
        if record.dimension != Dimension::OVERWORLD {
            return Err(ProtocolError::InvalidEnum);
        }
        if !record.position.iter().all(|value| value.is_finite())
            || !record.yaw.is_finite()
            || !record.pitch.is_finite()
            || record.pitch.abs() > PITCH_LIMIT
        {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(())
    }

    /// Builds a validated batch.
    ///
    /// The name is not carried by this family, so the companion name rule is
    /// only enforced where a name actually appears on the wire; the identity
    /// rule stays in `CompanionId`.
    pub fn new(tick: u64, states: Vec<CompanionState>) -> Result<Self, ProtocolError> {
        if states.is_empty() || states.len() > MAX_COMPANION_STATES as usize {
            return Err(ProtocolError::InvalidRange);
        }
        let mut ids = Vec::with_capacity(states.len());
        for record in &states {
            Self::valid(record)?;
            ids.push(record.companion_id.bytes());
        }
        if !strictly_increasing_ids(&ids) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self { tick, states })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        UvarintCountBatch::write(&mut encoder, self.tick, self.states.len() as u32);
        for record in &self.states {
            encoder.bytes(&record.companion_id.bytes());
            encoder.i32(i32::from(record.dimension.get()));
            for value in record.position {
                encoder.f32(value);
            }
            encoder.f32(record.yaw);
            encoder.f32(record.pitch);
            encoder.boolean(record.reset);
        }
        encoder
            .finish()
            .expect("validated companion states are encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        if payload.len() > COMPANION_STATES_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let batch = UvarintCountBatch::read(&mut decoder, MAX_COMPANION_STATES)?;
        batch.require_records(&decoder, COMPANION_STATE_WIRE_BYTES)?;
        let mut states = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let companion_id = CompanionId::new(decoder.bytes()?)?;
            let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
                .map_err(|_| ProtocolError::InvalidEnum)?;
            let mut position = [0f32; 3];
            for value in &mut position {
                *value = decoder.f32()?;
            }
            let yaw = decoder.f32()?;
            let pitch = decoder.f32()?;
            let reset = decoder.boolean()?;
            states.push(CompanionState {
                companion_id,
                dimension,
                position,
                yaw,
                pitch,
                reset,
            });
        }
        decoder.done()?;
        Self::new(batch.server_tick, states)
    }
}
