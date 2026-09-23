//! The fixed 5-byte item stack wire value shared by every inventory-carrying
//! packet family.
//!
//! The layout is `u16` item, `u8` count, `u16` durability in that order, and
//! the value is the domain's checked `ItemStack`, re-exported here so the
//! packet families, the domain events and the save codec all admit the same
//! slot values. The stack limits, durability maxima and smelting table the Go
//! `core.ItemStack.Valid` rule applies live once in `mornlea_domain`; this
//! module is the wire edge — it reads and writes the fixed stride and maps the
//! domain rejection into the protocol error vocabulary — plus the frozen item
//! numbering and the furnace slot predicates, which compose the domain table
//! instead of copying it.

use crate::bytes::{ByteDecoder, ByteEncoder};
use crate::error::ProtocolError;
pub use mornlea_domain::ItemStack;

/// Empty slot item number. A slot holding it must carry a zero count and a
/// zero durability; any other combination is not a canonical empty stack.
pub const ITEM_NONE: u16 = 0;

/// Exclusive upper bound of registered item numbers, the Go `ItemIDMax`
/// sentinel. Item IDs themselves are frozen wire data: the numbering is
/// protocol-stable, so the named constants below stay as they are while the
/// per-item rules come from the domain predicates.
pub const ITEM_ID_MAX: u16 = 66;

/// Item numbers referenced by the shared validation tables and by the
/// packet-family fixtures.
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

/// Wraps wire fields through the domain rule.
///
/// Unregistered item numbers are `InvalidEnum`; the count, durability and
/// canonical-empty rejections are `InvalidRange`, which is the mapping the
/// packet families published before the domain owned the rule.
pub(crate) fn checked(item: u16, count: u8, durability: u16) -> Result<ItemStack, ProtocolError> {
    ItemStack::try_new(item, count, durability).map_err(|error| match error {
        mornlea_domain::DomainError::InvalidItem => ProtocolError::InvalidEnum,
        _ => ProtocolError::InvalidRange,
    })
}

/// Reads one fixed-stride stack and runs the domain rule.
pub(crate) fn read(decoder: &mut ByteDecoder<'_>) -> Result<ItemStack, ProtocolError> {
    let item = decoder.u16()?;
    let count = decoder.u8()?;
    let durability = decoder.u16()?;
    checked(item, count, durability)
}

/// Writes one fixed-stride stack. The value is already checked, so the write
/// cannot fail.
pub(crate) fn write(stack: ItemStack, encoder: &mut ByteEncoder) {
    encoder.u16(stack.item());
    encoder.u8(stack.count());
    encoder.u16(stack.durability());
}

/// Reports the fixed smelting product of a registered furnace input, read from
/// the domain table so the wire predicate and the semantic rule cannot drift.
pub use mornlea_domain::smelting_output;

/// Reports whether a stack may occupy a furnace output slot. The whitelist
/// is the domain's product set plus the empty stack.
pub fn valid_furnace_output(stack: ItemStack) -> bool {
    stack.item() == ITEM_NONE || mornlea_domain::is_smelting_product(stack.item())
}

/// Reports whether a stack may occupy a furnace input slot: either the empty
/// stack or a registered smelting input.
pub fn valid_furnace_input(stack: ItemStack) -> bool {
    stack.item() == ITEM_NONE || mornlea_domain::smelting_output(stack.item()).is_some()
}
