use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::move_inventory_stack::INVENTORY_SLOTS;

/// Unified crafting grid slot count copied from the Go `CraftingGridSlots` pin.
pub const CRAFTING_GRID_SLOTS: u8 = 9;

/// Unified crafting view: grid `0..8` plus inventory `9..44`.
pub const GRID_CRAFTING_VIEW_SLOTS: u8 = CRAFTING_GRID_SLOTS + INVENTORY_SLOTS;

/// Play MoveCraftingStack payload. Source and target must be distinct view
/// slots, and at least one end must be inside the crafting grid. Inventory
/// to inventory moves stay on `MoveInventoryStack`.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct MoveCraftingStack {
    pub sequence: u64,
    pub from: u8,
    pub to: u8,
}

impl MoveCraftingStack {
    pub const PACKET_ID: u32 = 7;

    pub fn new(sequence: u64, from: u8, to: u8) -> Result<Self, ProtocolError> {
        if from >= GRID_CRAFTING_VIEW_SLOTS || to >= GRID_CRAFTING_VIEW_SLOTS || from == to {
            return Err(ProtocolError::InvalidRange);
        }
        if from >= CRAFTING_GRID_SLOTS && to >= CRAFTING_GRID_SLOTS {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self { sequence, from, to })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder.u8(self.from);
        encoder.u8(self.to);
        encoder
            .finish()
            .expect("validated crafting move is encodable")
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
