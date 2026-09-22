//! Topic module for domain.event player/outcome corpus cases.
//!
//! The four rules here are the Go producer's player publication and the three
//! command outcomes: the per-session `PlayerState` record, a refused
//! `CommandRejected`, a confirmed `PlaceBlockSucceeded` and a landed
//! `CombatHit`. Each executor parses the case input, selects the Go rejection
//! rule from the raw values in the producer's own precedence, and then proves
//! the matching public Rust constructor agrees: a classified acceptance must
//! construct, and a classified rejection must return the mapped
//! `DomainError`. Accepted fields are read from the constructed Rust values;
//! a rejected value has no getters, so its projection is built from the
//! parsed input instead.
//!
//! This family travels server-to-client: no record enters a
//! `CommandEnvelope`, and no case synthesizes a sequence envelope. The
//! classification runs on raw input because several Go rules share one Rust
//! error variant (four survival fields share `InvalidSurvivalValue`, two
//! mining shapes share `InvalidMiningState`), so only the raw field and its
//! precedence select the exact published rule.

use super::support::{
    DispatchError, ExecutedCase, JsonMap, assert_domain_normalized, exact_array, execute_topic,
    input_object, invalid_case, normalize_f32, normalize_u64, normalized_error, normalized_ok,
    parse_f32_token, required_array, required_bool, required_i8, required_i32, required_string,
    required_u8, required_u16, required_u64, value_i32, value_string,
};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};
use mornlea_domain::{
    BlockPos, CombatHit, CombatTarget, CommandRejection, Dimension, DomainError, FiniteVec3,
    LookAngles, MiningState, MiningStateParts, MotionState, MotionStateParts, PlacementSuccess,
    PlayerState, PlayerStateParts, RejectReason, Season, SurvivalState, SurvivalStateParts,
    Weather, WorldState, WorldStateParts,
};
use serde_json::Value;

pub const EXPECTED_COUNT: usize = 47;

const RULES: &[&str] = &[
    "player-state",
    "command-rejected",
    "place-block-succeeded",
    "combat-hit",
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
        "player-state" => execute_player_state(case, input),
        "command-rejected" => execute_command_rejected(case, input),
        "place-block-succeeded" => execute_place_block_succeeded(case, input),
        "combat-hit" => execute_combat_hit(case, input),
        unknown => Err(invalid_case(
            case,
            format!("unknown player event rule '{unknown}'"),
        )),
    }
}

/// Executes one `player-state` case, the only record in this family that
/// carries a full field map.
///
/// Every field is required, because the record is complete by definition and
/// a zero is a meaningful value on most of them. The rule is selected from
/// the raw values in the Go validator's precedence, then the owning
/// constructor verdict is cross-checked before the normalized result is
/// published.
fn execute_player_state(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let raw = parse_player_state(case, input)?;

    match player_state_rejection(&raw) {
        Some(rejection) => {
            verify_player_state_rejection(case, &raw, &rejection)?;
            normalized_error(
                case,
                rejection.category,
                rejection.rule,
                player_state_projection(&raw),
            )
        }
        None => {
            let state = construct_player_state(case, &raw)?;
            let fields = player_state_fields(&state);
            Ok(normalized_ok("player-state", fields))
        }
    }
}

/// Executes one `command-rejected` case, whose entire payload is the
/// sequence and one published reason name.
fn execute_command_rejected(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let sequence = required_u64(case, input, "sequence")?;
    let reason = required_string(case, input, "reason")?;

    match reason_variant(reason) {
        Some(variant) => {
            let rejection = CommandRejection::new(sequence, variant);
            let mut fields = JsonMap::new();
            fields.insert("sequence".to_string(), normalize_u64(rejection.sequence()));
            fields.insert(
                "reason".to_string(),
                Value::String(reason_text(rejection.reason()).to_string()),
            );
            Ok(normalized_ok("command-rejected", fields))
        }
        // Rust publishes no raw-string `RejectReason` parser, so an unknown
        // reason text is a classifier-only rejection: no Rust value can
        // represent it and there is no constructor verdict to check.
        None => {
            let mut fields = JsonMap::new();
            fields.insert("sequence".to_string(), normalize_u64(sequence));
            fields.insert("reason".to_string(), Value::String(reason.to_string()));
            normalized_error(
                case,
                "invalid-enum",
                "command_rejected.registered_reason",
                fields,
            )
        }
    }
}

