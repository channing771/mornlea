//! Topic module for domain.values corpus cases.
//!
//! The four rules here are the Go producer's item, slot, drop and container
//! admissions. Each executor parses the case input, selects the Go rejection
//! rule from the raw value in the producer's own precedence, and then proves
//! the matching public Rust constructor agrees: a classified acceptance must
//! construct, and a classified rejection must return the mapped `DomainError`.
//! Accepted fields are read back from constructed Rust values or from the
//! public item tables, never from the parsed input or the frozen expectation.
//!
//! Two of the tables the classifier reads are not exported by the domain crate
//! (the per-chunk drop and container array sizes). They are mirrored here as
//! local constants, and the constructor agreement check fails closed if a
//! mirror drifts, so a stale copy cannot silently publish a wrong rule.

use super::support::{
    DispatchError, ExecutedCase, JsonMap, assert_domain_normalized, execute_topic, input_object,
    invalid_case, normalized_error, normalized_ok, optional_string, optional_u8, optional_u16,
    optional_u32, required_i32, required_string, required_u8, required_u16,
};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};
use mornlea_domain::{
    ChunkPos, ContainerKind, ContainerRef, DomainError, DropId, ItemStack, durability_max,
    is_smelting_product, item_stack_limit, smelting_output,
};
use serde_json::Value;

pub const EXPECTED_COUNT: usize = 99;

const RULES: &[&str] = &["item-table", "item-stack", "drop-id", "container-ref"];

/// The absent item number, from the Go `core.ItemNone` zero value.
///
/// A stack on this number is rejected per field rather than as one unknown
/// item, which is why the classifier names it separately.
const ABSENT_ITEM: u16 = 0;

/// Authoritative drop slots one chunk holds, from the Go
/// `core.DropsPerChunk`; the domain crate keeps the same bound private.
const DROPS_PER_CHUNK: u8 = 32;

/// Fixed furnace array size of one chunk, from the Go
/// `core.FurnacesPerChunk`; the domain crate keeps the same bound private.
const FURNACES_PER_CHUNK: u8 = 32;

/// Fixed chest array size of one chunk, from the Go `core.ChestsPerChunk`; the
/// domain crate keeps the same bound private.
const CHESTS_PER_CHUNK: u8 = 16;

pub fn owns(case: &FrozenCase) -> bool {
    if case.consumer != CorpusConsumer::Domain || case.family != "domain.values" {
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
        "item-table" => execute_item_table(case, input),
        "item-stack" => execute_item_stack(case, input),
        "drop-id" => execute_drop_id(case, input),
        "container-ref" => execute_container_ref(case, input),
        unknown => Err(invalid_case(
            case,
            format!("unknown value rule '{unknown}'"),
        )),
    }
}

/// Executes one `item-table` case, which has no normalized rejection.
///
/// The four answers come from the public item tables, so a row is evidence for
/// the same tables the authority reads rather than for a second copy.
fn execute_item_table(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let item = required_u16(case, input, "item")?;

    let limit = item_stack_limit(item);
    let durable = durability_max(item);
    let output = smelting_output(item);

    let mut fields = JsonMap::new();
    fields.insert("item".to_string(), Value::from(item));
    fields.insert("registered".to_string(), Value::Bool(limit.is_some()));
    fields.insert(
        "smelting_product".to_string(),
        Value::Bool(is_smelting_product(item)),
    );
    // The optional fields are present exactly when the Go producer found an
    // answer, so a durable number and a smelting input are pinned by the same
    // row shape as an ordinary registered number.
    if let Some(limit) = limit {
        fields.insert("stack_limit".to_string(), Value::from(limit));
    }
    if let Some(max) = durable {
        fields.insert("durability_max".to_string(), Value::from(max));
    }
    if let Some(output) = output {
        fields.insert("smelting_output".to_string(), Value::from(output));
    }
    Ok(normalized_ok("item-table", fields))
}

