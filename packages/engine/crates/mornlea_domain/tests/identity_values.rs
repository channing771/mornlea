//! Identity, text, and scalar value contracts for `mornlea_domain`, pinned
//! against the verified Go baseline.
//!
//! The Go oracles are `core.PlayerID.Valid` and `core.NormalizeDisplayName`
//! for player identities and display names, `companion.ValidateName` for
//! companion names, and the network chat text validators for the bounded
//! command and speech slots.

use mornlea_domain::{
    CommandText, CompanionId, CompanionName, Dimension, DisplayName, DomainError, FiniteVec3,
    HostileId, HotbarSlot, LookAngles, PassiveId, PlayerId, ProjectileId, SpeechText,
};

/// Canonical UUIDv4 text `00112233-4455-4677-8899-aabbccddeeff` in wire order.
const VALID_UUID: [u8; 16] = [
    0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,
];

/// Same UUID with a non-v4 version nibble (`...-3677-...`).
const WRONG_VERSION_UUID: [u8; 16] = [
    0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x36, 0x77, 0x88, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,
];

/// Same UUID with a non-RFC-4122 variant nibble (`...-0099-...`).
const WRONG_VARIANT_UUID: [u8; 16] = [
    0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x46, 0x77, 0x00, 0x99, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF,
];

/// Sealed helper proving a player identity is its own type: implemented for
/// `PlayerId` only, so a companion identity cannot reach the player-only
/// helper without a compile error.
trait PlayerIdentity {
    fn player_bytes(self) -> [u8; 16];
}

impl PlayerIdentity for PlayerId {
    fn player_bytes(self) -> [u8; 16] {
        self.bytes()
    }
}

/// Sealed helper proving a companion identity is its own type.
trait CompanionIdentity {
    fn companion_bytes(self) -> [u8; 16];
}

impl CompanionIdentity for CompanionId {
    fn companion_bytes(self) -> [u8; 16] {
        self.bytes()
    }
}

#[test]
fn identity_values_player_id_rejects_zero_wrong_version_and_wrong_variant() {
    assert_eq!(
        PlayerId::try_from_bytes([0; 16]),
        Err(DomainError::InvalidIdentity)
    );
    assert_eq!(
        PlayerId::try_from_bytes(WRONG_VERSION_UUID),
        Err(DomainError::InvalidIdentity)
    );
    assert_eq!(
        PlayerId::try_from_bytes(WRONG_VARIANT_UUID),
        Err(DomainError::InvalidIdentity)
    );

    let player = PlayerId::try_from_bytes(VALID_UUID).expect("valid player identity");
    assert_eq!(player.bytes(), VALID_UUID);
}

#[test]
fn identity_values_companion_id_shares_the_uuid_rule() {
    assert_eq!(
        CompanionId::try_from_bytes([0; 16]),
        Err(DomainError::InvalidIdentity)
    );
    assert_eq!(
        CompanionId::try_from_bytes(WRONG_VERSION_UUID),
        Err(DomainError::InvalidIdentity)
    );
    assert_eq!(
        CompanionId::try_from_bytes(WRONG_VARIANT_UUID),
        Err(DomainError::InvalidIdentity)
    );

    let companion = CompanionId::try_from_bytes(VALID_UUID).expect("valid companion identity");
    assert_eq!(companion.bytes(), VALID_UUID);
}

#[test]
fn identity_values_player_and_companion_ids_are_distinct_types() {
    // Each identity has its own inherent byte accessor, and the sealed helper
    // traits above are implemented for exactly one newtype each, so passing a
    // companion identity to the player-only helper fails to compile. That is
    // the compile-time distinction this case pins.
    fn player_bytes<T: PlayerIdentity>(id: T) -> [u8; 16] {
        id.player_bytes()
    }

    fn companion_bytes<T: CompanionIdentity>(id: T) -> [u8; 16] {
        id.companion_bytes()
    }

    let player = PlayerId::try_from_bytes(VALID_UUID).expect("valid player identity");
    let companion = CompanionId::try_from_bytes(VALID_UUID).expect("valid companion identity");
    assert_eq!(player_bytes(player), VALID_UUID);
    assert_eq!(companion_bytes(companion), VALID_UUID);
    assert_eq!(player_bytes(player), companion_bytes(companion));
}

#[test]
fn identity_values_scalar_entity_ids_reject_zero() {
    assert_eq!(HostileId::try_new(0), Err(DomainError::InvalidIdentity));
    assert_eq!(PassiveId::try_new(0), Err(DomainError::InvalidIdentity));
    assert_eq!(ProjectileId::try_new(0), Err(DomainError::InvalidIdentity));

    assert_eq!(HostileId::try_new(7).expect("hostile id").get(), 7);
    assert_eq!(PassiveId::try_new(8).expect("passive id").get(), 8);
    assert_eq!(ProjectileId::try_new(9).expect("projectile id").get(), 9);
}

#[test]
fn identity_values_dimension_accepts_only_overworld_and_depths() {
    assert_eq!(Dimension::new(0), Ok(Dimension::OVERWORLD));
    assert_eq!(Dimension::new(1), Ok(Dimension::DEPTHS));
    assert_eq!(Dimension::new(2), Err(DomainError::InvalidDimension));
    assert_eq!(Dimension::OVERWORLD.get(), 0);
    assert_eq!(Dimension::DEPTHS.get(), 1);
}

#[test]
fn identity_values_hotbar_slot_closed_range_is_zero_through_eight() {
    assert!(HotbarSlot::new(8).is_ok());
    assert_eq!(HotbarSlot::new(8).expect("slot eight").get(), 8);
    assert_eq!(
        HotbarSlot::new(9),
        Err(DomainError::InvalidHotbarSlot),
        "slot nine is outside the closed range"
    );
}