/// Executes one `place-block-succeeded` case. The Go packet publishes no rule
/// of its own beyond the registry entry, so every sequence is admitted and
/// the classifier has nothing to name.
fn execute_place_block_succeeded(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let sequence = required_u64(case, input, "sequence")?;
    let success = PlacementSuccess::new(sequence);
    let mut fields = JsonMap::new();
    fields.insert("sequence".to_string(), normalize_u64(success.sequence()));
    Ok(normalized_ok("place-block-succeeded", fields))
}

/// Executes one `combat-hit` case, whose entire payload is the server tick,
/// the damage and the target kind.
///
/// The tick and the damage are selected from the raw input before the closed
/// target is attempted, because the Go validator checks them in that order;
/// the constructor verdict then replays the same order.
fn execute_combat_hit(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let server_tick = required_u64(case, input, "server_tick")?;
    let damage = required_u8(case, input, "damage")?;
    let target_kind = required_u8(case, input, "target_kind")?;

    match combat_hit_rejection(server_tick, damage, target_kind) {
        Some(rejection) => {
            // Replay the Go order in the constructor chain: the target first,
            // then the hit, so the first error returned is the classified
            // one whenever the classification agrees with the constructors.
            let constructed = match CombatTarget::try_new(target_kind) {
                Err(error) => Err(error),
                Ok(target) => CombatHit::try_new(server_tick, damage, target).map(|_| ()),
            };
            let Some(expected) = rejection.error else {
                return Err(classification_conflict(case, "combat-hit", rejection.rule));
            };
            match constructed {
                Err(error) if error == expected => {}
                _ => return Err(classification_conflict(case, "combat-hit", rejection.rule)),
            }
            let mut fields = JsonMap::new();
            fields.insert("server_tick".to_string(), normalize_u64(server_tick));
            fields.insert("damage".to_string(), Value::from(damage));
            fields.insert("target_kind".to_string(), Value::from(target_kind));
            normalized_error(case, rejection.category, rejection.rule, fields)
        }
        None => {
            let target = CombatTarget::try_new(target_kind)
                .map_err(|_| classification_conflict(case, "combat-hit", ""))?;
            let hit = CombatHit::try_new(server_tick, damage, target)
                .map_err(|_| classification_conflict(case, "combat-hit", ""))?;
            let mut fields = JsonMap::new();
            fields.insert("server_tick".to_string(), normalize_u64(hit.server_tick()));
            fields.insert("damage".to_string(), Value::from(hit.damage()));
            fields.insert(
                "target_kind".to_string(),
                Value::from(hit.target().wire_id()),
            );
            Ok(normalized_ok("combat-hit", fields))
        }
    }
}

/// One classified rejection: its normalized category, its exact Go rule name,
/// and the `DomainError` the owning constructor must return.
///
/// `error` is `None` for a classifier-only rejection whose bad value cannot
/// enter any Rust type, so no constructor verdict exists to check.
struct Rejection {
    category: &'static str,
    rule: &'static str,
    error: Option<DomainError>,
}

impl Rejection {
    /// A rejection only the classifier can decide, because the rejected raw
    /// value has no constructible Rust representation.
    fn classifier(category: &'static str, rule: &'static str) -> Self {
        Self {
            category,
            rule,
            error: None,
        }
    }
}

/// Authoritative health maximum, from the Go `core.MaxHealth`. The combat
/// damage bound reuses it because that is the Go rule.
const MAX_HEALTH: u8 = 20;

/// Authoritative oxygen maximum, from the Go `core.MaxOxygenTicks`.
const MAX_OXYGEN: u16 = 300;

/// Authoritative hunger maximum, from the Go `core.MaxHunger`.
const MAX_HUNGER: u8 = 20;

