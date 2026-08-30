use super::world::{RenderWorld, RenderWorldError, SectionData, SectionKey};
use std::sync::Arc;

const MAX_BATCH_BYTES: usize = 4 * 1024 * 1024;
const KEY: SectionKey = SectionKey::new(0, 1, 2, 3);

#[derive(Clone)]
struct Record {
    tag: u8,
    storage_kind: u8,
    bits: u8,
    reserved: u8,
    dimension: i32,
    x: i32,
    y: i32,
    z: i32,
    revision: u64,
    payload: Vec<u8>,
}

impl Record {
    fn section(revision: u64, storage_kind: u8, bits: u8, payload: Vec<u8>) -> Self {
        Self {
            tag: 1,
            storage_kind,
            bits,
            reserved: 0,
            dimension: KEY.dimension(),
            x: KEY.x(),
            y: KEY.y(),
            z: KEY.z(),
            revision,
            payload,
        }
    }

    fn column(revision: u64, payload: Vec<u8>) -> Self {
        Self {
            tag: 2,
            storage_kind: 0,
            bits: 0,
            reserved: 0,
            dimension: KEY.dimension(),
            x: KEY.x(),
            y: 0,
            z: KEY.z(),
            revision,
            payload,
        }
    }

    fn section_tombstone(revision: u64) -> Self {
        let mut record = Self::section(revision, 0, 0, Vec::new());
        record.tag = 3;
        record
    }

    fn column_tombstone(revision: u64) -> Self {
        let mut record = Self::column(revision, Vec::new());
        record.tag = 4;
        record
    }

    fn reset() -> Self {
        Self {
            tag: 5,
            storage_kind: 0,
            bits: 0,
            reserved: 0,
            dimension: 0,
            x: 0,
            y: 0,
            z: 0,
            revision: 0,
            payload: Vec::new(),
        }
    }
}

fn push_record(bytes: &mut Vec<u8>, record: &Record) {
    bytes.push(record.tag);
    bytes.push(record.storage_kind);
    bytes.push(record.bits);
    bytes.push(record.reserved);
    bytes.extend_from_slice(&record.dimension.to_le_bytes());
    bytes.extend_from_slice(&record.x.to_le_bytes());
    bytes.extend_from_slice(&record.y.to_le_bytes());
    bytes.extend_from_slice(&record.z.to_le_bytes());
    bytes.extend_from_slice(&record.revision.to_le_bytes());
    bytes.extend_from_slice(&(record.payload.len() as u32).to_le_bytes());
    bytes.extend_from_slice(&record.payload);
}

fn batch(epoch: u64, records: &[Record]) -> Vec<u8> {
    let mut bytes = Vec::new();
    bytes.extend_from_slice(b"MRW1");
    bytes.extend_from_slice(&1u16.to_le_bytes());
    bytes.extend_from_slice(&0u16.to_le_bytes());
    bytes.extend_from_slice(&epoch.to_le_bytes());
    bytes.extend_from_slice(&(records.len() as u32).to_le_bytes());
    bytes.extend_from_slice(&0u32.to_le_bytes());
    for record in records {
        push_record(&mut bytes, record);
    }
    bytes
}

fn section_payload(single: u16, palette: &[u16], packed_words: &[u64], reserved: u16) -> Vec<u8> {
    let mut payload = Vec::new();
    payload.extend_from_slice(&single.to_le_bytes());
    payload.extend_from_slice(&(palette.len() as u16).to_le_bytes());
    payload.extend_from_slice(&(packed_words.len() as u16).to_le_bytes());
    payload.extend_from_slice(&reserved.to_le_bytes());
    for value in palette {
        payload.extend_from_slice(&value.to_le_bytes());
    }
    for word in packed_words {
        payload.extend_from_slice(&word.to_le_bytes());
    }
    payload
}

fn single(block: u16) -> Record {
    single_at_revision(7, block)
}

fn single_at_revision(revision: u64, block: u16) -> Record {
    Record::section(revision, 0, 0, section_payload(block, &[], &[], 0))
}

fn constant_column(revision: u64, height: i16) -> Record {
    let mut payload = Vec::with_capacity(512);
    for _ in 0..256 {
        payload.extend_from_slice(&height.to_le_bytes());
    }
    Record::column(revision, payload)
}

