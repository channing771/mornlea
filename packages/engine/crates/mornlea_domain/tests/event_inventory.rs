//! Inventory, crafting and container publication contracts for `mornlea_domain`.
//!
//! The Go baseline is the `protocol.InventoryState`, `protocol.CraftingState`,
//! `protocol.FurnaceState`, `protocol.ChestState` and `protocol.ContainerClosed`
//! wire records in `packages/shared/network/protocol`: the inventory and the
//! crafting grid are the complete authoritative item state of one session, the
//! furnace and chest records are what the current viewer of one container
//! receives, and the closure notification retires a container both kinds
//! share. Every value rule below is the Go validator's rule, so a record this
//! crate admits is a record the protocol layer admits and the other way
//! around.
//!
//! Three Go rules deliberately stay out of this file. A container reference's
//! wire dimension is validated by the protocol conversion before a
//! `ContainerRef` exists, so the domain value carries no dimension to get
//! wrong; the per-chunk slot counts and the nonzero generation are enforced by
//! `ContainerRef` itself; and every slot value is already a validated
//! `ItemStack`, so the stack rules are not restated here. No timer consistency
//! rule relating the progress, the burn time and the three stacks exists in
//! the Go validator, and none is invented here.
//!
//! No record carries an equipped-armor array. Armor slots are a player-save
//! concern the inventory publication does not mirror, exactly as the private
//! player publication carries no inventory.

use mornlea_domain::{
    ChestState, ChestStateParts, ChunkPos, ContainerClosed, ContainerKind, ContainerRef,
    CraftingSize, CraftingState, CraftingStateParts, DomainError, FurnaceState, FurnaceStateParts,
    HotbarSlot, InventoryState, InventoryStateParts, ItemStack, is_smelting_product,
    smelting_output,
};

/// Fixed hotbar slots one inventory publishes, from the Go `core.HotbarSlots`.
const HOTBAR_SLOTS: usize = 9;

/// Fixed backpack slots one inventory publishes, from the Go
/// `core.BackpackSlots`.
const BACKPACK_SLOTS: usize = 27;

/// Fixed crafting grid slots one crafting state publishes, from the Go
/// `core.CraftingGridSlots`.
const CRAFTING_GRID_SLOTS: usize = 9;

/// Seed inventory: the last hotbar slot selected and every slot empty, which
/// is the canonical empty publication.
fn empty_inventory() -> InventoryStateParts {
    InventoryStateParts {
        selected: HotbarSlot::new(8).expect("the last hotbar slot is selectable"),
        hotbar: [ItemStack::EMPTY; HOTBAR_SLOTS],
        backpack: [ItemStack::EMPTY; BACKPACK_SLOTS],
    }
}

/// One furnace reference inside the fixed per-chunk furnace array.
fn furnace_ref() -> ContainerRef {
    ContainerRef::try_new(ChunkPos::new(2, -3), ContainerKind::Furnace, 31, 9)
        .expect("slot 31 and a nonzero generation are a legal furnace reference")
}

/// One chest reference inside the fixed per-chunk chest array.
fn chest_ref() -> ContainerRef {
    ContainerRef::try_new(ChunkPos::new(-4, 5), ContainerKind::Chest, 15, 4)
        .expect("slot 15 and a nonzero generation are a legal chest reference")
}

/// Seed furnace state: raw iron smelting into an iron ingot on a coal fire one
/// tick short of the requirement and at the burn maximum.
fn seed_furnace() -> FurnaceStateParts {
    FurnaceStateParts {
        container: furnace_ref(),
        input: ItemStack::try_new(6, 1, 0).expect("raw iron is a registered smelting input"),
        fuel: ItemStack::try_new(5, 1, 0).expect("coal is the registered furnace fuel"),
        output: ItemStack::try_new(7, 1, 0).expect("an iron ingot is a registered product"),
        progress_ticks: 199,
        burn_ticks: 1600,
    }
}

/// One empty crafting grid with the named size.
fn empty_crafting(size: CraftingSize) -> CraftingStateParts {
    CraftingStateParts {
        size,
        slots: [ItemStack::EMPTY; CRAFTING_GRID_SLOTS],
        output: ItemStack::EMPTY,
    }
}

