use crate::batch::read_fixed;
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::container_ref::ContainerRef;
use crate::error::ProtocolError;
use crate::item_stack::ItemStack;

/// Fixed chest slot count carried by one payload, copied from the Go
/// `core.ChestSlots` pin.
pub const CHEST_SLOTS: usize = 27;

/// Play ChestState payload: the container reference plus the fixed 27 slots.
/// Chest slots accept any registered item, so only the item-stack rule and
/// the chest reference gate apply.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ChestState {
    pub chest: ContainerRef,
    pub items: [ItemStack; CHEST_SLOTS],
}

impl ChestState {
    pub const PACKET_ID: u32 = 15;

    pub fn new(
        chest: ContainerRef,
        items: [ItemStack; CHEST_SLOTS],
    ) -> Result<Self, ProtocolError> {
        chest.validate_chest()?;
        // `ItemStack::new` is the single slot-value gate: chest slots accept
        // every registered item, so no further whitelist applies here.
        Ok(Self { chest, items })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        self.chest.write(&mut encoder);
        for stack in &self.items {
            stack.write(&mut encoder);
        }
        encoder
            .finish()
            .expect("validated chest state is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let chest = ContainerRef::read(&mut decoder)?;
        let items = read_fixed(&mut decoder, ItemStack::read)?;
        decoder.done()?;
        Self::new(chest, items)
    }
}
