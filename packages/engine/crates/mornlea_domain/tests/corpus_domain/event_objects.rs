//! Topic module for domain.event projectile and item-drop corpus cases.
//!
//! The five rules here are the transient-object observations an
//! authoritative session publishes to a subscribed client: a spawn, state
//! and despawn triple for the projectiles in flight, and an upserts and a
//! removes batch for the dropped item stacks. Each executor parses the case
//! input, selects the Go rejection rule from the raw values in the
//! producer's stated precedence, and then proves the applicable Rust
//! constructor agrees: a classified acceptance must construct, and a
//! classified rejection must fail with exactly the mapped `DomainError`.
//! Accepted fields are read from the constructed Rust values; a rejected
//! value has no getters, so its projection is built from the parsed input
//! instead.
//!
//! Two raw byte domains stay classifier-only because no closed Rust type
//! can carry their unknown values: a projectile kind byte outside {0, 1}
//! and a dimension byte outside the two playable ones. The adapter still
//! emits the exact Go category and rule for them, but no constructor
//! verdict exists to check, and no `Unknown` enum variant is invented. The
//! 128-record projectile and 32-record drop packet maxima are transport
//! budgets no frozen case exercises; the semantic walk below bounds work
//! with the shared 4,096 cap first, exactly as the batch constructors do.
//!
//! A drop's raw dimension stays deliberately unvalidated — the Go
//! `DropID.Valid` rule checks only the slot range and the generation — so a
//! raw dimension of `-1` constructs and is published back losslessly, and
//! the strict batch order compares the full identity key (dimension, chunk
//! column, slot, generation) with the raw dimension first.
//!
//! Submitted record order is never sorted: the classifier replays each
//! record in input order and checks the strictly-increasing identity rule
//! against the previous record after each otherwise valid one, and both the
//! accepted and the rejected projection publish the batch exactly as it was
//! submitted.

use super::support::{
    DispatchError, JsonMap, assert_domain_normalized, exact_array, execute_topic, input_object,
    invalid_case, normalize_f32, normalize_u64, normalized_error, normalized_ok, parse_f32_token,
    required_array, required_i32, required_object, required_string, required_u8, required_u16,
    required_u32, value_i32, value_object, value_string,
};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};
use mornlea_domain::{
    ChunkPos, Dimension, DomainError, DropId, FiniteVec3, ItemDrop, ItemDropParts, ItemDropRemoves,
    ItemDropRemovesParts, ItemDropUpserts, ItemDropUpsertsParts, ItemStack,
    MAX_SEMANTIC_BATCH_RECORDS, ProjectileDespawn, ProjectileDespawnParts, ProjectileId,
    ProjectileKind, ProjectileSpawn, ProjectileSpawnParts, ProjectileSpawnRecord,
    ProjectileSpawnRecordParts, ProjectileState, ProjectileStateParts, ProjectileStateRecord,
    ProjectileStateRecordParts, durability_max, item_stack_limit,
};
use serde_json::Value;

pub const EXPECTED_COUNT: usize = 45;

const RULES: &[&str] = &[
    "projectile-spawn",
    "projectile-state",
    "projectile-despawn",
    "item-drop-upserts",
    "item-drop-removes",
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
        "projectile-spawn" => execute_projectile_spawn(case, input),
        "projectile-state" => execute_projectile_state(case, input),
        "projectile-despawn" => execute_projectile_despawn(case, input),
        "item-drop-upserts" => execute_item_drop_upserts(case, input),
        "item-drop-removes" => execute_item_drop_removes(case, input),
        unknown => Err(invalid_case(
            case,
            format!("unknown objects event rule '{unknown}'"),
        )),
    }
}

/// One classified rejection: its normalized category, its exact Go rule name,
/// and the `DomainError` the owning constructor must return.
///
/// The rule is a `String` because the per-record rules are indexed by record
/// (`projectile_spawn.record_0.kind`). `error` is `None` for a
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

/// The shared semantic batch cap the domain constructors bound work with.
const SEMANTIC_BATCH_CAP: usize = MAX_SEMANTIC_BATCH_RECORDS;

/// Per-chunk drop slot count, from the Go `core.DropsPerChunk`: a drop slot
/// at or above it names no slot at all.
const DROPS_PER_CHUNK: u8 = 32;

/// Exclusive upper bound of a chunk-local block index, matching the private
/// domain constant behind `ItemDrop::try_new`'s `InvalidBlockIndex`.
const MAX_CHUNK_BLOCK_INDEX: u32 = 98_304;

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

/// Parses one exact XYZ float-token vector, preserving component order.
fn parse_vec3(case: &FrozenCase, object: &JsonMap, key: &str) -> Result<[f32; 3], DispatchError> {
    let values = required_array(case, object, key)?;
    let slots = exact_array::<3>(case, key, values)?;
    let mut vector = [0.0f32; 3];
    for (index, value) in slots.iter().enumerate() {
        let path = format!("{key}[{index}]");
        vector[index] = parse_f32_token(case, &path, value_string(case, &path, value)?)?;
    }
    Ok(vector)
}

/// Renders one XYZ vector in the normalized bit-exact form. A non-finite
/// component renders its exact bits, so a rejection shows the exact value
/// the record carried rather than a rounded decimal.
fn vector_value(vector: [f32; 3]) -> Value {
    Value::Array(
        vector
            .iter()
            .map(|component| normalize_f32(*component))
            .collect(),
    )
}

/// Renders one identity in the normalized decimal-string form.
fn id_value(id: u64) -> Value {
    normalize_u64(id)
}

/// Renders one projectile kind through its exhaustive match, so adding a
/// variant is a compile error here rather than a silent renumbering.
fn projectile_kind_value(kind: ProjectileKind) -> Value {
    match kind {
        ProjectileKind::Shard => Value::from(0),
        ProjectileKind::Arrow => Value::from(1),
    }
}

/// The batch-level count rejection shared by all five rules: a raw count
/// above the semantic cap classifies first, then an empty batch.
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

/// The strictly-increasing identity rejection shared by all five rules.
fn order_rejection(subject: &str) -> Rejection {
    Rejection {
        category: "invalid-value",
        rule: format!("{subject}.strictly_increasing_ids"),
        error: Some(DomainError::InvalidStateOrder),
    }
}

/// Admits one raw dimension for a projectile spawn record: both playable
/// dimensions pass, any other fitting byte rejects with the record
/// constructor's `InvalidDimension`, and a value outside the byte range
/// cannot enter any Rust type, so only the classifier decides.
fn admit_dimension(raw: i32) -> Result<Dimension, Option<DomainError>> {
    if let Ok(byte) = u8::try_from(raw) {
        return Dimension::new(byte).map_err(Some);
    }
    Err(None)
}

/// The parsed raw shape of one projectile spawn record.
struct RawProjectileSpawn {
    id: u64,
    kind: u8,
    dimension: i32,
    position: [f32; 3],
    velocity: [f32; 3],
}

/// The parsed raw shape of one projectile state record.
struct RawProjectileState {
    id: u64,
    position: [f32; 3],
}

/// The parsed raw shape of one drop identity: the raw i32 dimension stays
/// untouched because the Go `DropID.Valid` rule leaves it unvalidated.
struct RawDropId {
    dimension: i32,
    chunk: [i32; 2],
    slot: u8,
    generation: u32,
}

