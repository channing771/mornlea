use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;

/// Target kinds confirmed by an authoritative melee hit. The values are
/// appended in order, so existing kinds keep their wire numbers.
pub const COMBAT_TARGET_PLAYER: u8 = 1;
pub const COMBAT_TARGET_HOSTILE: u8 = 2;
pub const COMBAT_TARGET_PASSIVE: u8 = 3;

/// Authoritative player health upper bound. Legal health is `0..=20`, with
/// full health at 20.
pub const MAX_HEALTH: u8 = 20;

/// Fixed wire stride of one melee confirmation: the 8-byte tick, the damage
/// byte and the target kind byte.
const COMBAT_HIT_WIRE_BYTES: usize = 10;

/// Play CombatHit payload: a fixed 10-byte confirmation that the
/// authoritative tick resolved a melee hit this server tick. Damage is a
/// whole number the server already applied; the client only presents it.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CombatHit {
    pub server_tick: u64,
    pub damage: u8,
    pub target_kind: u8,
}

impl CombatHit {
    pub const PACKET_ID: u32 = 25;

    pub fn new(server_tick: u64, damage: u8, target_kind: u8) -> Result<Self, ProtocolError> {
        let hit = Self {
            server_tick,
            damage,
            target_kind,
        };
        hit.valid()?;
        Ok(hit)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into a zero tick, an out-of-range
    /// damage or an unknown kind after construction is refused rather than
    /// silently published. The order is the Go validator's — tick, then the
    /// damage range, then the kind — so both implementations refuse the same
    /// bytes with the same error variant.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.server_tick == 0 || self.damage == 0 || self.damage > MAX_HEALTH {
            return Err(ProtocolError::InvalidRange);
        }
        if !(COMBAT_TARGET_PLAYER..=COMBAT_TARGET_PASSIVE).contains(&self.target_kind) {
            return Err(ProtocolError::InvalidEnum);
        }
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its tick,
    /// damage or kind error here instead of reaching a size or capacity
    /// decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(COMBAT_HIT_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — tick, damage, then the target
    /// kind — and the destination is tested before the first byte is written,
    /// so a short call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.server_tick);
            writer.u8(self.damage);
            writer.u8(self.target_kind);
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
        let server_tick = decoder.u64()?;
        let damage = decoder.u8()?;
        let target_kind = decoder.u8()?;
        decoder.done()?;
        Self::new(server_tick, damage, target_kind)
    }
}
