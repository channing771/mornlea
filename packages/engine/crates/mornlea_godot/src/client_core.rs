//! Handwritten Rust FFI declarations for the Go c-shared client core.
//!
//! The producer package `packages/client/cmd/mornlea-godot-core` exports the
//! symbols declared in its `exports.go` from a shared library built by
//! `scripts/godot/build-core.sh`; this module is the Rust-side mirror of that
//! export surface. Constants, record layouts, sizes, and alignments stay in
//! [`crate::abi`] (pinned against `include/mornlea_client_core.h`); this
//! module owns the extern declarations themselves, the typed handle and
//! status wrappers the later bridge code passes across the boundary, and the
//! diagnostic status-text table.
//!
//! Scope boundary: the tests here validate the declarations only. No test
//! loads or calls the produced shared library, because cargo tests must not
//! depend on the dylib artifact's build ordering; symbol existence in the
//! real library is proven by the build script's verify step and live-call
//! behavior belongs to the bridge module that consumes this surface.
//!
//! Unsafe confinement: the `unsafe extern "C"` block below is the only unsafe
//! code this module needs, and confinement for the whole crate is enforced by
//! a source-scan test rather than a crate-level `#![forbid(unsafe_code)]`,
//! because the Godot `ExtensionLibrary` implementation in the crate root
//! legitimately requires `unsafe impl`. The scan pins that exact exception
//! and fails on any other `unsafe` token outside this file.

// Like the `abi` and `feature_negotiation` mirrors, this module is ahead of
// its non-test consumers: the bridge module that calls these exports through
// the produced shared library lands with the later client-core integration
// work, and until then only the tests below reference the declarations and
// wrappers. Allow `dead_code` module-wide so the FFI surface does not fail
// `cargo clippy --all-targets -- -D warnings` before that consumer exists;
// remove this allowance once production code consumes the module directly.
#![allow(dead_code)]

use core::mem::{align_of, size_of};

use crate::abi;

/// One-line diagnostic text for a defined status word, or `None` for any
/// undefined word so producer drift fails closed instead of aliasing a
/// defined meaning. The texts are the header's documenting comments for the
/// `MORNLEA_CLIENT_STATUS_*` defines, pinned verbatim by the tests below.
pub fn status_text(word: u32) -> Option<&'static str> {
    match word {
        abi::STATUS_OK => Some("The call succeeded and every committed output byte is valid."),
        abi::STATUS_INVALID_ARGUMENT => {
            Some("Null, misaligned, overlapping, or oversized pointer and length arguments.")
        }
        abi::STATUS_ABI_MISMATCH => {
            Some("Caller ABI major, magic, or record identity does not match the producer.")
        }
        abi::STATUS_INPUT_REJECTED => Some(
            "Readable buffer whose content violates the family domain; the whole batch is \
            rejected and no producer state is consumed.",
        ),
        abi::STATUS_INSUFFICIENT_CAPACITY => Some(
            "Two-phase capacity signal: output buffer too small; the required byte count is \
            reported and nothing is written.",
        ),
        abi::STATUS_INVALID_HANDLE => Some("Unknown or wrong-type handle."),
        abi::STATUS_INVALID_STATE => Some("Correct handle in the wrong lifecycle phase or epoch."),
        abi::STATUS_DISCONNECTED => Some("The session already reached its terminal disconnect."),
        abi::STATUS_INTERNAL => {
            Some("Producer-internal failure without a narrower stable classification.")
        }
        abi::STATUS_PANIC => {
            Some("A recovered panic converted at the ABI boundary; no output is written.")
        }
        _ => None,
    }
}

/// The status word every client-core export returns. The wire type is the
/// header's frozen `uint32_t` status code; the newtype keeps status words
/// from mixing with other `u32` values at the later bridge call sites.
#[repr(transparent)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct Status(u32);

impl Status {
    /// Decode one raw status word. `None` for any word outside the defined
    /// contiguous range, mirroring the fail-closed classification the status
    /// matrix demands for undefined producer words.
    pub const fn from_word(word: u32) -> Option<Status> {
        if word < abi::STATUS_COUNT {
            Some(Status(word))
        } else {
            None
        }
    }

