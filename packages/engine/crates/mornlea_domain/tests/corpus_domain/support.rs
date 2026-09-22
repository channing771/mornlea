//! Shared support for closed domain corpus testing.

use crate::runtime_corpus::{CorpusConsumer, FrozenCase, load_cases_for_consumer};
use std::collections::HashSet;

#[derive(Debug)]
pub enum DispatchError {
    NotImplemented { case_id: String, rule: String },
    InvalidCase { case_id: String, message: String },
}

impl std::fmt::Display for DispatchError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            DispatchError::NotImplemented { case_id, rule } => {
                write!(f, "case {case_id} rule '{rule}' not implemented")
            }
            DispatchError::InvalidCase { case_id, message } => {
                write!(f, "case {case_id} invalid: {message}")
            }
        }
    }
}

impl std::error::Error for DispatchError {}

pub struct ExecutedCase {
    pub case: FrozenCase,
    pub actual: serde_json::Value,
}

pub type OwnsCase = fn(&FrozenCase) -> bool;
pub type ExecuteCase = fn(&FrozenCase) -> Result<serde_json::Value, DispatchError>;

pub fn execute_topic(
    expected_count: usize,
    owns: OwnsCase,
    execute: ExecuteCase,
) -> Result<Vec<ExecutedCase>, DispatchError> {
    // The exact nonzero count is part of the closed partition contract, so a
    // zero budget is an invalid case rather than a process-aborting assertion.
    if expected_count == 0 {
        return Err(DispatchError::InvalidCase {
            case_id: "execute_topic".to_string(),
            message: "expected_count must be nonzero".to_string(),
        });
    }
    let all_cases = load_cases_for_consumer(CorpusConsumer::Domain);
    let mut executed = Vec::new();
    let mut seen_ids = HashSet::new();

    for case in all_cases {
        if owns(&case) {
            if !seen_ids.insert(case.id.clone()) {
                return Err(DispatchError::InvalidCase {
                    case_id: case.id.clone(),
                    message: format!("duplicate case id {}", case.id),
                });
            }
            let actual = execute_checked(&case, execute)?;
            executed.push(ExecutedCase { case, actual });
        }
    }

    if executed.len() != expected_count {
        return Err(DispatchError::InvalidCase {
            case_id: "execute_topic".to_string(),
            message: format!(
                "expected exactly {expected_count} cases, but executed {}",
                executed.len()
            ),
        });
    }

    Ok(executed)
}

pub fn assert_domain_normalized(executed: &ExecutedCase) {
    let mut expected_clone = executed.case.normalized.clone();

    // For accepted domain.command_control and domain.command_inventory cases,
    // remove exactly fields.wire from the expected clone.
    if (executed.case.family == "domain.command_control"
        || executed.case.family == "domain.command_inventory")
        && expected_clone.get("kind").and_then(|k| k.as_str()) == Some("ok")
        && let Some(fields) = expected_clone
            .get_mut("fields")
            .and_then(|f| f.as_object_mut())
    {
        fields.remove("wire");
    }

    assert_eq!(
        executed.actual, expected_clone,
        "case {} normalized output mismatch",
        executed.case.id
    );
}

pub type JsonMap = serde_json::Map<String, serde_json::Value>;

pub fn invalid_case(case: &FrozenCase, message: impl Into<String>) -> DispatchError {
    DispatchError::InvalidCase {
        case_id: case.id.clone(),
        message: message.into(),
    }
}

/// Validates the independently loaded case input identity before handing the
/// case to its semantic executor.
pub fn execute_checked(
    case: &FrozenCase,
    execute: ExecuteCase,
) -> Result<serde_json::Value, DispatchError> {
    let input = input_object(case)?;
    let consumer = required_string(case, input, "consumer")?;
    if consumer != "mornlea_domain" {
        return Err(invalid_case(
            case,
            format!("input consumer must be 'mornlea_domain', got '{consumer}'"),
        ));
    }
    execute(case)
}

pub fn input_object(case: &FrozenCase) -> Result<&JsonMap, DispatchError> {
    match &case.input_json {
        Some(serde_json::Value::Object(map)) => Ok(map),
        _ => Err(invalid_case(case, "input_json is not an object")),
    }
}

pub fn required_object<'a>(
    case: &FrozenCase,
    object: &'a JsonMap,
    key: &str,
) -> Result<&'a JsonMap, DispatchError> {
    match object.get(key) {
        Some(serde_json::Value::Object(map)) => Ok(map),
        Some(serde_json::Value::Null) | None => Err(invalid_case(
            case,
            format!("missing required object '{key}'"),
        )),
        Some(_) => Err(invalid_case(
            case,
            format!("field '{key}' is not an object"),
        )),
    }
}

