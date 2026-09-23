use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use mornlea_domain::RejectReason;

/// Command reject reason IDs copied from the Go `CommandRejectReasonID` pin.
pub const REJECT_INVALID_RAY: u8 = 1;
pub const REJECT_NO_TARGET: u8 = 2;
pub const REJECT_CHUNK_NOT_READY: u8 = 3;
pub const REJECT_PROTECTED_BLOCK: u8 = 4;
pub const REJECT_INVALID_BLOCK: u8 = 5;
pub const REJECT_OCCUPIED: u8 = 6;
pub const REJECT_INVALID_INPUT: u8 = 7;
pub const REJECT_PLAYER_NOT_READY: u8 = 8;
pub const REJECT_INVALID_SLOT: u8 = 9;
pub const REJECT_HOTBAR_FULL: u8 = 10;
pub const REJECT_DROP_CAPACITY: u8 = 11;
pub const REJECT_CONTAINER_CAPACITY: u8 = 12;
pub const REJECT_NOT_FLUID_SOURCE: u8 = 13;
pub const REJECT_BUCKET_MISMATCH: u8 = 14;
pub const REJECT_NOT_ARMOR: u8 = 15;

fn valid_reject_reason(reason: u8) -> bool {
    (REJECT_INVALID_RAY..=REJECT_NOT_ARMOR).contains(&reason)
}

/// Translates one domain reject reason into the wire value it publishes.
///
/// The Go internal enum runs `0..14` while the wire enum runs `1..15`, so the
/// wire value is never the discriminant. This is a closed match over the
/// fifteen published variants — no cast and no catch-all arm — so adding a
/// domain variant fails to compile here instead of silently publishing the
/// next number. The returned `Result` keeps the fallible surface uniform even
/// though every admitted reason has a row.
pub fn reject_reason_to_wire(reason: RejectReason) -> Result<u8, ProtocolError> {
    Ok(match reason {
        RejectReason::InvalidRay => REJECT_INVALID_RAY,
        RejectReason::NoTarget => REJECT_NO_TARGET,
        RejectReason::ChunkNotReady => REJECT_CHUNK_NOT_READY,
        RejectReason::ProtectedBlock => REJECT_PROTECTED_BLOCK,
        RejectReason::InvalidBlock => REJECT_INVALID_BLOCK,
        RejectReason::Occupied => REJECT_OCCUPIED,
        RejectReason::InvalidInput => REJECT_INVALID_INPUT,
        RejectReason::PlayerNotReady => REJECT_PLAYER_NOT_READY,
        RejectReason::InvalidSlot => REJECT_INVALID_SLOT,
        RejectReason::HotbarFull => REJECT_HOTBAR_FULL,
        RejectReason::DropCapacity => REJECT_DROP_CAPACITY,
        RejectReason::ContainerCapacity => REJECT_CONTAINER_CAPACITY,
        RejectReason::NotFluidSource => REJECT_NOT_FLUID_SOURCE,
        RejectReason::BucketMismatch => REJECT_BUCKET_MISMATCH,
        RejectReason::NotArmor => REJECT_NOT_ARMOR,
    })
}

/// Translates one wire reject reason into the domain reason it names.
///
/// The inbound half of the same closed matrix: each frozen wire value maps to
/// exactly one domain variant, and every other byte — including the retired
/// zero and the first number above the interval — is an unknown reason the
/// Go decoder answers with the same sentinel.
pub fn reject_reason_from_wire(reason: u8) -> Result<RejectReason, ProtocolError> {
    match reason {
        REJECT_INVALID_RAY => Ok(RejectReason::InvalidRay),
        REJECT_NO_TARGET => Ok(RejectReason::NoTarget),
        REJECT_CHUNK_NOT_READY => Ok(RejectReason::ChunkNotReady),
        REJECT_PROTECTED_BLOCK => Ok(RejectReason::ProtectedBlock),
        REJECT_INVALID_BLOCK => Ok(RejectReason::InvalidBlock),
        REJECT_OCCUPIED => Ok(RejectReason::Occupied),
        REJECT_INVALID_INPUT => Ok(RejectReason::InvalidInput),
        REJECT_PLAYER_NOT_READY => Ok(RejectReason::PlayerNotReady),
        REJECT_INVALID_SLOT => Ok(RejectReason::InvalidSlot),
        REJECT_HOTBAR_FULL => Ok(RejectReason::HotbarFull),
        REJECT_DROP_CAPACITY => Ok(RejectReason::DropCapacity),
        REJECT_CONTAINER_CAPACITY => Ok(RejectReason::ContainerCapacity),
        REJECT_NOT_FLUID_SOURCE => Ok(RejectReason::NotFluidSource),
        REJECT_BUCKET_MISMATCH => Ok(RejectReason::BucketMismatch),
        REJECT_NOT_ARMOR => Ok(RejectReason::NotArmor),
        _ => Err(ProtocolError::InvalidEnum),
    }
}

/// Fixed wire stride of one command rejection: the 8-byte sequence and the
/// one-byte reject reason.
const COMMAND_REJECTED_WIRE_BYTES: usize = 9;

/// Play CommandRejected payload. Unknown reason IDs fail before publication.
/// A zero sequence is legal and is copied as-is.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CommandRejected {
    pub sequence: u64,
    pub reason: u8,
}

impl CommandRejected {
    pub const PACKET_ID: u32 = 4;

    pub fn new(sequence: u64, reason: u8) -> Result<Self, ProtocolError> {
        let rejection = Self { sequence, reason };
        rejection.valid()?;
        Ok(rejection)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into an unregistered reason after
    /// construction is refused rather than silently published with the retired
    /// zero byte the Go encoder would have written for it. The closed interval
    /// is the Go `CommandRejectReasonID` table, so both implementations refuse
    /// the same bytes with the same error variant.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if !valid_reject_reason(self.reason) {
            return Err(ProtocolError::InvalidEnum);
        }
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its reason
    /// error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(COMMAND_REJECTED_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — sequence, then the reason byte —
    /// and the destination is tested before the first byte is written, so a
    /// short call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
            writer.u8(self.reason);
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
        let sequence = decoder.u64()?;
        let reason = decoder.u8()?;
        decoder.done()?;
        Self::new(sequence, reason)
    }
}
