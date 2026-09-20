use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
use mornlea_domain::PlayerInput as DomainPlayerInput;

/// Play PlayerInput payload, and the highest-frequency packet on the wire:
/// a `u64` sequence, two `i8` move axes, four action flags, and two `f32`
/// look angles. The authoritative tick consumes this as a semantic record;
/// the client's ability to claim a state never settles the world.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PlayerInput {
    pub sequence: u64,
    pub move_x: i8,
    pub move_z: i8,
    pub jump: bool,
    pub yaw: f32,
    pub pitch: f32,
    pub mining: bool,
    pub eating: bool,
    pub sprinting: bool,
    pub sneaking: bool,
}

impl PlayerInput {
    pub const PACKET_ID: u32 = 0;

    pub fn new(
        sequence: u64,
        move_x: i8,
        move_z: i8,
        jump: bool,
        yaw: f32,
        pitch: f32,
        mining: bool,
        eating: bool,
        sprinting: bool,
        sneaking: bool,
    ) -> Result<Self, ProtocolError> {
        DomainPlayerInput::new(
            sequence, move_x, move_z, jump, yaw, pitch, mining, eating, sprinting, sneaking,
        )
        .map_err(|_| ProtocolError::InvalidFloat)?;
        Ok(Self {
            sequence,
            move_x,
            move_z,
            jump,
            yaw,
            pitch,
            mining,
            eating,
            sprinting,
            sneaking,
        })
    }

    pub fn encode(self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.sequence);
        encoder.i8(self.move_x);
        encoder.i8(self.move_z);
        encoder.boolean(self.jump);
        encoder.f32(self.yaw);
        encoder.f32(self.pitch);
        encoder.boolean(self.mining);
        encoder.boolean(self.eating);
        encoder.boolean(self.sprinting);
        encoder.boolean(self.sneaking);
        encoder
            .finish()
            .expect("validated player input is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let sequence = decoder.u64()?;
        let move_x = decoder.i8()?;
        let move_z = decoder.i8()?;
        let jump = decoder.boolean()?;
        let yaw = decoder.f32()?;
        let pitch = decoder.f32()?;
        let mining = decoder.boolean()?;
        let eating = decoder.boolean()?;
        let sprinting = decoder.boolean()?;
        let sneaking = decoder.boolean()?;
        decoder.done()?;
        Self::new(
            sequence, move_x, move_z, jump, yaw, pitch, mining, eating, sprinting, sneaking,
        )
    }
}
