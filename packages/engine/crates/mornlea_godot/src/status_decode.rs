//! Typed decode of the status family's `MCM1` record set.
//!
//! Design decision 4 of the Godot pilot migration: Python consumes typed
//! Godot-visible semantic values only, so record decoding is Rust-owned.
//! This module decodes one pulled status record set — the frozen 16-byte
//! header followed by fixed 16-byte records (kind word, reserved word,
//! 64-bit value) — into the semantic fields the bridge exposes through
//! `session_status_typed`. Python never sees record bytes or geometry.
//! The module holds no unsafe code: it reads only the bridge's owned copy.
//!
//! Fail-closed contract: every structural violation — a wrong magic or
//! layout version, a record count disagreeing with the record size, a
//! count above the header's record limit, a missing or duplicated known
//! kind, a nonzero reserved word, or a phase/terminal-cause value beyond
//! its 32-bit domain — rejects the whole record set with the internal
//! status; no semantic field is ever fabricated as zero. Unknown record
//! kinds are skipped instead of rejected, because the producer may only
//! append kinds, and a typed surface that does not know a kind must not
//! fail because of one. The skip is a decode-layer guarantee, not a
//! versioning mechanism: a producer that appends a kind must raise the
//! status family's pull-buffer record limit in the same change (see
//! `pull_buffers`), or the capacity query refuses the larger record set
//! before this decoder ever runs.

use crate::abi;
use crate::pull_buffers::STATUS_RECORD_BYTES;

/// Status record kind words, pinned against the producer's frozen
/// `StatusRecord*` declarations by the source test below. Kinds are
/// append-only producer vocabulary; none is repurposed.
pub(crate) const STATUS_KIND_PHASE: u32 = 1;
pub(crate) const STATUS_KIND_TERMINAL_CAUSE: u32 = 2;
pub(crate) const STATUS_KIND_STEPS_COMPLETED: u32 = 3;
pub(crate) const STATUS_KIND_MESSAGES_PROCESSED: u32 = 4;

/// The typed semantic view of one status record set: the connection phase
/// word, the terminal-cause classification, and the two step counters.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub(crate) struct TypedStatus {
    pub(crate) phase: u32,
    pub(crate) terminal_cause: u32,
    pub(crate) steps_completed: u64,
    pub(crate) messages_processed: u64,
}

/// Decode one status record set into its typed semantic fields.
///
/// The header must carry the frozen `MCM1` magic and layout version, a
/// record count that both fits the header's record limit and exactly
/// describes the record's size, and a zero reserved word. Every known kind
/// must appear exactly once; a missing kind would otherwise fabricate a
/// zero field. All validation precedes every read, so no read can go out
/// of bounds.
pub(crate) fn decode_status_record(record: &[u8]) -> Result<TypedStatus, u32> {
    let fail = Err(abi::STATUS_INTERNAL);
    if record.len() < abi::STATUS_HEADER_BYTES + STATUS_RECORD_BYTES {
        return fail;
    }
    if !(record.len() - abi::STATUS_HEADER_BYTES).is_multiple_of(STATUS_RECORD_BYTES) {
        return fail;
    }
    if le_u32(record, 0) != abi::MAGIC_STATUS {
        return fail;
    }
    if le_u32(record, 4) != abi::STATUS_VERSION {
        return fail;
    }
    let count = le_u32(record, 8) as usize;
    if count == 0 || count > abi::MAX_STATUS_RECORDS as usize {
        return fail;
    }
    if abi::STATUS_HEADER_BYTES + count * STATUS_RECORD_BYTES != record.len() {
        return fail;
    }
    if le_u32(record, 12) != 0 {
        return fail;
    }
    let mut phase: Option<u32> = None;
    let mut terminal_cause: Option<u32> = None;
    let mut steps_completed: Option<u64> = None;
    let mut messages_processed: Option<u64> = None;
    for index in 0..count {
        let base = abi::STATUS_HEADER_BYTES + index * STATUS_RECORD_BYTES;
        if le_u32(record, base + 4) != 0 {
            return fail;
        }
        let kind = le_u32(record, base);
        let value = le_u64(record, base + 8);
        match kind {
            STATUS_KIND_PHASE => {
                if phase.is_some() {
                    return fail;
                }
                phase = Some(word32(value)?);
            }
            STATUS_KIND_TERMINAL_CAUSE => {
                if terminal_cause.is_some() {
                    return fail;
                }
                terminal_cause = Some(word32(value)?);
            }
            STATUS_KIND_STEPS_COMPLETED => {
                if steps_completed.is_some() {
                    return fail;
                }
                steps_completed = Some(value);
            }
            STATUS_KIND_MESSAGES_PROCESSED => {
                if messages_processed.is_some() {
                    return fail;
                }
                messages_processed = Some(value);
            }
            _ => {
                // Appended unknown kinds carry no field on this typed
                // surface; skipping keeps a compatible producer addition
                // from failing an older consumer.
            }
        }
    }
    Ok(TypedStatus {
        phase: phase.ok_or(abi::STATUS_INTERNAL)?,
        terminal_cause: terminal_cause.ok_or(abi::STATUS_INTERNAL)?,
        steps_completed: steps_completed.ok_or(abi::STATUS_INTERNAL)?,
        messages_processed: messages_processed.ok_or(abi::STATUS_INTERNAL)?,
    })
}

