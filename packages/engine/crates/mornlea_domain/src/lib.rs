//! Shared identifiers, value rules, and semantic input/event records.
//!
//! Production code in this crate must not depend on protocol codecs, save
//! codecs, the numerical kernel, a graphical host, or an online authority.
//! Incomplete identity, unknown family IDs, out-of-range values, and
//! non-finite rotations fail before a record is published.

#![deny(unsafe_code)]

mod event;
mod identity;
mod input;
mod text;
mod values;

pub use event::{FAMILY_EVENT, FAMILY_INPUT, Observation, order_observations};
pub use identity::{
    CompanionId, DomainError, HostileId, Identities, PassiveId, PlayerId, ProjectileId,
    ReplayIdentity,
};
pub use input::{PlaceBlock, PlayerInput, SelectHotbar, SemanticInput, order_inputs};
pub use text::{CommandText, CompanionName, DisplayName, SpeechText};
pub use values::{Dimension, FiniteVec3, HotbarSlot, LookAngles};

/// Workspace crate identity consumed by the foundation registration tests.
pub const CRATE_NAME: &str = "mornlea_domain";
