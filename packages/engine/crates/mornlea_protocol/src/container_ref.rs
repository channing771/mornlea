use crate::bytes::{ByteDecoder, ByteEncoder, SliceWriter};
use crate::error::ProtocolError;
use mornlea_domain::{ChunkPos, ContainerKind, ContainerRef as DomainContainerRef};

/// Fixed authoritative furnace slots per chunk, copied from the Go pin.
pub const FURNACES_PER_CHUNK: u8 = 32;

/// Fixed authoritative chest slots per chunk, copied from the Go pin.
pub const CHESTS_PER_CHUNK: u8 = 16;

/// Container kinds on the wire. Furnace is the zero value so existing
/// furnace references keep their meaning when the chest kind was added.
pub const CONTAINER_KIND_FURNACE: u8 = 0;
pub const CONTAINER_KIND_CHEST: u8 = 1;

/// Block identity of a fixed per-chunk container array. Furnaces and chests
/// share the same 18-byte wire layout, so only one encoding exists. The
/// generation counter distinguishes a reused slot from its old container.
///
/// The dimension stays the raw wire `i32`: both container arrays live in the
/// overworld, so a foreign dimension is an invalid real reference rather than
/// a value to narrow. Narrowing it at parse time would alias `256` or `-1`
/// into a valid `u8` dimension and publish a reference the authority would
/// reject, which is the raw-format loss the checked conversion below removes.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ContainerRef {
    pub dimension: i32,
    pub chunk_x: i32,
    pub chunk_z: i32,
    pub kind: u8,
    pub slot: u8,
    pub generation: u32,
}

impl ContainerRef {
    /// Zero reference carried by the inventory and crafting views, which
    /// address slots directly and must not point at a container block. The
    /// exact all-zero record is the only absent form: a zero generation
    /// beside a nonzero coordinate is a broken real reference, not absence.
    pub const NONE: Self = Self {
        dimension: 0,
        chunk_x: 0,
        chunk_z: 0,
        kind: 0,
        slot: 0,
        generation: 0,
    };

    /// Converts to the checked domain reference, rejecting the absent record
    /// and every invalid real reference.
    ///
    /// The dimension must be the overworld and the kind one of the two closed
    /// wire values; the chunk column, slot and generation then go through the
    /// domain constructor, which owns the per-kind slot bound and the
    /// nonzero-generation rule. The absent record fails here through its zero
    /// generation, so absence can never become a domain identity.
    pub fn to_domain_present(&self) -> Result<DomainContainerRef, ProtocolError> {
        if self.dimension != 0 {
            return Err(ProtocolError::InvalidRange);
        }
        let kind = match self.kind {
            CONTAINER_KIND_FURNACE => ContainerKind::Furnace,
            CONTAINER_KIND_CHEST => ContainerKind::Chest,
            _ => return Err(ProtocolError::InvalidEnum),
        };
        DomainContainerRef::try_new(
            ChunkPos::new(self.chunk_x, self.chunk_z),
            kind,
            self.slot,
            self.generation,
        )
        .map_err(|_| ProtocolError::InvalidRange)
    }

    /// Converts to the checked domain reference, mapping only the exact
    /// all-zero record to `None`.
    ///
    /// The equality test runs first so no partially zero record is mistaken
    /// for absence; every other record must be a valid real reference.
    pub fn to_domain_optional(&self) -> Result<Option<DomainContainerRef>, ProtocolError> {
        if *self == Self::NONE {
            return Ok(None);
        }
        self.to_domain_present().map(Some)
    }

    /// Validates the reference as a furnace: the kind must be the furnace
    /// value and the whole record must be a valid real reference.
    pub(crate) fn validate_furnace(&self) -> Result<(), ProtocolError> {
        if self.kind != CONTAINER_KIND_FURNACE {
            return Err(ProtocolError::InvalidEnum);
        }
        self.to_domain_present().map(|_| ())
    }

    /// Validates the reference as a chest: the kind must be the chest value
    /// and the whole record must be a valid real reference.
    pub(crate) fn validate_chest(&self) -> Result<(), ProtocolError> {
        if self.kind != CONTAINER_KIND_CHEST {
            return Err(ProtocolError::InvalidEnum);
        }
        self.to_domain_present().map(|_| ())
    }

    /// Validates a container-neutral reference: either known kind, with the
    /// slot bounds of that kind.
    pub(crate) fn validate_any(&self) -> Result<(), ProtocolError> {
        self.to_domain_present().map(|_| ())
    }

    /// The exact 18 wire bytes in the published field order.
    fn wire_bytes(&self) -> [u8; 18] {
        let mut bytes = [0u8; 18];
        bytes[0..4].copy_from_slice(&self.dimension.to_le_bytes());
        bytes[4..8].copy_from_slice(&self.chunk_x.to_le_bytes());
        bytes[8..12].copy_from_slice(&self.chunk_z.to_le_bytes());
        bytes[12] = self.kind;
        bytes[13] = self.slot;
        bytes[14..18].copy_from_slice(&self.generation.to_le_bytes());
        bytes
    }

    pub(crate) fn write(&self, encoder: &mut ByteEncoder) {
        encoder.bytes(&self.wire_bytes());
    }

    /// Publishes the same 18 bytes into a caller-owned publication window.
    ///
    /// The bytes come from one `wire_bytes` source, so the allocating encoder
    /// and the caller-owned window cannot drift apart on field order.
    pub(crate) fn write_into(&self, writer: &mut SliceWriter<'_>) {
        writer.bytes(&self.wire_bytes());
    }

    /// Reads the 18 raw wire bytes without validating them.
    ///
    /// Reference validity is a packet-level decision this node does not move:
    /// the families that reject a malformed real reference call
    /// [`ContainerRef::to_domain_present`] themselves, so a decode never
    /// narrows the dimension and never loses the byte the peer sent.
    pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<Self, ProtocolError> {
        let dimension = decoder.i32()?;
        let chunk_x = decoder.i32()?;
        let chunk_z = decoder.i32()?;
        let kind = decoder.u8()?;
        let slot = decoder.u8()?;
        let generation = decoder.u32()?;
        Ok(Self {
            dimension,
            chunk_x,
            chunk_z,
            kind,
            slot,
            generation,
        })
    }
}
