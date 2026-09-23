use crate::batch::read_fixed;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::container_ref::ContainerRef;
use crate::error::ProtocolError;
use crate::item_stack::{
    self, ITEM_COAL, ITEM_NONE, ItemStack, smelting_output, valid_furnace_input,
    valid_furnace_output,
};
use crate::server_hello::publish_packet;

/// Smelt progress ceiling, copied from the Go `core.FurnaceSmeltTicks` pin.
pub const FURNACE_SMELT_TICKS: u8 = 200;

/// Remaining burn-time ceiling, copied from the Go `core.FurnaceBurnTicks` pin.
pub const FURNACE_BURN_TICKS: u16 = 1600;

/// Fixed payload stride: the 18-byte reference, the three slot stacks and the
/// two timers.
const FURNACE_STATE_WIRE_BYTES: usize = 18 + 3 * 5 + 1 + 2;

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
        let state = Self {
            furnace,
            input,
            fuel,
            output,
            progress_ticks,
            burn_ticks,
        };
        state.valid()?;
        Ok(state)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into a malformed reference, an
    /// out-of-range timer or a slot holding an item it cannot contain after
    /// construction is refused rather than silently published. The Go
    /// `FurnaceState.Validate` order is the furnace reference
    /// (`validFurnaceRef`: kind, then dimension, then slot, then generation),
    /// then the two timer bounds, then the three slot whitelists, and this gate
    /// keeps that order so both implementations refuse the same bytes with the
    /// same error variant. No timer-versus-stack consistency relation exists in
    /// the Go validator and none is invented here: an idle furnace with progress
    /// zero and burn zero beside valid whitelisted stacks is publishable.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        self.furnace.validate_furnace()?;
        if self.progress_ticks >= FURNACE_SMELT_TICKS || self.burn_ticks > FURNACE_BURN_TICKS {
            return Err(ProtocolError::InvalidRange);
        }
        if !valid_furnace_input(self.input) {
            return Err(ProtocolError::InvalidRange);
        }
        if self.fuel.item() != ITEM_NONE && self.fuel.item() != ITEM_COAL {
            return Err(ProtocolError::InvalidRange);
        }
        if !valid_furnace_output(self.output) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its reference,
    /// timer or slot error here instead of reaching a size or capacity
    /// decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(FURNACE_STATE_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the 18-byte furnace reference,
    /// then the input, fuel and output stacks, then the progress byte and the
    /// little-endian burn time — and the destination is tested before the
    /// first byte is written, so a short call leaves every destination byte
    /// unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer: &mut SliceWriter<'_>| {
            self.furnace.write_into(writer);
            item_stack::write_into(self.input, writer);
            item_stack::write_into(self.fuel, writer);
            item_stack::write_into(self.output, writer);
            writer.u8(self.progress_ticks);
            writer.u16(self.burn_ticks);
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
        let furnace = ContainerRef::read(&mut decoder)?;
        let slots: [ItemStack; 3] = read_fixed(&mut decoder, item_stack::read)?;
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
