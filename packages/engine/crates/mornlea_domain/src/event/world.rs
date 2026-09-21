//! The compact world observations: the initial chunk snapshot, the per-tick
//! block-change batch and the forget batch.
//!
//! These are the three world records an authoritative session publishes: the
//! snapshot is the whole 24-section column a subscriber receives when a chunk
//! becomes ready, the change batch is the tick delta that continues one chunk
//! history, and the forget batch retires chunks the session no longer
//! observes. Every rule below is the Go `protocol` validator's rule for the
//! same record, with two deliberate differences: the wire's section `Y` field
//! is its array index here, so the domain value carries no `Y` that could
//! disagree with its position, and the 4096 wire batch caps stay in the
//! protocol layer because they are transport budgets rather than semantic
//! relations.
//!
//! The conversion discipline is preservation: a codec conversion copies the
//! sections and the change order exactly as they are, and never recompresses a
//! palette, reorders a batch or repacks a section.

use crate::identity::DomainError;
use crate::locations::{BlockPos, ChunkPos};
use crate::sections::{
    MAX_Y, MIN_Y, PalettedSection, SECTIONS_PER_CHUNK, chunk_block_index, chunk_of,
    registered_block,
};
use crate::values::Dimension;

/// Parts of one chunk snapshot.
///
/// The sections are the fixed 24-section array, so the column length is part
/// of the type and the array order is the section order: position `i` is
/// section `i` of the column, which is why the value carries no Y field.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ChunkSnapshotParts {
    pub dimension: Dimension,
    pub chunk: ChunkPos,
    pub revision: u64,
    pub sections: Box<[PalettedSection; SECTIONS_PER_CHUNK]>,
}

/// The initial world mirror one subscriber receives for one chunk column.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ChunkSnapshot {
    dimension: Dimension,
    chunk: ChunkPos,
    revision: u64,
    sections: Box<[PalettedSection; SECTIONS_PER_CHUNK]>,
}

impl ChunkSnapshot {
    /// Wraps one snapshot, rejecting a zero revision.
    ///
    /// A zero revision names no point in the chunk's history, which is the Go
    /// `ChunkSnapshot.Validate` rule. The dimension is an already-validated
    /// `Dimension` and the sections are already-validated values inside the
    /// fixed array, so the revision is the only relation left to check.
    pub fn try_new(parts: ChunkSnapshotParts) -> Result<Self, DomainError> {
        if parts.revision == 0 {
            return Err(DomainError::InvalidRevision);
        }
        Ok(Self {
            dimension: parts.dimension,
            chunk: parts.chunk,
            revision: parts.revision,
            sections: parts.sections,
        })
    }

    pub fn dimension(&self) -> Dimension {
        self.dimension
    }

    pub fn chunk(&self) -> ChunkPos {
        self.chunk
    }

    pub fn revision(&self) -> u64 {
        self.revision
    }

    /// The column's sections in section order, as compact values.
    pub fn sections(&self) -> &[PalettedSection] {
        &self.sections[..]
    }
}

/// One block write inside one chunk history.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct BlockChange {
    position: BlockPos,
    block: u16,
}

impl BlockChange {
    /// Wraps one block write, rejecting an unregistered block.
    ///
    /// The position carries no rule of its own: the world span and the chunk
    /// membership are relations between the change and the batch that holds
    /// it, so they are checked by `BlockChanges::try_new` rather than here.
    pub fn try_new(position: BlockPos, block: u16) -> Result<Self, DomainError> {
        if !registered_block(block) {
            return Err(DomainError::InvalidBlock);
        }
        Ok(Self { position, block })
    }

    pub fn position(self) -> BlockPos {
        self.position
    }

    pub fn block(self) -> u16 {
        self.block
    }
}

/// Parts of one block-change batch.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BlockChangesParts {
    pub dimension: Dimension,
    pub chunk: ChunkPos,
    pub base_revision: u64,
    pub new_revision: u64,
    pub changes: Box<[BlockChange]>,
}

