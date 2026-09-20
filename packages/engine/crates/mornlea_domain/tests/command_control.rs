//! Movement and ray command payload contracts for `mornlea_domain`.
//!
//! The Go baseline is the `core.PlayerInput` / `core.PlaceBlock` /
//! `core.SelectHotbar` / `core.RequestChunkResync` wire records: the move axes
//! and the held flags are client claims the authority judges later, while the
//! rotation is the one field the domain refuses to publish when it is not
//! finite.

use mornlea_domain::{
    ChunkPos, Command, DomainError, HeldActions, HotbarSlot, LookAngles, Movement, PlacementIntent,
    PlayerControl, PlayerControlParts, ResyncIntent,
};

/// Seed payload of the toggle case: every field carries a value that differs
/// from its zero value, so a toggle is always observable.
fn seed_parts() -> PlayerControlParts {
    PlayerControlParts {
        movement: Movement {
            move_x: 1,
            move_z: -1,
            jump: true,
        },
        look: LookAngles::try_new(0.5, -0.25).expect("finite seed angles"),
        actions: HeldActions {
            primary: true,
            eating: true,
            sprinting: true,
            sneaking: true,
        },
    }
}

#[test]
fn command_control_player_control_keeps_full_i8_axes_and_out_of_range_pitch() {
    let control = PlayerControl::new(PlayerControlParts {
        movement: Movement {
            move_x: i8::MIN,
            move_z: i8::MAX,
            jump: false,
        },
        look: LookAngles::try_new(0.0, 4.0).expect("pitch 4.0 is a finite angle"),
        actions: HeldActions {
            primary: false,
            eating: false,
            sprinting: false,
            sneaking: false,
        },
    });

    // The axes keep their full wire range: the −1..1 rule belongs to the
    // authority, and clamping here would rewrite a client claim.
    assert_eq!(control.movement().move_x, i8::MIN);
    assert_eq!(control.movement().move_z, i8::MAX);
    assert!(!control.movement().jump);
    // Pitch is finite-only with no clamping, so 4.0 stays 4.0.
    assert_eq!(control.look().pitch(), 4.0);
    assert_eq!(control.look().yaw(), 0.0);
}

#[test]
fn command_control_non_finite_rotation_fails_look_angles() {
    assert_eq!(
        LookAngles::try_new(f32::NAN, 0.0),
        Err(DomainError::NonFiniteRotation)
    );
    assert_eq!(
        LookAngles::try_new(0.0, f32::NAN),
        Err(DomainError::NonFiniteRotation)
    );
    assert_eq!(
        LookAngles::try_new(f32::INFINITY, 0.0),
        Err(DomainError::NonFiniteRotation)
    );
    assert_eq!(
        LookAngles::try_new(0.0, f32::NEG_INFINITY),
        Err(DomainError::NonFiniteRotation)
    );

    // A non-finite rotation cannot reach the grouped payload at all: the parts
    // carry an already-validated `LookAngles`, so the failure happens before
    // any payload exists rather than inside one.
    let err = LookAngles::try_new(0.0, f32::NAN).expect_err("NaN pitch must fail before parts");
    assert_eq!(err, DomainError::NonFiniteRotation);

    let finite = LookAngles::try_new(0.5, -0.25).expect("finite angles reach the parts");
    let control = PlayerControl::new(PlayerControlParts {
        movement: Movement {
            move_x: 0,
            move_z: 0,
            jump: false,
        },
        look: finite,
        actions: HeldActions {
            primary: false,
            eating: false,
            sprinting: false,
            sneaking: false,
        },
    });
    assert!(control.look().yaw().is_finite() && control.look().pitch().is_finite());
}

