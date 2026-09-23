//! The two item-drop publication families: the common fallible surface, the
//! raw-dimension identity order, and the minimum-records batch rule.
//!
//! `ItemDropUpserts` (S/Play/11) and `ItemDropRemoves` (S/Play/12) are the two
//! halves of the drop publication: the first adds or replaces whole drop
//! stacks, the second removes drop identities. Both carry a `u64` server tick
//! and a canonical uvarint count, and both order their records by the domain
//! `DropId` total order — dimension, chunk column, slot, generation — where
//! the dimension is the RAW wire `i32` and is never narrowed or validated,
//! because the Go `core.DropID.Valid` rule checks only the slot range and the
//! generation. A drop naming dimension −1 or 256 is a publishable identity on
//! both sides, and a −1-dimension record sorts before a 0-dimension one.
//!
//! Both records are concrete packets with public mutable fields, so the group
//! pins the design's surface order (`validate` → checked `encoded_len` →
//! capacity check → `publish_packet`): a short or invalid `encode_into` leaves
//! the caller's buffer untouched, an invalid value wins over a short
//! destination, and every proper truncation of a canonical payload rejects.
//! The wire literals are the Go encoder's output, so the round-trip tests pin
//! byte-level parity rather than self-agreement.
//!
//! The value gates keep the Go validator orders. The upsert gate applies the
//! batch count bound and then, per record, the block-index bound and the
//! shared item-stack rule with the strict identity order checked against the
//! previous record; the remove gate applies the count bound and then the
//! identity order. The exact empty stack triple `(0,0,0)` is wire-valid and is
//! never rejected.
//!
//! The batch length rule is the minimum-records rule on both sides: the Go
//! decoder rejects a payload shorter than `count` records before it allocates
//! and applies its end-of-payload check afterwards, so a short payload is a
//! truncation and a padded one is a trailing byte. This differs from the
//! companion batch, whose Go decoder applies an exact remaining-length rule
//! and therefore answers a padded payload at the truncation boundary.
//!
//! One boundary is latent across the two implementations and therefore pinned
//! here rather than in the corpus: the Go validator folds an unregistered item
//! number into the same `network: invalid item drop stack` message as the
//! count and durability violations, while the Rust rule answers an
//! unregistered number with `InvalidEnum`. The corpus freezes the count and
//! stack-limit violations, where both sides publish the value boundary, and
//! this suite pins the unregistered number at the Rust variant beside the Go
//! message coarseness.
//!
//! The corpus evidence these families publish is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.

use mornlea_domain::ChunkPos;
use mornlea_protocol::{
    DROP_ID_WIRE_BYTES, DropId, ITEM_DROP_WIRE_BYTES, ItemDrop, ItemDropRemoves, ItemDropUpserts,
    MAX_CHUNK_BLOCK_INDEX, MAX_ITEM_DROP_BATCH, ProtocolError,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The reviewed 61-byte ItemDropUpserts literal: tick 0, two records strictly
/// ordered by the full identity key with the raw dimension first.
///
/// The first record names the raw dimension −1 with the highest slot and
/// generation the wire can carry, and the second names dimension 256 with the
/// lowest ones, so the order pin is observable in the dimension field alone.
/// The first record carries the ordinary stone stack and the second the exact
/// empty triple, with block indexes 0 and 98303 (the inclusive upper bound of
/// the chunk-ordered block index minus one).
const ITEM_DROP_UPSERTS_WIRE: [u8; 61] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xff, 0xff, 0xff, 0x07, 0x00, 0x00,
    0x00, 0xfd, 0xff, 0xff, 0xff, 0x1f, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
    0x04, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0xfc, 0xff, 0xff, 0xff, 0x09, 0x00, 0x00, 0x00, 0x00,
    0x01, 0x00, 0x00, 0x00, 0xff, 0x7f, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
];
/// The reviewed 26-byte ItemDropRemoves literal: tick 0 and one identity, the
/// same reviewed identity the Go golden test carries.
const ITEM_DROP_REMOVES_WIRE: [u8; 26] = [
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00,
    0x00, 0xfe, 0xff, 0xff, 0xff, 0x03, 0x07, 0x00, 0x00, 0x00,
];

