//! Structural negotiation and pure admission for the inbound handshake and
//! login path.
//!
//! This suite is the Rust half of the node's paired evidence: the Go login
//! driver observes the same named cases in
//! `packages/shared/network/protocol_admission_oracle_test.go` through the real
//! `BeginServerLogin` decision. The case identities below are the same strings
//! the Go table carries, so a reviewer can match them one to one.
//!
//! The split under test is the architectural contract: `decode_inbound`
//! applies only the structural rules and keeps the peer's raw fields, and
//! `validate_hello` / `admit_login` own every policy decision. A decoder that
//! refused a semantically invalid record would destroy the record the peer has
//! to be answered about, so every rejection here names the function that owns
//! it.

use mornlea_protocol::{
    AdmittedLogin, ClientHello, HandshakeRejection, InboundLoginStart, LoginAdmissionError,
    LoginStart, MAX_SMALL_PAYLOAD_BYTES, ProtocolError, admit_login, validate_hello,
};

/// `hello-version-44`: a structurally valid hello naming the previous protocol
/// version, which admission answers with the negotiated version pair.
const CASE_HELLO_VERSION_44: &str = "hello-version-44";
/// `hello-trailing-byte`: a hello followed by one excess byte.
const CASE_HELLO_TRAILING_BYTE: &str = "hello-trailing-byte";
/// `login-zero-id-distance-one`: an all-zero identity together with an
/// out-of-domain distance, which must report the identity rule.
const CASE_LOGIN_ZERO_ID_DISTANCE_ONE: &str = "login-zero-id-distance-one";
/// `login-distance-one`: a valid identity and name with the distance one below
/// the published interval.
const CASE_LOGIN_DISTANCE_ONE: &str = "login-distance-one";
/// `login-distance-65`: the same record one above the published interval.
const CASE_LOGIN_DISTANCE_65: &str = "login-distance-65";
/// `login-name-trimmed`: a name the pinned whitespace set trims into a
/// canonical name, whose declared distance must survive admission unchanged.
const CASE_LOGIN_NAME_TRIMMED: &str = "login-name-trimmed";
/// `login-long-raw-name`: 128 leading spaces plus `Alice`, admitted by the Go
/// driver because the canonical byte bound applies after the trim.
const CASE_LOGIN_LONG_RAW_NAME: &str = "login-long-raw-name";
/// `login-33-scalars`: a name of 33 scalars after trim.
const CASE_LOGIN_33_SCALARS: &str = "login-33-scalars";
/// `login-control-character`: a name carrying the control scalar U+0001.
const CASE_LOGIN_CONTROL_CHARACTER: &str = "login-control-character";
/// `login-missing-distance`: a login start whose trailing distance byte is
/// absent.
const CASE_LOGIN_MISSING_DISTANCE: &str = "login-missing-distance";
/// `login-trailing-byte`: a login start followed by one excess byte.
const CASE_LOGIN_TRAILING_BYTE: &str = "login-trailing-byte";
/// `login-invalid-utf8-name`: a name field that is not valid UTF-8.
const CASE_LOGIN_INVALID_UTF8_NAME: &str = "login-invalid-utf8-name";

/// A well-formed UUIDv4: version nibble 4, variant bits 10.
fn valid_uuidv4() -> [u8; 16] {
    let mut bytes = [0u8; 16];
    for (index, slot) in bytes.iter_mut().enumerate() {
        *slot = index as u8;
    }
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    bytes
}

/// Builds the LoginStart payload the Go inbound decoder reads: 16 identity
/// bytes, a canonical length-prefixed name, and the trailing distance byte.
fn login_payload(player_id: [u8; 16], name: &[u8], view_distance: u8) -> Vec<u8> {
    let mut payload = player_id.to_vec();
    payload.extend(mornlea_protocol::encode_uvarint(
        u32::try_from(name.len()).expect("name length fits u32"),
    ));
    payload.extend_from_slice(name);
    payload.push(view_distance);
    payload
}

/// The hello payload one protocol version encodes to.
fn hello_payload(version: u32) -> Vec<u8> {
    mornlea_protocol::encode_uvarint(version)
}

/// Decodes one login start structurally and admits it, so a case exercises the
/// same two halves the Go driver exercises.
fn admit(payload: &[u8]) -> Result<AdmittedLogin, LoginAdmissionError> {
    let inbound = LoginStart::decode_inbound(payload).expect("structurally valid login start");
    admit_login(inbound)
}

/// A version-44 hello survives structural decoding so admission can answer
/// with the negotiated version pair. The strict outbound decoder already
/// applies the version policy itself, so it could never carry this record.
#[test]
fn hello_version_44_is_a_version_mismatch_not_a_decode_failure() {
    let inbound = ClientHello::decode_inbound(&hello_payload(44))
        .expect("a version-44 hello decodes structurally");
    assert_eq!(inbound.protocol_version(), 44);

    let rejection = validate_hello(inbound).expect_err("version 44 is not negotiable");
    assert_eq!(
        rejection,
        HandshakeRejection::VersionMismatch { server_version: 45 },
        "{CASE_HELLO_VERSION_44} must report the negotiated version pair"
    );
}

