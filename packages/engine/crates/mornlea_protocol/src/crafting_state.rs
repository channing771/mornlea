use crate::batch::read_fixed;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::item_stack::{self, ItemStack};
use crate::move_crafting_stack::CRAFTING_GRID_SLOTS;
use crate::server_hello::publish_packet;

/// Array length form of the shared grid slot count.
const CRAFTING_GRID_SLOTS_USIZE: usize = CRAFTING_GRID_SLOTS as usize;

/// Fixed crafting grid slot count carried by one payload. The grid command
/// family already pins this count, so this module reuses that constant instead
/// of defining a second copy.

/// Personal grid edge length, the only size that bounds the used grid cells.
pub const CRAFTING_GRID_SIZE_PERSONAL: u8 = 2;

/// Workbench grid edge length.
pub const CRAFTING_GRID_SIZE_WORKBENCH: u8 = 3;

/// Fixed payload stride: the size byte, the nine grid stacks and the output.
const CRAFTING_STATE_WIRE_BYTES: usize = 1 + (CRAFTING_GRID_SLOTS_USIZE + 1) * 5;

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
        let state = Self {
            size,
            slots,
            output,
        };
        state.valid()?;
        Ok(state)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into an unknown size or a residue slot
    /// after construction is refused rather than silently published. The Go
    /// `CraftingState.Validate` order is the size first, then every grid slot
    /// with the personal-grid residue rule, then the output; the slots and the
    /// output are the domain's checked `ItemStack`, so this gate restates the
    /// size domain and the residue relation alone.
    ///
    /// An unknown size is a range violation rather than an enum violation: the
    /// Go validator refuses it with its own size message, which the corpus
    /// producer records at the invalid-value boundary the Rust side publishes.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.size != CRAFTING_GRID_SIZE_PERSONAL && self.size != CRAFTING_GRID_SIZE_WORKBENCH {
            return Err(ProtocolError::InvalidRange);
        }
        let used = usize::from(self.size) * usize::from(self.size);
        if self.size == CRAFTING_GRID_SIZE_PERSONAL {
            for stack in self.slots.iter().skip(used) {
                if *stack != ItemStack::EMPTY {
                    return Err(ProtocolError::InvalidRange);
                }
            }
        }
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its size or
    /// residue error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(CRAFTING_STATE_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the size byte, the nine grid
    /// stacks, then the derived output stack — and the destination is tested
    /// before the first byte is written, so a short call leaves every
    /// destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer: &mut SliceWriter<'_>| {
            writer.u8(self.size);
            for stack in &self.slots {
                item_stack::write_into(*stack, writer);
            }
            item_stack::write_into(self.output, writer);
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
        let size = decoder.u8()?;
        let slots: [ItemStack; CRAFTING_GRID_SLOTS_USIZE] =
            read_fixed(&mut decoder, item_stack::read)?;
        let output = item_stack::read(&mut decoder)?;
        decoder.done()?;
        Self::new(size, slots, output)
    }
}