/// The reviewed cross-dimension ordered remove pair: a −1-dimension identity
/// with the highest slot and generation before a 256-dimension identity with
/// the lowest ones. The Go decoder admits this pair because the ordering
/// compares the raw dimension first.
const ITEM_DROP_ORDERED_PAIR_WIRE: [u8; 43] = [
    // tick 0 and the declared count of two records
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
    // first identity: dimension -1, chunk (31, 31), slot 31, generation u32::MAX
    0xff, 0xff, 0xff, 0xff, 0x1f, 0x00, 0x00, 0x00, 0x1f, 0x00, 0x00, 0x00, 0x1f, 0xff, 0xff, 0xff,
    0xff, // second identity: dimension 256, chunk (0, 0), slot 0, generation 1
    0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00,
    0x00,
];

/// Builds one drop identity in the wire field order.
fn drop_id(dimension: i32, chunk_x: i32, chunk_z: i32, slot: u8, generation: u32) -> DropId {
    DropId::try_new(dimension, ChunkPos::new(chunk_x, chunk_z), slot, generation)
        .expect("reviewed drop identity")
}

/// One upsert record: an identity, a block index and the fixed stack triple.
fn drop_record(id: DropId, block_index: u32, item: u16, count: u8, durability: u16) -> ItemDrop {
    ItemDrop {
        id,
        block_index,
        item,
        count,
        durability,
    }
}

/// The canonical two-record upsert batch the reviewed literal carries.
fn canonical_upserts() -> ItemDropUpserts {
    ItemDropUpserts::new(
        0,
        vec![
            drop_record(drop_id(-1, 7, -3, 31, u32::MAX), 0, 1, 4, 0),
            drop_record(
                drop_id(256, -4, 9, 0, 1),
                MAX_CHUNK_BLOCK_INDEX - 1,
                0,
                0,
                0,
            ),
        ],
    )
    .expect("the canonical batch is valid")
}

/// The canonical one-record remove batch the reviewed literal carries.
fn canonical_removes() -> ItemDropRemoves {
    ItemDropRemoves::new(0, vec![drop_id(0, 1, -2, 3, 7)]).expect("the canonical batch is valid")
}

/// The full 32-record remove batch, the fixed record ceiling.
fn full_removes() -> ItemDropRemoves {
    let ids: Vec<DropId> = (0..MAX_ITEM_DROP_BATCH as i32)
        .map(|dimension| drop_id(dimension, 0, 0, 0, 1))
        .collect();
    ItemDropRemoves::new(0, ids).expect("the full batch is valid")
}

/// The full 32-record upsert batch, the fixed record ceiling.
fn full_upserts() -> ItemDropUpserts {
    let drops: Vec<ItemDrop> = (0..MAX_ITEM_DROP_BATCH as i32)
        .map(|dimension| drop_record(drop_id(dimension, 0, 0, 0, 1), 1, 1, 1, 0))
        .collect();
    ItemDropUpserts::new(0, drops).expect("the full batch is valid")
}

/// Asserts the shared surface contract for one canonical batch: the reviewed
/// length, the exact window, the padded prefix, the allocating wrapper and the
/// decode/re-encode round trip.
fn assert_round_trip(
    label: &str,
    payload: &[u8],
    validate: impl FnOnce() -> Result<(), ProtocolError>,
    encoded_len: impl FnOnce() -> Result<usize, ProtocolError>,
    encode_into: impl Fn(&mut [u8]) -> Result<usize, ProtocolError>,
    encode: impl FnOnce() -> Result<Vec<u8>, ProtocolError>,
    decode: impl Fn(&[u8]) -> Result<(), ProtocolError>,
    re_encode: impl FnOnce() -> Result<Vec<u8>, ProtocolError>,
    equals: impl FnOnce() -> bool,
) {
    validate().unwrap_or_else(|err| panic!("{label}: valid batch fails validation: {err:?}"));
    let length = encoded_len().unwrap_or_else(|err| panic!("{label}: encoded_len fails: {err:?}"));
    assert_eq!(
        length,
        payload.len(),
        "{label}: encoded_len disagrees with the reviewed payload"
    );

    let mut exact = vec![0u8; length];
    let written =
        encode_into(&mut exact).unwrap_or_else(|err| panic!("{label}: encode_into fails: {err:?}"));
    assert_eq!(
        written, length,
        "{label}: encode_into reports a short write"
    );
    assert_eq!(
        exact, payload,
        "{label}: encode_into published unexpected bytes"
    );

    let mut padded = vec![SENTINEL; length + 3];
    let written = encode_into(&mut padded)
        .unwrap_or_else(|err| panic!("{label}: padded encode_into fails: {err:?}"));
    assert_eq!(
        written, length,
        "{label}: padded write reports a short length"
    );
    assert_eq!(
        &padded[..length],
        payload,
        "{label}: padded prefix differs from the reviewed payload"
    );
    assert!(
        padded[length..].iter().all(|byte| *byte == SENTINEL),
        "{label}: padded write touched bytes beyond the record"
    );

    let allocated = encode().unwrap_or_else(|err| panic!("{label}: encode fails: {err:?}"));
    assert_eq!(
        allocated, payload,
        "{label}: encode disagrees with encode_into"
    );

    decode(payload).unwrap_or_else(|err| panic!("{label}: decode fails: {err:?}"));
    assert!(
        equals(),
        "{label}: decoded batch differs from the encoded one"
    );
    let reencoded = re_encode().unwrap_or_else(|err| panic!("{label}: re-encode fails: {err:?}"));
    assert_eq!(
        reencoded, payload,
        "{label}: re-encoded payload differs from the reviewed bytes"
    );
}

