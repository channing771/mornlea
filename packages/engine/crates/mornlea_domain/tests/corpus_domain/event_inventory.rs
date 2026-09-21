//! Topic module for domain.event inventory corpus cases.
//!
//! The five rules here are the records an authoritative session publishes to
//! the one player that owns them: the complete inventory, the current crafting
//! grid, the furnace or chest a player is viewing, and the notification that a
//! viewed container went away. Each executor parses the case input, selects
//! the Go rejection rule from the raw values in the producer's stated
//! precedence, and then verifies that the applicable Rust constructor returns
//! the mapped `DomainError`: a classified acceptance must construct, and a
//! classified rejection must fail with exactly the mapped variant. Accepted
//! fields are read from the constructed Rust values; a rejected value has no
//! getters, so its projection is built from the parsed input instead.
//!
//! The raw precedence is what disambiguates the field-context rules: one
//! `DomainError` can name several slots (`InvalidFurnaceSlot` covers the input,
//! fuel and output whitelists; `InvalidFurnaceTimers` covers both timers), so
//! only the raw field and its order select the exact published rule. Slot
//! values themselves are never re-validated here: `ItemStack::try_new` and the
//! furnace and chest constructors enforce the registered-item, count,
//! durability and whitelist rules through the shared tables, and this module
//! only maps the returned variants.

use super::support::{
    DispatchError, ExecutedCase, JsonMap, assert_domain_normalized, exact_array, execute_topic,
    input_object, invalid_case, normalized_error, normalized_ok, optional_object, required_array,
    required_object, required_string, required_u8, required_u16, required_u32, value_i32,
    value_object, value_u8, value_u16,
};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};
use mornlea_domain::{
    ChestState, ChestStateParts, ChunkPos, ContainerClosed, ContainerKind, ContainerRef,
    CraftingSize, CraftingState, CraftingStateParts, DomainError, FurnaceState, FurnaceStateParts,
    HotbarSlot, InventoryState, InventoryStateParts, ItemStack, is_smelting_product,
    smelting_output,
};
use serde_json::Value;

pub const EXPECTED_COUNT: usize = 33;

const RULES: &[&str] = &[
    "inventory-state",
    "crafting-state",
    "furnace-state",
    "chest-state",
    "container-closed",
];

pub fn owns(case: &FrozenCase) -> bool {
    if case.consumer != CorpusConsumer::Domain || case.family != "domain.event" {
        return false;
    }
    let rule = match case
        .input_json
        .as_ref()
        .and_then(|v| v.get("rule"))
        .and_then(|v| v.as_str())
    {
        Some(r) => r,
        None => return false,
    };
    RULES.contains(&rule)
}

pub fn execute(case: &FrozenCase) -> Result<serde_json::Value, DispatchError> {
    let input = input_object(case)?;
    let rule = required_string(case, input, "rule")?;
    match rule {
        "inventory-state" => execute_inventory_state(case, input),
        "crafting-state" => execute_crafting_state(case, input),
        "furnace-state" => execute_furnace_state(case, input),
        "chest-state" => execute_chest_state(case, input),
        "container-closed" => execute_container_closed(case, input),
        unknown => Err(invalid_case(
            case,
            format!("unknown inventory event rule '{unknown}'"),
        )),
    }
}

/// One classified rejection: its normalized category, its exact Go rule name,
/// and the `DomainError` the owning constructor must return.
///
/// The rule is a `String` because several inventory rules are indexed by slot
/// (`inventory_state.hotbar_slot_3`). `error` is `None` for a
/// classifier-only rejection whose bad raw value cannot enter any Rust type,
/// so no constructor verdict exists to check.
struct Rejection {
    category: &'static str,
    rule: String,
    error: Option<DomainError>,
}

/// Authoritative fixed slot counts, from the Go `core` inventory, crafting and
/// chest layouts. `HotbarSlot::COUNT` is the exported domain constant for the
/// hotbar; the rest are private in the crate, so this module names them for
/// the fixed-array parse it owns.
const BACKPACK_SLOTS: usize = 27;
const CRAFTING_GRID_SLOTS: usize = 9;
const CHEST_SLOTS: usize = 27;

/// Crafting grid side lengths the Go packet publishes, from the Go producer's
/// `domainEventInventoryCraftingPersonal`/`Workbench`.
const CRAFTING_PERSONAL: u8 = 2;
const CRAFTING_WORKBENCH: u8 = 3;

/// Usable cells of the personal grid, the side length squared. The five cells
/// beyond them have to stay empty on a personal grid.
const PERSONAL_GRID_USABLE: usize = CRAFTING_PERSONAL as usize * CRAFTING_PERSONAL as usize;

/// Fixed furnace array size of one chunk, from the Go `core.FurnacesPerChunk`.
const FURNACES_PER_CHUNK: u8 = 32;

/// Fixed chest array size of one chunk, from the Go `core.ChestsPerChunk`.
const CHESTS_PER_CHUNK: u8 = 16;

/// Furnace smelt requirement, from the Go `core.FurnaceSmeltTicks`. The
/// progress has to stay strictly below it.
const FURNACE_SMELT_TICKS: u8 = 200;

/// Furnace burn maximum, from the Go `core.FurnaceBurnTicks`. Unlike the
/// progress bound this one is inclusive.
const FURNACE_BURN_TICKS: u16 = 1600;

