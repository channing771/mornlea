//! Topic module for domain.event remote-player/companion corpus cases.
//!
//! The six rules here are the visibility projections an authoritative session
//! publishes about the other players and the companions one subscriber can
//! see: each subject has a spawn, a per-tick state batch, and a despawn. Each
//! executor parses the case input, selects the Go rejection rule from the raw
//! values in the producer's stated precedence, and then verifies that the
//! applicable Rust constructor returns the mapped `DomainError`: a classified
//! acceptance must construct, and a classified rejection must fail with
//! exactly the mapped variant. Accepted fields are read from the constructed
//! Rust values; a rejected value has no getters, so its projection is built
//! from the parsed input instead.
//!
//! Two protocol budgets deliberately stay out of this module, exactly as they
//! stay out of the domain constructors: the seven-record remote-player and
//! four-record companion wire maxima are transport budgets, and no frozen case
//! exercises them. Submitted record order is never sorted: the Rust batch
//! constructor verifies that submitted identities are already strictly
//! increasing, and the classifier replays that check per adjacent pair in
//! input order, after each otherwise valid record. The per-record rule
//! differences are the Go ones and are kept exactly: a remote-player record
//! accepts either playable dimension and any finite pitch, while a companion
//! record accepts the overworld alone and a pitch inside the inclusive
//! vertical look range.

use super::support::{
    DispatchError, ExecutedCase, JsonMap, assert_domain_normalized, exact_array, execute_topic,
    input_object, invalid_case, normalize_f32, normalize_u64, normalize_uuid, normalized_error,
    normalized_ok, parse_f32_token, parse_uuid_hex, required_array, required_bool, required_i32,
    required_string, required_u64, value_string,
};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};
use mornlea_domain::{
    CompanionDespawn, CompanionId, CompanionName, CompanionSpawn, CompanionSpawnParts,
    CompanionState, CompanionStateParts, CompanionStates, CompanionStatesParts, Dimension,
    DisplayName, DomainError, FiniteVec3, LookAngles, PlayerId, RemotePlayerDespawn,
    RemotePlayerSpawn, RemotePlayerSpawnParts, RemotePlayerState, RemotePlayerStateParts,
    RemotePlayerStates, RemotePlayerStatesParts,
};
use serde_json::Value;

pub const EXPECTED_COUNT: usize = 46;

const RULES: &[&str] = &[
    "remote-player-spawn",
    "remote-player-despawn",
    "remote-player-states",
    "companion-spawn",
    "companion-states",
    "companion-despawn",
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
        "remote-player-spawn" => execute_remote_player_spawn(case, input),
        "remote-player-despawn" => execute_remote_player_despawn(case, input),
        "remote-player-states" => execute_remote_player_states(case, input),
        "companion-spawn" => execute_companion_spawn(case, input),
        "companion-states" => execute_companion_states(case, input),
        "companion-despawn" => execute_companion_despawn(case, input),
        unknown => Err(invalid_case(
            case,
            format!("unknown people event rule '{unknown}'"),
        )),
    }
}

/// One classified rejection: its normalized category, its exact Go rule name,
/// and the `DomainError` the owning constructor must return.
///
/// The rule is a `String` because the batch rules are indexed by record
/// (`remote_player_states.player_1.position_finite`). `error` is `None` for a
/// classifier-only rejection whose bad raw value cannot enter any Rust type,
/// so no constructor verdict exists to check.
struct Rejection {
    category: &'static str,
    rule: String,
    error: Option<DomainError>,
}

/// Canonical display-name bounds, from the Go `core.NormalizeDisplayName`
/// admission rule. They are private in the crate, so this module names them
/// for the classifier it owns; the constructors remain the admission
/// authority whose verdict is cross-checked.
const DISPLAY_NAME_MAX_SCALARS: usize = 32;
const DISPLAY_NAME_MAX_BYTES: usize = 128;

/// Inclusive vertical look limit of a companion record, from the Go
/// `validCompanionPose` bound `math.Pi/2`. It is a companion rule only: a
/// remote-player record publishes no pitch bound at all.
const COMPANION_PITCH_LIMIT: f32 = core::f32::consts::FRAC_PI_2;

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

/// The pinned control-character predicate the text rules consult, matching
/// the domain crate's explicit ranges so classification cannot drift with a
/// Unicode-version bump.
fn is_pinned_control(ch: char) -> bool {
    matches!(ch, '\u{0000}'..='\u{001F}' | '\u{007F}'..='\u{009F}')
}

/// The pinned whitespace predicate behind the surrounding-whitespace and
/// embedded-whitespace rules, matching the domain crate's explicit ranges.
fn is_pinned_whitespace(ch: char) -> bool {
    matches!(
        ch,
        '\u{0009}'..='\u{000D}'
            | '\u{0020}'
            | '\u{0085}'
            | '\u{00A0}'
            | '\u{1680}'
            | '\u{2000}'..='\u{200A}'
            | '\u{2028}'
            | '\u{2029}'
            | '\u{202F}'
            | '\u{205F}'
            | '\u{3000}'
    )
}

/// The identity rule suffix one raw 16-byte value breaks, in the Go
/// validator's order: nonzero first, then the version nibble, then the
/// RFC-4122 variant. Callers compose the suffix with their rule's own
/// subject prefix. `None` means the identity is publishable.
fn identity_rule(bytes: [u8; 16]) -> Option<&'static str> {
    if bytes == [0u8; 16] {
        Some("nonzero")
    } else if bytes[6] >> 4 != 4 {
        Some("version_v4")
    } else if bytes[8] & 0xc0 != 0x80 {
        Some("variant_rfc4122")
    } else {
        None
    }
}

