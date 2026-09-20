//! Item and inventory value rules shared by the entity save families.
//!
//! Mirrors the `core` item registry: the stable item numbering plus the stack
//! limit and durability tables that decide whether a persisted slot is a
//! canonical value. Both save codecs and their callers need the same answers,
//! so the rules live in one place inside this crate.

/// Stable global item number. Numbering is protocol-stable and append-only.
pub type ItemId = u16;

/// Empty slot and list terminator.
pub const ITEM_NONE: ItemId = 0;
const ITEM_STONE: ItemId = 1;
const ITEM_DIRT: ItemId = 2;
const ITEM_GRASS: ItemId = 3;
const ITEM_STONE_BRICK: ItemId = 4;
const ITEM_COAL: ItemId = 5;
const ITEM_RAW_IRON: ItemId = 6;
const ITEM_IRON_INGOT: ItemId = 7;
const ITEM_FURNACE: ItemId = 8;
const ITEM_IRON_BLOCK: ItemId = 9;
const ITEM_STONE_PICKAXE: ItemId = 10;
const ITEM_IRON_PICKAXE: ItemId = 11;
const ITEM_BROKEN_STONE_PICKAXE: ItemId = 12;
const ITEM_BROKEN_IRON_PICKAXE: ItemId = 13;
const ITEM_CHEST: ItemId = 14;
const ITEM_LIGHT_BLOCK: ItemId = 15;
const ITEM_COBBLESTONE: ItemId = 16;
const ITEM_SMOOTH_STONE: ItemId = 17;
const ITEM_SAND: ItemId = 18;
const ITEM_GRAVEL: ItemId = 19;
const ITEM_OAK_LOG: ItemId = 20;
const ITEM_OAK_PLANKS: ItemId = 21;
const ITEM_LEAVES: ItemId = 22;
const ITEM_GLASS: ItemId = 23;
const ITEM_BRICK: ItemId = 24;
const ITEM_WHITE_WOOL: ItemId = 25;
const ITEM_ROOF_TILE: ItemId = 26;
const ITEM_CLAY: ItemId = 27;
const ITEM_SNOW_BLOCK: ItemId = 28;
const ITEM_MOSSY_COBBLESTONE: ItemId = 29;
const ITEM_STONE_HOE: ItemId = 30;
const ITEM_IRON_HOE: ItemId = 31;
const ITEM_BROKEN_STONE_HOE: ItemId = 32;
const ITEM_BROKEN_IRON_HOE: ItemId = 33;
const ITEM_WHEAT_SEEDS: ItemId = 34;
const ITEM_WHEAT: ItemId = 35;
const ITEM_BREAD: ItemId = 36;
const ITEM_STICK: ItemId = 37;
const ITEM_WORKBENCH: ItemId = 38;
const ITEM_BONE_MEAL: ItemId = 39;
const ITEM_POTATO: ItemId = 40;
const ITEM_CARROT: ItemId = 41;
const ITEM_POISONOUS_POTATO: ItemId = 42;
const ITEM_DOOR: ItemId = 43;
const ITEM_TORCH: ItemId = 44;
const ITEM_ROTTEN_FLESH: ItemId = 45;
const ITEM_BED: ItemId = 46;
const ITEM_WOODEN_SWORD: ItemId = 47;
const ITEM_STONE_SWORD: ItemId = 48;
const ITEM_IRON_SWORD: ItemId = 49;
const ITEM_BROKEN_WOODEN_SWORD: ItemId = 50;
const ITEM_BROKEN_STONE_SWORD: ItemId = 51;
const ITEM_BROKEN_IRON_SWORD: ItemId = 52;
const ITEM_RAW_BEEF: ItemId = 53;
const ITEM_COOKED_BEEF: ItemId = 54;
const ITEM_EMPTY_BUCKET: ItemId = 55;
const ITEM_WATER_BUCKET: ItemId = 56;
const ITEM_SAPLING: ItemId = 57;
const ITEM_IRON_HELMET: ItemId = 58;
const ITEM_IRON_CHESTPLATE: ItemId = 59;
const ITEM_IRON_LEGGINGS: ItemId = 60;
const ITEM_IRON_BOOTS: ItemId = 61;
const ITEM_BOW: ItemId = 62;
const ITEM_ARROW: ItemId = 63;
const ITEM_BONE: ItemId = 64;
const ITEM_BROKEN_BOW: ItemId = 65;

