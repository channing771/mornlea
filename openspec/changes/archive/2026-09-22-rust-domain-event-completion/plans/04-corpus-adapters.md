# Executable Rust Event Corpus and Frozen Manifest

This packet owns tasks 4.1–4.3. Each node first publishes one complete manifest
candidate outside the repository, imports that reviewed candidate mechanically,
records the expected missing-owner failure, and only then implements the Rust
adapter that makes the new evidence executable. The tracked manifest is never
written by an ordinary test run.

The three nodes are deliberately serial. They share the canonical manifest,
the integrated Rust dispatcher and the runtime-oracle guide, and each node must
finish with a reviewed scoped commit before the next node starts. Workers do
not edit `tasks.md`, `ledger.md`, production Go protocol code or any Rust type
owned by tasks 2.1–3.1.

## Shared manifest-candidate contract

Node 4.1 creates
`packages/tools/cmd/runtime-oracle/domain_event_manifest_test.go`; nodes 4.2
and 4.3 reuse it without changing its ownership rules. The test-only helper has
this private shape:

```go
type domainEventSelection struct {
	Cases   []CaseSpec
	Sources []string
}

func mergeDomainEventSelections(
	t *testing.T,
	root string,
	base Inventory,
	selections ...domainEventSelection,
) Inventory

func writeDomainEventManifestCandidate(
	t *testing.T,
	root string,
	exportRoot string,
	inventory Inventory,
) string
```

Each producer exposes one selector with the same closed responsibility:
`domainEventMobsSelection`, `domainEventObjectsSelection` or
`domainEventChatSelection`. A selector returns only its exact reviewed
`CaseSpec` values and provenance paths. The merge helper must:

1. clone the loaded inventory and reject a duplicate case ID in either the
   base or any selection;
2. append the selected cases, sort the complete top-level case list by ID and
   leave every non-`domain.event` case byte-semantically unchanged;
3. locate exactly one `domain.event` family, replace its case IDs with the
   sorted union of every top-level `domain.event` case, and reject a family
   case that is absent from the top-level list;
4. union provenance by repository-relative path, recompute every
   `domain.event` source SHA-256 from `root`, sort sources by path, and leave
   all other family fields unchanged;
5. preserve `source_revision` in nodes 4.1 and 4.2; node 4.3 supplies the one
   explicitly refreshed revision described below;
6. call `Discover(root)` and `ReconcileWorking` with the baseline registries,
   encode through `encodeInventory`, load the written candidate again through
   `LoadInventory`, and reconcile that reloaded value before returning.

The writer delegates directory safety to the existing external-export
helpers, accepts only a fresh absolute `RUNTIME_ORACLE_EXPORT_DIR` outside the
repository with no symlinked ancestor, and writes exactly
`runtime-oracle/domain-event-manifest/contracts.json` below it. Its named tests
are:

```text
TestDomainEventManifestMergeRejectsDuplicateCase
TestDomainEventManifestMergeRejectsMissingFamilyCase
TestDomainEventManifestMergeSortsCasesSourcesAndFamilyCases
TestDomainEventManifestMergePreservesUnrelatedFamilies
TestDomainEventManifestCandidateRejectsRepositoryAndSymlinkTargets
TestDomainEventManifestCandidateReloadsAndReconciles
```

For each node, generate a fresh full candidate, inspect its cases, sources,
hashes and count, then copy that one file mechanically to
`testdata/runtime-migration/contracts.json`. Never assemble or edit hundreds
of JSON entries by hand. The candidate is the only generated manifest;
ordinary Go and Rust tests must leave both the tracked manifest and tracked
case directories unchanged.

All three Rust adapters use the existing `FrozenCase`, `execute_checked`,
strict JSON accessors and normalized comparison helpers. They parse raw input
themselves, reproduce the documented Go first-failure classification, call the
checked Rust constructor whenever the raw fields can be represented, and
normalize from the returned Rust value. An invalid raw enum byte or illegal
union combination that cannot enter a closed Rust type is classifier-only:
the adapter must still emit the exact Go error category/rule, but it must not
invent an `Unknown` Rust enum variant.

