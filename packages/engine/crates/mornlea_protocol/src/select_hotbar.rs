use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;

/// Fixed wire stride of one hotbar selection payload: the 8-byte sequence and
/// the selected slot byte.
const SELECT_HOTBAR_WIRE_BYTES: usize = 9;

/// Play SelectHotbar payload. Slot must be inside the domain hotbar range
/// `0..HotbarSlot::COUNT-1`; unknown slots fail before publication.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct SelectHotbar {
    pub sequence: u64,
    pub slot: u8,
}

impl SelectHotbar {
    pub const PACKET_ID: u32 = 5;

    pub fn new(sequence: u64, slot: u8) -> Result<Self, ProtocolError> {
        let selection = Self { sequence, slot };
        selection.valid()?;
        Ok(selection)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into an out-of-range slot after
    /// construction is refused rather than silently published, which is what
    /// keeps the admitted set identical to the Go validator's.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        mornlea_domain::HotbarSlot::new(self.slot).map_err(|_| ProtocolError::InvalidRange)?;
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its slot error
    /// here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(SELECT_HOTBAR_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — sequence, then the slot byte — and
    /// the destination is tested before the first byte is written, so a short
    /// call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
            writer.u8(self.slot);
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
        let slot = decoder.u8()?;
        decoder.done()?;
        Self::new(sequence, slot)
    }
}
