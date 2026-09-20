//! Versioned save records and supported migration codecs.
//!
//! This crate may depend on `mornlea_domain` and must not depend on protocol
//! codecs, the numerical kernel, a graphical host, or an online authority.
//! Standalone entity families stay first-class save rows.

#![deny(unsafe_code)]

/// Workspace crate identity consumed by the foundation registration tests.
pub const CRATE_NAME: &str = "mornlea_storage";

/// Domain crate identity that save records share.
pub const DOMAIN_CRATE_NAME: &str = mornlea_domain::CRATE_NAME;
