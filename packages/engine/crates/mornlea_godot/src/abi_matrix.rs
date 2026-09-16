//! Error-precedence matrix and status-dispatch contract for the client-core
//! ABI consumer side.
//!
//! Scope ruling, recorded honestly: the FFI bindings that call the Go
//! c-shared client core do not exist yet; they land with the later bridge
//! module. Until then this module tests what exists today — the constants
//! mirror in [`crate::abi`] and the negotiation mirror in
//! [`crate::feature_negotiation`] — plus the small pure-Rust status-dispatch
//! table defined here. The dispatch table is the contract baseline those
//! bindings must implement: every possible 32-bit status word classifies
//! totally into exactly one bucket (one exhaustive match, so the compiler
//! rejects any future status word without a bucket, and classification can
//! never panic), and each bucket pins the consumer-side obligation for that
//! outcome class — notably that no error status leaves partial Godot-side
//! state behind and that no status word may ever unwrap or panic the
//! binding.
//!
//! Two producer behaviors are pinned as recorded matrix facts rather than
//! defects to paper over: `STATUS_INTERNAL` currently conflates queue-full
//! backpressure from the step family (a compatible new status code is the
//! designated future split), and a receiver-driven self-close racing an
//! in-flight call can transiently surface as `STATUS_INTERNAL` (step) or
//! `STATUS_INPUT_REJECTED` (submission) instead of
//! `STATUS_DISCONNECTED` until the session observes the terminal phase. Both
//! directions are safe: the consumer treats the call as failed with no
//! output, exactly as the bucket obligations below demand.

// Like the `abi` and `feature_negotiation` mirrors, this module is ahead of
// its non-test consumers: the FFI dispatch that consults these buckets lands
// with the bridge module. Allow `dead_code` module-wide so the dispatch table
// does not fail `cargo clippy --all-targets -- -D warnings` before that
// consumer exists; remove this allowance once production code consumes the
// module directly.
#![allow(dead_code)]

use crate::abi::{
    STATUS_ABI_MISMATCH, STATUS_COUNT, STATUS_DISCONNECTED, STATUS_INPUT_REJECTED,
    STATUS_INSUFFICIENT_CAPACITY, STATUS_INTERNAL, STATUS_INVALID_ARGUMENT, STATUS_INVALID_HANDLE,
    STATUS_INVALID_STATE, STATUS_OK, STATUS_PANIC,
};

/// The bucket of consumer obligations one status word falls into. The bucket,
/// not the raw word, decides what the Godot-side binding does next, so new
/// producer codes slot into an obligation instead of silently falling through
/// a `match` arm.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum StatusBucket {
    /// `STATUS_OK`: the call's outputs are valid and complete.
    Ok,
    /// A caller mistake (`STATUS_INVALID_ARGUMENT`, `STATUS_ABI_MISMATCH`,
    /// `STATUS_INPUT_REJECTED`, `STATUS_INVALID_HANDLE`,
    /// `STATUS_INVALID_STATE`): fix the call and retry; the producer wrote no
    /// output and changed no session state.
    CallerError,
    /// `STATUS_INSUFFICIENT_CAPACITY`: the two-phase size signal; retry once
    /// with the reported required size, because nothing was written.
    CapacitySignal,
    /// `STATUS_DISCONNECTED`: the session reached its terminal disconnect;
    /// destroy the handle and create a new session, never retry in place.
    Terminal,
    /// `STATUS_INTERNAL` and `STATUS_PANIC`: a producer-side failure with no
    /// output. Includes the recorded queue-full conflation and the transient
    /// terminal-race misclassification.
    Internal,
}

/// Classify one status word totally: every defined code lands in its pinned
/// bucket and every out-of-range word fails closed as [`StatusBucket::Internal`],
/// because an undefined word is producer drift the consumer must treat as a
/// producer-side failure (tear the session down, retain no partial state)
/// rather than a retryable or caller-side condition.
pub fn classify_status(word: u32) -> StatusBucket {
    match word {
        STATUS_OK => StatusBucket::Ok,
        STATUS_INVALID_ARGUMENT
        | STATUS_ABI_MISMATCH
        | STATUS_INPUT_REJECTED
        | STATUS_INVALID_HANDLE
        | STATUS_INVALID_STATE => StatusBucket::CallerError,
        STATUS_INSUFFICIENT_CAPACITY => StatusBucket::CapacitySignal,
        STATUS_DISCONNECTED => StatusBucket::Terminal,
        STATUS_INTERNAL | STATUS_PANIC => StatusBucket::Internal,
        STATUS_COUNT..=u32::MAX => StatusBucket::Internal,
    }
}

