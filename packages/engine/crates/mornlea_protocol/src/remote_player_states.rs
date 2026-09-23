//! The bounded remote player state batch published every tick.
//!
//! The record has the same 41-byte stride as a companion state record but a
//! different sort key: remote players order by their own unsigned identity
//! space, and the batch ceiling is the fixed peer-session budget rather than
//! the companion activity limit. The batch also carries a fixed wire ceiling
//! that the Go decoder applies before it allocates, which the companion batch
//! does not need because its own ceiling is reached through the count bound.

use crate::batch::{UvarintCountBatch, strictly_increasing_ids};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};
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

    fn valid(record: &RemotePlayerState) -> Result<(), ProtocolError> {
        if !record.position.iter().all(|value| value.is_finite())
            || !record.yaw.is_finite()
            || !record.pitch.is_finite()
        {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(())
    }

    /// Builds a validated batch.
    ///
    /// Identity validity is enforced by `PlayerId` and the dimension by
    /// `Dimension`, so this gate only checks the batch bounds, the pose
    /// finiteness, and the identity order.
    pub fn new(server_tick: u64, players: Vec<RemotePlayerState>) -> Result<Self, ProtocolError> {
        if players.is_empty() || players.len() > MAX_REMOTE_PLAYER_STATES as usize {
            return Err(ProtocolError::InvalidRange);
        }
        let mut ids = Vec::with_capacity(players.len());
        for record in &players {
            Self::valid(record)?;
            ids.push(record.player_id.bytes());
        }
        if !strictly_increasing_ids(&ids) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self {
            server_tick,
            players,
        })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        UvarintCountBatch::write(&mut encoder, self.server_tick, self.players.len() as u32);
        for player in &self.players {
            encoder.bytes(&player.player_id.bytes());
            encoder.i32(i32::from(player.dimension.get()));
            for value in player.position {
                encoder.f32(value);
            }
            encoder.f32(player.yaw);
            encoder.f32(player.pitch);
            encoder.boolean(player.reset);
        }
        encoder
            .finish()
            .expect("validated remote player states are encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
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
