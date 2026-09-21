//! Topic module for domain.event world corpus cases.
//!
//! The three rules here are the compact world observations an authoritative
//! session publishes: the initial 24-section `chunk-snapshot` column, the
//! per-tick `block-changes` delta, and the `forget-chunks` batch that retires
//! columns. Each executor parses the case input, selects the Go rejection
//! rule in the producer's own precedence, and then proves the matching public
//! Rust constructor agrees: a classified acceptance must construct, and a
//! classified rejection must return the mapped `DomainError`. Accepted
//! fields are read from the constructed Rust values; a rejected value has no
//! getters, so its projection is built from the parsed input instead.
//!
//! This family travels server-to-client: no record enters a
//! `CommandEnvelope`, and the 4096 wire batch caps and the wire-only section
//! Y stay in the protocol layer, so this adapter imposes neither. The empty
//! block-change batch is the legal revision barrier the Go validator allows
//! and is executed as an accepted record.

use super::support::{
    DispatchError, ExecutedCase, JsonMap, assert_domain_normalized, exact_array, execute_topic,
    input_object, invalid_case, normalize_u64, normalized_error, normalized_ok, optional_array,
    optional_u8, optional_u16, required_array, required_i32, required_string, required_u8,
    required_u16, required_u64, value_array, value_i32, value_object, value_u16, value_u64,
};
use crate::runtime_corpus::{CorpusConsumer, FrozenCase};
use mornlea_domain::{
    BlockChange, BlockChanges, BlockChangesParts, BlockPos, ChunkPos, ChunkSnapshot,
    ChunkSnapshotParts, Dimension, DomainError, ForgetChunks, ForgetChunksParts, PalettedSection,
};
use serde_json::Value;

pub const EXPECTED_COUNT: usize = 38;

const RULES: &[&str] = &["chunk-snapshot", "block-changes", "forget-chunks"];

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
        "chunk-snapshot" => execute_chunk_snapshot(case, input),
        "block-changes" => execute_block_changes(case, input),
        "forget-chunks" => execute_forget_chunks(case, input),
        unknown => Err(invalid_case(
            case,
            format!("unknown world event rule '{unknown}'"),
        )),
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

/// Overworld dimension ID, from the Go `core.Overworld`.
const DIMENSION_OVERWORLD: i32 = 0;

/// Depths dimension ID, from the Go `core.Depths`.
const DIMENSION_DEPTHS: i32 = 1;

/// Sections in one chunk column, from the Go `core.SectionsPerChunk`.
const SECTIONS_PER_CHUNK: usize = 24;

/// Section storage kind `protocol.SectionSingle`.
const STORAGE_SINGLE: u8 = 0;

/// Section storage kind `protocol.SectionIndexed`.
const STORAGE_INDEXED: u8 = 1;

/// Section storage kind `protocol.SectionDirect`.
const STORAGE_DIRECT: u8 = 2;