/// The per-tick world delta that continues one chunk history.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct BlockChanges {
    dimension: Dimension,
    chunk: ChunkPos,
    base_revision: u64,
    new_revision: u64,
    changes: Box<[BlockChange]>,
}

impl BlockChanges {
    /// Wraps one batch, checking the revision transition, the world span, the
    /// chunk membership and the canonical order.
    ///
    /// The revision has to continue one history: a base inside
    /// `1..=u64::MAX - 1` and a new revision of exactly `base + 1`. A base of
    /// zero names no history and a saturated base cannot be continued, so both
    /// are rejected rather than wrapped. Every change has to sit inside the
    /// announced chunk and inside the world's vertical span, and the batch has
    /// to be strictly increasing by chunk-ordered block index — the order the
    /// authority publishes, which also rules out a duplicate position.
    ///
    /// An empty batch is admitted: a tick that moves an item but writes no
    /// block still advances the revision, so the empty batch is the revision
    /// barrier the Go validator allows. The 4096 wire cap is not checked here,
    /// because it is a transport budget the protocol layer applies.
    pub fn try_new(parts: BlockChangesParts) -> Result<Self, DomainError> {
        if parts.base_revision == 0
            || parts.base_revision == u64::MAX
            || parts.new_revision != parts.base_revision + 1
        {
            return Err(DomainError::InvalidRevision);
        }
        let mut previous: Option<u32> = None;
        for change in &parts.changes {
            let position = change.position();
            if position.y() < MIN_Y || position.y() >= MAX_Y {
                return Err(DomainError::InvalidBlockY);
            }
            if chunk_of(position) != parts.chunk {
                return Err(DomainError::InvalidBlockChunk);
            }
            let index = chunk_block_index(position);
            if previous.is_some_and(|last| index <= last) {
                return Err(DomainError::InvalidChangeOrder);
            }
            previous = Some(index);
        }
        Ok(Self {
            dimension: parts.dimension,
            chunk: parts.chunk,
            base_revision: parts.base_revision,
            new_revision: parts.new_revision,
            changes: parts.changes,
        })
    }

    pub fn dimension(&self) -> Dimension {
        self.dimension
    }

    pub fn chunk(&self) -> ChunkPos {
        self.chunk
    }

    pub fn base_revision(&self) -> u64 {
        self.base_revision
    }

    pub fn new_revision(&self) -> u64 {
        self.new_revision
    }

    /// The batch's changes in the order the authority published them.
    pub fn changes(&self) -> &[BlockChange] {
        &self.changes
    }
}

/// Parts of one forget batch.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ForgetChunksParts {
    pub dimension: Dimension,
    pub chunks: Box<[ChunkPos]>,
}

/// The batch that retires chunks a session no longer observes.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ForgetChunks {
    dimension: Dimension,
    chunks: Box<[ChunkPos]>,
}

impl ForgetChunks {
    /// Wraps one forget batch, rejecting an empty batch and a duplicate chunk.
    ///
    /// The order is preserved exactly as given because the protocol does not
    /// demand a sorted batch: the authority groups and sorts when it publishes,
    /// so a replay has to observe the sequence the record carried. The
    /// uniqueness check therefore sorts a copy rather than the batch itself.
    /// The 4096 wire cap is a protocol budget and is not checked here.
    pub fn try_new(parts: ForgetChunksParts) -> Result<Self, DomainError> {
        if parts.chunks.is_empty() {
            return Err(DomainError::InvalidForgetChunks);
        }
        let mut ordered: Vec<ChunkPos> = parts.chunks.to_vec();
        ordered.sort_unstable();
        if ordered.windows(2).any(|pair| pair[0] == pair[1]) {
            return Err(DomainError::InvalidForgetChunks);
        }
        Ok(Self {
            dimension: parts.dimension,
            chunks: parts.chunks,
        })
    }

    pub fn dimension(&self) -> Dimension {
        self.dimension
    }

    /// The retired chunks in the order the record carried them.
    pub fn chunks(&self) -> &[ChunkPos] {
        &self.chunks
    }
}
