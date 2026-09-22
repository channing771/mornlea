use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Play KeepAliveReply payload. A zero token is rejected before publication.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct KeepAliveReply {
    pub token: u64,
}

impl KeepAliveReply {
    pub const PACKET_ID: u32 = 4;

    pub fn new(token: u64) -> Result<Self, ProtocolError> {
        if token == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self { token })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.token);
        encoder
            .finish()
            .expect("validated keep alive reply is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let token = decoder.u64()?;
        decoder.done()?;
        Self::new(token)
    }
}
