use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::move_inventory_stack::INVENTORY_SLOTS;
use crate::server_hello::publish_packet;

/// Unified crafting grid slot count copied from the Go `CraftingGridSlots` pin.
pub const CRAFTING_GRID_SLOTS: u8 = 9;

/// Unified crafting view: grid `0..8` plus inventory `9..44`.
pub const GRID_CRAFTING_VIEW_SLOTS: u8 = CRAFTING_GRID_SLOTS + INVENTORY_SLOTS;

/// Fixed wire stride of one crafting move payload: the 8-byte sequence and
/// the two unified view slot bytes, with no moved item or count field.
const MOVE_CRAFTING_STACK_WIRE_BYTES: usize = 10;

/// Play MoveCraftingStack payload. Source and target must be distinct view
/// slots, and at least one end must be inside the crafting grid. Inventory
/// to inventory moves stay on `MoveInventoryStack`; the personal grid's
/// extension cells stay an authority rule this layer does not publish.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct MoveCraftingStack {
    pub sequence: u64,
    pub from: u8,
    pub to: u8,
}

impl MoveCraftingStack {
    pub const PACKET_ID: u32 = 7;

    pub fn new(sequence: u64, from: u8, to: u8) -> Result<Self, ProtocolError> {
        let command = Self { sequence, from, to };
        command.valid()?;
        Ok(command)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into an out-of-range, same-slot or
    /// both-in-inventory pair after construction is refused rather than
    /// silently published. The Go rule checks the view range, then the
    /// same-slot relation, then the both-in-inventory exclusion, and all three
    /// report `InvalidRange`, so the collapsed conditions keep one error
    /// variant. The personal grid's size-dependent extension cells are
    /// deliberately absent: the protocol layer does not know the grid size.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.from >= GRID_CRAFTING_VIEW_SLOTS
            || self.to >= GRID_CRAFTING_VIEW_SLOTS
            || self.from == self.to
        {
            return Err(ProtocolError::InvalidRange);
        }
        if self.from >= CRAFTING_GRID_SLOTS && self.to >= CRAFTING_GRID_SLOTS {
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
        Ok(MOVE_CRAFTING_STACK_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — sequence, then the two unified
    /// view slot bytes — and the destination is tested before the first byte
    /// is written, so a short call leaves every destination byte unchanged.
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