/// Narrow one 64-bit record value to the 32-bit phase/cause domain; a
/// nonzero high word is a producer contract violation.
fn word32(value: u64) -> Result<u32, u32> {
    u32::try_from(value).map_err(|_| abi::STATUS_INTERNAL)
}

/// Read one little-endian 32-bit word; callers validate the bounds first.
fn le_u32(record: &[u8], offset: usize) -> u32 {
    let chunk: [u8; 4] = record[offset..offset + 4].try_into().expect("four bytes");
    u32::from_le_bytes(chunk)
}

/// Read one little-endian 64-bit word; callers validate the bounds first.
fn le_u64(record: &[u8], offset: usize) -> u64 {
    let chunk: [u8; 8] = record[offset..offset + 8].try_into().expect("eight bytes");
    u64::from_le_bytes(chunk)
}

/// Build one well-formed pilot record set with the four frozen kinds. A
/// test-shared fixture like `pull_buffers::test_world_record`: the
/// bridge-level tests reuse it to script the status pull.
#[cfg(test)]
pub(crate) fn test_status_record(phase: u32, cause: u32, steps: u64, messages: u64) -> Vec<u8> {
    let mut bytes = vec![0u8; abi::STATUS_HEADER_BYTES + 4 * STATUS_RECORD_BYTES];
    bytes[0..4].copy_from_slice(&abi::MAGIC_STATUS.to_le_bytes());
    bytes[4..8].copy_from_slice(&abi::STATUS_VERSION.to_le_bytes());
    bytes[8..12].copy_from_slice(&4u32.to_le_bytes());
    put_record(&mut bytes, 0, STATUS_KIND_PHASE, u64::from(phase));
    put_record(&mut bytes, 1, STATUS_KIND_TERMINAL_CAUSE, u64::from(cause));
    put_record(&mut bytes, 2, STATUS_KIND_STEPS_COMPLETED, steps);
    put_record(&mut bytes, 3, STATUS_KIND_MESSAGES_PROCESSED, messages);
    bytes
}

/// Write one 16-byte record at `index` (test fixture helper).
#[cfg(test)]
fn put_record(bytes: &mut [u8], index: usize, kind: u32, value: u64) {
    let base = abi::STATUS_HEADER_BYTES + index * STATUS_RECORD_BYTES;
    bytes[base..base + 4].copy_from_slice(&kind.to_le_bytes());
    bytes[base + 8..base + 16].copy_from_slice(&value.to_le_bytes());
}

#[cfg(test)]
mod tests {
    use super::{
        STATUS_KIND_MESSAGES_PROCESSED, STATUS_KIND_PHASE, STATUS_KIND_STEPS_COMPLETED,
        STATUS_KIND_TERMINAL_CAUSE, TypedStatus, decode_status_record, put_record,
        test_status_record,
    };
    use crate::abi;

    const STATUS_GO: &str = include_str!("../../../../client/cmd/mornlea-godot-core/status.go");

    /// Parse one Go const declaration that carries an explicit type, like
    /// `StatusRecordPhase uint32 = 1`.
    fn go_typed_const(source: &str, name: &str) -> u32 {
        for raw in source.lines() {
            let Some((head, tail)) = raw.split_once('=') else {
                continue;
            };
            let mut parts = head.trim().trim_start_matches("const").split_whitespace();
            if parts.next() != Some(name) {
                continue;
            }
            let digits: String = tail
                .trim()
                .chars()
                .take_while(|character| character.is_ascii_digit())
                .collect();
            assert!(!digits.is_empty(), "Go const {name} carries no literal");
            return digits.parse().expect("decimal Go const value");
        }
        panic!("Go const {name} was not found");
    }

