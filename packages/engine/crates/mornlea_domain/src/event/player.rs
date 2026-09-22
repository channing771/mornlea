//! The private per-session player publication.
//!
//! `PlayerState` is the record an authoritative tick publishes to one session
//! about its own player: where the body is, how it is doing, and what the
//! world around it looks like. It is deliberately not a world mirror. The
//! inventory, crafting, furnace and chest publications are separate records
//! owned by later nodes, so this aggregate carries no slot array and no
//! equipped-armor field, and a later node cannot grow it into a second
//! inventory mirror.
//!
//! Every value rule below is the Go `protocol.PlayerState.Validate` rule, so a
//! record this crate admits is a record the protocol layer admits.

use crate::identity::DomainError;
use crate::locations::BlockPos;
use crate::values::{Dimension, FiniteVec3, LookAngles};

/// Authoritative health maximum, from the Go `core.MaxHealth`.
///
/// The combat damage bound reuses it because that is the Go rule, and the
/// survival health bound reuses it because a player cannot hold more.
pub(crate) const MAX_HEALTH: u8 = 20;

/// Authoritative oxygen maximum, from the Go `core.MaxOxygenTicks`.
const MAX_OXYGEN: u16 = 300;

/// Authoritative hunger maximum, from the Go `core.MaxHunger`.
const MAX_HUNGER: u8 = 20;

/// Authoritative armor-points maximum, from the Go `core.MaxArmorPoints`.
///
/// The maximum is deliberately above the current iron set's total so a
/// higher-tier set has room, which is why the wire bound is this value rather
/// than the sum of what exists today.
const MAX_ARMOR_POINTS: u8 = 20;

/// Length of one display day in ticks, from the Go `core.DayLengthTicks`.
///
/// The day phase offset has to stay strictly below it: an offset at or above
/// the day length no longer names a phase inside the cycle it shifts.
const DAY_LENGTH: u16 = 24000;

/// Published weather kinds.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum Weather {
    Clear,
    Rain,
    Thunder,
}

impl Weather {
    /// Wraps one raw wire kind, rejecting anything outside the three
    /// published values.
    pub fn try_new(id: u8) -> Result<Self, DomainError> {
        match id {
            0 => Ok(Self::Clear),
            1 => Ok(Self::Rain),
            2 => Ok(Self::Thunder),
            _ => Err(DomainError::InvalidWeather),
        }
    }

    /// Returns the wire value this kind publishes, from an explicit match
    /// rather than a discriminant cast.
    pub fn wire_id(self) -> u8 {
        match self {
            Self::Clear => 0,
            Self::Rain => 1,
            Self::Thunder => 2,
        }
    }
}

/// Published seasons.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum Season {
    Spring,
    Summer,
    Autumn,
    Winter,
}

impl Season {
    /// Wraps one raw wire season, rejecting anything outside the four
    /// published values.
    pub fn try_new(id: u8) -> Result<Self, DomainError> {
        match id {
            0 => Ok(Self::Spring),
            1 => Ok(Self::Summer),
            2 => Ok(Self::Autumn),
            3 => Ok(Self::Winter),
            _ => Err(DomainError::InvalidSeason),
        }
    }

    /// Returns the wire value this season publishes, from an explicit match
    /// rather than a discriminant cast.
    pub fn wire_id(self) -> u8 {
        match self {
            Self::Spring => 0,
            Self::Summer => 1,
            Self::Autumn => 2,
            Self::Winter => 3,
        }
    }
}

/// Body motion of one player at one tick.
///
/// The two vectors carry no rule of their own beyond finiteness, which
/// `FiniteVec3` enforces before the parts exist, so construction is total.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct MotionState {
    position: FiniteVec3,
    velocity: FiniteVec3,
    on_ground: bool,
}

/// Parts of one motion state: two already-validated vectors and the ground
/// bit.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct MotionStateParts {
    pub position: FiniteVec3,
    pub velocity: FiniteVec3,
    pub on_ground: bool,
}

impl MotionState {
    pub fn new(parts: MotionStateParts) -> Self {
        Self {
            position: parts.position,
            velocity: parts.velocity,
            on_ground: parts.on_ground,
        }
    }

    pub fn position(self) -> FiniteVec3 {
        self.position
    }

    pub fn velocity(self) -> FiniteVec3 {
        self.velocity
    }

    pub fn on_ground(self) -> bool {
        self.on_ground
    }
}

/// One swing in progress against one block.
///
/// The fields are private because the record carries a rule of its own: a
/// progress that is zero or at or above the requirement describes no swing.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ActiveMining {
    target: BlockPos,
    progress: u16,
    required: u16,
    harvestable: bool,
}

