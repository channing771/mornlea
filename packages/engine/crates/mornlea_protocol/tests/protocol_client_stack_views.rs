//! The four container and view-addressed stack command packet families: the
//! common fallible surface and the container-reference gates the Go validators
//! publish.
//!
//! Every stack-view command record is a concrete packet with public mutable
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
//! Two reference regimes are in scope. `MoveContainerStack` always carries a
//! real reference and dispatches its unified slot range by the reference's
//! kind; the three view-addressed commands carry the exact all-zero reference
//! in the inventory and crafting views and a real reference in the container
//! view. A malformed real reference is refused before any index rule, because
//! the Go validators check `validAnyContainerRef` first: a foreign dimension,
//! an unknown kind, a zero generation and a physical slot outside its
//! per-chunk array are all wire-level refusals rather than authority rules.
//! The moved amount, the transfer destination and the world drop position stay
//! server-owned, so no payload carries any of them.

use mornlea_protocol::{
    CONTAINER_KIND_CHEST, CONTAINER_KIND_FURNACE, ContainerRef, DropStack, MoveContainerStack,
    MoveStackPartial, ProtocolError, QuickMoveStack,
};

/// The sentinel a destination buffer is filled with, so an assertion that the
/// buffer is unchanged is an assertion about every byte.
const SENTINEL: u8 = 0xa5;

/// The fixed wire stride of one container move payload: the 8-byte sequence,
/// the 18-byte reference and the two unified view slot bytes.
const MOVE_CONTAINER_WIRE_BYTES: usize = 28;

/// The fixed wire stride of one partial move payload: the container move
/// prefix plus the view byte, the second slot and the single-item flag.
const MOVE_PARTIAL_WIRE_BYTES: usize = 30;

/// The fixed wire stride of one view-addressed payload with a single index:
/// the sequence, the reference, the view byte and one unified index byte.
const VIEW_ADDRESSED_WIRE_BYTES: usize = 28;

/// Builds the reviewed furnace reference bytes: overworld dimension, chunk
/// −1/2, kind 0, physical slot 31 and generation 1.
fn furnace_ref_bytes() -> [u8; 18] {
    ref_bytes(0, -1, 2, CONTAINER_KIND_FURNACE, 31, 1)
}

/// Builds the reviewed chest reference bytes: overworld dimension, chunk
/// −1/2, kind 1, physical slot 15 and generation 1.
fn chest_ref_bytes() -> [u8; 18] {
    ref_bytes(0, -1, 2, CONTAINER_KIND_CHEST, 15, 1)
}

/// Builds the exact all-zero absent reference the inventory and crafting views
/// carry.
fn none_ref_bytes() -> [u8; 18] {
    [0u8; 18]
}

/// Builds one 18-byte container reference in the wire's field order: the
/// little-endian `i32` dimension, the two chunk coordinates, the kind byte, the
/// physical slot byte and the little-endian `u32` generation.
fn ref_bytes(
    dimension: i32,
    chunk_x: i32,
    chunk_z: i32,
    kind: u8,
    slot: u8,
    generation: u32,
) -> [u8; 18] {
    let mut bytes = [0u8; 18];
    bytes[0..4].copy_from_slice(&dimension.to_le_bytes());
    bytes[4..8].copy_from_slice(&chunk_x.to_le_bytes());
    bytes[8..12].copy_from_slice(&chunk_z.to_le_bytes());
    bytes[12] = kind;
    bytes[13] = slot;
    bytes[14..18].copy_from_slice(&generation.to_le_bytes());
    bytes
}

/// Replaces the reference bytes of one payload, leaving every other field in
/// place, so a malformed-reference case is a single-field mutation of a valid
/// payload.
fn with_ref(payload: &[u8], reference: [u8; 18]) -> Vec<u8> {
    let mut mutated = payload.to_vec();
    mutated[8..26].copy_from_slice(&reference);
    mutated
}

/// Replaces one byte of a payload at `offset`.
fn with_byte(payload: &[u8], offset: usize, value: u8) -> Vec<u8> {
    let mut mutated = payload.to_vec();
    mutated[offset] = value;
    mutated
}

/// One type-erased stack-view command record for the family-generic
/// assertions.
///
/// Each variant carries the record by value, so the surface tests run the same
/// sequence for every family without generics over the packet types.
#[derive(Clone, Copy, Debug, Eq, PartialEq)]
enum ClientStackViewRecord {
    MoveContainer(MoveContainerStack),
    MovePartial(MoveStackPartial),
    QuickMove(QuickMoveStack),
    DropStack(DropStack),
}

