//! The unsequenced chat command packet family: the common fallible surface
//! and the domain-routed text gate the Go validator publishes.
//!
//! `ChatCommand` is the one client payload whose text is variable-length: a
//! canonical uvarint length prefix followed by the UTF-8 bytes, checked
//! against the Go decoder's fixed wire ceiling before anything is parsed. The
//! record is a concrete packet with a public mutable field, so the group pins
//! the design's surface order (`validate` → checked `encoded_len` → capacity
//! check → private `publish_packet`): a short or invalid `encode_into` leaves
//! the caller's buffer untouched, an invalid value wins over a short
//! destination, and every proper truncation of a canonical payload rejects.
//! The wire literals are the Go encoder's output, so the round-trip tests pin
//! byte-level parity rather than self-agreement. The corpus evidence this
//! family publishes is executed by `tests/protocol_corpus.rs` once the
//! controller integrates the exported assets, so this suite stays
//! self-contained and needs no corpus files.
//!
//! The text rule is the domain `CommandText` rule, so the wire, the domain and
//! the Go `validateCommandText` share one admitted set. The text itself is
//! never interpreted here: a leading `@` is not addressing, a leading `/` is
//! not a warp, and no case in this group declares a session, a deadline, a
//! FIFO or a send. The payload is exactly the length prefix and the text
//! bytes the exact-literal assertions below pin, so a sequence field, a
//! companion identity or a routing decision appearing on the wire would break
//! the literal rather than the encoder.

