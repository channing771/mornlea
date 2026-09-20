//! Versioned framing, negotiation, and packet-family codecs.
//!
//! This crate may depend on `mornlea_domain` and must not depend on save
//! codecs, the numerical kernel, a graphical host, or an online authority.
//! Packet families are ported one inventory row at a time. Framing rejects
//! empty, oversized, truncated, and non-canonical length prefixes before
//! publishing a payload.

#![deny(unsafe_code)]

mod error;
mod frame;
mod varint;

pub use error::ProtocolError;
pub use frame::{MAX_FRAME_BYTES, read_frame, write_frame};
pub use varint::{decode_uvarint, encode_uvarint};

/// Workspace crate identity consumed by the foundation registration tests.
pub const CRATE_NAME: &str = "mornlea_protocol";

/// Domain crate identity that protocol records share.
pub const DOMAIN_CRATE_NAME: &str = mornlea_domain::CRATE_NAME;
