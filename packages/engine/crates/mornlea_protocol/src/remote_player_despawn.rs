//! The remote player despawn payload that removes one mirrored peer session.
//!
//! The family carries no body and no reason: the identity is the whole
//! record, and why the peer left stays server-owned.

use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};
use crate::server_hello::publish_packet;

/// Fixed wire stride of the whole payload: the 16-byte identity alone.
const DESPAWN_WIRE_BYTES: usize = 16;

/// Play RemotePlayerDespawn payload: a 16-byte UUIDv4 identity of the
/// remote player the authoritative world removed. The remaining players
/// stay server-owned.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct RemotePlayerDespawn {
    pub player: PlayerId,
}

impl RemotePlayerDespawn {
    pub const PACKET_ID: u32 = 8;

    pub fn new(player: PlayerId) -> Self {
        Self { player }
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate is total because the identity is the checked domain `PlayerId`:
    /// a zero or non-UUIDv4 value cannot be constructed, so no field mutation
    /// after construction can make this record unpublishable and the gate
    /// restates no rule the domain type owns.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(DESPAWN_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The destination is tested before the first byte is written, so a short
    /// or invalid call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.bytes(&self.player.bytes());
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
        let player = player_id::read(&mut decoder)?;
        decoder.done()?;
        Ok(Self::new(player))
    }
}
