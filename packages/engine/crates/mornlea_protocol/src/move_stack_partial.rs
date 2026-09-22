use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::container_ref::{CONTAINER_KIND_CHEST, CONTAINER_KIND_FURNACE, ContainerRef};
use crate::error::ProtocolError;
use crate::move_container_stack::{CHEST_VIEW_SLOTS, FURNACE_VIEW_SLOTS};
use crate::move_crafting_stack::GRID_CRAFTING_VIEW_SLOTS;
use crate::move_inventory_stack::INVENTORY_SLOTS;

/// Stack view domains. The three views share one pair of split commands and
/// decide the meaning and upper bound of the unified index.
pub const STACK_VIEW_INVENTORY: u8 = 0;
pub const STACK_VIEW_CRAFTING: u8 = 1;
pub const STACK_VIEW_CONTAINER: u8 = 2;

/// Play MoveStackPartial payload: a half or single item move whose count is
/// derived by the server from the source stack, so the wire has no count
/// field. The moved amount is authoritative state, not a client decision.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct MoveStackPartial {
    pub sequence: u64,
    pub container: ContainerRef,
    pub view: u8,
    pub from: u8,
    pub to: u8,
    pub single: bool,
}

impl MoveStackPartial {
    pub const PACKET_ID: u32 = 19;

    pub fn new(
        sequence: u64,
        container: ContainerRef,
        view: u8,
        from: u8,
        to: u8,
        single: bool,
    ) -> Result<Self, ProtocolError> {
        validate_stack_split(view, &container, from, to)?;
        if from == to {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self {
            sequence,
            container,
            view,
            from,
            to,
            single,
        })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        self.container.write(&mut encoder);
        encoder.u8(self.view);
        encoder.u8(self.from);
        encoder.u8(self.to);
        encoder.boolean(self.single);
        encoder
            .finish()
            .expect("validated stack split move is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        let container = ContainerRef::read(&mut decoder)?;
        let view = decoder.u8()?;
        let from = decoder.u8()?;
        let to = decoder.u8()?;
        let single = decoder.boolean()?;
        decoder.done()?;
        Self::new(sequence, container, view, from, to, single)
    }
}

/// Play QuickMoveStack payload: an entire-stack move whose destination is a
/// fixed deterministic contract the server derives, so the wire carries no
/// target slot and there is no same-slot rejection.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct QuickMoveStack {
    pub sequence: u64,
    pub container: ContainerRef,
    pub view: u8,
    pub from: u8,
}

impl QuickMoveStack {
    pub const PACKET_ID: u32 = 20;

    pub fn new(
        sequence: u64,
        container: ContainerRef,
        view: u8,
        from: u8,
    ) -> Result<Self, ProtocolError> {
        validate_stack_split(view, &container, from, from)?;
        Ok(Self {
            sequence,
            container,
            view,
            from,
        })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        self.container.write(&mut encoder);
        encoder.u8(self.view);
        encoder.u8(self.from);
        encoder.finish().expect("validated quick move is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        let container = ContainerRef::read(&mut decoder)?;
        let view = decoder.u8()?;
        let from = decoder.u8()?;
        decoder.done()?;
        Self::new(sequence, container, view, from)
    }
}

/// Play DropStack payload: a whole-stack drop addressed by a unified view
/// slot. The drop position is derived by the server from the authoritative
/// player state, so the wire carries no coordinates.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct DropStack {
    pub sequence: u64,
    pub container: ContainerRef,
    pub view: u8,
    pub slot: u8,
}

impl DropStack {
    pub const PACKET_ID: u32 = 21;

    pub fn new(
        sequence: u64,
        container: ContainerRef,
        view: u8,
        slot: u8,
    ) -> Result<Self, ProtocolError> {
        validate_stack_split(view, &container, slot, slot)?;
        Ok(Self {
            sequence,
            container,
            view,
            slot,
        })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        self.container.write(&mut encoder);
        encoder.u8(self.view);
        encoder.u8(self.slot);
        encoder.finish().expect("validated stack drop is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        let container = ContainerRef::read(&mut decoder)?;
        let view = decoder.u8()?;
        let slot = decoder.u8()?;
        decoder.done()?;
        Self::new(sequence, container, view, slot)
    }
}

/// Static range check shared by the three view-addressed commands: the view
/// domain must be known, the container reference must match the view (a
/// legal reference in the container view, the zero reference elsewhere), and
/// both index ends must be inside the upper bound that view dispatches.
fn validate_stack_split(
    view: u8,
    container: &ContainerRef,
    from: u8,
    to: u8,
) -> Result<(), ProtocolError> {
    match view {
        STACK_VIEW_INVENTORY => {
            if *container != ContainerRef::NONE {
                return Err(ProtocolError::InvalidRange);
            }
            if from >= INVENTORY_SLOTS || to >= INVENTORY_SLOTS {
                return Err(ProtocolError::InvalidRange);
            }
        }
        STACK_VIEW_CRAFTING => {
            if *container != ContainerRef::NONE {
                return Err(ProtocolError::InvalidRange);
            }
            if from >= GRID_CRAFTING_VIEW_SLOTS || to >= GRID_CRAFTING_VIEW_SLOTS {
                return Err(ProtocolError::InvalidRange);
            }
        }
        STACK_VIEW_CONTAINER => match container.kind {
            CONTAINER_KIND_FURNACE => {
                if from >= FURNACE_VIEW_SLOTS || to >= FURNACE_VIEW_SLOTS {
                    return Err(ProtocolError::InvalidRange);
                }
            }
            CONTAINER_KIND_CHEST => {
                if from >= CHEST_VIEW_SLOTS || to >= CHEST_VIEW_SLOTS {
                    return Err(ProtocolError::InvalidRange);
                }
            }
            _ => return Err(ProtocolError::InvalidEnum),
        },
        _ => return Err(ProtocolError::InvalidEnum),
    }
    Ok(())
}
