# rust-domain-event-completion ledger

## 2026-09-22 — architecture-first planning correction

- Planning baseline: `3401fa7a12c97844791822f185071a3641998773`,
  which descends from `d241611316fb683d5eee99dc194a24d33fd8cd52` and
  contains merged `main` commit `effd8a247427d2ab5710a8f481349c4cd4676721`
  from PR #184. Its later `redesign-ci-standards` task packets are committed,
  unrelated planning-only additions and do not change this event successor's
  source, ownership or validation boundaries.
- Discovery: the accepted F1 baseline deliberately stops before hostile/passive, projectile/drop and chat domain events, complete protocol/save evidence, public numerical APIs, pathfinding and final F1 acceptance. Current `mornlea_domain` and the runtime-migration corpus confirm those boundaries.
- Ruling: create this event-only F1 successor before refining or implementing F2. Preserve the current Go authority, all versions and default startup paths; keep `rust-authoritative-server` blocked on every remaining F1 successor and final zero-gap acceptance.
- Scope: checked remaining event families, the exact 30-variant semantic union, separate recipient routing, bounded atomic batches and independently executed Go/Rust evidence. Protocol/storage conversion, numerical APIs, pathfinding and online authority are excluded.
- Orchestration: the controller performed the coupled architecture and artifact work directly. No implementation or delegated worker started. Detailed tasks remain intentionally absent until the user reviews and approves the written proposal, delta specification and design under the Superpowers planning gate.
- Validation: the placeholder scan was clean; the event-surface audit found exactly 30 unique variants; `git diff --check`, both scoped strict validations and `openspec validate --all --strict --no-interactive` passed (127 items). No runtime test result is claimed by this planning entry.
- Architecture skill: no change. The decision applies the existing target ownership and migration-order rules rather than adding a new stable cross-task convention.

## 2026-09-22 — detailed implementation-plan readiness

- User approval: the written proposal, delta specification and design were
  approved before detailed task decomposition began. No implementation or
  delegated worker was started.
- Superpowers planning: `writing-plans` produced one status plan in `tasks.md`
  and six linked execution packets. The 12 independently reviewable nodes are
  three Go evidence producers, three Rust value families plus the shared bounds
  gate, one exact event surface, three serial corpus registrations and one
  controller-owned closeout.
- Architecture correction: detailed review removed event-wire codec evidence
  from this successor. Current package-local Go validators are the
  compatibility oracle; codec evidence remains with the later protocol
  successor. The proposal/spec/design/task packets now agree on that boundary.
- Frozen evidence: producers emit exactly 68 mob, 45 object and 44 chat cases;
  Rust integration advances 376 → 444 → 489 → 533 total domain cases
  and 164 → 232 → 277 → 321 `domain.event` cases. Final provenance is
  30 unique source paths, and nine named semantic mutations cover kind, health,
  grazing, despawn reason, dimension, velocity, block index, identity order and
  chat branch drift.
- Readiness ruling: every node has exact predecessors, editable/read-only
  files, public or private interfaces, validation precedence, numeric limits,
  a behavioral red/green sequence, focused commands, nonzero expected counts,
  a scoped commit and rollback. `tasks.md` is the only checkbox source; linked
  packets contain no competing checkbox list.
- Self-review: all four delta requirements and their scenarios map to named
  tests; the placeholder scan is clean; public type names agree across value,
  event and corpus packets; the dependency graph is acyclic; and the five
  global review-focus risks each have an owning test node.
- Validation: `openspec validate rust-domain-event-completion --strict
  --no-interactive` passed; `openspec validate rust-authoritative-server
  --strict --no-interactive` passed; `openspec validate --all --strict
  --no-interactive` passed 127/127; whitespace/diff checks were clean. These are
  planning checks only and do not claim runtime implementation results.
- Orchestration readiness: no delegated assignment store was created because
  the user has not yet chosen native or subagent-driven execution. The plan
  recommends subagent-driven execution because each of the 12 durable contract
  or corpus nodes benefits from a fresh implementation context and independent
  acceptance review.
- Architecture skill: no change. Detailed planning applies the existing
  semantic-boundary, evidence and F1-before-F2 rules; it does not establish a
  new verified cross-task convention.