    /// The raw wire word of this status.
    pub const fn as_word(self) -> u32 {
        self.0
    }

    /// Whether this word is `STATUS_OK`, the only status that ever commits
    /// output bytes.
    pub const fn is_ok(self) -> bool {
        self.0 == abi::STATUS_OK
    }
}

/// An opaque session handle issued by `mornlea_client_core_create` and
/// consumed by every other export. The producer packs a table slot and a
/// generation into the 64-bit value; the zero value is never issued because
/// generations start at one, and a destroyed handle value is never reissued.
#[repr(transparent)]
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub struct ClientHandle(u64);

impl ClientHandle {
    /// Wrap one raw handle word exactly as the producer issued it.
    pub const fn from_word(word: u64) -> ClientHandle {
        ClientHandle(word)
    }

    /// The raw wire word of this handle.
    pub const fn as_word(self) -> u64 {
        self.0
    }
}

// The newtype widths are contract facts, not implementation details: the
// producer's C surface passes the status word as uint32_t and the handle as
// uint64_t, so any width or alignment change here is an ABI break.
const _: () = assert!(size_of::<Status>() == 4 && align_of::<Status>() == 4);
const _: () = assert!(size_of::<ClientHandle>() == 8 && align_of::<ClientHandle>() == 8);

/// The symbol names of every producer export, in `exports.go` declaration
/// order. The parity tests below assert this list equals both the `//export`
/// directives of `exports.go` and the declarations of the extern block in
/// this file, in both directions, so no side can gain or reorder an export
/// silently.
pub const EXPORT_SYMBOLS: [&str; 12] = [
    "mornlea_client_core_create",
    "mornlea_client_core_destroy",
    "mornlea_client_core_connect_begin",
    "mornlea_client_core_connect_poll",
    "mornlea_client_core_disconnect",
    "mornlea_client_core_submit_input",
    "mornlea_client_core_step",
    "mornlea_client_core_world_pull",
    "mornlea_client_core_frame_pull",
    "mornlea_client_core_status_pull",
    "mornlea_client_core_status_identity",
    "mornlea_client_core_abi_version",
];

