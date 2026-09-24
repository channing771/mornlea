//! Topic module for domain.event hostile/passive mob corpus cases.
//!
//! The six rules here are the mob observations an authoritative session
//! publishes to a subscribed client: a spawn and despawn pair plus the
//! per-tick state batch for the hostiles, and the same triple for the
//! passive mobs with a removal reason on the despawn. Each executor parses
//! the case input, selects the Go rejection rule from the raw values in the
//! producer's stated precedence, and then proves the applicable Rust
//! constructor agrees: a classified acceptance must construct, and a
//! classified rejection must fail with exactly the mapped `DomainError`.
//! Accepted fields are read from the constructed Rust values; a rejected
//! value has no getters, so its projection is built from the parsed input
//! instead.
//!
//! Three raw byte domains stay classifier-only because no closed Rust type
//! can carry their unknown values: a hostile kind byte outside {0, 1}, a
//! grazing byte outside {0, 1} and a passive despawn reason outside
//! {0, 1}. The adapter still emits the exact Go category and rule for them,
//! but no constructor verdict exists to check, and no `Unknown` enum variant
//! is invented. The 64-record packet maxima are transport budgets no frozen
//! case exercises; the semantic walk below bounds work with the shared
//! 4,096 cap first, exactly as the batch constructors do.
//!
//! Submitted record order is never sorted: the classifier replays each
//! record in input order and checks the strictly-increasing identity rule
//! against the previous record after each otherwise valid one, and both the
//! accepted and the rejected projection publish the batch exactly as it was
//! submitted.

use super::support::{
    DispatchError, JsonMap, assert_domain_normalized, exact_array, execute_topic, input_object,
    invalid_case, normalize_f32, normalize_u64, normalized_error, normalized_ok, parse_f32_token,
    required_array, required_i32, required_string, required_u8, value_object, value_string,
};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};
use mornlea_domain::{
    Dimension, DomainError, FiniteVec3, HostileDespawn, HostileDespawnParts, HostileId,
    HostileKind, HostileSpawn, HostileSpawnParts, HostileSpawnRecord, HostileSpawnRecordParts,
    HostileState, HostileStateParts, HostileStateRecord, HostileStateRecordParts,
    MAX_SEMANTIC_BATCH_RECORDS, PassiveDespawn, PassiveDespawnParts, PassiveDespawnReason,
    PassiveDespawnRecord, PassiveId, PassiveSpawn, PassiveSpawnParts, PassiveSpawnRecord,
    PassiveSpawnRecordParts, PassiveState, PassiveStateParts, PassiveStateRecord,
    PassiveStateRecordParts,
};
use serde_json::Value;

pub const EXPECTED_COUNT: usize = 68;

const RULES: &[&str] = &[
    "hostile-spawn",
    "hostile-state",
    "hostile-despawn",
    "passive-spawn",
    "passive-state",
    "passive-despawn",
];

pub fn owns(case: &FrozenCase) -> bool {
    if case.consumer != CorpusConsumer::Domain
        || case.family != "domain.event"
        || case.version != "1"
        || case.operation != "admit"
    {
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
        "hostile-spawn" => execute_hostile_spawn(case, input),
        "hostile-state" => execute_hostile_state(case, input),
        "hostile-despawn" => execute_hostile_despawn(case, input),
        "passive-spawn" => execute_passive_spawn(case, input),
        "passive-state" => execute_passive_state(case, input),
        "passive-despawn" => execute_passive_despawn(case, input),
        unknown => Err(invalid_case(
            case,
            format!("unknown mobs event rule '{unknown}'"),
        )),
    }
}

/// One classified rejection: its normalized category, its exact Go rule name,
/// and the `DomainError` the owning constructor must return.
///
/// The rule is a `String` because the per-record rules are indexed by record
/// (`hostile_spawn.record_0.health`). `error` is `None` for a
/// classifier-only rejection whose bad raw value cannot enter any Rust type,
/// so no constructor verdict exists to check.
struct Rejection {
    category: &'static str,
    rule: String,
    error: Option<DomainError>,
}

impl Rejection {
    /// A rejection only the classifier can decide, because the rejected raw
    /// value has no constructible Rust representation.
    fn classifier(category: &'static str, rule: String) -> Self {
        Self {
            category,
            rule,
            error: None,
        }
    }
}

/// Overworld dimension ID, from the Go `core.Overworld`. Both spawn rules
/// publish the overworld alone.
const DIMENSION_OVERWORLD: i32 = 0;

/// Inclusive health maximum, from the Go `core.MaxHealth` the record
/// constructors share through the domain's `MAX_HEALTH`.
const MAX_HEALTH: u8 = 20;

/// The shared semantic batch cap the domain constructors bound work with.
const SEMANTIC_BATCH_CAP: usize = MAX_SEMANTIC_BATCH_RECORDS;

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

/// Maps one constructor error to a classification conflict, so every accepted
/// construction failure names the contract conflict instead of panicking.
fn construction_conflict(case: &FrozenCase, subject: &str, error: DomainError) -> DispatchError {
    classification_conflict(case, subject, &format!("{error:?}"))
}

/// Parses one decimal `u64` token, the lossless rendering the frozen inputs
/// use for ticks and entity identities.
fn parse_decimal_u64(case: &FrozenCase, path: &str, text: &str) -> Result<u64, DispatchError> {
    text.parse::<u64>().map_err(|_| {
        invalid_case(
            case,
            format!("expected decimal u64 token at '{path}': '{text}'"),
        )
    })
}

/// Parses the required decimal-string server tick every rule carries. A zero
/// tick is a meaningful publish instant, so it stays admitted.
fn parse_server_tick(case: &FrozenCase, input: &JsonMap) -> Result<u64, DispatchError> {
    parse_decimal_u64(
        case,
        "server_tick",
        required_string(case, input, "server_tick")?,
    )
}

/// Parses one exact XYZ float-token position, preserving component order.
fn parse_position(case: &FrozenCase, object: &JsonMap) -> Result<[f32; 3], DispatchError> {
    let values = required_array(case, object, "position")?;
    let slots = exact_array::<3>(case, "position", values)?;
    let mut position = [0.0f32; 3];
    for (index, value) in slots.iter().enumerate() {
        let path = format!("position[{index}]");
        position[index] = parse_f32_token(case, &path, value_string(case, &path, value)?)?;
    }
    Ok(position)
}

/// Parses one float-token yaw field.
fn parse_yaw(case: &FrozenCase, object: &JsonMap) -> Result<f32, DispatchError> {
    parse_f32_token(case, "yaw", required_string(case, object, "yaw")?)
}

/// Renders one XYZ position in the normalized bit-exact form.
fn position_value(position: [f32; 3]) -> Value {
    Value::Array(
        position
            .iter()
            .map(|component| normalize_f32(*component))
            .collect(),
    )
}

/// Renders one identity in the normalized decimal-string form.
fn id_value(id: u64) -> Value {
    normalize_u64(id)
}

/// Renders the grazing byte the way the normalized vocabulary renders it: the
/// two admitted values as Booleans, any other raw number unchanged so a
/// rejection shows the exact byte the record carried.
fn grazing_value(grazing: u8) -> Value {
    match grazing {
        0 => Value::Bool(false),
        1 => Value::Bool(true),
        raw => Value::from(raw),
    }
}

/// Renders one hostile kind through its exhaustive match, so adding a variant
/// is a compile error here rather than a silent renumbering.
fn hostile_kind_value(kind: HostileKind) -> Value {
    match kind {
        HostileKind::Nightwalker => Value::from(0),
        HostileKind::BoneThrower => Value::from(1),
    }
}

/// Renders one passive despawn reason through its exhaustive match.
fn passive_reason_value(reason: PassiveDespawnReason) -> Value {
    match reason {
        PassiveDespawnReason::Vanished => Value::from(0),
        PassiveDespawnReason::Died => Value::from(1),
    }
}

/// The batch-level count rejection shared by all six rules: a raw count above
/// the semantic cap classifies first, then an empty batch.
fn count_rejection(subject: &str, count: usize) -> Option<Rejection> {
    if count > SEMANTIC_BATCH_CAP {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.count_range"),
            error: Some(DomainError::BatchTooLarge),
        });
    }
    if count == 0 {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.count_range"),
            error: Some(DomainError::EmptyStateBatch),
        });
    }
    None
}

/// The strictly-increasing identity rejection shared by all six rules.
fn order_rejection(subject: &str) -> Rejection {
    Rejection {
        category: "invalid-value",
        rule: format!("{subject}.strictly_increasing_ids"),
        error: Some(DomainError::InvalidStateOrder),
    }
}

