use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::container_ref::{CONTAINER_KIND_CHEST, CONTAINER_KIND_FURNACE, ContainerRef};
use crate::error::ProtocolError;
use crate::move_inventory_stack::INVENTORY_SLOTS;

/// Unified furnace view slots: inventory `0..35`, then fuel, input, and output.
pub const FURNACE_VIEW_SLOTS: u8 = INVENTORY_SLOTS + 3;

/// Furnace output slot can be read but never used as a move target.
pub const FURNACE_OUTPUT_SLOT: u8 = INVENTORY_SLOTS + 2;

/// Unified chest view slots: inventory `0..35` plus the 27 chest slots.
pub const CHEST_VIEW_SLOTS: u8 = INVENTORY_SLOTS + 27;

/// Play MoveContainerStack payload. Source and target are unified container
/// view slots, so the range depends on the referenced container kind.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct MoveContainerStack {
    pub sequence: u64,
    pub container: ContainerRef,
    pub from: u8,
    pub to: u8,
}

impl MoveContainerStack {
    pub const PACKET_ID: u32 = 9;

    pub fn new(
        sequence: u64,
        container: ContainerRef,
        from: u8,
        to: u8,
    ) -> Result<Self, ProtocolError> {
        if from == to {
            return Err(ProtocolError::InvalidRange);
        }
        match container.kind {
            CONTAINER_KIND_FURNACE => {
                if from >= FURNACE_VIEW_SLOTS || to >= FURNACE_VIEW_SLOTS {
                    return Err(ProtocolError::InvalidRange);
                }
                if to == FURNACE_OUTPUT_SLOT {
                    return Err(ProtocolError::InvalidRange);
                }
            }
            CONTAINER_KIND_CHEST => {
                if from >= CHEST_VIEW_SLOTS || to >= CHEST_VIEW_SLOTS {
                    return Err(ProtocolError::InvalidRange);
                }
            }
            _ => return Err(ProtocolError::InvalidEnum),
        }
        Ok(Self {
            sequence,
            container,
            from,
            to,
        })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        self.container.write(&mut encoder);
        encoder.u8(self.from);
        encoder.u8(self.to);
        encoder
            .finish()
            .expect("validated container move is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        let container = ContainerRef::read(&mut decoder)?;
        let from = decoder.u8()?;
        let to = decoder.u8()?;
        decoder.done()?;
        container.validate_any()?;
        Self::new(sequence, container, from, to)
    }
}
