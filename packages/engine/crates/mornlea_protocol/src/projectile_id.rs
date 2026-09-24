//! The checked projectile identity the three projectile families share.
//!
//! `ProjectileId` is the domain's nonzero `u64` newtype, re-exported here
//! beside the wire edge the spawn, state and despawn families all use: the
//! fixed eight-byte read and write. Zero is the absent form in every entity
//! family, so it is refused where the identity is read and cannot be
//! constructed at all on the outbound surface, which is the boundary the Go
//! validator answers with its `network: projectile spawn ID is zero`,
//! `network: projectile state ID is zero` and `network: projectile despawn %d
//! ID is zero` messages. No other identity rule exists on the wire, exactly as
//! the Go rule checks only nonzero, and the batch order compares the typed
//! value rather than a reinterpreted byte string.
//!
//! The shim mirrors `src/hostile_id.rs` and `src/passive_id.rs` rather than
//! generalizing them: the three identities are distinct domain newtypes over
//! the same bits, so a shared generic module would erase the type distinction
//! the domain keeps.

use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use mornlea_domain::DomainError;
pub use mornlea_domain::ProjectileId;

/// Fixed encoded length of one projectile identity: a little-endian `u64`.
pub const PROJECTILE_ID_WIRE_BYTES: usize = 8;

/// Reads one eight-byte identity and runs the domain rule.
///
/// The zero rejection is the identity boundary the Go decoder names, so the
/// protocol error vocabulary keeps `InvalidIdentity` for it rather than
/// answering a zero projectile as a range violation.
pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<ProjectileId, ProtocolError> {
    let id = decoder.u64()?;
    ProjectileId::try_new(id).map_err(|error| match error {
        DomainError::InvalidIdentity => ProtocolError::InvalidIdentity,
        _ => ProtocolError::InvalidRange,
    })
}

/// Publishes one identity into a caller-owned publication window.
///
/// The bytes are the fixed wire field order, so the caller-owned window cannot
/// drift from the decoded form.
pub(crate) fn write_into(id: ProjectileId, writer: &mut SliceWriter<'_>) {
    writer.u64(id.get());
}
