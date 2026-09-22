//! Resource-bound precedence contracts for semantic batch construction.
//!
//! Every public batch constructor must reject an oversized input with
//! `BatchTooLarge` at its first line, before any per-record content relation
//! runs: the revision, span, membership and order scans, the duplicate
//! detection, and the sorted uniqueness scratch. The 4097-record inputs below
//! deliberately also violate a later content rule, so a `BatchTooLarge`
//! result proves the length gate precedes the content scan rather than merely
//! existing. The admitted 4096-record inputs use sorted or unique records and
//! must construct successfully, proving the cap is a work bound and not a
//! disguised protocol packet budget.

use mornlea_domain::{
    BlockChange, BlockChanges, BlockChangesParts, BlockPos, ChunkPos, CompanionId, CompanionState,
    CompanionStateParts, CompanionStates, CompanionStatesParts, Dimension, DomainError, FiniteVec3,
    ForgetChunks, ForgetChunksParts, LookAngles, MAX_SEMANTIC_BATCH_RECORDS, PlayerId,
    RemotePlayerState, RemotePlayerStateParts, RemotePlayerStates, RemotePlayerStatesParts,
};

/// Builds a distinct RFC-4122 v4 UUID from a counter, in wire byte order.
fn uuid_v4(counter: u128) -> [u8; 16] {
    let mut bytes = counter.to_be_bytes();
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    bytes
}

/// Builds `count` strictly increasing player identities.
fn ascending_player_ids(count: usize) -> Vec<PlayerId> {
    (1..=count as u128)
        .map(|counter| PlayerId::try_from_bytes(uuid_v4(counter)).expect("countered uuid v4"))
        .collect()
}

/// Builds one remote-player state record around `player_id`.
fn remote_state(player_id: PlayerId) -> RemotePlayerState {
    RemotePlayerState::new(RemotePlayerStateParts {
        player_id,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([1.0, 64.0, 2.0]).expect("finite seed position"),
        look: LookAngles::try_new(0.25, -0.5).expect("finite seed look"),
        reset: false,
    })
}

/// Builds one companion state record around `id`.
fn companion_state(id: CompanionId) -> CompanionState {
    CompanionState::try_new(CompanionStateParts {
        id,
        dimension: Dimension::OVERWORLD,
        position: FiniteVec3::try_new([3.0, 65.0, 4.0]).expect("finite seed position"),
        look: LookAngles::try_new(0.5, 0.25).expect("finite seed look"),
        reset: false,
    })
    .expect("overworld companion state inside the vertical look range")
}

/// Builds `count` block changes inside chunk `(0, 0)` whose chunk-ordered
/// block indexes are strictly increasing from zero.
fn ascending_block_changes(count: usize) -> Vec<BlockChange> {
    (0..count)
        .map(|index| {
            let position = BlockPos::new(
                (index % 16) as i32,
                -64 + (index / 256) as i32,
                ((index / 16) % 16) as i32,
            );
            BlockChange::try_new(position, 1).expect("registered seed block")
        })
        .collect()
}

