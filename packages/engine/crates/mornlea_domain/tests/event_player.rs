//! Command-outcome and player-publication event contracts for `mornlea_domain`.
//!
//! The Go baseline is the `protocol.PlayerState`, `protocol.CommandRejected`,
//! `protocol.PlaceBlockSucceeded` and `protocol.CombatHit` wire records in
//! `packages/shared/network/protocol`: a rejection, a placement success and a
//! combat hit are the three outcomes an authoritative tick publishes to the
//! session that caused them, and the player state is the private per-session
//! mirror of body, survival and world scalars. Every value rule below is the
//! Go validator's rule, so a record this crate admits is a record the protocol
//! layer admits and the other way around.

use mornlea_domain::{
    ActiveMining, ActiveMiningParts, BlockPos, CombatHit, CombatTarget, CommandRejection,
    Dimension, DomainError, FiniteVec3, LookAngles, MiningState, MiningStateParts, MotionState,
    MotionStateParts, PlacementSuccess, PlayerState, PlayerStateParts, RejectReason, Season,
    SurvivalState, SurvivalStateParts, Weather, WorldState, WorldStateParts,
};

/// Seed player state: every finite vector is zero, the survival scalars sit at
/// their maxima, the day phase offset at the last tick of the cycle, the world
/// time at zero, the weather and season at their first value, the season
/// progress and the temperature at the extremes of their full ranges, and the
/// mining block is the canonical inactive one.
fn seed_parts() -> PlayerStateParts {
    PlayerStateParts {
        server_tick: 0,
        last_input_sequence: 0,
        dimension: Dimension::OVERWORLD,
        motion: MotionState::new(MotionStateParts {
            position: FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite seed position"),
            velocity: FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite seed velocity"),
            on_ground: true,
        }),
        look: LookAngles::try_new(0.0, 0.0).expect("finite seed angles"),
        ready: true,
        reset: false,
        mining: MiningState::try_new(MiningStateParts {
            active: false,
            target: BlockPos::ORIGIN,
            progress: 0,
            required: 0,
            harvestable: false,
        })
        .expect("an all-zero inactive block is the idle union member"),
        survival: SurvivalState::try_new(SurvivalStateParts {
            health: 20,
            oxygen: 300,
            hunger: 20,
            saturation_zero: false,
            armor_points: 20,
        })
        .expect("the seed survival scalars sit at their published maxima"),
        world: WorldState::try_new(WorldStateParts {
            day_phase_offset: 23999,
            world_time_ticks: 0,
            weather: Weather::Clear,
            season: Season::Spring,
            season_progress: 255,
            temperature: -128,
        })
        .expect("the seed world scalars sit at their published bounds"),
    }
}

#[test]
fn event_player_seed_player_state_is_admitted_and_keeps_every_field() {
    let state = PlayerState::new(seed_parts());

    assert_eq!(state.server_tick(), 0);
    assert_eq!(state.last_input_sequence(), 0);
    assert_eq!(state.dimension(), Dimension::OVERWORLD);
    assert_eq!(state.motion().position().get(), [0.0, 0.0, 0.0]);
    assert_eq!(state.motion().velocity().get(), [0.0, 0.0, 0.0]);
    assert!(state.motion().on_ground());
    assert_eq!(state.look().yaw(), 0.0);
    assert_eq!(state.look().pitch(), 0.0);
    assert!(state.ready());
    assert!(!state.reset());
    assert_eq!(state.mining(), MiningState::Idle);
    assert_eq!(state.survival().health(), 20);
    assert_eq!(state.survival().oxygen(), 300);
    assert_eq!(state.survival().hunger(), 20);
    assert!(!state.survival().saturation_zero());
    assert_eq!(state.survival().armor_points(), 20);
    assert_eq!(state.world().day_phase_offset(), 23999);
    assert_eq!(state.world().world_time_ticks(), 0);
    assert_eq!(state.world().weather(), Weather::Clear);
    assert_eq!(state.world().season(), Season::Spring);
    assert_eq!(state.world().season_progress(), 255);
    assert_eq!(state.world().temperature(), -128);
}

