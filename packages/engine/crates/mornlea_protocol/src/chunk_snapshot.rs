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
//! The module keeps the Go decomposition: [`crate::codec::ProtocolCodec`]
//! encodes the whole Play payload through one owned zstd context and
//! [`ChunkSnapshot::decode`] decodes it,
//! [`ChunkSnapshot::decode_envelope`] plus [`SnapshotEnvelope::decompress`] are
//! the envelope and its zstd frame, and [`ChunkSnapshot::encode_logical_checked`] plus
//! [`ChunkSnapshot::decode_logical`] are the uncompressed payload. The
//! intermediate layers are public so contract tests can prove decode exactness
//! byte for byte and rejection before allocation without reaching into private
//! state. [`compress_logical`] remains the one-shot frame builder the contract
//! tests use to construct malformed payloads; production compression runs
//! through the owned context instead, never through a per-call context.

use crate::block::{BLOCKS_PER_SECTION, SECTIONS_PER_CHUNK, registered_block};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::varint::canonical_uvarint_length;
use mornlea_domain::{Dimension, DomainError, PalettedSection};

/// Compressed ceiling of one snapshot envelope, copied from the Go
/// `MaxCompressedSnapshot`. A larger declared frame is rejected before the
/// frame is touched.
pub const MAX_COMPRESSED_SNAPSHOT: usize = 1 << 20;

/// Decoded ceiling of one logical snapshot payload, copied from the Go
/// `MaxDecodedSnapshot`. It bounds both the declared length in the envelope
/// and the size of an encoded logical payload.
pub const MAX_DECODED_SNAPSHOT: usize = 2 << 20;

/// Window-log ceiling of the decoder context, derived from the decoded
/// ceiling: a frame declaring a larger window is refused before its window is
/// allocated, which is the Rust side of the Go decoder's memory limit.
const MAX_DECODED_WINDOW_LOG: u32 = MAX_DECODED_SNAPSHOT.trailing_zeros();

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
    ///
    /// A block number that is not registered is the `InvalidEnum` boundary the
    /// registered-block predicate owns. The two value-range boundaries — a
    /// packed slot naming an entry beyond the palette and a direct word
    /// carrying bits above its slot — report `InvalidRange`, matching the Go
    /// validator's own messages for the same conditions
    /// (`palette slot ... exceeds palette length`, `unused high bits`), which
    /// the corpus publishes as the `invalid-value` category.
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
                        return Err(ProtocolError::InvalidRange);
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
                        return Err(ProtocolError::InvalidRange);
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

/// Maps a domain section rejection into the protocol error the wire
/// validator publishes for the same record.
///
/// The domain lumps the palette size and duplicate-entry checks into one
/// error, so the conversion reports that error as `InvalidRange`, the same
/// carrier the empty and oversized cases use; the block, width, high-bit and
/// slot violations are `InvalidEnum`, matching the decode path.
fn section_error_to_wire(error: DomainError) -> ProtocolError {
    match error {
        DomainError::InvalidBlock
        | DomainError::InvalidSectionBits
        | DomainError::InvalidSectionHighBits
        | DomainError::InvalidSectionSlot => ProtocolError::InvalidEnum,
        _ => ProtocolError::InvalidRange,
    }
}

/// Moves one wire section container into the checked domain section.
///
/// The palette and the packed words move between the two representations
/// without expanding the 4096 cells into block IDs, without sorting the
/// palette and without recompressing the slots, so a conversion is lossless
/// and bounded. The wire `y` field is the section's array position and is not
/// part of the domain value; `ChunkSnapshot::validate` pins it to the column
/// order before a conversion runs.
impl TryFrom<SectionData> for PalettedSection {
    type Error = ProtocolError;

    fn try_from(section: SectionData) -> Result<Self, ProtocolError> {
        match section.storage {
            SectionStorage::Single => PalettedSection::single(section.single),
            SectionStorage::Indexed => PalettedSection::indexed(
                section.bits,
                section.palette.into_boxed_slice(),
                section.packed.into_boxed_slice(),
            ),
            SectionStorage::Direct => PalettedSection::direct(section.packed.into_boxed_slice()),
        }
        .map_err(section_error_to_wire)
    }
}

/// Moves one checked domain section back into its wire container.
///
/// The caller supplies the section index separately, because the domain
/// carries the column as a fixed array where the position is implicit while
/// the wire states each section's `y`. The palette order and the packed word
/// bits are copied exactly, so the round trip neither reorders nor
/// recompresses the representation.
impl TryFrom<(PalettedSection, i32)> for SectionData {
    type Error = ProtocolError;

