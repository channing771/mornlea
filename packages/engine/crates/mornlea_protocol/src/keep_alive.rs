use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;

/// Fixed wire stride of one keep alive payload: a little-endian `u64` token.
/// The reply shares the stride because the wire record is identical in both
/// directions.
pub(crate) const KEEP_ALIVE_WIRE_BYTES: usize = 8;

/// Play KeepAlive payload. A zero token is rejected before publication.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct KeepAlive {
    pub token: u64,
}

impl KeepAlive {
    pub const PACKET_ID: u32 = 5;

    pub fn new(token: u64) -> Result<Self, ProtocolError> {
        let keep_alive = Self { token };
        keep_alive.valid()?;
        Ok(keep_alive)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The token is a public field, so the gate runs on every encode instead
    /// of only at construction: a record mutated to the zero token after
    /// construction is refused rather than silently published.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        if self.token == 0 {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    /// The exact encoded length, which is the fixed token stride.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(KEEP_ALIVE_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The token is little-endian, matching the Go encoder, and the
    /// destination is tested before the first byte is written, so a short or
    /// invalid call leaves every destination byte unchanged.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.token);
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
        let token = decoder.u64()?;
        decoder.done()?;
        Self::new(token)
    }
}
