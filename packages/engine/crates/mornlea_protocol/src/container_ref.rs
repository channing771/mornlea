use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

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
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ContainerRef {
    pub dimension: u8,
    pub chunk_x: i32,
    pub chunk_z: i32,
    pub kind: u8,
    pub slot: u8,
    pub generation: u32,
}

impl ContainerRef {
    /// Zero reference carried by the inventory and crafting views, which
    /// address slots directly and must not point at a container block.
    pub const NONE: Self = Self {
        dimension: 0,
        chunk_x: 0,
        chunk_z: 0,
        kind: 0,
        slot: 0,
        generation: 0,
    };

    /// Both container kinds live in the overworld's fixed per-chunk arrays.
    /// Unknown kinds are `InvalidEnum`; out-of-range slots, zero
    /// generations, and foreign dimensions are `InvalidRange`.
    pub fn new(
        dimension: u8,
        chunk_x: i32,
        chunk_z: i32,
        kind: u8,
        slot: u8,
        generation: u32,
    ) -> Result<Self, ProtocolError> {
        match kind {
            CONTAINER_KIND_FURNACE => {
                if slot >= FURNACES_PER_CHUNK {
                    return Err(ProtocolError::InvalidRange);
                }
            }
            CONTAINER_KIND_CHEST => {
                if slot >= CHESTS_PER_CHUNK {
                    return Err(ProtocolError::InvalidRange);
                }
            }
            _ => return Err(ProtocolError::InvalidEnum),
        }
        if generation == 0 || dimension != 0 {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self {
            dimension,
            chunk_x,
            chunk_z,
            kind,
            slot,
            generation,
        })
    }

    /// Validates the reference as a furnace: the kind must be the furnace
    /// value and the slot must fit the fixed per-chunk furnace array.
    pub(crate) fn validate_furnace(&self) -> Result<(), ProtocolError> {
        if self.kind != CONTAINER_KIND_FURNACE {
            return Err(ProtocolError::InvalidEnum);
        }
        Self::new(
            self.dimension,
            self.chunk_x,
            self.chunk_z,
            self.kind,
            self.slot,
            self.generation,
        )
        .map(|_| ())
    }

    /// Validates the reference as a chest: the kind must be the chest value
    /// and the slot must fit the fixed per-chunk chest array.
    pub(crate) fn validate_chest(&self) -> Result<(), ProtocolError> {
        if self.kind != CONTAINER_KIND_CHEST {
            return Err(ProtocolError::InvalidEnum);
        }
        Self::new(
            self.dimension,
            self.chunk_x,
            self.chunk_z,
            self.kind,
            self.slot,
            self.generation,
        )
        .map(|_| ())
    }

    /// Validates a container-neutral reference: either known kind, with the
    /// slot bounds of that kind.
    pub(crate) fn validate_any(&self) -> Result<(), ProtocolError> {
        Self::new(
            self.dimension,
            self.chunk_x,
            self.chunk_z,
            self.kind,
            self.slot,
            self.generation,
        )
        .map(|_| ())
    }

    pub(crate) fn write(&self, encoder: &mut ByteEncoder) {
        encoder.i32(i32::from(self.dimension));
        encoder.i32(self.chunk_x);
        encoder.i32(self.chunk_z);
        encoder.u8(self.kind);
        encoder.u8(self.slot);
        encoder.u32(self.generation);
    }

    pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<Self, ProtocolError> {
        let dimension = u8::try_from(decoder.i32()?).unwrap_or(u8::MAX);
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