## Node 4.1: Execute and register the 68 mob cases

**Deliverable and prerequisites**

After tasks 1.1, 2.1, 2.4 and 3.1, register all 68 mob cases, prove exactly one
Rust owner for each, and raise the integrated domain partition from 376 to 444.
The Rust adapter owns only these six rules:

```text
hostile-spawn
hostile-state
hostile-despawn
passive-spawn
passive-state
passive-despawn
```

**Editable files**

- Create `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_mobs.rs`.
- Modify `packages/engine/crates/mornlea_domain/tests/corpus_domain.rs` to add
  the module, one `TOPICS` entry and total 444; rename the integrated test to
  the count-neutral `corpus_domain_executes_exact_unique_partition`.
- Create the shared
  `packages/tools/cmd/runtime-oracle/domain_event_manifest_test.go`.
- Modify
  `packages/tools/cmd/runtime-oracle/domain_event_mobs_test.go` only to expose
  `domainEventMobsSelection` and the candidate-generation test.
- Mechanically replace `testdata/runtime-migration/contracts.json` with the
  reviewed candidate.
- Modify `packages/engine/crates/mornlea_domain/AGENTS.md` and
  `packages/tools/cmd/runtime-oracle/AGENTS.md` with the adapter, manifest
  candidate and 68/444 ownership facts.

The mob assets, production Rust modules, other corpus adapters and every
non-`domain.event` manifest entry are read-only.

**Frozen adapter interface and validation order**

Export `pub const EXPECTED_COUNT: usize = 68`, `pub fn owns`, and
`pub fn execute` with the same signatures as existing topic adapters. Ownership
requires family `domain.event`, version `1`, operation `admit`, consumer
`mornlea_domain`, plus one of the six exact rules; it must not use a directory
prefix as the deciding condition.

For every rule, classify a raw batch count above 4,096 first, then an empty
batch, then scan each record in submitted order, then check strict ID order.
The per-record order is:

- hostile spawn: nonzero ID, raw dimension, finite position, finite yaw,
  health 1..=20, then kind;
- hostile state: nonzero ID, finite position, finite velocity, finite yaw,
  health 1..=20, then kind;
- hostile despawn: nonzero ID;
- passive spawn: nonzero ID, raw dimension, finite position, finite yaw, then
  health 1..=20;
- passive state: nonzero ID, finite position, finite velocity, finite yaw,
  health 1..=20, then grazing byte 0/1;
- passive despawn: nonzero ID, then reason byte 0/1.

Once raw conversion succeeds, construct every checked ID/vector/record and the
matching batch from `mornlea_domain::event::mobs`, map `DomainError` to the
frozen category/rule and normalize tick plus records by calling getters. Keep
submitted order. Unknown hostile kind, dimension, grazing and despawn reason
remain raw classifier-only branches. A zero tick is accepted.

Add focused adapter tests for all 68 executed IDs, exact `ok`/`error`
normalization, both hostile kinds, both grazing values, both despawn reasons,
health bounds, zero tick, and reversed/duplicate order. Also construct direct
raw probes proving count overflow wins over an empty/invalid record and that
the adapter does not sort.

**Test-first and manifest sequence**

1. Add the selection and shared candidate helper tests. Export a full manifest
   candidate and require exactly 68 added case IDs, 444 total domain cases and
   232 `domain.event` cases. `source_revision` remains unchanged.
2. Review and copy the candidate to the tracked manifest. Run the existing
   integrated Rust corpus before adding the module; record the intended
   `case has no owning topic` failure for a mob case.
3. Add `event_mobs.rs`, its focused tests, the module and `TOPICS` entry. Run
   the focused filter, then the complete integrated partition.
4. Re-export the candidate from a second fresh directory and require a byte
   identity diff with the tracked manifest. Record the manifest and case
   directories as unchanged across ordinary non-export test runs.

**Validation and closure**

