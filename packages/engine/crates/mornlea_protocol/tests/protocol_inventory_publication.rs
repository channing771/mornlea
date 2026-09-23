//! The five inventory and container publication families: the common fallible
//! surface, the shared item-stack rule, and the container-reference gates.
//!
//! `InventoryState` (S/Play/10), `CraftingState` (S/Play/21), `FurnaceState`
//! (S/Play/13), `ChestState` (S/Play/15) and `ContainerClosed` (S/Play/14) are
//! the item and container publications an authoritative session addresses to
//! the one player that owns them: the complete fixed inventory, the fixed
//! crafting grid, the furnace mirror, the chest mirror and the container-view
//! closure notice. Viewer routing and crafting or smelting outcomes stay with
//! the authority, so the wire carries no recipient, no selected slot context
//! and no recipe.
//!
//! All five records are concrete packets with public mutable fields, so the
//! group pins the design's surface order (`validate` → checked `encoded_len`
//! → capacity check → `publish_packet`): a short or invalid `encode_into`
//! leaves the caller's buffer untouched, an invalid value wins over a short
//! destination, and every proper truncation of a canonical payload rejects.
//! The wire literals are the Go encoder's output, so the round-trip tests pin
//! byte-level parity rather than self-agreement.
//!
//! The value gates keep the Go validator orders: the inventory selected index
//! before every stack; the crafting size, then the personal-grid residue; the
//! furnace reference, timers and slot whitelists; the chest reference; and the
//! container-neutral reference. The slot values are the domain's checked
//! `ItemStack`, so an invalid slot value cannot be constructed on this surface
//! and the item-stack rule stays owned once. `ContainerClosed` is the family
//! this node adds to the compiled public surface: it existed on disk but was
//! absent from `lib.rs`, which is the import red this suite now closes.
//!
//! The category split this group pins follows the Go validators: reference
//! kind violations and unregistered item numbers report `InvalidEnum`, while
//! the selected index, size, residue, timer, generation, slot, dimension,
//! durability and whitelist violations report `InvalidRange`. Both sides
//! publish one category for the same bytes. The corpus evidence these families
//! publish is executed by `tests/protocol_corpus.rs` once the controller
//! integrates the exported assets, so this suite stays self-contained and
//! needs no corpus files.

