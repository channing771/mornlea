//! Command envelope and the authoritative ordering of one tick's commands.
//!
//! The payloads in `control`, `inventory` and `chat` deliberately carry no
//! sequence, so the intake metadata that orders a command lives here beside the
//! payload rather than inside it. `CommandEnvelope` is that pairing, and
//! `order_commands` is the single ordering rule the authoritative runtime and a
//! replay share.
//!
//! The compatibility source is the Go authoritative engine in
//! `packages/server/sim/runtime/engine_step.go`: it sorts the drained commands
//! by session and sequence, preserves arrival for an exact tie, and admits a
//! command only when its sequence is strictly above the session's last admitted
//! one. The winner of a same-sequence tie is therefore the earliest arrival
//! whatever the two kinds are, because that path has no kind-name tiebreaker.
//! A lexical kind sort cannot substitute for the arrival index: session
//! generation is runtime ingress lifecycle data, and two commands of different
//! kinds legitimately share a sequence.

use crate::identity::DomainError;
use crate::input::control::Command;

/// Parts of one command envelope.
///
/// Every field is intake metadata the producer assigns when it admits the
/// command, except the command payload itself. The arrival index is the
/// producer's admission position inside one tick and session, so it is the only
/// value that can break a tie the tick, session and sequence leave open.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct CommandEnvelopeParts {
    pub tick: u64,
    pub session: u64,
    pub sequence: u64,
    pub arrival_index: u64,
    pub command: Command,
}

/// One sequenced command together with the intake metadata that orders it.
///
/// The envelope owns the sequence: the payload families carry none, so a
/// payload cannot claim to know when it was admitted. None of these fields is
/// an authoritative result, and the envelope applies no admission policy of its
/// own beyond the one command whose acknowledgement rule needs a nonzero
/// sequence.
#[derive(Clone, Copy, Debug, PartialEq)]
pub struct CommandEnvelope {
    tick: u64,
    session: u64,
    sequence: u64,
    arrival_index: u64,
    command: Command,
}

impl CommandEnvelope {
    /// Wraps one command with its intake metadata.
    ///
    /// `TakeCraftingOutput` is the one sequenced intent whose sequence may not
    /// be zero, because it has to take part in command acknowledgement. Every
    /// other command accepts a zero sequence, which is wire-valid and normally
    /// stale against the session's initial last admitted sequence.
    pub fn try_new(parts: CommandEnvelopeParts) -> Result<Self, DomainError> {
        if parts.sequence == 0 && matches!(parts.command, Command::TakeCraftingOutput) {
            return Err(DomainError::InvalidSequence);
        }
        Ok(Self {
            tick: parts.tick,
            session: parts.session,
            sequence: parts.sequence,
            arrival_index: parts.arrival_index,
            command: parts.command,
        })
    }

    pub fn tick(self) -> u64 {
        self.tick
    }

    pub fn session(self) -> u64 {
        self.session
    }

    pub fn sequence(self) -> u64 {
        self.sequence
    }

    pub fn arrival_index(self) -> u64 {
        self.arrival_index
    }

    pub fn command(self) -> Command {
        self.command
    }
}

/// Reusable key storage for `order_commands`.
///
/// The scratch owns the `(tick, session, arrival_index)` key slots the ordering
/// rule needs, so a warm tick writes into storage it already holds instead of
/// allocating. A caller that keeps one scratch per draining loop therefore pays
/// for the storage once, and a batch that does not fit is refused rather than
/// silently growing the scratch behind the caller's capacity budget.
#[derive(Clone, Debug)]
pub struct CommandOrderScratch {
    keys: Vec<(u64, u64, u64)>,
}

impl CommandOrderScratch {
    /// Reserves key storage for `max_commands` envelopes.
    ///
    /// The reservation is fallible so a capacity the process cannot honour
    /// fails closed instead of aborting: an unreservable request is a capacity
    /// problem the caller has to see, not a crash.
    pub fn try_with_capacity(max_commands: usize) -> Result<Self, DomainError> {
        let mut keys: Vec<(u64, u64, u64)> = Vec::new();
        keys.try_reserve_exact(max_commands)
            .map_err(|_| DomainError::InsufficientScratch)?;
        keys.resize(max_commands, (0, 0, 0));
        Ok(Self { keys })
    }

    /// The number of key slots this scratch can hold, which never grows.
    pub fn capacity(&self) -> usize {
        self.keys.len()
    }
}

/// Orders one tick's commands in place and validates their arrival keys.
///
/// The rule is `(tick, session, sequence, arrival_index)`. The tick leads, so a
/// later tick never interleaves with an earlier one; the session comes next,
/// because cross-session sequence numbers are not globally comparable; and the
/// arrival index is the final tiebreaker, which is what makes a same-sequence
/// pair of different kinds resolve to the earliest arrival. No key consults the
/// command kind, so a kind-name ordering can never stand in for the arrival
/// index.
///
/// The arrival keys are validated before any command is reordered: two
/// envelopes naming the same tick, session and arrival index describe one
/// intake position twice, and reporting that as an ordering result would leave
/// the caller unable to tell which of the two it submitted first. A rejected
/// batch therefore leaves `commands` exactly as the caller passed it.
///
/// A warm call allocates nothing: the command slice is sorted in place and the
/// key slots come from `scratch`.
pub fn order_commands(
    commands: &mut [CommandEnvelope],
    scratch: &mut CommandOrderScratch,
) -> Result<(), DomainError> {
    if commands.len() > scratch.capacity() {
        return Err(DomainError::InsufficientScratch);
    }
    let keys = &mut scratch.keys[..commands.len()];
    for (key, envelope) in keys.iter_mut().zip(commands.iter()) {
        *key = (
            envelope.tick(),
            envelope.session(),
            envelope.arrival_index(),
        );
    }
    // Sorting the keys turns the duplicate check into one adjacent comparison
    // per pair, and leaves the commands untouched while it runs.
    keys.sort_unstable();
    for pair in keys.windows(2) {
        if pair[0] == pair[1] {
            return Err(DomainError::DuplicateArrival);
        }
    }
    // The validated arrival key is a suffix of this key, so the full key is
    // unique and the unstable sort has exactly one correct answer.
    commands.sort_unstable_by_key(|envelope| {
        (
            envelope.tick(),
            envelope.session(),
            envelope.sequence(),
            envelope.arrival_index(),
        )
    });
    Ok(())
}
