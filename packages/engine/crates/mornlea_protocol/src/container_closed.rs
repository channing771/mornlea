use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::container_ref::ContainerRef;
use crate::error::ProtocolError;

/// Play ContainerClosed payload: the fixed 18-byte container reference whose
/// view ended, shared by furnaces and chests. The client mirror drops the
/// container; the authoritative world keeps owning it.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ContainerClosed {
    pub container: ContainerRef,
}

impl ContainerClosed {
    pub const PACKET_ID: u32 = 14;

    pub fn new(container: ContainerRef) -> Result<Self, ProtocolError> {
        container.validate_any()?;
        Ok(Self { container })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        self.container.write(&mut encoder);
        encoder
            .finish()
            .expect("validated container reference is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let container = ContainerRef::read(&mut decoder)?;
        decoder.done()?;
        Self::new(container)
    }
}