/// The display-name rule suffix one canonical text breaks, in the Go
/// validator's order. Invalid UTF-8 cannot occur in a Rust `String` or frozen
/// JSON but remains the closed producer rule, so the `utf8` suffix stays in
/// the classification table without ever being produced here.
fn display_name_rule(text: &str) -> Option<&'static str> {
    let scalars = text.chars().count();
    if !(1..=DISPLAY_NAME_MAX_SCALARS).contains(&scalars) || text.len() > DISPLAY_NAME_MAX_BYTES {
        Some("length_range")
    } else if text.chars().any(is_pinned_control) {
        Some("control")
    } else if text.chars().next().is_some_and(is_pinned_whitespace)
        || text.chars().next_back().is_some_and(is_pinned_whitespace)
    {
        Some("canonical_trim")
    } else {
        None
    }
}

/// The name rule one companion name breaks: the display-name rules first,
/// then the companion-only embedded-whitespace rule.
fn companion_name_rule(text: &str) -> Option<&'static str> {
    if let Some(suffix) = display_name_rule(text) {
        return Some(suffix);
    }
    if text.chars().any(is_pinned_whitespace) {
        return Some("companion_name.embedded_whitespace");
    }
    None
}

/// Reports whether one pitch is inside the inclusive companion vertical look
/// range, on the exact `f32` the wire carries.
fn companion_pitch_admits(pitch: f32) -> bool {
    (-COMPANION_PITCH_LIMIT..=COMPANION_PITCH_LIMIT).contains(&pitch)
}

/// Admits one raw dimension for a remote-player record: either playable
/// dimension passes, any other fitting byte rejects with `InvalidDimension`,
/// and a value outside the byte range cannot enter the constructor, so only
/// the classifier decides.
fn admit_remote_dimension(raw: i32) -> Result<u8, Option<DomainError>> {
    match u8::try_from(raw) {
        Ok(byte @ (0 | 1)) => Ok(byte),
        Ok(_) => Err(Some(DomainError::InvalidDimension)),
        Err(_) => Err(None),
    }
}

/// Admits one raw dimension for a companion record: the overworld alone, the
/// depths reject with `InvalidCompanionDimension`, any other fitting byte
/// with `InvalidDimension`, and a value outside the byte range stays a
/// classifier-only rejection.
fn admit_companion_dimension(raw: i32) -> Result<u8, Option<DomainError>> {
    match u8::try_from(raw) {
        Ok(0) => Ok(0),
        Ok(1) => Err(Some(DomainError::InvalidCompanionDimension)),
        Ok(_) => Err(Some(DomainError::InvalidDimension)),
        Err(_) => Err(None),
    }
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

/// Parses one float-token look angle field.
fn parse_angle(case: &FrozenCase, object: &JsonMap, key: &str) -> Result<f32, DispatchError> {
    parse_f32_token(case, key, required_string(case, object, key)?)
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

/// Renders one look angle pair in the normalized bit-exact form.
fn look_value(yaw: f32, pitch: f32) -> Value {
    serde_json::json!({ "yaw": normalize_f32(yaw), "pitch": normalize_f32(pitch) })
}

/// The parsed raw shape of one remote-player spawn.
struct RawRemoteSpawn {
    player_id: [u8; 16],
    display_name: String,
    server_tick: u64,
    dimension: i32,
    position: [f32; 3],
    yaw: f32,
    pitch: f32,
}

/// Executes one `remote-player-spawn` case.
///
/// The rule is selected from the raw values in the Go validator's precedence:
/// the display name, the identity, the dimension, the position, and finally
/// the rotation. A remote-player spawn publishes no pitch bound, so a pitch
/// far outside the vertical look range stays publishable here.
fn execute_remote_player_spawn(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "remote-player-spawn";
    let raw = RawRemoteSpawn {
        player_id: parse_uuid_hex(
            case,
            "player_id",
            required_string(case, input, "player_id")?,
        )?,
        display_name: required_string(case, input, "display_name")?.to_string(),
        server_tick: required_u64(case, input, "server_tick")?,
        dimension: required_i32(case, input, "dimension")?,
        position: parse_position(case, input)?,
        yaw: parse_angle(case, input, "yaw")?,
        pitch: parse_angle(case, input, "pitch")?,
    };

    match remote_spawn_rejection(&raw) {
        Some(rejection) => {
            verify_remote_spawn_rejection(case, subject, &raw, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert("player_id".to_string(), normalize_uuid(raw.player_id));
            fields.insert(
                "display_name".to_string(),
                Value::String(raw.display_name.clone()),
            );
            fields.insert("server_tick".to_string(), normalize_u64(raw.server_tick));
            fields.insert("dimension".to_string(), Value::from(raw.dimension));
            fields.insert("position".to_string(), position_value(raw.position));
            fields.insert("look".to_string(), look_value(raw.yaw, raw.pitch));
            normalized_error(case, rejection.category, &rejection.rule, fields)
        }
        None => {
            let spawn = construct_remote_spawn(case, subject, raw)?;
            let mut fields = JsonMap::new();
            fields.insert(
                "player_id".to_string(),
                normalize_uuid(spawn.player_id().bytes()),
            );
            fields.insert(
                "display_name".to_string(),
                Value::String(spawn.display_name().as_str().to_string()),
            );
            fields.insert(
                "server_tick".to_string(),
                normalize_u64(spawn.server_tick()),
            );
            fields.insert(
                "dimension".to_string(),
                Value::from(spawn.dimension().get()),
            );
            fields.insert(
                "position".to_string(),
                position_value(spawn.position().get()),
            );
            fields.insert(
                "look".to_string(),
                look_value(spawn.look().yaw(), spawn.look().pitch()),
            );
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// Selects the first rule one remote-player spawn breaks, in the Go
/// validator's precedence: the display name fires before the identity.
fn remote_spawn_rejection(raw: &RawRemoteSpawn) -> Option<Rejection> {
    if let Some(suffix) = display_name_rule(&raw.display_name) {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("remote_player_spawn.display_name.{suffix}"),
            error: Some(DomainError::InvalidText),
        });
    }
    if let Some(suffix) = identity_rule(raw.player_id) {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("remote_player_spawn.player_id.{suffix}"),
            error: Some(DomainError::InvalidIdentity),
        });
    }
    if let Err(error) = admit_remote_dimension(raw.dimension) {
        return Some(Rejection {
            category: "invalid-enum",
            rule: "remote_player_spawn.dimension".to_string(),
            error,
        });
    }
    if raw.position.iter().any(|component| !component.is_finite()) {
        return Some(Rejection {
            category: "invalid-value",
            rule: "remote_player_spawn.position_finite".to_string(),
            error: Some(DomainError::NonFiniteRotation),
        });
    }
    if !raw.yaw.is_finite() || !raw.pitch.is_finite() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "remote_player_spawn.rotation_finite".to_string(),
            error: Some(DomainError::NonFiniteRotation),
        });
    }
    None
}

