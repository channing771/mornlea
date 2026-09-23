use crate::bytes::{ByteDecoder, SliceWriter};
use crate::container_ref::{CONTAINER_KIND_CHEST, CONTAINER_KIND_FURNACE, ContainerRef};
use crate::error::ProtocolError;
use crate::move_inventory_stack::INVENTORY_SLOTS;
use crate::server_hello::publish_packet;

/// Unified furnace view slots: inventory `0..35`, then fuel, input, and output.
pub const FURNACE_VIEW_SLOTS: u8 = INVENTORY_SLOTS + 3;

/// Furnace output slot can be read but never used as a move target.
pub const FURNACE_OUTPUT_SLOT: u8 = INVENTORY_SLOTS + 2;

/// Unified chest view slots: inventory `0..35` plus the 27 chest slots.
pub const CHEST_VIEW_SLOTS: u8 = INVENTORY_SLOTS + 27;

/// Fixed wire stride of one container move payload: the 8-byte sequence, the
/// 18-byte container reference and the two unified view slot bytes, with no
/// moved item or count field.
const MOVE_CONTAINER_STACK_WIRE_BYTES: usize = 28;

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
        let command = Self {
            sequence,
            container,
            from,
            to,
        };
        command.valid()?;
        Ok(command)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into a malformed reference, a same-slot
    /// pair or an out-of-range target after construction is refused rather than
    /// silently published. The Go `MoveContainerStack.Validate` order is the
    /// reference first (`validAnyContainerRef`), then the same-slot relation,
    /// then the per-kind range, then the furnace output-target exclusion, and
    /// this gate keeps that order so both implementations refuse the same bytes
    /// with the same error variant.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        self.container.validate_any()?;
        if self.from == self.to {
            return Err(ProtocolError::InvalidRange);
        }
        match self.container.kind {
            CONTAINER_KIND_FURNACE => {
                if self.from >= FURNACE_VIEW_SLOTS || self.to >= FURNACE_VIEW_SLOTS {
                    return Err(ProtocolError::InvalidRange);
                }
                if self.to == FURNACE_OUTPUT_SLOT {
                    return Err(ProtocolError::InvalidRange);
                }
            }
            CONTAINER_KIND_CHEST => {
                if self.from >= CHEST_VIEW_SLOTS || self.to >= CHEST_VIEW_SLOTS {
                    return Err(ProtocolError::InvalidRange);
                }
            }
            _ => return Err(ProtocolError::InvalidEnum),
        }
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its reference or
    /// slot error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(MOVE_CONTAINER_STACK_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — sequence, then the 18-byte
    /// container reference, then the two unified view slot bytes — and the
    /// destination is tested before the first byte is written, so a short call
    /// leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
            self.container.write_into(writer);
            writer.u8(self.from);
            writer.u8(self.to);
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
        let sequence = decoder.u64()?;
        let container = ContainerRef::read(&mut decoder)?;
        let from = decoder.u8()?;
        let to = decoder.u8()?;
        decoder.done()?;
        Self::new(sequence, container, from, to)
    }
}
