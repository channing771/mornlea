use crate::bytes::{ByteDecoder, ByteEncoder, SliceWriter};
use crate::error::ProtocolError;
use crate::player_id::{self, PlayerId};
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;
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
///
/// The record's wire form is exactly the bytes the decoder admitted, and
/// re-publishing it writes them back verbatim. That is why the identity rule,
/// the canonical display-name rule and the view-distance interval stay in
/// [`crate::admission::admit_login`] and in the outbound [`LoginStart`] instead
/// of in this raw record's gate: the gate restates no admission rule, so no
/// mutation after construction can make the record unpublishable.
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

    /// The total value gate this record's encoder shares with its siblings.
    ///
    /// Every field is a raw value the structural decoder already admitted, and
    /// the wire form is those bytes verbatim, so there is no field state the
    /// gate can refuse. The rules that do apply to a login — the UUIDv4
    /// identity, the canonical name after the pinned trim and the view-distance
    /// interval — are admission decisions owned by [`crate::admission`].
    pub fn validate(&self) -> Result<(), ProtocolError> {
        Ok(())
    }

    /// The exact encoded length of the raw record.
    ///
    /// The three additions are checked, so a length that cannot be represented
    /// is [`ProtocolError::Allocation`] rather than a wrapped size.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.validate()?;
        let prefix = canonical_uvarint_length(
            u32::try_from(self.display_name.len()).map_err(|_| ProtocolError::Allocation)?,
        );
        self.player_id
            .len()
            .checked_add(prefix)
            .and_then(|length| length.checked_add(self.display_name.len()))
            .and_then(|length| length.checked_add(1))
            .ok_or(ProtocolError::Allocation)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The order is the crate-wide packet pattern: validate, compute the exact
    /// length, test the destination, and only then write `dst[..length]`. A
    /// short call reports `OutputTooSmall` and leaves every destination byte
    /// unchanged. The name keeps the raw bytes the decoder admitted, including
    /// a name the canonical rule would refuse, because this record is the raw
    /// half of the inbound path.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.bytes(&self.player_id);
            let prefix = u32::try_from(self.display_name.len())
                .expect("the display name length already fitted the prefix");
            writer.uvarint(prefix);
            writer.bytes(self.display_name.as_bytes());
            writer.u8(self.view_distance);
        })
    }

    /// The allocating compatibility wrapper over `encode_into`.
    pub fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        let length = self.encoded_len()?;
        let mut wire = vec![0u8; length];
        let written = self.encode_into(&mut wire)?;
        wire.truncate(written);
        Ok(wire)
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