#[test]
fn item_drops_upserts_round_trips_through_the_fallible_surface() {
    let upserts = canonical_upserts();
    let decoded = ItemDropUpserts::decode(&ITEM_DROP_UPSERTS_WIRE).expect("decode");
    assert_round_trip(
        "item drop upserts",
        &ITEM_DROP_UPSERTS_WIRE,
        || upserts.validate(),
        || upserts.encoded_len(),
        |dst| upserts.encode_into(dst),
        || upserts.encode(),
        |payload| ItemDropUpserts::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == upserts,
    );
    assert_eq!(
        ITEM_DROP_UPSERTS_WIRE.len(),
        8 + 1 + 2 * ITEM_DROP_WIRE_BYTES
    );
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.drops.len(), 2);
    // The raw dimensions survive the round trip verbatim; neither the −1 nor
    // the 256 dimension is narrowed or rejected.
    assert_eq!(decoded.drops[0].id.dimension(), -1);
    assert_eq!(decoded.drops[1].id.dimension(), 256);
    assert_eq!(decoded.drops[0].id.slot(), 31);
    assert_eq!(decoded.drops[0].id.generation(), u32::MAX);
    assert_eq!(decoded.drops[1].id.slot(), 0);
    assert_eq!(decoded.drops[1].id.generation(), 1);
    // The block indexes carry the inclusive boundary and its floor.
    assert_eq!(decoded.drops[0].block_index, 0);
    assert_eq!(decoded.drops[1].block_index, MAX_CHUNK_BLOCK_INDEX - 1);
    // One ordinary stack and one exact empty triple, both wire-valid.
    assert_eq!(
        (
            decoded.drops[0].item,
            decoded.drops[0].count,
            decoded.drops[0].durability
        ),
        (1, 4, 0)
    );
    assert_eq!(
        (
            decoded.drops[1].item,
            decoded.drops[1].count,
            decoded.drops[1].durability
        ),
        (0, 0, 0)
    );
}

#[test]
fn item_drops_removes_round_trips_through_the_fallible_surface() {
    let removes = canonical_removes();
    let decoded = ItemDropRemoves::decode(&ITEM_DROP_REMOVES_WIRE).expect("decode");
    assert_round_trip(
        "item drop removes",
        &ITEM_DROP_REMOVES_WIRE,
        || removes.validate(),
        || removes.encoded_len(),
        |dst| removes.encode_into(dst),
        || removes.encode(),
        |payload| ItemDropRemoves::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == removes,
    );
    assert_eq!(ITEM_DROP_REMOVES_WIRE.len(), 8 + 1 + DROP_ID_WIRE_BYTES);
    assert_eq!(decoded.server_tick, 0);
    assert_eq!(decoded.ids.len(), 1);
    assert_eq!(decoded.ids[0].dimension(), 0);
    assert_eq!(decoded.ids[0].chunk().x(), 1);
    assert_eq!(decoded.ids[0].chunk().z(), -2);
    assert_eq!(decoded.ids[0].slot(), 3);
    assert_eq!(decoded.ids[0].generation(), 7);
}

