//! Topic module for domain.command_control corpus cases.
//!
//! The nine rules here are the Go producer's movement and ray command
//! admissions. Each executor parses the case input, selects the Go rejection
//! rule from the raw values in the producer's own precedence, and then proves
//! the matching public Rust constructor agrees: a classified acceptance must
//! construct, and a classified rejection must return the mapped `DomainError`.
//! Accepted fields are read back from the constructed Rust values, never from
//! the parsed input or the frozen expectation.
//!
//! Every accepted payload is wrapped in a `CommandEnvelope`. The producer
//! supplies only the sequence, so the tick, session and arrival index are
//! synthesized here; they are intake metadata that never reaches the
//! normalized projection.

use super::support::{
    DispatchError, ExecutedCase, JsonMap, assert_domain_normalized, execute_topic, input_object,
    invalid_case, normalize_f32, normalize_u64, normalized_error, normalized_ok, parse_f32_token,
    required_bool, required_i8, required_i32, required_string, required_u8, required_u64,
};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};
use mornlea_domain::{
    ChunkPos, Command, CommandEnvelope, CommandEnvelopeParts, Dimension, DomainError, HeldActions,
    HotbarSlot, LookAngles, Movement, PlacementIntent, PlayerControl, PlayerControlParts,
    ResyncIntent,
};
use serde_json::Value;

pub const EXPECTED_COUNT: usize = 28;

const RULES: &[&str] = &[
    "player-input",
    "place-block",
    "select-hotbar",
    "chunk-resync",
    "till-soil",
    "bone-meal",
    "collect-water",
    "place-water",
    "open-container",
];

pub fn owns(case: &FrozenCase) -> bool {
    if case.consumer != CorpusConsumer::Domain || case.family != "domain.command_control" {
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
        "player-input" => execute_player_input(case, input),
        "place-block" => execute_place_block(case, input),
        "select-hotbar" => execute_select_hotbar(case, input),
        "chunk-resync" => execute_chunk_resync(case, input),
        "till-soil" => execute_ray(case, input, RayRule::TillSoil),
        "bone-meal" => execute_ray(case, input, RayRule::BoneMeal),
        "collect-water" => execute_ray(case, input, RayRule::CollectWater),
        "place-water" => execute_ray(case, input, RayRule::PlaceWater),
        "open-container" => execute_ray(case, input, RayRule::OpenContainer),
        unknown => Err(invalid_case(
            case,
            format!("unknown control rule '{unknown}'"),
        )),
    }
}

/// Executes one `player-input` case.
///
/// The move axes keep their full `i8` range and the four held flags have no
/// range rule, so the finite rotation is the only rejection this payload
/// publishes. The accepted projection is read from the constructed control.
fn execute_player_input(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let sequence = required_u64(case, input, "sequence")?;
    let move_x = required_i8(case, input, "move_x")?;
    let move_z = required_i8(case, input, "move_z")?;
    let jump = required_bool(case, input, "jump")?;
    let yaw = required_angle(case, input, "yaw")?;
    let pitch = required_angle(case, input, "pitch")?;
    let mining = required_bool(case, input, "mining")?;
    let eating = required_bool(case, input, "eating")?;
    let sprinting = required_bool(case, input, "sprinting")?;
    let sneaking = required_bool(case, input, "sneaking")?;

    let classified = player_input_rule(yaw, pitch);
    let look = LookAngles::try_new(yaw, pitch);

    match classified {
        Some(expected) => match look {
            Err(error) if error == expected.error => normalized_error(
                case,
                expected.category,
                expected.rule,
                player_input_projection(
                    sequence, move_x, move_z, jump, yaw, pitch, mining, eating, sprinting, sneaking,
                ),
            ),
            _ => Err(classification_conflict(case, "player-input", expected.rule)),
        },
        None => match look {
            Ok(look) => {
                let control = PlayerControl::new(PlayerControlParts {
                    movement: Movement {
                        move_x,
                        move_z,
                        jump,
                    },
                    look,
                    actions: HeldActions {
                        primary: mining,
                        eating,
                        sprinting,
                        sneaking,
                    },
                });
                let envelope = wrap(sequence, Command::PlayerInput(control))
                    .map_err(|_| classification_conflict(case, "player-input", ""))?;

                let movement = control.movement();
                let look = control.look();
                let actions = control.actions();
                let mut fields = JsonMap::new();
                fields.insert("sequence".to_string(), normalize_u64(envelope.sequence()));
                fields.insert("move_x".to_string(), Value::from(movement.move_x));
                fields.insert("move_z".to_string(), Value::from(movement.move_z));
                fields.insert("jump".to_string(), Value::Bool(movement.jump));
                fields.insert("yaw".to_string(), normalize_f32(look.yaw()));
                fields.insert("pitch".to_string(), normalize_f32(look.pitch()));
                fields.insert("mining".to_string(), Value::Bool(actions.primary));
                fields.insert("eating".to_string(), Value::Bool(actions.eating));
                fields.insert("sprinting".to_string(), Value::Bool(actions.sprinting));
                fields.insert("sneaking".to_string(), Value::Bool(actions.sneaking));
                Ok(normalized_ok("player-input", fields))
            }
            Err(_) => Err(classification_conflict(case, "player-input", "")),
        },
    }
}