pub fn optional_object<'a>(
    case: &FrozenCase,
    object: &'a JsonMap,
    key: &str,
) -> Result<Option<&'a JsonMap>, DispatchError> {
    match object.get(key) {
        Some(serde_json::Value::Object(map)) => Ok(Some(map)),
        Some(serde_json::Value::Null) | None => Ok(None),
        Some(_) => Err(invalid_case(
            case,
            format!("field '{key}' is not an object"),
        )),
    }
}

pub fn required_array<'a>(
    case: &FrozenCase,
    object: &'a JsonMap,
    key: &str,
) -> Result<&'a [serde_json::Value], DispatchError> {
    match object.get(key) {
        Some(serde_json::Value::Array(arr)) => Ok(arr.as_slice()),
        Some(serde_json::Value::Null) | None => Err(invalid_case(
            case,
            format!("missing required array '{key}'"),
        )),
        Some(_) => Err(invalid_case(case, format!("field '{key}' is not an array"))),
    }
}

pub fn optional_array<'a>(
    case: &FrozenCase,
    object: &'a JsonMap,
    key: &str,
) -> Result<Option<&'a [serde_json::Value]>, DispatchError> {
    match object.get(key) {
        Some(serde_json::Value::Array(arr)) => Ok(Some(arr.as_slice())),
        Some(serde_json::Value::Null) | None => Ok(None),
        Some(_) => Err(invalid_case(case, format!("field '{key}' is not an array"))),
    }
}

pub fn required_string<'a>(
    case: &FrozenCase,
    object: &'a JsonMap,
    key: &str,
) -> Result<&'a str, DispatchError> {
    match object.get(key) {
        Some(serde_json::Value::String(s)) => Ok(s.as_str()),
        Some(serde_json::Value::Null) | None => Err(invalid_case(
            case,
            format!("missing required string '{key}'"),
        )),
        Some(_) => Err(invalid_case(case, format!("field '{key}' is not a string"))),
    }
}

pub fn optional_string<'a>(
    case: &FrozenCase,
    object: &'a JsonMap,
    key: &str,
) -> Result<Option<&'a str>, DispatchError> {
    match object.get(key) {
        Some(serde_json::Value::String(s)) => Ok(Some(s.as_str())),
        Some(serde_json::Value::Null) | None => Ok(None),
        Some(_) => Err(invalid_case(case, format!("field '{key}' is not a string"))),
    }
}

pub fn required_bool(
    case: &FrozenCase,
    object: &JsonMap,
    key: &str,
) -> Result<bool, DispatchError> {
    match object.get(key) {
        Some(serde_json::Value::Bool(b)) => Ok(*b),
        Some(serde_json::Value::Null) | None => {
            Err(invalid_case(case, format!("missing required bool '{key}'")))
        }
        Some(_) => Err(invalid_case(
            case,
            format!("field '{key}' is not a boolean"),
        )),
    }
}

pub fn optional_bool(
    case: &FrozenCase,
    object: &JsonMap,
    key: &str,
) -> Result<Option<bool>, DispatchError> {
    match object.get(key) {
        Some(serde_json::Value::Bool(b)) => Ok(Some(*b)),
        Some(serde_json::Value::Null) | None => Ok(None),
        Some(_) => Err(invalid_case(
            case,
            format!("field '{key}' is not a boolean"),
        )),
    }
}

macro_rules! impl_required_integer {
    ($fn_name:ident, $opt_name:ident, $val_name:ident, $ty:ident, $as_fn:ident) => {
        pub fn $fn_name(
            case: &FrozenCase,
            object: &JsonMap,
            key: &str,
        ) -> Result<$ty, DispatchError> {
            match object.get(key) {
                Some(v) => $val_name(case, key, v),
                None => Err(invalid_case(
                    case,
                    format!("missing required integer '{key}'"),
                )),
            }
        }

        pub fn $opt_name(
            case: &FrozenCase,
            object: &JsonMap,
            key: &str,
        ) -> Result<Option<$ty>, DispatchError> {
            match object.get(key) {
                Some(serde_json::Value::Null) | None => Ok(None),
                Some(v) => $val_name(case, key, v).map(Some),
            }
        }
    };
}

pub fn value_object<'a>(
    case: &FrozenCase,
    path: &str,
    value: &'a serde_json::Value,
) -> Result<&'a JsonMap, DispatchError> {
    match value {
        serde_json::Value::Object(map) => Ok(map),
        _ => Err(invalid_case(case, format!("expected object at '{path}'"))),
    }
}

