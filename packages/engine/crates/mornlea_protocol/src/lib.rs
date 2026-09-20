//! Versioned framing, negotiation, and packet-family codecs.
//!
//! This crate may depend on `mornlea_domain` and must not depend on save
//! codecs, the numerical kernel, a graphical host, or an online authority.
//! Packet families are ported one inventory row at a time.

#![deny(unsafe_code)]

/// Workspace crate identity consumed by the foundation registration tests.
pub const CRATE_NAME: &str = "mornlea_protocol";

/// Domain crate identity that protocol records share.
pub const DOMAIN_CRATE_NAME: &str = mornlea_domain::CRATE_NAME;
