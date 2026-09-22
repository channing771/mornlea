//! The inventory, crafting and container publications.
//!
//! These are the records an authoritative session publishes to the one player
//! that owns them: the complete inventory, the current crafting grid, the
//! furnace or chest a player is viewing, and the notification that a viewed
//! container went away. Every rule below is the Go `protocol` validator's rule
//! for the same record, so a record this crate admits is a record the protocol
//! layer admits and the other way around.
//!
//! Three Go rules deliberately stay out of this file. A container reference's
//! wire dimension is validated by the protocol conversion before a
//! `ContainerRef` exists, so the domain value carries no dimension that could
//! disagree; the per-chunk slot counts and the nonzero generation are enforced
//! by `ContainerRef` itself; and every slot value is already a validated
//! `ItemStack`, so the stack rules are not restated here. No timer consistency
//! rule relating the progress, the burn time and the three stacks exists in
//! the Go validator, and none is invented here.
//!
//! No record carries an equipped-armor array. Armor slots are a player-save
//! concern the inventory publication does not mirror, exactly as the private
//! player publication carries no inventory.

use crate::identity::DomainError;
use crate::items::{ItemStack, is_smelting_product, smelting_output};
use crate::locations::{ContainerKind, ContainerRef};
use crate::values::HotbarSlot;

/// Fixed hotbar slots one inventory publishes, from the Go `core.HotbarSlots`.
const HOTBAR_SLOTS: usize = 9;

/// Fixed backpack slots one inventory publishes, from the Go
/// `core.BackpackSlots`.
const BACKPACK_SLOTS: usize = 27;

/// Fixed crafting grid slots one crafting state publishes, from the Go
/// `core.CraftingGridSlots`.
const CRAFTING_GRID_SLOTS: usize = 9;

/// Fixed chest slots one chest state publishes, from the Go `core.ChestSlots`.
const CHEST_SLOTS: usize = 27;

/// Side length of the personal crafting grid, from the Go
/// `craftingGridSizePersonal`.
///
/// A personal grid publishes four usable slots, so the five cells beyond them
/// have to stay empty on the wire as well as in this record.
const PERSONAL_GRID_SIDE: usize = 2;

/// Furnace smelt requirement, from the Go `core.FurnaceSmeltTicks`.
///
/// The progress has to stay strictly below it: a swing that reached the
/// requirement is finished, and the Go validator publishes a finished furnace
/// with the progress reset rather than with an active one.
const FURNACE_SMELT_TICKS: u8 = 200;

/// Furnace burn maximum, from the Go `core.FurnaceBurnTicks`.
///
/// Unlike the progress bound this one is inclusive: a furnace may report a
/// full burn tank, but not more than one.
const FURNACE_BURN_TICKS: u16 = 1600;

/// The one fuel a furnace accepts beside the empty stack, from the Go
/// `core.ItemCoal`.
const FURNACE_FUEL_COAL: u16 = 5;

/// Parts of one inventory publication.
///
/// The selected index and both arrays are the whole record: the Go packet
/// carries no equipped-armor array, so this record has no field a later node
/// could grow into a second armor mirror.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct InventoryStateParts {
    pub selected: HotbarSlot,
    pub hotbar: [ItemStack; HOTBAR_SLOTS],
    pub backpack: [ItemStack; BACKPACK_SLOTS],
}

/// The complete authoritative item state of one session.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct InventoryState {
    selected: HotbarSlot,
    hotbar: [ItemStack; HOTBAR_SLOTS],
    backpack: [ItemStack; BACKPACK_SLOTS],
}

impl InventoryState {
    /// Wraps one inventory publication.
    ///
    /// Construction is total and named `new` rather than `try_new` because
    /// every part is an already-validated domain value: the selected index is
    /// a `HotbarSlot`, which enforces the Go `Hotbar.Valid` range, and every
    /// slot is a validated `ItemStack`. Together those are the exact Go
    /// `Inventory.Valid` rule, so nothing is left for this constructor to
    /// reject.
    pub fn new(parts: InventoryStateParts) -> Self {
        Self {
            selected: parts.selected,
            hotbar: parts.hotbar,
            backpack: parts.backpack,
        }
    }

