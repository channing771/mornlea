//! Compact chunk observation records for `mornlea_domain`.
//!
//! The Go baseline is the `protocol.ChunkSnapshot`, `protocol.BlockChanges`
//! and `protocol.ForgetChunks` wire records in
//! `packages/shared/network/protocol`: the chunk snapshot is the initial world
//! mirror one subscriber receives, the block-change batch is the per-tick
//! world delta, and the forget batch retires chunks a session no longer
//! observes. The section storage rules are the Go `protocol.SectionData.Validate`
//! rules, and the block numbering and the world geometry are the Go `core`
//! rules the protocol crate's `block.rs` mirrors. Every value rule below is
//! therefore the Go validator's rule, so a record this crate admits is a record
//! the protocol layer admits and the other way around.
//!
//! Two Go rules deliberately stay out of this file. A section's `Y` field is
//! its array index, so the domain value carries no `Y` that could disagree
//! with its position; and the 4096 wire batch caps are transport budgets the
//! protocol layer applies, not semantic relations these records own.

use mornlea_domain::{
    BlockChange, BlockChanges, BlockChangesParts, BlockPos, ChunkPos, ChunkSnapshot,
    ChunkSnapshotParts, Dimension, DomainError, ForgetChunks, ForgetChunksParts, PalettedSection,
    chunk_block_index, registered_block,
};

/// Cells one section holds, from the Go `core.BlocksPerSection`.
const SECTION_CELLS: usize = 4096;

/// Sections one chunk column holds, from the Go `core.SectionsPerChunk`.
const CHUNK_SECTIONS: usize = 24;

/// Builds the packed words of one section from its per-cell slots.
///
/// The layout is the Go `ReadSectionPacked` rule: `64 / bits` slots per word,
/// the low slot first, so cell `index` lives at word `index / per_word`
/// shifted by `(index % per_word) * bits`.
fn packed_words(bits: u8, slots: &[(usize, u64)]) -> Vec<u64> {
    let per_word = 64 / bits as usize;
    let mut words = vec![0u64; SECTION_CELLS.div_ceil(per_word)];
    for &(index, slot) in slots {
        words[index / per_word] |= slot << ((index % per_word) * bits as usize);
    }
    words
}

/// One single-storage section holding air, the empty value of a column.
fn air_section() -> PalettedSection {
    PalettedSection::single(0).expect("air is the registered zero block")
}

/// Renders one section list as the fixed array a snapshot carries.
///
/// The conversion is the checked `Box` conversion the node names: a list that
/// is not exactly `CHUNK_SECTIONS` long fails here instead of being padded or
/// truncated, and no large stack array is built on the way.
fn chunk_sections(mut sections: Vec<PalettedSection>) -> Box<[PalettedSection; CHUNK_SECTIONS]> {
    sections.resize(CHUNK_SECTIONS, air_section());
    sections
        .try_into()
        .expect("exactly one section per chunk column index")
}

#[test]
fn event_world_single_air_section_is_admitted_and_reads_every_cell() {
    let section = PalettedSection::single(0).expect("air is the registered zero block");

    assert_eq!(section.as_single(), Some(0));
    assert_eq!(section.as_indexed(), None);
    assert_eq!(section.as_direct(), None);
    assert_eq!(section.block_at(0), Some(0));
    assert_eq!(section.block_at(SECTION_CELLS - 1), Some(0));
    assert_eq!(section.block_at(SECTION_CELLS), None);

    // Air is the zero block and 89 is the last registered number: 90 is the
    // Go `BlockIDMax` sentinel, so a single section naming it publishes a
    // block no palette entry could name either.
    assert!(registered_block(0));
    assert!(registered_block(89));
    assert!(!registered_block(90));
    let last = PalettedSection::single(89).expect("the last registered block");
    assert_eq!(last.as_single(), Some(89));
    assert_eq!(
        PalettedSection::single(90),
        Err(DomainError::InvalidBlock),
        "an unregistered single block must fail the conversion"
    );
}

