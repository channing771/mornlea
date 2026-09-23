//! The shared-value boundary between the raw v45 wire representation and the
//! checked semantic values `mornlea_domain` owns.
//!
//! The protocol crate keeps the exact wire form wherever the wire is
//! observable, and consumes the domain's checked newtypes wherever a shared
//! meaning exists. The cases in this file pin that boundary: a foreign raw
//! container dimension survives decoding instead of being narrowed into a
//! valid `u8`, the all-zero container reference is the one absent sentinel,
//! the item rules have a single owner, a compact section moves between the
//! two representations without expanding cells or reordering a palette, and
//! the pinned whitespace set is the only trim rule.

use mornlea_domain::{ChunkPos, ContainerKind, PalettedSection};
use mornlea_protocol::{
    ContainerRef, MoveStackPartial, ProtocolError, STACK_VIEW_CONTAINER, SectionData,
    SectionStorage,
};

/// Wire bytes of one `MoveStackPartial` in the container view whose raw
/// dimension is `dimension`.
fn container_view_payload(dimension: i32) -> Vec<u8> {
    let mut payload = Vec::new();
    payload.extend_from_slice(&7u64.to_le_bytes());
    payload.extend_from_slice(&dimension.to_le_bytes());
    payload.extend_from_slice(&1i32.to_le_bytes());
    payload.extend_from_slice(&2i32.to_le_bytes());
    payload.push(mornlea_protocol::CONTAINER_KIND_CHEST);
    payload.push(5);
    payload.extend_from_slice(&9u32.to_le_bytes());
    payload.push(STACK_VIEW_CONTAINER);
    payload.push(0);
    payload.push(1);
    payload.push(0);
    payload
}

/// A raw dimension outside the `u8` range must survive decoding exactly: the
/// wire value is `i32` and narrowing it would alias a foreign dimension into
/// a valid one, which is the raw-format loss this node removes.
#[test]
fn container_view_decode_preserves_a_foreign_raw_dimension() {
    for dimension in [256i32, -1] {
        let decoded = MoveStackPartial::decode(&container_view_payload(dimension))
            .expect("a raw foreign dimension is publishable wire data");
        let raw: i64 = i64::from(decoded.container.dimension);
        assert_eq!(raw, i64::from(dimension));
    }
}

/// The exact all-zero record is the one absent container reference. A zero
/// generation beside a nonzero coordinate is a broken real reference, not an
/// absent one, so it is rejected rather than mapped to `None`.
#[test]
fn container_reference_absence_is_exactly_the_zero_record() {
    assert_eq!(ContainerRef::NONE.to_domain_optional(), Ok(None));
    assert!(ContainerRef::NONE.to_domain_present().is_err());

    let zero_generation = ContainerRef {
        dimension: 0,
        chunk_x: 3,
        chunk_z: 0,
        kind: 0,
        slot: 0,
        generation: 0,
    };
    assert_eq!(
        zero_generation.to_domain_optional(),
        Err(ProtocolError::InvalidRange)
    );
    assert_eq!(
        zero_generation.to_domain_present(),
        Err(ProtocolError::InvalidRange)
    );
}

