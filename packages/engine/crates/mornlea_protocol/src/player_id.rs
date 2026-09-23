//! The checked player identity, owned by `mornlea_domain`.
//!
//! A player identity is the same UUIDv4 rule everywhere it appears, so the
//! rule lives once in the domain and this module is only the wire edge: it
//! re-exports the domain value and reads one through the decoder with the
//! protocol error mapping. Keeping a second checked constructor here would
//! let the two rules drift apart.

use crate::bytes::ByteDecoder;
use crate::error::ProtocolError;
pub use mornlea_domain::PlayerId;

/// Reads one 16-byte identity and runs the domain rule. The wire failure is
/// the protocol's identity rejection; the domain's own error stays inside
/// the domain value type.
pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<PlayerId, ProtocolError> {
    PlayerId::try_from_bytes(decoder.bytes()?).map_err(|_| ProtocolError::InvalidIdentity)
}
