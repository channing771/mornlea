# Node 6.4: Spec synchronization, archive, and final head

## Identity and readiness

- Direct predecessor: Node 6.3's evidence-only head is green for required and optional acceptance.
- Deliverable: delta specifications are intelligently synchronized, the completed change is archived with all evidence, the archive commit is pushed, and the final PR head is green.
- Required skills: `openspec-archive-change`, `openspec-sync-specs`, and `superpowers:verification-before-completion`.

## Archive contract

- Using change: `redesign-ci-standards` (override only if the user explicitly names another change).
- Run `openspec instructions archive --change redesign-ci-standards --json`, then `openspec status --change redesign-ci-standards --json`; apply relevant advisory context without weakening built-in checks.
- Use `artifactPaths.specs.existingOutputPaths` as the only delta set. Compare every delta to its main spec and synchronize all needed changes using one valid `openspec instructions specs` rule snapshot. Preserve unrelated requirements/scenarios and verify every capability is fully synced before moving the change.
- Immediately before archive, controller marks Node 6.4 complete and adds the pre-archive ledger/architecture ruling in the same uncommitted operation. This lets the archive check observe a complete task list without a false intermediate commit.
- Move the change to the date-derived archive path, preserving all artifacts and the ledger. Do not archive `godot-production-tooling` or any other active change.

## Validation, commit, and final acceptance

Run after sync and move:

```bash
openspec validate --all --strict --no-interactive
git diff --check
git status --short
```

Commit only synchronized main specs, archived change artifacts, and the final task/ledger state with `docs(ci): archive layered validation standard`. Push normally.

Watch the new exact PR head until these are green again:

```text
Required CI / merge-gate
Godot CI / godot-static
Godot CI / godot-runtime
```

Do not create a follow-up evidence commit after this final run; report the exact SHA, URLs, durations, and conclusions in the user handoff so acceptance does not become self-referential. Leave the PR open and the worktree intact. Branch protection remains an explicitly external rollout step and is not changed.

## Closure

- Confirm the archive contains completed tasks and the full ledger, main specs contain no delta-operation headers, the active change no longer appears in `openspec list`, and all active/archived specs validate.
- Report PR URL, archive path, final exact-head evidence, unchanged contract versions, and any truly external repository-setting step.
