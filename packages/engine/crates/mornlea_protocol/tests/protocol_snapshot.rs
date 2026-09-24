//! The compressed Play chunk-snapshot family (S/Play/0): the owned bounded
//! compression context, logical-layer cross-implementation parity, and the
//! failure boundaries that keep a hostile envelope or frame from becoming a
//! large allocation.
//!
//! This is the only family whose wire payload is compressed, so it is the only
//! family with a context owner: `ProtocolCodec` holds one zstd compressor and
//! one zstd decompressor for its lifetime, and the group test pins that the
//! encode path revalidates the record, compresses through the owned context
//! into bounded scratch, and only then tests the caller's destination.
//!
//! Cross-implementation compatibility is defined at the logical layer, never
//! at the compressed one: the committed Go fixture's logical payload is
//! reproduced byte for byte, a Rust-encoded frame decodes back to the identical
//! snapshot, and the frame parts the two implementations share — magic, header
//! descriptor, content size and the trailing xxhash-64 checksum — are pinned
//! against the fixture. No assertion in this suite compares a Rust compressed
//! block with a Go compressed block, because the two encoders legitimately
//! differ.
//!
//! The failure boundaries are the plan's four compressed-layer pins beside the
//! five logical-layer pins. Envelope declared-length violations report
//! `FrameTooLarge`, a payload cut off inside the compressed body is refused by
//! the envelope's remaining-length check as `Truncated`, and a byte flipped
//! inside a length-complete frame is refused by the frame's own checksum as
//! `Integrity` — the first family to publish that category, which the envelope
//! checks could not catch. A frame that decompresses to a different length
//! than the envelope declares stays `Truncated`; the two conditions are never
//! conflated.

use std::fs;
use std::path::PathBuf;

