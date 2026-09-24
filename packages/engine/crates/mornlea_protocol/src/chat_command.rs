use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use crate::varint::canonical_uvarint_length;
use mornlea_domain::CommandText;

/// Maximum UTF-8 byte length of a chat command text, shared with the
/// planner instruction limit so the two bounds cannot drift apart.
pub const CHAT_COMMAND_TEXT_MAX_BYTES: usize = 1024;

/// Fixed wire upper bound of the whole payload, mirroring the Go
/// `ChatCommandMaxWireBytes`: the two-byte canonical uvarint prefix a
/// maximum-length text carries plus the text itself.
pub const CHAT_COMMAND_MAX_WIRE_BYTES: usize = CHAT_COMMAND_TEXT_MAX_BYTES + 2;

/// Reports whether a bounded text slot is publishable: at least one byte, at
/// most `max_bytes` bytes, no surrounding whitespace, and no control
/// character. The rule is the Go `validateSpeechText` shape with the bound
/// supplied by the caller, so the two text slots cannot drift apart.
///
/// This is the last local bounded-text rule in the crate: only the chat
/// event's speech slot still consumes it, and routing that slot through the
/// domain `SpeechText` is a later node's change. Until then this copy must
/// not drift from the domain bounds.
pub(crate) fn valid_bounded_text(text: &str, max_bytes: usize) -> bool {
    !text.is_empty()
        && text.len() <= max_bytes
        && text.trim() == text
        && !text.chars().any(char::is_control)
}

/// Reports whether a chat command text slot is publishable.
///
/// The rule is the domain `CommandText` rule — the Go `validateCommandText`
/// shape of at least one byte, at most `CHAT_COMMAND_TEXT_MAX_BYTES` bytes, no
/// surrounding whitespace and no control character, with the pinned whitespace
/// and control sets the domain owns. Both the `ChatCommand` wire slot and the
/// chat event's command restatement route through it, so the wire, the domain
/// and the Go validator share one admitted set instead of two copies that can
/// drift apart.
pub(crate) fn valid_command_text(text: &str) -> bool {
    CommandText::try_from_canonical(text.to_owned()).is_ok()
}

/// Play ChatCommand payload: a length-prefixed UTF-8 instruction for the
/// authority. The server decides what the text may do; this codec only
/// keeps the transfer bounded and well-formed.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ChatCommand {
    pub text: String,
}

impl ChatCommand {
    pub const PACKET_ID: u32 = 12;

    pub fn new(text: String) -> Result<Self, ProtocolError> {
        let command = Self { text };
        command.valid()?;
        Ok(command)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The text is a public field, so the gate runs on every encode instead of
    /// only at construction: a record mutated into an untrimmed, empty,
    /// control-carrying or oversized text after construction is refused rather
    /// than silently published. The admitted set is the domain `CommandText`
    /// rule, so the wire slot cannot admit a text the authority refuses.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if !valid_command_text(&self.text) {
            return Err(ProtocolError::InvalidString);
        }
        Ok(())
    }

    /// The exact encoded length: the canonical uvarint length prefix followed
    /// by the text bytes.
    ///
    /// The value gate runs first, so an invalid record reports its value error
    /// here instead of reaching the capacity check.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        command_text_len(&self.text)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — the length prefix, then the text
    /// bytes — and the destination is tested before the first byte is written,
    /// so a short or invalid call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.uvarint(command_text_prefix(&self.text));
            writer.bytes(self.text.as_bytes());
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
        // The Go decoder applies the payload ceiling before it reads a single
        // field, so an oversized payload reports the ceiling refusal rather
        // than a field error.
        if payload.len() > CHAT_COMMAND_MAX_WIRE_BYTES {
            return Err(ProtocolError::FrameTooLarge);
        }
        let mut decoder = ByteDecoder::new(payload);
        let text = read_command_text(&mut decoder)?;
        decoder.done()?;
        Self::new(text)
    }
}

/// The exact encoded length of one validated text slot: the canonical uvarint
/// length prefix followed by the text bytes.
///
/// Every arithmetic step is checked, so an oversized length counter is refused
/// as `Allocation` before the record is sized.
fn command_text_len(text: &str) -> Result<usize, ProtocolError> {
    canonical_uvarint_length(command_text_prefix(text))
        .checked_add(text.len())
        .ok_or(ProtocolError::Allocation)
}

/// The uvarint length prefix one validated text carries.
///
/// The domain bound is checked before any caller reaches this function, so the
/// conversion cannot fail and an unvalidated text is the caller's contract
/// break rather than a silently truncated prefix.
fn command_text_prefix(text: &str) -> u32 {
    u32::try_from(text.len()).expect("validated chat command fits its length prefix")
}

/// Reads one length-prefixed bounded text slot in field order.
///
/// The declared length is checked against the slot's byte bound and against
/// the bytes that remain before anything is copied. A declared length the
/// payload cannot complete is an incomplete payload and reports `Truncated`
/// rather than the `InvalidString` the shared string primitive reports for the
/// same bytes, mirroring the control message reader in
/// [`crate::handshake_reject::read_control_message`]: the Go
/// `byteDecoder.string` answers that condition with the same sentinel as a
/// malformed UTF-8 text, while the frozen corpus category for an incomplete
/// payload is `truncated`. Applying the same boundary here keeps the Go
/// producer and the Rust consumer publishing one category for every family
/// that carries a length-prefixed text slot.
pub(crate) fn read_bounded_text(
    decoder: &mut ByteDecoder<'_>,
    max_bytes: usize,
    max_runes: usize,
) -> Result<String, ProtocolError> {
    let length = usize::try_from(decoder.uvarint()?).map_err(|_| ProtocolError::InvalidString)?;
    if length > max_bytes {
        return Err(ProtocolError::InvalidString);
    }
    if length > decoder.remaining() {
        return Err(ProtocolError::Truncated);
    }
    let bytes = decoder.take(length)?;
    let text = std::str::from_utf8(bytes).map_err(|_| ProtocolError::InvalidString)?;
    if text.chars().count() > max_runes {
        return Err(ProtocolError::InvalidString);
    }
    Ok(text.to_owned())
}

/// Reads one length-prefixed chat command text in field order.
///
/// The slot's byte and rune bounds are the same number, because a text of at
/// most `CHAT_COMMAND_TEXT_MAX_BYTES` bytes carries at most that many runes.
fn read_command_text(decoder: &mut ByteDecoder<'_>) -> Result<String, ProtocolError> {
    read_bounded_text(
        decoder,
        CHAT_COMMAND_TEXT_MAX_BYTES,
        CHAT_COMMAND_TEXT_MAX_BYTES,
    )
}
