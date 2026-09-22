use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Play KeepAlive payload. A zero token is rejected before publication.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct KeepAlive {
    pub token: u64,
}

impl KeepAlive {
    pub const PACKET_ID: u32 = 5;

    pub fn new(token: u64) -> Result<Self, ProtocolError> {
        if token == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self { token })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.token);
        encoder.finish().expect("validated keep alive is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let token = decoder.u64()?;
        decoder.done()?;
        Self::new(token)
    }
}
