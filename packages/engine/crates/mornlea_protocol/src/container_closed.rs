use crate::bytes::{ByteDecoder, SliceWriter};
use crate::container_ref::ContainerRef;
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;

/// Fixed payload stride: the 18-byte container reference alone.
const CONTAINER_CLOSED_WIRE_BYTES: usize = 18;

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
        let closed = Self { container };
        closed.valid()?;
        Ok(closed)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The field is public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into a malformed reference after
    /// construction is refused rather than silently published. The gate is
    /// container-neutral (`validate_any`), which is the Go
    /// `validAnyContainerRef` rule: either known kind with its own slot
    /// bounds. The exact all-zero record is refused through its zero
    /// generation rather than treated as an absent container, because this
    /// family closes a real view and the absent form belongs to the inventory
    /// and crafting views alone.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        self.container.validate_any()
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its reference
    /// error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(CONTAINER_CLOSED_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The whole payload is the Go encoder's 18-byte container reference, and
    /// the destination is tested before the first byte is written, so a short
    /// call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer: &mut SliceWriter<'_>| {
            self.container.write_into(writer);
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
        let container = ContainerRef::read(&mut decoder)?;
        decoder.done()?;
        Self::new(container)
    }
}