#[test]
fn event_inventory_state_all_empty_with_selected_eight_is_admitted() {
    let state = InventoryState::new(empty_inventory());

    assert_eq!(state.selected().get(), 8);
    assert_eq!(state.hotbar().len(), HOTBAR_SLOTS);
    assert_eq!(state.backpack().len(), BACKPACK_SLOTS);
    assert!(
        state
            .hotbar()
            .iter()
            .all(|stack| *stack == ItemStack::EMPTY)
    );
    assert!(
        state
            .backpack()
            .iter()
            .all(|stack| *stack == ItemStack::EMPTY)
    );
}

#[test]
fn event_inventory_state_keeps_the_selected_slot_and_every_stack() {
    // The publication is a mirror, so the selected index and both arrays are
    // preserved exactly: a stone stack in the first hotbar slot, coal in the
    // last one and an iron ingot in the last backpack slot.
    let mut parts = empty_inventory();
    parts.selected = HotbarSlot::new(0).expect("the first hotbar slot is selectable");
    parts.hotbar[0] = ItemStack::try_new(1, 64, 0).expect("a full stone stack is valid");
    parts.hotbar[8] = ItemStack::try_new(5, 1, 0).expect("coal is valid");
    parts.backpack[26] = ItemStack::try_new(7, 1, 0).expect("an iron ingot is valid");

    let state = InventoryState::new(parts);

    assert_eq!(state.selected().get(), 0);
    assert_eq!(state.hotbar()[0], ItemStack::try_new(1, 64, 0).unwrap());
    assert_eq!(state.hotbar()[8], ItemStack::try_new(5, 1, 0).unwrap());
    assert_eq!(state.backpack()[26], ItemStack::try_new(7, 1, 0).unwrap());
    assert_eq!(state.backpack()[0], ItemStack::EMPTY);
}

#[test]
fn event_inventory_crafting_personal_grid_rejects_residue_beyond_its_own_slots() {
    // A personal grid is 2x2, so slots 4..8 are the cells the grid does not
    // have. The Go validator rejects a residue there because a client would
    // otherwise have to guess whether a stale slot still applies.
    for slot in [4, 8] {
        let mut parts = empty_crafting(CraftingSize::Personal);
        parts.slots[slot] = ItemStack::try_new(1, 1, 0).expect("a stone stack is valid");
        assert_eq!(
            CraftingState::try_new(parts),
            Err(DomainError::InvalidCraftingResidue),
            "personal slot {slot} is beyond the 2x2 grid"
        );
    }

    // The four usable slots accept any valid stack, and the output slot is
    // derived by the server rather than declared, so it carries its own rule
    // only through the ordinary stack validity.
    let mut parts = empty_crafting(CraftingSize::Personal);
    parts.slots[3] = ItemStack::try_new(1, 1, 0).expect("a stone stack is valid");
    parts.output = ItemStack::try_new(7, 1, 0).expect("an iron ingot is valid");
    let state =
        CraftingState::try_new(parts).expect("a personal grid inside its own slots is admitted");
    assert_eq!(state.size(), CraftingSize::Personal);
    assert_eq!(state.slots()[3], ItemStack::try_new(1, 1, 0).unwrap());
    assert_eq!(state.output(), ItemStack::try_new(7, 1, 0).unwrap());
}

#[test]
fn event_inventory_crafting_workbench_allows_slot_eight() {
    // The workbench is 3x3, so all nine slots are usable and the last one is
    // the case the personal rule refuses.
    let mut parts = empty_crafting(CraftingSize::Workbench);
    parts.slots[8] = ItemStack::try_new(1, 1, 0).expect("a stone stack is valid");

    let state = CraftingState::try_new(parts).expect("a workbench grid fills every slot");

    assert_eq!(state.size(), CraftingSize::Workbench);
    assert_eq!(state.slots().len(), CRAFTING_GRID_SLOTS);
    assert_eq!(state.slots()[8], ItemStack::try_new(1, 1, 0).unwrap());
}

