use crate::batch::read_fixed;
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::container_ref::ContainerRef;
use crate::error::ProtocolError;
use crate::item_stack::{
    ITEM_COAL, ITEM_NONE, ItemStack, smelting_output, valid_furnace_input, valid_furnace_output,
};

/// Smelt progress ceiling, copied from the Go `core.FurnaceSmeltTicks` pin.
pub const FURNACE_SMELT_TICKS: u8 = 200;

/// Remaining burn-time ceiling, copied from the Go `core.FurnaceBurnTicks` pin.
pub const FURNACE_BURN_TICKS: u16 = 1600;

/// Play FurnaceState payload: the furnace reference, the fixed input, fuel,
/// and output slots, and the two authoritative timers. The wire carries no
/// selected or hover state; the client presents the authoritative mirror.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct FurnaceState {
    pub furnace: ContainerRef,
    pub input: ItemStack,
    pub fuel: ItemStack,
    pub output: ItemStack,
    pub progress_ticks: u8,
    pub burn_ticks: u16,
}

impl FurnaceState {
    pub const PACKET_ID: u32 = 13;

    pub fn new(
        furnace: ContainerRef,
        input: ItemStack,
        fuel: ItemStack,
        output: ItemStack,
        progress_ticks: u8,
        burn_ticks: u16,
    ) -> Result<Self, ProtocolError> {
        furnace.validate_furnace()?;
        if progress_ticks >= FURNACE_SMELT_TICKS || burn_ticks > FURNACE_BURN_TICKS {
            return Err(ProtocolError::InvalidRange);
        }
        if !valid_furnace_input(input) {
            return Err(ProtocolError::InvalidEnum);
        }
        if fuel.item() != ITEM_NONE && fuel.item() != ITEM_COAL {
            return Err(ProtocolError::InvalidEnum);
        }
        if !valid_furnace_output(output) {
            return Err(ProtocolError::InvalidEnum);
        }
        Ok(Self {
            furnace,
            input,
            fuel,
            output,
            progress_ticks,
            burn_ticks,
        })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        self.furnace.write(&mut encoder);
        let slots = [self.input, self.fuel, self.output];
        for stack in &slots {
            stack.write(&mut encoder);
        }
        encoder.u8(self.progress_ticks);
        encoder.u16(self.burn_ticks);
        encoder
            .finish()
            .expect("validated furnace state is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let furnace = ContainerRef::read(&mut decoder)?;
        let slots: [ItemStack; 3] = read_fixed(&mut decoder, ItemStack::read)?;
        let progress_ticks = decoder.u8()?;
        let burn_ticks = decoder.u16()?;
        decoder.done()?;
        Self::new(
            furnace,
            slots[0],
            slots[1],
            slots[2],
            progress_ticks,
            burn_ticks,
        )
    }
}

/// Fixed smelting products published on the wire, exported so callers can
/// check a stack without re-deriving the table.
pub fn is_smelting_product(item: u16) -> bool {
    smelting_output(item).is_some()
}
