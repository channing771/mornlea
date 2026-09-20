//! Shared record-array and batch-header helpers.
//!
//! Several packet families carry the same shapes: a fixed-count array of
//! fixed-stride records, or a batch header of a server tick plus a record
//! count that is either a single byte or a canonical uvarint. Keeping one
//! implementation here means the count bounds, the exact-remaining-length
//! rejection, and the record ordering rule are decided once instead of being
//! copied per family and drifting apart.

use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Encodes a fixed-count record array in field order.
pub(crate) fn write_fixed<T, const N: usize>(
    encoder: &mut ByteEncoder,
    values: &[T; N],
    mut write: impl FnMut(&mut ByteEncoder, &T),
) {
    for value in values {
        write(encoder, value);
    }
}

/// Decodes a fixed-count record array. A truncated payload fails before any
/// record is published.
pub(crate) fn read_fixed<T, const N: usize>(
    decoder: &mut ByteDecoder<'_>,
    mut read: impl FnMut(&mut ByteDecoder<'_>) -> Result<T, ProtocolError>,
) -> Result<[T; N], ProtocolError> {
    let mut values = Vec::with_capacity(N);
    while values.len() < N {
        values.push(read(decoder)?);
    }
    values.try_into().map_err(|_| ProtocolError::Truncated)
}

/// Batch header of a server tick followed by a one-byte record count, used by
/// the hostile, passive, and projectile families.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) struct ByteCountBatch {
    pub server_tick: u64,
    pub count: u8,
}

impl ByteCountBatch {
    pub fn write(encoder: &mut ByteEncoder, server_tick: u64, count: u8) {
        encoder.u64(server_tick);
        encoder.u8(count);
    }

    /// Reads the header and enforces the record-count bounds.
    ///
    /// A zero count is rejected because an empty batch has no observable
    /// meaning, and a count above `max_records` overruns the fixed wire
    /// budget the Go side pins for the family.
    pub fn read(decoder: &mut ByteDecoder<'_>, max_records: u8) -> Result<Self, ProtocolError> {
        let server_tick = decoder.u64()?;
        let count = decoder.u8()?;
        if count < 1 || count > max_records {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self { server_tick, count })
    }

    /// Rejects a payload whose remaining length is not exactly `count`
    /// records of `stride` bytes. The check runs before any record is read
    /// so a short or padded batch never publishes partial records.
    pub fn require_records(
        &self,
        decoder: &ByteDecoder<'_>,
        stride: usize,
    ) -> Result<(), ProtocolError> {
        let want = usize::from(self.count) * stride;
        if decoder.remaining() != want {
            return Err(ProtocolError::Truncated);
        }
        Ok(())
    }
}

/// Batch header of a server tick followed by a canonical uvarint record
/// count, used by the companion and remote player families.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub(crate) struct UvarintCountBatch {
    pub server_tick: u64,
    pub count: u32,
}

impl UvarintCountBatch {
    pub fn write(encoder: &mut ByteEncoder, server_tick: u64, count: u32) {
        encoder.u64(server_tick);
        encoder.uvarint(count);
    }

    /// Reads the header and enforces the record-count bounds.
    pub fn read(decoder: &mut ByteDecoder<'_>, max_records: u32) -> Result<Self, ProtocolError> {
        let server_tick = decoder.u64()?;
        let count = decoder.uvarint()?;
        if count < 1 || count > max_records {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self { server_tick, count })
    }

    /// See `ByteCountBatch::require_records`.
    pub fn require_records(
        &self,
        decoder: &ByteDecoder<'_>,
        stride: usize,
    ) -> Result<(), ProtocolError> {
        let want = (self.count as usize).saturating_mul(stride);
        if decoder.remaining() != want {
            return Err(ProtocolError::Truncated);
        }
        Ok(())
    }
}

/// Reports whether the values are strictly increasing. Duplicate and
/// descending values are rejected by every sorted batch.
pub(crate) fn strictly_increasing<T: Ord>(values: &[T]) -> bool {
    values.windows(2).all(|pair| pair[0] < pair[1])
}

/// Reports whether the fixed-width identities are strictly increasing in
/// unsigned byte order, matching the Go `bytes.Compare` ordering.
pub(crate) fn strictly_increasing_ids(values: &[[u8; 16]]) -> bool {
    values.windows(2).all(|pair| pair[0] < pair[1])
}