#[test]
fn event_player_player_state_carries_no_inventory_or_equipped_armor() {
    // The private player publication is body, survival and world state only.
    // The inventory, crafting, furnace and chest publications are separate
    // records owned by later nodes, so this aggregate has no slot array and no
    // equipped-armor field for a later node to grow into a second inventory
    // mirror. The seed literal above lists every field explicitly, so an added
    // field fails to compile; this assertion fails on the same addition
    // through the published shape.
    let state = PlayerState::new(seed_parts());
    let debug = format!("{state:?}");
    for published in [
        "server_tick",
        "last_input_sequence",
        "dimension",
        "motion",
        "look",
        "ready",
        "reset",
        "mining",
        "survival",
        "world",
    ] {
        assert!(
            debug.contains(published),
            "player state must carry {published}"
        );
    }
    for absent in [
        "hotbar",
        "backpack",
        "inventory",
        "item",
        "stack",
        "slot",
        "equipped",
        "container",
        "crafting",
        "furnace",
        "chest",
    ] {
        assert!(
            !debug.contains(absent),
            "player state must not carry {absent}"
        );
    }
}

#[test]
fn event_player_survival_limits_reject_each_maximum_plus_one() {
    let admitted = SurvivalState::try_new(SurvivalStateParts {
        health: 20,
        oxygen: 300,
        hunger: 20,
        saturation_zero: true,
        armor_points: 20,
    })
    .expect("the four maxima are inside the published limits");
    assert!(admitted.saturation_zero());

    for parts in [
        SurvivalStateParts {
            health: 21,
            ..SurvivalStateParts {
                health: 20,
                oxygen: 300,
                hunger: 20,
                saturation_zero: false,
                armor_points: 20,
            }
        },
        SurvivalStateParts {
            oxygen: 301,
            ..SurvivalStateParts {
                health: 20,
                oxygen: 300,
                hunger: 20,
                saturation_zero: false,
                armor_points: 20,
            }
        },
        SurvivalStateParts {
            hunger: 21,
            ..SurvivalStateParts {
                health: 20,
                oxygen: 300,
                hunger: 20,
                saturation_zero: false,
                armor_points: 20,
            }
        },
        SurvivalStateParts {
            armor_points: 21,
            ..SurvivalStateParts {
                health: 20,
                oxygen: 300,
                hunger: 20,
                saturation_zero: false,
                armor_points: 20,
            }
        },
    ] {
        assert_eq!(
            SurvivalState::try_new(parts),
            Err(DomainError::InvalidSurvivalValue),
            "a survival scalar one above its maximum must be rejected"
        );
    }
}

