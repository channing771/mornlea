use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;

/// Unified inventory slot count copied from the Go `InventorySlots` pin.
/// Indices `0..8` are the hotbar; `9..35` are the backpack.
pub const INVENTORY_SLOTS: u8 = 36;

/// Fixed wire stride of one inventory move payload: the 8-byte sequence and
/// the two slot bytes, with no moved item or count field.
const MOVE_INVENTORY_STACK_WIRE_BYTES: usize = 10;

/// Play MoveInventoryStack payload. Source and target must be distinct
/// slots inside `0..INVENTORY_SLOTS-1`; the moved item, the resulting
/// inventory contents and the moved count stay server-owned.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct MoveInventoryStack {
    pub sequence: u64,
    pub from: u8,
    pub to: u8,
}

impl MoveInventoryStack {
    pub const PACKET_ID: u32 = 6;

    pub fn new(sequence: u64, from: u8, to: u8) -> Result<Self, ProtocolError> {
        let command = Self { sequence, from, to };
        command.valid()?;
        Ok(command)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into an out-of-range or same-slot pair
    /// after construction is refused rather than silently published, which is
    /// what keeps the admitted set identical to the Go validator's. The Go
    /// rule checks the range before the same-slot relation, and both report
    /// `InvalidRange`, so the collapsed condition keeps one error variant.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.from >= INVENTORY_SLOTS || self.to >= INVENTORY_SLOTS || self.from == self.to {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its slot error
    /// here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(MOVE_INVENTORY_STACK_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — sequence, then the two slot bytes
    /// — and the destination is tested before the first byte is written, so a
    /// short call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
            writer.u8(self.from);
            writer.u8(self.to);
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
        let from = decoder.u8()?;
        let to = decoder.u8()?;
        decoder.done()?;
        Self::new(sequence, from, to)
    }
}