/// Exclusive upper bound of the legal item numbering.
pub const ITEM_ID_MAX: ItemId = 66;

/// Fixed number of hotbar slots.
pub const HOTBAR_SLOTS: usize = 9;
/// Fixed number of backpack slots beyond the hotbar.
pub const BACKPACK_SLOTS: usize = 27;
/// Maximum stack size for a stackable item.
pub const MAX_STACK_COUNT: u8 = 64;

/// One hotbar or backpack slot. The zero value is an empty slot.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct ItemStack {
    pub item: ItemId,
    pub count: u8,
    /// Only meaningful for tools; other items keep it at zero.
    pub durability: u16,
}

impl ItemStack {
    /// Reports whether the slot value is canonical: an empty slot carries no
    /// item, count, or durability, and a non-empty slot is a registered item
    /// within its stack limit with durability inside the tool range.
    pub fn is_valid(&self) -> bool {
        let Some(limit) = item_stack_limit(self.item) else {
            return self.item == ITEM_NONE && self.count == 0 && self.durability == 0;
        };
        if self.count == 0 || self.count > limit {
            return false;
        }
        match item_max_durability(self.item) {
            None => self.durability == 0,
            Some(max) => self.durability >= 1 && self.durability <= max,
        }
    }
}

/// The fixed-capacity hotbar. `selected` must be inside `0..HOTBAR_SLOTS`.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct Hotbar {
    pub selected: u8,
    pub slots: [ItemStack; HOTBAR_SLOTS],
}

impl Hotbar {
    /// Reports whether the selection index and every slot value are canonical.
    pub fn is_valid(&self) -> bool {
        self.selected < HOTBAR_SLOTS as u8 && self.slots.iter().all(ItemStack::is_valid)
    }
}

/// The complete inventory state: hotbar plus the fixed-size backpack.
#[derive(Clone, Copy, Debug, Default, Eq, PartialEq)]
pub struct Inventory {
    pub hotbar: Hotbar,
    pub backpack: [ItemStack; BACKPACK_SLOTS],
}

impl Inventory {
    /// Reports whether the hotbar and every backpack slot are canonical.
    pub fn is_valid(&self) -> bool {
        self.hotbar.is_valid() && self.backpack.iter().all(ItemStack::is_valid)
    }
}

/// Returns the per-slot stack limit for `item`, or `None` for an unknown item.
pub fn item_stack_limit(item: ItemId) -> Option<u8> {
    match item {
        ITEM_STONE
        | ITEM_DIRT
        | ITEM_GRASS
        | ITEM_STONE_BRICK
        | ITEM_COAL
        | ITEM_RAW_IRON
        | ITEM_IRON_INGOT
        | ITEM_FURNACE
        | ITEM_IRON_BLOCK
        | ITEM_CHEST
        | ITEM_LIGHT_BLOCK
        | ITEM_COBBLESTONE
        | ITEM_SMOOTH_STONE
        | ITEM_SAND
        | ITEM_GRAVEL
        | ITEM_OAK_LOG
        | ITEM_OAK_PLANKS
        | ITEM_LEAVES
        | ITEM_GLASS
        | ITEM_BRICK
        | ITEM_WHITE_WOOL
        | ITEM_ROOF_TILE
        | ITEM_CLAY
        | ITEM_SNOW_BLOCK
        | ITEM_MOSSY_COBBLESTONE
        | ITEM_WHEAT_SEEDS
        | ITEM_WHEAT
        | ITEM_BREAD
        | ITEM_STICK
        | ITEM_WORKBENCH
        | ITEM_BONE_MEAL
        | ITEM_POTATO
        | ITEM_CARROT
        | ITEM_POISONOUS_POTATO
        | ITEM_DOOR
        | ITEM_TORCH
        | ITEM_ROTTEN_FLESH
        | ITEM_BED
        | ITEM_RAW_BEEF
        | ITEM_COOKED_BEEF
        | ITEM_SAPLING
        | ITEM_ARROW
        | ITEM_BONE => Some(MAX_STACK_COUNT),
        ITEM_STONE_PICKAXE
        | ITEM_IRON_PICKAXE
        | ITEM_BROKEN_STONE_PICKAXE
        | ITEM_BROKEN_IRON_PICKAXE
        | ITEM_STONE_HOE
        | ITEM_IRON_HOE
        | ITEM_BROKEN_STONE_HOE
        | ITEM_BROKEN_IRON_HOE
        | ITEM_WOODEN_SWORD
        | ITEM_STONE_SWORD
        | ITEM_IRON_SWORD
        | ITEM_BROKEN_WOODEN_SWORD
        | ITEM_BROKEN_STONE_SWORD
        | ITEM_BROKEN_IRON_SWORD
        | ITEM_EMPTY_BUCKET
        | ITEM_WATER_BUCKET
        // Armor pieces stack exactly like tools: one worn at a time.
        | ITEM_IRON_HELMET
        | ITEM_IRON_CHESTPLATE
        | ITEM_IRON_LEGGINGS
        | ITEM_IRON_BOOTS
        | ITEM_BOW
        | ITEM_BROKEN_BOW => Some(1),
        _ => None,
    }
}