/// Authoritative armor-points maximum, from the Go `core.MaxArmorPoints`.
const MAX_ARMOR_POINTS: u8 = 20;

/// Length of one display day in ticks, from the Go `core.DayLengthTicks`.
/// The day phase offset has to stay strictly below it.
const DAY_LENGTH_TICKS: u16 = 24000;

/// Highest published weather kind, from the Go `core.WeatherThunder`.
const WEATHER_THUNDER: u8 = 2;

/// Highest published season, from the Go `core.SeasonWinter`.
const SEASON_WINTER: u8 = 3;

/// Overworld dimension ID, from the Go `core.Overworld`.
const DIMENSION_OVERWORLD: i32 = 0;

/// Depths dimension ID, from the Go `core.Depths`.
const DIMENSION_DEPTHS: i32 = 1;

/// The parsed raw shape of one player publication.
struct RawPlayerState {
    server_tick: u64,
    last_input_sequence: u64,
    dimension: i32,
    position: [f32; 3],
    velocity: [f32; 3],
    yaw: f32,
    pitch: f32,
    on_ground: bool,
    ready: bool,
    reset: bool,
    mining_active: bool,
    mining_target: [i32; 3],
    mining_progress: u16,
    mining_required: u16,
    mining_harvestable: bool,
    health: u8,
    oxygen: u16,
    hunger: u8,
    saturation_zero: bool,
    day_phase_offset: u16,
    world_time_ticks: u64,
    weather_kind: u8,
    season: u8,
    season_progress: u8,
    temperature: i8,
    armor_points: u8,
}

/// Parses one complete player-publication envelope.
///
/// Every field the record carries is required; a missing field or a value
/// outside its declared width is a hard schema failure, never a normalized
/// rejection, because the producer's own decoder treats both the same way.
fn parse_player_state(case: &FrozenCase, input: &JsonMap) -> Result<RawPlayerState, DispatchError> {
    Ok(RawPlayerState {
        server_tick: required_u64(case, input, "server_tick")?,
        last_input_sequence: required_u64(case, input, "last_input_sequence")?,
        dimension: required_i32(case, input, "dimension")?,
        position: parse_vec3(case, input, "position")?,
        velocity: parse_vec3(case, input, "velocity")?,
        yaw: parse_angle(case, input, "yaw")?,
        pitch: parse_angle(case, input, "pitch")?,
        on_ground: required_bool(case, input, "on_ground")?,
        ready: required_bool(case, input, "ready")?,
        reset: required_bool(case, input, "reset")?,
        mining_active: required_bool(case, input, "mining_active")?,
        mining_target: parse_block_pos(case, input, "mining_target")?,
        mining_progress: required_u16(case, input, "mining_progress")?,
        mining_required: required_u16(case, input, "mining_required")?,
        mining_harvestable: required_bool(case, input, "mining_harvestable")?,
        health: required_u8(case, input, "health")?,
        oxygen: required_u16(case, input, "oxygen")?,
        hunger: required_u8(case, input, "hunger")?,
        saturation_zero: required_bool(case, input, "saturation_zero")?,
        day_phase_offset: required_u16(case, input, "day_phase_offset")?,
        world_time_ticks: required_u64(case, input, "world_time_ticks")?,
        weather_kind: required_u8(case, input, "weather_kind")?,
        season: required_u8(case, input, "season")?,
        season_progress: required_u8(case, input, "season_progress")?,
        temperature: required_i8(case, input, "temperature")?,
        armor_points: required_u8(case, input, "armor_points")?,
    })
}

/// Parses one exact three-component float-token vector.
fn parse_vec3(case: &FrozenCase, input: &JsonMap, key: &str) -> Result<[f32; 3], DispatchError> {
    let values = required_array(case, input, key)?;
    let slots = exact_array::<3>(case, key, values)?;
    let mut out = [0.0f32; 3];
    for (index, slot) in slots.iter().enumerate() {
        let path = format!("{key}[{index}]");
        let text = value_string(case, &path, slot)?;
        out[index] = parse_f32_token(case, &path, text)?;
    }
    Ok(out)
}

