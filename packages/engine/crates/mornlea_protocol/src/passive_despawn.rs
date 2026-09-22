use crate::batch::{ByteCountBatch, strictly_increasing};
use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Maximum passive mob records one payload may carry, copied from the Go
/// `MaxPassiveRecords` pin.
pub const MAX_PASSIVE_RECORDS: u8 = 64;

/// Fixed stride of one passive despawn record: an ID plus the reason byte.
pub const PASSIVE_DESPAWN_WIRE_BYTES: usize = 8 + 1;

/// Removal reasons: the mob left the subscription range or it died.
pub const PASSIVE_DESPAWN_VANISHED: u8 = 0;
pub const PASSIVE_DESPAWN_DIED: u8 = 1;

/// One passive mob removal fact: a non-zero ID and one of the two published
/// removal reasons.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PassiveDespawnRecord {
    pub id: u64,
    pub reason: u8,
}

/// Play PassiveDespawn payload: the passive mob bodies a session must drop.
/// Records carry the ID and the removal reason, are strictly ascending by ID,
/// and a zero ID or an unknown reason is rejected.
#[derive(Clone, Debug, Eq, PartialEq)]
pub struct PassiveDespawn {
    pub server_tick: u64,
    pub despawns: Vec<PassiveDespawnRecord>,
}

impl PassiveDespawn {
    pub const PACKET_ID: u32 = 28;

    pub fn new(
        server_tick: u64,
        despawns: Vec<PassiveDespawnRecord>,
    ) -> Result<Self, ProtocolError> {
        if despawns.is_empty() || despawns.len() > MAX_PASSIVE_RECORDS as usize {
            return Err(ProtocolError::InvalidRange);
        }
        for (index, record) in despawns.iter().enumerate() {
            if record.id == 0 {
                return Err(ProtocolError::InvalidRange);
            }
            if record.reason != PASSIVE_DESPAWN_VANISHED && record.reason != PASSIVE_DESPAWN_DIED {
                return Err(ProtocolError::InvalidEnum);
            }
            if index > 0 && despawns[index - 1].id >= record.id {
                return Err(ProtocolError::InvalidRange);
            }
        }
        Ok(Self {
            server_tick,
            despawns,
        })
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        ByteCountBatch::write(&mut encoder, self.server_tick, self.despawns.len() as u8);
        for record in &self.despawns {
            encoder.u64(record.id);
            encoder.u8(record.reason);
        }
        encoder
            .finish()
            .expect("validated passive despawn is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let batch = ByteCountBatch::read(&mut decoder, MAX_PASSIVE_RECORDS)?;
        batch.require_records(&decoder, PASSIVE_DESPAWN_WIRE_BYTES)?;
        let mut despawns = Vec::with_capacity(batch.count as usize);
        for _ in 0..batch.count {
            let id = decoder.u64()?;
            let reason = decoder.u8()?;
            despawns.push(PassiveDespawnRecord { id, reason });
        }
        decoder.done()?;
        Self::new(batch.server_tick, despawns)
    }
}
