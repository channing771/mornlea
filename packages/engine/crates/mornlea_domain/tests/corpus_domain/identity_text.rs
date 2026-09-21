//! Topic module for domain.identity_values corpus cases.
//!
//! The five rules here are the Go producer's identity and canonical-text
//! admissions. Each executor parses the case input, selects the Go rejection
//! rule from the raw value in the producer's own precedence, and then proves
//! the matching public Rust constructor agrees: a classified acceptance must
//! construct, and a classified rejection must return the mapped `DomainError`.
//! Accepted fields are read back from the constructed Rust value, never from
//! the parsed input or the frozen expectation.

use super::support::{
    DispatchError, ExecutedCase, JsonMap, assert_domain_normalized, execute_topic, input_object,
    invalid_case, normalize_uuid, normalized_error, normalized_ok, optional_string, parse_uuid_hex,
    required_string,
};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};
use mornlea_domain::{CommandText, CompanionName, DisplayName, DomainError, PlayerId, SpeechText};
use serde_json::Value;

pub const EXPECTED_COUNT: usize = 31;

const RULES: &[&str] = &[
    "player-id",
    "display-name",
    "companion-name",
    "command-text",
    "speech-text",
];

/// Scalar limit of the Go `core.NormalizeDisplayName` length rule.
const DISPLAY_NAME_MAX_SCALARS: usize = 32;
/// UTF-8 byte limit of the Go `core.NormalizeDisplayName` length rule.
const DISPLAY_NAME_MAX_BYTES: usize = 128;
/// Byte bound of the Go `protocol.ChatCommandTextMaxBytes` command slot.
///
/// This mirrors the private bound in `mornlea_domain::text`, which the domain
/// crate keeps unexported. The agreement check in `execute_bounded_text` fails
/// closed if the two drift, so a stale copy cannot silently widen or narrow the
/// accepted set.
const COMMAND_TEXT_MAX_BYTES: usize = 1024;
/// Byte bound of the Go `protocol.ChatSpeechTextMaxBytes` speech slot.
const SPEECH_TEXT_MAX_BYTES: usize = 256;

pub fn owns(case: &FrozenCase) -> bool {
    if case.consumer != CorpusConsumer::Domain || case.family != "domain.identity_values" {
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
        "player-id" => execute_player_id(case, input),
        "display-name" => execute_name(case, input, NameSlot::Display),
        "companion-name" => execute_name(case, input, NameSlot::Companion),
        "command-text" => execute_bounded_text(case, input, TextSlot::Command),
        "speech-text" => execute_bounded_text(case, input, TextSlot::Speech),
        unknown => Err(invalid_case(
            case,
            format!("unknown identity/text rule '{unknown}'"),
        )),
    }
}

/// Executes one `player-id` case.
///
/// The schema defaults an absent `uuid` to the empty string, so both a missing
/// and an empty identity reach the strict 32-hex decoder as hard schema
/// failures rather than as a normalized identity rejection.
fn execute_player_id(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let text = optional_string(case, input, "uuid")?.unwrap_or("");
    let bytes = parse_uuid_hex(case, "uuid", text)?;
    let rule = player_id_rule(bytes);

    match PlayerId::try_from_bytes(bytes) {
        Ok(id) => {
            if !rule.is_empty() {
                return Err(classification_conflict(case, "player-id", rule));
            }
            let mut fields = JsonMap::new();
            fields.insert("uuid".to_string(), normalize_uuid(id.bytes()));
            Ok(normalized_ok("player-id", fields))
        }
        Err(error) => {
            if rule.is_empty() || error != DomainError::InvalidIdentity {
                return Err(classification_conflict(case, "player-id", rule));
            }
            let mut fields = JsonMap::new();
            fields.insert("uuid".to_string(), normalize_uuid(bytes));
            normalized_error(case, "invalid-identity", rule, fields)
        }
    }
}

