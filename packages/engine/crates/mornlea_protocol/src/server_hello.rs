use crate::error::ProtocolError;
use crate::varint::{decode_uvarint, encode_uvarint};

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
}