    pub fn selected(self) -> HotbarSlot {
        self.selected
    }

    /// The nine hotbar slots in hotbar order.
    pub fn hotbar(&self) -> &[ItemStack] {
        &self.hotbar[..]
    }

    /// The twenty-seven backpack slots in backpack order.
    pub fn backpack(&self) -> &[ItemStack] {
        &self.backpack[..]
    }
}

/// Published crafting grid sizes.
///
/// The Go packet carries the grid side length as `2` or `3`; the domain names
/// the two published grids instead, because the number is a layout detail of
/// the wire while the record's rule is which cells a grid has.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum CraftingSize {
    Personal,
    Workbench,
}

/// Parts of one crafting publication.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CraftingStateParts {
    pub size: CraftingSize,
    pub slots: [ItemStack; CRAFTING_GRID_SLOTS],
    pub output: ItemStack,
}

/// The current crafting grid of one session.
///
/// The record is latest-wins and never broadcast to another session: the grid
/// belongs to the player whose inventory it mirrors, and the output is derived
/// by the authority from the grid rather than declared by the client.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CraftingState {
    size: CraftingSize,
    slots: [ItemStack; CRAFTING_GRID_SLOTS],
    output: ItemStack,
}

impl CraftingState {
    /// Wraps one crafting publication, rejecting a non-empty slot beyond the
    /// personal grid's own cells.
    ///
    /// The wire always carries all nine slots, so a personal grid publishes
    /// the five cells it does not have as empty. The Go validator rejects a
    /// residue there because a client that had to guess whether a stale slot
    /// still applies could disagree with the authority. The size is an
    /// already-closed enum and every slot is an already-validated stack, so
    /// the residue is the only rule left.
    pub fn try_new(parts: CraftingStateParts) -> Result<Self, DomainError> {
        if parts.size == CraftingSize::Personal {
            let usable = PERSONAL_GRID_SIDE * PERSONAL_GRID_SIDE;
            if parts.slots[usable..]
                .iter()
                .any(|stack| *stack != ItemStack::EMPTY)
            {
                return Err(DomainError::InvalidCraftingResidue);
            }
        }
        Ok(Self {
            size: parts.size,
            slots: parts.slots,
            output: parts.output,
        })
    }

    pub fn size(self) -> CraftingSize {
        self.size
    }

    /// The nine grid slots in grid order, the unused personal cells included.
    pub fn slots(&self) -> &[ItemStack] {
        &self.slots[..]
    }

    pub fn output(self) -> ItemStack {
        self.output
    }
}

/// Parts of one furnace publication.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct FurnaceStateParts {
    pub container: ContainerRef,
    pub input: ItemStack,
    pub fuel: ItemStack,
    pub output: ItemStack,
    pub progress_ticks: u8,
    pub burn_ticks: u16,
}

/// The furnace one session is currently viewing.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct FurnaceState {
    container: ContainerRef,
    input: ItemStack,
    fuel: ItemStack,
    output: ItemStack,
    progress_ticks: u8,
    burn_ticks: u16,
}

impl FurnaceState {
    /// Wraps one furnace publication, checking the reference kind, the two
    /// timers and the three slot whitelists.
    ///
    /// The reference has to name a furnace, which is the Go `validFurnaceRef`
    /// kind rule; its slot range and generation are already enforced by
    /// `ContainerRef`. The progress has to stay strictly below the smelt
    /// requirement and the burn time at or below its maximum, which are the Go
    /// `FurnaceSmeltTicks` and `FurnaceBurnTicks` bounds. The input accepts
    /// the empty stack or a registered smelting input, the fuel the empty
    /// stack or coal, and the output the empty stack or a registered smelting
    /// product, which are the Go `validFurnaceInput`, fuel and
    /// `validFurnaceOutput` rules. No relation between the timers and the
    /// stacks is checked, because the Go validator publishes none.
    pub fn try_new(parts: FurnaceStateParts) -> Result<Self, DomainError> {
        if parts.container.kind() != ContainerKind::Furnace {
            return Err(DomainError::InvalidContainerKind);
        }
        if parts.progress_ticks >= FURNACE_SMELT_TICKS || parts.burn_ticks > FURNACE_BURN_TICKS {
            return Err(DomainError::InvalidFurnaceTimers);
        }
        if !furnace_input_admits(parts.input)
            || !furnace_fuel_admits(parts.fuel)
            || !furnace_output_admits(parts.output)
        {
            return Err(DomainError::InvalidFurnaceSlot);
        }
        Ok(Self {
            container: parts.container,
            input: parts.input,
            fuel: parts.fuel,
            output: parts.output,
            progress_ticks: parts.progress_ticks,
            burn_ticks: parts.burn_ticks,
        })
    }

