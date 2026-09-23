use crate::batch::read_fixed;
use crate::bytes::{ByteDecoder, SliceWriter};
use crate::container_ref::ContainerRef;
use crate::error::ProtocolError;
use crate::item_stack::{self, ItemStack};
use crate::server_hello::publish_packet;

/// Fixed chest slot count carried by one payload, copied from the Go
/// `core.ChestSlots` pin.
pub const CHEST_SLOTS: usize = 27;

/// Fixed payload stride: the 18-byte chest reference plus every slot stack.
const CHEST_STATE_WIRE_BYTES: usize = 18 + CHEST_SLOTS * 5;

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
        let state = Self { chest, items };
        state.valid()?;
        Ok(state)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into a malformed chest reference after
    /// construction is refused rather than silently published. The Go
    /// `ChestState.Validate` order is the chest reference (`validChestRef`:
    /// kind, then dimension, then slot, then generation) and then every slot
    /// through `ItemStack.Valid`. The domain `ItemStack` rule is the single
    /// slot-value gate — chest slots accept every registered item, so no
    /// further whitelist applies here and the checked newtype makes an invalid
    /// slot value unconstructible on this surface.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        self.chest.validate_chest()
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its reference
    /// error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(CHEST_STATE_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the 18-byte chest reference, then
    /// the 27 slot stacks — and the destination is tested before the first
    /// byte is written, so a short call leaves every destination byte
    /// unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer: &mut SliceWriter<'_>| {
            self.chest.write_into(writer);
            for stack in &self.items {
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
        let chest = ContainerRef::read(&mut decoder)?;
        let items = read_fixed(&mut decoder, item_stack::read)?;
        decoder.done()?;
        Self::new(chest, items)
    }
}
