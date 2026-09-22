# Controller Review, Publication and Archive

Node 5.1 is controller-owned. It is not delegated to an implementation worker.
The controller uses Superpowers `verification-before-completion` before any
completion claim, `requesting-code-review` for the whole-range review and
`receiving-code-review` to reproduce and adjudicate every finding. A failure
uses `systematic-debugging` and leaves the task open. Publication uses the
project `mornlea-architecture`, `openspec-sync-specs` and
`openspec-archive-change` skills under their exact instructions.

`tasks.md` remains the only checkbox/status source. `ledger.md` is append-only
evidence and never substitutes for an open checkbox.

## Controller acceptance contract for tasks 1.1–4.3

Before node 5.1, the controller accepts each predecessor separately. For each
node it must:

1. verify the named red test failed for the expected behavioral reason, not a
   missing compiler symbol, malformed fixture or unrelated gate;
2. inspect the node's exclusive diff and confirm later-node files and
   out-of-scope protocol/runtime code are absent;
3. run every focused command in that node's packet and record exact pass/count
   output;
4. obtain an independent review with no unresolved Critical or Important
   finding; reproduce disputed findings before accepting or rejecting them;
5. create the scoped implementation commit named in the packet, append the
   commit SHA, commands, result counts, review ruling and architecture-skill
   ruling to `ledger.md`, mark only that task complete, and commit those
   controller-owned status files as
   `docs(openspec): record <node-slug> acceptance`;
6. require a clean tracked tree before the successor begins. Untracked files
   are acceptable only when their owner and exclusion are recorded in the
   ledger and they do not overlap this change.

The implementation workflow may be native or subagent-driven as selected by
the user after plan approval, but an implementer never edits its own checkbox
or acceptance entry. A contract-changing finding returns to the controller to
reconcile `proposal.md`, the delta spec, `design.md`, `tasks.md` and every
affected packet before implementation resumes.

## Node 5.1: Review, validate, sync and archive the event successor

**Deliverable and prerequisites**

After tasks 1.1–4.3 are accepted and committed, review the complete change
range, run all stage gates at one clean code-result SHA, publish only the
implemented event-domain capability, archive only
`rust-domain-event-completion`, and leave the Rust authoritative-server F2
change explicitly blocked on the remaining F1 successors and final F1
acceptance.

**Controller-owned files and operations**

- `openspec/changes/rust-domain-event-completion/tasks.md`
- `openspec/changes/rust-domain-event-completion/ledger.md`
- `openspec/specs/rust-runtime-foundation/spec.md` through the sync skill
- `openspec/changes/rust-authoritative-server/{proposal.md,design.md,tasks.md,ledger.md}`
  only if final inspection finds its already-planned F1 dependency wording has
  drifted
- `.codex/skills/mornlea-architecture/SKILL.md` and its byte-identical
  `.claude/skills/mornlea-architecture/SKILL.md` mirror only when a verified
  durable cross-task architecture rule qualifies for promotion
- the exact dated archive destination selected by the archive skill

Production code, tests and corpus assets are read-only during closeout. A
verified code defect reopens its owning task, is repaired under that packet and
gets a new focused commit/review before node 5.1 restarts.

**Pre-review inventory**

Record the execution-start commit immediately before task 1.1 and the final
implementation commit immediately after task 4.3. Confirm the range contains
the 12 scoped implementation commits and their controller status commits,
every predecessor checkbox is complete, every ledger entry names its exact
commit and commands, and the tracked tree is clean.

Run mechanical inventory checks and save their exact results in the ledger:

```bash
git grep -nE 'pub (struct Observation|fn order_observations|const FAMILY_(INPUT|EVENT))' -- packages/engine/crates/mornlea_domain/src
git grep -n 'enum Event' -- packages/engine/crates/mornlea_domain/src/event.rs
git grep -n 'BaselineSourceRevision' -- packages/tools/cmd/runtime-oracle/inventory.go
git status --short
```

The first grep must return no production digest API. Inspection of the second
must find exactly the frozen 30 variants and no catch-all. The Go constant and
manifest `source_revision` must be identical 40-hex values, and the only
working-tree entries may be explicitly recorded unrelated user files.

**Whole-range independent review**

Request review of the exact execution-start..task-4.3 range against the
proposal, delta spec, design and frozen execution contract. The reviewer must
specifically determine:

- all 30 event variants carry checked semantic values, routing remains a
  separate session/broadcast envelope, and no protocol DTO, packet number,
  digest or authority behavior crossed into `mornlea_domain`;
- each of the eleven new batches checks 4,097 before record inspection,
  accepts 4,096, preserves order, rejects empty/duplicate/reversed inputs and
  does not import packet maxima 64/128/32 as semantic limits;
- all raw enum/union incompatibilities are classified before closed Rust
  construction while every representable rejection exercises the appropriate
  Rust constructor and exact `DomainError` mapping;
- Go producers execute current validators, never read their expected outputs,
  never rewrite tracked evidence, reject in-repository/symlink export roots
  and emit exactly 68, 45 and 44 nonzero cases;
- Rust dispatch executes exactly 533 unique domain cases and exactly 321
  `domain.event` cases once each, with 68/45/44 exact new-topic ownership and
  no missing/overlapping topic;