    #[test]
    fn status_decode_pins_the_producer_kind_vocabulary() {
        for (name, value) in [
            ("StatusRecordPhase", STATUS_KIND_PHASE),
            ("StatusRecordTerminalCause", STATUS_KIND_TERMINAL_CAUSE),
            ("StatusRecordStepsCompleted", STATUS_KIND_STEPS_COMPLETED),
            (
                "StatusRecordMessagesProcessed",
                STATUS_KIND_MESSAGES_PROCESSED,
            ),
        ] {
            assert_eq!(go_typed_const(STATUS_GO, name), value, "kind {name}");
        }
    }

    #[test]
    fn status_decode_decodes_the_pilot_record_set() {
        assert_eq!(
            decode_status_record(&test_status_record(5, 1, 7, 900)),
            Ok(TypedStatus {
                phase: 5,
                terminal_cause: 1,
                steps_completed: 7,
                messages_processed: 900,
            })
        );
        // Boundary values of every domain round-trip unchanged.
        assert_eq!(
            decode_status_record(&test_status_record(u32::MAX, 6, u64::MAX, u64::MAX)),
            Ok(TypedStatus {
                phase: u32::MAX,
                terminal_cause: 6,
                steps_completed: u64::MAX,
                messages_processed: u64::MAX,
            })
        );
    }

    #[test]
    fn status_decode_ignores_unknown_appended_kinds() {
        let mut bytes = test_status_record(3, 0, 1, 2);
        bytes.resize(abi::STATUS_HEADER_BYTES + 5 * super::STATUS_RECORD_BYTES, 0);
        bytes[8..12].copy_from_slice(&5u32.to_le_bytes());
        put_record(&mut bytes, 4, 9, u64::MAX);
        assert_eq!(
            decode_status_record(&bytes),
            Ok(TypedStatus {
                phase: 3,
                terminal_cause: 0,
                steps_completed: 1,
                messages_processed: 2,
            })
        );
    }

    #[test]
    fn status_decode_rejects_structural_drift_fail_closed() {
        let cases: Vec<(&str, Vec<u8>)> = vec![
            ("empty record", Vec::new()),
            (
                "header only",
                test_status_record(0, 0, 0, 0)[..abi::STATUS_HEADER_BYTES].to_vec(),
            ),
            (
                "size off the record grid",
                test_status_record(0, 0, 0, 0)[..76].to_vec(),
            ),
            ("count disagrees with the size", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                bytes[8..12].copy_from_slice(&3u32.to_le_bytes());
                bytes
            }),
            ("count above the contract limit", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                bytes[8..12].copy_from_slice(&u32::MAX.to_le_bytes());
                bytes
            }),
            ("wrong magic", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                bytes[0..4].copy_from_slice(&abi::MAGIC_FRAME.to_le_bytes());
                bytes
            }),
            ("wrong layout version", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                bytes[4..8].copy_from_slice(&2u32.to_le_bytes());
                bytes
            }),
            ("nonzero header reserved word", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                bytes[12..16].copy_from_slice(&1u32.to_le_bytes());
                bytes
            }),
            ("nonzero record reserved word", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                bytes[abi::STATUS_HEADER_BYTES + 4..abi::STATUS_HEADER_BYTES + 8]
                    .copy_from_slice(&1u32.to_le_bytes());
                bytes
            }),
            ("duplicated phase kind", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                put_record(&mut bytes, 1, STATUS_KIND_PHASE, 0);
                bytes
            }),
            ("missing terminal-cause kind", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                put_record(&mut bytes, 1, STATUS_KIND_STEPS_COMPLETED, 0);
                bytes
            }),
            ("missing steps kind", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                put_record(&mut bytes, 2, STATUS_KIND_TERMINAL_CAUSE, 0);
                bytes
            }),
            ("missing messages kind", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                put_record(&mut bytes, 3, STATUS_KIND_PHASE, 0);
                bytes
            }),
            ("phase value beyond the 32-bit domain", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                put_record(&mut bytes, 0, STATUS_KIND_PHASE, 1 << 32);
                bytes
            }),
            ("terminal cause beyond the 32-bit domain", {
                let mut bytes = test_status_record(0, 0, 0, 0);
                put_record(&mut bytes, 1, STATUS_KIND_TERMINAL_CAUSE, 1 << 32);
                bytes
            }),
        ];
        for (name, bytes) in cases {
            assert_eq!(
                decode_status_record(&bytes),
                Err(abi::STATUS_INTERNAL),
                "{name}"
            );
        }
    }
}
