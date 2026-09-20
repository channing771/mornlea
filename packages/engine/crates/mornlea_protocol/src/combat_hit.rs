use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Target kinds confirmed by an authoritative melee hit. The values are
/// appended in order, so existing kinds keep their wire numbers.
pub const COMBAT_TARGET_PLAYER: u8 = 1;
pub const COMBAT_TARGET_HOSTILE: u8 = 2;
pub const COMBAT_TARGET_PASSIVE: u8 = 3;

/// Authoritative player health upper bound. Legal health is `0..=20`, with
/// full health at 20.
pub const MAX_HEALTH: u8 = 20;

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
        if server_tick == 0 || damage == 0 || damage > MAX_HEALTH {
            return Err(ProtocolError::InvalidRange);
        }
        if !(COMBAT_TARGET_PLAYER..=COMBAT_TARGET_PASSIVE).contains(&target_kind) {
            return Err(ProtocolError::InvalidEnum);
        }
        Ok(Self {
            server_tick,
            damage,
            target_kind,
        })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.server_tick);
        encoder.u8(self.damage);
        encoder.u8(self.target_kind);
        encoder.finish().expect("validated combat hit is encodable")
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
