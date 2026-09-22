//! Command envelope construction and the authoritative ordering of one tick's
//! commands.
//!
//! The Go baseline is `packages/server/sim/runtime/engine_step.go`: the
//! authoritative engine sorts the drained commands by session and sequence,
//! preserves arrival for an exact tie, and then admits a command only when its
//! sequence is strictly above the session's last admitted one. The winner of a
//! same-sequence tie is therefore the earliest arrival, whatever the two
//! command kinds are, because no kind-name tiebreaker exists on that path.
//! Session generation is runtime ingress lifecycle data, so a lexical kind
//! sort cannot substitute for the arrival index.

use mornlea_domain::{
    ChunkPos, Command, CommandEnvelope, CommandEnvelopeParts, CommandOrderScratch, DomainError,
    HeldActions, HotbarSlot, LookAngles, Movement, PlacementIntent, PlayerControl,
    PlayerControlParts, ResyncIntent, order_commands,
};

/// Red input of the packet, in arrival order at tick seven.
///
/// The contested pair is session one's two sequence-nine commands: the arrival
/// zero hotbar selection has to win over the arrival one block placement, and a
/// lexical kind sort would invert that pair because `place_block` sorts before
/// `select_hotbar`.
const RED_TICK: u64 = 7;

fn red_input() -> Vec<CommandEnvelope> {
    vec![
        envelope(2, 1, 0, select_hotbar(0)),
        envelope(1, 9, 0, select_hotbar(2)),
        envelope(1, 9, 1, place_block(0)),
        envelope(1, 8, 2, select_hotbar(1)),
    ]
}

/// Reversed arrival of the same contested pair, which moves the winner to the
/// block placement instead of the hotbar selection.
fn reversed_arrival_input() -> Vec<CommandEnvelope> {
    vec![
        envelope(2, 1, 0, select_hotbar(0)),
        envelope(1, 9, 0, place_block(0)),
        envelope(1, 9, 1, select_hotbar(2)),
        envelope(1, 8, 2, select_hotbar(1)),
    ]
}

fn envelope(session: u64, sequence: u64, arrival_index: u64, command: Command) -> CommandEnvelope {
    CommandEnvelope::try_new(CommandEnvelopeParts {
        tick: RED_TICK,
        session,
        sequence,
        arrival_index,
        command,
    })
    .expect("a legal envelope")
}

fn select_hotbar(slot: u8) -> Command {
    Command::SelectHotbar(HotbarSlot::new(slot).expect("a legal hotbar slot"))
}

fn place_block(slot: u8) -> Command {
    Command::PlaceBlock(
        PlacementIntent::try_new(LookAngles::try_new(0.0, 0.0).expect("finite angles"), slot)
            .expect("a legal placement slot"),
    )
}

/// Renders one batch as the `(tick, session, sequence, arrival_index)` key
/// order, which is the only thing the ordering rule is allowed to consult.
fn keys(commands: &[CommandEnvelope]) -> Vec<(u64, u64, u64, u64)> {
    commands
        .iter()
        .map(|command| {
            (
                command.tick(),
                command.session(),
                command.sequence(),
                command.arrival_index(),
            )
        })
        .collect()
}

fn scratch_for(commands: &[CommandEnvelope]) -> CommandOrderScratch {
    CommandOrderScratch::try_with_capacity(commands.len()).expect("an exactly sized scratch")
}

#[test]
fn command_order_red_input_orders_by_tick_session_sequence_then_arrival() {
    let mut commands = red_input();
    let mut scratch = scratch_for(&commands);
    order_commands(&mut commands, &mut scratch).expect("the red input has unique arrival keys");

    assert_eq!(
        keys(&commands),
        vec![
            (RED_TICK, 1, 8, 2),
            (RED_TICK, 1, 9, 0),
            (RED_TICK, 1, 9, 1),
            (RED_TICK, 2, 1, 0),
        ],
        "the red input has to order by tick, session, sequence and arrival index"
    );
    // A kind-name tiebreaker would place `place_block` before `select_hotbar`
    // inside session one's sequence nine pair, so the two orders are proven to
    // differ: the arrival index, not the kind name, decides the winner.
    assert_ne!(commands[1].command(), commands[2].command());
    assert_eq!(commands[1].command(), select_hotbar(2));
    assert_eq!(commands[2].command(), place_block(0));
}

#[test]
fn command_order_reverse_arrival_moves_the_same_sequence_winner() {
    let mut commands = reversed_arrival_input();
    let mut scratch = scratch_for(&commands);
    order_commands(&mut commands, &mut scratch).expect("reversed arrival keeps unique keys");

    // The same commands with the contested pair's arrival indices swapped keep
    // the same key order, but the winner inside session one's sequence nine
    // pair follows the arrival index rather than the kind name.
    assert_eq!(
        keys(&commands),
        vec![
            (RED_TICK, 1, 8, 2),
            (RED_TICK, 1, 9, 0),
            (RED_TICK, 1, 9, 1),
            (RED_TICK, 2, 1, 0),
        ]
    );
    assert_eq!(commands[1].command(), place_block(0));
    assert_eq!(commands[2].command(), select_hotbar(2));
}

