use crate::identity::DomainError;

/// Authoritative world dimension. Only overworld (`0`) and depths (`1`) are
/// in the current supported range; any other ID is unknown.
#[derive(Clone, Copy, Debug, Eq, Ord, PartialEq, PartialOrd)]
pub struct Dimension(u8);

impl Dimension {
    pub const OVERWORLD: Self = Self(0);
    pub const DEPTHS: Self = Self(1);

    pub fn new(id: u8) -> Result<Self, DomainError> {
        match id {
            0 | 1 => Ok(Self(id)),
            _ => Err(DomainError::InvalidDimension),
        }
    }

    pub fn get(self) -> u8 {
        self.0
    }
}

/// Selected hotbar index. The valid closed range is `0..COUNT-1`.
#[derive(Clone, Copy, Debug, Eq, Ord, PartialEq, PartialOrd)]
pub struct HotbarSlot(u8);

impl HotbarSlot {
    pub const COUNT: u8 = 9;

    pub fn new(slot: u8) -> Result<Self, DomainError> {
        if slot >= Self::COUNT {
            return Err(DomainError::InvalidHotbarSlot);
        }
        Ok(Self(slot))
    }

    pub fn get(self) -> u8 {
        self.0
    }
}

/// Finite three-component vector used by authoritative positions and
/// velocities.
///
/// Finiteness is the only rule: the components are stored exactly as
/// received, so a caller that needs a canonical direction normalizes before
/// construction and a replay keeps the recorded bit patterns.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct FiniteVec3([f32; 3]);

impl FiniteVec3 {
    pub fn try_new(components: [f32; 3]) -> Result<Self, DomainError> {
        if !components.iter().all(|component| component.is_finite()) {
            return Err(DomainError::NonFiniteRotation);
        }
        Ok(Self(components))
    }

    /// Returns the components as received, including negative zero.
    pub fn get(self) -> [f32; 3] {
        self.0
    }
}

/// Finite look angles shared by the semantic input and event records.
///
/// The angles are stored as received with no clamping, wrapping, or
/// reduction, so a replay preserves the exact IEEE-754 bits, including
/// negative zero. Normalizing here would make a Rust observation disagree
/// with a Go observation for the same input.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct LookAngles {
    yaw: f32,
    pitch: f32,
}

impl LookAngles {
    pub fn try_new(yaw: f32, pitch: f32) -> Result<Self, DomainError> {
        if !yaw.is_finite() || !pitch.is_finite() {
            return Err(DomainError::NonFiniteRotation);
        }
        Ok(Self { yaw, pitch })
    }

    pub fn yaw(self) -> f32 {
        self.yaw
    }

    pub fn pitch(self) -> f32 {
        self.pitch
    }
}
