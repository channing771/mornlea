use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;

/// Fixed wire stride of one take-crafting-output payload: the 8-byte sequence
/// alone, with no output recipe, count or destination field.
const TAKE_CRAFTING_OUTPUT_WIRE_BYTES: usize = 8;

/// Play TakeCraftingOutput payload. A zero sequence is rejected because it
/// cannot take part in the command-ack protocol; output contents stay
/// server-owned.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct TakeCraftingOutput {
    pub sequence: u64,
}

impl TakeCraftingOutput {
    pub const PACKET_ID: u32 = 15;

    pub fn new(sequence: u64) -> Result<Self, ProtocolError> {
        let command = Self { sequence };
        command.valid()?;
        Ok(command)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The field is public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into a zero sequence after
    /// construction is refused rather than silently published. Unlike the
    /// other sequence-only commands, this one carries a rule, because a zero
    /// sequence cannot take part in the acknowledgement protocol.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.sequence == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its sequence
    /// error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(TAKE_CRAFTING_OUTPUT_WIRE_BYTES)
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
        Self::new(sequence)
    }
}