#[test]
fn event_inventory_furnace_seed_is_admitted_and_keeps_every_field() {
    let state = FurnaceState::try_new(seed_furnace()).expect("the seed furnace is publishable");

    assert_eq!(state.container(), furnace_ref());
    assert_eq!(state.input(), ItemStack::try_new(6, 1, 0).unwrap());
    assert_eq!(state.fuel(), ItemStack::try_new(5, 1, 0).unwrap());
    assert_eq!(state.output(), ItemStack::try_new(7, 1, 0).unwrap());
    assert_eq!(state.progress_ticks(), 199);
    assert_eq!(state.burn_ticks(), 1600);

    // Every one of the three stacks also accepts the empty stack, which is how
    // an idle furnace is published.
    let mut idle = seed_furnace();
    idle.input = ItemStack::EMPTY;
    idle.fuel = ItemStack::EMPTY;
    idle.output = ItemStack::EMPTY;
    idle.progress_ticks = 0;
    idle.burn_ticks = 0;
    assert!(FurnaceState::try_new(idle).is_ok());
}

#[test]
fn event_inventory_furnace_accepts_every_smelting_input() {
    // The input whitelist is exactly the Go `core.SmeltingOutput` key set, so
    // each of the four inputs is admitted on its own seed row.
    for input in [6u16, 18, 27, 53] {
        let mut parts = seed_furnace();
        parts.input = ItemStack::try_new(input, 1, 0).expect("a registered smelting input");
        assert!(
            FurnaceState::try_new(parts).is_ok(),
            "smelting input {input} must be admitted"
        );
    }
}

#[test]
fn event_inventory_furnace_accepts_every_smelting_product() {
    // The output whitelist is exactly the Go product set, so each product is
    // admitted: the authority writes one of them into the output slot in the
    // same tick it sends the state.
    for output in [7u16, 23, 24, 54] {
        let mut parts = seed_furnace();
        parts.output = ItemStack::try_new(output, 1, 0).expect("a registered smelting product");
        assert!(
            FurnaceState::try_new(parts).is_ok(),
            "smelting product {output} must be admitted"
        );
    }
    // The whitelist is read from the shared smelting tables rather than listed
    // again here, so the domain predicates and the record agree by
    // construction.
    assert!(smelting_output(6) == Some(7));
    assert!(is_smelting_product(54));
}

#[test]
fn event_inventory_furnace_rejects_a_fuel_that_is_not_coal() {
    let mut parts = seed_furnace();
    parts.fuel = ItemStack::try_new(1, 1, 0).expect("a stone stack is valid");
    assert_eq!(
        FurnaceState::try_new(parts),
        Err(DomainError::InvalidFurnaceSlot),
        "stone is a registered item but not a furnace fuel"
    );
}

#[test]
fn event_inventory_furnace_rejects_an_input_that_is_not_smeltable() {
    let mut parts = seed_furnace();
    parts.input = ItemStack::try_new(1, 1, 0).expect("a stone stack is valid");
    assert_eq!(
        FurnaceState::try_new(parts),
        Err(DomainError::InvalidFurnaceSlot),
        "stone is a registered item but not a smelting input"
    );
}

#[test]
fn event_inventory_furnace_rejects_an_output_that_is_not_a_product() {
    let mut parts = seed_furnace();
    parts.output = ItemStack::try_new(1, 1, 0).expect("a stone stack is valid");
    assert_eq!(
        FurnaceState::try_new(parts),
        Err(DomainError::InvalidFurnaceSlot),
        "stone is a registered item but not a smelting product"
    );
}

#[test]
fn event_inventory_furnace_progress_stays_below_the_requirement() {
    let admitted = |progress| {
        let mut parts = seed_furnace();
        parts.progress_ticks = progress;
        FurnaceState::try_new(parts)
    };

    // The bound is exclusive: a progress at the requirement is a completed
    // swing, which the Go validator rejects instead of publishing as active.
    assert!(admitted(199).is_ok());
    assert_eq!(admitted(200), Err(DomainError::InvalidFurnaceTimers));
}