/// The parsed raw shape of one carried stack.
struct RawStack {
    item: u16,
    count: u8,
    durability: u16,
}

/// The parsed raw shape of one item-drop upsert record.
struct RawDrop {
    id: RawDropId,
    block_index: u32,
    stack: RawStack,
}

/// The full `DropId` ordering key the strict batch order compares: the raw
/// dimension first, then the chunk column, the slot and the generation.
fn raw_drop_id_less(a: &RawDropId, b: &RawDropId) -> bool {
    (a.dimension, a.chunk[0], a.chunk[1], a.slot, a.generation)
        < (b.dimension, b.chunk[0], b.chunk[1], b.slot, b.generation)
}

/// Parses one required ordered batch of projectile spawn records.
fn parse_projectile_spawns(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<Vec<RawProjectileSpawn>, DispatchError> {
    required_array(case, input, "spawns")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("spawns[{index}]");
            let object = value_object(case, &path, value)?;
            Ok(RawProjectileSpawn {
                id: parse_decimal_u64(
                    case,
                    &format!("{path}.id"),
                    required_string(case, object, "id")?,
                )?,
                kind: required_u8(case, object, "kind")?,
                dimension: required_i32(case, object, "dimension")?,
                position: parse_vec3(case, object, "position")?,
                velocity: parse_vec3(case, object, "velocity")?,
            })
        })
        .collect()
}

/// Parses one required ordered batch of projectile state records.
fn parse_projectile_states(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<Vec<RawProjectileState>, DispatchError> {
    required_array(case, input, "states")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("states[{index}]");
            let object = value_object(case, &path, value)?;
            Ok(RawProjectileState {
                id: parse_decimal_u64(
                    case,
                    &format!("{path}.id"),
                    required_string(case, object, "id")?,
                )?,
                position: parse_vec3(case, object, "position")?,
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

/// Parses one raw drop identity from its frozen object rendering.
fn parse_drop_id(case: &FrozenCase, path: &str, value: &Value) -> Result<RawDropId, DispatchError> {
    let object = value_object(case, path, value)?;
    let chunk_values = required_array(case, object, "chunk")?;
    let chunk_slots = exact_array::<2>(case, &format!("{path}.chunk"), chunk_values)?;
    Ok(RawDropId {
        dimension: required_i32(case, object, "dimension")?,
        chunk: [
            value_i32(case, &format!("{path}.chunk[0]"), &chunk_slots[0])?,
            value_i32(case, &format!("{path}.chunk[1]"), &chunk_slots[1])?,
        ],
        slot: required_u8(case, object, "slot")?,
        generation: required_u32(case, object, "generation")?,
    })
}

/// Parses one raw stack from its frozen object rendering.
fn parse_stack(case: &FrozenCase, path: &str, object: &JsonMap) -> Result<RawStack, DispatchError> {
    Ok(RawStack {
        item: required_u16(case, object, "item")?,
        count: required_u8(case, object, "count")?,
        durability: required_u16(case, object, "durability")?,
    })
    .map_err(|error| repath(error, path))
}

/// Rewrites one helper error's field path to carry its record prefix, so a
/// malformed stack field names the drop it belongs to.
fn repath(error: DispatchError, prefix: &str) -> DispatchError {
    match error {
        DispatchError::InvalidCase { case_id, message } => DispatchError::InvalidCase {
            case_id,
            message: format!("{prefix}: {message}"),
        },
        other => other,
    }
}

/// Parses one required ordered batch of item-drop upsert records.
fn parse_drops(case: &FrozenCase, input: &JsonMap) -> Result<Vec<RawDrop>, DispatchError> {
    required_array(case, input, "drops")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("drops[{index}]");
            let object = value_object(case, &path, value)?;
            Ok(RawDrop {
                id: parse_drop_id(
                    case,
                    &format!("{path}.id"),
                    object.get("id").ok_or_else(|| {
                        invalid_case(case, format!("missing required field '{path}.id'"))
                    })?,
                )?,
                block_index: required_u32(case, object, "block_index")?,
                stack: parse_stack(case, &path, required_object(case, object, "stack")?)?,
            })
        })
        .collect()
}

/// Parses one required ordered batch of raw drop identities.
fn parse_drop_ids(case: &FrozenCase, input: &JsonMap) -> Result<Vec<RawDropId>, DispatchError> {
    required_array(case, input, "ids")?
        .iter()
        .enumerate()
        .map(|(index, value)| parse_drop_id(case, &format!("ids[{index}]"), value))
        .collect()
}

/// Executes one `projectile-spawn` case.
///
/// The walk replays the Go validator exactly: the raw count bound, then each
/// record's identity, kind domain, playable dimension, finite pose and finite
/// velocity in submitted order, then the strictly-increasing identity rule
/// after each otherwise valid record. The kind byte outside {0, 1} is
/// classifier-only.
fn execute_projectile_spawn(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "projectile-spawn";
    let subject_rule = "projectile_spawn";
    let server_tick = parse_server_tick(case, input)?;
    let spawns = parse_projectile_spawns(case, input)?;

    if let Some(rejection) = count_rejection(subject_rule, spawns.len()) {
        verify_projectile_spawn_rejection(case, subject, server_tick, &spawns, &rejection)?;
        return projectile_spawn_error(case, server_tick, &spawns, &rejection);
    }
    for (index, spawn) in spawns.iter().enumerate() {
        if let Some(rejection) = projectile_spawn_record_rejection(subject_rule, index, spawn) {
            verify_projectile_spawn_rejection(case, subject, server_tick, &spawns, &rejection)?;
            return projectile_spawn_error(case, server_tick, &spawns, &rejection);
        }
        if index > 0 && spawns[index - 1].id >= spawn.id {
            let rejection = order_rejection(subject_rule);
            verify_projectile_spawn_rejection(case, subject, server_tick, &spawns, &rejection)?;
            return projectile_spawn_error(case, server_tick, &spawns, &rejection);
        }
    }

    let mut records = Vec::with_capacity(spawns.len());
    for spawn in &spawns {
        records.push(construct_projectile_spawn_record(case, subject, spawn)?);
    }
    let batch = ProjectileSpawn::try_new(ProjectileSpawnParts {
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
                        "kind": projectile_kind_value(record.kind()),
                        "dimension": record.dimension().get(),
                        "position": vector_value(record.position().get()),
                        "velocity": vector_value(record.velocity().get()),
                    })
                })
                .collect(),
        ),
    );
    Ok(normalized_ok(subject, fields))
}

/// Publishes one classified projectile spawn rejection from the raw batch.
fn projectile_spawn_error(
    case: &FrozenCase,
    server_tick: u64,
    spawns: &[RawProjectileSpawn],
    rejection: &Rejection,
) -> Result<serde_json::Value, DispatchError> {
    let mut fields = JsonMap::new();
    fields.insert("server_tick".to_string(), normalize_u64(server_tick));
    fields.insert("spawns".to_string(), projectile_spawns_raw_value(spawns));
    normalized_error(case, rejection.category, &rejection.rule, fields)
}