impl ClientStackViewRecord {
    fn validate(&self) -> Result<(), ProtocolError> {
        match self {
            ClientStackViewRecord::MoveContainer(record) => record.validate(),
            ClientStackViewRecord::MovePartial(record) => record.validate(),
            ClientStackViewRecord::QuickMove(record) => record.validate(),
            ClientStackViewRecord::DropStack(record) => record.validate(),
        }
    }

    fn encoded_len(&self) -> Result<usize, ProtocolError> {
        match self {
            ClientStackViewRecord::MoveContainer(record) => record.encoded_len(),
            ClientStackViewRecord::MovePartial(record) => record.encoded_len(),
            ClientStackViewRecord::QuickMove(record) => record.encoded_len(),
            ClientStackViewRecord::DropStack(record) => record.encoded_len(),
        }
    }

    fn encode_into(&self, dst: &mut [u8]) -> Result<usize, ProtocolError> {
        match self {
            ClientStackViewRecord::MoveContainer(record) => record.encode_into(dst),
            ClientStackViewRecord::MovePartial(record) => record.encode_into(dst),
            ClientStackViewRecord::QuickMove(record) => record.encode_into(dst),
            ClientStackViewRecord::DropStack(record) => record.encode_into(dst),
        }
    }

    fn encode(&self) -> Result<Vec<u8>, ProtocolError> {
        match self {
            ClientStackViewRecord::MoveContainer(record) => record.encode(),
            ClientStackViewRecord::MovePartial(record) => record.encode(),
            ClientStackViewRecord::QuickMove(record) => record.encode(),
            ClientStackViewRecord::DropStack(record) => record.encode(),
        }
    }

    /// The reference the record carries, so the absent-sentinel pins read the
    /// published value instead of the constructor's input.
    fn container(&self) -> ContainerRef {
        match self {
            ClientStackViewRecord::MoveContainer(record) => record.container,
            ClientStackViewRecord::MovePartial(record) => record.container,
            ClientStackViewRecord::QuickMove(record) => record.container,
            ClientStackViewRecord::DropStack(record) => record.container,
        }
    }
}

/// One stack-view command family's canonical valid instance and wire payload.
///
/// The wire literals are the Go encoder's output for these fields, so the
/// round-trip tests pin byte-level parity rather than self-agreement.
struct ClientStackViewVector {
    label: &'static str,
    payload: Vec<u8>,
    build: fn() -> ClientStackViewRecord,
}

/// Four canonical stack-view command records: the reviewed valid instance of
/// each family beside the exact wire payload the Go encoder publishes for it.
///
/// Every sequence is zero. `MoveContainerStack` addresses a furnace from slot 0
/// to slot 37, one slot below the output slot it may not target;
/// `MoveStackPartial` moves half a chest stack from slot 36 to the last chest
/// slot; `QuickMoveStack` and `DropStack` address the crafting and inventory
/// views and therefore carry the exact all-zero absent reference.
fn client_stack_view_vectors() -> Vec<ClientStackViewVector> {
    vec![
        ClientStackViewVector {
            label: "move container stack",
            payload: move_container_payload(),
            build: || {
                ClientStackViewRecord::MoveContainer(
                    MoveContainerStack::new(0, furnace_ref(), 0, 37).expect("valid container move"),
                )
            },
        },
        ClientStackViewVector {
            label: "move stack partial",
            payload: move_partial_payload(),
            build: || {
                ClientStackViewRecord::MovePartial(
                    MoveStackPartial::new(0, chest_ref(), 2, 36, 62, true)
                        .expect("valid partial move"),
                )
            },
        },
        ClientStackViewVector {
            label: "quick move stack",
            payload: quick_move_payload(),
            build: || {
                ClientStackViewRecord::QuickMove(
                    QuickMoveStack::new(0, ContainerRef::NONE, 1, 44).expect("valid quick move"),
                )
            },
        },
        ClientStackViewVector {
            label: "drop stack",
            payload: drop_stack_payload(),
            build: || {
                ClientStackViewRecord::DropStack(
                    DropStack::new(0, ContainerRef::NONE, 0, 35).expect("valid stack drop"),
                )
            },
        },
    ]
}

