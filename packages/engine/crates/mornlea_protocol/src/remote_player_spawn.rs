//! The remote player spawn payload that publishes another session's body.
//!
//! A remote player is a mirror of a peer session, so unlike a companion or a
//! mob it may appear in either playable dimension. Its name rule is the plain
//! canonical display name, not the companion rule that also rejects embedded
//! whitespace, because a player display name may legitimately contain spaces.
//! The pitch stays unrestricted for the same reason: a mirrored peer pose is
//! published as observed, so the companion vertical-look rule is not imported.

use crate::bytes::{ByteDecoder, SliceWriter};
use crate::entity_id::valid_display_name;
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;
use mornlea_domain::Dimension;

/// Display-name byte ceiling, copied from the Go `RemotePlayerSpawn` string
/// slot.
const DISPLAY_NAME_MAX_BYTES: usize = 128;

/// Display-name rune ceiling, copied from the Go `RemotePlayerSpawn` string
/// slot.
const DISPLAY_NAME_MAX_RUNES: usize = 32;

/// Fixed stride of the payload before the name slot: the 16-byte identity,
/// the eight-byte tick, the four-byte dimension and the 20-byte pose.
const SPAWN_FIXED_WIRE_BYTES: usize = 16 + 8 + 4 + 12 + 4 + 4;

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

    pub fn player_id(&self) -> PlayerId {
        self.player_id
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into a padded, empty or oversized
    /// name, or into a non-finite pose, after construction is refused instead
    /// of silently published. The order is the Go `RemotePlayerSpawn.Validate`
    /// order — the canonical display name first, then the pose finiteness —
    /// and the identity and the dimension are already checked by their domain
    /// types, so this gate restates no rule those types own.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

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

    /// The exact encoded length: the fixed fields, the canonical uvarint name
    /// prefix and the name bytes.
    ///
    /// The value gate runs first, so an invalid record reports its name or
    /// pose error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        spawn_wire_len(&self.display_name)
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
            writer.bytes(&self.player_id.bytes());
            writer.uvarint(display_name_prefix_len(&self.display_name));
            writer.bytes(self.display_name.as_bytes());
            writer.u64(self.server_tick);
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

/// The exact encoded length of one validated spawn: the fixed fields, the
/// canonical uvarint name prefix and the name bytes.
///
/// Every arithmetic step is checked, so an oversized length counter is refused
/// as `Allocation` before the record is sized.
fn spawn_wire_len(name: &str) -> Result<usize, ProtocolError> {
    canonical_uvarint_length(display_name_prefix_len(name))
        .checked_add(name.len())
        .and_then(|length| length.checked_add(SPAWN_FIXED_WIRE_BYTES))
        .ok_or(ProtocolError::Allocation)
}

/// The uvarint length prefix one validated name carries.
///
/// The byte and rune bounds are checked before any caller reaches this
/// function, so the conversion cannot fail and an unvalidated name is the
/// caller's contract break rather than a silently truncated prefix.
fn display_name_prefix_len(name: &str) -> u32 {
    u32::try_from(name.len()).expect("validated display name fits its length prefix")
}