/// The one fuel a furnace accepts beside the empty stack, from the Go
/// `core.ItemCoal`. The furnace constructor keeps its own private copy; this
/// one only feeds the classifier that selects the exact slot rule.
const FURNACE_FUEL_COAL: u16 = 5;

/// The absent item number, from the Go `core.ItemNone`.
const ITEM_NONE: u16 = 0;

/// The parsed raw shape of one slot value, beside the `ItemStack::try_new`
/// verdict for that triple.
struct ParsedStack {
    raw: RawStack,
    resolved: Result<ItemStack, DomainError>,
}

/// The raw slot triple exactly as the frozen envelope carries it.
#[derive(Clone, Copy, Eq, PartialEq)]
struct RawStack {
    item: u16,
    count: u8,
    durability: u16,
}

impl RawStack {
    fn is_empty(self) -> bool {
        self == RawStack {
            item: ITEM_NONE,
            count: 0,
            durability: 0,
        }
    }
}

/// The raw container reference exactly as the frozen envelope carries it,
/// including a kind byte the closed Rust enum cannot represent.
struct RawContainer {
    kind: u8,
    chunk: [i32; 2],
    slot: u8,
    generation: u32,
}

impl RawContainer {
    /// The `ContainerKind` this raw byte names, when it names a published one.
    fn known_kind(&self) -> Option<ContainerKind> {
        match self.kind {
            0 => Some(ContainerKind::Furnace),
            1 => Some(ContainerKind::Chest),
            _ => None,
        }
    }
}

/// Parses one slot triple from its envelope object and resolves it through
/// the real `ItemStack` constructor. A missing field or a value outside its
/// declared width is a hard schema failure, never a normalized rejection.
fn stack_from_object(
    case: &FrozenCase,
    path: &str,
    object: &JsonMap,
) -> Result<ParsedStack, DispatchError> {
    let raw = RawStack {
        item: value_u16(
            case,
            &format!("{path}.item"),
            required_field(case, path, object, "item")?,
        )?,
        count: value_u8(
            case,
            &format!("{path}.count"),
            required_field(case, path, object, "count")?,
        )?,
        durability: value_u16(
            case,
            &format!("{path}.durability"),
            required_field(case, path, object, "durability")?,
        )?,
    };
    let resolved = ItemStack::try_new(raw.item, raw.count, raw.durability);
    Ok(ParsedStack { raw, resolved })
}

/// Looks up one nested object field, failing closed when it is absent.
fn required_field<'a>(
    case: &FrozenCase,
    path: &str,
    object: &'a JsonMap,
    key: &str,
) -> Result<&'a Value, DispatchError> {
    object
        .get(key)
        .ok_or_else(|| invalid_case(case, format!("missing required field '{path}.{key}'")))
}

/// Parses one exact fixed-count slot array, preserving submitted order.
fn parse_stack_array(
    case: &FrozenCase,
    input: &JsonMap,
    key: &str,
    expected: usize,
) -> Result<Vec<ParsedStack>, DispatchError> {
    let values = required_array(case, input, key)?;
    if values.len() != expected {
        return Err(invalid_case(
            case,
            format!("requires {expected} '{key}' stacks, got {}", values.len()),
        ));
    }
    values
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("{key}[{index}]");
            let object = value_object(case, &path, value)?;
            stack_from_object(case, &path, object)
        })
        .collect()
}

/// Parses one required container reference. The raw kind byte is preserved
/// because a value outside the two published kinds is a normalized
/// rejection, not a schema failure.
fn parse_container(case: &FrozenCase, input: &JsonMap) -> Result<RawContainer, DispatchError> {
    let object = required_object(case, input, "container")?;
    let kind = required_u8(case, object, "kind")?;
    let chunk_values = required_array(case, object, "chunk")?;
    let chunk_slots = exact_array::<2>(case, "container.chunk", chunk_values)?;
    let chunk = [
        value_i32(case, "container.chunk[0]", &chunk_slots[0])?,
        value_i32(case, "container.chunk[1]", &chunk_slots[1])?,
    ];
    Ok(RawContainer {
        kind,
        chunk,
        slot: required_u8(case, object, "slot")?,
        generation: required_u32(case, object, "generation")?,
    })
}

/// The category one stack rejection publishes: an unregistered item number is
/// an enumeration failure, every other stack rule a value failure. This is
/// the values family's classification, applied to the slot rules this family
/// reuses.
fn stack_rejection(parsed: &ParsedStack) -> Option<(&'static str, DomainError)> {
    match parsed.resolved {
        Err(DomainError::InvalidItem) => Some(("invalid-enum", DomainError::InvalidItem)),
        Err(error @ (DomainError::InvalidCount | DomainError::InvalidDurability)) => {
            Some(("invalid-value", error))
        }
        _ => None,
    }
}

/// Constructs one container reference from the parsed raw shape, failing
/// closed when the classification said this reference should have built.
fn construct_container_ref(
    case: &FrozenCase,
    subject: &str,
    container: &RawContainer,
    kind: ContainerKind,
) -> Result<ContainerRef, DispatchError> {
    ContainerRef::try_new(
        ChunkPos::new(container.chunk[0], container.chunk[1]),
        kind,
        container.slot,
        container.generation,
    )
    .map_err(|error| {
        invalid_case(
            case,
            format!(
                "{subject} container reference construction disagrees with the classification: {error:?}"
            ),
        )
    })
}