/// Selects the first rule one projectile spawn record breaks, in the Go
/// validator's order. The kind byte outside {0, 1} is classifier-only.
fn projectile_spawn_record_rejection(
    subject: &str,
    index: usize,
    spawn: &RawProjectileSpawn,
) -> Option<Rejection> {
    if spawn.id == 0 {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("{subject}.record_{index}.id"),
            error: Some(DomainError::InvalidIdentity),
        });
    }
    if spawn.kind > 1 {
        return Some(Rejection::classifier(
            "invalid-enum",
            format!("{subject}.record_{index}.kind"),
        ));
    }
    if let Err(error) = admit_dimension(spawn.dimension) {
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
    if spawn.position.iter().any(|c| !c.is_finite()) {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.position"),
            error: Some(DomainError::NonFiniteValue),
        });
    }
    if spawn.velocity.iter().any(|c| !c.is_finite()) {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.velocity"),
            error: Some(DomainError::NonFiniteValue),
        });
    }
    None
}

/// Verifies the constructor agreement for one classified projectile spawn
/// rejection.
fn verify_projectile_spawn_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    spawns: &[RawProjectileSpawn],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "projectile_spawn.count_range" => {
            if spawns.is_empty() {
                ProjectileSpawn::try_new(ProjectileSpawnParts {
                    server_tick,
                    spawns: Vec::new().into_boxed_slice(),
                })
                .map(|_| ())
            } else {
                ProjectileSpawn::try_new(ProjectileSpawnParts {
                    server_tick,
                    spawns: minimal_projectile_spawn_records(SEMANTIC_BATCH_CAP + 1),
                })
                .map(|_| ())
            }
        }
        "projectile_spawn.strictly_increasing_ids" => {
            let prefix = order_prefix_end(spawns.len(), |index| {
                spawns[index - 1].id >= spawns[index].id
            })
            .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let mut records = Vec::new();
            for spawn in &spawns[..prefix] {
                records.push(construct_projectile_spawn_record(case, subject, spawn)?);
            }
            ProjectileSpawn::try_new(ProjectileSpawnParts {
                server_tick,
                spawns: records.into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let index = record_index(case, subject, rule, "projectile_spawn.record_")?;
            let spawn = spawns
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            let verdict = match rule.rsplit('.').next().unwrap_or("") {
                "id" => ProjectileId::try_new(spawn.id).map(|_| ()),
                "dimension" => match admit_dimension(spawn.dimension) {
                    Ok(_) => Ok(()),
                    Err(Some(error)) => Err(error),
                    Err(None) => {
                        return Err(classification_conflict(case, subject, rule));
                    }
                },
                "position" => FiniteVec3::try_new(spawn.position).map(|_| ()),
                "velocity" => FiniteVec3::try_new(spawn.velocity).map(|_| ()),
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

/// Builds `count` minimal valid projectile spawn records with strictly
/// increasing identities, the synthetic payload the count-overflow verdict
/// runs on. Both kinds are legal in both dimensions, so one kind suffices.
fn minimal_projectile_spawn_records(count: usize) -> Box<[ProjectileSpawnRecord]> {
    let vector = FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite synthetic vector");
    (1..=count as u64)
        .map(|id| {
            ProjectileSpawnRecord::new(ProjectileSpawnRecordParts {
                id: ProjectileId::try_new(id).expect("nonzero synthetic identity"),
                kind: ProjectileKind::Shard,
                dimension: Dimension::OVERWORLD,
                position: vector,
                velocity: vector,
            })
        })
        .collect()
}

/// Builds one checked projectile spawn record's parts from the raw record.
/// The kind and dimension bytes the classifier already cleared must
/// construct here; a failure names the contract conflict.
fn projectile_spawn_record_parts(
    case: &FrozenCase,
    subject: &str,
    spawn: &RawProjectileSpawn,
) -> Result<ProjectileSpawnRecordParts, DispatchError> {
    Ok(ProjectileSpawnRecordParts {
        id: ProjectileId::try_new(spawn.id)
            .map_err(|error| construction_conflict(case, subject, error))?,
        kind: match spawn.kind {
            0 => ProjectileKind::Shard,
            1 => ProjectileKind::Arrow,
            raw => {
                return Err(classification_conflict(
                    case,
                    subject,
                    &format!("kind byte {raw} has no Rust variant"),
                ));
            }
        },
        dimension: match admit_dimension(spawn.dimension) {
            Ok(dimension) => dimension,
            Err(Some(error)) => return Err(construction_conflict(case, subject, error)),
            Err(None) => {
                return Err(classification_conflict(
                    case,
                    subject,
                    "dimension outside the byte range",
                ));
            }
        },
        position: FiniteVec3::try_new(spawn.position)
            .map_err(|error| construction_conflict(case, subject, error))?,
        velocity: FiniteVec3::try_new(spawn.velocity)
            .map_err(|error| construction_conflict(case, subject, error))?,
    })
}

/// Constructs one checked projectile spawn record for an admitted case.
fn construct_projectile_spawn_record(
    case: &FrozenCase,
    subject: &str,
    spawn: &RawProjectileSpawn,
) -> Result<ProjectileSpawnRecord, DispatchError> {
    Ok(ProjectileSpawnRecord::new(projectile_spawn_record_parts(
        case, subject, spawn,
    )?))
}

/// Renders one raw projectile spawn batch in the normalized field map.
fn projectile_spawns_raw_value(spawns: &[RawProjectileSpawn]) -> Value {
    Value::Array(
        spawns
            .iter()
            .map(|spawn| {
                serde_json::json!({
                    "id": id_value(spawn.id),
                    "kind": spawn.kind,
                    "dimension": spawn.dimension,
                    "position": vector_value(spawn.position),
                    "velocity": vector_value(spawn.velocity),
                })
            })
            .collect(),
    )
}

/// Executes one `projectile-state` case, whose records carry no kind,
/// dimension or velocity because all three are fixed at spawn and the mirror
/// replays them from there.
fn execute_projectile_state(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "projectile-state";
    let subject_rule = "projectile_state";
    let server_tick = parse_server_tick(case, input)?;
    let states = parse_projectile_states(case, input)?;

    if let Some(rejection) = count_rejection(subject_rule, states.len()) {
        verify_projectile_state_rejection(case, subject, server_tick, &states, &rejection)?;
        return projectile_state_error(case, server_tick, &states, &rejection);
    }
    for (index, state) in states.iter().enumerate() {
        if let Some(rejection) = projectile_state_record_rejection(subject_rule, index, state) {
            verify_projectile_state_rejection(case, subject, server_tick, &states, &rejection)?;
            return projectile_state_error(case, server_tick, &states, &rejection);
        }
        if index > 0 && states[index - 1].id >= state.id {
            let rejection = order_rejection(subject_rule);
            verify_projectile_state_rejection(case, subject, server_tick, &states, &rejection)?;
            return projectile_state_error(case, server_tick, &states, &rejection);
        }
    }

    let mut records = Vec::with_capacity(states.len());
    for state in &states {
        records.push(ProjectileStateRecord::new(ProjectileStateRecordParts {
            id: ProjectileId::try_new(state.id)
                .map_err(|error| construction_conflict(case, subject, error))?,
            position: FiniteVec3::try_new(state.position)
                .map_err(|error| construction_conflict(case, subject, error))?,
        }));
    }
    let batch = ProjectileState::try_new(ProjectileStateParts {
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
                        "position": vector_value(record.position().get()),
                    })
                })
                .collect(),
        ),
    );
    Ok(normalized_ok(subject, fields))
}

