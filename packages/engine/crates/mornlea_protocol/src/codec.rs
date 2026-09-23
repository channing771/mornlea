//! One owned compression/decompression context for the compressed packet
//! family.
//!
//! `ChunkSnapshot` is the only wire family whose payload is compressed, so it
//! is the only family that needs a real encoder rather than a decoder-only
//! helper. [`ProtocolCodec`] owns that context pair: one zstd compressor and
//! one zstd decompressor, created together in [`ProtocolCodec::new`] and reused
//! by every snapshot call. No global or static context exists, no background
//! thread is started, and no call constructs a context of its own, so one
//! caller — one session — holds one context lifetime and the bounded scratch
//! stays with it.
//!
//! The encode path is the design §4 order for the one family whose payload
//! length is not precomputed: validate the record, produce the checked logical
//! payload, compress it through the owned context into scratch bounded by the
//! 2 MiB logical and 1 MiB compressed ceilings, check the compressed ceiling,
//! and only then test the caller's destination and copy the envelope plus the
//! frame once. A short destination reports
//! [`ProtocolError::OutputTooSmall`] with every caller byte unchanged, an
//! invalid value always wins over a short destination, and a compression or
//! reservation failure reports [`ProtocolError::Allocation`] without
//! publishing anything.
//!
//! The decode path checks the envelope and its declared lengths before the
//! frame is touched, decompresses through the owned decoder context, and
//! validates the logical section shape before publication. A frame the zstd
//! layer rejects reports [`ProtocolError::Integrity`], because the envelope's
//! length checks already proved the frame bytes are present and correctly
//! sized; a frame whose decompressed length disagrees with the envelope stays
//! [`ProtocolError::Truncated`].

use crate::chunk_snapshot::{
    ChunkSnapshot, MAX_COMPRESSED_SNAPSHOT, SNAPSHOT_ENVELOPE_LENGTH, compress_frame,
    new_compressor, new_decompressor,
};
use crate::error::ProtocolError;

/// One session's owned snapshot compression/decompression context.
///
/// The struct is not `Sync` by design: one codec belongs to one caller, and the
/// `&mut self` borrow on both operations is what serializes access to the
/// underlying contexts without an internal lock.
pub struct ProtocolCodec {
    compressor: zstd::bulk::Compressor<'static>,
    decompressor: zstd::bulk::Decompressor<'static>,
}

impl ProtocolCodec {
    /// Creates the context pair this codec owns for its lifetime.
    ///
    /// A context that cannot be created is a resource failure, reported as
    /// [`ProtocolError::Allocation`]; no partially initialized codec is
    /// published.
    pub fn new() -> Result<Self, ProtocolError> {
        Ok(Self {
            compressor: new_compressor()?,
            decompressor: new_decompressor()?,
        })
    }

    /// Decodes one Play chunk-snapshot payload through the owned decoder
    /// context.
    ///
    /// The envelope's declared lengths are checked before the frame is touched,
    /// the frame is decompressed through the owned context, and the logical
    /// section shape is validated before the snapshot is published, so a
    /// corrupt count or an expansion attempt never becomes a large allocation.
    pub fn decode_snapshot(&mut self, payload: &[u8]) -> Result<ChunkSnapshot, ProtocolError> {
        let envelope = ChunkSnapshot::decode_envelope(payload)?;
        let logical = envelope.decompress_with(&mut self.decompressor)?;
        ChunkSnapshot::decode_logical(&logical)
    }

    /// Encodes one Play chunk-snapshot payload into the caller's buffer and
    /// returns the bytes written.
    ///
    /// The record is re-validated first, so a snapshot mutated after
    /// construction is refused with its own boundary error before any scratch
    /// is reserved or a destination byte is touched. The compressed scratch is
    /// bounded by the compressed ceiling; a payload that would exceed it is
    /// refused with [`ProtocolError::FrameTooLarge`]. Only then is the
    /// destination tested: a short buffer reports
    /// [`ProtocolError::OutputTooSmall`] and leaves every byte unchanged, while
    /// a larger buffer is written only in `dst[..written]`.
    pub fn encode_snapshot_into(
        &mut self,
        packet: &ChunkSnapshot,
        dst: &mut [u8],
    ) -> Result<usize, ProtocolError> {
        let logical = packet.encode_logical_checked()?;
        let compressed = compress_frame(&logical, &mut self.compressor)?;
        if compressed.len() > MAX_COMPRESSED_SNAPSHOT {
            return Err(ProtocolError::FrameTooLarge);
        }
        let needed = SNAPSHOT_ENVELOPE_LENGTH + compressed.len();
        if dst.len() < needed {
            return Err(ProtocolError::OutputTooSmall {
                needed,
                available: dst.len(),
            });
        }
        dst[0..4].copy_from_slice(&(logical.len() as u32).to_le_bytes());
        dst[4..8].copy_from_slice(&(compressed.len() as u32).to_le_bytes());
        dst[8..needed].copy_from_slice(&compressed);
        Ok(needed)
    }
}