/// Parses one float-token look angle.
fn parse_angle(case: &FrozenCase, input: &JsonMap, key: &str) -> Result<f32, DispatchError> {
    let text = required_string(case, input, key)?;
    parse_f32_token(case, key, text)
}

/// Parses one exact three-coordinate block position.
fn parse_block_pos(
    case: &FrozenCase,
    input: &JsonMap,
    key: &str,
) -> Result<[i32; 3], DispatchError> {
    let values = required_array(case, input, key)?;
    let slots = exact_array::<3>(case, key, values)?;
    let mut out = [0i32; 3];
    for (index, slot) in slots.iter().enumerate() {
        let path = format!("{key}[{index}]");
        out[index] = value_i32(case, &path, slot)?;
    }
    Ok(out)
}

/// Selects the first rule one raw player publication breaks, in the same
/// order the Go validator checks them: the dimension, the three finite float
/// fields, the bounded survival scalars, the day phase offset, the two
/// enumerated world scalars, the armor points, and finally the mining union,
/// whose inactive member has to be entirely empty and whose active member
/// needs a progress strictly inside its requirement.
///
/// The raw precedence is what disambiguates shared `DomainError` variants:
/// four rules share `InvalidSurvivalValue`, two share `InvalidMiningState`,
/// and the two vector rules share the finite-vector error. The field, not
/// the error, names the published rule.
fn player_state_rejection(raw: &RawPlayerState) -> Option<Rejection> {
    if raw.dimension != DIMENSION_OVERWORLD && raw.dimension != DIMENSION_DEPTHS {
        // A fitting byte reaches `Dimension::new` and is verified there; a
        // negative or above-byte dimension cannot enter any Rust value, so
        // only the classifier decides.
        return Some(match u8::try_from(raw.dimension) {
            Ok(_) => Rejection {
                category: "invalid-enum",
                rule: "player_state.dimension",
                error: Some(DomainError::InvalidDimension),
            },
            Err(_) => Rejection::classifier("invalid-enum", "player_state.dimension"),
        });
    }
    if raw.position.iter().any(|value| !value.is_finite()) {
        // Vector finiteness reports `NonFiniteValue`; the rotation slots keep
        // the narrower `NonFiniteRotation` error.
        return Some(Rejection {
            category: "invalid-value",
            rule: "player_state.finite_position",
            error: Some(DomainError::NonFiniteValue),
        });
    }
    if raw.velocity.iter().any(|value| !value.is_finite()) {
        return Some(Rejection {
            category: "invalid-value",
            rule: "player_state.finite_velocity",
            error: Some(DomainError::NonFiniteValue),
        });
    }
    if !raw.yaw.is_finite() || !raw.pitch.is_finite() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "player_state.finite_rotation",
            error: Some(DomainError::NonFiniteRotation),
        });
    }
    if raw.health > MAX_HEALTH {
        return Some(Rejection {
            category: "invalid-value",
            rule: "player_state.health_range",
            error: Some(DomainError::InvalidSurvivalValue),
        });
    }
    if raw.oxygen > MAX_OXYGEN {
        return Some(Rejection {
            category: "invalid-value",
            rule: "player_state.oxygen_range",
            error: Some(DomainError::InvalidSurvivalValue),
        });
    }
    if raw.hunger > MAX_HUNGER {
        return Some(Rejection {
            category: "invalid-value",
            rule: "player_state.hunger_range",
            error: Some(DomainError::InvalidSurvivalValue),
        });
    }
    if raw.day_phase_offset >= DAY_LENGTH_TICKS {
        return Some(Rejection {
            category: "invalid-value",
            rule: "player_state.day_phase_offset_range",
            error: Some(DomainError::InvalidDayPhaseOffset),
        });
    }
    if raw.weather_kind > WEATHER_THUNDER {
        return Some(Rejection {
            category: "invalid-enum",
            rule: "player_state.weather_kind",
            error: Some(DomainError::InvalidWeather),
        });
    }
    if raw.season > SEASON_WINTER {
        return Some(Rejection {
            category: "invalid-enum",
            rule: "player_state.season",
            error: Some(DomainError::InvalidSeason),
        });
    }
    if raw.armor_points > MAX_ARMOR_POINTS {
        return Some(Rejection {
            category: "invalid-value",
            rule: "player_state.armor_points_range",
            error: Some(DomainError::InvalidSurvivalValue),
        });
    }
    if !raw.mining_active {
        if raw.mining_target != [0, 0, 0]
            || raw.mining_progress != 0
            || raw.mining_required != 0
            || raw.mining_harvestable
        {
            return Some(Rejection {
                category: "invalid-value",
                rule: "player_state.mining_inactive_residue",
                error: Some(DomainError::InvalidMiningState),
            });
        }
        return None;
    }
    if raw.mining_progress == 0 || raw.mining_progress >= raw.mining_required {
        return Some(Rejection {
            category: "invalid-value",
            rule: "player_state.mining_progress_range",
            error: Some(DomainError::InvalidMiningState),
        });
    }
    None
}