// All unsafe code of the client-core consumer lives in this block: calling
// any declaration is unsafe because the producer trusts the caller for the
// pointer disciplines documented per export below (the bridge module owns
// upholding them). No test references these items as values, so the test
// binary never needs the produced shared library at link time.
unsafe extern "C" {
    /// Mirrors the `mornlea_client_core_create` wrapper over `coreCreate`:
    /// `requested_families` addresses `family_count` little-endian 64-bit
    /// words, each packing a feature-family identifier in the low 32 bits and
    /// the requested family contract version in the high 32 bits; a positive
    /// count needs a non-null, 8-byte-aligned array, and `out_handle` must be
    /// non-null. Only full success writes the out handle.
    fn mornlea_client_core_create(
        abi_major: u32,
        abi_minor: u32,
        requested_families: *const u64,
        family_count: u32,
        out_handle: *mut ClientHandle,
    ) -> Status;

    /// Mirrors `mornlea_client_core_destroy` over `coreDestroy`: releasing a
    /// handle this producer issued is idempotent, so destroying an already
    /// destroyed handle reports success again and any never-issued or stale
    /// value reports an invalid-handle status. The invalid-state answer a
    /// destroyed handle earns on every other export never applies to the
    /// tombstone's own idempotent client.
    fn mornlea_client_core_destroy(handle: ClientHandle) -> Status;

    /// Mirrors `mornlea_client_core_connect_begin` over `coreConnectBegin`:
    /// `address` is `address_len` UTF-8 "host:port" bytes, non-empty, at most
    /// the header's connection-address bound, non-null, and 8-byte aligned;
    /// the buffer is copied and never retained after the call returns.
    fn mornlea_client_core_connect_begin(
        handle: ClientHandle,
        address: *const u8,
        address_len: u32,
    ) -> Status;

    /// Mirrors `mornlea_client_core_connect_poll` over `coreConnectPoll`:
    /// `out_phase` must be non-null and receives the connection phase word on
    /// success; a terminally disconnected session writes no output byte.
    fn mornlea_client_core_connect_poll(handle: ClientHandle, out_phase: *mut u32) -> Status;

    /// Mirrors `mornlea_client_core_disconnect` over `coreDisconnect`:
    /// idempotent teardown of one live session's connection.
    fn mornlea_client_core_disconnect(handle: ClientHandle) -> Status;

    /// Mirrors `mornlea_client_core_submit_input` over `coreSubmitInput`:
    /// `buffer` is one complete input-family batch of exactly `length` bytes
    /// (at least the input header size, at most the batch bound), non-null
    /// and 8-byte aligned for a positive length; the batch is copied and
    /// never retained after the call returns.
    fn mornlea_client_core_submit_input(
        handle: ClientHandle,
        buffer: *const u8,
        length: u32,
    ) -> Status;

    /// Mirrors `mornlea_client_core_step` over `coreStep`: `request` is
    /// exactly one step-request record of the fixed wire size pinned by
    /// [`abi::STEP_REQUEST_BYTES`], non-null and 8-byte aligned; the record
    /// is copied and never retained after the call returns.
    fn mornlea_client_core_step(handle: ClientHandle, request: *const u8, length: u32) -> Status;

    /// Mirrors `mornlea_client_core_world_pull` over `coreWorldPull`: the
    /// two-phase capacity protocol; `required_out` must be non-null because
    /// every call may report a size, and a positive `capacity` needs a
    /// non-null, 8-byte-aligned `out` buffer whose declared span does not
    /// contain the size word. The pull drains one retained world batch.
    fn mornlea_client_core_world_pull(
        handle: ClientHandle,
        out: *mut u8,
        capacity: u32,
        required_out: *mut u32,
    ) -> Status;

    /// Mirrors `mornlea_client_core_frame_pull` over `coreFramePull` with
    /// the same pointer discipline as the world pull; the pull is
    /// non-consuming and serves the retained per-step frame snapshot.
    fn mornlea_client_core_frame_pull(
        handle: ClientHandle,
        out: *mut u8,
        capacity: u32,
        required_out: *mut u32,
    ) -> Status;

    /// Mirrors `mornlea_client_core_status_pull` over `coreStatusPull` with
    /// the same pointer discipline as the world pull; the pull is
    /// non-consuming and serves bounded status and metrics records.
    fn mornlea_client_core_status_pull(
        handle: ClientHandle,
        out: *mut u8,
        capacity: u32,
        required_out: *mut u32,
    ) -> Status;

    /// Mirrors `mornlea_client_core_status_identity` over
    /// `coreStatusIdentity` with the same pointer discipline as the world
    /// pull; the pull is non-consuming and serves the producer identity
    /// record (identity header plus family descriptors).
    fn mornlea_client_core_status_identity(
        handle: ClientHandle,
        out: *mut u8,
        capacity: u32,
        required_out: *mut u32,
    ) -> Status;

    /// Mirrors `mornlea_client_core_abi_version` over `coreAbiVersion`: the
    /// packed producer identity with the ABI major in the high 32 bits and
    /// the minor in the low 32 bits; it takes no pointer arguments and
    /// cannot fail.
    fn mornlea_client_core_abi_version() -> u64;
}

#[cfg(test)]
mod tests {
    use super::{ClientHandle, EXPORT_SYMBOLS, Status, status_text};
    use crate::abi::{
        self, IDENTITY_HEADER_BYTES, STATUS_ABI_MISMATCH, STATUS_COUNT, STATUS_DISCONNECTED,
        STATUS_INPUT_REJECTED, STATUS_INSUFFICIENT_CAPACITY, STATUS_INTERNAL,
        STATUS_INVALID_ARGUMENT, STATUS_INVALID_HANDLE, STATUS_INVALID_STATE, STATUS_OK,
        STATUS_PANIC, STEP_REQUEST_BYTES,
    };
    use core::mem::{align_of, size_of};
    use std::collections::BTreeMap;
    use std::fs;
    use std::path::{Path, PathBuf};

