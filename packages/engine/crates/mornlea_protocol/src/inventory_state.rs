use crate::batch::read_fixed;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::item_stack::{self, ItemStack};
use crate::server_hello::publish_packet;
use mornlea_domain::HotbarSlot;

/// Fixed hotbar slot count, copied from the Go `core.HotbarSlots` pin.
pub const HOTBAR_SLOTS: usize = 9;

/// Fixed backpack slot count, copied from the Go `core.BackpackSlots` pin.
pub const BACKPACK_SLOTS: usize = 27;

/// Fixed payload stride: the selected byte plus every slot stack.
pub const INVENTORY_STATE_WIRE_BYTES: usize = 1 + (HOTBAR_SLOTS + BACKPACK_SLOTS) * 5;

/// Play InventoryState payload: the complete authoritative item state of one
/// player. The selected hotbar index is a domain `HotbarSlot` and every slot
/// is a validated item stack.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct InventoryState {
    pub selected: u8,
    pub hotbar: [ItemStack; HOTBAR_SLOTS],
    pub backpack: [ItemStack; BACKPACK_SLOTS],
}

impl InventoryState {
    pub const PACKET_ID: u32 = 10;

    pub fn new(
        selected: u8,
        hotbar: [ItemStack; HOTBAR_SLOTS],
        backpack: [ItemStack; BACKPACK_SLOTS],
    ) -> Result<Self, ProtocolError> {
        let state = Self {
            selected,
            hotbar,
            backpack,
        };
        state.valid()?;
        Ok(state)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into an out-of-range selected index
    /// after construction is refused rather than silently published. The Go
    /// `Inventory.Valid` order is the selected index first and every stack
    /// second; the stacks are the domain's checked `ItemStack`, whose private
    /// fields make an invalid slot value unconstructible, so the index is the
    /// only rule this gate restates and the stack rule stays owned once.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        HotbarSlot::new(self.selected).map_err(|_| ProtocolError::InvalidRange)?;
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its index error
    /// here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(INVENTORY_STATE_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the selected byte, then the nine
    /// hotbar stacks, then the 27 backpack stacks — and the destination is
    /// tested before the first byte is written, so a short call leaves every
    /// destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer: &mut SliceWriter<'_>| {
            writer.u8(self.selected);
            for stack in self.hotbar.iter().chain(self.backpack.iter()) {
                item_stack::write_into(*stack, writer);
            }
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
        let selected = decoder.u8()?;
        let hotbar: [ItemStack; HOTBAR_SLOTS] = read_fixed(&mut decoder, item_stack::read)?;
        let backpack: [ItemStack; BACKPACK_SLOTS] = read_fixed(&mut decoder, item_stack::read)?;
        decoder.done()?;
        Self::new(selected, hotbar, backpack)
    }
}
