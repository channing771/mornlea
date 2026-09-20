use crate::batch::{ByteCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Maximum projectile records one payload may carry, copied from the Go
/// `MaxProjectileRecords` pin.
pub const MAX_PROJECTILE_RECORDS: u8 = 128;

/// Fixed stride of one projectile despawn record: a little-endian `u64` ID.
pub const PROJECTILE_DESPAWN_WIRE_BYTES: usize = 8;

/// Play ProjectileDespawn payload: the authoritative projectile bodies a
/// session must drop. Records carry only the ID, are strictly ascending, and
/// a zero ID is never a live projectile.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct ProjectileDespawn {
    pub server_tick: u64,
    pub ids: Vec<u64>,
}

impl ProjectileDespawn {
    pub const PACKET_ID: u32 = 31;

    pub fn new(server_tick: u64, ids: Vec<u64>) -> Result<Self, ProtocolError> {
        if ids.is_empty() || ids.len() > MAX_PROJECTILE_RECORDS as usize {
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
            .expect("validated projectile despawn is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let batch = ByteCountBatch::read(&mut decoder, MAX_PROJECTILE_RECORDS)?;
        batch.require_records(&decoder, PROJECTILE_DESPAWN_WIRE_BYTES)?;
        let mut ids = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            ids.push(decoder.u64()?);
        }
        decoder.done()?;
        Self::new(batch.server_tick, ids)
    }
}