#[test]
fn hello_current_version_is_negotiated() {
    let inbound = ClientHello::decode_inbound(&hello_payload(45))
        .expect("the current hello decodes structurally");
    validate_hello(inbound).expect("the current version negotiates");
}

#[test]
fn hello_trailing_byte_is_a_structural_rejection() {
    let mut payload = hello_payload(45);
    payload.push(0);
    assert_eq!(
        ClientHello::decode_inbound(&payload),
        Err(ProtocolError::TrailingBytes),
        "{CASE_HELLO_TRAILING_BYTE} rejects at the owning structural boundary"
    );
}

/// The identity rule is decided before the view-distance rule, which is what
/// the Go driver's `LoginInvalidIdentity` answer encodes.
#[test]
fn login_identity_is_decided_before_view_distance() {
    let payload = login_payload([0u8; 16], b"Alice", 1);
    assert_eq!(
        admit(&payload),
        Err(LoginAdmissionError::InvalidIdentity),
        "{CASE_LOGIN_ZERO_ID_DISTANCE_ONE} must report the identity rule"
    );
}

#[test]
fn login_view_distance_boundaries_are_protocol_violations() {
    for (case, distance) in [(CASE_LOGIN_DISTANCE_ONE, 1u8), (CASE_LOGIN_DISTANCE_65, 65)] {
        let payload = login_payload(valid_uuidv4(), b"Alice", distance);
        assert_eq!(
            admit(&payload),
            Err(LoginAdmissionError::ProtocolViolation),
            "{case} must report the distance rule"
        );
    }
}

#[test]
fn login_view_distance_interval_endpoints_are_admitted_unchanged() {
    for distance in [2u8, 64] {
        let payload = login_payload(valid_uuidv4(), b"Alice", distance);
        let admitted = admit(&payload).expect("a distance inside the interval is admitted");
        assert_eq!(
            admitted.view_distance(),
            distance,
            "admission must not clamp or rewrite the declared distance"
        );
    }
}

#[test]
fn login_name_is_trimmed_by_the_pinned_set_and_keeps_the_distance() {
    let payload = login_payload(valid_uuidv4(), b"  Alice  ", 2);
    let admitted = admit(&payload).expect("surrounding whitespace trims away");
    assert_eq!(
        admitted.display_name().as_str(),
        "Alice",
        "{CASE_LOGIN_NAME_TRIMMED} must admit the canonical trimmed name"
    );
    assert_eq!(admitted.view_distance(), 2);
}

/// The canonical byte bound applies after the pinned trim, so a raw name of 128
/// leading spaces plus five scalars is admitted with the trimmed result. The
/// payload stays far inside the 64 KiB small-payload ceiling, which is the
/// size bound the structural decoder does enforce.
#[test]
fn login_long_raw_name_trims_into_a_canonical_name() {
    let raw = format!("{}Alice", " ".repeat(128));
    let payload = login_payload(valid_uuidv4(), raw.as_bytes(), 2);
    assert!(
        payload.len() <= MAX_SMALL_PAYLOAD_BYTES,
        "the hand-built payload must stay inside the small-payload ceiling"
    );
    let admitted = admit(&payload).expect("the trimmed name is canonical");
    assert_eq!(
        admitted.display_name().as_str(),
        "Alice",
        "{CASE_LOGIN_LONG_RAW_NAME} must admit the trimmed name"
    );
}

#[test]
fn login_name_scalar_count_is_decided_after_trim() {
    let payload = login_payload(valid_uuidv4(), b"a".repeat(33).as_slice(), 2);
    assert_eq!(
        admit(&payload),
        Err(LoginAdmissionError::InvalidIdentity),
        "{CASE_LOGIN_33_SCALARS} must report the canonical name rule"
    );
}

#[test]
fn login_control_character_is_rejected_by_the_name_rule() {
    let payload = login_payload(valid_uuidv4(), b"Ali\x01ce", 2);
    assert_eq!(
        admit(&payload),
        Err(LoginAdmissionError::InvalidIdentity),
        "{CASE_LOGIN_CONTROL_CHARACTER} must report the canonical name rule"
    );
}

#[test]
fn login_missing_distance_byte_is_a_structural_rejection() {
    let mut payload = login_payload(valid_uuidv4(), b"Alice", 2);
    payload.pop();
    assert_eq!(
        LoginStart::decode_inbound(&payload),
        Err(ProtocolError::Truncated),
        "{CASE_LOGIN_MISSING_DISTANCE} rejects at the owning structural boundary"
    );
}