/// Verifies the constructor agreement for one classified player-state
/// rejection: the constructor that owns the classified rule must fail with
/// exactly the mapped error, otherwise the Go rule and the Rust constructor
/// disagree and the topic fails closed.
fn verify_player_state_rejection(
    case: &FrozenCase,
    raw: &RawPlayerState,
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed = match rejection.rule {
        "player_state.dimension" => Dimension::new(raw.dimension as u8).map(|_| ()),
        "player_state.finite_position" => FiniteVec3::try_new(raw.position).map(|_| ()),
        "player_state.finite_velocity" => FiniteVec3::try_new(raw.velocity).map(|_| ()),
        "player_state.finite_rotation" => LookAngles::try_new(raw.yaw, raw.pitch).map(|_| ()),
        "player_state.health_range"
        | "player_state.oxygen_range"
        | "player_state.hunger_range"
        | "player_state.armor_points_range" => SurvivalState::try_new(SurvivalStateParts {
            health: raw.health,
            oxygen: raw.oxygen,
            hunger: raw.hunger,
            saturation_zero: raw.saturation_zero,
            armor_points: raw.armor_points,
        })
        .map(|_| ()),
        "player_state.day_phase_offset_range" => world_state_checked(raw).map(|_| ()),
        "player_state.weather_kind" => Weather::try_new(raw.weather_kind).map(|_| ()),
        "player_state.season" => Season::try_new(raw.season).map(|_| ()),
        "player_state.mining_inactive_residue" | "player_state.mining_progress_range" => {
            MiningState::try_new(MiningStateParts {
                active: raw.mining_active,
                target: BlockPos::new(
                    raw.mining_target[0],
                    raw.mining_target[1],
                    raw.mining_target[2],
                ),
                progress: raw.mining_progress,
                required: raw.mining_required,
                harvestable: raw.mining_harvestable,
            })
            .map(|_| ())
        }
        _ => {
            return Err(classification_conflict(
                case,
                "player-state",
                rejection.rule,
            ));
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(
            case,
            "player-state",
            rejection.rule,
        )),
    }
}