/// Admits one raw dimension for a spawn record: the overworld alone passes,
/// any other fitting byte rejects with the record constructor's
/// `InvalidDimension`, and a value outside the byte range cannot enter any
/// Rust type, so only the classifier decides.
fn admit_spawn_dimension(raw: i32) -> Result<u8, Option<DomainError>> {
    if raw == DIMENSION_OVERWORLD {
        return Ok(0);
    }
    match u8::try_from(raw) {
        Ok(_) => Err(Some(DomainError::InvalidDimension)),
        Err(_) => Err(None),
    }
}

/// The health range every body record shares: `1..=MAX_HEALTH` inclusive.
fn health_admits(health: u8) -> bool {
    (1..=MAX_HEALTH).contains(&health)
}

/// The parsed raw shape of one hostile spawn record.
struct RawHostileSpawn {
    id: u64,
    dimension: i32,
    position: [f32; 3],
    yaw: f32,
    health: u8,
    kind: u8,
}

/// The parsed raw shape of one hostile state record.
struct RawHostileState {
    id: u64,
    position: [f32; 3],
    velocity: [f32; 3],
    yaw: f32,
    health: u8,
    kind: u8,
}

/// The parsed raw shape of one passive spawn record.
struct RawPassiveSpawn {
    id: u64,
    dimension: i32,
    position: [f32; 3],
    yaw: f32,
    health: u8,
}

/// The parsed raw shape of one passive state record.
struct RawPassiveState {
    id: u64,
    position: [f32; 3],
    velocity: [f32; 3],
    yaw: f32,
    health: u8,
    grazing: u8,
}

/// The parsed raw shape of one passive despawn record.
struct RawPassiveDespawn {
    id: u64,
    reason: u8,
}

/// Parses one required ordered batch of hostile spawn records.
fn parse_hostile_spawns(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<Vec<RawHostileSpawn>, DispatchError> {
    required_array(case, input, "spawns")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("spawns[{index}]");
            let object = value_object(case, &path, value)?;
            Ok(RawHostileSpawn {
                id: parse_decimal_u64(
                    case,
                    &format!("{path}.id"),
                    required_string(case, object, "id")?,
                )?,
                dimension: required_i32(case, object, "dimension")?,
                position: parse_position(case, object)?,
                yaw: parse_yaw(case, object)?,
                health: required_u8(case, object, "health")?,
                kind: required_u8(case, object, "kind")?,
            })
        })
        .collect()
}

/// Parses one required ordered batch of hostile state records.
fn parse_hostile_states(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<Vec<RawHostileState>, DispatchError> {
    required_array(case, input, "states")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("states[{index}]");
            let object = value_object(case, &path, value)?;
            Ok(RawHostileState {
                id: parse_decimal_u64(
                    case,
                    &format!("{path}.id"),
                    required_string(case, object, "id")?,
                )?,
                position: parse_position(case, object)?,
                velocity: parse_velocity(case, object)?,
                yaw: parse_yaw(case, object)?,
                health: required_u8(case, object, "health")?,
                kind: required_u8(case, object, "kind")?,
            })
        })
        .collect()
}

/// Parses one required ordered batch of decimal-string identities.
fn parse_ids(case: &FrozenCase, input: &JsonMap) -> Result<Vec<u64>, DispatchError> {
    required_array(case, input, "ids")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("ids[{index}]");
            parse_decimal_u64(case, &path, value_string(case, &path, value)?)
        })
        .collect()
}

/// Parses one required ordered batch of passive spawn records.
fn parse_passive_spawns(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<Vec<RawPassiveSpawn>, DispatchError> {
    required_array(case, input, "spawns")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("spawns[{index}]");
            let object = value_object(case, &path, value)?;
            Ok(RawPassiveSpawn {
                id: parse_decimal_u64(
                    case,
                    &format!("{path}.id"),
                    required_string(case, object, "id")?,
                )?,
                dimension: required_i32(case, object, "dimension")?,
                position: parse_position(case, object)?,
                yaw: parse_yaw(case, object)?,
                health: required_u8(case, object, "health")?,
            })
        })
        .collect()
}

/// Parses one required ordered batch of passive state records.
fn parse_passive_states(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<Vec<RawPassiveState>, DispatchError> {
    required_array(case, input, "states")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("states[{index}]");
            let object = value_object(case, &path, value)?;
            Ok(RawPassiveState {
                id: parse_decimal_u64(
                    case,
                    &format!("{path}.id"),
                    required_string(case, object, "id")?,
                )?,
                position: parse_position(case, object)?,
                velocity: parse_velocity(case, object)?,
                yaw: parse_yaw(case, object)?,
                health: required_u8(case, object, "health")?,
                grazing: required_u8(case, object, "grazing")?,
            })
        })
        .collect()
}

/// Parses one required ordered batch of passive despawn records.
fn parse_passive_despawns(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<Vec<RawPassiveDespawn>, DispatchError> {
    required_array(case, input, "despawns")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("despawns[{index}]");
            let object = value_object(case, &path, value)?;
            Ok(RawPassiveDespawn {
                id: parse_decimal_u64(
                    case,
                    &format!("{path}.id"),
                    required_string(case, object, "id")?,
                )?,
                reason: required_u8(case, object, "reason")?,
            })
        })
        .collect()
}

/// Parses one exact XYZ float-token velocity, preserving component order.
fn parse_velocity(case: &FrozenCase, object: &JsonMap) -> Result<[f32; 3], DispatchError> {
    let values = required_array(case, object, "velocity")?;
    let slots = exact_array::<3>(case, "velocity", values)?;
    let mut velocity = [0.0f32; 3];
    for (index, value) in slots.iter().enumerate() {
        let path = format!("velocity[{index}]");
        velocity[index] = parse_f32_token(case, &path, value_string(case, &path, value)?)?;
    }
    Ok(velocity)
}

/// Executes one `hostile-spawn` case.
///
/// The walk replays the Go validator exactly: the raw count bound, then each
/// record's identity, overworld-only dimension, finite pose, finite yaw,
/// health range and kind domain in submitted order, then the
/// strictly-increasing identity rule after each otherwise valid record.
fn execute_hostile_spawn(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "hostile-spawn";
    let subject_rule = "hostile_spawn";
    let server_tick = parse_server_tick(case, input)?;
    let spawns = parse_hostile_spawns(case, input)?;

    if let Some(rejection) = count_rejection(subject_rule, spawns.len()) {
        verify_hostile_spawn_rejection(case, subject, server_tick, &spawns, &rejection)?;
        let mut fields = JsonMap::new();
        fields.insert("server_tick".to_string(), normalize_u64(server_tick));
        fields.insert("spawns".to_string(), hostile_spawns_raw_value(&spawns));
        return normalized_error(case, rejection.category, &rejection.rule, fields);
    }
    for (index, spawn) in spawns.iter().enumerate() {
        if let Some(rejection) = hostile_spawn_record_rejection(subject_rule, index, spawn) {
            verify_hostile_spawn_rejection(case, subject, server_tick, &spawns, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert("server_tick".to_string(), normalize_u64(server_tick));
            fields.insert("spawns".to_string(), hostile_spawns_raw_value(&spawns));
            return normalized_error(case, rejection.category, &rejection.rule, fields);
        }
        if index > 0 && spawns[index - 1].id >= spawn.id {
            let rejection = order_rejection(subject_rule);
            verify_hostile_spawn_rejection(case, subject, server_tick, &spawns, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert("server_tick".to_string(), normalize_u64(server_tick));
            fields.insert("spawns".to_string(), hostile_spawns_raw_value(&spawns));
            return normalized_error(case, rejection.category, &rejection.rule, fields);
        }
    }

    let mut records = Vec::with_capacity(spawns.len());
    for spawn in &spawns {
        records.push(construct_hostile_spawn_record(case, subject, spawn)?);
    }
    let batch = HostileSpawn::try_new(HostileSpawnParts {
        server_tick,
        spawns: records.into_boxed_slice(),
    })
    .map_err(|error| construction_conflict(case, subject, error))?;
    let mut fields = JsonMap::new();
    fields.insert(
        "server_tick".to_string(),
        normalize_u64(batch.server_tick()),
    );
    fields.insert(
        "spawns".to_string(),
        Value::Array(
            batch
                .spawns()
                .iter()
                .map(|record| {
                    serde_json::json!({
                        "id": id_value(record.id().get()),
                        "dimension": record.dimension().get(),
                        "position": position_value(record.position().get()),
                        "yaw": normalize_f32(record.yaw()),
                        "health": record.health(),
                        "kind": hostile_kind_value(record.kind()),
                    })
                })
                .collect(),
        ),
    );
    Ok(normalized_ok(subject, fields))
}

/// Selects the first rule one hostile spawn record breaks, in the Go
/// validator's order. The kind byte outside {0, 1} is classifier-only.
fn hostile_spawn_record_rejection(
    subject: &str,
    index: usize,
    spawn: &RawHostileSpawn,
) -> Option<Rejection> {
    if spawn.id == 0 {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("{subject}.record_{index}.id"),
            error: Some(DomainError::InvalidIdentity),
        });
    }
    if let Err(error) = admit_spawn_dimension(spawn.dimension) {
        return Some(match error {
            Some(expected) => Rejection {
                category: "invalid-enum",
                rule: format!("{subject}.record_{index}.dimension"),
                error: Some(expected),
            },
            None => Rejection::classifier(
                "invalid-enum",
                format!("{subject}.record_{index}.dimension"),
            ),
        });
    }
    if spawn
        .position
        .iter()
        .any(|component| !component.is_finite())
    {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.position"),
            error: Some(DomainError::NonFiniteValue),
        });
    }
    if !spawn.yaw.is_finite() {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.yaw"),
            error: Some(DomainError::NonFiniteRotation),
        });
    }
    if !health_admits(spawn.health) {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.health"),
            error: Some(DomainError::InvalidSurvivalValue),
        });
    }
    if spawn.kind > 1 {
        return Some(Rejection::classifier(
            "invalid-enum",
            format!("{subject}.record_{index}.kind"),
        ));
    }
    None
}

