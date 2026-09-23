use crate::bytes::SliceWriter;
use crate::error::ProtocolError;
use crate::varint::{canonical_uvarint_length, decode_uvarint, encode_uvarint};

/// Largest combined canonical packet ID and payload, matching the Go
/// `MaxFrameBytes` framing contract.
pub const MAX_FRAME_BYTES: u32 = 2 << 20;

/// One length-prefixed frame borrowed from the caller's input buffer.
///
/// The payload aliases the buffer the caller passed in, so reading a frame
/// costs no allocation and the borrow keeps the caller's buffer alive for as
/// long as the record is inspected. `consumed` is the exact number of input
/// bytes the frame occupies, which is how a coalesced stream advances to the
/// next record.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct FrameRef<'a> {
    pub packet_id: u32,
    pub payload: &'a [u8],
    pub consumed: usize,
}

/// Validated size pair for one frame: the body length the canonical prefix
/// declares, and the total bytes the caller's destination has to hold.
///
/// Every arithmetic step is checked, so a caller payload that would overflow
/// the frame ceiling or a length counter is refused as `FrameTooLarge` or
/// `Allocation` before any byte is written.
fn frame_sizes(packet_id: u32, payload: &[u8]) -> Result<(u32, usize), ProtocolError> {
    let body_len = canonical_uvarint_length(packet_id)
        .checked_add(payload.len())
        .ok_or(ProtocolError::Allocation)?;
    if body_len == 0 {
        // Unreachable while the packet ID varint is at least one byte, kept
        // because the Go writer publishes the same empty-frame refusal.
        return Err(ProtocolError::EmptyFrame);
    }
    let body = u32::try_from(body_len).map_err(|_| ProtocolError::FrameTooLarge)?;
    if body > MAX_FRAME_BYTES {
        return Err(ProtocolError::FrameTooLarge);
    }
    let length = canonical_uvarint_length(body)
        .checked_add(body_len)
        .ok_or(ProtocolError::Allocation)?;
    Ok((body, length))
}

/// Publishes one complete record into a caller-owned buffer.
///
/// The record's length is validated by the caller, so this function tests the
/// destination before it touches a byte and then hands `write` a window that
/// is exactly `length` bytes wide. A capacity refusal therefore reports
/// `OutputTooSmall` and leaves every destination byte unchanged, and an
/// oversized record is refused by the caller's size check before this function
/// is reached at all. The closure only lays out bytes the caller already
/// admitted, which is why it cannot fail and why a partial publication is not
/// reachable from this path.
fn publish_packet(
    length: usize,
    dst: &mut [u8],
    write: impl FnOnce(&mut SliceWriter<'_>),
) -> Result<usize, ProtocolError> {
    let Some(window) = dst.get_mut(..length) else {
        return Err(ProtocolError::OutputTooSmall {
            needed: length,
            available: dst.len(),
        });
    };
    let mut writer = SliceWriter::new(window);
    write(&mut writer);
    let written = writer.finish()?;
    debug_assert_eq!(
        written, length,
        "published record length disagrees with the validated size"
    );
    Ok(written)
}

/// Writes a length-prefixed packet ID and payload into a caller-owned buffer
/// and returns how many bytes it used.
///
/// The length prefix does not include itself. The body size is validated
/// against `MAX_FRAME_BYTES` before the destination is tested, so an invalid
/// frame size is reported as such even when the buffer is also too small. A
/// short or invalid call leaves every destination byte unchanged, and only
/// `dst[..length]` is written when the buffer is larger than the frame.
pub fn write_frame_into(
    packet_id: u32,
    payload: &[u8],
    dst: &mut [u8],
) -> Result<usize, ProtocolError> {
    let (body, length) = frame_sizes(packet_id, payload)?;
    publish_packet(length, dst, |writer| {
        writer.uvarint(body);
        writer.uvarint(packet_id);
        writer.bytes(payload);
    })
}

/// Writes a length-prefixed packet ID and payload into an owned buffer.
///
/// This is the allocating compatibility wrapper: it sizes the frame once,
/// reserves exactly that many bytes and publishes through
/// `write_frame_into`, so both entry points always agree byte for byte.
pub fn write_frame(packet_id: u32, payload: &[u8]) -> Result<Vec<u8>, ProtocolError> {
    let (_, length) = frame_sizes(packet_id, payload)?;
    let mut wire = vec![0u8; length];
    let written = write_frame_into(packet_id, payload, &mut wire)?;
    wire.truncate(written);
    Ok(wire)
}

/// Reads one bounded length-prefixed packet from `data` and borrows its payload.
///
/// The declared length is validated before any payload is published, so a
/// truncated or oversized frame is refused without copying a byte. The
/// returned payload aliases `data`; the caller owns the buffer's lifetime.
pub fn read_frame_ref(data: &[u8]) -> Result<FrameRef<'_>, ProtocolError> {
    let (frame_length, prefix_len) = decode_uvarint(data)?;
    if frame_length == 0 {
        return Err(ProtocolError::EmptyFrame);
    }
    if frame_length > MAX_FRAME_BYTES {
        return Err(ProtocolError::FrameTooLarge);
    }
    let frame_len = usize::try_from(frame_length).map_err(|_| ProtocolError::FrameTooLarge)?;
    let body_end = prefix_len
        .checked_add(frame_len)
        .ok_or(ProtocolError::FrameTooLarge)?;
    if data.len() < body_end {
        return Err(ProtocolError::Truncated);
    }
    let body = &data[prefix_len..body_end];
    let (packet_id, id_len) = decode_uvarint(body)?;
    Ok(FrameRef {
        packet_id,
        payload: &body[id_len..],
        consumed: body_end,
    })
}

/// Reads one bounded length-prefixed packet from `data`.
///
/// The declared length is validated before the payload is copied. Returns the
/// packet ID, an owned payload that does not alias `data`, and the number of
/// input bytes consumed. This is the allocating compatibility wrapper over
/// `read_frame_ref`.
pub fn read_frame(data: &[u8]) -> Result<(u32, Vec<u8>, usize), ProtocolError> {
    let frame = read_frame_ref(data)?;
    Ok((frame.packet_id, frame.payload.to_vec(), frame.consumed))
}