/// Constructs the full checked chain for one admitted publication: the
/// dimension, both motion vectors, the look angles, the mining union, the
/// survival and world records, and finally the total `PlayerState::new`
/// aggregate over the already-validated parts.
fn construct_player_state(
    case: &FrozenCase,
    raw: &RawPlayerState,
) -> Result<PlayerState, DispatchError> {
    let conflict = |_: DomainError| classification_conflict(case, "player-state", "");
    let dimension = Dimension::new(raw.dimension as u8).map_err(conflict)?;
    let position = FiniteVec3::try_new(raw.position).map_err(conflict)?;
    let velocity = FiniteVec3::try_new(raw.velocity).map_err(conflict)?;
    let look = LookAngles::try_new(raw.yaw, raw.pitch).map_err(conflict)?;
    let motion = MotionState::new(MotionStateParts {
        position,
        velocity,
        on_ground: raw.on_ground,
    });
    let mining = MiningState::try_new(MiningStateParts {
        active: raw.mining_active,
        target: BlockPos::new(
            raw.mining_target[0],
            raw.mining_target[1],
            raw.mining_target[2],
        ),
        progress: raw.mining_progress,
        required: raw.mining_required,
        harvestable: raw.mining_harvestable,
    })
    .map_err(conflict)?;
    let survival = SurvivalState::try_new(SurvivalStateParts {
        health: raw.health,
        oxygen: raw.oxygen,
        hunger: raw.hunger,
        saturation_zero: raw.saturation_zero,
        armor_points: raw.armor_points,
    })
    .map_err(conflict)?;
    let world = world_state_checked(raw).map_err(conflict)?;
    Ok(PlayerState::new(PlayerStateParts {
        server_tick: raw.server_tick,
        last_input_sequence: raw.last_input_sequence,
        dimension,
        motion,
        look,
        ready: raw.ready,
        reset: raw.reset,
        mining,
        survival,
        world,
    }))
}

/// Constructs the world record, whose weather and season parts are checked
/// enums the record carries as already-validated values.
fn world_state_checked(raw: &RawPlayerState) -> Result<WorldState, DomainError> {
    let weather = Weather::try_new(raw.weather_kind)?;
    let season = Season::try_new(raw.season)?;
    WorldState::try_new(WorldStateParts {
        day_phase_offset: raw.day_phase_offset,
        world_time_ticks: raw.world_time_ticks,
        weather,
        season,
        season_progress: raw.season_progress,
        temperature: raw.temperature,
    })
}

/// Builds the rejected player-state projection from the parsed input, because
/// a rejected Rust value has no getters to read.
fn player_state_projection(raw: &RawPlayerState) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("server_tick".to_string(), normalize_u64(raw.server_tick));
    fields.insert(
        "last_input_sequence".to_string(),
        normalize_u64(raw.last_input_sequence),
    );
    fields.insert("dimension".to_string(), Value::from(raw.dimension));
    fields.insert("position".to_string(), vec_bits(raw.position));
    fields.insert("velocity".to_string(), vec_bits(raw.velocity));
    fields.insert("yaw".to_string(), normalize_f32(raw.yaw));
    fields.insert("pitch".to_string(), normalize_f32(raw.pitch));
    fields.insert("on_ground".to_string(), Value::Bool(raw.on_ground));
    fields.insert("ready".to_string(), Value::Bool(raw.ready));
    fields.insert("reset".to_string(), Value::Bool(raw.reset));
    fields.insert("mining_active".to_string(), Value::Bool(raw.mining_active));
    fields.insert(
        "mining_target".to_string(),
        Value::Array(
            raw.mining_target
                .iter()
                .map(|coordinate| Value::from(*coordinate))
                .collect(),
        ),
    );
    fields.insert(
        "mining_progress".to_string(),
        Value::from(raw.mining_progress),
    );
    fields.insert(
        "mining_required".to_string(),
        Value::from(raw.mining_required),
    );
    fields.insert(
        "mining_harvestable".to_string(),
        Value::Bool(raw.mining_harvestable),
    );
    fields.insert("health".to_string(), Value::from(raw.health));
    fields.insert("oxygen".to_string(), Value::from(raw.oxygen));
    fields.insert("hunger".to_string(), Value::from(raw.hunger));
    fields.insert(
        "saturation_zero".to_string(),
        Value::Bool(raw.saturation_zero),
    );
    fields.insert(
        "day_phase_offset".to_string(),
        Value::from(raw.day_phase_offset),
    );
    fields.insert(
        "world_time_ticks".to_string(),
        normalize_u64(raw.world_time_ticks),
    );
    fields.insert("weather_kind".to_string(), Value::from(raw.weather_kind));
    fields.insert("season".to_string(), Value::from(raw.season));
    fields.insert(
        "season_progress".to_string(),
        Value::from(raw.season_progress),
    );
    fields.insert("temperature".to_string(), Value::from(raw.temperature));
    fields.insert("armor_points".to_string(), Value::from(raw.armor_points));
    fields
}

