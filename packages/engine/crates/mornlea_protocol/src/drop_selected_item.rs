use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;

/// Fixed wire stride of one drop-selected payload: the 8-byte sequence
/// alone, with no item count or drop location field.
const DROP_SELECTED_ITEM_WIRE_BYTES: usize = 8;

/// Play DropSelectedItem payload. A zero sequence is legal and is copied
/// as-is; the selected hotbar slot and drop position are server-owned.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct DropSelectedItem {
    pub sequence: u64,
}

impl DropSelectedItem {
    pub const PACKET_ID: u32 = 11;

    pub fn new(sequence: u64) -> Self {
        Self { sequence }
    }

    /// The single value gate shared by `encode_into` and `decode`.
    ///
    /// The gate is total: the payload carries only the sequence, and the Go
    /// validator places no rule on it — including the zero sequence and the
    /// full u64 range. No field mutation can therefore make the record
    /// unpublishable, which is why the family keeps the fallible surface
    /// without a rejection branch, exactly like the resync family's total
    /// gate.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.validate()?;
        Ok(DROP_SELECTED_ITEM_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the sequence alone — and the
    /// destination is tested before the first byte is written, so a short call
    /// leaves every destination byte unchanged.
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
