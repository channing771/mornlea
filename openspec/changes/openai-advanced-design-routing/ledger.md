# Change Ledger

## Scope and routing ruling

- This change adds a hard provider/model/effort gate for code architecture, feature design, and high-level durable system decisions.
- The current advanced OpenAI route is the highest eligible OpenAI model under the project ceiling, `gpt-5.6-sol`, at `high` or `max` reasoning.
- Classification and OpenAI filtering happen before the existing six-axis score and before Z Code quota weighting.
- Ordinary bounded implementation, extraction, and review work retains the existing quota-aware Z Code policy outside the 14:00–18:00 blackout.

## Execution shape

- Execution shape: direct. The change is a tightly coupled governance-document and audit update with synchronized file ownership; delegation would add handoff cost without improving validation.
- Backend: native controller. No Z Code inference was needed or authorized for this policy update.
- Fallback ruling: if no compliant advanced OpenAI configuration is exposed for a high-level task, report an unavailable route; never silently substitute Z Code.

## Validation

- Focused Node bridge suites, focused Go audit, synchronized `cmp`, `git diff --check`, `make test-race`, `make dev-check`, and strict OpenSpec validation are recorded in the closeout addendum below.

## Retrospective

- Architecture skill: no change. This is a provider-routing governance rule and does not alter Mornlea game ownership or runtime architecture.
- Model router: update required. The user supplied a reusable, cross-task provider constraint for high-level design work; both project copies must encode it.

## Closeout addendum

- The 14:00–18:00 local Z Code blackout remains an independent ordinary-task filter; the OpenAI hard gate is evaluated before it and before quota scoring for high-level work.
- `node --test .codex/skills/adaptive-model-router/scripts/zcode-agent.test.mjs .codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs`: passed, 35 tests.
- `go test ./packages/audit -run 'TestProjectAdaptiveModelRouter|TestProjectRouter' -count=1`: passed.
- All synchronized `.codex`/`.claude` skill, reference, and prompt files passed `cmp`; the stale inverted-window wording scan found no matches; `git diff --check` passed.
- `make test-race`: passed Rust release build and all six Go module race-test loops.
- `make dev-check`: vet passed, but the short-test phase reproduced the pre-existing unchanged `packages/server/server` failure `TestWarpParityMemoryVsTCP` (`unsupported passive dimension 1`); cleanup reported the same passive-storage error. No exemption or gate bypass was used.
- `openspec validate --all --strict --no-interactive`: passed, 117 items.
- Architecture skill: no change. No stable game ownership, dependency, lifecycle, or ABI rule was discovered.
- Model router: no further change. The hard provider/model/effort gate is the reusable routing improvement; this round found no additional over-routing, under-routing, retry, or validation rule to promote.