/// Verifies the constructor agreement for one classified remote-player spawn
/// rejection.
fn verify_remote_spawn_rejection(
    case: &FrozenCase,
    subject: &str,
    raw: &RawRemoteSpawn,
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "remote_player_spawn.player_id.nonzero"
        | "remote_player_spawn.player_id.version_v4"
        | "remote_player_spawn.player_id.variant_rfc4122" => {
            PlayerId::try_from_bytes(raw.player_id).map(|_| ())
        }
        "remote_player_spawn.display_name.utf8"
        | "remote_player_spawn.display_name.length_range"
        | "remote_player_spawn.display_name.control"
        | "remote_player_spawn.display_name.canonical_trim" => {
            DisplayName::try_from_canonical(raw.display_name.clone()).map(|_| ())
        }
        "remote_player_spawn.dimension" => {
            // Only a fitting unknown byte reaches here with an expected
            // error; a value outside the byte range is classifier-only.
            let byte = u8::try_from(raw.dimension)
                .map_err(|_| classification_conflict(case, subject, &rejection.rule))?;
            Dimension::new(byte).map(|_| ())
        }
        "remote_player_spawn.position_finite" => FiniteVec3::try_new(raw.position).map(|_| ()),
        "remote_player_spawn.rotation_finite" => {
            LookAngles::try_new(raw.yaw, raw.pitch).map(|_| ())
        }
        _ => return Err(classification_conflict(case, subject, &rejection.rule)),
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Constructs the whole checked remote-player spawn chain for one admitted
/// case, failing closed if any already-cleared stage rejects.
fn construct_remote_spawn(
    case: &FrozenCase,
    subject: &str,
    raw: RawRemoteSpawn,
) -> Result<RemotePlayerSpawn, DispatchError> {
    let display_name = DisplayName::try_from_canonical(raw.display_name)
        .map_err(|error| construction_conflict(case, subject, error))?;
    let player_id = PlayerId::try_from_bytes(raw.player_id)
        .map_err(|error| construction_conflict(case, subject, error))?;
    let byte = u8::try_from(raw.dimension)
        .map_err(|_| classification_conflict(case, subject, "dimension outside the byte range"))?;
    let dimension =
        Dimension::new(byte).map_err(|error| construction_conflict(case, subject, error))?;
    let position = FiniteVec3::try_new(raw.position)
        .map_err(|error| construction_conflict(case, subject, error))?;
    let look = LookAngles::try_new(raw.yaw, raw.pitch)
        .map_err(|error| construction_conflict(case, subject, error))?;
    Ok(RemotePlayerSpawn::new(RemotePlayerSpawnParts {
        player_id,
        display_name,
        server_tick: raw.server_tick,
        dimension,
        position,
        look,
    }))
}

/// Executes one `remote-player-despawn` case, whose entire payload is the
/// checked identity. The producer intentionally reuses the shared identity
/// rule name without a subject prefix.
fn execute_remote_player_despawn(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "remote-player-despawn";
    let player_id = parse_uuid_hex(
        case,
        "player_id",
        required_string(case, input, "player_id")?,
    )?;
    match identity_rule(player_id) {
        Some(suffix) => {
            let rule = format!("player_id.{suffix}");
            if PlayerId::try_from_bytes(player_id).map(|_| ()) != Err(DomainError::InvalidIdentity)
            {
                return Err(classification_conflict(case, subject, &rule));
            }
            let mut fields = JsonMap::new();
            fields.insert("player_id".to_string(), normalize_uuid(player_id));
            normalized_error(case, "invalid-identity", &rule, fields)
        }
        None => {
            let despawn = RemotePlayerDespawn::new(
                PlayerId::try_from_bytes(player_id)
                    .map_err(|error| construction_conflict(case, subject, error))?,
            );
            let mut fields = JsonMap::new();
            fields.insert(
                "player_id".to_string(),
                normalize_uuid(despawn.player_id().bytes()),
            );
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// Executes one `companion-despawn` case, whose entire payload is the checked
/// identity, with the same shared identity rule naming as the remote-player
/// despawn.
fn execute_companion_despawn(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "companion-despawn";
    let id = parse_uuid_hex(case, "id", required_string(case, input, "id")?)?;
    match identity_rule(id) {
        Some(suffix) => {
            let rule = format!("player_id.{suffix}");
            if CompanionId::try_from_bytes(id).map(|_| ()) != Err(DomainError::InvalidIdentity) {
                return Err(classification_conflict(case, subject, &rule));
            }
            let mut fields = JsonMap::new();
            fields.insert("id".to_string(), normalize_uuid(id));
            normalized_error(case, "invalid-identity", &rule, fields)
        }
        None => {
            let despawn = CompanionDespawn::new(
                CompanionId::try_from_bytes(id)
                    .map_err(|error| construction_conflict(case, subject, error))?,
            );
            let mut fields = JsonMap::new();
            fields.insert("id".to_string(), normalize_uuid(despawn.id().bytes()));
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// The parsed raw shape of one remote-player body record inside a batch.
struct RawRemoteState {
    player_id: [u8; 16],
    dimension: i32,
    position: [f32; 3],
    yaw: f32,
    pitch: f32,
    reset: bool,
}

/// Parses one required ordered batch of remote-player body records.
fn parse_remote_states(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<Vec<RawRemoteState>, DispatchError> {
    required_array(case, input, "players")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("players[{index}]");
            let object = super::support::value_object(case, &path, value)?;
            Ok(RawRemoteState {
                player_id: parse_uuid_hex(
                    case,
                    &format!("{path}.player_id"),
                    required_string(case, object, "player_id")?,
                )?,
                dimension: required_i32(case, object, "dimension")?,
                position: parse_position(case, object)?,
                yaw: parse_angle(case, object, "yaw")?,
                pitch: parse_angle(case, object, "pitch")?,
                reset: required_bool(case, object, "reset")?,
            })
        })
        .collect()
}

/// Executes one `remote-player-states` case, whose payload is the publish
/// tick and the ordered body batch.
///
/// The walk replays the Go validator exactly: each record's identity,
/// dimension, position and rotation rules run in input order, and the
/// strictly-increasing rule runs after each otherwise valid record against
/// the previous identity. Submitted order is never sorted.
fn execute_remote_player_states(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "remote-player-states";
    let server_tick = required_u64(case, input, "server_tick")?;
    let states = parse_remote_states(case, input)?;

    match remote_states_rejection(&states) {
        Some(rejection) => {
            verify_remote_states_rejection(case, subject, server_tick, &states, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert("server_tick".to_string(), normalize_u64(server_tick));
            fields.insert(
                "states".to_string(),
                Value::Array(
                    states
                        .iter()
                        .map(|state| {
                            serde_json::json!({
                                "player_id": normalize_uuid(state.player_id),
                                "dimension": state.dimension,
                                "position": position_value(state.position),
                                "look": look_value(state.yaw, state.pitch),
                                "reset": state.reset,
                            })
                        })
                        .collect(),
                ),
            );
            normalized_error(case, rejection.category, &rejection.rule, fields)
        }
        None => {
            let mut records = Vec::with_capacity(states.len());
            for state in &states {
                records.push(construct_remote_state(case, subject, state)?);
            }
            let batch = RemotePlayerStates::try_new(RemotePlayerStatesParts {
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
                        .map(|state| {
                            serde_json::json!({
                                "player_id": normalize_uuid(state.player_id().bytes()),
                                "dimension": state.dimension().get(),
                                "position": position_value(state.position().get()),
                                "look": look_value(state.look().yaw(), state.look().pitch()),
                                "reset": state.reset(),
                            })
                        })
                        .collect(),
                ),
            );
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// Selects the first rule one remote-player batch breaks, in the Go
/// validator's walk order.
fn remote_states_rejection(states: &[RawRemoteState]) -> Option<Rejection> {
    if states.is_empty() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "remote_player_states.count_range".to_string(),
            error: Some(DomainError::EmptyStateBatch),
        });
    }
    for (index, state) in states.iter().enumerate() {
        if let Some(suffix) = identity_rule(state.player_id) {
            return Some(Rejection {
                category: "invalid-identity",
                rule: format!("remote_player_states.player_{index}.player_id.{suffix}"),
                error: Some(DomainError::InvalidIdentity),
            });
        }
        if let Err(error) = admit_remote_dimension(state.dimension) {
            return Some(Rejection {
                category: "invalid-enum",
                rule: format!("remote_player_states.player_{index}.dimension"),
                error,
            });
        }
        if state
            .position
            .iter()
            .any(|component| !component.is_finite())
        {
            return Some(Rejection {
                category: "invalid-value",
                rule: format!("remote_player_states.player_{index}.position_finite"),
                error: Some(DomainError::NonFiniteRotation),
            });
        }
        if !state.yaw.is_finite() || !state.pitch.is_finite() {
            return Some(Rejection {
                category: "invalid-value",
                rule: format!("remote_player_states.player_{index}.rotation_finite"),
                error: Some(DomainError::NonFiniteRotation),
            });
        }
        if index > 0 && states[index - 1].player_id >= state.player_id {
            return Some(Rejection {
                category: "invalid-value",
                rule: "remote_player_states.strictly_increasing_ids".to_string(),
                error: Some(DomainError::InvalidStateOrder),
            });
        }
    }
    None
}

/// Verifies the constructor agreement for one classified remote-player batch
/// rejection. The empty-batch and order rules are owned by the batch
/// constructor; the per-record rules by the record's own value constructors.
fn verify_remote_states_rejection(
    case: &FrozenCase,
    subject: &str,
    server_tick: u64,
    states: &[RawRemoteState],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "remote_player_states.count_range" => {
            RemotePlayerStates::try_new(RemotePlayerStatesParts {
                server_tick,
                states: Vec::new().into_boxed_slice(),
            })
            .map(|_| ())
        }
        "remote_player_states.strictly_increasing_ids" => {
            // The order rule fires at the first non-increasing adjacent
            // pair, so the constructor verdict is taken on the checked
            // prefix that ends at that pair.
            let pair = states
                .windows(2)
                .position(|pair| pair[0].player_id >= pair[1].player_id)
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let mut records = Vec::new();
            for state in &states[..pair + 2] {
                records.push(construct_remote_state(case, subject, state)?);
            }
            RemotePlayerStates::try_new(RemotePlayerStatesParts {
                server_tick,
                states: records.into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let (index, suffix) = remote_state_rule_parts(case, subject, rule)?;
            let state = states
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let constructed = verify_remote_state_rule(case, subject, state, suffix)?;
            if constructed == Err(expected) {
                return Ok(());
            }
            return Err(classification_conflict(case, subject, &rejection.rule));
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Splits one indexed per-record rule into its record index and rule suffix.
fn remote_state_rule_parts<'a>(
    case: &FrozenCase,
    subject: &str,
    rule: &'a str,
) -> Result<(usize, &'a str), DispatchError> {
    let rest = rule
        .strip_prefix("remote_player_states.player_")
        .ok_or_else(|| classification_conflict(case, subject, rule))?;
    let (index_text, suffix) = rest
        .split_once('.')
        .ok_or_else(|| classification_conflict(case, subject, rule))?;
    let index: usize = index_text
        .parse()
        .map_err(|_| classification_conflict(case, subject, rule))?;
    Ok((index, suffix))
}

/// Runs one per-record remote-player constructor verdict for verification.
fn verify_remote_state_rule(
    case: &FrozenCase,
    subject: &str,
    state: &RawRemoteState,
    suffix: &str,
) -> Result<Result<(), DomainError>, DispatchError> {
    match suffix {
        "player_id.nonzero" | "player_id.version_v4" | "player_id.variant_rfc4122" => {
            Ok(PlayerId::try_from_bytes(state.player_id).map(|_| ()))
        }
        "dimension" => {
            let byte = u8::try_from(state.dimension)
                .map_err(|_| classification_conflict(case, subject, suffix))?;
            Ok(Dimension::new(byte).map(|_| ()))
        }
        "position_finite" => Ok(FiniteVec3::try_new(state.position).map(|_| ())),
        "rotation_finite" => Ok(LookAngles::try_new(state.yaw, state.pitch).map(|_| ())),
        _ => Err(classification_conflict(case, subject, suffix)),
    }
}

/// Constructs one checked remote-player record for an admitted case.
fn construct_remote_state(
    case: &FrozenCase,
    subject: &str,
    raw: &RawRemoteState,
) -> Result<RemotePlayerState, DispatchError> {
    let player_id = PlayerId::try_from_bytes(raw.player_id)
        .map_err(|error| construction_conflict(case, subject, error))?;
    let byte = u8::try_from(raw.dimension)
        .map_err(|_| classification_conflict(case, subject, "dimension outside the byte range"))?;
    let dimension =
        Dimension::new(byte).map_err(|error| construction_conflict(case, subject, error))?;
    let position = FiniteVec3::try_new(raw.position)
        .map_err(|error| construction_conflict(case, subject, error))?;
    let look = LookAngles::try_new(raw.yaw, raw.pitch)
        .map_err(|error| construction_conflict(case, subject, error))?;
    Ok(RemotePlayerState::new(RemotePlayerStateParts {
        player_id,
        dimension,
        position,
        look,
        reset: raw.reset,
    }))
}

/// The parsed raw shape of one companion spawn.
struct RawCompanionSpawn {
    id: [u8; 16],
    name: String,
    tick: u64,
    dimension: i32,
    position: [f32; 3],
    yaw: f32,
    pitch: f32,
}

/// Executes one `companion-spawn` case.
///
/// The rule is selected in the Go validator's precedence: the identity, the
/// companion name, the companion dimension rule, the position, the rotation,
/// and finally the companion-only inclusive pitch bound. The publish tick is
/// read from the input `tick` field and published as `server_tick`.
fn execute_companion_spawn(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "companion-spawn";
    let raw = RawCompanionSpawn {
        id: parse_uuid_hex(case, "id", required_string(case, input, "id")?)?,
        name: required_string(case, input, "name")?.to_string(),
        tick: required_u64(case, input, "tick")?,
        dimension: required_i32(case, input, "dimension")?,
        position: parse_position(case, input)?,
        yaw: parse_angle(case, input, "yaw")?,
        pitch: parse_angle(case, input, "pitch")?,
    };

    match companion_spawn_rejection(&raw) {
        Some(rejection) => {
            verify_companion_spawn_rejection(case, subject, &raw, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert("id".to_string(), normalize_uuid(raw.id));
            fields.insert("name".to_string(), Value::String(raw.name.clone()));
            fields.insert("server_tick".to_string(), normalize_u64(raw.tick));
            fields.insert("dimension".to_string(), Value::from(raw.dimension));
            fields.insert("position".to_string(), position_value(raw.position));
            fields.insert("look".to_string(), look_value(raw.yaw, raw.pitch));
            normalized_error(case, rejection.category, &rejection.rule, fields)
        }
        None => {
            let spawn = construct_companion_spawn(case, subject, &raw)?;
            let mut fields = JsonMap::new();
            fields.insert("id".to_string(), normalize_uuid(spawn.id().bytes()));
            fields.insert(
                "name".to_string(),
                Value::String(spawn.name().as_str().to_string()),
            );
            fields.insert(
                "server_tick".to_string(),
                normalize_u64(spawn.server_tick()),
            );
            fields.insert(
                "dimension".to_string(),
                Value::from(spawn.dimension().get()),
            );
            fields.insert(
                "position".to_string(),
                position_value(spawn.position().get()),
            );
            fields.insert(
                "look".to_string(),
                look_value(spawn.look().yaw(), spawn.look().pitch()),
            );
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// Selects the first rule one companion spawn breaks, in the Go validator's
/// precedence.
fn companion_spawn_rejection(raw: &RawCompanionSpawn) -> Option<Rejection> {
    if let Some(suffix) = identity_rule(raw.id) {
        return Some(Rejection {
            category: "invalid-identity",
            rule: format!("companion_spawn.player_id.{suffix}"),
            error: Some(DomainError::InvalidIdentity),
        });
    }
    if let Some(suffix) = companion_name_rule(&raw.name) {
        return Some(Rejection {
            category: "invalid-value",
            rule: format!("companion_spawn.{suffix}"),
            error: Some(DomainError::InvalidText),
        });
    }
    if let Err(error) = admit_companion_dimension(raw.dimension) {
        return Some(Rejection {
            category: "invalid-enum",
            rule: "companion_spawn.dimension".to_string(),
            error,
        });
    }
    if raw.position.iter().any(|component| !component.is_finite()) {
        return Some(Rejection {
            category: "invalid-value",
            rule: "companion_spawn.position_finite".to_string(),
            error: Some(DomainError::NonFiniteRotation),
        });
    }
    if !raw.yaw.is_finite() || !raw.pitch.is_finite() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "companion_spawn.rotation_finite".to_string(),
            error: Some(DomainError::NonFiniteRotation),
        });
    }
    if !companion_pitch_admits(raw.pitch) {
        return Some(Rejection {
            category: "invalid-value",
            rule: "companion_spawn.pitch_vertical_look_range".to_string(),
            error: Some(DomainError::InvalidCompanionPitch),
        });
    }
    None
}

/// Verifies the constructor agreement for one classified companion spawn
/// rejection. The depths-dimension and pitch rules are owned by the spawn
/// constructor itself, so their verdicts run on the whole checked chain.
fn verify_companion_spawn_rejection(
    case: &FrozenCase,
    subject: &str,
    raw: &RawCompanionSpawn,
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "companion_spawn.player_id.nonzero"
        | "companion_spawn.player_id.version_v4"
        | "companion_spawn.player_id.variant_rfc4122" => {
            CompanionId::try_from_bytes(raw.id).map(|_| ())
        }
        "companion_spawn.display_name.utf8"
        | "companion_spawn.display_name.length_range"
        | "companion_spawn.display_name.control"
        | "companion_spawn.display_name.canonical_trim"
        | "companion_spawn.companion_name.embedded_whitespace" => {
            CompanionName::try_from_canonical(raw.name.clone()).map(|_| ())
        }
        "companion_spawn.dimension" => match admit_companion_dimension(raw.dimension) {
            // The depths rejection is verified through the record
            // constructor, which owns the companion dimension rule.
            Err(Some(DomainError::InvalidCompanionDimension)) => {
                CompanionSpawn::try_new(companion_spawn_parts(case, subject, raw, 1)?).map(|_| ())
            }
            Err(Some(DomainError::InvalidDimension)) => {
                let byte = u8::try_from(raw.dimension)
                    .map_err(|_| classification_conflict(case, subject, &rejection.rule))?;
                Dimension::new(byte).map(|_| ())
            }
            _ => return Err(classification_conflict(case, subject, &rejection.rule)),
        },
        "companion_spawn.position_finite" => FiniteVec3::try_new(raw.position).map(|_| ()),
        "companion_spawn.rotation_finite" => LookAngles::try_new(raw.yaw, raw.pitch).map(|_| ()),
        "companion_spawn.pitch_vertical_look_range" => {
            // The pitch rule is owned by the record constructor, so the
            // verdict runs on the whole checked chain with an admitted
            // overworld dimension.
            CompanionSpawn::try_new(companion_spawn_parts(case, subject, raw, 0)?).map(|_| ())
        }
        _ => return Err(classification_conflict(case, subject, &rejection.rule)),
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Builds the checked companion spawn parts one constructor-verdict
/// verification needs, with the dimension forced to the given admitted byte.
fn companion_spawn_parts(
    case: &FrozenCase,
    subject: &str,
    raw: &RawCompanionSpawn,
    dimension_byte: u8,
) -> Result<CompanionSpawnParts, DispatchError> {
    Ok(CompanionSpawnParts {
        id: CompanionId::try_from_bytes(raw.id)
            .map_err(|error| construction_conflict(case, subject, error))?,
        name: CompanionName::try_from_canonical(raw.name.clone())
            .map_err(|error| construction_conflict(case, subject, error))?,
        server_tick: raw.tick,
        dimension: Dimension::new(dimension_byte)
            .map_err(|error| construction_conflict(case, subject, error))?,
        position: FiniteVec3::try_new(raw.position)
            .map_err(|error| construction_conflict(case, subject, error))?,
        look: LookAngles::try_new(raw.yaw, raw.pitch)
            .map_err(|error| construction_conflict(case, subject, error))?,
    })
}

/// Constructs the whole checked companion spawn chain for one admitted case.
fn construct_companion_spawn(
    case: &FrozenCase,
    subject: &str,
    raw: &RawCompanionSpawn,
) -> Result<CompanionSpawn, DispatchError> {
    let byte = u8::try_from(raw.dimension)
        .map_err(|_| classification_conflict(case, subject, "dimension outside the byte range"))?;
    CompanionSpawn::try_new(CompanionSpawnParts {
        id: CompanionId::try_from_bytes(raw.id)
            .map_err(|error| construction_conflict(case, subject, error))?,
        name: CompanionName::try_from_canonical(raw.name.clone())
            .map_err(|error| construction_conflict(case, subject, error))?,
        server_tick: raw.tick,
        dimension: Dimension::new(byte)
            .map_err(|error| construction_conflict(case, subject, error))?,
        position: FiniteVec3::try_new(raw.position)
            .map_err(|error| construction_conflict(case, subject, error))?,
        look: LookAngles::try_new(raw.yaw, raw.pitch)
            .map_err(|error| construction_conflict(case, subject, error))?,
    })
    .map_err(|error| construction_conflict(case, subject, error))
}

/// The parsed raw shape of one companion body record inside a batch.
struct RawCompanionState {
    id: [u8; 16],
    dimension: i32,
    position: [f32; 3],
    yaw: f32,
    pitch: f32,
    reset: bool,
}

/// Parses one required ordered batch of companion body records.
fn parse_companion_states(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<Vec<RawCompanionState>, DispatchError> {
    required_array(case, input, "states")?
        .iter()
        .enumerate()
        .map(|(index, value)| {
            let path = format!("states[{index}]");
            let object = super::support::value_object(case, &path, value)?;
            Ok(RawCompanionState {
                id: parse_uuid_hex(
                    case,
                    &format!("{path}.id"),
                    required_string(case, object, "id")?,
                )?,
                dimension: required_i32(case, object, "dimension")?,
                position: parse_position(case, object)?,
                yaw: parse_angle(case, object, "yaw")?,
                pitch: parse_angle(case, object, "pitch")?,
                reset: required_bool(case, object, "reset")?,
            })
        })
        .collect()
}

/// Executes one `companion-states` case, whose payload is the publish tick
/// and the ordered body batch.
///
/// The walk replays the Go validator exactly: each record's identity,
/// companion dimension, position, rotation and pitch rules run in input
/// order, and the strictly-increasing rule runs after each otherwise valid
/// record against the previous identity. Submitted order is never sorted.
fn execute_companion_states(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let subject = "companion-states";
    let tick = required_u64(case, input, "tick")?;
    let states = parse_companion_states(case, input)?;

    match companion_states_rejection(&states) {
        Some(rejection) => {
            verify_companion_states_rejection(case, subject, tick, &states, &rejection)?;
            let mut fields = JsonMap::new();
            fields.insert("server_tick".to_string(), normalize_u64(tick));
            fields.insert(
                "states".to_string(),
                Value::Array(
                    states
                        .iter()
                        .map(|state| {
                            serde_json::json!({
                                "id": normalize_uuid(state.id),
                                "dimension": state.dimension,
                                "position": position_value(state.position),
                                "look": look_value(state.yaw, state.pitch),
                                "reset": state.reset,
                            })
                        })
                        .collect(),
                ),
            );
            normalized_error(case, rejection.category, &rejection.rule, fields)
        }
        None => {
            let mut records = Vec::with_capacity(states.len());
            for state in &states {
                records.push(construct_companion_state(case, subject, state)?);
            }
            let batch = CompanionStates::try_new(CompanionStatesParts {
                server_tick: tick,
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
                        .map(|state| {
                            serde_json::json!({
                                "id": normalize_uuid(state.id().bytes()),
                                "dimension": state.dimension().get(),
                                "position": position_value(state.position().get()),
                                "look": look_value(state.look().yaw(), state.look().pitch()),
                                "reset": state.reset(),
                            })
                        })
                        .collect(),
                ),
            );
            Ok(normalized_ok(subject, fields))
        }
    }
}

/// Selects the first rule one companion batch breaks, in the Go validator's
/// walk order.
fn companion_states_rejection(states: &[RawCompanionState]) -> Option<Rejection> {
    if states.is_empty() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "companion_states.count_range".to_string(),
            error: Some(DomainError::EmptyStateBatch),
        });
    }
    for (index, state) in states.iter().enumerate() {
        if let Some(suffix) = identity_rule(state.id) {
            return Some(Rejection {
                category: "invalid-identity",
                rule: format!("companion_states.state_{index}.player_id.{suffix}"),
                error: Some(DomainError::InvalidIdentity),
            });
        }
        if let Err(error) = admit_companion_dimension(state.dimension) {
            return Some(Rejection {
                category: "invalid-enum",
                rule: format!("companion_states.state_{index}.dimension"),
                error,
            });
        }
        if state
            .position
            .iter()
            .any(|component| !component.is_finite())
        {
            return Some(Rejection {
                category: "invalid-value",
                rule: format!("companion_states.state_{index}.position_finite"),
                error: Some(DomainError::NonFiniteRotation),
            });
        }
        if !state.yaw.is_finite() || !state.pitch.is_finite() {
            return Some(Rejection {
                category: "invalid-value",
                rule: format!("companion_states.state_{index}.rotation_finite"),
                error: Some(DomainError::NonFiniteRotation),
            });
        }
        if !companion_pitch_admits(state.pitch) {
            return Some(Rejection {
                category: "invalid-value",
                rule: format!("companion_states.state_{index}.pitch_vertical_look_range"),
                error: Some(DomainError::InvalidCompanionPitch),
            });
        }
        if index > 0 && states[index - 1].id >= state.id {
            return Some(Rejection {
                category: "invalid-value",
                rule: "companion_states.strictly_increasing_ids".to_string(),
                error: Some(DomainError::InvalidStateOrder),
            });
        }
    }
    None
}

/// Verifies the constructor agreement for one classified companion batch
/// rejection. The empty-batch and order rules are owned by the batch
/// constructor; the per-record rules by the record constructor and the value
/// constructors it composes.
fn verify_companion_states_rejection(
    case: &FrozenCase,
    subject: &str,
    tick: u64,
    states: &[RawCompanionState],
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let constructed: Result<(), DomainError> = match rejection.rule.as_str() {
        "companion_states.count_range" => CompanionStates::try_new(CompanionStatesParts {
            server_tick: tick,
            states: Vec::new().into_boxed_slice(),
        })
        .map(|_| ()),
        "companion_states.strictly_increasing_ids" => {
            // The order rule fires at the first non-increasing adjacent
            // pair, so the constructor verdict is taken on the checked
            // prefix that ends at that pair.
            let pair = states
                .windows(2)
                .position(|pair| pair[0].id >= pair[1].id)
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let mut records = Vec::new();
            for state in &states[..pair + 2] {
                records.push(construct_companion_state(case, subject, state)?);
            }
            CompanionStates::try_new(CompanionStatesParts {
                server_tick: tick,
                states: records.into_boxed_slice(),
            })
            .map(|_| ())
        }
        rule => {
            let rest = rule
                .strip_prefix("companion_states.state_")
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            let (index_text, suffix) = rest
                .split_once('.')
                .ok_or_else(|| classification_conflict(case, subject, rule))?;
            let index: usize = index_text
                .parse()
                .map_err(|_| classification_conflict(case, subject, rule))?;
            let state = states
                .get(index)
                .ok_or_else(|| classification_conflict(case, subject, &rejection.rule))?;
            let constructed =
                verify_companion_state_rule(case, subject, state, suffix, &rejection.rule)?;
            if constructed == Err(expected) {
                return Ok(());
            }
            return Err(classification_conflict(case, subject, &rejection.rule));
        }
    };
    match constructed {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(case, subject, &rejection.rule)),
    }
}

/// Runs one per-record companion constructor verdict for verification.
fn verify_companion_state_rule(
    case: &FrozenCase,
    subject: &str,
    state: &RawCompanionState,
    suffix: &str,
    rule: &str,
) -> Result<Result<(), DomainError>, DispatchError> {
    match suffix {
        "player_id.nonzero" | "player_id.version_v4" | "player_id.variant_rfc4122" => {
            Ok(CompanionId::try_from_bytes(state.id).map(|_| ()))
        }
        "dimension" => match admit_companion_dimension(state.dimension) {
            // The depths rejection is verified through the record
            // constructor, which owns the companion dimension rule.
            Err(Some(DomainError::InvalidCompanionDimension)) => Ok(CompanionState::try_new(
                companion_state_parts(case, subject, state, 1)?,
            )
            .map(|_| ())),
            Err(Some(DomainError::InvalidDimension)) => {
                let byte = u8::try_from(state.dimension)
                    .map_err(|_| classification_conflict(case, subject, rule))?;
                Ok(Dimension::new(byte).map(|_| ()))
            }
            _ => Err(classification_conflict(case, subject, rule)),
        },
        "position_finite" => Ok(FiniteVec3::try_new(state.position).map(|_| ())),
        "rotation_finite" => Ok(LookAngles::try_new(state.yaw, state.pitch).map(|_| ())),
        "pitch_vertical_look_range" => {
            // The pitch rule is owned by the record constructor, so the
            // verdict runs on the whole checked chain with an admitted
            // overworld dimension.
            Ok(
                CompanionState::try_new(companion_state_parts(case, subject, state, 0)?)
                    .map(|_| ()),
            )
        }
        _ => Err(classification_conflict(case, subject, suffix)),
    }
}

/// Builds the checked companion state parts one constructor-verdict
/// verification needs, with the dimension forced to the given admitted byte.
fn companion_state_parts(
    case: &FrozenCase,
    subject: &str,
    raw: &RawCompanionState,
    dimension_byte: u8,
) -> Result<CompanionStateParts, DispatchError> {
    Ok(CompanionStateParts {
        id: CompanionId::try_from_bytes(raw.id)
            .map_err(|error| construction_conflict(case, subject, error))?,
        dimension: Dimension::new(dimension_byte)
            .map_err(|error| construction_conflict(case, subject, error))?,
        position: FiniteVec3::try_new(raw.position)
            .map_err(|error| construction_conflict(case, subject, error))?,
        look: LookAngles::try_new(raw.yaw, raw.pitch)
            .map_err(|error| construction_conflict(case, subject, error))?,
        reset: raw.reset,
    })
}

/// Constructs one checked companion record for an admitted case.
fn construct_companion_state(
    case: &FrozenCase,
    subject: &str,
    raw: &RawCompanionState,
) -> Result<CompanionState, DispatchError> {
    let byte = u8::try_from(raw.dimension)
        .map_err(|_| classification_conflict(case, subject, "dimension outside the byte range"))?;
    CompanionState::try_new(CompanionStateParts {
        id: CompanionId::try_from_bytes(raw.id)
            .map_err(|error| construction_conflict(case, subject, error))?,
        dimension: Dimension::new(byte)
            .map_err(|error| construction_conflict(case, subject, error))?,
        position: FiniteVec3::try_new(raw.position)
            .map_err(|error| construction_conflict(case, subject, error))?,
        look: LookAngles::try_new(raw.yaw, raw.pitch)
            .map_err(|error| construction_conflict(case, subject, error))?,
        reset: raw.reset,
    })
    .map_err(|error| construction_conflict(case, subject, error))
}

#[test]
fn event_people_execute_46_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}

/// Proves the shared comparator actually compares semantic fields: the
/// produced accepted result matches its frozen expectation, while the same
/// actual value no longer matches once one normalized field is mutated.
#[test]
fn event_people_comparator_detects_semantic_mutation() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    let accepted = executed
        .into_iter()
        .find(|case| {
            case.actual.get("kind").and_then(|kind| kind.as_str()) == Some("ok")
                && case
                    .actual
                    .get("category")
                    .and_then(|category| category.as_str())
                    == Some("remote-player-spawn")
        })
        .expect("one accepted remote-player-spawn case");

    assert_domain_normalized(&accepted);

    let mut mutated_case = accepted.case;
    let fields = mutated_case
        .normalized
        .get_mut("fields")
        .and_then(|fields| fields.as_object_mut())
        .expect("normalized fields object");
    fields.insert("display_name".to_string(), Value::from("Mutated"));

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