/// Selects the first rule one raw dimension breaks, in the shape all three
/// world records share.
///
/// A fitting byte reaches `Dimension::new` and is verified there; a negative
/// or above-byte dimension cannot enter any Rust value, so only the
/// classifier decides. Each rule prefixes the shared name with its own
/// subject.
fn dimension_rejection(dimension: i32, rule: &'static str) -> Option<Rejection> {
    if dimension == DIMENSION_OVERWORLD || dimension == DIMENSION_DEPTHS {
        return None;
    }
    Some(match u8::try_from(dimension) {
        Ok(_) => Rejection {
            category: "invalid-enum",
            rule,
            error: Some(DomainError::InvalidDimension),
        },
        Err(_) => Rejection::classifier("invalid-enum", rule),
    })
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

/// The parsed shape of one wire section: the storage kind plus exactly the
/// fields that kind names. The palette keeps absent (`None`) and explicit
/// empty (`Some(empty)`) distinct, and a nil packed list renders as empty
/// exactly as the Go producer's nil slice does.
enum SectionShape {
    Single {
        block: u16,
    },
    Indexed {
        bits: u8,
        palette: Vec<u16>,
        packed: Vec<u64>,
    },
    Direct {
        packed: Vec<u64>,
    },
}

/// The parsed raw shape of one chunk snapshot.
struct RawSnapshot {
    dimension: i32,
    chunk: [i32; 2],
    revision: u64,
    sections: Vec<SectionShape>,
}

/// The parsed raw shape of one block-change batch.
struct RawChanges {
    dimension: i32,
    chunk: [i32; 2],
    base_revision: u64,
    new_revision: u64,
    changes: Vec<([i32; 3], u16)>,
}

/// The parsed raw shape of one forget batch.
struct RawForget {
    dimension: i32,
    chunks: Vec<[i32; 2]>,
}

/// Executes one `chunk-snapshot` case, the only record in this family that
/// carries a section array.
fn execute_chunk_snapshot(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let raw = parse_snapshot(case, input)?;

    match snapshot_rejection(&raw) {
        Some(rejection) => {
            verify_snapshot_rejection(case, &raw, &rejection)?;
            normalized_error(
                case,
                rejection.category,
                rejection.rule,
                snapshot_projection(&raw),
            )
        }
        None => {
            let snapshot = construct_snapshot(case, &raw)?;
            Ok(normalized_ok("chunk-snapshot", snapshot_fields(&snapshot)))
        }
    }
}

/// Executes one `block-changes` case, the per-tick delta that continues one
/// chunk history. The explicit empty batch is the accepted revision barrier.
fn execute_block_changes(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let raw = parse_block_changes(case, input)?;

    match block_changes_rejection(&raw) {
        Some(rejection) => {
            verify_block_changes_rejection(case, &raw, &rejection)?;
            normalized_error(
                case,
                rejection.category,
                rejection.rule,
                changes_projection(&raw),
            )
        }
        None => {
            let batch = construct_block_changes(case, &raw)?;
            Ok(normalized_ok("block-changes", changes_fields(&batch)))
        }
    }
}

/// Executes one `forget-chunks` case, whose entire payload is the dimension
/// and the chunk list. Submitted order is preserved in every projection.
fn execute_forget_chunks(
    case: &FrozenCase,
    input: &JsonMap,
) -> Result<serde_json::Value, DispatchError> {
    let raw = parse_forget(case, input)?;

    match forget_rejection(&raw) {
        Some(rejection) => {
            verify_forget_rejection(case, &raw, &rejection)?;
            normalized_error(
                case,
                rejection.category,
                rejection.rule,
                forget_projection(&raw),
            )
        }
        None => {
            let forget = construct_forget(case, &raw)?;
            Ok(normalized_ok("forget-chunks", forget_fields(&forget)))
        }
    }
}

/// Parses one chunk snapshot. Every field is required, because the record is
/// complete by definition: a missing field or a value outside its declared
/// width is a hard schema failure, never a normalized rejection.
fn parse_snapshot(case: &FrozenCase, input: &JsonMap) -> Result<RawSnapshot, DispatchError> {
    let dimension = required_i32(case, input, "dimension")?;
    let chunk = parse_chunk(case, input)?;
    let revision = required_u64(case, input, "revision")?;
    let section_values = required_array(case, input, "sections")?;
    let mut sections = Vec::with_capacity(section_values.len());
    for (index, value) in section_values.iter().enumerate() {
        sections.push(parse_section(case, &format!("sections[{index}]"), value)?);
    }
    Ok(RawSnapshot {
        dimension,
        chunk,
        revision,
        sections,
    })
}

/// Parses one block-change batch. The `changes` key is required even when the
/// batch is the explicit empty revision barrier.
fn parse_block_changes(case: &FrozenCase, input: &JsonMap) -> Result<RawChanges, DispatchError> {
    let dimension = required_i32(case, input, "dimension")?;
    let chunk = parse_chunk(case, input)?;
    let base_revision = required_u64(case, input, "base_revision")?;
    let new_revision = required_u64(case, input, "new_revision")?;
    let change_values = required_array(case, input, "changes")?;
    let mut changes = Vec::with_capacity(change_values.len());
    for (index, value) in change_values.iter().enumerate() {
        let path = format!("changes[{index}]");
        let change = value_object(case, &path, value)?;
        let position_values = required_array(case, change, "position")?;
        let position_path = format!("{path}.position");
        let slots = exact_array::<3>(case, &position_path, position_values)?;
        let position = [
            value_i32(case, &format!("{position_path}[0]"), &slots[0])?,
            value_i32(case, &format!("{position_path}[1]"), &slots[1])?,
            value_i32(case, &format!("{position_path}[2]"), &slots[2])?,
        ];
        let block = required_u16(case, change, "block")?;
        changes.push((position, block));
    }
    Ok(RawChanges {
        dimension,
        chunk,
        base_revision,
        new_revision,
        changes,
    })
}

/// Parses one forget batch. An empty batch is carried, not absent, exactly as
/// the producer's pointer rendering keeps it.
fn parse_forget(case: &FrozenCase, input: &JsonMap) -> Result<RawForget, DispatchError> {
    let dimension = required_i32(case, input, "dimension")?;
    let chunk_values = required_array(case, input, "chunks")?;
    let mut chunks = Vec::with_capacity(chunk_values.len());
    for (index, value) in chunk_values.iter().enumerate() {
        let path = format!("chunks[{index}]");
        let coordinates = value_array(case, &path, value)?;
        let slots = exact_array::<2>(case, &path, coordinates)?;
        chunks.push([
            value_i32(case, &format!("{path}[0]"), &slots[0])?,
            value_i32(case, &format!("{path}[1]"), &slots[1])?,
        ]);
    }
    Ok(RawForget { dimension, chunks })
}

/// Parses the shared two-coordinate chunk field.
fn parse_chunk(case: &FrozenCase, input: &JsonMap) -> Result<[i32; 2], DispatchError> {
    let values = required_array(case, input, "chunk")?;
    let slots = exact_array::<2>(case, "chunk", values)?;
    Ok([
        value_i32(case, "chunk[0]", &slots[0])?,
        value_i32(case, "chunk[1]", &slots[1])?,
    ])
}

/// Parses one section from its frozen rendering. An unknown storage kind has
/// no Go normalized rule, so it is a schema failure, and the fields each
/// storage kind names are required exactly where the producer requires them.
fn parse_section(
    case: &FrozenCase,
    path: &str,
    value: &Value,
) -> Result<SectionShape, DispatchError> {
    let section = value_object(case, path, value)?;
    let storage = required_u8(case, section, "storage")?;
    match storage {
        STORAGE_SINGLE => {
            let block = optional_u16(case, section, "single")?
                .ok_or_else(|| invalid_case(case, format!("{path} requires a single block")))?;
            Ok(SectionShape::Single { block })
        }
        STORAGE_INDEXED => {
            let bits = optional_u8(case, section, "bits")?
                .ok_or_else(|| invalid_case(case, format!("{path} requires bits")))?;
            let palette_values = optional_array(case, section, "palette")?
                .ok_or_else(|| invalid_case(case, format!("{path} requires a palette")))?;
            let mut palette = Vec::with_capacity(palette_values.len());
            for (index, entry) in palette_values.iter().enumerate() {
                palette.push(value_u16(case, &format!("{path}.palette[{index}]"), entry)?);
            }
            let packed = parse_words(case, path, section)?;
            Ok(SectionShape::Indexed {
                bits,
                palette,
                packed,
            })
        }
        STORAGE_DIRECT => Ok(SectionShape::Direct {
            packed: parse_words(case, path, section)?,
        }),
        other => Err(invalid_case(
            case,
            format!("{path} names unknown storage {other}"),
        )),
    }
}

/// Parses one section's packed-word list. A nil list renders as empty, which
/// the owning constructor then rejects wherever an exact count is required.
fn parse_words(
    case: &FrozenCase,
    path: &str,
    section: &JsonMap,
) -> Result<Vec<u64>, DispatchError> {
    let mut words = Vec::new();
    if let Some(values) = optional_array(case, section, "packed")? {
        for (index, entry) in values.iter().enumerate() {
            words.push(value_u64(case, &format!("{path}.packed[{index}]"), entry)?);
        }
    }
    Ok(words)
}

/// Builds one section through the real constructor. Every raw section value
/// can enter its constructor, so section admission is decided there and the
/// raw classifier never re-implements a section rule.
fn build_section(shape: &SectionShape) -> Result<PalettedSection, DomainError> {
    match shape {
        SectionShape::Single { block } => PalettedSection::single(*block),
        SectionShape::Indexed {
            bits,
            palette,
            packed,
        } => PalettedSection::indexed(
            *bits,
            palette.clone().into_boxed_slice(),
            packed.clone().into_boxed_slice(),
        ),
        SectionShape::Direct { packed } => {
            PalettedSection::direct(packed.clone().into_boxed_slice())
        }
    }
}

/// Maps one section constructor verdict to its published rule, prefixed with
/// the snapshot subject. The palette length disambiguates the two
/// `InvalidSectionPalette` rules the way the Go validator's own check order
/// does: bounds before per-entry uniqueness. An unmapped verdict is a
/// contract conflict the caller reports as `InvalidCase`.
fn section_rule(shape: &SectionShape, error: &DomainError) -> Option<(&'static str, &'static str)> {
    match (shape, error) {
        (SectionShape::Single { .. }, DomainError::InvalidBlock) => {
            Some(("invalid-enum", "chunk_snapshot.section_single_registered"))
        }
        (SectionShape::Indexed { .. }, DomainError::InvalidSectionBits) => {
            Some(("invalid-enum", "chunk_snapshot.section_indexed_bits"))
        }
        (SectionShape::Indexed { bits, palette, .. }, DomainError::InvalidSectionPalette) => {
            if palette.is_empty() || palette.len() > 1usize << bits {
                Some(("invalid-value", "chunk_snapshot.section_palette_bounds"))
            } else {
                Some(("invalid-enum", "chunk_snapshot.section_palette_unique"))
            }
        }
        (SectionShape::Indexed { .. }, DomainError::InvalidBlock) => {
            Some(("invalid-enum", "chunk_snapshot.section_palette_registered"))
        }
        (SectionShape::Indexed { .. }, DomainError::InvalidSectionWords) => {
            Some(("invalid-value", "chunk_snapshot.section_words_exact"))
        }
        (SectionShape::Indexed { .. }, DomainError::InvalidSectionSlot) => {
            Some(("invalid-enum", "chunk_snapshot.section_slot_in_palette"))
        }
        (SectionShape::Direct { .. }, DomainError::InvalidSectionWords) => {
            Some(("invalid-value", "chunk_snapshot.section_words_exact"))
        }
        (SectionShape::Direct { .. }, DomainError::InvalidSectionHighBits) => {
            Some(("invalid-value", "chunk_snapshot.section_direct_high_bits"))
        }
        (SectionShape::Direct { .. }, DomainError::InvalidBlock) => Some((
            "invalid-enum",
            "chunk_snapshot.section_direct_block_registered",
        )),
        _ => None,
    }
}

/// Selects the first rule one section breaks, from the owning constructor's
/// verdict.
fn section_rejection(shape: &SectionShape) -> Option<Rejection> {
    let error = build_section(shape).err()?;
    let (category, rule) = section_rule(shape, &error)?;
    Some(Rejection {
        category,
        rule,
        error: Some(error),
    })
}

/// Selects the first rule one snapshot breaks, in the same order the Go
/// validator checks them: the dimension, the revision, the column length,
/// and then every section in column order.
fn snapshot_rejection(raw: &RawSnapshot) -> Option<Rejection> {
    if let Some(rejection) = dimension_rejection(raw.dimension, "chunk_snapshot.dimension") {
        return Some(rejection);
    }
    if raw.revision == 0 {
        return Some(Rejection {
            category: "invalid-value",
            rule: "chunk_snapshot.revision_nonzero",
            error: Some(DomainError::InvalidRevision),
        });
    }
    if raw.sections.len() != SECTIONS_PER_CHUNK {
        // The Rust snapshot carries the fixed 24-section array, so a wrong
        // column length cannot enter any constructor: the classifier alone
        // decides.
        return Some(Rejection::classifier(
            "invalid-value",
            "chunk_snapshot.section_count",
        ));
    }
    for shape in &raw.sections {
        if let Some(rejection) = section_rejection(shape) {
            return Some(rejection);
        }
    }
    None
}

/// Builds the fixed 24-section column through the real section constructors.
/// Any build failure here means the classified rule and the constructors
/// disagree, so the topic fails closed.
fn build_column(
    case: &FrozenCase,
    shapes: &[SectionShape],
) -> Result<Box<[PalettedSection; SECTIONS_PER_CHUNK]>, DispatchError> {
    let mut sections = Vec::with_capacity(shapes.len());
    for (index, shape) in shapes.iter().enumerate() {
        sections.push(build_section(shape).map_err(|_| {
            classification_conflict(case, "chunk-snapshot", &format!("sections[{index}]"))
        })?);
    }
    Box::<[PalettedSection; SECTIONS_PER_CHUNK]>::try_from(sections.into_boxed_slice())
        .map_err(|_| classification_conflict(case, "chunk-snapshot", "section_count"))
}

/// Verifies the constructor agreement for one classified snapshot rejection.
fn verify_snapshot_rejection(
    case: &FrozenCase,
    raw: &RawSnapshot,
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    // Section rules are classified by the owning constructor itself in
    // `section_rejection`, so the verdict quoted there is already the
    // constructor's and needs no second call.
    if rejection.rule.starts_with("chunk_snapshot.section_") {
        return Ok(());
    }
    let verdict = match rejection.rule {
        "chunk_snapshot.dimension" => Dimension::new(raw.dimension as u8).map(|_| ()),
        // The revision rule is checked before any section is examined, so a
        // column that cannot enter the fixed array leaves no constructor
        // verdict to obtain.
        "chunk_snapshot.revision_nonzero" => match build_column(case, &raw.sections) {
            Ok(sections) => snapshot_value(raw, raw.dimension as u8, sections).map(|_| ()),
            Err(_) => Ok(()),
        },
        _ => {
            return Err(classification_conflict(
                case,
                "chunk-snapshot",
                rejection.rule,
            ));
        }
    };
    match verdict {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(
            case,
            "chunk-snapshot",
            rejection.rule,
        )),
    }
}