## 2026-09-22 — execution start (subagent-driven)

- Workflow: user selected Superpowers `subagent-driven-development`. Fresh
  implementer plus task reviewer per node, controller-owned acceptance and
  closeout per `plans/05-closeout.md`.
- Execution baseline: branch `codex/rust-domain-event-completion` created at
  `af9b8c32ab0c924e5647fb8a62c4095192b5a558` (descends from planning baseline
  `3401fa7a12c97844791822f185071a3641998773` and merged-main `effd8a24`). The
  four-point orphan state passed: `tasks.md` shows 12 open/0 complete, ledger
  frontier is the planning-readiness entry, `git status` is clean, and change
  artifact mtimes are all within the planning window.
- Pre-flight plan scan: no packet conflicts found; the serial node order
  1.1→1.2→1.3→2.1→2.2→2.3→2.4→3.1→4.1→4.2→4.3→5.1 satisfies every declared
  prerequisite edge.
- SDD progress ledger for this change initialized at
  `.superpowers/sdd/progress.md` (prior file archived inline for the closed
  `rust-runtime-foundation-baseline` run).

## 2026-09-22 — controller ruling: rejection categories stay in the frozen corpus vocabulary

- Node 1.1 implementer escalated a real conflict before writing code: the
  packets named rejection categories `invalid-count`, `invalid-order`,
  `invalid-text` and `invalid-union`, while
  `TestCorpusOutcomeVocabularyMatchesExecutionContract` walks the whole cases
  tree and freezes error categories to the baseline set (structural
  truncated/trailing/invalid-varint/capacity/unsupported-version/
  invalid-enum/invalid-identity/invalid-value/integrity plus admission and
  storage sets). Existing same-family same-version `domain.event` cases
  already classify empty batches and ordering failures as `invalid-value`
  under `.count_range` / `.strictly_increasing_ids` rules.
- Ruling: rule names are unchanged; category values fold into the frozen
  vocabulary — `invalid-value` for empty batch, ordering, text-boundary and
  illegal cross-field rejections; `invalid-enum` for unknown kinds and
  reserved/out-of-domain reasons. Rationale: the user-approved proposal,
  delta spec and design name no categories, so only controller-authored
  packets are affected; same-version corpus evidence must classify one
  semantic class identically; extending a frozen gate without approved spec
  text is out of scope for this change.
- Secondary ruling: registering the new producer IDs
  (`runtime-oracle/domain-event-mobs`, later `-objects`, `-chat`) in
  `validProducerIDs` in `runner_helpers_test.go` is mechanical registration
  the mandated export path requires; it is an implied editable line for the
  oracle nodes, not an ownership violation.
- Artifacts reconciled before implementation resumed:
  `plans/01-go-oracles.md` category sentences only. Packet 04 needs no edit
  (its category references are producer-delegating). Accepted-branch subject
  categories on `kind: "ok"` outcomes remain unrestricted.

## 2026-09-22 — node 1.1 acceptance

- Implementation commit: `b261de1e` (`test(runtime-oracle): add mob event
  evidence`, 140 files). Red evidence verified: the routing regression failed
  on the baseline with `unknown rule "hostile-spawn"` before the producer
  existed, matching the packet's intended behavioral red.
- Controller gates at `b261de1e`: `go test ./packages/tools/cmd/runtime-oracle
  -race -count=1 -run '^(TestDomainEventMobs|TestDomainOracle_event_mobs)'` ok
  (12 tests; run with `GOCACHE=/tmp/mornlea-gocache` after the default
  user cache hit a sandbox-denied entry — environment workaround, same
  command otherwise); audit guards `TestRuntimeOracleInternalDependencies`,
  `TestCorpusTestFlagsCannotRewriteFrozenEvidence`,
  `TestEnglishCommentMigration`, `TestCodeCommentsExcludeTaskIDs` ok;
  `git diff --exit-code -- testdata/runtime-migration/contracts.json` clean;
  `git diff --check` clean. Implementer additionally proved 136 external
  files and an empty `diff -ru` against the tracked directory.
- Review ruling: spec compliant, quality Approved, 0 Critical, 0 Important.
  Reviewer independently confirmed zero-tick admission and
  classifier-vs-validator check order against the real Go validators.
