//! Item, drop and container value contracts for `mornlea_domain`, pinned
//! against the verified Go baseline.
//!
//! The Go oracles are `core.ItemStackLimit`, `core.ItemMaxDurability` and
//! `core.SmeltingOutput` in `packages/shared/core`, `core.ItemStack.Valid` for
//! the ordinary slot value, `core.DropID.Valid` for the drop identity, and the
//! `validFurnaceRef` / `validChestRef` container rules in
//! `packages/shared/network/protocol/message_container.go`. Each table is
//! transcribed independently below so the Rust table is compared against the
//! Go source rather than against itself.

use mornlea_domain::{
    ChunkPos, ContainerKind, ContainerRef, DomainError, DropId, ItemStack, durability_max,
    is_smelting_product, item_stack_limit, smelting_output,
};

/// Exclusive upper bound of the registered item numbering, from the Go
/// `core.ItemIDMax` sentinel: item 0 is the absent item and item 66 is the
/// first unregistered number.
const ITEM_ID_MAX: u16 = 66;

/// Independent transcription of the Go `core.ItemStackLimit` switch arms.
///
/// The arms are written as the ranges the Go source lists rather than as the
/// Rust constant array, so a drift in either direction fails here instead of
/// silently agreeing with itself.
fn go_item_stack_limit(item: u16) -> Option<u8> {
    match item {
        1..=9 | 14..=29 | 34..=46 | 53 | 54 | 57 | 63 | 64 => Some(64),
        10..=13 | 30..=33 | 47..=52 | 55 | 56 | 58..=62 | 65 => Some(1),
        _ => None,
    }
}

/// Independent transcription of the Go `core.ItemMaxDurability` switch,
/// including the four armor-domain values it references.
fn go_durability_max(item: u16) -> Option<u16> {
    match item {
        10 | 30 | 48 => Some(131),
        11 | 31 | 49 => Some(250),
        47 => Some(59),
        58 => Some(165),
        59 => Some(240),
        60 => Some(225),
        61 => Some(195),
        62 => Some(120),
        _ => None,
    }
}

/// Independent transcription of the Go `core.SmeltingOutput` switch.
fn go_smelting_output(item: u16) -> Option<u16> {
    match item {
        6 => Some(7),
        18 => Some(23),
        27 => Some(24),
        53 => Some(54),
        _ => None,
    }
}

/// Independent transcription of the furnace output whitelist the Go protocol
/// validator applies, which is exactly the product set of the smelting table.
fn go_smelting_product(item: u16) -> bool {
    matches!(item, 7 | 23 | 24 | 54)
}

/// Collects the registered item numbers, which is every number below the
/// sentinel except the absent item.
fn registered_items() -> impl Iterator<Item = u16> {
    (0..ITEM_ID_MAX).filter(|item| *item != 0)
}

#[test]
fn items_locations_item_tables_match_the_go_source_for_every_id() {
    for item in 0..=ITEM_ID_MAX {
        assert_eq!(
            item_stack_limit(item),
            go_item_stack_limit(item),
            "stack limit for item {item} does not match the Go table"
        );
        assert_eq!(
            durability_max(item),
            go_durability_max(item),
            "durability maximum for item {item} does not match the Go table"
        );
        assert_eq!(
            smelting_output(item),
            go_smelting_output(item),
            "smelting output for item {item} does not match the Go table"
        );
        assert_eq!(
            is_smelting_product(item),
            go_smelting_product(item),
            "smelting product membership for item {item} does not match the Go table"
        );
    }

    // The range itself is part of the contract: 1..=65 registered, the sentinel
    // and everything above it unregistered.
    assert_eq!(item_stack_limit(ITEM_ID_MAX), None);
    assert_eq!(item_stack_limit(ITEM_ID_MAX + 1), None);
    assert_eq!(registered_items().count(), 65);
    assert!(registered_items().all(|item| item_stack_limit(item).is_some()));
}

