//! `protocol.server.ChunkSnapshot`: the Play packet-ID 0 chunk payload.
//!
//! Ported from the Go `chunk_codec.go` snapshot path together with the
//! `protocol.SectionData`/`protocol.ChunkSnapshot` shapes and their `Validate`
//! rules. One payload is an eight-byte envelope — the declared decoded length,
//! then the declared compressed length — followed by a zstd frame carrying the
//! logical snapshot: the chunk identity and revision, a section count, and one
//! paletted container per section. This is the only packet family whose wire
//! payload is compressed, so it is also the only one that needs a real
//! encoder rather than a decoder-only helper.
//!
//! # Encoder output is intentionally not byte-identical to the Go encoder
//!
//! This was verified and ruled on; it is not an unresolved defect and must not
//! be "fixed". The Go payload is built by `github.com/klauspost/compress/zstd`,
//! a pure-Go zstd implementation whose compressed block payload differs from
//! the reference libzstd bound here at every compression level, even though the
//! frame header and the trailing xxhash-64 content checksum are byte-identical
//! and the total frame length can match. A standalone Go program re-encodes the
//! committed fixture exactly, so the divergence is exclusively a
//! Rust-versus-Go encoder difference, and no crate available here binds
//! klauspost.
//!
//! Cross-implementation compatibility is therefore defined at the logical/decode
//! level: zstd frames are self-describing, so the Go decoder reads what this
//! encoder writes and this decoder reads what the Go encoder wrote. Acceptance
//! for this family is semantic round-trip — exact decode of the committed
//! fixture, lossless encode/decode, round-trip through the fixture, and
//! rejection without implicit repair — never byte-identical re-encode. Do not
//! add an assertion that a frame produced here equals the committed fixture's
//! compressed bytes. The frame header and the trailing content checksum are
//! pinned separately because those two parts are identical across the two
//! implementations.
//!
//! # Layers
//!
//! The module keeps the Go decomposition: [`ChunkSnapshot::encode`] and
//! [`ChunkSnapshot::decode`] are the whole Play payload,
//! [`ChunkSnapshot::decode_envelope`] plus [`SnapshotEnvelope::decompress`] are
//! the envelope and its zstd frame, and [`ChunkSnapshot::encode_logical`] plus
//! [`ChunkSnapshot::decode_logical`] are the uncompressed payload. The
//! intermediate layers are public so contract tests can prove decode exactness
//! byte for byte and rejection before allocation without reaching into private
//! state.

use crate::block::{BLOCKS_PER_SECTION, SECTIONS_PER_CHUNK, registered_block};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::varint::canonical_uvarint_length;
use mornlea_domain::Dimension;

/// Compressed ceiling of one snapshot envelope, copied from the Go
/// `MaxCompressedSnapshot`. A larger declared frame is rejected before the
/// frame is touched.
pub const MAX_COMPRESSED_SNAPSHOT: usize = 1 << 20;

/// Decoded ceiling of one logical snapshot payload, copied from the Go
/// `MaxDecodedSnapshot`. It bounds both the declared length in the envelope
/// and the size of an encoded logical payload.
pub const MAX_DECODED_SNAPSHOT: usize = 2 << 20;

/// Fixed envelope header: the declared decoded length, then the declared
/// compressed length. Both are little-endian `u32` and neither includes the
/// header itself.
pub const SNAPSHOT_ENVELOPE_LENGTH: usize = 8;

/// zstd compression level mirroring the Go encoder's default, which is roughly
/// reference level 3. Only the level is mirrored: the emitted bytes are not.
const COMPRESSION_LEVEL: i32 = 3;

/// Section container kinds. The values are wire-stable and the set is closed:
/// a value this version does not define is rejected rather than repaired.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum SectionStorage {
    /// One block ID fills the whole section.
    Single,
    /// A palette of block IDs plus bit-packed palette slots.
    Indexed,
    /// Bit-packed block IDs with no palette.
    Direct,
}

impl SectionStorage {
    /// Reads a section container kind from its wire byte.
    pub fn from_wire(value: u8) -> Result<Self, ProtocolError> {
        match value {
            0 => Ok(Self::Single),
            1 => Ok(Self::Indexed),
            2 => Ok(Self::Direct),
            _ => Err(ProtocolError::InvalidEnum),
        }
    }