```bash
mornlea_export_dir=$(mktemp -d)
RUNTIME_ORACLE_EXPORT_DIR="$mornlea_export_dir" go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventMobs|TestDomainEventManifest)'
diff -u testdata/runtime-migration/contracts.json "$mornlea_export_dir/runtime-oracle/domain-event-manifest/contracts.json"
go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventMobs|TestDomainEventManifest)'
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain event_mobs:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain corpus_domain_executes_exact_unique_partition --locked
go test ./packages/audit -count=1 -run '^(TestRuntimeOracleInternalDependencies|TestCorpusTestFlagsCannotRewriteFrozenEvidence|TestInternalDependenciesAreOneWay|TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --check
```

Require 444 exact unique executions, 232 exact `domain.event` cases, 68 owned
mob cases, a byte-identical external manifest and no tracked evidence rewrite
from ordinary tests. Proposed commit:
`test(domain): execute mob event corpus`.

Rollback removes the mob adapter/topic entry and manifest helper, restores the
pre-node manifest mechanically, and leaves the already-reviewed 68 Go assets
intact for a corrected successor.

## Node 4.2: Execute and register the 45 object cases

**Deliverable and prerequisites**

After tasks 1.2, 2.2 and 4.1, register 45 projectile/drop cases and raise the
domain partition from 444 to 489 and `domain.event` from 232 to 277. The Rust
adapter owns exactly:

```text
projectile-spawn
projectile-state
projectile-despawn
item-drop-upserts
item-drop-removes
```

**Editable files**

- Create
  `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_objects.rs`.
- Modify `packages/engine/crates/mornlea_domain/tests/corpus_domain.rs` only
  for the module, topic and total 489.
- Modify
  `packages/tools/cmd/runtime-oracle/domain_event_objects_test.go` only for
  `domainEventObjectsSelection` and its candidate test.
- Mechanically replace the manifest candidate and update the two scoped
  `AGENTS.md` guides with 45/489 ownership.

Node 4.1 files may be read but not structurally refactored. The shared manifest
helper changes only if an independently failing helper test exposes a defect;
such a change belongs in this same commit and must preserve the node 4.1
candidate byte-for-byte.

**Frozen adapter interface and validation order**

Export `EXPECTED_COUNT = 45`, `owns` and `execute`. Apply the shared raw count
precedence, then:

- projectile spawn: each nonzero ID, kind byte, dimension byte, finite
  position and finite velocity; strict numeric ID order last;
- projectile state: each nonzero ID and finite position; order last;
- projectile despawn: each nonzero ID; order last;
- item-drop upserts: each raw `DropId` (dimension, chunk, slot, generation),
  block index, then `ItemStack`; strict full `DropId` order last;
- item-drop removes: each raw `DropId`; strict full `DropId` order last.

Convert valid records to `ProjectileKind`, checked IDs, `Dimension`,
`FiniteVec3`, `DropId`, `ItemStack` and the matching event constructors. Map
the returned domain error and normalize only through getters. Unknown
projectile kind/dimension stays classifier-only. Raw drop dimension `-1` and
`ItemStack::EMPTY` must construct successfully; block index 98,304, slot 32,
zero generation, unregistered item 66, zero count and nondurable durability
must retain their exact independent Go verdicts.

Focused tests execute all 45 IDs and pin the four projectile
kind/dimension combinations, lossless raw drop dimension, full `DropId`
ordering, accepted empty stack, stack rejection rules, block index boundary,
zero tick, and unsorted-input preservation.

**Test-first and manifest sequence**

1. Export and review a candidate with exactly 45 added cases, 489 total domain
   cases and 277 `domain.event` cases. Its domain-event provenance adds exactly
   `packages/shared/core/drop.go`,
   `packages/shared/network/protocol/message_drop.go` and
   `packages/shared/network/protocol/message_projectile.go`; every previously
   present path remains and `source_revision` is unchanged.
2. Copy the candidate and run the integrated Rust test before the adapter is
   registered; record the intended no-owner failure for an object case.
3. Add the object adapter and topic, then run focused and integrated tests.
4. Regenerate from a new export root, prove byte identity, and prove ordinary
   test runs do not touch tracked evidence.

**Validation and closure**

