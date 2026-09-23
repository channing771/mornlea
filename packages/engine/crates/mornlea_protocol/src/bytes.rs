use crate::error::ProtocolError;
use crate::varint::{decode_uvarint, encode_uvarint};

/// Fail-first payload encoder matching the Go `byteEncoder` contract.
/// The first error is retained and later writes are skipped so a packet
/// never publishes a partial payload.
pub(crate) struct ByteEncoder {
    data: Vec<u8>,
    err: Option<ProtocolError>,
}

impl ByteEncoder {
    pub(crate) fn new() -> Self {
        Self {
            data: Vec::new(),
            err: None,
        }
    }

    /// Builds an encoder that reserves `capacity` bytes up front, so a payload
    /// whose exact size is known does not reallocate while it is written.
    pub(crate) fn with_capacity(capacity: usize) -> Self {
        Self {
            data: Vec::with_capacity(capacity),
            err: None,
        }
    }

    fn fail(&mut self, err: ProtocolError) {
        if self.err.is_none() {
            self.err = Some(err);
        }
    }

    pub(crate) fn uvarint(&mut self, value: u32) {
        if self.err.is_some() {
            return;
        }
        self.data.extend(encode_uvarint(value));
    }

    pub(crate) fn u8(&mut self, value: u8) {
        if self.err.is_some() {
            return;
        }
        self.data.push(value);
    }

    pub(crate) fn bytes(&mut self, value: &[u8]) {
        if self.err.is_some() {
            return;
        }
        self.data.extend_from_slice(value);
    }

    pub(crate) fn u16(&mut self, value: u16) {
        if self.err.is_some() {
            return;
        }
        self.data.extend_from_slice(&value.to_le_bytes());
    }

    pub(crate) fn u32(&mut self, value: u32) {
        if self.err.is_some() {
            return;
        }
        self.data.extend_from_slice(&value.to_le_bytes());
    }

    pub(crate) fn i32(&mut self, value: i32) {
        self.u32(value as u32);
    }

    pub(crate) fn boolean(&mut self, value: bool) {
        self.u8(u8::from(value));
    }

    pub(crate) fn i8(&mut self, value: i8) {
        self.u8(value as u8);
    }

    pub(crate) fn u64(&mut self, value: u64) {
        if self.err.is_some() {
            return;
        }
        self.data.extend_from_slice(&value.to_le_bytes());
    }

    pub(crate) fn f32(&mut self, value: f32) {
        if self.err.is_some() {
            return;
        }
        if !value.is_finite() {
            self.fail(ProtocolError::InvalidFloat);
            return;
        }
        self.u32(value.to_bits());
    }

    pub(crate) fn string(&mut self, value: &str, max_bytes: usize) {
        if self.err.is_some() {
            return;
        }
        if value.len() > max_bytes {
            self.fail(ProtocolError::InvalidString);
            return;
        }
        let len = u32::try_from(value.len()).expect("string shorter than max_bytes");
        self.uvarint(len);
        if self.err.is_none() {
            self.data.extend_from_slice(value.as_bytes());
        }
    }

    pub(crate) fn finish(self) -> Result<Vec<u8>, ProtocolError> {
        match self.err {
            Some(err) => Err(err),
            None => Ok(self.data),
        }
    }
}

/// Sequential payload decoder. Length, UTF-8, and capacity checks happen
/// before a string is copied; trailing bytes fail after the last field.
pub(crate) struct ByteDecoder<'a> {
    data: &'a [u8],
    offset: usize,
}

impl<'a> ByteDecoder<'a> {
    pub(crate) fn new(data: &'a [u8]) -> Self {
        Self { data, offset: 0 }
    }

    pub(crate) fn remaining(&self) -> usize {
        self.data.len().saturating_sub(self.offset)
    }

    pub(crate) fn take(&mut self, n: usize) -> Result<&'a [u8], ProtocolError> {
        let end = self.offset.checked_add(n).ok_or(ProtocolError::Truncated)?;
        if end > self.data.len() {
            return Err(ProtocolError::Truncated);
        }
        let slice = &self.data[self.offset..end];
        self.offset = end;
        Ok(slice)
    }

    pub(crate) fn uvarint(&mut self) -> Result<u32, ProtocolError> {
        let (value, used) = decode_uvarint(&self.data[self.offset..])?;
        self.offset += used;
        Ok(value)
    }

    pub(crate) fn u16(&mut self) -> Result<u16, ProtocolError> {
        Ok(u16::from_le_bytes(self.bytes()?))
    }

    pub(crate) fn u8(&mut self) -> Result<u8, ProtocolError> {
        Ok(self.take(1)?[0])
    }

    pub(crate) fn bytes<const N: usize>(&mut self) -> Result<[u8; N], ProtocolError> {
        let slice = self.take(N)?;
        let mut value = [0u8; N];
        value.copy_from_slice(slice);
        Ok(value)
    }

    pub(crate) fn u32(&mut self) -> Result<u32, ProtocolError> {
        Ok(u32::from_le_bytes(self.bytes()?))
    }

    pub(crate) fn i32(&mut self) -> Result<i32, ProtocolError> {
        Ok(i32::from_le_bytes(self.bytes()?))
    }

    pub(crate) fn i8(&mut self) -> Result<i8, ProtocolError> {
        Ok(self.u8()? as i8)
    }

    pub(crate) fn boolean(&mut self) -> Result<bool, ProtocolError> {
        match self.u8()? {
            0 => Ok(false),
            1 => Ok(true),
            _ => Err(ProtocolError::InvalidEnum),
        }
    }

    pub(crate) fn u64(&mut self) -> Result<u64, ProtocolError> {
        Ok(u64::from_le_bytes(self.bytes()?))
    }

    pub(crate) fn f32(&mut self) -> Result<f32, ProtocolError> {
        let value = f32::from_bits(self.u32()?);
        if !value.is_finite() {
            return Err(ProtocolError::InvalidFloat);
        }
        Ok(value)
    }

    pub(crate) fn string(
        &mut self,
        max_bytes: usize,
        max_runes: usize,
    ) -> Result<String, ProtocolError> {
        let length = usize::try_from(self.uvarint()?).map_err(|_| ProtocolError::InvalidString)?;
        if length > max_bytes || length > self.remaining() {
            return Err(ProtocolError::InvalidString);
        }
        let bytes = self.take(length)?;
        let text = std::str::from_utf8(bytes).map_err(|_| ProtocolError::InvalidString)?;
        if text.chars().count() > max_runes {
            return Err(ProtocolError::InvalidString);
        }
        Ok(text.to_owned())
    }

    pub(crate) fn done(self) -> Result<(), ProtocolError> {
        if self.offset != self.data.len() {
            Err(ProtocolError::TrailingBytes)
        } else {
            Ok(())
        }
    }
}