/// A present reference converts through the domain constructor: the dimension
/// must be the overworld, the kind must be known, and the slot and generation
/// must fit the addressed array. A foreign dimension never aliases a valid
/// `u8` value.
#[test]
fn container_reference_present_conversion_rejects_foreign_dimensions() {
    for dimension in [256i32, -1] {
        let foreign = ContainerRef {
            dimension,
            chunk_x: 3,
            chunk_z: 4,
            kind: mornlea_protocol::CONTAINER_KIND_FURNACE,
            slot: 1,
            generation: 2,
        };
        assert_eq!(
            foreign.to_domain_present(),
            Err(ProtocolError::InvalidRange)
        );
        assert_eq!(
            foreign.to_domain_optional(),
            Err(ProtocolError::InvalidRange)
        );
    }
    let unknown_kind = ContainerRef {
        dimension: 0,
        chunk_x: 3,
        chunk_z: 4,
        kind: 2,
        slot: 1,
        generation: 2,
    };
    assert_eq!(
        unknown_kind.to_domain_present(),
        Err(ProtocolError::InvalidEnum)
    );
    let furnace = ContainerRef {
        dimension: 0,
        chunk_x: -3,
        chunk_z: 7,
        kind: mornlea_protocol::CONTAINER_KIND_FURNACE,
        slot: 5,
        generation: 9,
    };
    let domain = furnace.to_domain_present().expect("furnace reference");
    assert_eq!(domain.chunk(), ChunkPos::new(-3, 7));
    assert_eq!(domain.kind(), ContainerKind::Furnace);
    assert_eq!(domain.slot(), 5);
    assert_eq!(domain.generation(), 9);
    assert_eq!(furnace.to_domain_optional(), Ok(Some(domain)));
    let chest = ContainerRef {
        dimension: 0,
        chunk_x: 1,
        chunk_z: -2,
        kind: mornlea_protocol::CONTAINER_KIND_CHEST,
        slot: 16,
        generation: 4,
    };
    assert_eq!(
        chest.to_domain_present(),
        Err(ProtocolError::InvalidRange),
        "a chest slot past the fixed per-chunk array is not a real reference"
    );
}

/// The absent companion identity is a wire-only raw form: the domain companion
/// identity has no zero member, so a spawn or despawn can never name it.
#[test]
fn companion_absence_is_never_a_domain_identity() {
    assert!(mornlea_protocol::CompanionId::try_from_bytes([0; 16]).is_err());
    assert!(mornlea_protocol::CompanionDespawn::decode(&[0u8; 16]).is_err());
}

/// The protocol item stack is the domain value: the stack limit, durability
/// budget, smelting table and error mapping have exactly one owner. Every
/// registered item number is admitted by that single rule and the first
/// unregistered number fails, so a second table cannot reappear unnoticed.
#[test]
fn item_stack_rules_come_from_the_domain_tables() {
    // The protocol re-export and the domain value are the same type, so a
    // slot value crosses the boundary without a conversion.
    let shared: mornlea_domain::ItemStack = mornlea_protocol::ItemStack::EMPTY;
    assert_eq!(shared.count(), 0);

    for item in 1..=65u16 {
        let limit = mornlea_domain::item_stack_limit(item)
            .unwrap_or_else(|| panic!("registered item {item} has no stack limit"));
        let durability = mornlea_domain::durability_max(item);
        // The protocol constructor admits exactly what the domain predicates
        // describe, at the boundary counts and durabilities.
        assert!(
            mornlea_protocol::ItemStack::try_new(item, limit, 0).is_ok() || durability.is_some()
        );
        assert!(mornlea_protocol::ItemStack::try_new(item, limit + 1, 0).is_err());
        assert!(mornlea_protocol::ItemStack::try_new(item, 0, 0).is_err());
        match durability {
            Some(max) => {
                assert!(mornlea_protocol::ItemStack::try_new(item, 1, max).is_ok());
                assert!(mornlea_protocol::ItemStack::try_new(item, 1, max + 1).is_err());
                assert!(mornlea_protocol::ItemStack::try_new(item, 1, 0).is_err());
            }
            None => {
                assert!(mornlea_protocol::ItemStack::try_new(item, 1, 0).is_ok());
                assert!(mornlea_protocol::ItemStack::try_new(item, 1, 1).is_err());
            }
        }
        // The smelting and furnace-slot predicates read the same table.
        assert_eq!(
            mornlea_protocol::smelting_output(item),
            mornlea_domain::smelting_output(item)
        );
        let furnace_product = mornlea_protocol::ItemStack::try_new(item, 1, 0)
            .is_ok_and(mornlea_protocol::valid_furnace_output);
        assert_eq!(
            furnace_product,
            mornlea_domain::is_smelting_product(item),
            "furnace output whitelist diverged from the domain product set for item {item}"
        );
    }
    // The first unregistered number and the zero count have no rule.
    assert!(mornlea_protocol::ItemStack::try_new(66, 1, 0).is_err());
    assert!(mornlea_protocol::ItemStack::try_new(1, 0, 0).is_err());
}