#[test]
fn items_locations_smelting_product_set_is_the_smelting_output_range() {
    let mut products: Vec<u16> = (0..ITEM_ID_MAX)
        .filter(|item| is_smelting_product(*item))
        .collect();
    products.sort_unstable();
    assert_eq!(products, vec![7, 23, 24, 54]);

    // Every product is reachable from exactly one input, so the derived set and
    // the input table cannot describe different worlds.
    for product in products {
        let inputs: Vec<u16> = (0..ITEM_ID_MAX)
            .filter(|item| smelting_output(*item) == Some(product))
            .collect();
        assert_eq!(
            inputs.len(),
            1,
            "product {product} has {} inputs",
            inputs.len()
        );
    }
}

#[test]
fn items_locations_item_stack_empty_is_exactly_the_zero_triple() {
    assert_eq!(ItemStack::EMPTY.item(), 0);
    assert_eq!(ItemStack::EMPTY.count(), 0);
    assert_eq!(ItemStack::EMPTY.durability(), 0);
    assert_eq!(
        ItemStack::try_new(0, 0, 0),
        Ok(ItemStack::EMPTY),
        "the Go rule accepts the empty slot value, so the empty stack must be constructible"
    );

    // The empty form is the only value the absent item number admits.
    assert_eq!(
        ItemStack::try_new(0, 1, 0),
        Err(DomainError::InvalidCount),
        "a nonzero count on the absent item is a count rejection"
    );
    assert_eq!(
        ItemStack::try_new(0, 0, 1),
        Err(DomainError::InvalidDurability),
        "a nonzero durability on the absent item is a durability rejection"
    );
    assert_eq!(
        ItemStack::try_new(0, 1, 1),
        Err(DomainError::InvalidCount),
        "the count is reported before the durability on the absent item"
    );
}

#[test]
fn items_locations_item_stack_rejects_unregistered_item_numbers() {
    assert_eq!(
        ItemStack::try_new(ITEM_ID_MAX, 1, 0),
        Err(DomainError::InvalidItem)
    );
    assert_eq!(
        ItemStack::try_new(ITEM_ID_MAX + 1, 1, 0),
        Err(DomainError::InvalidItem)
    );
    assert_eq!(
        ItemStack::try_new(u16::MAX, 1, 0),
        Err(DomainError::InvalidItem)
    );
    assert!(
        registered_items().all(|item| ItemStack::try_new(item, 1, 0).is_ok()
            || ItemStack::try_new(item, 1, 0) == Err(DomainError::InvalidDurability)),
        "every registered item accepts a single intact unit"
    );
}

#[test]
fn items_locations_item_stack_count_boundaries_follow_the_stack_limit() {
    for item in registered_items() {
        let limit = item_stack_limit(item).expect("registered item has a stack limit");
        let durability = match durability_max(item) {
            Some(max) => max,
            None => 0,
        };

        assert_eq!(
            ItemStack::try_new(item, 0, durability),
            Err(DomainError::InvalidCount),
            "item {item} rejects a zero count"
        );
        assert_eq!(
            ItemStack::try_new(item, limit, durability)
                .expect("limit count")
                .count(),
            limit,
            "item {item} accepts exactly its limit"
        );
        assert_eq!(
            ItemStack::try_new(item, limit + 1, durability),
            Err(DomainError::InvalidCount),
            "item {item} rejects one above its limit"
        );
    }
}

#[test]
fn items_locations_item_stack_durability_boundaries_follow_the_durability_table() {
    for item in registered_items() {
        let limit = item_stack_limit(item).expect("registered item has a stack limit");
        let count = limit;

        match durability_max(item) {
            Some(max) => {
                assert_eq!(
                    ItemStack::try_new(item, count, 0),
                    Err(DomainError::InvalidDurability),
                    "durable item {item} rejects a zero durability"
                );
                assert_eq!(
                    ItemStack::try_new(item, count, 1)
                        .expect("lowest intact durability")
                        .durability(),
                    1,
                    "durable item {item} accepts durability one"
                );
                assert_eq!(
                    ItemStack::try_new(item, count, max)
                        .expect("maximum durability")
                        .durability(),
                    max,
                    "durable item {item} accepts exactly its maximum"
                );
                assert_eq!(
                    ItemStack::try_new(item, count, max + 1),
                    Err(DomainError::InvalidDurability),
                    "durable item {item} rejects one above its maximum"
                );
            }
            None => {
                assert_eq!(
                    ItemStack::try_new(item, count, 0)
                        .expect("nondurable item at zero durability")
                        .durability(),
                    0,
                    "nondurable item {item} accepts a zero durability"
                );
                assert_eq!(
                    ItemStack::try_new(item, count, 1),
                    Err(DomainError::InvalidDurability),
                    "nondurable item {item} rejects a nonzero durability"
                );
            }
        }
    }
}

