## Context

See `proposal.md` and `specs/openai-advanced-design-routing/spec.md` for motivation and observable behavior. The synchronized router currently filters explicit constraints, computes a six-axis score, and then applies a Z Code prior and quota factor. The new rule is a governance ordering constraint: high-level classification and OpenAI filtering happen before that existing score path.

## Goals / Non-Goals

**Goals:**

- Make advanced OpenAI routing mandatory for architecture, feature design, and durable high-level decisions.
- Preserve the existing Z Code quota policy for ordinary bounded tasks.
- Make ambiguous cross-package or contract-affecting work conservative.
- Keep the two project skill copies synchronized and independently understandable.
- Make routing decisions auditable without exposing credentials or raw account data.

**Non-Goals:**

- Adding a new OpenAI provider, changing the native delegation API, or changing the current project model ceiling.
- Re-evaluating model quality or claiming that `gpt-5.6-sol` is universally best.
- Disabling Z Code for mechanical edits, read-only extraction, or other tasks that do not affect high-level design.
- Changing quota collection, the 14:00–18:00 blackout rule, or the live supervisor protocol.

## Decisions

### Classify before scoring

The router will classify the task from its brief, requested outputs, affected boundaries, and consequence. Architecture signals include package/module boundaries, ownership/dependency direction, lifecycle, concurrency, protocol, ABI, storage, migration, and cross-language decisions. Feature-design signals include multi-component user behavior, durable contracts, and OpenSpec-level requirements or design. If a task may change a durable boundary and the brief is ambiguous, the conservative classification is high-level.

This classification is a hard candidate filter. It is intentionally evaluated before the six-axis score and before `Z Code prior × quota factor × time factor`, so quota cannot turn a non-compliant backend into a compliant one.

### Define the advanced OpenAI route using current capabilities

The project ceiling remains authoritative: the current highest eligible OpenAI model is `gpt-5.6-sol`, and the supported high-level reasoning efforts are `high` and `max`. The router must inspect the live native invocation surface and pass exact resolved values. If the current surface changes, the highest eligible OpenAI model under the existing ceiling becomes the advanced tier; if no advanced configuration is exposed, the route fails closed.

“Advanced” is a routing tier, not a vendor benchmark claim. The rule protects the class of work by provider, model tier, and reasoning effort while leaving ordinary tasks free to optimize cost and quota.

### Preserve existing Z Code allocation outside the gate

For non-high-level tasks, the prior, quota factor, unknown-quota behavior, rate-limit handling, and 14:00–18:00 blackout remain unchanged. The hard gate does not delete or weaken the existing Z Code bridge. It only removes Z Code from the candidate set when the task's design level requires the OpenAI quality floor; the time blackout independently removes it during the disabled window.

### Rejected alternatives

- **Give Z Code a larger weight for architecture tasks:** rejected because a weight is a preference, not a provider guarantee.
- **Use keywords alone:** rejected because architecture can be expressed without a fixed keyword and ordinary tasks can mention architecture incidentally; classification must consider requested outcome and affected boundaries.
- **Use any OpenAI model at any effort:** rejected because the user requested an advanced model and low effort does not provide the required quality floor.
- **Fall back to Z Code when the native model is unavailable:** rejected because it silently violates the hard provider requirement; the correct result is an unavailable-route report.
- **Remove Z Code entirely:** rejected because the user's rule is scoped to high-level work and the existing quota-aware policy remains valuable for bounded tasks.

## Risks / Trade-offs

- **Conservative classification may route some ordinary work to a stronger model** → Require evidence of durable design impact and record the classification for review.
- **Native OpenAI availability may be temporarily missing** → Fail closed with an actionable route error instead of silently violating the policy.
- **Skill copies can drift** → Update both copies atomically and retain byte-equality and fragment audits.
- **A model ceiling change could make the exact name stale** → Resolve the exact advanced model from the live native surface while keeping the current ceiling and policy wording synchronized.

## Migration Plan

1. Add the new hard-gate policy and capability-discovery guidance to both synchronized skills.
2. Extend `packages/audit/orchestration_skill_test.go` with exact classification, provider, model, effort, ordering, and fail-closed fragments.
3. Run focused Node bridge tests, focused audit tests, synchronized-copy checks, and strict OpenSpec validation.
4. Roll back by removing only the hard-gate section, prompt wording, audit fragments, and this change artifacts; the quota-aware Z Code policy remains intact.

No save, network, ABI, protocol, or data migration is required.
