//! The two server-to-client world delta packet families: the common fallible
//! surface, the variant split the collapsed constructor check needed, and the
//! batch rules that differ between the two families.
//!
//! `BlockChanges` (S/Play/1) and `ForgetChunks` (S/Play/2) are the first
//! variable-count batch families in this crate: a canonical uvarint record
//! count followed by fixed-stride records. `BlockChanges` admits a zero count
//! as the revision barrier an item-only tick carries, while `ForgetChunks`
//! refuses zero because an empty forget batch has no observable meaning. The
//! block changes are sorted by chunk-ordered block index; the forgotten
//! chunks keep the submitted wire order and are only checked for
//! uniqueness, which is the order the authority replayed.
//!
//! Both records are concrete packets with public mutable fields, so the group
//! pins the design's surface order (`validate` → checked `encoded_len` →
//! capacity check → `publish_packet`): a short or invalid `encode_into` leaves
//! the caller's buffer untouched, an invalid value wins over a short
//! destination, and every proper truncation of a canonical payload rejects.
//! The wire literals are the Go encoder's output, so the round-trip tests pin
//! byte-level parity rather than self-agreement. The corpus evidence these
//! families publish is executed by `tests/protocol_corpus.rs` once the
//! controller integrates the exported assets, so this suite stays
//! self-contained and needs no corpus files.
//!
//! The category split the group pins is one controller ruling made observable:
//! the Go validator answers an unregistered block with its own `invalid-enum`
//! message and an out-of-world Y with a range violation, so the constructor
//! check is split the same way and a Y-span violation reports `InvalidRange`
//! where it used to report `InvalidEnum`. Both sides publish one category for
//! the same bytes.

use mornlea_domain::Dimension;
use mornlea_protocol::{
    BlockChange, BlockChanges, ForgetChunks, MAX_BLOCK_CHANGES, MAX_FORGET_CHUNKS, MAX_Y, MIN_Y,
    ProtocolError,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The reviewed 43-byte BlockChanges literal: dimension 1 (Depths), chunk
/// (−1,0), base revision 1, new revision 2, the canonical uvarint count 1 and
/// one change at x=−1, y=−64, z=0, block 1.
const BLOCK_CHANGES_WIRE: [u8; 43] = [
    0x01, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xff, 0xff, 0xff,
    0xff, 0xc0, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
];

/// The reviewed 29-byte empty barrier: the same header with uvarint count 0
/// and no records, which a revision step with no block write carries.
const BLOCK_CHANGES_EMPTY_WIRE: [u8; 29] = [
    0x01, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
];

/// The reviewed 21-byte ForgetChunks literal: dimension 1 (Depths), the
/// canonical uvarint count 2 and the chunks (1,0) and (−1,0) in the submitted
/// order, which is the order the wire preserves.
const FORGET_CHUNKS_WIRE: [u8; 21] = [
    0x01, 0x00, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff,
    0xff, 0x00, 0x00, 0x00, 0x00,
];

/// The one change the canonical BlockChanges vector carries.
fn canonical_change() -> BlockChange {
    BlockChange {
        x: -1,
        y: -64,
        z: 0,
        block: 1,
    }
}

/// The canonical BlockChanges record the reviewed literals carry.
fn canonical_block_changes() -> BlockChanges {
    BlockChanges::new(Dimension::DEPTHS, -1, 0, 1, 2, vec![canonical_change()])
        .expect("the canonical block changes record is valid")
}

/// The empty barrier record the reviewed literal carries.
fn empty_barrier() -> BlockChanges {
    BlockChanges::new(Dimension::DEPTHS, -1, 0, 1, 2, Vec::new())
        .expect("a zero-count batch is the revision barrier")
}

/// The canonical ForgetChunks record the reviewed literal carries.
fn canonical_forget_chunks() -> ForgetChunks {
    ForgetChunks::new(Dimension::DEPTHS, vec![(1, 0), (-1, 0)])
        .expect("an unsorted unique list is valid")
}

/// Builds a block-change column of `count` records at strictly increasing
/// chunk-ordered indices inside the announced chunk, at a fixed block.
///
/// The index decomposition is the `chunk_block_index` rule: local X is the
/// fastest axis, then local Z, then local Y, then the section, so ascending
/// cell indices are exactly what the sorted rule admits.
fn sorted_block_changes(count: usize, chunk_x: i32, chunk_z: i32) -> Vec<BlockChange> {
    (0..count)
        .map(|index| {
            let index = index as u32;
            let section = index / 4096;
            let local = index % 4096;
            let local_y = local / 256;
            let local_z = (local % 256) / 16;
            let local_x = local % 16;
            BlockChange {
                x: chunk_x * 16 + local_x as i32,
                y: MIN_Y + section as i32 * 16 + local_y as i32,
                z: chunk_z * 16 + local_z as i32,
                block: 2,
            }
        })
        .collect()
}

/// Builds `count` unique chunk coordinates in a deterministic ascending
/// layout, which the uniqueness rule admits in any order.
fn unique_chunks(count: usize) -> Vec<(i32, i32)> {
    (0..count)
        .map(|index| ((index % 64) as i32, (index / 64) as i32))
        .collect()
}

/// Renders the canonical uvarint of `value`, the encoder's own shortest form,
/// so a reviewed header mutation is the encoding of the value it names.
fn uvarint_bytes(value: u32) -> Vec<u8> {
    let mut encoded = Vec::new();
    let mut value = value;
    while value >= 1 << 7 {
        encoded.push((value as u8) | 0x80);
        value >>= 7;
    }
    encoded.push(value as u8);
    encoded
}

/// Asserts the shared surface contract for one canonical record: the reviewed
/// length, the exact window, the padded prefix, the allocating wrapper and the
/// decode/re-encode round trip.
///
/// The families differ in their record type but not in the surface, so the
/// helper takes the pieces it needs instead of duplicating the assertions.
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
    validate().unwrap_or_else(|err| panic!("{label}: valid record fails validation: {err:?}"));
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
        "{label}: decoded record differs from the encoded one"
    );
    let reencoded = re_encode().unwrap_or_else(|err| panic!("{label}: re-encode fails: {err:?}"));
    assert_eq!(
        reencoded, payload,
        "{label}: re-encoded payload differs from the reviewed bytes"
    );
}