/// Parts of one active mining block.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct ActiveMiningParts {
    pub target: BlockPos,
    pub progress: u16,
    pub required: u16,
    pub harvestable: bool,
}

impl ActiveMining {
    /// Wraps one active mining block, requiring `0 < progress < required`.
    ///
    /// The upper bound is strict because the Go validator rejects a progress
    /// at or above the requirement: a completed swing is published as an
    /// inactive block instead of an active one that already finished. The
    /// target coordinates carry no rule of their own, matching the Go
    /// player-state validator.
    pub fn try_new(parts: ActiveMiningParts) -> Result<Self, DomainError> {
        if parts.progress == 0 || parts.progress >= parts.required {
            return Err(DomainError::InvalidMiningState);
        }
        Ok(Self {
            target: parts.target,
            progress: parts.progress,
            required: parts.required,
            harvestable: parts.harvestable,
        })
    }

    pub fn target(self) -> BlockPos {
        self.target
    }

    pub fn progress(self) -> u16 {
        self.progress
    }

    pub fn required(self) -> u16 {
        self.required
    }

    pub fn harvestable(self) -> bool {
        self.harvestable
    }
}

/// The mining union of one player state.
#[derive(Clone, Copy, Debug, PartialEq)]
pub enum MiningState {
    Idle,
    Active(ActiveMining),
}

/// Parts of one wire mining block: the active bit and the four fields the
/// inactive and active members interpret differently.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct MiningStateParts {
    pub active: bool,
    pub target: BlockPos,
    pub progress: u16,
    pub required: u16,
    pub harvestable: bool,
}

impl MiningState {
    /// Converts one wire mining block into the semantic union.
    ///
    /// The conversion is checked rather than inferred. An inactive block has
    /// to be entirely empty — an exact zero target, zero progress and
    /// requirement and a false harvestable flag — because the Go validator
    /// rejects an inactive block that still carries any part of a swing, and a
    /// client that had to guess whether a stale target still applies could
    /// disagree with the authority. An active block goes through the checked
    /// `ActiveMining` constructor.
    pub fn try_new(parts: MiningStateParts) -> Result<Self, DomainError> {
        if !parts.active {
            if parts.target != BlockPos::ORIGIN
                || parts.progress != 0
                || parts.required != 0
                || parts.harvestable
            {
                return Err(DomainError::InvalidMiningState);
            }
            return Ok(Self::Idle);
        }
        Ok(Self::Active(ActiveMining::try_new(ActiveMiningParts {
            target: parts.target,
            progress: parts.progress,
            required: parts.required,
            harvestable: parts.harvestable,
        })?))
    }
}

/// Survival scalars of one player.
///
/// The saturation-zero flag is a presentation hint the authority derives from
/// its own saturation quantity, which never reaches the wire, so the record
/// carries the hint and not the quantity.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct SurvivalState {
    health: u8,
    oxygen: u16,
    hunger: u8,
    saturation_zero: bool,
    armor_points: u8,
}

/// Parts of one survival state.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct SurvivalStateParts {
    pub health: u8,
    pub oxygen: u16,
    pub hunger: u8,
    pub saturation_zero: bool,
    pub armor_points: u8,
}

impl SurvivalState {
    /// Wraps one survival state, rejecting any scalar above the authoritative
    /// maximum the Go core publishes for it.
    ///
    /// The bounds are the Go `ValidHealth` / `ValidOxygen` / `ValidHunger` and
    /// armor-point rules, which reject rather than clamp: a wire value outside
    /// the authoritative domain is a protocol violation, not a value to
    /// silently fold back into range.
    pub fn try_new(parts: SurvivalStateParts) -> Result<Self, DomainError> {
        if parts.health > MAX_HEALTH
            || parts.oxygen > MAX_OXYGEN
            || parts.hunger > MAX_HUNGER
            || parts.armor_points > MAX_ARMOR_POINTS
        {
            return Err(DomainError::InvalidSurvivalValue);
        }
        Ok(Self {
            health: parts.health,
            oxygen: parts.oxygen,
            hunger: parts.hunger,
            saturation_zero: parts.saturation_zero,
            armor_points: parts.armor_points,
        })
    }

    pub fn health(self) -> u8 {
        self.health
    }

    pub fn oxygen(self) -> u16 {
        self.oxygen
    }

    pub fn hunger(self) -> u8 {
        self.hunger
    }

