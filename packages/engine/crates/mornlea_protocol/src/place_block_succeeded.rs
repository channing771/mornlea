use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;

/// Fixed wire stride of one placement acknowledgement: the 8-byte sequence
/// alone, with no placed block, held item or outcome field.
const PLACE_BLOCK_SUCCEEDED_WIRE_BYTES: usize = 8;

/// Play PlaceBlockSucceeded payload. A zero sequence is legal and is copied
/// as-is; the packet only acknowledges that a place command settled.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PlaceBlockSucceeded {
    pub sequence: u64,
}

impl PlaceBlockSucceeded {
    pub const PACKET_ID: u32 = 20;

    pub fn new(sequence: u64) -> Self {
        Self { sequence }
    }

    /// The total value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The record carries a sequence and nothing else, so no field mutation
    /// can make it unpublishable: the Go packet expresses no rule the wire
    /// could violate. The gate stays on the common surface anyway, so a later
    /// field arrives with a refusal path already in place instead of a silent
    /// publication.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.validate()?;
        Ok(PLACE_BLOCK_SUCCEEDED_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the little-endian sequence — and
    /// the destination is tested before the first byte is written, so a short
    /// call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
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
        decoder.done()?;
        Ok(Self::new(sequence))
    }
}