    const HEADER_TEXT: &str =
        include_str!("../../../../client/cmd/mornlea-godot-core/include/mornlea_client_core.h");
    const EXPORTS_GO: &str = include_str!("../../../../client/cmd/mornlea-godot-core/exports.go");
    const MODULE_SOURCE: &str = include_str!("client_core.rs");

    /// Collapse every whitespace run to one space so tests can compare
    /// source text and header comments without depending on wrapping.
    fn normalize_whitespace(text: &str) -> String {
        text.split_whitespace().collect::<Vec<_>>().join(" ")
    }

    /// Parse the `//export` symbol names from the producer's `exports.go` in
    /// declaration order. The wrappers are the shared library's sole entry
    /// points, so this list is the producer-side truth the extern block must
    /// mirror.
    fn exported_symbols() -> Vec<String> {
        let mut symbols = Vec::new();
        for raw in EXPORTS_GO.lines() {
            let Some(name) = raw.trim().strip_prefix("//export ") else {
                continue;
            };
            let name = name.trim();
            assert!(
                name.chars()
                    .all(|character| character.is_ascii_alphanumeric() || character == '_'),
                "malformed export name {name}"
            );
            assert!(!symbols.contains(&name.to_string()), "export {name} twice");
            symbols.push(name.to_string());
        }
        assert!(!symbols.is_empty(), "exports were not parsed");
        symbols
    }

    /// Extract the declarations of the module's extern block from its own
    /// source text, whitespace-normalized, in declaration order. Parsing the
    /// module itself ties the pinned name list to the actual declarations:
    /// a renamed, reordered, or re-typed extern item fails the pin instead of
    /// passing vacuously against the constant alone.
    fn extern_declarations() -> Vec<String> {
        let lines: Vec<&str> = MODULE_SOURCE.lines().collect();
        let openers: Vec<usize> = lines
            .iter()
            .enumerate()
            .filter(|(_, line)| line.trim() == "unsafe extern \"C\" {")
            .map(|(index, _)| index)
            .collect();
        assert_eq!(
            openers.len(),
            1,
            "the module must hold exactly one extern block"
        );
        let closer = lines[openers[0] + 1..]
            .iter()
            .position(|line| line.trim() == "}")
            .expect("extern block closer");
        let body: Vec<&str> = lines[openers[0] + 1..openers[0] + 1 + closer]
            .iter()
            .copied()
            .filter(|line| !line.trim().starts_with("//"))
            .collect();
        let joined = body.join("\n");
        let mut declarations = Vec::new();
        for segment in joined.split(';') {
            let normalized = normalize_whitespace(segment);
            if normalized.is_empty() {
                continue;
            }
            // Whitespace collapsing leaves spaces adjacent to the wrapping
            // punctuation rustfmt introduces (after the opening parenthesis,
            // before the closing one, and after a trailing argument comma);
            // drop those so the pin is formatting-independent.
            declarations.push(
                normalized
                    .replace(", )", ")")
                    .replace("( ", "(")
                    .replace(" )", ")"),
            );
        }
        declarations
    }

    /// The name of one parsed extern declaration.
    fn declaration_name(declaration: &str) -> &str {
        let without_prefix = declaration
            .strip_prefix("fn ")
            .expect("declaration starts with fn");
        &without_prefix[..without_prefix.find('(').expect("declaration argument list")]
    }

    /// Extract and normalize the block comment immediately preceding line
    /// `define_index` of the header. The status defines each carry their
    /// one-line meaning in an adjacent comment; the adjacency itself is
    /// part of the contract, so a define without its comment fails here.
    fn header_comment_text(lines: &[&str], define_index: usize) -> String {
        let previous = lines[define_index - 1].trim();
        assert!(
            previous.ends_with("*/"),
            "status define at line {} lacks an adjacent documenting comment",
            define_index + 1
        );
        let mut start = define_index - 1;
        while !lines[start].trim().starts_with("/*") {
            assert!(start > 0, "comment block has no opener");
            start -= 1;
        }
        let block = lines[start..define_index]
            .iter()
            .map(|line| line.trim())
            .collect::<Vec<_>>()
            .join("\n");
        let inner = block
            .strip_prefix("/*")
            .expect("comment opener")
            .strip_suffix("*/")
            .expect("comment closer");
        let text = inner
            .lines()
            .map(|line| line.trim().trim_start_matches('*').trim())
            .collect::<Vec<_>>()
            .join(" ");
        normalize_whitespace(&text)
    }

