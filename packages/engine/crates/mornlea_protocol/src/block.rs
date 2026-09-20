//! Registered block numbering and the world geometry the packet families
//! share.
//!
//! Block IDs are protocol-stable values, so this module only publishes the
//! exclusive upper bound and the geometry rules the protocol layer needs:
//! which vertical span is inside the world, and how a world position maps to
//! the chunk-ordered block index that sorted block-change batches compare.

/// Air is the zero block, the empty value of a single-storage section.
pub const BLOCK_AIR: u16 = 0;

/// Stone, the fixture block used by the frozen golden payloads.
pub const BLOCK_STONE: u16 = 2;

/// Exclusive upper bound of registered block numbers, copied from the Go
/// `BlockIDMax` sentinel.
pub const BLOCK_ID_MAX: u16 = 90;

/// Lowest block Y coordinate inside the world, inclusive.
pub const MIN_Y: i32 = -64;

/// Exclusive upper bound of block Y coordinates inside the world.
pub const MAX_Y: i32 = 320;

/// Section edge length in blocks.
pub const SECTION_SIZE: i32 = 16;

/// Number of sections in a chunk column.
pub const SECTIONS_PER_CHUNK: usize = 24;

/// Number of blocks in one section.
pub const BLOCKS_PER_SECTION: usize = 4096;

/// Reports whether a block ID is registered.
pub fn registered_block(block: u16) -> bool {
    block < BLOCK_ID_MAX
}

/// Reports whether a block Y coordinate is inside the world's vertical span.
pub fn inside_world(y: i32) -> bool {
    (MIN_Y..MAX_Y).contains(&y)
}

/// Chunk column coordinates of a world position. The shift is arithmetic so
/// negative coordinates floor toward negative infinity instead of toward
/// zero, matching the Go `BlockPos.Chunk` rule.
pub fn chunk_of(x: i32, z: i32) -> (i32, i32) {
    (x >> 4, z >> 4)
}

/// Section index of a block Y coordinate inside its chunk column. Callers
/// must keep the coordinate inside the world span.
pub fn section_index(y: i32) -> usize {
    (((y - MIN_Y) >> 4) as usize) % SECTIONS_PER_CHUNK
}

/// Chunk-ordered block index of a world position. Sorted block-change
/// batches compare this value, so the local coordinate decomposition has to
/// match the Go `chunkBlockIndex` rule exactly.
pub fn chunk_block_index(x: i32, y: i32, z: i32) -> u32 {
    let (local_x, local_y, local_z) = (x & 15, (y - MIN_Y) & 15, z & 15);
    (section_index(y) * BLOCKS_PER_SECTION
        + local_y as usize * 256
        + local_z as usize * 16
        + local_x as usize) as u32
}

/// Exclusive upper bound of the chunk-ordered block index, the value an
/// item drop's block index must stay below.
pub const MAX_CHUNK_BLOCK_INDEX: u32 = (SECTIONS_PER_CHUNK * BLOCKS_PER_SECTION) as u32;