#[test]
fn command_order_same_kind_duplicates_keep_arrival_order() {
    let mut commands = vec![
        envelope(1, 4, 1, select_hotbar(3)),
        envelope(1, 4, 0, select_hotbar(5)),
        envelope(1, 4, 2, select_hotbar(7)),
    ];
    let mut scratch = scratch_for(&commands);
    order_commands(&mut commands, &mut scratch).expect("same-kind duplicates stay unique");

    // Three commands of one kind, one session and one sequence differ only by
    // arrival index, so the arrival index is the whole ordering.
    assert_eq!(
        commands
            .iter()
            .map(|command| command.arrival_index())
            .collect::<Vec<_>>(),
        vec![0, 1, 2]
    );
    assert_eq!(commands[0].command(), select_hotbar(5));
    assert_eq!(commands[2].command(), select_hotbar(7));
}

#[test]
fn command_order_same_sequence_across_sessions_is_not_a_duplicate() {
    let mut commands = vec![
        envelope(2, 5, 0, select_hotbar(0)),
        envelope(1, 5, 0, select_hotbar(1)),
    ];
    let mut scratch = scratch_for(&commands);
    order_commands(&mut commands, &mut scratch).expect("sequences are only comparable per session");

    // Cross-session sequence numbers are not globally comparable, so the same
    // sequence in two sessions is legal and the session orders them.
    assert_eq!(
        keys(&commands),
        vec![(RED_TICK, 1, 5, 0), (RED_TICK, 2, 5, 0)]
    );
}

#[test]
fn command_order_two_ticks_keep_the_tick_first() {
    let mut commands = vec![
        CommandEnvelope::try_new(CommandEnvelopeParts {
            tick: 8,
            session: 1,
            sequence: 1,
            arrival_index: 0,
            command: select_hotbar(0),
        })
        .expect("a legal later envelope"),
        envelope(1, 9, 0, select_hotbar(2)),
    ];
    let mut scratch = scratch_for(&commands);
    order_commands(&mut commands, &mut scratch).expect("two ticks stay unique");

    // The tick leads the key, so a later tick's lowest sequence still follows
    // the earlier tick's highest one.
    assert_eq!(keys(&commands), vec![(RED_TICK, 1, 9, 0), (8, 1, 1, 0)]);
}

#[test]
fn command_order_duplicate_arrival_is_rejected_without_changing_commands() {
    let original = vec![
        envelope(2, 1, 0, select_hotbar(0)),
        envelope(1, 9, 0, select_hotbar(2)),
        envelope(1, 9, 0, place_block(0)),
        envelope(1, 8, 2, select_hotbar(1)),
    ];
    let mut commands = original.clone();
    let mut scratch = scratch_for(&commands);

    assert_eq!(
        order_commands(&mut commands, &mut scratch),
        Err(DomainError::DuplicateArrival),
        "two commands sharing tick, session and arrival index name one intake slot"
    );
    // The rejection has to happen before any command is reordered, so a caller
    // that keeps its own copy can still see the arrival order it submitted.
    assert_eq!(
        commands, original,
        "a rejected batch leaves the commands untouched"
    );
}

#[test]
fn command_order_empty_batch_is_accepted() {
    let mut commands: Vec<CommandEnvelope> = Vec::new();
    let mut scratch =
        CommandOrderScratch::try_with_capacity(0).expect("a zero-capacity scratch is reservable");
    assert_eq!(scratch.capacity(), 0);
    order_commands(&mut commands, &mut scratch).expect("an empty batch needs no key slot");
    assert!(commands.is_empty());
}

#[test]
fn command_order_exact_scratch_is_accepted_and_short_scratch_is_rejected() {
    let mut commands = red_input();
    let mut exact = scratch_for(&commands);
    let capacity = exact.capacity();
    order_commands(&mut commands, &mut exact).expect("an exactly sized scratch is enough");
    // The warm path writes into the scratch it was given and never grows it.
    assert_eq!(exact.capacity(), capacity);

    let mut commands = red_input();
    let mut short = CommandOrderScratch::try_with_capacity(commands.len() - 1)
        .expect("a short scratch is still reservable");
    let original = commands.clone();
    assert_eq!(
        order_commands(&mut commands, &mut short),
        Err(DomainError::InsufficientScratch),
        "a batch larger than the scratch has to be refused before any key is filled"
    );
    assert_eq!(
        commands, original,
        "a refused batch leaves the commands untouched"
    );
}