    /// The wire byte of a section container kind.
    pub fn wire(self) -> u8 {
        match self {
            Self::Single => 0,
            Self::Indexed => 1,
            Self::Direct => 2,
        }
    }
}

/// One paletted section container of a chunk column.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SectionData {
    /// Section index inside the chunk column, `0..SECTIONS_PER_CHUNK`.
    pub y: i32,
    /// Container kind, which decides which of the remaining fields are live.
    pub storage: SectionStorage,
    /// The single block ID of a `Single` section; zero otherwise.
    pub single: u16,
    /// Bits per packed slot. Zero for `Single`, 4 or 8 for `Indexed`, and 15
    /// for `Direct`.
    pub bits: u8,
    /// Palette of a `Indexed` section, indexed by the packed slots. Empty for
    /// the other kinds.
    pub palette: Vec<u16>,
    /// Packed slots of an `Indexed` or `Direct` section. Empty for `Single`.
    pub packed: Vec<u64>,
}

impl SectionData {
    /// Builds a validated single-block section.
    pub fn single(y: i32, block: u16) -> Self {
        Self {
            y,
            storage: SectionStorage::Single,
            single: block,
            bits: 0,
            palette: Vec::new(),
            packed: Vec::new(),
        }
    }

    /// Builds a validated paletted section. The caller owns the bits-per-slot
    /// choice, which the validation gate then pins to 4 or 8.
    pub fn indexed(y: i32, bits: u8, palette: Vec<u16>, packed: Vec<u64>) -> Self {
        Self {
            y,
            storage: SectionStorage::Indexed,
            single: 0,
            bits,
            palette,
            packed,
        }
    }

    /// Builds a validated direct section, which is always 15 bits per slot.
    pub fn direct(y: i32, packed: Vec<u64>) -> Self {
        Self {
            y,
            storage: SectionStorage::Direct,
            single: 0,
            bits: 15,
            palette: Vec::new(),
            packed,
        }
    }

    /// Byte length of the section's compressed payload, the part the Go
    /// `PayloadBytes` counts towards the logical snapshot size.
    pub fn payload_bytes(&self) -> usize {
        if self.storage == SectionStorage::Single {
            2
        } else {
            self.palette.len() * 2 + self.packed.len() * 8
        }
    }

    /// The single validation gate shared by `ChunkSnapshot::new` and
    /// `ChunkSnapshot::decode_logical`.
    ///
    /// A section must not carry a field that does not belong to its container
    /// kind, every packed slot must resolve inside the palette or the
    /// registered block range, and a direct word must not carry bits above its
    /// 15-bit slot. Rejecting the combination instead of ignoring it keeps a
    /// partially described section from being silently reinterpreted.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        if self.y < 0 || self.y >= SECTIONS_PER_CHUNK as i32 {
            return Err(ProtocolError::InvalidRange);
        }
        match self.storage {
            SectionStorage::Single => {
                if self.bits != 0 || !self.palette.is_empty() || !self.packed.is_empty() {
                    return Err(ProtocolError::InvalidRange);
                }
                if !registered_block(self.single) {
                    return Err(ProtocolError::InvalidEnum);
                }
            }
            SectionStorage::Indexed => {
                if self.bits != 4 && self.bits != 8 {
                    return Err(ProtocolError::InvalidEnum);
                }
                if self.single != 0 {
                    return Err(ProtocolError::InvalidRange);
                }
                if self.palette.is_empty() || self.palette.len() > 1 << self.bits {
                    return Err(ProtocolError::InvalidRange);
                }
                if self.packed.len() != section_words(self.bits) {
                    return Err(ProtocolError::InvalidRange);
                }
                let mut seen = [false; BLOCKS_PER_SECTION];
                for id in &self.palette {
                    if !registered_block(*id) || seen[*id as usize] {
                        return Err(ProtocolError::InvalidEnum);
                    }
                    seen[*id as usize] = true;
                }
                for index in 0..BLOCKS_PER_SECTION {
                    if read_section_packed(&self.packed, self.bits, index) as usize
                        >= self.palette.len()
                    {
                        return Err(ProtocolError::InvalidEnum);
                    }
                }
            }
            SectionStorage::Direct => {
                if self.bits != 15 || self.single != 0 || !self.palette.is_empty() {
                    return Err(ProtocolError::InvalidEnum);
                }
                if self.packed.len() != section_words(15) {
                    return Err(ProtocolError::InvalidRange);
                }
                for word in &self.packed {
                    if word >> 60 != 0 {
                        return Err(ProtocolError::InvalidEnum);
                    }
                }
                for index in 0..BLOCKS_PER_SECTION {
                    let id = read_section_packed(&self.packed, self.bits, index) as u16;
                    if !registered_block(id) {
                        return Err(ProtocolError::InvalidEnum);
                    }
                }
            }
        }
        Ok(())
    }
}