#[test]
fn command_control_held_flags_round_trip_independently() {
    for flag in 0..4 {
        let mut actions = HeldActions {
            primary: false,
            eating: false,
            sprinting: false,
            sneaking: false,
        };
        match flag {
            0 => actions.primary = true,
            1 => actions.eating = true,
            2 => actions.sprinting = true,
            _ => actions.sneaking = true,
        }
        let control = PlayerControl::new(PlayerControlParts {
            movement: Movement {
                move_x: 0,
                move_z: 0,
                jump: false,
            },
            look: LookAngles::try_new(0.0, 0.0).expect("finite angles"),
            actions,
        });

        assert_eq!(
            (
                control.actions().primary,
                control.actions().eating,
                control.actions().sprinting,
                control.actions().sneaking
            ),
            (flag == 0, flag == 1, flag == 2, flag == 3),
            "held flag {flag} must survive without disturbing its siblings"
        );
    }
}

#[test]
fn command_control_placement_slot_eight_succeeds_and_nine_fails() {
    let look = LookAngles::try_new(0.0, 0.0).expect("finite angles");
    let placed = PlacementIntent::try_new(look, 8).expect("slot eight is inside the hotbar range");
    assert_eq!(placed.slot(), HotbarSlot::new(8).expect("slot eight"));
    assert_eq!(placed.look(), look);

    assert_eq!(
        PlacementIntent::try_new(look, 9),
        Err(DomainError::InvalidHotbarSlot)
    );
    assert_eq!(
        PlacementIntent::try_new(look, u8::MAX),
        Err(DomainError::InvalidHotbarSlot)
    );
}

#[test]
fn command_control_resync_accepts_depths_dimension_and_zero_revision() {
    let chunk = ChunkPos::new(-3, 7);
    let resync =
        ResyncIntent::try_new(1, chunk, 0).expect("depths dimension and a zero revision are legal");
    assert_eq!(resync.dimension(), mornlea_domain::Dimension::DEPTHS);
    assert_eq!(resync.chunk(), chunk);
    assert_eq!(resync.have_revision(), 0);

    let held = ResyncIntent::try_new(0, chunk, 41).expect("overworld resync is legal");
    assert_eq!(held.dimension(), mornlea_domain::Dimension::OVERWORLD);
    assert_eq!(held.have_revision(), 41);

    assert_eq!(
        ResyncIntent::try_new(2, chunk, 0),
        Err(DomainError::InvalidDimension)
    );
}

#[test]
fn command_control_ray_intents_share_finite_rule_and_keep_signed_zero() {
    let negative_zero = LookAngles::try_new(-0.0, -0.0).expect("finite negative zero angles");
    let commands = [
        Command::OpenContainer(negative_zero),
        Command::TillSoil(negative_zero),
        Command::BoneMeal(negative_zero),
        Command::CollectWater(negative_zero),
        Command::PlaceWater(negative_zero),
    ];
    for command in commands {
        let look = match command {
            Command::OpenContainer(look)
            | Command::TillSoil(look)
            | Command::BoneMeal(look)
            | Command::CollectWater(look)
            | Command::PlaceWater(look) => look,
            _ => unreachable!("this case only builds ray intents"),
        };
        // The ray intents carry the same finite-only rule and keep the exact
        // bits, so a replay disagrees with neither Go nor the wire.
        assert_eq!(look.yaw().to_bits(), (-0.0f32).to_bits());
        assert_eq!(look.pitch().to_bits(), (-0.0f32).to_bits());
    }

    // The same rule rejects a non-finite angle before any ray intent exists.
    assert_eq!(
        LookAngles::try_new(f32::NAN, 0.0),
        Err(DomainError::NonFiniteRotation)
    );
}