/// Unwraps one parsed stack whose classification said it must resolve,
/// failing closed otherwise.
fn resolved_stack(
    case: &FrozenCase,
    subject: &str,
    parsed: &ParsedStack,
) -> Result<ItemStack, DispatchError> {
    parsed.resolved.map_err(|error| {
        invalid_case(
            case,
            format!("{subject} slot construction disagrees with the classification: {error:?}"),
        )
    })
}

/// Unwraps every parsed stack of an array whose classification said the array
/// resolves, preserving order.
fn resolved_stacks(
    case: &FrozenCase,
    subject: &str,
    parsed: &[ParsedStack],
) -> Result<Vec<ItemStack>, DispatchError> {
    parsed
        .iter()
        .map(|stack| resolved_stack(case, subject, stack))
        .collect()
}

/// Fails closed when the Go-rule classification and the Rust constructor
/// disagree about one case input.
///
/// A disagreement is a contract conflict rather than a corpus expectation: it
/// means one side's rule moved, so reporting `InvalidCase` stops the topic
/// instead of publishing whichever side happened to be consulted first.
fn classification_conflict(case: &FrozenCase, subject: &str, rule: &str) -> DispatchError {
    let classified = if rule.is_empty() { "accept" } else { rule };
    invalid_case(
        case,
        format!("{subject} classification '{classified}' disagrees with the Rust constructor"),
    )
}

/// Renders one raw kind byte as the stable text the normalized corpus
/// vocabulary uses. An unknown kind names itself as such rather than
/// collapsing onto a published one.
fn raw_kind_text(kind: u8) -> &'static str {
    match kind {
        0 => "furnace",
        1 => "chest",
        _ => "unknown",
    }
}

/// The stable text one constructed reference publishes.
fn kind_text(kind: ContainerKind) -> &'static str {
    match kind {
        ContainerKind::Furnace => "furnace",
        ContainerKind::Chest => "chest",
    }
}

/// Renders one raw slot triple in the normalized shape.
fn raw_stack_fields(stack: &RawStack) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("item".to_string(), Value::from(stack.item));
    fields.insert("count".to_string(), Value::from(stack.count));
    fields.insert("durability".to_string(), Value::from(stack.durability));
    fields
}

/// Renders one raw slot array in order.
fn raw_stack_list(stacks: &[ParsedStack]) -> Value {
    Value::Array(
        stacks
            .iter()
            .map(|stack| Value::Object(raw_stack_fields(&stack.raw)))
            .collect(),
    )
}

/// Renders one constructed slot value from its getters.
fn stack_fields(stack: ItemStack) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("item".to_string(), Value::from(stack.item()));
    fields.insert("count".to_string(), Value::from(stack.count()));
    fields.insert("durability".to_string(), Value::from(stack.durability()));
    fields
}

/// Renders one constructed slot array in order.
fn stack_list(stacks: &[ItemStack]) -> Value {
    Value::Array(
        stacks
            .iter()
            .map(|stack| Value::Object(stack_fields(*stack)))
            .collect(),
    )
}

/// Renders one raw container reference in the normalized shape.
fn raw_container_fields(container: &RawContainer) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert(
        "kind".to_string(),
        Value::String(raw_kind_text(container.kind).to_string()),
    );
    fields.insert(
        "chunk".to_string(),
        Value::Array(container.chunk.iter().map(|c| Value::from(*c)).collect()),
    );
    fields.insert("slot".to_string(), Value::from(container.slot));
    fields.insert("generation".to_string(), Value::from(container.generation));
    fields
}

/// Renders one constructed container reference from its getters.
fn container_fields(reference: ContainerRef) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert(
        "kind".to_string(),
        Value::String(kind_text(reference.kind()).to_string()),
    );
    fields.insert(
        "chunk".to_string(),
        Value::Array(vec![
            Value::from(reference.chunk().x()),
            Value::from(reference.chunk().z()),
        ]),
    );
    fields.insert("slot".to_string(), Value::from(reference.slot()));
    fields.insert(
        "generation".to_string(),
        Value::from(reference.generation()),
    );
    fields
}

