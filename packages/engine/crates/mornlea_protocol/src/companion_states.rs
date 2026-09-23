//! The bounded companion state batch published every tick.
//!
//! The batch reuses the shared uvarint-count header and the shared
//! unsigned-byte identity ordering so the companion batch, the hostile batch,
//! and the remote-player batch cannot drift apart in their count bounds or
//! their sort rule. Unlike a remote-player record, a companion record is
//! overworld-only and its pitch stays inside the inclusive vertical look
//! range, because a companion is a member of the player's own party rather
//! than a mirror of a peer session.
//!
//! The count bound fires before the record scan, and the exact-remaining-length
//! rule then rejects a payload whose declared count disagrees with the record
//! bytes present, matching the Go decoder's order.

use crate::batch::{UvarintCountBatch, strictly_increasing_ids};
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::entity_id::{self, CompanionId};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;
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

/// Fixed header stride before the records: the eight-byte server tick.
const BATCH_TICK_WIRE_BYTES: usize = 8;

/// Half turn in radians, the inclusive pitch ceiling of a companion pose.
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

    /// Builds a validated batch.
    ///
    /// The name is not carried by this family, so the companion name rule is
    /// only enforced where a name actually appears on the wire; the identity
    /// rule stays in `CompanionId`.
    pub fn new(tick: u64, states: Vec<CompanionState>) -> Result<Self, ProtocolError> {
        let batch = Self { tick, states };
        batch.valid()?;
        Ok(batch)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate order is the Go `CompanionStates.Validate` order: the count
    /// bound, then each record's overworld dimension and finite pose with its
    /// inclusive pitch bound, then the strictly increasing identity order in
    /// raw unsigned byte order.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.states.is_empty() || self.states.len() > MAX_COMPANION_STATES as usize {
            return Err(ProtocolError::InvalidRange);
        }
        let mut ids = Vec::with_capacity(self.states.len());
        for record in &self.states {
            Self::record_valid(record)?;
            ids.push(record.companion_id.bytes());
        }
        if !strictly_increasing_ids(&ids) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The per-record gate. The identity is checked by its domain type, so the
    /// record carries the overworld dimension and the bounded finite pose only.
    fn record_valid(record: &CompanionState) -> Result<(), ProtocolError> {
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

    /// The exact encoded length: the eight-byte tick, the canonical uvarint
    /// count and one fixed stride per record.
    ///
    /// The value gate runs first, so an invalid batch reports its count, pose
    /// or order error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        let count = self.states.len();
        let records = count
            .checked_mul(COMPANION_STATE_WIRE_BYTES)
            .ok_or(ProtocolError::Allocation)?;
        BATCH_TICK_WIRE_BYTES
            .checked_add(canonical_uvarint_length(count as u32))
            .and_then(|length| length.checked_add(records))
            .ok_or(ProtocolError::Allocation)
    }

    /// Publishes the batch into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the server tick, the canonical
    /// uvarint count and the per-record identity, dimension, position, yaw,
    /// pitch and reset flag — and the destination is tested before the first
    /// byte is written, so a short or invalid call leaves every destination
    /// byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.tick);
            writer.uvarint(self.states.len() as u32);
            for record in &self.states {
                writer.bytes(&record.companion_id.bytes());
                writer.i32(i32::from(record.dimension.get()));
                for value in record.position {
                    writer.f32(value);
                }
                writer.f32(record.yaw);
                writer.f32(record.pitch);
                writer.boolean(record.reset);
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
        // The fixed wire ceiling is a pre-allocation guard, so an oversized
        // payload reports the capacity refusal before a single field is read.
        if payload.len() > COMPANION_STATES_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let batch = UvarintCountBatch::read(&mut decoder, MAX_COMPANION_STATES)?;
        batch.require_records(&decoder, COMPANION_STATE_WIRE_BYTES)?;
        let mut states = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let companion_id = entity_id::read(&mut decoder)?;
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
