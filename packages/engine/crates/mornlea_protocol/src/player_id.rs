use crate::error::ProtocolError;

/// Stable UUIDv4 player identity. Zero and non-v4 values fail before the
/// identifier is published.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PlayerId([u8; 16]);

impl PlayerId {
    pub fn new(bytes: [u8; 16]) -> Result<Self, ProtocolError> {
        if bytes == [0; 16] || bytes[6] >> 4 != 4 || bytes[8] & 0xc0 != 0x80 {
            return Err(ProtocolError::InvalidIdentity);
        }
        Ok(Self(bytes))
    }

    pub fn bytes(self) -> [u8; 16] {
        self.0
    }
}