/// The binding obligation one bucket carries, as reviewable text. The strings
/// are the contract baseline for the later FFI module: no panic on any
/// status, no partial Godot state retained on any error bucket, and the
/// recorded conflations stay visible at the dispatch site.
pub fn bucket_obligation(bucket: StatusBucket) -> &'static str {
    match bucket {
        StatusBucket::Ok => "consume the outputs: the producer wrote exactly one complete record",
        StatusBucket::CallerError => {
            "fix the call and retry: no output was written and no session state changed"
        }
        StatusBucket::CapacitySignal => {
            "retry once with the reported required size: nothing was written"
        }
        StatusBucket::Terminal => {
            "destroy the handle and create a new session: no output was written"
        }
        StatusBucket::Internal => {
            "tear the session down without retaining partial state: no output was written; \
            queue-full backpressure is conflated here until the compatible status split lands"
        }
    }
}

#[cfg(test)]
mod tests {
    use super::{StatusBucket, bucket_obligation, classify_status};
    use crate::abi::{
        ABI_MAJOR, FAMILY_COUNT, FAMILY_STATUS, STATUS_ABI_MISMATCH, STATUS_COUNT,
        STATUS_DISCONNECTED, STATUS_INPUT_REJECTED, STATUS_INSUFFICIENT_CAPACITY, STATUS_INTERNAL,
        STATUS_INVALID_ARGUMENT, STATUS_INVALID_HANDLE, STATUS_INVALID_STATE, STATUS_OK,
        STATUS_PANIC,
    };
    use crate::feature_negotiation::{NegotiationReason, PILOT_FAMILIES, negotiate};

    /// Every defined status word lands in its pinned bucket. The list is
    /// hand-maintained on purpose: a status code appended to the header
    /// without a conscious bucket decision here fails this matrix until the
    /// dispatch table is extended.
    #[test]
    fn abi_matrix_classifies_every_defined_status_word() {
        let pinned = [
            (STATUS_OK, StatusBucket::Ok),
            (STATUS_INVALID_ARGUMENT, StatusBucket::CallerError),
            (STATUS_ABI_MISMATCH, StatusBucket::CallerError),
            (STATUS_INPUT_REJECTED, StatusBucket::CallerError),
            (STATUS_INSUFFICIENT_CAPACITY, StatusBucket::CapacitySignal),
            (STATUS_INVALID_HANDLE, StatusBucket::CallerError),
            (STATUS_INVALID_STATE, StatusBucket::CallerError),
            (STATUS_DISCONNECTED, StatusBucket::Terminal),
            (STATUS_INTERNAL, StatusBucket::Internal),
            (STATUS_PANIC, StatusBucket::Internal),
        ];
        for (word, want) in pinned {
            assert_eq!(classify_status(word), want, "status word {word}");
        }
    }

    /// Out-of-range words classify without panicking and fail closed as the
    /// internal bucket. Totality is structural — `classify_status` is one
    /// exhaustive `match` over `u32`, so the compiler rejects a future status
    /// word without a bucket — and the sweep pins the boundaries plus every
    /// word up to twice the frozen count so an accidentally off-by-one range
    /// arm cannot hide.
    #[test]
    fn abi_matrix_classifies_out_of_range_words_without_panic() {
        let boundaries = [
            STATUS_COUNT,
            STATUS_COUNT + 1,
            2 * STATUS_COUNT,
            0x8000_0000,
            0xFFFF_0000,
            u32::MAX - 1,
            u32::MAX,
        ];
        for word in boundaries {
            assert_eq!(
                classify_status(word),
                StatusBucket::Internal,
                "out-of-range word {word} must fail closed"
            );
        }
        // Every word up to twice the frozen count classifies without
        // panicking, and each one beyond the frozen count fails closed.
        for word in 0..=2 * STATUS_COUNT {
            let bucket = classify_status(word);
            if word >= STATUS_COUNT {
                assert_eq!(bucket, StatusBucket::Internal, "word {word}");
            }
        }
    }