/// Assembles one snapshot value from already-parsed parts.
fn snapshot_value(
    raw: &RawSnapshot,
    dimension_id: u8,
    sections: Box<[PalettedSection; SECTIONS_PER_CHUNK]>,
) -> Result<ChunkSnapshot, DomainError> {
    ChunkSnapshot::try_new(ChunkSnapshotParts {
        dimension: Dimension::new(dimension_id)?,
        chunk: ChunkPos::new(raw.chunk[0], raw.chunk[1]),
        revision: raw.revision,
        sections,
    })
}

/// Constructs the full checked chain for one admitted snapshot: the real
/// section constructors, the exact fixed-array conversion, and the snapshot
/// wrapper whose revision rule is the only relation left to check.
fn construct_snapshot(
    case: &FrozenCase,
    raw: &RawSnapshot,
) -> Result<ChunkSnapshot, DispatchError> {
    let dimension_id = u8::try_from(raw.dimension)
        .map_err(|_| classification_conflict(case, "chunk-snapshot", "dimension"))?;
    let sections = build_column(case, &raw.sections)?;
    snapshot_value(raw, dimension_id, sections)
        .map_err(|_| classification_conflict(case, "chunk-snapshot", ""))
}

/// Builds the accepted snapshot projection from the constructed record's
/// accessors, preserving column order exactly.
fn snapshot_fields(snapshot: &ChunkSnapshot) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert(
        "dimension".to_string(),
        Value::from(snapshot.dimension().get()),
    );
    fields.insert(
        "chunk".to_string(),
        Value::Array(vec![
            Value::from(snapshot.chunk().x()),
            Value::from(snapshot.chunk().z()),
        ]),
    );
    fields.insert("revision".to_string(), normalize_u64(snapshot.revision()));
    fields.insert(
        "sections".to_string(),
        Value::Array(
            snapshot
                .sections()
                .iter()
                .map(|section| Value::Object(section_fields(section)))
                .collect(),
        ),
    );
    fields
}

