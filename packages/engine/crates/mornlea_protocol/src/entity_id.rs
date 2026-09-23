//! Companion identity and the wire name predicates the companion-carrying
//! families share.
//!
//! The checked companion identity is the domain value, re-exported here so
//! the packet families and the domain agree on one rule. The companion name
//! and the plain display name are admitted by the domain's canonical text
//! constructors, which own the byte, rune, whitespace and control bounds;
//! this module only routes a wire `&str` through them so a family never
//! restates the rule.
//!
//! The absent companion identity a never-addressed chat event carries is a
//! wire-only raw form: the domain publishes no zero companion identity, so
//! the chat event keeps the raw 16 bytes and the checked identity is required
//! wherever a companion is actually named.

use crate::bytes::ByteDecoder;
use crate::error::ProtocolError;
pub use mornlea_domain::CompanionId;
use mornlea_domain::{CompanionName, DisplayName};

/// Reads one checked companion identity. The absent zero form fails here, so
/// a spawn, despawn or state record can never name it.
pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<CompanionId, ProtocolError> {
    CompanionId::try_from_bytes(decoder.bytes()?).map_err(|_| ProtocolError::InvalidIdentity)
}

/// Reports whether a display name is in canonical form for the wire: the Go
/// `core.NormalizeDisplayName` result compared against its own input, so a
/// name the authority would have to trim is rejected rather than normalized
/// here. Admission trims first through the domain helper and then asks this
/// rule.
pub fn valid_display_name(name: &str) -> bool {
    DisplayName::try_from_canonical(name.to_owned()).is_ok()
}

/// Reports whether a companion name is publishable. Companion names carry
/// the display-name rule and additionally reject Unicode whitespace, so a
/// name can never contain an embedded space.
pub fn valid_companion_name(name: &str) -> bool {
    CompanionName::try_from_canonical(name.to_owned()).is_ok()
}