#[test]
fn item_drops_identity_order_compares_the_raw_dimension_first() {
    // The domain order is (dimension, chunk x, chunk z, slot, generation) over
    // the raw wire values, so a −1-dimension identity sorts before a
    // 0-dimension one even when every other field descends.
    let negative = drop_id(-1, 31, 31, 31, u32::MAX);
    let zero = drop_id(0, 0, 0, 0, 1);
    let one = drop_id(1, 0, 0, 0, 1);
    let wide = drop_id(256, 0, 0, 0, 1);
    assert!(
        negative < zero,
        "the −1 dimension sorts before the 0 dimension"
    );
    assert!(
        zero < one && one < wide,
        "the raw dimension order is ascending"
    );

    // Both families admit the cross-dimension ordered pair and refuse its
    // reverse, and the reviewed Go-produced pair decodes through the same
    // order.
    let ordered = ItemDropRemoves::new(0, vec![negative, wide]).expect("an ordered pair is valid");
    assert_eq!(ordered.ids, vec![negative, wide]);
    assert_eq!(
        ItemDropRemoves::new(0, vec![wide, negative]),
        Err(ProtocolError::InvalidRange)
    );
    let decoded = ItemDropRemoves::decode(&ITEM_DROP_ORDERED_PAIR_WIRE).expect("decode");
    assert_eq!(decoded.ids, vec![negative, wide]);

    let ordered_upserts = ItemDropUpserts::new(
        0,
        vec![
            drop_record(negative, 0, 0, 0, 0),
            drop_record(wide, 1, 0, 0, 0),
        ],
    )
    .expect("an ordered pair is valid");
    assert_eq!(
        ItemDropUpserts::new(
            0,
            vec![
                drop_record(wide, 0, 0, 0, 0),
                drop_record(negative, 1, 0, 0, 0),
            ],
        ),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(ordered_upserts.drops.len(), 2);
}

#[test]
fn item_drops_block_index_boundary_is_exact() {
    // 98303 is the inclusive upper bound of the chunk-ordered block index and
    // 98304 is refused without clamping, on every entry point.
    let mut above = canonical_upserts();
    above.drops[1].block_index = MAX_CHUNK_BLOCK_INDEX;
    assert_eq!(
        ItemDropUpserts::new(0, above.drops.clone()),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(above.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(above.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(above.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(
        above.encode_into(&mut [0u8; ITEM_DROP_UPSERTS_WIRE.len()]),
        Err(ProtocolError::InvalidRange)
    );

    // The decode path answers at the same boundary: record two's block index
    // is the four bytes at offset 52.
    let mut payload = ITEM_DROP_UPSERTS_WIRE;
    payload[52..56].copy_from_slice(&MAX_CHUNK_BLOCK_INDEX.to_le_bytes());
    assert_eq!(
        ItemDropUpserts::decode(&payload),
        Err(ProtocolError::InvalidRange)
    );
    // The reviewed vector carries the inclusive boundary itself, which is one
    // below the exclusive upper bound.
    let boundary = MAX_CHUNK_BLOCK_INDEX - 1;
    assert_eq!(ITEM_DROP_UPSERTS_WIRE[52..56], boundary.to_le_bytes());
}

#[test]
fn item_drops_the_count_bound_fires_before_the_record_rule() {
    // A declared count outside 1..=32 is answered at the count bound even when
    // the payload cannot back it, which is the order the Go decoder applies:
    // the count message precedes its remaining-bytes check.
    for count in [0u8, 33] {
        let mut short = ITEM_DROP_UPSERTS_WIRE.to_vec();
        short[8] = count;
        short.truncate(20);
        assert_eq!(
            ItemDropUpserts::decode(&short),
            Err(ProtocolError::InvalidRange),
            "count {count} is refused at the count bound"
        );
        assert_eq!(
            ItemDropRemoves::decode(&short),
            Err(ProtocolError::InvalidRange),
            "count {count} is refused at the count bound"
        );
        let mut full = ITEM_DROP_UPSERTS_WIRE;
        full[8] = count;
        assert_eq!(
            ItemDropUpserts::decode(&full),
            Err(ProtocolError::InvalidRange)
        );
    }
    // A declared count the payload cannot back is the truncation boundary, on
    // both families.
    let mut mismatched = ITEM_DROP_REMOVES_WIRE;
    mismatched[8] = 2;
    assert_eq!(
        ItemDropRemoves::decode(&mismatched),
        Err(ProtocolError::Truncated)
    );
    let mut mismatched_upserts = ITEM_DROP_UPSERTS_WIRE;
    mismatched_upserts[8] = 3;
    assert_eq!(
        ItemDropUpserts::decode(&mismatched_upserts),
        Err(ProtocolError::Truncated)
    );
    // The constructors refuse the empty and the over-full batch.
    assert_eq!(
        ItemDropUpserts::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(
        ItemDropRemoves::new(0, Vec::new()),
        Err(ProtocolError::InvalidRange)
    );
    let over: Vec<DropId> = (0..=MAX_ITEM_DROP_BATCH as i32)
        .map(|dimension| drop_id(dimension, 0, 0, 0, 1))
        .collect();
    assert_eq!(
        ItemDropRemoves::new(0, over),
        Err(ProtocolError::InvalidRange)
    );
}

#[test]
fn item_drops_admit_the_full_batch_at_the_record_ceiling() {
    // Thirty-two records is the count and record ceiling: the batch admits,
    // its exact length is the tick, the one-byte count and the records, and
    // the 33rd record is refused by the count bound.
    let removes = full_removes();
    assert_eq!(removes.ids.len(), MAX_ITEM_DROP_BATCH as usize);
    assert_eq!(
        removes.encoded_len().expect("encoded_len"),
        8 + 1 + MAX_ITEM_DROP_BATCH as usize * DROP_ID_WIRE_BYTES
    );
    let wire = removes.encode().expect("encode the full batch");
    assert_eq!(wire.len(), 553);
    assert_eq!(
        ItemDropRemoves::decode(&wire).expect("decode the full batch"),
        removes
    );
    let mut exact = vec![0u8; 553];
    assert_eq!(removes.encode_into(&mut exact).expect("encode_into"), 553);
    assert_eq!(exact, wire);

    let upserts = full_upserts();
    assert_eq!(upserts.drops.len(), MAX_ITEM_DROP_BATCH as usize);
    assert_eq!(
        upserts.encoded_len().expect("encoded_len"),
        8 + 1 + MAX_ITEM_DROP_BATCH as usize * ITEM_DROP_WIRE_BYTES
    );
    let wire = upserts.encode().expect("encode the full upsert batch");
    assert_eq!(
        ItemDropUpserts::decode(&wire).expect("decode the full upsert batch"),
        upserts
    );
}

#[test]
fn item_drops_the_stack_rule_preserves_the_valid_empty_triple() {
    // The exact empty triple is wire-valid on both halves of the family, and a
    // non-canonical empty is refused: item 0 with a count or a durability is
    // not an empty stack.
    let empty = drop_record(drop_id(0, 0, 0, 0, 1), 0, 0, 0, 0);
    let upserts = ItemDropUpserts::new(0, vec![empty]).expect("the empty triple is valid");
    let decoded = ItemDropUpserts::decode(&upserts.encode().expect("encode")).expect("decode");
    assert_eq!(decoded, upserts);
    for bad in [
        drop_record(drop_id(0, 0, 0, 0, 1), 0, 0, 1, 0),
        drop_record(drop_id(0, 0, 0, 0, 1), 0, 0, 0, 1),
        drop_record(drop_id(0, 0, 0, 0, 1), 0, 0, 1, 1),
    ] {
        assert_eq!(
            ItemDropUpserts::new(0, vec![bad]),
            Err(ProtocolError::InvalidRange),
            "a non-canonical empty stack is refused"
        );
    }
    // A count above the item's stack limit and a durable item at durability
    // zero are refused at the shared stack rule.
    for bad in [
        drop_record(drop_id(0, 0, 0, 0, 1), 0, 1, 65, 0),
        drop_record(drop_id(0, 0, 0, 0, 1), 0, 10, 1, 0),
        drop_record(drop_id(0, 0, 0, 0, 1), 0, 10, 1, 200),
    ] {
        assert_eq!(
            ItemDropUpserts::new(0, vec![bad]),
            Err(ProtocolError::InvalidRange),
            "a stack the domain rule refuses is refused"
        );
    }
}

#[test]
fn item_drops_unregistered_item_number_is_pinned_here_not_in_the_corpus() {
    // The latent cross-implementation boundary class. The Go validator folds
    // an unregistered item number into `network: invalid item drop stack`,
    // which is the same message the count and durability boundaries answer
    // with, so a corpus case could not distinguish them; the Rust rule answers
    // an unregistered number with `InvalidEnum`. This suite pins the Rust
    // variant, and the corpus freezes the count and stack-limit violations
    // where both sides publish the value boundary.
    let mut payload = ITEM_DROP_UPSERTS_WIRE;
    payload[30] = 66;
    payload[31] = 0;
    payload[32] = 1;
    assert_eq!(
        ItemDropUpserts::decode(&payload),
        Err(ProtocolError::InvalidEnum),
        "an unregistered item number is the Rust enum boundary"
    );
    let mut unregistered = canonical_upserts();
    unregistered.drops[0].item = 66;
    assert_eq!(unregistered.encode(), Err(ProtocolError::InvalidEnum));
    assert_eq!(unregistered.validate(), Err(ProtocolError::InvalidEnum));
    assert_eq!(unregistered.encoded_len(), Err(ProtocolError::InvalidEnum));
    // The registered numbering stops at the exclusive upper bound 66, which is
    // why the number above the last registered item is the boundary.
    assert_eq!(mornlea_protocol::ITEM_ID_MAX, 66);
}

#[test]
fn item_drops_the_identity_rejection_is_the_identity_boundary() {
    // A slot past the fixed per-chunk array and a zero generation are refused
    // where the identity is read, at the identity boundary: the Go decoder
    // answers both with `network: invalid item drop ID` (and `network: item
    // drop remove %d: invalid ID`), which is the boundary the frozen corpus
    // records for drop-ID slot and generation errors.
    let mut slot_above = ITEM_DROP_UPSERTS_WIRE;
    slot_above[21] = 32;
    assert_eq!(
        ItemDropUpserts::decode(&slot_above),
        Err(ProtocolError::InvalidIdentity)
    );
    let mut zero_generation = ITEM_DROP_UPSERTS_WIRE;
    for byte in &mut zero_generation[22..26] {
        *byte = 0;
    }
    assert_eq!(
        ItemDropUpserts::decode(&zero_generation),
        Err(ProtocolError::InvalidIdentity)
    );

    let mut remove_slot_above = ITEM_DROP_REMOVES_WIRE;
    remove_slot_above[21] = 32;
    assert_eq!(
        ItemDropRemoves::decode(&remove_slot_above),
        Err(ProtocolError::InvalidIdentity)
    );
    let mut remove_zero_generation = ITEM_DROP_REMOVES_WIRE;
    for byte in &mut remove_zero_generation[22..26] {
        *byte = 0;
    }
    assert_eq!(
        ItemDropRemoves::decode(&remove_zero_generation),
        Err(ProtocolError::InvalidIdentity)
    );

    // The identity is a checked newtype, so neither violation is constructible
    // on the outbound surface.
    assert!(DropId::try_new(0, ChunkPos::new(0, 0), 32, 1).is_err());
    assert!(DropId::try_new(0, ChunkPos::new(0, 0), 0, 0).is_err());
    // The dimension stays unchecked beside those two rules.
    assert!(DropId::try_new(-1, ChunkPos::new(0, 0), 0, 1).is_ok());
    assert!(DropId::try_new(256, ChunkPos::new(0, 0), 0, 1).is_ok());
}

#[test]
fn item_drops_invalid_value_wins_over_short_capacity() {
    let mut above_block_index = canonical_upserts();
    above_block_index.drops[0].block_index = MAX_CHUNK_BLOCK_INDEX;
    let mut over_count = canonical_upserts();
    over_count.drops[0].count = 65;
    let mut non_canonical_empty = canonical_upserts();
    non_canonical_empty.drops[1] = drop_record(
        drop_id(256, -4, 9, 0, 1),
        MAX_CHUNK_BLOCK_INDEX - 1,
        0,
        1,
        0,
    );
    let mut duplicate = canonical_upserts();
    duplicate.drops[1] = duplicate.drops[0];
    let mut reversed = canonical_upserts();
    reversed.drops.reverse();
    let mut empty_upserts = canonical_upserts();
    empty_upserts.drops.clear();
    let mut empty_removes = canonical_removes();
    empty_removes.ids.clear();
    let mut duplicate_removes = canonical_removes();
    duplicate_removes.ids.push(duplicate_removes.ids[0]);
    let mut reversed_removes = canonical_removes();
    reversed_removes.ids.insert(0, drop_id(256, 0, 0, 0, 1));

    let cases: Vec<(
        &str,
        ProtocolError,
        usize,
        Vec<Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>>,
    )> = vec![
        (
            "upserts block index is outside the chunk",
            ProtocolError::InvalidRange,
            ITEM_DROP_UPSERTS_WIRE.len(),
            {
                let record = above_block_index.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "upserts count is above the stack limit",
            ProtocolError::InvalidRange,
            ITEM_DROP_UPSERTS_WIRE.len(),
            {
                let record = over_count.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "upserts carry a non-canonical empty stack",
            ProtocolError::InvalidRange,
            ITEM_DROP_UPSERTS_WIRE.len(),
            {
                let record = non_canonical_empty.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "upserts carry a duplicate identity",
            ProtocolError::InvalidRange,
            ITEM_DROP_UPSERTS_WIRE.len(),
            {
                let record = duplicate.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "upserts are descending",
            ProtocolError::InvalidRange,
            ITEM_DROP_UPSERTS_WIRE.len(),
            {
                let record = reversed.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "upserts batch is empty",
            ProtocolError::InvalidRange,
            ITEM_DROP_UPSERTS_WIRE.len(),
            {
                let record = empty_upserts.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "removes batch is empty",
            ProtocolError::InvalidRange,
            ITEM_DROP_REMOVES_WIRE.len(),
            {
                let record = empty_removes.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "removes carry a duplicate identity",
            ProtocolError::InvalidRange,
            8 + 1 + 2 * DROP_ID_WIRE_BYTES,
            {
                let record = duplicate_removes.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "removes are descending",
            ProtocolError::InvalidRange,
            8 + 1 + 2 * DROP_ID_WIRE_BYTES,
            {
                let record = reversed_removes.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
    ];

    for (label, error, needed, encoders) in cases {
        for encode_into in &encoders {
            // The value error is reported for every destination shape, so an
            // invalid batch never reaches the capacity decision.
            for available in [0usize, needed - 1, needed] {
                let mut dst = vec![SENTINEL; available];
                assert_eq!(
                    encode_into(&mut dst),
                    Err(error),
                    "{label}: a destination of {available} bytes masks the value error"
                );
                assert!(
                    dst.iter().all(|byte| *byte == SENTINEL),
                    "{label}: a destination of {available} bytes was modified on a value refusal"
                );
            }
            // The exact destination still refuses the value error.
            let mut exact = vec![0u8; needed];
            assert_eq!(
                encode_into(&mut exact),
                Err(error),
                "{label}: the exact destination still refuses the value error"
            );
            assert!(
                exact.iter().all(|byte| *byte == 0),
                "{label}: the exact destination was modified on a value refusal"
            );
        }
    }

    // Every allocating wrapper refuses the same mutations, so no entry point
    // publishes a mutated batch.
    assert_eq!(above_block_index.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(over_count.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(
        non_canonical_empty.encode(),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(duplicate.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(reversed.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(empty_upserts.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(empty_removes.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(duplicate_removes.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(reversed_removes.encode(), Err(ProtocolError::InvalidRange));
}

/// The silent-encode red this node closes, quoted from the previous surface.
///
/// A batch whose record was mutated into block index 98304 after construction
/// published that index silently, because the previous encoder applied no
/// value gate of its own — it trusted the constructor and only panicked at its
/// `expect` when a mutated stack violated the domain rule. An empty batch and
/// a duplicate or descending identity pair published silently as well, so the
/// count-zero, count-over and order relations the Go validator enforces were
/// not observable on the Rust surface. The fallible surface turns all of them
/// into value errors before any capacity decision.
#[test]
fn item_drops_mutated_public_fields_are_never_published() {
    let mut above = canonical_upserts();
    above.drops[0].block_index = MAX_CHUNK_BLOCK_INDEX;
    assert_eq!(above.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(above.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(above.encode(), Err(ProtocolError::InvalidRange));

    let mut empty = canonical_upserts();
    empty.drops.clear();
    assert_eq!(empty.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(empty.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(empty.encode(), Err(ProtocolError::InvalidRange));

    let mut duplicate = canonical_removes();
    duplicate.ids.push(duplicate.ids[0]);
    assert_eq!(duplicate.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(duplicate.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(duplicate.encode(), Err(ProtocolError::InvalidRange));

    let mut reversed = canonical_removes();
    reversed.ids.insert(0, drop_id(256, 0, 0, 0, 1));
    assert_eq!(reversed.validate(), Err(ProtocolError::InvalidRange));
    assert_eq!(reversed.encoded_len(), Err(ProtocolError::InvalidRange));
    assert_eq!(reversed.encode(), Err(ProtocolError::InvalidRange));
}

#[test]
fn item_drops_decode_rejects_every_proper_truncation() {
    // Both canonical payloads are short enough for a full sweep, and every cut
    // is the short-payload boundary: the tick, the count prefix and the record
    // region all answer with the truncation the Go decoder reports for the
    // same bytes.
    for cut in 0..ITEM_DROP_UPSERTS_WIRE.len() {
        let error = ItemDropUpserts::decode(&ITEM_DROP_UPSERTS_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("upserts: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "upserts: truncation at {cut} bytes reports {error:?}"
        );
    }
    for cut in 0..ITEM_DROP_REMOVES_WIRE.len() {
        let error = ItemDropRemoves::decode(&ITEM_DROP_REMOVES_WIRE[..cut])
            .err()
            .unwrap_or_else(|| panic!("removes: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "removes: truncation at {cut} bytes reports {error:?}"
        );
    }
    // The full 553-byte batch is cut inside its header and its record region
    // rather than swept byte by byte: the header cuts answer at the tick and
    // the count prefix, and the record-region cuts answer at the minimum
    // record budget.
    let full = full_removes().encode().expect("encode the full batch");
    for cut in 0..=9 {
        let error = ItemDropRemoves::decode(&full[..cut])
            .err()
            .unwrap_or_else(|| panic!("full batch: truncation at {cut} bytes decoded"));
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "full batch: truncation at {cut} bytes reports {error:?}"
        );
    }
    for record in 1..=MAX_ITEM_DROP_BATCH as usize {
        for offset in [
            record * DROP_ID_WIRE_BYTES - 1,
            record * DROP_ID_WIRE_BYTES / 2,
        ] {
            let cut = 9 + offset;
            let error = ItemDropRemoves::decode(&full[..cut])
                .err()
                .unwrap_or_else(|| panic!("full batch: truncation at {cut} bytes decoded"));
            assert_eq!(
                error,
                ProtocolError::Truncated,
                "full batch: truncation at {cut} bytes reports {error:?}"
            );
        }
    }
}

#[test]
fn item_drops_decode_rejects_one_trailing_byte() {
    // Both families apply the minimum-records rule, so the records decode and
    // the remaining byte is answered by the end-of-payload check. The Go
    // decoder applies the same rule — it rejects a payload shorter than the
    // declared records and then reports `network: trailing bytes` — so the two
    // sides publish one boundary for the same bytes. The companion batch is
    // the contrast case: its Go decoder applies an exact remaining-length rule
    // and answers the same extra byte at the truncation boundary.
    let mut upserts = ITEM_DROP_UPSERTS_WIRE.to_vec();
    upserts.push(0x00);
    assert_eq!(
        ItemDropUpserts::decode(&upserts),
        Err(ProtocolError::TrailingBytes)
    );
    let mut removes = ITEM_DROP_REMOVES_WIRE.to_vec();
    removes.push(0x00);
    assert_eq!(
        ItemDropRemoves::decode(&removes),
        Err(ProtocolError::TrailingBytes)
    );
    let mut ordered_pair = ITEM_DROP_ORDERED_PAIR_WIRE.to_vec();
    ordered_pair.push(0x00);
    assert_eq!(
        ItemDropRemoves::decode(&ordered_pair),
        Err(ProtocolError::TrailingBytes)
    );
    let mut full = full_removes().encode().expect("encode the full batch");
    full.push(0x00);
    assert_eq!(
        ItemDropRemoves::decode(&full),
        Err(ProtocolError::TrailingBytes)
    );
}

#[test]
fn item_drops_encode_into_leaves_a_short_destination_unchanged() {
    let cases: Vec<(
        &str,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "item drop upserts",
            ITEM_DROP_UPSERTS_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_upserts().encode_into(dst)),
        ),
        (
            "item drop upserts at the ceiling",
            8 + 1 + MAX_ITEM_DROP_BATCH as usize * ITEM_DROP_WIRE_BYTES,
            Box::new(|dst: &mut [u8]| full_upserts().encode_into(dst)),
        ),
        (
            "item drop removes",
            ITEM_DROP_REMOVES_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_removes().encode_into(dst)),
        ),
        (
            "item drop removes at the ceiling",
            8 + 1 + MAX_ITEM_DROP_BATCH as usize * DROP_ID_WIRE_BYTES,
            Box::new(|dst: &mut [u8]| full_removes().encode_into(dst)),
        ),
    ];
    for (label, length, encode_into) in cases {
        for available in [0usize, length - 1, length - 3] {
            let mut dst = vec![SENTINEL; available];
            let error = encode_into(&mut dst).expect_err("a short destination must be refused");
            assert_eq!(
                error,
                ProtocolError::OutputTooSmall {
                    needed: length,
                    available
                },
                "{label}: short destination of {available} bytes reports the wrong error"
            );
            assert!(
                dst.iter().all(|byte| *byte == SENTINEL),
                "{label}: short destination of {available} bytes was modified"
            );
        }
    }
}

#[test]
fn item_drops_packet_ids_are_pinned() {
    assert_eq!(ItemDropUpserts::PACKET_ID, 11);
    assert_eq!(ItemDropRemoves::PACKET_ID, 12);
}