#[test]
fn event_world_indexed4_palette_two_with_one_cell_set() {
    // Cell 1 names palette slot 1 and every other cell names slot 0, which is
    // the exact 4-bit layout: sixteen cells per word, 256 words per section.
    let words = packed_words(4, &[(1, 1)]);
    let section = PalettedSection::indexed(
        4,
        vec![0u16, 1].into_boxed_slice(),
        words.into_boxed_slice(),
    )
    .expect("a two-entry palette with 256 words is the exact 4-bit layout");

    let (bits, palette, words) = section.as_indexed().expect("the indexed union member");
    assert_eq!(bits, 4);
    assert_eq!(palette.to_vec(), vec![0, 1]);
    assert_eq!(words.len(), 256);
    assert_eq!(words[0], 1 << 4);
    assert_eq!(section.as_single(), None);
    assert_eq!(section.as_direct(), None);
    assert_eq!(section.block_at(0), Some(0));
    assert_eq!(section.block_at(1), Some(1));
    assert_eq!(section.block_at(SECTION_CELLS - 1), Some(0));
    assert_eq!(section.block_at(SECTION_CELLS), None);
}

#[test]
fn event_world_indexed8_ninety_registered_ids() {
    // Ninety entries is the whole registered numbering, and 90 is inside the
    // 256 entries an 8-bit palette holds, so every slot resolves.
    let palette: Vec<u16> = (0..90).collect();
    let words = packed_words(8, &[(0, 89), (SECTION_CELLS - 1, 0)]);
    let section = PalettedSection::indexed(8, palette.into_boxed_slice(), words.into_boxed_slice())
        .expect("ninety registered IDs fit an 8-bit palette");

    let (bits, palette, words) = section.as_indexed().expect("the indexed union member");
    assert_eq!(bits, 8);
    assert_eq!(palette.len(), 90);
    assert_eq!(palette[89], 89);
    assert_eq!(words.len(), 512);
    assert_eq!(section.block_at(0), Some(89));
    assert_eq!(section.block_at(SECTION_CELLS - 1), Some(0));
}

#[test]
fn event_world_direct15_block_89_at_the_last_cell() {
    // A direct section carries the block number itself in a 15-bit slot, four
    // slots per word, so the last cell is the top slot of the last word and
    // the three slots below it stay zero.
    let words = packed_words(15, &[(SECTION_CELLS - 1, 89)]);
    let section = PalettedSection::direct(words.into_boxed_slice())
        .expect("1024 words of registered 15-bit slots is the exact direct layout");

    let words = section.as_direct().expect("the direct union member");
    assert_eq!(words.len(), 1024);
    assert_eq!(words[1023], 89u64 << 45);
    assert_eq!(section.as_single(), None);
    assert_eq!(section.as_indexed(), None);
    assert_eq!(section.block_at(0), Some(0));
    assert_eq!(section.block_at(SECTION_CELLS - 1), Some(89));
    assert_eq!(section.block_at(SECTION_CELLS), None);

    // A direct slot naming the sentinel is unregistered, exactly as a palette
    // entry naming it is.
    let words = packed_words(15, &[(0, 90)]);
    assert_eq!(
        PalettedSection::direct(words.into_boxed_slice()),
        Err(DomainError::InvalidBlock),
        "a direct slot naming an unregistered block must fail the conversion"
    );
}

#[test]
fn event_world_indexed4_word_counts_255_256_257() {
    let palette = vec![0u16, 1].into_boxed_slice();

    // 4096 cells at sixteen cells per word is exactly 256 words. The count is
    // an exact layout rather than a capacity, so one word fewer and one word
    // more are both rejections instead of a silently padded section.
    assert_eq!(
        PalettedSection::indexed(4, palette.clone(), vec![0u64; 255].into_boxed_slice()),
        Err(DomainError::InvalidSectionWords)
    );
    PalettedSection::indexed(4, palette.clone(), vec![0u64; 256].into_boxed_slice())
        .expect("256 words is the exact 4-bit layout");
    assert_eq!(
        PalettedSection::indexed(4, palette, vec![0u64; 257].into_boxed_slice()),
        Err(DomainError::InvalidSectionWords)
    );
}