/// Verifies the constructor agreement for one classified hostile spawn
/// rejection.
fn verify_hostile_spawn_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    spawns: &[RawHostileSpawn],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> =
        match rejection.rule.as_str() {
            "hostile_spawn.count_range" => hostile_spawn_batch_verdict(server_tick, spawns),
            "hostile_spawn.strictly_increasing_ids" => {
                let prefix = order_prefix_end(spawns.len(), |index| {
                    spawns[index - 1].id >= spawns[index].id
                })
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
                let mut records = Vec::new();
                for spawn in &spawns[..prefix] {
                    records.push(construct_hostile_spawn_record(case, subject, spawn)?);
                }
                HostileSpawn::try_new(HostileSpawnParts {
                    server_tick,
                    spawns: records.into_boxed_slice(),
                })
                .map(|_| ())
            }
            rule => {
                let index = record_index(case, subject, rule, "hostile_spawn.record_")?;
                let spawn = spawns
                    .get(index)
                    .ok_or_else(|| classification_conflict(case, subject, rule))?;
                let verdict =
                    match rule.rsplit('.').next().unwrap_or("") {
                        "id" => HostileId::try_new(spawn.id).map(|_| ()),
                        "dimension" => HostileSpawnRecord::try_new(hostile_spawn_record_parts(
                            case,
                            subject,
                            spawn,
                            spawn.dimension,
                        )?)
                        .map(|_| ()),
                        "position" => FiniteVec3::try_new(spawn.position).map(|_| ()),
                        "yaw" | "health" => HostileSpawnRecord::try_new(
                            hostile_spawn_record_parts(case, subject, spawn, DIMENSION_OVERWORLD)?,
                        )
                        .map(|_| ()),
                        _ => return Err(classification_conflict(case, subject, rule)),
                    };
                return match verdict {
                    Err(error) if error == expected => Ok(()),
                    _ => Err(classification_conflict(case, subject, rule)),
                };
            }
        };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Runs the batch constructor verdict for the count rules: an empty batch
/// rejects with `EmptyStateBatch`, and a synthetic batch one record above the
/// semantic cap rejects with `BatchTooLarge` even when the case's own records
/// cannot individually construct.
fn hostile_spawn_batch_verdict(
    server_tick: u64,
    spawns: &[RawHostileSpawn],
) -> Result<(), DomainError> {
    if spawns.is_empty() {
        return HostileSpawn::try_new(HostileSpawnParts {
            server_tick,
            spawns: Vec::new().into_boxed_slice(),
        })
        .map(|_| ());
    }
    HostileSpawn::try_new(HostileSpawnParts {
        server_tick,
        spawns: minimal_hostile_spawn_records(SEMANTIC_BATCH_CAP + 1),
    })
    .map(|_| ())
}

/// Builds `count` minimal valid hostile spawn records with strictly
/// increasing identities, the synthetic payload the count-overflow verdict
/// runs on.
fn minimal_hostile_spawn_records(count: usize) -> Box<[HostileSpawnRecord]> {
    let position = FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite synthetic position");
    (1..=count as u64)
        .map(|id| {
            HostileSpawnRecord::try_new(HostileSpawnRecordParts {
                id: HostileId::try_new(id).expect("nonzero synthetic identity"),
                dimension: Dimension::OVERWORLD,
                position,
                yaw: 0.0,
                health: 1,
                kind: HostileKind::Nightwalker,
            })
            .expect("valid synthetic record")
        })
        .collect()
}

/// Builds one checked hostile spawn record's parts with the dimension forced
/// to the named raw value, the shape the dimension, yaw and health verdicts
/// run on. Every field the classifier already cleared must construct here; a
/// failure names the contract conflict.
fn hostile_spawn_record_parts(
    case: &FrozenCase,
    subject: &str,
    spawn: &RawHostileSpawn,
    dimension: i32,
) -> Result<HostileSpawnRecordParts, DispatchError> {
    let byte = u8::try_from(dimension)
        .map_err(|_| classification_conflict(case, subject, "dimension outside the byte range"))?;
    Ok(HostileSpawnRecordParts {
        id: HostileId::try_new(spawn.id)
            .map_err(|error| construction_conflict(case, subject, error))?,
        dimension: Dimension::new(byte)
            .map_err(|error| construction_conflict(case, subject, error))?,
        position: FiniteVec3::try_new(spawn.position)
            .map_err(|error| construction_conflict(case, subject, error))?,
        yaw: spawn.yaw,
        health: spawn.health,
        kind: match spawn.kind {
            0 => HostileKind::Nightwalker,
            1 => HostileKind::BoneThrower,
            raw => {
                return Err(classification_conflict(
                    case,
                    subject,
                    &format!("kind byte {raw} has no Rust variant"),
                ));
            }
        },
    })
}

/// Constructs one checked hostile spawn record for an admitted case.
fn construct_hostile_spawn_record(
    case: &FrozenCase,
    subject: &str,
    spawn: &RawHostileSpawn,
) -> Result<HostileSpawnRecord, DispatchError> {
    HostileSpawnRecord::try_new(hostile_spawn_record_parts(
        case,
        subject,
        spawn,
        spawn.dimension,
    )?)
    .map_err(|error| construction_conflict(case, subject, error))
}

/// Renders one raw hostile spawn batch in the normalized field map.
fn hostile_spawns_raw_value(spawns: &[RawHostileSpawn]) -> Value {
    Value::Array(
        spawns
            .iter()
            .map(|spawn| {
                serde_json::json!({
                    "id": id_value(spawn.id),
                    "dimension": spawn.dimension,
                    "position": position_value(spawn.position),
                    "yaw": normalize_f32(spawn.yaw),
                    "health": spawn.health,
                    "kind": spawn.kind,
                })
            })
            .collect(),
    )
}

/// Executes one `hostile-state` case, whose records carry the velocity the
/// spawn omits and no dimension, exactly as the wire record does.
fn execute_hostile_state(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "hostile-state";
    let subject_rule = "hostile_state";
    let server_tick = parse_server_tick(case, input)?;
    let states = parse_hostile_states(case, input)?;

    if let Some(rejection) = count_rejection(subject_rule, states.len()) {
        verify_hostile_state_rejection(case, subject, server_tick, &states, &rejection)?;
        return hostile_state_error(case, server_tick, &states, &rejection);
    }
    for (index, state) in states.iter().enumerate() {
        if let Some(rejection) = hostile_state_record_rejection(subject_rule, index, state) {
            verify_hostile_state_rejection(case, subject, server_tick, &states, &rejection)?;
            return hostile_state_error(case, server_tick, &states, &rejection);
        }
        if index > 0 && states[index - 1].id >= state.id {
            let rejection = order_rejection(subject_rule);
            verify_hostile_state_rejection(case, subject, server_tick, &states, &rejection)?;
            return hostile_state_error(case, server_tick, &states, &rejection);
        }
    }

    let mut records = Vec::with_capacity(states.len());
    for state in &states {
        records.push(construct_hostile_state_record(case, subject, state)?);
    }
    let batch = HostileState::try_new(HostileStateParts {
        server_tick,
        states: records.into_boxed_slice(),
    })
    .map_err(|error| construction_conflict(case, subject, error))?;
    let mut fields = JsonMap::new();
    fields.insert(
        "server_tick".to_string(),
        normalize_u64(batch.server_tick()),
    );
    fields.insert(
        "states".to_string(),
        Value::Array(
            batch
                .states()
                .iter()
                .map(|record| {
                    serde_json::json!({
                        "id": id_value(record.id().get()),
                        "position": position_value(record.position().get()),
                        "velocity": position_value(record.velocity().get()),
                        "yaw": normalize_f32(record.yaw()),
                        "health": record.health(),
                        "kind": hostile_kind_value(record.kind()),
                    })
                })
                .collect(),
        ),
    );
    Ok(normalized_ok(subject, fields))
}