    /// Header defines that share the `MORNLEA_CLIENT_STATUS_` prefix but are
    /// family vocabulary rather than status words: the status-family contract
    /// version and the status-family header wire size. A new such define must
    /// be listed here consciously, or the status parity check fails.
    const NON_STATUS_PREFIX_DEFINES: [&str; 2] = [
        "MORNLEA_CLIENT_STATUS_VERSION",
        "MORNLEA_CLIENT_STATUS_HEADER_BYTES",
    ];

    /// Parse every status-word define of the header into its value and
    /// one-line comment text. Only the pinned status names, the count
    /// sentinel, and the known non-status prefix defines are accepted, so a
    /// newly added status word fails until this module learns its text; the
    /// sentinel is a count, never a word a producer returns, so it carries no
    /// diagnostic text.
    fn header_status_defines() -> BTreeMap<String, (u32, String)> {
        let lines: Vec<&str> = HEADER_TEXT.lines().collect();
        let mut statuses = BTreeMap::new();
        for (index, raw) in lines.iter().enumerate() {
            let trimmed = raw.trim();
            let Some(rest) = trimmed.strip_prefix("#define MORNLEA_CLIENT_STATUS_") else {
                continue;
            };
            let mut fields = rest.split_whitespace();
            let suffix = fields.next().expect("status define name");
            let name = format!("MORNLEA_CLIENT_STATUS_{suffix}");
            if suffix == "COUNT" || NON_STATUS_PREFIX_DEFINES.contains(&name.as_str()) {
                continue;
            }
            let value = fields.next().expect("status define value");
            let value = value.strip_suffix('u').unwrap_or(value);
            assert!(
                value.len() == 1 || !value.starts_with('0'),
                "ambiguous literal {value}: decimal defines must not carry a leading zero"
            );
            let value: u32 = value.parse().expect("decimal status define value");
            let text = header_comment_text(&lines, index);
            assert!(
                statuses.insert(name.clone(), (value, text)).is_none(),
                "status {name} defined twice"
            );
        }
        assert_eq!(
            statuses.len(),
            STATUS_COUNT as usize,
            "the header must define exactly the pinned status words plus the count sentinel"
        );
        statuses
    }

    /// Blank out line comments, block comments, and string-literal bodies so
    /// a line-by-line token scan sees only code. The scanner fails closed on
    /// constructs it cannot model (every raw-string opener, including the
    /// zero-hash and prefixed forms, plus char literals whose body would
    /// open a string state) so new syntax forces a conscious scanner
    /// extension instead of a silent miss.
    fn strip_comments_and_string_bodies(source: &str) -> String {
        let characters: Vec<char> = source.chars().collect();
        let mut output = String::with_capacity(source.len());
        let mut index = 0;
        let mut in_string = false;
        let mut in_line_comment = false;
        let mut in_block_comment = false;
        while index < characters.len() {
            let character = characters[index];
            index += 1;
            if in_line_comment {
                if character == '\n' {
                    in_line_comment = false;
                    output.push('\n');
                } else {
                    output.push(' ');
                }
            } else if in_block_comment {
                if character == '*' && characters.get(index) == Some(&'/') {
                    index += 1;
                    in_block_comment = false;
                    output.push_str("  ");
                } else if character == '\n' {
                    output.push('\n');
                } else {
                    output.push(' ');
                }
            } else if in_string {
                output.push(' ');
                if character == '\\' {
                    // Keep an escape and its escaped character out of the
                    // scan; neither can close the literal.
                    if index < characters.len() {
                        index += 1;
                        output.push(' ');
                    }
                } else if character == '"' {
                    in_string = false;
                }
            } else if character == 'r' {
                // Every raw-string opener (`r"`, `r#"`, `r##"`, and the
                // `br"`/`cr"` prefixed forms, whose prefix the loop already
                // consumed as code) defeats this scanner: a raw body may
                // contain backslashes the escape rule would swallow and the
                // hashes defeat the closer match, either way hiding live
                // tokens. Fail closed on each opener instead.
                let mut probe = index;
                while characters.get(probe) == Some(&'#') {
                    probe += 1;
                }
                assert!(
                    characters.get(probe) != Some(&'"'),
                    "raw string literals need scanner support"
                );
                output.push(character);
            } else {
                match character {
                    '/' => {
                        if characters.get(index) == Some(&'/') {
                            index += 1;
                            in_line_comment = true;
                            output.push_str("  ");
                        } else if characters.get(index) == Some(&'*') {
                            index += 1;
                            in_block_comment = true;
                            output.push_str("  ");
                        } else {
                            output.push(character);
                        }
                    }
                    '"' => {
                        in_string = true;
                        output.push(' ');
                    }
                    '\'' => {
                        assert!(
                            characters.get(index) != Some(&'"')
                                && characters.get(index) != Some(&'\\'),
                            "char literal with scanner-sensitive body needs scanner support"
                        );
                        output.push(character);
                    }
                    _ => output.push(character),
                }
            }
        }
        output
    }