    fn try_from((section, y): (PalettedSection, i32)) -> Result<Self, ProtocolError> {
        if !(0..SECTIONS_PER_CHUNK as i32).contains(&y) {
            return Err(ProtocolError::InvalidRange);
        }
        if let Some(block) = section.as_single() {
            return Ok(Self::single(y, block));
        }
        if let Some((bits, palette, words)) = section.as_indexed() {
            return Ok(Self::indexed(y, bits, palette.to_vec(), words.to_vec()));
        }
        // A domain section always carries exactly one of the three
        // representations, so this arm is unreachable; the error keeps the
        // conversion total for the compiler without inventing a failure mode.
        let words = section.as_direct().ok_or(ProtocolError::InvalidEnum)?;
        Ok(Self::direct(y, words.to_vec()))
    }
}

/// One decoded snapshot envelope: the declared logical length and the zstd
/// frame the envelope carries.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct SnapshotEnvelope {
    /// Length the envelope declares for the decoded logical payload.
    decoded_length: usize,
    /// The zstd frame, exactly the bytes the envelope declares.
    compressed: Vec<u8>,
}

impl SnapshotEnvelope {
    /// Returns the checked logical length declared by the envelope.
    pub fn decoded_length(&self) -> usize {
        self.decoded_length
    }

    /// Borrows the bounded zstd frame carried by the envelope.
    pub fn compressed(&self) -> &[u8] {
        &self.compressed
    }

    /// Decompresses the frame into a destination of exactly `decoded_length`
    /// bytes through a one-shot context, matching the Go `DecodeAll` call.
    ///
    /// A frame the zstd layer rejects reports [`ProtocolError::Integrity`]:
    /// the envelope's length checks already proved the frame bytes are present
    /// and correctly sized, so the compressed stream itself — its content
    /// checksum or an equivalent frame-level failure — is what refused the
    /// payload. A frame that decompresses to a different length than the
    /// envelope declares is the length-incomplete condition, which stays
    /// [`ProtocolError::Truncated`] and is never conflated with the integrity
    /// boundary.
    pub fn decompress(&self) -> Result<Vec<u8>, ProtocolError> {
        decompress_frame(&self.compressed, self.decoded_length, None)
    }

    /// Decompresses the frame through an owned decoder context, so a session's
    /// stream never constructs a context per frame. The error mapping is the
    /// one-shot path's: a rejected frame is the integrity boundary and a
    /// length mismatch is the truncated boundary.
    pub(crate) fn decompress_with(
        &self,
        decompressor: &mut zstd::bulk::Decompressor<'_>,
    ) -> Result<Vec<u8>, ProtocolError> {
        decompress_frame(&self.compressed, self.decoded_length, Some(decompressor))
    }
}

/// Decompresses one length-complete frame into a destination bounded by the
/// envelope's declared decoded length.
///
/// The one decision behind both decompression entry points: a frame the zstd
/// layer rejects is the compressed stream's own failure (`Integrity`), because
/// the envelope's length checks already passed; a frame that decompresses to a
/// different length than declared is the length-incomplete condition
/// (`Truncated`).
fn decompress_frame(
    compressed: &[u8],
    decoded_length: usize,
    decompressor: Option<&mut zstd::bulk::Decompressor<'_>>,
) -> Result<Vec<u8>, ProtocolError> {
    if decoded_length > MAX_DECODED_SNAPSHOT || compressed.len() > MAX_COMPRESSED_SNAPSHOT {
        return Err(ProtocolError::FrameTooLarge);
    }
    let decoded = match decompressor {
        Some(context) => context.decompress(compressed, decoded_length),
        None => zstd::bulk::decompress(compressed, decoded_length),
    }
    .map_err(|_| ProtocolError::Integrity)?;
    if decoded.len() != decoded_length {
        return Err(ProtocolError::Truncated);
    }
    Ok(decoded)
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

    /// Encodes the uncompressed logical payload, re-validating the current
    /// value first so a record mutated after construction is refused here
    /// instead of panicking inside the encoder.
    ///
    /// The checked logical size is compared with the decoded ceiling before
    /// any byte is reserved: a validated snapshot is bounded well below that
    /// ceiling, so the reservation cannot overflow and the only remaining
    /// failure is the primitive encoder's own.
    pub fn encode_logical_checked(&self) -> Result<Vec<u8>, ProtocolError> {
        self.validate()?;
        let size = self.logical_size();
        if size > MAX_DECODED_SNAPSHOT {
            return Err(ProtocolError::FrameTooLarge);
        }
        let mut encoder = ByteEncoder::with_capacity(size);
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
        encoder.finish()
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

/// Creates the one zstd compressor context the codec and the one-shot helper
/// share. The settings mirror the Go encoder: compression level 3 (its default)
/// with the content checksum enabled, and the exact logical length pledged per
/// call, so the frame carries its content size and the trailing xxhash-64 the
/// Go decoder also verifies.
pub(crate) fn new_compressor() -> Result<zstd::bulk::Compressor<'static>, ProtocolError> {
    let mut compressor =
        zstd::bulk::Compressor::new(COMPRESSION_LEVEL).map_err(|_| ProtocolError::Allocation)?;
    compressor
        .set_parameter(zstd::zstd_safe::CParameter::ChecksumFlag(true))
        .map_err(|_| ProtocolError::Allocation)?;
    Ok(compressor)
}

/// Creates the one zstd decompressor context the codec and the one-shot helper
/// share. The window limit is the decoded ceiling itself, mirroring the Go
/// decoder's 2 MiB memory limit, so a frame declaring an oversized window is
/// refused instead of allocating it.
pub(crate) fn new_decompressor() -> Result<zstd::bulk::Decompressor<'static>, ProtocolError> {
    let mut decompressor =
        zstd::bulk::Decompressor::new().map_err(|_| ProtocolError::Allocation)?;
    decompressor
        .set_parameter(zstd::zstd_safe::DParameter::WindowLogMax(
            MAX_DECODED_WINDOW_LOG,
        ))
        .map_err(|_| ProtocolError::Allocation)?;
    Ok(decompressor)
}

