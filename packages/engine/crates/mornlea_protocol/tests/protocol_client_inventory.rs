//! Simple inventory and crafting command packet families: the common fallible
//! encode surface and the decode boundaries the six Play client-to-server
//! command records share.
//!
//! Every inventory command record is a concrete packet with public mutable
//! fields, so the group pins the design's surface order (`validate` → checked
//! `encoded_len` → capacity check → private `publish_packet`) for each family:
//! a short or invalid `encode_into` leaves the caller's buffer untouched, an
//! invalid value wins over a short destination, and every proper truncation of
//! a canonical payload rejects. The wire literals are the Go encoder's output
//! for these fields, so the round-trip tests pin byte-level parity rather than
//! self-agreement. The corpus evidence these families publish is executed by
//! `tests/protocol_corpus.rs` once the controller integrates the exported
//! assets, so this suite stays self-contained and needs no corpus files.
//!
//! Two payload shapes are in scope. The two move commands carry a sequence and
//! two slot bytes (10 bytes); the four sequence-only commands carry the
//! sequence alone (8 bytes). Inventory contents, the moved count, the crafting
//! output recipe, the drop position and the equipped armor slot stay
//! server-owned, so no payload carries any of them.

use mornlea_protocol::{
    CloseContainer, DropSelectedItem, EquipArmor, MoveCraftingStack, MoveInventoryStack,
    ProtocolError, TakeCraftingOutput,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The fixed wire stride of one move payload: the 8-byte sequence and the two
/// slot bytes, with no item, count or recipe field.
const MOVE_WIRE_BYTES: usize = 10;

/// The fixed wire stride of one sequence-only payload: the 8-byte sequence
/// alone, with no item count, drop position or armor slot.
const SEQUENCE_ONLY_WIRE_BYTES: usize = 8;

/// One type-erased inventory command record for the family-generic
/// assertions.
///
/// Each variant carries the record by value, so the surface tests run the same
/// sequence for every family without generics over the packet types.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum ClientInventoryRecord {
    MoveInventory(MoveInventoryStack),
    MoveCrafting(MoveCraftingStack),
    CloseContainer(CloseContainer),
    DropSelected(DropSelectedItem),
    EquipArmor(EquipArmor),
    TakeCraftingOutput(TakeCraftingOutput),
}

impl ClientInventoryRecord {
    fn validate(&self) -> Result<(), ProtocolError> {
        match self {
            ClientInventoryRecord::MoveInventory(record) => record.validate(),
            ClientInventoryRecord::MoveCrafting(record) => record.validate(),
            ClientInventoryRecord::CloseContainer(record) => record.validate(),
            ClientInventoryRecord::DropSelected(record) => record.validate(),
            ClientInventoryRecord::EquipArmor(record) => record.validate(),
            ClientInventoryRecord::TakeCraftingOutput(record) => record.validate(),
        }
    }

    fn encoded_len(&self) -> Result<usize, ProtocolError> {
        match self {
            ClientInventoryRecord::MoveInventory(record) => record.encoded_len(),
            ClientInventoryRecord::MoveCrafting(record) => record.encoded_len(),
            ClientInventoryRecord::CloseContainer(record) => record.encoded_len(),
            ClientInventoryRecord::DropSelected(record) => record.encoded_len(),
            ClientInventoryRecord::EquipArmor(record) => record.encoded_len(),
            ClientInventoryRecord::TakeCraftingOutput(record) => record.encoded_len(),
        }
    }

    fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        match self {
            ClientInventoryRecord::MoveInventory(record) => record.encode_into(dst),
            ClientInventoryRecord::MoveCrafting(record) => record.encode_into(dst),
            ClientInventoryRecord::CloseContainer(record) => record.encode_into(dst),
            ClientInventoryRecord::DropSelected(record) => record.encode_into(dst),
            ClientInventoryRecord::EquipArmor(record) => record.encode_into(dst),
            ClientInventoryRecord::TakeCraftingOutput(record) => record.encode_into(dst),
        }
    }

    fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        match self {
            ClientInventoryRecord::MoveInventory(record) => record.encode(),
            ClientInventoryRecord::MoveCrafting(record) => record.encode(),
            ClientInventoryRecord::CloseContainer(record) => record.encode(),
            ClientInventoryRecord::DropSelected(record) => record.encode(),
            ClientInventoryRecord::EquipArmor(record) => record.encode(),
            ClientInventoryRecord::TakeCraftingOutput(record) => record.encode(),
        }
    }
}

