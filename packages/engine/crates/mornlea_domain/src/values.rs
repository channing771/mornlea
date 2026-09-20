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