/// A compact section moves between the wire and domain representations with
/// its palette order and packed word bits intact: no cell is expanded, no
/// palette is sorted, and no data is recompressed.
#[test]
fn indexed_section_conversion_keeps_palette_and_word_order() {
    let palette: Vec<u16> = vec![1, 5, 9, 13];
    let words: Vec<u64> = (0..256u64)
        .map(|index| {
            let mut word = 0u64;
            for slot in 0..16u64 {
                word |= ((index + slot) % palette.len() as u64) << (slot * 4);
            }
            word
        })
        .collect();
    let wire = SectionData::indexed(3, 4, palette.clone(), words.clone());
    let section = PalettedSection::try_from(wire).expect("checked indexed section");
    let (bits, moved_palette, moved_words) = section.as_indexed().expect("indexed representation");
    assert_eq!(bits, 4);
    assert_eq!(moved_palette, &palette[..]);
    assert_eq!(moved_words, &words[..]);
    let back = SectionData::try_from((section, 3)).expect("checked wire section");
    assert_eq!(back.y, 3);
    assert_eq!(back.storage, SectionStorage::Indexed);
    assert_eq!(back.bits, 4);
    assert_eq!(back.palette, palette);
    assert_eq!(back.packed, words);
    // The section index is supplied separately: the wire Y must stay inside
    // the chunk column.
    assert!(SectionData::try_from((PalettedSection::single(1).expect("air"), 24)).is_err());
}

/// Single and direct sections move the same way, and the domain gate rejects
/// a section the wire would not admit before any representation is published.
#[test]
fn single_and_direct_section_conversions_round_trip() {
    let single = PalettedSection::try_from(SectionData::single(7, 2)).expect("checked single");
    assert_eq!(single.as_single(), Some(2));
    assert_eq!(single.block_at(0), Some(2));
    let single_wire = SectionData::try_from((single, 7)).expect("checked wire single");
    assert_eq!(single_wire.y, 7);
    assert_eq!(single_wire.storage, SectionStorage::Single);
    assert_eq!(single_wire.single, 2);

    let direct_words: Vec<u64> = (0..1024u64).map(|index| index % 90).collect();
    let direct = PalettedSection::try_from(SectionData::direct(11, direct_words.clone()))
        .expect("checked direct section");
    assert_eq!(direct.as_direct(), Some(&direct_words[..]));
    let direct_wire = SectionData::try_from((direct, 11)).expect("checked wire direct section");
    assert_eq!(direct_wire.y, 11);
    assert_eq!(direct_wire.storage, SectionStorage::Direct);
    assert_eq!(direct_wire.packed, direct_words);

    let short_words = SectionData::indexed(0, 4, vec![1], vec![0u64; 3]);
    assert!(PalettedSection::try_from(short_words).is_err());
    let unregistered = SectionData::indexed(0, 4, vec![90], vec![0u64; 256]);
    assert!(PalettedSection::try_from(unregistered).is_err());
}

/// The pinned whitespace set is the only trim rule the admission path uses,
/// so a format character the Go baseline keeps is not trimmed and the whole
/// Go whitespace set is.
#[test]
fn pinned_whitespace_trims_only_the_frozen_go_set() {
    assert_eq!(
        mornlea_domain::trim_pinned_whitespace("\u{0009}n\u{000A}"),
        "n"
    );
    assert_eq!(
        mornlea_domain::trim_pinned_whitespace("\u{00A0}n\u{3000}"),
        "n"
    );
    assert_eq!(
        mornlea_domain::trim_pinned_whitespace("\u{200B}n"),
        "\u{200B}n",
        "a zero width space is not Go whitespace and stays in the text"
    );
    assert_eq!(mornlea_domain::trim_pinned_whitespace(""), "");
    assert_eq!(mornlea_domain::trim_pinned_whitespace("plain"), "plain");
}
