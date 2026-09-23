//! The companion despawn payload that removes one companion from the client's
//! visible set.
//!
//! The family carries no body and no reason: the identity is the whole
//! record, and why the companion left stays server-owned. The identity is the
//! domain's checked `CompanionId`, so the absent zero form the chat event
//! carries as raw wire bytes never becomes a packet value here.

use crate::bytes::{ByteDecoder, SliceWriter};
use crate::entity_id::{self, CompanionId};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;

/// Fixed wire upper bound of the whole payload, copied from the Go decoder's
/// pre-parse guard for this packet ID. The stride equals the bound because the
/// identity alone is the whole record, so a payload above it is refused before
/// a single byte is read.
const COMPANION_DESPAWN_MAX_WIRE_BYTES: usize = 16;

/// Play CompanionDespawn payload: the 16-byte companion identity the
/// authoritative world removed. One message names exactly one companion.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CompanionDespawn {
    pub companion: CompanionId,
}

impl CompanionDespawn {
    pub const PACKET_ID: u32 = 19;

    pub fn new(companion: CompanionId) -> Self {
        Self { companion }
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The gate is total because the identity is the checked domain
    /// `CompanionId`: a zero or non-UUIDv4 value cannot be constructed, so no
    /// field mutation after construction can make this record unpublishable
    /// and the gate restates no rule the domain type owns.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(COMPANION_DESPAWN_MAX_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The destination is tested before the first byte is written, so a short
    /// or invalid call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.bytes(&self.companion.bytes());
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
        // The payload is exactly one identity, so the fixed wire bound is a
        // pre-allocation guard: an oversized payload reports the capacity
        // refusal before a single byte is read, matching the Go decoder's
        // fixed-maximum check for this packet ID.
        if payload.len() > COMPANION_DESPAWN_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let mut decoder = ByteDecoder::new(payload);
        let companion = entity_id::read(&mut decoder)?;
        decoder.done()?;
        Ok(Self::new(companion))
    }
}