    /// (header define name, status word, one-line text) for every defined
    /// status, pinned so the text table cannot drift from the header.
    const STATUS_TEXT_PINS: [(&str, u32, &str); 10] = [
        (
            "MORNLEA_CLIENT_STATUS_OK",
            STATUS_OK,
            "The call succeeded and every committed output byte is valid.",
        ),
        (
            "MORNLEA_CLIENT_STATUS_INVALID_ARGUMENT",
            STATUS_INVALID_ARGUMENT,
            "Null, misaligned, overlapping, or oversized pointer and length arguments.",
        ),
        (
            "MORNLEA_CLIENT_STATUS_ABI_MISMATCH",
            STATUS_ABI_MISMATCH,
            "Caller ABI major, magic, or record identity does not match the producer.",
        ),
        (
            "MORNLEA_CLIENT_STATUS_INPUT_REJECTED",
            STATUS_INPUT_REJECTED,
            "Readable buffer whose content violates the family domain; the whole batch is \
            rejected and no producer state is consumed.",
        ),
        (
            "MORNLEA_CLIENT_STATUS_INSUFFICIENT_CAPACITY",
            STATUS_INSUFFICIENT_CAPACITY,
            "Two-phase capacity signal: output buffer too small; the required byte count is \
            reported and nothing is written.",
        ),
        (
            "MORNLEA_CLIENT_STATUS_INVALID_HANDLE",
            STATUS_INVALID_HANDLE,
            "Unknown or wrong-type handle.",
        ),
        (
            "MORNLEA_CLIENT_STATUS_INVALID_STATE",
            STATUS_INVALID_STATE,
            "Correct handle in the wrong lifecycle phase or epoch.",
        ),
        (
            "MORNLEA_CLIENT_STATUS_DISCONNECTED",
            STATUS_DISCONNECTED,
            "The session already reached its terminal disconnect.",
        ),
        (
            "MORNLEA_CLIENT_STATUS_INTERNAL",
            STATUS_INTERNAL,
            "Producer-internal failure without a narrower stable classification.",
        ),
        (
            "MORNLEA_CLIENT_STATUS_PANIC",
            STATUS_PANIC,
            "A recovered panic converted at the ABI boundary; no output is written.",
        ),
    ];