/// Builds the rejected snapshot projection from the parsed input, because a
/// rejected Rust value has no getters to read.
fn snapshot_projection(raw: &RawSnapshot) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("dimension".to_string(), Value::from(raw.dimension));
    fields.insert(
        "chunk".to_string(),
        Value::Array(vec![Value::from(raw.chunk[0]), Value::from(raw.chunk[1])]),
    );
    fields.insert("revision".to_string(), normalize_u64(raw.revision));
    fields.insert(
        "sections".to_string(),
        Value::Array(
            raw.sections
                .iter()
                .map(|shape| Value::Object(section_shape_fields(shape)))
                .collect(),
        ),
    );
    fields
}

/// Projects one constructed section's representation: the single block, the
/// indexed width with its ordered palette and packed words, or the direct
/// packed words.
fn section_fields(section: &PalettedSection) -> JsonMap {
    if let Some(block) = section.as_single() {
        let mut fields = JsonMap::new();
        fields.insert("storage".to_string(), Value::String("single".to_string()));
        fields.insert("single".to_string(), Value::from(block));
        return fields;
    }
    if let Some((bits, palette, packed)) = section.as_indexed() {
        let mut fields = JsonMap::new();
        fields.insert("storage".to_string(), Value::String("indexed".to_string()));
        fields.insert("bits".to_string(), Value::from(bits));
        fields.insert(
            "palette".to_string(),
            Value::Array(palette.iter().map(|id| Value::from(*id)).collect()),
        );
        fields.insert("packed".to_string(), words_value(packed));
        return fields;
    }
    if let Some(packed) = section.as_direct() {
        let mut fields = JsonMap::new();
        fields.insert("storage".to_string(), Value::String("direct".to_string()));
        fields.insert("packed".to_string(), words_value(packed));
        return fields;
    }
    unreachable!("PalettedSection has no representation outside the three accessors")
}