/// Publishes one classified hostile state rejection from the raw batch.
fn hostile_state_error(
    case: &FrozenCase,
    server_tick: u64,
    states: &[RawHostileState],
    rejection: &Rejection,
) -> Result<serde_json::Value, DispatchError> {
    let mut fields = JsonMap::new();
    fields.insert("server_tick".to_string(), normalize_u64(server_tick));
    fields.insert("states".to_string(), hostile_states_raw_value(states));
    normalized_error(case, rejection.category, &rejection.rule, fields)
}

/// Selects the first rule one hostile state record breaks, in the Go
/// validator's order. The kind byte outside {0, 1} is classifier-only.
fn hostile_state_record_rejection(
    subject: &str,
    index: usize,
    state: &RawHostileState,
) -> Option<Rejection> {
    if state.id == 0 {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("{subject}.record_{index}.id"),
            error: Some(DomainError::InvalidIdentity),
        });
    }
    if state
        .position
        .iter()
        .any(|component| !component.is_finite())
    {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.position"),
            error: Some(DomainError::NonFiniteValue),
        });
    }
    if state
        .velocity
        .iter()
        .any(|component| !component.is_finite())
    {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.velocity"),
            error: Some(DomainError::NonFiniteValue),
        });
    }
    if !state.yaw.is_finite() {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.yaw"),
            error: Some(DomainError::NonFiniteRotation),
        });
    }
    if !health_admits(state.health) {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.health"),
            error: Some(DomainError::InvalidSurvivalValue),
        });
    }
    if state.kind > 1 {
        return Some(Rejection::classifier(
            "invalid-enum",
            format!("{subject}.record_{index}.kind"),
        ));
    }
    None
}

/// Verifies the constructor agreement for one classified hostile state
/// rejection.
fn verify_hostile_state_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    states: &[RawHostileState],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "hostile_state.count_range" => {
            if states.is_empty() {
                HostileState::try_new(HostileStateParts {
                    server_tick,
                    states: Vec::new().into_boxed_slice(),
                })
                .map(|_| ())
            } else {
                HostileState::try_new(HostileStateParts {
                    server_tick,
                    states: minimal_hostile_state_records(SEMANTIC_BATCH_CAP + 1),
                })
                .map(|_| ())
            }
        }
        "hostile_state.strictly_increasing_ids" => {
            let prefix = order_prefix_end(states.len(), |index| {
                states[index - 1].id >= states[index].id
            })
            .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let mut records = Vec::new();
            for state in &states[..prefix] {
                records.push(construct_hostile_state_record(case, subject, state)?);
            }
            HostileState::try_new(HostileStateParts {
                server_tick,
                states: records.into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let index = record_index(case, subject, rule, "hostile_state.record_")?;
            let state = states
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            let verdict = match rule.rsplit('.').next().unwrap_or("") {
                "id" => HostileId::try_new(state.id).map(|_| ()),
                "position" => FiniteVec3::try_new(state.position).map(|_| ()),
                "velocity" => FiniteVec3::try_new(state.velocity).map(|_| ()),
                "yaw" | "health" => {
                    HostileStateRecord::try_new(hostile_state_record_parts(case, subject, state)?)
                        .map(|_| ())
                }
                _ => return Err(classification_conflict(case, subject, rule)),
            };
            return match verdict {
                Err(error) if error == expected => Ok(()),
                _ => Err(classification_conflict(case, subject, rule)),
            };
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Builds `count` minimal valid hostile state records with strictly
/// increasing identities.
fn minimal_hostile_state_records(count: usize) -> Box<[HostileStateRecord]> {
    let vector = FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite synthetic vector");
    (1..=count as u64)
        .map(|id| {
            HostileStateRecord::try_new(HostileStateRecordParts {
                id: HostileId::try_new(id).expect("nonzero synthetic identity"),
                position: vector,
                velocity: vector,
                yaw: 0.0,
                health: 1,
                kind: HostileKind::Nightwalker,
            })
            .expect("valid synthetic record")
        })
        .collect()
}

/// Builds one checked hostile state record's parts from the raw record.
fn hostile_state_record_parts(
    case: &FrozenCase,
    subject: &str,
    state: &RawHostileState,
) -> Result<HostileStateRecordParts, DispatchError> {
    Ok(HostileStateRecordParts {
        id: HostileId::try_new(state.id)
            .map_err(|error| construction_conflict(case, subject, error))?,
        position: FiniteVec3::try_new(state.position)
            .map_err(|error| construction_conflict(case, subject, error))?,
        velocity: FiniteVec3::try_new(state.velocity)
            .map_err(|error| construction_conflict(case, subject, error))?,
        yaw: state.yaw,
        health: state.health,
        kind: match state.kind {
            0 => HostileKind::Nightwalker,
            1 => HostileKind::BoneThrower,
            raw => {
                return Err(classification_conflict(
                    case,
                    subject,
                    &format!("kind byte {raw} has no Rust variant"),
                ));
            }
        },
    })
}

/// Constructs one checked hostile state record for an admitted case.
fn construct_hostile_state_record(
    case: &FrozenCase,
    subject: &str,
    state: &RawHostileState,
) -> Result<HostileStateRecord, DispatchError> {
    HostileStateRecord::try_new(hostile_state_record_parts(case, subject, state)?)
        .map_err(|error| construction_conflict(case, subject, error))
}

/// Renders one raw hostile state batch in the normalized field map.
fn hostile_states_raw_value(states: &[RawHostileState]) -> Value {
    Value::Array(
        states
            .iter()
            .map(|state| {
                serde_json::json!({
                    "id": id_value(state.id),
                    "position": position_value(state.position),
                    "velocity": position_value(state.velocity),
                    "yaw": normalize_f32(state.yaw),
                    "health": state.health,
                    "kind": state.kind,
                })
            })
            .collect(),
    )
}

/// Executes one `hostile-despawn` case, whose records are bare identities.
fn execute_hostile_despawn(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "hostile-despawn";
    let subject_rule = "hostile_despawn";
    let server_tick = parse_server_tick(case, input)?;
    let ids = parse_ids(case, input)?;

    let rejection = if let Some(rejection) = count_rejection(subject_rule, ids.len()) {
        Some(rejection)
    } else {
        id_walk_rejection(subject_rule, &ids)
    };
    if let Some(rejection) = rejection {
        verify_hostile_despawn_rejection(case, subject, server_tick, &ids, &rejection)?;
        let mut fields = JsonMap::new();
        fields.insert("server_tick".to_string(), normalize_u64(server_tick));
        fields.insert(
            "ids".to_string(),
            Value::Array(ids.iter().map(|id| id_value(*id)).collect()),
        );
        return normalized_error(case, rejection.category, &rejection.rule, fields);
    }

    let mut typed = Vec::with_capacity(ids.len());
    for id in &ids {
        typed.push(
            HostileId::try_new(*id).map_err(|error| construction_conflict(case, subject, error))?,
        );
    }
    let batch = HostileDespawn::try_new(HostileDespawnParts {
        server_tick,
        ids: typed.into_boxed_slice(),
    })
    .map_err(|error| construction_conflict(case, subject, error))?;
    let mut fields = JsonMap::new();
    fields.insert(
        "server_tick".to_string(),
        normalize_u64(batch.server_tick()),
    );
    fields.insert(
        "ids".to_string(),
        Value::Array(batch.ids().iter().map(|id| id_value(id.get())).collect()),
    );
    Ok(normalized_ok(subject, fields))
}

/// Selects the per-record rules one identity batch breaks: each nonzero
/// identity in submitted order, then the strict order against the previous
/// identity after each otherwise valid one.
fn id_walk_rejection(subject: &str, ids: &[u64]) -> Option<Rejection> {
    for (index, id) in ids.iter().enumerate() {
        if *id == 0 {
            return Some(Rejection {
                category: "invalid-identity",
                rule: format!("{subject}.record_{index}.id"),
                error: Some(DomainError::InvalidIdentity),
            });
        }
        if index > 0 && ids[index - 1] >= *id {
            return Some(order_rejection(subject));
        }
    }
    None
}

/// Verifies the constructor agreement for one classified hostile despawn
/// rejection.
fn verify_hostile_despawn_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    ids: &[u64],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "hostile_despawn.count_range" => {
            if ids.is_empty() {
                HostileDespawn::try_new(HostileDespawnParts {
                    server_tick,
                    ids: Vec::new().into_boxed_slice(),
                })
                .map(|_| ())
            } else {
                HostileDespawn::try_new(HostileDespawnParts {
                    server_tick,
                    ids: (1..=(SEMANTIC_BATCH_CAP + 1) as u64)
                        .map(|id| HostileId::try_new(id).expect("nonzero synthetic identity"))
                        .collect::<Vec<_>>()
                        .into_boxed_slice(),
                })
                .map(|_| ())
            }
        }
        "hostile_despawn.strictly_increasing_ids" => {
            let prefix = order_prefix_end(ids.len(), |index| ids[index - 1] >= ids[index])
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            HostileDespawn::try_new(HostileDespawnParts {
                server_tick,
                ids: ids[..prefix]
                    .iter()
                    .map(|id| {
                        HostileId::try_new(*id)
                            .map_err(|error| construction_conflict(case, subject, error))
                    })
                    .collect::<Result<Vec<_>, _>>()?
                    .into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let index = record_index(case, subject, rule, "hostile_despawn.record_")?;
            let id = ids
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            HostileId::try_new(*id).map(|_| ())
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Executes one `passive-spawn` case, whose records carry no kind byte.
fn execute_passive_spawn(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "passive-spawn";
    let subject_rule = "passive_spawn";
    let server_tick = parse_server_tick(case, input)?;
    let spawns = parse_passive_spawns(case, input)?;

    if let Some(rejection) = count_rejection(subject_rule, spawns.len()) {
        verify_passive_spawn_rejection(case, subject, server_tick, &spawns, &rejection)?;
        return passive_spawn_error(case, server_tick, &spawns, &rejection);
    }
    for (index, spawn) in spawns.iter().enumerate() {
        if let Some(rejection) = passive_spawn_record_rejection(subject_rule, index, spawn) {
            verify_passive_spawn_rejection(case, subject, server_tick, &spawns, &rejection)?;
            return passive_spawn_error(case, server_tick, &spawns, &rejection);
        }
        if index > 0 && spawns[index - 1].id >= spawn.id {
            let rejection = order_rejection(subject_rule);
            verify_passive_spawn_rejection(case, subject, server_tick, &spawns, &rejection)?;
            return passive_spawn_error(case, server_tick, &spawns, &rejection);
        }
    }

    let mut records = Vec::with_capacity(spawns.len());
    for spawn in &spawns {
        records.push(construct_passive_spawn_record(case, subject, spawn)?);
    }
    let batch = PassiveSpawn::try_new(PassiveSpawnParts {
        server_tick,
        spawns: records.into_boxed_slice(),
    })
    .map_err(|error| construction_conflict(case, subject, error))?;
    let mut fields = JsonMap::new();
    fields.insert(
        "server_tick".to_string(),
        normalize_u64(batch.server_tick()),
    );
    fields.insert(
        "spawns".to_string(),
        Value::Array(
            batch
                .spawns()
                .iter()
                .map(|record| {
                    serde_json::json!({
                        "id": id_value(record.id().get()),
                        "dimension": record.dimension().get(),
                        "position": position_value(record.position().get()),
                        "yaw": normalize_f32(record.yaw()),
                        "health": record.health(),
                    })
                })
                .collect(),
        ),
    );
    Ok(normalized_ok(subject, fields))
}

/// Publishes one classified passive spawn rejection from the raw batch.
fn passive_spawn_error(
    case: &FrozenCase,
    server_tick: u64,
    spawns: &[RawPassiveSpawn],
    rejection: &Rejection,
) -> Result<serde_json::Value, DispatchError> {
    let mut fields = JsonMap::new();
    fields.insert("server_tick".to_string(), normalize_u64(server_tick));
    fields.insert("spawns".to_string(), passive_spawns_raw_value(spawns));
    normalized_error(case, rejection.category, &rejection.rule, fields)
}

/// Selects the first rule one passive spawn record breaks, in the Go
/// validator's order. No kind is named, because a passive mob has no
/// category to publish.
fn passive_spawn_record_rejection(
    subject: &str,
    index: usize,
    spawn: &RawPassiveSpawn,
) -> Option<Rejection> {
    if spawn.id == 0 {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("{subject}.record_{index}.id"),
            error: Some(DomainError::InvalidIdentity),
        });
    }
    if let Err(error) = admit_spawn_dimension(spawn.dimension) {
        return Some(match error {
            Some(expected) => Rejection {
                category: "invalid-enum",
                rule: format!("{subject}.record_{index}.dimension"),
                error: Some(expected),
            },
            None => Rejection::classifier(
                "invalid-enum",
                format!("{subject}.record_{index}.dimension"),
            ),
        });
    }
    if spawn
        .position
        .iter()
        .any(|component| !component.is_finite())
    {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.position"),
            error: Some(DomainError::NonFiniteValue),
        });
    }
    if !spawn.yaw.is_finite() {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.yaw"),
            error: Some(DomainError::NonFiniteRotation),
        });
    }
    if !health_admits(spawn.health) {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.health"),
            error: Some(DomainError::InvalidSurvivalValue),
        });
    }
    None
}

