//! Inventory, container and stack-splitting command payloads.
//!
//! These records name slots, never items: a move says which unified slot the
//! client wants to take from and which one it wants to land in, and the
//! authority decides whether the stack exists, fits and is allowed there. The
//! bounds below are therefore exactly the static value rules the wire protocol
//! publishes, and deliberately nothing more.
//!
//! Two of those rules are asymmetric on purpose. `CraftingMove` refuses two
//! inventory-region ends because the inventory command already covers that
//! case, and `ContainerMove` refuses the furnace output slot as a destination
//! because that slot is reserved for taking the smelting product. The partial,
//! quick-move and drop-stack commands publish neither rule, so they do not
//! inherit them: reusing the stricter validators here would reject a payload
//! the protocol still admits, which would turn a move the authority should
//! judge into one the client never learns about.

use crate::identity::DomainError;
use crate::locations::{ChunkPos, ContainerKind, ContainerRef};

/// Fixed player inventory slots, from the Go `core.InventorySlots`.
const INVENTORY_SLOTS: u8 = 36;

/// Fixed crafting grid slots, from the Go `core.CraftingGridSlots`.
const CRAFTING_GRID_SLOTS: u8 = 9;

/// Unified crafting view slots: grid `0..8` plus backpack `9..44`, from the Go
/// `GridCraftingViewSlots`.
const CRAFTING_VIEW_SLOTS: u8 = CRAFTING_GRID_SLOTS + INVENTORY_SLOTS;

/// Unified furnace view slots: inventory `0..35`, input `36`, fuel `37` and
/// output `38`, from the Go `core.FurnaceViewSlots`.
const FURNACE_VIEW_SLOTS: u8 = INVENTORY_SLOTS + 3;

/// Unified chest view slots: inventory `0..35` plus chest `36..62`, from the Go
/// `core.ChestViewSlots`.
const CHEST_VIEW_SLOTS: u8 = INVENTORY_SLOTS + 27;

/// Furnace output slot, from the Go `core.FurnaceOutputSlot`.
const FURNACE_OUTPUT_SLOT: u8 = INVENTORY_SLOTS + 2;

/// Whole-stack move inside the fixed player inventory.
///
/// Both indices are unified inventory slots, so a move can name a hotbar slot
/// or a backpack slot on either end. The moved stack, the capacity of the
/// destination and the resulting selection stay server-owned.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct InventoryMove {
    from: u8,
    to: u8,
}

impl InventoryMove {
    /// Wraps one inventory move, rejecting an index at or above the fixed slot
    /// count and a move that names one slot as both ends.
    pub fn try_new(from: u8, to: u8) -> Result<Self, DomainError> {
        if from >= INVENTORY_SLOTS || to >= INVENTORY_SLOTS {
            return Err(DomainError::InvalidSlot);
        }
        if from == to {
            return Err(DomainError::SourceEqualsTarget);
        }
        Ok(Self { from, to })
    }

    pub fn from(self) -> u8 {
        self.from
    }

    pub fn to(self) -> u8 {
        self.to
    }
}

/// Whole-stack move between the crafting grid and the backpack.
///
/// The unified view keeps the grid at `0..8` and the backpack at `9..44`, so a
/// move can cross the boundary in either direction. A move with both ends in
/// the backpack region is refused here because `InventoryMove` already covers
/// it, and the unused cells of a personal grid are a size-dependent rejection
/// the network layer cannot know about.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct CraftingMove {
    from: u8,
    to: u8,
}

impl CraftingMove {
    /// Wraps one crafting move, rejecting an index outside the unified view, a
    /// move that names one slot as both ends, and a move whose both ends are
    /// inside the backpack region.
    pub fn try_new(from: u8, to: u8) -> Result<Self, DomainError> {
        if from >= CRAFTING_VIEW_SLOTS || to >= CRAFTING_VIEW_SLOTS {
            return Err(DomainError::InvalidSlot);
        }
        if from == to {
            return Err(DomainError::SourceEqualsTarget);
        }
        if from >= CRAFTING_GRID_SLOTS && to >= CRAFTING_GRID_SLOTS {
            return Err(DomainError::CraftingMoveInsideInventory);
        }
        Ok(Self { from, to })
    }

    pub fn from(self) -> u8 {
        self.from
    }

    pub fn to(self) -> u8 {
        self.to
    }
}

/// Whole-stack move inside one container's unified view.
///
/// The reference is taken as the raw wire fields rather than as an assembled
/// `ContainerRef` so the reference is validated before the slot rules: a
/// malformed reference has to be reported as a reference rather than as a slot
/// the client could fix by picking another index.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct ContainerMove {
    container: ContainerRef,
    from: u8,
    to: u8,
}