/// Executes one `inventory-state` case, whose payload is the selected hotbar
/// index and the two fixed slot arrays.
///
/// The rule is selected from the raw values in the Go validator's precedence:
/// the selected index first, then every hotbar slot, then every backpack
/// slot. `InventoryState::new` is total, so an accepted case must construct
/// and a rejected case is verified against the owning slot constructor.
fn execute_inventory_state(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "inventory-state";
    let selected = required_u8(case, input, "selected")?;
    let hotbar = parse_stack_array(case, input, "hotbar", HotbarSlot::COUNT as usize)?;
    let backpack = parse_stack_array(case, input, "backpack", BACKPACK_SLOTS)?;

    match inventory_rejection(selected, &hotbar, &backpack) {
        Some(rejection) => {
            verify_inventory_rejection(case, subject, selected, &hotbar, &backpack, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert("selected".to_string(), Value::from(selected));
            fields.insert("hotbar".to_string(), raw_stack_list(&hotbar));
            fields.insert("backpack".to_string(), raw_stack_list(&backpack));
            normalized_error(case, rejection.category, &rejection.rule, fields)
        }
        None => {
            let selected = HotbarSlot::new(selected)
                .map_err(|error| classification_conflict(case, subject, &format!("{error:?}")))?;
            let hotbar_slots = resolved_stacks(case, subject, &hotbar)?;
            let backpack_slots = resolved_stacks(case, subject, &backpack)?;
            let mut hotbar_array = [ItemStack::EMPTY; HotbarSlot::COUNT as usize];
            hotbar_array.copy_from_slice(&hotbar_slots);
            let mut backpack_array = [ItemStack::EMPTY; BACKPACK_SLOTS];
            backpack_array.copy_from_slice(&backpack_slots);
            let state = InventoryState::new(InventoryStateParts {
                selected,
                hotbar: hotbar_array,
                backpack: backpack_array,
            });
            let mut fields = JsonMap::new();
            fields.insert("selected".to_string(), Value::from(state.selected().get()));
            fields.insert("hotbar".to_string(), stack_list(state.hotbar()));
            fields.insert("backpack".to_string(), stack_list(state.backpack()));
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// Selects the first rule one inventory publication breaks, in the same order
/// the Go validator checks them.
fn inventory_rejection(
    selected: u8,
    hotbar: &[ParsedStack],
    backpack: &[ParsedStack],
) -> Option<Rejection> {
    if selected >= HotbarSlot::COUNT {
        return Some(Rejection {
            category: "invalid-value",
            rule: "inventory_state.selected_range".to_string(),
            error: Some(DomainError::InvalidHotbarSlot),
        });
    }
    for (index, parsed) in hotbar.iter().enumerate() {
        if let Some((category, error)) = stack_rejection(parsed) {
            return Some(Rejection {
                category,
                rule: format!("inventory_state.hotbar_slot_{index}"),
                error: Some(error),
            });
        }
    }
    for (index, parsed) in backpack.iter().enumerate() {
        if let Some((category, error)) = stack_rejection(parsed) {
            return Some(Rejection {
                category,
                rule: format!("inventory_state.backpack_slot_{index}"),
                error: Some(error),
            });
        }
    }
    None
}

/// Verifies the constructor agreement for one classified inventory
/// rejection: the selected-index rule is owned by `HotbarSlot`, each slot rule
/// by `ItemStack`.
fn verify_inventory_rejection(
    case: &FrozenCase,
    subject: &str,
    selected: u8,
    hotbar: &[ParsedStack],
    backpack: &[ParsedStack],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed = match rejection.rule.as_str() {
        "inventory_state.selected_range" => HotbarSlot::new(selected).map(|_| ()),
        rule => {
            let (array, index_text) =
                if let Some(rest) = rule.strip_prefix("inventory_state.hotbar_slot_") {
                    (hotbar, rest)
                } else if let Some(rest) = rule.strip_prefix("inventory_state.backpack_slot_") {
                    (backpack, rest)
                } else {
                    return Err(classification_conflict(case, subject, rule));
                };
            let index: usize = index_text
                .parse()
                .map_err(|_| classification_conflict(case, subject, rule))?;
            match array.get(index) {
                Some(parsed) => parsed.resolved.map(|_| ()),
                None => return Err(classification_conflict(case, subject, rule)),
            }
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Executes one `crafting-state` case, whose payload is the grid size, the
/// nine grid slots and the derived output.
///
/// The Go validator checks the size, then walks the slots in grid order, and
/// inside that walk it checks the personal residue per slot before moving on,
/// then the output. The classifier replays that walk exactly, so the first
/// nonempty personal-residue cell names the rule even when a later slot would
/// also break.
fn execute_crafting_state(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "crafting-state";
    let size = required_u8(case, input, "size")?;
    let slots = parse_stack_array(case, input, "slots", CRAFTING_GRID_SLOTS)?;
    let output = stack_from_object(case, "output", required_object(case, input, "output")?)?;

    match crafting_rejection(size, &slots, &output) {
        Some(rejection) => {
            verify_crafting_rejection(case, subject, size, &slots, &output, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert(
                "size".to_string(),
                Value::String(raw_kind_crafting_text(size).to_string()),
            );
            fields.insert("slots".to_string(), raw_stack_list(&slots));
            fields.insert(
                "output".to_string(),
                Value::Object(raw_stack_fields(&output.raw)),
            );
            normalized_error(case, rejection.category, &rejection.rule, fields)
        }
        None => {
            let semantic_size = crafting_size(size)
                .ok_or_else(|| classification_conflict(case, subject, "crafting_state.size"))?;
            let slot_stacks = resolved_stacks(case, subject, &slots)?;
            let output_stack = resolved_stack(case, subject, &output)?;
            let mut slot_array = [ItemStack::EMPTY; CRAFTING_GRID_SLOTS];
            slot_array.copy_from_slice(&slot_stacks);
            let state = CraftingState::try_new(CraftingStateParts {
                size: semantic_size,
                slots: slot_array,
                output: output_stack,
            })
            .map_err(|error| classification_conflict(case, subject, &format!("{error:?}")))?;
            let mut fields = JsonMap::new();
            fields.insert(
                "size".to_string(),
                Value::String(
                    match state.size() {
                        CraftingSize::Personal => "personal",
                        CraftingSize::Workbench => "workbench",
                    }
                    .to_string(),
                ),
            );
            fields.insert("slots".to_string(), stack_list(state.slots()));
            fields.insert(
                "output".to_string(),
                Value::Object(stack_fields(state.output())),
            );
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// The published grid a raw side length names.
fn crafting_size(size: u8) -> Option<CraftingSize> {
    match size {
        CRAFTING_PERSONAL => Some(CraftingSize::Personal),
        CRAFTING_WORKBENCH => Some(CraftingSize::Workbench),
        _ => None,
    }
}

/// The normalized text a raw crafting size publishes.
fn raw_kind_crafting_text(size: u8) -> &'static str {
    match size {
        CRAFTING_PERSONAL => "personal",
        CRAFTING_WORKBENCH => "workbench",
        _ => "unknown",
    }
}

/// Selects the first rule one crafting publication breaks, in the same order
/// the Go validator checks them. The personal residue applies only to the
/// personal grid's five unused cells, checked inside the slot walk.
fn crafting_rejection(size: u8, slots: &[ParsedStack], output: &ParsedStack) -> Option<Rejection> {
    if crafting_size(size).is_none() {
        // A raw size outside the two published lengths has no Rust
        // representation, so only the classifier decides.
        return Some(Rejection {
            category: "invalid-enum",
            rule: "crafting_state.size".to_string(),
            error: None,
        });
    }
    for (index, parsed) in slots.iter().enumerate() {
        if let Some((category, error)) = stack_rejection(parsed) {
            return Some(Rejection {
                category,
                rule: format!("crafting_state.slot_{index}"),
                error: Some(error),
            });
        }
        if size == CRAFTING_PERSONAL && index >= PERSONAL_GRID_USABLE && !parsed.raw.is_empty() {
            return Some(Rejection {
                category: "invalid-value",
                rule: "crafting_state.personal_residue".to_string(),
                error: Some(DomainError::InvalidCraftingResidue),
            });
        }
    }
    if let Some((category, error)) = stack_rejection(output) {
        return Some(Rejection {
            category,
            rule: "crafting_state.output".to_string(),
            error: Some(error),
        });
    }
    None
}

/// Verifies the constructor agreement for one classified crafting rejection.
fn verify_crafting_rejection(
    case: &FrozenCase,
    subject: &str,
    size: u8,
    slots: &[ParsedStack],
    output: &ParsedStack,
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed = match rejection.rule.as_str() {
        "crafting_state.personal_residue" => {
            let semantic_size = crafting_size(size)
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let slot_stacks = resolved_stacks(case, subject, slots)?;
            let output_stack = resolved_stack(case, subject, output)?;
            let mut slot_array = [ItemStack::EMPTY; CRAFTING_GRID_SLOTS];
            slot_array.copy_from_slice(&slot_stacks);
            CraftingState::try_new(CraftingStateParts {
                size: semantic_size,
                slots: slot_array,
                output: output_stack,
            })
            .map(|_| ())
        }
        "crafting_state.output" => output.resolved.map(|_| ()),
        rule => {
            let index: usize = rule
                .strip_prefix("crafting_state.slot_")
                .and_then(|index| index.parse().ok())
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            match slots.get(index) {
                Some(parsed) => parsed.resolved.map(|_| ()),
                None => return Err(classification_conflict(case, subject, rule)),
            }
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Executes one `furnace-state` case, whose payload is the furnace reference,
/// the three slots and the two timers.
///
/// The slot whitelist itself is never reimplemented here: the classifier only
/// decides which slot the case breaks by consulting the same shared smelting
/// tables the constructor reads, and `FurnaceState::try_new` remains the
/// admission authority whose verdict is cross-checked.
fn execute_furnace_state(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "furnace-state";
    let raw = RawFurnace {
        container: parse_container(case, input)?,
        input: parse_optional_stack(case, input, "input")?,
        fuel: parse_optional_stack(case, input, "fuel")?,
        output: parse_optional_stack(case, input, "output")?,
        progress_ticks: required_u8(case, input, "progress_ticks")?,
        burn_ticks: required_u16(case, input, "burn_ticks")?,
    };

    match furnace_rejection(&raw) {
        Some(rejection) => {
            verify_furnace_rejection(case, subject, &raw, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert(
                "container".to_string(),
                Value::Object(raw_container_fields(&raw.container)),
            );
            fields.insert(
                "input".to_string(),
                Value::Object(raw_stack_fields(&raw.input.raw)),
            );
            fields.insert(
                "fuel".to_string(),
                Value::Object(raw_stack_fields(&raw.fuel.raw)),
            );
            fields.insert(
                "output".to_string(),
                Value::Object(raw_stack_fields(&raw.output.raw)),
            );
            fields.insert(
                "progress_ticks".to_string(),
                Value::from(raw.progress_ticks),
            );
            fields.insert("burn_ticks".to_string(), Value::from(raw.burn_ticks));
            normalized_error(case, rejection.category, &rejection.rule, fields)
        }
        None => {
            let state = construct_furnace_state(case, subject, &raw)?;
            let mut fields = JsonMap::new();
            fields.insert(
                "container".to_string(),
                Value::Object(container_fields(state.container())),
            );
            fields.insert(
                "input".to_string(),
                Value::Object(stack_fields(state.input())),
            );
            fields.insert(
                "fuel".to_string(),
                Value::Object(stack_fields(state.fuel())),
            );
            fields.insert(
                "output".to_string(),
                Value::Object(stack_fields(state.output())),
            );
            fields.insert(
                "progress_ticks".to_string(),
                Value::from(state.progress_ticks()),
            );
            fields.insert("burn_ticks".to_string(), Value::from(state.burn_ticks()));
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// Parses one optional slot triple, defaulting an absent field to the empty
/// stack the furnace publication carries.
fn parse_optional_stack(
    case: &FrozenCase,
    input: &JsonMap,
    key: &str,
) -> Result<ParsedStack, DispatchError> {
    match optional_object(case, input, key)? {
        Some(object) => stack_from_object(case, key, object),
        None => Ok(ParsedStack {
            raw: RawStack {
                item: ITEM_NONE,
                count: 0,
                durability: 0,
            },
            resolved: Ok(ItemStack::EMPTY),
        }),
    }
}

/// The parsed raw shape of one furnace publication.
struct RawFurnace {
    container: RawContainer,
    input: ParsedStack,
    fuel: ParsedStack,
    output: ParsedStack,
    progress_ticks: u8,
    burn_ticks: u16,
}

/// Selects the first rule one furnace publication breaks, in the same order
/// the Go validator checks them: the reference kind, slot and generation, the
/// two timers, and then the input, fuel and output slot rules.
fn furnace_rejection(raw: &RawFurnace) -> Option<Rejection> {
    match raw.container.known_kind() {
        Some(ContainerKind::Chest) => {
            return Some(Rejection {
                category: "invalid-enum",
                rule: "furnace_state.ref_kind".to_string(),
                error: Some(DomainError::InvalidContainerKind),
            });
        }
        // A kind byte neither container array names cannot enter any Rust
        // value, so only the classifier decides.
        None => {
            return Some(Rejection {
                category: "invalid-enum",
                rule: "furnace_state.ref_kind".to_string(),
                error: None,
            });
        }
        Some(ContainerKind::Furnace) => {}
    }
    if raw.container.slot >= FURNACES_PER_CHUNK {
        return Some(Rejection {
            category: "invalid-value",
            rule: "furnace_state.ref_slot".to_string(),
            error: Some(DomainError::InvalidContainerSlot),
        });
    }
    if raw.container.generation == 0 {
        return Some(Rejection {
            category: "invalid-value",
            rule: "furnace_state.ref_generation".to_string(),
            error: Some(DomainError::InvalidContainerGeneration),
        });
    }
    if raw.progress_ticks >= FURNACE_SMELT_TICKS {
        return Some(Rejection {
            category: "invalid-value",
            rule: "furnace_state.progress_range".to_string(),
            error: Some(DomainError::InvalidFurnaceTimers),
        });
    }
    if raw.burn_ticks > FURNACE_BURN_TICKS {
        return Some(Rejection {
            category: "invalid-value",
            rule: "furnace_state.burn_range".to_string(),
            error: Some(DomainError::InvalidFurnaceTimers),
        });
    }
    if let Some(rejection) = furnace_slot_rejection(
        &raw.input,
        "input",
        raw.input.raw.item == ITEM_NONE || smelting_output(raw.input.raw.item).is_some(),
    ) {
        return Some(rejection);
    }
    if let Some(rejection) = furnace_slot_rejection(
        &raw.fuel,
        "fuel",
        raw.fuel.raw.item == ITEM_NONE || raw.fuel.raw.item == FURNACE_FUEL_COAL,
    ) {
        return Some(rejection);
    }
    if let Some(rejection) = furnace_slot_rejection(
        &raw.output,
        "output",
        raw.output.raw.item == ITEM_NONE || is_smelting_product(raw.output.raw.item),
    ) {
        return Some(rejection);
    }
    None
}

/// Selects the rule one furnace slot breaks: a nested stack rule when the
/// triple itself is invalid, otherwise the whitelist rule when the recorded
/// item is one the slot cannot contain.
fn furnace_slot_rejection(
    parsed: &ParsedStack,
    slot: &str,
    whitelisted: bool,
) -> Option<Rejection> {
    if let Some((category, error)) = stack_rejection(parsed) {
        return Some(Rejection {
            category,
            rule: format!("furnace_state.{slot}"),
            error: Some(error),
        });
    }
    if whitelisted {
        return None;
    }
    Some(Rejection {
        category: "invalid-value",
        rule: format!("furnace_state.{slot}"),
        error: Some(DomainError::InvalidFurnaceSlot),
    })
}

/// Verifies the constructor agreement for one classified furnace rejection.
/// The kind rule is owned by `FurnaceState`, the reference slot and
/// generation rules by `ContainerRef`, the timer and whitelist rules by
/// `FurnaceState`, and a nested stack rule by `ItemStack`.
fn verify_furnace_rejection(
    case: &FrozenCase,
    subject: &str,
    raw: &RawFurnace,
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "furnace_state.ref_kind" => {
            // The recorded kind is a published one whose record constructor
            // rejects; the classifier branch for an unknown byte never has an
            // error to check.
            let kind = raw
                .container
                .known_kind()
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let reference = construct_container_ref(case, subject, &raw.container, kind)?;
            FurnaceState::try_new(FurnaceStateParts {
                container: reference,
                input: ItemStack::EMPTY,
                fuel: ItemStack::EMPTY,
                output: ItemStack::EMPTY,
                progress_ticks: 0,
                burn_ticks: 0,
            })
            .map(|_| ())
        }
        "furnace_state.ref_slot" | "furnace_state.ref_generation" => {
            let kind = raw
                .container
                .known_kind()
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            ContainerRef::try_new(
                ChunkPos::new(raw.container.chunk[0], raw.container.chunk[1]),
                kind,
                raw.container.slot,
                raw.container.generation,
            )
            .map(|_| ())
        }
        "furnace_state.progress_range" | "furnace_state.burn_range" => {
            FurnaceState::try_new(furnace_parts_checked(case, subject, raw)?).map(|_| ())
        }
        "furnace_state.input" | "furnace_state.fuel" | "furnace_state.output" => {
            let parsed = match rejection.rule.as_str() {
                "furnace_state.input" => &raw.input,
                "furnace_state.fuel" => &raw.fuel,
                _ => &raw.output,
            };
            if let Some((_, error)) = stack_rejection(parsed) {
                if error == expected {
                    return Ok(());
                }
                return Err(classification_conflict(case, subject, &rejection.rule));
            }
            FurnaceState::try_new(furnace_parts_checked(case, subject, raw)?).map(|_| ())
        }
        _ => return Err(classification_conflict(case, subject, &rejection.rule)),
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Builds the furnace parts every timer and whitelist verification shares,
/// with every already-classified part resolved through its own constructor.
fn furnace_parts_checked(
    case: &FrozenCase,
    subject: &str,
    raw: &RawFurnace,
) -> Result<FurnaceStateParts, DispatchError> {
    let kind = raw
        .container
        .known_kind()
        .ok_or_else(|| classification_conflict(case, subject, ""))?;
    Ok(FurnaceStateParts {
        container: construct_container_ref(case, subject, &raw.container, kind)?,
        input: resolved_stack(case, subject, &raw.input)?,
        fuel: resolved_stack(case, subject, &raw.fuel)?,
        output: resolved_stack(case, subject, &raw.output)?,
        progress_ticks: raw.progress_ticks,
        burn_ticks: raw.burn_ticks,
    })
}

/// Constructs the whole checked furnace chain for one admitted publication.
fn construct_furnace_state(
    case: &FrozenCase,
    subject: &str,
    raw: &RawFurnace,
) -> Result<FurnaceState, DispatchError> {
    FurnaceState::try_new(furnace_parts_checked(case, subject, raw)?)
        .map_err(|error| classification_conflict(case, subject, &format!("{error:?}")))
}

/// Executes one `chest-state` case, whose payload is the chest reference and
/// the twenty-seven chest slots.
fn execute_chest_state(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "chest-state";
    let container = parse_container(case, input)?;
    let items = parse_stack_array(case, input, "items", CHEST_SLOTS)?;

    match chest_rejection(&container, &items) {
        Some(rejection) => {
            verify_chest_rejection(case, subject, &container, &items, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert(
                "container".to_string(),
                Value::Object(raw_container_fields(&container)),
            );
            fields.insert("items".to_string(), raw_stack_list(&items));
            normalized_error(case, rejection.category, &rejection.rule, fields)
        }
        None => {
            let kind = container
                .known_kind()
                .ok_or_else(|| classification_conflict(case, subject, "chest_state.ref_kind"))?;
            let reference = construct_container_ref(case, subject, &container, kind)?;
            let item_stacks = resolved_stacks(case, subject, &items)?;
            let mut item_array = [ItemStack::EMPTY; CHEST_SLOTS];
            item_array.copy_from_slice(&item_stacks);
            let state = ChestState::try_new(ChestStateParts {
                container: reference,
                items: item_array,
            })
            .map_err(|error| classification_conflict(case, subject, &format!("{error:?}")))?;
            let mut fields = JsonMap::new();
            fields.insert(
                "container".to_string(),
                Value::Object(container_fields(state.container())),
            );
            fields.insert("items".to_string(), stack_list(state.items()));
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// Selects the first rule one chest publication breaks, in the same order the
/// Go validator checks them: the reference kind, slot and generation, then
/// every chest slot.
fn chest_rejection(container: &RawContainer, items: &[ParsedStack]) -> Option<Rejection> {
    match container.known_kind() {
        Some(ContainerKind::Furnace) => {
            return Some(Rejection {
                category: "invalid-enum",
                rule: "chest_state.ref_kind".to_string(),
                error: Some(DomainError::InvalidContainerKind),
            });
        }
        // A kind byte neither container array names has no Rust
        // representation, so only the classifier decides.
        None => {
            return Some(Rejection {
                category: "invalid-enum",
                rule: "chest_state.ref_kind".to_string(),
                error: None,
            });
        }
        Some(ContainerKind::Chest) => {}
    }
    if container.slot >= CHESTS_PER_CHUNK {
        return Some(Rejection {
            category: "invalid-value",
            rule: "chest_state.ref_slot".to_string(),
            error: Some(DomainError::InvalidContainerSlot),
        });
    }
    if container.generation == 0 {
        return Some(Rejection {
            category: "invalid-value",
            rule: "chest_state.ref_generation".to_string(),
            error: Some(DomainError::InvalidContainerGeneration),
        });
    }
    for (index, parsed) in items.iter().enumerate() {
        if let Some((category, error)) = stack_rejection(parsed) {
            return Some(Rejection {
                category,
                rule: format!("chest_state.item_{index}"),
                error: Some(error),
            });
        }
    }
    None
}

/// Verifies the constructor agreement for one classified chest rejection.
fn verify_chest_rejection(
    case: &FrozenCase,
    subject: &str,
    container: &RawContainer,
    items: &[ParsedStack],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "chest_state.ref_kind" => {
            let kind = container
                .known_kind()
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let reference = construct_container_ref(case, subject, container, kind)?;
            ChestState::try_new(ChestStateParts {
                container: reference,
                items: [ItemStack::EMPTY; CHEST_SLOTS],
            })
            .map(|_| ())
        }
        "chest_state.ref_slot" | "chest_state.ref_generation" => {
            let kind = container
                .known_kind()
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            ContainerRef::try_new(
                ChunkPos::new(container.chunk[0], container.chunk[1]),
                kind,
                container.slot,
                container.generation,
            )
            .map(|_| ())
        }
        rule => {
            let index: usize = rule
                .strip_prefix("chest_state.item_")
                .and_then(|index| index.parse().ok())
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            match items.get(index) {
                Some(parsed) => parsed.resolved.map(|_| ()),
                None => return Err(classification_conflict(case, subject, rule)),
            }
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Executes one `container-closed` case, whose entire payload is the container
/// reference. Both published kinds are accepted and `ContainerClosed::new` is
/// total, so only the reference rules can reject.
fn execute_container_closed(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "container-closed";
    let container = parse_container(case, input)?;

    match container_closed_rejection(&container) {
        Some(rejection) => {
            verify_container_closed_rejection(case, subject, &container, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert(
                "container".to_string(),
                Value::Object(raw_container_fields(&container)),
            );
            normalized_error(case, rejection.category, &rejection.rule, fields)
        }
        None => {
            let kind = container.known_kind().ok_or_else(|| {
                classification_conflict(case, subject, "container_closed.ref_kind")
            })?;
            let reference = construct_container_ref(case, subject, &container, kind)?;
            let closed = ContainerClosed::new(reference);
            let mut fields = JsonMap::new();
            fields.insert(
                "container".to_string(),
                Value::Object(container_fields(closed.container())),
            );
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// Selects the first rule one closure notification breaks. The record is
/// container-neutral, so both published kinds pass the kind rule and the slot
/// bound is the addressed kind's own per-chunk array size.
fn container_closed_rejection(container: &RawContainer) -> Option<Rejection> {
    // A kind byte neither container array names has no Rust representation,
    // so only the classifier decides.
    if container.known_kind().is_none() {
        return Some(Rejection {
            category: "invalid-enum",
            rule: "container_closed.ref_kind".to_string(),
            error: None,
        });
    }
    let bound = match container.kind {
        0 => FURNACES_PER_CHUNK,
        _ => CHESTS_PER_CHUNK,
    };
    if container.slot >= bound {
        return Some(Rejection {
            category: "invalid-value",
            rule: "container_closed.ref_slot".to_string(),
            error: Some(DomainError::InvalidContainerSlot),
        });
    }
    if container.generation == 0 {
        return Some(Rejection {
            category: "invalid-value",
            rule: "container_closed.ref_generation".to_string(),
            error: Some(DomainError::InvalidContainerGeneration),
        });
    }
    None
}

/// Verifies the constructor agreement for one classified closure rejection.
/// The reference constructor owns both the slot and the generation rule.
fn verify_container_closed_rejection(
    case: &FrozenCase,
    subject: &str,
    container: &RawContainer,
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let kind = container
        .known_kind()
        .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
    let constructed: Result<(), DomainError> = ContainerRef::try_new(
        ChunkPos::new(container.chunk[0], container.chunk[1]),
        kind,
        container.slot,
        container.generation,
    )
    .map(|_| ());
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

#[test]
fn event_inventory_execute_33_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}

/// Proves the shared comparator actually compares semantic fields: the
/// produced accepted result matches its frozen expectation, while the same
/// actual value no longer matches once one normalized field is mutated.
#[test]
fn event_inventory_comparator_detects_semantic_mutation() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    let accepted = executed
        .into_iter()
        .find(|case| {
            case.actual.get("kind").and_then(|kind| kind.as_str()) == Some("ok")
                && case
                    .actual
                    .get("category")
                    .and_then(|category| category.as_str())
                    == Some("inventory-state")
        })
        .expect("one accepted inventory-state case");

    assert_domain_normalized(&accepted);

    let mut mutated_case = accepted.case;
    let fields = mutated_case
        .normalized
        .get_mut("fields")
        .and_then(|fields| fields.as_object_mut())
        .expect("normalized fields object");
    fields.insert("selected".to_string(), Value::from(999));

    let mutated = ExecutedCase {
        case: mutated_case,
        actual: accepted.actual,
    };
    let outcome = std::panic::catch_unwind(|| assert_domain_normalized(&mutated));
    assert!(
        outcome.is_err(),
        "a semantic mutation must fail the normalized comparison"
    );
}