/// Verifies the constructor agreement for one classified passive spawn
/// rejection.
fn verify_passive_spawn_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    spawns: &[RawPassiveSpawn],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> =
        match rejection.rule.as_str() {
            "passive_spawn.count_range" => {
                if spawns.is_empty() {
                    PassiveSpawn::try_new(PassiveSpawnParts {
                        server_tick,
                        spawns: Vec::new().into_boxed_slice(),
                    })
                    .map(|_| ())
                } else {
                    PassiveSpawn::try_new(PassiveSpawnParts {
                        server_tick,
                        spawns: minimal_passive_spawn_records(SEMANTIC_BATCH_CAP + 1),
                    })
                    .map(|_| ())
                }
            }
            "passive_spawn.strictly_increasing_ids" => {
                let prefix = order_prefix_end(spawns.len(), |index| {
                    spawns[index - 1].id >= spawns[index].id
                })
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
                let mut records = Vec::new();
                for spawn in &spawns[..prefix] {
                    records.push(construct_passive_spawn_record(case, subject, spawn)?);
                }
                PassiveSpawn::try_new(PassiveSpawnParts {
                    server_tick,
                    spawns: records.into_boxed_slice(),
                })
                .map(|_| ())
            }
            rule => {
                let index = record_index(case, subject, rule, "passive_spawn.record_")?;
                let spawn = spawns
                    .get(index)
                    .ok_or_else(|| classification_conflict(case, subject, rule))?;
                let verdict =
                    match rule.rsplit('.').next().unwrap_or("") {
                        "id" => PassiveId::try_new(spawn.id).map(|_| ()),
                        "dimension" => PassiveSpawnRecord::try_new(passive_spawn_record_parts(
                            case,
                            subject,
                            spawn,
                            spawn.dimension,
                        )?)
                        .map(|_| ()),
                        "position" => FiniteVec3::try_new(spawn.position).map(|_| ()),
                        "yaw" | "health" => PassiveSpawnRecord::try_new(
                            passive_spawn_record_parts(case, subject, spawn, DIMENSION_OVERWORLD)?,
                        )
                        .map(|_| ()),
                        _ => return Err(classification_conflict(case, subject, rule)),
                    };
                return match verdict {
                    Err(error) if error == expected => Ok(()),
                    _ => Err(classification_conflict(case, subject, rule)),
                };
            }
        };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Builds `count` minimal valid passive spawn records with strictly
/// increasing identities.
fn minimal_passive_spawn_records(count: usize) -> Box<[PassiveSpawnRecord]> {
    let position = FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite synthetic position");
    (1..=count as u64)
        .map(|id| {
            PassiveSpawnRecord::try_new(PassiveSpawnRecordParts {
                id: PassiveId::try_new(id).expect("nonzero synthetic identity"),
                dimension: Dimension::OVERWORLD,
                position,
                yaw: 0.0,
                health: 1,
            })
            .expect("valid synthetic record")
        })
        .collect()
}

/// Builds one checked passive spawn record's parts with the dimension forced
/// to the named raw value.
fn passive_spawn_record_parts(
    case: &FrozenCase,
    subject: &str,
    spawn: &RawPassiveSpawn,
    dimension: i32,
) -> Result<PassiveSpawnRecordParts, DispatchError> {
    let byte = u8::try_from(dimension)
        .map_err(|_| classification_conflict(case, subject, "dimension outside the byte range"))?;
    Ok(PassiveSpawnRecordParts {
        id: PassiveId::try_new(spawn.id)
            .map_err(|error| construction_conflict(case, subject, error))?,
        dimension: Dimension::new(byte)
            .map_err(|error| construction_conflict(case, subject, error))?,
        position: FiniteVec3::try_new(spawn.position)
            .map_err(|error| construction_conflict(case, subject, error))?,
        yaw: spawn.yaw,
        health: spawn.health,
    })
}

/// Constructs one checked passive spawn record for an admitted case.
fn construct_passive_spawn_record(
    case: &FrozenCase,
    subject: &str,
    spawn: &RawPassiveSpawn,
) -> Result<PassiveSpawnRecord, DispatchError> {
    PassiveSpawnRecord::try_new(passive_spawn_record_parts(
        case,
        subject,
        spawn,
        spawn.dimension,
    )?)
    .map_err(|error| construction_conflict(case, subject, error))
}