#[test]
fn event_player_world_state_rejects_out_of_range_offset_weather_and_season() {
    let last_offset = WorldState::try_new(WorldStateParts {
        day_phase_offset: 23999,
        world_time_ticks: 0,
        weather: Weather::Clear,
        season: Season::Spring,
        season_progress: 255,
        temperature: -128,
    })
    .expect("the last offset of the cycle is inside the published bound");
    assert_eq!(last_offset.day_phase_offset(), 23999);

    // The offset bound is exclusive: one past the last tick of the day no
    // longer names a phase inside the cycle it shifts.
    assert_eq!(
        WorldState::try_new(WorldStateParts {
            day_phase_offset: 24000,
            ..WorldStateParts {
                day_phase_offset: 23999,
                world_time_ticks: 0,
                weather: Weather::Clear,
                season: Season::Spring,
                season_progress: 255,
                temperature: -128
            }
        }),
        Err(DomainError::InvalidDayPhaseOffset)
    );

    // Season progress and temperature are the two full-range scalars: every
    // u8 and i8 value is publishable, so neither has a maximum-plus-one case.
    for progress in [0u8, 128, 255] {
        let state = WorldState::try_new(WorldStateParts {
            season_progress: progress,
            ..WorldStateParts {
                day_phase_offset: 0,
                world_time_ticks: 0,
                weather: Weather::Clear,
                season: Season::Spring,
                season_progress: 255,
                temperature: -128,
            }
        })
        .expect("season progress is a whole u8");
        assert_eq!(state.season_progress(), progress);
    }
    for temperature in [i8::MIN, 0, i8::MAX] {
        let state = WorldState::try_new(WorldStateParts {
            temperature,
            ..WorldStateParts {
                day_phase_offset: 0,
                world_time_ticks: 0,
                weather: Weather::Clear,
                season: Season::Spring,
                season_progress: 255,
                temperature: -128,
            }
        })
        .expect("temperature is a whole i8");
        assert_eq!(state.temperature(), temperature);
    }

    // A zero world time and a nonzero one are both legal: the absolute time
    // has no lower bound and the day phase offset carries the presentation
    // phase.
    let timed = WorldState::try_new(WorldStateParts {
        world_time_ticks: 41,
        ..WorldStateParts {
            day_phase_offset: 0,
            world_time_ticks: 0,
            weather: Weather::Clear,
            season: Season::Spring,
            season_progress: 255,
            temperature: -128,
        }
    })
    .expect("a nonzero world time is legal");
    assert_eq!(timed.world_time_ticks(), 41);
}

#[test]
fn event_player_weather_and_season_enums_pin_their_wire_ids() {
    assert_eq!(Weather::try_new(0), Ok(Weather::Clear));
    assert_eq!(Weather::try_new(1), Ok(Weather::Rain));
    assert_eq!(Weather::try_new(2), Ok(Weather::Thunder));
    assert_eq!(Weather::try_new(3), Err(DomainError::InvalidWeather));
    assert_eq!(Weather::try_new(u8::MAX), Err(DomainError::InvalidWeather));
    assert_eq!(Weather::Clear.wire_id(), 0);
    assert_eq!(Weather::Rain.wire_id(), 1);
    assert_eq!(Weather::Thunder.wire_id(), 2);

    assert_eq!(Season::try_new(0), Ok(Season::Spring));
    assert_eq!(Season::try_new(1), Ok(Season::Summer));
    assert_eq!(Season::try_new(2), Ok(Season::Autumn));
    assert_eq!(Season::try_new(3), Ok(Season::Winter));
    assert_eq!(Season::try_new(4), Err(DomainError::InvalidSeason));
    assert_eq!(Season::try_new(u8::MAX), Err(DomainError::InvalidSeason));
    assert_eq!(Season::Spring.wire_id(), 0);
    assert_eq!(Season::Summer.wire_id(), 1);
    assert_eq!(Season::Autumn.wire_id(), 2);
    assert_eq!(Season::Winter.wire_id(), 3);
}

#[test]
fn event_player_player_state_rejects_non_finite_pose_and_angles() {
    // The grouped motion payload and the look angles carry already-validated
    // values, so a non-finite component cannot reach a player state at all:
    // the failure is the vector and angle constructors', pinned here.
    assert_eq!(
        FiniteVec3::try_new([f32::NAN, 0.0, 0.0]),
        Err(DomainError::NonFiniteValue)
    );
    assert_eq!(
        FiniteVec3::try_new([0.0, f32::INFINITY, 0.0]),
        Err(DomainError::NonFiniteValue)
    );
    assert_eq!(
        FiniteVec3::try_new([0.0, 0.0, f32::NEG_INFINITY]),
        Err(DomainError::NonFiniteValue)
    );
    assert_eq!(
        LookAngles::try_new(f32::NAN, 0.0),
        Err(DomainError::NonFiniteRotation)
    );
    assert_eq!(
        LookAngles::try_new(0.0, f32::NEG_INFINITY),
        Err(DomainError::NonFiniteRotation)
    );

    // A finite pose and finite angles survive, and their exact bits are kept,
    // so a replay disagrees with neither Go nor the wire.
    let position = FiniteVec3::try_new([0.5, -0.25, 64.0]).expect("finite seed position");
    assert_eq!(position.get(), [0.5, -0.25, 64.0]);
    let state = PlayerState::new(PlayerStateParts {
        motion: MotionState::new(MotionStateParts {
            position,
            velocity: FiniteVec3::try_new([0.0, 0.0, 0.0]).expect("finite velocity"),
            on_ground: false,
        }),
        look: LookAngles::try_new(-0.0, 0.0).expect("finite negative zero angles"),
        ..seed_parts()
    });
    assert!(!state.motion().on_ground());
    assert_eq!(state.look().yaw().to_bits(), (-0.0f32).to_bits());
}