- all nine named semantic mutations fail the live normalized comparison, not
  merely a path/hash/parser check;
- the canonical manifest has exactly 30 unique sorted `domain.event` sources,
  valid hashes, consistent family/top-level case lists and the exact matching
  source revision;
- protocol v45, all save/ABI/scenario versions, Go online authority and default
  startup paths remain unchanged; and
- there is no protocol/storage conversion, public numerical API, pathfinding,
  Rust server runtime or F2 implementation in the range.

Use `receiving-code-review` for every finding. Reproduce it on the reviewed
SHA, fix verified blockers only in their owning node, request re-review and
restart the result-SHA gates. Node 5.1 remains open until the final review has
zero unresolved Critical or Important findings. Record Minor findings with an
explicit fix-now or defer ruling and named successor; silence is not a ruling.

**Stage validation at one code-result SHA**

Start from a clean tracked tree. Record `git rev-parse HEAD` as the code-result
SHA, then run the following without source edits between commands:

```bash
git diff --check
make rust
go test ./packages/tools/cmd/runtime-oracle -race -count=1
rustup run 1.97.1 cargo fmt --manifest-path packages/engine/Cargo.toml --all --check
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_mobs --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_objects --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_chat --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test resource_bounds --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test event_surface --locked
rustup run 1.97.1 cargo test --manifest-path packages/engine/Cargo.toml -p mornlea_domain --test corpus_domain --locked
go test ./packages/audit -count=1
make rust-check
make test-race
make dev-check
openspec validate rust-domain-event-completion --strict --no-interactive
openspec validate --all --strict --no-interactive
git diff --exit-code -- testdata/runtime-migration
git status --short
```

No exemption variable, skipped test, graphical window, live network authority
or live save is allowed. The two full stage gates must be green; the user's
earlier permission to merge PR #184 despite its then-current CI errors does not
waive this new change's acceptance contract. Record one actual result line per
command tied to the code-result SHA. Any red command blocks sync and archive.

**Architecture retrospective and canonical sync**

Review only verified implementation discoveries through
`mornlea-architecture`. Promote a rule only when it is stable across future
tasks and backed by current code, tests or the canonical specification. If
nothing qualifies, append `Architecture skill: no change` and a concise reason
to the ledger. If a rule qualifies, patch both skill copies atomically and
require:

```bash
cmp -s .codex/skills/mornlea-architecture/SKILL.md .claude/skills/mornlea-architecture/SKILL.md
```

Then use `openspec-sync-specs` to apply only this change's implemented delta to
`openspec/specs/rust-runtime-foundation/spec.md`. Inspect the canonical diff
line by line: it may claim the typed missing event values, exact event/routing
surface, shared batch policy and executable evidence, but it must not claim
protocol/save completion, numerical/pathfinding completion, online Rust
authority, complete F1 or F2 readiness. Rerun both strict OpenSpec validations
after sync without an intermediate commit.

Inspect `rust-authoritative-server` after sync. Its proposal/design/tasks and
ledger must still say F2 waits for complete F1, including protocol, storage,
safe numerical APIs, pathfinding and final zero-gap acceptance; the archived
baseline plus this event successor alone are insufficient. Repair only wording
drift, not F2 scope.

**Status, archive and final commit sequence**

While the active change still exists:

1. append the exact reviewed range, code-result SHA, every stage result,
   source revision, counts `533/321/68/45/44/30/9`, review rulings, architecture
   decision, canonical-sync diff and F2 dependency inspection to `ledger.md`;
2. mark task 5.1 complete in `tasks.md` and require all 12 checkboxes complete;
3. resolve the archive skill's date-based target, require the active directory
   to exist and the target not to exist, then use `openspec-archive-change` to
   move exactly `rust-domain-event-completion`;
4. confirm the active path is absent and the exact archive path exists, run
   `openspec validate --all --strict --no-interactive` and `git diff --check`
   on the post-archive tree;
5. stage only the canonical spec, archived change, any necessary F2 wording
   correction and the two architecture-skill copies if promoted; inspect
   `git diff --cached --stat` and `git diff --cached --check`;
6. create one English one-line commit proposed as
   `docs(openspec): close domain event completion` and require a clean tracked
   tree afterward.

The archived change and canonical spec become the accepted dependency for the
next F1 successor. No step marks any `rust-authoritative-server` implementation
task complete.

**Failure recovery**

Before the final commit, an incorrect spec sync is reversed only by an exact
`apply_patch` restoring the recorded pre-sync canonical content. Do not use a
broad checkout/reset.

If the archive tool reports non-success, inspect exact paths before acting. If
the active path still exists, leave both paths in place, reverse only the sync
and conditional skill/F2 documentation hunks, return task 5.1 to open, append
the failure to the active ledger, rerun strict validation and stop. If the
active path is absent and the exact target exists, the move succeeded; record
that fact in the archived ledger and continue post-archive validation rather
than moving again. Any other path state is reported without deletion or an
invented recovery.

If post-archive validation fails, verify the exact path state, move the exact
archive directory back to the active name, reverse the canonical/conditional
hunks with `apply_patch`, return task 5.1 to open, append the evidence to the
restored ledger and stop. No recovery deletes evidence, changes a runtime
version, widens authority or treats an incomplete gate as completion.