/// Executes one `place-block` case.
///
/// The precedence is the Go DTO's own check order: the rotation before the
/// hotbar slot range, so a multiply-invalid payload reports the rotation.
fn execute_place_block(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let sequence = required_u64(case, input, "sequence")?;
    let yaw = required_angle(case, input, "yaw")?;
    let pitch = required_angle(case, input, "pitch")?;
    let slot = required_u8(case, input, "slot")?;

    let classified = place_block_rule(yaw, pitch, slot);

    match classified {
        Some(expected) => {
            let constructed = LookAngles::try_new(yaw, pitch)
                .and_then(|look| PlacementIntent::try_new(look, slot));
            match constructed {
                Err(error) if error == expected.error => normalized_error(
                    case,
                    expected.category,
                    expected.rule,
                    place_block_projection(sequence, yaw, pitch, slot),
                ),
                _ => Err(classification_conflict(case, "place-block", expected.rule)),
            }
        }
        None => match LookAngles::try_new(yaw, pitch)
            .and_then(|look| PlacementIntent::try_new(look, slot))
        {
            Ok(intent) => {
                let envelope = wrap(sequence, Command::PlaceBlock(intent))
                    .map_err(|_| classification_conflict(case, "place-block", ""))?;
                let look = intent.look();
                let mut fields = JsonMap::new();
                fields.insert("sequence".to_string(), normalize_u64(envelope.sequence()));
                fields.insert("yaw".to_string(), normalize_f32(look.yaw()));
                fields.insert("pitch".to_string(), normalize_f32(look.pitch()));
                fields.insert("slot".to_string(), Value::from(intent.slot().get()));
                Ok(normalized_ok("place-block", fields))
            }
            Err(_) => Err(classification_conflict(case, "place-block", "")),
        },
    }
}

/// Executes one `select-hotbar` case, whose only payload bound is the slot
/// range.
fn execute_select_hotbar(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let sequence = required_u64(case, input, "sequence")?;
    let slot = required_u8(case, input, "slot")?;

    match select_hotbar_rule(slot) {
        Some(expected) => match HotbarSlot::new(slot) {
            Err(error) if error == expected.error => normalized_error(
                case,
                expected.category,
                expected.rule,
                select_hotbar_projection(sequence, slot),
            ),
            _ => Err(classification_conflict(
                case,
                "select-hotbar",
                expected.rule,
            )),
        },
        None => match HotbarSlot::new(slot) {
            Ok(hotbar) => {
                let envelope = wrap(sequence, Command::SelectHotbar(hotbar))
                    .map_err(|_| classification_conflict(case, "select-hotbar", ""))?;
                let mut fields = JsonMap::new();
                fields.insert("sequence".to_string(), normalize_u64(envelope.sequence()));
                fields.insert("slot".to_string(), Value::from(hotbar.get()));
                Ok(normalized_ok("select-hotbar", fields))
            }
            Err(_) => Err(classification_conflict(case, "select-hotbar", "")),
        },
    }
}