#[test]
fn items_locations_broken_equipped_armor_stays_outside_the_ordinary_stack() {
    // The player-save armor slots keep a worn piece at durability zero. That is
    // raw historical fidelity for a separately owned slot, not a relaxation of
    // the ordinary stack rule, so a durable item at durability zero is still
    // rejected here.
    for armor in 58..=61u16 {
        assert_eq!(
            ItemStack::try_new(armor, 1, 0),
            Err(DomainError::InvalidDurability),
            "armor item {armor} at durability zero is not an ordinary stack"
        );
        assert!(ItemStack::try_new(armor, 1, 1).is_ok());
        assert!(ItemStack::try_new(armor, 1, durability_max(armor).expect("armor budget")).is_ok());
    }
}

#[test]
fn items_locations_item_stack_getters_return_the_constructed_values() {
    let stack = ItemStack::try_new(10, 1, 131).expect("intact stone pickaxe");
    assert_eq!(stack.item(), 10);
    assert_eq!(stack.count(), 1);
    assert_eq!(stack.durability(), 131);

    let bread = ItemStack::try_new(36, 64, 0).expect("a full bread stack");
    assert_eq!(bread.item(), 36);
    assert_eq!(bread.count(), 64);
    assert_eq!(bread.durability(), 0);
}

#[test]
fn items_locations_chunk_pos_preserves_negative_coordinates() {
    let chunk = ChunkPos::new(-7, 12);
    assert_eq!(chunk.x(), -7);
    assert_eq!(chunk.z(), 12);

    let extremes = ChunkPos::new(i32::MIN, i32::MAX);
    assert_eq!(extremes.x(), i32::MIN);
    assert_eq!(extremes.z(), i32::MAX);
}

#[test]
fn items_locations_drop_id_retains_an_arbitrary_raw_dimension() {
    // The Go `core.DropID.Valid` rule checks only the slot range and the
    // generation, so a dimension the playable set does not name is still a
    // publishable identity rather than a rejection.
    for dimension in [i32::MIN, -1, 0, 1, i32::MAX] {
        let drop = DropId::try_new(dimension, ChunkPos::new(0, 0), 0, 1)
            .expect("any dimension is retained");
        assert_eq!(drop.dimension(), dimension);
    }
}

#[test]
fn items_locations_drop_id_slot_and_generation_bounds() {
    let chunk = ChunkPos::new(3, -4);

    let last = DropId::try_new(0, chunk, 31, 1).expect("slot 31 is the last drop slot");
    assert_eq!(last.slot(), 31);
    assert_eq!(last.generation(), 1);
    assert_eq!(last.chunk(), chunk);

    assert_eq!(
        DropId::try_new(0, chunk, 32, 1),
        Err(DomainError::InvalidDropSlot),
        "slot 32 is outside the fixed per-chunk drop array"
    );
    assert_eq!(
        DropId::try_new(0, chunk, u8::MAX, 1),
        Err(DomainError::InvalidDropSlot)
    );

    assert_eq!(
        DropId::try_new(0, chunk, 0, 0),
        Err(DomainError::InvalidDropGeneration),
        "generation zero names a slot that was never used"
    );
    assert_eq!(
        DropId::try_new(0, chunk, 0, u32::MAX)
            .expect("the largest generation is legal")
            .generation(),
        u32::MAX
    );

    // The slot is reported before the generation, so a doubly invalid identity
    // fails on the field the Go rule names first.
    assert_eq!(
        DropId::try_new(0, chunk, 32, 0),
        Err(DomainError::InvalidDropSlot)
    );
}

