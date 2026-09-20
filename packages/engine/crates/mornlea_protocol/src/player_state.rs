//! The authoritative per-tick player state payload.
//!
//! This is the only message that carries the survival and world-time values a
//! session needs, so the module owns the value ranges the Go side pins for
//! health, oxygen, hunger, day phase, weather, season, and armor points. The
//! ranges live here rather than in a shared value module because no other
//! ported family carries them; promoting them later is a one-line move.

use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::combat_hit::MAX_HEALTH;
use crate::error::ProtocolError;
use mornlea_domain::Dimension;

/// Exclusive upper bound of the day phase offset. The display phase is
/// `(world_time_ticks + day_phase_offset) % DAY_PHASE_TICKS_MAX`, so an offset
/// at or above the bound names no reachable phase.
pub const DAY_PHASE_TICKS_MAX: u16 = 24000;

/// Authoritative oxygen upper bound. Legal oxygen is `0..=MAX_OXYGEN_TICKS`.
pub const MAX_OXYGEN_TICKS: u16 = 300;

/// Authoritative hunger upper bound. Legal hunger is `0..=MAX_HUNGER`.
pub const MAX_HUNGER: u8 = 20;

/// Summed armor-point upper bound of the four worn armor slots.
pub const MAX_ARMOR_POINTS: u8 = 20;

/// Weather kinds on the wire: clear, rain, and thunderstorm.
pub const WEATHER_CLEAR: u8 = 0;
pub const WEATHER_RAIN: u8 = 1;
pub const WEATHER_THUNDER: u8 = 2;

/// Seasons on the wire: spring, summer, autumn, and winter.
pub const SEASON_SPRING: u8 = 0;
pub const SEASON_SUMMER: u8 = 1;
pub const SEASON_AUTUMN: u8 = 2;
pub const SEASON_WINTER: u8 = 3;

/// Fixed encoded length of the whole payload. Every field is fixed width, so
/// the length is a frozen contract: any layout change moves it.
pub const PLAYER_STATE_WIRE_BYTES: usize = 93;

/// Absolute block position of the block the player is currently mining.
///
/// The wire carries no world-span check: the authority names a block it has
/// already resolved, and the protocol layer only rejects a non-zero target
/// while mining is inactive.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct BlockPos {
    pub x: i32,
    pub y: i32,
    pub z: i32,
}

impl BlockPos {
    /// The zero position, the only legal target while mining is inactive.
    pub const ZERO: Self = Self { x: 0, y: 0, z: 0 };

    fn is_zero(self) -> bool {
        self == Self::ZERO
    }
}

/// Play PlayerState payload: the authoritative body, survival, and world-time
/// values of one session for one tick.
#[derive(Clone, Debug, PartialEq)]
pub struct PlayerState {
    pub server_tick: u64,
    pub last_input_sequence: u64,
    pub dimension: Dimension,
    pub position: [f32; 3],
    pub velocity: [f32; 3],
    pub yaw: f32,
    pub pitch: f32,
    pub on_ground: bool,
    pub ready: bool,
    pub reset: bool,
    pub mining_active: bool,
    pub mining_target: BlockPos,
    pub mining_progress_ticks: u16,
    pub mining_required_ticks: u16,
    pub mining_harvestable: bool,
    pub health: u8,
    pub oxygen: u16,
    pub hunger: u8,
    pub saturation_zero: bool,
    pub day_phase_offset: u16,
    pub world_time_ticks: u64,
    pub weather_kind: u8,
    pub season: u8,
    pub season_progress: u8,
    pub temperature: i8,
    pub armor_points: u8,
}

impl PlayerState {
    pub const PACKET_ID: u32 = 3;

    /// Builds a validated player state.
    ///
    /// Health, oxygen, and hunger accept zero because a dead or fully drained
    /// player is still a publishable state. The weather, season, and armor
    /// bytes are rejected outright when out of range instead of being clamped,
    /// matching the Go decision that a wire value outside the authoritative
    /// domain is a protocol violation rather than a normalizable reading.
    /// Season progress and temperature have no sub-range: they are a whole
    /// `u8` and an `i8` respectively.
    pub fn new(
        server_tick: u64,
        last_input_sequence: u64,
        dimension: Dimension,
        position: [f32; 3],
        velocity: [f32; 3],
        yaw: f32,
        pitch: f32,
        on_ground: bool,
        ready: bool,
        reset: bool,
        mining_active: bool,
        mining_target: BlockPos,
        mining_progress_ticks: u16,
        mining_required_ticks: u16,
        mining_harvestable: bool,
        health: u8,
        oxygen: u16,
        hunger: u8,
        saturation_zero: bool,
        day_phase_offset: u16,
        world_time_ticks: u64,
        weather_kind: u8,
        season: u8,
        season_progress: u8,
        temperature: i8,
        armor_points: u8,
    ) -> Result<Self, ProtocolError> {
        let state = Self {
            server_tick,
            last_input_sequence,
            dimension,
            position,
            velocity,
            yaw,
            pitch,
            on_ground,
            ready,
            reset,
            mining_active,
            mining_target,
            mining_progress_ticks,
            mining_required_ticks,
            mining_harvestable,
            health,
            oxygen,
            hunger,
            saturation_zero,
            day_phase_offset,
            world_time_ticks,
            weather_kind,
            season,
            season_progress,
            temperature,
            armor_points,
        };
        state.validate()?;
        Ok(state)
    }