    pub fn saturation_zero(self) -> bool {
        self.saturation_zero
    }

    pub fn armor_points(self) -> u8 {
        self.armor_points
    }
}

/// World scalars one player observes at one tick.
///
/// The absolute world time and the display phase offset are separate fields
/// because the offset only shifts the presented phase and never writes back
/// the absolute time. Season progress and temperature are the two full-range
/// scalars: the Go validator accepts every `u8` and `i8` value, so neither
/// carries a sub-range rule here.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct WorldState {
    day_phase_offset: u16,
    world_time_ticks: u64,
    weather: Weather,
    season: Season,
    season_progress: u8,
    temperature: i8,
}

/// Parts of one world state. The weather and season are already-validated
/// enums, so the constructor's only range rule is the day phase offset.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct WorldStateParts {
    pub day_phase_offset: u16,
    pub world_time_ticks: u64,
    pub weather: Weather,
    pub season: Season,
    pub season_progress: u8,
    pub temperature: i8,
}

impl WorldState {
    /// Wraps one world state, rejecting a day phase offset at or above the
    /// length of one display day.
    ///
    /// The bound is exclusive and rejected rather than wrapped: the wire
    /// carries one authoritative value with no compatibility baggage, so an
    /// offset outside the cycle is a protocol violation instead of something
    /// to fold back into range.
    pub fn try_new(parts: WorldStateParts) -> Result<Self, DomainError> {
        if parts.day_phase_offset >= DAY_LENGTH {
            return Err(DomainError::InvalidDayPhaseOffset);
        }
        Ok(Self {
            day_phase_offset: parts.day_phase_offset,
            world_time_ticks: parts.world_time_ticks,
            weather: parts.weather,
            season: parts.season,
            season_progress: parts.season_progress,
            temperature: parts.temperature,
        })
    }

    pub fn day_phase_offset(self) -> u16 {
        self.day_phase_offset
    }

    pub fn world_time_ticks(self) -> u64 {
        self.world_time_ticks
    }

    pub fn weather(self) -> Weather {
        self.weather
    }

    pub fn season(self) -> Season {
        self.season
    }

    pub fn season_progress(self) -> u8 {
        self.season_progress
    }

    pub fn temperature(self) -> i8 {
        self.temperature
    }
}

/// Parts of one player publication.
///
/// Every checked value is already a checked domain value by the time the parts
/// exist: the dimension, the two motion vectors, the look angles, the mining
/// union and the survival and world records each enforce their own rule, so
/// the aggregate has nothing left to reject.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PlayerStateParts {
    pub server_tick: u64,
    pub last_input_sequence: u64,
    pub dimension: Dimension,
    pub motion: MotionState,
    pub look: LookAngles,
    pub ready: bool,
    pub reset: bool,
    pub mining: MiningState,
    pub survival: SurvivalState,
    pub world: WorldState,
}

/// The private per-session player publication.
///
/// Construction is total and named `new` rather than `try_new` for the same
/// reason `PlayerControl::new` is: every field is an already-validated domain
/// value, so there is no rule left for this constructor to enforce. A zero
/// server tick and a zero last input sequence are both legal, because the Go
/// packet accepts them.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct PlayerState {
    server_tick: u64,
    last_input_sequence: u64,
    dimension: Dimension,
    motion: MotionState,
    look: LookAngles,
    ready: bool,
    reset: bool,
    mining: MiningState,
    survival: SurvivalState,
    world: WorldState,
}

impl PlayerState {
    pub fn new(parts: PlayerStateParts) -> Self {
        Self {
            server_tick: parts.server_tick,
            last_input_sequence: parts.last_input_sequence,
            dimension: parts.dimension,
            motion: parts.motion,
            look: parts.look,
            ready: parts.ready,
            reset: parts.reset,
            mining: parts.mining,
            survival: parts.survival,
            world: parts.world,
        }
    }

    pub fn server_tick(self) -> u64 {
        self.server_tick
    }

    pub fn last_input_sequence(self) -> u64 {
        self.last_input_sequence
    }

    pub fn dimension(self) -> Dimension {
        self.dimension
    }

    pub fn motion(self) -> MotionState {
        self.motion
    }

    pub fn look(self) -> LookAngles {
        self.look
    }

    pub fn ready(self) -> bool {
        self.ready
    }

    pub fn reset(self) -> bool {
        self.reset
    }

    pub fn mining(self) -> MiningState {
        self.mining
    }

    pub fn survival(self) -> SurvivalState {
        self.survival
    }

    pub fn world(self) -> WorldState {
        self.world
    }
}