/// Builds the accepted player-state projection from the constructed record's
/// getters. An idle mining block projects the canonical empty block the
/// `MiningState` constructor pins; an active block reads its four fields.
fn player_state_fields(state: &PlayerState) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert(
        "server_tick".to_string(),
        normalize_u64(state.server_tick()),
    );
    fields.insert(
        "last_input_sequence".to_string(),
        normalize_u64(state.last_input_sequence()),
    );
    fields.insert(
        "dimension".to_string(),
        Value::from(state.dimension().get()),
    );
    let motion = state.motion();
    fields.insert("position".to_string(), vec_bits(motion.position().get()));
    fields.insert("velocity".to_string(), vec_bits(motion.velocity().get()));
    let look = state.look();
    fields.insert("yaw".to_string(), normalize_f32(look.yaw()));
    fields.insert("pitch".to_string(), normalize_f32(look.pitch()));
    fields.insert("on_ground".to_string(), Value::Bool(motion.on_ground()));
    fields.insert("ready".to_string(), Value::Bool(state.ready()));
    fields.insert("reset".to_string(), Value::Bool(state.reset()));
    match state.mining() {
        MiningState::Idle => {
            fields.insert("mining_active".to_string(), Value::Bool(false));
            let origin = BlockPos::ORIGIN;
            fields.insert(
                "mining_target".to_string(),
                Value::Array(vec![
                    Value::from(origin.x()),
                    Value::from(origin.y()),
                    Value::from(origin.z()),
                ]),
            );
            fields.insert("mining_progress".to_string(), Value::from(0));
            fields.insert("mining_required".to_string(), Value::from(0));
            fields.insert("mining_harvestable".to_string(), Value::Bool(false));
        }
        MiningState::Active(active) => {
            fields.insert("mining_active".to_string(), Value::Bool(true));
            let target = active.target();
            fields.insert(
                "mining_target".to_string(),
                Value::Array(vec![
                    Value::from(target.x()),
                    Value::from(target.y()),
                    Value::from(target.z()),
                ]),
            );
            fields.insert(
                "mining_progress".to_string(),
                Value::from(active.progress()),
            );
            fields.insert(
                "mining_required".to_string(),
                Value::from(active.required()),
            );
            fields.insert(
                "mining_harvestable".to_string(),
                Value::Bool(active.harvestable()),
            );
        }
    }
    let survival = state.survival();
    fields.insert("health".to_string(), Value::from(survival.health()));
    fields.insert("oxygen".to_string(), Value::from(survival.oxygen()));
    fields.insert("hunger".to_string(), Value::from(survival.hunger()));
    fields.insert(
        "saturation_zero".to_string(),
        Value::Bool(survival.saturation_zero()),
    );
    let world = state.world();
    fields.insert(
        "day_phase_offset".to_string(),
        Value::from(world.day_phase_offset()),
    );
    fields.insert(
        "world_time_ticks".to_string(),
        normalize_u64(world.world_time_ticks()),
    );
    fields.insert(
        "weather_kind".to_string(),
        Value::from(world.weather().wire_id()),
    );
    fields.insert("season".to_string(), Value::from(world.season().wire_id()));
    fields.insert(
        "season_progress".to_string(),
        Value::from(world.season_progress()),
    );
    fields.insert("temperature".to_string(), Value::from(world.temperature()));
    fields.insert(
        "armor_points".to_string(),
        Value::from(survival.armor_points()),
    );
    fields
}

/// Renders one three-component vector as the hexadecimal IEEE 754 bit strings
/// the normalized corpus vocabulary uses, so signed zeros stay observable.
fn vec_bits(components: [f32; 3]) -> Value {
    Value::Array(
        components
            .iter()
            .map(|value| normalize_f32(*value))
            .collect(),
    )
}