/// Projects one parsed section for a rejected snapshot: the same fields its
/// storage kind names, read from the parsed input.
fn section_shape_fields(shape: &SectionShape) -> JsonMap {
    match shape {
        SectionShape::Single { block } => {
            let mut fields = JsonMap::new();
            fields.insert("storage".to_string(), Value::String("single".to_string()));
            fields.insert("single".to_string(), Value::from(*block));
            fields
        }
        SectionShape::Indexed {
            bits,
            palette,
            packed,
        } => {
            let mut fields = JsonMap::new();
            fields.insert("storage".to_string(), Value::String("indexed".to_string()));
            fields.insert("bits".to_string(), Value::from(*bits));
            fields.insert(
                "palette".to_string(),
                Value::Array(palette.iter().map(|id| Value::from(*id)).collect()),
            );
            fields.insert("packed".to_string(), words_value(packed));
            fields
        }
        SectionShape::Direct { packed } => {
            let mut fields = JsonMap::new();
            fields.insert("storage".to_string(), Value::String("direct".to_string()));
            fields.insert("packed".to_string(), words_value(packed));
            fields
        }
    }
}

/// Renders packed words as the decimal strings the normalized vocabulary
/// uses for a u64 field.
fn words_value(words: &[u64]) -> Value {
    Value::Array(words.iter().map(|word| normalize_u64(*word)).collect())
}