fn indexed(revision: u64, bits: u8, palette: &[u16], slot: u8) -> Record {
    let words = match bits {
        4 => vec![u64::from(slot) * 0x1111_1111_1111_1111; 256],
        8 => vec![u64::from(slot) * 0x0101_0101_0101_0101; 512],
        _ => Vec::new(),
    };
    Record::section(revision, 1, bits, section_payload(0, palette, &words, 0))
}

fn direct(revision: u64, value: u16) -> Record {
    let packed = u64::from(value)
        | (u64::from(value) << 15)
        | (u64::from(value) << 30)
        | (u64::from(value) << 45);
    Record::section(
        revision,
        2,
        15,
        section_payload(0, &[], &vec![packed; 1024], 0),
    )
}

fn reset_then(epoch: u64, records: &[Record]) -> Vec<u8> {
    let mut all = Vec::with_capacity(records.len() + 1);
    all.push(Record::reset());
    all.extend_from_slice(records);
    batch(epoch, &all)
}

fn seeded_world() -> RenderWorld {
    let mut world = RenderWorld::default();
    world
        .apply_update_batch(&reset_then(1, &[single(3)]))
        .unwrap();
    world
}

fn world_from_reset(records: &[Record]) -> RenderWorld {
    let mut world = RenderWorld::default();
    world.apply_update_batch(&reset_then(1, records)).unwrap();
    world
}

fn assert_invalid_unchanged(world: &mut RenderWorld, bytes: &[u8]) {
    let before = world.snapshot_for_test();
    assert_eq!(
        world.apply_update_batch(bytes),
        Err(RenderWorldError::Invalid)
    );
    assert_eq!(world.snapshot_for_test(), before);
}

fn assert_invalid_without_panic_unchanged(world: &mut RenderWorld, bytes: &[u8]) {
    let before = world.snapshot_for_test();
    let result = std::panic::catch_unwind(std::panic::AssertUnwindSafe(|| {
        world.apply_update_batch(bytes)
    }));
    match result {
        Ok(result) => assert_eq!(result, Err(RenderWorldError::Invalid)),
        Err(_) => panic!("invalid MRW1 mutation panicked"),
    }
    assert_eq!(world.snapshot_for_test(), before);
}

#[test]
fn decodes_single_section_compactly() {
    let world = seeded_world();
    assert_eq!(world.section_for_test(KEY), Some(&SectionData::Single(3)));
}

#[test]
fn decodes_four_bit_indexed_section_compactly() {
    let record = indexed(1, 4, &[11, 22], 1);
    let mut world = RenderWorld::default();
    world.apply_update_batch(&reset_then(1, &[record])).unwrap();

    let SectionData::Indexed {
        bits,
        palette,
        packed_words,
    } = world.section_for_test(KEY).unwrap()
    else {
        panic!("expected indexed section");
    };
    assert_eq!(*bits, 4);
    assert_eq!(palette.as_ref(), &[11, 22]);
    assert_eq!(packed_words.len(), 256);
    assert!(
        packed_words
            .iter()
            .all(|word| *word == 0x1111_1111_1111_1111)
    );
}

#[test]
fn decodes_eight_bit_indexed_section_compactly() {
    let record = indexed(1, 8, &[11, 22, 33], 2);
    let mut world = RenderWorld::default();
    world.apply_update_batch(&reset_then(1, &[record])).unwrap();

    let SectionData::Indexed {
        bits,
        palette,
        packed_words,
    } = world.section_for_test(KEY).unwrap()
    else {
        panic!("expected indexed section");
    };
    assert_eq!(*bits, 8);
    assert_eq!(palette.as_ref(), &[11, 22, 33]);
    assert_eq!(packed_words.len(), 512);
    assert!(
        packed_words
            .iter()
            .all(|word| *word == 0x0202_0202_0202_0202)
    );
}

#[test]
fn decodes_direct_section_compactly() {
    let record = direct(1, 0x1234);
    let expected_word = 0x1234u64 | (0x1234u64 << 15) | (0x1234u64 << 30) | (0x1234u64 << 45);
    let mut world = RenderWorld::default();
    world.apply_update_batch(&reset_then(1, &[record])).unwrap();

    let SectionData::Direct { packed_words } = world.section_for_test(KEY).unwrap() else {
        panic!("expected direct section");
    };
    assert_eq!(packed_words.len(), 1024);
    assert!(packed_words.iter().all(|word| *word == expected_word));
}

