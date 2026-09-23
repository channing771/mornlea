//! The remote player spawn payload that publishes another session's body.
//!
//! A remote player is a mirror of a peer session, so unlike a companion or a
//! mob it may appear in either playable dimension. Its name rule is the plain
//! canonical display name, not the companion rule that also rejects embedded
//! whitespace, because a player display name may legitimately contain spaces.

use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::entity_id::valid_display_name;
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};
use mornlea_domain::Dimension;

/// Display-name byte ceiling, copied from the Go `RemotePlayerSpawn` string
/// slot.
const DISPLAY_NAME_MAX_BYTES: usize = 128;

/// Display-name rune ceiling, copied from the Go `RemotePlayerSpawn` string
/// slot.
const DISPLAY_NAME_MAX_RUNES: usize = 32;

/// Play RemotePlayerSpawn payload: the identity, name, and body of one peer
/// session the client can see for the first time.
#[derive(Clone, Debug, PartialEq)]
pub struct RemotePlayerSpawn {
    pub player_id: PlayerId,
    pub display_name: String,
    pub server_tick: u64,
    pub dimension: Dimension,
    pub position: [f32; 3],
    pub yaw: f32,
    pub pitch: f32,
}

impl RemotePlayerSpawn {
    pub const PACKET_ID: u32 = 7;

    fn valid(&self) -> Result<(), ProtocolError> {
        if !valid_display_name(&self.display_name) {
            return Err(ProtocolError::InvalidString);
        }
        if !self.position.iter().all(|value| value.is_finite())
            || !self.yaw.is_finite()
            || !self.pitch.is_finite()
        {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(())
    }

    /// Builds a validated spawn. The identity is validated by `PlayerId`
    /// itself and the dimension by `Dimension`, so a non-UUIDv4 identity or
    /// an unknown dimension fails at construction rather than here.
    pub fn new(
        player_id: PlayerId,
        display_name: String,
        server_tick: u64,
        dimension: Dimension,
        position: [f32; 3],
        yaw: f32,
        pitch: f32,
    ) -> Result<Self, ProtocolError> {
        let spawn = Self {
            player_id,
            display_name,
            server_tick,
            dimension,
            position,
            yaw,
            pitch,
        };
        spawn.valid()?;
        Ok(spawn)
    }

    /// The single validation gate shared by `new` and `decode`.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    pub fn player_id(&self) -> PlayerId {
        self.player_id
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.bytes(&self.player_id.bytes());
        encoder.string(&self.display_name, DISPLAY_NAME_MAX_BYTES);
        encoder.u64(self.server_tick);
        encoder.i32(i32::from(self.dimension.get()));
        for value in self.position {
            encoder.f32(value);
        }
        encoder.f32(self.yaw);
        encoder.f32(self.pitch);
        encoder
            .finish()
            .expect("validated remote player spawn is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let player_id = player_id::read(&mut decoder)?;
        let display_name = decoder.string(DISPLAY_NAME_MAX_BYTES, DISPLAY_NAME_MAX_RUNES)?;
        let server_tick = decoder.u64()?;
        let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
            .map_err(|_| ProtocolError::InvalidEnum)?;
        let mut position = [0f32; 3];
        for value in &mut position {
            *value = decoder.f32()?;
        }
        let yaw = decoder.f32()?;
        let pitch = decoder.f32()?;
        decoder.done()?;
        Self::new(
            player_id,
            display_name,
            server_tick,
            dimension,
            position,
            yaw,
            pitch,
        )
    }
}