/// Executes one `display-name` or `companion-name` case.
///
/// The accepted name comes from the constructed value's own getter; the
/// rejected projection echoes the raw input because a rejected value has no
/// getter to read.
fn execute_name(
    case: &FrozenCase,
    input: &JsonMap,
    slot: NameSlot,
) -> Result<serde_json::Value, DispatchError> {
    let name = required_string(case, input, "name")?;
    let rule = slot.classify(name);
    let constructed = slot.construct(name.to_string());

    if !rule.is_empty() {
        return match constructed {
            Err(DomainError::InvalidText) => {
                let mut fields = JsonMap::new();
                fields.insert("name".to_string(), Value::String(name.to_string()));
                normalized_error(case, "invalid-value", rule, fields)
            }
            _ => Err(classification_conflict(case, slot.category(), rule)),
        };
    }

    match constructed {
        Ok(value) => {
            let mut fields = JsonMap::new();
            fields.insert("name".to_string(), Value::String(value));
            Ok(normalized_ok(slot.category(), fields))
        }
        Err(_) => Err(classification_conflict(case, slot.category(), "")),
    }
}

/// Executes one `command-text` or `speech-text` case.
fn execute_bounded_text(
    case: &FrozenCase,
    input: &JsonMap,
    slot: TextSlot,
) -> Result<serde_json::Value, DispatchError> {
    let text = required_string(case, input, "text")?;
    let suffix = bounded_text_rule(text, slot.max_bytes());
    let constructed = slot.construct(text.to_string());

    match suffix {
        Some(suffix) => {
            let rule = format!("{}.{suffix}", slot.subject());
            match constructed {
                Err(DomainError::InvalidText) => {
                    let mut fields = JsonMap::new();
                    fields.insert("text".to_string(), Value::String(text.to_string()));
                    normalized_error(case, "invalid-value", &rule, fields)
                }
                _ => Err(classification_conflict(case, slot.category(), &rule)),
            }
        }
        None => match constructed {
            Ok(value) => {
                let mut fields = JsonMap::new();
                fields.insert("text".to_string(), Value::String(value));
                Ok(normalized_ok(slot.category(), fields))
            }
            Err(_) => Err(classification_conflict(case, slot.category(), "")),
        },
    }
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

/// The two canonical-name slots, which share the display-name rule.
#[derive(Clone, Copy)]
enum NameSlot {
    Display,
    Companion,
}

impl NameSlot {
    fn category(self) -> &'static str {
        match self {
            NameSlot::Display => "display-name",
            NameSlot::Companion => "companion-name",
        }
    }

    fn classify(self, name: &str) -> &'static str {
        match self {
            NameSlot::Display => display_name_rule(name),
            NameSlot::Companion => companion_name_rule(name),
        }
    }

    /// Builds the accepted value through the real public constructor, so the
    /// normalized field can only carry a value the domain actually admits.
    fn construct(self, name: String) -> Result<String, DomainError> {
        match self {
            NameSlot::Display => {
                DisplayName::try_from_canonical(name).map(|value| value.as_str().to_owned())
            }
            NameSlot::Companion => {
                CompanionName::try_from_canonical(name).map(|value| value.as_str().to_owned())
            }
        }
    }
}

/// The two bounded-text slots, whose rejection suffixes are identical but
/// whose prefix, byte bound and accepted category differ.
#[derive(Clone, Copy)]
enum TextSlot {
    Command,
    Speech,
}

impl TextSlot {
    fn max_bytes(self) -> usize {
        match self {
            TextSlot::Command => COMMAND_TEXT_MAX_BYTES,
            TextSlot::Speech => SPEECH_TEXT_MAX_BYTES,
        }
    }

    fn subject(self) -> &'static str {
        match self {
            TextSlot::Command => "command_text",
            TextSlot::Speech => "speech_text",
        }
    }

    fn category(self) -> &'static str {
        match self {
            TextSlot::Command => "command-text",
            TextSlot::Speech => "speech-text",
        }
    }

    /// Builds the accepted value through the real public constructor.
    fn construct(self, text: String) -> Result<String, DomainError> {
        match self {
            TextSlot::Command => {
                CommandText::try_from_canonical(text).map(|value| value.as_str().to_owned())
            }
            TextSlot::Speech => {
                SpeechText::try_from_canonical(text).map(|value| value.as_str().to_owned())
            }
        }
    }
}