/// Executes one `item-stack` case.
///
/// The absent `count` and `durability` both default to zero before the u8 and
/// u16 width checks, and the accepted slot value is read back from the
/// constructed `ItemStack`, so an accepted row can only carry a value the
/// domain admits.
fn execute_item_stack(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let item = required_u16(case, input, "item")?;
    let count = optional_u8(case, input, "count")?.unwrap_or(0);
    let durability = optional_u16(case, input, "durability")?.unwrap_or(0);

    let classified = item_stack_rule(item, count, durability);
    let constructed = ItemStack::try_new(item, count, durability);

    match classified {
        Some(expected) => match constructed {
            Err(error) if error == expected.error => normalized_error(
                case,
                expected.category,
                expected.rule,
                stack_projection(item, count, durability),
            ),
            _ => Err(classification_conflict(case, "item-stack", expected.rule)),
        },
        None => match constructed {
            Ok(stack) => {
                let mut fields = JsonMap::new();
                fields.insert("item".to_string(), Value::from(stack.item()));
                fields.insert("count".to_string(), Value::from(stack.count()));
                fields.insert("durability".to_string(), Value::from(stack.durability()));
                Ok(normalized_ok("item-stack", fields))
            }
            Err(_) => Err(classification_conflict(case, "item-stack", "")),
        },
    }
}

/// Executes one `drop-id` case.
///
/// The dimension is deliberately unchecked by the classifier because the Go
/// rule checks only the slot range and the generation, so an unusual dimension
/// has to stay a publishable identity.
fn execute_drop_id(case: &FrozenCase, input: &JsonMap) -> Result<serde_json::Value, DispatchError> {
    let dimension = required_i32(case, input, "dimension")?;
    let chunk_x = required_i32(case, input, "chunk_x")?;
    let chunk_z = required_i32(case, input, "chunk_z")?;
    let slot = required_u8(case, input, "slot")?;
    let generation = optional_u32(case, input, "generation")?.unwrap_or(0);

    let chunk = ChunkPos::new(chunk_x, chunk_z);
    let classified = drop_id_rule(slot, generation);
    let constructed = DropId::try_new(dimension, chunk, slot, generation);

    match classified {
        Some(expected) => match constructed {
            Err(error) if error == expected.error => normalized_error(
                case,
                "invalid-value",
                expected.rule,
                drop_projection(dimension, chunk_x, chunk_z, slot, generation),
            ),
            _ => Err(classification_conflict(case, "drop-id", expected.rule)),
        },
        None => match constructed {
            Ok(drop) => {
                let mut fields = JsonMap::new();
                fields.insert("dimension".to_string(), Value::from(drop.dimension()));
                fields.insert("chunk_x".to_string(), Value::from(drop.chunk().x()));
                fields.insert("chunk_z".to_string(), Value::from(drop.chunk().z()));
                fields.insert("slot".to_string(), Value::from(drop.slot()));
                fields.insert("generation".to_string(), Value::from(drop.generation()));
                Ok(normalized_ok("drop-id", fields))
            }
            Err(_) => Err(classification_conflict(case, "drop-id", "")),
        },
    }
}

/// Executes one `container-ref` case.
///
/// The reference is inherently overworld, so the case's `dimension` field is
/// ignored and the accepted projection reports the constant zero the Go
/// producer records. An unknown kind name is a hard schema failure rather than
/// a normalized rejection, because the Go classifier refuses it before it can
/// name a rule.
fn execute_container_ref(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let chunk_x = required_i32(case, input, "chunk_x")?;
    let chunk_z = required_i32(case, input, "chunk_z")?;
    let slot = required_u8(case, input, "slot")?;
    let generation = optional_u32(case, input, "generation")?.unwrap_or(0);
    let kind = container_kind(case, optional_string(case, input, "kind")?)?;

    let chunk = ChunkPos::new(chunk_x, chunk_z);
    let classified = container_ref_rule(kind, slot, generation);
    let constructed = ContainerRef::try_new(chunk, kind, slot, generation);

    match classified {
        Some(expected) => match constructed {
            Err(error) if error == expected.error => normalized_error(
                case,
                "invalid-value",
                expected.rule,
                container_ref_projection(kind, chunk_x, chunk_z, slot, generation),
            ),
            _ => Err(classification_conflict(
                case,
                "container-ref",
                expected.rule,
            )),
        },
        None => match constructed {
            Ok(reference) => {
                let mut fields = JsonMap::new();
                fields.insert(
                    "kind".to_string(),
                    Value::String(container_kind_name(reference.kind()).to_string()),
                );
                fields.insert("chunk_x".to_string(), Value::from(reference.chunk().x()));
                fields.insert("chunk_z".to_string(), Value::from(reference.chunk().z()));
                fields.insert("slot".to_string(), Value::from(reference.slot()));
                // The accepted reference carries no dimension of its own; the
                // frozen projection records the overworld constant.
                fields.insert("dimension".to_string(), Value::from(0));
                fields.insert(
                    "generation".to_string(),
                    Value::from(reference.generation()),
                );
                Ok(normalized_ok("container-ref", fields))
            }
            Err(_) => Err(classification_conflict(case, "container-ref", "")),
        },
    }
}