use mornlea_protocol::{
    CHAT_COMMAND_MAX_WIRE_BYTES, CHAT_COMMAND_TEXT_MAX_BYTES, ChatCommand, ProtocolError,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The reviewed valid literal for `"@mira follow"`: the single-byte canonical
/// uvarint length prefix 12 followed by the twelve text bytes.
const MIRA_WIRE: [u8; 13] = [
    0x0c, 0x40, 0x6d, 0x69, 0x72, 0x61, 0x20, 0x66, 0x6f, 0x6c, 0x6c, 0x6f, 0x77,
];

/// Builds the reviewed max-text literal: the two-byte canonical uvarint prefix
/// for 1024 followed by 1024 `a` bytes, which is exactly the wire ceiling.
fn max_text_wire() -> Vec<u8> {
    let mut wire = vec![0x80, 0x08];
    wire.extend(std::iter::repeat_n(b'a', CHAT_COMMAND_TEXT_MAX_BYTES));
    wire
}

/// One chat command vector: the reviewed record and the exact wire payload
/// the Go encoder publishes for it.
struct ClientChatVector {
    label: &'static str,
    text: String,
    payload: Vec<u8>,
}

/// The two canonical chat command vectors: the reviewed valid instance beside
/// the exact wire payload the Go encoder publishes for it.
fn client_chat_vectors() -> Vec<ClientChatVector> {
    vec![
        ClientChatVector {
            label: "@mira follow",
            text: "@mira follow".to_owned(),
            payload: MIRA_WIRE.to_vec(),
        },
        ClientChatVector {
            label: "1024 a bytes",
            text: "a".repeat(CHAT_COMMAND_TEXT_MAX_BYTES),
            payload: max_text_wire(),
        },
    ]
}

/// Builds a record through its public field instead of the constructor, which
/// is how a mutated record reaches the surface: the constructor gate is not
/// the only thing that has to hold.
fn mutated(text: String) -> ChatCommand {
    ChatCommand { text }
}

#[test]
fn client_chat_records_round_trip_through_the_fallible_surface() {
    for vector in client_chat_vectors() {
        let record = ChatCommand::new(vector.text.clone()).unwrap_or_else(|err| {
            panic!(
                "{}: constructor refuses a valid text: {err:?}",
                vector.label
            )
        });

        record.validate().unwrap_or_else(|err| {
            panic!("{}: valid record fails validation: {err:?}", vector.label)
        });
        let length = record
            .encoded_len()
            .unwrap_or_else(|err| panic!("{}: encoded_len fails: {err:?}", vector.label));
        assert_eq!(
            length,
            vector.payload.len(),
            "{}: encoded_len disagrees with the reviewed payload",
            vector.label
        );
        // The prefix strides are pinned per shape: one byte below 128 text
        // bytes, two bytes at the maximum.
        if vector.label == "@mira follow" {
            assert_eq!(length, 13);
        } else {
            assert_eq!(length, CHAT_COMMAND_MAX_WIRE_BYTES);
        }

        // An exact window publishes the reviewed bytes and reports the length.
        let mut exact = vec![0u8; length];
        let written = record
            .encode_into(&mut exact)
            .unwrap_or_else(|err| panic!("{}: encode_into fails: {err:?}", vector.label));
        assert_eq!(
            written, length,
            "{}: encode_into reports a short write",
            vector.label
        );
        assert_eq!(
            exact, vector.payload,
            "{}: encode_into published unexpected bytes",
            vector.label
        );

        // The allocating wrapper agrees byte for byte with the caller-owned one.
        let allocated = record
            .encode()
            .unwrap_or_else(|err| panic!("{}: encode fails: {err:?}", vector.label));
        assert_eq!(
            allocated, vector.payload,
            "{}: encode disagrees with encode_into",
            vector.label
        );

        // A larger destination is written only in dst[..length].
        let mut padded = vec![SENTINEL; length + 3];
        let written = record
            .encode_into(&mut padded)
            .unwrap_or_else(|err| panic!("{}: padded encode_into fails: {err:?}", vector.label));
        assert_eq!(
            written, length,
            "{}: padded write reports a short length",
            vector.label
        );
        assert_eq!(
            &padded[..length],
            vector.payload.as_slice(),
            "{}: padded prefix differs from the reviewed payload",
            vector.label
        );
        assert!(
            padded[length..].iter().all(|byte| *byte == SENTINEL),
            "{}: padded write touched bytes beyond the record",
            vector.label
        );

        // The decoded record equals the encoded one, and re-encoding it
        // reproduces the reviewed bytes, so both directions share one canonical
        // form.
        let decoded = ChatCommand::decode(&vector.payload)
            .unwrap_or_else(|err| panic!("{}: decode fails: {err:?}", vector.label));
        assert_eq!(
            decoded, record,
            "{}: decoded record differs from the encoded one",
            vector.label
        );
        assert_eq!(
            decoded.text, vector.text,
            "{}: the decoded text is not carried verbatim",
            vector.label
        );
        let reencoded = decoded
            .encode()
            .unwrap_or_else(|err| panic!("{}: re-encode fails: {err:?}", vector.label));
        assert_eq!(
            reencoded, vector.payload,
            "{}: re-encoded payload differs from the reviewed bytes",
            vector.label
        );
    }
}

#[test]
fn client_chat_the_text_is_never_interpreted_on_the_wire() {
    // A text without the mention prefix stays wire-valid, and the mention
    // text is carried verbatim: the payload is exactly the length prefix and
    // the text bytes, so no addressing, `/warp` handling, FIFO entry, session
    // or sequence field can ride along. The exact-literal assertions in the
    // round-trip test are the pin; this test states the invariant.
    let plain = ChatCommand::new("hello there".to_owned()).expect("plain text is wire-valid");
    let payload = plain.encode().expect("plain text encodes");
    assert_eq!(payload, {
        let mut expected = vec![11u8];
        expected.extend_from_slice(b"hello there");
        expected
    });
    assert_eq!(ChatCommand::decode(&payload), Ok(plain));

    let mention = ChatCommand::new("@mira follow".to_owned()).expect("mention text is wire-valid");
    assert_eq!(mention.text, "@mira follow");
    assert_eq!(mention.encode().expect("mention text encodes"), MIRA_WIRE);
}

#[test]
fn client_chat_text_boundaries_are_pinned() {
    // Every boundary is pinned through both the constructor and a mutated
    // public field, because the constructor gate is not the only thing that
    // has to hold: the field is public, so the encode-side gate owns the rule.
    let valid = ChatCommand::new("chop oak".to_owned()).expect("valid text");

    let rejects: Vec<(&str, String)> = vec![
        ("empty", String::new()),
        ("trailing space", "chop ".to_owned()),
        ("leading space", " chop".to_owned()),
        ("leading NBSP", "\u{00a0}abc".to_owned()),
        ("NUL", "a\u{0}b".to_owned()),
        ("U+007F control", "a\u{7f}b".to_owned()),
        ("1025 bytes", "a".repeat(CHAT_COMMAND_TEXT_MAX_BYTES + 1)),
    ];
    for (label, text) in rejects {
        assert!(
            ChatCommand::new(text.clone()).is_err(),
            "{label}: the constructor admits it"
        );
        assert_eq!(
            mutated(text).validate(),
            Err(ProtocolError::InvalidString),
            "{label}: the mutated field passes validation"
        );
    }

    // The maximum is admitted by the constructor and by a mutated field alike.
    let maximum = "a".repeat(CHAT_COMMAND_TEXT_MAX_BYTES);
    assert_eq!(
        ChatCommand::new(maximum.clone()).map(|record| record.text),
        Ok(maximum.clone())
    );
    assert_eq!(mutated(maximum).validate(), Ok(()));
    assert_eq!(valid.validate(), Ok(()));
}

/// One mutate-after-construction case: the invalid value a public mutable
/// field is set to after construction, and the error the surface has to
/// report for it before any size or capacity decision.
struct ClientChatMutation {
    label: &'static str,
    text: String,
}

/// The invalid mutable values. Each entry carries one violation, so the error
/// variant names the boundary that owns it.
fn client_chat_mutations() -> Vec<ClientChatMutation> {
    vec![
        ClientChatMutation {
            label: "untrimmed text after construction",
            text: " x".to_owned(),
        },
        ClientChatMutation {
            label: "text above the byte bound after construction",
            text: "a".repeat(CHAT_COMMAND_TEXT_MAX_BYTES + 1),
        },
    ]
}

#[test]
fn client_chat_invalid_value_wins_over_short_capacity() {
    for mutation in client_chat_mutations() {
        // The unmutated record sizes the destinations, so the capacity refusal
        // and the value refusal are compared on buffers of the same shape.
        let needed = ChatCommand::new("chop oak".to_owned())
            .expect("valid text")
            .encoded_len()
            .expect("the valid record sizes");
        let record = mutated(mutation.text);

        // The value gate runs before the size and capacity decisions, so every
        // entry point reports the same value error for the mutated record.
        assert_eq!(
            record.validate(),
            Err(ProtocolError::InvalidString),
            "{}: validate disagrees with the mutation",
            mutation.label
        );
        assert_eq!(
            record.encoded_len(),
            Err(ProtocolError::InvalidString),
            "{}: encoded_len disagrees with the mutation",
            mutation.label
        );
        assert_eq!(
            record.encode(),
            Err(ProtocolError::InvalidString),
            "{}: encode disagrees with the mutation",
            mutation.label
        );

        // A short destination refuses with the value error, not a capacity
        // error, and is left untouched.
        let mut short = vec![SENTINEL; needed - 1];
        assert_eq!(
            record.encode_into(&mut short),
            Err(ProtocolError::InvalidString),
            "{}: a short destination masks the value error",
            mutation.label
        );
        assert!(
            short.iter().all(|byte| *byte == SENTINEL),
            "{}: a short destination was modified on a value refusal",
            mutation.label
        );

        // An exact destination refuses the same way and is left untouched too.
        let mut exact = vec![SENTINEL; needed];
        assert_eq!(
            record.encode_into(&mut exact),
            Err(ProtocolError::InvalidString),
            "{}: an exact destination masks the value error",
            mutation.label
        );
        assert!(
            exact.iter().all(|byte| *byte == SENTINEL),
            "{}: an exact destination was modified on a value refusal",
            mutation.label
        );
    }
}

#[test]
fn client_chat_encode_into_leaves_a_short_destination_unchanged() {
    let record = ChatCommand::new("chop oak".to_owned()).expect("valid text");
    let needed = record.encoded_len().expect("the valid record sizes");
    for available in [0usize, needed - 1, needed - 3] {
        let mut dst = vec![SENTINEL; available];
        let error = record
            .encode_into(&mut dst)
            .expect_err("a short destination must be refused");
        assert_eq!(
            error,
            ProtocolError::OutputTooSmall { needed, available },
            "short destination of {available} bytes reports the wrong error"
        );
        assert!(
            dst.iter().all(|byte| *byte == SENTINEL),
            "short destination of {available} bytes was modified"
        );
    }
}

#[test]
fn client_chat_decode_rejects_every_proper_truncation() {
    // Cut 0 is the empty payload, where the Rust uvarint reader reports a
    // missing first byte as `Truncated`; the Go primitive answers the same
    // bytes with its invalid-uvarint sentinel instead, and no corpus case
    // registers an empty payload for this family, so the divergence stays
    // outside the frozen evidence. Every cut inside the text is the boundary
    // the new reader owns.
    for cut in 0..MIRA_WIRE.len() {
        let truncated = &MIRA_WIRE[..cut];
        let error =
            ChatCommand::decode(truncated).expect_err("a proper truncation must never decode");
        assert_eq!(
            error,
            ProtocolError::Truncated,
            "truncation at {cut} bytes reports {error:?}"
        );
    }
}

#[test]
fn client_chat_decode_rejects_one_trailing_byte() {
    // The trailing byte is pinned on the reviewed payload, whose length stays
    // below the wire ceiling. The maximum-length vector plus one byte is above
    // the ceiling instead, so the Go decoder and this one answer it with the
    // ceiling refusal before any field is read, which is the behavior the
    // ceiling test pins.
    let mut trailing = MIRA_WIRE.to_vec();
    trailing.push(0x00);
    let error = ChatCommand::decode(&trailing).expect_err("one trailing byte must never decode");
    assert_eq!(
        error,
        ProtocolError::TrailingBytes,
        "one trailing byte reports {error:?}"
    );
    let mut maximum_plus_one = max_text_wire();
    maximum_plus_one.push(0x00);
    assert_eq!(
        ChatCommand::decode(&maximum_plus_one),
        Err(ProtocolError::FrameTooLarge)
    );
}

#[test]
fn client_chat_decode_refuses_the_wire_ceiling_before_any_parse() {
    // A payload one byte above the fixed ceiling is refused with the ceiling
    // error before a single field is read, which is the `capacity` category
    // the Go decoder publishes for the same bytes through its own pre-parse
    // payload ceiling. A payload of exactly the ceiling is admitted, which the
    // max-text round-trip test pins.
    let mut above = vec![0x81, 0x08];
    above.extend(std::iter::repeat_n(b'a', CHAT_COMMAND_TEXT_MAX_BYTES + 1));
    assert_eq!(above.len(), CHAT_COMMAND_MAX_WIRE_BYTES + 1);
    assert_eq!(
        ChatCommand::decode(&above),
        Err(ProtocolError::FrameTooLarge)
    );
    assert_eq!(max_text_wire().len(), CHAT_COMMAND_MAX_WIRE_BYTES);
    assert!(ChatCommand::decode(&max_text_wire()).is_ok());
}

#[test]
fn client_chat_decode_refuses_the_pinned_invalid_values() {
    // The matrix mirrors the corpus negatives. Every entry is a single
    // violation, and every text-bound rejection is `InvalidString` on the
    // Rust side, which is the `invalid-value` category the Go producer records
    // for the same bytes through its validator.
    let cases: Vec<(&str, Vec<u8>, ProtocolError)> = vec![
        ("empty text", vec![0x00], ProtocolError::InvalidString),
        (
            "leading NBSP",
            vec![0x05, 0xc2, 0xa0, 0x61, 0x62, 0x63],
            ProtocolError::InvalidString,
        ),
        (
            "control character",
            vec![0x03, 0x61, 0x01, 0x62],
            ProtocolError::InvalidString,
        ),
        (
            "invalid UTF-8",
            vec![0x02, 0xc3, 0x28],
            ProtocolError::InvalidString,
        ),
        (
            "noncanonical length prefix",
            vec![0x80, 0x00, 0x61],
            ProtocolError::NonCanonicalUvarint,
        ),
        (
            "declared length the payload cannot complete",
            vec![0x0c, 0x40, 0x6d, 0x69, 0x72, 0x61],
            ProtocolError::Truncated,
        ),
    ];
    for (label, payload, error) in cases {
        assert_eq!(
            ChatCommand::decode(&payload),
            Err(error),
            "{label}: the decoder reports a different boundary"
        );
    }
}

#[test]
fn client_chat_packet_id_is_pinned() {
    assert_eq!(ChatCommand::PACKET_ID, 12);
}

#[test]
fn client_chat_the_text_bound_matches_the_planner_instruction_limit() {
    // The wire bound and the domain bound are one number, so a command that
    // reaches the planner always fits the wire slot the authority reads.
    assert_eq!(CHAT_COMMAND_TEXT_MAX_BYTES, 1024);
    assert_eq!(
        CHAT_COMMAND_MAX_WIRE_BYTES,
        CHAT_COMMAND_TEXT_MAX_BYTES + 2,
        "the wire ceiling is the two-byte maximum prefix plus the text"
    );
}
