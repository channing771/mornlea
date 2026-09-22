use crate::batch::{ByteCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Maximum hostile records one payload may carry, copied from the Go
/// `MaxHostileRecords` pin.
pub const MAX_HOSTILE_RECORDS: u8 = 64;

/// Fixed stride of one hostile despawn record: a little-endian `u64` ID.
pub const HOSTILE_DESPAWN_WIRE_BYTES: usize = 8;

/// Play HostileDespawn payload: the authoritative hostile bodies a session
/// must drop. Records carry only the ID, are strictly ascending, and a zero
/// ID is never a live hostile.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct HostileDespawn {
    pub server_tick: u64,
    pub ids: Vec<u64>,
}

impl HostileDespawn {
    pub const PACKET_ID: u32 = 24;

    pub fn new(server_tick: u64, ids: Vec<u64>) -> Result<Self, ProtocolError> {
        if ids.is_empty() || ids.len() > MAX_HOSTILE_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        if ids.iter().any(|id| *id == 0) || !strictly_increasing(&ids) {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(Self { server_tick, ids })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        ByteCountBatch::write(&mut encoder, self.server_tick, self.ids.len() as u8);
        for id in &self.ids {
            encoder.u64(*id);
        }
        encoder
            .finish()
            .expect("validated hostile despawn is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let batch = ByteCountBatch::read(&mut decoder, MAX_HOSTILE_RECORDS)?;
        batch.require_records(&decoder, HOSTILE_DESPAWN_WIRE_BYTES)?;
        let mut ids = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            ids.push(decoder.u64()?);
        }
        decoder.done()?;
        Self::new(batch.server_tick, ids)
    }
}