/// Builds one ordered list of per-change values through the real constructor.
/// Any unexpected failure means the classified rule and the constructors
/// disagree, so the topic fails closed.
fn build_change_list(
    case: &FrozenCase,
    raw: &RawChanges,
) -> Result<Box<[BlockChange]>, DispatchError> {
    let mut changes = Vec::with_capacity(raw.changes.len());
    for (index, (position, block)) in raw.changes.iter().enumerate() {
        match BlockChange::try_new(BlockPos::new(position[0], position[1], position[2]), *block) {
            Ok(change) => changes.push(change),
            Err(_) => {
                return Err(classification_conflict(
                    case,
                    "block-changes",
                    &format!("changes[{index}]"),
                ));
            }
        }
    }
    Ok(changes.into_boxed_slice())
}

/// Assembles one batch value from already-parsed parts.
fn changes_value(
    raw: &RawChanges,
    dimension_id: u8,
    changes: Box<[BlockChange]>,
) -> Result<BlockChanges, DomainError> {
    BlockChanges::try_new(BlockChangesParts {
        dimension: Dimension::new(dimension_id)?,
        chunk: ChunkPos::new(raw.chunk[0], raw.chunk[1]),
        base_revision: raw.base_revision,
        new_revision: raw.new_revision,
        changes,
    })
}

/// Selects the first rule one change batch breaks, in the same order the Go
/// validator checks them: the dimension, the revision transition, and then
/// every change's block, Y span, chunk membership and canonical index.
///
/// The revision transition is selected from the raw values because a
/// saturated base cannot be continued without wrapping. The per-change block
/// rule runs the real constructor in input order, and the batch relations
/// come from the owning constructor, whose per-change order matches the Go
/// validator exactly after the block rule.
fn block_changes_rejection(raw: &RawChanges) -> Option<Rejection> {
    if let Some(rejection) = dimension_rejection(raw.dimension, "block_changes.dimension") {
        return Some(rejection);
    }
    if raw.base_revision == 0
        || raw.base_revision == u64::MAX
        || raw.new_revision != raw.base_revision + 1
    {
        return Some(Rejection {
            category: "invalid-value",
            rule: "block_changes.revision_transition",
            error: Some(DomainError::InvalidRevision),
        });
    }
    for (position, block) in &raw.changes {
        match BlockChange::try_new(BlockPos::new(position[0], position[1], position[2]), *block) {
            Ok(_) => {}
            Err(DomainError::InvalidBlock) => {
                return Some(Rejection {
                    category: "invalid-enum",
                    rule: "block_changes.block_registered",
                    error: Some(DomainError::InvalidBlock),
                });
            }
            // Another verdict cannot occur; the accepted path below fails
            // closed through its own construction if one ever did.
            Err(_) => return None,
        }
    }
    let changes: Vec<BlockChange> = raw
        .changes
        .iter()
        .map(|(position, block)| {
            BlockChange::try_new(BlockPos::new(position[0], position[1], position[2]), *block)
                .expect("change already constructed above")
        })
        .collect();
    match changes_value(raw, raw.dimension as u8, changes.into_boxed_slice()) {
        Ok(_) => None,
        Err(DomainError::InvalidBlockY) => Some(Rejection {
            category: "invalid-value",
            rule: "block_changes.position_y_span",
            error: Some(DomainError::InvalidBlockY),
        }),
        Err(DomainError::InvalidBlockChunk) => Some(Rejection {
            category: "invalid-value",
            rule: "block_changes.position_chunk",
            error: Some(DomainError::InvalidBlockChunk),
        }),
        Err(DomainError::InvalidChangeOrder) => Some(Rejection {
            category: "invalid-value",
            rule: "block_changes.strictly_increasing_index",
            error: Some(DomainError::InvalidChangeOrder),
        }),
        // Another verdict cannot occur after the raw transition check; the
        // accepted path below fails closed through its own construction if
        // one ever did.
        Err(_) => None,
    }
}

