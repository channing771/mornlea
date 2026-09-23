//! The companion spawn payload that publishes a companion's full body.
//!
//! The companion name is the canonical display-name form with no Unicode
//! whitespace, so the name rule is shared with the despawn family through
//! `valid_companion_name` instead of being restated here.

use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::entity_id::{self, CompanionId, valid_companion_name};
use crate::error::ProtocolError;
use mornlea_domain::Dimension;

/// Fixed wire upper bound of the whole payload, derived from the Go
/// `CompanionSpawnMaxWireBytes`: 16-byte identity plus a name slot of 128
/// bytes and its two-byte length prefix, then the fixed body fields.
pub const COMPANION_SPAWN_MAX_WIRE_BYTES: usize = 16 + 130 + 8 + 4 + 12 + 4 + 4;

/// Companion display-name byte ceiling shared with every other companion
/// name-carrying family.
const COMPANION_NAME_MAX_BYTES: usize = 128;

/// Companion display-name rune ceiling shared with every other companion
/// name-carrying family.
const COMPANION_NAME_MAX_RUNES: usize = 32;

/// Half turn in radians. The pitch of a companion pose stays inside the
/// vertical look range so a mirrored or wrapped angle is rejected instead of
/// being normalized on the client.
const PITCH_LIMIT: f32 = std::f32::consts::FRAC_PI_2;

/// Play CompanionSpawn payload: the identity, name, and body of one companion
/// the session can see for the first time.
#[derive(Clone, Debug, PartialEq)]
pub struct CompanionSpawn {
    pub companion_id: CompanionId,
    pub name: String,
    pub tick: u64,
    pub dimension: Dimension,
    pub position: [f32; 3],
    pub yaw: f32,
    pub pitch: f32,
}

impl CompanionSpawn {
    pub const PACKET_ID: u32 = 17;

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.dimension != Dimension::OVERWORLD {
            return Err(ProtocolError::InvalidEnum);
        }
        if !valid_companion_name(&self.name) {
            return Err(ProtocolError::InvalidString);
        }
        if !self.position.iter().all(|value| value.is_finite())
            || !self.yaw.is_finite()
            || !self.pitch.is_finite()
            || self.pitch.abs() > PITCH_LIMIT
        {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(())
    }

    /// Builds a validated spawn. The identity is validated by `CompanionId`
    /// itself, so a non-UUIDv4 value fails at construction rather than here.
    pub fn new(
        companion_id: CompanionId,
        name: String,
        tick: u64,
        dimension: Dimension,
        position: [f32; 3],
        yaw: f32,
        pitch: f32,
    ) -> Result<Self, ProtocolError> {
        let spawn = Self {
            companion_id,
            name,
            tick,
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

    pub fn companion_id(&self) -> CompanionId {
        self.companion_id
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.bytes(&self.companion_id.bytes());
        encoder.string(&self.name, COMPANION_NAME_MAX_BYTES);
        encoder.u64(self.tick);
        encoder.i32(i32::from(self.dimension.get()));
        for value in self.position {
            encoder.f32(value);
        }
        encoder.f32(self.yaw);
        encoder.f32(self.pitch);
        encoder
            .finish()
            .expect("validated companion spawn is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        if payload.len() > COMPANION_SPAWN_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let companion_id = entity_id::read(&mut decoder)?;
        let name = decoder.string(COMPANION_NAME_MAX_BYTES, COMPANION_NAME_MAX_RUNES)?;
        let tick = decoder.u64()?;
        let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
            .map_err(|_| ProtocolError::InvalidEnum)?;
        let mut position = [0f32; 3];
        for value in &mut position {
            *value = decoder.f32()?;
        }
        let yaw = decoder.f32()?;
        let pitch = decoder.f32()?;
        decoder.done()?;
        Self::new(companion_id, name, tick, dimension, position, yaw, pitch)
    }
}