#[test]
fn identity_values_finite_vec3_rejects_non_finite_components() {
    assert_eq!(
        FiniteVec3::try_new([1.0, f32::NAN, 0.0]),
        Err(DomainError::NonFiniteValue)
    );
    assert_eq!(
        FiniteVec3::try_new([f32::INFINITY, 0.0, 0.0]),
        Err(DomainError::NonFiniteValue)
    );
    assert_eq!(
        FiniteVec3::try_new([0.0, 0.0, f32::NEG_INFINITY]),
        Err(DomainError::NonFiniteValue)
    );

    let vector = FiniteVec3::try_new([1.0, -2.0, 0.5]).expect("finite vector");
    assert_eq!(vector.get(), [1.0, -2.0, 0.5]);
}

#[test]
fn identity_values_look_angles_reject_non_finite_rotation() {
    assert_eq!(
        LookAngles::try_new(f32::NAN, 0.0),
        Err(DomainError::NonFiniteRotation)
    );
    assert_eq!(
        LookAngles::try_new(0.0, f32::INFINITY),
        Err(DomainError::NonFiniteRotation)
    );
    assert_eq!(
        LookAngles::try_new(0.0, f32::NEG_INFINITY),
        Err(DomainError::NonFiniteRotation)
    );
}

#[test]
fn identity_values_look_angles_preserve_negative_zero_bits() {
    let angles = LookAngles::try_new(-0.0, -0.0).expect("finite negative zero angles");
    assert_eq!(angles.yaw().to_bits(), (-0.0f32).to_bits());
    assert_eq!(angles.pitch().to_bits(), (-0.0f32).to_bits());

    let positive = LookAngles::try_new(0.0, 0.0).expect("finite positive zero angles");
    assert_eq!(positive.yaw().to_bits(), 0);
    assert_ne!(angles.yaw().to_bits(), positive.yaw().to_bits());

    let vector = FiniteVec3::try_new([-0.0, 0.0, -0.0]).expect("finite vector with negative zero");
    assert_eq!(vector.get()[0].to_bits(), (-0.0f32).to_bits());
    assert_eq!(vector.get()[1].to_bits(), 0);
    assert_eq!(vector.get()[2].to_bits(), (-0.0f32).to_bits());
}

#[test]
fn identity_values_display_name_matches_the_go_canonical_rule() {
    assert_eq!(
        DisplayName::try_from_canonical("Alice".to_string())
            .expect("canonical display name")
            .as_str(),
        "Alice"
    );
    assert_eq!(
        DisplayName::try_from_canonical("Alice B".to_string())
            .expect("interior space stays legal")
            .as_str(),
        "Alice B"
    );
    assert_eq!(
        DisplayName::try_from_canonical("Ali\u{200B}ce".to_string())
            .expect("zero width space is neither whitespace nor control")
            .as_str(),
        "Ali\u{200B}ce"
    );

    assert!(DisplayName::try_from_canonical("a".repeat(32)).is_ok());
    assert_eq!(
        DisplayName::try_from_canonical("a".repeat(33)),
        Err(DomainError::InvalidText)
    );
    assert!(DisplayName::try_from_canonical("\u{1F600}".repeat(32)).is_ok());
    assert_eq!(
        DisplayName::try_from_canonical("\u{1F600}".repeat(32) + "a"),
        Err(DomainError::InvalidText)
    );

    assert_eq!(
        DisplayName::try_from_canonical(String::new()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        DisplayName::try_from_canonical(" Alice".to_string()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        DisplayName::try_from_canonical("Alice ".to_string()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        DisplayName::try_from_canonical("Alice\u{0085}".to_string()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        DisplayName::try_from_canonical("Ali\u{007F}ce".to_string()),
        Err(DomainError::InvalidText)
    );
}

#[test]
fn identity_values_companion_name_rejects_embedded_whitespace() {
    assert_eq!(
        CompanionName::try_from_canonical("Buddy".to_string())
            .expect("canonical companion name")
            .as_str(),
        "Buddy"
    );

    assert_eq!(
        CompanionName::try_from_canonical("A B".to_string()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        CompanionName::try_from_canonical(" Buddy".to_string()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        CompanionName::try_from_canonical("Buddy\u{00A0}".to_string()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        CompanionName::try_from_canonical(String::new()),
        Err(DomainError::InvalidText)
    );
}

#[test]
fn identity_values_command_text_bounds_at_1024_bytes() {
    assert_eq!(
        CommandText::try_from_canonical("x".repeat(1024))
            .expect("command at the byte bound")
            .as_str()
            .len(),
        1024
    );
    assert_eq!(
        CommandText::try_from_canonical("x".repeat(1025)),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        CommandText::try_from_canonical(String::new()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        CommandText::try_from_canonical(" mine ".to_string()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        CommandText::try_from_canonical("mi\u{0001}ne".to_string()),
        Err(DomainError::InvalidText)
    );
}

#[test]
fn identity_values_speech_text_bounds_at_256_bytes() {
    assert_eq!(
        SpeechText::try_from_canonical("y".repeat(256))
            .expect("speech at the byte bound")
            .as_str()
            .len(),
        256
    );
    assert_eq!(
        SpeechText::try_from_canonical("y".repeat(257)),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        SpeechText::try_from_canonical(String::new()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        SpeechText::try_from_canonical(" hello ".to_string()),
        Err(DomainError::InvalidText)
    );
    assert_eq!(
        SpeechText::try_from_canonical("hi\u{009F}there".to_string()),
        Err(DomainError::InvalidText)
    );
}