#[test]
fn block_changes_admit_4096_records_and_reject_4097_before_revision_rules() {
    let admitted = BlockChanges::try_new(BlockChangesParts {
        dimension: Dimension::OVERWORLD,
        chunk: ChunkPos::new(0, 0),
        base_revision: 7,
        new_revision: 8,
        changes: ascending_block_changes(MAX_SEMANTIC_BATCH_RECORDS).into_boxed_slice(),
    })
    .expect("a cap-sized batch is a legal semantic batch");
    assert_eq!(admitted.changes().len(), MAX_SEMANTIC_BATCH_RECORDS);

    // The revision pair (base zero) also violates the later revision rule, so
    // `BatchTooLarge` proves the length gate precedes the revision check.
    let oversized = BlockChangesParts {
        dimension: Dimension::OVERWORLD,
        chunk: ChunkPos::new(0, 0),
        base_revision: 0,
        new_revision: 1,
        changes: ascending_block_changes(MAX_SEMANTIC_BATCH_RECORDS + 1).into_boxed_slice(),
    };
    assert_eq!(
        BlockChanges::try_new(oversized),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn empty_block_changes_remain_a_valid_revision_barrier() {
    let barrier = BlockChanges::try_new(BlockChangesParts {
        dimension: Dimension::OVERWORLD,
        chunk: ChunkPos::new(-3, 5),
        base_revision: 1,
        new_revision: 2,
        changes: Vec::new().into_boxed_slice(),
    })
    .expect("the empty revision barrier stays admitted");
    assert!(barrier.changes().is_empty());
}

#[test]
fn forget_chunks_admit_4096_chunks_and_reject_4097_before_duplicate_scan() {
    let admitted: Vec<ChunkPos> = (0..MAX_SEMANTIC_BATCH_RECORDS as i32)
        .map(|x| ChunkPos::new(x, 0))
        .collect();
    let wrapped = ForgetChunks::try_new(ForgetChunksParts {
        dimension: Dimension::OVERWORLD,
        chunks: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized forget batch is a legal semantic batch");
    assert_eq!(wrapped.chunks().len(), MAX_SEMANTIC_BATCH_RECORDS);

    // Every chunk is the same column, so the later duplicate scan would also
    // reject; `BatchTooLarge` proves the length gate precedes that scan.
    let oversized: Vec<ChunkPos> = vec![ChunkPos::new(0, 0); MAX_SEMANTIC_BATCH_RECORDS + 1];
    assert_eq!(
        ForgetChunks::try_new(ForgetChunksParts {
            dimension: Dimension::OVERWORLD,
            chunks: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn remote_player_states_admit_4096_records_and_reject_4097_before_order_scan() {
    let ids = ascending_player_ids(MAX_SEMANTIC_BATCH_RECORDS);
    let admitted: Vec<RemotePlayerState> = ids.iter().copied().map(remote_state).collect();
    let wrapped = RemotePlayerStates::try_new(RemotePlayerStatesParts {
        server_tick: 0,
        states: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized state batch is a legal semantic batch");
    assert_eq!(wrapped.states().len(), MAX_SEMANTIC_BATCH_RECORDS);

    // The oversized batch carries one record more than the cap with the first
    // two swapped, so the later order scan would also reject; `BatchTooLarge`
    // proves the length gate precedes the identity-order scan.
    let oversized_ids = ascending_player_ids(MAX_SEMANTIC_BATCH_RECORDS + 1);
    let mut oversized: Vec<RemotePlayerState> =
        oversized_ids.iter().copied().map(remote_state).collect();
    oversized.swap(0, 1);
    assert_eq!(
        RemotePlayerStates::try_new(RemotePlayerStatesParts {
            server_tick: 0,
            states: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}

#[test]
fn companion_states_admit_4096_records_and_reject_4097_before_order_scan() {
    let ids: Vec<CompanionId> = (1..=MAX_SEMANTIC_BATCH_RECORDS as u128)
        .map(|counter| CompanionId::try_from_bytes(uuid_v4(counter)).expect("countered uuid v4"))
        .collect();
    let admitted: Vec<CompanionState> = ids.iter().copied().map(companion_state).collect();
    let wrapped = CompanionStates::try_new(CompanionStatesParts {
        server_tick: 0,
        states: admitted.into_boxed_slice(),
    })
    .expect("a cap-sized companion batch is a legal semantic batch");
    assert_eq!(wrapped.states().len(), MAX_SEMANTIC_BATCH_RECORDS);

    let oversized_ids: Vec<CompanionId> = (1..=(MAX_SEMANTIC_BATCH_RECORDS + 1) as u128)
        .map(|counter| CompanionId::try_from_bytes(uuid_v4(counter)).expect("countered uuid v4"))
        .collect();
    let mut oversized: Vec<CompanionState> =
        oversized_ids.iter().copied().map(companion_state).collect();
    oversized.swap(0, 1);
    assert_eq!(
        CompanionStates::try_new(CompanionStatesParts {
            server_tick: 0,
            states: oversized.into_boxed_slice(),
        }),
        Err(DomainError::BatchTooLarge)
    );
}
