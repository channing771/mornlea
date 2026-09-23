use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;

/// Byte ceiling one control message carries, matching the Go codec's string
/// bound for the handshake reject, login reject and disconnect families.
pub(crate) const MESSAGE_MAX_BYTES: usize = 256;
/// Rune ceiling one control message carries, matching the Go codec's string
/// bound for the same three families.
pub(crate) const MESSAGE_MAX_RUNES: usize = 256;

/// Handshake reject code copied from the Go `HandshakeVersionMismatch` pin.
pub const HANDSHAKE_VERSION_MISMATCH: u8 = 1;

/// Reports whether one control message is publishable.
///
/// The rule is the Go codec's string bound for the control families: at most
/// 256 bytes and at most 256 runes. UTF-8 validity is a property of `String`
/// and needs no branch here. The three message-carrying control families share
/// this gate so their admitted sets cannot drift apart.
pub(crate) fn valid_control_message(message: &str) -> Result<(), ProtocolError> {
    if message.len() > MESSAGE_MAX_BYTES || message.chars().count() > MESSAGE_MAX_RUNES {
        return Err(ProtocolError::InvalidString);
    }
    Ok(())
}

/// The exact encoded length of one validated message slot: the canonical
/// uvarint length prefix followed by the message bytes.
///
/// Every arithmetic step is checked, so an oversized length counter is refused
/// as `Allocation` before the record is sized.
pub(crate) fn control_message_len(prefix: usize, message: &str) -> Result<usize, ProtocolError> {
    prefix
        .checked_add(canonical_uvarint_length(message_length_prefix(message)))
        .and_then(|sum| sum.checked_add(message.len()))
        .ok_or(ProtocolError::Allocation)
}

/// The uvarint length prefix one validated message carries.
///
/// The message bound is checked before any caller reaches this function, so
/// the conversion cannot fail and an unvalidated message is the caller's
/// contract break rather than a silently truncated prefix.
pub(crate) fn message_length_prefix(message: &str) -> u32 {
    u32::try_from(message.len()).expect("validated message fits its length prefix")
}

/// Reads one length-prefixed control message in field order.
///
/// The declared length is checked against the family bound and against the
/// bytes that remain before anything is copied. A declared length the payload
/// cannot complete is an incomplete payload and reports `Truncated` rather than
/// the `InvalidString` the shared string primitive reports for the same bytes:
/// the Go `byteDecoder.string` answers that condition with the same sentinel as
/// a malformed UTF-8 message, while the frozen corpus category for an
/// incomplete payload is `truncated`. Owning the boundary here gives the three
/// message-carrying control families one reader, so the Go producer and the
/// Rust consumer publish one category.
pub(crate) fn read_control_message(decoder: &mut ByteDecoder<'_>) -> Result<String, ProtocolError> {
    let length = usize::try_from(decoder.uvarint()?).map_err(|_| ProtocolError::InvalidString)?;
    if length > MESSAGE_MAX_BYTES {
        return Err(ProtocolError::InvalidString);
    }
    if length > decoder.remaining() {
        return Err(ProtocolError::Truncated);
    }
    let bytes = decoder.take(length)?;
    let text = std::str::from_utf8(bytes).map_err(|_| ProtocolError::InvalidString)?;
    if text.chars().count() > MESSAGE_MAX_RUNES {
        return Err(ProtocolError::InvalidString);
    }
    Ok(text.to_owned())
}

/// Handshake HandshakeReject payload. Unknown reject codes, oversized
/// messages, and malformed UTF-8 fail before the record is published.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct HandshakeReject {
    pub server_protocol_version: u32,
    pub code: u8,
    pub message: String,
}

impl HandshakeReject {
    pub const PACKET_ID: u32 = 1;

    pub fn new(
        server_protocol_version: u32,
        code: u8,
        message: impl Into<String>,
    ) -> Result<Self, ProtocolError> {
        let reject = Self {
            server_protocol_version,
            code,
            message: message.into(),
        };
        reject.valid()?;
        Ok(reject)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// Both the code and the message are public fields, so the gate runs on
    /// every encode instead of only at construction: a record mutated into an
    /// unpublished code or an oversized message after construction is refused
    /// rather than silently published. The server protocol version is
    /// informational and is deliberately not gated.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.code != HANDSHAKE_VERSION_MISMATCH {
            return Err(ProtocolError::InvalidEnum);
        }
        valid_control_message(&self.message)
    }

    /// The exact encoded length: the version varint, the code byte, and the
    /// message slot.
    ///
    /// The value gate runs first, so an invalid record reports its value error
    /// here instead of reaching the capacity check.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        let prefix = canonical_uvarint_length(self.server_protocol_version)
            .checked_add(1)
            .ok_or(ProtocolError::Allocation)?;
        control_message_len(prefix, &self.message)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's, and the destination is tested
    /// before the first byte is written, so a short or invalid call leaves
    /// every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.uvarint(self.server_protocol_version);
            writer.u8(self.code);
            writer.uvarint(message_length_prefix(&self.message));
            writer.bytes(self.message.as_bytes());
        })
    }

    /// The allocating compatibility wrapper.
    ///
    /// It reserves exactly the validated length and publishes through
    /// `encode_into`, so the two entry points always agree byte for byte.
    pub fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        let length = self.encoded_len()?;
        let mut wire = vec![0u8; length];
        let written = self.encode_into(&mut wire)?;
        wire.truncate(written);
        Ok(wire)
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let server_protocol_version = decoder.uvarint()?;
        let code = decoder.u8()?;
        let message = read_control_message(&mut decoder)?;
        decoder.done()?;
        Self::new(server_protocol_version, code, message)
    }
}
