use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};
use mornlea_domain::{DisplayName, trim_pinned_whitespace};

const DISPLAY_NAME_MAX_BYTES: usize = 128;
const DISPLAY_NAME_MAX_RUNES: usize = 32;

/// Closed interval copied from the Go `LoginViewDistanceMin` pin.
pub const LOGIN_VIEW_DISTANCE_MIN: u8 = 2;
/// Closed interval copied from the Go `LoginViewDistanceMax` pin.
pub const LOGIN_VIEW_DISTANCE_MAX: u8 = 64;

/// Login LoginStart payload. Non-UUIDv4 identities, invalid display names,
/// and view distances outside `2..=64` fail before the record is published.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct LoginStart {
    pub player_id: PlayerId,
    pub display_name: String,
    pub view_distance: u8,
}

impl LoginStart {
    pub const PACKET_ID: u32 = 0;

    pub fn new(
        player_id: PlayerId,
        display_name: impl Into<String>,
        view_distance: u8,
    ) -> Result<Self, ProtocolError> {
        let display_name = display_name.into();
        if !valid_display_name(&display_name) {
            return Err(ProtocolError::InvalidString);
        }
        if !(LOGIN_VIEW_DISTANCE_MIN..=LOGIN_VIEW_DISTANCE_MAX).contains(&view_distance) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self {
            player_id,
            display_name,
            view_distance,
        })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.bytes(&self.player_id.bytes());
        encoder.string(&self.display_name, DISPLAY_NAME_MAX_BYTES);
        encoder.u8(self.view_distance);
        encoder
            .finish()
            .expect("validated login start is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let player_id = player_id::read(&mut decoder)?;
        let display_name = decoder.string(DISPLAY_NAME_MAX_BYTES, DISPLAY_NAME_MAX_RUNES)?;
        let view_distance = decoder.u8()?;
        decoder.done()?;
        Self::new(player_id, display_name, view_distance)
    }
}

/// Reports whether the login display name is publishable.
///
/// The raw payload is length-bounded first, then trimmed by the domain's
/// pinned whitespace set and admitted by the domain's canonical display-name
/// rule, so the login path has one lexical rule shared with every other name
/// carrier instead of a local trim.
fn valid_display_name(name: &str) -> bool {
    if name.len() > DISPLAY_NAME_MAX_BYTES {
        return false;
    }
    DisplayName::try_from_canonical(trim_pinned_whitespace(name).to_owned()).is_ok()
}
