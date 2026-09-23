use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};
use mornlea_domain::{DisplayName, trim_pinned_whitespace};

const DISPLAY_NAME_MAX_BYTES: usize = 128;
const DISPLAY_NAME_MAX_RUNES: usize = 32;

/// The ceiling the Go codec applies to every small packet payload.
///
/// The inbound decoder enforces it before any field is read, exactly as the Go
/// decoder refuses an oversized payload before it parses one, so an oversized
/// payload is a size refusal rather than a field failure. It lives beside the
/// login decoder because this node's inbound path is its first user; the packet
/// dispatcher applies the same ceiling to every small family instead of each
/// module restating the number.
pub const MAX_SMALL_PAYLOAD_BYTES: usize = 64 * 1024;

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

    /// Decodes one inbound login start structurally, keeping the raw fields.
    ///
    /// The inbound decoder applies the Go inbound decoder's bounds and nothing
    /// else: a canonical length prefix, valid UTF-8, the declared view-distance
    /// byte, and full consumption inside the 64 KiB small-payload ceiling. The
    /// identity rule, the canonical name rule and the view-distance interval are
    /// admission decisions, so a raw name longer than the canonical byte bound
    /// survives here to be trimmed by
    /// [`crate::admission::admit_login`], exactly as the Go login driver admits
    /// it.
    pub fn decode_inbound(payload: &[u8]) -> Result<InboundLoginStart, ProtocolError> {
        if payload.len() > MAX_SMALL_PAYLOAD_BYTES {
            return Err(ProtocolError::Allocation);
        }
        let mut decoder = ByteDecoder::new(payload);
        let player_id = decoder.bytes::<16>()?;
        let display_name = decoder.string(MAX_SMALL_PAYLOAD_BYTES, MAX_SMALL_PAYLOAD_BYTES)?;
        let view_distance = decoder.u8()?;
        decoder.done()?;
        Ok(InboundLoginStart {
            player_id,
            display_name,
            view_distance,
        })
    }
}

/// One structurally decoded login start, before any admission decision.
///
/// Every field keeps the peer's raw value: the 16 identity bytes, the display
/// name as written, and the declared view distance. Admission is the only
/// place those raw values become checked ones, so a rejection can name the
/// earliest rule the record breaks.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct InboundLoginStart {
    player_id: [u8; 16],
    display_name: String,
    view_distance: u8,
}

impl InboundLoginStart {
    /// The 16 identity bytes exactly as the peer wrote them.
    pub fn player_id(&self) -> &[u8; 16] {
        &self.player_id
    }

    /// The display name exactly as the peer wrote it, before any trim.
    pub fn display_name(&self) -> &str {
        &self.display_name
    }

    /// The view distance exactly as the peer declared it.
    pub fn view_distance(&self) -> u8 {
        self.view_distance
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
