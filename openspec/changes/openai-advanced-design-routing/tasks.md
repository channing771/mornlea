## 1. High-level routing contract

- [x] 1.1 Add `openspec/changes/openai-advanced-design-routing/specs/openai-advanced-design-routing/spec.md` with classification, advanced OpenAI selection, ambiguity, unavailable-route, and audit scenarios; validate with `openspec validate --all --strict --no-interactive`.
- [x] 1.2 Add `proposal.md` and `design.md` defining the hard pre-scoring gate, current model ceiling, fallback behavior, and rollback; validate with `openspec status --change openai-advanced-design-routing --json`.

## 2. Synchronized policy and audit

- [x] 2.1 Update both `adaptive-model-router/SKILL.md` copies with the high-level classification gate and exact advanced OpenAI route; validate with `cmp` and the skill-local Node suites.
- [x] 2.2 Update both `capability-discovery.md` copies with the high-level gate and fail-closed capability rule; validate with `cmp` and `git diff --check`.
- [x] 2.3 Update both `agents/openai.yaml` prompts and `packages/audit/orchestration_skill_test.go` so the hard gate and synchronization are enforced; validate with the focused Go audit command.

## 3. Closeout

- [x] 3.1 Run `node --test .codex/skills/adaptive-model-router/scripts/zcode-agent.test.mjs .codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs` and record the result in `ledger.md`.
- [x] 3.2 Run `make test-race` and `make dev-check`, recording any pre-existing unrelated failure without bypassing gates.
- [x] 3.3 Run `openspec validate --all --strict --no-interactive`, `git diff --check`, and the round-end architecture/router retrospectives; record them in `ledger.md`.