/// Renders one raw passive spawn batch in the normalized field map.
fn passive_spawns_raw_value(spawns: &[RawPassiveSpawn]) -> Value {
    Value::Array(
        spawns
            .iter()
            .map(|spawn| {
                serde_json::json!({
                    "id": id_value(spawn.id),
                    "dimension": spawn.dimension,
                    "position": position_value(spawn.position),
                    "yaw": normalize_f32(spawn.yaw),
                    "health": spawn.health,
                })
            })
            .collect(),
    )
}

/// Executes one `passive-state` case, whose records carry the grazing byte.
/// The grazing byte outside {0, 1} is classifier-only.
fn execute_passive_state(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "passive-state";
    let subject_rule = "passive_state";
    let server_tick = parse_server_tick(case, input)?;
    let states = parse_passive_states(case, input)?;

    if let Some(rejection) = count_rejection(subject_rule, states.len()) {
        verify_passive_state_rejection(case, subject, server_tick, &states, &rejection)?;
        return passive_state_error(case, server_tick, &states, &rejection);
    }
    for (index, state) in states.iter().enumerate() {
        if let Some(rejection) = passive_state_record_rejection(subject_rule, index, state) {
            verify_passive_state_rejection(case, subject, server_tick, &states, &rejection)?;
            return passive_state_error(case, server_tick, &states, &rejection);
        }
        if index > 0 && states[index - 1].id >= state.id {
            let rejection = order_rejection(subject_rule);
            verify_passive_state_rejection(case, subject, server_tick, &states, &rejection)?;
            return passive_state_error(case, server_tick, &states, &rejection);
        }
    }

    let mut records = Vec::with_capacity(states.len());
    for state in &states {
        records.push(construct_passive_state_record(case, subject, state)?);
    }
    let batch = PassiveState::try_new(PassiveStateParts {
        server_tick,
        states: records.into_boxed_slice(),
    })
    .map_err(|error| construction_conflict(case, subject, error))?;
    let mut fields = JsonMap::new();
    fields.insert(
        "server_tick".to_string(),
        normalize_u64(batch.server_tick()),
    );
    fields.insert(
        "states".to_string(),
        Value::Array(
            batch
                .states()
                .iter()
                .map(|record| {
                    serde_json::json!({
                        "id": id_value(record.id().get()),
                        "position": position_value(record.position().get()),
                        "velocity": position_value(record.velocity().get()),
                        "yaw": normalize_f32(record.yaw()),
                        "health": record.health(),
                        "grazing": Value::Bool(record.grazing()),
                    })
                })
                .collect(),
        ),
    );
    Ok(normalized_ok(subject, fields))
}

/// Publishes one classified passive state rejection from the raw batch.
fn passive_state_error(
    case: &FrozenCase,
    server_tick: u64,
    states: &[RawPassiveState],
    rejection: &Rejection,
) -> Result<serde_json::Value, DispatchError> {
    let mut fields = JsonMap::new();
    fields.insert("server_tick".to_string(), normalize_u64(server_tick));
    fields.insert("states".to_string(), passive_states_raw_value(states));
    normalized_error(case, rejection.category, &rejection.rule, fields)
}

/// Selects the first rule one passive state record breaks, in the Go
/// validator's order.
fn passive_state_record_rejection(
    subject: &str,
    index: usize,
    state: &RawPassiveState,
) -> Option<Rejection> {
    if state.id == 0 {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("{subject}.record_{index}.id"),
            error: Some(DomainError::InvalidIdentity),
        });
    }
    if state
        .position
        .iter()
        .any(|component| !component.is_finite())
    {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.position"),
            error: Some(DomainError::NonFiniteValue),
        });
    }
    if state
        .velocity
        .iter()
        .any(|component| !component.is_finite())
    {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.velocity"),
            error: Some(DomainError::NonFiniteValue),
        });
    }
    if !state.yaw.is_finite() {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.yaw"),
            error: Some(DomainError::NonFiniteRotation),
        });
    }
    if !health_admits(state.health) {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.health"),
            error: Some(DomainError::InvalidSurvivalValue),
        });
    }
    if state.grazing > 1 {
        return Some(Rejection::classifier(
            "invalid-enum",
            format!("{subject}.record_{index}.grazing"),
        ));
    }
    None
}

