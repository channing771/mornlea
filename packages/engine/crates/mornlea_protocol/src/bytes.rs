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

    pub(crate) fn u64(&mut self, value: u64) {
        if self.err.is_some() {
            return;
        }
        self.data.extend_from_slice(&value.to_le_bytes());
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

    fn remaining(&self) -> usize {
        self.data.len().saturating_sub(self.offset)
    }

    fn take(&mut self, n: usize) -> Result<&'a [u8], ProtocolError> {
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

    pub(crate) fn u8(&mut self) -> Result<u8, ProtocolError> {
        Ok(self.take(1)?[0])
    }

    pub(crate) fn bytes<const N: usize>(&mut self) -> Result<[u8; N], ProtocolError> {
        let slice = self.take(N)?;
        let mut value = [0u8; N];
        value.copy_from_slice(slice);
        Ok(value)
    }

    pub(crate) fn u64(&mut self) -> Result<u64, ProtocolError> {
        Ok(u64::from_le_bytes(self.bytes()?))
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