/// The reviewed furnace reference: overworld, chunk −1/2, kind 0, slot 31 and
/// generation 1.
fn furnace_ref() -> ContainerRef {
    ContainerRef {
        dimension: 0,
        chunk_x: -1,
        chunk_z: 2,
        kind: CONTAINER_KIND_FURNACE,
        slot: 31,
        generation: 1,
    }
}

/// The reviewed chest reference: overworld, chunk −1/2, kind 1, slot 15 and
/// generation 1.
fn chest_ref() -> ContainerRef {
    ContainerRef {
        dimension: 0,
        chunk_x: -1,
        chunk_z: 2,
        kind: CONTAINER_KIND_CHEST,
        slot: 15,
        generation: 1,
    }
}

/// The reviewed wire literal the `MoveContainerStack` canonical vector pins.
fn move_container_payload() -> Vec<u8> {
    let mut payload = zero_sequence();
    payload.extend_from_slice(&furnace_ref_bytes());
    payload.push(0x00);
    payload.push(0x25);
    payload
}

/// The reviewed wire literal the `MoveStackPartial` canonical vector pins.
fn move_partial_payload() -> Vec<u8> {
    let mut payload = zero_sequence();
    payload.extend_from_slice(&chest_ref_bytes());
    payload.push(0x02);
    payload.push(0x24);
    payload.push(0x3e);
    payload.push(0x01);
    payload
}

/// The reviewed wire literal the `QuickMoveStack` canonical vector pins.
fn quick_move_payload() -> Vec<u8> {
    let mut payload = zero_sequence();
    payload.extend_from_slice(&none_ref_bytes());
    payload.push(0x01);
    payload.push(0x2c);
    payload
}

/// The reviewed wire literal the `DropStack` canonical vector pins.
fn drop_stack_payload() -> Vec<u8> {
    let mut payload = zero_sequence();
    payload.extend_from_slice(&none_ref_bytes());
    payload.push(0x00);
    payload.push(0x23);
    payload
}

/// The eight zero sequence bytes every canonical payload leads with.
fn zero_sequence() -> Vec<u8> {
    vec![0u8; 8]
}

/// Decodes one canonical payload through the family its label names.
fn decode_of(label: &str, payload: &[u8]) -> Result<ClientStackViewRecord, ProtocolError> {
    let decoded = match label {
        "move container stack" => {
            ClientStackViewRecord::MoveContainer(MoveContainerStack::decode(payload)?)
        }
        "move stack partial" => {
            ClientStackViewRecord::MovePartial(MoveStackPartial::decode(payload)?)
        }
        "quick move stack" => ClientStackViewRecord::QuickMove(QuickMoveStack::decode(payload)?),
        "drop stack" => ClientStackViewRecord::DropStack(DropStack::decode(payload)?),
        other => panic!("no decoder is registered for {other}"),
    };
    Ok(decoded)
}

#[test]
fn client_stack_view_canonical_literals_are_pinned() {
    // The full literals are pinned rather than derived, so a byte-for-byte
    // comparison against the Go encoder's output is the contract.
    assert_eq!(
        move_container_payload(),
        [
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff,
            0xff, 0xff, 0x02, 0x00, 0x00, 0x00, 0x00, 0x1f, 0x01, 0x00, 0x00, 0x00, 0x00, 0x25
        ]
    );
    assert_eq!(
        move_partial_payload(),
        [
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff,
            0xff, 0xff, 0x02, 0x00, 0x00, 0x00, 0x01, 0x0f, 0x01, 0x00, 0x00, 0x00, 0x02, 0x24,
            0x3e, 0x01
        ]
    );
    assert_eq!(
        quick_move_payload(),
        [
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x2c
        ]
    );
    assert_eq!(
        drop_stack_payload(),
        [
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
            0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x23
        ]
    );
}