#[test]
fn invalid_second_record_keeps_the_first_record_unapplied() {
    let mut world = seeded_world();
    let before = world.snapshot_for_test();

    let mut first = single(4);
    first.revision = 8;
    let invalid = indexed(9, 4, &[1], 1);
    let bytes = batch(1, &[first, invalid]);
    assert_eq!(
        world.apply_update_batch(&bytes),
        Err(RenderWorldError::Invalid)
    );
    assert_eq!(world.snapshot_for_test(), before);
}

#[test]
fn older_tombstoned_update_cannot_revive_a_section() {
    let mut world = seeded_world();
    world
        .apply_update_batch(&batch(1, &[Record::section_tombstone(11)]))
        .unwrap();
    let mut stale = single(2);
    stale.revision = 10;
    world.apply_update_batch(&batch(1, &[stale])).unwrap();
    let mut equal = single(4);
    equal.revision = 11;
    world.apply_update_batch(&batch(1, &[equal])).unwrap();
    assert!(world.section_for_test(KEY).is_none());
}

#[test]
fn newer_update_revives_a_tombstoned_section() {
    let mut world = seeded_world();
    world
        .apply_update_batch(&batch(1, &[Record::section_tombstone(11)]))
        .unwrap();
    let mut fresh = single(9);
    fresh.revision = 12;
    world.apply_update_batch(&batch(1, &[fresh])).unwrap();
    assert_eq!(world.section_for_test(KEY), Some(&SectionData::Single(9)));
}

#[test]
fn first_batch_requires_epoch_one_reset() {
    let mut world = RenderWorld::default();
    assert_invalid_unchanged(&mut world, &reset_then(2, &[]));
    assert_invalid_unchanged(&mut world, &batch(1, &[single(3)]));
}

#[test]
fn reset_epoch_must_increase() {
    let mut world = seeded_world();
    assert_invalid_unchanged(&mut world, &reset_then(1, &[]));
}

#[test]
fn reset_must_be_the_first_record() {
    let mut world = seeded_world();
    assert_invalid_unchanged(&mut world, &batch(2, &[single(4), Record::reset()]));
}

#[test]
fn reset_clears_old_entries_and_applies_remaining_records() {
    let mut world = seeded_world();
    world
        .apply_update_batch(&batch(1, &[Record::column_tombstone(4)]))
        .unwrap();
    let mut old_tombstone = Record::section_tombstone(1);
    old_tombstone.x = 7;
    world
        .apply_update_batch(&batch(1, &[old_tombstone]))
        .unwrap();
    let mut replacement = single(8);
    replacement.revision = 1;
    replacement.x = 99;
    let mut revived_section = single(6);
    revived_section.revision = 1;
    revived_section.x = 7;
    let revived_column = Record::column(1, vec![0; 512]);
    world
        .apply_update_batch(&reset_then(
            2,
            &[replacement.clone(), revived_section, revived_column],
        ))
        .unwrap();

    assert_eq!(world.epoch_for_test(), Some(2));
    assert!(world.section_for_test(KEY).is_none());
    assert_eq!(
        world.section_for_test(SectionKey::new(0, 7, 2, 3)),
        Some(&SectionData::Single(6))
    );
    assert_eq!(world.column_for_test(0, 1, 3), Some(&[0; 256]));
    assert_eq!(
        world.section_for_test(SectionKey::new(0, 99, 2, 3)),
        Some(&SectionData::Single(8))
    );
}

#[test]
fn invalid_record_after_reset_keeps_the_previous_epoch_and_maps() {
    let mut world = seeded_world();
    let invalid = indexed(1, 4, &[1], 1);
    assert_invalid_unchanged(&mut world, &reset_then(2, &[invalid]));
}

#[test]
fn non_reset_batch_epoch_must_match() {
    let mut world = seeded_world();
    assert_invalid_unchanged(&mut world, &batch(2, &[single(4)]));
}

#[test]
fn equal_revision_is_idempotent_and_greater_revision_replaces() {
    let mut world = seeded_world();
    let mut equal = single(4);
    equal.revision = 7;
    world.apply_update_batch(&batch(1, &[equal])).unwrap();
    assert_eq!(world.section_for_test(KEY), Some(&SectionData::Single(3)));

    let mut newer = single(5);
    newer.revision = 8;
    world.apply_update_batch(&batch(1, &[newer])).unwrap();
    assert_eq!(world.section_for_test(KEY), Some(&SectionData::Single(5)));
}