/// Verifies the constructor agreement for one classified batch rejection.
fn verify_block_changes_rejection(
    case: &FrozenCase,
    raw: &RawChanges,
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let verdict = match rejection.rule {
        "block_changes.dimension" => Dimension::new(raw.dimension as u8).map(|_| ()),
        "block_changes.revision_transition"
        | "block_changes.position_y_span"
        | "block_changes.position_chunk"
        | "block_changes.strictly_increasing_index" => {
            let changes = build_change_list(case, raw)?;
            let dimension_id = u8::try_from(raw.dimension)
                .map_err(|_| classification_conflict(case, "block-changes", "dimension"))?;
            changes_value(raw, dimension_id, changes).map(|_| ())
        }
        "block_changes.block_registered" => {
            let mut verdict = Ok(());
            for (position, block) in &raw.changes {
                match BlockChange::try_new(
                    BlockPos::new(position[0], position[1], position[2]),
                    *block,
                ) {
                    Ok(_) => {}
                    Err(error) => {
                        verdict = Err(error);
                        break;
                    }
                }
            }
            verdict
        }
        _ => {
            return Err(classification_conflict(
                case,
                "block-changes",
                rejection.rule,
            ));
        }
    };
    match verdict {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(
            case,
            "block-changes",
            rejection.rule,
        )),
    }
}

/// Constructs the full checked chain for one admitted batch.
fn construct_block_changes(
    case: &FrozenCase,
    raw: &RawChanges,
) -> Result<BlockChanges, DispatchError> {
    let changes = build_change_list(case, raw)?;
    let dimension_id = u8::try_from(raw.dimension)
        .map_err(|_| classification_conflict(case, "block-changes", "dimension"))?;
    changes_value(raw, dimension_id, changes)
        .map_err(|_| classification_conflict(case, "block-changes", ""))
}

/// Builds the accepted batch projection from the constructed record's
/// getters, preserving change order exactly.
fn changes_fields(batch: &BlockChanges) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert(
        "dimension".to_string(),
        Value::from(batch.dimension().get()),
    );
    fields.insert(
        "chunk".to_string(),
        Value::Array(vec![
            Value::from(batch.chunk().x()),
            Value::from(batch.chunk().z()),
        ]),
    );
    fields.insert(
        "base_revision".to_string(),
        normalize_u64(batch.base_revision()),
    );
    fields.insert(
        "new_revision".to_string(),
        normalize_u64(batch.new_revision()),
    );
    fields.insert(
        "changes".to_string(),
        Value::Array(
            batch
                .changes()
                .iter()
                .map(|change| {
                    let position = change.position();
                    serde_json::json!({
                        "position": [
                            position.x(),
                            position.y(),
                            position.z(),
                        ],
                        "block": change.block(),
                    })
                })
                .collect(),
        ),
    );
    fields
}

/// Builds the rejected batch projection from the parsed input.
fn changes_projection(raw: &RawChanges) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("dimension".to_string(), Value::from(raw.dimension));
    fields.insert(
        "chunk".to_string(),
        Value::Array(vec![Value::from(raw.chunk[0]), Value::from(raw.chunk[1])]),
    );
    fields.insert(
        "base_revision".to_string(),
        normalize_u64(raw.base_revision),
    );
    fields.insert("new_revision".to_string(), normalize_u64(raw.new_revision));
    fields.insert(
        "changes".to_string(),
        Value::Array(
            raw.changes
                .iter()
                .map(|(position, block)| {
                    serde_json::json!({
                        "position": [
                            position[0],
                            position[1],
                            position[2],
                        ],
                        "block": block,
                    })
                })
                .collect(),
        ),
    );
    fields
}

/// Selects the first rule one forget batch breaks: the dimension, then the
/// nonempty and unique position rules. The uniqueness rule comes from the
/// owning constructor, which checks emptiness first and sorts a copy, so the
/// submitted order stays untouched in every projection.
fn forget_rejection(raw: &RawForget) -> Option<Rejection> {
    if let Some(rejection) = dimension_rejection(raw.dimension, "forget_chunks.dimension") {
        return Some(rejection);
    }
    if raw.chunks.is_empty() {
        return Some(Rejection {
            category: "invalid-value",
            rule: "forget_chunks.nonempty_positions",
            error: Some(DomainError::InvalidForgetChunks),
        });
    }
    match forget_value(raw, raw.dimension as u8) {
        Ok(_) => None,
        Err(DomainError::InvalidForgetChunks) => Some(Rejection {
            category: "invalid-value",
            rule: "forget_chunks.unique_positions",
            error: Some(DomainError::InvalidForgetChunks),
        }),
        // Another verdict cannot occur; the accepted path below fails closed
        // through its own construction if one ever did.
        Err(_) => None,
    }
}