#[test]
fn client_stack_view_records_round_trip_through_the_fallible_surface() {
    for vector in client_stack_view_vectors() {
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
        // The stride is pinned per shape: the container move and the two
        // single-index view commands are 28 bytes, the partial move 30.
        let want_stride = if vector.label == "move stack partial" {
            MOVE_PARTIAL_WIRE_BYTES
        } else if vector.label == "move container stack" {
            MOVE_CONTAINER_WIRE_BYTES
        } else {
            VIEW_ADDRESSED_WIRE_BYTES
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
fn client_stack_view_encode_into_leaves_a_short_destination_unchanged() {
    for vector in client_stack_view_vectors() {
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

#[test]
fn view_addressed_records_carry_the_exact_absent_sentinel() {
    // The inventory and crafting views address slots directly, so their
    // canonical record carries the exact all-zero reference and round-trips it
    // byte for byte: no partially zero record is accepted as absence.
    for vector in client_stack_view_vectors()
        .into_iter()
        .filter(|vector| matches!(vector.label, "quick move stack" | "drop stack"))
    {
        let record = (vector.build)();
        assert_eq!(
            record.container(),
            ContainerRef::NONE,
            "{}: the canonical record does not carry the absent sentinel",
            vector.label
        );
        let mut expected = vector.payload.clone();
        expected[8..26].copy_from_slice(&none_ref_bytes());
        assert_eq!(
            record.encode().expect("absent sentinel encodes"),
            expected,
            "{}: the absent sentinel is not published as eighteen zero bytes",
            vector.label
        );
        let decoded = decode_of(vector.label, &vector.payload).expect("decode");
        assert_eq!(
            decoded.container(),
            ContainerRef::NONE,
            "{}: the decoded record does not carry the absent sentinel",
            vector.label
        );
    }
}

/// The malformed real-reference matrix: every reference that is not a valid
/// furnace or chest identity, with the field that makes it invalid and the
/// error the gate reports for it.
///
/// Each entry carries exactly one violation, so the category the Go producer
/// records for the same bytes is unambiguous: a foreign dimension, a zero
/// generation and an out-of-range physical slot are `invalid-value` on both
/// sides, while an unknown kind is `invalid-enum` because both validators check
/// the kind before anything else.
fn malformed_reference_matrix() -> Vec<(&'static str, ContainerRef, ProtocolError)> {
    let mut matrix = Vec::new();
    for dimension in [256i32, -1] {
        matrix.push((
            "foreign dimension",
            ContainerRef {
                dimension,
                ..furnace_ref()
            },
            ProtocolError::InvalidRange,
        ));
    }
    matrix.push((
        "unknown kind",
        ContainerRef {
            kind: 2,
            ..furnace_ref()
        },
        ProtocolError::InvalidEnum,
    ));
    matrix.push((
        "zero generation",
        ContainerRef {
            generation: 0,
            ..chest_ref()
        },
        ProtocolError::InvalidRange,
    ));
    matrix.push((
        "furnace physical slot above the per-chunk array",
        ContainerRef {
            slot: 32,
            ..furnace_ref()
        },
        ProtocolError::InvalidRange,
    ));
    matrix.push((
        "chest physical slot above the per-chunk array",
        ContainerRef {
            slot: 16,
            ..chest_ref()
        },
        ProtocolError::InvalidRange,
    ));
    matrix
}

#[test]
fn client_stack_view_refuses_a_malformed_real_reference() {
    // Both regimes refuse: the container-view records through the shared view
    // gate, and `MoveContainerStack` through its own reference gate. The
    // reference is checked before any index rule, so the payload's index bytes
    // stay at their valid canonical values.
    for (label, reference, error) in malformed_reference_matrix() {
        let raw = reference_bytes(&reference);

        let container_payload = with_ref(&move_container_payload(), raw);
        assert_eq!(
            MoveContainerStack::decode(&container_payload),
            Err(error),
            "{label}: the container move decoder reports a different boundary"
        );
        // The record is also built through its public fields, which is the red
        // this node fixes: a mutated reference used to reach the wire
        // unvalidated because only `decode` checked it.
        let container = MoveContainerStack {
            sequence: 0,
            container: reference,
            from: 0,
            to: 37,
        };
        assert_eq!(
            container.validate(),
            Err(error),
            "{label}: the container move gate disagrees with the reference"
        );
        assert_eq!(
            container.encoded_len(),
            Err(error),
            "{label}: the container move sizes an invalid reference"
        );
        assert_eq!(
            container.encode(),
            Err(error),
            "{label}: the container move publishes an invalid reference"
        );
        assert!(
            MoveContainerStack::new(0, reference, 0, 37).is_err(),
            "{label}: the container move constructor admits an invalid reference"
        );

        let partial_payload = with_ref(&move_partial_payload(), raw);
        assert_eq!(
            MoveStackPartial::decode(&partial_payload),
            Err(error),
            "{label}: the partial move decoder reports a different boundary"
        );
        let partial = MoveStackPartial {
            sequence: 0,
            container: reference,
            view: mornlea_protocol::STACK_VIEW_CONTAINER,
            from: 36,
            to: 62,
            single: true,
        };
        assert_eq!(
            partial.validate(),
            Err(error),
            "{label}: the partial move gate disagrees with the reference"
        );
        assert_eq!(
            partial.encode(),
            Err(error),
            "{label}: the partial move publishes an invalid reference"
        );
    }

    // The exact absent sentinel is refused by the container view as well: a
    // zero generation beside a zero coordinate is absence, and absence is not a
    // real reference.
    let absent = with_ref(&move_partial_payload(), none_ref_bytes());
    assert_eq!(
        MoveStackPartial::decode(&absent),
        Err(ProtocolError::InvalidRange),
        "the container view refuses the absent sentinel"
    );
    assert_eq!(
        ContainerRef::NONE.to_domain_present(),
        Err(ProtocolError::InvalidRange)
    );
}

/// Renders one reference as its 18 wire bytes.
fn reference_bytes(reference: &ContainerRef) -> [u8; 18] {
    ref_bytes(
        reference.dimension,
        reference.chunk_x,
        reference.chunk_z,
        reference.kind,
        reference.slot,
        reference.generation,
    )
}

#[test]
fn client_stack_view_the_chest_index_boundary_is_pinned() {
    // The plan's named pair: chest view index 62 is the last legal unified slot
    // and 63 is refused, for the container move, the partial move, the quick
    // move and the drop alike.
    let last = MoveContainerStack::new(0, chest_ref(), 0, 62).expect("chest index 62 is legal");
    assert_eq!(
        last.encode().expect("chest index 62 encodes").len(),
        MOVE_CONTAINER_WIRE_BYTES
    );
    assert_eq!(
        MoveContainerStack::new(0, chest_ref(), 0, 63),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(
        MoveStackPartial::new(0, chest_ref(), 2, 36, 62, true).map(|_| ()),
        Ok(())
    );
    assert_eq!(
        MoveStackPartial::new(0, chest_ref(), 2, 36, 63, true).map(|_| ()),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(
        QuickMoveStack::new(0, chest_ref(), 2, 62).map(|_| ()),
        Ok(())
    );
    assert_eq!(
        QuickMoveStack::new(0, chest_ref(), 2, 63).map(|_| ()),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(DropStack::new(0, chest_ref(), 2, 62).map(|_| ()), Ok(()));
    assert_eq!(
        DropStack::new(0, chest_ref(), 2, 63).map(|_| ()),
        Err(ProtocolError::InvalidRange)
    );
    // The byte-level pin: the last chest slot stays inside the reviewed
    // payload, and the first illegal one is a decode refusal.
    let mut last_slot = move_partial_payload();
    last_slot[28] = 62;
    assert!(MoveStackPartial::decode(&last_slot).is_ok());
    let mut above = move_partial_payload();
    above[28] = 63;
    assert_eq!(
        MoveStackPartial::decode(&above),
        Err(ProtocolError::InvalidRange)
    );
}

#[test]
fn move_stack_partial_keeps_the_authority_only_rules_off_the_wire() {
    // Two crafting inventory-region indices are wire-valid: unlike
    // `MoveCraftingStack`, this family does not add the both-in-inventory
    // exclusion, because the Go validator leaves that to the authority.
    let both_inventory = MoveStackPartial::new(0, ContainerRef::NONE, 1, 9, 44, false)
        .expect("two inventory-region indices are wire-valid");
    let encoded = both_inventory.encode().expect("crafting pair encodes");
    assert_eq!(encoded.len(), MOVE_PARTIAL_WIRE_BYTES);
    assert_eq!(MoveStackPartial::decode(&encoded), Ok(both_inventory));

    // The furnace output slot is legal as a target here: the same restriction
    // `MoveContainerStack` publishes is not part of this family's static rule
    // set, so the authority owns it.
    let output_target = MoveStackPartial::new(0, furnace_ref(), 2, 36, 38, true)
        .expect("the furnace output slot is a wire-valid target");
    let encoded = output_target.encode().expect("output target encodes");
    assert_eq!(encoded.len(), MOVE_PARTIAL_WIRE_BYTES);
    assert_eq!(MoveStackPartial::decode(&encoded), Ok(output_target));

    // The same-slot relation is this family's own rule, so it is still refused.
    assert_eq!(
        MoveStackPartial::new(0, chest_ref(), 2, 36, 36, true).map(|_| ()),
        Err(ProtocolError::InvalidRange)
    );
}

/// One family's mutate-after-construction case: the canonical instance, the
/// invalid value a public mutable field is set to after construction, and the
/// error the surface has to report for it before any size or capacity decision.
struct ClientStackViewMutation {
    label: &'static str,
    build: fn() -> ClientStackViewRecord,
    mutate: fn(&mut ClientStackViewRecord),
    error: ProtocolError,
}

/// The families with an invalid mutable value. Each entry carries one
/// violation, so the error variant names the boundary that owns it.
fn client_stack_view_mutations() -> Vec<ClientStackViewMutation> {
    vec![
        ClientStackViewMutation {
            label: "move container target equals the furnace output slot",
            build: || {
                ClientStackViewRecord::MoveContainer(
                    MoveContainerStack::new(0, furnace_ref(), 0, 37).expect("move"),
                )
            },
            mutate: |record| {
                if let ClientStackViewRecord::MoveContainer(move_stack) = record {
                    move_stack.to = 38;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientStackViewMutation {
            label: "move container zero generation after construction",
            build: || {
                ClientStackViewRecord::MoveContainer(
                    MoveContainerStack::new(0, furnace_ref(), 0, 37).expect("move"),
                )
            },
            mutate: |record| {
                if let ClientStackViewRecord::MoveContainer(move_stack) = record {
                    move_stack.container.generation = 0;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientStackViewMutation {
            label: "move container unknown kind after construction",
            build: || {
                ClientStackViewRecord::MoveContainer(
                    MoveContainerStack::new(0, furnace_ref(), 0, 37).expect("move"),
                )
            },
            mutate: |record| {
                if let ClientStackViewRecord::MoveContainer(move_stack) = record {
                    move_stack.container.kind = 2;
                }
            },
            error: ProtocolError::InvalidEnum,
        },
        ClientStackViewMutation {
            label: "move partial source equals target",
            build: || {
                ClientStackViewRecord::MovePartial(
                    MoveStackPartial::new(0, chest_ref(), 2, 36, 62, true).expect("partial"),
                )
            },
            mutate: |record| {
                if let ClientStackViewRecord::MovePartial(partial) = record {
                    partial.to = 36;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientStackViewMutation {
            label: "move partial real reference in the inventory view",
            build: || {
                ClientStackViewRecord::MovePartial(
                    MoveStackPartial::new(0, chest_ref(), 2, 36, 62, true).expect("partial"),
                )
            },
            mutate: |record| {
                if let ClientStackViewRecord::MovePartial(partial) = record {
                    partial.view = 0;
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientStackViewMutation {
            label: "quick move real reference in the crafting view",
            build: || {
                ClientStackViewRecord::QuickMove(
                    QuickMoveStack::new(0, ContainerRef::NONE, 1, 44).expect("quick"),
                )
            },
            mutate: |record| {
                if let ClientStackViewRecord::QuickMove(quick) = record {
                    quick.container = chest_ref();
                }
            },
            error: ProtocolError::InvalidRange,
        },
        ClientStackViewMutation {
            label: "drop stack real reference in the inventory view",
            build: || {
                ClientStackViewRecord::DropStack(
                    DropStack::new(0, ContainerRef::NONE, 0, 35).expect("drop"),
                )
            },
            mutate: |record| {
                if let ClientStackViewRecord::DropStack(drop) = record {
                    drop.container = chest_ref();
                }
            },
            error: ProtocolError::InvalidRange,
        },
    ]
}

#[test]
fn client_stack_view_invalid_value_wins_over_short_capacity() {
    for mutation in client_stack_view_mutations() {
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

#[test]
fn client_stack_view_decode_rejects_every_proper_truncation() {
    for vector in client_stack_view_vectors() {
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
fn client_stack_view_decode_rejects_one_trailing_byte() {
    for vector in client_stack_view_vectors() {
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

#[test]
fn client_stack_view_decode_rejects_the_pinned_invalid_values() {
    // The matrix mirrors the corpus negatives: the furnace output target, the
    // same-slot pair and the index above the chest bound of the container move,
    // the four single-violation reference mutations of that same payload, and
    // the unknown view, the single-tag byte, the same-slot pair, the reference
    // and index boundaries of the three view-addressed families. Every entry is
    // a single violation.
    let cases: Vec<(&str, Vec<u8>)> = vec![
        (
            "move container stack",
            with_byte(&move_container_payload(), 27, 0x26),
        ),
        (
            "move container stack",
            with_byte(&move_container_payload(), 27, 0x00),
        ),
        (
            "move container stack",
            with_byte(&move_container_payload(), 27, 0x3f),
        ),
        (
            "move container stack",
            with_ref(&move_container_payload(), ref_bytes(0, -1, 2, 2, 0, 1)),
        ),
        (
            "move container stack",
            with_ref(&move_container_payload(), ref_bytes(-1, -1, 2, 0, 31, 1)),
        ),
        (
            "move container stack",
            with_ref(&move_container_payload(), ref_bytes(0, -1, 2, 0, 31, 0)),
        ),
        (
            "move container stack",
            with_ref(&move_container_payload(), ref_bytes(0, -1, 2, 0, 32, 1)),
        ),
        (
            "move stack partial",
            with_byte(&move_partial_payload(), 26, 0x03),
        ),
        (
            "move stack partial",
            with_byte(&move_partial_payload(), 29, 0x02),
        ),
        (
            "move stack partial",
            with_byte(&move_partial_payload(), 28, 0x24),
        ),
        (
            "move stack partial",
            with_ref(&move_partial_payload(), ref_bytes(0, -1, 2, 0, 31, 1)),
        ),
        (
            "quick move stack",
            with_byte(&quick_move_payload(), 26, 0x03),
        ),
        (
            "quick move stack",
            with_byte(&quick_move_payload(), 27, 0x2d),
        ),
        (
            "quick move stack",
            with_ref(&quick_move_payload(), chest_ref_bytes()),
        ),
        ("drop stack", with_byte(&drop_stack_payload(), 26, 0x03)),
        ("drop stack", with_byte(&drop_stack_payload(), 27, 0x24)),
        (
            "drop stack",
            with_ref(&drop_stack_payload(), chest_ref_bytes()),
        ),
    ];
    for (label, payload) in cases {
        let error = decode_of(label, &payload)
            .expect_err("the pinned payload must be refused by the decoder");
        assert!(
            matches!(
                error,
                ProtocolError::InvalidEnum | ProtocolError::InvalidRange
            ),
            "{label}: the pinned payload reports {error:?}"
        );
    }
}

#[test]
fn client_stack_view_decode_refuses_the_unknown_view() {
    // A view above the closed {0,1,2} set is an unknown enum, never a range
    // refusal, on all three view-addressed families.
    let cases: Vec<(&str, Vec<u8>)> = vec![
        (
            "move stack partial",
            with_byte(&move_partial_payload(), 26, 0x03),
        ),
        (
            "quick move stack",
            with_byte(&quick_move_payload(), 26, 0x03),
        ),
        ("drop stack", with_byte(&drop_stack_payload(), 26, 0x03)),
    ];
    for (label, payload) in cases {
        let error = decode_of(label, &payload).expect_err("an unknown view must never decode");
        assert_eq!(
            error,
            ProtocolError::InvalidEnum,
            "{label}: an unknown view reports {error:?}"
        );
    }
}

#[test]
fn client_stack_view_packet_ids_are_pinned() {
    assert_eq!(MoveContainerStack::PACKET_ID, 9);
    assert_eq!(MoveStackPartial::PACKET_ID, 19);
    assert_eq!(QuickMoveStack::PACKET_ID, 20);
    assert_eq!(DropStack::PACKET_ID, 21);
}

#[test]
fn client_stack_view_payload_purity_is_pinned() {
    // Every payload is exactly the sequence, the reference and the index bytes
    // its family names: the moved count, the transfer destination and the world
    // drop position are all server-owned decisions the wire never carries, so a
    // payload that grew any of them would break the protocol rather than the
    // encoder.
    for vector in client_stack_view_vectors() {
        assert_eq!(
            &vector.payload[..8],
            &[0u8; 8],
            "{}: the canonical sequence is zero and leads the payload",
            vector.label
        );
        let want = match vector.label {
            "move container stack" => MOVE_CONTAINER_WIRE_BYTES,
            "move stack partial" => MOVE_PARTIAL_WIRE_BYTES,
            _ => VIEW_ADDRESSED_WIRE_BYTES,
        };
        assert_eq!(
            vector.payload.len(),
            want,
            "{}: the payload carries exactly the sequence, the reference and the index bytes",
            vector.label
        );
    }
}