impl ContainerMove {
    /// Wraps one container move, rejecting a malformed container reference
    /// first, then a move that names one slot as both ends, then an index
    /// outside the addressed kind's unified view, and finally the furnace
    /// output slot as a destination.
    pub fn try_new(
        chunk: ChunkPos,
        kind: ContainerKind,
        slot: u8,
        generation: u32,
        from: u8,
        to: u8,
    ) -> Result<Self, DomainError> {
        let container = ContainerRef::try_new(chunk, kind, slot, generation)?;
        if from == to {
            return Err(DomainError::SourceEqualsTarget);
        }
        let bound = match kind {
            ContainerKind::Furnace => FURNACE_VIEW_SLOTS,
            ContainerKind::Chest => CHEST_VIEW_SLOTS,
        };
        if from >= bound || to >= bound {
            return Err(DomainError::InvalidSlot);
        }
        if kind == ContainerKind::Furnace && to == FURNACE_OUTPUT_SLOT {
            return Err(DomainError::FurnaceOutputAsTarget);
        }
        Ok(Self {
            container,
            from,
            to,
        })
    }

    pub fn container(self) -> ContainerRef {
        self.container
    }

    pub fn from(self) -> u8 {
        self.from
    }

    pub fn to(self) -> u8 {
        self.to
    }
}

/// Unified view a stack-splitting command addresses.
///
/// The view replaces the wire's raw view number plus sentinel container
/// reference: the inventory and crafting views carry no container at all, and
/// the container view carries a reference that was already validated when it
/// was built. A zero or otherwise invalid reference therefore cannot name
/// "no container", because absence is the variant itself.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub enum StackView {
    Inventory,
    Crafting,
    Container(ContainerRef),
}

impl StackView {
    /// The exclusive upper bound of a unified slot index in this view.
    pub fn slot_bound(self) -> u8 {
        match self {
            Self::Inventory => INVENTORY_SLOTS,
            Self::Crafting => CRAFTING_VIEW_SLOTS,
            Self::Container(container) => match container.kind() {
                ContainerKind::Furnace => FURNACE_VIEW_SLOTS,
                ContainerKind::Chest => CHEST_VIEW_SLOTS,
            },
        }
    }

    /// The container this view addresses, or `None` when the view names no
    /// container at all.
    pub fn container(self) -> Option<ContainerRef> {
        match self {
            Self::Container(container) => Some(container),
            Self::Inventory | Self::Crafting => None,
        }
    }
}

/// Partial move of one stack between two slots of one view.
///
/// The moved amount is derived by the authority from the source stack, so the
/// `single` flag only chooses between the half and the single-item derivation
/// and the wire carries no count. This record publishes the view bounds and
/// the distinct-slot rule and nothing else: it deliberately does not reuse the
/// stricter `CraftingMove` and `ContainerMove` rules, because the protocol
/// admits two inventory-region crafting indices and a furnace-output
/// destination here and leaves the item and slot judgment to the authority.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct PartialMove {
    view: StackView,
    from: u8,
    to: u8,
    single: bool,
}

impl PartialMove {
    /// Wraps one partial move, rejecting an index outside the view's bound and
    /// a move that names one slot as both ends.
    pub fn try_new(view: StackView, from: u8, to: u8, single: bool) -> Result<Self, DomainError> {
        let bound = view.slot_bound();
        if from >= bound || to >= bound {
            return Err(DomainError::InvalidSlot);
        }
        if from == to {
            return Err(DomainError::SourceEqualsTarget);
        }
        Ok(Self {
            view,
            from,
            to,
            single,
        })
    }

    pub fn view(self) -> StackView {
        self.view
    }

    pub fn from(self) -> u8 {
        self.from
    }

    pub fn to(self) -> u8 {
        self.to
    }

    pub fn single(self) -> bool {
        self.single
    }
}

/// Source slot of a whole-stack command that carries no destination.
///
/// Both the quick-move and the drop-stack commands address one slot and let the
/// authority derive the destination, so they share this record and its view
/// bounds. There is no target, hence no distinct-slot rule to apply.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
pub struct StackSource {
    view: StackView,
    slot: u8,
}

impl StackSource {
    /// Wraps one source slot, rejecting an index outside the view's bound.
    pub fn try_new(view: StackView, slot: u8) -> Result<Self, DomainError> {
        if slot >= view.slot_bound() {
            return Err(DomainError::InvalidSlot);
        }
        Ok(Self { view, slot })
    }

    pub fn view(self) -> StackView {
        self.view
    }

    pub fn slot(self) -> u8 {
        self.slot
    }
}