#[test]
fn event_world_indexed_palette_rejects_duplicate_unregistered_and_oversized_entries() {
    let words = || vec![0u64; 256].into_boxed_slice();

    // A duplicate entry would leave two slots naming one block, which the Go
    // validator rejects so a palette stays a bijection onto its slots.
    assert_eq!(
        PalettedSection::indexed(4, vec![0u16, 0].into_boxed_slice(), words()),
        Err(DomainError::InvalidSectionPalette)
    );
    // Block 90 is the first unregistered number, so a palette naming it holds
    // a slot that resolves to no block.
    assert_eq!(
        PalettedSection::indexed(4, vec![0u16, 90].into_boxed_slice(), words()),
        Err(DomainError::InvalidBlock)
    );
    // Seventeen entries do not fit a 4-bit palette, whose capacity is 16.
    assert_eq!(
        PalettedSection::indexed(
            4,
            (0..17u16).collect::<Vec<_>>().into_boxed_slice(),
            words()
        ),
        Err(DomainError::InvalidSectionPalette)
    );
    // An empty palette leaves every slot unresolved.
    assert_eq!(
        PalettedSection::indexed(4, Vec::new().into_boxed_slice(), words()),
        Err(DomainError::InvalidSectionPalette)
    );
    // A width outside 4 and 8 has no word layout at all, so it is rejected
    // before the palette and the words are read.
    assert_eq!(
        PalettedSection::indexed(5, vec![0u16, 1].into_boxed_slice(), words()),
        Err(DomainError::InvalidSectionBits)
    );
}

#[test]
fn event_world_indexed_slot_outside_the_palette_is_rejected() {
    // Cell 5 names palette slot 2, but the palette holds two entries, so the
    // slot resolves to nothing: the Go validator rejects it instead of
    // reading a zero block.
    let words = packed_words(4, &[(5, 2)]);
    assert_eq!(
        PalettedSection::indexed(
            4,
            vec![0u16, 1].into_boxed_slice(),
            words.into_boxed_slice()
        ),
        Err(DomainError::InvalidSectionSlot)
    );
}

#[test]
fn event_world_direct_word_high_bits_are_rejected() {
    // A 15-bit slot leaves the top four bits of every word unused, so a word
    // carrying them describes no legal section and is rejected rather than
    // masked away.
    let mut words = packed_words(15, &[(0, 89)]);
    words[7] |= 1 << 60;
    assert_eq!(
        PalettedSection::direct(words.into_boxed_slice()),
        Err(DomainError::InvalidSectionHighBits)
    );
    // The direct word count is exact as well: 1024 words hold 4096 slots at
    // four slots per word.
    assert_eq!(
        PalettedSection::direct(vec![0u64; 1023].into_boxed_slice()),
        Err(DomainError::InvalidSectionWords)
    );
}

#[test]
fn event_world_chunk_snapshot_preserves_section_23() {
    // The last section of the column is the direct case, and the array order
    // is the section order: Y is implicit in the position, so the snapshot
    // carries no Y field to disagree with its index.
    let mut sections = vec![air_section(); CHUNK_SECTIONS - 1];
    let last =
        PalettedSection::direct(packed_words(15, &[(SECTION_CELLS - 1, 89)]).into_boxed_slice())
            .expect("the exact direct layout");
    sections.push(last.clone());

    let admitted = ChunkSnapshot::try_new(ChunkSnapshotParts {
        dimension: Dimension::DEPTHS,
        chunk: ChunkPos::new(-1, -1),
        revision: 41,
        sections: chunk_sections(sections),
    })
    .expect("a nonzero revision with a full column is admitted");

    assert_eq!(admitted.dimension(), Dimension::DEPTHS);
    assert_eq!(admitted.chunk(), ChunkPos::new(-1, -1));
    assert_eq!(admitted.revision(), 41);
    let sections = admitted.sections();
    assert_eq!(sections.len(), CHUNK_SECTIONS);
    assert_eq!(sections[0].as_single(), Some(0));
    assert_eq!(sections[CHUNK_SECTIONS - 1], last);
    assert_eq!(
        sections[CHUNK_SECTIONS - 1].block_at(SECTION_CELLS - 1),
        Some(89)
    );

    // A zero revision names no point in the chunk's history.
    assert_eq!(
        ChunkSnapshot::try_new(ChunkSnapshotParts {
            dimension: Dimension::OVERWORLD,
            chunk: ChunkPos::new(0, 0),
            revision: 0,
            sections: chunk_sections(Vec::new()),
        }),
        Err(DomainError::InvalidRevision)
    );
    // The fixed array type is the section-count gate: a column one section
    // short cannot become the array at all, and no padding invents the
    // missing section.
    let short: Result<Box<[PalettedSection; CHUNK_SECTIONS]>, _> =
        vec![air_section(); CHUNK_SECTIONS - 1].try_into();
    assert!(
        short.is_err(),
        "a 23-section column must not convert into the fixed array"
    );
}

