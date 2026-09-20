//! Versioned framing, negotiation, and packet-family codecs.
//!
//! This crate may depend on `mornlea_domain` and must not depend on save
//! codecs, the numerical kernel, a graphical host, or an online authority.
//! Packet families are ported one inventory row at a time. Framing rejects
//! empty, oversized, truncated, and non-canonical length prefixes before
//! publishing a payload.

#![deny(unsafe_code)]

mod bytes;
mod client_hello;
mod disconnect;
mod error;
mod frame;
mod handshake_reject;
mod login_reject;
mod login_start;
mod login_success;
mod player_id;
mod server_hello;
mod varint;

pub use client_hello::ClientHello;
pub use disconnect::{
    DISCONNECT_INTERNAL_ERROR, DISCONNECT_PROTOCOL_VIOLATION, DISCONNECT_SERVER_SHUTDOWN,
    DISCONNECT_SLOW_CLIENT, DISCONNECT_TIMEOUT, Disconnect,
};
pub use error::ProtocolError;
pub use frame::{MAX_FRAME_BYTES, read_frame, write_frame};
pub use handshake_reject::{HANDSHAKE_VERSION_MISMATCH, HandshakeReject};
pub use login_reject::{
    LOGIN_ALREADY_ONLINE, LOGIN_INTERNAL_ERROR, LOGIN_INVALID_IDENTITY, LOGIN_PLAYER_DATA_CORRUPT,
    LOGIN_PROTOCOL_VIOLATION, LOGIN_SERVER_FULL, LOGIN_STORE_UNAVAILABLE, LoginReject,
};
pub use login_start::{LOGIN_VIEW_DISTANCE_MAX, LOGIN_VIEW_DISTANCE_MIN, LoginStart};
pub use login_success::LoginSuccess;
pub use player_id::PlayerId;
pub use server_hello::ServerHello;
pub use varint::{decode_uvarint, encode_uvarint};

/// Workspace crate identity consumed by the foundation registration tests.
pub const CRATE_NAME: &str = "mornlea_protocol";

/// Domain crate identity that protocol records share.
pub const DOMAIN_CRATE_NAME: &str = mornlea_domain::CRATE_NAME;
