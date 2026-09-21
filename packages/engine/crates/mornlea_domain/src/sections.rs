//! Registered block numbering, the world and section geometry, and the compact
//! section storage value.
//!
//! Every rule here is ported from the Go `core` geometry and the Go
//! `protocol.SectionData.Validate` section rules, which the protocol crate's
//! `block.rs` mirrors while that crate still owns the wire codecs. This crate
//! sits below `mornlea_protocol` and cannot import it, so the numbering lives
//! here as the domain's single copy: the tests pin every value against the Go
//! rule, and a protocol port consumes this copy instead of growing a second
//! one. A section keeps the compact representation the wire and the save
//! format use, so a snapshot never expands into 4096 block IDs at the contract
//! boundary and a codec conversion never recompresses or reorders a palette.

use crate::identity::DomainError;
use crate::locations::{BlockPos, ChunkPos};

/// Exclusive upper bound of registered block numbers, from the Go
/// `core.BlockIDMax` sentinel.
const BLOCK_ID_MAX: u16 = 90;

/// Section edge length in blocks, from the Go `core.SectionSize`.
const SECTION_SIZE: i32 = 16;

/// Logarithm of `SECTION_SIZE` base two, from the Go `core.SectionShift`.
const SECTION_SHIFT: i32 = 4;

/// Mask that extracts a section-local coordinate, from `core.SectionMask`.
const SECTION_MASK: i32 = SECTION_SIZE - 1;

/// Lowest block Y coordinate inside the world, inclusive (`core.MinY`).
pub(crate) const MIN_Y: i32 = -64;

/// Exclusive upper bound of block Y coordinates inside the world (`core.MaxY`).
pub(crate) const MAX_Y: i32 = 320;

/// Number of sections in a chunk column (`core.SectionsPerChunk`).
pub(crate) const SECTIONS_PER_CHUNK: usize = 24;

/// Number of blocks in one section (`core.BlocksPerSection`).
const BLOCKS_PER_SECTION: usize = 4096;

/// Reports whether a block number is registered.
///
/// The bound is the exclusive sentinel the Go `core.RegisteredBlock` applies,
/// so air and the last real block are registered while the sentinel itself and
/// everything above it are not.
pub fn registered_block(block: u16) -> bool {
    block < BLOCK_ID_MAX
}

/// Section index of a block Y coordinate inside its chunk column.
///
/// The caller keeps the coordinate inside the world span, exactly as the Go
/// `BlockPos.SectionIndex` contract requires, so the shift needs no modulo
/// guard here.
fn section_index(y: i32) -> usize {
    ((y - MIN_Y) >> SECTION_SHIFT) as usize
}

/// Chunk column of a world position, from the Go `BlockPos.Chunk` rule.
///
/// The shift is arithmetic so a negative coordinate floors toward negative
/// infinity: `-1` belongs to chunk `-1` rather than to chunk `0`, which is what
/// makes a negative block still name its own chunk.
pub(crate) fn chunk_of(position: BlockPos) -> ChunkPos {
    ChunkPos::new(position.x() >> SECTION_SHIFT, position.z() >> SECTION_SHIFT)
}

/// Chunk-ordered block index of a world position.
///
/// Sorted block-change batches compare this value, so the decomposition is the
/// Go `chunkBlockIndex` rule exactly: the section index, then the
/// section-local `y * 16 * 16 + z * 16 + x` with every local coordinate masked
/// into `0..15`.
pub fn chunk_block_index(position: BlockPos) -> u32 {
    let local_x = (position.x() & SECTION_MASK) as usize;
    let local_y = ((position.y() - MIN_Y) & SECTION_MASK) as usize;
    let local_z = (position.z() & SECTION_MASK) as usize;
    (section_index(position.y()) * BLOCKS_PER_SECTION
        + local_y * SECTION_SIZE as usize * SECTION_SIZE as usize
        + local_z * SECTION_SIZE as usize
        + local_x) as u32
}

/// Number of packed words one section needs at a given slot width.
///
/// 4096 cells divide evenly by every published width, so the quotient is exact
/// and a section has neither a partial final word nor a tail entry.
fn section_words(bits: u8) -> usize {
    let per_word = 64 / bits as usize;
    BLOCKS_PER_SECTION.div_ceil(per_word)
}

/// Reads the slot one cell names, in the Go `ReadSectionPacked` layout:
/// `64 / bits` slots per word, the low slot first.
fn read_slot(words: &[u64], bits: u8, index: usize) -> u32 {
    let per_word = 64 / bits as usize;
    let shift = (index % per_word) * bits as usize;
    ((words[index / per_word] >> shift) & ((1u64 << bits) - 1)) as u32
}

/// One section's compact block storage.
///
/// The value keeps the representation the wire and the save format use, and
/// every constructor is fallible: a section this crate publishes is a section
/// the protocol layer admits, because the Go `SectionData.Validate` rules are
/// checked here in the order that validator checks them. `block_at` reads one
/// cell straight out of the packed layout, and the variant accessors hand out
/// borrowed slices, so a codec conversion copies the representation as it is.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PalettedSection {
    storage: SectionStorage,
}

