use crate::error::ProtocolError;
use crate::varint::{decode_uvarint, encode_uvarint};

/// Largest combined canonical packet ID and payload, matching the Go
/// `MaxFrameBytes` framing contract.
pub const MAX_FRAME_BYTES: u32 = 2 << 20;

/// Writes a length-prefixed packet ID and payload. The length prefix does not
/// include itself. Empty and oversized frames fail before any bytes are
/// published.
pub fn write_frame(packet_id: u32, payload: &[u8]) -> Result<Vec<u8>, ProtocolError> {
    let id = encode_uvarint(packet_id);
    let frame_length = u64::try_from(id.len())
        .ok()
        .and_then(|len| len.checked_add(u64::try_from(payload.len()).ok()?))
        .ok_or(ProtocolError::FrameTooLarge)?;
    if frame_length == 0 {
        return Err(ProtocolError::EmptyFrame);
    }
    if frame_length > u64::from(MAX_FRAME_BYTES) {
        return Err(ProtocolError::FrameTooLarge);
    }
    let length = u32::try_from(frame_length).map_err(|_| ProtocolError::FrameTooLarge)?;
    let mut wire = encode_uvarint(length);
    wire.extend_from_slice(&id);
    wire.extend_from_slice(payload);
    Ok(wire)
}

/// Reads one bounded length-prefixed packet from `data`.
///
/// The declared length is validated before the frame body is copied. Returns
/// the packet ID, an owned payload that does not alias `data`, and the number
/// of input bytes consumed.
pub fn read_frame(data: &[u8]) -> Result<(u32, Vec<u8>, usize), ProtocolError> {
    let (frame_length, prefix_len) = decode_uvarint(data)?;
    if frame_length == 0 {
        return Err(ProtocolError::EmptyFrame);
    }
    if frame_length > MAX_FRAME_BYTES {
        return Err(ProtocolError::FrameTooLarge);
    }
    let frame_len = frame_length as usize;
    let body_start = prefix_len;
    let body_end = body_start
        .checked_add(frame_len)
        .ok_or(ProtocolError::FrameTooLarge)?;
    if data.len() < body_end {
        return Err(ProtocolError::Truncated);
    }
    let body = &data[body_start..body_end];
    let (packet_id, id_len) = decode_uvarint(body)?;
    Ok((packet_id, body[id_len..].to_vec(), body_end))
}
