use crate::identity::DomainError;
use crate::values::HotbarSlot;

fn finite_rotation(yaw: f32, pitch: f32) -> Result<(), DomainError> {
    if yaw.is_finite() && pitch.is_finite() {
        Ok(())
    } else {
        Err(DomainError::NonFiniteRotation)
    }
}

/// Semantic player locomotion and held-action bits.
///
/// Rotation must be finite before the record is published. Move axes and
/// boolean action bits keep the protocol field order without adding
/// client-side gameplay settlement.
#[derive(Clone, Debug, PartialEq)]
pub struct PlayerInput {
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
}

impl PlayerInput {
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
    ) -> Result<Self, DomainError> {
        finite_rotation(yaw, pitch)?;
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

    pub fn sequence(&self) -> u64 {
        self.sequence
    }

    pub fn move_x(&self) -> i8 {
        self.move_x
    }

    pub fn move_z(&self) -> i8 {
        self.move_z
    }

    pub fn jump(&self) -> bool {
        self.jump
    }

    pub fn yaw(&self) -> f32 {
        self.yaw
    }

    pub fn pitch(&self) -> f32 {
        self.pitch
    }

    pub fn mining(&self) -> bool {
        self.mining
    }

    pub fn eating(&self) -> bool {
        self.eating
    }

    pub fn sprinting(&self) -> bool {
        self.sprinting
    }

    pub fn sneaking(&self) -> bool {
        self.sneaking
    }
}

/// Semantic place-block intent. Slot and rotation are validated here; the
/// authoritative target block remains a later server decision.
#[derive(Clone, Debug, PartialEq)]
pub struct PlaceBlock {
    sequence: u64,
    yaw: f32,
    pitch: f32,
    slot: HotbarSlot,
}

impl PlaceBlock {
    pub fn new(sequence: u64, yaw: f32, pitch: f32, slot: u8) -> Result<Self, DomainError> {
        finite_rotation(yaw, pitch)?;
        Ok(Self {
            sequence,
            yaw,
            pitch,
            slot: HotbarSlot::new(slot)?,
        })
    }

    pub fn sequence(&self) -> u64 {
        self.sequence
    }

    pub fn yaw(&self) -> f32 {
        self.yaw
    }

    pub fn pitch(&self) -> f32 {
        self.pitch
    }

    pub fn slot(&self) -> HotbarSlot {
        self.slot
    }
}

/// Semantic hotbar selection. Unknown slot IDs fail before publication.
#[derive(Clone, Debug, PartialEq)]
pub struct SelectHotbar {
    sequence: u64,
    slot: HotbarSlot,
}

impl SelectHotbar {
    pub fn new(sequence: u64, slot: u8) -> Result<Self, DomainError> {
        Ok(Self {
            sequence,
            slot: HotbarSlot::new(slot)?,
        })
    }

    pub fn sequence(&self) -> u64 {
        self.sequence
    }

    pub fn slot(&self) -> HotbarSlot {
        self.slot
    }
}

/// Language-neutral input family used by replay ordering.
#[derive(Clone, Debug, PartialEq)]
pub enum SemanticInput {
    Player(PlayerInput),
    Place(PlaceBlock),
    SelectHotbar(SelectHotbar),
}

impl SemanticInput {
    pub fn sequence(&self) -> u64 {
        match self {
            Self::Player(input) => input.sequence(),
            Self::Place(input) => input.sequence(),
            Self::SelectHotbar(input) => input.sequence(),
        }
    }

    pub fn kind(&self) -> &'static str {
        match self {
            Self::Player(_) => "player_input",
            Self::Place(_) => "place_block",
            Self::SelectHotbar(_) => "select_hotbar",
        }
    }
}

/// Orders inputs by sequence, then by kind name, so two independent runtimes
/// emit the same checkpoint schedule from the same bag of records.
pub fn order_inputs(inputs: impl IntoIterator<Item = SemanticInput>) -> Vec<SemanticInput> {
    let mut ordered: Vec<_> = inputs.into_iter().collect();
    ordered.sort_by(|left, right| {
        left.sequence()
            .cmp(&right.sequence())
            .then(left.kind().cmp(right.kind()))
    });
    ordered
}