/// Publishes one classified projectile state rejection from the raw batch.
fn projectile_state_error(
    case: &FrozenCase,
    server_tick: u64,
    states: &[RawProjectileState],
    rejection: &Rejection,
) -> Result<serde_json::Value, DispatchError> {
    let mut fields = JsonMap::new();
    fields.insert("server_tick".to_string(), normalize_u64(server_tick));
    fields.insert("states".to_string(), projectile_states_raw_value(states));
    normalized_error(case, rejection.category, &rejection.rule, fields)
}

/// Selects the first rule one projectile state record breaks, in the Go
/// validator's order: the identity, then the finite pose.
fn projectile_state_record_rejection(
    subject: &str,
    index: usize,
    state: &RawProjectileState,
) -> Option<Rejection> {
    if state.id == 0 {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("{subject}.record_{index}.id"),
            error: Some(DomainError::InvalidIdentity),
        });
    }
    if state.position.iter().any(|c| !c.is_finite()) {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.record_{index}.position"),
            error: Some(DomainError::NonFiniteValue),
        });
    }
    None
}

/// Verifies the constructor agreement for one classified projectile state
/// rejection.
fn verify_projectile_state_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    states: &[RawProjectileState],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "projectile_state.count_range" => {
            if states.is_empty() {
                ProjectileState::try_new(ProjectileStateParts {
                    server_tick,
                    states: Vec::new().into_boxed_slice(),
                })
                .map(|_| ())
            } else {
                ProjectileState::try_new(ProjectileStateParts {
                    server_tick,
                    states: minimal_projectile_state_records(SEMANTIC_BATCH_CAP + 1),
                })
                .map(|_| ())
            }
        }
        "projectile_state.strictly_increasing_ids" => {
            let prefix = order_prefix_end(states.len(), |index| {
                states[index - 1].id >= states[index].id
            })
            .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let mut records = Vec::new();
            for state in &states[..prefix] {
                records.push(ProjectileStateRecord::new(ProjectileStateRecordParts {
                    id: ProjectileId::try_new(state.id)
                        .map_err(|error| construction_conflict(case, subject, error))?,
                    position: FiniteVec3::try_new(state.position)
                        .map_err(|error| construction_conflict(case, subject, error))?,
                }));
            }
            ProjectileState::try_new(ProjectileStateParts {
                server_tick,
                states: records.into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let index = record_index(case, subject, rule, "projectile_state.record_")?;
            let state = states
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            let verdict = match rule.rsplit('.').next().unwrap_or("") {
                "id" => ProjectileId::try_new(state.id).map(|_| ()),
                "position" => FiniteVec3::try_new(state.position).map(|_| ()),
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

/// Builds `count` minimal valid projectile state records with strictly
/// increasing identities.
fn minimal_projectile_state_records(count: usize) -> Box<[ProjectileStateRecord]> {
    let vector = FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite synthetic vector");
    (1..=count as u64)
        .map(|id| {
            ProjectileStateRecord::new(ProjectileStateRecordParts {
                id: ProjectileId::try_new(id).expect("nonzero synthetic identity"),
                position: vector,
            })
        })
        .collect()
}

/// Renders one raw projectile state batch in the normalized field map.
fn projectile_states_raw_value(states: &[RawProjectileState]) -> Value {
    Value::Array(
        states
            .iter()
            .map(|state| {
                serde_json::json!({
                    "id": id_value(state.id),
                    "position": vector_value(state.position),
                })
            })
            .collect(),
    )
}

/// Executes one `projectile-despawn` case, whose records are bare decimal
/// identities.
fn execute_projectile_despawn(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "projectile-despawn";
    let subject_rule = "projectile_despawn";
    let server_tick = parse_server_tick(case, input)?;
    let ids = parse_ids(case, input)?;

    if let Some(rejection) = count_rejection(subject_rule, ids.len()) {
        verify_projectile_despawn_rejection(case, subject, server_tick, &ids, &rejection)?;
        return projectile_despawn_error(case, server_tick, &ids, &rejection);
    }
    for (index, id) in ids.iter().enumerate() {
        if *id == 0 {
            let rejection = Rejection {
                category: "invalid-identity",
                rule: format!("{subject_rule}.record_{index}.id"),
                error: Some(DomainError::InvalidIdentity),
            };
            verify_projectile_despawn_rejection(case, subject, server_tick, &ids, &rejection)?;
            return projectile_despawn_error(case, server_tick, &ids, &rejection);
        }
        if index > 0 && ids[index - 1] >= *id {
            let rejection = order_rejection(subject_rule);
            verify_projectile_despawn_rejection(case, subject, server_tick, &ids, &rejection)?;
            return projectile_despawn_error(case, server_tick, &ids, &rejection);
        }
    }

    let typed: Vec<ProjectileId> = ids
        .iter()
        .map(|id| {
            ProjectileId::try_new(*id).map_err(|error| construction_conflict(case, subject, error))
        })
        .collect::<Result<_, _>>()?;
    let batch = ProjectileDespawn::try_new(ProjectileDespawnParts {
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

/// Publishes one classified projectile despawn rejection from the raw batch.
fn projectile_despawn_error(
    case: &FrozenCase,
    server_tick: u64,
    ids: &[u64],
    rejection: &Rejection,
) -> Result<serde_json::Value, DispatchError> {
    let mut fields = JsonMap::new();
    fields.insert("server_tick".to_string(), normalize_u64(server_tick));
    fields.insert(
        "ids".to_string(),
        Value::Array(ids.iter().map(|id| id_value(*id)).collect()),
    );
    normalized_error(case, rejection.category, &rejection.rule, fields)
}

/// Verifies the constructor agreement for one classified projectile despawn
/// rejection.
fn verify_projectile_despawn_rejection(
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
        "projectile_despawn.count_range" => {
            if ids.is_empty() {
                ProjectileDespawn::try_new(ProjectileDespawnParts {
                    server_tick,
                    ids: Vec::new().into_boxed_slice(),
                })
                .map(|_| ())
            } else {
                ProjectileDespawn::try_new(ProjectileDespawnParts {
                    server_tick,
                    ids: (1..=(SEMANTIC_BATCH_CAP + 1) as u64)
                        .map(|id| ProjectileId::try_new(id).expect("nonzero synthetic identity"))
                        .collect::<Vec<_>>()
                        .into_boxed_slice(),
                })
                .map(|_| ())
            }
        }
        "projectile_despawn.strictly_increasing_ids" => {
            let prefix = order_prefix_end(ids.len(), |index| ids[index - 1] >= ids[index])
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            ProjectileDespawn::try_new(ProjectileDespawnParts {
                server_tick,
                ids: ids[..prefix]
                    .iter()
                    .map(|id| {
                        ProjectileId::try_new(*id)
                            .map_err(|error| construction_conflict(case, subject, error))
                    })
                    .collect::<Result<Vec<_>, _>>()?
                    .into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let index = record_index(case, subject, rule, "projectile_despawn.record_")?;
            let id = ids
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            ProjectileId::try_new(*id).map(|_| ())
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Executes one `item-drop-upserts` case.
///
/// The walk replays the Go validator exactly: the raw count bound, then each
/// record's identity slot range, identity generation, block index and
/// carried stack in submitted order, then the strictly-increasing full
/// `DropId` order after each otherwise valid record. The raw dimension is
/// deliberately not named because the Go `DropID.Valid` rule leaves it
/// unvalidated.
fn execute_item_drop_upserts(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "item-drop-upserts";
    let subject_rule = "item_drop_upserts";
    let server_tick = parse_server_tick(case, input)?;
    let drops = parse_drops(case, input)?;

    if let Some(rejection) = count_rejection(subject_rule, drops.len()) {
        verify_drop_upserts_rejection(case, subject, server_tick, &drops, &rejection)?;
        return drop_upserts_error(case, server_tick, &drops, &rejection);
    }
    for (index, drop) in drops.iter().enumerate() {
        if let Some(rejection) = drop_record_rejection(subject_rule, index, drop) {
            verify_drop_upserts_rejection(case, subject, server_tick, &drops, &rejection)?;
            return drop_upserts_error(case, server_tick, &drops, &rejection);
        }
        if index > 0 && !raw_drop_id_less(&drops[index - 1].id, &drop.id) {
            let rejection = order_rejection(subject_rule);
            verify_drop_upserts_rejection(case, subject, server_tick, &drops, &rejection)?;
            return drop_upserts_error(case, server_tick, &drops, &rejection);
        }
    }

    let mut records = Vec::with_capacity(drops.len());
    for drop in &drops {
        records.push(construct_item_drop(case, subject, drop)?);
    }
    let batch = ItemDropUpserts::try_new(ItemDropUpsertsParts {
        server_tick,
        drops: records.into_boxed_slice(),
    })
    .map_err(|error| construction_conflict(case, subject, error))?;
    let mut fields = JsonMap::new();
    fields.insert(
        "server_tick".to_string(),
        normalize_u64(batch.server_tick()),
    );
    fields.insert(
        "drops".to_string(),
        Value::Array(
            batch
                .drops()
                .iter()
                .map(|drop| {
                    serde_json::json!({
                        "id": drop_id_value(drop.id()),
                        "block_index": drop.block_index(),
                        "stack": stack_value(drop.stack()),
                    })
                })
                .collect(),
        ),
    );
    Ok(normalized_ok(subject, fields))
}

/// Publishes one classified item-drop upserts rejection from the raw batch.
fn drop_upserts_error(
    case: &FrozenCase,
    server_tick: u64,
    drops: &[RawDrop],
    rejection: &Rejection,
) -> Result<serde_json::Value, DispatchError> {
    let mut fields = JsonMap::new();
    fields.insert("server_tick".to_string(), normalize_u64(server_tick));
    fields.insert("drops".to_string(), drops_raw_value(drops));
    normalized_error(case, rejection.category, &rejection.rule, fields)
}

/// Selects the first rule one item-drop upsert record breaks: the identity's
/// slot range and generation, the block index inside the announced chunk,
/// and the carried stack, in the Go validator's precedence.
fn drop_record_rejection(subject: &str, index: usize, drop: &RawDrop) -> Option<Rejection> {
    if drop.id.slot >= DROPS_PER_CHUNK {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("{subject}.drop_{index}.id.slot"),
            error: Some(DomainError::InvalidDropSlot),
        });
    }
    if drop.id.generation == 0 {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("{subject}.drop_{index}.id.generation"),
            error: Some(DomainError::InvalidDropGeneration),
        });
    }
    if drop.block_index >= MAX_CHUNK_BLOCK_INDEX {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{subject}.drop_{index}.block_index"),
            error: Some(DomainError::InvalidBlockIndex),
        });
    }
    stack_rejection(&format!("{subject}.drop_{index}"), &drop.stack)
}

/// Selects the first rule one carried stack breaks, in the precedence the Go
/// `core.ItemStack.Valid` rule applies: registration first, then the count,
/// then the durability. The exactly-zero triple is the canonical empty stack
/// and stays admitted.
///
/// The Go rule reports one shared `stack.item` rejection for every value the
/// absent item number cannot carry, while the Rust constructor names the
/// finer `InvalidCount`/`InvalidDurability` for the same inputs, so that
/// corner maps the shared Go rule onto the constructor's own error.
fn stack_rejection(prefix: &str, stack: &RawStack) -> Option<Rejection> {
    if item_stack_limit(stack.item).is_none() {
        if stack.item == 0 && stack.count == 0 && stack.durability == 0 {
            return None;
        }
        let error = if stack.item != 0 {
            DomainError::InvalidItem
        } else if stack.count != 0 {
            DomainError::InvalidCount
        } else {
            DomainError::InvalidDurability
        };
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{prefix}.stack.item"),
            error: Some(error),
        });
    }
    let limit = item_stack_limit(stack.item).expect("registered item has a limit");
    if stack.count == 0 || stack.count > limit {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{prefix}.stack.count"),
            error: Some(DomainError::InvalidCount),
        });
    }
    let durable = durability_max(stack.item);
    let broken = match durable {
        Some(max) => stack.durability < 1 || stack.durability > max,
        None => stack.durability != 0,
    };
    if broken {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("{prefix}.stack.durability"),
            error: Some(DomainError::InvalidDurability),
        });
    }
    None
}