#[test]
fn command_order_scratch_is_reusable_after_a_failure() {
    let mut commands = red_input();
    let mut scratch = scratch_for(&commands);
    let capacity = scratch.capacity();

    let mut duplicated = red_input();
    duplicated.push(envelope(1, 3, 0, select_hotbar(4)));
    assert_eq!(
        order_commands(&mut duplicated, &mut scratch),
        Err(DomainError::InsufficientScratch),
        "the grown batch no longer fits the scratch"
    );
    let mut duplicate_arrival = vec![
        envelope(1, 9, 0, select_hotbar(2)),
        envelope(1, 9, 0, place_block(0)),
    ];
    assert_eq!(
        order_commands(&mut duplicate_arrival, &mut scratch),
        Err(DomainError::DuplicateArrival),
        "the duplicate arrival is still refused on the same scratch"
    );

    // Both failures leave the scratch's capacity intact, so the same scratch
    // admits the next batch without a fresh allocation.
    assert_eq!(scratch.capacity(), capacity);
    order_commands(&mut commands, &mut scratch).expect("the scratch is reusable after a failure");
    assert_eq!(
        keys(&commands),
        vec![
            (RED_TICK, 1, 8, 2),
            (RED_TICK, 1, 9, 0),
            (RED_TICK, 1, 9, 1),
            (RED_TICK, 2, 1, 0),
        ]
    );
}

#[test]
fn command_order_envelope_rejects_zero_sequence_only_for_take_crafting_output() {
    // Taking the crafting output has to take part in command acknowledgement,
    // so a zero sequence cannot name it. Every other command carries no such
    // rule and accepts a zero sequence, which is wire-valid.
    assert_eq!(
        CommandEnvelope::try_new(CommandEnvelopeParts {
            tick: RED_TICK,
            session: 1,
            sequence: 0,
            arrival_index: 0,
            command: Command::TakeCraftingOutput,
        }),
        Err(DomainError::InvalidSequence)
    );
    assert!(
        CommandEnvelope::try_new(CommandEnvelopeParts {
            tick: RED_TICK,
            session: 1,
            sequence: 0,
            arrival_index: 0,
            command: select_hotbar(0),
        })
        .is_ok()
    );
    assert!(
        CommandEnvelope::try_new(CommandEnvelopeParts {
            tick: RED_TICK,
            session: 1,
            sequence: 0,
            arrival_index: 0,
            command: Command::DropSelectedItem,
        })
        .is_ok()
    );
    assert!(
        CommandEnvelope::try_new(CommandEnvelopeParts {
            tick: RED_TICK,
            session: 1,
            sequence: 0,
            arrival_index: 0,
            command: Command::EquipArmor,
        })
        .is_ok()
    );
    assert!(
        CommandEnvelope::try_new(CommandEnvelopeParts {
            tick: RED_TICK,
            session: 1,
            sequence: 0,
            arrival_index: 0,
            command: Command::CloseContainer,
        })
        .is_ok()
    );

    // The envelope keeps the intake metadata verbatim and never derives it.
    let admitted = CommandEnvelope::try_new(CommandEnvelopeParts {
        tick: RED_TICK,
        session: 2,
        sequence: 3,
        arrival_index: 4,
        command: select_hotbar(5),
    })
    .expect("a legal envelope");
    assert_eq!(admitted.tick(), RED_TICK);
    assert_eq!(admitted.session(), 2);
    assert_eq!(admitted.sequence(), 3);
    assert_eq!(admitted.arrival_index(), 4);
    assert_eq!(admitted.command(), select_hotbar(5));
}

#[test]
fn command_order_envelope_carries_every_payload_family_unchanged() {
    let look = LookAngles::try_new(0.5, -0.25).expect("finite angles");
    let control = PlayerControl::new(PlayerControlParts {
        movement: Movement {
            move_x: 1,
            move_z: -1,
            jump: true,
        },
        look,
        actions: HeldActions {
            primary: true,
            eating: false,
            sprinting: false,
            sneaking: false,
        },
    });
    let commands = [
        Command::PlayerInput(control),
        Command::PlaceBlock(PlacementIntent::try_new(look, 8).expect("slot eight")),
        Command::Resync(ResyncIntent::try_new(1, ChunkPos::new(-1, 2), 0).expect("legal resync")),
        Command::OpenContainer(look),
        Command::TillSoil(look),
        Command::BoneMeal(look),
        Command::CollectWater(look),
        Command::PlaceWater(look),
    ];
    for command in commands {
        let admitted = CommandEnvelope::try_new(CommandEnvelopeParts {
            tick: 1,
            session: 1,
            sequence: 1,
            arrival_index: 0,
            command,
        })
        .expect("every command family is enveloped by the same constructor");
        assert_eq!(admitted.command(), command);
    }
}

#[test]
fn command_order_scratch_rejects_an_unreservable_capacity() {
    let err = CommandOrderScratch::try_with_capacity(usize::MAX)
        .expect_err("a capacity the process cannot honour fails closed");
    assert_eq!(
        err,
        DomainError::InsufficientScratch,
        "an unreservable request is a capacity problem the caller has to see"
    );
}