pub fn value_array<'a>(
    case: &FrozenCase,
    path: &str,
    value: &'a serde_json::Value,
) -> Result<&'a [serde_json::Value], DispatchError> {
    match value {
        serde_json::Value::Array(arr) => Ok(arr.as_slice()),
        _ => Err(invalid_case(case, format!("expected array at '{path}'"))),
    }
}

pub fn value_string<'a>(
    case: &FrozenCase,
    path: &str,
    value: &'a serde_json::Value,
) -> Result<&'a str, DispatchError> {
    match value {
        serde_json::Value::String(s) => Ok(s.as_str()),
        _ => Err(invalid_case(case, format!("expected string at '{path}'"))),
    }
}

pub fn value_bool(
    case: &FrozenCase,
    path: &str,
    value: &serde_json::Value,
) -> Result<bool, DispatchError> {
    match value {
        serde_json::Value::Bool(b) => Ok(*b),
        _ => Err(invalid_case(case, format!("expected boolean at '{path}'"))),
    }
}

pub fn value_i8(
    case: &FrozenCase,
    path: &str,
    value: &serde_json::Value,
) -> Result<i8, DispatchError> {
    match value.as_i64() {
        Some(n) if (i8::MIN as i64..=i8::MAX as i64).contains(&n) => Ok(n as i8),
        _ => Err(invalid_case(
            case,
            format!("expected i8 integer at '{path}'"),
        )),
    }
}

pub fn value_u8(
    case: &FrozenCase,
    path: &str,
    value: &serde_json::Value,
) -> Result<u8, DispatchError> {
    match value.as_u64() {
        Some(n) if n <= u8::MAX as u64 => Ok(n as u8),
        _ => Err(invalid_case(
            case,
            format!("expected u8 integer at '{path}'"),
        )),
    }
}

pub fn value_u16(
    case: &FrozenCase,
    path: &str,
    value: &serde_json::Value,
) -> Result<u16, DispatchError> {
    match value.as_u64() {
        Some(n) if n <= u16::MAX as u64 => Ok(n as u16),
        _ => Err(invalid_case(
            case,
            format!("expected u16 integer at '{path}'"),
        )),
    }
}

pub fn value_u32(
    case: &FrozenCase,
    path: &str,
    value: &serde_json::Value,
) -> Result<u32, DispatchError> {
    match value.as_u64() {
        Some(n) if n <= u32::MAX as u64 => Ok(n as u32),
        _ => Err(invalid_case(
            case,
            format!("expected u32 integer at '{path}'"),
        )),
    }
}

pub fn value_i32(
    case: &FrozenCase,
    path: &str,
    value: &serde_json::Value,
) -> Result<i32, DispatchError> {
    match value.as_i64() {
        Some(n) if (i32::MIN as i64..=i32::MAX as i64).contains(&n) => Ok(n as i32),
        _ => Err(invalid_case(
            case,
            format!("expected i32 integer at '{path}'"),
        )),
    }
}

pub fn value_i64(
    case: &FrozenCase,
    path: &str,
    value: &serde_json::Value,
) -> Result<i64, DispatchError> {
    match value.as_i64() {
        Some(n) => Ok(n),
        _ => Err(invalid_case(
            case,
            format!("expected i64 integer at '{path}'"),
        )),
    }
}

pub fn value_u64(
    case: &FrozenCase,
    path: &str,
    value: &serde_json::Value,
) -> Result<u64, DispatchError> {
    match value.as_u64() {
        Some(n) => Ok(n),
        _ => Err(invalid_case(
            case,
            format!("expected u64 integer at '{path}'"),
        )),
    }
}

impl_required_integer!(required_i8, optional_i8, value_i8, i8, as_i64);
impl_required_integer!(required_u8, optional_u8, value_u8, u8, as_u64);
impl_required_integer!(required_u16, optional_u16, value_u16, u16, as_u64);
impl_required_integer!(required_u32, optional_u32, value_u32, u32, as_u64);
impl_required_integer!(required_i32, optional_i32, value_i32, i32, as_i64);
impl_required_integer!(required_i64, optional_i64, value_i64, i64, as_i64);
impl_required_integer!(required_u64, optional_u64, value_u64, u64, as_u64);

pub fn exact_array<'a, const N: usize>(
    case: &FrozenCase,
    path: &str,
    values: &'a [serde_json::Value],
) -> Result<&'a [serde_json::Value; N], DispatchError> {
    values.try_into().map_err(|_| {
        invalid_case(
            case,
            format!(
                "expected array at '{path}' of exact length {N}, got {}",
                values.len()
            ),
        )
    })
}