/// Selects the Go identity rejection rule from the raw wire bytes.
///
/// The order is the Go `core.PlayerID.Valid` order: the zero value first, then
/// the version nibble, then the RFC-4122 variant bits. An empty result means
/// the identity is admitted, which the caller proves with
/// `PlayerId::try_from_bytes`.
fn player_id_rule(bytes: [u8; 16]) -> &'static str {
    if bytes == [0; 16] {
        return "player_id.nonzero";
    }
    if bytes[6] >> 4 != 4 {
        return "player_id.version_v4";
    }
    if bytes[8] & 0xc0 != 0x80 {
        return "player_id.variant_rfc4122";
    }
    ""
}

/// Selects the Go display-name rejection rule from the raw name.
///
/// The Go rule trims first and applies the length and control checks to the
/// trimmed form, so a surrounding whitespace character is reported as
/// `display_name.canonical_trim` rather than as a length or control failure.
/// Invalid UTF-8 cannot reach this classifier because the input is already a
/// Rust `String`; that closed producer rule has no frozen case.
fn display_name_rule(name: &str) -> &'static str {
    let trimmed = name.trim_matches(is_go_whitespace);
    if trimmed.is_empty()
        || trimmed.chars().count() > DISPLAY_NAME_MAX_SCALARS
        || trimmed.len() > DISPLAY_NAME_MAX_BYTES
    {
        return "display_name.length_range";
    }
    if trimmed.chars().any(is_go_control) {
        return "display_name.control";
    }
    if trimmed != name {
        return "display_name.canonical_trim";
    }
    ""
}

/// Selects the Go companion-name rejection rule from the raw name.
///
/// The companion rule is the display-name rule first, then a rejection of any
/// Unicode whitespace in the raw name, which is what keeps the interior space
/// that a display name legitimately carries.
fn companion_name_rule(name: &str) -> &'static str {
    let display = display_name_rule(name);
    if !display.is_empty() {
        return display;
    }
    if name.chars().any(is_go_whitespace) {
        return "companion_name.embedded_whitespace";
    }
    ""
}

/// Selects the Go bounded-text rejection suffix from the raw text.
///
/// The order is the Go `domainIdentityBoundedTextRule` order: byte range, then
/// trimming, then the control scan. A `None` result means the text is admitted,
/// which the caller proves with the slot's constructor. The Go UTF-8 arm is
/// unreachable from a Rust `String` and has no frozen case.
fn bounded_text_rule(text: &str, limit: usize) -> Option<&'static str> {
    if text.is_empty() || text.len() > limit {
        return Some("byte_range");
    }
    if text.trim_matches(is_go_whitespace) != text {
        return Some("untrimmed");
    }
    if text.chars().any(is_go_control) {
        return Some("control");
    }
    None
}

/// Reports whether `ch` is Unicode whitespace exactly as Go's
/// `unicode.IsSpace` (the `White_Space` property) reports it.
///
/// The ranges are written out rather than delegated to a Unicode table crate
/// so the classifier cannot drift with a table version bump. This is the same
/// pinned set the domain crate's private predicate uses.
fn is_go_whitespace(ch: char) -> bool {
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

/// Reports whether `ch` is a control character exactly as Go's
/// `unicode.IsControl` (the `Cc` category) reports it, including U+0000.
fn is_go_control(ch: char) -> bool {
    matches!(ch, '\u{0000}'..='\u{001F}' | '\u{007F}'..='\u{009F}')
}

#[test]
fn identity_text_execute_31_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}

/// Proves the shared comparator actually compares semantic fields: the produced
/// accepted result matches its frozen expectation, while the same actual value
/// no longer matches once one normalized field is mutated.
#[test]
fn identity_text_comparator_detects_semantic_mutation() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    let accepted = executed
        .into_iter()
        .find(|case| {
            case.actual.get("category").and_then(|value| value.as_str()) == Some("display-name")
        })
        .expect("one accepted display-name case");

    assert_domain_normalized(&accepted);

    let mut mutated_case = accepted.case;
    let fields = mutated_case
        .normalized
        .get_mut("fields")
        .and_then(|fields| fields.as_object_mut())
        .expect("normalized fields object");
    fields.insert("name".to_string(), Value::String("MUTATED".to_string()));

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
