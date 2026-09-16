---
name: adaptive-model-router
description: Select the lowest-cost live model and reasoning effort sufficient for Mornlea delegated work while protecting code quality and token efficiency.
---

# Adaptive Model Router

Use this project-owned skill before every new Mornlea subagent delegation. It is a required project governance rule and complements `mornlea-implementation-orchestration`: orchestration decides whether context isolation is justified; this skill selects the model and reasoning effort for that isolated work.

The routing objectives are coequal: protect code quality and token efficiency. Do not downshift below credible sufficiency merely to save tokens, and do not use the highest model or effort by default when a lower tier can produce a verifiable result.

## Discover the Live Capability Set

Treat the current invocation surface as authoritative. Read the native agent tool's model and reasoning-effort enums or another read-only, account-scoped capability source. Do not route from a remembered or copied model catalog.

When availability or effort support is unclear, read [capability discovery](references/capability-discovery.md). Keep discovery read-only: do not modify configuration, install providers, buy capacity, redeem credits, or send paid probes merely to compare options.

## Apply Constraints and Project Ceilings

Honor explicit user constraints for provider, model, effort, budget, latency, privacy, region, and tools before optimizing. An explicit user selection is not silently replaced.

For an OpenAI-backed model, the highest eligible model is `gpt-5.6-sol` and the highest eligible reasoning effort is `max`. Options above either boundary may be visible but remain ineligible unless the user changes this project rule. A compatible gateway does not remove the ceiling. This ceiling does not apply to non-OpenAI providers; route those options within their own verified constraints.

If provider identity or model ordering cannot be verified, retain the current eligible configuration rather than guessing. Do not describe inheritance as adaptive optimization.

## Route from Multiple Angles

Assess the isolated task on independent axes:

- reasoning difficulty, ambiguity, and novelty;
- consequence of an incorrect result and ease of rollback;
- context breadth, dependency depth, and architectural coupling;
- expected tool-call horizon and autonomy;
- required modalities and tool support;
- strength and speed of the available validation oracle;
- user preferences for speed, cost, and quality.

Filter incompatible or ineligible options first. From the remaining live host capability set, choose the lowest-cost configuration that is credibly sufficient. Model capability and reasoning effort are separate decisions: a larger model does not automatically require maximum effort.

Use semantic bands only as routing guidance, then select exact values exposed by the host:

- Lowest available effort: extraction, formatting, classification, deterministic edits, and narrow checks with strong validation.
- Low: bounded research, small isolated code changes, and straightforward transformations.
- Medium: ordinary implementation, synthesis across several sources, debugging with a plausible hypothesis, and multi-step tool use.
- High: ambiguous diagnosis, architecture, security or correctness review, and consequential work with interacting constraints.
- Above high: only the hardest eligible work when failure is costly, evaluation is difficult, and the added token and latency cost is justified.

## Apply, Verify, and Adjust

Pass the exact selected model and effort through the native delegation surface. If the host returns resolved values, treat them as truth. When an override requires a smaller context fork, preserve the evidence, paths, constraints, ownership, and acceptance criteria needed for a correct result.

Escalate only one eligible capability or effort step after observable insufficiency, such as failed validation, unresolved contradictions, repeated planning failure, or material uncertainty. Prefer targeted verification and follow-up over restarting completed expensive work. Downshift later independent tasks when the work becomes repetitive or mechanically verifiable.

Record a compact routing decision in the change ledger when delegation occurs:

```text
model: <exact resolved identifier or inherited>
effort: <exact supported value or provider default>
basis: <task axes and capability source>
fallback: <next eligible configuration if validation fails>
```

Distinguish host-reported facts from routing judgment.

## Round-End Router Retrospective

At the end of every implementation round, evaluate routing alongside the architecture retrospective. Review both over-routing and under-routing:

- Was a model or effort tier higher than the task and its validation oracle required?
- Did a lower choice cause retries, contradictions, missing context, weak review, or avoidable escalation?
- Did context transfer, tool support, latency, or token use materially affect quality?
- Would a reusable axis, constraint, or escalation rule improve future routing?

Update both project-owned skill copies only when verified evidence supports a stable cross-task rule that changes future routing. Do not add task history, volatile availability lists, one-off model anecdotes, or unverified preferences. Validate and synchronize both copies after an update. If no reusable improvement qualifies, record `Model router: no change` with a short reason in the change ledger.