#[test]
fn event_world_block_changes_empty_batch_is_a_revision_barrier() {
    // A tick that moves an item but writes no block still advances the
    // revision, so the empty batch is the barrier the Go validator allows.
    let changes = BlockChanges::try_new(BlockChangesParts {
        dimension: Dimension::OVERWORLD,
        chunk: ChunkPos::new(0, 0),
        base_revision: 5,
        new_revision: 6,
        changes: Vec::new().into_boxed_slice(),
    })
    .expect("an empty batch is a legal revision barrier");

    assert_eq!(changes.dimension(), Dimension::OVERWORLD);
    assert_eq!(changes.chunk(), ChunkPos::new(0, 0));
    assert_eq!(changes.base_revision(), 5);
    assert_eq!(changes.new_revision(), 6);
    assert!(changes.changes().is_empty());
}

#[test]
fn event_world_block_changes_negative_chunk_and_world_local_index() {
    // The chunk column (-1,-1) holds world x and z in -16..-1 and the world Y
    // span starts at -64, so the column's two extreme corners are the first
    // and the last chunk-ordered index of its first section. The arithmetic
    // shift is what puts -1 into chunk -1 rather than into chunk 0.
    assert_eq!(chunk_block_index(BlockPos::new(-16, -64, -16)), 0);
    assert_eq!(chunk_block_index(BlockPos::new(-1, -64, -1)), 255);
    assert_eq!(
        chunk_block_index(BlockPos::new(-1, 319, -1)),
        SECTION_CELLS as u32 * CHUNK_SECTIONS as u32 - 1
    );

    let changes = BlockChanges::try_new(BlockChangesParts {
        dimension: Dimension::OVERWORLD,
        chunk: ChunkPos::new(-1, -1),
        base_revision: 1,
        new_revision: 2,
        changes: vec![
            BlockChange::try_new(BlockPos::new(-16, -64, -16), 2).expect("stone is registered"),
            BlockChange::try_new(BlockPos::new(-1, -64, -1), 3).expect("a registered block"),
        ]
        .into_boxed_slice(),
    })
    .expect("the negative column orders by its local index");

    assert_eq!(changes.chunk(), ChunkPos::new(-1, -1));
    assert_eq!(changes.changes().len(), 2);
    assert_eq!(changes.changes()[0].block(), 2);
    assert_eq!(
        changes.changes()[0].position(),
        BlockPos::new(-16, -64, -16)
    );
    assert_eq!(changes.changes()[1].position(), BlockPos::new(-1, -64, -1));
    assert_eq!(changes.changes()[1].block(), 3);
}

#[test]
fn event_world_block_changes_reject_duplicate_and_unsorted_positions() {
    // A duplicate position names one index twice and a reversed pair is not
    // the increasing order the authority publishes. They are the same broken
    // relation, because a strictly increasing order already rules out a
    // duplicate.
    for changes in [
        vec![
            BlockChange::try_new(BlockPos::new(-16, -64, -16), 2).expect("stone is registered"),
            BlockChange::try_new(BlockPos::new(-16, -64, -16), 3).expect("a registered block"),
        ],
        vec![
            BlockChange::try_new(BlockPos::new(-1, -64, -1), 2).expect("stone is registered"),
            BlockChange::try_new(BlockPos::new(-16, -64, -16), 3).expect("a registered block"),
        ],
    ] {
        assert_eq!(
            BlockChanges::try_new(BlockChangesParts {
                dimension: Dimension::OVERWORLD,
                chunk: ChunkPos::new(-1, -1),
                base_revision: 1,
                new_revision: 2,
                changes: changes.into_boxed_slice(),
            }),
            Err(DomainError::InvalidChangeOrder)
        );
    }
}