/// Assembles one forget value from already-parsed parts.
fn forget_value(raw: &RawForget, dimension_id: u8) -> Result<ForgetChunks, DomainError> {
    let chunks: Vec<ChunkPos> = raw
        .chunks
        .iter()
        .map(|chunk| ChunkPos::new(chunk[0], chunk[1]))
        .collect();
    ForgetChunks::try_new(ForgetChunksParts {
        dimension: Dimension::new(dimension_id)?,
        chunks: chunks.into_boxed_slice(),
    })
}

/// Verifies the constructor agreement for one classified forget rejection.
fn verify_forget_rejection(
    case: &FrozenCase,
    raw: &RawForget,
    rejection: &Rejection,
) -> Result<(), DispatchError> {
    let Some(expected) = rejection.error else {
        return Ok(());
    };
    let dimension_id = u8::try_from(raw.dimension)
        .map_err(|_| classification_conflict(case, "forget-chunks", "dimension"))?;
    let verdict = match rejection.rule {
        "forget_chunks.dimension" => Dimension::new(dimension_id).map(|_| ()),
        "forget_chunks.nonempty_positions" | "forget_chunks.unique_positions" => {
            forget_value(raw, dimension_id).map(|_| ())
        }
        _ => {
            return Err(classification_conflict(
                case,
                "forget-chunks",
                rejection.rule,
            ));
        }
    };
    match verdict {
        Err(error) if error == expected => Ok(()),
        _ => Err(classification_conflict(
            case,
            "forget-chunks",
            rejection.rule,
        )),
    }
}

/// Constructs the full checked chain for one admitted forget batch.
fn construct_forget(case: &FrozenCase, raw: &RawForget) -> Result<ForgetChunks, DispatchError> {
    let dimension_id = u8::try_from(raw.dimension)
        .map_err(|_| classification_conflict(case, "forget-chunks", "dimension"))?;
    forget_value(raw, dimension_id).map_err(|_| classification_conflict(case, "forget-chunks", ""))
}

/// Builds the accepted forget projection from the constructed record's
/// getters, preserving submitted order exactly.
fn forget_fields(forget: &ForgetChunks) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert(
        "dimension".to_string(),
        Value::from(forget.dimension().get()),
    );
    fields.insert(
        "chunks".to_string(),
        Value::Array(
            forget
                .chunks()
                .iter()
                .map(|chunk| Value::Array(vec![Value::from(chunk.x()), Value::from(chunk.z())]))
                .collect(),
        ),
    );
    fields
}

/// Builds the rejected forget projection from the parsed input.
fn forget_projection(raw: &RawForget) -> JsonMap {
    let mut fields = JsonMap::new();
    fields.insert("dimension".to_string(), Value::from(raw.dimension));
    fields.insert(
        "chunks".to_string(),
        Value::Array(
            raw.chunks
                .iter()
                .map(|chunk| Value::Array(vec![Value::from(chunk[0]), Value::from(chunk[1])]))
                .collect(),
        ),
    );
    fields
}

#[test]
fn event_world_execute_38_cases() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    for case in &executed {
        assert_domain_normalized(case);
    }
}

/// Proves the shared comparator actually compares semantic fields: the
/// produced accepted result matches its frozen expectation, while the same
/// actual value no longer matches once one normalized field is mutated.
#[test]
fn event_world_comparator_detects_semantic_mutation() {
    let executed = execute_topic(EXPECTED_COUNT, owns, execute).expect("execute topic");
    let accepted = executed
        .into_iter()
        .find(|case| {
            case.actual.get("kind").and_then(|kind| kind.as_str()) == Some("ok")
                && case
                    .actual
                    .get("category")
                    .and_then(|category| category.as_str())
                    == Some("chunk-snapshot")
        })
        .expect("one accepted chunk-snapshot case");

    assert_domain_normalized(&accepted);

    let mut mutated_case = accepted.case;
    let fields = mutated_case
        .normalized
        .get_mut("fields")
        .and_then(|fields| fields.as_object_mut())
        .expect("normalized fields object");
    fields.insert("revision".to_string(), Value::String("999".to_string()));

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
