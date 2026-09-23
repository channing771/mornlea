use crate::batch::read_fixed;
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::item_stack::{self, ItemStack};
use crate::move_crafting_stack::CRAFTING_GRID_SLOTS;

/// Array length form of the shared grid slot count.
const CRAFTING_GRID_SLOTS_USIZE: usize = CRAFTING_GRID_SLOTS as usize;

/// Fixed crafting grid slot count carried by one payload. The grid command
/// family already pins this count, so this module reuses that constant instead
/// of defining a second copy.

/// Personal grid edge length, the only size that bounds the used grid cells.
pub const CRAFTING_GRID_SIZE_PERSONAL: u8 = 2;

/// Workbench grid edge length.
pub const CRAFTING_GRID_SIZE_WORKBENCH: u8 = 3;

/// Play CraftingState payload: the grid size plus the fixed nine grid slots
/// and the derived output slot. The output is always present on the wire so
/// the encoding never takes a variable-length branch, and a personal grid
/// must leave the cells beyond its own size empty.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct CraftingState {
    pub size: u8,
    pub slots: [ItemStack; CRAFTING_GRID_SLOTS_USIZE],
    pub output: ItemStack,
}

impl CraftingState {
    pub const PACKET_ID: u32 = 21;

    pub fn new(
        size: u8,
        slots: [ItemStack; CRAFTING_GRID_SLOTS_USIZE],
        output: ItemStack,
    ) -> Result<Self, ProtocolError> {
        if size != CRAFTING_GRID_SIZE_PERSONAL && size != CRAFTING_GRID_SIZE_WORKBENCH {
            return Err(ProtocolError::InvalidEnum);
        }
        let used = usize::from(size) * usize::from(size);
        for (index, stack) in slots.iter().enumerate() {
            if size == CRAFTING_GRID_SIZE_PERSONAL && index >= used && *stack != ItemStack::EMPTY {
                return Err(ProtocolError::InvalidRange);
            }
        }
        Ok(Self {
            size,
            slots,
            output,
        })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u8(self.size);
        for stack in &self.slots {
            item_stack::write(*stack, &mut encoder);
        }
        item_stack::write(self.output, &mut encoder);
        encoder
            .finish()
            .expect("validated crafting state is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let size = decoder.u8()?;
        let slots: [ItemStack; CRAFTING_GRID_SLOTS_USIZE] =
            read_fixed(&mut decoder, item_stack::read)?;
        let output = item_stack::read(&mut decoder)?;
        decoder.done()?;
        Self::new(size, slots, output)
    }
}
