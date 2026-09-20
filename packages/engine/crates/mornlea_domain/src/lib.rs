//! Shared identifiers, value rules, and semantic input/event records.
//!
//! Production code in this crate must not depend on protocol codecs, save
//! codecs, the numerical kernel, a graphical host, or an online authority.
//! Family ports land behind failing `runtime_contract` cases.

#![deny(unsafe_code)]

/// Workspace crate identity consumed by the foundation registration tests.
pub const CRATE_NAME: &str = "mornlea_domain";