#[test]
fn world_delta_block_changes_round_trips_through_the_fallible_surface() {
    let record = canonical_block_changes();
    let decoded = BlockChanges::decode(&BLOCK_CHANGES_WIRE).expect("decode the canonical payload");
    assert_round_trip(
        "block changes",
        &BLOCK_CHANGES_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| BlockChanges::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(BLOCK_CHANGES_WIRE.len(), 43);
    assert_eq!(record.changes, vec![canonical_change()]);
}

#[test]
fn world_delta_block_changes_empty_barrier_round_trips() {
    // A zero-count batch is legal as the revision barrier: the count, the
    // revision transition and the header are all the record carries.
    let record = empty_barrier();
    let decoded = BlockChanges::decode(&BLOCK_CHANGES_EMPTY_WIRE).expect("decode the barrier");
    assert_round_trip(
        "empty barrier",
        &BLOCK_CHANGES_EMPTY_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| BlockChanges::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(BLOCK_CHANGES_EMPTY_WIRE.len(), 29);
    assert!(record.changes.is_empty());
    assert!(decoded.changes.is_empty());
}

#[test]
fn world_delta_forget_chunks_round_trips_through_the_fallible_surface() {
    let record = canonical_forget_chunks();
    let decoded = ForgetChunks::decode(&FORGET_CHUNKS_WIRE).expect("decode the canonical payload");
    assert_round_trip(
        "forget chunks",
        &FORGET_CHUNKS_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| ForgetChunks::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(FORGET_CHUNKS_WIRE.len(), 21);
    assert_eq!(record.dimension, Dimension::DEPTHS);
    assert_eq!(record.chunks, vec![(1, 0), (-1, 0)]);
}

#[test]
fn world_delta_forget_chunks_preserves_the_unsorted_wire_order() {
    // The submitted order [(1,0),(−1,0)] is not sorted, and neither the wire
    // nor the record sorts it: the authority replayed the same sequence, so a
    // normalized comparison that sorted the chunks would hide a reordering.
    let record = canonical_forget_chunks();
    assert_eq!(
        record.chunks,
        vec![(1, 0), (-1, 0)],
        "the record reordered the submitted chunks"
    );
    assert_eq!(
        record.encode().expect("encode"),
        FORGET_CHUNKS_WIRE.to_vec()
    );
    let decoded = ForgetChunks::decode(&FORGET_CHUNKS_WIRE).expect("decode");
    assert_eq!(
        decoded.chunks,
        vec![(1, 0), (-1, 0)],
        "the decode reordered the wire chunks"
    );
    assert_eq!(
        decoded.encode().expect("re-encode"),
        FORGET_CHUNKS_WIRE.to_vec(),
        "the re-encode reordered the wire chunks"
    );
}

#[test]
fn world_delta_decode_checks_the_count_bound_before_the_record_length() {
    // A payload whose count is one above the ceiling is refused at the count
    // bound even though its remaining bytes cannot hold that many records, so
    // the Go decoder's order — count first, length second — is mirrored here.
    let mut above = Vec::from(&BLOCK_CHANGES_WIRE[..28]);
    above.extend_from_slice(&uvarint_bytes(4097));
    above.extend_from_slice(&canonical_change_wire());
    assert_eq!(
        BlockChanges::decode(&above),
        Err(ProtocolError::InvalidRange),
        "the count bound has to fire before the record-length rule"
    );

    let mut forget_above = Vec::from(&FORGET_CHUNKS_WIRE[..4]);
    forget_above.extend_from_slice(&uvarint_bytes(4097));
    forget_above.extend_from_slice(&[0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00]);
    assert_eq!(
        ForgetChunks::decode(&forget_above),
        Err(ProtocolError::InvalidRange),
        "the forget count bound has to fire before the record-length rule"
    );
}

/// The 14-byte wire form of the canonical change.
fn canonical_change_wire() -> [u8; 14] {
    let mut wire = [0u8; 14];
    wire[0..4].copy_from_slice(&(-1i32).to_le_bytes());
    wire[4..8].copy_from_slice(&(-64i32).to_le_bytes());
    wire[8..12].copy_from_slice(&0i32.to_le_bytes());
    wire[12..14].copy_from_slice(&1u16.to_le_bytes());
    wire
}

#[test]
fn world_delta_block_changes_count_boundaries_are_pinned() {
    // The maximum column of changes at strictly increasing indices admits, and
    // one more is refused — also when the extra record additionally violates a
    // later rule, because the count bound fires first.
    assert_eq!(MAX_BLOCK_CHANGES, 4096);
    let maximum = sorted_block_changes(MAX_BLOCK_CHANGES as usize, -1, 0);
    let admitted = BlockChanges::new(Dimension::DEPTHS, -1, 0, 1, 2, maximum.clone())
        .expect("4096 sorted changes are the maximum column");
    assert_eq!(admitted.changes.len(), MAX_BLOCK_CHANGES as usize);
    assert_eq!(
        admitted.encoded_len().expect("encoded_len"),
        28 + uvarint_bytes(MAX_BLOCK_CHANGES).len() + 14 * MAX_BLOCK_CHANGES as usize
    );

    let mut above = maximum;
    above.push(BlockChange {
        x: -1,
        y: MAX_Y,
        z: 0,
        block: 1,
    });
    assert_eq!(
        BlockChanges::new(Dimension::DEPTHS, -1, 0, 1, 2, above),
        Err(ProtocolError::InvalidRange),
        "the 4097th change also violates the world span, and the count bound still fires first"
    );
    assert_eq!(
        BlockChanges::new(
            Dimension::DEPTHS,
            -1,
            0,
            1,
            2,
            sorted_block_changes(4097, -1, 0)
        ),
        Err(ProtocolError::InvalidRange),
        "4097 changes are refused"
    );
}

#[test]
fn world_delta_forget_chunks_count_boundaries_are_pinned() {
    // The maximum unique column admits, and one more or zero are refused.
    assert_eq!(MAX_FORGET_CHUNKS, 4096);
    let maximum = unique_chunks(MAX_FORGET_CHUNKS as usize);
    let admitted = ForgetChunks::new(Dimension::DEPTHS, maximum.clone())
        .expect("4096 unique chunks are the maximum batch");
    assert_eq!(admitted.chunks.len(), MAX_FORGET_CHUNKS as usize);
    assert_eq!(
        admitted.encoded_len().expect("encoded_len"),
        4 + uvarint_bytes(MAX_FORGET_CHUNKS).len() + 8 * MAX_FORGET_CHUNKS as usize
    );
    let mut above = maximum;
    above.push((4096, 0));
    assert_eq!(
        ForgetChunks::new(Dimension::DEPTHS, above),
        Err(ProtocolError::InvalidRange),
        "4097 chunks are refused"
    );
    assert_eq!(
        ForgetChunks::new(Dimension::DEPTHS, Vec::new()),
        Err(ProtocolError::InvalidRange),
        "a zero-count batch carries no observable meaning"
    );
}

/// One mutate-after-construction case: the invalid value a public mutable
/// field is set to after construction, and the error the surface has to
/// report for it before any size or capacity decision.
struct WorldDeltaMutation {
    label: &'static str,
    changes: Vec<BlockChange>,
    chunks: Vec<(i32, i32)>,
    base_revision: u64,
    new_revision: u64,
    error: ProtocolError,
}

/// The invalid mutable values. Each entry carries one violation, so the error
/// variant names the boundary that owns it.
fn world_delta_mutations() -> Vec<WorldDeltaMutation> {
    vec![
        WorldDeltaMutation {
            label: "base revision zero after construction",
            changes: vec![canonical_change()],
            chunks: Vec::new(),
            base_revision: 0,
            new_revision: 1,
            error: ProtocolError::InvalidRange,
        },
        WorldDeltaMutation {
            label: "revision gap after construction",
            changes: vec![canonical_change()],
            chunks: Vec::new(),
            base_revision: 1,
            new_revision: 3,
            error: ProtocolError::InvalidRange,
        },
        WorldDeltaMutation {
            label: "unregistered block after construction",
            changes: vec![BlockChange {
                x: -1,
                y: -64,
                z: 0,
                block: 90,
            }],
            chunks: Vec::new(),
            base_revision: 1,
            new_revision: 2,
            error: ProtocolError::InvalidEnum,
        },
        WorldDeltaMutation {
            label: "Y above the world span after construction",
            changes: vec![BlockChange {
                x: -1,
                y: 320,
                z: 0,
                block: 1,
            }],
            chunks: Vec::new(),
            base_revision: 1,
            new_revision: 2,
            error: ProtocolError::InvalidRange,
        },
        WorldDeltaMutation {
            label: "duplicate chunk after construction",
            changes: Vec::new(),
            chunks: vec![(1, 0), (1, 0)],
            base_revision: 0,
            new_revision: 0,
            error: ProtocolError::InvalidRange,
        },
    ]
}

#[test]
fn world_delta_invalid_value_wins_over_short_capacity() {
    for mutation in world_delta_mutations() {
        // The unmutated record sizes the destinations, so the capacity refusal
        // and the value refusal are compared on buffers of the same shape.
        let needed = if mutation.chunks.is_empty() {
            canonical_block_changes()
                .encoded_len()
                .expect("the valid record sizes")
        } else {
            canonical_forget_chunks()
                .encoded_len()
                .expect("the valid record sizes")
        };
        // Every mutation value is invalid, so the same buffer sizes a longer
        // or shorter refused record; the sentinel assertions below stay exact.
        let _ = needed;

        if mutation.chunks.is_empty() {
            let record = BlockChanges {
                dimension: Dimension::DEPTHS,
                chunk_x: -1,
                chunk_z: 0,
                base_revision: mutation.base_revision,
                new_revision: mutation.new_revision,
                changes: mutation.changes.clone(),
            };
            assert_eq!(
                record.validate(),
                Err(mutation.error),
                "{}: validate disagrees with the mutation",
                mutation.label
            );
            assert_eq!(
                record.encoded_len(),
                Err(mutation.error),
                "{}: encoded_len disagrees with the mutation",
                mutation.label
            );
            assert_eq!(
                record.encode(),
                Err(mutation.error),
                "{}: encode disagrees with the mutation",
                mutation.label
            );
            for available in [needed - 1, needed] {
                let mut dst = vec![SENTINEL; available];
                assert_eq!(
                    record.encode_into(&mut dst),
                    Err(mutation.error),
                    "{}: a destination of {available} bytes masks the value error",
                    mutation.label
                );
                assert!(
                    dst.iter().all(|byte| *byte == SENTINEL),
                    "{}: a destination of {available} bytes was modified on a value refusal",
                    mutation.label
                );
            }
            continue;
        }

        let record = ForgetChunks {
            dimension: Dimension::DEPTHS,
            chunks: mutation.chunks.clone(),
        };
        assert_eq!(
            record.validate(),
            Err(mutation.error),
            "{}: validate disagrees with the mutation",
            mutation.label
        );
        assert_eq!(
            record.encoded_len(),
            Err(mutation.error),
            "{}: encoded_len disagrees with the mutation",
            mutation.label
        );
        assert_eq!(
            record.encode(),
            Err(mutation.error),
            "{}: encode disagrees with the mutation",
            mutation.label
        );
        for available in [needed - 1, needed] {
            let mut dst = vec![SENTINEL; available];
            assert_eq!(
                record.encode_into(&mut dst),
                Err(mutation.error),
                "{}: a destination of {available} bytes masks the value error",
                mutation.label
            );
            assert!(
                dst.iter().all(|byte| *byte == SENTINEL),
                "{}: a destination of {available} bytes was modified on a value refusal",
                mutation.label
            );
        }
    }
}

#[test]
fn world_delta_the_variant_split_is_pinned() {
    // One ruling made observable: the Go validator answers an unregistered
    // block with its invalid-enum message and an out-of-world Y with a range
    // violation, so the shared constructor check is split and the two fields
    // of the same family report different variants for their own boundary.
    let unregistered_block = BlockChange {
        x: -1,
        y: -64,
        z: 0,
        block: 90,
    };
    assert_eq!(
        BlockChanges::new(Dimension::DEPTHS, -1, 0, 1, 2, vec![unregistered_block]),
        Err(ProtocolError::InvalidEnum),
        "an unregistered block is the invalid-enum boundary"
    );
    assert_eq!(
        BlockChanges {
            dimension: Dimension::DEPTHS,
            chunk_x: -1,
            chunk_z: 0,
            base_revision: 1,
            new_revision: 2,
            changes: vec![unregistered_block],
        }
        .validate(),
        Err(ProtocolError::InvalidEnum),
        "the mutated block field is refused by the same variant"
    );

    for (label, y) in [
        ("above the world span", MAX_Y),
        ("below the world span", MIN_Y - 1),
    ] {
        assert_eq!(
            BlockChanges::new(
                Dimension::DEPTHS,
                -1,
                0,
                1,
                2,
                vec![BlockChange {
                    x: -1,
                    y,
                    z: 0,
                    block: 1,
                }],
            ),
            Err(ProtocolError::InvalidRange),
            "{label}: a Y outside the world span is the split range variant"
        );
        assert_eq!(
            BlockChanges {
                dimension: Dimension::DEPTHS,
                chunk_x: -1,
                chunk_z: 0,
                base_revision: 1,
                new_revision: 2,
                changes: vec![BlockChange {
                    x: -1,
                    y,
                    z: 0,
                    block: 1,
                }],
            }
            .validate(),
            Err(ProtocolError::InvalidRange),
            "{label}: the mutated Y field is refused by the split variant"
        );
    }
}

#[test]
fn world_delta_revision_boundaries_are_pinned() {
    // The revision transition is the Go rule: a non-zero, non-saturated base
    // with a new revision of exactly base + 1, so the last legal step carries
    // base u64::MAX−1 and new u64::MAX.
    let last = BlockChanges::new(
        Dimension::DEPTHS,
        -1,
        0,
        u64::MAX - 1,
        u64::MAX,
        vec![canonical_change()],
    )
    .expect("the last legal revision step admits");
    assert_eq!(last.base_revision, u64::MAX - 1);
    assert_eq!(last.new_revision, u64::MAX);
    assert_eq!(
        BlockChanges::new(
            Dimension::DEPTHS,
            -1,
            0,
            u64::MAX,
            0,
            vec![canonical_change()]
        ),
        Err(ProtocolError::InvalidRange),
        "a saturated base cannot name a next revision"
    );
}

#[test]
fn world_delta_decode_rejects_every_proper_truncation() {
    // Cut 0 is the empty payload, where the first fixed-width read reports a
    // missing byte. A cut inside the header fails at that field; a cut inside
    // the count or a record fails at the budget check. Every cut is the same
    // boundary the Go decoder answers with its short-input sentinel.
    let cases: Vec<(&str, &[u8], fn(&[u8]) -> Result<(), ProtocolError>)> = vec![
        ("block changes", &BLOCK_CHANGES_WIRE, |payload| {
            BlockChanges::decode(payload).map(|_| ())
        }),
        ("empty barrier", &BLOCK_CHANGES_EMPTY_WIRE, |payload| {
            BlockChanges::decode(payload).map(|_| ())
        }),
        ("forget chunks", &FORGET_CHUNKS_WIRE, |payload| {
            ForgetChunks::decode(payload).map(|_| ())
        }),
    ];
    for (label, payload, decode) in cases {
        for cut in 0..payload.len() {
            let error = decode(&payload[..cut])
                .err()
                .unwrap_or_else(|| panic!("{label}: truncation at {cut} bytes decoded"));
            assert_eq!(
                error,
                ProtocolError::Truncated,
                "{label}: truncation at {cut} bytes reports {error:?}"
            );
        }
    }
}

#[test]
fn world_delta_decode_rejects_one_trailing_byte() {
    let cases: Vec<(&str, Vec<u8>, fn(&[u8]) -> Result<(), ProtocolError>)> = vec![
        (
            "block changes",
            {
                let mut wire = BLOCK_CHANGES_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| BlockChanges::decode(payload).map(|_| ()),
        ),
        (
            "empty barrier",
            {
                let mut wire = BLOCK_CHANGES_EMPTY_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| BlockChanges::decode(payload).map(|_| ()),
        ),
        (
            "forget chunks",
            {
                let mut wire = FORGET_CHUNKS_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| ForgetChunks::decode(payload).map(|_| ()),
        ),
    ];
    for (label, payload, decode) in cases {
        let error = decode(&payload)
            .err()
            .unwrap_or_else(|| panic!("{label}: one trailing byte decoded"));
        assert_eq!(
            error,
            ProtocolError::TrailingBytes,
            "{label}: one trailing byte reports {error:?}"
        );
    }
}

#[test]
fn world_delta_decode_rejects_the_pinned_invalid_values() {
    // The matrix mirrors the corpus negatives. Every entry is a single
    // violation, and the variant the Rust decoder reports is the one the Go
    // producer records for the same bytes.
    let base_zero = {
        let mut wire = BLOCK_CHANGES_WIRE.to_vec();
        wire[12..20].copy_from_slice(&0u64.to_le_bytes());
        wire
    };
    assert_eq!(
        BlockChanges::decode(&base_zero),
        Err(ProtocolError::InvalidRange),
        "base revision zero"
    );
    let revision_gap = {
        let mut wire = BLOCK_CHANGES_WIRE.to_vec();
        wire[20..28].copy_from_slice(&3u64.to_le_bytes());
        wire
    };
    assert_eq!(
        BlockChanges::decode(&revision_gap),
        Err(ProtocolError::InvalidRange),
        "revision gap"
    );
    let above_world = {
        let mut wire = BLOCK_CHANGES_WIRE.to_vec();
        wire[33..37].copy_from_slice(&320i32.to_le_bytes());
        wire
    };
    assert_eq!(
        BlockChanges::decode(&above_world),
        Err(ProtocolError::InvalidRange),
        "Y above the world span"
    );
    let wrong_chunk = {
        let mut wire = BLOCK_CHANGES_WIRE.to_vec();
        wire[29..33].copy_from_slice(&15i32.to_le_bytes());
        wire
    };
    assert_eq!(
        BlockChanges::decode(&wrong_chunk),
        Err(ProtocolError::InvalidRange),
        "change outside the announced chunk"
    );
    let unregistered_block = {
        let mut wire = BLOCK_CHANGES_WIRE.to_vec();
        wire[41..43].copy_from_slice(&90u16.to_le_bytes());
        wire
    };
    assert_eq!(
        BlockChanges::decode(&unregistered_block),
        Err(ProtocolError::InvalidEnum),
        "unregistered block"
    );
    let unsorted = {
        let mut wire = Vec::from(&BLOCK_CHANGES_WIRE[..28]);
        wire.push(0x02);
        for (x, y, z) in [(-1i32, -64i32, 1i32), (-1, -64, 0)] {
            wire.extend_from_slice(&x.to_le_bytes());
            wire.extend_from_slice(&y.to_le_bytes());
            wire.extend_from_slice(&z.to_le_bytes());
            wire.extend_from_slice(&1u16.to_le_bytes());
        }
        wire
    };
    assert_eq!(unsorted.len(), 28 + 1 + 28);
    assert_eq!(
        BlockChanges::decode(&unsorted),
        Err(ProtocolError::InvalidRange),
        "reversed chunk-ordered index"
    );

    let forget_zero_count = {
        let mut wire = Vec::from(&FORGET_CHUNKS_WIRE[..5]);
        wire[4] = 0x00;
        wire
    };
    assert_eq!(forget_zero_count.len(), 5);
    assert_eq!(
        ForgetChunks::decode(&forget_zero_count),
        Err(ProtocolError::InvalidRange),
        "a zero count carries no observable meaning"
    );
    let forget_duplicate = {
        let mut wire = Vec::from(&FORGET_CHUNKS_WIRE[..5]);
        wire.extend_from_slice(&1i32.to_le_bytes());
        wire.extend_from_slice(&0i32.to_le_bytes());
        wire.extend_from_slice(&1i32.to_le_bytes());
        wire.extend_from_slice(&0i32.to_le_bytes());
        wire
    };
    assert_eq!(
        ForgetChunks::decode(&forget_duplicate),
        Err(ProtocolError::InvalidRange),
        "duplicate chunk"
    );
    let dimension_two = {
        let mut wire = FORGET_CHUNKS_WIRE.to_vec();
        wire[0..4].copy_from_slice(&2i32.to_le_bytes());
        wire
    };
    assert_eq!(
        ForgetChunks::decode(&dimension_two),
        Err(ProtocolError::InvalidEnum),
        "unknown dimension"
    );
}

#[test]
fn world_delta_encode_into_leaves_a_short_destination_unchanged() {
    let cases: Vec<(
        &str,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "block changes",
            BLOCK_CHANGES_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_block_changes().encode_into(dst)),
        ),
        (
            "empty barrier",
            BLOCK_CHANGES_EMPTY_WIRE.len(),
            Box::new(|dst: &mut [u8]| empty_barrier().encode_into(dst)),
        ),
        (
            "forget chunks",
            FORGET_CHUNKS_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_forget_chunks().encode_into(dst)),
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
fn world_delta_packet_ids_are_pinned() {
    assert_eq!(BlockChanges::PACKET_ID, 1);
    assert_eq!(ForgetChunks::PACKET_ID, 2);
}