/// One decoded snapshot envelope: the declared logical length and the zstd
/// frame the envelope carries.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SnapshotEnvelope {
    /// Length the envelope declares for the decoded logical payload.
    pub decoded_length: usize,
    /// The zstd frame, exactly the bytes the envelope declares.
    pub compressed: Vec<u8>,
}

impl SnapshotEnvelope {
    /// Decompresses the frame into a destination of exactly `decoded_length`
    /// bytes, matching the Go `DecodeAll` call. The frame's own content
    /// checksum is verified, so a truncated or altered frame is rejected here,
    /// and a frame whose content does not fit the declared length is rejected
    /// rather than decompressed into a larger buffer.
    pub fn decompress(&self) -> Result<Vec<u8>, ProtocolError> {
        let decoded = zstd::bulk::decompress(&self.compressed, self.decoded_length)
            .map_err(|_| ProtocolError::Truncated)?;
        if decoded.len() != self.decoded_length {
            return Err(ProtocolError::Truncated);
        }
        Ok(decoded)
    }
}

/// Play ChunkSnapshot payload: one chunk column of the authoritative world.
///
/// The revision is the authority's chunk revision and must be non-zero, the
/// sections are the full ordered column of `SECTIONS_PER_CHUNK` containers,
/// and only the two playable dimensions are legal.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ChunkSnapshot {
    pub dimension: Dimension,
    pub chunk_x: i32,
    pub chunk_z: i32,
    pub revision: u64,
    pub sections: Vec<SectionData>,
}

impl ChunkSnapshot {
    pub const PACKET_ID: u32 = 0;

    /// Builds a validated chunk snapshot.
    pub fn new(
        dimension: Dimension,
        chunk_x: i32,
        chunk_z: i32,
        revision: u64,
        sections: Vec<SectionData>,
    ) -> Result<Self, ProtocolError> {
        let snapshot = Self {
            dimension,
            chunk_x,
            chunk_z,
            revision,
            sections,
        };
        snapshot.validate()?;
        Ok(snapshot)
    }

