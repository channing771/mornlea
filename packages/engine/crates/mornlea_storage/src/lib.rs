//! Versioned save records and supported migration codecs.
//!
//! This crate may depend on `mornlea_domain` and must not depend on protocol
//! codecs, the numerical kernel, a graphical host, or an online authority.
//! Standalone entity families stay first-class save rows.

#![deny(unsafe_code)]

mod bytes;
mod crc32c;
mod error;
mod passive;

pub use crc32c::{crc32c, crc32c_join};
pub use error::{StorageError, StorageResult};
pub use passive::{
    CURRENT_SCHEMA as PASSIVE_CURRENT_SCHEMA, ENVELOPE_VERSION as PASSIVE_ENVELOPE_VERSION,
    MAX_FILE_LENGTH as PASSIVE_MAX_FILE_LENGTH, MAX_PASSIVE_MOBS, PassiveMob, PassiveMobs,
    PassiveMobsSave, decode as decode_passive_mobs, encode as encode_passive_mobs,
};

/// Workspace crate identity consumed by the foundation registration tests.
pub const CRATE_NAME: &str = "mornlea_storage";

/// Domain crate identity that save records share.
pub const DOMAIN_CRATE_NAME: &str = mornlea_domain::CRATE_NAME;
