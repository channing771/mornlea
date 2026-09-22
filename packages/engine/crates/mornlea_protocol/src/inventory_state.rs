use crate::batch::read_fixed;
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::item_stack::ItemStack;
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
        HotbarSlot::new(selected).map_err(|_| ProtocolError::InvalidRange)?;
        Ok(Self {
            selected,
            hotbar,
            backpack,
        })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u8(self.selected);
        for stack in self.hotbar.iter().chain(self.backpack.iter()) {
            stack.write(&mut encoder);
        }
        encoder
            .finish()
            .expect("validated inventory state is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let selected = decoder.u8()?;
        let hotbar: [ItemStack; HOTBAR_SLOTS] = read_fixed(&mut decoder, ItemStack::read)?;
        let backpack: [ItemStack; BACKPACK_SLOTS] = read_fixed(&mut decoder, ItemStack::read)?;
        decoder.done()?;
        Self::new(selected, hotbar, backpack)
    }
}