    /// The single validation gate shared by `new` and `decode`.
    ///
    /// The section list is the whole column in order: a missing or reordered
    /// section would leave the mirror with an undefined vertical span, so the
    /// count and the per-index Y are both checked here rather than being
    /// repaired by the decoder.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        if self.revision == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        if self.sections.len() != SECTIONS_PER_CHUNK {
            return Err(ProtocolError::InvalidRange);
        }
        for (index, section) in self.sections.iter().enumerate() {
            if section.y != index as i32 {
                return Err(ProtocolError::InvalidRange);
            }
            section.validate()?;
        }
        Ok(())
    }

    /// Byte length of the logical payload this snapshot encodes to. The value
    /// is the allocation size the encoder reserves, so it must match the wire
    /// exactly rather than being an estimate.
    pub fn logical_size(&self) -> usize {
        let mut size = 20 + canonical_uvarint_length(self.sections.len() as u32);
        for section in &self.sections {
            size += 2 + section.payload_bytes();
            match section.storage {
                SectionStorage::Indexed => {
                    size += 1
                        + canonical_uvarint_length(section.palette.len() as u32)
                        + canonical_uvarint_length(section.packed.len() as u32);
                }
                SectionStorage::Direct => {
                    size += 1 + canonical_uvarint_length(section.packed.len() as u32);
                }
                SectionStorage::Single => {}
            }
        }
        size
    }

    /// Encodes the whole Play payload: envelope header plus zstd frame.
    pub fn encode(&self) -> Vec<u8> {
        let logical = self.encode_logical();
        let compressed = compress_logical(&logical).expect("logical snapshot is compressible");
        let mut payload = Vec::with_capacity(SNAPSHOT_ENVELOPE_LENGTH + compressed.len());
        payload.extend_from_slice(&(logical.len() as u32).to_le_bytes());
        payload.extend_from_slice(&(compressed.len() as u32).to_le_bytes());
        payload.extend_from_slice(&compressed);
        payload
    }

    /// Decodes the whole Play payload.
    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let logical = Self::decode_envelope(payload)?.decompress()?;
        Self::decode_logical(&logical)
    }

    /// Decodes only the envelope: the two declared lengths and the zstd frame.
    ///
    /// Every bound is checked before the frame is touched, so an oversized or
    /// mismatched envelope never reaches the decompressor.
    pub fn decode_envelope(payload: &[u8]) -> Result<SnapshotEnvelope, ProtocolError> {
        if payload.len() < SNAPSHOT_ENVELOPE_LENGTH {
            return Err(ProtocolError::Truncated);
        }
        let mut decoder = ByteDecoder::new(payload);
        let decoded_length = decoder.u32()? as usize;
        if decoded_length > MAX_DECODED_SNAPSHOT {
            return Err(ProtocolError::FrameTooLarge);
        }
        let compressed_length = decoder.u32()? as usize;
        if compressed_length > MAX_COMPRESSED_SNAPSHOT {
            return Err(ProtocolError::FrameTooLarge);
        }
        // The declared compressed length must describe the rest of the payload
        // exactly: a short payload is truncated and a long one carries bytes
        // outside the envelope.
        if decoder.remaining() != compressed_length {
            return Err(ProtocolError::Truncated);
        }
        let compressed = decoder.take(compressed_length)?.to_vec();
        Ok(SnapshotEnvelope {
            decoded_length,
            compressed,
        })
    }

    /// Encodes the uncompressed logical payload.
    pub fn encode_logical(&self) -> Vec<u8> {
        self.validate()
            .expect("validated chunk snapshot is encodable");
        assert!(
            self.logical_size() <= MAX_DECODED_SNAPSHOT,
            "validated chunk snapshot exceeds the decoded ceiling"
        );
        let mut encoder = ByteEncoder::with_capacity(self.logical_size());
        encoder.i32(i32::from(self.dimension.get()));
        encoder.i32(self.chunk_x);
        encoder.i32(self.chunk_z);
        encoder.u64(self.revision);
        encoder.uvarint(self.sections.len() as u32);
        for section in &self.sections {
            encoder.u8(section.y as u8);
            encoder.u8(section.storage.wire());
            match section.storage {
                SectionStorage::Single => encoder.u16(section.single),
                SectionStorage::Indexed => {
                    encoder.u8(section.bits);
                    encoder.uvarint(section.palette.len() as u32);
                    for id in &section.palette {
                        encoder.u16(*id);
                    }
                    encoder.uvarint(section.packed.len() as u32);
                    for word in &section.packed {
                        encoder.u64(*word);
                    }
                }
                SectionStorage::Direct => {
                    encoder.u8(section.bits);
                    encoder.uvarint(section.packed.len() as u32);
                    for word in &section.packed {
                        encoder.u64(*word);
                    }
                }
            }
        }
        encoder
            .finish()
            .expect("validated chunk snapshot is encodable")
    }

    /// Decodes the uncompressed logical payload.
    ///
    /// Every declared count is checked against the bytes that remain before
    /// the matching buffer is allocated, so a corrupt count cannot turn into a
    /// large allocation.
    pub fn decode_logical(data: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(data);
        let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
            .map_err(|_| ProtocolError::InvalidEnum)?;
        let chunk_x = decoder.i32()?;
        let chunk_z = decoder.i32()?;
        let revision = decoder.u64()?;
        let count = decoder.uvarint()?;
        if count as usize != SECTIONS_PER_CHUNK {
            return Err(ProtocolError::InvalidRange);
        }
        let mut sections = Vec::with_capacity(SECTIONS_PER_CHUNK);
        for index in 0..SECTIONS_PER_CHUNK {
            sections.push(decode_logical_section(&mut decoder, index)?);
        }
        decoder.done()?;
        Self::new(dimension, chunk_x, chunk_z, revision, sections)
    }
}