/// Compresses one logical payload into a single-frame zstd stream through the
/// given context.
///
/// The pledged source size is set per call because it is the payload's own
/// length, and the compressed scratch is reserved with `try_reserve`, so an
/// unreservable request reports the allocation boundary instead of aborting.
/// A context or compression failure never publishes partial output.
pub(crate) fn compress_frame(
    logical: &[u8],
    compressor: &mut zstd::bulk::Compressor<'_>,
) -> Result<Vec<u8>, ProtocolError> {
    let bound = compress_scratch_bound(logical.len())?;
    compressor
        .context_mut()
        .set_pledged_src_size(Some(logical.len() as u64))
        .map_err(|_| ProtocolError::Allocation)?;
    let mut compressed = Vec::new();
    compressed
        .try_reserve(bound)
        .map_err(|_| ProtocolError::Allocation)?;
    let written = compressor
        .compress_to_buffer(logical, &mut compressed)
        .map_err(|_| ProtocolError::Allocation)?;
    compressed.truncate(written);
    Ok(compressed)
}

/// Compresses one logical payload into a single-frame zstd stream with a fresh
/// context.
///
/// This is the one-shot frame builder the contract tests use to construct
/// malformed payloads from mutated logical bytes; production compression runs
/// through [`crate::codec::ProtocolCodec`]'s owned context instead. The settings
/// mirror the Go encoder, so the frame's magic, header descriptor, content size
/// and trailing checksum are the parts the two implementations share, while the
/// compressed block payload legitimately differs from the Go encoder's; see the
/// module documentation.
pub fn compress_logical(logical: &[u8]) -> Result<Vec<u8>, ProtocolError> {
    // Reject impossible frames before constructing a one-shot zstd context.
    compress_scratch_bound(logical.len())?;
    compress_frame(logical, &mut new_compressor()?)
}

/// Computes the worst-case scratch reservation before compression allocates.
fn compress_scratch_bound(logical_len: usize) -> Result<usize, ProtocolError> {
    if logical_len > MAX_DECODED_SNAPSHOT {
        return Err(ProtocolError::FrameTooLarge);
    }
    let bound = zstd::zstd_safe::compress_bound(logical_len);
    if bound > MAX_COMPRESSED_SNAPSHOT {
        return Err(ProtocolError::FrameTooLarge);
    }
    Ok(bound)
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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn forged_envelope_cannot_decompress_beyond_protocol_bounds() {
        let decoded_over = SnapshotEnvelope {
            decoded_length: MAX_DECODED_SNAPSHOT + 1,
            compressed: vec![0],
        };
        assert_eq!(decoded_over.decompress(), Err(ProtocolError::FrameTooLarge));

        let compressed_over = SnapshotEnvelope {
            decoded_length: 1,
            compressed: vec![0; MAX_COMPRESSED_SNAPSHOT + 1],
        };
        assert_eq!(
            compressed_over.decompress(),
            Err(ProtocolError::FrameTooLarge)
        );
    }

    #[test]
    fn one_shot_compression_rejects_oversized_requests() {
        assert_eq!(
            compress_logical(&vec![0; MAX_DECODED_SNAPSHOT + 1]),
            Err(ProtocolError::FrameTooLarge)
        );
        assert_eq!(
            compress_logical(&vec![0; MAX_COMPRESSED_SNAPSHOT]),
            Err(ProtocolError::FrameTooLarge)
        );
    }
}
