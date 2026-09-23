use crate::bytes::ByteDecoder;
use crate::error::ProtocolError;
use crate::varint::{decode_uvarint, encode_uvarint};

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
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct InboundHello {
    protocol_version: u32,
}

impl InboundHello {
    /// The protocol version the peer declared.
    pub fn protocol_version(&self) -> u32 {
        self.protocol_version
    }
}