    /// The single validation gate shared by `new` and `decode`.
    ///
    /// The mining block is validated as one unit: an inactive block must be
    /// entirely empty so a client never has to guess whether a stale target
    /// still applies, and an active block must report progress strictly below
    /// the requirement so a completed swing is published as inactive instead.
    pub fn validate(&self) -> Result<(), ProtocolError> {
        if !self
            .position
            .iter()
            .chain(self.velocity.iter())
            .all(|value| value.is_finite())
            || !self.yaw.is_finite()
            || !self.pitch.is_finite()
        {
            return Err(ProtocolError::InvalidFloat);
        }
        if self.health > MAX_HEALTH {
            return Err(ProtocolError::InvalidRange);
        }
        if self.oxygen > MAX_OXYGEN_TICKS || self.hunger > MAX_HUNGER {
            return Err(ProtocolError::InvalidRange);
        }
        if self.day_phase_offset >= DAY_PHASE_TICKS_MAX {
            return Err(ProtocolError::InvalidRange);
        }
        if self.weather_kind > WEATHER_THUNDER || self.season > SEASON_WINTER {
            return Err(ProtocolError::InvalidEnum);
        }
        if self.armor_points > MAX_ARMOR_POINTS {
            return Err(ProtocolError::InvalidRange);
        }
        if !self.mining_active {
            if !self.mining_target.is_zero()
                || self.mining_progress_ticks != 0
                || self.mining_required_ticks != 0
                || self.mining_harvestable
            {
                return Err(ProtocolError::InvalidRange);
            }
        } else if self.mining_progress_ticks == 0
            || self.mining_progress_ticks >= self.mining_required_ticks
        {
            return Err(ProtocolError::InvalidRange);
        }
        Ok(())
    }

    pub fn encode(&self) -> Vec<u8> {
        let mut encoder = ByteEncoder::new();
        encoder.u64(self.server_tick);
        encoder.u64(self.last_input_sequence);
        encoder.i32(i32::from(self.dimension.get()));
        for value in self.position {
            encoder.f32(value);
        }
        for value in self.velocity {
            encoder.f32(value);
        }
        encoder.f32(self.yaw);
        encoder.f32(self.pitch);
        encoder.boolean(self.on_ground);
        encoder.boolean(self.ready);
        encoder.boolean(self.reset);
        encoder.boolean(self.mining_active);
        encoder.i32(self.mining_target.x);
        encoder.i32(self.mining_target.y);
        encoder.i32(self.mining_target.z);
        encoder.u16(self.mining_progress_ticks);
        encoder.u16(self.mining_required_ticks);
        encoder.boolean(self.mining_harvestable);
        encoder.u8(self.health);
        encoder.u16(self.oxygen);
        encoder.u8(self.hunger);
        encoder.boolean(self.saturation_zero);
        encoder.u16(self.day_phase_offset);
        encoder.u64(self.world_time_ticks);
        encoder.u8(self.weather_kind);
        encoder.u8(self.season);
        encoder.u8(self.season_progress);
        encoder.i8(self.temperature);
        encoder.u8(self.armor_points);
        encoder
            .finish()
            .expect("validated player state is encodable")
    }

    pub fn decode(payload: &[u8]) -> Result<Self, ProtocolError> {
        let mut decoder = ByteDecoder::new(payload);
        let server_tick = decoder.u64()?;
        let last_input_sequence = decoder.u64()?;
        let dimension = Dimension::new(u8::try_from(decoder.i32()?).unwrap_or(u8::MAX))
            .map_err(|_| ProtocolError::InvalidEnum)?;
        let mut position = [0f32; 3];
        for value in &mut position {
            *value = decoder.f32()?;
        }
        let mut velocity = [0f32; 3];
        for value in &mut velocity {
            *value = decoder.f32()?;
        }
        let yaw = decoder.f32()?;
        let pitch = decoder.f32()?;
        let on_ground = decoder.boolean()?;
        let ready = decoder.boolean()?;
        let reset = decoder.boolean()?;
        let mining_active = decoder.boolean()?;
        let mining_target = BlockPos {
            x: decoder.i32()?,
            y: decoder.i32()?,
            z: decoder.i32()?,
        };
        let mining_progress_ticks = decoder.u16()?;
        let mining_required_ticks = decoder.u16()?;
        let mining_harvestable = decoder.boolean()?;
        let health = decoder.u8()?;
        let oxygen = decoder.u16()?;
        let hunger = decoder.u8()?;
        let saturation_zero = decoder.boolean()?;
        let day_phase_offset = decoder.u16()?;
        let world_time_ticks = decoder.u64()?;
        let weather_kind = decoder.u8()?;
        let season = decoder.u8()?;
        let season_progress = decoder.u8()?;
        let temperature = decoder.i8()?;
        let armor_points = decoder.u8()?;
        decoder.done()?;
        Self::new(
            server_tick,
            last_input_sequence,
            dimension,
            position,
            velocity,
            yaw,
            pitch,
            on_ground,
            ready,
            reset,
            mining_active,
            mining_target,
            mining_progress_ticks,
            mining_required_ticks,
            mining_harvestable,
            health,
            oxygen,
            hunger,
            saturation_zero,
            day_phase_offset,
            world_time_ticks,
            weather_kind,
            season,
            season_progress,
            temperature,
            armor_points,
        )
    }
}
