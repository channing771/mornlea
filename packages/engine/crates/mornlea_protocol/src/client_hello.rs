use crate::bytes::ByteDecoder;
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use crate::varint::{canonical_uvarint_length, decode_uvarint, encode_uvarint};

/// Handshake ClientHello payload. Unknown protocol versions fail before the
/// record is published; structurally truncated or trailing bytes never decode
/// as a current hello.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ClientHello {
    pub protocol_version: u32,
}

impl ClientHello {
    pub const PACKET_ID: u32 = 0;

    pub fn new(protocol_version: u32) -> Result<Self, ProtocolError> {
        if protocol_version != mornlea_domain::Identities::current().protocol {
            return Err(ProtocolError::UnsupportedVersion);
        }
        Ok(Self { protocol_version })
    }

    pub fn encode(self) -> Vec<u8> {
        encode_uvarint(self.protocol_version)
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let (protocol_version, used) = decode_uvarint(payload)?;
        if used != payload.len() {
            return Err(ProtocolError::TrailingBytes);
        }
        Self::new(protocol_version)
    }

    /// Decodes one inbound hello structurally, keeping the peer's version.
    ///
    /// The inbound path and the outbound record are deliberately different
    /// decisions. An outbound `ClientHello` is this side's own statement, so it
    /// can only name the current version. An inbound hello is a peer's
    /// statement, and a peer running another version has to receive the
    /// negotiated version-mismatch answer rather than a bare decode failure;
    /// applying the version policy here would destroy the record that answer
    /// needs. Only the structural rules apply: a canonical uvarint and full
    /// consumption. [`crate::admission::validate_hello`] owns the version
    /// decision.
    pub fn decode_inbound(payload: &[u8]) -> Result<InboundHello, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let protocol_version = decoder.uvarint()?;
        decoder.done()?;
        Ok(InboundHello { protocol_version })
    }
}

/// One structurally decoded handshake hello, before any version decision.
///
/// The record keeps the peer's declared version verbatim so admission can
/// compare it against this side's version and answer with the negotiated pair.
/// Its wire form is that version as a canonical uvarint, and re-publishing the
/// record writes exactly the bytes the decoder admitted: the version rule
/// belongs to [`crate::admission::validate_hello`] and to the outbound
/// [`ClientHello`], so the raw record's gate restates neither.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct InboundHello {
    protocol_version: u32,
}

impl InboundHello {
    /// The protocol version the peer declared.
    pub fn protocol_version(&self) -> u32 {
        self.protocol_version
    }

    /// The total value gate this record's encoder shares with its siblings.
    ///
    /// The gate is total because every `u32` has one canonical uvarint form and
    /// the record carries no other field: no mutation after construction can
    /// make it unpublishable. A version this side does not run is still a
    /// publishable raw record, because publishing it is what lets a peer learn
    /// which version was measured against it.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        Ok(())
    }

    /// The exact encoded length: the canonical uvarint of the declared
    /// version.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.validate()?;
        Ok(canonical_uvarint_length(self.protocol_version))
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The order is the crate-wide packet pattern: validate, compute the exact
    /// length, test the destination, and only then write `dst[..length]`. A
    /// short call reports `OutputTooSmall` and leaves every destination byte
    /// unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.uvarint(self.protocol_version);
        })
    }

    /// The allocating compatibility wrapper over `encode_into`.
    pub fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        let length = self.encoded_len()?;
        let mut wire = vec![0u8; length];
        let written = self.encode_into(&mut wire)?;
        wire.truncate(written);
        Ok(wire)
    }
}
