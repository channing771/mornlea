use crate::error::ProtocolError;

pub(crate) fn canonical_uvarint_length(value: u32) -> usize {
    match value {
        0..=0x7f => 1,
        0x80..=0x3fff => 2,
        0x4000..=0x1f_ffff => 3,
        0x20_0000..=0xfff_ffff => 4,
        _ => 5,
    }
}

/// Encodes a canonical unsigned varint. Continuation bits stay set until the
/// shortest representation of `value` is complete.
pub fn encode_uvarint(mut value: u32) -> Vec<u8> {
    let mut encoded = Vec::with_capacity(canonical_uvarint_length(value));
    while value >= 1 << 7 {
        encoded.push((value as u8) | 0x80);
        value >>= 7;
    }
    encoded.push(value as u8);
    encoded
}

/// Writes the canonical encoding of `value` into the front of `dst` and
/// returns how many bytes it used.
///
/// The encoding length is derived from the value rather than accumulated, so
/// the bytes are canonical by construction. A destination shorter than the
/// encoding is refused with `OutputTooSmall` before any byte is written, which
/// keeps a caller-owned buffer untouched on a capacity refusal.
pub(crate) fn encode_uvarint_into(value: u32, dst: &mut [u8]) -> Result<usize, ProtocolError> {
    let length = canonical_uvarint_length(value);
    let available = dst.len();
    let window = dst.get_mut(..length).ok_or(ProtocolError::OutputTooSmall {
        needed: length,
        available,
    })?;
    for (index, slot) in window.iter_mut().enumerate() {
        let mut byte = ((value >> (7 * index)) & 0x7f) as u8;
        // Every group but the last keeps the continuation bit set, so the
        // shortest form is the only form this writes.
        if index + 1 < length {
            byte |= 0x80;
        }
        *slot = byte;
    }
    Ok(length)
}

/// Decodes a canonical unsigned varint and the number of bytes consumed.
///
/// Truncated input, overlong encodings, and values that do not fit in 32 bits
/// fail without consuming a payload.
pub fn decode_uvarint(data: &[u8]) -> Result<(u32, usize), ProtocolError> {
    let mut value = 0u32;
    for index in 0..5 {
        let Some(&byte) = data.get(index) else {
            return Err(ProtocolError::Truncated);
        };
        if index == 4 && byte & 0xf0 != 0 {
            return Err(ProtocolError::InvalidUvarint);
        }
        value |= u32::from(byte & 0x7f) << (7 * index);
        if byte & 0x80 == 0 {
            if canonical_uvarint_length(value) != index + 1 {
                return Err(ProtocolError::NonCanonicalUvarint);
            }
            return Ok((value, index + 1));
        }
    }
    Err(ProtocolError::InvalidUvarint)
}