use mornlea_protocol::{
    BACKPACK_SLOTS, CHEST_SLOTS, CHESTS_PER_CHUNK, CONTAINER_KIND_CHEST, CONTAINER_KIND_FURNACE,
    CRAFTING_GRID_SIZE_PERSONAL, CRAFTING_GRID_SIZE_WORKBENCH, ChestState, ContainerClosed,
    ContainerRef, CraftingState, FURNACE_BURN_TICKS, FURNACE_SMELT_TICKS, FurnaceState,
    HOTBAR_SLOTS, INVENTORY_STATE_WIRE_BYTES, InventoryState, ItemStack, ProtocolError,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// Wraps one slot value through the domain rule, which is the single owner of
/// the item-stack rule the Go `core.ItemStack.Valid` applies.
fn stack(item: u16, count: u8, durability: u16) -> ItemStack {
    ItemStack::try_new(item, count, durability).expect("a registered slot value")
}

/// The reviewed furnace reference: overworld dimension, chunk −1/2, kind 0,
/// physical slot 31 and generation 1.
const FURNACE_REF: ContainerRef = ContainerRef {
    dimension: 0,
    chunk_x: -1,
    chunk_z: 2,
    kind: CONTAINER_KIND_FURNACE,
    slot: 31,
    generation: 1,
};

/// The reviewed chest reference: the same chunk column with kind 1, physical
/// slot 15 and generation 1.
const CHEST_REF: ContainerRef = ContainerRef {
    dimension: 0,
    chunk_x: -1,
    chunk_z: 2,
    kind: CONTAINER_KIND_CHEST,
    slot: 15,
    generation: 1,
};

/// The reviewed 18-byte furnace reference literal in the wire field order.
const FURNACE_REF_WIRE: [u8; 18] = [
    0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x02, 0x00, 0x00, 0x00, 0x00, 0x1f, 0x01, 0x00,
    0x00, 0x00,
];

/// The reviewed 18-byte chest reference literal in the wire field order.
const CHEST_REF_WIRE: [u8; 18] = [
    0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0x02, 0x00, 0x00, 0x00, 0x01, 0x0f, 0x01, 0x00,
    0x00, 0x00,
];

/// The reviewed 181-byte InventoryState literal: selected index 8, a stone
/// stack and a full grass stack and one single-count stone pickaxe at legal
/// durability in the hotbar, and dirt, coal and stone stacks in the backpack,
/// with every other slot the exact empty triple.
const INVENTORY_STATE_WIRE: [u8; INVENTORY_STATE_WIRE_BYTES] = [
    0x08, // selected 8
    // hotbar 0..8
    0x01, 0x00, 0x05, 0x00, 0x00, // stone ×5
    0x00, 0x00, 0x00, 0x00, 0x00, // empty
    0x00, 0x00, 0x00, 0x00, 0x00, // empty
    0x00, 0x00, 0x00, 0x00, 0x00, // empty
    0x03, 0x00, 0x40, 0x00, 0x00, // grass ×64
    0x00, 0x00, 0x00, 0x00, 0x00, // empty
    0x00, 0x00, 0x00, 0x00, 0x00, // empty
    0x00, 0x00, 0x00, 0x00, 0x00, // empty
    0x0a, 0x00, 0x01, 0x3c, 0x00, // stone pickaxe ×1, durability 60
    // backpack 0
    0x02, 0x00, 0x01, 0x00, 0x00, // dirt ×1
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 1
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 2
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 3
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 4
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 5
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 6
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 7
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 8
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 9
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 10
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 11
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 12
    0x05, 0x00, 0x0c, 0x00, 0x00, // backpack 13: coal ×12
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 14
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 15
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 16
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 17
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 18
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 19
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 20
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 21
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 22
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 23
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 24
    0x00, 0x00, 0x00, 0x00, 0x00, // backpack 25
    0x01, 0x00, 0x09, 0x00, 0x00, // backpack 26: stone ×9
];

/// The reviewed 51-byte CraftingState literal: personal size, stone and stick
/// in the four usable cells, the five extension cells exactly empty, and the
/// derived stone-brick output.
const CRAFTING_STATE_WIRE: [u8; 51] = [
    0x02, // size 2 (personal)
    0x01, 0x00, 0x02, 0x00, 0x00, // grid 0: stone ×2
    0x01, 0x00, 0x02, 0x00, 0x00, // grid 1: stone ×2
    0x25, 0x00, 0x01, 0x00, 0x00, // grid 2: stick ×1
    0x25, 0x00, 0x01, 0x00, 0x00, // grid 3: stick ×1
    0x00, 0x00, 0x00, 0x00, 0x00, // grid 4: empty
    0x00, 0x00, 0x00, 0x00, 0x00, // grid 5: empty
    0x00, 0x00, 0x00, 0x00, 0x00, // grid 6: empty
    0x00, 0x00, 0x00, 0x00, 0x00, // grid 7: empty
    0x00, 0x00, 0x00, 0x00, 0x00, // grid 8: empty
    0x04, 0x00, 0x01, 0x00, 0x00, // output: stone brick ×1
];

/// The reviewed 36-byte FurnaceState literal: the furnace reference, raw beef
/// input, coal fuel, cooked beef output, progress 199 and burn 1600.
const FURNACE_STATE_WIRE: [u8; 36] = [
    0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, // furnace reference
    0x02, 0x00, 0x00, 0x00, 0x00, 0x1f, 0x01, 0x00, 0x00, 0x00, // furnace reference
    0x35, 0x00, 0x01, 0x00, 0x00, // input: raw beef ×1
    0x05, 0x00, 0x01, 0x00, 0x00, // fuel: coal ×1
    0x36, 0x00, 0x01, 0x00, 0x00, // output: cooked beef ×1
    0xc7, // progress 199
    0x40, 0x06, // burn 1600
];

/// The reviewed 153-byte ChestState literal: the chest reference with stone, a
/// single-count stone pickaxe at legal durability and dirt, and every other
/// slot the exact empty triple.
const CHEST_STATE_WIRE: [u8; 153] = [
    0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, // chest reference
    0x02, 0x00, 0x00, 0x00, 0x01, 0x0f, 0x01, 0x00, 0x00, 0x00, // chest reference
    0x01, 0x00, 0x05, 0x00, 0x00, // chest 0: stone ×5
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 1
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 2
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 3
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 4
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 5
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 6
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 7
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 8
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 9
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 10
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 11
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 12
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 13
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 14
    0x0a, 0x00, 0x01, 0x3c, 0x00, // chest 15: stone pickaxe ×1, durability 60
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 16
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 17
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 18
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 19
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 20
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 21
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 22
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 23
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 24
    0x00, 0x00, 0x00, 0x00, 0x00, // chest 25
    0x02, 0x00, 0x01, 0x00, 0x00, // chest 26: dirt ×1
];

/// The canonical inventory the reviewed literal carries.
fn canonical_inventory() -> InventoryState {
    let mut hotbar = [ItemStack::EMPTY; HOTBAR_SLOTS];
    hotbar[0] = stack(1, 5, 0);
    hotbar[4] = stack(3, 64, 0);
    hotbar[8] = stack(10, 1, 60);
    let mut backpack = [ItemStack::EMPTY; BACKPACK_SLOTS];
    backpack[0] = stack(2, 1, 0);
    backpack[13] = stack(5, 12, 0);
    backpack[BACKPACK_SLOTS - 1] = stack(1, 9, 0);
    InventoryState::new(8, hotbar, backpack).expect("the canonical inventory is valid")
}

/// The canonical personal crafting grid the reviewed literal carries.
fn canonical_crafting() -> CraftingState {
    let mut slots = [ItemStack::EMPTY; 9];
    slots[0] = stack(1, 2, 0);
    slots[1] = stack(1, 2, 0);
    slots[2] = stack(37, 1, 0);
    slots[3] = stack(37, 1, 0);
    CraftingState::new(CRAFTING_GRID_SIZE_PERSONAL, slots, stack(4, 1, 0))
        .expect("the canonical crafting state is valid")
}

/// The canonical furnace mirror the reviewed literal carries.
fn canonical_furnace() -> FurnaceState {
    FurnaceState::new(
        FURNACE_REF,
        stack(53, 1, 0),
        stack(5, 1, 0),
        stack(54, 1, 0),
        FURNACE_SMELT_TICKS - 1,
        FURNACE_BURN_TICKS,
    )
    .expect("the canonical furnace state is valid")
}

/// The canonical chest mirror the reviewed literal carries.
fn canonical_chest() -> ChestState {
    let mut items = [ItemStack::EMPTY; CHEST_SLOTS];
    items[0] = stack(1, 5, 0);
    items[15] = stack(10, 1, 60);
    items[CHEST_SLOTS - 1] = stack(2, 1, 0);
    ChestState::new(CHEST_REF, items).expect("the canonical chest state is valid")
}

/// The canonical closure notices: one per container kind.
fn canonical_closed_furnace() -> ContainerClosed {
    ContainerClosed::new(FURNACE_REF).expect("the canonical furnace closure is valid")
}

fn canonical_closed_chest() -> ContainerClosed {
    ContainerClosed::new(CHEST_REF).expect("the canonical chest closure is valid")
}

/// Asserts the shared surface contract for one canonical record: the reviewed
/// length, the exact window, the padded prefix, the allocating wrapper and the
/// decode/re-encode round trip.
///
/// The families differ in their record type but not in the surface, so the
/// helper takes the pieces it needs instead of duplicating the assertions.
fn assert_round_trip(
    label: &str,
    payload: &[u8],
    validate: impl FnOnce() -> Result<(), ProtocolError>,
    encoded_len: impl FnOnce() -> Result<usize, ProtocolError>,
    encode_into: impl Fn(&mut [u8]) -> Result<usize, ProtocolError>,
    encode: impl FnOnce() -> Result<Vec<u8>, ProtocolError>,
    decode: impl Fn(&[u8]) -> Result<(), ProtocolError>,
    re_encode: impl FnOnce() -> Result<Vec<u8>, ProtocolError>,
    equals: impl FnOnce() -> bool,
) {
    validate().unwrap_or_else(|err| panic!("{label}: valid record fails validation: {err:?}"));
    let length = encoded_len().unwrap_or_else(|err| panic!("{label}: encoded_len fails: {err:?}"));
    assert_eq!(
        length,
        payload.len(),
        "{label}: encoded_len disagrees with the reviewed payload"
    );

    let mut exact = vec![0u8; length];
    let written =
        encode_into(&mut exact).unwrap_or_else(|err| panic!("{label}: encode_into fails: {err:?}"));
    assert_eq!(
        written, length,
        "{label}: encode_into reports a short write"
    );
    assert_eq!(
        exact, payload,
        "{label}: encode_into published unexpected bytes"
    );

    let mut padded = vec![SENTINEL; length + 3];
    let written = encode_into(&mut padded)
        .unwrap_or_else(|err| panic!("{label}: padded encode_into fails: {err:?}"));
    assert_eq!(
        written, length,
        "{label}: padded write reports a short length"
    );
    assert_eq!(
        &padded[..length],
        payload,
        "{label}: padded prefix differs from the reviewed payload"
    );
    assert!(
        padded[length..].iter().all(|byte| *byte == SENTINEL),
        "{label}: padded write touched bytes beyond the record"
    );

    let allocated = encode().unwrap_or_else(|err| panic!("{label}: encode fails: {err:?}"));
    assert_eq!(
        allocated, payload,
        "{label}: encode disagrees with encode_into"
    );

    decode(payload).unwrap_or_else(|err| panic!("{label}: decode fails: {err:?}"));
    assert!(
        equals(),
        "{label}: decoded record differs from the encoded one"
    );
    let reencoded = re_encode().unwrap_or_else(|err| panic!("{label}: re-encode fails: {err:?}"));
    assert_eq!(
        reencoded, payload,
        "{label}: re-encoded payload differs from the reviewed bytes"
    );
}

#[test]
fn inventory_publication_inventory_state_round_trips_through_the_fallible_surface() {
    let record = canonical_inventory();
    let decoded = InventoryState::decode(&INVENTORY_STATE_WIRE).expect("decode");
    assert_round_trip(
        "inventory state",
        &INVENTORY_STATE_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| InventoryState::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(INVENTORY_STATE_WIRE.len(), 181);
    assert_eq!(decoded.selected, 8);
    assert_eq!(decoded.hotbar[8], stack(10, 1, 60));
    assert_eq!(decoded.backpack[13], stack(5, 12, 0));
}

#[test]
fn inventory_publication_crafting_state_round_trips_through_the_fallible_surface() {
    let record = canonical_crafting();
    let decoded = CraftingState::decode(&CRAFTING_STATE_WIRE).expect("decode");
    assert_round_trip(
        "crafting state",
        &CRAFTING_STATE_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| CraftingState::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(CRAFTING_STATE_WIRE.len(), 51);
    assert_eq!(decoded.size, CRAFTING_GRID_SIZE_PERSONAL);
    assert_eq!(decoded.output, stack(4, 1, 0));
}

#[test]
fn inventory_publication_furnace_state_round_trips_through_the_fallible_surface() {
    let record = canonical_furnace();
    let decoded = FurnaceState::decode(&FURNACE_STATE_WIRE).expect("decode");
    assert_round_trip(
        "furnace state",
        &FURNACE_STATE_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| FurnaceState::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(FURNACE_STATE_WIRE.len(), 36);
    // The furnace reference is the shared 18-byte layout, preserved whole.
    assert_eq!(&FURNACE_STATE_WIRE[..18], &FURNACE_REF_WIRE);
    assert_eq!(decoded.furnace, FURNACE_REF);
    assert_eq!(decoded.progress_ticks, 199);
    assert_eq!(decoded.burn_ticks, 1600);
}

#[test]
fn inventory_publication_chest_state_round_trips_through_the_fallible_surface() {
    let record = canonical_chest();
    let decoded = ChestState::decode(&CHEST_STATE_WIRE).expect("decode");
    assert_round_trip(
        "chest state",
        &CHEST_STATE_WIRE,
        || record.validate(),
        || record.encoded_len(),
        |dst| record.encode_into(dst),
        || record.encode(),
        |payload| ChestState::decode(payload).map(|_| ()),
        || decoded.encode(),
        || decoded == record,
    );
    assert_eq!(CHEST_STATE_WIRE.len(), 153);
    assert_eq!(&CHEST_STATE_WIRE[..18], &CHEST_REF_WIRE);
    assert_eq!(decoded.chest, CHEST_REF);
}

#[test]
fn inventory_publication_container_closed_round_trips_through_the_fallible_surface() {
    // The furnace reference is the decode literal, the chest reference the
    // encode twin: the family is container-neutral, so both kinds publish.
    let furnace = canonical_closed_furnace();
    let decoded_furnace = ContainerClosed::decode(&FURNACE_REF_WIRE).expect("decode");
    assert_round_trip(
        "container closed furnace",
        &FURNACE_REF_WIRE,
        || furnace.validate(),
        || furnace.encoded_len(),
        |dst| furnace.encode_into(dst),
        || furnace.encode(),
        |payload| ContainerClosed::decode(payload).map(|_| ()),
        || decoded_furnace.encode(),
        || decoded_furnace == furnace,
    );
    let chest = canonical_closed_chest();
    let decoded_chest = ContainerClosed::decode(&CHEST_REF_WIRE).expect("decode");
    assert_round_trip(
        "container closed chest",
        &CHEST_REF_WIRE,
        || chest.validate(),
        || chest.encoded_len(),
        |dst| chest.encode_into(dst),
        || chest.encode(),
        |payload| ContainerClosed::decode(payload).map(|_| ()),
        || decoded_chest.encode(),
        || decoded_chest == chest,
    );
    assert_eq!(FURNACE_REF_WIRE.len(), 18);
    assert_eq!(CHEST_REF_WIRE.len(), 18);
}

#[test]
fn inventory_publication_crafting_state_personal_residue_refuses_every_extension_slot() {
    // A personal grid may not carry residue in any cell beyond its own size,
    // and a workbench admits all nine cells. The residue is applied as a
    // post-construction mutation, because the constructor's own gate refuses
    // the record the public field can still reach.
    for slot in 4..9 {
        let mut base = [ItemStack::EMPTY; 9];
        base[0] = stack(1, 1, 0);
        let mut state =
            CraftingState::new(CRAFTING_GRID_SIZE_PERSONAL, base, ItemStack::EMPTY).expect("state");
        state.slots[slot] = stack(1, 1, 0);
        assert_eq!(
            state.validate(),
            Err(ProtocolError::InvalidRange),
            "personal residue in slot {slot} must be refused"
        );
        assert_eq!(
            state.encoded_len(),
            Err(ProtocolError::InvalidRange),
            "personal residue in slot {slot} must never reach a size decision"
        );
        assert_eq!(
            state.encode(),
            Err(ProtocolError::InvalidRange),
            "personal residue in slot {slot} must never be published"
        );
        // The same nine slots are admitted at the workbench size.
        state.size = CRAFTING_GRID_SIZE_WORKBENCH;
        assert!(
            state.validate().is_ok(),
            "the workbench admits all nine cells"
        );
    }
    // An unknown size is a range violation on every entry point, which is the
    // category the Go validator publishes for the same bytes.
    for size in [0u8, 1, 4, 255] {
        let mut state = canonical_crafting();
        state.size = size;
        assert_eq!(
            state.validate(),
            Err(ProtocolError::InvalidRange),
            "size {size} must be refused"
        );
        assert_eq!(
            state.encode(),
            Err(ProtocolError::InvalidRange),
            "size {size} must never be published"
        );
    }
}

#[test]
fn inventory_publication_furnace_state_invents_no_timer_relation() {
    // The canonical vector is at the admitted timer bounds, and the Go
    // validator carries no relation between the timers and the stacks: an idle
    // furnace with zero progress, zero burn and empty whitelisted slots is
    // publishable beside the active canonical vector.
    let active = canonical_furnace();
    assert!(active.validate().is_ok());
    assert_eq!(active.progress_ticks, FURNACE_SMELT_TICKS - 1);
    assert_eq!(active.burn_ticks, FURNACE_BURN_TICKS);
    let idle = FurnaceState::new(
        FURNACE_REF,
        ItemStack::EMPTY,
        ItemStack::EMPTY,
        ItemStack::EMPTY,
        0,
        0,
    )
    .expect("the idle furnace is valid");
    assert!(idle.validate().is_ok());
    assert_eq!(
        idle.encode().expect("encode the idle furnace").len(),
        36,
        "the idle furnace publishes the same stride"
    );
    // The same idle stacks beside a full burn time and zero progress are also
    // admissible: no relation ties burn time to progress.
    let burning = FurnaceState::new(
        FURNACE_REF,
        ItemStack::EMPTY,
        ItemStack::EMPTY,
        ItemStack::EMPTY,
        0,
        FURNACE_BURN_TICKS,
    )
    .expect("the burning furnace is valid");
    assert!(burning.validate().is_ok());
    // Progress alone at the ceiling and burn alone above the ceiling are the
    // two timer boundaries the Go validator publishes.
    let mut at_limit = active.clone();
    at_limit.progress_ticks = FURNACE_SMELT_TICKS;
    assert_eq!(at_limit.validate(), Err(ProtocolError::InvalidRange));
    let mut above = active;
    above.burn_ticks = FURNACE_BURN_TICKS + 1;
    assert_eq!(above.validate(), Err(ProtocolError::InvalidRange));
}

/// One mutate-after-construction case per family: the invalid value a public
/// mutable field is set to after construction, and the error the surface has
/// to report for it before any size or capacity decision.
#[test]
fn inventory_publication_invalid_value_wins_over_short_capacity() {
    let mut bad_selected = canonical_inventory();
    bad_selected.selected = 9;
    let mut bad_size = canonical_crafting();
    bad_size.size = 0;
    let mut bad_residue = canonical_crafting();
    bad_residue.slots[4] = stack(1, 1, 0);
    let mut bad_progress = canonical_furnace();
    bad_progress.progress_ticks = FURNACE_SMELT_TICKS;
    let mut bad_fuel = canonical_furnace();
    bad_fuel.fuel = stack(1, 1, 0);
    let mut bad_kind = canonical_furnace();
    bad_kind.furnace.kind = CONTAINER_KIND_CHEST;
    let mut bad_chest_kind = canonical_chest();
    bad_chest_kind.chest.kind = CONTAINER_KIND_FURNACE;
    let mut bad_generation = canonical_chest();
    bad_generation.chest.generation = 0;
    let mut bad_slot = canonical_chest();
    bad_slot.chest.slot = CHESTS_PER_CHUNK;
    let mut bad_none = canonical_closed_furnace();
    bad_none.container.generation = 0;
    let mut bad_unknown_kind = canonical_closed_furnace();
    bad_unknown_kind.container.kind = 2;

    let cases: Vec<(
        &str,
        ProtocolError,
        usize,
        Vec<Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>>,
    )> = vec![
        (
            "inventory state selected above the hotbar",
            ProtocolError::InvalidRange,
            INVENTORY_STATE_WIRE.len(),
            {
                let record = bad_selected.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "crafting state unknown size",
            ProtocolError::InvalidRange,
            CRAFTING_STATE_WIRE.len(),
            {
                let record = bad_size.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "crafting state personal residue",
            ProtocolError::InvalidRange,
            CRAFTING_STATE_WIRE.len(),
            {
                let record = bad_residue.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "furnace state progress at the requirement",
            ProtocolError::InvalidRange,
            FURNACE_STATE_WIRE.len(),
            {
                let record = bad_progress.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "furnace state fuel is not coal",
            ProtocolError::InvalidRange,
            FURNACE_STATE_WIRE.len(),
            {
                let record = bad_fuel.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "furnace state reference names a chest",
            ProtocolError::InvalidEnum,
            FURNACE_STATE_WIRE.len(),
            {
                let record = bad_kind.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "chest state reference names a furnace",
            ProtocolError::InvalidEnum,
            CHEST_STATE_WIRE.len(),
            {
                let record = bad_chest_kind.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "chest state reference generation is zero",
            ProtocolError::InvalidRange,
            CHEST_STATE_WIRE.len(),
            {
                let record = bad_generation.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "chest state reference slot above the array",
            ProtocolError::InvalidRange,
            CHEST_STATE_WIRE.len(),
            {
                let record = bad_slot.clone();
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "container closed generation is zero",
            ProtocolError::InvalidRange,
            FURNACE_REF_WIRE.len(),
            {
                let record = bad_none;
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
        (
            "container closed unknown kind",
            ProtocolError::InvalidEnum,
            FURNACE_REF_WIRE.len(),
            {
                let record = bad_unknown_kind;
                vec![Box::new(move |dst: &mut [u8]| record.encode_into(dst))]
            },
        ),
    ];

    for (label, error, needed, encoders) in cases {
        for encode_into in &encoders {
            // The value error is reported for every destination shape, so an
            // invalid record never reaches the capacity decision.
            for available in [0usize, needed - 1, needed] {
                let mut dst = vec![SENTINEL; available];
                assert_eq!(
                    encode_into(&mut dst),
                    Err(error),
                    "{label}: a destination of {available} bytes masks the value error"
                );
                assert!(
                    dst.iter().all(|byte| *byte == SENTINEL),
                    "{label}: a destination of {available} bytes was modified on a value refusal"
                );
            }
            // The exact destination still refuses the value error.
            let mut exact = vec![0u8; needed];
            assert_eq!(
                encode_into(&mut exact),
                Err(error),
                "{label}: the exact destination still refuses the value error"
            );
            assert!(
                exact.iter().all(|byte| *byte == 0),
                "{label}: the exact destination was modified on a value refusal"
            );
        }
    }

    // Every allocating wrapper refuses the same mutations, so no entry point
    // publishes a mutated record.
    assert_eq!(bad_selected.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_size.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_residue.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_progress.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_fuel.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_kind.encode(), Err(ProtocolError::InvalidEnum));
    assert_eq!(bad_chest_kind.encode(), Err(ProtocolError::InvalidEnum));
    assert_eq!(bad_generation.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_slot.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_none.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(bad_unknown_kind.encode(), Err(ProtocolError::InvalidEnum));
}

#[test]
fn inventory_publication_decode_rejects_every_proper_truncation() {
    let cases: Vec<(&str, Vec<u8>, fn(&[u8]) -> Result<(), ProtocolError>)> = vec![
        (
            "inventory state",
            INVENTORY_STATE_WIRE.to_vec(),
            |payload| InventoryState::decode(payload).map(|_| ()),
        ),
        ("crafting state", CRAFTING_STATE_WIRE.to_vec(), |payload| {
            CraftingState::decode(payload).map(|_| ())
        }),
        ("furnace state", FURNACE_STATE_WIRE.to_vec(), |payload| {
            FurnaceState::decode(payload).map(|_| ())
        }),
        ("chest state", CHEST_STATE_WIRE.to_vec(), |payload| {
            ChestState::decode(payload).map(|_| ())
        }),
        ("container closed", FURNACE_REF_WIRE.to_vec(), |payload| {
            ContainerClosed::decode(payload).map(|_| ())
        }),
    ];
    for (label, payload, decode) in cases {
        // Every cut is inside a fixed array or the trailing timer fields, and
        // the checked indexed reads report the same boundary the Go decoder
        // answers with its short-input sentinel.
        for cut in 0..payload.len() {
            let error = decode(&payload[..cut])
                .err()
                .unwrap_or_else(|| panic!("{label}: truncation at {cut} bytes decoded"));
            assert_eq!(
                error,
                ProtocolError::Truncated,
                "{label}: truncation at {cut} bytes reports {error:?}"
            );
        }
    }
}

#[test]
fn inventory_publication_decode_rejects_one_trailing_byte() {
    let cases: Vec<(&str, Vec<u8>, fn(&[u8]) -> Result<(), ProtocolError>)> = vec![
        (
            "inventory state",
            {
                let mut wire = INVENTORY_STATE_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| InventoryState::decode(payload).map(|_| ()),
        ),
        (
            "crafting state",
            {
                let mut wire = CRAFTING_STATE_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| CraftingState::decode(payload).map(|_| ()),
        ),
        (
            "furnace state",
            {
                let mut wire = FURNACE_STATE_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| FurnaceState::decode(payload).map(|_| ()),
        ),
        (
            "chest state",
            {
                let mut wire = CHEST_STATE_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| ChestState::decode(payload).map(|_| ()),
        ),
        (
            "container closed",
            {
                let mut wire = FURNACE_REF_WIRE.to_vec();
                wire.push(0x00);
                wire
            },
            |payload| ContainerClosed::decode(payload).map(|_| ()),
        ),
    ];
    for (label, payload, decode) in cases {
        let error = decode(&payload)
            .err()
            .unwrap_or_else(|| panic!("{label}: one trailing byte decoded"));
        assert_eq!(
            error,
            ProtocolError::TrailingBytes,
            "{label}: one trailing byte reports {error:?}"
        );
    }
}

/// Copies one canonical literal and hands it to one boundary mutator, so a
/// single-violation wire mutation is named by its mutator.
fn mutate_inventory_wire(mutator: impl FnOnce(&mut [u8])) -> Vec<u8> {
    let mut wire = INVENTORY_STATE_WIRE.to_vec();
    mutator(&mut wire);
    wire
}

/// Copies one reference-carrying literal and hands its leading 18-byte
/// reference to one field mutator, leaving every other field in place.
fn with_reference(wire: &[u8], mutator: impl FnOnce(&mut [u8; 18])) -> Vec<u8> {
    let mut owned = wire.to_vec();
    let (reference, _) = owned.split_at_mut(18);
    let reference: &mut [u8; 18] = reference.try_into().expect("the reference stride is 18");
    mutator(reference);
    owned
}

#[test]
fn inventory_publication_decode_rejects_the_pinned_invalid_values() {
    // The matrix mirrors the corpus negatives. Every entry is a single
    // violation, and the variant the Rust decoder reports is the one the Go
    // producer records for the same bytes.

    // InventoryState: the selected index above the hotbar bound, the tool
    // slot's durability at zero on a durable item, and an unregistered item
    // number.
    assert_eq!(
        InventoryState::decode(&mutate_inventory_wire(|wire| wire[0] = 9)),
        Err(ProtocolError::InvalidRange),
        "selected index nine"
    );
    assert_eq!(
        InventoryState::decode(&mutate_inventory_wire(|wire| {
            wire[44..46].copy_from_slice(&0u16.to_le_bytes());
        })),
        Err(ProtocolError::InvalidRange),
        "a durable tool at durability zero"
    );
    assert_eq!(
        InventoryState::decode(&mutate_inventory_wire(|wire| {
            wire[46..48].copy_from_slice(&200u16.to_le_bytes());
        })),
        Err(ProtocolError::InvalidEnum),
        "an unregistered item number"
    );

    // CraftingState: the two unknown sizes and the personal residue cell.
    assert_eq!(
        CraftingState::decode(&{
            let mut wire = CRAFTING_STATE_WIRE;
            wire[0] = 0;
            wire.to_vec()
        }),
        Err(ProtocolError::InvalidRange),
        "size zero"
    );
    assert_eq!(
        CraftingState::decode(&{
            let mut wire = CRAFTING_STATE_WIRE;
            wire[0] = 4;
            wire.to_vec()
        }),
        Err(ProtocolError::InvalidRange),
        "size four"
    );
    assert_eq!(
        CraftingState::decode(&{
            let mut wire = CRAFTING_STATE_WIRE;
            wire[21..26].copy_from_slice(&[0x01, 0x00, 0x01, 0x00, 0x00]);
            wire.to_vec()
        }),
        Err(ProtocolError::InvalidRange),
        "personal residue in slot four"
    );

    // FurnaceState: the two timer bounds, the wrong-kind reference, the zero
    // generation, and the fuel and output whitelists.
    assert_eq!(
        FurnaceState::decode(&{
            let mut wire = FURNACE_STATE_WIRE;
            wire[33] = FURNACE_SMELT_TICKS;
            wire.to_vec()
        }),
        Err(ProtocolError::InvalidRange),
        "progress at the requirement"
    );
    assert_eq!(
        FurnaceState::decode(&{
            let mut wire = FURNACE_STATE_WIRE;
            wire[34..36].copy_from_slice(&(FURNACE_BURN_TICKS + 1).to_le_bytes());
            wire.to_vec()
        }),
        Err(ProtocolError::InvalidRange),
        "burn time above the ceiling"
    );
    assert_eq!(
        FurnaceState::decode(&with_reference(&FURNACE_STATE_WIRE, |reference| {
            reference[12] = CONTAINER_KIND_CHEST;
        })),
        Err(ProtocolError::InvalidEnum),
        "the reference names a chest"
    );
    assert_eq!(
        FurnaceState::decode(&with_reference(&FURNACE_STATE_WIRE, |reference| {
            reference[14..18].copy_from_slice(&0u32.to_le_bytes());
        })),
        Err(ProtocolError::InvalidRange),
        "the reference generation is zero"
    );
    assert_eq!(
        FurnaceState::decode(&{
            let mut wire = FURNACE_STATE_WIRE;
            wire[23..28].copy_from_slice(&[0x01, 0x00, 0x01, 0x00, 0x00]);
            wire.to_vec()
        }),
        Err(ProtocolError::InvalidRange),
        "the fuel slot holds stone"
    );
    assert_eq!(
        FurnaceState::decode(&{
            let mut wire = FURNACE_STATE_WIRE;
            wire[28..33].copy_from_slice(&[0x01, 0x00, 0x01, 0x00, 0x00]);
            wire.to_vec()
        }),
        Err(ProtocolError::InvalidRange),
        "the output slot holds stone"
    );

    // ChestState: the wrong-kind reference, the reference slot above the
    // per-chunk array, and an unregistered item number in one slot.
    assert_eq!(
        ChestState::decode(&with_reference(&CHEST_STATE_WIRE, |reference| {
            reference[12] = CONTAINER_KIND_FURNACE;
        })),
        Err(ProtocolError::InvalidEnum),
        "the reference names a furnace"
    );
    assert_eq!(
        ChestState::decode(&with_reference(&CHEST_STATE_WIRE, |reference| {
            reference[13] = 16;
        })),
        Err(ProtocolError::InvalidRange),
        "the reference slot is above the chest array"
    );
    assert_eq!(
        ChestState::decode(&{
            let mut wire = CHEST_STATE_WIRE;
            wire[18..20].copy_from_slice(&200u16.to_le_bytes());
            wire.to_vec()
        }),
        Err(ProtocolError::InvalidEnum),
        "an unregistered item number"
    );

    // ContainerClosed: the exact all-zero record is refused through its zero
    // generation, a foreign dimension is a range violation, and an unknown
    // kind is the enum boundary.
    assert_eq!(
        ContainerClosed::decode(&[0u8; 18]),
        Err(ProtocolError::InvalidRange),
        "the exact all-zero absent record"
    );
    assert_eq!(
        ContainerClosed::decode(&{
            let mut wire = FURNACE_REF_WIRE;
            wire[0..4].copy_from_slice(&(-1i32).to_le_bytes());
            wire.to_vec()
        }),
        Err(ProtocolError::InvalidRange),
        "a foreign dimension"
    );
    assert_eq!(
        ContainerClosed::decode(&{
            let mut wire = CHEST_REF_WIRE;
            wire[12] = 2;
            wire.to_vec()
        }),
        Err(ProtocolError::InvalidEnum),
        "an unknown container kind"
    );
}

#[test]
fn inventory_publication_encode_into_leaves_a_short_destination_unchanged() {
    let cases: Vec<(
        &str,
        usize,
        Box<dyn Fn(&mut [u8]) -> Result<usize, ProtocolError>>,
    )> = vec![
        (
            "inventory state",
            INVENTORY_STATE_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_inventory().encode_into(dst)),
        ),
        (
            "crafting state",
            CRAFTING_STATE_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_crafting().encode_into(dst)),
        ),
        (
            "furnace state",
            FURNACE_STATE_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_furnace().encode_into(dst)),
        ),
        (
            "chest state",
            CHEST_STATE_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_chest().encode_into(dst)),
        ),
        (
            "container closed",
            FURNACE_REF_WIRE.len(),
            Box::new(|dst: &mut [u8]| canonical_closed_furnace().encode_into(dst)),
        ),
    ];
    for (label, length, encode_into) in cases {
        for available in [0usize, length - 1, length - 3] {
            let mut dst = vec![SENTINEL; available];
            let error = encode_into(&mut dst).expect_err("a short destination must be refused");
            assert_eq!(
                error,
                ProtocolError::OutputTooSmall {
                    needed: length,
                    available
                },
                "{label}: short destination of {available} bytes reports the wrong error"
            );
            assert!(
                dst.iter().all(|byte| *byte == SENTINEL),
                "{label}: short destination of {available} bytes was modified"
            );
        }
    }
}

#[test]
fn inventory_publication_the_stack_rule_boundaries_stay_pinned() {
    // The domain item-stack rule is the single slot gate, and its boundaries
    // are pinned through the decode path so a wire value the rule refuses
    // never reaches the family gate.
    let mut wire = INVENTORY_STATE_WIRE.to_vec();
    // A full stack of a stackable item is admissible.
    wire[1..6].copy_from_slice(&[0x01, 0x00, 0x40, 0x00, 0x00]);
    assert!(
        InventoryState::decode(&wire).is_ok(),
        "a full stack is admissible"
    );
    // One above the stack limit is a count violation.
    wire[1..6].copy_from_slice(&[0x01, 0x00, 0x41, 0x00, 0x00]);
    assert_eq!(
        InventoryState::decode(&wire),
        Err(ProtocolError::InvalidRange),
        "one above the stack limit"
    );
    // A zero count is a count violation.
    wire[1..6].copy_from_slice(&[0x01, 0x00, 0x00, 0x00, 0x00]);
    assert_eq!(
        InventoryState::decode(&wire),
        Err(ProtocolError::InvalidRange),
        "a zero count"
    );
    // The single-count tool admits exactly one, and its durability spans
    // 1..=131 with both boundaries admitted and one above refused. Slot 0
    // returns to its reviewed stone stack first, so the mutations below stay
    // single violations of the tool slot alone.
    wire[1..6].copy_from_slice(&[0x01, 0x00, 0x05, 0x00, 0x00]);
    wire[41..46].copy_from_slice(&[0x0a, 0x00, 0x01, 0x01, 0x00]);
    assert!(
        InventoryState::decode(&wire).is_ok(),
        "the tool at durability one is admissible"
    );
    wire[41..46].copy_from_slice(&[0x0a, 0x00, 0x01, 0x83, 0x00]);
    assert!(
        InventoryState::decode(&wire).is_ok(),
        "the tool at durability 131 is admissible"
    );
    wire[41..46].copy_from_slice(&[0x0a, 0x00, 0x01, 0x84, 0x00]);
    assert_eq!(
        InventoryState::decode(&wire),
        Err(ProtocolError::InvalidRange),
        "the tool above durability 131"
    );
    wire[41..46].copy_from_slice(&[0x0a, 0x00, 0x02, 0x00, 0x01]);
    assert_eq!(
        InventoryState::decode(&wire),
        Err(ProtocolError::InvalidRange),
        "the tool at count two"
    );
    // A nondurable item with a nonzero durability is a durability violation.
    wire[1..6].copy_from_slice(&[0x01, 0x00, 0x01, 0x00, 0x01]);
    assert_eq!(
        InventoryState::decode(&wire),
        Err(ProtocolError::InvalidRange),
        "stone with a durability"
    );
    // The absent item number admits exactly the empty triple.
    wire[1..6].copy_from_slice(&[0x00, 0x00, 0x01, 0x00, 0x00]);
    assert_eq!(
        InventoryState::decode(&wire),
        Err(ProtocolError::InvalidRange),
        "the absent item with a count"
    );
    wire[1..6].copy_from_slice(&[0x00, 0x00, 0x00, 0x00, 0x01]);
    assert_eq!(
        InventoryState::decode(&wire),
        Err(ProtocolError::InvalidRange),
        "the absent item with a durability"
    );
}

#[test]
fn inventory_publication_packet_ids_are_pinned() {
    assert_eq!(InventoryState::PACKET_ID, 10);
    assert_eq!(CraftingState::PACKET_ID, 21);
    assert_eq!(FurnaceState::PACKET_ID, 13);
    assert_eq!(ChestState::PACKET_ID, 15);
    assert_eq!(ContainerClosed::PACKET_ID, 14);
}
