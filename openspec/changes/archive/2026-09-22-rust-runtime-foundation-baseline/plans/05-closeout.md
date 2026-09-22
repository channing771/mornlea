# Controller closeout plan

Node 5.1 is controller-owned and is not delegated to an implementation worker.
The controller uses Superpowers `verification-before-completion` before any
completion claim, `requesting-code-review` for the whole-range review,
`receiving-code-review` to adjudicate every finding, and the project
`mornlea-architecture` skill to decide whether a stable cross-task rule should
be promoted. It then uses `openspec-sync-specs` and
`openspec-archive-change`; the archive directory move follows the archive
skill's exact procedure. If a gate fails, use `systematic-debugging` and leave
this task open.

## Node 5.1: Review, validate, sync and archive the extracted baseline

**Deliverable and prerequisites**

After nodes 1.2, 2.1, 2.2, 3.10 and 4.1 are accepted and committed, review the
entire extracted range, run all stage gates at one code-result SHA, publish the
narrow canonical capability, and archive only
`rust-runtime-foundation-baseline`. The broad `rust-runtime-foundation` parent
and its remaining nodes are not synchronized or archived here.

**Controller-owned files and operations**

- `openspec/changes/rust-runtime-foundation-baseline/tasks.md`
- `openspec/changes/rust-runtime-foundation-baseline/ledger.md`
- `openspec/specs/rust-runtime-foundation/spec.md` through the sync skill
- `.codex/skills/mornlea-architecture/SKILL.md`, only if a verified durable
  cross-task finding qualifies for promotion
- `.claude/skills/mornlea-architecture/SKILL.md`, the byte-identical atomic
  mirror of any qualifying promotion
- the archive destination selected by the archive skill

All production/test code and frozen evidence are read-only during closeout.
Only review-confirmed fixes reopen their owning implementation node.

**Per-node acceptance before the stage gate**

For every implementation node, the controller verifies the worker's red/green
evidence, checks its exclusive file diff, requests an independent code review,
adjudicates findings, reruns the named focused commands, and creates the scoped
implementation commit containing only that node's accepted code/test/guide
files. Only after Git returns the implementation SHA does the controller append
that SHA and the actual results to `ledger.md`, mark only that checkbox
complete, and create a second controller-only status commit named
`docs(openspec): record <node-slug> acceptance`. A worker never edits or
self-marks status. The next node starts only after both commits and a clean
tracked tree. Any finding that changes a shared contract first updates
`design.md` and the affected task packet in the status/planning commit; workers
do not invent a local policy. Rolling back an accepted node reverts its status
commit before its implementation commit so the ledger history and checkbox do
not claim code that is absent.

**Whole-range review**

Record the commit immediately before node 1.1 and the commit after node 4.1.
Request an independent review of that exact range against `proposal.md`, the
delta spec and `design.md`. The reviewer must specifically inspect:

- zero-case/version and unknown-consumer coverage paths;
- absence of expectation-derived observations or fail-open root handling;
- absence of tracked-corpus writers and replacement publication;
- Go/Rust loader agreement for the explicitly shared manifest/case subset,
  including duplicate keys, formats, consumers, paths and symlinks;
- exactly-once execution of 376 domain cases, exclusion of the one external
  authority case, and live mutation detection;
- byte/record checks before scans, copies, sorts and allocation;
- unchanged protocol v45, save schemas, ABI versions and default Go runtime.

The controller applies `receiving-code-review`: reproduce each claimed issue,
fix only verified blockers in the owning node, request re-review, and keep 5.1
open until no blocker remains.

**Stage validation at the code-result SHA**

Start from a clean tracked tree and record `git rev-parse HEAD` as the result
SHA. Run, in this order:

```bash
git diff --check
make rust
make rust-check
make test-race
make dev-check
openspec validate rust-runtime-foundation-baseline --strict --no-interactive
openspec validate --all --strict --no-interactive
git diff --exit-code -- testdata/runtime-migration
```

`make test-race` must cover all six Go modules; `make dev-check` supplies the
six-module vet and architecture gates. No exemption variable, skipped test,
graphical window, live network authority or live save is allowed. Record one
actual result line per command in `ledger.md`, tied to the result SHA. A failure
blocks sync/archive even if unrelated; document evidence and resolve or ask the
user rather than waiving it.

**Architecture and OpenSpec publication**

Review verified discoveries with `mornlea-architecture`. Promote only a stable
cross-task rule backed by current code/specs; otherwise append exactly
`Architecture skill: no change` plus the reason to the ledger. If a finding
qualifies, patch both listed skill copies in the same closeout diff and require
`cmp -s .codex/skills/mornlea-architecture/SKILL.md .claude/skills/mornlea-architecture/SKILL.md`;
a one-sided or byte-different promotion blocks archive. Then, without creating
an intermediate commit:

1. use `openspec-sync-specs` to create/update
   `openspec/specs/rust-runtime-foundation/spec.md` from this narrow delta;
2. inspect the canonical spec to ensure it contains no unimplemented protocol,
   storage, kernel or final-acceptance claims;
3. run both strict validation commands again after the documentation-only sync,
   always with `--no-interactive`;
4. resolve the exact dated archive target, require the active path to exist and
   that target not to exist, then mark task 5.1 complete while the active
   change still exists and append the exact result SHA, review outcome, sync
   decision, and archive target;
5. use `openspec-archive-change` to move
   `rust-runtime-foundation-baseline` to its dated archive destination;
6. confirm the active path is absent, the exact archive destination exists,
   and run `openspec validate --all --strict --no-interactive` from the
   post-archive tree;
7. run `git diff --check`, stage only the canonical spec, archived change and
   both architecture-skill copies when they changed, inspect the staged diff,
   rerun the byte-equality check when applicable, and create one final English
   one-line commit proposed as
   `docs(openspec): close runtime foundation baseline`.

The broad parent remains active only as decomposition history until the
seventeen successor OpenSpecs frozen in its `design.md` are scaffolded. Its
checked boxes must not be interpreted as re-review evidence after this baseline
archive; the archived baseline and canonical spec become the dependency.

**Expected results and rollback**

Closeout succeeds only when every checkbox is complete, the independent review
has no blocker, all commands pass, the canonical spec is narrow, the archive
exists, and the final commit contains both publication operations. Before the
final commit, recovery is explicit and local: if sync is wrong, reverse only
the canonical-spec hunk with `apply_patch` from the recorded pre-sync content.

Any archive-skill non-success triggers an exact path-state check before another
operation. If the active path still exists, do not move or delete either path:
reverse the canonical-spec hunk and any conditional two-copy architecture
promotion, change task 5.1 back to `[ ]`, append the failed archive result to
the active append-only ledger, verify the skill copies remain byte-identical,
rerun strict validation, and stop with the task open. If the active path is
absent and the exact target exists, the directory move completed before the
reported failure; record that fact in the archived ledger and continue with the
post-archive validation rather than attempting a second move. Any other
combination is an unexpected state that must be reported without destructive
recovery.

If post-archive validation fails, first verify that the active path is absent
and the exact dated archive target exists, move that exact directory back to
the active path, reverse the canonical-spec hunk and any two-copy architecture
promotion with `apply_patch`, change task 5.1 back to `[ ]`, append the failure
to the restored append-only ledger, verify the skill copies are byte-identical,
and stop with 5.1 open. Do not use a broad checkout/reset, delete a change
directory, or claim a recovery capability not provided by the archive skill.
No closeout action alters runtime code, versions or authority.