    /// The exact extern declaration of every export, normalized; each entry
    /// mirrors the `//export` wrapper of the same name in `exports.go` (the
    /// producer file holds the signature truth; its line numbers appear in
    /// the module's per-export documentation). Any argument type, argument
    /// order, or return-type drift fails this pin.
    const PINNED_DECLARATIONS: [&str; 12] = [
        "fn mornlea_client_core_create(abi_major: u32, abi_minor: u32, requested_families: \
         *const u64, family_count: u32, out_handle: *mut ClientHandle) -> Status",
        "fn mornlea_client_core_destroy(handle: ClientHandle) -> Status",
        "fn mornlea_client_core_connect_begin(handle: ClientHandle, address: *const u8, \
         address_len: u32) -> Status",
        "fn mornlea_client_core_connect_poll(handle: ClientHandle, out_phase: *mut u32) -> Status",
        "fn mornlea_client_core_disconnect(handle: ClientHandle) -> Status",
        "fn mornlea_client_core_submit_input(handle: ClientHandle, buffer: *const u8, length: \
         u32) -> Status",
        "fn mornlea_client_core_step(handle: ClientHandle, request: *const u8, length: u32) \
         -> Status",
        "fn mornlea_client_core_world_pull(handle: ClientHandle, out: *mut u8, capacity: u32, \
         required_out: *mut u32) -> Status",
        "fn mornlea_client_core_frame_pull(handle: ClientHandle, out: *mut u8, capacity: u32, \
         required_out: *mut u32) -> Status",
        "fn mornlea_client_core_status_pull(handle: ClientHandle, out: *mut u8, capacity: u32, \
         required_out: *mut u32) -> Status",
        "fn mornlea_client_core_status_identity(handle: ClientHandle, out: *mut u8, capacity: \
         u32, required_out: *mut u32) -> Status",
        "fn mornlea_client_core_abi_version() -> u64",
    ];

    #[test]
    fn client_core_export_symbols_match_the_go_producer() {
        let exported = exported_symbols();
        // Both directions and in order: a renamed, added, dropped, or
        // reordered producer export fails until this mirror is consciously
        // re-pinned with the same review as the header change.
        assert_eq!(EXPORT_SYMBOLS.len(), 12);
        assert_eq!(exported, EXPORT_SYMBOLS.to_vec());
    }

    #[test]
    fn client_core_extern_block_pins_every_declaration() {
        let declarations = extern_declarations();
        assert_eq!(declarations.len(), PINNED_DECLARATIONS.len());
        for (index, (declaration, pinned)) in
            declarations.iter().zip(PINNED_DECLARATIONS).enumerate()
        {
            assert_eq!(
                declaration,
                &normalize_whitespace(pinned),
                "extern declaration {index}"
            );
        }
        // The declared names equal the pinned symbol list, which the symbol
        // parity test separately proves equal to the producer's exports.
        let names: Vec<&str> = declarations
            .iter()
            .map(|item| declaration_name(item))
            .collect();
        assert_eq!(names, EXPORT_SYMBOLS.to_vec());
    }

    #[test]
    fn client_core_status_text_pins_every_defined_status() {
        for (name, word, text) in STATUS_TEXT_PINS {
            assert_eq!(status_text(word), Some(text), "status {name}");
        }
        // Contiguity: every word below the count sentinel decodes, the
        // sentinel and every word beyond it fail closed with no text.
        for word in 0..STATUS_COUNT {
            assert!(
                status_text(word).is_some(),
                "defined status word {word} has no text"
            );
            assert!(Status::from_word(word).is_some(), "word {word}");
        }
        for word in [STATUS_COUNT, STATUS_COUNT + 1, u32::MAX] {
            assert_eq!(status_text(word), None, "undefined status word {word}");
            assert!(Status::from_word(word).is_none(), "word {word}");
        }
        let mut words: Vec<u32> = STATUS_TEXT_PINS.iter().map(|pin| pin.1).collect();
        words.sort();
        words.dedup();
        assert_eq!(words.len(), STATUS_TEXT_PINS.len());
    }

    #[test]
    fn client_core_status_text_matches_the_header_documentation() {
        let defines = header_status_defines();
        let pinned: BTreeMap<&str, (u32, &str)> = STATUS_TEXT_PINS
            .iter()
            .map(|(name, word, text)| (*name, (*word, *text)))
            .collect();
        // Both directions: a status the header gains without this table
        // fails, and a table entry the header lost fails, so the mapping
        // cannot drift on either side.
        for (name, (word, text)) in &defines {
            let Some((pinned_word, pinned_text)) = pinned.get(name.as_str()) else {
                panic!("header status {name} has no pinned text");
            };
            assert_eq!(word, pinned_word, "status {name} value");
            assert_eq!(
                status_text(*word),
                Some(normalize_whitespace(pinned_text).as_str()),
                "status {name} text"
            );
            assert_eq!(
                &normalize_whitespace(text),
                &normalize_whitespace(pinned_text)
            );
        }
        for name in pinned.keys() {
            assert!(
                defines.contains_key(*name),
                "pinned status {name} is not in the header"
            );
        }
    }