#[test]
fn items_locations_drop_id_orders_by_dimension_chunk_slot_then_generation() {
    let mut ordered = vec![
        DropId::try_new(1, ChunkPos::new(0, 0), 0, 1).expect("dimension one"),
        DropId::try_new(0, ChunkPos::new(1, 0), 0, 1).expect("later chunk x"),
        DropId::try_new(0, ChunkPos::new(0, 1), 0, 1).expect("later chunk z"),
        DropId::try_new(0, ChunkPos::new(0, 0), 1, 1).expect("later slot"),
        DropId::try_new(0, ChunkPos::new(0, 0), 0, 2).expect("later generation"),
        DropId::try_new(0, ChunkPos::new(0, 0), 0, 1).expect("first"),
        DropId::try_new(-1, ChunkPos::new(0, 0), 0, 1).expect("negative dimension"),
    ];
    ordered.sort();

    let keys: Vec<(i32, i32, i32, u8, u32)> = ordered
        .iter()
        .map(|drop| {
            (
                drop.dimension(),
                drop.chunk().x(),
                drop.chunk().z(),
                drop.slot(),
                drop.generation(),
            )
        })
        .collect();
    assert_eq!(
        keys,
        vec![
            (-1, 0, 0, 0, 1),
            (0, 0, 0, 0, 1),
            (0, 0, 0, 0, 2),
            (0, 0, 0, 1, 1),
            (0, 0, 1, 0, 1),
            (0, 1, 0, 0, 1),
            (1, 0, 0, 0, 1),
        ]
    );

    // A reverse-sorted input has to reach the same order, so the comparison is
    // a total order rather than an artifact of the initial sequence.
    let mut reversed = ordered.clone();
    reversed.reverse();
    reversed.sort();
    assert_eq!(reversed, ordered);
}

#[test]
fn items_locations_container_ref_furnace_and_chest_slot_bounds() {
    let chunk = ChunkPos::new(-2, 5);

    let furnace = ContainerRef::try_new(chunk, ContainerKind::Furnace, 31, 1)
        .expect("slot 31 is the last furnace slot");
    assert_eq!(furnace.kind(), ContainerKind::Furnace);
    assert_eq!(furnace.slot(), 31);
    assert_eq!(furnace.chunk(), chunk);
    assert_eq!(
        ContainerRef::try_new(chunk, ContainerKind::Furnace, 32, 1),
        Err(DomainError::InvalidContainerSlot),
        "slot 32 is outside the fixed per-chunk furnace array"
    );

    let chest = ContainerRef::try_new(chunk, ContainerKind::Chest, 15, 1)
        .expect("slot 15 is the last chest slot");
    assert_eq!(chest.kind(), ContainerKind::Chest);
    assert_eq!(chest.slot(), 15);
    assert_eq!(
        ContainerRef::try_new(chunk, ContainerKind::Chest, 16, 1),
        Err(DomainError::InvalidContainerSlot),
        "slot 16 is outside the fixed per-chunk chest array"
    );

    // A furnace slot that is legal for a furnace is not legal for a chest,
    // because the two arrays have different sizes.
    assert_eq!(
        ContainerRef::try_new(chunk, ContainerKind::Chest, 31, 1),
        Err(DomainError::InvalidContainerSlot)
    );

    for kind in [ContainerKind::Furnace, ContainerKind::Chest] {
        assert_eq!(
            ContainerRef::try_new(chunk, kind, 0, 0),
            Err(DomainError::InvalidContainerGeneration),
            "generation zero names a container slot that was never used"
        );
        assert_eq!(
            ContainerRef::try_new(chunk, kind, 0, u32::MAX)
                .expect("the largest generation is legal")
                .generation(),
            u32::MAX
        );
    }
}

#[test]
fn items_locations_container_kind_orders_furnace_before_chest() {
    // The furnace variant is the Go zero value, so it has to stay first.
    assert!(ContainerKind::Furnace < ContainerKind::Chest);
}