/// One classified rejection: its normalized category, its exact Go rule name,
/// and the `DomainError` the matching constructor must return.
struct Rejection {
    category: &'static str,
    rule: &'static str,
    error: DomainError,
}

/// Selects the Go item-slot rejection rule from the raw slot value.
///
/// The order is the precedence `core.ItemStack.Valid` and the Go classifier
/// share: the absent item's count before its durability, the unregistered
/// item, the count range, the durable durability range, and finally the
/// nonzero durability on a nondurable item. A `None` result means the value is
/// admitted.
fn item_stack_rule(item: u16, count: u8, durability: u16) -> Option<Rejection> {
    let limit = match item_stack_limit(item) {
        Some(limit) => limit,
        None if item == ABSENT_ITEM => {
            if count != 0 {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "item_stack.absent_item_count",
                    error: DomainError::InvalidCount,
                });
            }
            if durability != 0 {
                return Some(Rejection {
                    category: "invalid-value",
                    rule: "item_stack.absent_item_durability",
                    error: DomainError::InvalidDurability,
                });
            }
            return None;
        }
        None => {
            return Some(Rejection {
                category: "invalid-enum",
                rule: "item_stack.registered_item",
                error: DomainError::InvalidItem,
            });
        }
    };
    if count == 0 || count > limit {
        return Some(Rejection {
            category: "invalid-value",
            rule: "item_stack.count_range",
            error: DomainError::InvalidCount,
        });
    }
    match durability_max(item) {
        Some(max) if durability < 1 || durability > max => Some(Rejection {
            category: "invalid-value",
            rule: "item_stack.durability_range",
            error: DomainError::InvalidDurability,
        }),
        Some(_) => None,
        None if durability != 0 => Some(Rejection {
            category: "invalid-value",
            rule: "item_stack.nondurable_durability",
            error: DomainError::InvalidDurability,
        }),
        None => None,
    }
}

/// Selects the Go drop-identity rejection rule from the raw slot and
/// generation. The dimension is deliberately not consulted, matching the Go
/// `core.DropID.Valid` rule.
fn drop_id_rule(slot: u8, generation: u32) -> Option<Rejection> {
    if slot >= DROPS_PER_CHUNK {
        return Some(Rejection {
            category: "invalid-value",
            rule: "drop_id.slot_range",
            error: DomainError::InvalidDropSlot,
        });
    }
    if generation == 0 {
        return Some(Rejection {
            category: "invalid-value",
            rule: "drop_id.generation_nonzero",
            error: DomainError::InvalidDropGeneration,
        });
    }
    None
}

/// Selects the Go container-reference rejection rule from the parsed kind.
///
/// The slot bound is the addressed kind's own per-chunk array size, so the two
/// kinds report different rules for the same numeric slot.
fn container_ref_rule(kind: ContainerKind, slot: u8, generation: u32) -> Option<Rejection> {
    let (slots, rule) = match kind {
        ContainerKind::Furnace => (FURNACES_PER_CHUNK, "container_ref.furnace_slot_range"),
        ContainerKind::Chest => (CHESTS_PER_CHUNK, "container_ref.chest_slot_range"),
    };
    if slot >= slots {
        return Some(Rejection {
            category: "invalid-value",
            rule,
            error: DomainError::InvalidContainerSlot,
        });
    }
    if generation == 0 {
        return Some(Rejection {
            category: "invalid-value",
            rule: "container_ref.generation_nonzero",
            error: DomainError::InvalidContainerGeneration,
        });
    }
    None
}