use mornlea_domain::Dimension;
use mornlea_protocol::{
    BLOCKS_PER_SECTION, MAX_COMPRESSED_SNAPSHOT, MAX_DECODED_SNAPSHOT, ProtocolCodec,
    ProtocolError, SECTIONS_PER_CHUNK, SectionData,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// Reads the committed Go chunk-snapshot fixture verbatim.
fn read_go_snapshot_fixture() -> Vec<u8> {
    let path = PathBuf::from(env!("CARGO_MANIFEST_DIR"))
        .join("../../../../packages/shared/network/codec/testdata/chunk-snapshot-v1.bin");
    fs::read(&path).unwrap_or_else(|err| panic!("read {}: {err}", path.display()))
}

/// Packs one section's cells the way the Go fixture builder does, so the
/// golden snapshot is the same logical value the committed fixture carries.
fn packed_words(bits: u8, modulus: usize, seed: usize) -> Vec<u64> {
    let per_word = 64 / bits as usize;
    let words = (BLOCKS_PER_SECTION + per_word - 1) / per_word;
    let mut packed = vec![0u64; words];
    for index in 0..BLOCKS_PER_SECTION {
        let value = ((index + seed) % modulus) as u64;
        packed[index / per_word] |= value << ((index % per_word) * bits as usize);
    }
    packed
}

/// Packs one section whose cells all carry `value`.
fn packed_words_uniform(bits: u8, value: u64) -> Vec<u64> {
    let per_word = 64 / bits as usize;
    let words = (BLOCKS_PER_SECTION + per_word - 1) / per_word;
    let mask = (1u64 << bits) - 1;
    let mut packed = vec![0u64; words];
    for index in 0..BLOCKS_PER_SECTION {
        let shift = (index % per_word) * bits as usize;
        packed[index / per_word] |= (value & mask) << shift;
    }
    packed
}

/// Packs one uniform section whose last cell carries `last` instead.
fn packed_words_with_last(bits: u8, value: u64, last: u64) -> Vec<u64> {
    let mut packed = packed_words_uniform(bits, value);
    let per_word = 64 / bits as usize;
    let last_index = BLOCKS_PER_SECTION - 1;
    let shift = (last_index % per_word) * bits as usize;
    let mask = (1u64 << bits) - 1;
    packed[last_index / per_word] &= !(mask << shift);
    packed[last_index / per_word] |= (last & mask) << shift;
    packed
}

/// The committed fixture's logical snapshot: overworld chunk (-3, 7) at
/// revision 19, cycling through all three section storages.
fn golden_snapshot() -> mornlea_protocol::ChunkSnapshot {
    let mut sections = Vec::with_capacity(SECTIONS_PER_CHUNK);
    for y in 0..SECTIONS_PER_CHUNK {
        let section = match y % 4 {
            0 => SectionData::single(y as i32, (y % 6) as u16),
            1 => SectionData::indexed(y as i32, 4, vec![0, 2, 3], packed_words(4, 3, y)),
            2 => SectionData::indexed(
                y as i32,
                8,
                vec![0, 1, 2, 3, 4, 5, 27, 34],
                packed_words(8, 8, y),
            ),
            _ => SectionData::direct(y as i32, packed_words(15, 35, y)),
        };
        sections.push(section);
    }
    mornlea_protocol::ChunkSnapshot::new(Dimension::OVERWORLD, -3, 7, 19, sections)
        .expect("golden snapshot")
}

/// The all-single canonical vector: 24 single sections with distinct
/// registered block numbers.
fn single_sections() -> Vec<SectionData> {
    (0..SECTIONS_PER_CHUNK)
        .map(|y| SectionData::single(y as i32, y as u16))
        .collect()
}

/// The canonical mixed vector: dimension 1 (Depths), chunk (-1, 0), revision 1,
/// with Y0..5 single, Y6..11 indexed 4-bit, Y12..17 indexed 8-bit over the
/// ninety-ID palette, and Y18..23 direct 15-bit with block 89 at the last cell
/// of Y23. Every kind is exercised and the palette order and packed words are
/// preserved by the round trip.
fn mixed_sections() -> Vec<SectionData> {
    let mut sections = Vec::with_capacity(SECTIONS_PER_CHUNK);
    for y in 0..SECTIONS_PER_CHUNK {
        let section = match y {
            0..=5 => SectionData::single(y as i32, y as u16),
            6..=11 => SectionData::indexed(y as i32, 4, vec![1, 2], packed_words(4, 2, y)),
            12..=17 => {
                SectionData::indexed(y as i32, 8, (0..90u16).collect(), packed_words(8, 90, y))
            }
            23 => SectionData::direct(y as i32, packed_words_with_last(15, 1, 89)),
            _ => SectionData::direct(y as i32, packed_words_uniform(15, 1)),
        };
        sections.push(section);
    }
    sections
}

/// The canonical mixed vector as one snapshot record.
fn mixed_snapshot() -> mornlea_protocol::ChunkSnapshot {
    mornlea_protocol::ChunkSnapshot::new(Dimension::DEPTHS, -1, 0, 1, mixed_sections())
        .expect("the canonical mixed snapshot is valid")
}

/// Asserts the frame parts the two implementations share: the zstd magic, the
/// frame header descriptor (single segment, four-byte content size, checksum
/// on) and the declared content size.
fn assert_frame_pins(frame: &[u8], logical_len: usize) {
    assert!(frame.len() >= 13, "frame is too short to carry a header");
    assert_eq!(&frame[0..4], &[0x28, 0xb5, 0x2f, 0xfd], "zstd magic");
    assert_eq!(frame[4], 0xa4, "frame header descriptor");
    assert_eq!(
        &frame[5..9],
        &(logical_len as u32).to_le_bytes(),
        "frame content size must equal the logical length"
    );
}

/// Encodes one snapshot through a fresh owned context into a generous buffer.
fn encode_with_codec(
    codec: &mut ProtocolCodec,
    packet: &mornlea_protocol::ChunkSnapshot,
) -> Vec<u8> {
    let mut dst = vec![0u8; MAX_COMPRESSED_SNAPSHOT];
    let written = codec
        .encode_snapshot_into(packet, &mut dst)
        .expect("the canonical snapshot encodes");
    dst.truncate(written);
    dst
}

#[test]
fn snapshot_committed_fixture_decodes_to_the_golden_logical_payload() {
    let fixture = read_go_snapshot_fixture();
    let snapshot = golden_snapshot();
    let logical = snapshot
        .encode_logical_checked()
        .expect("the golden snapshot encodes logically");

    let envelope =
        mornlea_protocol::ChunkSnapshot::decode_envelope(&fixture).expect("fixture envelope");
    assert_eq!(envelope.decoded_length(), 86_295);
    assert_eq!(envelope.compressed().len(), 439);
    assert_eq!(
        envelope.decompress().expect("fixture logical"),
        logical,
        "the Go fixture's logical payload is not reproduced byte for byte"
    );
    assert_eq!(
        mornlea_protocol::ChunkSnapshot::decode(&fixture).expect("fixture decode"),
        snapshot
    );

    // The frame parts the two implementations share, pinned against the
    // fixture: magic, descriptor, content size and the trailing checksum of a
    // Rust re-encoding of the same logical content.
    let mut codec = ProtocolCodec::new().expect("snapshot codec");
    let reencoded = encode_with_codec(&mut codec, &snapshot);
    assert_frame_pins(&fixture[8..], logical.len());
    assert_frame_pins(&reencoded[8..], logical.len());
    assert_eq!(
        &fixture[fixture.len() - 4..],
        &reencoded[reencoded.len() - 4..],
        "xxhash-64 content checksum must match across implementations"
    );
}

#[test]
fn snapshot_logical_encoding_round_trips_deterministically() {
    // Logical determinism is the pinned cross-implementation property: the
    // Rust logical bytes of the mixed vector decode back to the identical
    // record, and a second encoding through a second context publishes the
    // same logical bytes.
    let snapshot = mixed_snapshot();
    let logical = snapshot
        .encode_logical_checked()
        .expect("the mixed snapshot encodes logically");
    assert_eq!(logical.len(), 87_267);
    assert_eq!(
        mornlea_protocol::ChunkSnapshot::decode_logical(&logical).expect("logical decode"),
        snapshot
    );
    let mut codec = ProtocolCodec::new().expect("snapshot codec");
    let payload = encode_with_codec(&mut codec, &snapshot);
    let own_logical = mornlea_protocol::ChunkSnapshot::decode_envelope(&payload)
        .expect("envelope")
        .decompress()
        .expect("own logical");
    assert_eq!(
        own_logical, logical,
        "the logical payload is not deterministic"
    );
    let second = encode_with_codec(&mut codec, &snapshot);
    assert_eq!(second, payload, "the owned context is not deterministic");
}

#[test]
fn snapshot_codec_round_trips_the_mixed_vector_and_reuses_one_context() {
    let snapshot = mixed_snapshot();
    let logical = snapshot
        .encode_logical_checked()
        .expect("the mixed snapshot encodes logically");
    let mut codec = ProtocolCodec::new().expect("snapshot codec");

    let payload = encode_with_codec(&mut codec, &snapshot);
    // The envelope leads the payload and describes the frame exactly.
    assert_eq!(
        &payload[0..4],
        &(logical.len() as u32).to_le_bytes(),
        "declared decoded length"
    );
    assert_eq!(
        &payload[4..8],
        &((payload.len() - 8) as u32).to_le_bytes(),
        "declared compressed length"
    );
    assert_frame_pins(&payload[8..], logical.len());

    // The context decodes its own output, and stays usable for the next call:
    // the decoded record is identical and the logical bytes of the frame are
    // the canonical encoding.
    let decoded = codec
        .decode_snapshot(&payload)
        .expect("the codec decodes its own output");
    assert_eq!(decoded, snapshot);
    assert_eq!(decoded.sections.len(), SECTIONS_PER_CHUNK);
    assert_eq!(decoded.dimension, Dimension::DEPTHS);
    assert_eq!((decoded.chunk_x, decoded.chunk_z), (-1, 0));
    assert_eq!(decoded.revision, 1);
    for (index, section) in decoded.sections.iter().enumerate() {
        assert_eq!(section.y, index as i32, "section Y at index {index}");
    }
    let own_logical = mornlea_protocol::ChunkSnapshot::decode_envelope(&payload)
        .expect("envelope")
        .decompress()
        .expect("own logical");
    assert_eq!(own_logical, logical);

    // The same context serves a second, structurally different snapshot in
    // both directions.
    let all_single =
        mornlea_protocol::ChunkSnapshot::new(Dimension::DEPTHS, -1, 0, 1, single_sections())
            .expect("the all-single snapshot is valid");
    let second = encode_with_codec(&mut codec, &all_single);
    assert_eq!(
        codec
            .decode_snapshot(&second)
            .expect("decode the second payload"),
        all_single
    );
}

#[test]
fn snapshot_sections_pop_after_construction_returns_a_typed_error() {
    // The plan's red: mutating `sections.pop()` after construction used to
    // panic inside the infallible logical encoder. The checked surface refuses
    // it with the section-count boundary instead, before any scratch is
    // reserved and without touching the caller's destination.
    let mut snapshot = mixed_snapshot();
    snapshot.sections.pop();
    assert_eq!(
        snapshot.encode_logical_checked(),
        Err(ProtocolError::InvalidRange),
        "a 23-section column is the count boundary"
    );
    let mut codec = ProtocolCodec::new().expect("snapshot codec");
    let mut dst = vec![SENTINEL; 4096];
    assert_eq!(
        codec.encode_snapshot_into(&snapshot, &mut dst),
        Err(ProtocolError::InvalidRange),
        "the value error wins over any capacity question"
    );
    assert!(
        dst.iter().all(|byte| *byte == SENTINEL),
        "a value refusal modified the destination"
    );
}

#[test]
fn snapshot_invalid_value_wins_over_short_capacity() {
    // Every mutation carries exactly one violation, so the variant the gate
    // reports names the boundary that owns it, and the same variant is
    // published whether the destination is generous or empty.
    let mutations: Vec<(
        &str,
        Box<dyn Fn(&mut mornlea_protocol::ChunkSnapshot)>,
        ProtocolError,
    )> = vec![
        (
            "revision zero after construction",
            Box::new(|snapshot: &mut mornlea_protocol::ChunkSnapshot| snapshot.revision = 0),
            ProtocolError::InvalidRange,
        ),
        (
            "section removed after construction",
            Box::new(|snapshot: &mut mornlea_protocol::ChunkSnapshot| {
                snapshot.sections.pop();
            }),
            ProtocolError::InvalidRange,
        ),
        (
            "adjacent sections with swapped Y after construction",
            Box::new(|snapshot: &mut mornlea_protocol::ChunkSnapshot| {
                snapshot.sections.swap(3, 4);
            }),
            ProtocolError::InvalidRange,
        ),
        (
            "palette slot beyond the palette after construction",
            Box::new(|snapshot: &mut mornlea_protocol::ChunkSnapshot| {
                snapshot.sections[6].packed[0] |= 2;
            }),
            ProtocolError::InvalidRange,
        ),
        (
            "direct word with high bits after construction",
            Box::new(|snapshot: &mut mornlea_protocol::ChunkSnapshot| {
                snapshot.sections[18].packed[0] |= 1 << 60;
            }),
            ProtocolError::InvalidRange,
        ),
    ];
    let mut codec = ProtocolCodec::new().expect("snapshot codec");
    for (label, mutate, error) in mutations {
        let mut snapshot = mixed_snapshot();
        mutate(&mut snapshot);
        assert_eq!(
            snapshot.validate(),
            Err(error),
            "{label}: validate disagrees with the mutation"
        );
        assert_eq!(
            snapshot.encode_logical_checked(),
            Err(error),
            "{label}: encode_logical_checked disagrees with the mutation"
        );
        for available in [0usize, 1, 64, 4096] {
            let mut dst = vec![SENTINEL; available];
            assert_eq!(
                codec.encode_snapshot_into(&snapshot, &mut dst),
                Err(error),
                "{label}: a destination of {available} bytes masks the value error"
            );
            assert!(
                dst.iter().all(|byte| *byte == SENTINEL),
                "{label}: a destination of {available} bytes was modified on a value refusal"
            );
        }
    }
}

#[test]
fn snapshot_short_destination_is_refused_with_every_byte_unchanged() {
    let snapshot = mixed_snapshot();
    let logical = snapshot
        .encode_logical_checked()
        .expect("the mixed snapshot encodes logically");
    let mut codec = ProtocolCodec::new().expect("snapshot codec");

    // Size the refusal from a first, generous encoding, then refuse every
    // shorter destination on the same record.
    let payload = encode_with_codec(&mut codec, &snapshot);
    let needed = payload.len();
    for available in [0usize, needed - 1, needed - 8, 8] {
        let mut dst = vec![SENTINEL; available];
        assert_eq!(
            codec.encode_snapshot_into(&snapshot, &mut dst),
            Err(ProtocolError::OutputTooSmall { needed, available }),
            "short destination of {available} bytes reports the wrong error"
        );
        assert!(
            dst.iter().all(|byte| *byte == SENTINEL),
            "short destination of {available} bytes was modified"
        );
    }

    // An exact destination receives exactly the record, and a larger one keeps
    // every byte beyond it.
    let mut exact = vec![SENTINEL; needed];
    let written = codec
        .encode_snapshot_into(&snapshot, &mut exact)
        .expect("the exact destination accepts the record");
    assert_eq!(written, needed);
    assert_eq!(exact, payload);
    let mut padded = vec![SENTINEL; needed + 3];
    let written = codec
        .encode_snapshot_into(&snapshot, &mut padded)
        .expect("the padded destination accepts the record");
    assert_eq!(written, needed);
    assert_eq!(&padded[..needed], payload.as_slice());
    assert!(
        padded[needed..].iter().all(|byte| *byte == SENTINEL),
        "the padded write touched bytes beyond the record"
    );
    assert_eq!(logical.len(), 87_267);
}

#[test]
fn snapshot_compressed_layer_negatives_are_refused_at_their_boundary() {
    let snapshot = mixed_snapshot();
    let mut codec = ProtocolCodec::new().expect("snapshot codec");
    let valid = encode_with_codec(&mut codec, &snapshot);

    // Declared-length violations are envelope boundaries: the ceiling is
    // checked before the frame is touched.
    let mut compressed_cap = valid.clone();
    compressed_cap[4..8].copy_from_slice(&1_048_577u32.to_le_bytes());
    let mut decoded_cap = valid.clone();
    decoded_cap[0..4].copy_from_slice(&2_097_153u32.to_le_bytes());
    // A payload cut off inside the compressed body keeps the declared
    // compressed length, so the envelope's remaining-length check fires first.
    let mut truncated = valid.clone();
    truncated.truncate(truncated.len() - 5);
    // A byte flipped inside a length-complete frame is refused by the frame's
    // own content checksum.
    let mut bad_checksum = valid.clone();
    let last = bad_checksum.len() - 1;
    bad_checksum[last] ^= 0xff;

    let cases: Vec<(&str, Vec<u8>, ProtocolError)> = vec![
        (
            "compressed length above the cap",
            compressed_cap,
            ProtocolError::FrameTooLarge,
        ),
        (
            "decoded length above the cap",
            decoded_cap,
            ProtocolError::FrameTooLarge,
        ),
        ("truncated zstd frame", truncated, ProtocolError::Truncated),
        ("checksum failure", bad_checksum, ProtocolError::Integrity),
    ];
    for (label, payload, error) in cases {
        assert_eq!(
            codec.decode_snapshot(&payload),
            Err(error),
            "{label}: the owned decoder context reports the wrong variant"
        );
        assert_eq!(
            mornlea_protocol::ChunkSnapshot::decode(&payload),
            Err(error),
            "{label}: the one-shot decode path disagrees with the owned context"
        );
    }

    // The envelope boundary is checked before the frame: a payload that
    // violates a ceiling is never answered with the frame's integrity failure.
    let mut cap_with_garbage = valid.clone();
    cap_with_garbage[4..8].copy_from_slice(&1_048_577u32.to_le_bytes());
    cap_with_garbage[9] ^= 0xff;
    assert_eq!(
        codec.decode_snapshot(&cap_with_garbage),
        Err(ProtocolError::FrameTooLarge),
        "a ceiling violation must not reach the decompressor"
    );

    // A frame that decompresses to fewer bytes than the envelope declares is
    // the length-incomplete condition, which stays truncated rather than the
    // integrity boundary.
    let mut over_declared = valid.clone();
    let declared = u32::from_le_bytes(over_declared[0..4].try_into().unwrap());
    over_declared[0..4].copy_from_slice(&(declared + 64).to_le_bytes());
    assert_eq!(
        codec.decode_snapshot(&over_declared),
        Err(ProtocolError::Truncated),
        "a declared decoded length the frame cannot back is truncated, not integrity"
    );

    // A payload shorter than the envelope header, and one trailing byte.
    for length in 0..8 {
        assert_eq!(
            codec.decode_snapshot(&valid[..length]),
            Err(ProtocolError::Truncated),
            "accepted a truncated envelope at {length} bytes"
        );
    }
    let mut trailing = valid.clone();
    trailing.push(0);
    assert_eq!(
        codec.decode_snapshot(&trailing),
        Err(ProtocolError::Truncated),
        "one trailing byte is the envelope's remaining-length boundary"
    );
}

#[test]
fn snapshot_expansion_beyond_the_decoded_ceiling_is_refused() {
    let mut codec = ProtocolCodec::new().expect("snapshot codec");
    // An expansion bomb: a frame whose content is one byte past the declared
    // ceiling. The envelope accepts the declared length, and the bounded
    // decompression refuses the frame instead of expanding past the ceiling.
    let bomb = vec![0u8; MAX_DECODED_SNAPSHOT + 1];
    let frame = zstd::bulk::compress(&bomb, 3).expect("compress test-only bomb");
    assert!(frame.len() <= MAX_COMPRESSED_SNAPSHOT);
    let mut payload = Vec::with_capacity(8 + frame.len());
    payload.extend_from_slice(&(MAX_DECODED_SNAPSHOT as u32).to_le_bytes());
    payload.extend_from_slice(&(frame.len() as u32).to_le_bytes());
    payload.extend_from_slice(&frame);
    assert_eq!(
        codec.decode_snapshot(&payload),
        Err(ProtocolError::Integrity),
        "the expansion bomb must be refused by the bounded decompression"
    );

    // The encode-side compressed ceiling is defensive rather than reachable:
    // a validated snapshot's logical payload is bounded far below 1 MiB and
    // its packed sections are highly compressible, so no legal record can
    // produce a frame above the ceiling. The envelope-decode side above is the
    // pin the plan names for this bound.
    let snapshot = mixed_snapshot();
    let payload = encode_with_codec(&mut codec, &snapshot);
    assert!(
        payload.len() - 8 <= MAX_COMPRESSED_SNAPSHOT,
        "the canonical frame must stay under the compressed ceiling"
    );
}

#[test]
fn snapshot_packet_id_is_pinned() {
    assert_eq!(mornlea_protocol::ChunkSnapshot::PACKET_ID, 0);
    assert_eq!(MAX_COMPRESSED_SNAPSHOT, 1 << 20);
    assert_eq!(MAX_DECODED_SNAPSHOT, 2 << 20);
}