/// Bounded writer that publishes fixed-width fields into a caller-owned slice.
///
/// This is the publication half of the packet pattern: the caller validates
/// and sizes the record first, then `publish_packet` hands the writer a window
/// that is exactly that many bytes long. It owns no heap buffer and never
/// allocates, so an encoder on the authoritative hot path cannot allocate
/// through it. It performs no semantic validation either: a value rule such
/// as a finite rotation, a slot range or a string bound is an admission
/// decision the packet validator owns, and the writer only lays out bytes the
/// caller already admitted.
pub(crate) struct SliceWriter<'a> {
    dst: &'a mut [u8],
    offset: usize,
    err: Option<ProtocolError>,
}

impl<'a> SliceWriter<'a> {
    pub(crate) fn new(dst: &'a mut [u8]) -> Self {
        Self {
            dst,
            offset: 0,
            err: None,
        }
    }

    /// Bytes published into the caller's window so far.
    pub(crate) fn written(&self) -> usize {
        self.offset
    }

    /// Bytes left in the caller's window.
    pub(crate) fn remaining(&self) -> usize {
        self.dst.len().saturating_sub(self.offset)
    }

    /// Retains the first failure so a later write cannot overwrite it, exactly
    /// like the allocating `ByteEncoder`.
    fn fail(&mut self, err: ProtocolError) {
        if self.err.is_none() {
            self.err = Some(err);
        }
    }

    fn put(&mut self, bytes: &[u8]) {
        if self.err.is_some() {
            return;
        }
        let Some(end) = self.offset.checked_add(bytes.len()) else {
            self.fail(ProtocolError::Allocation);
            return;
        };
        let Some(window) = self.dst.get_mut(self.offset..end) else {
            // Only reachable when a caller-sized window disagrees with the
            // bytes the record actually writes, which is a crate bug the
            // `publish_packet` debug assertion catches.
            self.fail(ProtocolError::OutputTooSmall {
                needed: end,
                available: self.dst.len(),
            });
            return;
        };
        window.copy_from_slice(bytes);
        self.offset = end;
    }

    pub(crate) fn u8(&mut self, value: u8) {
        self.put(&[value]);
    }

    pub(crate) fn i8(&mut self, value: i8) {
        self.u8(value as u8);
    }

    pub(crate) fn u16(&mut self, value: u16) {
        self.put(&value.to_le_bytes());
    }

    pub(crate) fn u32(&mut self, value: u32) {
        self.put(&value.to_le_bytes());
    }

    pub(crate) fn i32(&mut self, value: i32) {
        self.u32(value as u32);
    }

    pub(crate) fn u64(&mut self, value: u64) {
        self.put(&value.to_le_bytes());
    }

    pub(crate) fn boolean(&mut self, value: bool) {
        self.u8(u8::from(value));
    }

    pub(crate) fn bytes(&mut self, value: &[u8]) {
        self.put(value);
    }

    pub(crate) fn uvarint(&mut self, value: u32) {
        // Five bytes is the largest canonical u32 varint, so the scratch write
        // below cannot fail and only the destination window is a limit.
        let mut encoded = [0u8; 5];
        match crate::varint::encode_uvarint_into(value, &mut encoded) {
            Ok(length) => self.put(&encoded[..length]),
            Err(err) => self.fail(err),
        }
    }

    /// Publishes the exact IEEE-754 bits of an already-admitted rotation.
    ///
    /// Rejecting a non-finite value is the packet validator's decision, so this
    /// writer does not test it and cannot silently clamp an admitted angle.
    pub(crate) fn f32(&mut self, value: f32) {
        self.u32(value.to_bits());
    }

    /// Completes the publication and reports how many bytes were written.
    pub(crate) fn finish(self) -> Result<usize, ProtocolError> {
        match self.err {
            Some(err) => Err(err),
            None => Ok(self.offset),
        }
    }
}