/// Compresses one logical payload into a single-frame zstd stream.
///
/// The settings mirror the Go encoder: one worker and the content checksum
/// enabled, with the exact logical length pledged so the frame carries its
/// content size. The compressed block payload legitimately differs from the Go
/// encoder's; see the module documentation.
pub fn compress_logical(logical: &[u8]) -> Result<Vec<u8>, ProtocolError> {
    let mut encoder = zstd::stream::write::Encoder::new(Vec::new(), COMPRESSION_LEVEL)
        .map_err(|_| ProtocolError::InvalidRange)?;
    encoder
        .set_pledged_src_size(Some(logical.len() as u64))
        .map_err(|_| ProtocolError::InvalidRange)?;
    encoder
        .include_checksum(true)
        .map_err(|_| ProtocolError::InvalidRange)?;
    encoder
        .include_contentsize(true)
        .map_err(|_| ProtocolError::InvalidRange)?;
    std::io::Write::write_all(&mut encoder, logical).map_err(|_| ProtocolError::Truncated)?;
    encoder.finish().map_err(|_| ProtocolError::Truncated)
}

/// Rejects a declared field count the remaining logical payload cannot back,
/// before the matching buffer is allocated.
fn require_remaining(decoder: &ByteDecoder<'_>, need: u64) -> Result<(), ProtocolError> {
    if need > decoder.remaining() as u64 {
        return Err(ProtocolError::Truncated);
    }
    Ok(())
}

/// Number of packed words a section of `bits` per slot needs, derived exactly
/// as the Go `SectionWords` does so both sides allocate the same stride.
fn section_words(bits: u8) -> usize {
    let per_word = 64 / bits as usize;
    (BLOCKS_PER_SECTION + per_word - 1) / per_word
}

/// Reads the `index`-th packed slot of a section, using the same bit layout as
/// the Go `ReadSectionPacked`.
fn read_section_packed(packed: &[u64], bits: u8, index: usize) -> u32 {
    let per_word = 64 / bits as usize;
    let shift = (index % per_word) * bits as usize;
    ((packed[index / per_word] >> shift) & ((1u64 << bits) - 1)) as u32
}

/// Decodes one section container and validates it in place.
fn decode_logical_section(
    decoder: &mut ByteDecoder<'_>,
    index: usize,
) -> Result<SectionData, ProtocolError> {
    let y = decoder.u8()?;
    if y as usize != index {
        return Err(ProtocolError::InvalidRange);
    }
    let storage = SectionStorage::from_wire(decoder.u8()?)?;
    let mut section = SectionData {
        y: i32::from(y),
        storage,
        single: 0,
        bits: 0,
        palette: Vec::new(),
        packed: Vec::new(),
    };
    match storage {
        SectionStorage::Single => {
            section.single = decoder.u16()?;
        }
        SectionStorage::Indexed => {
            section.bits = decoder.u8()?;
            if section.bits != 4 && section.bits != 8 {
                return Err(ProtocolError::InvalidEnum);
            }
            let palette_count = decoder.uvarint()?;
            if palette_count == 0 || palette_count > 1 << section.bits {
                return Err(ProtocolError::InvalidRange);
            }
            // One byte for the word count follows the palette, so the bound
            // covers both before either buffer exists.
            require_remaining(decoder, u64::from(palette_count) * 2 + 1)?;
            let mut seen = [false; BLOCKS_PER_SECTION];
            let mut palette = Vec::with_capacity(palette_count as usize);
            for _ in 0..palette_count {
                let id = decoder.u16()?;
                if !registered_block(id) || seen[id as usize] {
                    return Err(ProtocolError::InvalidEnum);
                }
                seen[id as usize] = true;
                palette.push(id);
            }
            let word_count = decoder.uvarint()?;
            if word_count as usize != section_words(section.bits) {
                return Err(ProtocolError::InvalidRange);
            }
            require_remaining(decoder, u64::from(word_count) * 8)?;
            section.palette = palette;
            section.packed = read_packed_words(decoder, word_count as usize)?;
        }
        SectionStorage::Direct => {
            section.bits = decoder.u8()?;
            if section.bits != 15 {
                return Err(ProtocolError::InvalidEnum);
            }
            let word_count = decoder.uvarint()?;
            if word_count as usize != section_words(15) {
                return Err(ProtocolError::InvalidRange);
            }
            require_remaining(decoder, u64::from(word_count) * 8)?;
            section.packed = read_packed_words(decoder, word_count as usize)?;
        }
    }
    section.validate()?;
    Ok(section)
}

/// Reads the section's packed words after the caller has proved they fit.
fn read_packed_words(
    decoder: &mut ByteDecoder<'_>,
    count: usize,
) -> Result<Vec<u64>, ProtocolError> {
    let mut packed = Vec::with_capacity(count);
    for _ in 0..count {
        packed.push(decoder.u64()?);
    }
    Ok(packed)
}
