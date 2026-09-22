# Exhaustive Production Event Surface

This packet owns task 3.1. It integrates the checked values from tasks
2.1–2.3 but does not edit Go evidence, corpus assets, the corpus manifest or
the Rust corpus dispatcher.

## Node 3.1: Replace digest observations with the exact 30-variant event surface

**Deliverable and prerequisites**

After all new value types exist, replace the domain crate's digest-only
production observation API with the exact semantic `Event`, `EventRecipient`
and `RoutedEvent` contract in `00-execution-contract.md`. This is a public API
replacement, but repository-wide discovery confirms the old Rust API is used
only by two domain runtime-contract tests; the Go runtime-oracle type with the
same English word is out of scope and remains intact.

**Editable files**

- Modify `packages/engine/crates/mornlea_domain/src/event.rs`.
- Modify `packages/engine/crates/mornlea_domain/src/lib.rs`.
- Create `packages/engine/crates/mornlea_domain/tests/event_surface.rs`.
- Modify `packages/engine/crates/mornlea_domain/tests/runtime_contract.rs` only
  to delete the two obsolete digest-observation tests.
- Replace the `Observations` section in
  `packages/engine/crates/mornlea_domain/AGENTS.md` with the exact
  event-surface/routing ownership and focused test entry.

All leaf event modules and their tests are read-only. No protocol packet or
worker lifecycle type may be imported.

**Pre-edit consumer proof**

Run and record:

```bash
rg -n '\bObservation\b|order_observations|FAMILY_EVENT|FAMILY_INPUT' packages/engine/crates/mornlea_domain packages/engine/crates/mornlea_protocol packages/engine/crates/mornlea_storage
```

The only allowed production definitions/exports and consumers are
`mornlea_domain/src/event.rs`, `src/lib.rs`, the guide, and the two tests in
`tests/runtime_contract.rs`. Any additional consumer stops the node and returns
to the controller for artifact reconciliation.

**Exact implementation**

Delete the two family constants, `Observation`, its constructor/getters and
`order_observations`. Add all 30 variants and exact payload types from the
common packet in the listed order. No variant carries packet IDs, raw bytes,
digests, handshake/login/rejection/keepalive/disconnect, chunk-worker
acquire/generate/ready/resync, generated chunk pointers or a catch-all.

`EventRecipient::Session(u64)` deliberately accepts zero because session
existence and broadcast policy are runtime concerns; `Broadcast` is explicit
and not encoded as a sentinel session. `RoutedEvent::new` stores the supplied
recipient and event unchanged. Values that already contain a tick retain it;
the envelope adds none.

**Red/green sequence and exact tests**

1. Add `domain_public_api_has_no_digest_observation_exports` using
   `include_str!` for `src/event.rs` and `src/lib.rs`; assert the old four public
   names are absent. Run it and record the baseline source-contract failure.
2. Add seed helpers that construct legal values for every existing/new event
   payload. Fixtures use minimal registered values, checked IDs/text, one-record
   batches and existing public constructors; they do not duplicate validation
   logic.
3. Add `event_surface_constructs_exactly_30_semantic_variants`. Build one
   value of every variant in the specified order, pass each through an
   exhaustive `match` without a wildcard, and compare this exact name list:

```text
ChunkSnapshot
BlockChanges
ForgetChunks
PlayerState
CommandRejected
RemotePlayerSpawn
RemotePlayerDespawn
RemotePlayerStates
InventoryState
ItemDropUpserts
ItemDropRemoves
FurnaceState
ContainerClosed
ChestState
Chat
CompanionSpawn
CompanionStates
CompanionDespawn
PlaceBlockSucceeded
CraftingState
HostileSpawn
HostileState
HostileDespawn
CombatHit
PassiveSpawn
PassiveState
PassiveDespawn
ProjectileSpawn
ProjectileState
ProjectileDespawn
```

   Assert length 30 and 30 unique names. Exhaustiveness makes an unplanned
   future variant a compile failure rather than silently accepting it.
4. Add `routed_event_preserves_session_zero_and_event` and
   `routed_event_preserves_broadcast_and_event`. For the initial compiling
   skeleton, keep the input recipient unimplemented and record the session test
   failing because it observes the wrong recipient; then implement exact
   storage/getters.
5. Delete the old API and its two runtime-contract tests, add the exact enum and
   envelope derives/exports, update the guide, and format.

**Validation and derived-consumer review**

```bash
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_surface --locked -- --list
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_surface --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test runtime_contract --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --locked
rg -n '\bObservation\b|order_observations|FAMILY_EVENT|FAMILY_INPUT' packages/engine/crates/mornlea_domain packages/engine/crates/mornlea_protocol packages/engine/crates/mornlea_storage
go test ./packages/audit -count=1 -run '^(TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --check
```

The final `rg` may find explanatory prose in the active OpenSpec change only;
it must find no Rust production definition, re-export or crate test use. Require
the four new tests, exactly 30 names and the complete existing domain suite.

Independent review checks variant/payload mapping, the absence of transport and
worker messages, explicit routing, session-zero preservation, no fabricated
tick, and repository-wide old-API removal. Proposed commit:
`refactor(domain): publish exhaustive event surface`.

**Rollback**

Reverting this node restores the digest API and its two tests without removing
the new checked leaf values. It does not alter corpus evidence or any runtime
owner.