- Minors recorded for the final whole-branch review: (1) implementer report
  prose says 37 pinned rejection rule names, the test pins 36 (7+8+3+7+8+3) —
  code correct; (2) `domainEventMobsExportPublished` is a novel once-per-
  process export guard with no sibling precedent — safe here (sequential
  same-package tests), but the double-publication hazard it solves exists for
  sibling producers too; (3) the outcomes test infers the batch array key
  from the first accepted record instead of naming it per rule.
- Architecture skill: no change. The producer mirrors established
  corpus conventions; no new cross-task convention qualified.

## 2026-09-22 — node 1.2 acceptance

- Implementation commit: `6d97b1f5` (`test(runtime-oracle): add object event
  evidence`, 94 files). Red evidence verified: routing regression failed with
  `unknown rule "projectile-spawn"` on the node 1.1 baseline, matching the
  intended behavioral red.
- Controller gates at `6d97b1f5`: `go test ./packages/tools/cmd/runtime-oracle
  -race -count=1 -run '^(TestDomainEventObjects|TestDomainOracle_event_objects)'`
  ok (`GOCACHE=/tmp/mornlea-gocache`); the four audit guards ok;
  `git diff --exit-code -- testdata/runtime-migration/contracts.json` clean;
  `git diff --check` clean. Implementer proved 90 external files and an empty
  `diff -ru`, and the whole runtime-oracle package green.
- Review ruling: spec compliant, quality Approved, 0 Critical, 0 Important.
  Reviewer independently verified the five validators' check order and the
  zero-tick admission against the read-only authority files.
- Controller adjudication of the reviewer's observation: removes rule names
  use `id_<index>.{slot,generation}` rather than mob-style `record_<index>`.
  Accepted — the brief blesses subject-specific prefixes via its explicit
  `drop_<index>` examples, removes records are bare IDs, the naming is pinned
  by the outcomes test and documented in the runtime-oracle guide.
- Minors recorded for the final whole-branch review: (1) the
  `domainEventObjectsDecodeRecords` doc comment claims absent-key vs
  empty-list stay distinguishable on the typed field, which is inaccurate
  (both decode to an empty slice; only JSON `null` errors); (2) case labels
  hard-code 98303/98304 while the mutation derives the bound from core
  geometry constants (theoretical coupling to a frozen contract).
- Architecture skill: no change.

## 2026-09-22 — node 1.3 acceptance (Go evidence stage closed)

- Implementation commit: `48c49e13` (`test(runtime-oracle): add chat event
  evidence`, 92 files). Red evidence verified: routing regression failed with
  `unknown rule "chat"` on the node 1.2 baseline, matching the intended
  behavioral red.
- Controller gates at `48c49e13`: focused chat tests ok; full
  `go test ./packages/tools/cmd/runtime-oracle -race -count=1` ok (63.8s —
  shared router proves all twelve event rules single-owned); five audit
  guards ok; `git diff --exit-code -- testdata/runtime-migration/contracts.json`
  clean; `git diff --check` clean. Implementer proved 88 external files
  byte-identical to tracked.
- Review ruling: spec compliant, quality Approved, 0 Critical, 0 Important.
  Reviewer's named-risk check verified the classifier mirrors
  `ChatEvent.Validate` (`message_companion.go:157-220`) branch-for-branch,
  including per-branch sub-orders and text-bound predicates, with bounds
  aliased to the same constants the private validators use.
- Reviewer Minor 1 (validProducerIDs edit outside the brief's file list) is
  ratified by the controller's standing ruling from node 1.1 — the one-line
  producer registration is the mechanically required export-path step.
- Minors recorded for the final whole-branch review: (1) the
  outcomes test's category-coverage loop resolves categories through the
  rule→category table instead of the outcome's own category (transitivity
  holds via the illegal-combinations test); (2) local mirrors of
  protocol-private predicates (`domainEventChatValidPlayerName`,
  `domainEventChatValidTaskFailReason`) are drift-pinned only on the 44
  exercised inputs — inherent to the mandated classifier design.
- Architecture skill: no change.

## 2026-09-22 — node 2.1 acceptance

