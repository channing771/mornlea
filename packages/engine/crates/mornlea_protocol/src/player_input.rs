use crate::bytes::{ByteDecoder, SliceWriter};
use crate::error::ProtocolError;
use crate::server_hello::publish_packet;
use mornlea_domain::LookAngles;

/// Fixed wire stride of one player input payload: the 8-byte sequence, the two
/// move axes, the five action flags and the two look angles.
const PLAYER_INPUT_WIRE_BYTES: usize = 23;

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
        let input = Self {
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
        };
        input.valid()?;
        Ok(input)
    }

    /// The single value gate shared by `new`, `encode_into` and `decode`.
    ///
    /// The fields are public, so the gate runs on every encode instead of only
    /// at construction: a record mutated into a non-finite rotation after
    /// construction is refused rather than silently published, which is what
    /// keeps the admitted set identical to the Go validator's.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        self.valid()
    }

    fn valid(&self) -> Result<(), ProtocolError> {
        // The domain `LookAngles` rule is the single owner of the finite
        // rotation contract, so this DTO validates through it instead of
        // keeping a second copy of the check that could drift from the
        // semantic record.
        LookAngles::try_new(self.yaw, self.pitch).map_err(|_| ProtocolError::InvalidFloat)?;
        Ok(())
    }

    /// The exact encoded length, which is the fixed payload stride.
    ///
    /// The value gate runs first, so an invalid record reports its rotation
    /// error here instead of reaching a size or capacity decision.
    pub fn encoded_len(&self) -> Result<usize, ProtocolError> {
        self.valid()?;
        Ok(PLAYER_INPUT_WIRE_BYTES)
    }

    /// Publishes the record into a caller-owned buffer and returns the bytes
    /// written.
    ///
    /// The field order is the Go encoder's — sequence, the two move axes, the
    /// jump flag, the two look angles, then the four action flags — and the
    /// destination is tested before the first byte is written, so a short call
    /// leaves every destination byte unchanged. The angles are published as
    /// their exact IEEE-754 bits, so a negative zero survives the round trip.
    pub fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        let length = self.encoded_len()?;
        publish_packet(length, dst, |writer| {
            writer.u64(self.sequence);
            writer.i8(self.move_x);
            writer.i8(self.move_z);
            writer.boolean(self.jump);
            writer.f32(self.yaw);
            writer.f32(self.pitch);
            writer.boolean(self.mining);
            writer.boolean(self.eating);
            writer.boolean(self.sprinting);
            writer.boolean(self.sneaking);
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