pub fn parse_uuid_hex(
    case: &FrozenCase,
    path: &str,
    text: &str,
) -> Result<[u8; 16], DispatchError> {
    if text.len() != 32 {
        return Err(invalid_case(
            case,
            format!(
                "expected 32-char hex UUID at '{path}', got len {}",
                text.len()
            ),
        ));
    }
    // Reject non-ASCII before slicing fixed-width pairs: a multi-byte character
    // at an odd byte offset would otherwise split a UTF-8 boundary and panic.
    if !text.is_ascii() {
        return Err(invalid_case(
            case,
            format!("non-ASCII characters in UUID at '{path}': '{text}'"),
        ));
    }
    let mut bytes = [0u8; 16];
    for i in 0..16 {
        let chunk = &text[i * 2..i * 2 + 2];
        if !chunk
            .chars()
            .all(|c| c.is_ascii_hexdigit() && !c.is_ascii_uppercase())
        {
            return Err(invalid_case(
                case,
                format!("non-lowercase-hex characters in UUID at '{path}': '{text}'"),
            ));
        }
        bytes[i] = u8::from_str_radix(chunk, 16).map_err(|e| {
            invalid_case(case, format!("failed to parse hex byte at '{path}': {e}"))
        })?;
    }
    Ok(bytes)
}

pub fn parse_f32_token(case: &FrozenCase, path: &str, text: &str) -> Result<f32, DispatchError> {
    match text {
        "NaN" => Ok(f32::NAN),
        "Inf" | "+Inf" => Ok(f32::INFINITY),
        "-Inf" => Ok(f32::NEG_INFINITY),
        _ => {
            if !is_decimal_f32_token(text) {
                return Err(invalid_case(
                    case,
                    format!("invalid decimal grammar in float token at '{path}': '{text}'"),
                ));
            }
            let val = text.parse::<f32>().map_err(|e| {
                invalid_case(
                    case,
                    format!("failed to parse float token at '{path}': '{text}': {e}"),
                )
            })?;
            // "only the four explicit spellings may intentionally produce non-finite bits"
            // "A decimal token that overflows or underflows with a parser error is also hard"
            if !val.is_finite() {
                return Err(invalid_case(
                    case,
                    format!("decimal overflow in float token at '{path}': '{text}'"),
                ));
            }
            Ok(val)
        }
    }
}

fn is_decimal_f32_token(text: &str) -> bool {
    let bytes = text.as_bytes();
    let mut index = 0;
    if matches!(bytes.first(), Some(b'+') | Some(b'-')) {
        index += 1;
    }

    let mut mantissa_digits = 0;
    let mut saw_decimal_point = false;
    while let Some(byte) = bytes.get(index) {
        match byte {
            b'0'..=b'9' => mantissa_digits += 1,
            b'.' if !saw_decimal_point => saw_decimal_point = true,
            _ => break,
        }
        index += 1;
    }
    if mantissa_digits == 0 {
        return false;
    }

    if matches!(bytes.get(index), Some(b'e') | Some(b'E')) {
        index += 1;
        if matches!(bytes.get(index), Some(b'+') | Some(b'-')) {
            index += 1;
        }
        let exponent_start = index;
        while matches!(bytes.get(index), Some(b'0'..=b'9')) {
            index += 1;
        }
        if index == exponent_start {
            return false;
        }
    }

    index == bytes.len()
}

pub fn normalize_u64(value: u64) -> serde_json::Value {
    serde_json::Value::String(value.to_string())
}

pub fn normalize_f32(value: f32) -> serde_json::Value {
    serde_json::Value::String(format!("{:08x}", value.to_bits()))
}

pub fn normalize_uuid(value: [u8; 16]) -> serde_json::Value {
    let mut s = String::with_capacity(32);
    for b in value {
        use std::fmt::Write;
        write!(&mut s, "{:02x}", b).unwrap();
    }
    serde_json::Value::String(s)
}

pub fn normalized_ok(category: &str, fields: JsonMap) -> serde_json::Value {
    serde_json::json!({
        "kind": "ok",
        "category": category,
        "fields": serde_json::Value::Object(fields),
    })
}

