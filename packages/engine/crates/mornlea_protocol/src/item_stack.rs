//! Fixed 5-byte item stack wire value shared by every inventory-carrying
//! packet family.
//!
//! The layout is `u16` item, `u8` count, `u16` durability in that order. It
//! encodes an authoritative slot, so this module also owns the registered
//! item table the Go side uses for `ItemStack.Valid`: stack limits, tool and
//! armor durability maxima, and the empty-stack canonical form. Duplicating
//! those rules per family would let the families disagree about which slot
//! values are publishable.

use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;

/// Empty slot item number. A slot holding it must carry a zero count and a
/// zero durability; any other combination is not a canonical empty stack.
pub const ITEM_NONE: u16 = 0;

/// Exclusive upper bound of registered item numbers, copied from the Go
/// `ItemIDMax` sentinel.
pub const ITEM_ID_MAX: u16 = 66;

/// Per-slot upper bound shared by every stackable item.
pub const MAX_STACK_COUNT: u8 = 64;

/// Item numbers referenced by the shared validation tables and by the
/// packet-family fixtures. The numbering itself is frozen wire data: item
/// IDs are protocol-stable values and reordering them would break saved and
/// in-flight bytes.
pub const ITEM_STONE: u16 = 1;
pub const ITEM_DIRT: u16 = 2;
pub const ITEM_GRASS: u16 = 3;
pub const ITEM_STONE_BRICK: u16 = 4;
pub const ITEM_COAL: u16 = 5;
pub const ITEM_RAW_IRON: u16 = 6;
pub const ITEM_IRON_INGOT: u16 = 7;
pub const ITEM_CHEST: u16 = 14;
pub const ITEM_SAND: u16 = 18;
pub const ITEM_GLASS: u16 = 23;
pub const ITEM_CLAY: u16 = 27;
pub const ITEM_BRICK: u16 = 24;
pub const ITEM_STICK: u16 = 37;
pub const ITEM_RAW_BEEF: u16 = 53;
pub const ITEM_COOKED_BEEF: u16 = 54;
pub const ITEM_STONE_PICKAXE: u16 = 10;
pub const ITEM_IRON_PICKAXE: u16 = 11;
pub const ITEM_BOW: u16 = 62;

/// Items whose per-slot limit is one because they are held as a single worn
/// or carried piece: tools, broken tool forms, hoes, swords, buckets, armor
/// pieces, and the bow.
const SINGLE_SLOT_ITEMS: [u16; 22] = [
    ITEM_STONE_PICKAXE,
    ITEM_IRON_PICKAXE,
    12,
    13,
    30,
    31,
    32,
    33,
    47,
    48,
    49,
    50,
    51,
    52,
    55,
    56,
    58,
    59,
    60,
    61,
    ITEM_BOW,
    65,
];

/// Durability maxima for the items that carry a durability budget. Broken
/// tool forms deliberately have no entry: they are already spent.
const DURABILITY_MAXIMA: [(u16, u16); 12] = [
    (ITEM_STONE_PICKAXE, 131),
    (ITEM_IRON_PICKAXE, 250),
    (30, 131),
    (31, 250),
    (47, 59),
    (48, 131),
    (49, 250),
    (ITEM_BOW, 120),
    (58, 165),
    (59, 240),
    (60, 225),
    (61, 195),
];

/// Smelting inputs and their single fixed product, copied from the Go
/// `SmeltingOutput` table.
const SMELTING_OUTPUTS: [(u16, u16); 4] = [
    (ITEM_RAW_IRON, ITEM_IRON_INGOT),
    (ITEM_SAND, ITEM_GLASS),
    (ITEM_CLAY, ITEM_BRICK),
    (ITEM_RAW_BEEF, ITEM_COOKED_BEEF),
];

/// Reports the fixed smelting product of a registered furnace input. Unknown
/// inputs have no product and therefore cannot occupy a furnace input slot.
pub fn smelting_output(item: u16) -> Option<u16> {
    SMELTING_OUTPUTS
        .iter()
        .find(|(input, _)| *input == item)
        .map(|(_, output)| *output)
}

/// Reports whether a stack may occupy a furnace output slot. The whitelist
/// covers every product `smelting_output` can produce plus the empty stack.
pub fn valid_furnace_output(stack: ItemStack) -> bool {
    match stack.item() {
        ITEM_NONE => true,
        ITEM_IRON_INGOT | ITEM_GLASS | ITEM_BRICK | ITEM_COOKED_BEEF => true,
        _ => false,
    }
}

/// Reports whether a stack may occupy a furnace input slot: either the empty
/// stack or a registered smelting input.
pub fn valid_furnace_input(stack: ItemStack) -> bool {
    stack.item() == ITEM_NONE || smelting_output(stack.item()).is_some()
}

/// One authoritative slot value. The empty stack is the zero value, so an
/// absent slot needs no sentinel item number.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ItemStack {
    item: u16,
    count: u8,
    durability: u16,
}

impl ItemStack {
    /// The canonical empty stack.
    pub const EMPTY: Self = Self {
        item: ITEM_NONE,
        count: 0,
        durability: 0,
    };

    /// Builds a stack from wire fields.
    ///
    /// Unregistered item numbers are `InvalidEnum`; a non-canonical empty
    /// stack, a zero or over-limit count, and a durability outside the item's
    /// budget are `InvalidRange`. Items without a durability concept must
    /// carry a zero durability.
    pub fn new(item: u16, count: u8, durability: u16) -> Result<Self, ProtocolError> {
        if item == ITEM_NONE {
            if count == 0 && durability == 0 {
                return Ok(Self::EMPTY);
            }
            return Err(ProtocolError::InvalidRange);
        }
        if item >= ITEM_ID_MAX {
            return Err(ProtocolError::InvalidEnum);
        }
        let limit = if SINGLE_SLOT_ITEMS.contains(&item) {
            1
        } else {
            MAX_STACK_COUNT
        };
        if count == 0 || count > limit {
            return Err(ProtocolError::InvalidRange);
        }
        match DURABILITY_MAXIMA
            .iter()
            .find(|(durable, _)| *durable == item)
            .map(|(_, maximum)| *maximum)
        {
            Some(maximum) => {
                if durability < 1 || durability > maximum {
                    return Err(ProtocolError::InvalidRange);
                }
            }
            None => {
                if durability != 0 {
                    return Err(ProtocolError::InvalidRange);
                }
            }
        }
        Ok(Self {
            item,
            count,
            durability,
        })
    }

    pub fn item(self) -> u16 {
        self.item
    }

    pub fn count(self) -> u8 {
        self.count
    }

    pub fn durability(self) -> u16 {
        self.durability
    }

    pub(crate) fn write(&self, encoder: &mut ByteEncoder) {
        encoder.u16(self.item);
        encoder.u8(self.count);
        encoder.u16(self.durability);
    }

    pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<Self, ProtocolError> {
        let item = decoder.u16()?;
        let count = decoder.u8()?;
        let durability = decoder.u16()?;
        Self::new(item, count, durability)
    }
}
