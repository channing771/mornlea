//! The companion spawn payload that publishes a companion's full body.
//!
//! A companion is a member of the player's own party, so unlike a remote
//! player it may appear in the overworld alone and its pitch stays inside the
//! vertical look range. The companion name is the canonical display-name form
//! with no Unicode whitespace, so the name rule is shared with the despawn
//! family through `valid_companion_name` instead of being restated here.
//!
//! The record carries public mutable fields, so the value gate runs on every
//! encode rather than only at construction: a spawn mutated into a padded
//! name, a foreign dimension or an out-of-range pitch after construction is
//! refused instead of silently published. The gate order is the Go
//! `CompanionSpawn.Validate` order — the name, the dimension, then the pose —
//! and the identity is already checked by the domain `CompanionId`, whose
//! private representation makes the absent zero form unconstructible on this
//! surface.

use crate::bytes::{ByteDecoder, SliceWriter};
use crate::entity_id::{self, CompanionId, valid_companion_name};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;
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
/// being normalized on the client. The bound is inclusive on both ends.
const PITCH_LIMIT: f32 = std::f32::consts::FRAC_PI_2;

/// Fixed stride of the payload before the name slot: the 16-byte identity,
/// the eight-byte tick, the four-byte dimension and the 20-byte pose.
const SPAWN_FIXED_WIRE_BYTES: usize = 16 + 8 + 4 + 12 + 4 + 4;

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

    pub fn companion_id(&self) -> CompanionId {
        self.companion_id
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate order is the Go validator's: the companion name first, then
    /// the overworld-only dimension, then the finite pose with its inclusive
    /// pitch bound. The identity carries no rule here because the domain type
    /// already refuses a zero or non-UUIDv4 value, so the absent companion
    /// form never reaches a packet.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if !valid_companion_name(&self.name) {
            return Err(ProtocolError::InvalidString);
        }
        if self.dimension != Dimension::OVERWORLD {
            return Err(ProtocolError::InvalidEnum);
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

    /// The exact encoded length: the fixed fields, the canonical uvarint name
    /// prefix and the name bytes.
    ///
    /// The value gate runs first, so an invalid record reports its name,
    /// dimension or pose error here instead of reaching a size or capacity
    /// decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        spawn_wire_len(&self.name)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — identity, the length-prefixed
    /// name, tick, dimension, position, yaw, pitch — and the destination is
    /// tested before the first byte is written, so a short or invalid call
    /// leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.bytes(&self.companion_id.bytes());
            writer.uvarint(companion_name_prefix_len(&self.name));
            writer.bytes(self.name.as_bytes());
            writer.u64(self.tick);
            writer.i32(i32::from(self.dimension.get()));
            for value in self.position {
                writer.f32(value);
            }
            writer.f32(self.yaw);
            writer.f32(self.pitch);
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

/// The exact encoded length of one validated spawn: the fixed fields, the
/// canonical uvarint name prefix and the name bytes.
///
/// Every arithmetic step is checked, so an oversized length counter is refused
/// as `Allocation` before the record is sized.
fn spawn_wire_len(name: &str) -> Result<usize, ProtocolError> {
    canonical_uvarint_length(companion_name_prefix_len(name))
        .checked_add(name.len())
        .and_then(|length| length.checked_add(SPAWN_FIXED_WIRE_BYTES))
        .ok_or(ProtocolError::Allocation)
}

/// The uvarint length prefix one validated name carries.
///
/// The byte and rune bounds are checked before any caller reaches this
/// function, so the conversion cannot fail and an unvalidated name is the
/// caller's contract break rather than a silently truncated prefix.
fn companion_name_prefix_len(name: &str) -> u32 {
    u32::try_from(name.len()).expect("validated companion name fits its length prefix")
}
