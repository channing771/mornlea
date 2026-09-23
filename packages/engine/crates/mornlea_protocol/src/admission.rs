//! Pure negotiation and login admission decisions.
//!
//! These functions are the policy half of the inbound handshake and login
//! path. The wire half lives in [`crate::client_hello`] and
//! [`crate::login_start`], which decode a structurally valid record and keep
//! the raw fields; this module turns those raw fields into the two rejections
//! the Go login driver publishes and into the checked values an accepted login
//! carries.
//!
//! Nothing here owns a session, a deadline, a socket or a subscription: the
//! later server decides when to call these functions and what to send. Keeping
//! transport and authority metadata out of this crate is what lets the same
//! decisions run in a test, in a server and in a client without a network.

use crate::client_hello::InboundHello;
use crate::login_start::{InboundLoginStart, LOGIN_VIEW_DISTANCE_MAX, LOGIN_VIEW_DISTANCE_MIN};
use mornlea_domain::{DisplayName, Identities, PlayerId, trim_pinned_whitespace};

/// The one handshake rejection this crate publishes.
///
/// A version mismatch is an admission outcome, not a decode failure: the peer
/// that sent the record has to learn which version this side runs, so the
/// mismatch carries the server version the record is measured against.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum HandshakeRejection {
    /// The hello names a protocol version this side does not run.
    VersionMismatch {
        /// The protocol version this side answers with.
        server_version: u32,
    },
}

/// The two login admission rejections, in the order the Go driver decides them.
///
/// Identity covers both the UUIDv4 rule and the canonical display-name rule,
/// because the Go driver answers both with `LoginInvalidIdentity`; a view
/// distance outside the published interval is a protocol violation of the
/// declared payload and is answered separately.
#[derive(Clone, Debug, Eq, PartialEq)]
pub enum LoginAdmissionError {
    /// The identity bytes are not UUIDv4, or the trimmed name is not a
    /// canonical display name.
    InvalidIdentity,
    /// The declared view distance is outside `2..=64`.
    ProtocolViolation,
}

/// The checked values an accepted login carries.
///
/// The display name is the canonical result of the pinned trim, and the view
/// distance is the declared value unchanged: clamping it to a server-side
/// maximum is an admission-point decision the protocol layer does not make.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct AdmittedLogin {
    player_id: PlayerId,
    display_name: DisplayName,
    view_distance: u8,
}

impl AdmittedLogin {
    /// The checked player identity this login runs under.
    pub fn player_id(&self) -> PlayerId {
        self.player_id
    }

    /// The canonical display name the raw name trims to.
    pub fn display_name(&self) -> &DisplayName {
        &self.display_name
    }

    /// The view distance exactly as the peer declared it.
    pub fn view_distance(&self) -> u8 {
        self.view_distance
    }
}

/// Decides whether one structurally decoded hello can be negotiated.
///
/// Every version other than the current one is the same answer: the peer
/// learns the version this side runs and the connection ends there. A decode
/// failure never reaches this function, so a structurally valid record cannot
/// be reported as a version mismatch.
pub fn validate_hello(hello: InboundHello) -> Result<(), HandshakeRejection> {
    if hello.protocol_version() != Identities::current().protocol {
        return Err(HandshakeRejection::VersionMismatch {
            server_version: Identities::current().protocol,
        });
    }
    Ok(())
}

/// Admits one structurally decoded login start.
///
/// The decision order is the Go login driver's order and is part of the
/// contract: the identity bytes first, the trimmed and canonical display name
/// second, the view distance third. A record that fails two rules therefore
/// reports the earlier one, which is what lets a client tell an unusable
/// identity from an unusable declared distance.
///
/// The name is trimmed by the domain's pinned whitespace set before the
/// canonical rule runs, so a raw name the Go driver trims into a canonical
/// name is admitted here with the same result instead of being rejected by a
/// raw byte bound applied too early.
pub fn admit_login(start: InboundLoginStart) -> Result<AdmittedLogin, LoginAdmissionError> {
    let player_id = PlayerId::try_from_bytes(*start.player_id())
        .map_err(|_| LoginAdmissionError::InvalidIdentity)?;
    let display_name =
        DisplayName::try_from_canonical(trim_pinned_whitespace(start.display_name()).to_owned())
            .map_err(|_| LoginAdmissionError::InvalidIdentity)?;
    let view_distance = start.view_distance();
    if !(LOGIN_VIEW_DISTANCE_MIN..=LOGIN_VIEW_DISTANCE_MAX).contains(&view_distance) {
        return Err(LoginAdmissionError::ProtocolViolation);
    }
    Ok(AdmittedLogin {
        player_id,
        display_name,
        view_distance,
    })
}
