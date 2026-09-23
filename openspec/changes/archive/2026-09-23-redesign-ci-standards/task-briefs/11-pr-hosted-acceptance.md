# Node 6.3: Pull request and hosted acceptance

## Identity and readiness

- Direct predecessor: Node 6.2 with a clean, fully verified branch.
- Deliverable: the implementation is pushed in an open pull request and one exact candidate head is green for the required merge gate and both optional Godot jobs.
- Required sub-skills: `superpowers:finishing-a-development-branch`, `superpowers:verification-before-completion`, and `superpowers:systematic-debugging` for every hosted failure.

## Integration contract

- The user already selected the skill's “Push and create a Pull Request” outcome; do not ask again or merge locally.
- Worktree: `/Users/chen/.codex/worktrees/redesign-ci-standards-impl/mornlea`.
- Branch: `codex/redesign-ci-standards-impl`; base: `main`. Preserve the worktree for PR repairs.
- Push without force. Create or update one PR titled `refactor(ci): layer platform-owned validation` using `.github/PULL_REQUEST_TEMPLATE.md` in English. Summary includes contract/version impact; validation has one actual command/result per line; no generated signature.
- Attach the PR to the current task immediately after creation.
- Do not mutate branch protection or merge. The intended protected status remains exactly `Required CI / merge-gate`.

## Hosted acceptance

1. Record the exact pushed SHA. Watch checks without silent retries. Acceptance requires:

   ```text
   Required CI / merge-gate
   Godot CI / godot-static
   Godot CI / godot-runtime
   ```

   The optional jobs are implementation acceptance but not merge authority.

2. Record run URLs, job durations, runner identities, and the exact SHA. If any job fails, inspect its original logs, reproduce locally where possible, use systematic debugging, add the smallest tested fix, push normally, and restart acceptance for the new exact head. Do not use exemption variables or “rerun failed jobs” as evidence for a code/config defect.

3. After a green candidate, controller marks Node 6.3 complete and records its evidence in `tasks.md`/`ledger.md`, commits with `docs(ci): record hosted validation evidence`, pushes, and requires the resulting evidence-only head to pass the same three statuses before Node 6.4 starts. This extra run prevents an unvalidated status commit from becoming the archive base.

## Closure

- Confirm `git status --short`, `git log --oneline origin/main..HEAD`, PR head SHA, and check conclusions.
- Rollback unit: hosted fixes remain individually revertible; PR/status evidence is planning-only.