#[test]
fn same_batch_section_duplicates_follow_sequential_revision_semantics() {
    let world = world_from_reset(&[single_at_revision(1, 1), single_at_revision(2, 2)]);
    assert_eq!(world.section_for_test(KEY), Some(&SectionData::Single(2)));

    let world = world_from_reset(&[single_at_revision(2, 2), single_at_revision(1, 1)]);
    assert_eq!(world.section_for_test(KEY), Some(&SectionData::Single(2)));

    let world = world_from_reset(&[single_at_revision(1, 1), single_at_revision(1, 2)]);
    assert_eq!(world.section_for_test(KEY), Some(&SectionData::Single(1)));

    let world = world_from_reset(&[single_at_revision(1, 1), Record::section_tombstone(2)]);
    assert!(world.section_for_test(KEY).is_none());

    let world = world_from_reset(&[Record::section_tombstone(1), single_at_revision(2, 2)]);
    assert_eq!(world.section_for_test(KEY), Some(&SectionData::Single(2)));
}

#[test]
fn same_batch_column_duplicates_follow_sequential_revision_semantics() {
    let world = world_from_reset(&[constant_column(1, 1), constant_column(2, 2)]);
    assert_eq!(world.column_for_test(0, 1, 3), Some(&[2; 256]));

    let world = world_from_reset(&[constant_column(2, 2), constant_column(1, 1)]);
    assert_eq!(world.column_for_test(0, 1, 3), Some(&[2; 256]));

    let world = world_from_reset(&[constant_column(1, 1), constant_column(1, 2)]);
    assert_eq!(world.column_for_test(0, 1, 3), Some(&[1; 256]));

    let world = world_from_reset(&[constant_column(1, 1), Record::column_tombstone(2)]);
    assert!(world.column_for_test(0, 1, 3).is_none());

    let world = world_from_reset(&[Record::column_tombstone(1), constant_column(2, 2)]);
    assert_eq!(world.column_for_test(0, 1, 3), Some(&[2; 256]));
}

#[test]
fn column_round_trips_and_tombstone_revision_is_retained() {
    let heights: Vec<i16> = (0..256).map(|index| index as i16 - 128).collect();
    let payload: Vec<u8> = heights
        .iter()
        .flat_map(|height| height.to_le_bytes())
        .collect();
    let mut world = RenderWorld::default();
    world
        .apply_update_batch(&reset_then(1, &[Record::column(7, payload.clone())]))
        .unwrap();
    assert_eq!(world.column_for_test(0, 1, 3).unwrap(), heights.as_slice());

    world
        .apply_update_batch(&batch(1, &[Record::column_tombstone(9)]))
        .unwrap();
    world
        .apply_update_batch(&batch(1, &[Record::column(8, payload)]))
        .unwrap();
    assert!(world.column_for_test(0, 1, 3).is_none());

    world
        .apply_update_batch(&batch(1, &[Record::column(10, vec![0; 512])]))
        .unwrap();
    assert_eq!(world.column_for_test(0, 1, 3), Some(&[0; 256]));
}

#[test]
fn staged_snapshot_shares_immutable_column_payload() {
    let world = world_from_reset(&[constant_column(1, 42)]);
    let live = world.column_payload_arc_for_test(0, 1, 3).unwrap();
    assert_eq!(Arc::strong_count(live), 1);

    let staged = world.snapshot_for_test();
    let staged_payload = staged.column_payload_arc_for_test(0, 1, 3).unwrap();
    assert!(Arc::ptr_eq(live, staged_payload));
    assert_eq!(Arc::strong_count(live), 2);

    drop(staged);
    assert_eq!(Arc::strong_count(live), 1);
}

