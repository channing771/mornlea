//! Chunk-scoped position and identity values: a chunk column, an authoritative
//! drop identity, and a container reference.
//!
//! A drop identity keeps an arbitrary raw dimension because the Go
//! `core.DropID.Valid` rule checks only the slot range and the generation, so a
//! stricter Rust rule would reject a drop the authority still publishes. A
//! container reference is the opposite: it is inherently an overworld value, so
//! the raw wire dimension is validated before construction and the constructor
//! itself carries no dimension to get wrong. Absence of a container is expressed
//! as `Option<ContainerRef>`; this crate publishes no invalid reference and no
//! sentinel value.

use crate::identity::DomainError;

/// Authoritative drop slots one chunk holds, from the Go
/// `core.DropsPerChunk`.
const DROPS_PER_CHUNK: u8 = 32;

/// Fixed furnace array size of one chunk, from the Go `core.FurnacesPerChunk`.
const FURNACES_PER_CHUNK: u8 = 32;

/// Fixed chest array size of one chunk, from the Go `core.ChestsPerChunk`.
const CHESTS_PER_CHUNK: u8 = 16;

/// Chunk column coordinate pair.
///
/// The coordinates are stored exactly as received with no normalization,
/// clamping or range check, because the Go `core.ChunkPos` imposes none and a
/// world coordinate that was legal before a replay has to stay legal. The type
/// carries no invariant, so construction is total and named `new` rather than
/// `try_new`.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct ChunkPos {
    x: i32,
    z: i32,
}

impl ChunkPos {
    pub fn new(x: i32, z: i32) -> Self {
        Self { x, z }
    }

    pub fn x(self) -> i32 {
        self.x
    }

    pub fn z(self) -> i32 {
        self.z
    }
}

/// Stable identity of one authoritative drop for its whole lifetime.
///
/// The field declaration order is the Go `core.DropID.Compare` total order:
/// dimension, then chunk column, then slot, then generation. Deriving `Ord`
/// over that order is deliberate, so a sorted batch of drops compares the same
/// way in both languages.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct DropId {
    dimension: i32,
    chunk: ChunkPos,
    slot: u8,
    generation: u32,
}

impl DropId {
    /// Wraps a drop identity, rejecting a slot at or above the per-chunk slot
    /// count and a zero generation, which is the exact Go
    /// `core.DropID.Valid` rule.
    ///
    /// The dimension is deliberately unchecked: the Go rule checks only the
    /// slot range and the generation, so a drop naming an unusual dimension is
    /// still a publishable identity and rejecting it here would be a parity
    /// break rather than a tightening.
    pub fn try_new(
        dimension: i32,
        chunk: ChunkPos,
        slot: u8,
        generation: u32,
    ) -> Result<Self, DomainError> {
        if slot >= DROPS_PER_CHUNK {
            return Err(DomainError::InvalidDropSlot);
        }
        if generation == 0 {
            return Err(DomainError::InvalidDropGeneration);
        }
        Ok(Self {
            dimension,
            chunk,
            slot,
            generation,
        })
    }

    pub fn dimension(self) -> i32 {
        self.dimension
    }

    pub fn chunk(self) -> ChunkPos {
        self.chunk
    }

    pub fn slot(self) -> u8 {
        self.slot
    }

    pub fn generation(self) -> u32 {
        self.generation
    }
}

/// Container array a container reference addresses.
///
/// The furnace variant is the zero value, matching the Go
/// `core.ContainerKindFurnace`: existing furnace references construct without
/// naming a kind, so the first variant has to keep that meaning.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub enum ContainerKind {
    Furnace,
    Chest,
}

/// Stable identity of one container for its whole lifetime.
///
/// A reference is inherently an overworld value: both container arrays live in
/// the overworld's fixed per-chunk storage, so the protocol conversion
/// validates the raw wire dimension before it constructs one and this type
/// carries no dimension field that could disagree.
#[derive(Clone, Copy, Debug, Eq, Hash, Ord, PartialEq, PartialOrd)]
pub struct ContainerRef {
    chunk: ChunkPos,
    kind: ContainerKind,
    slot: u8,
    generation: u32,
}

impl ContainerRef {
    /// Wraps a container reference, rejecting a slot outside the addressed
    /// kind's fixed per-chunk array and a zero generation, which is the Go
    /// `validFurnaceRef` / `validChestRef` rule without its dimension check.
    pub fn try_new(
        chunk: ChunkPos,
        kind: ContainerKind,
        slot: u8,
        generation: u32,
    ) -> Result<Self, DomainError> {
        let slots = match kind {
            ContainerKind::Furnace => FURNACES_PER_CHUNK,
            ContainerKind::Chest => CHESTS_PER_CHUNK,
        };
        if slot >= slots {
            return Err(DomainError::InvalidContainerSlot);
        }
        if generation == 0 {
            return Err(DomainError::InvalidContainerGeneration);
        }
        Ok(Self {
            chunk,
            kind,
            slot,
            generation,
        })
    }

    pub fn chunk(self) -> ChunkPos {
        self.chunk
    }

    pub fn kind(self) -> ContainerKind {
        self.kind
    }

    pub fn slot(self) -> u8 {
        self.slot
    }

    pub fn generation(self) -> u32 {
        self.generation
    }
}
