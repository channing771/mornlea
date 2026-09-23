//! The checked hostile identity the three hostile families share.
//!
//! `HostileId` is the domain's nonzero `u64` newtype, re-exported here beside
//! the wire edge the spawn, state and despawn families all use: the fixed
//! eight-byte read and write. Zero is the absent form in every entity family,
//! so it is refused where the identity is read and cannot be constructed at
//! all on the outbound surface, which is the boundary the Go validator answers
//! with its `network: hostile spawn ID is zero`, `network: hostile state ID is
//! zero` and `network: hostile despawn %d ID is zero` messages. No other
//! identity rule exists on the wire, exactly as the Go rule checks only
//! nonzero, and the batch order compares the typed value rather than a
//! reinterpreted byte string.

use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use mornlea_domain::DomainError;
pub use mornlea_domain::HostileId;

/// Fixed encoded length of one hostile identity: a little-endian `u64`.
pub const HOSTILE_ID_WIRE_BYTES: usize = 8;

/// Reads one eight-byte identity and runs the domain rule.
///
/// The zero rejection is the identity boundary the Go decoder names, so the
/// protocol error vocabulary keeps `InvalidIdentity` for it rather than
/// answering a zero hostile as a range violation.
pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<HostileId, ProtocolError> {
    let id = decoder.u64()?;
    HostileId::try_new(id).map_err(|error| match error {
        DomainError::InvalidIdentity => ProtocolError::InvalidIdentity,
        _ => ProtocolError::InvalidRange,
    })
}

/// Publishes one identity into a caller-owned publication window.
///
/// The bytes are the fixed wire field order, so the caller-owned window cannot
/// drift from the decoded form.
pub(crate) fn write_into(id: HostileId, writer: &mut SliceWriter<'_>) {
    writer.u64(id.get());
}
