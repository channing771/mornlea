//! Companion identity and the name rule the companion-carrying families
//! share.
//!
//! A companion identity is the same frozen 16-byte UUIDv4 value as a player
//! identity, but it is a distinct wire concept: companion records are sorted
//! by their own ID space and the despawn message names a companion, not a
//! player. Keeping the type separate lets each family name what it carries
//! while the validation rule stays in one place.

use crate::error::ProtocolError;

/// Stable UUIDv4 companion identity. Zero and non-v4 values fail before the
/// identifier is published.
#[derive(Clone, Copy, Debug, Eq, Ord, PartialEq, PartialOrd)]
pub struct CompanionId([u8; 16]);

impl CompanionId {
    pub fn new(bytes: [u8; 16]) -> Result<Self, ProtocolError> {
        if bytes == [0; 16] || bytes[6] >> 4 != 4 || bytes[8] & 0xc0 != 0x80 {
            return Err(ProtocolError::InvalidIdentity);
        }
        Ok(Self(bytes))
    }

    pub fn bytes(self) -> [u8; 16] {
        self.0
    }
}

/// Reports whether a display name is in canonical form for the wire:
/// 1..=32 runes, at most 128 bytes, valid UTF-8, no control characters, and
/// no surrounding whitespace. The rule matches the Go
/// `core.NormalizeDisplayName` result compared against its own input, so a
/// name the authority would have to trim is rejected rather than normalized.
pub fn valid_display_name(name: &str) -> bool {
    const MAX_BYTES: usize = 128;
    const MAX_RUNES: usize = 32;
    if name.len() > MAX_BYTES || name.trim() != name {
        return false;
    }
    let runes = name.chars().count();
    (1..=MAX_RUNES).contains(&runes) && !name.chars().any(char::is_control)
}

/// Reports whether a companion name is publishable. Companion names carry
/// the display-name rule and additionally reject Unicode whitespace, so a
/// name can never contain an embedded space.
pub fn valid_companion_name(name: &str) -> bool {
    valid_display_name(name) && !name.chars().any(char::is_whitespace)
}
