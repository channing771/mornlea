use crate::bytes::{ByteDecoder, SliceWriter};
use crate::container_ref::ContainerRef;
use crate::error::ProtocolError;
use crate::move_container_stack::{CHEST_VIEW_SLOTS, FURNACE_VIEW_SLOTS};
use crate::move_crafting_stack::GRID_CRAFTING_VIEW_SLOTS;
use crate::move_inventory_stack::INVENTORY_SLOTS;
use crate::server_hello::publish_packet;
use mornlea_domain::{ContainerKind, ContainerRef as DomainContainerRef};

/// Stack view domains. The three views share one pair of split commands and
/// decide the meaning and upper bound of the unified index.
pub const STACK_VIEW_INVENTORY: u8 = 0;
pub const STACK_VIEW_CRAFTING: u8 = 1;
pub const STACK_VIEW_CONTAINER: u8 = 2;

/// Fixed wire stride of one partial move payload: the 8-byte sequence, the
/// 18-byte container reference, the view byte, the two unified index bytes and
/// the single-item flag, with no moved count field.
const MOVE_STACK_PARTIAL_WIRE_BYTES: usize = 30;

/// Fixed wire stride of one view-addressed payload with a single index: the
/// sequence, the reference, the view byte and one unified index byte.
const VIEW_ADDRESSED_WIRE_BYTES: usize = 28;

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
        let command = Self {
            sequence,
            container,
            view,
            from,
            to,
            single,
        };
        command.valid()?;
        Ok(command)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into an unknown view, a reference that
    /// does not match its view, an out-of-range index or a same-slot pair after
    /// construction is refused rather than silently published. The Go
    /// `validateStackSplit` order is the view domain, then the reference
    /// regime, then the per-view index bounds; the same-slot relation follows.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        validate_stack_view(self.view, &self.container, &[self.from, self.to])?;
        if self.from == self.to {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(MOVE_STACK_PARTIAL_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — sequence, the 18-byte reference,
    /// the view byte, the two index bytes and the single-item flag — and the
    /// destination is tested before the first byte is written, so a short call
    /// leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
            self.container.write_into(writer);
            writer.u8(self.view);
            writer.u8(self.from);
            writer.u8(self.to);
            writer.boolean(self.single);
        })
    }

    /// The allocating compatibility wrapper.
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
        let command = Self {
            sequence,
            container,
            view,
            from,
        };
        command.valid()?;
        Ok(command)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The family carries one index, so the gate hands that index to the shared
    /// view validator as both ends and adds no rule of its own: a destination
    /// the server derives cannot be checked here.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        validate_stack_view(self.view, &self.container, &[self.from, self.from]).map(|_| ())
    }

    /// The exact encoded length, which is the fixed payload stride.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(VIEW_ADDRESSED_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
            self.container.write_into(writer);
            writer.u8(self.view);
            writer.u8(self.from);
        })
    }

    /// The allocating compatibility wrapper.
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
        let command = Self {
            sequence,
            container,
            view,
            slot,
        };
        command.valid()?;
        Ok(command)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        validate_stack_view(self.view, &self.container, &[self.slot, self.slot]).map(|_| ())
    }

    /// The exact encoded length, which is the fixed payload stride.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(VIEW_ADDRESSED_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
            self.container.write_into(writer);
            writer.u8(self.view);
            writer.u8(self.slot);
        })
    }

    /// The allocating compatibility wrapper.
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
        let view = decoder.u8()?;
        let slot = decoder.u8()?;
        decoder.done()?;
        Self::new(sequence, container, view, slot)
    }
}

/// The shared static rule of the three view-addressed commands: the view
/// domain, the reference regime its view names, and the index bound that view
/// dispatches.
///
/// The Go `validateStackSplit` order is preserved exactly: an unknown view is
/// refused before anything else, then the reference regime (the exact all-zero
/// record in the inventory and crafting views, a checked real reference in the
/// container view), then the per-view index bounds. The container view's
/// conversion runs [`ContainerRef::to_domain_present`] even though the packet
/// layer publishes the raw reference, so a malformed real reference — a foreign
/// dimension, an unknown kind, a zero generation or a physical slot outside the
/// per-chunk array — is refused here instead of being handed to the authority.
///
/// The returned domain reference is the checked identity the container view
/// addresses; the inventory and crafting views return `None` because they carry
/// the absent sentinel. Target-cell capacity, furnace slot item rules, the
/// moved count and the drop position are authority rules this gate does not
/// publish, so the two crafting inventory-region indices and the furnace output
/// slot as a target stay wire-valid here exactly as they are in Go.
fn validate_stack_view(
    view: u8,
    container: &ContainerRef,
    indices: &[u8],
) -> Result<Option<DomainContainerRef>, ProtocolError> {
    match view {
        STACK_VIEW_INVENTORY => {
            if *container != ContainerRef::NONE {
                return Err(ProtocolError::InvalidRange);
            }
            require_indices(indices, INVENTORY_SLOTS)?;
            Ok(None)
        }
        STACK_VIEW_CRAFTING => {
            if *container != ContainerRef::NONE {
                return Err(ProtocolError::InvalidRange);
            }
            require_indices(indices, GRID_CRAFTING_VIEW_SLOTS)?;
            Ok(None)
        }
        STACK_VIEW_CONTAINER => {
            let reference = container.to_domain_present()?;
            let bound = match reference.kind() {
                ContainerKind::Furnace => FURNACE_VIEW_SLOTS,
                ContainerKind::Chest => CHEST_VIEW_SLOTS,
            };
            require_indices(indices, bound)?;
            Ok(Some(reference))
        }
        _ => Err(ProtocolError::InvalidEnum),
    }
}

/// Reports whether every unified index stays inside `bound`, which is the
/// exclusive upper bound the view dispatches.
fn require_indices(indices: &[u8], bound: u8) -> Result<(), ProtocolError> {
    if indices.iter().any(|index| *index >= bound) {
        return Err(ProtocolError::InvalidRange);
    }
    Ok(())
}