```bash
mornlea_export_dir=$(mktemp -d)
RUNTIME_ORACLE_EXPORT_DIR="$mornlea_export_dir" go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventObjects|TestDomainEventManifest)'
diff -u testdata/runtime-migration/contracts.json "$mornlea_export_dir/runtime-oracle/domain-event-manifest/contracts.json"
go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventObjects|TestDomainEventManifest)'
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain event_objects:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain corpus_domain_executes_exact_unique_partition --locked
go test ./packages/audit -count=1 -run '^(TestRuntimeOracleInternalDependencies|TestCorpusTestFlagsCannotRewriteFrozenEvidence|TestInternalDependenciesAreOneWay|TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --check
```

Require 489 exact unique executions, 277 exact `domain.event` cases, 45 owned
object cases and an unchanged source revision. Proposed commit:
`test(domain): execute object event corpus`.

Rollback removes the object adapter/topic, restores the node 4.1 manifest and
counts, and retains the reviewed object assets.

## Node 4.3: Execute and register the 44 chat cases and close the 533-case partition

**Deliverable and prerequisites**

After tasks 1.3, 2.3 and 4.2, register all 44 chat cases, refresh the manifest's
source baseline once, add nine semantic anti-copy mutations and close the exact
533-case domain partition. Final `domain.event` count is 321.

**Editable files**

- Create
  `packages/engine/crates/mornlea_domain/tests/corpus_domain/event_chat.rs`.
- Modify `packages/engine/crates/mornlea_domain/tests/corpus_domain.rs` for the
  chat topic, total 533 and the nine mutation tests.
- Modify `packages/tools/cmd/runtime-oracle/domain_event_chat_test.go` only for
  `domainEventChatSelection` and the final candidate test.
- Modify `packages/tools/cmd/runtime-oracle/inventory.go` only to replace
  `BaselineSourceRevision` with the captured 40-hex execution SHA.
- Mechanically replace the manifest candidate and update the domain and
  runtime-oracle guides with the final 44/533/321/30-source facts.

All other implementation and evidence files are read-only.

**Frozen baseline refresh**

At the start of this node, after node 4.2 is committed and before any edit,
capture:

```bash
CORPUS_SOURCE_SHA=$(git rev-parse HEAD)
```

Require 40 lowercase hexadecimal characters and record the value in the
ledger handoff. Write exactly that same value to the manifest's
`source_revision` and Go `BaselineSourceRevision`; no later working-tree or
documentation commit changes it. The final `domain.event` source set has 30
unique sorted paths: the prior 25 plus exactly these five additions from the
three producer stages:

```text
packages/shared/core/drop.go
packages/shared/network/protocol/message_drop.go
packages/shared/network/protocol/message_hostile.go
packages/shared/network/protocol/message_passive.go
packages/shared/network/protocol/message_projectile.go
```

Every one of the 30 hashes is recomputed from that checkout. The final
candidate must reject duplicate/missing paths and a mismatch between the two
source-revision consumers.

**Frozen adapter interface and validation order**

Export `EXPECTED_COUNT = 44`, `owns` and `execute`; ownership is the exact
`chat` rule. Parse every raw field, apply global event/player identity and name
validation, reject non-speech payload carrying speech before kind dispatch,
then apply the exact kind-local order frozen in node 1.3: reason, companion
identity/name and command or speech text. Raw kind/reason/field combinations
that cannot form `ChatBody` are classifier-only.

For each legal branch, construct checked `PlayerId`, `DisplayName`,
`CompanionId`, `CompanionName`, `CommandText` or `SpeechText`, the exact closed
`ChatBody`, and finally `ChatEvent::try_new`. Normalize event/player identity
and only the branch's legal fields through getters. A zero event ID must reach
the Rust constructor and return `DomainError::InvalidIdentity`; no adapter
precheck may hide that domain proof.

Focused tests execute all 44 IDs, every 16 legal body branches, all 28 invalid
raw combinations, reserved reasons 3/15/21, zero companion sentinel semantics,
illegal command/speech leakage and zero event ID.

**Nine mandatory semantic mutations**

Add a helper that clones a loaded `FrozenCase`, mutates only its frozen
expected normalized JSON, retains the independently executed actual value,
calls `assert_domain_normalized` inside `catch_unwind`, and requires a panic.
Each named test uses one exact case and mutation. Together they cover every
semantic-drift class named by the delta specification:

```text
corpus_domain_rejects_mutated_hostile_kind_semantics
  domain.event/1/hostile-spawn-seed
  fields.spawns[0].kind: 0 -> 1

corpus_domain_rejects_mutated_hostile_health_semantics
  domain.event/1/hostile-spawn-seed
  fields.spawns[0].health: 10 -> 11

corpus_domain_rejects_mutated_passive_grazing_semantics
  domain.event/1/passive-state-seed
  fields.states[0].grazing: false -> true

corpus_domain_rejects_mutated_passive_despawn_reason_semantics
  domain.event/1/passive-despawn-seed
  fields.despawns[0].reason: 0 -> 1

corpus_domain_rejects_mutated_projectile_dimension_semantics
  domain.event/1/projectile-spawn-all-kind-dimension-combinations
  fields.spawns[0].dimension: 0 -> 1

corpus_domain_rejects_mutated_projectile_velocity_semantics
  domain.event/1/projectile-spawn-all-kind-dimension-combinations
  fields.spawns[0].velocity[0]: "3e800000" -> "00000000"

corpus_domain_rejects_mutated_item_drop_block_index_semantics
  domain.event/1/item-drop-upserts-seed
  fields.drops[0].block_index: 17 -> 18

corpus_domain_rejects_mutated_item_drop_order_semantics
  domain.event/1/item-drop-upserts-seed
  swap fields.drops[0] and fields.drops[1]

corpus_domain_rejects_mutated_chat_branch_semantics
  domain.event/1/chat-accepted
  category: "accepted" -> "speech"
```

The mob/passive/projectile seeds therefore retain the exact numeric and float
normalization frozen in node 1, and the item-drop seed contains at least two
valid, strictly ordered drops. These tests must fail because semantic content
changed, not because a hash, path or JSON shape became invalid.

**Test-first and manifest sequence**

1. Capture `CORPUS_SOURCE_SHA`. Export and review a candidate with 44 added
   cases, 533 total domain cases, 321 `domain.event` cases, 30 exact source
   paths and the same captured revision in both consumers.
2. Copy the candidate and update the Go constant. Before adding the chat topic,
   run the integrated Rust corpus and record the intended no-owner failure.
3. Add `event_chat.rs`, register its topic and total, then run the focused and
   integrated corpus suites.
4. Add each mutation test first with an unchanged expectation and record that
   it does not panic; apply its one named semantic mutation and require the
   comparison panic. Do not substitute malformed JSON or a missing file.
5. Re-export from a fresh directory, prove exact manifest identity, run the
   final source/revision/count assertions, and prove normal test runs cannot
   rewrite tracked evidence.

**Validation and closure**

```bash
mornlea_export_dir=$(mktemp -d)
RUNTIME_ORACLE_EXPORT_DIR="$mornlea_export_dir" go test ./packages/tools/cmd/runtime-oracle -race -count=1 -run '^(TestDomainEventChat|TestDomainEventManifest)'
diff -u testdata/runtime-migration/contracts.json "$mornlea_export_dir/runtime-oracle/domain-event-manifest/contracts.json"
go test ./packages/tools/cmd/runtime-oracle -race -count=1
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain event_chat:: --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain corpus_domain_executes_exact_unique_partition --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain corpus_domain_rejects_mutated_ --locked
go test ./packages/audit -count=1 -run '^(TestRuntimeOracleInternalDependencies|TestCorpusTestFlagsCannotRewriteFrozenEvidence|TestInternalDependenciesAreOneWay|TestEnglishCommentMigration|TestCodeCommentsExcludeTaskIDs)$'
git diff --check
```

Require 533 exact unique domain executions, 321 exact `domain.event` cases, 44
owned chat cases, all nine meaningful comparison failures, 30 unique verified
source hashes and one matching source revision. Proposed commit:
`test(domain): close event corpus coverage`.

Rollback removes the chat adapter/topic and mutation tests, restores the node
4.2 manifest, source-revision constant and counts, and retains the reviewed
chat assets. Never leave the manifest revision and Go constant mismatched.
