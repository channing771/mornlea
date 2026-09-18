# Change Ledger

## Scope and implementation

- The change is limited to synchronized project-owned routing guidance, capability-discovery guidance, and the corresponding governance audit. No game runtime, save, network, protocol, ABI, or benchmark behavior changes.
- The implemented policy uses `Z Code prior = 1.06`, `quota factor = 0.90 + 0.20 × quota ratio`, and `time factor = 1.00`.
- The quota input is read-only and non-secret. Freshness is limited to 15 minutes; unknown quota uses ratio `0.50`; confirmed zero remaining or a provider rate limit removes Z Code for the current decision.
- The local 14:00–18:00 window has no blackout. A healthy, non-exhausted Z Code candidate remains eligible and is still subject to ordinary fit, validation, user, and provider constraints.

## Routing decision

- Execution shape: direct, because the work is a tightly coupled documentation-and-audit change with synchronized file ownership and deterministic validation.
- Backend decision for implementation: native controller, no external inference request; the work did not require a delegated implementation or private protocol investigation.
- Fallback: preserve native routing for high-consequence or weak-oracle tasks and for any actual Z Code capability, provider, user, validation, or confirmed-quota failure.

## Validation

- `node --test .codex/skills/adaptive-model-router/scripts/zcode-agent.test.mjs .codex/skills/adaptive-model-router/scripts/zcode-live-agent.test.mjs`: passed, 35 tests.
- `node .codex/skills/adaptive-model-router/scripts/zcode-agent.mjs probe`: passed and reported the configured `builtin:bigmodel-coding-plan/GLM-5.3` worker without disclosing credentials.
- `node .codex/skills/adaptive-model-router/scripts/zcode-agent.mjs live-probe --cwd /Users/chen/work/mornlea`: the installed CLI exited with `child_exit` because it could not locate its bundled `zcode-builtin.json`; no model prompt or inference request was sent, so live supervision remains ineligible until the host installation is repaired.
- `go test ./packages/audit -run 'TestProjectAdaptiveModelRouter|TestProjectRouter' -count=1`: passed.
- `cmp .codex/skills/adaptive-model-router/SKILL.md .claude/skills/adaptive-model-router/SKILL.md`: passed.
- `cmp .codex/skills/adaptive-model-router/references/capability-discovery.md .claude/skills/adaptive-model-router/references/capability-discovery.md`: passed.
- `git diff --check`: passed.
- `make test-race`: passed for the Rust release build and all six Go module loops; package results were a mix of fresh and cached runs.
- `make dev-check`: the vet phase completed, but the short-test phase reproduced the pre-existing unchanged `packages/server/server` failure `TestWarpParityMemoryVsTCP` (`unsupported passive dimension 1`) and cleanup reported the same passive-storage error. No exemption or gate bypass was used.
- `openspec validate --all --strict --no-interactive`: passed, 116 items.

## Review and rulings

- The policy is intentionally advisory: score modifiers cannot bypass explicit constraints or risk-based native fallback.
- Current quota is never hardcoded into the repository. A live host/provider snapshot is consumed per routing decision, and an unavailable source is neutral rather than exhausted.
- Credentials and raw account responses remain outside skill text, ledger output, and routing records.
- Architecture skill: no change. This is a developer-tool policy and does not establish a new game ownership, dependency, lifecycle, or ABI boundary.
- Model router: updated in both synchronized copies because the quota-aware prior, freshness treatment, and afternoon eligibility are reusable routing rules backed by the requested behavior and focused policy tests.