#[test]
fn event_world_block_changes_revision_relations_are_exact() {
    let batch = |base_revision: u64, new_revision: u64| {
        BlockChanges::try_new(BlockChangesParts {
            dimension: Dimension::OVERWORLD,
            chunk: ChunkPos::new(0, 0),
            base_revision,
            new_revision,
            changes: Vec::new().into_boxed_slice(),
        })
    };

    // The transition has to continue one history exactly: a base of zero
    // names no history, a saturated base cannot be continued, and a new
    // revision that is not base + 1 skips or repeats one.
    assert!(batch(1, 2).is_ok());
    assert_eq!(batch(0, 1).unwrap_err(), DomainError::InvalidRevision);
    assert_eq!(
        batch(u64::MAX, 0).unwrap_err(),
        DomainError::InvalidRevision
    );
    assert_eq!(batch(1, 3).unwrap_err(), DomainError::InvalidRevision);
    assert_eq!(batch(1, 1).unwrap_err(), DomainError::InvalidRevision);
}

#[test]
fn event_world_block_changes_reject_positions_outside_the_chunk_or_the_world() {
    let batch = |position: BlockPos| {
        BlockChanges::try_new(BlockChangesParts {
            dimension: Dimension::OVERWORLD,
            chunk: ChunkPos::new(0, 0),
            base_revision: 1,
            new_revision: 2,
            changes: vec![BlockChange::try_new(position, 2).expect("stone is registered")]
                .into_boxed_slice(),
        })
    };

    // The Y span is -64..=319 and the position has to name the announced
    // chunk, which the Go validator checks before it orders the batch.
    assert_eq!(
        batch(BlockPos::new(0, -65, 0)).unwrap_err(),
        DomainError::InvalidBlockY
    );
    assert_eq!(
        batch(BlockPos::new(0, 320, 0)).unwrap_err(),
        DomainError::InvalidBlockY
    );
    assert_eq!(
        batch(BlockPos::new(16, 64, 0)).unwrap_err(),
        DomainError::InvalidBlockChunk
    );
    assert_eq!(
        batch(BlockPos::new(0, 64, -1)).unwrap_err(),
        DomainError::InvalidBlockChunk
    );
    // Both span ends and the chunk's far corner are inside the announced
    // chunk, so the same batch is admitted.
    assert!(batch(BlockPos::new(0, -64, 0)).is_ok());
    assert!(batch(BlockPos::new(15, 319, 15)).is_ok());
}

#[test]
fn event_world_block_change_rejects_an_unregistered_block() {
    let admitted = BlockChange::try_new(BlockPos::new(0, 64, 0), 89)
        .expect("the last registered block is publishable");
    assert_eq!(admitted.position(), BlockPos::new(0, 64, 0));
    assert_eq!(admitted.block(), 89);

    assert_eq!(
        BlockChange::try_new(BlockPos::new(0, 64, 0), 90),
        Err(DomainError::InvalidBlock),
        "a change naming the sentinel must fail the conversion"
    );
}

#[test]
fn event_world_forget_chunks_preserves_reversed_unique_order() {
    // The protocol does not demand a sorted batch, so the record keeps the
    // order it was given: a replay observes the same sequence, and the
    // authority's own grouping and sorting stay a publication concern.
    let forget = ForgetChunks::try_new(ForgetChunksParts {
        dimension: Dimension::DEPTHS,
        chunks: vec![
            ChunkPos::new(5, 5),
            ChunkPos::new(-1, -1),
            ChunkPos::new(0, 0),
        ]
        .into_boxed_slice(),
    })
    .expect("three distinct chunks are a legal batch");

    assert_eq!(forget.dimension(), Dimension::DEPTHS);
    assert_eq!(
        forget.chunks().to_vec(),
        vec![
            ChunkPos::new(5, 5),
            ChunkPos::new(-1, -1),
            ChunkPos::new(0, 0),
        ]
    );
}

#[test]
fn event_world_forget_chunks_reject_an_empty_batch_and_a_duplicate() {
    assert_eq!(
        ForgetChunks::try_new(ForgetChunksParts {
            dimension: Dimension::OVERWORLD,
            chunks: Vec::new().into_boxed_slice(),
        }),
        Err(DomainError::InvalidForgetChunks),
        "an empty batch retires nothing"
    );
    assert_eq!(
        ForgetChunks::try_new(ForgetChunksParts {
            dimension: Dimension::OVERWORLD,
            chunks: vec![ChunkPos::new(0, 0), ChunkPos::new(0, 0)].into_boxed_slice(),
        }),
        Err(DomainError::InvalidForgetChunks),
        "a duplicated chunk is named twice"
    );
}