/// Maps one published reason name to its `RejectReason` variant. The names
/// are the frozen Go `protocol.RejectReason` strings.
fn reason_variant(text: &str) -> Option<RejectReason> {
    match text {
        "invalid_ray" => Some(RejectReason::InvalidRay),
        "no_target" => Some(RejectReason::NoTarget),
        "chunk_not_ready" => Some(RejectReason::ChunkNotReady),
        "protected_block" => Some(RejectReason::ProtectedBlock),
        "invalid_block" => Some(RejectReason::InvalidBlock),
        "occupied" => Some(RejectReason::Occupied),
        "invalid_input" => Some(RejectReason::InvalidInput),
        "player_not_ready" => Some(RejectReason::PlayerNotReady),
        "invalid_slot" => Some(RejectReason::InvalidSlot),
        "hotbar_full" => Some(RejectReason::HotbarFull),
        "drop_capacity" => Some(RejectReason::DropCapacity),
        "container_capacity" => Some(RejectReason::ContainerCapacity),
        "not_fluid_source" => Some(RejectReason::NotFluidSource),
        "bucket_mismatch" => Some(RejectReason::BucketMismatch),
        "not_armor" => Some(RejectReason::NotArmor),
        _ => None,
    }
}

/// The exhaustive inverse of `reason_variant`: the stable text one
/// constructed variant publishes, from a match rather than the raw input.
fn reason_text(reason: RejectReason) -> &'static str {
    match reason {
        RejectReason::InvalidRay => "invalid_ray",
        RejectReason::NoTarget => "no_target",
        RejectReason::ChunkNotReady => "chunk_not_ready",
        RejectReason::ProtectedBlock => "protected_block",
        RejectReason::InvalidBlock => "invalid_block",
        RejectReason::Occupied => "occupied",
        RejectReason::InvalidInput => "invalid_input",
        RejectReason::PlayerNotReady => "player_not_ready",
        RejectReason::InvalidSlot => "invalid_slot",
        RejectReason::HotbarFull => "hotbar_full",
        RejectReason::DropCapacity => "drop_capacity",
        RejectReason::ContainerCapacity => "container_capacity",
        RejectReason::NotFluidSource => "not_fluid_source",
        RejectReason::BucketMismatch => "bucket_mismatch",
        RejectReason::NotArmor => "not_armor",
    }
}

/// Selects the first rule one combat hit breaks, in the same order the Go
/// validator checks them: the tick, the damage, then the target kind.
fn combat_hit_rejection(server_tick: u64, damage: u8, target_kind: u8) -> Option<Rejection> {
    if server_tick == 0 {
        return Some(Rejection {
            category: "invalid-value",
            rule: "combat_hit.server_tick_nonzero",
            error: Some(DomainError::InvalidCombatHit),
        });
    }
    if damage == 0 || damage > MAX_HEALTH {
        return Some(Rejection {
            category: "invalid-value",
            rule: "combat_hit.damage_range",
            error: Some(DomainError::InvalidCombatHit),
        });
    }
    if CombatTarget::try_new(target_kind).is_err() {
        return Some(Rejection {
            category: "invalid-enum",
            rule: "combat_hit.target_kind",
            error: Some(DomainError::InvalidCombatTarget),
        });
    }
    None
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
fn event_player_execute_47_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}

/// Proves the shared comparator actually compares semantic fields: the
/// produced accepted result matches its frozen expectation, while the same
/// actual value no longer matches once one normalized field is mutated.
#[test]
fn event_player_comparator_detects_semantic_mutation() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    let accepted = executed
        .into_iter()
        .find(|case| {
            case.actual.get("kind").and_then(|kind| kind.as_str()) == Some("ok")
                && case
                    .actual
                    .get("category")
                    .and_then(|category| category.as_str())
                    == Some("player-state")
        })
        .expect("one accepted player-state case");

    assert_domain_normalized(&accepted);

    let mut mutated_case = accepted.case;
    let fields = mutated_case
        .normalized
        .get_mut("fields")
        .and_then(|fields| fields.as_object_mut())
        .expect("normalized fields object");
    fields.insert("health".to_string(), Value::from(999));

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