#[test]
fn event_player_player_state_rejects_unknown_dimension() {
    assert_eq!(Dimension::new(2), Err(DomainError::InvalidDimension));
    assert_eq!(Dimension::new(u8::MAX), Err(DomainError::InvalidDimension));
    assert_eq!(Dimension::new(1), Ok(Dimension::DEPTHS));
    assert_eq!(Dimension::DEPTHS.get(), 1);
    assert_eq!(Dimension::OVERWORLD.get(), 0);
}

#[test]
fn event_player_inactive_mining_block_with_residue_fails_conversion() {
    let idle = MiningStateParts {
        active: false,
        target: BlockPos::ORIGIN,
        progress: 0,
        required: 0,
        harvestable: false,
    };
    assert_eq!(MiningState::try_new(idle), Ok(MiningState::Idle));

    // An inactive block that still carries any part of a swing describes no
    // legal union member: the client would have to guess whether a stale
    // target still applies, so the conversion fails instead of inferring.
    for parts in [
        MiningStateParts {
            target: BlockPos::new(1, 2, 3),
            ..idle
        },
        MiningStateParts {
            progress: 1,
            ..idle
        },
        MiningStateParts {
            required: 2,
            ..idle
        },
        MiningStateParts {
            harvestable: true,
            ..idle
        },
    ] {
        assert_eq!(
            MiningState::try_new(parts),
            Err(DomainError::InvalidMiningState),
            "an inactive mining block with residue must fail the conversion"
        );
    }
}

#[test]
fn event_player_active_mining_requires_progress_inside_the_open_range() {
    let active = MiningState::try_new(MiningStateParts {
        active: true,
        target: BlockPos::new(-4, 64, 7),
        progress: 1,
        required: 2,
        harvestable: true,
    })
    .expect("one of two ticks is a swing in progress");
    let MiningState::Active(mining) = active else {
        panic!("an active block converts into the active union member");
    };
    assert_eq!(mining.target(), BlockPos::new(-4, 64, 7));
    assert_eq!(mining.progress(), 1);
    assert_eq!(mining.required(), 2);
    assert!(mining.harvestable());

    // A completed swing is published as inactive, so a progress at or above
    // the requirement is not a legal active block, and neither is a zero one.
    for parts in [
        MiningStateParts {
            active: true,
            target: BlockPos::ORIGIN,
            progress: 0,
            required: 2,
            harvestable: false,
        },
        MiningStateParts {
            active: true,
            target: BlockPos::ORIGIN,
            progress: 2,
            required: 2,
            harvestable: false,
        },
    ] {
        assert_eq!(
            MiningState::try_new(parts),
            Err(DomainError::InvalidMiningState),
            "an active block outside 0<progress<required must fail the conversion"
        );
    }

    // The checked parts constructor carries the same rule on its own, so a
    // caller that already knows the block is active cannot bypass it.
    assert_eq!(
        ActiveMining::try_new(ActiveMiningParts {
            target: BlockPos::ORIGIN,
            progress: 0,
            required: 2,
            harvestable: false,
        }),
        Err(DomainError::InvalidMiningState)
    );
    assert_eq!(
        ActiveMining::try_new(ActiveMiningParts {
            target: BlockPos::ORIGIN,
            progress: 2,
            required: 2,
            harvestable: false,
        }),
        Err(DomainError::InvalidMiningState)
    );
    assert_eq!(
        ActiveMining::try_new(ActiveMiningParts {
            target: BlockPos::new(0, 319, 0),
            progress: 1,
            required: 2,
            harvestable: false,
        })
        .expect("one of two ticks is inside the open range")
        .progress(),
        1
    );
}