    pub fn container(self) -> ContainerRef {
        self.container
    }

    pub fn input(self) -> ItemStack {
        self.input
    }

    pub fn fuel(self) -> ItemStack {
        self.fuel
    }

    pub fn output(self) -> ItemStack {
        self.output
    }

    pub fn progress_ticks(self) -> u8 {
        self.progress_ticks
    }

    pub fn burn_ticks(self) -> u16 {
        self.burn_ticks
    }
}

/// Reports whether one stack may sit in the furnace input slot.
///
/// The whitelist is read from the shared smelting table rather than listed
/// again, so an input that stops being smeltable stops being admitted in the
/// same edit that changes the table.
fn furnace_input_admits(stack: ItemStack) -> bool {
    stack == ItemStack::EMPTY || smelting_output(stack.item()).is_some()
}

/// Reports whether one stack may sit in the furnace fuel slot.
fn furnace_fuel_admits(stack: ItemStack) -> bool {
    stack == ItemStack::EMPTY || stack.item() == FURNACE_FUEL_COAL
}

/// Reports whether one stack may sit in the furnace output slot.
///
/// The whitelist has to cover every product the smelting table publishes: the
/// authority writes a product into this slot in the same tick it sends the
/// state, so a product missing here would make the viewer's session reject a
/// record the authority just published.
fn furnace_output_admits(stack: ItemStack) -> bool {
    stack == ItemStack::EMPTY || is_smelting_product(stack.item())
}

/// Parts of one chest publication.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ChestStateParts {
    pub container: ContainerRef,
    pub items: [ItemStack; CHEST_SLOTS],
}

/// The chest one session is currently viewing.
///
/// A chest slot accepts any registered item, so the only rule beyond the
/// already-validated stacks is the reference kind.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ChestState {
    container: ContainerRef,
    items: [ItemStack; CHEST_SLOTS],
}

impl ChestState {
    /// Wraps one chest publication, rejecting a reference that does not name a
    /// chest.
    ///
    /// The kind rule is the Go `validChestRef` rule; the slot range and the
    /// generation are already enforced by `ContainerRef`, and every item is
    /// already a validated stack, so the kind is the only relation left.
    pub fn try_new(parts: ChestStateParts) -> Result<Self, DomainError> {
        if parts.container.kind() != ContainerKind::Chest {
            return Err(DomainError::InvalidContainerKind);
        }
        Ok(Self {
            container: parts.container,
            items: parts.items,
        })
    }

    pub fn container(self) -> ContainerRef {
        self.container
    }

    /// The twenty-seven chest slots in chest order.
    pub fn items(&self) -> &[ItemStack] {
        &self.items[..]
    }
}

/// Notification that the container one session was viewing is gone.
///
/// The Go packet is named `FurnaceEnd` for historical reasons but carries both
/// container kinds, so this record permits either one and keeps the neutral
/// name. Construction is total because a `ContainerRef` is already validated:
/// the kind is one of the two published variants and the slot and generation
/// bounds were checked when the reference was built.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ContainerClosed {
    container: ContainerRef,
}

impl ContainerClosed {
    pub fn new(container: ContainerRef) -> Self {
        Self { container }
    }

    pub fn container(self) -> ContainerRef {
        self.container
    }
}