#[test]
fn staged_snapshot_shares_and_isolates_immutable_section_payloads() {
    let mut initial_direct = direct(1, 0x1234);
    initial_direct.x = 2;
    let direct_key = SectionKey::new(0, 2, 2, 3);
    let world = world_from_reset(&[indexed(1, 4, &[11, 22], 1), initial_direct]);
    let (live_palette, live_indexed_words) = world.indexed_payload_arcs_for_test(KEY).unwrap();
    let live_direct_words = world.direct_payload_arc_for_test(direct_key).unwrap();
    assert_eq!(Arc::strong_count(live_palette), 1);
    assert_eq!(Arc::strong_count(live_indexed_words), 1);
    assert_eq!(Arc::strong_count(live_direct_words), 1);

    let mut staged = world.snapshot_for_test();
    {
        let (staged_palette, staged_indexed_words) =
            staged.indexed_payload_arcs_for_test(KEY).unwrap();
        let staged_direct_words = staged.direct_payload_arc_for_test(direct_key).unwrap();
        assert!(Arc::ptr_eq(live_palette, staged_palette));
        assert!(Arc::ptr_eq(live_indexed_words, staged_indexed_words));
        assert!(Arc::ptr_eq(live_direct_words, staged_direct_words));
    }
    assert_eq!(Arc::strong_count(live_palette), 2);
    assert_eq!(Arc::strong_count(live_indexed_words), 2);
    assert_eq!(Arc::strong_count(live_direct_words), 2);

    let mut replacement_direct = direct(2, 0x4321);
    replacement_direct.x = 2;
    staged
        .apply_update_batch(&batch(
            1,
            &[indexed(2, 4, &[33, 44], 0), replacement_direct],
        ))
        .unwrap();

    assert_eq!(Arc::strong_count(live_palette), 1);
    assert_eq!(Arc::strong_count(live_indexed_words), 1);
    assert_eq!(Arc::strong_count(live_direct_words), 1);
    assert_eq!(live_palette.as_ref(), &[11, 22]);
    assert!(
        live_indexed_words
            .iter()
            .all(|word| *word == 0x1111_1111_1111_1111)
    );
    let original_direct_word =
        0x1234u64 | (0x1234u64 << 15) | (0x1234u64 << 30) | (0x1234u64 << 45);
    assert!(
        live_direct_words
            .iter()
            .all(|word| *word == original_direct_word)
    );

    let (replacement_palette, replacement_indexed_words) =
        staged.indexed_payload_arcs_for_test(KEY).unwrap();
    let replacement_direct_words = staged.direct_payload_arc_for_test(direct_key).unwrap();
    assert!(!Arc::ptr_eq(live_palette, replacement_palette));
    assert!(!Arc::ptr_eq(live_indexed_words, replacement_indexed_words));
    assert!(!Arc::ptr_eq(live_direct_words, replacement_direct_words));
}

#[test]
fn rejects_wrong_column_payload_length_and_metadata() {
    let mut world = seeded_world();
    let mut storage = Record::column(1, vec![0; 512]);
    storage.storage_kind = 1;
    let mut bits = Record::column(1, vec![0; 512]);
    bits.bits = 1;
    let mut y = Record::column(1, vec![0; 512]);
    y.y = 1;
    for record in [
        Record::column(1, vec![0; 510]),
        Record::column(1, vec![0; 514]),
        storage,
        bits,
        y,
    ] {
        assert_invalid_unchanged(&mut world, &batch(1, &[record]));
    }
}

#[test]
fn rejects_direct_words_with_high_four_bits_set() {
    let mut record = direct(8, 1);
    let word_offset = 8;
    record.payload[word_offset..word_offset + 8]
        .copy_from_slice(&0xf000_0000_0000_0000u64.to_le_bytes());
    let mut world = seeded_world();
    assert_invalid_unchanged(&mut world, &batch(1, &[record]));
}

#[test]
fn rejects_reserved_fields() {
    let mut world = seeded_world();

    let mut batch_reserved_16 = batch(1, &[single(8)]);
    batch_reserved_16[6] = 1;
    assert_invalid_unchanged(&mut world, &batch_reserved_16);

    let mut batch_reserved_32 = batch(1, &[single(8)]);
    batch_reserved_32[20] = 1;
    assert_invalid_unchanged(&mut world, &batch_reserved_32);

    let mut record_reserved = single(8);
    record_reserved.reserved = 1;
    assert_invalid_unchanged(&mut world, &batch(1, &[record_reserved]));

    let mut meta_reserved = single(8);
    meta_reserved.payload[6] = 1;
    assert_invalid_unchanged(&mut world, &batch(1, &[meta_reserved]));
}