/// The three compact representations one section can hold.
///
/// The enum is private because the variants carry the validation contract with
/// them: the only way to build an `Indexed4` is the checked `indexed`
/// constructor, so no caller can assemble a palette that is too long or a word
/// count that does not match its width.
#[derive(Clone, Debug, Eq, PartialEq)]
enum SectionStorage {
    Single(u16),
    Indexed4 {
        palette: Box<[u16]>,
        words: Box<[u64]>,
    },
    Indexed8 {
        palette: Box<[u16]>,
        words: Box<[u64]>,
    },
    Direct15 {
        words: Box<[u64]>,
    },
}

impl PalettedSection {
    /// Wraps one single-storage section, rejecting an unregistered block.
    ///
    /// Air is the zero block and is registered, so the empty section is the
    /// `single(0)` form. A single section carries no palette and no packed
    /// words, which the representation itself guarantees: there is no field
    /// left for a caller to fill with residue.
    pub fn single(block: u16) -> Result<Self, DomainError> {
        if !registered_block(block) {
            return Err(DomainError::InvalidBlock);
        }
        Ok(Self {
            storage: SectionStorage::Single(block),
        })
    }

    /// Wraps one indexed section at 4 or 8 bits per slot.
    ///
    /// The rules are checked in the Go validator's order: the slot width, the
    /// palette bounds, the palette uniqueness and registration, the exact word
    /// count, and finally that every packed slot resolves inside the palette.
    /// The palette is kept exactly as given, so a conversion never reorders or
    /// recompresses it.
    pub fn indexed(bits: u8, palette: Box<[u16]>, words: Box<[u64]>) -> Result<Self, DomainError> {
        if bits != 4 && bits != 8 {
            return Err(DomainError::InvalidSectionBits);
        }
        if palette.is_empty() || palette.len() > 1usize << bits {
            return Err(DomainError::InvalidSectionPalette);
        }
        // The table is indexed by block number rather than by palette
        // position, so a duplicate entry is a direct hit instead of a second
        // search, and 90 bytes of stack replace an allocation per section.
        let mut seen = [false; BLOCK_ID_MAX as usize];
        for id in &palette {
            if !registered_block(*id) {
                return Err(DomainError::InvalidBlock);
            }
            if seen[*id as usize] {
                return Err(DomainError::InvalidSectionPalette);
            }
            seen[*id as usize] = true;
        }
        if words.len() != section_words(bits) {
            return Err(DomainError::InvalidSectionWords);
        }
        for index in 0..BLOCKS_PER_SECTION {
            if read_slot(&words, bits, index) as usize >= palette.len() {
                return Err(DomainError::InvalidSectionSlot);
            }
        }
        let storage = if bits == 4 {
            SectionStorage::Indexed4 { palette, words }
        } else {
            SectionStorage::Indexed8 { palette, words }
        };
        Ok(Self { storage })
    }

    /// Wraps one direct section of 15-bit slots.
    ///
    /// The word count is exact — 1024 words hold 4096 slots at four slots per
    /// word — and the top four bits of every word are unused, so a word
    /// carrying them is rejected rather than masked away. Every decoded value
    /// has to be a registered block, which keeps a direct section from
    /// publishing a number no palette entry could name.
    pub fn direct(words: Box<[u64]>) -> Result<Self, DomainError> {
        if words.len() != section_words(15) {
            return Err(DomainError::InvalidSectionWords);
        }
        for word in &words {
            if word >> 60 != 0 {
                return Err(DomainError::InvalidSectionHighBits);
            }
        }
        for index in 0..BLOCKS_PER_SECTION {
            if !registered_block(read_slot(&words, 15, index) as u16) {
                return Err(DomainError::InvalidBlock);
            }
        }
        Ok(Self {
            storage: SectionStorage::Direct15 { words },
        })
    }

    /// Reads the block one cell holds, or `None` past the last cell.
    ///
    /// The read follows the packed layout directly, so a caller that wants one
    /// block never pays for the whole section to be expanded.
    pub fn block_at(&self, index: usize) -> Option<u16> {
        if index >= BLOCKS_PER_SECTION {
            return None;
        }
        Some(match &self.storage {
            SectionStorage::Single(block) => *block,
            SectionStorage::Indexed4 { palette, words } => {
                palette[read_slot(words, 4, index) as usize]
            }
            SectionStorage::Indexed8 { palette, words } => {
                palette[read_slot(words, 8, index) as usize]
            }
            SectionStorage::Direct15 { words } => read_slot(words, 15, index) as u16,
        })
    }

    /// The single-storage block, or `None` for a packed representation.
    pub fn as_single(&self) -> Option<u16> {
        match &self.storage {
            SectionStorage::Single(block) => Some(*block),
            _ => None,
        }
    }

    /// The indexed representation: its slot width, its palette and its packed
    /// words, or `None` for a representation that is not indexed.
    pub fn as_indexed(&self) -> Option<(u8, &[u16], &[u64])> {
        match &self.storage {
            SectionStorage::Indexed4 { palette, words } => Some((4, &palette[..], &words[..])),
            SectionStorage::Indexed8 { palette, words } => Some((8, &palette[..], &words[..])),
            _ => None,
        }
    }

    /// The direct representation's packed words, or `None` for a
    /// representation that is not direct.
    pub fn as_direct(&self) -> Option<&[u64]> {
        match &self.storage {
            SectionStorage::Direct15 { words } => Some(&words[..]),
            _ => None,
        }
    }
}
