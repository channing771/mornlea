## Context

See `proposal.md` and `specs/zcode-quota-aware-routing/spec.md` for the motivation and observable contract. The project-owned router is a synchronized pair of Markdown skills with a capability-discovery reference and a Go audit that prevents policy drift. The Z Code bridge already verifies the configured `GLM-5.3` worker, but no stable provider quota endpoint is available to the repository and a probe is not a safe substitute for an account-usage read.

## Goals / Non-Goals

**Goals:**

- Make the Z Code preference modest, deterministic, and bounded.
- Let a current non-secret quota snapshot increase or reduce the Z Code score without confusing capability with entitlement.
- Enforce the local 14:00–18:00 Z Code blackout while preserving ordinary capability, provider, user, and validation constraints.
- Keep both skill copies independently usable and byte-identical.
- Make stale, unavailable, and exhausted quota decisions explicit and auditable.

**Non-Goals:**

- Discovering or guessing an undocumented Z Code billing API.
- Sending inference traffic to estimate remaining quota.
- Persisting credentials, raw account responses, or a permanent quota value in the repository.
- Changing the bridge protocol, native model enum, game runtime, saves, network contracts, ABI versions, or concurrency ceiling.
- Guaranteeing that Z Code wins every eligible task.

## Decisions

### Keep quota as a normalized routing input

The policy accepts `used`, `limit`, `remaining`, `observed_at`, and optional `reset_at` from a read-only host/provider surface. `remaining` is derived only when both `used` and `limit` are valid. A positive `limit` and an observation no older than 15 minutes are required for a fresh value. The router clamps the ratio so an over-report or clock-independent negative remainder cannot produce an unbounded score.

An unavailable or stale source maps to ratio `0.50`, which preserves a small preference without claiming that capacity is full or depleted. Confirmed zero remaining and an actual provider rate limit are distinct hard signals and remove Z Code for the current decision. This avoids both accidental starvation from missing telemetry and waste after a real exhaustion response.

### Apply a bounded modifier after existing score axes

The existing six-axis score remains the primary fit and risk signal. The new modifier uses a `1.06` prior and a quota factor from `0.90` to `1.10`; it cannot override explicit user constraints or the native fallback rules. The factor is applied after filtering incompatible candidates and after calculating the existing base score. The local 14:00–18:00 check happens before scoring and removes Z Code entirely; outside the disabled window, a constant `time factor = 1.00` prevents time-of-day from silently becoming a second quota heuristic.

The prior and bounds are policy judgments rather than model-accuracy claims. They give healthy Z Code a greater overall opportunity while allowing low remaining capacity to reduce allocation pressure. Future measured evaluations may revise them through another OpenSpec change.

### Keep the source of truth in synchronized skills

The policy and quota-discovery guidance are edited in `.codex/skills/adaptive-model-router/` and copied byte-for-byte to `.claude/skills/adaptive-model-router/`. The Go audit checks representative formulas, freshness, safety, time-window, and fallback language in the Codex copy and checks byte equality for both copies. No runtime router or provider client is added because the available Z Code host does not expose a verified quota API.

### Record only safe decision metadata

When a controller records a routing decision, it may include the normalized ratio, freshness classification, local timezone, and observation timestamp. It must not include credentials or raw provider payloads. This keeps the policy useful for review while preserving the existing bridge credential boundary.

### Rejected alternatives

- **Use a fixed Z Code percentage or random sampling:** rejected because it ignores current entitlement and makes routing hard to reproduce.
- **Treat missing quota as zero:** rejected because a telemetry outage would incorrectly disable a healthy worker.
- **Probe quota with a model request:** rejected because it spends capacity and cannot reliably distinguish quota from provider failure.
- **Leave Z Code enabled from 14:00–18:00:** rejected because the requested policy explicitly reserves that local window and needs a deterministic pre-score exclusion.
- **Add an undocumented provider API client:** rejected because the repository has no verified contract and would create credential and compatibility risk.

## Risks / Trade-offs

- **Quota snapshots can lag provider accounting** → Bound freshness to 15 minutes, keep the ratio conservative, and honor actual rate-limit responses immediately.
- **A prior can select Z Code for an unsuitable task** → Filter explicit constraints first, retain six-axis fit and validation gates, and keep native fallback for high-consequence or weak-oracle work.
- **Skill copies can drift** → Keep the files synchronized and enforce byte equality plus policy-fragment audits.
- **Unknown quota may spend capacity while telemetry is unavailable** → Use only the small prior with a neutral ratio and never infer full capacity from missing data.

## Migration Plan

1. Update the two synchronized router policy files and capability-discovery references.
2. Extend `packages/audit/orchestration_skill_test.go` with stable policy fragments that cover the formula, freshness, safety, fallback, and afternoon eligibility rules.
3. Run the focused Node bridge suites, focused Go audit tests, strict OpenSpec validation, and repository diff checks.
4. Keep existing synchronous and live Z Code bridge commands unchanged; rollback removes only the new policy and audit fragments.

No save, network, protocol, ABI, or data migration is required.