- Implementation commit: `e4ac00a4` (`feat(domain): add mob event values`).
  Red evidence verified: after interface setup, the permissive skeleton
  admitted depths + non-finite yaw + health 0 (returned `Ok`) and the
  precedence test failed for that behavioral reason, matching the packet's
  mandated two-stage red.
- Controller gates at `e4ac00a4`: `cargo fmt --all --check` ok;
  `--test event_mobs --locked` 18 passed / 0 failed;
  `--test runtime_contract production_manifest_has_no_codec_kernel_or_host_dependencies`
  ok; audit comment gates ok; `git diff --check` clean.
- Review ruling: spec compliant, quality Approved, 0 Critical, 0 Important.
  Reviewer verified all 15 public types field-for-field, both precedence
  orders, the six batch constructors against the frozen algorithm, derive
  discipline, `MAX_HEALTH` reuse (`super::player::MAX_HEALTH`), and module
  wiring matching the existing pattern.
- Minors recorded for the final whole-branch review: (1) the `BatchTooLarge`
  branch of the six new batch constructors has no direct coverage inside this
  node — plan-constrained, and node 2.4's eleven resource-bounds tests are the
  scheduled owner; (2) `hostile_spawn_records(count)` helper doc names the
  65-record call shape instead of the parameter.
- Architecture skill: no change.

## 2026-09-22 — node 2.2 acceptance

- Implementation commit: `a3420b8a` (`feat(domain): add object event values`).
  Red evidence verified: permissive skeleton admitted `block_index` 98304
  (returned `Ok`) before the exact check landed, matching the mandated
  behavioral red.
- Controller gates at `a3420b8a`: `cargo fmt --all --check` ok;
  `--test event_objects` 16 passed / 0 failed; `--test items_locations` 14
  passed (existing `DropId`/`ItemStack` behavior untouched);
  `--test runtime_contract production_manifest_has_no_codec_kernel_or_host_dependencies`
  ok; audit comment gates ok; `git diff --check` clean.
- Review ruling: spec compliant, quality Approved, 0 Critical, 0 Important.
  Reviewer's named-risk checks confirmed the `DomainError` edit is append-only
  with no exhaustive-match breakage, and `DropId`'s derived `Ord` really is
  dimension → chunk x → chunk z → slot → generation (the batch order key).
- Minors recorded for the final whole-branch review: (1) `MAX_CHUNK_BLOCK_INDEX`
  hard-codes 98304 instead of deriving from `sections::SECTIONS_PER_CHUNK`
  (consolidation candidate for a later node — implementer flagged it too);
  (2) the `BatchTooLarge` branch of the five new constructors has no direct
  4,097-rejection test inside this node — node 2.4's eleven resource-bounds
  tests are the scheduled owner.
- Architecture skill: no change.

## 2026-09-22 — node 2.3 acceptance

- Implementation commit: `fd2c7de9` (`feat(domain): add chat event union`).
  Red evidence verified: permissive constructor admitted `event_id: 0`
  (returned `Ok`) before the nonzero check landed, matching the mandated
  behavioral red.
- Controller gates at `fd2c7de9`: `cargo fmt --all --check` ok;
  `--test event_chat` 10 passed / 0 failed; `--test identity_values` 13
  passed; `--test runtime_contract production_manifest_has_no_codec_kernel_or_host_dependencies`
  ok; audit comment gates ok; `git diff --check` clean.
- Review ruling: spec compliant, quality Approved, 0 Critical, 0 Important.
  Reviewer verified the seven-variant `ChatBody`, six-state `TaskState`,
  five-reason `TaskFailure`, the zero-ID-only rejection, the wildcard-free
  16-branch exhaustive match (18 ok corpus cases collapse to 16 shapes — the
  1024-byte command and 256-byte speech are boundary duplicates), derive
  discipline against crate precedent, and fixture identity matching the Go
  producer byte-for-byte.
- Minor recorded for the final whole-branch review: the absence test
  `chat_event_has_no_recipient_tick_reason_byte_or_command_sequence`
  duplicates the getter readback of the keeps test; its independent content
  is the four-field parts literal (mandated name, inherent to an absence
  test).
- Architecture skill: no change.