#[test]
fn event_inventory_furnace_burn_stays_at_or_below_the_maximum() {
    let admitted = |burn| {
        let mut parts = seed_furnace();
        parts.burn_ticks = burn;
        FurnaceState::try_new(parts)
    };

    // The bound is inclusive on the burn time, unlike the progress bound.
    assert!(admitted(1600).is_ok());
    assert_eq!(admitted(1601), Err(DomainError::InvalidFurnaceTimers));
}

#[test]
fn event_inventory_furnace_rejects_a_chest_reference() {
    let mut parts = seed_furnace();
    parts.container = chest_ref();
    assert_eq!(
        FurnaceState::try_new(parts),
        Err(DomainError::InvalidContainerKind),
        "a furnace publication needs a furnace reference"
    );
}

#[test]
fn event_inventory_chest_seed_is_admitted_and_keeps_every_item() {
    let mut parts = ChestStateParts {
        container: chest_ref(),
        items: [ItemStack::EMPTY; 27],
    };
    parts.items[26] = ItemStack::try_new(1, 64, 0).expect("a full stone stack is valid");

    let state = ChestState::try_new(parts).expect("the seed chest is publishable");

    assert_eq!(state.container(), chest_ref());
    assert_eq!(state.items().len(), 27);
    assert_eq!(state.items()[26], ItemStack::try_new(1, 64, 0).unwrap());
    assert_eq!(state.items()[0], ItemStack::EMPTY);
}

#[test]
fn event_inventory_chest_rejects_a_furnace_reference() {
    let parts = ChestStateParts {
        container: furnace_ref(),
        items: [ItemStack::EMPTY; 27],
    };
    assert_eq!(
        ChestState::try_new(parts),
        Err(DomainError::InvalidContainerKind),
        "a chest publication needs a chest reference"
    );
}

#[test]
fn event_inventory_container_closed_accepts_both_kinds() {
    // The Go packet is named `FurnaceEnd` for historical reasons but carries
    // both container kinds, so the closure notification accepts either one.
    let furnace = ContainerClosed::new(furnace_ref());
    let chest = ContainerClosed::new(chest_ref());

    assert_eq!(furnace.container(), furnace_ref());
    assert_eq!(furnace.container().kind(), ContainerKind::Furnace);
    assert_eq!(chest.container(), chest_ref());
    assert_eq!(chest.container().kind(), ContainerKind::Chest);
}

#[test]
fn event_inventory_records_carry_no_equipped_armor_slots() {
    // Equipped armor stays outside these records, exactly as it stays outside
    // the private player publication: the inventory mirror publishes the
    // hotbar, the backpack, the crafting grid and the viewed container, and an
    // armor array is a player-save concern no publication mirrors. The debug
    // renderings below list every published field, so an added armor field
    // fails here through the published shape.
    let inventory = format!("{:?}", InventoryState::new(empty_inventory()));
    for published in ["selected", "hotbar", "backpack"] {
        assert!(
            inventory.contains(published),
            "inventory state must carry {published}"
        );
    }
    let crafting = format!(
        "{:?}",
        CraftingState::try_new(empty_crafting(CraftingSize::Personal)).unwrap()
    );
    for published in ["size", "slots", "output"] {
        assert!(
            crafting.contains(published),
            "crafting state must carry {published}"
        );
    }
    let furnace = format!("{:?}", FurnaceState::try_new(seed_furnace()).unwrap());
    for published in [
        "container",
        "input",
        "fuel",
        "output",
        "progress_ticks",
        "burn_ticks",
    ] {
        assert!(
            furnace.contains(published),
            "furnace state must carry {published}"
        );
    }
    let chest = format!(
        "{:?}",
        ChestState::try_new(ChestStateParts {
            container: chest_ref(),
            items: [ItemStack::EMPTY; 27],
        })
        .unwrap()
    );
    for published in ["container", "items"] {
        assert!(
            chest.contains(published),
            "chest state must carry {published}"
        );
    }
    let closed = format!("{:?}", ContainerClosed::new(chest_ref()));
    assert!(closed.contains("container"), "closure must carry container");

    for rendered in [&inventory, &crafting, &furnace, &chest, &closed] {
        assert!(
            !rendered.contains("armor"),
            "no inventory or container publication carries an equipped armor slot"
        );
    }
}