/// Verifies the constructor agreement for one classified item-drop upserts
/// rejection. The stack rules are verified through the shared
/// `ItemStack::try_new`, the identity rules through `DropId::try_new`, and
/// the block index through the record constructor.
fn verify_drop_upserts_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    drops: &[RawDrop],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "item_drop_upserts.count_range" => {
            if drops.is_empty() {
                ItemDropUpserts::try_new(ItemDropUpsertsParts {
                    server_tick,
                    drops: Vec::new().into_boxed_slice(),
                })
                .map(|_| ())
            } else {
                ItemDropUpserts::try_new(ItemDropUpsertsParts {
                    server_tick,
                    drops: minimal_item_drops(SEMANTIC_BATCH_CAP + 1),
                })
                .map(|_| ())
            }
        }
        "item_drop_upserts.strictly_increasing_ids" => {
            let prefix = order_prefix_end(drops.len(), |index| {
                !raw_drop_id_less(&drops[index - 1].id, &drops[index].id)
            })
            .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let mut records = Vec::new();
            for drop in &drops[..prefix] {
                records.push(construct_item_drop(case, subject, drop)?);
            }
            ItemDropUpserts::try_new(ItemDropUpsertsParts {
                server_tick,
                drops: records.into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let (index, field) = drop_rule_index(case, subject, rule, "item_drop_upserts.drop_")?;
            let drop = drops
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            let verdict = match field {
                "id.slot" | "id.generation" => raw_drop_id_verdict(&drop.id).map(|_| ()),
                "block_index" => {
                    let id = raw_drop_id_verdict(&drop.id)
                        .map_err(|error| construction_conflict(case, subject, error))?;
                    let stack = ItemStack::try_new(
                        drop.stack.item,
                        drop.stack.count,
                        drop.stack.durability,
                    )
                    .map_err(|error| construction_conflict(case, subject, error))?;
                    ItemDrop::try_new(ItemDropParts {
                        id,
                        block_index: drop.block_index,
                        stack,
                    })
                    .map(|_| ())
                }
                "stack.item" | "stack.count" | "stack.durability" => {
                    ItemStack::try_new(drop.stack.item, drop.stack.count, drop.stack.durability)
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

/// Builds `count` minimal valid drops with strictly increasing full identity
/// keys, the synthetic payload the count-overflow verdict runs on. The empty
/// stack is a valid slot value, so the synthetic drops carry it.
fn minimal_item_drops(count: usize) -> Box<[ItemDrop]> {
    (0..count)
        .map(|index| {
            ItemDrop::try_new(ItemDropParts {
                id: synthetic_drop_id(index),
                block_index: 0,
                stack: ItemStack::EMPTY,
            })
            .expect("valid synthetic drop")
        })
        .collect()
}

/// Builds one synthetic strictly ordered drop identity from its linear
/// index: the chunk column carries the overflow past the per-chunk slots.
fn synthetic_drop_id(index: usize) -> DropId {
    DropId::try_new(
        0,
        ChunkPos::new((index / DROPS_PER_CHUNK as usize) as i32, 0),
        (index % DROPS_PER_CHUNK as usize) as u8,
        1,
    )
    .expect("valid synthetic drop identity")
}

/// Constructs one checked `DropId` from a raw identity.
fn construct_drop_id(
    case: &FrozenCase,
    subject: &str,
    id: &RawDropId,
) -> Result<DropId, DispatchError> {
    DropId::try_new(
        id.dimension,
        ChunkPos::new(id.chunk[0], id.chunk[1]),
        id.slot,
        id.generation,
    )
    .map_err(|error| construction_conflict(case, subject, error))
}

/// Constructs one checked `ItemStack` from a raw stack.
fn construct_stack(
    case: &FrozenCase,
    subject: &str,
    stack: &RawStack,
) -> Result<ItemStack, DispatchError> {
    ItemStack::try_new(stack.item, stack.count, stack.durability)
        .map_err(|error| construction_conflict(case, subject, error))
}

/// Constructs one checked drop for an admitted case.
fn construct_item_drop(
    case: &FrozenCase,
    subject: &str,
    drop: &RawDrop,
) -> Result<ItemDrop, DispatchError> {
    ItemDrop::try_new(ItemDropParts {
        id: construct_drop_id(case, subject, &drop.id)?,
        block_index: drop.block_index,
        stack: construct_stack(case, subject, &drop.stack)?,
    })
    .map_err(|error| construction_conflict(case, subject, error))
}

/// Renders one checked drop identity in the normalized field map: the object
/// of its five ordered key fields, keeping the raw dimension exactly as the
/// record carried it.
fn drop_id_value(id: DropId) -> Value {
    serde_json::json!({
        "dimension": id.dimension(),
        "chunk": [id.chunk().x(), id.chunk().z()],
        "slot": id.slot(),
        "generation": id.generation(),
    })
}

/// Renders one checked stack in the normalized field map.
fn stack_value(stack: ItemStack) -> Value {
    serde_json::json!({
        "item": stack.item(),
        "count": stack.count(),
        "durability": stack.durability(),
    })
}

/// Renders one raw drop identity in the normalized field map.
fn raw_drop_id_value(id: &RawDropId) -> Value {
    serde_json::json!({
        "dimension": id.dimension,
        "chunk": [id.chunk[0], id.chunk[1]],
        "slot": id.slot,
        "generation": id.generation,
    })
}

/// Renders one raw item-drop upsert batch in the normalized field map.
fn drops_raw_value(drops: &[RawDrop]) -> Value {
    Value::Array(
        drops
            .iter()
            .map(|drop| {
                serde_json::json!({
                    "id": raw_drop_id_value(&drop.id),
                    "block_index": drop.block_index,
                    "stack": {
                        "item": drop.stack.item,
                        "count": drop.stack.count,
                        "durability": drop.stack.durability,
                    },
                })
            })
            .collect(),
    )
}

/// Executes one `item-drop-removes` case, whose records are bare drop
/// identities.
fn execute_item_drop_removes(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "item-drop-removes";
    let subject_rule = "item_drop_removes";
    let server_tick = parse_server_tick(case, input)?;
    let ids = parse_drop_ids(case, input)?;

    if let Some(rejection) = count_rejection(subject_rule, ids.len()) {
        verify_drop_removes_rejection(case, subject, server_tick, &ids, &rejection)?;
        return drop_removes_error(case, server_tick, &ids, &rejection);
    }
    for (index, id) in ids.iter().enumerate() {
        if let Some(rejection) = drop_id_rejection(subject_rule, index, id) {
            verify_drop_removes_rejection(case, subject, server_tick, &ids, &rejection)?;
            return drop_removes_error(case, server_tick, &ids, &rejection);
        }
        if index > 0 && !raw_drop_id_less(&ids[index - 1], id) {
            let rejection = order_rejection(subject_rule);
            verify_drop_removes_rejection(case, subject, server_tick, &ids, &rejection)?;
            return drop_removes_error(case, server_tick, &ids, &rejection);
        }
    }

    let typed: Vec<DropId> = ids
        .iter()
        .map(|id| construct_drop_id(case, subject, id))
        .collect::<Result<_, _>>()?;
    let batch = ItemDropRemoves::try_new(ItemDropRemovesParts {
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
        Value::Array(batch.ids().iter().map(|id| drop_id_value(*id)).collect()),
    );
    Ok(normalized_ok(subject, fields))
}

/// Publishes one classified item-drop removes rejection from the raw batch.
fn drop_removes_error(
    case: &FrozenCase,
    server_tick: u64,
    ids: &[RawDropId],
    rejection: &Rejection,
) -> Result<serde_json::Value, DispatchError> {
    let mut fields = JsonMap::new();
    fields.insert("server_tick".to_string(), normalize_u64(server_tick));
    fields.insert(
        "ids".to_string(),
        Value::Array(ids.iter().map(raw_drop_id_value).collect()),
    );
    normalized_error(case, rejection.category, &rejection.rule, fields)
}

/// Selects the first rule one bare drop identity breaks: the slot range,
/// then the nonzero generation.
fn drop_id_rejection(subject: &str, index: usize, id: &RawDropId) -> Option<Rejection> {
    if id.slot >= DROPS_PER_CHUNK {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("{subject}.id_{index}.slot"),
            error: Some(DomainError::InvalidDropSlot),
        });
    }
    if id.generation == 0 {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("{subject}.id_{index}.generation"),
            error: Some(DomainError::InvalidDropGeneration),
        });
    }
    None
}

/// Verifies the constructor agreement for one classified item-drop removes
/// rejection.
fn verify_drop_removes_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    ids: &[RawDropId],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "item_drop_removes.count_range" => {
            if ids.is_empty() {
                ItemDropRemoves::try_new(ItemDropRemovesParts {
                    server_tick,
                    ids: Vec::new().into_boxed_slice(),
                })
                .map(|_| ())
            } else {
                ItemDropRemoves::try_new(ItemDropRemovesParts {
                    server_tick,
                    ids: minimal_drop_ids(SEMANTIC_BATCH_CAP + 1),
                })
                .map(|_| ())
            }
        }
        "item_drop_removes.strictly_increasing_ids" => {
            let prefix = order_prefix_end(ids.len(), |index| {
                !raw_drop_id_less(&ids[index - 1], &ids[index])
            })
            .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            ItemDropRemoves::try_new(ItemDropRemovesParts {
                server_tick,
                ids: ids[..prefix]
                    .iter()
                    .map(|id| construct_drop_id(case, subject, id))
                    .collect::<Result<Vec<_>, _>>()?
                    .into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let (index, field) = drop_rule_index(case, subject, rule, "item_drop_removes.id_")?;
            let id = ids
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            let verdict = match field {
                "slot" | "generation" => raw_drop_id_verdict(id).map(|_| ()),
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

/// Builds `count` minimal valid drop identities with strictly increasing
/// full identity keys.
fn minimal_drop_ids(count: usize) -> Box<[DropId]> {
    (0..count).map(synthetic_drop_id).collect()
}

/// Runs the checked `DropId` constructor verdict on one raw identity, the
/// shape the identity-rule verdicts compare against their mapped error.
fn raw_drop_id_verdict(id: &RawDropId) -> Result<DropId, DomainError> {
    DropId::try_new(
        id.dimension,
        ChunkPos::new(id.chunk[0], id.chunk[1]),
        id.slot,
        id.generation,
    )
}

/// Splits one indexed per-record rule into its record index and its trailing
/// field path, for the `drop_<index>.<field>` and `id_<index>.<field>`
/// shapes the two drop rules publish.
fn drop_rule_index<'a>(
    case: &FrozenCase,
    subject: &str,
    rule: &'a str,
    prefix: &str,
) -> Result<(usize, &'a str), DispatchError> {
    let rest = rule
        .strip_prefix(prefix)
        .ok_or_else(|| classification_conflict(case, subject, rule))?;
    let (index_text, field) = rest
        .split_once('.')
        .ok_or_else(|| classification_conflict(case, subject, rule))?;
    let index = index_text
        .parse()
        .map_err(|_| classification_conflict(case, subject, rule))?;
    Ok((index, field))
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
fn event_objects_execute_45_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}

/// Pins the exact normalized content of the boundary cases: the complete
/// kind-by-dimension matrix, the lossless raw drop dimension, the accepted
/// empty stack, every stack rejection rule, both block-index ends, the slot
/// and generation identity bounds, the zero tick, and the reversed and
/// duplicate identity orders with the submitted order preserved.
#[test]
fn event_objects_pin_seed_and_boundary_outcomes() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    let by_id = |id: &str| {
        executed
            .iter()
            .find(|case| case.case.id == id)
            .unwrap_or_else(|| panic!("missing case {id}"))
    };

    // The seed batch enumerates every kind-by-dimension combination and the
    // authority admits the whole batch, with the first record's exact pose
    // and velocity bits.
    let seed = by_id("domain.event/1/projectile-spawn-all-kind-dimension-combinations");
    assert_eq!(seed.actual["kind"], "ok");
    let spawns = seed.actual["fields"]["spawns"].as_array().unwrap();
    assert_eq!(spawns.len(), 4);
    let mut seen = [false; 4];
    for record in spawns {
        let kind = record["kind"].as_u64().unwrap();
        let dimension = record["dimension"].as_u64().unwrap();
        let slot = match (kind, dimension) {
            (0, 0) => 0,
            (0, 1) => 1,
            (1, 0) => 2,
            (1, 1) => 3,
            _ => panic!("unexpected kind {kind} dimension {dimension}"),
        };
        seen[slot] = true;
    }
    assert!(seen.iter().all(|value| *value), "all four combinations");
    assert_eq!(spawns[0]["id"], "1");
    assert_eq!(spawns[0]["position"][0], "3fc00000");
    assert_eq!(spawns[0]["position"][1], "42800000");
    assert_eq!(spawns[0]["position"][2], "c0500000");
    assert_eq!(spawns[0]["velocity"][0], "3e800000");
    // The unknown kind and dimension bytes reject as enums and keep their
    // raw values in the rejection.
    for (id, field, raw) in [
        ("projectile-spawn-unknown-kind", "kind", 2),
        ("projectile-spawn-unknown-dimension", "dimension", 2),
    ] {
        let rejected = by_id(&format!("domain.event/1/{id}"));
        assert_eq!(rejected.actual["kind"], "error", "{id}");
        assert_eq!(rejected.actual["category"], "invalid-enum", "{id}");
        assert_eq!(
            rejected.actual["fields"]["rule"],
            format!("projectile_spawn.record_0.{field}"),
            "{id}"
        );
        assert_eq!(rejected.actual["fields"]["spawns"][0][field], raw, "{id}");
    }

    // The raw dimension -1 is admitted and published back losslessly by
    // both drop rules.
    for rule in ["item-drop-upserts", "item-drop-removes"] {
        let admitted = by_id(&format!("domain.event/1/{rule}-raw-dimension-negative-one"));
        assert_eq!(admitted.actual["kind"], "ok", "{rule}");
        let array = if rule == "item-drop-upserts" {
            "drops"
        } else {
            "ids"
        };
        let subject = if rule == "item-drop-upserts" {
            admitted.actual["fields"][array][0]["id"]["dimension"].clone()
        } else {
            admitted.actual["fields"][array][0]["dimension"].clone()
        };
        assert_eq!(subject, -1, "{rule}");
    }

    // The canonical empty stack is an admitted stack value.
    let empty_stack = by_id("domain.event/1/item-drop-upserts-empty-stack");
    assert_eq!(empty_stack.actual["kind"], "ok");
    assert_eq!(empty_stack.actual["fields"]["drops"][0]["stack"]["item"], 0);
    assert_eq!(
        empty_stack.actual["fields"]["drops"][0]["stack"]["count"],
        0
    );
    assert_eq!(
        empty_stack.actual["fields"]["drops"][0]["stack"]["durability"],
        0
    );

    // Every stack rejection keeps its exact Go verdict and category.
    for (id, field) in [
        ("unregistered-item-66", "item"),
        ("zero-count", "count"),
        ("nondurable-with-durability", "durability"),
    ] {
        let rejected = by_id(&format!("domain.event/1/item-drop-upserts-{id}"));
        assert_eq!(rejected.actual["kind"], "error", "{id}");
        assert_eq!(rejected.actual["category"], "invalid-value", "{id}");
        assert_eq!(
            rejected.actual["fields"]["rule"],
            format!("item_drop_upserts.drop_0.stack.{field}"),
            "{id}"
        );
    }

    // The inclusive block-index end stays admitted while the exclusive end
    // one step above it is the rejection.
    let inclusive = by_id("domain.event/1/item-drop-upserts-block-index-98303");
    assert_eq!(inclusive.actual["kind"], "ok");
    assert_eq!(
        inclusive.actual["fields"]["drops"][0]["block_index"],
        98_303
    );
    let exclusive = by_id("domain.event/1/item-drop-upserts-block-index-98304");
    assert_eq!(exclusive.actual["kind"], "error");
    assert_eq!(exclusive.actual["category"], "invalid-value");
    assert_eq!(
        exclusive.actual["fields"]["rule"],
        "item_drop_upserts.drop_0.block_index"
    );

    // The slot and generation identity bounds reject as identities in both
    // drop rules.
    for (id, subject, prefix, field) in [
        (
            "item-drop-upserts-slot-32",
            "item_drop_upserts",
            "drop_0.id",
            "slot",
        ),
        (
            "item-drop-upserts-zero-generation",
            "item_drop_upserts",
            "drop_0.id",
            "generation",
        ),
        (
            "item-drop-removes-slot-32",
            "item_drop_removes",
            "id_0",
            "slot",
        ),
        (
            "item-drop-removes-zero-generation",
            "item_drop_removes",
            "id_0",
            "generation",
        ),
    ] {
        let rejected = by_id(&format!("domain.event/1/{id}"));
        assert_eq!(rejected.actual["kind"], "error", "{id}");
        assert_eq!(rejected.actual["category"], "invalid-identity", "{id}");
        assert_eq!(
            rejected.actual["fields"]["rule"],
            format!("{subject}.{prefix}.{field}"),
            "{id}"
        );
    }

    // The zero tick is an admitted publish instant for every rule.
    for id in [
        "projectile-spawn-zero-tick",
        "projectile-state-zero-tick",
        "projectile-despawn-zero-tick",
        "item-drop-upserts-zero-tick",
        "item-drop-removes-zero-tick",
    ] {
        let zero = by_id(&format!("domain.event/1/{id}"));
        assert_eq!(zero.actual["kind"], "ok", "{id}");
        assert_eq!(zero.actual["fields"]["server_tick"], "0", "{id}");
    }

    // The reversed and duplicate orders reject with the shared rule, and the
    // rejection publishes the records in the submitted order: the reversed
    // batch leads with the identity that was submitted first.
    for (id, subject) in [
        ("projectile-spawn-reversed", "projectile_spawn"),
        ("projectile-spawn-duplicate", "projectile_spawn"),
        ("projectile-state-reversed", "projectile_state"),
        ("projectile-state-duplicate", "projectile_state"),
        ("projectile-despawn-reversed", "projectile_despawn"),
        ("projectile-despawn-duplicate", "projectile_despawn"),
        ("item-drop-upserts-reversed", "item_drop_upserts"),
        ("item-drop-upserts-duplicate", "item_drop_upserts"),
        ("item-drop-removes-reversed", "item_drop_removes"),
        ("item-drop-removes-duplicate", "item_drop_removes"),
    ] {
        let rejected = by_id(&format!("domain.event/1/{id}"));
        assert_eq!(rejected.actual["kind"], "error", "{id}");
        assert_eq!(
            rejected.actual["fields"]["rule"],
            format!("{subject}.strictly_increasing_ids"),
            "{id}"
        );
    }
    let reversed_ids = by_id("domain.event/1/projectile-despawn-reversed");
    assert_eq!(reversed_ids.actual["fields"]["ids"][0], "2");
    assert_eq!(reversed_ids.actual["fields"]["ids"][1], "1");
    let reversed_drops = by_id("domain.event/1/item-drop-upserts-reversed");
    assert_eq!(reversed_drops.actual["fields"]["drops"][0]["id"]["slot"], 2);
    assert_eq!(reversed_drops.actual["fields"]["drops"][1]["id"]["slot"], 1);
}

/// Pins the full `DropId` ordering with raw probes: a batch whose first
/// record carries a smaller raw dimension stays admitted even when every
/// later ordering field is greater, and the same records with their
/// dimensions equalized flip to the strict-order rejection because the chunk
/// column then decides. The dimension is compared as a raw number first,
/// exactly as the derived domain ordering does.
#[test]
fn event_objects_drop_ordering_uses_raw_dimension_first() {
    let first = serde_json::json!({
        "dimension": -1, "chunk": [5, 5], "slot": 9, "generation": 3
    });
    let second = serde_json::json!({
        "dimension": 0, "chunk": [-9, -9], "slot": 0, "generation": 1
    });
    let stack = serde_json::json!({"item": 1, "count": 1, "durability": 0});

    let cross_dimension = probe_case(
        "domain.event/1/probe-drop-cross-dimension",
        serde_json::json!({
            "consumer": "mornlea_domain",
            "rule": "item-drop-upserts",
            "server_tick": "7",
            "spawns": [],
            "states": [],
            "ids": [],
            "drops": [
                {"id": first, "block_index": 17, "stack": stack},
                {"id": second, "block_index": 18, "stack": stack},
            ],
        }),
    );
    let outcome = execute(&cross_dimension).expect("execute cross-dimension probe");
    assert_eq!(outcome["kind"], "ok");
    assert_eq!(outcome["fields"]["drops"][0]["id"]["dimension"], -1);

    let mut equalized = cross_dimension;
    let input = equalized
        .input_json
        .as_mut()
        .and_then(|value| value.as_object_mut())
        .expect("probe input object");
    if let Some(drops) = input.get_mut("drops").and_then(|d| d.as_array_mut()) {
        drops[0]["id"]["dimension"] = serde_json::json!(0);
    }
    let outcome = execute(&equalized).expect("execute equalized probe");
    assert_eq!(outcome["kind"], "error");
    assert_eq!(
        outcome["fields"]["rule"],
        "item_drop_upserts.strictly_increasing_ids"
    );
    // The submitted order is preserved in the rejection: the greater chunk
    // column stays first.
    assert_eq!(outcome["fields"]["drops"][0]["id"]["chunk"][0], 5);
    assert_eq!(outcome["fields"]["drops"][1]["id"]["chunk"][0], -9);
}

/// Proves the frozen precedence with direct raw probes: a raw batch count
/// above the semantic cap classifies as the count rule even when its first
/// record is also invalid, and the adapter never sorts a submitted batch
/// before publishing it.
#[test]
fn event_objects_raw_probe_count_overflow_and_no_sorting() {
    let overflow = probe_case(
        "domain.event/1/probe-projectile-spawn-overflow",
        serde_json::json!({
            "consumer": "mornlea_domain",
            "rule": "projectile-spawn",
            "server_tick": "1",
            "spawns": (0..(MAX_SEMANTIC_BATCH_RECORDS + 1))
                .map(|index| {
                    let id = if index == 0 { 0u64 } else { index as u64 };
                    serde_json::json!({
                        "id": id.to_string(),
                        "kind": 0,
                        "dimension": 0,
                        "position": ["0", "0", "0"],
                        "velocity": ["0", "0", "0"],
                    })
                })
                .collect::<Vec<_>>(),
            "states": [],
            "ids": [],
            "drops": [],
        }),
    );
    let outcome = execute(&overflow).expect("execute overflow probe");
    assert_eq!(outcome["kind"], "error");
    assert_eq!(outcome["category"], "invalid-value");
    assert_eq!(outcome["fields"]["rule"], "projectile_spawn.count_range");

    let overflow_drops = probe_case(
        "domain.event/1/probe-item-drop-removes-overflow",
        serde_json::json!({
            "consumer": "mornlea_domain",
            "rule": "item-drop-removes",
            "server_tick": "1",
            "spawns": [],
            "states": [],
            "ids": (0..(MAX_SEMANTIC_BATCH_RECORDS + 1))
                .map(|index| {
                    serde_json::json!({
                        "dimension": if index == 0 { -1 } else { 0 },
                        "chunk": [index as i32 / 32, 0],
                        "slot": index % 32,
                        "generation": if index == 0 { 0 } else { 1 },
                    })
                })
                .collect::<Vec<_>>(),
            "drops": [],
        }),
    );
    let outcome = execute(&overflow_drops).expect("execute drop overflow probe");
    assert_eq!(outcome["fields"]["rule"], "item_drop_removes.count_range");
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