    /// The obligation strings are the pinned consumer contract the later FFI
    /// bindings implement: no panic on any status, no partial state retained
    /// on any error bucket, and the recorded conflations documented at the
    /// dispatch site.
    #[test]
    fn abi_matrix_bucket_obligations_pin_the_binding_contract() {
        let all = [
            StatusBucket::Ok,
            StatusBucket::CallerError,
            StatusBucket::CapacitySignal,
            StatusBucket::Terminal,
            StatusBucket::Internal,
        ];
        for bucket in all {
            assert!(!bucket_obligation(bucket).is_empty(), "bucket {bucket:?}");
        }
        // No error bucket may permit retaining partial state, so every
        // non-OK obligation must state that nothing was written or demand a
        // teardown.
        assert!(bucket_obligation(StatusBucket::CallerError).contains("no output was written"));
        assert!(bucket_obligation(StatusBucket::CapacitySignal).contains("nothing was written"));
        assert!(bucket_obligation(StatusBucket::Terminal).contains("no output was written"));
        assert!(bucket_obligation(StatusBucket::Internal).contains("no output was written"));
        assert!(
            bucket_obligation(StatusBucket::Internal).contains("without retaining partial state")
        );
        // The recorded queue-full conflation stays visible at the dispatch
        // site until the compatible status split lands.
        assert!(bucket_obligation(StatusBucket::Internal).contains("queue-full"));
    }

    /// The expected decision for one negotiation cell. This helper
    /// intentionally mirrors the documented precedence (unknown family, then
    /// major, then minor) rather than calling `negotiate`, so reordering the
    /// checks inside `negotiate` fails the matrix instead of vacuously
    /// passing it.
    fn expected_decision(
        registered: Option<u32>,
        major: u32,
        minor: u32,
    ) -> (bool, NegotiationReason) {
        let Some(version) = registered else {
            return (false, NegotiationReason::UnknownFamily);
        };
        if major != ABI_MAJOR {
            return (false, NegotiationReason::MajorMismatch);
        }
        if minor > version {
            return (false, NegotiationReason::MinorTooNew);
        }
        (true, NegotiationReason::Compatible)
    }

    /// Every cell of the negotiation matrix — each known family and several
    /// unknown identifiers, crossed with major equal/high/low and minor
    /// below/equal/above the registered contract version — yields the
    /// documented decision, with no cell panicking and no cell accepted on a
    /// mismatch.
    #[test]
    fn abi_matrix_negotiation_covers_every_family_major_minor_cell() {
        let known = PILOT_FAMILIES;
        let unknown = [0, FAMILY_COUNT + 1, FAMILY_STATUS + 1, u32::MAX];
        // `ABI_MAJOR` is 1 today; saturating keeps the "low" cell
        // representable without underflow if the major ever moves.
        let majors = [ABI_MAJOR, ABI_MAJOR + 1, ABI_MAJOR.saturating_sub(1)];
        let mut cells = 0;

        for descriptor in known {
            let registered = descriptor.version;
            let minors = [registered.saturating_sub(1), registered, registered + 1];
            for major in majors {
                for minor in minors {
                    let decision = negotiate(&known, descriptor.family, major, minor);
                    let (accepted, reason) = expected_decision(Some(registered), major, minor);
                    assert_eq!(
                        decision.accepted, accepted,
                        "family {} major {major} minor {minor}",
                        descriptor.family
                    );
                    assert_eq!(
                        decision.reason, reason,
                        "family {} major {major} minor {minor}",
                        descriptor.family
                    );
                    cells += 1;
                }
            }
        }
        for family in unknown {
            for major in majors {
                for minor in [0u32, 1, 2] {
                    let decision = negotiate(&known, family, major, minor);
                    let (accepted, reason) = expected_decision(None, major, minor);
                    assert_eq!(decision.accepted, accepted, "unknown family {family}");
                    // An unknown family wins over every version dimension.
                    assert_eq!(decision.reason, reason, "unknown family {family}");
                    cells += 1;
                }
            }
        }
        assert_eq!(cells, known.len() * 9 + unknown.len() * 9);
    }
}