#[test]
fn event_player_command_rejection_and_placement_success_preserve_zero_sequence() {
    let rejection = CommandRejection::new(0, RejectReason::InvalidRay);
    assert_eq!(rejection.sequence(), 0);
    assert_eq!(rejection.reason(), RejectReason::InvalidRay);

    let success = PlacementSuccess::new(0);
    assert_eq!(success.sequence(), 0);

    let acknowledged = CommandRejection::new(41, RejectReason::NotArmor);
    assert_eq!(acknowledged.sequence(), 41);
    assert_eq!(acknowledged.reason(), RejectReason::NotArmor);
    assert_eq!(PlacementSuccess::new(41).sequence(), 41);
}

#[test]
fn event_player_reject_reason_wire_ids_are_the_frozen_table() {
    // The wire value comes from this explicit mapping, never from a
    // discriminant cast, so reordering the enum cannot change what the wire
    // carries. The internal Go enum is 0..14 while the wire enum is 1..15.
    let table = [
        (RejectReason::InvalidRay, 1),
        (RejectReason::NoTarget, 2),
        (RejectReason::ChunkNotReady, 3),
        (RejectReason::ProtectedBlock, 4),
        (RejectReason::InvalidBlock, 5),
        (RejectReason::Occupied, 6),
        (RejectReason::InvalidInput, 7),
        (RejectReason::PlayerNotReady, 8),
        (RejectReason::InvalidSlot, 9),
        (RejectReason::HotbarFull, 10),
        (RejectReason::DropCapacity, 11),
        (RejectReason::ContainerCapacity, 12),
        (RejectReason::NotFluidSource, 13),
        (RejectReason::BucketMismatch, 14),
        (RejectReason::NotArmor, 15),
    ];
    for (reason, wire) in table {
        assert_eq!(reason.wire_id(), wire);
    }
}

#[test]
fn event_player_combat_hit_requires_nonzero_tick_and_damage_inside_one_to_twenty() {
    let hit = CombatHit::try_new(7, 1, CombatTarget::Hostile).expect("a landed hit is publishable");
    assert_eq!(hit.server_tick(), 7);
    assert_eq!(hit.damage(), 1);
    assert_eq!(hit.target(), CombatTarget::Hostile);

    let lethal =
        CombatHit::try_new(8, 20, CombatTarget::Passive).expect("twenty damage is the maximum");
    assert_eq!(lethal.damage(), 20);

    assert_eq!(
        CombatHit::try_new(0, 1, CombatTarget::Player),
        Err(DomainError::InvalidCombatHit)
    );
    for damage in [0u8, 21] {
        assert_eq!(
            CombatHit::try_new(7, damage, CombatTarget::Player),
            Err(DomainError::InvalidCombatHit),
            "damage {damage} is outside 1..=20"
        );
    }
}

#[test]
fn event_player_combat_target_pins_its_wire_ids() {
    assert_eq!(CombatTarget::try_new(1), Ok(CombatTarget::Player));
    assert_eq!(CombatTarget::try_new(2), Ok(CombatTarget::Hostile));
    assert_eq!(CombatTarget::try_new(3), Ok(CombatTarget::Passive));
    assert_eq!(
        CombatTarget::try_new(0),
        Err(DomainError::InvalidCombatTarget)
    );
    assert_eq!(
        CombatTarget::try_new(4),
        Err(DomainError::InvalidCombatTarget)
    );
    assert_eq!(CombatTarget::Player.wire_id(), 1);
    assert_eq!(CombatTarget::Hostile.wire_id(), 2);
    assert_eq!(CombatTarget::Passive.wire_id(), 3);
}