/// Executes one `chunk-resync` case.
///
/// The dimension is an enumerated payload field, so an unknown one is an enum
/// rejection rather than a range rejection. A dimension outside the `u8` width
/// the Rust constructor takes has no constructible Rust value, so it is decided
/// by the pre-construction classifier alone; a fitting unknown dimension is
/// additionally proved against `Dimension::new`.
fn execute_chunk_resync(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let sequence = required_u64(case, input, "sequence")?;
    let dimension = required_i32(case, input, "dimension")?;
    let chunk_x = required_i32(case, input, "chunk_x")?;
    let chunk_z = required_i32(case, input, "chunk_z")?;
    let have_revision = required_u64(case, input, "have_revision")?;

    let chunk = ChunkPos::new(chunk_x, chunk_z);
    let classified = chunk_resync_rule(dimension);

    match classified {
        Some(expected) => {
            // Only a dimension the constructor can represent has an
            // independent Rust verdict to check against the classifier.
            if let Ok(id) = u8::try_from(dimension) {
                match ResyncIntent::try_new(id, chunk, have_revision) {
                    Err(error) if error == expected.error => {}
                    _ => {
                        return Err(classification_conflict(case, "chunk-resync", expected.rule));
                    }
                }
            }
            normalized_error(
                case,
                expected.category,
                expected.rule,
                chunk_resync_projection(sequence, dimension, chunk_x, chunk_z, have_revision),
            )
        }
        None => {
            let id = u8::try_from(dimension)
                .map_err(|_| classification_conflict(case, "chunk-resync", ""))?;
            match ResyncIntent::try_new(id, chunk, have_revision) {
                Ok(intent) => {
                    let envelope = wrap(sequence, Command::Resync(intent))
                        .map_err(|_| classification_conflict(case, "chunk-resync", ""))?;
                    let intent_chunk = intent.chunk();
                    let mut fields = JsonMap::new();
                    fields.insert("sequence".to_string(), normalize_u64(envelope.sequence()));
                    fields.insert(
                        "dimension".to_string(),
                        Value::from(intent.dimension().get()),
                    );
                    fields.insert("chunk_x".to_string(), Value::from(intent_chunk.x()));
                    fields.insert("chunk_z".to_string(), Value::from(intent_chunk.z()));
                    fields.insert(
                        "have_revision".to_string(),
                        normalize_u64(intent.have_revision()),
                    );
                    Ok(normalized_ok("chunk-resync", fields))
                }
                Err(_) => Err(classification_conflict(case, "chunk-resync", "")),
            }
        }
    }
}

/// Executes one of the five ray intents, whose entire payload is the sequence
/// and the two look angles.
fn execute_ray(
    case: &FrozenCase,
    input: &JsonMap,
    rule: RayRule,
) -> Result<serde_json::Value, DispatchError> {
    let sequence = required_u64(case, input, "sequence")?;
    let yaw = required_angle(case, input, "yaw")?;
    let pitch = required_angle(case, input, "pitch")?;

    let classified = ray_rule(yaw, pitch, rule.rule_name());
    let look = LookAngles::try_new(yaw, pitch);

    match classified {
        Some(expected) => match look {
            Err(error) if error == expected.error => normalized_error(
                case,
                expected.category,
                expected.rule,
                ray_projection(sequence, yaw, pitch),
            ),
            _ => Err(classification_conflict(
                case,
                rule.category(),
                expected.rule,
            )),
        },
        None => match look {
            Ok(look) => {
                let envelope = wrap(sequence, rule.command(look))
                    .map_err(|_| classification_conflict(case, rule.category(), ""))?;
                let mut fields = JsonMap::new();
                fields.insert("sequence".to_string(), normalize_u64(envelope.sequence()));
                fields.insert("yaw".to_string(), normalize_f32(look.yaw()));
                fields.insert("pitch".to_string(), normalize_f32(look.pitch()));
                Ok(normalized_ok(rule.category(), fields))
            }
            Err(_) => Err(classification_conflict(case, rule.category(), "")),
        },
    }
}

/// The five ray intents, which share one payload shape and one finite-angle
/// rule but differ in their normalized category and their Go rule prefix.
#[derive(Clone, Copy)]
enum RayRule {
    TillSoil,
    BoneMeal,
    CollectWater,
    PlaceWater,
    OpenContainer,
}