/// One inventory command family's canonical valid instance and wire payload.
///
/// The wire literals are the Go encoder's output for these fields, so the
/// round-trip tests pin byte-level parity rather than self-agreement.
struct ClientInventoryVector {
    label: &'static str,
    payload: Vec<u8>,
    build: fn() -> ClientInventoryRecord,
}

/// Six canonical inventory command records: the reviewed valid instance of
/// each family beside the exact wire payload the Go encoder publishes for it.
///
/// Every sequence is zero except `TakeCraftingOutput`, whose zero sequence is
/// refused, so its canonical vector carries sequence 1. The canonical move
/// slots are the reviewed boundary values: an inventory move from hotbar slot 0
/// to backpack slot 35, and a crafting move from grid slot 8 to inventory slot
/// 44.
fn client_inventory_vectors() -> Vec<ClientInventoryVector> {
    vec![
        ClientInventoryVector {
            label: "move inventory stack",
            payload: vec![0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x23],
            build: || {
                ClientInventoryRecord::MoveInventory(
                    MoveInventoryStack::new(0, 0, 35).expect("valid inventory move"),
                )
            },
        },
        ClientInventoryVector {
            label: "move crafting stack",
            payload: vec![0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0x2c],
            build: || {
                ClientInventoryRecord::MoveCrafting(
                    MoveCraftingStack::new(0, 8, 44).expect("valid crafting move"),
                )
            },
        },
        ClientInventoryVector {
            label: "close container",
            payload: vec![0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
            build: || ClientInventoryRecord::CloseContainer(CloseContainer::new(0)),
        },
        ClientInventoryVector {
            label: "drop selected item",
            payload: vec![0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
            build: || ClientInventoryRecord::DropSelected(DropSelectedItem::new(0)),
        },
        ClientInventoryVector {
            label: "equip armor",
            payload: vec![0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
            build: || ClientInventoryRecord::EquipArmor(EquipArmor::new(0)),
        },
        ClientInventoryVector {
            label: "take crafting output",
            payload: vec![0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00],
            build: || {
                ClientInventoryRecord::TakeCraftingOutput(
                    TakeCraftingOutput::new(1).expect("valid take crafting output"),
                )
            },
        },
    ]
}

/// Decodes one canonical payload through the family its label names.
fn decode_of(label: &str, payload: &[u8]) -> Result<ClientInventoryRecord, ProtocolError> {
    let decoded = match label {
        "move inventory stack" => {
            ClientInventoryRecord::MoveInventory(MoveInventoryStack::decode(payload)?)
        }
        "move crafting stack" => {
            ClientInventoryRecord::MoveCrafting(MoveCraftingStack::decode(payload)?)
        }
        "close container" => {
            ClientInventoryRecord::CloseContainer(CloseContainer::decode(payload)?)
        }
        "drop selected item" => {
            ClientInventoryRecord::DropSelected(DropSelectedItem::decode(payload)?)
        }
        "equip armor" => ClientInventoryRecord::EquipArmor(EquipArmor::decode(payload)?),
        "take crafting output" => {
            ClientInventoryRecord::TakeCraftingOutput(TakeCraftingOutput::decode(payload)?)
        }
        other => panic!("no decoder is registered for {other}"),
    };
    Ok(decoded)
}

#[test]
fn client_inventory_records_round_trip_through_the_fallible_surface() {
    for vector in client_inventory_vectors() {
        let record = (vector.build)();
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
        // The two move payloads are 10 bytes and the four sequence-only
        // payloads are 8, so the stride is pinned per shape rather than once
        // for the group.
        let want_stride = if vector.payload.len() == MOVE_WIRE_BYTES {
            MOVE_WIRE_BYTES
        } else {
            SEQUENCE_ONLY_WIRE_BYTES
        };
        assert_eq!(
            length, want_stride,
            "{}: payload stride disagrees with its shape",
            vector.label
        );

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
        let decoded = decode_of(vector.label, &vector.payload)
            .unwrap_or_else(|err| panic!("{}: decode fails: {err:?}", vector.label));
        assert_eq!(
            decoded, record,
            "{}: decoded record differs from the encoded one",
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
fn client_inventory_encode_into_leaves_a_short_destination_unchanged() {
    for vector in client_inventory_vectors() {
        let record = (vector.build)();
        let needed = vector.payload.len();

        for available in [0usize, needed - 1, needed - 3] {
            let mut dst = vec![SENTINEL; available];
            let error = record
                .encode_into(&mut dst)
                .expect_err("a short destination must be refused");
            assert_eq!(
                error,
                ProtocolError::OutputTooSmall { needed, available },
                "{}: short destination of {available} bytes reports the wrong error",
                vector.label
            );
            assert!(
                dst.iter().all(|byte| *byte == SENTINEL),
                "{}: short destination of {available} bytes was modified",
                vector.label
            );
        }
    }
}

/// One family's mutate-after-construction case: the canonical instance, the
/// invalid value a public mutable field is set to after construction, and the
/// error the surface has to report for it before any size or capacity decision.
struct ClientInventoryMutation {
    label: &'static str,
    build: fn() -> ClientInventoryRecord,
    mutate: fn(&mut ClientInventoryRecord),
    error: ProtocolError,
}

/// The families with an invalid mutable value: the two move commands and
/// `TakeCraftingOutput`. `CloseContainer`, `DropSelectedItem` and `EquipArmor`
/// have none, so the mutation matrix skips them.
fn client_inventory_mutations() -> Vec<ClientInventoryMutation> {
    vec![
        ClientInventoryMutation {
            label: "move inventory source above the inventory range",
            build: || {
                ClientInventoryRecord::MoveInventory(
                    MoveInventoryStack::new(0, 0, 35).expect("move"),
                )
            },
            mutate: |record| {
                if let ClientInventoryRecord::MoveInventory(stack) = record {
                    stack.from = 36;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientInventoryMutation {
            label: "move inventory target above the inventory range",
            build: || {
                ClientInventoryRecord::MoveInventory(
                    MoveInventoryStack::new(0, 0, 35).expect("move"),
                )
            },
            mutate: |record| {
                if let ClientInventoryRecord::MoveInventory(stack) = record {
                    stack.to = 36;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientInventoryMutation {
            label: "move inventory target equals source",
            build: || {
                ClientInventoryRecord::MoveInventory(
                    MoveInventoryStack::new(0, 0, 35).expect("move"),
                )
            },
            mutate: |record| {
                if let ClientInventoryRecord::MoveInventory(stack) = record {
                    stack.to = 0;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientInventoryMutation {
            label: "move crafting both ends in the inventory region",
            build: || {
                ClientInventoryRecord::MoveCrafting(MoveCraftingStack::new(0, 8, 44).expect("move"))
            },
            mutate: |record| {
                if let ClientInventoryRecord::MoveCrafting(stack) = record {
                    stack.from = 9;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientInventoryMutation {
            label: "move crafting target equals source",
            build: || {
                ClientInventoryRecord::MoveCrafting(MoveCraftingStack::new(0, 8, 44).expect("move"))
            },
            mutate: |record| {
                if let ClientInventoryRecord::MoveCrafting(stack) = record {
                    stack.to = 8;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientInventoryMutation {
            label: "move crafting target above the unified view",
            build: || {
                ClientInventoryRecord::MoveCrafting(MoveCraftingStack::new(0, 8, 44).expect("move"))
            },
            mutate: |record| {
                if let ClientInventoryRecord::MoveCrafting(stack) = record {
                    stack.to = 45;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientInventoryMutation {
            label: "take crafting output zero sequence",
            build: || {
                ClientInventoryRecord::TakeCraftingOutput(TakeCraftingOutput::new(1).expect("take"))
            },
            mutate: |record| {
                if let ClientInventoryRecord::TakeCraftingOutput(take) = record {
                    take.sequence = 0;
                }
            },
            error: ProtocolError::InvalidRange,
        },
    ]
}

#[test]
fn client_inventory_invalid_value_wins_over_short_capacity() {
    for mutation in client_inventory_mutations() {
        let mut record = (mutation.build)();
        // The unmutated record sizes the destinations, so the capacity refusal
        // and the value refusal are compared on buffers of the same shape.
        let needed = record.encoded_len().unwrap_or_else(|err| {
            panic!(
                "{}: the valid record fails to size: {err:?}",
                mutation.label
            )
        });
        (mutation.mutate)(&mut record);

        // The value gate runs before the size and capacity decisions, so every
        // entry point reports the same value error for the mutated record.
        assert_eq!(
            record.validate(),
            Err(mutation.error),
            "{}: validate disagrees with the mutation",
            mutation.label
        );
        assert_eq!(
            record.encoded_len(),
            Err(mutation.error),
            "{}: encoded_len disagrees with the mutation",
            mutation.label
        );
        assert_eq!(
            record.encode(),
            Err(mutation.error),
            "{}: encode disagrees with the mutation",
            mutation.label
        );

        // A short destination refuses with the value error, not a capacity
        // error, and is left untouched.
        let mut short = vec![SENTINEL; needed - 1];
        assert_eq!(
            record.encode_into(&mut short),
            Err(mutation.error),
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
            Err(mutation.error),
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

/// The three sequence-only families whose value gate is total: the Go
/// validator places no rule on the sequence, so no field mutation can make the
/// record unpublishable.
fn client_total_validate_vectors() -> Vec<ClientInventoryVector> {
    client_inventory_vectors()
        .into_iter()
        .filter(|vector| {
            matches!(
                vector.label,
                "close container" | "drop selected item" | "equip armor"
            )
        })
        .collect()
}

#[test]
fn sequence_only_families_keep_a_total_value_gate() {
    // `CloseContainer`, `DropSelectedItem` and `EquipArmor` carry only the
    // sequence, which the Go validator leaves unrestricted — including the full
    // u64 range. Mutating the sequence to `u64::MAX` is therefore not a
    // rejection but a re-encode byte-exactness proof: a total `validate` keeps
    // the fallible surface uniform without inventing a rule the Go side does
    // not publish.
    for vector in client_total_validate_vectors() {
        let mut record = (vector.build)();
        match &mut record {
            ClientInventoryRecord::CloseContainer(close) => close.sequence = u64::MAX,
            ClientInventoryRecord::DropSelected(drop) => drop.sequence = u64::MAX,
            ClientInventoryRecord::EquipArmor(equip) => equip.sequence = u64::MAX,
            other => panic!("{:?} is not a sequence-only family", other),
        }

        record.validate().unwrap_or_else(|err| {
            panic!(
                "{}: a total gate refuses a legal sequence: {err:?}",
                vector.label
            )
        });
        let mut expected = vec![0xff; SEQUENCE_ONLY_WIRE_BYTES];
        assert_eq!(
            record.encoded_len(),
            Ok(SEQUENCE_ONLY_WIRE_BYTES),
            "{}: the total gate sizes the record",
            vector.label
        );
        let mut exact = vec![SENTINEL; SEQUENCE_ONLY_WIRE_BYTES];
        assert_eq!(
            record.encode_into(&mut exact),
            Ok(SEQUENCE_ONLY_WIRE_BYTES),
            "{}: encode_into fails for a maximum sequence",
            vector.label
        );
        expected.resize(SEQUENCE_ONLY_WIRE_BYTES, 0xff);
        assert_eq!(
            exact, expected,
            "{}: the maximum sequence is not little-endian",
            vector.label
        );
        assert_eq!(
            record.encode(),
            Ok(expected.clone()),
            "{}: the allocating wrapper disagrees for a maximum sequence",
            vector.label
        );
        let decoded = decode_of(vector.label, &expected)
            .unwrap_or_else(|err| panic!("{}: decode fails: {err:?}", vector.label));
        assert_eq!(
            decoded, record,
            "{}: the maximum sequence does not round-trip",
            vector.label
        );
    }
}

#[test]
fn client_inventory_decode_rejects_every_proper_truncation() {
    for vector in client_inventory_vectors() {
        for cut in 0..vector.payload.len() {
            let truncated = &vector.payload[..cut];
            let error = decode_of(vector.label, truncated)
                .expect_err("a proper truncation must never decode");
            assert_eq!(
                error,
                ProtocolError::Truncated,
                "{}: truncation at {cut} bytes reports {error:?}",
                vector.label
            );
        }
    }
}

#[test]
fn client_inventory_decode_rejects_one_trailing_byte() {
    for vector in client_inventory_vectors() {
        let mut trailing = vector.payload.clone();
        trailing.push(0x00);
        let error =
            decode_of(vector.label, &trailing).expect_err("one trailing byte must never decode");
        assert_eq!(
            error,
            ProtocolError::TrailingBytes,
            "{}: one trailing byte reports {error:?}",
            vector.label
        );
    }
}

/// Builds one 10-byte move payload from a sequence and two slot bytes.
fn move_payload(from: u8, to: u8) -> Vec<u8> {
    let mut payload = vec![0u8; 8];
    payload.push(from);
    payload.push(to);
    payload
}

/// Builds one 8-byte sequence-only payload.
fn sequence_payload() -> Vec<u8> {
    vec![0u8; 8]
}

#[test]
fn client_inventory_decode_rejects_the_pinned_invalid_values() {
    // The matrix mirrors the corpus negatives: an out-of-range slot on either
    // end, a same-slot pair, the crafting view's two-inventory exclusion and
    // the crafting output's zero sequence. Every one is `InvalidRange` because
    // the Go validators reject the same bytes.
    let cases: Vec<(&str, Vec<u8>)> = vec![
        ("move inventory stack", move_payload(36, 35)),
        ("move inventory stack", move_payload(0, 36)),
        ("move inventory stack", move_payload(0, 0)),
        ("move crafting stack", move_payload(9, 44)),
        ("move crafting stack", move_payload(8, 8)),
        ("move crafting stack", move_payload(8, 45)),
        ("take crafting output", sequence_payload()),
    ];
    for (label, payload) in cases {
        let error = decode_of(label, &payload)
            .expect_err("the pinned payload must be refused by the decoder");
        assert_eq!(
            error,
            ProtocolError::InvalidRange,
            "{label}: the pinned payload reports {error:?}"
        );
    }
}

#[test]
fn client_inventory_packet_ids_are_pinned() {
    assert_eq!(MoveInventoryStack::PACKET_ID, 6);
    assert_eq!(MoveCraftingStack::PACKET_ID, 7);
    assert_eq!(CloseContainer::PACKET_ID, 10);
    assert_eq!(DropSelectedItem::PACKET_ID, 11);
    assert_eq!(TakeCraftingOutput::PACKET_ID, 15);
    assert_eq!(EquipArmor::PACKET_ID, 18);
}

#[test]
fn client_inventory_payload_purity_is_pinned() {
    // The three sequence-only payloads are exactly 8 bytes: an item count, a
    // drop location and a target armor slot are all server-owned decisions the
    // wire never carries, so a payload that grew any of them would break the
    // protocol rather than the encoder. The two move payloads carry exactly
    // the sequence and the two slots, with no moved item or count.
    for vector in client_inventory_vectors() {
        match vector.label {
            "close container" | "drop selected item" | "equip armor" => assert_eq!(
                vector.payload.len(),
                SEQUENCE_ONLY_WIRE_BYTES,
                "{}: a sequence-only payload is exactly the 8-byte sequence",
                vector.label
            ),
            "move inventory stack" | "move crafting stack" => {
                assert_eq!(
                    vector.payload.len(),
                    MOVE_WIRE_BYTES,
                    "{}: a move payload is exactly the sequence and two slots",
                    vector.label
                );
                assert_eq!(
                    &vector.payload[..8],
                    &[0u8; 8],
                    "{}: the canonical sequence is zero and leads the payload",
                    vector.label
                );
            }
            "take crafting output" => assert_eq!(
                vector.payload.len(),
                SEQUENCE_ONLY_WIRE_BYTES,
                "{}: a sequence-only payload is exactly the 8-byte sequence",
                vector.label
            ),
            other => panic!("unknown inventory vector {other}"),
        }
    }
}