/// Resolves the container kind one case names.
///
/// The furnace kind is the Go zero value, so an absent, blank or
/// whitespace-and-case-normalized `furnace` name all select it. Any other text
/// is a hard schema failure: the Go classifier rejects it before a normalized
/// rule can be named, so no frozen row exists for an unknown kind.
fn container_kind(case: &FrozenCase, raw: Option<&str>) -> Result<ContainerKind, DispatchError> {
    let normalized = raw.map(|text| text.trim().to_lowercase());
    match normalized.as_deref() {
        None | Some("") | Some("furnace") => Ok(ContainerKind::Furnace),
        Some("chest") => Ok(ContainerKind::Chest),
        Some(_) => Err(invalid_case(
            case,
            format!("unknown container kind '{}'", raw.unwrap_or_default()),
        )),
    }
}

/// Renders one container kind for the recorded fields.
fn container_kind_name(kind: ContainerKind) -> &'static str {
    match kind {
        ContainerKind::Furnace => "furnace",
        ContainerKind::Chest => "chest",
    }
}

/// Builds the rejected `item-stack` projection from the parsed input, because a
/// rejected Rust value has no getters to read.
fn stack_projection(item: u16, count: u8, durability: u16) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("item".to_string(), Value::from(item));
    fields.insert("count".to_string(), Value::from(count));
    fields.insert("durability".to_string(), Value::from(durability));
    fields
}

/// Builds the rejected `drop-id` projection from the parsed input.
fn drop_projection(
    dimension: i32,
    chunk_x: i32,
    chunk_z: i32,
    slot: u8,
    generation: u32,
) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("dimension".to_string(), Value::from(dimension));
    fields.insert("chunk_x".to_string(), Value::from(chunk_x));
    fields.insert("chunk_z".to_string(), Value::from(chunk_z));
    fields.insert("slot".to_string(), Value::from(slot));
    fields.insert("generation".to_string(), Value::from(generation));
    fields
}

/// Builds the rejected `container-ref` projection from the parsed input.
fn container_ref_projection(
    kind: ContainerKind,
    chunk_x: i32,
    chunk_z: i32,
    slot: u8,
    generation: u32,
) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert(
        "kind".to_string(),
        Value::String(container_kind_name(kind).to_string()),
    );
    fields.insert("chunk_x".to_string(), Value::from(chunk_x));
    fields.insert("chunk_z".to_string(), Value::from(chunk_z));
    fields.insert("slot".to_string(), Value::from(slot));
    fields.insert("dimension".to_string(), Value::from(0));
    fields.insert("generation".to_string(), Value::from(generation));
    fields
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

#[test]
fn values_execute_99_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}

/// Proves the shared comparator actually compares semantic fields: the produced
/// accepted result matches its frozen expectation, while the same actual value
/// no longer matches once one normalized field is mutated.
#[test]
fn values_comparator_detects_semantic_mutation() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    let accepted = executed
        .into_iter()
        .find(|case| {
            case.actual.get("kind").and_then(|kind| kind.as_str()) == Some("ok")
                && case
                    .actual
                    .get("category")
                    .and_then(|category| category.as_str())
                    == Some("item-table")
        })
        .expect("one accepted item-table case");

    assert_domain_normalized(&accepted);

    let mut mutated_case = accepted.case;
    let fields = mutated_case
        .normalized
        .get_mut("fields")
        .and_then(|fields| fields.as_object_mut())
        .expect("normalized fields object");
    let item = fields
        .get("item")
        .and_then(|item| item.as_i64())
        .expect("normalized item field is an integer");
    fields.insert("item".to_string(), Value::from(item + 1));

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