#[test]
fn rejects_record_count_and_batch_size_limits() {
    let zero_records = batch(1, &[]);
    let mut world = seeded_world();
    assert_invalid_unchanged(&mut world, &zero_records);

    let mut direct_records = Vec::with_capacity(510);
    for x in 0..510 {
        let mut record = direct(8, 1);
        record.x = x;
        direct_records.push(record);
    }
    let oversized = batch(1, &direct_records);
    assert!(oversized.len() > MAX_BATCH_BYTES);
    assert!(direct_records.len() <= 4096);
    assert_invalid_unchanged(&mut world, &oversized);

    let mut tombstones = Vec::with_capacity(4097);
    for x in 0..4097 {
        let mut record = Record::section_tombstone(8);
        record.x = x;
        tombstones.push(record);
    }
    let too_many_records = batch(1, &tombstones);
    assert_eq!(tombstones.len(), 4097);
    assert!(too_many_records.len() <= MAX_BATCH_BYTES);
    assert_invalid_unchanged(&mut world, &too_many_records);
}

#[test]
fn accepts_the_record_count_limit() {
    let mut records = Vec::with_capacity(4096);
    records.push(Record::reset());
    for x in 0..4095 {
        let mut record = Record::section_tombstone(1);
        record.x = x;
        records.push(record);
    }
    let mut world = RenderWorld::default();
    world.apply_update_batch(&batch(1, &records)).unwrap();
}

#[test]
fn rejects_bad_header_tags_and_trailing_bytes() {
    let mut world = seeded_world();
    for bytes in [
        Vec::new(),
        b"MRW1".to_vec(),
        batch(0, &[single(8)]),
        {
            let mut bytes = batch(1, &[single(8)]);
            bytes[0..4].copy_from_slice(b"MRC1");
            bytes
        },
        {
            let mut bytes = batch(1, &[single(8)]);
            bytes[4..6].copy_from_slice(&2u16.to_le_bytes());
            bytes
        },
        {
            let mut record = single(8);
            record.tag = 6;
            batch(1, &[record])
        },
        {
            let mut bytes = batch(1, &[single(8)]);
            bytes.push(0);
            bytes
        },
    ] {
        assert_invalid_unchanged(&mut world, &bytes);
    }
}

#[test]
fn rejects_malformed_payload_lengths_without_panicking() {
    let mut world = seeded_world();
    let mut truncated = batch(1, &[single(8)]);
    truncated.pop();
    assert_invalid_unchanged(&mut world, &truncated);

    let mut overflowing = batch(1, &[single(8)]);
    overflowing[52..56].copy_from_slice(&u32::MAX.to_le_bytes());
    assert_invalid_unchanged(&mut world, &overflowing);
}

#[test]
fn mutation_properties_reject_truncation_and_declared_lengths_atomically() {
    let record = indexed(8, 4, &[1, 2], 1);
    let payload_len = record.payload.len() as u32;
    let valid = batch(1, &[record]);
    let mut world = seeded_world();

    for length in 0..valid.len() {
        assert_invalid_without_panic_unchanged(&mut world, &valid[..length]);
    }

    for declared_length in [0, 1, payload_len - 1, payload_len + 1, u32::MAX] {
        let mut mutated = valid.clone();
        mutated[52..56].copy_from_slice(&declared_length.to_le_bytes());
        assert_invalid_without_panic_unchanged(&mut world, &mutated);
    }
}

#[test]
fn mutation_properties_reject_packed_words_and_palette_slots_atomically() {
    let indexed = batch(1, &[indexed(8, 4, &[1, 2], 1)]);
    let mut world = seeded_world();

    for packed_word_count in [0, 1, 255, 257, 512, u16::MAX] {
        let mut mutated = indexed.clone();
        mutated[60..62].copy_from_slice(&packed_word_count.to_le_bytes());
        assert_invalid_without_panic_unchanged(&mut world, &mutated);
    }

    let packed_words_offset = 68;
    for word_index in 0..256 {
        let mut mutated = indexed.clone();
        let slot_offset = packed_words_offset + word_index * 8;
        mutated[slot_offset] = (mutated[slot_offset] & 0xf0) | 2;
        assert_invalid_without_panic_unchanged(&mut world, &mutated);
    }

    let direct = batch(1, &[direct(8, 1)]);
    let direct_words_offset = 64;
    for word_index in 0..1024 {
        let mut mutated = direct.clone();
        let high_byte_offset = direct_words_offset + word_index * 8 + 7;
        mutated[high_byte_offset] |= 0xf0;
        assert_invalid_without_panic_unchanged(&mut world, &mutated);
    }
}