impl RayRule {
    fn category(self) -> &'static str {
        match self {
            RayRule::TillSoil => "till-soil",
            RayRule::BoneMeal => "bone-meal",
            RayRule::CollectWater => "collect-water",
            RayRule::PlaceWater => "place-water",
            RayRule::OpenContainer => "open-container",
        }
    }

    /// The Go rule prefix, which uses hyphens for these intents and an
    /// underscore for `place_block`. The spelling is part of the frozen
    /// vocabulary, so the two forms are pinned separately.
    fn rule_name(self) -> &'static str {
        match self {
            RayRule::TillSoil => "till-soil.finite_rotation",
            RayRule::BoneMeal => "bone-meal.finite_rotation",
            RayRule::CollectWater => "collect-water.finite_rotation",
            RayRule::PlaceWater => "place-water.finite_rotation",
            RayRule::OpenContainer => "open-container.finite_rotation",
        }
    }

    fn command(self, look: LookAngles) -> Command {
        match self {
            RayRule::TillSoil => Command::TillSoil(look),
            RayRule::BoneMeal => Command::BoneMeal(look),
            RayRule::CollectWater => Command::CollectWater(look),
            RayRule::PlaceWater => Command::PlaceWater(look),
            RayRule::OpenContainer => Command::OpenContainer(look),
        }
    }
}

/// One classified rejection: its normalized category, its exact Go rule name,
/// and the `DomainError` the matching constructor must return.
struct Rejection {
    category: &'static str,
    rule: &'static str,
    error: DomainError,
}

/// Wraps one control payload in an envelope with the synthesized intake
/// metadata.
///
/// The producer names only the sequence, so the tick, session and arrival
/// index are the fixed zeros this adapter records. None of the three reaches
/// the normalized projection.
fn wrap(sequence: u64, command: Command) -> Result<CommandEnvelope, DomainError> {
    CommandEnvelope::try_new(CommandEnvelopeParts {
        tick: 0,
        session: 0,
        sequence,
        arrival_index: 0,
        command,
    })
}

/// Reads one required angle from its float-token text.
fn required_angle(case: &FrozenCase, input: &JsonMap, key: &str) -> Result<f32, DispatchError> {
    let text = required_string(case, input, key)?;
    parse_f32_token(case, key, text)
}

/// Selects the one rejection a movement payload publishes: its rotation.
fn player_input_rule(yaw: f32, pitch: f32) -> Option<Rejection> {
    finite_rotation(yaw, pitch, "player_input.finite_rotation")
}

/// Selects the placement rejection in the Go DTO's check order: the rotation,
/// then the hotbar slot range.
fn place_block_rule(yaw: f32, pitch: f32, slot: u8) -> Option<Rejection> {
    if let Some(rejection) = finite_rotation(yaw, pitch, "place_block.finite_rotation") {
        return Some(rejection);
    }
    if slot >= HotbarSlot::COUNT {
        return Some(Rejection {
            category: "invalid-value",
            rule: "place_block.slot_range",
            error: DomainError::InvalidHotbarSlot,
        });
    }
    None
}

/// Selects the hotbar-selection rejection: the slot range.
fn select_hotbar_rule(slot: u8) -> Option<Rejection> {
    if slot >= HotbarSlot::COUNT {
        return Some(Rejection {
            category: "invalid-value",
            rule: "select_hotbar.slot_range",
            error: DomainError::InvalidHotbarSlot,
        });
    }
    None
}

/// Selects the resync rejection: an unknown dimension is an enum rejection.
///
/// Only the overworld and depths identifiers are in range, so both a negative
/// and an above-`u8` value report the same normalized rule rather than a hard
/// failure.
fn chunk_resync_rule(dimension: i32) -> Option<Rejection> {
    if dimension == i32::from(Dimension::OVERWORLD.get())
        || dimension == i32::from(Dimension::DEPTHS.get())
    {
        return None;
    }
    Some(Rejection {
        category: "invalid-enum",
        rule: "chunk_resync.dimension",
        error: DomainError::InvalidDimension,
    })
}

/// Selects the shared finite-angle rejection one ray intent publishes.
fn ray_rule(yaw: f32, pitch: f32, rule: &'static str) -> Option<Rejection> {
    finite_rotation(yaw, pitch, rule)
}