#[test]
fn command_control_command_carries_every_movement_and_ray_variant() {
    let seed = PlayerControl::new(seed_parts());
    let look = LookAngles::try_new(0.5, -0.25).expect("finite angles");

    assert_eq!(
        Command::PlayerInput(seed),
        Command::PlayerInput(PlayerControl::new(seed_parts()))
    );
    assert_eq!(
        Command::PlaceBlock(PlacementIntent::try_new(look, 8).expect("slot eight")),
        Command::PlaceBlock(PlacementIntent::try_new(look, 8).expect("slot eight"))
    );
    assert_eq!(
        Command::Resync(ResyncIntent::try_new(1, ChunkPos::new(-1, 2), 0).expect("legal resync")),
        Command::Resync(ResyncIntent::try_new(1, ChunkPos::new(-1, 2), 0).expect("legal resync"))
    );
    assert_eq!(
        Command::SelectHotbar(HotbarSlot::new(3).expect("slot three")),
        Command::SelectHotbar(HotbarSlot::new(3).expect("slot three"))
    );

    // A ray intent is not interchangeable with the movement payload: the
    // variants stay distinct even when the angles are equal.
    assert_ne!(Command::OpenContainer(look), Command::TillSoil(look));
    assert_ne!(Command::OpenContainer(look), Command::PlayerInput(seed));
}

#[test]
fn command_control_seed_input_toggles_every_field() {
    let base = seed_parts();
    let seed = PlayerControl::new(base);
    assert_eq!(seed.movement().move_x, 1);
    assert_eq!(seed.movement().move_z, -1);
    assert!(seed.movement().jump);
    assert_eq!(seed.look().yaw(), 0.5);
    assert_eq!(seed.look().pitch(), -0.25);
    assert!(seed.actions().primary);
    assert!(seed.actions().eating);
    assert!(seed.actions().sprinting);
    assert!(seed.actions().sneaking);

    // Toggle one field at a time and assert the change is observable through
    // the normalized record while every sibling keeps the seed value.
    let toggled_move_x = PlayerControl::new(PlayerControlParts {
        movement: Movement {
            move_x: 0,
            ..base.movement
        },
        ..base
    });
    assert_eq!(toggled_move_x.movement().move_x, 0);
    assert_eq!(toggled_move_x.movement().move_z, base.movement.move_z);
    assert_eq!(toggled_move_x.movement().jump, base.movement.jump);

    let toggled_move_z = PlayerControl::new(PlayerControlParts {
        movement: Movement {
            move_z: 0,
            ..base.movement
        },
        ..base
    });
    assert_eq!(toggled_move_z.movement().move_z, 0);
    assert_eq!(toggled_move_z.movement().move_x, base.movement.move_x);

    let toggled_jump = PlayerControl::new(PlayerControlParts {
        movement: Movement {
            jump: false,
            ..base.movement
        },
        ..base
    });
    assert!(!toggled_jump.movement().jump);
    assert_eq!(toggled_jump.movement().move_x, base.movement.move_x);
    assert_eq!(toggled_jump.movement().move_z, base.movement.move_z);

    let toggled_yaw = PlayerControl::new(PlayerControlParts {
        look: LookAngles::try_new(0.0, base.look.pitch()).expect("finite angles"),
        ..base
    });
    assert_eq!(toggled_yaw.look().yaw(), 0.0);
    assert_eq!(toggled_yaw.look().pitch(), base.look.pitch());

    let toggled_pitch = PlayerControl::new(PlayerControlParts {
        look: LookAngles::try_new(base.look.yaw(), 0.0).expect("finite angles"),
        ..base
    });
    assert_eq!(toggled_pitch.look().pitch(), 0.0);
    assert_eq!(toggled_pitch.look().yaw(), base.look.yaw());

    for flag in 0..4 {
        let toggled_actions = HeldActions {
            primary: base.actions.primary && flag != 0,
            eating: base.actions.eating && flag != 1,
            sprinting: base.actions.sprinting && flag != 2,
            sneaking: base.actions.sneaking && flag != 3,
        };
        let toggled = PlayerControl::new(PlayerControlParts {
            actions: toggled_actions,
            ..base
        });
        assert_eq!(toggled.actions().primary, flag != 0);
        assert_eq!(toggled.actions().eating, flag != 1);
        assert_eq!(toggled.actions().sprinting, flag != 2);
        assert_eq!(toggled.actions().sneaking, flag != 3);
        assert_eq!(toggled.movement(), base.movement);
        assert_eq!(toggled.look(), base.look);
    }
}