/// Verifies the constructor agreement for one classified passive state
/// rejection.
fn verify_passive_state_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    states: &[RawPassiveState],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "passive_state.count_range" => {
            if states.is_empty() {
                PassiveState::try_new(PassiveStateParts {
                    server_tick,
                    states: Vec::new().into_boxed_slice(),
                })
                .map(|_| ())
            } else {
                PassiveState::try_new(PassiveStateParts {
                    server_tick,
                    states: minimal_passive_state_records(SEMANTIC_BATCH_CAP + 1),
                })
                .map(|_| ())
            }
        }
        "passive_state.strictly_increasing_ids" => {
            let prefix = order_prefix_end(states.len(), |index| {
                states[index - 1].id >= states[index].id
            })
            .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let mut records = Vec::new();
            for state in &states[..prefix] {
                records.push(construct_passive_state_record(case, subject, state)?);
            }
            PassiveState::try_new(PassiveStateParts {
                server_tick,
                states: records.into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let index = record_index(case, subject, rule, "passive_state.record_")?;
            let state = states
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            let verdict = match rule.rsplit('.').next().unwrap_or("") {
                "id" => PassiveId::try_new(state.id).map(|_| ()),
                "position" => FiniteVec3::try_new(state.position).map(|_| ()),
                "velocity" => FiniteVec3::try_new(state.velocity).map(|_| ()),
                "yaw" | "health" => {
                    PassiveStateRecord::try_new(passive_state_record_parts(case, subject, state)?)
                        .map(|_| ())
                }
                _ => return Err(classification_conflict(case, subject, rule)),
            };
            return match verdict {
                Err(error) if error == expected => Ok(()),
                _ => Err(classification_conflict(case, subject, rule)),
            };
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Builds `count` minimal valid passive state records with strictly
/// increasing identities.
fn minimal_passive_state_records(count: usize) -> Box<[PassiveStateRecord]> {
    let vector = FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite synthetic vector");
    (1..=count as u64)
        .map(|id| {
            PassiveStateRecord::try_new(PassiveStateRecordParts {
                id: PassiveId::try_new(id).expect("nonzero synthetic identity"),
                position: vector,
                velocity: vector,
                yaw: 0.0,
                health: 1,
                grazing: false,
            })
            .expect("valid synthetic record")
        })
        .collect()
}

/// Builds one checked passive state record's parts from the raw record.
fn passive_state_record_parts(
    case: &FrozenCase,
    subject: &str,
    state: &RawPassiveState,
) -> Result<PassiveStateRecordParts, DispatchError> {
    Ok(PassiveStateRecordParts {
        id: PassiveId::try_new(state.id)
            .map_err(|error| construction_conflict(case, subject, error))?,
        position: FiniteVec3::try_new(state.position)
            .map_err(|error| construction_conflict(case, subject, error))?,
        velocity: FiniteVec3::try_new(state.velocity)
            .map_err(|error| construction_conflict(case, subject, error))?,
        yaw: state.yaw,
        health: state.health,
        grazing: state.grazing == 1,
    })
}

/// Constructs one checked passive state record for an admitted case.
fn construct_passive_state_record(
    case: &FrozenCase,
    subject: &str,
    state: &RawPassiveState,
) -> Result<PassiveStateRecord, DispatchError> {
    PassiveStateRecord::try_new(passive_state_record_parts(case, subject, state)?)
        .map_err(|error| construction_conflict(case, subject, error))
}

/// Renders one raw passive state batch in the normalized field map.
fn passive_states_raw_value(states: &[RawPassiveState]) -> Value {
    Value::Array(
        states
            .iter()
            .map(|state| {
                serde_json::json!({
                    "id": id_value(state.id),
                    "position": position_value(state.position),
                    "velocity": position_value(state.velocity),
                    "yaw": normalize_f32(state.yaw),
                    "health": state.health,
                    "grazing": grazing_value(state.grazing),
                })
            })
            .collect(),
    )
}

/// Executes one `passive-despawn` case, whose records carry identities and
/// removal reasons. The reason byte outside {0, 1} is classifier-only.
fn execute_passive_despawn(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "passive-despawn";
    let subject_rule = "passive_despawn";
    let server_tick = parse_server_tick(case, input)?;
    let despawns = parse_passive_despawns(case, input)?;

    if let Some(rejection) = count_rejection(subject_rule, despawns.len()) {
        verify_passive_despawn_rejection(case, subject, server_tick, &despawns, &rejection)?;
        return passive_despawn_error(case, server_tick, &despawns, &rejection);
    }
    for (index, despawn) in despawns.iter().enumerate() {
        if despawn.id == 0 {
            let rejection = Rejection {
                category: "invalid-identity",
                rule: format!("{subject_rule}.record_{index}.id"),
                error: Some(DomainError::InvalidIdentity),
            };
            verify_passive_despawn_rejection(case, subject, server_tick, &despawns, &rejection)?;
            return passive_despawn_error(case, server_tick, &despawns, &rejection);
        }
        if despawn.reason > 1 {
            let rejection = Rejection::classifier(
                "invalid-enum",
                format!("{subject_rule}.record_{index}.reason"),
            );
            return passive_despawn_error(case, server_tick, &despawns, &rejection);
        }
        if index > 0 && despawns[index - 1].id >= despawn.id {
            let rejection = order_rejection(subject_rule);
            verify_passive_despawn_rejection(case, subject, server_tick, &despawns, &rejection)?;
            return passive_despawn_error(case, server_tick, &despawns, &rejection);
        }
    }

    let mut records = Vec::with_capacity(despawns.len());
    for despawn in &despawns {
        records.push(PassiveDespawnRecord::new(
            PassiveId::try_new(despawn.id)
                .map_err(|error| construction_conflict(case, subject, error))?,
            match despawn.reason {
                0 => PassiveDespawnReason::Vanished,
                _ => PassiveDespawnReason::Died,
            },
        ));
    }
    let batch = PassiveDespawn::try_new(PassiveDespawnParts {
        server_tick,
        despawns: records.into_boxed_slice(),
    })
    .map_err(|error| construction_conflict(case, subject, error))?;
    let mut fields = JsonMap::new();
    fields.insert(
        "server_tick".to_string(),
        normalize_u64(batch.server_tick()),
    );
    fields.insert(
        "despawns".to_string(),
        Value::Array(
            batch
                .despawns()
                .iter()
                .map(|record| {
                    serde_json::json!({
                        "id": id_value(record.id().get()),
                        "reason": passive_reason_value(record.reason()),
                    })
                })
                .collect(),
        ),
    );
    Ok(normalized_ok(subject, fields))
}

/// Publishes one classified passive despawn rejection from the raw batch.
fn passive_despawn_error(
    case: &FrozenCase,
    server_tick: u64,
    despawns: &[RawPassiveDespawn],
    rejection: &Rejection,
) -> Result<serde_json::Value, DispatchError> {
    let mut fields = JsonMap::new();
    fields.insert("server_tick".to_string(), normalize_u64(server_tick));
    fields.insert("despawns".to_string(), passive_despawns_raw_value(despawns));
    normalized_error(case, rejection.category, &rejection.rule, fields)
}

/// Verifies the constructor agreement for one classified passive despawn
/// rejection. The reason rule is classifier-only, so it never reaches here.
fn verify_passive_despawn_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    despawns: &[RawPassiveDespawn],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "passive_despawn.count_range" => {
            if despawns.is_empty() {
                PassiveDespawn::try_new(PassiveDespawnParts {
                    server_tick,
                    despawns: Vec::new().into_boxed_slice(),
                })
                .map(|_| ())
            } else {
                let records = (1..=(SEMANTIC_BATCH_CAP + 1) as u64)
                    .map(|id| {
                        PassiveDespawnRecord::new(
                            PassiveId::try_new(id).expect("nonzero synthetic identity"),
                            PassiveDespawnReason::Vanished,
                        )
                    })
                    .collect::<Vec<_>>();
                PassiveDespawn::try_new(PassiveDespawnParts {
                    server_tick,
                    despawns: records.into_boxed_slice(),
                })
                .map(|_| ())
            }
        }
        "passive_despawn.strictly_increasing_ids" => {
            let prefix = order_prefix_end(despawns.len(), |index| {
                despawns[index - 1].id >= despawns[index].id
            })
            .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let records = despawns[..prefix]
                .iter()
                .map(|despawn| {
                    Ok(PassiveDespawnRecord::new(
                        PassiveId::try_new(despawn.id)
                            .map_err(|error| construction_conflict(case, subject, error))?,
                        match despawn.reason {
                            0 => PassiveDespawnReason::Vanished,
                            _ => PassiveDespawnReason::Died,
                        },
                    ))
                })
                .collect::<Result<Vec<_>, DispatchError>>()?;
            PassiveDespawn::try_new(PassiveDespawnParts {
                server_tick,
                despawns: records.into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let index = record_index(case, subject, rule, "passive_despawn.record_")?;
            let despawn = despawns
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            PassiveId::try_new(despawn.id).map(|_| ())
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Renders one raw passive despawn batch in the normalized field map.
fn passive_despawns_raw_value(despawns: &[RawPassiveDespawn]) -> Value {
    Value::Array(
        despawns
            .iter()
            .map(|despawn| {
                serde_json::json!({
                    "id": id_value(despawn.id),
                    "reason": despawn.reason,
                })
            })
            .collect(),
    )
}

/// Splits one indexed per-record rule into its record index.
fn record_index(
    case: &FrozenCase,
    subject: &str,
    rule: &str,
    prefix: &str,
) -> Result<usize, DispatchError> {
    let rest = rule
        .strip_prefix(prefix)
        .ok_or_else(|| classification_conflict(case, subject, rule))?;
    let index_text = rest
        .split_once('.')
        .map(|(index, _)| index)
        .ok_or_else(|| classification_conflict(case, subject, rule))?;
    index_text
        .parse()
        .map_err(|_| classification_conflict(case, subject, rule))
}

/// Returns the exclusive prefix end for the order-rule verdict: the position
/// after the first non-increasing adjacent pair, or `None` when the batch has
/// no such pair. The classifier fires the order rule at the first such pair,
/// so the constructor verdict runs on exactly the checked prefix that ends
/// there.
fn order_prefix_end<F>(len: usize, non_increasing: F) -> Option<usize>
where
    F: Fn(usize) -> bool,
{
    for index in 1..len {
        if non_increasing(index) {
            return Some(index + 1);
        }
    }
    None
}

#[test]
fn event_mobs_execute_68_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}

/// Pins the exact normalized content of the boundary cases: both hostile
/// kinds, both grazing values, both despawn reasons, the inclusive health
/// ends with the exclusive ends one step outside, the accepted zero tick,
/// and the reversed and duplicate identity orders with the submitted order
/// preserved.
#[test]
fn event_mobs_pin_seed_and_boundary_outcomes() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    let by_id = |id: &str| {
        executed
            .iter()
            .find(|case| case.case.id == id)
            .unwrap_or_else(|| panic!("missing case {id}"))
    };

    // Both published hostile kinds ride one admitted batch.
    let both_kinds = by_id("domain.event/1/hostile-spawn-both-kinds");
    assert_eq!(both_kinds.actual["kind"], "ok");
    assert_eq!(both_kinds.actual["fields"]["spawns"][0]["kind"], 0);
    assert_eq!(both_kinds.actual["fields"]["spawns"][1]["kind"], 1);

    // Both grazing values ride one admitted batch, as Booleans.
    let grazing = by_id("domain.event/1/passive-state-seed");
    assert_eq!(grazing.actual["kind"], "ok");
    assert_eq!(grazing.actual["fields"]["states"][0]["grazing"], false);
    assert_eq!(grazing.actual["fields"]["states"][1]["grazing"], true);
    // An unknown grazing byte keeps its raw number in the rejection.
    let unknown_grazing = by_id("domain.event/1/passive-state-unknown-grazing");
    assert_eq!(unknown_grazing.actual["kind"], "error");
    assert_eq!(
        unknown_grazing.actual["fields"]["rule"],
        "passive_state.record_0.grazing"
    );
    assert_eq!(unknown_grazing.actual["category"], "invalid-enum");
    assert_eq!(unknown_grazing.actual["fields"]["states"][0]["grazing"], 2);

    // Both published despawn reasons ride one admitted batch.
    let reasons = by_id("domain.event/1/passive-despawn-seed");
    assert_eq!(reasons.actual["kind"], "ok");
    assert_eq!(reasons.actual["fields"]["despawns"][0]["reason"], 0);
    assert_eq!(reasons.actual["fields"]["despawns"][1]["reason"], 1);
    let unknown_reason = by_id("domain.event/1/passive-despawn-unknown-reason");
    assert_eq!(unknown_reason.actual["kind"], "error");
    assert_eq!(
        unknown_reason.actual["fields"]["rule"],
        "passive_despawn.record_0.reason"
    );
    assert_eq!(unknown_reason.actual["category"], "invalid-enum");

    // The inclusive health ends are admitted, the exclusive ends reject with
    // the record rule.
    for (id, expected) in [
        ("hostile-spawn-health-one", 1),
        ("hostile-spawn-health-twenty", 20),
        ("hostile-state-health-one", 1),
        ("hostile-state-health-twenty", 20),
        ("passive-spawn-health-one", 1),
        ("passive-spawn-health-twenty", 20),
        ("passive-state-health-one", 1),
        ("passive-state-health-twenty", 20),
    ] {
        let admitted = by_id(&format!("domain.event/1/{id}"));
        assert_eq!(admitted.actual["kind"], "ok", "{id}");
        let array = if id.contains("spawn") && !id.contains("despawn") {
            "spawns"
        } else {
            "states"
        };
        assert_eq!(
            admitted.actual["fields"][array][0]["health"], expected,
            "{id}"
        );
    }
    for (id, subject) in [
        ("hostile-spawn-health-zero", "hostile_spawn"),
        ("hostile-spawn-health-twenty-one", "hostile_spawn"),
        ("hostile-state-health-zero", "hostile_state"),
        ("hostile-state-health-twenty-one", "hostile_state"),
        ("passive-spawn-health-zero", "passive_spawn"),
        ("passive-spawn-health-twenty-one", "passive_spawn"),
        ("passive-state-health-zero", "passive_state"),
        ("passive-state-health-twenty-one", "passive_state"),
    ] {
        let rejected = by_id(&format!("domain.event/1/{id}"));
        assert_eq!(rejected.actual["kind"], "error", "{id}");
        assert_eq!(
            rejected.actual["fields"]["rule"],
            format!("{subject}.record_0.health"),
            "{id}"
        );
        assert_eq!(rejected.actual["category"], "invalid-value", "{id}");
    }

    // The zero tick is an admitted publish instant for every rule.
    for id in [
        "hostile-spawn-zero-tick",
        "hostile-state-zero-tick",
        "hostile-despawn-zero-tick",
        "passive-spawn-zero-tick",
        "passive-state-zero-tick",
        "passive-despawn-zero-tick",
    ] {
        let zero = by_id(&format!("domain.event/1/{id}"));
        assert_eq!(zero.actual["kind"], "ok", "{id}");
        assert_eq!(zero.actual["fields"]["server_tick"], "0", "{id}");
    }

    // The reversed and duplicate orders reject with the shared rule, and the
    // rejection publishes the records in the submitted order: the reversed
    // batch leads with the identity that was submitted first.
    for (id, subject) in [
        ("hostile-spawn-reversed", "hostile_spawn"),
        ("hostile-spawn-duplicate", "hostile_spawn"),
        ("hostile-state-reversed", "hostile_state"),
        ("hostile-state-duplicate", "hostile_state"),
        ("hostile-despawn-reversed", "hostile_despawn"),
        ("hostile-despawn-duplicate", "hostile_despawn"),
        ("passive-spawn-reversed", "passive_spawn"),
        ("passive-spawn-duplicate", "passive_spawn"),
        ("passive-state-reversed", "passive_state"),
        ("passive-state-duplicate", "passive_state"),
        ("passive-despawn-reversed", "passive_despawn"),
        ("passive-despawn-duplicate", "passive_despawn"),
    ] {
        let rejected = by_id(&format!("domain.event/1/{id}"));
        assert_eq!(rejected.actual["kind"], "error", "{id}");
        assert_eq!(
            rejected.actual["fields"]["rule"],
            format!("{subject}.strictly_increasing_ids"),
            "{id}"
        );
    }
    let reversed = by_id("domain.event/1/hostile-spawn-reversed");
    assert_eq!(reversed.actual["fields"]["spawns"][0]["id"], "2");
    assert_eq!(reversed.actual["fields"]["spawns"][1]["id"], "1");
    let reversed_ids = by_id("domain.event/1/hostile-despawn-reversed");
    assert_eq!(reversed_ids.actual["fields"]["ids"][0], "2");
    assert_eq!(reversed_ids.actual["fields"]["ids"][1], "1");
}

/// Proves the frozen precedence with direct raw probes: a raw batch count
/// above the semantic cap classifies as the count rule even when its first
/// record is also invalid, and the adapter never sorts a submitted batch
/// before publishing it.
#[test]
fn event_mobs_raw_probe_count_overflow_and_no_sorting() {
    let overflow = probe_case(
        "domain.event/1/probe-hostile-spawn-overflow",
        serde_json::json!({
            "consumer": "mornlea_domain",
            "rule": "hostile-spawn",
            "server_tick": "1",
            "spawns": (0..(MAX_SEMANTIC_BATCH_RECORDS + 1))
                .map(|index| {
                    let id = if index == 0 { 0u64 } else { index as u64 };
                    serde_json::json!({
                        "id": id.to_string(),
                        "dimension": 0,
                        "position": ["0", "0", "0"],
                        "yaw": "0",
                        "health": 1,
                        "kind": 0,
                    })
                })
                .collect::<Vec<_>>(),
            "states": [],
            "ids": [],
            "despawns": [],
        }),
    );
    let outcome = execute(&overflow).expect("execute overflow probe");
    assert_eq!(outcome["kind"], "error");
    assert_eq!(outcome["category"], "invalid-value");
    assert_eq!(outcome["fields"]["rule"], "hostile_spawn.count_range");

    let overflow_ids = probe_case(
        "domain.event/1/probe-hostile-despawn-overflow",
        serde_json::json!({
            "consumer": "mornlea_domain",
            "rule": "hostile-despawn",
            "server_tick": "1",
            "spawns": [],
            "states": [],
            "ids": (0..(MAX_SEMANTIC_BATCH_RECORDS + 1))
                .map(|index| {
                    let id = if index == 0 { 0u64 } else { index as u64 };
                    id.to_string()
                })
                .collect::<Vec<_>>(),
            "despawns": [],
        }),
    );
    let outcome = execute(&overflow_ids).expect("execute id overflow probe");
    assert_eq!(outcome["fields"]["rule"], "hostile_despawn.count_range");

    // An accepted reversed-shape batch cannot exist, so the no-sorting proof
    // uses the rejection projection: the submitted first record stays first.
    let reversed = probe_case(
        "domain.event/1/probe-passive-state-reversed",
        serde_json::json!({
            "consumer": "mornlea_domain",
            "rule": "passive-state",
            "server_tick": "5",
            "spawns": [],
            "states": [
                {
                    "id": "9",
                    "position": ["1", "2", "3"],
                    "velocity": ["0", "0", "0"],
                    "yaw": "0.5",
                    "health": 3,
                    "grazing": 1,
                },
                {
                    "id": "4",
                    "position": ["1", "2", "3"],
                    "velocity": ["0", "0", "0"],
                    "yaw": "0.5",
                    "health": 3,
                    "grazing": 0,
                },
            ],
            "ids": [],
            "despawns": [],
        }),
    );
    let outcome = execute(&reversed).expect("execute reversed probe");
    assert_eq!(outcome["kind"], "error");
    assert_eq!(
        outcome["fields"]["rule"],
        "passive_state.strictly_increasing_ids"
    );
    assert_eq!(outcome["fields"]["states"][0]["id"], "9");
    assert_eq!(outcome["fields"]["states"][1]["id"], "4");
}

/// A raw unknown spawn kind is a classifier-only rejection. This probe
/// checks that branch without changing the approved 68-case corpus table.
#[test]
fn event_mobs_rejects_unknown_hostile_spawn_kind() {
    let case = probe_case(
        "domain.event/1/probe-hostile-spawn-unknown-kind",
        serde_json::json!({
            "consumer": "mornlea_domain",
            "rule": "hostile-spawn",
            "server_tick": "1",
            "spawns": [{
                "id": "1",
                "dimension": 0,
                "position": ["0", "0", "0"],
                "yaw": "0",
                "health": 10,
                "kind": 2,
            }],
            "states": [],
            "ids": [],
            "despawns": [],
        }),
    );
    let outcome = execute(&case).expect("execute unknown hostile spawn kind probe");
    assert_eq!(outcome["kind"], "error");
    assert_eq!(outcome["category"], "invalid-enum");
    assert_eq!(outcome["fields"]["rule"], "hostile_spawn.record_0.kind");
    assert_eq!(outcome["fields"]["spawns"][0]["kind"], 2);
}

/// Builds one synthetic case around a raw input object, the shape the raw
/// probes execute. The frozen `normalized` value stays null because the
/// probes compare the executed outcome directly rather than through the
/// frozen comparator.
fn probe_case(id: &str, input: serde_json::Value) -> FrozenCase {
    FrozenCase {
        id: id.to_string(),
        family: "domain.event".to_string(),
        version: "1".to_string(),
        consumer: CorpusConsumer::Domain,
        packet_key: None,
        operation: "admit".to_string(),
        arguments: serde_json::Value::Null,
        input_format: crate::runtime_corpus::InputFormat::Json,
        input: Vec::new(),
        input_json: Some(input),
        normalized: serde_json::Value::Null,
        encoded: None,
        category: String::new(),
    }
}
