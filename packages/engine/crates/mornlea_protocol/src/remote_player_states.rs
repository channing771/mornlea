//! The bounded remote player state batch published every tick.
//!
//! The record has the same 41-byte stride as a companion state record but a
//! different sort key: remote players order by their own unsigned identity
//! space, and the batch ceiling is the fixed peer-session budget rather than
//! the companion activity limit. The batch also carries a fixed wire ceiling
//! that the Go decoder applies before it allocates, which the companion batch
//! does not need because its own ceiling is reached through the count bound.
//! A peer pose publishes whatever look angle the mirrored session carried, so
//! the pitch is unrestricted here.

use crate::batch::{UvarintCountBatch, strictly_increasing_ids};
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;
use mornlea_domain::Dimension;

/// Maximum remote player states one payload may carry, copied from the Go
/// peer-session budget.
pub const MAX_REMOTE_PLAYER_STATES: u32 = 7;

/// Fixed stride of one remote player state record: 16-byte identity, `i32`
/// dimension, three `f32` position components, `f32` yaw, `f32` pitch, and a
/// reset boolean.
pub const REMOTE_PLAYER_STATE_WIRE_BYTES: usize = 16 + 4 + 12 + 4 + 4 + 1;

/// Fixed wire upper bound of the whole payload.
pub const REMOTE_PLAYER_STATES_MAX_WIRE_BYTES: usize =
    8 + 1 + MAX_REMOTE_PLAYER_STATES as usize * REMOTE_PLAYER_STATE_WIRE_BYTES;

/// Fixed header stride before the records: the eight-byte server tick.
const BATCH_TICK_WIRE_BYTES: usize = 8;

/// One peer session body state for one tick.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct RemotePlayerState {
    pub player_id: PlayerId,
    pub dimension: Dimension,
    pub position: [f32; 3],
    pub yaw: f32,
    pub pitch: f32,
    pub reset: bool,
}

/// Play RemotePlayerStates payload: the peer session bodies of one tick,
/// strictly ascending by identity.
#[derive(Clone, Debug, PartialEq)]
pub struct RemotePlayerStates {
    pub server_tick: u64,
    pub players: Vec<RemotePlayerState>,
}

impl RemotePlayerStates {
    pub const PACKET_ID: u32 = 9;

    /// Builds a validated batch.
    ///
    /// Identity validity is enforced by `PlayerId` and the dimension by
    /// `Dimension`, so this gate only checks the batch bounds, the pose
    /// finiteness, and the identity order.
    pub fn new(server_tick: u64, players: Vec<RemotePlayerState>) -> Result<Self, ProtocolError> {
        let states = Self {
            server_tick,
            players,
        };
        states.valid()?;
        Ok(states)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a batch mutated into an empty or over-full record set,
    /// a duplicate or descending identity, or a non-finite pose after
    /// construction is refused instead of silently published. The order is the
    /// Go `RemotePlayerStates.Validate` order — the count bound, then each
    /// record's finiteness, then the strictly increasing identity order — and
    /// the identity comparison is the raw unsigned byte order the Go
    /// `bytes.Compare` applies, which is what `strictly_increasing_ids`
    /// implements.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.players.is_empty() || self.players.len() > MAX_REMOTE_PLAYER_STATES as usize {
            return Err(ProtocolError::InvalidRange);
        }
        let mut ids = Vec::with_capacity(self.players.len());
        for record in &self.players {
            Self::record_valid(record)?;
            ids.push(record.player_id.bytes());
        }
        if !strictly_increasing_ids(&ids) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The per-record finite-pose gate. The identity and the dimension are
    /// checked by their domain types, so the record carries no rule this
    /// helper would restate.
    fn record_valid(record: &RemotePlayerState) -> Result<(), ProtocolError> {
        if !record.position.iter().all(|value| value.is_finite())
            || !record.yaw.is_finite()
            || !record.pitch.is_finite()
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
        let count = self.players.len();
        let records = count
            .checked_mul(REMOTE_PLAYER_STATE_WIRE_BYTES)
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
            writer.u64(self.server_tick);
            writer.uvarint(self.players.len() as u32);
            for player in &self.players {
                writer.bytes(&player.player_id.bytes());
                writer.i32(i32::from(player.dimension.get()));
                for value in player.position {
                    writer.f32(value);
                }
                writer.f32(player.yaw);
                writer.f32(player.pitch);
                writer.boolean(player.reset);
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
        if payload.len() > REMOTE_PLAYER_STATES_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let batch = UvarintCountBatch::read(&mut decoder, MAX_REMOTE_PLAYER_STATES)?;
        batch.require_minimum_records(&decoder, REMOTE_PLAYER_STATE_WIRE_BYTES)?;
        let mut players = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let player_id = player_id::read(&mut decoder)?;
            let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
                .map_err(|_| ProtocolError::InvalidEnum)?;
            let mut position = [0f32; 3];
            for value in &mut position {
                *value = decoder.f32()?;
            }
            let yaw = decoder.f32()?;
            let pitch = decoder.f32()?;
            let reset = decoder.boolean()?;
            players.push(RemotePlayerState {
                player_id,
                dimension,
                position,
                yaw,
                pitch,
                reset,
            });
        }
        decoder.done()?;
        Self::new(batch.server_tick, players)
    }
}
