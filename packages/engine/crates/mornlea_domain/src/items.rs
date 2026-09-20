//! Registered item values and the ordinary item-stack slot value.
//!
//! The stack-limit, durability and smelting tables are copied once from the Go
//! core (`packages/shared/core/item.go` and `smelting.go`) and are the single
//! Rust authority for those rules: every function here reads the same tables,
//! so a second copy in another module cannot drift from this one.
//!
//! The equipped broken armor exception is deliberately absent. An armor save
//! slot keeps a worn piece at durability zero as raw historical fidelity, which
//! is a player-save concern; relaxing the ordinary stack rule to match it would
//! also let a spent tool pass as an intact one, so this type rejects a durable
//! item at durability zero exactly as the Go `core.ItemStack.Valid` does.

use crate::identity::DomainError;

/// The absent item number, from the Go `core.ItemNone` zero value.
const ITEM_NONE: u16 = 0;

/// Exclusive upper bound of the registered item numbering, from the Go
/// `core.ItemIDMax` sentinel: every number below it other than `ITEM_NONE` is
/// registered, and the sentinel itself is the first unregistered number.
const ITEM_ID_MAX: u16 = 66;

/// Per-slot count of a stackable item, from the Go `core.MaxStackCount`.
const MAX_STACK_COUNT: u8 = 64;

/// Item numbers that hold at most one per slot, from the Go
/// `core.ItemStackLimit` branch that returns `1`: tools and their spent forms,
/// buckets, the four iron armor pieces, and the bow with its spent form.
const SINGLE_COUNT_ITEMS: [u16; 22] = [
    10, 11, 12, 13, 30, 31, 32, 33, 47, 48, 49, 50, 51, 52, 55, 56, 58, 59, 60, 61, 62, 65,
];

/// Durability maxima by item number, from the Go `core.ItemMaxDurability`
/// including the armor-domain values that table references. The broken tool
/// and armor forms carry no entry because their durability is expressed by the
/// item number itself rather than by a remaining budget.
const DURABILITY_MAXIMA: [(u16, u16); 12] = [
    (10, 131),
    (11, 250),
    (30, 131),
    (31, 250),
    (47, 59),
    (48, 131),
    (49, 250),
    (58, 165),
    (59, 240),
    (60, 225),
    (61, 195),
    (62, 120),
];

/// Smelting inputs and their unique product, from the Go `core.SmeltingOutput`.
/// This one table also defines the product set, so `is_smelting_product` reads
/// the same rows instead of a second whitelist.
const SMELTING_INPUTS: [(u16, u16); 4] = [(6, 7), (18, 23), (27, 24), (53, 54)];

/// Reports the per-slot count limit of one item number.
///
/// An unregistered number, including the absent item and the sentinel and
/// everything above it, has no limit and returns `None`, which is what the Go
/// `core.ItemStackLimit` default arm reports.
pub fn item_stack_limit(item: u16) -> Option<u8> {
    if item == ITEM_NONE || item >= ITEM_ID_MAX {
        return None;
    }
    Some(if SINGLE_COUNT_ITEMS.contains(&item) {
        1
    } else {
        MAX_STACK_COUNT
    })
}

/// Reports the durability budget of one item number.
///
/// Only the items the Go `core.ItemMaxDurability` switch names carry a budget;
/// every other registered number is nondurable and returns `None` so a caller
/// can require a zero durability field instead of clamping it.
pub fn durability_max(item: u16) -> Option<u16> {
    DURABILITY_MAXIMA
        .iter()
        .find(|(id, _)| *id == item)
        .map(|(_, max)| *max)
}

/// Reports the unique smelting product of one item number.
///
/// The mapping is a fixed table rather than a rule, so a new product is a new
/// row and an input with no row is simply not smeltable.
pub fn smelting_output(item: u16) -> Option<u16> {
    SMELTING_INPUTS
        .iter()
        .find(|(input, _)| *input == item)
        .map(|(_, output)| *output)
}

/// Reports whether one item number is a smelting product.
///
/// The set is read from `SMELTING_INPUTS` rather than listed again, so a
/// product that stops being produced stops being accepted here in the same
/// edit.
pub fn is_smelting_product(item: u16) -> bool {
    SMELTING_INPUTS.iter().any(|(_, output)| *output == item)
}

/// One ordinary inventory, crafting, container or drop slot value.
///
/// The zero triple is the canonical empty stack and is accepted, because the
/// authority publishes empty slots as exactly that value in every inventory
/// carrying family. Any other value on the absent item number is rejected,
/// which keeps a non-empty stack from hiding behind the empty form.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ItemStack {
    item: u16,
    count: u8,
    durability: u16,
}

impl ItemStack {
    /// The empty slot value: item, count and durability are all exactly zero.
    pub const EMPTY: Self = Self {
        item: ITEM_NONE,
        count: 0,
        durability: 0,
    };

    /// Wraps a slot value, rejecting anything the Go `core.ItemStack.Valid`
    /// rule rejects: an unregistered item number, a count outside `1..=limit`,
    /// a durable item whose durability is zero or above its budget, and a
    /// nondurable item with a nonzero durability.
    pub fn try_new(item: u16, count: u8, durability: u16) -> Result<Self, DomainError> {
        if item == ITEM_NONE {
            // The absent number admits exactly the empty stack, so a nonzero
            // count and a nonzero durability are different rejections rather
            // than one shared "unknown item".
            if count == 0 && durability == 0 {
                return Ok(Self::EMPTY);
            }
            if count != 0 {
                return Err(DomainError::InvalidCount);
            }
            return Err(DomainError::InvalidDurability);
        }
        let limit = item_stack_limit(item).ok_or(DomainError::InvalidItem)?;
        if count == 0 || count > limit {
            return Err(DomainError::InvalidCount);
        }
        match durability_max(item) {
            // A durable item must be intact: durability zero is the spent
            // armor-slot exception, which this ordinary type does not carry.
            Some(max) if (1..=max).contains(&durability) => Ok(Self {
                item,
                count,
                durability,
            }),
            Some(_) => Err(DomainError::InvalidDurability),
            // A nondurable item must keep the field at zero, otherwise two
            // stacks of the same item would refuse to merge over a field that
            // carries no meaning for them.
            None if durability == 0 => Ok(Self {
                item,
                count,
                durability,
            }),
            None => Err(DomainError::InvalidDurability),
        }
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
}
