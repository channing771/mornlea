use crate::bytes::SliceWriter;
use crate::error::ProtocolError;
use crate::varint::{canonical_uvarint_length, decode_uvarint};

/// Handshake ServerHello payload. Unknown protocol versions fail before the
/// record is published; structurally truncated or trailing bytes never decode
/// as a current hello.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ServerHello {
    pub protocol_version: u32,
}

impl ServerHello {
    pub const PACKET_ID: u32 = 0;

    pub fn new(protocol_version: u32) -> Result<Self, ProtocolError> {
        let hello = Self { protocol_version };
        hello.valid()?;
        Ok(hello)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The field is public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into another version after
    /// construction is refused rather than silently published, which is what
    /// keeps the admitted set identical to the Go validator's.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.protocol_version != mornlea_domain::Identities::current().protocol {
            return Err(ProtocolError::UnsupportedVersion);
        }
        Ok(())
    }

    /// The exact encoded length of the current version.
    ///
    /// The value gate runs first, so an invalid record reports its version
    /// error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(canonical_uvarint_length(self.protocol_version))
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The order is the crate-wide packet pattern: validate, compute the exact
    /// length, test the destination, and only then write `dst[..length]`. A
    /// short or invalid call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.uvarint(self.protocol_version);
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
        let (protocol_version, used) = decode_uvarint(payload)?;
        if used != payload.len() {
            return Err(ProtocolError::TrailingBytes);
        }
        Self::new(protocol_version)
    }
}

/// Publishes one validated control record into a caller-owned buffer.
///
/// This is the control group's shared publication half of the packet pattern:
/// `validate` → checked `encoded_len` → capacity check → publish. The record's
/// length is validated by the caller, so this function tests the destination
/// before it touches a byte and then hands `write` a window that is exactly
/// `length` bytes wide. A capacity refusal therefore reports `OutputTooSmall`
/// and leaves every destination byte unchanged, and the closure only lays out
/// bytes the caller already admitted, which is why it cannot fail and why a
/// partial publication is not reachable from this path. The framing module
/// keeps its own private copy because its record is the frame envelope rather
/// than a packet payload.
pub(crate) fn publish_packet(
    length: usize,
    dst: &mut [u8],
    write: impl FnOnce(&mut SliceWriter<'_>),
) -> Result<usize, ProtocolError> {
    let Some(window) = dst.get_mut(..length) else {
        return Err(ProtocolError::OutputTooSmall {
            needed: length,
            available: dst.len(),
        });
    };
    let mut writer = SliceWriter::new(window);
    write(&mut writer);
    let written = writer.finish()?;
    debug_assert_eq!(
        written, length,
        "published record length disagrees with the validated size"
    );
    Ok(written)
}