#[test]
fn login_trailing_byte_is_a_structural_rejection() {
    let mut payload = login_payload(valid_uuidv4(), b"Alice", 2);
    payload.push(0);
    assert_eq!(
        LoginStart::decode_inbound(&payload),
        Err(ProtocolError::TrailingBytes),
        "{CASE_LOGIN_TRAILING_BYTE} rejects at the owning structural boundary"
    );
}

#[test]
fn login_invalid_utf8_name_is_a_structural_rejection() {
    let payload = login_payload(valid_uuidv4(), b"Al\xffice", 2);
    assert_eq!(
        LoginStart::decode_inbound(&payload),
        Err(ProtocolError::InvalidString),
        "{CASE_LOGIN_INVALID_UTF8_NAME} rejects at the owning structural boundary"
    );
}

/// An oversized payload is refused before any field is read, matching the Go
/// decoder's small-payload ceiling check.
#[test]
fn login_payload_over_the_small_ceiling_is_a_size_refusal_before_any_field() {
    let mut payload = login_payload(valid_uuidv4(), b"Alice", 2);
    payload.extend(std::iter::repeat_n(
        0u8,
        MAX_SMALL_PAYLOAD_BYTES + 1 - payload.len(),
    ));
    assert_eq!(
        LoginStart::decode_inbound(&payload),
        Err(ProtocolError::Allocation),
        "an oversized payload is a size refusal, not a field failure"
    );
}

/// The inbound form is a plain owned value: it can be moved into admission
/// without borrowing the payload it was decoded from.
#[test]
fn inbound_login_start_keeps_the_raw_fields_admission_reads() {
    let payload = login_payload(valid_uuidv4(), b"  Alice  ", 2);
    let inbound = LoginStart::decode_inbound(&payload).expect("structurally valid login start");
    assert_eq!(inbound.player_id(), &valid_uuidv4());
    assert_eq!(inbound.display_name(), "  Alice  ");
    assert_eq!(inbound.view_distance(), 2);

    let moved: InboundLoginStart = inbound;
    assert_eq!(
        admit_login(moved)
            .expect("admitted")
            .display_name()
            .as_str(),
        "Alice"
    );
}

/// The strict decode stays the outbound record's convenience path and is never
/// the inbound path: it keeps applying the version and canonical-name rules
/// that admission now owns for inbound records.
#[test]
fn strict_outbound_records_still_apply_their_own_policy() {
    assert_eq!(
        ClientHello::decode(&hello_payload(44)),
        Err(ProtocolError::UnsupportedVersion)
    );
    let raw = format!("{}Alice", " ".repeat(128));
    assert_eq!(
        LoginStart::decode(&login_payload(valid_uuidv4(), raw.as_bytes(), 2)),
        Err(ProtocolError::InvalidString)
    );
}

#[test]
fn mutated_outbound_hello_refuses_publication_without_touching_destination() {
    let mut hello = ClientHello::new(45).expect("current protocol");
    hello.protocol_version = 44;
    let mut destination = [0xa5; 4];
    assert_eq!(hello.validate(), Err(ProtocolError::UnsupportedVersion));
    assert_eq!(hello.encoded_len(), Err(ProtocolError::UnsupportedVersion));
    assert_eq!(
        hello.encode_into(&mut destination),
        Err(ProtocolError::UnsupportedVersion)
    );
    assert_eq!(destination, [0xa5; 4]);
    assert_eq!(hello.encode(), Err(ProtocolError::UnsupportedVersion));
}

#[test]
fn mutated_outbound_login_checks_name_then_distance_before_capacity() {
    let id = mornlea_protocol::PlayerId::try_from_bytes(valid_uuidv4()).expect("uuid v4");
    let mut login = LoginStart::new(id, "Alice", 8).expect("valid login");
    login.display_name = "a".repeat(129);
    login.view_distance = 1;
    let mut destination = [0xa5; 32];
    assert_eq!(login.validate(), Err(ProtocolError::InvalidString));
    assert_eq!(login.encoded_len(), Err(ProtocolError::InvalidString));
    assert_eq!(
        login.encode_into(&mut destination[..0]),
        Err(ProtocolError::InvalidString)
    );
    assert_eq!(destination, [0xa5; 32]);
    assert_eq!(login.encode(), Err(ProtocolError::InvalidString));

    login.display_name = "Alice".to_string();
    assert_eq!(login.encode(), Err(ProtocolError::InvalidRange));
    assert_eq!(
        login.encode_into(&mut destination),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(destination, [0xa5; 32]);

    login.view_distance = 8;
    let needed = login.encoded_len().expect("valid encoded length");
    assert_eq!(needed, 16 + 1 + 5 + 1);
    assert_eq!(
        login.encode_into(&mut destination[..needed - 1]),
        Err(ProtocolError::OutputTooSmall {
            needed,
            available: needed - 1,
        })
    );
    assert_eq!(destination, [0xa5; 32]);
}
