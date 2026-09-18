## 1. Quota-aware policy contract

- [x] 1.1 Add `openspec/changes/adaptive-zcode-allocation/specs/zcode-quota-aware-routing/spec.md` with quota normalization, bounded score modifiers, unknown/exhausted behavior, local-time eligibility, and credential-safe scenarios; validate with `openspec validate --all --strict --no-interactive`.
- [x] 1.2 Add `proposal.md` and `design.md` describing the affected synchronized skills, rejected alternatives, compatibility boundaries, and rollback; validate the change with `openspec status --change adaptive-zcode-allocation --json`.

## 2. Synchronized router guidance and audit

- [x] 2.1 Update `.codex/skills/adaptive-model-router/SKILL.md` and `.claude/skills/adaptive-model-router/SKILL.md` with the deterministic Z Code prior, quota factor, freshness rules, and 14:00–18:00 eligibility policy; validate with `cmp` and the skill-local Node suites.
- [x] 2.2 Update `.codex/skills/adaptive-model-router/references/capability-discovery.md` and its Claude copy with the read-only quota-source contract; validate with `cmp` and `git diff --check`.
- [x] 2.3 Extend `packages/audit/orchestration_skill_test.go` so synchronized policy fragments cover quota, safety, freshness, fallback, and local-time behavior; validate with `go test ./packages/audit -run 'TestProjectAdaptiveModelRouter|TestProjectRouter' -count=1`.

## 3. Closeout validation

- [x] 3.1 Run `node --test .codex/skills/adaptive-model-router/scripts/zcode-agent.test.mjs .codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs` and record the result in `ledger.md`.
- [x] 3.2 Run `make dev-check` and record any pre-existing unrelated failure without weakening gates; run `make test-race` for the six Go modules and record the result in `ledger.md`.
- [x] 3.3 Run `openspec validate --all --strict --no-interactive`, confirm `git diff --check`, and record architecture and router retrospectives in `ledger.md`.