/// Returns the durability ceiling for `item`, or `None` when it has none.
pub fn item_max_durability(item: ItemId) -> Option<u16> {
    match item {
        ITEM_STONE_PICKAXE => Some(131),
        ITEM_IRON_PICKAXE => Some(250),
        ITEM_WOODEN_SWORD => Some(59),
        ITEM_STONE_SWORD => Some(131),
        ITEM_IRON_SWORD => Some(250),
        ITEM_BOW => Some(120),
        // Hoes take the same ceiling as the pick of the same material: both
        // spend exactly one point per successful action.
        ITEM_STONE_HOE => Some(131),
        ITEM_IRON_HOE => Some(250),
        ITEM_IRON_HELMET => Some(165),
        ITEM_IRON_CHESTPLATE => Some(240),
        ITEM_IRON_LEGGINGS => Some(225),
        ITEM_IRON_BOOTS => Some(195),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::{
        BACKPACK_SLOTS, HOTBAR_SLOTS, ITEM_NONE, ITEM_STONE, Inventory, ItemStack, MAX_STACK_COUNT,
    };

    #[test]
    fn empty_slot_is_the_only_valid_zero_value() {
        assert!(ItemStack::default().is_valid());
        assert!(
            !ItemStack {
                item: ITEM_NONE,
                count: 1,
                durability: 0,
            }
            .is_valid()
        );
        assert!(
            !ItemStack {
                item: ITEM_NONE,
                count: 0,
                durability: 1,
            }
            .is_valid()
        );
    }

    #[test]
    fn stackable_and_tool_slots_use_their_own_bounds() {
        assert!(
            ItemStack {
                item: ITEM_STONE,
                count: MAX_STACK_COUNT,
                durability: 0,
            }
            .is_valid()
        );
        assert!(
            !ItemStack {
                item: ITEM_STONE,
                count: MAX_STACK_COUNT + 1,
                durability: 0,
            }
            .is_valid()
        );
        let tool = ItemStack {
            item: super::ITEM_STONE_PICKAXE,
            count: 1,
            durability: 131,
        };
        assert!(tool.is_valid());
        assert!(
            !ItemStack {
                durability: 0,
                ..tool
            }
            .is_valid()
        );
        assert!(
            !ItemStack {
                durability: 132,
                ..tool
            }
            .is_valid()
        );
    }

    #[test]
    fn unknown_items_are_rejected() {
        assert!(
            !ItemStack {
                item: super::ITEM_ID_MAX,
                count: 1,
                durability: 0,
            }
            .is_valid()
        );
        assert!(
            !ItemStack {
                item: 4_000,
                count: 1,
                durability: 0,
            }
            .is_valid()
        );
    }

    #[test]
    fn inventory_covers_the_fixed_slot_counts() {
        assert_eq!(HOTBAR_SLOTS, 9);
        assert_eq!(BACKPACK_SLOTS, 27);
        assert!(Inventory::default().is_valid());
        assert!(
            !Inventory {
                hotbar: super::Hotbar {
                    selected: 9,
                    slots: [ItemStack::default(); HOTBAR_SLOTS],
                },
                ..Inventory::default()
            }
            .is_valid()
        );
    }
}