    #[test]
    fn client_core_type_widths_and_abi_layouts_stay_pinned() {
        assert_eq!((size_of::<Status>(), align_of::<Status>()), (4, 4));
        assert_eq!(
            (size_of::<ClientHandle>(), align_of::<ClientHandle>()),
            (8, 8)
        );
        // The byte-buffer exports operate on the wire layouts mirrored in
        // `abi`; reusing those structs (no new mirrors) keeps the extern
        // disciplines and the layout pins on one definition.
        assert_eq!(size_of::<abi::StepRequest>(), STEP_REQUEST_BYTES);
        assert_eq!(size_of::<abi::IdentityHeader>(), IDENTITY_HEADER_BYTES);
        assert_eq!(align_of::<abi::StepRequest>(), abi::ABI_ALIGNMENT);
        assert_eq!(align_of::<abi::IdentityHeader>() * 2, abi::ABI_ALIGNMENT);
    }

    #[test]
    fn client_core_status_and_handle_round_trip_their_words() {
        for (_, word, _) in STATUS_TEXT_PINS {
            let status = Status::from_word(word).expect("defined status word");
            assert_eq!(status.as_word(), word);
            assert_eq!(status.is_ok(), word == STATUS_OK);
        }
        assert!(Status::from_word(STATUS_COUNT).is_none());
        let handle = ClientHandle::from_word(u64::MAX);
        assert_eq!(handle.as_word(), u64::MAX);
        assert_eq!(ClientHandle::from_word(0).as_word(), 0);
    }

    /// Collect every `.rs` file under `directory`, recursing into
    /// subdirectories and failing closed on any read error so the unsafe
    /// confinement scan cannot silently skip a tree it could not open.
    fn rust_sources_under(directory: &Path, sink: &mut Vec<PathBuf>) {
        for entry in fs::read_dir(directory).expect("read the source directory") {
            let path = entry.expect("source directory entry").path();
            if path.is_dir() {
                rust_sources_under(&path, sink);
                continue;
            }
            if path.extension().is_some_and(|extension| extension == "rs") {
                sink.push(path);
            }
        }
    }

    /// All unsafe code of the crate lives in this module. The crate root's
    /// `unsafe impl ExtensionLibrary` is the single pre-existing exception
    /// (Godot's extension trait is unsafe by contract), pinned exactly; any
    /// other `unsafe` token anywhere under `src/` fails this scan, including
    /// files added later in nested subdirectories, because the whole tree is
    /// enumerated at test time. This module itself is the sanctioned home and
    /// is exempt from the token scan; doc comments and string bodies of the
    /// other files are stripped first so prose cannot hide or fake an
    /// occurrence.
    #[test]
    fn client_core_unsafe_is_confined_to_the_ffi_module() {
        let source_dir = Path::new(env!("CARGO_MANIFEST_DIR")).join("src");
        let mut sources = Vec::new();
        rust_sources_under(&source_dir, &mut sources);
        sources.sort();
        assert!(
            sources.iter().any(|path| path
                .file_name()
                .is_some_and(|name| name == "client_core.rs")),
            "the FFI module source is missing from the scan"
        );
        for path in &sources {
            let name = path
                .file_name()
                .expect("source file name")
                .to_str()
                .expect("utf-8 source file name");
            if name == "client_core.rs" {
                // The FFI module is the one sanctioned home of unsafe code.
                continue;
            }
            let source = fs::read_to_string(path).expect("read the source file");
            let stripped = strip_comments_and_string_bodies(&source);
            for (index, line) in stripped.lines().enumerate() {
                if !line.split_whitespace().any(|token| token == "unsafe") {
                    continue;
                }
                let allowed = name == "lib.rs"
                    && line.trim() == "unsafe impl ExtensionLibrary for MornleaGodotExtension {";
                assert!(
                    allowed,
                    "unsafe code outside the client_core FFI module: {name} line {}: {}",
                    index + 1,
                    line.trim()
                );
            }
        }
    }
}