pub fn normalized_error(
    case: &FrozenCase,
    category: &str,
    rule: &str,
    mut fields: JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    if fields.contains_key("rule") {
        return Err(invalid_case(
            case,
            "fields must not contain reserved key 'rule'",
        ));
    }
    fields.insert(
        "rule".to_string(),
        serde_json::Value::String(rule.to_string()),
    );
    Ok(serde_json::json!({
        "kind": "error",
        "category": category,
        "fields": serde_json::Value::Object(fields),
    }))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::runtime_corpus::{CorpusConsumer, InputFormat};
    use std::sync::atomic::{AtomicBool, Ordering};

    static EXECUTOR_CALLED: AtomicBool = AtomicBool::new(false);

    fn spy_executor(case: &FrozenCase) -> Result<serde_json::Value, DispatchError> {
        EXECUTOR_CALLED.store(true, Ordering::SeqCst);
        Ok(case.normalized.clone())
    }

    fn test_case() -> FrozenCase {
        FrozenCase {
            id: "domain.test/1/sample".to_string(),
            family: "domain.test".to_string(),
            version: "1".to_string(),
            consumer: CorpusConsumer::Domain,
            operation: "admit".to_string(),
            arguments: serde_json::Value::Null,
            input_format: InputFormat::Json,
            input: Vec::new(),
            input_json: None,
            normalized: serde_json::Value::Null,
            encoded: None,
            category: "sample".to_string(),
        }
    }

    #[test]
    fn support_required_missing_null_failure() {
        let case = test_case();
        let mut map = JsonMap::new();
        map.insert("null_key".to_string(), serde_json::Value::Null);

        assert!(required_object(&case, &map, "missing").is_err());
        assert!(required_object(&case, &map, "null_key").is_err());

        assert!(required_array(&case, &map, "missing").is_err());
        assert!(required_array(&case, &map, "null_key").is_err());

        assert!(required_string(&case, &map, "missing").is_err());
        assert!(required_string(&case, &map, "null_key").is_err());

        assert!(required_bool(&case, &map, "missing").is_err());
        assert!(required_bool(&case, &map, "null_key").is_err());

        assert!(required_i8(&case, &map, "missing").is_err());
        assert!(required_i8(&case, &map, "null_key").is_err());
        assert!(required_u8(&case, &map, "missing").is_err());
        assert!(required_u8(&case, &map, "null_key").is_err());
        assert!(required_u16(&case, &map, "missing").is_err());
        assert!(required_u16(&case, &map, "null_key").is_err());
        assert!(required_u32(&case, &map, "missing").is_err());
        assert!(required_u32(&case, &map, "null_key").is_err());
        assert!(required_i32(&case, &map, "missing").is_err());
        assert!(required_i32(&case, &map, "null_key").is_err());
        assert!(required_i64(&case, &map, "missing").is_err());
        assert!(required_i64(&case, &map, "null_key").is_err());
        assert!(required_u64(&case, &map, "missing").is_err());
        assert!(required_u64(&case, &map, "null_key").is_err());
    }

    #[test]
    fn support_optional_missing_null_vs_explicit_empty_false_zero() {
        let case = test_case();
        let mut map = JsonMap::new();
        map.insert("null_val".to_string(), serde_json::Value::Null);
        map.insert(
            "empty_str".to_string(),
            serde_json::Value::String("".to_string()),
        );
        map.insert("false_bool".to_string(), serde_json::Value::Bool(false));
        map.insert("zero_i8".to_string(), serde_json::json!(0));
        map.insert("zero_u8".to_string(), serde_json::json!(0));
        map.insert("zero_u16".to_string(), serde_json::json!(0));
        map.insert("zero_u32".to_string(), serde_json::json!(0));
        map.insert("zero_i32".to_string(), serde_json::json!(0));
        map.insert("zero_i64".to_string(), serde_json::json!(0));
        map.insert("zero_u64".to_string(), serde_json::json!(0));
        map.insert(
            "empty_arr".to_string(),
            serde_json::Value::Array(Vec::new()),
        );
        map.insert(
            "empty_obj".to_string(),
            serde_json::Value::Object(JsonMap::new()),
        );

        assert_eq!(optional_string(&case, &map, "missing").unwrap(), None);
        assert_eq!(optional_string(&case, &map, "null_val").unwrap(), None);
        assert_eq!(optional_string(&case, &map, "empty_str").unwrap(), Some(""));

        assert_eq!(optional_bool(&case, &map, "missing").unwrap(), None);
        assert_eq!(optional_bool(&case, &map, "null_val").unwrap(), None);
        assert_eq!(
            optional_bool(&case, &map, "false_bool").unwrap(),
            Some(false)
        );

        assert_eq!(optional_i8(&case, &map, "missing").unwrap(), None);
        assert_eq!(optional_i8(&case, &map, "null_val").unwrap(), None);
        assert_eq!(optional_i8(&case, &map, "zero_i8").unwrap(), Some(0));

        assert_eq!(optional_u8(&case, &map, "missing").unwrap(), None);
        assert_eq!(optional_u8(&case, &map, "null_val").unwrap(), None);
        assert_eq!(optional_u8(&case, &map, "zero_u8").unwrap(), Some(0));

        assert_eq!(optional_u16(&case, &map, "zero_u16").unwrap(), Some(0));
        assert_eq!(optional_u32(&case, &map, "zero_u32").unwrap(), Some(0));
        assert_eq!(optional_i32(&case, &map, "zero_i32").unwrap(), Some(0));
        assert_eq!(optional_i64(&case, &map, "zero_i64").unwrap(), Some(0));
        assert_eq!(optional_u64(&case, &map, "zero_u64").unwrap(), Some(0));

        assert!(optional_array(&case, &map, "empty_arr").unwrap().is_some());
        assert!(optional_object(&case, &map, "empty_obj").unwrap().is_some());
    }

    #[test]
    fn support_integer_boundaries_and_overflow() {
        let case = test_case();
        // i8
        assert_eq!(
            value_i8(&case, "i8", &serde_json::json!(-128)).unwrap(),
            -128
        );
        assert_eq!(value_i8(&case, "i8", &serde_json::json!(127)).unwrap(), 127);
        assert!(value_i8(&case, "i8", &serde_json::json!(-129)).is_err());
        assert!(value_i8(&case, "i8", &serde_json::json!(128)).is_err());

        // u8
        assert_eq!(value_u8(&case, "u8", &serde_json::json!(0)).unwrap(), 0);
        assert_eq!(value_u8(&case, "u8", &serde_json::json!(255)).unwrap(), 255);
        assert!(value_u8(&case, "u8", &serde_json::json!(-1)).is_err());
        assert!(value_u8(&case, "u8", &serde_json::json!(256)).is_err());

        // u16
        assert_eq!(value_u16(&case, "u16", &serde_json::json!(0)).unwrap(), 0);
        assert_eq!(
            value_u16(&case, "u16", &serde_json::json!(65535)).unwrap(),
            65535
        );
        assert!(value_u16(&case, "u16", &serde_json::json!(-1)).is_err());
        assert!(value_u16(&case, "u16", &serde_json::json!(65536)).is_err());

        // u32
        assert_eq!(value_u32(&case, "u32", &serde_json::json!(0)).unwrap(), 0);
        assert_eq!(
            value_u32(&case, "u32", &serde_json::json!(4294967295u64)).unwrap(),
            4294967295
        );
        assert!(value_u32(&case, "u32", &serde_json::json!(-1)).is_err());
        assert!(value_u32(&case, "u32", &serde_json::json!(4294967296u64)).is_err());

        // i32
        assert_eq!(
            value_i32(&case, "i32", &serde_json::json!(-2147483648i64)).unwrap(),
            -2147483648
        );
        assert_eq!(
            value_i32(&case, "i32", &serde_json::json!(2147483647i64)).unwrap(),
            2147483647
        );
        assert!(value_i32(&case, "i32", &serde_json::json!(-2147483649i64)).is_err());
        assert!(value_i32(&case, "i32", &serde_json::json!(2147483648i64)).is_err());

        // i64
        assert_eq!(
            value_i64(&case, "i64", &serde_json::json!(-9223372036854775808i64)).unwrap(),
            i64::MIN
        );
        assert_eq!(
            value_i64(&case, "i64", &serde_json::json!(9223372036854775807i64)).unwrap(),
            i64::MAX
        );
        // One above i64::MAX is not representable and must be rejected.
        assert!(value_i64(&case, "i64", &serde_json::json!(9223372036854775808u64)).is_err());
        // One below i64::MIN parses as a float, which is never an integral i64.
        assert!(
            value_i64(
                &case,
                "i64",
                &serde_json::from_str::<serde_json::Value>("-9223372036854775809").unwrap()
            )
            .is_err()
        );

        // u64
        assert_eq!(
            value_u64(&case, "u64", &serde_json::json!(18446744073709551615u64)).unwrap(),
            u64::MAX
        );
        // Negative and above-maximum values must be rejected.
        assert!(value_u64(&case, "u64", &serde_json::json!(-1)).is_err());
        assert!(
            value_u64(
                &case,
                "u64",
                &serde_json::from_str::<serde_json::Value>("18446744073709551616").unwrap()
            )
            .is_err()
        );
    }

    #[test]
    fn support_array_element_paths_and_exact_array() {
        let case = test_case();
        let arr = vec![
            serde_json::json!(1),
            serde_json::json!(2),
            serde_json::json!(3),
        ];

        // path preserved in error
        let err = value_i8(&case, "items[1]", &serde_json::json!("not a number")).unwrap_err();
        match err {
            DispatchError::InvalidCase { message, .. } => {
                assert!(message.contains("items[1]"), "error must contain path");
            }
            _ => panic!("unexpected error type"),
        }

        // exact array at N-1, N, N+1
        assert!(exact_array::<2>(&case, "arr", &arr).is_err());
        assert!(exact_array::<3>(&case, "arr", &arr).is_ok());
        assert!(exact_array::<4>(&case, "arr", &arr).is_err());
    }

    #[test]
    fn support_parse_uuid_hex() {
        let case = test_case();
        // valid lowercase 32 hex
        let valid = "0123456789abcdef0123456789abcdef";
        let parsed = parse_uuid_hex(&case, "uuid", valid).unwrap();
        assert_eq!(
            parsed,
            [
                0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab,
                0xcd, 0xef,
            ]
        );

        // length != 32
        assert!(parse_uuid_hex(&case, "uuid", "0123").is_err());
        assert!(parse_uuid_hex(&case, "uuid", "0123456789abcdef0123456789abcdef0").is_err());

        // uppercase rejected
        assert!(parse_uuid_hex(&case, "uuid", "0123456789ABCDEF0123456789abcdef").is_err());

        // non-hex rejected
        assert!(parse_uuid_hex(&case, "uuid", "0123456789abcdef0123456789abcdeg").is_err());

        // A multi-byte character must be rejected rather than splitting a UTF-8
        // boundary while slicing fixed-width hex pairs. This token is exactly 32
        // bytes long and starts its two-byte character at an odd byte offset.
        let split_boundary = format!("{}é{}", "a".repeat(29), "b");
        assert_eq!(split_boundary.len(), 32);
        assert!(parse_uuid_hex(&case, "uuid", &split_boundary).is_err());
    }

    #[test]
    fn support_parse_f32_token() {
        let case = test_case();
        // NaN, Inf, +Inf, -Inf
        assert!(parse_f32_token(&case, "token", "NaN").unwrap().is_nan());
        let pos_inf = parse_f32_token(&case, "token", "Inf").unwrap();
        assert!(pos_inf.is_infinite() && pos_inf.is_sign_positive());
        let plus_inf = parse_f32_token(&case, "token", "+Inf").unwrap();
        assert!(plus_inf.is_infinite() && plus_inf.is_sign_positive());
        let neg_inf = parse_f32_token(&case, "token", "-Inf").unwrap();
        assert!(neg_inf.is_infinite() && neg_inf.is_sign_negative());

        // negative zero bit preservation
        let neg_zero = parse_f32_token(&case, "token", "-0").unwrap();
        assert_eq!(neg_zero.to_bits(), (-0.0f32).to_bits());
        let neg_zero_float = parse_f32_token(&case, "token", "-0.0").unwrap();
        assert_eq!(neg_zero_float.to_bits(), (-0.0f32).to_bits());
        let exponent_neg_zero = parse_f32_token(&case, "token", "-0e0").unwrap();
        assert_eq!(exponent_neg_zero.to_bits(), (-0.0f32).to_bits());

        // Finite decimal grammar includes signed exponents and either side of
        // the decimal point when the mantissa still contains a digit.
        for (token, expected) in [
            ("1.5", 1.5f32),
            ("1e-3", 0.001f32),
            ("1E+3", 1000.0f32),
            (".5e2", 50.0f32),
            ("1.", 1.0f32),
        ] {
            assert_eq!(parse_f32_token(&case, "token", token).unwrap(), expected);
        }

        // Malformed spellings, aliases, whitespace, hexadecimal notation and
        // decimal overflow are hard input failures.
        for token in [
            "e3", "1e", "1e+", "--1", "0x1p0", "1_0", " 1", "1 ", "1e1000", "infinity", "invalid",
        ] {
            assert!(
                parse_f32_token(&case, "token", token).is_err(),
                "token '{token}' must be rejected"
            );
        }

        // A decimal token with no exponent letter that overflows f32 must be a
        // hard failure, not a silently accepted infinity.
        assert!(
            parse_f32_token(&case, "token", "9999999999999999999999999999999999999999").is_err()
        );
    }

    #[test]
    fn support_normalization_formatting() {
        assert_eq!(
            normalize_u64(42),
            serde_json::Value::String("42".to_string())
        );
        assert_eq!(
            normalize_f32(1.0f32),
            serde_json::Value::String("3f800000".to_string())
        );
        let uuid_bytes = [
            0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef, 0x01, 0x23, 0x45, 0x67, 0x89, 0xab,
            0xcd, 0xef,
        ];
        assert_eq!(
            normalize_uuid(uuid_bytes),
            serde_json::Value::String("0123456789abcdef0123456789abcdef".to_string())
        );
    }

    #[test]
    fn support_normalized_error_reserved_rule_rejection() {
        let case = test_case();
        let mut fields = JsonMap::new();
        fields.insert(
            "rule".to_string(),
            serde_json::Value::String("already_present".to_string()),
        );
        assert!(normalized_error(&case, "cat", "rule", fields).is_err());

        let mut fields2 = JsonMap::new();
        fields2.insert("key".to_string(), serde_json::Value::Bool(true));
        let err_json = normalized_error(&case, "invalid-val", "my.rule", fields2).unwrap();
        assert_eq!(err_json["kind"], "error");
        assert_eq!(err_json["category"], "invalid-val");
        assert_eq!(err_json["fields"]["rule"], "my.rule");
        assert_eq!(err_json["fields"]["key"], true);
    }

    #[test]
    fn support_comparator_ignores_wire_for_commands_only() {
        let mut case = test_case();
        case.family = "domain.command_control".to_string();
        case.normalized = serde_json::json!({
            "kind": "ok",
            "category": "player-input",
            "fields": {
                "sequence": "1",
                "wire": "aabbcc"
            }
        });

        // Actual output does NOT have wire
        let executed = ExecutedCase {
            case: case.clone(),
            actual: serde_json::json!({
                "kind": "ok",
                "category": "player-input",
                "fields": {
                    "sequence": "1"
                }
            }),
        };
        // Should succeed because wire is stripped from expected clone
        assert_domain_normalized(&executed);

        // But non-wire semantic mutation MUST fail
        let executed_mutated = ExecutedCase {
            case: case.clone(),
            actual: serde_json::json!({
                "kind": "ok",
                "category": "player-input",
                "fields": {
                    "sequence": "2"
                }
            }),
        };
        let res = std::panic::catch_unwind(|| {
            assert_domain_normalized(&executed_mutated);
        });
        assert!(res.is_err(), "semantic difference must fail comparison");

        // Error cases in command_control should NOT have wire stripped
        let mut err_case = test_case();
        err_case.family = "domain.command_control".to_string();
        err_case.normalized = serde_json::json!({
            "kind": "error",
            "category": "invalid-value",
            "fields": {
                "rule": "player_input.finite_rotation",
                "wire": "1234"
            }
        });
        let executed_err = ExecutedCase {
            case: err_case.clone(),
            actual: serde_json::json!({
                "kind": "error",
                "category": "invalid-value",
                "fields": {
                    "rule": "player_input.finite_rotation"
                }
            }),
        };
        let res_err = std::panic::catch_unwind(|| {
            assert_domain_normalized(&executed_err);
        });
        assert!(res_err.is_err(), "wire in error must NOT be stripped");
    }

    #[test]
    fn support_execute_topic_rejects_zero_expected_count() {
        // A zero count contradicts the "exact nonzero count" contract, so it must
        // be reported as an invalid case instead of aborting the process.
        let result = execute_topic(0, |_| false, |case| Ok(case.normalized.clone()));
        match result {
            Err(DispatchError::InvalidCase { case_id, .. }) => {
                assert_eq!(case_id, "execute_topic");
            }
            Err(DispatchError::NotImplemented { .. }) => panic!("unexpected NotImplemented error"),
            Ok(_) => panic!("zero expected count must not be accepted"),
        }
    }

    #[test]
    fn support_execute_checked_validates_consumer_before_execution() {
        let mut case = test_case();
        for (label, consumer) in [
            ("missing", None),
            ("null", Some(serde_json::Value::Null)),
            ("number", Some(serde_json::json!(7))),
            ("other known", Some(serde_json::json!("corpus_frame"))),
        ] {
            let mut input = JsonMap::new();
            if let Some(consumer) = consumer {
                input.insert("consumer".to_string(), consumer);
            }
            case.input_json = Some(serde_json::Value::Object(input));
            EXECUTOR_CALLED.store(false, Ordering::SeqCst);

            let error = match execute_checked(&case, spy_executor) {
                Err(error) => error,
                Ok(_) => panic!("{label} consumer must fail"),
            };
            assert!(
                matches!(error, DispatchError::InvalidCase { .. }),
                "{label} consumer must be an invalid case"
            );
            assert!(
                !EXECUTOR_CALLED.load(Ordering::SeqCst),
                "{label} consumer must fail before execution"
            );
        }

        case.input_json = Some(serde_json::json!({ "consumer": "mornlea_domain" }));
        EXECUTOR_CALLED.store(false, Ordering::SeqCst);
        execute_checked(&case, spy_executor).expect("domain consumer must execute");
        assert!(EXECUTOR_CALLED.load(Ordering::SeqCst));
    }
}