/// Reports the shared finite-rotation rejection under one exact rule name.
fn finite_rotation(yaw: f32, pitch: f32, rule: &'static str) -> Option<Rejection> {
    if yaw.is_finite() && pitch.is_finite() {
        return None;
    }
    Some(Rejection {
        category: "invalid-value",
        rule,
        error: DomainError::NonFiniteRotation,
    })
}

/// Builds the rejected movement projection from the parsed input, because a
/// rejected Rust value has no getters to read.
#[allow(clippy::too_many_arguments)]
fn player_input_projection(
    sequence: u64,
    move_x: i8,
    move_z: i8,
    jump: bool,
    yaw: f32,
    pitch: f32,
    mining: bool,
    eating: bool,
    sprinting: bool,
    sneaking: bool,
) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("sequence".to_string(), normalize_u64(sequence));
    fields.insert("move_x".to_string(), Value::from(move_x));
    fields.insert("move_z".to_string(), Value::from(move_z));
    fields.insert("jump".to_string(), Value::Bool(jump));
    fields.insert("yaw".to_string(), normalize_f32(yaw));
    fields.insert("pitch".to_string(), normalize_f32(pitch));
    fields.insert("mining".to_string(), Value::Bool(mining));
    fields.insert("eating".to_string(), Value::Bool(eating));
    fields.insert("sprinting".to_string(), Value::Bool(sprinting));
    fields.insert("sneaking".to_string(), Value::Bool(sneaking));
    fields
}

/// Builds the rejected placement projection from the parsed input.
fn place_block_projection(sequence: u64, yaw: f32, pitch: f32, slot: u8) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("sequence".to_string(), normalize_u64(sequence));
    fields.insert("yaw".to_string(), normalize_f32(yaw));
    fields.insert("pitch".to_string(), normalize_f32(pitch));
    fields.insert("slot".to_string(), Value::from(slot));
    fields
}

/// Builds the rejected hotbar-selection projection from the parsed input.
fn select_hotbar_projection(sequence: u64, slot: u8) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("sequence".to_string(), normalize_u64(sequence));
    fields.insert("slot".to_string(), Value::from(slot));
    fields
}

/// Builds the rejected resync projection from the parsed input. The dimension
/// is rendered from the raw `i32`, so a value the Rust dimension type cannot
/// represent still publishes the value the case named.
fn chunk_resync_projection(
    sequence: u64,
    dimension: i32,
    chunk_x: i32,
    chunk_z: i32,
    have_revision: u64,
) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("sequence".to_string(), normalize_u64(sequence));
    fields.insert("dimension".to_string(), Value::from(dimension));
    fields.insert("chunk_x".to_string(), Value::from(chunk_x));
    fields.insert("chunk_z".to_string(), Value::from(chunk_z));
    fields.insert("have_revision".to_string(), normalize_u64(have_revision));
    fields
}

/// Builds the rejected ray projection from the parsed input.
fn ray_projection(sequence: u64, yaw: f32, pitch: f32) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("sequence".to_string(), normalize_u64(sequence));
    fields.insert("yaw".to_string(), normalize_f32(yaw));
    fields.insert("pitch".to_string(), normalize_f32(pitch));
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
fn command_control_execute_28_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}

/// Proves the shared comparator actually compares semantic fields: the produced
/// accepted result matches its frozen expectation, while the same actual value
/// no longer matches once one normalized field is mutated.
#[test]
fn command_control_comparator_detects_semantic_mutation() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    let accepted = executed
        .into_iter()
        .find(|case| {
            case.actual.get("kind").and_then(|kind| kind.as_str()) == Some("ok")
                && case
                    .actual
                    .get("category")
                    .and_then(|category| category.as_str())
                    == Some("player-input")
        })
        .expect("one accepted player-input case");

    assert_domain_normalized(&accepted);

    let mut mutated_case = accepted.case;
    let fields = mutated_case
        .normalized
        .get_mut("fields")
        .and_then(|fields| fields.as_object_mut())
        .expect("normalized fields object");
    fields.insert("sequence".to_string(), Value::String("9999".to_string()));

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
