# Node 6.2: Full-branch review and local acceptance

## Identity and readiness

- Direct predecessor: Node 6.1.
- Deliverable: two fresh independent reviews find no unresolved Critical or Important issue, every accepted finding is repaired at its owner, and the complete stage-boundary suite passes on one recorded branch head.
- Required sub-skills: `superpowers:requesting-code-review`, `superpowers:verification-before-completion`, and `superpowers:systematic-debugging` for any failure.

## Ownership and review contract

- No production file is pre-owned. A finding reopens the narrow originating node and exact files needed for its test-first fix.
- Controller-only files: active change `tasks.md` and `ledger.md`.
- Review baseline: repository fork point `3401fa7a`; review all branch commits and the effective tree, not only the last node.
- One fresh reviewer checks code, shell/workflow security, data flow, artifacts, races, cold-cache behavior, and tests. A second fresh reviewer checks proposal/spec/design/task coverage, producer/consumer interfaces, version impact, rollback, active-plan consistency, and archival readiness. Neither edits.
- Every finding is classified Critical/Important/Minor with file/line evidence. Accepted Critical/Important findings block hosted acceptance. Accepted Minor findings are fixed or explicitly ruled with cost; no finding is silently deferred.

## Focused acceptance

Run from a clean worktree:

```bash
make ci-preflight
go test ./packages/audit -count=1
scripts/godot/validate-project.sh
scripts/godot/sync-assets.sh --check
scripts/godot/rollback-check.sh
bash -n scripts/ci/*.sh scripts/godot/build-extension.sh scripts/godot/fetch.sh scripts/godot/validate-project.sh
openspec validate redesign-ci-standards --strict --no-interactive
```

## Stage-boundary acceptance

Run every command and record its actual result and tested SHA:

```bash
make rust-check
make frontend-check
make companion-agent-check
make companion-agent-integration
make dev-check
make test-race-short
make test-race
openspec validate --all --strict --no-interactive
git diff --check origin/main...HEAD
```

`make test-race` is mandatory across all six modules. A failure is reproduced and root-caused; do not label it inherited or flaky without the repository's documented isolated rerun evidence. Automated tests must not launch or focus a foreground game window.

## Closure

- Re-review every repaired finding with the original reviewer until no Critical or Important issue remains.
- Confirm protocol/save/schema/ABI/benchmark/visual versions are unchanged.
- Controller records review rulings, exact commands/results, and the round-end architecture-skill decision, then commits only `tasks.md` and `ledger.md` with `docs(ci): record validation redesign evidence`.
- Rollback unit: each scoped review fix separately; evidence-only planning commit has no runtime effect.