#[test]
fn rejects_invalid_indexed_word_counts_and_payload_trailing_bytes() {
    let mut world = seeded_world();
    for (bits, count) in [(4, 255), (4, 257), (8, 511), (8, 513)] {
        let words = vec![0; count];
        let record = Record::section(8, 1, bits, section_payload(0, &[1], &words, 0));
        assert_invalid_unchanged(&mut world, &batch(1, &[record]));
    }

    let mut trailing = indexed(8, 4, &[1], 0);
    trailing.payload.push(0);
    assert_invalid_unchanged(&mut world, &batch(1, &[trailing]));
}

#[test]
fn rejects_out_of_range_palette_slots_and_palette_sizes() {
    let mut world = seeded_world();
    assert_invalid_unchanged(&mut world, &batch(1, &[indexed(8, 4, &[1], 1)]));
    assert_invalid_unchanged(&mut world, &batch(1, &[indexed(8, 8, &[1], 1)]));

    let empty = indexed(8, 4, &[], 0);
    assert_invalid_unchanged(&mut world, &batch(1, &[empty]));
    let too_large = indexed(8, 4, &[1; 17], 0);
    assert_invalid_unchanged(&mut world, &batch(1, &[too_large]));
}

#[test]
fn rejects_invalid_section_storage_and_y_but_accepts_signed_key_extremes() {
    let mut world = seeded_world();
    let mut single_bits = single(8);
    single_bits.bits = 1;
    let single_palette = Record::section(8, 0, 0, section_payload(8, &[1], &[], 0));
    let indexed_bits = Record::section(8, 1, 5, section_payload(0, &[1], &[], 0));
    let mut direct_single = direct(8, 1);
    direct_single.payload[0..2].copy_from_slice(&1u16.to_le_bytes());
    let direct_short = Record::section(8, 2, 15, section_payload(0, &[], &vec![0; 1023], 0));
    let direct_palette = Record::section(8, 2, 15, section_payload(0, &[1], &vec![0; 1024], 0));
    let unknown = Record::section(8, 3, 0, section_payload(0, &[], &[], 0));
    for record in [
        single_bits,
        single_palette,
        indexed_bits,
        direct_single,
        direct_short,
        direct_palette,
        unknown,
    ] {
        assert_invalid_unchanged(&mut world, &batch(1, &[record]));
    }

    let mut y_low = single(8);
    y_low.y = -1;
    assert_invalid_unchanged(&mut world, &batch(1, &[y_low]));
    let mut y_high = single(8);
    y_high.y = 24;
    assert_invalid_unchanged(&mut world, &batch(1, &[y_high]));

    let mut extreme = single(8);
    extreme.dimension = i32::MIN;
    extreme.x = i32::MAX;
    extreme.z = i32::MIN;
    extreme.y = 23;
    world.apply_update_batch(&batch(1, &[extreme])).unwrap();
    assert_eq!(
        world.section_for_test(SectionKey::new(i32::MIN, i32::MAX, 23, i32::MIN)),
        Some(&SectionData::Single(8))
    );
}

#[test]
fn rejects_nonempty_tombstones_and_invalid_reset_metadata() {
    let mut world = seeded_world();
    let mut tombstone = Record::section_tombstone(8);
    tombstone.payload.push(0);
    assert_invalid_unchanged(&mut world, &batch(1, &[tombstone]));

    let mut reset_dimension = Record::reset();
    reset_dimension.dimension = 1;
    let mut reset_x = Record::reset();
    reset_x.x = 1;
    let mut reset_y = Record::reset();
    reset_y.y = 1;
    let mut reset_z = Record::reset();
    reset_z.z = 1;
    for reset in [reset_dimension, reset_x, reset_y, reset_z] {
        assert_invalid_unchanged(&mut world, &batch(2, &[reset]));
    }

    let mut reset_revision = Record::reset();
    reset_revision.revision = 1;
    assert_invalid_unchanged(&mut world, &batch(2, &[reset_revision]));
    let mut reset_payload = Record::reset();
    reset_payload.payload.push(0);
    assert_invalid_unchanged(&mut world, &batch(2, &[reset_payload]));
    let mut reset_storage = Record::reset();
    reset_storage.storage_kind = 1;
    assert_invalid_unchanged(&mut world, &batch(2, &[reset_storage]));
}
